package bot

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/sirupsen/logrus"
)

// SystemResourceMonitor monitors system resources and sends alerts when thresholds are exceeded
type SystemResourceMonitor struct {
	config           *common.BotConfig
	logger           *logrus.Logger
	cpuThreshold     float64
	memThreshold     float64
	diskThreshold    float64
	alertChan        chan *common.ResourceMetrics
	mu               sync.RWMutex
	isMonitoring     bool
	cancelFunc       context.CancelFunc
	wg               sync.WaitGroup
	lastMetrics      *common.ResourceMetrics
	alertsSent       int64
	metricsCollected int64
}

// NewResourceMonitor creates a new system resource monitor
func NewResourceMonitor(config *common.BotConfig, logger *logrus.Logger) *SystemResourceMonitor {
	return &SystemResourceMonitor{
		config:        config,
		logger:        logger,
		cpuThreshold:  80.0, // Default 80% CPU threshold
		memThreshold:  80.0, // Default 80% memory threshold
		diskThreshold: 90.0, // Default 90% disk threshold
		alertChan:     make(chan *common.ResourceMetrics, 100),
	}
}

// GetMetrics collects current system resource metrics
func (m *SystemResourceMonitor) GetMetrics() (*common.ResourceMetrics, error) {
	metrics := &common.ResourceMetrics{
		Timestamp: time.Now(),
	}

	// Get CPU usage
	cpuPercent, err := m.getCPUUsage()
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU usage: %w", err)
	}
	metrics.CPU = cpuPercent

	// Get memory usage
	memStats, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get memory stats: %w", err)
	}
	metrics.Memory = int64(memStats.Used)

	// Get disk usage for the work directory
	diskPath := m.config.Fuzzing.WorkDir
	if diskPath == "" {
		diskPath = "/"
	}

	diskStats, err := disk.Usage(diskPath)
	if err != nil {
		m.logger.WithError(err).WithField("path", diskPath).Warn("Failed to get disk stats, using root path")
		// Fallback to root path
		diskStats, err = disk.Usage("/")
		if err != nil {
			return nil, fmt.Errorf("failed to get disk stats: %w", err)
		}
	}
	metrics.Disk = int64(diskStats.Used)

	// Get process count
	processes, err := process.Processes()
	if err != nil {
		m.logger.WithError(err).Warn("Failed to get process count")
		// Non-fatal, set to 0
		metrics.ProcessCount = 0
	} else {
		metrics.ProcessCount = len(processes)
	}

	// Update last metrics
	m.mu.Lock()
	m.lastMetrics = metrics
	m.metricsCollected++
	m.mu.Unlock()

	return metrics, nil
}

// getCPUUsage gets the current CPU usage percentage
func (m *SystemResourceMonitor) getCPUUsage() (float64, error) {
	// Get CPU usage over a 1 second interval
	percentages, err := cpu.Percent(time.Second, false)
	if err != nil {
		return 0, err
	}

	if len(percentages) == 0 {
		return 0, fmt.Errorf("no CPU usage data available")
	}

	// Return overall CPU usage (first element is the average across all cores)
	return percentages[0], nil
}

// StartMonitoring starts the resource monitoring loop
func (m *SystemResourceMonitor) StartMonitoring(ctx context.Context, interval time.Duration) error {
	m.mu.Lock()
	if m.isMonitoring {
		m.mu.Unlock()
		return fmt.Errorf("monitoring is already running")
	}

	// Create a cancellable context
	monitorCtx, cancel := context.WithCancel(ctx)
	m.cancelFunc = cancel
	m.isMonitoring = true
	m.mu.Unlock()

	// Validate interval
	if interval <= 0 {
		interval = 10 * time.Second // Default to 10 seconds
	}

	m.logger.WithFields(logrus.Fields{
		"interval":       interval,
		"cpu_threshold":  m.cpuThreshold,
		"mem_threshold":  m.memThreshold,
		"disk_threshold": m.diskThreshold,
	}).Info("Starting resource monitoring")

	// Start monitoring goroutine
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.monitorLoop(monitorCtx, interval)
	}()

	return nil
}

// monitorLoop is the main monitoring loop
func (m *SystemResourceMonitor) monitorLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Resource monitoring stopped")
			return
		case <-ticker.C:
			// Collect metrics
			metrics, err := m.GetMetrics()
			if err != nil {
				m.logger.WithError(err).Error("Failed to collect resource metrics")
				continue
			}

			// Log current metrics
			m.logger.WithFields(logrus.Fields{
				"cpu_percent":   fmt.Sprintf("%.2f%%", metrics.CPU),
				"memory_mb":     metrics.Memory / (1024 * 1024),
				"disk_mb":       metrics.Disk / (1024 * 1024),
				"process_count": metrics.ProcessCount,
			}).Debug("Resource metrics collected")

			// Check thresholds
			m.checkThresholds(metrics)
		}
	}
}

