package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

// SQLiteStorage implements the Database interface using SQLite
type SQLiteStorage struct {
	db     *sql.DB
	path   string
	mu     sync.RWMutex
	logger *logrus.Logger
	config common.DatabaseConfig
}

// SQLiteTransaction implements the Transaction interface
type SQLiteTransaction struct {
	tx     *sql.Tx
	logger *logrus.Logger
}

// NewSQLiteStorage creates a new SQLite storage instance
func NewSQLiteStorage(config common.DatabaseConfig) (common.AdvancedDatabase, error) {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

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
	db.SetMaxOpenConns(1) // SQLite limitation
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	storage := &SQLiteStorage{
		db:     db,
		path:   config.Path,
		logger: logger,
		config: config,
	}

	// Initialize database schema
	if err := storage.createTables(); err != nil {
		db.Close()
		return nil, common.NewDatabaseError("create_tables", err)
	}

	// Apply migrations for normalized schema
	if err := MigrateExistingData(db); err != nil {
		db.Close()
		return nil, common.NewDatabaseError("apply_migrations", err)
	}

	logger.WithField("path", config.Path).Info("SQLite storage initialized with normalized schema")
	return storage, nil
}

// createTables initializes the database schema
func (s *SQLiteStorage) createTables() error {
	schema := `
	-- Bots table
	CREATE TABLE IF NOT EXISTS bots (
		id TEXT PRIMARY KEY,
		hostname TEXT NOT NULL,
		status TEXT NOT NULL,
		last_seen DATETIME NOT NULL,
		registered_at DATETIME NOT NULL,
		current_job TEXT,
		capabilities TEXT, -- JSON array
		timeout_at DATETIME NOT NULL,
		is_online BOOLEAN DEFAULT FALSE,
		failure_count INTEGER DEFAULT 0,
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

	-- Create indexes for performance
	CREATE INDEX IF NOT EXISTS idx_bots_status ON bots(status);
	CREATE INDEX IF NOT EXISTS idx_bots_timeout ON bots(timeout_at);
	CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
	CREATE INDEX IF NOT EXISTS idx_jobs_assigned_bot ON jobs(assigned_bot);
	CREATE INDEX IF NOT EXISTS idx_jobs_timeout ON jobs(timeout_at);
	CREATE INDEX IF NOT EXISTS idx_crashes_job_id ON crashes(job_id);
	CREATE INDEX IF NOT EXISTS idx_crashes_hash ON crashes(hash);
	CREATE INDEX IF NOT EXISTS idx_coverage_job_id ON coverage(job_id);
	CREATE INDEX IF NOT EXISTS idx_corpus_job_id ON corpus_updates(job_id);
	`

	_, err := s.db.Exec(schema)
	if err != nil {
		return common.NewDatabaseError("create_schema", err)
	}

	return nil
}

// Store implements the Database interface
func (s *SQLiteStorage) Store(key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Convert value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		return common.NewDatabaseError("marshal_value", err)
	}

	// Determine table and perform operation based on key prefix
	return s.storeByKey(key, string(data))
}

// Get implements the Database interface
func (s *SQLiteStorage) Get(key string, dest interface{}) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := s.getByKey(key)
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
func (s *SQLiteStorage) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.deleteByKey(key)
}

// Transaction implements the Database interface
func (s *SQLiteStorage) Transaction(fn func(tx common.Transaction) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sqlTx, err := s.db.Begin()
	if err != nil {
		return common.NewDatabaseError("begin_transaction", err)
	}

	tx := &SQLiteTransaction{
		tx:     sqlTx,
		logger: s.logger,
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
}

// Close implements the Database interface
func (s *SQLiteStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		err := s.db.Close()
		s.db = nil
		if err != nil {
			return common.NewDatabaseError("close_database", err)
		}
	}
	return nil
}

// Ping implements the Database interface
func (s *SQLiteStorage) Ping() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil {
		return common.ErrDatabaseClosed
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.db.PingContext(ctx); err != nil {
		return common.NewDatabaseError("ping", err)
	}

	return nil
}

