package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/config"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	queueasynq "github.com/ethpandaops/pandafuzz/pkg/infrastructure/queue/asynq"
	"github.com/sirupsen/logrus"
)

// AsynqQueue implements the Queue interface using asynq
type AsynqQueue struct {
	client    *queueasynq.Client
	repo      repository.JobRepository
	config    *Config
	log       logrus.FieldLogger
	processor JobProcessor
}

// NewAsynqQueue creates a new asynq-based queue implementation
func NewAsynqQueue(cfg Config, repo repository.JobRepository, processor JobProcessor, log logrus.FieldLogger) (*AsynqQueue, error) {
	// Create Redis config
	redisCfg := &config.RedisConfig{
		Host:         "localhost",
		Port:         6379,
		Password:     "",
		DB:           0,
		PoolSize:     10,
		MinIdleConns: 5,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		IdleTimeout:  5 * time.Minute,
		MaxConnAge:   30 * time.Minute,
	}

	// Create queue config
	queueCfg := &config.QueueConfig{
		Backend:     "asynq",
		Concurrency: cfg.Workers,
		Queues: map[string]int{
			queueasynq.QueueCritical: 6,
			queueasynq.QueueDefault:  3,
			queueasynq.QueueLow:      1,
		},
		StrictPriority: cfg.EnablePriority,
		Retry: config.RetryConfig{
			MaxRetries:         cfg.MaxRetries,
			RetryDelay:         cfg.RetryDelay,
			MaxRetryDelay:      10 * time.Minute,
			ExponentialBackoff: true,
			RetryOnFailure:     true,
		},
		HealthCheckInterval: 30 * time.Second,
		ShutdownTimeout:     30 * time.Second,
		LogLevel:            "info",
	}

	// Create asynq client
	client, err := queueasynq.NewClient(redisCfg, queueCfg, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create asynq client: %w", err)
	}

	return &AsynqQueue{
		client:    client,
		repo:      repo,
		config:    &cfg,
		log:       log.WithField("component", "asynq-queue"),
		processor: processor,
	}, nil
}

// Start begins processing jobs from the queue
func (q *AsynqQueue) Start(ctx context.Context) error {
	q.log.Info("Asynq queue started (client-only mode)")
	// In asynq architecture, the server (worker) is separate from the client
	// This queue implementation is client-only for enqueueing
	// Workers will be started separately
	return nil
}

// Stop gracefully shuts down the queue
func (q *AsynqQueue) Stop() error {
	q.log.Info("Stopping asynq queue...")
	if err := q.client.Close(); err != nil {
		return fmt.Errorf("failed to close asynq client: %w", err)
	}
	q.log.Info("Asynq queue stopped")
	return nil
}

// Enqueue adds a job to the queue
func (q *AsynqQueue) Enqueue(ctx context.Context, job *types.Job) error {
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

	// Persist to repository first
	if err := q.repo.Create(ctx, job); err != nil {
		return fmt.Errorf("failed to persist job: %w", err)
	}

	// Enqueue to asynq
	if err := q.client.EnqueueJob(job); err != nil {
		// Try to update job status to failed
		job.Status = types.StatusFailed
		job.ErrorMessage = fmt.Sprintf("Failed to enqueue: %v", err)
		q.repo.Update(ctx, job)

		return fmt.Errorf("failed to enqueue job to asynq: %w", err)
	}

	q.log.WithFields(logrus.Fields{
		"job_id":   job.ID,
		"priority": job.Priority,
		"name":     job.Name,
	}).Info("Job enqueued to asynq")

	return nil
}

// EnqueueWithDelay schedules a job for future execution
func (q *AsynqQueue) EnqueueWithDelay(ctx context.Context, job *types.Job, delay time.Duration) error {
	scheduledAt := time.Now().UTC().Add(delay)
	job.ScheduledAt = &scheduledAt
	return q.Enqueue(ctx, job)
}

// Cancel removes a job from the queue
func (q *AsynqQueue) Cancel(ctx context.Context, jobID string) error {
	job, err := q.repo.Get(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	if job.IsFinished() {
		return fmt.Errorf("cannot cancel finished job: %s", job.Status)
	}

	// Cancel in asynq
	if err := q.client.CancelTask(jobID); err != nil {
		q.log.WithError(err).WithField("job_id", jobID).Warn("Failed to cancel task in asynq")
		// Continue anyway - mark as cancelled in DB
	}

	// Update job status
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
func (q *AsynqQueue) GetStats() QueueStats {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get stats from asynq
	asynqStats, err := q.client.GetAllQueueStats()
	if err != nil {
		q.log.WithError(err).Warn("Failed to get asynq stats")
		// Return empty stats on error
		return QueueStats{}
	}

	stats := QueueStats{
		EnqueuedCount:  asynqStats.Processed + asynqStats.Pending + asynqStats.Active,
		ProcessedCount: asynqStats.Processed,
		FailedCount:    asynqStats.Failed,
		RetryCount:     asynqStats.Retry,
		WorkersTotal:   q.config.Workers,
		WorkersActive:  int(asynqStats.Active),
		TotalJobs:      asynqStats.Pending + asynqStats.Active + asynqStats.Scheduled + asynqStats.Retry + asynqStats.Archived,
		QueueDepth:     int(asynqStats.Pending),
	}

	// Get job counts by status from repository
	if counts, err := q.repo.CountByStatus(ctx); err == nil {
		stats.JobsByStatus = counts
	}

	return stats
}

// GetJob retrieves job information
func (q *AsynqQueue) GetJob(ctx context.Context, jobID string) (*types.Job, error) {
	return q.repo.Get(ctx, jobID)
}

// ListJobs lists jobs matching the filter
func (q *AsynqQueue) ListJobs(ctx context.Context, filter repository.JobFilter) ([]*types.Job, error) {
	return q.repo.List(ctx, filter)
}

// SetRedisConfig allows updating Redis configuration.
// Dynamic reconfiguration is not supported - the queue must be recreated with new config.
func (q *AsynqQueue) SetRedisConfig(cfg *config.RedisConfig) error {
	return fmt.Errorf("dynamic Redis config update not supported: restart required")
}

// SetQueueConfig allows updating queue configuration.
// Dynamic reconfiguration is not supported - the queue must be recreated with new config.
func (q *AsynqQueue) SetQueueConfig(cfg *config.QueueConfig) error {
	return fmt.Errorf("dynamic queue config update not supported: restart required")
}

// Ping checks if Redis is reachable
func (q *AsynqQueue) Ping() error {
	return q.client.Ping()
}
