package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
)

// MetricsConfig holds configuration for metrics middleware
type MetricsConfig struct {
	Namespace       string   // Prometheus namespace
	Subsystem       string   // Prometheus subsystem
	SkipPaths       []string // Paths to skip metrics collection
	Logger          logrus.FieldLogger
	CustomLabels    []string                              // Additional labels to extract from requests
	LabelExtractors map[string]func(*http.Request) string // Custom label extractors
}

// Metrics holds all the Prometheus metrics
type Metrics struct {
	requestDuration  *prometheus.HistogramVec
	requestsTotal    *prometheus.CounterVec
	requestsInFlight prometheus.Gauge
	responseSize     *prometheus.HistogramVec
	requestSize      *prometheus.HistogramVec
	businessMetrics  map[string]*prometheus.CounterVec
	customGauges     map[string]*prometheus.GaugeVec
	customHistograms map[string]*prometheus.HistogramVec
	logger           logrus.FieldLogger
}

// MetricsRecorder provides an interface for recording custom metrics
type MetricsRecorder interface {
	RecordBusinessMetric(name string, labels map[string]string)
	RecordGauge(name string, value float64, labels map[string]string)
	RecordHistogram(name string, value float64, labels map[string]string)
	RecordJobCreated(fuzzer, campaign string)
	RecordCrashFound(fuzzer, severity string)
	RecordCorpusSync(source, status string)
	RecordBotRegistration(botType, version string)
}

// Default histogram buckets for different metrics
var (
	durationBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
	sizeBuckets     = []float64{100, 1000, 10000, 100000, 1000000, 10000000, 100000000}
)

// NewMetrics creates a new metrics instance with Prometheus collectors
func NewMetrics(config MetricsConfig) *Metrics {
	if config.Namespace == "" {
		config.Namespace = "pandafuzz"
	}
	if config.Subsystem == "" {
		config.Subsystem = "api"
	}
	if config.Logger == nil {
		config.Logger = logrus.NewEntry(logrus.StandardLogger())
	}

	labels := []string{"method", "endpoint", "status_code"}
	labels = append(labels, config.CustomLabels...)

	m := &Metrics{
		requestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   durationBuckets,
			},
			labels,
		),

		requestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "requests_total",
				Help:      "Total number of HTTP requests",
			},
			labels,
		),

		requestsInFlight: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "requests_in_flight",
				Help:      "Number of HTTP requests currently being processed",
			},
		),

		responseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "response_size_bytes",
				Help:      "HTTP response size in bytes",
				Buckets:   sizeBuckets,
			},
			labels,
		),

		requestSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "request_size_bytes",
				Help:      "HTTP request size in bytes",
				Buckets:   sizeBuckets,
			},
			labels,
		),

		businessMetrics:  make(map[string]*prometheus.CounterVec),
		customGauges:     make(map[string]*prometheus.GaugeVec),
		customHistograms: make(map[string]*prometheus.HistogramVec),
		logger:           config.Logger,
	}

	// Initialize business-specific metrics
	m.initBusinessMetrics(config.Namespace)

	return m
}

// initBusinessMetrics initializes business-specific metrics for PandaFuzz
func (m *Metrics) initBusinessMetrics(namespace string) {
	// Jobs metrics
	m.businessMetrics["jobs_created"] = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "created_total",
			Help:      "Total number of fuzzing jobs created",
		},
		[]string{"fuzzer", "campaign"},
	)

	m.businessMetrics["jobs_completed"] = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "completed_total",
			Help:      "Total number of fuzzing jobs completed",
		},
		[]string{"fuzzer", "campaign", "status"},
	)

	// Crashes metrics
	m.businessMetrics["crashes_found"] = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "crashes",
			Name:      "found_total",
			Help:      "Total number of crashes found",
		},
		[]string{"fuzzer", "severity"},
	)

	m.businessMetrics["crashes_deduplicated"] = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "crashes",
			Name:      "deduplicated_total",
			Help:      "Total number of crashes deduplicated",
		},
		[]string{"algorithm"},
	)

	// Corpus metrics
	m.businessMetrics["corpus_sync"] = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "corpus",
			Name:      "sync_total",
			Help:      "Total number of corpus sync operations",
		},
		[]string{"source", "status"},
	)

	// Bot metrics
	m.businessMetrics["bot_registrations"] = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "bots",
			Name:      "registrations_total",
			Help:      "Total number of bot registrations",
		},
		[]string{"bot_type", "version"},
	)

	// Coverage metrics
	m.customGauges["coverage_percentage"] = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "coverage",
			Name:      "percentage",
			Help:      "Code coverage percentage",
		},
		[]string{"job_id", "fuzzer"},
	)

	// Performance metrics
	m.customHistograms["fuzzer_performance"] = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "fuzzer",
			Name:      "executions_per_second",
			Help:      "Fuzzer execution rate in executions per second",
			Buckets:   []float64{10, 50, 100, 500, 1000, 5000, 10000, 50000},
		},
		[]string{"fuzzer", "bot_id"},
	)
}

