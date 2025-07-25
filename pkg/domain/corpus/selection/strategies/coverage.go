package strategies

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
)

// CoverageBasedStrategy selects corpus entries based on code coverage
type CoverageBasedStrategy struct {
	mu                sync.RWMutex
	coverageThreshold float64
	selectionCount    uint64
	totalPriority     float64
	coverageHistory   map[string]*CoverageRecord
	edgeCoverageMap   map[uint32]string // Maps edge ID to entry ID that first covered it
}

// CoverageRecord tracks coverage history for an entry
type CoverageRecord struct {
	EntryID          string
	LastCoverage     float64
	CoverageGrowth   float64
	UniqueEdges      []uint32
	SelectionCount   uint64
	LastSelectedTime time.Time
}

// NewCoverageBasedStrategy creates a new coverage-based selection strategy
func NewCoverageBasedStrategy(coverageThreshold float64) *CoverageBasedStrategy {
	if coverageThreshold <= 0 {
		coverageThreshold = 0.5
	}
	return &CoverageBasedStrategy{
		coverageThreshold: coverageThreshold,
		coverageHistory:   make(map[string]*CoverageRecord),
		edgeCoverageMap:   make(map[uint32]string),
	}
}

// Name returns the strategy name
func (s *CoverageBasedStrategy) Name() string {
	return "coverage-based"
}

// Select selects entries based on coverage metrics
func (s *CoverageBasedStrategy) Select(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error) {
	if count <= 0 {
		return nil, errors.New("count must be positive")
	}

	if len(collection) == 0 {
		return nil, errors.New("collection is empty")
	}

	s.mu.Lock()
	// Update coverage records
	s.updateCoverageRecords(collection)

	// Score and rank entries (while holding lock)
	scoredEntries := s.scoreEntriesLocked(collection, options)
	s.mu.Unlock()

	// Sort by score (descending) - done without lock
	sort.Slice(scoredEntries, func(i, j int) bool {
		return scoredEntries[i].score > scoredEntries[j].score
	})

	// Select top entries
	selected := make([]*types.CorpusEntry, 0, count)
	for i := 0; i < count && i < len(scoredEntries); i++ {
		entry := scoredEntries[i].entry
		selected = append(selected, entry)
	}

	// Update selection metrics
	s.mu.Lock()
	for i := 0; i < count && i < len(scoredEntries); i++ {
		s.selectionCount++
		s.totalPriority += scoredEntries[i].score

		// Update coverage record
		if record, exists := s.coverageHistory[scoredEntries[i].entry.ID]; exists {
			record.SelectionCount++
			record.LastSelectedTime = time.Now()
		}
	}
	s.mu.Unlock()

	return selected, nil
}

// Priority computes priority score based on coverage
func (s *CoverageBasedStrategy) Priority(entry *types.CorpusEntry) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	score := entry.Coverage.CoverageScore

	// Boost for new coverage
	if entry.Coverage.NewCoverage {
		score *= 2.0
	}

	// Boost for coverage gain
	if entry.Coverage.CoverageGained > 0 {
		score += float64(entry.Coverage.CoverageGained) / 100.0
	}

	// Check historical coverage growth
	if record, exists := s.coverageHistory[entry.ID]; exists {
		if record.CoverageGrowth > 0 {
			score *= (1.0 + record.CoverageGrowth)
		}
	}

	return score
}

// SupportsCriteria indicates if the strategy supports custom selection criteria
func (s *CoverageBasedStrategy) SupportsCriteria() bool {
	return true
}

// Reset resets the internal state
func (s *CoverageBasedStrategy) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.selectionCount = 0
	s.totalPriority = 0
	s.coverageHistory = make(map[string]*CoverageRecord)
	s.edgeCoverageMap = make(map[uint32]string)
}

// updateCoverageRecords updates internal coverage tracking
func (s *CoverageBasedStrategy) updateCoverageRecords(entries []*types.CorpusEntry) {
	for _, entry := range entries {
		record, exists := s.coverageHistory[entry.ID]
		if !exists {
			record = &CoverageRecord{
				EntryID:     entry.ID,
				UniqueEdges: make([]uint32, 0),
			}
			s.coverageHistory[entry.ID] = record
		}

		// Calculate coverage growth
		if record.LastCoverage > 0 {
			record.CoverageGrowth = entry.Coverage.CoverageScore - record.LastCoverage
		}
		record.LastCoverage = entry.Coverage.CoverageScore

		// Track unique edges (simplified - in real implementation would use actual edge IDs)
		for i := uint32(0); i < entry.Coverage.CoveredEdges; i++ {
			if firstCoverer, exists := s.edgeCoverageMap[i]; !exists || firstCoverer == entry.ID {
				s.edgeCoverageMap[i] = entry.ID
				record.UniqueEdges = append(record.UniqueEdges, i)
			}
		}
	}
}

