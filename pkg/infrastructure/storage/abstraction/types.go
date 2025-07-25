package abstraction

import "time"

// Type represents the type of storage driver
type Type string

const (
	// TypeFilesystem represents filesystem storage
	TypeFilesystem Type = "filesystem"

	// TypeS3 represents S3-compatible storage
	TypeS3 Type = "s3"

	// TypeComposite represents composite storage (writes to multiple backends)
	TypeComposite Type = "composite"
)

// MiddlewareConfig contains configuration for middleware layers
type MiddlewareConfig struct {
	// EnableLogging enables request/response logging
	EnableLogging bool

	// EnableMetrics enables metrics collection
	EnableMetrics bool

	// EnableRetry enables automatic retry on failures
	EnableRetry bool

	// EnableCaching enables caching layer
	EnableCaching bool

	// RetryConfig contains retry configuration
	RetryConfig RetryConfig

	// CacheConfig contains cache configuration
	CacheConfig CacheConfig
}

// RetryConfig contains configuration for retry middleware
type RetryConfig struct {
	// MaxAttempts is the maximum number of retry attempts
	MaxAttempts int

	// InitialDelay is the initial delay between retries
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration

	// Multiplier is the backoff multiplier
	Multiplier float64
}

// CacheConfig contains configuration for cache middleware
type CacheConfig struct {
	// MaxSize is the maximum cache size in bytes
	MaxSize int64

	// TTL is the default cache TTL
	TTL time.Duration

	// MaxEntries is the maximum number of cache entries
	MaxEntries int
}

// CompositeConfig contains configuration for composite storage
type CompositeConfig struct {
	// Primary is the primary storage backend
	Primary FactoryConfig

	// Secondaries are additional storage backends
	Secondaries []FactoryConfig

	// WriteMode determines how writes are handled
	WriteMode CompositeWriteMode

	// ReadMode determines how reads are handled
	ReadMode CompositeReadMode
}

// CompositeWriteMode determines how composite storage handles writes
type CompositeWriteMode string

const (
	// WriteAll writes to all backends (fails if any fail)
	WriteAll CompositeWriteMode = "all"

	// WritePrimaryFirst writes to primary first, then secondaries asynchronously
	WritePrimaryFirst CompositeWriteMode = "primary_first"

	// WriteBestEffort writes to all backends but only requires primary to succeed
	WriteBestEffort CompositeWriteMode = "best_effort"
)

// CompositeReadMode determines how composite storage handles reads
type CompositeReadMode string

const (
	// ReadPrimary always reads from primary
	ReadPrimary CompositeReadMode = "primary"

	// ReadFallback reads from primary, falls back to secondaries on failure
	ReadFallback CompositeReadMode = "fallback"

	// ReadFastest reads from all backends and returns the fastest response
	ReadFastest CompositeReadMode = "fastest"
)
