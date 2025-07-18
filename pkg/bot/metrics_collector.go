package bot

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// defaultMetricsCollector provides a base implementation for metrics collection
type defaultMetricsCollector struct {
	log            logrus.FieldLogger
	currentMetrics *common.EnhancedMetrics
	mu             sync.RWMutex
	done           chan struct{}
	wg             sync.WaitGroup

	// Collection interval
	interval time.Duration

	// Performance tracking
	execCount     int64
	totalExecTime time.Duration
	execTimes     []time.Duration

	// Resource tracking
	startTime time.Time
	memStats  runtime.MemStats
}

// NewDefaultMetricsCollector creates a new default metrics collector
func NewDefaultMetricsCollector(log logrus.FieldLogger, interval time.Duration) common.MetricsCollector {
	return &defaultMetricsCollector{
		log:      log.WithField("component", "metrics_collector"),
		interval: interval,
		done:     make(chan struct{}),
		currentMetrics: &common.EnhancedMetrics{
			FuzzerSpecific: make(map[string]interface{}),
		},
		execTimes: make([]time.Duration, 0, 1000),
		startTime: time.Now(),
	}
}

// Start begins the metrics collection process
func (mc *defaultMetricsCollector) Start(ctx context.Context) error {
	mc.log.Info("Starting metrics collector")

	mc.wg.Add(1)
	go func() {
		defer mc.wg.Done()
		ticker := time.NewTicker(mc.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				mc.log.Info("Metrics collector context cancelled")
				return
			case <-mc.done:
				mc.log.Info("Metrics collector stopped")
				return
			case <-ticker.C:
				if _, err := mc.Collect(); err != nil {
					mc.log.WithError(err).Error("Failed to collect metrics")
				}
			}
		}
	}()

	return nil
}

// Stop halts the metrics collection
func (mc *defaultMetricsCollector) Stop() error {
	mc.log.Info("Stopping metrics collector")
	close(mc.done)
	mc.wg.Wait()
	return nil
}

// Collect gathers current metrics
func (mc *defaultMetricsCollector) Collect() (*common.EnhancedMetrics, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Update timestamp
	mc.currentMetrics.Timestamp = time.Now()

	// Collect resource metrics
	mc.collectResourceMetrics()

	// Calculate performance metrics
	mc.calculatePerformanceMetrics()

	return mc.currentMetrics, nil
}

// GetMetrics returns the current metrics snapshot
func (mc *defaultMetricsCollector) GetMetrics() *common.EnhancedMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// Return a copy to avoid concurrent modification
	metrics := *mc.currentMetrics
	return &metrics
}

// collectResourceMetrics gathers resource utilization data
func (mc *defaultMetricsCollector) collectResourceMetrics() {
	runtime.ReadMemStats(&mc.memStats)

	mc.currentMetrics.ResourceUsage = common.ResourceUsageMetrics{
		MemoryUsageMB: float64(mc.memStats.Alloc) / 1024 / 1024,
		Threads:       runtime.NumGoroutine(),
	}

	// Calculate resource efficiency (execs per MB of memory)
	if mc.currentMetrics.ResourceUsage.MemoryUsageMB > 0 {
		mc.currentMetrics.ResourceUsage.ResourceEfficiency =
			mc.currentMetrics.Performance.ExecutionsPerSecond / mc.currentMetrics.ResourceUsage.MemoryUsageMB
	}

	// Add to history
	snapshot := common.ResourceSnapshot{
		Timestamp:   time.Now(),
		MemoryUsage: mc.currentMetrics.ResourceUsage.MemoryUsageMB,
	}

	mc.currentMetrics.ResourceUsage.ResourceHistory = append(
		mc.currentMetrics.ResourceUsage.ResourceHistory,
		snapshot,
	)

	// Keep only last 100 snapshots
	if len(mc.currentMetrics.ResourceUsage.ResourceHistory) > 100 {
		mc.currentMetrics.ResourceUsage.ResourceHistory =
			mc.currentMetrics.ResourceUsage.ResourceHistory[len(mc.currentMetrics.ResourceUsage.ResourceHistory)-100:]
	}
}

