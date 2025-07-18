-- Description: Initial database schema for PandaFuzz

-- +migrate Up
-- Create jobs table
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    client_id TEXT NOT NULL,
    config TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    error TEXT
);

-- Create corpus table
CREATE TABLE IF NOT EXISTS corpus (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    filename TEXT NOT NULL,
    data BLOB NOT NULL,
    size INTEGER NOT NULL,
    hash TEXT NOT NULL,
    coverage_data TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE,
    UNIQUE(job_id, hash)
);

-- Create crashes table
CREATE TABLE IF NOT EXISTS crashes (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    filename TEXT NOT NULL,
    data BLOB NOT NULL,
    size INTEGER NOT NULL,
    hash TEXT NOT NULL,
    crash_info TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    reproduced BOOLEAN DEFAULT FALSE,
    reproduction_count INTEGER DEFAULT 0,
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE,
    UNIQUE(job_id, hash)
);

-- Create logs table
CREATE TABLE IF NOT EXISTS logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL,
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    metadata TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
);

-- Create metrics table
CREATE TABLE IF NOT EXISTS metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    metric_value REAL NOT NULL,
    metric_type TEXT NOT NULL,
    metadata TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
);

-- Create client_status table
CREATE TABLE IF NOT EXISTS client_status (
    client_id TEXT PRIMARY KEY,
    hostname TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'offline',
    last_heartbeat TIMESTAMP,
    capabilities TEXT,
    current_job_id TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (current_job_id) REFERENCES jobs(id) ON DELETE SET NULL
);

-- +migrate Down
-- Drop all tables in reverse order
DROP TABLE IF EXISTS client_status;
DROP TABLE IF EXISTS metrics;
DROP TABLE IF EXISTS logs;
DROP TABLE IF EXISTS crashes;
DROP TABLE IF EXISTS corpus;
DROP TABLE IF EXISTS jobs;