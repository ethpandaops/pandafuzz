# PandaFuzz Integration Tests

This directory contains comprehensive integration tests for the PandaFuzz distributed fuzzing platform.

## Test Structure

The integration tests are organized into the following categories:

### 1. Master-Bot Communication Tests (`master_bot_test.go`)
- Bot registration and deregistration
- Heartbeat mechanism and timeout handling
- Multiple bot connections
- Network failure and reconnection scenarios
- Concurrent bot operations

### 2. Job Flow Tests (`job_flow_test.go`)
- Job creation and assignment workflow
- Job completion and failure handling
- Job timeout and cancellation
- Priority-based job scheduling
- Multi-bot job distribution
- Job filtering by capabilities

### 3. Crash Reporting Tests (`crash_reporting_test.go`)
- Crash report submission and storage
- Crash deduplication mechanisms
- Integration with crash analysis engine
- Coverage reporting
- Corpus update handling
- Large input handling
- Concurrent crash reporting

### 4. Recovery Tests (`recovery_test.go`)
- System recovery on startup
- Orphaned job recovery
- Bot failure recovery procedures
- Timeout recovery mechanisms
- System state validation
- Maintenance recovery
- Recovery metrics collection

### 5. API Tests (`api_test.go`)
- RESTful API endpoint testing
- Health and status endpoints
- CRUD operations for bots and jobs
- Error handling and validation
- Pagination and filtering
- Concurrent API requests
- Authentication testing

### 6. Fuzzer Integration Tests (`fuzzer_test.go`)
- AFL++ fuzzer integration
- LibFuzzer integration
- Fuzzer configuration and validation
- Crash detection and reporting
- Coverage collection
- Corpus management
- Event handling

## Running Tests

### Run All Integration Tests
```bash
go test ./tests/integration/... -v
```

### Run Specific Test Suite
```bash
# Run only master-bot communication tests
go test ./tests/integration -run TestMasterBot -v

# Run only job flow tests
go test ./tests/integration -run TestJob -v

# Run only crash reporting tests
go test ./tests/integration -run TestCrash -v
```

### Run with Debug Logging
```bash
PANDAFUZZ_LOG_LEVEL=debug go test ./tests/integration/... -v
```

### Run Benchmarks
```bash
go test ./tests/integration -bench=. -benchmem
```

### Skip Long-Running Tests
```bash
go test ./tests/integration -short
```

## Test Helpers

The `helpers_test.go` file provides utilities for:
- Setting up test environments
- Creating test data (bots, jobs, crashes)
- Managing test lifecycle
- Asserting eventual consistency
- Mock implementations

## Writing New Tests

When adding new integration tests:

1. Use the `SetupTestEnvironment()` helper to create a test environment
2. Always defer `env.Cleanup()` to ensure proper cleanup
3. Use `require` for critical assertions that should stop the test
4. Use `assert` for non-critical checks
5. Use `AssertEventually()` for conditions that may take time
6. Add appropriate timeouts to prevent hanging tests

Example:
```go
func TestNewFeature(t *testing.T) {
    env := SetupTestEnvironment(t)
    
    // Start master server
    err := env.StartMaster()
    require.NoError(t, err)
    
    // Your test logic here
    
    // Use AssertEventually for async operations
    AssertEventually(t, func() bool {
        // Check condition
        return true
    }, 5*time.Second, "Condition not met")
}
```

## Test Coverage

To generate test coverage report:
```bash
go test ./tests/integration/... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

## CI/CD Integration

These tests are designed to run in CI/CD pipelines. They:
- Use temporary directories for all file operations
- Clean up resources after completion
- Have configurable timeouts
- Support parallel execution
- Generate JUnit-compatible reports with `-json` flag

## Troubleshooting

### Tests Hanging
- Check for proper cleanup in deferred functions
- Ensure all goroutines are properly terminated
- Use shorter timeouts in test configurations

### Port Conflicts
- Tests use port 0 to let the OS assign available ports
- Each test creates its own isolated environment

### Database Issues
- Each test uses a separate SQLite database in a temp directory
- Database files are automatically cleaned up

### Fuzzer Not Found
- Some tests skip if fuzzers (AFL++, LibFuzzer) are not installed
- Install fuzzers or use `-short` flag to skip these tests