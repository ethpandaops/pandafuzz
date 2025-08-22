package middleware

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// LoggingConfig holds configuration for logging middleware
type LoggingConfig struct {
	// Logger is the logrus logger instance to use
	Logger logrus.FieldLogger

	// SkipPaths is a list of paths to skip logging
	SkipPaths []string

	// SkipMethods is a list of HTTP methods to skip logging
	SkipMethods []string

	// LogRequestBody enables logging of request bodies
	LogRequestBody bool

	// LogResponseBody enables logging of response bodies
	LogResponseBody bool

	// MaxBodySize limits the size of request/response bodies to log
	MaxBodySize int

	// SensitiveHeaders is a list of headers to mask in logs
	SensitiveHeaders []string

	// SensitiveFields is a list of JSON fields to mask in request/response bodies
	SensitiveFields []string

	// CorrelationIDHeader is the header name for correlation ID
	CorrelationIDHeader string

	// LogLevel is the log level for request logs
	LogLevel logrus.Level

	// IncludeUserAgent enables logging of User-Agent header
	IncludeUserAgent bool

	// IncludeTimings enables logging of detailed timing information
	IncludeTimings bool

	// CustomFieldExtractors allows adding custom fields to log entries
	CustomFieldExtractors map[string]func(*http.Request) interface{}
}

// LoggingContextKey is used for storing logging-related data in request context
type LoggingContextKey string

const (
	// CorrelationIDKey is the context key for correlation ID
	CorrelationIDKey LoggingContextKey = "correlation_id"
	// RequestStartTimeKey is the context key for request start time
	RequestStartTimeKey LoggingContextKey = "request_start_time"
	// RequestLoggerKey is the context key for request logger
	RequestLoggerKey LoggingContextKey = "request_logger"
)

// ResponseCapture wraps http.ResponseWriter to capture response data
type ResponseCapture struct {
	http.ResponseWriter
	statusCode   int
	body         *bytes.Buffer
	captureBody  bool
	bytesWritten int
}

// DefaultLoggingConfig returns a default logging configuration
func DefaultLoggingConfig() LoggingConfig {
	return LoggingConfig{
		Logger: logrus.NewEntry(logrus.StandardLogger()),
		SkipPaths: []string{
			"/health",
			"/metrics",
			"/favicon.ico",
		},
		LogRequestBody:      false,
		LogResponseBody:     false,
		MaxBodySize:         4096, // 4KB
		CorrelationIDHeader: "X-Correlation-ID",
		LogLevel:            logrus.InfoLevel,
		IncludeUserAgent:    true,
		IncludeTimings:      true,
		SensitiveHeaders: []string{
			"Authorization",
			"X-API-Key",
			"Cookie",
			"Set-Cookie",
			"X-Auth-Token",
		},
		SensitiveFields: []string{
			"password",
			"secret",
			"token",
			"key",
			"auth",
			"credential",
			"private",
		},
	}
}

// RequestLogger creates a logging middleware
func RequestLogger() func(http.Handler) http.Handler {
	return RequestLoggerWithConfig(DefaultLoggingConfig())
}

// RequestLoggerWithConfig creates a logging middleware with configuration
func RequestLoggerWithConfig(config LoggingConfig) func(http.Handler) http.Handler {
	if config.Logger == nil {
		config.Logger = logrus.NewEntry(logrus.StandardLogger())
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Skip logging for configured paths
			for _, path := range config.SkipPaths {
				if strings.HasPrefix(r.URL.Path, path) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Skip logging for configured methods
			for _, method := range config.SkipMethods {
				if strings.EqualFold(r.Method, method) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Generate or extract correlation ID
			correlationID := extractOrGenerateCorrelationID(r, config.CorrelationIDHeader)

			// Create request-scoped logger
			requestLogger := config.Logger.WithFields(logrus.Fields{
				"correlation_id": correlationID,
				"request_id":     correlationID, // Alias for compatibility
				"method":         r.Method,
				"path":           r.URL.Path,
				"remote_addr":    getClientIP(r),
			})

			// Add custom fields
			for fieldName, extractor := range config.CustomFieldExtractors {
				if value := extractor(r); value != nil {
					requestLogger = requestLogger.WithField(fieldName, value)
				}
			}

			// Add correlation ID to response headers
			w.Header().Set(config.CorrelationIDHeader, correlationID)

			// Store logger and correlation ID in context
			ctx := context.WithValue(r.Context(), CorrelationIDKey, correlationID)
			ctx = context.WithValue(ctx, RequestStartTimeKey, start)
			ctx = context.WithValue(ctx, RequestLoggerKey, requestLogger)
			r = r.WithContext(ctx)

			// Log request
			logRequest(requestLogger, r, config)

			// Wrap response writer to capture response data
			rc := &ResponseCapture{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
				body:           &bytes.Buffer{},
				captureBody:    config.LogResponseBody,
			}

			// Process request
			next.ServeHTTP(rc, r)

			// Calculate duration
			duration := time.Since(start)

			// Log response
			logResponse(requestLogger, r, rc, duration, config)
		})
	}
}

