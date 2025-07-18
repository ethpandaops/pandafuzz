package backend

import (
	"context"
	"crypto/md5" //nolint:gosec // MD5 is used for ETag generation, not security
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/config"
	"github.com/ethpandaops/pandafuzz/pkg/monitoring"
)

// FilesystemBackend implements StorageBackend using local filesystem.
type FilesystemBackend struct {
	basePath string
	logger   logrus.FieldLogger
	metrics  *monitoring.StorageMetrics
}

// Ensure FilesystemBackend implements StorageBackend interface
var _ StorageBackend = (*FilesystemBackend)(nil)

// NewFilesystemBackend creates a new filesystem storage backend
func NewFilesystemBackend(cfg config.FilesystemConfig, logger logrus.FieldLogger, metrics *monitoring.StorageMetrics) (*FilesystemBackend, error) {
	// Ensure base path exists
	if err := os.MkdirAll(cfg.BasePath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	// Create subdirectories for different bucket types
	buckets := []string{"corpus", "quarantine", "backup"}
	for _, bucket := range buckets {
		bucketPath := filepath.Join(cfg.BasePath, bucket)
		if err := os.MkdirAll(bucketPath, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create %s directory: %w", bucket, err)
		}
	}

	backend := &FilesystemBackend{
		basePath: cfg.BasePath,
		logger:   logger.WithField("backend", "filesystem"),
		metrics:  metrics,
	}

	// Log initialization
	backend.logger.WithFields(logrus.Fields{
		"base_path": cfg.BasePath,
		"buckets":   buckets,
	}).Info("Initialized filesystem storage backend")

	return backend, nil
}

// Store saves data to a file.
// It writes to a temporary file first and then atomically renames it to ensure data integrity.
func (fs *FilesystemBackend) Store(ctx context.Context, key string, reader io.Reader, size int64) error {
	startTime := time.Now()
	if fs.metrics != nil {
		defer fs.metrics.TrackActiveOperation("filesystem", "store")()
	}

	// Add request ID to logger if available
	logger := monitoring.WithRequestID(ctx, fs.logger)

	fullPath := fs.getFullPath("corpus", key)
	logger = logger.WithFields(logrus.Fields{
		"key":           key,
		"path":          fullPath,
		"expected_size": size,
	})

	logger.Debug("Starting file store operation")

	// Create directory if it doesn't exist
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		err = fmt.Errorf("failed to create directory: %w", err)
		if fs.metrics != nil {
			fs.metrics.RecordOperation("filesystem", "store", "corpus", size, err, time.Since(startTime))
		}
		logger.WithError(err).Error("Failed to create directory for storage")
		return err
	}

	// Create temporary file first
	tempPath := fullPath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		err = fmt.Errorf("failed to create file: %w", err)
		if fs.metrics != nil {
			fs.metrics.RecordOperation("filesystem", "store", "corpus", size, err, time.Since(startTime))
		}
		logger.WithError(err).Error("Failed to create temporary file")
		return err
	}
	defer file.Close()

	// Copy data
	written, err := io.Copy(file, reader)
	if err != nil {
		_ = os.Remove(tempPath)
		err = fmt.Errorf("failed to write file: %w", err)
		if fs.metrics != nil {
			fs.metrics.RecordOperation("filesystem", "store", "corpus", written, err, time.Since(startTime))
		}
		logger.WithError(err).WithField("bytes_written", written).Error("Failed to write file content")
		return err
	}

	if size >= 0 && written != size {
		_ = os.Remove(tempPath)
		err = fmt.Errorf("size mismatch: expected %d, wrote %d", size, written)
		if fs.metrics != nil {
			fs.metrics.RecordOperation("filesystem", "store", "corpus", written, err, time.Since(startTime))
		}
		logger.WithError(err).WithFields(logrus.Fields{
			"expected": size,
			"actual":   written,
		}).Error("File size mismatch")
		return err
	}

	// Sync to disk
	if err := file.Sync(); err != nil {
		_ = os.Remove(tempPath)
		err = fmt.Errorf("failed to sync file: %w", err)
		if fs.metrics != nil {
			fs.metrics.RecordOperation("filesystem", "store", "corpus", written, err, time.Since(startTime))
		}
		logger.WithError(err).Error("Failed to sync file to disk")
		return err
	}

	// Atomic rename
	if err := os.Rename(tempPath, fullPath); err != nil {
		_ = os.Remove(tempPath)
		err = fmt.Errorf("failed to rename file: %w", err)
		if fs.metrics != nil {
			fs.metrics.RecordOperation("filesystem", "store", "corpus", written, err, time.Since(startTime))
		}
		logger.WithError(err).Error("Failed to perform atomic rename")
		return err
	}

	// Store metadata
	if err := fs.writeMetadata(fullPath, nil); err != nil {
		logger.WithError(err).Warn("Failed to write metadata")
	}

	// Record success metrics
	if fs.metrics != nil {
		fs.metrics.RecordOperation("filesystem", "store", "corpus", written, nil, time.Since(startTime))
	}

	logger.WithFields(logrus.Fields{
		"size":     written,
		"duration": time.Since(startTime).Seconds(),
	}).Info("Successfully stored file")

	return nil
}

