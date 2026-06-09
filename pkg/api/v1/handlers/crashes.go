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

	// Delegate to adapter
	h.adapter.ListCrashes(w, r, params)
}

// HandleGetCrash handles GET /api/v1/crashes/{id}
func (h *Handlers) HandleGetCrash(w http.ResponseWriter, r *http.Request) {
	crashId := h.extractCrashID(r)

	// Parse query parameters
	params := generated.GetCrashParams{}

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
