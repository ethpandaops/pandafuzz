package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// CreateCrashGroup creates a new crash group for deduplication
func (s *SQLiteStorage) CreateCrashGroup(ctx context.Context, cg *common.CrashGroup) error {
	// Serialize stack frames to JSON
	stackFramesJSON, err := json.Marshal(cg.StackFrames)
	if err != nil {
		return fmt.Errorf("failed to marshal stack frames: %w", err)
	}

	return ExecuteWithRetry(ctx, s.config, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO crash_groups (
				id, campaign_id, stack_hash, first_seen, last_seen,
				count, severity, stack_frames, example_crash
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, cg.ID, cg.CampaignID, cg.StackHash, cg.FirstSeen, cg.LastSeen,
			cg.Count, cg.Severity, string(stackFramesJSON), cg.ExampleCrash)
		return err
	})
}

// GetCrashGroup retrieves a crash group by campaign ID and stack hash
func (s *SQLiteStorage) GetCrashGroup(ctx context.Context, campaignID, stackHash string) (*common.CrashGroup, error) {
	var cg common.CrashGroup
	var stackFramesJSON string

	err := ExecuteWithRetry(ctx, s.config, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT id, campaign_id, stack_hash, first_seen, last_seen,
				count, severity, stack_frames, example_crash
			FROM crash_groups
			WHERE campaign_id = ? AND stack_hash = ?
		`, campaignID, stackHash).Scan(
			&cg.ID, &cg.CampaignID, &cg.StackHash, &cg.FirstSeen, &cg.LastSeen,
			&cg.Count, &cg.Severity, &stackFramesJSON, &cg.ExampleCrash)
	})

	if err == sql.ErrNoRows {
		return nil, common.ErrCrashGroupNotFound
	}
	if err != nil {
		return nil, err
	}

	// Deserialize stack frames
	if err := json.Unmarshal([]byte(stackFramesJSON), &cg.StackFrames); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stack frames: %w", err)
	}

	return &cg, nil
}

// UpdateCrashGroupCount increments the crash count and updates last seen time
func (s *SQLiteStorage) UpdateCrashGroupCount(ctx context.Context, id string) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		result, err := s.db.ExecContext(ctx, `
			UPDATE crash_groups 
			SET count = count + 1, last_seen = CURRENT_TIMESTAMP
			WHERE id = ?
		`, id)
		if err != nil {
			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}

		if rowsAffected == 0 {
			return common.ErrCrashGroupNotFound
		}

		return nil
	})
}

// ListCrashGroups retrieves all crash groups for a campaign
func (s *SQLiteStorage) ListCrashGroups(ctx context.Context, campaignID string) ([]*common.CrashGroup, error) {
	var groups []*common.CrashGroup

	err := ExecuteWithRetry(ctx, s.config, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, campaign_id, stack_hash, first_seen, last_seen,
				count, severity, stack_frames, example_crash
			FROM crash_groups
			WHERE campaign_id = ?
			ORDER BY count DESC, last_seen DESC
		`, campaignID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var cg common.CrashGroup
			var stackFramesJSON string

			err := rows.Scan(
				&cg.ID, &cg.CampaignID, &cg.StackHash, &cg.FirstSeen, &cg.LastSeen,
				&cg.Count, &cg.Severity, &stackFramesJSON, &cg.ExampleCrash)
			if err != nil {
				return err
			}

			// Deserialize stack frames
			if err := json.Unmarshal([]byte(stackFramesJSON), &cg.StackFrames); err != nil {
				return fmt.Errorf("failed to unmarshal stack frames: %w", err)
			}

			groups = append(groups, &cg)
		}

		return rows.Err()
	})

	return groups, err
}

