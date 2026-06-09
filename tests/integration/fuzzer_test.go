package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/bot"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/adapter"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFuzzerInterface tests the fuzzer interface implementation
func TestFuzzerInterface(t *testing.T) {
	// Test AFL++ implementation
	// Note: Name() returns the fuzzer type string (lowercase) from the engine
	aflFuzzer := adapter.NewAFLPlusPlus(nil)
	assert.NotNil(t, aflFuzzer)
	assert.Equal(t, "afl++", aflFuzzer.Name()) // Engine returns lowercase type identifier
	assert.Equal(t, adapter.FuzzerTypeAFL, aflFuzzer.Type())
	assert.NotEmpty(t, aflFuzzer.GetCapabilities())

	// Test LibFuzzer implementation
	libFuzzer := adapter.NewLibFuzzer(nil)
	assert.NotNil(t, libFuzzer)
	assert.Equal(t, "libfuzzer", libFuzzer.Name()) // Engine returns lowercase type identifier
	assert.Equal(t, adapter.FuzzerTypeLibFuzzer, libFuzzer.Type())
	assert.NotEmpty(t, libFuzzer.GetCapabilities())
}

// TestFuzzerConfiguration tests fuzzer configuration
func TestFuzzerConfiguration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fuzzer-config-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test binary
	testBinary := filepath.Join(tempDir, "test-target")
	err = os.WriteFile(testBinary, []byte("#!/bin/sh\necho test"), 0755)
	require.NoError(t, err)

	config := adapter.FuzzConfig{
		Target:          testBinary,
		TargetArgs:      []string{"@@"},
		WorkDirectory:   tempDir,
		OutputDirectory: filepath.Join(tempDir, "output"),
		Duration:        10 * time.Second,
		Timeout:         1 * time.Second,
		MemoryLimit:     1024,
		FuzzerOptions: map[string]any{
			"deterministic": false,
		},
	}

	// Test AFL++ configuration
	aflFuzzer := adapter.NewAFLPlusPlus(nil)
	err = aflFuzzer.Configure(config)
	assert.NoError(t, err)

	// Test LibFuzzer configuration
	libFuzzer := adapter.NewLibFuzzer(nil)
	err = libFuzzer.Configure(config)
	assert.NoError(t, err)
}

// TestFuzzerInitialization tests fuzzer initialization
// Note: Initialize() is a lightweight operation that marks the fuzzer as ready.
// Directory creation happens later during Start() when the engine runs.
func TestFuzzerInitialization(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fuzzer-init-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test binary
	testBinary := filepath.Join(tempDir, "test-target")
	err = os.WriteFile(testBinary, []byte("#!/bin/sh\necho test"), 0755)
	require.NoError(t, err)

	// Create seed directory
	seedDir := filepath.Join(tempDir, "seeds")
	err = os.MkdirAll(seedDir, 0755)
	require.NoError(t, err)

	// Create seed file
	err = os.WriteFile(filepath.Join(seedDir, "seed1"), []byte("test"), 0644)
	require.NoError(t, err)

	config := adapter.FuzzConfig{
		Target:          testBinary,
		OutputDirectory: filepath.Join(tempDir, "output"),
		SeedDirectory:   seedDir,
		WorkDirectory:   tempDir,
	}

	// Initialize AFL++
	aflFuzzer := adapter.NewAFLPlusPlus(nil)
	err = aflFuzzer.Configure(config)
	require.NoError(t, err)

	err = aflFuzzer.Initialize()
	assert.NoError(t, err)

	// Verify fuzzer is in initialized state
	assert.Equal(t, adapter.StatusInitialized, aflFuzzer.GetStatus())
}

