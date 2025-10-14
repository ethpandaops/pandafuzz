package fuzzer

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// AFLPlusPlus implements the Fuzzer interface for AFL++
type AFLPlusPlus struct {
	*BaseFuzzer      // Embed base fuzzer for event handling
	config           FuzzConfig
	status           FuzzerStatus
	logger           *logrus.Logger
	cmd              *exec.Cmd
	ctx              context.Context
	cancel           context.CancelFunc
	eventHandler     EventHandler
	stats            FuzzerStats
	statsFile        string
	outputDir        string
	crashDir         string
	corpusDir        string
	mu               sync.RWMutex
	monitorTicker    *time.Ticker
	wg               sync.WaitGroup
	botID            string
	metricsCollector common.MetricsCollector

	// Performance tracking
	lastExecCount   int64
	lastStatsUpdate time.Time
	execHistory     []float64
	peakExecSpeed   float64

	// Crash tracking
	lastReportedCrashes int
}

// Compile-time interface compliance check
var _ Fuzzer = (*AFLPlusPlus)(nil)

// NewAFLPlusPlus creates a new AFL++ fuzzer instance
func NewAFLPlusPlus(logger *logrus.Logger) *AFLPlusPlus {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	afl := &AFLPlusPlus{
		BaseFuzzer:   NewBaseFuzzer(logger),
		status:       StatusUninitialized,
		logger:       logger,
		eventHandler: &DefaultEventHandler{},
		stats: FuzzerStats{
			StartTime: time.Now(),
		},
		execHistory:     make([]float64, 0, 100),
		lastStatsUpdate: time.Now(),
	}

	// Create metrics collector
	afl.metricsCollector = NewDefaultMetricsCollector(
		logger.WithField("component", "afl_metrics"),
		5*time.Second,
	)

	return afl
}

// Name returns the name of the fuzzer
func (afl *AFLPlusPlus) Name() string {
	return "AFL++"
}

// Type returns the fuzzer type
func (afl *AFLPlusPlus) Type() FuzzerType {
	return FuzzerTypeAFL
}

// Version returns the AFL++ version
func (afl *AFLPlusPlus) Version() string {
	cmd := exec.Command("afl-fuzz", "-h")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "unknown"
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "afl-fuzz++") || strings.Contains(line, "version") {
			parts := strings.Fields(line)
			for i, part := range parts {
				if strings.Contains(part, "version") && i+1 < len(parts) {
					return parts[i+1]
				}
			}
		}
	}

	return "unknown"
}

// GetCapabilities returns AFL++ capabilities
func (afl *AFLPlusPlus) GetCapabilities() []string {
	return []string{
		"persistent_mode",
		"deferred_forkserver",
		"shared_memory",
		"cmplog",
		"laf_intel",
		"redqueen",
		"deterministic",
		"havoc",
		"splice",
		"python_mutators",
		"custom_mutators",
		"qemu_mode",
		"unicorn_mode",
		"frida_mode",
	}
}

// Configure sets up the fuzzer configuration
func (afl *AFLPlusPlus) Configure(config FuzzConfig) error {
	afl.mu.Lock()
	defer afl.mu.Unlock()

	if afl.status != StatusUninitialized && afl.status != StatusStopped {
		return &FuzzerError{
			Type:    ErrInvalidConfig,
			Message: "cannot configure fuzzer while running",
			Fuzzer:  afl.Name(),
			Code:    1,
		}
	}

	// Validate configuration
	if err := afl.validateConfig(config); err != nil {
		return err
	}

	afl.config = config

	// Set up directories
	afl.outputDir = filepath.Join(config.OutputDirectory, "afl_output")
	afl.crashDir = filepath.Join(afl.outputDir, "crashes")
	afl.corpusDir = filepath.Join(afl.outputDir, "queue")
	afl.statsFile = filepath.Join(afl.outputDir, "fuzzer_stats")

	afl.status = StatusInitialized

	afl.logger.WithFields(logrus.Fields{
		"target":     config.Target,
		"output_dir": afl.outputDir,
		"duration":   config.Duration,
	}).Info("AFL++ configured")

	return nil
}

// Initialize prepares AFL++ for execution
func (afl *AFLPlusPlus) Initialize() error {
	afl.mu.Lock()
	defer afl.mu.Unlock()

	if afl.status != StatusInitialized {
		return &FuzzerError{
			Type:    ErrInternal,
			Message: "fuzzer must be configured before initialization",
			Fuzzer:  afl.Name(),
			Code:    2,
		}
	}

	// Create output directories
	if err := os.MkdirAll(afl.outputDir, 0755); err != nil {
		return &FuzzerError{
			Type:    ErrPermissionDenied,
			Message: fmt.Sprintf("failed to create output directory: %v", err),
			Fuzzer:  afl.Name(),
			Code:    3,
		}
	}

	// Check for AFL++ installation
	if _, err := exec.LookPath("afl-fuzz"); err != nil {
		return &FuzzerError{
			Type:    ErrTargetNotFound,
			Message: "afl-fuzz not found in PATH",
			Fuzzer:  afl.Name(),
			Code:    4,
		}
	}

	// Set AFL++ environment variables
	os.Setenv("AFL_SKIP_CPUFREQ", "1")
	os.Setenv("AFL_NO_AFFINITY", "1")
	os.Setenv("AFL_NO_UI", "1")
	// Removed AFL_SKIP_BIN_CHECK to allow AFL++ to detect instrumentation
	os.Setenv("AFL_I_DONT_CARE_ABOUT_MISSING_CRASHES", "1")

	// Enable specific AFL++ features based on config
	if features, ok := afl.config.FuzzerOptions["afl_features"].(map[string]bool); ok {
		if features["cmplog"] {
			os.Setenv("AFL_CMPLOG", "1")
		}
		if features["autodictionary"] {
			os.Setenv("AFL_AUTODICT", "1")
		}
	}

	afl.logger.Info("AFL++ initialized")

	return nil
}

// Validate checks if the fuzzer is properly configured
func (afl *AFLPlusPlus) Validate() error {
	afl.mu.RLock()
	defer afl.mu.RUnlock()

	// Check target binary exists
	if _, err := os.Stat(afl.config.Target); err != nil {
		return &FuzzerError{
			Type:    ErrTargetNotFound,
			Message: fmt.Sprintf("target binary not found: %s", afl.config.Target),
			Fuzzer:  afl.Name(),
			Code:    5,
		}
	}

	// Check seed directory if specified
	if afl.config.SeedDirectory != "" {
		if _, err := os.Stat(afl.config.SeedDirectory); err != nil {
			return &FuzzerError{
				Type:    ErrInvalidConfig,
				Message: fmt.Sprintf("seed directory not found: %s", afl.config.SeedDirectory),
				Fuzzer:  afl.Name(),
				Code:    6,
			}
		}
	}

	// Check dictionary if specified
	if afl.config.Dictionary != "" {
		if _, err := os.Stat(afl.config.Dictionary); err != nil {
			return &FuzzerError{
				Type:    ErrInvalidConfig,
				Message: fmt.Sprintf("dictionary file not found: %s", afl.config.Dictionary),
				Fuzzer:  afl.Name(),
				Code:    7,
			}
		}
	}

	return nil
}

