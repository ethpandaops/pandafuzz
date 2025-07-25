package abstraction

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

// LoggingMiddleware adds logging to storage operations
type LoggingMiddleware struct {
	driver Driver
	logger logrus.FieldLogger
}

// NewLoggingMiddleware creates a new logging middleware
func NewLoggingMiddleware(driver Driver, logger logrus.FieldLogger) Driver {
	return &LoggingMiddleware{
		driver: driver,
		logger: logger.WithField("middleware", "logging"),
	}
}

// Put implements Driver.Put with logging
func (m *LoggingMiddleware) Put(ctx context.Context, key string, data []byte) error {
	start := time.Now()
	err := m.driver.Put(ctx, key, data)
	duration := time.Since(start)

	fields := logrus.Fields{
		"operation": "put",
		"key":       key,
		"size":      len(data),
		"duration":  duration,
	}

	if err != nil {
		m.logger.WithError(err).WithFields(fields).Error("Storage put failed")
	} else {
		m.logger.WithFields(fields).Debug("Storage put succeeded")
	}

	return err
}

// Get implements Driver.Get with logging
func (m *LoggingMiddleware) Get(ctx context.Context, key string) ([]byte, error) {
	start := time.Now()
	data, err := m.driver.Get(ctx, key)
	duration := time.Since(start)

	fields := logrus.Fields{
		"operation": "get",
		"key":       key,
		"duration":  duration,
	}

	if err != nil {
		if err == ErrNotFound {
			m.logger.WithFields(fields).Debug("Storage get - key not found")
		} else {
			m.logger.WithError(err).WithFields(fields).Error("Storage get failed")
		}
	} else {
		fields["size"] = len(data)
		m.logger.WithFields(fields).Debug("Storage get succeeded")
	}

	return data, err
}

// Delete implements Driver.Delete with logging
func (m *LoggingMiddleware) Delete(ctx context.Context, key string) error {
	start := time.Now()
	err := m.driver.Delete(ctx, key)
	duration := time.Since(start)

	fields := logrus.Fields{
		"operation": "delete",
		"key":       key,
		"duration":  duration,
	}

	if err != nil {
		m.logger.WithError(err).WithFields(fields).Error("Storage delete failed")
	} else {
		m.logger.WithFields(fields).Debug("Storage delete succeeded")
	}

	return err
}

// List implements Driver.List with logging
func (m *LoggingMiddleware) List(ctx context.Context, prefix string) ([]string, error) {
	start := time.Now()
	keys, err := m.driver.List(ctx, prefix)
	duration := time.Since(start)

	fields := logrus.Fields{
		"operation": "list",
		"prefix":    prefix,
		"duration":  duration,
	}

	if err != nil {
		m.logger.WithError(err).WithFields(fields).Error("Storage list failed")
	} else {
		fields["count"] = len(keys)
		m.logger.WithFields(fields).Debug("Storage list succeeded")
	}

	return keys, err
}

// Exists implements Driver.Exists with logging
func (m *LoggingMiddleware) Exists(ctx context.Context, key string) (bool, error) {
	start := time.Now()
	exists, err := m.driver.Exists(ctx, key)
	duration := time.Since(start)

	fields := logrus.Fields{
		"operation": "exists",
		"key":       key,
		"duration":  duration,
		"exists":    exists,
	}

	if err != nil {
		m.logger.WithError(err).WithFields(fields).Error("Storage exists failed")
	} else {
		m.logger.WithFields(fields).Debug("Storage exists succeeded")
	}

	return exists, err
}

// GetURL implements Driver.GetURL with logging
func (m *LoggingMiddleware) GetURL(ctx context.Context, key string) (string, error) {
	start := time.Now()
	url, err := m.driver.GetURL(ctx, key)
	duration := time.Since(start)

	fields := logrus.Fields{
		"operation": "get_url",
		"key":       key,
		"duration":  duration,
	}

	if err != nil {
		m.logger.WithError(err).WithFields(fields).Error("Storage get URL failed")
	} else {
		m.logger.WithFields(fields).Debug("Storage get URL succeeded")
	}

	return url, err
}