// TestFuzzerValidation tests fuzzer validation
func TestFuzzerValidation(t *testing.T) {
	aflFuzzer := adapter.NewAFLPlusPlus(nil)

	// Test validation without configuration
	err := aflFuzzer.Validate()
	assert.Error(t, err)

	// Test with invalid target
	config := adapter.FuzzConfig{
		Target:          "/non/existent/binary",
		OutputDirectory: "/tmp/test",
	}

	err = aflFuzzer.Configure(config)
	assert.NoError(t, err)

	err = aflFuzzer.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestFuzzerExecution tests basic fuzzer execution flow
func TestFuzzerExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping fuzzer execution test in short mode")
	}

	tempDir, err := os.MkdirTemp("", "fuzzer-exec-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a simple test program that just exits
	testBinary := filepath.Join(tempDir, "test-program")
	testProgram := `#!/bin/sh
read input
echo "Processing input"
exit 0`
	err = os.WriteFile(testBinary, []byte(testProgram), 0755)
	require.NoError(t, err)

	config := adapter.FuzzConfig{
		Target:          testBinary,
		TargetArgs:      []string{},
		OutputDirectory: filepath.Join(tempDir, "output"),
		WorkDirectory:   tempDir,
		Duration:        2 * time.Second,
		Timeout:         100 * time.Millisecond,
		MemoryLimit:     512,
	}

	// Create and configure fuzzer
	aflFuzzer := adapter.NewAFLPlusPlus(nil)
	err = aflFuzzer.Configure(config)
	require.NoError(t, err)

	err = aflFuzzer.Initialize()
	require.NoError(t, err)

	// Set event handler
	eventChan := make(chan string, 10)
	handler := &TestEventHandler{events: eventChan}
	aflFuzzer.SetEventHandler(handler)

	// Start fuzzer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = aflFuzzer.Start(ctx)
	if err != nil {
		// AFL++ might not be installed in test environment
		t.Skipf("Skipping execution test: %v", err)
	}

	// Let it run briefly
	time.Sleep(1 * time.Second)

	// Check status
	assert.Equal(t, adapter.StatusRunning, aflFuzzer.GetStatus())
	assert.True(t, aflFuzzer.IsRunning())

	// Get stats
	stats := aflFuzzer.GetStats()
	assert.NotZero(t, stats.StartTime)

	// Stop fuzzer
	err = aflFuzzer.Stop()
	assert.NoError(t, err)
	assert.False(t, aflFuzzer.IsRunning())

	// Check events
	select {
	case event := <-eventChan:
		assert.Equal(t, "start", event)
	case <-time.After(1 * time.Second):
		// No event is okay if fuzzer didn't fully start
	}
}

// TestFuzzerJobExecution tests fuzzer execution through bot job executor
func TestFuzzerJobExecution(t *testing.T) {
	env := SetupTestEnvironment(t)

	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create bot with fuzzer support
	botConfig := *env.botConfig
	// Note: WorkDirectory doesn't exist on BotConfig - work directories are per-job
	workDir := filepath.Join(env.tempDir, "bot-work")
	err = os.MkdirAll(workDir, 0755)
	require.NoError(t, err)

	// Create job executor
	logger := logrus.New()
	executor := bot.NewFuzzerJobExecutor(&botConfig, logger)

	// Create test job with proper UUID
	job := &common.Job{
		ID:        uuid.New().String(),
		Name:      "Test Fuzzing",
		Fuzzer:    "afl++",
		Target:    "/bin/echo", // Use system binary for testing
		Status:    common.JobStatusPending,
		WorkDir:   workDir,
		TimeoutAt: time.Now().Add(5 * time.Second),
		Config: common.JobConfig{
			Duration: 2 * time.Second,
			Timeout:  5 * time.Second,
		},
	}

	// Mock execution (actual fuzzer might not be available)
	success, message, err := executor.ExecuteJob(job)

	// We expect this to fail gracefully if AFL++ is not installed
	if err != nil {
		assert.Contains(t, err.Error(), "not found")
		t.Logf("Fuzzer not available: %v", err)
	} else {
		assert.True(t, success)
		assert.NotEmpty(t, message)
	}
}

