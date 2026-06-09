package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	jobrepo "github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/mappers"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/models"
	"github.com/sirupsen/logrus"
)

// Compile-time interface compliance check
var _ jobrepo.JobRepository = (*JobRepository)(nil)

// JobRepository implements jobrepo.JobRepository using SQLite
type JobRepository struct {
	db     *sql.DB
	logger logrus.FieldLogger
}

// NewJobRepository creates a new SQLite-based job repository
func NewJobRepository(db *sql.DB, logger logrus.FieldLogger) *JobRepository {
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	return &JobRepository{
		db:     db,
		logger: logger.WithField("component", "job_repository"),
	}
}

// Create persists a new job
func (r *JobRepository) Create(ctx context.Context, job *jobtypes.Job) error {
	if job == nil {
		return NewRepositoryError("create", "job", "", fmt.Errorf("job cannot be nil"))
	}

	row := mappers.DomainJobToRow(job)

	query := `
		INSERT INTO jobs (
			id, name, target, fuzzer, status, created_at, timeout_at,
			work_dir, config, progress, priority, enable_coverage,
			coverage_format, coverage_report_id, lease_token,
			lease_expires_at, last_heartbeat, updated_at,
			description, scheduled_at, queued_at, dequeue_count,
			retry_count, max_retries, retry_delay,
			locked_by, locked_at, lock_expires_at,
			error_message, execution_time, corpus_path, output_path
		) VALUES (
			?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?
		)
	`

	_, err := r.db.ExecContext(ctx, query,
		row.ID, row.Name, row.Target, row.Fuzzer, row.Status, row.CreatedAt, row.TimeoutAt,
		row.WorkDir, row.ConfigJSON, row.Progress, row.Priority, row.EnableCoverage,
		row.CoverageFormat, row.CoverageReportID, row.LeaseToken,
		row.LeaseExpiresAt, row.LastHeartbeat, row.UpdatedAt,
		row.Description, row.ScheduledAt, row.QueuedAt, row.DequeueCount,
		row.RetryCount, row.MaxRetries, row.RetryDelayNanos,
		row.LockedBy, row.LockedAt, row.LockExpiresAt,
		row.ErrorMessage, row.ExecutionTimeNanos, row.CorpusPath, row.OutputPath,
	)

	if err != nil {
		return NewRepositoryError("create", "job", job.ID, err)
	}

	r.logger.WithField("job_id", job.ID).Debug("Job created")
	return nil
}

// Get retrieves a job by ID
func (r *JobRepository) Get(ctx context.Context, id string) (*jobtypes.Job, error) {
	row, err := r.scanJobRow(ctx, "SELECT "+jobSelectColumns()+" FROM jobs WHERE id = ?", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, NewRepositoryError("get", "job", id, err)
	}

	return mappers.JobRowToDomain(row), nil
}

