package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// LinkJobToCampaignWithCorpus links a job to a campaign with optional corpus inheritance
func (s *SQLiteStorage) LinkJobToCampaignWithCorpus(ctx context.Context, jobID, campaignID string, useCampaignCorpus bool) error {
	return s.Transaction(ctx, func(tx common.Transaction) error {
		// Update job with campaign ID and corpus usage flag
		query := `UPDATE jobs SET
			campaign_id = ?,
			use_campaign_corpus = ?,
			updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`

		var useCorpusInt int
		if useCampaignCorpus {
			useCorpusInt = 1
		} else {
			useCorpusInt = 0
		}

		sqlTx := tx.(*SQLiteTransaction).tx
		result, err := sqlTx.ExecContext(ctx, query, campaignID, useCorpusInt, jobID)
		if err != nil {
			return common.NewDatabaseError("update_job_campaign", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return common.NewDatabaseError("check_rows_affected", err)
		}

		if rowsAffected == 0 {
			return common.ErrKeyNotFound
		}

		// Insert into campaign_jobs relationship table
		_, err = sqlTx.ExecContext(ctx, `
			INSERT OR IGNORE INTO campaign_jobs (campaign_id, job_id)
			VALUES (?, ?)`, campaignID, jobID)

		if err != nil {
			return common.NewDatabaseError("insert_campaign_job", err)
		}

		s.logger.WithFields(logrus.Fields{
			"job_id":              jobID,
			"campaign_id":         campaignID,
			"use_campaign_corpus": useCampaignCorpus,
		}).Info("Linked job to campaign")

		return nil
	})
}

// GetCampaignCorpusForJob retrieves corpus files for a job from its campaign
func (s *SQLiteStorage) GetCampaignCorpusForJob(ctx context.Context, jobID string) ([]*common.CorpusFile, error) {
	// First get the campaign ID and check if job uses campaign corpus
	type jobCampaignInfo struct {
		campaignID        sql.NullString
		useCampaignCorpus int
	}

	info, err := RetryableQueryRow(ctx, s.db, s.config,
		`SELECT campaign_id, use_campaign_corpus FROM jobs WHERE id = ?`,
		func(row *sql.Row) (jobCampaignInfo, error) {
			var info jobCampaignInfo
			err := row.Scan(&info.campaignID, &info.useCampaignCorpus)
			return info, err
		}, jobID)

	if err == sql.ErrNoRows {
		return nil, common.ErrKeyNotFound
	}
	if err != nil {
		return nil, common.NewDatabaseError("get_job_campaign", err)
	}

	campaignID := info.campaignID
	useCampaignCorpus := info.useCampaignCorpus

	// If job doesn't use campaign corpus or has no campaign, return empty
	if !campaignID.Valid || useCampaignCorpus == 0 {
		return []*common.CorpusFile{}, nil
	}

	// Get corpus files from the campaign
	query := `SELECT
		id, campaign_id, job_id, bot_id, filename, hash, size,
		coverage, new_coverage, parent_hash, generation, created_at,
		synced_at, is_seed
		FROM campaign_corpus_files
		WHERE campaign_id = ?
		ORDER BY created_at DESC`

	return RetryableQuery(ctx, s.db, s.config, query, func(rows *sql.Rows) (*common.CorpusFile, error) {
		file := &common.CorpusFile{}
		var jobID, botID, parentHash sql.NullString
		var syncedAt sql.NullTime

		err := rows.Scan(
			&file.ID, &file.CampaignID, &jobID, &botID, &file.Filename,
			&file.Hash, &file.Size, &file.Coverage, &file.NewCoverage,
			&parentHash, &file.Generation, &file.CreatedAt, &syncedAt,
			&file.IsSeed)

		if err != nil {
			return nil, err
		}

		// Handle nullable fields
		if jobID.Valid {
			file.JobID = jobID.String
		}
		if botID.Valid {
			file.BotID = botID.String
		}
		if parentHash.Valid {
			file.ParentHash = parentHash.String
		}
		if syncedAt.Valid {
			file.SyncedAt = &syncedAt.Time
		}

		return file, nil
	}, campaignID.String)
}

// PromoteCrashToCorpus promotes a crash input to campaign corpus
func (s *SQLiteStorage) PromoteCrashToCorpus(ctx context.Context, crashID, campaignID string, coverage int64) error {
	return s.Transaction(ctx, func(tx common.Transaction) error {
		sqlTx := tx.(*SQLiteTransaction).tx

		// Get crash details
		var crash common.CrashResult
		var jobID, botID sql.NullString

		err := sqlTx.QueryRowContext(ctx, `
			SELECT id, job_id, bot_id, hash, file_path, size
			FROM crashes WHERE id = ?`, crashID).Scan(
			&crash.ID, &jobID, &botID, &crash.Hash, &crash.FilePath, &crash.Size)

		if err == sql.ErrNoRows {
			return common.ErrKeyNotFound
		}
		if err != nil {
			return common.NewDatabaseError("get_crash_for_promotion", err)
		}

		// Get crash input
		input, err := s.GetCrashInput(ctx, crashID)
		if err != nil {
			return common.NewDatabaseError("get_crash_input_for_promotion", err)
		}

		// Create corpus file entry
		corpusID := fmt.Sprintf("corpus_%s_%d", crashID, time.Now().Unix())
		filename := fmt.Sprintf("crash_%s_%s", crash.Hash[:8], filepath.Base(crash.FilePath))

		_, err = sqlTx.ExecContext(ctx, `
			INSERT INTO campaign_corpus_files (
				id, campaign_id, job_id, bot_id, filename, hash, size,
				coverage, new_coverage, source_type, source_crash_id,
				generation, is_seed
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			corpusID, campaignID, jobID, botID, filename, crash.Hash,
			crash.Size, coverage, coverage, "crash_promotion", crashID,
			1, 0) // generation=1, is_seed=false

		if err != nil {
			return common.NewDatabaseError("create_corpus_file", err)
		}

		// Store the actual input data
		inputKey := fmt.Sprintf("corpus_input:%s", corpusID)
		if err := tx.Store(ctx, inputKey, input); err != nil {
			return common.NewDatabaseError("store_corpus_input", err)
		}

		s.logger.WithFields(logrus.Fields{
			"crash_id":    crashID,
			"campaign_id": campaignID,
			"corpus_id":   corpusID,
			"coverage":    coverage,
		}).Info("Promoted crash to corpus")

		return nil
	})
}

// CreateCorpusCollection creates a new corpus collection
func (s *SQLiteStorage) CreateCorpusCollection(ctx context.Context, collection *common.CorpusCollection) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		// Convert tags to JSON
		tagsJSON, err := json.Marshal(collection.Tags)
		if err != nil {
			return common.NewValidationError("marshal_tags", err)
		}

		_, err = s.db.ExecContext(ctx, `
			INSERT INTO corpus_collections (
				id, name, description, created_at, updated_at, file_count, total_size, tags
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			collection.ID, collection.Name, collection.Description,
			collection.CreatedAt, collection.UpdatedAt, collection.FileCount,
			collection.TotalSize, string(tagsJSON))

		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return common.NewValidationError("collection_exists", fmt.Errorf("collection with name '%s' already exists", collection.Name))
			}
			return common.NewDatabaseError("create_corpus_collection", err)
		}

		return nil
	})
}

