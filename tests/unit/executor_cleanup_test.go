package unit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/bot"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecutorCleanupAfterJobCompletion tests that CleanupJob properly removes
// the fuzzer from active jobs and cleans up resources
func TestExecutorCleanupAfterJobCompletion(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()

	// Create logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create bot config with minimal required fields
	botConfig := &common.BotConfig{
		ID:        "test-bot",
		MasterURL: "http://localhost:8080",
		Fuzzing: common.FuzzingConfig{
			WorkDir: tempDir,
			MaxJobs: 1,
		},
	}

	// Create fuzzer executor
	executor := bot.NewFuzzerJobExecutor(botConfig, logger)

	// Create a simple test job
	job := &common.Job{
		ID:      "test-job-123",
		Name:    "Test Job",
		Fuzzer:  "libfuzzer",
		Target:  "/bin/echo",
		WorkDir: filepath.Join(tempDir, "test-job-123"),
		Config: common.JobConfig{
			Duration:    1 * time.Second,
			Timeout:     1000,
			MemoryLimit: 512,
		},
		Status: common.JobStatusPending,
	}

	// Create work directory
	require.NoError(t, os.MkdirAll(job.WorkDir, 0755))

	// Start job execution in a goroutine
	_, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var execSuccess bool
	var execMessage string
	var execErr error

	go func() {
		// The job will start and run briefly
		execSuccess, execMessage, execErr = executor.ExecuteJob(job)
		close(done)
	}()

	// Wait a moment for the job to initialize
	time.Sleep(200 * time.Millisecond)

	// The fuzzer should exist while the job is running
	_, exists := executor.GetFuzzer(job.ID)
	assert.True(t, exists, "Fuzzer should exist while job is running")

	// Cancel context to stop the job
	cancel()

	// Wait for execution to complete
	<-done

	t.Logf("Job completed: success=%v, message=%s, err=%v", execSuccess, execMessage, execErr)

	// IMPORTANT: After ExecuteJob returns, the fuzzer should STILL exist
	// This is the fix - we no longer clean up in the deferred function
	fuzz, exists := executor.GetFuzzer(job.ID)
	assert.True(t, exists, "Fuzzer should still exist after ExecuteJob returns (before explicit cleanup)")

	if exists {
		// We can still access the fuzzer for crash detection
		status := fuzz.GetStatus()
		t.Logf("Fuzzer status after job completion: %s", status)
	}

	// Now explicitly clean up the job
	err := executor.CleanupJob(job.ID)
	assert.NoError(t, err, "CleanupJob should succeed")

	// After cleanup, fuzzer should be gone
	_, exists = executor.GetFuzzer(job.ID)
	assert.False(t, exists, "Fuzzer should not exist after CleanupJob")

	// Cleanup again should not error (idempotent)
	err = executor.CleanupJob(job.ID)
	assert.NoError(t, err, "CleanupJob should be idempotent")
}

// TestCleanupNonExistentJob tests that cleaning up a non-existent job doesn't error
func TestCleanupNonExistentJob(t *testing.T) {
	// Create logger
	logger := logrus.New()

	// Create bot config
	botConfig := &common.BotConfig{
		ID:        "test-bot",
		MasterURL: "http://localhost:8080",
		Fuzzing: common.FuzzingConfig{
			WorkDir: t.TempDir(),
		},
	}

	// Create fuzzer executor
	executor := bot.NewFuzzerJobExecutor(botConfig, logger)

	// Try to cleanup non-existent job
	err := executor.CleanupJob("non-existent-job")
	assert.NoError(t, err, "Cleaning up non-existent job should not error")
}
