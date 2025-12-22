package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/sqlite"
)

func ExampleNewConnection() {
	// Create a connection with custom configuration
	config := sqlite.DefaultConfig()
	config.FilePath = "example.db"
	config.MaxOpenConnections = 10

	logger := logrus.New()
	conn, err := sqlite.NewConnection(config, logger)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Use the connection
	ctx := context.Background()
	_, err = conn.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleConnection_Transaction() {
	config := sqlite.DefaultConfig()
	conn, _ := sqlite.NewConnection(config, nil)
	defer conn.Close()

	ctx := context.Background()

	// Execute operations in a transaction
	err := conn.Transaction(ctx, func(tx *sql.Tx) error {
		// Create table
		_, err := tx.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS accounts (
				id INTEGER PRIMARY KEY,
				balance REAL NOT NULL
			)
		`)
		if err != nil {
			return err
		}

		// Insert initial data
		_, err = tx.ExecContext(ctx, "INSERT INTO accounts (balance) VALUES (?), (?)", 100.0, 200.0)
		if err != nil {
			return err
		}

		// Transfer money between accounts
		_, err = tx.ExecContext(ctx, "UPDATE accounts SET balance = balance - 50 WHERE id = 1")
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, "UPDATE accounts SET balance = balance + 50 WHERE id = 2")
		return err
	})

	if err != nil {
		fmt.Printf("Transaction failed: %v\n", err)
	} else {
		fmt.Println("Transaction completed successfully")
	}
}

func ExampleConnection_Migrate() {
	config := sqlite.DefaultConfig()
	conn, _ := sqlite.NewConnection(config, nil)
	defer conn.Close()

	// Define migrations
	migrations := []sqlite.Migration{
		{
			Version: 1,
			Name:    "create_products_table",
			SQL: `
				CREATE TABLE products (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL,
					price REAL NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)
			`,
		},
		{
			Version: 2,
			Name:    "add_category_to_products",
			SQL: `
				ALTER TABLE products ADD COLUMN category TEXT DEFAULT 'general'
			`,
		},
		{
			Version: 3,
			Name:    "create_categories_table",
			SQL: `
				CREATE TABLE categories (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL UNIQUE
				);
				
				CREATE INDEX idx_products_category ON products(category);
			`,
		},
	}

	// Run migrations
	ctx := context.Background()
	err := conn.Migrate(ctx, migrations)
	if err != nil {
		fmt.Printf("Migration failed: %v\n", err)
	} else {
		fmt.Println("Migrations completed successfully")
	}
}

func ExampleRetryableOperation() {
	config := sqlite.DefaultConfig()
	conn, _ := sqlite.NewConnection(config, nil)
	defer conn.Close()

	ctx := context.Background()

	// Operation that might fail due to database being locked
	retryConfig := sqlite.DefaultRetryConfig()
	err := sqlite.RetryableOperation(ctx, retryConfig, func() error {
		result, err := conn.ExecContext(ctx, "UPDATE stats SET counter = counter + 1")
		if err != nil {
			return err
		}

		rowsAffected, _ := result.RowsAffected()
		fmt.Printf("Updated %d rows\n", rowsAffected)
		return nil
	})

	if err != nil {
		fmt.Printf("Operation failed after retries: %v\n", err)
	}
}

func ExampleBuildInsertQuery() {
	// Build an INSERT query
	query, err := sqlite.BuildInsertQuery("users", []string{"name", "email", "age"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(query)
	// Output: INSERT INTO users (name, email, age) VALUES (?, ?, ?)
}

func ExampleBuildUpdateQuery() {
	// Build an UPDATE query
	query, err := sqlite.BuildUpdateQuery("users", []string{"name", "email"}, "id = ?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(query)
	// Output: UPDATE users SET name = ?, email = ? WHERE id = ?
}

func ExampleBuildBulkInsertQuery() {
	// Build a bulk INSERT query for 3 rows
	query, err := sqlite.BuildBulkInsertQuery("logs", []string{"level", "message", "timestamp"}, 3)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(query)
	// Output: INSERT INTO logs (level, message, timestamp) VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?)
}

func ExampleCountRows() {
	config := sqlite.DefaultConfig()
	conn, _ := sqlite.NewConnection(config, nil)
	defer conn.Close()

	ctx := context.Background()

	// Count active users
	count, err := sqlite.CountRows(ctx, conn,
		"SELECT COUNT(*) FROM users WHERE last_login > ?",
		time.Now().Add(-24*time.Hour),
	)
	if err != nil {
		fmt.Printf("Error counting rows: %v\n", err)
	} else {
		fmt.Printf("Active users in last 24 hours: %d\n", count)
	}
}

func ExampleExecuteBatch() {
	config := sqlite.DefaultConfig()
	conn, _ := sqlite.NewConnection(config, nil)
	defer conn.Close()

	ctx := context.Background()

	// Execute multiple operations in a single transaction
	operations := []sqlite.BatchOperation{
		{
			Query: "INSERT INTO audit_log (action, user_id, timestamp) VALUES (?, ?, ?)",
			Args:  []any{"login", 123, time.Now()},
		},
		{
			Query: "UPDATE users SET last_login = ? WHERE id = ?",
			Args:  []any{time.Now(), 123},
		},
		{
			Query: "UPDATE statistics SET login_count = login_count + 1",
			Args:  []any{},
		},
	}

	err := sqlite.ExecuteBatch(ctx, conn, operations)
	if err != nil {
		fmt.Printf("Batch operation failed: %v\n", err)
	} else {
		fmt.Println("Batch operations completed successfully")
	}
}
