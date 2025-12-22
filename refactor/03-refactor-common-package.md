# 03: Refactor pkg/common Package

## Priority: HIGH
## Risk Level: HIGH
## Estimated Effort: 16-24 hours

## Problem Statement

`pkg/common/` is a 2,614-line catch-all package containing unrelated concerns:
- Configuration types (677 lines)
- Business domain types (417 lines)
- Database abstractions (345 lines)
- Retry logic (463 lines)
- Error definitions (122 lines)
- Interface definitions (232 lines)
- Metrics (89 lines)
- Corpus types (120 lines)
- Crash types (126 lines)

This violates Go's package design principles and makes the codebase hard to navigate.

## Invariants (MUST NOT CHANGE)

1. All existing types must remain accessible (via new import paths)
2. JSON serialization must remain identical (field names, tags)
3. Database operations must continue to work
4. API responses must remain unchanged
5. Configuration loading must work identically
6. No circular import dependencies

## Target Package Structure

```
pkg/
├── config/                    # Configuration types only
│   ├── master.go             # MasterConfig, ServerConfig, etc.
│   ├── bot.go                # BotConfig
│   ├── storage.go            # StorageConfig
│   ├── retry.go              # RetryPolicy, RetryConfig
│   └── validation.go         # Config validation helpers
│
├── domain/
│   ├── job/
│   │   └── job.go            # Job, JobStatus, JobConfig (move from common)
│   ├── bot/
│   │   └── bot.go            # Bot, BotStatus (move from common)
│   ├── crash/
│   │   └── crash.go          # CrashResult (move from common)
│   ├── corpus/
│   │   └── corpus.go         # CorpusUpdate, CorpusEntry (move from common)
│   └── coverage/
│       └── coverage.go       # CoverageResult (move from common)
│
├── database/                  # Database abstractions
│   ├── interface.go          # Database, Transaction, AdvancedDatabase
│   ├── errors.go             # Database-specific errors
│   └── stats.go              # DatabaseStats
│
├── retry/                     # Retry logic (standalone)
│   ├── manager.go            # RetryManager
│   ├── policy.go             # RetryPolicy
│   └── circuit.go            # CircuitBreaker
│
├── errors/                    # Application errors
│   ├── errors.go             # Custom error types
│   └── codes.go              # Error codes
│
├── storage/                   # Storage interface (keep existing)
│   └── interface.go          # Storage, FileStorage interfaces
│
└── metrics/                   # Metrics definitions
    └── types.go              # MetricsProvider, MetricsReporter
```

## Migration Strategy

### Phase 1: Create New Packages (No Breaking Changes)

Create new packages that re-export from common temporarily:

**pkg/config/master.go:**
```go
package config

// Temporarily re-export from common for backward compatibility
import "github.com/ethpandaops/pandafuzz/pkg/common"

type MasterConfig = common.MasterConfig
type ServerConfig = common.ServerConfig
// ... etc
```

### Phase 2: Move Types One Package at a Time

Order matters to avoid circular imports:

