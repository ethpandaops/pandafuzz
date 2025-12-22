# 07: Fix Broken Tests

## Priority: MEDIUM
## Risk Level: LOW
## Estimated Effort: 8-16 hours

## Prerequisites

- Complete steps 01-06 first
- Type definitions must be stable before fixing tests

## Problem Statement

Over 50 TODO comments in test files indicate broken or outdated tests:
- Tests reference non-existent fields
- Tests reference non-existent methods
- Tests use outdated API patterns
- Tests are commented out entirely

This means the test suite provides false confidence - tests that don't run can't catch regressions.

## Invariants (MUST NOT CHANGE)

1. Working tests must continue to pass
2. Test coverage should increase, not decrease
3. Integration test infrastructure must remain functional
4. E2E test setup must continue to work

## Test File Inventory

### Unit Tests with Issues

**tests/unit/bot_job_test.go:**
```
Line 23:  // TODO: This test needs to be rewritten to use the actual API endpoints
Line 187: // TODO: This test also needs to be rewritten - uses JobResult and JobProgress types
Line 381: // TODO: This test uses UpdateJobStatus which doesn't exist on RetryClient
Line 515: // TODO: This benchmark also needs to be rewritten - uses JobResult
```

**tests/unit/edge_case_test.go:**
- Contains panic tests - verify these still work after step 01

### Integration Tests with Issues

**tests/integration/bot_workflow_test.go:**
```
Line 19: // TODO: This test needs to be updated to use the current API
```

**tests/integration/job_flow_test.go:**
```
Line 131: // TODO: Check if Message field exists
Line 182: // TODO: This method doesn't exist (CheckTimeouts)
Line 188: // TODO: Check if Message field exists
Line 202: // TODO: Priority field doesn't exist
Line 211: // TODO: Priority field doesn't exist
Line 327: // TODO: apiHandlers not available on TestEnvironment
```

**tests/integration/crash_reporting_test.go:**
```
Line 16: // TODO: All tests in this file need to be updated
```

**tests/integration/api_test.go:**
```
Line 488: // TODO: This test needs to be updated when authentication is implemented
```

**tests/integration/helpers_test.go:**
```
Line 252: // TODO: Find correct priority constant
Line 255-259: // TODO: Multiple fields don't exist (TargetArgs, Corpus, Dictionary, TimeoutSec, MemoryLimit)
Line 281: // TODO: Check if IP field exists
Line 311-313: // TODO: CoveredEdges, CoveragePercent fields
```

**tests/integration/fuzzer_test.go:**
```
Line 505: // TODO: corpusDir is an unexported field - this test needs to be rewritten
```

**tests/integration/master_bot_test.go:**
```
Line 159: // TODO: BotIdle doesn't exist on TimeoutConfig
Line 357: // TODO: EnableMetrics doesn't exist
Line 379: // TODO: MetricsPort doesn't exist on ServerConfig
```

**tests/integration/recovery_test.go:**
```
Line 121: // TODO: WorkDirectory doesn't exist on BotConfig
Line 311: // TODO: This method doesn't exist (CheckTimeouts)
Line 317: // TODO: Message field doesn't exist
```

## Resolution Strategy

### Category 1: Missing Fields/Methods

For tests referencing non-existent fields:
1. Check if field was renamed
2. Check if field moved to nested struct
3. If field was removed, update test to use current API
4. If field should exist, add it to the type

**Example Resolution:**
```go
// Before (broken)
job.Priority = common.JobPriorityHigh

// Option A: Field was moved to Config
job.Config.Priority = common.JobPriorityHigh

// Option B: Field was removed, remove from test
// Delete the line and adjust test expectations
```

### Category 2: Missing Methods

For tests calling non-existent methods:
1. Check if method was renamed
2. Check if functionality moved to different component
3. Update test to use current API

**Example Resolution:**
```go
// Before (broken)
env.timeoutMgr.CheckTimeouts()

// After (find current implementation)
// Option A: Method renamed
env.state.FindTimedOutBots(ctx)
env.state.FindOrphanedJobs(ctx)

// Option B: Functionality in different place
env.maintenance.RunTimeoutCheck(ctx)
```

### Category 3: Entire Tests Need Rewriting

For tests marked "needs to be rewritten":
1. Understand what the test was trying to verify
2. Write new test using current APIs
3. Ensure same scenarios are covered

### Category 4: Commented Out Tests

For tests that are commented out:
1. Try to uncomment and run
2. If fails, apply category 1-3 fixes
3. If unclear purpose, delete and document

## Implementation Plan

### Phase 1: Triage and Categorize

Create tracking list:

```markdown
## Test Fix Tracker

### P1 - Critical (blocking CI)
- [ ] tests/unit/bot_job_test.go - Rewrite API tests

### P2 - Important (reduces coverage)
- [ ] tests/integration/job_flow_test.go - Fix field references
- [ ] tests/integration/master_bot_test.go - Fix config references
- [ ] tests/integration/recovery_test.go - Fix method calls

### P3 - Nice to have
- [ ] tests/integration/api_test.go - Auth test placeholder
- [ ] tests/integration/crash_reporting_test.go - Full rewrite needed
```

### Phase 2: Fix Unit Tests

**tests/unit/bot_job_test.go:**

```go
// Current broken test (line ~23)
func TestBotJobExecution(t *testing.T) {
    // TODO: This test needs to be rewritten to use the actual API endpoints
}

// Fixed version
func TestBotJobExecution(t *testing.T) {
    ctx := context.Background()

    // Setup test server
    srv := testutil.NewTestServer(t)
    defer srv.Close()

    // Create job via API
    job := &api.CreateJobRequest{
        Name:   "test-job",
        Target: "/path/to/target",
        Fuzzer: "libfuzzer",
    }

    resp, err := srv.Client().CreateJob(ctx, job)
    require.NoError(t, err)
    require.NotEmpty(t, resp.ID)

    // Verify job created
    getResp, err := srv.Client().GetJob(ctx, resp.ID)
    require.NoError(t, err)
    require.Equal(t, "pending", getResp.Status)
}
```

