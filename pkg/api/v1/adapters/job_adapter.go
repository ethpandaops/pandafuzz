package adapters

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	pferrors "github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/ethpandaops/pandafuzz/pkg/service"
)

// JobAdapter implements the job-related endpoints of the generated ServerInterface
type JobAdapter struct {
	repository     jobRepo.JobRepository
	executor       executor.Executor
	jobService     service.JobService
	storage        common.Storage
	fileStorage    common.FileStorage
	sse            *sse.Manager
	logger         logrus.FieldLogger
	maxRequestSize int64
}

// NewJobAdapter creates a new job adapter
func NewJobAdapter(
	repository jobRepo.JobRepository,
	executor executor.Executor,
	jobService service.JobService,
	storage common.Storage,
	fileStorage common.FileStorage,
	sse *sse.Manager,
	logger logrus.FieldLogger,
	maxRequestSize int64,
) *JobAdapter {
	return &JobAdapter{
		repository:     repository,
		executor:       executor,
		jobService:     jobService,
		storage:        storage,
		fileStorage:    fileStorage,
		sse:            sse,
		logger:         logger.WithField("component", "job_adapter"),
		maxRequestSize: maxRequestSize,
	}
}

// SetStorage sets the storage backend for the adapter
func (a *JobAdapter) SetStorage(storage common.Storage) {
	a.storage = storage
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
			if pferrors.IsValidationError(err) {
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

// PushJobLogs receives log data from bots and stores it
func (a *JobAdapter) PushJobLogs(w http.ResponseWriter, r *http.Request, jobId string) {
	ctx := r.Context()

	if a.storage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Storage not configured", nil)
		return
	}

	// Read the raw log content from the request body
	maxSize := a.maxRequestSize
	if maxSize <= 0 {
		maxSize = 10 * 1024 * 1024
	}
	body, err := readRequestBody(r, maxSize)
	if err != nil {
		status := http.StatusBadRequest
		if isRequestBodyTooLarge(err) {
			status = http.StatusRequestEntityTooLarge
		}
		a.writeError(w, status, "INVALID_REQUEST", "Failed to read log content", err)
		return
	}

	// Parse the log content - each line is a log entry
	// Format: TIMESTAMP level=LEVEL source=SOURCE msg="MESSAGE"
	// Or simpler format: [TIMESTAMP] [LEVEL] [SOURCE] MESSAGE
	lines := strings.Split(string(body), "\n")
	logs := make([]*common.JobLog, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		log := parseLogLine(line)
		if log != nil {
			log.JobID = jobId
			logs = append(logs, log)
		}
	}

	if len(logs) == 0 {
		a.writeError(w, http.StatusBadRequest, "INVALID_CONTENT", "No valid log entries found", nil)
		return
	}

	// Store logs in database
	if err := a.storage.StoreJobLogs(ctx, jobId, logs); err != nil {
		a.writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", "Failed to store logs", err)
		return
	}

	a.logger.WithFields(logrus.Fields{
		"job_id":    jobId,
		"log_count": len(logs),
	}).Info("Stored job logs from bot")

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "success",
		"log_count": len(logs),
	})
}

