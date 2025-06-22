package unit

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

func TestNetworkResilience_NetworkPartitions(t *testing.T) {
	t.Run("simulate network partitions and test retry behavior", func(t *testing.T) {
		// Create a test server that simulates network partitions
		var requestCount int32
		var networkPartitioned atomic.Bool
		networkPartitioned.Store(true)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&requestCount, 1)
			
			// Simulate network partition for first 3 requests
			if count <= 3 && networkPartitioned.Load() {
				// Abruptly close the connection to simulate network partition
				hj, ok := w.(http.Hijacker)
				if ok {
					conn, _, _ := hj.Hijack()
					conn.Close()
				}
				return
			}
			
			// After 3 attempts, "heal" the network partition
			networkPartitioned.Store(false)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}))
		defer server.Close()

		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   5,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     1 * time.Second,
			Multiplier:   2.0,
			Jitter:       false,
			RetryableErrors: []string{"EOF", "connection reset", "broken pipe"},
		})

		var lastErr error
		err := rm.Execute(func() error {
			client := &http.Client{
				Timeout: 500 * time.Millisecond,
			}
			
			resp, err := client.Get(server.URL)
			if err != nil {
				lastErr = err
				return err
			}
			defer resp.Body.Close()
			
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status: %d", resp.StatusCode)
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
			MaxRetries:   3,
			InitialDelay: 50 * time.Millisecond,
			MaxDelay:     500 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       false,
			RetryableErrors: []string{"no such host", "not resolve"},
		})

		attempts := 0
		err := rm.Execute(func() error {
			attempts++
			
			// Simulate DNS resolution failure
			_, err := net.LookupHost("non-existent-domain-that-should-not-resolve.invalid")
			if err != nil {
				// On the 3rd attempt, use a valid domain
				if attempts >= 3 {
					_, err = net.LookupHost("localhost")
					return err
				}
				return err
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
		
		server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			failures := atomic.AddInt32(&server1Failures, 1)
			if failures <= 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server1.Close()

		server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			failures := atomic.AddInt32(&server2Failures, 1)
			if failures <= 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server2.Close()

		rc := common.NewResilientClient(
			common.RetryPolicy{
				MaxRetries:   3,
				InitialDelay: 50 * time.Millisecond,
				MaxDelay:     500 * time.Millisecond,
				Multiplier:   2.0,
				Jitter:       false,
				RetryableErrors: []string{"server error", "503"},
			},
			5,
			1*time.Second,
		)

		// Test with multiple endpoints
		endpoints := []string{server1.URL, server2.URL}
		successCount := 0

		for _, endpoint := range endpoints {
			err := rc.Execute(func() error {
				client := &http.Client{Timeout: 1 * time.Second}
				resp, err := client.Get(endpoint)
				if err != nil {
					return err
				}
				defer resp.Body.Close()
				
				if resp.StatusCode != http.StatusOK {
					return fmt.Errorf("server error: %d", resp.StatusCode)
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
		var requestCount int32
		latencies := []time.Duration{
			10 * time.Millisecond,
			500 * time.Millisecond,
			100 * time.Millisecond,
			1 * time.Second,
			50 * time.Millisecond,
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&requestCount, 1)
			if int(count) <= len(latencies) {
				time.Sleep(latencies[count-1])
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

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

		for i := 0; i < 5; i++ {
			err := rm.ExecuteWithContext(func() error {
				client := &http.Client{
					Timeout: 300 * time.Millisecond,
				}
				
				resp, err := client.Get(server.URL)
				if err != nil {
					if isTimeoutError(err) {
						timeoutCount++
					}
					return err
				}
				defer resp.Body.Close()
				
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
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&connectionCount, 1)
			
			// Simulate connection issues on first few attempts
			if count <= 2 {
				// Force connection close
				w.Header().Set("Connection", "close")
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			
			// Allow connection reuse
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Create a client with connection pooling
		transport := &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     90 * time.Second,
			DisableKeepAlives:   false,
		}
		client := &http.Client{
			Transport: transport,
			Timeout:   2 * time.Second,
		}

		rm := common.NewRetryManager(common.RetryPolicy{
			MaxRetries:   3,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     1 * time.Second,
			Multiplier:   2.0,
			Jitter:       false,
			RetryableErrors: []string{"server error", "503"},
		})

		err := rm.Execute(func() error {
			resp, err := client.Get(server.URL)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server error: %d", resp.StatusCode)
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