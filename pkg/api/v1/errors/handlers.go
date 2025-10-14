package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// HandlerConfig configures error handling behavior
type HandlerConfig struct {
	// Logger for error logging
	Logger logrus.FieldLogger

	// DebugMode enables stack traces in responses
	DebugMode bool

	// IncludeRequestID adds request IDs to error responses
	IncludeRequestID bool

	// CORSEnabled enables CORS headers for error responses
	CORSEnabled bool

	// ServiceName is included in error logging
	ServiceName string
}

// DefaultHandlerConfig returns a default configuration
func DefaultHandlerConfig() *HandlerConfig {
	return &HandlerConfig{
		Logger:           logrus.New(),
		DebugMode:        false,
		IncludeRequestID: true,
		CORSEnabled:      true,
		ServiceName:      "pandafuzz-api",
	}
}

// Handler provides RFC 7807 compliant error handling
type Handler struct {
	config *HandlerConfig
}

// NewHandler creates a new error handler with the given configuration
func NewHandler(config *HandlerConfig) *Handler {
	if config == nil {
		config = DefaultHandlerConfig()
	}
	return &Handler{config: config}
}

// ErrorHandler is the main error handling function for HTTP requests
func (h *Handler) ErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	// Extract request context information
	requestID := h.getRequestID(r)
	traceID := h.getTraceID(r)

	// Convert error to Problem Details
	problem := h.convertErrorToProblem(err, requestID, traceID)

	// Log the error
	h.logError(r, problem, err)

	// Write the response
	h.writeErrorResponse(w, r, problem)
}

// ValidationError creates a validation error response
func (h *Handler) ValidationError(fields []FieldError) *Problem {
	detail := "One or more validation errors occurred"
	if len(fields) == 1 {
		detail = fields[0].Message
	}

	problem := NewValidationProblem(detail, fields)
	return problem
}

// NotFoundError creates a 404 error response
func (h *Handler) NotFoundError(resource string) *Problem {
	return NewNotFoundProblem(resource)
}

// InternalError creates a 500 error response
func (h *Handler) InternalError(err error) *Problem {
	problem := NewInternalErrorProblem("An unexpected error occurred")

	// Add debug information if enabled
	if h.config.DebugMode && err != nil {
		problem.AddExtension("debug_message", err.Error())
		if stack := debug.Stack(); len(stack) > 0 {
			problem.AddStackTrace(string(stack))
		}
	}

	return problem
}

// UnauthorizedError creates a 401 error response
func (h *Handler) UnauthorizedError(reason string) *Problem {
	return NewUnauthorizedProblem(reason)
}

// ForbiddenError creates a 403 error response
func (h *Handler) ForbiddenError(resource string) *Problem {
	return NewForbiddenProblem(resource)
}

// ConflictError creates a 409 error response
func (h *Handler) ConflictError(resource string, reason string) *Problem {
	return NewConflictProblem(resource, reason)
}

// RateLimitError creates a 429 error response
func (h *Handler) RateLimitError(retryAfter int) *Problem {
	return NewRateLimitProblem(retryAfter)
}

// BadRequestError creates a 400 error response
func (h *Handler) BadRequestError(detail string) *Problem {
	return NewBadRequestProblem(detail)
}

// ServiceUnavailableError creates a 503 error response
func (h *Handler) ServiceUnavailableError(detail string) *Problem {
	return NewServiceUnavailableProblem(detail)
}

// PayloadTooLargeError creates a 413 error response
func (h *Handler) PayloadTooLargeError(maxSize int64) *Problem {
	return NewPayloadTooLargeProblem(maxSize)
}

// WriteError is a convenience method to write an error response
func (h *Handler) WriteError(w http.ResponseWriter, r *http.Request, problem *Problem) {
	h.writeErrorResponse(w, r, problem)
}

// WriteErrorFromCode creates and writes an error response from an HTTP status code
func (h *Handler) WriteErrorFromCode(w http.ResponseWriter, r *http.Request, statusCode int, detail string) {
	var problem *Problem

	switch statusCode {
	case http.StatusBadRequest:
		problem = h.BadRequestError(detail)
	case http.StatusUnauthorized:
		problem = h.UnauthorizedError(detail)
	case http.StatusForbidden:
		problem = h.ForbiddenError("resource")
	case http.StatusNotFound:
		problem = h.NotFoundError("resource")
	case http.StatusConflict:
		problem = h.ConflictError("resource", detail)
	case http.StatusRequestEntityTooLarge:
		problem = h.PayloadTooLargeError(0)
	case http.StatusTooManyRequests:
		problem = h.RateLimitError(0)
	case http.StatusInternalServerError:
		problem = h.InternalError(fmt.Errorf("%s", detail))
	case http.StatusServiceUnavailable:
		problem = h.ServiceUnavailableError(detail)
	default:
		problem = h.InternalError(fmt.Errorf("unhandled error: %s", detail))
	}

	// Add request context
	if requestID := h.getRequestID(r); requestID != "" {
		problem.AddRequestID(requestID)
	}
	if traceID := h.getTraceID(r); traceID != "" {
		problem.AddTraceID(traceID)
	}

	h.writeErrorResponse(w, r, problem)
}

