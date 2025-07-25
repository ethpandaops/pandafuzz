package quarantine

import (
	"fmt"
	"strings"
	"time"

	crashtypes "github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
)

// Rules defines the quarantine rules and policies
type Rules struct {
	// Execution time thresholds
	MaxExecutionTime  time.Duration
	SlowExecutionTime time.Duration

	// Memory usage thresholds
	MaxMemoryUsage       int64
	ExcessiveMemoryUsage int64

	// Review and release policies
	MaxReviewCount      int
	MinQuarantinePeriod time.Duration
	MaxQuarantinePeriod time.Duration

	// Crash handling policies
	CriticalCrashTypes    []crashtypes.CrashType
	AutoQuarantineCrashes bool

	// Failure thresholds
	MaxConsecutiveFailures int
	FailureWindowDuration  time.Duration
}

// DefaultRules returns the default quarantine rules
func DefaultRules() *Rules {
	return &Rules{
		MaxExecutionTime:      30 * time.Second,
		SlowExecutionTime:     10 * time.Second,
		MaxMemoryUsage:        1 << 30,   // 1GB
		ExcessiveMemoryUsage:  512 << 20, // 512MB
		MaxReviewCount:        3,
		MinQuarantinePeriod:   24 * time.Hour,
		MaxQuarantinePeriod:   30 * 24 * time.Hour, // 30 days
		AutoQuarantineCrashes: true,
		CriticalCrashTypes: []crashtypes.CrashType{
			crashtypes.CrashTypeHeapOverflow,
			crashtypes.CrashTypeStackOverflow,
			crashtypes.CrashTypeSegmentationFault,
		},
		MaxConsecutiveFailures: 5,
		FailureWindowDuration:  1 * time.Hour,
	}
}

// NewRules creates a new Rules instance with custom configuration
func NewRules(config RulesConfig) *Rules {
	rules := DefaultRules()

	// Apply custom configuration
	if config.MaxExecutionTime > 0 {
		rules.MaxExecutionTime = config.MaxExecutionTime
	}
	if config.SlowExecutionTime > 0 {
		rules.SlowExecutionTime = config.SlowExecutionTime
	}
	if config.MaxMemoryUsage > 0 {
		rules.MaxMemoryUsage = config.MaxMemoryUsage
	}
	if config.ExcessiveMemoryUsage > 0 {
		rules.ExcessiveMemoryUsage = config.ExcessiveMemoryUsage
	}
	if config.MaxReviewCount > 0 {
		rules.MaxReviewCount = config.MaxReviewCount
	}
	if config.MinQuarantinePeriod > 0 {
		rules.MinQuarantinePeriod = config.MinQuarantinePeriod
	}
	if config.MaxQuarantinePeriod > 0 {
		rules.MaxQuarantinePeriod = config.MaxQuarantinePeriod
	}
	if len(config.CriticalCrashTypes) > 0 {
		rules.CriticalCrashTypes = config.CriticalCrashTypes
	}
	rules.AutoQuarantineCrashes = config.AutoQuarantineCrashes
	if config.MaxConsecutiveFailures > 0 {
		rules.MaxConsecutiveFailures = config.MaxConsecutiveFailures
	}
	if config.FailureWindowDuration > 0 {
		rules.FailureWindowDuration = config.FailureWindowDuration
	}

	return rules
}

// RulesConfig represents the configuration for quarantine rules
type RulesConfig struct {
	MaxExecutionTime       time.Duration
	SlowExecutionTime      time.Duration
	MaxMemoryUsage         int64
	ExcessiveMemoryUsage   int64
	MaxReviewCount         int
	MinQuarantinePeriod    time.Duration
	MaxQuarantinePeriod    time.Duration
	CriticalCrashTypes     []crashtypes.CrashType
	AutoQuarantineCrashes  bool
	MaxConsecutiveFailures int
	FailureWindowDuration  time.Duration
}

