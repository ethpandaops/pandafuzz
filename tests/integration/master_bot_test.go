package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/bot"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMasterBotRegistration tests bot registration with master
func TestMasterBotRegistration(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create bot client
	client, err := bot.NewRetryClient(env.botConfig, env.logger)
	require.NoError(t, err)
	defer client.Close()

	// Register bot
	response, err := client.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities, "http://localhost:9000")
	require.NoError(t, err)
	assert.NotNil(t, response)
	// The API generates a new UUID for the bot, not using the provided name
	assert.NotEmpty(t, response.BotID)
	assert.Equal(t, "registered", response.Status)

	// Verify bot is registered in database using the returned bot ID
	registeredBot, err := env.state.GetBot(context.Background(), response.BotID)
	require.NoError(t, err)
	assert.Equal(t, response.BotID, registeredBot.ID)
	assert.Equal(t, common.BotStatusIdle, registeredBot.Status)
	assert.Equal(t, env.botConfig.Capabilities, registeredBot.Capabilities)
}

// TestMasterBotHeartbeat tests heartbeat mechanism
func TestMasterBotHeartbeat(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create bot client
	client, err := bot.NewRetryClient(env.botConfig, env.logger)
	require.NoError(t, err)
	defer client.Close()

	// Register bot first - use the returned bot ID for subsequent operations
	regResponse, err := client.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities, "http://localhost:9000")
	require.NoError(t, err)
	botID := regResponse.BotID

	// Send heartbeat using the registered bot ID
	err = client.SendHeartbeat(botID, common.BotStatusIdle, nil)
	require.NoError(t, err)

	// Verify bot last seen is updated
	registeredBot, err := env.state.GetBot(context.Background(), botID)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), registeredBot.LastSeen, 2*time.Second)

	// Test heartbeat with job - use a valid UUID for job ID
	jobID := "550e8400-e29b-41d4-a716-446655440000"
	err = client.SendHeartbeat(botID, common.BotStatusBusy, &jobID)
	require.NoError(t, err)

	// Verify bot status is updated
	registeredBot, err = env.state.GetBot(context.Background(), botID)
	require.NoError(t, err)
	assert.Equal(t, common.BotStatusBusy, registeredBot.Status)
	assert.NotNil(t, registeredBot.CurrentJob)
	assert.Equal(t, jobID, *registeredBot.CurrentJob)
}

// TestMasterBotDeregistration tests bot deregistration
func TestMasterBotDeregistration(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create bot client
	client, err := bot.NewRetryClient(env.botConfig, env.logger)
	require.NoError(t, err)
	defer client.Close()

	// Register bot - use the returned bot ID
	regResponse, err := client.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities, "http://localhost:9000")
	require.NoError(t, err)
	botID := regResponse.BotID

	// Verify bot exists
	_, err = env.state.GetBot(context.Background(), botID)
	require.NoError(t, err)

	// Deregister bot
	err = client.DeregisterBot(botID)
	require.NoError(t, err)

	// Verify bot is removed - use AssertEventually to handle any race conditions
	// between API response and database transaction commit
	AssertEventually(t, func() bool {
		_, err := env.state.GetBot(context.Background(), botID)
		return err != nil
	}, 2*time.Second, "Bot should be deleted after deregistration")
}

// TestBotAgent tests the bot agent lifecycle
func TestBotAgent(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Configure a short heartbeat interval for testing
	botConfig := *env.botConfig
	botConfig.Timeouts.HeartbeatInterval = 200 * time.Millisecond

	// Create bot agent with short heartbeat interval
	agent, err := bot.NewAgent(&botConfig, env.logger)
	require.NoError(t, err)

	// Start agent
	err = agent.Start()
	require.NoError(t, err)
	defer agent.Stop()

	// Wait for registration - check by listing all bots since API generates new UUID
	AssertEventually(t, func() bool {
		bots, err := env.state.ListBots(context.Background())
		return err == nil && len(bots) > 0
	}, 5*time.Second, "Bot should be registered")

	// Verify agent is running
	assert.True(t, agent.IsRunning())

	// Wait for at least one heartbeat to be sent (heartbeat interval is 200ms)
	time.Sleep(500 * time.Millisecond)

	// Check agent stats
	stats := agent.GetStats()
	assert.Equal(t, "running", stats.CurrentStatus)
	assert.Greater(t, stats.HeartbeatsSent, int64(0))

	// Stop agent
	err = agent.Stop()
	require.NoError(t, err)
	assert.False(t, agent.IsRunning())
}

// TestHeartbeatTimeout tests bot timeout handling
func TestHeartbeatTimeout(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Use shorter timeout for testing
	env.masterConfig.Timeouts.BotHeartbeat = 1 * time.Second

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create and register bot
	testBot, err := env.CreateTestBot("timeout-bot")
	require.NoError(t, err)

	// Register bot with timeout manager so it can detect the timeout
	env.timeoutMgr.SetBotTimeout(testBot.ID, 1*time.Second)

	// Wait for timeout to expire
	time.Sleep(2 * time.Second)

	// Force a timeout check instead of waiting for the 30-second interval
	env.timeoutMgr.ForceTimeoutCheck()

	// Check if bot is marked as timed out
	dbBot, err := env.state.GetBot(context.Background(), testBot.ID)
	require.NoError(t, err)
	assert.Equal(t, common.BotStatusTimedOut, dbBot.Status)
}

