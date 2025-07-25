// Package v1 provides the REST API v1 implementation for PandaFuzz.
//
// This package implements the HTTP handlers for the v1 API endpoints,
// following a clean architecture approach where handlers are thin controllers
// that delegate business logic to application services.
//
// # Structure
//
// The package is organized into sub-packages by domain:
//   - handlers/campaign: Campaign management endpoints
//   - handlers/bot: Bot registration and management endpoints
//   - handlers/crash: Crash result and analysis endpoints
//   - handlers/corpus: Corpus management and synchronization endpoints
//
// # Handler Pattern
//
// All handlers follow a consistent pattern:
//  1. Parse and validate request parameters
//  2. Call appropriate application service methods
//  3. Transform results to API response format
//  4. Handle errors consistently
//
// # Dependency Injection
//
// Handlers receive dependencies through their constructors, making them
// testable and following the dependency inversion principle.
//
// # Backward Compatibility
//
// This implementation maintains backward compatibility with the existing
// v1 API while refactoring the internal structure for better maintainability.
package v1
