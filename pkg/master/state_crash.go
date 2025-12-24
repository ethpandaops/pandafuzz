package master

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Crash and result processing operations

// ProcessCrashResultWithRetry processes and stores a crash result with retry logic
func (ps *PersistentState) ProcessCrashResultWithRetry(ctx context.Context, crash *common.CrashResult) error {
	// Log incoming crash for processing
	ps.logger.WithFields(logrus.Fields{
		"crash_id":  crash.ID,
		"job_id":    crash.JobID,
		"bot_id":    crash.BotID,
		"hash":      crash.Hash,
		"type":      crash.Type,
		"size":      crash.Size,
		"timestamp": crash.Timestamp,
	}).Info("Processing crash result from bot")

	return ps.retryManager.Execute(func() error {
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Generate crash ID if not provided
			if crash.ID == "" {
				crash.ID = uuid.New().String()
				ps.logger.WithFields(logrus.Fields{
					"crash_id": crash.ID,
					"job_id":   crash.JobID,
				}).Debug("Generated new crash ID")
			}

			// Check for duplicates based on hash
			duplicate, err := ps.checkCrashDuplicateTx(ctx, tx, crash.Hash)
			if err != nil {
				return err
			}

			crash.IsUnique = !duplicate

			if duplicate {
				ps.logger.WithFields(logrus.Fields{
					"crash_hash": crash.Hash,
					"crash_id":   crash.ID,
					"job_id":     crash.JobID,
				}).Info("Crash is a duplicate of existing crash")
			} else {
				ps.logger.WithFields(logrus.Fields{
					"crash_hash": crash.Hash,
					"crash_id":   crash.ID,
					"job_id":     crash.JobID,
				}).Info("Crash is unique")
			}

			// Store crash input separately if provided
			hasInput := len(crash.Input) > 0

			// Log crash input status
			if hasInput {
				ps.logger.WithFields(logrus.Fields{
					"crash_id":   crash.ID,
					"input_size": len(crash.Input),
				}).Info("Received crash with input data")
			} else if crash.InputBase64 != "" {
				ps.logger.WithFields(logrus.Fields{
					"crash_id": crash.ID,
				}).Info("Received crash with base64 input")
			} else {
				ps.logger.WithFields(logrus.Fields{
					"crash_id": crash.ID,
				}).Warn("WARNING: Crash received without input data")
			}

			if hasInput {
				// Check if we're using SQLiteStorage
				if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
					// Store outside transaction for SQLite (it has its own locking)
					if err := sqliteDB.StoreCrashInput(ctx, crash.ID, crash.Input); err != nil {
						return common.NewDatabaseError("save_crash_input", err)
					}
				} else {
					// Fallback for other databases
					if err := tx.Store(ctx, "crash_input:"+crash.ID, crash.Input); err != nil {
						return common.NewDatabaseError("save_crash_input", err)
					}
				}
				// Clear the input from the crash object to avoid storing it twice
				crash.Input = nil
			}

			// Store crash result in the crashes table (not as key-value object)
			// This ensures ListCrashes can find the crashes properly
			if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
				if err := sqliteDB.CreateCrash(ctx, crash); err != nil {
					return common.NewDatabaseError("save_crash", err)
				}
			} else {
				// Fallback for other databases - use key-value store
				if err := tx.Store(ctx, "crash:"+crash.ID, crash); err != nil {
					return common.NewDatabaseError("save_crash", err)
				}
			}

			ps.mu.Lock()
			ps.stats.CrashesRecorded++
			ps.stats.TransactionCount++
			ps.mu.Unlock()

			ps.logger.WithFields(logrus.Fields{
				"crash_id":  crash.ID,
				"job_id":    crash.JobID,
				"bot_id":    crash.BotID,
				"hash":      crash.Hash,
				"is_unique": crash.IsUnique,
				"type":      crash.Type,
				"signal":    crash.Signal,
				"exit_code": crash.ExitCode,
				"size":      crash.Size,
				"file_path": crash.FilePath,
				"has_input": hasInput,
				"timestamp": crash.Timestamp,
			}).Info("Crash result successfully processed and stored")

			return nil
		})
	})
}

// ProcessCoverageResultWithRetry processes and stores a coverage result with retry logic
func (ps *PersistentState) ProcessCoverageResultWithRetry(ctx context.Context, coverage *common.CoverageResult) error {
	return ps.retryManager.Execute(func() error {
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Generate coverage ID if not provided
			if coverage.ID == "" {
				coverage.ID = uuid.New().String()
			}

			// Store coverage result
			if err := tx.Store(ctx, "coverage:"+coverage.ID, coverage); err != nil {
				return common.NewDatabaseError("save_coverage", err)
			}

			ps.mu.Lock()
			ps.stats.CoverageReports++
			ps.stats.TransactionCount++
			ps.mu.Unlock()

			ps.logger.WithFields(logrus.Fields{
				"coverage_id": coverage.ID,
				"job_id":      coverage.JobID,
				"bot_id":      coverage.BotID,
				"edges":       coverage.Edges,
				"new_edges":   coverage.NewEdges,
				"exec_count":  coverage.ExecCount,
			}).Debug("Coverage result processed")

			return nil
		})
	})
}

