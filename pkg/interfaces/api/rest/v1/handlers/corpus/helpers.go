package corpus

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

// writeJSONResponse writes a JSON response to the HTTP writer
func writeJSONResponse(w http.ResponseWriter, data interface{}, logger logrus.FieldLogger) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.WithError(err).Error("Failed to encode response")
	}
}

// writeErrorResponse writes an error response to the HTTP writer
func writeErrorResponse(w http.ResponseWriter, statusCode int, message string, err error, logger logrus.FieldLogger) {
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
		logger.WithError(err).Error("Failed to encode error response")
	}
}
