package repository

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
)

// JobRepository defines the interface for job persistence
type JobRepository interface {
	// Create persists a new job
	Create(ctx context.Context, job *types.Job) error

	// Get retrieves a job by ID
	Get(ctx context.Context, id string) (*types.Job, error)

	// Update persists changes to an existing job
	Update(ctx context.Context, job *types.Job) error

	// Delete removes a job by ID
	Delete(ctx context.Context, id string) error

	// List retrieves jobs with filtering and pagination
	List(ctx context.Context, filter JobFilter) ([]*types.Job, error)

	// ListByStatus retrieves all jobs with a specific status
	ListByStatus(ctx context.Context, status types.JobStatus) ([]*types.Job, error)

	// ListPending retrieves pending jobs ordered by priority and creation time
	ListPending(ctx context.Context, limit int) ([]*types.Job, error)

	// ListScheduled retrieves jobs scheduled to run at or before the given time
	ListScheduled(ctx context.Context, before time.Time) ([]*types.Job, error)

	// CountByStatus returns the count of jobs for each status
	CountByStatus(ctx context.Context) (map[types.JobStatus]int64, error)

	// UpdateStatus atomically updates a job's status with validation
	UpdateStatus(ctx context.Context, id string, from, to types.JobStatus) error

	// IncrementRetries atomically increments the retry count for a job
	IncrementRetries(ctx context.Context, id string) error

	// GetDependencies retrieves all jobs that depend on the given job
	GetDependencies(ctx context.Context, jobID string) ([]*types.Job, error)

	// GetDependents retrieves all jobs that the given job depends on
	GetDependents(ctx context.Context, jobID string) ([]*types.Job, error)

	// AddDependency creates a dependency relationship between jobs
	AddDependency(ctx context.Context, jobID, dependsOnID string) error

	// RemoveDependency removes a dependency relationship between jobs
	RemoveDependency(ctx context.Context, jobID, dependsOnID string) error

	// LockForProcessing attempts to lock a job for processing by a worker
	// Returns the locked job or nil if unable to lock
	LockForProcessing(ctx context.Context, jobID string, workerID string, lockDuration time.Duration) (*types.Job, error)

	// UnlockJob releases a processing lock on a job
	UnlockJob(ctx context.Context, jobID string, workerID string) error

	// GetStaleJobs retrieves jobs that have been locked for longer than the specified duration
	GetStaleJobs(ctx context.Context, staleDuration time.Duration) ([]*types.Job, error)

	// GetMetrics retrieves repository performance metrics
	GetMetrics(ctx context.Context) (*JobRepositoryMetrics, error)
}

// JobFilter defines filtering options for job queries
type JobFilter struct {
	// Status filters by job status
	Status *types.JobStatus

	// Priority filters by minimum priority level
	MinPriority *types.JobPriority

	// Tags filters by job tags (jobs must have all specified tags)
	Tags []string

	// FuzzerType filters by fuzzer type
	FuzzerType *string

	// CreatedAfter filters jobs created after this time
	CreatedAfter *time.Time

	// CreatedBefore filters jobs created before this time
	CreatedBefore *time.Time

	// Limit specifies maximum number of results
	Limit int

	// Offset specifies number of results to skip
	Offset int

	// OrderBy specifies the field to order by
	OrderBy JobOrderBy

	// OrderDirection specifies ascending or descending order
	OrderDirection OrderDirection
}

// JobOrderBy defines fields that can be used for ordering
type JobOrderBy string

const (
	OrderByCreatedAt JobOrderBy = "created_at"
	OrderByPriority  JobOrderBy = "priority"
	OrderByStatus    JobOrderBy = "status"
	OrderByScheduled JobOrderBy = "scheduled_at"
)

// OrderDirection defines sort order
type OrderDirection string

const (
	OrderAsc  OrderDirection = "asc"
	OrderDesc OrderDirection = "desc"
)

// JobRepositoryMetrics contains performance metrics for the repository
type JobRepositoryMetrics struct {
	TotalJobs        int64
	JobsByStatus     map[types.JobStatus]int64
	AverageQueryTime time.Duration
	LockContention   float64
}

// JobDependency represents a dependency between two jobs
type JobDependency struct {
	JobID       string
	DependsOnID string
	CreatedAt   time.Time
}

// ProcessingLock represents a lock held by a worker on a job
type ProcessingLock struct {
	JobID     string
	WorkerID  string
	LockedAt  time.Time
	ExpiresAt time.Time
}
