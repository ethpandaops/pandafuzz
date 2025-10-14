package errors

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Problem represents RFC 7807 Problem Details for HTTP APIs
type Problem struct {
	// Type identifies the problem type (URI reference)
	Type string `json:"type"`

	// Title is a short, human-readable summary of the problem type
	Title string `json:"title"`

	// Status is the HTTP status code for this occurrence
	Status int `json:"status"`

	// Detail is a human-readable explanation specific to this occurrence
	Detail string `json:"detail,omitempty"`

	// Instance identifies the specific occurrence (URI reference)
	Instance string `json:"instance,omitempty"`

	// Timestamp when the error occurred
	Timestamp time.Time `json:"timestamp"`

	// Extensions holds any additional problem-specific fields
	Extensions map[string]any `json:"-"`
}

// FieldError represents a validation error for a specific field
type FieldError struct {
	// Field is the name of the field that failed validation
	Field string `json:"field"`

	// Code is the error code for programmatic handling
	Code string `json:"code"`

	// Message is a human-readable error message
	Message string `json:"message"`

	// Value is the rejected value (optional, may be omitted for security)
	Value any `json:"value,omitempty"`
}

// MarshalJSON customizes JSON serialization to include extensions
func (p *Problem) MarshalJSON() ([]byte, error) {
	// Create a map with all the standard fields
	result := map[string]any{
		"type":      p.Type,
		"title":     p.Title,
		"status":    p.Status,
		"timestamp": p.Timestamp,
	}

	// Add optional fields if they exist
	if p.Detail != "" {
		result["detail"] = p.Detail
	}
	if p.Instance != "" {
		result["instance"] = p.Instance
	}

	// Add extensions if they exist
	for key, value := range p.Extensions {
		result[key] = value
	}

	return json.Marshal(result)
}

// AddExtension adds a custom field to the problem details
func (p *Problem) AddExtension(key string, value any) *Problem {
	if p.Extensions == nil {
		p.Extensions = make(map[string]any)
	}
	p.Extensions[key] = value
	return p
}

// AddRequestID adds the request ID to the instance field
func (p *Problem) AddRequestID(requestID string) *Problem {
	if requestID != "" {
		p.Instance = requestID
	}
	return p
}

// AddTraceID adds a trace ID for distributed tracing
func (p *Problem) AddTraceID(traceID string) *Problem {
	if traceID != "" {
		return p.AddExtension("trace_id", traceID)
	}
	return p
}

// AddStackTrace adds stack trace information (for debug mode only)
func (p *Problem) AddStackTrace(stackTrace string) *Problem {
	if stackTrace != "" {
		return p.AddExtension("stack_trace", stackTrace)
	}
	return p
}

// AddValidationErrors adds field validation errors
func (p *Problem) AddValidationErrors(errors []FieldError) *Problem {
	if len(errors) > 0 {
		return p.AddExtension("errors", errors)
	}
	return p
}

// AddRetryAfter adds retry information for rate limiting
func (p *Problem) AddRetryAfter(seconds int) *Problem {
	if seconds > 0 {
		return p.AddExtension("retry_after", seconds)
	}
	return p
}

// Common problem type URIs
const (
	TypeValidationError    = "https://pandafuzz.io/problems/validation-error"
	TypeNotFound           = "https://pandafuzz.io/problems/not-found"
	TypeUnauthorized       = "https://pandafuzz.io/problems/unauthorized"
	TypeForbidden          = "https://pandafuzz.io/problems/forbidden"
	TypeConflict           = "https://pandafuzz.io/problems/conflict"
	TypeRateLimit          = "https://pandafuzz.io/problems/rate-limit"
	TypeInternalError      = "https://pandafuzz.io/problems/internal-error"
	TypeBadRequest         = "https://pandafuzz.io/problems/bad-request"
	TypeServiceUnavailable = "https://pandafuzz.io/problems/service-unavailable"
	TypePayloadTooLarge    = "https://pandafuzz.io/problems/payload-too-large"
)