// Start begins the fuzzing process
func (afl *AFLPlusPlus) Start(ctx context.Context) error {
	afl.mu.Lock()
	defer afl.mu.Unlock()

	if afl.status == StatusRunning || afl.status == StatusStarting {
		return &FuzzerError{
			Type:    ErrInternal,
			Message: "fuzzer is already running",
			Fuzzer:  afl.Name(),
			Code:    8,
		}
	}

	if afl.status != StatusInitialized && afl.status != StatusPaused {
		return &FuzzerError{
			Type:    ErrInternal,
			Message: "fuzzer must be initialized before starting",
			Fuzzer:  afl.Name(),
			Code:    9,
		}
	}

	afl.status = StatusStarting
	afl.ctx, afl.cancel = context.WithCancel(ctx)

	// Build AFL++ command
	args := afl.buildAFLArgs()
	afl.logger.WithField("afl_args", args).Debug("AFL++ command arguments")
	afl.cmd = exec.CommandContext(afl.ctx, "afl-fuzz", args...)

	// Set up pipes for output
	stdout, err := afl.cmd.StdoutPipe()
	if err != nil {
		afl.status = StatusError
		return &FuzzerError{
			Type:    ErrInternal,
			Message: fmt.Sprintf("failed to create stdout pipe: %v", err),
			Fuzzer:  afl.Name(),
			Code:    10,
		}
	}

	stderr, err := afl.cmd.StderrPipe()
	if err != nil {
		afl.status = StatusError
		return &FuzzerError{
			Type:    ErrInternal,
			Message: fmt.Sprintf("failed to create stderr pipe: %v", err),
			Fuzzer:  afl.Name(),
			Code:    11,
		}
	}

	// Start AFL++
	if err := afl.cmd.Start(); err != nil {
		afl.status = StatusError
		return &FuzzerError{
			Type:    ErrInternal,
			Message: fmt.Sprintf("failed to start AFL++: %v", err),
			Fuzzer:  afl.Name(),
			Code:    12,
		}
	}

	afl.status = StatusRunning
	afl.stats.StartTime = time.Now()

	// Start output monitoring
	afl.wg.Add(2)
	go afl.monitorOutput(stdout, "stdout")
	go afl.monitorOutput(stderr, "stderr")

	// Start stats monitoring
	afl.startStatsMonitoring()

	// Start metrics collector
	if err := afl.metricsCollector.Start(afl.ctx); err != nil {
		afl.logger.WithError(err).Warn("Failed to start metrics collector")
	}

	// Notify event handler
	if afl.eventHandler != nil {
		afl.eventHandler.OnStart(afl)
	}

	// Emit started event through base fuzzer
	afl.EmitStartedEvent(afl.ctx, afl.config.Target, map[string]interface{}{
		"fuzzer": "AFL++",
		"pid":    afl.cmd.Process.Pid,
		"bot_id": afl.botID,
	})

	// Monitor process completion
	afl.wg.Add(1)
	go afl.monitorProcess()

	afl.logger.WithField("pid", afl.cmd.Process.Pid).Info("AFL++ started")

	return nil
}

// Stop gracefully stops the fuzzing process
func (afl *AFLPlusPlus) Stop() error {
	afl.mu.Lock()
	defer afl.mu.Unlock()

	if afl.status != StatusRunning && afl.status != StatusPaused {
		return nil
	}

	afl.status = StatusStopping

	// Cancel context to stop monitoring
	if afl.cancel != nil {
		afl.cancel()
	}

	// Stop stats monitoring
	if afl.monitorTicker != nil {
		afl.monitorTicker.Stop()
	}

	// Stop metrics collector
	if afl.metricsCollector != nil {
		if err := afl.metricsCollector.Stop(); err != nil {
			afl.logger.WithError(err).Warn("Failed to stop metrics collector")
		}
	}

	// Send SIGTERM to AFL++
	if afl.cmd != nil && afl.cmd.Process != nil {
		if err := afl.cmd.Process.Signal(os.Interrupt); err != nil {
			afl.logger.WithError(err).Warn("Failed to send interrupt signal")
		}

		// Give it a moment to exit gracefully
		time.Sleep(100 * time.Millisecond)

		// Force kill if still running
		if afl.cmd.Process != nil {
			if err := afl.cmd.Process.Kill(); err != nil {
				afl.logger.WithError(err).Debug("Process may have already exited")
			}
		}
	}

	// Update status immediately so IsRunning() returns false
	afl.status = StatusStopped

	// Wait for goroutines to finish (with timeout to avoid hanging)
	done := make(chan struct{})
	go func() {
		afl.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Goroutines finished normally
	case <-time.After(5 * time.Second):
		// Timeout waiting for goroutines
		afl.logger.Warn("Timeout waiting for fuzzer goroutines to finish")
	}

	// Notify event handler
	if afl.eventHandler != nil {
		afl.eventHandler.OnStop(afl, "user requested")
	}

	// Emit stopped event through base fuzzer
	afl.EmitStoppedEvent(afl.ctx, afl.config.Target, "user requested")

	afl.logger.Info("AFL++ stopped")

	return nil
}

// Pause pauses the fuzzing process
func (afl *AFLPlusPlus) Pause() error {
	afl.mu.Lock()
	defer afl.mu.Unlock()

	if afl.status != StatusRunning {
		return &FuzzerError{
			Type:    ErrInternal,
			Message: "fuzzer is not running",
			Fuzzer:  afl.Name(),
			Code:    13,
		}
	}

	// Send SIGSTOP to pause
	if afl.cmd != nil && afl.cmd.Process != nil {
		if err := afl.cmd.Process.Signal(syscall.SIGSTOP); err != nil {
			return &FuzzerError{
				Type:    ErrInternal,
				Message: fmt.Sprintf("failed to pause AFL++: %v", err),
				Fuzzer:  afl.Name(),
				Code:    14,
			}
		}
	}

	afl.status = StatusPaused
	afl.logger.Info("AFL++ paused")

	return nil
}

// Resume resumes the fuzzing process
func (afl *AFLPlusPlus) Resume() error {
	afl.mu.Lock()
	defer afl.mu.Unlock()

	if afl.status != StatusPaused {
		return &FuzzerError{
			Type:    ErrInternal,
			Message: "fuzzer is not paused",
			Fuzzer:  afl.Name(),
			Code:    15,
		}
	}

	// Send SIGCONT to resume
	if afl.cmd != nil && afl.cmd.Process != nil {
		if err := afl.cmd.Process.Signal(syscall.SIGCONT); err != nil {
			return &FuzzerError{
				Type:    ErrInternal,
				Message: fmt.Sprintf("failed to resume AFL++: %v", err),
				Fuzzer:  afl.Name(),
				Code:    16,
			}
		}
	}

	afl.status = StatusRunning
	afl.logger.Info("AFL++ resumed")

	return nil
}

