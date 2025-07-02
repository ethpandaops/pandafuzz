package bot

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/fuzzer"
	"github.com/sirupsen/logrus"
)

const (
	defaultBatchSize     = 100
	defaultFlushInterval = 10 * time.Second
	maxRetries           = 3
	retryDelay           = 1 * time.Second
)

// ResultCollector handles collection and batch submission of fuzzing results
type ResultCollector struct {
	config      *common.BotConfig
	logger      *logrus.Logger
	masterURL   string
	httpClient  *http.Client
	eventChan   chan common.FuzzerEvent
	batchSize   int
	flushTicker *time.Ticker

	// Batch buffers
	crashBatch    []*common.CrashResult
	coverageBatch []*common.CoverageResult
	corpusBatch   []*common.CorpusUpdate
	statsBatch    []*fuzzer.FuzzerStats

	// Synchronization
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	retryClient *RetryClient
}

// NewResultCollector creates a new result collector
func NewResultCollector(config *common.BotConfig, masterURL string, logger *logrus.Logger) (*ResultCollector, error) {
	// Create retry client for network communication
	retryClient, err := NewRetryClient(config, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create retry client: %w", err)
	}

	// Configure HTTP client with appropriate timeouts
	httpClient := &http.Client{
		Timeout: config.Timeouts.ResultReporting,
		Transport: &http.Transport{
			MaxIdleConns:       10,
			IdleConnTimeout:    30 * time.Second,
			DisableCompression: false,
			MaxConnsPerHost:    5,
		},
	}

	return &ResultCollector{
		config:        config,
		logger:        logger,
		masterURL:     masterURL,
		httpClient:    httpClient,
		eventChan:     make(chan common.FuzzerEvent, 1000),
		batchSize:     defaultBatchSize,
		flushTicker:   time.NewTicker(defaultFlushInterval),
		crashBatch:    make([]*common.CrashResult, 0, defaultBatchSize),
		coverageBatch: make([]*common.CoverageResult, 0, defaultBatchSize),
		corpusBatch:   make([]*common.CorpusUpdate, 0, defaultBatchSize),
		statsBatch:    make([]*fuzzer.FuzzerStats, 0, defaultBatchSize),
		retryClient:   retryClient,
	}, nil
}

// Start begins the result collection process
func (rc *ResultCollector) Start(ctx context.Context) error {
	rc.mu.Lock()
	if rc.ctx != nil {
		rc.mu.Unlock()
		return fmt.Errorf("result collector already started")
	}

	rc.ctx, rc.cancel = context.WithCancel(ctx)
	rc.mu.Unlock()

	rc.logger.Info("Starting result collector")

	// Start the main processing loop
	rc.wg.Add(1)
	go rc.processLoop()

	return nil
}

// Stop halts the result collector and flushes remaining results
func (rc *ResultCollector) Stop() error {
	rc.mu.Lock()
	if rc.cancel == nil {
		rc.mu.Unlock()
		return fmt.Errorf("result collector not started")
	}

	// Get the cancel function and set to nil to prevent double stop
	cancel := rc.cancel
	rc.cancel = nil
	rc.mu.Unlock()

	rc.logger.Info("Stopping result collector")

	// Cancel context to signal shutdown
	cancel()

	// Wait for processing to complete
	rc.wg.Wait()

	// Final flush of any remaining results
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := rc.flushAll(ctx); err != nil {
		rc.logger.WithError(err).Error("Failed to flush final results")
		return err
	}

	// Stop ticker
	rc.flushTicker.Stop()

	rc.logger.Info("Result collector stopped")
	return nil
}

// HandleEvent processes incoming fuzzer events
func (rc *ResultCollector) HandleEvent(event common.FuzzerEvent) error {
	select {
	case rc.eventChan <- event:
		return nil
	case <-rc.ctx.Done():
		return fmt.Errorf("result collector is shutting down")
	default:
		return fmt.Errorf("event channel full, dropping event")
	}
}

// processLoop is the main event processing loop
func (rc *ResultCollector) processLoop() {
	defer rc.wg.Done()

	for {
		select {
		case <-rc.ctx.Done():
			rc.logger.Debug("Process loop context cancelled")
			return

		case event := <-rc.eventChan:
			rc.processEvent(event)

		case <-rc.flushTicker.C:
			if err := rc.flushAll(rc.ctx); err != nil {
				rc.logger.WithError(err).Error("Failed to flush results on timer")
			}
		}
	}
}

// processEvent routes events to appropriate processors
func (rc *ResultCollector) processEvent(event common.FuzzerEvent) {
	rc.logger.WithFields(logrus.Fields{
		"type":   event.Type,
		"job_id": event.JobID,
	}).Debug("Processing fuzzer event")

	switch event.Type {
	case common.FuzzerEventCrashFound:
		rc.processCrashEvent(event)
	case common.FuzzerEventCoverage:
		rc.processCoverageEvent(event)
	case common.FuzzerEventCorpusUpdate:
		rc.processCorpusEvent(event)
	case common.FuzzerEventStats:
		rc.processStatsEvent(event)
	default:
		rc.logger.WithField("type", event.Type).Warn("Unknown event type")
	}
}

