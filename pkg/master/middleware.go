package master

import (
	"net/http"
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

// timeoutMiddleware enforces request timeouts
func (s *Server) timeoutMiddleware(next http.Handler) http.Handler {
	timeout := s.config.Timeouts.HTTPRequest
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	
	return http.TimeoutHandler(next, timeout, "Request timeout")
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

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}