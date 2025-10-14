package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/sirupsen/logrus"
)

// JobRecoveryManager handles recovery of stuck and timed-out jobs
type JobRecoveryManager struct {
	repo              repository.JobRepository
	logger            logrus.FieldLogger
	mu                sync.RWMutex
	stuckJobThreshold time.Duration
	checkInterval     time.Duration
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	stats             RecoveryStats
}

// RecoveryStats tracks recovery statistics
type RecoveryStats struct {
	StuckJobsRecovered    int64
	TimedOutJobsRecovered int64
	OrphanedJobsRecovered int64
	LastCheckTime         time.Time
	TotalChecks           int64
}

// NewJobRecoveryManager creates a new job recovery manager
func NewJobRecoveryManager(repo repository.JobRepository, logger logrus.FieldLogger) *JobRecoveryManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &JobRecoveryManager{
		repo:              repo,
		logger:            logger.WithField("component", "job_recovery"),
		stuckJobThreshold: 30 * time.Minute, // Jobs stuck for >30 min are considered stuck
		checkInterval:     1 * time.Minute,  // Check every minute
		ctx:               ctx,
		cancel:            cancel,
	}
}

// Start begins the recovery monitoring process
func (jrm *JobRecoveryManager) Start() error {
	jrm.logger.Info("Starting job recovery manager")

	// Start recovery loop
	jrm.wg.Add(1)
	go jrm.recoveryLoop()

	// Initial recovery check
	if err := jrm.recoverJobs(); err != nil {
		jrm.logger.WithError(err).Error("Initial job recovery failed")
	}

	return nil
}

// Stop stops the recovery manager
func (jrm *JobRecoveryManager) Stop() error {
	jrm.logger.Info("Stopping job recovery manager")
	jrm.cancel()
	jrm.wg.Wait()
	return nil
}

// recoveryLoop runs periodic recovery checks
func (jrm *JobRecoveryManager) recoveryLoop() {
	defer jrm.wg.Done()

	ticker := time.NewTicker(jrm.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := jrm.recoverJobs(); err != nil {
				jrm.logger.WithError(err).Error("Job recovery check failed")
			}
		case <-jrm.ctx.Done():
			return
		}
	}
}

// recoverJobs performs all recovery operations
func (jrm *JobRecoveryManager) recoverJobs() error {
	jrm.mu.Lock()
	jrm.stats.LastCheckTime = time.Now()
	jrm.stats.TotalChecks++
	jrm.mu.Unlock()

	// Release stuck jobs (jobs that have been running too long without progress)
	if err := jrm.ReleaseStuckJobs(); err != nil {
		jrm.logger.WithError(err).Error("Failed to release stuck jobs")
	}

	// Reassign timed-out jobs
	if err := jrm.ReassignTimedOutJobs(); err != nil {
		jrm.logger.WithError(err).Error("Failed to reassign timed-out jobs")
	}

	// Recover orphaned jobs (jobs with no assigned bot or invalid assignment)
	if err := jrm.RecoverOrphanedJobs(); err != nil {
		jrm.logger.WithError(err).Error("Failed to recover orphaned jobs")
	}

	return nil
}

// ReleaseStuckJobs finds and releases jobs that are stuck
func (jrm *JobRecoveryManager) ReleaseStuckJobs() error {
	ctx := context.Background()

	// Get all running jobs
	runningJobs, err := jrm.repo.ListByStatus(ctx, types.StatusRunning)
	if err != nil {
		return fmt.Errorf("failed to list running jobs: %w", err)
	}

	stuckJobs := 0
	now := time.Now()

	for _, job := range runningJobs {
		// Check if job has been running too long without updates
		if job.StartedAt != nil {
			runningDuration := now.Sub(*job.StartedAt)

			// Check if job exceeds stuck threshold
			if runningDuration > jrm.stuckJobThreshold {
				// Check if job has progress updates
				if !jrm.hasRecentProgress(job) {
					jrm.logger.WithFields(logrus.Fields{
						"job_id":           job.ID,
						"running_duration": runningDuration,
						"locked_by":        job.LockedBy,
					}).Warn("Found stuck job, releasing")

					// Unlock the job and set it back to pending
					if err := jrm.repo.UnlockJob(ctx, job.ID, job.LockedBy); err != nil {
						jrm.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to unlock stuck job")
						continue
					}

					// Update job status to pending for reassignment
					if err := jrm.repo.UpdateStatus(ctx, job.ID, types.StatusRunning, types.StatusPending); err != nil {
						jrm.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to update stuck job status")
						continue
					}

					stuckJobs++
				}
			}
		}
	}

	if stuckJobs > 0 {
		jrm.mu.Lock()
		jrm.stats.StuckJobsRecovered += int64(stuckJobs)
		jrm.mu.Unlock()

		jrm.logger.WithField("count", stuckJobs).Info("Released stuck jobs")
	}

	return nil
}

