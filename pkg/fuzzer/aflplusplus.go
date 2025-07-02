package fuzzer

import (
	"bufio"
	"context"
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
	*BaseFuzzer   // Embed base fuzzer for event handling
	config        FuzzConfig
	status        FuzzerStatus
	logger        *logrus.Logger
	cmd           *exec.Cmd
	ctx           context.Context
	cancel        context.CancelFunc
	eventHandler  EventHandler
	stats         FuzzerStats
	statsFile     string
	outputDir     string
	crashDir      string
	corpusDir     string
	mu            sync.RWMutex
	monitorTicker *time.Ticker
	wg            sync.WaitGroup
	botID         string
}

// Compile-time interface compliance check
var _ Fuzzer = (*AFLPlusPlus)(nil)

// NewAFLPlusPlus creates a new AFL++ fuzzer instance
func NewAFLPlusPlus(logger *logrus.Logger) *AFLPlusPlus {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	return &AFLPlusPlus{
		BaseFuzzer:   NewBaseFuzzer(logger),
		status:       StatusUninitialized,
		logger:       logger,
		eventHandler: &DefaultEventHandler{},
		stats: FuzzerStats{
			StartTime: time.Now(),
		},
	}
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

	// Send SIGTERM to AFL++
	if afl.cmd != nil && afl.cmd.Process != nil {
		if err := afl.cmd.Process.Signal(os.Interrupt); err != nil {
			afl.logger.WithError(err).Warn("Failed to send interrupt signal")
			// Force kill if interrupt fails
			afl.cmd.Process.Kill()
		}
	}

	// Wait for goroutines to finish
	afl.wg.Wait()

	afl.status = StatusStopped

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
				ID:        file.Name(),
				JobID:     afl.config.Target,
				BotID:     afl.botID,
				Timestamp: info.ModTime(),
				FilePath:  filepath.Join(afl.crashDir, file.Name()),
				Size:      int64(len(crashData)),
				Hash:      crashHash,
				Type:      crashType,
				Input:     crashData, // Include the crash input data
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

func (afl *AFLPlusPlus) buildAFLArgs() []string {
	args := []string{}

	// Input directory
	if afl.config.SeedDirectory != "" {
		args = append(args, "-i", afl.config.SeedDirectory)
	} else {
		// Create minimal seed if none provided
		seedDir := filepath.Join(afl.outputDir, "seeds")
		os.MkdirAll(seedDir, 0755)
		seedFile := filepath.Join(seedDir, "seed")
		os.WriteFile(seedFile, []byte("0"), 0644)
		args = append(args, "-i", seedDir)
	}

	// Output directory
	args = append(args, "-o", afl.outputDir)

	// Memory limit
	args = append(args, "-m", fmt.Sprintf("%d", afl.config.MemoryLimit))

	// Timeout
	timeoutMs := afl.config.Timeout.Milliseconds()
	args = append(args, "-t", fmt.Sprintf("%d", timeoutMs))

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

	// Parse stats
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "execs_done":
			if val, err := strconv.ParseInt(value, 10, 64); err == nil {
				afl.stats.Executions = val
			}
		case "execs_per_sec":
			if val, err := strconv.ParseFloat(value, 64); err == nil {
				afl.stats.ExecPerSecond = val
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

	// Simple check: if crash count increased, notify about latest crash
	if len(crashes) > afl.stats.TotalCrashes {
		afl.stats.TotalCrashes = len(crashes)
		afl.stats.LastCrash = time.Now()

		if afl.eventHandler != nil && len(crashes) > 0 {
			// Notify about the latest crash
			afl.eventHandler.OnCrash(afl, crashes[len(crashes)-1])
		}

		// Emit crash event through base fuzzer
		if len(crashes) > 0 {
			afl.detectAndEmitCrash(crashes[len(crashes)-1])
		}
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
	// Would need to track this over time
	// For now, return current speed as peak
	return afl.stats.ExecPerSecond
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
	// AFL++ includes crash type in filename
	if strings.Contains(filename, "sig:11") {
		return "segmentation_fault"
	} else if strings.Contains(filename, "sig:06") {
		return "abort"
	} else if strings.Contains(filename, "sig:08") {
		return "arithmetic_exception"
	} else if strings.Contains(filename, "timeout") {
		return "timeout"
	}

	return "unknown"
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
		// Emit crash event through base fuzzer
		afl.EmitCrashFoundEvent(afl.ctx, afl.config.Target, v)
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

// CreateAFLPlusPlus creates a new AFL++ instance with optional logger
func CreateAFLPlusPlus(logger *logrus.Logger) (Fuzzer, error) {
	return NewAFLPlusPlus(logger), nil
}
