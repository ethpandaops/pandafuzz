-- Description: Add coverage support for fuzzing jobs

-- +migrate Up
-- Create coverage_reports table
CREATE TABLE IF NOT EXISTS coverage_reports (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    format TEXT NOT NULL, -- 'lcov', 'llvm-cov', 'gcov', etc.
    storage_path TEXT NOT NULL,
    size INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
);

-- Create coverage_metadata table
CREATE TABLE IF NOT EXISTS coverage_metadata (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    report_id TEXT NOT NULL,
    line_coverage REAL, -- Percentage (0.0-100.0)
    function_coverage REAL, -- Percentage (0.0-100.0)
    branch_coverage REAL, -- Percentage (0.0-100.0)
    total_lines INTEGER,
    covered_lines INTEGER,
    total_functions INTEGER,
    covered_functions INTEGER,
    collected_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (report_id) REFERENCES coverage_reports(id) ON DELETE CASCADE
);

-- Add coverage columns to jobs table
ALTER TABLE jobs ADD COLUMN enable_coverage BOOLEAN DEFAULT FALSE;
ALTER TABLE jobs ADD COLUMN coverage_format TEXT;
ALTER TABLE jobs ADD COLUMN coverage_report_id TEXT;

-- Add foreign key constraint for coverage_report_id (SQLite requires recreation for FK)
-- Note: In SQLite, we can't add FK constraints after table creation, so we'll add an index instead
-- and rely on application-level integrity for now

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_coverage_reports_job_id ON coverage_reports(job_id);
CREATE INDEX IF NOT EXISTS idx_coverage_reports_created_at ON coverage_reports(created_at);
CREATE INDEX IF NOT EXISTS idx_coverage_reports_format ON coverage_reports(format);

CREATE INDEX IF NOT EXISTS idx_coverage_metadata_report_id ON coverage_metadata(report_id);
CREATE INDEX IF NOT EXISTS idx_coverage_metadata_collected_at ON coverage_metadata(collected_at);
CREATE INDEX IF NOT EXISTS idx_coverage_metadata_line_coverage ON coverage_metadata(line_coverage);

CREATE INDEX IF NOT EXISTS idx_jobs_enable_coverage ON jobs(enable_coverage);
CREATE INDEX IF NOT EXISTS idx_jobs_coverage_format ON jobs(coverage_format);
CREATE INDEX IF NOT EXISTS idx_jobs_coverage_report_id ON jobs(coverage_report_id);

-- +migrate Down
-- Drop indexes
DROP INDEX IF EXISTS idx_coverage_reports_job_id;
DROP INDEX IF EXISTS idx_coverage_reports_created_at;
DROP INDEX IF EXISTS idx_coverage_reports_format;

DROP INDEX IF EXISTS idx_coverage_metadata_report_id;
DROP INDEX IF EXISTS idx_coverage_metadata_collected_at;
DROP INDEX IF EXISTS idx_coverage_metadata_line_coverage;

DROP INDEX IF EXISTS idx_jobs_enable_coverage;
DROP INDEX IF EXISTS idx_jobs_coverage_format;
DROP INDEX IF EXISTS idx_jobs_coverage_report_id;

-- Remove columns from jobs table (SQLite doesn't support DROP COLUMN directly)
-- Note: In SQLite, dropping columns requires table recreation. For now, we'll leave them
-- as they don't cause harm and can be ignored by older code

-- Drop tables in reverse order
DROP TABLE IF EXISTS coverage_metadata;
DROP TABLE IF EXISTS coverage_reports;