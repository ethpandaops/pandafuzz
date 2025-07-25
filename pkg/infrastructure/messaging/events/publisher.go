package events

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// PublisherConfig contains configuration for the event publisher
type PublisherConfig struct {
	// BufferSize is the size of the event buffer
	BufferSize int
	// Workers is the number of worker goroutines
	Workers int
	// PublishTimeout is the timeout for publishing events
	PublishTimeout time.Duration
	// RetryDelay is the delay between retry attempts
	RetryDelay time.Duration
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int
	// Logger is the logger instance
	Logger logrus.FieldLogger
}

// DefaultPublisherConfig returns default publisher configuration
func DefaultPublisherConfig() PublisherConfig {
	return PublisherConfig{
		BufferSize:     1000,
		Workers:        10,
		PublishTimeout: 5 * time.Second,
		RetryDelay:     100 * time.Millisecond,
		MaxRetries:     3,
		Logger:         logrus.New().WithField("component", "event-publisher"),
	}
}

// publisher implements the Publisher interface
type publisher struct {
	config    PublisherConfig
	eventChan chan *EventEnvelope
	bus       Bus
	wg        sync.WaitGroup
	done      chan struct{}
	logger    logrus.FieldLogger
}

// NewPublisher creates a new event publisher
func NewPublisher(config PublisherConfig, bus Bus) Publisher {
	if config.BufferSize <= 0 {
		config.BufferSize = 1000
	}
	if config.Workers <= 0 {
		config.Workers = 10
	}
	if config.PublishTimeout <= 0 {
		config.PublishTimeout = 5 * time.Second
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = 100 * time.Millisecond
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.Logger == nil {
		config.Logger = logrus.New().WithField("component", "event-publisher")
	}

	p := &publisher{
		config:    config,
		eventChan: make(chan *EventEnvelope, config.BufferSize),
		bus:       bus,
		done:      make(chan struct{}),
		logger:    config.Logger,
	}

	// Start workers
	p.start()

	return p
}

// start starts the publisher workers
func (p *publisher) start() {
	for i := 0; i < p.config.Workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// worker processes events from the channel
func (p *publisher) worker(id int) {
	defer p.wg.Done()

	logger := p.logger.WithField("worker_id", id)
	logger.Debug("Starting event publisher worker")

	for {
		select {
		case envelope := <-p.eventChan:
			if envelope == nil {
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), p.config.PublishTimeout)
			err := p.publishWithRetry(ctx, envelope)
			cancel()

			if err != nil {
				logger.WithError(err).WithFields(logrus.Fields{
					"event_type":   envelope.Event.Type(),
					"aggregate_id": envelope.Event.AggregateID(),
					"retry_count":  envelope.RetryCount,
				}).Error("Failed to publish event after retries")
			}

		case <-p.done:
			logger.Debug("Stopping event publisher worker")
			return
		}
	}
}

// publishWithRetry publishes an event with retry logic
func (p *publisher) publishWithRetry(ctx context.Context, envelope *EventEnvelope) error {
	var lastErr error

	for envelope.RetryCount <= envelope.MaxRetries {
		select {
		case <-ctx.Done():
			return fmt.Errorf("publish timeout: %w", ctx.Err())
		default:
		}

		// Attempt to publish
		err := p.bus.Publish(ctx, envelope.Event)
		if err == nil {
			return nil
		}

		lastErr = err
		envelope.IncrementRetry()

		if envelope.ShouldRetry() {
			p.logger.WithError(err).WithFields(logrus.Fields{
				"event_type":   envelope.Event.Type(),
				"aggregate_id": envelope.Event.AggregateID(),
				"retry_count":  envelope.RetryCount,
				"max_retries":  envelope.MaxRetries,
			}).Warn("Event publish failed, retrying")

			// Wait before retry with exponential backoff
			backoff := p.config.RetryDelay * time.Duration(1<<uint(envelope.RetryCount-1))
			select {
			case <-time.After(backoff):
				continue
			case <-ctx.Done():
				return fmt.Errorf("publish cancelled during retry: %w", ctx.Err())
			}
		}
	}

	return fmt.Errorf("failed after %d retries: %w", envelope.RetryCount, lastErr)
}

// Publish publishes an event synchronously
func (p *publisher) Publish(ctx context.Context, event Event) error {
	if event == nil {
		return errors.New("event cannot be nil")
	}

	envelope := NewEventEnvelope(event)

	// Publish directly with timeout
	return p.publishWithRetry(ctx, envelope)
}

// PublishAsync publishes an event asynchronously
func (p *publisher) PublishAsync(ctx context.Context, event Event) error {
	if event == nil {
		return errors.New("event cannot be nil")
	}

	envelope := NewEventEnvelope(event)

	select {
	case p.eventChan <- envelope:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("failed to queue event: %w", ctx.Err())
	default:
		return errors.New("event buffer full")
	}
}

// Stop stops the publisher
func (p *publisher) Stop() error {
	close(p.done)

	// Wait for workers to finish processing
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	// Wait with timeout
	select {
	case <-done:
		close(p.eventChan)
		return nil
	case <-time.After(30 * time.Second):
		return errors.New("timeout waiting for publisher workers to stop")
	}
}

// DirectPublisher implements a simple synchronous publisher
type DirectPublisher struct {
	handlers map[string][]Handler
	mu       sync.RWMutex
	logger   logrus.FieldLogger
}

// NewDirectPublisher creates a new direct publisher
func NewDirectPublisher(logger logrus.FieldLogger) Publisher {
	if logger == nil {
		logger = logrus.New().WithField("component", "direct-publisher")
	}

	return &DirectPublisher{
		handlers: make(map[string][]Handler),
		logger:   logger,
	}
}

// Publish publishes an event to all registered handlers
func (p *DirectPublisher) Publish(ctx context.Context, event Event) error {
	if event == nil {
		return errors.New("event cannot be nil")
	}

	p.mu.RLock()
	handlers := p.handlers[event.Type()]
	// Also get wildcard handlers
	wildcardHandlers := p.handlers["*"]
	p.mu.RUnlock()

	allHandlers := append(handlers, wildcardHandlers...)
	if len(allHandlers) == 0 {
		p.logger.WithField("event_type", event.Type()).Debug("No handlers registered for event type")
		return nil
	}

	var errs []error
	for _, handler := range allHandlers {
		if err := handler.Handle(ctx, event); err != nil {
			p.logger.WithError(err).WithFields(logrus.Fields{
				"event_type":   event.Type(),
				"aggregate_id": event.AggregateID(),
			}).Error("Handler failed to process event")
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to handle event: %d errors occurred", len(errs))
	}

	return nil
}

// PublishAsync publishes an event asynchronously
func (p *DirectPublisher) PublishAsync(ctx context.Context, event Event) error {
	// For direct publisher, async is the same as sync in a goroutine
	go func() {
		if err := p.Publish(context.Background(), event); err != nil {
			p.logger.WithError(err).WithFields(logrus.Fields{
				"event_type":   event.Type(),
				"aggregate_id": event.AggregateID(),
			}).Error("Async publish failed")
		}
	}()
	return nil
}

// AddHandler adds a handler for an event type
func (p *DirectPublisher) AddHandler(eventType string, handler Handler) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.handlers[eventType] = append(p.handlers[eventType], handler)
}

// RemoveHandler removes a handler for an event type
func (p *DirectPublisher) RemoveHandler(eventType string, handler Handler) {
	p.mu.Lock()
	defer p.mu.Unlock()

	handlers := p.handlers[eventType]
	for i, h := range handlers {
		if h == handler {
			p.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
			break
		}
	}
}
