# RFC 7807 Error Handling for PandaFuzz API v1

This package provides comprehensive RFC 7807 compliant error handling for the PandaFuzz API v1. It implements Problem Details for HTTP APIs as specified in [RFC 7807](https://tools.ietf.org/html/rfc7807).

## Features

- **RFC 7807 Compliant**: Full compliance with Problem Details for HTTP APIs specification
- **Type Safety**: Structured error types with proper interfaces
- **Content Negotiation**: Supports both JSON and plain text responses based on Accept headers
- **Request Tracing**: Automatic request ID and trace ID inclusion
- **CORS Support**: Proper CORS headers for cross-origin error responses
- **Logging Integration**: Structured logging with appropriate log levels
- **Debug Mode**: Optional stack traces and debug information
- **Extensible**: Custom error types and extensions support
- **Validation Errors**: Special handling for field validation errors
- **Rate Limiting**: Built-in support for rate limit error responses

## Components

### 1. Core Types (`types.go`)

- **`Problem`**: Main RFC 7807 Problem Details structure
- **`FieldError`**: Validation error for specific fields  
- **Problem type constants**: Standard URIs for different error types
- **Validation codes**: Common field validation error codes
- **Constructor functions**: Easy creation of different problem types

### 2. Error Handlers (`handlers.go`)

- **`Handler`**: Main error handler with configurable behavior
- **`HandlerConfig`**: Configuration for logging, debug mode, CORS, etc.
- **HTTP error functions**: Create responses for different status codes
- **Content negotiation**: Automatic JSON/text selection based on Accept header
- **Request context**: Extract request IDs and trace IDs from headers/context

### 3. Custom Error Types (`errors.go`)

- **`PandaFuzzError`**: Interface for RFC 7807 compatible errors
- **Specific error types**: ValidationErrors, ResourceNotFoundError, AuthenticationError, etc.
- **Error utilities**: Wrapping, chaining, and conversion functions
- **Helper functions**: Check if errors are retryable, client errors, server errors

## Basic Usage

### Simple Error Response

```go
import apierrors "github.com/ethpandaops/pandafuzz/pkg/api/v1/errors"

// In your HTTP handler
func (h *Handler) GetCampaign(w http.ResponseWriter, r *http.Request, campaignID string) {
    campaign, err := h.service.GetCampaign(r.Context(), campaignID)
    if err != nil {
        apierrors.ErrorHandler(w, r, err)
        return
    }
    // ... success response
}
```

### Validation Errors

```go
func validateJobRequest(req *JobRequest) error {
    var fieldErrors []apierrors.ValidationErrorDetail
    
    if req.Fuzzer == "" {
        fieldErrors = append(fieldErrors, apierrors.ValidationErrorDetail{
            Field:   "fuzzer",
            Code:    apierrors.CodeRequired,
            Message: "Fuzzer is required",
        })
    }
    
    if req.Timeout < 0 {
        fieldErrors = append(fieldErrors, apierrors.ValidationErrorDetail{
            Field:   "timeout",
            Code:    apierrors.CodeInvalidRange,
            Message: "Timeout must be positive",
            Value:   req.Timeout,
        })
    }
    
    if len(fieldErrors) > 0 {
        return apierrors.NewValidationErrors("Invalid job request", fieldErrors)
    }
    
    return nil
}
```

### Custom Error Types

```go
// Resource not found
err := apierrors.NewResourceNotFoundError("campaign", campaignID)

// Authentication failure
err := apierrors.NewAuthenticationError("Invalid API key")

// Authorization failure
err := apierrors.NewAuthorizationError("campaign", "read")

// Rate limiting
err := apierrors.NewRateLimitExceededError(100, "1h", 3600)

// Business logic error
err := apierrors.NewBusinessLogicError("CAMPAIGN_RUNNING", "Cannot delete running campaign")
```

### Custom Handler Configuration

```go
import "github.com/sirupsen/logrus"

config := &apierrors.HandlerConfig{
    Logger:           logrus.WithField("component", "api"),
    DebugMode:        true,  // Include stack traces
    IncludeRequestID: true,  // Add request IDs
    CORSEnabled:      true,  // CORS headers
    ServiceName:      "pandafuzz-api",
}

handler := apierrors.NewHandler(config)

// Use in your HTTP handler
handler.ErrorHandler(w, r, err)
```

## Example Responses

### Validation Error (400)

```json
{
  "type": "https://pandafuzz.io/problems/validation-error",
  "title": "Validation Error",
  "status": 400,
  "detail": "One or more validation errors occurred",
  "instance": "req-550e8400-e29b-41d4-a716-446655440001",
  "timestamp": "2023-12-01T10:30:00Z",
  "errors": [
    {
      "field": "fuzzer",
      "code": "REQUIRED",
      "message": "Fuzzer is required"
    },
    {
      "field": "timeout",
      "code": "INVALID_RANGE", 
      "message": "Timeout must be positive",
      "value": -1
    }
  ]
}
```

### Not Found Error (404)

```json
{
  "type": "https://pandafuzz.io/problems/not-found",
  "title": "Not Found",
  "status": 404,
  "detail": "The requested campaign was not found",
  "instance": "req-550e8400-e29b-41d4-a716-446655440001",
  "timestamp": "2023-12-01T10:30:00Z",
  "identifier": "550e8400-e29b-41d4-a716-446655440001"
}
```

### Rate Limit Error (429)

```json
{
  "type": "https://pandafuzz.io/problems/rate-limit",
  "title": "Rate Limit Exceeded", 
  "status": 429,
  "detail": "Rate limit exceeded. Please try again later",
  "instance": "req-550e8400-e29b-41d4-a716-446655440001",
  "timestamp": "2023-12-01T10:30:00Z",
  "limit": 100,
  "window": "1h",
  "retry_after": 3600
}
```

## Integration with Generated Types

This package is designed to work seamlessly with the generated OpenAPI types in `pkg/api/v1/generated/`. The existing `ProblemDetails` type from the generated code is compatible with this implementation.

## Error Type Detection

The package includes automatic error type detection based on error messages and types:

- Validation errors (contains "validation")
- Not found errors (contains "not found" or "does not exist") 
- Authentication errors (contains "unauthorized" or "authentication")
- Forbidden errors (contains "forbidden" or "access denied")
- Conflict errors (contains "conflict" or "already exists")
- Rate limit errors (contains "rate limit" or "too many requests")

## Testing

The package includes comprehensive tests demonstrating all functionality:

```bash
go test ./pkg/api/v1/errors -v
```

## Best Practices

1. **Use specific error types** rather than generic errors when possible
2. **Include request context** (request IDs, trace IDs) for debugging
3. **Don't expose sensitive information** in error details
4. **Log errors at appropriate levels** (warn for client errors, error for server errors)
5. **Use validation errors** for input validation with specific field details
6. **Implement PandaFuzzError interface** for custom domain errors
7. **Enable debug mode only in development** to avoid leaking stack traces

## Error Type Hierarchy

```
error (Go interface)
├── PandaFuzzError (RFC 7807 compatible)
│   ├── ValidationErrors
│   ├── ResourceNotFoundError
│   ├── AuthenticationError
│   ├── AuthorizationError
│   ├── ConflictError
│   ├── RateLimitExceededError
│   ├── BusinessLogicError
│   ├── ExternalServiceError
│   ├── TimeoutError
│   └── PayloadTooLargeError
└── Standard Go errors (converted automatically)
```

This implementation provides a robust, standards-compliant foundation for error handling across the PandaFuzz API, ensuring consistent, debuggable, and user-friendly error responses.