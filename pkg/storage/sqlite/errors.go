package sqlite

import (
	"errors"
	"fmt"
)

// Repository errors
var (
	// ErrNotFound indicates a resource was not found
	ErrNotFound = errors.New("resource not found")

	// ErrJobAlreadyLocked indicates the job is already locked by another worker
	ErrJobAlreadyLocked = errors.New("job is already locked by another worker")

	// ErrInvalidTransition indicates an invalid status transition was attempted
	ErrInvalidTransition = errors.New("invalid status transition")

	// ErrDatabaseClosed indicates the database connection has been closed
	ErrDatabaseClosed = errors.New("database connection is closed")

	// ErrConcurrentModification indicates a concurrent modification conflict
	ErrConcurrentModification = errors.New("concurrent modification detected")

	// ErrDependencyCycle indicates a circular dependency was detected
	ErrDependencyCycle = errors.New("circular dependency detected")

	// ErrJobNotLocked indicates an unlock operation was attempted on an unlocked job
	ErrJobNotLocked = errors.New("job is not locked")

	// ErrLockExpired indicates the lock has expired
	ErrLockExpired = errors.New("lock has expired")

	// ErrInvalidLockHolder indicates the worker doesn't hold the lock
	ErrInvalidLockHolder = errors.New("worker does not hold the lock")
)

// RepositoryError wraps database errors with context
type RepositoryError struct {
	Op     string // Operation name
	Entity string // Entity type (job, bot, etc.)
	ID     string // Entity ID if available
	Err    error  // Underlying error
}

// Error implements the error interface
func (e *RepositoryError) Error() string {
	if e.ID != "" {
		return fmt.Sprintf("%s %s (id=%s): %v", e.Op, e.Entity, e.ID, e.Err)
	}
	return fmt.Sprintf("%s %s: %v", e.Op, e.Entity, e.Err)
}

// Unwrap returns the underlying error
func (e *RepositoryError) Unwrap() error {
	return e.Err
}

// NewRepositoryError creates a new RepositoryError
func NewRepositoryError(op, entity, id string, err error) *RepositoryError {
	return &RepositoryError{
		Op:     op,
		Entity: entity,
		ID:     id,
		Err:    err,
	}
}

// IsNotFoundError checks if the error indicates a resource not found
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsAlreadyLockedError checks if the error indicates a locking conflict
func IsAlreadyLockedError(err error) bool {
	return errors.Is(err, ErrJobAlreadyLocked)
}

// IsInvalidTransitionError checks if the error indicates an invalid transition
func IsInvalidTransitionError(err error) bool {
	return errors.Is(err, ErrInvalidTransition)
}
