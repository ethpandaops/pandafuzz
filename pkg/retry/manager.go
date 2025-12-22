// Package retry provides exponential backoff retry logic and circuit breaker
// patterns for resilient operation execution in the PandaFuzz system.
package retry

import (
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Manager handles exponential backoff retry logic
type Manager struct {
	policy Policy
	random *rand.Rand
}

// NewManager creates a new retry manager with the given policy
func NewManager(policy Policy) *Manager {
	return &Manager{
		policy: policy,
		random: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Execute runs the operation with exponential backoff retry logic
func (rm *Manager) Execute(operation func() error) error {
	var lastErr error
	delay := rm.policy.InitialDelay

	for attempt := 0; attempt <= rm.policy.MaxRetries; attempt++ {
		err := operation()
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if error is retryable
		if !rm.isRetryableError(err) {
			return fmt.Errorf("non-retryable error: %w", err)
		}

		// Don't wait after the last attempt
		if attempt == rm.policy.MaxRetries {
			break
		}

		// Calculate delay with optional jitter
		actualDelay := rm.calculateDelay(delay)
		time.Sleep(actualDelay)

		// Exponential backoff with max delay cap
		delay = time.Duration(float64(delay) * rm.policy.Multiplier)
		if delay > rm.policy.MaxDelay {
			delay = rm.policy.MaxDelay
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", rm.policy.MaxRetries+1, lastErr)
}

// ExecuteWithContext runs the operation with context support
func (rm *Manager) ExecuteWithContext(operation func() error, timeout time.Duration) error {
	done := make(chan error, 1)

	go func() {
		done <- rm.Execute(operation)
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("operation timed out after %v", timeout)
	}
}

// calculateDelay applies jitter to the base delay if configured
func (rm *Manager) calculateDelay(baseDelay time.Duration) time.Duration {
	if !rm.policy.Jitter {
		return baseDelay
	}

	// Add up to 10% jitter
	jitter := time.Duration(rm.random.Float64() * float64(baseDelay) * 0.1)
	return baseDelay + jitter
}

// isRetryableError determines if an error should trigger a retry
func (rm *Manager) isRetryableError(err error) bool {
	// If specific retryable errors are configured, use those
	if len(rm.policy.RetryableErrors) > 0 {
		errStr := err.Error()
		for _, retryable := range rm.policy.RetryableErrors {
			if strings.Contains(errStr, retryable) {
				return true
			}
		}
		return false
	}

	// Default retryable error detection
	return IsNetworkError(err) || IsTimeoutError(err) || IsTemporaryError(err) || IsDatabaseError(err)
}

// IsNetworkError checks if an error is a network error
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Check for network errors
	if e, ok := err.(net.Error); ok {
		return e.Temporary() || e.Timeout()
	}

	// Check for common network error strings
	errStr := strings.ToLower(err.Error())
	networkErrors := []string{
		"connection refused",
		"connection reset",
		"network unreachable",
		"no route to host",
		"host unreachable",
		"connection timed out",
		"temporary failure",
		"name resolution failed",
		"dns lookup failed",
	}

	for _, netErr := range networkErrors {
		if strings.Contains(errStr, netErr) {
			return true
		}
	}

	return false
}

// IsTimeoutError checks if an error is a timeout error
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// Check for timeout interface
	if e, ok := err.(net.Error); ok && e.Timeout() {
		return true
	}

	// Check for timeout strings
	errStr := strings.ToLower(err.Error())
	timeoutErrors := []string{
		"timeout",
		"timed out",
		"deadline exceeded",
		"context deadline exceeded",
		"i/o timeout",
	}

	for _, timeoutErr := range timeoutErrors {
		if strings.Contains(errStr, timeoutErr) {
			return true
		}
	}

	return false
}

// IsTemporaryError checks if an error is a temporary error
func IsTemporaryError(err error) bool {
	if err == nil {
		return false
	}

	// Check for temporary interface
	if e, ok := err.(interface{ Temporary() bool }); ok && e.Temporary() {
		return true
	}

	// Check for syscall errors that are temporary
	if e, ok := err.(syscall.Errno); ok {
		switch e {
		case syscall.ECONNRESET, syscall.ECONNREFUSED, syscall.EAGAIN:
			return true
		}
	}

	// Check for temporary error strings
	errStr := strings.ToLower(err.Error())
	temporaryErrors := []string{
		"temporary failure",
		"service unavailable",
		"try again",
		"resource temporarily unavailable",
		"operation would block",
	}

	for _, tempErr := range temporaryErrors {
		if strings.Contains(errStr, tempErr) {
			return true
		}
	}

	return false
}

// IsDatabaseError checks if an error is a database error
func IsDatabaseError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	databaseErrors := []string{
		"database is locked",
		"database disk image is malformed",
		"disk i/o error",
		"file is not a database",
		"unable to open database file",
		"sql: database is closed",
		"connection pool exhausted",
		"too many connections",
	}

	for _, dbErr := range databaseErrors {
		if strings.Contains(errStr, dbErr) {
			return true
		}
	}

	return false
}

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	// CircuitClosed allows all operations through
	CircuitClosed CircuitState = iota
	// CircuitOpen rejects all operations
	CircuitOpen
	// CircuitHalfOpen allows test operations through
	CircuitHalfOpen
)

// String returns the string representation of CircuitState
func (cs CircuitState) String() string {
	switch cs {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	maxFailures  int
	resetTimeout time.Duration
	state        CircuitState
	failures     int
	lastFailTime time.Time
	mu           sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        CircuitClosed,
	}
}

// Execute runs the operation through the circuit breaker
func (cb *CircuitBreaker) Execute(operation func() error) error {
	if !cb.canExecute() {
		return fmt.Errorf("circuit breaker is open")
	}

	err := operation()
	cb.recordResult(err)
	return err
}

// canExecute checks if the circuit breaker allows execution
func (cb *CircuitBreaker) canExecute() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.lastFailTime) >= cb.resetTimeout {
			// Move to half-open state
			cb.mu.RUnlock()
			cb.mu.Lock()
			if cb.state == CircuitOpen && time.Since(cb.lastFailTime) >= cb.resetTimeout {
				cb.state = CircuitHalfOpen
			}
			cb.mu.Unlock()
			cb.mu.RLock()
			return cb.state == CircuitHalfOpen
		}
		return false
	case CircuitHalfOpen:
		return true
	default:
		return false
	}
}