// convertErrorToProblem converts various error types to Problem Details
func (h *Handler) convertErrorToProblem(err error, requestID, traceID string) *Problem {
	var problem *Problem

	// Check if error implements PandaFuzzError interface
	if pandaErr, ok := err.(PandaFuzzError); ok {
		problem = pandaErr.ToProblem()
	} else {
		// Handle specific error types
		switch {
		case isValidationError(err):
			problem = h.ValidationError(extractFieldErrors(err))
		case isNotFoundError(err):
			problem = h.NotFoundError(extractResource(err))
		case isUnauthorizedError(err):
			problem = h.UnauthorizedError(err.Error())
		case isForbiddenError(err):
			problem = h.ForbiddenError(extractResource(err))
		case isConflictError(err):
			problem = h.ConflictError(extractResource(err), err.Error())
		case isRateLimitError(err):
			problem = h.RateLimitError(extractRetryAfter(err))
		default:
			problem = h.InternalError(err)
		}
	}

	// Add context information
	if requestID != "" {
		problem.AddRequestID(requestID)
	}
	if traceID != "" {
		problem.AddTraceID(traceID)
	}

	return problem
}

// writeErrorResponse writes the Problem Details response
func (h *Handler) writeErrorResponse(w http.ResponseWriter, r *http.Request, problem *Problem) {
	// Set CORS headers if enabled
	if h.config.CORSEnabled {
		h.setCORSHeaders(w, r)
	}

	// Determine content type based on Accept header
	contentType := h.determineContentType(r)
	w.Header().Set("Content-Type", contentType)

	// Set additional headers
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if problem.Status == http.StatusTooManyRequests {
		if retryAfter, exists := problem.Extensions["retry_after"]; exists {
			if seconds, ok := retryAfter.(int); ok {
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
			}
		}
	}

	// Write status code
	w.WriteHeader(problem.Status)

	// Write response body
	if strings.Contains(contentType, "application/json") {
		h.writeJSONError(w, problem)
	} else {
		h.writeTextError(w, problem)
	}
}

// writeJSONError writes the error as JSON
func (h *Handler) writeJSONError(w http.ResponseWriter, problem *Problem) {
	// First try to marshal the problem to ensure it's valid JSON
	jsonData, err := json.Marshal(problem)
	if err != nil {
		// Fallback to plain text if JSON marshaling fails
		h.config.Logger.WithError(err).Error("Failed to marshal error response as JSON")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "Error: %s\n", problem.Title)
		return
	}

	// Write the JSON directly
	if _, err := w.Write(jsonData); err != nil {
		h.config.Logger.WithError(err).Error("Failed to write JSON error response")
	}
}

// writeTextError writes the error as plain text
func (h *Handler) writeTextError(w http.ResponseWriter, problem *Problem) {
	fmt.Fprintf(w, "Error: %s\n", problem.Title)
	if problem.Detail != "" {
		fmt.Fprintf(w, "Detail: %s\n", problem.Detail)
	}
	fmt.Fprintf(w, "Status: %d\n", problem.Status)
}

// determineContentType determines the response content type based on Accept header
func (h *Handler) determineContentType(r *http.Request) string {
	accept := r.Header.Get("Accept")
	if accept == "" {
		return "application/json; charset=utf-8"
	}

	// Check for JSON preference
	if strings.Contains(accept, "application/json") || strings.Contains(accept, "*/*") {
		return "application/json; charset=utf-8"
	}

	// Check for text preference
	if strings.Contains(accept, "text/plain") || strings.Contains(accept, "text/*") {
		return "text/plain; charset=utf-8"
	}

	// Default to JSON
	return "application/json; charset=utf-8"
}

// setCORSHeaders sets CORS headers for error responses
func (h *Handler) setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, X-Trace-ID")
}