// parseLogLine parses a single log line into a JobLog
func parseLogLine(line string) *common.JobLog {
	// Try to parse different log formats
	log := &common.JobLog{
		Timestamp: time.Now(),
	}

	// Format 1: TIMESTAMP level=LEVEL source=SOURCE msg="MESSAGE"
	if strings.Contains(line, "level=") && strings.Contains(line, "msg=") {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) >= 1 {
			// Try to parse timestamp
			if t, err := time.Parse(time.RFC3339, parts[0]); err == nil {
				log.Timestamp = t
			}
		}

		// Parse key=value pairs
		if idx := strings.Index(line, "level="); idx >= 0 {
			rest := line[idx+6:]
			if spaceIdx := strings.IndexAny(rest, " \t"); spaceIdx > 0 {
				log.Level = rest[:spaceIdx]
			} else {
				log.Level = rest
			}
		}

		if idx := strings.Index(line, "source="); idx >= 0 {
			rest := line[idx+7:]
			if spaceIdx := strings.IndexAny(rest, " \t"); spaceIdx > 0 {
				log.Source = rest[:spaceIdx]
			} else {
				log.Source = rest
			}
		}

		if idx := strings.Index(line, "msg=\""); idx >= 0 {
			rest := line[idx+5:]
			if endIdx := strings.Index(rest, "\""); endIdx > 0 {
				log.Message = rest[:endIdx]
			} else {
				log.Message = rest
			}
		} else if idx := strings.Index(line, "msg="); idx >= 0 {
			log.Message = line[idx+4:]
		}

		return log
	}

	// Format 2: [TIMESTAMP] [LEVEL] [SOURCE] MESSAGE
	if strings.HasPrefix(line, "[") {
		// Parse bracket-delimited format
		parts := strings.SplitN(line, "]", 4)
		if len(parts) >= 4 {
			if t, err := time.Parse("2006-01-02T15:04:05", strings.TrimPrefix(parts[0], "[")); err == nil {
				log.Timestamp = t
			}
			log.Level = strings.TrimPrefix(strings.TrimSpace(parts[1]), "[")
			log.Source = strings.TrimPrefix(strings.TrimSpace(parts[2]), "[")
			log.Message = strings.TrimSpace(parts[3])
			return log
		}
	}

	// Format 3: Simple text - treat entire line as message
	log.Level = "info"
	log.Source = "unknown"
	log.Message = line
	return log
}

// readRequestBody reads the request body with a size limit
func readRequestBody(r *http.Request, maxSize int64) ([]byte, error) {
	limit := maxSize
	if limit <= 0 {
		limit = 10 * 1024 * 1024
	}
	if limit > 0 && r.ContentLength > limit {
		return nil, errUploadTooLarge
	}

	// Create a limited reader
	limitedReader := io.LimitReader(r.Body, limit+1)
	defer r.Body.Close()

	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	if limit > 0 && int64(len(body)) > limit {
		return nil, errUploadTooLarge
	}

	return body, nil
}

