package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage"
)

func TestFilesystemDriver(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "filesystem-driver-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	config := Config{
		BasePath: tempDir,
		FileMode: 0o644,
		DirMode:  0o755,
		Config:   storage.DefaultConfig(),
	}

	driver, err := NewDriver(config, logger)
	require.NoError(t, err)
	require.NotNil(t, driver)

	ctx := context.Background()

	t.Run("Put and Get", func(t *testing.T) {
		key := "test/data.txt"
		data := []byte("hello world")

		// Put data
		err := driver.Put(ctx, key, data)
		require.NoError(t, err)

		// Get data
		retrieved, err := driver.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, data, retrieved)
	})

	t.Run("Get non-existent key", func(t *testing.T) {
		_, err := driver.Get(ctx, "non-existent")
		assert.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("Delete", func(t *testing.T) {
		key := "test/delete.txt"
		data := []byte("delete me")

		// Put data
		err := driver.Put(ctx, key, data)
		require.NoError(t, err)

		// Delete
		err = driver.Delete(ctx, key)
		require.NoError(t, err)

		// Verify deleted
		_, err = driver.Get(ctx, key)
		assert.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("Exists", func(t *testing.T) {
		key := "test/exists.txt"
		data := []byte("I exist")

		// Check non-existent
		exists, err := driver.Exists(ctx, key)
		require.NoError(t, err)
		assert.False(t, exists)

		// Put data
		err = driver.Put(ctx, key, data)
		require.NoError(t, err)

		// Check exists
		exists, err = driver.Exists(ctx, key)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("List", func(t *testing.T) {
		// Put multiple files
		keys := []string{
			"list/file1.txt",
			"list/file2.txt",
			"list/subdir/file3.txt",
			"other/file4.txt",
		}

		for _, key := range keys {
			err := driver.Put(ctx, key, []byte("data"))
			require.NoError(t, err)
		}

		// List with prefix
		listed, err := driver.List(ctx, "list/")
		require.NoError(t, err)
		assert.Len(t, listed, 3)

		// List all
		listed, err = driver.List(ctx, "")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(listed), 4)
	})

	t.Run("GetURL", func(t *testing.T) {
		key := "test/url.txt"
		data := []byte("url test")

		// Put data
		err := driver.Put(ctx, key, data)
		require.NoError(t, err)

		// Get URL
		url, err := driver.GetURL(ctx, key)
		require.NoError(t, err)
		assert.Contains(t, url, "file://")
		assert.Contains(t, url, filepath.Join(tempDir, "test", "url.txt"))
	})

	t.Run("Invalid keys", func(t *testing.T) {
		invalidKeys := []string{
			"",
			"../escape",
			"/absolute",
			"\\absolute",
			"path/../traversal",
		}

		for _, key := range invalidKeys {
			err := driver.Put(ctx, key, []byte("data"))
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid key")
		}
	})
}
