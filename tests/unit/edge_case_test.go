package unit

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

func TestEdgeCase_ZeroAndNegativeValues(t *testing.T) {
	t.Run("test with zero retry delays and timeouts", func(t *testing.T) {
		// Zero initial delay
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   3,
			InitialDelay: 0,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       false,
		})

		attempts := 0
		start := time.Now()
		err := rm.Execute(func() error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary failure")
			}
			return nil
		})
		duration := time.Since(start)

		if err != nil {
			t.Errorf("Expected successful retry, got error: %v", err)
		}

		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}

		// With zero initial delay, retries should happen very quickly
		if duration > 200*time.Millisecond {
			t.Errorf("Execution took too long with zero initial delay: %v", duration)
		}
	})

	t.Run("test with zero max delay", func(t *testing.T) {
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     0, // Zero max delay
			Multiplier:   2.0,
			Jitter:       false,
		})

		attempts := 0
		err := rm.Execute(func() error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary failure")
			}
			return nil
		})

		if err != nil {
			t.Errorf("Expected successful retry despite zero max delay, got error: %v", err)
		}
	})

	t.Run("test with negative values in circuit breaker", func(t *testing.T) {
		// This should ideally be handled gracefully or panic
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Circuit breaker correctly panicked with negative values: %v", r)
			}
		}()

		// Try to create circuit breaker with invalid values
		cb := common.NewCircuitBreaker(-1, -1*time.Second)
		
		// If we get here, check if it handles operations correctly
		err := cb.Execute(func() error {
			return errors.New("test error")
		})

		if err == nil {
			t.Error("Expected error from circuit breaker operation")
		}
	})
}

func TestEdgeCase_ExtremelyLargeValues(t *testing.T) {
	t.Run("verify behavior with extremely large retry counts", func(t *testing.T) {
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   1000000, // 1 million retries
			InitialDelay: 1 * time.Microsecond,
			MaxDelay:     1 * time.Millisecond,
			Multiplier:   1.1,
			Jitter:       false,
		})

		attempts := int32(0)
		maxAttempts := int32(10) // We'll succeed after 10 attempts

		// Use context to limit execution time
		done := make(chan struct{})
		var err error

		go func() {
			err = rm.Execute(func() error {
				current := atomic.AddInt32(&attempts, 1)
				if current < maxAttempts {
					return errors.New("temporary failure")
				}
				return nil
			})
			close(done)
		}()

		select {
		case <-done:
			if err != nil {
				t.Errorf("Expected successful retry, got error: %v", err)
			}
			if atomic.LoadInt32(&attempts) != maxAttempts {
				t.Errorf("Expected %d attempts, got %d", maxAttempts, attempts)
			}
		case <-time.After(5 * time.Second):
			t.Error("Operation timed out with large retry count")
		}
	})

	t.Run("test with extremely large delays", func(t *testing.T) {
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   2,
			InitialDelay: 365 * 24 * time.Hour, // 1 year
			MaxDelay:     365 * 24 * time.Hour,
			Multiplier:   1.0,
			Jitter:       false,
		})

		// This should timeout quickly in ExecuteWithContext
		err := rm.ExecuteWithContext(func() error {
			return errors.New("temporary failure")
		}, 100*time.Millisecond)

		if err == nil {
			t.Error("Expected timeout error with extremely large delays")
		}

		// The error message already contains "timeout"
	})
}

func TestEdgeCase_PanicRecovery(t *testing.T) {
	t.Run("test error handling for panic recovery during retries", func(t *testing.T) {
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       false,
		})

		attempts := 0
		panicMessage := "intentional panic for testing"

		// The retry manager should not handle panics - they should propagate
		defer func() {
			if r := recover(); r != nil {
				if r != panicMessage {
					t.Errorf("Expected panic with message '%s', got '%v'", panicMessage, r)
				}
				t.Logf("Correctly propagated panic: %v", r)
			} else {
				t.Error("Expected panic to propagate, but no panic occurred")
			}
		}()

		rm.Execute(func() error {
			attempts++
			if attempts == 2 {
				panic(panicMessage)
			}
			return errors.New("temporary failure")
		})
	})

	t.Run("test circuit breaker with panicking operation", func(t *testing.T) {
		cb := common.NewCircuitBreaker(3, 100*time.Millisecond)

		// First, cause some failures to partially fill the failure count
		for i := 0; i < 2; i++ {
			cb.Execute(func() error {
				return errors.New("regular failure")
			})
		}

		// Now test with panic
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Circuit breaker correctly propagated panic: %v", r)
				
				// Check that circuit state is still consistent after panic
				state := cb.GetState()
				failures := cb.GetFailures()
				t.Logf("Circuit state after panic: %v, failures: %d", state, failures)
			}
		}()

		cb.Execute(func() error {
			panic("circuit breaker panic test")
		})
	})
}