// Retrieve reads data from a file.
// The caller is responsible for closing the returned reader.
func (fs *FilesystemBackend) Retrieve(ctx context.Context, key string) (io.ReadCloser, error) {
	startTime := time.Now()
	if fs.metrics != nil {
		defer fs.metrics.TrackActiveOperation("filesystem", "retrieve")()
	}

	logger := monitoring.WithRequestID(ctx, fs.logger)
	fullPath := fs.getFullPath("corpus", key)

	logger = logger.WithFields(logrus.Fields{
		"key":  key,
		"path": fullPath,
	})

	logger.Debug("Starting file retrieve operation")

	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			err = fmt.Errorf("object not found: %s", key)
			if fs.metrics != nil {
				fs.metrics.RecordOperation("filesystem", "retrieve", "corpus", 0, err, time.Since(startTime))
			}
			logger.WithError(err).Warn("File not found")
			return nil, err
		}
		err = fmt.Errorf("failed to open file: %w", err)
		if fs.metrics != nil {
			fs.metrics.RecordOperation("filesystem", "retrieve", "corpus", 0, err, time.Since(startTime))
		}
		logger.WithError(err).Error("Failed to open file")
		return nil, err
	}

	// Get file size for metrics
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		err = fmt.Errorf("failed to stat file: %w", err)
		if fs.metrics != nil {
			fs.metrics.RecordOperation("filesystem", "retrieve", "corpus", 0, err, time.Since(startTime))
		}
		logger.WithError(err).Error("Failed to stat file")
		return nil, err
	}

	// Record success metrics
	if fs.metrics != nil {
		fs.metrics.RecordOperation("filesystem", "retrieve", "corpus", stat.Size(), nil, time.Since(startTime))
	}

	logger.WithFields(logrus.Fields{
		"size":     stat.Size(),
		"duration": time.Since(startTime).Seconds(),
	}).Debug("Successfully opened file for retrieval")

	return file, nil
}

// Delete removes a file and its associated metadata.
func (fs *FilesystemBackend) Delete(ctx context.Context, key string) error {
	fullPath := fs.getFullPath("corpus", key)

	// Remove metadata first
	metaPath := fullPath + ".meta"
	_ = os.Remove(metaPath)

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("object not found: %s", key)
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}

	fs.logger.WithField("key", key).Debug("Deleted file")
	return nil
}

// Exists checks if a file exists in the filesystem.
func (fs *FilesystemBackend) Exists(ctx context.Context, key string) (bool, error) {
	fullPath := fs.getFullPath("corpus", key)

	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	return true, nil
}

