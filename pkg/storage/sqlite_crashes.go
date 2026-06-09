package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// scanCrash scans a crash from a database row
func (s *SQLiteStorage) scanCrash(rows *sql.Rows) (*common.CrashResult, error) {
	crash := &common.CrashResult{}
	var output, stackTrace sql.NullString

	err := rows.Scan(
		&crash.ID, &crash.JobID, &crash.BotID, &crash.Hash, &crash.FilePath,
		&crash.Type, &crash.Signal, &crash.ExitCode, &crash.Timestamp, &crash.Size,
		&crash.IsUnique, &output, &stackTrace)
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if output.Valid {
		crash.Output = output.String
	}
	if stackTrace.Valid {
		crash.StackTrace = stackTrace.String
	}

	return crash, nil
}

// GetCrashes retrieves crashes with pagination
func (s *SQLiteStorage) GetCrashes(ctx context.Context, limit, offset int) ([]*common.CrashResult, error) {
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		s.logger.WithError(err).Debug("Context cancelled before querying crashes")
		return nil, err
	}

	query := `SELECT id, job_id, bot_id, hash, file_path, type, signal, exit_code, timestamp, size, is_unique, output, stack_trace
	          FROM crashes
	          ORDER BY timestamp DESC
	          LIMIT ? OFFSET ?`

	return RetryableQuery(ctx, s.db, s.config, query, func(rows *sql.Rows) (*common.CrashResult, error) {
		// Check context during iteration
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		crash := &common.CrashResult{}
		var output, stackTrace sql.NullString
		err := rows.Scan(&crash.ID, &crash.JobID, &crash.BotID, &crash.Hash, &crash.FilePath,
			&crash.Type, &crash.Signal, &crash.ExitCode, &crash.Timestamp, &crash.Size, &crash.IsUnique,
			&output, &stackTrace)
		if err != nil {
			return nil, err
		}
		crash.Output = output.String
		crash.StackTrace = stackTrace.String

		// Load crash input data from separate table if context not cancelled
		if ctx.Err() == nil {
			if input, err := s.GetCrashInput(ctx, crash.ID); err == nil && input != nil {
				crash.Input = input
			}
		}

		return crash, nil
	}, limit, offset)
}

// GetCrashesSorted retrieves crashes with sorting support
func (s *SQLiteStorage) GetCrashesSorted(ctx context.Context, limit, offset int, sortBy, sortOrder string) ([]*common.CrashResult, error) {
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		s.logger.WithError(err).Debug("Context cancelled before querying crashes")
		return nil, err
	}

	// Map sort field names to database columns
	columnMap := map[string]string{
		"timestamp": "timestamp",
		"type":      "type",
		"signal":    "signal",
		"size":      "size",
		"job_id":    "job_id",
		"bot_id":    "bot_id",
	}

	// Default to timestamp if invalid sort field
	sortColumn, ok := columnMap[sortBy]
	if !ok {
		sortColumn = "timestamp"
	}

	// Validate sort order
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	// Build query with dynamic ORDER BY clause
	query := fmt.Sprintf(`SELECT id, job_id, bot_id, hash, file_path, type, signal, exit_code, timestamp, size, is_unique, output, stack_trace
	          FROM crashes
	          ORDER BY %s %s
	          LIMIT ? OFFSET ?`, sortColumn, sortOrder)

	return RetryableQuery(ctx, s.db, s.config, query, func(rows *sql.Rows) (*common.CrashResult, error) {
		// Check context during iteration
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		crash := &common.CrashResult{}
		var output, stackTrace sql.NullString
		err := rows.Scan(&crash.ID, &crash.JobID, &crash.BotID, &crash.Hash, &crash.FilePath,
			&crash.Type, &crash.Signal, &crash.ExitCode, &crash.Timestamp, &crash.Size, &crash.IsUnique,
			&output, &stackTrace)
		if err != nil {
			return nil, err
		}
		crash.Output = output.String
		crash.StackTrace = stackTrace.String

		// Load crash input data from separate table if context not cancelled
		if ctx.Err() == nil {
			if input, err := s.GetCrashInput(ctx, crash.ID); err == nil && input != nil {
				crash.Input = input
			}
		}

		return crash, nil
	}, limit, offset)
}

