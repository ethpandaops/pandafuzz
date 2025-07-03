package storage

import (
	"context"
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
		{
			ID:          "003_add_crash_output_columns",
			Description: "Add output and stack_trace columns to crashes table",
			Up:          addCrashOutputColumnsUp,
			Down:        addCrashOutputColumnsDown,
		},
		{
			ID:          "004_populate_missing_crash_inputs",
			Description: "Populate crash_inputs table for existing crashes",
			Up:          populateMissingCrashInputsUp,
			Down:        populateMissingCrashInputsDown,
		},
		{
			ID:          "005_add_job_progress",
			Description: "Add progress column to jobs table",
			Up:          addJobProgressUp,
			Down:        addJobProgressDown,
		},
		{
			ID:          "006_add_campaign_tables",
			Description: "Add campaign management tables for grouping jobs",
			Up:          addCampaignTablesUp,
			Down:        addCampaignTablesDown,
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

	return nil
}

// normalizeSchemaDown reverts the normalization migration
func normalizeSchemaDown(tx *sql.Tx) error {
	// Drop the normalized tables
	tables := []string{
		"corpus_files",
		"job_configs",
		"bot_capabilities",
	}

	for _, table := range tables {
		if _, err := tx.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", table)); err != nil {
			return fmt.Errorf("failed to drop table %s: %w", table, err)
		}
	}

	return nil
}