// List returns files with the given prefix.
// It walks the filesystem recursively and filters out metadata files.
func (fs *FilesystemBackend) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	searchPath := fs.getFullPath("corpus", prefix)
	baseDir := filepath.Join(fs.basePath, "corpus")

	var objects []ObjectInfo

	// If prefix is empty, search entire corpus directory
	if prefix == "" {
		searchPath = baseDir
	}

	walkErr := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and metadata files
		if info.IsDir() || strings.HasSuffix(path, ".meta") {
			return nil
		}

		// Get relative path from corpus base
		relPath, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}

		// Convert to forward slashes for consistency with S3
		key := filepath.ToSlash(relPath)

		// Calculate ETag (MD5 for small files)
		etag := ""
		if info.Size() < 5*1024*1024 { // 5MB
			if data, err := os.ReadFile(path); err == nil {
				hash := md5.Sum(data) //nolint:gosec // MD5 used for ETag, not security
				etag = hex.EncodeToString(hash[:])
			}
		}

		objects = append(objects, ObjectInfo{
			Key:          key,
			Size:         info.Size(),
			LastModified: info.ModTime(),
			ETag:         etag,
		})

		return nil
	})

	if walkErr != nil {
		if os.IsNotExist(walkErr) {
			return []ObjectInfo{}, nil
		}
		return nil, fmt.Errorf("failed to list files: %w", walkErr)
	}

	fs.logger.WithFields(logrus.Fields{
		"prefix": prefix,
		"count":  len(objects),
	}).Debug("Listed files")

	return objects, nil
}

// DeleteMany removes multiple files.
// It continues attempting all deletions even if some fail.
func (fs *FilesystemBackend) DeleteMany(ctx context.Context, keys []string) error {
	var errs []error

	for _, key := range keys {
		if err := fs.Delete(ctx, key); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete %s: %w", key, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to delete %d objects: %v", len(errs), errs)
	}

	fs.logger.WithField("count", len(keys)).Debug("Deleted multiple files")
	return nil
}

// GetMetadata retrieves file metadata including user-defined metadata from sidecar files.
func (fs *FilesystemBackend) GetMetadata(ctx context.Context, key string) (*ObjectMetadata, error) {
	fullPath := fs.getFullPath("corpus", key)

	stat, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("object not found: %s", key)
		}
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Read user metadata from sidecar file
	userMeta := make(map[string]string)
	metaPath := fullPath + ".meta"
	if data, err := os.ReadFile(metaPath); err == nil {
		_ = json.Unmarshal(data, &userMeta)
	}

	// Calculate ETag
	etag := ""
	if stat.Size() < 5*1024*1024 { // 5MB
		if data, err := os.ReadFile(fullPath); err == nil {
			hash := md5.Sum(data) //nolint:gosec // MD5 used for ETag, not security
			etag = hex.EncodeToString(hash[:])
		}
	}

	return &ObjectMetadata{
		ContentType:  "application/octet-stream",
		Size:         stat.Size(),
		ETag:         etag,
		LastModified: stat.ModTime(),
		UserMetadata: userMeta,
	}, nil
}

// SetMetadata updates file metadata by writing to a sidecar .meta file.
func (fs *FilesystemBackend) SetMetadata(ctx context.Context, key string, metadata map[string]string) error {
	fullPath := fs.getFullPath("corpus", key)

	// Verify file exists
	if _, err := os.Stat(fullPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("object not found: %s", key)
		}
		return fmt.Errorf("failed to check file: %w", err)
	}

	return fs.writeMetadata(fullPath, metadata)
}

