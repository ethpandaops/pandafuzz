package memory

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// CacheEntry represents a single cache entry with value and metadata
type CacheEntry struct {
	Key         string
	Value       interface{}
	ExpiresAt   time.Time
	AccessTime  time.Time
	AccessCount int64
}

// CacheStats represents cache performance statistics
type CacheStats struct {
	Hits       int64
	Misses     int64
	Evictions  int64
	TotalItems int64
	SizeBytes  int64
}

// EvictionPolicy defines cache eviction strategies
type EvictionPolicy string

const (
	// LRU evicts least recently used items
	LRU EvictionPolicy = "lru"
	// LFU evicts least frequently used items
	LFU EvictionPolicy = "lfu"
	// FIFO evicts oldest items first
	FIFO EvictionPolicy = "fifo"
)

// CacheConfig contains cache configuration options
type CacheConfig struct {
	// MaxSize is the maximum number of items in cache (0 = unlimited)
	MaxSize int
	// DefaultTTL is the default time-to-live for entries
	DefaultTTL time.Duration
	// CleanupInterval is how often to run cleanup of expired entries
	CleanupInterval time.Duration
	// EvictionPolicy determines which items to remove when cache is full
	EvictionPolicy EvictionPolicy
	// OnEviction is called when an item is evicted
	OnEviction func(key string, value interface{})
}

// DefaultConfig returns a default cache configuration
func DefaultConfig() *CacheConfig {
	return &CacheConfig{
		MaxSize:         1000,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
		EvictionPolicy:  LRU,
		OnEviction:      nil,
	}
}

// Cache is a thread-safe in-memory cache with TTL support
type Cache struct {
	mu           sync.RWMutex
	items        map[string]*list.Element
	evictionList *list.List
	config       *CacheConfig
	stats        CacheStats
	stopCleanup  chan struct{}
	wg           sync.WaitGroup
}

// New creates a new cache instance
func New(config *CacheConfig) *Cache {
	if config == nil {
		config = DefaultConfig()
	}

	c := &Cache{
		items:        make(map[string]*list.Element),
		evictionList: list.New(),
		config:       config,
		stopCleanup:  make(chan struct{}),
	}

	// Start cleanup goroutine if cleanup interval is set
	if config.CleanupInterval > 0 {
		c.wg.Add(1)
		go c.cleanupExpired()
	}

	return c
}

// Get retrieves a value from the cache
func (c *Cache) Get(ctx context.Context, key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, found := c.items[key]
	if !found {
		atomic.AddInt64(&c.stats.Misses, 1)
		return nil, false
	}

	entry := elem.Value.(*CacheEntry)

	// Check if expired
	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		c.removeElement(elem)
		atomic.AddInt64(&c.stats.Misses, 1)
		return nil, false
	}

	// Update access time and count
	entry.AccessTime = time.Now()
	atomic.AddInt64(&entry.AccessCount, 1)

	// Move to front for LRU
	if c.config.EvictionPolicy == LRU {
		c.evictionList.MoveToFront(elem)
	}

	atomic.AddInt64(&c.stats.Hits, 1)
	return entry.Value, true
}

// Set stores a value in the cache with default TTL
func (c *Cache) Set(ctx context.Context, key string, value interface{}) error {
	return c.SetWithTTL(ctx, key, value, c.config.DefaultTTL)
}

// SetWithTTL stores a value in the cache with custom TTL
func (c *Cache) SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	entry := &CacheEntry{
		Key:        key,
		Value:      value,
		ExpiresAt:  expiresAt,
		AccessTime: time.Now(),
	}

	// Update existing entry
	if elem, exists := c.items[key]; exists {
		elem.Value = entry
		if c.config.EvictionPolicy == LRU {
			c.evictionList.MoveToFront(elem)
		}
		return nil
	}

	// Check capacity
	if c.config.MaxSize > 0 && c.evictionList.Len() >= c.config.MaxSize {
		c.evict()
	}

	// Add new entry
	elem := c.evictionList.PushFront(entry)
	c.items[key] = elem
	atomic.AddInt64(&c.stats.TotalItems, 1)

	return nil
}

// Delete removes a value from the cache
func (c *Cache) Delete(ctx context.Context, key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, found := c.items[key]
	if !found {
		return false
	}

	c.removeElement(elem)
	return true
}

// Clear removes all items from the cache
func (c *Cache) Clear(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.evictionList.Init()
	atomic.StoreInt64(&c.stats.TotalItems, 0)
}

// GetMulti retrieves multiple values from the cache
func (c *Cache) GetMulti(ctx context.Context, keys []string) map[string]interface{} {
	result := make(map[string]interface{})

	for _, key := range keys {
		if value, found := c.Get(ctx, key); found {
			result[key] = value
		}
	}

	return result
}

