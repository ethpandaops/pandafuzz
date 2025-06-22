# PandaFuzz Storage Architecture

## Overview

PandaFuzz uses a master-centric storage architecture where only the master writes to the filesystem. This ensures data consistency, prevents conflicts, and enables atomic operations for critical state changes.

## Storage Components

### 1. Persistent Database (SQLite/BadgerDB)

**Location**: `/storage/data/pandafuzz.db`

**Purpose**: Store all metadata and system state for recovery

**Schema**:
```sql
-- Bots table
CREATE TABLE bots (
    id TEXT PRIMARY KEY,
    hostname TEXT NOT NULL,
    status TEXT NOT NULL,
    last_seen DATETIME NOT NULL,
    registered_at DATETIME NOT NULL,
    current_job TEXT,
    capabilities TEXT, -- JSON array
    timeout_at DATETIME NOT NULL,
    failure_count INTEGER DEFAULT 0
);

-- Jobs table
CREATE TABLE jobs (
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
    config TEXT -- JSON object
);

-- Crash results
CREATE TABLE crashes (
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
    FOREIGN KEY(job_id) REFERENCES jobs(id),
    FOREIGN KEY(bot_id) REFERENCES bots(id)
);

-- Coverage results
CREATE TABLE coverage (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    bot_id TEXT NOT NULL,
    edges INTEGER NOT NULL,
    new_edges INTEGER NOT NULL,
    timestamp DATETIME NOT NULL,
    exec_count INTEGER NOT NULL,
    FOREIGN KEY(job_id) REFERENCES jobs(id),
    FOREIGN KEY(bot_id) REFERENCES bots(id)
);

-- Corpus updates
CREATE TABLE corpus_updates (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    bot_id TEXT NOT NULL,
    files TEXT NOT NULL, -- JSON array
    timestamp DATETIME NOT NULL,
    total_size INTEGER NOT NULL,
    FOREIGN KEY(job_id) REFERENCES jobs(id),
    FOREIGN KEY(bot_id) REFERENCES bots(id)
);

-- Job assignments (for atomic operations)
CREATE TABLE job_assignments (
    job_id TEXT PRIMARY KEY,
    bot_id TEXT NOT NULL,
    timestamp DATETIME NOT NULL,
    status TEXT NOT NULL, -- "assigned", "started", "completed"
    FOREIGN KEY(job_id) REFERENCES jobs(id),
    FOREIGN KEY(bot_id) REFERENCES bots(id)
);

-- System metadata
CREATE TABLE metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME NOT NULL
);
```

### 2. Filesystem Structure

**Master-Only Write Access**:
```
/storage/
├── data/
│   └── pandafuzz.db              # SQLite database
├── corpus/
│   └── {job_id}/                 # Job-specific corpus isolation
│       ├── input_000001
│       ├── input_000002
│       └── metadata.json         # Corpus metadata
├── crashes/
│   └── {job_id}/                 # Job-specific crash isolation
│       ├── crash_001.input
│       ├── crash_002.input
│       └── metadata.json         # Crash metadata
├── jobs/
│   └── {job_id}/                 # Job working directories
│       ├── target               # Target binary
│       ├── dictionary           # Optional fuzzing dictionary
│       ├── seeds/               # Initial seed inputs
│       └── output/              # Fuzzer output directory
├── metadata/
│   ├── system.json              # System configuration
│   ├── recovery.json            # Recovery state information
│   └── stats.json               # Runtime statistics
└── logs/
    ├── master.log               # Master logs
    ├── bot_{id}.log             # Per-bot logs
    └── job_{id}.log             # Per-job logs
```

### 3. Bot Communication Protocol

**Bots Never Write to Shared Storage**:
- Bots execute fuzzers in local temporary directories
- All results communicated via HTTP API to master
- Master handles all persistent storage operations
- Bots receive corpus/seed data via API responses

## Atomic Operations

### 1. Job Assignment

