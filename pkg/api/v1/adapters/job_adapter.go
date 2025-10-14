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
	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/executor"
	jobRepo "github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	jobTypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
)

// JobAdapter implements the job-related endpoints of the generated ServerInterface
type JobAdapter struct {
	repository jobRepo.JobRepository
	executor   executor.Executor
	sse        *sse.Manager
	logger     logrus.FieldLogger
}

// NewJobAdapter creates a new job adapter
func NewJobAdapter(
	repository jobRepo.JobRepository,
	executor executor.Executor,
	sse *sse.Manager,
	logger logrus.FieldLogger,
) *JobAdapter {
	return &JobAdapter{
		repository: repository,
		executor:   executor,
		sse:        sse,
		logger:     logger.WithField("component", "job_adapter"),
	}
}

// ListJobs retrieves jobs with filtering and pagination
func (a *JobAdapter) ListJobs(w http.ResponseWriter, r *http.Request, params generated.ListJobsParams) {
	ctx := r.Context()

	// Build filter from parameters
	filter := jobRepo.JobFilter{
		Limit:  50,
		Offset: 0,
	}

	if params.Limit != nil && *params.Limit > 0 {
		filter.Limit = *params.Limit
		if filter.Limit > 1000 {
			filter.Limit = 1000
		}
	}

	if params.Offset != nil && *params.Offset >= 0 {
		filter.Offset = *params.Offset
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
		Limit:   filter.Limit,
		Offset:  filter.Offset,
		Total:   len(apiJobs),
		HasMore: len(apiJobs) == filter.Limit,
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

	var req generated.JobCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Create new job - need to provide corpus and output paths
	corpusPath := fmt.Sprintf("/tmp/corpus/%s", uuid.New().String())
	outputPath := fmt.Sprintf("/tmp/output/%s", uuid.New().String())
	job, err := jobTypes.NewJob(req.Name, string(req.Fuzzer), req.TargetBinary, corpusPath, outputPath)
	if err != nil {
		a.logger.WithError(err).Error("failed to create job")
		a.writeError(w, http.StatusBadRequest, "JOB_CREATION_FAILED", "Failed to create job", err)
		return
	}

	// Set optional fields
	// Note: Job doesn't have CampaignID field
	// This would need to be tracked separately

	if req.Priority != nil {
		job.Priority = jobTypes.JobPriority(*req.Priority)
	}

	if req.TimeoutSeconds != nil {
		// Job doesn't have TimeoutAt, use MaxDuration instead
		job.MaxDuration = time.Duration(*req.TimeoutSeconds) * time.Second
	}

	// Note: Job doesn't have EnableCoverage field
	// This would need to be tracked in FuzzerConfig or separately
	if req.EnableCoverage != nil {
		if job.FuzzerConfig == nil {
			job.FuzzerConfig = make(map[string]any)
		}
		job.FuzzerConfig["enable_coverage"] = *req.EnableCoverage
	}

	if req.Config != nil {
		// Use FuzzerConfig instead of Config
		job.FuzzerConfig = *req.Config
	}

	// Save job to repository
	if err := a.repository.Create(ctx, job); err != nil {
		a.logger.WithError(err).Error("failed to save job")
		a.writeError(w, http.StatusInternalServerError, "SAVE_FAILED", "Failed to save job", err)
		return
	}

	apiJob := a.convertJobToAPI(job)

	// Publish SSE event
	jobUUID := uuid.MustParse(job.ID)
	// Note: Job doesn't have CampaignID field
	campaignUUID := uuid.New() // Using placeholder
	event := sse.NewJobEvent("job.created", jobUUID, campaignUUID, map[string]any{
		"job":       apiJob,
		"timestamp": time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast job created event")
	}

	a.writeJSONResponse(w, http.StatusCreated, apiJob)
}

// GetJob retrieves a specific job by ID
func (a *JobAdapter) GetJob(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, params generated.GetJobParams) {
	ctx := r.Context()

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

	// Verify job exists
	job, err := a.repository.Get(ctx, jobId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
		return
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

	// Verify job exists
	_, err := a.repository.Get(ctx, jobId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
		return
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

	// Verify job exists
	_, err := a.repository.Get(ctx, jobId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
		return
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

	// Verify job exists
	_, err := a.repository.Get(ctx, jobId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
		return
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
