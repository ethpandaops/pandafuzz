package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// TracingConfig holds configuration for tracing middleware
type TracingConfig struct {
	// ServiceName is the name of the service for tracing
	ServiceName string

	// ServiceVersion is the version of the service
	ServiceVersion string

	// Environment is the deployment environment (dev, staging, prod)
	Environment string

	// SamplingRate controls the percentage of traces to sample (0.0 to 1.0)
	SamplingRate float64

	// SkipPaths is a list of paths to skip tracing
	SkipPaths []string

	// SkipMethods is a list of HTTP methods to skip tracing
	SkipMethods []string

	// Logger for tracing messages
	Logger logrus.FieldLogger

	// CustomAttributes allows adding custom attributes to spans
	CustomAttributes map[string]interface{}

	// PropagationHeaders defines which headers to use for trace propagation
	PropagationHeaders []string

	// EnableW3CTraceContext enables W3C Trace Context propagation
	EnableW3CTraceContext bool

	// EnableB3Propagation enables B3 propagation format
	EnableB3Propagation bool

	// SpanProcessor allows custom span processing
	SpanProcessor SpanProcessor
}

// SpanProcessor interface for processing spans
type SpanProcessor interface {
	OnStart(span *Span)
	OnEnd(span *Span)
}

// Span represents a tracing span with simplified OpenTelemetry-like interface
type Span struct {
	TraceID   string
	SpanID    string
	ParentID  string
	Operation string
	StartTime int64
	EndTime   int64
	Duration  int64
	Status    SpanStatus
	Tags      map[string]interface{}
	Logs      []SpanLog
	Baggage   map[string]string
	Context   context.Context
}

// SpanStatus represents the status of a span
type SpanStatus struct {
	Code    StatusCode
	Message string
	IsError bool
}

// StatusCode represents different span status codes
type StatusCode int

const (
	StatusCodeUnset StatusCode = iota
	StatusCodeOK
	StatusCodeError
)

// SpanLog represents a log entry within a span
type SpanLog struct {
	Timestamp int64
	Fields    map[string]interface{}
}

// TracingContextKey is used for storing tracing data in context
type TracingContextKey string

const (
	// SpanContextKey is the context key for the current span
	SpanContextKey TracingContextKey = "span"
	// TraceIDKey is the context key for trace ID
	TraceIDKey TracingContextKey = "trace_id"
	// SpanIDKey is the context key for span ID
	SpanIDKey TracingContextKey = "span_id"
)

// W3C Trace Context headers
const (
	TraceParentHeader = "traceparent"
	TraceStateHeader  = "tracestate"
)

// B3 Propagation headers
const (
	B3TraceIDHeader  = "X-B3-TraceId"
	B3SpanIDHeader   = "X-B3-SpanId"
	B3ParentIDHeader = "X-B3-ParentSpanId"
	B3SampledHeader  = "X-B3-Sampled"
	B3FlagsHeader    = "X-B3-Flags"
)

// DefaultTracingConfig returns a default tracing configuration
func DefaultTracingConfig() TracingConfig {
	return TracingConfig{
		ServiceName:           "pandafuzz-api",
		ServiceVersion:        "v1.0.0",
		Environment:           "development",
		SamplingRate:          1.0, // Sample all traces by default
		Logger:                logrus.NewEntry(logrus.StandardLogger()),
		CustomAttributes:      make(map[string]interface{}),
		PropagationHeaders:    []string{TraceParentHeader, TraceStateHeader},
		EnableW3CTraceContext: true,
		EnableB3Propagation:   false,
		SkipPaths: []string{
			"/health",
			"/metrics",
			"/favicon.ico",
		},
	}
}

// Tracing creates a tracing middleware with default configuration
func Tracing() func(http.Handler) http.Handler {
	return TracingWithConfig(DefaultTracingConfig())
}

// TracingWithConfig creates a tracing middleware with custom configuration
func TracingWithConfig(config TracingConfig) func(http.Handler) http.Handler {
	if config.Logger == nil {
		config.Logger = logrus.NewEntry(logrus.StandardLogger())
	}

	if config.ServiceName == "" {
		config.ServiceName = "unknown-service"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip tracing for configured paths
			for _, path := range config.SkipPaths {
				if strings.HasPrefix(r.URL.Path, path) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Skip tracing for configured methods
			for _, method := range config.SkipMethods {
				if strings.EqualFold(r.Method, method) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Extract or create trace context
			traceContext := extractTraceContext(r, config)

			// Create span
			span := createSpan(r, traceContext, config)

			// Add span to context
			ctx := context.WithValue(r.Context(), SpanContextKey, span)
			ctx = context.WithValue(ctx, TraceIDKey, span.TraceID)
			ctx = context.WithValue(ctx, SpanIDKey, span.SpanID)
			span.Context = ctx

			// Process span start
			if config.SpanProcessor != nil {
				config.SpanProcessor.OnStart(span)
			}

			// Add trace headers to response
			injectTraceHeaders(w, span, config)

			// Wrap response writer to capture response data
			ww := &tracingResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Start span timing
			span.startTiming()

			// Process request
			next.ServeHTTP(ww, r.WithContext(ctx))

			// End span timing and set status
			span.endTiming()
			span.setHTTPStatus(ww.statusCode)

			// Add final span attributes
			span.setHTTPAttributes(r, ww)

			// Process span end
			if config.SpanProcessor != nil {
				config.SpanProcessor.OnEnd(span)
			}

			// Log span completion
			logSpanCompletion(span, config)
		})
	}
}

