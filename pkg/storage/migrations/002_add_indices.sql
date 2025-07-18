-- Description: Add performance indices for common queries

-- +migrate Up
-- Index for job status queries
CREATE INDEX idx_jobs_status ON jobs(status);
CREATE INDEX idx_jobs_client_id ON jobs(client_id);
CREATE INDEX idx_jobs_created_at ON jobs(created_at);

-- Index for corpus queries
CREATE INDEX idx_corpus_job_id ON corpus(job_id);
CREATE INDEX idx_corpus_created_at ON corpus(created_at);
CREATE INDEX idx_corpus_size ON corpus(size);

-- Index for crash queries
CREATE INDEX idx_crashes_job_id ON crashes(job_id);
CREATE INDEX idx_crashes_created_at ON crashes(created_at);
CREATE INDEX idx_crashes_reproduced ON crashes(reproduced);

-- Index for log queries
CREATE INDEX idx_logs_job_id ON logs(job_id);
CREATE INDEX idx_logs_level ON logs(level);
CREATE INDEX idx_logs_created_at ON logs(created_at);

-- Index for metrics queries
CREATE INDEX idx_metrics_job_id ON metrics(job_id);
CREATE INDEX idx_metrics_name ON metrics(metric_name);
CREATE INDEX idx_metrics_created_at ON metrics(created_at);

-- Index for client status queries
CREATE INDEX idx_client_status_status ON client_status(status);
CREATE INDEX idx_client_status_last_heartbeat ON client_status(last_heartbeat);

-- +migrate Down
-- Drop all indices
DROP INDEX IF EXISTS idx_jobs_status;
DROP INDEX IF EXISTS idx_jobs_client_id;
DROP INDEX IF EXISTS idx_jobs_created_at;

DROP INDEX IF EXISTS idx_corpus_job_id;
DROP INDEX IF EXISTS idx_corpus_created_at;
DROP INDEX IF EXISTS idx_corpus_size;

DROP INDEX IF EXISTS idx_crashes_job_id;
DROP INDEX IF EXISTS idx_crashes_created_at;
DROP INDEX IF EXISTS idx_crashes_reproduced;

DROP INDEX IF EXISTS idx_logs_job_id;
DROP INDEX IF EXISTS idx_logs_level;
DROP INDEX IF EXISTS idx_logs_created_at;

DROP INDEX IF EXISTS idx_metrics_job_id;
DROP INDEX IF EXISTS idx_metrics_name;
DROP INDEX IF EXISTS idx_metrics_created_at;

DROP INDEX IF EXISTS idx_client_status_status;
DROP INDEX IF EXISTS idx_client_status_last_heartbeat;