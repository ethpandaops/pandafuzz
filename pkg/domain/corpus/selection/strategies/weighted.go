package strategies

import (
	"context"
	"errors"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
)

// WeightedSelectionStrategy implements multi-factor weighted selection
type WeightedSelectionStrategy struct {
	mu              sync.RWMutex
	weightFactors   WeightFactors
	selectionCount  uint64
	totalPriority   float64
	selectedEntries map[string]uint64
	entryScores     map[string]float64
	adaptiveWeights bool
	performanceData map[string]*PerformanceRecord
}

// PerformanceRecord tracks entry performance over time
type PerformanceRecord struct {
	EntryID          string
	CoverageGains    []float64
	ExecutionTimes   []time.Duration
	MutationSuccess  float64
	SelectionCount   uint64
	LastScore        float64
	LastSelectedTime time.Time
}

// NewWeightedSelectionStrategy creates a new weighted selection strategy
func NewWeightedSelectionStrategy(factors WeightFactors, adaptive bool) *WeightedSelectionStrategy {
	return &WeightedSelectionStrategy{
		weightFactors:   factors,
		selectedEntries: make(map[string]uint64),
		entryScores:     make(map[string]float64),
		adaptiveWeights: adaptive,
		performanceData: make(map[string]*PerformanceRecord),
	}
}

// Name returns the strategy name
func (s *WeightedSelectionStrategy) Name() string {
	if s.adaptiveWeights {
		return "adaptive-weighted"
	}
	return "weighted"
}