// extractTraceContext extracts trace context from incoming request headers
func extractTraceContext(r *http.Request, config TracingConfig) map[string]string {
	context := make(map[string]string)

	// Extract W3C Trace Context
	if config.EnableW3CTraceContext {
		if traceparent := r.Header.Get(TraceParentHeader); traceparent != "" {
			context["traceparent"] = traceparent
		}
		if tracestate := r.Header.Get(TraceStateHeader); tracestate != "" {
			context["tracestate"] = tracestate
		}
	}

	// Extract B3 propagation
	if config.EnableB3Propagation {
		if traceID := r.Header.Get(B3TraceIDHeader); traceID != "" {
			context["b3_trace_id"] = traceID
		}
		if spanID := r.Header.Get(B3SpanIDHeader); spanID != "" {
			context["b3_span_id"] = spanID
		}
		if parentID := r.Header.Get(B3ParentIDHeader); parentID != "" {
			context["b3_parent_id"] = parentID
		}
	}

	return context
}

// createSpan creates a new span for the request
func createSpan(r *http.Request, traceContext map[string]string, config TracingConfig) *Span {
	span := &Span{
		TraceID:   generateTraceID(traceContext),
		SpanID:    generateSpanID(),
		ParentID:  extractParentSpanID(traceContext),
		Operation: fmt.Sprintf("%s %s", r.Method, normalizeEndpoint(r.URL.Path)),
		Tags:      make(map[string]interface{}),
		Logs:      make([]SpanLog, 0),
		Baggage:   make(map[string]string),
	}

	// Set basic HTTP attributes
	span.Tags["http.method"] = r.Method
	span.Tags["http.url"] = r.URL.String()
	span.Tags["http.path"] = r.URL.Path
	span.Tags["http.query"] = r.URL.RawQuery
	span.Tags["http.scheme"] = getScheme(r)
	span.Tags["http.host"] = r.Host
	span.Tags["http.user_agent"] = r.Header.Get("User-Agent")
	span.Tags["http.remote_addr"] = getClientIP(r)

	// Set service attributes
	span.Tags["service.name"] = config.ServiceName
	span.Tags["service.version"] = config.ServiceVersion
	span.Tags["service.environment"] = config.Environment

	// Add custom attributes
	for key, value := range config.CustomAttributes {
		span.Tags[key] = value
	}

	// Add correlation ID if available
	if correlationID := GetCorrelationID(r); correlationID != "" {
		span.Tags["correlation_id"] = correlationID
	}

	return span
}

// injectTraceHeaders injects trace context into response headers
func injectTraceHeaders(w http.ResponseWriter, span *Span, config TracingConfig) {
	if config.EnableW3CTraceContext {
		// Create W3C traceparent header
		traceparent := fmt.Sprintf("00-%s-%s-01", span.TraceID, span.SpanID)
		w.Header().Set(TraceParentHeader, traceparent)
	}

	if config.EnableB3Propagation {
		w.Header().Set(B3TraceIDHeader, span.TraceID)
		w.Header().Set(B3SpanIDHeader, span.SpanID)
		if span.ParentID != "" {
			w.Header().Set(B3ParentIDHeader, span.ParentID)
		}
		w.Header().Set(B3SampledHeader, "1")
	}

	// Add trace ID to standard response header
	w.Header().Set("X-Trace-ID", span.TraceID)
}

// logSpanCompletion logs the completion of a span
func logSpanCompletion(span *Span, config TracingConfig) {
	fields := logrus.Fields{
		"trace_id":    span.TraceID,
		"span_id":     span.SpanID,
		"parent_id":   span.ParentID,
		"operation":   span.Operation,
		"duration_ms": float64(span.Duration) / 1e6, // Convert nanoseconds to milliseconds
		"status_code": span.Status.Code,
		"is_error":    span.Status.IsError,
	}

	// Add selected span tags to log
	if method, ok := span.Tags["http.method"]; ok {
		fields["http_method"] = method
	}
	if path, ok := span.Tags["http.path"]; ok {
		fields["http_path"] = path
	}
	if statusCode, ok := span.Tags["http.status_code"]; ok {
		fields["http_status_code"] = statusCode
	}

	level := logrus.InfoLevel
	if span.Status.IsError {
		level = logrus.ErrorLevel
	}

	config.Logger.WithFields(fields).Log(level, "Span completed")
}

// Span methods

// startTiming starts the span timing
func (s *Span) startTiming() {
	s.StartTime = getCurrentTimestamp()
}

