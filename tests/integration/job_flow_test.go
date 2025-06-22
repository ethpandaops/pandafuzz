package integration

import (
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/bot"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJobCreationAndAssignment tests job creation and assignment flow
func TestJobCreationAndAssignment(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create and register bot
	botClient, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer botClient.Close()

	_, err = botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)

	// Create a job
	job, err := env.CreateTestJob("test-fuzzing")
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusPending, job.Status)

	// Bot requests a job
	assignedJob, err := botClient.GetJob(env.botConfig.ID)
	require.NoError(t, err)
	require.NotNil(t, assignedJob)
	assert.Equal(t, job.ID, assignedJob.ID)
	assert.Equal(t, common.JobStatusAssigned, assignedJob.Status)
	assert.Equal(t, env.botConfig.ID, *assignedJob.AssignedBot)

	// Verify job status in database
	dbJob, err := env.state.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusAssigned, dbJob.Status)
	assert.Equal(t, env.botConfig.ID, *dbJob.AssignedBot)

	// Verify bot status
	dbBot, err := env.state.GetBot(env.botConfig.ID)
	require.NoError(t, err)
	assert.Equal(t, common.BotStatusBusy, dbBot.Status)
	assert.Equal(t, job.ID, *dbBot.CurrentJob)
}

// TestJobCompletion tests job completion flow
func TestJobCompletion(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create and register bot
	botClient, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer botClient.Close()

	_, err = botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)

	// Create and get job
	job, err := env.CreateTestJob("completion-test")
	require.NoError(t, err)

	assignedJob, err := botClient.GetJob(env.botConfig.ID)
	require.NoError(t, err)
	require.NotNil(t, assignedJob)

	// Complete the job
	err = botClient.CompleteJob(env.botConfig.ID, true, "Job completed successfully")
	require.NoError(t, err)

	// Verify job status
	dbJob, err := env.state.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusCompleted, dbJob.Status)
	assert.NotNil(t, dbJob.CompletedAt)

	// Verify bot is idle again
	dbBot, err := env.state.GetBot(env.botConfig.ID)
	require.NoError(t, err)
	assert.Equal(t, common.BotStatusIdle, dbBot.Status)
	assert.Nil(t, dbBot.CurrentJob)
}

// TestJobFailure tests job failure handling
func TestJobFailure(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create and register bot
	botClient, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer botClient.Close()

	_, err = botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)

	// Create and get job
	job, err := env.CreateTestJob("failure-test")
	require.NoError(t, err)

	assignedJob, err := botClient.GetJob(env.botConfig.ID)
	require.NoError(t, err)
	require.NotNil(t, assignedJob)

	// Fail the job
	err = botClient.CompleteJob(env.botConfig.ID, false, "Job failed with error")
	require.NoError(t, err)

	// Verify job status
	dbJob, err := env.state.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusFailed, dbJob.Status)
	assert.NotNil(t, dbJob.CompletedAt)
	assert.Contains(t, dbJob.Message, "Job failed with error")

	// Verify bot is idle again
	dbBot, err := env.state.GetBot(env.botConfig.ID)
	require.NoError(t, err)
	assert.Equal(t, common.BotStatusIdle, dbBot.Status)
	assert.Nil(t, dbBot.CurrentJob)
}

