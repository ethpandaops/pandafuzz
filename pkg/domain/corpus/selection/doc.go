// Package selection provides corpus entry selection strategies for fuzzing.
//
// The selection package implements various strategies for selecting corpus entries
// during fuzzing campaigns. It follows domain-driven design principles and provides
// a pluggable architecture for different selection algorithms.
//
// # Architecture
//
// The package consists of:
//   - Selector: Main service that orchestrates selection strategies
//   - Strategies: Pluggable selection algorithms (random, coverage-based, weighted)
//   - Metrics: Performance tracking and effectiveness measurement
//   - Cache: Entry caching for improved performance
//
// # Available Strategies
//
// Random Selection:
//   - uniform-random: Equal probability selection
//   - weighted-random: Probability based on weights
//   - reservoir-sampling: Streaming selection algorithm
//
// Coverage-Based Selection:
//   - coverage-based: Prioritizes high coverage entries
//   - incremental-coverage: Focuses on coverage growth
//   - rare-coverage: Targets rarely-hit code paths
//
// Weighted Selection:
//   - weighted: Multi-factor scoring with fixed weights
//   - adaptive-weighted: Dynamically adjusts weights based on performance
//
// Priority Queue Selection:
//   - priority-queue: Maintains sorted queue by priority
//   - dynamic-priority-queue: Adjusts priorities after selection
//   - multi-queue: Multiple queues for different entry categories
//
// # Usage Example
//
//	cfg := selection.DefaultConfig()
//	selector, err := selection.NewSelector(cfg, logger, entryRepo, collectionRepo)
//	if err != nil {
//	    return err
//	}
//
//	// Start the selector
//	if err := selector.Start(ctx); err != nil {
//	    return err
//	}
//	defer selector.Stop()
//
//	// Select entries using default strategy
//	entries, err := selector.Select(ctx, 100)
//	if err != nil {
//	    return err
//	}
//
//	// Select using specific strategy
//	entries, err := selector.SelectWithStrategy(ctx, "coverage-based", 50)
//	if err != nil {
//	    return err
//	}
//
// # Custom Strategies
//
// To implement a custom selection strategy:
//
//	type MyStrategy struct {
//	    // fields
//	}
//
//	func (s *MyStrategy) Name() string {
//	    return "my-strategy"
//	}
//
//	func (s *MyStrategy) Select(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error) {
//	    // Implementation
//	}
//
//	func (s *MyStrategy) Priority(entry *types.CorpusEntry) float64 {
//	    // Calculate priority
//	}
//
//	func (s *MyStrategy) SupportsCriteria() bool {
//	    return true
//	}
//
//	func (s *MyStrategy) Reset() {
//	    // Reset state
//	}
//
// Then register it:
//
//	selector.RegisterStrategy(myStrategy)
//
// # Selection Options
//
// Selection behavior can be customized through SelectionOptions:
//   - MinCoverage: Minimum coverage score threshold
//   - MaxAge: Maximum entry age in seconds
//   - PreferInteresting: Prioritize entries with new coverage
//   - ExcludeExecuted: Skip recently executed entries
//   - Tags: Filter by specific tags
//   - WeightFactors: Customize scoring weights
//
// # Metrics
//
// The selector tracks various metrics:
//   - Total selections made
//   - Unique entries selected
//   - Coverage improvement
//   - Selection distribution
//   - Per-strategy performance
//
// Access metrics:
//
//	metrics := selector.GetMetrics()
//	fmt.Printf("Total selections: %d\n", metrics.TotalSelections)
//	fmt.Printf("Coverage improvement: %.2f%%\n", metrics.CoverageImprovement*100)
package selection
