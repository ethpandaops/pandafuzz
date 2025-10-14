package handlers

import (
	"net/http"
	"strconv"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// HandleCreateCrash handles POST /api/v1/crashes
func (h *Handlers) HandleCreateCrash(w http.ResponseWriter, r *http.Request) {
	// Delegate to adapter
	h.adapter.CreateCrash(w, r)
}

// HandleListCrashes handles GET /api/v1/crashes
func (h *Handlers) HandleListCrashes(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	params := generated.ListCrashesParams{}

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

	if severity := r.URL.Query().Get("severity"); severity != "" {
		severityVal := generated.CrashSeverity(severity)
		params.Severity = &severityVal
	}

	// TODO: Add these parameters to OpenAPI spec
	// if crashType := r.URL.Query().Get("type"); crashType != "" {
	// 	typeVal := generated.CrashType(crashType)
	// 	params.Type = &typeVal
	// }

	// if fuzzer := r.URL.Query().Get("fuzzer"); fuzzer != "" {
	// 	fuzzerVal := generated.FuzzerType(fuzzer)
	// 	params.Fuzzer = &fuzzerVal
	// }

	if campaignId := r.URL.Query().Get("campaign_id"); campaignId != "" {
		if parsedUUID, err := uuid.Parse(campaignId); err == nil {
			uuidVal := openapi_types.UUID(parsedUUID)
			params.CampaignId = &uuidVal
		}
	}

	if jobId := r.URL.Query().Get("job_id"); jobId != "" {
		if parsedUUID, err := uuid.Parse(jobId); err == nil {
			uuidVal := openapi_types.UUID(parsedUUID)
			params.JobId = &uuidVal
		}
	}

	// TODO: Add these parameters to OpenAPI spec
	// if minimized := r.URL.Query().Get("minimized"); minimized != "" {
	// 	if isMinimized, err := strconv.ParseBool(minimized); err == nil {
	// 		params.Minimized = &isMinimized
	// 	}
	// }

	// if reproduced := r.URL.Query().Get("reproduced"); reproduced != "" {
	// 	if isReproduced, err := strconv.ParseBool(reproduced); err == nil {
	// 		params.Reproduced = &isReproduced
	// 	}
	// }

	// if deduplicated := r.URL.Query().Get("deduplicated"); deduplicated != "" {
	// 	if isDeduplicated, err := strconv.ParseBool(deduplicated); err == nil {
	// 		params.Deduplicated = &isDeduplicated
	// 	}
	// }

	// if sortBy := r.URL.Query().Get("sort_by"); sortBy != "" {
	// 	params.SortBy = &sortBy
	// }

	// if sortOrder := r.URL.Query().Get("sort_order"); sortOrder != "" {
	// 	sortOrderVal := generated.SortOrder(sortOrder)
	// 	params.SortOrder = &sortOrderVal
	// }

	// TODO: Add these parameters to OpenAPI spec
	// if since := r.URL.Query().Get("since"); since != "" {
	// 	params.Since = &since
	// }

	// if until := r.URL.Query().Get("until"); until != "" {
	// 	params.Until = &until
	// }

	// if includeStackTrace := r.URL.Query().Get("include_stack_trace"); includeStackTrace != "" {
	// 	if include, err := strconv.ParseBool(includeStackTrace); err == nil {
	// 		params.IncludeStackTrace = &include
	// 	}
	// }

	// if includeInput := r.URL.Query().Get("include_input"); includeInput != "" {
	// 	if include, err := strconv.ParseBool(includeInput); err == nil {
	// 		params.IncludeInput = &include
	// 	}
	// }

	// if includeAnalysis := r.URL.Query().Get("include_analysis"); includeAnalysis != "" {
	// 	if include, err := strconv.ParseBool(includeAnalysis); err == nil {
	// 		params.IncludeAnalysis = &include
	// 	}
	// }

	// Delegate to adapter
	h.adapter.ListCrashes(w, r, params)
}

// HandleGetCrash handles GET /api/v1/crashes/{id}
func (h *Handlers) HandleGetCrash(w http.ResponseWriter, r *http.Request) {
	crashId := h.extractCrashID(r)

	// Parse query parameters
	params := generated.GetCrashParams{}

	// TODO: Add these parameters to OpenAPI spec
	// if includeStackTrace := r.URL.Query().Get("include_stack_trace"); includeStackTrace != "" {
	// 	if include, err := strconv.ParseBool(includeStackTrace); err == nil {
	// 		params.IncludeStackTrace = &include
	// 	}
	// }

	// if includeInput := r.URL.Query().Get("include_input"); includeInput != "" {
	// 	if include, err := strconv.ParseBool(includeInput); err == nil {
	// 		params.IncludeInput = &include
	// 	}
	// }

	// if includeAnalysis := r.URL.Query().Get("include_analysis"); includeAnalysis != "" {
	// 	if include, err := strconv.ParseBool(includeAnalysis); err == nil {
	// 		params.IncludeAnalysis = &include
	// 	}
	// }

	// if includeReproduction := r.URL.Query().Get("include_reproduction"); includeReproduction != "" {
	// 	if include, err := strconv.ParseBool(includeReproduction); err == nil {
	// 		params.IncludeReproduction = &include
	// 	}
	// }

	// if includeMinimization := r.URL.Query().Get("include_minimization"); includeMinimization != "" {
	// 	if include, err := strconv.ParseBool(includeMinimization); err == nil {
	// 		params.IncludeMinimization = &include
	// 	}
	// }

	// if includeDeduplication := r.URL.Query().Get("include_deduplication"); includeDeduplication != "" {
	// 	if include, err := strconv.ParseBool(includeDeduplication); err == nil {
	// 		params.IncludeDeduplication = &include
	// 	}
	// }

	// Delegate to adapter
	h.adapter.GetCrash(w, r, crashId, params)
}

// HandleMinimizeCrash handles POST /api/v1/crashes/{id}/minimize
func (h *Handlers) HandleMinimizeCrash(w http.ResponseWriter, r *http.Request) {
	crashId := h.extractCrashID(r)
	h.adapter.MinimizeCrash(w, r, crashId)
}

// HandleReproduceCrash handles POST /api/v1/crashes/{id}/reproduce
func (h *Handlers) HandleReproduceCrash(w http.ResponseWriter, r *http.Request) {
	crashId := h.extractCrashID(r)
	h.adapter.ReproduceCrash(w, r, crashId)
}

// HandleDeduplicateCrash handles POST /api/v1/crashes/{id}/deduplicate
func (h *Handlers) HandleDeduplicateCrash(w http.ResponseWriter, r *http.Request) {
	crashId := h.extractCrashID(r)
	h.adapter.DeduplicateCrash(w, r, crashId)
}