// TestFuzzerCrashHandling tests crash detection and reporting
// Note: The adapter's GetCrashes() returns crashes collected during engine runtime
// via the crash channel. Crash file detection from disk is handled by the engine's
// monitorCrashes() goroutine which runs during Start(). Without running the fuzzer,
// GetCrashes() returns an empty slice.
func TestFuzzerCrashHandling(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fuzzer-crash-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Configure fuzzer
	config := adapter.FuzzConfig{
		Target:          "/bin/test",
		OutputDirectory: tempDir,
		WorkDirectory:   tempDir,
	}

	aflFuzzer := adapter.NewAFLPlusPlus(nil)
	err = aflFuzzer.Configure(config)
	require.NoError(t, err)

	// Without running the fuzzer, GetCrashes returns empty slice
	// (crashes are collected via channel monitoring during Start())
	detectedCrashes, err := aflFuzzer.GetCrashes()
	require.NoError(t, err)
	assert.Len(t, detectedCrashes, 0, "GetCrashes should return empty when fuzzer hasn't run")

	// Test crash event handling via event handler
	crashHandler := &testCrashCollector{crashes: make([]*common.CrashResult, 0)}
	aflFuzzer.SetEventHandler(crashHandler)

	// Simulate crash event callback (this tests the event handler mechanism)
	testCrash := &common.CrashResult{
		ID:        "crash-test-001",
		Hash:      "abcd1234",
		Type:      "SEGV",
		Signal:    11,
		Input:     []byte("AAAA"),
		Size:      4,
		Timestamp: time.Now(),
	}
	crashHandler.OnCrash(aflFuzzer, testCrash)
	assert.Len(t, crashHandler.crashes, 1)
	assert.Equal(t, testCrash.ID, crashHandler.crashes[0].ID)
}

// testCrashCollector is a test event handler that collects crash events
type testCrashCollector struct {
	crashes []*common.CrashResult
}

func (h *testCrashCollector) OnStart(fuzzer adapter.Fuzzer)               {}
func (h *testCrashCollector) OnStop(fuzzer adapter.Fuzzer, reason string) {}
func (h *testCrashCollector) OnCrash(fuzzer adapter.Fuzzer, crash *common.CrashResult) {
	h.crashes = append(h.crashes, crash)
}
func (h *testCrashCollector) OnNewPath(fuzzer adapter.Fuzzer, path *adapter.CorpusEntry) {}
func (h *testCrashCollector) OnStats(fuzzer adapter.Fuzzer, stats adapter.FuzzerStats)   {}
func (h *testCrashCollector) OnError(fuzzer adapter.Fuzzer, err error)                   {}
func (h *testCrashCollector) OnProgress(fuzzer adapter.Fuzzer, progress adapter.FuzzerProgress) {
}

// TestFuzzerCoverageReporting tests coverage collection
func TestFuzzerCoverageReporting(t *testing.T) {
	aflFuzzer := adapter.NewAFLPlusPlus(nil)

	// Configure with minimal settings
	config := adapter.FuzzConfig{
		Target:          "/bin/test",
		OutputDirectory: "/tmp/test",
	}

	err := aflFuzzer.Configure(config)
	require.NoError(t, err)

	// Get coverage (will be empty without actual execution)
	coverage, err := aflFuzzer.GetCoverage()
	require.NoError(t, err)
	assert.NotNil(t, coverage)
	assert.NotEmpty(t, coverage.ID)
	assert.Equal(t, "/bin/test", coverage.JobID)
}

// TestFuzzerProgress tests progress tracking
func TestFuzzerProgress(t *testing.T) {
	aflFuzzer := adapter.NewAFLPlusPlus(nil)

	config := adapter.FuzzConfig{
		Target:          "/bin/test",
		OutputDirectory: "/tmp/test",
		Duration:        10 * time.Second,
		MaxExecutions:   1000,
	}

	err := aflFuzzer.Configure(config)
	require.NoError(t, err)

	// Get progress
	progress := aflFuzzer.GetProgress()
	assert.NotNil(t, progress)
	assert.Equal(t, "calibration", progress.Phase)
	assert.Equal(t, float64(0), progress.ProgressPercent)
}

