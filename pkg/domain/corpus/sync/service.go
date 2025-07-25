package sync

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
)

// SyncDirection represents the direction of synchronization
type SyncDirection string

const (
	// SyncDirectionPush pushes local changes to remote
	SyncDirectionPush SyncDirection = "push"
	// SyncDirectionPull pulls remote changes to local
	SyncDirectionPull SyncDirection = "pull"
	// SyncDirectionBidirectional syncs in both directions
	SyncDirectionBidirectional SyncDirection = "bidirectional"
)

// SyncOptions configures the synchronization behavior
type SyncOptions struct {
	Direction          SyncDirection
	ConflictResolution ConflictResolution
	BatchSize          int
	MaxRetries         int
	RetryDelay         time.Duration
	DryRun             bool
	IncludeMetadata    bool
	IncludeTags        bool
}

// ConflictResolution defines how to handle conflicts during sync
type ConflictResolution string

const (
	// ConflictResolutionLatest keeps the most recent entry
	ConflictResolutionLatest ConflictResolution = "latest"
	// ConflictResolutionHighestCoverage keeps entry with highest coverage
	ConflictResolutionHighestCoverage ConflictResolution = "highest_coverage"
	// ConflictResolutionSource keeps the source entry
	ConflictResolutionSource ConflictResolution = "source"
	// ConflictResolutionTarget keeps the target entry
	ConflictResolutionTarget ConflictResolution = "target"
	// ConflictResolutionMerge attempts to merge entries
	ConflictResolutionMerge ConflictResolution = "merge"
)

// SyncResult contains the result of a synchronization operation
type SyncResult struct {
	StartTime         time.Time
	EndTime           time.Time
	Duration          time.Duration
	EntriesAdded      int
	EntriesUpdated    int
	EntriesDeleted    int
	EntriesSkipped    int
	Conflicts         []SyncConflict
	Errors            []error
	Success           bool
	DryRun            bool
	CollectionsSynced []string
}

// SyncConflict represents a conflict during synchronization
type SyncConflict struct {
	EntryID     string
	SourceEntry *types.CorpusEntry
	TargetEntry *types.CorpusEntry
	Resolution  ConflictResolution
	Resolved    bool
	Error       error
}

// SyncService handles corpus synchronization between repositories
type SyncService struct {
	mu               sync.RWMutex
	eventEmitter     EventEmitter
	differ           *Differ
	activeSyncs      map[string]*syncOperation
	maxConcurrent    int
	defaultBatchSize int
}

// syncOperation tracks an active synchronization
type syncOperation struct {
	id        string
	startTime time.Time
	source    repository.CorpusTransactionRepository
	target    repository.CorpusTransactionRepository
	options   SyncOptions
	result    *SyncResult
	cancel    context.CancelFunc
	done      chan struct{}
}

// EventEmitter defines the interface for emitting sync events
type EventEmitter interface {
	Emit(event Event)
}

// NewSyncService creates a new synchronization service
func NewSyncService(eventEmitter EventEmitter) *SyncService {
	return &SyncService{
		eventEmitter:     eventEmitter,
		differ:           NewDiffer(),
		activeSyncs:      make(map[string]*syncOperation),
		maxConcurrent:    5,
		defaultBatchSize: 100,
	}
}