// RequestMetrics creates a metrics collection middleware
func RequestMetrics() func(http.Handler) http.Handler {
	return RequestMetricsWithConfig(MetricsConfig{})
}

// RequestMetricsWithConfig creates a metrics collection middleware with configuration
func RequestMetricsWithConfig(config MetricsConfig) func(http.Handler) http.Handler {
	metrics := NewMetrics(config)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip metrics for configured paths
			for _, path := range config.SkipPaths {
				if r.URL.Path == path {
					next.ServeHTTP(w, r)
					return
				}
			}

			start := time.Now()
			metrics.requestsInFlight.Inc()
			defer metrics.requestsInFlight.Dec()

			// Wrap response writer to capture response size and status code
			ww := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Process request
			next.ServeHTTP(ww, r)

			// Calculate metrics
			duration := time.Since(start).Seconds()
			endpoint := normalizeEndpoint(r.URL.Path)
			statusCode := strconv.Itoa(ww.statusCode)

			// Extract custom labels
			labels := map[string]string{
				"method":      r.Method,
				"endpoint":    endpoint,
				"status_code": statusCode,
			}

			// Add custom labels from extractors
			for label, extractor := range config.LabelExtractors {
				if value := extractor(r); value != "" {
					labels[label] = value
				}
			}

			// Convert labels map to slice for Prometheus
			labelValues := make([]string, 0, len(labels))
			for _, key := range []string{"method", "endpoint", "status_code"} {
				labelValues = append(labelValues, labels[key])
			}
			for _, customLabel := range config.CustomLabels {
				if value, exists := labels[customLabel]; exists {
					labelValues = append(labelValues, value)
				} else {
					labelValues = append(labelValues, "")
				}
			}

			// Record metrics
			metrics.requestDuration.WithLabelValues(labelValues...).Observe(duration)
			metrics.requestsTotal.WithLabelValues(labelValues...).Inc()

			// Record request and response sizes
			if r.ContentLength > 0 {
				metrics.requestSize.WithLabelValues(labelValues...).Observe(float64(r.ContentLength))
			}
			if ww.bytesWritten > 0 {
				metrics.responseSize.WithLabelValues(labelValues...).Observe(float64(ww.bytesWritten))
			}

			config.Logger.WithFields(logrus.Fields{
				"method":        r.Method,
				"endpoint":      endpoint,
				"status_code":   statusCode,
				"duration_ms":   duration * 1000,
				"response_size": ww.bytesWritten,
			}).Debug("Request metrics recorded")
		})
	}
}

// RecordBusinessMetric records a business-specific metric
func (m *Metrics) RecordBusinessMetric(name string, labels map[string]string) {
	if metric, exists := m.businessMetrics[name]; exists {
		// For business metrics, we know the label order from initialization
		// This is a simplified approach - in production you'd want more robust label handling
		labelValues := make([]string, 0, len(labels))

		// Extract labels in the order they were defined during metric creation
		switch name {
		case "jobs_created":
			labelValues = append(labelValues, getOrDefault(labels, "fuzzer", ""))
			labelValues = append(labelValues, getOrDefault(labels, "campaign", ""))
		case "jobs_completed":
			labelValues = append(labelValues, getOrDefault(labels, "fuzzer", ""))
			labelValues = append(labelValues, getOrDefault(labels, "campaign", ""))
			labelValues = append(labelValues, getOrDefault(labels, "status", ""))
		case "crashes_found":
			labelValues = append(labelValues, getOrDefault(labels, "fuzzer", ""))
			labelValues = append(labelValues, getOrDefault(labels, "severity", ""))
		case "crashes_deduplicated":
			labelValues = append(labelValues, getOrDefault(labels, "algorithm", ""))
		case "corpus_sync":
			labelValues = append(labelValues, getOrDefault(labels, "source", ""))
			labelValues = append(labelValues, getOrDefault(labels, "status", ""))
		case "bot_registrations":
			labelValues = append(labelValues, getOrDefault(labels, "bot_type", ""))
			labelValues = append(labelValues, getOrDefault(labels, "version", ""))
		}

		metric.WithLabelValues(labelValues...).Inc()
	} else {
		m.logger.WithField("metric", name).Warn("Unknown business metric")
	}
}

