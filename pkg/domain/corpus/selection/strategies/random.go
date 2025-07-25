package strategies

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
)

// RandomSelectionStrategy implements random selection of corpus entries
type RandomSelectionStrategy struct {
	mu              sync.RWMutex
	rng             *rand.Rand
	selectionCount  uint64
	totalPriority   float64
	selectedEntries map[string]uint64 // Track selection frequency
}

// NewRandomSelectionStrategy creates a new random selection strategy
func NewRandomSelectionStrategy(seed int64) *RandomSelectionStrategy {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &RandomSelectionStrategy{
		rng:             rand.New(rand.NewSource(seed)),
		selectedEntries: make(map[string]uint64),
	}
}

// Name returns the strategy name
func (s *RandomSelectionStrategy) Name() string {
	return "random"
}

// Select randomly selects corpus entries
func (s *RandomSelectionStrategy) Select(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error) {
	if count <= 0 {
		return nil, errors.New("count must be positive")
	}

	if len(collection) == 0 {
		return nil, errors.New("collection is empty")
	}

	// Apply filters
	eligible := s.filterEntries(collection, options)
	if len(eligible) == 0 {
		return nil, errors.New("no eligible entries after filtering")
	}

	// Adjust count if necessary
	if count > len(eligible) {
		count = len(eligible)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Perform random selection
	selected := make([]*types.CorpusEntry, 0, count)
	selectedMap := make(map[string]bool)

	// Fisher-Yates shuffle for uniform distribution
	indices := make([]int, len(eligible))
	for i := range indices {
		indices[i] = i
	}

	for i := len(indices) - 1; i > 0; i-- {
		j := s.rng.Intn(i + 1)
		indices[i], indices[j] = indices[j], indices[i]
	}

	// Select first 'count' entries
	for i := 0; i < count && i < len(indices); i++ {
		entry := eligible[indices[i]]
		if !selectedMap[entry.ID] {
			selected = append(selected, entry)
			selectedMap[entry.ID] = true
			s.selectedEntries[entry.ID]++
			s.selectionCount++
		}
	}

	return selected, nil
}

// Priority computes priority score for a corpus entry
func (s *RandomSelectionStrategy) Priority(entry *types.CorpusEntry) float64 {
	// Random strategy assigns equal priority to all entries
	return 1.0
}

// SupportsCriteria indicates if the strategy supports custom selection criteria
func (s *RandomSelectionStrategy) SupportsCriteria() bool {
	return true
}

// Reset resets the internal state
func (s *RandomSelectionStrategy) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.selectionCount = 0
	s.totalPriority = 0
	s.selectedEntries = make(map[string]uint64)
}

// GetMetrics returns metrics for this strategy
func (s *RandomSelectionStrategy) GetMetrics() *StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	avgPriority := 1.0 // All entries have equal priority in random selection

	return &StrategyMetrics{
		Name:            s.Name(),
		SelectionCount:  s.selectionCount,
		AveragePriority: avgPriority,
	}
}

// filterEntries applies selection options to filter eligible entries
func (s *RandomSelectionStrategy) filterEntries(entries []*types.CorpusEntry, options SelectionOptions) []*types.CorpusEntry {
	eligible := make([]*types.CorpusEntry, 0, len(entries))

	now := time.Now()

	for _, entry := range entries {
		// Check minimum coverage
		if options.MinCoverage > 0 && entry.Coverage.CoverageScore < options.MinCoverage {
			continue
		}

		// Check maximum age
		if options.MaxAge > 0 {
			age := now.Sub(entry.CreatedAt).Seconds()
			if age > float64(options.MaxAge) {
				continue
			}
		}

		// Check execution exclusion
		if options.ExcludeExecuted && entry.LastExecutedAt != nil {
			executionAge := now.Sub(*entry.LastExecutedAt).Seconds()
			if executionAge < float64(options.ExcludeWindow) {
				continue
			}
		}

		// Check tags
		if len(options.Tags) > 0 {
			hasTag := false
			for _, requiredTag := range options.Tags {
				for _, entryTag := range entry.Tags {
					if entryTag == requiredTag {
						hasTag = true
						break
					}
				}
				if hasTag {
					break
				}
			}
			if !hasTag {
				continue
			}
		}

		// Prefer interesting if specified
		if options.PreferInteresting && !entry.IsInteresting() {
			// Skip with some probability
			if s.rng.Float64() > 0.2 { // 20% chance to include non-interesting
				continue
			}
		}

		eligible = append(eligible, entry)
	}

	return eligible
}

