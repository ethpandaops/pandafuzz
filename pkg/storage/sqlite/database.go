// Package sqlite provides SQLite-based storage implementations for PandaFuzz.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/database"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

// Database implements the database interfaces using SQLite.
// This is the main storage implementation that provides database connectivity
// and delegates to specialized repositories for domain operations.
type Database struct {
	db     *sql.DB
	path   string
	logger logrus.FieldLogger
	config database.Config
}

// Transaction implements database.Transaction for SQLite.
type Transaction struct {
	tx     *sql.Tx
	logger logrus.FieldLogger
	ctx    context.Context
}

// Compile-time interface compliance checks
var (
	_ database.Database    = (*Database)(nil)
	_ database.Advanced    = (*Database)(nil)
	_ database.Transaction = (*Transaction)(nil)
)

// NewDatabase creates a new SQLite database instance.
func NewDatabase(config database.Config, logger logrus.FieldLogger) (*Database, error) {
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	logger = logger.WithField("component", "sqlite_database")

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(config.Path), 0755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	// Build connection string with production settings
	connStr := config.Path + "?cache=shared&mode=rwc&_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000"

	// Add additional options if specified
	for key, value := range config.Options {
		connStr += fmt.Sprintf("&_%s=%s", key, value)
	}

	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Configure connection pool for SQLite
	maxConns := config.MaxConns
	if maxConns == 0 {
		maxConns = 3
	}
	idleConns := config.IdleConns
	if idleConns == 0 {
		idleConns = 2
	}

	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(idleConns)
	db.SetConnMaxLifetime(0)

	// Set optimal pragmas for concurrent access
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA cache_size = -64000",
	}

	ctx := context.Background()
	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("set pragma %s: %w", pragma, err)
		}
	}

	storage := &Database{
		db:     db,
		path:   config.Path,
		logger: logger,
		config: config,
	}

	logger.WithField("path", config.Path).Info("SQLite database initialized")
	return storage, nil
}

// GetDB returns the underlying SQL database connection.
func (d *Database) GetDB() *sql.DB {
	return d.db
}

// Store stores a value with the given key.
func (d *Database) Store(ctx context.Context, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal value: %w", err)
	}

	table, id := parseKey(key)
	return d.storeByKeyContext(ctx, table, id, string(data))
}

// Get retrieves a value by key.
func (d *Database) Get(ctx context.Context, key string, dest any) error {
	table, id := parseKey(key)
	data, err := d.getByKeyContext(ctx, table, id)
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(data), dest)
}

// Delete removes a value by key.
func (d *Database) Delete(ctx context.Context, key string) error {
	table, id := parseKey(key)
	return d.deleteByKeyContext(ctx, table, id)
}

// Transaction executes a function within a database transaction.
func (d *Database) Transaction(ctx context.Context, fn func(tx database.Transaction) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	sqliteTx := &Transaction{
		tx:     tx,
		logger: d.logger,
		ctx:    ctx,
	}

	if err := fn(sqliteTx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			d.logger.WithError(rbErr).Error("failed to rollback transaction")
		}
		return err
	}

	return tx.Commit()
}

// Close closes the database connection.
func (d *Database) Close(ctx context.Context) error {
	return d.db.Close()
}

// Ping checks if the database connection is alive.
func (d *Database) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