// Update persists changes to an existing job
func (r *JobRepository) Update(ctx context.Context, job *jobtypes.Job) error {
	if job == nil {
		return NewRepositoryError("update", "job", "", fmt.Errorf("job cannot be nil"))
	}

	row := mappers.DomainJobToRow(job)

	query := `
		UPDATE jobs SET
			name = ?, target = ?, fuzzer = ?, status = ?,
			started_at = ?, completed_at = ?, timeout_at = ?,
			work_dir = ?, config = ?, progress = ?, priority = ?,
			enable_coverage = ?, coverage_format = ?, coverage_report_id = ?,
			lease_token = ?, lease_expires_at = ?, last_heartbeat = ?,
			updated_at = ?, description = ?, scheduled_at = ?, queued_at = ?,
			dequeue_count = ?, retry_count = ?, max_retries = ?, retry_delay = ?,
			locked_by = ?, locked_at = ?, lock_expires_at = ?,
			error_message = ?, execution_time = ?, corpus_path = ?, output_path = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query,
		row.Name, row.Target, row.Fuzzer, row.Status,
		row.StartedAt, row.CompletedAt, row.TimeoutAt,
		row.WorkDir, row.ConfigJSON, row.Progress, row.Priority,
		row.EnableCoverage, row.CoverageFormat, row.CoverageReportID,
		row.LeaseToken, row.LeaseExpiresAt, row.LastHeartbeat,
		sql.NullTime{Time: time.Now().UTC(), Valid: true}, row.Description, row.ScheduledAt, row.QueuedAt,
		row.DequeueCount, row.RetryCount, row.MaxRetries, row.RetryDelayNanos,
		row.LockedBy, row.LockedAt, row.LockExpiresAt,
		row.ErrorMessage, row.ExecutionTimeNanos, row.CorpusPath, row.OutputPath,
		job.ID,
	)

	if err != nil {
		return NewRepositoryError("update", "job", job.ID, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("update", "job", job.ID, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	r.logger.WithField("job_id", job.ID).Debug("Job updated")
	return nil
}

// Delete removes a job by ID
func (r *JobRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM jobs WHERE id = ?", id)
	if err != nil {
		return NewRepositoryError("delete", "job", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("delete", "job", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	r.logger.WithField("job_id", id).Debug("Job deleted")
	return nil
}

// List retrieves jobs with filtering and pagination
func (r *JobRepository) List(ctx context.Context, filter jobrepo.JobFilter) ([]*jobtypes.Job, error) {
	query := "SELECT " + jobSelectColumns() + " FROM jobs WHERE 1=1"
	args := make([]interface{}, 0, 8)

	// Apply filters
	if filter.Status != nil {
		query += " AND status = ?"
		args = append(args, filter.Status.String())
	}
	if filter.MinPriority != nil {
		query += " AND priority >= ?"
		args = append(args, mappers.DomainPriorityToCommon(*filter.MinPriority))
	}
	if filter.FuzzerType != nil {
		query += " AND fuzzer = ?"
		args = append(args, *filter.FuzzerType)
	}
	if filter.CreatedAfter != nil {
		query += " AND created_at > ?"
		args = append(args, *filter.CreatedAfter)
	}
	if filter.CreatedBefore != nil {
		query += " AND created_at < ?"
		args = append(args, *filter.CreatedBefore)
	}
	if len(filter.Tags) > 0 {
		// Tags are stored in metadata JSON - need to check each tag
		for _, tag := range filter.Tags {
			query += " AND metadata LIKE ?"
			args = append(args, "%"+tag+"%")
		}
	}

	// Apply ordering
	orderBy := "created_at"
	if filter.OrderBy != "" {
		switch filter.OrderBy {
		case jobrepo.OrderByCreatedAt:
			orderBy = "created_at"
		case jobrepo.OrderByPriority:
			orderBy = "priority"
		case jobrepo.OrderByStatus:
			orderBy = "status"
		case jobrepo.OrderByScheduled:
			orderBy = "scheduled_at"
		}
	}

	orderDir := "DESC"
	if filter.OrderDirection == jobrepo.OrderAsc {
		orderDir = "ASC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s", orderBy, orderDir)

	// Apply pagination
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	return r.scanJobRows(ctx, query, args...)
}

// ListByStatus retrieves all jobs with a specific status
func (r *JobRepository) ListByStatus(ctx context.Context, status jobtypes.JobStatus) ([]*jobtypes.Job, error) {
	query := "SELECT " + jobSelectColumns() + " FROM jobs WHERE status = ? ORDER BY created_at DESC"
	return r.scanJobRows(ctx, query, status.String())
}

// ListPending retrieves pending jobs ordered by priority and creation time
func (r *JobRepository) ListPending(ctx context.Context, limit int) ([]*jobtypes.Job, error) {
	query := `
		SELECT ` + jobSelectColumns() + `
		FROM jobs
		WHERE status IN ('pending', 'queued')
		  AND (scheduled_at IS NULL OR scheduled_at <= ?)
		ORDER BY priority DESC, created_at ASC
		LIMIT ?
	`
	return r.scanJobRows(ctx, query, time.Now().UTC(), limit)
}

// ListScheduled retrieves jobs scheduled to run at or before the given time
func (r *JobRepository) ListScheduled(ctx context.Context, before time.Time) ([]*jobtypes.Job, error) {
	query := `
		SELECT ` + jobSelectColumns() + `
		FROM jobs
		WHERE status = 'pending'
		  AND scheduled_at IS NOT NULL
		  AND scheduled_at <= ?
		ORDER BY scheduled_at ASC
	`
	return r.scanJobRows(ctx, query, before)
}

// CountByStatus returns the count of jobs for each status
func (r *JobRepository) CountByStatus(ctx context.Context) (map[jobtypes.JobStatus]int64, error) {
	query := "SELECT status, COUNT(*) as count FROM jobs GROUP BY status"
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, NewRepositoryError("count_by_status", "job", "", err)
	}
	defer rows.Close()

	result := make(map[jobtypes.JobStatus]int64, 8)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, NewRepositoryError("count_by_status", "job", "", err)
		}
		result[mappers.StatusStringToDomain(status)] = count
	}

	if err := rows.Err(); err != nil {
		return nil, NewRepositoryError("count_by_status", "job", "", err)
	}

	return result, nil
}

// UpdateStatus atomically updates a job's status with validation
func (r *JobRepository) UpdateStatus(ctx context.Context, id string, from, to jobtypes.JobStatus) error {
	// Validate transition
	if !from.CanTransitionTo(to) {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidTransition, from, to)
	}

	query := `
		UPDATE jobs
		SET status = ?, updated_at = ?
		WHERE id = ? AND status = ?
	`
	result, err := r.db.ExecContext(ctx, query, to.String(), time.Now().UTC(), id, from.String())
	if err != nil {
		return NewRepositoryError("update_status", "job", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("update_status", "job", id, err)
	}
	if rows == 0 {
		// Either job doesn't exist or status doesn't match (concurrent modification)
		existing, getErr := r.Get(ctx, id)
		if getErr != nil {
			return ErrNotFound
		}
		return fmt.Errorf("%w: job status is %s, expected %s", ErrConcurrentModification, existing.Status, from)
	}

	r.logger.WithFields(logrus.Fields{
		"job_id": id,
		"from":   from,
		"to":     to,
	}).Debug("Job status updated")
	return nil
}

// IncrementRetries atomically increments the retry count for a job
func (r *JobRepository) IncrementRetries(ctx context.Context, id string) error {
	query := `
		UPDATE jobs
		SET retry_count = retry_count + 1, updated_at = ?
		WHERE id = ?
	`
	result, err := r.db.ExecContext(ctx, query, time.Now().UTC(), id)
	if err != nil {
		return NewRepositoryError("increment_retries", "job", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("increment_retries", "job", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// GetDependencies retrieves all jobs that depend on the given job
func (r *JobRepository) GetDependencies(ctx context.Context, jobID string) ([]*jobtypes.Job, error) {
	query := `
		SELECT ` + jobSelectColumnsWithPrefix("j.") + `
		FROM jobs j
		INNER JOIN job_dependencies d ON j.id = d.job_id
		WHERE d.depends_on_job_id = ?
	`
	return r.scanJobRows(ctx, query, jobID)
}

// GetDependents retrieves all jobs that the given job depends on
func (r *JobRepository) GetDependents(ctx context.Context, jobID string) ([]*jobtypes.Job, error) {
	query := `
		SELECT ` + jobSelectColumnsWithPrefix("j.") + `
		FROM jobs j
		INNER JOIN job_dependencies d ON j.id = d.depends_on_job_id
		WHERE d.job_id = ?
	`
	return r.scanJobRows(ctx, query, jobID)
}

// AddDependency creates a dependency relationship between jobs
func (r *JobRepository) AddDependency(ctx context.Context, jobID, dependsOnID string) error {
	// Check for self-dependency
	if jobID == dependsOnID {
		return fmt.Errorf("%w: job cannot depend on itself", ErrDependencyCycle)
	}

	// Check for circular dependency
	// This is a simple check - a full check would require graph traversal
	deps, err := r.GetDependents(ctx, dependsOnID)
	if err != nil && err != ErrNotFound {
		return NewRepositoryError("add_dependency", "job", jobID, err)
	}
	for _, dep := range deps {
		if dep.ID == jobID {
			return fmt.Errorf("%w: adding this dependency would create a cycle", ErrDependencyCycle)
		}
	}

	query := `
		INSERT INTO job_dependencies (job_id, depends_on_job_id, created_at)
		VALUES (?, ?, ?)
	`
	_, err = r.db.ExecContext(ctx, query, jobID, dependsOnID, time.Now().UTC())
	if err != nil {
		return NewRepositoryError("add_dependency", "job", jobID, err)
	}

	return nil
}

// RemoveDependency removes a dependency relationship between jobs
func (r *JobRepository) RemoveDependency(ctx context.Context, jobID, dependsOnID string) error {
	query := "DELETE FROM job_dependencies WHERE job_id = ? AND depends_on_job_id = ?"
	_, err := r.db.ExecContext(ctx, query, jobID, dependsOnID)
	if err != nil {
		return NewRepositoryError("remove_dependency", "job", jobID, err)
	}
	return nil
}

// LockForProcessing attempts to lock a job for processing by a worker.
// Uses atomic compare-and-set to prevent race conditions.
func (r *JobRepository) LockForProcessing(ctx context.Context, jobID string, workerID string, lockDuration time.Duration) (*jobtypes.Job, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(lockDuration)

	// Atomic lock acquisition:
	// Only succeed if job is not locked OR lock has expired
	query := `
		UPDATE jobs
		SET locked_by = ?, locked_at = ?, lock_expires_at = ?,
		    status = 'running', updated_at = ?
		WHERE id = ?
		  AND status IN ('pending', 'queued')
		  AND (locked_by IS NULL OR locked_by = '' OR lock_expires_at < ?)
	`

	result, err := r.db.ExecContext(ctx, query,
		workerID, now, expiresAt, now,
		jobID, now,
	)
	if err != nil {
		return nil, NewRepositoryError("lock_for_processing", "job", jobID, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return nil, NewRepositoryError("lock_for_processing", "job", jobID, err)
	}

	if rows == 0 {
		// Job either doesn't exist, is in wrong status, or is already locked
		existing, getErr := r.Get(ctx, jobID)
		if getErr != nil {
			if getErr == ErrNotFound {
				return nil, ErrNotFound
			}
			return nil, NewRepositoryError("lock_for_processing", "job", jobID, getErr)
		}

		// Check why we couldn't lock
		if existing.LockedBy != "" && existing.LockExpiresAt != nil && existing.LockExpiresAt.After(now) {
			return nil, ErrJobAlreadyLocked
		}
		if existing.Status != jobtypes.StatusPending && existing.Status != jobtypes.StatusQueued {
			return nil, fmt.Errorf("%w: job is in %s status", ErrInvalidTransition, existing.Status)
		}

		return nil, ErrJobAlreadyLocked
	}

	// Return the locked job
	job, err := r.Get(ctx, jobID)
	if err != nil {
		return nil, NewRepositoryError("lock_for_processing", "job", jobID, err)
	}

	r.logger.WithFields(logrus.Fields{
		"job_id":     jobID,
		"worker_id":  workerID,
		"expires_at": expiresAt,
	}).Debug("Job locked for processing")

	return job, nil
}

// UnlockJob releases a processing lock on a job
func (r *JobRepository) UnlockJob(ctx context.Context, jobID string, workerID string) error {
	query := `
		UPDATE jobs
		SET locked_by = NULL, locked_at = NULL, lock_expires_at = NULL, updated_at = ?
		WHERE id = ? AND locked_by = ?
	`

	result, err := r.db.ExecContext(ctx, query, time.Now().UTC(), jobID, workerID)
	if err != nil {
		return NewRepositoryError("unlock_job", "job", jobID, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("unlock_job", "job", jobID, err)
	}

	if rows == 0 {
		// Either job doesn't exist or worker doesn't hold the lock
		existing, getErr := r.Get(ctx, jobID)
		if getErr != nil {
			return ErrNotFound
		}
		if existing.LockedBy == "" {
			return ErrJobNotLocked
		}
		return ErrInvalidLockHolder
	}

	r.logger.WithFields(logrus.Fields{
		"job_id":    jobID,
		"worker_id": workerID,
	}).Debug("Job unlocked")

	return nil
}

// GetStaleJobs retrieves jobs that have been locked for longer than the specified duration
func (r *JobRepository) GetStaleJobs(ctx context.Context, staleDuration time.Duration) ([]*jobtypes.Job, error) {
	staleTime := time.Now().UTC().Add(-staleDuration)
	query := `
		SELECT ` + jobSelectColumns() + `
		FROM jobs
		WHERE locked_by IS NOT NULL
		  AND locked_by != ''
		  AND locked_at IS NOT NULL
		  AND locked_at < ?
	`
	return r.scanJobRows(ctx, query, staleTime)
}

// GetMetrics retrieves repository performance metrics
func (r *JobRepository) GetMetrics(ctx context.Context) (*jobrepo.JobRepositoryMetrics, error) {
	metrics := &jobrepo.JobRepositoryMetrics{
		JobsByStatus: make(map[jobtypes.JobStatus]int64, 8),
	}

	// Get total jobs
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs").Scan(&metrics.TotalJobs)
	if err != nil {
		return nil, NewRepositoryError("get_metrics", "job", "", err)
	}

	// Get jobs by status
	statusCounts, err := r.CountByStatus(ctx)
	if err != nil {
		return nil, err
	}
	metrics.JobsByStatus = statusCounts

	// Calculate lock contention (jobs currently locked / total running)
	var lockedCount int64
	err = r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM jobs
		WHERE locked_by IS NOT NULL AND locked_by != '' AND lock_expires_at > ?
	`, time.Now().UTC()).Scan(&lockedCount)
	if err != nil {
		return nil, NewRepositoryError("get_metrics", "job", "", err)
	}

	runningCount := statusCounts[jobtypes.StatusRunning]
	if runningCount > 0 {
		metrics.LockContention = float64(lockedCount) / float64(runningCount)
	}

	return metrics, nil
}

