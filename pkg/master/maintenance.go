package master

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// MaintenanceService handles database and storage maintenance operations
type MaintenanceService struct {
	db      common.AdvancedDatabase
	config  *common.MasterConfig
	logger  *logrus.Logger
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	ticker  *time.Ticker
	running bool
	stats   MaintenanceStats
}

// MaintenanceStats tracks maintenance operation statistics
type MaintenanceStats struct {
	LastRun        time.Time     `json:"last_run"`
	VacuumRuns     int64         `json:"vacuum_runs"`
	CrashesPurged  int64         `json:"crashes_purged"`
	JobsPurged     int64         `json:"jobs_purged"`
	BytesReclaimed int64         `json:"bytes_reclaimed"`
	Errors         int64         `json:"errors"`
	LastVacuumSize int64         `json:"last_vacuum_size"`
	LastVacuumTime time.Duration `json:"last_vacuum_time"`
}

// NewMaintenanceService creates a new maintenance service
func NewMaintenanceService(db common.AdvancedDatabase, config *common.MasterConfig, logger *logrus.Logger) *MaintenanceService {
	ctx, cancel := context.WithCancel(context.Background())

	return &MaintenanceService{
		db:     db,
		config: config,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}
}

// RunMaintenance performs all maintenance operations
func (s *MaintenanceService) RunMaintenance(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return common.NewSystemError("run_maintenance", fmt.Errorf("maintenance already running"))
	}
	s.running = true
	s.stats.LastRun = time.Now()
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	s.logger.Info("Starting database maintenance")

	startTime := time.Now()
	var totalErrors int

	// Purge old crashes
	if err := s.purgeOldCrashes(ctx); err != nil {
		s.logger.WithError(err).Error("Failed to purge old crashes")
		totalErrors++
		s.mu.Lock()
		s.stats.Errors++
		s.mu.Unlock()
	}

	// Purge old jobs
	if err := s.purgeOldJobs(ctx); err != nil {
		s.logger.WithError(err).Error("Failed to purge old jobs")
		totalErrors++
		s.mu.Lock()
		s.stats.Errors++
		s.mu.Unlock()
	}

	// Vacuum database
	if err := s.vacuumDatabase(ctx); err != nil {
		s.logger.WithError(err).Error("Failed to vacuum database")
		totalErrors++
		s.mu.Lock()
		s.stats.Errors++
		s.mu.Unlock()
	}

	duration := time.Since(startTime)
	s.logger.WithFields(logrus.Fields{
		"duration": duration,
		"errors":   totalErrors,
	}).Info("Database maintenance completed")

	if totalErrors > 0 {
		return common.NewSystemError("maintenance_operation", fmt.Errorf("maintenance completed with %d errors", totalErrors))
	}

	return nil
}

// ScheduleMaintenance starts periodic maintenance operations
func (s *MaintenanceService) ScheduleMaintenance(interval time.Duration) error {
	if interval <= 0 {
		return common.NewValidationError("schedule_maintenance", fmt.Errorf("interval must be positive"))
	}

	s.mu.Lock()
	if s.ticker != nil {
		s.mu.Unlock()
		return common.NewSystemError("schedule_maintenance", fmt.Errorf("maintenance already scheduled"))
	}

	s.ticker = time.NewTicker(interval)
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.ticker.Stop()

		for {
			select {
			case <-s.ctx.Done():
				return
			case <-s.ticker.C:
				if err := s.RunMaintenance(s.ctx); err != nil {
					s.logger.WithError(err).Error("Scheduled maintenance failed")
				}
			}
		}
	}()

	s.logger.WithField("interval", interval).Info("Scheduled periodic maintenance")
	return nil
}

// Stop stops the maintenance service
func (s *MaintenanceService) Stop() {
	s.logger.Info("Stopping maintenance service")
	s.cancel()
	s.wg.Wait()
}

