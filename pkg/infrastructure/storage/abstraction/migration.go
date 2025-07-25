package abstraction

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// MigrationConfig contains configuration for storage migration
type MigrationConfig struct {
	// BatchSize is the number of keys to migrate in each batch
	BatchSize int

	// Parallelism is the number of concurrent migration workers
	Parallelism int

	// DeleteAfterMigration removes data from source after successful migration
	DeleteAfterMigration bool

	// VerifyMigration verifies data integrity after migration
	VerifyMigration bool

	// ContinueOnError continues migration even if some items fail
	ContinueOnError bool

	// ProgressCallback is called periodically with migration progress
	ProgressCallback func(progress MigrationProgress)
}

// MigrationProgress represents the current state of a migration
type MigrationProgress struct {
	TotalKeys      int64
	MigratedKeys   int64
	FailedKeys     int64
	BytesMigrated  int64
	StartTime      time.Time
	CurrentKey     string
	EstimatedTime  time.Duration
	CompletedBatch int
	TotalBatches   int
}

// MigrationResult contains the final result of a migration
type MigrationResult struct {
	TotalKeys     int64
	MigratedKeys  int64
	FailedKeys    int64
	BytesMigrated int64
	Duration      time.Duration
	Errors        []error
}

// Migrator handles migration between storage backends
type Migrator struct {
	source Driver
	target Driver
	config MigrationConfig
	logger logrus.FieldLogger

	// Progress tracking
	totalKeys     int64
	migratedKeys  int64
	failedKeys    int64
	bytesMigrated int64
	errors        []error
	errorsMu      sync.Mutex
}

// NewMigrator creates a new storage migrator
func NewMigrator(source, target Driver, config MigrationConfig, logger logrus.FieldLogger) *Migrator {
	// Set defaults
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.Parallelism <= 0 {
		config.Parallelism = 4
	}

	return &Migrator{
		source: source,
		target: target,
		config: config,
		logger: logger.WithField("component", "migrator"),
	}
}

// Migrate performs the migration from source to target
func (m *Migrator) Migrate(ctx context.Context, prefix string) (*MigrationResult, error) {
	startTime := time.Now()

	m.logger.WithFields(logrus.Fields{
		"prefix":      prefix,
		"batch_size":  m.config.BatchSize,
		"parallelism": m.config.Parallelism,
	}).Info("Starting storage migration")

	// List all keys to migrate
	keys, err := m.source.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list source keys: %w", err)
	}

	atomic.StoreInt64(&m.totalKeys, int64(len(keys)))

	if len(keys) == 0 {
		m.logger.Info("No keys to migrate")
		return &MigrationResult{
			Duration: time.Since(startTime),
		}, nil
	}

	// Create batches
	batches := m.createBatches(keys)
	totalBatches := len(batches)

	// Create worker pool
	workCh := make(chan []string, len(batches))
	for _, batch := range batches {
		workCh <- batch
	}
	close(workCh)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < m.config.Parallelism; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			m.worker(ctx, workerID, workCh, totalBatches)
		}(i)
	}

	// Progress reporter
	if m.config.ProgressCallback != nil {
		progressCtx, cancelProgress := context.WithCancel(ctx)
		defer cancelProgress()
		go m.reportProgress(progressCtx, startTime, totalBatches)
	}

	// Wait for completion
	wg.Wait()

	// Final result
	result := &MigrationResult{
		TotalKeys:     atomic.LoadInt64(&m.totalKeys),
		MigratedKeys:  atomic.LoadInt64(&m.migratedKeys),
		FailedKeys:    atomic.LoadInt64(&m.failedKeys),
		BytesMigrated: atomic.LoadInt64(&m.bytesMigrated),
		Duration:      time.Since(startTime),
		Errors:        m.errors,
	}

	m.logger.WithFields(logrus.Fields{
		"total_keys":     result.TotalKeys,
		"migrated_keys":  result.MigratedKeys,
		"failed_keys":    result.FailedKeys,
		"bytes_migrated": result.BytesMigrated,
		"duration":       result.Duration,
	}).Info("Migration completed")

	return result, nil
}

