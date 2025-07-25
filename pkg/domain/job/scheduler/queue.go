package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/sirupsen/logrus"
)

// Queue manages job scheduling and execution
type Queue interface {
	// Start begins processing jobs from the queue
	Start(ctx context.Context) error

	// Stop gracefully shuts down the queue
	Stop() error

	// Enqueue adds a job to the queue
	Enqueue(ctx context.Context, job *types.Job) error

	// EnqueueWithDelay schedules a job for future execution
	EnqueueWithDelay(ctx context.Context, job *types.Job, delay time.Duration) error

	// Cancel removes a job from the queue
	Cancel(ctx context.Context, jobID string) error

	// GetStats returns queue statistics
	GetStats() QueueStats

	// GetJob retrieves job information
	GetJob(ctx context.Context, jobID string) (*types.Job, error)

	// ListJobs lists jobs matching the filter
	ListJobs(ctx context.Context, filter repository.JobFilter) ([]*types.Job, error)
}

// QueueStats contains queue performance metrics
type QueueStats struct {
	TotalJobs       int64                     `json:"total_jobs"`
	JobsByStatus    map[types.JobStatus]int64 `json:"jobs_by_status"`
	EnqueuedCount   int64                     `json:"enqueued_count"`
	ProcessedCount  int64                     `json:"processed_count"`
	FailedCount     int64                     `json:"failed_count"`
	RetryCount      int64                     `json:"retry_count"`
	AverageWaitTime time.Duration             `json:"average_wait_time"`
	AverageExecTime time.Duration             `json:"average_exec_time"`
	WorkersActive   int                       `json:"workers_active"`
	WorkersTotal    int                       `json:"workers_total"`
	LastProcessedAt time.Time                 `json:"last_processed_at"`
	QueueDepth      int                       `json:"queue_depth"`
}

// JobProcessor defines the interface for processing jobs
type JobProcessor interface {
	// Process executes a job
	Process(ctx context.Context, job *types.Job) error
}

// Config contains queue configuration
type Config struct {
	Workers          int           `json:"workers"`
	MaxRetries       int           `json:"max_retries"`
	RetryDelay       time.Duration `json:"retry_delay"`
	LockDuration     time.Duration `json:"lock_duration"`
	StaleLockTimeout time.Duration `json:"stale_lock_timeout"`
	PollInterval     time.Duration `json:"poll_interval"`
	EnablePriority   bool          `json:"enable_priority"`
}

// DefaultConfig returns default queue configuration
func DefaultConfig() Config {
	return Config{
		Workers:          4,
		MaxRetries:       3,
		RetryDelay:       30 * time.Second,
		LockDuration:     5 * time.Minute,
		StaleLockTimeout: 10 * time.Minute,
		PollInterval:     1 * time.Second,
		EnablePriority:   true,
	}
}

// queue implements the Queue interface
type queue struct {
	config    Config
	repo      repository.JobRepository
	processor JobProcessor
	log       logrus.FieldLogger

	// Synchronization
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	running   atomic.Bool
	workerIDs chan string

	// Statistics
	stats struct {
		enqueuedCount  atomic.Int64
		processedCount atomic.Int64
		failedCount    atomic.Int64
		retryCount     atomic.Int64
		totalWaitTime  atomic.Int64
		totalExecTime  atomic.Int64
	}
}

// NewQueue creates a new job queue
func NewQueue(config Config, repo repository.JobRepository, processor JobProcessor, log logrus.FieldLogger) Queue {
	workerIDs := make(chan string, config.Workers)
	for i := 0; i < config.Workers; i++ {
		workerIDs <- fmt.Sprintf("worker-%d", i)
	}

	return &queue{
		config:    config,
		repo:      repo,
		processor: processor,
		log:       log.WithField("component", "job-queue"),
		workerIDs: workerIDs,
	}
}

