package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/scheduler"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
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
	corpusService  common.CorpusService
	queue          scheduler.Queue // Optional queue for asynq mode
	useAsynq       bool

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
	corpusService common.CorpusService,
) JobService {
	return &jobService{
		state:          state,
		timeoutManager: timeoutManager,
		config:         config,
		logger:         logger,
		corpusService:  corpusService,
		useAsynq:       config.Queue.Backend == "asynq",
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

	s.logger.WithFields(logrus.Fields{
		"job_id":               jobID,
		"req_duration":         req.Duration,
		"req_duration_seconds": req.Duration.Seconds(),
	}).Info("DEBUG: CreateJob called with duration")

	// Set default duration if not provided
	duration := req.Duration
	if duration == 0 {
		duration = s.config.Timeouts.JobExecution
	}

	// Handle campaign/corpus linking
	var campaignID *string
	var collectionID *string
	useCampaignCorpus := req.UseCampaignCorpus

	// If a corpus collection is specified
	if req.CollectionID != "" {
		collectionID = &req.CollectionID
	} else if req.CorpusID != "" {
		// If a standalone corpus is specified, treat it as a campaign
		campaignID = &req.CorpusID
		useCampaignCorpus = true
	} else if req.CampaignID != "" {
		campaignID = &req.CampaignID
		// Use campaign corpus if explicitly requested or by default
		if !req.UseCampaignCorpus {
			useCampaignCorpus = true // Default to using campaign corpus
		}
	}

	// Ensure duration is stored in config
	jobConfig := req.Config
	// Always use the top-level duration if provided
	if duration > 0 {
		jobConfig.Duration = duration
		s.logger.WithFields(logrus.Fields{
			"job_id":                 jobID,
			"duration":               duration,
			"duration_seconds":       duration.Seconds(),
			"config_duration_before": req.Config.Duration,
			"config_duration_after":  jobConfig.Duration,
		}).Info("Setting job config duration")
	} else {
		s.logger.WithFields(logrus.Fields{
			"job_id":           jobID,
			"request_duration": req.Duration,
			"duration_var":     duration,
			"config_duration":  req.Config.Duration,
		}).Warn("Duration is zero or not provided")
	}

	job := &common.Job{
		ID:                jobID,
		Name:              req.Name,
		Target:            req.Target,
		Fuzzer:            req.Fuzzer,
		Status:            common.JobStatusPending,
		CreatedAt:         now,
		TimeoutAt:         now.Add(duration),
		WorkDir:           fmt.Sprintf("job_%s", jobID), // Use relative path that bot will resolve
		Config:            jobConfig,
		Progress:          0, // Initialize progress to 0
		CampaignID:        campaignID,
		CollectionID:      collectionID,
		UseCampaignCorpus: useCampaignCorpus,
		Priority:          req.Priority,
		EnableCoverage:    req.EnableCoverage,
		CoverageFormat:    req.CoverageFormat,
	}

	// Save job with context
	if err := s.state.SaveJobWithRetry(job); err != nil {
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "create_job", "Failed to save job", err)
	}

	// If using asynq, enqueue the job
	if s.useAsynq && s.queue != nil {
		// Convert to domain job type for queue
		domainJob := &types.Job{
			ID:           job.ID,
			Name:         job.Name,
			FuzzerType:   job.Fuzzer,
			TargetBinary: job.Target,
			TargetArgs:   []string{}, // Would need to parse from config
			CorpusPath:   fmt.Sprintf("%s/corpus", job.WorkDir),
			OutputPath:   fmt.Sprintf("%s/outputs", job.WorkDir),
			FuzzerConfig: make(map[string]any),
			Priority:     convertPriorityToDomain(job.Priority),
			Status:       types.StatusQueued,
			MaxRetries:   3,
			RetryDelay:   30 * time.Second,
			MaxDuration:  duration,
			QueuedAt:     &now,
			CreatedAt:    now,
			UpdatedAt:    now,
			Metadata: map[string]string{
				"fuzzer":   job.Fuzzer,
				"target":   job.Target,
				"work_dir": job.WorkDir,
			},
		}

		if campaignID != nil {
			domainJob.Metadata["campaign_id"] = *campaignID
		}
		if collectionID != nil {
			domainJob.Metadata["collection_id"] = *collectionID
		}
		domainJob.Metadata["use_campaign_corpus"] = fmt.Sprintf("%v", useCampaignCorpus)

		// Enqueue to asynq
		if err := s.queue.Enqueue(ctx, domainJob); err != nil {
			// Log error but don't fail - job is already saved in DB
			s.logger.WithError(err).WithField("job_id", jobID).Error("Failed to enqueue job to asynq")
		} else {
			s.logger.WithField("job_id", jobID).Info("Job enqueued to asynq")
		}
	}

	// Link job to campaign/corpus if provided
	linkID := ""
	if req.CorpusID != "" {
		linkID = req.CorpusID
	} else if req.CampaignID != "" {
		linkID = req.CampaignID
	}

	if linkID != "" && s.corpusService != nil {
		if err := s.corpusService.LinkJobCorpus(ctx, jobID, linkID); err != nil {
			// Log error but don't fail job creation
			s.logger.WithFields(logrus.Fields{
				"job_id":    jobID,
				"link_id":   linkID,
				"is_corpus": req.CorpusID != "",
				"error":     err,
			}).Warn("Failed to link job to campaign/corpus")
		}
	}

	// Set job timeout
	s.timeoutManager.SetJobTimeout(jobID, duration)

	logFields := logrus.Fields{
		"job_id":   jobID,
		"job_name": req.Name,
		"fuzzer":   req.Fuzzer,
		"target":   req.Target,
		"duration": duration,
	}
	if req.CollectionID != "" {
		logFields["collection_id"] = req.CollectionID
		logFields["use_collection"] = true
	} else if req.CorpusID != "" {
		logFields["corpus_id"] = req.CorpusID
		logFields["use_corpus"] = true
	} else if req.CampaignID != "" {
		logFields["campaign_id"] = req.CampaignID
		logFields["use_campaign_corpus"] = useCampaignCorpus
	}
	s.logger.WithFields(logFields).Info("Job created successfully")

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

	// If using asynq, jobs are pulled by workers, not assigned
	if s.useAsynq {
		return nil, errors.New(errors.ErrorTypeNotFound, "assign_job", "Job assignment not available in queue mode - bots should run as workers")
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

// AssignNextJob assigns the next available job to a bot
func (s *jobService) AssignNextJob(ctx context.Context, botID string) (*common.Job, error) {
	// This is just an alias for AssignJob for compatibility
	return s.AssignJob(ctx, botID)
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

	// If using asynq and queue is set, start it
	if s.useAsynq && s.queue != nil {
		if err := s.queue.Start(ctx); err != nil {
			return errors.Wrap(errors.ErrorTypeSystem, "start_job_service", "Failed to start queue", err)
		}
		s.logger.Info("Asynq queue started")
	}

	s.logger.Info("Job service started")
	return nil
}

// GetJobCorpus retrieves corpus files for a job
func (s *jobService) GetJobCorpus(ctx context.Context, jobID string) ([]*common.CorpusFile, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("get_job_corpus", "Job ID is required")
	}

	// Verify job exists
	_, err := s.state.GetJob(jobID)
	if err != nil {
		if common.IsNotFoundError(err) {
			return nil, errors.NewNotFoundError("get_job_corpus", "job")
		}
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "get_job_corpus", "Failed to get job", err)
	}

	// Check if corpus service is available
	if s.corpusService == nil {
		return nil, errors.New(errors.ErrorTypeSystem, "get_job_corpus", "Corpus service not available")
	}

	// Delegate to corpus service
	corpusFiles, err := s.corpusService.GetCorpusForJob(ctx, jobID)
	if err != nil {
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "get_job_corpus", "Failed to get corpus files", err)
	}

	s.logger.WithFields(logrus.Fields{
		"job_id":     jobID,
		"file_count": len(corpusFiles),
	}).Debug("Retrieved corpus files for job")

	return corpusFiles, nil
}

