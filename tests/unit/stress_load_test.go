package unit

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

func TestStressLoad_HighVolumeOperations(t *testing.T) {
	t.Run("test RetryManager under high load 10k+ operations/sec", func(t *testing.T) {
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   2,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       true,
		})

		const (
			numGoroutines = 100
			opsPerGoroutine = 100
			totalOps = numGoroutines * opsPerGoroutine
		)

		var (
			successCount int64
			failureCount int64
			retryCount   int64
			totalLatency int64
		)

		start := time.Now()
		var wg sync.WaitGroup

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				
				for j := 0; j < opsPerGoroutine; j++ {
					opStart := time.Now()
					attempts := 0
					
					err := rm.Execute(func() error {
						attempts++
						if attempts > 1 {
							atomic.AddInt64(&retryCount, 1)
						}
						
						// Simulate 20% failure rate on first attempt
						if attempts == 1 && (id*opsPerGoroutine+j)%5 == 0 {
							return errors.New("temporary failure")
						}
						
						// Simulate some work
						time.Sleep(100 * time.Microsecond)
						return nil
					})
					
					latency := time.Since(opStart).Microseconds()
					atomic.AddInt64(&totalLatency, latency)
					
					if err == nil {
						atomic.AddInt64(&successCount, 1)
					} else {
						atomic.AddInt64(&failureCount, 1)
					}
				}
			}(i)
		}

		wg.Wait()
		duration := time.Since(start)

		// Calculate metrics
		opsPerSecond := float64(totalOps) / duration.Seconds()
		avgLatencyMicros := float64(atomic.LoadInt64(&totalLatency)) / float64(totalOps)
		successRate := float64(atomic.LoadInt64(&successCount)) / float64(totalOps) * 100

		t.Logf("Performance Metrics:")
		t.Logf("  Total operations: %d", totalOps)
		t.Logf("  Duration: %v", duration)
		t.Logf("  Operations/second: %.2f", opsPerSecond)
		t.Logf("  Average latency: %.2f μs", avgLatencyMicros)
		t.Logf("  Success rate: %.2f%%", successRate)
		t.Logf("  Total retries: %d", atomic.LoadInt64(&retryCount))

		// Verify performance thresholds
		if opsPerSecond < 10000 {
			t.Errorf("Operations per second %.2f is below 10,000 threshold", opsPerSecond)
		}

		if successRate < 95 {
			t.Errorf("Success rate %.2f%% is below 95%% threshold", successRate)
		}
	})
}

func TestStressLoad_MemoryUsage(t *testing.T) {
	t.Run("measure memory usage during extended retry cycles", func(t *testing.T) {
		// Get initial memory stats
		var initialMem runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&initialMem)

		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   5,
			InitialDelay: 5 * time.Millisecond,
			MaxDelay:     50 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       true,
		})

		const (
			numGoroutines = 50
			testDuration = 2 * time.Second
		)

		var (
			operationCount int64
			memSamples     []uint64
			mu             sync.Mutex
		)

		ctx := make(chan struct{})
		var wg sync.WaitGroup

		// Memory monitoring goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					mu.Lock()
					memSamples = append(memSamples, m.Alloc)
					mu.Unlock()
				case <-ctx:
					return
				}
			}
		}()

		// Worker goroutines
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				
				for {
					select {
					case <-ctx:
						return
					default:
						rm.Execute(func() error {
							atomic.AddInt64(&operationCount, 1)
							
							// Allocate some memory to simulate real work
							data := make([]byte, 1024)
							_ = data
							
							// 30% failure rate to trigger retries
							if time.Now().UnixNano()%10 < 3 {
								return errors.New("temporary failure")
							}
							return nil
						})
					}
				}
			}(i)
		}

		// Run test for specified duration
		time.Sleep(testDuration)
		close(ctx)
		wg.Wait()

		// Get final memory stats
		runtime.GC()
		var finalMem runtime.MemStats
		runtime.ReadMemStats(&finalMem)

		// Analyze memory samples
		mu.Lock()
		defer mu.Unlock()

		if len(memSamples) == 0 {
			t.Fatal("No memory samples collected")
		}

		var totalMem, peakMem uint64
		for _, mem := range memSamples {
			totalMem += mem
			if mem > peakMem {
				peakMem = mem
			}
		}
		avgMem := totalMem / uint64(len(memSamples))

		t.Logf("Memory Usage Analysis:")
		t.Logf("  Initial memory: %.2f MB", float64(initialMem.Alloc)/1024/1024)
		t.Logf("  Final memory: %.2f MB", float64(finalMem.Alloc)/1024/1024)
		t.Logf("  Average memory: %.2f MB", float64(avgMem)/1024/1024)
		t.Logf("  Peak memory: %.2f MB", float64(peakMem)/1024/1024)
		t.Logf("  Total operations: %d", atomic.LoadInt64(&operationCount))
		t.Logf("  Memory per 1000 ops: %.2f KB", float64(avgMem-initialMem.Alloc)/float64(operationCount)*1000/1024)

		// Check for memory leaks
		memoryGrowth := float64(finalMem.Alloc) - float64(initialMem.Alloc)
		memoryGrowthMB := memoryGrowth / 1024 / 1024
		
		if memoryGrowthMB > 50 {
			t.Errorf("Excessive memory growth: %.2f MB", memoryGrowthMB)
		}
	})
}

