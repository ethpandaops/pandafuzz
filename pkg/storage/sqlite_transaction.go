package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// SQLiteTransaction implements the Transaction interface
type SQLiteTransaction struct {
	tx     *sql.Tx
	logger *logrus.Logger
	ctx    context.Context
}

// Compile-time interface compliance check
var _ common.Transaction = (*SQLiteTransaction)(nil)

// Store stores a value in the transaction
func (tx *SQLiteTransaction) Store(ctx context.Context, key string, value any) error {
	// Marshal the value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		return common.NewDatabaseError("marshal_value", err)
	}

	// Parse key to determine table and ID
	table, id := parseKey(key)
	if table == "" || id == "" {
		// Fallback to metadata table for unstructured keys
		_, err = tx.tx.ExecContext(ctx, "INSERT OR REPLACE INTO metadata (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)", key, string(data))
		return err
	}

	// Route to appropriate table based on key prefix
	switch table {
	case "bot":
		return tx.storeBotInTx(ctx, id, string(data))
	case "job":
		return tx.storeJobInTx(ctx, id, string(data))
	case "crash":
		return tx.storeCrashInTx(ctx, id, string(data))
	case "coverage":
		return tx.storeCoverageInTx(ctx, id, string(data))
	case "corpus":
		return tx.storeCorpusInTx(ctx, id, string(data))
	case "assignment":
		return tx.storeAssignmentInTx(ctx, id, string(data))
	case "crash_input":
		// Store crash input as binary data
		if binaryData, ok := value.([]byte); ok {
			_, err = tx.tx.ExecContext(ctx, "INSERT OR REPLACE INTO crash_inputs (crash_id, input) VALUES (?, ?)", id, binaryData)
			return err
		}
		return fmt.Errorf("crash_input value must be []byte")
	default:
		// Store in metadata table for unknown types
		_, err = tx.tx.ExecContext(ctx, "INSERT OR REPLACE INTO metadata (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)", key, string(data))
		return err
	}
}

// Get retrieves a value from the transaction
func (tx *SQLiteTransaction) Get(ctx context.Context, key string, dest any) error {
	var data string
	err := tx.tx.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key = ?", key).Scan(&data)
	if err == sql.ErrNoRows {
		return common.ErrKeyNotFound
	}
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(data), dest)
}

// Delete removes a value from the transaction
func (tx *SQLiteTransaction) Delete(ctx context.Context, key string) error {
	// Route delete based on key prefix (same as deleteByKeyContext)
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		_, err := tx.tx.ExecContext(ctx, "DELETE FROM metadata WHERE key = ?", key)
		return err
	}

	table := parts[0]
	id := parts[1]

	var query string
	switch table {
	case "bot":
		query = "DELETE FROM bots WHERE id = ?"
	case "job":
		query = "DELETE FROM jobs WHERE id = ?"
	case "crash":
		query = "DELETE FROM crashes WHERE id = ?"
	case "coverage":
		query = "DELETE FROM coverage WHERE id = ?"
	case "corpus":
		query = "DELETE FROM corpus WHERE id = ?"
	case "assignment":
		query = "DELETE FROM assignments WHERE id = ?"
	default:
		_, err := tx.tx.ExecContext(ctx, "DELETE FROM metadata WHERE key = ?", key)
		return err
	}

	_, err := tx.tx.ExecContext(ctx, query, id)
	return err
}

// Commit commits the transaction
func (tx *SQLiteTransaction) Commit(ctx context.Context) error {
	return tx.tx.Commit()
}

// Rollback rolls back the transaction
func (tx *SQLiteTransaction) Rollback(ctx context.Context) error {
	return tx.tx.Rollback()
}

// Transaction helper methods for storing data in proper tables
func (tx *SQLiteTransaction) storeBotInTx(ctx context.Context, id, data string) error {
	query := `INSERT OR REPLACE INTO bots (id, name, hostname, status, last_seen, registered_at, current_job, capabilities, timeout_at, is_online, failure_count, api_endpoint)
			  SELECT ?, json_extract(?, '$.name'), json_extract(?, '$.hostname'), json_extract(?, '$.status'), json_extract(?, '$.last_seen'),
			         json_extract(?, '$.registered_at'), json_extract(?, '$.current_job'), json_extract(?, '$.capabilities'),
			         json_extract(?, '$.timeout_at'), json_extract(?, '$.is_online'), json_extract(?, '$.failure_count'), json_extract(?, '$.api_endpoint')`

	_, err := tx.tx.ExecContext(ctx, query, id, data, data, data, data, data, data, data, data, data, data, data)
	return err
}

