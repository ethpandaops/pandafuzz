package migration

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/storage/backend"
)

// MigrationOptions configures the migration behavior
type MigrationOptions struct {
	DryRun          bool                                                          // Perform dry run without migrating
	Parallel        int                                                           // Number of parallel workers
	DeleteSource    bool                                                          // Delete files from source after migration
	VerifyChecksum  bool                                                          // Verify checksums after migration
	VerifySize      bool                                                          // Verify file sizes after migration
	MaxRetries      int                                                           // Maximum retry attempts for failed transfers
	RetryDelay      time.Duration                                                 // Delay between retry attempts
	BatchSize       int                                                           // Number of files to process in each batch
	ProgressHandler func(current, total int64, currentFile string, eta time.Time) // Progress callback
	ResumeFromKey   string                                                        // Resume migration from specific key
	ConfirmCallback func(prompt string) bool                                      // Callback for user confirmation
}

// Migrator handles migration between storage backends
type Migrator struct {
	source      backend.StorageBackend
	destination backend.StorageBackend
	logger      logrus.FieldLogger
	options     MigrationOptions

	// Runtime state
	startTime     time.Time
	processedSize int64
	cancelFunc    context.CancelFunc
}

// NewMigrator creates a new storage migrator
func NewMigrator(source, dest backend.StorageBackend, opts MigrationOptions, logger logrus.FieldLogger) *Migrator {
	if opts.Parallel <= 0 {
		opts.Parallel = 4
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = 3
	}
	if opts.RetryDelay <= 0 {
		opts.RetryDelay = 2 * time.Second
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 100
	}

	return &Migrator{
		source:      source,
		destination: dest,
		logger:      logger,
		options:     opts,
	}
}

// Migrate performs the migration from source to destination
func (m *Migrator) Migrate(ctx context.Context, prefix string) (*MigrationResult, error) {
	// Setup cancellation context and signal handling
	ctx, cancel := context.WithCancel(ctx)
	m.cancelFunc = cancel
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		m.logger.Warn("Migration interrupted by signal, cancelling...")
		cancel()
	}()

	// List all objects to migrate
	m.logger.WithField("prefix", prefix).Info("Listing source objects")
	objects, err := m.source.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list source objects: %w", err)
	}

	// Filter objects if resuming
	if m.options.ResumeFromKey != "" {
		objects = m.filterObjectsForResume(objects, m.options.ResumeFromKey)
		m.logger.WithFields(logrus.Fields{
			"resume_key": m.options.ResumeFromKey,
			"remaining":  len(objects),
		}).Info("Resuming migration from checkpoint")
	}

	result := &MigrationResult{
		TotalFiles:   len(objects),
		TotalSize:    0,
		SuccessCount: 0,
		FailureCount: 0,
		SkippedCount: 0,
		FailedFiles:  make([]string, 0),
		StartTime:    time.Now(),
	}

	// Calculate total size
	for _, obj := range objects {
		result.TotalSize += obj.Size
	}

	m.logger.WithFields(logrus.Fields{
		"files":      result.TotalFiles,
		"total_size": result.TotalSize,
		"dry_run":    m.options.DryRun,
	}).Info("Starting migration")

	// Confirm destructive operations
	if m.options.DeleteSource && !m.options.DryRun && m.options.ConfirmCallback != nil {
		prompt := fmt.Sprintf("WARNING: This will delete %d files from the source after migration. Continue?", result.TotalFiles)
		if !m.options.ConfirmCallback(prompt) {
			return nil, fmt.Errorf("migration cancelled by user")
		}
	}

	if m.options.DryRun {
		m.logger.Info("Dry run - no files will be migrated")
		result.SuccessCount = result.TotalFiles // Simulate success
		return result, nil
	}

	// Initialize runtime state
	m.startTime = time.Now()

	// Create worker pool
	sem := make(chan struct{}, m.options.Parallel)
	var wg sync.WaitGroup
	var successCount, failureCount, skippedCount int64
	var processedSize int64
	var failedFilesMu sync.Mutex

	// Process in batches for better memory management
	for batchStart := 0; batchStart < len(objects); batchStart += m.options.BatchSize {
		batchEnd := batchStart + m.options.BatchSize
		if batchEnd > len(objects) {
			batchEnd = len(objects)
		}

		batch := objects[batchStart:batchEnd]

		for i, obj := range batch {
			select {
			case <-ctx.Done():
				// Wait for in-progress operations to complete
				wg.Wait()
				result.SuccessCount = int(successCount)
				result.FailureCount = int(failureCount)
				result.SkippedCount = int(skippedCount)
				result.EndTime = time.Now()
				return result, ctx.Err()
			case sem <- struct{}{}: // Acquire semaphore
				wg.Add(1)

				go func(idx int, object backend.ObjectInfo) {
					defer wg.Done()
					defer func() { <-sem }() // Release semaphore

					// Update processed size for ETA calculation
					atomic.AddInt64(&processedSize, object.Size)
					m.processedSize = atomic.LoadInt64(&processedSize)

					// Report progress with ETA
					if m.options.ProgressHandler != nil {
						eta := m.calculateETA(atomic.LoadInt64(&processedSize), result.TotalSize)
						m.options.ProgressHandler(int64(batchStart+idx+1), int64(len(objects)), object.Key, eta)
					}

					// Check if already exists
					exists, err := m.destination.Exists(ctx, object.Key)
					if err != nil {
						m.logger.WithError(err).WithField("key", object.Key).Error("Failed to check existence")
						atomic.AddInt64(&failureCount, 1)
						failedFilesMu.Lock()
						result.FailedFiles = append(result.FailedFiles, object.Key)
						failedFilesMu.Unlock()
						return
					}

					if exists && !m.shouldOverwrite(ctx, object) {
						m.logger.WithField("key", object.Key).Debug("Object already exists, skipping")
						atomic.AddInt64(&skippedCount, 1)
						return
					}

					// Migrate the object with retry
					err = m.migrateObjectWithRetry(ctx, object)
					if err != nil {
						m.logger.WithError(err).WithField("key", object.Key).Error("Failed to migrate object after retries")
						atomic.AddInt64(&failureCount, 1)
						failedFilesMu.Lock()
						result.FailedFiles = append(result.FailedFiles, object.Key)
						failedFilesMu.Unlock()
						return
					}

					atomic.AddInt64(&successCount, 1)

					// Delete from source if requested
					if m.options.DeleteSource {
						if err := m.source.Delete(ctx, object.Key); err != nil {
							m.logger.WithError(err).WithField("key", object.Key).Warn("Failed to delete source object")
						}
					}
				}(i, obj)
			}
		}
	}

	wg.Wait()

	result.SuccessCount = int(successCount)
	result.FailureCount = int(failureCount)
	result.SkippedCount = int(skippedCount)
	result.EndTime = time.Now()

	m.logger.WithFields(logrus.Fields{
		"success":  result.SuccessCount,
		"failed":   result.FailureCount,
		"skipped":  result.SkippedCount,
		"duration": result.EndTime.Sub(result.StartTime),
	}).Info("Migration completed")

	return result, nil
}