// Stats implements the Database interface
func (s *SQLiteStorage) Stats() common.DatabaseStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := common.DatabaseStats{
		Type:        "sqlite",
		Path:        s.path,
		IsHealthy:   true,
	}

	// Get database file size
	if fileInfo, err := os.Stat(s.path); err == nil {
		stats.Size = fileInfo.Size()
	}

	// Get connection stats
	if s.db != nil {
		dbStats := s.db.Stats()
		stats.Connections = dbStats.OpenConnections
	}

	// Count total keys (tables)
	if keyCount, err := s.getTotalKeys(); err == nil {
		stats.Keys = keyCount
	}

	return stats
}

// storeByKey stores data based on key prefix
func (s *SQLiteStorage) storeByKey(key, data string) error {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return s.storeMetadata(key, data)
	}

	table := parts[0]
	id := parts[1]

	switch table {
	case "bot":
		return s.storeBot(id, data)
	case "job":
		return s.storeJob(id, data)
	case "crash":
		return s.storeCrash(id, data)
	case "coverage":
		return s.storeCoverage(id, data)
	case "corpus":
		return s.storeCorpus(id, data)
	case "assignment":
		return s.storeAssignment(id, data)
	default:
		return s.storeMetadata(key, data)
	}
}

// getByKey retrieves data based on key prefix
func (s *SQLiteStorage) getByKey(key string) (string, error) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return s.getMetadata(key)
	}

	table := parts[0]
	id := parts[1]

	switch table {
	case "bot":
		return s.getBot(id)
	case "job":
		return s.getJob(id)
	case "crash":
		return s.getCrash(id)
	case "coverage":
		return s.getCoverage(id)
	case "corpus":
		return s.getCorpus(id)
	case "assignment":
		return s.getAssignment(id)
	default:
		return s.getMetadata(key)
	}
}

// deleteByKey deletes data based on key prefix
func (s *SQLiteStorage) deleteByKey(key string) error {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return s.deleteMetadata(key)
	}

	table := parts[0]
	id := parts[1]

	switch table {
	case "bot":
		return s.deleteBot(id)
	case "job":
		return s.deleteJob(id)
	case "crash":
		return s.deleteCrash(id)
	case "coverage":
		return s.deleteCoverage(id)
	case "corpus":
		return s.deleteCorpus(id)
	case "assignment":
		return s.deleteAssignment(id)
	default:
		return s.deleteMetadata(key)
	}
}

// Table-specific operations
func (s *SQLiteStorage) storeBot(id, data string) error {
	query := `INSERT OR REPLACE INTO bots (id, hostname, status, last_seen, registered_at, current_job, capabilities, timeout_at, is_online, failure_count, updated_at) 
			  SELECT ?, json_extract(?, '$.hostname'), json_extract(?, '$.status'), json_extract(?, '$.last_seen'), 
			         json_extract(?, '$.registered_at'), json_extract(?, '$.current_job'), json_extract(?, '$.capabilities'),
			         json_extract(?, '$.timeout_at'), json_extract(?, '$.is_online'), json_extract(?, '$.failure_count'), CURRENT_TIMESTAMP`
	
	_, err := s.db.Exec(query, id, data, data, data, data, data, data, data, data, data)
	return err
}

func (s *SQLiteStorage) getBot(id string) (string, error) {
	query := `SELECT json_object('id', id, 'hostname', hostname, 'status', status, 'last_seen', last_seen,
			         'registered_at', registered_at, 'current_job', current_job, 'capabilities', json(capabilities),
			         'timeout_at', timeout_at, 'is_online', is_online, 'failure_count', failure_count) FROM bots WHERE id = ?`
	
	var data string
	err := s.db.QueryRow(query, id).Scan(&data)
	if err == sql.ErrNoRows {
		return "", common.ErrKeyNotFound
	}
	return data, err
}

func (s *SQLiteStorage) deleteBot(id string) error {
	_, err := s.db.Exec("DELETE FROM bots WHERE id = ?", id)
	return err
}

func (s *SQLiteStorage) storeJob(id, data string) error {
	query := `INSERT OR REPLACE INTO jobs (id, name, target, fuzzer, status, assigned_bot, created_at, started_at, completed_at, timeout_at, work_dir, config, updated_at)
			  SELECT ?, json_extract(?, '$.name'), json_extract(?, '$.target'), json_extract(?, '$.fuzzer'),
			         json_extract(?, '$.status'), json_extract(?, '$.assigned_bot'), json_extract(?, '$.created_at'),
			         json_extract(?, '$.started_at'), json_extract(?, '$.completed_at'), json_extract(?, '$.timeout_at'),
			         json_extract(?, '$.work_dir'), json_extract(?, '$.config'), CURRENT_TIMESTAMP`
	
	_, err := s.db.Exec(query, id, data, data, data, data, data, data, data, data, data, data, data)
	return err
}

