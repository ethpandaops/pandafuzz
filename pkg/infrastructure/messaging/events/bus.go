package events

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// BusConfig contains configuration for the event bus
type BusConfig struct {
	// BufferSize is the size of the event channel buffer
	BufferSize int
	// Workers is the number of worker goroutines processing events
	Workers int
	// MaxRetries is the maximum number of retries for failed handlers
	MaxRetries int
	// RetryDelay is the delay between retry attempts
	RetryDelay time.Duration
	// HandlerTimeout is the timeout for each handler execution
	HandlerTimeout time.Duration
	// Logger is the logger instance
	Logger logrus.FieldLogger
}

// DefaultBusConfig returns default bus configuration
func DefaultBusConfig() BusConfig {
	return BusConfig{
		BufferSize:     1000,
		Workers:        10,
		MaxRetries:     3,
		RetryDelay:     100 * time.Millisecond,
		HandlerTimeout: 30 * time.Second,
		Logger:         logrus.New().WithField("component", "event-bus"),
	}
}

// eventBus implements the Bus interface
type eventBus struct {
	config     BusConfig
	handlers   map[string][]Handler
	handlersMu sync.RWMutex
	eventChan  chan *EventEnvelope
	errorChan  chan error
	wg         sync.WaitGroup
	done       chan struct{}
	started    bool
	startedMu  sync.Mutex
	logger     logrus.FieldLogger
	metrics    *busMetrics
	filters    []Filter
	filtersMu  sync.RWMutex
}

// busMetrics tracks event bus metrics
type busMetrics struct {
	mu               sync.RWMutex
	eventsPublished  uint64
	eventsProcessed  uint64
	eventsFailed     uint64
	eventsRetried    uint64
	handlersExecuted uint64
	handlersFailed   uint64
}

// NewBus creates a new event bus
func NewBus(config BusConfig) Bus {
	if config.BufferSize <= 0 {
		config.BufferSize = 1000
	}
	if config.Workers <= 0 {
		config.Workers = 10
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = 100 * time.Millisecond
	}
	if config.HandlerTimeout <= 0 {
		config.HandlerTimeout = 30 * time.Second
	}
	if config.Logger == nil {
		config.Logger = logrus.New().WithField("component", "event-bus")
	}

	return &eventBus{
		config:    config,
		handlers:  make(map[string][]Handler),
		eventChan: make(chan *EventEnvelope, config.BufferSize),
		errorChan: make(chan error, config.Workers),
		done:      make(chan struct{}),
		logger:    config.Logger,
		metrics:   &busMetrics{},
		filters:   make([]Filter, 0),
	}
}

// Start starts the event bus
func (b *eventBus) Start(ctx context.Context) error {
	b.startedMu.Lock()
	defer b.startedMu.Unlock()

	if b.started {
		return errors.New("event bus already started")
	}

	b.logger.Info("Starting event bus")

	// Start worker goroutines
	for i := 0; i < b.config.Workers; i++ {
		b.wg.Add(1)
		go b.worker(i)
	}

	// Start error handler
	b.wg.Add(1)
	go b.errorHandler()

	b.started = true
	b.logger.WithField("workers", b.config.Workers).Info("Event bus started")

	return nil
}

// Stop stops the event bus
func (b *eventBus) Stop() error {
	b.startedMu.Lock()
	defer b.startedMu.Unlock()

	if !b.started {
		return errors.New("event bus not started")
	}

	b.logger.Info("Stopping event bus")

	// Signal shutdown
	close(b.done)

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		close(b.eventChan)
		close(b.errorChan)
		b.started = false
		b.logger.Info("Event bus stopped")
		return nil
	case <-time.After(30 * time.Second):
		return errors.New("timeout waiting for event bus to stop")
	}
}

// worker processes events from the channel
func (b *eventBus) worker(id int) {
	defer b.wg.Done()

	logger := b.logger.WithField("worker_id", id)
	logger.Debug("Starting event bus worker")

	for {
		select {
		case envelope := <-b.eventChan:
			if envelope == nil {
				continue
			}

			b.processEvent(envelope)

		case <-b.done:
			logger.Debug("Stopping event bus worker")
			return
		}
	}
}

