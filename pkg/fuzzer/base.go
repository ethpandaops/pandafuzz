package fuzzer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// BaseFuzzer provides common functionality for all fuzzer implementations
// including event handling, thread-safe operations, and basic lifecycle management.
type BaseFuzzer struct {
	// eventHandlers holds all registered event handlers
	eventHandlers []common.FuzzerEventHandler

	// mu provides thread safety for event handler operations
	mu sync.RWMutex

	// logger for debugging and error reporting
	logger logrus.FieldLogger
}

// NewBaseFuzzer creates a new BaseFuzzer instance with the given logger.
// If logger is nil, a default logger will be created.
func NewBaseFuzzer(logger logrus.FieldLogger) *BaseFuzzer {
	if logger == nil {
		defaultLogger := logrus.New()
		defaultLogger.SetLevel(logrus.InfoLevel)
		logger = defaultLogger.WithField("component", "base_fuzzer")
	}

	return &BaseFuzzer{
		eventHandlers: make([]common.FuzzerEventHandler, 0),
		logger:        logger,
	}
}

// RegisterEventHandler adds a new event handler to the fuzzer.
// Multiple handlers can be registered and will be called in order.
// Thread-safe: can be called concurrently.
func (b *BaseFuzzer) RegisterEventHandler(handler common.FuzzerEventHandler) error {
	if handler == nil {
		return fmt.Errorf("cannot register nil event handler")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.eventHandlers = append(b.eventHandlers, handler)
	b.logger.WithField("handler_count", len(b.eventHandlers)).Debug("Registered new event handler")

	return nil
}

// EmitEvent sends an event to all registered handlers.
// Events are sent asynchronously to prevent blocking the fuzzer.
// Errors from individual handlers are logged but don't stop other handlers.
// Thread-safe: can be called concurrently.
func (b *BaseFuzzer) EmitEvent(ctx context.Context, event common.FuzzerEvent) {
	// Make a copy of handlers under read lock to minimize lock time
	b.mu.RLock()
	handlers := make([]common.FuzzerEventHandler, len(b.eventHandlers))
	copy(handlers, b.eventHandlers)
	b.mu.RUnlock()

	if len(handlers) == 0 {
		b.logger.WithFields(logrus.Fields{
			"event_type": event.Type,
			"job_id":     event.JobID,
		}).Debug("No handlers registered for event")
		return
	}

	// Process each handler asynchronously
	for i, handler := range handlers {
		handler := handler // Capture for goroutine
		handlerIndex := i

		go func() {
			// Create a timeout context for each handler
			handlerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			b.logger.WithFields(logrus.Fields{
				"event_type":    event.Type,
				"job_id":        event.JobID,
				"handler_index": handlerIndex,
			}).Debug("Dispatching event to handler")

			if err := handler.HandleEvent(handlerCtx, event); err != nil {
				b.logger.WithError(err).WithFields(logrus.Fields{
					"event_type":    event.Type,
					"job_id":        event.JobID,
					"handler_index": handlerIndex,
				}).Error("Event handler failed")
			}
		}()
	}
}

// ClearEventHandlers removes all registered event handlers.
// Useful for cleanup or reconfiguration.
// Thread-safe: can be called concurrently.
func (b *BaseFuzzer) ClearEventHandlers() {
	b.mu.Lock()
	defer b.mu.Unlock()

	previousCount := len(b.eventHandlers)
	b.eventHandlers = make([]common.FuzzerEventHandler, 0)

	b.logger.WithField("cleared_count", previousCount).Debug("Cleared all event handlers")
}

// GetEventHandlerCount returns the number of registered event handlers.
// Thread-safe: can be called concurrently.
func (b *BaseFuzzer) GetEventHandlerCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.eventHandlers)
}

// EmitStartedEvent is a convenience method to emit a fuzzer started event
func (b *BaseFuzzer) EmitStartedEvent(ctx context.Context, jobID string, data map[string]interface{}) {
	event := common.FuzzerEvent{
		Type:      common.FuzzerEventStarted,
		Timestamp: time.Now(),
		JobID:     jobID,
		Data:      data,
	}
	b.EmitEvent(ctx, event)
}

