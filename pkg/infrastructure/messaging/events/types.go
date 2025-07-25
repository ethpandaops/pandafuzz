package events

import (
	"context"
	"time"
)

// Event is the base interface for all events in the system
type Event interface {
	// Type returns the event type identifier
	Type() string
	// Timestamp returns when the event occurred
	Timestamp() time.Time
	// AggregateID returns the ID of the aggregate this event belongs to
	AggregateID() string
	// Version returns the event version for schema evolution
	Version() int
	// Metadata returns additional event metadata
	Metadata() map[string]string
}

// BaseEvent provides common event functionality
type BaseEvent struct {
	EventType    string            `json:"event_type"`
	EventTime    time.Time         `json:"event_time"`
	EventID      string            `json:"event_id"`
	AggregateID_ string            `json:"aggregate_id"`
	Version_     int               `json:"version"`
	Metadata_    map[string]string `json:"metadata"`
}

// Type returns the event type
func (e *BaseEvent) Type() string {
	return e.EventType
}

// Timestamp returns when the event occurred
func (e *BaseEvent) Timestamp() time.Time {
	return e.EventTime
}

// AggregateID returns the aggregate ID
func (e *BaseEvent) AggregateID() string {
	return e.AggregateID_
}

// Version returns the event version
func (e *BaseEvent) Version() int {
	if e.Version_ == 0 {
		return 1
	}
	return e.Version_
}

// Metadata returns event metadata
func (e *BaseEvent) Metadata() map[string]string {
	if e.Metadata_ == nil {
		return make(map[string]string)
	}
	return e.Metadata_
}

// SetMetadata sets a metadata key-value pair
func (e *BaseEvent) SetMetadata(key, value string) {
	if e.Metadata_ == nil {
		e.Metadata_ = make(map[string]string)
	}
	e.Metadata_[key] = value
}

// Handler defines the interface for event handlers
type Handler interface {
	// Handle processes an event
	Handle(ctx context.Context, event Event) error
	// CanHandle checks if the handler can process the given event type
	CanHandle(eventType string) bool
}

// HandlerFunc is a function that handles events
type HandlerFunc func(ctx context.Context, event Event) error

// HandlerWrapper wraps a function to implement the Handler interface
type HandlerWrapper struct {
	handler   HandlerFunc
	eventType string
}

// NewHandlerWrapper creates a new handler wrapper
func NewHandlerWrapper(eventType string, handler HandlerFunc) Handler {
	return &HandlerWrapper{
		handler:   handler,
		eventType: eventType,
	}
}

// Handle processes the event
func (h *HandlerWrapper) Handle(ctx context.Context, event Event) error {
	return h.handler(ctx, event)
}

// CanHandle checks if this handler can process the event type
func (h *HandlerWrapper) CanHandle(eventType string) bool {
	return h.eventType == eventType || h.eventType == "*"
}

// Publisher defines the interface for publishing events
type Publisher interface {
	// Publish publishes an event
	Publish(ctx context.Context, event Event) error
	// PublishAsync publishes an event asynchronously
	PublishAsync(ctx context.Context, event Event) error
}

// Subscriber defines the interface for subscribing to events
type Subscriber interface {
	// Subscribe registers a handler for an event type
	Subscribe(eventType string, handler Handler) error
	// Unsubscribe removes a handler for an event type
	Unsubscribe(eventType string, handler Handler) error
}

// Bus combines publishing and subscribing capabilities
type Bus interface {
	Publisher
	Subscriber
	// Start starts the event bus
	Start(ctx context.Context) error
	// Stop stops the event bus
	Stop() error
}

// Filter defines an event filter
type Filter interface {
	// Apply checks if an event passes the filter
	Apply(event Event) bool
}

// FilterFunc is a function that filters events
type FilterFunc func(event Event) bool

// Apply implements the Filter interface
func (f FilterFunc) Apply(event Event) bool {
	return f(event)
}

// MetadataFilter filters events based on metadata
type MetadataFilter struct {
	Key   string
	Value string
}

// Apply checks if the event has the required metadata
func (f *MetadataFilter) Apply(event Event) bool {
	if event.Metadata() == nil {
		return false
	}
	val, exists := event.Metadata()[f.Key]
	return exists && val == f.Value
}

// AggregateFilter filters events by aggregate ID
type AggregateFilter struct {
	AggregateID string
}

// Apply checks if the event belongs to the aggregate
func (f *AggregateFilter) Apply(event Event) bool {
	return event.AggregateID() == f.AggregateID
}

// TypeFilter filters events by type
type TypeFilter struct {
	EventTypes []string
}

// Apply checks if the event type matches
func (f *TypeFilter) Apply(event Event) bool {
	for _, t := range f.EventTypes {
		if event.Type() == t {
			return true
		}
	}
	return false
}

// CompositeFilter combines multiple filters with AND logic
type CompositeFilter struct {
	Filters []Filter
}

// Apply checks if all filters pass
func (f *CompositeFilter) Apply(event Event) bool {
	for _, filter := range f.Filters {
		if !filter.Apply(event) {
			return false
		}
	}
	return true
}

// EventEnvelope wraps an event with additional routing information
type EventEnvelope struct {
	Event       Event             `json:"event"`
	PublishedAt time.Time         `json:"published_at"`
	RetryCount  int               `json:"retry_count"`
	MaxRetries  int               `json:"max_retries"`
	Headers     map[string]string `json:"headers"`
}

// NewEventEnvelope creates a new event envelope
func NewEventEnvelope(event Event) *EventEnvelope {
	return &EventEnvelope{
		Event:       event,
		PublishedAt: time.Now().UTC(),
		RetryCount:  0,
		MaxRetries:  3,
		Headers:     make(map[string]string),
	}
}

// ShouldRetry checks if the event should be retried
func (e *EventEnvelope) ShouldRetry() bool {
	return e.RetryCount < e.MaxRetries
}

// IncrementRetry increments the retry counter
func (e *EventEnvelope) IncrementRetry() {
	e.RetryCount++
}

// SetHeader sets an envelope header
func (e *EventEnvelope) SetHeader(key, value string) {
	if e.Headers == nil {
		e.Headers = make(map[string]string)
	}
	e.Headers[key] = value
}

// GetHeader gets an envelope header
func (e *EventEnvelope) GetHeader(key string) (string, bool) {
	val, exists := e.Headers[key]
	return val, exists
}