// UniformRandomStrategy is an alias for basic random selection
type UniformRandomStrategy struct {
	*RandomSelectionStrategy
}

// NewUniformRandomStrategy creates a uniform random selection strategy
func NewUniformRandomStrategy() *UniformRandomStrategy {
	return &UniformRandomStrategy{
		RandomSelectionStrategy: NewRandomSelectionStrategy(0),
	}
}

// Name returns the strategy name
func (s *UniformRandomStrategy) Name() string {
	return "uniform-random"
}

// WeightedRandomStrategy implements weighted random selection
type WeightedRandomStrategy struct {
	mu              sync.RWMutex
	rng             *rand.Rand
	weightFactors   WeightFactors
	selectionCount  uint64
	totalPriority   float64
	selectedEntries map[string]uint64
}

// NewWeightedRandomStrategy creates a new weighted random selection strategy
func NewWeightedRandomStrategy(seed int64, factors WeightFactors) *WeightedRandomStrategy {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &WeightedRandomStrategy{
		rng:             rand.New(rand.NewSource(seed)),
		weightFactors:   factors,
		selectedEntries: make(map[string]uint64),
	}
}

// Name returns the strategy name
func (s *WeightedRandomStrategy) Name() string {
	return "weighted-random"
}

// Select performs weighted random selection
func (s *WeightedRandomStrategy) Select(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error) {
	if count <= 0 {
		return nil, errors.New("count must be positive")
	}

	if len(collection) == 0 {
		return nil, errors.New("collection is empty")
	}

	// Calculate weights for all entries
	weights := make([]float64, len(collection))
	totalWeight := 0.0

	for i, entry := range collection {
		weight := s.Priority(entry)
		if options.WeightFactors.CoverageWeight > 0 || options.WeightFactors.AgeWeight > 0 {
			// Use custom weight factors if provided
			weight = s.calculateWeight(entry, options.WeightFactors)
		}
		weights[i] = weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		return nil, errors.New("total weight is zero")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Perform weighted selection
	selected := make([]*types.CorpusEntry, 0, count)
	selectedMap := make(map[string]bool)

	for len(selected) < count && len(selected) < len(collection) {
		// Select based on weights
		r := s.rng.Float64() * totalWeight
		cumulative := 0.0

		for i, weight := range weights {
			cumulative += weight
			if cumulative >= r {
				entry := collection[i]
				if !selectedMap[entry.ID] {
					selected = append(selected, entry)
					selectedMap[entry.ID] = true
					s.selectedEntries[entry.ID]++
					s.selectionCount++
					s.totalPriority += weight

					// Remove selected entry from next round
					totalWeight -= weight
					weights[i] = 0
				}
				break
			}
		}
	}

	return selected, nil
}

// Priority computes priority score for a corpus entry
func (s *WeightedRandomStrategy) Priority(entry *types.CorpusEntry) float64 {
	return s.calculateWeight(entry, s.weightFactors)
}

// SupportsCriteria indicates if the strategy supports custom selection criteria
func (s *WeightedRandomStrategy) SupportsCriteria() bool {
	return true
}

// Reset resets the internal state
func (s *WeightedRandomStrategy) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.selectionCount = 0
	s.totalPriority = 0
	s.selectedEntries = make(map[string]uint64)
}

