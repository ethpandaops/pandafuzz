# Storage Infrastructure

This package provides a comprehensive storage abstraction layer for the pandafuzz infrastructure.

## Architecture

### Structure

```
storage/
├── abstraction/          # Core abstraction layer
│   ├── interface.go      # Driver interface definition
│   ├── factory.go        # Factory for creating drivers with middleware
│   ├── types.go          # Common types and configuration
│   ├── middleware.go     # Middleware implementations (logging, metrics, retry, cache)
│   ├── composite.go      # Composite driver for multi-backend storage
│   ├── migration.go      # Storage migration utilities
│   ├── helpers.go        # Convenience functions
│   └── doc.go            # Package documentation
├── drivers/              # Storage driver implementations
│   ├── filesystem/       # Local filesystem driver
│   ├── s3/               # S3-compatible storage driver
│   └── all/              # Import all drivers
├── interface.go          # Backward compatibility exports
└── factory.go            # Backward compatibility factories
```

## Usage

### Basic Usage

```go
import (
    "github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/abstraction"
    _ "github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/drivers/all"
)

// Create a simple filesystem driver
driver, err := abstraction.NewFilesystemDriver("/var/data", logger)

// Store data
err = driver.Put(ctx, "user/123", userData)

// Retrieve data
data, err := driver.Get(ctx, "user/123")
```

### Advanced Configuration with Middleware

```go
factory := abstraction.NewFactory(logger)
driver, err := factory.NewDriver(abstraction.FactoryConfig{
    Type: abstraction.TypeFilesystem,
    Config: &abstraction.FilesystemConfig{
        BasePath: "/var/data",
        FileMode: 0o644,
        DirMode:  0o755,
    },
    Middleware: abstraction.MiddlewareConfig{
        EnableLogging: true,
        EnableMetrics: true,
        EnableRetry:   true,
        EnableCaching: true,
        RetryConfig: abstraction.RetryConfig{
            MaxAttempts:  3,
            InitialDelay: 100 * time.Millisecond,
            MaxDelay:     5 * time.Second,
            Multiplier:   2.0,
        },
        CacheConfig: abstraction.CacheConfig{
            MaxSize:    100 * 1024 * 1024, // 100MB
            TTL:        5 * time.Minute,
            MaxEntries: 1000,
        },
    },
})
```

### Composite Storage for High Availability

```go
driver, err := factory.NewDriver(abstraction.FactoryConfig{
    Type: abstraction.TypeComposite,
    Composite: &abstraction.CompositeConfig{
        Primary: abstraction.FactoryConfig{
            Type: abstraction.TypeS3,
            Config: &abstraction.S3Config{
                Bucket: "primary-bucket",
                Region: "us-west-2",
            },
        },
        Secondaries: []abstraction.FactoryConfig{
            {
                Type: abstraction.TypeS3,
                Config: &abstraction.S3Config{
                    Bucket: "backup-bucket",
                    Region: "us-east-1",
                },
            },
        },
        WriteMode: abstraction.WriteBestEffort,
        ReadMode:  abstraction.ReadFallback,
    },
})
```

### Storage Migration

```go
migrator := abstraction.NewMigrator(source, target, abstraction.MigrationConfig{
    BatchSize:            100,
    Parallelism:          4,
    VerifyMigration:      true,
    DeleteAfterMigration: false,
    ContinueOnError:      true,
    ProgressCallback: func(progress abstraction.MigrationProgress) {
        fmt.Printf("Progress: %d/%d keys migrated\n",
            progress.MigratedKeys, progress.TotalKeys)
    },
}, logger)

result, err := migrator.Migrate(ctx, "")
```

## Features

### Middleware

1. **Logging Middleware**: Logs all storage operations with timing information
2. **Metrics Middleware**: Collects operation counts, error rates, and data transfer metrics
3. **Retry Middleware**: Implements exponential backoff retry logic for transient failures
4. **Cache Middleware**: Adds an in-memory cache layer with TTL and size limits

### Composite Storage

- **Write Modes**:
  - `WriteAll`: Writes to all backends, fails if any fail (transactional)
  - `WritePrimaryFirst`: Writes to primary synchronously, secondaries asynchronously
  - `WriteBestEffort`: Writes to all backends but only requires primary to succeed

- **Read Modes**:
  - `ReadPrimary`: Always reads from the primary backend
  - `ReadFallback`: Tries primary first, falls back to secondaries on failure
  - `ReadFastest`: Reads from all backends in parallel, returns first response

### Migration

- Batch processing for efficient large-scale migrations
- Parallel workers for improved performance
- Optional data verification
- Progress tracking and reporting
- Configurable error handling (stop on error or continue)

## Production Considerations

1. **Always use context**: Pass context to all operations for proper cancellation
2. **Handle errors appropriately**: Check for `ErrNotFound` vs other errors
3. **Configure middleware thoughtfully**: More middleware means more overhead
4. **Monitor metrics**: Use MetricsMiddleware in production for observability
5. **Test migrations**: Always test migrations with `VerifyMigration` enabled
6. **Size your cache appropriately**: Consider memory constraints when enabling caching
7. **Use appropriate write modes**: Choose based on consistency vs performance needs

## Thread Safety

All drivers and middleware implementations are thread-safe and can be used concurrently from multiple goroutines.