// processCrashEvent handles crash events
func (rc *ResultCollector) processCrashEvent(event common.FuzzerEvent) {
	crashData, ok := event.Data["crash"].(*common.CrashResult)
	if !ok {
		rc.logger.Error("Invalid crash data in event")
		return
	}

	rc.mu.Lock()
	rc.crashBatch = append(rc.crashBatch, crashData)
	shouldFlush := len(rc.crashBatch) >= rc.batchSize
	rc.mu.Unlock()

	if shouldFlush {
		if err := rc.flushCrashes(rc.ctx); err != nil {
			rc.logger.WithError(err).Error("Failed to flush crash batch")
		}
	}
}

// processCoverageEvent handles coverage events
func (rc *ResultCollector) processCoverageEvent(event common.FuzzerEvent) {
	coverageData, ok := event.Data["coverage"].(*common.CoverageResult)
	if !ok {
		rc.logger.Error("Invalid coverage data in event")
		return
	}

	rc.mu.Lock()
	rc.coverageBatch = append(rc.coverageBatch, coverageData)
	shouldFlush := len(rc.coverageBatch) >= rc.batchSize
	rc.mu.Unlock()

	if shouldFlush {
		if err := rc.flushCoverage(rc.ctx); err != nil {
			rc.logger.WithError(err).Error("Failed to flush coverage batch")
		}
	}
}

// processCorpusEvent handles corpus update events
func (rc *ResultCollector) processCorpusEvent(event common.FuzzerEvent) {
	corpusData, ok := event.Data["corpus"].(*common.CorpusUpdate)
	if !ok {
		rc.logger.Error("Invalid corpus data in event")
		return
	}

	rc.mu.Lock()
	rc.corpusBatch = append(rc.corpusBatch, corpusData)
	shouldFlush := len(rc.corpusBatch) >= rc.batchSize
	rc.mu.Unlock()

	if shouldFlush {
		if err := rc.flushCorpus(rc.ctx); err != nil {
			rc.logger.WithError(err).Error("Failed to flush corpus batch")
		}
	}
}

// processStatsEvent handles statistics events
func (rc *ResultCollector) processStatsEvent(event common.FuzzerEvent) {
	statsData, ok := event.Data["stats"].(*fuzzer.FuzzerStats)
	if !ok {
		rc.logger.Error("Invalid stats data in event")
		return
	}

	rc.mu.Lock()
	rc.statsBatch = append(rc.statsBatch, statsData)
	shouldFlush := len(rc.statsBatch) >= rc.batchSize
	rc.mu.Unlock()

	if shouldFlush {
		if err := rc.flushStats(rc.ctx); err != nil {
			rc.logger.WithError(err).Error("Failed to flush stats batch")
		}
	}
}