// ReassignTimedOutJobs moves timed-out jobs back to pending
func (jrm *JobRecoveryManager) ReassignTimedOutJobs() error {
	ctx := context.Background()

	// Get all jobs in various states
	allJobs, err := jrm.repo.List(ctx, repository.JobFilter{})
	if err != nil {
		return fmt.Errorf("failed to list jobs: %w", err)
	}

	timedOutJobs := 0
	now := time.Now()

	for _, job := range allJobs {
		// Check if job has exceeded its timeout based on start time and max duration
		if job.StartedAt != nil && job.MaxDuration > 0 {
			timeoutAt := job.StartedAt.Add(job.MaxDuration)
			if now.After(timeoutAt) && (job.Status == types.StatusRunning || job.Status == types.StatusQueued) {
				jrm.logger.WithFields(logrus.Fields{
					"job_id":     job.ID,
					"timeout_at": timeoutAt,
					"status":     job.Status,
				}).Warn("Found timed-out job, reassigning")

				// Unlock if locked
				if job.LockedBy != "" {
					if err := jrm.repo.UnlockJob(ctx, job.ID, job.LockedBy); err != nil {
						jrm.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to unlock timed-out job")
						continue
					}
				}

				// Mark as failed due to timeout
				if err := jrm.repo.UpdateStatus(ctx, job.ID, job.Status, types.StatusFailed); err != nil {
					jrm.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to update timed-out job status")
					continue
				}

				// Increment retry count
				if err := jrm.repo.IncrementRetries(ctx, job.ID); err != nil {
					jrm.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to increment retries")
				}

				// If retries are available, set back to pending
				if job.RetryCount < job.MaxRetries {
					if err := jrm.repo.UpdateStatus(ctx, job.ID, types.StatusFailed, types.StatusPending); err != nil {
						jrm.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to reset job to pending")
						continue
					}
				}

				timedOutJobs++
			}
		}
	}

	if timedOutJobs > 0 {
		jrm.mu.Lock()
		jrm.stats.TimedOutJobsRecovered += int64(timedOutJobs)
		jrm.mu.Unlock()

		jrm.logger.WithField("count", timedOutJobs).Info("Reassigned timed-out jobs")
	}

	return nil
}

// RecoverOrphanedJobs finds jobs with invalid assignments and recovers them
func (jrm *JobRecoveryManager) RecoverOrphanedJobs() error {
	ctx := context.Background()

	// Get all running or queued jobs
	runningJobs, err := jrm.repo.ListByStatus(ctx, types.StatusRunning)
	if err != nil {
		return fmt.Errorf("failed to list running jobs: %w", err)
	}

	queuedJobs, err := jrm.repo.ListByStatus(ctx, types.StatusQueued)
	if err != nil {
		return fmt.Errorf("failed to list queued jobs: %w", err)
	}

	allJobs := append(runningJobs, queuedJobs...)
	orphanedJobs := 0

	for _, job := range allJobs {
		// Check if job has a lock but the lock is stale
		if job.LockedBy != "" {
			if !jrm.ValidateJobAssignment(job) {
				jrm.logger.WithFields(logrus.Fields{
					"job_id":    job.ID,
					"locked_by": job.LockedBy,
					"status":    job.Status,
				}).Warn("Found orphaned job, recovering")

				// Unlock the job
				if err := jrm.repo.UnlockJob(ctx, job.ID, job.LockedBy); err != nil {
					jrm.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to unlock orphaned job")
					continue
				}

				// Set back to pending
				if err := jrm.repo.UpdateStatus(ctx, job.ID, job.Status, types.StatusPending); err != nil {
					jrm.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to update orphaned job status")
					continue
				}

				orphanedJobs++
			}
		}
	}

	if orphanedJobs > 0 {
		jrm.mu.Lock()
		jrm.stats.OrphanedJobsRecovered += int64(orphanedJobs)
		jrm.mu.Unlock()

		jrm.logger.WithField("count", orphanedJobs).Info("Recovered orphaned jobs")
	}

	return nil
}

// ValidateJobAssignment checks if a job's assignment is still valid
func (jrm *JobRecoveryManager) ValidateJobAssignment(job *types.Job) bool {
	// Check if lock has expired
	if job.LockExpiresAt != nil && time.Now().After(*job.LockExpiresAt) {
		return false
	}

	// Additional validation can be added here
	// e.g., check if bot still exists, is online, etc.

	return true
}

// hasRecentProgress checks if a job has recent progress updates
func (jrm *JobRecoveryManager) hasRecentProgress(job *types.Job) bool {
	// Check if job has progress field and it was updated recently
	// UpdatedAt is always set, so check how recent it is
	progressAge := time.Since(job.UpdatedAt)
	// Consider progress stale if not updated in 10 minutes
	return progressAge < 10*time.Minute
}

// GetStats returns recovery statistics
func (jrm *JobRecoveryManager) GetStats() RecoveryStats {
	jrm.mu.RLock()
	defer jrm.mu.RUnlock()
	return jrm.stats
}

// SetStuckJobThreshold sets the threshold for considering a job stuck
func (jrm *JobRecoveryManager) SetStuckJobThreshold(threshold time.Duration) {
	jrm.mu.Lock()
	defer jrm.mu.Unlock()
	jrm.stuckJobThreshold = threshold
}

// SetCheckInterval sets the interval between recovery checks
func (jrm *JobRecoveryManager) SetCheckInterval(interval time.Duration) {
	jrm.mu.Lock()
	defer jrm.mu.Unlock()
	jrm.checkInterval = interval
}
