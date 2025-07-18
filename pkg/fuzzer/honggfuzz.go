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

// Honggfuzz implements the Fuzzer interface for Honggfuzz
type Honggfuzz struct {
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
}

// Compile-time interface compliance check
var _ Fuzzer = (*Honggfuzz)(nil)

// NewHonggfuzz creates a new Honggfuzz fuzzer instance
func NewHonggfuzz(logger *logrus.Logger) *Honggfuzz {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	hf := &Honggfuzz{
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
	hf.metricsCollector = NewDefaultMetricsCollector(
		logger.WithField("component", "honggfuzz_metrics"),
		5*time.Second,
	)

	return hf
}

// Name returns the name of the fuzzer
func (hf *Honggfuzz) Name() string {
	return "Honggfuzz"
}

// Type returns the fuzzer type
func (hf *Honggfuzz) Type() FuzzerType {
	return FuzzerTypeHonggfuzz
}

// Version returns the Honggfuzz version
func (hf *Honggfuzz) Version() string {
	cmd := exec.Command("honggfuzz", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "unknown"
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "version") || strings.Contains(line, "honggfuzz") {
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

// GetCapabilities returns Honggfuzz capabilities
func (hf *Honggfuzz) GetCapabilities() []string {
	return []string{
		"hardware_feedback",
		"persistent_mode",
		"sanitizers",
		"coverage_guided",
		"minimize_corpus",
		"unique_crashes",
		"custom_mutators",
		"network_fuzzing",
		"multiprocess",
		"timeout_detection",
	}
}

// Configure sets up the fuzzer configuration
func (hf *Honggfuzz) Configure(config FuzzConfig) error {
	hf.mu.Lock()
	defer hf.mu.Unlock()

	if hf.status != StatusUninitialized && hf.status != StatusStopped {
		return &FuzzerError{
			Type:    ErrInvalidConfig,
			Message: "cannot configure fuzzer while running",
			Fuzzer:  hf.Name(),
			Code:    1,
		}
	}

	// Validate configuration
	if err := hf.validateConfig(config); err != nil {
		return err
	}

	hf.config = config

	// Set up directories
	hf.outputDir = filepath.Join(config.OutputDirectory, "honggfuzz_output")
	hf.crashDir = filepath.Join(hf.outputDir, "crashes")
	hf.corpusDir = filepath.Join(hf.outputDir, "corpus")
	hf.statsFile = filepath.Join(hf.outputDir, "honggfuzz.stats")

	hf.status = StatusInitialized

	hf.logger.WithFields(logrus.Fields{
		"target":     config.Target,
		"output_dir": hf.outputDir,
		"duration":   config.Duration,
	}).Info("Honggfuzz configured")

	return nil
}

// Initialize prepares Honggfuzz for execution
func (hf *Honggfuzz) Initialize() error {
	hf.mu.Lock()
	defer hf.mu.Unlock()

	if hf.status != StatusInitialized {
		return &FuzzerError{
			Type:    ErrInternal,
			Message: "fuzzer must be configured before initialization",
			Fuzzer:  hf.Name(),
			Code:    2,
		}
	}

	// Create output directories
	if err := os.MkdirAll(hf.outputDir, 0755); err != nil {
		return &FuzzerError{
			Type:    ErrPermissionDenied,
			Message: fmt.Sprintf("failed to create output directory: %v", err),
			Fuzzer:  hf.Name(),
			Code:    3,
		}
	}

	if err := os.MkdirAll(hf.crashDir, 0755); err != nil {
		return &FuzzerError{
			Type:    ErrPermissionDenied,
			Message: fmt.Sprintf("failed to create crash directory: %v", err),
			Fuzzer:  hf.Name(),
			Code:    3,
		}
	}

	if err := os.MkdirAll(hf.corpusDir, 0755); err != nil {
		return &FuzzerError{
			Type:    ErrPermissionDenied,
			Message: fmt.Sprintf("failed to create corpus directory: %v", err),
			Fuzzer:  hf.Name(),
			Code:    3,
		}
	}

	// Check for Honggfuzz installation
	if _, err := exec.LookPath("honggfuzz"); err != nil {
		return &FuzzerError{
			Type:    ErrTargetNotFound,
			Message: "honggfuzz not found in PATH",
			Fuzzer:  hf.Name(),
			Code:    4,
		}
	}

	hf.logger.Info("Honggfuzz initialized")

	return nil
}

// Validate checks if the fuzzer is properly configured
func (hf *Honggfuzz) Validate() error {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	// Check target binary exists
	if _, err := os.Stat(hf.config.Target); err != nil {
		return &FuzzerError{
			Type:    ErrTargetNotFound,
			Message: fmt.Sprintf("target binary not found: %s", hf.config.Target),
			Fuzzer:  hf.Name(),
			Code:    5,
		}
	}

	// Check seed directory if specified
	if hf.config.SeedDirectory != "" {
		if _, err := os.Stat(hf.config.SeedDirectory); err != nil {
			return &FuzzerError{
				Type:    ErrInvalidConfig,
				Message: fmt.Sprintf("seed directory not found: %s", hf.config.SeedDirectory),
				Fuzzer:  hf.Name(),
				Code:    6,
			}
		}
	}

	// Check dictionary if specified
	if hf.config.Dictionary != "" {
		if _, err := os.Stat(hf.config.Dictionary); err != nil {
			return &FuzzerError{
				Type:    ErrInvalidConfig,
				Message: fmt.Sprintf("dictionary file not found: %s", hf.config.Dictionary),
				Fuzzer:  hf.Name(),
				Code:    7,
			}
		}
	}

	return nil
}

// Start begins the fuzzing process
func (hf *Honggfuzz) Start(ctx context.Context) error {
	hf.mu.Lock()
	defer hf.mu.Unlock()

	if hf.status == StatusRunning || hf.status == StatusStarting {
		return &FuzzerError{
			Type:    ErrInternal,
			Message: "fuzzer is already running",
			Fuzzer:  hf.Name(),
			Code:    8,
		}
	}

	if hf.status != StatusInitialized && hf.status != StatusPaused {
		return &FuzzerError{
			Type:    ErrInternal,
			Message: "fuzzer must be initialized before starting",
			Fuzzer:  hf.Name(),
			Code:    9,
		}
	}

	hf.status = StatusStarting
	hf.ctx, hf.cancel = context.WithCancel(ctx)

	// Build Honggfuzz command
	args := hf.buildHonggfuzzArgs()
	hf.cmd = exec.CommandContext(hf.ctx, "honggfuzz", args...)

	// Set up pipes for output
	stdout, err := hf.cmd.StdoutPipe()
	if err != nil {
		hf.status = StatusError
		return &FuzzerError{
			Type:    ErrInternal,
			Message: fmt.Sprintf("failed to create stdout pipe: %v", err),
			Fuzzer:  hf.Name(),
			Code:    10,
		}
	}

	stderr, err := hf.cmd.StderrPipe()
	if err != nil {
		hf.status = StatusError
		return &FuzzerError{
			Type:    ErrInternal,
			Message: fmt.Sprintf("failed to create stderr pipe: %v", err),
			Fuzzer:  hf.Name(),
			Code:    11,
		}
	}

	// Start Honggfuzz
	if err := hf.cmd.Start(); err != nil {
		hf.status = StatusError
		return &FuzzerError{
			Type:    ErrInternal,
			Message: fmt.Sprintf("failed to start Honggfuzz: %v", err),
			Fuzzer:  hf.Name(),
			Code:    12,
		}
	}

	hf.status = StatusRunning
	hf.stats.StartTime = time.Now()

	// Start output monitoring
	hf.wg.Add(2)
	go hf.monitorOutput(stdout, "stdout")
	go hf.monitorOutput(stderr, "stderr")

	// Start stats monitoring
	hf.startStatsMonitoring()

	// Start metrics collector
	if err := hf.metricsCollector.Start(hf.ctx); err != nil {
		hf.logger.WithError(err).Warn("Failed to start metrics collector")
	}

	// Notify event handler
	if hf.eventHandler != nil {
		hf.eventHandler.OnStart(hf)
	}

	// Emit started event through base fuzzer
	hf.EmitStartedEvent(hf.ctx, hf.config.Target, map[string]interface{}{
		"fuzzer": "Honggfuzz",
		"pid":    hf.cmd.Process.Pid,
		"bot_id": hf.botID,
	})

	// Monitor process completion
	hf.wg.Add(1)
	go hf.monitorProcess()

	hf.logger.WithField("pid", hf.cmd.Process.Pid).Info("Honggfuzz started")

	return nil
}

// Stop gracefully stops the fuzzing process
func (hf *Honggfuzz) Stop() error {
	hf.mu.Lock()
	defer hf.mu.Unlock()

	if hf.status != StatusRunning && hf.status != StatusPaused {
		return nil
	}

	hf.status = StatusStopping

	// Cancel context to stop monitoring
	if hf.cancel != nil {
		hf.cancel()
	}

	// Stop stats monitoring
	if hf.monitorTicker != nil {
		hf.monitorTicker.Stop()
	}

	// Stop metrics collector
	if hf.metricsCollector != nil {
		if err := hf.metricsCollector.Stop(); err != nil {
			hf.logger.WithError(err).Warn("Failed to stop metrics collector")
		}
	}

	// Send SIGTERM to Honggfuzz
	if hf.cmd != nil && hf.cmd.Process != nil {
		if err := hf.cmd.Process.Signal(os.Interrupt); err != nil {
			hf.logger.WithError(err).Warn("Failed to send interrupt signal")
		}

		// Give it a moment to exit gracefully
		time.Sleep(100 * time.Millisecond)

		// Force kill if still running
		if hf.cmd.Process != nil {
			if err := hf.cmd.Process.Kill(); err != nil {
				hf.logger.WithError(err).Debug("Process may have already exited")
			}
		}
	}

	// Update status immediately so IsRunning() returns false
	hf.status = StatusStopped

	// Wait for goroutines to finish (with timeout to avoid hanging)
	done := make(chan struct{})
	go func() {
		hf.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Goroutines finished normally
	case <-time.After(5 * time.Second):
		// Timeout waiting for goroutines
		hf.logger.Warn("Timeout waiting for fuzzer goroutines to finish")
	}

	// Notify event handler
	if hf.eventHandler != nil {
		hf.eventHandler.OnStop(hf, "user requested")
	}

	// Emit stopped event through base fuzzer
	hf.EmitStoppedEvent(hf.ctx, hf.config.Target, "user requested")

	hf.logger.Info("Honggfuzz stopped")

	return nil
}

// Pause pauses the fuzzing process
func (hf *Honggfuzz) Pause() error {
	hf.mu.Lock()
	defer hf.mu.Unlock()

	if hf.status != StatusRunning {
		return &FuzzerError{
			Type:    ErrInternal,
			Message: "fuzzer is not running",
			Fuzzer:  hf.Name(),
			Code:    13,
		}
	}

	// Send SIGSTOP to pause
	if hf.cmd != nil && hf.cmd.Process != nil {
		if err := hf.cmd.Process.Signal(syscall.SIGSTOP); err != nil {
			return &FuzzerError{
				Type:    ErrInternal,
				Message: fmt.Sprintf("failed to pause Honggfuzz: %v", err),
				Fuzzer:  hf.Name(),
				Code:    14,
			}
		}
	}

	hf.status = StatusPaused
	hf.logger.Info("Honggfuzz paused")

	return nil
}

// Resume resumes the fuzzing process
func (hf *Honggfuzz) Resume() error {
	hf.mu.Lock()
	defer hf.mu.Unlock()

	if hf.status != StatusPaused {
		return &FuzzerError{
			Type:    ErrInternal,
			Message: "fuzzer is not paused",
			Fuzzer:  hf.Name(),
			Code:    15,
		}
	}

	// Send SIGCONT to resume
	if hf.cmd != nil && hf.cmd.Process != nil {
		if err := hf.cmd.Process.Signal(syscall.SIGCONT); err != nil {
			return &FuzzerError{
				Type:    ErrInternal,
				Message: fmt.Sprintf("failed to resume Honggfuzz: %v", err),
				Fuzzer:  hf.Name(),
				Code:    16,
			}
		}
	}

	hf.status = StatusRunning
	hf.logger.Info("Honggfuzz resumed")

	return nil
}

// GetStatus returns the current fuzzer status
func (hf *Honggfuzz) GetStatus() FuzzerStatus {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	return hf.status
}

// SetBotID sets the bot ID for crash reporting
func (hf *Honggfuzz) SetBotID(botID string) {
	hf.mu.Lock()
	defer hf.mu.Unlock()
	hf.botID = botID
}

// GetStats returns current fuzzing statistics
func (hf *Honggfuzz) GetStats() FuzzerStats {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	stats := hf.stats
	stats.ElapsedTime = time.Since(stats.StartTime)

	return stats
}

// GetProgress returns fuzzing progress information
func (hf *Honggfuzz) GetProgress() FuzzerProgress {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	progress := FuzzerProgress{
		Phase:           hf.getPhase(),
		ProgressPercent: hf.calculateProgress(),
		CurrentInput:    hf.getCurrentInput(),
		QueuePosition:   int(hf.stats.CorpusSize),
		QueueSize:       int(hf.stats.PathsTotal),
		LastUpdate:      time.Now(),
	}

	if hf.config.Duration > 0 {
		elapsed := time.Since(hf.stats.StartTime)
		remaining := hf.config.Duration - elapsed
		if remaining > 0 {
			progress.ETA = remaining
		}
	}

	return progress
}

// IsRunning returns whether the fuzzer is currently running
func (hf *Honggfuzz) IsRunning() bool {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	return hf.status == StatusRunning
}

// GetResults retrieves all fuzzing results
func (hf *Honggfuzz) GetResults() (*FuzzerResults, error) {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	crashes, err := hf.GetCrashes()
	if err != nil {
		return nil, err
	}

	coverage, err := hf.GetCoverage()
	if err != nil {
		return nil, err
	}

	corpus, err := hf.GetCorpus()
	if err != nil {
		return nil, err
	}

	results := &FuzzerResults{
		Summary: ResultSummary{
			TotalExecutions:  hf.stats.Executions,
			ExecutionTime:    time.Since(hf.stats.StartTime),
			UniqueCrashes:    hf.stats.UniqueCrashes,
			CoverageAchieved: hf.stats.CoveragePercent,
			NewInputsFound:   hf.stats.NewPaths,
			Success:          hf.stats.UniqueCrashes > 0 || hf.stats.NewPaths > 0,
			ExitReason:       hf.getExitReason(),
		},
		Crashes:  crashes,
		Coverage: coverage,
		Corpus:   corpus,
		Performance: PerformanceMetrics{
			AverageExecSpeed: hf.stats.ExecPerSecond,
			PeakExecSpeed:    hf.getPeakExecSpeed(),
			AverageCPU:       hf.stats.CPUUsage,
			PeakMemory:       hf.stats.MemoryUsage,
			StartupTime:      1 * time.Second, // Honggfuzz typically starts quickly
		},
	}

	return results, nil
}

// GetCrashes retrieves crash information with InputBase64 field populated
func (hf *Honggfuzz) GetCrashes() ([]*common.CrashResult, error) {
	crashes := make([]*common.CrashResult, 0)

	hf.logger.WithFields(logrus.Fields{
		"crash_dir": hf.crashDir,
		"job_id":    hf.config.Target,
	}).Info("Scanning Honggfuzz crash directory for new crashes")

	// Read crashes from Honggfuzz crash directory
	if _, err := os.Stat(hf.crashDir); err == nil {
		files, err := os.ReadDir(hf.crashDir)
		if err != nil {
			return nil, err
		}

		hf.logger.WithFields(logrus.Fields{
			"crash_dir":  hf.crashDir,
			"file_count": len(files),
		}).Debug("Found files in Honggfuzz crash directory")

		for _, file := range files {
			if file.IsDir() || strings.HasPrefix(file.Name(), ".") {
				continue
			}

			crashPath := filepath.Join(hf.crashDir, file.Name())

			hf.logger.WithFields(logrus.Fields{
				"crash_file": file.Name(),
				"crash_path": crashPath,
			}).Info("Found Honggfuzz crash file")

			crashData, err := os.ReadFile(crashPath)
			if err != nil {
				hf.logger.WithError(err).WithField("file", file.Name()).Warn("Failed to read crash file")
				continue
			}

			info, err := file.Info()
			if err != nil {
				continue
			}

			crashType := hf.detectCrashType(file.Name())
			crashHash := hf.hashInput(crashData)

			crash := &common.CrashResult{
				ID:          file.Name(),
				JobID:       hf.config.Target,
				BotID:       hf.botID,
				Timestamp:   info.ModTime(),
				FilePath:    filepath.Join(hf.crashDir, file.Name()),
				Size:        int64(len(crashData)),
				Hash:        crashHash,
				Type:        crashType,
				Input:       crashData,                                    // Include the crash input data
				InputBase64: base64.StdEncoding.EncodeToString(crashData), // Base64 encode the crash data
			}

			hf.logger.WithFields(logrus.Fields{
				"crash_id":   crash.ID,
				"crash_type": crashType,
				"crash_hash": crashHash,
				"crash_size": crash.Size,
				"job_id":     crash.JobID,
				"bot_id":     crash.BotID,
				"file_name":  file.Name(),
			}).Info("Detected Honggfuzz crash")

			crashes = append(crashes, crash)
		}

		hf.logger.WithFields(logrus.Fields{
			"crash_count": len(crashes),
			"job_id":      hf.config.Target,
		}).Info("Completed Honggfuzz crash scan")
	} else {
		hf.logger.WithFields(logrus.Fields{
			"crash_dir": hf.crashDir,
			"error":     err,
		}).Debug("Honggfuzz crash directory does not exist yet")
	}

	return crashes, nil
}

// GetCoverage retrieves coverage information
func (hf *Honggfuzz) GetCoverage() (*common.CoverageResult, error) {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	coverage := &common.CoverageResult{
		ID:        fmt.Sprintf("honggfuzz_%d", time.Now().Unix()),
		JobID:     hf.config.Target,
		BotID:     hf.botID,
		Timestamp: time.Now(),
		Edges:     int(hf.stats.TotalEdges),
		NewEdges:  int(hf.stats.NewPaths),
	}

	return coverage, nil
}

// GetCorpus retrieves corpus entries
func (hf *Honggfuzz) GetCorpus() ([]*CorpusEntry, error) {
	corpus := make([]*CorpusEntry, 0)

	// Read corpus from Honggfuzz corpus directory
	if _, err := os.Stat(hf.corpusDir); err == nil {
		files, err := os.ReadDir(hf.corpusDir)
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
				Source:    "honggfuzz_corpus",
			}

			corpus = append(corpus, entry)
		}
	}

	return corpus, nil
}

// SetEventHandler sets the event handler for fuzzer events
func (hf *Honggfuzz) SetEventHandler(handler EventHandler) {
	hf.mu.Lock()
	defer hf.mu.Unlock()

	hf.eventHandler = handler
}

// Cleanup cleans up fuzzer resources
func (hf *Honggfuzz) Cleanup() error {
	// Stop if running
	if hf.IsRunning() {
		if err := hf.Stop(); err != nil {
			return err
		}
	}

	// Remove temporary files if configured
	if cleanTemp, ok := hf.config.FuzzerOptions["clean_temp"].(bool); ok && cleanTemp {
		if err := os.RemoveAll(hf.outputDir); err != nil {
			hf.logger.WithError(err).Warn("Failed to clean temporary files")
		}
	}

	return nil
}

// Private helper methods

func (hf *Honggfuzz) validateConfig(config FuzzConfig) error {
	if config.Target == "" {
		return &FuzzerError{
			Type:    ErrInvalidConfig,
			Message: "target binary is required",
			Fuzzer:  hf.Name(),
			Code:    17,
		}
	}

	if config.OutputDirectory == "" {
		return &FuzzerError{
			Type:    ErrInvalidConfig,
			Message: "output directory is required",
			Fuzzer:  hf.Name(),
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

func (hf *Honggfuzz) buildHonggfuzzArgs() []string {
	args := []string{}

	// Input directory
	if hf.config.SeedDirectory != "" {
		args = append(args, "--input", hf.config.SeedDirectory)
	}

	// Output/crash directory
	args = append(args, "--output", hf.crashDir)

	// Workspace directory (for corpus)
	args = append(args, "--workspace", hf.corpusDir)

	// Number of threads (default to CPU count)
	if threads, ok := hf.config.FuzzerOptions["threads"].(int); ok {
		args = append(args, "--threads", strconv.Itoa(threads))
	}

	// Memory limit
	args = append(args, "--rlimit_rss", fmt.Sprintf("%d", hf.config.MemoryLimit))

	// Timeout
	timeoutSec := int(hf.config.Timeout.Seconds())
	if timeoutSec < 1 {
		timeoutSec = 1
	}
	args = append(args, "--timeout", strconv.Itoa(timeoutSec))

	// Dictionary
	if hf.config.Dictionary != "" {
		args = append(args, "--dict", hf.config.Dictionary)
	}

	// Enable sanitizers if available
	if sanitizers, ok := hf.config.FuzzerOptions["sanitizers"].(bool); ok && sanitizers {
		args = append(args, "--sanitizers")
	}

	// Save all crashes
	args = append(args, "--save_all")

	// Verbose output for better monitoring
	args = append(args, "--verbose")

	// Stats file
	args = append(args, "--logfile", hf.statsFile)

	// Honggfuzz specific options
	if options, ok := hf.config.FuzzerOptions["honggfuzz_args"].([]string); ok {
		args = append(args, options...)
	}

	// Target binary and arguments
	args = append(args, "--")
	args = append(args, hf.config.Target)

	// Add ___FILE___ placeholder for Honggfuzz
	targetArgs := make([]string, len(hf.config.TargetArgs))
	copy(targetArgs, hf.config.TargetArgs)

	// If no ___FILE___ placeholder found, append it
	hasPlaceholder := false
	for _, arg := range targetArgs {
		if strings.Contains(arg, "___FILE___") {
			hasPlaceholder = true
			break
		}
	}
	if !hasPlaceholder {
		targetArgs = append(targetArgs, "___FILE___")
	}

	args = append(args, targetArgs...)

	return args
}

func (hf *Honggfuzz) monitorOutput(pipe io.Reader, name string) {
	defer hf.wg.Done()

	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		hf.logger.WithField("stream", name).Debug(line)

		// Parse Honggfuzz output for important information
		if strings.Contains(line, "Crash") || strings.Contains(line, "crash") {
			hf.detectAndEmitCrash(line)
		}

		// Parse stats from output
		hf.parseOutputStats(line)
	}
}

func (hf *Honggfuzz) parseOutputStats(line string) {
	// Honggfuzz output format parsing
	// Example: "Fuzzing : 12345/0 [3%], Crashes : 5 (unique: 2), Speed : 1234/sec"

	if strings.Contains(line, "Fuzzing") {
		hf.mu.Lock()
		defer hf.mu.Unlock()

		// Extract executions
		if match := strings.Split(line, "Fuzzing : "); len(match) > 1 {
			if parts := strings.Split(match[1], "/"); len(parts) > 0 {
				if val, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64); err == nil {
					hf.stats.Executions = val
				}
			}
		}

		// Extract crashes
		if match := strings.Split(line, "Crashes : "); len(match) > 1 {
			if parts := strings.Split(match[1], " "); len(parts) > 0 {
				if val, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
					hf.stats.TotalCrashes = val
				}
			}

			// Extract unique crashes
			if strings.Contains(match[1], "unique:") {
				if uniqueMatch := strings.Split(match[1], "unique: "); len(uniqueMatch) > 1 {
					if parts := strings.Split(uniqueMatch[1], ")"); len(parts) > 0 {
						if val, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
							hf.stats.UniqueCrashes = val
						}
					}
				}
			}
		}

		// Extract speed
		if match := strings.Split(line, "Speed : "); len(match) > 1 {
			if parts := strings.Split(match[1], "/"); len(parts) > 0 {
				if val, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64); err == nil {
					hf.stats.ExecPerSecond = val
					// Track peak execution speed
					if val > hf.peakExecSpeed {
						hf.peakExecSpeed = val
					}
					// Add to history
					hf.execHistory = append(hf.execHistory, val)
					if len(hf.execHistory) > 100 {
						hf.execHistory = hf.execHistory[1:]
					}
				}
			}
		}

		// Update elapsed time
		hf.stats.ElapsedTime = time.Since(hf.stats.StartTime)

		// Collect enhanced metrics
		hf.collectEnhancedMetrics(line)

		// Record execution for performance tracking
		if hf.lastExecCount > 0 {
			execDiff := hf.stats.Executions - hf.lastExecCount
			timeDiff := time.Since(hf.lastStatsUpdate)
			if timeDiff > 0 && execDiff > 0 {
				avgExecTime := timeDiff / time.Duration(execDiff)
				hf.metricsCollector.RecordExecution(avgExecTime)
			}
		}
		hf.lastExecCount = hf.stats.Executions
		hf.lastStatsUpdate = time.Now()
	}
}

