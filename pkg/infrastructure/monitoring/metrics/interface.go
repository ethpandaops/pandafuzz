package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Collector defines the interface for metrics collection
type Collector interface {
	// RecordMetric records a metric value with labels
	RecordMetric(name string, value float64, labels map[string]string)

	// RecordDuration records the duration of an operation
	RecordDuration(name string, start time.Time, labels map[string]string)

	// IncrementCounter increments a counter metric
	IncrementCounter(name string, labels map[string]string)

	// SetGauge sets a gauge metric value
	SetGauge(name string, value float64, labels map[string]string)

	// RecordHistogram records a value in a histogram
	RecordHistogram(name string, value float64, labels map[string]string)

	// RegisterCollector registers a custom Prometheus collector
	RegisterCollector(collector prometheus.Collector) error

	// Start starts the metrics collection service
	Start(ctx context.Context) error

	// Stop stops the metrics collection service
	Stop() error
}

// SystemMetrics represents system-level metrics
type SystemMetrics struct {
	// CPU usage percentage (0-100)
	CPUUsage float64

	// Memory usage in bytes
	MemoryUsage uint64

	// Number of goroutines
	Goroutines int

	// Heap allocation in bytes
	HeapAlloc uint64

	// System memory in bytes
	SysMemory uint64

	// Number of GC cycles completed
	GCCycles uint32

	// Time spent in GC
	GCPauseTotal time.Duration

	// Last GC pause duration
	LastGCPause time.Duration

	// Timestamp of the metrics collection
	Timestamp time.Time
}

// MetricType represents the type of a metric
type MetricType int

const (
	// CounterType represents a counter metric
	CounterType MetricType = iota
	// GaugeType represents a gauge metric
	GaugeType
	// HistogramType represents a histogram metric
	HistogramType
	// SummaryType represents a summary metric
	SummaryType
)

// MetricDefinition defines a metric that can be registered
type MetricDefinition struct {
	// Name is the metric name
	Name string

	// Help is the metric help text
	Help string

	// Type is the metric type
	Type MetricType

	// Labels are the label names for this metric
	Labels []string

	// Buckets are the histogram buckets (only for HistogramType)
	Buckets []float64

	// Objectives are the summary objectives (only for SummaryType)
	Objectives map[float64]float64
}

// Options configures the metrics collector
type Options struct {
	// Namespace is the Prometheus namespace
	Namespace string

	// Subsystem is the Prometheus subsystem
	Subsystem string

	// EnableSystemMetrics enables automatic system metrics collection
	EnableSystemMetrics bool

	// SystemMetricsInterval is the interval for collecting system metrics
	SystemMetricsInterval time.Duration

	// MetricsPath is the HTTP path for exposing metrics
	MetricsPath string

	// MetricsAddr is the address to expose metrics on
	MetricsAddr string

	// DefaultLabels are labels added to all metrics
	DefaultLabels map[string]string

	// Registry is the Prometheus registry to use (optional)
	Registry *prometheus.Registry
}

// DefaultOptions returns default options for the metrics collector
func DefaultOptions() *Options {
	return &Options{
		Namespace:             "pandafuzz",
		Subsystem:             "infrastructure",
		EnableSystemMetrics:   true,
		SystemMetricsInterval: 15 * time.Second,
		MetricsPath:           "/metrics",
		MetricsAddr:           ":9090",
		DefaultLabels:         make(map[string]string),
	}
}
