# PandaFuzz API v3

This directory contains the implementation of PandaFuzz API v3, a comprehensive REST API for distributed fuzzing orchestration.

## Features

- **OpenAPI 3.0 Specification**: Complete API documentation in `openapi.yaml`
- **Request Validation**: Comprehensive input validation using struct tags
- **API Versioning**: Clean versioning with backwards compatibility
- **Middleware Support**: Rate limiting, CORS, deprecation warnings
- **Streaming Support**: Server-Sent Events for real-time updates
- **Batch Operations**: Efficient batch result submission

## Architecture

### Files

- `openapi.yaml` - OpenAPI 3.0 specification
- `handlers.go` - HTTP handlers for all endpoints
- `validators.go` - Request validation logic
- `types.go` - Request/response type definitions
- `helpers.go` - Helper functions for pagination, filtering, etc.
- `middleware.go` - Middleware implementations

### Key Components

#### HandlerV3
The main handler struct that implements all API endpoints. It uses dependency injection for services and includes built-in validation and error handling.

#### Validator
Custom validation framework that supports:
- Required field validation
- Min/max length/value validation
- UUID validation
- URL validation
- Enum validation (oneof)
- Custom validators

#### Middleware

1. **Versioning Middleware**: Adds API version headers and rate limit information
2. **Request Validation Middleware**: Validates request size and adds request IDs
3. **Logging Middleware**: Logs all API requests with timing information
4. **Backwards Compatibility Middleware**: Transforms v2 requests/responses to v3 format
5. **Deprecation Middleware**: Adds deprecation warnings for old API versions
6. **CORS Middleware**: Handles Cross-Origin Resource Sharing

## Usage

### Initialization

```go
import (
    "github.com/ethpandaops/pandafuzz/pkg/master/api_v3"
    "github.com/ethpandaops/pandafuzz/pkg/service"
)

// Create handler
config := &api_v3.Config{
    MaxRequestSize:  10 * 1024 * 1024, // 10MB
    RequestTimeout:  30 * time.Second,
    MaxBatchSize:    1000,
    EnableSwaggerUI: true,
}

handler := api_v3.NewHandlerV3(services, logger, config)

// Register routes
router := mux.NewRouter()
handler.RegisterRoutes(router)

// Apply middleware
wrapped := api_v3.NewDeprecationMiddleware(
    api_v3.NewBackwardsCompatibilityMiddleware(
        api_v3.NewCORSMiddleware(router, []string{"*"}),
    ),
)

// Start server
http.ListenAndServe(":8080", wrapped)
```

### API Endpoints

#### Bot Management
- `GET /api/v3/bots` - List all bots
- `POST /api/v3/bots` - Register a new bot
- `GET /api/v3/bots/{botId}` - Get bot details
- `DELETE /api/v3/bots/{botId}` - Deregister bot
- `POST /api/v3/bots/{botId}/heartbeat` - Send heartbeat
- `POST /api/v3/bots/{botId}/jobs/next` - Get next job
- `POST /api/v3/bots/{botId}/jobs/complete` - Complete job
- `GET /api/v3/bots/{botId}/metrics` - Get bot metrics

#### Job Management
- `GET /api/v3/jobs` - List all jobs
- `POST /api/v3/jobs` - Create a new job
- `GET /api/v3/jobs/{jobId}` - Get job details
- `DELETE /api/v3/jobs/{jobId}` - Cancel job
- `GET /api/v3/jobs/{jobId}/logs` - Get job logs (supports SSE)
- `GET /api/v3/jobs/{jobId}/progress` - Get real-time progress (SSE)
- `GET /api/v3/jobs/{jobId}/crashes` - Get job crashes

#### Campaign Management
- `GET /api/v3/campaigns` - List campaigns
- `POST /api/v3/campaigns` - Create campaign
- `GET /api/v3/campaigns/{campaignId}` - Get campaign
- `PATCH /api/v3/campaigns/{campaignId}` - Update campaign
- `DELETE /api/v3/campaigns/{campaignId}` - Delete campaign
- `GET /api/v3/campaigns/{campaignId}/stats` - Get statistics

#### Corpus Management
- `GET /api/v3/corpus` - List corpus files
- `POST /api/v3/corpus` - Upload corpus files (multipart)
- `GET /api/v3/corpus/{corpusId}` - Get corpus file details
- `DELETE /api/v3/corpus/{corpusId}` - Delete corpus file
- `GET /api/v3/corpus/{corpusId}/download` - Download corpus file
- `POST /api/v3/corpus/sync` - Synchronize corpus
- `POST /api/v3/corpus/promote` - Promote crash to corpus

#### Crash Management
- `GET /api/v3/crashes` - List crashes
- `GET /api/v3/crashes/{crashId}` - Get crash details
- `GET /api/v3/crashes/{crashId}/input` - Download crash input

