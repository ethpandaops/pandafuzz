# SQLite Database Infrastructure

This package provides a robust SQLite database connection infrastructure with connection pooling, migrations, transaction support, and various helper utilities.

## Features

- **Connection Pooling**: Configurable connection pool with limits on open/idle connections
- **Database Migrations**: Built-in migration system to manage schema changes
- **Transaction Support**: Easy-to-use transaction wrapper with automatic rollback on error
- **Context Support**: All operations support context for cancellation and timeouts
- **Health Checks**: Connection health monitoring
- **Retry Logic**: Automatic retry for transient errors (database locked, busy)
- **Performance Optimizations**: WAL mode, memory-mapped I/O, optimized pragmas
- **Helper Functions**: Utilities for common database operations

## Usage

### Basic Connection

```go
import (
    "github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/sqlite"
    "github.com/sirupsen/logrus"
)

// Create connection with default config
config := sqlite.DefaultConfig()
config.FilePath = "myapp.db"

conn, err := sqlite.NewConnection(config, logrus.New())
if err != nil {
    log.Fatal(err)
}
defer conn.Close()
```

### Configuration Options

```go
config := sqlite.ConnectionConfig{
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
```

### Transactions

```go
err := conn.Transaction(ctx, func(tx *sql.Tx) error {
    // All operations in this function run in a transaction
    _, err := tx.ExecContext(ctx, "INSERT INTO users (name) VALUES (?)", "Alice")
    if err != nil {
        return err // Transaction will be rolled back
    }
    
    _, err = tx.ExecContext(ctx, "UPDATE stats SET user_count = user_count + 1")
    return err // Transaction commits if no error
})
```

### Migrations

```go
migrations := []sqlite.Migration{
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
        SQL: `ALTER TABLE users ADD COLUMN email TEXT`,
    },
}

err := conn.Migrate(ctx, migrations)
```

### Retry Operations

```go
config := sqlite.DefaultRetryConfig()
err := sqlite.RetryableOperation(ctx, config, func() error {
    // This operation will be retried on transient errors
    _, err := conn.ExecContext(ctx, "UPDATE users SET last_seen = ?", time.Now())
    return err
})
```

### Helper Functions

```go
// Count rows
count, err := sqlite.CountRows(ctx, conn, "SELECT COUNT(*) FROM users WHERE active = ?", true)

// Check existence
exists, err := sqlite.ExistsQuery(ctx, conn, "SELECT 1 FROM users WHERE email = ?", email)

// Batch operations
operations := []sqlite.BatchOperation{
    {Query: "INSERT INTO logs (message) VALUES (?)", Args: []any{"Operation started"}},
    {Query: "UPDATE stats SET last_run = ?", Args: []any{time.Now()}},
}
err := sqlite.ExecuteBatch(ctx, conn, operations)

// Build queries
insertQuery := sqlite.BuildInsertQuery("users", []string{"name", "email", "age"})
updateQuery := sqlite.BuildUpdateQuery("users", []string{"name", "email"}, "id = ?")
```

## Performance Considerations

1. **WAL Mode**: Write-Ahead Logging is enabled by default for better concurrency
2. **Connection Pool**: Configure based on your workload (default: 25 connections)
3. **Busy Timeout**: Set appropriately for your use case (default: 5 seconds)
4. **Cache Size**: Adjust based on available memory (default: 2MB)
5. **Batch Operations**: Use `ExecuteBatch` for multiple related operations
6. **Prepared Statements**: Use `PrepareContext` for frequently executed queries

## Error Handling

All errors are wrapped using the custom error types from `pkg/errors`:

```go
if err != nil {
    if errors.IsDatabaseError(err) {
        // Handle database-specific error
    }
    if errors.IsTimeoutError(err) {
        // Handle timeout
    }
}
```

## Testing

The package includes comprehensive tests demonstrating usage patterns:

```bash
go test ./pkg/infrastructure/persistence/sqlite/...
```

## Thread Safety

- The Connection type is thread-safe
- Multiple goroutines can safely use the same connection
- Transactions provide isolation between concurrent operations
- SQLite's locking mechanisms prevent data corruption

## Best Practices

1. Always use contexts for cancellation support
2. Close connections when done
3. Use transactions for related operations
4. Handle errors appropriately
5. Use prepared statements for repeated queries
6. Monitor connection pool statistics
7. Run VACUUM periodically for long-running applications