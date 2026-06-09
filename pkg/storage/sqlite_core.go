package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

// SQLiteStorage implements the Database interface using SQLite
type SQLiteStorage struct {
	db     *sql.DB
	pool   *ConnectionPool
	path   string
	logger *logrus.Logger
	config common.DatabaseConfig
}

// Compile-time interface compliance check
var _ common.AdvancedDatabase = (*SQLiteStorage)(nil)

// NewSQLiteStorage creates a new SQLite storage instance
func NewSQLiteStorage(config common.DatabaseConfig, logger *logrus.Logger) (common.AdvancedDatabase, error) {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(config.Path), 0755); err != nil {
		return nil, common.NewStorageError("create_directory", err)
	}

	// Build connection string with production settings
	connStr := config.Path + "?cache=shared&mode=rwc&_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000"

	// Add additional options if specified
	for key, value := range config.Options {
		connStr += fmt.Sprintf("&_%s=%s", key, value)
	}

	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return nil, common.NewDatabaseError("open_database", err)
	}

	// Configure connection pool for SQLite
	// With WAL mode, SQLite can handle multiple readers + one writer
	// Using a small pool to balance performance and lock contention
	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(0) // Don't expire connections

	// Set optimal pragmas for concurrent access
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA cache_size = -64000", // 64MB cache
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, common.NewDatabaseError("set_pragma", fmt.Errorf("%s: %w", pragma, err))
		}
	}

	// Create connection pool with retry and health check capabilities
	pool, err := ConfigurePool(db, config, logger)
	if err != nil {
		db.Close()
		return nil, common.NewDatabaseError("configure_pool", err)
	}

	storage := &SQLiteStorage{
		db:     db,
		pool:   pool,
		path:   config.Path,
		logger: logger,
		config: config,
	}

	// Initialize database schema with retry
	err = pool.ExecuteWithRetry(context.Background(), "create_tables", func(ctx context.Context) error {
		return storage.createTablesContext(ctx)
	})
	if err != nil {
		pool.Stop()
		return nil, common.NewDatabaseError("create_tables", err)
	}

	// Apply migrations for normalized schema with retry
	err = pool.ExecuteWithRetry(context.Background(), "migrate_data", func(ctx context.Context) error {
		return MigrateExistingData(ctx, db)
	})
	if err != nil {
		pool.Stop()
		return nil, common.NewDatabaseError("apply_migrations", err)
	}

	logger.WithField("path", config.Path).Info("SQLite storage initialized with normalized schema and connection pooling")
	return storage, nil
}

// GetDB returns the underlying SQL database connection
func (s *SQLiteStorage) GetDB() *sql.DB {
	return s.db
}

// GetPool returns the connection pool for advanced usage
func (s *SQLiteStorage) GetPool() *ConnectionPool {
	return s.pool
}

// Store implements the Database interface
func (s *SQLiteStorage) Store(ctx context.Context, key string, value any) error {
	// Check context before proceeding
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Convert value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		return common.NewDatabaseError("marshal_value", err)
	}

	// Use retry logic for the store operation
	return s.pool.ExecuteWithRetry(ctx, "store", func(ctx context.Context) error {
		return s.storeByKeyContext(ctx, key, string(data))
	})
}

// Get implements the Database interface
func (s *SQLiteStorage) Get(ctx context.Context, key string, dest any) error {
	// Check context before proceeding
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Use retry logic for the get operation
	data, err := ExecutePoolWithRetryResult(s.pool, ctx, "get", func(ctx context.Context) (string, error) {
		return s.getByKeyContext(ctx, key)
	})
	if err != nil {
		return err
	}

	// Unmarshal JSON data
	if err := json.Unmarshal([]byte(data), dest); err != nil {
		return common.NewDatabaseError("unmarshal_value", err)
	}

	return nil
}

// Delete implements the Database interface
func (s *SQLiteStorage) Delete(ctx context.Context, key string) error {
	// Check context before proceeding
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Use retry logic for the delete operation
	return s.pool.ExecuteWithRetry(ctx, "delete", func(ctx context.Context) error {
		return s.deleteByKeyContext(ctx, key)
	})
}

