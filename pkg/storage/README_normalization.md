# Database Schema Normalization

This document describes the database schema normalization implemented for PandaFuzz.

## Overview

The original schema used JSON columns to store structured data like bot capabilities, job configurations, and corpus files. This has been normalized into proper relational tables for better performance, querying, and data integrity.

## Schema Changes

### 1. Bot Capabilities
**Before**: `bots.capabilities` stored as JSON array
**After**: Normalized into `bot_capabilities` table with foreign key relationship

```sql
CREATE TABLE bot_capabilities (
    bot_id TEXT NOT NULL,
    capability TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (bot_id, capability),
    FOREIGN KEY (bot_id) REFERENCES bots(id) ON DELETE CASCADE
)
```

### 2. Job Configuration
**Before**: `jobs.config` stored as JSON object
**After**: Normalized into `job_configs` table

```sql
CREATE TABLE job_configs (
    job_id TEXT PRIMARY KEY,
    memory_limit INTEGER,
    timeout_seconds INTEGER,
    max_iterations INTEGER,
    dictionary_path TEXT,
    seed_corpus TEXT,
    extra_args TEXT,      -- Still JSON for flexibility
    env_vars TEXT,        -- Still JSON for key-value pairs
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
)
```

### 3. Corpus Files
**Before**: `corpus_updates.files` stored as JSON array
**After**: Normalized into `corpus_files` table

```sql
CREATE TABLE corpus_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    corpus_update_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    file_hash TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (corpus_update_id) REFERENCES corpus_updates(id) ON DELETE CASCADE
)
```

## Benefits

1. **Better Query Performance**: Can query directly on capabilities, config values, or files
2. **Data Integrity**: Foreign key constraints ensure referential integrity
3. **Easier Aggregations**: Can easily count capabilities, sum file sizes, etc.
4. **Type Safety**: Proper column types instead of JSON strings
5. **Indexing**: Can add indexes on frequently queried columns

## Migration

The migration is automatically applied when the storage is initialized. Existing data in JSON columns is preserved and can be migrated using the provided migration functions.

## Usage Examples

### Query bots by capability
```sql
SELECT DISTINCT b.* 
FROM bots b
JOIN bot_capabilities bc ON b.id = bc.bot_id
WHERE bc.capability IN ('afl++', 'libfuzzer')
```

### Get job statistics
```sql
SELECT 
    j.id,
    j.name,
    COUNT(DISTINCT c.hash) as unique_crashes,
    MAX(cov.edges) as max_coverage
FROM jobs j
LEFT JOIN crashes c ON j.id = c.job_id
LEFT JOIN coverage cov ON j.id = cov.job_id
GROUP BY j.id, j.name
```

### Find large corpus files
```sql
SELECT cu.job_id, cf.file_path, cf.file_size
FROM corpus_files cf
JOIN corpus_updates cu ON cf.corpus_update_id = cu.id
WHERE cf.file_size > 1048576  -- Files larger than 1MB
ORDER BY cf.file_size DESC
```

## Implementation

The normalized schema is implemented in:
- `migrations.go`: Migration definitions
- `normalized_queries.go`: Helper functions for working with normalized tables
- Updated `SQLiteStorage` methods to use the new schema when available