# PandaFuzz - Reliable Fuzzing Orchestration Tool

## Overview
PandaFuzz is a self-hosted fuzzing orchestration tool written in Go designed for reliability and fault tolerance. It coordinates fuzzing bots with persistent state management, expecting all components to be breakable and recoverable.

## Core Design Principles
1. **Reliability First**: Persistent state, atomic operations, and fault tolerance
2. **Self-Hosted**: No cloud dependencies, fully local operation behind VPN
3. **Master-Centric**: Only master writes to filesystem, bots communicate results
4. **Fault Tolerant**: All services (master, bots, jobs) have timeouts and recovery
5. **State Recovery**: Complete system state persisted and recoverable on restart
6. **Language**: Pure Go for performance and single binary distribution

## Architecture

### Components

#### 1. Master Node
- **Purpose**: Single point of coordination and persistence
- **Responsibilities**:
  - Bot registration with timeout management
  - Atomic job assignment and state tracking
  - Exclusive filesystem write operations
  - Persistent state management (SQLite/BadgerDB)
  - Timeout handling for all operations
  - Result aggregation from bot communications
  - Recovery from crashes with full state restoration

#### 2. Bot Agent
- **Purpose**: Execute fuzzing tasks and report results
- **Responsibilities**:
  - Register with single master (no multi-master support)
  - Heartbeat with configurable timeout
  - Execute fuzzers without writing to shared storage
  - Communicate all results via API to master
  - Handle master unavailability gracefully
  - Recover from job failures and restarts

#### 3. Persistent Storage System
- **Master-exclusive structured storage**:
  ```
  /storage/
    /data/
      pandafuzz.db      # SQLite/BadgerDB for metadata
    /corpus/
      /{job_id}/        # Job-specific corpus isolation
    /crashes/
      /{job_id}/        # Job-specific crash isolation
    /metadata/
      /bots/            # Bot state persistence
      /jobs/            # Job state persistence
      /system/          # System state persistence
    /logs/
      /{component}/     # Component-specific logs
  ```

## Feature Set

### Core Features (Phase 1 - Minimalist)
1. **Reliable Bot Management**
   - Registration with timeout enforcement
   - Atomic job assignment
   - Heartbeat monitoring with failure detection
   - Graceful bot disconnection handling

2. **Fault-Tolerant Job Orchestration**
   - Job creation with persistent state
   - Timeout-based job lifecycle management
   - Job recovery and reassignment
   - Isolated job workspaces

3. **Master-Centric Result Collection**
   - Bots communicate results via API only
   - Master performs all filesystem writes
   - Atomic result persistence
   - Crash deduplication by hash

4. **State Persistence & Recovery**
   - SQLite/BadgerDB for metadata storage
   - Complete system state recovery on restart
   - Persistent corpus metadata
   - Bot and job state snapshots

### Advanced Features (Phase 2 - Optional)
1. **Enhanced Fuzzing Modes**
   - AFL++ and LibFuzzer (core)
   - Blackbox fuzzing (optional)
   - Basic custom mutators (optional)

2. **Simple Strategies**
   - Random mutation
   - Dictionary-based
   - Basic corpus management

3. **Basic Analysis**
   - Memory leak detection with sanitizers
   - Simple crash categorization
   - Coverage tracking

### Security Model
- **VPN-Only Operation**: All services behind VPN, no external auth required
- **Input Validation**: Sanitize all bot inputs and fuzzer arguments
- **Process Isolation**: Isolate fuzzer execution per job
- **Resource Limits**: Enforce memory, disk, and time limits

### Explicitly Excluded Features
- ❌ Multi-master support
- ❌ Cloud integrations
- ❌ Complex authentication (VPN provides security)
- ❌ Advanced analytics and ML features
- ❌ Real-time collaboration features
- ❌ External integrations (issue trackers, notifications)

## API Design

