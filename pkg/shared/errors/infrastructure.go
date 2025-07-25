package errors

import "fmt"

// InfrastructureError represents an error in the infrastructure layer
type InfrastructureError struct {
	Code    string
	Message string
	Details map[string]interface{}
	Cause   error
}

// Error implements the error interface
func (e InfrastructureError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the cause of the error
func (e InfrastructureError) Unwrap() error {
	return e.Cause
}

// NewInfrastructureError creates a new infrastructure error
func NewInfrastructureError(code, message string, cause error) InfrastructureError {
	return InfrastructureError{
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
		Cause:   cause,
	}
}

// WithDetails adds additional details to the error
func (e InfrastructureError) WithDetails(key string, value interface{}) InfrastructureError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// Common infrastructure error codes
const (
	ErrCodeDatabaseConnection = "DATABASE_CONNECTION"
	ErrCodeDatabaseQuery      = "DATABASE_QUERY"
	ErrCodeStorageRead        = "STORAGE_READ"
	ErrCodeStorageWrite       = "STORAGE_WRITE"
	ErrCodeNetworkFailure     = "NETWORK_FAILURE"
	ErrCodeCacheFailure       = "CACHE_FAILURE"
	ErrCodeMessageBusFailure  = "MESSAGE_BUS_FAILURE"
	ErrCodeExternalService    = "EXTERNAL_SERVICE"
)

// IsInfrastructureError checks if an error is an InfrastructureError
func IsInfrastructureError(err error) bool {
	_, ok := err.(InfrastructureError)
	return ok
}

// IsRetryable checks if an infrastructure error is retryable
func (e InfrastructureError) IsRetryable() bool {
	switch e.Code {
	case ErrCodeDatabaseConnection, ErrCodeNetworkFailure, ErrCodeExternalService:
		return true
	default:
		return false
	}
}
