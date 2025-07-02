package fuzzer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEventHandler is a test implementation of FuzzerEventHandler
type mockEventHandler struct {
	mu         sync.Mutex
	events     []common.FuzzerEvent
	handleFunc func(ctx context.Context, event common.FuzzerEvent) error
}

func (m *mockEventHandler) HandleEvent(ctx context.Context, event common.FuzzerEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.events = append(m.events, event)

	if m.handleFunc != nil {
		return m.handleFunc(ctx, event)
	}
	return nil
}

func (m *mockEventHandler) getEvents() []common.FuzzerEvent {
	m.mu.Lock()
	defer m.mu.Unlock()

	events := make([]common.FuzzerEvent, len(m.events))
	copy(events, m.events)
	return events
}

func TestNewBaseFuzzer(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		logger := logrus.New()
		bf := NewBaseFuzzer(logger.WithField("test", "true"))

		assert.NotNil(t, bf)
		assert.NotNil(t, bf.logger)
		assert.Empty(t, bf.eventHandlers)
	})

	t.Run("without logger", func(t *testing.T) {
		bf := NewBaseFuzzer(nil)

		assert.NotNil(t, bf)
		assert.NotNil(t, bf.logger)
		assert.Empty(t, bf.eventHandlers)
	})
}

func TestRegisterEventHandler(t *testing.T) {
	bf := NewBaseFuzzer(nil)

	t.Run("register valid handler", func(t *testing.T) {
		handler := &mockEventHandler{}
		err := bf.RegisterEventHandler(handler)

		require.NoError(t, err)
		assert.Equal(t, 1, bf.GetEventHandlerCount())
	})

	t.Run("register multiple handlers", func(t *testing.T) {
		handler2 := &mockEventHandler{}
		handler3 := &mockEventHandler{}

		err := bf.RegisterEventHandler(handler2)
		require.NoError(t, err)

		err = bf.RegisterEventHandler(handler3)
		require.NoError(t, err)

		assert.Equal(t, 3, bf.GetEventHandlerCount())
	})

	t.Run("register nil handler", func(t *testing.T) {
		err := bf.RegisterEventHandler(nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot register nil event handler")
	})
}

func TestEmitEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("emit to single handler", func(t *testing.T) {
		bf := NewBaseFuzzer(nil)
		handler := &mockEventHandler{}

		err := bf.RegisterEventHandler(handler)
		require.NoError(t, err)

		event := common.FuzzerEvent{
			Type:      common.FuzzerEventStarted,
			Timestamp: time.Now(),
			JobID:     "test-job-1",
			Data:      map[string]interface{}{"test": true},
		}

		bf.EmitEvent(ctx, event)

		// Wait for async processing
		time.Sleep(100 * time.Millisecond)

		events := handler.getEvents()
		require.Len(t, events, 1)
		assert.Equal(t, event.Type, events[0].Type)
		assert.Equal(t, event.JobID, events[0].JobID)
	})

	t.Run("emit to multiple handlers", func(t *testing.T) {
		bf := NewBaseFuzzer(nil)
		handler1 := &mockEventHandler{}
		handler2 := &mockEventHandler{}
		handler3 := &mockEventHandler{}

		err := bf.RegisterEventHandler(handler1)
		require.NoError(t, err)
		err = bf.RegisterEventHandler(handler2)
		require.NoError(t, err)
		err = bf.RegisterEventHandler(handler3)
		require.NoError(t, err)

		event := common.FuzzerEvent{
			Type:      common.FuzzerEventCrashFound,
			Timestamp: time.Now(),
			JobID:     "test-job-2",
			Data:      map[string]interface{}{"crash_id": "crash-123"},
		}

		bf.EmitEvent(ctx, event)

		// Wait for async processing
		time.Sleep(100 * time.Millisecond)

		// All handlers should receive the event
		assert.Len(t, handler1.getEvents(), 1)
		assert.Len(t, handler2.getEvents(), 1)
		assert.Len(t, handler3.getEvents(), 1)
	})

	t.Run("handler error does not affect others", func(t *testing.T) {
		bf := NewBaseFuzzer(nil)

		// Handler that returns an error
		errorHandler := &mockEventHandler{
			handleFunc: func(ctx context.Context, event common.FuzzerEvent) error {
				return errors.New("handler error")
			},
		}

		// Handler that works normally
		successHandler := &mockEventHandler{}

		err := bf.RegisterEventHandler(errorHandler)
		require.NoError(t, err)
		err = bf.RegisterEventHandler(successHandler)
		require.NoError(t, err)

		event := common.FuzzerEvent{
			Type:      common.FuzzerEventError,
			Timestamp: time.Now(),
			JobID:     "test-job-3",
		}

		bf.EmitEvent(ctx, event)

		// Wait for async processing
		time.Sleep(100 * time.Millisecond)

		// Both handlers should have received the event
		assert.Len(t, errorHandler.getEvents(), 1)
		assert.Len(t, successHandler.getEvents(), 1)
	})

	t.Run("emit with no handlers", func(t *testing.T) {
		bf := NewBaseFuzzer(nil)

		event := common.FuzzerEvent{
			Type:      common.FuzzerEventStarted,
			Timestamp: time.Now(),
			JobID:     "test-job-4",
		}

		// Should not panic
		bf.EmitEvent(ctx, event)
	})
}

