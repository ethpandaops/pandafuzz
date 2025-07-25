package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/errors"
)

func TestConnection(t *testing.T) {
	// Create temporary directory for test databases
	tmpDir, err := os.MkdirTemp("", "sqlite_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("NewConnection", func(t *testing.T) {
		config := DefaultConfig()
		config.FilePath = filepath.Join(tmpDir, "test_new.db")

		conn, err := NewConnection(config, logger)
		require.NoError(t, err)
		require.NotNil(t, conn)
		defer conn.Close()

		// Verify database file was created
		_, err = os.Stat(config.FilePath)
		assert.NoError(t, err)
	})

	t.Run("HealthCheck", func(t *testing.T) {
		config := DefaultConfig()
		config.FilePath = filepath.Join(tmpDir, "test_health.db")

		conn, err := NewConnection(config, logger)
		require.NoError(t, err)
		defer conn.Close()

		ctx := context.Background()
		err = conn.IsHealthy(ctx)
		assert.NoError(t, err)

		// Close connection and verify health check fails
		conn.Close()
		err = conn.IsHealthy(ctx)
		assert.Error(t, err)
		assert.True(t, errors.IsDatabaseError(err))
	})

	t.Run("Transaction", func(t *testing.T) {
		config := DefaultConfig()
		config.FilePath = filepath.Join(tmpDir, "test_transaction.db")

		conn, err := NewConnection(config, logger)
		require.NoError(t, err)
		defer conn.Close()

		ctx := context.Background()

		// Create test table
		_, err = conn.ExecContext(ctx, `
			CREATE TABLE test_table (
				id INTEGER PRIMARY KEY,
				value TEXT
			)
		`)
		require.NoError(t, err)

		// Test successful transaction
		err = conn.Transaction(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, "INSERT INTO test_table (value) VALUES (?)", "test1")
			return err
		})
		assert.NoError(t, err)

		// Verify data was inserted
		var count int
		err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_table").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		// Test rollback on error
		err = conn.Transaction(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, "INSERT INTO test_table (value) VALUES (?)", "test2")
			if err != nil {
				return err
			}
			// Force an error
			return errors.New(errors.ErrorTypeDatabase, "test", "forced error")
		})
		assert.Error(t, err)

		// Verify rollback worked
		err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_table").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count) // Still only 1 record
	})

	t.Run("Migrations", func(t *testing.T) {
		config := DefaultConfig()
		config.FilePath = filepath.Join(tmpDir, "test_migrations.db")

		conn, err := NewConnection(config, logger)
		require.NoError(t, err)
		defer conn.Close()

		ctx := context.Background()

		migrations := []Migration{
			{
				Version: 1,
				Name:    "create_users_table",
				SQL: `
					CREATE TABLE users (
						id INTEGER PRIMARY KEY AUTOINCREMENT,
						username TEXT NOT NULL UNIQUE,
						created_at DATETIME DEFAULT CURRENT_TIMESTAMP
					)
				`,
			},
			{
				Version: 2,
				Name:    "add_email_to_users",
				SQL: `
					ALTER TABLE users ADD COLUMN email TEXT
				`,
			},
		}

		// Run migrations
		err = conn.Migrate(ctx, migrations)
		assert.NoError(t, err)

		// Verify migrations were applied
		var count int
		err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 2, count)

		// Run migrations again - should be idempotent
		err = conn.Migrate(ctx, migrations)
		assert.NoError(t, err)

		// Verify table structure
		_, err = conn.ExecContext(ctx, "INSERT INTO users (username, email) VALUES (?, ?)", "test", "test@example.com")
		assert.NoError(t, err)
	})

	t.Run("PreparedStatements", func(t *testing.T) {
		config := DefaultConfig()
		config.FilePath = filepath.Join(tmpDir, "test_prepared.db")

		conn, err := NewConnection(config, logger)
		require.NoError(t, err)
		defer conn.Close()

		ctx := context.Background()

		// Create test table
		_, err = conn.ExecContext(ctx, `
			CREATE TABLE items (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL
			)
		`)
		require.NoError(t, err)

		// Prepare statement
		stmt, err := conn.PrepareContext(ctx, "INSERT INTO items (name) VALUES (?)")
		require.NoError(t, err)
		defer stmt.Close()

		// Use prepared statement multiple times
		for i := 0; i < 5; i++ {
			_, err = stmt.ExecContext(ctx, fmt.Sprintf("item_%d", i))
			assert.NoError(t, err)
		}

		// Verify data
		var count int
		err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM items").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 5, count)
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		config := DefaultConfig()
		config.FilePath = filepath.Join(tmpDir, "test_concurrent.db")
		config.MaxOpenConnections = 1 // SQLite performs better with single connection for writes

		conn, err := NewConnection(config, logger)
		require.NoError(t, err)
		defer conn.Close()

		ctx := context.Background()

		// Create test table
		_, err = conn.ExecContext(ctx, `
			CREATE TABLE counter (
				id INTEGER PRIMARY KEY,
				value INTEGER NOT NULL
			)
		`)
		require.NoError(t, err)

		// Initialize counter
		_, err = conn.ExecContext(ctx, "INSERT INTO counter (id, value) VALUES (1, 0)")
		require.NoError(t, err)

		// Use atomic counter for successful updates
		var successCount int32
		var wg sync.WaitGroup

		// Run concurrent updates with retry
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					retryConfig := DefaultRetryConfig()
					err := RetryableOperation(ctx, retryConfig, func() error {
						return conn.Transaction(ctx, func(tx *sql.Tx) error {
							var current int
							err := tx.QueryRowContext(ctx, "SELECT value FROM counter WHERE id = 1").Scan(&current)
							if err != nil {
								return err
							}
							_, err = tx.ExecContext(ctx, "UPDATE counter SET value = ? WHERE id = 1", current+1)
							if err == nil {
								atomic.AddInt32(&successCount, 1)
							}
							return err
						})
					})
					if err != nil {
						t.Logf("Transaction error after retries: %v", err)
					}
				}
			}()
		}

		// Wait for all goroutines
		wg.Wait()

		// Verify final count matches successful updates
		var finalValue int
		err = conn.QueryRowContext(ctx, "SELECT value FROM counter WHERE id = 1").Scan(&finalValue)
		require.NoError(t, err)

		// The final value should match the number of successful updates
		assert.Equal(t, int(successCount), finalValue)
		// We should have completed most if not all updates
		assert.GreaterOrEqual(t, finalValue, 90)
	})

	t.Run("QueryTimeout", func(t *testing.T) {
		config := DefaultConfig()
		config.FilePath = filepath.Join(tmpDir, "test_timeout.db")
		config.DefaultQueryTimeout = 100 * time.Millisecond

		conn, err := NewConnection(config, logger)
		require.NoError(t, err)
		defer conn.Close()

		ctx := context.Background()

		// Create a large table to make queries slow
		_, err = conn.ExecContext(ctx, `
			CREATE TABLE large_table (
				id INTEGER PRIMARY KEY,
				data TEXT
			)
		`)
		require.NoError(t, err)

		// Insert many rows
		for i := 0; i < 1000; i++ {
			_, err = conn.ExecContext(ctx, "INSERT INTO large_table (data) VALUES (?)",
				fmt.Sprintf("data_%d_%s", i, string(make([]byte, 1000))))
			require.NoError(t, err)
		}

		// This query should timeout (if the system is slow enough)
		// Note: This is a best-effort test and may not always trigger a timeout
		start := time.Now()
		rows, err := conn.QueryContext(ctx, `
			SELECT a.*, b.*, c.*
			FROM large_table a, large_table b, large_table c
			WHERE length(a.data) > 0
		`)
		duration := time.Since(start)

		if err != nil {
			// If error occurred, verify it happened within timeout window
			assert.True(t, duration < 200*time.Millisecond)
		} else {
			// If no error, close rows
			rows.Close()
		}
	})

	t.Run("DatabaseStats", func(t *testing.T) {
		config := DefaultConfig()
		config.FilePath = filepath.Join(tmpDir, "test_stats.db")
		config.MaxOpenConnections = 5
		config.MaxIdleConnections = 2

		conn, err := NewConnection(config, logger)
		require.NoError(t, err)
		defer conn.Close()

		stats := conn.Stats()
		assert.LessOrEqual(t, stats.OpenConnections, 5)
	})
}