// TestJobTimeout tests job timeout handling
func TestJobTimeout(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Use very short timeout for testing
	env.masterConfig.Timeouts.JobExecution = 1 * time.Second
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create and register bot
	_, err = env.CreateTestBot(env.botConfig.ID)
	require.NoError(t, err)

	// Create job with short timeout
	job, err := env.CreateTestJob("timeout-test")
	require.NoError(t, err)
	job.TimeoutAt = time.Now().Add(1 * time.Second)
	err = env.state.SaveJob(job)
	require.NoError(t, err)

	// Assign job to bot
	job.Status = common.JobStatusAssigned
	job.AssignedBot = &env.botConfig.ID
	job.StartedAt = &time.Time{}
	*job.StartedAt = time.Now()
	err = env.state.SaveJob(job)
	require.NoError(t, err)

	// Update bot status
	bot, err := env.state.GetBot(env.botConfig.ID)
	require.NoError(t, err)
	bot.Status = common.BotStatusBusy
	bot.CurrentJob = &job.ID
	err = env.state.SaveBot(bot)
	require.NoError(t, err)

	// Wait for timeout
	time.Sleep(2 * time.Second)

	// Run timeout check
	env.timeoutMgr.CheckTimeouts()

	// Verify job is timed out
	dbJob, err := env.state.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusFailed, dbJob.Status)
	assert.Contains(t, dbJob.Message, "timeout")
}

// TestJobPriority tests job priority assignment
func TestJobPriority(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create jobs with different priorities
	highPriorityJob, err := env.CreateTestJob("high-priority")
	require.NoError(t, err)
	highPriorityJob.Priority = common.JobPriorityHigh
	err = env.state.SaveJob(highPriorityJob)
	require.NoError(t, err)

	normalPriorityJob, err := env.CreateTestJob("normal-priority")
	require.NoError(t, err)

	lowPriorityJob, err := env.CreateTestJob("low-priority")
	require.NoError(t, err)
	lowPriorityJob.Priority = common.JobPriorityLow
	err = env.state.SaveJob(lowPriorityJob)
	require.NoError(t, err)

	// Register bot and get jobs
	botClient, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer botClient.Close()

	_, err = botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)

	// First job should be high priority
	job1, err := botClient.GetJob(env.botConfig.ID)
	require.NoError(t, err)
	assert.Equal(t, highPriorityJob.ID, job1.ID)

	// Complete job
	err = botClient.CompleteJob(env.botConfig.ID, true, "Done")
	require.NoError(t, err)

	// Next job should be normal priority
	job2, err := botClient.GetJob(env.botConfig.ID)
	require.NoError(t, err)
	assert.Equal(t, normalPriorityJob.ID, job2.ID)
}

// TestMultipleBotsJobDistribution tests job distribution among multiple bots
func TestMultipleBotsJobDistribution(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create multiple jobs
	numJobs := 10
	jobs := make([]*common.Job, numJobs)
	for i := 0; i < numJobs; i++ {
		job, err := env.CreateTestJob(fmt.Sprintf("job-%d", i))
		require.NoError(t, err)
		jobs[i] = job
	}

	// Create multiple bots
	numBots := 3
	botClients := make([]*bot.RetryClient, numBots)
	for i := 0; i < numBots; i++ {
		config := *env.botConfig
		config.ID = fmt.Sprintf("worker-%d", i)
		
		client, err := bot.NewRetryClient(&config)
		require.NoError(t, err)
		botClients[i] = client
		
		_, err = client.RegisterBot(config.ID, config.Capabilities)
		require.NoError(t, err)
	}

	// Each bot requests jobs
	assignedJobs := make(map[string]string) // job ID -> bot ID
	for i := 0; i < numBots; i++ {
		for j := 0; j < numJobs/numBots; j++ {
			job, err := botClients[i].GetJob(fmt.Sprintf("worker-%d", i))
			if err == nil && job != nil {
				assignedJobs[job.ID] = fmt.Sprintf("worker-%d", i)
			}
		}
	}

	// Verify jobs are distributed
	assert.Greater(t, len(assignedJobs), 0)
	
	// Check no duplicate assignments
	uniqueBots := make(map[string]int)
	for _, botID := range assignedJobs {
		uniqueBots[botID]++
	}
	
	// Each bot should have some jobs
	for i := 0; i < numBots; i++ {
		botID := fmt.Sprintf("worker-%d", i)
		assert.Greater(t, uniqueBots[botID], 0, "Bot %s should have jobs", botID)
	}

	// Cleanup
	for _, client := range botClients {
		client.Close()
	}
}

