-- Migration to create job_logs table with correct schema
-- The original 'logs' table had a different structure; this creates the proper table for job logs

-- +migrate Up
CREATE TABLE IF NOT EXISTS job_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL,
    level TEXT NOT NULL,
    source TEXT,
    message TEXT NOT NULL,
    timestamp DATETIME NOT NULL,
    metadata TEXT,
    FOREIGN KEY (job_id) REFERENCES jobs(id)
);

CREATE INDEX IF NOT EXISTS idx_job_logs_job_id ON job_logs(job_id);
CREATE INDEX IF NOT EXISTS idx_job_logs_timestamp ON job_logs(timestamp);

-- +migrate Down
DROP INDEX IF EXISTS idx_job_logs_timestamp;
DROP INDEX IF EXISTS idx_job_logs_job_id;
DROP TABLE IF EXISTS job_logs;