// flushAll flushes all pending results
func (rc *ResultCollector) flushAll(ctx context.Context) error {
	var firstErr error

	if err := rc.flushCrashes(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := rc.flushCoverage(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := rc.flushCorpus(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := rc.flushStats(ctx); err != nil && firstErr == nil {
		firstErr = err
	}

	return firstErr
}

// flushCrashes sends all pending crash results
func (rc *ResultCollector) flushCrashes(ctx context.Context) error {
	rc.mu.Lock()
	if len(rc.crashBatch) == 0 {
		rc.mu.Unlock()
		return nil
	}

	// Copy and clear the batch
	crashes := make([]*common.CrashResult, len(rc.crashBatch))
	copy(crashes, rc.crashBatch)
	rc.crashBatch = rc.crashBatch[:0]
	rc.mu.Unlock()

	rc.logger.WithField("count", len(crashes)).Debug("Flushing crash batch")

	// Send individual crashes using retry client
	for _, crash := range crashes {
		// Skip if retry client is nil (for testing)
		if rc.retryClient == nil {
			rc.logger.Debug("Skipping crash report - no retry client configured")
			continue
		}

		if err := rc.retryClient.ReportCrash(crash); err != nil {
			rc.logger.WithError(err).WithField("crash_id", crash.ID).Error("Failed to report crash")
			// Re-add to batch for retry
			rc.mu.Lock()
			rc.crashBatch = append(rc.crashBatch, crash)
			rc.mu.Unlock()
		}
	}

	return nil
}

// flushCoverage sends all pending coverage results
func (rc *ResultCollector) flushCoverage(ctx context.Context) error {
	rc.mu.Lock()
	if len(rc.coverageBatch) == 0 {
		rc.mu.Unlock()
		return nil
	}

	// Copy and clear the batch
	coverage := make([]*common.CoverageResult, len(rc.coverageBatch))
	copy(coverage, rc.coverageBatch)
	rc.coverageBatch = rc.coverageBatch[:0]
	rc.mu.Unlock()

	rc.logger.WithField("count", len(coverage)).Debug("Flushing coverage batch")

	// Send individual coverage results using retry client
	for _, cov := range coverage {
		// Skip if retry client is nil (for testing)
		if rc.retryClient == nil {
			rc.logger.Debug("Skipping coverage report - no retry client configured")
			continue
		}

		if err := rc.retryClient.ReportCoverage(cov); err != nil {
			rc.logger.WithError(err).WithField("coverage_id", cov.ID).Error("Failed to report coverage")
			// Re-add to batch for retry
			rc.mu.Lock()
			rc.coverageBatch = append(rc.coverageBatch, cov)
			rc.mu.Unlock()
		}
	}

	return nil
}

// flushCorpus sends all pending corpus updates
func (rc *ResultCollector) flushCorpus(ctx context.Context) error {
	rc.mu.Lock()
	if len(rc.corpusBatch) == 0 {
		rc.mu.Unlock()
		return nil
	}

	// Copy and clear the batch
	corpus := make([]*common.CorpusUpdate, len(rc.corpusBatch))
	copy(corpus, rc.corpusBatch)
	rc.corpusBatch = rc.corpusBatch[:0]
	rc.mu.Unlock()

	rc.logger.WithField("count", len(corpus)).Debug("Flushing corpus batch")

	// Send individual corpus updates using retry client
	for _, corp := range corpus {
		// Skip if retry client is nil (for testing)
		if rc.retryClient == nil {
			rc.logger.Debug("Skipping corpus report - no retry client configured")
			continue
		}

		if err := rc.retryClient.ReportCorpusUpdate(corp); err != nil {
			rc.logger.WithError(err).WithField("corpus_id", corp.ID).Error("Failed to report corpus update")
			// Re-add to batch for retry
			rc.mu.Lock()
			rc.corpusBatch = append(rc.corpusBatch, corp)
			rc.mu.Unlock()
		}
	}

	return nil
}

// flushStats sends all pending statistics
func (rc *ResultCollector) flushStats(ctx context.Context) error {
	rc.mu.Lock()
	if len(rc.statsBatch) == 0 {
		rc.mu.Unlock()
		return nil
	}

	// Copy and clear the batch
	stats := make([]*fuzzer.FuzzerStats, len(rc.statsBatch))
	copy(stats, rc.statsBatch)
	rc.statsBatch = rc.statsBatch[:0]
	rc.mu.Unlock()

	rc.logger.WithField("count", len(stats)).Debug("Flushing stats batch")

	// Convert stats to status map and send using retry client
	for _, stat := range stats {
		// Skip if retry client is nil (for testing)
		if rc.retryClient == nil {
			rc.logger.Debug("Skipping stats report - no retry client configured")
			continue
		}

		statusMap := map[string]any{
			"type":             "fuzzer_stats",
			"timestamp":        time.Now(),
			"executions":       stat.Executions,
			"exec_per_second":  stat.ExecPerSecond,
			"total_edges":      stat.TotalEdges,
			"covered_edges":    stat.CoveredEdges,
			"coverage_percent": stat.CoveragePercent,
			"unique_crashes":   stat.UniqueCrashes,
			"total_crashes":    stat.TotalCrashes,
			"corpus_size":      stat.CorpusSize,
			"cpu_usage":        stat.CPUUsage,
			"memory_usage":     stat.MemoryUsage,
		}

		if err := rc.retryClient.ReportStatus(statusMap); err != nil {
			rc.logger.WithError(err).Error("Failed to report stats")
			// Re-add to batch for retry
			rc.mu.Lock()
			rc.statsBatch = append(rc.statsBatch, stat)
			rc.mu.Unlock()
		}
	}

	return nil
}

// batchAndSend is a convenience method that handles batching and sending results
func (rc *ResultCollector) batchAndSend(ctx context.Context) error {
	// This method is called by external components to trigger a manual flush
	return rc.flushAll(ctx)
}

// GetStats returns statistics about the collector
func (rc *ResultCollector) GetStats() map[string]any {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	return map[string]any{
		"crash_batch_size":    len(rc.crashBatch),
		"coverage_batch_size": len(rc.coverageBatch),
		"corpus_batch_size":   len(rc.corpusBatch),
		"stats_batch_size":    len(rc.statsBatch),
		"event_queue_size":    len(rc.eventChan),
		"event_queue_cap":     cap(rc.eventChan),
		"batch_size_limit":    rc.batchSize,
		"flush_interval":      rc.flushTicker,
	}
}

// SetBatchSize updates the batch size for result collection
func (rc *ResultCollector) SetBatchSize(size int) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if size > 0 {
		rc.batchSize = size
		rc.logger.WithField("batch_size", size).Info("Updated batch size")
	}
}

// SetFlushInterval updates the flush interval
func (rc *ResultCollector) SetFlushInterval(interval time.Duration) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if interval > 0 {
		rc.flushTicker.Stop()
		rc.flushTicker = time.NewTicker(interval)
		rc.logger.WithField("interval", interval).Info("Updated flush interval")
	}
}
