package service

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	jobRepo        JobRepository
	botRepo        BotRepository
	crashRepo      CrashRepository
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

// NewJobService creates a new job service using repository interfaces.
func NewJobService(
	jobRepo JobRepository,
	botRepo BotRepository,
	crashRepo CrashRepository,
	timeoutManager TimeoutManager,
	config *common.MasterConfig,
	logger *logrus.Logger,
	corpusService common.CorpusService,
) JobService {
	return &jobService{
		jobRepo:        jobRepo,
		botRepo:        botRepo,
		crashRepo:      crashRepo,
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

	// Security: Validate job name to prevent injection attacks
	if err := common.ValidateJobName(req.Name); err != nil {
		return nil, errors.NewValidationError("create_job", err.Error())
	}

	// Security: Validate target binary path
	if common.IsPathTraversal(req.Target) {
		return nil, errors.NewValidationError("create_job", "path traversal detected in target binary path")
	}

	// Validate fuzzer type
	validFuzzers := []string{"aflplusplus", "afl++", "libfuzzer", "honggfuzz"}
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

	// Debug log to verify coverage settings
	s.logger.WithFields(logrus.Fields{
		"job_id":              jobID,
		"enable_coverage":     req.EnableCoverage,
		"coverage_format":     req.CoverageFormat,
		"job_enable_coverage": job.EnableCoverage,
		"job_coverage_format": job.CoverageFormat,
	}).Info("DEBUG: Creating job with coverage settings")

	// Save job using repository
	if err := s.jobRepo.Create(ctx, job); err != nil {
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

	job, err := s.jobRepo.Get(ctx, jobID)
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
	// Calculate offset from page
	offset := 0
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if filter.Page > 0 {
		offset = (filter.Page - 1) * limit
	}

	// Try to use filtered query
	jobs, err := s.jobRepo.List(ctx, filter.Status, filter.Fuzzer, limit, offset)
	if err == nil {
		return jobs, nil
	}

	// Fallback to listing all and filtering in memory
	allJobs, listErr := s.jobRepo.ListAll(ctx)
	if listErr != nil {
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "list_jobs", "Failed to list jobs", listErr)
	}

	// Apply filters
	var filtered []*common.Job
	for _, job := range allJobs {
		if filter.Status != nil && job.Status != *filter.Status {
			continue
		}
		if filter.Fuzzer != nil && job.Fuzzer != *filter.Fuzzer {
			continue
		}
		filtered = append(filtered, job)
	}

	// Apply pagination
	start := offset
	end := start + limit

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

	// Use atomic assignment from repository
	job, err := s.jobRepo.AtomicAssign(ctx, botID)
	if err != nil {
		if errors.IsNotFoundError(err) || err.Error() == "no jobs available" {
			return nil, errors.New(errors.ErrorTypeNotFound, "assign_job", "No jobs available for assignment")
		}
		if errors.IsConflictError(err) {
			return nil, errors.Wrap(errors.ErrorTypeConflict, "assign_job", "Lock conflict during job assignment", err)
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

	// Use repository completion method
	if err := s.jobRepo.Complete(ctx, jobID, botID, success); err != nil {
		if errors.IsValidationError(err) {
			return err // Pass through validation errors
		}
		if errors.IsConflictError(err) {
			return errors.Wrap(errors.ErrorTypeConflict, "complete_job", "Lock conflict during job completion", err)
		}
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

	job, err := s.jobRepo.Get(ctx, jobID)
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
	botID := job.AssignedBot
	job.AssignedBot = nil // Unassign the job

	// Save the job with cancelled status
	if err := s.jobRepo.Update(ctx, job); err != nil {
		return errors.Wrap(errors.ErrorTypeDatabase, "cancel_job", "Failed to save job", err)
	}

	// If job was assigned, free up the bot
	if botID != nil && s.botRepo != nil {
		bot, err := s.botRepo.Get(ctx, *botID)
		if err == nil && bot != nil {
			bot.Status = common.BotStatusIdle
			bot.CurrentJob = nil
			if err := s.botRepo.Update(ctx, bot); err != nil {
				s.logger.WithError(err).WithField("bot_id", *botID).Warn("Failed to free up bot after job cancellation")
			}
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

	// Get job to find its work directory
	job, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		if common.IsNotFoundError(err) {
			return nil, errors.NewNotFoundError("get_job_logs", "job")
		}
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "get_job_logs", "Failed to get job", err)
	}

	// Read logs from the job's work directory
	return s.readJobLogFiles(job.WorkDir, jobID)
}

// readJobLogFiles reads log files from a job's work directory
func (s *jobService) readJobLogFiles(workDir, jobID string) ([]string, error) {
	if workDir == "" {
		return []string{fmt.Sprintf("[INFO] No work directory configured for job %s", jobID)}, nil
	}

	var allLogs []string

	// Common log file names to look for
	logFiles := []string{
		"fuzzer.log",
		"output.log",
		"stderr.log",
		"stdout.log",
		"afl.log",
		"libfuzzer.log",
	}

	for _, logFile := range logFiles {
		logPath := filepath.Join(workDir, logFile)
		if _, err := os.Stat(logPath); os.IsNotExist(err) {
			continue
		}

		file, err := os.Open(logPath)
		if err != nil {
			s.logger.WithError(err).WithField("path", logPath).Debug("Failed to open log file")
			continue
		}

		scanner := bufio.NewScanner(file)
		// Limit to last 1000 lines to avoid memory issues
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
			if len(lines) > 1000 {
				lines = lines[1:]
			}
		}
		file.Close()

		if err := scanner.Err(); err != nil {
			s.logger.WithError(err).WithField("path", logPath).Debug("Error reading log file")
			continue
		}

		// Prefix lines with the log file name for clarity
		for _, line := range lines {
			allLogs = append(allLogs, fmt.Sprintf("[%s] %s", strings.TrimSuffix(logFile, ".log"), line))
		}
	}

	if len(allLogs) == 0 {
		return []string{fmt.Sprintf("[INFO] No log files found in %s", workDir)}, nil
	}

	return allLogs, nil
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
	_, err := s.jobRepo.Get(ctx, jobID)
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

// StreamLogs streams job logs (tails the log file)
func (s *jobService) StreamLogs(ctx context.Context, jobID string) (<-chan string, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("stream_logs", "Job ID is required")
	}

	// Get job to find its work directory
	job, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		if common.IsNotFoundError(err) {
			return nil, errors.NewNotFoundError("stream_logs", "job")
		}
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "stream_logs", "Failed to get job", err)
	}

	// Create a channel for streaming logs
	logsChan := make(chan string, 100)

	if job.WorkDir == "" {
		// No work directory, close immediately
		close(logsChan)
		return logsChan, nil
	}

	// Find the primary log file
	logPath := s.findPrimaryLogFile(job.WorkDir)
	if logPath == "" {
		// No log file found, close immediately
		close(logsChan)
		return logsChan, nil
	}

	// Start a goroutine to tail the log file
	go func() {
		defer close(logsChan)
		s.tailLogFile(ctx, logPath, logsChan)
	}()

	return logsChan, nil
}

// findPrimaryLogFile finds the primary log file in a work directory
func (s *jobService) findPrimaryLogFile(workDir string) string {
	logFiles := []string{"fuzzer.log", "output.log", "stdout.log", "afl.log", "libfuzzer.log"}
	for _, logFile := range logFiles {
		logPath := filepath.Join(workDir, logFile)
		if _, err := os.Stat(logPath); err == nil {
			return logPath
		}
	}
	return ""
}

// tailLogFile tails a log file and sends new lines to the channel
func (s *jobService) tailLogFile(ctx context.Context, logPath string, logsChan chan<- string) {
	file, err := os.Open(logPath)
	if err != nil {
		s.logger.WithError(err).WithField("path", logPath).Error("Failed to open log file for tailing")
		return
	}
	defer file.Close()

	// Seek to end of file
	_, err = file.Seek(0, 2)
	if err != nil {
		s.logger.WithError(err).WithField("path", logPath).Error("Failed to seek to end of log file")
		return
	}

	reader := bufio.NewReader(file)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					break // No more data available
				}
				line = strings.TrimSpace(line)
				if line != "" {
					select {
					case logsChan <- line:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}
}

// GetLogs retrieves job logs (alias for GetJobLogs)
func (s *jobService) GetLogs(ctx context.Context, jobID string) ([]string, error) {
	return s.GetJobLogs(ctx, jobID)
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

	// Calculate statistics with proper deduplication based on crash hash
	uniqueHashes := make(map[string]struct{})
	for _, crash := range crashes {
		if crash.Hash != "" {
			uniqueHashes[crash.Hash] = struct{}{}
		} else {
			// If no hash, treat each crash as unique
			uniqueHashes[crash.ID] = struct{}{}
		}
	}

	stats := &JobStats{
		JobID:         jobID,
		CrashesFound:  len(crashes),
		UniqueCrashes: len(uniqueHashes),
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

	// Get coverage and execution metrics from the latest coverage result
	stats.CoveragePercent = 0.0
	stats.ExecutionsTotal = 0
	stats.ExecutionsPerSec = 0.0

	// Query coverage history to get the latest metrics
	// Use a wide time range to ensure we get all coverage data
	startTime := job.CreatedAt.Add(-time.Hour) // Buffer for any timing issues
	endTime := time.Now().Add(time.Hour)

	coverageHistory, err := s.jobRepo.GetCoverageHistory(ctx, jobID, startTime, endTime)
	if err != nil {
		s.logger.WithError(err).WithField("job_id", jobID).Debug("Failed to get coverage history")
	} else if len(coverageHistory) > 0 {
		// Use the latest coverage result
		latest := coverageHistory[len(coverageHistory)-1]
		stats.ExecutionsTotal = latest.ExecCount

		// Calculate executions per second if we have duration
		if stats.Duration > 0 {
			stats.ExecutionsPerSec = float64(stats.ExecutionsTotal) / stats.Duration.Seconds()
		}

		// Calculate coverage percentage from edges if available
		// Coverage is approximated as new edges / total edges (if we have baseline)
		if latest.Edges > 0 {
			stats.CoveragePercent = float64(latest.NewEdges) / float64(latest.Edges) * 100.0
		}
	}

	return stats, nil
}

// GetJobCrashes retrieves crashes for a job
func (s *jobService) GetJobCrashes(ctx context.Context, jobID string) ([]*common.CrashResult, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("get_job_crashes", "Job ID is required")
	}

	// Use crash repository
	crashes, err := s.crashRepo.ListByJob(ctx, jobID)
	if err != nil {
		// Log but don't fail - crashes may not be available in all storage backends
		s.logger.WithError(err).WithField("job_id", jobID).Debug("Failed to get job crashes")
		return []*common.CrashResult{}, nil
	}

	return crashes, nil
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

// UpdateJob updates an existing job
func (s *jobService) UpdateJob(ctx context.Context, job *common.Job) error {
	if job == nil {
		return errors.NewValidationError("update_job", "job cannot be nil")
	}

	return s.jobRepo.Update(ctx, job)
}

// CancelJobExecution cancels a running job and signals the executor
// This is different from CancelJob which just updates the status
func (s *jobService) CancelJobExecution(ctx context.Context, jobID string) error {
	job, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		return err
	}

	// Only cancel if job is running
	if job.Status != common.JobStatusRunning {
		return errors.NewValidationError("cancel_job_execution", "job is not running")
	}

	// Update job status first
	job.Status = common.JobStatusCancelled
	now := time.Now()
	job.CompletedAt = &now

	if err := s.jobRepo.Update(ctx, job); err != nil {
		return err
	}

	// Note: Executor cancellation would be handled by the caller or through
	// event-based notification. The service doesn't directly call the executor.
	s.logger.WithField("job_id", jobID).Info("Job execution canceled")

	return nil
}

// GetJobLogsPaginated retrieves paginated logs for a job
func (s *jobService) GetJobLogsPaginated(ctx context.Context, jobID string, limit, offset int) ([]string, int, error) {
	// Validate job exists
	_, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		return nil, 0, err
	}

	// Use repository to get paginated logs
	logs, total, err := s.jobRepo.GetLogs(ctx, jobID, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// StoreJobLogs stores logs for a job
func (s *jobService) StoreJobLogs(ctx context.Context, jobID string, logs []string) error {
	// Validate job exists
	_, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		return err
	}

	return s.jobRepo.StoreLogs(ctx, jobID, logs)
}

// GetJobCrashesPaginated retrieves paginated crashes for a job
func (s *jobService) GetJobCrashesPaginated(ctx context.Context, jobID string, limit, offset int) ([]*common.CrashResult, int, error) {
	// Validate job exists
	_, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		return nil, 0, err
	}

	crashes, err := s.crashRepo.List(ctx, jobID, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	// Get total count (this is inefficient but works for now)
	total := len(crashes)
	if total == limit {
		// If we got exactly limit results, there might be more
		allCrashes, _ := s.crashRepo.List(ctx, jobID, 10000, 0)
		total = len(allCrashes)
	}

	return crashes, total, nil
}

// GetCoverageReport retrieves the coverage report for a job
func (s *jobService) GetCoverageReport(ctx context.Context, jobID string) (*CoverageReport, error) {
	job, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}

	// Check if coverage is enabled
	if !job.EnableCoverage {
		return nil, errors.New(errors.ErrorTypeNotFound, "get_coverage_report", "coverage not enabled for this job")
	}

	// Get coverage data from repository
	coverageData, err := s.jobRepo.GetCoverageData(ctx, jobID)
	if err != nil {
		return nil, err
	}

	return &CoverageReport{
		JobID:            jobID,
		Format:           job.CoverageFormat,
		TotalLines:       coverageData.TotalLines,
		CoveredLines:     coverageData.CoveredLines,
		CoveragePercent:  float64(coverageData.CoveredLines) / float64(max(coverageData.TotalLines, 1)) * 100,
		TotalFunctions:   coverageData.TotalFunctions,
		CoveredFunctions: coverageData.CoveredFunctions,
		TotalBranches:    coverageData.TotalBranches,
		CoveredBranches:  coverageData.CoveredBranches,
		Files:            []string{}, // Would be populated from file listing
		GeneratedAt:      time.Now(),
	}, nil
}

