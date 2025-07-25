package cache

import (
	"context"
	"time"
)

// Cache defines the interface for cache implementations
type Cache interface {
	// Get retrieves a value from the cache
	Get(ctx context.Context, key string) (interface{}, bool)

	// Set stores a value in the cache with default TTL
	Set(ctx context.Context, key string, value interface{}) error

	// SetWithTTL stores a value in the cache with custom TTL
	SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error

	// Delete removes a value from the cache
	Delete(ctx context.Context, key string) bool

	// Clear removes all items from the cache
	Clear(ctx context.Context)

	// Exists checks if a key exists in the cache
	Exists(ctx context.Context, key string) bool

	// Size returns the current number of items in the cache
	Size() int

	// Close stops the cache and cleanup routines
	Close() error
}

// BatchCache extends Cache with batch operations
type BatchCache interface {
	Cache

	// GetMulti retrieves multiple values from the cache
	GetMulti(ctx context.Context, keys []string) map[string]interface{}

	// SetMulti stores multiple values in the cache
	SetMulti(ctx context.Context, items map[string]interface{}) error

	// DeleteMulti removes multiple values from the cache
	DeleteMulti(ctx context.Context, keys []string) int
}

// StatsCache extends Cache with statistics
type StatsCache interface {
	Cache

	// Stats returns cache statistics
	Stats() CacheStats

	// HitRate returns the cache hit rate as a percentage
	HitRate() float64
}

// TTLCache extends Cache with TTL operations
type TTLCache interface {
	Cache

	// TTL returns the remaining time-to-live for a key
	TTL(ctx context.Context, key string) (time.Duration, bool)

	// UpdateTTL updates the TTL for an existing key
	UpdateTTL(ctx context.Context, key string, ttl time.Duration) error
}

// PreloadableCache extends Cache with preloading capabilities
type PreloadableCache interface {
	Cache

	// Preload warms the cache with initial data
	Preload(ctx context.Context, items map[string]interface{}) error

	// PreloadWithTTL warms the cache with initial data and custom TTL
	PreloadWithTTL(ctx context.Context, items map[string]interface{}, ttl time.Duration) error
}

// CacheStats represents cache performance statistics
type CacheStats struct {
	Hits       int64
	Misses     int64
	Evictions  int64
	TotalItems int64
	SizeBytes  int64
}

// FullCache combines all cache interfaces
type FullCache interface {
	BatchCache
	StatsCache
	TTLCache
	PreloadableCache

	// Keys returns all keys in the cache
	Keys() []string
}
