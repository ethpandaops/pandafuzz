package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/monitoring/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

// Verify interface compliance
var _ metrics.Collector = (*Collector)(nil)

// Collector implements the metrics.Collector interface using Prometheus
type Collector struct {
	logger  logrus.FieldLogger
	options *metrics.Options

	// Metrics storage
	counters   map[string]*prometheus.CounterVec
	gauges     map[string]*prometheus.GaugeVec
	histograms map[string]*prometheus.HistogramVec
	summaries  map[string]*prometheus.SummaryVec
	mu         sync.RWMutex

	// System metrics collectors
	cpuGauge        prometheus.Gauge
	memoryGauge     prometheus.Gauge
	goroutinesGauge prometheus.Gauge
	heapAllocGauge  prometheus.Gauge
	sysMemoryGauge  prometheus.Gauge
	gcCyclesGauge   prometheus.Gauge
	gcPauseGauge    prometheus.Gauge

	// Control
	server   *http.Server
	stopCh   chan struct{}
	wg       sync.WaitGroup
	registry *prometheus.Registry
}

// NewCollector creates a new Prometheus metrics collector
func NewCollector(logger logrus.FieldLogger, options *metrics.Options) (*Collector, error) {
	if logger == nil {
		logger = logrus.New().WithField("component", "metrics")
	}
	if options == nil {
		options = metrics.DefaultOptions()
	}

	registry := options.Registry
	if registry == nil {
		registry = prometheus.NewRegistry()
	}

	c := &Collector{
		logger:     logger,
		options:    options,
		counters:   make(map[string]*prometheus.CounterVec),
		gauges:     make(map[string]*prometheus.GaugeVec),
		histograms: make(map[string]*prometheus.HistogramVec),
		summaries:  make(map[string]*prometheus.SummaryVec),
		stopCh:     make(chan struct{}),
		registry:   registry,
	}

	// Register system metrics if enabled
	if options.EnableSystemMetrics {
		if err := c.registerSystemMetrics(); err != nil {
			return nil, fmt.Errorf("failed to register system metrics: %w", err)
		}
	}

	return c, nil
}

// RecordMetric records a metric value with labels
func (c *Collector) RecordMetric(name string, value float64, labels map[string]string) {
	c.mu.RLock()
	gauge, exists := c.gauges[name]
	c.mu.RUnlock()

	if !exists {
		c.logger.WithField("metric", name).Warn("metric not registered, auto-creating gauge")
		c.createGauge(name, fmt.Sprintf("Auto-created gauge for %s", name), c.getLabelNames(labels))
		c.mu.RLock()
		gauge = c.gauges[name]
		c.mu.RUnlock()
	}

	if gauge != nil {
		gauge.With(c.mergeLabels(labels)).Set(value)
	}
}

// RecordDuration records the duration of an operation
func (c *Collector) RecordDuration(name string, start time.Time, labels map[string]string) {
	duration := time.Since(start).Seconds()
	c.RecordHistogram(name, duration, labels)
}

// IncrementCounter increments a counter metric
func (c *Collector) IncrementCounter(name string, labels map[string]string) {
	c.mu.RLock()
	counter, exists := c.counters[name]
	c.mu.RUnlock()

	if !exists {
		c.logger.WithField("metric", name).Warn("counter not registered, auto-creating")
		c.createCounter(name, fmt.Sprintf("Auto-created counter for %s", name), c.getLabelNames(labels))
		c.mu.RLock()
		counter = c.counters[name]
		c.mu.RUnlock()
	}

	if counter != nil {
		counter.With(c.mergeLabels(labels)).Inc()
	}
}

// SetGauge sets a gauge metric value
func (c *Collector) SetGauge(name string, value float64, labels map[string]string) {
	c.RecordMetric(name, value, labels)
}

// RecordHistogram records a value in a histogram
func (c *Collector) RecordHistogram(name string, value float64, labels map[string]string) {
	c.mu.RLock()
	histogram, exists := c.histograms[name]
	c.mu.RUnlock()

	if !exists {
		c.logger.WithField("metric", name).Warn("histogram not registered, auto-creating")
		c.createHistogram(name, fmt.Sprintf("Auto-created histogram for %s", name), c.getLabelNames(labels), prometheus.DefBuckets)
		c.mu.RLock()
		histogram = c.histograms[name]
		c.mu.RUnlock()
	}

	if histogram != nil {
		histogram.With(c.mergeLabels(labels)).Observe(value)
	}
}

