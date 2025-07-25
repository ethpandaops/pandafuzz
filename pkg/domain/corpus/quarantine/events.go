package quarantine

import (
	"context"
	"sync"
)

// EventHandler defines the interface for handling quarantine events
type EventHandler func(event QuarantineEvent)

// SimpleEventPublisher provides a basic in-memory event publisher implementation
type SimpleEventPublisher struct {
	mu       sync.RWMutex
	handlers map[QuarantineEventType][]EventHandler
}

// NewSimpleEventPublisher creates a new simple event publisher
func NewSimpleEventPublisher() *SimpleEventPublisher {
	return &SimpleEventPublisher{
		handlers: make(map[QuarantineEventType][]EventHandler),
	}
}

// PublishEvent publishes a quarantine event to all registered handlers
func (p *SimpleEventPublisher) PublishEvent(event QuarantineEvent) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Get handlers for this event type
	handlers, exists := p.handlers[event.Type]
	if !exists {
		return nil // No handlers registered
	}

	// Call each handler asynchronously
	for _, handler := range handlers {
		go handler(event)
	}

	return nil
}

// Subscribe registers a handler for specific event types
func (p *SimpleEventPublisher) Subscribe(eventType QuarantineEventType, handler EventHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.handlers[eventType] == nil {
		p.handlers[eventType] = make([]EventHandler, 0)
	}
	p.handlers[eventType] = append(p.handlers[eventType], handler)
}

// SubscribeAll registers a handler for all event types
func (p *SimpleEventPublisher) SubscribeAll(handler EventHandler) {
	eventTypes := []QuarantineEventType{
		EventEntryQuarantined,
		EventEntryReleased,
		EventEntryReviewed,
		EventQuarantineFailed,
	}

	for _, eventType := range eventTypes {
		p.Subscribe(eventType, handler)
	}
}

// ChannelEventPublisher publishes events to a channel
type ChannelEventPublisher struct {
	eventChan chan QuarantineEvent
	ctx       context.Context
}

// NewChannelEventPublisher creates a new channel-based event publisher
func NewChannelEventPublisher(ctx context.Context, bufferSize int) *ChannelEventPublisher {
	return &ChannelEventPublisher{
		eventChan: make(chan QuarantineEvent, bufferSize),
		ctx:       ctx,
	}
}

// PublishEvent publishes an event to the channel
func (p *ChannelEventPublisher) PublishEvent(event QuarantineEvent) error {
	select {
	case p.eventChan <- event:
		return nil
	case <-p.ctx.Done():
		return p.ctx.Err()
	default:
		// Channel is full, drop the event
		// In production, you might want to handle this differently
		return nil
	}
}

// Events returns the event channel for consumers
func (p *ChannelEventPublisher) Events() <-chan QuarantineEvent {
	return p.eventChan
}

// NullEventPublisher is a no-op event publisher for testing
type NullEventPublisher struct{}

// PublishEvent does nothing
func (n *NullEventPublisher) PublishEvent(event QuarantineEvent) error {
	return nil
}
