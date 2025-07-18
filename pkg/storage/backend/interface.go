// Package backend provides a unified interface for storing corpus files in different storage systems.
package backend

import (
	"context"
	"io"
	"time"
)

// StorageBackend defines the interface for corpus storage.
// All methods that perform I/O operations accept context.Context as the first parameter
// to support cancellation and timeouts.
type StorageBackend interface {
	// Store saves data to the storage backend.
	// The reader will be read until EOF or size bytes have been read.
	// If size is -1, the reader will be read until EOF.
	Store(ctx context.Context, key string, reader io.Reader, size int64) error

	// Retrieve returns a reader for the stored data.
	// The caller must close the returned reader when done.
	Retrieve(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the object from storage.
	Delete(ctx context.Context, key string) error

	// Exists checks if an object exists in storage.
	Exists(ctx context.Context, key string) (bool, error)

	// List returns all objects with the given prefix.
	// If prefix is empty, all objects are returned.
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)

	// DeleteMany removes multiple objects from storage.
	// Returns an error if any deletion fails, but continues attempting all deletions.
	DeleteMany(ctx context.Context, keys []string) error

	// GetMetadata retrieves metadata for the specified object.
	GetMetadata(ctx context.Context, key string) (*ObjectMetadata, error)

	// SetMetadata updates metadata for the specified object.
	// The metadata map is merged with existing metadata.
	SetMetadata(ctx context.Context, key string, metadata map[string]string) error

	// GetPresignedURL generates a presigned URL for downloading an object.
	// The URL will be valid for the specified duration.
	GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)

	// PutPresignedURL generates a presigned URL for uploading an object.
	// The URL will be valid for the specified duration.
	PutPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)

	// HealthCheck verifies the storage backend is accessible and functioning.
	HealthCheck(ctx context.Context) error
}

// ObjectInfo contains basic information about a stored object.
type ObjectInfo struct {
	Key          string    // Object key/path
	Size         int64     // Size in bytes
	LastModified time.Time // Last modification time
	ETag         string    // Entity tag for cache validation
}

// ObjectMetadata contains detailed metadata about a stored object.
type ObjectMetadata struct {
	ContentType  string            // MIME type of the object
	Size         int64             // Size in bytes
	ETag         string            // Entity tag for cache validation
	LastModified time.Time         // Last modification time
	UserMetadata map[string]string // Custom user-defined metadata
}