// MetricsMiddleware collects metrics for storage operations
type MetricsMiddleware struct {
	driver Driver
	logger logrus.FieldLogger

	// Operation counters
	putCount    uint64
	getCount    uint64
	deleteCount uint64
	listCount   uint64
	existsCount uint64
	getURLCount uint64

	// Error counters
	putErrors    uint64
	getErrors    uint64
	deleteErrors uint64
	listErrors   uint64
	existsErrors uint64
	getURLErrors uint64

	// Byte counters
	bytesWritten uint64
	bytesRead    uint64
}

// NewMetricsMiddleware creates a new metrics middleware
func NewMetricsMiddleware(driver Driver, logger logrus.FieldLogger) Driver {
	return &MetricsMiddleware{
		driver: driver,
		logger: logger.WithField("middleware", "metrics"),
	}
}

// Put implements Driver.Put with metrics
func (m *MetricsMiddleware) Put(ctx context.Context, key string, data []byte) error {
	atomic.AddUint64(&m.putCount, 1)
	atomic.AddUint64(&m.bytesWritten, uint64(len(data)))

	err := m.driver.Put(ctx, key, data)
	if err != nil {
		atomic.AddUint64(&m.putErrors, 1)
	}

	return err
}

// Get implements Driver.Get with metrics
func (m *MetricsMiddleware) Get(ctx context.Context, key string) ([]byte, error) {
	atomic.AddUint64(&m.getCount, 1)

	data, err := m.driver.Get(ctx, key)
	if err != nil {
		atomic.AddUint64(&m.getErrors, 1)
	} else {
		atomic.AddUint64(&m.bytesRead, uint64(len(data)))
	}

	return data, err
}

// Delete implements Driver.Delete with metrics
func (m *MetricsMiddleware) Delete(ctx context.Context, key string) error {
	atomic.AddUint64(&m.deleteCount, 1)

	err := m.driver.Delete(ctx, key)
	if err != nil {
		atomic.AddUint64(&m.deleteErrors, 1)
	}

	return err
}

// List implements Driver.List with metrics
func (m *MetricsMiddleware) List(ctx context.Context, prefix string) ([]string, error) {
	atomic.AddUint64(&m.listCount, 1)

	keys, err := m.driver.List(ctx, prefix)
	if err != nil {
		atomic.AddUint64(&m.listErrors, 1)
	}

	return keys, err
}

// Exists implements Driver.Exists with metrics
func (m *MetricsMiddleware) Exists(ctx context.Context, key string) (bool, error) {
	atomic.AddUint64(&m.existsCount, 1)

	exists, err := m.driver.Exists(ctx, key)
	if err != nil {
		atomic.AddUint64(&m.existsErrors, 1)
	}

	return exists, err
}

// GetURL implements Driver.GetURL with metrics
func (m *MetricsMiddleware) GetURL(ctx context.Context, key string) (string, error) {
	atomic.AddUint64(&m.getURLCount, 1)

	url, err := m.driver.GetURL(ctx, key)
	if err != nil {
		atomic.AddUint64(&m.getURLErrors, 1)
	}

	return url, err
}

// GetMetrics returns current metrics
func (m *MetricsMiddleware) GetMetrics() map[string]uint64 {
	return map[string]uint64{
		"put_count":      atomic.LoadUint64(&m.putCount),
		"get_count":      atomic.LoadUint64(&m.getCount),
		"delete_count":   atomic.LoadUint64(&m.deleteCount),
		"list_count":     atomic.LoadUint64(&m.listCount),
		"exists_count":   atomic.LoadUint64(&m.existsCount),
		"get_url_count":  atomic.LoadUint64(&m.getURLCount),
		"put_errors":     atomic.LoadUint64(&m.putErrors),
		"get_errors":     atomic.LoadUint64(&m.getErrors),
		"delete_errors":  atomic.LoadUint64(&m.deleteErrors),
		"list_errors":    atomic.LoadUint64(&m.listErrors),
		"exists_errors":  atomic.LoadUint64(&m.existsErrors),
		"get_url_errors": atomic.LoadUint64(&m.getURLErrors),
		"bytes_written":  atomic.LoadUint64(&m.bytesWritten),
		"bytes_read":     atomic.LoadUint64(&m.bytesRead),
	}
}

