package unit

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/ethpandaops/pandafuzz/pkg/common"
)

func TestDatabasePool_ConnectionExhaustion(t *testing.T) {
	t.Run("test retry behavior with database connection exhaustion", func(t *testing.T) {
		// Simulate a scenario where we need to retry database operations
		rm := common.NewRetryManager(common.DatabaseRetryPolicy)

		var attempts int32
		var lastError error

		err := rm.Execute(func() error {
			currentAttempt := atomic.AddInt32(&attempts, 1)
			
			// Simulate database being busy/locked for first 2 attempts
			if currentAttempt < 3 {
				lastError = errors.New("database is locked")
				return lastError
			}
			
			// Success on third attempt
			return nil
		})

		if err != nil {
			t.Errorf("Expected successful retry, got error: %v", err)
		}

		if atomic.LoadInt32(&attempts) != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}

		if lastError == nil || !strings.Contains(lastError.Error(), "database is locked") {
			t.Error("Expected 'database is locked' error during retries")
		}
	})
}

func TestDatabasePool_CleanupAfterFailures(t *testing.T) {
	t.Run("verify proper cleanup after failed database operations", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("Failed to open test database: %v", err)
		}
		defer db.Close()

		db.SetMaxOpenConns(5)
		db.SetMaxIdleConns(2)

		// Create tables
		_, err = db.Exec(`
			CREATE TABLE accounts (id INTEGER PRIMARY KEY, balance INTEGER);
			INSERT INTO accounts (id, balance) VALUES (1, 1000), (2, 1000);
		`)
		if err != nil {
			t.Fatalf("Failed to setup test data: %v", err)
		}

		rm := common.NewRetryManager(common.DatabaseRetryPolicy)

		// Test transaction rollback on retry
		attempts := 0
		initialBalance := 0
		
		// Get initial balance
		db.QueryRow("SELECT balance FROM accounts WHERE id = 1").Scan(&initialBalance)

		err = rm.Execute(func() error {
			attempts++
			
			tx, err := db.Begin()
			if err != nil {
				return err
			}
			defer tx.Rollback() // Ensure rollback on error

			// Deduct from account 1
			_, err = tx.Exec("UPDATE accounts SET balance = balance - 100 WHERE id = 1")
			if err != nil {
				return err
			}

			// Simulate failure on first 2 attempts
			if attempts < 3 {
				return errors.New("database is locked")
			}

			// Add to account 2
			_, err = tx.Exec("UPDATE accounts SET balance = balance + 100 WHERE id = 2")
			if err != nil {
				return err
			}

			return tx.Commit()
		})

		if err != nil {
			t.Errorf("Transaction failed after retries: %v", err)
		}

		// Verify final state is correct
		var balance1, balance2 int
		db.QueryRow("SELECT balance FROM accounts WHERE id = 1").Scan(&balance1)
		db.QueryRow("SELECT balance FROM accounts WHERE id = 2").Scan(&balance2)

		if balance1 != initialBalance-100 {
			t.Errorf("Account 1 balance incorrect: got %d, want %d", balance1, initialBalance-100)
		}

		if balance2 != 1100 {
			t.Errorf("Account 2 balance incorrect: got %d, want %d", balance2, 1100)
		}

		// Verify connection pool is healthy
		stats := db.Stats()
		if stats.OpenConnections > stats.MaxOpenConnections {
			t.Errorf("Connection leak detected: %d open connections", stats.OpenConnections)
		}
	})
}

func TestDatabasePool_CircuitBreakerWithFailover(t *testing.T) {
	t.Run("test circuit breaker with database failover scenarios", func(t *testing.T) {
		// Simulate primary and secondary databases
		primaryDB, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("Failed to open primary database: %v", err)
		}
		defer primaryDB.Close()

		secondaryDB, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("Failed to open secondary database: %v", err)
		}
		defer secondaryDB.Close()

		// Setup both databases
		for _, db := range []*sql.DB{primaryDB, secondaryDB} {
			_, err = db.Exec("CREATE TABLE data (id INTEGER PRIMARY KEY, value TEXT)")
			if err != nil {
				t.Fatalf("Failed to create table: %v", err)
			}
		}

		rc := common.NewResilientClient(
			common.DatabaseRetryPolicy,
			3,  // max failures before circuit opens
			2*time.Second, // reset timeout
		)

		var primaryFailures int32
		var useSecondary atomic.Bool

		// Function to get the current database
		getCurrentDB := func() *sql.DB {
			if useSecondary.Load() {
				return secondaryDB
			}
			return primaryDB
		}

		// Simulate primary database failures
		for i := 0; i < 5; i++ {
			err := rc.Execute(func() error {
				db := getCurrentDB()
				
				// Simulate primary failures for first 3 attempts
				if !useSecondary.Load() && atomic.LoadInt32(&primaryFailures) < 3 {
					atomic.AddInt32(&primaryFailures, 1)
					return errors.New("database is locked")
				}

				// After 3 failures, switch to secondary
				if atomic.LoadInt32(&primaryFailures) >= 3 {
					useSecondary.Store(true)
				}

				// Execute query
				_, err := db.Exec("INSERT INTO data (value) VALUES (?)", fmt.Sprintf("test-%d", i))
				return err
			})

			if err != nil && i >= 3 {
				// After circuit opens, we expect errors
				if !strings.Contains(err.Error(), "circuit breaker is open") {
					t.Logf("Operation %d failed: %v", i, err)
				}
			}
		}

		// Verify data was written to secondary
		var count int
		secondaryDB.QueryRow("SELECT COUNT(*) FROM data").Scan(&count)
		if count == 0 {
			t.Error("Expected data to be written to secondary database after failover")
		}

		// Verify circuit breaker state
		stats := rc.GetStats()
		t.Logf("Circuit breaker stats: state=%v, failures=%d", stats.CircuitState, stats.Failures)
	})
}