func TestEdgeCase_SystemClockChanges(t *testing.T) {
	t.Run("validate behavior when system clock changes during retry", func(t *testing.T) {
		// This is difficult to test directly without mocking time
		// We'll test that the retry mechanism is resilient to time-based edge cases
		
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   3,
			InitialDelay: 50 * time.Millisecond,
			MaxDelay:     500 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       true,
		})

		var delays []time.Duration
		var mu sync.Mutex
		lastAttemptTime := time.Now()

		attempts := 0
		err := rm.Execute(func() error {
			attempts++
			
			now := time.Now()
			if attempts > 1 {
				delay := now.Sub(lastAttemptTime)
				mu.Lock()
				delays = append(delays, delay)
				mu.Unlock()
			}
			lastAttemptTime = now

			if attempts < 4 {
				return errors.New("temporary failure")
			}
			return nil
		})

		if err != nil {
			t.Errorf("Expected successful retry, got error: %v", err)
		}

		// Verify delays are reasonable even with potential clock skew
		mu.Lock()
		defer mu.Unlock()
		
		for i, delay := range delays {
			// Allow for some variance due to scheduling and jitter
			minDelay := 10 * time.Millisecond // Minimum reasonable delay
			maxDelay := 1 * time.Second       // Maximum reasonable delay
			
			if delay < minDelay || delay > maxDelay {
				t.Errorf("Delay %d is out of reasonable range: %v", i, delay)
			}
		}
	})
}

func TestEdgeCase_NilAndEmptyErrors(t *testing.T) {
	t.Run("test retry behavior with nil errors", func(t *testing.T) {
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       false,
		})

		attempts := 0
		err := rm.Execute(func() error {
			attempts++
			// Return nil immediately - should not retry
			return nil
		})

		if err != nil {
			t.Errorf("Expected nil error, got: %v", err)
		}

		if attempts != 1 {
			t.Errorf("Expected 1 attempt for nil error, got %d", attempts)
		}
	})

	t.Run("test with empty error messages", func(t *testing.T) {
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:      3,
			InitialDelay:    10 * time.Millisecond,
			MaxDelay:        100 * time.Millisecond,
			Multiplier:      2.0,
			Jitter:          false,
			RetryableErrors: []string{""}, // Empty string as retryable error
		})

		attempts := 0
		err := rm.Execute(func() error {
			attempts++
			if attempts < 3 {
				return errors.New("") // Empty error message
			}
			return nil
		})

		if err != nil {
			t.Errorf("Expected successful retry with empty error message, got: %v", err)
		}

		if attempts != 3 {
			t.Errorf("Expected 3 attempts with empty error message, got %d", attempts)
		}
	})
}

func TestEdgeCase_ConcurrentStateModification(t *testing.T) {
	t.Run("test circuit breaker with rapid concurrent state changes", func(t *testing.T) {
		cb := common.NewCircuitBreaker(5, 50*time.Millisecond)
		
		const numGoroutines = 100
		var wg sync.WaitGroup
		
		// Create a pattern that will cause rapid state changes
		for round := 0; round < 3; round++ {
			wg.Add(numGoroutines)
			
			// Cause failures to open circuit
			for i := 0; i < numGoroutines; i++ {
				go func(id int) {
					defer wg.Done()
					cb.Execute(func() error {
						return errors.New("concurrent failure")
					})
				}(i)
			}
			
			wg.Wait()
			
			// Wait for circuit to potentially transition to half-open
			time.Sleep(60 * time.Millisecond)
			
			// Try success to close circuit
			wg.Add(1)
			go func() {
				defer wg.Done()
				cb.Execute(func() error {
					return nil
				})
			}()
			
			wg.Wait()
		}
		
		// Verify circuit breaker is still in a valid state
		finalState := cb.GetState()
		stats := cb.GetStats()
		
		t.Logf("Final circuit state after concurrent modifications: %v", finalState)
		t.Logf("Circuit stats: %+v", stats)
		
		// State should be one of the valid states
		validStates := []common.CircuitState{
			common.CircuitClosed,
			common.CircuitOpen,
			common.CircuitHalfOpen,
		}
		
		isValidState := false
		for _, valid := range validStates {
			if finalState == valid {
				isValidState = true
				break
			}
		}
		
		if !isValidState {
			t.Errorf("Circuit breaker in invalid state: %v", finalState)
		}
	})
}