// Transaction implements the Database interface
func (s *SQLiteStorage) Transaction(ctx context.Context, fn func(tx common.Transaction) error) error {
	// Wrap entire transaction in retry logic to handle transient locking issues
	return s.pool.ExecuteWithRetry(ctx, "transaction", func(ctx context.Context) error {
		// Check context before proceeding
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Use IMMEDIATE mode to avoid deadlocks in concurrent scenarios
		sqlTx, err := s.db.BeginTx(ctx, &sql.TxOptions{
			Isolation: sql.LevelDefault,
		})
		if err != nil {
			return common.NewDatabaseError("begin_transaction", err)
		}

		tx := &SQLiteTransaction{
			tx:     sqlTx,
			logger: s.logger,
			ctx:    ctx,
		}

		defer func() {
			if p := recover(); p != nil {
				sqlTx.Rollback()
				panic(p)
			}
		}()

		if err := fn(tx); err != nil {
			if rollbackErr := sqlTx.Rollback(); rollbackErr != nil {
				s.logger.WithError(rollbackErr).Error("Failed to rollback transaction")
			}
			return err
		}

		if err := sqlTx.Commit(); err != nil {
			return common.NewDatabaseError("commit_transaction", err)
		}

		return nil
	})
}

// Close implements the Database interface
func (s *SQLiteStorage) Close(ctx context.Context) error {
	// Check context before proceeding
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Stop the connection pool first (which will close the database)
	if s.pool != nil {
		if err := s.pool.Stop(); err != nil {
			return common.NewDatabaseError("close_pool", err)
		}
		s.pool = nil
		s.db = nil // db is closed by pool.Stop()
	} else if s.db != nil {
		// Fallback if pool is not initialized
		err := s.db.Close()
		s.db = nil
		if err != nil {
			return common.NewDatabaseError("close_database", err)
		}
	}
	return nil
}

// Ping implements the Database interface
func (s *SQLiteStorage) Ping(ctx context.Context) error {
	if s.pool == nil || s.db == nil {
		return common.ErrDatabaseClosed
	}

	// Use the pool's health check mechanism
	if !s.pool.IsHealthy() {
		return common.NewDatabaseError("ping", fmt.Errorf("connection pool is unhealthy"))
	}

	// Create a timeout context if none exists
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	if err := s.db.PingContext(ctx); err != nil {
		return common.NewDatabaseError("ping", err)
	}

	return nil
}

// Stats implements the Database interface
func (s *SQLiteStorage) Stats(ctx context.Context) common.DatabaseStats {
	s.logger.Debug("Stats: Starting database stats collection")
	start := time.Now()

	stats := common.DatabaseStats{
		Type:      "sqlite",
		Path:      s.path,
		IsHealthy: true,
	}

	// Get database file size
	fileStart := time.Now()
	if fileInfo, err := os.Stat(s.path); err == nil {
		stats.Size = fileInfo.Size()
		s.logger.WithField("duration", time.Since(fileStart)).Debug("Stats: File size retrieved")
	} else {
		s.logger.WithError(err).Warn("Stats: Failed to get file size")
	}

	// Get connection stats
	connStart := time.Now()
	if s.db != nil {
		dbStats := s.db.Stats()
		stats.Connections = dbStats.OpenConnections
		s.logger.WithFields(logrus.Fields{
			"duration":         time.Since(connStart),
			"open_connections": dbStats.OpenConnections,
			"in_use":           dbStats.InUse,
			"idle":             dbStats.Idle,
		}).Debug("Stats: Connection stats retrieved")
	}

	// Count total keys (tables)
	keyStart := time.Now()
	if keyCount, err := s.getTotalKeysContext(ctx); err == nil {
		stats.Keys = keyCount
		s.logger.WithFields(logrus.Fields{
			"duration": time.Since(keyStart),
			"keys":     keyCount,
		}).Debug("Stats: Key count retrieved")
	} else {
		s.logger.WithError(err).WithField("duration", time.Since(keyStart)).Warn("Stats: Failed to get key count")
		stats.IsHealthy = false
	}

	s.logger.WithFields(logrus.Fields{
		"total_duration": time.Since(start),
		"keys":           stats.Keys,
		"size":           stats.Size,
		"connections":    stats.Connections,
	}).Debug("Stats: Database stats collection completed")

	return stats
}

