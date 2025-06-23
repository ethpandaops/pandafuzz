package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// Migration represents a database migration
type Migration struct {
	ID          string
	Description string
	Up          func(*sql.Tx) error
	Down        func(*sql.Tx) error
}

// GetMigrations returns all database migrations
func GetMigrations() []Migration {
	return []Migration{
		{
			ID:          "001_normalize_schema",
			Description: "Normalize database schema by extracting JSON fields",
			Up:          normalizeSchemaUp,
			Down:        normalizeSchemaDown,
		},
		{
			ID:          "002_add_bot_api_endpoint",
			Description: "Add api_endpoint column to bots table for polling",
			Up:          addBotAPIEndpointUp,
			Down:        addBotAPIEndpointDown,
		},
	}
}

// normalizeSchemaUp applies the normalization migration
func normalizeSchemaUp(tx *sql.Tx) error {
	// Create bot_capabilities table
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS bot_capabilities (
			bot_id TEXT NOT NULL,
			capability TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (bot_id, capability),
			FOREIGN KEY (bot_id) REFERENCES bots(id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("failed to create bot_capabilities table: %w", err)
	}

	// Create job_config table for normalized job configuration
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS job_configs (
			job_id TEXT PRIMARY KEY,
			memory_limit INTEGER,
			timeout_seconds INTEGER,
			max_iterations INTEGER,
			dictionary_path TEXT,
			seed_corpus TEXT,
			extra_args TEXT,
			env_vars TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("failed to create job_configs table: %w", err)
	}

	// Create corpus_files table to normalize the files array
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS corpus_files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			corpus_update_id TEXT NOT NULL,
			file_path TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			file_hash TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (corpus_update_id) REFERENCES corpus_updates(id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("failed to create corpus_files table: %w", err)
	}

	// Create indices for better query performance
	indices := []string{
		"CREATE INDEX IF NOT EXISTS idx_bot_capabilities_bot_id ON bot_capabilities(bot_id)",
		"CREATE INDEX IF NOT EXISTS idx_job_configs_job_id ON job_configs(job_id)",
		"CREATE INDEX IF NOT EXISTS idx_corpus_files_update_id ON corpus_files(corpus_update_id)",
		"CREATE INDEX IF NOT EXISTS idx_crashes_job_id ON crashes(job_id)",
		"CREATE INDEX IF NOT EXISTS idx_crashes_bot_id ON crashes(bot_id)",
		"CREATE INDEX IF NOT EXISTS idx_crashes_hash ON crashes(hash)",
		"CREATE INDEX IF NOT EXISTS idx_coverage_job_id ON coverage(job_id)",
		"CREATE INDEX IF NOT EXISTS idx_coverage_timestamp ON coverage(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status)",
		"CREATE INDEX IF NOT EXISTS idx_jobs_assigned_bot ON jobs(assigned_bot)",
		"CREATE INDEX IF NOT EXISTS idx_bots_status ON bots(status)",
		"CREATE INDEX IF NOT EXISTS idx_bots_hostname ON bots(hostname)",
	}

	for _, index := range indices {
		if _, err := tx.Exec(index); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	// Add migration record
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id TEXT PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO schema_migrations (id, applied_at) VALUES (?, ?)
	`, "001_normalize_schema", time.Now()); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return nil
}

// normalizeSchemaDown reverts the normalization migration
func normalizeSchemaDown(tx *sql.Tx) error {
	// Drop the normalized tables
	tables := []string{
		"corpus_files",
		"job_configs",
		"bot_capabilities",
		"schema_migrations",
	}

	for _, table := range tables {
		if _, err := tx.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", table)); err != nil {
			return fmt.Errorf("failed to drop table %s: %w", table, err)
		}
	}

	return nil
}

// MigrateExistingData migrates existing JSON data to normalized tables
func MigrateExistingData(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check if migration already applied
	var count int
	err = tx.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master 
		WHERE type='table' AND name='schema_migrations'
	`).Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		// Check if this specific migration was applied
		err = tx.QueryRow(`
			SELECT COUNT(*) FROM schema_migrations WHERE id = '001_normalize_schema'
		`).Scan(&count)
		if err != nil {
			return err
		}
		if count > 0 {
			return nil // Migration already applied
		}
	}

	// Apply the migration
	if err := normalizeSchemaUp(tx); err != nil {
		return err
	}

	// TODO: Migrate existing data from JSON columns to normalized tables
	// This would involve:
	// 1. Parsing bot capabilities from JSON and inserting into bot_capabilities
	// 2. Parsing job configs from JSON and inserting into job_configs
	// 3. Parsing corpus files from JSON and inserting into corpus_files

	return tx.Commit()
}

// addBotAPIEndpointUp adds the api_endpoint column to bots table
func addBotAPIEndpointUp(tx *sql.Tx) error {
	// Check if column already exists
	var count int
	err := tx.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('bots') 
		WHERE name = 'api_endpoint'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check for api_endpoint column: %w", err)
	}
	
	if count > 0 {
		return nil // Column already exists
	}
	
	// Add the api_endpoint column
	if _, err := tx.Exec(`
		ALTER TABLE bots ADD COLUMN api_endpoint TEXT DEFAULT ''
	`); err != nil {
		return fmt.Errorf("failed to add api_endpoint column: %w", err)
	}
	
	// Create index for api_endpoint
	if _, err := tx.Exec(`
		CREATE INDEX IF NOT EXISTS idx_bots_api_endpoint ON bots(api_endpoint)
	`); err != nil {
		return fmt.Errorf("failed to create api_endpoint index: %w", err)
	}
	
	return nil
}

// addBotAPIEndpointDown removes the api_endpoint column from bots table
func addBotAPIEndpointDown(tx *sql.Tx) error {
	// SQLite doesn't support dropping columns directly
	// We would need to recreate the table without the column
	// For simplicity, we'll just leave the column as is
	return nil
}