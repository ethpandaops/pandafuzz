package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// Compile-time interface compliance check
var _ common.Storage = (*SQLiteStorage)(nil)

// Bot operations

// CreateBot creates a new bot record
func (s *SQLiteStorage) CreateBot(ctx context.Context, bot *common.Bot) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO bots (
				id, name, hostname, status, last_seen, registered_at,
				current_job, timeout_at, is_online, failure_count, api_endpoint
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, bot.ID, bot.Name, bot.Hostname, bot.Status, bot.LastSeen, bot.RegisteredAt,
			bot.CurrentJob, bot.TimeoutAt, bot.IsOnline, bot.FailureCount, bot.APIEndpoint)
		return err
	})
}

// GetBot retrieves a bot by ID
func (s *SQLiteStorage) GetBot(ctx context.Context, id string) (*common.Bot, error) {
	var bot common.Bot
	var currentJob sql.NullString
	var timeoutAt sql.NullTime

	err := ExecuteWithRetry(ctx, s.config, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT id, name, hostname, status, last_seen, registered_at,
				current_job, timeout_at, is_online, failure_count, api_endpoint
			FROM bots WHERE id = ?
		`, id).Scan(
			&bot.ID, &bot.Name, &bot.Hostname, &bot.Status, &bot.LastSeen, &bot.RegisteredAt,
			&currentJob, &timeoutAt, &bot.IsOnline, &bot.FailureCount, &bot.APIEndpoint)
	})

	if err == sql.ErrNoRows {
		return nil, common.ErrKeyNotFound
	}
	if err != nil {
		return nil, err
	}

	if currentJob.Valid {
		bot.CurrentJob = &currentJob.String
	}
	if timeoutAt.Valid {
		bot.TimeoutAt = timeoutAt.Time
	}

	// Get capabilities
	rows, err := s.db.QueryContext(ctx, `SELECT capability FROM bot_capabilities WHERE bot_id = ?`, id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var cap string
			if err := rows.Scan(&cap); err == nil {
				bot.Capabilities = append(bot.Capabilities, cap)
			}
		}
	}

	return &bot, nil
}

// UpdateBot updates a bot record
func (s *SQLiteStorage) UpdateBot(ctx context.Context, id string, updates map[string]interface{}) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		// Build dynamic update query
		query := "UPDATE bots SET "
		args := make([]interface{}, 0)
		first := true

		for key, value := range updates {
			if !first {
				query += ", "
			}
			query += key + " = ?"
			args = append(args, value)
			first = false
		}

		query += " WHERE id = ?"
		args = append(args, id)

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

// ListBots retrieves all bots
func (s *SQLiteStorage) ListBots(ctx context.Context) ([]*common.Bot, error) {
	var bots []*common.Bot

	err := ExecuteWithRetry(ctx, s.config, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, name, hostname, status, last_seen, registered_at,
				current_job, timeout_at, is_online, failure_count, api_endpoint
			FROM bots
			ORDER BY registered_at DESC
		`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var bot common.Bot
			var currentJob sql.NullString
			var timeoutAt sql.NullTime

			err := rows.Scan(
				&bot.ID, &bot.Name, &bot.Hostname, &bot.Status, &bot.LastSeen, &bot.RegisteredAt,
				&currentJob, &timeoutAt, &bot.IsOnline, &bot.FailureCount, &bot.APIEndpoint)
			if err != nil {
				return err
			}

			if currentJob.Valid {
				bot.CurrentJob = &currentJob.String
			}
			if timeoutAt.Valid {
				bot.TimeoutAt = timeoutAt.Time
			}

			bots = append(bots, &bot)
		}

		return rows.Err()
	})

	return bots, err
}

// DeleteBot deletes a bot by ID
func (s *SQLiteStorage) DeleteBot(ctx context.Context, id string) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		result, err := s.db.ExecContext(ctx, "DELETE FROM bots WHERE id = ?", id)
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

// Corpus file operations

