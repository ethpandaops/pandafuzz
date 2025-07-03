package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// AddCorpusFile adds a new corpus file to the campaign
func (s *SQLiteStorage) AddCorpusFile(ctx context.Context, cf *common.CorpusFile) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO campaign_corpus_files (
				id, campaign_id, job_id, bot_id, filename, hash, size,
				coverage, new_coverage, parent_hash, generation, created_at, is_seed
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, cf.ID, cf.CampaignID, cf.JobID, cf.BotID, cf.Filename, cf.Hash, cf.Size,
			cf.Coverage, cf.NewCoverage, cf.ParentHash, cf.Generation, cf.CreatedAt, cf.IsSeed)
		return err
	})
}

// GetCorpusFiles retrieves all corpus files for a campaign
func (s *SQLiteStorage) GetCorpusFiles(ctx context.Context, campaignID string) ([]*common.CorpusFile, error) {
	var files []*common.CorpusFile

	err := ExecuteWithRetry(ctx, s.config, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, campaign_id, job_id, bot_id, filename, hash, size,
				coverage, new_coverage, parent_hash, generation, created_at, synced_at, is_seed
			FROM campaign_corpus_files
			WHERE campaign_id = ?
			ORDER BY created_at DESC
		`, campaignID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			cf := &common.CorpusFile{}
			var syncedAt sql.NullTime

			err := rows.Scan(
				&cf.ID, &cf.CampaignID, &cf.JobID, &cf.BotID, &cf.Filename, &cf.Hash, &cf.Size,
				&cf.Coverage, &cf.NewCoverage, &cf.ParentHash, &cf.Generation,
				&cf.CreatedAt, &syncedAt, &cf.IsSeed)
			if err != nil {
				return err
			}

			if syncedAt.Valid {
				cf.SyncedAt = &syncedAt.Time
			}

			files = append(files, cf)
		}

		return rows.Err()
	})

	return files, err
}

// GetCorpusFileByHash retrieves a corpus file by its hash
func (s *SQLiteStorage) GetCorpusFileByHash(ctx context.Context, hash string) (*common.CorpusFile, error) {
	cf := &common.CorpusFile{}
	var syncedAt sql.NullTime

	err := ExecuteWithRetry(ctx, s.config, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT id, campaign_id, job_id, bot_id, filename, hash, size,
				coverage, new_coverage, parent_hash, generation, created_at, synced_at, is_seed
			FROM campaign_corpus_files
			WHERE hash = ?
		`, hash).Scan(
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

// UpdateCorpusCoverage updates the coverage information for a corpus file
func (s *SQLiteStorage) UpdateCorpusCoverage(ctx context.Context, id string, coverage, newCoverage int64) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		result, err := s.db.ExecContext(ctx, `
			UPDATE campaign_corpus_files
			SET coverage = ?, new_coverage = ?
			WHERE id = ?
		`, coverage, newCoverage, id)
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

// RecordCorpusEvolution records a snapshot of corpus evolution
func (s *SQLiteStorage) RecordCorpusEvolution(ctx context.Context, ce *common.CorpusEvolution) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO corpus_evolution (
				campaign_id, timestamp, total_files, total_size,
				total_coverage, new_files, new_coverage
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`, ce.CampaignID, ce.Timestamp, ce.TotalFiles, ce.TotalSize,
			ce.TotalCoverage, ce.NewFiles, ce.NewCoverage)
		return err
	})
}

// GetCorpusEvolution retrieves corpus evolution history for a campaign
func (s *SQLiteStorage) GetCorpusEvolution(ctx context.Context, campaignID string, limit int) ([]*common.CorpusEvolution, error) {
	var evolution []*common.CorpusEvolution

	query := `
		SELECT campaign_id, timestamp, total_files, total_size,
			total_coverage, new_files, new_coverage
		FROM corpus_evolution
		WHERE campaign_id = ?
		ORDER BY timestamp DESC
	`
	args := []interface{}{campaignID}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	err := ExecuteWithRetry(ctx, s.config, func() error {
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			ce := &common.CorpusEvolution{}
			err := rows.Scan(
				&ce.CampaignID, &ce.Timestamp, &ce.TotalFiles, &ce.TotalSize,
				&ce.TotalCoverage, &ce.NewFiles, &ce.NewCoverage)
			if err != nil {
				return err
			}
			evolution = append(evolution, ce)
		}

		return rows.Err()
	})

	return evolution, err
}

// GetUnsyncedCorpusFiles retrieves corpus files that haven't been synced to a specific bot
func (s *SQLiteStorage) GetUnsyncedCorpusFiles(ctx context.Context, campaignID, botID string) ([]*common.CorpusFile, error) {
	var files []*common.CorpusFile

	// First, get the last sync time for this bot (if any)
	var lastSyncTime sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT MAX(synced_at)
		FROM campaign_corpus_files
		WHERE campaign_id = ? AND bot_id = ?
	`, campaignID, botID).Scan(&lastSyncTime)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// Get files that are newer than the last sync or have never been synced to this bot
	query := `
		SELECT DISTINCT cf.id, cf.campaign_id, cf.job_id, cf.bot_id, cf.filename, 
			cf.hash, cf.size, cf.coverage, cf.new_coverage, cf.parent_hash, 
			cf.generation, cf.created_at, cf.synced_at, cf.is_seed
		FROM campaign_corpus_files cf
		WHERE cf.campaign_id = ?
		AND cf.new_coverage > 0
	`
	args := []interface{}{campaignID}

	if lastSyncTime.Valid {
		query += " AND cf.created_at > ?"
		args = append(args, lastSyncTime.Time)
	}

	query += ` AND NOT EXISTS (
		SELECT 1 FROM campaign_corpus_files sync
		WHERE sync.hash = cf.hash
		AND sync.bot_id = ?
		AND sync.synced_at IS NOT NULL
	)`
	args = append(args, botID)

	query += " ORDER BY cf.new_coverage DESC, cf.created_at ASC LIMIT 100"

	err = ExecuteWithRetry(ctx, s.config, func() error {
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			cf := &common.CorpusFile{}
			var syncedAt sql.NullTime

			err := rows.Scan(
				&cf.ID, &cf.CampaignID, &cf.JobID, &cf.BotID, &cf.Filename, &cf.Hash, &cf.Size,
				&cf.Coverage, &cf.NewCoverage, &cf.ParentHash, &cf.Generation,
				&cf.CreatedAt, &syncedAt, &cf.IsSeed)
			if err != nil {
				return err
			}

			if syncedAt.Valid {
				cf.SyncedAt = &syncedAt.Time
			}

			files = append(files, cf)
		}

		return rows.Err()
	})

	return files, err
}

