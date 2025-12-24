package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/errors"
)

// RetryConfig defines retry behavior for database operations
type RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
}

// DefaultRetryConfig returns sensible retry defaults for SQLite
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 50 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Multiplier:     2.0,
	}
}

// RetryableOperation executes a database operation with retry logic
func RetryableOperation(ctx context.Context, config RetryConfig, operation func() error) error {
	var lastErr error
	backoff := config.InitialBackoff

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Check context before attempting
		if err := ctx.Err(); err != nil {
			return errors.Wrap(errors.ErrorTypeTimeout, "retry_operation", "context cancelled", err)
		}

		// Execute operation
		lastErr = operation()
		if lastErr == nil {
			return nil
		}

		// Check if error is retryable
		if !isRetryableError(lastErr) {
			return lastErr
		}

		// Don't sleep on last attempt
		if attempt < config.MaxRetries {
			// Calculate sleep duration
			sleepDuration := backoff
			if sleepDuration > config.MaxBackoff {
				sleepDuration = config.MaxBackoff
			}

			// Sleep with context
			timer := time.NewTimer(sleepDuration)
			select {
			case <-ctx.Done():
				timer.Stop()
				return errors.Wrap(errors.ErrorTypeTimeout, "retry_operation", "context cancelled during retry", ctx.Err())
			case <-timer.C:
			}

			// Increase backoff for next attempt
			backoff = time.Duration(float64(backoff) * config.Multiplier)
		}
	}

	return errors.Wrap(errors.ErrorTypeDatabase, "retry_operation",
		fmt.Sprintf("operation failed after %d retries", config.MaxRetries), lastErr)
}

// isRetryableError checks if an error is retryable for SQLite
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// SQLite busy errors
	if strings.Contains(errStr, "database is locked") ||
		strings.Contains(errStr, "database table is locked") ||
		strings.Contains(errStr, "busy") {
		return true
	}

	// Check for timeout errors
	if errors.IsTimeoutError(err) {
		return true
	}

	return false
}

// ScanRow is a helper to scan a single row with proper error handling
func ScanRow(rows *sql.Rows, dest ...any) error {
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return errors.NewDatabaseError("scan_row", err)
		}
		return errors.NewNotFoundError("scan_row", "row")
	}

	if err := rows.Scan(dest...); err != nil {
		return errors.NewDatabaseError("scan_row", err)
	}

	return rows.Err()
}

// ScanRows is a helper to scan multiple rows
func ScanRows(rows *sql.Rows, scanner func() error) error {
	for rows.Next() {
		if err := scanner(); err != nil {
			return err
		}
	}
	return rows.Err()
}

// BuildInsertQuery builds a parameterized INSERT query
func BuildInsertQuery(table string, columns []string) (string, error) {
	if len(columns) == 0 {
		return "", fmt.Errorf("BuildInsertQuery: no columns provided for table %s", table)
	}

	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	return fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	), nil
}

// BuildUpdateQuery builds a parameterized UPDATE query
func BuildUpdateQuery(table string, columns []string, whereClause string) (string, error) {
	if len(columns) == 0 {
		return "", fmt.Errorf("BuildUpdateQuery: no columns provided for table %s", table)
	}

	setClauses := make([]string, len(columns))
	for i, col := range columns {
		setClauses[i] = fmt.Sprintf("%s = ?", col)
	}

	query := fmt.Sprintf(
		"UPDATE %s SET %s",
		table,
		strings.Join(setClauses, ", "),
	)

	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	return query, nil
}

// BuildBulkInsertQuery builds a bulk INSERT query for multiple rows
func BuildBulkInsertQuery(table string, columns []string, rowCount int) (string, error) {
	if len(columns) == 0 {
		return "", fmt.Errorf("BuildBulkInsertQuery: no columns provided for table %s", table)
	}
	if rowCount <= 0 {
		return "", fmt.Errorf("BuildBulkInsertQuery: invalid row count %d for table %s", rowCount, table)
	}

	valuePlaceholders := make([]string, rowCount)
	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	singleRow := "(" + strings.Join(placeholders, ", ") + ")"

	for i := range valuePlaceholders {
		valuePlaceholders[i] = singleRow
	}

	return fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s",
		table,
		strings.Join(columns, ", "),
		strings.Join(valuePlaceholders, ", "),
	), nil
}