// GetCorpusFile retrieves a specific corpus file by ID
func (s *SQLiteStorage) GetCorpusFile(ctx context.Context, fileID string) (*common.CorpusFile, error) {
	cf := &common.CorpusFile{}
	var syncedAt sql.NullTime

	err := ExecuteWithRetry(ctx, s.config, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT id, campaign_id, job_id, bot_id, filename, hash, size,
				coverage, new_coverage, parent_hash, generation, created_at, synced_at, is_seed
			FROM campaign_corpus_files
			WHERE id = ?
		`, fileID).Scan(
			&cf.ID, &cf.CampaignID, &cf.JobID, &cf.BotID, &cf.Filename, &cf.Hash, &cf.Size,
			&cf.Coverage, &cf.NewCoverage, &cf.ParentHash, &cf.Generation,
			&cf.CreatedAt, &syncedAt, &cf.IsSeed)
	})

	if err == sql.ErrNoRows {
		return nil, common.ErrCorpusFileNotFound
	}
	if err != nil {
		return nil, err
	}

	if syncedAt.Valid {
		cf.SyncedAt = &syncedAt.Time
	}

	return cf, nil
}

// UpdateCorpusFile updates a corpus file record
func (s *SQLiteStorage) UpdateCorpusFile(ctx context.Context, fileID string, updates map[string]interface{}) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		query := "UPDATE campaign_corpus_files SET "
		args := make([]interface{}, 0)
		first := true

		for key, value := range updates {
			if !first {
				query += ", "
			}
			query += key + " = ?"
			args = append(args, value)
			first = false
		}

		query += " WHERE id = ?"
		args = append(args, fileID)

		result, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			return common.ErrCorpusFileNotFound
		}

		return nil
	})
}

// DeleteCorpusFile deletes a corpus file by ID
func (s *SQLiteStorage) DeleteCorpusFile(ctx context.Context, fileID string) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		result, err := s.db.ExecContext(ctx, "DELETE FROM campaign_corpus_files WHERE id = ?", fileID)
		if err != nil {
			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			return common.ErrCorpusFileNotFound
		}

		return nil
	})
}

// Quarantine operations

// AddQuarantinedFile adds a file to quarantine
func (s *SQLiteStorage) AddQuarantinedFile(ctx context.Context, qf *common.QuarantinedFile) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO quarantined_files (
				id, file_id, campaign_id, hash, reason, details,
				quarantined_at, quarantined_by, reviewed_at, reviewed_by, resolution
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, qf.ID, qf.FileID, qf.CampaignID, qf.Hash, qf.Reason, qf.Details,
			qf.QuarantinedAt, qf.QuarantinedBy, qf.ReviewedAt, qf.ReviewedBy, qf.Resolution)
		return err
	})
}

// GetQuarantinedFile retrieves a quarantined file by ID
func (s *SQLiteStorage) GetQuarantinedFile(ctx context.Context, fileID string) (*common.QuarantinedFile, error) {
	var qf common.QuarantinedFile
	var reviewedAt sql.NullTime
	var reviewedBy, resolution sql.NullString

	err := ExecuteWithRetry(ctx, s.config, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT id, file_id, campaign_id, hash, reason, details,
				quarantined_at, quarantined_by, reviewed_at, reviewed_by, resolution
			FROM quarantined_files
			WHERE id = ?
		`, fileID).Scan(
			&qf.ID, &qf.FileID, &qf.CampaignID, &qf.Hash, &qf.Reason, &qf.Details,
			&qf.QuarantinedAt, &qf.QuarantinedBy, &reviewedAt, &reviewedBy, &resolution)
	})

	if err == sql.ErrNoRows {
		return nil, common.ErrQuarantinedFileNotFound
	}
	if err != nil {
		return nil, err
	}

	if reviewedAt.Valid {
		qf.ReviewedAt = &reviewedAt.Time
	}
	if reviewedBy.Valid {
		qf.ReviewedBy = &reviewedBy.String
	}
	if resolution.Valid {
		qf.Resolution = &resolution.String
	}

	return &qf, nil
}

