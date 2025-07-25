package strategies

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
)

// CompositeStrategy combines multiple selection strategies
type CompositeStrategy struct {
	mu                  sync.RWMutex
	strategies          []SelectionStrategy
	weights             []float64
	mode                CompositeMode
	rng                 *rand.Rand
	selectionCount      uint64
	strategySelections  map[string]uint64
	performanceTracking bool
	strategyPerformance map[string]*PerformanceMetrics
}

// CompositeMode defines how strategies are combined
type CompositeMode int

const (
	// ModeRotation rotates through strategies
	ModeRotation CompositeMode = iota
	// ModeWeightedRandom selects strategies randomly based on weights
	ModeWeightedRandom
	// ModeBestPerforming uses the best performing strategy
	ModeBestPerforming
	// ModeConsensus combines results from multiple strategies
	ModeConsensus
	// ModeHybrid uses different strategies for different selection sizes
	ModeHybrid
)

// PerformanceMetrics tracks strategy performance
type PerformanceMetrics struct {
	SelectionCount uint64
	CoverageGained float64
	SuccessRate    float64
	AverageQuality float64
	LastUpdateTime time.Time
}

// NewCompositeStrategy creates a new composite selection strategy
func NewCompositeStrategy(mode CompositeMode, performanceTracking bool) *CompositeStrategy {
	return &CompositeStrategy{
		strategies:          make([]SelectionStrategy, 0),
		weights:             make([]float64, 0),
		mode:                mode,
		rng:                 rand.New(rand.NewSource(time.Now().UnixNano())),
		strategySelections:  make(map[string]uint64),
		performanceTracking: performanceTracking,
		strategyPerformance: make(map[string]*PerformanceMetrics),
	}
}

// Name returns the strategy name
func (s *CompositeStrategy) Name() string {
	modeStr := ""
	switch s.mode {
	case ModeRotation:
		modeStr = "rotation"
	case ModeWeightedRandom:
		modeStr = "weighted"
	case ModeBestPerforming:
		modeStr = "best"
	case ModeConsensus:
		modeStr = "consensus"
	case ModeHybrid:
		modeStr = "hybrid"
	}
	return fmt.Sprintf("composite-%s", modeStr)
}

// AddStrategy adds a strategy to the composite with a weight
func (s *CompositeStrategy) AddStrategy(strategy SelectionStrategy, weight float64) error {
	if strategy == nil {
		return errors.New("strategy cannot be nil")
	}
	if weight <= 0 {
		return errors.New("weight must be positive")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.strategies = append(s.strategies, strategy)
	s.weights = append(s.weights, weight)

	if s.performanceTracking {
		s.strategyPerformance[strategy.Name()] = &PerformanceMetrics{
			LastUpdateTime: time.Now(),
		}
	}

	return nil
}

// Select performs selection based on the composite mode
func (s *CompositeStrategy) Select(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error) {
	if count <= 0 {
		return nil, errors.New("count must be positive")
	}

	if len(collection) == 0 {
		return nil, errors.New("collection is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.strategies) == 0 {
		return nil, errors.New("no strategies registered")
	}

	var selected []*types.CorpusEntry
	var err error

	switch s.mode {
	case ModeRotation:
		selected, err = s.selectRotation(ctx, collection, count, options)
	case ModeWeightedRandom:
		selected, err = s.selectWeightedRandom(ctx, collection, count, options)
	case ModeBestPerforming:
		selected, err = s.selectBestPerforming(ctx, collection, count, options)
	case ModeConsensus:
		selected, err = s.selectConsensus(ctx, collection, count, options)
	case ModeHybrid:
		selected, err = s.selectHybrid(ctx, collection, count, options)
	default:
		return nil, fmt.Errorf("unknown composite mode: %v", s.mode)
	}

	if err == nil {
		s.selectionCount++
	}

	return selected, err
}

// Priority computes average priority across all strategies
func (s *CompositeStrategy) Priority(entry *types.CorpusEntry) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.strategies) == 0 {
		return 0.0
	}

	totalPriority := 0.0
	totalWeight := 0.0

	for i, strategy := range s.strategies {
		priority := strategy.Priority(entry)
		weight := s.weights[i]
		totalPriority += priority * weight
		totalWeight += weight
	}

	if totalWeight > 0 {
		return totalPriority / totalWeight
	}

	return 0.0
}

