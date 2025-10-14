package handlers

import (
	"net/http"
)

// HandleHealth handles GET /health
// Returns system health status with detailed component checks
func (h *Handlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	// The adapter handles health check logic including:
	// - Database connectivity
	// - Storage backend status
	// - Bot registration status
	// - Queue system health
	// - External service dependencies
	// - System resource utilization
	h.adapter.GetHealth(w, r)
}

// HandleReady handles GET /ready
// Returns readiness status for load balancers and orchestration systems
func (h *Handlers) HandleReady(w http.ResponseWriter, r *http.Request) {
	// The adapter handles readiness check logic including:
	// - Service initialization completion
	// - Critical dependency availability
	// - Configuration validation
	// - Resource allocation
	h.adapter.GetReadiness(w, r)
}