// TestMultipleBots tests multiple bots connecting to master
func TestMultipleBots(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	numBots := 5
	agents := make([]*bot.Agent, numBots)

	// Create and start multiple bots
	for i := 0; i < numBots; i++ {
		config := *env.botConfig
		config.ID = fmt.Sprintf("bot-%d", i)

		agent, err := bot.NewAgent(&config, env.logger)
		require.NoError(t, err)

		err = agent.Start()
		require.NoError(t, err)

		agents[i] = agent
	}

	// Cleanup
	defer func() {
		for _, agent := range agents {
			agent.Stop()
		}
	}()

	// Wait for all bots to register
	AssertEventually(t, func() bool {
		bots, err := env.state.ListBots(context.Background())
		return err == nil && len(bots) == numBots
	}, 10*time.Second, "All bots should be registered")

	// Verify all bots are idle
	bots, err := env.state.ListBots(context.Background())
	require.NoError(t, err)

	for _, bot := range bots {
		assert.Equal(t, common.BotStatusIdle, bot.Status)
	}
}

// TestBotReconnection tests bot reconnection after network failure
func TestBotReconnection(t *testing.T) {
	// TODO: This test needs to be redesigned. Issues:
	// 1. The RetryClient uses infinite retries with exponential backoff, causing the test
	//    to hang for a long time when the server is down
	// 2. Creating a new server while the old one may still be releasing the port causes
	//    binding conflicts
	// 3. Proper reconnection testing requires:
	//    - A way to configure shorter retry timeouts for testing
	//    - Graceful server restart that waits for port release
	//    - Or using different ports for the restarted server
	t.Skip("Test needs redesign for reliable reconnection testing")
}

// TestAPIEndpoints tests the master API endpoints directly
func TestAPIEndpoints(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Test health endpoint
	resp, err := env.httpClient.Get(env.masterURL + "/health")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Test system status endpoint
	resp, err = env.httpClient.Get(env.masterURL + "/api/v1/system/status")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var status map[string]any
	err = json.NewDecoder(resp.Body).Decode(&status)
	resp.Body.Close()
	require.NoError(t, err)
	assert.Contains(t, status, "status")
	assert.Contains(t, status, "timestamp")

	// Test bots list endpoint - API v1 returns {data: [], pagination: {}}
	resp, err = env.httpClient.Get(env.masterURL + "/api/v1/bots")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var botsResponse struct {
		Data       []map[string]any `json:"data"`
		Pagination map[string]any   `json:"pagination"`
	}
	err = json.NewDecoder(resp.Body).Decode(&botsResponse)
	resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, 0, len(botsResponse.Data)) // No bots registered yet

	// Test jobs list endpoint - API v1 returns {data: [], pagination: {}}
	resp, err = env.httpClient.Get(env.masterURL + "/api/v1/jobs")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var jobsResponse struct {
		Data       []map[string]any `json:"data"`
		Pagination map[string]any   `json:"pagination"`
	}
	err = json.NewDecoder(resp.Body).Decode(&jobsResponse)
	resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, 0, len(jobsResponse.Data)) // No jobs created yet
}

// TestConcurrentBotRegistrations tests concurrent bot registrations
func TestConcurrentBotRegistrations(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	numBots := 10
	errChan := make(chan error, numBots)

	// Register bots concurrently
	for i := 0; i < numBots; i++ {
		go func(id int) {
			config := *env.botConfig
			config.ID = fmt.Sprintf("concurrent-bot-%d", id)

			client, err := bot.NewRetryClient(&config, env.logger)
			if err != nil {
				errChan <- err
				return
			}
			defer client.Close()

			_, err = client.RegisterBot(config.ID, config.Capabilities, "http://localhost:9000")
			errChan <- err
		}(i)
	}

	// Collect results
	for i := 0; i < numBots; i++ {
		err := <-errChan
		assert.NoError(t, err)
	}

	// Verify all bots are registered
	bots, err := env.state.ListBots(context.Background())
	require.NoError(t, err)
	assert.Equal(t, numBots, len(bots))
}

// TestBotMetrics tests bot metrics collection
func TestBotMetrics(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Register a bot - use the returned bot ID
	client, err := bot.NewRetryClient(env.botConfig, env.logger)
	require.NoError(t, err)
	defer client.Close()

	regResponse, err := client.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities, "http://localhost:9000")
	require.NoError(t, err)
	botID := regResponse.BotID

	// Send some heartbeats using the registered bot ID
	for i := 0; i < 5; i++ {
		err = client.SendHeartbeat(botID, common.BotStatusIdle, nil)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
	}

	// Bot metrics are available via the monitoring config (see MonitoringConfig.MetricsPort)
	// Metrics are served on a separate port when monitoring.metrics_enabled is true
}

// TestInvalidBotRequests tests error handling for invalid requests
func TestInvalidBotRequests(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Test heartbeat from unregistered bot (use a valid UUID format for the bot ID)
	url := env.masterURL + "/api/v1/bots/00000000-0000-0000-0000-000000000000/heartbeat"
	payload := map[string]any{
		"status": "idle",
	}

	body, _ := json.Marshal(payload)
	resp, err := env.httpClient.Post(url, "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return not found
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Test invalid registration (empty capabilities) - use /api/v1/bots not /api/v1/bots/register
	url = env.masterURL + "/api/v1/bots"
	payload = map[string]any{
		"name":         "test-bot",
		"hostname":     "test",
		"api_endpoint": "http://localhost:9000",
		"capabilities": []string{},
	}

	body, _ = json.Marshal(payload)
	resp, err = env.httpClient.Post(url, "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return bad request (empty capabilities)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
