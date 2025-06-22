package common

import (
	"context"
	"fmt"
)

// Database defines the interface for persistent storage
type Database interface {
	// Basic operations
	Store(key string, value interface{}) error
	Get(key string, dest interface{}) error
	Delete(key string) error
	
	// Transaction support
	Transaction(fn func(tx Transaction) error) error
	
	// Lifecycle
	Close() error
	
	// Health and status
	Ping() error
	Stats() DatabaseStats
}

// Transaction defines the interface for database transactions
type Transaction interface {
	Store(key string, value interface{}) error
	Get(key string, dest interface{}) error
	Delete(key string) error
	
	// Transaction control
	Commit() error
	Rollback() error
}

// DatabaseStats provides runtime statistics about the database
type DatabaseStats struct {
	Type          string `json:"type"`           // "sqlite", "badger", "memory"
	Path          string `json:"path"`           // Database file path
	Size          int64  `json:"size"`           // Database size in bytes
	Keys          int64  `json:"keys"`           // Total number of keys
	Connections   int    `json:"connections"`    // Active connections
	Transactions  int64  `json:"transactions"`   // Total transactions
	LastBackup    string `json:"last_backup"`    // Last backup timestamp
	IsHealthy     bool   `json:"is_healthy"`     // Health status
}

// DatabaseConfig holds configuration for database initialization
type DatabaseConfig struct {
	Type        string            `json:"type" yaml:"type"`                 // "sqlite", "badger", "memory"
	Path        string            `json:"path" yaml:"path"`                 // Database file path
	MaxConns    int               `json:"max_conns" yaml:"max_conns"`       // Maximum connections
	IdleConns   int               `json:"idle_conns" yaml:"idle_conns"`     // Idle connections
	Timeout     string            `json:"timeout" yaml:"timeout"`           // Connection timeout
	Options     map[string]string `json:"options" yaml:"options"`           // Database-specific options
	BackupPath  string            `json:"backup_path" yaml:"backup_path"`   // Backup directory
	BackupFreq  string            `json:"backup_freq" yaml:"backup_freq"`   // Backup frequency
}

// Query interface for advanced database operations
type Query interface {
	// Basic querying
	Select(query string, args ...interface{}) ([]map[string]interface{}, error)
	SelectOne(query string, args ...interface{}) (map[string]interface{}, error)
	Execute(query string, args ...interface{}) (int64, error)
	
	// Context-aware operations
	SelectContext(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error)
	ExecuteContext(ctx context.Context, query string, args ...interface{}) (int64, error)
}

// Advanced database interface for complex operations
type AdvancedDatabase interface {
	Database
	Query
	
	// Batch operations
	BatchStore(items map[string]interface{}) error
	BatchDelete(keys []string) error
	
	// Iteration
	Iterate(prefix string, fn func(key string, value []byte) error) error
	
	// Backup and restore
	Backup(path string) error
	Restore(path string) error
	
	// Schema management (for SQL databases)
	CreateTables() error
	Migrate(version int) error
	
	// Maintenance
	Vacuum() error
	Compact() error
}

// DatabaseFactory creates database instances based on configuration
type DatabaseFactory interface {
	Create(config DatabaseConfig) (Database, error)
	CreateAdvanced(config DatabaseConfig) (AdvancedDatabase, error)
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
	BeforeStore(key string, value interface{}) error
	AfterStore(key string, value interface{}) error
	BeforeGet(key string) error
	AfterGet(key string, value interface{}) error
	BeforeDelete(key string) error
	AfterDelete(key string) error
	BeforeTransaction() error
	AfterTransaction(err error) error
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

func (dw *DatabaseWithMiddleware) Store(key string, value interface{}) error {
	// Run before middleware
	for _, mw := range dw.middleware {
		if err := mw.BeforeStore(key, value); err != nil {
			return err
		}
	}
	
	// Execute operation
	err := dw.db.Store(key, value)
	
	// Run after middleware
	for _, mw := range dw.middleware {
		if afterErr := mw.AfterStore(key, value); afterErr != nil {
			// Log but don't override original error
			return err
		}
	}
	
	return err
}

func (dw *DatabaseWithMiddleware) Get(key string, dest interface{}) error {
	// Run before middleware
	for _, mw := range dw.middleware {
		if err := mw.BeforeGet(key); err != nil {
			return err
		}
	}
	
	// Execute operation
	err := dw.db.Get(key, dest)
	
	// Run after middleware
	for _, mw := range dw.middleware {
		if afterErr := mw.AfterGet(key, dest); afterErr != nil {
			// Log but don't override original error
			return err
		}
	}
	
	return err
}

func (dw *DatabaseWithMiddleware) Delete(key string) error {
	// Run before middleware
	for _, mw := range dw.middleware {
		if err := mw.BeforeDelete(key); err != nil {
			return err
		}
	}
	
	// Execute operation
	err := dw.db.Delete(key)
	
	// Run after middleware
	for _, mw := range dw.middleware {
		if afterErr := mw.AfterDelete(key); afterErr != nil {
			// Log but don't override original error
			return err
		}
	}
	
	return err
}

func (dw *DatabaseWithMiddleware) Transaction(fn func(tx Transaction) error) error {
	// Run before middleware
	for _, mw := range dw.middleware {
		if err := mw.BeforeTransaction(); err != nil {
			return err
		}
	}
	
	// Execute transaction
	err := dw.db.Transaction(fn)
	
	// Run after middleware
	for _, mw := range dw.middleware {
		if afterErr := mw.AfterTransaction(err); afterErr != nil {
			// Log but don't override original error
			return err
		}
	}
	
	return err
}

func (dw *DatabaseWithMiddleware) Close() error {
	return dw.db.Close()
}

func (dw *DatabaseWithMiddleware) Ping() error {
	return dw.db.Ping()
}

func (dw *DatabaseWithMiddleware) Stats() DatabaseStats {
	return dw.db.Stats()
}