// MarkCorpusFilesSynced marks corpus files as synced to a specific bot
func (s *SQLiteStorage) MarkCorpusFilesSynced(ctx context.Context, fileIDs []string, botID string) error {
	if len(fileIDs) == 0 {
		return nil
	}

	return ExecuteWithRetry(ctx, s.config, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		syncTime := time.Now()

		// Create sync records for each file-bot combination
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO campaign_corpus_files (
				id, campaign_id, job_id, bot_id, filename, hash, size,
				coverage, new_coverage, parent_hash, generation, created_at, synced_at, is_seed
			)
			SELECT 
				id || '_' || ? as id,
				campaign_id, job_id, ?, filename, hash, size,
				coverage, new_coverage, parent_hash, generation, created_at, ?, is_seed
			FROM campaign_corpus_files
			WHERE id = ?
			ON CONFLICT(id) DO UPDATE SET synced_at = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, fileID := range fileIDs {
			_, err := stmt.ExecContext(ctx, botID, botID, syncTime, fileID, syncTime)
			if err != nil {
				return err
			}
		}

		return tx.Commit()
	})
}

// GetCorpusStats retrieves statistics about the corpus for a campaign
func (s *SQLiteStorage) GetCorpusStats(ctx context.Context, campaignID string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	err := ExecuteWithRetry(ctx, s.config, func() error {
		// Basic corpus statistics
		row := s.db.QueryRowContext(ctx, `
			SELECT 
				COUNT(*) as total_files,
				COALESCE(SUM(size), 0) as total_size,
				COALESCE(MAX(coverage), 0) as max_coverage,
				COALESCE(SUM(new_coverage), 0) as total_new_coverage,
				COUNT(DISTINCT parent_hash) as unique_parents,
				MAX(generation) as max_generation
			FROM campaign_corpus_files
			WHERE campaign_id = ?
		`, campaignID)

		var totalFiles, totalSize, maxCoverage, totalNewCoverage, uniqueParents, maxGeneration int64
		err := row.Scan(&totalFiles, &totalSize, &maxCoverage, &totalNewCoverage, &uniqueParents, &maxGeneration)
		if err != nil {
			return err
		}

		stats["total_files"] = totalFiles
		stats["total_size"] = totalSize
		stats["max_coverage"] = maxCoverage
		stats["total_new_coverage"] = totalNewCoverage
		stats["unique_parents"] = uniqueParents
		stats["max_generation"] = maxGeneration

		// Files by generation
		rows, err := s.db.QueryContext(ctx, `
			SELECT generation, COUNT(*) as count
			FROM campaign_corpus_files
			WHERE campaign_id = ?
			GROUP BY generation
			ORDER BY generation
		`, campaignID)
		if err != nil {
			return err
		}
		defer rows.Close()

		generations := make(map[int]int)
		for rows.Next() {
			var gen, count int
			if err := rows.Scan(&gen, &count); err != nil {
				return err
			}
			generations[gen] = count
		}
		stats["files_by_generation"] = generations

		// Coverage increase over time (last 10 snapshots)
		var coverageHistory []map[string]interface{}
		historyRows, err := s.db.QueryContext(ctx, `
			SELECT timestamp, total_coverage, new_coverage
			FROM corpus_evolution
			WHERE campaign_id = ?
			ORDER BY timestamp DESC
			LIMIT 10
		`, campaignID)
		if err != nil {
			return err
		}
		defer historyRows.Close()

		for historyRows.Next() {
			var timestamp time.Time
			var totalCoverage, newCoverage int64
			if err := historyRows.Scan(&timestamp, &totalCoverage, &newCoverage); err != nil {
				return err
			}
			coverageHistory = append(coverageHistory, map[string]interface{}{
				"timestamp":      timestamp,
				"total_coverage": totalCoverage,
				"new_coverage":   newCoverage,
			})
		}
		stats["coverage_history"] = coverageHistory

		return nil
	})

	return stats, err
}

// GetCoverageIncreasingFiles finds corpus files that contributed new coverage
func (s *SQLiteStorage) GetCoverageIncreasingFiles(ctx context.Context, campaignID string) ([]*common.CorpusFile, error) {
	var files []*common.CorpusFile

	err := ExecuteWithRetry(ctx, s.config, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, campaign_id, job_id, bot_id, filename, hash, size,
				coverage, new_coverage, parent_hash, generation, created_at, synced_at, is_seed
			FROM campaign_corpus_files
			WHERE campaign_id = ? AND new_coverage > 0
			ORDER BY new_coverage DESC, created_at ASC
		`, campaignID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			cf := &common.CorpusFile{}
			var syncedAt sql.NullTime

			err := rows.Scan(
				&cf.ID, &cf.CampaignID, &cf.JobID, &cf.BotID, &cf.Filename, &cf.Hash, &cf.Size,
				&cf.Coverage, &cf.NewCoverage, &cf.ParentHash, &cf.Generation,
				&cf.CreatedAt, &syncedAt, &cf.IsSeed)
			if err != nil {
				return err
			}

			if syncedAt.Valid {
				cf.SyncedAt = &syncedAt.Time
			}

			files = append(files, cf)
		}

		return rows.Err()
	})

	return files, err
}