// Helper functions

// jobSelectColumns returns the column list for SELECT queries.
// Order must match the scanJobRow scanner.
func jobSelectColumns() string {
	return jobSelectColumnsWithPrefix("")
}

// jobSelectColumnsWithPrefix returns job column names with an optional table prefix
func jobSelectColumnsWithPrefix(prefix string) string {
	columns := []string{
		"id", "name", "target", "fuzzer", "type", "status", "assigned_bot",
		"created_at", "started_at", "completed_at", "timeout_at",
		"work_dir", "config", "progress", "campaign_id", "collection_id",
		"use_campaign_corpus", "enable_coverage", "coverage_format", "coverage_report_id",
		"lease_token", "lease_expires_at", "last_heartbeat", "updated_at",
	}

	// Special columns with COALESCE need different handling
	coalesceColumns := []string{
		"COALESCE(%sdescription, '') as description",
		"%sscheduled_at",
		"%squeued_at",
		"COALESCE(%sdequeue_count, 0) as dequeue_count",
		"COALESCE(%sretry_count, 0) as retry_count",
		"COALESCE(%smax_retries, 3) as max_retries",
		"COALESCE(%sretry_delay, 0) as retry_delay",
		"%slocked_by",
		"%slocked_at",
		"%slock_expires_at",
		"COALESCE(%serror_message, '') as error_message",
		"COALESCE(%sexecution_time, 0) as execution_time",
		"%scorpus_path",
		"%soutput_path",
	}

	var result []string
	for _, col := range columns {
		result = append(result, prefix+col)
	}
	for _, col := range coalesceColumns {
		result = append(result, fmt.Sprintf(col, prefix))
	}
	return strings.Join(result, ", ")
}