// MigrateExistingData runs all pending database migrations
func MigrateExistingData(ctx context.Context, db *sql.DB) error {
	// Create migrations table if it doesn't exist
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id TEXT PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get all migrations
	migrations := GetMigrations()

	for _, migration := range migrations {
		// Check if migration was already applied
		var count int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM schema_migrations WHERE id = ?
		`, migration.ID).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check migration %s: %w", migration.ID, err)
		}

		if count > 0 {
			continue // Migration already applied
		}

		// Start transaction for this migration
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %s: %w", migration.ID, err)
		}

		// Apply the migration
		if err := migration.Up(tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to apply migration %s: %w", migration.ID, err)
		}

		// Record the migration
		if _, err := tx.Exec(`
			INSERT INTO schema_migrations (id, applied_at) VALUES (?, ?)
		`, migration.ID, time.Now()); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %s: %w", migration.ID, err)
		}

		// Commit the transaction
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", migration.ID, err)
		}

		fmt.Printf("Applied migration: %s - %s\n", migration.ID, migration.Description)
	}

	return nil
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

// addCrashOutputColumnsUp adds output and stack_trace columns to crashes table
func addCrashOutputColumnsUp(tx *sql.Tx) error {
	// Check if output column already exists
	var count int
	err := tx.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('crashes') 
		WHERE name = 'output'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check for output column: %w", err)
	}

	if count == 0 {
		// Add the output column
		if _, err := tx.Exec(`
			ALTER TABLE crashes ADD COLUMN output TEXT
		`); err != nil {
			return fmt.Errorf("failed to add output column: %w", err)
		}
	}

	// Check if stack_trace column already exists
	err = tx.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('crashes') 
		WHERE name = 'stack_trace'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check for stack_trace column: %w", err)
	}

	if count == 0 {
		// Add the stack_trace column
		if _, err := tx.Exec(`
			ALTER TABLE crashes ADD COLUMN stack_trace TEXT
		`); err != nil {
			return fmt.Errorf("failed to add stack_trace column: %w", err)
		}
	}

	return nil
}

// addCrashOutputColumnsDown removes output and stack_trace columns from crashes table
func addCrashOutputColumnsDown(tx *sql.Tx) error {
	// SQLite doesn't support dropping columns directly
	// We would need to recreate the table without the columns
	// For simplicity, we'll just leave the columns as is
	return nil
}

// populateMissingCrashInputsUp populates crash_inputs for existing crashes
func populateMissingCrashInputsUp(tx *sql.Tx) error {
	// Find crashes that don't have corresponding entries in crash_inputs
	query := `
		SELECT c.id
		FROM crashes c
		LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
		WHERE ci.crash_id IS NULL
	`

	rows, err := tx.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query crashes without inputs: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var crashID string
		if err := rows.Scan(&crashID); err != nil {
			continue
		}

		// For old crashes, we'll create a placeholder input
		// This ensures the download button works, even if the actual input is lost
		placeholderInput := []byte(fmt.Sprintf("Input data for crash %s is no longer available.\nThis crash was found before input storage was implemented.", crashID))

		// Insert the placeholder input
		_, err := tx.Exec(`INSERT INTO crash_inputs (crash_id, input) VALUES (?, ?)`, crashID, placeholderInput)
		if err != nil {
			fmt.Printf("Failed to insert placeholder for crash %s: %v\n", crashID, err)
			continue
		}

		count++
	}

	fmt.Printf("Added placeholder inputs for %d crashes\n", count)
	return rows.Err()
}

// populateMissingCrashInputsDown removes populated crash inputs
func populateMissingCrashInputsDown(tx *sql.Tx) error {
	// This would remove the placeholder inputs, but it's safer to keep them
	return nil
}

// addJobProgressUp adds the progress column to jobs table
func addJobProgressUp(tx *sql.Tx) error {
	// Check if progress column already exists
	var count int
	err := tx.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('jobs') 
		WHERE name = 'progress'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check for progress column: %w", err)
	}

	if count > 0 {
		return nil // Column already exists
	}

	// Add the progress column with default value of 0
	if _, err := tx.Exec(`
		ALTER TABLE jobs ADD COLUMN progress INTEGER DEFAULT 0
	`); err != nil {
		return fmt.Errorf("failed to add progress column: %w", err)
	}

	// Update existing jobs to have progress = 0
	if _, err := tx.Exec(`
		UPDATE jobs SET progress = 0 WHERE progress IS NULL
	`); err != nil {
		return fmt.Errorf("failed to set default progress values: %w", err)
	}

	return nil
}

// addJobProgressDown removes the progress column from jobs table
func addJobProgressDown(tx *sql.Tx) error {
	// SQLite doesn't support dropping columns directly
	// We would need to recreate the table without the column
	// For simplicity, we'll just leave the column as is
	return nil
}

// addCampaignTablesUp creates all campaign-related tables
func addCampaignTablesUp(tx *sql.Tx) error {
	// Create campaigns table
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS campaigns (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL,
			target_binary TEXT NOT NULL,
			binary_hash TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME,
			auto_restart INTEGER DEFAULT 0,
			max_duration INTEGER,
			max_jobs INTEGER DEFAULT 10,
			job_template TEXT NOT NULL, -- JSON
			shared_corpus INTEGER DEFAULT 1,
			tags TEXT -- JSON array
		)
	`); err != nil {
		return fmt.Errorf("failed to create campaigns table: %w", err)
	}

	// Create campaign_jobs relationship table
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS campaign_jobs (
			campaign_id TEXT NOT NULL,
			job_id TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (campaign_id, job_id),
			FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
			FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("failed to create campaign_jobs table: %w", err)
	}

	// Create corpus_files tracking table (different from the existing one)
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS campaign_corpus_files (
			id TEXT PRIMARY KEY,
			campaign_id TEXT NOT NULL,
			job_id TEXT,
			bot_id TEXT,
			filename TEXT NOT NULL,
			hash TEXT NOT NULL UNIQUE,
			size INTEGER NOT NULL,
			coverage INTEGER DEFAULT 0,
			new_coverage INTEGER DEFAULT 0,
			parent_hash TEXT,
			generation INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME,
			is_seed INTEGER DEFAULT 0,
			FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("failed to create campaign_corpus_files table: %w", err)
	}

	// Create corpus evolution tracking table
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS corpus_evolution (
			campaign_id TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			total_files INTEGER NOT NULL,
			total_size INTEGER NOT NULL,
			total_coverage INTEGER NOT NULL,
			new_files INTEGER DEFAULT 0,
			new_coverage INTEGER DEFAULT 0,
			PRIMARY KEY (campaign_id, timestamp),
			FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("failed to create corpus_evolution table: %w", err)
	}

	// Create crash groups for deduplication
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS crash_groups (
			id TEXT PRIMARY KEY,
			campaign_id TEXT NOT NULL,
			stack_hash TEXT NOT NULL,
			first_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
			count INTEGER DEFAULT 1,
			severity TEXT,
			stack_frames TEXT NOT NULL, -- JSON
			example_crash TEXT,
			UNIQUE(campaign_id, stack_hash),
			FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("failed to create crash_groups table: %w", err)
	}

	// Create stack traces table
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS stack_traces (
			crash_id TEXT PRIMARY KEY,
			top_n_hash TEXT NOT NULL,
			full_hash TEXT NOT NULL,
			frames TEXT NOT NULL, -- JSON
			raw_trace TEXT,
			FOREIGN KEY (crash_id) REFERENCES crashes(id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("failed to create stack_traces table: %w", err)
	}

	// Add campaign_id to crashes table
	var count int
	err := tx.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('crashes') 
		WHERE name = 'campaign_id'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check for campaign_id column: %w", err)
	}

	if count == 0 {
		if _, err := tx.Exec(`
			ALTER TABLE crashes ADD COLUMN campaign_id TEXT
		`); err != nil {
			return fmt.Errorf("failed to add campaign_id column to crashes: %w", err)
		}
	}

	// Add crash_group_id to crashes table
	err = tx.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('crashes') 
		WHERE name = 'crash_group_id'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check for crash_group_id column: %w", err)
	}

	if count == 0 {
		if _, err := tx.Exec(`
			ALTER TABLE crashes ADD COLUMN crash_group_id TEXT
		`); err != nil {
			return fmt.Errorf("failed to add crash_group_id column to crashes: %w", err)
		}
	}

	// Create indices for performance
	indices := []string{
		"CREATE INDEX IF NOT EXISTS idx_campaign_status ON campaigns(status)",
		"CREATE INDEX IF NOT EXISTS idx_corpus_campaign ON campaign_corpus_files(campaign_id)",
		"CREATE INDEX IF NOT EXISTS idx_corpus_hash ON campaign_corpus_files(hash)",
		"CREATE INDEX IF NOT EXISTS idx_crash_group_campaign ON crash_groups(campaign_id)",
		"CREATE INDEX IF NOT EXISTS idx_stack_trace_hash ON stack_traces(top_n_hash)",
		"CREATE INDEX IF NOT EXISTS idx_crashes_campaign ON crashes(campaign_id)",
		"CREATE INDEX IF NOT EXISTS idx_crashes_group ON crashes(crash_group_id)",
	}

	for _, index := range indices {
		if _, err := tx.Exec(index); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// addCampaignTablesDown removes all campaign-related tables
func addCampaignTablesDown(tx *sql.Tx) error {
	// Drop indices first
	indices := []string{
		"DROP INDEX IF EXISTS idx_campaign_status",
		"DROP INDEX IF EXISTS idx_corpus_campaign",
		"DROP INDEX IF EXISTS idx_corpus_hash",
		"DROP INDEX IF EXISTS idx_crash_group_campaign",
		"DROP INDEX IF EXISTS idx_stack_trace_hash",
		"DROP INDEX IF EXISTS idx_crashes_campaign",
		"DROP INDEX IF EXISTS idx_crashes_group",
	}

	for _, index := range indices {
		if _, err := tx.Exec(index); err != nil {
			return fmt.Errorf("failed to drop index: %w", err)
		}
	}

	// Drop tables
	tables := []string{
		"corpus_evolution",
		"campaign_corpus_files",
		"stack_traces",
		"crash_groups",
		"campaign_jobs",
		"campaigns",
	}

	for _, table := range tables {
		if _, err := tx.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", table)); err != nil {
			return fmt.Errorf("failed to drop table %s: %w", table, err)
		}
	}

	// We can't easily remove columns from crashes table in SQLite
	// So we'll leave campaign_id and crash_group_id columns

	return nil
}
