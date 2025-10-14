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

// SQLiteTransaction implements the Transaction interface
type SQLiteTransaction struct {
	tx     *sql.Tx
	logger *logrus.Logger
	ctx    context.Context
}

// Compile-time interface compliance check
var _ common.Transaction = (*SQLiteTransaction)(nil)

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

// createTablesContext initializes the database schema with context
func (s *SQLiteStorage) createTablesContext(ctx context.Context) error {
	schema := `
	-- Bots table
	CREATE TABLE IF NOT EXISTS bots (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		hostname TEXT NOT NULL,
		status TEXT NOT NULL,
		last_seen DATETIME NOT NULL,
		registered_at DATETIME NOT NULL,
		current_job TEXT,
		capabilities TEXT, -- JSON array
		timeout_at DATETIME NOT NULL,
		is_online BOOLEAN DEFAULT FALSE,
		failure_count INTEGER DEFAULT 0,
		api_endpoint TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Jobs table
	CREATE TABLE IF NOT EXISTS jobs (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		target TEXT NOT NULL,
		fuzzer TEXT NOT NULL,
		status TEXT NOT NULL,
		assigned_bot TEXT,
		created_at DATETIME NOT NULL,
		started_at DATETIME,
		completed_at DATETIME,
		timeout_at DATETIME NOT NULL,
		work_dir TEXT NOT NULL,
		config TEXT, -- JSON object
		progress INTEGER DEFAULT 0,
		campaign_id TEXT,
		use_campaign_corpus INTEGER DEFAULT 0,
		collection_id VARCHAR(255),
		enable_coverage BOOLEAN DEFAULT FALSE,
		coverage_format TEXT,
		coverage_report_id TEXT,
		lease_token VARCHAR(64),
		lease_expires_at DATETIME,
		last_heartbeat DATETIME,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(assigned_bot) REFERENCES bots(id)
	);

	-- Crash results
	CREATE TABLE IF NOT EXISTS crashes (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		bot_id TEXT NOT NULL,
		hash TEXT NOT NULL,
		file_path TEXT NOT NULL,
		type TEXT NOT NULL,
		signal INTEGER,
		exit_code INTEGER,
		timestamp DATETIME NOT NULL,
		size INTEGER,
		is_unique BOOLEAN DEFAULT TRUE,
		output TEXT,
		stack_trace TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(job_id) REFERENCES jobs(id),
		FOREIGN KEY(bot_id) REFERENCES bots(id)
	);

	-- Coverage results
	CREATE TABLE IF NOT EXISTS coverage (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		bot_id TEXT NOT NULL,
		edges INTEGER NOT NULL,
		new_edges INTEGER NOT NULL,
		timestamp DATETIME NOT NULL,
		exec_count INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(job_id) REFERENCES jobs(id),
		FOREIGN KEY(bot_id) REFERENCES bots(id)
	);

	-- Corpus updates
	CREATE TABLE IF NOT EXISTS corpus_updates (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		bot_id TEXT NOT NULL,
		files TEXT NOT NULL, -- JSON array
		timestamp DATETIME NOT NULL,
		total_size INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(job_id) REFERENCES jobs(id),
		FOREIGN KEY(bot_id) REFERENCES bots(id)
	);

	-- Job assignments (for atomic operations)
	CREATE TABLE IF NOT EXISTS job_assignments (
		job_id TEXT PRIMARY KEY,
		bot_id TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		status TEXT NOT NULL, -- "assigned", "started", "completed"
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(job_id) REFERENCES jobs(id),
		FOREIGN KEY(bot_id) REFERENCES bots(id)
	);

	-- System metadata
	CREATE TABLE IF NOT EXISTS metadata (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME NOT NULL
	);
	
	-- crash input storage (separate table for binary data)
	CREATE TABLE IF NOT EXISTS crash_inputs (
		crash_id TEXT PRIMARY KEY,
		input BLOB NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(crash_id) REFERENCES crashes(id) ON DELETE CASCADE
	);

	-- Coverage reports table
	CREATE TABLE IF NOT EXISTS coverage_reports (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		format TEXT NOT NULL,
		storage_path TEXT NOT NULL,
		size INTEGER NOT NULL,
		file_type TEXT,
		fuzzer_stats_path TEXT,
		plot_data_path TEXT,
		fuzz_bitmap_path TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(job_id) REFERENCES jobs(id) ON DELETE CASCADE
	);

	-- Coverage metadata table
	CREATE TABLE IF NOT EXISTS coverage_metadata (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		report_id TEXT NOT NULL,
		total_functions INTEGER DEFAULT 0,
		covered_functions INTEGER DEFAULT 0,
		total_lines INTEGER DEFAULT 0,
		covered_lines INTEGER DEFAULT 0,
		total_branches INTEGER DEFAULT 0,
		covered_branches INTEGER DEFAULT 0,
		function_coverage REAL DEFAULT 0.0,
		line_coverage REAL DEFAULT 0.0,
		branch_coverage REAL DEFAULT 0.0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(job_id) REFERENCES jobs(id) ON DELETE CASCADE,
		FOREIGN KEY(report_id) REFERENCES coverage_reports(id) ON DELETE CASCADE
	);

	-- Create indexes for performance
	CREATE INDEX IF NOT EXISTS idx_bots_status ON bots(status);
	CREATE INDEX IF NOT EXISTS idx_bots_timeout ON bots(timeout_at);
	CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
	CREATE INDEX IF NOT EXISTS idx_jobs_assigned_bot ON jobs(assigned_bot);
	CREATE INDEX IF NOT EXISTS idx_jobs_timeout ON jobs(timeout_at);
	CREATE INDEX IF NOT EXISTS idx_crashes_job_id ON crashes(job_id);
	CREATE INDEX IF NOT EXISTS idx_crashes_hash ON crashes(hash);
	CREATE INDEX IF NOT EXISTS idx_crashes_timestamp ON crashes(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_coverage_job_id ON coverage(job_id);
	CREATE INDEX IF NOT EXISTS idx_corpus_job_id ON corpus_updates(job_id);
	CREATE INDEX IF NOT EXISTS idx_jobs_enable_coverage ON jobs(enable_coverage);
	CREATE INDEX IF NOT EXISTS idx_jobs_coverage_report_id ON jobs(coverage_report_id);
	CREATE INDEX IF NOT EXISTS idx_coverage_reports_job_id ON coverage_reports(job_id);
	CREATE INDEX IF NOT EXISTS idx_coverage_reports_file_type ON coverage_reports(file_type);
	CREATE INDEX IF NOT EXISTS idx_coverage_metadata_job_id ON coverage_metadata(job_id);
	CREATE INDEX IF NOT EXISTS idx_coverage_metadata_report_id ON coverage_metadata(report_id);
	`

	_, err := s.db.ExecContext(ctx, schema)
	if err != nil {
		return common.NewDatabaseError("create_schema", err)
	}

	return nil
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
		// First begin a regular transaction
		sqlTx, err := s.db.BeginTx(ctx, &sql.TxOptions{
			Isolation: sql.LevelDefault,
		})
		if err != nil {
			return common.NewDatabaseError("begin_transaction", err)
		}

		// SQLite doesn't support BEGIN IMMEDIATE after BeginTx
		// The retry logic will handle any lock contention

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

// GetPool returns the connection pool for advanced usage
func (s *SQLiteStorage) GetPool() *ConnectionPool {
	return s.pool
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

// Table-specific operations
func (s *SQLiteStorage) storeBotContext(ctx context.Context, id, data string) error {
	query := `INSERT OR REPLACE INTO bots (id, name, hostname, status, last_seen, registered_at, current_job, capabilities, timeout_at, is_online, failure_count, api_endpoint, updated_at) 
			  SELECT ?, json_extract(?, '$.name'), json_extract(?, '$.hostname'), json_extract(?, '$.status'), json_extract(?, '$.last_seen'), 
			         json_extract(?, '$.registered_at'), json_extract(?, '$.current_job'), json_extract(?, '$.capabilities'),
			         json_extract(?, '$.timeout_at'), json_extract(?, '$.is_online'), json_extract(?, '$.failure_count'), json_extract(?, '$.api_endpoint'), CURRENT_TIMESTAMP`

	_, err := RetryableExec(ctx, s.db, s.config, query, id, data, data, data, data, data, data, data, data, data, data, data)
	return err
}

func (s *SQLiteStorage) getBotContext(ctx context.Context, id string) (string, error) {
	query := `SELECT json_object('id', id, 'name', name, 'hostname', hostname, 'status', status, 'last_seen', last_seen,
			         'registered_at', registered_at, 'current_job', current_job, 'capabilities', json(capabilities),
			         'timeout_at', timeout_at, 'is_online', json(CASE WHEN is_online THEN 'true' ELSE 'false' END), 'failure_count', failure_count, 'api_endpoint', api_endpoint) FROM bots WHERE id = ?`

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
			         'assigned_bot', assigned_bot, 'created_at', created_at, 'started_at', started_at,
			         'completed_at', completed_at, 'timeout_at', timeout_at, 'work_dir', work_dir,
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
			         'type', type, 'signal', signal, 'exit_code', exit_code, 'timestamp', timestamp,
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
			         'new_edges', new_edges, 'timestamp', timestamp, 'exec_count', exec_count) FROM coverage WHERE id = ?`

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
	// Use a single query to get all counts at once for better performance
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
// Expected format: "table:id" (e.g., "crash:abc123", "bot:bot-1")
func parseKey(key string) (table, id string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// SQLiteTransaction methods
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

func (tx *SQLiteTransaction) Delete(ctx context.Context, key string) error {
	_, err := tx.tx.ExecContext(ctx, "DELETE FROM metadata WHERE key = ?", key)
	return err
}

func (tx *SQLiteTransaction) Commit(ctx context.Context) error {
	return tx.tx.Commit()
}

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

// GetAllJobs retrieves all jobs from the database
func (s *SQLiteStorage) GetAllJobs(ctx context.Context) ([]map[string]any, error) {
	query := `SELECT id, name, target, fuzzer, status, assigned_bot, created_at, 
	          started_at, completed_at, timeout_at, work_dir, config, progress 
	          FROM jobs`

	return RetryableQuery(ctx, s.db, s.config, query, func(rows *sql.Rows) (map[string]any, error) {
		var id, name, target, fuzzer, status, workDir string
		var assignedBot, config sql.NullString
		var createdAt, timeoutAt time.Time
		var startedAt, completedAt sql.NullTime
		var progress sql.NullInt64

		err := rows.Scan(&id, &name, &target, &fuzzer, &status, &assignedBot,
			&createdAt, &startedAt, &completedAt, &timeoutAt, &workDir, &config, &progress)
		if err != nil {
			return nil, err
		}

		job := map[string]any{
			"id":         id,
			"name":       name,
			"target":     target,
			"fuzzer":     fuzzer,
			"status":     status,
			"created_at": createdAt,
			"timeout_at": timeoutAt,
			"work_dir":   workDir,
			"progress":   0, // Default to 0
		}

		if assignedBot.Valid {
			job["assigned_bot"] = assignedBot.String
		}
		if startedAt.Valid {
			job["started_at"] = startedAt.Time
		}
		if completedAt.Valid {
			job["completed_at"] = completedAt.Time
		}
		if config.Valid {
			job["config"] = config.String
		}
		if progress.Valid {
			job["progress"] = int(progress.Int64)
		}

		return job, nil
	})
}

// Iterate implements iteration over keys with a given prefix
func (s *SQLiteStorage) Iterate(ctx context.Context, prefix string, fn func(key string, value []byte) error) error {
	// Add timeout check
	if err := ctx.Err(); err != nil {
		return err
	}

	return ExecuteWithRetry(ctx, s.config, func() error {
		// Determine which table to query based on prefix
		var query string
		switch prefix {
		case "job:":
			query = `SELECT id, json_object('id', id, 'name', name, 'target', target, 'fuzzer', fuzzer, 
			         'status', status, 'assigned_bot', assigned_bot, 'created_at', created_at, 
			         'started_at', started_at, 'completed_at', completed_at, 'timeout_at', timeout_at, 
			         'work_dir', work_dir, 'config', json(config), 'progress', progress) FROM jobs`
		case "bot:":
			query = `SELECT id, json_object('id', id, 'hostname', hostname, 'status', status, 
			         'last_seen', last_seen, 'registered_at', registered_at, 'current_job', current_job, 
			         'capabilities', json(capabilities), 'timeout_at', timeout_at, 'is_online', is_online, 
			         'failure_count', failure_count) FROM bots`
		default:
			// For metadata table
			query = `SELECT key, value FROM metadata WHERE key LIKE ? || '%'`
		}

		if prefix == "job:" || prefix == "bot:" {
			rows, err := s.db.QueryContext(ctx, query)
			if err != nil {
				return common.NewDatabaseError("iterate_query", err)
			}
			defer rows.Close()

			for rows.Next() {
				// Check context timeout during iteration
				if err := ctx.Err(); err != nil {
					return err
				}

				var id, data string
				if err := rows.Scan(&id, &data); err != nil {
					s.logger.WithError(err).Warn("Failed to scan row during iteration")
					continue
				}

				key := prefix + id
				if err := fn(key, []byte(data)); err != nil {
					return err
				}
			}
			return rows.Err()
		} else {
			// Query metadata table
			rows, err := s.db.QueryContext(ctx, query, prefix)
			if err != nil {
				return common.NewDatabaseError("iterate_metadata", err)
			}
			defer rows.Close()

			for rows.Next() {
				// Check context timeout during iteration
				if err := ctx.Err(); err != nil {
					return err
				}

				var key, value string
				if err := rows.Scan(&key, &value); err != nil {
					s.logger.WithError(err).Warn("Failed to scan metadata row")
					continue
				}

				if err := fn(key, []byte(value)); err != nil {
					return err
				}
			}
			return rows.Err()
		}
	})
}

// AdvancedDatabase interface implementation

// Select implements Query interface
func (s *SQLiteStorage) Select(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	return ExecuteWithRetryResult(ctx, s.config, func() ([]map[string]any, error) {
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		// Get column names
		cols, err := rows.Columns()
		if err != nil {
			return nil, err
		}

		var results []map[string]any
		for rows.Next() {
			// Check context timeout during iteration
			if err := ctx.Err(); err != nil {
				return results, err
			}

			// Create a slice of any to hold column values
			values := make([]any, len(cols))
			valuePtrs := make([]any, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				return nil, err
			}

			// Create map for this row
			row := make(map[string]any)
			for i, col := range cols {
				row[col] = values[i]
			}
			results = append(results, row)
		}

		return results, rows.Err()
	})
}

// SelectOne implements Query interface
func (s *SQLiteStorage) SelectOne(ctx context.Context, query string, args ...any) (map[string]any, error) {
	results, err := s.Select(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, sql.ErrNoRows
	}
	return results[0], nil
}

// Execute implements Query interface
func (s *SQLiteStorage) Execute(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := RetryableExec(ctx, s.db, s.config, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Note: SelectContext and ExecuteContext methods are no longer needed
// as the main Select and Execute methods now accept context

// GetCrashes retrieves crashes with pagination
func (s *SQLiteStorage) GetCrashes(ctx context.Context, limit, offset int) ([]*common.CrashResult, error) {
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		s.logger.WithError(err).Debug("Context cancelled before querying crashes")
		return nil, err
	}

	query := `SELECT id, job_id, bot_id, hash, file_path, type, signal, exit_code, timestamp, size, is_unique, output, stack_trace 
	          FROM crashes 
	          ORDER BY timestamp DESC 
	          LIMIT ? OFFSET ?`

	return RetryableQuery(ctx, s.db, s.config, query, func(rows *sql.Rows) (*common.CrashResult, error) {
		// Check context during iteration
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		crash := &common.CrashResult{}
		var output, stackTrace sql.NullString
		err := rows.Scan(&crash.ID, &crash.JobID, &crash.BotID, &crash.Hash, &crash.FilePath,
			&crash.Type, &crash.Signal, &crash.ExitCode, &crash.Timestamp, &crash.Size, &crash.IsUnique,
			&output, &stackTrace)
		if err != nil {
			return nil, err
		}
		crash.Output = output.String
		crash.StackTrace = stackTrace.String

		// Load crash input data from separate table if context not cancelled
		if ctx.Err() == nil {
			if input, err := s.GetCrashInput(ctx, crash.ID); err == nil && input != nil {
				crash.Input = input
			}
		}

		return crash, nil
	}, limit, offset)
}

// GetCrashesSorted retrieves crashes with sorting support
func (s *SQLiteStorage) GetCrashesSorted(ctx context.Context, limit, offset int, sortBy, sortOrder string) ([]*common.CrashResult, error) {
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		s.logger.WithError(err).Debug("Context cancelled before querying crashes")
		return nil, err
	}

	// Map sort field names to database columns
	columnMap := map[string]string{
		"timestamp": "timestamp",
		"type":      "type",
		"signal":    "signal",
		"size":      "size",
		"job_id":    "job_id",
		"bot_id":    "bot_id",
	}

	// Default to timestamp if invalid sort field
	sortColumn, ok := columnMap[sortBy]
	if !ok {
		sortColumn = "timestamp"
	}

	// Validate sort order
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	// Build query with dynamic ORDER BY clause
	query := fmt.Sprintf(`SELECT id, job_id, bot_id, hash, file_path, type, signal, exit_code, timestamp, size, is_unique, output, stack_trace 
	          FROM crashes 
	          ORDER BY %s %s 
	          LIMIT ? OFFSET ?`, sortColumn, sortOrder)

	return RetryableQuery(ctx, s.db, s.config, query, func(rows *sql.Rows) (*common.CrashResult, error) {
		// Check context during iteration
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		crash := &common.CrashResult{}
		var output, stackTrace sql.NullString
		err := rows.Scan(&crash.ID, &crash.JobID, &crash.BotID, &crash.Hash, &crash.FilePath,
			&crash.Type, &crash.Signal, &crash.ExitCode, &crash.Timestamp, &crash.Size, &crash.IsUnique,
			&output, &stackTrace)
		if err != nil {
			return nil, err
		}
		crash.Output = output.String
		crash.StackTrace = stackTrace.String

		// Load crash input data from separate table if context not cancelled
		if ctx.Err() == nil {
			if input, err := s.GetCrashInput(ctx, crash.ID); err == nil && input != nil {
				crash.Input = input
			}
		}

		return crash, nil
	}, limit, offset)
}

// GetCrash retrieves a specific crash by ID
func (s *SQLiteStorage) GetCrash(ctx context.Context, crashID string) (*common.CrashResult, error) {
	query := `SELECT id, job_id, bot_id, hash, file_path, type, signal, exit_code, timestamp, size, is_unique, output, stack_trace 
	          FROM crashes 
	          WHERE id = ?`

	return RetryableQueryRow(ctx, s.db, s.config, query, func(row *sql.Row) (*common.CrashResult, error) {
		crash := &common.CrashResult{}
		var output, stackTrace sql.NullString
		err := row.Scan(&crash.ID, &crash.JobID, &crash.BotID, &crash.Hash, &crash.FilePath,
			&crash.Type, &crash.Signal, &crash.ExitCode, &crash.Timestamp, &crash.Size, &crash.IsUnique,
			&output, &stackTrace)

		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}

		crash.Output = output.String
		crash.StackTrace = stackTrace.String

		// Load crash input data from separate table
		if input, err := s.GetCrashInput(ctx, crash.ID); err == nil && input != nil {
			crash.Input = input
		}

		return crash, nil
	}, crashID)
}

// GetJobCrashes retrieves all crashes for a specific job
func (s *SQLiteStorage) GetJobCrashes(ctx context.Context, jobID string) ([]*common.CrashResult, error) {
	query := `SELECT id, job_id, bot_id, hash, file_path, type, signal, exit_code, timestamp, size, is_unique, output, stack_trace 
	          FROM crashes 
	          WHERE job_id = ?
	          ORDER BY timestamp DESC`

	return RetryableQuery(ctx, s.db, s.config, query, func(rows *sql.Rows) (*common.CrashResult, error) {
		crash := &common.CrashResult{}
		var output, stackTrace sql.NullString
		err := rows.Scan(&crash.ID, &crash.JobID, &crash.BotID, &crash.Hash, &crash.FilePath,
			&crash.Type, &crash.Signal, &crash.ExitCode, &crash.Timestamp, &crash.Size, &crash.IsUnique,
			&output, &stackTrace)
		if err != nil {
			return nil, err
		}
		crash.Output = output.String
		crash.StackTrace = stackTrace.String

		// Load crash input data from separate table
		if input, err := s.GetCrashInput(ctx, crash.ID); err == nil && input != nil {
			crash.Input = input
		}

		return crash, nil
	}, jobID)
}

// StoreCrashInput stores crash input data separately
func (s *SQLiteStorage) StoreCrashInput(ctx context.Context, crashID string, input []byte) error {
	// Don't hold lock during database operation - SQLite handles its own locking

	s.logger.WithFields(logrus.Fields{
		"crash_id":   crashID,
		"input_size": len(input),
	}).Debug("Storing crash input to database")

	query := `INSERT OR REPLACE INTO crash_inputs (crash_id, input) VALUES (?, ?)`
	_, err := RetryableExec(ctx, s.db, s.config, query, crashID, input)

	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"crash_id":   crashID,
			"input_size": len(input),
		}).Error("Failed to store crash input in database")
		return err
	}

	s.logger.WithFields(logrus.Fields{
		"crash_id":   crashID,
		"input_size": len(input),
	}).Info("Successfully stored crash input in database")

	return nil
}

// GetCrashInput retrieves crash input data
func (s *SQLiteStorage) GetCrashInput(ctx context.Context, crashID string) ([]byte, error) {
	query := `SELECT input FROM crash_inputs WHERE crash_id = ?`

	return RetryableQueryRow(ctx, s.db, s.config, query, func(row *sql.Row) ([]byte, error) {
		var input []byte
		err := row.Scan(&input)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return input, nil
	}, crashID)
}

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

// CreateTables is already implemented in createTables
func (s *SQLiteStorage) CreateTables(ctx context.Context) error {
	return s.createTablesContext(ctx)
}

// Migrate implements database migrations
func (s *SQLiteStorage) Migrate(ctx context.Context, version int) error {
	// For now, just ensure tables exist
	return s.createTablesContext(ctx)
}

// Backup creates a backup of the database
func (s *SQLiteStorage) Backup(ctx context.Context, path string) error {

	// Validate backup path to prevent directory traversal
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		return fmt.Errorf("backup path must be absolute")
	}

	// Additional validation to prevent SQL injection
	// Check that path doesn't contain SQL special characters
	if strings.ContainsAny(cleanPath, "';\"") {
		return fmt.Errorf("invalid characters in backup path")
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Use SQLite backup by copying the file directly while ensuring consistency
	// This avoids SQL injection risks from VACUUM INTO
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
	// These don't have cascading deletes, so we need to clean them manually
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
	// Get page count and page size to calculate total database size
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

// scanJob scans a job from a database row
func (s *SQLiteStorage) scanJob(rows *sql.Rows) (*common.Job, error) {
	job := &common.Job{}
	var assignedBot sql.NullString
	var startedAt, completedAt sql.NullTime
	var configJSON sql.NullString
	var progress sql.NullInt64

	err := rows.Scan(
		&job.ID, &job.Name, &job.Target, &job.Fuzzer, &job.Status, &assignedBot,
		&job.CreatedAt, &startedAt, &completedAt, &job.TimeoutAt, &job.WorkDir, &configJSON, &progress)
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if assignedBot.Valid {
		job.AssignedBot = &assignedBot.String
	}
	if startedAt.Valid {
		job.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Time
	}
	if progress.Valid {
		job.Progress = int(progress.Int64)
	}

	// Parse job config JSON
	if configJSON.Valid && configJSON.String != "" {
		if err := json.Unmarshal([]byte(configJSON.String), &job.Config); err != nil {
			// Log error but don't fail - use default config
			s.logger.WithError(err).WithField("job_id", job.ID).Warn("Failed to unmarshal job config")
		}
	}

	return job, nil
}

// scanCrash scans a crash from a database row
func (s *SQLiteStorage) scanCrash(rows *sql.Rows) (*common.CrashResult, error) {
	crash := &common.CrashResult{}
	var output, stackTrace sql.NullString

	err := rows.Scan(
		&crash.ID, &crash.JobID, &crash.BotID, &crash.Hash, &crash.FilePath,
		&crash.Type, &crash.Signal, &crash.ExitCode, &crash.Timestamp, &crash.Size,
		&crash.IsUnique, &output, &stackTrace)
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if output.Valid {
		crash.Output = output.String
	}
	if stackTrace.Valid {
		crash.StackTrace = stackTrace.String
	}

	return crash, nil
}

// CreateJob creates a new job in the database
func (s *SQLiteStorage) CreateJob(ctx context.Context, job *common.Job) error {
	// Serialize job config to JSON
	configJSON, err := json.Marshal(job.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal job config: %w", err)
	}

	return ExecuteWithRetry(ctx, s.config, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO jobs (
				id, name, target, fuzzer, status, assigned_bot,
				created_at, started_at, completed_at, timeout_at, work_dir, config, progress
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, job.ID, job.Name, job.Target, job.Fuzzer, job.Status, job.AssignedBot,
			job.CreatedAt, job.StartedAt, job.CompletedAt, job.TimeoutAt, job.WorkDir,
			string(configJSON), job.Progress)
		return err
	})
}

// GetJob retrieves a job by ID
func (s *SQLiteStorage) GetJob(ctx context.Context, id string) (*common.Job, error) {
	query := `SELECT id, name, target, fuzzer, status, assigned_bot,
		created_at, started_at, completed_at, timeout_at, work_dir, config, progress,
		COALESCE(enable_coverage, 0), COALESCE(coverage_format, '')
		FROM jobs WHERE id = ?`

	var job common.Job
	var assignedBot sql.NullString
	var startedAt, completedAt sql.NullTime
	var configJSON sql.NullString
	var progress sql.NullInt64
	var enableCoverage int
	var coverageFormat string

	err := ExecuteWithRetry(ctx, s.config, func() error {
		return s.db.QueryRowContext(ctx, query, id).Scan(
			&job.ID, &job.Name, &job.Target, &job.Fuzzer, &job.Status, &assignedBot,
			&job.CreatedAt, &startedAt, &completedAt, &job.TimeoutAt, &job.WorkDir,
			&configJSON, &progress, &enableCoverage, &coverageFormat)
	})

	if err == sql.ErrNoRows {
		return nil, common.ErrKeyNotFound
	}
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if assignedBot.Valid {
		job.AssignedBot = &assignedBot.String
	}
	if startedAt.Valid {
		job.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Time
	}
	if progress.Valid {
		job.Progress = int(progress.Int64)
	}

	// Parse job config JSON
	if configJSON.Valid && configJSON.String != "" {
		if err := json.Unmarshal([]byte(configJSON.String), &job.Config); err != nil {
			// Log error but don't fail - use default config
			s.logger.WithError(err).WithField("job_id", job.ID).Warn("Failed to unmarshal job config")
		}
	}

	// Set coverage fields
	job.EnableCoverage = enableCoverage == 1
	job.CoverageFormat = coverageFormat

	return &job, nil
}

// UpdateJob updates a job with the provided fields
func (s *SQLiteStorage) UpdateJob(ctx context.Context, id string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	// Build dynamic update query
	query := "UPDATE jobs SET updated_at = CURRENT_TIMESTAMP"
	args := []interface{}{}

	for field, value := range updates {
		switch field {
		case "name", "target", "fuzzer", "status", "work_dir":
			query += fmt.Sprintf(", %s = ?", field)
			args = append(args, value)
		case "assigned_bot":
			query += ", assigned_bot = ?"
			args = append(args, value)
		case "started_at", "completed_at", "timeout_at":
			query += fmt.Sprintf(", %s = ?", field)
			args = append(args, value)
		case "progress":
			query += ", progress = ?"
			args = append(args, value)
		case "config":
			configJSON, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("failed to marshal job config: %w", err)
			}
			query += ", config = ?"
			args = append(args, string(configJSON))
		}
	}

	query += " WHERE id = ?"
	args = append(args, id)

	return ExecuteWithRetry(ctx, s.config, func() error {
		result, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}

		if rowsAffected == 0 {
			return common.ErrKeyNotFound
		}

		return nil
	})
}

// ListJobs retrieves jobs with pagination and optional status filter
func (s *SQLiteStorage) ListJobs(ctx context.Context, limit, offset int, status string) ([]*common.Job, error) {
	// Build query to get ALL non-deleted jobs
	query := `SELECT id, name, target, fuzzer, status, assigned_bot,
		created_at, started_at, completed_at, timeout_at, work_dir, config, progress
		FROM jobs
		WHERE 1=1` // Start with a true condition for easier query building

	args := []interface{}{}

	// If status filter is provided, add it to the WHERE clause
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	// Order by creation time to maintain consistent ordering
	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
		if offset > 0 {
			query += " OFFSET ?"
			args = append(args, offset)
		}
	}

	var jobs []*common.Job
	err := ExecuteWithRetry(ctx, s.config, func() error {
		rows, err := s.db.QueryContext(ctx, query, args...)
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

// DeleteJob deletes a job from the database
func (s *SQLiteStorage) DeleteJob(ctx context.Context, id string) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		result, err := s.db.ExecContext(ctx, "DELETE FROM jobs WHERE id = ?", id)
		if err != nil {
			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}

		if rowsAffected == 0 {
			return common.ErrKeyNotFound
		}

		return nil
	})
}

// CreateCrash creates a new crash result in the database
func (s *SQLiteStorage) CreateCrash(ctx context.Context, crash *common.CrashResult) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO crashes (
				id, job_id, bot_id, hash, file_path, type, signal, exit_code,
				timestamp, size, is_unique, output, stack_trace
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, crash.ID, crash.JobID, crash.BotID, crash.Hash, crash.FilePath,
			crash.Type, crash.Signal, crash.ExitCode, crash.Timestamp, crash.Size,
			crash.IsUnique, crash.Output, crash.StackTrace)
		return err
	})
}

// ListCrashes retrieves crashes for a job with pagination
func (s *SQLiteStorage) ListCrashes(ctx context.Context, jobID string, limit, offset int) ([]*common.CrashResult, error) {
	query := `SELECT id, job_id, bot_id, hash, file_path, type, signal, exit_code, 
		timestamp, size, is_unique, output, stack_trace 
		FROM crashes`
	args := []interface{}{}

	if jobID != "" {
		query += " WHERE job_id = ?"
		args = append(args, jobID)
	}

	query += " ORDER BY timestamp DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
		if offset > 0 {
			query += " OFFSET ?"
			args = append(args, offset)
		}
	}

	var crashes []*common.CrashResult
	err := ExecuteWithRetry(ctx, s.config, func() error {
		rows, err := s.db.QueryContext(ctx, query, args...)
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

// CreateCoverage creates a new coverage result
func (s *SQLiteStorage) CreateCoverage(ctx context.Context, coverage *common.CoverageResult) error {
	return ExecuteWithRetry(ctx, s.config, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO coverage (
				id, job_id, bot_id, edges, new_edges, timestamp, exec_count
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`, coverage.ID, coverage.JobID, coverage.BotID, coverage.Edges,
			coverage.NewEdges, coverage.Timestamp, coverage.ExecCount)
		return err
	})
}

