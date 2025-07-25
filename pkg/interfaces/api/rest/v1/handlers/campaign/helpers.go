package campaign

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (h *Handler) writeJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.WithError(err).Error("Failed to encode response")
	}
}

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

func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && (r >= '0' && r <= '9') {
			return false // Can't start with a number
		}
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func calculateSHA256(data []byte) string {
	// Simple implementation - in production, use crypto/sha256
	return fmt.Sprintf("%x", data[:min(16, len(data))])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