// Stop stops the job service
func (s *jobService) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}

	// Stop the queue if using asynq
	if s.useAsynq && s.queue != nil {
		if err := s.queue.Stop(); err != nil {
			s.logger.WithError(err).Error("Failed to stop queue cleanly")
		}
	}

	s.logger.Info("Job service stopped")
	return nil
}

// StreamLogs streams job logs
func (s *jobService) StreamLogs(ctx context.Context, jobID string) (<-chan string, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("stream_logs", "Job ID is required")
	}

	// Create a channel for streaming logs
	logsChan := make(chan string, 100)

	// TODO: Implement actual log streaming from storage or log files
	// For now, return a channel that closes immediately
	close(logsChan)

	return logsChan, nil
}

// GetLogs retrieves job logs
func (s *jobService) GetLogs(ctx context.Context, jobID string) ([]string, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("get_logs", "Job ID is required")
	}

	// TODO: Implement actual log retrieval from storage
	// For now, return empty logs
	return []string{}, nil
}

// GetJobStats retrieves statistics for a job
func (s *jobService) GetJobStats(ctx context.Context, jobID string) (*JobStats, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("get_job_stats", "Job ID is required")
	}

	// Get job details
	job, err := s.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}

	// Get crashes for this job
	crashes, err := s.GetJobCrashes(ctx, jobID)
	if err != nil {
		return nil, err
	}

	// Get corpus files
	corpusFiles, err := s.GetJobCorpus(ctx, jobID)
	if err != nil {
		return nil, err
	}

	// Calculate statistics
	stats := &JobStats{
		JobID:         jobID,
		CrashesFound:  len(crashes),
		UniqueCrashes: len(crashes), // TODO: Implement proper deduplication
		CorpusSize:    len(corpusFiles),
		StartTime:     job.CreatedAt,
	}

	if job.StartedAt != nil {
		stats.StartTime = *job.StartedAt
	}

	if job.CompletedAt != nil {
		stats.EndTime = job.CompletedAt
		stats.Duration = stats.EndTime.Sub(stats.StartTime)
	} else if job.Status == common.JobStatusRunning {
		stats.Duration = time.Since(stats.StartTime)
	}

	// TODO: Get actual coverage and execution metrics from fuzzer
	stats.CoveragePercent = 0.0
	stats.ExecutionsTotal = 0
	stats.ExecutionsPerSec = 0.0

	return stats, nil
}

