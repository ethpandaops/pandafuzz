package events_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/messaging/events"
)

// TestEvent is a simple test event
type TestEvent struct {
	events.BaseEvent
	Message string `json:"message"`
}

func NewTestEvent(message string) *TestEvent {
	return &TestEvent{
		BaseEvent: events.BaseEvent{
			EventType:    "test.event",
			EventTime:    time.Now().UTC(),
			AggregateID_: "test-123",
			Version_:     1,
			Metadata_:    make(map[string]string),
		},
		Message: message,
	}
}

func TestBusStartStop(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	config := events.DefaultBusConfig()
	config.Logger = logger.WithField("test", "bus_start_stop")
	bus := events.NewBus(config)

	ctx := context.Background()

	// Test starting the bus
	err := bus.Start(ctx)
	require.NoError(t, err)

	// Test starting already started bus
	err = bus.Start(ctx)
	assert.Error(t, err)

	// Test stopping the bus
	err = bus.Stop()
	require.NoError(t, err)

	// Test stopping already stopped bus
	err = bus.Stop()
	assert.Error(t, err)
}

func TestBusPublishSubscribe(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	config := events.DefaultBusConfig()
	config.Logger = logger.WithField("test", "publish_subscribe")
	bus := events.NewBus(config)

	ctx := context.Background()
	err := bus.Start(ctx)
	require.NoError(t, err)
	defer bus.Stop()

	// Create a handler that tracks received events
	var receivedEvents []events.Event
	var mu sync.Mutex

	handler := events.NewHandlerWrapper("test.event", func(ctx context.Context, event events.Event) error {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents = append(receivedEvents, event)
		return nil
	})

	// Subscribe to events
	err = bus.Subscribe("test.event", handler)
	require.NoError(t, err)

	// Publish events
	event1 := NewTestEvent("Hello")
	err = bus.Publish(ctx, event1)
	require.NoError(t, err)

	event2 := NewTestEvent("World")
	err = bus.Publish(ctx, event2)
	require.NoError(t, err)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Verify events were received
	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, receivedEvents, 2)
	assert.Equal(t, "Hello", receivedEvents[0].(*TestEvent).Message)
	assert.Equal(t, "World", receivedEvents[1].(*TestEvent).Message)
}

func TestBusMultipleHandlers(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	config := events.DefaultBusConfig()
	config.Logger = logger.WithField("test", "multiple_handlers")
	bus := events.NewBus(config)

	ctx := context.Background()
	err := bus.Start(ctx)
	require.NoError(t, err)
	defer bus.Stop()

	// Create multiple handlers
	var handler1Count, handler2Count int32

	handler1 := events.NewHandlerWrapper("test.event", func(ctx context.Context, event events.Event) error {
		atomic.AddInt32(&handler1Count, 1)
		return nil
	})

	handler2 := events.NewHandlerWrapper("test.event", func(ctx context.Context, event events.Event) error {
		atomic.AddInt32(&handler2Count, 1)
		return nil
	})

	// Subscribe both handlers
	err = bus.Subscribe("test.event", handler1)
	require.NoError(t, err)

	err = bus.Subscribe("test.event", handler2)
	require.NoError(t, err)

	// Publish event
	event := NewTestEvent("Multi handler test")
	err = bus.Publish(ctx, event)
	require.NoError(t, err)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Both handlers should have received the event
	assert.Equal(t, int32(1), atomic.LoadInt32(&handler1Count))
	assert.Equal(t, int32(1), atomic.LoadInt32(&handler2Count))
}

func TestBusWildcardHandler(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	config := events.DefaultBusConfig()
	config.Logger = logger.WithField("test", "wildcard_handler")
	bus := events.NewBus(config)

	ctx := context.Background()
	err := bus.Start(ctx)
	require.NoError(t, err)
	defer bus.Stop()

	// Create wildcard handler
	var wildcardCount int32

	wildcardHandler := events.NewHandlerWrapper("*", func(ctx context.Context, event events.Event) error {
		atomic.AddInt32(&wildcardCount, 1)
		return nil
	})

	// Subscribe wildcard handler
	err = bus.Subscribe("*", wildcardHandler)
	require.NoError(t, err)

	// Publish different event types
	event1 := NewTestEvent("Event 1")
	err = bus.Publish(ctx, event1)
	require.NoError(t, err)

	// Create another event type
	event2 := &TestEvent{
		BaseEvent: events.BaseEvent{
			EventType:    "other.event",
			EventTime:    time.Now().UTC(),
			AggregateID_: "test-456",
		},
		Message: "Event 2",
	}
	err = bus.Publish(ctx, event2)
	require.NoError(t, err)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Wildcard handler should receive all events
	assert.Equal(t, int32(2), atomic.LoadInt32(&wildcardCount))
}

