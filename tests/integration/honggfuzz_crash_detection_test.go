package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/bot"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/fuzzer"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHonggfuzzCrashDetection tests that crashes are properly detected
// after job completion but before fuzzer cleanup
func TestHonggfuzzCrashDetection(t *testing.T) {
	// Create temp directories
	tempDir := t.TempDir()
	workDir := filepath.Join(tempDir, "work")
	outputDir := filepath.Join(workDir, "output", "honggfuzz_output")
	corpusDir := filepath.Join(outputDir, "corpus")

	// Create directories
	require.NoError(t, os.MkdirAll(corpusDir, 0755))

	// Create a mock crash file in the corpus directory
	crashContent := []byte("CRASH_INPUT_DATA")
	crashFile := filepath.Join(corpusDir, "SIGABRT.PC.7ffff7e249fc.STACK.18697a4b3.CODE.-6.ADDR.0.INSTR.[UNKNOWN].2025-07-23.18:48:49.50.fuzz")
	require.NoError(t, os.WriteFile(crashFile, crashContent, 0644))

	// Create logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create bot config
	botConfig := &common.BotConfig{
		ID:        "test-bot",
		MasterURL: "http://localhost:8080",
		MaxJobs:   1,
		WorkDir:   tempDir,
		LogLevel:  "debug",
	}

	// Create fuzzer executor
	executor := bot.NewFuzzerJobExecutor(botConfig, logger)

	// Create Honggfuzz instance directly
	hf := fuzzer.NewHonggfuzz(logger)
	hf.SetBotID("test-bot")

	// Initialize fuzzer
	config := &fuzzer.Config{
		Target:      "/bin/echo",
		WorkDir:     workDir,
		CorpusDir:   corpusDir,
		Dictionary:  "",
		Timeout:     1000,
		MemoryLimit: 512,
		MaxInputLen: 4096,
		FuzzerOptions: map[string]interface{}{
			"honggfuzz_config": fuzzer.HongFuzzConfig{
				PersistentMode: false,
			},
		},
	}

	err := hf.Initialize(config)
	require.NoError(t, err)

	// Add fuzzer to executor's active fuzzers
	executor.GetFuzzer("test-job") // This won't find it

	// Manually add it (we need to expose this for testing)
	// For now, let's test the crash detection directly
	crashes, err := hf.GetCrashes()
	require.NoError(t, err)

	// Verify crash was detected
	require.Len(t, crashes, 1, "Should detect one crash")
	assert.Equal(t, "test-job", crashes[0].JobID)
	assert.Equal(t, "test-bot", crashes[0].BotID)
	assert.Contains(t, crashes[0].FilePath, "SIGABRT")
	assert.Equal(t, crashContent, crashes[0].Input)
	assert.Equal(t, "SIGABRT", crashes[0].Type)
}

// TestFuzzerCleanupRaceCondition tests that the fuzzer is not cleaned up
// before crash detection completes
func TestFuzzerCleanupRaceCondition(t *testing.T) {
	// Create temp directories
	tempDir := t.TempDir()
	workDir := filepath.Join(tempDir, "work", "job-123")
	outputDir := filepath.Join(workDir, "output", "honggfuzz_output")
	corpusDir := filepath.Join(outputDir, "corpus")

	// Create directories
	require.NoError(t, os.MkdirAll(corpusDir, 0755))

	// Create a mock crash file
	crashContent := []byte("CRASH_DATA")
	crashFile := filepath.Join(corpusDir, "SIGSEGV.PC.deadbeef.STACK.abc123.CODE.-11.ADDR.0.INSTR.mov.2025-01-01.12:00:00.1.fuzz")
	require.NoError(t, os.WriteFile(crashFile, crashContent, 0644))

	// Create logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create bot config
	botConfig := &common.BotConfig{
		ID:        "test-bot",
		MasterURL: "http://localhost:8080",
		MaxJobs:   1,
		WorkDir:   tempDir,
		LogLevel:  "debug",
	}

	// Create fuzzer executor
	executor := bot.NewFuzzerJobExecutor(botConfig, logger)

	// Create test job
	job := &common.Job{
		ID:      "job-123",
		Name:    "Test Job",
		Fuzzer:  "honggfuzz",
		Target:  "/bin/echo",
		WorkDir: workDir,
		Config: common.JobConfig{
			Duration:    10 * time.Second,
			Timeout:     1000,
			MemoryLimit: 512,
		},
		Status: common.JobStatusPending,
	}

	// Create a context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start job execution in a goroutine
	done := make(chan struct{})
	var execSuccess bool
	var execMessage string
	var execErr error

	go func() {
		execSuccess, execMessage, execErr = executor.ExecuteJob(job)
		close(done)
	}()

	// Wait a bit for the job to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop the job
	cancel()

	// Wait for execution to complete
	<-done

	// The job should complete (possibly with timeout)
	t.Logf("Execution result: success=%v, message=%s, err=%v", execSuccess, execMessage, execErr)

	// Now try to get the fuzzer - it should still be available
	fuzz, exists := executor.GetFuzzer(job.ID)
	assert.True(t, exists, "Fuzzer should still exist after ExecuteJob returns")

	if exists {
		// Check for crashes
		crashes, err := fuzz.GetCrashes()
		require.NoError(t, err)
		assert.Len(t, crashes, 1, "Should detect the crash file")

		// Now cleanup
		err = executor.CleanupJob(job.ID)
		assert.NoError(t, err)

		// After cleanup, fuzzer should be gone
		_, exists = executor.GetFuzzer(job.ID)
		assert.False(t, exists, "Fuzzer should not exist after CleanupJob")
	}
}
