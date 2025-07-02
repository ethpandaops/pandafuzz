//go:build ignore
// +build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/bot"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// Example of how to use the SystemResourceMonitor
func main() {
	// Create logger
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// Create bot config
	config := &common.BotConfig{
		ID:        "example-bot",
		MasterURL: "http://localhost:8080",
		Fuzzing: common.FuzzingConfig{
			WorkDir: "/tmp/pandafuzz",
		},
	}

	// Create resource monitor
	monitor := bot.NewResourceMonitor(config, logger)

	// Set custom thresholds
	err := monitor.SetAlertThresholds(75.0, 70.0, 85.0)
	if err != nil {
		log.Fatal(err)
	}

	// Get system info
	info := monitor.GetSystemInfo()
	fmt.Println("System Information:")
	for key, value := range info {
		fmt.Printf("  %s: %v\n", key, value)
	}

	// Start monitoring
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = monitor.StartMonitoring(ctx, 5*time.Second)
	if err != nil {
		log.Fatal(err)
	}

	// Handle alerts in a separate goroutine
	go func() {
		alertChan := monitor.GetAlertChannel()
		for alert := range alertChan {
			fmt.Printf("\n🚨 ALERT: Resource threshold exceeded!\n")
			fmt.Printf("  CPU: %.2f%%\n", alert.CPU)
			fmt.Printf("  Memory: %d MB\n", alert.Memory/(1024*1024))
			fmt.Printf("  Disk: %d MB\n", alert.Disk/(1024*1024))
			fmt.Printf("  Processes: %d\n", alert.ProcessCount)
			fmt.Printf("  Time: %s\n\n", alert.Timestamp.Format(time.RFC3339))
		}
	}()

	// Run for 30 seconds
	fmt.Println("\nMonitoring resources for 30 seconds...")
	time.Sleep(30 * time.Second)

	// Get final stats
	collected, alerts := monitor.GetStats()
	fmt.Printf("\nMonitoring Statistics:\n")
	fmt.Printf("  Metrics collected: %d\n", collected)
	fmt.Printf("  Alerts sent: %d\n", alerts)

	// Get last metrics
	if lastMetrics := monitor.GetLastMetrics(); lastMetrics != nil {
		fmt.Printf("\nLast Metrics:\n")
		fmt.Printf("  CPU: %.2f%%\n", lastMetrics.CPU)
		fmt.Printf("  Memory: %d MB\n", lastMetrics.Memory/(1024*1024))
		fmt.Printf("  Disk: %d MB\n", lastMetrics.Disk/(1024*1024))
		fmt.Printf("  Processes: %d\n", lastMetrics.ProcessCount)
	}

	// Stop monitoring
	err = monitor.Stop()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\nMonitoring stopped.")
}
