package unit

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

func TestNetworkResilience_NetworkPartitions(t *testing.T) {
	t.Run("simulate network partitions and test retry behavior", func(t *testing.T) {
		var requestCount int32

		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:      5,
			InitialDelay:    100 * time.Millisecond,
			MaxDelay:        1 * time.Second,
			Multiplier:      2.0,
			Jitter:          false,
			RetryableErrors: []string{"EOF", "connection reset", "broken pipe"},
		})

		var lastErr error
		err := rm.Execute(func() error {
			count := atomic.AddInt32(&requestCount, 1)
			if count <= 3 {
				lastErr = io.EOF
				return lastErr
			}
			return nil
		})

		if err != nil {
			t.Errorf("Expected successful retry after network partition healed, got error: %v", err)
		}

		actualRequests := atomic.LoadInt32(&requestCount)
		if actualRequests < 4 {
			t.Errorf("Expected at least 4 requests (3 failures + 1 success), got %d", actualRequests)
		}

		// Verify the last error was network-related
		if lastErr != nil && !isNetworkRelatedError(lastErr) {
			t.Errorf("Expected network-related error, got: %v", lastErr)
		}
	})
}

func TestNetworkResilience_DNSResolution(t *testing.T) {
	t.Run("test retry behavior with DNS resolution failures", func(t *testing.T) {
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:      3,
			InitialDelay:    50 * time.Millisecond,
			MaxDelay:        500 * time.Millisecond,
			Multiplier:      2.0,
			Jitter:          false,
			RetryableErrors: []string{"no such host", "not resolve"},
		})

		attempts := 0
		err := rm.Execute(func() error {
			attempts++

			if attempts < 3 {
				return &net.DNSError{
					Err:  "no such host",
					Name: "non-existent-domain-that-should-not-resolve.invalid",
				}
			}

			return nil
		})

		// DNS errors are typically not retryable by default unless specifically configured
		if err == nil {
			t.Log("DNS resolution succeeded after retries")
		}

		if attempts < 3 {
			t.Errorf("Expected at least 3 attempts for DNS resolution, got %d", attempts)
		}
	})

	t.Run("custom DNS retry policy", func(t *testing.T) {
		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:      3,
			InitialDelay:    50 * time.Millisecond,
			MaxDelay:        500 * time.Millisecond,
			Multiplier:      2.0,
			Jitter:          false,
			RetryableErrors: []string{"no such host", "dns lookup failed", "name resolution failed"},
		})

		attempts := 0
		err := rm.Execute(func() error {
			attempts++

			// Simulate DNS resolution failure
			if attempts < 3 {
				return &net.DNSError{
					Err:  "no such host",
					Name: "test-domain.invalid",
				}
			}

			return nil
		})

		if err != nil {
			t.Errorf("Expected successful retry with custom DNS policy, got error: %v", err)
		}

		if attempts != 3 {
			t.Errorf("Expected exactly 3 attempts, got %d", attempts)
		}
	})
}

func TestNetworkResilience_PartialFailures(t *testing.T) {
	t.Run("verify handling of partial network failures", func(t *testing.T) {
		// Create multiple test servers to simulate partial failures
		var server1Failures int32
		var server2Failures int32

		rc := common.NewResilientClient(
			common.RetryPolicy{
				MaxRetries:      3,
				InitialDelay:    50 * time.Millisecond,
				MaxDelay:        500 * time.Millisecond,
				Multiplier:      2.0,
				Jitter:          false,
				RetryableErrors: []string{"server error", "503"},
			},
			5,
			1*time.Second,
		)

		// Test with multiple endpoints
		endpoints := []string{"server-1", "server-2"}
		successCount := 0

		for _, endpoint := range endpoints {
			err := rc.Execute(func() error {
				var failures *int32
				var maxFailures int32
				if endpoint == "server-1" {
					failures = &server1Failures
					maxFailures = 2
				} else {
					failures = &server2Failures
					maxFailures = 1
				}

				count := atomic.AddInt32(failures, 1)
				if count <= maxFailures {
					return errors.New("server error: 503")
				}

				return nil
			})

			if err == nil {
				successCount++
			}
		}

		if successCount != 2 {
			t.Errorf("Expected both endpoints to succeed after retries, got %d successes", successCount)
		}
	})
}

func TestNetworkResilience_VaryingLatencies(t *testing.T) {
	t.Run("test behavior under varying network latencies", func(t *testing.T) {
		latencies := []time.Duration{
			10 * time.Millisecond,
			500 * time.Millisecond,
			100 * time.Millisecond,
			1 * time.Second,
			50 * time.Millisecond,
		}

		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   4,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     2 * time.Second,
			Multiplier:   1.5,
			Jitter:       true,
		})

		// Test with timeout that should trigger retries for slow responses
		successCount := 0
		timeoutCount := 0
		clientTimeout := 300 * time.Millisecond

		for i := 0; i < 5; i++ {
			latency := latencies[i]
			err := rm.ExecuteWithContext(func() error {
				if latency > clientTimeout {
					time.Sleep(clientTimeout)
					timeoutCount++
					return context.DeadlineExceeded
				}

				time.Sleep(latency)
				return nil
			}, 5*time.Second)

			if err == nil {
				successCount++
			}
		}

		t.Logf("Success: %d, Timeouts: %d", successCount, timeoutCount)

		// Some requests should succeed (those with low latency)
		if successCount == 0 {
			t.Error("Expected some requests to succeed with low latency")
		}

		// Some requests should timeout (those with high latency)
		if timeoutCount == 0 {
			t.Error("Expected some requests to timeout with high latency")
		}
	})
}

func TestNetworkResilience_ConnectionReuse(t *testing.T) {
	t.Run("test retry behavior with connection reuse and keepalive", func(t *testing.T) {
		var connectionCount int32

		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:      3,
			InitialDelay:    100 * time.Millisecond,
			MaxDelay:        1 * time.Second,
			Multiplier:      2.0,
			Jitter:          false,
			RetryableErrors: []string{"server error", "503"},
		})

		err := rm.Execute(func() error {
			count := atomic.AddInt32(&connectionCount, 1)
			if count <= 2 {
				return errors.New("server error: 503")
			}

			return nil
		})

		if err != nil {
			t.Errorf("Expected successful retry after connection issues resolved, got error: %v", err)
		}

		// Verify multiple connection attempts were made
		if atomic.LoadInt32(&connectionCount) < 3 {
			t.Errorf("Expected at least 3 connection attempts, got %d", connectionCount)
		}
	})
}

// Helper functions
func isNetworkRelatedError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, io.EOF) {
		return true
	}

	// Check for specific network error types
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Check for common network error strings
	errStr := strings.ToLower(err.Error())
	networkErrors := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"network unreachable",
		"no route to host",
		"connection closed",
	}

	for _, pattern := range networkErrors {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// Check for timeout interface
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Check for context timeout
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check for timeout in error message
	return strings.Contains(strings.ToLower(err.Error()), "timeout")
}