// GetPresignedURL returns a file:// URL for the filesystem backend.
// Note: This doesn't provide actual presigning functionality as filesystem access is direct.
func (fs *FilesystemBackend) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	startTime := time.Now()
	logger := monitoring.WithRequestID(ctx, fs.logger)

	logger = logger.WithFields(logrus.Fields{
		"key":    key,
		"expiry": expiry.String(),
	})

	logger.Debug("Generating presigned GET URL for filesystem")

	// For filesystem backend, return a file:// URL
	fullPath := fs.getFullPath("corpus", key)

	// Verify file exists
	if _, err := os.Stat(fullPath); err != nil {
		if os.IsNotExist(err) {
			err = fmt.Errorf("object not found: %s", key)
			if fs.metrics != nil {
				fs.metrics.RecordPresignedURL("filesystem", "get", "corpus", err, time.Since(startTime))
			}
			logger.WithError(err).Warn("File not found for presigned URL")
			return "", err
		}
		err = fmt.Errorf("failed to check file: %w", err)
		if fs.metrics != nil {
			fs.metrics.RecordPresignedURL("filesystem", "get", "corpus", err, time.Since(startTime))
		}
		logger.WithError(err).Error("Failed to check file existence")
		return "", err
	}

	url := "file://" + fullPath

	// Record success metrics
	if fs.metrics != nil {
		fs.metrics.RecordPresignedURL("filesystem", "get", "corpus", nil, time.Since(startTime))
	}

	logger.WithFields(logrus.Fields{
		"url":      url,
		"duration": time.Since(startTime).Seconds(),
	}).Debug("Generated presigned GET URL")

	return url, nil
}

// PutPresignedURL returns a file:// URL for the filesystem backend.
// Note: This doesn't provide actual presigning functionality as filesystem access is direct.
func (fs *FilesystemBackend) PutPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	startTime := time.Now()
	logger := monitoring.WithRequestID(ctx, fs.logger)

	logger = logger.WithFields(logrus.Fields{
		"key":    key,
		"expiry": expiry.String(),
	})

	logger.Debug("Generating presigned PUT URL for filesystem")

	// For filesystem backend, return a file:// URL
	fullPath := fs.getFullPath("corpus", key)

	// Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		err = fmt.Errorf("failed to create directory: %w", err)
		if fs.metrics != nil {
			fs.metrics.RecordPresignedURL("filesystem", "put", "corpus", err, time.Since(startTime))
		}
		logger.WithError(err).Error("Failed to create directory for presigned PUT URL")
		return "", err
	}

	url := "file://" + fullPath

	// Record success metrics
	if fs.metrics != nil {
		fs.metrics.RecordPresignedURL("filesystem", "put", "corpus", nil, time.Since(startTime))
	}

	logger.WithFields(logrus.Fields{
		"url":      url,
		"duration": time.Since(startTime).Seconds(),
	}).Debug("Generated presigned PUT URL")

	return url, nil
}

// HealthCheck verifies filesystem accessibility by attempting to write and read a test file.
func (fs *FilesystemBackend) HealthCheck(ctx context.Context) error {
	startTime := time.Now()
	logger := monitoring.WithRequestID(ctx, fs.logger)

	logger.Debug("Starting filesystem health check")

	// Try to write and read a test file
	testPath := filepath.Join(fs.basePath, ".health-check")
	testData := []byte("health-check")

	// Test write
	if err := os.WriteFile(testPath, testData, 0o644); err != nil {
		err = fmt.Errorf("filesystem health check failed (write): %w", err)
		if fs.metrics != nil {
			fs.metrics.RecordHealthCheck("filesystem", err, time.Since(startTime))
		}
		logger.WithError(err).Error("Health check write failed")
		return err
	}

	// Test read
	if data, err := os.ReadFile(testPath); err != nil {
		_ = os.Remove(testPath)
		err = fmt.Errorf("filesystem health check failed (read): %w", err)
		if fs.metrics != nil {
			fs.metrics.RecordHealthCheck("filesystem", err, time.Since(startTime))
		}
		logger.WithError(err).Error("Health check read failed")
		return err
	} else if string(data) != string(testData) {
		_ = os.Remove(testPath)
		err = fmt.Errorf("filesystem health check failed: data mismatch")
		if fs.metrics != nil {
			fs.metrics.RecordHealthCheck("filesystem", err, time.Since(startTime))
		}
		logger.WithError(err).Error("Health check data mismatch")
		return err
	}

	// Cleanup
	_ = os.Remove(testPath)

	// Record success
	if fs.metrics != nil {
		fs.metrics.RecordHealthCheck("filesystem", nil, time.Since(startTime))
	}

	logger.WithField("duration", time.Since(startTime).Seconds()).Info("Filesystem health check passed")
	return nil
}