// calculatePerformanceMetrics computes performance statistics
func (mc *defaultMetricsCollector) calculatePerformanceMetrics() {
	if mc.execCount == 0 {
		return
	}

	// Calculate average execution time
	avgExecTime := mc.totalExecTime / time.Duration(mc.execCount)
	mc.currentMetrics.Performance.AverageExecTime = avgExecTime

	// Calculate executions per second
	elapsed := time.Since(mc.startTime).Seconds()
	if elapsed > 0 {
		mc.currentMetrics.Performance.ExecutionsPerSecond = float64(mc.execCount) / elapsed
	}

	// Calculate percentiles if we have execution times
	if len(mc.execTimes) > 0 {
		mc.currentMetrics.Performance.MedianExecTime = mc.calculatePercentile(mc.execTimes, 50)
		mc.currentMetrics.Performance.P95ExecTime = mc.calculatePercentile(mc.execTimes, 95)
		mc.currentMetrics.Performance.P99ExecTime = mc.calculatePercentile(mc.execTimes, 99)
	}

	// Add to history
	snapshot := common.PerformanceSnapshot{
		Timestamp:           time.Now(),
		ExecutionsPerSecond: mc.currentMetrics.Performance.ExecutionsPerSecond,
		MemoryUsage:         mc.currentMetrics.ResourceUsage.MemoryUsageMB,
	}

	mc.currentMetrics.Performance.PerformanceHistory = append(
		mc.currentMetrics.Performance.PerformanceHistory,
		snapshot,
	)

	// Keep only last 100 snapshots
	if len(mc.currentMetrics.Performance.PerformanceHistory) > 100 {
		mc.currentMetrics.Performance.PerformanceHistory =
			mc.currentMetrics.Performance.PerformanceHistory[len(mc.currentMetrics.Performance.PerformanceHistory)-100:]
	}
}

// calculatePercentile calculates the nth percentile of durations
func (mc *defaultMetricsCollector) calculatePercentile(times []time.Duration, percentile float64) time.Duration {
	if len(times) == 0 {
		return 0
	}

	index := int(float64(len(times)) * percentile / 100)
	if index >= len(times) {
		index = len(times) - 1
	}

	return times[index]
}

// UpdateCoverageMetrics updates coverage-related metrics
func (mc *defaultMetricsCollector) UpdateCoverageMetrics(coverage common.CoverageMetrics) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.currentMetrics.Coverage = coverage
}

// UpdatePerformanceMetrics updates performance-related metrics
func (mc *defaultMetricsCollector) UpdatePerformanceMetrics(perf common.PerformanceMetrics) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.currentMetrics.Performance = perf
}

// UpdateFuzzerSpecificMetrics updates fuzzer-specific metrics
func (mc *defaultMetricsCollector) UpdateFuzzerSpecificMetrics(key string, value interface{}) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.currentMetrics.FuzzerSpecific[key] = value
}

// RecordExecution records a single execution for performance tracking
func (mc *defaultMetricsCollector) RecordExecution(duration time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.execCount++
	mc.totalExecTime += duration
	mc.execTimes = append(mc.execTimes, duration)

	// Keep only last 1000 execution times for percentile calculation
	if len(mc.execTimes) > 1000 {
		mc.execTimes = mc.execTimes[len(mc.execTimes)-1000:]
	}
}

// MetricsAggregator aggregates metrics from multiple collectors
type MetricsAggregator struct {
	log        logrus.FieldLogger
	collectors map[string]common.MetricsCollector
	mu         sync.RWMutex
}

// NewMetricsAggregator creates a new metrics aggregator
func NewMetricsAggregator(log logrus.FieldLogger) *MetricsAggregator {
	return &MetricsAggregator{
		log:        log.WithField("component", "metrics_aggregator"),
		collectors: make(map[string]common.MetricsCollector),
	}
}

// RegisterCollector registers a metrics collector
func (ma *MetricsAggregator) RegisterCollector(name string, collector common.MetricsCollector) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	ma.collectors[name] = collector
	ma.log.WithField("collector", name).Info("Registered metrics collector")
}

// GetAggregatedMetrics returns aggregated metrics from all collectors
func (ma *MetricsAggregator) GetAggregatedMetrics() map[string]*common.EnhancedMetrics {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	metrics := make(map[string]*common.EnhancedMetrics)
	for name, collector := range ma.collectors {
		metrics[name] = collector.GetMetrics()
	}

	return metrics
}

// GetCollectorMetrics returns metrics from a specific collector
func (ma *MetricsAggregator) GetCollectorMetrics(name string) (*common.EnhancedMetrics, error) {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	collector, exists := ma.collectors[name]
	if !exists {
		return nil, fmt.Errorf("collector %s not found", name)
	}

	return collector.GetMetrics(), nil
}
