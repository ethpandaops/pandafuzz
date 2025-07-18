package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/storage/backend"
	"github.com/sirupsen/logrus"
)

// LocalFileStorage implements the FileStorage interface using the local filesystem
type LocalFileStorage struct {
	basePath string
	logger   logrus.FieldLogger
}

// NewLocalFileStorage creates a new local file storage instance
func NewLocalFileStorage(basePath string, logger *logrus.Logger) common.FileStorage {
	return &LocalFileStorage{
		basePath: basePath,
		logger:   logger.WithField("component", "file_storage"),
	}
}

// SaveFile saves data to a file
func (fs *LocalFileStorage) SaveFile(ctx context.Context, path string, data []byte) error {
	// Ensure path is relative
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths not allowed: %s", path)
	}

	// Sanitize path to prevent directory traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid path: %s", path)
	}

	fullPath := filepath.Join(fs.basePath, cleanPath)

	// Create directory if it doesn't exist
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fs.logger.WithFields(logrus.Fields{
		"path": path,
		"size": len(data),
	}).Debug("File saved")

	return nil
}

// ReadFile reads data from a file
func (fs *LocalFileStorage) ReadFile(ctx context.Context, path string) ([]byte, error) {
	// Ensure path is relative
	if filepath.IsAbs(path) {
		return nil, fmt.Errorf("absolute paths not allowed: %s", path)
	}

	// Sanitize path to prevent directory traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("invalid path: %s", path)
	}

	fullPath := filepath.Join(fs.basePath, cleanPath)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", path)
		}
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	fs.logger.WithFields(logrus.Fields{
		"path": path,
		"size": len(data),
	}).Debug("File read")

	return data, nil
}

// DeleteFile deletes a file
func (fs *LocalFileStorage) DeleteFile(ctx context.Context, path string) error {
	// Ensure path is relative
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths not allowed: %s", path)
	}

	// Sanitize path to prevent directory traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid path: %s", path)
	}

	fullPath := filepath.Join(fs.basePath, cleanPath)

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", path)
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}

	fs.logger.WithField("path", path).Debug("File deleted")

	return nil
}

// ListFiles lists files with a given prefix
func (fs *LocalFileStorage) ListFiles(ctx context.Context, prefix string) ([]string, error) {
	// Ensure prefix is relative
	if filepath.IsAbs(prefix) {
		return nil, fmt.Errorf("absolute paths not allowed: %s", prefix)
	}

	// Sanitize prefix to prevent directory traversal
	cleanPrefix := filepath.Clean(prefix)
	if strings.Contains(cleanPrefix, "..") {
		return nil, fmt.Errorf("invalid prefix: %s", prefix)
	}

	searchPath := filepath.Join(fs.basePath, cleanPrefix)
	var files []string

	err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			// Get relative path from base
			relPath, err := filepath.Rel(fs.basePath, path)
			if err != nil {
				return err
			}
			files = append(files, relPath)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	fs.logger.WithFields(logrus.Fields{
		"prefix": prefix,
		"count":  len(files),
	}).Debug("Files listed")

	return files, nil
}

// FileExists checks if a file exists
func (fs *LocalFileStorage) FileExists(ctx context.Context, path string) (bool, error) {
	// Ensure path is relative
	if filepath.IsAbs(path) {
		return false, fmt.Errorf("absolute paths not allowed: %s", path)
	}

	// Sanitize path to prevent directory traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return false, fmt.Errorf("invalid path: %s", path)
	}

	fullPath := filepath.Join(fs.basePath, cleanPath)

	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	return true, nil
}

// CopyFile copies a file from source to destination
func (fs *LocalFileStorage) CopyFile(ctx context.Context, src, dst string) error {
	// Check context cancellation early
	select {
	case <-ctx.Done():
		return fmt.Errorf("context cancelled: %w", ctx.Err())
	default:
	}

	srcData, err := fs.ReadFile(ctx, src)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	if err := fs.SaveFile(ctx, dst, srcData); err != nil {
		return fmt.Errorf("failed to write destination file: %w", err)
	}

	fs.logger.WithFields(logrus.Fields{
		"src": src,
		"dst": dst,
	}).Debug("File copied")

	return nil
}

// StreamFile streams a file for efficient handling of large files
func (fs *LocalFileStorage) StreamFile(ctx context.Context, path string, writer io.Writer) error {
	// Ensure path is relative
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths not allowed: %s", path)
	}

	// Sanitize path to prevent directory traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid path: %s", path)
	}

	fullPath := filepath.Join(fs.basePath, cleanPath)

	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", path)
		}
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	written, err := io.Copy(writer, file)
	if err != nil {
		return fmt.Errorf("failed to stream file: %w", err)
	}

	fs.logger.WithFields(logrus.Fields{
		"path":  path,
		"bytes": written,
	}).Debug("File streamed")

	return nil
}

// BackendFileStorage implements the FileStorage interface using a storage backend
type BackendFileStorage struct {
	backend backend.StorageBackend
	logger  logrus.FieldLogger
}

// NewBackendFileStorage creates a new backend file storage instance
func NewBackendFileStorage(storageBackend backend.StorageBackend, logger *logrus.Logger) common.FileStorage {
	return &BackendFileStorage{
		backend: storageBackend,
		logger:  logger.WithField("component", "backend_file_storage"),
	}
}