// SyncCollections synchronizes corpus collections between two repositories
func (s *SyncService) SyncCollections(
	ctx context.Context,
	source repository.CorpusTransactionRepository,
	target repository.CorpusTransactionRepository,
	collectionNames []string,
	options SyncOptions,
) (*SyncResult, error) {
	// Validate inputs
	if source == nil || target == nil {
		return nil, errors.New("source and target repositories cannot be nil")
	}

	// Apply defaults
	if options.BatchSize <= 0 {
		options.BatchSize = s.defaultBatchSize
	}
	if options.MaxRetries <= 0 {
		options.MaxRetries = 3
	}
	if options.RetryDelay <= 0 {
		options.RetryDelay = time.Second
	}

	// Create sync operation
	syncOp := &syncOperation{
		id:        generateSyncID(),
		startTime: time.Now(),
		source:    source,
		target:    target,
		options:   options,
		result: &SyncResult{
			StartTime: time.Now(),
			DryRun:    options.DryRun,
			Conflicts: make([]SyncConflict, 0),
			Errors:    make([]error, 0),
		},
		done: make(chan struct{}),
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	syncOp.cancel = cancel

	// Track active sync
	s.mu.Lock()
	if len(s.activeSyncs) >= s.maxConcurrent {
		s.mu.Unlock()
		return nil, errors.New("maximum concurrent syncs reached")
	}
	s.activeSyncs[syncOp.id] = syncOp
	s.mu.Unlock()

	// Emit start event
	s.emitEvent(NewSyncStartedEvent(syncOp.id, len(collectionNames)))

	// Perform synchronization
	go s.performSync(ctx, syncOp, collectionNames)

	// Wait for completion
	select {
	case <-syncOp.done:
		// Normal completion
	case <-ctx.Done():
		// Context cancelled
		syncOp.result.Success = false
		syncOp.result.Errors = append(syncOp.result.Errors, ctx.Err())
	}

	// Clean up
	s.mu.Lock()
	delete(s.activeSyncs, syncOp.id)
	s.mu.Unlock()

	// Calculate duration and emit completion event
	syncOp.result.EndTime = time.Now()
	syncOp.result.Duration = syncOp.result.EndTime.Sub(syncOp.result.StartTime)

	if syncOp.result.Success {
		s.emitEvent(NewSyncCompletedEvent(syncOp.id, syncOp.result))
	} else {
		s.emitEvent(NewSyncFailedEvent(syncOp.id, syncOp.result.Errors))
	}

	return syncOp.result, nil
}

// performSync executes the synchronization operation
func (s *SyncService) performSync(ctx context.Context, op *syncOperation, collectionNames []string) {
	defer close(op.done)

	// If no specific collections provided, sync all
	if len(collectionNames) == 0 {
		collections, err := op.source.ListCollections(ctx)
		if err != nil {
			op.result.Errors = append(op.result.Errors, fmt.Errorf("failed to list collections: %w", err))
			return
		}
		for _, col := range collections {
			collectionNames = append(collectionNames, col.Name())
		}
	}

	// Sync each collection
	for _, colName := range collectionNames {
		if err := ctx.Err(); err != nil {
			op.result.Errors = append(op.result.Errors, err)
			break
		}

		if err := s.syncCollection(ctx, op, colName); err != nil {
			op.result.Errors = append(op.result.Errors, fmt.Errorf("failed to sync collection %s: %w", colName, err))
			// Continue with other collections
		} else {
			op.result.CollectionsSynced = append(op.result.CollectionsSynced, colName)
		}

		// Emit progress event
		s.emitEvent(NewSyncProgressEvent(op.id, colName, op.result))
	}

	// Set success flag if no errors
	op.result.Success = len(op.result.Errors) == 0
}

// syncCollection synchronizes a single collection
func (s *SyncService) syncCollection(ctx context.Context, op *syncOperation, collectionName string) error {
	// Get or create collection in target
	targetCol, err := op.target.FindCollectionByName(ctx, collectionName)
	if err != nil {
		// Create collection if it doesn't exist
		sourceCol, err := op.source.FindCollectionByName(ctx, collectionName)
		if err != nil {
			return fmt.Errorf("failed to find source collection: %w", err)
		}

		targetCol = &types.CorpusCollection{}
		*targetCol = *sourceCol // Copy collection metadata

		if !op.options.DryRun {
			if err := op.target.CreateCollection(ctx, targetCol); err != nil {
				return fmt.Errorf("failed to create target collection: %w", err)
			}
		}
	}

	// Get entries from both collections
	sourceEntries, err := op.source.GetCollectionEntries(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("failed to get source entries: %w", err)
	}

	targetEntries, err := op.target.GetCollectionEntries(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("failed to get target entries: %w", err)
	}

	// Calculate differences
	diff := s.differ.CalculateDiff(sourceEntries, targetEntries)

	// Process based on sync direction
	switch op.options.Direction {
	case SyncDirectionPush:
		return s.processPush(ctx, op, collectionName, diff)
	case SyncDirectionPull:
		return s.processPull(ctx, op, collectionName, diff)
	case SyncDirectionBidirectional:
		if err := s.processPush(ctx, op, collectionName, diff); err != nil {
			return err
		}
		return s.processPull(ctx, op, collectionName, diff)
	default:
		return fmt.Errorf("invalid sync direction: %s", op.options.Direction)
	}
}

// processPush handles pushing changes from source to target
func (s *SyncService) processPush(ctx context.Context, op *syncOperation, collectionName string, diff *CollectionDiff) error {
	// Start transaction if not dry run
	var tx repository.CorpusTransaction
	if !op.options.DryRun {
		var err error
		tx, err = op.target.BeginTransaction(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer func() {
			if err != nil {
				tx.Rollback()
			}
		}()
	}

	// Process additions
	for _, entry := range diff.Added {
		if err := ctx.Err(); err != nil {
			return err
		}

		op.result.EntriesAdded++
		if !op.options.DryRun {
			if err := s.addEntry(ctx, tx, op.target, entry, collectionName); err != nil {
				op.result.Errors = append(op.result.Errors, fmt.Errorf("failed to add entry %s: %w", entry.ID, err))
				op.result.EntriesAdded--
			}
		}
	}

	// Process modifications
	for _, mod := range diff.Modified {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Handle conflict
		conflict := SyncConflict{
			EntryID:     mod.Source.ID,
			SourceEntry: mod.Source,
			TargetEntry: mod.Target,
			Resolution:  op.options.ConflictResolution,
		}

		resolved := s.resolveConflict(&conflict, op.options.ConflictResolution)
		op.result.Conflicts = append(op.result.Conflicts, conflict)

		if resolved && !op.options.DryRun {
			if err := s.updateEntry(ctx, tx, op.target, conflict.SourceEntry); err != nil {
				op.result.Errors = append(op.result.Errors, fmt.Errorf("failed to update entry %s: %w", mod.Source.ID, err))
			} else {
				op.result.EntriesUpdated++
			}
		} else if !resolved {
			op.result.EntriesSkipped++
		}
	}

	// Process deletions (only in bidirectional mode)
	if op.options.Direction == SyncDirectionBidirectional {
		for _, entry := range diff.Removed {
			if err := ctx.Err(); err != nil {
				return err
			}

			op.result.EntriesDeleted++
			if !op.options.DryRun {
				if err := s.removeEntry(ctx, tx, op.target, entry, collectionName); err != nil {
					op.result.Errors = append(op.result.Errors, fmt.Errorf("failed to remove entry %s: %w", entry.ID, err))
					op.result.EntriesDeleted--
				}
			}
		}
	}

	// Commit transaction
	if !op.options.DryRun && tx != nil {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	return nil
}

// processPull handles pulling changes from target to source
func (s *SyncService) processPull(ctx context.Context, op *syncOperation, collectionName string, diff *CollectionDiff) error {
	// For pull, we reverse the diff perspective
	reverseDiff := &CollectionDiff{
		Added:    diff.Removed,
		Removed:  diff.Added,
		Modified: make([]ModifiedEntry, len(diff.Modified)),
	}

	// Reverse modified entries
	for i, mod := range diff.Modified {
		reverseDiff.Modified[i] = ModifiedEntry{
			Source: mod.Target,
			Target: mod.Source,
		}
	}

	// Use push logic with reversed diff
	return s.processPush(ctx, op, collectionName, reverseDiff)
}

// resolveConflict resolves a sync conflict based on the resolution strategy
func (s *SyncService) resolveConflict(conflict *SyncConflict, resolution ConflictResolution) bool {
	switch resolution {
	case ConflictResolutionLatest:
		if conflict.SourceEntry.CreatedAt.After(conflict.TargetEntry.CreatedAt) {
			conflict.Resolved = true
			return true
		}
		return false

	case ConflictResolutionHighestCoverage:
		if conflict.SourceEntry.Coverage.CoverageScore > conflict.TargetEntry.Coverage.CoverageScore {
			conflict.Resolved = true
			return true
		}
		return false

	case ConflictResolutionSource:
		conflict.Resolved = true
		return true

	case ConflictResolutionTarget:
		return false

	case ConflictResolutionMerge:
		// Merge metadata and tags
		merged := *conflict.SourceEntry

		// Merge tags
		tagSet := make(map[string]bool)
		for _, tag := range conflict.SourceEntry.Tags {
			tagSet[tag] = true
		}
		for _, tag := range conflict.TargetEntry.Tags {
			tagSet[tag] = true
		}
		merged.Tags = make([]string, 0, len(tagSet))
		for tag := range tagSet {
			merged.Tags = append(merged.Tags, tag)
		}

		// Merge metadata
		if merged.Metadata == nil {
			merged.Metadata = make(map[string]string)
		}
		for k, v := range conflict.TargetEntry.Metadata {
			if _, exists := merged.Metadata[k]; !exists {
				merged.Metadata[k] = v
			}
		}

		// Use highest coverage
		if conflict.TargetEntry.Coverage.CoverageScore > merged.Coverage.CoverageScore {
			merged.Coverage = conflict.TargetEntry.Coverage
		}

		conflict.SourceEntry = &merged
		conflict.Resolved = true
		return true

	default:
		return false
	}
}

// Helper methods for entry operations

func (s *SyncService) addEntry(ctx context.Context, tx repository.CorpusTransaction, repo repository.CorpusTransactionRepository, entry *types.CorpusEntry, collectionName string) error {
	if tx != nil {
		if err := tx.CreateEntryTx(ctx, entry); err != nil {
			return err
		}
		return tx.AddEntryToCollectionTx(ctx, collectionName, entry.ID)
	}

	if err := repo.Create(ctx, entry); err != nil {
		return err
	}
	return repo.AddEntryToCollection(ctx, collectionName, entry.ID)
}

func (s *SyncService) updateEntry(ctx context.Context, tx repository.CorpusTransaction, repo repository.CorpusTransactionRepository, entry *types.CorpusEntry) error {
	if tx != nil {
		return tx.UpdateEntryTx(ctx, entry)
	}
	return repo.Update(ctx, entry)
}

func (s *SyncService) removeEntry(ctx context.Context, tx repository.CorpusTransaction, repo repository.CorpusTransactionRepository, entry *types.CorpusEntry, collectionName string) error {
	if tx != nil {
		if err := tx.RemoveEntryFromCollectionTx(ctx, collectionName, entry.ID); err != nil {
			return err
		}
		return tx.DeleteEntryTx(ctx, entry.ID)
	}

	if err := repo.RemoveEntryFromCollection(ctx, collectionName, entry.ID); err != nil {
		return err
	}
	return repo.Delete(ctx, entry.ID)
}

// GetActiveSync returns information about an active sync operation
func (s *SyncService) GetActiveSync(syncID string) (*SyncResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if op, exists := s.activeSyncs[syncID]; exists {
		return op.result, true
	}
	return nil, false
}

// ListActiveSyncs returns all active sync operations
func (s *SyncService) ListActiveSyncs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.activeSyncs))
	for id := range s.activeSyncs {
		ids = append(ids, id)
	}
	return ids
}

// CancelSync cancels an active sync operation
func (s *SyncService) CancelSync(syncID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if op, exists := s.activeSyncs[syncID]; exists {
		op.cancel()
		return nil
	}
	return errors.New("sync operation not found")
}

// emitEvent emits a sync event
func (s *SyncService) emitEvent(event Event) {
	if s.eventEmitter != nil {
		s.eventEmitter.Emit(event)
	}
}

// generateSyncID generates a unique sync operation ID
func generateSyncID() string {
	return fmt.Sprintf("sync-%d", time.Now().UnixNano())
}
