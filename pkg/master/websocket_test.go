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

func TestWSHub_Run(t *testing.T) {
	t.Run("hub handles client registration and unregistration", func(t *testing.T) {
		hub := NewWSHub()
		go hub.Run()
		defer hub.Stop()

		// Create test client
		client := &WSClient{
			hub:    hub,
			send:   make(chan []byte, 256),
			topics: make(map[string]bool),
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
		hub := NewWSHub()
		go hub.Run()
		defer hub.Stop()

		// Create test clients
		client1 := &WSClient{
			hub:    hub,
			send:   make(chan []byte, 256),
			topics: make(map[string]bool),
		}
		client2 := &WSClient{
			hub:    hub,
			send:   make(chan []byte, 256),
			topics: make(map[string]bool),
		}

		// Register clients
		hub.register <- client1
		hub.register <- client2
		time.Sleep(10 * time.Millisecond)

		// Broadcast message
		testMessage := WSMessage{
			Type: "test_broadcast",
			Data: map[string]string{"message": "hello"},
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
		hub := NewWSHub()
		go hub.Run()
		defer hub.Stop()

		// Create test clients
		client1 := &WSClient{
			hub:    hub,
			send:   make(chan []byte, 256),
			topics: map[string]bool{"campaign:123": true},
		}
		client2 := &WSClient{
			hub:    hub,
			send:   make(chan []byte, 256),
			topics: map[string]bool{"campaign:456": true},
		}

		// Register clients
		hub.register <- client1
		hub.register <- client2
		time.Sleep(10 * time.Millisecond)

		// Subscribe client1 to topic
		hub.mu.Lock()
		if hub.topics["campaign:123"] == nil {
			hub.topics["campaign:123"] = make(map[*WSClient]bool)
		}
		hub.topics["campaign:123"][client1] = true
		hub.mu.Unlock()

		// Broadcast to specific topic
		testMessage := WSMessage{
			Type: "campaign_update",
			Data: map[string]string{"campaign_id": "123"},
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

func TestWSClient_readPump(t *testing.T) {
	t.Run("client handles incoming messages", func(t *testing.T) {
		hub := NewWSHub()
		go hub.Run()
		defer hub.Stop()

		// Create test WebSocket server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{
				CheckOrigin: func(r *http.Request) bool { return true },
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)

			client := &WSClient{
				hub:    hub,
				conn:   conn,
				send:   make(chan []byte, 256),
				topics: make(map[string]bool),
			}

			// Handle subscription message
			go func() {
				_, message, err := conn.ReadMessage()
				if err == nil {
					var msg WSMessage
					json.Unmarshal(message, &msg)
					if msg.Type == "subscribe" {
						// Process subscription
						if topics, ok := msg.Data.(map[string]interface{})["topics"].([]interface{}); ok {
							for _, topic := range topics {
								client.topics[topic.(string)] = true
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
			Type: "subscribe",
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

		hub := NewWSHub()
		client := &WSClient{
			hub:  hub,
			conn: conn,
			send: make(chan []byte, 256),
		}

		// Start write pump
		go client.writePump()

		// Send message through channel
		testMsg := WSMessage{
			Type: "test_message",
			Data: map[string]string{"test": "data"},
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
		hub := NewWSHub()
		go hub.Run()
		defer hub.Stop()

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

		// Send a ping message
		pingMsg := WSMessage{
			Type: "ping",
			Data: map[string]interface{}{},
		}
		err = conn.WriteJSON(pingMsg)
		assert.NoError(t, err)

		// Should receive pong response
		var pongMsg WSMessage
		err = conn.ReadJSON(&pongMsg)
		assert.NoError(t, err)
		assert.Equal(t, "pong", pongMsg.Type)
	})
}

func TestWSMessage_Marshal(t *testing.T) {
	t.Run("marshal and unmarshal WSMessage", func(t *testing.T) {
		originalMsg := WSMessage{
			Type: "campaign_created",
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
		assert.Equal(t, originalMsg.Data.(map[string]interface{})["id"],
			decodedMsg.Data.(map[string]interface{})["id"])
	})
}

func TestWSHub_handleSubscription(t *testing.T) {
	t.Run("handle topic subscription", func(t *testing.T) {
		hub := NewWSHub()
		go hub.Run()
		defer hub.Stop()

		client := &WSClient{
			hub:    hub,
			send:   make(chan []byte, 256),
			topics: make(map[string]bool),
		}

		// Register client
		hub.register <- client
		time.Sleep(10 * time.Millisecond)

		// Subscribe to topics
		topics := []string{"campaign:123", "crashes:all"}
		hub.handleSubscription(client, topics)

		// Verify subscriptions
		hub.mu.RLock()
		defer hub.mu.RUnlock()

		for _, topic := range topics {
			assert.True(t, client.topics[topic])
			assert.True(t, hub.topics[topic][client])
		}
	})

	t.Run("handle topic unsubscription", func(t *testing.T) {
		hub := NewWSHub()
		go hub.Run()
		defer hub.Stop()

		client := &WSClient{
			hub:    hub,
			send:   make(chan []byte, 256),
			topics: make(map[string]bool),
		}

		// Register client and subscribe to topics
		hub.register <- client
		time.Sleep(10 * time.Millisecond)

		topics := []string{"campaign:123", "crashes:all"}
		hub.handleSubscription(client, topics)

		// Unsubscribe from one topic
		hub.handleUnsubscription(client, []string{"campaign:123"})

		// Verify unsubscription
		hub.mu.RLock()
		defer hub.mu.RUnlock()

		assert.False(t, client.topics["campaign:123"])
		assert.False(t, hub.topics["campaign:123"][client])
		assert.True(t, client.topics["crashes:all"])
		assert.True(t, hub.topics["crashes:all"][client])
	})
}

func TestWSHub_cleanupClient(t *testing.T) {
	t.Run("cleanup client subscriptions on disconnect", func(t *testing.T) {
		hub := NewWSHub()

		client := &WSClient{
			hub:    hub,
			send:   make(chan []byte, 256),
			topics: map[string]bool{"topic1": true, "topic2": true},
		}

		// Set up topics
		hub.mu.Lock()
		hub.clients[client] = true
		hub.topics["topic1"] = map[*WSClient]bool{client: true}
		hub.topics["topic2"] = map[*WSClient]bool{client: true}
		hub.mu.Unlock()

		// Cleanup client
		hub.cleanupClient(client)

		// Verify cleanup
		hub.mu.RLock()
		defer hub.mu.RUnlock()

		_, exists := hub.clients[client]
		assert.False(t, exists)
		assert.False(t, hub.topics["topic1"][client])
		assert.False(t, hub.topics["topic2"][client])
	})
}