// scanJobRow scans a single job row from a query
func (r *JobRepository) scanJobRow(ctx context.Context, query string, args ...interface{}) (*models.JobRow, error) {
	row := r.db.QueryRowContext(ctx, query, args...)
	return r.scanSingleRow(row)
}

// scanJobRows scans multiple job rows from a query
func (r *JobRepository) scanJobRows(ctx context.Context, query string, args ...interface{}) ([]*jobtypes.Job, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]*jobtypes.Job, 0, 16)
	for rows.Next() {
		jobRow, err := r.scanRowsRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, mappers.JobRowToDomain(jobRow))
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return jobs, nil
}

// scanSingleRow scans a single *sql.Row into a JobRow
func (r *JobRepository) scanSingleRow(row *sql.Row) (*models.JobRow, error) {
	jr := &models.JobRow{}
	var metadata sql.NullString

	err := row.Scan(
		&jr.ID, &jr.Name, &jr.Target, &jr.Fuzzer, &jr.Type, &jr.Status, &jr.AssignedBot,
		&jr.CreatedAt, &jr.StartedAt, &jr.CompletedAt, &jr.TimeoutAt,
		&jr.WorkDir, &jr.ConfigJSON, &jr.Progress, &jr.CampaignID, &jr.CollectionID,
		&jr.UseCampaignCorpus, &jr.EnableCoverage, &jr.CoverageFormat, &jr.CoverageReportID,
		&jr.LeaseToken, &jr.LeaseExpiresAt, &jr.LastHeartbeat, &jr.UpdatedAt,
		&jr.Description,
		&jr.ScheduledAt, &jr.QueuedAt, &jr.DequeueCount,
		&jr.RetryCount, &jr.MaxRetries, &jr.RetryDelayNanos,
		&jr.LockedBy, &jr.LockedAt, &jr.LockExpiresAt,
		&jr.ErrorMessage, &jr.ExecutionTimeNanos,
		&jr.CorpusPath, &jr.OutputPath,
	)
	if err != nil {
		return nil, err
	}

	jr.MetadataJSON = metadata
	return jr, nil
}

