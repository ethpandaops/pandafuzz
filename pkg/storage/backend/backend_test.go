package backend

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/config"
)

// TestFilesystemBackend tests the filesystem backend implementation
func TestFilesystemBackend(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "pandafuzz-fs-test-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	cfg := config.FilesystemConfig{
		BasePath: tmpDir,
	}

	backend, err := NewFilesystemBackend(cfg, logger, nil)
	require.NoError(t, err)

	ctx := context.Background()

	// Test health check
	err = backend.HealthCheck(ctx)
	assert.NoError(t, err)

	// Test store and retrieve
	testKey := "test/corpus/file1.bin"
	testData := []byte("test corpus data")

	err = backend.Store(ctx, testKey, bytes.NewReader(testData), int64(len(testData)))
	assert.NoError(t, err)

	// Test exists
	exists, err := backend.Exists(ctx, testKey)
	assert.NoError(t, err)
	assert.True(t, exists)

	// Test retrieve
	reader, err := backend.Retrieve(ctx, testKey)
	assert.NoError(t, err)
	defer reader.Close()

	retrieved, err := io.ReadAll(reader)
	assert.NoError(t, err)
	assert.Equal(t, testData, retrieved)

	// Test metadata
	metadata, err := backend.GetMetadata(ctx, testKey)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(testData)), metadata.Size)
	assert.Equal(t, "application/octet-stream", metadata.ContentType)

	// Test set metadata
	userMeta := map[string]string{
		"fuzzer": "aflplusplus",
		"edges":  "12345",
	}
	err = backend.SetMetadata(ctx, testKey, userMeta)
	assert.NoError(t, err)

	// Verify metadata was set
	metadata, err = backend.GetMetadata(ctx, testKey)
	assert.NoError(t, err)
	assert.Equal(t, "aflplusplus", metadata.UserMetadata["fuzzer"])
	assert.Equal(t, "12345", metadata.UserMetadata["edges"])

	// Test list
	objects, err := backend.List(ctx, "test/")
	assert.NoError(t, err)
	assert.Len(t, objects, 1)
	assert.Equal(t, testKey, objects[0].Key)

	// Test presigned URLs (filesystem returns file:// URLs)
	getURL, err := backend.GetPresignedURL(ctx, testKey, 1*time.Hour)
	assert.NoError(t, err)
	assert.Contains(t, getURL, "file://")

	putURL, err := backend.PutPresignedURL(ctx, "test/corpus/file2.bin", 1*time.Hour)
	assert.NoError(t, err)
	assert.Contains(t, putURL, "file://")

	// Test delete
	err = backend.Delete(ctx, testKey)
	assert.NoError(t, err)

	exists, err = backend.Exists(ctx, testKey)
	assert.NoError(t, err)
	assert.False(t, exists)

	// Test DeleteMany
	keys := []string{"test/file1", "test/file2", "test/file3"}
	for _, key := range keys {
		err = backend.Store(ctx, key, bytes.NewReader([]byte("data")), 4)
		require.NoError(t, err)
	}

	err = backend.DeleteMany(ctx, keys)
	assert.NoError(t, err)

	for _, key := range keys {
		exists, err = backend.Exists(ctx, key)
		assert.NoError(t, err)
		assert.False(t, exists)
	}
}

// TestStorageBackendInterface ensures both backends implement the interface correctly
func TestStorageBackendInterface(t *testing.T) {
	// Compile-time checks
	var _ StorageBackend = (*FilesystemBackend)(nil)
	var _ StorageBackend = (*S3Backend)(nil)
}

// TestFactory tests the storage backend factory
func TestFactory(t *testing.T) {
	logger := logrus.New()
	tmpDir := filepath.Join(os.TempDir(), "pandafuzz-factory-test")
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	tests := []struct {
		name    string
		config  config.StorageConfig
		wantErr bool
	}{
		{
			name: "filesystem backend",
			config: config.StorageConfig{
				Type: config.StorageTypeFilesystem,
				Filesystem: config.FilesystemConfig{
					BasePath: tmpDir,
				},
				MaxFileSize: 100 * 1024 * 1024, // 100MB
			},
			wantErr: false,
		},
		{
			name: "invalid type",
			config: config.StorageConfig{
				Type: "invalid",
			},
			wantErr: true,
		},
		{
			name: "filesystem without base path",
			config: config.StorageConfig{
				Type:       config.StorageTypeFilesystem,
				Filesystem: config.FilesystemConfig{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := NewStorageBackend(tt.config, logger)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, backend)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, backend)
			}
		})
	}
}