func (hf *Honggfuzz) monitorProcess() {
	defer hf.wg.Done()

	// Wait for process to exit
	err := hf.cmd.Wait()

	hf.mu.Lock()
	defer hf.mu.Unlock()

	if err != nil {
		hf.logger.WithError(err).Warn("Honggfuzz process exited with error")
		hf.status = StatusError
		if hf.eventHandler != nil {
			hf.eventHandler.OnError(hf, err)
		}
		// Emit error event through base fuzzer
		hf.EmitErrorEvent(hf.ctx, hf.config.Target, err)
	} else {
		hf.status = StatusCompleted
	}

	// Notify completion
	if hf.eventHandler != nil {
		reason := "completed"
		if hf.ctx.Err() != nil {
			reason = "cancelled"
		}
		hf.eventHandler.OnStop(hf, reason)
	}

	// Emit stopped event through base fuzzer
	reason := "completed"
	if hf.ctx.Err() != nil {
		reason = "cancelled"
	}
	hf.EmitStoppedEvent(context.Background(), hf.config.Target, reason)
}

func (hf *Honggfuzz) startStatsMonitoring() {
	interval := 5 * time.Second
	if statsInterval, ok := hf.config.FuzzerOptions["stats_interval"].(time.Duration); ok {
		interval = statsInterval
	}

	hf.monitorTicker = time.NewTicker(interval)

	hf.wg.Add(1)
	go func() {
		defer hf.wg.Done()

		for {
			select {
			case <-hf.ctx.Done():
				return
			case <-hf.monitorTicker.C:
				hf.updateStats()
			}
		}
	}()
}

