package memory

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCache_BasicOperations(t *testing.T) {
	ctx := context.Background()
	cache := New(DefaultConfig())
	defer cache.Close()

	// Test Set and Get
	err := cache.Set(ctx, "key1", "value1")
	assert.NoError(t, err)

	value, found := cache.Get(ctx, "key1")
	assert.True(t, found)
	assert.Equal(t, "value1", value)

	// Test Get non-existent key
	_, found = cache.Get(ctx, "nonexistent")
	assert.False(t, found)

	// Test Delete
	deleted := cache.Delete(ctx, "key1")
	assert.True(t, deleted)

	_, found = cache.Get(ctx, "key1")
	assert.False(t, found)

	// Test Delete non-existent key
	deleted = cache.Delete(ctx, "nonexistent")
	assert.False(t, deleted)
}

func TestCache_TTL(t *testing.T) {
	ctx := context.Background()
	config := &CacheConfig{
		MaxSize:         100,
		DefaultTTL:      100 * time.Millisecond,
		CleanupInterval: 50 * time.Millisecond,
		EvictionPolicy:  LRU,
	}
	cache := New(config)
	defer cache.Close()

	// Set with default TTL
	err := cache.Set(ctx, "key1", "value1")
	assert.NoError(t, err)

	// Value should exist initially
	value, found := cache.Get(ctx, "key1")
	assert.True(t, found)
	assert.Equal(t, "value1", value)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Value should be expired
	_, found = cache.Get(ctx, "key1")
	assert.False(t, found)

	// Test custom TTL
	err = cache.SetWithTTL(ctx, "key2", "value2", 200*time.Millisecond)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	_, found = cache.Get(ctx, "key2")
	assert.True(t, found) // Should still exist

	time.Sleep(150 * time.Millisecond)
	_, found = cache.Get(ctx, "key2")
	assert.False(t, found) // Should be expired
}

func TestCache_Eviction(t *testing.T) {
	ctx := context.Background()
	evictedKeys := make([]string, 0)
	config := &CacheConfig{
		MaxSize:         3,
		DefaultTTL:      0, // No expiration
		CleanupInterval: 0,
		EvictionPolicy:  LRU,
		OnEviction: func(key string, value interface{}) {
			evictedKeys = append(evictedKeys, key)
		},
	}
	cache := New(config)
	defer cache.Close()

	// Fill cache to capacity
	cache.Set(ctx, "key1", "value1")
	cache.Set(ctx, "key2", "value2")
	cache.Set(ctx, "key3", "value3")

	assert.Equal(t, 3, cache.Size())

	// Access key1 to make it recently used
	cache.Get(ctx, "key1")

	// Add new item, should evict key2 (least recently used)
	cache.Set(ctx, "key4", "value4")

	assert.Equal(t, 3, cache.Size())
	assert.Contains(t, evictedKeys, "key2")

	// Verify key2 was evicted
	_, found := cache.Get(ctx, "key2")
	assert.False(t, found)

	// Other keys should still exist
	_, found = cache.Get(ctx, "key1")
	assert.True(t, found)
	_, found = cache.Get(ctx, "key3")
	assert.True(t, found)
	_, found = cache.Get(ctx, "key4")
	assert.True(t, found)
}

func TestCache_BatchOperations(t *testing.T) {
	ctx := context.Background()
	cache := New(DefaultConfig())
	defer cache.Close()

	// Test SetMulti
	items := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	err := cache.SetMulti(ctx, items)
	assert.NoError(t, err)

	// Test GetMulti
	keys := []string{"key1", "key2", "key3", "nonexistent"}
	results := cache.GetMulti(ctx, keys)
	assert.Len(t, results, 3)
	assert.Equal(t, "value1", results["key1"])
	assert.Equal(t, "value2", results["key2"])
	assert.Equal(t, "value3", results["key3"])
	assert.NotContains(t, results, "nonexistent")

	// Test DeleteMulti
	deleted := cache.DeleteMulti(ctx, []string{"key1", "key2", "nonexistent"})
	assert.Equal(t, 2, deleted)

	// Verify deletions
	_, found := cache.Get(ctx, "key1")
	assert.False(t, found)
	_, found = cache.Get(ctx, "key2")
	assert.False(t, found)
	_, found = cache.Get(ctx, "key3")
	assert.True(t, found)
}

func TestCache_Stats(t *testing.T) {
	ctx := context.Background()
	cache := New(DefaultConfig())
	defer cache.Close()

	// Initial stats
	stats := cache.Stats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)

	// Generate some hits and misses
	cache.Set(ctx, "key1", "value1")
	cache.Get(ctx, "key1") // Hit
	cache.Get(ctx, "key1") // Hit
	cache.Get(ctx, "key2") // Miss

	stats = cache.Stats()
	assert.Equal(t, int64(2), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)

	// Test hit rate
	hitRate := cache.HitRate()
	assert.InDelta(t, 66.67, hitRate, 0.01)
}

func TestCache_Concurrency(t *testing.T) {
	ctx := context.Background()
	cache := New(DefaultConfig())
	defer cache.Close()

	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				value := fmt.Sprintf("value-%d-%d", id, j)
				err := cache.Set(ctx, key, value)
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all writes
	for i := 0; i < numGoroutines; i++ {
		for j := 0; j < numOperations; j++ {
			key := fmt.Sprintf("key-%d-%d", i, j)
			expectedValue := fmt.Sprintf("value-%d-%d", i, j)
			value, found := cache.Get(ctx, key)
			assert.True(t, found)
			assert.Equal(t, expectedValue, value)
		}
	}
}

