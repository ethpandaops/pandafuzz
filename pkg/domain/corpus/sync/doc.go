// Package sync provides corpus synchronization services for the pandafuzz fuzzing framework.
//
// The sync package implements domain-driven design principles to handle synchronization
// of corpus collections between different repositories. It supports various synchronization
// strategies, conflict resolution, and incremental updates.
//
// # Core Components
//
// Service: The main synchronization service that orchestrates sync operations between
// corpus repositories. It supports concurrent operations, transaction management, and
// event emission for monitoring sync progress.
//
// Differ: Calculates differences between corpus collections, identifying added, removed,
// and modified entries. It supports various comparison options including metadata, tags,
// and coverage information.
//
// Strategies: Multiple synchronization strategies are provided:
//   - Full sync: Complete synchronization of all entries
//   - Incremental sync: Only sync entries modified since last sync
//   - Merge sync: Intelligent merging of conflicting entries
//   - Parallel sync: Concurrent synchronization for improved performance
//
// # Usage Example
//
//	// Create sync service with event emitter
//	eventEmitter := NewEventEmitter()
//	syncService := sync.NewSyncService(eventEmitter)
//
//	// Configure sync options
//	options := sync.SyncOptions{
//		Direction:          sync.SyncDirectionBidirectional,
//		ConflictResolution: sync.ConflictResolutionHighestCoverage,
//		BatchSize:          100,
//		MaxRetries:         3,
//		RetryDelay:         time.Second,
//	}
//
//	// Perform synchronization
//	result, err := syncService.SyncCollections(
//		ctx,
//		sourceRepo,
//		targetRepo,
//		[]string{"main", "experimental"},
//		options,
//	)
//
// # Conflict Resolution
//
// The package provides several conflict resolution strategies:
//   - Latest: Keep the most recently modified entry
//   - Highest Coverage: Keep the entry with highest code coverage
//   - Source/Target: Always prefer source or target entry
//   - Merge: Intelligently merge entries, combining metadata and tags
//
// # Events
//
// The sync service emits events throughout the synchronization process:
//   - SyncStarted: Emitted when sync begins
//   - SyncProgress: Regular progress updates
//   - SyncConflict: When conflicts are detected
//   - SyncCompleted: When sync finishes successfully
//   - SyncFailed: When sync encounters errors
//
// # Thread Safety
//
// All components in this package are designed to be thread-safe and support
// concurrent operations. The service manages active sync operations and enforces
// concurrency limits.
package sync
