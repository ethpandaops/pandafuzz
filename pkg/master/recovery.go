package master

import (
	"context"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/sirupsen/logrus"
)

// RecoveryManager handles system recovery procedures
type RecoveryManager struct {
	state        *PersistentState
	timeout      *TimeoutManager
	retryManager *common.RetryManager
	logger       *logrus.Logger
	config       *common.MasterConfig
	stats        RecoveryStats
}

// Compile-time interface compliance check
var _ service.RecoveryManager = (*RecoveryManager)(nil)

// RecoveryStats tracks recovery operation statistics
type RecoveryStats struct {
	TotalRecoveries       int64     `json:"total_recoveries"`
	LastRecovery          time.Time `json:"last_recovery"`
	OrphanedJobsRecovered int64     `json:"orphaned_jobs_recovered"`
	TimedOutBotsReset     int64     `json:"timed_out_bots_reset"`
	StaleDataCleaned      int64     `json:"stale_data_cleaned"`
	RecoveryDuration      time.Duration `json:"recovery_duration"`
	RecoveryErrors        int64     `json:"recovery_errors"`
}

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(state *PersistentState, timeout *TimeoutManager, config *common.MasterConfig, logger *logrus.Logger) *RecoveryManager {
	
	// Retry policy for recovery operations
	recoveryPolicy := common.RetryPolicy{
		MaxRetries:   5,
		InitialDelay: 2 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
		RetryableErrors: []string{
			"database is locked",
			"connection refused",
			"timeout",
			"temporary failure",
		},
	}
	
	return &RecoveryManager{
		state:        state,
		timeout:      timeout,
		retryManager: common.NewRetryManager(recoveryPolicy),
		logger:       logger,
		config:       config,
	}
}

// RecoverOnStartup performs comprehensive system recovery on startup
func (rm *RecoveryManager) RecoverOnStartup(ctx context.Context) error {
	start := time.Now()
	rm.logger.Info("Starting system recovery on startup")
	
	err := rm.retryManager.Execute(func() error {
		// Step 1: Load all persisted state from disk
		rm.logger.Info("Loading persisted state from database")
		if err := rm.state.LoadPersistedState(ctx); err != nil {
			return fmt.Errorf("failed to load persisted state: %w", err)
		}
		
		// Step 2: Check for orphaned jobs (assigned but bot offline)
		rm.logger.Info("Recovering orphaned jobs")
		if err := rm.recoverOrphanedJobs(ctx); err != nil {
			return fmt.Errorf("failed to recover orphaned jobs: %w", err)
		}
		
		// Step 3: Reset timed-out bots to idle
		rm.logger.Info("Resetting timed-out bots")
		if err := rm.resetTimedOutBots(ctx); err != nil {
			return fmt.Errorf("failed to reset timed-out bots: %w", err)
		}
		
		// Step 4: Resume pending jobs
		rm.logger.Info("Resuming pending jobs")
		if err := rm.resumePendingJobs(ctx); err != nil {
			return fmt.Errorf("failed to resume pending jobs: %w", err)
		}
		
		// Step 5: Cleanup stale data
		rm.logger.Info("Cleaning up stale data")
		if err := rm.cleanupStaleData(ctx); err != nil {
			return fmt.Errorf("failed to cleanup stale data: %w", err)
		}
		
		// Step 6: Validate system state
		rm.logger.Info("Validating system state")
		if err := rm.validateSystemState(ctx); err != nil {
			return fmt.Errorf("system state validation failed: %w", err)
		}
		
		return nil
	})
	
	duration := time.Since(start)
	
	if err != nil {
		rm.stats.RecoveryErrors++
		rm.logger.WithError(err).Error("System recovery failed")
		return common.NewSystemError("recovery_on_startup", err)
	}
	
	// Update statistics
	rm.stats.TotalRecoveries++
	rm.stats.LastRecovery = time.Now()
	rm.stats.RecoveryDuration = duration
	
	rm.logger.WithField("duration", duration).Info("System recovery completed successfully")
	return nil
}

