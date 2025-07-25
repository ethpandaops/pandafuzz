package handlers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/messaging/events"
)

// Registry manages event handler registration and routing
type Registry struct {
	handlers      map[string][]HandlerInfo
	mu            sync.RWMutex
	logger        logrus.FieldLogger
	interceptors  []HandlerInterceptor
	interceptorMu sync.RWMutex
}

// HandlerInfo contains information about a registered handler
type HandlerInfo struct {
	ID           string
	EventType    string
	Handler      events.Handler
	Priority     int
	Description  string
	RegisteredAt time.Time
	Metadata     map[string]string
}

// HandlerInterceptor allows intercepting handler execution
type HandlerInterceptor interface {
	// Before is called before handler execution
	Before(ctx context.Context, event events.Event, handler events.Handler) error
	// After is called after handler execution
	After(ctx context.Context, event events.Event, handler events.Handler, err error) error
}

// NewRegistry creates a new handler registry
func NewRegistry(logger logrus.FieldLogger) *Registry {
	if logger == nil {
		logger = logrus.New().WithField("component", "handler-registry")
	}

	return &Registry{
		handlers:     make(map[string][]HandlerInfo),
		logger:       logger,
		interceptors: make([]HandlerInterceptor, 0),
	}
}

// Register registers a new handler
func (r *Registry) Register(eventType string, handler events.Handler, opts ...HandlerOption) (string, error) {
	if eventType == "" {
		return "", errors.New("event type cannot be empty")
	}
	if handler == nil {
		return "", errors.New("handler cannot be nil")
	}

	// Apply options
	info := HandlerInfo{
		ID:           generateHandlerID(),
		EventType:    eventType,
		Handler:      handler,
		Priority:     0,
		RegisteredAt: time.Now().UTC(),
		Metadata:     make(map[string]string),
	}

	for _, opt := range opts {
		opt(&info)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Add handler to registry
	r.handlers[eventType] = append(r.handlers[eventType], info)

	// Sort by priority
	r.sortHandlers(eventType)

	r.logger.WithFields(logrus.Fields{
		"handler_id": info.ID,
		"event_type": eventType,
		"priority":   info.Priority,
	}).Debug("Handler registered")

	return info.ID, nil
}

// Unregister removes a handler by ID
func (r *Registry) Unregister(handlerID string) error {
	if handlerID == "" {
		return errors.New("handler ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for eventType, handlers := range r.handlers {
		for i, info := range handlers {
			if info.ID == handlerID {
				r.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
				r.logger.WithFields(logrus.Fields{
					"handler_id": handlerID,
					"event_type": eventType,
				}).Debug("Handler unregistered")
				return nil
			}
		}
	}

	return fmt.Errorf("handler not found: %s", handlerID)
}

// UnregisterByType removes all handlers for an event type
func (r *Registry) UnregisterByType(eventType string) error {
	if eventType == "" {
		return errors.New("event type cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if handlers, exists := r.handlers[eventType]; exists {
		delete(r.handlers, eventType)
		r.logger.WithFields(logrus.Fields{
			"event_type": eventType,
			"count":      len(handlers),
		}).Debug("All handlers unregistered for event type")
		return nil
	}

	return fmt.Errorf("no handlers found for event type: %s", eventType)
}

// GetHandlers returns all handlers for an event type
func (r *Registry) GetHandlers(eventType string) []HandlerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []HandlerInfo

	// Get specific handlers
	if handlers, ok := r.handlers[eventType]; ok {
		result = append(result, handlers...)
	}

	// Get wildcard handlers
	if handlers, ok := r.handlers["*"]; ok {
		result = append(result, handlers...)
	}

	return result
}

// GetHandlerInfo returns information about a specific handler
func (r *Registry) GetHandlerInfo(handlerID string) (*HandlerInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, handlers := range r.handlers {
		for _, info := range handlers {
			if info.ID == handlerID {
				return &info, nil
			}
		}
	}

	return nil, fmt.Errorf("handler not found: %s", handlerID)
}

// ListHandlers returns all registered handlers
func (r *Registry) ListHandlers() map[string][]HandlerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]HandlerInfo)
	for eventType, handlers := range r.handlers {
		result[eventType] = append([]HandlerInfo{}, handlers...)
	}

	return result
}

// CreateBusHandler creates a composite handler for use with the event bus
func (r *Registry) CreateBusHandler(eventType string) events.Handler {
	return &registryHandler{
		registry:  r,
		eventType: eventType,
	}
}

// AddInterceptor adds a handler interceptor
func (r *Registry) AddInterceptor(interceptor HandlerInterceptor) {
	r.interceptorMu.Lock()
	defer r.interceptorMu.Unlock()

	r.interceptors = append(r.interceptors, interceptor)
}

// RemoveInterceptor removes a handler interceptor
func (r *Registry) RemoveInterceptor(interceptor HandlerInterceptor) {
	r.interceptorMu.Lock()
	defer r.interceptorMu.Unlock()

	for i, h := range r.interceptors {
		if h == interceptor {
			r.interceptors = append(r.interceptors[:i], r.interceptors[i+1:]...)
			break
		}
	}
}

// sortHandlers sorts handlers by priority (higher priority first)
func (r *Registry) sortHandlers(eventType string) {
	handlers := r.handlers[eventType]
	for i := 0; i < len(handlers)-1; i++ {
		for j := i + 1; j < len(handlers); j++ {
			if handlers[i].Priority < handlers[j].Priority {
				handlers[i], handlers[j] = handlers[j], handlers[i]
			}
		}
	}
}

// registryHandler implements events.Handler for the registry
type registryHandler struct {
	registry  *Registry
	eventType string
}

