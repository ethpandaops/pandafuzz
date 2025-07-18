package migrations

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed *.sql
var migrationFiles embed.FS

// Migration represents a single database migration
type Migration struct {
	Version     int
	Name        string
	Description string
	UpSQL       string
	DownSQL     string
	AppliedAt   *time.Time
}

// MigrationSystem manages database migrations
type MigrationSystem struct {
	db         *sql.DB
	migrations []Migration
}

// NewMigrationSystem creates a new migration system
func NewMigrationSystem(db *sql.DB) (*MigrationSystem, error) {
	ms := &MigrationSystem{
		db:         db,
		migrations: []Migration{},
	}

	// Ensure migrations table exists
	if err := ms.createMigrationsTable(); err != nil {
		return nil, fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Load migration files
	if err := ms.loadMigrations(); err != nil {
		return nil, fmt.Errorf("failed to load migrations: %w", err)
	}

	return ms, nil
}

// createMigrationsTable ensures the migrations tracking table exists
func (ms *MigrationSystem) createMigrationsTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`

	_, err := ms.db.Exec(query)
	return err
}

// loadMigrations loads all migration files from the embedded filesystem
func (ms *MigrationSystem) loadMigrations() error {
	entries, err := migrationFiles.ReadDir(".")
	if err != nil {
		return fmt.Errorf("failed to read migration files: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		migration, err := ms.parseMigrationFile(entry.Name())
		if err != nil {
			return fmt.Errorf("failed to parse migration %s: %w", entry.Name(), err)
		}

		ms.migrations = append(ms.migrations, migration)
	}

	// Sort migrations by version
	sort.Slice(ms.migrations, func(i, j int) bool {
		return ms.migrations[i].Version < ms.migrations[j].Version
	})

	return nil
}

// parseMigrationFile parses a migration file and extracts version, name, and SQL
func (ms *MigrationSystem) parseMigrationFile(filename string) (Migration, error) {
	// Expected format: 001_initial_schema.sql
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) != 2 {
		return Migration{}, fmt.Errorf("invalid migration filename format: %s", filename)
	}

	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return Migration{}, fmt.Errorf("invalid version number in filename %s: %w", filename, err)
	}

	name := strings.TrimSuffix(parts[1], ".sql")

	content, err := migrationFiles.ReadFile(filename)
	if err != nil {
		return Migration{}, fmt.Errorf("failed to read migration file %s: %w", filename, err)
	}

	upSQL, downSQL, description := parseMigrationContent(string(content))

	return Migration{
		Version:     version,
		Name:        name,
		Description: description,
		UpSQL:       upSQL,
		DownSQL:     downSQL,
	}, nil
}

// parseMigrationContent splits migration content into UP and DOWN sections
func parseMigrationContent(content string) (upSQL, downSQL, description string) {
	lines := strings.Split(content, "\n")
	var currentSection string
	var upBuilder, downBuilder strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for description comment
		if strings.HasPrefix(trimmed, "-- Description:") {
			description = strings.TrimSpace(strings.TrimPrefix(trimmed, "-- Description:"))
			continue
		}

		// Check for section markers
		if strings.HasPrefix(trimmed, "-- +migrate Up") {
			currentSection = "up"
			continue
		}
		if strings.HasPrefix(trimmed, "-- +migrate Down") {
			currentSection = "down"
			continue
		}

		// Add line to appropriate section
		switch currentSection {
		case "up":
			upBuilder.WriteString(line + "\n")
		case "down":
			downBuilder.WriteString(line + "\n")
		}
	}

	return strings.TrimSpace(upBuilder.String()), strings.TrimSpace(downBuilder.String()), description
}

// Up runs all pending migrations
func (ms *MigrationSystem) Up() error {
	applied, err := ms.getAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	appliedMap := make(map[int]bool)
	for _, version := range applied {
		appliedMap[version] = true
	}

	tx, err := ms.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, migration := range ms.migrations {
		if appliedMap[migration.Version] {
			continue
		}

		fmt.Printf("Applying migration %03d: %s\n", migration.Version, migration.Name)

		if err := ms.applyMigration(tx, migration); err != nil {
			return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
		}
	}

	return tx.Commit()
}

// Down rolls back the last applied migration
func (ms *MigrationSystem) Down() error {
	applied, err := ms.getAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	if len(applied) == 0 {
		fmt.Println("No migrations to roll back")
		return nil
	}

	// Get the last applied version
	lastVersion := applied[len(applied)-1]

	// Find the migration
	var migration *Migration
	for i := range ms.migrations {
		if ms.migrations[i].Version == lastVersion {
			migration = &ms.migrations[i]
			break
		}
	}

	if migration == nil {
		return fmt.Errorf("migration %d not found", lastVersion)
	}

	if migration.DownSQL == "" {
		return fmt.Errorf("migration %d has no down migration", lastVersion)
	}

	tx, err := ms.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	fmt.Printf("Rolling back migration %03d: %s\n", migration.Version, migration.Name)

	// Execute down migration
	if _, err := tx.Exec(migration.DownSQL); err != nil {
		return fmt.Errorf("failed to execute down migration: %w", err)
	}

	// Remove from migrations table
	if _, err := tx.Exec("DELETE FROM schema_migrations WHERE version = ?", migration.Version); err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	return tx.Commit()
}

// Status returns the current migration status
func (ms *MigrationSystem) Status() ([]MigrationStatus, error) {
	appliedMap := make(map[int]time.Time)

	// Get application times
	rows, err := ms.db.Query("SELECT version, applied_at FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to query migrations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var version int
		var appliedAt time.Time
		if err := rows.Scan(&version, &appliedAt); err != nil {
			return nil, fmt.Errorf("failed to scan migration row: %w", err)
		}
		appliedMap[version] = appliedAt
	}

	var statuses []MigrationStatus
	for _, migration := range ms.migrations {
		status := MigrationStatus{
			Version:     migration.Version,
			Name:        migration.Name,
			Description: migration.Description,
		}

		if appliedAt, ok := appliedMap[migration.Version]; ok {
			status.Applied = true
			status.AppliedAt = &appliedAt
		}

		statuses = append(statuses, status)
	}

	return statuses, nil
}

// MigrationStatus represents the status of a migration
type MigrationStatus struct {
	Version     int
	Name        string
	Description string
	Applied     bool
	AppliedAt   *time.Time
}

// getAppliedMigrations returns a list of applied migration versions
func (ms *MigrationSystem) getAppliedMigrations() ([]int, error) {
	rows, err := ms.db.Query("SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []int
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}

	return versions, rows.Err()
}

// applyMigration applies a single migration
func (ms *MigrationSystem) applyMigration(tx *sql.Tx, migration Migration) error {
	// Execute migration SQL
	if _, err := tx.Exec(migration.UpSQL); err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	// Record migration
	_, err := tx.Exec(
		"INSERT INTO schema_migrations (version, name, description) VALUES (?, ?, ?)",
		migration.Version, migration.Name, migration.Description,
	)
	return err
}

// Reset drops all tables and reruns all migrations (dangerous!)
func (ms *MigrationSystem) Reset() error {
	// This is a dangerous operation - should be used only in development
	fmt.Println("WARNING: This will drop all tables and data!")

	// Get all tables
	rows, err := ms.db.Query(`
		SELECT name FROM sqlite_master 
		WHERE type='table' AND name NOT LIKE 'sqlite_%'
	`)
	if err != nil {
		return fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, table)
	}

	tx, err := ms.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Drop all tables
	for _, table := range tables {
		if _, err := tx.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", table)); err != nil {
			return fmt.Errorf("failed to drop table %s: %w", table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Recreate migrations table
	if err := ms.createMigrationsTable(); err != nil {
		return fmt.Errorf("failed to recreate migrations table: %w", err)
	}

	// Run all migrations
	return ms.Up()
}

// CreateMigration creates a new migration file template
func CreateMigration(name string) error {
	// Find the next version number
	entries, err := migrationFiles.ReadDir(".")
	if err != nil {
		return fmt.Errorf("failed to read migration files: %w", err)
	}

	maxVersion := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		if version > maxVersion {
			maxVersion = version
		}
	}

	nextVersion := maxVersion + 1
	filename := fmt.Sprintf("%03d_%s.sql", nextVersion, name)

	template := `-- Description: %s

-- +migrate Up
-- SQL for applying the migration


-- +migrate Down
-- SQL for rolling back the migration

`

	// Note: In actual implementation, this would write to the filesystem
	// For embedded files, migrations need to be added at compile time
	fmt.Printf("Migration template created: %s\n", filename)
	fmt.Printf(template, name)

	return nil
}