// logRequest logs the incoming request
func logRequest(logger logrus.FieldLogger, r *http.Request, config LoggingConfig) {
	fields := logrus.Fields{
		"event":          "request_start",
		"url":            r.URL.String(),
		"query":          maskSensitiveData(r.URL.RawQuery, config.SensitiveFields),
		"proto":          r.Proto,
		"headers":        maskHeaders(r.Header, config.SensitiveHeaders),
		"content_length": r.ContentLength,
	}

	if config.IncludeUserAgent {
		fields["user_agent"] = r.Header.Get("User-Agent")
	}

	// Add request body if enabled
	if config.LogRequestBody && r.ContentLength > 0 && r.ContentLength <= int64(config.MaxBodySize) {
		if body := readAndRestoreBody(r, config.MaxBodySize); body != "" {
			fields["request_body"] = maskSensitiveData(body, config.SensitiveFields)
		}
	}

	logger.WithFields(fields).Log(config.LogLevel, "HTTP request received")
}

// logResponse logs the outgoing response
func logResponse(logger logrus.FieldLogger, r *http.Request, rc *ResponseCapture, duration time.Duration, config LoggingConfig) {
	fields := logrus.Fields{
		"event":         "request_complete",
		"status_code":   rc.statusCode,
		"response_size": rc.bytesWritten,
		"duration_ms":   float64(duration.Nanoseconds()) / 1e6,
	}

	if config.IncludeTimings {
		fields["duration_ns"] = duration.Nanoseconds()
		fields["duration_seconds"] = duration.Seconds()
	}

	// Add response body if enabled and captured
	if config.LogResponseBody && rc.captureBody && rc.body.Len() > 0 {
		responseBody := rc.body.String()
		if len(responseBody) > config.MaxBodySize {
			responseBody = responseBody[:config.MaxBodySize] + "... (truncated)"
		}
		fields["response_body"] = maskSensitiveData(responseBody, config.SensitiveFields)
	}

	// Determine log level based on status code
	logLevel := config.LogLevel
	if rc.statusCode >= 400 && rc.statusCode < 500 {
		logLevel = logrus.WarnLevel
	} else if rc.statusCode >= 500 {
		logLevel = logrus.ErrorLevel
	}

	logger.WithFields(fields).Log(logLevel, "HTTP request completed")
}

// extractOrGenerateCorrelationID extracts correlation ID from header or generates new one
func extractOrGenerateCorrelationID(r *http.Request, headerName string) string {
	if id := r.Header.Get(headerName); id != "" {
		return id
	}
	return generateCorrelationID()
}