func TestBusHandlerError(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	config := events.DefaultBusConfig()
	config.Logger = logger.WithField("test", "handler_error")
	config.MaxRetries = 2
	config.RetryDelay = 10 * time.Millisecond
	bus := events.NewBus(config)

	ctx := context.Background()
	err := bus.Start(ctx)
	require.NoError(t, err)
	defer bus.Stop()

	// Create handler that fails
	var attemptCount int32
	handler := events.NewHandlerWrapper("test.event", func(ctx context.Context, event events.Event) error {
		count := atomic.AddInt32(&attemptCount, 1)
		if count <= 2 {
			return errors.New("handler error")
		}
		return nil
	})

	// Subscribe handler
	err = bus.Subscribe("test.event", handler)
	require.NoError(t, err)

	// Publish event
	event := NewTestEvent("Error test")
	err = bus.Publish(ctx, event)
	require.NoError(t, err)

	// Wait for retries
	time.Sleep(200 * time.Millisecond)

	// Handler should have been called multiple times due to retries
	assert.GreaterOrEqual(t, atomic.LoadInt32(&attemptCount), int32(3))
}

func TestBusUnsubscribe(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	config := events.DefaultBusConfig()
	config.Logger = logger.WithField("test", "unsubscribe")
	bus := events.NewBus(config)

	ctx := context.Background()
	err := bus.Start(ctx)
	require.NoError(t, err)
	defer bus.Stop()

	// Create handler
	var count int32
	handler := events.NewHandlerWrapper("test.event", func(ctx context.Context, event events.Event) error {
		atomic.AddInt32(&count, 1)
		return nil
	})

	// Subscribe handler
	err = bus.Subscribe("test.event", handler)
	require.NoError(t, err)

	// Publish event
	event := NewTestEvent("Before unsubscribe")
	err = bus.Publish(ctx, event)
	require.NoError(t, err)

	// Wait for processing
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&count))

	// Unsubscribe handler
	err = bus.Unsubscribe("test.event", handler)
	require.NoError(t, err)

	// Publish another event
	event2 := NewTestEvent("After unsubscribe")
	err = bus.Publish(ctx, event2)
	require.NoError(t, err)

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	// Count should still be 1 since handler was unsubscribed
	assert.Equal(t, int32(1), atomic.LoadInt32(&count))
}

func TestBusPublishAsync(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	config := events.DefaultBusConfig()
	config.Logger = logger.WithField("test", "publish_async")
	bus := events.NewBus(config)

	ctx := context.Background()
	err := bus.Start(ctx)
	require.NoError(t, err)
	defer bus.Stop()

	// Create handler
	received := make(chan events.Event, 1)
	handler := events.NewHandlerWrapper("test.event", func(ctx context.Context, event events.Event) error {
		select {
		case received <- event:
		default:
		}
		return nil
	})

	// Subscribe handler
	err = bus.Subscribe("test.event", handler)
	require.NoError(t, err)

	// Publish async
	event := NewTestEvent("Async test")
	err = bus.PublishAsync(ctx, event)
	require.NoError(t, err)

	// Wait for event to be received
	select {
	case e := <-received:
		assert.Equal(t, "Async test", e.(*TestEvent).Message)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for async event")
	}
}

func TestBusBufferFull(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	config := events.DefaultBusConfig()
	config.Logger = logger.WithField("test", "buffer_full")
	config.BufferSize = 1
	config.Workers = 1
	bus := events.NewBus(config)

	ctx := context.Background()
	err := bus.Start(ctx)
	require.NoError(t, err)
	defer bus.Stop()

	// Create slow handler
	handler := events.NewHandlerWrapper("test.event", func(ctx context.Context, event events.Event) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})

	// Subscribe handler
	err = bus.Subscribe("test.event", handler)
	require.NoError(t, err)

	// Fill the buffer
	event1 := NewTestEvent("Event 1")
	err = bus.Publish(ctx, event1)
	require.NoError(t, err)

	event2 := NewTestEvent("Event 2")
	err = bus.Publish(ctx, event2)
	require.NoError(t, err)

	// This should fail as buffer is full
	event3 := NewTestEvent("Event 3")
	err = bus.Publish(ctx, event3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "buffer full")
}
