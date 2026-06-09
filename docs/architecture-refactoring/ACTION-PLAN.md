# PandaFuzz Architecture Refactoring Action Plan

**Created**: 2026-01-15
**Status**: Draft - Pending Review
**Estimated Duration**: 4-6 weeks of focused effort

---

## Executive Summary

This plan addresses critical architectural issues in PandaFuzz that impact maintainability, testability, and developer productivity. The refactoring is organized into 6 phases, ordered by dependency and risk.

### Critical Issues Being Addressed

| Issue | Severity | Current State | Target State |
|-------|----------|---------------|--------------|
| Duplicate Job types | Critical | 2 divergent definitions | Single source of truth |
| God Storage interface | Critical | 50+ methods in one interface | 8 focused repositories |
| Monolithic files | High | sqlite.go (3015 LOC), agent.go (1842 LOC) | <500 LOC per file |
| API layer violations | High | Adapters import repositories directly | API → Services only |
| Missing tests | High | 0 service/adapter tests | 70%+ coverage |
| Config scattered | Medium | 3+ packages | Single config package |

---

## Phase 1: Foundation - Domain Repository Interfaces

**Duration**: 3-4 days
**Risk**: Low
**Dependencies**: None

### Objective
Define clean, focused repository interfaces for each domain entity without breaking existing code.

### Tasks

#### 1.1 Create Job Repository Interface
**File**: `pkg/domain/job/repository/interface.go`

```go
package repository

import (
    "context"
    "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
)

type JobRepository interface {
    Create(ctx context.Context, job *types.Job) error
    Get(ctx context.Context, id string) (*types.Job, error)
    Update(ctx context.Context, id string, updates map[string]any) error
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, filters JobFilters) ([]*types.Job, error)
    GetByStatus(ctx context.Context, status types.JobStatus) ([]*types.Job, error)
    GetByCampaign(ctx context.Context, campaignID string) ([]*types.Job, error)
    AssignToBot(ctx context.Context, jobID, botID string) error
    UpdateProgress(ctx context.Context, jobID string, progress int) error
}

type JobFilters struct {
    Status     *types.JobStatus
    CampaignID *string
    BotID      *string
    Limit      int
    Offset     int
}
```

#### 1.2 Create Campaign Repository Interface
**File**: `pkg/domain/campaign/repository/interface.go`

```go
package repository

type CampaignRepository interface {
    Create(ctx context.Context, campaign *types.Campaign) error
    Get(ctx context.Context, id string) (*types.Campaign, error)
    List(ctx context.Context, filters CampaignFilters) ([]*types.Campaign, error)
    Update(ctx context.Context, id string, updates map[string]any) error
    Delete(ctx context.Context, id string) error
    GetStatistics(ctx context.Context, campaignID string) (*types.CampaignStats, error)
    LinkJob(ctx context.Context, campaignID, jobID string) error
}
```

#### 1.3 Create Corpus Repository Interface
**File**: `pkg/domain/corpus/repository/interface.go`

```go
package repository

type CorpusRepository interface {
    AddFile(ctx context.Context, file *types.CorpusFile) error
    GetFile(ctx context.Context, fileID string) (*types.CorpusFile, error)
    GetFileByHash(ctx context.Context, hash string) (*types.CorpusFile, error)
    GetFiles(ctx context.Context, campaignID string) ([]*types.CorpusFile, error)
    UpdateFile(ctx context.Context, fileID string, updates map[string]any) error
    DeleteFile(ctx context.Context, fileID string) error
    GetUnsynced(ctx context.Context, campaignID, botID string) ([]*types.CorpusFile, error)
    MarkSynced(ctx context.Context, fileIDs []string, botID string) error
    RecordEvolution(ctx context.Context, evolution *types.CorpusEvolution) error
    GetEvolution(ctx context.Context, campaignID string, limit int) ([]*types.CorpusEvolution, error)
}
```

#### 1.4 Create Crash Repository Interface
**File**: `pkg/domain/crash/repository/interface.go`

