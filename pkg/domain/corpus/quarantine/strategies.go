package quarantine

import (
	"time"
)

// EvaluationStrategy defines the interface for corpus entry evaluation strategies
type EvaluationStrategy interface {
	// ShouldQuarantine evaluates if an entry should be quarantined based on the strategy
	ShouldQuarantine(result ExecutionResult, history []ExecutionResult) (bool, QuarantineReason, string)
	// Priority returns the priority of this strategy (higher values = higher priority)
	Priority() int
}

// ConsecutiveFailureStrategy quarantines entries that fail consecutively
type ConsecutiveFailureStrategy struct {
	MaxFailures int
	Window      time.Duration
}

// ShouldQuarantine checks for consecutive failures
func (s *ConsecutiveFailureStrategy) ShouldQuarantine(result ExecutionResult, history []ExecutionResult) (bool, QuarantineReason, string) {
	if result.Success {
		return false, "", ""
	}

	// Count consecutive failures within the window
	consecutiveFailures := 1 // Current failure
	cutoff := time.Now().Add(-s.Window)

	for i := len(history) - 1; i >= 0; i-- {
		if history[i].EntryID != result.EntryID {
			continue
		}

		// Check if within time window
		if i < len(history) && i >= 0 {
			// In a real implementation, you'd have timestamps in history
			if !history[i].Success {
				consecutiveFailures++
			} else {
				break // Success breaks the consecutive chain
			}
		}
	}

	if consecutiveFailures >= s.MaxFailures {
		return true, ReasonRepeatedFailures,
			"Entry has failed " + string(rune(consecutiveFailures)) + " consecutive times"
	}

	return false, "", ""
}

// Priority returns the priority of this strategy
func (s *ConsecutiveFailureStrategy) Priority() int {
	return 50
}

// ResourcePatternStrategy identifies entries with problematic resource usage patterns
type ResourcePatternStrategy struct {
	MemoryGrowthThreshold float64 // Percentage growth between executions
	TimeGrowthThreshold   float64 // Percentage growth in execution time
}

// ShouldQuarantine checks for problematic resource patterns
func (s *ResourcePatternStrategy) ShouldQuarantine(result ExecutionResult, history []ExecutionResult) (bool, QuarantineReason, string) {
	if len(history) < 2 {
		return false, "", "" // Need history to detect patterns
	}

	// Find previous executions of the same entry
	var prevMemory int64
	var prevTime time.Duration
	foundPrevious := false

	for i := len(history) - 1; i >= 0; i-- {
		if history[i].EntryID == result.EntryID {
			prevMemory = history[i].MemoryUsage
			prevTime = history[i].ExecutionTime
			foundPrevious = true
			break
		}
	}

	if !foundPrevious {
		return false, "", ""
	}

	// Check memory growth
	if prevMemory > 0 && result.MemoryUsage > 0 {
		memoryGrowth := float64(result.MemoryUsage-prevMemory) / float64(prevMemory)
		if memoryGrowth > s.MemoryGrowthThreshold {
			return true, ReasonExcessiveMemory,
				"Memory usage growing rapidly: " + string(rune(int(memoryGrowth*100))) + "% increase"
		}
	}

	// Check execution time growth
	if prevTime > 0 && result.ExecutionTime > 0 {
		timeGrowth := float64(result.ExecutionTime-prevTime) / float64(prevTime)
		if timeGrowth > s.TimeGrowthThreshold {
			return true, ReasonSlowExecution,
				"Execution time growing rapidly: " + string(rune(int(timeGrowth*100))) + "% increase"
		}
	}

	return false, "", ""
}

// Priority returns the priority of this strategy
func (s *ResourcePatternStrategy) Priority() int {
	return 30
}

// StatisticalAnomalyStrategy uses statistical analysis to detect anomalies
type StatisticalAnomalyStrategy struct {
	StdDevMultiplier float64 // Number of standard deviations for anomaly detection
}

// ShouldQuarantine checks for statistical anomalies
func (s *StatisticalAnomalyStrategy) ShouldQuarantine(result ExecutionResult, history []ExecutionResult) (bool, QuarantineReason, string) {
	if len(history) < 10 {
		return false, "", "" // Need sufficient data for statistics
	}

	// Calculate statistics for execution time
	var sum time.Duration
	var count int

	for _, h := range history {
		if h.Success && !h.Crashed && !h.TimedOut {
			sum += h.ExecutionTime
			count++
		}
	}

	if count == 0 {
		return false, "", ""
	}

	mean := sum / time.Duration(count)

	// Calculate standard deviation
	var varianceSum float64
	for _, h := range history {
		if h.Success && !h.Crashed && !h.TimedOut {
			diff := float64(h.ExecutionTime - mean)
			varianceSum += diff * diff
		}
	}

	stdDev := time.Duration(float64(varianceSum) / float64(count))
	threshold := mean + time.Duration(float64(stdDev)*s.StdDevMultiplier)

	if result.ExecutionTime > threshold {
		return true, ReasonSlowExecution,
			"Execution time is a statistical anomaly: " + result.ExecutionTime.String() +
				" (threshold: " + threshold.String() + ")"
	}

	return false, "", ""
}

// Priority returns the priority of this strategy
func (s *StatisticalAnomalyStrategy) Priority() int {
	return 20
}

// CompositeStrategy combines multiple strategies
type CompositeStrategy struct {
	strategies []EvaluationStrategy
}

// NewCompositeStrategy creates a new composite strategy
func NewCompositeStrategy(strategies ...EvaluationStrategy) *CompositeStrategy {
	return &CompositeStrategy{
		strategies: strategies,
	}
}

// ShouldQuarantine evaluates all strategies and returns the highest priority match
func (s *CompositeStrategy) ShouldQuarantine(result ExecutionResult, history []ExecutionResult) (bool, QuarantineReason, string) {
	var bestReason QuarantineReason
	var bestDetails string
	var bestPriority int
	shouldQuarantine := false

	for _, strategy := range s.strategies {
		if quarantine, reason, details := strategy.ShouldQuarantine(result, history); quarantine {
			if strategy.Priority() > bestPriority {
				shouldQuarantine = true
				bestReason = reason
				bestDetails = details
				bestPriority = strategy.Priority()
			}
		}
	}

	return shouldQuarantine, bestReason, bestDetails
}

// Priority returns the highest priority among all strategies
func (s *CompositeStrategy) Priority() int {
	maxPriority := 0
	for _, strategy := range s.strategies {
		if p := strategy.Priority(); p > maxPriority {
			maxPriority = p
		}
	}
	return maxPriority
}

// DefaultEvaluationStrategies returns the default set of evaluation strategies
func DefaultEvaluationStrategies() *CompositeStrategy {
	return NewCompositeStrategy(
		&ConsecutiveFailureStrategy{
			MaxFailures: 5,
			Window:      1 * time.Hour,
		},
		&ResourcePatternStrategy{
			MemoryGrowthThreshold: 0.5, // 50% growth
			TimeGrowthThreshold:   1.0, // 100% growth
		},
		&StatisticalAnomalyStrategy{
			StdDevMultiplier: 3.0, // 3 standard deviations
		},
	)
}
