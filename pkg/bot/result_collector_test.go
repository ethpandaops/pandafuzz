package bot

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/fuzzer"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResultCollector_NewResultCollector(t *testing.T) {
	config := &common.BotConfig{
		ID:        "test-bot",
		MasterURL: "http://localhost:8080",
		Timeouts: common.BotTimeoutConfig{
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

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	rc, err := NewResultCollector(config, config.MasterURL, logger)
	require.NoError(t, err)
	require.NotNil(t, rc)

	assert.Equal(t, config, rc.config)
	assert.Equal(t, config.MasterURL, rc.masterURL)
	assert.Equal(t, defaultBatchSize, rc.batchSize)
	assert.NotNil(t, rc.eventChan)
	assert.NotNil(t, rc.flushTicker)
	assert.NotNil(t, rc.retryClient)
}

func TestResultCollector_StartStop(t *testing.T) {
	rc := createTestResultCollector(t)

	// Test starting
	ctx := context.Background()
	err := rc.Start(ctx)
	require.NoError(t, err)

	// Test double start
	err = rc.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")

	// Test stopping
	err = rc.Stop()
	require.NoError(t, err)

	// Test double stop
	err = rc.Stop()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestResultCollector_HandleEvent(t *testing.T) {
	rc := createTestResultCollector(t)
	ctx := context.Background()

	err := rc.Start(ctx)
	require.NoError(t, err)
	defer rc.Stop()

	// Test crash event
	crashEvent := common.FuzzerEvent{
		Type:      common.FuzzerEventCrashFound,
		Timestamp: time.Now(),
		JobID:     "test-job",
		Data: map[string]interface{}{
			"crash": &common.CrashResult{
				ID:        "crash-1",
				JobID:     "test-job",
				BotID:     "test-bot",
				Hash:      "abc123",
				Type:      "segfault",
				Timestamp: time.Now(),
			},
		},
	}

	err = rc.HandleEvent(crashEvent)
	assert.NoError(t, err)

	// Give time for event processing
	time.Sleep(100 * time.Millisecond)

	// Check batch contains the crash
	rc.mu.Lock()
	assert.Len(t, rc.crashBatch, 1)
	rc.mu.Unlock()
}

func TestResultCollector_ProcessEvents(t *testing.T) {
	rc := createTestResultCollector(t)

	// Test crash event processing
	t.Run("CrashEvent", func(t *testing.T) {
		event := common.FuzzerEvent{
			Type:      common.FuzzerEventCrashFound,
			Timestamp: time.Now(),
			JobID:     "test-job",
			Data: map[string]interface{}{
				"crash": &common.CrashResult{
					ID:    "crash-1",
					JobID: "test-job",
					BotID: "test-bot",
				},
			},
		}

		rc.processCrashEvent(event)
		assert.Len(t, rc.crashBatch, 1)
	})

	// Test coverage event processing
	t.Run("CoverageEvent", func(t *testing.T) {
		event := common.FuzzerEvent{
			Type:      common.FuzzerEventCoverage,
			Timestamp: time.Now(),
			JobID:     "test-job",
			Data: map[string]interface{}{
				"coverage": &common.CoverageResult{
					ID:       "cov-1",
					JobID:    "test-job",
					BotID:    "test-bot",
					Edges:    1000,
					NewEdges: 50,
				},
			},
		}

		rc.processCoverageEvent(event)
		assert.Len(t, rc.coverageBatch, 1)
	})

	// Test corpus event processing
	t.Run("CorpusEvent", func(t *testing.T) {
		event := common.FuzzerEvent{
			Type:      common.FuzzerEventCorpusUpdate,
			Timestamp: time.Now(),
			JobID:     "test-job",
			Data: map[string]interface{}{
				"corpus": &common.CorpusUpdate{
					ID:        "corpus-1",
					JobID:     "test-job",
					BotID:     "test-bot",
					Files:     []string{"file1", "file2"},
					TotalSize: 1024,
				},
			},
		}

		rc.processCorpusEvent(event)
		assert.Len(t, rc.corpusBatch, 1)
	})

	// Test stats event processing
	t.Run("StatsEvent", func(t *testing.T) {
		event := common.FuzzerEvent{
			Type:      common.FuzzerEventStats,
			Timestamp: time.Now(),
			JobID:     "test-job",
			Data: map[string]interface{}{
				"stats": &fuzzer.FuzzerStats{
					Executions:    10000,
					ExecPerSecond: 500,
					TotalEdges:    2000,
					CoveredEdges:  1500,
				},
			},
		}

		rc.processStatsEvent(event)
		assert.Len(t, rc.statsBatch, 1)
	})
}

func TestResultCollector_BatchSizeLimit(t *testing.T) {
	rc := createTestResultCollector(t)
	rc.SetBatchSize(2) // Small batch size for testing

	// Disable actual network calls by setting a nil retry client
	rc.retryClient = nil

	// Add crashes to trigger batch flush
	for i := 0; i < 3; i++ {
		event := common.FuzzerEvent{
			Type:      common.FuzzerEventCrashFound,
			Timestamp: time.Now(),
			JobID:     "test-job",
			Data: map[string]interface{}{
				"crash": &common.CrashResult{
					ID:    fmt.Sprintf("crash-%d", i),
					JobID: "test-job",
					BotID: "test-bot",
				},
			},
		}
		rc.processCrashEvent(event)

		// If we hit batch size, the batch should have been flushed
		if i == 1 {
			// After processing 2 items, batch should be empty due to flush
			rc.mu.Lock()
			batchLen := len(rc.crashBatch)
			rc.mu.Unlock()
			assert.Equal(t, 0, batchLen, "Batch should be empty after flush")
		}
	}

	// Should have 1 crash remaining after batch flush
	rc.mu.Lock()
	assert.Len(t, rc.crashBatch, 1)
	rc.mu.Unlock()
}

func TestResultCollector_GetStats(t *testing.T) {
	rc := createTestResultCollector(t)

	// Add some data
	rc.crashBatch = append(rc.crashBatch, &common.CrashResult{})
	rc.coverageBatch = append(rc.coverageBatch, &common.CoverageResult{})

	stats := rc.GetStats()

	assert.Equal(t, 1, stats["crash_batch_size"])
	assert.Equal(t, 1, stats["coverage_batch_size"])
	assert.Equal(t, 0, stats["corpus_batch_size"])
	assert.Equal(t, 0, stats["stats_batch_size"])
	assert.Equal(t, defaultBatchSize, stats["batch_size_limit"])
}

func TestResultCollector_SetBatchSize(t *testing.T) {
	rc := createTestResultCollector(t)

	rc.SetBatchSize(50)
	assert.Equal(t, 50, rc.batchSize)

	// Test invalid size
	rc.SetBatchSize(0)
	assert.Equal(t, 50, rc.batchSize) // Should not change

	rc.SetBatchSize(-1)
	assert.Equal(t, 50, rc.batchSize) // Should not change
}

func TestResultCollector_SetFlushInterval(t *testing.T) {
	rc := createTestResultCollector(t)
	oldTicker := rc.flushTicker

	rc.SetFlushInterval(5 * time.Second)
	assert.NotEqual(t, oldTicker, rc.flushTicker)

	// Test invalid interval
	rc.SetFlushInterval(0)
	// Ticker should not change for invalid interval
}

// Helper function to create a test result collector
func createTestResultCollector(t *testing.T) *ResultCollector {
	config := &common.BotConfig{
		ID:        "test-bot",
		MasterURL: "http://localhost:8080",
		Timeouts: common.BotTimeoutConfig{
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

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	rc, err := NewResultCollector(config, config.MasterURL, logger)
	require.NoError(t, err)

	return rc
}
