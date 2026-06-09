package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// GetAllJobs retrieves all jobs from the database
func (s *SQLiteStorage) GetAllJobs(ctx context.Context) ([]map[string]any, error) {
	query := `SELECT id, name, target, fuzzer, status, assigned_bot, created_at,
	          started_at, completed_at, timeout_at, work_dir, config, progress
	          FROM jobs`

	return RetryableQuery(ctx, s.db, s.config, query, func(rows *sql.Rows) (map[string]any, error) {
		var id, name, target, fuzzer, status, workDir string
		var assignedBot, config sql.NullString
		var createdAt, timeoutAt sql.NullTime
		var startedAt, completedAt sql.NullTime
		var progress sql.NullInt64

		err := rows.Scan(&id, &name, &target, &fuzzer, &status, &assignedBot,
			&createdAt, &startedAt, &completedAt, &timeoutAt, &workDir, &config, &progress)
		if err != nil {
			return nil, err
		}

		job := map[string]any{
			"id":       id,
			"name":     name,
			"target":   target,
			"fuzzer":   fuzzer,
			"status":   status,
			"work_dir": workDir,
			"progress": 0, // Default to 0
		}

		if createdAt.Valid {
			job["created_at"] = createdAt.Time
		}
		if timeoutAt.Valid {
			job["timeout_at"] = timeoutAt.Time
		}
		if assignedBot.Valid {
			job["assigned_bot"] = assignedBot.String
		}
		if startedAt.Valid {
			job["started_at"] = startedAt.Time
		}
		if completedAt.Valid {
			job["completed_at"] = completedAt.Time
		}
		if config.Valid {
			job["config"] = config.String
		}
		if progress.Valid {
			job["progress"] = int(progress.Int64)
		}

		return job, nil
	})
}

// Iterate implements iteration over keys with a given prefix
func (s *SQLiteStorage) Iterate(ctx context.Context, prefix string, fn func(key string, value []byte) error) error {
	// Add timeout check
	if err := ctx.Err(); err != nil {
		return err
	}

	// Check if database is closed
	if s.db == nil {
		return common.ErrDatabaseClosed
	}

	return ExecuteWithRetry(ctx, s.config, func() error {
		// Determine which table to query based on prefix
		var query string
		switch prefix {
		case "job:":
			query = `SELECT id, json_object('id', id, 'name', name, 'target', target, 'fuzzer', fuzzer,
			         'status', status, 'assigned_bot', assigned_bot, 'created_at', created_at,
			         'started_at', started_at, 'completed_at', completed_at, 'timeout_at', timeout_at,
			         'work_dir', work_dir, 'config', json(config), 'progress', progress) FROM jobs`
		case "bot:":
			query = `SELECT id, json_object('id', id, 'hostname', hostname, 'status', status,
			         'last_seen', last_seen, 'registered_at', registered_at, 'current_job', current_job,
			         'capabilities', json(capabilities), 'timeout_at', timeout_at,
			         'is_online', CASE WHEN is_online = 1 THEN json('true') ELSE json('false') END,
			         'failure_count', failure_count) FROM bots`
		default:
			// For metadata table
			query = `SELECT key, value FROM metadata WHERE key LIKE ? || '%'`
		}

		if prefix == "job:" || prefix == "bot:" {
			rows, err := s.db.QueryContext(ctx, query)
			if err != nil {
				return common.NewDatabaseError("iterate_query", err)
			}
			defer rows.Close()

			for rows.Next() {
				// Check context timeout during iteration
				if err := ctx.Err(); err != nil {
					return err
				}

				var id, data string
				if err := rows.Scan(&id, &data); err != nil {
					s.logger.WithError(err).Warn("Failed to scan row during iteration")
					continue
				}

				key := prefix + id
				if err := fn(key, []byte(data)); err != nil {
					return err
				}
			}
			return rows.Err()
		}
		// Query metadata table
		rows, err := s.db.QueryContext(ctx, query, prefix)
		if err != nil {
			return common.NewDatabaseError("iterate_metadata", err)
		}
		defer rows.Close()

		for rows.Next() {
			// Check context timeout during iteration
			if err := ctx.Err(); err != nil {
				return err
			}

			var key, value string
			if err := rows.Scan(&key, &value); err != nil {
				s.logger.WithError(err).Warn("Failed to scan metadata row")
				continue
			}

			if err := fn(key, []byte(value)); err != nil {
				return err
			}
		}
		return rows.Err()
	})
}