// NullString converts a string to sql.NullString
func NullString(s string) sql.NullString {
	return sql.NullString{
		String: s,
		Valid:  s != "",
	}
}

// NullInt64 converts an int64 to sql.NullInt64
func NullInt64(i int64, valid bool) sql.NullInt64 {
	return sql.NullInt64{
		Int64: i,
		Valid: valid,
	}
}

// NullTime converts a time.Time to sql.NullTime
func NullTime(t time.Time) sql.NullTime {
	return sql.NullTime{
		Time:  t,
		Valid: !t.IsZero(),
	}
}

// StringFromNull safely extracts string from sql.NullString
func StringFromNull(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// Int64FromNull safely extracts int64 from sql.NullInt64
func Int64FromNull(ni sql.NullInt64) int64 {
	if ni.Valid {
		return ni.Int64
	}
	return 0
}

// TimeFromNull safely extracts time.Time from sql.NullTime
func TimeFromNull(nt sql.NullTime) time.Time {
	if nt.Valid {
		return nt.Time
	}
	return time.Time{}
}

// CountRows executes a COUNT query
func CountRows(ctx context.Context, conn *Connection, query string, args ...any) (int64, error) {
	var count int64
	err := conn.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, errors.NewDatabaseError("count_rows", err).
			WithDetail("query", query)
	}
	return count, nil
}

// ExistsQuery checks if any rows exist for a query
func ExistsQuery(ctx context.Context, conn *Connection, query string, args ...any) (bool, error) {
	var exists bool
	existsQuery := fmt.Sprintf("SELECT EXISTS(%s)", strings.TrimSuffix(query, ";"))

	err := conn.QueryRowContext(ctx, existsQuery, args...).Scan(&exists)
	if err != nil {
		return false, errors.NewDatabaseError("exists_query", err).
			WithDetail("query", query)
	}

	return exists, nil
}

// BatchOperation represents a batch database operation
type BatchOperation struct {
	Query string
	Args  []any
}

// ExecuteBatch executes multiple operations in a single transaction
func ExecuteBatch(ctx context.Context, conn *Connection, operations []BatchOperation) error {
	return conn.Transaction(ctx, func(tx *sql.Tx) error {
		for i, op := range operations {
			if _, err := tx.ExecContext(ctx, op.Query, op.Args...); err != nil {
				return errors.NewDatabaseError("execute_batch", err).
					WithDetail("operation_index", i).
					WithDetail("query", op.Query)
			}
		}
		return nil
	})
}

// TableExists checks if a table exists in the database
func TableExists(ctx context.Context, conn *Connection, tableName string) (bool, error) {
	query := `
		SELECT name FROM sqlite_master 
		WHERE type='table' AND name=?
	`

	var name string
	err := conn.QueryRowContext(ctx, query, tableName).Scan(&name)

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, errors.NewDatabaseError("table_exists", err).
			WithDetail("table", tableName)
	}

	return true, nil
}

// VacuumDatabase performs VACUUM operation to reclaim space
func VacuumDatabase(ctx context.Context, conn *Connection) error {
	_, err := conn.ExecContext(ctx, "VACUUM")
	if err != nil {
		return errors.NewDatabaseError("vacuum_database", err)
	}
	return nil
}

// AnalyzeDatabase updates SQLite statistics for query optimization
func AnalyzeDatabase(ctx context.Context, conn *Connection) error {
	_, err := conn.ExecContext(ctx, "ANALYZE")
	if err != nil {
		return errors.NewDatabaseError("analyze_database", err)
	}
	return nil
}
