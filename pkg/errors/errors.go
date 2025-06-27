package errors

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"
)

// ErrorType represents the type of error
type ErrorType string

const (
	// System errors
	ErrorTypeSystem      ErrorType = "system"
	ErrorTypeDatabase    ErrorType = "database"
	ErrorTypeNetwork     ErrorType = "network"
	ErrorTypeStorage     ErrorType = "storage"
	ErrorTypeTimeout     ErrorType = "timeout"
	ErrorTypeConfig      ErrorType = "config"
	
	// Business logic errors
	ErrorTypeValidation  ErrorType = "validation"
	ErrorTypeNotFound    ErrorType = "not_found"
	ErrorTypeConflict    ErrorType = "conflict"
	ErrorTypeUnauthorized ErrorType = "unauthorized"
	ErrorTypeForbidden   ErrorType = "forbidden"
	
	// Job-specific errors
	ErrorTypeJobFailed   ErrorType = "job_failed"
	ErrorTypeBotOffline  ErrorType = "bot_offline"
	ErrorTypeCapability  ErrorType = "capability"
)

// Error represents a structured error with context
type Error struct {
	Type      ErrorType              `json:"type"`
	Operation string                 `json:"operation"`
	Message   string                 `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	Cause     error                  `json:"-"`
	Stack     []string               `json:"stack,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// Error implements the error interface
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s error in %s: %s (caused by: %v)", e.Type, e.Operation, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s error in %s: %s", e.Type, e.Operation, e.Message)
}

// Unwrap returns the underlying error
func (e *Error) Unwrap() error {
	return e.Cause
}

// WithDetail adds a detail to the error
func (e *Error) WithDetail(key string, value any) *Error {
	if e.Details == nil {
		e.Details = make(map[string]any)
	}
	e.Details[key] = value
	return e
}

// WithDetails adds multiple details to the error
func (e *Error) WithDetails(details map[string]any) *Error {
	if e.Details == nil {
		e.Details = make(map[string]any)
	}
	for k, v := range details {
		e.Details[k] = v
	}
	return e
}

// MarshalJSON implements json.Marshaler
func (e *Error) MarshalJSON() ([]byte, error) {
	type Alias Error
	return json.Marshal(&struct {
		*Alias
		Error string `json:"error"`
	}{
		Alias: (*Alias)(e),
		Error: e.Error(),
	})
}

// New creates a new error
func New(errorType ErrorType, operation string, message string) *Error {
	return &Error{
		Type:      errorType,
		Operation: operation,
		Message:   message,
		Timestamp: time.Now(),
		Stack:     captureStack(),
	}
}

// Wrap wraps an existing error
func Wrap(errorType ErrorType, operation string, message string, cause error) *Error {
	return &Error{
		Type:      errorType,
		Operation: operation,
		Message:   message,
		Cause:     cause,
		Timestamp: time.Now(),
		Stack:     captureStack(),
	}
}

// captureStack captures the current call stack
func captureStack() []string {
	var stack []string
	pcs := make([]uintptr, 10)
	n := runtime.Callers(3, pcs)
	
	for i := 0; i < n; i++ {
		pc := pcs[i]
		fn := runtime.FuncForPC(pc)
		if fn != nil {
			file, line := fn.FileLine(pc)
			stack = append(stack, fmt.Sprintf("%s:%d %s", file, line, fn.Name()))
		}
	}
	
	return stack
}

// Common error constructors

// NewSystemError creates a system error
func NewSystemError(operation string, cause error) *Error {
	return Wrap(ErrorTypeSystem, operation, "System error occurred", cause)
}

// NewDatabaseError creates a database error
func NewDatabaseError(operation string, cause error) *Error {
	return Wrap(ErrorTypeDatabase, operation, "Database error occurred", cause)
}

// NewNetworkError creates a network error
func NewNetworkError(operation string, cause error) *Error {
	return Wrap(ErrorTypeNetwork, operation, "Network error occurred", cause)
}

// NewStorageError creates a storage error
func NewStorageError(operation string, cause error) *Error {
	return Wrap(ErrorTypeStorage, operation, "Storage error occurred", cause)
}

// NewTimeoutError creates a timeout error
func NewTimeoutError(operation string, message string) *Error {
	return New(ErrorTypeTimeout, operation, message)
}

// NewValidationError creates a validation error
func NewValidationError(operation string, message string) *Error {
	return New(ErrorTypeValidation, operation, message)
}

// NewNotFoundError creates a not found error
func NewNotFoundError(operation string, resource string) *Error {
	return New(ErrorTypeNotFound, operation, fmt.Sprintf("%s not found", resource))
}

// NewConflictError creates a conflict error
func NewConflictError(operation string, message string) *Error {
	return New(ErrorTypeConflict, operation, message)
}

// NewUnauthorizedError creates an unauthorized error
func NewUnauthorizedError(operation string) *Error {
	return New(ErrorTypeUnauthorized, operation, "Unauthorized access")
}

// NewForbiddenError creates a forbidden error
func NewForbiddenError(operation string) *Error {
	return New(ErrorTypeForbidden, operation, "Access forbidden")
}

// Error type checking functions

// IsSystemError checks if error is a system error
func IsSystemError(err error) bool {
	return isErrorType(err, ErrorTypeSystem)
}

// IsDatabaseError checks if error is a database error
func IsDatabaseError(err error) bool {
	return isErrorType(err, ErrorTypeDatabase)
}

// IsNetworkError checks if error is a network error
func IsNetworkError(err error) bool {
	return isErrorType(err, ErrorTypeNetwork)
}

// IsStorageError checks if error is a storage error
func IsStorageError(err error) bool {
	return isErrorType(err, ErrorTypeStorage)
}

// IsTimeoutError checks if error is a timeout error
func IsTimeoutError(err error) bool {
	return isErrorType(err, ErrorTypeTimeout)
}

// IsValidationError checks if error is a validation error
func IsValidationError(err error) bool {
	return isErrorType(err, ErrorTypeValidation)
}

// IsNotFoundError checks if error is a not found error
func IsNotFoundError(err error) bool {
	return isErrorType(err, ErrorTypeNotFound)
}

// IsConflictError checks if error is a conflict error
func IsConflictError(err error) bool {
	return isErrorType(err, ErrorTypeConflict)
}

// IsUnauthorizedError checks if error is an unauthorized error
func IsUnauthorizedError(err error) bool {
	return isErrorType(err, ErrorTypeUnauthorized)
}

// IsForbiddenError checks if error is a forbidden error
func IsForbiddenError(err error) bool {
	return isErrorType(err, ErrorTypeForbidden)
}

// isErrorType checks if an error is of a specific type
func isErrorType(err error, errorType ErrorType) bool {
	if err == nil {
		return false
	}
	
	if e, ok := err.(*Error); ok {
		return e.Type == errorType
	}
	
	// Check wrapped errors
	if unwrapper, ok := err.(interface{ Unwrap() error }); ok {
		return isErrorType(unwrapper.Unwrap(), errorType)
	}
	
	return false
}

// GetErrorType returns the error type if it's a structured error
func GetErrorType(err error) (ErrorType, bool) {
	if err == nil {
		return "", false
	}
	
	if e, ok := err.(*Error); ok {
		return e.Type, true
	}
	
	// Check wrapped errors
	if unwrapper, ok := err.(interface{ Unwrap() error }); ok {
		return GetErrorType(unwrapper.Unwrap())
	}
	
	return "", false
}