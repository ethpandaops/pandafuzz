# 06: Unify API Versions

## Priority: MEDIUM
## Risk Level: MEDIUM
## Estimated Effort: 8-12 hours

## Prerequisites

- Complete steps 01-05 first
- State refactoring must be complete as API handlers depend on it

## Problem Statement

The codebase has inconsistent API organization:

1. **`pkg/api/v1/`** - Well-organized with:
   - `handlers/` - Request handlers
   - `adapters/` - Service adapters
   - `middleware/` - HTTP middleware
   - `errors/` - Error types
   - `sse/` - Server-sent events
   - `generated/` - OpenAPI-generated code
   - `openapi/` - OpenAPI specs

2. **`pkg/master/api_v3/`** - Inconsistently organized:
   - Handlers directly in package
   - No OpenAPI spec
   - No clear adapter pattern
   - Mixed with master package

There's no visible v2, suggesting v3 may be a misnomer or v2 was skipped.

## Invariants (MUST NOT CHANGE)

1. All existing API endpoints must continue to work
2. API response formats must remain identical
3. Authentication/authorization behavior must be preserved
4. Rate limiting and CORS behavior must be preserved
5. SSE subscriptions must continue to work
6. Backward compatibility for API clients

## Discovery: Current API Routes

### From pkg/api/v1/handlers/

Endpoints likely served under `/api/v1/`:
- `/api/v1/bots` - Bot management
- `/api/v1/jobs` - Job management
- `/api/v1/campaigns` - Campaign management
- `/api/v1/crashes` - Crash data
- `/api/v1/corpus` - Corpus management
- `/api/v1/health` - Health checks
- `/api/v1/events` - SSE events
- `/api/v1/analytics` - Analytics

### From pkg/master/api_v3/handlers.go

Endpoints likely served under `/api/v3/`:
- Additional handlers for extended functionality
- Need to audit actual routes

## Decision: Consolidation Strategy

**Option A: Merge v3 into v1** (Recommended)
- v3 handlers become additional v1 handlers
- Single coherent API surface
- Simpler for API consumers

**Option B: Keep Both, Standardize Structure**
- Move v3 to `pkg/api/v3/` with proper structure
- Generate OpenAPI spec for v3
- Maintain both versions

**Recommendation: Option A** - Merge v3 functionality into v1 and deprecate v3 routes.

## Implementation Plan

### Phase 1: Audit All Routes

Create route inventory:

```bash
# Find all route registrations
grep -rn "router\." --include="*.go" pkg/api/ pkg/master/
grep -rn "\.Get\|\.Post\|\.Put\|\.Delete\|\.Patch" --include="*.go" pkg/api/ pkg/master/
```

**Expected routes document:**
```markdown
## v1 Routes (pkg/api/v1/)
- GET    /api/v1/bots
- POST   /api/v1/bots/register
- GET    /api/v1/bots/{id}
- DELETE /api/v1/bots/{id}
- GET    /api/v1/jobs
- POST   /api/v1/jobs
- GET    /api/v1/jobs/{id}
- PUT    /api/v1/jobs/{id}/status
- ...

## v3 Routes (pkg/master/api_v3/)
- GET    /api/v3/coverage/{job_id}
- POST   /api/v3/coverage/{job_id}/upload
- ...
```

### Phase 2: Identify v3-Only Features

Features in v3 not in v1:
- Coverage file uploads
- Job lease management
- Enhanced campaign management
- Corpus service integration
- Crash minimization triggers

### Phase 3: Create v1 Handlers for v3 Features

**New file: pkg/api/v1/handlers/coverage.go**
```go
package handlers

import (
    "net/http"

    "github.com/go-chi/chi/v5"
)

// CoverageHandler handles coverage-related API requests
type CoverageHandler struct {
    state StateProvider
    // ...
}

// GetJobCoverage returns coverage data for a job
// @Summary Get job coverage
// @Tags coverage
// @Param job_id path string true "Job ID"
// @Success 200 {object} CoverageResponse
// @Router /api/v1/jobs/{job_id}/coverage [get]
func (h *CoverageHandler) GetJobCoverage(w http.ResponseWriter, r *http.Request) {
    jobID := chi.URLParam(r, "job_id")
    // ... implementation migrated from v3
}

// UploadCoverageFile handles coverage file uploads
// @Summary Upload coverage file
// @Tags coverage
// @Param job_id path string true "Job ID"
// @Accept multipart/form-data
// @Router /api/v1/jobs/{job_id}/coverage [post]
func (h *CoverageHandler) UploadCoverageFile(w http.ResponseWriter, r *http.Request) {
    // ... implementation migrated from v3
}
```

### Phase 4: Update OpenAPI Specification

**Update pkg/api/v1/openapi/pandafuzz.yaml:**

Add new endpoints:
```yaml
paths:
  /api/v1/jobs/{job_id}/coverage:
    get:
      summary: Get job coverage data
      tags:
        - coverage
      parameters:
        - name: job_id
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Coverage data
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/CoverageResponse'

    post:
      summary: Upload coverage file
      tags:
        - coverage
      # ...
```

### Phase 5: Add Backward Compatibility Routes

For existing v3 clients, add redirect or alias routes:

