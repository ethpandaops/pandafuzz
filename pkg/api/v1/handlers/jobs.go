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

	// TODO: Add these parameters to OpenAPI spec
	// if sortBy := r.URL.Query().Get("sort_by"); sortBy != "" {
	// 	params.SortBy = &sortBy
	// }

	// if sortOrder := r.URL.Query().Get("sort_order"); sortOrder != "" {
	// 	sortOrderVal := generated.SortOrder(sortOrder)
	// 	params.SortOrder = &sortOrderVal
	// }

	// if since := r.URL.Query().Get("since"); since != "" {
	// 	params.Since = &since
	// }

	// if until := r.URL.Query().Get("until"); until != "" {
	// 	params.Until = &until
	// }

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

	// TODO: Add these parameters to OpenAPI spec
	// if includeArtifacts := r.URL.Query().Get("include_artifacts"); includeArtifacts != "" {
	// 	if include, err := strconv.ParseBool(includeArtifacts); err == nil {
	// 		params.IncludeArtifacts = &include
	// 	}
	// }

	// if includeLogs := r.URL.Query().Get("include_logs"); includeLogs != "" {
	// 	if include, err := strconv.ParseBool(includeLogs); err == nil {
	// 		params.IncludeLogs = &include
	// 	}
	// }

	// if includeCoverage := r.URL.Query().Get("include_coverage"); includeCoverage != "" {
	// 	if include, err := strconv.ParseBool(includeCoverage); err == nil {
	// 		params.IncludeCoverage = &include
	// 	}
	// }

	// if includeMetrics := r.URL.Query().Get("include_metrics"); includeMetrics != "" {
	// 	if include, err := strconv.ParseBool(includeMetrics); err == nil {
	// 		params.IncludeMetrics = &include
	// 	}
	// }

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

	// TODO: Add these parameters to OpenAPI spec
	// The GetJobLogsParams likely has a different structure
	// if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
	// 	if limit, err := strconv.Atoi(limitStr); err == nil {
	// 		limitVal := limit
	// 		params.Limit = &limitVal
	// 	}
	// }

	// if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
	// 	if offset, err := strconv.Atoi(offsetStr); err == nil {
	// 		offsetVal := offset
	// 		params.Offset = &offsetVal
	// 	}
	// }

	// if level := r.URL.Query().Get("level"); level != "" {
	// 	levelVal := generated.LogLevel(level)
	// 	params.Level = &levelVal
	// }

	// Check if Since is actually a time.Time type
	if since := r.URL.Query().Get("since"); since != "" {
		// For now, just comment out as it expects time.Time, not string
		_ = since
		// params.Since = &since
	}

	// if until := r.URL.Query().Get("until"); until != "" {
	// 	params.Until = &until
	// }

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

	// TODO: Add these parameters to OpenAPI spec
	// if includeFiles := r.URL.Query().Get("include_files"); includeFiles != "" {
	// 	if include, err := strconv.ParseBool(includeFiles); err == nil {
	// 		params.IncludeFiles = &include
	// 	}
	// }

	// if includeFunctions := r.URL.Query().Get("include_functions"); includeFunctions != "" {
	// 	if include, err := strconv.ParseBool(includeFunctions); err == nil {
	// 		params.IncludeFunctions = &include
	// 	}
	// }

	// if includeLines := r.URL.Query().Get("include_lines"); includeLines != "" {
	// 	if include, err := strconv.ParseBool(includeLines); err == nil {
	// 		params.IncludeLines = &include
	// 	}
	// }

	// if includeBranches := r.URL.Query().Get("include_branches"); includeBranches != "" {
	// 	if include, err := strconv.ParseBool(includeBranches); err == nil {
	// 		params.IncludeBranches = &include
	// 	}
	// }

	// if minCoverage := r.URL.Query().Get("min_coverage"); minCoverage != "" {
	// 	if coverage, err := strconv.ParseFloat(minCoverage, 32); err == nil {
	// 		coverageVal := float32(coverage)
	// 		params.MinCoverage = &coverageVal
	// 	}
	// }

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

	// TODO: Add includeContent parameter to OpenAPI spec
	// if includeContent := r.URL.Query().Get("include_content"); includeContent != "" {
	// 	if include, err := strconv.ParseBool(includeContent); err == nil {
	// 		params.IncludeContent = &include
	// 	}
	// }

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
