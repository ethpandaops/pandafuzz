package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// LoggingMiddleware adds request logging and request ID generation
type LoggingMiddleware struct {
	logger logrus.FieldLogger
}

// NewLoggingMiddleware creates a new logging middleware
func NewLoggingMiddleware(logger logrus.FieldLogger) *LoggingMiddleware {
	return &LoggingMiddleware{
		logger: logger,
	}
}

// Middleware returns the HTTP middleware handler
func (lm *LoggingMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		// Generate or extract request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Add request ID to context
		ctx := context.WithValue(r.Context(), "request_id", requestID)
		r = r.WithContext(ctx)

		// Create response writer wrapper to capture status code
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Set request ID in response header
		w.Header().Set("X-Request-ID", requestID)

		// Create logger with request context
		logger := lm.logger.WithFields(logrus.Fields{
			"request_id": requestID,
			"method":     r.Method,
			"path":       r.URL.Path,
			"remote_ip":  getRemoteIP(r),
			"user_agent": r.UserAgent(),
		})

		// Log request start
		logger.Debug("Request started")

		// Call the next handler
		next.ServeHTTP(wrapped, r)

		// Calculate duration
		duration := time.Since(startTime)

		// Log request completion
		logger.WithFields(logrus.Fields{
			"status":        wrapped.statusCode,
			"duration_ms":   duration.Milliseconds(),
			"bytes_written": wrapped.bytesWritten,
		}).Info("Request completed")

		// Log slow requests as warnings
		if duration > 5*time.Second {
			logger.WithFields(logrus.Fields{
				"duration_seconds": duration.Seconds(),
			}).Warn("Slow request detected")
		}
	})
}

// responseWriter wraps http.ResponseWriter to capture status code and bytes written
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(bytes []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(bytes)
	rw.bytesWritten += n
	return n, err
}

// getRemoteIP extracts the remote IP address from the request
func getRemoteIP(r *http.Request) string {
	// Check X-Forwarded-For header
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		return forwarded
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// RequestIDFromContext extracts request ID from context
func RequestIDFromContext(ctx context.Context) string {
	if reqID := ctx.Value("request_id"); reqID != nil {
		if id, ok := reqID.(string); ok {
			return id
		}
	}
	return ""
}

// WithRequestID adds request ID to logger fields
func WithRequestID(ctx context.Context, logger logrus.FieldLogger) logrus.FieldLogger {
	if reqID := RequestIDFromContext(ctx); reqID != "" {
		return logger.WithField("request_id", reqID)
	}
	return logger
}
