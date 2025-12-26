package master

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/ethpandaops/pandafuzz/pkg/service"
)

// StateStoreAdapter adapts PersistentState to implement service.StateStore interface
type StateStoreAdapter struct {
	PS *PersistentState // Exported for access in main
}

// Compile-time interface compliance check
var _ service.StateStore = (*StateStoreAdapter)(nil)

// NewStateStoreAdapter creates a new adapter for PersistentState
func NewStateStoreAdapter(ps *PersistentState) service.StateStore {
	return &StateStoreAdapter{PS: ps}
}

// Bot operations
func (a *StateStoreAdapter) SaveBotWithRetry(bot *common.Bot) error {
	return a.PS.SaveBotWithRetry(context.Background(), bot)
}

func (a *StateStoreAdapter) GetBot(botID string) (*common.Bot, error) {
	return a.PS.GetBot(context.Background(), botID)
}

func (a *StateStoreAdapter) DeleteBot(botID string) error {
	return a.PS.DeleteBot(context.Background(), botID)
}

func (a *StateStoreAdapter) ListBots() ([]*common.Bot, error) {
	return a.PS.ListBots(context.Background())
}

// Job operations
func (a *StateStoreAdapter) SaveJobWithRetry(job *common.Job) error {
	return a.PS.SaveJobWithRetry(context.Background(), job)
}

func (a *StateStoreAdapter) GetJob(jobID string) (*common.Job, error) {
	return a.PS.GetJob(context.Background(), jobID)
}

func (a *StateStoreAdapter) ListJobs() ([]*common.Job, error) {
	return a.PS.ListJobs(context.Background())
}

func (a *StateStoreAdapter) AtomicJobAssignmentWithRetry(botID string) (*common.Job, error) {
	return a.PS.AtomicJobAssignmentWithRetry(context.Background(), botID)
}

func (a *StateStoreAdapter) CompleteJobWithRetry(jobID, botID string, success bool) error {
	return a.PS.CompleteJobWithRetry(context.Background(), jobID, botID, success)
}

// Result processing
func (a *StateStoreAdapter) ProcessCrashResultWithRetry(crash *common.CrashResult) error {
	return a.PS.ProcessCrashResultWithRetry(context.Background(), crash)
}

func (a *StateStoreAdapter) ProcessCoverageResultWithRetry(coverage *common.CoverageResult) error {
	return a.PS.ProcessCoverageResultWithRetry(context.Background(), coverage)
}

func (a *StateStoreAdapter) ProcessCorpusUpdateWithRetry(corpus *common.CorpusUpdate) error {
	return a.PS.ProcessCorpusUpdateWithRetry(context.Background(), corpus)
}

// Stats and health
func (a *StateStoreAdapter) GetStats() any {
	return a.PS.GetStats(context.Background())
}

func (a *StateStoreAdapter) GetDatabaseStats() any {
	return a.PS.GetDatabaseStats(context.Background())
}

func (a *StateStoreAdapter) HealthCheck() error {
	return a.PS.HealthCheck(context.Background())
}

// Optimized bot operations
func (a *StateStoreAdapter) UpdateBotHeartbeat(ctx context.Context, botID string, status common.BotStatus, currentJob *string) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return fmt.Errorf("context cancelled: %w", ctx.Err())
	default:
	}

	// Validate input
	if botID == "" {
		return common.NewValidationError("update_bot_heartbeat", fmt.Errorf("bot ID is required"))
	}

	// Check if the underlying implementation has this method
	if updater, ok := a.PS.db.(interface {
		Execute(ctx context.Context, query string, args ...any) (int64, error)
	}); ok {
		now := time.Now()
		// Get timeout from config if available
		var timeoutDuration time.Duration = 30 * time.Second
		if a.PS.config != nil && a.PS.config.Timeouts.BotHeartbeat > 0 {
			timeoutDuration = a.PS.config.Timeouts.BotHeartbeat
		}
		timeout := now.Add(timeoutDuration)

		// Use direct UPDATE query
		query := `UPDATE bots SET last_seen = ?, status = ?, current_job = ?, is_online = ?, timeout_at = ? WHERE id = ?`
		rowsAffected, err := updater.Execute(ctx, query, now, status, currentJob, true, timeout, botID)
		if err != nil {
			return common.NewStorageError("update_bot_heartbeat", err)
		}
		if rowsAffected == 0 {
			return errors.NewNotFoundError("update_bot_heartbeat", "bot")
		}

		// Update in-memory cache if needed
		a.PS.UpdateBotInCache(botID, status, currentJob, now, timeout)
		return nil
	}
	return errors.New(errors.ErrorTypeMethodNotFound, "update_bot_heartbeat", "Method not implemented")
}

