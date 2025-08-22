package sse

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// Manager handles SSE client connections and event broadcasting
type Manager struct {
	// Core channels for client lifecycle
	clients    sync.Map // map[string]*Client
	register   chan *Client
	unregister chan *Client
	broadcast  chan Event
	topics     sync.Map // map[string]map[string]*Client (topic -> clientID -> client)

	// Configuration
	config Config
	logger logrus.FieldLogger

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	metrics *Metrics
	running atomic.Bool
}

// Config holds SSE manager configuration
type Config struct {
	// Client configuration
	MaxClients        int           `json:"max_clients" yaml:"max_clients"`
	ClientTimeout     time.Duration `json:"client_timeout" yaml:"client_timeout"`
	WriteTimeout      time.Duration `json:"write_timeout" yaml:"write_timeout"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval" yaml:"heartbeat_interval"`

	// Buffer configuration
	EventBufferSize     int `json:"event_buffer_size" yaml:"event_buffer_size"`
	ClientBufferSize    int `json:"client_buffer_size" yaml:"client_buffer_size"`
	BroadcastBufferSize int `json:"broadcast_buffer_size" yaml:"broadcast_buffer_size"`

	// Rate limiting
	MaxEventsPerSecond int `json:"max_events_per_second" yaml:"max_events_per_second"`
	BurstSize          int `json:"burst_size" yaml:"burst_size"`

	// Cleanup configuration
	CleanupInterval time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`
	MaxEventHistory int           `json:"max_event_history" yaml:"max_event_history"`
}

// DefaultConfig returns sensible defaults for SSE manager
func DefaultConfig() Config {
	return Config{
		MaxClients:          1000,
		ClientTimeout:       5 * time.Minute,
		WriteTimeout:        10 * time.Second,
		HeartbeatInterval:   30 * time.Second,
		EventBufferSize:     1000,
		ClientBufferSize:    100,
		BroadcastBufferSize: 10000,
		MaxEventsPerSecond:  100,
		BurstSize:           200,
		CleanupInterval:     1 * time.Minute,
		MaxEventHistory:     10000,
	}
}

// NewManager creates a new SSE manager with the given configuration
func NewManager(config Config, logger logrus.FieldLogger) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		register:   make(chan *Client, config.BroadcastBufferSize),
		unregister: make(chan *Client, config.BroadcastBufferSize),
		broadcast:  make(chan Event, config.EventBufferSize),
		config:     config,
		logger:     logger.WithField("component", "sse_manager"),
		ctx:        ctx,
		cancel:     cancel,
		metrics:    NewMetrics(),
	}
}

// Start begins the SSE manager's main event loop
func (m *Manager) Start(ctx context.Context) error {
	if !m.running.CompareAndSwap(false, true) {
		return ErrManagerAlreadyRunning
	}

	m.logger.Info("starting SSE manager")

	// Start main event loop
	m.wg.Add(1)
	go m.run()

	// Start heartbeat sender
	m.wg.Add(1)
	go m.heartbeatLoop()

	// Start cleanup routine
	m.wg.Add(1)
	go m.cleanupLoop()

	m.logger.Info("SSE manager started successfully")
	return nil
}

// Stop gracefully shuts down the SSE manager
func (m *Manager) Stop() error {
	if !m.running.CompareAndSwap(true, false) {
		return ErrManagerNotRunning
	}

	m.logger.Info("stopping SSE manager")

	// Cancel context to signal shutdown
	m.cancel()

	// Close all clients
	m.clients.Range(func(key, value any) bool {
		client := value.(*Client)
		client.Close()
		return true
	})

	// Wait for all goroutines to finish
	m.wg.Wait()

	m.logger.Info("SSE manager stopped successfully")
	return nil
}

// Register adds a new SSE client to the manager
func (m *Manager) Register(client *Client) error {
	if !m.running.Load() {
		return ErrManagerNotRunning
	}

	// Check client limit
	clientCount := m.metrics.GetActiveClients()
	if clientCount >= int64(m.config.MaxClients) {
		return ErrMaxClientsReached
	}

	select {
	case m.register <- client:
		m.logger.WithField("client_id", client.ID).Debug("client registration queued")
		return nil
	case <-m.ctx.Done():
		return ErrManagerStopped
	default:
		return ErrRegistrationQueueFull
	}
}

// Unregister removes an SSE client from the manager
func (m *Manager) Unregister(client *Client) error {
	if !m.running.Load() {
		return nil // Ignore unregistration when not running
	}

	select {
	case m.unregister <- client:
		m.logger.WithField("client_id", client.ID).Debug("client unregistration queued")
		return nil
	case <-m.ctx.Done():
		return ErrManagerStopped
	default:
		return ErrUnregistrationQueueFull
	}
}

// Broadcast sends an event to all connected clients
func (m *Manager) Broadcast(event Event) error {
	if !m.running.Load() {
		return ErrManagerNotRunning
	}

	select {
	case m.broadcast <- event:
		m.metrics.IncrementEventsSent()
		return nil
	case <-m.ctx.Done():
		return ErrManagerStopped
	default:
		return ErrBroadcastQueueFull
	}
}

// BroadcastToTopic sends an event to all clients subscribed to a specific topic
func (m *Manager) BroadcastToTopic(topic string, event Event) error {
	if !m.running.Load() {
		return ErrManagerNotRunning
	}

	topicClients, exists := m.topics.Load(topic)
	if !exists {
		m.logger.WithField("topic", topic).Debug("no clients subscribed to topic")
		return nil
	}

	clients := topicClients.(map[string]*Client)
	sentCount := 0

	for clientID, client := range clients {
		if client.IsAlive() {
			if err := client.SendEvent(event); err != nil {
				m.logger.WithFields(logrus.Fields{
					"client_id": clientID,
					"topic":     topic,
					"error":     err,
				}).Warn("failed to send event to client")
			} else {
				sentCount++
			}
		}
	}

	m.logger.WithFields(logrus.Fields{
		"topic":      topic,
		"sent_count": sentCount,
		"event_type": event.GetType(),
	}).Debug("broadcasted event to topic")

	m.metrics.IncrementEventsSent()
	return nil
}

// Subscribe adds a client to a topic subscription
func (m *Manager) Subscribe(clientID string, topic string) error {
	client, exists := m.clients.Load(clientID)
	if !exists {
		return ErrClientNotFound
	}

	// Add client to topic
	topicClients, _ := m.topics.LoadOrStore(topic, make(map[string]*Client))
	clients := topicClients.(map[string]*Client)
	clients[clientID] = client.(*Client)

	// Update client's topic subscriptions
	clientObj := client.(*Client)
	clientObj.mu.Lock()
	clientObj.Topics[topic] = true
	clientObj.mu.Unlock()

	m.logger.WithFields(logrus.Fields{
		"client_id": clientID,
		"topic":     topic,
	}).Debug("client subscribed to topic")

	return nil
}

// Unsubscribe removes a client from a topic subscription
func (m *Manager) Unsubscribe(clientID string, topic string) error {
	// Remove client from topic
	topicClients, exists := m.topics.Load(topic)
	if exists {
		clients := topicClients.(map[string]*Client)
		delete(clients, clientID)

		// Remove topic if no clients remain
		if len(clients) == 0 {
			m.topics.Delete(topic)
		}
	}

	// Update client's topic subscriptions
	client, exists := m.clients.Load(clientID)
	if exists {
		clientObj := client.(*Client)
		clientObj.mu.Lock()
		delete(clientObj.Topics, topic)
		clientObj.mu.Unlock()
	}

	m.logger.WithFields(logrus.Fields{
		"client_id": clientID,
		"topic":     topic,
	}).Debug("client unsubscribed from topic")

	return nil
}

// GetClients returns information about all connected clients
func (m *Manager) GetClients() []ClientInfo {
	var clients []ClientInfo

	m.clients.Range(func(key, value any) bool {
		client := value.(*Client)
		clients = append(clients, ClientInfo{
			ID:          client.ID,
			Connected:   client.Connected,
			LastEventID: client.LastEventID,
			Topics:      getTopicsList(client.Topics),
			IsAlive:     client.IsAlive(),
		})
		return true
	})

	return clients
}

// GetMetrics returns current metrics
func (m *Manager) GetMetrics() *Metrics {
	return m.metrics
}

// run is the main event loop for the SSE manager
func (m *Manager) run() {
	defer m.wg.Done()

	for {
		select {
		case client := <-m.register:
			m.handleRegister(client)

		case client := <-m.unregister:
			m.handleUnregister(client)

		case event := <-m.broadcast:
			m.handleBroadcast(event)

		case <-m.ctx.Done():
			m.logger.Debug("SSE manager event loop stopped")
			return
		}
	}
}

// handleRegister processes client registration
func (m *Manager) handleRegister(client *Client) {
	m.clients.Store(client.ID, client)
	m.metrics.IncrementActiveClients()

	m.logger.WithFields(logrus.Fields{
		"client_id":      client.ID,
		"active_clients": m.metrics.GetActiveClients(),
	}).Info("client registered")

	// Send welcome event
	welcomeEvent := NewSystemEvent("client.connected", map[string]any{
		"client_id": client.ID,
		"timestamp": time.Now(),
	})

	if err := client.SendEvent(welcomeEvent); err != nil {
		m.logger.WithFields(logrus.Fields{
			"client_id": client.ID,
			"error":     err,
		}).Warn("failed to send welcome event to new client")
	}
}

// handleUnregister processes client unregistration
func (m *Manager) handleUnregister(client *Client) {
	// Remove from clients
	if _, exists := m.clients.LoadAndDelete(client.ID); exists {
		m.metrics.DecrementActiveClients()
	}

	// Remove from all topics
	m.topics.Range(func(topicKey, topicValue any) bool {
		topic := topicKey.(string)
		clients := topicValue.(map[string]*Client)
		delete(clients, client.ID)

		// Remove topic if no clients remain
		if len(clients) == 0 {
			m.topics.Delete(topic)
		}
		return true
	})

	// Close client
	client.Close()

	m.logger.WithFields(logrus.Fields{
		"client_id":      client.ID,
		"active_clients": m.metrics.GetActiveClients(),
	}).Info("client unregistered")
}

// handleBroadcast processes event broadcasting
func (m *Manager) handleBroadcast(event Event) {
	sentCount := 0

	m.clients.Range(func(key, value any) bool {
		client := value.(*Client)
		if client.IsAlive() {
			if err := client.SendEvent(event); err != nil {
				m.logger.WithFields(logrus.Fields{
					"client_id":  client.ID,
					"event_type": event.GetType(),
					"error":      err,
				}).Warn("failed to send event to client")
			} else {
				sentCount++
			}
		}
		return true
	})

	m.logger.WithFields(logrus.Fields{
		"event_type": event.GetType(),
		"sent_count": sentCount,
	}).Debug("broadcasted event to all clients")
}

// heartbeatLoop sends periodic heartbeat events to all clients
func (m *Manager) heartbeatLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			heartbeatEvent := NewSystemEvent("heartbeat", map[string]any{
				"timestamp": time.Now(),
			})

			if err := m.Broadcast(heartbeatEvent); err != nil {
				m.logger.WithError(err).Warn("failed to send heartbeat")
			}

		case <-m.ctx.Done():
			return
		}
	}
}

// cleanupLoop periodically cleans up disconnected clients
func (m *Manager) cleanupLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.performCleanup()

		case <-m.ctx.Done():
			return
		}
	}
}

// performCleanup removes disconnected clients
func (m *Manager) performCleanup() {
	var deadClients []*Client

	m.clients.Range(func(key, value any) bool {
		client := value.(*Client)
		if !client.IsAlive() {
			deadClients = append(deadClients, client)
		}
		return true
	})

	for _, client := range deadClients {
		if err := m.Unregister(client); err != nil {
			m.logger.WithFields(logrus.Fields{
				"client_id": client.ID,
				"error":     err,
			}).Warn("failed to unregister dead client")
		}
	}

	if len(deadClients) > 0 {
		m.logger.WithField("cleaned_clients", len(deadClients)).Debug("cleaned up disconnected clients")
	}
}

// getTopicsList converts a topic map to a slice
func getTopicsList(topics map[string]bool) []string {
	var result []string
	for topic := range topics {
		result = append(result, topic)
	}
	return result
}
