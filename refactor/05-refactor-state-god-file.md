# 05: Refactor State God File

## Priority: MEDIUM
## Risk Level: MEDIUM
## Estimated Effort: 8-12 hours

## Prerequisites

- Complete steps 01-04 first (panic fixes, fuzzer consolidation, common refactor, type consolidation)
- The imports in state.go will be cleaner after those steps

## Problem Statement

`pkg/master/state.go` is a 2,045-line "god file" that handles too many concerns:
- Bot state management
- Job state management
- Crash processing
- Coverage processing
- Corpus updates
- Database operations
- Cache management
- Statistics
- Analytics queries
- Lease management
- Database maintenance

This violates the Single Responsibility Principle and makes the code hard to:
- Test in isolation
- Understand
- Modify safely
- Reuse

## Invariants (MUST NOT CHANGE)

1. All state operations must remain atomic where currently atomic
2. Transaction boundaries must be preserved
3. Cache invalidation behavior must be preserved
4. Retry logic must continue to work
5. Lock ordering must be preserved (prevent deadlocks)
6. All existing API handlers must continue to work
7. Recovery on startup must continue to work

## Target Structure

Split `state.go` into focused modules:

```
pkg/master/
├── state/
│   ├── state.go           # PersistentState struct and core initialization
│   ├── bot_ops.go         # Bot CRUD operations
│   ├── job_ops.go         # Job CRUD operations
│   ├── crash_ops.go       # Crash processing
│   ├── coverage_ops.go    # Coverage processing
│   ├── corpus_ops.go      # Corpus updates
│   ├── cache.go           # Cache management and eviction
│   ├── analytics.go       # Analytics queries
│   ├── maintenance.go     # Database maintenance operations
│   ├── lease.go           # Job lease management
│   └── recovery.go        # State recovery operations
└── state.go               # Re-exports for backward compatibility
```

## Current Code Analysis

### Line Count by Concern (Approximate)

| Lines | Concern | Target File |
|-------|---------|-------------|
| 1-86 | Struct and initialization | state/state.go |
| 89-200 | Bot CRUD | state/bot_ops.go |
| 203-500 | Job CRUD | state/job_ops.go |
| 503-600 | Job assignment | state/job_ops.go |
| 600-715 | Job timeout/completion | state/job_ops.go |
| 718-970 | Crash processing | state/crash_ops.go |
| 970-1060 | Coverage/corpus processing | state/coverage_ops.go |
| 1060-1130 | Recovery operations | state/recovery.go |
| 1130-1250 | Metadata and stats | state/state.go |
| 1250-1440 | Crash queries | state/crash_ops.go |
| 1440-1590 | Cache operations | state/cache.go |
| 1590-1770 | Analytics queries | state/analytics.go |
| 1770-1920 | Database maintenance | state/maintenance.go |
| 1920-2045 | Lease sweep | state/lease.go |

## Implementation Strategy

### Phase 1: Create Package Structure

```bash
mkdir -p pkg/master/state
```

### Phase 2: Extract Core State (state/state.go)

**New file: pkg/master/state/state.go**
```go
package state

import (
    "context"
    "sync"
    "time"

    "github.com/ethpandaops/pandafuzz/pkg/common"
    "github.com/sirupsen/logrus"
)

// PersistentState manages all system state with persistence and recovery
type PersistentState struct {
    db           common.Database
    mu           sync.RWMutex
    bots         map[string]*common.Bot
    jobs         map[string]*common.Job
    metadata     map[string]any
    retryManager *common.RetryManager
    logger       *logrus.Logger
    config       *common.MasterConfig
    stats        StateStats

    // Cache management
    maxCacheSize    int
    cacheAccessTime map[string]time.Time

    // Campaign management
    campaignManager CampaignManager // Interface, not concrete type

    // Storage backend
    Storage common.Storage
}

// StateStats tracks statistics about the state manager
type StateStats struct {
    BotsRegistered   int64     `json:"bots_registered"`
    JobsCreated      int64     `json:"jobs_created"`
    CrashesRecorded  int64     `json:"crashes_recorded"`
    CoverageReports  int64     `json:"coverage_reports"`
    CorpusUpdates    int64     `json:"corpus_updates"`
    TransactionCount int64     `json:"transaction_count"`
    LastRecovery     time.Time `json:"last_recovery"`
    LastBackup       time.Time `json:"last_backup"`
    Uptime           time.Time `json:"uptime"`
}

// CampaignManager interface to break circular dependency
type CampaignManager interface {
    Stop()
}

// NewPersistentState creates a new persistent state manager
func NewPersistentState(db common.Database, config *common.MasterConfig, logger *logrus.Logger) *PersistentState {
    // ... initialization code from current state.go lines 55-86
}

// Core accessor methods
func (ps *PersistentState) GetDB() common.Database { return ps.db }
func (ps *PersistentState) GetLogger() *logrus.Logger { return ps.logger }
func (ps *PersistentState) GetConfig() *common.MasterConfig { return ps.config }

// Lock helpers for use by other files in this package
func (ps *PersistentState) Lock()    { ps.mu.Lock() }
func (ps *PersistentState) Unlock()  { ps.mu.Unlock() }
func (ps *PersistentState) RLock()   { ps.mu.RLock() }
func (ps *PersistentState) RUnlock() { ps.mu.RUnlock() }
```