// ShouldQuarantine determines if an execution result warrants quarantine
func (r *Rules) ShouldQuarantine(result ExecutionResult) (QuarantineReason, string) {
	// Check for crashes
	if result.Crashed && r.AutoQuarantineCrashes {
		if r.isCriticalCrash(result.CrashType) {
			return ReasonCrashCausing, fmt.Sprintf("Critical crash detected: %s - %s", result.CrashType, result.CrashSignature)
		}
		return ReasonCrashCausing, fmt.Sprintf("Crash detected: %s", result.CrashSignature)
	}

	// Check for timeout
	if result.TimedOut {
		return ReasonTimeout, fmt.Sprintf("Execution timed out after %s", result.ExecutionTime)
	}

	// Check for excessive execution time
	if result.ExecutionTime > r.MaxExecutionTime {
		return ReasonTimeout, fmt.Sprintf("Execution time exceeded maximum: %s > %s", result.ExecutionTime, r.MaxExecutionTime)
	}

	// Check for slow execution
	if result.ExecutionTime > r.SlowExecutionTime && !result.Success {
		return ReasonSlowExecution, fmt.Sprintf("Slow execution with failure: %s", result.ExecutionTime)
	}

	// Check for excessive memory usage
	if result.MemoryUsage > r.MaxMemoryUsage {
		return ReasonExcessiveMemory, fmt.Sprintf("Memory usage exceeded maximum: %d bytes > %d bytes", result.MemoryUsage, r.MaxMemoryUsage)
	}

	// Check for high memory usage with failure
	if result.MemoryUsage > r.ExcessiveMemoryUsage && !result.Success {
		return ReasonExcessiveMemory, fmt.Sprintf("High memory usage with failure: %d bytes", result.MemoryUsage)
	}

	// Check for general failure with error
	if !result.Success && result.Error != nil {
		errorStr := result.Error.Error()
		if strings.Contains(errorStr, "malformed") || strings.Contains(errorStr, "invalid") {
			return ReasonMalformed, fmt.Sprintf("Malformed input detected: %s", errorStr)
		}
		// Don't quarantine for all failures, only specific ones
	}

	return "", "" // No quarantine needed
}

// IsPermanentBanReason checks if a quarantine reason warrants permanent ban
func (r *Rules) IsPermanentBanReason(reason QuarantineReason) bool {
	// By default, no reasons lead to permanent ban
	// This can be customized based on security requirements
	return false
}

// CanRelease determines if a quarantined entry can be released
func (r *Rules) CanRelease(entry *QuarantineEntry) bool {
	// Cannot release if permanently banned
	if entry.PermanentBan {
		return false
	}

	// Check minimum quarantine period
	quarantineDuration := time.Since(entry.QuarantinedAt)
	if quarantineDuration < r.MinQuarantinePeriod {
		return false
	}

	// Check if it has been reviewed
	if entry.ReviewCount == 0 {
		return false // Must be reviewed at least once
	}

	// Check specific release criteria based on quarantine reason
	switch entry.Reason {
	case ReasonCrashCausing:
		// For crash-causing entries, require multiple reviews
		return entry.ReviewCount >= 2
	case ReasonTimeout, ReasonSlowExecution:
		// Timeout issues can be released after review
		return entry.ReviewCount >= 1
	case ReasonExcessiveMemory:
		// Memory issues require investigation
		return entry.ReviewCount >= 2
	case ReasonMalformed:
		// Malformed entries should be carefully reviewed
		return entry.ReviewCount >= 3
	case ReasonManualQuarantine:
		// Manual quarantine requires manual release (at least one review)
		return entry.ReviewCount >= 1
	case ReasonRepeatedFailures:
		// Repeated failures need thorough review
		return entry.ReviewCount >= 2
	default:
		return entry.ReviewCount >= 1
	}
}

// ShouldPermanentlyBan determines if an entry should be permanently banned
func (r *Rules) ShouldPermanentlyBan(entry *QuarantineEntry) bool {
	// Ban if exceeded maximum review count without successful release
	if entry.ReviewCount > r.MaxReviewCount {
		return true
	}

	// Ban critical crash-causing entries that repeatedly cause issues
	if entry.Reason == ReasonCrashCausing && entry.ReviewCount >= r.MaxReviewCount {
		for _, criticalType := range r.CriticalCrashTypes {
			if strings.Contains(entry.Details, string(criticalType)) {
				return true
			}
		}
	}

	// Ban entries that have been quarantined for too long
	if time.Since(entry.QuarantinedAt) > r.MaxQuarantinePeriod {
		return true
	}

	return false
}

// IsExpired checks if a quarantine entry has expired and should be auto-released
func (r *Rules) IsExpired(entry *QuarantineEntry, now time.Time) bool {
	// Permanently banned entries never expire
	if entry.PermanentBan {
		return false
	}

	// Check if past maximum quarantine period
	quarantineDuration := now.Sub(entry.QuarantinedAt)
	if quarantineDuration > r.MaxQuarantinePeriod {
		return true
	}

	// Auto-release timeout and slow execution issues after extended period
	if entry.Reason == ReasonTimeout || entry.Reason == ReasonSlowExecution {
		if quarantineDuration > 7*24*time.Hour { // 7 days
			return true
		}
	}

	return false
}

