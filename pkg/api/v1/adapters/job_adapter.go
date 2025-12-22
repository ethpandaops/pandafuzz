package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/sse"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/executor"
	jobRepo "github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	jobTypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/ethpandaops/pandafuzz/pkg/service"
)

// JobAdapter implements the job-related endpoints of the generated ServerInterface
type JobAdapter struct {
	repository jobRepo.JobRepository
	executor   executor.Executor
	jobService service.JobService
	sse        *sse.Manager
	logger     logrus.FieldLogger
}

// NewJobAdapter creates a new job adapter
func NewJobAdapter(
	repository jobRepo.JobRepository,
	executor executor.Executor,
	jobService service.JobService,
	sse *sse.Manager,
	logger logrus.FieldLogger,
) *JobAdapter {
	return &JobAdapter{
		repository: repository,
		executor:   executor,
		jobService: jobService,
		sse:        sse,
		logger:     logger.WithField("component", "job_adapter"),
	}
}

// ListJobs retrieves jobs with filtering and pagination
func (a *JobAdapter) ListJobs(w http.ResponseWriter, r *http.Request, params generated.ListJobsParams) {
	ctx := r.Context()

	// Check dependencies
	if a.jobService == nil && a.repository == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Job service not configured", nil)
		return
	}

	// Build filter from parameters
	limit := 50
	offset := 0

	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
		if limit > 1000 {
			limit = 1000
		}
	}

	if params.Offset != nil && *params.Offset >= 0 {
		offset = *params.Offset
	}

	// Use jobService if available
	if a.jobService != nil {
		// Build service filter - service uses 1-based page numbers
		serviceFilter := service.JobFilter{
			Page:  (offset / limit) + 1,
			Limit: limit,
		}

		if params.Status != nil {
			status := generatedToCommonJobStatus(*params.Status)
			serviceFilter.Status = &status
		}

		if params.Fuzzer != nil {
			fuzzer := string(*params.Fuzzer)
			serviceFilter.Fuzzer = &fuzzer
		}

		// Get jobs from service
		commonJobs, err := a.jobService.ListJobs(ctx, serviceFilter)
		if err != nil {
			a.logger.WithError(err).Error("failed to list jobs")
			a.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve jobs", err)
			return
		}

		// Convert to API types
		apiJobs := make([]generated.Job, len(commonJobs))
		for i, job := range commonJobs {
			apiJobs[i] = a.convertCommonJobToAPI(job)
		}

		// Create pagination info
		pagination := generated.Pagination{
			Limit:   limit,
			Offset:  offset,
			Total:   len(apiJobs),
			HasMore: len(apiJobs) == limit,
		}

		response := generated.JobListResponse{
			Data:       apiJobs,
			Pagination: pagination,
		}

		a.writeJSONResponse(w, http.StatusOK, response)
		return
	}

	// Fallback to repository
	filter := jobRepo.JobFilter{
		Limit:  limit,
		Offset: offset,
	}

	if params.Status != nil {
		domainStatus := generatedJobStatusToDomain(*params.Status)
		filter.Status = &domainStatus
	}

	if params.Fuzzer != nil {
		fuzzerType := string(*params.Fuzzer)
		filter.FuzzerType = &fuzzerType
	}

	// Get jobs from repository
	jobs, err := a.repository.List(ctx, filter)
	if err != nil {
		a.logger.WithError(err).Error("failed to list jobs")
		a.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve jobs", err)
		return
	}

	// Convert to API types
	apiJobs := make([]generated.Job, len(jobs))
	for i, job := range jobs {
		apiJobs[i] = a.convertJobToAPI(job)
	}

	// Create pagination info
	pagination := generated.Pagination{
		Limit:   limit,
		Offset:  offset,
		Total:   len(apiJobs),
		HasMore: len(apiJobs) == limit,
	}

	response := generated.JobListResponse{
		Data:       apiJobs,
		Pagination: pagination,
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// CreateJob creates a new fuzzing job
func (a *JobAdapter) CreateJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check dependencies - prefer jobService
	if a.jobService == nil && a.repository == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Job service not configured", nil)
		return
	}

	var req generated.JobCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Use jobService if available
	if a.jobService != nil {
		// Build service request
		serviceReq := service.CreateJobRequest{
			Name:   req.Name,
			Target: req.TargetBinary,
			Fuzzer: string(req.Fuzzer),
		}

		if req.Priority != nil {
			serviceReq.Priority = *req.Priority
		}

		if req.TimeoutSeconds != nil {
			serviceReq.Duration = time.Duration(*req.TimeoutSeconds) * time.Second
		}

		if req.EnableCoverage != nil {
			serviceReq.EnableCoverage = *req.EnableCoverage
		}

		if req.CampaignId != nil {
			serviceReq.CampaignID = req.CampaignId.String()
		}

		commonJob, err := a.jobService.CreateJob(ctx, serviceReq)
		if err != nil {
			a.logger.WithError(err).Error("failed to create job")
			// Check if it's a validation error and return 400 instead of 500
			if errors.IsValidationError(err) {
				a.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
				return
			}
			a.writeError(w, http.StatusInternalServerError, "JOB_CREATION_FAILED", "Failed to create job", err)
			return
		}

		apiJob := a.convertCommonJobToAPI(commonJob)

		// Publish SSE event
		if a.sse != nil {
			campaignUUID := uuid.New()
			if commonJob.CampaignID != nil {
				if id, err := uuid.Parse(*commonJob.CampaignID); err == nil {
					campaignUUID = id
				}
			}
			event := sse.NewJobEvent("job.created", apiJob.Id, campaignUUID, map[string]any{
				"job":       apiJob,
				"timestamp": time.Now(),
			})
			if err := a.sse.Broadcast(event); err != nil {
				a.logger.WithError(err).Warn("failed to broadcast job created event")
			}
		}

		a.writeJSONResponse(w, http.StatusCreated, apiJob)
		return
	}

	// Fallback to repository
	corpusPath := fmt.Sprintf("/tmp/corpus/%s", uuid.New().String())
	outputPath := fmt.Sprintf("/tmp/output/%s", uuid.New().String())
	job, err := jobTypes.NewJob(req.Name, string(req.Fuzzer), req.TargetBinary, corpusPath, outputPath)
	if err != nil {
		a.logger.WithError(err).Error("failed to create job")
		a.writeError(w, http.StatusBadRequest, "JOB_CREATION_FAILED", "Failed to create job", err)
		return
	}

	if req.Priority != nil {
		job.Priority = jobTypes.JobPriority(*req.Priority)
	}

	if req.TimeoutSeconds != nil {
		job.MaxDuration = time.Duration(*req.TimeoutSeconds) * time.Second
	}

	if req.EnableCoverage != nil {
		if job.FuzzerConfig == nil {
			job.FuzzerConfig = make(map[string]any)
		}
		job.FuzzerConfig["enable_coverage"] = *req.EnableCoverage
	}

	if req.Config != nil {
		job.FuzzerConfig = *req.Config
	}

	if err := a.repository.Create(ctx, job); err != nil {
		a.logger.WithError(err).Error("failed to save job")
		a.writeError(w, http.StatusInternalServerError, "SAVE_FAILED", "Failed to save job", err)
		return
	}

	apiJob := a.convertJobToAPI(job)

	// Publish SSE event
	jobUUID := uuid.MustParse(job.ID)
	campaignUUID := uuid.New()
	if a.sse != nil {
		event := sse.NewJobEvent("job.created", jobUUID, campaignUUID, map[string]any{
			"job":       apiJob,
			"timestamp": time.Now(),
		})
		if err := a.sse.Broadcast(event); err != nil {
			a.logger.WithError(err).Warn("failed to broadcast job created event")
		}
	}

	a.writeJSONResponse(w, http.StatusCreated, apiJob)
}