// RetryMiddleware adds retry logic to storage operations
type RetryMiddleware struct {
	driver Driver
	config RetryConfig
	logger logrus.FieldLogger
}

// NewRetryMiddleware creates a new retry middleware
func NewRetryMiddleware(driver Driver, config RetryConfig, logger logrus.FieldLogger) Driver {
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 100 * time.Millisecond
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 5 * time.Second
	}
	if config.Multiplier <= 0 {
		config.Multiplier = 2.0
	}

	return &RetryMiddleware{
		driver: driver,
		config: config,
		logger: logger.WithField("middleware", "retry"),
	}
}

// retry executes a function with exponential backoff
func (m *RetryMiddleware) retry(ctx context.Context, operation string, fn func() error) error {
	var lastErr error
	delay := m.config.InitialDelay

	for attempt := 1; attempt <= m.config.MaxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		// Don't retry on context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Don't retry on ErrNotFound
		if err == ErrNotFound {
			return err
		}

		lastErr = err

		if attempt < m.config.MaxAttempts {
			m.logger.WithError(err).WithFields(logrus.Fields{
				"operation": operation,
				"attempt":   attempt,
				"delay":     delay,
			}).Warn("Operation failed, retrying")

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}

			// Calculate next delay with exponential backoff
			delay = time.Duration(float64(delay) * m.config.Multiplier)
			if delay > m.config.MaxDelay {
				delay = m.config.MaxDelay
			}
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", m.config.MaxAttempts, lastErr)
}

// Put implements Driver.Put with retry
func (m *RetryMiddleware) Put(ctx context.Context, key string, data []byte) error {
	return m.retry(ctx, "put", func() error {
		return m.driver.Put(ctx, key, data)
	})
}

// Get implements Driver.Get with retry
func (m *RetryMiddleware) Get(ctx context.Context, key string) ([]byte, error) {
	var data []byte
	err := m.retry(ctx, "get", func() error {
		var err error
		data, err = m.driver.Get(ctx, key)
		return err
	})
	return data, err
}

// Delete implements Driver.Delete with retry
func (m *RetryMiddleware) Delete(ctx context.Context, key string) error {
	return m.retry(ctx, "delete", func() error {
		return m.driver.Delete(ctx, key)
	})
}

// List implements Driver.List with retry
func (m *RetryMiddleware) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	err := m.retry(ctx, "list", func() error {
		var err error
		keys, err = m.driver.List(ctx, prefix)
		return err
	})
	return keys, err
}

// Exists implements Driver.Exists with retry
func (m *RetryMiddleware) Exists(ctx context.Context, key string) (bool, error) {
	var exists bool
	err := m.retry(ctx, "exists", func() error {
		var err error
		exists, err = m.driver.Exists(ctx, key)
		return err
	})
	return exists, err
}

// GetURL implements Driver.GetURL with retry
func (m *RetryMiddleware) GetURL(ctx context.Context, key string) (string, error) {
	var url string
	err := m.retry(ctx, "get_url", func() error {
		var err error
		url, err = m.driver.GetURL(ctx, key)
		return err
	})
	return url, err
}

// CacheEntry represents a cached storage entry
type CacheEntry struct {
	Data      []byte
	ExpiresAt time.Time
}

// CacheMiddleware adds caching to storage operations
type CacheMiddleware struct {
	driver Driver
	config CacheConfig
	logger logrus.FieldLogger

	mu    sync.RWMutex
	cache map[string]*CacheEntry
	size  int64

	// Use singleflight to prevent cache stampede
	group singleflight.Group
}

// NewCacheMiddleware creates a new cache middleware
func NewCacheMiddleware(driver Driver, config CacheConfig, logger logrus.FieldLogger) Driver {
	if config.MaxSize <= 0 {
		config.MaxSize = 100 * 1024 * 1024 // 100MB
	}
	if config.TTL <= 0 {
		config.TTL = 5 * time.Minute
	}
	if config.MaxEntries <= 0 {
		config.MaxEntries = 1000
	}

	m := &CacheMiddleware{
		driver: driver,
		config: config,
		logger: logger.WithField("middleware", "cache"),
		cache:  make(map[string]*CacheEntry),
	}

	// Start cleanup goroutine
	go m.cleanupLoop()

	return m
}

