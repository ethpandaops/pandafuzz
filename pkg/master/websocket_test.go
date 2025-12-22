package master

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func newTestLogger() logrus.FieldLogger {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	return logger
}

func TestWSHub_Run(t *testing.T) {
	t.Run("hub handles client registration and unregistration", func(t *testing.T) {
		logger := newTestLogger()
		hub := NewWSHub(logger)
		go hub.Run()
		// Note: WSHub.Run runs forever, we use timeout in tests

		// Create test client
		client := &WSClient{
			ID:            "test-client-1",
			hub:           hub,
			send:          make(chan []byte, 256),
			subscriptions: make(map[string]bool),
			logger:        logger,
		}

		// Register client
		hub.register <- client
		time.Sleep(10 * time.Millisecond)

		// Verify client is registered
		hub.mu.RLock()
		_, exists := hub.clients[client]
		hub.mu.RUnlock()
		assert.True(t, exists)

		// Unregister client
		hub.unregister <- client
		time.Sleep(10 * time.Millisecond)

		// Verify client is unregistered
		hub.mu.RLock()
		_, exists = hub.clients[client]
		hub.mu.RUnlock()
		assert.False(t, exists)
	})

	t.Run("hub broadcasts messages to all clients", func(t *testing.T) {
		logger := newTestLogger()
		hub := NewWSHub(logger)
		go hub.Run()

		// Create test clients
		client1 := &WSClient{
			ID:            "test-client-1",
			hub:           hub,
			send:          make(chan []byte, 256),
			subscriptions: make(map[string]bool),
			logger:        logger,
		}
		client2 := &WSClient{
			ID:            "test-client-2",
			hub:           hub,
			send:          make(chan []byte, 256),
			subscriptions: make(map[string]bool),
			logger:        logger,
		}

		// Register clients
		hub.register <- client1
		hub.register <- client2
		time.Sleep(10 * time.Millisecond)

		// Consume the welcome messages sent on registration
		<-client1.send
		<-client2.send

		// Broadcast message
		testMessage := WSMessage{
			Type: "test_broadcast",
			Data: map[string]interface{}{"message": "hello"},
		}
		hub.Broadcast(testMessage)

		// Verify both clients receive the message
		select {
		case msg := <-client1.send:
			var received WSMessage
			err := json.Unmarshal(msg, &received)
			assert.NoError(t, err)
			assert.Equal(t, testMessage.Type, received.Type)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Client 1 did not receive broadcast message")
		}

		select {
		case msg := <-client2.send:
			var received WSMessage
			err := json.Unmarshal(msg, &received)
			assert.NoError(t, err)
			assert.Equal(t, testMessage.Type, received.Type)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Client 2 did not receive broadcast message")
		}
	})

	t.Run("hub broadcasts to topic subscribers only", func(t *testing.T) {
		logger := newTestLogger()
		hub := NewWSHub(logger)
		go hub.Run()

		// Create test clients
		client1 := &WSClient{
			ID:            "test-client-1",
			hub:           hub,
			send:          make(chan []byte, 256),
			subscriptions: map[string]bool{"campaign:123": true},
			logger:        logger,
		}
		client2 := &WSClient{
			ID:            "test-client-2",
			hub:           hub,
			send:          make(chan []byte, 256),
			subscriptions: map[string]bool{"campaign:456": true},
			logger:        logger,
		}

		// Register clients
		hub.register <- client1
		hub.register <- client2
		time.Sleep(10 * time.Millisecond)

		// Consume the welcome messages sent on registration
		<-client1.send
		<-client2.send

		// Subscribe client1 to topic using client method
		client1.subscribeToTopic("campaign:123")

		// Broadcast to specific topic
		testMessage := WSMessage{
			Type: "campaign_update",
			Data: map[string]interface{}{"campaign_id": "123"},
		}
		hub.BroadcastToTopic("campaign:123", testMessage)

		// Only client1 should receive the message
		select {
		case msg := <-client1.send:
			var received WSMessage
			err := json.Unmarshal(msg, &received)
			assert.NoError(t, err)
			assert.Equal(t, testMessage.Type, received.Type)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Client 1 did not receive topic message")
		}

		// Client2 should not receive the message
		select {
		case <-client2.send:
			t.Fatal("Client 2 should not have received the message")
		case <-time.After(50 * time.Millisecond):
			// Expected - no message received
		}
	})
}