// RegisterCollector registers a custom Prometheus collector
func (c *Collector) RegisterCollector(collector prometheus.Collector) error {
	return c.registry.Register(collector)
}

// Start starts the metrics collection service
func (c *Collector) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.Handle(c.options.MetricsPath, promhttp.HandlerFor(c.registry, promhttp.HandlerOpts{
		Registry: c.registry,
	}))

	c.server = &http.Server{
		Addr:    c.options.MetricsAddr,
		Handler: mux,
	}

	// Start system metrics collection if enabled
	if c.options.EnableSystemMetrics {
		c.wg.Add(1)
		go c.collectSystemMetrics(ctx)
	}

	// Start HTTP server
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.logger.WithField("addr", c.options.MetricsAddr).Info("starting metrics server")
		if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			c.logger.WithError(err).Error("metrics server error")
		}
	}()

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.server.Shutdown(shutdownCtx); err != nil {
			c.logger.WithError(err).Error("failed to shutdown metrics server")
		}
	}()

	return nil
}

// Stop stops the metrics collection service
func (c *Collector) Stop() error {
	close(c.stopCh)

	if c.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.server.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown metrics server: %w", err)
		}
	}

	c.wg.Wait()
	return nil
}

// registerSystemMetrics registers system-level metrics
func (c *Collector) registerSystemMetrics() error {
	c.cpuGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   c.options.Namespace,
		Subsystem:   c.options.Subsystem,
		Name:        "cpu_usage_percent",
		Help:        "Current CPU usage percentage",
		ConstLabels: c.options.DefaultLabels,
	})

	c.memoryGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   c.options.Namespace,
		Subsystem:   c.options.Subsystem,
		Name:        "memory_usage_bytes",
		Help:        "Current memory usage in bytes",
		ConstLabels: c.options.DefaultLabels,
	})

	c.goroutinesGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   c.options.Namespace,
		Subsystem:   c.options.Subsystem,
		Name:        "goroutines_total",
		Help:        "Current number of goroutines",
		ConstLabels: c.options.DefaultLabels,
	})

	c.heapAllocGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   c.options.Namespace,
		Subsystem:   c.options.Subsystem,
		Name:        "heap_alloc_bytes",
		Help:        "Current heap allocation in bytes",
		ConstLabels: c.options.DefaultLabels,
	})

	c.sysMemoryGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   c.options.Namespace,
		Subsystem:   c.options.Subsystem,
		Name:        "sys_memory_bytes",
		Help:        "Total system memory obtained from OS",
		ConstLabels: c.options.DefaultLabels,
	})

	c.gcCyclesGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   c.options.Namespace,
		Subsystem:   c.options.Subsystem,
		Name:        "gc_cycles_total",
		Help:        "Total number of GC cycles completed",
		ConstLabels: c.options.DefaultLabels,
	})

	c.gcPauseGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   c.options.Namespace,
		Subsystem:   c.options.Subsystem,
		Name:        "gc_pause_seconds",
		Help:        "Time spent in GC pause",
		ConstLabels: c.options.DefaultLabels,
	})

	// Register all system metrics
	collectors := []prometheus.Collector{
		c.cpuGauge,
		c.memoryGauge,
		c.goroutinesGauge,
		c.heapAllocGauge,
		c.sysMemoryGauge,
		c.gcCyclesGauge,
		c.gcPauseGauge,
	}

	for _, collector := range collectors {
		if err := c.registry.Register(collector); err != nil {
			return fmt.Errorf("failed to register system metric: %w", err)
		}
	}

	return nil
}

// collectSystemMetrics periodically collects system metrics
func (c *Collector) collectSystemMetrics(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.options.SystemMetricsInterval)
	defer ticker.Stop()

	// Collect initial metrics
	c.updateSystemMetrics()

	for {
		select {
		case <-ticker.C:
			c.updateSystemMetrics()
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		}
	}
}

