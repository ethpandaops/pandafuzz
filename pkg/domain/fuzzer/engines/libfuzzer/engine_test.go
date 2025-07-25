package libfuzzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/fuzzer"
)

func TestNewEngine(t *testing.T) {
	tests := []struct {
		name string
		log  logrus.FieldLogger
		want func(t *testing.T, engine fuzzer.Fuzzer)
	}{
		{
			name: "with logger",
			log:  logrus.New(),
			want: func(t *testing.T, engine fuzzer.Fuzzer) {
				assert.NotNil(t, engine)
				assert.Equal(t, "LibFuzzer", engine.Name())
				assert.Equal(t, fuzzer.FuzzerTypeLibFuzzer, engine.Type())
				assert.Equal(t, fuzzer.StatusUninitialized, engine.GetStatus())
			},
		},
		{
			name: "without logger",
			log:  nil,
			want: func(t *testing.T, engine fuzzer.Fuzzer) {
				assert.NotNil(t, engine)
				assert.Equal(t, "LibFuzzer", engine.Name())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine(tt.log)
			tt.want(t, engine)
		})
	}
}

func TestEngine_GetCapabilities(t *testing.T) {
	engine := NewEngine(nil)
	capabilities := engine.GetCapabilities()

	expected := []string{
		fuzzer.CapabilityPersistentMode,
		fuzzer.CapabilityCrashVerification,
		fuzzer.CapabilityCorpusMinimization,
		fuzzer.CapabilityHardwareFeedback,
		"edge_coverage",
		"value_profile",
		"data_flow_trace",
		"focus_function",
		"entropic",
	}

	assert.ElementsMatch(t, expected, capabilities)
}

func TestEngine_Configure(t *testing.T) {
	tests := []struct {
		name    string
		config  fuzzer.FuzzConfig
		wantErr bool
		errType string
	}{
		{
			name: "valid configuration",
			config: fuzzer.FuzzConfig{
				Target:        "/usr/bin/fuzz_target",
				WorkDirectory: "/tmp/work",
			},
			wantErr: false,
		},
		{
			name: "missing target",
			config: fuzzer.FuzzConfig{
				WorkDirectory: "/tmp/work",
			},
			wantErr: true,
			errType: fuzzer.ErrInvalidConfig,
		},
		{
			name: "missing work directory",
			config: fuzzer.FuzzConfig{
				Target: "/usr/bin/fuzz_target",
			},
			wantErr: true,
			errType: fuzzer.ErrInvalidConfig,
		},
		{
			name: "with defaults applied",
			config: fuzzer.FuzzConfig{
				Target:        "/usr/bin/fuzz_target",
				WorkDirectory: "/tmp/work",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine(nil)
			err := engine.Configure(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				if fuzzErr, ok := err.(*fuzzer.FuzzerError); ok {
					assert.Equal(t, tt.errType, fuzzErr.Type)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, fuzzer.StatusInitialized, engine.GetStatus())

				// Check defaults were applied
				e := engine.(*Engine)
				if tt.config.OutputDirectory == "" {
					assert.Equal(t, filepath.Join(tt.config.WorkDirectory, "output"), e.config.OutputDirectory)
				}
				if tt.config.CrashDirectory == "" {
					assert.Equal(t, filepath.Join(tt.config.WorkDirectory, "crashes"), e.config.CrashDirectory)
				}
				if tt.config.CorpusDirectory == "" {
					assert.Equal(t, filepath.Join(tt.config.WorkDirectory, "corpus"), e.config.CorpusDirectory)
				}
			}
		})
	}
}