// scanRowsRow scans a single row from *sql.Rows into a JobRow
func (r *JobRepository) scanRowsRow(rows *sql.Rows) (*models.JobRow, error) {
	jr := &models.JobRow{}
	var metadata sql.NullString

	err := rows.Scan(
		&jr.ID, &jr.Name, &jr.Target, &jr.Fuzzer, &jr.Type, &jr.Status, &jr.AssignedBot,
		&jr.CreatedAt, &jr.StartedAt, &jr.CompletedAt, &jr.TimeoutAt,
		&jr.WorkDir, &jr.ConfigJSON, &jr.Progress, &jr.CampaignID, &jr.CollectionID,
		&jr.UseCampaignCorpus, &jr.EnableCoverage, &jr.CoverageFormat, &jr.CoverageReportID,
		&jr.LeaseToken, &jr.LeaseExpiresAt, &jr.LastHeartbeat, &jr.UpdatedAt,
		&jr.Description,
		&jr.ScheduledAt, &jr.QueuedAt, &jr.DequeueCount,
		&jr.RetryCount, &jr.MaxRetries, &jr.RetryDelayNanos,
		&jr.LockedBy, &jr.LockedAt, &jr.LockExpiresAt,
		&jr.ErrorMessage, &jr.ExecutionTimeNanos,
		&jr.CorpusPath, &jr.OutputPath,
	)
	if err != nil {
		return nil, err
	}

	jr.MetadataJSON = metadata
	return jr, nil
}