// cleanupLoop periodically removes expired entries
func (m *CacheMiddleware) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.cleanup()
	}
}

// cleanup removes expired entries
func (m *CacheMiddleware) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	var keysToDelete []string

	for key, entry := range m.cache {
		if now.After(entry.ExpiresAt) {
			keysToDelete = append(keysToDelete, key)
			m.size -= int64(len(entry.Data))
		}
	}

	for _, key := range keysToDelete {
		delete(m.cache, key)
	}

	if len(keysToDelete) > 0 {
		m.logger.WithField("expired_entries", len(keysToDelete)).Debug("Cleaned up expired cache entries")
	}
}

// Put implements Driver.Put with caching
func (m *CacheMiddleware) Put(ctx context.Context, key string, data []byte) error {
	err := m.driver.Put(ctx, key, data)
	if err != nil {
		// Invalidate cache on error
		m.invalidate(key)
		return err
	}

	// Update cache
	m.set(key, data)
	return nil
}

// Get implements Driver.Get with caching
func (m *CacheMiddleware) Get(ctx context.Context, key string) ([]byte, error) {
	// Check cache first
	if data := m.get(key); data != nil {
		m.logger.WithField("key", key).Debug("Cache hit")
		return data, nil
	}

	// Use singleflight to prevent multiple concurrent fetches
	result, err, _ := m.group.Do(key, func() (interface{}, error) {
		data, err := m.driver.Get(ctx, key)
		if err != nil {
			return nil, err
		}

		// Update cache
		m.set(key, data)
		return data, nil
	})

	if err != nil {
		return nil, err
	}

	return result.([]byte), nil
}

// Delete implements Driver.Delete with caching
func (m *CacheMiddleware) Delete(ctx context.Context, key string) error {
	err := m.driver.Delete(ctx, key)
	// Always invalidate cache, even on error
	m.invalidate(key)
	return err
}

// List implements Driver.List without caching
func (m *CacheMiddleware) List(ctx context.Context, prefix string) ([]string, error) {
	// List operations are not cached to ensure consistency
	return m.driver.List(ctx, prefix)
}

// Exists implements Driver.Exists with caching
func (m *CacheMiddleware) Exists(ctx context.Context, key string) (bool, error) {
	// Check cache first
	if m.get(key) != nil {
		return true, nil
	}

	return m.driver.Exists(ctx, key)
}

// GetURL implements Driver.GetURL without caching
func (m *CacheMiddleware) GetURL(ctx context.Context, key string) (string, error) {
	// URL operations are not cached as they may be time-sensitive
	return m.driver.GetURL(ctx, key)
}

// get retrieves an entry from cache
func (m *CacheMiddleware) get(key string) []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.cache[key]
	if !ok || time.Now().After(entry.ExpiresAt) {
		return nil
	}

	return entry.Data
}

// set adds or updates an entry in cache
func (m *CacheMiddleware) set(key string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if we need to evict entries
	if len(m.cache) >= m.config.MaxEntries || m.size+int64(len(data)) > m.config.MaxSize {
		// Simple eviction: remove oldest entries
		// In production, consider LRU or other strategies
		for k, v := range m.cache {
			delete(m.cache, k)
			m.size -= int64(len(v.Data))
			if len(m.cache) < m.config.MaxEntries && m.size+int64(len(data)) <= m.config.MaxSize {
				break
			}
		}
	}

	// Add new entry
	m.cache[key] = &CacheEntry{
		Data:      data,
		ExpiresAt: time.Now().Add(m.config.TTL),
	}
	m.size += int64(len(data))
}

// invalidate removes an entry from cache
func (m *CacheMiddleware) invalidate(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, ok := m.cache[key]; ok {
		delete(m.cache, key)
		m.size -= int64(len(entry.Data))
	}
}
