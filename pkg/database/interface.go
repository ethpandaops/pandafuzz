// Package database provides database abstractions and interfaces for the PandaFuzz system.
// This package defines the core database interfaces that all storage implementations
// must satisfy, enabling clean separation between business logic and storage.
package database

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
	Stats(ctx context.Context) Stats
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

// Stats provides runtime statistics about the database
type Stats struct {
	Type         string `json:"type"`         // "sqlite", "badger", "memory"
	Path         string `json:"path"`         // Database file path
	Size         int64  `json:"size"`         // Database size in bytes
	Keys         int64  `json:"keys"`         // Total number of keys
	Connections  int    `json:"connections"`  // Active connections
	Transactions int64  `json:"transactions"` // Total transactions
	LastBackup   string `json:"last_backup"`  // Last backup timestamp
	IsHealthy    bool   `json:"is_healthy"`   // Health status
}

// Config holds configuration for database initialization
type Config struct {
	Type       string            `json:"type" yaml:"type"`               // "sqlite", "badger", "memory"
	Path       string            `json:"path" yaml:"path"`               // Database file path
	MaxConns   int               `json:"max_conns" yaml:"max_conns"`     // Maximum connections
	IdleConns  int               `json:"idle_conns" yaml:"idle_conns"`   // Idle connections
	Timeout    string            `json:"timeout" yaml:"timeout"`         // Connection timeout
	Options    map[string]string `json:"options" yaml:"options"`         // Database-specific options
	BackupPath string            `json:"backup_path" yaml:"backup_path"` // Backup directory
	BackupFreq string            `json:"backup_freq" yaml:"backup_freq"` // Backup frequency

	// Connection pool configuration
	MaxOpenConns        int           `json:"max_open_conns" yaml:"max_open_conns"`               // Maximum open connections
	MaxIdleConns        int           `json:"max_idle_conns" yaml:"max_idle_conns"`               // Maximum idle connections
	ConnMaxLifetime     time.Duration `json:"conn_max_lifetime" yaml:"conn_max_lifetime"`         // Maximum connection lifetime
	ConnMaxIdleTime     time.Duration `json:"conn_max_idle_time" yaml:"conn_max_idle_time"`       // Maximum connection idle time
	HealthCheckInterval time.Duration `json:"health_check_interval" yaml:"health_check_interval"` // Health check interval

	// Retry configuration
	MaxRetries      int           `json:"max_retries" yaml:"max_retries"`           // Maximum number of retries
	RetryDelay      time.Duration `json:"retry_delay" yaml:"retry_delay"`           // Initial delay between retries
	MaxRetryDelay   time.Duration `json:"max_retry_delay" yaml:"max_retry_delay"`   // Maximum delay between retries
	RetryMultiplier float64       `json:"retry_multiplier" yaml:"retry_multiplier"` // Multiplier for exponential backoff
}

// SetDefaults sets default values for retry configuration
func (dc *Config) SetDefaults() {
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
func (dc *Config) Validate() error {
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

// Advanced defines the interface for complex database operations
type Advanced interface {
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

// Factory creates database instances based on configuration
type Factory interface {
	Create(ctx context.Context, config Config) (Database, error)
	CreateAdvanced(ctx context.Context, config Config) (Advanced, error)
}

// Middleware provides middleware functionality for database operations
type Middleware interface {
	BeforeStore(ctx context.Context, key string, value any) error
	AfterStore(ctx context.Context, key string, value any) error
	BeforeGet(ctx context.Context, key string) error
	AfterGet(ctx context.Context, key string, value any) error
	BeforeDelete(ctx context.Context, key string) error
	AfterDelete(ctx context.Context, key string) error
	BeforeTransaction(ctx context.Context) error
	AfterTransaction(ctx context.Context, err error) error
}

// WithMiddleware wraps a database with middleware
type WithMiddleware struct {
	db         Database
	middleware []Middleware
}

// NewWithMiddleware creates a database wrapper with middleware support
func NewWithMiddleware(db Database, middleware ...Middleware) *WithMiddleware {
	return &WithMiddleware{
		db:         db,
		middleware: middleware,
	}
}

// Store implements Database.Store with middleware
func (dw *WithMiddleware) Store(ctx context.Context, key string, value any) error {
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

// Get implements Database.Get with middleware
func (dw *WithMiddleware) Get(ctx context.Context, key string, dest any) error {
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
			return err
		}
	}

	return err
}

// Delete implements Database.Delete with middleware
func (dw *WithMiddleware) Delete(ctx context.Context, key string) error {
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
			return err
		}
	}

	return err
}

// Transaction implements Database.Transaction with middleware
func (dw *WithMiddleware) Transaction(ctx context.Context, fn func(tx Transaction) error) error {
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
			return err
		}
	}

	return err
}

// Close implements Database.Close
func (dw *WithMiddleware) Close(ctx context.Context) error {
	return dw.db.Close(ctx)
}

// Ping implements Database.Ping
func (dw *WithMiddleware) Ping(ctx context.Context) error {
	return dw.db.Ping(ctx)
}

// Stats implements Database.Stats
func (dw *WithMiddleware) Stats(ctx context.Context) Stats {
	return dw.db.Stats(ctx)
}

// Helper functions for error checking

// IsTransactionError checks if the error is a transaction error
func IsTransactionError(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "transaction failed"
}

// IsDatabaseClosed checks if the database is closed
func IsDatabaseClosed(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "database is closed"
}
