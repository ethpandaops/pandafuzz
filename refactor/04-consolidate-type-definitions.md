# 04: Consolidate Type Definitions

## Priority: MEDIUM
## Risk Level: MEDIUM
## Estimated Effort: 4-8 hours

## Prerequisites

- Complete `03-refactor-common-package.md` first
- This task handles remaining duplicate types not addressed in step 03

## Problem Statement

After refactoring `pkg/common/`, there are still duplicate type definitions across packages:

1. **Job types**: `pkg/common/types.go` vs `pkg/domain/job/types/job.go`
2. **Bot types**: `pkg/common/types.go` vs `pkg/domain/bot/types/agent.go`
3. **Fuzzer stats**: `pkg/fuzzer/interface.go` vs `pkg/domain/fuzzer/types/interface.go`
4. **Crash types**: `pkg/common/crash.go` vs `pkg/domain/crash/` types

## Invariants (MUST NOT CHANGE)

1. Database schema compatibility - stored JSON must deserialize correctly
2. API response format must remain identical
3. All fuzzer engines must produce compatible output
4. Crash deduplication logic must continue to work
5. Job scheduling and assignment must continue to work

## Type Comparison and Consolidation Plan

### 1. Job Type Consolidation

**pkg/common/types.go - Job (CURRENT):**
```go
type Job struct {
    ID              string                 `json:"id"`
    Name            string                 `json:"name"`
    Target          string                 `json:"target"`
    Fuzzer          string                 `json:"fuzzer"`
    Status          JobStatus              `json:"status"`
    CreatedAt       time.Time              `json:"created_at"`
    StartedAt       *time.Time             `json:"started_at,omitempty"`
    CompletedAt     *time.Time             `json:"completed_at,omitempty"`
    TimeoutAt       time.Time              `json:"timeout_at"`
    AssignedBot     *string                `json:"assigned_bot,omitempty"`
    WorkDir         string                 `json:"work_dir"`
    Config          JobConfig              `json:"config"`
    Progress        int                    `json:"progress"`
    CampaignID      *string                `json:"campaign_id,omitempty"`
    Metadata        map[string]interface{} `json:"metadata,omitempty"`
    EnableCoverage  bool                   `json:"enable_coverage"`
    CoverageFormat  string                 `json:"coverage_format,omitempty"`
    LeaseToken      *string                `json:"lease_token,omitempty"`
    LeaseExpiresAt  *time.Time             `json:"lease_expires_at,omitempty"`
}
```

**pkg/domain/job/types/job.go - Job (CHECK IF EXISTS):**
- Need to verify this file's contents
- May be a simpler definition or may not exist

**Decision**: Keep `pkg/common/types.go` Job as the canonical definition until step 03 moves it properly.

### 2. Bot Type Consolidation

**pkg/common/types.go - Bot:**
```go
type Bot struct {
    ID            string          `json:"id"`
    Hostname      string          `json:"hostname"`
    IPAddress     string          `json:"ip_address"`
    Status        BotStatus       `json:"status"`
    Capabilities  []string        `json:"capabilities"`
    CurrentJob    *string         `json:"current_job,omitempty"`
    RegisteredAt  time.Time       `json:"registered_at"`
    LastSeen      time.Time       `json:"last_seen"`
    TimeoutAt     time.Time       `json:"timeout_at"`
    FailureCount  int             `json:"failure_count"`
    SuccessCount  int             `json:"success_count"`
    Metadata      map[string]any  `json:"metadata,omitempty"`
    IsOnline      bool            `json:"is_online"`
}
```

**pkg/domain/bot/types/agent.go - Agent:**
- Check if this uses different field names or structure
- May be a state machine representation

**Action**: Read and compare `pkg/domain/bot/types/agent.go`

### 3. Crash Type Consolidation

**pkg/common/crash.go - CrashResult:**
```go
type CrashResult struct {
    ID          string                 `json:"id"`
    JobID       string                 `json:"job_id"`
    BotID       string                 `json:"bot_id"`
    Hash        string                 `json:"hash"`
    Type        string                 `json:"type"`
    Signal      int                    `json:"signal"`
    ExitCode    int                    `json:"exit_code"`
    StackTrace  string                 `json:"stack_trace,omitempty"`
    Input       []byte                 `json:"input,omitempty"`
    InputBase64 string                 `json:"input_base64,omitempty"`
    Size        int64                  `json:"size"`
    FilePath    string                 `json:"file_path,omitempty"`
    Timestamp   time.Time              `json:"timestamp"`
    IsUnique    bool                   `json:"is_unique"`
    Metadata    map[string]interface{} `json:"metadata,omitempty"`
}
```

**pkg/domain/fuzzer/types/interface.go - CrashInfo:**
```go
type CrashInfo struct {
    ID           string            `json:"id"`
    Input        []byte            `json:"input"`
    StackTrace   string            `json:"stack_trace"`
    Signal       int               `json:"signal"`
    DiscoveredAt time.Time         `json:"discovered_at"`
    FuzzerType   string            `json:"fuzzer_type"`
    Metadata     map[string]string `json:"metadata,omitempty"`
}
```