// GetStats returns maintenance statistics
func (s *MaintenanceService) GetStats() MaintenanceStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// vacuumDatabase performs database vacuum operation
func (s *MaintenanceService) vacuumDatabase(ctx context.Context) error {
	startTime := time.Now()

	// Get database size before vacuum
	var sizeBefore int64
	// Try to get database size if available
	result, err := s.db.SelectOne(ctx, "SELECT page_count * page_size as size FROM pragma_page_count(), pragma_page_size()")
	if err == nil {
		if size, ok := result["size"].(int64); ok {
			sizeBefore = size
		}
	}

	s.logger.Info("Starting database vacuum")

	// Perform vacuum
	if err := s.db.Vacuum(ctx); err != nil {
		return common.NewDatabaseError("vacuum_database", err)
	}

	// Get database size after vacuum
	var sizeAfter int64
	result, err = s.db.SelectOne(ctx, "SELECT page_count * page_size as size FROM pragma_page_count(), pragma_page_size()")
	if err == nil {
		if size, ok := result["size"].(int64); ok {
			sizeAfter = size
		}
	}

	duration := time.Since(startTime)
	bytesReclaimed := sizeBefore - sizeAfter

	s.mu.Lock()
	s.stats.VacuumRuns++
	s.stats.LastVacuumSize = sizeAfter
	s.stats.LastVacuumTime = duration
	if bytesReclaimed > 0 {
		s.stats.BytesReclaimed += bytesReclaimed
	}
	s.mu.Unlock()

	s.logger.WithFields(logrus.Fields{
		"duration":        duration,
		"size_before":     sizeBefore,
		"size_after":      sizeAfter,
		"bytes_reclaimed": bytesReclaimed,
	}).Info("Database vacuum completed")

	return nil
}

// purgeOldCrashes removes old crash records from the database
func (s *MaintenanceService) purgeOldCrashes(ctx context.Context) error {
	// Default to 30 days if not configured
	maxAge := 30 * 24 * time.Hour
	if s.config != nil && s.config.Limits.MaxJobDuration > 0 {
		// Use 3x the max job duration as crash retention period
		maxAge = s.config.Limits.MaxJobDuration * 3
	}

	cutoffTime := time.Now().Add(-maxAge)

	s.logger.WithFields(logrus.Fields{
		"max_age": maxAge,
		"cutoff":  cutoffTime,
	}).Info("Purging old crashes")

	// Delete old crashes
	query := `DELETE FROM crashes WHERE timestamp < ?`
	result, err := s.db.Execute(ctx, query, cutoffTime)
	if err != nil {
		return common.NewDatabaseError("purge_old_crashes", err)
	}

	s.mu.Lock()
	s.stats.CrashesPurged += result
	s.mu.Unlock()

	if result > 0 {
		s.logger.WithField("count", result).Info("Purged old crash records")

		// Also clean up orphaned crash inputs
		cleanupQuery := `DELETE FROM crash_inputs WHERE crash_id NOT IN (SELECT id FROM crashes)`
		orphaned, err := s.db.Execute(ctx, cleanupQuery)
		if err != nil {
			s.logger.WithError(err).Warn("Failed to clean up orphaned crash inputs")
		} else if orphaned > 0 {
			s.logger.WithField("count", orphaned).Info("Cleaned up orphaned crash inputs")
		}
	}

	return nil
}

// purgeOldJobs removes old job records from the database
func (s *MaintenanceService) purgeOldJobs(ctx context.Context) error {
	// Default to 7 days for completed jobs
	maxAge := 7 * 24 * time.Hour
	if s.config != nil && s.config.Limits.MaxJobDuration > 0 {
		// Keep job records for at least the max job duration
		if maxAge < s.config.Limits.MaxJobDuration {
			maxAge = s.config.Limits.MaxJobDuration
		}
	}

	cutoffTime := time.Now().Add(-maxAge)

	s.logger.WithFields(logrus.Fields{
		"max_age": maxAge,
		"cutoff":  cutoffTime,
	}).Info("Purging old jobs")

	// Delete old completed/failed jobs
	query := `
		DELETE FROM jobs 
		WHERE status IN ('completed', 'failed', 'cancelled', 'timed_out') 
		AND completed_at < ?
	`
	result, err := s.db.Execute(ctx, query, cutoffTime)
	if err != nil {
		return common.NewDatabaseError("purge_old_jobs", err)
	}

	s.mu.Lock()
	s.stats.JobsPurged += result
	s.mu.Unlock()

	if result > 0 {
		s.logger.WithField("count", result).Info("Purged old job records")

		// Clean up related data
		tables := []string{"coverage", "corpus_updates", "job_assignments"}
		for _, table := range tables {
			cleanupQuery := fmt.Sprintf("DELETE FROM %s WHERE job_id NOT IN (SELECT id FROM jobs)", table)
			orphaned, err := s.db.Execute(ctx, cleanupQuery)
			if err != nil {
				s.logger.WithError(err).WithField("table", table).Warn("Failed to clean up orphaned records")
			} else if orphaned > 0 {
				s.logger.WithFields(logrus.Fields{
					"table": table,
					"count": orphaned,
				}).Info("Cleaned up orphaned records")
			}
		}
	}

	return nil
}