// GetJobCrashes retrieves crashes for a job
func (s *jobService) GetJobCrashes(ctx context.Context, jobID string) ([]*common.CrashResult, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("get_job_crashes", "Job ID is required")
	}

	// TODO: Implement crash retrieval
	// This requires access to the storage interface which is not exposed through StateStore
	// For now, return empty list
	return []*common.CrashResult{}, nil
}

// GetQueueStats returns queue statistics (for asynq mode)
func (s *jobService) GetQueueStats(ctx context.Context) (*QueueStats, error) {
	if !s.useAsynq || s.queue == nil {
		return nil, errors.New(errors.ErrorTypeNotFound, "get_queue_stats", "Queue statistics not available in polling mode")
	}

	stats := s.queue.GetStats()

	// Convert scheduler.QueueStats to service.QueueStats
	return &QueueStats{
		TotalJobs:       int(stats.TotalJobs),
		PendingJobs:     int(stats.QueueDepth),
		RunningJobs:     stats.WorkersActive,
		CompletedJobs:   int(stats.ProcessedCount),
		FailedJobs:      int(stats.FailedCount),
		EnqueuedCount:   int(stats.EnqueuedCount),
		ProcessedCount:  int(stats.ProcessedCount),
		FailedCount:     int(stats.FailedCount),
		RetryCount:      int(stats.RetryCount),
		AverageWaitTime: stats.AverageWaitTime,
		AverageExecTime: stats.AverageExecTime,
		WorkersActive:   stats.WorkersActive,
		WorkersTotal:    stats.WorkersTotal,
		LastProcessedAt: stats.LastProcessedAt,
	}, nil
}

// SetQueue sets the queue instance (called during initialization)
func (s *jobService) SetQueue(queue scheduler.Queue) {
	s.queue = queue
}

// Helper functions

func convertPriorityToDomain(priority int) types.JobPriority {
	switch {
	case priority >= 90:
		return types.PriorityCritical
	case priority >= 50:
		return types.PriorityHigh
	case priority >= 20:
		return types.PriorityNormal
	case priority >= 10:
		return types.PriorityLow
	default:
		return types.PriorityLow
	}
}
