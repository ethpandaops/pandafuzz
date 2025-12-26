package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

// RetryOptions configures retry behavior
type RetryOptions struct {
	MaxRetries      int
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	Multiplier      float64
	Jitter          bool
	RetryableErrors []error
	OnRetry         func(attempt int, err error)
}

// DefaultRetryOptions returns sensible defaults for database operations
func DefaultRetryOptions() RetryOptions {
	return RetryOptions{
		MaxRetries:   3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
		RetryableErrors: []error{
			sql.ErrConnDone,
			sql.ErrTxDone,
		},
	}
}

// WithRetry executes a function with retry logic
func WithRetry[T any](ctx context.Context, opts RetryOptions, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	var lastErr error

	for attempt := 0; attempt <= opts.MaxRetries; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		default:
		}

		// Execute the function
		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if we should retry
		if attempt == opts.MaxRetries || !isRetryable(err, opts.RetryableErrors) {
			break
		}

		// Call the retry callback if provided
		if opts.OnRetry != nil {
			opts.OnRetry(attempt+1, err)
		}

		// Calculate delay
		delay := calculateDelay(attempt, opts)

		// Wait before retrying
		select {
		case <-time.After(delay):
			// Continue to next attempt
		case <-ctx.Done():
			return zero, ctx.Err()
		}
	}

	// Wrap the error with retry context
	return zero, fmt.Errorf("operation failed after %d attempts: %w", opts.MaxRetries+1, lastErr)
}

// WithRetryVoid is a convenience wrapper for functions that don't return a value
func WithRetryVoid(ctx context.Context, opts RetryOptions, fn func(context.Context) error) error {
	_, err := WithRetry(ctx, opts, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn(ctx)
	})
	return err
}

// isRetryable checks if an error is retryable
func isRetryable(err error, retryableErrors []error) bool {
	if err == nil {
		return false
	}

	// Check SQLite specific errors
	if isSQLiteRetryable(err) {
		return true
	}

	// Check configured retryable errors
	for _, retryableErr := range retryableErrors {
		if errors.Is(err, retryableErr) {
			return true
		}
	}

	// Check if it's a database error with retryable operation
	var dbErr *common.PandaFuzzError
	if errors.As(err, &dbErr) && dbErr.Type == common.ErrorTypeDatabase {
		// Check for specific database error patterns
		errMsg := err.Error()
		return isRetryableDatabaseError(errMsg)
	}

	return false
}