// RecordGauge records a gauge metric
func (m *Metrics) RecordGauge(name string, value float64, labels map[string]string) {
	if gauge, exists := m.customGauges[name]; exists {
		labelValues := make([]string, 0, len(labels))

		// Handle known gauge metrics
		switch name {
		case "coverage_percentage":
			labelValues = append(labelValues, getOrDefault(labels, "job_id", ""))
			labelValues = append(labelValues, getOrDefault(labels, "fuzzer", ""))
		}

		gauge.WithLabelValues(labelValues...).Set(value)
	} else {
		m.logger.WithField("gauge", name).Warn("Unknown gauge metric")
	}
}

// RecordHistogram records a histogram metric
func (m *Metrics) RecordHistogram(name string, value float64, labels map[string]string) {
	if histogram, exists := m.customHistograms[name]; exists {
		labelValues := make([]string, 0, len(labels))

		// Handle known histogram metrics
		switch name {
		case "fuzzer_performance":
			labelValues = append(labelValues, getOrDefault(labels, "fuzzer", ""))
			labelValues = append(labelValues, getOrDefault(labels, "bot_id", ""))
		}

		histogram.WithLabelValues(labelValues...).Observe(value)
	} else {
		m.logger.WithField("histogram", name).Warn("Unknown histogram metric")
	}
}

// RecordJobCreated records a job creation event
func (m *Metrics) RecordJobCreated(fuzzer, campaign string) {
	m.RecordBusinessMetric("jobs_created", map[string]string{
		"fuzzer":   fuzzer,
		"campaign": campaign,
	})
}

// RecordCrashFound records a crash discovery event
func (m *Metrics) RecordCrashFound(fuzzer, severity string) {
	m.RecordBusinessMetric("crashes_found", map[string]string{
		"fuzzer":   fuzzer,
		"severity": severity,
	})
}

// RecordCorpusSync records a corpus synchronization event
func (m *Metrics) RecordCorpusSync(source, status string) {
	m.RecordBusinessMetric("corpus_sync", map[string]string{
		"source": source,
		"status": status,
	})
}

// RecordBotRegistration records a bot registration event
func (m *Metrics) RecordBotRegistration(botType, version string) {
	m.RecordBusinessMetric("bot_registrations", map[string]string{
		"bot_type": botType,
		"version":  version,
	})
}

// GetMetricsRecorder returns a metrics recorder instance
func GetMetricsRecorder(r *http.Request) MetricsRecorder {
	// In a real implementation, you might store the metrics instance in context
	// For now, return a default instance
	return NewMetrics(MetricsConfig{})
}

// responseWriter wraps http.ResponseWriter to capture response metrics
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(data)
	rw.bytesWritten += n
	return n, err
}

// normalizeEndpoint normalizes URL paths for metrics to avoid high cardinality
func normalizeEndpoint(path string) string {
	// Remove query parameters
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	// Normalize common ID patterns
	path = strings.ReplaceAll(path, "/api/v1/", "/api/v1/")
	path = normalizeIDs(path)

	return path
}

// normalizeIDs replaces ID patterns with placeholders to reduce cardinality
func normalizeIDs(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		// Replace UUIDs and numeric IDs with placeholders
		if isUUID(part) {
			parts[i] = "{uuid}"
		} else if isNumeric(part) {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

// isUUID checks if a string looks like a UUID
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// isNumeric checks if a string is numeric
func isNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// getOrDefault returns the value for a key from map or default value if not found
func getOrDefault(m map[string]string, key, defaultValue string) string {
	if value, exists := m[key]; exists {
		return value
	}
	return defaultValue
}