// Select performs weighted selection based on multiple factors
func (s *WeightedSelectionStrategy) Select(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error) {
	if count <= 0 {
		return nil, errors.New("count must be positive")
	}

	if len(collection) == 0 {
		return nil, errors.New("collection is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Update performance records
	s.updatePerformanceRecords(collection)

	// Adapt weights if enabled
	if s.adaptiveWeights {
		s.adaptWeights()
	}

	// Calculate scores for all entries
	scoredEntries := s.calculateScores(collection, options)

	// Sort by score (descending)
	sort.Slice(scoredEntries, func(i, j int) bool {
		return scoredEntries[i].score > scoredEntries[j].score
	})

	// Apply diversity factor to avoid selecting similar entries
	diverseSelection := s.applyDiversitySelection(scoredEntries, count)

	// Select entries
	selected := make([]*types.CorpusEntry, 0, count)
	for i := 0; i < len(diverseSelection) && i < count; i++ {
		entry := diverseSelection[i].entry
		selected = append(selected, entry)

		// Update metrics
		s.selectionCount++
		s.totalPriority += diverseSelection[i].score
		s.selectedEntries[entry.ID]++

		// Update performance record
		if record, exists := s.performanceData[entry.ID]; exists {
			record.SelectionCount++
			record.LastSelectedTime = time.Now()
			record.LastScore = diverseSelection[i].score
		}
	}

	return selected, nil
}

// Priority computes priority score for an entry
func (s *WeightedSelectionStrategy) Priority(entry *types.CorpusEntry) float64 {
	return s.calculateEntryScore(entry, s.weightFactors)
}

// SupportsCriteria indicates if the strategy supports custom selection criteria
func (s *WeightedSelectionStrategy) SupportsCriteria() bool {
	return true
}

// Reset resets the internal state
func (s *WeightedSelectionStrategy) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.selectionCount = 0
	s.totalPriority = 0
	s.selectedEntries = make(map[string]uint64)
	s.entryScores = make(map[string]float64)
	s.performanceData = make(map[string]*PerformanceRecord)
}

// scoredEntry holds an entry with its calculated score
type weightedScoredEntry struct {
	entry      *types.CorpusEntry
	score      float64
	components map[string]float64
}

// calculateScores calculates weighted scores for all entries
func (s *WeightedSelectionStrategy) calculateScores(entries []*types.CorpusEntry, options SelectionOptions) []weightedScoredEntry {
	scored := make([]weightedScoredEntry, 0, len(entries))

	// Use custom weights if provided
	weights := s.weightFactors
	if options.WeightFactors.CoverageWeight > 0 || options.WeightFactors.AgeWeight > 0 {
		weights = options.WeightFactors
	}

	for _, entry := range entries {
		// Calculate component scores
		components := s.calculateComponentScores(entry)

		// Apply weights
		score := s.applyWeights(components, weights)

		// Apply selection options
		if options.PreferInteresting && entry.IsInteresting() {
			score *= 1.5
		}

		// Apply exclusion penalties
		if options.ExcludeExecuted && entry.LastExecutedAt != nil {
			timeSince := time.Since(*entry.LastExecutedAt).Seconds()
			if timeSince < float64(options.ExcludeWindow) {
				score *= 0.1
			}
		}

		// Store score for future reference
		s.entryScores[entry.ID] = score

		scored = append(scored, weightedScoredEntry{
			entry:      entry,
			score:      score,
			components: components,
		})
	}

	return scored
}

// calculateComponentScores calculates individual component scores
func (s *WeightedSelectionStrategy) calculateComponentScores(entry *types.CorpusEntry) map[string]float64 {
	components := make(map[string]float64)

	// Coverage score (0-1)
	components["coverage"] = entry.Coverage.CoverageScore

	// Age score (newer is better, exponential decay)
	ageHours := time.Since(entry.CreatedAt).Hours()
	components["age"] = math.Exp(-ageHours / 168.0) // Decay over a week

	// Execution score (less executed is better)
	components["execution"] = 1.0 / (1.0 + math.Log1p(float64(entry.ExecutionCount)))

	// Generation score (balance between exploration and exploitation)
	genScore := 1.0
	if entry.MutationInfo.Generation > 0 {
		// Prefer middle generations
		optimalGen := 5.0
		genDiff := math.Abs(float64(entry.MutationInfo.Generation) - optimalGen)
		genScore = math.Exp(-genDiff / 3.0)
	}
	components["generation"] = genScore

	// Size score (prefer smaller inputs)
	sizeKB := float64(entry.Size) / 1024.0
	components["size"] = 1.0 / (1.0 + math.Log1p(sizeKB))

	// Performance-based adjustments
	if record, exists := s.performanceData[entry.ID]; exists {
		// Mutation success rate
		if record.MutationSuccess > 0 {
			components["mutation_success"] = record.MutationSuccess
		}

		// Coverage growth trend
		if len(record.CoverageGains) > 0 {
			avgGain := 0.0
			for _, gain := range record.CoverageGains {
				avgGain += gain
			}
			avgGain /= float64(len(record.CoverageGains))
			components["coverage_trend"] = 1.0 + avgGain
		}
	}

	return components
}

// applyWeights applies weight factors to component scores
func (s *WeightedSelectionStrategy) applyWeights(components map[string]float64, weights WeightFactors) float64 {
	score := 0.0

	score += components["coverage"] * weights.CoverageWeight
	score += components["age"] * weights.AgeWeight
	score += components["execution"] * weights.ExecutionWeight
	score += components["generation"] * weights.GenerationWeight
	score += components["size"] * weights.SizeWeight

	// Additional performance-based components
	if mutSuccess, exists := components["mutation_success"]; exists {
		score += mutSuccess * 0.1 // Fixed weight for mutation success
	}

	if covTrend, exists := components["coverage_trend"]; exists {
		score *= covTrend // Multiplicative boost for coverage trend
	}

	// Ensure positive score
	if score < 0.01 {
		score = 0.01
	}

	return score
}

// calculateEntryScore calculates score for a single entry
func (s *WeightedSelectionStrategy) calculateEntryScore(entry *types.CorpusEntry, weights WeightFactors) float64 {
	components := s.calculateComponentScores(entry)
	return s.applyWeights(components, weights)
}

// applyDiversitySelection ensures diverse selection
func (s *WeightedSelectionStrategy) applyDiversitySelection(scored []weightedScoredEntry, count int) []weightedScoredEntry {
	if count >= len(scored) {
		return scored
	}

	diverse := make([]weightedScoredEntry, 0, count)
	selected := make(map[string]bool)

	// First, select the highest scoring entry
	if len(scored) > 0 {
		diverse = append(diverse, scored[0])
		selected[scored[0].entry.ID] = true
	}

	// Select remaining entries with diversity consideration
	for len(diverse) < count && len(diverse) < len(scored) {
		bestIdx := -1
		bestScore := 0.0

		for i, candidate := range scored {
			if selected[candidate.entry.ID] {
				continue
			}

			// Calculate diversity score
			diversityScore := s.calculateDiversityScore(candidate.entry, diverse)
			adjustedScore := candidate.score * diversityScore

			if adjustedScore > bestScore {
				bestScore = adjustedScore
				bestIdx = i
			}
		}

		if bestIdx >= 0 {
			diverse = append(diverse, scored[bestIdx])
			selected[scored[bestIdx].entry.ID] = true
		} else {
			break
		}
	}

	return diverse
}

// calculateDiversityScore calculates how different an entry is from selected ones
func (s *WeightedSelectionStrategy) calculateDiversityScore(candidate *types.CorpusEntry, selected []weightedScoredEntry) float64 {
	if len(selected) == 0 {
		return 1.0
	}

	minSimilarity := 1.0

	for _, sel := range selected {
		similarity := 0.0

		// Size similarity
		sizeDiff := math.Abs(float64(candidate.Size-sel.entry.Size)) / float64(candidate.Size+sel.entry.Size)
		similarity += (1.0 - sizeDiff) * 0.3

		// Coverage similarity
		covDiff := math.Abs(candidate.Coverage.CoverageScore - sel.entry.Coverage.CoverageScore)
		similarity += (1.0 - covDiff) * 0.3

		// Generation similarity
		genDiff := math.Abs(float64(candidate.MutationInfo.Generation - sel.entry.MutationInfo.Generation))
		genSim := 1.0 / (1.0 + genDiff/5.0)
		similarity += genSim * 0.2

		// Parent similarity
		if candidate.MutationInfo.ParentID == sel.entry.MutationInfo.ParentID && candidate.MutationInfo.ParentID != "" {
			similarity += 0.2
		}

		if similarity < minSimilarity {
			minSimilarity = similarity
		}
	}

	// Return diversity (inverse of similarity)
	return 1.0 - minSimilarity
}

// updatePerformanceRecords updates performance tracking
func (s *WeightedSelectionStrategy) updatePerformanceRecords(entries []*types.CorpusEntry) {
	for _, entry := range entries {
		record, exists := s.performanceData[entry.ID]
		if !exists {
			record = &PerformanceRecord{
				EntryID:        entry.ID,
				CoverageGains:  make([]float64, 0),
				ExecutionTimes: make([]time.Duration, 0),
			}
			s.performanceData[entry.ID] = record
		}

		// Update coverage gains if available
		if len(record.CoverageGains) > 0 {
			lastCoverage := record.CoverageGains[len(record.CoverageGains)-1]
			gain := entry.Coverage.CoverageScore - lastCoverage
			record.CoverageGains = append(record.CoverageGains, gain)
		} else {
			record.CoverageGains = append(record.CoverageGains, entry.Coverage.CoverageScore)
		}

		// Keep only recent history
		if len(record.CoverageGains) > 10 {
			record.CoverageGains = record.CoverageGains[len(record.CoverageGains)-10:]
		}
	}
}

// adaptWeights adjusts weights based on performance
func (s *WeightedSelectionStrategy) adaptWeights() {
	// Calculate effectiveness of each component
	effectiveness := s.calculateComponentEffectiveness()

	// Adjust weights based on effectiveness
	totalEffectiveness := 0.0
	for _, eff := range effectiveness {
		totalEffectiveness += eff
	}

	if totalEffectiveness > 0 {
		// Normalize and update weights
		s.weightFactors.CoverageWeight = effectiveness["coverage"] / totalEffectiveness
		s.weightFactors.AgeWeight = effectiveness["age"] / totalEffectiveness
		s.weightFactors.ExecutionWeight = effectiveness["execution"] / totalEffectiveness
		s.weightFactors.GenerationWeight = effectiveness["generation"] / totalEffectiveness
		s.weightFactors.SizeWeight = effectiveness["size"] / totalEffectiveness
	}
}

// calculateComponentEffectiveness estimates how effective each component is
func (s *WeightedSelectionStrategy) calculateComponentEffectiveness() map[string]float64 {
	effectiveness := map[string]float64{
		"coverage":   1.0,
		"age":        1.0,
		"execution":  1.0,
		"generation": 1.0,
		"size":       1.0,
	}

	// Analyze performance data to estimate effectiveness
	for _, record := range s.performanceData {
		if record.SelectionCount > 0 && len(record.CoverageGains) > 1 {
			// Calculate average coverage gain
			avgGain := 0.0
			for i := 1; i < len(record.CoverageGains); i++ {
				avgGain += record.CoverageGains[i] - record.CoverageGains[i-1]
			}
			avgGain /= float64(len(record.CoverageGains) - 1)

			// Boost effectiveness of components that contributed to high gains
			if avgGain > 0 {
				effectiveness["coverage"] += avgGain * 2.0
			}
		}
	}

	return effectiveness
}

// GetMetrics returns metrics for this strategy
func (s *WeightedSelectionStrategy) GetMetrics() *StrategyMetrics {
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

// GetWeightFactors returns the current weight factors
func (s *WeightedSelectionStrategy) GetWeightFactors() WeightFactors {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.weightFactors
}

// SetWeightFactors updates the weight factors
func (s *WeightedSelectionStrategy) SetWeightFactors(factors WeightFactors) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.weightFactors = factors
}
