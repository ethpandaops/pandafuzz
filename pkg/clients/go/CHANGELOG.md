# Changelog

All notable changes to the PandaFuzz Go Client SDK will be documented in this file.

## [1.0.0] - 2024-01-15

### Added
- **Generated Client**: Complete Go client auto-generated from OpenAPI specification using oapi-codegen v2
- **SimpleClient**: High-level wrapper providing convenient methods for common operations
- **Authentication Support**: API key and Bearer token authentication methods
- **Context Integration**: Full context.Context support for timeouts and cancellation
- **Type Safety**: Strongly-typed request/response structures from OpenAPI spec
- **HTTP Client Configuration**: Customizable HTTP client with connection pooling
- **Structured Logging**: Integration with logrus for structured logging
- **Error Handling**: Comprehensive error handling patterns
- **UUID Support**: Proper handling of UUID parameters for bot and campaign IDs
- **Testing**: Complete test suite with mock server testing
- **Documentation**: Comprehensive README with usage examples
- **Examples**: Working example programs demonstrating client usage
- **Build System**: Makefile with common development tasks

### Core Features
- Health check endpoint support
- Bot management (create, get, list, update, delete)
- Campaign management (create, get, list, update, delete)  
- Job management (create, get, list, update, delete)
- Batch operations support
- Analytics and metrics endpoints
- Corpus management
- Crash analysis
- Event streaming preparation (SSE structure in place)

### Development Tools
- **oapi-codegen**: Automatic client generation from OpenAPI spec
- **Make targets**: build, test, lint, format, examples
- **Go modules**: Proper dependency management
- **CI ready**: Test suite suitable for continuous integration

### Generated Types
- Complete type definitions for all API entities
- Request/response parameter structures  
- Enum types for statuses and configurations
- UUID parameter types for entity IDs
- Pagination and filtering parameter support

### Client Architecture
- `SimpleClient`: High-level wrapper with convenience methods
- `generated.Client`: Direct access to all API endpoints
- Configurable HTTP client with timeouts and pooling
- Authentication middleware for API keys and Bearer tokens
- Structured logging with configurable levels
- Context-aware operations with proper cancellation

### Testing
- Unit tests for client creation and configuration
- Mock server tests for API interactions
- Context cancellation and timeout testing
- Error handling verification
- HTTP response validation

### Documentation
- Comprehensive README with examples
- API reference for all client methods
- Authentication setup guide
- Error handling patterns
- Context usage best practices
- Type reference documentation