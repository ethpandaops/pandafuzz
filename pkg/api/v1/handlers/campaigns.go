package handlers

import (
	"net/http"
	"strconv"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
)

// HandleListCampaigns handles GET /api/v1/campaigns
func (h *Handlers) HandleListCampaigns(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	params := generated.ListCampaignsParams{}

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
		statusVal := generated.CampaignStatus(status)
		params.Status = &statusVal
	}

	// TODO: Add these parameters to OpenAPI spec
	// if fuzzer := r.URL.Query().Get("fuzzer"); fuzzer != "" {
	// 	fuzzerVal := generated.FuzzerType(fuzzer)
	// 	params.Fuzzer = &fuzzerVal
	// }

	// if tags := r.URL.Query().Get("tags"); tags != "" {
	// 	params.Tags = &tags
	// }

	// if createdBy := r.URL.Query().Get("created_by"); createdBy != "" {
	// 	params.CreatedBy = &createdBy
	// }

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
	h.adapter.ListCampaigns(w, r, params)
}

// HandleCreateCampaign handles POST /api/v1/campaigns
func (h *Handlers) HandleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	h.adapter.CreateCampaign(w, r)
}

// HandleGetCampaign handles GET /api/v1/campaigns/{id}
func (h *Handlers) HandleGetCampaign(w http.ResponseWriter, r *http.Request) {
	campaignId := h.extractCampaignID(r)

	// Parse query parameters
	params := generated.GetCampaignParams{}

	// TODO: Add these include parameters to OpenAPI spec
	// if includeJobs := r.URL.Query().Get("include_jobs"); includeJobs != "" {
	// 	if include, err := strconv.ParseBool(includeJobs); err == nil {
	// 		params.IncludeJobs = &include
	// 	}
	// }

	// if includeStats := r.URL.Query().Get("include_stats"); includeStats != "" {
	// 	if include, err := strconv.ParseBool(includeStats); err == nil {
	// 		params.IncludeStats = &include
	// 	}
	// }

	// if includeCrashes := r.URL.Query().Get("include_crashes"); includeCrashes != "" {
	// 	if include, err := strconv.ParseBool(includeCrashes); err == nil {
	// 		params.IncludeCrashes = &include
	// 	}
	// }

	// if includeCorpus := r.URL.Query().Get("include_corpus"); includeCorpus != "" {
	// 	if include, err := strconv.ParseBool(includeCorpus); err == nil {
	// 		params.IncludeCorpus = &include
	// 	}
	// }

	// if includeCoverage := r.URL.Query().Get("include_coverage"); includeCoverage != "" {
	// 	if include, err := strconv.ParseBool(includeCoverage); err == nil {
	// 		params.IncludeCoverage = &include
	// 	}
	// }

	// Delegate to adapter
	h.adapter.GetCampaign(w, r, campaignId, params)
}

// HandleUpdateCampaign handles PUT /api/v1/campaigns/{id}
func (h *Handlers) HandleUpdateCampaign(w http.ResponseWriter, r *http.Request) {
	campaignId := h.extractCampaignID(r)
	h.adapter.UpdateCampaign(w, r, campaignId)
}

// HandleDeleteCampaign handles DELETE /api/v1/campaigns/{id}
func (h *Handlers) HandleDeleteCampaign(w http.ResponseWriter, r *http.Request) {
	campaignId := h.extractCampaignID(r)
	h.adapter.DeleteCampaign(w, r, campaignId)
}

// HandleStartCampaign handles POST /api/v1/campaigns/{id}/start
func (h *Handlers) HandleStartCampaign(w http.ResponseWriter, r *http.Request) {
	campaignId := h.extractCampaignID(r)
	h.adapter.StartCampaign(w, r, campaignId)
}

// HandleStopCampaign handles POST /api/v1/campaigns/{id}/stop
func (h *Handlers) HandleStopCampaign(w http.ResponseWriter, r *http.Request) {
	campaignId := h.extractCampaignID(r)
	h.adapter.StopCampaign(w, r, campaignId)
}

// HandleGetCampaignStats handles GET /api/v1/campaigns/{id}/stats
func (h *Handlers) HandleGetCampaignStats(w http.ResponseWriter, r *http.Request) {
	campaignId := h.extractCampaignID(r)
	h.adapter.GetCampaignStats(w, r, campaignId)
}
