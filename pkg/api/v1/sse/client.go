package sse

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

// Client represents an SSE client connection
type Client struct {
	// Identity
	ID string

	// HTTP connection
	ResponseWriter http.ResponseWriter
	Request        *http.Request

	// Event channel
	Send chan Event

	// Subscriptions and filters
	Topics  map[string]bool
	Filters []Filter

	// SSE state
	LastEventID string
	Connected   time.Time

	// Rate limiting
	limiter *rate.Limiter

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
	closed bool

	// Configuration
	config ClientConfig
	logger logrus.FieldLogger

	// Metrics
	eventsSent    int64
	eventsDropped int64
	lastActivity  time.Time
}

// ClientConfig holds per-client configuration
type ClientConfig struct {
	BufferSize        int           `json:"buffer_size" yaml:"buffer_size"`
	WriteTimeout      time.Duration `json:"write_timeout" yaml:"write_timeout"`
	MaxEventsPerSec   int           `json:"max_events_per_sec" yaml:"max_events_per_sec"`
	BurstSize         int           `json:"burst_size" yaml:"burst_size"`
	EnableCompression bool          `json:"enable_compression" yaml:"enable_compression"`
}

// DefaultClientConfig returns sensible defaults for SSE clients
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		BufferSize:        100,
		WriteTimeout:      10 * time.Second,
		MaxEventsPerSec:   10,
		BurstSize:         20,
		EnableCompression: false,
	}
}

// ClientInfo provides information about a client for status purposes
type ClientInfo struct {
	ID          string    `json:"id"`
	Connected   time.Time `json:"connected"`
	LastEventID string    `json:"last_event_id"`
	Topics      []string  `json:"topics"`
	IsAlive     bool      `json:"is_alive"`
	EventsSent  int64     `json:"events_sent"`
}

// NewClient creates a new SSE client with the given configuration
func NewClient(id string, w http.ResponseWriter, r *http.Request, config ClientConfig, logger logrus.FieldLogger) *Client {
	ctx, cancel := context.WithCancel(r.Context())

	client := &Client{
		ID:             id,
		ResponseWriter: w,
		Request:        r,
		Send:           make(chan Event, config.BufferSize),
		Topics:         make(map[string]bool),
		Filters:        make([]Filter, 0),
		Connected:      time.Now(),
		LastEventID:    r.Header.Get("Last-Event-ID"),
		limiter:        rate.NewLimiter(rate.Limit(config.MaxEventsPerSec), config.BurstSize),
		ctx:            ctx,
		cancel:         cancel,
		config:         config,
		logger:         logger.WithField("client_id", id),
		lastActivity:   time.Now(),
	}

	return client
}

// ServeSSE starts the SSE connection and serves events
func (c *Client) ServeSSE() {
	defer c.Close()

	c.logger.Info("starting SSE connection")

	// Set SSE headers
	c.setupSSEHeaders()

	// Send initial connection confirmation
	if err := c.sendRaw("retry: 5000\n\n"); err != nil {
		c.logger.WithError(err).Error("failed to send initial SSE headers")
		return
	}

	// Send missed events if Last-Event-ID is provided
	if c.LastEventID != "" {
		c.sendMissedEvents()
	}

	// Main event serving loop
	for {
		select {
		case event, ok := <-c.Send:
			if !ok {
				c.logger.Debug("send channel closed, ending SSE connection")
				return
			}

			if err := c.sendEvent(event); err != nil {
				c.logger.WithError(err).Error("failed to send event, closing connection")
				return
			}

		case <-c.ctx.Done():
			c.logger.Debug("context cancelled, ending SSE connection")
			return
		}
	}
}

// SendEvent sends an event to the client (non-blocking)
func (c *Client) SendEvent(event Event) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return ErrClientClosed
	}
	c.mu.RUnlock()

	// Apply filters
	if !c.passesFilters(event) {
		return nil
	}

	// Rate limiting
	if !c.limiter.Allow() {
		c.eventsDropped++
		c.logger.WithField("event_type", event.GetType()).Debug("event dropped due to rate limiting")
		return ErrRateLimited
	}

	// Non-blocking send
	select {
	case c.Send <- event:
		return nil
	default:
		c.eventsDropped++
		c.logger.WithField("event_type", event.GetType()).Warn("event dropped due to full buffer")
		return ErrBufferFull
	}
}