// Stats returns database statistics.
func (d *Database) Stats(ctx context.Context) database.Stats {
	var size int64
	if fi, err := os.Stat(d.path); err == nil {
		size = fi.Size()
	}

	var keyCount int64
	row := d.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM bots) +
			(SELECT COUNT(*) FROM jobs) +
			(SELECT COUNT(*) FROM crashes) +
			(SELECT COUNT(*) FROM coverage)
	`)
	row.Scan(&keyCount)

	return database.Stats{
		Type:        "sqlite",
		Path:        d.path,
		Size:        size,
		Keys:        keyCount,
		Connections: d.db.Stats().OpenConnections,
		IsHealthy:   d.db.PingContext(ctx) == nil,
	}
}

// Select executes a SELECT query and returns all rows.
func (d *Database) Select(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	results := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		row := make(map[string]any, len(columns))
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return results, rows.Err()
}

// SelectOne executes a SELECT query and returns one row.
func (d *Database) SelectOne(ctx context.Context, query string, args ...any) (map[string]any, error) {
	results, err := d.Select(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, sql.ErrNoRows
	}
	return results[0], nil
}

// Execute executes a non-SELECT query.
func (d *Database) Execute(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := d.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("execute query: %w", err)
	}
	return result.RowsAffected()
}

// BatchStore stores multiple items in a single transaction.
func (d *Database) BatchStore(ctx context.Context, items map[string]any) error {
	return d.Transaction(ctx, func(tx database.Transaction) error {
		for key, value := range items {
			if err := tx.Store(ctx, key, value); err != nil {
				return err
			}
		}
		return nil
	})
}

// BatchDelete deletes multiple items in a single transaction.
func (d *Database) BatchDelete(ctx context.Context, keys []string) error {
	return d.Transaction(ctx, func(tx database.Transaction) error {
		for _, key := range keys {
			if err := tx.Delete(ctx, key); err != nil {
				return err
			}
		}
		return nil
	})
}

// Iterate iterates over all keys with the given prefix.
func (d *Database) Iterate(ctx context.Context, prefix string, fn func(key string, value []byte) error) error {
	parts := strings.SplitN(prefix, ":", 2)
	if len(parts) == 0 {
		return fmt.Errorf("invalid prefix: %s", prefix)
	}

	table := parts[0]
	query := fmt.Sprintf("SELECT id, data FROM %s", table)
	if len(parts) > 1 && parts[1] != "" {
		query += " WHERE id LIKE ?"
	}

	var rows *sql.Rows
	var err error
	if len(parts) > 1 && parts[1] != "" {
		rows, err = d.db.QueryContext(ctx, query, parts[1]+"%")
	} else {
		rows, err = d.db.QueryContext(ctx, query)
	}
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		if err := fn(table+":"+id, []byte(data)); err != nil {
			return err
		}
	}

	return rows.Err()
}

// Backup creates a backup of the database.
func (d *Database) Backup(ctx context.Context, path string) error {
	// Ensure backup directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}

	// Checkpoint WAL
	if _, err := d.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		d.logger.WithError(err).Warn("failed to checkpoint WAL before backup")
	}

	// Copy database file
	return copyFile(d.path, path)
}

// Restore restores the database from a backup.
func (d *Database) Restore(ctx context.Context, path string) error {
	// Close current connection
	if err := d.db.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}

	// Copy backup to database path
	if err := copyFile(path, d.path); err != nil {
		return fmt.Errorf("copy backup: %w", err)
	}

	// Reopen database
	db, err := sql.Open("sqlite3", d.path+"?cache=shared&mode=rwc&_journal_mode=WAL")
	if err != nil {
		return fmt.Errorf("reopen database: %w", err)
	}

	d.db = db
	return nil
}

// CreateTables creates the database schema.
func (d *Database) CreateTables(ctx context.Context) error {
	return d.createSchema(ctx)
}

// Migrate runs database migrations up to the specified version.
func (d *Database) Migrate(ctx context.Context, version int) error {
	// Migration logic - for now just ensure tables exist
	return d.CreateTables(ctx)
}

// Vacuum optimizes the database.
func (d *Database) Vacuum(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, "VACUUM")
	return err
}

// Compact compacts the database.
func (d *Database) Compact(ctx context.Context) error {
	return d.Vacuum(ctx)
}

// Helper methods

func (d *Database) storeByKeyContext(ctx context.Context, table, id, data string) error {
	query := fmt.Sprintf(`
		INSERT INTO %s (id, data, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at
	`, table)
	now := time.Now()
	_, err := d.db.ExecContext(ctx, query, id, data, now, now)
	return err
}

func (d *Database) getByKeyContext(ctx context.Context, table, id string) (string, error) {
	query := fmt.Sprintf("SELECT data FROM %s WHERE id = ?", table)
	var data string
	err := d.db.QueryRowContext(ctx, query, id).Scan(&data)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return data, err
}

func (d *Database) deleteByKeyContext(ctx context.Context, table, id string) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = ?", table)
	_, err := d.db.ExecContext(ctx, query, id)
	return err
}

func parseKey(key string) (table, id string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "metadata", key
}

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

	if _, err := dstFile.ReadFrom(srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}

// createSchema creates the database schema.
func (d *Database) createSchema(ctx context.Context) error {
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
		capabilities TEXT,
		timeout_at DATETIME NOT NULL,
		is_online BOOLEAN DEFAULT FALSE,
		failure_count INTEGER DEFAULT 0,
		api_endpoint TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		data TEXT
	);

	-- Jobs table
	CREATE TABLE IF NOT EXISTS jobs (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		status TEXT NOT NULL,
		fuzzer TEXT NOT NULL,
		target TEXT NOT NULL,
		type TEXT,
		assigned_bot TEXT,
		created_at DATETIME NOT NULL,
		started_at DATETIME,
		completed_at DATETIME,
		timeout_at DATETIME NOT NULL,
		work_dir TEXT NOT NULL,
		config TEXT,
		progress INTEGER DEFAULT 0,
		campaign_id TEXT,
		collection_id TEXT,
		use_campaign_corpus BOOLEAN DEFAULT FALSE,
		metadata TEXT,
		priority INTEGER DEFAULT 0,
		enable_coverage BOOLEAN DEFAULT FALSE,
		coverage_format TEXT,
		lease_token TEXT,
		lease_expires_at DATETIME,
		last_heartbeat DATETIME,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		data TEXT
	);

	-- Crashes table
	CREATE TABLE IF NOT EXISTS crashes (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		bot_id TEXT,
		hash TEXT NOT NULL,
		type TEXT NOT NULL,
		output TEXT,
		stack_trace TEXT,
		timestamp DATETIME NOT NULL,
		size INTEGER DEFAULT 0,
		reproducible BOOLEAN DEFAULT FALSE,
		crash_group_id TEXT,
		campaign_id TEXT,
		is_unique BOOLEAN DEFAULT TRUE,
		dedup_hash TEXT,
		repro_score REAL DEFAULT 0.0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		data TEXT
	);

	-- Crash inputs table
	CREATE TABLE IF NOT EXISTS crash_inputs (
		crash_id TEXT PRIMARY KEY,
		input BLOB,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (crash_id) REFERENCES crashes(id) ON DELETE CASCADE
	);

	-- Coverage table
	CREATE TABLE IF NOT EXISTS coverage (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		bot_id TEXT,
		edges INTEGER DEFAULT 0,
		new_edges INTEGER DEFAULT 0,
		exec_count INTEGER DEFAULT 0,
		timestamp DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		data TEXT
	);

	-- Corpus table
	CREATE TABLE IF NOT EXISTS corpus (
		id TEXT PRIMARY KEY,
		campaign_id TEXT,
		job_id TEXT,
		hash TEXT NOT NULL,
		size INTEGER DEFAULT 0,
		coverage INTEGER DEFAULT 0,
		timestamp DATETIME NOT NULL,
		source TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		data TEXT
	);

	-- Metadata table
	CREATE TABLE IF NOT EXISTS metadata (
		id TEXT PRIMARY KEY,
		data TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Campaigns table
	CREATE TABLE IF NOT EXISTS campaigns (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		status TEXT NOT NULL,
		target TEXT NOT NULL,
		fuzzer TEXT NOT NULL,
		config TEXT,
		created_at DATETIME NOT NULL,
		started_at DATETIME,
		completed_at DATETIME,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		data TEXT
	);

	-- Create indexes
	CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
	CREATE INDEX IF NOT EXISTS idx_jobs_campaign_id ON jobs(campaign_id);
	CREATE INDEX IF NOT EXISTS idx_crashes_job_id ON crashes(job_id);
	CREATE INDEX IF NOT EXISTS idx_crashes_hash ON crashes(hash);
	CREATE INDEX IF NOT EXISTS idx_coverage_job_id ON coverage(job_id);
	CREATE INDEX IF NOT EXISTS idx_corpus_campaign_id ON corpus(campaign_id);
	CREATE INDEX IF NOT EXISTS idx_bots_status ON bots(status);
	`

	_, err := d.db.ExecContext(ctx, schema)
	return err
}

