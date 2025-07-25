// Package v2 implements the enhanced V2 REST API for Pandafuzz.
//
// The V2 API provides improved functionality over V1 including:
//   - Enhanced error responses with detailed error codes
//   - Comprehensive pagination support for all list endpoints
//   - Real-time streaming endpoints for metrics and logs
//   - Batch operations for efficient bulk processing
//   - OpenAPI 3.0 documentation with request/response examples
//   - Advanced filtering and sorting capabilities
//   - WebSocket support for real-time updates
//
// Architecture:
//
// The package follows a clean architecture pattern with:
//   - handlers/ - HTTP request handlers organized by domain
//   - middleware.go - Enhanced middleware for auth, rate limiting, etc.
//   - router.go - Route configuration and setup
//   - types.go - Request/response DTOs
//   - validators.go - Input validation logic
//
// All handlers use the application layer services (pkg/service) for business logic,
// ensuring proper separation of concerns and testability.
//
// Example usage:
//
//	router := mux.NewRouter()
//	v2Router := router.PathPrefix("/api/v2").Subrouter()
//
//	v2API := v2.New(services, logger, config)
//	v2API.RegisterRoutes(v2Router)
//
// OpenAPI Documentation:
//
// All endpoints include OpenAPI annotations for automatic documentation generation.
// The API specification can be accessed at /api/v2/openapi.json
package v2