func TestCache_Preload(t *testing.T) {
	ctx := context.Background()
	cache := New(DefaultConfig())
	defer cache.Close()

	// Preload data
	preloadData := map[string]interface{}{
		"config:db_host":     "localhost",
		"config:db_port":     5432,
		"config:cache_size":  1000,
		"config:enable_logs": true,
	}

	err := cache.Preload(ctx, preloadData)
	assert.NoError(t, err)

	// Verify all data is loaded
	for key, expectedValue := range preloadData {
		value, found := cache.Get(ctx, key)
		assert.True(t, found)
		assert.Equal(t, expectedValue, value)
	}

	// Test preload with TTL
	cache.Clear(ctx)
	err = cache.PreloadWithTTL(ctx, preloadData, 100*time.Millisecond)
	assert.NoError(t, err)

	// Data should exist initially
	assert.Equal(t, len(preloadData), cache.Size())

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// All data should be expired
	for key := range preloadData {
		_, found := cache.Get(ctx, key)
		assert.False(t, found)
	}
}

func TestCache_Clear(t *testing.T) {
	ctx := context.Background()
	cache := New(DefaultConfig())
	defer cache.Close()

	// Add some data
	cache.Set(ctx, "key1", "value1")
	cache.Set(ctx, "key2", "value2")
	cache.Set(ctx, "key3", "value3")

	assert.Equal(t, 3, cache.Size())

	// Clear cache
	cache.Clear(ctx)

	assert.Equal(t, 0, cache.Size())
	assert.Empty(t, cache.Keys())

	// Verify all data is gone
	_, found := cache.Get(ctx, "key1")
	assert.False(t, found)
}

func TestCache_Keys(t *testing.T) {
	ctx := context.Background()
	cache := New(DefaultConfig())
	defer cache.Close()

	// Add some data
	expectedKeys := []string{"key1", "key2", "key3"}
	for _, key := range expectedKeys {
		cache.Set(ctx, key, "value")
	}

	// Get all keys
	keys := cache.Keys()
	assert.Len(t, keys, 3)
	for _, key := range expectedKeys {
		assert.Contains(t, keys, key)
	}
}

func TestCache_Exists(t *testing.T) {
	ctx := context.Background()
	cache := New(DefaultConfig())
	defer cache.Close()

	// Test non-existent key
	exists := cache.Exists(ctx, "key1")
	assert.False(t, exists)

	// Add key
	cache.Set(ctx, "key1", "value1")
	exists = cache.Exists(ctx, "key1")
	assert.True(t, exists)

	// Test with expired key
	cache.SetWithTTL(ctx, "key2", "value2", 50*time.Millisecond)
	exists = cache.Exists(ctx, "key2")
	assert.True(t, exists)

	time.Sleep(100 * time.Millisecond)
	exists = cache.Exists(ctx, "key2")
	assert.False(t, exists)
}

func TestCache_TTLOperations(t *testing.T) {
	ctx := context.Background()
	cache := New(DefaultConfig())
	defer cache.Close()

	// Test TTL for non-existent key
	_, found := cache.TTL(ctx, "nonexistent")
	assert.False(t, found)

	// Test key with no expiration
	cache.SetWithTTL(ctx, "key1", "value1", 0)
	ttl, found := cache.TTL(ctx, "key1")
	assert.True(t, found)
	assert.Equal(t, time.Duration(0), ttl)

	// Test key with TTL
	cache.SetWithTTL(ctx, "key2", "value2", 1*time.Second)
	ttl, found = cache.TTL(ctx, "key2")
	assert.True(t, found)
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, 1*time.Second)

	// Test UpdateTTL
	err := cache.UpdateTTL(ctx, "key1", 500*time.Millisecond)
	assert.NoError(t, err)

	ttl, found = cache.TTL(ctx, "key1")
	assert.True(t, found)
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, 500*time.Millisecond)

	// Test UpdateTTL on non-existent key
	err = cache.UpdateTTL(ctx, "nonexistent", 1*time.Second)
	assert.Error(t, err)
}

func TestCache_LFUEviction(t *testing.T) {
	ctx := context.Background()
	config := &CacheConfig{
		MaxSize:         3,
		DefaultTTL:      0,
		CleanupInterval: 0,
		EvictionPolicy:  LFU,
	}
	cache := New(config)
	defer cache.Close()

	// Add items
	cache.Set(ctx, "key1", "value1")
	cache.Set(ctx, "key2", "value2")
	cache.Set(ctx, "key3", "value3")

	// Access items different number of times
	cache.Get(ctx, "key1") // 1 access
	cache.Get(ctx, "key2") // 2 accesses
	cache.Get(ctx, "key2")
	cache.Get(ctx, "key3") // 3 accesses
	cache.Get(ctx, "key3")
	cache.Get(ctx, "key3")

	// Add new item, should evict key1 (least frequently used)
	cache.Set(ctx, "key4", "value4")

	_, found := cache.Get(ctx, "key1")
	assert.False(t, found) // key1 should be evicted

	// Other keys should exist
	_, found = cache.Get(ctx, "key2")
	assert.True(t, found)
	_, found = cache.Get(ctx, "key3")
	assert.True(t, found)
	_, found = cache.Get(ctx, "key4")
	assert.True(t, found)
}

func BenchmarkCache_Get(b *testing.B) {
	ctx := context.Background()
	cache := New(DefaultConfig())
	defer cache.Close()

	// Preload some data
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		cache.Set(ctx, key, value)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key%d", i%1000)
			cache.Get(ctx, key)
			i++
		}
	})
}

func BenchmarkCache_Set(b *testing.B) {
	ctx := context.Background()
	cache := New(&CacheConfig{
		MaxSize:         10000,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
		EvictionPolicy:  LRU,
	})
	defer cache.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key%d", i)
			value := fmt.Sprintf("value%d", i)
			cache.Set(ctx, key, value)
			i++
		}
	})
}
