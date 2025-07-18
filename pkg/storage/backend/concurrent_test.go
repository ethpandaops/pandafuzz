package backend

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/config"
)

// TestConcurrentAccess tests concurrent access to the storage backend
func TestConcurrentAccess(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "pandafuzz-concurrent-test-*")
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

	// Test concurrent writes
	t.Run("ConcurrentWrites", func(t *testing.T) {
		const numWorkers = 10
		const filesPerWorker = 10

		var wg sync.WaitGroup
		errors := make(chan error, numWorkers*filesPerWorker)

		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				for j := 0; j < filesPerWorker; j++ {
					key := fmt.Sprintf("worker-%d/file-%d.bin", workerID, j)
					data := []byte(fmt.Sprintf("worker %d file %d content", workerID, j))

					if err := backend.Store(ctx, key, bytes.NewReader(data), int64(len(data))); err != nil {
						errors <- fmt.Errorf("worker %d file %d: %w", workerID, j, err)
					}
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for errors
		var errs []error
		for err := range errors {
			errs = append(errs, err)
		}
		assert.Empty(t, errs, "Concurrent writes should not produce errors")

		// Verify all files were written
		objects, err := backend.List(ctx, "")
		assert.NoError(t, err)
		assert.Len(t, objects, numWorkers*filesPerWorker)
	})

	// Test concurrent reads
	t.Run("ConcurrentReads", func(t *testing.T) {
		// First, create a test file
		testKey := "shared/test.bin"
		testData := []byte("shared test data")
		err := backend.Store(ctx, testKey, bytes.NewReader(testData), int64(len(testData)))
		require.NoError(t, err)

		const numReaders = 20
		var wg sync.WaitGroup
		errors := make(chan error, numReaders)

		for i := 0; i < numReaders; i++ {
			wg.Add(1)
			go func(readerID int) {
				defer wg.Done()

				reader, err := backend.Retrieve(ctx, testKey)
				if err != nil {
					errors <- fmt.Errorf("reader %d: failed to retrieve: %w", readerID, err)
					return
				}
				defer reader.Close()

				buf := new(bytes.Buffer)
				if _, err := buf.ReadFrom(reader); err != nil {
					errors <- fmt.Errorf("reader %d: failed to read: %w", readerID, err)
					return
				}

				if !bytes.Equal(buf.Bytes(), testData) {
					errors <- fmt.Errorf("reader %d: data mismatch", readerID)
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for errors
		var errs []error
		for err := range errors {
			errs = append(errs, err)
		}
		assert.Empty(t, errs, "Concurrent reads should not produce errors")
	})

	// Test concurrent mixed operations
	t.Run("ConcurrentMixedOps", func(t *testing.T) {
		const numWorkers = 5
		const opsPerWorker = 10

		var wg sync.WaitGroup
		errors := make(chan error, numWorkers*opsPerWorker)

		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				for j := 0; j < opsPerWorker; j++ {
					key := fmt.Sprintf("mixed/worker-%d-op-%d.bin", workerID, j)
					data := []byte(fmt.Sprintf("mixed op %d-%d", workerID, j))

					// Store
					if err := backend.Store(ctx, key, bytes.NewReader(data), int64(len(data))); err != nil {
						errors <- fmt.Errorf("store %s: %w", key, err)
						continue
					}

					// Check exists
					exists, err := backend.Exists(ctx, key)
					if err != nil {
						errors <- fmt.Errorf("exists %s: %w", key, err)
						continue
					}
					if !exists {
						errors <- fmt.Errorf("file %s should exist", key)
						continue
					}

					// Get metadata
					meta, err := backend.GetMetadata(ctx, key)
					if err != nil {
						errors <- fmt.Errorf("metadata %s: %w", key, err)
						continue
					}
					if meta.Size != int64(len(data)) {
						errors <- fmt.Errorf("size mismatch for %s: expected %d, got %d", key, len(data), meta.Size)
					}

					// Set metadata
					userMeta := map[string]string{
						"worker": fmt.Sprintf("%d", workerID),
						"op":     fmt.Sprintf("%d", j),
					}
					if err := backend.SetMetadata(ctx, key, userMeta); err != nil {
						errors <- fmt.Errorf("set metadata %s: %w", key, err)
					}
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for errors
		var errs []error
		for err := range errors {
			errs = append(errs, err)
		}
		assert.Empty(t, errs, "Concurrent mixed operations should not produce errors")
	})

	// Test race condition with file deletion
	t.Run("ConcurrentDeleteWhileRead", func(t *testing.T) {
		key := "race/test.bin"
		data := []byte("race test data")

		// Create file
		err := backend.Store(ctx, key, bytes.NewReader(data), int64(len(data)))
		require.NoError(t, err)

		var wg sync.WaitGroup
		readErrors := make(chan error, 10)
		deleteErrors := make(chan error, 1)

		// Start readers
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(readerID int) {
				defer wg.Done()

				reader, err := backend.Retrieve(ctx, key)
				if err != nil {
					// It's OK if file was deleted
					return
				}
				defer reader.Close()

				buf := new(bytes.Buffer)
				_, _ = buf.ReadFrom(reader) // Ignore error, file might be deleted
			}(i)
		}

		// Start deleter
		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := backend.Delete(ctx, key); err != nil {
				deleteErrors <- err
			}
		}()

		wg.Wait()
		close(readErrors)
		close(deleteErrors)

		// Check delete errors
		for err := range deleteErrors {
			assert.NoError(t, err, "Delete should not fail")
		}

		// Verify file is deleted
		exists, err := backend.Exists(ctx, key)
		assert.NoError(t, err)
		assert.False(t, exists, "File should be deleted")
	})
}

// TestConcurrentBatchOperations tests concurrent batch operations
func TestConcurrentBatchOperations(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "pandafuzz-batch-test-*")
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

	// Create test files
	const numFiles = 100
	var keys []string
	for i := 0; i < numFiles; i++ {
		key := fmt.Sprintf("batch/file-%03d.bin", i)
		keys = append(keys, key)
		data := []byte(fmt.Sprintf("file %d content", i))
		err := backend.Store(ctx, key, bytes.NewReader(data), int64(len(data)))
		require.NoError(t, err)
	}

	// Test concurrent batch deletes
	t.Run("ConcurrentBatchDeletes", func(t *testing.T) {
		const numWorkers = 4
		batchSize := len(keys) / numWorkers

		var wg sync.WaitGroup
		errors := make(chan error, numWorkers)

		for i := 0; i < numWorkers; i++ {
			start := i * batchSize
			end := start + batchSize
			if i == numWorkers-1 {
				end = len(keys)
			}

			wg.Add(1)
			go func(batch []string, workerID int) {
				defer wg.Done()

				if err := backend.DeleteMany(ctx, batch); err != nil {
					errors <- fmt.Errorf("worker %d: %w", workerID, err)
				}
			}(keys[start:end], i)
		}

		wg.Wait()
		close(errors)

		// Check for errors
		var errs []error
		for err := range errors {
			errs = append(errs, err)
		}
		assert.Empty(t, errs, "Concurrent batch deletes should not produce errors")

		// Verify all files are deleted
		objects, err := backend.List(ctx, "batch/")
		assert.NoError(t, err)
		assert.Empty(t, objects, "All files should be deleted")
	})
}