// GetQuarantinedFiles retrieves all quarantined files for a campaign
func (s *SQLiteStorage) GetQuarantinedFiles(ctx context.Context, campaignID string) ([]*common.QuarantinedFile, error) {
	var files []*common.QuarantinedFile

	err := ExecuteWithRetry(ctx, s.config, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, file_id, campaign_id, hash, reason, details,
				quarantined_at, quarantined_by, reviewed_at, reviewed_by, resolution
			FROM quarantined_files
			WHERE campaign_id = ?
			ORDER BY quarantined_at DESC
		`, campaignID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var qf common.QuarantinedFile
			var reviewedAt sql.NullTime
			var reviewedBy, resolution sql.NullString

			err := rows.Scan(
				&qf.ID, &qf.FileID, &qf.CampaignID, &qf.Hash, &qf.Reason, &qf.Details,
				&qf.QuarantinedAt, &qf.QuarantinedBy, &reviewedAt, &reviewedBy, &resolution)
			if err != nil {
				return err
			}

			if reviewedAt.Valid {
				qf.ReviewedAt = &reviewedAt.Time
			}
			if reviewedBy.Valid {
				qf.ReviewedBy = &reviewedBy.String
			}
			if resolution.Valid {
				qf.Resolution = &resolution.String
			}

			files = append(files, &qf)
		}

		return rows.Err()
	})

	return files, err
}

// UpdateQuarantinedFile updates a quarantined file record
func (s *SQLiteStorage) UpdateQuarantinedFile(ctx context.Context, id string, updates map[string]interface{}) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		query := "UPDATE quarantined_files SET "
		args := make([]interface{}, 0)
		first := true

		for key, value := range updates {
			if !first {
				query += ", "
			}
			query += key + " = ?"
			args = append(args, value)
			first = false
		}

		query += " WHERE id = ?"
		args = append(args, id)

		result, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			return common.ErrQuarantinedFileNotFound
		}

		return nil
	})
}

// Corpus metrics operations

// GetCorpusFileMetrics retrieves metrics for a corpus file
func (s *SQLiteStorage) GetCorpusFileMetrics(ctx context.Context, fileID string) (*common.CorpusFileMetrics, error) {
	var metrics common.CorpusFileMetrics
	var avgExecTimeNs int64

	err := ExecuteWithRetry(ctx, s.config, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT file_id, crash_count, timeout_count, avg_exec_time_ns,
				max_memory_usage, last_executed, exec_count
			FROM corpus_file_metrics
			WHERE file_id = ?
		`, fileID).Scan(
			&metrics.FileID, &metrics.CrashCount, &metrics.TimeoutCount,
			&avgExecTimeNs, &metrics.MaxMemoryUsage, &metrics.LastExecuted,
			&metrics.ExecCount)
	})

	if err == sql.ErrNoRows {
		// Return empty metrics if not found
		return &common.CorpusFileMetrics{FileID: fileID}, nil
	}
	if err != nil {
		return nil, err
	}

	metrics.AvgExecTime = time.Duration(avgExecTimeNs)

	return &metrics, nil
}

// UpdateCorpusFileMetrics updates metrics for a corpus file
func (s *SQLiteStorage) UpdateCorpusFileMetrics(ctx context.Context, fileID string, metrics *common.CorpusFileMetrics) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO corpus_file_metrics (
				file_id, crash_count, timeout_count, avg_exec_time_ns,
				max_memory_usage, last_executed, exec_count
			) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(file_id) DO UPDATE SET
				crash_count = excluded.crash_count,
				timeout_count = excluded.timeout_count,
				avg_exec_time_ns = excluded.avg_exec_time_ns,
				max_memory_usage = excluded.max_memory_usage,
				last_executed = excluded.last_executed,
				exec_count = excluded.exec_count
		`, fileID, metrics.CrashCount, metrics.TimeoutCount, int64(metrics.AvgExecTime),
			metrics.MaxMemoryUsage, metrics.LastExecuted, metrics.ExecCount)
		return err
	})
}

// Minimization-related operations

// CreateMinimizationResult creates a new minimization result
func (s *SQLiteStorage) CreateMinimizationResult(ctx context.Context, result *common.MinimizationResult) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO minimization_results (
				id, crash_id, job_id, bot_id, strategy, original_size, minimized_size,
				reduction_percent, iterations, execution_time_ns, success,
				minimized_path, minimized_hash, still_reproduces, error, timestamp
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, result.ID, result.CrashID, result.JobID, result.BotID, result.Strategy,
			result.OriginalSize, result.MinimizedSize, result.ReductionPercent,
			result.Iterations, int64(result.ExecutionTime), result.Success,
			result.MinimizedPath, result.MinimizedHash, result.StillReproduces,
			result.Error, result.Timestamp)
		return err
	})
}

// GetMinimizationResult retrieves a minimization result by ID
func (s *SQLiteStorage) GetMinimizationResult(ctx context.Context, resultID string) (*common.MinimizationResult, error) {
	var r common.MinimizationResult
	var execTimeNs int64

	err := ExecuteWithRetry(ctx, s.config, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT id, crash_id, job_id, bot_id, strategy, original_size, minimized_size,
				reduction_percent, iterations, execution_time_ns, success,
				minimized_path, minimized_hash, still_reproduces, error, timestamp
			FROM minimization_results
			WHERE id = ?
		`, resultID).Scan(
			&r.ID, &r.CrashID, &r.JobID, &r.BotID, &r.Strategy, &r.OriginalSize,
			&r.MinimizedSize, &r.ReductionPercent, &r.Iterations, &execTimeNs,
			&r.Success, &r.MinimizedPath, &r.MinimizedHash, &r.StillReproduces,
			&r.Error, &r.Timestamp)
	})

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	r.ExecutionTime = time.Duration(execTimeNs)
	return &r, nil
}