func isRequestBodyTooLarge(err error) bool {
	if errors.Is(err, errUploadTooLarge) {
		return true
	}
	return strings.Contains(err.Error(), "request body too large")
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
	// Define the result type matching the return signature
	type logEntry = struct {
		Level     generated.JobLogsResponseLogsLevel `json:"level"`
		Message   string                             `json:"message"`
		Metadata  *map[string]interface{}            `json:"metadata,omitempty"`
		Source    *string                            `json:"source,omitempty"`
		Timestamp time.Time                          `json:"timestamp"`
	}

	// Try to fetch from real storage
	if a.storage != nil {
		limit := 1000
		offset := 0
		storedLogs, _, err := a.storage.GetJobLogs(ctx, jobID, limit, offset)
		if err == nil && len(storedLogs) > 0 {
			logs := make([]logEntry, 0, len(storedLogs))
			for _, log := range storedLogs {
				entry := logEntry{
					Level:     generated.JobLogsResponseLogsLevel(log.Level),
					Message:   log.Message,
					Timestamp: log.Timestamp,
				}
				if log.Source != "" {
					source := log.Source
					entry.Source = &source
				}
				if log.Metadata != nil {
					meta := log.Metadata
					entry.Metadata = &meta
				}
				logs = append(logs, entry)
			}
			return logs
		}
	}

	// Fallback to empty response if no logs found
	// Return empty slice (not nil) so JSON serializes to [] instead of null
	return []logEntry{}
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

	// Fetch crashes from storage
	var crashes []map[string]interface{}
	if a.storage != nil {
		storedCrashes, err := a.storage.ListCrashes(ctx, jobId.String(), 100, 0)
		if err != nil {
			a.logger.WithError(err).Error("Failed to list crashes from storage")
			// Continue with empty list rather than failing
		} else {
			for _, crash := range storedCrashes {
				crashMap := map[string]interface{}{
					"id":            crash.ID,
					"job_id":        crash.JobID,
					"type":          crash.Type,
					"signal":        crash.Signal,
					"hash":          crash.Hash,
					"discovered_at": crash.Timestamp,
					"is_unique":     crash.IsUnique,
				}
				if crash.ExitCode != 0 {
					crashMap["exit_code"] = crash.ExitCode
				}
				if crash.FilePath != "" {
					crashMap["file_path"] = crash.FilePath
				}
				crashes = append(crashes, crashMap)
			}
		}
	}

	// Count unique crashes
	uniqueCount := 0
	for _, c := range crashes {
		if isUnique, ok := c["is_unique"].(bool); ok && isUnique {
			uniqueCount++
		}
	}

	response := map[string]interface{}{
		"job_id":       jobId.String(),
		"crashes":      crashes,
		"total_count":  len(crashes),
		"unique_count": uniqueCount,
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

// DownloadJobBinary handles downloading the binary for a job
// The bot calls this endpoint to download the fuzz target binary before executing the job
func (a *JobAdapter) DownloadJobBinary(w http.ResponseWriter, r *http.Request, jobID string) {
	ctx := r.Context()

	a.logger.WithFields(logrus.Fields{
		"job_id": jobID,
	}).Info("Binary download requested")

	// Check if file storage is configured
	if a.fileStorage == nil {
		a.logger.Error("File storage not configured for binary download")
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "File storage not configured", nil)
		return
	}

	// Get the job to find the target binary path
	var targetBinary string
	if a.repository != nil {
		job, err := a.repository.Get(ctx, jobID)
		if err != nil {
			a.logger.WithError(err).WithField("job_id", jobID).Error("Failed to get job")
			a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
			return
		}
		targetBinary = job.TargetBinary
	} else if a.jobService != nil {
		job, err := a.jobService.GetJob(ctx, jobID)
		if err != nil {
			a.logger.WithError(err).WithField("job_id", jobID).Error("Failed to get job")
			a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
			return
		}
		// common.Job uses Target field instead of TargetBinary
		targetBinary = job.Target
	} else {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Job service not configured", nil)
		return
	}

	if targetBinary == "" {
		a.logger.WithField("job_id", jobID).Error("Job has no target binary specified")
		a.writeError(w, http.StatusBadRequest, "NO_TARGET_BINARY", "Job has no target binary specified", nil)
		return
	}

	// Convert the target binary path to storage path
	// Bot expects: /app/work/binaries/fuzz_target
	// Master stores at: binaries/fuzz_target (relative to storage base path)
	// We need to extract just the filename or relative path within binaries/
	storagePath := extractBinaryStoragePath(targetBinary)

	a.logger.WithFields(logrus.Fields{
		"job_id":        jobID,
		"target_binary": targetBinary,
		"storage_path":  storagePath,
	}).Debug("Attempting to read binary from storage")

	// Read the binary from storage
	data, err := a.fileStorage.ReadFile(ctx, storagePath)
	if err != nil {
		a.logger.WithError(err).WithFields(logrus.Fields{
			"job_id":        jobID,
			"storage_path":  storagePath,
			"target_binary": targetBinary,
		}).Error("Failed to read binary from storage")
		a.writeError(w, http.StatusNotFound, "BINARY_NOT_FOUND", "Binary file not found in storage", err)
		return
	}

	// Get the filename for Content-Disposition header
	filename := filepath.Base(targetBinary)

	a.logger.WithFields(logrus.Fields{
		"job_id":   jobID,
		"filename": filename,
		"size":     len(data),
	}).Info("Binary download successful")

	// Set response headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusOK)

	// Write the binary data
	if _, err := w.Write(data); err != nil {
		a.logger.WithError(err).Error("Failed to write binary to response")
	}
}

// extractBinaryStoragePath converts a target binary path to a storage-relative path
// Examples:
//   - /app/work/binaries/fuzz_target -> binaries/fuzz_target
//   - binaries/fuzz_target -> binaries/fuzz_target
//   - /app/data/binaries/fuzz_target -> binaries/fuzz_target
//   - fuzz_target -> binaries/fuzz_target
func extractBinaryStoragePath(targetBinary string) string {
	// Common prefixes to strip
	prefixes := []string{
		"/app/work/binaries/",
		"/app/data/binaries/",
		"/app/work/",
		"/app/data/",
	}

	path := targetBinary
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			path = strings.TrimPrefix(path, prefix)
			break
		}
	}

	// If the path already starts with "binaries/", keep it
	if strings.HasPrefix(path, "binaries/") {
		return path
	}

	// Otherwise, prepend "binaries/"
	return filepath.Join("binaries", path)
}

