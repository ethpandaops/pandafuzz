package strategies

import (
	"context"

	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
)

// SelectionStrategy defines the interface for corpus entry selection strategies
type SelectionStrategy interface {
	// Name returns the strategy name
	Name() string

	// Select returns a batch of corpus entries based on the strategy
	// The entries are selected from the provided collection
	Select(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error)

	// Priority computes priority score for a corpus entry
	// Higher scores indicate higher priority for selection
	Priority(entry *types.CorpusEntry) float64

	// SupportsCriteria indicates if the strategy supports custom selection criteria
	SupportsCriteria() bool

	// Reset resets any internal state of the strategy
	Reset()
}

// SelectionOptions configures selection behavior
type SelectionOptions struct {
	// MinCoverage specifies minimum coverage score for selection
	MinCoverage float64

	// MaxAge specifies maximum age for entries (in seconds)
	MaxAge int64

	// PreferInteresting prioritizes entries marked as interesting
	PreferInteresting bool

	// ExcludeExecuted excludes recently executed entries
	ExcludeExecuted bool

	// ExcludeWindow time window in seconds for execution exclusion
	ExcludeWindow int64

	// Tags filters entries by tags (empty means no filter)
	Tags []string

	// WeightFactors customizes weight calculation
	WeightFactors WeightFactors

	// Seed for deterministic selection (0 for random)
	Seed int64
}

// WeightFactors defines weights for different selection criteria
type WeightFactors struct {
	// CoverageWeight weight for coverage score (0.0 - 1.0)
	CoverageWeight float64

	// AgeWeight weight for entry age (0.0 - 1.0)
	AgeWeight float64

	// ExecutionWeight weight for execution count (0.0 - 1.0)
	ExecutionWeight float64

	// GenerationWeight weight for mutation generation (0.0 - 1.0)
	GenerationWeight float64

	// SizeWeight weight for input size (0.0 - 1.0)
	SizeWeight float64
}

// DefaultWeightFactors returns balanced weight factors
func DefaultWeightFactors() WeightFactors {
	return WeightFactors{
		CoverageWeight:   0.4,
		AgeWeight:        0.2,
		ExecutionWeight:  0.2,
		GenerationWeight: 0.1,
		SizeWeight:       0.1,
	}
}

// SelectionMetrics tracks selection effectiveness
type SelectionMetrics struct {
	// TotalSelections total number of selections made
	TotalSelections uint64

	// UniqueSelections number of unique entries selected
	UniqueSelections uint64

	// CoverageImprovement coverage gained through selections
	CoverageImprovement float64

	// SelectionDistribution distribution of selections across entries
	SelectionDistribution map[string]uint64

	// StrategyPerformance performance metrics per strategy
	StrategyPerformance map[string]*StrategyMetrics
}

// StrategyMetrics tracks metrics for a specific strategy
type StrategyMetrics struct {
	// Name of the strategy
	Name string

	// SelectionCount number of times this strategy was used
	SelectionCount uint64

	// AveragePriority average priority score of selected entries
	AveragePriority float64

	// CoverageGained coverage gained through this strategy
	CoverageGained float64

	// ExecutionTime average execution time in milliseconds
	ExecutionTime float64
}

// SelectionResult contains the result of a selection operation
type SelectionResult struct {
	// Selected entries
	Entries []*types.CorpusEntry

	// Metrics for this selection
	Metrics *StrategyMetrics

	// Reason for each selection
	Reasons map[string]string
}
