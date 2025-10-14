package pandafuzz

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// SSEEvent represents a Server-Sent Event
type SSEEvent struct {
	ID    string                 `json:"id,omitempty"`
	Type  string                 `json:"type"`
	Data  map[string]interface{} `json:"data"`
	Retry *int                   `json:"retry,omitempty"`
}

// SSEConfig configures the SSE client
type SSEConfig struct {
	APIKey      string
	BearerToken string
	EventTypes  []string
	Logger      logrus.FieldLogger
	HTTPClient  *http.Client
	BufferSize  int
	RetryDelay  time.Duration
	MaxRetries  int
}

// SSEClient handles Server-Sent Events streaming from PandaFuzz
type SSEClient struct {
	baseURL     string
	config      SSEConfig
	events      chan SSEEvent
	errors      chan error
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	mu          sync.RWMutex
	connected   bool
	lastEventID string
}

// NewSSEClient creates a new SSE client for real-time event streaming
func NewSSEClient(baseURL string, config SSEConfig) (*SSEClient, error) {
	if config.Logger == nil {
		config.Logger = logrus.WithField("component", "sse-client")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 0} // No timeout for SSE
	}
	if config.BufferSize == 0 {
		config.BufferSize = 100
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 5 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 10
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &SSEClient{
		baseURL: baseURL,
		config:  config,
		events:  make(chan SSEEvent, config.BufferSize),
		errors:  make(chan error, 10),
		ctx:     ctx,
		cancel:  cancel,
	}

	return client, nil
}

// Connect starts the SSE connection and begins streaming events
func (c *SSEClient) Connect() error {
	c.wg.Add(1)
	go c.streamEvents()
	return nil
}

// Events returns a channel of SSE events
func (c *SSEClient) Events() <-chan SSEEvent {
	return c.events
}

// Errors returns a channel of connection errors
func (c *SSEClient) Errors() <-chan error {
	return c.errors
}

// Close closes the SSE connection and releases resources
func (c *SSEClient) Close() error {
	c.cancel()
	c.wg.Wait()
	close(c.events)
	close(c.errors)
	return nil
}

// IsConnected returns true if the client is currently connected
func (c *SSEClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// streamEvents handles the main SSE streaming loop with reconnection logic
func (c *SSEClient) streamEvents() {
	defer c.wg.Done()

	retryCount := 0
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		if retryCount > 0 {
			c.config.Logger.WithField("retry_count", retryCount).Info("Reconnecting to SSE stream")
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(c.config.RetryDelay):
			}
		}

		if err := c.connectAndStream(); err != nil {
			c.config.Logger.WithError(err).Error("SSE stream connection failed")

			select {
			case c.errors <- err:
			default:
				// Error channel is full, skip
			}

			retryCount++
			if retryCount >= c.config.MaxRetries {
				c.config.Logger.Error("Max retries exceeded, stopping SSE client")
				return
			}
			continue
		}

		// Reset retry count on successful connection
		retryCount = 0
	}
}

// connectAndStream establishes SSE connection and processes events
func (c *SSEClient) connectAndStream() error {
	// Build the events endpoint URL
	eventsURL := fmt.Sprintf("%s/api/v1/events/stream", strings.TrimSuffix(c.baseURL, "/"))

	// Create the request
	req, err := http.NewRequestWithContext(c.ctx, "GET", eventsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create SSE request: %w", err)
	}

	// Add authentication
	if c.config.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.BearerToken)
	} else if c.config.APIKey != "" {
		req.Header.Set("X-API-Key", c.config.APIKey)
	}

	// SSE headers
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	// Add Last-Event-ID if we have one
	if c.lastEventID != "" {
		req.Header.Set("Last-Event-ID", c.lastEventID)
	}

	// Add event type filters
	if len(c.config.EventTypes) > 0 {
		q := req.URL.Query()
		for _, eventType := range c.config.EventTypes {
			q.Add("event_types", eventType)
		}
		req.URL.RawQuery = q.Encode()
	}

	// Make the request
	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("SSE request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SSE connection failed with status: %d", resp.StatusCode)
	}

	c.setConnected(true)
	defer c.setConnected(false)

	c.config.Logger.Info("SSE connection established")

	// Process the event stream
	scanner := bufio.NewScanner(resp.Body)
	event := SSEEvent{}

	for scanner.Scan() {
		line := scanner.Text()

		// Check for context cancellation
		select {
		case <-c.ctx.Done():
			return nil
		default:
		}

		// Parse SSE format
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if err := json.Unmarshal([]byte(data), &event.Data); err != nil {
				c.config.Logger.WithError(err).Warn("Failed to parse event data")
				continue
			}
		} else if strings.HasPrefix(line, "event: ") {
			event.Type = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "id: ") {
			event.ID = strings.TrimPrefix(line, "id: ")
			c.lastEventID = event.ID
		} else if strings.HasPrefix(line, "retry: ") {
			// Parse retry time
			if retryStr := strings.TrimPrefix(line, "retry: "); retryStr != "" {
				// Implementation could parse retry time here
			}
		} else if line == "" {
			// Empty line signals end of event
			if event.Type != "" {
				select {
				case c.events <- event:
				case <-c.ctx.Done():
					return nil
				default:
					// Event channel is full, skip this event
					c.config.Logger.Warn("Event channel full, dropping event")
				}
			}
			// Reset for next event
			event = SSEEvent{}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("SSE stream error: %w", err)
	}

	return nil
}

// setConnected updates the connection status thread-safely
func (c *SSEClient) setConnected(connected bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = connected
}
