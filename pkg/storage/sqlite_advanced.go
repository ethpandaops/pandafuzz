package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// BatchStore implements batch storage operations
func (s *SQLiteStorage) BatchStore(ctx context.Context, items map[string]any) error {
	return s.Transaction(ctx, func(tx common.Transaction) error {
		for key, value := range items {
			if err := tx.Store(ctx, key, value); err != nil {
				return err
			}
		}
		return nil
	})
}

// BatchDelete implements batch delete operations
func (s *SQLiteStorage) BatchDelete(ctx context.Context, keys []string) error {
	return s.Transaction(ctx, func(tx common.Transaction) error {
		for _, key := range keys {
			if err := tx.Delete(ctx, key); err != nil {
				return err
			}
		}
		return nil
	})
}

// Backup creates a backup of the database
func (s *SQLiteStorage) Backup(ctx context.Context, path string) error {
	// Validate backup path to prevent directory traversal
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		return fmt.Errorf("backup path must be absolute")
	}

	// Additional validation to prevent SQL injection
	if strings.ContainsAny(cleanPath, "';\"") {
		return fmt.Errorf("invalid characters in backup path")
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Use SQLite backup by copying the file directly while ensuring consistency
	return ExecuteWithRetry(ctx, s.config, func() error {
		// First, ensure all changes are written to disk
		if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			return fmt.Errorf("failed to checkpoint WAL: %w", err)
		}

		// Get the database file path
		var dbPath string
		err := s.db.QueryRowContext(ctx, "PRAGMA database_list").Scan(nil, nil, &dbPath)
		if err != nil {
			return fmt.Errorf("failed to get database path: %w", err)
		}

		// Copy the database file
		srcFile, err := os.Open(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open source database: %w", err)
		}
		defer srcFile.Close()

		destFile, err := os.Create(cleanPath)
		if err != nil {
			return fmt.Errorf("failed to create backup file: %w", err)
		}
		defer destFile.Close()

		// Copy the data
		if _, err := io.Copy(destFile, srcFile); err != nil {
			os.Remove(cleanPath) // Clean up on failure
			return fmt.Errorf("failed to copy database: %w", err)
		}

		// Also copy WAL and SHM files if they exist
		walPath := dbPath + "-wal"
		if _, err := os.Stat(walPath); err == nil {
			if err := copyFile(walPath, cleanPath+"-wal"); err != nil {
				s.logger.WithError(err).Warn("Failed to copy WAL file")
			}
		}

		shmPath := dbPath + "-shm"
		if _, err := os.Stat(shmPath); err == nil {
			if err := copyFile(shmPath, cleanPath+"-shm"); err != nil {
				s.logger.WithError(err).Warn("Failed to copy SHM file")
			}
		}

		s.logger.WithField("backup_path", cleanPath).Info("Database backup completed")
		return nil
	})
}

// copyFile is a helper function to copy files
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// Restore restores database from a backup
func (s *SQLiteStorage) Restore(ctx context.Context, path string) error {
	// Check if backup file exists
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	// Close current connection
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("failed to close current database: %w", err)
	}

	// Copy backup file to database path
	backupData, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	if err := os.WriteFile(s.path, backupData, 0644); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	// Reopen database
	connStr := s.path + "?cache=shared&mode=rwc&_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return fmt.Errorf("failed to reopen database: %w", err)
	}

	s.db = db
	s.logger.WithField("restore_path", path).Info("Database restore completed")
	return nil
}

// Vacuum optimizes the database
func (s *SQLiteStorage) Vacuum(ctx context.Context) error {
	_, err := RetryableExec(ctx, s.db, s.config, "VACUUM")
	return err
}

// Compact is an alias for Vacuum in SQLite
func (s *SQLiteStorage) Compact(ctx context.Context) error {
	return s.Vacuum(ctx)
}

// DeleteOldCrashes deletes crashes older than the specified time
func (s *SQLiteStorage) DeleteOldCrashes(ctx context.Context, before time.Time) error {
	s.logger.WithField("before", before).Info("Deleting old crashes")

	// First delete from crashes table
	query := `DELETE FROM crashes WHERE timestamp < ?`
	result, err := RetryableExec(ctx, s.db, s.config, query, before)
	if err != nil {
		return common.NewDatabaseError("delete_old_crashes", err)
	}

	s.logger.WithField("deleted", result).Info("Deleted old crash records")

	// The crash_inputs table has ON DELETE CASCADE, so entries are automatically removed
	return nil
}

