# PandaFuzz Architecture Refactoring Action Plan (REVISED)

**Created**: 2026-01-15
**Revised**: 2026-01-15 (Based on GPT Codex Review)
**Status**: Ready for Implementation
**Estimated Duration**: 5-8 weeks

---

## Revision Summary

This revision addresses critical feedback from GPT Codex review:

| Original Issue | Resolution |
|----------------|------------|
| Type aliases won't work (structs differ) | Use explicit mapper functions instead |
| Proposed interfaces less complete than existing | Reuse existing domain repository interfaces |
| LegacyStorageAdapter adds complexity | Keep common.Storage temporarily, delete when ready |
| Missing database migrations | Added migration tasks |
| Phase sequencing suboptimal | Reordered for safer incremental migration |

---

## Executive Summary

### Critical Discovery: Type Alias Approach Won't Work

The original plan proposed using Go type aliases:
```go
type Job = jobtypes.Job  // This will NOT compile!
```

**Why it fails:**
- `common.Job` has 26 fields with `db` tags: `Target`, `Fuzzer`, `WorkDir`, `Config JobConfig`
- `domain.Job` has 33 fields with different names: `TargetBinary`, `FuzzerType`, `CorpusPath`, `OutputPath`
- Go type aliases require **identical underlying types**

**Solution:** Use explicit mapper/conversion functions between types.

---

## Revised Phase Order

| Phase | Duration | Focus |
|-------|----------|-------|
| 1 | 2-3 weeks | Repository Implementations + Mappers + Migrations |
| 2 | 1-2 weeks | API Adapter Simplification (services-only) |
| 3 | 1-2 weeks | Type Conversion Layer (NOT aliases) |
| 4 | 1 week | Storage Split (incremental extraction) |
| 5 | 1 week | Service Cleanup + Delete StateStore |
| 6 | 3-4 days | Config & Testing Polish |

---

## Phase 1: Repository Implementations + Mappers + Migrations

**Duration**: 2-3 weeks
**Risk**: Medium
**Dependencies**: None

### Objective
Implement repository implementations using existing interfaces, add required migrations, and create type mappers.

### Key Discovery: Existing Interfaces Are Good

The existing domain repository interfaces at:
- `pkg/domain/job/repository/interface.go` (20+ methods)
- `pkg/domain/bot/repository/interface.go` (AgentRepository)

Are **more complete** than what was proposed in the original plan. They include:
- Typed update methods (`UpdateStatus`, `IncrementRetries`)
- Locking semantics (`LockForProcessing`)
- Filtering capabilities

### Tasks

#### 1.1 Database Schema Migrations

**File**: `pkg/storage/sqlite/migrations/004_job_scheduling.sql`

```sql
-- Add missing columns for domain Job type
ALTER TABLE jobs ADD COLUMN scheduled_at DATETIME;
ALTER TABLE jobs ADD COLUMN queued_at DATETIME;
ALTER TABLE jobs ADD COLUMN dequeue_count INTEGER DEFAULT 0;
ALTER TABLE jobs ADD COLUMN retry_count INTEGER DEFAULT 0;
ALTER TABLE jobs ADD COLUMN max_retries INTEGER DEFAULT 3;
ALTER TABLE jobs ADD COLUMN retry_delay INTEGER DEFAULT 0;
ALTER TABLE jobs ADD COLUMN locked_by TEXT;
ALTER TABLE jobs ADD COLUMN locked_at DATETIME;
ALTER TABLE jobs ADD COLUMN lock_expires_at DATETIME;
ALTER TABLE jobs ADD COLUMN description TEXT;
ALTER TABLE jobs ADD COLUMN error_message TEXT;
ALTER TABLE jobs ADD COLUMN execution_time INTEGER DEFAULT 0;
ALTER TABLE jobs ADD COLUMN corpus_path TEXT;
ALTER TABLE jobs ADD COLUMN output_path TEXT;

-- Job dependencies table
CREATE TABLE IF NOT EXISTS job_dependencies (
    job_id TEXT NOT NULL,
    depends_on_job_id TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (job_id, depends_on_job_id),
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE,
    FOREIGN KEY (depends_on_job_id) REFERENCES jobs(id) ON DELETE CASCADE
);
```