// storeByKeyContext stores data based on key prefix with context
func (s *SQLiteStorage) storeByKeyContext(ctx context.Context, key, data string) error {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return s.storeMetadataContext(context.Background(), key, data)
	}

	table := parts[0]
	id := parts[1]

	switch table {
	case "bot":
		return s.storeBotContext(ctx, id, data)
	case "job":
		return s.storeJobContext(ctx, id, data)
	case "crash":
		return s.storeCrashContext(ctx, id, data)
	case "coverage":
		return s.storeCoverageContext(ctx, id, data)
	case "corpus":
		return s.storeCorpusContext(ctx, id, data)
	case "assignment":
		return s.storeAssignmentContext(ctx, id, data)
	default:
		return s.storeMetadataContext(ctx, key, data)
	}
}

// getByKeyContext retrieves data based on key prefix with context
func (s *SQLiteStorage) getByKeyContext(ctx context.Context, key string) (string, error) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return s.getMetadataContext(ctx, key)
	}

	table := parts[0]
	id := parts[1]

	switch table {
	case "bot":
		return s.getBotContext(ctx, id)
	case "job":
		return s.getJobContext(ctx, id)
	case "crash":
		return s.getCrashContext(ctx, id)
	case "coverage":
		return s.getCoverageContext(ctx, id)
	case "corpus":
		return s.getCorpusContext(ctx, id)
	case "assignment":
		return s.getAssignmentContext(ctx, id)
	default:
		return s.getMetadataContext(ctx, key)
	}
}

// deleteByKeyContext deletes data based on key prefix with context
func (s *SQLiteStorage) deleteByKeyContext(ctx context.Context, key string) error {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return s.deleteMetadataContext(context.Background(), key)
	}

	table := parts[0]
	id := parts[1]

	switch table {
	case "bot":
		return s.deleteBotContext(ctx, id)
	case "job":
		return s.deleteJobContext(ctx, id)
	case "crash":
		return s.deleteCrashContext(ctx, id)
	case "coverage":
		return s.deleteCoverageContext(ctx, id)
	case "corpus":
		return s.deleteCorpusContext(ctx, id)
	case "assignment":
		return s.deleteAssignmentContext(ctx, id)
	default:
		return s.deleteMetadataContext(ctx, key)
	}
}

// Table-specific storage operations
func (s *SQLiteStorage) storeBotContext(ctx context.Context, id, data string) error {
	query := `INSERT OR REPLACE INTO bots (id, name, hostname, status, last_seen, registered_at, current_job, capabilities, timeout_at, is_online, failure_count, api_endpoint, updated_at)
			  SELECT ?, json_extract(?, '$.name'), json_extract(?, '$.hostname'), json_extract(?, '$.status'), json_extract(?, '$.last_seen'),
			         json_extract(?, '$.registered_at'), json_extract(?, '$.current_job'), json_extract(?, '$.capabilities'),
			         json_extract(?, '$.timeout_at'), json_extract(?, '$.is_online'), json_extract(?, '$.failure_count'), json_extract(?, '$.api_endpoint'), CURRENT_TIMESTAMP`

	_, err := RetryableExec(ctx, s.db, s.config, query, id, data, data, data, data, data, data, data, data, data, data, data)
	return err
}

