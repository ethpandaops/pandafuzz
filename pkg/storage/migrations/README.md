# PandaFuzz Database Migrations

This directory contains the database migration system for PandaFuzz.

## Overview

The migration system provides versioned schema management with the following features:
- Sequential migration execution
- Rollback support
- Migration status tracking
- Embedded SQL files for easy deployment
- Atomic transactions for each migration

## Migration File Format

Migration files follow the naming convention: `XXX_migration_name.sql`

- `XXX`: Three-digit version number (e.g., 001, 002, 003)
- `migration_name`: Descriptive name using underscores

Each migration file contains:
```sql
-- Description: Brief description of the migration

-- +migrate Up
-- SQL statements for applying the migration

-- +migrate Down
-- SQL statements for rolling back the migration
```

## Using the Migration Tool

Build the migration tool:
```bash
go build -o migrate ./cmd/migration
```

### Commands

#### Apply all pending migrations
```bash
./migrate up
./migrate -v up  # Verbose output
```

#### Roll back the last migration
```bash
./migrate down
```

#### Check migration status
```bash
./migrate status
```

#### Reset database (DANGEROUS - drops all data)
```bash
./migrate reset
```

#### Create a new migration template
```bash
./migrate create add_new_feature
```

### Options

- `-db <path>`: Specify database file path (default: ./pandafuzz.db)
- `-v`: Enable verbose output

## Writing Migrations

### Best Practices

1. **Keep migrations small and focused**: Each migration should do one thing
2. **Always provide rollback**: Include a Down migration for every Up
3. **Test migrations**: Test both Up and Down paths before committing
4. **Use transactions**: Migrations run in transactions automatically
5. **Be careful with data migrations**: Test with production-like data

### Common Patterns

#### Adding a table
```sql
-- +migrate Up
CREATE TABLE new_table (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- +migrate Down
DROP TABLE IF EXISTS new_table;
```

#### Adding columns (SQLite limitation)
```sql
-- +migrate Up
ALTER TABLE existing_table ADD COLUMN new_column TEXT;

-- +migrate Down
-- SQLite doesn't support DROP COLUMN
-- Need to recreate table without the column
```

#### Adding indices
```sql
-- +migrate Up
CREATE INDEX idx_table_column ON table(column);

-- +migrate Down
DROP INDEX IF EXISTS idx_table_column;
```

## Integration with Application

The migration system is integrated into the storage layer:

```go
import "github.com/parithosh/pandafuzz/pkg/storage/migrations"

// In your storage initialization
db, _ := sql.Open("sqlite3", dbPath)
ms, _ := migrations.NewMigrationSystem(db)

// Run migrations on startup
if err := ms.Up(); err != nil {
    log.Fatal("Failed to run migrations:", err)
}
```

## Current Migrations

1. **001_initial_schema.sql**: Creates base tables (jobs, corpus, crashes, logs, metrics, client_status)
2. **002_add_indices.sql**: Adds performance indices for common queries
3. **003_add_corpus_metadata.sql.example**: Example migration for adding corpus metadata

## Troubleshooting

### Migration fails
- Check the error message for SQL syntax issues
- Verify foreign key constraints
- Ensure the Down migration works correctly

### Cannot rollback
- Some migrations may not be reversible (e.g., data deletions)
- Always backup before running migrations in production

### Database locked
- Ensure no other processes are accessing the database
- SQLite has limited concurrent access support