// logError logs the error with appropriate level and context
func (h *Handler) logError(r *http.Request, problem *Problem, originalErr error) {
	fields := logrus.Fields{
		"service":     h.config.ServiceName,
		"status_code": problem.Status,
		"error_type":  problem.Type,
		"path":        r.URL.Path,
		"method":      r.Method,
		"user_agent":  r.UserAgent(),
		"remote_addr": r.RemoteAddr,
	}

	if problem.Instance != "" {
		fields["request_id"] = problem.Instance
	}

	if traceID, exists := problem.Extensions["trace_id"]; exists {
		fields["trace_id"] = traceID
	}

	logger := h.config.Logger.WithFields(fields)

	// Log at appropriate level based on status code
	switch {
	case problem.Status >= 500:
		if originalErr != nil {
			logger.WithError(originalErr).Error("Internal server error")
		} else {
			logger.Error("Internal server error")
		}
	case problem.Status >= 400:
		logger.Warn("Client error")
	default:
		logger.Info("Error response")
	}
}

// getRequestID extracts request ID from context or headers
func (h *Handler) getRequestID(r *http.Request) string {
	if !h.config.IncludeRequestID {
		return ""
	}

	// Try to get from context
	if ctx := r.Context(); ctx != nil {
		if id := ctx.Value("request_id"); id != nil {
			if reqID, ok := id.(string); ok {
				return reqID
			}
		}
	}

	// Try to get from headers
	if reqID := r.Header.Get("X-Request-ID"); reqID != "" {
		return reqID
	}

	// Generate new one
	return generateRequestID()
}

// getTraceID extracts trace ID from context or headers
func (h *Handler) getTraceID(r *http.Request) string {
	// Try to get from context
	if ctx := r.Context(); ctx != nil {
		if id := ctx.Value("trace_id"); id != nil {
			if traceID, ok := id.(string); ok {
				return traceID
			}
		}
	}

	// Try to get from headers
	if traceID := r.Header.Get("X-Trace-ID"); traceID != "" {
		return traceID
	}

	return ""
}

// Helper functions for error type detection
func isValidationError(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "validation")
}

func isNotFoundError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "does not exist")
}

func isUnauthorizedError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unauthorized") || strings.Contains(msg, "authentication")
}

func isForbiddenError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "forbidden") || strings.Contains(msg, "access denied")
}

func isConflictError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "conflict") || strings.Contains(msg, "already exists")
}

func isRateLimitError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "rate limit") || strings.Contains(msg, "too many requests")
}

func extractFieldErrors(err error) []FieldError {
	// This would need to be implemented based on your validation library
	// For now, return a generic field error
	return []FieldError{
		{
			Field:   "unknown",
			Code:    CodeRequired,
			Message: err.Error(),
		},
	}
}

func extractResource(err error) string {
	// Extract resource name from error message
	msg := err.Error()
	if strings.Contains(msg, "campaign") {
		return "campaign"
	}
	if strings.Contains(msg, "job") {
		return "job"
	}
	if strings.Contains(msg, "bot") {
		return "bot"
	}
	if strings.Contains(msg, "corpus") {
		return "corpus"
	}
	if strings.Contains(msg, "crash") {
		return "crash"
	}
	return "resource"
}

func extractRetryAfter(err error) int {
	// Extract retry-after value from error message if present
	// Default to 60 seconds
	return 60
}

// Global handler instance for convenience
var defaultHandler = NewHandler(DefaultHandlerConfig())

// Package-level convenience functions using the default handler
func ErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	defaultHandler.ErrorHandler(w, r, err)
}

func ValidationError(fields []FieldError) *Problem {
	return defaultHandler.ValidationError(fields)
}

func NotFoundError(resource string) *Problem {
	return defaultHandler.NotFoundError(resource)
}

func InternalError(err error) *Problem {
	return defaultHandler.InternalError(err)
}

func UnauthorizedError(reason string) *Problem {
	return defaultHandler.UnauthorizedError(reason)
}

func ForbiddenError(resource string) *Problem {
	return defaultHandler.ForbiddenError(resource)
}

func ConflictErrorResponse(resource string, reason string) *Problem {
	return defaultHandler.ConflictError(resource, reason)
}

func RateLimitError(retryAfter int) *Problem {
	return defaultHandler.RateLimitError(retryAfter)
}

func BadRequestError(detail string) *Problem {
	return defaultHandler.BadRequestError(detail)
}

func WriteError(w http.ResponseWriter, r *http.Request, problem *Problem) {
	defaultHandler.WriteError(w, r, problem)
}

func WriteErrorFromCode(w http.ResponseWriter, r *http.Request, statusCode int, detail string) {
	defaultHandler.WriteErrorFromCode(w, r, statusCode, detail)
}
