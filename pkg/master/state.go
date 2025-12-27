package master

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
	"github.com/sirupsen/logrus"
)

// PersistentState, StateStats, and NewPersistentState are defined in state_core.go
// Bot operations (SaveBotWithRetry, GetBot, DeleteBot, ListBots, etc.) are defined in state_bot.go
// Job operations (SaveJobWithRetry, GetJob, DeleteJob, ListJobs, AtomicJobAssignmentWithRetry, etc.) are defined in state_job.go
// Crash operations (ProcessCrashResultWithRetry, GetCrashes, GetCrash, etc.) are defined in state_crash.go

// Recovery operations
func (ps *PersistentState) LoadPersistedState(ctx context.Context) error {
	ps.logger.Info("Loading persisted state from database")

	return ps.retryManager.Execute(func() error {
		// Check if database supports advanced operations
		advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
		if isAdvanced {
			// Use Iterate method if available
			ps.mu.Lock()
			defer ps.mu.Unlock()

			// Load all jobs - use SQLiteStorage.GetJob if available for proper field loading
			jobsLoaded := 0
			ps.logger.WithField("db_type", fmt.Sprintf("%T", ps.db)).Info("Checking db type for job loading")
			if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
				// Use SQLiteStorage to load jobs from the jobs table with all fields
				ps.logger.Info("Loading jobs from SQLiteStorage jobs table")

				// We need to get job IDs first, then load each job
				if err := advDB.Iterate(ctx, "job:", func(key string, value []byte) error {
					// Extract job ID from key (format: "job:uuid")
					jobID := strings.TrimPrefix(key, "job:")
					if jobID == key {
						return nil // Skip if not a job key
					}

					// Load job from jobs table instead of using JSON value
					job, err := sqliteDB.GetJob(ctx, jobID)
					if err != nil {
						// Fallback to JSON if not in jobs table
						var jsonJob common.Job
						if err := json.Unmarshal(value, &jsonJob); err != nil {
							ps.logger.WithError(err).WithField("key", key).Warn("Failed to load job")
							return nil
						}
						ps.jobs[jsonJob.ID] = &jsonJob
					} else {
						ps.jobs[job.ID] = job
					}
					jobsLoaded++
					return nil
				}); err != nil {
					ps.logger.WithError(err).Warn("Failed to iterate jobs, continuing without loaded state")
				}
			} else {
				// Fallback: load from metadata table
				if err := advDB.Iterate(ctx, "job:", func(key string, value []byte) error {
					var job common.Job
					if err := json.Unmarshal(value, &job); err != nil {
						ps.logger.WithError(err).WithField("key", key).Warn("Failed to unmarshal job")
						return nil // Continue with other jobs
					}
					ps.jobs[job.ID] = &job
					jobsLoaded++
					return nil
				}); err != nil {
					ps.logger.WithError(err).Warn("Failed to iterate jobs, continuing without loaded state")
				}
			}

			// Load all bots with "bot:" prefix
			botsLoaded := 0
			if err := advDB.Iterate(ctx, "bot:", func(key string, value []byte) error {
				var bot common.Bot
				if err := json.Unmarshal(value, &bot); err != nil {
					ps.logger.WithError(err).WithField("key", key).Warn("Failed to unmarshal bot")
					return nil // Continue with other bots
				}
				ps.bots[bot.ID] = &bot
				botsLoaded++
				return nil
			}); err != nil {
				ps.logger.WithError(err).Warn("Failed to iterate bots, continuing without loaded state")
			}

			ps.stats.LastRecovery = time.Now()
			ps.logger.WithFields(logrus.Fields{
				"bots_loaded": botsLoaded,
				"jobs_loaded": jobsLoaded,
			}).Info("Persisted state loaded from advanced database")
		} else {
			// For basic database, we can't iterate
			// This is a limitation - jobs/bots will need to be loaded as they're accessed
			ps.stats.LastRecovery = time.Now()
			ps.logger.Warn("Database doesn't support iteration, state will be loaded on-demand")
		}

		return nil
	})
}

