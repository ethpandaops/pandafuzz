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
	"github.com/ethpandaops/pandafuzz/pkg/fuzzer"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFuzzerInterface tests the fuzzer interface implementation
func TestFuzzerInterface(t *testing.T) {
	// Test AFL++ implementation
	aflFuzzer := fuzzer.NewAFLPlusPlus()
	assert.NotNil(t, aflFuzzer)
	assert.Equal(t, "AFL++", aflFuzzer.Name())
	assert.Equal(t, fuzzer.FuzzerTypeAFL, aflFuzzer.Type())
	assert.NotEmpty(t, aflFuzzer.GetCapabilities())

	// Test LibFuzzer implementation
	libFuzzer := fuzzer.NewLibFuzzer()
	assert.NotNil(t, libFuzzer)
	assert.Equal(t, "LibFuzzer", libFuzzer.Name())
	assert.Equal(t, fuzzer.FuzzerTypeLibFuzzer, libFuzzer.Type())
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

	config := fuzzer.FuzzConfig{
		Target:          testBinary,
		TargetArgs:      []string{"@@"},
		WorkDirectory:   tempDir,
		OutputDirectory: filepath.Join(tempDir, "output"),
		Duration:        10 * time.Second,
		Timeout:         1 * time.Second,
		MemoryLimit:     1024,
		FuzzerOptions: map[string]interface{}{
			"deterministic": false,
		},
	}

	// Test AFL++ configuration
	aflFuzzer := fuzzer.NewAFLPlusPlus()
	err = aflFuzzer.Configure(config)
	assert.NoError(t, err)

	// Test LibFuzzer configuration
	libFuzzer := fuzzer.NewLibFuzzer()
	err = libFuzzer.Configure(config)
	assert.NoError(t, err)
}

// TestFuzzerInitialization tests fuzzer initialization
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

	config := fuzzer.FuzzConfig{
		Target:          testBinary,
		OutputDirectory: filepath.Join(tempDir, "output"),
		SeedDirectory:   seedDir,
		WorkDirectory:   tempDir,
	}

	// Initialize AFL++
	aflFuzzer := fuzzer.NewAFLPlusPlus()
	err = aflFuzzer.Configure(config)
	require.NoError(t, err)
	
	err = aflFuzzer.Initialize()
	assert.NoError(t, err)

	// Verify output directories were created
	assert.DirExists(t, filepath.Join(tempDir, "output", "afl_output"))
}