// GetCrash retrieves a specific crash by ID
func (s *SQLiteStorage) GetCrash(ctx context.Context, crashID string) (*common.CrashResult, error) {
	query := `SELECT id, job_id, bot_id, hash, file_path, type, signal, exit_code, timestamp, size, is_unique, output, stack_trace
	          FROM crashes
	          WHERE id = ?`

	return RetryableQueryRow(ctx, s.db, s.config, query, func(row *sql.Row) (*common.CrashResult, error) {
		crash := &common.CrashResult{}
		var output, stackTrace sql.NullString
		err := row.Scan(&crash.ID, &crash.JobID, &crash.BotID, &crash.Hash, &crash.FilePath,
			&crash.Type, &crash.Signal, &crash.ExitCode, &crash.Timestamp, &crash.Size, &crash.IsUnique,
			&output, &stackTrace)

		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}

		crash.Output = output.String
		crash.StackTrace = stackTrace.String

		// Load crash input data from separate table
		if input, err := s.GetCrashInput(ctx, crash.ID); err == nil && input != nil {
			crash.Input = input
		}

		return crash, nil
	}, crashID)
}

// GetJobCrashes retrieves all crashes for a specific job
func (s *SQLiteStorage) GetJobCrashes(ctx context.Context, jobID string) ([]*common.CrashResult, error) {
	query := `SELECT id, job_id, bot_id, hash, file_path, type, signal, exit_code, timestamp, size, is_unique, output, stack_trace
	          FROM crashes
	          WHERE job_id = ?
	          ORDER BY timestamp DESC`

	return RetryableQuery(ctx, s.db, s.config, query, func(rows *sql.Rows) (*common.CrashResult, error) {
		crash := &common.CrashResult{}
		var output, stackTrace sql.NullString
		err := rows.Scan(&crash.ID, &crash.JobID, &crash.BotID, &crash.Hash, &crash.FilePath,
			&crash.Type, &crash.Signal, &crash.ExitCode, &crash.Timestamp, &crash.Size, &crash.IsUnique,
			&output, &stackTrace)
		if err != nil {
			return nil, err
		}
		crash.Output = output.String
		crash.StackTrace = stackTrace.String

		// Load crash input data from separate table
		if input, err := s.GetCrashInput(ctx, crash.ID); err == nil && input != nil {
			crash.Input = input
		}

		return crash, nil
	}, jobID)
}

// StoreCrashInput stores crash input data separately
func (s *SQLiteStorage) StoreCrashInput(ctx context.Context, crashID string, input []byte) error {
	s.logger.WithFields(logrus.Fields{
		"crash_id":   crashID,
		"input_size": len(input),
	}).Debug("Storing crash input to database")

	query := `INSERT OR REPLACE INTO crash_inputs (crash_id, input) VALUES (?, ?)`
	_, err := RetryableExec(ctx, s.db, s.config, query, crashID, input)

	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"crash_id":   crashID,
			"input_size": len(input),
		}).Error("Failed to store crash input in database")
		return err
	}

	s.logger.WithFields(logrus.Fields{
		"crash_id":   crashID,
		"input_size": len(input),
	}).Info("Successfully stored crash input in database")

	return nil
}

// GetCrashInput retrieves crash input data
func (s *SQLiteStorage) GetCrashInput(ctx context.Context, crashID string) ([]byte, error) {
	query := `SELECT input FROM crash_inputs WHERE crash_id = ?`

	return RetryableQueryRow(ctx, s.db, s.config, query, func(row *sql.Row) ([]byte, error) {
		var input []byte
		err := row.Scan(&input)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return input, nil
	}, crashID)
}

// CreateCrash creates a new crash result in the database
func (s *SQLiteStorage) CreateCrash(ctx context.Context, crash *common.CrashResult) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		result, err := s.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO crashes (
				id, job_id, bot_id, hash, file_path, type, signal, exit_code,
				timestamp, size, is_unique, output, stack_trace
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, crash.ID, crash.JobID, crash.BotID, crash.Hash, crash.FilePath,
			crash.Type, crash.Signal, crash.ExitCode, crash.Timestamp, crash.Size,
			crash.IsUnique, crash.Output, crash.StackTrace)
		if err != nil {
			return err
		}
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			s.logger.WithFields(logrus.Fields{
				"job_id": crash.JobID,
				"hash":   crash.Hash,
			}).Debug("Crash already exists (duplicate hash for job), skipped insertion")
			return common.ErrDuplicateCrash
		}
		return nil
	})
}

// ListCrashes retrieves crashes for a job with pagination
func (s *SQLiteStorage) ListCrashes(ctx context.Context, jobID string, limit, offset int) ([]*common.CrashResult, error) {
	query := `SELECT id, job_id, bot_id, hash, file_path, type, signal, exit_code,
		timestamp, size, is_unique, output, stack_trace
		FROM crashes`
	args := []interface{}{}

	if jobID != "" {
		query += " WHERE job_id = ?"
		args = append(args, jobID)
	}

	query += " ORDER BY timestamp DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
		if offset > 0 {
			query += " OFFSET ?"
			args = append(args, offset)
		}
	}

	var crashes []*common.CrashResult
	err := ExecuteWithRetry(ctx, s.config, func() error {
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			crash, err := s.scanCrash(rows)
			if err != nil {
				return err
			}
			crashes = append(crashes, crash)
		}

		return rows.Err()
	})

	return crashes, err
}

