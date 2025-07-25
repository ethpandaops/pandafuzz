package errors

import (
	"fmt"
)

// DomainError represents a business logic error in the domain layer
type DomainError struct {
	Code    string
	Message string
	Details map[string]interface{}
}

// Error implements the error interface
func (e DomainError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewDomainError creates a new domain error
func NewDomainError(code, message string) DomainError {
	return DomainError{
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
	}
}

// WithDetails adds additional details to the error
func (e DomainError) WithDetails(key string, value interface{}) DomainError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// Common domain error codes
const (
	ErrCodeNotFound           = "NOT_FOUND"
	ErrCodeAlreadyExists      = "ALREADY_EXISTS"
	ErrCodeInvalidInput       = "INVALID_INPUT"
	ErrCodeOperationFailed    = "OPERATION_FAILED"
	ErrCodeUnauthorized       = "UNAUTHORIZED"
	ErrCodeForbidden          = "FORBIDDEN"
	ErrCodeConflict           = "CONFLICT"
	ErrCodePreconditionFailed = "PRECONDITION_FAILED"
	ErrCodeResourceExhausted  = "RESOURCE_EXHAUSTED"
	ErrCodeInvalidState       = "INVALID_STATE"
)

// Common domain errors
var (
	ErrNotFound      = NewDomainError(ErrCodeNotFound, "Resource not found")
	ErrAlreadyExists = NewDomainError(ErrCodeAlreadyExists, "Resource already exists")
	ErrInvalidInput  = NewDomainError(ErrCodeInvalidInput, "Invalid input provided")
	ErrUnauthorized  = NewDomainError(ErrCodeUnauthorized, "Unauthorized access")
	ErrForbidden     = NewDomainError(ErrCodeForbidden, "Access forbidden")
	ErrInvalidState  = NewDomainError(ErrCodeInvalidState, "Invalid state for operation")
)
