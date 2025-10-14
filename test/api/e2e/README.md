# PandaFuzz API v1 End-to-End Tests (Playwright)

This directory contains comprehensive end-to-end tests for PandaFuzz API v1 using the Playwright framework and the official TypeScript client library.

## Overview

The test suite validates critical user journeys and API workflows including:

- **Bot Lifecycle**: Registration, heartbeat, job assignment, and disconnection scenarios
- **Campaign Execution**: Creation, orchestration, monitoring, and completion workflows  
- **Crash Discovery**: Detection, minimization, deduplication, and analysis processes

## Directory Structure

```
test/api/e2e/
├── specs/                          # Test specifications
│   ├── bot-lifecycle.spec.ts       # Bot lifecycle tests
│   ├── campaign-execution.spec.ts  # Campaign workflow tests
│   └── crash-discovery.spec.ts     # Crash handling tests
├── utils/                          # Shared utilities (future)
├── test-utils.ts                   # Test helpers and utilities
├── global-setup.ts                 # Global test setup
├── global-teardown.ts              # Global test cleanup
├── playwright.config.ts            # Playwright configuration
├── package.json                    # Dependencies and scripts
├── tsconfig.json                   # TypeScript configuration
└── README.md                       # This documentation
```

## Prerequisites

### System Requirements

- Node.js 18.0.0 or higher
- TypeScript 5.2.2 or higher
- PandaFuzz API server running (local or remote)

### PandaFuzz Setup

Ensure PandaFuzz master and bot services are running:

```bash
# Option 1: Run locally
cd ../../../
make run-master  # Terminal 1
make run-bot     # Terminal 2

# Option 2: Using Docker
docker-compose up -d pandafuzz-master pandafuzz-bot
```

## Installation

Install dependencies and set up the test environment:

```bash
# Navigate to e2e test directory
cd test/api/e2e

# Install dependencies
npm install

# Install Playwright browsers
npm run install:browsers

# Verify setup
npm run type-check
```

## Running Tests

### Basic Test Execution

```bash
# Run all e2e tests
npm test

# Run with visible browser (headed mode)
npm run test:headed

# Run with Playwright UI (interactive)
npm run test:ui

# Debug specific test
npm run test:debug -- --grep "bot registration"
```

### Test Filtering

Run specific test suites:

```bash
# Bot lifecycle tests only
npx playwright test bot-lifecycle.spec.ts

# Campaign execution tests only  
npx playwright test campaign-execution.spec.ts

# Crash discovery tests only
npx playwright test crash-discovery.spec.ts

# Run tests matching pattern
npx playwright test --grep "heartbeat"
```

### Environment Configuration

Configure API endpoint and test settings:

```bash
# Set custom API URL
export PANDAFUZZ_API_URL="http://localhost:8080/api/v1"

# Run tests with custom configuration
npm test

# Use different environment
PANDAFUZZ_API_URL="http://staging-api:8080/api/v1" npm test
```

### Docker Integration

Run tests against dockerized PandaFuzz:

```bash
# Start services and run tests
npm run e2e

# Start services for development
npm run e2e:dev
```

## Test Architecture

### Test Utilities

The test suite provides comprehensive utilities in `test-utils.ts`:

#### TestSetup Class
- **Resource Tracking**: Automatic cleanup of bots, jobs, campaigns, and crashes
- **Lifecycle Management**: Setup and teardown for test resources

#### EventCollector Class  
- **SSE Event Collection**: Real-time event monitoring and validation
- **Event Filtering**: Selective event collection based on types or predicates
- **Promise-based Waiting**: Await specific events with timeout handling

#### DataGenerators Class
- **Test Data Creation**: Generate realistic bot, job, campaign, and crash data
- **Unique Identifiers**: Ensure test isolation with unique IDs
- **Configurable Overrides**: Customize generated data for specific test scenarios

#### ApiHelpers Class
- **Status Polling**: Wait for resources to reach target states
- **Error Handling**: Robust retry and error recovery mechanisms
- **Timing Utilities**: Sleep and timeout management

### Custom Matchers

Extended Playwright assertions for API testing:

```typescript
// Validate API response structure
expect(response).toMatchApiResponse();

// Check timestamp validity
expect(timestamp).toHaveValidTimestamp();

// Validate resource usage percentages
expect(resourceUsage).toBeValidResourceUsage();
```

## Test Scenarios

### Bot Lifecycle Tests

**Registration and Validation**
- New bot registration with valid data
- Duplicate registration prevention
- Invalid data rejection
- Optional field handling

**Heartbeat Mechanism**
- Status updates and resource reporting
- Command reception and processing
- Resource usage tracking over time
- Timeout and reconnection handling

**Job Assignment**
- Job assignment through heartbeat
- Progress reporting during execution
- Cancellation handling
- Resource constraint respect

**Connection Management**
- Graceful disconnection
- Reconnection after timeouts
- Job state recovery
- Event emission for status changes

### Campaign Execution Tests

**Campaign Creation**
- Valid campaign configuration
- Fuzzer type variations (AFL++, LibFuzzer, Honggfuzz)
- Resource requirement specification
- Configuration validation

**Orchestration**
- Campaign start/stop operations
- Concurrent job limit enforcement
- Bot distribution algorithms
- Failure recovery mechanisms

**Monitoring**
- Real-time progress via SSE events
- Statistics accuracy and consistency
- Job completion tracking
- Performance metrics collection

**Error Scenarios**
- No available bots handling
- Invalid operation rejection
- Campaign deletion workflows