func TestWSHub_ClientCount(t *testing.T) {
	t.Run("client count reflects registered clients", func(t *testing.T) {
		logger := newTestLogger()
		hub := NewWSHub(logger)
		go hub.Run()

		// Initially no clients
		assert.Equal(t, 0, hub.ClientCount())

		// Create and register clients
		client1 := &WSClient{
			ID:            "test-client-1",
			hub:           hub,
			send:          make(chan []byte, 256),
			subscriptions: make(map[string]bool),
			logger:        logger,
		}
		client2 := &WSClient{
			ID:            "test-client-2",
			hub:           hub,
			send:          make(chan []byte, 256),
			subscriptions: make(map[string]bool),
			logger:        logger,
		}

		hub.register <- client1
		time.Sleep(10 * time.Millisecond)
		assert.Equal(t, 1, hub.ClientCount())

		hub.register <- client2
		time.Sleep(10 * time.Millisecond)
		assert.Equal(t, 2, hub.ClientCount())

		hub.unregister <- client1
		time.Sleep(10 * time.Millisecond)
		assert.Equal(t, 1, hub.ClientCount())
	})
}

func TestWSClient_readPump(t *testing.T) {
	t.Run("client handles incoming messages", func(t *testing.T) {
		logger := newTestLogger()
		hub := NewWSHub(logger)
		go hub.Run()

		// Create test WebSocket server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{
				CheckOrigin: func(r *http.Request) bool { return true },
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)

			client := &WSClient{
				ID:            "server-client",
				hub:           hub,
				conn:          conn,
				send:          make(chan []byte, 256),
				subscriptions: make(map[string]bool),
				logger:        logger,
			}

			// Handle subscription message
			go func() {
				_, message, err := conn.ReadMessage()
				if err == nil {
					var msg WSMessage
					json.Unmarshal(message, &msg)
					if msg.Type == WSTypeSubscribe {
						// Process subscription
						if data, ok := msg.Data["topics"].([]interface{}); ok {
							for _, topic := range data {
								if topicStr, ok := topic.(string); ok {
									client.subscriptions[topicStr] = true
								}
							}
						}
					}
				}
			}()

			// Keep connection open
			time.Sleep(100 * time.Millisecond)
			conn.Close()
		}))
		defer server.Close()

		// Connect to WebSocket
		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		assert.NoError(t, err)
		defer conn.Close()

		// Send subscription message
		subMsg := WSMessage{
			Type: WSTypeSubscribe,
			Data: map[string]interface{}{
				"topics": []string{"campaign:123", "campaign:456"},
			},
		}
		err = conn.WriteJSON(subMsg)
		assert.NoError(t, err)

		time.Sleep(50 * time.Millisecond)
	})
}

func TestWSClient_writePump(t *testing.T) {
	t.Run("client sends messages from send channel", func(t *testing.T) {
		logger := newTestLogger()

		// Create test WebSocket server that echoes messages
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{
				CheckOrigin: func(r *http.Request) bool { return true },
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)
			defer conn.Close()

			// Read and echo message
			_, message, err := conn.ReadMessage()
			assert.NoError(t, err)

			var msg WSMessage
			err = json.Unmarshal(message, &msg)
			assert.NoError(t, err)
			assert.Equal(t, "test_message", msg.Type)
		}))
		defer server.Close()

		// Connect to WebSocket
		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		assert.NoError(t, err)
		defer conn.Close()

		hub := NewWSHub(logger)
		client := &WSClient{
			ID:     "test-client",
			hub:    hub,
			conn:   conn,
			send:   make(chan []byte, 256),
			logger: logger,
		}

		// Start write pump
		go client.writePump()

		// Send message through channel
		testMsg := WSMessage{
			Type: "test_message",
			Data: map[string]interface{}{"test": "data"},
		}
		msgBytes, _ := json.Marshal(testMsg)
		client.send <- msgBytes

		time.Sleep(50 * time.Millisecond)
	})
}