func (hf *Honggfuzz) updateStats() {
	// Read Honggfuzz stats file if available
	if data, err := os.ReadFile(hf.statsFile); err == nil {
		// Parse stats from log file
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			hf.parseOutputStats(line)
		}
	}

	hf.mu.Lock()
	defer hf.mu.Unlock()

	// Update derived stats
	hf.stats.ElapsedTime = time.Since(hf.stats.StartTime)
	if hf.stats.Executions > 0 && hf.stats.ElapsedTime.Seconds() > 0 {
		hf.stats.ExecPerSecond = float64(hf.stats.Executions) / hf.stats.ElapsedTime.Seconds()
	}

	// Get system resource usage
	if hf.cmd != nil && hf.cmd.Process != nil {
		// This would require platform-specific code to get actual CPU/memory usage
		// For now, using placeholder values
		hf.stats.CPUUsage = 50.0
		hf.stats.MemoryUsage = hf.config.MemoryLimit * 1024 * 1024
	}

	// Notify event handler
	if hf.eventHandler != nil {
		hf.eventHandler.OnStats(hf, hf.stats)
		hf.eventHandler.OnProgress(hf, hf.GetProgress())
	}

	// Emit stats event through base fuzzer
	hf.parseAndEmitStats(map[string]string{
		"execs_done":     strconv.FormatInt(hf.stats.Executions, 10),
		"execs_per_sec":  strconv.FormatFloat(hf.stats.ExecPerSecond, 'f', 2, 64),
		"paths_total":    strconv.Itoa(hf.stats.PathsTotal),
		"unique_crashes": strconv.Itoa(hf.stats.UniqueCrashes),
		"total_crashes":  strconv.Itoa(hf.stats.TotalCrashes),
	})

	// Check for new crashes
	hf.checkForNewCrashes()
}