// UploadBinary handles uploading a binary file to storage
// This allows test scripts and external tools to upload fuzz targets
func (a *JobAdapter) UploadBinary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if file storage is configured
	if a.fileStorage == nil {
		a.logger.Error("File storage not configured for binary upload")
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "File storage not configured", nil)
		return
	}

	// Get the binary name from query parameter or form field
	binaryName := r.URL.Query().Get("name")
	if binaryName == "" {
		binaryName = r.FormValue("name")
	}
	if binaryName == "" {
		a.writeError(w, http.StatusBadRequest, "MISSING_NAME", "Binary name is required (use 'name' query parameter or form field)", nil)
		return
	}

	// Sanitize the binary name to prevent path traversal
	binaryName = filepath.Base(binaryName)
	if binaryName == "." || binaryName == ".." {
		a.writeError(w, http.StatusBadRequest, "INVALID_NAME", "Invalid binary name", nil)
		return
	}

	// Read the binary data from request body
	maxSize := a.maxRequestSize
	if maxSize <= 0 {
		maxSize = 100 * 1024 * 1024
	}
	if maxSize > 0 && r.ContentLength > maxSize {
		a.writeError(w, http.StatusRequestEntityTooLarge, "INVALID_REQUEST", "Binary exceeds size limit", nil)
		return
	}

	tempPath, size, _, err := streamToTempFile(r.Body, maxSize, false)
	r.Body.Close()
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errUploadTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		a.writeError(w, status, "INVALID_REQUEST", "Failed to stream binary data", err)
		return
	}

	if size == 0 {
		os.Remove(tempPath)
		a.writeError(w, http.StatusBadRequest, "EMPTY_BINARY", "Binary data is empty", nil)
		return
	}

	// Store the binary in the binaries/ directory
	storagePath := filepath.Join("binaries", binaryName)

	a.logger.WithFields(logrus.Fields{
		"binary_name":  binaryName,
		"storage_path": storagePath,
		"size":         size,
	}).Info("Uploading binary to storage")

	tempFile, err := os.Open(tempPath)
	if err != nil {
		os.Remove(tempPath)
		a.logger.WithError(err).WithField("storage_path", storagePath).Error("Failed to open temp binary file")
		a.writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", "Failed to save binary", err)
		return
	}

	if err := a.fileStorage.SaveFileStream(ctx, storagePath, tempFile, size); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		a.logger.WithError(err).WithField("storage_path", storagePath).Error("Failed to save binary to storage")
		a.writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", "Failed to save binary", err)
		return
	}
	tempFile.Close()
	os.Remove(tempPath)

	a.logger.WithFields(logrus.Fields{
		"binary_name":  binaryName,
		"storage_path": storagePath,
		"size":         size,
	}).Info("Binary uploaded successfully")

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "success",
		"binary_name":  binaryName,
		"storage_path": storagePath,
		"size":         size,
	})
}