### Master API Endpoints (v1 - Core)
```
# Bot Lifecycle Management
POST   /api/v1/bots/register           # Bot registration with timeout
DELETE /api/v1/bots/{id}               # Bot deregistration
POST   /api/v1/bots/{id}/heartbeat     # Bot heartbeat with status
GET    /api/v1/bots/{id}/job           # Atomic job assignment
POST   /api/v1/bots/{id}/job/complete  # Job completion notification

# Result Communication (Bot -> Master)
POST   /api/v1/results/crash           # Report crash with metadata
POST   /api/v1/results/coverage        # Report coverage data
POST   /api/v1/results/corpus          # Report corpus updates
POST   /api/v1/results/status          # Report job status updates

# Job Management (Admin)
POST   /api/v1/jobs                    # Create fuzzing job
GET    /api/v1/jobs                    # List jobs (paginated)
GET    /api/v1/jobs/{id}               # Get job details
PUT    /api/v1/jobs/{id}/cancel        # Cancel job
GET    /api/v1/jobs/{id}/logs          # Get job logs

# System Status
GET    /api/v1/status                  # System health check
GET    /api/v1/metrics                 # Basic metrics
GET    /api/v1/bots                    # List active bots
```

### Error Handling & Timeouts
```
# All endpoints include:
- Request timeout: 30s
- Bot operation timeout: 5m
- Job execution timeout: configurable (default: 1h)
- Master restart recovery: automatic
- Atomic operations for state changes
```

## Data Structures

### Core Data Models

#### Bot (Simplified)
```go
type Bot struct {
    ID           string         `json:"id" db:"id"`
    Hostname     string         `json:"hostname" db:"hostname"`
    Status       BotStatus      `json:"status" db:"status"`
    LastSeen     time.Time      `json:"last_seen" db:"last_seen"`
    RegisteredAt time.Time      `json:"registered_at" db:"registered_at"`
    CurrentJob   *string        `json:"current_job" db:"current_job"`
    Capabilities []string       `json:"capabilities" db:"capabilities"`
    TimeoutAt    time.Time      `json:"timeout_at" db:"timeout_at"`
    IsOnline     bool           `json:"is_online" db:"is_online"`
    FailureCount int            `json:"failure_count" db:"failure_count"`
}

type BotStatus string
const (
    BotStatusRegistering BotStatus = "registering"
    BotStatusIdle        BotStatus = "idle"
    BotStatusBusy        BotStatus = "busy"
    BotStatusTimedOut    BotStatus = "timed_out"
    BotStatusFailed      BotStatus = "failed"
)

// Bot configuration and resource tracking
type BotConfig struct {
    HeartbeatInterval time.Duration `json:"heartbeat_interval"`
    JobTimeout        time.Duration `json:"job_timeout"`
    MaxFailures       int           `json:"max_failures"`
    WorkDirectory     string        `json:"work_directory"`
}
```

#### Job (Simplified)
```go
type Job struct {
    ID          string    `json:"id" db:"id"`
    Name        string    `json:"name" db:"name"`
    Target      string    `json:"target" db:"target"`
    Fuzzer      string    `json:"fuzzer" db:"fuzzer"` // "afl++", "libfuzzer"
    Status      JobStatus `json:"status" db:"status"`
    AssignedBot *string   `json:"assigned_bot" db:"assigned_bot"`
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
    StartedAt   *time.Time `json:"started_at" db:"started_at"`
    CompletedAt *time.Time `json:"completed_at" db:"completed_at"`
    TimeoutAt   time.Time `json:"timeout_at" db:"timeout_at"`
    WorkDir     string    `json:"work_dir" db:"work_dir"`
    Config      JobConfig `json:"config" db:"config"`
}

type JobStatus string
const (
    JobStatusPending   JobStatus = "pending"
    JobStatusAssigned  JobStatus = "assigned"
    JobStatusRunning   JobStatus = "running"
    JobStatusCompleted JobStatus = "completed"
    JobStatusFailed    JobStatus = "failed"
    JobStatusTimedOut  JobStatus = "timed_out"
    JobStatusCancelled JobStatus = "cancelled"
)

type JobConfig struct {
    Duration      time.Duration `json:"duration"`        // Maximum runtime
    MemoryLimit   int64         `json:"memory_limit"`    // Memory limit in bytes
    Timeout       time.Duration `json:"timeout"`         // Execution timeout
    Dictionary    string        `json:"dictionary"`      // Optional dictionary file
    SeedCorpus    []string      `json:"seed_corpus"`     // Initial corpus files
    OutputDir     string        `json:"output_dir"`      // Job-specific output directory
}
```