func TestEngine_Initialize(t *testing.T) {
	// Create temporary work directory
	tmpDir, err := os.MkdirTemp("", "libfuzzer-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	engine := NewEngine(nil)
	config := fuzzer.FuzzConfig{
		Target:        "/usr/bin/fuzz_target",
		WorkDirectory: tmpDir,
	}

	// Configure first
	err = engine.Configure(config)
	require.NoError(t, err)

	// Initialize
	err = engine.Initialize()
	require.NoError(t, err)

	// Check directories were created
	dirs := []string{
		config.WorkDirectory,
		filepath.Join(config.WorkDirectory, "output"),
		filepath.Join(config.WorkDirectory, "crashes"),
		filepath.Join(config.WorkDirectory, "corpus"),
	}

	for _, dir := range dirs {
		_, err := os.Stat(dir)
		assert.NoError(t, err, "Directory should exist: %s", dir)
	}
}

func TestEngine_StatusTransitions(t *testing.T) {
	engine := NewEngine(nil).(*Engine)

	// Initial status
	assert.Equal(t, fuzzer.StatusUninitialized, engine.GetStatus())

	// Configure
	config := fuzzer.FuzzConfig{
		Target:        "/usr/bin/fuzz_target",
		WorkDirectory: "/tmp/work",
	}
	err := engine.Configure(config)
	require.NoError(t, err)
	assert.Equal(t, fuzzer.StatusInitialized, engine.GetStatus())

	// Can't initialize from wrong status
	engine.updateStatus(fuzzer.StatusRunning)
	err = engine.Initialize()
	assert.Error(t, err)
}

func TestEngine_GetStats(t *testing.T) {
	engine := NewEngine(nil).(*Engine)

	// Initial stats
	stats := engine.GetStats()
	assert.Equal(t, int64(0), stats.Executions)
	assert.Equal(t, float64(0), stats.ExecPerSecond)

	// Update stats
	engine.statsMutex.Lock()
	engine.stats.Executions = 1000
	engine.stats.ExecPerSecond = 100.5
	engine.stats.CoveredEdges = 500
	engine.statsMutex.Unlock()

	stats = engine.GetStats()
	assert.Equal(t, int64(1000), stats.Executions)
	assert.Equal(t, 100.5, stats.ExecPerSecond)
	assert.Equal(t, 500, stats.CoveredEdges)
}

func TestEngine_ParseStatsLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		validate func(t *testing.T, engine *Engine)
	}{
		{
			name: "full stats line",
			line: "#12345 NEW cov: 1234 ft: 5678 corp: 90/123Kb lim: 4096 exec/s: 1000 rss: 84Mb",
			validate: func(t *testing.T, engine *Engine) {
				stats := engine.GetStats()
				assert.Equal(t, int64(12345), stats.Executions)
				assert.Equal(t, 1234, stats.CoveredEdges)
				assert.Equal(t, 90, stats.CorpusSize)
				assert.Equal(t, float64(1000), stats.ExecPerSecond)
				assert.Equal(t, int64(84*1024*1024), stats.MemoryUsage)
			},
		},
		{
			name: "partial stats line",
			line: "#5000 cov: 100 exec/s: 500",
			validate: func(t *testing.T, engine *Engine) {
				stats := engine.GetStats()
				assert.Equal(t, int64(5000), stats.Executions)
				assert.Equal(t, 100, stats.CoveredEdges)
				assert.Equal(t, float64(500), stats.ExecPerSecond)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine(nil).(*Engine)
			engine.parseStatsLine(tt.line)
			tt.validate(t, engine)
		})
	}
}

func TestEngine_ReproduceCrash(t *testing.T) {
	// Create temporary work directory
	tmpDir, err := os.MkdirTemp("", "libfuzzer-repro-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	engine := NewEngine(nil)
	config := fuzzer.FuzzConfig{
		Target:        "/bin/echo", // Use echo as a safe test target
		WorkDirectory: tmpDir,
		TargetArgs:    []string{"test"},
	}

	err = engine.Configure(config)
	require.NoError(t, err)

	// Test reproduction
	ctx := context.Background()
	crashInput := []byte("crash input data")
	reproConfig := fuzzer.ReproductionConfig{
		OriginalCrashID: "crash-123",
		Timeout:         5 * time.Second,
		Attempts:        1,
	}

	result, err := engine.ReproduceCrash(ctx, crashInput, reproConfig)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "crash-123", result.CrashID)
	assert.Equal(t, "local", result.BotID)
	assert.False(t, result.Reproduced) // echo won't crash
}