// SupportsCriteria indicates if the strategy supports custom selection criteria
func (s *CompositeStrategy) SupportsCriteria() bool {
	return true
}

// Reset resets the internal state
func (s *CompositeStrategy) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.selectionCount = 0
	s.strategySelections = make(map[string]uint64)

	// Reset all component strategies
	for _, strategy := range s.strategies {
		strategy.Reset()
	}

	// Reset performance metrics
	if s.performanceTracking {
		for name := range s.strategyPerformance {
			s.strategyPerformance[name] = &PerformanceMetrics{
				LastUpdateTime: time.Now(),
			}
		}
	}
}

// selectRotation rotates through strategies in order
func (s *CompositeStrategy) selectRotation(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error) {
	// Determine which strategy to use based on selection count
	strategyIndex := int(s.selectionCount) % len(s.strategies)
	strategy := s.strategies[strategyIndex]

	s.strategySelections[strategy.Name()]++

	return strategy.Select(ctx, collection, count, options)
}

// selectWeightedRandom randomly selects a strategy based on weights
func (s *CompositeStrategy) selectWeightedRandom(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error) {
	// Calculate total weight
	totalWeight := 0.0
	for _, weight := range s.weights {
		totalWeight += weight
	}

	// Select strategy based on weights
	r := s.rng.Float64() * totalWeight
	cumulative := 0.0

	for i, weight := range s.weights {
		cumulative += weight
		if cumulative >= r {
			strategy := s.strategies[i]
			s.strategySelections[strategy.Name()]++
			return strategy.Select(ctx, collection, count, options)
		}
	}

	// Fallback to last strategy
	strategy := s.strategies[len(s.strategies)-1]
	s.strategySelections[strategy.Name()]++
	return strategy.Select(ctx, collection, count, options)
}

// selectBestPerforming uses the best performing strategy
func (s *CompositeStrategy) selectBestPerforming(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error) {
	if !s.performanceTracking {
		// Fallback to weighted random if performance tracking is disabled
		return s.selectWeightedRandom(ctx, collection, count, options)
	}

	// Find best performing strategy
	bestStrategy := s.strategies[0]
	bestScore := 0.0

	for _, strategy := range s.strategies {
		if metrics, exists := s.strategyPerformance[strategy.Name()]; exists {
			score := s.calculatePerformanceScore(metrics)
			if score > bestScore {
				bestScore = score
				bestStrategy = strategy
			}
		}
	}

	s.strategySelections[bestStrategy.Name()]++
	return bestStrategy.Select(ctx, collection, count, options)
}

// selectConsensus combines results from multiple strategies
func (s *CompositeStrategy) selectConsensus(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error) {
	// Get selections from all strategies
	allSelections := make([][]*types.CorpusEntry, 0, len(s.strategies))
	voteCounts := make(map[string]float64)

	for i, strategy := range s.strategies {
		selected, err := strategy.Select(ctx, collection, count, options)
		if err != nil {
			continue
		}

		allSelections = append(allSelections, selected)

		// Count votes with weights
		weight := s.weights[i]
		for _, entry := range selected {
			voteCounts[entry.ID] += weight
		}

		s.strategySelections[strategy.Name()]++
	}

	if len(allSelections) == 0 {
		return nil, errors.New("all strategies failed")
	}

	// Sort entries by vote count
	type votedEntry struct {
		entry *types.CorpusEntry
		votes float64
	}

	voted := make([]votedEntry, 0, len(voteCounts))
	entryMap := make(map[string]*types.CorpusEntry)

	// Build entry map
	for _, entries := range allSelections {
		for _, entry := range entries {
			entryMap[entry.ID] = entry
		}
	}

	// Create voted entries
	for id, votes := range voteCounts {
		if entry, exists := entryMap[id]; exists {
			voted = append(voted, votedEntry{entry: entry, votes: votes})
		}
	}

	// Sort by votes
	sort.Slice(voted, func(i, j int) bool {
		return voted[i].votes > voted[j].votes
	})

	// Select top entries
	selected := make([]*types.CorpusEntry, 0, count)
	for i := 0; i < count && i < len(voted); i++ {
		selected = append(selected, voted[i].entry)
	}

	return selected, nil
}