// recoverOrphanedJobs finds and reassigns orphaned jobs
func (rm *RecoveryManager) recoverOrphanedJobs(ctx context.Context) error {
	orphanedJobs, err := rm.state.FindOrphanedJobs()
	if err != nil {
		return err
	}
	
	if len(orphanedJobs) == 0 {
		rm.logger.Debug("No orphaned jobs found")
		return nil
	}
	
	rm.logger.WithField("count", len(orphanedJobs)).Info("Found orphaned jobs, reassigning")
	
	for _, job := range orphanedJobs {
		err := rm.retryManager.Execute(func() error {
			// Reset job to pending status
			job.Status = common.JobStatusPending
			job.AssignedBot = nil
			startedAt := (*time.Time)(nil)
			job.StartedAt = startedAt
			
			// Save updated job
			if err := rm.state.SaveJobWithRetry(ctx, job); err != nil {
				return err
			}
			
			// Remove job timeout if it exists
			rm.timeout.RemoveJobTimeout(job.ID)
			
			// Set new timeout
			rm.timeout.UpdateJobTimeout(job.ID)
			
			rm.logger.WithFields(logrus.Fields{
				"job_id":   job.ID,
				"job_name": job.Name,
			}).Info("Orphaned job reassigned")
			
			return nil
		})
		
		if err != nil {
			rm.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to recover orphaned job")
			continue
		}
		
		rm.stats.OrphanedJobsRecovered++
	}
	
	return nil
}

// resetTimedOutBots resets bots that have timed out
func (rm *RecoveryManager) resetTimedOutBots(ctx context.Context) error {
	timedOutBots, err := rm.state.FindTimedOutBots()
	if err != nil {
		return err
	}
	
	if len(timedOutBots) == 0 {
		rm.logger.Debug("No timed-out bots found")
		return nil
	}
	
	rm.logger.WithField("count", len(timedOutBots)).Info("Found timed-out bots, resetting")
	
	for _, botID := range timedOutBots {
		err := rm.retryManager.Execute(func() error {
			if err := rm.state.ResetBot(ctx, botID); err != nil {
				return err
			}
			
			// Remove bot timeout
			rm.timeout.RemoveBotTimeout(botID)
			
			rm.logger.WithField("bot_id", botID).Info("Timed-out bot reset")
			return nil
		})
		
		if err != nil {
			rm.logger.WithError(err).WithField("bot_id", botID).Error("Failed to reset timed-out bot")
			continue
		}
		
		rm.stats.TimedOutBotsReset++
	}
	
	return nil
}

// resumePendingJobs ensures pending jobs are ready for assignment
func (rm *RecoveryManager) resumePendingJobs(ctx context.Context) error {
	jobs, err := rm.state.ListJobs()
	if err != nil {
		return err
	}
	
	pendingJobs := 0
	for _, job := range jobs {
		if job.Status == common.JobStatusPending {
			// Ensure job has proper timeout
			if job.TimeoutAt.Before(time.Now()) {
				// Extend timeout if it has already passed
				newTimeout := time.Now().Add(rm.config.Timeouts.JobExecution)
				job.TimeoutAt = newTimeout
				
				if err := rm.state.SaveJobWithRetry(ctx, job); err != nil {
					rm.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to update job timeout")
					continue
				}
			}
			
			// Set timeout in timeout manager
			rm.timeout.UpdateJobTimeout(job.ID)
			pendingJobs++
			
			rm.logger.WithFields(logrus.Fields{
				"job_id":     job.ID,
				"job_name":   job.Name,
				"timeout_at": job.TimeoutAt,
			}).Debug("Pending job resumed")
		}
	}
	
	if pendingJobs > 0 {
		rm.logger.WithField("count", pendingJobs).Info("Pending jobs resumed")
	}
	
	return nil
}