// ListMinimizationResults lists minimization results for a crash
func (s *SQLiteStorage) ListMinimizationResults(ctx context.Context, crashID string) ([]*common.MinimizationResult, error) {
	var results []*common.MinimizationResult

	err := ExecuteWithRetry(ctx, s.config, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, crash_id, job_id, bot_id, strategy, original_size, minimized_size,
				reduction_percent, iterations, execution_time_ns, success,
				minimized_path, minimized_hash, still_reproduces, error, timestamp
			FROM minimization_results
			WHERE crash_id = ?
			ORDER BY timestamp DESC
		`, crashID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var r common.MinimizationResult
			var execTimeNs int64
			err := rows.Scan(
				&r.ID, &r.CrashID, &r.JobID, &r.BotID, &r.Strategy, &r.OriginalSize,
				&r.MinimizedSize, &r.ReductionPercent, &r.Iterations, &execTimeNs,
				&r.Success, &r.MinimizedPath, &r.MinimizedHash, &r.StillReproduces,
				&r.Error, &r.Timestamp)
			if err != nil {
				return err
			}
			r.ExecutionTime = time.Duration(execTimeNs)
			results = append(results, &r)
		}

		return rows.Err()
	})

	return results, err
}

// GetMinimizationStats gets statistics about minimization for a campaign
func (s *SQLiteStorage) GetMinimizationStats(ctx context.Context, campaignID string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	err := ExecuteWithRetry(ctx, s.config, func() error {
		row := s.db.QueryRowContext(ctx, `
			SELECT
				COUNT(*) as total,
				SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as successful,
				AVG(CASE WHEN success = 1 THEN reduction_percent ELSE NULL END) as avg_reduction
			FROM minimization_results mr
			JOIN crashes c ON mr.crash_id = c.id
			WHERE c.campaign_id = ?
		`, campaignID)

		var total, successful int
		var avgReduction sql.NullFloat64
		if err := row.Scan(&total, &successful, &avgReduction); err != nil {
			return err
		}

		stats["total_attempts"] = total
		stats["successful"] = successful
		if avgReduction.Valid {
			stats["avg_reduction_percent"] = avgReduction.Float64
		} else {
			stats["avg_reduction_percent"] = 0.0
		}

		return nil
	})

	return stats, err
}

// Helper: ensure quarantined_files table creation in schema
func (s *SQLiteStorage) createQuarantinedFilesTable() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS quarantined_files (
			id TEXT PRIMARY KEY,
			file_id TEXT NOT NULL,
			campaign_id TEXT NOT NULL,
			hash TEXT NOT NULL,
			reason TEXT NOT NULL,
			details TEXT,
			quarantined_at DATETIME NOT NULL,
			quarantined_by TEXT NOT NULL,
			reviewed_at DATETIME,
			reviewed_by TEXT,
			resolution TEXT
		)
	`)
	return err
}