func TestDatabasePool_TransactionRetry(t *testing.T) {
	t.Run("ensure proper transaction rollback on retried operations", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("Failed to open test database: %v", err)
		}
		defer db.Close()

		// Create test table with constraints
		_, err = db.Exec(`
			CREATE TABLE inventory (
				id INTEGER PRIMARY KEY,
				product TEXT UNIQUE,
				quantity INTEGER CHECK(quantity >= 0)
			);
			INSERT INTO inventory (product, quantity) VALUES ('item1', 10);
		`)
		if err != nil {
			t.Fatalf("Failed to create test table: %v", err)
		}

		rm := common.NewRetryManager(common.DatabaseRetryPolicy)

		// Test constraint violation with retry
		attempts := 0
		err = rm.Execute(func() error {
			attempts++
			
			tx, err := db.Begin()
			if err != nil {
				return err
			}
			defer tx.Rollback()

			// This will succeed
			_, err = tx.Exec("UPDATE inventory SET quantity = quantity - 5 WHERE product = 'item1'")
			if err != nil {
				return err
			}

			// On first attempt, try to violate constraint
			if attempts == 1 {
				_, err = tx.Exec("UPDATE inventory SET quantity = quantity - 10 WHERE product = 'item1'")
				if err != nil {
					// Wrap the error to make it retryable
					return fmt.Errorf("temporary failure: %w", err)
				}
			}

			return tx.Commit()
		})

		if err != nil {
			t.Errorf("Transaction failed: %v", err)
		}

		// Verify final quantity is correct (only the successful update applied)
		var quantity int
		db.QueryRow("SELECT quantity FROM inventory WHERE product = 'item1'").Scan(&quantity)
		if quantity != 5 {
			t.Errorf("Expected quantity to be 5, got %d", quantity)
		}

		if attempts < 2 {
			t.Errorf("Expected at least 2 attempts due to constraint violation, got %d", attempts)
		}
	})
}

func TestDatabasePool_ConcurrentTransactions(t *testing.T) {
	t.Run("test retry behavior with concurrent database transactions", func(t *testing.T) {
		// Simulate concurrent database operations with conflicts
		rm := common.NewRetryManager(common.DatabaseRetryPolicy)

		const numGoroutines = 10
		var wg sync.WaitGroup
		var successCount int32
		var retryCount int32
		var conflictSimulator int32

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				
				attempts := 0
				err := rm.Execute(func() error {
					attempts++
					if attempts > 1 {
						atomic.AddInt32(&retryCount, 1)
					}

					// Simulate database lock conflicts
					// First few goroutines will experience conflicts
					conflictCount := atomic.AddInt32(&conflictSimulator, 1)
					if conflictCount <= 15 && attempts == 1 {
						// Simulate a conflict on first attempt
						return errors.New("database is locked")
					}

					// Simulate successful operation
					time.Sleep(5 * time.Millisecond)
					return nil
				})

				if err == nil {
					atomic.AddInt32(&successCount, 1)
				} else {
					t.Logf("Goroutine %d failed: %v", id, err)
				}
			}(i)
		}

		wg.Wait()

		t.Logf("Successful operations: %d, Total retries: %d", 
			atomic.LoadInt32(&successCount), atomic.LoadInt32(&retryCount))

		// All operations should eventually succeed
		if atomic.LoadInt32(&successCount) != numGoroutines {
			t.Errorf("Expected all %d operations to succeed, got %d", 
				numGoroutines, atomic.LoadInt32(&successCount))
		}

		// Some operations should have required retries
		if atomic.LoadInt32(&retryCount) == 0 {
			t.Error("Expected some operations to retry due to conflicts")
		}
	})
}