func (ps *PersistentState) FindOrphanedJobs(ctx context.Context) ([]*common.Job, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	var orphaned []*common.Job
	now := time.Now()

	// Add grace period for network delays and timing edge cases
	const gracePeriod = 10 * time.Second

	for _, job := range ps.jobs {
		// Job is orphaned if it's assigned but the bot is not available
		// Include JobStatusStarting in the check
		if job.Status != common.JobStatusAssigned &&
			job.Status != common.JobStatusStarting &&
			job.Status != common.JobStatusRunning {
			continue
		}

		// Early exit for jobs without assigned bots
		if job.AssignedBot == nil {
			orphaned = append(orphaned, job)
			continue
		}

		// Single bot lookup with defensive copy to avoid race conditions
		bot, exists := ps.bots[*job.AssignedBot]
		if !exists {
			orphaned = append(orphaned, job)
			continue
		}

		// Create defensive copy to avoid race conditions with concurrent bot updates
		botStatus := bot.Status
		botIsOnline := bot.IsOnline
		var botCurrentJob *string
		if bot.CurrentJob != nil {
			jobID := *bot.CurrentJob
			botCurrentJob = &jobID
		}

		// Check bot health status
		if botStatus == common.BotStatusFailed ||
			botStatus == common.BotStatusTimedOut ||
			!botIsOnline {
			orphaned = append(orphaned, job)
			continue
		}

		// Check lease expiry with grace period for jobs that have lease tokens
		// Only mark as orphaned if the lease has expired (bot failed to ACK)
		if job.LeaseExpiresAt != nil && now.Add(-gracePeriod).After(*job.LeaseExpiresAt) {
			orphaned = append(orphaned, job)
			continue
		}

		// Check job timeout ONLY if bot is not actively working on it
		// Don't timeout jobs where the bot is still online and reporting progress
		if now.After(job.TimeoutAt.Add(gracePeriod)) {
			// Verify bot is not actively working on this job
			if botCurrentJob == nil || *botCurrentJob != job.ID {
				orphaned = append(orphaned, job)
			}
			// If bot claims to be working on it, trust the bot despite timeout
		}
	}

	return orphaned, nil
}

// FindTimedOutBots and ResetBot are defined in state_bot.go

// Metadata operations
func (ps *PersistentState) SetMetadata(ctx context.Context, key string, value any) error {
	return ps.retryManager.Execute(func() error {
		// Update in-memory state first
		ps.mu.Lock()
		ps.metadata[key] = value
		ps.mu.Unlock()

		// Don't hold lock during database operation
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Persist to database
			if err := tx.Store(ctx, "metadata:"+key, value); err != nil {
				return common.NewDatabaseError("set_metadata", err)
			}

			ps.mu.Lock()
			ps.stats.TransactionCount++
			ps.mu.Unlock()

			return nil
		})
	})
}

func (ps *PersistentState) GetMetadata(ctx context.Context, key string) (any, error) {
	// Check in-memory cache first with RLock
	ps.mu.RLock()
	if value, exists := ps.metadata[key]; exists {
		ps.mu.RUnlock()
		return value, nil
	}
	ps.mu.RUnlock()

	// Load from database without holding lock
	var value any
	err := ps.retryManager.Execute(func() error {
		return ps.db.Get(ctx, "metadata:"+key, &value)
	})

	if err != nil {
		if common.IsNotFoundError(err) {
			return nil, common.NewValidationError("get_metadata", fmt.Errorf("metadata not found: %s", key))
		}
		return nil, common.NewDatabaseError("get_metadata", err)
	}

	// Update cache with proper synchronization to avoid race condition
	ps.mu.Lock()
	// Double-check if another goroutine already cached it
	if existingValue, exists := ps.metadata[key]; exists {
		ps.mu.Unlock()
		return existingValue, nil
	}
	ps.metadata[key] = value
	ps.mu.Unlock()

	return value, nil
}

// Statistics and monitoring
func (ps *PersistentState) GetStats(ctx context.Context) any {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	stats := ps.stats
	stats.BotsRegistered = int64(len(ps.bots))
	stats.JobsCreated = int64(len(ps.jobs))

	return stats
}

// GetStatsTyped returns typed state statistics
func (ps *PersistentState) GetStatsTyped(ctx context.Context) StateStats {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	stats := ps.stats
	stats.BotsRegistered = int64(len(ps.bots))
	stats.JobsCreated = int64(len(ps.jobs))

	return stats
}

func (ps *PersistentState) GetDatabaseStats(ctx context.Context) any {
	return ps.db.Stats(ctx)
}

func (ps *PersistentState) GetDatabaseStatsTyped(ctx context.Context) common.DatabaseStats {
	return ps.db.Stats(ctx)
}

