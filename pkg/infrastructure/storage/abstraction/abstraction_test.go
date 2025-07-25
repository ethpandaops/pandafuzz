package abstraction_test

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/abstraction"
	_ "github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/drivers/all"
)

// MockDriver is a simple mock implementation for testing
type MockDriver struct {
	data map[string][]byte
}

func NewMockDriver() *MockDriver {
	return &MockDriver{
		data: make(map[string][]byte),
	}
}

func (m *MockDriver) Put(ctx context.Context, key string, data []byte) error {
	m.data[key] = data
	return nil
}

func (m *MockDriver) Get(ctx context.Context, key string) ([]byte, error) {
	data, ok := m.data[key]
	if !ok {
		return nil, abstraction.ErrNotFound
	}
	return data, nil
}

func (m *MockDriver) Delete(ctx context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *MockDriver) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	for key := range m.data {
		if len(prefix) == 0 || len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (m *MockDriver) Exists(ctx context.Context, key string) (bool, error) {
	_, ok := m.data[key]
	return ok, nil
}

func (m *MockDriver) GetURL(ctx context.Context, key string) (string, error) {
	if _, ok := m.data[key]; !ok {
		return "", abstraction.ErrNotFound
	}
	return "mock://storage/" + key, nil
}

func TestLoggingMiddleware(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	base := NewMockDriver()
	driver := abstraction.NewLoggingMiddleware(base, logger)

	ctx := context.Background()
	key := "test-key"
	data := []byte("test-data")

	// Test Put
	err := driver.Put(ctx, key, data)
	require.NoError(t, err)

	// Test Get
	retrieved, err := driver.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, data, retrieved)

	// Test Delete
	err = driver.Delete(ctx, key)
	require.NoError(t, err)
}

func TestMetricsMiddleware(t *testing.T) {
	logger := logrus.New()
	base := NewMockDriver()
	driver := abstraction.NewMetricsMiddleware(base, logger)

	ctx := context.Background()
	key := "metrics-test"
	data := []byte("metrics-data")

	// Perform operations
	err := driver.Put(ctx, key, data)
	require.NoError(t, err)

	_, err = driver.Get(ctx, key)
	require.NoError(t, err)

	// Check metrics
	metrics := driver.(*abstraction.MetricsMiddleware).GetMetrics()
	assert.Equal(t, uint64(1), metrics["put_count"])
	assert.Equal(t, uint64(1), metrics["get_count"])
	assert.Equal(t, uint64(0), metrics["put_errors"])
	assert.Equal(t, uint64(0), metrics["get_errors"])
}

func TestRetryMiddleware(t *testing.T) {
	logger := logrus.New()
	base := NewMockDriver()

	config := abstraction.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	driver := abstraction.NewRetryMiddleware(base, config, logger)

	ctx := context.Background()
	key := "retry-test"
	data := []byte("retry-data")

	// Normal operation should work
	err := driver.Put(ctx, key, data)
	require.NoError(t, err)

	retrieved, err := driver.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, data, retrieved)
}

func TestCacheMiddleware(t *testing.T) {
	logger := logrus.New()
	base := NewMockDriver()

	config := abstraction.CacheConfig{
		MaxSize:    1024,
		TTL:        1 * time.Second,
		MaxEntries: 10,
	}

	driver := abstraction.NewCacheMiddleware(base, config, logger)

	ctx := context.Background()
	key := "cache-test"
	data := []byte("cache-data")

	// First put
	err := driver.Put(ctx, key, data)
	require.NoError(t, err)

	// First get (cache miss)
	retrieved, err := driver.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, data, retrieved)

	// Delete from base storage
	err = base.Delete(ctx, key)
	require.NoError(t, err)

	// Second get should still work (cache hit)
	retrieved, err = driver.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, data, retrieved)

	// Wait for cache to expire
	time.Sleep(2 * time.Second)

	// Now get should fail
	_, err = driver.Get(ctx, key)
	assert.ErrorIs(t, err, abstraction.ErrNotFound)
}

func TestCompositeDriver(t *testing.T) {
	logger := logrus.New()

	primary := NewMockDriver()
	secondary1 := NewMockDriver()
	secondary2 := NewMockDriver()

	config := abstraction.CompositeDriverConfig{
		Primary:     primary,
		Secondaries: []abstraction.Driver{secondary1, secondary2},
		WriteMode:   abstraction.WriteAll,
		ReadMode:    abstraction.ReadFallback,
		Logger:      logger,
	}

	driver, err := abstraction.NewCompositeDriver(config)
	require.NoError(t, err)

	ctx := context.Background()
	key := "composite-test"
	data := []byte("composite-data")

	// Put should write to all backends
	err = driver.Put(ctx, key, data)
	require.NoError(t, err)

	// Verify data in all backends
	for _, backend := range []abstraction.Driver{primary, secondary1, secondary2} {
		retrieved, err := backend.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, data, retrieved)
	}

	// Delete from primary
	err = primary.Delete(ctx, key)
	require.NoError(t, err)

	// Get should still work (fallback to secondary)
	retrieved, err := driver.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, data, retrieved)
}

func TestFactory(t *testing.T) {
	logger := logrus.New()
	factory := abstraction.NewFactory(logger)

	t.Run("Filesystem with middleware", func(t *testing.T) {
		driver, err := factory.NewDriver(abstraction.FactoryConfig{
			Type: abstraction.TypeFilesystem,
			Config: &abstraction.FilesystemConfig{
				BasePath: t.TempDir(),
				FileMode: 0o644,
				DirMode:  0o755,
			},
			Middleware: abstraction.MiddlewareConfig{
				EnableLogging: true,
				EnableMetrics: true,
				EnableRetry:   true,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, driver)

		// Test basic operations
		ctx := context.Background()
		key := "test/file.txt"
		data := []byte("test content")

		err = driver.Put(ctx, key, data)
		require.NoError(t, err)

		retrieved, err := driver.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, data, retrieved)
	})
}