// migrateObject handles the migration of a single object
func (m *Migrator) migrateObject(ctx context.Context, obj backend.ObjectInfo) error {
	// Retrieve from source
	reader, err := m.source.Retrieve(ctx, obj.Key)
	if err != nil {
		return fmt.Errorf("failed to retrieve from source: %w", err)
	}
	defer reader.Close()

	// Create a tee reader to calculate checksum if needed
	var finalReader io.Reader = reader
	var sourceChecksum string

	if m.options.VerifyChecksum {
		hasher := md5.New()
		teeReader := io.TeeReader(reader, hasher)
		finalReader = teeReader

		// Store to destination while calculating checksum
		if err := m.destination.Store(ctx, obj.Key, finalReader, obj.Size); err != nil {
			return fmt.Errorf("failed to store to destination: %w", err)
		}

		sourceChecksum = hex.EncodeToString(hasher.Sum(nil))
	} else {
		// Store to destination without checksum
		if err := m.destination.Store(ctx, obj.Key, finalReader, obj.Size); err != nil {
			return fmt.Errorf("failed to store to destination: %w", err)
		}
	}

	// Copy metadata
	metadata, err := m.source.GetMetadata(ctx, obj.Key)
	if err == nil && metadata.UserMetadata != nil && len(metadata.UserMetadata) > 0 {
		if err := m.destination.SetMetadata(ctx, obj.Key, metadata.UserMetadata); err != nil {
			m.logger.WithError(err).WithField("key", obj.Key).Warn("Failed to copy metadata")
		}
	}

	// Verify size if requested
	if m.options.VerifySize {
		destMeta, err := m.destination.GetMetadata(ctx, obj.Key)
		if err != nil {
			return fmt.Errorf("failed to get destination metadata for verification: %w", err)
		}
		if destMeta.Size != obj.Size {
			return fmt.Errorf("size mismatch: source=%d, destination=%d", obj.Size, destMeta.Size)
		}
	}

	// Verify checksum if requested
	if m.options.VerifyChecksum && sourceChecksum != "" {
		// Retrieve from destination to calculate checksum
		destReader, err := m.destination.Retrieve(ctx, obj.Key)
		if err != nil {
			return fmt.Errorf("failed to retrieve destination for checksum verification: %w", err)
		}
		defer destReader.Close()

		destHasher := md5.New()
		if _, err := io.Copy(destHasher, destReader); err != nil {
			return fmt.Errorf("failed to calculate destination checksum: %w", err)
		}

		destChecksum := hex.EncodeToString(destHasher.Sum(nil))
		if sourceChecksum != destChecksum {
			return fmt.Errorf("checksum mismatch: source=%s, destination=%s", sourceChecksum, destChecksum)
		}

		m.logger.WithFields(logrus.Fields{
			"key":      obj.Key,
			"checksum": sourceChecksum,
		}).Debug("Checksum verified")
	}

	m.logger.WithFields(logrus.Fields{
		"key":  obj.Key,
		"size": obj.Size,
	}).Debug("Migrated object successfully")

	return nil
}

