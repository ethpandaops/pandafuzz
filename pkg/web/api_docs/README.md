# PandaFuzz API Documentation

Interactive API documentation for PandaFuzz using OpenAPI 3.0 specification and Swagger UI.

## Features

- **Interactive API Explorer**: Browse and test API endpoints directly from the documentation
- **Code Examples**: Auto-generated code examples in Python, Go, and cURL
- **API Key Testing**: Test endpoints with API key authentication
- **Live API Testing**: Try out API calls directly from the browser
- **OpenAPI 3.0**: Full OpenAPI specification with request/response schemas

## Running the Documentation Server

### Using the CLI

```bash
# Build the documentation server
go build -o api-docs ./cmd/api-docs

# Run with default settings (port 8081)
./api-docs

# Run with custom port
./api-docs -port 8080

# Enable API testing with a test key
./api-docs -api-key test-key-123

# Specify custom OpenAPI spec location
./api-docs -openapi ./api_v3/openapi.yaml
```

### Accessing the Documentation

Once running, access the documentation at:
- http://localhost:8081/api/docs

## Integration with Master Server

The API documentation can be integrated into the master server by:

1. Adding a route handler in the master server:
```go
// In master server setup
router.PathPrefix("/api/docs").Handler(apidocs.Handler(config))
```

2. Or running as a standalone service alongside the master

## Code Examples

The documentation includes code examples for:

- **Python**: Using requests library
- **Go**: Using net/http package
- **cURL**: Command-line examples

Each example demonstrates:
- Authentication with API keys
- Creating campaigns and jobs
- Retrieving statistics and analytics
- Error handling

## Customization

The documentation can be customized by:

1. Modifying the OpenAPI specification (`pkg/master/api_v3/openapi.yaml`)
2. Updating the HTML template (`templates/index.html`)
3. Adding new code examples in `server.go`

## Development

To update Swagger UI assets:

1. Download the latest Swagger UI distribution
2. Copy the necessary files to `swagger-ui/` directory
3. Update the embed directive in `server.go`

Currently, the documentation uses CDN-hosted Swagger UI for simplicity.