func TestServer_handleWebSocket(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("upgrade connection and handle client", func(t *testing.T) {
		hub := NewWSHub(logger)
		go hub.Run()

		server := &Server{
			logger: logger,
			wsHub:  hub,
		}

		// Create test server
		ts := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
		defer ts.Close()

		// Connect WebSocket client
		wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
		conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
		defer conn.Close()

		// Verify we receive welcome message
		var welcomeMsg WSMessage
		err = conn.ReadJSON(&welcomeMsg)
		assert.NoError(t, err)
		assert.Equal(t, "welcome", welcomeMsg.Type)
	})
}

func TestWSMessage_Marshal(t *testing.T) {
	t.Run("marshal and unmarshal WSMessage", func(t *testing.T) {
		originalMsg := WSMessage{
			Type: WSTypeCampaignCreated,
			Data: map[string]interface{}{
				"id":     "campaign123",
				"name":   "Test Campaign",
				"status": "running",
			},
			Timestamp: time.Now(),
		}

		// Marshal to JSON
		data, err := json.Marshal(originalMsg)
		assert.NoError(t, err)

		// Unmarshal back
		var decodedMsg WSMessage
		err = json.Unmarshal(data, &decodedMsg)
		assert.NoError(t, err)

		assert.Equal(t, originalMsg.Type, decodedMsg.Type)
		originalData := originalMsg.Data
		decodedData := decodedMsg.Data
		assert.Equal(t, originalData["id"], decodedData["id"])
	})
}

func TestWSClient_subscribeToTopic(t *testing.T) {
	t.Run("client subscribes to topic", func(t *testing.T) {
		logger := newTestLogger()
		hub := NewWSHub(logger)
		go hub.Run()

		client := &WSClient{
			ID:            "test-client",
			hub:           hub,
			send:          make(chan []byte, 256),
			subscriptions: make(map[string]bool),
			logger:        logger,
		}

		// Register client first
		hub.register <- client
		time.Sleep(10 * time.Millisecond)

		// Subscribe to topics
		topics := []string{"campaign:123", "crashes:all"}
		for _, topic := range topics {
			client.subscribeToTopic(topic)
		}

		// Verify subscriptions
		hub.mu.RLock()
		defer hub.mu.RUnlock()

		for _, topic := range topics {
			assert.True(t, hub.topics[topic][client], "Client should be subscribed to topic %s", topic)
		}
	})

	t.Run("client unsubscribes from topic", func(t *testing.T) {
		logger := newTestLogger()
		hub := NewWSHub(logger)
		go hub.Run()

		client := &WSClient{
			ID:            "test-client",
			hub:           hub,
			send:          make(chan []byte, 256),
			subscriptions: make(map[string]bool),
			logger:        logger,
		}

		// Register client and subscribe to topics
		hub.register <- client
		time.Sleep(10 * time.Millisecond)

		topics := []string{"campaign:123", "crashes:all"}
		for _, topic := range topics {
			client.subscribeToTopic(topic)
		}

		// Unsubscribe from one topic
		client.unsubscribeFromTopic("campaign:123")

		// Verify unsubscription
		hub.mu.RLock()
		defer hub.mu.RUnlock()

		assert.False(t, hub.topics["campaign:123"][client], "Client should not be subscribed to campaign:123")
		assert.True(t, hub.topics["crashes:all"][client], "Client should still be subscribed to crashes:all")
	})
}

func TestWSHub_BroadcastToTopic_NoSubscribers(t *testing.T) {
	t.Run("broadcast to topic with no subscribers does not panic", func(t *testing.T) {
		logger := newTestLogger()
		hub := NewWSHub(logger)
		go hub.Run()

		// Should not panic when broadcasting to topic with no subscribers
		msg := WSMessage{
			Type:      "test",
			Data:      map[string]interface{}{"key": "value"},
			Timestamp: time.Now(),
		}
		hub.BroadcastToTopic("nonexistent:topic", msg)
	})
}