#### Results & Findings (Simplified)
```go
type CrashResult struct {
    ID        string    `json:"id" db:"id"`
    JobID     string    `json:"job_id" db:"job_id"`
    BotID     string    `json:"bot_id" db:"bot_id"`
    Hash      string    `json:"hash" db:"hash"`         // SHA256 for deduplication
    FilePath  string    `json:"file_path" db:"file_path"` // Relative to job work dir
    Type      string    `json:"type" db:"type"`         // "segfault", "assertion", "timeout"
    Signal    int       `json:"signal" db:"signal"`     // Signal number if applicable
    ExitCode  int       `json:"exit_code" db:"exit_code"`
    Timestamp time.Time `json:"timestamp" db:"timestamp"`
    Size      int64     `json:"size" db:"size"`         // Crash input size
    IsUnique  bool      `json:"is_unique" db:"is_unique"` // Not a duplicate
}

type CoverageResult struct {
    ID        string    `json:"id" db:"id"`
    JobID     string    `json:"job_id" db:"job_id"`
    BotID     string    `json:"bot_id" db:"bot_id"`
    Edges     int       `json:"edges" db:"edges"`       // Total edges hit
    NewEdges  int       `json:"new_edges" db:"new_edges"` // New edges this run
    Timestamp time.Time `json:"timestamp" db:"timestamp"`
    ExecCount int64     `json:"exec_count" db:"exec_count"` // Total executions
}

type CorpusUpdate struct {
    ID        string    `json:"id" db:"id"`
    JobID     string    `json:"job_id" db:"job_id"`
    BotID     string    `json:"bot_id" db:"bot_id"`
    Files     []string  `json:"files" db:"files"`       // New corpus files
    Timestamp time.Time `json:"timestamp" db:"timestamp"`
    TotalSize int64     `json:"total_size" db:"total_size"`
}
```

#### Persistent Storage Structures
```go
// Master's persistent state management
type PersistentState struct {
    db       Database
    mu       sync.RWMutex
    bots     map[string]*Bot
    jobs     map[string]*Job
    metadata map[string]interface{}
}

type Database interface {
    Store(key string, value interface{}) error
    Get(key string, dest interface{}) error
    Delete(key string) error
    Transaction(fn func(tx Transaction) error) error
    Close() error
}

// Atomic job assignment with persistence
type JobAssignment struct {
    JobID     string    `json:"job_id" db:"job_id"`
    BotID     string    `json:"bot_id" db:"bot_id"`
    Timestamp time.Time `json:"timestamp" db:"timestamp"`
    Status    string    `json:"status" db:"status"` // "assigned", "started", "completed"
}

// Simple corpus metadata (persisted)
type CorpusMetadata struct {
    JobID       string            `json:"job_id" db:"job_id"`
    FileCount   int               `json:"file_count" db:"file_count"`
    TotalSize   int64             `json:"total_size" db:"total_size"`
    LastUpdated time.Time         `json:"last_updated" db:"last_updated"`
    FileHashes  map[string]string `json:"file_hashes" db:"file_hashes"` // filename -> hash
}

// System configuration persisted to disk
type SystemConfig struct {
    MasterID          string        `json:"master_id"`
    BotTimeout        time.Duration `json:"bot_timeout"`
    JobTimeout        time.Duration `json:"job_timeout"`
    HeartbeatInterval time.Duration `json:"heartbeat_interval"`
    MaxConcurrentJobs int           `json:"max_concurrent_jobs"`
    StoragePath       string        `json:"storage_path"`
}
```

## Fault Tolerance & Recovery

### Timeout Management
```go
type TimeoutManager struct {
    botTimeouts map[string]time.Time
    jobTimeouts map[string]time.Time
    mu          sync.RWMutex
}

func (tm *TimeoutManager) SetBotTimeout(botID string, timeout time.Duration) {
    tm.mu.Lock()
    defer tm.mu.Unlock()
    tm.botTimeouts[botID] = time.Now().Add(timeout)
}

func (tm *TimeoutManager) CheckTimeouts() (timedOutBots []string, timedOutJobs []string) {
    // Return bots and jobs that have timed out
}
```

### Atomic Operations
```go
type AtomicJobAssigner struct {
    state *PersistentState
    mu    sync.Mutex
}

func (aja *AtomicJobAssigner) AssignJob(botID string) (*Job, error) {
    aja.mu.Lock()
    defer aja.mu.Unlock()
    
    return aja.state.Transaction(func(tx Transaction) error {
        // 1. Find available job
        // 2. Mark job as assigned
        // 3. Update bot status
        // 4. Persist changes atomically
    })
}
```

