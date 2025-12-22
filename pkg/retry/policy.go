package retry

import "time"

// Policy defines the retry behavior for operations
type Policy struct {
	MaxRetries      int           `json:"max_retries" yaml:"max_retries"`
	InitialDelay    time.Duration `json:"initial_delay" yaml:"initial_delay"`
	MaxDelay        time.Duration `json:"max_delay" yaml:"max_delay"`
	Multiplier      float64       `json:"multiplier" yaml:"multiplier"`
	Jitter          bool          `json:"jitter" yaml:"jitter"`
	RetryableErrors []string      `json:"retryable_errors" yaml:"retryable_errors"`
}

// Default retry policies for common scenarios
var (
	// DefaultPolicy for general operations
	DefaultPolicy = Policy{
		MaxRetries:   3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
	}

	// DatabasePolicy for database operations
	DatabasePolicy = Policy{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   1.5,
		Jitter:       true,
		RetryableErrors: []string{
			"database is locked",
			"timeout",
			"temporary failure",
			"disk i/o error",
		},
	}

	// NetworkPolicy for network operations
	NetworkPolicy = Policy{
		MaxRetries:   3,
		InitialDelay: 2 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
		RetryableErrors: []string{
			"connection refused",
			"timeout",
			"network unreachable",
			"no route to host",
			"temporary failure",
		},
	}

	// UpdatePolicy for system updates (longer delays)
	UpdatePolicy = Policy{
		MaxRetries:   20,
		InitialDelay: 5 * time.Second,
		MaxDelay:     60 * time.Second,
		Multiplier:   1.5,
		Jitter:       true,
	}
)

// Validate ensures the policy has valid values
func (p Policy) Validate() error {
	if p.MaxRetries < 0 {
		return &PolicyError{Field: "MaxRetries", Message: "cannot be negative"}
	}
	if p.InitialDelay < 0 {
		return &PolicyError{Field: "InitialDelay", Message: "cannot be negative"}
	}
	if p.MaxDelay < 0 {
		return &PolicyError{Field: "MaxDelay", Message: "cannot be negative"}
	}
	if p.Multiplier < 1.0 {
		return &PolicyError{Field: "Multiplier", Message: "must be at least 1.0"}
	}
	if p.MaxDelay > 0 && p.InitialDelay > p.MaxDelay {
		return &PolicyError{Field: "InitialDelay", Message: "cannot be greater than MaxDelay"}
	}
	return nil
}

// PolicyError represents a validation error in a retry policy
type PolicyError struct {
	Field   string
	Message string
}

// Error implements the error interface
func (e *PolicyError) Error() string {
	return "retry policy error: " + e.Field + " " + e.Message
}
