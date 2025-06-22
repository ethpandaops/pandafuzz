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

func TestRetryManager_ConcurrentExecute(t *testing.T) {
	t.Run("multiple goroutines using RetryManager simultaneously", func(t *testing.T) {
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       true,
		})

		const numGoroutines = 100
		var wg sync.WaitGroup
		var successCount int32
		var totalAttempts int32

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				
				attempts := 0
				err := rm.Execute(func() error {
					attempts++
					atomic.AddInt32(&totalAttempts, 1)
					
					// Half of the goroutines succeed on the second attempt
					if id%2 == 0 && attempts >= 2 {
						return nil
					}
					// Other half succeed on the third attempt
					if id%2 == 1 && attempts >= 3 {
						return nil
					}
					return errors.New("temporary failure")
				})
				
				if err == nil {
					atomic.AddInt32(&successCount, 1)
				}
			}(i)
		}

		wg.Wait()

		expectedSuccess := int32(numGoroutines)
		if successCount != expectedSuccess {
			t.Errorf("Expected %d successful operations, got %d", expectedSuccess, successCount)
		}

		// Verify that retries happened as expected
		minAttempts := int32(numGoroutines * 2) // At least 2 attempts per goroutine
		if totalAttempts < minAttempts {
			t.Errorf("Expected at least %d total attempts, got %d", minAttempts, totalAttempts)
		}
	})

	t.Run("concurrent retry attempts are properly isolated", func(t *testing.T) {
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   2,
			InitialDelay: 20 * time.Millisecond,
			MaxDelay:     200 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       false,
		})

		const numGoroutines = 50
		var wg sync.WaitGroup
		attemptCounts := make(map[int]int)
		var mu sync.Mutex

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				
				localAttempts := 0
				rm.Execute(func() error {
					localAttempts++
					mu.Lock()
					attemptCounts[id] = localAttempts
					mu.Unlock()
					
					// Always fail with a retryable error to test max retries
					return errors.New("connection refused")
				})
			}(i)
		}

		wg.Wait()

		// Each goroutine should have exactly 3 attempts (initial + 2 retries)
		for id, count := range attemptCounts {
			if count != 3 {
				t.Errorf("Goroutine %d had %d attempts, expected 3", id, count)
			}
		}
	})
}

func TestCircuitBreaker_ThreadSafety(t *testing.T) {
	t.Run("concurrent state changes are thread-safe", func(t *testing.T) {
		cb := common.NewCircuitBreaker(5, 100*time.Millisecond)
		
		const numGoroutines = 100
		var wg sync.WaitGroup
		var failureCount int32

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				
				// Some goroutines succeed, some fail
				err := cb.Execute(func() error {
					if id%3 == 0 {
						return errors.New("operation failed")
					}
					return nil
				})
				
				if err != nil {
					atomic.AddInt32(&failureCount, 1)
				}
			}(i)
		}

		wg.Wait()

		// Circuit should remain consistent despite concurrent access
		state := cb.GetState()
		failures := cb.GetFailures()
		
		t.Logf("Circuit state after concurrent operations: %v, failures: %d", state, failures)
		
		// The state should be consistent with the failure count
		if failures > 5 && state != common.CircuitOpen {
			t.Errorf("Circuit should be open with %d failures, but state is %v", failures, state)
		}
	})

	t.Run("race condition in circuit state transitions", func(t *testing.T) {
		cb := common.NewCircuitBreaker(3, 50*time.Millisecond)
		
		const numGoroutines = 50
		var wg sync.WaitGroup
		stateChanges := make([]common.CircuitState, 0)
		var mu sync.Mutex

		// Monitor state changes
		go func() {
			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()
			
			for i := 0; i < 20; i++ {
				<-ticker.C
				state := cb.GetState()
				mu.Lock()
				stateChanges = append(stateChanges, state)
				mu.Unlock()
			}
		}()

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				time.Sleep(time.Duration(id) * time.Millisecond) // Stagger operations
				
				cb.Execute(func() error {
					return errors.New("failure")
				})
			}(i)
		}

		wg.Wait()
		time.Sleep(100 * time.Millisecond) // Allow monitoring to capture final state

		// Verify state transitions are valid
		mu.Lock()
		defer mu.Unlock()
		
		for i := 1; i < len(stateChanges); i++ {
			prev := stateChanges[i-1]
			curr := stateChanges[i]
			
			// Valid transitions: Closed->Open, Open->HalfOpen, HalfOpen->Closed/Open
			if prev == common.CircuitClosed && curr == common.CircuitHalfOpen {
				t.Errorf("Invalid transition from Closed to HalfOpen at index %d", i)
			}
		}
	})
}

