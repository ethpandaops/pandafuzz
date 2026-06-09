package handlers

import (
	"net/http"
	"strconv"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// HandleListJobs handles GET /api/v1/jobs
func (h *Handlers) HandleListJobs(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	params := generated.ListJobsParams{}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			limitVal := limit
			params.Limit = &limitVal
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			offsetVal := offset
			params.Offset = &offsetVal
		}
	}

	if status := r.URL.Query().Get("status"); status != "" {
		statusVal := generated.JobStatus(status)
		params.Status = &statusVal
	}

	if fuzzer := r.URL.Query().Get("fuzzer"); fuzzer != "" {
		fuzzerVal := generated.FuzzerType(fuzzer)
		params.Fuzzer = &fuzzerVal
	}

	if campaignId := r.URL.Query().Get("campaign_id"); campaignId != "" {
		if parsedUUID, err := uuid.Parse(campaignId); err == nil {
			uuidVal := openapi_types.UUID(parsedUUID)
			params.CampaignId = &uuidVal
		}
	}

	if botId := r.URL.Query().Get("bot_id"); botId != "" {
		if parsedUUID, err := uuid.Parse(botId); err == nil {
			uuidVal := openapi_types.UUID(parsedUUID)
			params.BotId = &uuidVal
		}
	}

	// Delegate to adapter
	h.adapter.ListJobs(w, r, params)
}

// HandleCreateJob handles POST /api/v1/jobs
func (h *Handlers) HandleCreateJob(w http.ResponseWriter, r *http.Request) {
	h.adapter.CreateJob(w, r)
}

// HandleGetJob handles GET /api/v1/jobs/{id}
func (h *Handlers) HandleGetJob(w http.ResponseWriter, r *http.Request) {
	jobId := h.extractJobID(r)

	// Parse query parameters
	params := generated.GetJobParams{}

	// Delegate to adapter
	h.adapter.GetJob(w, r, jobId, params)
}

// HandleUpdateJob handles PUT /api/v1/jobs/{id}
func (h *Handlers) HandleUpdateJob(w http.ResponseWriter, r *http.Request) {
	jobId := h.extractJobID(r)
	h.adapter.UpdateJob(w, r, jobId)
}

// HandleDeleteJob handles DELETE /api/v1/jobs/{id}
func (h *Handlers) HandleDeleteJob(w http.ResponseWriter, r *http.Request) {
	jobId := h.extractJobID(r)
	h.adapter.DeleteJob(w, r, jobId)
}

// HandleGetJobLogs handles GET /api/v1/jobs/{id}/logs
func (h *Handlers) HandleGetJobLogs(w http.ResponseWriter, r *http.Request) {
	jobId := h.extractJobID(r)

	// Parse query parameters
	params := generated.GetJobLogsParams{}

	if follow := r.URL.Query().Get("follow"); follow != "" {
		if followVal, err := strconv.ParseBool(follow); err == nil {
			params.Follow = &followVal
		}
	}

	// Delegate to adapter
	h.adapter.GetJobLogs(w, r, jobId, params)
}

// HandleGetJobCoverage handles GET /api/v1/jobs/{id}/coverage
func (h *Handlers) HandleGetJobCoverage(w http.ResponseWriter, r *http.Request) {
	jobId := h.extractJobID(r)

	// Parse query parameters
	params := generated.GetJobCoverageParams{}

	if format := r.URL.Query().Get("format"); format != "" {
		formatVal := generated.CoverageFormat(format)
		params.Format = &formatVal
	}

	// Delegate to adapter
	h.adapter.GetJobCoverage(w, r, jobId, params)
}

// HandleGetJobArtifacts handles GET /api/v1/jobs/{id}/artifacts
func (h *Handlers) HandleGetJobArtifacts(w http.ResponseWriter, r *http.Request) {
	jobId := h.extractJobID(r)

	// Parse query parameters
	params := generated.GetJobArtifactsParams{}

	// Check if Type field exists and has correct type
	if artifactType := r.URL.Query().Get("type"); artifactType != "" {
		// Use the correct generated type name if it exists
		typeVal := generated.GetJobArtifactsParamsType(artifactType)
		params.Type = &typeVal
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			limitVal := limit
			params.Limit = &limitVal
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			offsetVal := offset
			params.Offset = &offsetVal
		}
	}

	// Delegate to adapter
	h.adapter.GetJobArtifacts(w, r, jobId, params)
}

// HandleDownloadCoverageReport handles GET /api/v1/jobs/{id}/coverage/reports/{reportId}
func (h *Handlers) HandleDownloadCoverageReport(w http.ResponseWriter, r *http.Request) {
	jobId := h.extractJobID(r)
	reportId := h.extractReportID(r)

	// Delegate to adapter
	h.adapter.DownloadCoverageReport(w, r, jobId, reportId)
}

// HandleCancelJob handles POST /api/v1/jobs/{id}/cancel
func (h *Handlers) HandleCancelJob(w http.ResponseWriter, r *http.Request) {
	jobId := h.extractJobID(r)
	h.adapter.CancelJob(w, r, jobId)
}

// HandlePushJobLogs handles POST /api/v1/jobs/{id}/logs/push
func (h *Handlers) HandlePushJobLogs(w http.ResponseWriter, r *http.Request) {
	jobId := h.extractJobID(r)
	h.adapter.PushJobLogs(w, r, jobId.String())
}

// HandleDownloadJobBinary handles GET /api/v1/jobs/{id}/binary/download
func (h *Handlers) HandleDownloadJobBinary(w http.ResponseWriter, r *http.Request) {
	jobId := h.extractJobID(r)
	h.adapter.DownloadJobBinary(w, r, jobId.String())
}

// HandleUploadBinary handles POST /api/v1/binaries
func (h *Handlers) HandleUploadBinary(w http.ResponseWriter, r *http.Request) {
	h.adapter.UploadBinary(w, r)
}

// HandleListRawCoverage handles GET /api/v1/jobs/{id}/coverage/raw
func (h *Handlers) HandleListRawCoverage(w http.ResponseWriter, r *http.Request) {
	jobId := h.extractJobID(r)
	h.adapter.ListRawCoverage(w, r, generated.JobIdParam(jobId))
}

// HandleDownloadRawCoverageFile handles GET /api/v1/jobs/{id}/coverage/raw/{fileType}
func (h *Handlers) HandleDownloadRawCoverageFile(w http.ResponseWriter, r *http.Request) {
	jobId := h.extractJobID(r)
	fileType := r.URL.Query().Get("fileType")
	if fileType == "" {
		// Extract from URL path if not in query
		fileType = chi.URLParam(r, "fileType")
	}
	h.adapter.DownloadRawCoverageFile(w, r, generated.JobIdParam(jobId), fileType)
}

// HandleDownloadRawCoverageZip handles GET /api/v1/jobs/{id}/coverage/raw/all/zip
func (h *Handlers) HandleDownloadRawCoverageZip(w http.ResponseWriter, r *http.Request) {
	jobId := h.extractJobID(r)
	h.adapter.DownloadRawCoverageZip(w, r, generated.JobIdParam(jobId))
}
