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
