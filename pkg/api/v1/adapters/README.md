# API v1 Adapters

This package contains adapter implementations that bridge the generated OpenAPI server interface to the existing domain services in PandaFuzz.

## Architecture

The adapters follow a layered architecture pattern:

```
Generated API ↔ Adapters ↔ Domain Services ↔ Repositories
```

## Files

### Core Adapters

- **`bot_adapter.go`** - Handles bot registration, status management, and heartbeat processing
- **`job_adapter.go`** - Manages job creation, execution, monitoring, and artifact retrieval  
- **`campaign_adapter.go`** - Campaign lifecycle management and statistics
- **`corpus_adapter.go`** - Corpus file management, selection, and synchronization
- **`crash_adapter.go`** - Crash analysis, deduplication, and minimization
- **`analytics_adapter.go`** - System metrics, performance stats, and trend analysis

### Composite

- **`composite_adapter.go`** - Combines all adapters into a single implementation of the generated `ServerInterface`

## Key Features

### 1. Type Conversion
Each adapter handles conversion between:
- Generated OpenAPI types (e.g., `generated.Bot`) 
- Domain model types (e.g., `botTypes.Agent`)

### 2. Error Handling
All adapters implement consistent error handling using:
- RFC 7807 Problem Details format
- Structured error responses
- Proper HTTP status codes

### 3. Server-Sent Events (SSE)
Adapters integrate with the SSE manager to broadcast real-time events:
- Bot lifecycle events
- Job progress updates
- Campaign state changes
- Crash discoveries
- Corpus synchronization

### 4. Pagination
List endpoints support both offset-based and cursor-based pagination with configurable limits.

### 5. Filtering & Sorting
List endpoints support filtering by various criteria and sorting options.

## Usage

### Creating the Composite Adapter

```go
// Initialize individual adapters
botAdapter := NewBotAdapter(botRegistry, botRepo, jobRepo, sseManager, logger)
jobAdapter := NewJobAdapter(jobRepo, executor, sseManager, logger)
campaignAdapter := NewCampaignAdapter(campaignService, campaignRepo, sseManager, logger)
corpusAdapter := NewCorpusAdapter(corpusRepo, syncService, quarantine, sseManager, logger)
crashAdapter := NewCrashAdapter(crashRepo, dedupService, minimizer, sseManager, logger)
analyticsAdapter := NewAnalyticsAdapter(jobRepo, crashRepo, campaignRepo, sseManager, logger)

// Create composite adapter
composite := NewCompositeAdapter(
    botAdapter,
    jobAdapter, 
    campaignAdapter,
    corpusAdapter,
    crashAdapter,
    analyticsAdapter,
    sseManager,
    logger,
)

// Use with HTTP server
server := &http.Server{
    Handler: generated.HandlerFromMux(composite, mux),
}
```

## Implementation Details

### Bot Adapter
- Converts between `botTypes.Agent` and `generated.Bot`
- Manages bot capabilities and status transitions
- Publishes heartbeat and registration events
- Retrieves jobs assigned to specific bots

### Job Adapter  
- Handles job lifecycle (create, update, cancel)
- Streams job logs via SSE
- Provides coverage report downloads
- Manages job artifacts and progress tracking

### Campaign Adapter
- Campaign state management (start, stop, pause)
- Statistics aggregation and reporting
- Integration with job template system
- Campaign configuration validation

### Corpus Adapter
- File upload handling with deduplication
- Corpus selection algorithms
- Synchronization between campaigns
- Quarantine management for suspicious files

### Crash Adapter
- Crash deduplication using multiple algorithms
- Input minimization with various strategies
- Crash reproduction and verification
- Classification and severity assessment

### Analytics Adapter
- Real-time system metrics
- Performance bottleneck detection
- Coverage trend analysis
- Optimization recommendations

## Mock Implementations

Most adapters include mock implementations for data that would normally come from external services or databases. These are clearly marked with comments and should be replaced with actual service calls in production.

## Error Handling

All adapters use a consistent error handling pattern:

```go
func (a *Adapter) writeError(w http.ResponseWriter, statusCode int, errorType, title string, err error) {
    problem := generated.ProblemDetails{
        Type:      fmt.Sprintf("/errors/%s", strings.ToLower(errorType)),
        Title:     title,
        Status:    statusCode,
        Timestamp: &[]time.Time{time.Now()}[0],
    }
    
    if err != nil {
        detail := err.Error()
        problem.Detail = &detail
    }
    
    w.Header().Set("Content-Type", "application/problem+json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(problem)
}
```

## Testing

Each adapter should have corresponding unit tests that:
- Test type conversions
- Verify error handling
- Mock domain service interactions
- Validate SSE event publishing
- Check pagination and filtering

## Thread Safety

All adapters are designed to be thread-safe and can handle concurrent requests. They rely on the underlying domain services and repositories for thread safety guarantees.

## Configuration

Adapters are configured through their constructors and depend on:
- Domain service instances
- Repository interfaces
- SSE manager for real-time events
- Structured logger for observability

## Observability

All adapters include structured logging with:
- Request/response logging
- Error tracking
- Performance metrics
- SSE event publishing

## Future Enhancements

- Rate limiting per endpoint
- Request/response caching
- Advanced filtering capabilities
- Webhook support for external integrations
- Batch operation optimization