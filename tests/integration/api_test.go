package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealthEndpoint tests the health check endpoint
func TestHealthEndpoint(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Test health endpoint
	resp, err := env.httpClient.Get(env.masterURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var health map[string]any
	err = json.NewDecoder(resp.Body).Decode(&health)
	require.NoError(t, err)

	assert.Equal(t, "healthy", health["status"])
	assert.Contains(t, health, "timestamp")
	// Note: "database" key is no longer in the health response
	// The health endpoint was simplified to return basic status info
}

// TestSystemStatusEndpoint tests the system status endpoint
func TestSystemStatusEndpoint(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create some data
	_, err = env.CreateTestBot("status-bot")
	require.NoError(t, err)

	_, err = env.CreateTestJob("status-job")
	require.NoError(t, err)

	// Test system status endpoint
	resp, err := env.httpClient.Get(env.masterURL + "/api/v1/system/status")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var status map[string]any
	err = json.NewDecoder(resp.Body).Decode(&status)
	require.NoError(t, err)

	assert.Equal(t, "operational", status["status"])
	assert.Contains(t, status, "bots")
	assert.Contains(t, status, "jobs")
	assert.Contains(t, status, "uptime")

	// Check counts
	bots := status["bots"].(map[string]any)
	assert.Equal(t, float64(1), bots["total"])

	jobs := status["jobs"].(map[string]any)
	assert.Equal(t, float64(1), jobs["total"])
}

// TestBotListEndpoint tests the bot list endpoint
func TestBotListEndpoint(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create bots
	for i := 0; i < 3; i++ {
		_, err = env.CreateTestBot(fmt.Sprintf("list-bot-%d", i))
		require.NoError(t, err)
	}

	// Test bot list endpoint
	resp, err := env.httpClient.Get(env.masterURL + "/api/v1/bots")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// API v1 returns response with "data" and "pagination" fields
	var response struct {
		Data []struct {
			Id       string `json:"id"`
			Name     string `json:"name"`
			Hostname string `json:"hostname"`
			Status   string `json:"status"`
			IsOnline bool   `json:"is_online"`
		} `json:"data"`
		Pagination struct {
			Limit   int  `json:"limit"`
			Offset  int  `json:"offset"`
			Total   int  `json:"total"`
			HasMore bool `json:"has_more"`
		} `json:"pagination"`
	}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Len(t, response.Data, 3)
	for _, bot := range response.Data {
		assert.Contains(t, bot.Name, "list-bot-")
	}
}

// TestGetBotEndpoint tests getting a specific bot
func TestGetBotEndpoint(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create a bot to retrieve
	testBot, err := env.CreateTestBot("get-test-bot")
	require.NoError(t, err)

	// Test getting the bot by ID
	resp, err := env.httpClient.Get(env.masterURL + "/api/v1/bots/" + testBot.ID)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Parse response - API v1 returns the bot directly
	var botResponse struct {
		Id           string   `json:"id"`
		Name         string   `json:"name"`
		Hostname     string   `json:"hostname"`
		Status       string   `json:"status"`
		IsOnline     bool     `json:"is_online"`
		Capabilities []string `json:"capabilities"`
	}
	err = json.NewDecoder(resp.Body).Decode(&botResponse)
	require.NoError(t, err)

	assert.Equal(t, testBot.ID, botResponse.Id)
	assert.Equal(t, testBot.Name, botResponse.Name)

	// Test getting a non-existent bot
	resp, err = env.httpClient.Get(env.masterURL + "/api/v1/bots/00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestJobEndpoints tests job-related endpoints
func TestJobEndpoints(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Test creating job via API - using correct API v1 schema
	jobRequest := map[string]any{
		"name":            "api-test-job",
		"priority":        5,
		"fuzzer":          "afl++",
		"target_binary":   "/bin/test",
		"timeout_seconds": 300,
	}

	body, _ := json.Marshal(jobRequest)
	resp, err := env.httpClient.Post(
		env.masterURL+"/api/v1/jobs",
		"application/json",
		bytes.NewBuffer(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// API returns generated.Job format with different field types
	var createdJob struct {
		Id           string `json:"id"`
		Name         string `json:"name"`
		Status       string `json:"status"`
		TargetBinary string `json:"target_binary"`
		Fuzzer       string `json:"fuzzer"`
	}
	err = json.NewDecoder(resp.Body).Decode(&createdJob)
	require.NoError(t, err)

	assert.Equal(t, "api-test-job", createdJob.Name)
	assert.Equal(t, "pending", createdJob.Status)

	// Test getting job
	resp, err = env.httpClient.Get(env.masterURL + "/api/v1/jobs/" + createdJob.Id)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test listing jobs
	resp, err = env.httpClient.Get(env.masterURL + "/api/v1/jobs")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// API returns JobListResponse with data array
	var jobListResponse struct {
		Data []struct {
			Id     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"data"`
		Pagination struct {
			Total int `json:"total"`
		} `json:"pagination"`
	}
	err = json.NewDecoder(resp.Body).Decode(&jobListResponse)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(jobListResponse.Data), 1)
	// Find our job in the list
	found := false
	for _, job := range jobListResponse.Data {
		if job.Id == createdJob.Id {
			found = true
			break
		}
	}
	assert.True(t, found, "Created job should be in the list")
}

// TestJobCancellationEndpoint tests job cancellation via API
func TestJobCancellationEndpoint(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create job
	job, err := env.CreateTestJob("cancel-api-test")
	require.NoError(t, err)

	// Cancel job via API
	req, err := http.NewRequest(
		"POST",
		env.masterURL+"/api/v1/jobs/"+job.ID+"/cancel",
		nil,
	)
	require.NoError(t, err)

	resp, err := env.httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify job is cancelled
	cancelledJob, err := env.state.GetJob(context.Background(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusCancelled, cancelledJob.Status)
}

// TestResultsEndpoints tests result reporting endpoints
func TestResultsEndpoints(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create job for results
	job, err := env.CreateTestJob("results-test")
	require.NoError(t, err)

	// Test crash reporting endpoint
	crashRequest := map[string]any{
		"id":        "crash-123",
		"job_id":    job.ID,
		"bot_id":    "test-bot",
		"timestamp": time.Now(),
		"input":     []byte("AAAA"),
		"size":      4,
		"hash":      "deadbeef",
		"type":      "segmentation_fault",
		"output":    "Segmentation fault",
	}

	body, _ := json.Marshal(crashRequest)
	resp, err := env.httpClient.Post(
		env.masterURL+"/api/v1/results/crash",
		"application/json",
		bytes.NewBuffer(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Test coverage reporting endpoint
	coverageRequest := map[string]any{
		"id":               "coverage-123",
		"job_id":           job.ID,
		"bot_id":           "test-bot",
		"timestamp":        time.Now(),
		"edges":            1000,
		"covered_edges":    500,
		"new_edges":        10,
		"coverage_percent": 50.0,
	}

	body, _ = json.Marshal(coverageRequest)
	resp, err = env.httpClient.Post(
		env.masterURL+"/api/v1/results/coverage",
		"application/json",
		bytes.NewBuffer(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

// TestPaginationAndFiltering tests API pagination and filtering
func TestPaginationAndFiltering(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create many jobs
	for i := 0; i < 25; i++ {
		job, err := env.CreateTestJob(fmt.Sprintf("page-job-%d", i))
		require.NoError(t, err)

		// Set different statuses
		if i%3 == 0 {
			job.Status = common.JobStatusCompleted
			now := time.Now()
			job.CompletedAt = &now
		} else if i%3 == 1 {
			job.Status = common.JobStatusRunning
		}

		err = env.state.SaveJobWithRetry(context.Background(), job)
		require.NoError(t, err)
	}

	// API response format with data and pagination
	type jobListResponse struct {
		Data []struct {
			Id     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"data"`
		Pagination struct {
			Total   int  `json:"total"`
			Limit   int  `json:"limit"`
			Offset  int  `json:"offset"`
			HasMore bool `json:"has_more"`
		} `json:"pagination"`
	}

	// Test pagination
	resp, err := env.httpClient.Get(env.masterURL + "/api/v1/jobs?limit=10&offset=0")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var page1 jobListResponse
	err = json.NewDecoder(resp.Body).Decode(&page1)
	require.NoError(t, err)
	assert.Len(t, page1.Data, 10)

	// Test second page
	resp, err = env.httpClient.Get(env.masterURL + "/api/v1/jobs?limit=10&offset=10")
	require.NoError(t, err)
	defer resp.Body.Close()

	var page2 jobListResponse
	err = json.NewDecoder(resp.Body).Decode(&page2)
	require.NoError(t, err)
	assert.Len(t, page2.Data, 10)

	// Ensure different jobs
	assert.NotEqual(t, page1.Data[0].Id, page2.Data[0].Id)

	// Test filtering by status
	resp, err = env.httpClient.Get(env.masterURL + "/api/v1/jobs?status=completed")
	require.NoError(t, err)
	defer resp.Body.Close()

	var completedResp jobListResponse
	err = json.NewDecoder(resp.Body).Decode(&completedResp)
	require.NoError(t, err)

	for _, job := range completedResp.Data {
		assert.Equal(t, "completed", job.Status)
	}
}

// TestErrorHandling tests API error handling
func TestErrorHandling(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Test invalid JSON
	resp, err := env.httpClient.Post(
		env.masterURL+"/api/v1/jobs",
		"application/json",
		bytes.NewBufferString("invalid json"),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errorResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err)
	assert.Contains(t, errorResp, "error")

	// Test missing required fields
	invalidJob := map[string]any{
		"name": "missing-fields",
		// Missing required fields
	}

	body, _ := json.Marshal(invalidJob)
	resp, err = env.httpClient.Post(
		env.masterURL+"/api/v1/jobs",
		"application/json",
		bytes.NewBuffer(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Test method not allowed
	resp, err = env.httpClient.Post(
		env.masterURL+"/api/v1/jobs/123", // POST not allowed on specific job
		"application/json",
		nil,
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

// TestConcurrentAPIRequests tests handling of concurrent API requests
func TestConcurrentAPIRequests(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Make concurrent requests
	numRequests := 50
	results := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(index int) {
			// Mix of different API calls
			switch index % 4 {
			case 0:
				// Get system status
				resp, err := env.httpClient.Get(env.masterURL + "/api/v1/system/status")
				if err == nil {
					resp.Body.Close()
				}
				results <- err

			case 1:
				// List bots
				resp, err := env.httpClient.Get(env.masterURL + "/api/v1/bots")
				if err == nil {
					resp.Body.Close()
				}
				results <- err

			case 2:
				// List jobs
				resp, err := env.httpClient.Get(env.masterURL + "/api/v1/jobs")
				if err == nil {
					resp.Body.Close()
				}
				results <- err

			case 3:
				// Create job
				jobReq := map[string]any{
					"name":        fmt.Sprintf("concurrent-job-%d", index),
					"fuzzer":      "afl++",
					"target":      "/bin/test",
					"target_args": []string{"@@"},
				}
				body, _ := json.Marshal(jobReq)
				resp, err := env.httpClient.Post(
					env.masterURL+"/api/v1/jobs",
					"application/json",
					bytes.NewBuffer(body),
				)
				if err == nil {
					resp.Body.Close()
				}
				results <- err
			}
		}(i)
	}

	// Collect results
	for i := 0; i < numRequests; i++ {
		err := <-results
		assert.NoError(t, err)
	}
}

// TestAPIAuthentication tests API authentication (if enabled)
// TODO: This test needs to be updated when authentication is implemented
/*
func TestAPIAuthentication(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Enable authentication
	env.masterConfig.Security.EnableAuth = true
	env.masterConfig.Security.AuthToken = "test-token-123"

	// Recreate API handlers with auth enabled
	env.apiHandlers = master.NewAPIHandlers(
		env.state,
		env.timeoutMgr,
		env.recoveryMgr,
		env.masterConfig,
	)
	env.server = master.NewServer(env.masterConfig, env.apiHandlers)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Test without auth token
	resp, err := env.httpClient.Get(env.masterURL + "/api/v1/bots")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Test with invalid auth token
	req, err := http.NewRequest("GET", env.masterURL+"/api/v1/bots", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer invalid-token")

	resp, err = env.httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Test with valid auth token
	req, err = http.NewRequest("GET", env.masterURL+"/api/v1/bots", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-token-123")

	resp, err = env.httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
*/

// TestWebSocketEndpoint tests WebSocket connections for real-time updates
func TestWebSocketEndpoint(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Build WebSocket URL from HTTP URL
	wsURL := "ws" + strings.TrimPrefix(env.masterURL, "http") + "/ws"

	// Connect to WebSocket endpoint
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	defer conn.Close()

	// Should receive welcome message
	var welcomeMsg struct {
		Type      string                 `json:"type"`
		Data      map[string]interface{} `json:"data"`
		Timestamp time.Time              `json:"timestamp"`
	}
	err = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	require.NoError(t, err)
	err = conn.ReadJSON(&welcomeMsg)
	require.NoError(t, err)
	assert.Equal(t, "welcome", welcomeMsg.Type)
	assert.NotEmpty(t, welcomeMsg.Data["client_id"])
	assert.Equal(t, "2.0", welcomeMsg.Data["version"])

	// Send subscription message
	subMsg := map[string]interface{}{
		"type": "subscribe",
		"data": map[string]interface{}{
			"topics": []string{"campaign:test", "crashes:all"},
		},
	}
	err = conn.WriteJSON(subMsg)
	require.NoError(t, err)

	// Send pong message (responding to ping)
	pongMsg := map[string]interface{}{
		"type": "pong",
		"data": map[string]interface{}{},
	}
	err = conn.WriteJSON(pongMsg)
	require.NoError(t, err)

	// Connection should remain open - close gracefully
	err = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	assert.NoError(t, err)
}

// TestAPIDocumentation tests API documentation endpoint
func TestAPIDocumentation(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Test OpenAPI/Swagger endpoint (if available)
	resp, err := env.httpClient.Get(env.masterURL + "/api/docs")
	require.NoError(t, err)
	defer resp.Body.Close()

	// May return 404 if not implemented
	if resp.StatusCode == http.StatusOK {
		// Verify it returns valid documentation
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "swagger")
	}
}
