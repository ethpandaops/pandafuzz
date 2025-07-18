package common

import (
	"fmt"
	"time"
)

// ErrorCode represents standardized error codes for the system
type ErrorCode string

const (
	// General errors
	ErrCodeInternal      ErrorCode = "INTERNAL_ERROR"
	ErrCodeInvalidInput  ErrorCode = "INVALID_INPUT"
	ErrCodeNotFound      ErrorCode = "NOT_FOUND"
	ErrCodeAlreadyExists ErrorCode = "ALREADY_EXISTS"
	ErrCodeUnauthorized  ErrorCode = "UNAUTHORIZED"
	ErrCodeForbidden     ErrorCode = "FORBIDDEN"

	// Fuzzing-specific errors
	ErrCodeFuzzerInit     ErrorCode = "FUZZER_INIT_ERROR"
	ErrCodeFuzzerExec     ErrorCode = "FUZZER_EXEC_ERROR"
	ErrCodeFuzzerTimeout  ErrorCode = "FUZZER_TIMEOUT"
	ErrCodeCorpusSync     ErrorCode = "CORPUS_SYNC_ERROR"
	ErrCodeCorpusInvalid  ErrorCode = "CORPUS_INVALID"
	ErrCodeJobInvalid     ErrorCode = "JOB_INVALID"
	ErrCodeJobNotFound    ErrorCode = "JOB_NOT_FOUND"
	ErrCodeBinaryNotFound ErrorCode = "BINARY_NOT_FOUND"

	// Storage errors
	ErrCodeStorageRead  ErrorCode = "STORAGE_READ_ERROR"
	ErrCodeStorageWrite ErrorCode = "STORAGE_WRITE_ERROR"
	ErrCodeStorageFull  ErrorCode = "STORAGE_FULL"

	// Network errors
	ErrCodeNetworkTimeout    ErrorCode = "NETWORK_TIMEOUT"
	ErrCodeNetworkConnection ErrorCode = "NETWORK_CONNECTION_ERROR"
)

// Error represents a standardized error in the system
type Error struct {
	Code    ErrorCode              `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// NewError creates a new standardized error
func NewError(code ErrorCode, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
	}
}

// Error implements the error interface
func (e *Error) Error() string {
	if len(e.Details) > 0 {
		return fmt.Sprintf("[%s] %s (details: %v)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// WithDetails adds additional details to the error
func (e *Error) WithDetails(key string, value interface{}) *Error {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// TimeoutError represents an error when an operation times out
type TimeoutError struct {
	Operation string
	Duration  time.Duration
}

// Error returns the error message for TimeoutError
func (e *TimeoutError) Error() string {
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

// NewRetryExhaustedError creates a new RetryExhaustedError
func NewRetryExhaustedError(operation string, attempts int, lastError error) *RetryExhaustedError {
	return &RetryExhaustedError{
		Operation: operation,
		Attempts:  attempts,
		LastError: lastError,
	}
}

// Campaign-related errors
var (
	ErrCampaignNotFound     = fmt.Errorf("campaign not found")
	ErrCampaignRunning      = fmt.Errorf("campaign is already running")
	ErrInvalidStackTrace    = fmt.Errorf("invalid stack trace format")
	ErrCorpusFileTooLarge   = fmt.Errorf("corpus file exceeds size limit")
	ErrDuplicateCorpusFile  = fmt.Errorf("corpus file already exists")
	ErrCampaignCompleted    = fmt.Errorf("campaign is already completed")
	ErrCampaignPaused       = fmt.Errorf("campaign is paused")
	ErrNoCampaignJobs       = fmt.Errorf("no jobs found for campaign")
	ErrInvalidCampaignState = fmt.Errorf("invalid campaign state transition")
	ErrCrashGroupNotFound   = fmt.Errorf("crash group not found")
	ErrCorpusFileNotFound   = fmt.Errorf("corpus file not found")
	ErrBinaryHashMismatch   = fmt.Errorf("binary hash mismatch between campaigns")
)
