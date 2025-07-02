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
	"github.com/ethpandaops/pandafuzz/pkg/fuzzer"
	"github.com/sirupsen/logrus"
)

// Example demonstrates how to use the ResultCollector
func main() {
	// Configure logger
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.JSONFormatter{})

	// Bot configuration
	config := &common.BotConfig{
		ID:        "example-bot-1",
		Name:      "Example Bot",
		MasterURL: "http://localhost:8080",
		Timeouts: common.BotTimeoutConfig{
			HeartbeatInterval:   30 * time.Second,
			ResultReporting:     30 * time.Second,
			MasterCommunication: 30 * time.Second,
		},
		Retry: common.BotRetryConfig{
			Communication: common.RetryPolicy{
				MaxRetries:   3,
				InitialDelay: 1 * time.Second,
				MaxDelay:     30 * time.Second,
				Multiplier:   2,
			},
		},
	}

	// Create result collector
	collector, err := bot.NewResultCollector(config, config.MasterURL, logger)
	if err != nil {
		log.Fatalf("Failed to create result collector: %v", err)
	}

	// Configure batch settings
	collector.SetBatchSize(50)                  // Batch up to 50 results
	collector.SetFlushInterval(5 * time.Second) // Flush every 5 seconds

	// Start the collector
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := collector.Start(ctx); err != nil {
		log.Fatalf("Failed to start collector: %v", err)
	}

	// Simulate fuzzer events
	jobID := "example-job-123"
	botID := config.ID

	// Example 1: Report a crash
	crashEvent := common.FuzzerEvent{
		Type:      common.FuzzerEventCrashFound,
		Timestamp: time.Now(),
		JobID:     jobID,
		Data: map[string]interface{}{
			"crash": &common.CrashResult{
				ID:        fmt.Sprintf("crash-%d", time.Now().Unix()),
				JobID:     jobID,
				BotID:     botID,
				Hash:      "deadbeef123456",
				FilePath:  "crashes/crash_001.bin",
				Type:      "segfault",
				Signal:    11,
				ExitCode:  -11,
				Timestamp: time.Now(),
				Size:      1024,
				IsUnique:  true,
				Output:    "Segmentation fault at 0xdeadbeef",
			},
		},
	}

	if err := collector.HandleEvent(crashEvent); err != nil {
		logger.WithError(err).Error("Failed to handle crash event")
	}

	// Example 2: Report coverage update
	coverageEvent := common.FuzzerEvent{
		Type:      common.FuzzerEventCoverage,
		Timestamp: time.Now(),
		JobID:     jobID,
		Data: map[string]interface{}{
			"coverage": &common.CoverageResult{
				ID:        fmt.Sprintf("cov-%d", time.Now().Unix()),
				JobID:     jobID,
				BotID:     botID,
				Edges:     5000,
				NewEdges:  150,
				Timestamp: time.Now(),
				ExecCount: 100000,
			},
		},
	}

	if err := collector.HandleEvent(coverageEvent); err != nil {
		logger.WithError(err).Error("Failed to handle coverage event")
	}

	// Example 3: Report corpus update
	corpusEvent := common.FuzzerEvent{
		Type:      common.FuzzerEventCorpusUpdate,
		Timestamp: time.Now(),
		JobID:     jobID,
		Data: map[string]interface{}{
			"corpus": &common.CorpusUpdate{
				ID:        fmt.Sprintf("corpus-%d", time.Now().Unix()),
				JobID:     jobID,
				BotID:     botID,
				Files:     []string{"input_001.bin", "input_002.bin", "input_003.bin"},
				Timestamp: time.Now(),
				TotalSize: 3072,
			},
		},
	}

	if err := collector.HandleEvent(corpusEvent); err != nil {
		logger.WithError(err).Error("Failed to handle corpus event")
	}

	// Example 4: Report fuzzer statistics
	statsEvent := common.FuzzerEvent{
		Type:      common.FuzzerEventStats,
		Timestamp: time.Now(),
		JobID:     jobID,
		Data: map[string]interface{}{
			"stats": &fuzzer.FuzzerStats{
				StartTime:       time.Now().Add(-1 * time.Hour),
				ElapsedTime:     1 * time.Hour,
				Executions:      1000000,
				ExecPerSecond:   277.77,
				TotalEdges:      10000,
				CoveredEdges:    7500,
				CoveragePercent: 75.0,
				UniqueCrashes:   3,
				TotalCrashes:    5,
				CrashRate:       0.0005,
				CorpusSize:      250,
				NewPaths:        50,
				PathsTotal:      300,
				CPUUsage:        85.5,
				MemoryUsage:     2048 * 1024 * 1024, // 2GB
				DiskUsage:       512 * 1024 * 1024,  // 512MB
			},
		},
	}

	if err := collector.HandleEvent(statsEvent); err != nil {
		logger.WithError(err).Error("Failed to handle stats event")
	}

	// Get collector statistics
	stats := collector.GetStats()
	logger.WithField("stats", stats).Info("Collector statistics")

	// Manually trigger a batch send
	if err := collector.batchAndSend(ctx); err != nil {
		logger.WithError(err).Error("Failed to manually flush results")
	}

	// Simulate running for a while
	logger.Info("Running for 10 seconds...")
	time.Sleep(10 * time.Second)

	// Stop the collector (will flush remaining results)
	logger.Info("Stopping collector...")
	if err := collector.Stop(); err != nil {
		logger.WithError(err).Error("Failed to stop collector")
	}

	logger.Info("Example completed")
}

// Integration example with fuzzer event handler
type FuzzerEventHandler struct {
	collector *bot.ResultCollector
	jobID     string
	botID     string
}

func (h *FuzzerEventHandler) OnCrash(f fuzzer.Fuzzer, crash *common.CrashResult) {
	event := common.FuzzerEvent{
		Type:      common.FuzzerEventCrashFound,
		Timestamp: time.Now(),
		JobID:     h.jobID,
		Data: map[string]interface{}{
			"crash": crash,
		},
	}
	h.collector.HandleEvent(event)
}

func (h *FuzzerEventHandler) OnStats(f fuzzer.Fuzzer, stats fuzzer.FuzzerStats) {
	event := common.FuzzerEvent{
		Type:      common.FuzzerEventStats,
		Timestamp: time.Now(),
		JobID:     h.jobID,
		Data: map[string]interface{}{
			"stats": &stats,
		},
	}
	h.collector.HandleEvent(event)
}

// Implement other event handler methods...