// GetRawDB returns the underlying SQL database connection
// This should only be used for direct SQL operations when necessary
func (ps *PersistentState) GetRawDB() *sql.DB {
	// Check if the database is SQLiteStorage type which has the raw DB
	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		return sqliteDB.GetDB()
	}
	return nil
}

// Health check
func (ps *PersistentState) HealthCheck(ctx context.Context) error {
	return ps.db.Ping(ctx)
}

// Close gracefully shuts down the persistent state
func (ps *PersistentState) Close(ctx context.Context) error {
	ps.logger.Info("Shutting down persistent state manager")

	// Stop campaign manager if running
	if ps.campaignManager != nil {
		ps.campaignManager.Stop()
	}

	if ps.db != nil {
		return ps.db.Close(ctx)
	}

	return nil
}

// SetCampaignManager sets the campaign state manager
func (ps *PersistentState) SetCampaignManager(manager *CampaignStateManager) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.campaignManager = manager
}

// GetCampaignManager returns the campaign state manager
func (ps *PersistentState) GetCampaignManager() *CampaignStateManager {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.campaignManager
}

// Crash retrieval operations (GetCrashes, GetCrashesSorted, GetCrash, GetJobCrashes, GetCrashInput)
// are defined in state_crash.go

// Cache update methods for optimized operations
// UpdateBotInCache and UpdateBotInCacheForJob are defined in state_bot.go
// UpdateJobInCache, UpdateJobStatusInCache, and evictOldestJobFromCache are defined in state_job.go
// evictOldestBotFromCache is defined in state_bot.go

// cleanupCacheAccessTimes periodically removes stale access time entries
func (ps *PersistentState) cleanupCacheAccessTimes() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Remove access times for items no longer in cache
	for key := range ps.cacheAccessTime {
		if strings.HasPrefix(key, "bot:") {
			botID := strings.TrimPrefix(key, "bot:")
			if _, exists := ps.bots[botID]; !exists {
				delete(ps.cacheAccessTime, key)
			}
		} else if strings.HasPrefix(key, "job:") {
			jobID := strings.TrimPrefix(key, "job:")
			if _, exists := ps.jobs[jobID]; !exists {
				delete(ps.cacheAccessTime, key)
			}
		}
	}
}

// Database maintenance operations

// GetJobCoverageStats retrieves coverage statistics for a job
func (ps *PersistentState) GetJobCoverageStats(ctx context.Context, jobID string) (*CoverageStats, error) {
	// Check if database supports advanced operations
	advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
	if !isAdvanced {
		return nil, fmt.Errorf("database doesn't support coverage stats query")
	}

	// Query for latest coverage data
	query := `
		SELECT 
			MAX(edges) as total_edges,
			MAX(blocks) as total_blocks,
			MAX(features) as total_features,
			COUNT(*) as report_count
		FROM coverage 
		WHERE job_id = ?
	`

	result, err := advDB.SelectOne(ctx, query, jobID)
	if err != nil {
		return nil, err
	}

	stats := &CoverageStats{
		JobID: jobID,
	}

	if edges, ok := result["total_edges"].(int64); ok {
		stats.TotalEdges = int(edges)
	}
	if blocks, ok := result["total_blocks"].(int64); ok {
		stats.TotalBlocks = int(blocks)
	}
	if features, ok := result["total_features"].(int64); ok {
		stats.TotalFeatures = int(features)
	}
	if count, ok := result["report_count"].(int64); ok {
		stats.ReportCount = int(count)
	}

	return stats, nil
}

// GetBotCompletedJobs returns the number of jobs completed by a bot
func (ps *PersistentState) GetBotCompletedJobs(ctx context.Context, botID string) (int, error) {
	// Check if database supports advanced operations
	advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
	if !isAdvanced {
		// Fallback to counting in memory
		ps.mu.RLock()
		defer ps.mu.RUnlock()

		count := 0
		for _, job := range ps.jobs {
			if job.AssignedBot != nil && *job.AssignedBot == botID &&
				(job.Status == common.JobStatusCompleted || job.Status == common.JobStatusFailed) {
				count++
			}
		}
		return count, nil
	}

	query := `
		SELECT COUNT(*) as count
		FROM jobs 
		WHERE assigned_bot = ? 
		AND status IN ('completed', 'failed')
	`

	result, err := advDB.SelectOne(ctx, query, botID)
	if err != nil {
		return 0, err
	}

	if count, ok := result["count"].(int64); ok {
		return int(count), nil
	}

	return 0, nil
}