// TestJobCancellation tests job cancellation
func TestJobCancellation(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create and register bot
	botClient, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer botClient.Close()

	_, err = botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)

	// Create and get job
	job, err := env.CreateTestJob("cancel-test")
	require.NoError(t, err)

	assignedJob, err := botClient.GetJob(env.botConfig.ID)
	require.NoError(t, err)
	require.NotNil(t, assignedJob)

	// Cancel the job via API
	err = env.apiHandlers.CancelJob(job.ID)
	require.NoError(t, err)

	// Verify job status
	dbJob, err := env.state.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusCancelled, dbJob.Status)

	// Bot should be idle
	dbBot, err := env.state.GetBot(env.botConfig.ID)
	require.NoError(t, err)
	assert.Equal(t, common.BotStatusIdle, dbBot.Status)
}

// TestJobRetryOnBotFailure tests job retry when bot fails
func TestJobRetryOnBotFailure(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create first bot
	bot1Config := *env.botConfig
	bot1Config.ID = "bot-1"
	bot1Client, err := bot.NewRetryClient(&bot1Config)
	require.NoError(t, err)
	defer bot1Client.Close()

	_, err = bot1Client.RegisterBot(bot1Config.ID, bot1Config.Capabilities)
	require.NoError(t, err)

	// Create job
	job, err := env.CreateTestJob("retry-test")
	require.NoError(t, err)

	// Bot 1 gets the job
	assignedJob, err := bot1Client.GetJob(bot1Config.ID)
	require.NoError(t, err)
	require.NotNil(t, assignedJob)

	// Simulate bot 1 failure (mark as failed)
	err = env.recoveryMgr.HandleBotFailureWithRetry(bot1Config.ID)
	require.NoError(t, err)

	// Create second bot
	bot2Config := *env.botConfig
	bot2Config.ID = "bot-2"
	bot2Client, err := bot.NewRetryClient(&bot2Config)
	require.NoError(t, err)
	defer bot2Client.Close()

	_, err = bot2Client.RegisterBot(bot2Config.ID, bot2Config.Capabilities)
	require.NoError(t, err)

	// Bot 2 should get the same job
	reassignedJob, err := bot2Client.GetJob(bot2Config.ID)
	require.NoError(t, err)
	require.NotNil(t, reassignedJob)
	assert.Equal(t, job.ID, reassignedJob.ID)
	assert.Equal(t, bot2Config.ID, *reassignedJob.AssignedBot)
}

// TestJobFiltering tests job filtering by capabilities
func TestJobFiltering(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create AFL++ only job
	aflJob, err := env.CreateTestJob("afl-only")
	require.NoError(t, err)
	aflJob.Fuzzer = "afl++"
	err = env.state.SaveJob(aflJob)
	require.NoError(t, err)

	// Create LibFuzzer only job
	libfuzzerJob, err := env.CreateTestJob("libfuzzer-only")
	require.NoError(t, err)
	libfuzzerJob.Fuzzer = "libfuzzer"
	err = env.state.SaveJob(libfuzzerJob)
	require.NoError(t, err)

	// Create bot with only AFL++ capability
	aflBotConfig := *env.botConfig
	aflBotConfig.ID = "afl-bot"
	aflBotConfig.Capabilities = []string{"afl++"}
	
	aflClient, err := bot.NewRetryClient(&aflBotConfig)
	require.NoError(t, err)
	defer aflClient.Close()

	_, err = aflClient.RegisterBot(aflBotConfig.ID, aflBotConfig.Capabilities)
	require.NoError(t, err)

	// AFL bot should only get AFL job
	job, err := aflClient.GetJob(aflBotConfig.ID)
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, aflJob.ID, job.ID)

	// Complete the job
	err = aflClient.CompleteJob(aflBotConfig.ID, true, "Done")
	require.NoError(t, err)

	// Should not get LibFuzzer job
	job, err = aflClient.GetJob(aflBotConfig.ID)
	require.NoError(t, err)
	assert.Nil(t, job)
}