// cleanupStaleData removes old and stale data
func (rm *RecoveryManager) cleanupStaleData(ctx context.Context) error {
	rm.logger.Debug("Starting stale data cleanup")
	
	// Define stale data thresholds
	staleJobThreshold := 24 * time.Hour      // Jobs older than 24 hours
	_ = 7 * 24 * time.Hour // staleCrashThreshold - Crashes older than 7 days (unused for now)
	
	cleanedCount := int64(0)
	
	// Cleanup old completed/failed jobs
	jobs, err := rm.state.ListJobs()
	if err != nil {
		return err
	}
	
	for _, job := range jobs {
		isStale := false
		
		// Check if job is completed/failed and old
		if job.Status == common.JobStatusCompleted || job.Status == common.JobStatusFailed || job.Status == common.JobStatusCancelled {
			var completionTime time.Time
			if job.CompletedAt != nil {
				completionTime = *job.CompletedAt
			} else {
				completionTime = job.CreatedAt
			}
			
			if time.Since(completionTime) > staleJobThreshold {
				isStale = true
			}
		}
		
		// Check if job has been pending too long (might be stuck)
		if job.Status == common.JobStatusPending && time.Since(job.CreatedAt) > staleJobThreshold {
			isStale = true
		}
		
		if isStale {
			err := rm.retryManager.Execute(func() error {
				// Remove job timeout
				rm.timeout.RemoveJobTimeout(job.ID)
				
				// Delete job (in a real implementation, you might archive instead)
				if err := rm.state.DeleteJob(ctx, job.ID); err != nil {
					return err
				}
				
				rm.logger.WithFields(logrus.Fields{
					"job_id":   job.ID,
					"job_name": job.Name,
					"status":   job.Status,
					"age":      time.Since(job.CreatedAt),
				}).Info("Stale job cleaned up")
				
				return nil
			})
			
			if err != nil {
				rm.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to cleanup stale job")
				continue
			}
			
			cleanedCount++
		}
	}
	
	rm.stats.StaleDataCleaned += cleanedCount
	
	if cleanedCount > 0 {
		rm.logger.WithField("count", cleanedCount).Info("Stale data cleanup completed")
	}
	
	return nil
}

// validateSystemState performs system state validation
func (rm *RecoveryManager) validateSystemState(ctx context.Context) error {
	rm.logger.Debug("Validating system state")
	
	// Check database health
	if err := rm.state.HealthCheck(ctx); err != nil {
		return fmt.Errorf("database health check failed: %w", err)
	}
	
	// Check timeout manager health
	if err := rm.timeout.HealthCheck(); err != nil {
		return fmt.Errorf("timeout manager health check failed: %w", err)
	}
	
	// Validate bot and job counts
	bots, err := rm.state.ListBots()
	if err != nil {
		return fmt.Errorf("failed to list bots for validation: %w", err)
	}
	
	jobs, err := rm.state.ListJobs()
	if err != nil {
		return fmt.Errorf("failed to list jobs for validation: %w", err)
	}
	
	// Check for consistency issues
	assignedJobs := 0
	busyBots := 0
	
	for _, job := range jobs {
		if job.Status == common.JobStatusAssigned || job.Status == common.JobStatusRunning {
			assignedJobs++
			
			// Verify bot assignment consistency
			if job.AssignedBot != nil {
				botFound := false
				for _, bot := range bots {
					if bot.ID == *job.AssignedBot {
						botFound = true
						if bot.CurrentJob == nil || *bot.CurrentJob != job.ID {
							rm.logger.WithFields(logrus.Fields{
								"job_id": job.ID,
								"bot_id": bot.ID,
							}).Warn("Inconsistent job-bot assignment detected")
						}
						break
					}
				}
				
				if !botFound {
					rm.logger.WithFields(logrus.Fields{
						"job_id": job.ID,
						"bot_id": *job.AssignedBot,
					}).Warn("Job assigned to non-existent bot")
				}
			}
		}
	}
	
	for _, bot := range bots {
		if bot.Status == common.BotStatusBusy {
			busyBots++
		}
	}
	
	rm.logger.WithFields(logrus.Fields{
		"total_bots":     len(bots),
		"busy_bots":      busyBots,
		"total_jobs":     len(jobs),
		"assigned_jobs":  assignedJobs,
	}).Info("System state validation completed")
	
	return nil
}