### Phase 3: Fix Integration Test Helpers

**tests/integration/helpers_test.go:**

Update helper functions to use current types:

```go
// Before (broken)
func createTestJob(t *testing.T) *common.Job {
    return &common.Job{
        ID:         uuid.New().String(),
        Name:       "test-job",
        Priority:   common.JobPriorityNormal,    // TODO: Find correct constant
        TargetArgs: []string{"@@"},              // TODO: Check if exists
        // ...
    }
}

// After (fixed)
func createTestJob(t *testing.T) *common.Job {
    return &common.Job{
        ID:        uuid.New().String(),
        Name:      "test-job",
        Target:    "/test/target",
        Fuzzer:    "libfuzzer",
        Status:    common.JobStatusPending,
        CreatedAt: time.Now(),
        TimeoutAt: time.Now().Add(1 * time.Hour),
        Config: common.JobConfig{
            TargetArgs: []string{"@@"},
            Timeout:    300,
            MemLimit:   1024,
        },
    }
}
```

### Phase 4: Fix or Remove Broken Integration Tests

For each integration test file:

1. **Run the test file in isolation:**
   ```bash
   go test -v ./tests/integration/job_flow_test.go
   ```

2. **Document failures:**
   - Compile errors (type/method not found)
   - Runtime errors (nil pointer, assertion failures)

3. **Fix or skip:**
   ```go
   func TestJobPriority(t *testing.T) {
       t.Skip("Skipped: Priority field removed in v2.0, see #123")
       // Original test code
   }
   ```

### Phase 5: Create Missing Test Infrastructure

If tests reference missing test utilities:

**tests/testutil/server.go:**
```go
package testutil

import (
    "testing"

    "github.com/ethpandaops/pandafuzz/pkg/master"
)

// TestServer provides a test HTTP server
type TestServer struct {
    *httptest.Server
    state  *master.PersistentState
    client *api.Client
}

func NewTestServer(t *testing.T) *TestServer {
    t.Helper()

    // Setup in-memory database
    db := NewTestDatabase(t)

    // Setup state
    config := DefaultTestConfig()
    logger := NewTestLogger(t)
    state := master.NewPersistentState(db, config, logger)

    // Setup server
    handler := master.NewAPIHandler(state)
    srv := httptest.NewServer(handler)

    return &TestServer{
        Server: srv,
        state:  state,
        client: api.NewClient(srv.URL),
    }
}

func (s *TestServer) Client() *api.Client {
    return s.client
}
```

### Phase 6: Update Test Documentation

**tests/README.md:**
```markdown
# PandaFuzz Tests

## Running Tests

### Unit Tests
```bash
make test-unit
```

### Integration Tests
```bash
make test-integration
```

### E2E Tests
```bash
npm test
```

## Test Structure

- `tests/unit/` - Unit tests for individual functions
- `tests/integration/` - Integration tests requiring database
- `tests/e2e/` - End-to-end Playwright tests
- `tests/testutil/` - Shared test utilities

## Known Limitations

- Some integration tests require running master server
- E2E tests require Docker environment
```

## Verification Steps

### 1. Run All Tests
```bash
make test
```

### 2. Check Coverage
```bash
make test-coverage
```

### 3. Verify No Skipped Tests Unexpectedly
```bash
go test -v ./... 2>&1 | grep -i skip
```

### 4. Run With Race Detector
```bash
go test -race ./...
```

## Notes for Future Runs

### Test Naming Convention

```go
// Unit tests: Test<Function>_<Scenario>
func TestSaveBot_Success(t *testing.T)
func TestSaveBot_DatabaseError(t *testing.T)

// Integration tests: TestIntegration_<Feature>_<Scenario>
func TestIntegration_JobFlow_Complete(t *testing.T)

// Table-driven tests for multiple scenarios
func TestJobStatus(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected JobStatus
    }{
        {"pending", "pending", JobStatusPending},
        {"running", "running", JobStatusRunning},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ...
        })
    }
}
```

### Test Fixtures

Store test fixtures in `test-resources/`:
```
test-resources/
├── test-corpus/      # Sample corpus files
├── test-targets/     # Test fuzzing targets
├── fixtures/         # JSON fixtures for tests
│   ├── job.json
│   ├── bot.json
│   └── crash.json
└── golden/           # Golden output files
```

### Mocking

Use interfaces for easy mocking:
```go
// In production code
type DatabaseProvider interface {
    GetJob(ctx context.Context, id string) (*Job, error)
}

// In test code
type MockDatabase struct {
    mock.Mock
}

func (m *MockDatabase) GetJob(ctx context.Context, id string) (*Job, error) {
    args := m.Called(ctx, id)
    return args.Get(0).(*Job), args.Error(1)
}
```

## Completion Checklist

- [ ] Triage all TODO comments in test files
- [ ] Fix tests/unit/bot_job_test.go
- [ ] Fix tests/integration/job_flow_test.go
- [ ] Fix tests/integration/helpers_test.go
- [ ] Fix tests/integration/master_bot_test.go
- [ ] Fix tests/integration/recovery_test.go
- [ ] Fix tests/integration/crash_reporting_test.go
- [ ] Fix tests/integration/fuzzer_test.go
- [ ] Fix tests/integration/api_test.go
- [ ] Create missing test utilities
- [ ] Update test documentation
- [ ] All tests pass: `make test`
- [ ] Coverage maintained or improved
- [ ] No tests skipped without documentation
- [ ] CI pipeline passes