### Exponential Backoff Retry Logic
```go
type RetryPolicy struct {
    MaxRetries    int           `json:"max_retries"`
    InitialDelay  time.Duration `json:"initial_delay"`
    MaxDelay      time.Duration `json:"max_delay"`
    Multiplier    float64       `json:"multiplier"`
    Jitter        bool          `json:"jitter"`
    RetryableErrors []string    `json:"retryable_errors"`
}

type RetryManager struct {
    policy RetryPolicy
    random *rand.Rand
}

func NewRetryManager(policy RetryPolicy) *RetryManager {
    return &RetryManager{
        policy: policy,
        random: rand.New(rand.NewSource(time.Now().UnixNano())),
    }
}

func (rm *RetryManager) Execute(operation func() error) error {
    var lastErr error
    delay := rm.policy.InitialDelay
    
    for attempt := 0; attempt <= rm.policy.MaxRetries; attempt++ {
        err := operation()
        if err == nil {
            return nil // Success
        }
        
        lastErr = err
        
        // Check if error is retryable
        if !rm.isRetryableError(err) {
            return err // Non-retryable error
        }
        
        // Don't wait after the last attempt
        if attempt == rm.policy.MaxRetries {
            break
        }
        
        // Calculate delay with optional jitter
        actualDelay := delay
        if rm.policy.Jitter {
            jitter := time.Duration(rm.random.Float64() * float64(delay) * 0.1)
            actualDelay = delay + jitter
        }
        
        time.Sleep(actualDelay)
        
        // Exponential backoff with max delay cap
        delay = time.Duration(float64(delay) * rm.policy.Multiplier)
        if delay > rm.policy.MaxDelay {
            delay = rm.policy.MaxDelay
        }
    }
    
    return fmt.Errorf("operation failed after %d attempts: %w", rm.policy.MaxRetries+1, lastErr)
}

func (rm *RetryManager) isRetryableError(err error) bool {
    if len(rm.policy.RetryableErrors) == 0 {
        // Default retryable errors
        return isNetworkError(err) || isTimeoutError(err) || isTemporaryError(err)
    }
    
    errStr := err.Error()
    for _, retryable := range rm.policy.RetryableErrors {
        if strings.Contains(errStr, retryable) {
            return true
        }
    }
    return false
}
```

### Bot Retry Logic
```go
type BotRetryClient struct {
    client       *http.Client
    retryManager *RetryManager
    masterURL    string
}

func NewBotRetryClient(masterURL string, policy RetryPolicy) *BotRetryClient {
    return &BotRetryClient{
        client: &http.Client{
            Timeout: 30 * time.Second,
        },
        retryManager: NewRetryManager(policy),
        masterURL:    masterURL,
    }
}

func (brc *BotRetryClient) RegisterWithRetry(bot *Bot) error {
    return brc.retryManager.Execute(func() error {
        return brc.doRegister(bot)
    })
}

func (brc *BotRetryClient) HeartbeatWithRetry(botID string) error {
    return brc.retryManager.Execute(func() error {
        return brc.doHeartbeat(botID)
    })
}

func (brc *BotRetryClient) GetJobWithRetry(botID string) (*Job, error) {
    var job *Job
    err := brc.retryManager.Execute(func() error {
        var err error
        job, err = brc.doGetJob(botID)
        return err
    })
    return job, err
}

func (brc *BotRetryClient) ReportResultWithRetry(result interface{}) error {
    return brc.retryManager.Execute(func() error {
        return brc.doReportResult(result)
    })
}

// Graceful degradation during master updates
func (brc *BotRetryClient) WaitForMasterRecovery() error {
    // Extended retry policy for master updates
    updatePolicy := RetryPolicy{
        MaxRetries:   20,  // Up to ~17 minutes total wait
        InitialDelay: 5 * time.Second,
        MaxDelay:     60 * time.Second,
        Multiplier:   1.5,
        Jitter:       true,
    }
    
    retryManager := NewRetryManager(updatePolicy)
    return retryManager.Execute(func() error {
        return brc.pingMaster()
    })
}
```