func (s *SQLiteStorage) getJob(id string) (string, error) {
	query := `SELECT json_object('id', id, 'name', name, 'target', target, 'fuzzer', fuzzer, 'status', status,
			         'assigned_bot', assigned_bot, 'created_at', created_at, 'started_at', started_at,
			         'completed_at', completed_at, 'timeout_at', timeout_at, 'work_dir', work_dir,
			         'config', json(config)) FROM jobs WHERE id = ?`
	
	var data string
	err := s.db.QueryRow(query, id).Scan(&data)
	if err == sql.ErrNoRows {
		return "", common.ErrKeyNotFound
	}
	return data, err
}

func (s *SQLiteStorage) deleteJob(id string) error {
	_, err := s.db.Exec("DELETE FROM jobs WHERE id = ?", id)
	return err
}

func (s *SQLiteStorage) storeCrash(id, data string) error {
	query := `INSERT OR REPLACE INTO crashes (id, job_id, bot_id, hash, file_path, type, signal, exit_code, timestamp, size, is_unique)
			  SELECT ?, json_extract(?, '$.job_id'), json_extract(?, '$.bot_id'), json_extract(?, '$.hash'),
			         json_extract(?, '$.file_path'), json_extract(?, '$.type'), json_extract(?, '$.signal'),
			         json_extract(?, '$.exit_code'), json_extract(?, '$.timestamp'), json_extract(?, '$.size'),
			         json_extract(?, '$.is_unique')`
	
	_, err := s.db.Exec(query, id, data, data, data, data, data, data, data, data, data, data)
	return err
}

func (s *SQLiteStorage) getCrash(id string) (string, error) {
	query := `SELECT json_object('id', id, 'job_id', job_id, 'bot_id', bot_id, 'hash', hash, 'file_path', file_path,
			         'type', type, 'signal', signal, 'exit_code', exit_code, 'timestamp', timestamp,
			         'size', size, 'is_unique', is_unique) FROM crashes WHERE id = ?`
	
	var data string
	err := s.db.QueryRow(query, id).Scan(&data)
	if err == sql.ErrNoRows {
		return "", common.ErrKeyNotFound
	}
	return data, err
}

func (s *SQLiteStorage) deleteCrash(id string) error {
	_, err := s.db.Exec("DELETE FROM crashes WHERE id = ?", id)
	return err
}

func (s *SQLiteStorage) storeCoverage(id, data string) error {
	query := `INSERT OR REPLACE INTO coverage (id, job_id, bot_id, edges, new_edges, timestamp, exec_count)
			  SELECT ?, json_extract(?, '$.job_id'), json_extract(?, '$.bot_id'), json_extract(?, '$.edges'),
			         json_extract(?, '$.new_edges'), json_extract(?, '$.timestamp'), json_extract(?, '$.exec_count')`
	
	_, err := s.db.Exec(query, id, data, data, data, data, data, data)
	return err
}

func (s *SQLiteStorage) getCoverage(id string) (string, error) {
	query := `SELECT json_object('id', id, 'job_id', job_id, 'bot_id', bot_id, 'edges', edges,
			         'new_edges', new_edges, 'timestamp', timestamp, 'exec_count', exec_count) FROM coverage WHERE id = ?`
	
	var data string
	err := s.db.QueryRow(query, id).Scan(&data)
	if err == sql.ErrNoRows {
		return "", common.ErrKeyNotFound
	}
	return data, err
}

func (s *SQLiteStorage) deleteCoverage(id string) error {
	_, err := s.db.Exec("DELETE FROM coverage WHERE id = ?", id)
	return err
}

func (s *SQLiteStorage) storeCorpus(id, data string) error {
	query := `INSERT OR REPLACE INTO corpus_updates (id, job_id, bot_id, files, timestamp, total_size)
			  SELECT ?, json_extract(?, '$.job_id'), json_extract(?, '$.bot_id'), json_extract(?, '$.files'),
			         json_extract(?, '$.timestamp'), json_extract(?, '$.total_size')`
	
	_, err := s.db.Exec(query, id, data, data, data, data, data)
	return err
}