func (tx *SQLiteTransaction) storeJobInTx(ctx context.Context, id, data string) error {
	// Debug: log the JSON data to verify coverage fields
	var jobData map[string]interface{}
	if err := json.Unmarshal([]byte(data), &jobData); err == nil {
		tx.logger.WithFields(logrus.Fields{
			"job_id":          id,
			"enable_coverage": jobData["enable_coverage"],
			"coverage_format": jobData["coverage_format"],
		}).Info("DEBUG: Storing job in SQLite with coverage settings")
	}

	query := `INSERT OR REPLACE INTO jobs (id, name, target, fuzzer, status, assigned_bot, created_at, started_at, completed_at, timeout_at, work_dir, config, collection_id, campaign_id, use_campaign_corpus, enable_coverage, coverage_format, updated_at)
			  SELECT ?, json_extract(?, '$.name'), json_extract(?, '$.target'), json_extract(?, '$.fuzzer'),
			         json_extract(?, '$.status'), json_extract(?, '$.assigned_bot'), json_extract(?, '$.created_at'),
			         json_extract(?, '$.started_at'), json_extract(?, '$.completed_at'), json_extract(?, '$.timeout_at'),
			         json_extract(?, '$.work_dir'), json_extract(?, '$.config'), json_extract(?, '$.collection_id'),
			         json_extract(?, '$.campaign_id'), json_extract(?, '$.use_campaign_corpus'),
			         json_extract(?, '$.enable_coverage'), json_extract(?, '$.coverage_format'), CURRENT_TIMESTAMP`

	_, err := tx.tx.ExecContext(ctx, query, id, data, data, data, data, data, data, data, data, data, data, data, data, data, data, data, data)
	return err
}

func (tx *SQLiteTransaction) storeCrashInTx(ctx context.Context, id, data string) error {
	// Parse data to extract crash information for logging
	var crashInfo map[string]interface{}
	if err := json.Unmarshal([]byte(data), &crashInfo); err == nil {
		tx.logger.WithFields(logrus.Fields{
			"crash_id": id,
			"job_id":   crashInfo["job_id"],
			"bot_id":   crashInfo["bot_id"],
			"hash":     crashInfo["hash"],
			"type":     crashInfo["type"],
			"size":     crashInfo["size"],
		}).Info("Storing crash in database")
	}

	query := `INSERT OR REPLACE INTO crashes (id, job_id, bot_id, hash, file_path, type, signal, exit_code, timestamp, size, is_unique, output, stack_trace)
			  SELECT ?, json_extract(?, '$.job_id'), json_extract(?, '$.bot_id'), json_extract(?, '$.hash'),
			         json_extract(?, '$.file_path'), json_extract(?, '$.type'), json_extract(?, '$.signal'),
			         json_extract(?, '$.exit_code'), json_extract(?, '$.timestamp'), json_extract(?, '$.size'),
			         json_extract(?, '$.is_unique'), json_extract(?, '$.output'), json_extract(?, '$.stack_trace')`

	_, err := tx.tx.ExecContext(ctx, query, id, data, data, data, data, data, data, data, data, data, data, data, data)

	if err != nil {
		tx.logger.WithError(err).WithField("crash_id", id).Error("Failed to store crash in database")
		return err
	}

	tx.logger.WithField("crash_id", id).Debug("Crash stored successfully in database")
	return nil
}

func (tx *SQLiteTransaction) storeCoverageInTx(ctx context.Context, id, data string) error {
	query := `INSERT OR REPLACE INTO coverage (id, job_id, bot_id, edges, new_edges, timestamp, exec_count)
			  SELECT ?, json_extract(?, '$.job_id'), json_extract(?, '$.bot_id'), json_extract(?, '$.edges'),
			         json_extract(?, '$.new_edges'), json_extract(?, '$.timestamp'), json_extract(?, '$.exec_count')`

	_, err := tx.tx.ExecContext(ctx, query, id, data, data, data, data, data, data)
	return err
}

func (tx *SQLiteTransaction) storeCorpusInTx(ctx context.Context, id, data string) error {
	query := `INSERT OR REPLACE INTO corpus_updates (id, job_id, bot_id, files, timestamp, total_size)
			  SELECT ?, json_extract(?, '$.job_id'), json_extract(?, '$.bot_id'), json_extract(?, '$.files'),
			         json_extract(?, '$.timestamp'), json_extract(?, '$.total_size')`

	_, err := tx.tx.ExecContext(ctx, query, id, data, data, data, data, data)
	return err
}

func (tx *SQLiteTransaction) storeAssignmentInTx(ctx context.Context, id, data string) error {
	query := `INSERT OR REPLACE INTO job_assignments (job_id, bot_id, timestamp, status)
			  SELECT ?, json_extract(?, '$.bot_id'), json_extract(?, '$.timestamp'), json_extract(?, '$.status')`

	_, err := tx.tx.ExecContext(ctx, query, id, data, data, data)
	return err
}

// BeginTx starts a new database transaction
func (s *SQLiteStorage) BeginTx(ctx context.Context) (common.Transaction, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &SQLiteTransaction{
		tx:     tx,
		logger: s.logger,
		ctx:    ctx,
	}, nil
}