// GetStatus returns the current fuzzer status
func (afl *AFLPlusPlus) GetStatus() FuzzerStatus {
	afl.mu.RLock()
	defer afl.mu.RUnlock()

	return afl.status
}

// SetBotID sets the bot ID for crash reporting
func (afl *AFLPlusPlus) SetBotID(botID string) {
	afl.mu.Lock()
	defer afl.mu.Unlock()
	afl.botID = botID
}

// GetStats returns current fuzzing statistics
func (afl *AFLPlusPlus) GetStats() FuzzerStats {
	afl.mu.RLock()
	defer afl.mu.RUnlock()

	stats := afl.stats
	stats.ElapsedTime = time.Since(stats.StartTime)

	return stats
}

// GetProgress returns fuzzing progress information
func (afl *AFLPlusPlus) GetProgress() FuzzerProgress {
	afl.mu.RLock()
	defer afl.mu.RUnlock()

	progress := FuzzerProgress{
		Phase:           afl.getPhase(),
		ProgressPercent: afl.calculateProgress(),
		CurrentInput:    afl.getCurrentInput(),
		QueuePosition:   int(afl.stats.CorpusSize),
		QueueSize:       int(afl.stats.PathsTotal),
		LastUpdate:      time.Now(),
	}

	if afl.config.Duration > 0 {
		elapsed := time.Since(afl.stats.StartTime)
		remaining := afl.config.Duration - elapsed
		if remaining > 0 {
			progress.ETA = remaining
		}
	}

	return progress
}

// IsRunning returns whether the fuzzer is currently running
func (afl *AFLPlusPlus) IsRunning() bool {
	afl.mu.RLock()
	defer afl.mu.RUnlock()

	return afl.status == StatusRunning
}

// GetResults retrieves all fuzzing results
func (afl *AFLPlusPlus) GetResults() (*FuzzerResults, error) {
	afl.mu.RLock()
	defer afl.mu.RUnlock()

	crashes, err := afl.GetCrashes()
	if err != nil {
		return nil, err
	}

	coverage, err := afl.GetCoverage()
	if err != nil {
		return nil, err
	}

	corpus, err := afl.GetCorpus()
	if err != nil {
		return nil, err
	}

	results := &FuzzerResults{
		Summary: ResultSummary{
			TotalExecutions:  afl.stats.Executions,
			ExecutionTime:    time.Since(afl.stats.StartTime),
			UniqueCrashes:    afl.stats.UniqueCrashes,
			CoverageAchieved: afl.stats.CoveragePercent,
			NewInputsFound:   afl.stats.NewPaths,
			Success:          afl.stats.UniqueCrashes > 0 || afl.stats.NewPaths > 0,
			ExitReason:       afl.getExitReason(),
		},
		Crashes:  crashes,
		Coverage: coverage,
		Corpus:   corpus,
		Performance: PerformanceMetrics{
			AverageExecSpeed: afl.stats.ExecPerSecond,
			PeakExecSpeed:    afl.getPeakExecSpeed(),
			AverageCPU:       afl.stats.CPUUsage,
			PeakMemory:       afl.stats.MemoryUsage,
			StartupTime:      1 * time.Second, // AFL++ typically starts quickly
		},
	}

	return results, nil
}

// GetCrashes retrieves crash information
func (afl *AFLPlusPlus) GetCrashes() ([]*common.CrashResult, error) {
	crashes := make([]*common.CrashResult, 0)

	afl.logger.WithFields(logrus.Fields{
		"crash_dir": afl.crashDir,
		"job_id":    afl.config.Target,
	}).Info("Scanning AFL++ crash directory for new crashes")

	// Read crashes from AFL++ crash directory
	if _, err := os.Stat(afl.crashDir); err == nil {
		files, err := os.ReadDir(afl.crashDir)
		if err != nil {
			return nil, err
		}

		afl.logger.WithFields(logrus.Fields{
			"crash_dir":  afl.crashDir,
			"file_count": len(files),
		}).Debug("Found files in AFL++ crash directory")

		for _, file := range files {
			if file.IsDir() || strings.HasPrefix(file.Name(), "README") {
				continue
			}

			crashPath := filepath.Join(afl.crashDir, file.Name())

			afl.logger.WithFields(logrus.Fields{
				"crash_file": file.Name(),
				"crash_path": crashPath,
			}).Info("Found AFL++ crash file")

			crashData, err := os.ReadFile(crashPath)
			if err != nil {
				afl.logger.WithError(err).WithField("file", file.Name()).Warn("Failed to read crash file")
				continue
			}

			info, err := file.Info()
			if err != nil {
				continue
			}

			crashType := afl.detectCrashType(file.Name())
			crashHash := afl.hashInput(crashData)

			crash := &common.CrashResult{
				ID:          file.Name(),
				JobID:       afl.config.JobID, // Use actual job ID from config
				BotID:       afl.botID,
				Timestamp:   info.ModTime(),
				FilePath:    filepath.Join(afl.crashDir, file.Name()),
				Size:        int64(len(crashData)),
				Hash:        crashHash,
				Type:        crashType,
				Input:       crashData,                                    // Include the crash input data
				InputBase64: base64.StdEncoding.EncodeToString(crashData), // Base64 encode the crash data
				IsUnique:    true,                                         // Mark as unique for now
			}

			afl.logger.WithFields(logrus.Fields{
				"crash_id":   crash.ID,
				"crash_type": crashType,
				"crash_hash": crashHash,
				"crash_size": crash.Size,
				"job_id":     crash.JobID,
				"bot_id":     crash.BotID,
				"file_name":  file.Name(),
			}).Info("Detected AFL++ crash")

			crashes = append(crashes, crash)
		}

		afl.logger.WithFields(logrus.Fields{
			"crash_count": len(crashes),
			"job_id":      afl.config.Target,
		}).Info("Completed AFL++ crash scan")
	} else {
		afl.logger.WithFields(logrus.Fields{
			"crash_dir": afl.crashDir,
			"error":     err,
		}).Debug("AFL++ crash directory does not exist yet")
	}

	return crashes, nil
}

// GetCoverage retrieves coverage information
func (afl *AFLPlusPlus) GetCoverage() (*common.CoverageResult, error) {
	afl.mu.RLock()
	defer afl.mu.RUnlock()

	coverage := &common.CoverageResult{
		ID:        fmt.Sprintf("afl_%d", time.Now().Unix()),
		JobID:     afl.config.Target,
		BotID:     afl.botID,
		Timestamp: time.Now(),
		Edges:     int(afl.stats.TotalEdges),
		NewEdges:  int(afl.stats.NewPaths),
	}

	return coverage, nil
}

