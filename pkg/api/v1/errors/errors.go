package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// PandaFuzzError is the interface that all custom errors should implement
// to be properly converted to RFC 7807 Problem Details
type PandaFuzzError interface {
	error
	ToProblem() *Problem
}

// ValidationErrorDetail represents a single validation error
type ValidationErrorDetail struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Value   any    `json:"value,omitempty"`
}

// ValidationErrors represents multiple validation errors
type ValidationErrors struct {
	Message string                  `json:"message"`
	Errors  []ValidationErrorDetail `json:"errors"`
}

// Error implements the error interface
func (ve *ValidationErrors) Error() string {
	if ve.Message != "" {
		return ve.Message
	}
	return "validation failed"
}

// ToProblem converts ValidationErrors to RFC 7807 Problem Details
func (ve *ValidationErrors) ToProblem() *Problem {
	fieldErrors := make([]FieldError, len(ve.Errors))
	for i, err := range ve.Errors {
		fieldErrors[i] = FieldError{
			Field:   err.Field,
			Code:    err.Code,
			Message: err.Message,
			Value:   err.Value,
		}
	}

	problem := NewValidationProblem(ve.Message, fieldErrors)
	return problem
}

// NewValidationErrors creates a new ValidationErrors instance
func NewValidationErrors(message string, errors []ValidationErrorDetail) *ValidationErrors {
	if message == "" {
		message = "One or more validation errors occurred"
	}
	return &ValidationErrors{
		Message: message,
		Errors:  errors,
	}
}

