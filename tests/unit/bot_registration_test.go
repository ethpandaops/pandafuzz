package unit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pandafuzz/pkg/bot"
	"pandafuzz/pkg/common"
)

// TestBotRegistration tests the bot registration workflow
func TestBotRegistration(t *testing.T) {
	tests := []struct {
		name           string
		setupServer    func() *httptest.Server
		expectedError  bool
		errorContains  string
		validateResult func(t *testing.T, result *common.BotRegistrationResponse)
	}{
		{
			name: "successful registration",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/api/v1/bots/register", r.URL.Path)
					assert.Equal(t, "POST", r.Method)
					assert.Contains(t, r.Header.Get("User-Agent"), "PandaFuzz-Bot")

					var req common.BotRegistrationRequest
					err := json.NewDecoder(r.Body).Decode(&req)
					require.NoError(t, err)
					assert.NotEmpty(t, req.Hostname)
					assert.Contains(t, req.Capabilities, "afl++")

					resp := common.BotRegistrationResponse{
						BotID:     "test-bot-123",
						Status:    "registered",
						Timestamp: time.Now(),
						Timeout:   time.Now().Add(time.Hour),
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(resp)
				}))
			},
			expectedError: false,
			validateResult: func(t *testing.T, result *common.BotRegistrationResponse) {
				assert.Equal(t, "test-bot-123", result.BotID)
				assert.Equal(t, "registered", result.Status)
				assert.NotZero(t, result.Timestamp)
				assert.NotZero(t, result.Timeout)
			},
		},
		{
			name: "server returns error",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{
						"error": "internal server error",
					})
				}))
			},
			expectedError: true,
			errorContains: "registration failed",
		},
		{
			name: "invalid response format",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte("invalid json"))
				}))
			},
			expectedError: true,
			errorContains: "failed to decode",
		},
		{
			name: "network error",
			setupServer: func() *httptest.Server {
				// Create server and immediately close it to simulate network error
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
				server.Close()
				return server
			},
			expectedError: true,
			errorContains: "connection refused",
		},
		{
			name: "timeout",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Simulate timeout by sleeping longer than client timeout
					time.Sleep(100 * time.Millisecond)
				}))
			},
			expectedError: true,
			errorContains: "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			if server != nil && tt.name != "network error" {
				defer server.Close()
			}

			// Create bot config
			cfg := &common.BotConfig{
				ID:         "test-bot",
				Name:       "test-bot",
				MasterURL:  server.URL,
				Capabilities: []string{"afl++", "libfuzzer"},
				Timeouts: common.BotTimeouts{
					MasterCommunication: 50 * time.Millisecond, // Short timeout for tests
				},
				Retry: common.RetryConfig{
					MaxRetries:    1, // Minimal retries for tests
					InitialDelay:  time.Millisecond,
					MaxDelay:      10 * time.Millisecond,
					Multiplier:    2.0,
				},
			}

			// Create client
			client := bot.NewRetryClient(cfg)

			// Perform registration
			ctx := context.Background()
			result, err := client.RegisterBot(ctx, cfg.Capabilities)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				if tt.validateResult != nil {
					tt.validateResult(t, result)
				}
			}
		})
	}
}