func (a *StateStoreAdapter) GetAvailableBotWithCapabilities(ctx context.Context, requiredCapabilities []string) (*common.Bot, error) {
	// This would require a more complex query implementation
	return nil, errors.New(errors.ErrorTypeMethodNotFound, "get_available_bot_with_capabilities", "Method not implemented")
}

func (a *StateStoreAdapter) BatchUpdateBotStatus(ctx context.Context, botIDs []string, status common.BotStatus) error {
	if updater, ok := a.PS.db.(interface {
		Execute(ctx context.Context, query string, args ...any) (int64, error)
	}); ok {
		// Build placeholders for IN clause
		placeholders := make([]string, len(botIDs))
		args := make([]any, 0, len(botIDs)+2)
		args = append(args, status, false) // status and is_online

		for i, id := range botIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}

		query := fmt.Sprintf(`UPDATE bots SET status = ?, is_online = ? WHERE id IN (%s)`, strings.Join(placeholders, ","))
		_, err := updater.Execute(ctx, query, args...)
		return err
	}
	return errors.New(errors.ErrorTypeMethodNotFound, "batch_update_bot_status", "Method not implemented")
}

// Optimized job operations
func (a *StateStoreAdapter) ListJobsFiltered(ctx context.Context, status *common.JobStatus, fuzzer *string, limit, page int) ([]*common.Job, error) {
	// This would require implementing filtered queries
	return nil, errors.New(errors.ErrorTypeMethodNotFound, "list_jobs_filtered", "Method not implemented")
}

func (a *StateStoreAdapter) AtomicJobAssignmentOptimized(ctx context.Context, botID string) (*common.Job, error) {
	if db, ok := a.PS.db.(interface {
		Transaction(ctx context.Context, fn func(tx common.Transaction) error) error
		Execute(ctx context.Context, query string, args ...any) (int64, error)
		SelectOne(ctx context.Context, query string, args ...any) (map[string]any, error)
	}); ok {
		var assignedJob *common.Job
		var leaseToken string

		err := db.Transaction(ctx, func(tx common.Transaction) error {
			// Find and lock an available job in a single query
			if executor, ok := tx.(interface {
				SelectOne(ctx context.Context, query string, args ...any) (map[string]any, error)
				Execute(ctx context.Context, query string, args ...any) (int64, error)
			}); ok {
				// Select available job with row lock, also check for expired leases
				now := time.Now()
				// For backward compatibility: jobs with NULL lease_expires_at that have been assigned for > 45 seconds are considered stale
				staleTime := now.Add(-45 * time.Second)
				query := `SELECT id, name, target, fuzzer, config FROM jobs 
				          WHERE (status = 'pending' OR 
				                (status IN ('assigned', 'starting') AND 
				                 ((lease_expires_at IS NOT NULL AND lease_expires_at <= ?) OR
				                  (lease_expires_at IS NULL AND started_at IS NOT NULL AND started_at <= ?))))
				                AND timeout_at > ? 
				          ORDER BY created_at ASC 
				          LIMIT 1 FOR UPDATE`

				row, err := executor.SelectOne(ctx, query, now, staleTime, now)
				if err != nil {
					if err == sql.ErrNoRows {
						return errors.NewNotFoundError("job_assignment", "available job")
					}
					return err
				}

				// Parse job from row
				jobID := row["id"].(string)

				// Generate secure lease token
				leaseToken = generateSecureToken()
				leaseExpiresAt := now.Add(45 * time.Second) // 45 seconds to ACK (accounts for network delays)

				// Update job assignment with lease
				updateQuery := `UPDATE jobs SET 
				                status = 'assigned', 
				                assigned_bot = ?, 
				                started_at = ?,
				                lease_token = ?,
				                lease_expires_at = ?
				                WHERE id = ?`
				if _, err := executor.Execute(ctx, updateQuery, botID, now, leaseToken, leaseExpiresAt, jobID); err != nil {
					return err
				}

				// Update bot status
				botQuery := `UPDATE bots SET status = 'busy', current_job = ?, last_seen = ? WHERE id = ? AND status = 'idle'`
				rowsAffected, err := executor.Execute(ctx, botQuery, jobID, now, botID)
				if err != nil {
					return err
				}
				if rowsAffected == 0 {
					return errors.NewValidationError("job_assignment", "Bot not available")
				}

				// Create assignment record
				assignQuery := `INSERT INTO job_assignments (job_id, bot_id, timestamp, status) VALUES (?, ?, ?, 'assigned')`
				if _, err := executor.Execute(ctx, assignQuery, jobID, botID, now); err != nil {
					return err
				}

				// Build job object
				assignedJob = &common.Job{
					ID:             jobID,
					Name:           row["name"].(string),
					Target:         row["target"].(string),
					Fuzzer:         row["fuzzer"].(string),
					Status:         common.JobStatusAssigned,
					AssignedBot:    &botID,
					StartedAt:      &now,
					WorkDir:        fmt.Sprintf("job_%s", jobID), // Relative path for bot to resolve
					LeaseToken:     &leaseToken,
					LeaseExpiresAt: &leaseExpiresAt,
				}

				// Parse config JSON from database
				if configVal, ok := row["config"]; ok && configVal != nil {
					var configStr string
					switch v := configVal.(type) {
					case string:
						configStr = v
					case []byte:
						configStr = string(v)
					}
					if configStr != "" {
						if err := json.Unmarshal([]byte(configStr), &assignedJob.Config); err != nil {
							// Log error but don't fail - use default config
							a.PS.logger.WithError(err).WithField("job_id", jobID).Warn("Failed to unmarshal job config during assignment")
						}
					}
				}

				// Update caches
				a.PS.UpdateJobInCache(assignedJob)
				a.PS.UpdateBotInCacheForJob(botID, &jobID, common.BotStatusBusy)

				return nil
			}
			return errors.New(errors.ErrorTypeMethodNotFound, "job_assignment", "Transaction methods not available")
		})

		if err != nil {
			return nil, err
		}
		return assignedJob, nil
	}
	return nil, errors.New(errors.ErrorTypeMethodNotFound, "atomic_job_assignment_optimized", "Method not implemented")
}

