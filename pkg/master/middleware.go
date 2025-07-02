package master

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// loggingMiddleware logs HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap ResponseWriter to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Process request
		next.ServeHTTP(wrapped, r)

		// Log request
		duration := time.Since(start)
		s.logger.WithFields(logrus.Fields{
			"method":     r.Method,
			"path":       r.URL.Path,
			"status":     wrapped.statusCode,
			"duration":   duration,
			"remote_ip":  r.RemoteAddr,
			"user_agent": r.UserAgent(),
		}).Info("HTTP request processed")

		// Update metrics
		s.updateRequestMetrics(duration, wrapped.statusCode >= 400)
	})
}

// metricsMiddleware tracks request metrics
func (s *Server) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.stats.ActiveRequests++
		s.stats.RequestCount++
		s.stats.LastRequest = time.Now()
		s.stats.TotalConnections++
		s.mu.Unlock()

		defer func() {
			s.mu.Lock()
			s.stats.ActiveRequests--
			s.mu.Unlock()
		}()

		next.ServeHTTP(w, r)
	})
}

// recoveryMiddleware handles panics
func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				s.logger.WithFields(logrus.Fields{
					"error":  err,
					"method": r.Method,
					"path":   r.URL.Path,
				}).Error("Panic recovered in HTTP handler")

				http.Error(w, "Internal Server Error", http.StatusInternalServerError)

				s.mu.Lock()
				s.stats.ErrorCount++
				s.mu.Unlock()
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// timeoutMiddleware enforces request timeouts with configurable duration.
// It wraps the HTTP handler with a timeout context and returns a 504 Gateway Timeout
// status if the request exceeds the specified timeout duration.
//
// The middleware:
// - Creates a context with the specified timeout (or uses default from config)
// - Processes the request in a goroutine
// - Returns 504 Gateway Timeout with a JSON error response if timeout occurs
// - Prevents writing to the response after timeout has occurred
// - Logs timeout events and updates error metrics
//
// Usage:
//
//	router.Use(s.timeoutMiddleware(30 * time.Second))
//	// Or use 0 to use the default from config
//	router.Use(s.timeoutMiddleware(0))
func (s *Server) timeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// Use default timeout if not specified
		if timeout == 0 {
			timeout = s.config.Timeouts.HTTPRequest
			if timeout == 0 {
				timeout = 30 * time.Second
			}
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Create a context with timeout
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			// Create a channel to signal completion
			done := make(chan bool, 1)

			// Create a custom response writer to track if response has been written
			tw := &timeoutResponseWriter{
				ResponseWriter: w,
				written:        false,
			}

			go func() {
				// Process the request with the timeout context
				next.ServeHTTP(tw, r.WithContext(ctx))
				done <- true
			}()

			select {
			case <-done:
				// Request completed successfully
				return
			case <-ctx.Done():
				// Timeout occurred
				tw.mu.Lock()
				defer tw.mu.Unlock()

				// Only write error if response hasn't been written yet
				if !tw.written {
					// Set proper timeout status code
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusGatewayTimeout)

					// Write timeout error response
					errorResponse := map[string]interface{}{
						"error":   "Gateway Timeout",
						"message": fmt.Sprintf("Request exceeded timeout of %v", timeout),
						"code":    http.StatusGatewayTimeout,
					}

					if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
						s.logger.WithError(err).Error("Failed to write timeout response")
					}

					// Log the timeout
					s.logger.WithFields(logrus.Fields{
						"method":    r.Method,
						"path":      r.URL.Path,
						"timeout":   timeout,
						"remote_ip": r.RemoteAddr,
					}).Warn("Request timed out")

					// Update error metrics
					s.mu.Lock()
					s.stats.ErrorCount++
					s.mu.Unlock()
				}
			}
		})
	}
}

// timeoutResponseWriter wraps http.ResponseWriter to track if response has been written
type timeoutResponseWriter struct {
	http.ResponseWriter
	mu      sync.Mutex
	written bool
}

func (tw *timeoutResponseWriter) WriteHeader(code int) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if !tw.written {
		tw.written = true
		tw.ResponseWriter.WriteHeader(code)
	}
}

func (tw *timeoutResponseWriter) Write(b []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if !tw.written {
		tw.written = true
	}
	return tw.ResponseWriter.Write(b)
}

// corsMiddleware handles CORS headers
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		if len(s.config.Server.CORSOrigins) == 0 {
			allowed = true // Allow all if none specified
		} else {
			for _, allowedOrigin := range s.config.Server.CORSOrigins {
				if origin == allowedOrigin || allowedOrigin == "*" {
					allowed = true
					break
				}
			}
		}

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware implements rate limiting
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	// This is a simplified rate limiter
	// In production, you'd use a more sophisticated implementation
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Implement proper rate limiting based on config
		next.ServeHTTP(w, r)
	})
}

// requestSizeLimitMiddleware limits the size of request bodies to prevent OOM attacks
func (s *Server) requestSizeLimitMiddleware(maxSize int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip size limit for GET requests
			if r.Method == http.MethodGet {
				next.ServeHTTP(w, r)
				return
			}

			// Apply different limits based on the endpoint
			var limit int64
			switch {
			case r.URL.Path == "/api/v1/jobs/upload" || r.URL.Path == "/api/v1/results/corpus":
				// Allow larger uploads for binary/corpus files (100MB)
				limit = 100 * 1024 * 1024
			case r.URL.Path == "/api/v1/results/crash":
				// Allow medium size for crash reports (10MB)
				limit = 10 * 1024 * 1024
			default:
				// Default limit for regular API calls (1MB)
				limit = maxSize
				if limit == 0 {
					limit = 1 * 1024 * 1024
				}
			}

			// Limit the request body size
			r.Body = http.MaxBytesReader(w, r.Body, limit)

			// Check content length header
			if r.ContentLength > limit {
				s.logger.WithFields(logrus.Fields{
					"method":         r.Method,
					"path":           r.URL.Path,
					"content_length": r.ContentLength,
					"limit":          limit,
					"remote_ip":      r.RemoteAddr,
				}).Warn("Request body too large")

				http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
				return
			}

			// Process the request
			next.ServeHTTP(w, r)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
