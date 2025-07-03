package master

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// WebSocket message types
const (
	WSTypeCampaignCreated   = "campaign_created"
	WSTypeCampaignUpdated   = "campaign_updated"
	WSTypeCampaignCompleted = "campaign_completed"
	WSTypeCrashFound        = "crash_found"
	WSTypeCorpusUpdate      = "corpus_update"
	WSTypeBotStatus         = "bot_status"
	WSTypeJobProgress       = "job_progress"
	WSTypeSystemAlert       = "system_alert"
	WSTypePing              = "ping"
	WSTypePong              = "pong"
	WSTypeSubscribe         = "subscribe"
	WSTypeUnsubscribe       = "unsubscribe"
)

// WSMessage represents a WebSocket message
type WSMessage struct {
	Type      string                 `json:"type"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// WSClient represents a WebSocket client connection
type WSClient struct {
	ID            string
	hub           *WSHub
	conn          *websocket.Conn
	send          chan []byte
	subscriptions map[string]bool // Subscribed topics
	mu            sync.RWMutex
	logger        logrus.FieldLogger
	lastPing      time.Time
}

// WSHub manages WebSocket connections
type WSHub struct {
	// Registered clients
	clients map[*WSClient]bool

	// Inbound messages from clients
	broadcast chan WSMessage

	// Register requests from clients
	register chan *WSClient

	// Unregister requests from clients
	unregister chan *WSClient

	// Topic-based subscriptions
	topics map[string]map[*WSClient]bool

	// Mutex for thread safety
	mu sync.RWMutex

	logger logrus.FieldLogger
}

// WebSocket configuration
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from any origin
		// In production, implement proper origin checking
		return true
	},
}

// NewWSHub creates a new WebSocket hub
func NewWSHub(logger logrus.FieldLogger) *WSHub {
	return &WSHub{
		broadcast:  make(chan WSMessage, 256),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
		clients:    make(map[*WSClient]bool),
		topics:     make(map[string]map[*WSClient]bool),
		logger:     logger.WithField("component", "websocket_hub"),
	}
}

// Run starts the WebSocket hub
func (h *WSHub) Run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

			h.logger.WithField("client_id", client.ID).Info("Client connected")

			// Send welcome message
			welcome := WSMessage{
				Type: "welcome",
				Data: map[string]interface{}{
					"client_id": client.ID,
					"version":   "2.0",
				},
				Timestamp: time.Now(),
			}
			client.sendMessage(welcome)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)

				// Remove from all topic subscriptions
				for topic, subscribers := range h.topics {
					delete(subscribers, client)
					if len(subscribers) == 0 {
						delete(h.topics, topic)
					}
				}
			}
			h.mu.Unlock()

			h.logger.WithField("client_id", client.ID).Info("Client disconnected")

		case message := <-h.broadcast:
			h.broadcastToClients(message)

		case <-ticker.C:
			// Send ping to all clients
			h.pingClients()
		}
	}
}

// Broadcast sends a message to all connected clients
func (h *WSHub) Broadcast(message WSMessage) {
	select {
	case h.broadcast <- message:
	default:
		h.logger.Warn("Broadcast channel full, dropping message")
	}
}

// BroadcastToTopic sends a message to clients subscribed to a topic
func (h *WSHub) BroadcastToTopic(topic string, message WSMessage) {
	h.mu.RLock()
	subscribers, exists := h.topics[topic]
	h.mu.RUnlock()

	if !exists || len(subscribers) == 0 {
		return
	}

	data, err := json.Marshal(message)
	if err != nil {
		h.logger.WithError(err).Error("Failed to marshal message")
		return
	}

	h.mu.RLock()
	for client := range subscribers {
		select {
		case client.send <- data:
		default:
			// Client's send channel is full, skip
			h.logger.WithField("client_id", client.ID).Warn("Client send buffer full")
		}
	}
	h.mu.RUnlock()
}

// ClientCount returns the number of connected clients
func (h *WSHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// broadcastToClients sends a message to all clients
func (h *WSHub) broadcastToClients(message WSMessage) {
	data, err := json.Marshal(message)
	if err != nil {
		h.logger.WithError(err).Error("Failed to marshal broadcast message")
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		select {
		case client.send <- data:
		default:
			// Client's send channel is full, close it
			h.logger.WithField("client_id", client.ID).Warn("Closing slow client")
			close(client.send)
			delete(h.clients, client)
		}
	}
}

// pingClients sends ping messages to all clients
func (h *WSHub) pingClients() {
	ping := WSMessage{
		Type:      WSTypePing,
		Data:      map[string]interface{}{},
		Timestamp: time.Now(),
	}

	h.broadcastToClients(ping)
}

// handleWebSocket handles WebSocket upgrade requests
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.WithError(err).Error("Failed to upgrade connection")
		return
	}

	// Create new client
	clientID := generateClientID()
	client := &WSClient{
		ID:            clientID,
		hub:           s.wsHub,
		conn:          conn,
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]bool),
		logger:        s.logger.WithField("client_id", clientID),
		lastPing:      time.Now(),
	}

	// Register client
	client.hub.register <- client

	// Start client goroutines
	go client.writePump()
	go client.readPump()
}

// readPump handles incoming messages from the WebSocket connection
func (c *WSClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	// Configure connection
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		c.lastPing = time.Now()
		return nil
	})

	// Maximum message size
	c.conn.SetReadLimit(512 * 1024) // 512KB

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.WithError(err).Error("WebSocket read error")
			}
			break
		}

		// Parse message
		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			c.logger.WithError(err).Error("Failed to parse message")
			continue
		}

		// Handle message based on type
		c.handleMessage(msg)
	}
}

// writePump handles outgoing messages to the WebSocket connection
func (c *WSClient) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// Channel closed
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Write message
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

			// Send queued messages in same write
			n := len(c.send)
			for i := 0; i < n; i++ {
				if err := c.conn.WriteMessage(websocket.TextMessage, <-c.send); err != nil {
					return
				}
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes incoming client messages
func (c *WSClient) handleMessage(msg WSMessage) {
	switch msg.Type {
	case WSTypePong:
		// Pong received, update last ping time
		c.lastPing = time.Now()

	case WSTypeSubscribe:
		// Subscribe to topics
		if topics, ok := msg.Data["topics"].([]interface{}); ok {
			c.mu.Lock()
			for _, topic := range topics {
				if topicStr, ok := topic.(string); ok {
					c.subscriptions[topicStr] = true
					c.subscribeToTopic(topicStr)
				}
			}
			c.mu.Unlock()
		}

	case WSTypeUnsubscribe:
		// Unsubscribe from topics
		if topics, ok := msg.Data["topics"].([]interface{}); ok {
			c.mu.Lock()
			for _, topic := range topics {
				if topicStr, ok := topic.(string); ok {
					delete(c.subscriptions, topicStr)
					c.unsubscribeFromTopic(topicStr)
				}
			}
			c.mu.Unlock()
		}

	default:
		c.logger.WithField("type", msg.Type).Debug("Received unknown message type")
	}
}

// sendMessage sends a message to the client
func (c *WSClient) sendMessage(msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		c.logger.WithError(err).Error("Failed to marshal message")
		return
	}

	select {
	case c.send <- data:
	default:
		c.logger.Warn("Send buffer full, dropping message")
	}
}

// subscribeToTopic subscribes the client to a topic
func (c *WSClient) subscribeToTopic(topic string) {
	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()

	if c.hub.topics[topic] == nil {
		c.hub.topics[topic] = make(map[*WSClient]bool)
	}
	c.hub.topics[topic][c] = true

	c.logger.WithField("topic", topic).Debug("Subscribed to topic")
}

// unsubscribeFromTopic unsubscribes the client from a topic
func (c *WSClient) unsubscribeFromTopic(topic string) {
	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()

	if subscribers, ok := c.hub.topics[topic]; ok {
		delete(subscribers, c)
		if len(subscribers) == 0 {
			delete(c.hub.topics, topic)
		}
	}

	c.logger.WithField("topic", topic).Debug("Unsubscribed from topic")
}

// WebSocket broadcast helpers for specific events

// broadcastCampaignUpdate broadcasts campaign update to all clients
func (s *Server) broadcastCampaignUpdate(campaignID string, update interface{}) {
	msg := WSMessage{
		Type: WSTypeCampaignUpdated,
		Data: map[string]interface{}{
			"campaign_id": campaignID,
			"update":      update,
		},
		Timestamp: time.Now(),
	}

	// Broadcast to all clients and campaign-specific topic
	s.wsHub.Broadcast(msg)
	s.wsHub.BroadcastToTopic("campaign:"+campaignID, msg)
}

// broadcastCrashFound broadcasts new crash discovery
func (s *Server) broadcastCrashFound(crash *CrashNotification) {
	msg := WSMessage{
		Type: WSTypeCrashFound,
		Data: map[string]interface{}{
			"crash_id":    crash.CrashID,
			"campaign_id": crash.CampaignID,
			"job_id":      crash.JobID,
			"severity":    crash.Severity,
			"is_unique":   crash.IsUnique,
		},
		Timestamp: time.Now(),
	}

	// Broadcast to all clients and campaign-specific topic
	s.wsHub.Broadcast(msg)
	s.wsHub.BroadcastToTopic("campaign:"+crash.CampaignID, msg)
}

// broadcastCorpusUpdate broadcasts corpus evolution update
func (s *Server) broadcastCorpusUpdate(campaignID string, evolution *CorpusEvolutionUpdate) {
	msg := WSMessage{
		Type: WSTypeCorpusUpdate,
		Data: map[string]interface{}{
			"campaign_id":    campaignID,
			"total_files":    evolution.TotalFiles,
			"total_coverage": evolution.TotalCoverage,
			"new_files":      evolution.NewFiles,
			"new_coverage":   evolution.NewCoverage,
		},
		Timestamp: time.Now(),
	}

	// Broadcast to campaign-specific topic
	s.wsHub.BroadcastToTopic("campaign:"+campaignID, msg)
}

// broadcastJobProgress broadcasts job progress update
func (s *Server) broadcastJobProgress(jobID string, progress float64, status string) {
	msg := WSMessage{
		Type: WSTypeJobProgress,
		Data: map[string]interface{}{
			"job_id":   jobID,
			"progress": progress,
			"status":   status,
		},
		Timestamp: time.Now(),
	}

	// Broadcast to job-specific topic
	s.wsHub.BroadcastToTopic("job:"+jobID, msg)
}

// broadcastSystemAlert broadcasts system-wide alerts
func (s *Server) broadcastSystemAlert(level, message string, details map[string]interface{}) {
	msg := WSMessage{
		Type: WSTypeSystemAlert,
		Data: map[string]interface{}{
			"level":   level,
			"message": message,
			"details": details,
		},
		Timestamp: time.Now(),
	}

	// Broadcast to all clients
	s.wsHub.Broadcast(msg)
}

// Notification structures

// CrashNotification represents a crash discovery notification
type CrashNotification struct {
	CrashID    string
	CampaignID string
	JobID      string
	Severity   string
	IsUnique   bool
}

// CorpusEvolutionUpdate represents corpus growth notification
type CorpusEvolutionUpdate struct {
	TotalFiles    int
	TotalCoverage int64
	NewFiles      int
	NewCoverage   int64
}

// generateClientID generates a unique client ID
func generateClientID() string {
	return "ws-" + generateID()
}

// generateID generates a unique ID (simplified version)
func generateID() string {
	// In production, use a proper UUID library
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Unix()%1000)
}