// GetCoverageFile retrieves a specific coverage file
func (s *jobService) GetCoverageFile(ctx context.Context, jobID string, filePath string) ([]byte, error) {
	// Validate job exists and has coverage enabled
	job, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}

	if !job.EnableCoverage {
		return nil, errors.New(errors.ErrorTypeNotFound, "get_coverage_file", "coverage not enabled for this job")
	}

	// Security: Validate file path to prevent path traversal
	if common.IsPathTraversal(filePath) {
		return nil, errors.NewValidationError("get_coverage_file", "path traversal detected")
	}

	// Build full path to coverage file
	coveragePath := filepath.Join(job.WorkDir, "coverage", filePath)

	// Read file
	data, err := os.ReadFile(coveragePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New(errors.ErrorTypeNotFound, "get_coverage_file", "coverage file not found")
		}
		return nil, err
	}

	return data, nil
}

// StoreCoverageFile stores a coverage file for a job
func (s *jobService) StoreCoverageFile(ctx context.Context, jobID string, filePath string, data []byte) error {
	// Validate job exists
	job, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		return err
	}

	// Security: Validate file path to prevent path traversal
	if common.IsPathTraversal(filePath) {
		return errors.NewValidationError("store_coverage_file", "path traversal detected")
	}

	// Build full path to coverage file
	coveragePath := filepath.Join(job.WorkDir, "coverage", filePath)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(coveragePath), 0755); err != nil {
		return err
	}

	// Write file with secure permissions
	return os.WriteFile(coveragePath, data, 0600)
}

// ListCoverageFiles lists all coverage files for a job
func (s *jobService) ListCoverageFiles(ctx context.Context, jobID string) ([]string, error) {
	// Validate job exists and has coverage enabled
	job, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}

	if !job.EnableCoverage {
		return nil, errors.New(errors.ErrorTypeNotFound, "list_coverage_files", "coverage not enabled for this job")
	}

	// Build path to coverage directory
	coverageDir := filepath.Join(job.WorkDir, "coverage")

	// List files
	var files []string
	err = filepath.WalkDir(coverageDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			relPath, _ := filepath.Rel(coverageDir, path)
			files = append(files, relPath)
		}
		return nil
	})

	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	return files, nil
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