func (hf *Honggfuzz) checkForNewCrashes() {
	crashes, err := hf.GetCrashes()
	if err != nil {
		return
	}

	// Simple check: if crash count increased, notify about latest crash
	if len(crashes) > hf.stats.TotalCrashes {
		hf.stats.TotalCrashes = len(crashes)
		hf.stats.LastCrash = time.Now()

		if hf.eventHandler != nil && len(crashes) > 0 {
			// Notify about the latest crash
			hf.eventHandler.OnCrash(hf, crashes[len(crashes)-1])
		}

		// Emit crash event through base fuzzer
		if len(crashes) > 0 {
			hf.detectAndEmitCrash(crashes[len(crashes)-1])
		}
	}
}

func (hf *Honggfuzz) getPhase() string {
	if hf.stats.Executions < 10000 {
		return "initialization"
	} else if hf.stats.Executions < 100000 {
		return "exploration"
	} else {
		return "exploitation"
	}
}

func (hf *Honggfuzz) calculateProgress() float64 {
	if hf.config.Duration > 0 {
		elapsed := time.Since(hf.stats.StartTime)
		progress := elapsed.Seconds() / hf.config.Duration.Seconds() * 100
		if progress > 100 {
			progress = 100
		}
		return progress
	}

	// If no duration set, use execution count
	if hf.config.MaxExecutions > 0 {
		progress := float64(hf.stats.Executions) / float64(hf.config.MaxExecutions) * 100
		if progress > 100 {
			progress = 100
		}
		return progress
	}

	return 0
}

