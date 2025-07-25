package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite3 driver
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/errors"
)

// ConnectionConfig holds configuration for SQLite connection
type ConnectionConfig struct {
	// Database file path
	FilePath string

	// Connection pool settings
	MaxOpenConnections    int
	MaxIdleConnections    int
	ConnectionMaxLifetime time.Duration
	ConnectionMaxIdleTime time.Duration

	// Query timeouts
	DefaultQueryTimeout time.Duration

	// Enable foreign keys
	EnableForeignKeys bool

	// Enable Write-Ahead Logging
	EnableWAL bool

	// Busy timeout in milliseconds
	BusyTimeout int

	// Cache size in pages (-2000 means 2MB)
	CacheSize int
}

// DefaultConfig returns a default configuration
func DefaultConfig() ConnectionConfig {
	return ConnectionConfig{
		FilePath:              "pandafuzz.db",
		MaxOpenConnections:    25,
		MaxIdleConnections:    25,
		ConnectionMaxLifetime: 0, // No lifetime limit for SQLite
		ConnectionMaxIdleTime: 5 * time.Minute,
		DefaultQueryTimeout:   30 * time.Second,
		EnableForeignKeys:     true,
		EnableWAL:             true,
		BusyTimeout:           5000,  // 5 seconds
		CacheSize:             -2000, // 2MB cache
	}
}

// Connection represents a SQLite database connection with pooling
type Connection struct {
	db     *sql.DB
	config ConnectionConfig
	log    logrus.FieldLogger
	mu     sync.RWMutex
	closed bool
}

// NewConnection creates a new SQLite connection
func NewConnection(config ConnectionConfig, log logrus.FieldLogger) (*Connection, error) {
	if log == nil {
		log = logrus.New()
	}
	log = log.WithField("component", "sqlite_connection")

	conn := &Connection{
		config: config,
		log:    log,
	}

	if err := conn.connect(); err != nil {
		return nil, err
	}

	return conn, nil
}

// connect establishes the database connection
func (c *Connection) connect() error {
	// Build connection string with parameters
	connStr := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=%d&_cache_size=%d&_foreign_keys=%t",
		c.config.FilePath,
		c.config.BusyTimeout,
		c.config.CacheSize,
		c.config.EnableForeignKeys,
	)

	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return errors.NewDatabaseError("open_database", err).
			WithDetail("file_path", c.config.FilePath)
	}

	// Configure connection pool
	db.SetMaxOpenConns(c.config.MaxOpenConnections)
	db.SetMaxIdleConns(c.config.MaxIdleConnections)
	db.SetConnMaxLifetime(c.config.ConnectionMaxLifetime)
	db.SetConnMaxIdleTime(c.config.ConnectionMaxIdleTime)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return errors.NewDatabaseError("ping_database", err).
			WithDetail("file_path", c.config.FilePath)
	}

	// Set additional pragmas
	if err := c.setPragmas(db); err != nil {
		db.Close()
		return err
	}

	c.db = db
	c.log.WithField("file_path", c.config.FilePath).Info("Database connection established")

	return nil
}

// setPragmas sets SQLite-specific pragmas for performance and reliability
func (c *Connection) setPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA synchronous = NORMAL",    // Good balance of safety and performance
		"PRAGMA temp_store = MEMORY",     // Use memory for temporary tables
		"PRAGMA mmap_size = 30000000000", // 30GB memory map
	}

	if c.config.EnableWAL {
		pragmas = append(pragmas, "PRAGMA journal_mode = WAL")
		pragmas = append(pragmas, "PRAGMA wal_autocheckpoint = 1000") // Checkpoint every 1000 pages
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return errors.NewDatabaseError("set_pragma", err).
				WithDetail("pragma", pragma)
		}
	}

	return nil
}

// DB returns the underlying database connection
func (c *Connection) DB() *sql.DB {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.db
}

// Close closes the database connection
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	if c.db != nil {
		c.log.Info("Closing database connection")
		return c.db.Close()
	}

	return nil
}

// IsHealthy checks if the connection is healthy
func (c *Connection) IsHealthy(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return errors.New(errors.ErrorTypeDatabase, "health_check", "connection is closed")
	}

	if c.db == nil {
		return errors.New(errors.ErrorTypeDatabase, "health_check", "database is nil")
	}

	// Use a timeout for health check
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := c.db.PingContext(checkCtx); err != nil {
		return errors.NewDatabaseError("health_check", err)
	}

	// Check if we can execute a simple query
	var result int
	err := c.db.QueryRowContext(checkCtx, "SELECT 1").Scan(&result)
	if err != nil {
		return errors.NewDatabaseError("health_check_query", err)
	}

	return nil
}

