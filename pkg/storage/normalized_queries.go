package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// SaveBotWithCapabilities saves a bot with its capabilities in normalized tables
func (s *SQLiteStorage) SaveBotWithCapabilities(bot *common.Bot) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Save bot
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO bots (
			id, name, hostname, status, last_seen, registered_at, 
			current_job, timeout_at, is_online, failure_count, api_endpoint
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, bot.ID, bot.Name, bot.Hostname, bot.Status, bot.LastSeen, bot.RegisteredAt,
		bot.CurrentJob, bot.TimeoutAt, bot.IsOnline, bot.FailureCount, bot.APIEndpoint)
	if err != nil {
		return fmt.Errorf("failed to save bot: %w", err)
	}

	// Delete existing capabilities
	_, err = tx.Exec("DELETE FROM bot_capabilities WHERE bot_id = ?", bot.ID)
	if err != nil {
		return fmt.Errorf("failed to delete existing capabilities: %w", err)
	}

	// Insert new capabilities
	for _, capability := range bot.Capabilities {
		_, err = tx.Exec(`
			INSERT INTO bot_capabilities (bot_id, capability) VALUES (?, ?)
		`, bot.ID, capability)
		if err != nil {
			return fmt.Errorf("failed to insert capability: %w", err)
		}
	}

	return tx.Commit()
}

// GetBotWithCapabilities retrieves a bot with its capabilities from normalized tables
func (s *SQLiteStorage) GetBotWithCapabilities(botID string) (*common.Bot, error) {
	var bot common.Bot
	var currentJob sql.NullString

	// Get bot data
	err := s.db.QueryRow(`
		SELECT id, hostname, status, last_seen, registered_at,
			   current_job, timeout_at, is_online, failure_count
		FROM bots WHERE id = ?
	`, botID).Scan(
		&bot.ID, &bot.Hostname, &bot.Status, &bot.LastSeen, &bot.RegisteredAt,
		&currentJob, &bot.TimeoutAt, &bot.IsOnline, &bot.FailureCount,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, common.ErrKeyNotFound
		}
		return nil, err
	}

	if currentJob.Valid {
		bot.CurrentJob = &currentJob.String
	}

	// Get capabilities
	rows, err := s.db.Query(`
		SELECT capability FROM bot_capabilities WHERE bot_id = ?
	`, botID)
	if err != nil {
		return nil, fmt.Errorf("failed to get capabilities: %w", err)
	}
	defer rows.Close()

	var capabilities []string
	for rows.Next() {
		var capability string
		if err := rows.Scan(&capability); err != nil {
			return nil, err
		}
		capabilities = append(capabilities, capability)
	}
	bot.Capabilities = capabilities

	return &bot, nil
}

// SaveJobWithConfig saves a job with its configuration in normalized tables
func (s *SQLiteStorage) SaveJobWithConfig(job *common.Job) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Save job
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO jobs (
			id, name, target, fuzzer, status, assigned_bot,
			created_at, started_at, completed_at, timeout_at, work_dir
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, job.ID, job.Name, job.Target, job.Fuzzer, job.Status, job.AssignedBot,
		job.CreatedAt, job.StartedAt, job.CompletedAt, job.TimeoutAt, job.WorkDir)
	if err != nil {
		return fmt.Errorf("failed to save job: %w", err)
	}

	// Save job config if present
	if job.Config.MemoryLimit > 0 || job.Config.Timeout > 0 {
		seedCorpusJSON, _ := json.Marshal(job.Config.SeedCorpus)

		_, err = tx.Exec(`
			INSERT OR REPLACE INTO job_configs (
				job_id, memory_limit, timeout_seconds, 
				dictionary_path, seed_corpus
			) VALUES (?, ?, ?, ?, ?)
		`, job.ID, job.Config.MemoryLimit, int(job.Config.Timeout.Seconds()),
			job.Config.Dictionary, string(seedCorpusJSON))
		if err != nil {
			return fmt.Errorf("failed to save job config: %w", err)
		}
	}

	return tx.Commit()
}

// SaveCorpusUpdateWithFiles saves a corpus update with individual file entries
func (s *SQLiteStorage) SaveCorpusUpdateWithFiles(update *common.CorpusUpdate) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Save corpus update
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO corpus_updates (
			id, job_id, bot_id, timestamp, total_size
		) VALUES (?, ?, ?, ?, ?)
	`, update.ID, update.JobID, update.BotID, update.Timestamp, update.TotalSize)
	if err != nil {
		return fmt.Errorf("failed to save corpus update: %w", err)
	}

	// Save individual files (for now just store the paths)
	// In a real implementation, you would parse file metadata
	for _, filePath := range update.Files {
		_, err = tx.Exec(`
			INSERT INTO corpus_files (
				corpus_update_id, file_path, file_size, file_hash
			) VALUES (?, ?, ?, ?)
		`, update.ID, filePath, 0, "") // Size and hash would be calculated
		if err != nil {
			return fmt.Errorf("failed to save corpus file: %w", err)
		}
	}

	return tx.Commit()
}

// GetJobStats returns aggregated statistics for a job
func (s *SQLiteStorage) GetJobStats(jobID string) (*JobStatistics, error) {
	var stats JobStatistics

	// Get crash statistics
	err := s.db.QueryRow(`
		SELECT COUNT(*), COUNT(DISTINCT hash) 
		FROM crashes WHERE job_id = ?
	`, jobID).Scan(&stats.TotalCrashes, &stats.UniqueCrashes)
	if err != nil {
		return nil, err
	}

	// Get coverage statistics
	err = s.db.QueryRow(`
		SELECT MAX(edges), SUM(new_edges), MAX(exec_count)
		FROM coverage WHERE job_id = ?
	`, jobID).Scan(&stats.MaxEdges, &stats.TotalNewEdges, &stats.MaxExecCount)
	if err != nil {
		return nil, err
	}

	// Get corpus statistics
	err = s.db.QueryRow(`
		SELECT COUNT(*), SUM(file_size)
		FROM corpus_files cf
		JOIN corpus_updates cu ON cf.corpus_update_id = cu.id
		WHERE cu.job_id = ?
	`, jobID).Scan(&stats.CorpusFiles, &stats.CorpusSize)
	if err != nil {
		return nil, err
	}

	return &stats, nil
}

// JobStatistics holds aggregated job statistics
type JobStatistics struct {
	TotalCrashes  int64
	UniqueCrashes int64
	MaxEdges      int64
	TotalNewEdges int64
	MaxExecCount  int64
	CorpusFiles   int64
	CorpusSize    int64
}