// GetCorpus retrieves corpus entries
func (afl *AFLPlusPlus) GetCorpus() ([]*CorpusEntry, error) {
	corpus := make([]*CorpusEntry, 0)

	// Read corpus from AFL++ queue directory
	if _, err := os.Stat(afl.corpusDir); err == nil {
		files, err := os.ReadDir(afl.corpusDir)
		if err != nil {
			return nil, err
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}

			info, err := file.Info()
			if err != nil {
				continue
			}

			entry := &CorpusEntry{
				ID:        file.Name(),
				FileName:  file.Name(),
				Size:      info.Size(),
				Timestamp: info.ModTime(),
				Source:    "afl_queue",
			}

			corpus = append(corpus, entry)
		}
	}

	return corpus, nil
}

// SetEventHandler sets the event handler for fuzzer events
func (afl *AFLPlusPlus) SetEventHandler(handler EventHandler) {
	afl.mu.Lock()
	defer afl.mu.Unlock()

	afl.eventHandler = handler
}

// Cleanup cleans up fuzzer resources
func (afl *AFLPlusPlus) Cleanup() error {
	// Stop if running
	if afl.IsRunning() {
		if err := afl.Stop(); err != nil {
			return err
		}
	}

	// Remove temporary files if configured
	if cleanTemp, ok := afl.config.FuzzerOptions["clean_temp"].(bool); ok && cleanTemp {
		if err := os.RemoveAll(afl.outputDir); err != nil {
			afl.logger.WithError(err).Warn("Failed to clean temporary files")
		}
	}

	return nil
}

// Private helper methods

func (afl *AFLPlusPlus) validateConfig(config FuzzConfig) error {
	if config.Target == "" {
		return &FuzzerError{
			Type:    ErrInvalidConfig,
			Message: "target binary is required",
			Fuzzer:  afl.Name(),
			Code:    17,
		}
	}

	if config.OutputDirectory == "" {
		return &FuzzerError{
			Type:    ErrInvalidConfig,
			Message: "output directory is required",
			Fuzzer:  afl.Name(),
			Code:    18,
		}
	}

	if config.MemoryLimit <= 0 {
		config.MemoryLimit = 1024 // Default 1GB
	}

	if config.Timeout <= 0 {
		config.Timeout = 1000 * time.Millisecond // Default 1s timeout
	}

	return nil
}

// isInstrumented checks if the binary is instrumented for AFL++
func (afl *AFLPlusPlus) isInstrumented() bool {
	// Check for AFL++ instrumentation signatures in the binary
	cmd := exec.Command("strings", afl.config.Target)
	output, err := cmd.Output()
	if err != nil {
		afl.logger.WithError(err).Warn("Failed to check binary instrumentation, assuming not instrumented")
		return false
	}

	// Look for AFL++ instrumentation markers
	outputStr := string(output)
	aflMarkers := []string{
		"__afl_",
		"__AFL_",
		"afl-compiler-rt",
		"__sanitizer_cov_",
		"SanitizerCoverage",
	}

	for _, marker := range aflMarkers {
		if strings.Contains(outputStr, marker) {
			afl.logger.Debug("Found AFL++ instrumentation marker in binary")
			return true
		}
	}

	afl.logger.Debug("No AFL++ instrumentation detected in binary, will use dumb mode")
	return false
}

func (afl *AFLPlusPlus) buildAFLArgs() []string {
	afl.logger.Debug("buildAFLArgs called")
	args := []string{}

	// Check if binary is instrumented, if not use dumb mode
	afl.logger.WithField("target", afl.config.Target).Debug("Checking if binary is instrumented")
	if afl.config.Target != "" && !afl.isInstrumented() {
		args = append(args, "-n")
		afl.logger.Debug("Binary is not instrumented, running AFL++ in dumb mode (-n)")
	} else if afl.config.Target != "" {
		afl.logger.Debug("Binary is instrumented, running AFL++ with instrumentation feedback")
	} else {
		afl.logger.Debug("Target not set, cannot check instrumentation")
	}

	// Input directory
	// The work directory is the parent of the parent of outputDir
	// outputDir is like /app/work/jobs/job_XXX/output/afl_output
	// workDir should be /app/work/jobs/job_XXX
	workDir := filepath.Dir(filepath.Dir(afl.outputDir))
	inputDir := filepath.Join(workDir, "input")

	afl.logger.WithFields(logrus.Fields{
		"output_dir": afl.outputDir,
		"work_dir":   workDir,
		"input_dir":  inputDir,
		"seed_dir":   afl.config.SeedDirectory,
	}).Debug("Calculated directories for AFL++")

	// Determine seed directory to use
	seedDir := afl.config.SeedDirectory
	if seedDir == "" {
		seedDir = inputDir
	}

	// Ensure seed directory exists
	if err := os.MkdirAll(seedDir, 0755); err != nil {
		afl.logger.WithError(err).Error("Failed to create seed directory")
	}

	// Check if seed directory is empty
	entries, err := os.ReadDir(seedDir)
	if err != nil {
		afl.logger.WithError(err).Error("Failed to read seed directory")
	}

	// If directory is empty, create a default seed
	if len(entries) == 0 {
		afl.logger.WithField("seed_dir", seedDir).Debug("Seed directory is empty, creating default seed")
		seedFile := filepath.Join(seedDir, "seed01.txt")
		if err := os.WriteFile(seedFile, []byte("0"), 0644); err != nil {
			afl.logger.WithError(err).Error("Failed to create seed file")
		} else {
			afl.logger.WithField("seed_file", seedFile).Info("Created default seed file")
		}
	}

	args = append(args, "-i", seedDir)

	// Output directory
	args = append(args, "-o", afl.outputDir)

	// Memory limit - AFL++ expects MB, config is in bytes
	if afl.config.MemoryLimit > 0 {
		memMB := afl.config.MemoryLimit / (1024 * 1024)
		if memMB == 0 {
			// If less than 1MB, use minimum of 512MB for AFL++
			memMB = 512
		}
		args = append(args, "-m", fmt.Sprintf("%d", memMB))
	} else {
		// Default to 512MB if not specified
		args = append(args, "-m", "512")
	}

	// Timeout (per test case execution timeout)
	timeoutMs := afl.config.Timeout.Milliseconds()
	if timeoutMs <= 0 {
		timeoutMs = 1000 // Default to 1 second
	}
	afl.logger.WithField("timeout_ms", timeoutMs).Debug("Setting AFL++ timeout")
	args = append(args, "-t", fmt.Sprintf("%d", timeoutMs))

	// Time-limited fuzzing (graceful exit after specified duration)
	// Use AFL++'s -V flag to run for a specific duration then exit gracefully
	if afl.config.Duration > 0 {
		seconds := int(afl.config.Duration.Seconds())
		if seconds > 0 {
			args = append(args, "-V", fmt.Sprintf("%d", seconds))
			afl.logger.WithField("duration_seconds", seconds).Info("AFL++ will run for limited time and exit gracefully")
		}
	}

	// Dictionary
	if afl.config.Dictionary != "" {
		args = append(args, "-x", afl.config.Dictionary)
	}

	// AFL++ specific options
	if options, ok := afl.config.FuzzerOptions["afl_args"].([]string); ok {
		args = append(args, options...)
	}

	// Deterministic mode
	if deterministic, ok := afl.config.FuzzerOptions["deterministic"].(bool); ok && !deterministic {
		args = append(args, "-d")
	}

	// Target binary and arguments
	args = append(args, "--")
	args = append(args, afl.config.Target)
	args = append(args, afl.config.TargetArgs...)

	return args
}

