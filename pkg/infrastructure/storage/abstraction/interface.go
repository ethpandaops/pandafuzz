// Package abstraction provides the core storage abstraction layer.
// It defines interfaces and common types for all storage implementations.
package abstraction

import (
	"context"
	"errors"
)

// Common errors returned by storage drivers
var (
	// ErrNotFound is returned when a requested key does not exist
	ErrNotFound = errors.New("key not found")

	// ErrInvalidKey is returned when a key contains invalid characters
	ErrInvalidKey = errors.New("invalid key")

	// ErrStorageUnavailable is returned when the storage backend is unavailable
	ErrStorageUnavailable = errors.New("storage backend unavailable")
)

// Driver defines the interface for storage backends.
// All operations are context-aware for cancellation and timeout support.
type Driver interface {
	// Put stores data with the given key.
	// It overwrites any existing data with the same key.
	Put(ctx context.Context, key string, data []byte) error

	// Get retrieves data for the given key.
	// Returns ErrNotFound if the key does not exist.
	Get(ctx context.Context, key string) ([]byte, error)

	// Delete removes data for the given key.
	// It is not an error if the key does not exist.
	Delete(ctx context.Context, key string) error

	// List returns all keys with the given prefix.
	// If prefix is empty, all keys are returned.
	// Keys are returned in lexicographical order.
	List(ctx context.Context, prefix string) ([]string, error)

	// Exists checks if a key exists in storage.
	Exists(ctx context.Context, key string) (bool, error)

	// GetURL returns a URL for accessing the stored data.
	// For filesystem storage, this returns a file:// URL.
	// For S3 storage, this returns a presigned URL valid for a reasonable duration.
	GetURL(ctx context.Context, key string) (string, error)
}

// Config provides common configuration options for storage drivers.
type Config struct {
	// MaxKeyLength is the maximum allowed length for keys
	MaxKeyLength int

	// MaxValueSize is the maximum allowed size for values in bytes
	MaxValueSize int64

	// EnableCompression enables transparent compression of stored data
	EnableCompression bool
}

// DefaultConfig returns default configuration values.
func DefaultConfig() Config {
	return Config{
		MaxKeyLength:      1024,
		MaxValueSize:      100 * 1024 * 1024, // 100MB
		EnableCompression: false,
	}
}