// Helper: ensure corpus_file_metrics table creation in schema
func (s *SQLiteStorage) createCorpusFileMetricsTable() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS corpus_file_metrics (
			file_id TEXT PRIMARY KEY,
			crash_count INTEGER DEFAULT 0,
			timeout_count INTEGER DEFAULT 0,
			avg_exec_time_ns INTEGER DEFAULT 0,
			max_memory_usage INTEGER DEFAULT 0,
			last_executed DATETIME,
			exec_count INTEGER DEFAULT 0
		)
	`)
	return err
}

// Helper: ensure reproduction tables creation in schema
func (s *SQLiteStorage) createReproductionTables() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS reproduction_requests (
			id TEXT PRIMARY KEY,
			crash_id TEXT NOT NULL,
			status TEXT NOT NULL,
			priority INTEGER DEFAULT 0,
			created_at DATETIME NOT NULL,
			started_at DATETIME,
			completed_at DATETIME,
			attempts INTEGER DEFAULT 0,
			max_attempts INTEGER DEFAULT 3
		);

		CREATE TABLE IF NOT EXISTS minimization_results (
			id TEXT PRIMARY KEY,
			crash_id TEXT NOT NULL,
			strategy TEXT NOT NULL,
			original_size INTEGER NOT NULL,
			minimized_size INTEGER NOT NULL,
			minimized_hash TEXT,
			success BOOLEAN NOT NULL,
			timestamp DATETIME NOT NULL,
			duration_ms INTEGER
		);
	`)
	return err
}

// EnsureStorageInterfaceTables creates all tables needed for the Storage interface
func (s *SQLiteStorage) EnsureStorageInterfaceTables() error {
	if err := s.createQuarantinedFilesTable(); err != nil {
		return err
	}
	if err := s.createCorpusFileMetricsTable(); err != nil {
		return err
	}
	if err := s.createReproductionTables(); err != nil {
		return err
	}
	return nil
}

// GetCrashesByCampaign retrieves all crashes for a campaign
func (s *SQLiteStorage) GetCrashesByCampaign(ctx context.Context, campaignID string) ([]*common.CrashResult, error) {
	var crashes []*common.CrashResult

	err := ExecuteWithRetry(ctx, s.config, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, job_id, bot_id, hash, file_path, type,
				signal, exit_code, timestamp, size, is_unique, output, stack_trace
			FROM crashes
			WHERE campaign_id = ?
			ORDER BY timestamp DESC
		`, campaignID)
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

// Job Log operations

// StoreJobLogs stores job log entries
func (s *SQLiteStorage) StoreJobLogs(ctx context.Context, jobID string, logs []*common.JobLog) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO job_logs (job_id, level, source, message, timestamp, metadata)
			VALUES (?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, log := range logs {
			var metadataJSON *string
			if log.Metadata != nil {
				data, err := json.Marshal(log.Metadata)
				if err == nil {
					s := string(data)
					metadataJSON = &s
				}
			}

			_, err := stmt.ExecContext(ctx, jobID, log.Level, log.Source, log.Message, log.Timestamp, metadataJSON)
			if err != nil {
				return err
			}
		}

		return tx.Commit()
	})
}

// GetJobLogs retrieves job log entries with pagination
func (s *SQLiteStorage) GetJobLogs(ctx context.Context, jobID string, limit, offset int) ([]*common.JobLog, int, error) {
	var logs []*common.JobLog
	var total int

	err := ExecuteWithRetry(ctx, s.config, func() error {
		// Get total count
		err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM job_logs WHERE job_id = ?`, jobID).Scan(&total)
		if err != nil {
			return err
		}

		// Get logs with pagination
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, job_id, level, source, message, timestamp, metadata
			FROM job_logs
			WHERE job_id = ?
			ORDER BY timestamp ASC
			LIMIT ? OFFSET ?
		`, jobID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var log common.JobLog
			var source sql.NullString
			var metadataJSON sql.NullString

			err := rows.Scan(&log.ID, &log.JobID, &log.Level, &source, &log.Message, &log.Timestamp, &metadataJSON)
			if err != nil {
				return err
			}

			if source.Valid {
				log.Source = source.String
			}

			if metadataJSON.Valid && metadataJSON.String != "" {
				if err := json.Unmarshal([]byte(metadataJSON.String), &log.Metadata); err != nil {
					// Ignore JSON parse errors
				}
			}

			logs = append(logs, &log)
		}

		return rows.Err()
	})

	return logs, total, err
}