```go
func (s *State) AtomicJobAssignment(botID string) (*Job, error) {
    return s.db.Transaction(func(tx Transaction) error {
        // 1. Find available job
        job, err := s.findAvailableJob(tx)
        if err != nil {
            return err
        }
        
        // 2. Update job status
        job.Status = JobStatusAssigned
        job.AssignedBot = &botID
        job.StartedAt = &time.Now()
        
        // 3. Update bot status
        bot, err := s.getBot(tx, botID)
        if err != nil {
            return err
        }
        bot.Status = BotStatusBusy
        bot.CurrentJob = &job.ID
        
        // 4. Create assignment record
        assignment := &JobAssignment{
            JobID:     job.ID,
            BotID:     botID,
            Timestamp: time.Now(),
            Status:    "assigned",
        }
        
        // 5. Persist all changes atomically
        if err := tx.Store("job:"+job.ID, job); err != nil {
            return err
        }
        if err := tx.Store("bot:"+botID, bot); err != nil {
            return err
        }
        if err := tx.Store("assignment:"+job.ID, assignment); err != nil {
            return err
        }
        
        return nil
    })
}
```

### 2. Result Processing

```go
func (s *State) ProcessCrashResult(crash *CrashResult) error {
    return s.db.Transaction(func(tx Transaction) error {
        // 1. Check for duplicates
        duplicate, err := s.checkCrashDuplicate(tx, crash.Hash)
        if err != nil {
            return err
        }
        
        if duplicate {
            crash.IsUnique = false
        }
        
        // 2. Store crash metadata
        if err := tx.Store("crash:"+crash.ID, crash); err != nil {
            return err
        }
        
        // 3. Update job statistics
        job, err := s.getJob(tx, crash.JobID)
        if err != nil {
            return err
        }
        
        // Update crash count, etc.
        if err := tx.Store("job:"+job.ID, job); err != nil {
            return err
        }
        
        // 4. Write crash file to filesystem (master-only)
        crashPath := s.getCrashPath(crash.JobID, crash.ID)
        if err := s.writeCrashFile(crashPath, crash); err != nil {
            return err
        }
        
        return nil
    })
}
```

## Recovery Procedures

### 1. Master Startup Recovery

```go
func (s *State) RecoverOnStartup() error {
    // 1. Load all persistent state from database
    if err := s.loadPersistedState(); err != nil {
        return err
    }
    
    // 2. Check for orphaned jobs
    orphanedJobs, err := s.findOrphanedJobs()
    if err != nil {
        return err
    }
    
    // 3. Reset timed-out bots
    timedOutBots, err := s.findTimedOutBots()
    if err != nil {
        return err
    }
    
    for _, botID := range timedOutBots {
        if err := s.resetBot(botID); err != nil {
            log.Errorf("Failed to reset bot %s: %v", botID, err)
        }
    }
    
    // 4. Reassign orphaned jobs
    for _, job := range orphanedJobs {
        job.Status = JobStatusPending
        job.AssignedBot = nil
        if err := s.SaveJob(job); err != nil {
            log.Errorf("Failed to reset job %s: %v", job.ID, err)
        }
    }
    
    // 5. Cleanup stale data
    if err := s.cleanupStaleData(); err != nil {
        log.Errorf("Failed to cleanup stale data: %v", err)
    }
    
    return nil
}
```

### 2. Bot Failure Handling

```go
func (s *State) HandleBotFailure(botID string) error {
    return s.db.Transaction(func(tx Transaction) error {
        // 1. Mark bot as failed
        bot, err := s.getBot(tx, botID)
        if err != nil {
            return err
        }
        
        bot.Status = BotStatusFailed
        bot.FailureCount++
        
        // 2. Handle current job
        if bot.CurrentJob != nil {
            job, err := s.getJob(tx, *bot.CurrentJob)
            if err != nil {
                return err
            }
            
            // Reset job for reassignment
            job.Status = JobStatusPending
            job.AssignedBot = nil
            
            if err := tx.Store("job:"+job.ID, job); err != nil {
                return err
            }
        }
        
        // 3. Clear bot's current job
        bot.CurrentJob = nil
        
        // 4. Persist changes
        if err := tx.Store("bot:"+botID, bot); err != nil {
            return err
        }
        
        return nil
    })
}
```

## Corpus Metadata Management

### 1. Persistent Corpus Metadata

```go
type CorpusMetadata struct {
    JobID       string            `json:"job_id"`
    FileCount   int               `json:"file_count"`
    TotalSize   int64             `json:"total_size"`
    LastUpdated time.Time         `json:"last_updated"`
    FileHashes  map[string]string `json:"file_hashes"` // filename -> sha256
}

func (s *State) UpdateCorpusMetadata(jobID string, files []string) error {
    corpusPath := s.getCorpusPath(jobID)
    metadataPath := filepath.Join(corpusPath, "metadata.json")
    
    // Calculate metadata
    metadata := &CorpusMetadata{
        JobID:       jobID,
        FileCount:   len(files),
        LastUpdated: time.Now(),
        FileHashes:  make(map[string]string),
    }
    
    var totalSize int64
    for _, file := range files {
        filePath := filepath.Join(corpusPath, file)
        
        // Calculate file hash
        hash, size, err := s.calculateFileHash(filePath)
        if err != nil {
            return err
        }
        
        metadata.FileHashes[file] = hash
        totalSize += size
    }
    
    metadata.TotalSize = totalSize
    
    // Write metadata atomically
    return s.writeJSONFile(metadataPath, metadata)
}
```

