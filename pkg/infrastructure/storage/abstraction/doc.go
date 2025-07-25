// Package abstraction provides a comprehensive storage abstraction layer for the pandafuzz infrastructure.
// It offers a unified interface for different storage backends with extensive middleware support,
// composite storage patterns, and migration capabilities.
//
// # Core Components
//
// The abstraction layer consists of several key components:
//
// ## Interface
//
// The Driver interface defines the contract that all storage implementations must satisfy:
//
//	type Driver interface {
//	    Put(ctx context.Context, key string, data []byte) error
//	    Get(ctx context.Context, key string) ([]byte, error)
//	    Delete(ctx context.Context, key string) error
//	    List(ctx context.Context, prefix string) ([]string, error)
//	    Exists(ctx context.Context, key string) (bool, error)
//	    GetURL(ctx context.Context, key string) (string, error)
//	}
//
// ## Factory
//
// The Factory provides a flexible way to create storage drivers with middleware:
//
//	factory := abstraction.NewFactory(logger)
//	driver, err := factory.NewDriver(abstraction.FactoryConfig{
//	    Type: abstraction.TypeFilesystem,
//	    Filesystem: &filesystem.Config{
//	        BasePath: "/var/data",
//	    },
//	    Middleware: abstraction.MiddlewareConfig{
//	        EnableLogging: true,
//	        EnableMetrics: true,
//	        EnableRetry:   true,
//	    },
//	})
//
// ## Middleware
//
// The abstraction layer provides several middleware components that can be composed:
//
// - **LoggingMiddleware**: Logs all storage operations with timing information
// - **MetricsMiddleware**: Collects operation counts, error rates, and data transfer metrics
// - **RetryMiddleware**: Implements exponential backoff retry logic for transient failures
// - **CacheMiddleware**: Adds an in-memory cache layer with TTL and size limits
//
// Middleware is applied in a specific order to ensure proper behavior:
// Caching → Retry → Metrics → Logging
//
// ## Composite Storage
//
// Composite storage allows writing to multiple backends with different strategies:
//
//	composite, err := factory.NewDriver(abstraction.FactoryConfig{
//	    Type: abstraction.TypeComposite,
//	    Composite: &abstraction.CompositeConfig{
//	        Primary: primaryConfig,
//	        Secondaries: []abstraction.FactoryConfig{
//	            secondaryConfig1,
//	            secondaryConfig2,
//	        },
//	        WriteMode: abstraction.WriteBestEffort,
//	        ReadMode:  abstraction.ReadFallback,
//	    },
//	})
//
// Write modes:
// - **WriteAll**: Writes to all backends, fails if any fail (transactional)
// - **WritePrimaryFirst**: Writes to primary synchronously, secondaries asynchronously
// - **WriteBestEffort**: Writes to all backends but only requires primary to succeed
//
// Read modes:
// - **ReadPrimary**: Always reads from the primary backend
// - **ReadFallback**: Tries primary first, falls back to secondaries on failure
// - **ReadFastest**: Reads from all backends in parallel, returns first response
//
// ## Migration
//
// The migration system supports moving data between storage backends:
//
//	migrator := abstraction.NewMigrator(source, target, abstraction.MigrationConfig{
//	    BatchSize:            100,
//	    Parallelism:          4,
//	    VerifyMigration:      true,
//	    DeleteAfterMigration: false,
//	    ContinueOnError:      true,
//	    ProgressCallback: func(progress abstraction.MigrationProgress) {
//	        fmt.Printf("Progress: %d/%d keys migrated\n",
//	            progress.MigratedKeys, progress.TotalKeys)
//	    },
//	}, logger)
//
//	result, err := migrator.Migrate(ctx, "")
//
// # Usage Examples
//
// ## Basic Usage
//
//	// Create a simple filesystem driver
//	driver, err := abstraction.NewFilesystemDriver("/var/data", logger)
//
//	// Store data
//	err = driver.Put(ctx, "user/123", userData)
//
//	// Retrieve data
//	data, err := driver.Get(ctx, "user/123")
//
//	// List keys
//	keys, err := driver.List(ctx, "user/")
//
// ## Advanced Configuration
//
//	// Create S3 driver with full middleware stack
//	factory := abstraction.NewFactory(logger)
//	driver, err := factory.NewDriver(abstraction.FactoryConfig{
//	    Type: abstraction.TypeS3,
//	    S3: &s3.Config{
//	        Bucket: "my-bucket",
//	        Region: "us-east-1",
//	    },
//	    Middleware: abstraction.MiddlewareConfig{
//	        EnableLogging: true,
//	        EnableMetrics: true,
//	        EnableRetry: true,
//	        EnableCaching: true,
//	        RetryConfig: abstraction.RetryConfig{
//	            MaxAttempts: 5,
//	            InitialDelay: 200 * time.Millisecond,
//	        },
//	        CacheConfig: abstraction.CacheConfig{
//	            MaxSize: 500 * 1024 * 1024, // 500MB
//	            TTL: 10 * time.Minute,
//	        },
//	    },
//	})
//
// ## High Availability Setup
//
//	// Create a composite driver for high availability
//	driver, err := factory.NewDriver(abstraction.FactoryConfig{
//	    Type: abstraction.TypeComposite,
//	    Composite: &abstraction.CompositeConfig{
//	        Primary: abstraction.FactoryConfig{
//	            Type: abstraction.TypeS3,
//	            S3: primaryS3Config,
//	        },
//	        Secondaries: []abstraction.FactoryConfig{
//	            {
//	                Type: abstraction.TypeS3,
//	                S3: secondaryS3Config,
//	            },
//	            {
//	                Type: abstraction.TypeFilesystem,
//	                Filesystem: backupFSConfig,
//	            },
//	        },
//	        WriteMode: abstraction.WriteBestEffort,
//	        ReadMode: abstraction.ReadFallback,
//	    },
//	    Middleware: abstraction.DefaultMiddlewareConfig(),
//	})
//
// # Best Practices
//
// 1. **Always use context**: Pass context to all operations for proper cancellation
// 2. **Handle errors appropriately**: Check for ErrNotFound vs other errors
// 3. **Configure middleware thoughtfully**: More middleware means more overhead
// 4. **Monitor metrics**: Use MetricsMiddleware in production for observability
// 5. **Test migrations**: Always test migrations with VerifyMigration enabled
// 6. **Size your cache appropriately**: Consider memory constraints when enabling caching
// 7. **Use appropriate write modes**: Choose based on consistency vs performance needs
//
// # Thread Safety
//
// All drivers and middleware implementations are thread-safe and can be used
// concurrently from multiple goroutines.
package abstraction
