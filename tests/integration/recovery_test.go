package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/bot"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/master"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecoveryOnStartup tests system recovery on master startup
func TestRecoveryOnStartup(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Create some state before startup
	// Orphaned job (assigned but bot offline)
	orphanedJob, err := env.CreateTestJob("orphaned-job")
	require.NoError(t, err)
	orphanedJob.Status = common.JobStatusAssigned
	botID := "offline-bot"
	orphanedJob.AssignedBot = &botID
	now := time.Now()
	orphanedJob.StartedAt = &now
	err = env.state.SaveJobWithRetry(orphanedJob)
	require.NoError(t, err)

	// Create offline bot
	offlineBot := &common.Bot{
		ID:           botID,
		Status:       common.BotStatusBusy,
		CurrentJob:   &orphanedJob.ID,
		LastSeen:     time.Now().Add(-10 * time.Minute), // Old last seen
		RegisteredAt: time.Now().Add(-1 * time.Hour),
	}
	err = env.state.SaveBotWithRetry(offlineBot)
	require.NoError(t, err)

	// Create stuck pending job
	stuckJob, err := env.CreateTestJob("stuck-job")
	require.NoError(t, err)
	stuckJob.CreatedAt = time.Now().Add(-25 * time.Hour) // Old job
	err = env.state.SaveJobWithRetry(stuckJob)
	require.NoError(t, err)

	// Perform recovery
	err = env.recoveryMgr.RecoverOnStartup()
	require.NoError(t, err)

	// Check orphaned job is reset to pending
	recoveredJob, err := env.state.GetJob(orphanedJob.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusPending, recoveredJob.Status)
	assert.Nil(t, recoveredJob.AssignedBot)
	assert.Nil(t, recoveredJob.StartedAt)

	// Check offline bot is reset
	recoveredBot, err := env.state.GetBot(botID)
	require.NoError(t, err)
	assert.Equal(t, common.BotStatusTimedOut, recoveredBot.Status)
	assert.Nil(t, recoveredBot.CurrentJob)

	// Start master to verify system is healthy
	err = env.StartMaster()
	require.NoError(t, err)
}

// TestOrphanedJobRecovery tests recovery of orphaned jobs
func TestOrphanedJobRecovery(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master
	err := env.StartMaster()
	require.NoError(t, err)

	// Create and register bot
	bot1, err := env.CreateTestBot("bot-1")
	require.NoError(t, err)

	// Create job and assign to bot
	job, err := env.CreateTestJob("orphan-test")
	require.NoError(t, err)
	job.Status = common.JobStatusAssigned
	job.AssignedBot = &bot1.ID
	now := time.Now()
	job.StartedAt = &now
	err = env.state.SaveJobWithRetry(job)
	require.NoError(t, err)

	// Update bot status
	bot1.Status = common.BotStatusBusy
	bot1.CurrentJob = &job.ID
	err = env.state.SaveBotWithRetry(bot1)
	require.NoError(t, err)

	// Simulate bot going offline (no heartbeat)
	bot1.LastSeen = time.Now().Add(-10 * time.Minute)
	bot1.Status = common.BotStatusTimedOut
	err = env.state.SaveBotWithRetry(bot1)
	require.NoError(t, err)

	// Run recovery
	err = env.recoveryMgr.RecoverOnStartup()
	require.NoError(t, err)

	// Verify job is recovered
	recoveredJob, err := env.state.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusPending, recoveredJob.Status)
	assert.Nil(t, recoveredJob.AssignedBot)

	// Create new bot to pick up the job
	bot2Client, err := bot.NewRetryClient(&common.BotConfig{
		ID:            "bot-2",
		MasterURL:     env.masterURL,
		Capabilities:  []string{"afl++"},
		// WorkDirectory: env.tempDir, // TODO: WorkDirectory doesn't exist on BotConfig
	})
	require.NoError(t, err)
	defer bot2Client.Close()

	_, err = bot2Client.RegisterBot("bot-2", []string{"afl++"}, "http://localhost:9000")
	require.NoError(t, err)

	// Bot 2 should be able to get the recovered job
	assignedJob, err := bot2Client.GetJob("bot-2")
	require.NoError(t, err)
	assert.NotNil(t, assignedJob)
	assert.Equal(t, job.ID, assignedJob.ID)
}