// ResourceNotFoundError represents a resource not found error
type ResourceNotFoundError struct {
	Resource   string `json:"resource"`
	Identifier string `json:"identifier,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Error implements the error interface
func (e *ResourceNotFoundError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Identifier != "" {
		return fmt.Sprintf("%s with identifier '%s' not found", e.Resource, e.Identifier)
	}
	return fmt.Sprintf("%s not found", e.Resource)
}

// ToProblem converts ResourceNotFoundError to RFC 7807 Problem Details
func (e *ResourceNotFoundError) ToProblem() *Problem {
	problem := NewNotFoundProblem(e.Resource)
	if e.Identifier != "" {
		problem.AddExtension("identifier", e.Identifier)
	}
	if e.Message != "" {
		problem.Detail = e.Message
	}
	return problem
}

// NewResourceNotFoundError creates a new ResourceNotFoundError
func NewResourceNotFoundError(resource, identifier string) *ResourceNotFoundError {
	return &ResourceNotFoundError{
		Resource:   resource,
		Identifier: identifier,
	}
}

// AuthenticationError represents an authentication failure
type AuthenticationError struct {
	Reason  string `json:"reason"`
	Message string `json:"message,omitempty"`
}

// Error implements the error interface
func (e *AuthenticationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Reason != "" {
		return fmt.Sprintf("authentication failed: %s", e.Reason)
	}
	return "authentication failed"
}

// ToProblem converts AuthenticationError to RFC 7807 Problem Details
func (e *AuthenticationError) ToProblem() *Problem {
	reason := e.Reason
	if reason == "" {
		reason = "Authentication is required to access this resource"
	}
	problem := NewUnauthorizedProblem(reason)
	if e.Message != "" {
		problem.Detail = e.Message
	}
	return problem
}

// NewAuthenticationError creates a new AuthenticationError
func NewAuthenticationError(reason string) *AuthenticationError {
	return &AuthenticationError{
		Reason: reason,
	}
}

// AuthorizationError represents an authorization failure
type AuthorizationError struct {
	Resource   string `json:"resource"`
	Permission string `json:"permission,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Error implements the error interface
func (e *AuthorizationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Permission != "" {
		return fmt.Sprintf("access to %s requires %s permission", e.Resource, e.Permission)
	}
	return fmt.Sprintf("access to %s is forbidden", e.Resource)
}

// ToProblem converts AuthorizationError to RFC 7807 Problem Details
func (e *AuthorizationError) ToProblem() *Problem {
	problem := NewForbiddenProblem(e.Resource)
	if e.Permission != "" {
		problem.AddExtension("required_permission", e.Permission)
	}
	if e.Message != "" {
		problem.Detail = e.Message
	}
	return problem
}

// NewAuthorizationError creates a new AuthorizationError
func NewAuthorizationError(resource, permission string) *AuthorizationError {
	return &AuthorizationError{
		Resource:   resource,
		Permission: permission,
	}
}

// ConflictError represents a resource conflict
type ConflictError struct {
	Resource   string `json:"resource"`
	Identifier string `json:"identifier,omitempty"`
	Reason     string `json:"reason"`
	Message    string `json:"message,omitempty"`
}

// Error implements the error interface
func (e *ConflictError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Identifier != "" {
		return fmt.Sprintf("conflict with %s '%s': %s", e.Resource, e.Identifier, e.Reason)
	}
	return fmt.Sprintf("conflict with %s: %s", e.Resource, e.Reason)
}

// ToProblem converts ConflictError to RFC 7807 Problem Details
func (e *ConflictError) ToProblem() *Problem {
	problem := NewConflictProblem(e.Resource, e.Reason)
	if e.Identifier != "" {
		problem.AddExtension("identifier", e.Identifier)
	}
	if e.Message != "" {
		problem.Detail = e.Message
	}
	return problem
}

// NewConflictError creates a new ConflictError
func NewConflictError(resource, identifier, reason string) *ConflictError {
	return &ConflictError{
		Resource:   resource,
		Identifier: identifier,
		Reason:     reason,
	}
}

// RateLimitExceededError represents a rate limit violation
type RateLimitExceededError struct {
	Limit      int    `json:"limit"`
	Window     string `json:"window"`
	RetryAfter int    `json:"retry_after"`
	Message    string `json:"message,omitempty"`
}

// Error implements the error interface
func (e *RateLimitExceededError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("rate limit exceeded: %d requests per %s", e.Limit, e.Window)
}

// ToProblem converts RateLimitExceededError to RFC 7807 Problem Details
func (e *RateLimitExceededError) ToProblem() *Problem {
	problem := NewRateLimitProblem(e.RetryAfter)
	problem.AddExtension("limit", e.Limit)
	problem.AddExtension("window", e.Window)
	if e.Message != "" {
		problem.Detail = e.Message
	}
	return problem
}

// NewRateLimitExceededError creates a new RateLimitExceededError
func NewRateLimitExceededError(limit int, window string, retryAfter int) *RateLimitExceededError {
	return &RateLimitExceededError{
		Limit:      limit,
		Window:     window,
		RetryAfter: retryAfter,
	}
}

// BusinessLogicError represents a domain-specific business logic error
type BusinessLogicError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// Error implements the error interface
func (e *BusinessLogicError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("[%s] %s", e.Code, e.Message)
	}
	return fmt.Sprintf("[%s] business logic error", e.Code)
}

// ToProblem converts BusinessLogicError to RFC 7807 Problem Details
func (e *BusinessLogicError) ToProblem() *Problem {
	problem := NewBadRequestProblem(e.Message)
	problem.AddExtension("code", e.Code)
	for key, value := range e.Details {
		problem.AddExtension(key, value)
	}
	return problem
}

// NewBusinessLogicError creates a new BusinessLogicError
func NewBusinessLogicError(code, message string) *BusinessLogicError {
	return &BusinessLogicError{
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
	}
}

// WithDetail adds a detail to the business logic error
func (e *BusinessLogicError) WithDetail(key string, value interface{}) *BusinessLogicError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// ExternalServiceError represents an error from an external service
type ExternalServiceError struct {
	Service    string `json:"service"`
	Operation  string `json:"operation"`
	StatusCode int    `json:"status_code,omitempty"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable"`
}

