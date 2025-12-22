# 10: Documentation Updates

## Priority: LOW
## Risk Level: NONE
## Estimated Effort: 4-8 hours

## Prerequisites

- Complete steps 01-09 first
- All code changes must be stable before documenting

## Problem Statement

Documentation has several issues:

1. **Outdated comments**: Fields referenced in TODOs and comments don't exist
2. **Missing godoc**: Many interfaces lack proper documentation
3. **Stale docs/**: Some markdown files may reference old APIs
4. **Missing architecture doc**: No high-level design document
5. **Missing contribution guide**: No CONTRIBUTING.md

## Invariants (MUST NOT CHANGE)

1. No code changes in this step (documentation only)
2. Keep existing valid documentation
3. Documentation must match actual code behavior

## Documentation Audit

### Godoc Comments to Add/Update

#### Interfaces Without Documentation

**pkg/common/interfaces.go:**
```go
// Add before each interface
```

**pkg/domain/fuzzer/types/interface.go:**
```go
// Fuzzer defines the contract for fuzzing engine implementations.
// Implementations must be safe for concurrent use after configuration.
//
// Lifecycle:
//   1. Create via factory
//   2. Configure with FuzzerConfig
//   3. SetCorpus and SetOutput
//   4. Start (blocks until completion or Stop)
//   5. GetStats/GetCrashes while running
//   6. Stop to terminate
//
// Example:
//
//   fuzzer, err := factory.Create("libfuzzer", target, args)
//   if err != nil {
//       return err
//   }
//   fuzzer.Configure(&types.FuzzerConfig{...})
//   fuzzer.SetCorpus("/path/to/corpus")
//   fuzzer.SetOutput("/path/to/output")
//
//   go func() {
//       for crash := range fuzzer.GetCrashes() {
//           handleCrash(crash)
//       }
//   }()
//
//   return fuzzer.Start(ctx)
type Fuzzer interface {
    // ...
}
```

**pkg/database/interface.go (after refactor):**
```go
// Database defines the interface for database operations.
// Implementations must be safe for concurrent use.
//
// Transaction guarantees:
//   - ACID properties within a single Transaction call
//   - Automatic rollback on error or panic
//   - Retry logic should be implemented by callers using RetryManager
type Database interface {
    // Get retrieves a value by key.
    // Returns ErrKeyNotFound if the key does not exist.
    Get(ctx context.Context, key string, dest interface{}) error

    // ...
}
```

### Comments Referencing Non-Existent Fields

Search and fix:
```bash
grep -rn "TODO.*field.*exist\|TODO.*method.*exist" --include="*.go"
```

**Example fixes:**

```go
// Before
// job.Priority = common.JobPriorityHigh // TODO: Priority field doesn't exist

// After (remove or update)
// Note: Job priority is set via job.Config.Priority if needed
```

### Files to Update in docs/

| File | Status | Action |
|------|--------|--------|
| `docs/architecture.md` | Needs update | Update with current architecture after refactoring |
| `docs/configuration.md` | Needs update | Already addressed in step 09 |
| `docs/development.md` | Needs review | Verify build/test commands |
| `docs/deployment.md` | Needs review | Verify Docker setup |
| `docs/fuzzer-configuration.md` | Needs update | Update after fuzzer consolidation |
| `docs/coverage-testing-guide.md` | Needs review | Verify current API |

## New Documentation to Create

### 1. Architecture Overview (docs/architecture.md)

```markdown
# PandaFuzz Architecture

## System Overview

PandaFuzz is a distributed fuzzing orchestration system consisting of:

- **Master Server**: Coordinates fuzzing jobs, manages bots, stores results
- **Bot Agents**: Execute fuzzing jobs using AFL++, LibFuzzer, or Honggfuzz
- **Web UI**: React-based dashboard for monitoring and management
- **Storage Backend**: Filesystem or S3-compatible storage for corpus/crashes

## Component Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                        Web Browser                           │
│                     (React Dashboard)                        │
└─────────────────────────┬───────────────────────────────────┘
                          │ HTTP/WebSocket
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                      Master Server                           │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────────┐ │
│  │   API v1    │  │ State Manager│  │  Storage Backend    │ │
│  │  Handlers   │──│  (SQLite)    │──│  (FS/S3/MinIO)      │ │
│  └─────────────┘  └──────────────┘  └─────────────────────┘ │
└─────────────────────────┬───────────────────────────────────┘
                          │ HTTP API
          ┌───────────────┼───────────────┐
          ▼               ▼               ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│    Bot Agent    │ │    Bot Agent    │ │    Bot Agent    │
│  ┌───────────┐  │ │  ┌───────────┐  │ │  ┌───────────┐  │
│  │ LibFuzzer │  │ │  │   AFL++   │  │ │  │ Honggfuzz │  │
│  └───────────┘  │ │  └───────────┘  │ │  └───────────┘  │
└─────────────────┘ └─────────────────┘ └─────────────────┘
```

## Package Structure

```
pkg/
├── api/v1/          # REST API handlers and middleware
├── bot/             # Bot agent implementation
├── config/          # Configuration management
├── database/        # Database interfaces
├── domain/          # Business logic
│   ├── bot/         # Bot management
│   ├── campaign/    # Campaign orchestration
│   ├── corpus/      # Corpus management
│   ├── crash/       # Crash analysis
│   ├── fuzzer/      # Fuzzer engines
│   └── job/         # Job scheduling
├── errors/          # Error types
├── master/          # Master server
│   └── state/       # State management
├── retry/           # Retry logic
└── storage/         # Storage backends
```

## Data Flow

### Job Execution Flow

1. User creates job via API or UI
2. Master stores job with status "pending"
3. Bot polls for available jobs (heartbeat)
4. Master assigns job to bot (atomic operation)
5. Bot downloads corpus and starts fuzzer
6. Bot reports progress, crashes, coverage
7. Bot uploads results on completion
8. Master updates job status to "completed"

### Crash Reporting Flow

1. Fuzzer detects crash
2. Bot captures crash input and stack trace
3. Bot calculates crash hash for deduplication
4. Bot sends crash to master via API
5. Master checks for duplicates
6. Master stores unique crashes
7. Master updates crash statistics

## Key Design Decisions

### Master-Only Writes
Only the master server writes to storage, preventing conflicts when multiple bots operate on the same corpus.

### Job Leasing
Jobs are leased to bots with an expiration time. If a bot fails to acknowledge, the job is reassigned.

### SHA256 Deduplication
Corpus entries and crashes are deduplicated using SHA256 hashes.

### Atomic Job Assignment
Job assignment uses database transactions to prevent race conditions when multiple bots request jobs simultaneously.
```

### 2. Contributing Guide (CONTRIBUTING.md)

```markdown
# Contributing to PandaFuzz

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/pandafuzz.git`
3. Create a branch: `git checkout -b feature/your-feature`

## Development Setup

### Prerequisites

- Go 1.23+
- Node.js 16+
- Docker and Docker Compose (for testing)

### Building

```bash
# Build all binaries
make build

# Build web UI
make build-web

# Build Docker images
make docker
```

### Testing

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run integration tests
make test-integration

# Run E2E tests
npm test
```

### Code Style

- Go code is formatted with `gofmt`
- Run `make lint` before committing
- Follow Go best practices and idioms

## Pull Request Process

1. Ensure tests pass: `make test`
2. Ensure linting passes: `make lint`
3. Update documentation if needed
4. Create PR with descriptive title and description
5. Wait for review and address feedback

## Code Review Guidelines

- Keep PRs focused and small
- Include tests for new functionality
- Update CHANGELOG.md for user-facing changes
- Follow existing patterns in the codebase

## Reporting Issues

- Use GitHub Issues for bug reports
- Include reproduction steps
- Include relevant logs and configuration
```

### 3. API Documentation

After step 06 (API unification), generate API docs:

```bash
# Generate OpenAPI HTML docs
npx @redocly/cli build-docs pkg/api/v1/openapi/pandafuzz.yaml -o docs/api.html
```

## Godoc Updates

### Package-Level Documentation

Add to each package's doc.go:

**pkg/master/state/doc.go:**
```go
// Package state provides persistent state management for the PandaFuzz master server.
//
// The state package manages all in-memory and persistent state including:
//   - Bot registry and status tracking
//   - Job queue and assignment
//   - Crash storage and deduplication
//   - Coverage data aggregation
//
// Thread Safety
//
// All methods on PersistentState are safe for concurrent use.
// Internal synchronization uses sync.RWMutex for optimal read performance.
//
// Persistence
//
// State is persisted to SQLite database. On startup, LoadPersistedState
// recovers all data from the database. Changes are persisted immediately
// with retry logic for transient failures.
//
// Example usage:
//
//   state := state.NewPersistentState(db, config, logger)
//   if err := state.LoadPersistedState(ctx); err != nil {
//       return err
//   }
//
//   // Save a bot
//   err := state.SaveBotWithRetry(ctx, &common.Bot{ID: "bot-1"})
//
//   // Get a job
//   job, err := state.GetJob(ctx, "job-123")
package state
```

**pkg/domain/fuzzer/doc.go:**
```go
// Package fuzzer provides the fuzzing engine abstraction layer.
//
// This package defines the Fuzzer interface and provides factory functions
// for creating fuzzer instances. Supported engines:
//
//   - LibFuzzer: Coverage-guided, in-process fuzzer
//   - AFL++: Fork-based fuzzer with advanced mutation strategies
//   - Honggfuzz: Multi-threaded fuzzer with hardware feedback
//
// Creating a Fuzzer
//
// Use the factory to create fuzzer instances:
//
//   factory := fuzzer.NewFactory()
//   engine, err := factory.Create("libfuzzer", "/path/to/target", nil)
//   if err != nil {
//       return err
//   }
//
// Configuration
//
// Configure the fuzzer before starting:
//
//   err := engine.Configure(&types.FuzzerConfig{
//       Duration:    1 * time.Hour,
//       MemoryLimit: 1024 * 1024 * 1024, // 1GB
//   })
//
// Running
//
// The fuzzer runs until completion, timeout, or explicit stop:
//
//   ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
//   defer cancel()
//
//   err := engine.Start(ctx)
package fuzzer
```

## Implementation Checklist

### Godoc Updates

- [ ] Add package documentation to pkg/master/state/doc.go
- [ ] Add package documentation to pkg/domain/fuzzer/doc.go
- [ ] Add package documentation to pkg/api/v1/doc.go
- [ ] Add package documentation to pkg/config/doc.go
- [ ] Document all exported interfaces in pkg/database/
- [ ] Document all exported types in pkg/domain/

### Inline Comment Cleanup

- [ ] Remove TODO comments that reference non-existent fields
- [ ] Update comments that reference old package names
- [ ] Add comments explaining complex logic in pkg/master/state/

### Markdown Documentation

- [ ] Create/update docs/architecture.md
- [ ] Update docs/development.md with current commands
- [ ] Update docs/deployment.md with Docker instructions
- [ ] Update docs/fuzzer-configuration.md
- [ ] Create CONTRIBUTING.md
- [ ] Update README.md with current project status

### API Documentation

- [ ] Validate OpenAPI spec is complete
- [ ] Generate HTML API documentation
- [ ] Add API examples to docs/

## Verification

### 1. Godoc Renders Correctly
```bash
godoc -http=:6060
# Browse to http://localhost:6060/pkg/github.com/ethpandaops/pandafuzz/
```

### 2. No Broken Links
```bash
# Check markdown links
npx markdown-link-check docs/*.md
```

### 3. Examples Compile
```bash
# Verify example code in documentation compiles
go test -run=Example ./...
```

## Notes for Future Runs

### Documentation Standards

- Use complete sentences in godoc
- Include examples for complex interfaces
- Document thread safety guarantees
- Document error conditions

### Keeping Docs Updated

- Update docs when changing APIs
- Run doc verification in CI
- Review docs quarterly for staleness

## Completion Checklist

- [ ] Package-level doc.go for all packages
- [ ] Interface documentation complete
- [ ] Inline comments cleaned up
- [ ] docs/architecture.md updated
- [ ] docs/development.md verified
- [ ] docs/deployment.md verified
- [ ] CONTRIBUTING.md created
- [ ] README.md updated
- [ ] API documentation generated
- [ ] All examples compile
- [ ] No broken links
