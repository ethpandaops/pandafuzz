// Package errors provides error types and handling utilities for PandaFuzz.
//
// This package centralizes error definitions across the system, providing
// typed errors, error codes, and helper functions for error classification.
// It follows Go's error wrapping conventions for proper error chain handling.
//
// # Error Types
//
// The package defines several error types for different domains:
//
//   - SystemError: Internal system failures
//   - ValidationError: Invalid input or state
//   - NotFoundError: Missing resources
//   - StorageError: Storage backend failures
//   - NetworkError: Network communication issues
//   - RetryExhaustedError: Retry attempts exceeded
//   - TimeoutErr: Operation timeout
//
// # Creating Errors
//
// Use the constructor functions to create domain-specific errors:
//
//	// Validation error
//	err := errors.NewValidationError("create_job", "target path is required")
//
//	// Not found error
//	err := errors.NewNotFoundError("get_bot", "bot")
//
//	// Storage error
//	err := errors.NewStorageError("save_crash", underlyingErr)
//
// # Error Codes
//
// The ErrorCode type provides numeric codes for programmatic error handling:
//
//	const (
//	    ErrorCodeUnknown ErrorCode = iota
//	    ErrorCodeNotFound
//	    ErrorCodeValidation
//	    ErrorCodeDatabase
//	    ErrorCodeTimeout
//	    // ...
//	)
//
// # Error Classification
//
// Use classification functions to determine error handling strategy:
//
//	if errors.IsRetryable(err) {
//	    // Safe to retry the operation
//	}
//
//	if errors.IsSystemError(err) {
//	    // Log at ERROR level, may need operator attention
//	}
//
//	if errors.IsDatabaseError(err) {
//	    // Database-specific recovery may be needed
//	}
//
// # Sentinel Errors
//
// Common errors are defined as sentinel values for comparison:
//
//	var (
//	    ErrNotFound       = errors.New("not found")
//	    ErrAlreadyExists  = errors.New("already exists")
//	    ErrInvalidInput   = errors.New("invalid input")
//	    ErrOperationFailed = errors.New("operation failed")
//	)
//
// Use errors.Is() for sentinel comparison:
//
//	if errors.Is(err, errors.ErrNotFound) {
//	    return nil, status.NotFound("resource not found")
//	}
//
// # Error Wrapping
//
// Always wrap errors with context when propagating:
//
//	if err := db.Get(ctx, key, &result); err != nil {
//	    return fmt.Errorf("get job %s: %w", jobID, err)
//	}
package errors
