package master

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/ethpandaops/pandafuzz/pkg/service"
)

// StateStoreAdapter adapts PersistentState to implement service.StateStore interface
type StateStoreAdapter struct {
	ps *PersistentState
}

// Compile-time interface compliance check
var _ service.StateStore = (*StateStoreAdapter)(nil)

// NewStateStoreAdapter creates a new adapter for PersistentState
func NewStateStoreAdapter(ps *PersistentState) service.StateStore {
	return &StateStoreAdapter{ps: ps}
}

// Bot operations
func (a *StateStoreAdapter) SaveBotWithRetry(bot *common.Bot) error {
	return a.ps.SaveBotWithRetry(context.Background(), bot)
}

func (a *StateStoreAdapter) GetBot(botID string) (*common.Bot, error) {
	return a.ps.GetBot(context.Background(), botID)
}

func (a *StateStoreAdapter) DeleteBot(botID string) error {
	return a.ps.DeleteBot(context.Background(), botID)
}

func (a *StateStoreAdapter) ListBots() ([]*common.Bot, error) {
	return a.ps.ListBots(context.Background())
}

// Job operations
func (a *StateStoreAdapter) SaveJobWithRetry(job *common.Job) error {
	return a.ps.SaveJobWithRetry(context.Background(), job)
}

func (a *StateStoreAdapter) GetJob(jobID string) (*common.Job, error) {
	return a.ps.GetJob(context.Background(), jobID)
}

func (a *StateStoreAdapter) ListJobs() ([]*common.Job, error) {
	return a.ps.ListJobs(context.Background())
}

func (a *StateStoreAdapter) AtomicJobAssignmentWithRetry(botID string) (*common.Job, error) {
	return a.ps.AtomicJobAssignmentWithRetry(context.Background(), botID)
}

func (a *StateStoreAdapter) CompleteJobWithRetry(jobID, botID string, success bool) error {
	return a.ps.CompleteJobWithRetry(context.Background(), jobID, botID, success)
}

// Result processing
func (a *StateStoreAdapter) ProcessCrashResultWithRetry(crash *common.CrashResult) error {
	return a.ps.ProcessCrashResultWithRetry(context.Background(), crash)
}

func (a *StateStoreAdapter) ProcessCoverageResultWithRetry(coverage *common.CoverageResult) error {
	return a.ps.ProcessCoverageResultWithRetry(context.Background(), coverage)
}

func (a *StateStoreAdapter) ProcessCorpusUpdateWithRetry(corpus *common.CorpusUpdate) error {
	return a.ps.ProcessCorpusUpdateWithRetry(context.Background(), corpus)
}

// Stats and health
func (a *StateStoreAdapter) GetStats() any {
	return a.ps.GetStats(context.Background())
}

func (a *StateStoreAdapter) GetDatabaseStats() any {
	return a.ps.GetDatabaseStats(context.Background())
}

func (a *StateStoreAdapter) HealthCheck() error {
	return a.ps.HealthCheck(context.Background())
}

// Optimized bot operations
func (a *StateStoreAdapter) UpdateBotHeartbeat(ctx context.Context, botID string, status common.BotStatus, currentJob *string) error {
	// Check if the underlying implementation has this method
	if updater, ok := a.ps.db.(interface {
		Execute(ctx context.Context, query string, args ...any) (int64, error)
	}); ok {
		now := time.Now()
		// Get timeout from config if available
		var timeoutDuration time.Duration = 30 * time.Second
		if a.ps.config != nil && a.ps.config.Timeouts.BotHeartbeat > 0 {
			timeoutDuration = a.ps.config.Timeouts.BotHeartbeat
		}
		timeout := now.Add(timeoutDuration)

		// Use direct UPDATE query
		query := `UPDATE bots SET last_seen = ?, status = ?, current_job = ?, is_online = ?, timeout_at = ? WHERE id = ?`
		rowsAffected, err := updater.Execute(ctx, query, now, status, currentJob, true, timeout, botID)
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			return errors.NewNotFoundError("update_bot_heartbeat", "bot")
		}

		// Update in-memory cache if needed
		a.ps.UpdateBotInCache(botID, status, currentJob, now, timeout)
		return nil
	}
	return errors.New(errors.ErrorTypeMethodNotFound, "update_bot_heartbeat", "Method not implemented")
}

func (a *StateStoreAdapter) GetAvailableBotWithCapabilities(ctx context.Context, requiredCapabilities []string) (*common.Bot, error) {
	// This would require a more complex query implementation
	return nil, errors.New(errors.ErrorTypeMethodNotFound, "get_available_bot_with_capabilities", "Method not implemented")
}

func (a *StateStoreAdapter) BatchUpdateBotStatus(ctx context.Context, botIDs []string, status common.BotStatus) error {
	if updater, ok := a.ps.db.(interface {
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
	if db, ok := a.ps.db.(interface {
		Transaction(ctx context.Context, fn func(tx common.Transaction) error) error
		Execute(ctx context.Context, query string, args ...any) (int64, error)
		SelectOne(ctx context.Context, query string, args ...any) (map[string]any, error)
	}); ok {
		var assignedJob *common.Job

		err := db.Transaction(ctx, func(tx common.Transaction) error {
			// Find and lock an available job in a single query
			if executor, ok := tx.(interface {
				SelectOne(ctx context.Context, query string, args ...any) (map[string]any, error)
				Execute(ctx context.Context, query string, args ...any) (int64, error)
			}); ok {
				// Select available job with row lock
				query := `SELECT id, name, target, fuzzer, config FROM jobs 
				          WHERE status = 'pending' AND timeout_at > ? 
				          ORDER BY created_at ASC 
				          LIMIT 1 FOR UPDATE`

				row, err := executor.SelectOne(ctx, query, time.Now())
				if err != nil {
					if err == sql.ErrNoRows {
						return errors.NewNotFoundError("job_assignment", "available job")
					}
					return err
				}

				// Parse job from row
				jobID := row["id"].(string)
				now := time.Now()

				// Update job assignment
				updateQuery := `UPDATE jobs SET status = 'assigned', assigned_bot = ?, started_at = ? WHERE id = ?`
				if _, err := executor.Execute(ctx, updateQuery, botID, now, jobID); err != nil {
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
					ID:          jobID,
					Name:        row["name"].(string),
					Target:      row["target"].(string),
					Fuzzer:      row["fuzzer"].(string),
					Status:      common.JobStatusAssigned,
					AssignedBot: &botID,
					StartedAt:   &now,
					WorkDir:     fmt.Sprintf("/tmp/pandafuzz/job_%s", jobID),
				}

				// Update caches
				a.ps.UpdateJobInCache(assignedJob)
				a.ps.UpdateBotInCacheForJob(botID, &jobID, common.BotStatusBusy)

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
	if db, ok := a.ps.db.(interface {
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
				a.ps.UpdateJobStatusInCache(jobID, status, &now)
				a.ps.UpdateBotInCacheForJob(botID, nil, common.BotStatusIdle)

				return nil
			}
			return errors.New(errors.ErrorTypeMethodNotFound, "complete_job", "Transaction methods not available")
		})
	}
	return errors.New(errors.ErrorTypeMethodNotFound, "complete_job_optimized", "Method not implemented")
}