// GetCorpusCollection retrieves a corpus collection by ID
func (s *SQLiteStorage) GetCorpusCollection(ctx context.Context, collectionID string) (*common.CorpusCollection, error) {
	var collection common.CorpusCollection
	var tagsJSON string
	var createdAtStr, updatedAtStr string

	err := ExecuteWithRetry(ctx, s.config, func() error {
		row := s.db.QueryRowContext(ctx, `
			SELECT id, name, description, created_at, updated_at, file_count, total_size, tags
			FROM corpus_collections WHERE id = ?`, collectionID)

		err := row.Scan(&collection.ID, &collection.Name, &collection.Description,
			&createdAtStr, &updatedAtStr, &collection.FileCount,
			&collection.TotalSize, &tagsJSON)

		if err == sql.ErrNoRows {
			return common.ErrKeyNotFound
		}
		return err
	})

	if err != nil {
		return nil, err
	}

	// Parse timestamps
	if createdAt, err := time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAtStr); err == nil {
		collection.CreatedAt = createdAt
	} else if createdAt, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
		collection.CreatedAt = createdAt
	} else {
		collection.CreatedAt = time.Now()
	}

	if updatedAt, err := time.Parse("2006-01-02 15:04:05.999999999-07:00", updatedAtStr); err == nil {
		collection.UpdatedAt = updatedAt
	} else if updatedAt, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
		collection.UpdatedAt = updatedAt
	} else {
		collection.UpdatedAt = time.Now()
	}

	// Parse tags JSON
	if tagsJSON != "" {
		if err := json.Unmarshal([]byte(tagsJSON), &collection.Tags); err != nil {
			s.logger.WithError(err).Warn("Failed to unmarshal collection tags")
			collection.Tags = []string{}
		}
	}

	return &collection, nil
}