// Transaction methods

// Store stores a value within the transaction.
func (tx *Transaction) Store(ctx context.Context, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal value: %w", err)
	}

	table, id := parseKey(key)
	query := fmt.Sprintf(`
		INSERT INTO %s (id, data, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at
	`, table)
	now := time.Now()
	_, err = tx.tx.ExecContext(ctx, query, id, string(data), now, now)
	return err
}

// Get retrieves a value within the transaction.
func (tx *Transaction) Get(ctx context.Context, key string, dest any) error {
	table, id := parseKey(key)
	query := fmt.Sprintf("SELECT data FROM %s WHERE id = ?", table)
	var data string
	err := tx.tx.QueryRowContext(ctx, query, id).Scan(&data)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(data), dest)
}

// Delete removes a value within the transaction.
func (tx *Transaction) Delete(ctx context.Context, key string) error {
	table, id := parseKey(key)
	query := fmt.Sprintf("DELETE FROM %s WHERE id = ?", table)
	_, err := tx.tx.ExecContext(ctx, query, id)
	return err
}

// Commit commits the transaction.
func (tx *Transaction) Commit(ctx context.Context) error {
	return tx.tx.Commit()
}

// Rollback rolls back the transaction.
func (tx *Transaction) Rollback(ctx context.Context) error {
	return tx.tx.Rollback()
}