func TestEdgeCase_PolicyValidation(t *testing.T) {
	t.Run("test with zero multiplier", func(t *testing.T) {
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   0, // Zero multiplier
			Jitter:       false,
		})

		var delays []time.Duration
		lastTime := time.Now()
		attempts := 0

		err := rm.Execute(func() error {
			attempts++
			now := time.Now()
			if attempts > 1 {
				delays = append(delays, now.Sub(lastTime))
			}
			lastTime = now
			
			if attempts < 3 {
				return errors.New("temporary failure")
			}
			return nil
		})

		if err != nil {
			t.Errorf("Expected successful retry, got error: %v", err)
		}

		// With zero multiplier, delays might go to zero after multiplication
		// Just verify the test completed successfully
		if len(delays) < 1 {
			t.Error("Expected at least one retry delay to be recorded")
		}
	})

	t.Run("test with multiplier less than 1", func(t *testing.T) {
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   3,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     200 * time.Millisecond,
			Multiplier:   0.5, // Decreasing delays
			Jitter:       false,
		})

		attempts := 0
		err := rm.Execute(func() error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary failure")
			}
			return nil
		})

		if err != nil {
			t.Errorf("Expected successful retry with decreasing delays, got error: %v", err)
		}
	})
}

// Custom error type that implements temporary interface
type temporaryError struct {
	msg string
}

func (e temporaryError) Error() string {
	return e.msg
}

func (e temporaryError) Temporary() bool {
	return true
}

func TestEdgeCase_InterfaceImplementation(t *testing.T) {
	t.Run("test custom error types with retry detection", func(t *testing.T) {

		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       false,
		})

		attempts := 0
		err := rm.Execute(func() error {
			attempts++
			if attempts < 3 {
				return temporaryError{msg: "custom temporary error"}
			}
			return nil
		})

		if err != nil {
			t.Errorf("Expected successful retry with custom temporary error, got: %v", err)
		}

		if attempts != 3 {
			t.Errorf("Expected 3 attempts with custom temporary error, got %d", attempts)
		}
	})
}

func TestEdgeCase_ResourceExhaustion(t *testing.T) {
	t.Run("test behavior under goroutine exhaustion", func(t *testing.T) {
		// Create many retry managers and circuit breakers
		const numInstances = 1000
		
		retryManagers := make([]*common.RetryManager, numInstances)
		circuitBreakers := make([]*common.CircuitBreaker, numInstances)
		
		for i := 0; i < numInstances; i++ {
			retryManagers[i] = common.NewRetryManager(common.RetryPolicy{
				MaxRetries:   2,
				InitialDelay: 1 * time.Millisecond,
				MaxDelay:     10 * time.Millisecond,
				Multiplier:   2.0,
				Jitter:       true,
			})
			
			circuitBreakers[i] = common.NewCircuitBreaker(3, 50*time.Millisecond)
		}

		// Run operations on all instances concurrently
		var wg sync.WaitGroup
		var successCount int32
		
		for i := 0; i < numInstances; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				
				rm := retryManagers[idx]
				cb := circuitBreakers[idx]
				
				// Combine retry manager and circuit breaker
				err := cb.Execute(func() error {
					return rm.Execute(func() error {
						// Simple operation that sometimes fails
						if time.Now().UnixNano()%2 == 0 {
							return errors.New("random failure")
						}
						atomic.AddInt32(&successCount, 1)
						return nil
					})
				})
				
				if err != nil && !strings.Contains(err.Error(), "random failure") {
					t.Logf("Instance %d failed with unexpected error: %v", idx, err)
				}
			}(i)
		}

		// Set a timeout for the test
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			t.Logf("Successfully completed %d operations with %d successes", 
				numInstances, atomic.LoadInt32(&successCount))
		case <-time.After(30 * time.Second):
			t.Error("Test timed out - possible resource exhaustion")
		}
	})
}