// GetCorpusCollections retrieves all corpus collections
func (s *SQLiteStorage) GetCorpusCollections(ctx context.Context) ([]*common.CorpusCollection, error) {
	var collections []*common.CorpusCollection

	err := ExecuteWithRetry(ctx, s.config, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, name, description, created_at, updated_at, file_count, total_size, tags
			FROM corpus_collections ORDER BY created_at DESC`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var collection common.CorpusCollection
			var tagsJSON string
			var createdAtStr, updatedAtStr string

			err := rows.Scan(&collection.ID, &collection.Name, &collection.Description,
				&createdAtStr, &updatedAtStr, &collection.FileCount,
				&collection.TotalSize, &tagsJSON)
			if err != nil {
				return err
			}

			// Parse timestamps
			if createdAt, err := time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAtStr); err == nil {
				collection.CreatedAt = createdAt
			} else if createdAt, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
				collection.CreatedAt = createdAt
			} else {
				collection.CreatedAt = time.Now()
			}

			if updatedAt, err := time.Parse("2006-01-02 15:04:05.999999999-07:00", updatedAtStr); err == nil {
				collection.UpdatedAt = updatedAt
			} else if updatedAt, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
				collection.UpdatedAt = updatedAt
			} else {
				collection.UpdatedAt = time.Now()
			}

			// Parse tags JSON
			if tagsJSON != "" {
				if err := json.Unmarshal([]byte(tagsJSON), &collection.Tags); err != nil {
					s.logger.WithError(err).Warn("Failed to unmarshal collection tags")
					collection.Tags = []string{}
				}
			} else {
				collection.Tags = []string{}
			}

			collections = append(collections, &collection)
		}

		return rows.Err()
	})

	if err != nil {
		return nil, common.NewDatabaseError("get_corpus_collections", err)
	}

	return collections, nil
}

// UpdateCorpusCollection updates a corpus collection
func (s *SQLiteStorage) UpdateCorpusCollection(ctx context.Context, collection *common.CorpusCollection) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		// Convert tags to JSON
		tagsJSON, err := json.Marshal(collection.Tags)
		if err != nil {
			return common.NewValidationError("marshal_tags", err)
		}

		result, err := s.db.ExecContext(ctx, `
			UPDATE corpus_collections SET
				name = ?, description = ?, updated_at = ?, file_count = ?, total_size = ?, tags = ?
			WHERE id = ?`,
			collection.Name, collection.Description, collection.UpdatedAt,
			collection.FileCount, collection.TotalSize, string(tagsJSON), collection.ID)

		if err != nil {
			return common.NewDatabaseError("update_corpus_collection", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return common.NewDatabaseError("update_corpus_collection_rows", err)
		}

		if rowsAffected == 0 {
			return common.ErrKeyNotFound
		}

		return nil
	})
}

// DeleteCorpusCollection deletes a corpus collection and all associated files
func (s *SQLiteStorage) DeleteCorpusCollection(ctx context.Context, collectionID string) error {
	return RetryableTransaction(ctx, s.db, s.config, func(tx *sql.Tx) error {
		// Delete the collection (cascade will delete files)
		result, err := tx.ExecContext(ctx, "DELETE FROM corpus_collections WHERE id = ?", collectionID)
		if err != nil {
			return common.NewDatabaseError("delete_corpus_collection", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return common.NewDatabaseError("delete_corpus_collection_rows", err)
		}

		if rowsAffected == 0 {
			return common.ErrKeyNotFound
		}

		return nil
	})
}

// AddCorpusCollectionFile adds a file to a corpus collection
func (s *SQLiteStorage) AddCorpusCollectionFile(ctx context.Context, file *common.CorpusCollectionFile) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO corpus_collection_files (
				id, collection_id, filename, hash, size, uploaded_at
			) VALUES (?, ?, ?, ?, ?, ?)`,
			file.ID, file.CollectionID, file.Filename, file.Hash, file.Size, file.UploadedAt)

		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return common.NewValidationError("file_exists", fmt.Errorf("file with hash '%s' already exists in collection", file.Hash))
			}
			return common.NewDatabaseError("add_corpus_collection_file", err)
		}

		return nil
	})
}

// GetCorpusCollectionFiles retrieves all files in a corpus collection
func (s *SQLiteStorage) GetCorpusCollectionFiles(ctx context.Context, collectionID string) ([]*common.CorpusCollectionFile, error) {
	var files []*common.CorpusCollectionFile

	err := ExecuteWithRetry(ctx, s.config, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, collection_id, filename, hash, size, uploaded_at
			FROM corpus_collection_files WHERE collection_id = ? ORDER BY uploaded_at DESC`,
			collectionID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var file common.CorpusCollectionFile
			var uploadedAtStr string

			err := rows.Scan(&file.ID, &file.CollectionID, &file.Filename,
				&file.Hash, &file.Size, &uploadedAtStr)
			if err != nil {
				return err
			}

			// Parse timestamp
			if uploadedAt, err := time.Parse("2006-01-02 15:04:05.999999999-07:00", uploadedAtStr); err == nil {
				file.UploadedAt = uploadedAt
			} else if uploadedAt, err := time.Parse(time.RFC3339, uploadedAtStr); err == nil {
				file.UploadedAt = uploadedAt
			} else {
				file.UploadedAt = time.Now()
			}

			files = append(files, &file)
		}

		return rows.Err()
	})

	if err != nil {
		return nil, common.NewDatabaseError("get_corpus_collection_files", err)
	}

	return files, nil
}

// DeleteCorpusCollectionFile deletes a specific file from a corpus collection
func (s *SQLiteStorage) DeleteCorpusCollectionFile(ctx context.Context, fileID string) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		result, err := s.db.ExecContext(ctx, "DELETE FROM corpus_collection_files WHERE id = ?", fileID)
		if err != nil {
			return common.NewDatabaseError("delete_corpus_collection_file", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return common.NewDatabaseError("delete_corpus_collection_file_rows", err)
		}

		if rowsAffected == 0 {
			return common.ErrKeyNotFound
		}

		return nil
	})
}
