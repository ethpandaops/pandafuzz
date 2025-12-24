// Package database defines interfaces for database operations in PandaFuzz.
//
// This package provides a database abstraction layer that separates business
// logic from specific database implementations. The interfaces support both
// simple key-value operations and advanced SQL queries.
//
// # Core Interface
//
// The Database interface defines the minimal contract for data persistence:
//
//	type Database interface {
//	    Store(ctx context.Context, key string, value any) error
//	    Get(ctx context.Context, key string, dest any) error
//	    Delete(ctx context.Context, key string) error
//	    Transaction(ctx context.Context, fn func(Transaction) error) error
//	    Close() error
//	    Ping(ctx context.Context) error
//	    Stats() Stats
//	}
//
// # Transactions
//
// All database implementations must support ACID transactions. The Transaction
// interface allows multiple operations to be executed atomically:
//
//	err := db.Transaction(ctx, func(tx database.Transaction) error {
//	    if err := tx.Store(ctx, "job:123", job); err != nil {
//	        return err // Triggers rollback
//	    }
//	    return tx.Store(ctx, "bot:456", bot)
//	})
//
// # Advanced Queries
//
// The AdvancedDatabase interface extends Database with SQL-like query support:
//
//	type AdvancedDatabase interface {
//	    Database
//	    Query(ctx context.Context, query string, args ...any) ([]map[string]any, error)
//	    Execute(ctx context.Context, query string, args ...any) (int64, error)
//	    SelectOne(ctx context.Context, query string, args ...any) (map[string]any, error)
//	}
//
// # Middleware
//
// Database operations can be wrapped with middleware for cross-cutting concerns
// like logging, metrics, and retry logic:
//
//	db := sqlite.NewDatabase(cfg)
//	db = database.WithMiddleware(db, loggingMiddleware, metricsMiddleware)
//
// # Error Handling
//
// The package defines error types for common database conditions:
//
//   - ErrKeyNotFound: Requested key does not exist
//   - ErrTransactionFailed: Transaction could not complete
//   - ErrDatabaseClosed: Database connection is closed
//
// Use errors.Is() for error checking:
//
//	if errors.Is(err, database.ErrKeyNotFound) {
//	    // Handle missing key
//	}
//
// # Thread Safety
//
// All Database implementations must be safe for concurrent use. Connection
// pooling is handled internally by the implementation.
package database
