package bot

import (
	"context"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewResourceMonitor(t *testing.T) {
	config := &common.BotConfig{
		ID:        "test-bot",
		MasterURL: "http://localhost:8080",
		Fuzzing: common.FuzzingConfig{
			WorkDir: "/tmp/test",
		},
	}
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	monitor := NewResourceMonitor(config, logger)
	assert.NotNil(t, monitor)
	assert.Equal(t, config, monitor.config)
	assert.NotNil(t, monitor.logger)
	assert.Equal(t, 80.0, monitor.cpuThreshold)
	assert.Equal(t, 80.0, monitor.memThreshold)
	assert.Equal(t, 90.0, monitor.diskThreshold)
	assert.NotNil(t, monitor.alertChan)
}

func TestGetMetrics(t *testing.T) {
	config := &common.BotConfig{
		ID:        "test-bot",
		MasterURL: "http://localhost:8080",
		Fuzzing: common.FuzzingConfig{
			WorkDir: "/tmp",
		},
	}
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	monitor := NewResourceMonitor(config, logger)

	metrics, err := monitor.GetMetrics()
	require.NoError(t, err)
	assert.NotNil(t, metrics)

	// Verify metrics are populated
	assert.True(t, metrics.CPU >= 0 && metrics.CPU <= 100, "CPU usage should be between 0-100%")
	assert.True(t, metrics.Memory > 0, "Memory usage should be positive")
	assert.True(t, metrics.Disk > 0, "Disk usage should be positive")
	assert.True(t, metrics.ProcessCount >= 0, "Process count should be non-negative")
	assert.False(t, metrics.Timestamp.IsZero(), "Timestamp should be set")
}

func TestSetAlertThresholds(t *testing.T) {
	config := &common.BotConfig{
		ID:        "test-bot",
		MasterURL: "http://localhost:8080",
		Fuzzing: common.FuzzingConfig{
			WorkDir: "/tmp",
		},
	}
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	monitor := NewResourceMonitor(config, logger)

	// Test valid thresholds
	err := monitor.SetAlertThresholds(70.0, 75.0, 85.0)
	assert.NoError(t, err)

	cpu, mem, disk := monitor.GetThresholds()
	assert.Equal(t, 70.0, cpu)
	assert.Equal(t, 75.0, mem)
	assert.Equal(t, 85.0, disk)

	// Test invalid CPU threshold
	err = monitor.SetAlertThresholds(101.0, 75.0, 85.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CPU threshold must be between 0 and 100")

	// Test invalid memory threshold
	err = monitor.SetAlertThresholds(70.0, -1.0, 85.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "memory threshold must be between 0 and 100")

	// Test invalid disk threshold
	err = monitor.SetAlertThresholds(70.0, 75.0, 150.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "disk threshold must be between 0 and 100")
}

func TestStartStopMonitoring(t *testing.T) {
	config := &common.BotConfig{
		ID:        "test-bot",
		MasterURL: "http://localhost:8080",
		Fuzzing: common.FuzzingConfig{
			WorkDir: "/tmp",
		},
	}
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	monitor := NewResourceMonitor(config, logger)

	// Start monitoring
	ctx := context.Background()
	err := monitor.StartMonitoring(ctx, 100*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, monitor.IsMonitoring())

	// Try to start again (should fail)
	err = monitor.StartMonitoring(ctx, 100*time.Millisecond)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "monitoring is already running")

	// Wait for a few monitoring cycles (CPU collection takes 1 second)
	time.Sleep(1500 * time.Millisecond)

	// Check that metrics were collected
	collected, _ := monitor.GetStats()
	assert.True(t, collected > 0, "Should have collected some metrics")

	// Stop monitoring
	err = monitor.Stop()
	require.NoError(t, err)
	assert.False(t, monitor.IsMonitoring())

	// Try to stop again (should fail)
	err = monitor.Stop()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "monitoring is not running")
}

func TestAlertChannel(t *testing.T) {
	config := &common.BotConfig{
		ID:        "test-bot",
		MasterURL: "http://localhost:8080",
		Fuzzing: common.FuzzingConfig{
			WorkDir: "/tmp",
		},
	}
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	monitor := NewResourceMonitor(config, logger)

	// Set very low thresholds to trigger alerts
	err := monitor.SetAlertThresholds(0.1, 0.1, 0.1)
	require.NoError(t, err)

	// Get alert channel
	alertChan := monitor.GetAlertChannel()
	assert.NotNil(t, alertChan)

	// Start monitoring with short interval
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = monitor.StartMonitoring(ctx, 100*time.Millisecond)
	require.NoError(t, err)

	// Wait for an alert (should trigger quickly with low thresholds)
	select {
	case alert := <-alertChan:
		assert.NotNil(t, alert)
		assert.True(t, alert.CPU >= 0)
		assert.True(t, alert.Memory > 0)
		assert.True(t, alert.Disk > 0)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for alert")
	}

	// Check alert stats
	_, alertsSent := monitor.GetStats()
	assert.True(t, alertsSent > 0, "Should have sent at least one alert")

	// Stop monitoring
	err = monitor.Stop()
	require.NoError(t, err)
}

func TestGetSystemInfo(t *testing.T) {
	config := &common.BotConfig{
		ID:        "test-bot",
		MasterURL: "http://localhost:8080",
		Fuzzing: common.FuzzingConfig{
			WorkDir: "/tmp",
		},
	}
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	monitor := NewResourceMonitor(config, logger)

	info := monitor.GetSystemInfo()
	assert.NotNil(t, info)

	// Check that basic system info is present
	assert.Contains(t, info, "go_version")
	assert.Contains(t, info, "num_cpu")
	assert.Contains(t, info, "os")
	assert.Contains(t, info, "arch")

	// Verify some values
	assert.True(t, info["num_cpu"].(int) > 0)
	assert.NotEmpty(t, info["os"].(string))
	assert.NotEmpty(t, info["arch"].(string))
}
