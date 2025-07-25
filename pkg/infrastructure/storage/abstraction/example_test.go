package abstraction_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/abstraction"
	_ "github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/drivers/all"
)

func ExampleFactory_middleware() {
	// Create logger
	logger := logrus.New()

	// Create factory
	factory := abstraction.NewFactory(logger)

	// Create driver with full middleware stack
	driver, err := factory.NewDriver(abstraction.FactoryConfig{
		Type: abstraction.TypeFilesystem,
		Config: &abstraction.FilesystemConfig{
			BasePath: "/tmp/pandafuzz",
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
				MaxDelay:     1 * time.Second,
				Multiplier:   2.0,
			},
			CacheConfig: abstraction.CacheConfig{
				MaxSize:    10 * 1024 * 1024, // 10MB
				TTL:        5 * time.Minute,
				MaxEntries: 100,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// Operations will now have logging, metrics, retry, and caching
	data := []byte("Important data with middleware")
	if err := driver.Put(ctx, "middleware-example.txt", data); err != nil {
		log.Fatal(err)
	}

	// First get will hit storage
	retrieved, err := driver.Get(ctx, "middleware-example.txt")
	if err != nil {
		log.Fatal(err)
	}

	// Second get will hit cache
	retrieved, err = driver.Get(ctx, "middleware-example.txt")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Retrieved: %s\n", retrieved)
}

func ExampleCompositeDriver() {
	// Create logger
	logger := logrus.New()

	// Create factory
	factory := abstraction.NewFactory(logger)

	// Create composite driver with primary and secondary storage
	driver, err := factory.NewDriver(abstraction.FactoryConfig{
		Type: abstraction.TypeComposite,
		Composite: &abstraction.CompositeConfig{
			Primary: abstraction.FactoryConfig{
				Type: abstraction.TypeFilesystem,
				Config: &abstraction.FilesystemConfig{
					BasePath: "/var/lib/pandafuzz/primary",
				},
			},
			Secondaries: []abstraction.FactoryConfig{
				{
					Type: abstraction.TypeFilesystem,
					Config: &abstraction.FilesystemConfig{
						BasePath: "/var/lib/pandafuzz/backup",
					},
				},
			},
			WriteMode: abstraction.WriteBestEffort,
			ReadMode:  abstraction.ReadFallback,
		},
		Middleware: abstraction.MiddlewareConfig{
			EnableLogging: true,
			EnableMetrics: true,
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// Write will go to both primary and secondary
	data := []byte("Replicated data")
	if err := driver.Put(ctx, "important/data.bin", data); err != nil {
		log.Fatal(err)
	}

	// Read will try primary first, fall back to secondary if needed
	retrieved, err := driver.Get(ctx, "important/data.bin")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Retrieved: %s\n", retrieved)
}

func ExampleMigrator() {
	// Create logger
	logger := logrus.New()

	// Create source driver (filesystem)
	source, err := abstraction.NewFilesystemDriver("/tmp/old-storage", logger)
	if err != nil {
		log.Fatal(err)
	}

	// Create target driver (S3)
	target, err := abstraction.NewS3Driver(
		"new-bucket",
		"us-west-2",
		"access-key",
		"secret-key",
		logger,
	)
	if err != nil {
		log.Fatal(err)
	}

	// Create migrator with progress tracking
	migrator := abstraction.NewMigrator(source, target, abstraction.MigrationConfig{
		BatchSize:            100,
		Parallelism:          4,
		VerifyMigration:      true,
		DeleteAfterMigration: false,
		ContinueOnError:      true,
		ProgressCallback: func(progress abstraction.MigrationProgress) {
			pct := float64(progress.MigratedKeys) / float64(progress.TotalKeys) * 100
			fmt.Printf("Migration progress: %.1f%% (%d/%d keys)\n",
				pct, progress.MigratedKeys, progress.TotalKeys)
		},
	}, logger)

	ctx := context.Background()

	// Migrate all data
	result, err := migrator.Migrate(ctx, "")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Migration completed: %d keys migrated, %d failed\n",
		result.MigratedKeys, result.FailedKeys)
}

func ExampleFactory_highAvailability() {
	// Create logger
	logger := logrus.New()

	// Create factory
	factory := abstraction.NewFactory(logger)

	// Create high-availability setup with S3 primary and filesystem backup
	driver, err := factory.NewDriver(abstraction.FactoryConfig{
		Type: abstraction.TypeComposite,
		Composite: &abstraction.CompositeConfig{
			Primary: abstraction.FactoryConfig{
				Type: abstraction.TypeS3,
				Config: &abstraction.S3Config{
					Bucket:          "pandafuzz-primary",
					Region:          "us-west-2",
					AccessKeyID:     "primary-access-key",
					SecretAccessKey: "primary-secret-key",
				},
				Middleware: abstraction.MiddlewareConfig{
					EnableRetry: true,
					RetryConfig: abstraction.RetryConfig{
						MaxAttempts: 5,
					},
				},
			},
			Secondaries: []abstraction.FactoryConfig{
				{
					Type: abstraction.TypeS3,
					Config: &abstraction.S3Config{
						Bucket:          "pandafuzz-secondary",
						Region:          "us-east-1",
						AccessKeyID:     "secondary-access-key",
						SecretAccessKey: "secondary-secret-key",
					},
				},
				{
					Type: abstraction.TypeFilesystem,
					Config: &abstraction.FilesystemConfig{
						BasePath: "/mnt/backup/pandafuzz",
					},
				},
			},
			WriteMode: abstraction.WriteBestEffort,
			ReadMode:  abstraction.ReadFallback,
		},
		Middleware: abstraction.MiddlewareConfig{
			EnableLogging: true,
			EnableMetrics: true,
			EnableCaching: true,
			CacheConfig: abstraction.CacheConfig{
				MaxSize: 500 * 1024 * 1024, // 500MB cache
				TTL:     10 * time.Minute,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// Data is written to all backends with best effort
	// Reads fall back through primary -> secondary S3 -> filesystem
	// Results are cached for performance
	data := []byte("Critical fuzzing corpus")
	if err := driver.Put(ctx, "corpus/seed-001.bin", data); err != nil {
		log.Fatal(err)
	}

	fmt.Println("High availability storage configured successfully")
}
