package fuzzer

import (
	"context"
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
	interval       time.Duration
	done           chan struct{}
	wg             sync.WaitGroup

	// Performance tracking
	execCount     int64
	totalExecTime time.Duration
	execTimes     []time.Duration
	startTime     time.Time
	memStats      runtime.MemStats
}

// NewDefaultMetricsCollector creates a new default metrics collector
func NewDefaultMetricsCollector(log logrus.FieldLogger, interval time.Duration) common.MetricsCollector {
	return &defaultMetricsCollector{
		log:      log.WithField("component", "metrics_collector"),
		interval: interval,
		done:     make(chan struct{}),
		currentMetrics: &common.EnhancedMetrics{
			Timestamp:      time.Now(),
			FuzzerSpecific: make(map[string]interface{}),
		},
		execTimes: make([]time.Duration, 0, 1000),
		startTime: time.Now(),
	}
}

// Start begins metrics collection
func (mc *defaultMetricsCollector) Start(ctx context.Context) error {
	mc.wg.Add(1)
	go func() {
		defer mc.wg.Done()
		ticker := time.NewTicker(mc.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-mc.done:
				return
			case <-ticker.C:
				if _, err := mc.Collect(); err != nil {
					mc.log.WithError(err).Error("Failed to collect metrics")
				}
			}
		}
	}()

	mc.log.Info("Started metrics collection")
	return nil
}

// Stop halts metrics collection
func (mc *defaultMetricsCollector) Stop() error {
	close(mc.done)
	mc.wg.Wait()
	mc.log.Info("Stopped metrics collection")
	return nil
}

// Collect gathers current metrics
func (mc *defaultMetricsCollector) Collect() (*common.EnhancedMetrics, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

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

// RecordExecution records a single execution duration
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