// isCriticalCrash checks if a crash type is considered critical
func (r *Rules) isCriticalCrash(crashType crashtypes.CrashType) bool {
	for _, critical := range r.CriticalCrashTypes {
		if crashType == critical {
			return true
		}
	}
	return false
}

// ValidateExecutionResult validates an execution result for consistency
func (r *Rules) ValidateExecutionResult(result ExecutionResult) error {
	if result.EntryID == "" {
		return fmt.Errorf("execution result must have entry ID")
	}

	// If crashed, should have crash signature
	if result.Crashed && result.CrashSignature == "" {
		return fmt.Errorf("crashed execution must have crash signature")
	}

	// If timed out, execution time should reflect that
	if result.TimedOut && result.ExecutionTime < r.SlowExecutionTime {
		return fmt.Errorf("timed out execution should have longer execution time")
	}

	// Memory usage should be reasonable
	if result.MemoryUsage < 0 {
		return fmt.Errorf("memory usage cannot be negative")
	}

	// Success and failure states should be consistent
	if result.Success && (result.Crashed || result.TimedOut) {
		return fmt.Errorf("successful execution cannot have crashed or timed out")
	}

	return nil
}

// GetQuarantineRecommendation provides a recommendation for quarantine action
func (r *Rules) GetQuarantineRecommendation(result ExecutionResult) string {
	reason, details := r.ShouldQuarantine(result)
	if reason == "" {
		return "No quarantine recommended"
	}

	recommendation := fmt.Sprintf("Quarantine recommended: %s - %s", reason, details)

	// Add specific recommendations based on reason
	switch reason {
	case ReasonCrashCausing:
		if r.isCriticalCrash(result.CrashType) {
			recommendation += ". This is a critical crash type requiring immediate attention."
		}
	case ReasonTimeout:
		recommendation += fmt.Sprintf(". Consider adjusting timeout threshold (current: %s).", r.MaxExecutionTime)
	case ReasonExcessiveMemory:
		recommendation += fmt.Sprintf(". Memory limit exceeded (limit: %d bytes).", r.MaxMemoryUsage)
	case ReasonSlowExecution:
		recommendation += ". This entry consistently causes slow execution."
	case ReasonMalformed:
		recommendation += ". Input appears to be malformed or invalid."
	}

	return recommendation
}

// GetReleaseRequirements returns the requirements for releasing a quarantined entry
func (r *Rules) GetReleaseRequirements(entry *QuarantineEntry) []string {
	requirements := []string{}

	// Check permanent ban
	if entry.PermanentBan {
		requirements = append(requirements, "Entry is permanently banned and cannot be released")
		return requirements
	}

	// Check minimum quarantine period
	quarantineDuration := time.Since(entry.QuarantinedAt)
	if quarantineDuration < r.MinQuarantinePeriod {
		remaining := r.MinQuarantinePeriod - quarantineDuration
		requirements = append(requirements, fmt.Sprintf("Must wait %s more (minimum quarantine period: %s)", remaining, r.MinQuarantinePeriod))
	}

	// Check review requirements
	requiredReviews := r.getRequiredReviewCount(entry.Reason)
	if entry.ReviewCount < requiredReviews {
		requirements = append(requirements, fmt.Sprintf("Needs %d more review(s) (current: %d, required: %d)", requiredReviews-entry.ReviewCount, entry.ReviewCount, requiredReviews))
	}

	// Add reason-specific requirements
	switch entry.Reason {
	case ReasonCrashCausing:
		requirements = append(requirements, "Verify crash has been fixed or is non-exploitable")
	case ReasonExcessiveMemory:
		requirements = append(requirements, "Confirm memory usage is within acceptable limits")
	case ReasonMalformed:
		requirements = append(requirements, "Validate input format and ensure proper handling")
	}

	if len(requirements) == 0 {
		requirements = append(requirements, "Entry meets all release requirements")
	}

	return requirements
}

// getRequiredReviewCount returns the required number of reviews for a quarantine reason
func (r *Rules) getRequiredReviewCount(reason QuarantineReason) int {
	switch reason {
	case ReasonCrashCausing:
		return 2
	case ReasonTimeout, ReasonSlowExecution:
		return 1
	case ReasonExcessiveMemory:
		return 2
	case ReasonMalformed:
		return 3
	case ReasonManualQuarantine:
		return 1
	case ReasonRepeatedFailures:
		return 2
	default:
		return 1
	}
}
