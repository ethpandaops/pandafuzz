package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Example demonstrates how to use the cache in repository implementations

// CachedRepository wraps a repository with caching capabilities
type CachedRepository struct {
	cache      FullCache
	repository interface{} // Your actual repository implementation
	keyPrefix  string
	ttl        time.Duration
}

// NewCachedRepository creates a new cached repository wrapper
func NewCachedRepository(cache FullCache, repo interface{}, keyPrefix string, ttl time.Duration) *CachedRepository {
	return &CachedRepository{
		cache:      cache,
		repository: repo,
		keyPrefix:  keyPrefix,
		ttl:        ttl,
	}
}

// makeKey creates a cache key with prefix
func (r *CachedRepository) makeKey(parts ...string) string {
	key := r.keyPrefix
	for _, part := range parts {
		key += ":" + part
	}
	return key
}

// Example: Caching a FindByID operation
func (r *CachedRepository) CachedFindByID(ctx context.Context, id string, finder func(context.Context, string) (interface{}, error)) (interface{}, error) {
	// Try cache first
	cacheKey := r.makeKey("id", id)
	if cached, found := r.cache.Get(ctx, cacheKey); found {
		return cached, nil
	}

	// Cache miss - fetch from repository
	result, err := finder(ctx, id)
	if err != nil {
		return nil, err
	}

	// Store in cache
	if err := r.cache.SetWithTTL(ctx, cacheKey, result, r.ttl); err != nil {
		// Log error but don't fail the operation
		// In production, you'd use proper logging here
		fmt.Printf("Failed to cache result: %v\n", err)
	}

	return result, nil
}

// Example: Invalidating cache on update
func (r *CachedRepository) InvalidateOnUpdate(ctx context.Context, id string, updater func(context.Context, string) error) error {
	// Perform the update
	if err := updater(ctx, id); err != nil {
		return err
	}

	// Invalidate related cache entries
	r.cache.Delete(ctx, r.makeKey("id", id))

	// You might also want to invalidate list caches, etc.
	r.cache.Delete(ctx, r.makeKey("list"))

	return nil
}

// Example: Caching list operations with pagination
func (r *CachedRepository) CachedList(ctx context.Context, offset, limit int, lister func(context.Context, int, int) ([]interface{}, int, error)) ([]interface{}, int, error) {
	// Create cache key for this specific page
	cacheKey := r.makeKey("list", fmt.Sprintf("offset:%d:limit:%d", offset, limit))

	// Try cache first
	if cached, found := r.cache.Get(ctx, cacheKey); found {
		// Unmarshal from cache
		type cachedList struct {
			Items []interface{}
			Total int
		}
		if data, ok := cached.(*cachedList); ok {
			return data.Items, data.Total, nil
		}
	}

	// Cache miss - fetch from repository
	items, total, err := lister(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}

	// Store in cache
	cacheData := struct {
		Items []interface{}
		Total int
	}{
		Items: items,
		Total: total,
	}

	if err := r.cache.SetWithTTL(ctx, cacheKey, &cacheData, r.ttl); err != nil {
		fmt.Printf("Failed to cache list result: %v\n", err)
	}

	return items, total, nil
}

// Example: Cache warming for frequently accessed data
func (r *CachedRepository) WarmCache(ctx context.Context, preloader func(context.Context) (map[string]interface{}, error)) error {
	// Fetch data to preload
	data, err := preloader(ctx)
	if err != nil {
		return err
	}

	// Transform keys to include prefix
	prefixedData := make(map[string]interface{})
	for k, v := range data {
		prefixedData[r.makeKey(k)] = v
	}

	// Preload cache
	return r.cache.PreloadWithTTL(ctx, prefixedData, r.ttl)
}

// Example: Using cache for computed/aggregated data
func (r *CachedRepository) CachedAggregation(ctx context.Context, aggregationType string, computer func(context.Context) (interface{}, error)) (interface{}, error) {
	cacheKey := r.makeKey("aggregation", aggregationType)

	// Try cache first
	if cached, found := r.cache.Get(ctx, cacheKey); found {
		return cached, nil
	}

	// Compute the aggregation
	result, err := computer(ctx)
	if err != nil {
		return nil, err
	}

	// Cache with shorter TTL for aggregations
	aggregationTTL := r.ttl / 2 // Use half the normal TTL for aggregations
	if err := r.cache.SetWithTTL(ctx, cacheKey, result, aggregationTTL); err != nil {
		fmt.Printf("Failed to cache aggregation: %v\n", err)
	}

	return result, nil
}

// Example: Batch operations with cache
func (r *CachedRepository) CachedBatchFind(ctx context.Context, ids []string, batchFinder func(context.Context, []string) (map[string]interface{}, error)) (map[string]interface{}, error) {
	// Check cache for existing items
	cacheKeys := make([]string, len(ids))
	for i, id := range ids {
		cacheKeys[i] = r.makeKey("id", id)
	}

	cached := r.cache.GetMulti(ctx, cacheKeys)
	results := make(map[string]interface{})
	missingIDs := make([]string, 0)

	// Collect cached results and identify missing IDs
	for i, id := range ids {
		if value, found := cached[cacheKeys[i]]; found {
			results[id] = value
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	// Fetch missing items from repository
	if len(missingIDs) > 0 {
		fetched, err := batchFinder(ctx, missingIDs)
		if err != nil {
			return nil, err
		}

		// Cache fetched items
		toCache := make(map[string]interface{})
		for id, value := range fetched {
			results[id] = value
			toCache[r.makeKey("id", id)] = value
		}

		if err := r.cache.SetMulti(ctx, toCache); err != nil {
			fmt.Printf("Failed to cache batch results: %v\n", err)
		}
	}

	return results, nil
}

// Example: Cache stats monitoring
func (r *CachedRepository) GetCacheStats() CacheStats {
	return r.cache.Stats()
}

// Example: Serialization helper for complex types
func (r *CachedRepository) serializeForCache(v interface{}) (interface{}, error) {
	// For complex types, you might want to serialize to JSON
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (r *CachedRepository) deserializeFromCache(cached interface{}, target interface{}) error {
	// Deserialize from JSON
	if data, ok := cached.(string); ok {
		return json.Unmarshal([]byte(data), target)
	}
	return fmt.Errorf("invalid cached data type")
}
