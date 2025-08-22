# PandaFuzz API v1 Middleware Stack

This directory contains a comprehensive middleware stack for the PandaFuzz API, designed to work with Chi router and follow ethPandaOps Go coding standards.

## Middleware Components

### 1. Authentication (`auth.go`)
- **JWT Authentication**: Validates Bearer tokens using HMAC-SHA256
- **API Key Authentication**: Validates API keys from headers
- **Permission-based Authorization**: Role and permission checking
- **Context Injection**: Stores user/claims data in request context

**Key Features:**
- Support for both JWT and API key authentication
- Custom JWT validation with proper signature verification
- Permission-based access control
- Configurable skip paths
- Thread-safe implementation

**Usage:**
```go
// JWT Authentication
r.Use(JWTAuth("your-secret-key"))

// API Key Authentication with validator
r.Use(APIKeyAuth(yourValidatorFunc))

// Require specific permission
r.Use(RequirePermission("admin"))
```

### 2. Rate Limiting (`ratelimit.go`)
- **Token Bucket Algorithm**: In-memory rate limiting with configurable rates
- **Redis Support**: Distributed rate limiting using sliding window
- **Per-Client Limiting**: IP-based or API key-based rate limits
- **Graceful Degradation**: Fails open if rate limiting backend fails

**Key Features:**
- Configurable rate limits (requests per second and burst capacity)
- Support for both in-memory and Redis backends
- Per-API-key rate limiting
- Proper HTTP headers (Retry-After, X-RateLimit-*)
- Cleanup mechanism for memory efficiency

**Usage:**
```go
// Basic rate limiting
r.Use(RateLimit())

// Custom configuration
r.Use(RateLimitWithConfig(RateLimitConfig{
    Rate:  1000,
    Burst: 2000,
}))

// Per-API-key rate limiting
r.Use(PerAPIKeyRateLimit())
```

### 3. Metrics Collection (`metrics.go`)
- **Prometheus Integration**: Standard HTTP metrics collection
- **Business Metrics**: PandaFuzz-specific metrics (jobs, crashes, corpus)
- **Request Tracking**: Duration, status codes, payload sizes
- **Custom Metrics**: Gauges and histograms for business logic

**Key Features:**
- Standard HTTP request metrics (duration, count, size)
- Business-specific metrics for fuzzing operations
- Low cardinality through endpoint normalization
- Custom label support
- Performance-optimized with minimal overhead

**Metrics Collected:**
- Request duration, count, and size
- Job creation and completion
- Crash discovery and deduplication
- Corpus synchronization
- Bot registrations
- Coverage percentage
- Fuzzer performance

**Usage:**
```go
// Basic metrics
r.Use(RequestMetrics())

// Record business events
recorder := GetMetricsRecorder(r)
recorder.RecordJobCreated("libfuzzer", "campaign-123")
recorder.RecordCrashFound("afl++", "high")
```

### 4. CORS Handling (`cors.go`)
- **Configurable CORS**: Support for multiple origins, methods, headers
- **Preflight Handling**: Proper OPTIONS request processing
- **Security-First**: Validates origin combinations with credentials
- **Development Mode**: Permissive settings for local development

**Key Features:**
- W3C CORS specification compliance
- Configurable allowed origins, methods, and headers
- Credentials support with security validation
- Preflight request caching
- Development and production configurations

**Usage:**
```go
// Default CORS
r.Use(CORS())

// Production CORS
r.Use(CORSWithConfig(SecureCORS([]string{
    "https://app.pandafuzz.com",
})))

// Development CORS
r.Use(CORSWithConfig(DevelopmentCORS()))
```

### 5. Request Validation (`validation.go`)
- **JSON Schema Validation**: Request body validation using go-playground/validator
- **Query Parameter Validation**: Common parameter validation
- **Content-Type Checking**: Validates request content types
- **Size Limiting**: Prevents oversized requests

**Key Features:**
- JSON syntax and structure validation
- Custom validators for PandaFuzz entities
- Query and path parameter validation
- Request size limits
- Configurable strict mode
- Unknown field handling

**Custom Validators:**
- `fuzzer_type`: Validates fuzzer types (libfuzzer, afl++, honggfuzz)
- `campaign_status`: Validates campaign statuses
- `job_status`: Validates job statuses
- `bot_capability`: Validates bot capabilities

**Usage:**
```go
// Basic validation
r.Use(ValidateRequest())

// Strict validation
r.Use(ValidateRequestWithConfig(ValidationConfig{
    StrictMode:         true,
    AllowUnknownFields: false,
    MaxRequestSize:     10 * 1024 * 1024, // 10MB
}))
```

### 6. Structured Logging (`logging.go`)
- **Request/Response Logging**: Comprehensive HTTP request logging
- **Correlation IDs**: Request tracing across services
- **Sensitive Data Masking**: Automatic masking of passwords, tokens, etc.
- **Performance Metrics**: Request timing and sizing

**Key Features:**
- Structured logging with logrus
- Correlation ID generation and propagation
- Sensitive data masking (passwords, tokens, API keys)
- Request/response body logging (configurable)
- Performance timing information
- Context-aware logging throughout request lifecycle

**Usage:**
```go
// Basic logging
r.Use(RequestLogger())

// Production logging
r.Use(RequestLoggerWithConfig(LoggingConfig{
    LogLevel:        logrus.InfoLevel,
    LogRequestBody:  false,
    LogResponseBody: false,
    MaxBodySize:     1024,
}))

// Use in handlers
LogInfo(r, "Processing job", logrus.Fields{"job_id": jobID})
LogError(r, err, "Failed to create job")
```

