package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// CreateCampaign creates a new campaign in the database
func (s *SQLiteStorage) CreateCampaign(ctx context.Context, c *common.Campaign) error {
	// Serialize JSON fields
	jobTemplateJSON, err := json.Marshal(c.JobTemplate)
	if err != nil {
		return fmt.Errorf("failed to marshal job template: %w", err)
	}

	tagsJSON, err := json.Marshal(c.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	// Execute insert with retry logic
	return s.ExecuteWithRetry(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO campaigns (
				id, name, description, status, target_binary, binary_hash,
				created_at, updated_at, auto_restart, max_duration, max_jobs,
				job_template, shared_corpus, tags
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, c.ID, c.Name, c.Description, c.Status, c.TargetBinary, c.BinaryHash,
			c.CreatedAt, c.UpdatedAt, c.AutoRestart, int64(c.MaxDuration.Seconds()),
			c.MaxJobs, string(jobTemplateJSON), c.SharedCorpus, string(tagsJSON))
		return err
	})
}

// GetCampaign retrieves a campaign by ID
func (s *SQLiteStorage) GetCampaign(ctx context.Context, id string) (*common.Campaign, error) {
	var c common.Campaign
	var completedAt sql.NullTime
	var maxDurationSeconds sql.NullInt64
	var jobTemplateJSON, tagsJSON string

	err := s.ExecuteWithRetry(ctx, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT id, name, description, status, target_binary, binary_hash,
				created_at, updated_at, completed_at, auto_restart, max_duration,
				max_jobs, job_template, shared_corpus, tags
			FROM campaigns WHERE id = ?
		`, id).Scan(
			&c.ID, &c.Name, &c.Description, &c.Status, &c.TargetBinary, &c.BinaryHash,
			&c.CreatedAt, &c.UpdatedAt, &completedAt, &c.AutoRestart, &maxDurationSeconds,
			&c.MaxJobs, &jobTemplateJSON, &c.SharedCorpus, &tagsJSON)
	})

	if err == sql.ErrNoRows {
		return nil, common.ErrCampaignNotFound
	}
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if completedAt.Valid {
		c.CompletedAt = &completedAt.Time
	}
	if maxDurationSeconds.Valid {
		c.MaxDuration = time.Duration(maxDurationSeconds.Int64) * time.Second
	}

	// Deserialize JSON fields
	if err := json.Unmarshal([]byte(jobTemplateJSON), &c.JobTemplate); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job template: %w", err)
	}
	if err := json.Unmarshal([]byte(tagsJSON), &c.Tags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	return &c, nil
}

// ListCampaigns retrieves campaigns with pagination and optional status filter
func (s *SQLiteStorage) ListCampaigns(ctx context.Context, limit, offset int, status string) ([]*common.Campaign, error) {
	query := `
		SELECT id, name, description, status, target_binary, binary_hash,
			created_at, updated_at, completed_at, auto_restart, max_duration,
			max_jobs, job_template, shared_corpus, tags
		FROM campaigns
	`
	args := []interface{}{}

	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
		if offset > 0 {
			query += " OFFSET ?"
			args = append(args, offset)
		}
	}

	var campaigns []*common.Campaign
	err := s.ExecuteWithRetry(ctx, func() error {
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c common.Campaign
			var completedAt sql.NullTime
			var maxDurationSeconds sql.NullInt64
			var jobTemplateJSON, tagsJSON string

			err := rows.Scan(
				&c.ID, &c.Name, &c.Description, &c.Status, &c.TargetBinary, &c.BinaryHash,
				&c.CreatedAt, &c.UpdatedAt, &completedAt, &c.AutoRestart, &maxDurationSeconds,
				&c.MaxJobs, &jobTemplateJSON, &c.SharedCorpus, &tagsJSON)
			if err != nil {
				return err
			}

			// Handle nullable fields
			if completedAt.Valid {
				c.CompletedAt = &completedAt.Time
			}
			if maxDurationSeconds.Valid {
				c.MaxDuration = time.Duration(maxDurationSeconds.Int64) * time.Second
			}

			// Deserialize JSON fields
			if err := json.Unmarshal([]byte(jobTemplateJSON), &c.JobTemplate); err != nil {
				return fmt.Errorf("failed to unmarshal job template: %w", err)
			}
			if err := json.Unmarshal([]byte(tagsJSON), &c.Tags); err != nil {
				return fmt.Errorf("failed to unmarshal tags: %w", err)
			}

			campaigns = append(campaigns, &c)
		}

		return rows.Err()
	})

	return campaigns, err
}

// UpdateCampaign updates a campaign with the provided fields
func (s *SQLiteStorage) UpdateCampaign(ctx context.Context, id string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	// Build dynamic update query
	query := "UPDATE campaigns SET updated_at = CURRENT_TIMESTAMP"
	args := []interface{}{}

	for field, value := range updates {
		switch field {
		case "name", "description", "status", "target_binary", "binary_hash":
			query += fmt.Sprintf(", %s = ?", field)
			args = append(args, value)
		case "auto_restart", "shared_corpus":
			query += fmt.Sprintf(", %s = ?", field)
			args = append(args, value)
		case "max_jobs":
			query += ", max_jobs = ?"
			args = append(args, value)
		case "max_duration":
			if duration, ok := value.(time.Duration); ok {
				query += ", max_duration = ?"
				args = append(args, int64(duration.Seconds()))
			}
		case "completed_at":
			if t, ok := value.(*time.Time); ok && t != nil {
				query += ", completed_at = ?"
				args = append(args, *t)
			} else {
				query += ", completed_at = NULL"
			}
		case "job_template":
			jobTemplateJSON, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("failed to marshal job template: %w", err)
			}
			query += ", job_template = ?"
			args = append(args, string(jobTemplateJSON))
		case "tags":
			tagsJSON, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("failed to marshal tags: %w", err)
			}
			query += ", tags = ?"
			args = append(args, string(tagsJSON))
		}
	}

	query += " WHERE id = ?"
	args = append(args, id)

	return s.ExecuteWithRetry(ctx, func() error {
		result, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}

		if rowsAffected == 0 {
			return common.ErrCampaignNotFound
		}

		return nil
	})
}

// DeleteCampaign deletes a campaign and all related data
func (s *SQLiteStorage) DeleteCampaign(ctx context.Context, id string) error {
	return s.ExecuteWithRetry(ctx, func() error {
		result, err := s.db.ExecContext(ctx, "DELETE FROM campaigns WHERE id = ?", id)
		if err != nil {
			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}

		if rowsAffected == 0 {
			return common.ErrCampaignNotFound
		}

		return nil
	})
}

// GetCampaignJobs retrieves all jobs associated with a campaign
func (s *SQLiteStorage) GetCampaignJobs(ctx context.Context, campaignID string) ([]*common.Job, error) {
	var jobs []*common.Job

	err := s.ExecuteWithRetry(ctx, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT j.id, j.name, j.target, j.fuzzer, j.status, j.assigned_bot,
				j.created_at, j.started_at, j.completed_at, j.timeout_at, j.work_dir, j.config, j.progress
			FROM jobs j
			INNER JOIN campaign_jobs cj ON j.id = cj.job_id
			WHERE cj.campaign_id = ?
			ORDER BY j.created_at DESC
		`, campaignID)
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

// LinkJobToCampaign associates a job with a campaign
func (s *SQLiteStorage) LinkJobToCampaign(ctx context.Context, campaignID, jobID string) error {
	return s.ExecuteWithRetry(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO campaign_jobs (campaign_id, job_id) VALUES (?, ?)
		`, campaignID, jobID)
		return err
	})
}

// GetCampaignStatistics retrieves aggregated statistics for a campaign
func (s *SQLiteStorage) GetCampaignStatistics(ctx context.Context, campaignID string) (*common.CampaignStats, error) {
	stats := &common.CampaignStats{
		CampaignID:  campaignID,
		LastUpdated: time.Now(),
	}

	// Get job statistics
	err := s.ExecuteWithRetry(ctx, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT 
				COUNT(*) as total_jobs,
				SUM(CASE WHEN j.status = 'completed' THEN 1 ELSE 0 END) as completed_jobs
			FROM jobs j
			INNER JOIN campaign_jobs cj ON j.id = cj.job_id
			WHERE cj.campaign_id = ?
		`, campaignID).Scan(&stats.TotalJobs, &stats.CompletedJobs)
	})
	if err != nil {
		return nil, err
	}

	// Get crash statistics
	err = s.ExecuteWithRetry(ctx, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT 
				COUNT(*) as total_crashes,
				COUNT(DISTINCT crash_group_id) as unique_crashes
			FROM crashes
			WHERE campaign_id = ?
		`, campaignID).Scan(&stats.TotalCrashes, &stats.UniqueCrashes)
	})
	if err != nil {
		return nil, err
	}

	// Get coverage statistics
	err = s.ExecuteWithRetry(ctx, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT 
				COALESCE(MAX(edges), 0) as total_coverage
			FROM coverage c
			INNER JOIN campaign_jobs cj ON c.job_id = cj.job_id
			WHERE cj.campaign_id = ?
		`, campaignID).Scan(&stats.TotalCoverage)
	})
	if err != nil {
		return nil, err
	}

	// Get corpus size
	err = s.ExecuteWithRetry(ctx, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT 
				COALESCE(SUM(size), 0) as corpus_size
			FROM campaign_corpus_files
			WHERE campaign_id = ?
		`, campaignID).Scan(&stats.CorpusSize)
	})
	if err != nil {
		return nil, err
	}

	return stats, nil
}