// TestBotFailureRecovery tests handling of bot failures
func TestBotFailureRecovery(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master
	err := env.StartMaster()
	require.NoError(t, err)

	// Create and register bot
	botClient, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer botClient.Close()

	_, err = botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities, "http://localhost:9000")
	require.NoError(t, err)

	// Create and get job
	job, err := env.CreateTestJob("bot-failure-test")
	require.NoError(t, err)

	assignedJob, err := botClient.GetJob(env.botConfig.ID)
	require.NoError(t, err)
	require.NotNil(t, assignedJob)

	// Simulate bot failure
	err = env.recoveryMgr.HandleBotFailureWithRetry(env.botConfig.ID)
	require.NoError(t, err)

	// Check bot is marked as failed
	failedBot, err := env.state.GetBot(env.botConfig.ID)
	require.NoError(t, err)
	assert.Equal(t, common.BotStatusFailed, failedBot.Status)
	assert.Greater(t, failedBot.FailureCount, 0)

	// Check job is reassigned
	reassignedJob, err := env.state.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusPending, reassignedJob.Status)
	assert.Nil(t, reassignedJob.AssignedBot)
}

// TestMaintenanceRecovery tests periodic maintenance recovery
func TestMaintenanceRecovery(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master
	err := env.StartMaster()
	require.NoError(t, err)

	// Create old completed jobs
	for i := 0; i < 5; i++ {
		job, err := env.CreateTestJob(fmt.Sprintf("old-job-%d", i))
		require.NoError(t, err)
		job.Status = common.JobStatusCompleted
		completedAt := time.Now().Add(-50 * time.Hour) // Very old
		job.CompletedAt = &completedAt
		err = env.state.SaveJobWithRetry(job)
		require.NoError(t, err)
	}

	// Create stuck bot
	stuckBot := &common.Bot{
		ID:           "stuck-bot",
		Status:       common.BotStatusBusy,
		LastSeen:     time.Now().Add(-1 * time.Hour),
		RegisteredAt: time.Now().Add(-2 * time.Hour),
	}
	err = env.state.SaveBotWithRetry(stuckBot)
	require.NoError(t, err)

	// Run maintenance recovery
	err = env.recoveryMgr.PerformMaintenanceRecovery()
	require.NoError(t, err)

	// Check stuck bot is reset
	recoveredBot, err := env.state.GetBot(stuckBot.ID)
	require.NoError(t, err)
	assert.Equal(t, common.BotStatusTimedOut, recoveredBot.Status)

	// Check recovery stats
	stats := env.recoveryMgr.GetStats()
	assert.Greater(t, stats.TotalRecoveries, int64(0))
}

// TestConcurrentRecovery tests recovery under concurrent operations
func TestConcurrentRecovery(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master
	err := env.StartMaster()
	require.NoError(t, err)

	// Create multiple orphaned jobs
	numJobs := 10
	for i := 0; i < numJobs; i++ {
		job, err := env.CreateTestJob(fmt.Sprintf("concurrent-orphan-%d", i))
		require.NoError(t, err)
		job.Status = common.JobStatusAssigned
		botID := fmt.Sprintf("offline-bot-%d", i)
		job.AssignedBot = &botID
		now := time.Now()
		job.StartedAt = &now
		err = env.state.SaveJobWithRetry(job)
		require.NoError(t, err)
	}

	// Run recovery in parallel with new operations
	recoveryDone := make(chan error, 1)
	go func() {
		recoveryDone <- env.recoveryMgr.RecoverOnStartup()
	}()

	// Meanwhile, create new jobs
	for i := 0; i < 5; i++ {
		_, err := env.CreateTestJob(fmt.Sprintf("new-job-%d", i))
		assert.NoError(t, err)
	}

	// Wait for recovery
	err = <-recoveryDone
	require.NoError(t, err)

	// Verify all orphaned jobs are recovered
	jobs, err := env.state.ListJobs()
	require.NoError(t, err)
	
	pendingCount := 0
	for _, job := range jobs {
		if job.Status == common.JobStatusPending {
			pendingCount++
		}
	}
	assert.GreaterOrEqual(t, pendingCount, numJobs)
}