### 2. File Deduplication

```go
func (s *State) DeduplicateCorpusFile(jobID, fileName string, content []byte) (bool, error) {
    hash := sha256.Sum256(content)
    hashStr := hex.EncodeToString(hash[:])
    
    // Check if file already exists
    metadata, err := s.getCorpusMetadata(jobID)
    if err != nil {
        return false, err
    }
    
    for _, existingHash := range metadata.FileHashes {
        if existingHash == hashStr {
            return true, nil // Duplicate found
        }
    }
    
    return false, nil // Not a duplicate
}
```

## Timeout Management

### 1. Timeout Tracking

```go
type TimeoutManager struct {
    state *State
    mu    sync.RWMutex
}

func (tm *TimeoutManager) CheckTimeouts() error {
    now := time.Now()
    
    // Check bot timeouts
    timedOutBots, err := tm.state.findBotsWithTimeout(now)
    if err != nil {
        return err
    }
    
    for _, botID := range timedOutBots {
        if err := tm.state.HandleBotTimeout(botID); err != nil {
            log.Errorf("Failed to handle bot timeout %s: %v", botID, err)
        }
    }
    
    // Check job timeouts
    timedOutJobs, err := tm.state.findJobsWithTimeout(now)
    if err != nil {
        return err
    }
    
    for _, jobID := range timedOutJobs {
        if err := tm.state.HandleJobTimeout(jobID); err != nil {
            log.Errorf("Failed to handle job timeout %s: %v", jobID, err)
        }
    }
    
    return nil
}
```

## Security Considerations

### 1. Input Validation

```go
func (s *State) ValidateCrashReport(crash *CrashResult) error {
    // Validate job ID exists
    if _, err := s.getJob(nil, crash.JobID); err != nil {
        return fmt.Errorf("invalid job_id: %v", err)
    }
    
    // Validate bot ID exists
    if _, err := s.getBot(nil, crash.BotID); err != nil {
        return fmt.Errorf("invalid bot_id: %v", err)
    }
    
    // Validate file path is within job directory
    jobPath := s.getJobPath(crash.JobID)
    absPath := filepath.Join(jobPath, crash.FilePath)
    if !strings.HasPrefix(absPath, jobPath) {
        return fmt.Errorf("invalid file path: outside job directory")
    }
    
    // Validate file size limits
    if crash.Size > maxCrashSize {
        return fmt.Errorf("crash file too large: %d bytes", crash.Size)
    }
    
    return nil
}
```

### 2. Resource Limits

```go
type ResourceLimits struct {
    MaxCorpusSize     int64         // Maximum corpus size per job
    MaxCrashSize      int64         // Maximum crash file size
    MaxCrashCount     int           // Maximum crashes per job
    MaxJobDuration    time.Duration // Maximum job runtime
    MaxConcurrentJobs int           // Maximum concurrent jobs
}

func (s *State) EnforceResourceLimits(jobID string) error {
    // Check corpus size limit
    corpusSize, err := s.getCorpusSize(jobID)
    if err != nil {
        return err
    }
    
    if corpusSize > s.limits.MaxCorpusSize {
        return fmt.Errorf("corpus size limit exceeded: %d bytes", corpusSize)
    }
    
    // Check crash count limit
    crashCount, err := s.getCrashCount(jobID)
    if err != nil {
        return err
    }
    
    if crashCount > s.limits.MaxCrashCount {
        return fmt.Errorf("crash count limit exceeded: %d crashes", crashCount)
    }
    
    return nil
}
```

This storage architecture ensures:
1. **Data Consistency**: Only master writes to filesystem
2. **Atomic Operations**: Database transactions for critical operations
3. **Fault Tolerance**: Complete state recovery from persistent storage
4. **Isolation**: Job-specific directories prevent conflicts
5. **Deduplication**: Hash-based duplicate detection
6. **Resource Management**: Configurable limits and monitoring