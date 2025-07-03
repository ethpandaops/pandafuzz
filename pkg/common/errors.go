package common

import (
	"fmt"
	"time"
)

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
