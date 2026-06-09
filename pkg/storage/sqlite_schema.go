package storage

import (
	"context"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// createTablesContext initializes the database schema with context
func (s *SQLiteStorage) createTablesContext(ctx context.Context) error {
	schema := `
	-- Bots table
	CREATE TABLE IF NOT EXISTS bots (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		hostname TEXT NOT NULL,
		status TEXT NOT NULL,
		last_seen DATETIME NOT NULL,
		registered_at DATETIME NOT NULL,
		current_job TEXT,
		capabilities TEXT, -- JSON array
		timeout_at DATETIME NOT NULL,
		is_online BOOLEAN DEFAULT FALSE,
		failure_count INTEGER DEFAULT 0,
		api_endpoint TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Jobs table
	CREATE TABLE IF NOT EXISTS jobs (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		target TEXT NOT NULL,
		fuzzer TEXT NOT NULL,
		status TEXT NOT NULL,
		assigned_bot TEXT,
		created_at DATETIME NOT NULL,
		started_at DATETIME,
		completed_at DATETIME,
		timeout_at DATETIME NOT NULL,
		work_dir TEXT NOT NULL,
		config TEXT, -- JSON object
		progress INTEGER DEFAULT 0,
		campaign_id TEXT,
		use_campaign_corpus INTEGER DEFAULT 0,
		collection_id VARCHAR(255),
		enable_coverage BOOLEAN DEFAULT FALSE,
		coverage_format TEXT,
		coverage_report_id TEXT,
		lease_token VARCHAR(64),
		lease_expires_at DATETIME,
		last_heartbeat DATETIME,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(assigned_bot) REFERENCES bots(id)
	);

	-- Crash results
	CREATE TABLE IF NOT EXISTS crashes (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		bot_id TEXT NOT NULL,
		hash TEXT NOT NULL,
		file_path TEXT NOT NULL,
		type TEXT NOT NULL,
		signal INTEGER,
		exit_code INTEGER,
		timestamp DATETIME NOT NULL,
		size INTEGER,
		is_unique BOOLEAN DEFAULT TRUE,
		output TEXT,
		stack_trace TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(job_id) REFERENCES jobs(id),
		FOREIGN KEY(bot_id) REFERENCES bots(id),
		UNIQUE(hash, job_id)
	);

	-- Coverage results
	CREATE TABLE IF NOT EXISTS coverage (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		bot_id TEXT NOT NULL,
		edges INTEGER NOT NULL,
		new_edges INTEGER NOT NULL,
		timestamp DATETIME NOT NULL,
		exec_count INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(job_id) REFERENCES jobs(id),
		FOREIGN KEY(bot_id) REFERENCES bots(id)
	);

	-- Corpus updates
	CREATE TABLE IF NOT EXISTS corpus_updates (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		bot_id TEXT NOT NULL,
		files TEXT NOT NULL, -- JSON array
		timestamp DATETIME NOT NULL,
		total_size INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(job_id) REFERENCES jobs(id),
		FOREIGN KEY(bot_id) REFERENCES bots(id)
	);

	-- Job assignments (for atomic operations)
	CREATE TABLE IF NOT EXISTS job_assignments (
		job_id TEXT PRIMARY KEY,
		bot_id TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		status TEXT NOT NULL, -- "assigned", "started", "completed"
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(job_id) REFERENCES jobs(id),
		FOREIGN KEY(bot_id) REFERENCES bots(id)
	);

	-- System metadata
	CREATE TABLE IF NOT EXISTS metadata (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME NOT NULL
	);

	-- crash input storage (separate table for binary data)
	CREATE TABLE IF NOT EXISTS crash_inputs (
		crash_id TEXT PRIMARY KEY,
		input BLOB NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(crash_id) REFERENCES crashes(id) ON DELETE CASCADE
	);

	-- Coverage reports table
	CREATE TABLE IF NOT EXISTS coverage_reports (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		format TEXT NOT NULL,
		storage_path TEXT NOT NULL,
		size INTEGER NOT NULL,
		file_type TEXT,
		fuzzer_stats_path TEXT,
		plot_data_path TEXT,
		fuzz_bitmap_path TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(job_id) REFERENCES jobs(id) ON DELETE CASCADE
	);

	-- Coverage metadata table
	CREATE TABLE IF NOT EXISTS coverage_metadata (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		report_id TEXT NOT NULL,
		total_functions INTEGER DEFAULT 0,
		covered_functions INTEGER DEFAULT 0,
		total_lines INTEGER DEFAULT 0,
		covered_lines INTEGER DEFAULT 0,
		total_branches INTEGER DEFAULT 0,
		covered_branches INTEGER DEFAULT 0,
		function_coverage REAL DEFAULT 0.0,
		line_coverage REAL DEFAULT 0.0,
		branch_coverage REAL DEFAULT 0.0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(job_id) REFERENCES jobs(id) ON DELETE CASCADE,
		FOREIGN KEY(report_id) REFERENCES coverage_reports(id) ON DELETE CASCADE
	);

	-- Job logs table for storing execution logs
	CREATE TABLE IF NOT EXISTS job_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id TEXT NOT NULL,
		level TEXT NOT NULL,
		source TEXT,
		message TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		metadata TEXT, -- JSON object
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(job_id) REFERENCES jobs(id) ON DELETE CASCADE
	);

	-- Create indexes for performance
	CREATE INDEX IF NOT EXISTS idx_bots_status ON bots(status);
	CREATE INDEX IF NOT EXISTS idx_bots_timeout ON bots(timeout_at);
	CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
	CREATE INDEX IF NOT EXISTS idx_jobs_assigned_bot ON jobs(assigned_bot);
	CREATE INDEX IF NOT EXISTS idx_jobs_timeout ON jobs(timeout_at);
	CREATE INDEX IF NOT EXISTS idx_crashes_job_id ON crashes(job_id);
	CREATE INDEX IF NOT EXISTS idx_crashes_hash ON crashes(hash);
	CREATE INDEX IF NOT EXISTS idx_crashes_timestamp ON crashes(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_coverage_job_id ON coverage(job_id);
	CREATE INDEX IF NOT EXISTS idx_corpus_job_id ON corpus_updates(job_id);
	CREATE INDEX IF NOT EXISTS idx_jobs_enable_coverage ON jobs(enable_coverage);
	CREATE INDEX IF NOT EXISTS idx_jobs_coverage_report_id ON jobs(coverage_report_id);
	CREATE INDEX IF NOT EXISTS idx_coverage_reports_job_id ON coverage_reports(job_id);
	CREATE INDEX IF NOT EXISTS idx_coverage_reports_file_type ON coverage_reports(file_type);
	CREATE INDEX IF NOT EXISTS idx_coverage_metadata_job_id ON coverage_metadata(job_id);
	CREATE INDEX IF NOT EXISTS idx_coverage_metadata_report_id ON coverage_metadata(report_id);
	CREATE INDEX IF NOT EXISTS idx_job_logs_job_id ON job_logs(job_id);
	CREATE INDEX IF NOT EXISTS idx_job_logs_timestamp ON job_logs(timestamp DESC);
	`

	_, err := s.db.ExecContext(ctx, schema)
	if err != nil {
		return common.NewDatabaseError("create_schema", err)
	}

	return nil
}

// CreateTables is already implemented in createTables
func (s *SQLiteStorage) CreateTables(ctx context.Context) error {
	return s.createTablesContext(ctx)
}

// Migrate implements database migrations
func (s *SQLiteStorage) Migrate(ctx context.Context, version int) error {
	// For now, just ensure tables exist
	return s.createTablesContext(ctx)
}
