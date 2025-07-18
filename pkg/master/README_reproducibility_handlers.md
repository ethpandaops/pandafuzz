# Reproducibility Handlers Implementation

This document describes the reproducibility HTTP handlers that have been implemented in `handlers_reproducibility.go`.

## Overview

Four HTTP handlers have been created to support the reproducibility feature:

1. **handleCrashReproduce** - POST /api/crashes/:crashID/reproduce
   - Queues a crash for reproduction testing
   - Accepts optional priority parameter (0-10, default 5)

2. **handleGetCrashReproduction** - GET /api/crashes/:crashID/reproduction
   - Gets the current status of a reproduction task
   - Returns reproduction request details and score if available

3. **handleSubmitReproductionResult** - POST /api/reproduction/results
   - Records the result of a reproduction attempt
   - Used by bots to submit their reproduction test results

4. **handleGetReproductionResults** - GET /api/crashes/:crashID/reproduction/results
   - Gets all reproduction results for a crash
   - Supports pagination and returns reproducibility score

## Integration Status

The handlers are currently **NOT FUNCTIONAL** and return HTTP 501 (Not Implemented) because the reproducibility service is not yet integrated into the service.Manager.

## Required Integration Steps

To make these handlers functional, the following steps need to be completed:

1. **Add Reproducibility field to service.Manager** in `pkg/service/manager.go`:
   ```go
   type Manager struct {
       // ... existing fields ...
       Reproducibility common.ReproducibilityService
   }
   ```

2. **Initialize the reproducibility service** in `NewManager()`:
   ```go
   reproducibilityService := NewReproducibilityService(storage, config, logger)
   ```

3. **Add missing Storage interface methods** in `pkg/common/interfaces.go`:
   ```go
   CreateReproductionResult(ctx context.Context, result *ReproductionResult) error
   ```

4. **Fix TimeoutConfig** to include JobTimeout field or use existing timeout configuration

5. **Start the reproducibility service** in the Manager's Start method

## Handler Features

- **Input validation**: All handlers validate required parameters
- **Error handling**: Proper HTTP status codes and error messages
- **Pagination support**: Results endpoint supports page/limit query parameters
- **JSON response format**: Consistent response structure using httputil helpers
- **Logging**: Structured logging with relevant fields
- **Priority queue**: Reproduction requests can be prioritized (0-10 scale)

## Testing

Once the service integration is complete:

1. Remove the "Not Implemented" responses
2. Uncomment the actual service calls
3. Test with curl or API client:

```bash
# Queue reproduction
curl -X POST http://localhost:8080/api/crashes/crash123/reproduce \
  -H "Content-Type: application/json" \
  -d '{"priority": 8}'

# Check status
curl http://localhost:8080/api/crashes/crash123/reproduction

# Submit result (from bot)
curl -X POST http://localhost:8080/api/reproduction/results \
  -H "Content-Type: application/json" \
  -d '{
    "crash_id": "crash123",
    "request_id": "req456",
    "bot_id": "bot789",
    "attempt_number": 1,
    "reproduced": true,
    "matches_original": true,
    "execution_time": 5000000000,
    "signal": 11,
    "exit_code": -1,
    "output": "Crash reproduced successfully",
    "stack_trace": "...",
    "stack_hash": "abc123"
  }'

# Get results
curl http://localhost:8080/api/crashes/crash123/reproduction/results?page=1&limit=10
```

## Notes

- The routes are already defined in `routes.go` (lines 162-166)
- The handlers follow the existing patterns in the codebase
- All TODO comments in the code indicate where service integration is needed