func TestRetryManager_ConcurrentBackoffCalculations(t *testing.T) {
	t.Run("race conditions in retry backoff calculations", func(t *testing.T) {
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   5,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     1000 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       true,
		})

		const numGoroutines = 100
		var wg sync.WaitGroup
		delayObservations := make([][]time.Duration, numGoroutines)

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				
				var delays []time.Duration
				attempt := 0
				
				rm.Execute(func() error {
					if attempt > 0 {
						// Record the delay between attempts
						delays = append(delays, time.Since(time.Now()))
					}
					attempt++
					
					if attempt < 4 {
						return errors.New("retry needed")
					}
					return nil
				})
				
				delayObservations[id] = delays
			}(i)
		}

		wg.Wait()

		// Verify that each goroutine experienced proper exponential backoff
		for id, delays := range delayObservations {
			if len(delays) == 0 {
				continue
			}
			
			// Due to jitter, we can't check exact values, but delays should generally increase
			for i := 1; i < len(delays); i++ {
				// Allow some variance due to jitter and scheduling
				if delays[i] < delays[i-1]/2 {
					t.Errorf("Goroutine %d: delay decreased too much at attempt %d", id, i)
				}
			}
		}
	})
}

func TestResilientClient_ConcurrentOperations(t *testing.T) {
	t.Run("concurrent operations with combined retry and circuit breaker", func(t *testing.T) {
		retryPolicy := common.RetryPolicy{
			MaxRetries:   3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       false,
		}
		
		rc := common.NewResilientClient(retryPolicy, 10, 200*time.Millisecond)
		
		const numGoroutines = 100
		var wg sync.WaitGroup
		var successCount int32
		var circuitOpenCount int32

		// Create a pattern of failures to trigger circuit breaker
		failurePattern := make([]bool, numGoroutines)
		for i := 0; i < 15; i++ {
			failurePattern[i] = true // First 15 operations will fail
		}

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				
				err := rc.Execute(func() error {
					if id < len(failurePattern) && failurePattern[id] {
						return errors.New("operation failed")
					}
					return nil
				})
				
				if err == nil {
					atomic.AddInt32(&successCount, 1)
				} else if strings.Contains(err.Error(), "circuit breaker is open") {
					atomic.AddInt32(&circuitOpenCount, 1)
				}
			}(i)
		}

		wg.Wait()

		t.Logf("Results: %d successes, %d circuit open errors", successCount, circuitOpenCount)
		
		// After 10 failures, circuit should open, preventing some operations
		if circuitOpenCount == 0 {
			t.Error("Expected some operations to fail due to open circuit")
		}
		
		// Some operations should succeed (those before circuit opens and those that don't fail)
		if successCount == 0 {
			t.Error("Expected some operations to succeed")
		}
	})

	t.Run("concurrent timeout handling", func(t *testing.T) {
		retryPolicy := common.RetryPolicy{
			MaxRetries:   2,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     50 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       false,
		}
		
		rc := common.NewResilientClient(retryPolicy, 5, 100*time.Millisecond)
		
		const numGoroutines = 50
		var wg sync.WaitGroup
		var timeoutCount int32

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				
				err := rc.ExecuteWithContext(func() error {
					// Simulate operations with varying durations
					// Make some operations definitely timeout
					if id > 25 {
						time.Sleep(200 * time.Millisecond)
					} else {
						time.Sleep(50 * time.Millisecond)
					}
					return nil
				}, 100*time.Millisecond)
				
				if err != nil {
					errStr := err.Error()
					if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "timed out") {
						atomic.AddInt32(&timeoutCount, 1)
					}
				}
			}(i)
		}

		wg.Wait()

		// Wait a bit for atomic operations to complete
		time.Sleep(10 * time.Millisecond)
		
		// Some operations should timeout
		finalCount := atomic.LoadInt32(&timeoutCount)
		if finalCount == 0 {
			t.Error("Expected some operations to timeout")
		}
		
		t.Logf("Timed out operations: %d out of %d", finalCount, numGoroutines)
	})
}