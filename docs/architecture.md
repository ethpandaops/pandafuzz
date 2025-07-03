# PandaFuzz Architecture Documentation

## Overview

PandaFuzz is a self-hosted fuzzing orchestration tool written in Go designed for reliability and fault tolerance. It coordinates fuzzing bots with persistent state management, expecting all components to be breakable and recoverable.

## Core Design Principles

1. **Reliability First**: Persistent state, atomic operations, and fault tolerance
2. **Self-Hosted**: No cloud dependencies, fully local operation behind VPN
3. **Master-Centric**: Only master writes to filesystem, bots communicate results
4. **Fault Tolerant**: All services (master, bots, jobs) have timeouts and recovery
5. **State Recovery**: Complete system state persisted and recoverable on restart
6. **Language**: Pure Go for performance and single binary distribution

## Architecture Components

### 1. Master Node

- **Purpose**: Single point of coordination and persistence
- **Responsibilities**:
  - Bot registration with timeout management
  - Atomic job assignment and state tracking
  - Exclusive filesystem write operations
  - Persistent state management (SQLite/BadgerDB)
  - Timeout handling for all operations
  - Result aggregation from bot communications
  - Recovery from crashes with full state restoration

### 2. Bot Agent

- **Purpose**: Execute fuzzing tasks and report results
- **Responsibilities**:
  - Register with single master (no multi-master support)
  - Heartbeat with configurable timeout
  - Execute fuzzers without writing to shared storage
  - Communicate all results via API to master
  - Handle master unavailability gracefully
  - Recover from job failures and restarts

### 3. Persistent Storage System

**Master-exclusive structured storage**:
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

## ClusterFuzz Architecture Analysis & Validation

### Key Findings from ClusterFuzz Codebase

#### 1. Bot Management & Task Execution
**ClusterFuzz Pattern:**
- Central `testcase_manager.py` handles the core test execution lifecycle
- Task-based architecture with specialized modules in `/bot/tasks/`
- Commands are mapped to specific task modules via `commands.py`
- Each task has retry mechanisms, error handling, and status tracking
- Environment setup is dynamic per job type and platform

**PandaFuzz Validation:**
✅ **Correct**: Our bot agent design with capability detection aligns well
❌ **Missing**: Dynamic environment configuration per job type
✅ **Correct**: Modular task execution approach is sound

#### 2. Data Models & Persistence
**ClusterFuzz Pattern:**
- Core entities include Testcase, Job, TestcaseVariant, and Fuzzer
- Comprehensive crash metadata and reproduction tracking
- Job configuration inheritance and templating

**PandaFuzz Issues:**
❌ **Major Gap**: Our data models need enhancement for:
- TestcaseVariant concept for cross-platform tracking
- Comprehensive crash metadata and reproduction tracking
- Job configuration inheritance and templating

#### 3. Corpus Management
**ClusterFuzz Pattern:**
- Uses `GcsCorpus` and `FuzzTargetCorpus` classes
- SHA-based file naming for deduplication
- Sophisticated sync between local and remote storage
- Quarantine corpus handling for problematic test cases
- Engine-specific corpus management

**PandaFuzz Enhancements Needed:**
- SHA-based deduplication
- Quarantine corpus concept
- Engine-specific corpus handling

#### 4. Fuzzing Strategies
**ClusterFuzz Pattern:**
- Multi-armed bandit strategy selection
- Configurable probability weights per strategy
- Strategy combinations tracking
- Engine-specific strategy lists (LibFuzzer vs AFL)

**PandaFuzz Improvements:**
- Dynamic strategy selection
- Probability-based strategy selection
- Strategy effectiveness tracking

### Architecture Validation Results

| Component | ClusterFuzz Pattern | PandaFuzz Status | Severity |
|-----------|-------------------|------------------|----------|
| Bot Management | ✅ Task-based execution | ✅ Correct | Low |
| Data Models | ✅ Comprehensive tracking | ⚠️ Enhanced | Medium |
| Corpus Management | ✅ SHA-based dedup | ✅ Implemented | Low |
| Strategy Selection | ✅ Dynamic selection | ⚠️ Basic config | Medium |
| Environment Management | ✅ Dynamic per job | ⚠️ Static config | Medium |
| Error Handling | ✅ Retry mechanisms | ✅ Implemented | Low |

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