// Start begins processing jobs from the queue
func (q *queue) Start(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.running.Load() {
		return errors.New("queue already running")
	}

	q.ctx, q.cancel = context.WithCancel(ctx)
	q.running.Store(true)

	// Start worker goroutines
	for i := 0; i < q.config.Workers; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}

	// Start maintenance tasks
	q.wg.Add(3)
	go q.scheduledJobPoller()
	go q.staleLockCleaner()
	go q.retryHandler()

	q.log.WithField("workers", q.config.Workers).Info("Queue started")
	return nil
}

// Stop gracefully shuts down the queue
func (q *queue) Stop() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if !q.running.Load() {
		return errors.New("queue not running")
	}

	q.log.Info("Stopping queue...")
	q.cancel()
	q.running.Store(false)

	// Wait for all workers to finish
	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		q.log.Info("Queue stopped gracefully")
		return nil
	case <-time.After(30 * time.Second):
		q.log.Warn("Queue stop timeout")
		return errors.New("queue stop timeout")
	}
}

// Enqueue adds a job to the queue
func (q *queue) Enqueue(ctx context.Context, job *types.Job) error {
	if !q.running.Load() {
		return errors.New("queue not running")
	}

	// Validate job
	if err := job.Validate(); err != nil {
		return fmt.Errorf("invalid job: %w", err)
	}

	// Set defaults if not provided
	if job.MaxRetries == 0 {
		job.MaxRetries = q.config.MaxRetries
	}
	if job.RetryDelay == 0 {
		job.RetryDelay = q.config.RetryDelay
	}

	// Mark as queued
	if err := job.Queue(); err != nil {
		return fmt.Errorf("failed to queue job: %w", err)
	}

	// Persist to repository
	if err := q.repo.Create(ctx, job); err != nil {
		return fmt.Errorf("failed to persist job: %w", err)
	}

	q.stats.enqueuedCount.Add(1)
	q.log.WithFields(logrus.Fields{
		"job_id":   job.ID,
		"priority": job.Priority,
		"name":     job.Name,
	}).Info("Job enqueued")

	return nil
}

// EnqueueWithDelay schedules a job for future execution
func (q *queue) EnqueueWithDelay(ctx context.Context, job *types.Job, delay time.Duration) error {
	scheduledAt := time.Now().UTC().Add(delay)
	job.ScheduledAt = &scheduledAt
	return q.Enqueue(ctx, job)
}