```go
package repository

type CrashRepository interface {
    Create(ctx context.Context, crash *types.CrashResult) error
    Get(ctx context.Context, id string) (*types.CrashResult, error)
    List(ctx context.Context, jobID string, limit, offset int) ([]*types.CrashResult, error)
    GetByCampaign(ctx context.Context, campaignID string) ([]*types.CrashResult, error)
    GetCount(ctx context.Context, jobID string) (int, error)
    StoreInput(ctx context.Context, crashID string, input []byte) error
    GetInput(ctx context.Context, crashID string) ([]byte, error)

    // Crash grouping
    CreateGroup(ctx context.Context, group *types.CrashGroup) error
    GetGroup(ctx context.Context, campaignID, stackHash string) (*types.CrashGroup, error)
    ListGroups(ctx context.Context, campaignID string) ([]*types.CrashGroup, error)
    LinkToGroup(ctx context.Context, crashID, groupID string) error
}
```

#### 1.5 Create Bot Repository Interface
**File**: `pkg/domain/bot/repository/interface.go` (enhance existing)

```go
package repository

type AgentRepository interface {
    // Existing methods...
    Create(ctx context.Context, agent *types.Agent) error
    Get(ctx context.Context, id string) (*types.Agent, error)
    Update(ctx context.Context, id string, updates map[string]any) error
    Delete(ctx context.Context, id string) error
    List(ctx context.Context) ([]*types.Agent, error)

    // Add these
    GetByStatus(ctx context.Context, status types.Status) ([]*types.Agent, error)
    UpdateHeartbeat(ctx context.Context, id string) error
    AssignJob(ctx context.Context, botID, jobID string) error
    ClearJob(ctx context.Context, botID string) error
}
```

#### 1.6 Create Coverage Repository Interface
**File**: `pkg/domain/coverage/repository/interface.go` (new package)

```go
package repository

type CoverageRepository interface {
    Create(ctx context.Context, coverage *types.CoverageResult) error
    GetLatest(ctx context.Context, jobID string) (*types.CoverageResult, error)
    GetHistory(ctx context.Context, jobID string, limit int) ([]*types.CoverageResult, error)
}
```

### Verification
- [ ] All interfaces compile
- [ ] No circular imports
- [ ] Each interface has <15 methods
- [ ] Methods follow Go naming conventions

---

## Phase 2: Type Consolidation

**Duration**: 2-3 days
**Risk**: Medium
**Dependencies**: Phase 1 complete

### Objective
Establish single source of truth for domain types, eliminating duplication.

### Tasks

#### 2.1 Audit Current Type Locations

