# Cache Infrastructure

This package provides a high-performance, thread-safe in-memory caching layer for repository implementations.

## Features

- **Generic in-memory cache** - Store any type with type-safe operations
- **TTL support** - Automatic expiration of cached entries
- **Multiple eviction policies** - LRU (Least Recently Used), LFU (Least Frequently Used), FIFO
- **Thread-safe** - Safe for concurrent access with read/write locks
- **Cache statistics** - Track hits, misses, evictions for monitoring
- **Batch operations** - Efficiently get/set/delete multiple items
- **Cache warming** - Preload frequently accessed data
- **Context support** - All operations accept context for cancellation
- **Configurable cleanup** - Automatic removal of expired entries

## Usage

### Basic Usage

```go
import (
    "context"
    "time"
    "github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/cache/memory"
)

// Create cache with default configuration
cache := memory.New(memory.DefaultConfig())
defer cache.Close()

// Store a value
ctx := context.Background()
cache.Set(ctx, "user:123", user)

// Retrieve a value
if value, found := cache.Get(ctx, "user:123"); found {
    user := value.(*User)
    // Use the user
}

// Store with custom TTL
cache.SetWithTTL(ctx, "session:abc", session, 30*time.Minute)
```

### Configuration

```go
config := &memory.CacheConfig{
    MaxSize:         10000,              // Maximum items (0 = unlimited)
    DefaultTTL:      5 * time.Minute,    // Default expiration time
    CleanupInterval: 1 * time.Minute,    // How often to clean expired items
    EvictionPolicy:  memory.LRU,         // LRU, LFU, or FIFO
    OnEviction: func(key string, value interface{}) {
        log.Printf("Evicted: %s", key)
    },
}

cache := memory.New(config)
```

### Repository Integration

```go
type CachedUserRepository struct {
    cache      cache.FullCache
    repository UserRepository
}

func (r *CachedUserRepository) FindByID(ctx context.Context, id string) (*User, error) {
    // Check cache first
    cacheKey := fmt.Sprintf("user:%s", id)
    if cached, found := r.cache.Get(ctx, cacheKey); found {
        return cached.(*User), nil
    }

    // Cache miss - fetch from database
    user, err := r.repository.FindByID(ctx, id)
    if err != nil {
        return nil, err
    }

    // Store in cache for 5 minutes
    r.cache.SetWithTTL(ctx, cacheKey, user, 5*time.Minute)
    
    return user, nil
}

func (r *CachedUserRepository) Update(ctx context.Context, user *User) error {
    // Update in database
    if err := r.repository.Update(ctx, user); err != nil {
        return err
    }

    // Invalidate cache
    cacheKey := fmt.Sprintf("user:%s", user.ID)
    r.cache.Delete(ctx, cacheKey)
    
    return nil
}
```

### Batch Operations

```go
// Get multiple items
keys := []string{"user:1", "user:2", "user:3"}
results := cache.GetMulti(ctx, keys)

// Set multiple items
items := map[string]interface{}{
    "user:1": user1,
    "user:2": user2,
    "user:3": user3,
}
cache.SetMulti(ctx, items)

// Delete multiple items
deleted := cache.DeleteMulti(ctx, keys)
```

### Cache Warming

```go
// Preload frequently accessed data
preloadData := map[string]interface{}{
    "config:db_host":     "localhost",
    "config:db_port":     5432,
    "config:cache_size":  1000,
}

// Preload with default TTL
cache.Preload(ctx, preloadData)

// Preload with custom TTL
cache.PreloadWithTTL(ctx, preloadData, 1*time.Hour)
```

### Monitoring

```go
// Get cache statistics
stats := cache.Stats()
fmt.Printf("Hits: %d, Misses: %d, Hit Rate: %.2f%%\n", 
    stats.Hits, stats.Misses, cache.HitRate())

// Check cache size
size := cache.Size()

// List all keys (use sparingly in production)
keys := cache.Keys()
```

## Best Practices

1. **Use appropriate TTL values** - Balance between performance and data freshness
2. **Choose the right eviction policy**:
   - LRU: Good for general use cases
   - LFU: Better when some items are accessed much more frequently
   - FIFO: Simple and predictable, good for time-based data
3. **Monitor cache statistics** - Track hit rates to ensure cache effectiveness
4. **Invalidate on updates** - Always invalidate cache entries when data changes
5. **Use batch operations** - More efficient for multiple operations
6. **Set reasonable max size** - Prevent unbounded memory growth
7. **Use context** - Pass context for proper cancellation support

## Example: Complete Repository with Caching

See `example_repository_cache.go` for a comprehensive example of integrating caching with repository patterns, including:

- Single item caching
- List/pagination caching
- Cache invalidation strategies
- Batch operations
- Cache warming
- Aggregation caching
- Serialization for complex types

## Performance

The cache is designed for high-performance concurrent access:

- Read operations use RWMutex for minimal contention
- Atomic operations for statistics
- Efficient eviction algorithms
- Configurable cleanup intervals

Benchmark results (example):
- Get: ~50ns per operation (concurrent)
- Set: ~200ns per operation (concurrent)
- Batch operations: ~30% faster than individual operations