// Cancel removes a job from the queue
func (q *queue) Cancel(ctx context.Context, jobID string) error {
	job, err := q.repo.Get(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	if job.IsFinished() {
		return fmt.Errorf("cannot cancel finished job: %s", job.Status)
	}

	if err := job.Cancel(); err != nil {
		return fmt.Errorf("failed to cancel job: %w", err)
	}

	if err := q.repo.Update(ctx, job); err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	q.log.WithField("job_id", jobID).Info("Job cancelled")
	return nil
}

// GetStats returns queue statistics
func (q *queue) GetStats() QueueStats {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats := QueueStats{
		EnqueuedCount:  q.stats.enqueuedCount.Load(),
		ProcessedCount: q.stats.processedCount.Load(),
		FailedCount:    q.stats.failedCount.Load(),
		RetryCount:     q.stats.retryCount.Load(),
		WorkersTotal:   q.config.Workers,
		WorkersActive:  q.config.Workers - len(q.workerIDs),
	}

	// Calculate average times
	if stats.ProcessedCount > 0 {
		stats.AverageWaitTime = time.Duration(q.stats.totalWaitTime.Load() / stats.ProcessedCount)
		stats.AverageExecTime = time.Duration(q.stats.totalExecTime.Load() / stats.ProcessedCount)
	}

	// Get job counts by status
	if counts, err := q.repo.CountByStatus(ctx); err == nil {
		stats.JobsByStatus = counts
		for _, count := range counts {
			stats.TotalJobs += count
		}
		stats.QueueDepth = int(counts[types.StatusQueued])
	}

	return stats
}

// GetJob retrieves job information
func (q *queue) GetJob(ctx context.Context, jobID string) (*types.Job, error) {
	return q.repo.Get(ctx, jobID)
}

// ListJobs lists jobs matching the filter
func (q *queue) ListJobs(ctx context.Context, filter repository.JobFilter) ([]*types.Job, error) {
	return q.repo.List(ctx, filter)
}

// worker processes jobs from the queue
func (q *queue) worker(id int) {
	defer q.wg.Done()

	workerID := fmt.Sprintf("worker-%d", id)
	log := q.log.WithField("worker_id", workerID)
	log.Info("Worker started")

	for {
		select {
		case <-q.ctx.Done():
			log.Info("Worker stopped")
			return
		default:
			// Get worker ID from pool
			select {
			case wid := <-q.workerIDs:
				q.processNextJob(wid)
				q.workerIDs <- wid
			case <-q.ctx.Done():
				log.Info("Worker stopped")
				return
			}
		}
	}
}

// processNextJob attempts to process the next available job
func (q *queue) processNextJob(workerID string) {
	ctx, cancel := context.WithTimeout(q.ctx, 30*time.Second)
	defer cancel()

	// Try to get and lock a job
	job, err := q.getNextJob(ctx, workerID)
	if err != nil {
		if !errors.Is(err, ErrNoJobsAvailable) {
			q.log.WithError(err).Error("Failed to get next job")
		}
		time.Sleep(q.config.PollInterval)
		return
	}

	// Process the job
	q.executeJob(ctx, job, workerID)
}

// getNextJob retrieves and locks the next available job
func (q *queue) getNextJob(ctx context.Context, workerID string) (*types.Job, error) {
	// Get pending jobs (priority ordering handled by repository)
	jobs, err := q.repo.ListPending(ctx, 10)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending jobs: %w", err)
	}

	if len(jobs) == 0 {
		return nil, ErrNoJobsAvailable
	}

	// Try to lock a job
	for _, job := range jobs {
		// Check dependencies
		if job.HasDependencies() {
			ready, err := q.checkDependencies(ctx, job)
			if err != nil {
				q.log.WithError(err).WithField("job_id", job.ID).Error("Failed to check dependencies")
				continue
			}
			if !ready {
				continue
			}
		}

		// Try to lock the job
		lockedJob, err := q.repo.LockForProcessing(ctx, job.ID, workerID, q.config.LockDuration)
		if err != nil {
			continue // Job was locked by another worker
		}
		if lockedJob != nil {
			return lockedJob, nil
		}
	}

	return nil, ErrNoJobsAvailable
}

// checkDependencies checks if all job dependencies are completed
func (q *queue) checkDependencies(ctx context.Context, job *types.Job) (bool, error) {
	for _, depID := range job.Dependencies {
		dep, err := q.repo.Get(ctx, depID)
		if err != nil {
			return false, fmt.Errorf("failed to get dependency %s: %w", depID, err)
		}
		if dep.Status != types.StatusCompleted {
			return false, nil
		}
	}
	return true, nil
}

// executeJob processes a job
func (q *queue) executeJob(ctx context.Context, job *types.Job, workerID string) {
	log := q.log.WithFields(logrus.Fields{
		"job_id":    job.ID,
		"worker_id": workerID,
	})

	// Calculate wait time
	if job.QueuedAt != nil {
		waitTime := time.Since(*job.QueuedAt)
		q.stats.totalWaitTime.Add(int64(waitTime))
	}

	// Start the job
	if err := job.Start(); err != nil {
		log.WithError(err).Error("Failed to start job")
		q.handleJobError(ctx, job, err)
		return
	}

	if err := q.repo.Update(ctx, job); err != nil {
		log.WithError(err).Error("Failed to update job status")
		return
	}

	startTime := time.Now()
	log.Info("Processing job")

	// Execute with timeout
	execCtx := ctx
	if job.MaxDuration > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, job.MaxDuration)
		defer cancel()
	}

	// Process the job
	err := q.processor.Process(execCtx, job)
	execTime := time.Since(startTime)
	q.stats.totalExecTime.Add(int64(execTime))

	// Handle result
	if err != nil {
		log.WithError(err).Error("Job failed")
		q.handleJobError(ctx, job, err)
	} else {
		if err := job.Complete(); err != nil {
			log.WithError(err).Error("Failed to complete job")
			return
		}

		if err := q.repo.Update(ctx, job); err != nil {
			log.WithError(err).Error("Failed to update completed job")
			return
		}

		q.stats.processedCount.Add(1)
		log.WithField("duration", execTime).Info("Job completed")
	}

	// Unlock the job
	if err := q.repo.UnlockJob(ctx, job.ID, workerID); err != nil {
		log.WithError(err).Error("Failed to unlock job")
	}
}