// scoredEntry holds an entry with its calculated score
type scoredEntry struct {
	entry *types.CorpusEntry
	score float64
}

// scoreEntriesLocked calculates scores for all entries (must be called with lock held)
func (s *CoverageBasedStrategy) scoreEntriesLocked(entries []*types.CorpusEntry, options SelectionOptions) []scoredEntry {
	scored := make([]scoredEntry, 0, len(entries))

	for _, entry := range entries {
		// Base score from coverage (without calling Priority which acquires lock)
		score := entry.Coverage.CoverageScore

		// Boost for new coverage
		if entry.Coverage.NewCoverage {
			score *= 2.0
		}

		// Boost for coverage gain
		if entry.Coverage.CoverageGained > 0 {
			score += float64(entry.Coverage.CoverageGained) / 100.0
		}

		// Check historical coverage growth
		if record, exists := s.coverageHistory[entry.ID]; exists {
			if record.CoverageGrowth > 0 {
				score *= (1.0 + record.CoverageGrowth)
			}
		}

		// Apply threshold filter
		if entry.Coverage.CoverageScore < s.coverageThreshold && !entry.Coverage.NewCoverage {
			score *= 0.1 // Heavily penalize low coverage without new edges
		}

		// Apply option-based adjustments
		if options.PreferInteresting && entry.IsInteresting() {
			score *= 1.5
		}

		// Penalize frequently selected entries
		if record, exists := s.coverageHistory[entry.ID]; exists {
			if record.SelectionCount > 10 {
				score *= 0.8
			}
			// Penalize recently selected
			timeSinceSelection := time.Since(record.LastSelectedTime)
			if timeSinceSelection < time.Minute {
				score *= 0.5
			}
		}

		// Consider unique edges
		if record, exists := s.coverageHistory[entry.ID]; exists && len(record.UniqueEdges) > 0 {
			score *= (1.0 + float64(len(record.UniqueEdges))/100.0)
		}

		scored = append(scored, scoredEntry{entry: entry, score: score})
	}

	return scored
}

// GetMetrics returns metrics for this strategy
func (s *CoverageBasedStrategy) GetMetrics() *StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	avgPriority := 0.0
	if s.selectionCount > 0 {
		avgPriority = s.totalPriority / float64(s.selectionCount)
	}

	return &StrategyMetrics{
		Name:            s.Name(),
		SelectionCount:  s.selectionCount,
		AveragePriority: avgPriority,
	}
}

// GetCoverageStats returns coverage statistics
func (s *CoverageBasedStrategy) GetCoverageStats() (totalEdges int, uniqueEntries int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	uniqueEntriesMap := make(map[string]bool)
	for _, entryID := range s.edgeCoverageMap {
		uniqueEntriesMap[entryID] = true
	}

	return len(s.edgeCoverageMap), len(uniqueEntriesMap)
}

// IncrementalCoverageStrategy focuses on entries that incrementally increase coverage
type IncrementalCoverageStrategy struct {
	*CoverageBasedStrategy
	incrementThreshold float64
}

// NewIncrementalCoverageStrategy creates a strategy focused on incremental coverage gains
func NewIncrementalCoverageStrategy(incrementThreshold float64) *IncrementalCoverageStrategy {
	if incrementThreshold <= 0 {
		incrementThreshold = 0.01 // 1% improvement threshold
	}
	return &IncrementalCoverageStrategy{
		CoverageBasedStrategy: NewCoverageBasedStrategy(0.3), // Lower base threshold
		incrementThreshold:    incrementThreshold,
	}
}

// Name returns the strategy name
func (s *IncrementalCoverageStrategy) Name() string {
	return "incremental-coverage"
}

