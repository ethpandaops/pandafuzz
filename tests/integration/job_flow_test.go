package integration

import (
	"context"
	"fmt"
	"net/http"
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

	// Create and register bot - use the returned bot ID for subsequent operations
	botClient, err := bot.NewRetryClient(env.botConfig, env.logger)
	require.NoError(t, err)
	defer botClient.Close()

	regResponse, err := botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities, "http://localhost:9000")
	require.NoError(t, err)
	botID := regResponse.BotID

	// Create a job
	job, err := env.CreateTestJob("test-fuzzing")
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusPending, job.Status)

	// Bot requests a job using the registered bot ID
	assignedJob, err := botClient.GetJob(botID)
	require.NoError(t, err)
	require.NotNil(t, assignedJob)
	assert.Equal(t, job.ID, assignedJob.ID)
	assert.Equal(t, common.JobStatusAssigned, assignedJob.Status)
	assert.Equal(t, botID, *assignedJob.AssignedBot)

	// Verify job status in database
	dbJob, err := env.state.GetJob(context.Background(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusAssigned, dbJob.Status)
	assert.Equal(t, botID, *dbJob.AssignedBot)

	// Verify bot status using the registered bot ID
	dbBot, err := env.state.GetBot(context.Background(), botID)
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

	// Create and register bot - use the returned bot ID
	botClient, err := bot.NewRetryClient(env.botConfig, env.logger)
	require.NoError(t, err)
	defer botClient.Close()

	regResponse, err := botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities, "http://localhost:9000")
	require.NoError(t, err)
	botID := regResponse.BotID

	// Create and get job
	job, err := env.CreateTestJob("completion-test")
	require.NoError(t, err)

	assignedJob, err := botClient.GetJob(botID)
	require.NoError(t, err)
	require.NotNil(t, assignedJob)

	// Complete the job
	err = botClient.CompleteJob(botID, assignedJob.ID, true, "Job completed successfully")
	require.NoError(t, err)

	// Verify job status
	dbJob, err := env.state.GetJob(context.Background(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusCompleted, dbJob.Status)
	assert.NotNil(t, dbJob.CompletedAt)

	// Verify bot is idle again
	dbBot, err := env.state.GetBot(context.Background(), botID)
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

	// Create and register bot - use the returned bot ID
	botClient, err := bot.NewRetryClient(env.botConfig, env.logger)
	require.NoError(t, err)
	defer botClient.Close()

	regResponse, err := botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities, "http://localhost:9000")
	require.NoError(t, err)
	botID := regResponse.BotID

	// Create and get job
	job, err := env.CreateTestJob("failure-test")
	require.NoError(t, err)

	assignedJob, err := botClient.GetJob(botID)
	require.NoError(t, err)
	require.NotNil(t, assignedJob)

	// Fail the job
	err = botClient.CompleteJob(botID, assignedJob.ID, false, "Job failed with error")
	require.NoError(t, err)

	// Verify job status
	dbJob, err := env.state.GetJob(context.Background(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusFailed, dbJob.Status)
	assert.NotNil(t, dbJob.CompletedAt)

	// Verify bot is idle again
	dbBot, err := env.state.GetBot(context.Background(), botID)
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

	// Create and register bot - use the returned bot ID
	createdBot, err := env.CreateTestBot(env.botConfig.ID)
	require.NoError(t, err)
	botID := createdBot.ID

	// Create job with short timeout
	job, err := env.CreateTestJob("timeout-test")
	require.NoError(t, err)
	job.TimeoutAt = time.Now().Add(1 * time.Second)
	err = env.state.SaveJobWithRetry(context.Background(), job)
	require.NoError(t, err)

	// Assign job to bot using the created bot's ID
	job.Status = common.JobStatusAssigned
	job.AssignedBot = &botID
	job.StartedAt = &time.Time{}
	*job.StartedAt = time.Now()
	err = env.state.SaveJobWithRetry(context.Background(), job)
	require.NoError(t, err)

	// Register job with timeout manager so it can detect the timeout
	env.timeoutMgr.SetJobTimeout(job.ID, 1*time.Second)

	// Update bot status using the created bot's ID
	dbBot, err := env.state.GetBot(context.Background(), botID)
	require.NoError(t, err)
	dbBot.Status = common.BotStatusBusy
	dbBot.CurrentJob = &job.ID
	err = env.state.SaveBotWithRetry(context.Background(), dbBot)
	require.NoError(t, err)

	// Wait for the timeout to expire
	time.Sleep(2 * time.Second)

	// Force a timeout check instead of waiting for the 30-second interval
	env.timeoutMgr.ForceTimeoutCheck()

	// Verify job is timed out
	dbJob, err := env.state.GetJob(context.Background(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusFailed, dbJob.Status)
}

// TestJobPriority tests job assignment in FIFO order (creation order)
func TestJobPriority(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create jobs in order - they should be assigned in FIFO order
	// CreateTestJob already saves the job, so no need for extra save calls
	firstJob, err := env.CreateTestJob("first-job")
	require.NoError(t, err)

	secondJob, err := env.CreateTestJob("second-job")
	require.NoError(t, err)

	thirdJob, err := env.CreateTestJob("third-job")
	require.NoError(t, err)
	_ = thirdJob // Will be used after second job completes

	// Register bot and get jobs - use the returned bot ID
	botClient, err := bot.NewRetryClient(env.botConfig, env.logger)
	require.NoError(t, err)
	defer botClient.Close()

	regResponse, err := botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities, "http://localhost:9000")
	require.NoError(t, err)
	botID := regResponse.BotID

	// First job should be the first one created
	job1, err := botClient.GetJob(botID)
	require.NoError(t, err)
	assert.Equal(t, firstJob.ID, job1.ID)

	// Complete job
	err = botClient.CompleteJob(botID, job1.ID, true, "Done")
	require.NoError(t, err)

	// Next job should be the second one created
	job2, err := botClient.GetJob(botID)
	require.NoError(t, err)
	assert.Equal(t, secondJob.ID, job2.ID)
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

	// Create multiple bots - track returned bot IDs
	numBots := 3
	botClients := make([]*bot.RetryClient, numBots)
	botIDs := make([]string, numBots)
	for i := 0; i < numBots; i++ {
		config := *env.botConfig
		config.ID = fmt.Sprintf("worker-%d", i)

		client, err := bot.NewRetryClient(&config, env.logger)
		require.NoError(t, err)
		botClients[i] = client

		regResponse, err := client.RegisterBot(config.ID, config.Capabilities, "http://localhost:9000")
		require.NoError(t, err)
		botIDs[i] = regResponse.BotID
	}

	// Each bot requests jobs using returned bot IDs
	assignedJobs := make(map[string]string) // job ID -> bot ID
	for i := 0; i < numBots; i++ {
		for j := 0; j < numJobs/numBots; j++ {
			job, err := botClients[i].GetJob(botIDs[i])
			if err == nil && job != nil {
				assignedJobs[job.ID] = botIDs[i]
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
		assert.Greater(t, uniqueBots[botIDs[i]], 0, "Bot %s should have jobs", botIDs[i])
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

	// Create and register bot - use returned bot ID
	botClient, err := bot.NewRetryClient(env.botConfig, env.logger)
	require.NoError(t, err)
	defer botClient.Close()

	regResponse, err := botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities, "http://localhost:9000")
	require.NoError(t, err)
	botID := regResponse.BotID

	// Create and get job
	job, err := env.CreateTestJob("cancel-test")
	require.NoError(t, err)

	assignedJob, err := botClient.GetJob(botID)
	require.NoError(t, err)
	require.NotNil(t, assignedJob)

	// Cancel the job via HTTP API
	cancelReq, err := http.NewRequest("POST", env.masterURL+"/api/v1/jobs/"+job.ID+"/cancel", nil)
	require.NoError(t, err)
	cancelResp, err := env.httpClient.Do(cancelReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, cancelResp.StatusCode)
	cancelResp.Body.Close()

	// Verify job status
	dbJob, err := env.state.GetJob(context.Background(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusCancelled, dbJob.Status)

	// Bot should be idle
	dbBot, err := env.state.GetBot(context.Background(), botID)
	require.NoError(t, err)
	assert.Equal(t, common.BotStatusIdle, dbBot.Status)
}

// TestJobRetryOnBotFailure tests job retry when bot fails
func TestJobRetryOnBotFailure(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create first bot - use returned bot ID
	bot1Config := *env.botConfig
	bot1Config.ID = "bot-1"
	bot1Client, err := bot.NewRetryClient(&bot1Config, env.logger)
	require.NoError(t, err)
	defer bot1Client.Close()

	bot1Response, err := bot1Client.RegisterBot(bot1Config.ID, bot1Config.Capabilities, "http://localhost:9000")
	require.NoError(t, err)
	bot1ID := bot1Response.BotID

	// Create job
	job, err := env.CreateTestJob("retry-test")
	require.NoError(t, err)

	// Bot 1 gets the job using returned bot ID
	assignedJob, err := bot1Client.GetJob(bot1ID)
	require.NoError(t, err)
	require.NotNil(t, assignedJob)

	// Simulate bot 1 failure (mark as failed) using returned bot ID
	err = env.recoveryMgr.HandleBotFailureWithRetry(context.Background(), bot1ID)
	require.NoError(t, err)

	// Create second bot - use returned bot ID
	bot2Config := *env.botConfig
	bot2Config.ID = "bot-2"
	bot2Client, err := bot.NewRetryClient(&bot2Config, env.logger)
	require.NoError(t, err)
	defer bot2Client.Close()

	bot2Response, err := bot2Client.RegisterBot(bot2Config.ID, bot2Config.Capabilities, "http://localhost:9000")
	require.NoError(t, err)
	bot2ID := bot2Response.BotID

	// Bot 2 should get the same job
	reassignedJob, err := bot2Client.GetJob(bot2ID)
	require.NoError(t, err)
	require.NotNil(t, reassignedJob)
	assert.Equal(t, job.ID, reassignedJob.ID)
	assert.Equal(t, bot2ID, *reassignedJob.AssignedBot)
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
	err = env.state.SaveJobWithRetry(context.Background(), aflJob)
	require.NoError(t, err)

	// Create LibFuzzer only job
	libfuzzerJob, err := env.CreateTestJob("libfuzzer-only")
	require.NoError(t, err)
	libfuzzerJob.Fuzzer = "libfuzzer"
	err = env.state.SaveJobWithRetry(context.Background(), libfuzzerJob)
	require.NoError(t, err)

	// Create bot with only AFL++ capability - use returned bot ID
	aflBotConfig := *env.botConfig
	aflBotConfig.ID = "afl-bot"
	aflBotConfig.Capabilities = []string{"afl++"}

	aflClient, err := bot.NewRetryClient(&aflBotConfig, env.logger)
	require.NoError(t, err)
	defer aflClient.Close()

	regResponse, err := aflClient.RegisterBot(aflBotConfig.ID, aflBotConfig.Capabilities, "http://localhost:9000")
	require.NoError(t, err)
	aflBotID := regResponse.BotID

	// AFL bot should only get AFL job
	job, err := aflClient.GetJob(aflBotID)
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, aflJob.ID, job.ID)

	// Complete the job
	err = aflClient.CompleteJob(aflBotID, job.ID, true, "Done")
	require.NoError(t, err)

	// Should not get LibFuzzer job
	job, err = aflClient.GetJob(aflBotID)
	require.NoError(t, err)
	assert.Nil(t, job)
}
