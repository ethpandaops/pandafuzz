package fuzzer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventHandler is a test implementation of FuzzerEventHandler
type TestEventHandler struct {
	mu     sync.Mutex
	events []common.FuzzerEvent
}

func (h *TestEventHandler) HandleEvent(ctx context.Context, event common.FuzzerEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, event)
	return nil
}

func (h *TestEventHandler) GetEvents() []common.FuzzerEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]common.FuzzerEvent{}, h.events...)
}

func TestAFLPlusPlusEventEmission(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	afl := NewAFLPlusPlus(logger)

	// Create test event handler
	handler := &TestEventHandler{}

	// Register the handler with the base fuzzer
	err := afl.BaseFuzzer.RegisterEventHandler(handler)
	require.NoError(t, err)

	ctx := context.Background()

	// Test started event
	afl.EmitStartedEvent(ctx, "test_target", map[string]interface{}{
		"fuzzer": "AFL++",
		"test":   true,
	})

	// Test stats event
	stats := FuzzerStats{
		Executions:      1000,
		ExecPerSecond:   500,
		CoveragePercent: 75.5,
		UniqueCrashes:   2,
		CorpusSize:      50,
	}
	afl.EmitStatsEvent(ctx, "test_target", stats)

	// Test crash event
	crash := &common.CrashResult{
		ID:    "test_crash_1",
		JobID: "test_target",
		Hash:  "abc123",
		Type:  "segfault",
	}
	afl.EmitCrashFoundEvent(ctx, "test_target", crash)

	// Test stopped event
	afl.EmitStoppedEvent(ctx, "test_target", "test completed")

	// Give events time to be processed
	time.Sleep(200 * time.Millisecond)

	// Verify events were emitted
	events := handler.GetEvents()
	assert.Len(t, events, 4)

	// Since events are processed asynchronously, order might vary
	// Create a map to find events by type
	eventsByType := make(map[common.FuzzerEventType]common.FuzzerEvent)
	for _, event := range events {
		eventsByType[event.Type] = event
	}

	// Verify all event types are present
	assert.Contains(t, eventsByType, common.FuzzerEventStarted)
	assert.Contains(t, eventsByType, common.FuzzerEventStats)
	assert.Contains(t, eventsByType, common.FuzzerEventCrashFound)
	assert.Contains(t, eventsByType, common.FuzzerEventStopped)

	// Check event data
	startedEvent := eventsByType[common.FuzzerEventStarted]
	assert.Equal(t, "test_target", startedEvent.JobID)
	assert.Equal(t, "AFL++", startedEvent.Data["fuzzer"])

	statsEvent := eventsByType[common.FuzzerEventStats]
	assert.Equal(t, int64(1000), statsEvent.Data["executions"])
	assert.Equal(t, float64(500), statsEvent.Data["exec_per_second"])

	crashEvent := eventsByType[common.FuzzerEventCrashFound]
	assert.Equal(t, "test_crash_1", crashEvent.Data["crash_id"])
	assert.Equal(t, "abc123", crashEvent.Data["hash"])

	stoppedEvent := eventsByType[common.FuzzerEventStopped]
	assert.Equal(t, "test completed", stoppedEvent.Data["reason"])
}

func TestLibFuzzerEventEmission(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	lf := NewLibFuzzer(logger)

	// Create test event handler
	handler := &TestEventHandler{}

	// Register the handler with the base fuzzer
	err := lf.BaseFuzzer.RegisterEventHandler(handler)
	require.NoError(t, err)

	ctx := context.Background()

	// Test coverage event
	coverage := &common.CoverageResult{
		ID:        "test_cov_1",
		JobID:     "test_target",
		Edges:     1000,
		NewEdges:  50,
		ExecCount: 5000,
	}
	lf.EmitCoverageEvent(ctx, "test_target", coverage)

	// Test corpus update event
	corpusUpdate := &common.CorpusUpdate{
		ID:        "test_corpus_1",
		JobID:     "test_target",
		Files:     []string{"file1", "file2", "file3"},
		TotalSize: 1024,
	}
	lf.EmitCorpusUpdateEvent(ctx, "test_target", corpusUpdate)

	// Test error event
	testErr := &FuzzerError{
		Type:    ErrInternal,
		Message: "test error",
		Fuzzer:  "LibFuzzer",
		Code:    99,
	}
	lf.EmitErrorEvent(ctx, "test_target", testErr)

	// Give events time to be processed
	time.Sleep(200 * time.Millisecond)

	// Verify events were emitted
	events := handler.GetEvents()
	assert.Len(t, events, 3)

	// Since events are processed asynchronously, order might vary
	// Create a map to find events by type
	eventsByType := make(map[common.FuzzerEventType]common.FuzzerEvent)
	for _, event := range events {
		eventsByType[event.Type] = event
	}

	// Verify all event types are present
	assert.Contains(t, eventsByType, common.FuzzerEventCoverage)
	assert.Contains(t, eventsByType, common.FuzzerEventCorpusUpdate)
	assert.Contains(t, eventsByType, common.FuzzerEventError)

	// Check event data
	coverageEvent := eventsByType[common.FuzzerEventCoverage]
	assert.Equal(t, "test_cov_1", coverageEvent.Data["coverage_id"])
	assert.Equal(t, int(1000), coverageEvent.Data["edges"])
	assert.Equal(t, int(50), coverageEvent.Data["new_edges"])

	corpusEvent := eventsByType[common.FuzzerEventCorpusUpdate]
	assert.Equal(t, "test_corpus_1", corpusEvent.Data["corpus_id"])
	assert.Equal(t, int(3), corpusEvent.Data["file_count"])

	errorEvent := eventsByType[common.FuzzerEventError]
	assert.Equal(t, "test error", errorEvent.Data["error"])
}

func TestBaseFuzzerEventHandling(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	base := NewBaseFuzzer(logger)

	// Test registering nil handler
	err := base.RegisterEventHandler(nil)
	assert.Error(t, err)

	// Register multiple handlers
	handler1 := &TestEventHandler{}
	handler2 := &TestEventHandler{}

	err = base.RegisterEventHandler(handler1)
	require.NoError(t, err)

	err = base.RegisterEventHandler(handler2)
	require.NoError(t, err)

	assert.Equal(t, 2, base.GetEventHandlerCount())

	// Emit event
	ctx := context.Background()
	event := common.FuzzerEvent{
		Type:      common.FuzzerEventStarted,
		Timestamp: time.Now(),
		JobID:     "test_job",
		Data: map[string]interface{}{
			"test": true,
		},
	}

	base.EmitEvent(ctx, event)

	// Give events time to be processed
	time.Sleep(100 * time.Millisecond)

	// Both handlers should receive the event
	assert.Len(t, handler1.GetEvents(), 1)
	assert.Len(t, handler2.GetEvents(), 1)

	// Clear handlers
	base.ClearEventHandlers()
	assert.Equal(t, 0, base.GetEventHandlerCount())
}