func (a *StateStoreAdapter) CompleteJobOptimized(ctx context.Context, jobID, botID string, success bool) error {
	if db, ok := a.PS.db.(interface {
		Transaction(ctx context.Context, fn func(tx common.Transaction) error) error
		Execute(ctx context.Context, query string, args ...any) (int64, error)
	}); ok {
		return db.Transaction(ctx, func(tx common.Transaction) error {
			if executor, ok := tx.(interface {
				Execute(ctx context.Context, query string, args ...any) (int64, error)
			}); ok {
				now := time.Now()
				status := common.JobStatusCompleted
				if !success {
					status = common.JobStatusFailed
				}

				// Update job status and verify assignment in one query
				jobQuery := `UPDATE jobs SET status = ?, completed_at = ?, assigned_bot = NULL 
				             WHERE id = ? AND assigned_bot = ?`
				rowsAffected, err := executor.Execute(ctx, jobQuery, status, now, jobID, botID)
				if err != nil {
					return err
				}
				if rowsAffected == 0 {
					return errors.NewValidationError("complete_job", "Job not assigned to this bot")
				}

				// Update bot status
				botQuery := `UPDATE bots SET status = 'idle', current_job = NULL, last_seen = ? WHERE id = ?`
				if _, err := executor.Execute(ctx, botQuery, now, botID); err != nil {
					return err
				}

				// Update assignment record
				assignQuery := `UPDATE job_assignments SET status = 'completed' WHERE job_id = ? AND bot_id = ?`
				if _, err := executor.Execute(ctx, assignQuery, jobID, botID); err != nil {
					return err
				}

				// Update caches
				a.PS.UpdateJobStatusInCache(jobID, status, &now)
				a.PS.UpdateBotInCacheForJob(botID, nil, common.BotStatusIdle)

				return nil
			}
			return errors.New(errors.ErrorTypeMethodNotFound, "complete_job", "Transaction methods not available")
		})
	}
	return errors.New(errors.ErrorTypeMethodNotFound, "complete_job_optimized", "Method not implemented")
}

// GetStorage returns the underlying storage interface
// This is needed for services that require direct storage access
func (a *StateStoreAdapter) GetStorage() common.Storage {
	// Note: common.Database and common.Storage have incompatible Close() signatures,
	// so a database.Database cannot also implement common.Storage directly.
	// Check if PersistentState provides a storage reference
	if a.PS != nil && a.PS.Storage != nil {
		return a.PS.Storage
	}
	return nil
}

// generateSecureToken generates a cryptographically secure random token
func generateSecureToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based token if crypto/rand fails
		return fmt.Sprintf("lease_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// Analytics operations
func (a *StateStoreAdapter) GetCampaignJobs(ctx context.Context, campaignID string) ([]*common.Job, error) {
	return a.PS.GetCampaignJobs(ctx, campaignID)
}

func (a *StateStoreAdapter) GetJobCrashes(ctx context.Context, jobID string) ([]*common.CrashResult, error) {
	return a.PS.GetJobCrashes(ctx, jobID)
}

func (a *StateStoreAdapter) GetCrashesInTimeRange(ctx context.Context, startTime, endTime time.Time) ([]*common.CrashResult, error) {
	return a.PS.GetCrashesInTimeRange(ctx, startTime, endTime)
}

func (a *StateStoreAdapter) GetJobCoverageHistory(ctx context.Context, jobID string, startTime, endTime time.Time) ([]*common.CoverageResult, error) {
	return a.PS.GetJobCoverageHistory(ctx, jobID, startTime, endTime)
}