// GetCrashCount returns the total count of crashes, optionally filtered by job ID
func (s *SQLiteStorage) GetCrashCount(ctx context.Context, jobID string) (int, error) {
	query := `SELECT COUNT(*) FROM crashes`
	args := []interface{}{}

	if jobID != "" {
		query += " WHERE job_id = ?"
		args = append(args, jobID)
	}

	var count int
	err := ExecuteWithRetry(ctx, s.config, func() error {
		return s.db.QueryRowContext(ctx, query, args...).Scan(&count)
	})

	return count, err
}

// UpdateCrashReproducibility updates crash reproducibility fields
func (s *SQLiteStorage) UpdateCrashReproducibility(ctx context.Context, crashID string, reproducible bool, score float64) error {
	query := `UPDATE crashes SET
		reproducible = ?,
		reproducibility_score = ?,
		reproduction_attempts = reproduction_attempts + 1,
		last_reproduction_at = CURRENT_TIMESTAMP,
		updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`

	var reproducibleInt int
	if reproducible {
		reproducibleInt = 1
	} else {
		reproducibleInt = 0
	}

	result, err := RetryableExec(ctx, s.db, s.config, query, reproducibleInt, score, crashID)
	if err != nil {
		return common.NewDatabaseError("update_crash_reproducibility", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return common.NewDatabaseError("check_rows_affected", err)
	}

	if rowsAffected == 0 {
		return common.ErrKeyNotFound
	}

	s.logger.WithFields(logrus.Fields{
		"crash_id":     crashID,
		"reproducible": reproducible,
		"score":        score,
	}).Info("Updated crash reproducibility")

	return nil
}

// CreateReproductionResult stores a reproduction attempt result
func (s *SQLiteStorage) CreateReproductionResult(ctx context.Context, result *common.ReproductionResult) error {
	query := `INSERT INTO reproduction_results (
		id, crash_id, campaign_id, job_id, bot_id, attempt_number,
		success, execution_time, output, stack_trace, stack_hash,
		matches_original, test_binary_hash, corpus_used, notes
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// Determine success value (1 for reproduced, 0 for not)
	var success int
	if result.Reproduced {
		success = 1
	} else {
		success = 0
	}

	// Get campaign ID from the crash's job
	var campaignID sql.NullString
	var jobID string
	err := s.db.QueryRowContext(ctx, `
		SELECT c.job_id, j.campaign_id
		FROM crashes c
		LEFT JOIN jobs j ON c.job_id = j.id
		WHERE c.id = ?`, result.CrashID).Scan(&jobID, &campaignID)

	if err != nil && err != sql.ErrNoRows {
		return common.NewDatabaseError("get_crash_campaign", err)
	}

	// Build notes from environment info and status
	notes := fmt.Sprintf("Status: %s", result.Status)
	if result.Signal > 0 {
		notes += fmt.Sprintf("\nSignal: %d", result.Signal)
	}
	if result.ExitCode != 0 {
		notes += fmt.Sprintf("\nExit Code: %d", result.ExitCode)
	}
	if len(result.EnvironmentInfo) > 0 {
		envJSON, _ := json.Marshal(result.EnvironmentInfo)
		notes += fmt.Sprintf("\nEnvironment: %s", string(envJSON))
	}

	// Determine corpus used based on request info
	corpusUsed := "original"
	if result.RequestID != "" {
		corpusUsed = "campaign"
	}

	_, err = RetryableExec(ctx, s.db, s.config, query,
		result.ID, result.CrashID, campaignID, jobID, result.BotID,
		result.AttemptNumber, success, result.ExecutionTime.Milliseconds(),
		result.Output, result.StackTrace, result.StackHash,
		result.MatchesOriginal, "", corpusUsed, notes)

	if err != nil {
		return common.NewDatabaseError("create_reproduction_result", err)
	}

	// Update crash reproducibility based on result
	if result.Reproduced {
		score := 0.5
		if result.MatchesOriginal {
			score = 1.0
		}
		if err := s.UpdateCrashReproducibility(ctx, result.CrashID, true, score); err != nil {
			s.logger.WithError(err).WithField("crash_id", result.CrashID).Warn("Failed to update crash reproducibility")
		}
	}

	s.logger.WithFields(logrus.Fields{
		"result_id":  result.ID,
		"crash_id":   result.CrashID,
		"reproduced": result.Reproduced,
		"matches":    result.MatchesOriginal,
	}).Info("Created reproduction result")

	return nil
}

// GetReproductionResults retrieves all reproduction results for a crash
func (s *SQLiteStorage) GetReproductionResults(ctx context.Context, crashID string) ([]*common.ReproductionResult, error) {
	query := `SELECT
		id, crash_id, campaign_id, job_id, bot_id, attempt_number,
		success, execution_time, output, stack_trace, stack_hash,
		matches_original, test_binary_hash, corpus_used, notes, created_at
		FROM reproduction_results
		WHERE crash_id = ?
		ORDER BY created_at DESC`

	return RetryableQuery(ctx, s.db, s.config, query, func(rows *sql.Rows) (*common.ReproductionResult, error) {
		result := &common.ReproductionResult{}
		var campaignID, jobID, testBinaryHash, corpusUsed, notes sql.NullString
		var execTimeMs sql.NullInt64
		var success int
		var matchesOriginal sql.NullBool

		err := rows.Scan(
			&result.ID, &result.CrashID, &campaignID, &jobID, &result.BotID,
			&result.AttemptNumber, &success, &execTimeMs, &result.Output,
			&result.StackTrace, &result.StackHash, &matchesOriginal,
			&testBinaryHash, &corpusUsed, &notes, &result.Timestamp)

		if err != nil {
			return nil, err
		}

		// Set reproduced based on success value
		result.Reproduced = (success == 1)

		// Convert execution time from milliseconds
		if execTimeMs.Valid {
			result.ExecutionTime = time.Duration(execTimeMs.Int64) * time.Millisecond
		}

		// Handle nullable fields
		if matchesOriginal.Valid {
			result.MatchesOriginal = matchesOriginal.Bool
		}

		// Parse notes to extract additional info
		if notes.Valid && notes.String != "" {
			lines := strings.Split(notes.String, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "Status: ") {
					result.Status = common.ReproducibilityStatus(strings.TrimPrefix(line, "Status: "))
				} else if strings.HasPrefix(line, "Signal: ") {
					fmt.Sscanf(line, "Signal: %d", &result.Signal)
				} else if strings.HasPrefix(line, "Exit Code: ") {
					fmt.Sscanf(line, "Exit Code: %d", &result.ExitCode)
				} else if strings.HasPrefix(line, "Environment: ") {
					envJSON := strings.TrimPrefix(line, "Environment: ")
					if err := json.Unmarshal([]byte(envJSON), &result.EnvironmentInfo); err != nil {
						result.EnvironmentInfo = make(map[string]string)
					}
				}
			}
		}

		// Initialize environment info if not set
		if result.EnvironmentInfo == nil {
			result.EnvironmentInfo = make(map[string]string)
		}

		// Set default status if not found in notes
		if result.Status == "" {
			if result.Reproduced {
				result.Status = common.ReproducibilityStatusConfirmed
			} else {
				result.Status = common.ReproducibilityStatusFailed
			}
		}

		return result, nil
	}, crashID)
}

// GetCrashesForReproduction retrieves crashes that need reproduction testing
func (s *SQLiteStorage) GetCrashesForReproduction(ctx context.Context, limit int) ([]*common.CrashResult, error) {
	query := `SELECT
		id, job_id, bot_id, hash, file_path, type, signal, exit_code,
		timestamp, size, is_unique, output, stack_trace
		FROM crashes
		WHERE
			(reproducible IS NULL) OR
			(reproduction_attempts < 3 AND reproducibility_score < 0.8) OR
			(last_reproduction_at < datetime('now', '-24 hours'))
		ORDER BY
			CASE WHEN reproducible IS NULL THEN 0 ELSE 1 END,
			reproduction_attempts ASC,
			timestamp DESC
		LIMIT ?`

	return RetryableQuery(ctx, s.db, s.config, query, func(rows *sql.Rows) (*common.CrashResult, error) {
		crash := &common.CrashResult{}
		var output, stackTrace sql.NullString

		err := rows.Scan(
			&crash.ID, &crash.JobID, &crash.BotID, &crash.Hash, &crash.FilePath,
			&crash.Type, &crash.Signal, &crash.ExitCode, &crash.Timestamp,
			&crash.Size, &crash.IsUnique, &output, &stackTrace)

		if err != nil {
			return nil, err
		}

		crash.Output = output.String
		crash.StackTrace = stackTrace.String

		// Load crash input
		if input, err := s.GetCrashInput(ctx, crash.ID); err == nil && input != nil {
			crash.Input = input
		}

		return crash, nil
	}, limit)
}