// processEvent processes a single event
func (b *eventBus) processEvent(envelope *EventEnvelope) {
	event := envelope.Event
	logger := b.logger.WithFields(logrus.Fields{
		"event_type":   event.Type(),
		"aggregate_id": event.AggregateID(),
		"event_id":     event.Metadata()["event_id"],
	})

	// Apply filters
	if !b.applyFilters(event) {
		logger.Debug("Event filtered out")
		return
	}

	// Get handlers for this event type
	handlers := b.getHandlers(event.Type())
	if len(handlers) == 0 {
		logger.Debug("No handlers registered for event type")
		b.incrementProcessed()
		return
	}

	// Execute handlers
	var handlerErrors []error
	for _, handler := range handlers {
		b.incrementHandlersExecuted()

		ctx, cancel := context.WithTimeout(context.Background(), b.config.HandlerTimeout)
		err := b.executeHandler(ctx, handler, event)
		cancel()

		if err != nil {
			logger.WithError(err).Error("Handler failed to process event")
			handlerErrors = append(handlerErrors, err)
			b.incrementHandlersFailed()
		}
	}

	// Handle failures
	if len(handlerErrors) > 0 {
		if envelope.ShouldRetry() {
			envelope.IncrementRetry()
			b.incrementRetried()

			// Re-queue with delay
			go func() {
				backoff := b.config.RetryDelay * time.Duration(1<<uint(envelope.RetryCount-1))
				time.Sleep(backoff)

				select {
				case b.eventChan <- envelope:
					logger.WithField("retry_count", envelope.RetryCount).Debug("Event re-queued for retry")
				case <-b.done:
					return
				default:
					logger.Error("Failed to re-queue event: buffer full")
					b.incrementFailed()
				}
			}()
		} else {
			logger.WithField("errors", len(handlerErrors)).Error("Event processing failed after retries")
			b.incrementFailed()
		}
	} else {
		b.incrementProcessed()
		logger.Debug("Event processed successfully")
	}
}

// executeHandler executes a single handler with error recovery
func (b *eventBus) executeHandler(ctx context.Context, handler Handler, event Event) (err error) {
	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panic: %v", r)
			b.logger.WithField("panic", r).Error("Handler panicked")
		}
	}()

	// Execute handler
	return handler.Handle(ctx, event)
}

// errorHandler processes errors from the error channel
func (b *eventBus) errorHandler() {
	defer b.wg.Done()

	for {
		select {
		case err := <-b.errorChan:
			if err != nil {
				b.logger.WithError(err).Error("Event bus error")
			}
		case <-b.done:
			return
		}
	}
}

// Publish publishes an event to the bus
func (b *eventBus) Publish(ctx context.Context, event Event) error {
	if event == nil {
		return errors.New("event cannot be nil")
	}

	b.startedMu.Lock()
	started := b.started
	b.startedMu.Unlock()

	if !started {
		return errors.New("event bus not started")
	}

	envelope := NewEventEnvelope(event)
	envelope.MaxRetries = b.config.MaxRetries

	select {
	case b.eventChan <- envelope:
		b.incrementPublished()
		return nil
	case <-ctx.Done():
		return fmt.Errorf("publish cancelled: %w", ctx.Err())
	case <-b.done:
		return errors.New("event bus is shutting down")
	default:
		return errors.New("event buffer full")
	}
}

// PublishAsync publishes an event asynchronously
func (b *eventBus) PublishAsync(ctx context.Context, event Event) error {
	go func() {
		if err := b.Publish(context.Background(), event); err != nil {
			select {
			case b.errorChan <- err:
			default:
				b.logger.WithError(err).Error("Failed to report async publish error")
			}
		}
	}()
	return nil
}

