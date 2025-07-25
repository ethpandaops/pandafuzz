package health

import (
	"context"
	"time"
)

// Status represents the health status of a component
type Status string

const (
	// StatusHealthy indicates the component is healthy
	StatusHealthy Status = "healthy"

	// StatusUnhealthy indicates the component is unhealthy
	StatusUnhealthy Status = "unhealthy"

	// StatusDegraded indicates the component is degraded but functional
	StatusDegraded Status = "degraded"
)

// CheckResult represents the result of a health check
type CheckResult struct {
	// Name is the name of the health check
	Name string `json:"name"`

	// Status is the health status
	Status Status `json:"status"`

	// Message provides additional information about the status
	Message string `json:"message,omitempty"`

	// Error contains any error encountered during the check
	Error error `json:"error,omitempty"`

	// Duration is how long the check took
	Duration time.Duration `json:"duration"`

	// Timestamp is when the check was performed
	Timestamp time.Time `json:"timestamp"`

	// Metadata contains additional check-specific information
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// HealthStatus represents the overall health status
type HealthStatus struct {
	// Status is the overall health status
	Status Status `json:"status"`

	// Checks contains the results of individual health checks
	Checks []CheckResult `json:"checks"`

	// Version is the application version
	Version string `json:"version,omitempty"`

	// Timestamp is when the health check was performed
	Timestamp time.Time `json:"timestamp"`

	// Duration is how long all checks took
	Duration time.Duration `json:"duration"`
}

// CheckFunc is a function that performs a health check
type CheckFunc func(ctx context.Context) error

// Check represents a health check
type Check interface {
	// Name returns the name of the health check
	Name() string

	// Check performs the health check
	Check(ctx context.Context) error
}

// CheckType represents the type of health check
type CheckType string

const (
	// Liveness checks if the application is running
	Liveness CheckType = "liveness"

	// Readiness checks if the application is ready to serve requests
	Readiness CheckType = "readiness"

	// Startup checks if the application has started successfully
	Startup CheckType = "startup"
)

// CheckOptions configures a health check
type CheckOptions struct {
	// Timeout is the maximum time allowed for the check
	Timeout time.Duration

	// Interval is how often to run the check (for periodic checks)
	Interval time.Duration

	// FailureThreshold is the number of consecutive failures before marking unhealthy
	FailureThreshold int

	// SuccessThreshold is the number of consecutive successes before marking healthy
	SuccessThreshold int

	// Critical indicates if this check failure should mark the entire service unhealthy
	Critical bool

	// Type indicates the type of health check
	Type CheckType
}

// DefaultCheckOptions returns default options for a health check
func DefaultCheckOptions() *CheckOptions {
	return &CheckOptions{
		Timeout:          5 * time.Second,
		Interval:         30 * time.Second,
		FailureThreshold: 3,
		SuccessThreshold: 1,
		Critical:         false,
		Type:             Readiness,
	}
}

// FuncCheck wraps a CheckFunc as a Check
type FuncCheck struct {
	name string
	fn   CheckFunc
}

// NewFuncCheck creates a new function-based health check
func NewFuncCheck(name string, fn CheckFunc) Check {
	return &FuncCheck{
		name: name,
		fn:   fn,
	}
}

// Name returns the name of the health check
func (f *FuncCheck) Name() string {
	return f.name
}

// Check performs the health check
func (f *FuncCheck) Check(ctx context.Context) error {
	return f.fn(ctx)
}

// CompositeCheck combines multiple checks
type CompositeCheck struct {
	name   string
	checks []Check
}

// NewCompositeCheck creates a new composite health check
func NewCompositeCheck(name string, checks ...Check) Check {
	return &CompositeCheck{
		name:   name,
		checks: checks,
	}
}

// Name returns the name of the composite check
func (c *CompositeCheck) Name() string {
	return c.name
}

// Check performs all sub-checks
func (c *CompositeCheck) Check(ctx context.Context) error {
	for _, check := range c.checks {
		if err := check.Check(ctx); err != nil {
			return err
		}
	}
	return nil
}

// CheckerOptions configures the health checker
type CheckerOptions struct {
	// DefaultTimeout is the default timeout for checks without explicit timeout
	DefaultTimeout time.Duration

	// MaxConcurrentChecks limits concurrent health check execution
	MaxConcurrentChecks int

	// EnableMetrics enables metrics collection for health checks
	EnableMetrics bool

	// CacheDuration is how long to cache health check results
	CacheDuration time.Duration

	// Logger for health check events
	Logger Logger
}

// DefaultCheckerOptions returns default options for the health checker
func DefaultCheckerOptions() *CheckerOptions {
	return &CheckerOptions{
		DefaultTimeout:      5 * time.Second,
		MaxConcurrentChecks: 10,
		EnableMetrics:       true,
		CacheDuration:       5 * time.Second,
	}
}

// Logger interface for health check logging
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}
