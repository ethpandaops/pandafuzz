package filesystem

import (
	"context"
	"crypto/md5" //nolint:gosec // MD5 is used for checksum validation, not security
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/abstraction"
)

var (
	// ErrInvalidConfig is returned when configuration is invalid
	ErrInvalidConfig = errors.New("invalid configuration")
)

// Config contains filesystem-specific configuration
type Config struct {
	// BasePath is the root directory for all storage operations
	BasePath string

	// FileMode is the permission mode for created files
	FileMode os.FileMode

	// DirMode is the permission mode for created directories
	DirMode os.FileMode

	// Common configuration options
	abstraction.Config
}

// DefaultConfig returns default filesystem configuration
func DefaultConfig() Config {
	return Config{
		BasePath: "/var/lib/pandafuzz/storage",
		FileMode: 0o644,
		DirMode:  0o755,
		Config:   abstraction.DefaultConfig(),
	}
}

// driver implements the abstraction.Driver interface using the local filesystem
type driver struct {
	config Config
	logger logrus.FieldLogger

	// mu protects concurrent operations on the same paths
	mu sync.RWMutex
}

// Ensure driver implements abstraction.Driver interface
var _ abstraction.Driver = (*driver)(nil)

// NewDriver creates a new filesystem storage driver
func NewDriver(config Config, logger logrus.FieldLogger) (abstraction.Driver, error) {
	// Validate configuration
	if config.BasePath == "" {
		return nil, fmt.Errorf("base path cannot be empty")
	}

	// Ensure base path is absolute
	absPath, err := filepath.Abs(config.BasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}
	config.BasePath = absPath

	// Create base directory if it doesn't exist
	if err := os.MkdirAll(config.BasePath, config.DirMode); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	// Test write permissions
	testFile := filepath.Join(config.BasePath, ".write_test")
	if err := os.WriteFile(testFile, []byte("test"), config.FileMode); err != nil {
		return nil, fmt.Errorf("base path is not writable: %w", err)
	}
	_ = os.Remove(testFile)

	d := &driver{
		config: config,
		logger: logger.WithField("driver", "filesystem"),
	}

	d.logger.WithFields(logrus.Fields{
		"base_path": config.BasePath,
		"file_mode": config.FileMode,
		"dir_mode":  config.DirMode,
	}).Info("Initialized filesystem storage driver")

	return d, nil
}

// Put stores data with the given key
func (d *driver) Put(ctx context.Context, key string, data []byte) error {
	// Validate key
	if err := d.validateKey(key); err != nil {
		return err
	}

	// Check data size
	if int64(len(data)) > d.config.MaxValueSize {
		return fmt.Errorf("data size %d exceeds maximum allowed size %d", len(data), d.config.MaxValueSize)
	}

	fullPath := d.getFullPath(key)
	d.logger.WithFields(logrus.Fields{
		"key":  key,
		"path": fullPath,
		"size": len(data),
	}).Debug("Storing data")

	// Create directory if needed
	dir := filepath.Dir(fullPath)
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := os.MkdirAll(dir, d.config.DirMode); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write to temporary file first for atomic operation
	tempPath := fullPath + ".tmp"
	if err := d.writeAtomic(tempPath, fullPath, data); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// Get retrieves data for the given key
func (d *driver) Get(ctx context.Context, key string) ([]byte, error) {
	// Validate key
	if err := d.validateKey(key); err != nil {
		return nil, err
	}

	fullPath := d.getFullPath(key)
	d.logger.WithFields(logrus.Fields{
		"key":  key,
		"path": fullPath,
	}).Debug("Retrieving data")

	d.mu.RLock()
	defer d.mu.RUnlock()

	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, abstraction.ErrNotFound
		}
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return data, nil
}