// Close gracefully closes the client connection
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	c.cancel()
	close(c.Send)

	c.logger.WithFields(logrus.Fields{
		"events_sent":    c.eventsSent,
		"events_dropped": c.eventsDropped,
		"duration":       time.Since(c.Connected),
	}).Info("SSE client connection closed")
}

// IsAlive checks if the client connection is still active
func (c *Client) IsAlive() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return false
	}

	// Check if context is done
	select {
	case <-c.ctx.Done():
		return false
	default:
		return true
	}
}

// AddFilter adds an event filter to the client
func (c *Client) AddFilter(filter Filter) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Filters = append(c.Filters, filter)
}

// RemoveFilter removes an event filter from the client
func (c *Client) RemoveFilter(filterIndex int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if filterIndex >= 0 && filterIndex < len(c.Filters) {
		c.Filters = append(c.Filters[:filterIndex], c.Filters[filterIndex+1:]...)
	}
}

// GetInfo returns client information for monitoring
func (c *Client) GetInfo() ClientInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	topics := make([]string, 0, len(c.Topics))
	for topic := range c.Topics {
		topics = append(topics, topic)
	}

	return ClientInfo{
		ID:          c.ID,
		Connected:   c.Connected,
		LastEventID: c.LastEventID,
		Topics:      topics,
		IsAlive:     c.IsAlive(),
		EventsSent:  c.eventsSent,
	}
}

// setupSSEHeaders configures the HTTP response for Server-Sent Events
func (c *Client) setupSSEHeaders() {
	headers := c.ResponseWriter.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	headers.Set("Connection", "keep-alive")
	headers.Set("Access-Control-Allow-Origin", "*")
	headers.Set("Access-Control-Allow-Headers", "Cache-Control")
	headers.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	headers.Set("Access-Control-Expose-Headers", "Content-Type")

	// Enable compression if configured
	if c.config.EnableCompression {
		headers.Set("Content-Encoding", "gzip")
	}

	// Disable buffering for real-time streaming
	if flusher, ok := c.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// sendEvent formats and sends an SSE event
func (c *Client) sendEvent(event Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrClientClosed
	}

	// Build SSE formatted message
	var message string

	// Add event ID if available
	if eventID := event.GetID(); eventID != "" {
		message += fmt.Sprintf("id: %s\n", eventID)
		c.LastEventID = eventID
	}

	// Add event type
	if eventType := event.GetType(); eventType != "" {
		message += fmt.Sprintf("event: %s\n", eventType)
	}

	// Add event data
	data := event.GetData()
	message += fmt.Sprintf("data: %s\n\n", data)

	// Send the formatted message
	if err := c.sendRaw(message); err != nil {
		return fmt.Errorf("failed to send SSE event: %w", err)
	}

	c.eventsSent++
	c.lastActivity = time.Now()

	c.logger.WithFields(logrus.Fields{
		"event_type": event.GetType(),
		"event_id":   event.GetID(),
	}).Debug("sent SSE event")

	return nil
}

// sendRaw sends raw data to the client with timeout
func (c *Client) sendRaw(data string) error {
	// Create a context with timeout for the write operation
	ctx, cancel := context.WithTimeout(c.ctx, c.config.WriteTimeout)
	defer cancel()

	// Create a channel to signal write completion
	done := make(chan error, 1)

	go func() {
		_, err := c.ResponseWriter.Write([]byte(data))
		if err != nil {
			done <- err
			return
		}

		// Flush the data immediately
		if flusher, ok := c.ResponseWriter.(http.Flusher); ok {
			flusher.Flush()
		}

		done <- nil
	}()

	// Wait for write completion or timeout
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("write timeout: %w", ctx.Err())
	}
}

// passesFilters checks if an event passes all configured filters
func (c *Client) passesFilters(event Event) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, filter := range c.Filters {
		if !filter.Matches(event) {
			return false
		}
	}

	return true
}

// sendMissedEvents attempts to send events that the client may have missed
func (c *Client) sendMissedEvents() {
	// This would typically query an event store for events newer than LastEventID
	// For now, we just log that we received a Last-Event-ID
	c.logger.WithField("last_event_id", c.LastEventID).Debug("client reconnecting with Last-Event-ID")

	// Send a reconnection event to inform the client
	reconnectEvent := NewSystemEvent("client.reconnected", map[string]any{
		"client_id":     c.ID,
		"last_event_id": c.LastEventID,
		"timestamp":     time.Now(),
	})

	if err := c.sendEvent(reconnectEvent); err != nil {
		c.logger.WithError(err).Warn("failed to send reconnection event")
	}
}
