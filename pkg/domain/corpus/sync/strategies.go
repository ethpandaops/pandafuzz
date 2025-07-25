package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
)

// SyncStrategy defines the interface for synchronization strategies
type SyncStrategy interface {
	// Name returns the strategy name
	Name() string

	// Execute performs the synchronization using this strategy
	Execute(ctx context.Context, source, target []*types.CorpusEntry, options SyncOptions) (*SyncResult, error)

	// SupportsIncremental indicates if the strategy supports incremental sync
	SupportsIncremental() bool
}

// IncrementalSyncStrategy performs incremental synchronization
type IncrementalSyncStrategy struct {
	mu             sync.RWMutex
	lastSyncTime   map[string]time.Time
	syncCheckpoint map[string]string
}

// NewIncrementalSyncStrategy creates a new incremental sync strategy
func NewIncrementalSyncStrategy() *IncrementalSyncStrategy {
	return &IncrementalSyncStrategy{
		lastSyncTime:   make(map[string]time.Time),
		syncCheckpoint: make(map[string]string),
	}
}

// Name returns the strategy name
func (s *IncrementalSyncStrategy) Name() string {
	return "incremental"
}

// Execute performs incremental synchronization
func (s *IncrementalSyncStrategy) Execute(ctx context.Context, source, target []*types.CorpusEntry, options SyncOptions) (*SyncResult, error) {
	result := &SyncResult{
		StartTime: time.Now(),
		DryRun:    options.DryRun,
		Conflicts: make([]SyncConflict, 0),
		Errors:    make([]error, 0),
	}

	// Get last sync time for this collection
	s.mu.RLock()
	lastSync, hasLastSync := s.lastSyncTime[s.getCollectionKey(source)]
	s.mu.RUnlock()

	// Filter entries based on last sync time
	var entriesToSync []*types.CorpusEntry
	if hasLastSync {
		for _, entry := range source {
			if entry.CreatedAt.After(lastSync) {
				entriesToSync = append(entriesToSync, entry)
			}
		}
	} else {
		// First sync - sync all entries
		entriesToSync = source
	}

	// Process entries in batches
	for i := 0; i < len(entriesToSync); i += options.BatchSize {
		if err := ctx.Err(); err != nil {
			result.Errors = append(result.Errors, err)
			break
		}

		end := i + options.BatchSize
		if end > len(entriesToSync) {
			end = len(entriesToSync)
		}

		batch := entriesToSync[i:end]

		// Process batch
		for _, entry := range batch {
			// Check if entry exists in target
			exists := false
			for _, targetEntry := range target {
				if targetEntry.ID == entry.ID {
					exists = true
					// Check for conflicts
					if s.hasConflict(entry, targetEntry) {
						conflict := SyncConflict{
							EntryID:     entry.ID,
							SourceEntry: entry,
							TargetEntry: targetEntry,
							Resolution:  options.ConflictResolution,
						}
						result.Conflicts = append(result.Conflicts, conflict)
					} else {
						result.EntriesSkipped++
					}
					break
				}
			}

			if !exists {
				result.EntriesAdded++
			}
		}
	}

	// Update last sync time
	if len(result.Errors) == 0 && !options.DryRun {
		s.mu.Lock()
		s.lastSyncTime[s.getCollectionKey(source)] = time.Now()
		s.mu.Unlock()
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Success = len(result.Errors) == 0

	return result, nil
}

// SupportsIncremental indicates this strategy supports incremental sync
func (s *IncrementalSyncStrategy) SupportsIncremental() bool {
	return true
}

// hasConflict checks if two entries have conflicts
func (s *IncrementalSyncStrategy) hasConflict(source, target *types.CorpusEntry) bool {
	return source.Hash != target.Hash || source.Coverage.CoverageScore != target.Coverage.CoverageScore
}

// getCollectionKey generates a key for tracking sync state
func (s *IncrementalSyncStrategy) getCollectionKey(entries []*types.CorpusEntry) string {
	if len(entries) > 0 {
		// Use first entry's metadata to identify collection
		if collName, ok := entries[0].Metadata["collection"]; ok {
			return collName
		}
	}
	return "default"
}

// FullSyncStrategy performs full synchronization
type FullSyncStrategy struct {
	differ *Differ
}

// NewFullSyncStrategy creates a new full sync strategy
func NewFullSyncStrategy() *FullSyncStrategy {
	return &FullSyncStrategy{
		differ: NewDiffer(),
	}
}

// Name returns the strategy name
func (s *FullSyncStrategy) Name() string {
	return "full"
}

// Execute performs full synchronization
func (s *FullSyncStrategy) Execute(ctx context.Context, source, target []*types.CorpusEntry, options SyncOptions) (*SyncResult, error) {
	result := &SyncResult{
		StartTime: time.Now(),
		DryRun:    options.DryRun,
		Conflicts: make([]SyncConflict, 0),
		Errors:    make([]error, 0),
	}

	// Calculate full diff
	diff := s.differ.CalculateDiff(source, target)

	// Process additions
	result.EntriesAdded = len(diff.Added)

	// Process modifications
	for _, mod := range diff.Modified {
		conflict := SyncConflict{
			EntryID:     mod.Source.ID,
			SourceEntry: mod.Source,
			TargetEntry: mod.Target,
			Resolution:  options.ConflictResolution,
		}
		result.Conflicts = append(result.Conflicts, conflict)
		result.EntriesUpdated++
	}

	// Process deletions (if bidirectional)
	if options.Direction == SyncDirectionBidirectional {
		result.EntriesDeleted = len(diff.Removed)
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Success = true

	return result, nil
}

// SupportsIncremental indicates this strategy does not support incremental sync
func (s *FullSyncStrategy) SupportsIncremental() bool {
	return false
}

// MergeSyncStrategy performs merge-based synchronization
type MergeSyncStrategy struct {
	differ        *Differ
	mergeFunction MergeFunction
}

// MergeFunction defines how to merge two entries
type MergeFunction func(source, target *types.CorpusEntry) (*types.CorpusEntry, error)

// NewMergeSyncStrategy creates a new merge sync strategy
func NewMergeSyncStrategy(mergeFunc MergeFunction) *MergeSyncStrategy {
	if mergeFunc == nil {
		mergeFunc = DefaultMergeFunction
	}
	return &MergeSyncStrategy{
		differ:        NewDiffer(),
		mergeFunction: mergeFunc,
	}
}

// Name returns the strategy name
func (s *MergeSyncStrategy) Name() string {
	return "merge"
}

// Execute performs merge-based synchronization
func (s *MergeSyncStrategy) Execute(ctx context.Context, source, target []*types.CorpusEntry, options SyncOptions) (*SyncResult, error) {
	result := &SyncResult{
		StartTime: time.Now(),
		DryRun:    options.DryRun,
		Conflicts: make([]SyncConflict, 0),
		Errors:    make([]error, 0),
	}

	// Calculate diff
	diff := s.differ.CalculateDiff(source, target)

	// Process additions
	result.EntriesAdded = len(diff.Added)

	// Process modifications with merge
	for _, mod := range diff.Modified {
		merged, err := s.mergeFunction(mod.Source, mod.Target)
		if err != nil {
			// Merge failed - treat as conflict
			conflict := SyncConflict{
				EntryID:     mod.Source.ID,
				SourceEntry: mod.Source,
				TargetEntry: mod.Target,
				Resolution:  options.ConflictResolution,
				Error:       err,
			}
			result.Conflicts = append(result.Conflicts, conflict)
		} else {
			// Merge succeeded
			result.EntriesUpdated++
			// In a real implementation, we would apply the merged entry
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Success = len(result.Errors) == 0

	return result, nil
}

// SupportsIncremental indicates this strategy supports incremental sync
func (s *MergeSyncStrategy) SupportsIncremental() bool {
	return true
}

// DefaultMergeFunction provides a default merge implementation
func DefaultMergeFunction(source, target *types.CorpusEntry) (*types.CorpusEntry, error) {
	// Start with a copy of the source
	merged := *source

	// Use highest coverage
	if target.Coverage.CoverageScore > source.Coverage.CoverageScore {
		merged.Coverage = target.Coverage
	}

	// Merge tags (union)
	tagSet := make(map[string]bool)
	for _, tag := range source.Tags {
		tagSet[tag] = true
	}
	for _, tag := range target.Tags {
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
	for k, v := range target.Metadata {
		if _, exists := merged.Metadata[k]; !exists {
			merged.Metadata[k] = v
		}
	}

	// Use higher execution count
	if target.ExecutionCount > source.ExecutionCount {
		merged.ExecutionCount = target.ExecutionCount
		merged.LastExecutedAt = target.LastExecutedAt
	}

	return &merged, nil
}

// ParallelSyncStrategy performs parallel synchronization
type ParallelSyncStrategy struct {
	baseStrategy SyncStrategy
	workers      int
	chunkSize    int
}

// NewParallelSyncStrategy creates a new parallel sync strategy
func NewParallelSyncStrategy(baseStrategy SyncStrategy, workers int) *ParallelSyncStrategy {
	if workers <= 0 {
		workers = 4
	}
	return &ParallelSyncStrategy{
		baseStrategy: baseStrategy,
		workers:      workers,
		chunkSize:    100,
	}
}

// Name returns the strategy name
func (s *ParallelSyncStrategy) Name() string {
	return fmt.Sprintf("parallel-%s", s.baseStrategy.Name())
}

// Execute performs parallel synchronization
func (s *ParallelSyncStrategy) Execute(ctx context.Context, source, target []*types.CorpusEntry, options SyncOptions) (*SyncResult, error) {
	// Split source entries into chunks
	chunks := s.splitIntoChunks(source, s.chunkSize)

	// Create worker pool
	var wg sync.WaitGroup
	resultChan := make(chan *SyncResult, len(chunks))
	errorChan := make(chan error, len(chunks))

	// Create semaphore for worker limit
	sem := make(chan struct{}, s.workers)

	// Process chunks in parallel
	for _, chunk := range chunks {
		wg.Add(1)
		go func(entries []*types.CorpusEntry) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Check context
			if err := ctx.Err(); err != nil {
				errorChan <- err
				return
			}

			// Process chunk
			result, err := s.baseStrategy.Execute(ctx, entries, target, options)
			if err != nil {
				errorChan <- err
			} else {
				resultChan <- result
			}
		}(chunk)
	}

	// Wait for all workers to complete
	wg.Wait()
	close(resultChan)
	close(errorChan)

	// Aggregate results
	finalResult := &SyncResult{
		StartTime: time.Now(),
		DryRun:    options.DryRun,
		Conflicts: make([]SyncConflict, 0),
		Errors:    make([]error, 0),
	}

	// Collect errors
	for err := range errorChan {
		finalResult.Errors = append(finalResult.Errors, err)
	}

	// Aggregate partial results
	for result := range resultChan {
		finalResult.EntriesAdded += result.EntriesAdded
		finalResult.EntriesUpdated += result.EntriesUpdated
		finalResult.EntriesDeleted += result.EntriesDeleted
		finalResult.EntriesSkipped += result.EntriesSkipped
		finalResult.Conflicts = append(finalResult.Conflicts, result.Conflicts...)
		finalResult.Errors = append(finalResult.Errors, result.Errors...)
	}

	finalResult.EndTime = time.Now()
	finalResult.Duration = finalResult.EndTime.Sub(finalResult.StartTime)
	finalResult.Success = len(finalResult.Errors) == 0

	return finalResult, nil
}

// SupportsIncremental delegates to base strategy
func (s *ParallelSyncStrategy) SupportsIncremental() bool {
	return s.baseStrategy.SupportsIncremental()
}

// splitIntoChunks splits entries into chunks for parallel processing
func (s *ParallelSyncStrategy) splitIntoChunks(entries []*types.CorpusEntry, chunkSize int) [][]*types.CorpusEntry {
	var chunks [][]*types.CorpusEntry

	for i := 0; i < len(entries); i += chunkSize {
		end := i + chunkSize
		if end > len(entries) {
			end = len(entries)
		}
		chunks = append(chunks, entries[i:end])
	}

	return chunks
}