// ProcessCorpusUpdateWithRetry processes and stores a corpus update with retry logic
func (ps *PersistentState) ProcessCorpusUpdateWithRetry(ctx context.Context, corpus *common.CorpusUpdate) error {
	return ps.retryManager.Execute(func() error {
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Generate corpus ID if not provided
			if corpus.ID == "" {
				corpus.ID = uuid.New().String()
			}

			// Store corpus update
			if err := tx.Store(ctx, "corpus:"+corpus.ID, corpus); err != nil {
				return common.NewDatabaseError("save_corpus", err)
			}

			ps.mu.Lock()
			ps.stats.CorpusUpdates++
			ps.stats.TransactionCount++
			ps.mu.Unlock()

			ps.logger.WithFields(logrus.Fields{
				"corpus_id":  corpus.ID,
				"job_id":     corpus.JobID,
				"bot_id":     corpus.BotID,
				"file_count": len(corpus.Files),
				"total_size": corpus.TotalSize,
			}).Debug("Corpus update processed")

			return nil
		})
	})
}

// checkCrashDuplicateTx checks if a crash with the given hash already exists
func (ps *PersistentState) checkCrashDuplicateTx(ctx context.Context, tx common.Transaction, hash string) (bool, error) {
	// This is a simplified implementation
	// In a real implementation, you would query existing crashes by hash
	// For now, we'll assume it's unique
	return false, nil
}

// Crash retrieval operations

// GetCrashes retrieves crashes with pagination
func (ps *PersistentState) GetCrashes(ctx context.Context, limit, offset int) ([]*common.CrashResult, error) {
	// Don't hold any locks - database has its own concurrency control
	// Check if the database is SQLiteStorage and use its optimized methods
	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		return sqliteDB.GetCrashes(ctx, limit, offset)
	}

	// Fallback for other database implementations that support AdvancedDatabase
	if advDB, ok := ps.db.(common.AdvancedDatabase); ok {
		crashes := make([]*common.CrashResult, 0, limit)
		count := 0
		skipped := 0

		err := advDB.Iterate(ctx, "crash:", func(key string, value []byte) error {
			// Skip until we reach the offset
			if skipped < offset {
				skipped++
				return nil
			}

			// Stop when we've collected enough
			if count >= limit {
				return fmt.Errorf("limit reached") // Stop iteration
			}

			var crash common.CrashResult
			if err := json.Unmarshal(value, &crash); err != nil {
				ps.logger.WithError(err).WithField("key", key).Warn("Failed to unmarshal crash")
				return nil // Continue with next crash
			}

			crashes = append(crashes, &crash)
			count++
			return nil
		})

		if err != nil && err.Error() != "limit reached" {
			return nil, err
		}

		return crashes, nil
	}

	// Basic database fallback - not efficient but functional
	return nil, fmt.Errorf("database does not support efficient crash listing")
}

// GetCrashesSorted retrieves crashes with sorting support
func (ps *PersistentState) GetCrashesSorted(ctx context.Context, limit, offset int, sortBy, sortOrder string) ([]*common.CrashResult, error) {
	// Don't hold any locks - database has its own concurrency control
	// Check if the database is SQLiteStorage and use its optimized methods
	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		return sqliteDB.GetCrashesSorted(ctx, limit, offset, sortBy, sortOrder)
	}

	// Fallback to unsorted for other database implementations
	// For now, just use the regular GetCrashes method
	return ps.GetCrashes(ctx, limit, offset)
}

// GetCrash retrieves a specific crash by ID
func (ps *PersistentState) GetCrash(ctx context.Context, crashID string) (*common.CrashResult, error) {
	// Don't hold any locks - database has its own concurrency control
	// Check if the database is SQLiteStorage and use its optimized methods
	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		return sqliteDB.GetCrash(ctx, crashID)
	}

	// Fallback for other database implementations
	var crash common.CrashResult
	err := ps.db.Get(ctx, "crash:"+crashID, &crash)

	if err != nil {
		if err == common.ErrKeyNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &crash, nil
}