#### Reproducibility
- `GET /api/v3/reproducibility/requests` - List reproduction requests
- `POST /api/v3/reproducibility/requests` - Create reproduction request
- `GET /api/v3/reproducibility/requests/{requestId}` - Get request details
- `POST /api/v3/reproducibility/results` - Submit reproduction result

#### Result Submission
- `POST /api/v3/results/batch` - Submit batch results
- `POST /api/v3/results/crash` - Submit crash result
- `POST /api/v3/results/coverage` - Submit coverage result
- `POST /api/v3/results/corpus` - Submit corpus update

#### System Management
- `GET /api/v3/system/stats` - Get system statistics
- `GET /api/v3/system/health` - Health check
- `POST /api/v3/system/recovery` - Trigger recovery
- `POST /api/v3/system/maintenance` - Trigger maintenance
- `GET /api/v3/system/timeouts` - List active timeouts
- `POST /api/v3/system/timeouts/{type}/{id}` - Force timeout

### Request/Response Examples

#### Register Bot
```json
POST /api/v3/bots
{
  "hostname": "worker-01.example.com",
  "name": "Worker 01",
  "capabilities": ["afl++", "libfuzzer"],
  "api_endpoint": "http://worker-01.example.com:8081"
}

Response:
{
  "bot_id": "123e4567-e89b-12d3-a456-426614174000",
  "status": "registered",
  "timestamp": "2024-01-15T10:00:00Z",
  "timeout": "2024-01-15T10:05:00Z"
}
```

#### Create Job
```json
POST /api/v3/jobs
{
  "name": "Fuzz libpng",
  "target": "/binaries/libpng_harness",
  "fuzzer": "afl++",
  "duration": "1h",
  "config": {
    "memory_limit": 2147483648,
    "timeout": "30s",
    "dictionary": "/dicts/png.dict"
  },
  "campaign_id": "456e7890-e89b-12d3-a456-426614174000"
}
```

#### Batch Results
```json
POST /api/v3/results/batch
{
  "bot_id": "123e4567-e89b-12d3-a456-426614174000",
  "job_id": "789e0123-e89b-12d3-a456-426614174000",
  "crashes": [
    {
      "hash": "a1b2c3d4...",
      "file_path": "crash_001.bin",
      "type": "segfault",
      "signal": 11,
      "size": 1024
    }
  ],
  "coverage": [
    {
      "edges": 50000,
      "new_edges": 120,
      "exec_count": 1000000
    }
  ]
}
```

## Backwards Compatibility

The API supports backwards compatibility with v2 clients through the `BackwardsCompatibilityMiddleware`. This middleware:

1. Transforms v2 paths to v3 paths
2. Converts request formats (e.g., capabilities string to array)
3. Transforms response formats (e.g., campaign_id to corpus_id)
4. Adds compatibility headers

## Deprecation Policy

Old API versions (v1 and v2) include deprecation headers:
- `X-API-Deprecated: true`
- `X-API-Deprecation-Date: 2024-12-31`
- `X-API-Sunset-Date: 2025-06-30`
- `Warning: 299 - "API version v2 is deprecated. Please migrate to v3."`

## Error Handling

All errors follow a consistent format:

```json
{
  "error": "validation_error",
  "message": "Invalid request",
  "details": {
    "field": "fuzzer"
  },
  "timestamp": "2024-01-15T10:00:00Z",
  "request_id": "123e4567-e89b-12d3-a456-426614174000"
}
```

Error codes:
- `400` - Bad Request (validation errors)
- `401` - Unauthorized
- `404` - Not Found
- `409` - Conflict
- `413` - Payload Too Large
- `500` - Internal Server Error
- `503` - Service Unavailable
- `504` - Gateway Timeout

## Development

### Adding New Endpoints

1. Add endpoint definition to `openapi.yaml`
2. Add handler method to `handlers.go`
3. Add request/response types to `types.go`
4. Add route registration in `RegisterRoutes`
5. Add validation rules if needed

### Custom Validators

To add a custom validator:

```go
validator.RegisterValidator("custom", func(fieldName string, fieldValue interface{}, tag string) error {
    // Validation logic
    if invalid {
        return &ValidationError{
            Field:   fieldName,
            Message: "custom validation failed",
        }
    }
    return nil
})
```

### Testing

The API includes comprehensive test coverage. Run tests with:

```bash
go test ./pkg/master/api_v3/...
```

## Performance Considerations

1. **Batch Operations**: Use batch endpoints for submitting multiple results
2. **Pagination**: All list endpoints support pagination with configurable limits
3. **Streaming**: Use SSE endpoints for real-time updates instead of polling
4. **Caching**: Response headers include cache control directives
5. **Rate Limiting**: Built-in rate limiting per API key/IP

## Security

1. **Authentication**: Bearer token or API key authentication
2. **Request Validation**: All inputs are validated before processing
3. **Rate Limiting**: Configurable rate limits per endpoint
4. **CORS**: Configurable CORS policies
5. **Request Size Limits**: Configurable maximum request size
6. **Timeout Protection**: All operations have timeouts