func (s *SQLiteStorage) getBotContext(ctx context.Context, id string) (string, error) {
	query := `SELECT json_object('id', id, 'name', name, 'hostname', hostname, 'status', status,
			         'last_seen', replace(last_seen, ' ', 'T'),
			         'registered_at', replace(registered_at, ' ', 'T'),
			         'current_job', current_job, 'capabilities', json(capabilities),
			         'timeout_at', replace(timeout_at, ' ', 'T'),
			         'is_online', json(CASE WHEN is_online THEN 'true' ELSE 'false' END),
			         'failure_count', failure_count, 'api_endpoint', api_endpoint) FROM bots WHERE id = ?`

	return RetryableQueryRow(ctx, s.db, s.config, query, func(row *sql.Row) (string, error) {
		var data string
		err := row.Scan(&data)
		if err == sql.ErrNoRows {
			return "", common.ErrKeyNotFound
		}
		return data, err
	}, id)
}

func (s *SQLiteStorage) deleteBotContext(ctx context.Context, id string) error {
	_, err := RetryableExec(ctx, s.db, s.config, "DELETE FROM bots WHERE id = ?", id)
	return err
}

func (s *SQLiteStorage) storeJobContext(ctx context.Context, id, data string) error {
	query := `INSERT OR REPLACE INTO jobs (id, name, target, fuzzer, status, assigned_bot, created_at, started_at, completed_at, timeout_at, work_dir, config, collection_id, campaign_id, use_campaign_corpus, enable_coverage, coverage_format, updated_at)
			  SELECT ?, json_extract(?, '$.name'), json_extract(?, '$.target'), json_extract(?, '$.fuzzer'),
			         json_extract(?, '$.status'), json_extract(?, '$.assigned_bot'), json_extract(?, '$.created_at'),
			         json_extract(?, '$.started_at'), json_extract(?, '$.completed_at'), json_extract(?, '$.timeout_at'),
			         json_extract(?, '$.work_dir'), json_extract(?, '$.config'), json_extract(?, '$.collection_id'),
			         json_extract(?, '$.campaign_id'), json_extract(?, '$.use_campaign_corpus'),
			         json_extract(?, '$.enable_coverage'), json_extract(?, '$.coverage_format'), CURRENT_TIMESTAMP`

	_, err := RetryableExec(ctx, s.db, s.config, query, id, data, data, data, data, data, data, data, data, data, data, data, data, data, data, data, data)
	return err
}

func (s *SQLiteStorage) getJobContext(ctx context.Context, id string) (string, error) {
	query := `SELECT json_object('id', id, 'name', name, 'target', target, 'fuzzer', fuzzer, 'status', status,
			         'assigned_bot', assigned_bot,
			         'created_at', replace(created_at, ' ', 'T'),
			         'started_at', CASE WHEN started_at IS NOT NULL THEN replace(started_at, ' ', 'T') ELSE NULL END,
			         'completed_at', CASE WHEN completed_at IS NOT NULL THEN replace(completed_at, ' ', 'T') ELSE NULL END,
			         'timeout_at', replace(timeout_at, ' ', 'T'),
			         'work_dir', work_dir,
			         'config', json(config), 'collection_id', collection_id, 'campaign_id', campaign_id,
			         'use_campaign_corpus', json(CASE WHEN use_campaign_corpus THEN 'true' ELSE 'false' END),
			         'enable_coverage', json(CASE WHEN enable_coverage THEN 'true' ELSE 'false' END),
			         'coverage_format', coverage_format) FROM jobs WHERE id = ?`

	return RetryableQueryRow(ctx, s.db, s.config, query, func(row *sql.Row) (string, error) {
		var data string
		err := row.Scan(&data)
		if err == sql.ErrNoRows {
			return "", common.ErrKeyNotFound
		}
		return data, err
	}, id)
}

func (s *SQLiteStorage) deleteJobContext(ctx context.Context, id string) error {
	_, err := RetryableExec(ctx, s.db, s.config, "DELETE FROM jobs WHERE id = ?", id)
	return err
}