// GetJobCrashes retrieves all crashes for a specific job
func (ps *PersistentState) GetJobCrashes(ctx context.Context, jobID string) ([]*common.CrashResult, error) {
	// Don't hold any locks - database has its own concurrency control
	// Check if the database is SQLiteStorage and use its optimized methods
	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		return sqliteDB.GetJobCrashes(ctx, jobID)
	}

	// Fallback for other database implementations that support AdvancedDatabase
	if advDB, ok := ps.db.(common.AdvancedDatabase); ok {
		crashes := make([]*common.CrashResult, 0)

		err := advDB.Iterate(ctx, "crash:", func(key string, value []byte) error {
			var crash common.CrashResult
			if err := json.Unmarshal(value, &crash); err != nil {
				ps.logger.WithError(err).WithField("key", key).Warn("Failed to unmarshal crash")
				return nil // Continue with next crash
			}

			if crash.JobID == jobID {
				crashes = append(crashes, &crash)
			}
			return nil
		})

		if err != nil {
			return nil, err
		}

		return crashes, nil
	}

	// Basic database fallback - not efficient but functional
	return nil, fmt.Errorf("database does not support efficient job crash listing")
}

// GetCrashInput retrieves the input data for a specific crash
func (ps *PersistentState) GetCrashInput(ctx context.Context, crashID string) ([]byte, error) {
	// Don't hold any locks - database has its own concurrency control
	// Check if the database is SQLiteStorage and use its optimized methods
	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		return sqliteDB.GetCrashInput(ctx, crashID)
	}

	// Fallback for other database implementations
	var input []byte
	err := ps.db.Get(ctx, "crash_input:"+crashID, &input)

	if err != nil {
		if err == common.ErrKeyNotFound {
			return nil, nil
		}
		return nil, err
	}

	return input, nil
}

// Crash analytics operations

// GetJobCrashesInTimeRange retrieves crashes for a job within a time range
func (ps *PersistentState) GetJobCrashesInTimeRange(ctx context.Context, jobID string, startTime, endTime time.Time) ([]*common.CrashResult, error) {
	crashes, err := ps.GetJobCrashes(ctx, jobID)
	if err != nil {
		return nil, err
	}

	// Filter by time range
	var filtered []*common.CrashResult
	for _, crash := range crashes {
		if crash.Timestamp.After(startTime) && crash.Timestamp.Before(endTime) {
			filtered = append(filtered, crash)
		}
	}

	return filtered, nil
}

// GetCampaignCrashesInTimeRange retrieves crashes for a campaign within a time range
func (ps *PersistentState) GetCampaignCrashesInTimeRange(ctx context.Context, campaignID string, startTime, endTime time.Time) ([]*common.CrashResult, error) {
	var crashes []*common.CrashResult

	// Get all jobs for this campaign
	jobs, err := ps.GetCampaignJobs(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	// Get crashes for each job
	for _, job := range jobs {
		jobCrashes, err := ps.GetJobCrashesInTimeRange(ctx, job.ID, startTime, endTime)
		if err != nil {
			ps.logger.WithError(err).WithField("job_id", job.ID).Warn("Failed to get job crashes")
			continue
		}
		crashes = append(crashes, jobCrashes...)
	}

	return crashes, nil
}

// GetCrashesInTimeRange retrieves all crashes within a time range
func (ps *PersistentState) GetCrashesInTimeRange(ctx context.Context, startTime, endTime time.Time) ([]*common.CrashResult, error) {
	// Check if database supports advanced operations
	advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
	if !isAdvanced {
		return nil, fmt.Errorf("database doesn't support crash time range query")
	}

	query := `
		SELECT id, job_id, bot_id, hash, file_path, type, signal, exit_code,
		       timestamp, size, is_unique, output, stack_trace
		FROM crashes
		WHERE timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp DESC
	`

	rows, err := advDB.Select(ctx, query, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query crashes in time range: %w", err)
	}

	results := make([]*common.CrashResult, 0, len(rows))
	for _, row := range rows {
		crash := &common.CrashResult{}

		if id, ok := row["id"].(string); ok {
			crash.ID = id
		}
		if jobID, ok := row["job_id"].(string); ok {
			crash.JobID = jobID
		}
		if botID, ok := row["bot_id"].(string); ok {
			crash.BotID = botID
		}
		if hash, ok := row["hash"].(string); ok {
			crash.Hash = hash
		}
		if filePath, ok := row["file_path"].(string); ok {
			crash.FilePath = filePath
		}
		if crashType, ok := row["type"].(string); ok {
			crash.Type = crashType
		}
		if signal, ok := row["signal"].(int64); ok {
			crash.Signal = int(signal)
		}
		if exitCode, ok := row["exit_code"].(int64); ok {
			crash.ExitCode = int(exitCode)
		}
		if ts, ok := row["timestamp"].(time.Time); ok {
			crash.Timestamp = ts
		}
		if size, ok := row["size"].(int64); ok {
			crash.Size = size
		}
		if isUnique, ok := row["is_unique"].(bool); ok {
			crash.IsUnique = isUnique
		} else if isUnique, ok := row["is_unique"].(int64); ok {
			crash.IsUnique = isUnique != 0
		}
		if output, ok := row["output"].(string); ok {
			crash.Output = output
		}
		if stackTrace, ok := row["stack_trace"].(string); ok {
			crash.StackTrace = stackTrace
		}

		results = append(results, crash)
	}

	return results, nil
}
