package handlers

import (
	"net/http"
)

// HandleBatchOperations handles POST /api/v1/batch
// Processes multiple operations in a single request with transaction support
func (h *Handlers) HandleBatchOperations(w http.ResponseWriter, r *http.Request) {
	// The adapter handles all the batch processing logic including:
	// - Request validation
	// - Transaction management
	// - Individual operation execution
	// - Error handling and rollback
	// - Response aggregation
	h.adapter.ExecuteBatch(w, r)
}