func (s *SQLiteStorage) storeCrashContext(ctx context.Context, id, data string) error {
	query := `INSERT OR REPLACE INTO crashes (id, job_id, bot_id, hash, file_path, type, signal, exit_code, timestamp, size, is_unique, output, stack_trace)
			  SELECT ?, json_extract(?, '$.job_id'), json_extract(?, '$.bot_id'), json_extract(?, '$.hash'),
			         json_extract(?, '$.file_path'), json_extract(?, '$.type'), json_extract(?, '$.signal'),
			         json_extract(?, '$.exit_code'), json_extract(?, '$.timestamp'), json_extract(?, '$.size'),
			         json_extract(?, '$.is_unique'), json_extract(?, '$.output'), json_extract(?, '$.stack_trace')`

	_, err := s.db.ExecContext(ctx, query, id, data, data, data, data, data, data, data, data, data, data, data, data)
	return err
}

func (s *SQLiteStorage) getCrashContext(ctx context.Context, id string) (string, error) {
	query := `SELECT json_object('id', id, 'job_id', job_id, 'bot_id', bot_id, 'hash', hash, 'file_path', file_path,
			         'type', type, 'signal', signal, 'exit_code', exit_code,
			         'timestamp', replace(timestamp, ' ', 'T'),
			         'size', size, 'is_unique', is_unique, 'output', output, 'stack_trace', stack_trace) FROM crashes WHERE id = ?`

	return RetryableQueryRow(ctx, s.db, s.config, query, func(row *sql.Row) (string, error) {
		var data string
		err := row.Scan(&data)
		if err == sql.ErrNoRows {
			return "", common.ErrKeyNotFound
		}
		return data, err
	}, id)
}

func (s *SQLiteStorage) deleteCrashContext(ctx context.Context, id string) error {
	_, err := RetryableExec(ctx, s.db, s.config, "DELETE FROM crashes WHERE id = ?", id)
	return err
}

func (s *SQLiteStorage) storeCoverageContext(ctx context.Context, id, data string) error {
	query := `INSERT OR REPLACE INTO coverage (id, job_id, bot_id, edges, new_edges, timestamp, exec_count)
			  SELECT ?, json_extract(?, '$.job_id'), json_extract(?, '$.bot_id'), json_extract(?, '$.edges'),
			         json_extract(?, '$.new_edges'), json_extract(?, '$.timestamp'), json_extract(?, '$.exec_count')`

	_, err := s.db.ExecContext(ctx, query, id, data, data, data, data, data, data)
	return err
}

func (s *SQLiteStorage) getCoverageContext(ctx context.Context, id string) (string, error) {
	query := `SELECT json_object('id', id, 'job_id', job_id, 'bot_id', bot_id, 'edges', edges,
			         'new_edges', new_edges,
			         'timestamp', replace(timestamp, ' ', 'T'),
			         'exec_count', exec_count) FROM coverage WHERE id = ?`

	return RetryableQueryRow(ctx, s.db, s.config, query, func(row *sql.Row) (string, error) {
		var data string
		err := row.Scan(&data)
		if err == sql.ErrNoRows {
			return "", common.ErrKeyNotFound
		}
		return data, err
	}, id)
}

func (s *SQLiteStorage) deleteCoverageContext(ctx context.Context, id string) error {
	_, err := RetryableExec(ctx, s.db, s.config, "DELETE FROM coverage WHERE id = ?", id)
	return err
}

func (s *SQLiteStorage) storeCorpusContext(ctx context.Context, id, data string) error {
	query := `INSERT OR REPLACE INTO corpus_updates (id, job_id, bot_id, files, timestamp, total_size)
			  SELECT ?, json_extract(?, '$.job_id'), json_extract(?, '$.bot_id'), json_extract(?, '$.files'),
			         json_extract(?, '$.timestamp'), json_extract(?, '$.total_size')`

	_, err := s.db.ExecContext(ctx, query, id, data, data, data, data, data)
	return err
}