**Differences:**
- CrashResult has more fields (JobID, BotID, Hash, etc.)
- CrashResult.Timestamp vs CrashInfo.DiscoveredAt
- CrashResult.Metadata is `map[string]interface{}` vs CrashInfo.Metadata is `map[string]string`

**Decision**:
- Keep CrashResult as the storage/API type (complete)
- Use CrashInfo as the fuzzer-internal type
- Create conversion function:

```go
// pkg/domain/crash/converter.go
func CrashInfoToResult(info *types.CrashInfo, jobID, botID string) *CrashResult {
    return &CrashResult{
        ID:         info.ID,
        JobID:      jobID,
        BotID:      botID,
        Input:      info.Input,
        StackTrace: info.StackTrace,
        Signal:     info.Signal,
        Timestamp:  info.DiscoveredAt,
        Hash:       calculateHash(info.Input),
        Type:       classifyCrashType(info),
        // ...
    }
}
```

### 4. FuzzerStats Consolidation

**pkg/fuzzer/interface.go - FuzzerStats:**
```go
type FuzzerStats struct {
    StartTime       time.Time     `json:"start_time"`
    ElapsedTime     time.Duration `json:"elapsed_time"`
    Executions      int64         `json:"executions"`
    ExecPerSecond   float64       `json:"exec_per_second"`
    TotalEdges      int           `json:"total_edges"`
    CoveredEdges    int           `json:"covered_edges"`
    CoveragePercent float64       `json:"coverage_percent"`
    UniqueCrashes   int           `json:"unique_crashes"`
    TotalCrashes    int           `json:"total_crashes"`
    CrashRate       float64       `json:"crash_rate"`
    CorpusSize      int           `json:"corpus_size"`
    NewPaths        int           `json:"new_paths"`
    PathsTotal      int           `json:"paths_total"`
    CPUUsage        float64       `json:"cpu_usage"`
    MemoryUsage     int64         `json:"memory_usage"`
    DiskUsage       int64         `json:"disk_usage"`
    Stability       float64       `json:"stability"`
    FuzzingRatio    float64       `json:"fuzzing_ratio"`
    LastNewPath     time.Time     `json:"last_new_path"`
    LastCrash       time.Time     `json:"last_crash"`
}
```

**pkg/domain/fuzzer/types/interface.go - FuzzerStats:**
```go
type FuzzerStats struct {
    StartTime       time.Time     `json:"start_time"`
    RunTime         time.Duration `json:"run_time"`
    TotalExecutions uint64        `json:"total_executions"`
    ExecsPerSecond  uint64        `json:"execs_per_second"`
    CorpusSize      uint64        `json:"corpus_size"`
    Coverage        float64       `json:"coverage"`
    CrashesFound    uint64        `json:"crashes_found"`
    TimeoutsFound   uint64        `json:"timeouts_found"`
    MemoryPeak      uint64        `json:"memory_peak"`
    LastCrashTime   *time.Time    `json:"last_crash_time,omitempty"`
    LastNewPathTime *time.Time    `json:"last_new_path_time,omitempty"`
}
```

**Key Differences:**
- Field names differ (ElapsedTime vs RunTime)
- Type differences (int64 vs uint64)
- Domain version is simpler/minimal

**Decision**: After step 02 (fuzzer consolidation), keep domain version as the fuzzer-internal type. Create an extended stats type for API responses:

```go
// pkg/api/v1/types/stats.go
type FuzzerStatsResponse struct {
    types.FuzzerStats
    // Additional computed fields for API
    CoveragePercent float64 `json:"coverage_percent"`
    CrashRate       float64 `json:"crash_rate"`
    // ...
}
```

## Implementation Steps

### Step 1: Audit All Type Usages

```bash
# Find all usages of each type
grep -rn "common\.Job" --include="*.go" | wc -l
grep -rn "common\.Bot" --include="*.go" | wc -l
grep -rn "common\.CrashResult" --include="*.go" | wc -l
grep -rn "types\.CrashInfo" --include="*.go" | wc -l
```

### Step 2: Create Canonical Type Package

After step 03 moves types to domain packages, ensure each domain has ONE authoritative type:

```
pkg/domain/
├── job/
│   ├── job.go           # Job, JobStatus, JobConfig - CANONICAL
│   └── types/           # DELETE or merge into job.go
├── bot/
│   ├── bot.go           # Bot, BotStatus - CANONICAL
│   └── types/           # DELETE or merge into bot.go
├── crash/
│   ├── crash.go         # CrashResult - CANONICAL
│   └── types.go         # CrashInfo for internal fuzzer use only
└── fuzzer/
    └── types/
        └── interface.go # FuzzerStats - CANONICAL for fuzzer internal
```