func TestStressLoad_CircuitBreakerUnderLoad(t *testing.T) {
	t.Run("test circuit breaker performance under sustained failures", func(t *testing.T) {
		cb := common.NewCircuitBreaker(10, 100*time.Millisecond)

		const (
			numGoroutines = 100
			testDuration = 3 * time.Second
		)

		var (
			totalOps       int64
			successOps     int64
			failedOps      int64
			circuitOpenOps int64
			stateChanges   int64
		)

		ctx := make(chan struct{})
		var wg sync.WaitGroup

		// Monitor circuit state changes
		lastState := cb.GetState()
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					currentState := cb.GetState()
					if currentState != lastState {
						atomic.AddInt64(&stateChanges, 1)
						lastState = currentState
					}
				case <-ctx:
					return
				}
			}
		}()

		// Worker goroutines
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				
				for {
					select {
					case <-ctx:
						return
					default:
						atomic.AddInt64(&totalOps, 1)
						
						err := cb.Execute(func() error {
							// Simulate varying failure patterns
							now := time.Now()
							
							// High failure rate for first second
							if now.Sub(time.Now()).Seconds() < 1 {
								if now.UnixNano()%10 < 8 { // 80% failure
									return errors.New("high failure period")
								}
							} else if now.UnixNano()%10 < 2 { // 20% failure
								return errors.New("normal failure")
							}
							
							return nil
						})
						
						if err == nil {
							atomic.AddInt64(&successOps, 1)
						} else if strings.Contains(err.Error(), "circuit breaker is open") {
							atomic.AddInt64(&circuitOpenOps, 1)
						} else {
							atomic.AddInt64(&failedOps, 1)
						}
						
						// Small delay to prevent spinning
						time.Sleep(time.Microsecond)
					}
				}
			}(i)
		}

		// Run test
		start := time.Now()
		time.Sleep(testDuration)
		close(ctx)
		wg.Wait()
		duration := time.Since(start)

		// Calculate metrics
		opsPerSecond := float64(totalOps) / duration.Seconds()
		circuitOpenRate := float64(circuitOpenOps) / float64(totalOps) * 100

		t.Logf("Circuit Breaker Performance:")
		t.Logf("  Total operations: %d", totalOps)
		t.Logf("  Operations/second: %.2f", opsPerSecond)
		t.Logf("  Successful operations: %d", successOps)
		t.Logf("  Failed operations: %d", failedOps)
		t.Logf("  Circuit open rejections: %d (%.2f%%)", circuitOpenOps, circuitOpenRate)
		t.Logf("  State changes: %d", stateChanges)
		t.Logf("  Final state: %v", cb.GetState())

		// Verify circuit breaker is functioning
		if stateChanges == 0 {
			t.Error("Expected circuit breaker state changes under sustained failures")
		}

		if circuitOpenOps == 0 {
			t.Error("Expected some operations to be rejected when circuit is open")
		}
	})
}