func (s *SQLiteStorage) getCorpusContext(ctx context.Context, id string) (string, error) {
	query := `SELECT json_object('id', id, 'job_id', job_id, 'bot_id', bot_id, 'files', json(files),
			         'timestamp', timestamp, 'total_size', total_size) FROM corpus_updates WHERE id = ?`

	return RetryableQueryRow(ctx, s.db, s.config, query, func(row *sql.Row) (string, error) {
		var data string
		err := row.Scan(&data)
		if err == sql.ErrNoRows {
			return "", common.ErrKeyNotFound
		}
		return data, err
	}, id)
}

func (s *SQLiteStorage) deleteCorpusContext(ctx context.Context, id string) error {
	_, err := RetryableExec(ctx, s.db, s.config, "DELETE FROM corpus_updates WHERE id = ?", id)
	return err
}

func (s *SQLiteStorage) storeAssignmentContext(ctx context.Context, id, data string) error {
	query := `INSERT OR REPLACE INTO job_assignments (job_id, bot_id, timestamp, status)
			  SELECT ?, json_extract(?, '$.bot_id'), json_extract(?, '$.timestamp'), json_extract(?, '$.status')`

	_, err := s.db.ExecContext(ctx, query, id, data, data, data)
	return err
}

func (s *SQLiteStorage) getAssignmentContext(ctx context.Context, id string) (string, error) {
	query := `SELECT json_object('job_id', job_id, 'bot_id', bot_id, 'timestamp', timestamp, 'status', status)
			  FROM job_assignments WHERE job_id = ?`

	return RetryableQueryRow(ctx, s.db, s.config, query, func(row *sql.Row) (string, error) {
		var data string
		err := row.Scan(&data)
		if err == sql.ErrNoRows {
			return "", common.ErrKeyNotFound
		}
		return data, err
	}, id)
}

func (s *SQLiteStorage) deleteAssignmentContext(ctx context.Context, id string) error {
	_, err := RetryableExec(ctx, s.db, s.config, "DELETE FROM job_assignments WHERE job_id = ?", id)
	return err
}

func (s *SQLiteStorage) storeMetadataContext(ctx context.Context, key, data string) error {
	_, err := RetryableExec(ctx, s.db, s.config, "INSERT OR REPLACE INTO metadata (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)", key, data)
	return err
}

func (s *SQLiteStorage) getMetadataContext(ctx context.Context, key string) (string, error) {
	return RetryableQueryRow(ctx, s.db, s.config, "SELECT value FROM metadata WHERE key = ?", func(row *sql.Row) (string, error) {
		var data string
		err := row.Scan(&data)
		if err == sql.ErrNoRows {
			return "", common.ErrKeyNotFound
		}
		return data, err
	}, key)
}

func (s *SQLiteStorage) deleteMetadataContext(ctx context.Context, key string) error {
	_, err := RetryableExec(ctx, s.db, s.config, "DELETE FROM metadata WHERE key = ?", key)
	return err
}

func (s *SQLiteStorage) getTotalKeysContext(ctx context.Context) (int64, error) {
	query := `
		SELECT
			(SELECT COUNT(*) FROM bots) +
			(SELECT COUNT(*) FROM jobs) +
			(SELECT COUNT(*) FROM crashes) +
			(SELECT COUNT(*) FROM coverage) +
			(SELECT COUNT(*) FROM corpus_updates) +
			(SELECT COUNT(*) FROM job_assignments) +
			(SELECT COUNT(*) FROM metadata) as total
	`

	s.logger.Debug("getTotalKeysContext: Starting count query")
	start := time.Now()

	result, err := RetryableQueryRow(ctx, s.db, s.config, query, func(row *sql.Row) (int64, error) {
		var total int64
		err := row.Scan(&total)
		return total, err
	})

	duration := time.Since(start)
	if err != nil {
		s.logger.WithError(err).WithField("duration", duration).Error("getTotalKeysContext: Count query failed")
	} else {
		s.logger.WithFields(logrus.Fields{
			"duration": duration,
			"total":    result,
		}).Debug("getTotalKeysContext: Count query completed")
	}

	return result, err
}

// parseKey extracts the table name and ID from a key
func parseKey(key string) (table, id string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}
