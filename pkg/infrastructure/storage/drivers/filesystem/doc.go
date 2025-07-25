// Package filesystem provides a storage driver implementation using the local filesystem.
//
// The filesystem driver stores data as files on disk, organizing them using the key as
// the file path relative to a configured base directory. It provides atomic writes,
// automatic directory management, and safe concurrent access.
//
// # Features
//
//   - Atomic file writes using temporary files and rename operations
//   - Automatic directory creation and cleanup
//   - Thread-safe concurrent access
//   - Key validation to prevent directory traversal attacks
//   - Support for hierarchical key structures using forward slashes
//
// # Configuration
//
// The driver is configured using the Config struct:
//
//	config := filesystem.Config{
//	    BasePath: "/var/lib/pandafuzz/storage",
//	    FileMode: 0644,
//	    DirMode:  0755,
//	    Config: storage.Config{
//	        MaxKeyLength: 1024,
//	        MaxValueSize: 100 * 1024 * 1024, // 100MB
//	    },
//	}
//
// # Key Format
//
// Keys are interpreted as relative file paths under the base directory.
// Forward slashes in keys are converted to the appropriate path separator
// for the operating system. Keys must not:
//   - Be empty
//   - Start with a path separator (/ or \)
//   - Contain ".." sequences
//   - Exceed the configured maximum length
//
// # Example Usage
//
//	// Create a new filesystem driver
//	driver, err := filesystem.NewDriver(config, logger)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Store data
//	err = driver.Put(ctx, "users/123/profile.json", profileData)
//
//	// Retrieve data
//	data, err := driver.Get(ctx, "users/123/profile.json")
//
//	// List all user profiles
//	keys, err := driver.List(ctx, "users/")
//
// # Thread Safety
//
// All operations are thread-safe and can be called concurrently.
// The driver uses read-write mutexes to ensure safe concurrent access
// while maximizing read performance.
//
// # Error Handling
//
// The driver returns storage.ErrNotFound when attempting to retrieve
// non-existent keys, and storage.ErrInvalidKey for invalid key formats.
// All other errors are wrapped with context about the operation that failed.
package filesystem