// OptimizeDatabase optimizes the database for better performance
func (ps *PersistentState) OptimizeDatabase(ctx context.Context) error {
	// Check if database supports advanced operations
	advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
	if !isAdvanced {
		return fmt.Errorf("database doesn't support optimization")
	}

	// Run ANALYZE to update statistics
	if _, err := advDB.Execute(ctx, "ANALYZE"); err != nil {
		return fmt.Errorf("failed to analyze database: %v", err)
	}

	ps.logger.Info("Database optimization completed")
	return nil
}

// CleanupOldRecords removes old records from the database
func (ps *PersistentState) CleanupOldRecords(ctx context.Context, maxAge time.Duration) error {
	// Check if database supports advanced operations
	advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
	if !isAdvanced {
		return fmt.Errorf("database doesn't support cleanup operations")
	}

	cutoffTime := time.Now().Add(-maxAge)
	totalDeleted := int64(0)

	// Clean up old completed jobs
	query := `
		DELETE FROM jobs 
		WHERE status IN ('completed', 'failed', 'cancelled', 'timed_out') 
		AND completed_at < ?
	`
	deleted, err := advDB.Execute(ctx, query, cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to cleanup jobs: %v", err)
	}
	totalDeleted += deleted

	// Clean up old crashes
	query = `DELETE FROM crashes WHERE timestamp < ?`
	deleted, err = advDB.Execute(ctx, query, cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to cleanup crashes: %v", err)
	}
	totalDeleted += deleted

	// Clean up old coverage data
	query = `DELETE FROM coverage WHERE timestamp < ?`
	deleted, err = advDB.Execute(ctx, query, cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to cleanup coverage: %v", err)
	}
	totalDeleted += deleted

	ps.logger.WithFields(logrus.Fields{
		"max_age": maxAge,
		"deleted": totalDeleted,
	}).Info("Cleaned up old records")

	return nil
}

// VacuumDatabase performs database vacuum operation
func (ps *PersistentState) VacuumDatabase(ctx context.Context) error {
	// Check if database supports advanced operations
	advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
	if !isAdvanced {
		return fmt.Errorf("database doesn't support vacuum operation")
	}

	// For SQLite, VACUUM reclaims unused space
	if err := advDB.Vacuum(ctx); err != nil {
		return fmt.Errorf("failed to vacuum database: %v", err)
	}

	ps.logger.Info("Database vacuum completed")
	return nil
}

// BackupDatabase creates a backup of the database
func (ps *PersistentState) BackupDatabase(ctx context.Context, backupPath string) error {
	// Check if database supports advanced operations
	advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
	if !isAdvanced {
		return fmt.Errorf("database doesn't support backup operation")
	}

	// Perform backup
	if err := advDB.Backup(ctx, backupPath); err != nil {
		return fmt.Errorf("failed to backup database: %v", err)
	}

	ps.mu.Lock()
	ps.stats.LastBackup = time.Now()
	ps.mu.Unlock()

	ps.logger.WithField("path", backupPath).Info("Database backup completed")
	return nil
}

// CoverageStats represents coverage statistics for a job
type CoverageStats struct {
	JobID         string `json:"job_id"`
	TotalEdges    int    `json:"total_edges"`
	TotalBlocks   int    `json:"total_blocks"`
	TotalFeatures int    `json:"total_features"`
	ReportCount   int    `json:"report_count"`
	ExecCount     int64  `json:"exec_count"`
}

// Analytics methods for data retrieval