// Select implements Query interface
func (s *SQLiteStorage) Select(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	return ExecuteWithRetryResult(ctx, s.config, func() ([]map[string]any, error) {
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		// Get column names
		cols, err := rows.Columns()
		if err != nil {
			return nil, err
		}

		var results []map[string]any
		for rows.Next() {
			// Check context timeout during iteration
			if err := ctx.Err(); err != nil {
				return results, err
			}

			// Create a slice of any to hold column values
			values := make([]any, len(cols))
			valuePtrs := make([]any, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				return nil, err
			}

			// Create map for this row
			row := make(map[string]any)
			for i, col := range cols {
				row[col] = values[i]
			}
			results = append(results, row)
		}

		return results, rows.Err()
	})
}

// SelectOne implements Query interface
func (s *SQLiteStorage) SelectOne(ctx context.Context, query string, args ...any) (map[string]any, error) {
	results, err := s.Select(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, sql.ErrNoRows
	}
	return results[0], nil
}

// Execute implements Query interface
func (s *SQLiteStorage) Execute(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := RetryableExec(ctx, s.db, s.config, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// scanJob scans a job from a database row
func (s *SQLiteStorage) scanJob(rows *sql.Rows) (*common.Job, error) {
	job := &common.Job{}
	var assignedBot sql.NullString
	var startedAt, completedAt sql.NullTime
	var configJSON sql.NullString
	var progress sql.NullInt64

	err := rows.Scan(
		&job.ID, &job.Name, &job.Target, &job.Fuzzer, &job.Status, &assignedBot,
		&job.CreatedAt, &startedAt, &completedAt, &job.TimeoutAt, &job.WorkDir, &configJSON, &progress)
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if assignedBot.Valid {
		job.AssignedBot = &assignedBot.String
	}
	if startedAt.Valid {
		job.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Time
	}
	if progress.Valid {
		job.Progress = int(progress.Int64)
	}

	// Parse job config JSON
	if configJSON.Valid && configJSON.String != "" {
		if err := json.Unmarshal([]byte(configJSON.String), &job.Config); err != nil {
			// Log error but don't fail - use default config
			s.logger.WithError(err).WithField("job_id", job.ID).Warn("Failed to unmarshal job config")
		}
	}

	return job, nil
}

// CreateJob creates a new job in the database
func (s *SQLiteStorage) CreateJob(ctx context.Context, job *common.Job) error {
	// Serialize job config to JSON
	configJSON, err := json.Marshal(job.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal job config: %w", err)
	}

	return ExecuteWithRetry(ctx, s.config, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO jobs (
				id, name, target, fuzzer, status, assigned_bot,
				created_at, started_at, completed_at, timeout_at, work_dir, config, progress
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, job.ID, job.Name, job.Target, job.Fuzzer, job.Status, job.AssignedBot,
			job.CreatedAt, job.StartedAt, job.CompletedAt, job.TimeoutAt, job.WorkDir,
			string(configJSON), job.Progress)
		return err
	})
}

// GetJob retrieves a job by ID
func (s *SQLiteStorage) GetJob(ctx context.Context, id string) (*common.Job, error) {
	query := `SELECT id, name, target, fuzzer, status, assigned_bot,
		created_at, started_at, completed_at, timeout_at, work_dir, config, progress,
		COALESCE(enable_coverage, 0), COALESCE(coverage_format, '')
		FROM jobs WHERE id = ?`

	var job common.Job
	var assignedBot sql.NullString
	var startedAt, completedAt sql.NullTime
	var configJSON sql.NullString
	var progress sql.NullInt64
	var enableCoverage int
	var coverageFormat string

	err := ExecuteWithRetry(ctx, s.config, func() error {
		return s.db.QueryRowContext(ctx, query, id).Scan(
			&job.ID, &job.Name, &job.Target, &job.Fuzzer, &job.Status, &assignedBot,
			&job.CreatedAt, &startedAt, &completedAt, &job.TimeoutAt, &job.WorkDir,
			&configJSON, &progress, &enableCoverage, &coverageFormat)
	})

	if err == sql.ErrNoRows {
		return nil, common.ErrKeyNotFound
	}
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if assignedBot.Valid {
		job.AssignedBot = &assignedBot.String
	}
	if startedAt.Valid {
		job.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Time
	}
	if progress.Valid {
		job.Progress = int(progress.Int64)
	}

	// Parse job config JSON
	if configJSON.Valid && configJSON.String != "" {
		if err := json.Unmarshal([]byte(configJSON.String), &job.Config); err != nil {
			// Log error but don't fail - use default config
			s.logger.WithError(err).WithField("job_id", job.ID).Warn("Failed to unmarshal job config")
		}
	}

	// Set coverage fields
	job.EnableCoverage = enableCoverage == 1
	job.CoverageFormat = coverageFormat

	return &job, nil
}

// UpdateJob updates a job with the provided fields
func (s *SQLiteStorage) UpdateJob(ctx context.Context, id string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	// Build dynamic update query
	query := "UPDATE jobs SET updated_at = CURRENT_TIMESTAMP"
	args := []interface{}{}

	for field, value := range updates {
		switch field {
		case "name", "target", "fuzzer", "status", "work_dir":
			query += fmt.Sprintf(", %s = ?", field)
			args = append(args, value)
		case "assigned_bot":
			query += ", assigned_bot = ?"
			args = append(args, value)
		case "started_at", "completed_at", "timeout_at":
			query += fmt.Sprintf(", %s = ?", field)
			args = append(args, value)
		case "progress":
			query += ", progress = ?"
			args = append(args, value)
		case "config":
			configJSON, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("failed to marshal job config: %w", err)
			}
			query += ", config = ?"
			args = append(args, string(configJSON))
		}
	}

	query += " WHERE id = ?"
	args = append(args, id)

	return ExecuteWithRetry(ctx, s.config, func() error {
		result, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}

		if rowsAffected == 0 {
			return common.ErrKeyNotFound
		}

		return nil
	})
}