func (s *SQLiteStorage) getCorpus(id string) (string, error) {
	query := `SELECT json_object('id', id, 'job_id', job_id, 'bot_id', bot_id, 'files', json(files),
			         'timestamp', timestamp, 'total_size', total_size) FROM corpus_updates WHERE id = ?`
	
	var data string
	err := s.db.QueryRow(query, id).Scan(&data)
	if err == sql.ErrNoRows {
		return "", common.ErrKeyNotFound
	}
	return data, err
}

func (s *SQLiteStorage) deleteCorpus(id string) error {
	_, err := s.db.Exec("DELETE FROM corpus_updates WHERE id = ?", id)
	return err
}

func (s *SQLiteStorage) storeAssignment(id, data string) error {
	query := `INSERT OR REPLACE INTO job_assignments (job_id, bot_id, timestamp, status)
			  SELECT ?, json_extract(?, '$.bot_id'), json_extract(?, '$.timestamp'), json_extract(?, '$.status')`
	
	_, err := s.db.Exec(query, id, data, data, data)
	return err
}

func (s *SQLiteStorage) getAssignment(id string) (string, error) {
	query := `SELECT json_object('job_id', job_id, 'bot_id', bot_id, 'timestamp', timestamp, 'status', status) 
			  FROM job_assignments WHERE job_id = ?`
	
	var data string
	err := s.db.QueryRow(query, id).Scan(&data)
	if err == sql.ErrNoRows {
		return "", common.ErrKeyNotFound
	}
	return data, err
}

func (s *SQLiteStorage) deleteAssignment(id string) error {
	_, err := s.db.Exec("DELETE FROM job_assignments WHERE job_id = ?", id)
	return err
}

func (s *SQLiteStorage) storeMetadata(key, data string) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO metadata (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)", key, data)
	return err
}

func (s *SQLiteStorage) getMetadata(key string) (string, error) {
	var data string
	err := s.db.QueryRow("SELECT value FROM metadata WHERE key = ?", key).Scan(&data)
	if err == sql.ErrNoRows {
		return "", common.ErrKeyNotFound
	}
	return data, err
}

func (s *SQLiteStorage) deleteMetadata(key string) error {
	_, err := s.db.Exec("DELETE FROM metadata WHERE key = ?", key)
	return err
}

func (s *SQLiteStorage) getTotalKeys() (int64, error) {
	var total int64
	
	tables := []string{"bots", "jobs", "crashes", "coverage", "corpus_updates", "job_assignments", "metadata"}
	for _, table := range tables {
		var count int64
		err := s.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
		if err != nil {
			return 0, err
		}
		total += count
	}
	
	return total, nil
}

// SQLiteTransaction methods
func (tx *SQLiteTransaction) Store(key string, value interface{}) error {
	// For transactions, we use a simplified approach
	data, err := json.Marshal(value)
	if err != nil {
		return common.NewDatabaseError("marshal_value", err)
	}

	// Store in metadata table for simplicity in transactions
	_, err = tx.tx.Exec("INSERT OR REPLACE INTO metadata (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)", key, string(data))
	return err
}

func (tx *SQLiteTransaction) Get(key string, dest interface{}) error {
	var data string
	err := tx.tx.QueryRow("SELECT value FROM metadata WHERE key = ?", key).Scan(&data)
	if err == sql.ErrNoRows {
		return common.ErrKeyNotFound
	}
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(data), dest)
}

func (tx *SQLiteTransaction) Delete(key string) error {
	_, err := tx.tx.Exec("DELETE FROM metadata WHERE key = ?", key)
	return err
}

func (tx *SQLiteTransaction) Commit() error {
	return tx.tx.Commit()
}

func (tx *SQLiteTransaction) Rollback() error {
	return tx.tx.Rollback()
}

