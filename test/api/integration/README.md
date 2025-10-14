# PandaFuzz API v1 Integration Tests

This directory contains comprehensive integration tests for the PandaFuzz API v1 implementation. The tests validate the entire API functionality from client-server communication to complex workflows.

## Overview

The integration tests are built using the following components:

- **Go testing package** for test framework
- **Testify suite** for structured test organization
- **Generated Go client** from `pkg/clients/go/` for API interaction
- **Real SQLite database** for persistence layer testing
- **In-memory test server** for fast execution

## Test Structure

### Core Test Files

- **`suite_test.go`** - Main test suite setup and configuration
- **`utils.go`** - Utilities, data generators, and validators
- **`api_test.go`** - API initialization and basic functionality
- **`bots_test.go`** - Bot registration, management, and lifecycle
- **`jobs_test.go`** - Job creation, execution, and monitoring
- **`campaigns_test.go`** - Campaign orchestration and management
- **`corpus_test.go`** - Corpus upload, selection, and synchronization
- **`crashes_test.go`** - Crash reporting, deduplication, and analysis
- **`sse_test.go`** - Server-Sent Events streaming and real-time updates
- **`batch_test.go`** - Batch operations and transaction handling

### Test Categories

#### 1. API Foundation Tests (`api_test.go`)
- API server initialization and lifecycle
- Health and readiness endpoints
- Middleware functionality (CORS, rate limiting, authentication)
- Error handling and content negotiation
- Concurrent request handling
- API documentation accessibility

#### 2. Bot Management Tests (`bots_test.go`)
- Bot registration and authentication
- Heartbeat mechanism and status tracking
- Resource usage monitoring
- Bot capabilities management
- Concurrent bot operations
- Bot lifecycle (registration → active → maintenance → deletion)

#### 3. Job Management Tests (`jobs_test.go`)
- Job creation and validation
- Job lifecycle (pending → running → completed/failed)
- Job assignment to bots
- Progress tracking and reporting
- Log streaming and coverage collection
- Job cancellation and timeout handling

#### 4. Campaign Management Tests (`campaigns_test.go`)
- Campaign creation and configuration
- Status transitions (draft → active → paused → completed)
- Job template management
- Campaign statistics and performance metrics
- Multi-fuzzer campaign support
- Campaign deletion with job dependencies

#### 5. Corpus Management Tests (`corpus_test.go`)
- File upload and validation
- Corpus selection algorithms
- Synchronization with external sources
- Quarantine operations
- Coverage tracking and generation info
- Concurrent upload operations

#### 6. Crash Analysis Tests (`crashes_test.go`)
- Crash reporting and classification
- Deduplication algorithms
- Minimization and reproduction
- Severity level management
- Stack trace analysis
- Concurrent crash processing

#### 7. Event Streaming Tests (`sse_test.go`)
- SSE connection establishment
- Event filtering and topic subscription
- Reconnection handling
- Backpressure management
- Event format compliance
- Connection limits and multiplexing

#### 8. Batch Operations Tests (`batch_test.go`)
- Multi-operation batch requests
- Transaction support and rollback
- Concurrent vs sequential execution
- Partial failure handling
- Timeout and error management
- Large batch processing

## Test Patterns

### Table-Driven Tests
```go
testCases := []struct {
    name        string
    request     SomeRequest
    expectedErr bool
}{
    {"valid_case", validRequest, false},
    {"invalid_case", invalidRequest, true},
}

for _, tc := range testCases {
    s.Run(tc.name, func() {
        // Test implementation
    })
}
```

### Concurrent Testing
```go
const numOperations = 10
results := make(chan error, numOperations)

for i := 0; i < numOperations; i++ {
    go func(index int) {
        // Concurrent operation
        results <- err
    }(i)
}

for i := 0; i < numOperations; i++ {
    s.NoError(<-results)
}
```

### Event-Driven Testing
```go
collector := NewEventCollector()
ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
defer cancel()

go s.connectAndCollectEvents(ctx, collector, "topic")
// Trigger events
s.createTestResource()
// Verify events received
s.assertTrue(collector.WaitForEvents(1, 5*time.Second))
```

## Test Utilities

