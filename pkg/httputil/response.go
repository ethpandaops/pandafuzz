package httputil

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ResponseWriter provides utility functions for writing HTTP responses
type ResponseWriter struct {
	logger *logrus.Logger
}

// NewResponseWriter creates a new response writer with a logger
func NewResponseWriter(logger *logrus.Logger) *ResponseWriter {
	if logger == nil {
		logger = logrus.New()
	}
	return &ResponseWriter{logger: logger}
}

// WriteJSON writes a JSON response with the given status code
func (rw *ResponseWriter) WriteJSON(w http.ResponseWriter, statusCode int, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	if err := json.NewEncoder(w).Encode(data); err != nil {
		rw.logger.WithError(err).Error("Failed to encode JSON response")
		return err
	}
	
	return nil
}

// WriteJSONOK writes a successful JSON response
func (rw *ResponseWriter) WriteJSONOK(w http.ResponseWriter, data interface{}) error {
	return rw.WriteJSON(w, http.StatusOK, data)
}

// WriteJSONCreated writes a created JSON response
func (rw *ResponseWriter) WriteJSONCreated(w http.ResponseWriter, data interface{}) error {
	return rw.WriteJSON(w, http.StatusCreated, data)
}

// WriteError writes an error response based on error type
func (rw *ResponseWriter) WriteError(w http.ResponseWriter, err error) {
	statusCode := http.StatusInternalServerError
	message := "Internal server error"
	
	// Determine status code based on error type
	if errType, ok := errors.GetErrorType(err); ok {
		switch errType {
		case errors.ErrorTypeValidation:
			statusCode = http.StatusBadRequest
			message = "Validation error"
		case errors.ErrorTypeNotFound:
			statusCode = http.StatusNotFound
			message = "Resource not found"
		case errors.ErrorTypeUnauthorized:
			statusCode = http.StatusUnauthorized
			message = "Unauthorized"
		case errors.ErrorTypeForbidden:
			statusCode = http.StatusForbidden
			message = "Forbidden"
		case errors.ErrorTypeConflict:
			statusCode = http.StatusConflict
			message = "Resource conflict"
		case errors.ErrorTypeTimeout:
			statusCode = http.StatusGatewayTimeout
			message = "Request timeout"
		case errors.ErrorTypeNetwork:
			statusCode = http.StatusBadGateway
			message = "Network error"
		}
	}
	
	rw.WriteErrorWithStatus(w, statusCode, message, err)
}

// WriteErrorWithStatus writes an error response with a specific status code
func (rw *ResponseWriter) WriteErrorWithStatus(w http.ResponseWriter, statusCode int, message string, err error) {
	rw.logger.WithError(err).WithField("status", statusCode).Error(message)
	
	response := ErrorResponse{
		Error:     message,
		Timestamp: time.Now(),
		Status:    statusCode,
	}
	
	if err != nil {
		response.Details = err.Error()
	}
	
	rw.WriteJSON(w, statusCode, response)
}

// WriteNoContent writes a no content response
func (rw *ResponseWriter) WriteNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error     string    `json:"error"`
	Details   string    `json:"details,omitempty"`
	Status    int       `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

// SuccessResponse represents a standard success response
type SuccessResponse struct {
	Success   bool        `json:"success"`
	Message   string      `json:"message,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// PaginatedResponse represents a paginated response
type PaginatedResponse struct {
	Items      interface{} `json:"items"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	Limit      int         `json:"limit"`
	TotalPages int         `json:"total_pages"`
	HasNext    bool        `json:"has_next"`
	HasPrev    bool        `json:"has_prev"`
}

// NewSuccessResponse creates a new success response
func NewSuccessResponse(message string, data interface{}) SuccessResponse {
	return SuccessResponse{
		Success:   true,
		Message:   message,
		Data:      data,
		Timestamp: time.Now(),
	}
}

// NewPaginatedResponse creates a new paginated response
func NewPaginatedResponse(items interface{}, total, page, limit int) PaginatedResponse {
	totalPages := (total + limit - 1) / limit
	return PaginatedResponse{
		Items:      items,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
		HasNext:    page < totalPages,
		HasPrev:    page > 1,
	}
}

// Helper functions for common responses

// WriteJSONResponse is a standalone function for simple JSON responses
func WriteJSONResponse(w http.ResponseWriter, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(data)
}

// WriteErrorResponse is a standalone function for error responses
func WriteErrorResponse(w http.ResponseWriter, statusCode int, message string, err error) {
	response := ErrorResponse{
		Error:     message,
		Timestamp: time.Now(),
		Status:    statusCode,
	}
	
	if err != nil {
		response.Details = err.Error()
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}