// GetAllJobs retrieves all jobs from the database
func (s *SQLiteStorage) GetAllJobs() ([]map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, name, target, fuzzer, status, assigned_bot, created_at, 
	          started_at, completed_at, timeout_at, work_dir, config 
	          FROM jobs`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, common.NewDatabaseError("query_jobs", err)
	}
	defer rows.Close()

	var jobs []map[string]interface{}
	for rows.Next() {
		var id, name, target, fuzzer, status, workDir string
		var assignedBot, config sql.NullString
		var createdAt, timeoutAt time.Time
		var startedAt, completedAt sql.NullTime

		err := rows.Scan(&id, &name, &target, &fuzzer, &status, &assignedBot,
			&createdAt, &startedAt, &completedAt, &timeoutAt, &workDir, &config)
		if err != nil {
			s.logger.WithError(err).Warn("Failed to scan job row")
			continue
		}

		job := map[string]interface{}{
			"id":         id,
			"name":       name,
			"target":     target,
			"fuzzer":     fuzzer,
			"status":     status,
			"created_at": createdAt,
			"timeout_at": timeoutAt,
			"work_dir":   workDir,
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

		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

// Iterate implements iteration over keys with a given prefix
func (s *SQLiteStorage) Iterate(prefix string, fn func(key string, value []byte) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Determine which table to query based on prefix
	var query string
	switch prefix {
	case "job:":
		query = `SELECT id, json_object('id', id, 'name', name, 'target', target, 'fuzzer', fuzzer, 
		         'status', status, 'assigned_bot', assigned_bot, 'created_at', created_at, 
		         'started_at', started_at, 'completed_at', completed_at, 'timeout_at', timeout_at, 
		         'work_dir', work_dir, 'config', json(config)) FROM jobs`
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
		rows, err := s.db.Query(query)
		if err != nil {
			return common.NewDatabaseError("iterate_query", err)
		}
		defer rows.Close()

		for rows.Next() {
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
		rows, err := s.db.Query(query, prefix)
		if err != nil {
			return common.NewDatabaseError("iterate_metadata", err)
		}
		defer rows.Close()

		for rows.Next() {
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
}

// AdvancedDatabase interface implementation

// Select implements Query interface
func (s *SQLiteStorage) Select(query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Get column names
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for rows.Next() {
		// Create a slice of interface{} to hold column values
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		// Create map for this row
		row := make(map[string]interface{})
		for i, col := range cols {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return results, rows.Err()
}

// SelectOne implements Query interface
func (s *SQLiteStorage) SelectOne(query string, args ...interface{}) (map[string]interface{}, error) {
	results, err := s.Select(query, args...)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, sql.ErrNoRows
	}
	return results[0], nil
}

// Execute implements Query interface
func (s *SQLiteStorage) Execute(query string, args ...interface{}) (int64, error) {
	result, err := s.db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// SelectContext implements Query interface
func (s *SQLiteStorage) SelectContext(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error) {
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

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]interface{})
		for i, col := range cols {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return results, rows.Err()
}

// ExecuteContext implements Query interface
func (s *SQLiteStorage) ExecuteContext(ctx context.Context, query string, args ...interface{}) (int64, error) {
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// BatchStore implements batch storage operations
func (s *SQLiteStorage) BatchStore(items map[string]interface{}) error {
	return s.Transaction(func(tx common.Transaction) error {
		for key, value := range items {
			if err := tx.Store(key, value); err != nil {
				return err
			}
		}
		return nil
	})
}

// BatchDelete implements batch delete operations
func (s *SQLiteStorage) BatchDelete(keys []string) error {
	return s.Transaction(func(tx common.Transaction) error {
		for _, key := range keys {
			if err := tx.Delete(key); err != nil {
				return err
			}
		}
		return nil
	})
}

// CreateTables is already implemented in createTables
func (s *SQLiteStorage) CreateTables() error {
	return s.createTables()
}

// Migrate implements database migrations
func (s *SQLiteStorage) Migrate(version int) error {
	// For now, just ensure tables exist
	return s.createTables()
}

// Backup creates a backup of the database
func (s *SQLiteStorage) Backup(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure backup directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Use SQLite backup API
	query := fmt.Sprintf("VACUUM INTO '%s'", path)
	_, err := s.db.Exec(query)
	if err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	s.logger.WithField("backup_path", path).Info("Database backup completed")
	return nil
}

// Restore restores database from a backup
func (s *SQLiteStorage) Restore(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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
func (s *SQLiteStorage) Vacuum() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("VACUUM")
	return err
}

// Compact is an alias for Vacuum in SQLite
func (s *SQLiteStorage) Compact() error {
	return s.Vacuum()
}