// MigrationResult contains statistics about the migration
type MigrationResult struct {
	TotalFiles   int       // Total number of files to migrate
	TotalSize    int64     // Total size in bytes
	SuccessCount int       // Number of successfully migrated files
	FailureCount int       // Number of failed migrations
	SkippedCount int       // Number of skipped files (already existed)
	FailedFiles  []string  // List of files that failed to migrate
	StartTime    time.Time // Migration start time
	EndTime      time.Time // Migration end time
}

// Summary returns a human-readable summary of the migration
func (r *MigrationResult) Summary() string {
	duration := r.EndTime.Sub(r.StartTime)
	throughput := float64(r.TotalSize) / duration.Seconds() / (1024 * 1024)

	return fmt.Sprintf("Migration Summary: Total=%d, Success=%d, Failed=%d, Skipped=%d, Size=%.2f GB, Duration=%v, Throughput=%.2f MB/s",
		r.TotalFiles, r.SuccessCount, r.FailureCount, r.SkippedCount,
		float64(r.TotalSize)/(1024*1024*1024), duration, throughput)
}

// migrateObjectWithRetry attempts to migrate an object with retry logic
func (m *Migrator) migrateObjectWithRetry(ctx context.Context, obj backend.ObjectInfo) error {
	var lastErr error

	for attempt := 0; attempt <= m.options.MaxRetries; attempt++ {
		if attempt > 0 {
			m.logger.WithFields(logrus.Fields{
				"key":     obj.Key,
				"attempt": attempt,
			}).Debug("Retrying migration")

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(m.options.RetryDelay * time.Duration(attempt)):
				// Exponential backoff
			}
		}

		err := m.migrateObject(ctx, obj)
		if err == nil {
			return nil
		}

		lastErr = err
		m.logger.WithError(err).WithFields(logrus.Fields{
			"key":     obj.Key,
			"attempt": attempt,
		}).Warn("Migration attempt failed")
	}

	return fmt.Errorf("failed after %d attempts: %w", m.options.MaxRetries+1, lastErr)
}

// filterObjectsForResume filters objects to resume from a specific key
func (m *Migrator) filterObjectsForResume(objects []backend.ObjectInfo, resumeKey string) []backend.ObjectInfo {
	var filtered []backend.ObjectInfo
	found := false

	for _, obj := range objects {
		if obj.Key == resumeKey {
			found = true
		}
		if found {
			filtered = append(filtered, obj)
		}
	}

	return filtered
}

// shouldOverwrite determines if an existing object should be overwritten
func (m *Migrator) shouldOverwrite(ctx context.Context, obj backend.ObjectInfo) bool {
	// In this implementation, we never overwrite existing files
	// This could be extended to check timestamps, sizes, or checksums
	return false
}

// calculateETA estimates the time of completion
func (m *Migrator) calculateETA(processedSize, totalSize int64) time.Time {
	if processedSize == 0 || totalSize == 0 {
		return time.Time{}
	}

	elapsed := time.Since(m.startTime)
	rate := float64(processedSize) / elapsed.Seconds()
	remainingSize := totalSize - processedSize
	remainingTime := time.Duration(float64(remainingSize) / rate * float64(time.Second))

	return time.Now().Add(remainingTime)
}