// TestBotHeartbeat tests the heartbeat mechanism
func TestBotHeartbeat(t *testing.T) {
	botID := "test-bot-123"
	heartbeatCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := fmt.Sprintf("/api/v1/bots/%s/heartbeat", botID)
		assert.Equal(t, expectedPath, r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var req common.HeartbeatRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.NotEmpty(t, req.Status)
		assert.NotNil(t, req.ResourceUsage)

		heartbeatCount++
		
		resp := common.HeartbeatResponse{
			Status:    "acknowledged",
			Timestamp: time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &common.BotConfig{
		ID:        botID,
		MasterURL: server.URL,
		Timeouts: common.BotTimeouts{
			MasterCommunication: time.Second,
			HeartbeatInterval:   50 * time.Millisecond, // Fast heartbeat for tests
		},
	}

	client := bot.NewRetryClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Send heartbeat
	status := &common.BotStatus{
		Status: "idle",
		ResourceUsage: &common.ResourceUsage{
			CPUPercent: 10.5,
			MemoryUsed: 104857600,
			DiskUsed:   209715200,
		},
	}

	err := client.SendHeartbeat(ctx, botID, status)
	require.NoError(t, err)

	// Verify heartbeat was sent
	assert.Equal(t, 1, heartbeatCount)
}

// TestBotReconnection tests bot reconnection after network failure
func TestBotReconnection(t *testing.T) {
	connectionAttempts := 0
	isHealthy := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectionAttempts++

		if !isHealthy && connectionAttempts < 3 {
			// Simulate network failure for first 2 attempts
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		// After 3rd attempt, server becomes healthy
		isHealthy = true

		if r.URL.Path == "/api/v1/bots/register" {
			resp := common.BotRegistrationResponse{
				BotID:     "reconnect-bot",
				Status:    "registered",
				Timestamp: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	cfg := &common.BotConfig{
		ID:           "test-bot",
		MasterURL:    server.URL,
		Capabilities: []string{"afl++"},
		Retry: common.RetryConfig{
			MaxRetries:   5,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		},
		Timeouts: common.BotTimeouts{
			MasterCommunication: time.Second,
		},
	}

	client := bot.NewRetryClient(cfg)
	ctx := context.Background()

	// Try to register - should succeed after retries
	result, err := client.RegisterBot(ctx, cfg.Capabilities)
	require.NoError(t, err)
	assert.Equal(t, "reconnect-bot", result.BotID)
	assert.GreaterOrEqual(t, connectionAttempts, 3)
}

// TestConcurrentBotRegistrations tests multiple bots registering simultaneously
func TestConcurrentBotRegistrations(t *testing.T) {
	registrationCount := 0
	registrationChan := make(chan string, 10)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/bots/register" {
			var req common.BotRegistrationRequest
			json.NewDecoder(r.Body).Decode(&req)

			registrationCount++
			botID := fmt.Sprintf("bot-%d", registrationCount)
			registrationChan <- botID

			resp := common.BotRegistrationResponse{
				BotID:     botID,
				Status:    "registered",
				Timestamp: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	// Register 5 bots concurrently
	numBots := 5
	errors := make(chan error, numBots)
	results := make(chan string, numBots)

	for i := 0; i < numBots; i++ {
		go func(index int) {
			cfg := &common.BotConfig{
				ID:           fmt.Sprintf("bot-%d", index),
				MasterURL:    server.URL,
				Capabilities: []string{"afl++"},
				Timeouts: common.BotTimeouts{
					MasterCommunication: time.Second,
				},
			}

			client := bot.NewRetryClient(cfg)
			result, err := client.RegisterBot(context.Background(), cfg.Capabilities)
			if err != nil {
				errors <- err
				return
			}
			results <- result.BotID
		}(i)
	}

	// Wait for all registrations
	timeout := time.After(2 * time.Second)
	registered := make([]string, 0, numBots)

	for i := 0; i < numBots; i++ {
		select {
		case err := <-errors:
			t.Fatalf("Registration failed: %v", err)
		case botID := <-results:
			registered = append(registered, botID)
		case <-timeout:
			t.Fatal("Timeout waiting for registrations")
		}
	}

	assert.Equal(t, numBots, len(registered))
	assert.Equal(t, numBots, registrationCount)
}

// TestBotRegistrationValidation tests input validation for bot registration
func TestBotRegistrationValidation(t *testing.T) {
	tests := []struct {
		name          string
		capabilities  []string
		expectedError string
	}{
		{
			name:          "empty capabilities",
			capabilities:  []string{},
			expectedError: "no capabilities",
		},
		{
			name:          "nil capabilities",
			capabilities:  nil,
			expectedError: "no capabilities",
		},
		{
			name:         "valid capabilities",
			capabilities: []string{"afl++", "libfuzzer"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req common.BotRegistrationRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Server-side validation
		if len(req.Capabilities) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "no capabilities provided",
			})
			return
		}

		resp := common.BotRegistrationResponse{
			BotID:  "valid-bot",
			Status: "registered",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &common.BotConfig{
				MasterURL: server.URL,
				Timeouts: common.BotTimeouts{
					MasterCommunication: time.Second,
				},
			}

			client := bot.NewRetryClient(cfg)
			result, err := client.RegisterBot(context.Background(), tt.capabilities)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

// BenchmarkBotRegistration benchmarks the registration process
func BenchmarkBotRegistration(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := common.BotRegistrationResponse{
			BotID:     "bench-bot",
			Status:    "registered",
			Timestamp: time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &common.BotConfig{
		ID:           "bench-bot",
		MasterURL:    server.URL,
		Capabilities: []string{"afl++"},
		Timeouts: common.BotTimeouts{
			MasterCommunication: time.Second,
		},
	}

	client := bot.NewRetryClient(cfg)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.RegisterBot(ctx, cfg.Capabilities)
		if err != nil {
			b.Fatal(err)
		}
	}
}