// selectHybrid uses different strategies based on selection size
func (s *CompositeStrategy) selectHybrid(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error) {
	// Use different strategies based on count
	var strategy SelectionStrategy

	if count < 10 {
		// Small selections: use most precise strategy (usually coverage-based)
		for _, strat := range s.strategies {
			if strat.Name() == "coverage-based" || strat.Name() == "incremental-coverage" {
				strategy = strat
				break
			}
		}
	} else if count < 50 {
		// Medium selections: use weighted strategy
		for _, strat := range s.strategies {
			if strat.Name() == "weighted" || strat.Name() == "adaptive-weighted" {
				strategy = strat
				break
			}
		}
	} else {
		// Large selections: use fast strategy (random)
		for _, strat := range s.strategies {
			if strat.Name() == "uniform-random" || strat.Name() == "weighted-random" {
				strategy = strat
				break
			}
		}
	}

	// Fallback to first strategy if specific type not found
	if strategy == nil {
		strategy = s.strategies[0]
	}

	s.strategySelections[strategy.Name()]++
	return strategy.Select(ctx, collection, count, options)
}

// calculatePerformanceScore calculates a performance score for a strategy
func (s *CompositeStrategy) calculatePerformanceScore(metrics *PerformanceMetrics) float64 {
	if metrics.SelectionCount == 0 {
		return 0.0
	}

	// Weighted score based on different metrics
	score := 0.0

	// Coverage contribution (40%)
	score += metrics.CoverageGained * 0.4

	// Success rate (30%)
	score += metrics.SuccessRate * 0.3

	// Quality (30%)
	score += metrics.AverageQuality * 0.3

	// Decay factor for old data
	age := time.Since(metrics.LastUpdateTime).Hours()
	decayFactor := 1.0 / (1.0 + age/24.0) // Decay over days

	return score * decayFactor
}

// UpdatePerformance updates performance metrics for a strategy
func (s *CompositeStrategy) UpdatePerformance(strategyName string, coverageGain float64, success bool, quality float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.performanceTracking {
		return
	}

	metrics, exists := s.strategyPerformance[strategyName]
	if !exists {
		metrics = &PerformanceMetrics{}
		s.strategyPerformance[strategyName] = metrics
	}

	// Update metrics
	metrics.SelectionCount++
	metrics.CoverageGained = (metrics.CoverageGained*(float64(metrics.SelectionCount-1)) + coverageGain) / float64(metrics.SelectionCount)

	if success {
		metrics.SuccessRate = (metrics.SuccessRate*(float64(metrics.SelectionCount-1)) + 1.0) / float64(metrics.SelectionCount)
	} else {
		metrics.SuccessRate = (metrics.SuccessRate * float64(metrics.SelectionCount-1)) / float64(metrics.SelectionCount)
	}

	metrics.AverageQuality = (metrics.AverageQuality*(float64(metrics.SelectionCount-1)) + quality) / float64(metrics.SelectionCount)
	metrics.LastUpdateTime = time.Now()
}

// GetStrategySelections returns selection counts for each strategy
func (s *CompositeStrategy) GetStrategySelections() map[string]uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	selections := make(map[string]uint64)
	for name, count := range s.strategySelections {
		selections[name] = count
	}
	return selections
}

// GetPerformanceMetrics returns performance metrics for all strategies
func (s *CompositeStrategy) GetPerformanceMetrics() map[string]*PerformanceMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.performanceTracking {
		return nil
	}

	metrics := make(map[string]*PerformanceMetrics)
	for name, perf := range s.strategyPerformance {
		metrics[name] = &PerformanceMetrics{
			SelectionCount: perf.SelectionCount,
			CoverageGained: perf.CoverageGained,
			SuccessRate:    perf.SuccessRate,
			AverageQuality: perf.AverageQuality,
			LastUpdateTime: perf.LastUpdateTime,
		}
	}
	return metrics
}