// HandleBotFailureWithRetry handles bot failure with retry logic
func (rm *RecoveryManager) HandleBotFailureWithRetry(ctx context.Context, botID string) error {
	return rm.retryManager.Execute(func() error {
		return rm.state.db.Transaction(ctx, func(tx common.Transaction) error {
			// Get bot
			bot, err := rm.state.GetBot(ctx, botID)
			if err != nil {
				return err
			}
			
			// Mark bot as failed
			bot.Status = common.BotStatusFailed
			bot.FailureCount++
			
			// Handle current job if any
			if bot.CurrentJob != nil {
				job, err := rm.state.GetJob(ctx, *bot.CurrentJob)
				if err != nil {
					return err
				}
				
				// Reset job for reassignment
				job.Status = common.JobStatusPending
				job.AssignedBot = nil
				startedAt := (*time.Time)(nil)
				job.StartedAt = startedAt
				
				if err := rm.state.SaveJobWithRetry(ctx, job); err != nil {
					return err
				}
				
				// Remove job timeout and set new one
				rm.timeout.RemoveJobTimeout(job.ID)
				rm.timeout.UpdateJobTimeout(job.ID)
				
				rm.logger.WithFields(logrus.Fields{
					"job_id":   job.ID,
					"bot_id":   botID,
					"job_name": job.Name,
				}).Info("Job reassigned due to bot failure")
			}
			
			// Clear bot's current job
			bot.CurrentJob = nil
			
			// Save bot state
			if err := rm.state.SaveBotWithRetry(ctx, bot); err != nil {
				return err
			}
			
			// Remove bot timeout
			rm.timeout.RemoveBotTimeout(botID)
			
			rm.logger.WithFields(logrus.Fields{
				"bot_id":        botID,
				"failure_count": bot.FailureCount,
			}).Warn("Bot failure handled")
			
			return nil
		})
	})
}

// PerformMaintenanceRecovery performs periodic maintenance recovery
func (rm *RecoveryManager) PerformMaintenanceRecovery(ctx context.Context) error {
	rm.logger.Info("Performing maintenance recovery")
	
	start := time.Now()
	
	// Check for orphaned jobs
	if err := rm.recoverOrphanedJobs(ctx); err != nil {
		rm.logger.WithError(err).Error("Maintenance: Failed to recover orphaned jobs")
	}
	
	// Check for timed-out bots
	if err := rm.resetTimedOutBots(ctx); err != nil {
		rm.logger.WithError(err).Error("Maintenance: Failed to reset timed-out bots")
	}
	
	// Light cleanup (less aggressive than startup)
	if err := rm.lightCleanup(ctx); err != nil {
		rm.logger.WithError(err).Error("Maintenance: Failed to perform light cleanup")
	}
	
	duration := time.Since(start)
	rm.logger.WithField("duration", duration).Info("Maintenance recovery completed")
	
	return nil
}

// lightCleanup performs light cleanup for maintenance
func (rm *RecoveryManager) lightCleanup(ctx context.Context) error {
	// Only clean very old data during maintenance
	staleThreshold := 48 * time.Hour
	
	jobs, err := rm.state.ListJobs()
	if err != nil {
		return err
	}
	
	cleanedCount := 0
	for _, job := range jobs {
		if (job.Status == common.JobStatusCompleted || job.Status == common.JobStatusFailed) &&
			job.CompletedAt != nil && time.Since(*job.CompletedAt) > staleThreshold {
			
			if err := rm.state.DeleteJob(ctx, job.ID); err != nil {
				rm.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to cleanup old job")
				continue
			}
			
			cleanedCount++
		}
	}
	
	if cleanedCount > 0 {
		rm.logger.WithField("count", cleanedCount).Info("Light cleanup completed")
	}
	
	return nil
}

// GetStats returns recovery statistics
func (rm *RecoveryManager) GetStats() RecoveryStats {
	return rm.stats
}

// ResetStats resets recovery statistics
func (rm *RecoveryManager) ResetStats() {
	rm.stats = RecoveryStats{}
}