func (afl *AFLPlusPlus) monitorOutput(pipe io.Reader, name string) {
	defer afl.wg.Done()

	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		afl.logger.WithField("stream", name).Debug(line)

		// Write to OutputWriter if configured
		if afl.config.OutputWriter != nil {
			// Write the line with a newline
			fmt.Fprintf(afl.config.OutputWriter, "[%s] %s\n", name, line)
		}

		// Check for important messages
		if strings.Contains(line, "Looks like there are no valid") {
			afl.logger.Warn("No valid test cases in input directory")
		}
		if strings.Contains(line, "PROGRAM ABORT") {
			afl.logger.Error("AFL++ encountered an error")
		}

		// Check for crash indicators
		if strings.Contains(line, "Saved crash") || strings.Contains(line, "crash found") {
			afl.detectAndEmitCrash(line)
		}
	}
}

func (afl *AFLPlusPlus) monitorProcess() {
	defer afl.wg.Done()

	// Wait for process to exit
	err := afl.cmd.Wait()

	afl.mu.Lock()
	defer afl.mu.Unlock()

	if err != nil {
		afl.logger.WithError(err).Warn("AFL++ process exited with error")
		afl.status = StatusError
		if afl.eventHandler != nil {
			afl.eventHandler.OnError(afl, err)
		}
		// Emit error event through base fuzzer
		afl.EmitErrorEvent(afl.ctx, afl.config.Target, err)
	} else {
		afl.status = StatusCompleted
	}

	// Notify completion
	if afl.eventHandler != nil {
		reason := "completed"
		if afl.ctx.Err() != nil {
			reason = "cancelled"
		}
		afl.eventHandler.OnStop(afl, reason)
	}

	// Emit stopped event through base fuzzer
	reason := "completed"
	if afl.ctx.Err() != nil {
		reason = "cancelled"
	}
	afl.EmitStoppedEvent(context.Background(), afl.config.Target, reason)
}

func (afl *AFLPlusPlus) startStatsMonitoring() {
	interval := 5 * time.Second
	if statsInterval, ok := afl.config.FuzzerOptions["stats_interval"].(time.Duration); ok {
		interval = statsInterval
	}

	afl.monitorTicker = time.NewTicker(interval)

	afl.wg.Add(1)
	go func() {
		defer afl.wg.Done()

		for {
			select {
			case <-afl.ctx.Done():
				return
			case <-afl.monitorTicker.C:
				afl.updateStats()
			}
		}
	}()
}

func (afl *AFLPlusPlus) updateStats() {
	// Read AFL++ stats file
	data, err := os.ReadFile(afl.statsFile)
	if err != nil {
		return
	}

	afl.mu.Lock()
	defer afl.mu.Unlock()

	// Parse stats for enhanced metrics
	parsedStats := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		parsedStats[key] = value

		switch key {
		case "execs_done":
			if val, err := strconv.ParseInt(value, 10, 64); err == nil {
				afl.stats.Executions = val
			}
		case "execs_per_sec":
			if val, err := strconv.ParseFloat(value, 64); err == nil {
				afl.stats.ExecPerSecond = val
				// Track peak execution speed
				if val > afl.peakExecSpeed {
					afl.peakExecSpeed = val
				}
				// Add to history
				afl.execHistory = append(afl.execHistory, val)
				if len(afl.execHistory) > 100 {
					afl.execHistory = afl.execHistory[1:]
				}
			}
		case "paths_total":
			if val, err := strconv.ParseInt(value, 10, 64); err == nil {
				afl.stats.PathsTotal = int(val)
			}
		case "paths_favored":
			if val, err := strconv.ParseInt(value, 10, 64); err == nil {
				afl.stats.NewPaths = int(val)
			}
		case "unique_crashes":
			if val, err := strconv.ParseInt(value, 10, 64); err == nil {
				afl.stats.UniqueCrashes = int(val)
			}
		case "corpus_count":
			if val, err := strconv.ParseInt(value, 10, 64); err == nil {
				afl.stats.CorpusSize = int(val)
			}
		case "bitmap_cvg":
			if strings.HasSuffix(value, "%") {
				percentStr := strings.TrimSuffix(value, "%")
				if val, err := strconv.ParseFloat(percentStr, 64); err == nil {
					afl.stats.CoveragePercent = val
				}
			}
		}
	}

	// Collect enhanced metrics
	afl.collectEnhancedMetrics(parsedStats)

	// Update derived stats
	afl.stats.ElapsedTime = time.Since(afl.stats.StartTime)
	if afl.stats.Executions > 0 && afl.stats.ElapsedTime.Seconds() > 0 {
		afl.stats.ExecPerSecond = float64(afl.stats.Executions) / afl.stats.ElapsedTime.Seconds()
	}

	// Get system resource usage
	if afl.cmd != nil && afl.cmd.Process != nil {
		// This would require platform-specific code to get actual CPU/memory usage
		// For now, using placeholder values
		afl.stats.CPUUsage = 50.0
		afl.stats.MemoryUsage = afl.config.MemoryLimit * 1024 * 1024
	}

	// Record execution for performance tracking
	if afl.lastExecCount > 0 {
		execDiff := afl.stats.Executions - afl.lastExecCount
		timeDiff := time.Since(afl.lastStatsUpdate)
		if timeDiff > 0 && execDiff > 0 {
			avgExecTime := timeDiff / time.Duration(execDiff)
			afl.metricsCollector.RecordExecution(avgExecTime)
		}
	}
	afl.lastExecCount = afl.stats.Executions
	afl.lastStatsUpdate = time.Now()

	// Notify event handler
	if afl.eventHandler != nil {
		afl.eventHandler.OnStats(afl, afl.stats)
		afl.eventHandler.OnProgress(afl, afl.GetProgress())
	}

	// Emit stats event through base fuzzer
	afl.parseAndEmitStats(map[string]string{
		"execs_done":     strconv.FormatInt(afl.stats.Executions, 10),
		"execs_per_sec":  strconv.FormatFloat(afl.stats.ExecPerSecond, 'f', 2, 64),
		"paths_total":    strconv.Itoa(afl.stats.PathsTotal),
		"unique_crashes": strconv.Itoa(afl.stats.UniqueCrashes),
		"bitmap_cvg":     fmt.Sprintf("%.2f%%", afl.stats.CoveragePercent),
	})

	// Check for new crashes
	afl.checkForNewCrashes()
}

