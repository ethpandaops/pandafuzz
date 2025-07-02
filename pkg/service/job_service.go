package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// jobService implements JobService interface
type jobService struct {
	state          StateStore
	timeoutManager TimeoutManager
	config         *common.MasterConfig
	logger         *logrus.Logger

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
}

// Compile-time interface compliance check
var _ JobService = (*jobService)(nil)

// NewJobService creates a new job service
func NewJobService(
	state StateStore,
	timeoutManager TimeoutManager,
	config *common.MasterConfig,
	logger *logrus.Logger,
) JobService {
	return &jobService{
		state:          state,
		timeoutManager: timeoutManager,
		config:         config,
		logger:         logger,
	}
}

// CreateJob creates a new job
func (s *jobService) CreateJob(ctx context.Context, req CreateJobRequest) (*common.Job, error) {
	// Validate request
	if req.Name == "" || req.Target == "" || req.Fuzzer == "" {
		return nil, errors.NewValidationError("create_job", "Name, target, and fuzzer are required")
	}

	// Validate fuzzer type
	validFuzzers := []string{"afl++", "libfuzzer", "honggfuzz"}
	isValid := false
	for _, valid := range validFuzzers {
		if req.Fuzzer == valid {
			isValid = true
			break
		}
	}
	if !isValid {
		return nil, errors.NewValidationError("create_job",
			fmt.Sprintf("Invalid fuzzer type. Must be one of: %v", validFuzzers))
	}

	// Create job
	jobID := uuid.New().String()
	now := time.Now()

	// Set default duration if not provided
	duration := req.Duration
	if duration == 0 {
		duration = s.config.Timeouts.JobExecution
	}

	job := &common.Job{
		ID:        jobID,
		Name:      req.Name,
		Target:    req.Target,
		Fuzzer:    req.Fuzzer,
		Status:    common.JobStatusPending,
		CreatedAt: now,
		TimeoutAt: now.Add(duration),
		WorkDir:   fmt.Sprintf("/tmp/pandafuzz/job_%s", jobID),
		Config:    req.Config,
		Progress:  0, // Initialize progress to 0
	}

	// Save job with context
	if err := s.state.SaveJobWithRetry(job); err != nil {
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "create_job", "Failed to save job", err)
	}

	// Set job timeout
	s.timeoutManager.SetJobTimeout(jobID, duration)

	s.logger.WithFields(logrus.Fields{
		"job_id":   jobID,
		"job_name": req.Name,
		"fuzzer":   req.Fuzzer,
		"target":   req.Target,
		"duration": duration,
	}).Info("Job created successfully")

	return job, nil
}

// GetJob retrieves a job by ID
func (s *jobService) GetJob(ctx context.Context, jobID string) (*common.Job, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("get_job", "Job ID is required")
	}

	// Use the provided context directly
	job, err := s.state.GetJob(jobID)
	if err != nil {
		if common.IsNotFoundError(err) {
			return nil, errors.NewNotFoundError("get_job", "job")
		}
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "get_job", "Failed to get job", err)
	}

	return job, nil
}

// ListJobs returns jobs with optional filters
func (s *jobService) ListJobs(ctx context.Context, filter JobFilter) ([]*common.Job, error) {
	// Try to use optimized filtered query if available
	if jobs, err := s.state.ListJobsFiltered(ctx, filter.Status, filter.Fuzzer, filter.Limit, filter.Page); err == nil {
		return jobs, nil
	} else if !errors.IsMethodNotFound(err) {
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "list_jobs", "Failed to list jobs", err)
	}

	// Fallback to in-memory filtering
	jobs, err := s.state.ListJobs()
	if err != nil {
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "list_jobs", "Failed to list jobs", err)
	}

	// Apply filters
	var filtered []*common.Job
	for _, job := range jobs {
		if filter.Status != nil && job.Status != *filter.Status {
			continue
		}
		if filter.Fuzzer != nil && job.Fuzzer != *filter.Fuzzer {
			continue
		}
		filtered = append(filtered, job)
	}

	// Apply pagination
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}

	start := (filter.Page - 1) * filter.Limit
	end := start + filter.Limit

	if start >= len(filtered) {
		return []*common.Job{}, nil
	}
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[start:end], nil
}

// AssignJob assigns a job to a bot
func (s *jobService) AssignJob(ctx context.Context, botID string) (*common.Job, error) {
	if botID == "" {
		return nil, errors.NewValidationError("assign_job", "Bot ID is required")
	}

	// Use optimized assignment method if available
	if job, err := s.state.AtomicJobAssignmentOptimized(ctx, botID); err == nil {
		// Set job timeout
		s.timeoutManager.UpdateJobTimeout(job.ID)

		s.logger.WithFields(logrus.Fields{
			"bot_id":   botID,
			"job_id":   job.ID,
			"job_name": job.Name,
			"fuzzer":   job.Fuzzer,
		}).Info("Job assigned to bot")

		return job, nil
	} else if !errors.IsMethodNotFound(err) {
		// Handle actual errors
		if errors.IsNotFoundError(err) {
			return nil, errors.New(errors.ErrorTypeNotFound, "assign_job", "No jobs available for assignment")
		}
		if errors.IsConflictError(err) {
			return nil, errors.Wrap(errors.ErrorTypeConflict, "assign_job", "Lock conflict during job assignment", err)
		}
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "assign_job", "Failed to assign job", err)
	}

	// Fallback to traditional method
	job, err := s.state.AtomicJobAssignmentWithRetry(botID)
	if err != nil {
		if err.Error() == "no jobs available" {
			return nil, errors.New(errors.ErrorTypeNotFound, "assign_job", "No jobs available for assignment")
		}
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "assign_job", "Failed to assign job", err)
	}

	// Set job timeout
	s.timeoutManager.UpdateJobTimeout(job.ID)

	s.logger.WithFields(logrus.Fields{
		"bot_id":   botID,
		"job_id":   job.ID,
		"job_name": job.Name,
		"fuzzer":   job.Fuzzer,
	}).Info("Job assigned to bot")

	return job, nil
}

