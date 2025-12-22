// Package retry provides retry logic and circuit breaker implementations.
//
// This package offers resilient execution patterns for operations that may
// fail transiently. It includes exponential backoff retry, circuit breaker
// for fault isolation, and a resilient client combining both patterns.
//
// # Retry Manager
//
// The Manager handles automatic retry with exponential backoff:
//
//	manager := retry.NewManager(retry.Policy{
//	    MaxRetries:   3,
//	    InitialDelay: time.Second,
//	    MaxDelay:     30 * time.Second,
//	    Multiplier:   2.0,
//	})
//
//	err := manager.Execute(func() error {
//	    return someOperation()
//	})
//
// # Context Support
//
// For operations that should respect cancellation:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
//	defer cancel()
//
//	err := manager.ExecuteWithContext(ctx, func() error {
//	    return someOperation()
//	})
//
// # Retry Policies
//
// Pre-configured policies are available for common scenarios:
//
//	// Default policy for general operations
//	policy := retry.DefaultPolicy()
//
//	// Database-specific policy with more retries
//	policy := retry.DatabasePolicy()
//
//	// Network operations with shorter delays
//	policy := retry.NetworkPolicy()
//
// # Circuit Breaker
//
// The CircuitBreaker prevents cascading failures by opening after repeated
// errors and periodically attempting recovery:
//
//	cb := retry.NewCircuitBreaker(retry.CircuitBreakerConfig{
//	    MaxFailures:  5,
//	    ResetTimeout: 60 * time.Second,
//	})
//
//	err := cb.Execute(func() error {
//	    return riskyOperation()
//	})
//
// Circuit states:
//
//   - Closed: Normal operation, requests pass through
//   - Open: All requests fail immediately, no execution
//   - Half-Open: Single test request allowed, success closes circuit
//
// # Resilient Client
//
// Combine retry and circuit breaker for maximum resilience:
//
//	client := retry.NewResilientClient(
//	    retry.DefaultPolicy(),
//	    retry.CircuitBreakerConfig{
//	        MaxFailures:  5,
//	        ResetTimeout: time.Minute,
//	    },
//	)
//
//	err := client.Execute(func() error {
//	    return callExternalService()
//	})
//
// # Error Classification
//
// The package provides functions to classify errors for retry decisions:
//
//	if retry.IsNetworkError(err) {
//	    // Network errors are typically retryable
//	}
//
//	if retry.IsTimeoutError(err) {
//	    // Timeout may indicate overload
//	}
//
// # Thread Safety
//
// All types in this package are safe for concurrent use from multiple
// goroutines.
package retry
