package pandafuzz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/clients/go/generated"
)

func TestNewSimpleClient(t *testing.T) {
	client, err := NewSimpleClient("http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	if client.baseURL != "http://localhost:8080" {
		t.Errorf("Expected baseURL to be http://localhost:8080, got %s", client.baseURL)
	}
}

func TestNewSimpleClientWithOptions(t *testing.T) {
	client, err := NewSimpleClient(
		"http://localhost:8080",
		WithSimpleAPIKey("test-key"),
		WithSimpleHTTPClient(&http.Client{Timeout: 60 * time.Second}),
	)
	if err != nil {
		t.Fatalf("Failed to create client with options: %v", err)
	}
	defer client.Close()

	if client.httpClient.Timeout != 60*time.Second {
		t.Errorf("Expected timeout to be 60s, got %v", client.httpClient.Timeout)
	}
}

func TestNewSimpleClientEmptyURL(t *testing.T) {
	_, err := NewSimpleClient("")
	if err == nil {
		t.Error("Expected error for empty URL, got nil")
	}
}

func TestSimpleClientHealth(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("Expected path /health, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "healthy"}`))
	}))
	defer server.Close()

	client, err := NewSimpleClient(server.URL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Health(ctx)
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestSimpleClientListBots(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bots" {
			t.Errorf("Expected path /bots, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": [], "pagination": {"total": 0}}`))
	}))
	defer server.Close()

	client, err := NewSimpleClient(server.URL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.ListBots(ctx, nil)
	if err != nil {
		t.Fatalf("List bots failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestSimpleClientCreateBot(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bots" {
			t.Errorf("Expected path /bots, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": "123", "name": "test-bot"}`))
	}))
	defer server.Close()

	client, err := NewSimpleClient(server.URL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	createReq := generated.BotCreateRequest{
		ApiEndpoint:  "http://localhost:9090",
		Hostname:     "test-bot",
		Capabilities: []generated.BotCreateRequestCapabilities{},
	}

	resp, err := client.CreateBot(ctx, createReq)
	if err != nil {
		t.Fatalf("Create bot failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", resp.StatusCode)
	}
}

func TestSimpleClientContextCancellation(t *testing.T) {
	// Create a mock server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewSimpleClient(server.URL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = client.Health(ctx)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}

func TestSimpleClientGetClient(t *testing.T) {
	client, err := NewSimpleClient("http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	generatedClient := client.GetClient()
	if generatedClient == nil {
		t.Error("Expected generated client, got nil")
	}
}