// CreateStackTrace stores a parsed stack trace for a crash
func (s *SQLiteStorage) CreateStackTrace(ctx context.Context, crashID string, st *common.StackTrace) error {
	// Serialize frames to JSON
	framesJSON, err := json.Marshal(st.Frames)
	if err != nil {
		return fmt.Errorf("failed to marshal stack frames: %w", err)
	}

	return ExecuteWithRetry(ctx, s.config, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO stack_traces (
				crash_id, top_n_hash, full_hash, frames, raw_trace
			) VALUES (?, ?, ?, ?, ?)
		`, crashID, st.TopNHash, st.FullHash, string(framesJSON), st.RawTrace)
		return err
	})
}

// GetStackTrace retrieves a stack trace by crash ID
func (s *SQLiteStorage) GetStackTrace(ctx context.Context, crashID string) (*common.StackTrace, error) {
	var st common.StackTrace
	var framesJSON string

	err := ExecuteWithRetry(ctx, s.config, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT top_n_hash, full_hash, frames, raw_trace
			FROM stack_traces
			WHERE crash_id = ?
		`, crashID).Scan(&st.TopNHash, &st.FullHash, &framesJSON, &st.RawTrace)
	})

	if err == sql.ErrNoRows {
		return nil, nil // Stack trace not found is not an error
	}
	if err != nil {
		return nil, err
	}

	// Deserialize frames
	if err := json.Unmarshal([]byte(framesJSON), &st.Frames); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stack frames: %w", err)
	}

	return &st, nil
}

// LinkCrashToGroup associates a crash with a crash group
func (s *SQLiteStorage) LinkCrashToGroup(ctx context.Context, crashID, groupID string) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		result, err := s.db.ExecContext(ctx, `
			UPDATE crashes 
			SET crash_group_id = ?
			WHERE id = ?
		`, groupID, crashID)
		if err != nil {
			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}

		if rowsAffected == 0 {
			return fmt.Errorf("crash not found: %s", crashID)
		}

		return nil
	})
}

// UpdateCrashWithCampaign updates a crash to associate it with a campaign
func (s *SQLiteStorage) UpdateCrashWithCampaign(ctx context.Context, crashID, campaignID string) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		result, err := s.db.ExecContext(ctx, `
			UPDATE crashes 
			SET campaign_id = ?
			WHERE id = ?
		`, campaignID, crashID)
		if err != nil {
			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}

		if rowsAffected == 0 {
			return fmt.Errorf("crash not found: %s", crashID)
		}

		return nil
	})
}

// GetCrashesWithoutStackTrace finds crashes that need stack trace processing
func (s *SQLiteStorage) GetCrashesWithoutStackTrace(ctx context.Context, limit int) ([]*common.CrashResult, error) {
	var crashes []*common.CrashResult

	err := ExecuteWithRetry(ctx, s.config, func() error {
		query := `
			SELECT c.id, c.job_id, c.bot_id, c.hash, c.file_path, c.type,
				c.signal, c.exit_code, c.timestamp, c.size, c.is_unique,
				c.output, c.stack_trace
			FROM crashes c
			LEFT JOIN stack_traces st ON c.id = st.crash_id
			WHERE st.crash_id IS NULL
			AND c.stack_trace IS NOT NULL
			AND c.stack_trace != ''
		`
		if limit > 0 {
			query += fmt.Sprintf(" LIMIT %d", limit)
		}

		rows, err := s.db.QueryContext(ctx, query)
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

// GetCrashGroupStats gets statistics about crash groups for a campaign
func (s *SQLiteStorage) GetCrashGroupStats(ctx context.Context, campaignID string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	err := ExecuteWithRetry(ctx, s.config, func() error {
		// Total unique crash groups
		var totalGroups int
		err := s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM crash_groups WHERE campaign_id = ?
		`, campaignID).Scan(&totalGroups)
		if err != nil {
			return err
		}
		stats["total_groups"] = totalGroups

		// Total crashes across all groups
		var totalCrashes int
		err = s.db.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(count), 0) FROM crash_groups WHERE campaign_id = ?
		`, campaignID).Scan(&totalCrashes)
		if err != nil {
			return err
		}
		stats["total_crashes"] = totalCrashes

		// Most common crash
		var mostCommonID, mostCommonHash sql.NullString
		var mostCommonCount sql.NullInt64
		err = s.db.QueryRowContext(ctx, `
			SELECT id, stack_hash, count 
			FROM crash_groups 
			WHERE campaign_id = ?
			ORDER BY count DESC
			LIMIT 1
		`, campaignID).Scan(&mostCommonID, &mostCommonHash, &mostCommonCount)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if mostCommonID.Valid {
			stats["most_common_crash"] = map[string]interface{}{
				"id":         mostCommonID.String,
				"stack_hash": mostCommonHash.String,
				"count":      mostCommonCount.Int64,
			}
		}

		return nil
	})

	return stats, err
}