// SetMulti stores multiple values in the cache
func (c *Cache) SetMulti(ctx context.Context, items map[string]interface{}) error {
	for key, value := range items {
		if err := c.Set(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

// DeleteMulti removes multiple values from the cache
func (c *Cache) DeleteMulti(ctx context.Context, keys []string) int {
	deleted := 0
	for _, key := range keys {
		if c.Delete(ctx, key) {
			deleted++
		}
	}
	return deleted
}

// Stats returns cache statistics
func (c *Cache) Stats() CacheStats {
	return CacheStats{
		Hits:       atomic.LoadInt64(&c.stats.Hits),
		Misses:     atomic.LoadInt64(&c.stats.Misses),
		Evictions:  atomic.LoadInt64(&c.stats.Evictions),
		TotalItems: atomic.LoadInt64(&c.stats.TotalItems),
		SizeBytes:  atomic.LoadInt64(&c.stats.SizeBytes),
	}
}

// Size returns the current number of items in the cache
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.evictionList.Len()
}

// Keys returns all keys in the cache
func (c *Cache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.items))
	for k := range c.items {
		keys = append(keys, k)
	}
	return keys
}

// Preload warms the cache with initial data
func (c *Cache) Preload(ctx context.Context, items map[string]interface{}) error {
	return c.SetMulti(ctx, items)
}

// PreloadWithTTL warms the cache with initial data and custom TTL
func (c *Cache) PreloadWithTTL(ctx context.Context, items map[string]interface{}, ttl time.Duration) error {
	for key, value := range items {
		if err := c.SetWithTTL(ctx, key, value, ttl); err != nil {
			return err
		}
	}
	return nil
}

// Close stops the cache and cleanup routines
func (c *Cache) Close() error {
	close(c.stopCleanup)
	c.wg.Wait()
	return nil
}

// evict removes items based on eviction policy
func (c *Cache) evict() {
	if c.evictionList.Len() == 0 {
		return
	}

	var victim *list.Element

	switch c.config.EvictionPolicy {
	case LRU:
		// Remove from back (least recently used)
		victim = c.evictionList.Back()
	case LFU:
		// Find least frequently used
		victim = c.findLFUVictim()
	case FIFO:
		// Remove from back (oldest)
		victim = c.evictionList.Back()
	default:
		victim = c.evictionList.Back()
	}

	if victim != nil {
		c.removeElement(victim)
		atomic.AddInt64(&c.stats.Evictions, 1)
	}
}

// findLFUVictim finds the least frequently used item
func (c *Cache) findLFUVictim() *list.Element {
	var victim *list.Element
	var minCount int64 = -1

	for elem := c.evictionList.Back(); elem != nil; elem = elem.Prev() {
		entry := elem.Value.(*CacheEntry)
		if minCount == -1 || entry.AccessCount < minCount {
			minCount = entry.AccessCount
			victim = elem
		}
	}

	return victim
}

// removeElement removes an element from the cache
func (c *Cache) removeElement(elem *list.Element) {
	entry := elem.Value.(*CacheEntry)
	delete(c.items, entry.Key)
	c.evictionList.Remove(elem)
	atomic.AddInt64(&c.stats.TotalItems, -1)

	if c.config.OnEviction != nil {
		c.config.OnEviction(entry.Key, entry.Value)
	}
}

// cleanupExpired periodically removes expired entries
func (c *Cache) cleanupExpired() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.removeExpired()
		case <-c.stopCleanup:
			return
		}
	}
}

// removeExpired removes all expired entries
func (c *Cache) removeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var toRemove []*list.Element

	for elem := c.evictionList.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(*CacheEntry)
		if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
			toRemove = append(toRemove, elem)
		}
	}

	for _, elem := range toRemove {
		c.removeElement(elem)
	}
}

// HitRate returns the cache hit rate as a percentage
func (c *Cache) HitRate() float64 {
	hits := atomic.LoadInt64(&c.stats.Hits)
	misses := atomic.LoadInt64(&c.stats.Misses)
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total) * 100
}

// Exists checks if a key exists in the cache
func (c *Cache) Exists(ctx context.Context, key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	elem, found := c.items[key]
	if !found {
		return false
	}

	entry := elem.Value.(*CacheEntry)
	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		return false
	}

	return true
}

// TTL returns the remaining time-to-live for a key
func (c *Cache) TTL(ctx context.Context, key string) (time.Duration, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	elem, found := c.items[key]
	if !found {
		return 0, false
	}

	entry := elem.Value.(*CacheEntry)
	if entry.ExpiresAt.IsZero() {
		return 0, true // No expiration
	}

	ttl := time.Until(entry.ExpiresAt)
	if ttl < 0 {
		return 0, false
	}

	return ttl, true
}

// UpdateTTL updates the TTL for an existing key
func (c *Cache) UpdateTTL(ctx context.Context, key string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, found := c.items[key]
	if !found {
		return fmt.Errorf("key not found: %s", key)
	}

	entry := elem.Value.(*CacheEntry)
	if ttl > 0 {
		entry.ExpiresAt = time.Now().Add(ttl)
	} else {
		entry.ExpiresAt = time.Time{} // No expiration
	}

	return nil
}