// calculateWeight calculates the weight for an entry based on factors
func (s *WeightedRandomStrategy) calculateWeight(entry *types.CorpusEntry, factors WeightFactors) float64 {
	weight := 0.0

	// Coverage component
	if factors.CoverageWeight > 0 {
		weight += entry.Coverage.CoverageScore * factors.CoverageWeight
	}

	// Age component (newer is better)
	if factors.AgeWeight > 0 {
		age := time.Since(entry.CreatedAt).Hours()
		ageScore := 1.0 / (1.0 + age/24.0) // Decay over days
		weight += ageScore * factors.AgeWeight
	}

	// Execution component (less executed is better)
	if factors.ExecutionWeight > 0 {
		execScore := 1.0 / (1.0 + float64(entry.ExecutionCount)/100.0)
		weight += execScore * factors.ExecutionWeight
	}

	// Generation component (lower generation is better for diversity)
	if factors.GenerationWeight > 0 {
		genScore := 1.0 / (1.0 + float64(entry.MutationInfo.Generation)/10.0)
		weight += genScore * factors.GenerationWeight
	}

	// Size component (prefer smaller inputs)
	if factors.SizeWeight > 0 {
		sizeScore := 1.0 / (1.0 + float64(entry.Size)/1024.0) // Decay over KB
		weight += sizeScore * factors.SizeWeight
	}

	// Ensure minimum weight
	if weight < 0.01 {
		weight = 0.01
	}

	return weight
}

// GetMetrics returns metrics for this strategy
func (s *WeightedRandomStrategy) GetMetrics() *StrategyMetrics {
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

// GetSelectionDistribution returns the distribution of selections
func (s *WeightedRandomStrategy) GetSelectionDistribution() map[string]uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dist := make(map[string]uint64)
	for id, count := range s.selectedEntries {
		dist[id] = count
	}
	return dist
}

// ReservoirSamplingStrategy implements reservoir sampling for streaming selection
type ReservoirSamplingStrategy struct {
	mu            sync.RWMutex
	rng           *rand.Rand
	reservoirSize int
	seenCount     uint64
}

// NewReservoirSamplingStrategy creates a new reservoir sampling strategy
func NewReservoirSamplingStrategy(reservoirSize int, seed int64) *ReservoirSamplingStrategy {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	if reservoirSize <= 0 {
		reservoirSize = 100
	}
	return &ReservoirSamplingStrategy{
		rng:           rand.New(rand.NewSource(seed)),
		reservoirSize: reservoirSize,
	}
}

// Name returns the strategy name
func (s *ReservoirSamplingStrategy) Name() string {
	return "reservoir-sampling"
}

// Select performs reservoir sampling selection
func (s *ReservoirSamplingStrategy) Select(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error) {
	if count <= 0 {
		return nil, errors.New("count must be positive")
	}

	if len(collection) == 0 {
		return nil, errors.New("collection is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize reservoir
	reservoir := make([]*types.CorpusEntry, 0, count)

	// Fill reservoir with first 'count' elements
	for i := 0; i < count && i < len(collection); i++ {
		reservoir = append(reservoir, collection[i])
		s.seenCount++
	}

	// Process remaining elements
	for i := count; i < len(collection); i++ {
		s.seenCount++
		// Random index from 0 to i
		j := s.rng.Intn(i + 1)
		if j < count {
			reservoir[j] = collection[i]
		}

		// Check context
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	return reservoir, nil
}

// Priority computes priority score for a corpus entry
func (s *ReservoirSamplingStrategy) Priority(entry *types.CorpusEntry) float64 {
	// Reservoir sampling treats all entries equally
	return 1.0
}

// SupportsCriteria indicates if the strategy supports custom selection criteria
func (s *ReservoirSamplingStrategy) SupportsCriteria() bool {
	return false // Reservoir sampling doesn't support filtering
}

// Reset resets the internal state
func (s *ReservoirSamplingStrategy) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seenCount = 0
}

// GetStats returns statistics about the reservoir sampling
func (s *ReservoirSamplingStrategy) GetStats() (uint64, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.seenCount, s.reservoirSize
}

// Validate checks if the strategy is properly configured
func (s *ReservoirSamplingStrategy) Validate() error {
	if s.reservoirSize <= 0 {
		return fmt.Errorf("reservoir size must be positive, got %d", s.reservoirSize)
	}
	return nil
}