// generateCorrelationID generates a new correlation ID
func generateCorrelationID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if random generation fails
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// getClientIP extracts the real client IP address
func getClientIP(r *http.Request) string {
	// Check X-Real-IP header
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	// Check X-Forwarded-For header
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// Take the first IP in the list
		if ips := strings.Split(forwarded, ","); len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// maskHeaders masks sensitive headers in the header map
func maskHeaders(headers http.Header, sensitiveHeaders []string) map[string]interface{} {
	masked := make(map[string]interface{})

	for name, values := range headers {
		if isSensitiveHeader(name, sensitiveHeaders) {
			masked[name] = "[MASKED]"
		} else {
			if len(values) == 1 {
				masked[name] = values[0]
			} else {
				masked[name] = values
			}
		}
	}

	return masked
}

// isSensitiveHeader checks if a header should be masked
func isSensitiveHeader(headerName string, sensitiveHeaders []string) bool {
	for _, sensitive := range sensitiveHeaders {
		if strings.EqualFold(headerName, sensitive) {
			return true
		}
	}
	return false
}

// maskSensitiveData masks sensitive data in strings using regex patterns
func maskSensitiveData(data string, sensitiveFields []string) string {
	if data == "" {
		return data
	}

	masked := data

	// Mask JSON fields
	for _, field := range sensitiveFields {
		// Pattern for JSON field: "field": "value" or "field":"value"
		pattern := fmt.Sprintf(`"(%s)"\s*:\s*"([^"]*)"`, regexp.QuoteMeta(field))
		re := regexp.MustCompile(pattern)
		masked = re.ReplaceAllString(masked, `"$1": "[MASKED]"`)

		// Pattern for form data: field=value
		pattern = fmt.Sprintf(`(%s)=([^&\s]*)`, regexp.QuoteMeta(field))
		re = regexp.MustCompile(pattern)
		masked = re.ReplaceAllString(masked, `$1=[MASKED]`)
	}

	// Mask common sensitive patterns
	patterns := []struct {
		pattern string
		replace string
	}{
		{`"password"\s*:\s*"[^"]*"`, `"password": "[MASKED]"`},
		{`"token"\s*:\s*"[^"]*"`, `"token": "[MASKED]"`},
		{`"secret"\s*:\s*"[^"]*"`, `"secret": "[MASKED]"`},
		{`password=([^&\s]*)`, `password=[MASKED]`},
		{`token=([^&\s]*)`, `token=[MASKED]`},
		{`key=([^&\s]*)`, `key=[MASKED]`},
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p.pattern)
		masked = re.ReplaceAllString(masked, p.replace)
	}

	return masked
}

// readAndRestoreBody reads the request body and restores it for further processing
func readAndRestoreBody(r *http.Request, maxSize int) string {
	if r.Body == nil {
		return ""
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, int64(maxSize)))
	if err != nil {
		return ""
	}

	// Restore the body for further reading
	r.Body = io.NopCloser(bytes.NewReader(body))

	return string(body)
}

// WriteHeader captures the status code
func (rc *ResponseCapture) WriteHeader(statusCode int) {
	rc.statusCode = statusCode
	rc.ResponseWriter.WriteHeader(statusCode)
}

// Write captures the response body and tracks bytes written
func (rc *ResponseCapture) Write(data []byte) (int, error) {
	n, err := rc.ResponseWriter.Write(data)
	rc.bytesWritten += n

	// Capture response body if enabled
	if rc.captureBody && rc.body.Len() < 4096 { // Limit capture size
		rc.body.Write(data[:n])
	}

	return n, err
}

// GetCorrelationID extracts correlation ID from request context
func GetCorrelationID(r *http.Request) string {
	if id, ok := r.Context().Value(CorrelationIDKey).(string); ok {
		return id
	}
	return ""
}

// GetRequestLogger extracts the request-scoped logger from context
func GetRequestLogger(r *http.Request) logrus.FieldLogger {
	if logger, ok := r.Context().Value(RequestLoggerKey).(logrus.FieldLogger); ok {
		return logger
	}
	return logrus.NewEntry(logrus.StandardLogger())
}

// GetRequestStartTime extracts the request start time from context
func GetRequestStartTime(r *http.Request) time.Time {
	if startTime, ok := r.Context().Value(RequestStartTimeKey).(time.Time); ok {
		return startTime
	}
	return time.Now()
}

// LogError logs an error with request context
func LogError(r *http.Request, err error, message string) {
	logger := GetRequestLogger(r)
	logger.WithError(err).Error(message)
}

// LogWarn logs a warning with request context
func LogWarn(r *http.Request, message string, fields logrus.Fields) {
	logger := GetRequestLogger(r)
	if fields != nil {
		logger = logger.WithFields(fields)
	}
	logger.Warn(message)
}

// LogInfo logs an info message with request context
func LogInfo(r *http.Request, message string, fields logrus.Fields) {
	logger := GetRequestLogger(r)
	if fields != nil {
		logger = logger.WithFields(fields)
	}
	logger.Info(message)
}

// LogDebug logs a debug message with request context
func LogDebug(r *http.Request, message string, fields logrus.Fields) {
	logger := GetRequestLogger(r)
	if fields != nil {
		logger = logger.WithFields(fields)
	}
	logger.Debug(message)
}