// createBatches divides keys into batches
func (m *Migrator) createBatches(keys []string) [][]string {
	var batches [][]string
	batchSize := m.config.BatchSize

	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}
		batches = append(batches, keys[i:end])
	}

	return batches
}

// worker processes migration batches
func (m *Migrator) worker(ctx context.Context, workerID int, workCh <-chan []string, totalBatches int) {
	logger := m.logger.WithField("worker", workerID)

	for batch := range workCh {
		select {
		case <-ctx.Done():
			logger.Warn("Worker cancelled")
			return
		default:
		}

		for _, key := range batch {
			if err := m.migrateKey(ctx, key); err != nil {
				atomic.AddInt64(&m.failedKeys, 1)
				m.addError(fmt.Errorf("failed to migrate key %s: %w", key, err))

				if !m.config.ContinueOnError {
					logger.WithError(err).Error("Migration failed, stopping")
					return
				}

				logger.WithError(err).WithField("key", key).Warn("Failed to migrate key")
			} else {
				atomic.AddInt64(&m.migratedKeys, 1)
			}
		}
	}
}

// migrateKey migrates a single key
func (m *Migrator) migrateKey(ctx context.Context, key string) error {
	// Get data from source
	data, err := m.source.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to read from source: %w", err)
	}

	// Put data to target
	if err := m.target.Put(ctx, key, data); err != nil {
		return fmt.Errorf("failed to write to target: %w", err)
	}

	atomic.AddInt64(&m.bytesMigrated, int64(len(data)))

	// Verify if requested
	if m.config.VerifyMigration {
		targetData, err := m.target.Get(ctx, key)
		if err != nil {
			return fmt.Errorf("failed to verify target data: %w", err)
		}

		if len(data) != len(targetData) {
			return fmt.Errorf("data size mismatch: source=%d, target=%d", len(data), len(targetData))
		}

		// Simple byte comparison - in production, consider using checksums
		for i := range data {
			if data[i] != targetData[i] {
				return fmt.Errorf("data content mismatch at byte %d", i)
			}
		}
	}

	// Delete from source if requested
	if m.config.DeleteAfterMigration {
		if err := m.source.Delete(ctx, key); err != nil {
			// Log but don't fail the migration
			m.logger.WithError(err).WithField("key", key).Warn("Failed to delete from source")
		}
	}

	return nil
}

// addError safely adds an error to the list
func (m *Migrator) addError(err error) {
	m.errorsMu.Lock()
	defer m.errorsMu.Unlock()
	m.errors = append(m.errors, err)
}

// reportProgress periodically reports migration progress
func (m *Migrator) reportProgress(ctx context.Context, startTime time.Time, totalBatches int) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			migrated := atomic.LoadInt64(&m.migratedKeys)
			total := atomic.LoadInt64(&m.totalKeys)
			failed := atomic.LoadInt64(&m.failedKeys)
			bytes := atomic.LoadInt64(&m.bytesMigrated)

			progress := MigrationProgress{
				TotalKeys:     total,
				MigratedKeys:  migrated,
				FailedKeys:    failed,
				BytesMigrated: bytes,
				StartTime:     startTime,
			}

			// Estimate completion time
			if migrated > 0 {
				elapsed := time.Since(startTime)
				perKey := elapsed / time.Duration(migrated)
				remaining := total - migrated - failed
				progress.EstimatedTime = perKey * time.Duration(remaining)
			}

			m.config.ProgressCallback(progress)
		}
	}
}

// MigratePrefix is a convenience function to migrate all keys with a specific prefix
func MigratePrefix(ctx context.Context, source, target Driver, prefix string, config MigrationConfig, logger logrus.FieldLogger) (*MigrationResult, error) {
	migrator := NewMigrator(source, target, config, logger)
	return migrator.Migrate(ctx, prefix)
}

// MigrateAll is a convenience function to migrate all data
func MigrateAll(ctx context.Context, source, target Driver, config MigrationConfig, logger logrus.FieldLogger) (*MigrationResult, error) {
	migrator := NewMigrator(source, target, config, logger)
	return migrator.Migrate(ctx, "")
}