func TestEngine_EventHandler(t *testing.T) {
	engine := NewEngine(nil)

	// Default handler should be set
	assert.NotNil(t, engine.(*Engine).eventHandler)

	// Custom handler
	called := false
	handler := &mockEventHandler{
		onStart: func(f fuzzer.Fuzzer) {
			called = true
		},
	}

	engine.SetEventHandler(handler)
	engine.(*Engine).eventHandler.OnStart(engine)
	assert.True(t, called)
}

func TestEngine_BuildCommandArgs(t *testing.T) {
	engine := NewEngine(nil).(*Engine)
	engine.config = fuzzer.FuzzConfig{
		CorpusDirectory: "/tmp/corpus",
		MemoryLimit:     1024 * 1024 * 1024, // 1GB
		Timeout:         30 * time.Second,
		Duration:        3600 * time.Second,
		Dictionary:      "/tmp/dict.txt",
		CrashDirectory:  "/tmp/crashes",
		MaxCrashes:      10,
		FuzzerOptions: map[string]any{
			"jobs":    4,
			"max_len": 1000,
		},
		TargetArgs: []string{"--flag", "value"},
	}

	args := engine.buildCommandArgs()

	// Check expected arguments
	assert.Contains(t, args, "/tmp/corpus")
	assert.Contains(t, args, "-rss_limit_mb=1024")
	assert.Contains(t, args, "-timeout=30")
	assert.Contains(t, args, "-max_total_time=3600")
	assert.Contains(t, args, "-dict=/tmp/dict.txt")
	assert.Contains(t, args, "-artifact_prefix=/tmp/crashes/")
	assert.Contains(t, args, "-error_exitcode=0")
	assert.Contains(t, args, "-jobs=4")
	assert.Contains(t, args, "-max_len=1000")
	assert.Contains(t, args, "--flag")
	assert.Contains(t, args, "value")
}

// Mock event handler for testing
type mockEventHandler struct {
	onStart    func(fuzzer.Fuzzer)
	onStop     func(fuzzer.Fuzzer, string)
	onCrash    func(fuzzer.Fuzzer, *common.CrashResult)
	onNewPath  func(fuzzer.Fuzzer, *fuzzer.CorpusEntry)
	onStats    func(fuzzer.Fuzzer, fuzzer.FuzzerStats)
	onError    func(fuzzer.Fuzzer, error)
	onProgress func(fuzzer.Fuzzer, fuzzer.FuzzerProgress)
}

func (h *mockEventHandler) OnStart(fuzzer fuzzer.Fuzzer) {
	if h.onStart != nil {
		h.onStart(fuzzer)
	}
}

func (h *mockEventHandler) OnStop(fuzzer fuzzer.Fuzzer, reason string) {
	if h.onStop != nil {
		h.onStop(fuzzer, reason)
	}
}

func (h *mockEventHandler) OnCrash(fuzzer fuzzer.Fuzzer, crash *common.CrashResult) {
	if h.onCrash != nil {
		h.onCrash(fuzzer, crash)
	}
}

func (h *mockEventHandler) OnNewPath(fuzzer fuzzer.Fuzzer, path *fuzzer.CorpusEntry) {
	if h.onNewPath != nil {
		h.onNewPath(fuzzer, path)
	}
}

func (h *mockEventHandler) OnStats(fuzzer fuzzer.Fuzzer, stats fuzzer.FuzzerStats) {
	if h.onStats != nil {
		h.onStats(fuzzer, stats)
	}
}

func (h *mockEventHandler) OnError(fuzzer fuzzer.Fuzzer, err error) {
	if h.onError != nil {
		h.onError(fuzzer, err)
	}
}

func (h *mockEventHandler) OnProgress(fuzzer fuzzer.Fuzzer, progress fuzzer.FuzzerProgress) {
	if h.onProgress != nil {
		h.onProgress(fuzzer, progress)
	}
}
