// Package errors provides standardized error types and error handling utilities
// for the PandaFuzz system. This package consolidates error handling from across
// the codebase into a single location.
package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"time"
)

// ErrorCode represents standardized error codes for the system (API-level codes)
type ErrorCode string

const (
	// General error codes
	ErrCodeInternal      ErrorCode = "INTERNAL_ERROR"
	ErrCodeInvalidInput  ErrorCode = "INVALID_INPUT"
	ErrCodeNotFound      ErrorCode = "NOT_FOUND"
	ErrCodeAlreadyExists ErrorCode = "ALREADY_EXISTS"
	ErrCodeUnauthorized  ErrorCode = "UNAUTHORIZED"
	ErrCodeForbidden     ErrorCode = "FORBIDDEN"

	// Fuzzing-specific error codes
	ErrCodeFuzzerInit     ErrorCode = "FUZZER_INIT_ERROR"
	ErrCodeFuzzerExec     ErrorCode = "FUZZER_EXEC_ERROR"
	ErrCodeFuzzerTimeout  ErrorCode = "FUZZER_TIMEOUT"
	ErrCodeCorpusSync     ErrorCode = "CORPUS_SYNC_ERROR"
	ErrCodeCorpusInvalid  ErrorCode = "CORPUS_INVALID"
	ErrCodeJobInvalid     ErrorCode = "JOB_INVALID"
	ErrCodeJobNotFound    ErrorCode = "JOB_NOT_FOUND"
	ErrCodeBinaryNotFound ErrorCode = "BINARY_NOT_FOUND"

	// Storage error codes
	ErrCodeStorageRead  ErrorCode = "STORAGE_READ_ERROR"
	ErrCodeStorageWrite ErrorCode = "STORAGE_WRITE_ERROR"
	ErrCodeStorageFull  ErrorCode = "STORAGE_FULL"

	// Network error codes
	ErrCodeNetworkTimeout    ErrorCode = "NETWORK_TIMEOUT"
	ErrCodeNetworkConnection ErrorCode = "NETWORK_CONNECTION_ERROR"
)

// Sentinel errors for common error conditions
var (
	ErrNotImplemented       = errors.New("not implemented")
	ErrCampaignNotFound     = errors.New("campaign not found")
	ErrCampaignRunning      = errors.New("campaign is already running")
	ErrInvalidStackTrace    = errors.New("invalid stack trace format")
	ErrCorpusFileTooLarge   = errors.New("corpus file exceeds size limit")
	ErrDuplicateCorpusFile  = errors.New("corpus file already exists")
	ErrCampaignCompleted    = errors.New("campaign is already completed")
	ErrCampaignPaused       = errors.New("campaign is paused")
	ErrNoCampaignJobs       = errors.New("no jobs found for campaign")
	ErrInvalidCampaignState = errors.New("invalid campaign state transition")
	ErrCrashGroupNotFound   = errors.New("crash group not found")
	ErrCorpusFileNotFound   = errors.New("corpus file not found")
	ErrBinaryHashMismatch   = errors.New("binary hash mismatch between campaigns")
	ErrKeyNotFound          = errors.New("key not found")
	ErrTransactionFail      = errors.New("transaction failed")
	ErrDatabaseClosed       = errors.New("database is closed")
	ErrInvalidConfig        = errors.New("invalid database configuration")
	ErrMigrationFailed      = errors.New("database migration failed")
	ErrBackupFailed         = errors.New("database backup failed")
	ErrRestoreFailed        = errors.New("database restore failed")
)

// TimeoutErr represents an error when an operation times out
type TimeoutErr struct {
	Operation string
	Duration  time.Duration
}

// Error returns the error message for TimeoutErr
func (e *TimeoutErr) Error() string {
	return fmt.Sprintf("operation '%s' timed out after %v", e.Operation, e.Duration)
}

// RetryExhaustedError represents an error when all retry attempts have been exhausted
type RetryExhaustedError struct {
	Operation string
	Attempts  int
	LastError error
}

// Error returns the error message for RetryExhaustedError
func (e *RetryExhaustedError) Error() string {
	if e.LastError != nil {
		return fmt.Sprintf("operation '%s' failed after %d attempts: %v", e.Operation, e.Attempts, e.LastError)
	}
	return fmt.Sprintf("operation '%s' failed after %d attempts", e.Operation, e.Attempts)
}

// Unwrap returns the underlying error
func (e *RetryExhaustedError) Unwrap() error {
	return e.LastError
}

// NewRetryExhaustedError creates a new RetryExhaustedError
func NewRetryExhaustedError(operation string, attempts int, lastError error) *RetryExhaustedError {
	return &RetryExhaustedError{
		Operation: operation,
		Attempts:  attempts,
		LastError: lastError,
	}
}

// CodedError represents an error with an API-level error code
type CodedError struct {
	Code    ErrorCode      `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// NewCodedError creates a new error with an error code
func NewCodedError(code ErrorCode, message string) *CodedError {
	return &CodedError{
		Code:    code,
		Message: message,
		Details: make(map[string]any, 4),
	}
}

// Error implements the error interface
func (e *CodedError) Error() string {
	if len(e.Details) > 0 {
		return fmt.Sprintf("[%s] %s (details: %v)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// WithDetails adds additional details to the error
func (e *CodedError) WithDetails(key string, value any) *CodedError {
	if e.Details == nil {
		e.Details = make(map[string]any, 4)
	}
	e.Details[key] = value
	return e
}

// ErrorType represents the type of error
type ErrorType string

const (
	// System errors
	ErrorTypeSystem   ErrorType = "system"
	ErrorTypeDatabase ErrorType = "database"
	ErrorTypeNetwork  ErrorType = "network"
	ErrorTypeStorage  ErrorType = "storage"
	ErrorTypeTimeout  ErrorType = "timeout"
	ErrorTypeConfig   ErrorType = "config"

	// Business logic errors
	ErrorTypeValidation   ErrorType = "validation"
	ErrorTypeNotFound     ErrorType = "not_found"
	ErrorTypeConflict     ErrorType = "conflict"
	ErrorTypeUnauthorized ErrorType = "unauthorized"
	ErrorTypeForbidden    ErrorType = "forbidden"

	// Job-specific errors
	ErrorTypeJobFailed  ErrorType = "job_failed"
	ErrorTypeBotOffline ErrorType = "bot_offline"
	ErrorTypeCapability ErrorType = "capability"

	// Implementation errors
	ErrorTypeMethodNotFound ErrorType = "method_not_found"
)

// Error represents a structured error with context
type Error struct {
	Type      ErrorType      `json:"type"`
	Operation string         `json:"operation"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	Cause     error          `json:"-"`
	Stack     []string       `json:"stack,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
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

// IsMethodNotFound checks if error indicates a method is not implemented
func IsMethodNotFound(err error) bool {
	return isErrorType(err, ErrorTypeMethodNotFound)
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