// GetLatestCoverage gets the latest coverage for a job
func (s *SQLiteStorage) GetLatestCoverage(ctx context.Context, jobID string) (*common.CoverageResult, error) {
	var coverage common.CoverageResult

	err := ExecuteWithRetry(ctx, s.config, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT id, job_id, bot_id, edges, new_edges, timestamp, exec_count
			FROM coverage
			WHERE job_id = ?
			ORDER BY timestamp DESC
			LIMIT 1
		`, jobID).Scan(
			&coverage.ID, &coverage.JobID, &coverage.BotID, &coverage.Edges,
			&coverage.NewEdges, &coverage.Timestamp, &coverage.ExecCount)
	})

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &coverage, nil
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

// Cleanup performs database cleanup operations
func (s *SQLiteStorage) Cleanup(ctx context.Context) error {
	// Clean up old data based on configured retention
	// This is a placeholder - implement based on retention policy
	return nil
}

// UpdateCrashReproducibility updates crash reproducibility fields
func (s *SQLiteStorage) UpdateCrashReproducibility(ctx context.Context, crashID string, reproducible bool, score float64) error {
	query := `UPDATE crashes SET 
		reproducible = ?, 
		reproducibility_score = ?, 
		reproduction_attempts = reproduction_attempts + 1,
		last_reproduction_at = CURRENT_TIMESTAMP,
		updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`

	var reproducibleInt int
	if reproducible {
		reproducibleInt = 1
	} else {
		reproducibleInt = 0
	}

	result, err := RetryableExec(ctx, s.db, s.config, query, reproducibleInt, score, crashID)
	if err != nil {
		return common.NewDatabaseError("update_crash_reproducibility", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return common.NewDatabaseError("check_rows_affected", err)
	}

	if rowsAffected == 0 {
		return common.ErrKeyNotFound
	}

	s.logger.WithFields(logrus.Fields{
		"crash_id":     crashID,
		"reproducible": reproducible,
		"score":        score,
	}).Info("Updated crash reproducibility")

	return nil
}

// CreateReproductionResult stores a reproduction attempt result
func (s *SQLiteStorage) CreateReproductionResult(ctx context.Context, result *common.ReproductionResult) error {
	query := `INSERT INTO reproduction_results (
		id, crash_id, campaign_id, job_id, bot_id, attempt_number, 
		success, execution_time, output, stack_trace, stack_hash, 
		matches_original, test_binary_hash, corpus_used, notes
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// Determine success value (1 for reproduced, 0 for not)
	var success int
	if result.Reproduced {
		success = 1
	} else {
		success = 0
	}

	// Get campaign ID from the crash's job
	var campaignID sql.NullString
	var jobID string
	err := s.db.QueryRowContext(ctx, `
		SELECT c.job_id, j.campaign_id 
		FROM crashes c
		LEFT JOIN jobs j ON c.job_id = j.id
		WHERE c.id = ?`, result.CrashID).Scan(&jobID, &campaignID)

	if err != nil && err != sql.ErrNoRows {
		return common.NewDatabaseError("get_crash_campaign", err)
	}

	// Build notes from environment info and status
	notes := fmt.Sprintf("Status: %s", result.Status)
	if result.Signal > 0 {
		notes += fmt.Sprintf("\nSignal: %d", result.Signal)
	}
	if result.ExitCode != 0 {
		notes += fmt.Sprintf("\nExit Code: %d", result.ExitCode)
	}
	if len(result.EnvironmentInfo) > 0 {
		envJSON, _ := json.Marshal(result.EnvironmentInfo)
		notes += fmt.Sprintf("\nEnvironment: %s", string(envJSON))
	}

	// Determine corpus used based on request info
	corpusUsed := "original"
	if result.RequestID != "" {
		// This would typically be determined from the reproduction request
		corpusUsed = "campaign"
	}

	_, err = RetryableExec(ctx, s.db, s.config, query,
		result.ID, result.CrashID, campaignID, jobID, result.BotID,
		result.AttemptNumber, success, result.ExecutionTime.Milliseconds(),
		result.Output, result.StackTrace, result.StackHash,
		result.MatchesOriginal, "", corpusUsed, notes)

	if err != nil {
		return common.NewDatabaseError("create_reproduction_result", err)
	}

	// Update crash reproducibility based on result
	if result.Reproduced {
		// Calculate score based on whether stack matches original
		score := 0.5
		if result.MatchesOriginal {
			score = 1.0
		}
		if err := s.UpdateCrashReproducibility(ctx, result.CrashID, true, score); err != nil {
			s.logger.WithError(err).WithField("crash_id", result.CrashID).Warn("Failed to update crash reproducibility")
		}
	}

	s.logger.WithFields(logrus.Fields{
		"result_id":  result.ID,
		"crash_id":   result.CrashID,
		"reproduced": result.Reproduced,
		"matches":    result.MatchesOriginal,
	}).Info("Created reproduction result")

	return nil
}

// GetReproductionResults retrieves all reproduction results for a crash
func (s *SQLiteStorage) GetReproductionResults(ctx context.Context, crashID string) ([]*common.ReproductionResult, error) {
	query := `SELECT 
		id, crash_id, campaign_id, job_id, bot_id, attempt_number,
		success, execution_time, output, stack_trace, stack_hash,
		matches_original, test_binary_hash, corpus_used, notes, created_at
		FROM reproduction_results 
		WHERE crash_id = ?
		ORDER BY created_at DESC`

	return RetryableQuery(ctx, s.db, s.config, query, func(rows *sql.Rows) (*common.ReproductionResult, error) {
		result := &common.ReproductionResult{}
		var campaignID, jobID, testBinaryHash, corpusUsed, notes sql.NullString
		var execTimeMs sql.NullInt64
		var success int
		var matchesOriginal sql.NullBool

		err := rows.Scan(
			&result.ID, &result.CrashID, &campaignID, &jobID, &result.BotID,
			&result.AttemptNumber, &success, &execTimeMs, &result.Output,
			&result.StackTrace, &result.StackHash, &matchesOriginal,
			&testBinaryHash, &corpusUsed, &notes, &result.Timestamp)

		if err != nil {
			return nil, err
		}

		// Set reproduced based on success value
		result.Reproduced = (success == 1)

		// Convert execution time from milliseconds
		if execTimeMs.Valid {
			result.ExecutionTime = time.Duration(execTimeMs.Int64) * time.Millisecond
		}

		// Handle nullable fields
		if matchesOriginal.Valid {
			result.MatchesOriginal = matchesOriginal.Bool
		}

		// Parse notes to extract additional info
		if notes.Valid && notes.String != "" {
			// Extract status, signal, exit code, and environment from notes
			lines := strings.Split(notes.String, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "Status: ") {
					result.Status = common.ReproducibilityStatus(strings.TrimPrefix(line, "Status: "))
				} else if strings.HasPrefix(line, "Signal: ") {
					fmt.Sscanf(line, "Signal: %d", &result.Signal)
				} else if strings.HasPrefix(line, "Exit Code: ") {
					fmt.Sscanf(line, "Exit Code: %d", &result.ExitCode)
				} else if strings.HasPrefix(line, "Environment: ") {
					envJSON := strings.TrimPrefix(line, "Environment: ")
					if err := json.Unmarshal([]byte(envJSON), &result.EnvironmentInfo); err != nil {
						result.EnvironmentInfo = make(map[string]string)
					}
				}
			}
		}

		// Initialize environment info if not set
		if result.EnvironmentInfo == nil {
			result.EnvironmentInfo = make(map[string]string)
		}

		// Set default status if not found in notes
		if result.Status == "" {
			if result.Reproduced {
				result.Status = common.ReproducibilityStatusConfirmed
			} else {
				result.Status = common.ReproducibilityStatusFailed
			}
		}

		return result, nil
	}, crashID)
}

// GetCrashesForReproduction retrieves crashes that need reproduction testing
func (s *SQLiteStorage) GetCrashesForReproduction(ctx context.Context, limit int) ([]*common.CrashResult, error) {
	// Get crashes that:
	// - Have not been tested for reproducibility (reproducible IS NULL)
	// - Have been tested but have low confidence (reproduction_attempts < 3 AND reproducibility_score < 0.8)
	// - Haven't been tested recently (last_reproduction_at < 24 hours ago)
	query := `SELECT 
		id, job_id, bot_id, hash, file_path, type, signal, exit_code,
		timestamp, size, is_unique, output, stack_trace
		FROM crashes
		WHERE 
			(reproducible IS NULL) OR
			(reproduction_attempts < 3 AND reproducibility_score < 0.8) OR
			(last_reproduction_at < datetime('now', '-24 hours'))
		ORDER BY 
			CASE WHEN reproducible IS NULL THEN 0 ELSE 1 END,
			reproduction_attempts ASC,
			timestamp DESC
		LIMIT ?`

	return RetryableQuery(ctx, s.db, s.config, query, func(rows *sql.Rows) (*common.CrashResult, error) {
		crash := &common.CrashResult{}
		var output, stackTrace sql.NullString

		err := rows.Scan(
			&crash.ID, &crash.JobID, &crash.BotID, &crash.Hash, &crash.FilePath,
			&crash.Type, &crash.Signal, &crash.ExitCode, &crash.Timestamp,
			&crash.Size, &crash.IsUnique, &output, &stackTrace)

		if err != nil {
			return nil, err
		}

		crash.Output = output.String
		crash.StackTrace = stackTrace.String

		// Load crash input
		if input, err := s.GetCrashInput(ctx, crash.ID); err == nil && input != nil {
			crash.Input = input
		}

		return crash, nil
	}, limit)
}

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

		// Store the actual input data (this would typically be handled by file storage)
		// For now, we'll store a reference in metadata
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
				// Initialize empty tags array if JSON is empty/null
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
