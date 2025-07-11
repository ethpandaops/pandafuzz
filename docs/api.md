# PandaFuzz API Documentation

## Overview

PandaFuzz provides a RESTful API for fuzzing orchestration. All endpoints follow standard HTTP conventions and return JSON responses.

## Core Operations

### Bot Management

- `POST /api/bots/register` - Register a new bot with capabilities
- `GET /api/jobs` - List all jobs with filtering
- `POST /api/jobs` - Create a new fuzzing job with strategy
- `GET /api/crashes/{job_id}` - Get crashes for a job with deduplication

### Advanced Features

- `POST /api/mutators` - Upload custom mutator
- `POST /api/grammars` - Upload grammar definition
- `GET /api/corpus/seeds` - Get prioritized corpus seeds
- `POST /api/leaks` - Report memory leaks
- `GET /api/strategies` - List available fuzzing strategies

## Master API Endpoints (v1 - Core)

### Bot Lifecycle Management

```
POST   /api/v1/bots/register           # Bot registration with timeout
DELETE /api/v1/bots/{id}               # Bot deregistration
POST   /api/v1/bots/{id}/heartbeat     # Bot heartbeat with status
GET    /api/v1/bots/{id}/job           # Atomic job assignment
POST   /api/v1/bots/{id}/job/complete  # Job completion notification
```

### Result Communication (Bot -> Master)

```
POST   /api/v1/results/crash           # Report crash with metadata
POST   /api/v1/results/coverage        # Report coverage data
POST   /api/v1/results/corpus          # Report corpus updates
POST   /api/v1/results/status          # Report job status updates
```

### Job Management (Admin)

```
POST   /api/v1/jobs                    # Create fuzzing job
GET    /api/v1/jobs                    # List jobs (paginated)
GET    /api/v1/jobs/{id}               # Get job details
PUT    /api/v1/jobs/{id}/cancel        # Cancel job
GET    /api/v1/jobs/{id}/logs          # Get job logs
```

### System Status

```
GET    /api/v1/status                  # System health check
GET    /api/v1/metrics                 # Basic metrics
GET    /api/v1/bots                    # List active bots
```

## Error Handling & Timeouts

All endpoints include:
- Request timeout: 30s
- Bot operation timeout: 5m
- Job execution timeout: configurable (default: 1h)
- Master restart recovery: automatic
- Atomic operations for state changes

## Response Formats

### Success Response
```json
{
  "success": true,
  "data": { ... },
  "timestamp": "2024-01-20T12:00:00Z"
}
```

### Error Response
```json
{
  "success": false,
  "error": {
    "code": "RESOURCE_NOT_FOUND",
    "message": "Job not found",
    "details": { ... }
  },
  "timestamp": "2024-01-20T12:00:00Z"
}
```

## Authentication

PandaFuzz operates behind a VPN and does not require additional authentication. All requests are trusted within the VPN environment.

## Rate Limiting

No rate limiting is enforced as the system operates in a trusted environment.