func (afl *AFLPlusPlus) checkForNewCrashes() {
	crashes, err := afl.GetCrashes()
	if err != nil {
		return
	}

	// Check if we have new crashes since last report
	if len(crashes) > afl.lastReportedCrashes {
		// Report all new crashes since last check
		newCrashes := crashes[afl.lastReportedCrashes:]

		afl.logger.WithFields(logrus.Fields{
			"new_crashes":   len(newCrashes),
			"total_crashes": len(crashes),
			"job_id":        afl.config.JobID,
		}).Info("Found new crashes during periodic check")

		// Update stats
		afl.stats.TotalCrashes = len(crashes)
		afl.stats.UniqueCrashes = len(crashes) // Assuming all are unique for now
		afl.stats.LastCrash = time.Now()

		// Report each new crash
		for i, crash := range newCrashes {
			afl.logger.WithFields(logrus.Fields{
				"crash_index": afl.lastReportedCrashes + i,
				"crash_id":    crash.ID,
				"crash_hash":  crash.Hash,
				"crash_type":  crash.Type,
			}).Debug("Reporting new crash")

			if afl.eventHandler != nil {
				afl.eventHandler.OnCrash(afl, crash)
			}
			// Also emit crash event through base fuzzer
			afl.detectAndEmitCrash(crash)
		}

		// Update the last reported count
		afl.lastReportedCrashes = len(crashes)
	}
}

func (afl *AFLPlusPlus) getPhase() string {
	if afl.stats.Executions < 10000 {
		return "calibration"
	} else if afl.stats.Executions < 100000 {
		return "deterministic"
	} else {
		return "havoc"
	}
}

func (afl *AFLPlusPlus) calculateProgress() float64 {
	if afl.config.Duration > 0 {
		elapsed := time.Since(afl.stats.StartTime)
		progress := elapsed.Seconds() / afl.config.Duration.Seconds() * 100
		if progress > 100 {
			progress = 100
		}
		return progress
	}

	// If no duration set, use execution count
	if afl.config.MaxExecutions > 0 {
		progress := float64(afl.stats.Executions) / float64(afl.config.MaxExecutions) * 100
		if progress > 100 {
			progress = 100
		}
		return progress
	}

	return 0
}

func (afl *AFLPlusPlus) getCurrentInput() string {
	// AFL++ doesn't easily expose current input being tested
	// Would need to monitor the .cur_input file
	curInputFile := filepath.Join(afl.outputDir, ".cur_input")
	if data, err := os.ReadFile(curInputFile); err == nil {
		if len(data) > 50 {
			return fmt.Sprintf("%x...", data[:50])
		}
		return fmt.Sprintf("%x", data)
	}

	return "unknown"
}

func (afl *AFLPlusPlus) getPeakExecSpeed() float64 {
	return afl.peakExecSpeed
}

// collectEnhancedMetrics collects and updates enhanced metrics
func (afl *AFLPlusPlus) collectEnhancedMetrics(parsedStats map[string]string) {
	// Coverage metrics
	coverage := common.CoverageMetrics{
		LineCoverage:     afl.stats.CoveragePercent,
		BranchCoverage:   afl.stats.CoveragePercent, // AFL++ bitmap coverage
		NewEdgesFound:    int64(afl.stats.NewPaths),
		TotalEdges:       int64(afl.stats.TotalEdges),
		CoverageByModule: make(map[string]float64),
	}

	// Parse additional coverage info from stats
	if edges, exists := parsedStats["edges_found"]; exists {
		if val, err := strconv.ParseInt(edges, 10, 64); err == nil {
			coverage.TotalEdges = val
		}
	}

	// Calculate coverage growth rate
	if afl.stats.ElapsedTime.Seconds() > 0 {
		coverage.CoverageGrowth = float64(afl.stats.NewPaths) / afl.stats.ElapsedTime.Seconds()
	}

	// Performance metrics
	performance := common.PerformanceMetrics{
		ExecutionsPerSecond: afl.stats.ExecPerSecond,
		ThroughputMBps:      afl.calculateThroughput(),
	}

	// Calculate mutation efficiency from AFL++ stats
	if pending, exists := parsedStats["pending_total"]; exists {
		if pendingVal, err := strconv.ParseInt(pending, 10, 64); err == nil {
			if afl.stats.PathsTotal > 0 {
				performance.MutationEfficiency = float64(afl.stats.PathsTotal-int(pendingVal)) / float64(afl.stats.PathsTotal)
			}
		}
	}

	// Queue utilization
	if queueCur, exists := parsedStats["queue_cur"]; exists {
		if curVal, err := strconv.ParseInt(queueCur, 10, 64); err == nil {
			if afl.stats.PathsTotal > 0 {
				performance.QueueUtilization = float64(curVal) / float64(afl.stats.PathsTotal)
			}
		}
	}

	// Update metrics collector
	afl.metricsCollector.UpdateCoverageMetrics(coverage)
	afl.metricsCollector.UpdatePerformanceMetrics(performance)

	// Add AFL++ specific metrics
	afl.metricsCollector.UpdateFuzzerSpecificMetrics("stability", parsedStats["stability"])
	afl.metricsCollector.UpdateFuzzerSpecificMetrics("variable_paths", parsedStats["variable_paths"])
	afl.metricsCollector.UpdateFuzzerSpecificMetrics("max_depth", parsedStats["max_depth"])
	afl.metricsCollector.UpdateFuzzerSpecificMetrics("pending_favs", parsedStats["pending_favs"])
	afl.metricsCollector.UpdateFuzzerSpecificMetrics("pending_total", parsedStats["pending_total"])
	afl.metricsCollector.UpdateFuzzerSpecificMetrics("cycles_done", parsedStats["cycles_done"])
	afl.metricsCollector.UpdateFuzzerSpecificMetrics("bit_flips", parsedStats["bit_flips"])
	afl.metricsCollector.UpdateFuzzerSpecificMetrics("byte_flips", parsedStats["byte_flips"])
	afl.metricsCollector.UpdateFuzzerSpecificMetrics("arithmetics", parsedStats["arithmetics"])
	afl.metricsCollector.UpdateFuzzerSpecificMetrics("known_ints", parsedStats["known_ints"])
	afl.metricsCollector.UpdateFuzzerSpecificMetrics("dictionary", parsedStats["dictionary"])
	afl.metricsCollector.UpdateFuzzerSpecificMetrics("havoc", parsedStats["havoc"])
	afl.metricsCollector.UpdateFuzzerSpecificMetrics("splice", parsedStats["splice"])
}

// calculateThroughput calculates throughput in MB/s
func (afl *AFLPlusPlus) calculateThroughput() float64 {
	if afl.stats.ElapsedTime.Seconds() == 0 {
		return 0
	}

	// Estimate average input size (would need actual tracking)
	avgInputSize := 1024.0 // 1KB average estimate
	totalBytes := float64(afl.stats.Executions) * avgInputSize
	totalMB := totalBytes / (1024 * 1024)

	return totalMB / afl.stats.ElapsedTime.Seconds()
}

func (afl *AFLPlusPlus) getExitReason() string {
	switch afl.status {
	case StatusCompleted:
		return "completed successfully"
	case StatusError:
		return "exited with error"
	case StatusStopped:
		return "stopped by user"
	default:
		return "unknown"
	}
}

func (afl *AFLPlusPlus) hashInput(data []byte) string {
	// Simple hash implementation
	// In production, use proper crypto hash
	hash := uint32(0)
	for _, b := range data {
		hash = hash*31 + uint32(b)
	}
	return fmt.Sprintf("%08x", hash)
}