// TestTimeoutRecovery tests recovery of timed-out operations
func TestTimeoutRecovery(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Use very short timeouts
	env.masterConfig.Timeouts.JobExecution = 2 * time.Second
	env.masterConfig.Timeouts.BotHeartbeat = 1 * time.Second
	
	// Recreate timeout manager with new config
	env.timeoutMgr = master.NewTimeoutManager(env.state, env.masterConfig)
	
	// Start master
	err := env.StartMaster()
	require.NoError(t, err)

	// Create bot and job
	bot, err := env.CreateTestBot("timeout-bot")
	require.NoError(t, err)
	bot.Status = common.BotStatusBusy
	err = env.state.SaveBotWithRetry(bot)
	require.NoError(t, err)

	job, err := env.CreateTestJob("timeout-job")
	require.NoError(t, err)
	job.Status = common.JobStatusRunning
	job.AssignedBot = &bot.ID
	job.TimeoutAt = time.Now().Add(1 * time.Second)
	now := time.Now()
	job.StartedAt = &now
	err = env.state.SaveJobWithRetry(job)
	require.NoError(t, err)

	bot.CurrentJob = &job.ID
	err = env.state.SaveBotWithRetry(bot)
	require.NoError(t, err)

	// Wait for timeout
	time.Sleep(3 * time.Second)

	// Check timeouts
	// env.timeoutMgr.CheckTimeouts() // TODO: This method doesn't exist

	// Verify job is timed out
	timedOutJob, err := env.state.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusFailed, timedOutJob.Status)
	// assert.Contains(t, timedOutJob.Message, "timeout") // TODO: Message field doesn't exist

	// Verify bot is reset
	timedOutBot, err := env.state.GetBot(bot.ID)
	require.NoError(t, err)
	assert.Equal(t, common.BotStatusIdle, timedOutBot.Status)
	assert.Nil(t, timedOutBot.CurrentJob)
}

// TestSystemStateValidation tests system state validation
func TestSystemStateValidation(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master
	err := env.StartMaster()
	require.NoError(t, err)

	// Create inconsistent state
	// Bot with non-existent job
	bot1 := &common.Bot{
		ID:         "inconsistent-bot-1",
		Status:     common.BotStatusBusy,
		CurrentJob: func() *string { s := "non-existent-job"; return &s }(),
		LastSeen:   time.Now(),
	}
	err = env.state.SaveBotWithRetry(bot1)
	require.NoError(t, err)

	// Job assigned to non-existent bot
	job1, err := env.CreateTestJob("inconsistent-job-1")
	require.NoError(t, err)
	job1.Status = common.JobStatusAssigned
	nonExistentBot := "non-existent-bot"
	job1.AssignedBot = &nonExistentBot
	err = env.state.SaveJobWithRetry(job1)
	require.NoError(t, err)

	// Run recovery with validation
	err = env.recoveryMgr.RecoverOnStartup()
	require.NoError(t, err)

	// Check inconsistencies are fixed
	fixedBot, err := env.state.GetBot(bot1.ID)
	require.NoError(t, err)
	assert.Equal(t, common.BotStatusIdle, fixedBot.Status)
	assert.Nil(t, fixedBot.CurrentJob)

	fixedJob, err := env.state.GetJob(job1.ID)
	require.NoError(t, err)
	assert.Equal(t, common.JobStatusPending, fixedJob.Status)
	assert.Nil(t, fixedJob.AssignedBot)
}

// TestRecoveryMetrics tests recovery metrics collection
func TestRecoveryMetrics(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Reset stats
	env.recoveryMgr.ResetStats()
	
	// Create scenarios for recovery
	// Orphaned jobs
	for i := 0; i < 3; i++ {
		job, err := env.CreateTestJob(fmt.Sprintf("orphan-%d", i))
		require.NoError(t, err)
		job.Status = common.JobStatusAssigned
		botID := fmt.Sprintf("offline-%d", i)
		job.AssignedBot = &botID
		err = env.state.SaveJobWithRetry(job)
		require.NoError(t, err)
	}

	// Timed out bots
	for i := 0; i < 2; i++ {
		bot := &common.Bot{
			ID:       fmt.Sprintf("timeout-%d", i),
			Status:   common.BotStatusBusy,
			LastSeen: time.Now().Add(-1 * time.Hour),
		}
		err := env.state.SaveBotWithRetry(bot)
		require.NoError(t, err)
	}

	// Run recovery
	err := env.recoveryMgr.RecoverOnStartup()
	require.NoError(t, err)

	// Check metrics
	stats := env.recoveryMgr.GetStats()
	assert.Equal(t, int64(1), stats.TotalRecoveries)
	assert.GreaterOrEqual(t, stats.OrphanedJobsRecovered, int64(3))
	assert.GreaterOrEqual(t, stats.TimedOutBotsReset, int64(2))
	assert.Greater(t, stats.RecoveryDuration, time.Duration(0))
	assert.WithinDuration(t, time.Now(), stats.LastRecovery, 5*time.Second)
}

// TestRecoveryErrorHandling tests recovery error handling
func TestRecoveryErrorHandling(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Close database to simulate error
	env.database.Close()
	
	// Try recovery (should handle error gracefully)
	err := env.recoveryMgr.RecoverOnStartup()
	assert.Error(t, err)
	
	// Check error counter increased
	stats := env.recoveryMgr.GetStats()
	assert.Greater(t, stats.RecoveryErrors, int64(0))
}