// CompactDatabase performs database compaction
func (s *MaintenanceService) CompactDatabase(ctx context.Context) error {
	s.logger.Info("Starting database compaction")

	if err := s.db.Compact(ctx); err != nil {
		return common.NewDatabaseError("compact_database", err)
	}

	s.logger.Info("Database compaction completed")
	return nil
}

// OptimizeIndexes rebuilds database indexes for better performance
func (s *MaintenanceService) OptimizeIndexes(ctx context.Context) error {
	s.logger.Info("Optimizing database indexes")

	// ANALYZE updates SQLite's internal statistics
	if _, err := s.db.Execute(ctx, "ANALYZE"); err != nil {
		return common.NewDatabaseError("optimize_indexes", err)
	}

	// REINDEX rebuilds all indexes
	if _, err := s.db.Execute(ctx, "REINDEX"); err != nil {
		return common.NewDatabaseError("reindex_database", err)
	}

	s.logger.Info("Database index optimization completed")
	return nil
}

// BackupDatabase creates a database backup
func (s *MaintenanceService) BackupDatabase(ctx context.Context, backupPath string) error {
	s.logger.WithField("path", backupPath).Info("Creating database backup")

	if err := s.db.Backup(ctx, backupPath); err != nil {
		return common.NewDatabaseError("backup_database", err)
	}

	s.logger.Info("Database backup completed")
	return nil
}

// GetDatabaseHealth checks database health metrics
func (s *MaintenanceService) GetDatabaseHealth(ctx context.Context) (*DatabaseHealth, error) {
	health := &DatabaseHealth{
		CheckTime: time.Now(),
		IsHealthy: true,
	}

	// Check database connectivity
	if err := s.db.Ping(ctx); err != nil {
		health.IsHealthy = false
		health.Errors = append(health.Errors, fmt.Sprintf("ping failed: %v", err))
	}

	// Get database stats
	stats := s.db.Stats(ctx)
	health.DatabaseSize = stats.Size
	health.TotalRecords = stats.Keys

	// Check table integrity (SQLite specific)
	if _, err := s.db.Execute(ctx, "PRAGMA integrity_check"); err != nil {
		health.IsHealthy = false
		health.Errors = append(health.Errors, fmt.Sprintf("integrity check failed: %v", err))
	}

	// Check for long-running queries
	if results, err := s.db.Select(ctx, `
		SELECT COUNT(*) as count 
		FROM jobs 
		WHERE status = 'running' 
		AND started_at < ?
	`, time.Now().Add(-24*time.Hour)); err == nil && len(results) > 0 {
		if count, ok := results[0]["count"].(int64); ok && count > 0 {
			health.Warnings = append(health.Warnings, fmt.Sprintf("%d jobs running for over 24 hours", count))
		}
	}

	return health, nil
}

// DatabaseHealth represents database health status
type DatabaseHealth struct {
	CheckTime    time.Time `json:"check_time"`
	IsHealthy    bool      `json:"is_healthy"`
	DatabaseSize int64     `json:"database_size"`
	TotalRecords int64     `json:"total_records"`
	Errors       []string  `json:"errors,omitempty"`
	Warnings     []string  `json:"warnings,omitempty"`
}