// SaveFile saves data to a file using the storage backend
func (fs *BackendFileStorage) SaveFile(ctx context.Context, path string, data []byte) error {
	// Ensure path is relative
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths not allowed: %s", path)
	}

	// Sanitize path to prevent directory traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid path: %s", path)
	}

	// Convert Windows paths to forward slashes for S3 compatibility
	key := strings.ReplaceAll(cleanPath, "\\", "/")

	reader := bytes.NewReader(data)
	if err := fs.backend.Store(ctx, key, reader, int64(len(data))); err != nil {
		return fmt.Errorf("failed to store file: %w", err)
	}

	fs.logger.WithFields(logrus.Fields{
		"key":  key,
		"size": len(data),
	}).Debug("File saved to backend storage")

	return nil
}

// ReadFile reads data from a file using the storage backend
func (fs *BackendFileStorage) ReadFile(ctx context.Context, path string) ([]byte, error) {
	// Ensure path is relative
	if filepath.IsAbs(path) {
		return nil, fmt.Errorf("absolute paths not allowed: %s", path)
	}

	// Sanitize path to prevent directory traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("invalid path: %s", path)
	}

	// Convert Windows paths to forward slashes for S3 compatibility
	key := strings.ReplaceAll(cleanPath, "\\", "/")

	reader, err := fs.backend.Retrieve(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve file: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}

	fs.logger.WithFields(logrus.Fields{
		"key":  key,
		"size": len(data),
	}).Debug("File read from backend storage")

	return data, nil
}

// DeleteFile deletes a file using the storage backend
func (fs *BackendFileStorage) DeleteFile(ctx context.Context, path string) error {
	// Ensure path is relative
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths not allowed: %s", path)
	}

	// Sanitize path to prevent directory traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid path: %s", path)
	}

	// Convert Windows paths to forward slashes for S3 compatibility
	key := strings.ReplaceAll(cleanPath, "\\", "/")

	if err := fs.backend.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	fs.logger.WithField("key", key).Debug("File deleted from backend storage")

	return nil
}

// ListFiles lists files with a given prefix using the storage backend
func (fs *BackendFileStorage) ListFiles(ctx context.Context, prefix string) ([]string, error) {
	// Ensure prefix is relative
	if filepath.IsAbs(prefix) {
		return nil, fmt.Errorf("absolute paths not allowed: %s", prefix)
	}

	// Sanitize prefix to prevent directory traversal
	cleanPrefix := filepath.Clean(prefix)
	if strings.Contains(cleanPrefix, "..") {
		return nil, fmt.Errorf("invalid prefix: %s", prefix)
	}

	// Convert Windows paths to forward slashes for S3 compatibility
	keyPrefix := strings.ReplaceAll(cleanPrefix, "\\", "/")

	objects, err := fs.backend.List(ctx, keyPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	var files []string
	for _, obj := range objects {
		// Convert back to local path format
		localPath := strings.ReplaceAll(obj.Key, "/", string(filepath.Separator))
		files = append(files, localPath)
	}

	fs.logger.WithFields(logrus.Fields{
		"prefix": prefix,
		"count":  len(files),
	}).Debug("Files listed from backend storage")

	return files, nil
}

// FileExists checks if a file exists using the storage backend
func (fs *BackendFileStorage) FileExists(ctx context.Context, path string) (bool, error) {
	// Ensure path is relative
	if filepath.IsAbs(path) {
		return false, fmt.Errorf("absolute paths not allowed: %s", path)
	}

	// Sanitize path to prevent directory traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return false, fmt.Errorf("invalid path: %s", path)
	}

	// Convert Windows paths to forward slashes for S3 compatibility
	key := strings.ReplaceAll(cleanPath, "\\", "/")

	exists, err := fs.backend.Exists(ctx, key)
	if err != nil {
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	return exists, nil
}

// CopyFile copies a file from source to destination using the storage backend
func (fs *BackendFileStorage) CopyFile(ctx context.Context, src, dst string) error {
	// Check context cancellation early
	select {
	case <-ctx.Done():
		return fmt.Errorf("context cancelled: %w", ctx.Err())
	default:
	}

	// Read source file
	srcData, err := fs.ReadFile(ctx, src)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	// Write to destination
	if err := fs.SaveFile(ctx, dst, srcData); err != nil {
		return fmt.Errorf("failed to write destination file: %w", err)
	}

	fs.logger.WithFields(logrus.Fields{
		"src": src,
		"dst": dst,
	}).Debug("File copied in backend storage")

	return nil
}

// StreamFile streams a file for efficient handling of large files using the storage backend
func (fs *BackendFileStorage) StreamFile(ctx context.Context, path string, writer io.Writer) error {
	// Ensure path is relative
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths not allowed: %s", path)
	}

	// Sanitize path to prevent directory traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid path: %s", path)
	}

	// Convert Windows paths to forward slashes for S3 compatibility
	key := strings.ReplaceAll(cleanPath, "\\", "/")

	// Retrieve file as stream
	reader, err := fs.backend.Retrieve(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to retrieve file stream: %w", err)
	}
	defer reader.Close()

	// Stream to writer with context cancellation support
	written, err := io.Copy(writer, reader)
	if err != nil {
		return fmt.Errorf("failed to stream file: %w", err)
	}

	fs.logger.WithFields(logrus.Fields{
		"key":   key,
		"bytes": written,
	}).Debug("File streamed from backend storage")

	return nil
}
