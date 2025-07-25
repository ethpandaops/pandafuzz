package crash

import (
	"encoding/json"
	"net/http"
	"time"
)

// writeJSONResponse writes a JSON response to the HTTP response writer
func (h *Handler) writeJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.WithError(err).Error("Failed to encode response")
	}
}

// writeErrorResponse writes an error response to the HTTP response writer
func (h *Handler) writeErrorResponse(w http.ResponseWriter, statusCode int, message string, err error) {
	response := ErrorResponse{
		Error:     http.StatusText(statusCode),
		Message:   message,
		Timestamp: time.Now(),
	}

	if err != nil {
		response.Details = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.WithError(err).Error("Failed to encode error response")
	}
}