// TestFuzzerValidation tests fuzzer validation
func TestFuzzerValidation(t *testing.T) {
	aflFuzzer := fuzzer.NewAFLPlusPlus()

	// Test validation without configuration
	err := aflFuzzer.Validate()
	assert.Error(t, err)

	// Test with invalid target
	config := fuzzer.FuzzConfig{
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

	config := fuzzer.FuzzConfig{
		Target:          testBinary,
		TargetArgs:      []string{},
		OutputDirectory: filepath.Join(tempDir, "output"),
		WorkDirectory:   tempDir,
		Duration:        2 * time.Second,
		Timeout:         100 * time.Millisecond,
		MemoryLimit:     512,
	}

	// Create and configure fuzzer
	aflFuzzer := fuzzer.NewAFLPlusPlus()
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
	assert.Equal(t, fuzzer.StatusRunning, aflFuzzer.GetStatus())
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
	executor := bot.NewJobExecutor(&botConfig, logger)

	// Create test job
	job := &common.Job{
		ID:       "fuzzer-job-1",
		Name:     "Test Fuzzing",
		Fuzzer:   "afl++",
		Target:   "/bin/echo", // Use system binary for testing
		Status:   common.JobStatusPending,
		WorkDir:  workDir,
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
func TestFuzzerCrashHandling(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fuzzer-crash-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create AFL++ output structure with crashes
	aflOutput := filepath.Join(tempDir, "afl_output")
	crashDir := filepath.Join(aflOutput, "crashes")
	err = os.MkdirAll(crashDir, 0755)
	require.NoError(t, err)

	// Create mock crash files
	crashes := []struct {
		name    string
		content []byte
	}{
		{"id:000000,sig:11,src:000000,op:havoc,rep:4", []byte("AAAA")},
		{"id:000001,sig:06,src:000001,op:havoc,rep:2", []byte("BBBB")},
	}

	for _, crash := range crashes {
		err = os.WriteFile(filepath.Join(crashDir, crash.name), crash.content, 0644)
		require.NoError(t, err)
	}

	// Configure fuzzer with existing output
	config := fuzzer.FuzzConfig{
		Target:          "/bin/test",
		OutputDirectory: tempDir,
		WorkDirectory:   tempDir,
	}

	aflFuzzer := fuzzer.NewAFLPlusPlus()
	err = aflFuzzer.Configure(config)
	require.NoError(t, err)

	// Get crashes
	detectedCrashes, err := aflFuzzer.GetCrashes()
	require.NoError(t, err)
	assert.Len(t, detectedCrashes, 2)

	// Verify crash details
	for i, crash := range detectedCrashes {
		assert.NotEmpty(t, crash.ID)
		assert.Equal(t, crashes[i].content, crash.Input)
		assert.Equal(t, int64(len(crashes[i].content)), crash.Size)
		assert.NotEmpty(t, crash.Hash)
		
		// Check crash type detection
		if i == 0 {
			assert.Equal(t, "segmentation_fault", crash.Type)
		} else if i == 1 {
			assert.Equal(t, "abort", crash.Type)
		}
	}
}

// TestFuzzerCoverageReporting tests coverage collection
func TestFuzzerCoverageReporting(t *testing.T) {
	aflFuzzer := fuzzer.NewAFLPlusPlus()
	
	// Configure with minimal settings
	config := fuzzer.FuzzConfig{
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
	aflFuzzer := fuzzer.NewAFLPlusPlus()
	
	config := fuzzer.FuzzConfig{
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
func TestFuzzerCleanup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fuzzer-cleanup-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	config := fuzzer.FuzzConfig{
		Target:          "/bin/test",
		OutputDirectory: tempDir,
		FuzzerOptions: map[string]interface{}{
			"clean_temp": true,
		},
	}

	aflFuzzer := fuzzer.NewAFLPlusPlus()
	err = aflFuzzer.Configure(config)
	require.NoError(t, err)
	
	err = aflFuzzer.Initialize()
	require.NoError(t, err)

	// Create some output files
	outputDir := filepath.Join(tempDir, "afl_output")
	assert.DirExists(t, outputDir)

	// Cleanup
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

	config := fuzzer.FuzzConfig{
		Target:          testBinary,
		OutputDirectory: filepath.Join(tempDir, "output"),
		WorkDirectory:   tempDir,
		Duration:        5 * time.Second,
		MaxExecutions:   100,
		FuzzerOptions: map[string]interface{}{
			"workers":        2,
			"fork":           true,
			"value_profile":  true,
			"entropic":       true,
		},
	}

	libFuzzer := fuzzer.NewLibFuzzer()
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

	aflFuzzer := fuzzer.NewAFLPlusPlus()
	aflFuzzer.SetEventHandler(handler)

	config := fuzzer.FuzzConfig{
		Target:          "/bin/test",
		OutputDirectory: "/tmp/test",
	}
	
	err := aflFuzzer.Configure(config)
	require.NoError(t, err)

	// Simulate events by calling handler directly
	handler.OnStart(aflFuzzer)
	handler.OnStats(aflFuzzer, fuzzer.FuzzerStats{Executions: 100})
	handler.OnCrash(aflFuzzer, &common.CrashResult{ID: "crash-1"})
	handler.OnStop(aflFuzzer, "test complete")

	assert.Equal(t, []string{"start", "stats", "crash", "stop"}, events)
}

// TestFuzzerFactory tests fuzzer factory pattern
func TestFuzzerFactory(t *testing.T) {
	// Test creating AFL++
	aflFuzzer, err := fuzzer.CreateAFLPlusPlus()
	require.NoError(t, err)
	assert.NotNil(t, aflFuzzer)
	assert.Equal(t, fuzzer.FuzzerTypeAFL, aflFuzzer.Type())

	// Test creating LibFuzzer
	libFuzzer, err := fuzzer.CreateLibFuzzer()
	require.NoError(t, err)
	assert.NotNil(t, libFuzzer)
	assert.Equal(t, fuzzer.FuzzerTypeLibFuzzer, libFuzzer.Type())
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

	config := fuzzer.FuzzConfig{
		Target:          "/bin/test",
		OutputDirectory: tempDir,
		WorkDirectory:   tempDir,
	}

	libFuzzer := fuzzer.NewLibFuzzer()
	err = libFuzzer.Configure(config)
	require.NoError(t, err)

	// TODO: corpusDir is an unexported field - this test needs to be rewritten
	// to use the public API for setting corpus directory
	// libFuzzer.corpusDir = corpusDir

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

func (h *TestEventHandler) OnStart(fuzzer fuzzer.Fuzzer) {
	h.events <- "start"
}

func (h *TestEventHandler) OnStop(fuzzer fuzzer.Fuzzer, reason string) {
	h.events <- "stop"
}

func (h *TestEventHandler) OnCrash(fuzzer fuzzer.Fuzzer, crash *common.CrashResult) {
	h.events <- "crash"
}

func (h *TestEventHandler) OnNewPath(fuzzer fuzzer.Fuzzer, path *fuzzer.CorpusEntry) {
	h.events <- "newpath"
}

func (h *TestEventHandler) OnStats(fuzzer fuzzer.Fuzzer, stats fuzzer.FuzzerStats) {
	h.events <- "stats"
}

func (h *TestEventHandler) OnError(fuzzer fuzzer.Fuzzer, err error) {
	h.events <- "error"
}

func (h *TestEventHandler) OnProgress(fuzzer fuzzer.Fuzzer, progress fuzzer.FuzzerProgress) {
	h.events <- "progress"
}

type CollectingEventHandler struct {
	events *[]string
}

func (h *CollectingEventHandler) OnStart(fuzzer fuzzer.Fuzzer) {
	*h.events = append(*h.events, "start")
}

func (h *CollectingEventHandler) OnStop(fuzzer fuzzer.Fuzzer, reason string) {
	*h.events = append(*h.events, "stop")
}

func (h *CollectingEventHandler) OnCrash(fuzzer fuzzer.Fuzzer, crash *common.CrashResult) {
	*h.events = append(*h.events, "crash")
}

func (h *CollectingEventHandler) OnNewPath(fuzzer fuzzer.Fuzzer, path *fuzzer.CorpusEntry) {
	*h.events = append(*h.events, "newpath")
}

func (h *CollectingEventHandler) OnStats(fuzzer fuzzer.Fuzzer, stats fuzzer.FuzzerStats) {
	*h.events = append(*h.events, "stats")
}

func (h *CollectingEventHandler) OnError(fuzzer fuzzer.Fuzzer, err error) {
	*h.events = append(*h.events, "error")
}

func (h *CollectingEventHandler) OnProgress(fuzzer fuzzer.Fuzzer, progress fuzzer.FuzzerProgress) {
	*h.events = append(*h.events, "progress")
}