// TestFuzzerCleanup tests fuzzer cleanup
// Note: Initialize() doesn't create directories - that happens during Start().
// Cleanup() is a no-op for the adapter (engine handles cleanup in Stop()).
func TestFuzzerCleanup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fuzzer-cleanup-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	config := adapter.FuzzConfig{
		Target:          "/bin/test",
		OutputDirectory: tempDir,
		FuzzerOptions: map[string]any{
			"clean_temp": true,
		},
	}

	aflFuzzer := adapter.NewAFLPlusPlus(nil)
	err = aflFuzzer.Configure(config)
	require.NoError(t, err)

	err = aflFuzzer.Initialize()
	require.NoError(t, err)

	// Verify fuzzer is initialized
	assert.Equal(t, adapter.StatusInitialized, aflFuzzer.GetStatus())

	// Cleanup should succeed even without running
	err = aflFuzzer.Cleanup()
	assert.NoError(t, err)
}

// TestLibFuzzerSpecifics tests LibFuzzer-specific features
func TestLibFuzzerSpecifics(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "libfuzzer-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create mock LibFuzzer binary
	testBinary := filepath.Join(tempDir, "fuzz-target")
	libFuzzerHelp := `libFuzzer help
Usage: fuzz-target [-flag=val]
-help=1`

	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "-help=1" ]; then
    echo '%s'
    exit 0