func TestStressLoad_RetryPolicyLimits(t *testing.T) {
	t.Run("verify system behavior at retry policy limits", func(t *testing.T) {
		// Test with extreme retry policy
		extremePolicy := common.RetryPolicy{
			MaxRetries:   50,
			InitialDelay: 1 * time.Microsecond,
			MaxDelay:     10 * time.Second,
			Multiplier:   3.0,
			Jitter:       true,
		}

		rm := common.NewRetryManager(extremePolicy)

		var (
			maxAttemptsObserved int32
			totalRetries        int64
			totalDuration       int64
		)

		const numOperations = 100

		var wg sync.WaitGroup
		wg.Add(numOperations)

		for i := 0; i < numOperations; i++ {
			go func(id int) {
				defer wg.Done()
				
				attempts := int32(0)
				start := time.Now()
				
				err := rm.Execute(func() error {
					currentAttempt := atomic.AddInt32(&attempts, 1)
					
					// Update max attempts
					for {
						old := atomic.LoadInt32(&maxAttemptsObserved)
						if currentAttempt <= old || atomic.CompareAndSwapInt32(&maxAttemptsObserved, old, currentAttempt) {
							break
						}
					}
					
					// Fail until we reach a high number of attempts
					if currentAttempt < 10 {
						atomic.AddInt64(&totalRetries, 1)
						return errors.New("temporary failure")
					}
					
					return nil
				})
				
				duration := time.Since(start).Milliseconds()
				atomic.AddInt64(&totalDuration, duration)
				
				if err != nil {
					t.Logf("Operation %d failed after %d attempts: %v", id, attempts, err)
				}
			}(i)
		}

		wg.Wait()

		avgDuration := float64(totalDuration) / float64(numOperations)
		avgRetries := float64(totalRetries) / float64(numOperations)

		t.Logf("Retry Policy Limits Test:")
		t.Logf("  Max attempts observed: %d", maxAttemptsObserved)
		t.Logf("  Average duration per operation: %.2f ms", avgDuration)
		t.Logf("  Average retries per operation: %.2f", avgRetries)
		t.Logf("  Total retries across all operations: %d", totalRetries)

		// Verify retry limits are respected
		if int(maxAttemptsObserved) > extremePolicy.MaxRetries+1 {
			t.Errorf("Exceeded max retries: observed %d attempts, policy allows %d retries",
				maxAttemptsObserved-1, extremePolicy.MaxRetries)
		}

		// Verify operations complete in reasonable time despite high retry count
		maxAcceptableDuration := float64(1000) // 1 second average
		if avgDuration > maxAcceptableDuration {
			t.Errorf("Average operation duration %.2f ms exceeds acceptable limit of %.2f ms",
				avgDuration, maxAcceptableDuration)
		}
	})
}

func TestStressLoad_ConcurrentCircuitBreakers(t *testing.T) {
	t.Run("test multiple circuit breakers under concurrent load", func(t *testing.T) {
		const numCircuits = 10
		const numWorkersPerCircuit = 10
		const testDuration = 2 * time.Second

		// Create multiple circuit breakers
		circuits := make([]*common.CircuitBreaker, numCircuits)
		for i := 0; i < numCircuits; i++ {
			circuits[i] = common.NewCircuitBreaker(5, 100*time.Millisecond)
		}

		var (
			totalOps      int64
			totalFailures int64
			openCircuits  int64
		)

		ctx := make(chan struct{})
		var wg sync.WaitGroup

		// Workers for each circuit breaker
		for i := 0; i < numCircuits; i++ {
			cb := circuits[i]
			
			for j := 0; j < numWorkersPerCircuit; j++ {
				wg.Add(1)
				go func(circuitID, workerID int) {
					defer wg.Done()
					
					for {
						select {
						case <-ctx:
							return
						default:
							atomic.AddInt64(&totalOps, 1)
							
							err := cb.Execute(func() error {
								// Different failure patterns for different circuits
								if circuitID < 3 {
									// High failure rate circuits
									if time.Now().UnixNano()%10 < 7 {
										return errors.New("high failure circuit")
									}
								} else if circuitID < 6 {
									// Medium failure rate circuits
									if time.Now().UnixNano()%10 < 3 {
										return errors.New("medium failure circuit")
									}
								}
								// Low failure rate for remaining circuits
								if time.Now().UnixNano()%100 < 5 {
									return errors.New("low failure circuit")
								}
								return nil
							})
							
							if err != nil {
								atomic.AddInt64(&totalFailures, 1)
							}
						}
					}
				}(i, j)
			}
		}

		// Monitor circuit states
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					open := int64(0)
					for _, cb := range circuits {
						if cb.GetState() == common.CircuitOpen {
							open++
						}
					}
					atomic.StoreInt64(&openCircuits, open)
				case <-ctx:
					return
				}
			}
		}()

		// Run test
		start := time.Now()
		time.Sleep(testDuration)
		close(ctx)
		wg.Wait()
		duration := time.Since(start)

		// Final circuit states
		var finalStates []string
		openCount := 0
		for i, cb := range circuits {
			state := cb.GetState()
			finalStates = append(finalStates, fmt.Sprintf("CB%d: %v", i, state))
			if state == common.CircuitOpen {
				openCount++
			}
		}

		opsPerSecond := float64(totalOps) / duration.Seconds()
		failureRate := float64(totalFailures) / float64(totalOps) * 100

		t.Logf("Multiple Circuit Breakers Performance:")
		t.Logf("  Total operations: %d", totalOps)
		t.Logf("  Operations/second: %.2f", opsPerSecond)
		t.Logf("  Failure rate: %.2f%%", failureRate)
		t.Logf("  Max open circuits observed: %d", atomic.LoadInt64(&openCircuits))
		t.Logf("  Final open circuits: %d/%d", openCount, numCircuits)
		t.Logf("  Final states: %v", finalStates)

		// Verify high-failure circuits opened
		if openCount < 2 {
			t.Error("Expected at least 2 circuits to be open given the failure patterns")
		}
	})
}

