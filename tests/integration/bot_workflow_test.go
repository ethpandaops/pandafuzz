package integration

import (
	// "context"
	// "fmt"
	// "testing"
	// "time"

	// "github.com/stretchr/testify/assert"
	// "github.com/stretchr/testify/require"

	// "github.com/ethpandaops/pandafuzz/pkg/bot"
	// "github.com/ethpandaops/pandafuzz/pkg/common"
	// "github.com/ethpandaops/pandafuzz/pkg/master"
)

// TestCompleteBottWorkflow tests the complete bot workflow from registration to job completion
// TODO: This test needs to be updated to use the current API
/*
func TestCompleteBotWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test environment
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Start master server
	masterCfg := &common.MasterConfig{
		Server: common.ServerConfig{
			Host: "127.0.0.1",
			Port: 0, // Random port
		},
		Database: common.DatabaseConfig{
			Type: "sqlite",
			Path: ":memory:",
		},
	}

	// Note: NewServer signature has changed - needs to be updated
	// masterServer, err := master.NewServer(masterCfg)
	require.NoError(t, err)

	// Start master in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := masterServer.Start(ctx); err != nil && err != context.Canceled {
			t.Errorf("Master server error: %v", err)
		}
	}()

	// Wait for master to be ready
	masterURL := masterServer.GetURL()
	require.Eventually(t, func() bool {
		return env.IsMasterHealthy(masterURL)
	}, 10*time.Second, 100*time.Millisecond, "Master did not become healthy")

	// Create and start bot
	botCfg := &common.BotConfig{
		ID:           "integration-bot-1",
		Name:         "Integration Test Bot",
		MasterURL:    masterURL,
		Capabilities: []string{"afl++", "libfuzzer"},
		WorkDirectory: env.TempDir,
		Timeouts: common.BotTimeouts{
			MasterCommunication: 5 * time.Second,
			HeartbeatInterval:   1 * time.Second,
			JobExecution:        30 * time.Second,
		},
		Retry: common.RetryConfig{
			MaxRetries:   3,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     1 * time.Second,
			Multiplier:   2.0,
		},
	}

	agent, err := bot.NewAgent(botCfg)
	require.NoError(t, err)

	// Start bot agent
	botCtx, botCancel := context.WithCancel(context.Background())
	defer botCancel()

	botReady := make(chan bool)
	go func() {
		if err := agent.Start(botCtx, botReady); err != nil && err != context.Canceled {
			t.Errorf("Bot agent error: %v", err)
		}
	}()

	// Wait for bot to register
	select {
	case <-botReady:
		t.Log("Bot registered successfully")
	case <-time.After(5 * time.Second):
		t.Fatal("Bot registration timeout")
	}

	// Verify bot appears in master's bot list
	bots, err := env.GetBots(masterURL)
	require.NoError(t, err)
	require.Len(t, bots, 1)
	assert.Equal(t, botCfg.ID, bots[0].ID)
	assert.Equal(t, "idle", bots[0].Status)

	// Create a fuzzing campaign
	campaign := &common.Campaign{
		Name:        "Test Campaign",
		Description: "Integration test campaign",
		Status:      common.CampaignStatusActive,
		Config: common.CampaignConfig{
			FuzzerType:    "afl++",
			TargetBinary:  "/test/binary",
			Seeds:         []string{"/test/seeds"},
			MaxCrashes:    10,
			MaxRuntime:    3600,
		},
	}

	campaignID, err := env.CreateCampaign(masterURL, campaign)
	require.NoError(t, err)
	require.NotEmpty(t, campaignID)

	// Create a job for the bot
	job := &common.Job{
		CampaignID: campaignID,
		FuzzerType: "afl++",
		TargetPath: "/test/binary",
		Duration:   5 * time.Second, // Short duration for test
		Config: common.JobConfig{
			Arguments:   []string{"-i", "/test/seeds", "-o", "/test/output"},
			MemoryLimit: 1024,
		},
	}

	jobID, err := env.CreateJob(masterURL, job)
	require.NoError(t, err)
	require.NotEmpty(t, jobID)

	// Wait for bot to pick up and process the job
	require.Eventually(t, func() bool {
		status, err := env.GetJobStatus(masterURL, jobID)
		if err != nil {
			return false
		}
		return status == string(common.JobStatusRunning) || 
		       status == string(common.JobStatusCompleted)
	}, 10*time.Second, 100*time.Millisecond, "Job was not picked up by bot")

	// Wait for job completion
	require.Eventually(t, func() bool {
		status, err := env.GetJobStatus(masterURL, jobID)
		if err != nil {
			return false
		}
		return status == string(common.JobStatusCompleted) || 
		       status == string(common.JobStatusFailed)
	}, 15*time.Second, 100*time.Millisecond, "Job did not complete")

	// Verify job results were submitted
	results, err := env.GetJobResults(masterURL, jobID)
	require.NoError(t, err)
	assert.NotNil(t, results)
	assert.Equal(t, jobID, results.JobID)
	assert.Equal(t, botCfg.ID, results.BotID)
	assert.NotZero(t, results.TotalExecs)

	// Check bot metrics
	metrics, err := env.GetBotMetrics(masterURL, botCfg.ID)
	require.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Greater(t, metrics.JobsCompleted, 0)

	// Gracefully stop bot
	botCancel()
	
	// Wait for bot to deregister
	require.Eventually(t, func() bool {
		bots, err := env.GetBots(masterURL)
		if err != nil {
			return false
		}
		for _, bot := range bots {
			if bot.ID == botCfg.ID {
				return bot.Status == "offline"
			}
		}
		return true
	}, 5*time.Second, 100*time.Millisecond, "Bot did not deregister")
}

// TestMultipleBotCoordination tests multiple bots working together
func TestMultipleBotCoordination(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Start master
	masterServer := env.StartMaster(t)
	masterURL := masterServer.GetURL()

	// Start multiple bots
	numBots := 3
	bots := make([]*bot.Agent, numBots)
	contexts := make([]context.Context, numBots)
	cancels := make([]context.CancelFunc, numBots)

	for i := 0; i < numBots; i++ {
		botCfg := &common.BotConfig{
			ID:           fmt.Sprintf("bot-%d", i),
			Name:         fmt.Sprintf("Test Bot %d", i),
			MasterURL:    masterURL,
			Capabilities: []string{"afl++"},
			WorkDirectory: env.TempDir,
			Timeouts: common.BotTimeouts{
				HeartbeatInterval: 500 * time.Millisecond,
			},
		}

		agent, err := bot.NewAgent(botCfg)
		require.NoError(t, err)
		bots[i] = agent

		ctx, cancel := context.WithCancel(context.Background())
		contexts[i] = ctx
		cancels[i] = cancel

		// Start bot
		ready := make(chan bool)
		go func(a *bot.Agent, c context.Context, r chan bool) {
			if err := a.Start(c, r); err != nil && err != context.Canceled {
				t.Errorf("Bot error: %v", err)
			}
		}(agent, ctx, ready)

		// Wait for registration
		select {
		case <-ready:
			t.Logf("Bot %d registered", i)
		case <-time.After(5 * time.Second):
			t.Fatalf("Bot %d registration timeout", i)
		}
	}

	// Verify all bots are registered
	registeredBots, err := env.GetBots(masterURL)
	require.NoError(t, err)
	assert.Len(t, registeredBots, numBots)

	// Create campaign with multiple jobs
	campaign := env.CreateTestCampaign(t, masterURL, "Multi-bot Campaign")
	
	// Create more jobs than bots to test distribution
	numJobs := numBots * 2
	jobIDs := make([]string, numJobs)
	
	for i := 0; i < numJobs; i++ {
		job := &common.Job{
			CampaignID: campaign.ID,
			FuzzerType: "afl++",
			TargetPath: "/test/binary",
			Duration:   2 * time.Second,
		}
		jobID, err := env.CreateJob(masterURL, job)
		require.NoError(t, err)
		jobIDs[i] = jobID
	}

	// Wait for all jobs to be picked up
	require.Eventually(t, func() bool {
		for _, jobID := range jobIDs {
			status, _ := env.GetJobStatus(masterURL, jobID)
			if status != string(common.JobStatusRunning) && 
			   status != string(common.JobStatusCompleted) {
				return false
			}
		}
		return true
	}, 10*time.Second, 100*time.Millisecond, "Not all jobs were picked up")

	// Verify job distribution across bots
	jobAssignments := make(map[string]int)
	for _, jobID := range jobIDs {
		job, err := env.GetJob(masterURL, jobID)
		require.NoError(t, err)
		jobAssignments[job.BotID]++
	}

	// Each bot should have picked up at least one job
	assert.Len(t, jobAssignments, numBots)
	for botID, count := range jobAssignments {
		assert.Greater(t, count, 0, "Bot %s didn't pick up any jobs", botID)
	}

	// Stop all bots
	for i := range cancels {
		cancels[i]()
	}
}

// TestBotReconnectionResilience tests bot reconnection after master restart
func TestBotReconnectionResilience(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Start master
	masterServer1 := env.StartMaster(t)
	masterURL := masterServer1.GetURL()

	// Start bot with aggressive retry settings
	botCfg := &common.BotConfig{
		ID:        "resilient-bot",
		MasterURL: masterURL,
		Capabilities: []string{"afl++"},
		Retry: common.RetryConfig{
			MaxRetries:   10,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     2 * time.Second,
		},
		Timeouts: common.BotTimeouts{
			HeartbeatInterval: 500 * time.Millisecond,
		},
	}

	agent, err := bot.NewAgent(botCfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan bool)
	go func() {
		if err := agent.Start(ctx, ready); err != nil && err != context.Canceled {
			t.Errorf("Bot error: %v", err)
		}
	}()

	// Wait for initial registration
	select {
	case <-ready:
		t.Log("Bot registered initially")
	case <-time.After(5 * time.Second):
		t.Fatal("Initial registration timeout")
	}

	// Verify bot is registered
	bots, err := env.GetBots(masterURL)
	require.NoError(t, err)
	require.Len(t, bots, 1)
	assert.Equal(t, "idle", bots[0].Status)

	// Stop master to simulate crash
	masterServer1.Stop()
	time.Sleep(1 * time.Second)

	// Start new master instance on same port
	masterServer2 := env.StartMasterOnPort(t, masterServer1.GetPort())
	
	// Bot should reconnect automatically
	require.Eventually(t, func() bool {
		bots, err := env.GetBots(masterURL)
		if err != nil {
			return false
		}
		return len(bots) == 1 && bots[0].Status == "idle"
	}, 10*time.Second, 100*time.Millisecond, "Bot did not reconnect")

	t.Log("Bot successfully reconnected to new master instance")
}

// TestBotLoadBalancing tests job distribution under load
func TestBotLoadBalancing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	masterServer := env.StartMaster(t)
	masterURL := masterServer.GetURL()

	// Start bots with different capabilities
	botConfigs := []struct {
		id           string
		capabilities []string
	}{
		{"fast-bot-1", []string{"afl++", "libfuzzer"}},
		{"fast-bot-2", []string{"afl++", "libfuzzer"}},
		{"slow-bot-1", []string{"afl++"}},
	}

	for _, bc := range botConfigs {
		botCfg := &common.BotConfig{
			ID:           bc.id,
			MasterURL:    masterURL,
			Capabilities: bc.capabilities,
			Timeouts: common.BotTimeouts{
				HeartbeatInterval: 500 * time.Millisecond,
			},
		}

		agent, err := bot.NewAgent(botCfg)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ready := make(chan bool)
		go agent.Start(ctx, ready)
		<-ready
	}

	// Create jobs requiring different capabilities
	campaign := env.CreateTestCampaign(t, masterURL, "Load Balance Test")
	
	// Create AFL++ jobs
	for i := 0; i < 6; i++ {
		job := &common.Job{
			CampaignID: campaign.ID,
			FuzzerType: "afl++",
			Duration:   1 * time.Second,
		}
		_, err := env.CreateJob(masterURL, job)
		require.NoError(t, err)
	}

	// Create LibFuzzer jobs (only fast bots can handle these)
	for i := 0; i < 4; i++ {
		job := &common.Job{
			CampaignID: campaign.ID,
			FuzzerType: "libfuzzer",
			Duration:   1 * time.Second,
		}
		_, err := env.CreateJob(masterURL, job)
		require.NoError(t, err)
	}

	// Wait for all jobs to complete
	time.Sleep(5 * time.Second)

	// Analyze job distribution
	stats, err := env.GetCampaignStats(masterURL, campaign.ID)
	require.NoError(t, err)
	
	assert.Equal(t, 10, stats.TotalJobs)
	assert.Equal(t, 10, stats.CompletedJobs)
	
	// Verify LibFuzzer jobs only went to capable bots
	libfuzzerJobs, err := env.GetJobsByFuzzer(masterURL, campaign.ID, "libfuzzer")
	require.NoError(t, err)
	
	for _, job := range libfuzzerJobs {
		assert.Contains(t, []string{"fast-bot-1", "fast-bot-2"}, job.BotID)
	}
}
*/