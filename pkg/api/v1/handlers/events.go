package handlers

import (
	"net/http"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
)

// HandleEvents handles GET /api/v1/events
// Sets up Server-Sent Events (SSE) connection for real-time updates
func (h *Handlers) HandleEvents(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters for SSE filtering
	params := generated.GetEventStreamParams{}

	if types := r.URL.Query().Get("types"); types != "" {
		params.Types = &types
	}

	// TODO: Add these parameters to OpenAPI spec
	// Most SSE parameters are not defined in the generated types
	// The adapter should handle the raw query parameters directly

	// if campaignId := r.URL.Query().Get("campaign_id"); campaignId != "" {
	// 	campaignIdVal := generated.UUID(campaignId)
	// 	params.CampaignId = &campaignIdVal
	// }

	// if botId := r.URL.Query().Get("bot_id"); botId != "" {
	// 	botIdVal := generated.UUID(botId)
	// 	params.BotId = &botIdVal
	// }

	// if jobId := r.URL.Query().Get("job_id"); jobId != "" {
	// 	jobIdVal := generated.UUID(jobId)
	// 	params.JobId = &jobIdVal
	// }

	// if severity := r.URL.Query().Get("severity"); severity != "" {
	// 	severityVal := generated.EventSeverity(severity)
	// 	params.Severity = &severityVal
	// }

	// if bufferSize := r.URL.Query().Get("buffer_size"); bufferSize != "" {
	// 	if size, err := strconv.Atoi(bufferSize); err == nil {
	// 		sizeVal := size
	// 		params.BufferSize = &sizeVal
	// 	}
	// }

	// if heartbeatInterval := r.URL.Query().Get("heartbeat_interval"); heartbeatInterval != "" {
	// 	if interval, err := strconv.Atoi(heartbeatInterval); err == nil {
	// 		intervalVal := interval
	// 		params.HeartbeatInterval = &intervalVal
	// 	}
	// }

	// if includeHistorical := r.URL.Query().Get("include_historical"); includeHistorical != "" {
	// 	if include, err := strconv.ParseBool(includeHistorical); err == nil {
	// 		params.IncludeHistorical = &include
	// 	}
	// }

	// if since := r.URL.Query().Get("since"); since != "" {
	// 	params.Since = &since
	// }

	// The adapter handles all SSE logic including:
	// - Setting proper SSE headers
	// - Client registration and management
	// - Event filtering and subscription
	// - Connection lifecycle management
	// - Heartbeat and keep-alive
	h.adapter.GetEventStream(w, r, params)
}