// recordResult updates the circuit breaker state based on the operation result
func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		cb.lastFailTime = time.Now()

		if cb.failures >= cb.maxFailures {
			cb.state = CircuitOpen
		}
	} else {
		// Success - reset failures and close circuit
		cb.failures = 0
		cb.state = CircuitClosed
	}
}

// GetState returns the current circuit breaker state
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetFailures returns the current failure count
func (cb *CircuitBreaker) GetFailures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}

// GetStats returns circuit breaker statistics
func (cb *CircuitBreaker) GetStats() map[string]any {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return map[string]any{
		"state":          cb.state.String(),
		"failures":       cb.failures,
		"max_failures":   cb.maxFailures,
		"reset_timeout":  cb.resetTimeout,
		"last_fail_time": cb.lastFailTime,
	}
}

// ResilientClient combines retry logic with circuit breaker
type ResilientClient struct {
	retryManager   *Manager
	circuitBreaker *CircuitBreaker
}

// NewResilientClient creates a new resilient client
func NewResilientClient(retryPolicy Policy, maxFailures int, resetTimeout time.Duration) *ResilientClient {
	return &ResilientClient{
		retryManager:   NewManager(retryPolicy),
		circuitBreaker: NewCircuitBreaker(maxFailures, resetTimeout),
	}
}

// Execute runs the operation through both retry logic and circuit breaker
func (rc *ResilientClient) Execute(operation func() error) error {
	return rc.circuitBreaker.Execute(func() error {
		return rc.retryManager.Execute(operation)
	})
}

// ExecuteWithContext runs the operation with context support
func (rc *ResilientClient) ExecuteWithContext(operation func() error, timeout time.Duration) error {
	return rc.circuitBreaker.Execute(func() error {
		return rc.retryManager.ExecuteWithContext(operation, timeout)
	})
}

// GetStats returns statistics about the resilient client
func (rc *ResilientClient) GetStats() ResilientClientStats {
	return ResilientClientStats{
		CircuitState: rc.circuitBreaker.GetState(),
		Failures:     rc.circuitBreaker.GetFailures(),
		MaxFailures:  rc.circuitBreaker.maxFailures,
		ResetTimeout: rc.circuitBreaker.resetTimeout,
		RetryPolicy:  rc.retryManager.policy,
	}
}

// ResilientClientStats provides statistics about the resilient client
type ResilientClientStats struct {
	CircuitState CircuitState  `json:"circuit_state"`
	Failures     int           `json:"failures"`
	MaxFailures  int           `json:"max_failures"`
	ResetTimeout time.Duration `json:"reset_timeout"`
	RetryPolicy  Policy        `json:"retry_policy"`
}
