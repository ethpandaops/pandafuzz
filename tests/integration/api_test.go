package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
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
	
	var health map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&health)
	require.NoError(t, err)
	
	assert.Equal(t, "healthy", health["status"])
	assert.Contains(t, health, "timestamp")
	assert.Contains(t, health, "database")
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
	
	var status map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&status)
	require.NoError(t, err)
	
	assert.Equal(t, "operational", status["status"])
	assert.Contains(t, status, "bots")
	assert.Contains(t, status, "jobs")
	assert.Contains(t, status, "uptime")
	
	// Check counts
	bots := status["bots"].(map[string]interface{})
	assert.Equal(t, float64(1), bots["total"])
	
	jobs := status["jobs"].(map[string]interface{})
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
	
	var bots []common.Bot
	err = json.NewDecoder(resp.Body).Decode(&bots)
	require.NoError(t, err)
	
	assert.Len(t, bots, 3)
	for i, bot := range bots {
		assert.Equal(t, fmt.Sprintf("list-bot-%d", i), bot.ID)
		assert.Equal(t, common.BotStatusIdle, bot.Status)
	}
}

// TestGetBotEndpoint tests getting a specific bot
func TestGetBotEndpoint(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create bot
	bot, err := env.CreateTestBot("get-bot")
	require.NoError(t, err)

	// Test get bot endpoint
	resp, err := env.httpClient.Get(env.masterURL + "/api/v1/bots/" + bot.ID)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	var retrievedBot common.Bot
	err = json.NewDecoder(resp.Body).Decode(&retrievedBot)
	require.NoError(t, err)
	
	assert.Equal(t, bot.ID, retrievedBot.ID)
	assert.Equal(t, bot.Status, retrievedBot.Status)
	assert.Equal(t, bot.Capabilities, retrievedBot.Capabilities)

	// Test non-existent bot
	resp, err = env.httpClient.Get(env.masterURL + "/api/v1/bots/non-existent")
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

	// Test creating job via API
	jobRequest := map[string]interface{}{
		"name":         "api-test-job",
		"priority":     "normal",
		"fuzzer":       "afl++",
		"target":       "/bin/test",
		"target_args":  []string{"@@"},
		"corpus":       []string{"/corpus"},
		"timeout_sec":  300,
		"memory_limit": 1024,
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
	
	var createdJob common.Job
	err = json.NewDecoder(resp.Body).Decode(&createdJob)
	require.NoError(t, err)
	
	assert.Equal(t, "api-test-job", createdJob.Name)
	assert.Equal(t, common.JobStatusPending, createdJob.Status)

	// Test getting job
	resp, err = env.httpClient.Get(env.masterURL + "/api/v1/jobs/" + createdJob.ID)
	require.NoError(t, err)
	defer resp.Body.Close()
	
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test listing jobs
	resp, err = env.httpClient.Get(env.masterURL + "/api/v1/jobs")
	require.NoError(t, err)
	defer resp.Body.Close()
	
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	var jobs []common.Job
	err = json.NewDecoder(resp.Body).Decode(&jobs)
	require.NoError(t, err)
	
	assert.Len(t, jobs, 1)
	assert.Equal(t, createdJob.ID, jobs[0].ID)
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
	cancelledJob, err := env.state.GetJob(job.ID)
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
	crashRequest := map[string]interface{}{
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
	coverageRequest := map[string]interface{}{
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
		
		err = env.state.SaveJobWithRetry(job)
		require.NoError(t, err)
	}

	// Test pagination
	resp, err := env.httpClient.Get(env.masterURL + "/api/v1/jobs?limit=10&offset=0")
	require.NoError(t, err)
	defer resp.Body.Close()
	
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	var page1Jobs []common.Job
	err = json.NewDecoder(resp.Body).Decode(&page1Jobs)
	require.NoError(t, err)
	assert.Len(t, page1Jobs, 10)

	// Test second page
	resp, err = env.httpClient.Get(env.masterURL + "/api/v1/jobs?limit=10&offset=10")
	require.NoError(t, err)
	defer resp.Body.Close()
	
	var page2Jobs []common.Job
	err = json.NewDecoder(resp.Body).Decode(&page2Jobs)
	require.NoError(t, err)
	assert.Len(t, page2Jobs, 10)
	
	// Ensure different jobs
	assert.NotEqual(t, page1Jobs[0].ID, page2Jobs[0].ID)

	// Test filtering by status
	resp, err = env.httpClient.Get(env.masterURL + "/api/v1/jobs?status=completed")
	require.NoError(t, err)
	defer resp.Body.Close()
	
	var completedJobs []common.Job
	err = json.NewDecoder(resp.Body).Decode(&completedJobs)
	require.NoError(t, err)
	
	for _, job := range completedJobs {
		assert.Equal(t, common.JobStatusCompleted, job.Status)
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
	
	var errorResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err)
	assert.Contains(t, errorResp, "error")

	// Test missing required fields
	invalidJob := map[string]interface{}{
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
				jobReq := map[string]interface{}{
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

// TestWebSocketEndpoint tests WebSocket connections (if implemented)
func TestWebSocketEndpoint(t *testing.T) {
	// This would test WebSocket endpoints for real-time updates
	// Currently just a placeholder
	t.Skip("WebSocket support not yet implemented")
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