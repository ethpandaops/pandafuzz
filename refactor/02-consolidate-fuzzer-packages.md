# 02: Consolidate Fuzzer Packages

## Priority: HIGH
## Risk Level: HIGH
## Estimated Effort: 8-16 hours

## Problem Statement

There are two completely separate fuzzer implementations with incompatible interfaces:

1. **`pkg/fuzzer/`** (9 files, ~448 lines interface) - Original implementation
2. **`pkg/domain/fuzzer/`** (24 files) - Domain-driven design implementation

These have different:
- Interface method signatures
- Return types (values vs channels)
- Type definitions (FuzzerStats, CrashInfo, etc.)

## Invariants (MUST NOT CHANGE)

1. Bot must be able to execute fuzzing jobs with AFL++, LibFuzzer, and Honggfuzz
2. Crash detection and reporting must continue to work
3. Coverage collection must continue to work
4. Job status updates must flow correctly to master
5. All existing API endpoints must return the same data structures
6. Docker container must build and run successfully

## Decision: Which Implementation to Keep

**Recommendation: Keep `pkg/domain/fuzzer/` and deprecate `pkg/fuzzer/`**

Rationale:
- Domain version follows clean architecture
- Domain version has better separation of concerns
- Domain version has factory pattern for extensibility
- Domain version has actual engine implementations (not just interfaces)

## Current Interface Comparison

### pkg/fuzzer/interface.go (TO BE DEPRECATED)
```go
type Fuzzer interface {
    Name() string
    Type() FuzzerType
    Version() string
    GetCapabilities() []string
    Configure(config FuzzConfig) error
    Initialize() error
    Validate() error
    Start(ctx context.Context) error
    Stop() error
    Pause() error
    Resume() error
    GetStatus() FuzzerStatus
    GetStats() FuzzerStats          // Returns value
    GetProgress() FuzzerProgress
    IsRunning() bool
    GetResults() (*FuzzerResults, error)
    GetCrashes() ([]*common.CrashResult, error)  // Returns slice
    GetCoverage() (*common.CoverageResult, error)
    GetCorpus() ([]*CorpusEntry, error)
    ReproduceCrash(...) (*common.ReproductionResult, error)
    SetEventHandler(handler EventHandler)
    Cleanup() error
}
```

### pkg/domain/fuzzer/types/interface.go (TO KEEP)
```go
type Fuzzer interface {
    Start(ctx context.Context) error
    Stop() error
    GetStats() (*FuzzerStats, error)  // Returns pointer
    GetCrashes() <-chan *CrashInfo    // Returns channel
    GetProgress() <-chan *ProgressUpdate
    IsRunning() bool
    GetType() string
    GetVersion() string
    SetCorpus(path string) error
    SetOutput(path string) error
    Configure(config *FuzzerConfig) error
}
```

## Migration Strategy

### Phase 1: Identify All Usages of pkg/fuzzer/

```bash
# Find all imports of pkg/fuzzer
grep -rn '"github.com/ethpandaops/pandafuzz/pkg/fuzzer"' --include="*.go"
```

**Expected locations:**
- `pkg/bot/` - Bot executor uses fuzzer interface
- `cmd/bot/` - Bot initialization
- Tests

### Phase 2: Create Adapter Layer (if needed)

If `pkg/bot/` uses methods not in domain interface, create an adapter:

**File: `pkg/domain/fuzzer/adapter/legacy_adapter.go`**
```go
package adapter

import (
    "context"

    domaintypes "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
    legacytypes "github.com/ethpandaops/pandafuzz/pkg/fuzzer"
)

// LegacyAdapter wraps domain fuzzer to provide legacy interface
type LegacyAdapter struct {
    fuzzer domaintypes.Fuzzer
}

func NewLegacyAdapter(f domaintypes.Fuzzer) *LegacyAdapter {
    return &LegacyAdapter{fuzzer: f}
}

// Implement missing methods from legacy interface
func (a *LegacyAdapter) Name() string {
    return a.fuzzer.GetType()
}

func (a *LegacyAdapter) Type() legacytypes.FuzzerType {
    return legacytypes.FuzzerType(a.fuzzer.GetType())
}

// ... implement other methods
```

### Phase 3: Update Bot Package

**Files to modify in `pkg/bot/`:**

1. **`pkg/bot/executor_fuzzer.go`** - Main fuzzer execution logic
   - Update imports
   - Use domain fuzzer factory
   - Handle channel-based crash/progress reporting

2. **`pkg/bot/agent.go`** - Bot agent
   - Update fuzzer initialization
   - Update type references

**Key change pattern:**
```go
// Before
import "github.com/ethpandaops/pandafuzz/pkg/fuzzer"
crashes, err := fuzzer.GetCrashes()

// After
import "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
crashChan := fuzzer.GetCrashes()
for crash := range crashChan {
    // Process crash
}
```

### Phase 4: Update Type References