### Master Retry Logic for Bot Operations
```go
type MasterRetryManager struct {
    retryManager *RetryManager
    state        *PersistentState
}

func NewMasterRetryManager(policy RetryPolicy, state *PersistentState) *MasterRetryManager {
    return &MasterRetryManager{
        retryManager: NewRetryManager(policy),
        state:        state,
    }
}

func (mrm *MasterRetryManager) ProcessBotOperationWithRetry(operation func() error) error {
    return mrm.retryManager.Execute(operation)
}

func (mrm *MasterRetryManager) SaveStateWithRetry(key string, value interface{}) error {
    return mrm.retryManager.Execute(func() error {
        return mrm.state.db.Store(key, value)
    })
}

// Database operation retries
func (mrm *MasterRetryManager) AtomicOperationWithRetry(operation func() error) error {
    return mrm.retryManager.Execute(func() error {
        return mrm.state.db.Transaction(func(tx Transaction) error {
            return operation()
        })
    })
}
```

### Recovery Procedures with Retry Logic
```go
type RecoveryManager struct {
    state        *PersistentState
    timeout      *TimeoutManager
    retryManager *RetryManager
}

func NewRecoveryManager(state *PersistentState, timeout *TimeoutManager) *RecoveryManager {
    // Retry policy for recovery operations
    recoveryPolicy := RetryPolicy{
        MaxRetries:   5,
        InitialDelay: 2 * time.Second,
        MaxDelay:     30 * time.Second,
        Multiplier:   2.0,
        Jitter:       true,
        RetryableErrors: []string{
            "database is locked",
            "connection refused", 
            "timeout",
            "temporary failure",
        },
    }
    
    return &RecoveryManager{
        state:        state,
        timeout:      timeout,
        retryManager: NewRetryManager(recoveryPolicy),
    }
}

func (rm *RecoveryManager) RecoverOnStartup() error {
    return rm.retryManager.Execute(func() error {
        // 1. Load all persisted state from disk
        if err := rm.state.LoadPersistedState(); err != nil {
            return err
        }
        
        // 2. Check for orphaned jobs (assigned but bot offline)
        if err := rm.recoverOrphanedJobs(); err != nil {
            return err
        }
        
        // 3. Reset timed-out bots to idle
        if err := rm.resetTimedOutBots(); err != nil {
            return err
        }
        
        // 4. Resume pending jobs
        if err := rm.resumePendingJobs(); err != nil {
            return err
        }
        
        // 5. Cleanup stale data
        return rm.cleanupStaleData()
    })
}

func (rm *RecoveryManager) HandleBotFailureWithRetry(botID string) error {
    return rm.retryManager.Execute(func() error {
        return rm.state.db.Transaction(func(tx Transaction) error {
            // 1. Mark bot as failed
            bot, err := rm.state.getBot(tx, botID)
            if err != nil {
                return err
            }
            
            bot.Status = BotStatusFailed
            bot.FailureCount++
            
            // 2. Reassign bot's current job if any
            if bot.CurrentJob != nil {
                if err := rm.reassignJob(tx, *bot.CurrentJob); err != nil {
                    return err
                }
                bot.CurrentJob = nil
            }
            
            // 3. Update persistent state
            return tx.Store("bot:"+botID, bot)
        })
    })
}
```

