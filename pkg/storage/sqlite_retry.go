package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

// RetryableFunc represents a function that returns an error and can be retried
type RetryableFunc func() error

// RetryableResultFunc represents a function that returns a result and an error and can be retried
type RetryableResultFunc[T any] func() (T, error)

// ExecuteWithRetry executes a function with exponential backoff retry logic
func ExecuteWithRetry(ctx context.Context, config common.DatabaseConfig, fn RetryableFunc) error {
	logger := logrus.WithField("component", "sqlite_retry")

	// Ensure defaults are set
	config.SetDefaults()

	var lastErr error
	delay := config.RetryDelay

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Check context cancellation before each attempt
		select {
		case <-ctx.Done():
			logger.WithError(ctx.Err()).Debug("Context cancelled during retry")
			return ctx.Err()
		default:
		}

		// Execute the function
		err := fn()
		if err == nil {
			// Success
			if attempt > 0 {
				logger.WithField("attempts", attempt+1).Debug("Operation succeeded after retries")
			}
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			logger.WithError(err).Debug("Non-retryable error encountered")
			return err
		}

		// If this was the last attempt, return the error
		if attempt == config.MaxRetries {
			logger.WithError(err).WithField("attempts", attempt+1).Warn("Max retries exhausted")
			return err
		}

		// Log retry attempt
		logger.WithFields(logrus.Fields{
			"error":   err.Error(),
			"attempt": attempt + 1,
			"delay":   delay.String(),
		}).Debug("Retrying after error")

		// Wait with exponential backoff
		select {
		case <-time.After(delay):
			// Calculate next delay with exponential backoff
			delay = time.Duration(float64(delay) * config.RetryMultiplier)
			if delay > config.MaxRetryDelay {
				delay = config.MaxRetryDelay
			}
		case <-ctx.Done():
			logger.WithError(ctx.Err()).Debug("Context cancelled during backoff")
			return ctx.Err()
		}
	}

	return lastErr
}

// ExecuteWithRetryResult executes a function that returns a result with exponential backoff retry logic
func ExecuteWithRetryResult[T any](ctx context.Context, config common.DatabaseConfig, fn RetryableResultFunc[T]) (T, error) {
	logger := logrus.WithField("component", "sqlite_retry")

	// Ensure defaults are set
	config.SetDefaults()

	var lastErr error
	var zeroValue T
	delay := config.RetryDelay

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Check context cancellation before each attempt
		select {
		case <-ctx.Done():
			logger.WithError(ctx.Err()).Debug("Context cancelled during retry")
			return zeroValue, ctx.Err()
		default:
		}

		// Execute the function
		result, err := fn()
		if err == nil {
			// Success
			if attempt > 0 {
				logger.WithField("attempts", attempt+1).Debug("Operation succeeded after retries")
			}
			return result, nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			logger.WithError(err).Debug("Non-retryable error encountered")
			return zeroValue, err
		}

		// If this was the last attempt, return the error
		if attempt == config.MaxRetries {
			logger.WithError(err).WithField("attempts", attempt+1).Warn("Max retries exhausted")
			return zeroValue, err
		}

		// Log retry attempt
		logger.WithFields(logrus.Fields{
			"error":   err.Error(),
			"attempt": attempt + 1,
			"delay":   delay.String(),
		}).Debug("Retrying after error")

		// Wait with exponential backoff
		select {
		case <-time.After(delay):
			// Calculate next delay with exponential backoff
			delay = time.Duration(float64(delay) * config.RetryMultiplier)
			if delay > config.MaxRetryDelay {
				delay = config.MaxRetryDelay
			}
		case <-ctx.Done():
			logger.WithError(ctx.Err()).Debug("Context cancelled during backoff")
			return zeroValue, ctx.Err()
		}
	}

	return zeroValue, lastErr
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for context errors (not retryable)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check for SQLite specific errors
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		// SQLite error codes that indicate retryable conditions
		switch sqliteErr.Code {
		case sqlite3.ErrLocked: // Database is locked
			return true
		case sqlite3.ErrBusy: // Database is busy
			return true
		case sqlite3.ErrCantOpen: // Unable to open database file (might be temporary)
			return true
		case sqlite3.ErrProtocol: // Database locking protocol error
			return true
		case sqlite3.ErrSchema: // Schema changed (can retry after re-preparing statements)
			return true
		case sqlite3.ErrTooBig: // String or BLOB too big (not retryable)
			return false
		case sqlite3.ErrConstraint: // Constraint violation (not retryable)
			return false
		case sqlite3.ErrCorrupt: // Database corruption (not retryable)
			return false
		default:
			// For other errors, check extended result codes
			if sqliteErr.ExtendedCode == sqlite3.ErrBusySnapshot {
				return true
			}
		}
	}

	// Check for standard SQL errors
	if errors.Is(err, sql.ErrTxDone) {
		// Transaction already completed (not retryable)
		return false
	}

	// Check error messages for patterns that indicate retryable conditions
	errMsg := strings.ToLower(err.Error())

	// Common retryable patterns
	retryablePatterns := []string{
		"database is locked",
		"database table is locked",
		"database schema has changed",
		"cannot commit transaction",
		"cannot start a transaction within a transaction",
		"database is busy",
		"unable to open database",
		"disk i/o error",
		"no such table", // Might be temporary during migrations
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	// Non-retryable patterns
	nonRetryablePatterns := []string{
		"constraint failed",
		"foreign key constraint failed",
		"unique constraint failed",
		"not null constraint failed",
		"datatype mismatch",
		"no such column",
		"syntax error",
		"database disk image is malformed",
		"file is not a database",
	}

	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(errMsg, pattern) {
			return false
		}
	}

	// Default to not retrying unknown errors
	return false
}

// RetryableTransaction wraps a transaction function with retry logic
func RetryableTransaction(ctx context.Context, db *sql.DB, config common.DatabaseConfig, fn func(tx *sql.Tx) error) error {
	return ExecuteWithRetry(ctx, config, func() error {
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

// RetryableQuery wraps a query with retry logic and returns multiple rows
func RetryableQuery[T any](ctx context.Context, db *sql.DB, config common.DatabaseConfig, query string, scanFunc func(*sql.Rows) (T, error), args ...any) ([]T, error) {
	return ExecuteWithRetryResult(ctx, config, func() ([]T, error) {
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

// RetryableQueryRow wraps a single row query with retry logic
func RetryableQueryRow[T any](ctx context.Context, db *sql.DB, config common.DatabaseConfig, query string, scanFunc func(*sql.Row) (T, error), args ...any) (T, error) {
	return ExecuteWithRetryResult(ctx, config, func() (T, error) {
		row := db.QueryRowContext(ctx, query, args...)
		return scanFunc(row)
	})
}

// RetryableExec wraps an exec operation with retry logic
func RetryableExec(ctx context.Context, db *sql.DB, config common.DatabaseConfig, query string, args ...any) (sql.Result, error) {
	return ExecuteWithRetryResult(ctx, config, func() (sql.Result, error) {
		return db.ExecContext(ctx, query, args...)
	})
}