// Priority computes priority with focus on incremental gains
func (s *IncrementalCoverageStrategy) Priority(entry *types.CorpusEntry) float64 {
	baseScore := s.CoverageBasedStrategy.Priority(entry)

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Boost entries with recent coverage growth
	if record, exists := s.coverageHistory[entry.ID]; exists {
		if record.CoverageGrowth >= s.incrementThreshold {
			// Exponential boost for significant growth
			baseScore *= (1.0 + record.CoverageGrowth*10.0)
		}
	}

	return baseScore
}

// RareCoverageStrategy prioritizes entries covering rarely-hit code paths
type RareCoverageStrategy struct {
	mu              sync.RWMutex
	edgeHitCount    map[uint32]uint64   // Count how many times each edge is hit
	entryEdges      map[string][]uint32 // Track which edges each entry covers
	selectionCount  uint64
	rarityThreshold uint64 // Edges hit less than this are considered rare
}

// NewRareCoverageStrategy creates a strategy focused on rare code paths
func NewRareCoverageStrategy(rarityThreshold uint64) *RareCoverageStrategy {
	if rarityThreshold == 0 {
		rarityThreshold = 5
	}
	return &RareCoverageStrategy{
		edgeHitCount:    make(map[uint32]uint64),
		entryEdges:      make(map[string][]uint32),
		rarityThreshold: rarityThreshold,
	}
}

// Name returns the strategy name
func (s *RareCoverageStrategy) Name() string {
	return "rare-coverage"
}

// Select selects entries covering rare code paths
func (s *RareCoverageStrategy) Select(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error) {
	if count <= 0 {
		return nil, errors.New("count must be positive")
	}

	if len(collection) == 0 {
		return nil, errors.New("collection is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Update edge hit counts
	s.updateEdgeHitCounts(collection)

	// Score entries based on rare edge coverage
	type scoredEntry struct {
		entry     *types.CorpusEntry
		rareEdges int
		score     float64
	}

	scored := make([]scoredEntry, 0, len(collection))
	for _, entry := range collection {
		rareEdges := s.countRareEdges(entry)
		score := float64(rareEdges) * entry.Coverage.CoverageScore

		scored = append(scored, scoredEntry{
			entry:     entry,
			rareEdges: rareEdges,
			score:     score,
		})
	}

	// Sort by score (descending)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Select top entries
	selected := make([]*types.CorpusEntry, 0, count)
	for i := 0; i < count && i < len(scored); i++ {
		selected = append(selected, scored[i].entry)
		s.selectionCount++
	}

	return selected, nil
}

// Priority computes priority based on rare edge coverage
func (s *RareCoverageStrategy) Priority(entry *types.CorpusEntry) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rareEdges := s.countRareEdges(entry)
	return float64(rareEdges) * entry.Coverage.CoverageScore
}

// SupportsCriteria indicates if the strategy supports custom selection criteria
func (s *RareCoverageStrategy) SupportsCriteria() bool {
	return true
}

// Reset resets the internal state
func (s *RareCoverageStrategy) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.edgeHitCount = make(map[uint32]uint64)
	s.entryEdges = make(map[string][]uint32)
	s.selectionCount = 0
}

// updateEdgeHitCounts updates the hit count for edges
func (s *RareCoverageStrategy) updateEdgeHitCounts(entries []*types.CorpusEntry) {
	for _, entry := range entries {
		// Track edges for this entry (simplified - real implementation would use actual edge IDs)
		edges := make([]uint32, 0, entry.Coverage.CoveredEdges)
		for i := uint32(0); i < entry.Coverage.CoveredEdges; i++ {
			edges = append(edges, i)
			s.edgeHitCount[i] += entry.ExecutionCount
		}
		s.entryEdges[entry.ID] = edges
	}
}

// countRareEdges counts how many rare edges an entry covers
func (s *RareCoverageStrategy) countRareEdges(entry *types.CorpusEntry) int {
	edges, exists := s.entryEdges[entry.ID]
	if !exists {
		return 0
	}

	rareCount := 0
	for _, edge := range edges {
		if hitCount, exists := s.edgeHitCount[edge]; exists && hitCount <= s.rarityThreshold {
			rareCount++
		}
	}

	return rareCount
}

// GetRareEdgeStats returns statistics about rare edges
func (s *RareCoverageStrategy) GetRareEdgeStats() (totalRare int, totalEdges int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, count := range s.edgeHitCount {
		if count <= s.rarityThreshold {
			totalRare++
		}
	}

	return totalRare, len(s.edgeHitCount)
}