#### 1.2 Create Type Mappers

**File**: `pkg/storage/sqlite/mappers/job_mapper.go`

```go
package mappers

import (
    "github.com/ethpandaops/pandafuzz/pkg/common"
    jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
)

// CommonJobToDomain converts common.Job to domain Job
func CommonJobToDomain(cj *common.Job) *jobtypes.Job {
    if cj == nil {
        return nil
    }

    dj := &jobtypes.Job{
        ID:           cj.ID,
        Name:         cj.Name,
        Status:       mapJobStatus(cj.Status),
        FuzzerType:   cj.Fuzzer,        // Field name differs
        TargetBinary: cj.Target,        // Field name differs
        CreatedAt:    cj.CreatedAt,
        StartedAt:    cj.StartedAt,
        CompletedAt:  cj.CompletedAt,
        UpdatedAt:    cj.CreatedAt,     // Use CreatedAt as fallback
        Priority:     mapPriority(cj.Priority),
        CorpusPath:   "",               // Not in common.Job
        OutputPath:   cj.WorkDir,       // Map WorkDir to OutputPath
        Metadata:     mapMetadata(cj.Metadata),
    }

    // Map JobConfig to FuzzerConfig
    dj.FuzzerConfig = map[string]any{
        "duration":     cj.Config.Duration,
        "memory_limit": cj.Config.MemoryLimit,
        "timeout":      cj.Config.Timeout,
        "dictionary":   cj.Config.Dictionary,
        "seed_corpus":  cj.Config.SeedCorpus,
    }

    if cj.Config.Duration > 0 {
        dj.MaxDuration = cj.Config.Duration
    }

    // Map lease fields
    dj.LeaseToken = cj.LeaseToken
    dj.LeaseExpiresAt = cj.LeaseExpiresAt
    dj.LastHeartbeat = cj.LastHeartbeat

    // Coverage
    dj.EnableCoverage = cj.EnableCoverage
    dj.CoverageFormat = cj.CoverageFormat

    return dj
}

// DomainJobToCommon converts domain Job to common.Job
func DomainJobToCommon(dj *jobtypes.Job) *common.Job {
    if dj == nil {
        return nil
    }

    cj := &common.Job{
        ID:             dj.ID,
        Name:           dj.Name,
        Target:         dj.TargetBinary,
        Fuzzer:         dj.FuzzerType,
        Status:         mapDomainStatusToCommon(dj.Status),
        CreatedAt:      dj.CreatedAt,
        StartedAt:      dj.StartedAt,
        CompletedAt:    dj.CompletedAt,
        WorkDir:        dj.OutputPath,
        Progress:       0,
        Priority:       mapDomainPriorityToInt(dj.Priority),
        EnableCoverage: dj.EnableCoverage,
        CoverageFormat: dj.CoverageFormat,
        LeaseToken:     dj.LeaseToken,
        LeaseExpiresAt: dj.LeaseExpiresAt,
        LastHeartbeat:  dj.LastHeartbeat,
    }

    // Map progress
    if dj.Progress != nil {
        cj.Progress = int(dj.Progress.Coverage) // Approximate
    }

    // Map FuzzerConfig back to JobConfig
    cj.Config = common.JobConfig{}
    if dj.MaxDuration > 0 {
        cj.Config.Duration = dj.MaxDuration
    }

    return cj
}

func mapJobStatus(cs common.JobStatus) jobtypes.JobStatus {
    switch cs {
    case common.JobStatusPending:
        return jobtypes.StatusPending
    case common.JobStatusRunning:
        return jobtypes.StatusRunning
    case common.JobStatusCompleted:
        return jobtypes.StatusCompleted
    case common.JobStatusFailed:
        return jobtypes.StatusFailed
    case common.JobStatusCancelled:
        return jobtypes.StatusCancelled
    default:
        return jobtypes.StatusPending
    }
}

func mapPriority(p int) jobtypes.JobPriority {
    switch {
    case p >= 90:
        return jobtypes.PriorityCritical
    case p >= 50:
        return jobtypes.PriorityHigh
    case p >= 20:
        return jobtypes.PriorityNormal
    default:
        return jobtypes.PriorityLow
    }
}

func mapMetadata(m map[string]interface{}) map[string]string {
    result := make(map[string]string)
    for k, v := range m {
        if s, ok := v.(string); ok {
            result[k] = s
        }
    }
    return result
}
```