// GetJob retrieves a specific job by ID
func (a *JobAdapter) GetJob(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, params generated.GetJobParams) {
	ctx := r.Context()

	// Check dependencies
	if a.jobService == nil && a.repository == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Job service not configured", nil)
		return
	}

	// Use jobService if available
	if a.jobService != nil {
		commonJob, err := a.jobService.GetJob(ctx, jobId.String())
		if err != nil {
			a.logger.WithError(err).WithField("job_id", jobId).Error("failed to get job")
			a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
			return
		}

		apiJob := a.convertCommonJobToAPI(commonJob)
		a.writeJSONResponse(w, http.StatusOK, apiJob)
		return
	}

	// Fallback to repository
	job, err := a.repository.Get(ctx, jobId.String())
	if err != nil {
		a.logger.WithError(err).WithField("job_id", jobId).Error("failed to get job")
		a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
		return
	}

	apiJob := a.convertJobToAPI(job)
	a.writeJSONResponse(w, http.StatusOK, apiJob)
}

// UpdateJob updates an existing job
func (a *JobAdapter) UpdateJob(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam) {
	ctx := r.Context()

	if a.jobService == nil && a.repository == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Job service not configured", nil)
		return
	}

	var req generated.JobUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Get existing job
	job, err := a.repository.Get(ctx, jobId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
		return
	}

	// Check if job can be updated (only pending/failed jobs can be updated)
	if job.Status != jobTypes.StatusPending && job.Status != jobTypes.StatusFailed {
		a.writeError(w, http.StatusConflict, "INVALID_STATUS", "Job cannot be updated in current status", nil)
		return
	}

	// Update fields if provided
	if req.Name != nil {
		job.Name = *req.Name
	}

	if req.Priority != nil {
		job.Priority = jobTypes.JobPriority(*req.Priority)
	}

	if req.TimeoutSeconds != nil {
		// Job doesn't have TimeoutAt, use MaxDuration instead
		job.MaxDuration = time.Duration(*req.TimeoutSeconds) * time.Second
	}

	if req.Config != nil {
		// Use FuzzerConfig instead of Config
		job.FuzzerConfig = *req.Config
	}

	// Save changes
	if err := a.repository.Update(ctx, job); err != nil {
		a.logger.WithError(err).Error("failed to update job")
		a.writeError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update job", err)
		return
	}

	apiJob := a.convertJobToAPI(job)

	// Publish SSE event
	jobUUID := uuid.MustParse(job.ID)
	// Note: Job doesn't have CampaignID field
	campaignUUID := uuid.New() // Using placeholder
	event := sse.NewJobEvent("job.updated", jobUUID, campaignUUID, map[string]any{
		"job":       apiJob,
		"timestamp": time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast job updated event")
	}

	a.writeJSONResponse(w, http.StatusOK, apiJob)
}

