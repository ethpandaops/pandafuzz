package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/ethpandaops/pandafuzz/pkg/config"
	"github.com/ethpandaops/pandafuzz/pkg/storage/backend"
	"github.com/ethpandaops/pandafuzz/pkg/storage/migration"
)

func main() {
	var (
		sourceConfig = flag.String("source", "", "Source storage config file (YAML)")
		destConfig   = flag.String("dest", "", "Destination storage config file (YAML)")
		prefix       = flag.String("prefix", "", "Prefix to migrate (empty for all)")
		dryRun       = flag.Bool("dry-run", false, "Perform dry run without migrating")
		parallel     = flag.Int("parallel", 8, "Number of parallel workers")
		deleteSource = flag.Bool("delete-source", false, "Delete files from source after migration")
		verify       = flag.Bool("verify", true, "Verify checksums after migration")
		verifySize   = flag.Bool("verify-size", true, "Verify file sizes after migration")
		retries      = flag.Int("retries", 3, "Maximum retry attempts for failed transfers")
		batchSize    = flag.Int("batch-size", 100, "Number of files to process in each batch")
		resumeKey    = flag.String("resume", "", "Resume migration from specific key")
		noConfirm    = flag.Bool("yes", false, "Skip confirmation prompts")
		verbose      = flag.Bool("verbose", false, "Enable verbose logging")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "PandaFuzz Corpus Migration Tool\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "This tool migrates corpus files between different storage backends.\n")
		fmt.Fprintf(os.Stderr, "It supports migrating between filesystem, S3, and MinIO storage.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Dry run migration from filesystem to S3\n")
		fmt.Fprintf(os.Stderr, "  %s -source fs.yaml -dest s3.yaml -dry-run\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Migrate all corpus files from filesystem to MinIO\n")
		fmt.Fprintf(os.Stderr, "  %s -source fs.yaml -dest minio.yaml\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Migrate specific campaign with cleanup\n")
		fmt.Fprintf(os.Stderr, "  %s -source fs.yaml -dest s3.yaml -prefix corpus/campaign123 -delete-source\n\n", os.Args[0])
	}

	flag.Parse()

	if *sourceConfig == "" || *destConfig == "" {
		flag.Usage()
		log.Fatal("Both -source and -dest config files are required")
	}

	// Setup logger
	logger := logrus.New()
	if *verbose {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Load configurations
	logger.Info("Loading storage configurations")
	srcCfg, err := loadStorageConfig(*sourceConfig)
	if err != nil {
		log.Fatalf("Failed to load source config: %v", err)
	}
	logger.WithField("type", srcCfg.Type).Info("Loaded source configuration")

	dstCfg, err := loadStorageConfig(*destConfig)
	if err != nil {
		log.Fatalf("Failed to load destination config: %v", err)
	}
	logger.WithField("type", dstCfg.Type).Info("Loaded destination configuration")

	// Create storage backends
	logger.Info("Initializing storage backends")
	sourceBackend, err := backend.NewStorageBackend(*srcCfg, logger)
	if err != nil {
		log.Fatalf("Failed to create source backend: %v", err)
	}

	destBackend, err := backend.NewStorageBackend(*dstCfg, logger)
	if err != nil {
		log.Fatalf("Failed to create destination backend: %v", err)
	}

	// Health check backends
	ctx := context.Background()
	logger.Info("Performing health checks")

	if err := sourceBackend.HealthCheck(ctx); err != nil {
		log.Fatalf("Source backend health check failed: %v", err)
	}
	logger.Info("Source backend is healthy")

	if err := destBackend.HealthCheck(ctx); err != nil {
		log.Fatalf("Destination backend health check failed: %v", err)
	}
	logger.Info("Destination backend is healthy")

	// Create progress bar with ETA
	var lastProgress time.Time
	var progressMutex sync.Mutex
	progressHandler := func(current, total int64, file string, eta time.Time) {
		progressMutex.Lock()
		defer progressMutex.Unlock()

		// Update progress at most once per 100ms for smoother updates
		if time.Since(lastProgress) < 100*time.Millisecond && current != total {
			return
		}
		lastProgress = time.Now()

		percent := float64(current) / float64(total) * 100

		// Clear the line and write progress
		fmt.Printf("\r\033[K") // Clear line

		if !eta.IsZero() {
			remaining := time.Until(eta)
			fmt.Printf("Progress: %d/%d (%.1f%%) - ETA: %s - %s",
				current, total, percent, formatDuration(remaining), truncatePath(file, 50))
		} else {
			fmt.Printf("Progress: %d/%d (%.1f%%) - %s",
				current, total, percent, truncatePath(file, 50))
		}

		if current == total {
			fmt.Println() // New line when complete
		}
	}

	// Confirmation callback
	confirmCallback := func(prompt string) bool {
		if *noConfirm {
			return true
		}

		fmt.Printf("\n%s [y/N]: ", prompt)
		var response string
		fmt.Scanln(&response)
		return strings.ToLower(response) == "y"
	}

	// Create migrator
	opts := migration.MigrationOptions{
		DryRun:          *dryRun,
		Parallel:        *parallel,
		DeleteSource:    *deleteSource,
		VerifyChecksum:  *verify,
		VerifySize:      *verifySize,
		MaxRetries:      *retries,
		RetryDelay:      2 * time.Second,
		BatchSize:       *batchSize,
		ResumeFromKey:   *resumeKey,
		ProgressHandler: progressHandler,
		ConfirmCallback: confirmCallback,
	}

	migrator := migration.NewMigrator(sourceBackend, destBackend, opts, logger)

	// Perform migration
	startTime := time.Now()

	fmt.Printf("\nStarting migration from %s to %s\n", srcCfg.Type, dstCfg.Type)
	if *prefix != "" {
		fmt.Printf("Migrating prefix: %s\n", *prefix)
	} else {
		fmt.Printf("Migrating all files\n")
	}
	if *dryRun {
		fmt.Println("DRY RUN MODE - No files will be migrated")
	}
	if *deleteSource {
		fmt.Println("WARNING: Source files will be deleted after successful migration")
	}
	fmt.Println()

	result, err := migrator.Migrate(ctx, *prefix)
	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	duration := time.Since(startTime)

	// Print results
	fmt.Printf("\n\nMigration completed in %v\n", duration)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Total files:  %d\n", result.TotalFiles)
	fmt.Printf("Total size:   %.2f GB\n", float64(result.TotalSize)/(1024*1024*1024))
	fmt.Printf("Migrated:     %d\n", result.SuccessCount)
	fmt.Printf("Failed:       %d\n", result.FailureCount)
	fmt.Printf("Skipped:      %d (already existed)\n", result.SkippedCount)

	if result.TotalFiles > 0 {
		throughput := float64(result.TotalSize) / duration.Seconds() / (1024 * 1024)
		fmt.Printf("Throughput:   %.2f MB/s\n", throughput)
	}

	// Print failed files if any
	if result.FailureCount > 0 && len(result.FailedFiles) > 0 {
		fmt.Println("\nFailed files:")
		for i, file := range result.FailedFiles {
			fmt.Printf("  %d. %s\n", i+1, file)
			if i >= 9 && result.FailureCount > 10 {
				fmt.Printf("  ... and %d more\n", result.FailureCount-10)
				break
			}
		}
	}

	// Exit with error code if failures occurred
	if result.FailureCount > 0 {
		os.Exit(1)
	}
}

// loadStorageConfig loads a storage configuration from a YAML file
func loadStorageConfig(path string) (*config.StorageConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg config.StorageConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < 0 {
		return "calculating..."
	}

	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	} else if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// truncatePath truncates a path to fit within maxLen characters
func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}

	// Try to keep the filename visible
	parts := strings.Split(path, "/")
	if len(parts) > 1 {
		filename := parts[len(parts)-1]
		if len(filename) < maxLen-5 {
			prefix := path[:maxLen-len(filename)-5]
			return prefix + ".../" + filename
		}
	}

	return path[:maxLen-3] + "..."
}

// Example configuration files that should be created by users:
//
// filesystem.yaml:
// type: filesystem
// filesystem:
//   base_path: /var/pandafuzz/corpus
//
// minio.yaml:
// type: minio
// minio:
//   endpoint: localhost:9000
//   access_key_id: pandafuzz
//   secret_access_key: pandafuzz123
//   corpus_bucket: corpus
//   use_ssl: false
//   use_path_style: true
//
// s3.yaml:
// type: s3
// s3:
//   region: us-east-1
//   corpus_bucket: pandafuzz-corpus-prod
//   use_ssl: true