func TestClearEventHandlers(t *testing.T) {
	bf := NewBaseFuzzer(nil)

	// Add some handlers
	for i := 0; i < 5; i++ {
		err := bf.RegisterEventHandler(&mockEventHandler{})
		require.NoError(t, err)
	}

	assert.Equal(t, 5, bf.GetEventHandlerCount())

	bf.ClearEventHandlers()

	assert.Equal(t, 0, bf.GetEventHandlerCount())
}

func TestConvenienceEmitMethods(t *testing.T) {
	ctx := context.Background()
	bf := NewBaseFuzzer(nil)
	handler := &mockEventHandler{}

	err := bf.RegisterEventHandler(handler)
	require.NoError(t, err)

	jobID := "test-job-convenience"

	t.Run("EmitStartedEvent", func(t *testing.T) {
		data := map[string]interface{}{"version": "1.0"}
		bf.EmitStartedEvent(ctx, jobID, data)

		time.Sleep(100 * time.Millisecond)
		events := handler.getEvents()

		require.Len(t, events, 1)
		assert.Equal(t, common.FuzzerEventStarted, events[0].Type)
		assert.Equal(t, jobID, events[0].JobID)
		assert.Equal(t, "1.0", events[0].Data["version"])
	})

	t.Run("EmitCrashFoundEvent", func(t *testing.T) {
		handler.events = nil // Clear previous events

		crash := &common.CrashResult{
			ID:       "crash-001",
			Hash:     "abc123",
			Type:     "segfault",
			Signal:   11,
			ExitCode: 139,
			Size:     1024,
			IsUnique: true,
		}

		bf.EmitCrashFoundEvent(ctx, jobID, crash)

		time.Sleep(100 * time.Millisecond)
		events := handler.getEvents()

		require.Len(t, events, 1)
		assert.Equal(t, common.FuzzerEventCrashFound, events[0].Type)
		assert.Equal(t, "crash-001", events[0].Data["crash_id"])
		assert.Equal(t, "abc123", events[0].Data["hash"])
		assert.Equal(t, true, events[0].Data["is_unique"])
	})

	t.Run("EmitErrorEvent", func(t *testing.T) {
		handler.events = nil // Clear previous events

		testErr := errors.New("test error")
		bf.EmitErrorEvent(ctx, jobID, testErr)

		time.Sleep(100 * time.Millisecond)
		events := handler.getEvents()

		require.Len(t, events, 1)
		assert.Equal(t, common.FuzzerEventError, events[0].Type)
		assert.Equal(t, "test error", events[0].Data["error"])
	})
}

func TestConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	bf := NewBaseFuzzer(nil)

	// Test concurrent handler registration and event emission
	var wg sync.WaitGroup

	// Register handlers concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handler := &mockEventHandler{}
			err := bf.RegisterEventHandler(handler)
			assert.NoError(t, err)
		}()
	}

	// Emit events concurrently
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			event := common.FuzzerEvent{
				Type:      common.FuzzerEventStats,
				Timestamp: time.Now(),
				JobID:     fmt.Sprintf("job-%d", id),
			}
			bf.EmitEvent(ctx, event)
		}(i)
	}

	wg.Wait()

	// Verify all handlers were registered
	assert.Equal(t, 10, bf.GetEventHandlerCount())
}