| Type | common/types.go | domain/*/types/ | Action |
|------|-----------------|-----------------|--------|
| Job | Lines 61-87 | job/types/job.go:10-61 | Keep domain, alias in common |
| JobStatus | Lines 97-108 | job/types/status.go | Keep domain, alias in common |
| Bot | Lines 27-40 | bot/types/agent.go | Keep domain as Agent, alias Bot in common |
| BotStatus | Lines 42-50 | bot/types/status.go | Keep domain, alias in common |
| Campaign | Lines 316-332 | campaign/types/ | Verify single location |
| CrashResult | Exists | crash/types/ | Verify single location |
| CorpusFile | Exists | corpus/types/ | Verify single location |

#### 2.2 Create Type Aliases in common/types.go

```go
package common

import (
    jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
    bottypes "github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
)

// Job types - source of truth is pkg/domain/job/types
type Job = jobtypes.Job
type JobStatus = jobtypes.JobStatus
type JobConfig = jobtypes.JobConfig
type JobProgress = jobtypes.JobProgress

// Bot types - source of truth is pkg/domain/bot/types
type Bot = bottypes.Agent
type BotStatus = bottypes.Status

// Keep these constants for backward compatibility
const (
    JobStatusPending   = jobtypes.StatusPending
    JobStatusRunning   = jobtypes.StatusRunning
    JobStatusCompleted = jobtypes.StatusCompleted
    JobStatusFailed    = jobtypes.StatusFailed
    // etc.
)
```

#### 2.3 Migrate Domain Job Type

Update `pkg/domain/job/types/job.go` to be the canonical Job type:

1. Add missing fields from `common.Job` that domain Job lacks:
   - `Type JobType` (fuzzing, minimization, reproduction)
   - `WorkDir string`
   - `CampaignID *string`
   - `CollectionID *string`
   - `UseCampaignCorpus bool`

2. Rename fields to match common usage:
   - `FuzzerType` → `Fuzzer` (or add alias)
   - `TargetBinary` → `Target` (or add alias)

3. Add `db` struct tags for SQLite compatibility

#### 2.4 Update Imports Across Codebase

Run automated refactoring:
```bash
# Find all files importing common.Job
grep -r "common\.Job" pkg/ --include="*.go" -l

# Update imports (manual review required)
```

### Verification
- [ ] Single Job type definition
- [ ] All tests pass
- [ ] No duplicate struct definitions
- [ ] Backward compatibility maintained via aliases

---

## Phase 3: Storage Layer Refactoring

**Duration**: 5-7 days
**Risk**: High
**Dependencies**: Phase 1 and 2 complete

### Objective
Split the monolithic `sqlite.go` (3015 LOC) into focused repository implementations.

### Tasks

#### 3.1 Create Storage Package Structure

```
pkg/storage/sqlite/
├── connection.go       # DB connection, pool, pragmas (~150 LOC)
├── migrations.go       # Schema migrations (~200 LOC)
├── transaction.go      # Transaction wrapper (~100 LOC)
├── job_repo.go        # JobRepository impl (~400 LOC)
├── bot_repo.go        # AgentRepository impl (~300 LOC)
├── campaign_repo.go   # CampaignRepository impl (~350 LOC)
├── corpus_repo.go     # CorpusRepository impl (~400 LOC)
├── crash_repo.go      # CrashRepository impl (~350 LOC)
├── coverage_repo.go   # CoverageRepository impl (~150 LOC)
├── helpers.go         # Shared SQL helpers (~200 LOC)
└── sqlite_test.go     # Integration tests
```

#### 3.2 Extract Connection Management

**File**: `pkg/storage/sqlite/connection.go`

```go
package sqlite

import (
    "context"
    "database/sql"
    "sync"

    _ "github.com/mattn/go-sqlite3"
)

type Connection struct {
    db     *sql.DB
    mu     sync.RWMutex
    closed bool
}

func NewConnection(dsn string) (*Connection, error) {
    db, err := sql.Open("sqlite3", dsn)
    if err != nil {
        return nil, err
    }

    // Set pragmas for performance
    pragmas := []string{
        "PRAGMA journal_mode=WAL",
        "PRAGMA synchronous=NORMAL",
        "PRAGMA foreign_keys=ON",
        "PRAGMA busy_timeout=5000",
    }
    for _, pragma := range pragmas {
        if _, err := db.Exec(pragma); err != nil {
            return nil, err
        }
    }

    return &Connection{db: db}, nil
}

func (c *Connection) DB() *sql.DB { return c.db }
func (c *Connection) Close() error { ... }
func (c *Connection) Ping(ctx context.Context) error { ... }
```

#### 3.3 Implement Job Repository

**File**: `pkg/storage/sqlite/job_repo.go`

```go
package sqlite

import (
    "context"
    jobrepo "github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
    jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
)

// Verify interface compliance
var _ jobrepo.JobRepository = (*JobRepository)(nil)

type JobRepository struct {
    conn *Connection
}

func NewJobRepository(conn *Connection) *JobRepository {
    return &JobRepository{conn: conn}
}

func (r *JobRepository) Create(ctx context.Context, job *jobtypes.Job) error {
    query := `INSERT INTO jobs (id, name, status, fuzzer_type, target_binary, ...)
              VALUES (?, ?, ?, ?, ?, ...)`
    _, err := r.conn.db.ExecContext(ctx, query,
        job.ID, job.Name, job.Status, job.FuzzerType, job.TargetBinary, ...)
    return err
}

func (r *JobRepository) Get(ctx context.Context, id string) (*jobtypes.Job, error) {
    query := `SELECT id, name, status, ... FROM jobs WHERE id = ?`
    row := r.conn.db.QueryRowContext(ctx, query, id)

    job := &jobtypes.Job{}
    err := row.Scan(&job.ID, &job.Name, &job.Status, ...)
    if err == sql.ErrNoRows {
        return nil, ErrNotFound
    }
    return job, err
}

// Implement remaining methods...
```

#### 3.4 Create Repository Factory

**File**: `pkg/storage/sqlite/factory.go`

```go
package sqlite

type Repositories struct {
    Jobs      *JobRepository
    Bots      *BotRepository
    Campaigns *CampaignRepository
    Corpus    *CorpusRepository
    Crashes   *CrashRepository
    Coverage  *CoverageRepository
}

func NewRepositories(conn *Connection) *Repositories {
    return &Repositories{
        Jobs:      NewJobRepository(conn),
        Bots:      NewBotRepository(conn),
        Campaigns: NewCampaignRepository(conn),
        Corpus:    NewCorpusRepository(conn),
        Crashes:   NewCrashRepository(conn),
        Coverage:  NewCoverageRepository(conn),
    }
}
```

#### 3.5 Deprecate common.Storage Interface

**File**: `pkg/common/interfaces.go`

```go
// Deprecated: Use domain-specific repositories instead.
// This interface will be removed in v2.0.
type Storage interface {
    // ... existing methods with deprecation comments
}
```

#### 3.6 Create Adapter for Backward Compatibility

**File**: `pkg/storage/sqlite/legacy_adapter.go`

```go
package sqlite

import "github.com/ethpandaops/pandafuzz/pkg/common"

// LegacyStorageAdapter wraps new repositories to implement common.Storage
// This allows gradual migration without breaking existing code
type LegacyStorageAdapter struct {
    repos *Repositories
}

func NewLegacyStorageAdapter(repos *Repositories) common.Storage {
    return &LegacyStorageAdapter{repos: repos}
}

func (a *LegacyStorageAdapter) CreateJob(ctx context.Context, job *common.Job) error {
    // Convert common.Job to domain job and delegate
    domainJob := convertToDomainJob(job)
    return a.repos.Jobs.Create(ctx, domainJob)
}
// ... implement all common.Storage methods by delegating to repos
```

### Verification
- [ ] All repository implementations pass interface compliance
- [ ] sqlite.go can be deleted after migration
- [ ] Legacy adapter provides backward compatibility
- [ ] All existing tests pass
- [ ] New repository tests added

---

## Phase 4: Service Layer Cleanup

**Duration**: 3-4 days
**Risk**: Medium
**Dependencies**: Phase 3 complete

### Objective
Update services to use new repositories and add comprehensive tests.

### Tasks

#### 4.1 Update Service Constructors

**Before** (`pkg/service/job_service.go`):
```go
type jobService struct {
    state StateStore  // God interface with 40+ methods
}

func NewJobService(state StateStore, ...) JobService {
    return &jobService{state: state, ...}
}
```

**After**:
```go
type jobService struct {
    jobRepo      jobrepo.JobRepository
    crashRepo    crashrepo.CrashRepository
    corpusRepo   corpusrepo.CorpusRepository
    // Only the repos this service actually needs
}

func NewJobService(
    jobRepo jobrepo.JobRepository,
    crashRepo crashrepo.CrashRepository,
    corpusRepo corpusrepo.CorpusRepository,
    ...,
) JobService {
    return &jobService{
        jobRepo:    jobRepo,
        crashRepo:  crashRepo,
        corpusRepo: corpusRepo,
        ...
    }
}
```

#### 4.2 Update Service Manager

**File**: `pkg/service/manager.go`

```go
type Manager struct {
    // Repositories
    repos *sqlite.Repositories

    // Services
    Bot       BotService
    Job       JobService
    Campaign  CampaignService
    // ...
}

func NewManager(conn *sqlite.Connection, config *Config) (*Manager, error) {
    repos := sqlite.NewRepositories(conn)

    m := &Manager{repos: repos}

    // Initialize services with specific repos they need
    m.Job = NewJobService(repos.Jobs, repos.Crashes, repos.Corpus, ...)
    m.Bot = NewBotService(repos.Bots, repos.Jobs, ...)
    m.Campaign = NewCampaignService(repos.Campaigns, repos.Jobs, ...)

    return m, nil
}
```

#### 4.3 Add Service Layer Tests

**File**: `pkg/service/job_service_test.go`

```go
package service_test

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

// Mock repositories
type mockJobRepo struct {
    mock.Mock
}

func (m *mockJobRepo) Create(ctx context.Context, job *types.Job) error {
    args := m.Called(ctx, job)
    return args.Error(0)
}
// ... implement other methods

func TestJobService_CreateJob(t *testing.T) {
    // Arrange
    mockRepo := new(mockJobRepo)
    mockRepo.On("Create", mock.Anything, mock.Anything).Return(nil)

    service := NewJobService(mockRepo, nil, nil, ...)

    // Act
    job, err := service.CreateJob(ctx, CreateJobRequest{...})

    // Assert
    assert.NoError(t, err)
    assert.NotEmpty(t, job.ID)
    mockRepo.AssertExpectations(t)
}
```

#### 4.4 Remove StateStore Interface

Once all services are migrated:

1. Delete `pkg/service/dependencies.go` (StateStore definition)
2. Update any remaining references
3. Remove from service interfaces

### Verification
- [ ] All services use domain repositories
- [ ] StateStore interface removed
- [ ] Service tests achieve 70%+ coverage
- [ ] No circular dependencies

---

## Phase 5: API Layer Simplification

**Duration**: 4-5 days
**Risk**: Medium-High
**Dependencies**: Phase 4 complete

### Objective
Remove adapter layer duplication and ensure API only depends on services.

### Tasks

#### 5.1 Audit Current API Dependencies

**Current** (`pkg/api/v1/adapters/job_adapter.go`):
```go
type JobAdapter struct {
    repository   jobRepo.JobRepository  // ❌ Should not import repo
    executor     executor.Executor       // ❌ Should not import executor
    jobService   service.JobService      // ✓ Correct
    storage      common.Storage          // ❌ Should not import storage
    fileStorage  common.FileStorage      // ❌ Should go through service
    sse          *sse.Manager
    logger       logrus.FieldLogger
    maxReqSize   int64
}
```

**Target**:
```go
type JobAdapter struct {
    jobService service.JobService  // Only service dependency
    sse        *sse.Manager
    logger     logrus.FieldLogger
    maxReqSize int64
}
```

#### 5.2 Move Business Logic to Services

Identify logic in adapters that should be in services:

| Adapter Method | Current Location | Target Location |
|----------------|------------------|-----------------|
| Job validation | job_adapter.go | job_service.go |
| File upload processing | corpus_adapter.go | corpus_service.go |
| Crash deduplication | crash_adapter.go | dedup_service.go |
| Coverage report generation | job_adapter.go | coverage_service.go |

#### 5.3 Simplify Adapter Constructors

**File**: `pkg/api/v1/adapters/job_adapter.go`

```go
// Before: 8 parameters
func NewJobAdapter(
    repository jobRepo.JobRepository,
    executor executor.Executor,
    jobService service.JobService,
    storage common.Storage,
    fileStorage common.FileStorage,
    sseManager *sse.Manager,
    logger logrus.FieldLogger,
    maxRequestSize int64,
) *JobAdapter

// After: 4 parameters
func NewJobAdapter(
    jobService service.JobService,
    sseManager *sse.Manager,
    logger logrus.FieldLogger,
    maxRequestSize int64,
) *JobAdapter
```

#### 5.4 Consider Merging Adapters into Handlers

Evaluate whether adapter layer adds value:

**Option A: Keep adapters (thin)**
- Adapters handle HTTP concerns (request parsing, response formatting)
- Handlers route to adapters
- Services contain business logic

**Option B: Remove adapters**
- Handlers directly call services
- Reduces ~6000 LOC
- Simpler mental model

**Recommendation**: Option A with thin adapters, but significantly simplified.

#### 5.5 Split Large Adapter Files

```
pkg/api/v1/adapters/
├── job/
│   ├── adapter.go      # Core adapter struct
│   ├── create.go       # CreateJob handler
│   ├── list.go         # ListJobs handler
│   ├── get.go          # GetJob handler
│   ├── update.go       # UpdateJob handler
│   ├── logs.go         # GetJobLogs handler
│   └── coverage.go     # Coverage-related handlers
├── bot/
│   └── ...
├── campaign/
│   └── ...
└── composite.go        # CompositeAdapter
```

### Verification
- [ ] Adapters only depend on services
- [ ] No repository imports in API layer
- [ ] No direct storage access in API layer
- [ ] Adapter files <500 LOC each

---

## Phase 6: Configuration & Testing Polish

**Duration**: 2-3 days
**Risk**: Low
**Dependencies**: Phase 5 complete

### Objective
Consolidate configuration and ensure comprehensive test coverage.

### Tasks

#### 6.1 Consolidate Configuration

**Create unified config structure**:

**File**: `pkg/config/config.go`

```go
package config

type Config struct {
    Master   MasterConfig   `yaml:"master"`
    Bot      BotConfig      `yaml:"bot"`
    Database DatabaseConfig `yaml:"database"`
    Storage  StorageConfig  `yaml:"storage"`
    Queue    QueueConfig    `yaml:"queue"`
    API      APIConfig      `yaml:"api"`
}

func Load(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    cfg := DefaultConfig()
    if err := yaml.Unmarshal(data, cfg); err != nil {
        return nil, err
    }

    return cfg, cfg.Validate()
}

func DefaultConfig() *Config {
    return &Config{
        Master:   DefaultMasterConfig(),
        Bot:      DefaultBotConfig(),
        Database: DefaultDatabaseConfig(),
        Storage:  DefaultStorageConfig(),
        Queue:    DefaultQueueConfig(),
        API:      DefaultAPIConfig(),
    }
}
```

#### 6.2 Remove Duplicate Config Definitions

| Location | Action |
|----------|--------|
| `pkg/common/types.go` (SystemConfig, BotOperationalConfig) | Move to pkg/config/ |
| `pkg/config/defaults.go` | Merge into config.go |
| Service-specific configs | Keep in respective packages, import from config |

#### 6.3 Add Integration Tests

**File**: `tests/integration/repository_test.go`

```go
func TestJobRepository_CRUD(t *testing.T) {
    // Setup test database
    conn := setupTestDB(t)
    defer conn.Close()

    repo := sqlite.NewJobRepository(conn)

    t.Run("Create and Get", func(t *testing.T) {
        job := &types.Job{ID: "test-1", Name: "Test Job", ...}
        err := repo.Create(ctx, job)
        require.NoError(t, err)

        retrieved, err := repo.Get(ctx, "test-1")
        require.NoError(t, err)
        assert.Equal(t, job.Name, retrieved.Name)
    })

    t.Run("List with filters", func(t *testing.T) { ... })
    t.Run("Update", func(t *testing.T) { ... })
    t.Run("Delete", func(t *testing.T) { ... })
}
```

#### 6.4 Add API Handler Tests

**File**: `pkg/api/v1/handlers/jobs_test.go`

```go
func TestJobHandlers(t *testing.T) {
    // Setup mock services
    mockService := new(mockJobService)
    handler := NewJobHandler(mockService, ...)

    t.Run("POST /jobs creates job", func(t *testing.T) {
        req := httptest.NewRequest("POST", "/api/v1/jobs", ...)
        rec := httptest.NewRecorder()

        handler.CreateJob(rec, req)

        assert.Equal(t, http.StatusCreated, rec.Code)
    })
}
```

#### 6.5 Update Documentation

- [ ] Update CLAUDE.md with new architecture
- [ ] Create architecture diagram
- [ ] Document migration guide for external consumers

### Verification
- [ ] Single config loading path
- [ ] No duplicate config structs
- [ ] Integration test coverage for all repositories
- [ ] API handler test coverage >70%
- [ ] Documentation updated

---

## Risk Mitigation

### High-Risk Changes

| Change | Risk | Mitigation |
|--------|------|------------|
| Type consolidation | Breaking API contracts | Use type aliases for backward compat |
| Storage split | Data corruption | Extensive integration tests, backup before migration |
| Service rewiring | Runtime errors | Feature flags, gradual rollout |

### Rollback Strategy

Each phase should be:
1. Developed on a feature branch
2. Reviewed thoroughly
3. Deployed to staging first
4. Monitored for 24-48 hours before production

### Feature Flags

```go
// pkg/config/features.go
type FeatureFlags struct {
    UseNewRepositories bool `yaml:"use_new_repositories"`
    UseNewServices     bool `yaml:"use_new_services"`
    UseLegacyStorage   bool `yaml:"use_legacy_storage"`
}
```

---

## Success Metrics

| Metric | Current | Target |
|--------|---------|--------|
| Largest file (LOC) | 3015 (sqlite.go) | <500 |
| Storage interface methods | 50+ | 0 (deprecated) |
| Duplicate type definitions | 5+ | 0 |
| Service test coverage | 0% | 70%+ |
| API adapter dependencies | 7+ | 2-3 |
| Time to understand Job flow | Hours | Minutes |

---

## Timeline Summary

| Phase | Duration | Dependencies | Risk |
|-------|----------|--------------|------|
| Phase 1: Repository Interfaces | 3-4 days | None | Low |
| Phase 2: Type Consolidation | 2-3 days | Phase 1 | Medium |
| Phase 3: Storage Refactoring | 5-7 days | Phase 1, 2 | High |
| Phase 4: Service Cleanup | 3-4 days | Phase 3 | Medium |
| Phase 5: API Simplification | 4-5 days | Phase 4 | Medium-High |
| Phase 6: Config & Testing | 2-3 days | Phase 5 | Low |

**Total Estimated Duration**: 4-6 weeks

---

## Appendix A: File Changes Summary

### New Files
- `pkg/storage/sqlite/connection.go`
- `pkg/storage/sqlite/migrations.go`
- `pkg/storage/sqlite/transaction.go`
- `pkg/storage/sqlite/job_repo.go`
- `pkg/storage/sqlite/bot_repo.go`
- `pkg/storage/sqlite/campaign_repo.go`
- `pkg/storage/sqlite/corpus_repo.go`
- `pkg/storage/sqlite/crash_repo.go`
- `pkg/storage/sqlite/coverage_repo.go`
- `pkg/storage/sqlite/factory.go`
- `pkg/storage/sqlite/legacy_adapter.go`
- `pkg/domain/coverage/repository/interface.go`
- `pkg/config/config.go`
- `pkg/service/*_test.go` (multiple test files)
- `pkg/api/v1/handlers/*_test.go` (multiple test files)

### Modified Files
- `pkg/common/types.go` (add type aliases)
- `pkg/common/interfaces.go` (deprecate Storage)
- `pkg/domain/job/types/job.go` (add missing fields)
- `pkg/service/job_service.go` (use repos)
- `pkg/service/bot_service.go` (use repos)
- `pkg/service/manager.go` (new initialization)
- `pkg/api/v1/adapters/*.go` (simplify dependencies)

### Deleted Files (after migration)
- `pkg/storage/sqlite.go` (3015 LOC → split into multiple files)
- `pkg/service/dependencies.go` (StateStore interface)

---

## Appendix B: Commands Reference

```bash
# Run all tests
make test

# Run specific test file
go test ./pkg/storage/sqlite/... -v

# Check for circular imports
go build ./...

# Generate mocks (if using mockery)
mockery --all --keeptree --output=./mocks

# Verify interface compliance
go build -v ./pkg/storage/sqlite/...

# Check test coverage
go test ./pkg/service/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```
