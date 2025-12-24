// Package common provides shared types and interfaces used across PandaFuzz.
//
// This package serves as the central type registry for domain entities that
// are used by multiple packages. It defines the canonical representations
// for jobs, bots, crashes, and other core types that appear in APIs and storage.
//
// # Core Types
//
// The main entity types defined in this package:
//
//   - Job: Fuzzing job with target, configuration, and lifecycle state
//   - Bot: Agent that executes fuzzing jobs
//   - CrashResult: Captured crash from fuzzing execution
//   - CoverageResult: Code coverage metrics from fuzzing
//   - CorpusUpdate: Corpus synchronization data
//
// # Status Types
//
// Typed status values with JSON/database serialization:
//
//	type JobStatus string
//	const (
//	    JobStatusPending   JobStatus = "pending"
//	    JobStatusAssigned  JobStatus = "assigned"
//	    JobStatusStarting  JobStatus = "starting"
//	    JobStatusRunning   JobStatus = "running"
//	    JobStatusCompleted JobStatus = "completed"
//	    JobStatusFailed    JobStatus = "failed"
//	)
//
//	type BotStatus string
//	const (
//	    BotStatusIdle     BotStatus = "idle"
//	    BotStatusBusy     BotStatus = "busy"
//	    BotStatusFailed   BotStatus = "failed"
//	    BotStatusTimedOut BotStatus = "timed_out"
//	)
//
// # Configuration
//
// Master and bot configuration types with validation:
//
//	type MasterConfig struct {
//	    Server     ServerConfig
//	    Database   DatabaseConfig
//	    Storage    StorageConfig
//	    Timeouts   TimeoutConfig
//	    Limits     ResourceLimits
//	    Monitoring MonitoringConfig
//	}
//
//	func (c *MasterConfig) Validate() []error
//
// # Interfaces
//
// Key interfaces for dependency injection:
//
//   - Storage: Abstraction for corpus/crash storage
//   - Database: Key-value and transaction support
//   - CorpusService: Corpus management operations
//
// # Re-exports
//
// For convenience and backward compatibility, this package re-exports types
// from focused packages:
//
//   - Error types from pkg/errors
//   - Retry types from pkg/retry
//   - Database types from pkg/database
//
// New code should import directly from the focused packages when only those
// types are needed.
//
// # Thread Safety
//
// Entity types (Job, Bot, CrashResult) are value types and should be copied
// when sharing between goroutines. The interfaces defined here (Storage,
// Database) have thread safety requirements documented in their respective
// implementation packages.
package common
