package errors

import "fmt"

// ApplicationError represents an error in the application layer
type ApplicationError struct {
	Code    string
	Message string
	Details map[string]interface{}
	Cause   error
}

// Error implements the error interface
func (e ApplicationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the cause of the error
func (e ApplicationError) Unwrap() error {
	return e.Cause
}

// NewApplicationError creates a new application error
func NewApplicationError(code, message string, cause error) ApplicationError {
	return ApplicationError{
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
		Cause:   cause,
	}
}

// WithDetails adds additional details to the error
func (e ApplicationError) WithDetails(key string, value interface{}) ApplicationError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// Common application error codes
const (
	ErrCodeValidationFailed  = "VALIDATION_FAILED"
	ErrCodeCommandFailed     = "COMMAND_FAILED"
	ErrCodeQueryFailed       = "QUERY_FAILED"
	ErrCodeTransactionFailed = "TRANSACTION_FAILED"
	ErrCodeIntegrationFailed = "INTEGRATION_FAILED"
	ErrCodeTimeout           = "TIMEOUT"
	ErrCodeCancelled         = "CANCELLED"
)

// IsApplicationError checks if an error is an ApplicationError
func IsApplicationError(err error) bool {
	_, ok := err.(ApplicationError)
	return ok
}