```go
// pkg/master/api.go or similar
func (s *Server) setupRoutes(r chi.Router) {
    // v1 routes (canonical)
    r.Mount("/api/v1", api.NewV1Router(s.handlers))

    // v3 routes (deprecated, redirect to v1)
    r.Mount("/api/v3", deprecatedV3Router(s.handlers))
}

func deprecatedV3Router(h *handlers.Handlers) chi.Router {
    r := chi.NewRouter()

    // Add deprecation header to all v3 responses
    r.Use(func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Deprecation", "true")
            w.Header().Set("Sunset", "2025-06-01")
            w.Header().Set("Link", "</api/v1>; rel=\"successor-version\"")
            next.ServeHTTP(w, r)
        })
    })

    // Map v3 routes to v1 handlers
    r.Get("/coverage/{job_id}", h.Coverage.GetJobCoverage)
    // ...

    return r
}
```

### Phase 6: Migrate v3 Code to v1 Structure

For each handler in `pkg/master/api_v3/`:

1. **Create corresponding file in `pkg/api/v1/handlers/`**
2. **Create adapter in `pkg/api/v1/adapters/` if needed**
3. **Add error types to `pkg/api/v1/errors/` if needed**
4. **Update OpenAPI spec**
5. **Add tests**

### Phase 7: Delete pkg/master/api_v3/

After migration and verification:
```bash
rm -rf pkg/master/api_v3/
```

## File Migration Map

| Source (v3) | Destination (v1) |
|-------------|------------------|
| `api_v3/handlers.go` | `api/v1/handlers/coverage.go`, `api/v1/handlers/lease.go` |
| `api_v3/middleware.go` | `api/v1/middleware/` (merge if different) |
| `api_v3/validators.go` | `api/v1/middleware/validation.go` (merge) |
| `api_v3/types.go` | `api/v1/generated/types.gen.go` (regenerate from OpenAPI) |
| `api_v3/helpers.go` | `api/v1/handlers/helpers.go` or utils |
| `api_v3/campaign_service_adapter.go` | `api/v1/adapters/campaign_adapter.go` (merge) |
| `api_v3/corpus_service_adapter.go` | `api/v1/adapters/corpus_adapter.go` (merge) |
| `api_v3/integration.go` | Integrate into main router setup |

## Adapter Pattern

Ensure all handlers use the adapter pattern consistently:

```go
// pkg/api/v1/adapters/coverage_adapter.go
package adapters

type CoverageAdapter struct {
    state  StateProvider
    logger *logrus.Logger
}

func NewCoverageAdapter(state StateProvider, logger *logrus.Logger) *CoverageAdapter {
    return &CoverageAdapter{state: state, logger: logger}
}

func (a *CoverageAdapter) GetJobCoverage(ctx context.Context, jobID string) (*CoverageData, error) {
    // Business logic here, handler just does HTTP concerns
}
```

```go
// pkg/api/v1/handlers/coverage.go
type CoverageHandler struct {
    adapter *adapters.CoverageAdapter
}

func (h *CoverageHandler) GetJobCoverage(w http.ResponseWriter, r *http.Request) {
    jobID := chi.URLParam(r, "job_id")

    data, err := h.adapter.GetJobCoverage(r.Context(), jobID)
    if err != nil {
        errors.HandleError(w, err)
        return
    }

    render.JSON(w, r, data)
}
```

## Verification Steps

### 1. Route Inventory
```bash
# List all registered routes
go run cmd/master/main.go --list-routes
# Or add debug endpoint to list routes
```

### 2. API Contract Tests
```bash
# Test all endpoints return expected status codes
curl -s http://localhost:8080/api/v1/jobs | jq .
curl -s http://localhost:8080/api/v3/jobs | jq .  # Should show deprecation header
```

### 3. OpenAPI Validation
```bash
# Validate spec
npx @redocly/cli lint pkg/api/v1/openapi/pandafuzz.yaml
```

### 4. E2E Tests
```bash
npm test  # Playwright tests
```

### 5. Client Compatibility
Test with existing scripts:
```bash
./scripts/create-job.sh
./scripts/run-test-with-corpus.sh
```

## Notes for Future Runs

### OpenAPI Code Generation

After updating the OpenAPI spec, regenerate code:
```bash
make generate-api
# or
oapi-codegen -package generated -generate types,server pkg/api/v1/openapi/pandafuzz.yaml > pkg/api/v1/generated/api.gen.go
```

### SSE Event Types

If v3 added new SSE event types, add them to:
- `pkg/api/v1/sse/events.go`
- `pkg/api/v1/sse/types.go`

### Middleware Ordering

Middleware order matters. Current order should be preserved:
1. Recovery (panic handling)
2. Logging
3. CORS
4. Rate limiting
5. Authentication
6. Validation
7. Handler

### Response Format Consistency

All API responses should follow the same format:
```json
{
    "data": { ... },       // Success response
    "error": {             // Error response
        "code": "string",
        "message": "string",
        "details": { ... }
    },
    "meta": {              // Optional metadata
        "page": 1,
        "total": 100
    }
}
```

## Completion Checklist

- [ ] Audit all v1 and v3 routes
- [ ] Document all endpoints
- [ ] Create coverage handler in v1
- [ ] Create lease handler in v1
- [ ] Create any missing adapters
- [ ] Update OpenAPI specification
- [ ] Regenerate OpenAPI code
- [ ] Add deprecation middleware for v3
- [ ] Map v3 routes to v1 handlers
- [ ] Update all tests
- [ ] Verify backward compatibility
- [ ] Update API documentation
- [ ] Remove pkg/master/api_v3/ after verification period