// Transaction executes a function within a database transaction
func (c *Connection) Transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	c.mu.RLock()
	db := c.db
	c.mu.RUnlock()

	if db == nil {
		return errors.New(errors.ErrorTypeDatabase, "transaction", "database is nil")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errors.NewDatabaseError("begin_transaction", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p) // Re-panic after rollback
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			c.log.WithError(rbErr).Error("Failed to rollback transaction")
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return errors.NewDatabaseError("commit_transaction", err)
	}

	return nil
}

// Migrate runs database migrations
func (c *Connection) Migrate(ctx context.Context, migrations []Migration) error {
	c.mu.RLock()
	db := c.db
	c.mu.RUnlock()

	if db == nil {
		return errors.New(errors.ErrorTypeDatabase, "migrate", "database is nil")
	}

	// Create migrations table if it doesn't exist
	if err := c.createMigrationsTable(ctx); err != nil {
		return err
	}

	// Run each migration
	for _, migration := range migrations {
		if err := c.runMigration(ctx, migration); err != nil {
			return err
		}
	}

	return nil
}

// createMigrationsTable creates the migrations tracking table
func (c *Connection) createMigrationsTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`

	_, err := c.db.ExecContext(ctx, query)
	if err != nil {
		return errors.NewDatabaseError("create_migrations_table", err)
	}

	return nil
}

// runMigration runs a single migration if not already applied
func (c *Connection) runMigration(ctx context.Context, migration Migration) error {
	// Check if migration was already applied
	var count int
	err := c.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM schema_migrations WHERE version = ?",
		migration.Version,
	).Scan(&count)
	if err != nil {
		return errors.NewDatabaseError("check_migration", err).
			WithDetail("version", migration.Version)
	}

	if count > 0 {
		c.log.WithField("version", migration.Version).Debug("Migration already applied")
		return nil
	}

	// Run migration in a transaction
	return c.Transaction(ctx, func(tx *sql.Tx) error {
		c.log.WithFields(logrus.Fields{
			"version": migration.Version,
			"name":    migration.Name,
		}).Info("Running migration")

		// Execute migration SQL
		if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
			return errors.NewDatabaseError("execute_migration", err).
				WithDetail("version", migration.Version).
				WithDetail("name", migration.Name)
		}

		// Record migration
		_, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations (version) VALUES (?)",
			migration.Version,
		)
		if err != nil {
			return errors.NewDatabaseError("record_migration", err).
				WithDetail("version", migration.Version)
		}

		return nil
	})
}

// ExecContext executes a query with context
func (c *Connection) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	c.mu.RLock()
	db := c.db
	c.mu.RUnlock()

	if db == nil {
		return nil, errors.New(errors.ErrorTypeDatabase, "exec", "database is nil")
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, errors.NewDatabaseError("exec_query", err).
			WithDetail("query", query)
	}

	return result, nil
}

// QueryContext executes a query that returns rows
func (c *Connection) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	c.mu.RLock()
	db := c.db
	c.mu.RUnlock()

	if db == nil {
		return nil, errors.New(errors.ErrorTypeDatabase, "query", "database is nil")
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.NewDatabaseError("query", err).
			WithDetail("query", query)
	}

	return rows, nil
}

// QueryRowContext executes a query that returns a single row
func (c *Connection) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	c.mu.RLock()
	db := c.db
	c.mu.RUnlock()

	if db == nil {
		// Return a row that will error when scanned
		return &sql.Row{}
	}

	return db.QueryRowContext(ctx, query, args...)
}

// PrepareContext creates a prepared statement
func (c *Connection) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	c.mu.RLock()
	db := c.db
	c.mu.RUnlock()

	if db == nil {
		return nil, errors.New(errors.ErrorTypeDatabase, "prepare", "database is nil")
	}

	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return nil, errors.NewDatabaseError("prepare_statement", err).
			WithDetail("query", query)
	}

	return stmt, nil
}

// Migration represents a database migration
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// Stats returns database statistics
func (c *Connection) Stats() sql.DBStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.db == nil {
		return sql.DBStats{}
	}

	return c.db.Stats()
}

// ExecWithTimeout executes a query with a timeout
func (c *Connection) ExecWithTimeout(ctx context.Context, timeout time.Duration, query string, args ...any) (sql.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.ExecContext(ctx, query, args...)
}

// QueryWithTimeout executes a query with a timeout
func (c *Connection) QueryWithTimeout(ctx context.Context, timeout time.Duration, query string, args ...any) (*sql.Rows, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.QueryContext(ctx, query, args...)
}

// QueryRowWithTimeout executes a query that returns a single row with a timeout
func (c *Connection) QueryRowWithTimeout(ctx context.Context, timeout time.Duration, query string, args ...any) *sql.Row {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.QueryRowContext(ctx, query, args...)
}