// SetAlertThresholds configures the alert thresholds
func (m *SystemResourceMonitor) SetAlertThresholds(cpu, memory, disk float64) error {
	// Validate thresholds
	if cpu < 0 || cpu > 100 {
		return fmt.Errorf("CPU threshold must be between 0 and 100")
	}
	if memory < 0 || memory > 100 {
		return fmt.Errorf("memory threshold must be between 0 and 100")
	}
	if disk < 0 || disk > 100 {
		return fmt.Errorf("disk threshold must be between 0 and 100")
	}

	m.mu.Lock()
	m.cpuThreshold = cpu
	m.memThreshold = memory
	m.diskThreshold = disk
	m.mu.Unlock()

	m.logger.WithFields(logrus.Fields{
		"cpu_threshold":  cpu,
		"mem_threshold":  memory,
		"disk_threshold": disk,
	}).Info("Alert thresholds updated")

	return nil
}

// checkThresholds checks if any resource thresholds are exceeded
func (m *SystemResourceMonitor) checkThresholds(metrics *common.ResourceMetrics) {
	alerts := []string{}

	// Check CPU threshold
	if metrics.CPU > m.cpuThreshold {
		alerts = append(alerts, fmt.Sprintf("CPU usage %.2f%% exceeds threshold %.2f%%",
			metrics.CPU, m.cpuThreshold))
	}

	// Check memory threshold
	memStats, err := mem.VirtualMemory()
	if err == nil {
		memPercent := memStats.UsedPercent
		if memPercent > m.memThreshold {
			alerts = append(alerts, fmt.Sprintf("Memory usage %.2f%% exceeds threshold %.2f%%",
				memPercent, m.memThreshold))
		}
	}

	// Check disk threshold
	diskPath := m.config.Fuzzing.WorkDir
	if diskPath == "" {
		diskPath = "/"
	}

	diskStats, err := disk.Usage(diskPath)
	if err == nil {
		diskPercent := diskStats.UsedPercent
		if diskPercent > m.diskThreshold {
			alerts = append(alerts, fmt.Sprintf("Disk usage %.2f%% exceeds threshold %.2f%%",
				diskPercent, m.diskThreshold))
		}
	}

	// Send alerts if any thresholds exceeded
	if len(alerts) > 0 {
		m.logger.WithFields(logrus.Fields{
			"alerts":        alerts,
			"cpu_percent":   fmt.Sprintf("%.2f%%", metrics.CPU),
			"memory_mb":     metrics.Memory / (1024 * 1024),
			"disk_mb":       metrics.Disk / (1024 * 1024),
			"process_count": metrics.ProcessCount,
		}).Warn("Resource thresholds exceeded")

		// Try to send alert through channel (non-blocking)
		select {
		case m.alertChan <- metrics:
			m.mu.Lock()
			m.alertsSent++
			m.mu.Unlock()
		default:
			m.logger.Warn("Alert channel full, dropping resource alert")
		}
	}
}

// Stop stops the resource monitoring
func (m *SystemResourceMonitor) Stop() error {
	m.mu.Lock()
	if !m.isMonitoring {
		m.mu.Unlock()
		return fmt.Errorf("monitoring is not running")
	}

	// Cancel the monitoring context
	if m.cancelFunc != nil {
		m.cancelFunc()
	}
	m.isMonitoring = false
	m.mu.Unlock()

	// Wait for monitoring goroutine to finish
	m.wg.Wait()

	// Close alert channel
	close(m.alertChan)

	m.logger.Info("Resource monitoring stopped")
	return nil
}

// GetAlertChannel returns the alert channel for receiving threshold alerts
func (m *SystemResourceMonitor) GetAlertChannel() <-chan *common.ResourceMetrics {
	return m.alertChan
}

// GetLastMetrics returns the most recently collected metrics
func (m *SystemResourceMonitor) GetLastMetrics() *common.ResourceMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastMetrics
}

// GetStats returns monitoring statistics
func (m *SystemResourceMonitor) GetStats() (metricsCollected, alertsSent int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.metricsCollected, m.alertsSent
}

// IsMonitoring returns whether monitoring is currently active
func (m *SystemResourceMonitor) IsMonitoring() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isMonitoring
}

// GetThresholds returns the current alert thresholds
func (m *SystemResourceMonitor) GetThresholds() (cpu, memory, disk float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cpuThreshold, m.memThreshold, m.diskThreshold
}

// GetSystemInfo returns static system information
func (m *SystemResourceMonitor) GetSystemInfo() map[string]interface{} {
	info := make(map[string]interface{})

	// Runtime info
	info["go_version"] = runtime.Version()
	info["num_cpu"] = runtime.NumCPU()
	info["os"] = runtime.GOOS
	info["arch"] = runtime.GOARCH

	// Memory info
	if memStats, err := mem.VirtualMemory(); err == nil {
		info["total_memory_mb"] = memStats.Total / (1024 * 1024)
		info["available_memory_mb"] = memStats.Available / (1024 * 1024)
	}

	// Disk info
	diskPath := m.config.Fuzzing.WorkDir
	if diskPath == "" {
		diskPath = "/"
	}

	if diskStats, err := disk.Usage(diskPath); err == nil {
		info["total_disk_mb"] = diskStats.Total / (1024 * 1024)
		info["available_disk_mb"] = diskStats.Free / (1024 * 1024)
		info["disk_path"] = diskPath
	}

	// CPU info
	if cpuInfo, err := cpu.Info(); err == nil && len(cpuInfo) > 0 {
		info["cpu_model"] = cpuInfo[0].ModelName
		info["cpu_cores"] = cpuInfo[0].Cores
	}

	return info
}