// GetJobCoverageHistory retrieves coverage history for a job within a time range
func (ps *PersistentState) GetJobCoverageHistory(ctx context.Context, jobID string, startTime, endTime time.Time) ([]*common.CoverageResult, error) {
	// Check if database supports advanced operations
	advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
	if !isAdvanced {
		return nil, fmt.Errorf("database doesn't support coverage history query")
	}

	query := `
		SELECT id, job_id, bot_id, edges, new_edges, timestamp, exec_count
		FROM coverage
		WHERE job_id = ? AND timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp ASC
	`

	rows, err := advDB.Select(ctx, query, jobID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query coverage history: %w", err)
	}

	results := make([]*common.CoverageResult, 0, len(rows))
	for _, row := range rows {
		coverage := &common.CoverageResult{
			JobID: jobID,
		}

		if id, ok := row["id"].(string); ok {
			coverage.ID = id
		}
		if botID, ok := row["bot_id"].(string); ok {
			coverage.BotID = botID
		}
		if edges, ok := row["edges"].(int64); ok {
			coverage.Edges = int(edges)
		}
		if newEdges, ok := row["new_edges"].(int64); ok {
			coverage.NewEdges = int(newEdges)
		}
		if ts, ok := row["timestamp"].(time.Time); ok {
			coverage.Timestamp = ts
		}
		if execCount, ok := row["exec_count"].(int64); ok {
			coverage.ExecCount = execCount
		}

		results = append(results, coverage)
	}

	return results, nil
}

// GetCampaignCoverageHistory retrieves coverage history for a campaign within a time range
func (ps *PersistentState) GetCampaignCoverageHistory(ctx context.Context, campaignID string, startTime, endTime time.Time) ([]*common.CoverageResult, error) {
	var coverage []*common.CoverageResult

	// Get all jobs for this campaign
	jobs, err := ps.GetCampaignJobs(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	// Get coverage for each job
	for _, job := range jobs {
		jobCoverage, err := ps.GetJobCoverageHistory(ctx, job.ID, startTime, endTime)
		if err != nil {
			ps.logger.WithError(err).WithField("job_id", job.ID).Warn("Failed to get job coverage history")
			continue
		}
		coverage = append(coverage, jobCoverage...)
	}

	return coverage, nil
}

// Crash time-range analytics (GetJobCrashesInTimeRange, GetCampaignCrashesInTimeRange, GetCrashesInTimeRange)
// are defined in state_crash.go

// GetJobsInTimeRange and GetCampaignJobs are defined in state_job.go

// GetJobCoverageStats is implemented in the database maintenance section

// GetCampaignCorpusUpdates retrieves corpus updates for a campaign
func (ps *PersistentState) GetCampaignCorpusUpdates(ctx context.Context, campaignID string) ([]*common.CorpusUpdate, error) {
	// Use AdvancedDatabase.Select to query corpus_updates joined with jobs
	advDB, ok := ps.db.(common.AdvancedDatabase)
	if !ok {
		ps.logger.Warn("Database does not support advanced operations, returning empty corpus updates")
		return []*common.CorpusUpdate{}, nil
	}

	query := `
		SELECT cu.id, cu.job_id, cu.bot_id, cu.files, cu.timestamp, cu.total_size
		FROM corpus_updates cu
		INNER JOIN jobs j ON cu.job_id = j.id
		WHERE j.campaign_id = ?
		ORDER BY cu.timestamp DESC
	`

	rows, err := advDB.Select(ctx, query, campaignID)
	if err != nil {
		if err == sql.ErrNoRows {
			return []*common.CorpusUpdate{}, nil
		}
		return nil, fmt.Errorf("failed to query corpus updates: %w", err)
	}

	updates := make([]*common.CorpusUpdate, 0, len(rows))
	for _, row := range rows {
		update := &common.CorpusUpdate{}

		if id, ok := row["id"].(string); ok {
			update.ID = id
		}
		if jobID, ok := row["job_id"].(string); ok {
			update.JobID = jobID
		}
		if botID, ok := row["bot_id"].(string); ok {
			update.BotID = botID
		}
		if totalSize, ok := row["total_size"].(int64); ok {
			update.TotalSize = totalSize
		}
		if timestamp, ok := row["timestamp"].(time.Time); ok {
			update.Timestamp = timestamp
		}

		// Parse files JSON array
		if filesJSON, ok := row["files"].(string); ok && filesJSON != "" {
			var files []string
			if err := json.Unmarshal([]byte(filesJSON), &files); err != nil {
				ps.logger.WithError(err).WithField("update_id", update.ID).Warn("Failed to parse corpus files JSON")
			} else {
				update.Files = files
			}
		}

		updates = append(updates, update)
	}

	return updates, nil
}

// GetBotCompletedJobs is now implemented above (line 1382)

// Helper function to sort corpus updates by timestamp
func sortCorpusUpdatesByTimestamp(updates []*common.CorpusUpdate) {
	// Simple bubble sort since corpus updates are typically small
	n := len(updates)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if updates[j].Timestamp.After(updates[j+1].Timestamp) {
				updates[j], updates[j+1] = updates[j+1], updates[j]
			}
		}
	}
}

