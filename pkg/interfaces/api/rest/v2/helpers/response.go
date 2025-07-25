package helpers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/sirupsen/logrus"
)

// contextKey is a custom type for context keys
type contextKey string

const (
	// LoggerKey is the context key for the logger
	LoggerKey contextKey = "logger"
	// RequestIDKey is the context key for request ID
	RequestIDKey contextKey = "request_id"
)

// GetLogger retrieves the logger from the request context
func GetLogger(ctx context.Context) logrus.FieldLogger {
	if logger, ok := ctx.Value(LoggerKey).(logrus.FieldLogger); ok {
		return logger
	}
	// Return a default logger if none is found
	return logrus.StandardLogger()
}

// WriteJSON writes a JSON response
func WriteJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Log error but don't write it to response (headers already sent)
		GetLogger(context.Background()).WithError(err).Error("Failed to encode JSON response")
	}
}

// WriteError writes an error response
func WriteError(w http.ResponseWriter, statusCode int, code, message string, err error) {
	requestID := w.Header().Get("X-Request-ID")

	response := map[string]interface{}{
		"error": map[string]interface{}{
			"code":       code,
			"message":    message,
			"request_id": requestID,
		},
	}

	if err != nil && statusCode >= 500 {
		// Only include error details for server errors
		response["error"].(map[string]interface{})["details"] = err.Error()
	}

	WriteJSON(w, statusCode, response)
}

// WriteSuccess writes a success response with optional metadata and pagination
func WriteSuccess(w http.ResponseWriter, statusCode int, data interface{}, metadata, pagination interface{}) {
	requestID := w.Header().Get("X-Request-ID")

	response := map[string]interface{}{
		"data":       data,
		"request_id": requestID,
	}

	if metadata != nil {
		response["metadata"] = metadata
	}

	if pagination != nil {
		response["pagination"] = pagination
	}

	WriteJSON(w, statusCode, response)
}