### Phase 3: Extract Bot Operations (state/bot_ops.go)

**New file: pkg/master/state/bot_ops.go**
```go
package state

import (
    "context"
    "fmt"
    "time"

    "github.com/ethpandaops/pandafuzz/pkg/common"
    "github.com/sirupsen/logrus"
)

// SaveBotWithRetry persists a bot to database with retry logic
func (ps *PersistentState) SaveBotWithRetry(ctx context.Context, bot *common.Bot) error {
    // ... code from lines 89-117
}

// GetBot retrieves a bot by ID
func (ps *PersistentState) GetBot(ctx context.Context, botID string) (*common.Bot, error) {
    // ... code from lines 119-163
}

// DeleteBot removes a bot
func (ps *PersistentState) DeleteBot(ctx context.Context, botID string) error {
    // ... code from lines 165-189
}

// ListBots returns all registered bots
func (ps *PersistentState) ListBots(ctx context.Context) ([]*common.Bot, error) {
    // ... code from lines 191-201
}

// UpdateBotInCache updates bot information in the in-memory cache
func (ps *PersistentState) UpdateBotInCache(botID string, status common.BotStatus, currentJob *string, lastSeen, timeoutAt time.Time) {
    // ... code from lines 1465-1476
}

// UpdateBotInCacheForJob updates bot status related to job assignment
func (ps *PersistentState) UpdateBotInCacheForJob(botID string, jobID *string, status common.BotStatus) {
    // ... code from lines 1487-1496
}

// FindTimedOutBots returns IDs of bots that have timed out
func (ps *PersistentState) FindTimedOutBots(ctx context.Context) ([]string, error) {
    // ... code from lines 1133-1147
}

// ResetBot resets a bot's state after timeout
func (ps *PersistentState) ResetBot(ctx context.Context, botID string) error {
    // ... code from lines 1149-1183
}
```

### Phase 4: Extract Job Operations (state/job_ops.go)

**New file: pkg/master/state/job_ops.go**
```go
package state

// SaveJobWithRetry, GetJob, DeleteJob, ListJobs, ListJobsSorted
// AtomicJobAssignmentWithRetry, findAvailableJobTx
// normalizeCapability, normalizeFuzzer
// UpdateJobStatusToTimedOut, CompleteJobWithRetry
// UpdateJobInCache, UpdateJobStatusInCache
```

### Phase 5: Extract Crash Operations (state/crash_ops.go)

**New file: pkg/master/state/crash_ops.go**
```go
package state

// ProcessCrashResultWithRetry
// checkCrashDuplicateTx
// GetCrashes, GetCrashesSorted, GetCrash, GetJobCrashes, GetCrashInput
```

### Phase 6: Extract Other Modules

Continue extracting:
- `state/coverage_ops.go` - ProcessCoverageResultWithRetry, ProcessCorpusUpdateWithRetry, GetJobCoverageStats
- `state/cache.go` - evictOldestBotFromCache, evictOldestJobFromCache, cleanupCacheAccessTimes
- `state/analytics.go` - GetJobCoverageHistory, GetCampaignCoverageHistory, GetJobCrashesInTimeRange, etc.
- `state/maintenance.go` - OptimizeDatabase, CleanupOldRecords, VacuumDatabase, BackupDatabase
- `state/lease.go` - StartLeaseExpirySweep, sweepExpiredLeases
- `state/recovery.go` - LoadPersistedState, FindOrphanedJobs

### Phase 7: Create Backward Compatibility Layer