// ListJobs retrieves jobs with pagination and optional status filter
func (s *SQLiteStorage) ListJobs(ctx context.Context, limit, offset int, status string) ([]*common.Job, error) {
	// Check if database is closed
	if s.db == nil {
		return nil, common.ErrDatabaseClosed
	}

	// Build query to get ALL non-deleted jobs
	query := `SELECT id, name, target, fuzzer, status, assigned_bot,
		created_at, started_at, completed_at, timeout_at, work_dir, config, progress
		FROM jobs
		WHERE 1=1` // Start with a true condition for easier query building

	args := []interface{}{}

	// If status filter is provided, add it to the WHERE clause
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	// Order by creation time to maintain consistent ordering
	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
		if offset > 0 {
			query += " OFFSET ?"
			args = append(args, offset)
		}
	}

	var jobs []*common.Job
	err := ExecuteWithRetry(ctx, s.config, func() error {
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			job, err := s.scanJob(rows)
			if err != nil {
				return err
			}
			jobs = append(jobs, job)
		}

		return rows.Err()
	})

	return jobs, err
}

// DeleteJob deletes a job from the database
func (s *SQLiteStorage) DeleteJob(ctx context.Context, id string) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		result, err := s.db.ExecContext(ctx, "DELETE FROM jobs WHERE id = ?", id)
		if err != nil {
			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}

		if rowsAffected == 0 {
			return common.ErrKeyNotFound
		}

		return nil
	})
}

// CreateCoverage creates a new coverage result
func (s *SQLiteStorage) CreateCoverage(ctx context.Context, coverage *common.CoverageResult) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO coverage (
				id, job_id, bot_id, edges, new_edges, timestamp, exec_count
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`, coverage.ID, coverage.JobID, coverage.BotID, coverage.Edges,
			coverage.NewEdges, coverage.Timestamp, coverage.ExecCount)
		return err
	})
}

// GetLatestCoverage gets the latest coverage for a job
func (s *SQLiteStorage) GetLatestCoverage(ctx context.Context, jobID string) (*common.CoverageResult, error) {
	var coverage common.CoverageResult

	err := ExecuteWithRetry(ctx, s.config, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT id, job_id, bot_id, edges, new_edges, timestamp, exec_count
			FROM coverage
			WHERE job_id = ?
			ORDER BY timestamp DESC
			LIMIT 1
		`, jobID).Scan(
			&coverage.ID, &coverage.JobID, &coverage.BotID, &coverage.Edges,
			&coverage.NewEdges, &coverage.Timestamp, &coverage.ExecCount)
	})

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &coverage, nil
}