// DeleteJob cancels/deletes a job
func (a *JobAdapter) DeleteJob(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam) {
	ctx := r.Context()

	if a.jobService == nil && a.repository == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Job service not configured", nil)
		return
	}

	// Use jobService if available
	if a.jobService != nil {
		// Cancel the job through the service
		if err := a.jobService.CancelJob(ctx, jobId.String()); err != nil {
			a.logger.WithError(err).Error("failed to cancel/delete job")
			a.writeError(w, http.StatusInternalServerError, "DELETE_FAILED", "Failed to delete job", err)
			return
		}

		// Publish SSE event
		if a.sse != nil {
			jobUUID, _ := uuid.Parse(jobId.String())
			if jobUUID == uuid.Nil {
				jobUUID = uuid.NewSHA1(uuid.NameSpaceDNS, []byte(jobId.String()))
			}
			campaignUUID := uuid.New()
			event := sse.NewJobEvent("job.cancelled", jobUUID, campaignUUID, map[string]any{
				"job_id":    jobId.String(),
				"timestamp": time.Now(),
			})
			if err := a.sse.Broadcast(event); err != nil {
				a.logger.WithError(err).Warn("failed to broadcast job cancelled event")
			}
		}

		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Get job to check its status
	job, err := a.repository.Get(ctx, jobId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
		return
	}

	// Cancel job if it's running
	if job.Status == jobTypes.StatusRunning || job.Status == jobTypes.StatusQueued {
		if err := a.cancelJob(ctx, job); err != nil {
			a.logger.WithError(err).Error("failed to cancel job")
			a.writeError(w, http.StatusInternalServerError, "CANCEL_FAILED", "Failed to cancel job", err)
			return
		}
	} else {
		// Mark as canceled if not running
		job.Status = jobTypes.StatusCancelled
		job.CompletedAt = &[]time.Time{time.Now()}[0]
		if err := a.repository.Update(ctx, job); err != nil {
			a.logger.WithError(err).Error("failed to update job status")
			a.writeError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update job", err)
			return
		}
	}

	// Publish SSE event
	jobUUID := uuid.MustParse(job.ID)
	// Note: Job doesn't have CampaignID field
	campaignUUID := uuid.New() // Using placeholder
	event := sse.NewJobEvent("job.cancelled", jobUUID, campaignUUID, map[string]any{
		"job_id":    job.ID,
		"timestamp": time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast job cancelled event")
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetJobLogs retrieves or streams job logs
func (a *JobAdapter) GetJobLogs(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, params generated.GetJobLogsParams) {
	ctx := r.Context()

	if a.jobService == nil && a.repository == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Job service not configured", nil)
		return
	}

	// Verify job exists - use service if available
	var jobExists bool
	var job *jobTypes.Job
	if a.jobService != nil {
		commonJob, err := a.jobService.GetJob(ctx, jobId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
			return
		}
		jobExists = true
		// Create a minimal job type for streaming
		job = &jobTypes.Job{ID: commonJob.ID}
		_ = jobExists
	} else {
		var err error
		job, err = a.repository.Get(ctx, jobId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
			return
		}
	}

	// Check if streaming is requested
	if params.Follow != nil && *params.Follow {
		a.streamJobLogs(w, r, job)
		return
	}

	// Get static logs
	logs := a.getJobLogs(ctx, jobId.String(), params)

	response := generated.JobLogsResponse{
		JobId:     uuid.MustParse(job.ID),
		Logs:      logs,
		Timestamp: time.Now(),
		HasMore:   &[]bool{false}[0], // For now, assume no more logs
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// GetJobCoverage retrieves job coverage reports
func (a *JobAdapter) GetJobCoverage(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, params generated.GetJobCoverageParams) {
	ctx := r.Context()

	if a.jobService == nil && a.repository == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Job service not configured", nil)
		return
	}

	// Verify job exists - use service if available
	if a.jobService != nil {
		_, err := a.jobService.GetJob(ctx, jobId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
			return
		}
	} else {
		_, err := a.repository.Get(ctx, jobId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
			return
		}
	}

	// Get coverage reports for job
	reports := a.getCoverageReports(ctx, jobId.String(), params)

	// Create pagination
	pagination := generated.Pagination{
		Limit:   50,
		Offset:  0,
		Total:   len(reports),
		HasMore: false,
	}

	if params.Limit != nil {
		pagination.Limit = *params.Limit
	}

	if params.Offset != nil {
		pagination.Offset = *params.Offset
	}

	response := generated.CoverageReportListResponse{
		Data:       reports,
		Pagination: pagination,
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// GetJobArtifacts retrieves job artifacts
func (a *JobAdapter) GetJobArtifacts(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, params generated.GetJobArtifactsParams) {
	ctx := r.Context()

	if a.jobService == nil && a.repository == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Job service not configured", nil)
		return
	}

	// Verify job exists - use service if available
	if a.jobService != nil {
		_, err := a.jobService.GetJob(ctx, jobId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
			return
		}
	} else {
		_, err := a.repository.Get(ctx, jobId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
			return
		}
	}

	// Get artifacts for job
	artifacts := a.getJobArtifacts(ctx, jobId.String(), params)

	// Create pagination
	pagination := generated.Pagination{
		Limit:   50,
		Offset:  0,
		Total:   len(artifacts),
		HasMore: false,
	}

	if params.Limit != nil {
		pagination.Limit = *params.Limit
	}

	if params.Offset != nil {
		pagination.Offset = *params.Offset
	}

	response := generated.ArtifactListResponse{
		Data:       artifacts,
		Pagination: pagination,
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// DownloadCoverageReport downloads a specific coverage report
func (a *JobAdapter) DownloadCoverageReport(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, reportId generated.ReportIdParam) {
	ctx := r.Context()

	if a.jobService == nil && a.repository == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Job service not configured", nil)
		return
	}

	// Verify job exists - use service if available
	if a.jobService != nil {
		_, err := a.jobService.GetJob(ctx, jobId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
			return
		}
	} else {
		_, err := a.repository.Get(ctx, jobId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
			return
		}
	}

	// Get coverage report file
	reportData, contentType, filename, err := a.getCoverageReportFile(ctx, reportId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "REPORT_NOT_FOUND", "Coverage report not found", err)
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.WriteHeader(http.StatusOK)
	w.Write(reportData)
}

// Helper methods

func (a *JobAdapter) convertJobToAPI(job *jobTypes.Job) generated.Job {
	apiJob := generated.Job{
		Id:           uuid.MustParse(job.ID),
		Name:         job.Name,
		Status:       domainJobStatusToGenerated(job.Status),
		CreatedAt:    job.CreatedAt,
		TargetBinary: job.TargetBinary,
		// Note: Job doesn't have TimeoutAt, calculating from MaxDuration
		TimeoutAt: time.Now().Add(job.MaxDuration),
		Fuzzer:    generated.FuzzerType(job.FuzzerType),
	}

	// Note: Job doesn't have CampaignID field
	// Would need to be tracked separately

	// Use LockedBy as AssignedBotID
	if job.LockedBy != "" {
		botID := uuid.New() // Would need to map LockedBy string to UUID
		apiJob.AssignedBotId = &botID
	}

	if job.StartedAt != nil {
		apiJob.StartedAt = job.StartedAt
	}

	if job.CompletedAt != nil {
		apiJob.CompletedAt = job.CompletedAt
	}

	// Check if coverage is enabled in FuzzerConfig
	if enableCoverage, ok := job.FuzzerConfig["enable_coverage"].(bool); ok && enableCoverage {
		apiJob.EnableCoverage = &enableCoverage
	}

	if len(job.FuzzerConfig) > 0 {
		config := make(map[string]interface{})
		for k, v := range job.FuzzerConfig {
			config[k] = v
		}
		apiJob.Config = &config
	}

	// Set priority
	priority := int(job.Priority)
	apiJob.Priority = &priority

	return apiJob
}

func (a *JobAdapter) cancelJob(ctx context.Context, job *jobTypes.Job) error {
	// If job is running, signal cancellation to executor
	// Use LockedBy instead of AssignedBotID
	if job.Status == jobTypes.StatusRunning && job.LockedBy != "" {
		// Note: executor doesn't have CancelJob method
		// Would need to implement cancellation differently
		if err := a.executor.Cancel(ctx, job.LockedBy, job.ID); err != nil {
			a.logger.WithError(err).Warn("failed to cancel job via executor")
		}
	}

	// Update job status
	job.Status = jobTypes.StatusCancelled
	job.CompletedAt = &[]time.Time{time.Now()}[0]

	return a.repository.Update(ctx, job)
}

func (a *JobAdapter) streamJobLogs(w http.ResponseWriter, r *http.Request, job *jobTypes.Job) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Subscribe to job-specific events
	clientID := uuid.New().String()
	config := sse.ClientConfig{
		BufferSize:        100,
		WriteTimeout:      30 * time.Second,
		MaxEventsPerSec:   100,
		BurstSize:         10,
		EnableCompression: false,
	}
	client := sse.NewClient(clientID, w, r, config, a.logger)

	if err := a.sse.Register(client); err != nil {
		a.logger.WithError(err).Error("failed to register SSE client for job logs")
		return
	}
	defer a.sse.Unregister(client)

	// Subscribe to job events
	if err := a.sse.Subscribe(clientID, "job."+job.ID); err != nil {
		a.logger.WithError(err).Error("failed to subscribe to job events")
	}

	// Keep connection alive until client disconnects
	<-r.Context().Done()
}

func (a *JobAdapter) getJobLogs(ctx context.Context, jobID string, params generated.GetJobLogsParams) []struct {
	Level     generated.JobLogsResponseLogsLevel `json:"level"`
	Message   string                             `json:"message"`
	Metadata  *map[string]interface{}            `json:"metadata,omitempty"`
	Source    *string                            `json:"source,omitempty"`
	Timestamp time.Time                          `json:"timestamp"`
} {
	// Mock implementation - in reality, this would fetch from log storage
	logs := []struct {
		Level     generated.JobLogsResponseLogsLevel `json:"level"`
		Message   string                             `json:"message"`
		Metadata  *map[string]interface{}            `json:"metadata,omitempty"`
		Source    *string                            `json:"source,omitempty"`
		Timestamp time.Time                          `json:"timestamp"`
	}{
		{
			Level:     "info",
			Message:   fmt.Sprintf("Job %s started", jobID),
			Source:    &[]string{"job_manager"}[0],
			Timestamp: time.Now().Add(-5 * time.Minute),
		},
		{
			Level:     "info",
			Message:   "Fuzzing in progress...",
			Source:    &[]string{"fuzzer"}[0],
			Timestamp: time.Now().Add(-2 * time.Minute),
		},
	}

	return logs
}

func (a *JobAdapter) getCoverageReports(ctx context.Context, jobID string, params generated.GetJobCoverageParams) []generated.CoverageReport {
	// Mock implementation - in reality, this would fetch from storage
	reports := []generated.CoverageReport{
		{
			Id:        uuid.New(),
			JobId:     uuid.MustParse(jobID),
			Format:    generated.Html,
			CreatedAt: time.Now().Add(-30 * time.Minute),
			SizeBytes: 1024 * 1024, // 1MB
			FilePath:  &[]string{"/coverage/" + jobID + "/report.html"}[0],
		},
	}

	return reports
}

func (a *JobAdapter) getJobArtifacts(ctx context.Context, jobID string, params generated.GetJobArtifactsParams) []generated.Artifact {
	// Mock implementation - in reality, this would fetch from storage
	artifacts := []generated.Artifact{
		{
			Id:          uuid.New(),
			JobId:       uuid.MustParse(jobID),
			Type:        generated.ArtifactTypeLog,
			Filename:    "fuzzer.log",
			SizeBytes:   2048,
			Hash:        "abc123def456",
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			ContentType: &[]string{"text/plain"}[0],
		},
	}

	return artifacts
}

func (a *JobAdapter) getCoverageReportFile(ctx context.Context, reportID string) ([]byte, string, string, error) {
	// Mock implementation - in reality, this would fetch from file storage
	data := []byte("<html><body>Coverage Report</body></html>")
	contentType := "text/html"
	filename := "coverage_report.html"

	return data, contentType, filename, nil
}

func (a *JobAdapter) writeJSONResponse(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		a.logger.WithError(err).Error("failed to encode JSON response")
	}
}

func (a *JobAdapter) writeError(w http.ResponseWriter, statusCode int, errorType, title string, err error) {
	problem := generated.ProblemDetails{
		Type:      fmt.Sprintf("/errors/%s", strings.ToLower(errorType)),
		Title:     title,
		Status:    statusCode,
		Timestamp: &[]time.Time{time.Now()}[0],
	}

	if err != nil {
		detail := err.Error()
		problem.Detail = &detail
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)
	if encodeErr := json.NewEncoder(w).Encode(problem); encodeErr != nil {
		a.logger.WithError(encodeErr).Error("failed to encode error response")
	}
}

// AckJob handles job acknowledgment with lease token
func (a *JobAdapter) AckJob(w http.ResponseWriter, r *http.Request, jobID, botID, leaseToken string) {
	ctx := r.Context()

	// For backward compatibility: if the job has no lease token (NULL in DB), accept any ACK
	job, err := a.repository.Get(ctx, jobID)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
		return
	}

	// Check if the job has a lease token
	domainJob := job

	// If job has a lease token, validate it
	if domainJob.LeaseToken != nil && *domainJob.LeaseToken != "" {
		if leaseToken != *domainJob.LeaseToken {
			a.writeError(w, http.StatusUnauthorized, "INVALID_LEASE", "Invalid lease token", nil)
			return
		}
	}

	// Extend lease expiry
	now := time.Now()
	leaseExpiresAt := now.Add(60 * time.Second)

	// Update job status to starting
	domainJob.Status = jobTypes.StatusStarting
	domainJob.LeaseExpiresAt = &leaseExpiresAt

	if err := a.repository.Update(ctx, domainJob); err != nil {
		a.writeError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update job", err)
		return
	}

	// Send SSE event
	jobUUID := uuid.MustParse(jobID)
	campaignUUID := uuid.New() // Using placeholder since job doesn't have campaign
	event := sse.NewJobEvent("job.started", jobUUID, campaignUUID, map[string]interface{}{
		"job_id":    jobID,
		"bot_id":    botID,
		"status":    "starting",
		"timestamp": time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast job started event")
	}

	// Return success response
	response := map[string]interface{}{
		"acknowledged":     true,
		"lease_expires_at": leaseExpiresAt,
		"message":          "Job acknowledged successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// JobHeartbeat handles job heartbeat to renew lease
func (a *JobAdapter) JobHeartbeat(w http.ResponseWriter, r *http.Request, jobID, botID, leaseToken string) {
	ctx := r.Context()

	// Get the job
	job, err := a.repository.Get(ctx, jobID)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
		return
	}

	domainJob := job

	// For backward compatibility: if the job has no lease token (NULL in DB), accept any heartbeat
	if domainJob.LeaseToken != nil && *domainJob.LeaseToken != "" {
		if leaseToken != *domainJob.LeaseToken {
			a.writeError(w, http.StatusUnauthorized, "INVALID_LEASE", "Invalid lease token", nil)
			return
		}
	}

	// Extend lease expiry
	now := time.Now()
	leaseExpiresAt := now.Add(60 * time.Second)
	domainJob.LeaseExpiresAt = &leaseExpiresAt
	domainJob.LastHeartbeat = &now

	// Update job with new lease expiry
	if err := a.repository.Update(ctx, domainJob); err != nil {
		a.writeError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update job", err)
		return
	}

	// Return success response
	response := map[string]interface{}{
		"success":          true,
		"lease_expires_at": leaseExpiresAt,
		"message":          "Heartbeat received",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// GetJobProgress returns real-time progress for a job (from v3)
func (a *JobAdapter) GetJobProgress(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam) {
	ctx := r.Context()

	if a.jobService == nil && a.repository == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Job service not configured", nil)
		return
	}

	// Use jobService if available
	if a.jobService != nil {
		commonJob, err := a.jobService.GetJob(ctx, jobId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
			return
		}

		// Calculate progress metrics
		var progressPercent float64
		var elapsedSeconds float64

		if commonJob.StartedAt != nil {
			elapsed := time.Since(*commonJob.StartedAt)
			elapsedSeconds = elapsed.Seconds()

			if commonJob.Config.Duration > 0 {
				progressPercent = (elapsed.Seconds() / commonJob.Config.Duration.Seconds()) * 100
				if progressPercent > 100 {
					progressPercent = 100
				}
			}
		}

		progress := map[string]interface{}{
			"job_id":           commonJob.ID,
			"status":           commonJobStatusToGenerated(commonJob.Status),
			"progress_percent": progressPercent,
			"elapsed_seconds":  elapsedSeconds,
			"started_at":       commonJob.StartedAt,
			"completed_at":     commonJob.CompletedAt,
			"timestamp":        time.Now(),
		}

		a.writeJSONResponse(w, http.StatusOK, progress)
		return
	}

	// Fallback to repository
	job, err := a.repository.Get(ctx, jobId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
		return
	}

	// Calculate progress metrics
	var progressPercent float64
	var elapsedSeconds float64

	if job.StartedAt != nil {
		elapsed := time.Since(*job.StartedAt)
		elapsedSeconds = elapsed.Seconds()

		if job.MaxDuration > 0 {
			progressPercent = (elapsed.Seconds() / job.MaxDuration.Seconds()) * 100
			if progressPercent > 100 {
				progressPercent = 100
			}
		}
	}

	progress := map[string]interface{}{
		"job_id":           job.ID,
		"status":           domainJobStatusToGenerated(job.Status),
		"progress_percent": progressPercent,
		"elapsed_seconds":  elapsedSeconds,
		"started_at":       job.StartedAt,
		"completed_at":     job.CompletedAt,
		"last_heartbeat":   job.LastHeartbeat,
		"timestamp":        time.Now(),
	}

	// Add fuzzer-specific metrics if available
	if job.FuzzerConfig != nil {
		if execsPerSec, ok := job.FuzzerConfig["execs_per_sec"]; ok {
			progress["executions_per_sec"] = execsPerSec
		}
		if totalExecs, ok := job.FuzzerConfig["total_execs"]; ok {
			progress["total_executions"] = totalExecs
		}
		if coverage, ok := job.FuzzerConfig["coverage"]; ok {
			progress["coverage_percent"] = coverage
		}
		if edges, ok := job.FuzzerConfig["edges_found"]; ok {
			progress["edges_found"] = edges
		}
	}

	a.writeJSONResponse(w, http.StatusOK, progress)
}

// CancelJob cancels a specific job
func (a *JobAdapter) CancelJob(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam) {
	ctx := r.Context()

	// Check dependencies
	if a.jobService == nil && a.repository == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Job service not configured", nil)
		return
	}

	// Use jobService if available
	if a.jobService != nil {
		// First get the job to check its status
		commonJob, err := a.jobService.GetJob(ctx, jobId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
			return
		}

		// Check if job can be cancelled
		if commonJob.Status == common.JobStatusCompleted || commonJob.Status == common.JobStatusCancelled || commonJob.Status == common.JobStatusFailed {
			a.writeError(w, http.StatusConflict, "INVALID_STATUS", "Job cannot be cancelled in current status", nil)
			return
		}

		// Cancel the job
		if err := a.jobService.CancelJob(ctx, jobId.String()); err != nil {
			a.logger.WithError(err).Error("failed to cancel job")
			a.writeError(w, http.StatusInternalServerError, "CANCEL_FAILED", "Failed to cancel job", err)
			return
		}

		// Get updated job
		commonJob, err = a.jobService.GetJob(ctx, jobId.String())
		if err != nil {
			// Job was cancelled, but we couldn't get the updated state - return success anyway
			a.writeJSONResponse(w, http.StatusOK, map[string]any{
				"id":     jobId.String(),
				"status": "cancelled",
			})
			return
		}

		apiJob := a.convertCommonJobToAPI(commonJob)

		// Publish SSE event
		if a.sse != nil {
			campaignUUID := uuid.New()
			if commonJob.CampaignID != nil {
				if id, err := uuid.Parse(*commonJob.CampaignID); err == nil {
					campaignUUID = id
				}
			}
			event := sse.NewJobEvent("job.cancelled", apiJob.Id, campaignUUID, map[string]any{
				"job_id":    commonJob.ID,
				"timestamp": time.Now(),
			})
			if err := a.sse.Broadcast(event); err != nil {
				a.logger.WithError(err).Warn("failed to broadcast job cancelled event")
			}
		}

		a.writeJSONResponse(w, http.StatusOK, apiJob)
		return
	}

	// Fallback to repository
	job, err := a.repository.Get(ctx, jobId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
		return
	}

	if job.Status == jobTypes.StatusCompleted || job.Status == jobTypes.StatusCancelled || job.Status == jobTypes.StatusFailed {
		a.writeError(w, http.StatusConflict, "INVALID_STATUS", "Job cannot be cancelled in current status", nil)
		return
	}

	if err := a.cancelJob(ctx, job); err != nil {
		a.logger.WithError(err).Error("failed to cancel job")
		a.writeError(w, http.StatusInternalServerError, "CANCEL_FAILED", "Failed to cancel job", err)
		return
	}

	// Publish SSE event
	jobUUID := uuid.MustParse(job.ID)
	campaignUUID := uuid.New()
	if a.sse != nil {
		event := sse.NewJobEvent("job.cancelled", jobUUID, campaignUUID, map[string]any{
			"job_id":    job.ID,
			"timestamp": time.Now(),
		})
		if err := a.sse.Broadcast(event); err != nil {
			a.logger.WithError(err).Warn("failed to broadcast job cancelled event")
		}
	}

	apiJob := a.convertJobToAPI(job)
	a.writeJSONResponse(w, http.StatusOK, apiJob)
}

// GetJobCrashes returns crashes found during a job (from v3)
func (a *JobAdapter) GetJobCrashes(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam) {
	ctx := r.Context()

	if a.jobService == nil && a.repository == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Job service not configured", nil)
		return
	}

	// Verify job exists - use service if available
	if a.jobService != nil {
		_, err := a.jobService.GetJob(ctx, jobId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
			return
		}
	} else {
		_, err := a.repository.Get(ctx, jobId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
			return
		}
	}

	// Return mock data - in production, this would fetch from crash repository
	crashes := []map[string]interface{}{
		{
			"id":            uuid.New().String(),
			"job_id":        jobId.String(),
			"type":          "segfault",
			"signal":        11,
			"hash":          "abc123",
			"discovered_at": time.Now().Add(-30 * time.Minute),
			"is_unique":     true,
		},
	}

	response := map[string]interface{}{
		"job_id":       jobId.String(),
		"crashes":      crashes,
		"total_count":  len(crashes),
		"unique_count": len(crashes),
		"timestamp":    time.Now(),
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// generatedToCommonJobStatus converts generated.JobStatus to common.JobStatus
func generatedToCommonJobStatus(status generated.JobStatus) common.JobStatus {
	switch status {
	case generated.JobStatusPending:
		return common.JobStatusPending
	case generated.JobStatusAssigned:
		return common.JobStatusAssigned
	case generated.JobStatusRunning:
		return common.JobStatusRunning
	case generated.JobStatusCompleted:
		return common.JobStatusCompleted
	case generated.JobStatusFailed:
		return common.JobStatusFailed
	case generated.JobStatusCancelled:
		return common.JobStatusCancelled
	case generated.JobStatusTimeout:
		return common.JobStatusTimedOut
	default:
		return common.JobStatusPending
	}
}

// convertCommonJobToAPI converts a common.Job to a generated.Job
func (a *JobAdapter) convertCommonJobToAPI(job *common.Job) generated.Job {
	// Try to parse ID as UUID, if it fails generate a deterministic UUID from the ID
	jobID, err := uuid.Parse(job.ID)
	if err != nil {
		// Generate a deterministic UUID from the job ID using UUID v5 with DNS namespace
		jobID = uuid.NewSHA1(uuid.NameSpaceDNS, []byte(job.ID))
	}

	apiJob := generated.Job{
		Id:           jobID,
		Name:         job.Name,
		Status:       commonJobStatusToGenerated(job.Status),
		CreatedAt:    job.CreatedAt,
		TargetBinary: job.Target,
		TimeoutAt:    job.TimeoutAt,
		Fuzzer:       generated.FuzzerType(job.Fuzzer),
	}

	// Set campaign ID if exists
	if job.CampaignID != nil {
		if id, err := uuid.Parse(*job.CampaignID); err == nil {
			apiJob.CampaignId = &id
		}
	}

	// Set assigned bot ID
	if job.AssignedBot != nil {
		botID, err := uuid.Parse(*job.AssignedBot)
		if err != nil {
			botID = uuid.NewSHA1(uuid.NameSpaceDNS, []byte(*job.AssignedBot))
		}
		apiJob.AssignedBotId = &botID
	}

	if job.StartedAt != nil {
		apiJob.StartedAt = job.StartedAt
	}

	if job.CompletedAt != nil {
		apiJob.CompletedAt = job.CompletedAt
	}

	// Set priority
	priority := job.Priority
	apiJob.Priority = &priority

	// Set coverage
	enableCoverage := job.EnableCoverage
	apiJob.EnableCoverage = &enableCoverage

	return apiJob
}
