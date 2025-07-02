package storage

import (
	"context"
	"database/sql"
	"fmt"
	
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// PopulateMissingCrashInputs checks for crashes without inputs and attempts to populate them
func (s *SQLiteStorage) PopulateMissingCrashInputs(ctx context.Context) error {
	s.logger.Info("Checking for crashes with missing inputs...")
	
	// Find crashes that don't have corresponding entries in crash_inputs
	query := `
		SELECT c.id, c.job_id, c.bot_id, c.hash, c.file_path, c.type, 
		       c.signal, c.exit_code, c.timestamp, c.size, c.is_unique, 
		       c.output, c.stack_trace
		FROM crashes c
		LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
		WHERE ci.crash_id IS NULL
	`
	
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query crashes without inputs: %w", err)
	}
	defer rows.Close()
	
	count := 0
	for rows.Next() {
		var crash common.CrashResult
		var output, stackTrace sql.NullString
		
		err := rows.Scan(&crash.ID, &crash.JobID, &crash.BotID, &crash.Hash, 
			&crash.FilePath, &crash.Type, &crash.Signal, &crash.ExitCode, 
			&crash.Timestamp, &crash.Size, &crash.IsUnique, &output, &stackTrace)
		if err != nil {
			s.logger.WithError(err).Error("Failed to scan crash row")
			continue
		}
		
		crash.Output = output.String
		crash.StackTrace = stackTrace.String
		
		// For old crashes, we'll create a placeholder input
		// In a real scenario, you might try to recover the actual input from backups
		placeholderInput := []byte(fmt.Sprintf("PLACEHOLDER_INPUT_FOR_CRASH_%s", crash.ID))
		
		// Store the placeholder input
		if err := s.StoreCrashInput(ctx, crash.ID, placeholderInput); err != nil {
			s.logger.WithError(err).WithField("crash_id", crash.ID).Error("Failed to store placeholder input")
			continue
		}
		
		count++
		s.logger.WithFields(logrus.Fields{
			"crash_id": crash.ID,
			"job_id":   crash.JobID,
		}).Info("Added placeholder input for crash")
	}
	
	s.logger.WithField("count", count).Info("Finished populating missing crash inputs")
	return rows.Err()
}