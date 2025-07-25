# Corpus Selection Strategies

This package implements corpus entry selection strategies for fuzzing campaigns, following domain-driven design principles with a pluggable architecture.

## Architecture Overview

### Main Components

1. **Selector Service** (`selector.go`)
   - Main orchestration service
   - Strategy registration and management
   - Caching layer for performance
   - Metrics collection

2. **Strategy Interface** (`strategies/interface.go`)
   - Common interface for all selection strategies
   - Support for custom selection criteria
   - Priority calculation
   - Batch selection

3. **Selection Strategies**
   - Random Selection (`strategies/random.go`)
     - Uniform random
     - Weighted random
     - Reservoir sampling
   - Coverage-Based Selection (`strategies/coverage.go`)
     - Coverage score optimization
     - Incremental coverage focus
     - Rare code path targeting
   - Weighted Selection (`strategies/weighted.go`)
     - Multi-factor scoring
     - Adaptive weight adjustment
     - Diversity optimization
   - Priority Queue Selection (`strategies/priority.go`)
     - Dynamic priority adjustment
     - Multi-queue management
     - Category-based selection
   - Composite Selection (`strategies/composite.go`)
     - Strategy combination
     - Performance tracking
     - Consensus selection

## Usage Example

```go
// Create selector with default configuration
cfg := selection.DefaultConfig()
selector, err := selection.NewSelector(cfg, logger, entryRepo, collectionRepo)
if err != nil {
    return err
}

// Start the selector service
if err := selector.Start(ctx); err != nil {
    return err
}
defer selector.Stop()

// Select entries using default strategy
entries, err := selector.Select(ctx, 100)

// Select using specific strategy
entries, err := selector.SelectWithStrategy(ctx, "coverage-based", 50)

// Select from specific collection
entries, err := selector.SelectFromCollection(ctx, "interesting-inputs", 25)
```

## Available Strategies

### Random Selection
- **uniform-random**: Equal probability for all entries
- **weighted-random**: Probability based on configurable weights
- **reservoir-sampling**: Efficient streaming selection

### Coverage-Based Selection
- **coverage-based**: Prioritizes entries with high code coverage
- **incremental-coverage**: Focuses on entries showing coverage growth
- **rare-coverage**: Targets entries covering rarely-hit code paths

### Weighted Selection
- **weighted**: Multi-factor scoring with fixed weights
- **adaptive-weighted**: Dynamically adjusts weights based on performance

### Priority Queue Selection
- **priority-queue**: Maintains sorted queue by priority
- **dynamic-priority-queue**: Adjusts priorities after each selection
- **multi-queue**: Separate queues for different entry categories

### Composite Selection
- **composite-rotation**: Rotates through strategies
- **composite-weighted**: Random strategy selection based on weights
- **composite-best**: Uses best performing strategy
- **composite-consensus**: Combines results from multiple strategies
- **composite-hybrid**: Different strategies for different selection sizes

## Selection Options

```go
type SelectionOptions struct {
    MinCoverage       float64      // Minimum coverage score
    MaxAge            int64        // Maximum entry age in seconds
    PreferInteresting bool         // Prioritize entries with new coverage
    ExcludeExecuted   bool         // Skip recently executed entries
    ExcludeWindow     int64        // Window for execution exclusion
    Tags              []string     // Filter by tags
    WeightFactors     WeightFactors // Custom weight factors
    Seed              int64        // For deterministic selection
}
```

## Custom Strategy Implementation

```go
type MyStrategy struct {
    // fields
}

func (s *MyStrategy) Name() string {
    return "my-strategy"
}

func (s *MyStrategy) Select(
    ctx context.Context, 
    collection []*types.CorpusEntry, 
    count int, 
    options SelectionOptions,
) ([]*types.CorpusEntry, error) {
    // Implementation
}

func (s *MyStrategy) Priority(entry *types.CorpusEntry) float64 {
    // Calculate priority score
}

func (s *MyStrategy) SupportsCriteria() bool {
    return true
}

func (s *MyStrategy) Reset() {
    // Reset internal state
}

// Register the strategy
selector.RegisterStrategy(myStrategy)
```

## Metrics

The selector tracks various performance metrics:

- Total selections made
- Unique entries selected
- Coverage improvement
- Selection distribution across entries
- Per-strategy performance metrics

Access metrics using:
```go
metrics := selector.GetMetrics()
```

## Features

- **Pluggable Architecture**: Easy to add new selection strategies
- **Performance Optimization**: Built-in caching and batch operations
- **Concurrent Safe**: Thread-safe implementation with proper locking
- **Metrics Collection**: Comprehensive performance tracking
- **Flexible Configuration**: Customizable selection criteria and options
- **Domain-Driven Design**: Clean separation of concerns
- **Testable**: Comprehensive test coverage with mocks