### Crash Discovery Tests

**Detection and Reporting**
- Crash detection during fuzzing
- Manual crash reporting validation
- Metadata preservation
- Multiple crash type support

**Minimization**
- Minimization process initiation
- Strategy-specific behavior (binary search, linear, delta)
- Input size reduction verification
- Error handling and recovery

**Deduplication**
- Duplicate crash identification
- Unique crash distinction
- Strategy comparison (stack trace, input hash, combined)
- Similarity threshold tuning

**Analysis**
- Detailed crash information retrieval
- Reproduction request handling
- Filtering and search capabilities
- Statistical insights generation

## Configuration

### Playwright Configuration

Key configuration options in `playwright.config.ts`:

```typescript
{
  testDir: './specs',           // Test location
  timeout: 30 * 1000,          // 30 second test timeout
  expect: { timeout: 10 * 1000 }, // 10 second assertion timeout
  retries: process.env.CI ? 2 : 0, // Retry on CI
  workers: process.env.CI ? 1 : undefined, // Parallel execution
  use: {
    baseURL: 'http://localhost:8080/api/v1',
    trace: 'on-first-retry',    // Debug traces
    video: 'retain-on-failure', // Video recording
    screenshot: 'only-on-failure'
  }
}
```

### Global Setup

The `global-setup.ts` ensures:
- API availability before test execution
- System readiness validation
- Connection verification
- Environment preparation

### Test Isolation

Each test maintains isolation through:
- Unique resource identifiers
- Automatic cleanup after each test
- Independent client instances
- Resource tracking and management

## Debugging and Troubleshooting

### Common Issues

**API Connection Failures**
```bash
# Check API availability
curl http://localhost:8080/api/v1/health

# Verify network connectivity
ping localhost

# Check service logs
docker-compose logs pandafuzz-master
```

**Test Timeouts**
```bash
# Run with longer timeout
npx playwright test --timeout=60000

# Enable debug mode
DEBUG=pw:api npm test

# Check resource contention
npx playwright test --workers=1
```

**Resource Cleanup Issues**
```bash
# Manual cleanup if needed
curl -X DELETE http://localhost:8080/api/v1/bots/test-bot-id
curl -X DELETE http://localhost:8080/api/v1/jobs/test-job-id
```

### Debug Mode

Enable detailed debugging:

```bash
# Playwright debug mode
npx playwright test --debug

# API request logging
DEBUG=pw:api npm test

# Full debug output
DEBUG=* npm test
```

### Test Results

View test results and reports:

```bash
# Show HTML report
npm run test:report

# View last run results
npx playwright show-report

# Check specific test output
npx playwright test --reporter=list
```

## Performance Considerations

### Test Execution Time

- **Individual tests**: 30-120 seconds depending on complexity
- **Full suite**: 10-20 minutes with parallel execution
- **Resource setup**: 5-10 seconds per test for bot/job creation

### Resource Usage

- **Memory**: ~200MB for test runner + API client overhead
- **Network**: Moderate HTTP traffic to API endpoints
- **Disk**: Test artifacts stored in `test-results/` directory

### Optimization Tips

```bash
# Run specific test patterns for faster iteration
npx playwright test --grep "registration"

# Reduce parallelism for resource-constrained environments  
npx playwright test --workers=1

# Skip browser installation for API-only tests
PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 npm install
```

## Continuous Integration

### GitHub Actions Integration

Example workflow configuration:

```yaml
name: E2E Tests
on: [push, pull_request]
jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-node@v3
        with:
          node-version: '18'
      - run: npm ci
        working-directory: test/api/e2e
      - run: npx playwright install --with-deps
        working-directory: test/api/e2e  
      - run: npm run e2e
        working-directory: test/api/e2e
        env:
          PANDAFUZZ_API_URL: http://localhost:8080/api/v1
```

### Docker Integration

Run tests in containerized environment:

```bash
# Build and run with Docker Compose
docker-compose -f docker-compose.test.yml up --build

# Run tests against specific environment
docker run -e PANDAFUZZ_API_URL=http://api:8080/api/v1 pandafuzz-e2e-tests
```

## Contributing

### Adding New Tests

1. **Create test specification**:
   ```typescript
   // specs/new-feature.spec.ts
   import { test, expect } from '@playwright/test';
   import { TestSetup, DataGenerators } from '../test-utils';
   
   test.describe('New Feature', () => {
     // Test implementation
   });
   ```

2. **Use existing utilities**:
   ```typescript
   const testSetup = new TestSetup(client);
   const testData = DataGenerators.createJobData();
   await ApiHelpers.waitForJobStatus(jobId, ['completed']);
   ```

3. **Follow naming conventions**:
   - Test files: `feature-name.spec.ts`
   - Test IDs: `DataGenerators.generateId('test-prefix')`
   - Resource tracking: `testSetup.trackResource(id)`

### Test Guidelines

- **Isolation**: Each test should be independent and clean up after itself
- **Timing**: Use appropriate timeouts for async operations
- **Assertions**: Prefer specific assertions over generic checks
- **Documentation**: Include clear test descriptions and comments
- **Error Handling**: Test both success and failure scenarios

### Code Quality

```bash
# Lint code
npm run lint

# Fix auto-fixable issues
npm run lint:fix

# Type checking
npm run type-check

# Run all quality checks
npm run lint && npm run type-check
```

## License

This test suite is part of the PandaFuzz project and follows the same licensing terms.