// endTiming ends the span timing
func (s *Span) endTiming() {
	s.EndTime = getCurrentTimestamp()
	s.Duration = s.EndTime - s.StartTime
}

// setHTTPStatus sets the span status based on HTTP status code
func (s *Span) setHTTPStatus(statusCode int) {
	s.Tags["http.status_code"] = statusCode

	if statusCode >= 400 {
		s.Status.IsError = true
		s.Status.Code = StatusCodeError
		if statusCode >= 500 {
			s.Status.Message = "Internal server error"
		} else {
			s.Status.Message = "Client error"
		}
	} else {
		s.Status.Code = StatusCodeOK
		s.Status.Message = "OK"
	}
}

// setHTTPAttributes sets HTTP-specific attributes on the span
func (s *Span) setHTTPAttributes(r *http.Request, w *tracingResponseWriter) {
	s.Tags["http.response_size"] = w.bytesWritten
	if r.ContentLength > 0 {
		s.Tags["http.request_size"] = r.ContentLength
	}
}

// AddTag adds a tag to the span
func (s *Span) AddTag(key string, value interface{}) {
	s.Tags[key] = value
}

// AddLog adds a log entry to the span
func (s *Span) AddLog(fields map[string]interface{}) {
	log := SpanLog{
		Timestamp: getCurrentTimestamp(),
		Fields:    fields,
	}
	s.Logs = append(s.Logs, log)
}

// SetBaggage sets a baggage item
func (s *Span) SetBaggage(key, value string) {
	s.Baggage[key] = value
}

// GetBaggage gets a baggage item
func (s *Span) GetBaggage(key string) string {
	return s.Baggage[key]
}

// tracingResponseWriter wraps http.ResponseWriter to capture response data
type tracingResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (w *tracingResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *tracingResponseWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.bytesWritten += n
	return n, err
}

// Helper functions

// generateTraceID generates a new trace ID or extracts from context
func generateTraceID(context map[string]string) string {
	// Try to extract from W3C context
	if traceparent, exists := context["traceparent"]; exists {
		parts := strings.Split(traceparent, "-")
		if len(parts) >= 2 {
			return parts[1]
		}
	}

	// Try to extract from B3 context
	if traceID, exists := context["b3_trace_id"]; exists {
		return traceID
	}

	// Generate new trace ID
	return generateRandomID(32)
}

// generateSpanID generates a new span ID
func generateSpanID() string {
	return generateRandomID(16)
}

// extractParentSpanID extracts parent span ID from context
func extractParentSpanID(context map[string]string) string {
	// Try to extract from W3C context
	if traceparent, exists := context["traceparent"]; exists {
		parts := strings.Split(traceparent, "-")
		if len(parts) >= 3 {
			return parts[2]
		}
	}

	// Try to extract from B3 context
	if parentID, exists := context["b3_parent_id"]; exists {
		return parentID
	}

	return ""
}

// generateRandomID generates a random ID of specified length
func generateRandomID(length int) string {
	bytes := make([]byte, length/2)
	for i := range bytes {
		bytes[i] = byte(getCurrentTimestamp() >> (i % 8))
	}
	return fmt.Sprintf("%x", bytes)[:length]
}

// getCurrentTimestamp returns current timestamp in nanoseconds
func getCurrentTimestamp() int64 {
	return time.Now().UnixNano()
}

// getScheme determines the scheme (http/https) from the request
func getScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if scheme := r.Header.Get("X-Forwarded-Proto"); scheme != "" {
		return scheme
	}
	return "http"
}

// Context helper functions

// GetSpan extracts the current span from request context
func GetSpan(r *http.Request) *Span {
	if span, ok := r.Context().Value(SpanContextKey).(*Span); ok {
		return span
	}
	return nil
}

// GetTraceID extracts trace ID from request context
func GetTraceID(r *http.Request) string {
	if traceID, ok := r.Context().Value(TraceIDKey).(string); ok {
		return traceID
	}
	return ""
}

// GetSpanID extracts span ID from request context
func GetSpanID(r *http.Request) string {
	if spanID, ok := r.Context().Value(SpanIDKey).(string); ok {
		return spanID
	}
	return ""
}

// AddSpanTag adds a tag to the current span
func AddSpanTag(r *http.Request, key string, value interface{}) {
	if span := GetSpan(r); span != nil {
		span.AddTag(key, value)
	}
}

// AddSpanLog adds a log to the current span
func AddSpanLog(r *http.Request, fields map[string]interface{}) {
	if span := GetSpan(r); span != nil {
		span.AddLog(fields)
	}
}

// SetSpanError marks the current span as having an error
func SetSpanError(r *http.Request, err error) {
	if span := GetSpan(r); span != nil {
		span.Status.IsError = true
		span.Status.Code = StatusCodeError
		span.Status.Message = err.Error()
		span.AddTag("error", true)
		span.AddTag("error.message", err.Error())
		span.AddLog(map[string]interface{}{
			"event":        "error",
			"error.object": err,
		})
	}
}
