package common

import (
	"context"
	"fmt"
	"time"
)

// Database defines the interface for persistent storage
type Database interface {
	// Basic operations
	Store(ctx context.Context, key string, value any) error
	Get(ctx context.Context, key string, dest any) error
	Delete(ctx context.Context, key string) error

	// Transaction support
	Transaction(ctx context.Context, fn func(tx Transaction) error) error

	// Lifecycle
	Close(ctx context.Context) error

	// Health and status
	Ping(ctx context.Context) error
	Stats(ctx context.Context) DatabaseStats
}

// Transaction defines the interface for database transactions
type Transaction interface {
	Store(ctx context.Context, key string, value any) error
	Get(ctx context.Context, key string, dest any) error
	Delete(ctx context.Context, key string) error

	// Transaction control
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// DatabaseStats provides runtime statistics about the database
type DatabaseStats struct {
	Type         string `json:"type"`         // "sqlite", "badger", "memory"
	Path         string `json:"path"`         // Database file path
	Size         int64  `json:"size"`         // Database size in bytes
	Keys         int64  `json:"keys"`         // Total number of keys
	Connections  int    `json:"connections"`  // Active connections
	Transactions int64  `json:"transactions"` // Total transactions
	LastBackup   string `json:"last_backup"`  // Last backup timestamp
	IsHealthy    bool   `json:"is_healthy"`   // Health status
}

// DatabaseConfig holds configuration for database initialization
type DatabaseConfig struct {
	Type       string            `json:"type" yaml:"type"`               // "sqlite", "badger", "memory"
	Path       string            `json:"path" yaml:"path"`               // Database file path
	MaxConns   int               `json:"max_conns" yaml:"max_conns"`     // Maximum connections
	IdleConns  int               `json:"idle_conns" yaml:"idle_conns"`   // Idle connections
	Timeout    string            `json:"timeout" yaml:"timeout"`         // Connection timeout
	Options    map[string]string `json:"options" yaml:"options"`         // Database-specific options
	BackupPath string            `json:"backup_path" yaml:"backup_path"` // Backup directory
	BackupFreq string            `json:"backup_freq" yaml:"backup_freq"` // Backup frequency

	// Retry configuration
	MaxRetries      int           `json:"max_retries" yaml:"max_retries"`           // Maximum number of retries
	RetryDelay      time.Duration `json:"retry_delay" yaml:"retry_delay"`           // Initial delay between retries
	MaxRetryDelay   time.Duration `json:"max_retry_delay" yaml:"max_retry_delay"`   // Maximum delay between retries
	RetryMultiplier float64       `json:"retry_multiplier" yaml:"retry_multiplier"` // Multiplier for exponential backoff
}

// SetDefaults sets default values for retry configuration
func (dc *DatabaseConfig) SetDefaults() {
	if dc.MaxRetries == 0 {
		dc.MaxRetries = 5
	}
	if dc.RetryDelay == 0 {
		dc.RetryDelay = 10 * time.Millisecond
	}
	if dc.MaxRetryDelay == 0 {
		dc.MaxRetryDelay = 1 * time.Second
	}
	if dc.RetryMultiplier == 0 {
		dc.RetryMultiplier = 2.0
	}
}

// Validate validates the database configuration
func (dc *DatabaseConfig) Validate() error {
	// Validate database type
	if dc.Type == "" {
		return fmt.Errorf("database type is required")
	}

	validTypes := []string{"sqlite", "badger", "memory"}
	validType := false
	for _, t := range validTypes {
		if dc.Type == t {
			validType = true
			break
		}
	}
	if !validType {
		return fmt.Errorf("invalid database type: %s (must be one of: sqlite, badger, memory)", dc.Type)
	}

	// Validate path for non-memory databases
	if dc.Type != "memory" && dc.Path == "" {
		return fmt.Errorf("database path is required for %s database", dc.Type)
	}

	// Validate retry configuration
	if dc.MaxRetries < 0 {
		return fmt.Errorf("max_retries cannot be negative")
	}
	if dc.MaxRetries > 100 {
		return fmt.Errorf("max_retries too high (max: 100)")
	}

	if dc.RetryDelay < 0 {
		return fmt.Errorf("retry_delay cannot be negative")
	}
	if dc.RetryDelay > 10*time.Second {
		return fmt.Errorf("retry_delay too high (max: 10s)")
	}

	if dc.MaxRetryDelay < 0 {
		return fmt.Errorf("max_retry_delay cannot be negative")
	}
	if dc.MaxRetryDelay > 5*time.Minute {
		return fmt.Errorf("max_retry_delay too high (max: 5m)")
	}

	if dc.MaxRetryDelay < dc.RetryDelay {
		return fmt.Errorf("max_retry_delay must be greater than or equal to retry_delay")
	}

	if dc.RetryMultiplier < 1.0 {
		return fmt.Errorf("retry_multiplier must be at least 1.0")
	}
	if dc.RetryMultiplier > 10.0 {
		return fmt.Errorf("retry_multiplier too high (max: 10.0)")
	}

	// Validate connection settings
	if dc.MaxConns < 0 {
		return fmt.Errorf("max_conns cannot be negative")
	}
	if dc.IdleConns < 0 {
		return fmt.Errorf("idle_conns cannot be negative")
	}
	if dc.IdleConns > dc.MaxConns && dc.MaxConns > 0 {
		return fmt.Errorf("idle_conns cannot be greater than max_conns")
	}

	return nil
}

// Query interface for advanced database operations
type Query interface {
	// All operations require context for proper cancellation and timeout handling
	Select(ctx context.Context, query string, args ...any) ([]map[string]any, error)
	SelectOne(ctx context.Context, query string, args ...any) (map[string]any, error)
	Execute(ctx context.Context, query string, args ...any) (int64, error)
}

// Advanced database interface for complex operations
type AdvancedDatabase interface {
	Database
	Query

	// Batch operations
	BatchStore(ctx context.Context, items map[string]any) error
	BatchDelete(ctx context.Context, keys []string) error

	// Iteration
	Iterate(ctx context.Context, prefix string, fn func(key string, value []byte) error) error

	// Backup and restore
	Backup(ctx context.Context, path string) error
	Restore(ctx context.Context, path string) error

	// Schema management (for SQL databases)
	CreateTables(ctx context.Context) error
	Migrate(ctx context.Context, version int) error

	// Maintenance
	Vacuum(ctx context.Context) error
	Compact(ctx context.Context) error
}

// DatabaseFactory creates database instances based on configuration
type DatabaseFactory interface {
	Create(ctx context.Context, config DatabaseConfig) (Database, error)
	CreateAdvanced(ctx context.Context, config DatabaseConfig) (AdvancedDatabase, error)
}

// Common errors
var (
	ErrKeyNotFound     = fmt.Errorf("key not found")
	ErrTransactionFail = fmt.Errorf("transaction failed")
	ErrDatabaseClosed  = fmt.Errorf("database is closed")
	ErrInvalidConfig   = fmt.Errorf("invalid database configuration")
	ErrMigrationFailed = fmt.Errorf("database migration failed")
	ErrBackupFailed    = fmt.Errorf("database backup failed")
	ErrRestoreFailed   = fmt.Errorf("database restore failed")
)

// Helper functions for database operations

func IsTransactionError(err error) bool {
	return err == ErrTransactionFail
}

func IsDatabaseClosed(err error) bool {
	return err == ErrDatabaseClosed
}

// DatabaseMiddleware provides middleware functionality
type DatabaseMiddleware interface {
	BeforeStore(ctx context.Context, key string, value any) error
	AfterStore(ctx context.Context, key string, value any) error
	BeforeGet(ctx context.Context, key string) error
	AfterGet(ctx context.Context, key string, value any) error
	BeforeDelete(ctx context.Context, key string) error
	AfterDelete(ctx context.Context, key string) error
	BeforeTransaction(ctx context.Context) error
	AfterTransaction(ctx context.Context, err error) error
}

// DatabaseWithMiddleware wraps a database with middleware
type DatabaseWithMiddleware struct {
	db         Database
	middleware []DatabaseMiddleware
}

func NewDatabaseWithMiddleware(db Database, middleware ...DatabaseMiddleware) *DatabaseWithMiddleware {
	return &DatabaseWithMiddleware{
		db:         db,
		middleware: middleware,
	}
}

func (dw *DatabaseWithMiddleware) Store(ctx context.Context, key string, value any) error {
	// Run before middleware
	for _, mw := range dw.middleware {
		if err := mw.BeforeStore(ctx, key, value); err != nil {
			return err
		}
	}

	// Execute operation
	err := dw.db.Store(ctx, key, value)

	// Run after middleware
	for _, mw := range dw.middleware {
		if afterErr := mw.AfterStore(ctx, key, value); afterErr != nil {
			// Log but don't override original error
			return err
		}
	}

	return err
}

func (dw *DatabaseWithMiddleware) Get(ctx context.Context, key string, dest any) error {
	// Run before middleware
	for _, mw := range dw.middleware {
		if err := mw.BeforeGet(ctx, key); err != nil {
			return err
		}
	}

	// Execute operation
	err := dw.db.Get(ctx, key, dest)

	// Run after middleware
	for _, mw := range dw.middleware {
		if afterErr := mw.AfterGet(ctx, key, dest); afterErr != nil {
			// Log but don't override original error
			return err
		}
	}

	return err
}

func (dw *DatabaseWithMiddleware) Delete(ctx context.Context, key string) error {
	// Run before middleware
	for _, mw := range dw.middleware {
		if err := mw.BeforeDelete(ctx, key); err != nil {
			return err
		}
	}

	// Execute operation
	err := dw.db.Delete(ctx, key)

	// Run after middleware
	for _, mw := range dw.middleware {
		if afterErr := mw.AfterDelete(ctx, key); afterErr != nil {
			// Log but don't override original error
			return err
		}
	}

	return err
}

func (dw *DatabaseWithMiddleware) Transaction(ctx context.Context, fn func(tx Transaction) error) error {
	// Run before middleware
	for _, mw := range dw.middleware {
		if err := mw.BeforeTransaction(ctx); err != nil {
			return err
		}
	}

	// Execute transaction
	err := dw.db.Transaction(ctx, fn)

	// Run after middleware
	for _, mw := range dw.middleware {
		if afterErr := mw.AfterTransaction(ctx, err); afterErr != nil {
			// Log but don't override original error
			return err
		}
	}

	return err
}

func (dw *DatabaseWithMiddleware) Close(ctx context.Context) error {
	return dw.db.Close(ctx)
}

func (dw *DatabaseWithMiddleware) Ping(ctx context.Context) error {
	return dw.db.Ping(ctx)
}

func (dw *DatabaseWithMiddleware) Stats(ctx context.Context) DatabaseStats {
	return dw.db.Stats(ctx)
}