// EmitStoppedEvent is a convenience method to emit a fuzzer stopped event
func (b *BaseFuzzer) EmitStoppedEvent(ctx context.Context, jobID string, reason string) {
	data := map[string]interface{}{
		"reason": reason,
	}
	event := common.FuzzerEvent{
		Type:      common.FuzzerEventStopped,
		Timestamp: time.Now(),
		JobID:     jobID,
		Data:      data,
	}
	b.EmitEvent(ctx, event)
}

// EmitCrashFoundEvent is a convenience method to emit a crash found event
func (b *BaseFuzzer) EmitCrashFoundEvent(ctx context.Context, jobID string, crash *common.CrashResult) {
	data := map[string]interface{}{
		"crash_id":  crash.ID,
		"hash":      crash.Hash,
		"type":      crash.Type,
		"signal":    crash.Signal,
		"exit_code": crash.ExitCode,
		"size":      crash.Size,
		"is_unique": crash.IsUnique,
	}
	event := common.FuzzerEvent{
		Type:      common.FuzzerEventCrashFound,
		Timestamp: time.Now(),
		JobID:     jobID,
		Data:      data,
	}
	b.EmitEvent(ctx, event)
}

// EmitCorpusUpdateEvent is a convenience method to emit a corpus update event
func (b *BaseFuzzer) EmitCorpusUpdateEvent(ctx context.Context, jobID string, corpusUpdate *common.CorpusUpdate) {
	data := map[string]interface{}{
		"corpus_id":  corpusUpdate.ID,
		"files":      corpusUpdate.Files,
		"total_size": corpusUpdate.TotalSize,
		"file_count": len(corpusUpdate.Files),
	}
	event := common.FuzzerEvent{
		Type:      common.FuzzerEventCorpusUpdate,
		Timestamp: time.Now(),
		JobID:     jobID,
		Data:      data,
	}
	b.EmitEvent(ctx, event)
}

// EmitCoverageEvent is a convenience method to emit a coverage event
func (b *BaseFuzzer) EmitCoverageEvent(ctx context.Context, jobID string, coverage *common.CoverageResult) {
	data := map[string]interface{}{
		"coverage_id": coverage.ID,
		"edges":       coverage.Edges,
		"new_edges":   coverage.NewEdges,
		"exec_count":  coverage.ExecCount,
	}
	event := common.FuzzerEvent{
		Type:      common.FuzzerEventCoverage,
		Timestamp: time.Now(),
		JobID:     jobID,
		Data:      data,
	}
	b.EmitEvent(ctx, event)
}

// EmitStatsEvent is a convenience method to emit a stats event
func (b *BaseFuzzer) EmitStatsEvent(ctx context.Context, jobID string, stats FuzzerStats) {
	data := map[string]interface{}{
		"executions":       stats.Executions,
		"exec_per_second":  stats.ExecPerSecond,
		"coverage_percent": stats.CoveragePercent,
		"unique_crashes":   stats.UniqueCrashes,
		"corpus_size":      stats.CorpusSize,
		"cpu_usage":        stats.CPUUsage,
		"memory_usage":     stats.MemoryUsage,
	}
	event := common.FuzzerEvent{
		Type:      common.FuzzerEventStats,
		Timestamp: time.Now(),
		JobID:     jobID,
		Data:      data,
	}
	b.EmitEvent(ctx, event)
}

// EmitErrorEvent is a convenience method to emit an error event
func (b *BaseFuzzer) EmitErrorEvent(ctx context.Context, jobID string, err error) {
	data := map[string]interface{}{
		"error": err.Error(),
		"type":  fmt.Sprintf("%T", err),
	}
	event := common.FuzzerEvent{
		Type:      common.FuzzerEventError,
		Timestamp: time.Now(),
		JobID:     jobID,
		Data:      data,
	}
	b.EmitEvent(ctx, event)
}

// EmitTimeoutEvent is a convenience method to emit a timeout event
func (b *BaseFuzzer) EmitTimeoutEvent(ctx context.Context, jobID string, duration time.Duration) {
	data := map[string]interface{}{
		"duration_seconds": duration.Seconds(),
		"duration_string":  duration.String(),
	}
	event := common.FuzzerEvent{
		Type:      common.FuzzerEventTimeout,
		Timestamp: time.Now(),
		JobID:     jobID,
		Data:      data,
	}
	b.EmitEvent(ctx, event)
}