// DeleteOldJobs deletes jobs older than the specified time
func (s *SQLiteStorage) DeleteOldJobs(ctx context.Context, before time.Time) error {
	s.logger.WithField("before", before).Info("Deleting old jobs")

	// Delete completed/failed/cancelled jobs older than the specified time
	query := `
		DELETE FROM jobs
		WHERE status IN ('completed', 'failed', 'cancelled', 'timed_out')
		AND (completed_at < ? OR (completed_at IS NULL AND created_at < ?))
	`
	result, err := RetryableExec(ctx, s.db, s.config, query, before, before)
	if err != nil {
		return common.NewDatabaseError("delete_old_jobs", err)
	}

	s.logger.WithField("deleted", result).Info("Deleted old job records")

	// Clean up orphaned records in related tables
	orphanQueries := map[string]string{
		"coverage":        "DELETE FROM coverage WHERE job_id NOT IN (SELECT id FROM jobs)",
		"corpus_updates":  "DELETE FROM corpus_updates WHERE job_id NOT IN (SELECT id FROM jobs)",
		"job_assignments": "DELETE FROM job_assignments WHERE job_id NOT IN (SELECT id FROM jobs)",
	}

	for table, cleanupQuery := range orphanQueries {
		orphaned, err := RetryableExec(ctx, s.db, s.config, cleanupQuery)
		if err != nil {
			s.logger.WithError(err).WithField("table", table).Warn("Failed to clean orphaned records")
		} else if rows, _ := orphaned.RowsAffected(); rows > 0 {
			s.logger.WithFields(logrus.Fields{
				"table": table,
				"count": rows,
			}).Info("Cleaned orphaned records")
		}
	}

	return nil
}

// GetDatabaseSize returns the size of the database in bytes
func (s *SQLiteStorage) GetDatabaseSize(ctx context.Context) (int64, error) {
	query := `SELECT page_count * page_size as size FROM pragma_page_count(), pragma_page_size()`

	return RetryableQueryRow(ctx, s.db, s.config, query, func(row *sql.Row) (int64, error) {
		var size int64
		err := row.Scan(&size)
		if err != nil {
			return 0, common.NewDatabaseError("get_database_size", err)
		}
		return size, nil
	})
}

// GetSystemStats returns system statistics
func (s *SQLiteStorage) GetSystemStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get bot statistics
	var totalBots, onlineBots int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), SUM(CASE WHEN is_online = 1 THEN 1 ELSE 0 END)
		FROM bots
	`).Scan(&totalBots, &onlineBots)
	if err != nil {
		return nil, err
	}
	stats["total_bots"] = totalBots
	stats["online_bots"] = onlineBots

	// Get job statistics
	var totalJobs, runningJobs, completedJobs int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
			SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END)
		FROM jobs
	`).Scan(&totalJobs, &runningJobs, &completedJobs)
	if err != nil {
		return nil, err
	}
	stats["total_jobs"] = totalJobs
	stats["running_jobs"] = runningJobs
	stats["completed_jobs"] = completedJobs

	// Get crash statistics
	var totalCrashes, uniqueCrashes int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COUNT(DISTINCT hash) FROM crashes
	`).Scan(&totalCrashes, &uniqueCrashes)
	if err != nil {
		return nil, err
	}
	stats["total_crashes"] = totalCrashes
	stats["unique_crashes"] = uniqueCrashes

	// Get database size
	size, err := s.GetDatabaseSize(ctx)
	if err == nil {
		stats["database_size"] = size
	}

	return stats, nil
}

// Cleanup performs database cleanup operations.
func (s *SQLiteStorage) Cleanup(ctx context.Context) error {
	// Default retention periods
	const (
		jobRetention   = 30 * 24 * time.Hour // 30 days for completed jobs
		crashRetention = 90 * 24 * time.Hour // 90 days for crash data
	)

	jobCutoff := time.Now().Add(-jobRetention).Format("2006-01-02 15:04:05")
	crashCutoff := time.Now().Add(-crashRetention).Format("2006-01-02 15:04:05")

	// Delete old completed jobs
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM jobs WHERE status IN ('completed', 'failed') AND completed_at < ?`,
		jobCutoff)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to cleanup old jobs")
	} else if rows, _ := result.RowsAffected(); rows > 0 {
		s.logger.WithField("deleted_jobs", rows).Info("Cleaned up old jobs")
	}

	// Delete old job assignments for deleted jobs
	_, err = s.db.ExecContext(ctx,
		`DELETE FROM job_assignments WHERE job_id NOT IN (SELECT id FROM jobs)`)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to cleanup orphan job assignments")
	}

	// Delete old crashes
	result, err = s.db.ExecContext(ctx,
		`DELETE FROM crashes WHERE discovered_at < ?`,
		crashCutoff)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to cleanup old crashes")
	} else if rows, _ := result.RowsAffected(); rows > 0 {
		s.logger.WithField("deleted_crashes", rows).Info("Cleaned up old crashes")
	}

	// Vacuum database to reclaim space
	if _, err = s.db.ExecContext(ctx, "VACUUM"); err != nil {
		s.logger.WithError(err).Warn("Failed to vacuum database")
	}

	return nil
}

// RecordCorpusUpdate records a corpus update
func (s *SQLiteStorage) RecordCorpusUpdate(ctx context.Context, update *common.CorpusUpdate) error {
	// Serialize files array to JSON
	filesJSON, err := json.Marshal(update.Files)
	if err != nil {
		return fmt.Errorf("failed to marshal files: %w", err)
	}

	return ExecuteWithRetry(ctx, s.config, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO corpus_updates (
				id, job_id, bot_id, files, timestamp, total_size
			) VALUES (?, ?, ?, ?, ?, ?)
		`, update.ID, update.JobID, update.BotID, string(filesJSON),
			update.Timestamp, update.TotalSize)
		return err
	})
}