### 7. Panic Recovery (`recovery.go`)
- **Panic Handling**: Graceful panic recovery with logging
- **Stack Trace Capture**: Detailed stack traces for debugging
- **Notification System**: Panic alert channels
- **Environment-Aware**: Different behavior for dev/prod

**Key Features:**
- Graceful panic recovery with proper HTTP responses
- Detailed stack trace capture
- Panic notification channels for alerting
- Development vs production modes
- Request context preservation
- RFC 7807 error responses

**Usage:**
```go
// Basic recovery
r.Use(Recovery())

// Development recovery (detailed errors)
r.Use(RecoveryWithConfig(DevelopmentRecovery()))

// Production recovery (minimal errors)
r.Use(RecoveryWithConfig(ProductionRecovery()))

// With alerting
r.Use(AdvancedRecovery(config, alertHandler))
```

### 8. Distributed Tracing (`tracing.go`)
- **OpenTelemetry-Compatible**: Simplified tracing implementation
- **W3C Trace Context**: Standard trace propagation
- **B3 Propagation**: Zipkin-compatible trace headers
- **Span Management**: Request-scoped tracing spans

**Key Features:**
- W3C Trace Context and B3 propagation support
- Request-scoped span creation
- HTTP request attributes
- Distributed trace correlation
- Configurable sampling rates
- Context propagation

**Usage:**
```go
// Basic tracing
r.Use(Tracing())

// Production tracing
r.Use(TracingWithConfig(TracingConfig{
    ServiceName:               "pandafuzz-api",
    Environment:               "production",
    SamplingRate:              0.1,
    EnableW3CTraceContext:     true,
}))

// Use in handlers
span := GetSpan(r)
span.AddTag("job_id", jobID)
AddSpanLog(r, map[string]interface{}{
    "event": "job_created",
    "job_id": jobID,
})
```

## Middleware Stack Order

The recommended order for middleware application:

1. **Recovery** - Catch panics in other middleware
2. **Logging** - Log all requests early
3. **CORS** - Handle preflight requests early
4. **Tracing** - Start distributed traces
5. **Metrics** - Measure everything
6. **Rate Limiting** - Protect against abuse
7. **Validation** - Validate requests before processing
8. **Authentication** - Authenticate and authorize users

## Configuration Examples

### Production Stack
```go
func ProductionAPI(jwtSecret string, logger logrus.FieldLogger) http.Handler {
    r := chi.NewRouter()
    
    r.Use(RecoveryWithConfig(ProductionRecovery()))
    r.Use(RequestLoggerWithConfig(productionLoggingConfig()))
    r.Use(CORSWithConfig(SecureCORS(allowedOrigins)))
    r.Use(TracingWithConfig(productionTracingConfig()))
    r.Use(RequestMetricsWithConfig(productionMetricsConfig()))
    r.Use(RateLimitWithConfig(productionRateLimitConfig()))
    r.Use(ValidateRequestWithConfig(strictValidationConfig()))
    r.Use(JWTAuth(jwtSecret))
    
    return r
}
```

### Development Stack
```go
func DevelopmentAPI(logger logrus.FieldLogger) http.Handler {
    r := chi.NewRouter()
    
    r.Use(RecoveryWithConfig(DevelopmentRecovery()))
    r.Use(RequestLoggerWithConfig(developmentLoggingConfig()))
    r.Use(CORSWithConfig(DevelopmentCORS()))
    r.Use(TracingWithConfig(developmentTracingConfig()))
    r.Use(RequestMetrics())
    r.Use(RateLimitWithConfig(lenientRateLimitConfig()))
    r.Use(ValidateRequestWithConfig(lenientValidationConfig()))
    
    return r
}
```

## Error Handling

All middleware use RFC 7807 error responses via the existing error handling in `pkg/interfaces/api/rest/v1/errors.go`. Errors include:

- Proper HTTP status codes
- Structured error messages
- Request correlation IDs
- Detailed error information (development mode)

## Performance Considerations

- **Memory Efficient**: Minimal allocations in hot paths
- **Thread Safe**: All middleware are goroutine-safe
- **Configurable**: Skip expensive operations via configuration
- **Fail-Safe**: Graceful degradation when backends are unavailable
- **Low Latency**: Optimized for p95 < 100ms target

## Security Features

- **Authentication**: JWT and API key validation
- **Authorization**: Permission-based access control
- **Rate Limiting**: Protect against DoS attacks
- **Input Validation**: Prevent injection attacks
- **CORS**: Control cross-origin access
- **Data Masking**: Prevent sensitive data leakage
- **Secure Defaults**: Production-ready configurations

## Integration with PandaFuzz

The middleware stack is specifically designed for PandaFuzz's needs:

- **Fuzzer-Specific Validation**: Custom validators for fuzzer types
- **Campaign Metrics**: Business metrics for campaigns and jobs
- **Bot Authentication**: API key authentication for bot agents
- **Crash Tracking**: Metrics and logging for crash discovery
- **Corpus Management**: Validation and metrics for corpus operations

## Dependencies

- `github.com/go-chi/chi/v5`: HTTP router
- `github.com/sirupsen/logrus`: Structured logging
- `github.com/prometheus/client_golang`: Metrics collection
- `github.com/go-playground/validator/v10`: Request validation
- `github.com/redis/go-redis/v9`: Redis client (optional)
- `golang.org/x/time/rate`: Rate limiting

All dependencies are already included in the project's go.mod file.