// StartLeaseExpirySweep starts a goroutine that periodically checks for expired job leases
func (ps *PersistentState) StartLeaseExpirySweep(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second // Default to 15 seconds to reduce DB load
	}

	ps.logger.WithField("interval", interval).Info("Starting lease expiry sweep")

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ps.sweepExpiredLeases(ctx)
			case <-ctx.Done():
				ps.logger.Info("Stopping lease expiry sweep")
				return
			}
		}
	}()
}

// sweepExpiredLeases checks for expired job leases and marks them as timed out
func (ps *PersistentState) sweepExpiredLeases(ctx context.Context) {
	now := time.Now()

	// Query for jobs with expired leases
	if executor, ok := ps.db.(interface {
		Execute(ctx context.Context, query string, args ...any) (int64, error)
		SelectAll(ctx context.Context, query string, args ...any) ([]map[string]any, error)
	}); ok {
		// First check for absolute timeout violations
		// This ensures jobs don't run forever even if they keep heartbeating
		absoluteTimeoutQuery := `UPDATE jobs SET 
		                        status = 'timed_out',
		                        assigned_bot = NULL,
		                        lease_token = NULL,
		                        lease_expires_at = NULL,
		                        completed_at = ?
		                        WHERE status IN ('assigned', 'starting', 'running')
		                        AND timeout_at IS NOT NULL 
		                        AND timeout_at <= ?`

		rowsAffected, err := executor.Execute(ctx, absoluteTimeoutQuery, now, now)
		if err != nil {
			ps.logger.WithError(err).Error("Failed to enforce absolute timeouts")
		} else if rowsAffected > 0 {
			ps.logger.WithField("count", rowsAffected).Warn("Jobs timed out due to absolute timeout")
		}

		// Find expired leases AND legacy jobs without leases that are stale
		// For backward compatibility: jobs with NULL lease_expires_at that have been assigned for > 45 seconds are considered stale
		staleTime := now.Add(-45 * time.Second)
		query := `SELECT id, assigned_bot FROM jobs 
		          WHERE status IN ('assigned', 'starting') 
		          AND ((lease_expires_at IS NOT NULL AND lease_expires_at <= ?) OR
		               (lease_expires_at IS NULL AND started_at IS NOT NULL AND started_at <= ?))`

		rows, err := executor.SelectAll(ctx, query, now, staleTime)
		if err != nil {
			ps.logger.WithError(err).Error("Failed to query expired leases")
			return
		}

		if len(rows) == 0 {
			return // No expired leases
		}

		ps.logger.WithField("count", len(rows)).Debug("Found expired leases")

		// Mark each expired job as timed out
		for _, row := range rows {
			jobID, ok := row["id"].(string)
			if !ok {
				continue
			}

			var assignedBot *string
			if bot, ok := row["assigned_bot"].(string); ok && bot != "" {
				assignedBot = &bot
			}

			// Update job to timed_out status
			updateQuery := `UPDATE jobs 
			                SET status = 'timed_out', 
			                    assigned_bot = NULL,
			                    lease_token = NULL,
			                    lease_expires_at = NULL,
			                    completed_at = ?
			                WHERE id = ? AND status IN ('assigned', 'starting')`

			rowsAffected, err := executor.Execute(ctx, updateQuery, now, jobID)
			if err != nil {
				ps.logger.WithError(err).WithField("job_id", jobID).Error("Failed to mark job as timed out")
				continue
			}

			if rowsAffected > 0 {
				ps.logger.WithFields(logrus.Fields{
					"job_id": jobID,
					"bot_id": assignedBot,
				}).Warn("Job lease expired, marked as timed out")

				// Update bot status if it was assigned
				if assignedBot != nil {
					botQuery := `UPDATE bots SET status = 'idle', current_job = NULL WHERE id = ? AND current_job = ?`
					executor.Execute(ctx, botQuery, *assignedBot, jobID)

					// Update cache
					ps.UpdateBotInCacheForJob(*assignedBot, nil, common.BotStatusIdle)
				}

				// Update job cache
				ps.UpdateJobStatusInCache(jobID, common.JobStatusTimedOut, &now)
			}
		}
	}
}
