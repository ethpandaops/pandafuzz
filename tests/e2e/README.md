# PandaFuzz E2E Tests

This directory contains end-to-end tests for PandaFuzz using Playwright.

## Test Coverage

The E2E test suite covers:

### Legacy Features
- Job creation and management
- Bot registration and heartbeat
- Crash reporting
- Job assignment and reassignment

### New Campaign Features
- Campaign CRUD operations
- Stack-based crash deduplication
- Corpus evolution tracking
- Cross-campaign corpus sharing
- Campaign statistics and metrics
- Auto-restart functionality

### Web UI
- Dashboard functionality
- Campaign management interface
- Crash analysis views
- Real-time WebSocket updates

### API
- REST API v1 endpoints
- REST API v2 endpoints
- WebSocket connections
- Server-Sent Events (SSE) for job progress

### Resilience
- Bot disconnection handling
- Master restart recovery
- State persistence

## Running Tests

### Prerequisites
- Node.js 18+ and npm
- Docker and Docker Compose
- Playwright browsers (installed automatically)

### Quick Start

```bash
# Using the test runner script
./run-e2e-tests.sh

# Or manually with npm
npm install
npm run e2e
```

### Individual Commands

```bash
# Start services
npm run docker:up

# Run tests
npm test

# Run tests with UI
npm run test:ui

# Debug tests
npm run test:debug

# View test report
npm run test:report

# Stop services
npm run docker:down
```

### Environment Variables

- `MASTER_URL`: Override the master URL (default: http://localhost:8088)
- `CI`: Set to true for CI environments

### Test Configuration

See `playwright.config.ts` for test configuration options:
- Timeout: 60 seconds per test
- Parallel execution enabled
- Automatic retries on CI
- Screenshots and videos on failure

## Writing New Tests

Tests are organized by feature area. Follow the existing patterns:

```typescript
test.describe('Feature Name', () => {
  test('should do something', async ({ page }) => {
    // Arrange
    const response = await page.request.post('/api/v1/endpoint', {
      data: { /* request data */ }
    });
    
    // Act & Assert
    expect(response.ok()).toBeTruthy();
  });
});
```

## Debugging Failed Tests

1. Check the test report: `npm run test:report`
2. View failure screenshots in `test-results/`
3. Watch failure videos for visual debugging
4. Run specific test in debug mode: `npx playwright test --debug -g "test name"`
5. Check Docker logs: `npm run docker:logs`

## CI/CD Integration

Tests run automatically on:
- Push to main or develop branches
- Pull requests to main

See `.github/workflows/e2e-tests.yml` for CI configuration.