// ListRawCoverage lists available raw coverage files for a job
func (a *JobAdapter) ListRawCoverage(w http.ResponseWriter, r *http.Request, jobID string) {
	ctx := r.Context()

	// Verify job exists
	_, err := a.repository.Get(ctx, jobID)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
		return
	}

	// Define the file types available for AFL++
	fileTypes := []string{"fuzzer_stats", "plot_data", "fuzz_bitmap"}

	// Build response with file info
	type RawCoverageFile struct {
		FileType    string `json:"file_type"`
		Description string `json:"description"`
		Available   bool   `json:"available"`
		Path        string `json:"path,omitempty"`
	}

	files := make([]RawCoverageFile, 0, len(fileTypes))
	for _, ft := range fileTypes {
		// Construct the expected path
		path := filepath.Join("coverage", jobID, ft)

		// Try to check if file exists in storage
		available := false
		if a.fileStorage != nil {
			// Check if the file exists
			if _, err := a.fileStorage.ReadFile(ctx, path); err == nil {
				available = true
			}
		}

		description := ""
		switch ft {
		case "fuzzer_stats":
			description = "AFL++ statistics and metrics"
		case "plot_data":
			description = "Time-series coverage data for plotting"
		case "fuzz_bitmap":
			description = "Binary coverage bitmap data"
		}

		file := RawCoverageFile{
			FileType:    ft,
			Description: description,
			Available:   available,
		}
		if available {
			file.Path = path
		}
		files = append(files, file)
	}

	response := map[string]interface{}{
		"job_id": jobID,
		"files":  files,
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// DownloadRawCoverageFile downloads a specific raw coverage file
func (a *JobAdapter) DownloadRawCoverageFile(w http.ResponseWriter, r *http.Request, jobID string, fileType string) {
	ctx := r.Context()

	// Validate file type
	validTypes := map[string]bool{
		"fuzzer_stats": true,
		"plot_data":    true,
		"fuzz_bitmap":  true,
	}

	if !validTypes[fileType] {
		a.writeError(w, http.StatusBadRequest, "INVALID_FILE_TYPE", "Invalid file type. Must be one of: fuzzer_stats, plot_data, fuzz_bitmap", nil)
		return
	}

	// Verify job exists
	_, err := a.repository.Get(ctx, jobID)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
		return
	}

	// Construct file path
	filePath := filepath.Join("coverage", jobID, fileType)

	// Read file from storage
	if a.fileStorage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "File storage not configured", nil)
		return
	}

	data, err := a.fileStorage.ReadFile(ctx, filePath)
	if err != nil {
		a.logger.WithError(err).WithFields(logrus.Fields{
			"job_id":    jobID,
			"file_type": fileType,
			"path":      filePath,
		}).Warn("Raw coverage file not found")
		a.writeError(w, http.StatusNotFound, "FILE_NOT_FOUND", "Coverage file not found", err)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_%s\"", jobID[:8], fileType))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(data); err != nil {
		a.logger.WithError(err).Error("Failed to write coverage file to response")
	}
}

// DownloadRawCoverageZip downloads all raw coverage files as a ZIP archive
func (a *JobAdapter) DownloadRawCoverageZip(w http.ResponseWriter, r *http.Request, jobID string) {
	ctx := r.Context()

	// Verify job exists
	_, err := a.repository.Get(ctx, jobID)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
		return
	}

	if a.fileStorage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "File storage not configured", nil)
		return
	}

	// Create ZIP archive in memory
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	fileTypes := []string{"fuzzer_stats", "plot_data", "fuzz_bitmap"}
	filesAdded := 0

	for _, fileType := range fileTypes {
		filePath := filepath.Join("coverage", jobID, fileType)
		data, err := a.fileStorage.ReadFile(ctx, filePath)
		if err != nil {
			// Skip files that don't exist
			a.logger.WithError(err).WithFields(logrus.Fields{
				"job_id":    jobID,
				"file_type": fileType,
			}).Debug("Raw coverage file not found, skipping")
			continue
		}

		// Add file to ZIP
		fileWriter, err := zipWriter.Create(fileType)
		if err != nil {
			a.logger.WithError(err).Error("Failed to create zip entry")
			continue
		}

		if _, err := fileWriter.Write(data); err != nil {
			a.logger.WithError(err).Error("Failed to write to zip entry")
			continue
		}

		filesAdded++
	}

	if err := zipWriter.Close(); err != nil {
		a.writeError(w, http.StatusInternalServerError, "ZIP_ERROR", "Failed to create ZIP archive", err)
		return
	}

	if filesAdded == 0 {
		a.writeError(w, http.StatusNotFound, "NO_FILES", "No raw coverage files found for this job", nil)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"coverage_%s.zip\"", jobID[:8]))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", buf.Len()))
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(buf.Bytes()); err != nil {
		a.logger.WithError(err).Error("Failed to write ZIP to response")
	}
}
