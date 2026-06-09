package handlers

import (
	"net/http"
	"strconv"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
)

// HandleListBots handles GET /api/v1/bots
func (h *Handlers) HandleListBots(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	params := generated.ListBotsParams{}

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
		statusVal := generated.BotStatus(status)
		params.Status = &statusVal
	}

	// Delegate to adapter
	h.adapter.ListBots(w, r, params)
}

// HandleCreateBot handles POST /api/v1/bots
func (h *Handlers) HandleCreateBot(w http.ResponseWriter, r *http.Request) {
	h.adapter.CreateBot(w, r)
}

// HandleGetBot handles GET /api/v1/bots/{id}
func (h *Handlers) HandleGetBot(w http.ResponseWriter, r *http.Request) {
	botId := h.extractBotID(r)

	// Parse query parameters
	params := generated.GetBotParams{}

	// Delegate to adapter
	h.adapter.GetBot(w, r, botId, params)
}

// HandleUpdateBot handles PUT /api/v1/bots/{id}
func (h *Handlers) HandleUpdateBot(w http.ResponseWriter, r *http.Request) {
	botId := h.extractBotID(r)
	h.adapter.UpdateBot(w, r, botId)
}

// HandleDeleteBot handles DELETE /api/v1/bots/{id}
func (h *Handlers) HandleDeleteBot(w http.ResponseWriter, r *http.Request) {
	botId := h.extractBotID(r)
	h.adapter.DeleteBot(w, r, botId)
}

// HandleBotHeartbeat handles POST /api/v1/bots/{id}/heartbeat
func (h *Handlers) HandleBotHeartbeat(w http.ResponseWriter, r *http.Request) {
	botId := h.extractBotID(r)
	h.adapter.SendBotHeartbeat(w, r, botId)
}

// HandleGetBotJobs handles GET /api/v1/bots/{id}/jobs
func (h *Handlers) HandleGetBotJobs(w http.ResponseWriter, r *http.Request) {
	botId := h.extractBotID(r)

	// Parse query parameters
	params := generated.GetBotJobsParams{}

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

	// Delegate to adapter
	h.adapter.GetBotJobs(w, r, botId, params)
}