// Subscribe registers a handler for an event type
func (b *eventBus) Subscribe(eventType string, handler Handler) error {
	if eventType == "" {
		return errors.New("event type cannot be empty")
	}
	if handler == nil {
		return errors.New("handler cannot be nil")
	}

	b.handlersMu.Lock()
	defer b.handlersMu.Unlock()

	b.handlers[eventType] = append(b.handlers[eventType], handler)
	b.logger.WithField("event_type", eventType).Debug("Handler subscribed")

	return nil
}

// Unsubscribe removes a handler for an event type
func (b *eventBus) Unsubscribe(eventType string, handler Handler) error {
	if eventType == "" {
		return errors.New("event type cannot be empty")
	}
	if handler == nil {
		return errors.New("handler cannot be nil")
	}

	b.handlersMu.Lock()
	defer b.handlersMu.Unlock()

	handlers := b.handlers[eventType]
	for i, h := range handlers {
		if h == handler {
			b.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
			b.logger.WithField("event_type", eventType).Debug("Handler unsubscribed")
			return nil
		}
	}

	return fmt.Errorf("handler not found for event type: %s", eventType)
}

// getHandlers returns all handlers for an event type
func (b *eventBus) getHandlers(eventType string) []Handler {
	b.handlersMu.RLock()
	defer b.handlersMu.RUnlock()

	var handlers []Handler

	// Get specific handlers
	if h, ok := b.handlers[eventType]; ok {
		handlers = append(handlers, h...)
	}

	// Get wildcard handlers
	if h, ok := b.handlers["*"]; ok {
		handlers = append(handlers, h...)
	}

	return handlers
}

// AddFilter adds a filter to the event bus
func (b *eventBus) AddFilter(filter Filter) {
	b.filtersMu.Lock()
	defer b.filtersMu.Unlock()

	b.filters = append(b.filters, filter)
}

// RemoveFilter removes a filter from the event bus
func (b *eventBus) RemoveFilter(filter Filter) {
	b.filtersMu.Lock()
	defer b.filtersMu.Unlock()

	for i, f := range b.filters {
		if f == filter {
			b.filters = append(b.filters[:i], b.filters[i+1:]...)
			break
		}
	}
}

// applyFilters applies all filters to an event
func (b *eventBus) applyFilters(event Event) bool {
	b.filtersMu.RLock()
	defer b.filtersMu.RUnlock()

	for _, filter := range b.filters {
		if !filter.Apply(event) {
			return false
		}
	}
	return true
}

// Metrics methods
func (b *eventBus) incrementPublished() {
	b.metrics.mu.Lock()
	b.metrics.eventsPublished++
	b.metrics.mu.Unlock()
}

func (b *eventBus) incrementProcessed() {
	b.metrics.mu.Lock()
	b.metrics.eventsProcessed++
	b.metrics.mu.Unlock()
}

func (b *eventBus) incrementFailed() {
	b.metrics.mu.Lock()
	b.metrics.eventsFailed++
	b.metrics.mu.Unlock()
}

func (b *eventBus) incrementRetried() {
	b.metrics.mu.Lock()
	b.metrics.eventsRetried++
	b.metrics.mu.Unlock()
}

func (b *eventBus) incrementHandlersExecuted() {
	b.metrics.mu.Lock()
	b.metrics.handlersExecuted++
	b.metrics.mu.Unlock()
}

func (b *eventBus) incrementHandlersFailed() {
	b.metrics.mu.Lock()
	b.metrics.handlersFailed++
	b.metrics.mu.Unlock()
}

// GetMetrics returns current metrics
func (b *eventBus) GetMetrics() map[string]uint64 {
	b.metrics.mu.RLock()
	defer b.metrics.mu.RUnlock()

	return map[string]uint64{
		"events_published":  b.metrics.eventsPublished,
		"events_processed":  b.metrics.eventsProcessed,
		"events_failed":     b.metrics.eventsFailed,
		"events_retried":    b.metrics.eventsRetried,
		"handlers_executed": b.metrics.handlersExecuted,
		"handlers_failed":   b.metrics.handlersFailed,
	}
}