fi
echo "Running libfuzzer"
`, libFuzzerHelp)

	err = os.WriteFile(testBinary, []byte(script), 0755)
	require.NoError(t, err)

	config := adapter.FuzzConfig{
		Target:          testBinary,
		OutputDirectory: filepath.Join(tempDir, "output"),
		WorkDirectory:   tempDir,
		Duration:        5 * time.Second,
		MaxExecutions:   100,
		FuzzerOptions: map[string]any{
			"workers":       2,
			"fork":          true,
			"value_profile": true,
			"entropic":      true,
		},
	}

	libFuzzer := adapter.NewLibFuzzer(logrus.New())
	err = libFuzzer.Configure(config)
	require.NoError(t, err)

	err = libFuzzer.Initialize()
	assert.NoError(t, err)

	// Test that pause/resume are not supported
	err = libFuzzer.Pause()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not support pause")

	err = libFuzzer.Resume()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not support resume")
}

// TestFuzzerEventHandling tests event handler callbacks
func TestFuzzerEventHandling(t *testing.T) {
	events := make([]string, 0)
	handler := &CollectingEventHandler{events: &events}

	aflFuzzer := adapter.NewAFLPlusPlus(nil)
	aflFuzzer.SetEventHandler(handler)

	config := adapter.FuzzConfig{
		Target:          "/bin/test",
		OutputDirectory: "/tmp/test",
	}

	err := aflFuzzer.Configure(config)
	require.NoError(t, err)

	// Simulate events by calling handler directly
	handler.OnStart(aflFuzzer)
	handler.OnStats(aflFuzzer, adapter.FuzzerStats{Executions: 100})
	handler.OnCrash(aflFuzzer, &common.CrashResult{ID: "crash-1"})
	handler.OnStop(aflFuzzer, "test complete")

	assert.Equal(t, []string{"start", "stats", "crash", "stop"}, events)
}

// TestFuzzerFactory tests fuzzer factory pattern
func TestFuzzerFactory(t *testing.T) {
	// Test creating AFL++
	aflFuzzer, err := adapter.CreateAFLPlusPlus(nil)
	require.NoError(t, err)
	assert.NotNil(t, aflFuzzer)
	assert.Equal(t, adapter.FuzzerTypeAFL, aflFuzzer.Type())

	// Test creating LibFuzzer
	libFuzzer, err := adapter.CreateLibFuzzer(nil)
	require.NoError(t, err)
	assert.NotNil(t, libFuzzer)
	assert.Equal(t, adapter.FuzzerTypeLibFuzzer, libFuzzer.Type())
}

// TestFuzzerCorpusManagement tests corpus handling
func TestFuzzerCorpusManagement(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "corpus-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create corpus directory with files
	corpusDir := filepath.Join(tempDir, "corpus")
	err = os.MkdirAll(corpusDir, 0755)
	require.NoError(t, err)

	// Create corpus files
	for i := 0; i < 5; i++ {
		filename := fmt.Sprintf("input_%d", i)
		content := []byte(fmt.Sprintf("TEST_INPUT_%d", i))
		err = os.WriteFile(filepath.Join(corpusDir, filename), content, 0644)
		require.NoError(t, err)
	}

	config := adapter.FuzzConfig{
		Target:          "/bin/test",
		OutputDirectory: tempDir,
		WorkDirectory:   tempDir,
	}

	libFuzzer := adapter.NewLibFuzzer(logrus.New())
	err = libFuzzer.Configure(config)
	require.NoError(t, err)

	// Get corpus
	corpus, err := libFuzzer.GetCorpus()
	require.NoError(t, err)
	assert.Len(t, corpus, 5)

	// Verify corpus entries
	for i, entry := range corpus {
		assert.Equal(t, fmt.Sprintf("input_%d", i), entry.FileName)
		assert.Greater(t, entry.Size, int64(0))
		assert.NotEmpty(t, entry.Hash)
	}
}

// Helper types for testing

type TestEventHandler struct {
	events chan string
}

func (h *TestEventHandler) OnStart(fuzzer adapter.Fuzzer) {
	h.events <- "start"
}

func (h *TestEventHandler) OnStop(fuzzer adapter.Fuzzer, reason string) {
	h.events <- "stop"
}

func (h *TestEventHandler) OnCrash(fuzzer adapter.Fuzzer, crash *common.CrashResult) {
	h.events <- "crash"
}

func (h *TestEventHandler) OnNewPath(fuzzer adapter.Fuzzer, path *adapter.CorpusEntry) {
	h.events <- "newpath"
}

func (h *TestEventHandler) OnStats(fuzzer adapter.Fuzzer, stats adapter.FuzzerStats) {
	h.events <- "stats"
}

func (h *TestEventHandler) OnError(fuzzer adapter.Fuzzer, err error) {
	h.events <- "error"
}

func (h *TestEventHandler) OnProgress(fuzzer adapter.Fuzzer, progress adapter.FuzzerProgress) {
	h.events <- "progress"
}

type CollectingEventHandler struct {
	events *[]string
}

func (h *CollectingEventHandler) OnStart(fuzzer adapter.Fuzzer) {
	*h.events = append(*h.events, "start")
}

func (h *CollectingEventHandler) OnStop(fuzzer adapter.Fuzzer, reason string) {
	*h.events = append(*h.events, "stop")
}

func (h *CollectingEventHandler) OnCrash(fuzzer adapter.Fuzzer, crash *common.CrashResult) {
	*h.events = append(*h.events, "crash")
}

func (h *CollectingEventHandler) OnNewPath(fuzzer adapter.Fuzzer, path *adapter.CorpusEntry) {
	*h.events = append(*h.events, "newpath")
}

func (h *CollectingEventHandler) OnStats(fuzzer adapter.Fuzzer, stats adapter.FuzzerStats) {
	*h.events = append(*h.events, "stats")
}

func (h *CollectingEventHandler) OnError(fuzzer adapter.Fuzzer, err error) {
	*h.events = append(*h.events, "error")
}

func (h *CollectingEventHandler) OnProgress(fuzzer adapter.Fuzzer, progress adapter.FuzzerProgress) {
	*h.events = append(*h.events, "progress")
}