// Handle processes an event by calling all registered handlers
func (h *registryHandler) Handle(ctx context.Context, event events.Event) error {
	handlers := h.registry.GetHandlers(h.eventType)
	if len(handlers) == 0 {
		return nil
	}

	var errors []error
	for _, info := range handlers {
		// Apply interceptors before
		if err := h.registry.applyBeforeInterceptors(ctx, event, info.Handler); err != nil {
			h.registry.logger.WithError(err).WithField("handler_id", info.ID).Error("Before interceptor failed")
			errors = append(errors, err)
			continue
		}

		// Execute handler
		err := info.Handler.Handle(ctx, event)

		// Apply interceptors after
		if interceptErr := h.registry.applyAfterInterceptors(ctx, event, info.Handler, err); interceptErr != nil {
			h.registry.logger.WithError(interceptErr).WithField("handler_id", info.ID).Error("After interceptor failed")
		}

		if err != nil {
			h.registry.logger.WithError(err).WithFields(logrus.Fields{
				"handler_id": info.ID,
				"event_type": event.Type(),
			}).Error("Handler failed")
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("handler errors: %d failed", len(errors))
	}

	return nil
}

// CanHandle checks if the handler can process the event type
func (h *registryHandler) CanHandle(eventType string) bool {
	return h.eventType == eventType || h.eventType == "*"
}

// applyBeforeInterceptors applies all before interceptors
func (r *Registry) applyBeforeInterceptors(ctx context.Context, event events.Event, handler events.Handler) error {
	r.interceptorMu.RLock()
	defer r.interceptorMu.RUnlock()

	for _, interceptor := range r.interceptors {
		if err := interceptor.Before(ctx, event, handler); err != nil {
			return err
		}
	}
	return nil
}

// applyAfterInterceptors applies all after interceptors
func (r *Registry) applyAfterInterceptors(ctx context.Context, event events.Event, handler events.Handler, handlerErr error) error {
	r.interceptorMu.RLock()
	defer r.interceptorMu.RUnlock()

	for _, interceptor := range r.interceptors {
		if err := interceptor.After(ctx, event, handler, handlerErr); err != nil {
			return err
		}
	}
	return nil
}

// HandlerOption is a function that configures a HandlerInfo
type HandlerOption func(*HandlerInfo)

// WithPriority sets the handler priority
func WithPriority(priority int) HandlerOption {
	return func(info *HandlerInfo) {
		info.Priority = priority
	}
}

// WithDescription sets the handler description
func WithDescription(description string) HandlerOption {
	return func(info *HandlerInfo) {
		info.Description = description
	}
}

// WithMetadata sets handler metadata
func WithMetadata(key, value string) HandlerOption {
	return func(info *HandlerInfo) {
		if info.Metadata == nil {
			info.Metadata = make(map[string]string)
		}
		info.Metadata[key] = value
	}
}

// generateHandlerID generates a unique handler ID
func generateHandlerID() string {
	return fmt.Sprintf("handler_%d", time.Now().UnixNano())
}

// LoggingInterceptor logs handler execution
type LoggingInterceptor struct {
	logger logrus.FieldLogger
}

// NewLoggingInterceptor creates a new logging interceptor
func NewLoggingInterceptor(logger logrus.FieldLogger) HandlerInterceptor {
	if logger == nil {
		logger = logrus.New().WithField("component", "handler-interceptor")
	}
	return &LoggingInterceptor{logger: logger}
}

// Before logs before handler execution
func (i *LoggingInterceptor) Before(ctx context.Context, event events.Event, handler events.Handler) error {
	i.logger.WithFields(logrus.Fields{
		"event_type":   event.Type(),
		"aggregate_id": event.AggregateID(),
	}).Debug("Executing handler")
	return nil
}

// After logs after handler execution
func (i *LoggingInterceptor) After(ctx context.Context, event events.Event, handler events.Handler, err error) error {
	logger := i.logger.WithFields(logrus.Fields{
		"event_type":   event.Type(),
		"aggregate_id": event.AggregateID(),
	})

	if err != nil {
		logger.WithError(err).Error("Handler execution failed")
	} else {
		logger.Debug("Handler execution completed")
	}

	return nil
}

// MetricsInterceptor collects handler execution metrics
type MetricsInterceptor struct {
	mu           sync.Mutex
	executions   map[string]uint64
	failures     map[string]uint64
	durations    map[string]time.Duration
	lastExecuted map[string]time.Time
}

// NewMetricsInterceptor creates a new metrics interceptor
func NewMetricsInterceptor() *MetricsInterceptor {
	return &MetricsInterceptor{
		executions:   make(map[string]uint64),
		failures:     make(map[string]uint64),
		durations:    make(map[string]time.Duration),
		lastExecuted: make(map[string]time.Time),
	}
}

// Before records execution start
func (i *MetricsInterceptor) Before(ctx context.Context, event events.Event, handler events.Handler) error {
	return nil
}

// After records execution completion
func (i *MetricsInterceptor) After(ctx context.Context, event events.Event, handler events.Handler, err error) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	eventType := event.Type()
	i.executions[eventType]++
	i.lastExecuted[eventType] = time.Now().UTC()

	if err != nil {
		i.failures[eventType]++
	}

	return nil
}

// GetMetrics returns current metrics
func (i *MetricsInterceptor) GetMetrics() map[string]interface{} {
	i.mu.Lock()
	defer i.mu.Unlock()

	metrics := make(map[string]interface{})
	metrics["executions"] = i.executions
	metrics["failures"] = i.failures
	metrics["durations"] = i.durations
	metrics["last_executed"] = i.lastExecuted

	return metrics
}