// CompleteJob marks a job as completed
func (s *jobService) CompleteJob(ctx context.Context, jobID, botID string, success bool) error {
	if jobID == "" || botID == "" {
		return errors.NewValidationError("complete_job", "Job ID and Bot ID are required")
	}

	// Use optimized completion method if available
	if err := s.state.CompleteJobOptimized(ctx, jobID, botID, success); err == nil {
		// Remove job timeout
		s.timeoutManager.RemoveJobTimeout(jobID)

		s.logger.WithFields(logrus.Fields{
			"bot_id":  botID,
			"job_id":  jobID,
			"success": success,
		}).Info("Job completed")

		return nil
	} else if !errors.IsMethodNotFound(err) {
		// Handle actual errors
		if errors.IsValidationError(err) {
			return err // Pass through validation errors
		}
		if errors.IsConflictError(err) {
			return errors.Wrap(errors.ErrorTypeConflict, "complete_job", "Lock conflict during job completion", err)
		}
		return errors.Wrap(errors.ErrorTypeDatabase, "complete_job", "Failed to complete job", err)
	}

	// Fallback: Get bot's current job
	bot, err := s.state.GetBot(botID)
	if err != nil {
		return errors.Wrap(errors.ErrorTypeDatabase, "complete_job", "Failed to get bot", err)
	}

	if bot.CurrentJob == nil || *bot.CurrentJob != jobID {
		return errors.NewValidationError("complete_job", "Bot is not assigned to this job")
	}

	// Complete job
	if err := s.state.CompleteJobWithRetry(jobID, botID, success); err != nil {
		return errors.Wrap(errors.ErrorTypeDatabase, "complete_job", "Failed to complete job", err)
	}

	// Remove job timeout
	s.timeoutManager.RemoveJobTimeout(jobID)

	s.logger.WithFields(logrus.Fields{
		"bot_id":  botID,
		"job_id":  jobID,
		"success": success,
	}).Info("Job completed")

	return nil
}

// CancelJob cancels a job
func (s *jobService) CancelJob(ctx context.Context, jobID string) error {
	if jobID == "" {
		return errors.NewValidationError("cancel_job", "Job ID is required")
	}

	// Use the provided context directly
	job, err := s.state.GetJob(jobID)
	if err != nil {
		return errors.Wrap(errors.ErrorTypeDatabase, "cancel_job", "Failed to get job", err)
	}

	// Check if job can be cancelled
	if job.Status == common.JobStatusCompleted || job.Status == common.JobStatusFailed {
		return errors.NewValidationError("cancel_job", "Cannot cancel completed or failed job")
	}

	// Update job status
	job.Status = common.JobStatusCancelled
	now := time.Now()
	job.CompletedAt = &now

	// If job is assigned, free up the bot
	if job.AssignedBot != nil {
		if err := s.state.CompleteJobWithRetry(jobID, *job.AssignedBot, false); err != nil {
			return errors.Wrap(errors.ErrorTypeDatabase, "cancel_job", "Failed to cancel job", err)
		}
	} else {
		// Just update job status
		if err := s.state.SaveJobWithRetry(job); err != nil {
			return errors.Wrap(errors.ErrorTypeDatabase, "cancel_job", "Failed to save job", err)
		}
	}

	// Remove job timeout
	s.timeoutManager.RemoveJobTimeout(jobID)

	s.logger.WithField("job_id", jobID).Info("Job cancelled")
	return nil
}

// GetJobLogs retrieves logs for a job
func (s *jobService) GetJobLogs(ctx context.Context, jobID string) ([]string, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("get_job_logs", "Job ID is required")
	}

	// Verify job exists
	_, err := s.state.GetJob(jobID)
	if err != nil {
		if common.IsNotFoundError(err) {
			return nil, errors.NewNotFoundError("get_job_logs", "job")
		}
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "get_job_logs", "Failed to get job", err)
	}

	// TODO: Implement actual log retrieval from storage
	// For now, return placeholder logs
	logs := []string{
		fmt.Sprintf("[%s] Job %s started", time.Now().Format(time.RFC3339), jobID),
		fmt.Sprintf("[%s] Fuzzer initialized", time.Now().Format(time.RFC3339)),
		fmt.Sprintf("[%s] Fuzzing in progress...", time.Now().Format(time.RFC3339)),
	}

	return logs, nil
}

// Start starts the job service
func (s *jobService) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Start any background goroutines here
	// Currently job service doesn't have background tasks

	s.logger.Info("Job service started")
	return nil
}

// Stop stops the job service
func (s *jobService) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}

	// Clean up any resources
	// Currently job service doesn't have resources to clean up

	s.logger.Info("Job service stopped")
	return nil
}