### Data Generators
- `TestDataGenerator` - Creates realistic test data
- `generateTestName()` - Unique test resource names
- `generateTestUUID()` - Test-safe UUIDs

### Response Validators
- `ResponseValidator` - Validates API response structure
- `parseJSONResponse()` - Safe JSON parsing with error handling
- `assertStatusCode()` - HTTP status code assertions

### Event Collectors
- `EventCollector` - SSE event collection and filtering
- `WaitForEvents()` - Event-based test synchronization
- `GetLastEvent()` - Event content validation

## Running Tests

### Full Test Suite
```bash
go test ./test/api/integration/...
```

### Specific Test Files
```bash
go test ./test/api/integration/ -run TestBots
go test ./test/api/integration/ -run TestJobs
go test ./test/api/integration/ -run TestCampaigns
```

### Verbose Output
```bash
go test ./test/api/integration/ -v
```

### Test Coverage
```bash
go test ./test/api/integration/ -cover
```

### Parallel Execution
```bash
go test ./test/api/integration/ -parallel 4
```

## Test Environment

### Configuration
- **Database**: In-memory SQLite for fast execution
- **Storage**: Temporary filesystem backend
- **Server**: Localhost with dynamic port allocation
- **Timeouts**: Reduced timeouts for faster test cycles
- **Logging**: Structured logging for test debugging

### Resource Management
- Automatic cleanup between tests
- Temporary directory creation/removal
- Database table cleanup
- Context cancellation for timeout handling

### Test Isolation
- Each test starts with clean state
- No shared resources between tests
- Independent database transactions
- Separate temporary directories

## Best Practices

### Test Design
1. **Idempotent Tests** - Tests can run multiple times
2. **Independent Tests** - No dependencies between tests
3. **Realistic Scenarios** - Test real-world usage patterns
4. **Error Coverage** - Test both success and failure paths
5. **Concurrent Safety** - Validate thread safety

### Performance
1. **Fast Execution** - Tests should complete quickly
2. **Parallel Safe** - Tests can run concurrently
3. **Resource Efficient** - Minimal memory and CPU usage
4. **Proper Cleanup** - No resource leaks

### Maintainability
1. **Clear Test Names** - Descriptive test function names
2. **Good Documentation** - Comments explaining complex tests
3. **Consistent Patterns** - Similar tests follow same structure
4. **Helper Functions** - Reusable test utilities

## Troubleshooting

### Common Issues

#### Server Not Starting
```
API server not ready after retries
```
**Solution**: Check port conflicts, increase timeout, verify configuration

#### Test Timeouts
```
Context deadline exceeded
```
**Solution**: Increase test timeouts, check for resource contention

#### Database Errors
```
Database locked or table doesn't exist
```
**Solution**: Ensure proper cleanup, check migration status

#### SSE Connection Failures
```
SSE endpoint not available
```
**Solution**: Verify server supports SSE, check Content-Type headers

### Debug Mode
Enable verbose logging for troubleshooting:
```go
func init() {
    EnableDebugLogging()
}
```

### Test Environment Variables
```bash
export PANDAFUZZ_TEST_MODE=true
export LOG_LEVEL=debug
export TEST_TIMEOUT=60s
```

## Integration with CI/CD

### GitHub Actions
```yaml
- name: Run API Integration Tests
  run: |
    go test ./test/api/integration/... -v -timeout=10m
    go test ./test/api/integration/... -race -timeout=15m
```

### Test Reporting
- JUnit XML output for CI integration
- Coverage reports for code quality metrics
- Performance benchmarks for regression detection

## Contributing

### Adding New Tests
1. Follow existing test patterns and naming conventions
2. Include both positive and negative test cases
3. Add appropriate documentation and comments
4. Ensure tests are deterministic and repeatable

### Modifying Existing Tests
1. Maintain backward compatibility
2. Update documentation when behavior changes
3. Consider impact on test execution time
4. Validate tests pass in different environments

### Test Review Checklist
- [ ] Tests follow existing patterns
- [ ] All edge cases are covered
- [ ] Tests are properly documented
- [ ] No flaky or intermittent failures
- [ ] Resource cleanup is handled correctly
- [ ] Tests run efficiently and complete quickly