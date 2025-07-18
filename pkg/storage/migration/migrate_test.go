package migration

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/ethpandaops/pandafuzz/pkg/storage/backend"
)

// MockStorageBackend is a mock implementation of the StorageBackend interface
type MockStorageBackend struct {
	mock.Mock
}

func (m *MockStorageBackend) Store(ctx context.Context, key string, reader io.Reader, size int64) error {
	args := m.Called(ctx, key, reader, size)
	return args.Error(0)
}

func (m *MockStorageBackend) Retrieve(ctx context.Context, key string) (io.ReadCloser, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockStorageBackend) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *MockStorageBackend) Exists(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageBackend) List(ctx context.Context, prefix string) ([]backend.ObjectInfo, error) {
	args := m.Called(ctx, prefix)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]backend.ObjectInfo), args.Error(1)
}

func (m *MockStorageBackend) DeleteMany(ctx context.Context, keys []string) error {
	args := m.Called(ctx, keys)
	return args.Error(0)
}

func (m *MockStorageBackend) GetMetadata(ctx context.Context, key string) (*backend.ObjectMetadata, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*backend.ObjectMetadata), args.Error(1)
}

func (m *MockStorageBackend) SetMetadata(ctx context.Context, key string, metadata map[string]string) error {
	args := m.Called(ctx, key, metadata)
	return args.Error(0)
}

func (m *MockStorageBackend) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	args := m.Called(ctx, key, expiry)
	return args.String(0), args.Error(1)
}

func (m *MockStorageBackend) PutPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	args := m.Called(ctx, key, expiry)
	return args.String(0), args.Error(1)
}

func (m *MockStorageBackend) HealthCheck(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func TestMigratorDryRun(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	source := new(MockStorageBackend)
	dest := new(MockStorageBackend)

	// Mock source listing - use mock.Anything for context since it will be wrapped
	objects := []backend.ObjectInfo{
		{Key: "file1.txt", Size: 100},
		{Key: "file2.txt", Size: 200},
		{Key: "file3.txt", Size: 300},
	}
	source.On("List", mock.Anything, "").Return(objects, nil)

	opts := MigrationOptions{
		DryRun:   true,
		Parallel: 2,
	}

	migrator := NewMigrator(source, dest, opts, logger)
	result, err := migrator.Migrate(ctx, "")

	assert.NoError(t, err)
	assert.Equal(t, 3, result.TotalFiles)
	assert.Equal(t, int64(600), result.TotalSize)
	assert.Equal(t, 3, result.SuccessCount)
	assert.Equal(t, 0, result.FailureCount)

	// Verify no actual operations were performed
	source.AssertExpectations(t)
	dest.AssertExpectations(t)
}

func TestMigratorWithRetry(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	source := new(MockStorageBackend)
	dest := new(MockStorageBackend)

	// Mock source listing
	objects := []backend.ObjectInfo{
		{Key: "file1.txt", Size: 100},
	}
	source.On("List", mock.Anything, "").Return(objects, nil)

	// Mock file operations
	content := "test content"
	reader := io.NopCloser(strings.NewReader(content))
	source.On("Retrieve", mock.Anything, "file1.txt").Return(reader, nil)

	// First check - file doesn't exist
	dest.On("Exists", mock.Anything, "file1.txt").Return(false, nil).Once()

	// First store attempt fails
	dest.On("Store", mock.Anything, "file1.txt", mock.Anything, int64(100)).Return(fmt.Errorf("network error")).Once()

	// Second store attempt succeeds
	dest.On("Store", mock.Anything, "file1.txt", mock.Anything, int64(100)).Return(nil).Once()

	// Mock metadata operations
	metadata := &backend.ObjectMetadata{
		UserMetadata: map[string]string{"key": "value"},
	}
	source.On("GetMetadata", mock.Anything, "file1.txt").Return(metadata, nil)
	dest.On("SetMetadata", mock.Anything, "file1.txt", metadata.UserMetadata).Return(nil)

	opts := MigrationOptions{
		DryRun:     false,
		Parallel:   1,
		MaxRetries: 1,
		RetryDelay: 10 * time.Millisecond,
	}

	migrator := NewMigrator(source, dest, opts, logger)
	result, err := migrator.Migrate(ctx, "")

	assert.NoError(t, err)
	assert.Equal(t, 1, result.SuccessCount)
	assert.Equal(t, 0, result.FailureCount)

	source.AssertExpectations(t)
	dest.AssertExpectations(t)
}

func TestMigratorSkipExisting(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	source := new(MockStorageBackend)
	dest := new(MockStorageBackend)

	// Mock source listing
	objects := []backend.ObjectInfo{
		{Key: "existing.txt", Size: 100},
		{Key: "new.txt", Size: 200},
	}
	source.On("List", mock.Anything, "").Return(objects, nil)

	// First file already exists
	dest.On("Exists", mock.Anything, "existing.txt").Return(true, nil)

	// Second file doesn't exist
	dest.On("Exists", mock.Anything, "new.txt").Return(false, nil)

	// Mock operations for new file
	content := "new content"
	reader := io.NopCloser(strings.NewReader(content))
	source.On("Retrieve", mock.Anything, "new.txt").Return(reader, nil)
	dest.On("Store", mock.Anything, "new.txt", mock.Anything, int64(200)).Return(nil)

	// Mock metadata
	source.On("GetMetadata", mock.Anything, "new.txt").Return(&backend.ObjectMetadata{}, nil)

	opts := MigrationOptions{
		DryRun:   false,
		Parallel: 2,
	}

	migrator := NewMigrator(source, dest, opts, logger)
	result, err := migrator.Migrate(ctx, "")

	assert.NoError(t, err)
	assert.Equal(t, 2, result.TotalFiles)
	assert.Equal(t, 1, result.SuccessCount)
	assert.Equal(t, 1, result.SkippedCount)
	assert.Equal(t, 0, result.FailureCount)

	source.AssertExpectations(t)
	dest.AssertExpectations(t)
}

func TestMigratorResume(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	source := new(MockStorageBackend)
	dest := new(MockStorageBackend)

	// Mock source listing
	objects := []backend.ObjectInfo{
		{Key: "file1.txt", Size: 100},
		{Key: "file2.txt", Size: 200},
		{Key: "file3.txt", Size: 300},
	}
	source.On("List", mock.Anything, "").Return(objects, nil)

	// Only file3.txt should be processed
	dest.On("Exists", mock.Anything, "file3.txt").Return(false, nil)

	content := "content3"
	reader := io.NopCloser(strings.NewReader(content))
	source.On("Retrieve", mock.Anything, "file3.txt").Return(reader, nil)
	dest.On("Store", mock.Anything, "file3.txt", mock.Anything, int64(300)).Return(nil)
	source.On("GetMetadata", mock.Anything, "file3.txt").Return(&backend.ObjectMetadata{}, nil)

	opts := MigrationOptions{
		DryRun:        false,
		Parallel:      1,
		ResumeFromKey: "file3.txt",
	}

	migrator := NewMigrator(source, dest, opts, logger)
	result, err := migrator.Migrate(ctx, "")

	assert.NoError(t, err)
	assert.Equal(t, 1, result.TotalFiles) // Only file3.txt
	assert.Equal(t, 1, result.SuccessCount)

	source.AssertExpectations(t)
	dest.AssertExpectations(t)
}
