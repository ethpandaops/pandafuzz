package integration

import (
	"bytes"
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
	client, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer client.Close()

	// Register bot
	response, err := client.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, env.botConfig.ID, response.BotID)
	assert.Equal(t, "registered", response.Status)

	// Verify bot is registered in database
	registeredBot, err := env.state.GetBot(env.botConfig.ID)
	require.NoError(t, err)
	assert.Equal(t, env.botConfig.ID, registeredBot.ID)
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
	client, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer client.Close()

	// Register bot first
	_, err = client.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)

	// Send heartbeat
	err = client.SendHeartbeat(env.botConfig.ID, common.BotStatusIdle, nil)
	require.NoError(t, err)

	// Verify bot last seen is updated
	bot, err := env.state.GetBot(env.botConfig.ID)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), bot.LastSeen, 2*time.Second)

	// Test heartbeat with job
	jobID := "test-job-123"
	err = client.SendHeartbeat(env.botConfig.ID, common.BotStatusBusy, &jobID)
	require.NoError(t, err)

	// Verify bot status is updated
	bot, err = env.state.GetBot(env.botConfig.ID)
	require.NoError(t, err)
	assert.Equal(t, common.BotStatusBusy, bot.Status)
	assert.NotNil(t, bot.CurrentJob)
	assert.Equal(t, jobID, *bot.CurrentJob)
}

// TestMasterBotDeregistration tests bot deregistration
func TestMasterBotDeregistration(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create bot client
	client, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer client.Close()

	// Register bot
	_, err = client.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)

	// Verify bot exists
	_, err = env.state.GetBot(env.botConfig.ID)
	require.NoError(t, err)

	// Deregister bot
	err = client.DeregisterBot(env.botConfig.ID)
	require.NoError(t, err)

	// Verify bot is removed
	_, err = env.state.GetBot(env.botConfig.ID)
	assert.Error(t, err)
}

// TestBotAgent tests the bot agent lifecycle
func TestBotAgent(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create bot agent
	agent, err := bot.NewAgent(env.botConfig)
	require.NoError(t, err)

	// Start agent
	err = agent.Start()
	require.NoError(t, err)
	defer agent.Stop()

	// Wait for registration
	AssertEventually(t, func() bool {
		_, err := env.state.GetBot(env.botConfig.ID)
		return err == nil
	}, 5*time.Second, "Bot should be registered")

	// Verify agent is running
	assert.True(t, agent.IsRunning())

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
	env.masterConfig.Timeouts.BotIdle = 2 * time.Second
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create and register bot
	bot, err := env.CreateTestBot("timeout-bot")
	require.NoError(t, err)

	// Wait for timeout
	time.Sleep(3 * time.Second)

	// Check if bot is marked as offline
	err = env.WaitForBotStatus(bot.ID, common.BotStatusOffline, 5*time.Second)
	require.NoError(t, err)
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
		
		agent, err := bot.NewAgent(&config)
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
		bots, err := env.state.ListBots()
		return err == nil && len(bots) == numBots
	}, 10*time.Second, "All bots should be registered")

	// Verify all bots are idle
	bots, err := env.state.ListBots()
	require.NoError(t, err)
	
	for _, bot := range bots {
		assert.Equal(t, common.BotStatusIdle, bot.Status)
	}
}

// TestBotReconnection tests bot reconnection after network failure
func TestBotReconnection(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create bot client with retry
	client, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer client.Close()

	// Register bot
	_, err = client.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)

	// Simulate network failure by stopping server
	env.server.Shutdown(env.ctx)

	// Try to send heartbeat (should fail but retry)
	err = client.SendHeartbeat(env.botConfig.ID, common.BotStatusIdle, nil)
	assert.Error(t, err)

	// Restart server
	env.server = master.NewServer(env.masterConfig, env.apiHandlers)
	err = env.StartMaster()
	require.NoError(t, err)

	// Wait for master recovery
	err = client.WaitForMasterRecovery()
	require.NoError(t, err)

	// Heartbeat should work again
	err = client.SendHeartbeat(env.botConfig.ID, common.BotStatusIdle, nil)
	require.NoError(t, err)
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
	
	var status map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&status)
	resp.Body.Close()
	require.NoError(t, err)
	assert.Contains(t, status, "status")
	assert.Contains(t, status, "timestamp")

	// Test bots list endpoint
	resp, err = env.httpClient.Get(env.masterURL + "/api/v1/bots")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	var bots []common.Bot
	err = json.NewDecoder(resp.Body).Decode(&bots)
	resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, 0, len(bots)) // No bots registered yet

	// Test jobs list endpoint
	resp, err = env.httpClient.Get(env.masterURL + "/api/v1/jobs")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	var jobs []common.Job
	err = json.NewDecoder(resp.Body).Decode(&jobs)
	resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, 0, len(jobs)) // No jobs created yet
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
			
			client, err := bot.NewRetryClient(&config)
			if err != nil {
				errChan <- err
				return
			}
			defer client.Close()
			
			_, err = client.RegisterBot(config.ID, config.Capabilities)
			errChan <- err
		}(i)
	}

	// Collect results
	for i := 0; i < numBots; i++ {
		err := <-errChan
		assert.NoError(t, err)
	}

	// Verify all bots are registered
	bots, err := env.state.ListBots()
	require.NoError(t, err)
	assert.Equal(t, numBots, len(bots))
}

// TestBotMetrics tests bot metrics collection
func TestBotMetrics(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Enable metrics
	env.masterConfig.Server.EnableMetrics = true
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Register a bot
	client, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer client.Close()
	
	_, err = client.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)

	// Send some heartbeats
	for i := 0; i < 5; i++ {
		err = client.SendHeartbeat(env.botConfig.ID, common.BotStatusIdle, nil)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
	}

	// Check metrics endpoint
	resp, err := env.httpClient.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", env.masterConfig.Server.MetricsPort))
	require.NoError(t, err)
	defer resp.Body.Close()
	
	// Metrics endpoint should return text
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestInvalidBotRequests tests error handling for invalid requests
func TestInvalidBotRequests(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Test heartbeat from unregistered bot
	url := env.masterURL + "/api/v1/bots/unknown-bot/heartbeat"
	payload := map[string]interface{}{
		"status": "idle",
	}
	
	body, _ := json.Marshal(payload)
	resp, err := env.httpClient.Post(url, "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	
	// Should return not found
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Test invalid registration (empty capabilities)
	url = env.masterURL + "/api/v1/bots/register"
	payload = map[string]interface{}{
		"hostname":     "test",
		"capabilities": []string{},
	}
	
	body, _ = json.Marshal(payload)
	resp, err = env.httpClient.Post(url, "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	
	// Should return bad request
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}