// Error implements the error interface
func (e *ExternalServiceError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("external service %s failed on %s (status %d): %s",
			e.Service, e.Operation, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("external service %s failed on %s: %s",
		e.Service, e.Operation, e.Message)
}

// ToProblem converts ExternalServiceError to RFC 7807 Problem Details
func (e *ExternalServiceError) ToProblem() *Problem {
	var problem *Problem
	if e.Retryable {
		problem = NewServiceUnavailableProblem(e.Message)
	} else {
		problem = NewInternalErrorProblem(e.Message)
	}

	problem.AddExtension("external_service", e.Service)
	problem.AddExtension("operation", e.Operation)
	problem.AddExtension("retryable", e.Retryable)

	if e.StatusCode > 0 {
		problem.AddExtension("external_status_code", e.StatusCode)
	}

	return problem
}

// NewExternalServiceError creates a new ExternalServiceError
func NewExternalServiceError(service, operation string, statusCode int, message string, retryable bool) *ExternalServiceError {
	return &ExternalServiceError{
		Service:    service,
		Operation:  operation,
		StatusCode: statusCode,
		Message:    message,
		Retryable:  retryable,
	}
}

// TimeoutError represents an operation timeout
type TimeoutError struct {
	Operation string `json:"operation"`
	Timeout   string `json:"timeout"`
	Message   string `json:"message,omitempty"`
}

// Error implements the error interface
func (e *TimeoutError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("operation %s timed out after %s", e.Operation, e.Timeout)
}

// ToProblem converts TimeoutError to RFC 7807 Problem Details
func (e *TimeoutError) ToProblem() *Problem {
	problem := NewInternalErrorProblem(e.Error())
	problem.AddExtension("operation", e.Operation)
	problem.AddExtension("timeout", e.Timeout)
	problem.AddExtension("error_type", "timeout")
	return problem
}

// NewTimeoutError creates a new TimeoutError
func NewTimeoutError(operation, timeout string) *TimeoutError {
	return &TimeoutError{
		Operation: operation,
		Timeout:   timeout,
	}
}

// PayloadTooLargeError represents a payload size error
type PayloadTooLargeError struct {
	MaxSize     int64  `json:"max_size"`
	ActualSize  int64  `json:"actual_size,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Message     string `json:"message,omitempty"`
}

// Error implements the error interface
func (e *PayloadTooLargeError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.ActualSize > 0 {
		return fmt.Sprintf("payload size %d bytes exceeds maximum allowed size %d bytes",
			e.ActualSize, e.MaxSize)
	}
	return fmt.Sprintf("payload exceeds maximum allowed size %d bytes", e.MaxSize)
}

// ToProblem converts PayloadTooLargeError to RFC 7807 Problem Details
func (e *PayloadTooLargeError) ToProblem() *Problem {
	problem := NewPayloadTooLargeProblem(e.MaxSize)
	if e.ActualSize > 0 {
		problem.AddExtension("actual_size_bytes", e.ActualSize)
	}
	if e.ContentType != "" {
		problem.AddExtension("content_type", e.ContentType)
	}
	if e.Message != "" {
		problem.Detail = e.Message
	}
	return problem
}

// NewPayloadTooLargeError creates a new PayloadTooLargeError
func NewPayloadTooLargeError(maxSize, actualSize int64, contentType string) *PayloadTooLargeError {
	return &PayloadTooLargeError{
		MaxSize:     maxSize,
		ActualSize:  actualSize,
		ContentType: contentType,
	}
}

// Error wrapping utilities

// WrapError wraps an error with additional context, preserving PandaFuzzError interface
func WrapError(err error, message string) error {
	if err == nil {
		return nil
	}

	// If it's already a PandaFuzzError, preserve it
	if pandaErr, ok := err.(PandaFuzzError); ok {
		return &wrappedPandaFuzzError{
			PandaFuzzError: pandaErr,
			context:        message,
		}
	}

	// Otherwise, wrap as a generic error
	return fmt.Errorf("%s: %w", message, err)
}

// wrappedPandaFuzzError preserves the PandaFuzzError interface while adding context
type wrappedPandaFuzzError struct {
	PandaFuzzError
	context string
}

// Error implements the error interface with additional context
func (w *wrappedPandaFuzzError) Error() string {
	return fmt.Sprintf("%s: %s", w.context, w.PandaFuzzError.Error())
}

// ToProblem implements PandaFuzzError interface, delegating to the wrapped error
func (w *wrappedPandaFuzzError) ToProblem() *Problem {
	problem := w.PandaFuzzError.ToProblem()
	// Add context as an extension
	problem.AddExtension("context", w.context)
	return problem
}

// ChainErrors creates a chain of errors with context
func ChainErrors(errors ...error) error {
	var result error
	for i, err := range errors {
		if err == nil {
			continue
		}
		if result == nil {
			result = err
		} else {
			result = fmt.Errorf("error %d: %w; %s", i+1, err, result.Error())
		}
	}
	return result
}

// ErrorFromStatusCode converts an HTTP status code to an appropriate PandaFuzzError
func ErrorFromStatusCode(statusCode int, message string) PandaFuzzError {
	switch statusCode {
	case http.StatusBadRequest:
		return NewBusinessLogicError("BAD_REQUEST", message)
	case http.StatusUnauthorized:
		return NewAuthenticationError(message)
	case http.StatusForbidden:
		return NewAuthorizationError("resource", "")
	case http.StatusNotFound:
		return NewResourceNotFoundError("resource", "")
	case http.StatusConflict:
		return NewConflictError("resource", "", message)
	case http.StatusRequestEntityTooLarge:
		return NewPayloadTooLargeError(0, 0, "")
	case http.StatusTooManyRequests:
		return NewRateLimitExceededError(0, "", 60)
	case http.StatusServiceUnavailable:
		return NewExternalServiceError("external", "request", statusCode, message, true)
	default:
		return NewBusinessLogicError("UNKNOWN_ERROR", message)
	}
}

// IsRetryableError checks if an error is retryable
func IsRetryableError(err error) bool {
	if pandaErr, ok := err.(PandaFuzzError); ok {
		problem := pandaErr.ToProblem()
		// Check for 5xx errors or specific retryable conditions
		return problem.Status >= 500 ||
			problem.Status == http.StatusTooManyRequests ||
			problem.Status == http.StatusRequestTimeout
	}

	// Check specific error types
	switch err.(type) {
	case *ExternalServiceError:
		extErr := err.(*ExternalServiceError)
		return extErr.Retryable
	case *TimeoutError:
		return true
	case *RateLimitExceededError:
		return true
	default:
		return false
	}
}

// IsClientError checks if an error is a client error (4xx)
func IsClientError(err error) bool {
	if pandaErr, ok := err.(PandaFuzzError); ok {
		problem := pandaErr.ToProblem()
		return problem.Status >= 400 && problem.Status < 500
	}
	return false
}

// IsServerError checks if an error is a server error (5xx)
func IsServerError(err error) bool {
	if pandaErr, ok := err.(PandaFuzzError); ok {
		problem := pandaErr.ToProblem()
		return problem.Status >= 500
	}
	return false
}

// GetStatusCode extracts the HTTP status code from an error
func GetStatusCode(err error) int {
	if pandaErr, ok := err.(PandaFuzzError); ok {
		return pandaErr.ToProblem().Status
	}
	return http.StatusInternalServerError
}

// ConvertToJSON converts an error to JSON for logging or debugging
func ConvertToJSON(err error) ([]byte, error) {
	if pandaErr, ok := err.(PandaFuzzError); ok {
		return json.Marshal(pandaErr.ToProblem())
	}

	// Fallback for regular errors
	errorInfo := map[string]interface{}{
		"error":  err.Error(),
		"type":   fmt.Sprintf("%T", err),
		"status": http.StatusInternalServerError,
	}

	return json.Marshal(errorInfo)
}