// Common validation error codes
const (
	CodeRequired      = "REQUIRED"
	CodeInvalidFormat = "INVALID_FORMAT"
	CodeInvalidEnum   = "INVALID_ENUM"
	CodeInvalidRange  = "INVALID_RANGE"
	CodeInvalidLength = "INVALID_LENGTH"
	CodeInvalidType   = "INVALID_TYPE"
	CodeInvalidUUID   = "INVALID_UUID"
	CodeDuplicate     = "DUPLICATE"
	CodeNotFound      = "NOT_FOUND"
	CodeTooLarge      = "TOO_LARGE"
	CodeTooSmall      = "TOO_SMALL"
)

// newProblem creates a new Problem with default values
func newProblem(problemType, title string, status int) *Problem {
	return &Problem{
		Type:       problemType,
		Title:      title,
		Status:     status,
		Timestamp:  time.Now().UTC(),
		Extensions: make(map[string]any),
	}
}

// NewValidationProblem creates a validation error problem
func NewValidationProblem(detail string, fieldErrors []FieldError) *Problem {
	p := newProblem(TypeValidationError, "Validation Error", http.StatusBadRequest)
	p.Detail = detail
	if len(fieldErrors) > 0 {
		p.AddValidationErrors(fieldErrors)
	}
	return p
}

// NewNotFoundProblem creates a not found error problem
func NewNotFoundProblem(resource string) *Problem {
	p := newProblem(TypeNotFound, "Not Found", http.StatusNotFound)
	p.Detail = "The requested " + resource + " was not found"
	return p
}

// NewUnauthorizedProblem creates an unauthorized error problem
func NewUnauthorizedProblem(reason string) *Problem {
	p := newProblem(TypeUnauthorized, "Unauthorized", http.StatusUnauthorized)
	if reason != "" {
		p.Detail = reason
	} else {
		p.Detail = "Authentication is required to access this resource"
	}
	return p
}

// NewForbiddenProblem creates a forbidden error problem
func NewForbiddenProblem(resource string) *Problem {
	p := newProblem(TypeForbidden, "Forbidden", http.StatusForbidden)
	p.Detail = "Access to " + resource + " is forbidden"
	return p
}

// NewConflictProblem creates a conflict error problem
func NewConflictProblem(resource, reason string) *Problem {
	p := newProblem(TypeConflict, "Conflict", http.StatusConflict)
	if reason != "" {
		p.Detail = "Conflict with " + resource + ": " + reason
	} else {
		p.Detail = "The request conflicts with the current state of " + resource
	}
	return p
}

// NewRateLimitProblem creates a rate limit error problem
func NewRateLimitProblem(retryAfter int) *Problem {
	p := newProblem(TypeRateLimit, "Rate Limit Exceeded", http.StatusTooManyRequests)
	p.Detail = "Rate limit exceeded. Please try again later"
	if retryAfter > 0 {
		p.AddRetryAfter(retryAfter)
	}
	return p
}

// NewInternalErrorProblem creates an internal server error problem
func NewInternalErrorProblem(detail string) *Problem {
	p := newProblem(TypeInternalError, "Internal Server Error", http.StatusInternalServerError)
	if detail != "" {
		p.Detail = detail
	} else {
		p.Detail = "An unexpected error occurred"
	}
	return p
}

// NewBadRequestProblem creates a bad request error problem
func NewBadRequestProblem(detail string) *Problem {
	p := newProblem(TypeBadRequest, "Bad Request", http.StatusBadRequest)
	p.Detail = detail
	return p
}

// NewServiceUnavailableProblem creates a service unavailable error problem
func NewServiceUnavailableProblem(detail string) *Problem {
	p := newProblem(TypeServiceUnavailable, "Service Unavailable", http.StatusServiceUnavailable)
	if detail != "" {
		p.Detail = detail
	} else {
		p.Detail = "The service is temporarily unavailable"
	}
	return p
}

// NewPayloadTooLargeProblem creates a payload too large error problem
func NewPayloadTooLargeProblem(maxSize int64) *Problem {
	p := newProblem(TypePayloadTooLarge, "Payload Too Large", http.StatusRequestEntityTooLarge)
	if maxSize > 0 {
		p.Detail = "Request payload exceeds the maximum allowed size"
		p.AddExtension("max_size_bytes", maxSize)
	} else {
		p.Detail = "Request payload is too large"
	}
	return p
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return uuid.New().String()
}
