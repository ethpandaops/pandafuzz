-- Migration: 004_job_scheduling.sql
-- Description: Add scheduling, locking, and retry fields to jobs table for domain Job type support
-- Created: 2026-01-15

-- Add missing columns for domain Job type
-- Using IF NOT EXISTS pattern via PRAGMA to avoid errors on re-run

-- Check if columns exist and add them if not
-- Note: SQLite requires separate ALTER TABLE statements for each column

-- Scheduling fields
ALTER TABLE jobs ADD COLUMN scheduled_at DATETIME;
ALTER TABLE jobs ADD COLUMN queued_at DATETIME;
ALTER TABLE jobs ADD COLUMN dequeue_count INTEGER DEFAULT 0;

-- Retry fields
ALTER TABLE jobs ADD COLUMN retry_count INTEGER DEFAULT 0;
ALTER TABLE jobs ADD COLUMN max_retries INTEGER DEFAULT 3;
ALTER TABLE jobs ADD COLUMN retry_delay INTEGER DEFAULT 0;

-- Locking fields for distributed processing
ALTER TABLE jobs ADD COLUMN locked_by TEXT;
ALTER TABLE jobs ADD COLUMN locked_at DATETIME;
ALTER TABLE jobs ADD COLUMN lock_expires_at DATETIME;

-- Additional metadata fields
ALTER TABLE jobs ADD COLUMN description TEXT;
ALTER TABLE jobs ADD COLUMN error_message TEXT;
ALTER TABLE jobs ADD COLUMN execution_time INTEGER DEFAULT 0;

-- Path fields for domain compatibility
ALTER TABLE jobs ADD COLUMN corpus_path TEXT;
ALTER TABLE jobs ADD COLUMN output_path TEXT;

-- Job dependencies table for dependency tracking
CREATE TABLE IF NOT EXISTS job_dependencies (
    job_id TEXT NOT NULL,
    depends_on_job_id TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (job_id, depends_on_job_id),
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE,
    FOREIGN KEY (depends_on_job_id) REFERENCES jobs(id) ON DELETE CASCADE
);

-- Create indexes for new columns used in queries
CREATE INDEX IF NOT EXISTS idx_jobs_scheduled_at ON jobs(scheduled_at);
CREATE INDEX IF NOT EXISTS idx_jobs_queued_at ON jobs(queued_at);
CREATE INDEX IF NOT EXISTS idx_jobs_locked_by ON jobs(locked_by);
CREATE INDEX IF NOT EXISTS idx_jobs_lock_expires_at ON jobs(lock_expires_at);
CREATE INDEX IF NOT EXISTS idx_job_dependencies_job_id ON job_dependencies(job_id);
CREATE INDEX IF NOT EXISTS idx_job_dependencies_depends_on ON job_dependencies(depends_on_job_id);
