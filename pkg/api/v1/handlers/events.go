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

	// The adapter handles all SSE logic including:
	// - Setting proper SSE headers
	// - Client registration and management
	// - Event filtering and subscription
	// - Connection lifecycle management
	// - Heartbeat and keep-alive
	h.adapter.GetEventStream(w, r, params)
}