#### Bot
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
```

#### Job
```go
type Job struct {
    ID          string    `json:"id" db:"id"`
    Name        string    `json:"name" db:"name"`
    Target      string    `json:"target" db:"target"`
    Fuzzer      string    `json:"fuzzer" db:"fuzzer"`
    Status      JobStatus `json:"status" db:"status"`
    AssignedBot *string   `json:"assigned_bot" db:"assigned_bot"`
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
    StartedAt   *time.Time `json:"started_at" db:"started_at"`
    CompletedAt *time.Time `json:"completed_at" db:"completed_at"`
    TimeoutAt   time.Time `json:"timeout_at" db:"timeout_at"`
    WorkDir     string    `json:"work_dir" db:"work_dir"`
    Config      JobConfig `json:"config" db:"config"`
}
```

#### Results & Findings
```go
type CrashResult struct {
    ID        string    `json:"id" db:"id"`
    JobID     string    `json:"job_id" db:"job_id"`
    BotID     string    `json:"bot_id" db:"bot_id"`
    Hash      string    `json:"hash" db:"hash"`
    FilePath  string    `json:"file_path" db:"file_path"`
    Type      string    `json:"type" db:"type"`
    Signal    int       `json:"signal" db:"signal"`
    ExitCode  int       `json:"exit_code" db:"exit_code"`
    Timestamp time.Time `json:"timestamp" db:"timestamp"`
    Size      int64     `json:"size" db:"size"`
    IsUnique  bool      `json:"is_unique" db:"is_unique"`
}
```

## Fault Tolerance & Recovery

### Retry Logic Implementation

PandaFuzz implements comprehensive retry logic with exponential backoff:

```go
type RetryPolicy struct {
    MaxRetries      int           `yaml:"max_retries"`
    InitialDelay    time.Duration `yaml:"initial_delay"`
    MaxDelay        time.Duration `yaml:"max_delay"`
    Multiplier      float64       `yaml:"multiplier"`
    Jitter          bool          `yaml:"jitter"`
    RetryableErrors []string      `yaml:"retryable_errors"`
}
```

Key features:
- Exponential backoff with configurable multiplier
- Jitter to prevent thundering herd
- Retryable error classification
- Circuit breaker integration
- Extended retry for master updates (up to ~17 minutes)

### Recovery Procedures

1. **Master Recovery**: Complete state restoration from persistent storage
2. **Bot Recovery**: Automatic re-registration and job resumption
3. **Job Recovery**: Orphaned job detection and reassignment
4. **Network Recovery**: Graceful degradation during network partitions

### Timeout Management

- Bot heartbeat timeout: 30s (configurable)
- Job execution timeout: 1h (configurable per job)
- API request timeout: 30s
- Database operation timeout: 5s

## Security Model

- **VPN-Only Operation**: All services behind VPN, no external auth required
- **Input Validation**: Sanitize all bot inputs and fuzzer arguments
- **Process Isolation**: Isolate fuzzer execution per job
- **Resource Limits**: Enforce memory, disk, and time limits

## Deployment Architecture

### Docker Containerization
- Multi-stage builds for minimal images
- Security-hardened containers (non-root user)
- Resource limits enforced at container level

### Configuration Management
- YAML-based configuration with validation
- Environment variable substitution
- Secure defaults
- Retry policies configurable per component

### Monitoring & Observability
- Prometheus metrics endpoints
- Health check endpoints for all services
- Structured logging with request IDs
- Distributed tracing support

## Storage Architecture

### Database Schema
- SQLite with WAL mode for concurrent reads
- Normalized tables with proper constraints
- Foreign key relationships enforced
- Migration system for schema evolution

### File Storage
- Master-exclusive write access
- Job-isolated directories
- SHA256-based deduplication
- Atomic file operations

## Explicitly Excluded Features

- ❌ Multi-master support
- ❌ Cloud integrations
- ❌ Complex authentication (VPN provides security)
- ❌ Advanced analytics and ML features
- ❌ Real-time collaboration features
- ❌ External integrations (issue trackers, notifications)

## Conclusion

PandaFuzz provides a robust, scalable platform for distributed fuzzing with high availability through recovery mechanisms, horizontal scalability with multiple bots, comprehensive crash analysis, and production-ready deployment options. The architecture is designed for reliability and fault tolerance while maintaining simplicity in deployment and operation.