func (afl *AFLPlusPlus) detectCrashType(filename string) string {
	// Return "afl++" for all AFL++ crashes
	return "afl++"
}

// parseAndEmitStats parses AFL++ stats and emits stats event
func (afl *AFLPlusPlus) parseAndEmitStats(stats map[string]string) {
	// Emit stats event through base fuzzer
	afl.EmitStatsEvent(afl.ctx, afl.config.Target, afl.stats)
}

// detectAndEmitCrash detects crashes and emits crash event
func (afl *AFLPlusPlus) detectAndEmitCrash(crashOrLine interface{}) {
	switch v := crashOrLine.(type) {
	case *common.CrashResult:
		// Emit through event handler - this includes the full crash data
		if afl.eventHandler != nil {
			afl.eventHandler.OnCrash(afl, v)
		}
	case string:
		// Parse crash info from output line
		afl.logger.WithField("line", v).Debug("Detected crash in output")
		// Could parse more details from the line if needed
	}
}

// monitorCorpusChanges monitors corpus directory for changes and emits corpus events
func (afl *AFLPlusPlus) monitorCorpusChanges() {
	// This could be called periodically to check for corpus updates
	files, err := os.ReadDir(afl.corpusDir)
	if err != nil {
		return
	}

	fileNames := make([]string, 0, len(files))
	var totalSize int64
	for _, file := range files {
		if !file.IsDir() {
			fileNames = append(fileNames, file.Name())
			if info, err := file.Info(); err == nil {
				totalSize += info.Size()
			}
		}
	}

	if len(fileNames) > 0 {
		corpusUpdate := &common.CorpusUpdate{
			ID:        fmt.Sprintf("afl_corpus_%d", time.Now().Unix()),
			JobID:     afl.config.Target,
			BotID:     afl.botID,
			Files:     fileNames,
			Timestamp: time.Now(),
			TotalSize: totalSize,
		}

		// Emit corpus update event
		afl.EmitCorpusUpdateEvent(afl.ctx, afl.config.Target, corpusUpdate)
	}
}

// ReproduceCrash attempts to reproduce a crash with the given input
func (afl *AFLPlusPlus) ReproduceCrash(ctx context.Context, crashInput []byte, config ReproductionConfig) (*common.ReproductionResult, error) {
	afl.logger.WithFields(logrus.Fields{
		"crash_size": len(crashInput),
		"attempts":   config.Attempts,
		"timeout":    config.Timeout,
	}).Info("Starting crash reproduction with AFL++")

	// Default attempts if not specified
	if config.Attempts <= 0 {
		config.Attempts = 3
	}

	// Default timeout if not specified
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}

	// Create temporary directory for reproduction
	tempDir, err := os.MkdirTemp("", "afl_reproduce_*")
	if err != nil {
		return nil, &FuzzerError{
			Type:    ErrInternal,
			Message: fmt.Sprintf("failed to create temp directory: %v", err),
			Fuzzer:  afl.Name(),
			Code:    100,
		}
	}
	defer os.RemoveAll(tempDir)

	// Save crash input to file
	crashFile := filepath.Join(tempDir, "crash_input")
	if err := os.WriteFile(crashFile, crashInput, 0644); err != nil {
		return nil, &FuzzerError{
			Type:    ErrInternal,
			Message: fmt.Sprintf("failed to write crash input: %v", err),
			Fuzzer:  afl.Name(),
			Code:    101,
		}
	}

	// Try to reproduce the crash multiple times
	for attempt := 1; attempt <= config.Attempts; attempt++ {
		afl.logger.WithField("attempt", attempt).Debug("Attempting crash reproduction")

		// Create a context with timeout for this attempt
		attemptCtx, cancel := context.WithTimeout(ctx, config.Timeout)
		defer cancel()

		// Build command to run target directly with crash input
		// AFL++ doesn't have a crash reproduction mode, so we run the target directly
		var args []string

		// Add target arguments, replacing @@ with the crash file path
		for _, arg := range afl.config.TargetArgs {
			if arg == "@@" {
				args = append(args, crashFile)
			} else {
				args = append(args, arg)
			}
		}

		// If no @@ was found, assume input is via stdin
		var cmd *exec.Cmd
		if contains(afl.config.TargetArgs, "@@") {
			cmd = exec.CommandContext(attemptCtx, afl.config.Target, args...)
		} else {
			// Input via stdin
			cmd = exec.CommandContext(attemptCtx, afl.config.Target, args...)
			cmd.Stdin = strings.NewReader(string(crashInput))
		}

		// Set environment variables if provided
		if config.Environment != nil {
			env := os.Environ()
			for k, v := range config.Environment {
				env = append(env, fmt.Sprintf("%s=%s", k, v))
			}
			cmd.Env = env
		}

		// Start timing
		startTime := time.Now()

		// Capture output
		output, err := cmd.CombinedOutput()

		// Calculate execution time
		executionTime := time.Since(startTime)

		// Determine if crash was reproduced
		reproduced := false
		var signal int
		var exitCode int

		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
				// Check if it's a signal-based crash
				if exitCode == -1 {
					// Process was killed by signal
					if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
						if status.Signaled() {
							signal = int(status.Signal())
							reproduced = true
						}
					}
				} else if exitCode != 0 {
					// Non-zero exit code might indicate crash
					reproduced = true
				}
			}
		}

		// Extract stack trace from output
		stackTrace := extractStackTrace(string(output))
		stackHash := afl.hashInput([]byte(stackTrace))

		// Build environment info
		envInfo := make(map[string]string)
		envInfo["fuzzer"] = "AFL++"
		envInfo["version"] = afl.Version()
		envInfo["bot_id"] = afl.botID
		envInfo["attempt"] = strconv.Itoa(attempt)

		result := &common.ReproductionResult{
			ID:              fmt.Sprintf("afl_repro_%d_%d", time.Now().Unix(), attempt),
			RequestID:       config.OriginalCrashID,
			CrashID:         config.OriginalCrashID,
			BotID:           afl.botID,
			AttemptNumber:   attempt,
			Status:          common.ReproducibilityStatusConfirmed,
			Reproduced:      reproduced,
			ExecutionTime:   executionTime,
			Signal:          signal,
			ExitCode:        exitCode,
			Output:          string(output),
			StackTrace:      stackTrace,
			StackHash:       stackHash,
			MatchesOriginal: true, // This would need to be compared with original crash
			EnvironmentInfo: envInfo,
			Timestamp:       time.Now(),
		}

		if reproduced {
			afl.logger.WithFields(logrus.Fields{
				"attempt":    attempt,
				"signal":     signal,
				"exit_code":  exitCode,
				"stack_hash": stackHash,
			}).Info("Successfully reproduced crash with AFL++")
			return result, nil
		}

		afl.logger.WithField("attempt", attempt).Debug("Crash not reproduced in this attempt")

		// If this was the last attempt and we didn't reproduce
		if attempt == config.Attempts {
			result.Status = common.ReproducibilityStatusFailed
			return result, nil
		}
	}

	// Should not reach here, but just in case
	return &common.ReproductionResult{
		ID:              fmt.Sprintf("afl_repro_%d_failed", time.Now().Unix()),
		RequestID:       config.OriginalCrashID,
		CrashID:         config.OriginalCrashID,
		BotID:           afl.botID,
		Status:          common.ReproducibilityStatusFailed,
		Reproduced:      false,
		EnvironmentInfo: map[string]string{"fuzzer": "AFL++"},
		Timestamp:       time.Now(),
	}, nil
}