#### 1.3 Implement Job Repository

**File**: `pkg/storage/sqlite/job_repo.go`

```go
package sqlite

import (
    "context"
    "database/sql"
    "time"

    jobrepo "github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
    jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
    "github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/mappers"
)

var _ jobrepo.JobRepository = (*JobRepository)(nil)

type JobRepository struct {
    db *sql.DB
}

func NewJobRepository(db *sql.DB) *JobRepository {
    return &JobRepository{db: db}
}

func (r *JobRepository) Create(ctx context.Context, job *jobtypes.Job) error {
    query := `
        INSERT INTO jobs (
            id, name, description, status, fuzzer, target,
            created_at, started_at, completed_at, timeout_at,
            work_dir, config, progress, campaign_id, priority,
            enable_coverage, coverage_format, lease_token,
            lease_expires_at, last_heartbeat,
            scheduled_at, queued_at, dequeue_count,
            retry_count, max_retries, retry_delay,
            locked_by, locked_at, lock_expires_at,
            error_message, execution_time
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `
    _, err := r.db.ExecContext(ctx, query,
        job.ID, job.Name, job.Description, job.Status.String(),
        job.FuzzerType, job.TargetBinary,
        job.CreatedAt, job.StartedAt, job.CompletedAt, nil,
        job.OutputPath, nil, 0, nil, int(job.Priority),
        job.EnableCoverage, job.CoverageFormat,
        job.LeaseToken, job.LeaseExpiresAt, job.LastHeartbeat,
        job.ScheduledAt, job.QueuedAt, job.DequeueCount,
        job.RetryCount, job.MaxRetries, job.RetryDelay.Nanoseconds(),
        job.LockedBy, job.LockedAt, job.LockExpiresAt,
        job.ErrorMessage, job.ExecutionTime.Nanoseconds(),
    )
    return err
}

func (r *JobRepository) Get(ctx context.Context, id string) (*jobtypes.Job, error) {
    query := `
        SELECT id, name, description, status, fuzzer, target,
               created_at, started_at, completed_at,
               work_dir, progress, campaign_id, priority,
               enable_coverage, coverage_format,
               lease_token, lease_expires_at, last_heartbeat,
               scheduled_at, queued_at, dequeue_count,
               retry_count, max_retries, retry_delay,
               locked_by, locked_at, lock_expires_at,
               error_message, execution_time
        FROM jobs WHERE id = ?
    `
    row := r.db.QueryRowContext(ctx, query, id)
    return r.scanJob(row)
}

func (r *JobRepository) UpdateStatus(ctx context.Context, id string, status jobtypes.JobStatus) error {
    query := `UPDATE jobs SET status = ?, updated_at = ? WHERE id = ?`
    _, err := r.db.ExecContext(ctx, query, status.String(), time.Now().UTC(), id)
    return err
}

func (r *JobRepository) LockForProcessing(ctx context.Context, id, workerID string, duration time.Duration) error {
    now := time.Now().UTC()
    expiresAt := now.Add(duration)

    query := `
        UPDATE jobs
        SET locked_by = ?, locked_at = ?, lock_expires_at = ?, status = ?
        WHERE id = ? AND (locked_by IS NULL OR lock_expires_at < ?)
    `
    result, err := r.db.ExecContext(ctx, query,
        workerID, now, expiresAt, jobtypes.StatusRunning.String(),
        id, now,
    )
    if err != nil {
        return err
    }

    rows, _ := result.RowsAffected()
    if rows == 0 {
        return ErrJobAlreadyLocked
    }
    return nil
}

// ... implement remaining interface methods

func (r *JobRepository) scanJob(row *sql.Row) (*jobtypes.Job, error) {
    job := &jobtypes.Job{}
    var (
        status        string
        startedAt     sql.NullTime
        completedAt   sql.NullTime
        campaignID    sql.NullString
        leaseToken    sql.NullString
        leaseExpires  sql.NullTime
        lastHeartbeat sql.NullTime
        scheduledAt   sql.NullTime
        queuedAt      sql.NullTime
        lockedBy      sql.NullString
        lockedAt      sql.NullTime
        lockExpires   sql.NullTime
        retryDelay    int64
        execTime      int64
    )

    err := row.Scan(
        &job.ID, &job.Name, &job.Description, &status,
        &job.FuzzerType, &job.TargetBinary,
        &job.CreatedAt, &startedAt, &completedAt,
        &job.OutputPath, nil, &campaignID, &job.Priority,
        &job.EnableCoverage, &job.CoverageFormat,
        &leaseToken, &leaseExpires, &lastHeartbeat,
        &scheduledAt, &queuedAt, &job.DequeueCount,
        &job.RetryCount, &job.MaxRetries, &retryDelay,
        &lockedBy, &lockedAt, &lockExpires,
        &job.ErrorMessage, &execTime,
    )
    if err == sql.ErrNoRows {
        return nil, ErrNotFound
    }
    if err != nil {
        return nil, err
    }

    // Map nullable fields
    job.Status = jobtypes.ParseJobStatus(status)
    if startedAt.Valid {
        job.StartedAt = &startedAt.Time
    }
    // ... map remaining nullable fields

    job.RetryDelay = time.Duration(retryDelay)
    job.ExecutionTime = time.Duration(execTime)

    return job, nil
}
```

#### 1.4 Create Coverage Repository (New Interface)

**File**: `pkg/domain/coverage/repository/interface.go`

```go
package repository

import (
    "context"
    "time"
)

type CoverageResult struct {
    ID        string
    JobID     string
    BotID     string
    Edges     int
    NewEdges  int
    ExecCount int64
    Timestamp time.Time
}

type CoverageRepository interface {
    Create(ctx context.Context, result *CoverageResult) error
    GetLatest(ctx context.Context, jobID string) (*CoverageResult, error)
    GetHistory(ctx context.Context, jobID string, limit int) ([]*CoverageResult, error)
}
```

#### 1.5 Add Concurrency Tests

**File**: `tests/integration/job_locking_test.go`

```go
func TestJobRepository_ConcurrentLocking(t *testing.T) {
    repo := setupJobRepo(t)
    ctx := context.Background()

    // Create test job
    job := &jobtypes.Job{ID: "lock-test", Status: jobtypes.StatusPending}
    require.NoError(t, repo.Create(ctx, job))

    // Concurrent lock attempts
    var wg sync.WaitGroup
    successCount := atomic.Int32{}
    failCount := atomic.Int32{}

    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            err := repo.LockForProcessing(ctx, "lock-test",
                fmt.Sprintf("worker-%d", workerID), time.Minute)
            if err == nil {
                successCount.Add(1)
            } else {
                failCount.Add(1)
            }
        }(i)
    }

    wg.Wait()

    // Only one worker should succeed
    assert.Equal(t, int32(1), successCount.Load())
    assert.Equal(t, int32(9), failCount.Load())
}
```

### Verification Checklist
- [ ] Migrations apply cleanly
- [ ] All domain repository methods implemented
- [ ] Mappers handle all field conversions
- [ ] Concurrency tests pass
- [ ] No circular imports

---

## Phase 2: API Adapter Simplification

**Duration**: 1-2 weeks
**Risk**: Medium
**Dependencies**: Phase 1 complete

### Objective
Simplify API adapters to depend only on services, moving all business logic to service layer.

### Tasks

#### 2.1 Expand Service Interfaces

Services need additional methods to support API operations:

**File**: `pkg/service/interfaces.go`

```go
type JobService interface {
    // Existing methods...
    CreateJob(ctx context.Context, req CreateJobRequest) (*common.Job, error)
    GetJob(ctx context.Context, jobID string) (*common.Job, error)
    ListJobs(ctx context.Context, filter JobFilter) ([]*common.Job, error)
    CancelJob(ctx context.Context, jobID string) error

    // Add these for API adapter simplification
    GetJobLogs(ctx context.Context, jobID string, limit, offset int) ([]*common.JobLog, int, error)
    StreamJobLogs(ctx context.Context, jobID string) (<-chan string, error)
    GetJobCorpus(ctx context.Context, jobID string) ([]*common.CorpusFile, error)
    GetCoverageReport(ctx context.Context, jobID string) (*CoverageReport, error)
    DownloadCoverageZip(ctx context.Context, jobID string) (io.Reader, error)
    GetRawCoverageFiles(ctx context.Context, jobID string) ([]string, error)
}

type CorpusService interface {
    // Existing methods...

    // Add for file uploads
    UploadCorpusFile(ctx context.Context, req UploadRequest) (*common.CorpusFile, error)
    UploadCorpusArchive(ctx context.Context, req ArchiveUploadRequest) ([]*common.CorpusFile, error)
}
```

#### 2.2 Simplify Adapter Dependencies

**Before** (`pkg/api/v1/adapters/job_adapter.go`):
```go
type JobAdapter struct {
    repository   jobRepo.JobRepository  // Remove
    executor     executor.Executor       // Remove
    jobService   service.JobService      // Keep
    storage      common.Storage          // Remove
    fileStorage  common.FileStorage      // Remove - move to service
    sse          *sse.Manager            // Keep
    logger       logrus.FieldLogger      // Keep
    maxReqSize   int64                   // Keep
}
```

**After**:
```go
type JobAdapter struct {
    jobService service.JobService   // Only service
    sse        *sse.Manager
    logger     logrus.FieldLogger
    maxReqSize int64
}

func NewJobAdapter(
    jobService service.JobService,
    sse *sse.Manager,
    logger logrus.FieldLogger,
    maxReqSize int64,
) *JobAdapter {
    return &JobAdapter{
        jobService: jobService,
        sse:        sse,
        logger:     logger,
        maxReqSize: maxReqSize,
    }
}
```

#### 2.3 Move Business Logic to Services

| Current Location | Business Logic | Move To |
|------------------|----------------|---------|
| `job_adapter.go:GetCoverageReport` | File reading, zip creation | `job_service.go` |
| `corpus_adapter.go:UploadFile` | Hash calculation, dedup | `corpus_service.go` |
| `crash_adapter.go:ProcessCrash` | Stack parsing, grouping | `dedup_service.go` |

#### 2.4 Update API Constructor

**File**: `pkg/api/v1/api.go`

```go
func NewAPI(config *Config, services Services, logger logrus.FieldLogger) (*API, error) {
    // Simplified adapter creation
    jobAdapter := adapters.NewJobAdapter(
        services.Job,
        sseManager,
        apiLogger,
        config.MaxRequestSize,
    )

    botAdapter := adapters.NewBotAdapter(
        services.Bot,
        sseManager,
        apiLogger,
    )

    // ... similarly for other adapters
}
```

### Verification Checklist
- [ ] Adapters have ≤4 dependencies
- [ ] No repository imports in adapters
- [ ] No storage imports in adapters
- [ ] All API tests pass
- [ ] Business logic in services, not adapters

---

## Phase 3: Type Conversion Layer

**Duration**: 1-2 weeks
**Risk**: Medium
**Dependencies**: Phase 2 complete

### Objective
Establish clean conversion between persistence types, domain types, and API types.

### Strategy: Explicit Conversion (NOT Type Aliases)

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   API Types     │ ←── │  Domain Types   │ ←── │ Persistence     │
│ (OpenAPI DTOs)  │     │ (Business Logic)│     │ (DB structs)    │
└─────────────────┘     └─────────────────┘     └─────────────────┘
         ↓                      ↓                       ↓
   pkg/api/v1/gen/      pkg/domain/*/types/     pkg/storage/sqlite/
                                                    models/
```

### Tasks

#### 3.1 Create Persistence Models

**File**: `pkg/storage/sqlite/models/job.go`

```go
package models

import (
    "database/sql"
    "time"
)

// JobRow represents the database row for jobs table
// This is the "source of truth" for what's in the database
type JobRow struct {
    ID              string
    Name            string
    Description     sql.NullString
    Status          string
    Fuzzer          string
    Target          string
    Type            sql.NullString
    AssignedBot     sql.NullString
    CreatedAt       time.Time
    StartedAt       sql.NullTime
    CompletedAt     sql.NullTime
    TimeoutAt       time.Time
    WorkDir         string
    ConfigJSON      sql.NullString  // JSON blob
    Progress        int
    CampaignID      sql.NullString
    CollectionID    sql.NullString
    UseCampaignCorpus bool
    MetadataJSON    sql.NullString  // JSON blob
    Priority        int
    EnableCoverage  bool
    CoverageFormat  sql.NullString
    LeaseToken      sql.NullString
    LeaseExpiresAt  sql.NullTime
    LastHeartbeat   sql.NullTime
    // New scheduling fields
    ScheduledAt     sql.NullTime
    QueuedAt        sql.NullTime
    DequeueCount    int
    RetryCount      int
    MaxRetries      int
    RetryDelayNanos int64
    LockedBy        sql.NullString
    LockedAt        sql.NullTime
    LockExpiresAt   sql.NullTime
    ErrorMessage    sql.NullString
    ExecutionTimeNanos int64
}
```

#### 3.2 Centralize All Mappers

**File**: `pkg/storage/sqlite/mappers/mappers.go`

```go
package mappers

// Export all conversion functions
var (
    // Job conversions
    JobRowToDomain = jobRowToDomain
    DomainJobToRow = domainJobToRow
    CommonJobToDomain = commonJobToDomain
    DomainJobToCommon = domainJobToCommon
    DomainJobToAPI = domainJobToAPI
    APIJobToDomain = apiJobToDomain

    // Bot conversions
    BotRowToDomain = botRowToDomain
    DomainBotToRow = domainBotToRow
    // ... etc
)
```

#### 3.3 API ↔ Domain Mappers

**File**: `pkg/api/v1/adapters/mappers.go`

```go
package adapters

import (
    gen "github.com/ethpandaops/pandafuzz/pkg/api/v1/gen"
    jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
)

func DomainJobToAPIJob(dj *jobtypes.Job) *gen.Job {
    return &gen.Job{
        Id:           dj.ID,
        Name:         dj.Name,
        Status:       gen.JobStatus(dj.Status.String()),
        FuzzerType:   dj.FuzzerType,
        TargetBinary: dj.TargetBinary,
        CreatedAt:    dj.CreatedAt,
        // ... map all fields
    }
}

func APICreateJobRequestToDomain(req *gen.CreateJobRequest) *jobtypes.Job {
    return &jobtypes.Job{
        Name:         req.Name,
        FuzzerType:   req.FuzzerType,
        TargetBinary: req.TargetBinary,
        // ... map all fields
    }
}
```

#### 3.4 Gradually Route Through Domain Types

Update services to return domain types:

```go
// Before
func (s *jobService) GetJob(ctx context.Context, jobID string) (*common.Job, error)

// After (with compatibility)
func (s *jobService) GetJob(ctx context.Context, jobID string) (*common.Job, error) {
    // Get domain job from repository
    domainJob, err := s.jobRepo.Get(ctx, jobID)
    if err != nil {
        return nil, err
    }

    // Convert to common.Job for backward compatibility
    return mappers.DomainJobToCommon(domainJob), nil
}
```

### Verification Checklist
- [ ] All type conversions are explicit functions
- [ ] No type aliases between different structs
- [ ] Conversion functions are tested
- [ ] Domain types have no `db` struct tags
- [ ] API types generated from OpenAPI

---

## Phase 4: Storage Split (Incremental)

**Duration**: 1 week
**Risk**: Medium
**Dependencies**: Phase 3 complete

### Objective
Extract sqlite.go (3015 LOC) into focused repository files.

### Tasks

#### 4.1 Create Package Structure

```bash
mkdir -p pkg/storage/sqlite/{models,mappers}
```

```
pkg/storage/sqlite/
├── connection.go      # DB connection, pragmas
├── migrations.go      # Schema management
├── transaction.go     # Tx wrapper
├── models/            # Persistence structs
│   ├── job.go
│   ├── bot.go
│   └── ...
├── mappers/           # Type conversions
│   ├── job_mapper.go
│   ├── bot_mapper.go
│   └── ...
├── job_repo.go        # JobRepository
├── bot_repo.go        # AgentRepository
├── campaign_repo.go
├── corpus_repo.go
├── crash_repo.go
├── coverage_repo.go
└── errors.go          # Custom errors
```

#### 4.2 Extract Incrementally

Move code from `sqlite.go` one repository at a time:

1. Week 1 Day 1-2: Extract `job_repo.go`
2. Week 1 Day 3: Extract `bot_repo.go`
3. Week 1 Day 4: Extract `campaign_repo.go`, `corpus_repo.go`
4. Week 1 Day 5: Extract `crash_repo.go`, `coverage_repo.go`
5. After all extracted: Delete original `sqlite.go`

#### 4.3 Keep common.Storage Temporarily

**DO NOT** create LegacyStorageAdapter. Instead:

1. Keep `common.Storage` interface during migration
2. Services that are migrated use new repositories
3. Services not yet migrated continue using StateStore
4. Once all services migrated, delete `common.Storage`

### Verification Checklist
- [ ] Each repo file <500 LOC
- [ ] sqlite.go deleted
- [ ] All migrations run cleanly
- [ ] No functionality regression

---

## Phase 5: Service Cleanup + Delete StateStore

**Duration**: 1 week
**Risk**: Low (if Phase 4 complete)
**Dependencies**: Phase 4 complete

### Objective
Remove StateStore interface and legacy storage patterns.

### Tasks

#### 5.1 Update All Services to Use Repositories

```go
// Before
type jobService struct {
    state StateStore  // God interface
}

// After
type jobService struct {
    jobRepo    jobrepo.JobRepository
    crashRepo  crashrepo.CrashRepository
    corpusRepo corpusrepo.CorpusRepository
}
```

#### 5.2 Delete StateStore

1. Delete `pkg/service/dependencies.go`
2. Update `pkg/master/state.go` to use repositories directly
3. Remove `StateStore` references from all constructors

#### 5.3 Delete common.Storage

1. Remove `Storage` interface from `pkg/common/interfaces.go`
2. Update any remaining usages

### Verification Checklist
- [ ] StateStore interface deleted
- [ ] common.Storage interface deleted
- [ ] All services use domain repositories
- [ ] No circular dependencies
- [ ] All tests pass

---

## Phase 6: Config & Testing Polish

**Duration**: 3-4 days
**Risk**: Low
**Dependencies**: Phase 5 complete

### Tasks

#### 6.1 Consolidate Configuration

Move scattered config to single package:

```go
// pkg/config/config.go
type Config struct {
    Master   MasterConfig
    Bot      BotConfig
    Database DatabaseConfig
    Storage  StorageConfig
    Queue    QueueConfig
    API      APIConfig
}
```

#### 6.2 Add Missing Tests

Target coverage:
- Repository implementations: 80%+
- Services: 70%+
- API handlers: 60%+

#### 6.3 Update Documentation

- [ ] Update CLAUDE.md with new architecture
- [ ] Create architecture diagram
- [ ] Document type conversion patterns

### Verification Checklist
- [ ] Single config loading path
- [ ] Test coverage targets met
- [ ] Documentation current

---

## Appendix: Status Enum Alignment

### Current Misalignment

| Source | Statuses |
|--------|----------|
| `common.JobStatus` | pending, assigned, starting, running, completed, failed, timed_out, cancelled |
| `domain.JobStatus` | pending, queued, running, completed, failed, cancelled, timeout |
| OpenAPI | queued, paused (exist), starting (missing) |

### Resolution

Add missing statuses to domain type and create bidirectional mappings.

---

## Success Criteria

| Metric | Current | Target |
|--------|---------|--------|
| Largest file (LOC) | 3015 | <500 |
| Storage interface methods | 50+ | 0 (deleted) |
| StateStore methods | 40+ | 0 (deleted) |
| Adapter dependencies | 7-8 | 3-4 |
| Service test coverage | 0% | 70%+ |
| Type conversion bugs | Unknown | 0 (explicit mappers) |

---

## Timeline (Revised)

| Phase | Duration | Risk |
|-------|----------|------|
| 1: Repos + Mappers + Migrations | 2-3 weeks | Medium |
| 2: API Adapter Simplification | 1-2 weeks | Medium |
| 3: Type Conversion Layer | 1-2 weeks | Medium |
| 4: Storage Split | 1 week | Medium |
| 5: Service Cleanup | 1 week | Low |
| 6: Config & Testing | 3-4 days | Low |

**Total: 5-8 weeks** (realistic for 1-2 developers)
