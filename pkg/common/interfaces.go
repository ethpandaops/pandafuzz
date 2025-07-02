package common

import (
	"context"
	"time"
)

// ResourceMonitor provides system resource monitoring capabilities
type ResourceMonitor interface {
	// GetMetrics returns current resource usage metrics
	GetMetrics(ctx context.Context) (*ResourceMetrics, error)

	// StartMonitoring begins continuous resource monitoring
	// The interval parameter specifies how often to collect metrics
	StartMonitoring(ctx context.Context, interval time.Duration) error

	// SetAlertThresholds configures alert thresholds for resource usage
	// The thresholds map uses metric names as keys (e.g., "cpu", "memory", "disk")
	// and threshold values as percentages or absolute values
	SetAlertThresholds(thresholds map[string]float64) error
}

// CleanupManager handles resource cleanup based on defined policies
type CleanupManager interface {
	// SetPolicy updates the cleanup policy
	SetPolicy(policy CleanupPolicy) error

	// RunCleanup executes cleanup based on current policy
	// Returns the number of items cleaned and any error encountered
	RunCleanup(ctx context.Context) (int, error)

	// ScheduleCleanup schedules periodic cleanup based on policy interval
	// Cleanup will run at the interval specified in the policy
	ScheduleCleanup(ctx context.Context) error
}

// FuzzerEventHandler processes fuzzer events
type FuzzerEventHandler interface {
	// HandleEvent processes a fuzzer event
	// Implementations should handle events asynchronously when appropriate
	HandleEvent(ctx context.Context, event FuzzerEvent) error
}