// handleJobError handles job execution errors
func (q *queue) handleJobError(ctx context.Context, job *types.Job, err error) {
	q.stats.failedCount.Add(1)

	// Mark job as failed
	if failErr := job.Fail(err.Error()); failErr != nil {
		q.log.WithError(failErr).Error("Failed to mark job as failed")
		return
	}

	// Check if job can be retried
	if job.CanRetry() {
		job.IncrementRetries()
		job.Status = types.StatusQueued // Reset to queued for retry
		q.stats.retryCount.Add(1)

		q.log.WithFields(logrus.Fields{
			"job_id":      job.ID,
			"retry_count": job.RetryCount,
			"max_retries": job.MaxRetries,
		}).Info("Job scheduled for retry")
	}

	// Update job in repository
	if err := q.repo.Update(ctx, job); err != nil {
		q.log.WithError(err).Error("Failed to update failed job")
	}
}

// scheduledJobPoller checks for scheduled jobs ready to run
func (q *queue) scheduledJobPoller() {
	defer q.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-q.ctx.Done():
			return
		case <-ticker.C:
			q.processScheduledJobs()
		}
	}
}

// processScheduledJobs moves scheduled jobs to the queue
func (q *queue) processScheduledJobs() {
	ctx, cancel := context.WithTimeout(q.ctx, 30*time.Second)
	defer cancel()

	jobs, err := q.repo.ListScheduled(ctx, time.Now().UTC())
	if err != nil {
		q.log.WithError(err).Error("Failed to list scheduled jobs")
		return
	}

	for _, job := range jobs {
		if err := job.Queue(); err != nil {
			q.log.WithError(err).WithField("job_id", job.ID).Error("Failed to queue scheduled job")
			continue
		}

		if err := q.repo.Update(ctx, job); err != nil {
			q.log.WithError(err).WithField("job_id", job.ID).Error("Failed to update scheduled job")
			continue
		}

		q.log.WithField("job_id", job.ID).Info("Scheduled job queued")
	}
}

// staleLockCleaner releases stale job locks
func (q *queue) staleLockCleaner() {
	defer q.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-q.ctx.Done():
			return
		case <-ticker.C:
			q.cleanStaleLocks()
		}
	}
}

// cleanStaleLocks releases locks that have expired
func (q *queue) cleanStaleLocks() {
	ctx, cancel := context.WithTimeout(q.ctx, 30*time.Second)
	defer cancel()

	jobs, err := q.repo.GetStaleJobs(ctx, q.config.StaleLockTimeout)
	if err != nil {
		q.log.WithError(err).Error("Failed to get stale jobs")
		return
	}

	for _, job := range jobs {
		job.Unlock()
		job.Status = types.StatusQueued // Return to queue

		if err := q.repo.Update(ctx, job); err != nil {
			q.log.WithError(err).WithField("job_id", job.ID).Error("Failed to unlock stale job")
			continue
		}

		q.log.WithField("job_id", job.ID).Warn("Released stale job lock")
	}
}

// retryHandler processes jobs that need to be retried
func (q *queue) retryHandler() {
	defer q.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-q.ctx.Done():
			return
		case <-ticker.C:
			// Scheduled retries are handled by scheduledJobPoller
			// This could be extended for additional retry logic
		}
	}
}

// ErrNoJobsAvailable is returned when no jobs are available for processing
var ErrNoJobsAvailable = errors.New("no jobs available")