**Type mapping:**
| Old (pkg/fuzzer) | New (pkg/domain/fuzzer/types) |
|------------------|-------------------------------|
| `fuzzer.FuzzerType` | `types.SupportedFuzzerType` |
| `fuzzer.FuzzerStatus` | Create new or use string |
| `fuzzer.FuzzConfig` | `types.FuzzerConfig` |
| `fuzzer.FuzzerStats` | `types.FuzzerStats` |
| `fuzzer.CorpusEntry` | Create in types or use common |
| `fuzzer.EventHandler` | `types.FuzzerHooks` |

### Phase 5: Deprecate Old Package

1. Add deprecation notice to `pkg/fuzzer/interface.go`:
```go
// Deprecated: Use github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer instead.
// This package will be removed in a future version.
package fuzzer
```

2. After all usages are migrated, delete `pkg/fuzzer/` entirely

## Files to Create

### pkg/domain/fuzzer/types/config.go (extend if needed)

Ensure FuzzerConfig has all fields from old FuzzConfig:
```go
type FuzzerConfig struct {
    JobID           string            `json:"job_id"`
    Target          string            `json:"target"`
    TargetArgs      []string          `json:"target_args"`
    WorkDirectory   string            `json:"work_directory"`
    Duration        time.Duration     `json:"duration"`
    Timeout         time.Duration     `json:"timeout"`
    MemoryLimit     int64             `json:"memory_limit"`
    SeedDirectory   string            `json:"seed_directory"`
    Dictionary      string            `json:"dictionary"`
    OutputDirectory string            `json:"output_directory"`
    CrashDirectory  string            `json:"crash_directory"`
    CorpusDirectory string            `json:"corpus_directory"`
    FuzzerOptions   map[string]any    `json:"fuzzer_options"`
    MaxCrashes      int               `json:"max_crashes"`
    StatsInterval   time.Duration     `json:"stats_interval"`
    EnableCoverage  bool              `json:"enable_coverage"`
    CoverageFormat  string            `json:"coverage_format"`
}
```

## Files to Delete (After Migration)

```
pkg/fuzzer/
├── base.go
├── base_test.go
├── event_test.go
├── metrics.go
├── interface.go
├── types.go
├── honggfuzz.go
├── libfuzzer.go
└── aflplusplus.go
```

## Verification Steps

### 1. Build Verification
```bash
make build
```

### 2. Unit Tests
```bash
make test-unit
```

### 3. Integration Test with All Fuzzers
```bash
./scripts/run-test-with-corpus.sh both
```

### 4. Docker Build
```bash
docker-compose build --no-cache
docker-compose up -d
```

### 5. Create Test Jobs for Each Fuzzer
```bash
# LibFuzzer job
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{"name":"libfuzzer-test","fuzzer":"libfuzzer","target":"/targets/test"}'

# AFL++ job
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{"name":"afl-test","fuzzer":"afl++","target":"/targets/test"}'

# Honggfuzz job
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{"name":"honggfuzz-test","fuzzer":"honggfuzz","target":"/targets/test"}'
```

### 6. Verify Crashes Are Detected
```bash
curl http://localhost:8080/api/v1/crashes
```

## Rollback Plan

1. Keep `pkg/fuzzer/` in a separate branch until migration is verified
2. If issues arise, revert to using old package
3. The adapter pattern allows gradual migration

## Notes for Future Runs

### Critical: Channel vs Return Value

The domain interface uses channels for crashes and progress:
```go
GetCrashes() <-chan *CrashInfo
GetProgress() <-chan *ProgressUpdate
```

Bot code must be updated to:
1. Start goroutines to consume these channels
2. Forward events to master via API
3. Handle channel closure on Stop()

### Critical: Factory Registration

Domain fuzzer uses factory pattern. Ensure all engines are registered:
```go
// In pkg/domain/fuzzer/factory/register.go or init
factory.Register("libfuzzer", libfuzzer.NewEngine)
factory.Register("afl++", aflplusplus.NewEngine)
factory.Register("honggfuzz", honggfuzz.NewEngine)
```

### Config Field Mapping

When creating FuzzerConfig from job config:
```go
fuzzerConfig := &types.FuzzerConfig{
    JobID:           job.ID,
    Target:          job.Target,
    WorkDirectory:   job.WorkDir,
    CorpusDirectory: job.Config.CorpusDir,
    // ... map all fields
}
```

## Completion Checklist

- [ ] Audit all usages of pkg/fuzzer
- [ ] Extend domain FuzzerConfig with missing fields
- [ ] Create adapter if needed for gradual migration
- [ ] Update pkg/bot/executor_fuzzer.go
- [ ] Update pkg/bot/agent.go
- [ ] Update any other consumers
- [ ] Verify factory registration for all engines
- [ ] Run all fuzzer integration tests
- [ ] Add deprecation notice to pkg/fuzzer
- [ ] Delete pkg/fuzzer after verification period
- [ ] Update imports in all files