// updateSystemMetrics updates system metrics
func (c *Collector) updateSystemMetrics() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Update metrics
	c.goroutinesGauge.Set(float64(runtime.NumGoroutine()))
	c.heapAllocGauge.Set(float64(memStats.HeapAlloc))
	c.sysMemoryGauge.Set(float64(memStats.Sys))
	c.gcCyclesGauge.Set(float64(memStats.NumGC))
	c.gcPauseGauge.Set(float64(memStats.PauseTotalNs) / 1e9) // Convert to seconds

	// Memory usage approximation
	c.memoryGauge.Set(float64(memStats.Alloc))

	// CPU usage would require platform-specific implementation
	// For now, we'll leave it at 0
	c.cpuGauge.Set(0)
}

// Helper methods for creating metrics

func (c *Collector) createCounter(name, help string, labels []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.counters[name]; exists {
		return
	}

	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   c.options.Namespace,
		Subsystem:   c.options.Subsystem,
		Name:        name,
		Help:        help,
		ConstLabels: c.options.DefaultLabels,
	}, labels)

	if err := c.registry.Register(counter); err != nil {
		c.logger.WithError(err).WithField("metric", name).Error("failed to register counter")
		return
	}

	c.counters[name] = counter
}

func (c *Collector) createGauge(name, help string, labels []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.gauges[name]; exists {
		return
	}

	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   c.options.Namespace,
		Subsystem:   c.options.Subsystem,
		Name:        name,
		Help:        help,
		ConstLabels: c.options.DefaultLabels,
	}, labels)

	if err := c.registry.Register(gauge); err != nil {
		c.logger.WithError(err).WithField("metric", name).Error("failed to register gauge")
		return
	}

	c.gauges[name] = gauge
}

func (c *Collector) createHistogram(name, help string, labels []string, buckets []float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.histograms[name]; exists {
		return
	}

	histogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   c.options.Namespace,
		Subsystem:   c.options.Subsystem,
		Name:        name,
		Help:        help,
		Buckets:     buckets,
		ConstLabels: c.options.DefaultLabels,
	}, labels)

	if err := c.registry.Register(histogram); err != nil {
		c.logger.WithError(err).WithField("metric", name).Error("failed to register histogram")
		return
	}

	c.histograms[name] = histogram
}

// mergeLabels merges provided labels with default labels
func (c *Collector) mergeLabels(labels map[string]string) prometheus.Labels {
	merged := make(prometheus.Labels)

	// Add default labels first
	for k, v := range c.options.DefaultLabels {
		merged[k] = v
	}

	// Override with provided labels
	for k, v := range labels {
		merged[k] = v
	}

	return merged
}

// getLabelNames extracts label names from a map
func (c *Collector) getLabelNames(labels map[string]string) []string {
	names := make([]string, 0, len(labels)+len(c.options.DefaultLabels))

	// Add default label names
	for k := range c.options.DefaultLabels {
		names = append(names, k)
	}

	// Add provided label names
	for k := range labels {
		// Skip if already in defaults
		if _, exists := c.options.DefaultLabels[k]; !exists {
			names = append(names, k)
		}
	}

	return names
}

// RegisterMetric pre-registers a metric definition
func (c *Collector) RegisterMetric(def *metrics.MetricDefinition) error {
	switch def.Type {
	case metrics.CounterType:
		c.createCounter(def.Name, def.Help, def.Labels)
	case metrics.GaugeType:
		c.createGauge(def.Name, def.Help, def.Labels)
	case metrics.HistogramType:
		buckets := def.Buckets
		if len(buckets) == 0 {
			buckets = prometheus.DefBuckets
		}
		c.createHistogram(def.Name, def.Help, def.Labels, buckets)
	case metrics.SummaryType:
		c.createSummary(def.Name, def.Help, def.Labels, def.Objectives)
	default:
		return fmt.Errorf("unknown metric type: %v", def.Type)
	}
	return nil
}

func (c *Collector) createSummary(name, help string, labels []string, objectives map[float64]float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.summaries[name]; exists {
		return
	}

	summary := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace:   c.options.Namespace,
		Subsystem:   c.options.Subsystem,
		Name:        name,
		Help:        help,
		Objectives:  objectives,
		ConstLabels: c.options.DefaultLabels,
	}, labels)

	if err := c.registry.Register(summary); err != nil {
		c.logger.WithError(err).WithField("metric", name).Error("failed to register summary")
		return
	}

	c.summaries[name] = summary
}