### Circuit Breaker Integration
```go
type CircuitBreaker struct {
    maxFailures   int
    resetTimeout  time.Duration
    state         CircuitState
    failures      int
    lastFailTime  time.Time
    mu            sync.RWMutex
}

type CircuitState int

const (
    CircuitClosed CircuitState = iota
    CircuitOpen
    CircuitHalfOpen
)

func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
    return &CircuitBreaker{
        maxFailures:  maxFailures,
        resetTimeout: resetTimeout,
        state:        CircuitClosed,
    }
}

func (cb *CircuitBreaker) Execute(operation func() error) error {
    if !cb.canExecute() {
        return fmt.Errorf("circuit breaker is open")
    }
    
    err := operation()
    cb.recordResult(err)
    return err
}

func (cb *CircuitBreaker) canExecute() bool {
    cb.mu.RLock()
    defer cb.mu.RUnlock()
    
    switch cb.state {
    case CircuitClosed:
        return true
    case CircuitOpen:
        return time.Since(cb.lastFailTime) >= cb.resetTimeout
    case CircuitHalfOpen:
        return true
    default:
        return false
    }
}

func (cb *CircuitBreaker) recordResult(err error) {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    if err != nil {
        cb.failures++
        cb.lastFailTime = time.Now()
        
        if cb.failures >= cb.maxFailures {
            cb.state = CircuitOpen
        }
    } else {
        cb.failures = 0
        cb.state = CircuitClosed
    }
}

// Combined Retry + Circuit Breaker
type ResilientClient struct {
    retryManager   *RetryManager
    circuitBreaker *CircuitBreaker
}

func NewResilientClient(retryPolicy RetryPolicy, maxFailures int, resetTimeout time.Duration) *ResilientClient {
    return &ResilientClient{
        retryManager:   NewRetryManager(retryPolicy),
        circuitBreaker: NewCircuitBreaker(maxFailures, resetTimeout),
    }
}

func (rc *ResilientClient) Execute(operation func() error) error {
    return rc.circuitBreaker.Execute(func() error {
        return rc.retryManager.Execute(operation)
    })
}
```

## Deployment

### Single Docker Container
```dockerfile
FROM golang:1.21-alpine AS builder
# Build master and bot binaries

FROM alpine:latest
# Copy binaries and setup
EXPOSE 8080
CMD ["./pandafuzz-master"]
```

### Configuration with Retry Policies
Enhanced YAML configuration:
```yaml
master:
  port: 8080
  storage_path: /storage
  
  # Database retry configuration
  database_retry:
    max_retries: 3
    initial_delay: 1s
    max_delay: 10s
    multiplier: 2.0
    jitter: true
    retryable_errors:
      - "database is locked"
      - "timeout"
      - "temporary failure"
  
  # Bot operation retry configuration
  bot_operation_retry:
    max_retries: 5
    initial_delay: 2s
    max_delay: 30s
    multiplier: 1.5
    jitter: true
  
  # Circuit breaker configuration
  circuit_breaker:
    max_failures: 5
    reset_timeout: 60s
  
bot:
  master_url: http://master:8080
  
  # Bot communication retry configuration
  communication_retry:
    max_retries: 3
    initial_delay: 2s
    max_delay: 15s
    multiplier: 2.0
    jitter: true
    retryable_errors:
      - "connection refused"
      - "timeout"
      - "network unreachable"
      - "no route to host"
  
  # Master update recovery (extended retry for updates)
  update_recovery:
    max_retries: 20
    initial_delay: 5s
    max_delay: 60s
    multiplier: 1.5
    jitter: true
  
  capabilities:
    - afl++
    - libfuzzer
  
  timeouts:
    heartbeat_interval: 30s
    job_execution: 3600s
    master_communication: 30s
```

## Implementation Phases (Revised - Minimalist First)

### Phase 1: Core Infrastructure (Week 1-2)
- Master HTTP server with basic API
- SQLite/BadgerDB integration for persistence
- Bot registration with timeout management
- Simple job queue with atomic assignment
- Basic filesystem isolation (job-specific directories)
- Timeout management for all operations
- State recovery on master restart

### Phase 2: Bot Communication (Week 3-4)
- Bot agent with master communication
- Heartbeat mechanism with failure detection
- Result reporting API (crashes, coverage, corpus)
- Job execution isolation
- Bot failure handling and recovery
- Master-only filesystem writes

### Phase 3: Basic Fuzzing (Week 5-6)
- AFL++ integration with basic options
- LibFuzzer integration with basic options
- Crash collection and SHA256 deduplication
- Simple corpus management (file-based)
- Basic coverage tracking
- Job timeout and cleanup

### Phase 4: Persistence & Recovery (Week 7-8)
- Complete state persistence to disk
- Atomic job assignment implementation
- Corpus metadata persistence
- Bot and job recovery procedures
- Orphaned job cleanup
- System health monitoring

### Phase 5: Production Readiness (Week 9-10)
- Error handling and logging
- Resource management and limits
- Basic monitoring and metrics
- Docker packaging optimization
- Integration testing
- Documentation

### Phase 6: Optional Enhancements (Week 11-12)
- Advanced fuzzing options (if needed)
- Basic web dashboard
- Performance optimizations
- Additional fuzzer support
- Enhanced monitoring