// Helper methods

func (fs *FilesystemBackend) getFullPath(bucket, key string) string {
	// Sanitize key to prevent directory traversal
	cleanKey := filepath.Clean(key)
	if strings.Contains(cleanKey, "..") {
		// Fallback to safe key
		cleanKey = strings.ReplaceAll(key, "..", "")
	}

	return filepath.Join(fs.basePath, bucket, cleanKey)
}

// getStorageBasePath returns the base storage path
func (fs *FilesystemBackend) getStorageBasePath() string {
	return fs.basePath
}

func (fs *FilesystemBackend) writeMetadata(fullPath string, metadata map[string]string) error {
	metaPath := fullPath + ".meta"

	// Read existing metadata
	existingMeta := make(map[string]string)
	if data, err := os.ReadFile(metaPath); err == nil {
		_ = json.Unmarshal(data, &existingMeta)
	}

	// Merge with new metadata
	if metadata != nil {
		for k, v := range metadata {
			existingMeta[k] = v
		}
	}

	// Always add timestamp
	existingMeta["last-modified"] = time.Now().UTC().Format(time.RFC3339)

	// Write metadata
	data, err := json.MarshalIndent(existingMeta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, data, 0o644)
}

// MoveToQuarantine moves a file from corpus to quarantine.
// It also moves the associated metadata file if it exists.
func (fs *FilesystemBackend) MoveToQuarantine(ctx context.Context, key string) error {
	srcPath := fs.getFullPath("corpus", key)
	dstPath := fs.getFullPath("quarantine", key)

	// Ensure destination directory exists
	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create quarantine directory: %w", err)
	}

	// Move file
	if err := os.Rename(srcPath, dstPath); err != nil {
		return fmt.Errorf("failed to move to quarantine: %w", err)
	}

	// Move metadata if exists
	srcMeta := srcPath + ".meta"
	dstMeta := dstPath + ".meta"
	_ = os.Rename(srcMeta, dstMeta)

	fs.logger.WithField("key", key).Info("Moved file to quarantine")
	return nil
}

// RestoreFromQuarantine moves a file from quarantine back to corpus.
// It also moves the associated metadata file if it exists.
func (fs *FilesystemBackend) RestoreFromQuarantine(ctx context.Context, key string) error {
	srcPath := fs.getFullPath("quarantine", key)
	dstPath := fs.getFullPath("corpus", key)

	// Ensure destination directory exists
	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create corpus directory: %w", err)
	}

	// Move file
	if err := os.Rename(srcPath, dstPath); err != nil {
		return fmt.Errorf("failed to restore from quarantine: %w", err)
	}

	// Move metadata if exists
	srcMeta := srcPath + ".meta"
	dstMeta := dstPath + ".meta"
	_ = os.Rename(srcMeta, dstMeta)

	fs.logger.WithField("key", key).Info("Restored file from quarantine")
	return nil
}

// BackupObject copies a file to the backup directory with a timestamp suffix.
func (fs *FilesystemBackend) BackupObject(ctx context.Context, key string) error {
	srcPath := fs.getFullPath("corpus", key)

	// Create backup key with timestamp
	backupKey := fmt.Sprintf("%s.%d", key, time.Now().Unix())
	dstPath := fs.getFullPath("backup", backupKey)

	// Ensure destination directory exists
	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Copy file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		_ = os.Remove(dstPath)
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Copy metadata if exists
	srcMeta := srcPath + ".meta"
	if _, err := os.Stat(srcMeta); err == nil {
		dstMeta := dstPath + ".meta"
		if srcData, err := os.ReadFile(srcMeta); err == nil {
			_ = os.WriteFile(dstMeta, srcData, 0o644)
		}
	}

	fs.logger.WithFields(logrus.Fields{
		"key":       key,
		"backupKey": backupKey,
	}).Debug("Backed up file")

	return nil
}