func (hf *Honggfuzz) getCurrentInput() string {
	// Honggfuzz doesn't easily expose current input being tested
	return "unknown"
}

func (hf *Honggfuzz) getPeakExecSpeed() float64 {
	// Would need to track this over time
	// For now, return current speed as peak
	return hf.stats.ExecPerSecond
}

func (hf *Honggfuzz) getExitReason() string {
	switch hf.status {
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

func (hf *Honggfuzz) hashInput(data []byte) string {
	// Simple hash implementation
	// In production, use proper crypto hash
	hash := uint32(0)
	for _, b := range data {
		hash = hash*31 + uint32(b)
	}
	return fmt.Sprintf("%08x", hash)
}

func (hf *Honggfuzz) detectCrashType(filename string) string {
	// Honggfuzz includes crash type in filename
	if strings.Contains(filename, "SIGSEGV") || strings.Contains(filename, "sig11") {
		return "segmentation_fault"
	} else if strings.Contains(filename, "SIGABRT") || strings.Contains(filename, "sig6") {
		return "abort"
	} else if strings.Contains(filename, "SIGFPE") || strings.Contains(filename, "sig8") {
		return "arithmetic_exception"
	} else if strings.Contains(filename, "SIGILL") {
		return "illegal_instruction"
	} else if strings.Contains(filename, "SIGBUS") {
		return "bus_error"
	} else if strings.Contains(filename, "timeout") || strings.Contains(filename, "TIMEOUT") {
		return "timeout"
	}

	return "unknown"
}

// parseAndEmitStats parses Honggfuzz stats and emits stats event
func (hf *Honggfuzz) parseAndEmitStats(stats map[string]string) {
	// Emit stats event through base fuzzer
	hf.EmitStatsEvent(hf.ctx, hf.config.Target, hf.stats)
}

// detectAndEmitCrash detects crashes and emits crash event
func (hf *Honggfuzz) detectAndEmitCrash(crashOrLine interface{}) {
	switch v := crashOrLine.(type) {
	case *common.CrashResult:
		// Emit through event handler - this includes the full crash data
		if hf.eventHandler != nil {
			hf.eventHandler.OnCrash(hf, v)
		}
	case string:
		// Parse crash info from output line
		hf.logger.WithField("line", v).Debug("Detected crash in output")
		// Could parse more details from the line if needed
	}
}

// ReproduceCrash attempts to reproduce a crash with the given input
func (hf *Honggfuzz) ReproduceCrash(ctx context.Context, crashInput []byte, config ReproductionConfig) (*common.ReproductionResult, error) {
	hf.logger.WithFields(logrus.Fields{
		"crash_size": len(crashInput),
		"attempts":   config.Attempts,
		"timeout":    config.Timeout,
	}).Info("Starting crash reproduction with Honggfuzz")

	// Default attempts if not specified
	if config.Attempts <= 0 {
		config.Attempts = 3
	}

	// Default timeout if not specified
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}

	// Create temporary directory for reproduction
	tempDir, err := os.MkdirTemp("", "honggfuzz_reproduce_*")
	if err != nil {
		return nil, &FuzzerError{
			Type:    ErrInternal,
			Message: fmt.Sprintf("failed to create temp directory: %v", err),
			Fuzzer:  hf.Name(),
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
			Fuzzer:  hf.Name(),
			Code:    101,
		}
	}

	// Try to reproduce the crash multiple times
	for attempt := 1; attempt <= config.Attempts; attempt++ {
		hf.logger.WithField("attempt", attempt).Debug("Attempting crash reproduction")

		// Create a context with timeout for this attempt
		attemptCtx, cancel := context.WithTimeout(ctx, config.Timeout)
		defer cancel()

		// Build command to run target directly with crash input
		// Honggfuzz doesn't have a specific reproduction mode, so we run the target directly
		var args []string

		// Add target arguments, replacing ___FILE___ with the crash file path
		for _, arg := range hf.config.TargetArgs {
			if arg == "___FILE___" {
				args = append(args, crashFile)
			} else {
				args = append(args, arg)
			}
		}

		// If no ___FILE___ was found, assume input is via stdin or append the file
		var cmd *exec.Cmd
		if containsHonggfuzzPlaceholder(hf.config.TargetArgs) {
			cmd = exec.CommandContext(attemptCtx, hf.config.Target, args...)
		} else {
			// Input via stdin
			cmd = exec.CommandContext(attemptCtx, hf.config.Target, args...)
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
		stackTrace := extractHonggfuzzStackTrace(string(output))
		stackHash := hf.hashInput([]byte(stackTrace))

		// Build environment info
		envInfo := make(map[string]string)
		envInfo["fuzzer"] = "Honggfuzz"
		envInfo["version"] = hf.Version()
		envInfo["bot_id"] = hf.botID
		envInfo["attempt"] = strconv.Itoa(attempt)

		result := &common.ReproductionResult{
			ID:              fmt.Sprintf("honggfuzz_repro_%d_%d", time.Now().Unix(), attempt),
			RequestID:       config.OriginalCrashID,
			CrashID:         config.OriginalCrashID,
			BotID:           hf.botID,
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
			hf.logger.WithFields(logrus.Fields{
				"attempt":    attempt,
				"signal":     signal,
				"exit_code":  exitCode,
				"stack_hash": stackHash,
			}).Info("Successfully reproduced crash with Honggfuzz")
			return result, nil
		}

		hf.logger.WithField("attempt", attempt).Debug("Crash not reproduced in this attempt")

		// If this was the last attempt and we didn't reproduce
		if attempt == config.Attempts {
			result.Status = common.ReproducibilityStatusFailed
			return result, nil
		}
	}

	// Should not reach here, but just in case
	return &common.ReproductionResult{
		ID:              fmt.Sprintf("honggfuzz_repro_%d_failed", time.Now().Unix()),
		RequestID:       config.OriginalCrashID,
		CrashID:         config.OriginalCrashID,
		BotID:           hf.botID,
		Status:          common.ReproducibilityStatusFailed,
		Reproduced:      false,
		EnvironmentInfo: map[string]string{"fuzzer": "Honggfuzz"},
		Timestamp:       time.Now(),
	}, nil
}

// Helper function to check if slice contains Honggfuzz placeholder
func containsHonggfuzzPlaceholder(slice []string) bool {
	for _, s := range slice {
		if s == "___FILE___" {
			return true
		}
	}
	return false
}

// Helper function to extract stack trace from output
func extractHonggfuzzStackTrace(output string) string {
	// Look for common stack trace indicators
	lines := strings.Split(output, "\n")
	var stackLines []string
	inStack := false

	for _, line := range lines {
		// Common stack trace patterns for Honggfuzz output
		if strings.Contains(line, "Stack trace:") ||
			strings.Contains(line, "backtrace:") ||
			strings.Contains(line, "#0 ") ||
			strings.Contains(line, "at 0x") ||
			strings.Contains(line, "frame #") {
			inStack = true
		}

		if inStack {
			stackLines = append(stackLines, line)
			// Stop at empty line or common end markers
			if line == "" || strings.Contains(line, "===") || strings.Contains(line, "---") {
				break
			}
		}
	}

	return strings.Join(stackLines, "\n")
}

// collectEnhancedMetrics collects and updates enhanced metrics
func (hf *Honggfuzz) collectEnhancedMetrics(line string) {
	// Coverage metrics
	coverage := common.CoverageMetrics{
		NewEdgesFound:    int64(hf.stats.NewPaths),
		CoverageByModule: make(map[string]float64),
	}

	// Extract coverage percentage from output
	if strings.Contains(line, "[") && strings.Contains(line, "%]") {
		if match := strings.Split(line, "["); len(match) > 1 {
			if parts := strings.Split(match[1], "%]"); len(parts) > 0 {
				if val, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64); err == nil {
					hf.stats.CoveragePercent = val
					coverage.LineCoverage = val
					coverage.BranchCoverage = val
				}
			}
		}
	}

	// Calculate coverage growth rate
	if hf.stats.ElapsedTime.Seconds() > 0 {
		coverage.CoverageGrowth = float64(hf.stats.NewPaths) / hf.stats.ElapsedTime.Seconds()
	}

	// Performance metrics
	performance := common.PerformanceMetrics{
		ExecutionsPerSecond: hf.stats.ExecPerSecond,
		ThroughputMBps:      hf.calculateThroughput(),
		InputGenerationRate: hf.stats.ExecPerSecond,
	}

	// Update metrics collector
	hf.metricsCollector.UpdateCoverageMetrics(coverage)
	hf.metricsCollector.UpdatePerformanceMetrics(performance)

	// Extract Honggfuzz specific metrics from output
	hf.extractFuzzerSpecificMetrics(line)
}

// extractFuzzerSpecificMetrics extracts Honggfuzz-specific metrics from output
func (hf *Honggfuzz) extractFuzzerSpecificMetrics(line string) {
	// Extract various Honggfuzz metrics
	if strings.Contains(line, "hwfeedback") {
		hf.metricsCollector.UpdateFuzzerSpecificMetrics("hardware_feedback", line)
	}
	if strings.Contains(line, "sanitizer") {
		hf.metricsCollector.UpdateFuzzerSpecificMetrics("sanitizer_feedback", line)
	}
	if strings.Contains(line, "ptrace") {
		hf.metricsCollector.UpdateFuzzerSpecificMetrics("ptrace_feedback", line)
	}
	if strings.Contains(line, "coverage") {
		hf.metricsCollector.UpdateFuzzerSpecificMetrics("coverage_type", line)
	}

	// Extract thread count if present
	if strings.Contains(line, "threads:") {
		if match := strings.Split(line, "threads:"); len(match) > 1 {
			parts := strings.Fields(match[1])
			if len(parts) > 0 {
				hf.metricsCollector.UpdateFuzzerSpecificMetrics("threads", parts[0])
			}
		}
	}

	// Extract timeout count
	if strings.Contains(line, "timeouts:") {
		if match := strings.Split(line, "timeouts:"); len(match) > 1 {
			parts := strings.Fields(match[1])
			if len(parts) > 0 {
				hf.metricsCollector.UpdateFuzzerSpecificMetrics("timeouts", parts[0])
			}
		}
	}
}

// calculateThroughput calculates throughput in MB/s
func (hf *Honggfuzz) calculateThroughput() float64 {
	if hf.stats.ElapsedTime.Seconds() == 0 {
		return 0
	}

	// Estimate average input size (would need actual tracking)
	avgInputSize := 1024.0 // 1KB average estimate
	totalBytes := float64(hf.stats.Executions) * avgInputSize
	totalMB := totalBytes / (1024 * 1024)

	return totalMB / hf.stats.ElapsedTime.Seconds()
}

// GetEnhancedMetrics returns enhanced metrics from the fuzzer
func (hf *Honggfuzz) GetEnhancedMetrics() *common.EnhancedMetrics {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	if hf.metricsCollector != nil {
		return hf.metricsCollector.GetMetrics()
	}

	return nil
}

// CreateHonggfuzz creates a new Honggfuzz instance with optional logger
func CreateHonggfuzz(logger *logrus.Logger) (Fuzzer, error) {
	return NewHonggfuzz(logger), nil
}
