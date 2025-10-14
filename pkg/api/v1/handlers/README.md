# API v1 Handlers

This package provides thin HTTP endpoint handlers for the PandaFuzz API v1. The handlers act as a bridge between HTTP requests and the service adapters, parsing query parameters and delegating business logic to the appropriate adapter methods.

## Architecture

The handlers follow a clean architecture pattern:
- **Thin Handlers**: Parse HTTP requests, extract parameters, delegate to adapters
- **No Business Logic**: All business logic is handled by the adapters
- **Parameter Extraction**: Convert URL parameters and query strings to typed parameters
- **Error Handling**: Rely on adapters for comprehensive error handling

## Files

### Core Handler Infrastructure

- **`handlers.go`** - Main handler struct, route registration, and common utilities
- **`health.go`** - Health check and readiness endpoints

### Resource Handlers

- **`bots.go`** - Bot management endpoints (CRUD, heartbeat, job listing)
- **`jobs.go`** - Job lifecycle endpoints (CRUD, logs, coverage, artifacts)
- **`campaigns.go`** - Campaign orchestration endpoints (CRUD, start/stop, stats)
- **`corpus.go`** - Corpus management endpoints (upload, sync, quarantine)
- **`crashes.go`** - Crash analysis endpoints (minimize, reproduce, deduplicate)
- **`analytics.go`** - Analytics and metrics endpoints

### Special Operations

- **`batch.go`** - Batch operation processing
- **`events.go`** - Server-Sent Events (SSE) streaming

## Usage

```go
package main

import (
    "net/http"
    
    "github.com/go-chi/chi/v5"
    "github.com/ethpandaops/pandafuzz/pkg/api/v1/adapters"
    "github.com/ethpandaops/pandafuzz/pkg/api/v1/handlers"
    "github.com/ethpandaops/pandafuzz/pkg/api/v1/middleware"
)

func main() {
    // Create adapters (business logic)
    adapter := adapters.NewCompositeAdapter(...)
    
    // Create middleware stack
    middlewareStack := middleware.NewStack(...)
    
    // Create handlers
    h := handlers.NewHandlers(adapter, middlewareStack, logger)
    
    // Set up router and register routes
    r := chi.NewRouter()
    h.RegisterRoutes(r)
    
    // Start server
    http.ListenAndServe(":8080", r)
}
```

## Handler Responsibilities

Each handler is responsible for:

1. **Parameter Extraction**: Parse URL parameters using Chi's `URLParam()`
2. **Query Parameter Parsing**: Convert query strings to typed parameters
3. **Parameter Validation**: Basic type conversion (strconv) with error handling
4. **Adapter Delegation**: Pass processed parameters to the appropriate adapter method
5. **Error Logging**: Log any parsing errors (adapters handle response errors)

## Query Parameter Patterns

Handlers support common query parameter patterns:

- **Pagination**: `limit`, `offset`
- **Filtering**: `status`, `fuzzer`, `campaign_id`, `bot_id`
- **Sorting**: `sort_by`, `sort_order`
- **Time Ranges**: `since`, `until`
- **Inclusion Flags**: `include_jobs`, `include_stats`, `include_coverage`
- **Granularity**: `granularity` for analytics endpoints

## Error Handling

Handlers use a lightweight error handling approach:
- Parse parameters with graceful fallback (ignore invalid values)
- Log parsing errors but continue with request processing
- Delegate all business logic errors to adapters
- Adapters return RFC 7807 Problem Details responses

## Design Principles

- **Single Responsibility**: Each handler handles one endpoint
- **Thin Layer**: Minimal logic, maximum delegation
- **Type Safety**: Use generated types for all parameters
- **Graceful Degradation**: Invalid parameters are ignored, not failed
- **Logging**: Comprehensive logging for debugging and monitoring