// isSQLiteRetryable checks for SQLite-specific retryable errors
func isSQLiteRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for SQLite specific errors using the driver's error type
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		// SQLite error codes that indicate retryable conditions
		switch sqliteErr.Code {
		case sqlite3.ErrLocked: // Database is locked
			return true
		case sqlite3.ErrBusy: // Database is busy
			return true
		case sqlite3.ErrConstraint: // Constraint violation (might be temporary)
			// Only retry UNIQUE constraint violations as they might be due to race conditions
			return sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique
		}
	}

	// Fall back to string matching for errors that don't use the SQLite error type
	errMsg := err.Error()

	// SQLite error codes that indicate retryable conditions
	retryablePatterns := []string{
		"database is locked",
		"database table is locked",
		"SQLITE_BUSY",
		"SQLITE_LOCKED",
		"cannot commit - no transaction is active",
		"cannot commit transaction - SQL statements in progress",
		"cannot start a transaction within a transaction",
	}

	for _, pattern := range retryablePatterns {
		if contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// isRetryableDatabaseError checks if a database error message indicates a retryable condition
func isRetryableDatabaseError(errMsg string) bool {
	retryablePatterns := []string{
		"connection reset by peer",
		"broken pipe",
		"no such host",
		"connection refused",
		"i/o timeout",
		"context deadline exceeded",
		"too many connections",
	}

	for _, pattern := range retryablePatterns {
		if contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// calculateDelay calculates the retry delay with exponential backoff and optional jitter
func calculateDelay(attempt int, opts RetryOptions) time.Duration {
	// Calculate base delay with exponential backoff
	delay := float64(opts.InitialDelay) * math.Pow(opts.Multiplier, float64(attempt))

	// Apply max delay cap
	if delay > float64(opts.MaxDelay) {
		delay = float64(opts.MaxDelay)
	}

	// Apply jitter if enabled
	if opts.Jitter {
		// Add random jitter between -25% and +25% of the delay
		jitter := (rand.Float64() - 0.5) * 0.5
		delay = delay * (1 + jitter)
	}

	return time.Duration(delay)
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsIgnoreCase(s, substr)
}

// containsIgnoreCase performs case-insensitive substring search
func containsIgnoreCase(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}

	// Simple case-insensitive search
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if toLower(s[i+j]) != toLower(substr[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// toLower converts a single byte to lowercase
func toLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// RetryableOperation wraps an operation with retry logic and logging
type RetryableOperation struct {
	Name    string
	Logger  logrus.FieldLogger
	Options RetryOptions
}

// Execute runs the operation with configured retry logic
func (r *RetryableOperation) Execute(ctx context.Context, fn func(context.Context) error) error {
	opts := r.Options
	if opts.OnRetry == nil {
		opts.OnRetry = func(attempt int, err error) {
			if r.Logger != nil {
				r.Logger.WithFields(logrus.Fields{
					"operation": r.Name,
					"attempt":   attempt,
					"error":     err.Error(),
				}).Warn("Retrying operation")
			}
		}
	}

	return WithRetryVoid(ctx, opts, fn)
}

// ExecuteWithResult cannot be a method with generics, so we'll provide a standalone function
// Use ExecuteRetryableWithResult instead for generic operations

// ExecuteRetryableWithResult runs the operation with configured retry logic and returns a result
func ExecuteRetryableWithResult[T any](r *RetryableOperation, ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	opts := r.Options
	if opts.OnRetry == nil {
		opts.OnRetry = func(attempt int, err error) {
			if r.Logger != nil {
				r.Logger.WithFields(logrus.Fields{
					"operation": r.Name,
					"attempt":   attempt,
					"error":     err.Error(),
				}).Warn("Retrying operation")
			}
		}
	}

	return WithRetry(ctx, opts, fn)
}

// Legacy compatibility functions for gradual migration

// ExecuteWithRetry provides backward compatibility for existing code
func ExecuteWithRetry(ctx context.Context, config common.DatabaseConfig, fn func() error) error {
	opts := RetryOptions{
		MaxRetries:      config.MaxRetries,
		InitialDelay:    config.RetryDelay,
		MaxDelay:        config.MaxRetryDelay,
		Multiplier:      config.RetryMultiplier,
		Jitter:          true,
		RetryableErrors: []error{sql.ErrConnDone, sql.ErrTxDone},
	}

	return WithRetryVoid(ctx, opts, func(ctx context.Context) error {
		return fn()
	})
}

// ExecuteWithRetryResult provides backward compatibility for existing code
func ExecuteWithRetryResult[T any](ctx context.Context, config common.DatabaseConfig, fn func() (T, error)) (T, error) {
	opts := RetryOptions{
		MaxRetries:      config.MaxRetries,
		InitialDelay:    config.RetryDelay,
		MaxDelay:        config.MaxRetryDelay,
		Multiplier:      config.RetryMultiplier,
		Jitter:          true,
		RetryableErrors: []error{sql.ErrConnDone, sql.ErrTxDone},
	}

	return WithRetry(ctx, opts, func(ctx context.Context) (T, error) {
		return fn()
	})
}

// RetryableExec executes a SQL statement with retry logic
func RetryableExec(ctx context.Context, db *sql.DB, config common.DatabaseConfig, query string, args ...any) (sql.Result, error) {
	opts := RetryOptions{
		MaxRetries:      config.MaxRetries,
		InitialDelay:    config.RetryDelay,
		MaxDelay:        config.MaxRetryDelay,
		Multiplier:      config.RetryMultiplier,
		Jitter:          true,
		RetryableErrors: []error{sql.ErrConnDone, sql.ErrTxDone},
	}

	return WithRetry(ctx, opts, func(ctx context.Context) (sql.Result, error) {
		return db.ExecContext(ctx, query, args...)
	})
}

// RetryableQuery executes a query with retry logic and returns multiple rows
func RetryableQuery[T any](ctx context.Context, db *sql.DB, config common.DatabaseConfig, query string, scanFunc func(*sql.Rows) (T, error), args ...any) ([]T, error) {
	opts := RetryOptions{
		MaxRetries:      config.MaxRetries,
		InitialDelay:    config.RetryDelay,
		MaxDelay:        config.MaxRetryDelay,
		Multiplier:      config.RetryMultiplier,
		Jitter:          true,
		RetryableErrors: []error{sql.ErrConnDone, sql.ErrTxDone},
	}

	return WithRetry(ctx, opts, func(ctx context.Context) ([]T, error) {
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var results []T
		for rows.Next() {
			item, err := scanFunc(rows)
			if err != nil {
				return nil, err
			}
			results = append(results, item)
		}

		return results, rows.Err()
	})
}

// RetryableQueryRow executes a single row query with retry logic
func RetryableQueryRow[T any](ctx context.Context, db *sql.DB, config common.DatabaseConfig, query string, scanFunc func(*sql.Row) (T, error), args ...any) (T, error) {
	opts := RetryOptions{
		MaxRetries:      config.MaxRetries,
		InitialDelay:    config.RetryDelay,
		MaxDelay:        config.MaxRetryDelay,
		Multiplier:      config.RetryMultiplier,
		Jitter:          true,
		RetryableErrors: []error{sql.ErrConnDone, sql.ErrTxDone},
	}

	return WithRetry(ctx, opts, func(ctx context.Context) (T, error) {
		row := db.QueryRowContext(ctx, query, args...)
		return scanFunc(row)
	})
}

// RetryableTransaction wraps a transaction function with retry logic
func RetryableTransaction(ctx context.Context, db *sql.DB, config common.DatabaseConfig, fn func(tx *sql.Tx) error) error {
	opts := RetryOptions{
		MaxRetries:      config.MaxRetries,
		InitialDelay:    config.RetryDelay,
		MaxDelay:        config.MaxRetryDelay,
		Multiplier:      config.RetryMultiplier,
		Jitter:          true,
		RetryableErrors: []error{sql.ErrConnDone, sql.ErrTxDone},
	}

	return WithRetryVoid(ctx, opts, func(ctx context.Context) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}

		// Ensure rollback on panic
		defer func() {
			if p := recover(); p != nil {
				tx.Rollback()
				panic(p)
			}
		}()

		// Execute transaction function
		if err := fn(tx); err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				logrus.WithError(rollbackErr).Error("Failed to rollback transaction")
			}
			return err
		}

		// Commit transaction
		return tx.Commit()
	})
}