**Updated pkg/master/state.go:**
```go
package master

// Re-export for backward compatibility
import "github.com/ethpandaops/pandafuzz/pkg/master/state"

// PersistentState is an alias for backward compatibility
// Deprecated: Import from github.com/ethpandaops/pandafuzz/pkg/master/state
type PersistentState = state.PersistentState

// StateStats is an alias for backward compatibility
type StateStats = state.StateStats

// NewPersistentState creates a new persistent state manager
// Deprecated: Use state.NewPersistentState
func NewPersistentState(db common.Database, config *common.MasterConfig, logger *logrus.Logger) *PersistentState {
    return state.NewPersistentState(db, config, logger)
}
```

## Critical: Preserving Lock Ordering

The current code has a specific lock ordering pattern:
1. Acquire `mu.Lock()` or `mu.RLock()`
2. Modify in-memory state
3. Release lock
4. Perform database operation (often in retry manager)
5. Re-acquire lock if needed for cache update

This pattern MUST be preserved in the refactored code. Document lock requirements:

```go
// SaveBotWithRetry persists a bot to database with retry logic.
//
// Lock behavior:
//   - Acquires mu.Lock briefly for in-memory update
//   - Releases lock before database operation
//   - Re-acquires lock for stats update
//
// Thread safety: Safe for concurrent use
func (ps *PersistentState) SaveBotWithRetry(ctx context.Context, bot *common.Bot) error {
    // ...
}
```

## Testing Strategy

### 1. Create Interface for PersistentState

Define interface for easier testing:

```go
// pkg/master/state/interface.go
type StateManager interface {
    // Bot operations
    SaveBotWithRetry(ctx context.Context, bot *common.Bot) error
    GetBot(ctx context.Context, botID string) (*common.Bot, error)
    // ... all public methods
}
```

### 2. Test Each Module in Isolation

```go
// pkg/master/state/bot_ops_test.go
func TestSaveBotWithRetry(t *testing.T) {
    db := mocks.NewMockDatabase()
    ps := NewPersistentState(db, testConfig, testLogger)

    bot := &common.Bot{ID: "test-bot"}
    err := ps.SaveBotWithRetry(context.Background(), bot)
    require.NoError(t, err)
}
```

### 3. Integration Tests

Ensure all existing integration tests pass after refactoring.

## File Size Guidelines

After refactoring, each file should be:
- Under 500 lines (ideal)
- Under 800 lines (acceptable)
- Never over 1000 lines

## Verification Steps

### 1. Compile Check
```bash
make build
```

### 2. Test Suite
```bash
make test
```

### 3. Method Coverage Check
Ensure all public methods are still accessible:
```bash
go doc github.com/ethpandaops/pandafuzz/pkg/master/state | grep "func"
```

### 4. Lock Contention Test
Run under race detector:
```bash
go test -race ./pkg/master/state/...
```

### 5. Integration Test
```bash
docker-compose up -d
./scripts/run-test-with-corpus.sh both
```

## Notes for Future Runs

### Import Cycles

If import cycles occur between `state/` files:
1. Move shared types to `state/types.go`
2. Use interfaces to break dependencies
3. Consider if the code belongs in the same file

### Retry Manager Access

All files in `state/` package can access `ps.retryManager` directly since they're in the same package.

### Database Type Assertions

Current code does type assertions like:
```go
if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
```

This coupling should be addressed in a future refactor by:
1. Adding methods to the Database interface
2. Using feature detection interfaces

### Stats Updates

Multiple methods update `ps.stats`. Ensure thread safety:
```go
ps.mu.Lock()
ps.stats.TransactionCount++
ps.mu.Unlock()
```

Consider atomic operations for hot paths:
```go
atomic.AddInt64(&ps.stats.TransactionCount, 1)
```

## Completion Checklist

- [ ] Create pkg/master/state/ directory
- [ ] Extract state/state.go with core struct
- [ ] Extract state/bot_ops.go
- [ ] Extract state/job_ops.go
- [ ] Extract state/crash_ops.go
- [ ] Extract state/coverage_ops.go
- [ ] Extract state/cache.go
- [ ] Extract state/analytics.go
- [ ] Extract state/maintenance.go
- [ ] Extract state/lease.go
- [ ] Extract state/recovery.go
- [ ] Create backward compatibility layer
- [ ] Add comprehensive documentation
- [ ] All tests pass
- [ ] Race detector passes
- [ ] Each file under 500 lines
- [ ] Integration tests pass