// Helper function to check if slice contains string
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// Helper function to extract stack trace from output
func extractStackTrace(output string) string {
	// Look for common stack trace indicators
	lines := strings.Split(output, "\n")
	var stackLines []string
	inStack := false

	for _, line := range lines {
		// Common stack trace patterns
		if strings.Contains(line, "Stack trace:") ||
			strings.Contains(line, "backtrace:") ||
			strings.Contains(line, "#0 ") ||
			strings.Contains(line, "at 0x") {
			inStack = true
		}

		if inStack {
			stackLines = append(stackLines, line)
			// Stop at empty line or common end markers
			if line == "" || strings.Contains(line, "===") {
				break
			}
		}
	}

	return strings.Join(stackLines, "\n")
}

// GetEnhancedMetrics returns enhanced metrics from the fuzzer
func (afl *AFLPlusPlus) GetEnhancedMetrics() *common.EnhancedMetrics {
	afl.mu.RLock()
	defer afl.mu.RUnlock()

	if afl.metricsCollector != nil {
		return afl.metricsCollector.GetMetrics()
	}

	return nil
}

// CollectCoverageData collects coverage data for AFL++
func (a *AFLPlusPlus) CollectCoverageData() (map[string]interface{}, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	a.logger.Debug("DEBUG: CollectCoverageData called")

	if a.config.Coverage == "" {
		return nil, fmt.Errorf("coverage collection not enabled")
	}

	coverageData := make(map[string]interface{})

	// Collect basic AFL++ coverage statistics from fuzzer stats
	// Try multiple possible locations for the stats file
	possibleStatsFiles := []string{
		filepath.Join(a.outputDir, "afl_output", "default", "fuzzer_stats"),
		filepath.Join(a.outputDir, "afl_output", "fuzzer_stats"),
		filepath.Join(a.outputDir, "fuzzer_stats"),
	}

	var statsData []byte
	var statsErr error
	for _, statsFile := range possibleStatsFiles {
		if data, err := os.ReadFile(statsFile); err == nil {
			statsData = data
			a.logger.WithField("stats_file", statsFile).Debug("Found AFL++ stats file")
			break
		} else {
			statsErr = err
		}
	}

	if statsData != nil {
		stats := make(map[string]string)
		lines := strings.Split(string(statsData), "\n")
		for _, line := range lines {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				stats[key] = value
			}
		}

		// Extract coverage metrics - check for both old and new key names
		if val, ok := stats["edges_found"]; ok {
			a.logger.WithFields(logrus.Fields{
				"edges_found_raw": val,
				"type":            fmt.Sprintf("%T", val),
			}).Debug("DEBUG: Found edges_found in stats")

			coverageData["edges_found"] = val
			// Convert edges to approximate line coverage percentage
			if edges, err := strconv.ParseInt(val, 10, 64); err == nil && edges > 0 {
				// Rough approximation: assume we're covering some percentage based on edges
				lineCov := float64(edges) * 2.0
				coverageData["line_coverage"] = lineCov // Each edge ~ 2% coverage
				coverageData["coverage_percent"] = lineCov

				a.logger.WithFields(logrus.Fields{
					"edges_parsed":  edges,
					"line_coverage": lineCov,
					"type":          fmt.Sprintf("%T", lineCov),
				}).Debug("DEBUG: Calculated coverage from edges")
			} else if err != nil {
				a.logger.WithError(err).WithField("val", val).Debug("DEBUG: Failed to parse edges_found")
			}
		}
		if val, ok := stats["bitmap_cvg"]; ok {
			coverageData["bitmap_coverage"] = val
		}
		if val, ok := stats["paths_total"]; ok {
			coverageData["paths_total"] = val
		}
		if val, ok := stats["unique_crashes"]; ok {
			coverageData["unique_crashes"] = val
		}
		if val, ok := stats["unique_hangs"]; ok {
			coverageData["unique_hangs"] = val
		}
		if val, ok := stats["corpus_count"]; ok {
			coverageData["corpus_count"] = val
		}
		if val, ok := stats["exec_timeout"]; ok {
			coverageData["exec_timeout"] = val
		}
		if val, ok := stats["max_depth"]; ok {
			coverageData["max_depth"] = val
		}
	} else if statsErr != nil {
		a.logger.WithError(statsErr).Debug("Could not read AFL++ stats file from any location")
	}

	// Try to collect LLVM coverage if available
	if a.isLLVMMode() {
		if err := a.collectLLVMCoverage(coverageData); err != nil {
			a.logger.WithError(err).Debug("Failed to collect LLVM coverage")
		}
	}

	// Add metadata
	coverageData["timestamp"] = time.Now().Unix()
	coverageData["collected_at"] = time.Now().Format(time.RFC3339)
	coverageData["fuzzer"] = "afl++"
	coverageData["format"] = "afl"

	// Add current stats
	coverageData["total_executions"] = a.stats.Executions
	coverageData["exec_per_second"] = a.stats.ExecPerSecond
	coverageData["coverage_percent"] = a.stats.CoveragePercent

	return coverageData, nil
}

// isLLVMMode checks if AFL++ is running in LLVM mode
func (a *AFLPlusPlus) isLLVMMode() bool {
	// Check if we're using afl-clang-fast or afl-clang-lto
	return strings.Contains(a.config.Target, "afl-clang") ||
		strings.Contains(os.Getenv("AFL_USE_LLVM"), "1")
}

// collectLLVMCoverage collects LLVM-based coverage data
func (a *AFLPlusPlus) collectLLVMCoverage(coverageData map[string]interface{}) error {
	// Check for llvm-cov output
	llvmCovFile := filepath.Join(a.outputDir, "coverage.json")
	if _, err := os.Stat(llvmCovFile); err == nil {
		if data, err := os.ReadFile(llvmCovFile); err == nil {
			coverageData["llvm_coverage"] = string(data)
			return nil
		}
	}

	// Try to generate LLVM coverage report
	cmd := exec.Command("llvm-cov", "report", a.config.Target,
		"-instr-profile", filepath.Join(a.outputDir, "default.profdata"))
	output, err := cmd.Output()
	if err == nil {
		coverageData["llvm_coverage_report"] = string(output)
	}

	return err
}

// CreateAFLPlusPlus creates a new AFL++ instance with optional logger
func CreateAFLPlusPlus(logger *logrus.Logger) (Fuzzer, error) {
	return NewAFLPlusPlus(logger), nil
}