### Step 3: Create Conversion Functions

**pkg/domain/crash/converter.go:**
```go
package crash

import (
    "crypto/sha256"
    "encoding/hex"
    "time"

    fuzzertypes "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
)

// FromFuzzerCrashInfo converts fuzzer's CrashInfo to storage CrashResult
func FromFuzzerCrashInfo(info *fuzzertypes.CrashInfo, jobID, botID string) *CrashResult {
    hash := sha256.Sum256(info.Input)

    return &CrashResult{
        ID:         info.ID,
        JobID:      jobID,
        BotID:      botID,
        Hash:       hex.EncodeToString(hash[:]),
        Input:      info.Input,
        StackTrace: info.StackTrace,
        Signal:     info.Signal,
        Timestamp:  info.DiscoveredAt,
        Type:       classifyCrashType(info.Signal),
        IsUnique:   true, // Set by deduplication later
    }
}

func classifyCrashType(signal int) string {
    switch signal {
    case 6:
        return "abort"
    case 11:
        return "segfault"
    case 8:
        return "fpe"
    default:
        return "unknown"
    }
}
```

### Step 4: Update Consumers

Update all code that creates or uses these types to use the canonical version.

**Example - Bot crash reporting:**
```go
// Before (using CrashInfo directly)
crash := &types.CrashInfo{...}
api.ReportCrash(crash)

// After (converting to CrashResult)
crashInfo := &types.CrashInfo{...}
crashResult := crash.FromFuzzerCrashInfo(crashInfo, job.ID, bot.ID)
api.ReportCrash(crashResult)
```

### Step 5: Delete Duplicate Types

After all usages are updated:
1. Remove duplicate type definitions
2. Remove now-empty `types/` sub-packages
3. Update imports

## Verification Steps

### 1. JSON Compatibility Test
```go
func TestJobJSONCompatibility(t *testing.T) {
    // Create job with old package
    oldJob := common.Job{ID: "test", Status: common.JobStatusPending}
    oldJSON, _ := json.Marshal(oldJob)

    // Unmarshal with new package
    var newJob job.Job
    err := json.Unmarshal(oldJSON, &newJob)
    require.NoError(t, err)
    require.Equal(t, "test", newJob.ID)
}
```

### 2. Database Compatibility Test
```go
func TestExistingDataLoads(t *testing.T) {
    // Load existing database
    db := loadTestDatabase()

    // Verify all jobs load correctly
    jobs, err := db.ListJobs(ctx)
    require.NoError(t, err)
    require.NotEmpty(t, jobs)
}
```

### 3. API Response Test
```bash
# Compare API response format before and after
curl http://localhost:8080/api/v1/jobs > before.json
# ... apply changes ...
curl http://localhost:8080/api/v1/jobs > after.json
diff before.json after.json
```

## Files to Modify

### High Priority (Many Usages)
- `pkg/master/state.go` - Uses common.Job, common.Bot extensively
- `pkg/bot/agent.go` - Uses common.Job, common.Bot
- `pkg/api/v1/handlers/jobs.go` - Uses common.Job
- `pkg/api/v1/handlers/bots.go` - Uses common.Bot

### Medium Priority
- `pkg/bot/executor_fuzzer.go` - Crash reporting
- `pkg/master/api_v3/handlers.go` - API types

### Low Priority (Tests)
- `tests/integration/*.go`
- `tests/unit/*.go`

## Notes for Future Runs

### Type Alias Transition

During transition, use type aliases in the old location:

```go
// pkg/common/types.go (temporary)
package common

import "github.com/ethpandaops/pandafuzz/pkg/domain/job"

// Deprecated: Use github.com/ethpandaops/pandafuzz/pkg/domain/job.Job
type Job = job.Job

// Deprecated: Use github.com/ethpandaops/pandafuzz/pkg/domain/job.JobStatus
type JobStatus = job.JobStatus
```

### JSON Field Names are Contracts

These field names are stored in the database and returned by APIs:
- `"id"`, `"name"`, `"status"` - Job fields
- `"hostname"`, `"capabilities"` - Bot fields
- `"hash"`, `"stack_trace"` - Crash fields

**NEVER CHANGE** these without a database migration.

### Metadata Type Consistency

Both Job and Bot use `map[string]interface{}` for Metadata.
CrashInfo uses `map[string]string`.

Decision: Standardize on `map[string]any` (Go 1.18+) for all Metadata fields.

## Completion Checklist

- [ ] Audit all duplicate type definitions
- [ ] Create converter functions for fuzzer types
- [ ] Update pkg/master/state.go to use canonical types
- [ ] Update pkg/bot/ to use canonical types
- [ ] Update pkg/api/ handlers
- [ ] Add type aliases for backward compatibility
- [ ] Verify JSON serialization unchanged
- [ ] Run database loading tests
- [ ] Run API response tests
- [ ] Remove duplicate type files
- [ ] Update documentation