1. **pkg/errors/** - No dependencies on other new packages
2. **pkg/database/** - Depends only on errors
3. **pkg/retry/** - Depends on errors
4. **pkg/config/** - Depends on retry, database
5. **pkg/domain/** types - Depends on config, errors

### Phase 3: Update Imports Gradually

Use `goimports` or IDE refactoring to update imports file by file.

## Detailed File Mappings

### From pkg/common/config.go (677 lines)

**Move to pkg/config/master.go:**
```go
type MasterConfig struct { ... }
type ServerConfig struct { ... }
type DatabaseConfig struct { ... }
type StorageConfig struct { ... }
type FilesystemStorageConfig struct { ... }
type S3StorageConfig struct { ... }
type MinioStorageConfig struct { ... }
type TimeoutConfig struct { ... }
type LimitsConfig struct { ... }
type RetryConfig struct { ... }
type CircuitConfig struct { ... }
type MonitoringConfig struct { ... }
type SecurityConfig struct { ... }
type LoggingConfig struct { ... }
```

**Move to pkg/config/bot.go:**
```go
type BotConfig struct { ... }
type FuzzerConfig struct { ... }  // Bot's fuzzer config, not domain fuzzer
```

### From pkg/common/types.go (417 lines)

**Move to pkg/domain/job/job.go:**
```go
type Job struct { ... }
type JobStatus string
const (
    JobStatusPending JobStatus = "pending"
    // ... all status constants
)
type JobConfig struct { ... }
type JobAssignment struct { ... }
type JobResult struct { ... }
```

**Move to pkg/domain/bot/bot.go:**
```go
type Bot struct { ... }
type BotStatus string
const (
    BotStatusIdle BotStatus = "idle"
    // ... all status constants
)
```

### From pkg/common/crash.go (126 lines)

**Move to pkg/domain/crash/crash.go:**
```go
type CrashResult struct { ... }
type ReproductionResult struct { ... }
type ReproductionRequest struct { ... }
```

### From pkg/common/corpus.go (120 lines)

**Move to pkg/domain/corpus/corpus.go:**
```go
type CorpusUpdate struct { ... }
type CorpusEntry struct { ... }
type CorpusFile struct { ... }
```

### From pkg/common/database.go (345 lines)

**Move to pkg/database/interface.go:**
```go
type Database interface { ... }
type Transaction interface { ... }
type AdvancedDatabase interface { ... }
type DatabaseStats struct { ... }
```

### From pkg/common/errors.go (122 lines)

**Move to pkg/errors/errors.go:**
```go
type DatabaseError struct { ... }
type ValidationError struct { ... }
type NotFoundError struct { ... }
// Error constructors and helpers
func NewDatabaseError(...) error { ... }
func IsNotFoundError(err error) bool { ... }
```

### From pkg/common/retry.go (463 lines)

**Move to pkg/retry/manager.go:**
```go
type RetryManager struct { ... }
func NewRetryManager(policy RetryPolicy) *RetryManager { ... }
func (rm *RetryManager) Execute(fn func() error) error { ... }
```

**Move to pkg/retry/policy.go:**
```go
type RetryPolicy struct { ... }
var DatabaseRetryPolicy = RetryPolicy{ ... }
var BotOperationRetryPolicy = RetryPolicy{ ... }
```

### From pkg/common/interfaces.go (232 lines)

**Move to respective domain packages:**
```go
// pkg/domain/campaign/service.go
type CampaignService interface { ... }

// pkg/domain/corpus/service.go
type CorpusService interface { ... }

// pkg/domain/crash/service.go
type CrashMinimizerService interface { ... }
type ReproducibilityService interface { ... }

// pkg/storage/interface.go
type Storage interface { ... }
type FileStorage interface { ... }
```

## Import Update Script

Create a script to help with import updates:

**scripts/refactor-imports.sh:**
```bash
#!/bin/bash

# Update common imports to new packages
find . -name "*.go" -type f | xargs sed -i '' \
  -e 's|"github.com/ethpandaops/pandafuzz/pkg/common"|"github.com/ethpandaops/pandafuzz/pkg/config"|g'

# This is a starting point - manual review is required
```

## Handling Circular Import Risk

### Identify Potential Cycles

Before moving types, map dependencies:

```
common.Job uses:
  - common.JobStatus (same package, OK)
  - common.JobConfig (same package, OK)
  - time.Time (stdlib, OK)

common.Bot uses:
  - common.BotStatus (same package, OK)
  - time.Time (stdlib, OK)

common.MasterConfig uses:
  - common.RetryPolicy (move together)
  - common.TimeoutConfig (move together)
```

### Break Cycles with Interfaces

If cycles exist, define interfaces at the boundary:

```go
// pkg/domain/job/job.go
type Job struct {
    AssignedBot *string  // Use string ID, not *Bot
}

// pkg/domain/bot/bot.go
type Bot struct {
    CurrentJob *string   // Use string ID, not *Job
}
```

## Backward Compatibility Layer

Keep `pkg/common/` as a facade during transition:

**pkg/common/deprecated.go:**
```go
// Deprecated: Import from specific packages instead.
// This file will be removed in v2.0.
package common

import (
    "github.com/ethpandaops/pandafuzz/pkg/config"
    "github.com/ethpandaops/pandafuzz/pkg/domain/job"
    // ...
)

// Type aliases for backward compatibility
type MasterConfig = config.MasterConfig
type Job = job.Job
type JobStatus = job.JobStatus
// ...
```

## Verification Steps

### 1. Compile Check
```bash
make build
```

### 2. Import Cycle Check
```bash
go build ./...
# Go compiler will fail on import cycles
```

### 3. Test Suite
```bash
make test
```

### 4. JSON Serialization Test
Verify JSON output is identical:
```go
func TestJobJSONBackwardCompatibility(t *testing.T) {
    job := &job.Job{ID: "test", Status: job.JobStatusPending}
    data, _ := json.Marshal(job)
    // Compare with expected JSON structure
}
```

### 5. API Response Verification
```bash
# Start server and verify API responses match expected format
curl http://localhost:8080/api/v1/jobs | jq .
```

## Files with Most Imports to Update

Run this to find files with most common imports:
```bash
grep -rn '"github.com/ethpandaops/pandafuzz/pkg/common"' --include="*.go" | \
  cut -d: -f1 | sort | uniq -c | sort -rn | head -20
```

**Expected high-impact files:**
- `pkg/master/state.go` - 50+ usages
- `pkg/bot/agent.go` - 30+ usages
- `pkg/api/v1/handlers/*.go` - Multiple files
- `cmd/master/main.go`
- `cmd/bot/main.go`

## Notes for Future Runs

### Import Path Convention
```go
// Standard library first
import (
    "context"
    "fmt"
    "time"
)

// External dependencies
import (
    "github.com/sirupsen/logrus"
)

// Internal packages - new structure
import (
    "github.com/ethpandaops/pandafuzz/pkg/config"
    "github.com/ethpandaops/pandafuzz/pkg/domain/job"
    "github.com/ethpandaops/pandafuzz/pkg/errors"
)
```

### Do NOT Move These
- `pkg/common/config_helpers.go` - Move with config.go
- Constants that are used across many packages - Consider a `pkg/constants/` package

### Testing Strategy
1. Move one type at a time
2. Update all imports for that type
3. Run tests
4. Commit
5. Repeat

## Completion Checklist

- [ ] Create pkg/errors/ with error types
- [ ] Create pkg/database/ with interfaces
- [ ] Create pkg/retry/ with retry logic
- [ ] Create pkg/config/ with configuration types
- [ ] Move Job types to pkg/domain/job/
- [ ] Move Bot types to pkg/domain/bot/
- [ ] Move Crash types to pkg/domain/crash/
- [ ] Move Corpus types to pkg/domain/corpus/
- [ ] Move Coverage types to pkg/domain/coverage/
- [ ] Move Storage interfaces to pkg/storage/
- [ ] Create backward compatibility layer in pkg/common/
- [ ] Update all imports in pkg/master/
- [ ] Update all imports in pkg/bot/
- [ ] Update all imports in pkg/api/
- [ ] Update all imports in cmd/
- [ ] Verify no import cycles
- [ ] Verify JSON serialization unchanged
- [ ] Run full test suite
- [ ] Run integration tests