// Delete removes data for the given key
func (d *driver) Delete(ctx context.Context, key string) error {
	// Validate key
	if err := d.validateKey(key); err != nil {
		return err
	}

	fullPath := d.getFullPath(key)
	d.logger.WithFields(logrus.Fields{
		"key":  key,
		"path": fullPath,
	}).Debug("Deleting data")

	d.mu.Lock()
	defer d.mu.Unlock()

	err := os.Remove(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// Try to clean up empty directories
	d.cleanupEmptyDirs(filepath.Dir(fullPath))

	return nil
}

// List returns all keys with the given prefix
func (d *driver) List(ctx context.Context, prefix string) ([]string, error) {
	basePath := d.config.BasePath
	searchPath := basePath

	// If prefix is provided, adjust search path
	if prefix != "" {
		searchPath = filepath.Join(basePath, filepath.Dir(prefix))
	}

	d.logger.WithFields(logrus.Fields{
		"prefix":      prefix,
		"search_path": searchPath,
	}).Debug("Listing keys")

	d.mu.RLock()
	defer d.mu.RUnlock()

	var keys []string
	err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Convert path to key
		relPath, err := filepath.Rel(basePath, path)
		if err != nil {
			return err
		}

		// Convert to forward slashes for consistency
		key := filepath.ToSlash(relPath)

		// Check if key matches prefix
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return keys, nil
}

// Exists checks if a key exists in storage
func (d *driver) Exists(ctx context.Context, key string) (bool, error) {
	// Validate key
	if err := d.validateKey(key); err != nil {
		return false, err
	}

	fullPath := d.getFullPath(key)

	d.mu.RLock()
	defer d.mu.RUnlock()

	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat file: %w", err)
	}

	return true, nil
}

// GetURL returns a file:// URL for the stored data
func (d *driver) GetURL(ctx context.Context, key string) (string, error) {
	// Validate key
	if err := d.validateKey(key); err != nil {
		return "", err
	}

	// Check if file exists
	exists, err := d.Exists(ctx, key)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", abstraction.ErrNotFound
	}

	fullPath := d.getFullPath(key)

	// Create file:// URL
	u := &url.URL{
		Scheme: "file",
		Path:   fullPath,
	}

	return u.String(), nil
}

// validateKey checks if a key is valid
func (d *driver) validateKey(key string) error {
	if key == "" {
		return abstraction.ErrInvalidKey
	}

	if len(key) > d.config.MaxKeyLength {
		return fmt.Errorf("key length %d exceeds maximum allowed length %d", len(key), d.config.MaxKeyLength)
	}

	// Check for invalid characters
	if strings.Contains(key, "..") {
		return fmt.Errorf("%w: key contains '..'", abstraction.ErrInvalidKey)
	}

	if strings.HasPrefix(key, "/") || strings.HasPrefix(key, "\\") {
		return fmt.Errorf("%w: key cannot start with path separator", abstraction.ErrInvalidKey)
	}

	return nil
}

// getFullPath returns the full filesystem path for a key
func (d *driver) getFullPath(key string) string {
	// Convert forward slashes to OS-specific separators
	osKey := filepath.FromSlash(key)
	return filepath.Join(d.config.BasePath, osKey)
}

// writeAtomic writes data atomically by writing to a temp file and renaming
func (d *driver) writeAtomic(tempPath, finalPath string, data []byte) error {
	// Write to temporary file
	if err := os.WriteFile(tempPath, data, d.config.FileMode); err != nil {
		return err
	}

	// Ensure data is flushed to disk
	file, err := os.Open(tempPath)
	if err != nil {
		_ = os.Remove(tempPath)
		return err
	}

	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return err
	}
	_ = file.Close()

	// Atomic rename
	if err := os.Rename(tempPath, finalPath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}

	return nil
}

// cleanupEmptyDirs removes empty directories up to the base path
func (d *driver) cleanupEmptyDirs(dir string) {
	for dir != d.config.BasePath && strings.HasPrefix(dir, d.config.BasePath) {
		// Try to remove directory (will fail if not empty)
		if err := os.Remove(dir); err != nil {
			// Directory not empty or other error, stop cleanup
			break
		}
		// Move to parent directory
		dir = filepath.Dir(dir)
	}
}

// calculateChecksum computes MD5 checksum of data
func calculateChecksum(data []byte) string {
	//nolint:gosec // MD5 is used for checksum validation, not security
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:])
}
