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
		logger.SetLevel(logrus.DebugLevel) // Ensure debug level for Honggfuzz logs
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

	// Note: Persistent mode detection removed as it should be configured via HongFuzzConfig

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

	// Copy seed files if seed directory is provided
	if hf.config.SeedDirectory != "" {
		seedFiles, err := os.ReadDir(hf.config.SeedDirectory)
		if err == nil {
			for _, file := range seedFiles {
				if !file.IsDir() {
					srcPath := filepath.Join(hf.config.SeedDirectory, file.Name())
					dstPath := filepath.Join(hf.corpusDir, file.Name())
					if data, err := os.ReadFile(srcPath); err == nil {
						if err := os.WriteFile(dstPath, data, 0644); err == nil {
							hf.logger.WithField("file", file.Name()).Debug("Copied seed file to corpus")
						}
					}
				}
			}
		}
	}

	// Ensure corpus directory has at least one seed file for honggfuzz
	// Honggfuzz requires an input directory with at least one file to start
	corpusFiles, err := os.ReadDir(hf.corpusDir)
	if err != nil || len(corpusFiles) == 0 {
		hf.logger.Info("Creating initial seed file for Honggfuzz corpus")
		// Create a minimal seed file
		seedFile := filepath.Join(hf.corpusDir, "seed_0")
		if err := os.WriteFile(seedFile, []byte("test"), 0644); err != nil {
			hf.logger.WithError(err).Warn("Failed to create seed file")
		} else {
			hf.logger.WithField("file", seedFile).Debug("Created initial seed file")
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

	// Check if target binary exists and is executable
	if _, err := os.Stat(hf.config.Target); err != nil {
		hf.status = StatusError
		return &FuzzerError{
			Type:    ErrTargetNotFound,
			Message: fmt.Sprintf("target binary not found or not accessible: %s - %v", hf.config.Target, err),
			Fuzzer:  hf.Name(),
			Code:    14,
		}
	}

	// Check if target is executable
	if info, err := os.Stat(hf.config.Target); err == nil {
		mode := info.Mode()
		if !mode.IsRegular() || mode.Perm()&0111 == 0 {
			hf.logger.WithFields(logrus.Fields{
				"target": hf.config.Target,
				"mode":   mode.String(),
			}).Warn("Target binary may not be executable")
		}
	}

	// Build Honggfuzz command
	args := hf.buildHonggfuzzArgs()

	// Log the full command for debugging
	hf.logger.WithFields(logrus.Fields{
		"command": "honggfuzz",
		"args":    args,
		"workdir": hf.outputDir,
	}).Info("Starting HongFuzz with command")

	// Check if honggfuzz binary exists first
	honggfuzzPath, err := exec.LookPath("honggfuzz")
	if err != nil {
		hf.status = StatusError
		return &FuzzerError{
			Type:    ErrTargetNotFound,
			Message: fmt.Sprintf("honggfuzz binary not found in PATH: %v", err),
			Fuzzer:  hf.Name(),
			Code:    13,
		}
	}

	hf.logger.WithFields(logrus.Fields{
		"binary_path": honggfuzzPath,
		"args_count":  len(args),
		"workdir":     hf.outputDir,
	}).Info("Found honggfuzz binary")

	hf.cmd = exec.CommandContext(hf.ctx, honggfuzzPath, args...)
	hf.cmd.Dir = hf.outputDir
	// Set process group to enable killing all child processes
	hf.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Set environment variables for network fuzzing if configured
	if hongFuzzConfig, ok := hf.config.FuzzerOptions["honggfuzz_config"].(*HongFuzzConfig); ok {
		if hongFuzzConfig.NetworkPort > 0 {
			env := os.Environ()
			env = append(env, fmt.Sprintf("HFND_TCP_PORT=%d", hongFuzzConfig.NetworkPort))
			hf.cmd.Env = env
		}
	} else if hongFuzzConfig, ok := hf.config.FuzzerOptions["honggfuzz_config"].(HongFuzzConfig); ok {
		if hongFuzzConfig.NetworkPort > 0 {
			env := os.Environ()
			env = append(env, fmt.Sprintf("HFND_TCP_PORT=%d", hongFuzzConfig.NetworkPort))
			hf.cmd.Env = env
		}
	}

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

	// Log the process PID for tracking
	hf.logger.WithFields(logrus.Fields{
		"pid":     hf.cmd.Process.Pid,
		"command": "honggfuzz",
		"args":    args,
		"workdir": hf.outputDir,
	}).Info("Honggfuzz process started")

	hf.status = StatusRunning
	hf.stats.StartTime = time.Now()

	// Write initial command info to OutputWriter
	if hf.config.OutputWriter != nil {
		timestamp := time.Now().Format("15:04:05")
		fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] Starting HongFuzz\n", timestamp, "info", "fuzzer")
		fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] Command: honggfuzz %s\n", timestamp, "info", "fuzzer", strings.Join(args, " "))
		fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] Working directory: %s\n", timestamp, "info", "fuzzer", hf.outputDir)
		fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] Process PID: %d\n", timestamp, "info", "fuzzer", hf.cmd.Process.Pid)
	}

	// Set environment to ensure output is not buffered
	if hf.cmd.Env == nil {
		hf.cmd.Env = os.Environ()
	}
	// Force line buffering for better output capture
	hf.cmd.Env = append(hf.cmd.Env, "LIBC_FATAL_STDERR_=1")

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

	// Give HongFuzz a moment to start and create stats file
	go func() {
		time.Sleep(2 * time.Second)
		hf.monitorStatsFile() // Initial stats read
	}()

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

	// Immediately check if process is still alive
	go func() {
		time.Sleep(500 * time.Millisecond)
		if hf.cmd.Process != nil {
			// Check if process is still running
			err := hf.cmd.Process.Signal(syscall.Signal(0))
			if err != nil {
				hf.logger.WithError(err).Error("Honggfuzz process died immediately after starting")
				// Try to capture any early exit output
				if hf.config.OutputWriter != nil {
					timestamp := time.Now().Format("15:04:05")
					fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] Honggfuzz process died immediately: %v\n", timestamp, "error", "fuzzer", err)
				}
			} else {
				hf.logger.WithField("pid", hf.cmd.Process.Pid).Debug("Honggfuzz process is still running after 500ms")
			}
		}
	}()

	hf.logger.WithField("pid", hf.cmd.Process.Pid).Info("Honggfuzz monitoring started")

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
		"corpus_dir": hf.corpusDir,
		"crash_dir":  hf.crashDir,
		"job_id":     hf.config.Target,
		"status":     hf.status,
		"bot_id":     hf.botID,
	}).Info("Scanning Honggfuzz directories for crashes")

	// HongFuzz primarily saves crashes to the corpus directory (workspace)
	// Check corpus directory first as it's the primary location
	if _, err := os.Stat(hf.corpusDir); err == nil {
		files, err := os.ReadDir(hf.corpusDir)
		if err != nil {
			hf.logger.WithError(err).Warn("Failed to read corpus directory")
		} else {
			hf.logger.WithFields(logrus.Fields{
				"corpus_dir": hf.corpusDir,
				"file_count": len(files),
			}).Debug("Scanning corpus directory for crash files")

			for _, file := range files {
				fileName := file.Name()

				// Log every file for debugging
				hf.logger.WithFields(logrus.Fields{
					"file_name":       fileName,
					"is_dir":          file.IsDir(),
					"starts_with_dot": strings.HasPrefix(fileName, "."),
				}).Debug("Checking file in corpus directory")

				if file.IsDir() || strings.HasPrefix(fileName, ".") {
					continue
				}

				// Check if this is a crash file (contains SIGABRT, SIGSEGV, etc.)
				// HongFuzz crash files have pattern: SIGNAL.PC.address.STACK.hash.CODE.code.ADDR.addr.INSTR.[UNKNOWN].timestamp.fuzz
				if strings.Contains(fileName, "SIG") || strings.Contains(fileName, "crash") {
					hf.logger.WithFields(logrus.Fields{
						"file_name":      fileName,
						"contains_sig":   strings.Contains(fileName, "SIG"),
						"contains_crash": strings.Contains(fileName, "crash"),
					}).Info("File name matches crash pattern")
					crashPath := filepath.Join(hf.corpusDir, file.Name())

					hf.logger.WithFields(logrus.Fields{
						"crash_file": file.Name(),
						"crash_path": crashPath,
					}).Info("Found HongFuzz crash file in corpus directory")

					crashData, err := os.ReadFile(crashPath)
					if err != nil {
						hf.logger.WithError(err).WithField("file", file.Name()).Warn("Failed to read crash file from corpus")
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
						JobID:       hf.config.JobID, // Use actual job ID from config
						BotID:       hf.botID,
						Timestamp:   info.ModTime(),
						FilePath:    crashPath,
						Size:        int64(len(crashData)),
						Hash:        crashHash,
						Type:        crashType,
						Input:       crashData,
						InputBase64: base64.StdEncoding.EncodeToString(crashData),
						IsUnique:    true, // Mark as unique for now
					}

					hf.logger.WithFields(logrus.Fields{
						"crash_id":   crash.ID,
						"crash_type": crashType,
						"crash_hash": crashHash,
						"crash_size": crash.Size,
						"source":     "corpus",
					}).Info("Detected HongFuzz crash")

					crashes = append(crashes, crash)
				}
			}
		}
	} else {
		hf.logger.WithFields(logrus.Fields{
			"corpus_dir": hf.corpusDir,
			"error":      err,
		}).Debug("HongFuzz corpus directory does not exist yet")
	}

	// Also check the crashes directory (though HongFuzz rarely uses it)
	if _, err := os.Stat(hf.crashDir); err == nil {
		files, err := os.ReadDir(hf.crashDir)
		if err != nil {
			hf.logger.WithError(err).Warn("Failed to read crash directory")
		} else if len(files) > 0 {
			hf.logger.WithFields(logrus.Fields{
				"crash_dir":  hf.crashDir,
				"file_count": len(files),
			}).Debug("Checking crash directory for additional crashes")

			for _, file := range files {
				if file.IsDir() || strings.HasPrefix(file.Name(), ".") {
					continue
				}

				crashPath := filepath.Join(hf.crashDir, file.Name())

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

				// Check if we already have this crash from corpus
				duplicate := false
				for _, existing := range crashes {
					if existing.Hash == crashHash {
						duplicate = true
						break
					}
				}

				if !duplicate {
					crash := &common.CrashResult{
						ID:          file.Name(),
						JobID:       hf.config.JobID, // Use actual job ID from config
						BotID:       hf.botID,
						Timestamp:   info.ModTime(),
						FilePath:    crashPath,
						Size:        int64(len(crashData)),
						Hash:        crashHash,
						Type:        crashType,
						Input:       crashData,
						InputBase64: base64.StdEncoding.EncodeToString(crashData),
						IsUnique:    true, // Mark as unique for now
					}

					hf.logger.WithFields(logrus.Fields{
						"crash_id":   crash.ID,
						"crash_type": crashType,
						"crash_hash": crashHash,
						"source":     "crashes",
					}).Info("Found additional crash in crashes directory")

					crashes = append(crashes, crash)
				}
			}
		}
	}

	hf.logger.WithFields(logrus.Fields{
		"total_crashes": len(crashes),
		"corpus_dir":    hf.corpusDir,
		"job_id":        hf.config.Target,
	}).Info("Completed HongFuzz crash scan")

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

	// Input directory - ALWAYS required for honggfuzz
	// Use seed directory if provided, otherwise use corpus directory
	inputDir := hf.config.SeedDirectory
	if inputDir == "" {
		inputDir = hf.corpusDir
	}
	args = append(args, "--input", inputDir)

	// Output/crash directory
	args = append(args, "--output", hf.crashDir)

	// Workspace directory (for corpus)
	args = append(args, "--workspace", hf.corpusDir)

	// Number of threads - set to 1 for controlled resource usage
	args = append(args, "-n", "1")

	// Memory limit
	args = append(args, "--rlimit_rss", fmt.Sprintf("%d", hf.config.MemoryLimit))

	// Timeout (per-test timeout)
	timeoutSec := int(hf.config.Timeout.Seconds())
	if timeoutSec < 1 {
		timeoutSec = 1
	}
	args = append(args, "--timeout", strconv.Itoa(timeoutSec))

	// Total runtime duration (-T flag)
	if hf.config.Duration > 0 {
		// Convert duration to seconds for honggfuzz
		runtimeSec := int(hf.config.Duration.Seconds())
		if runtimeSec > 0 {
			args = append(args, "-T", strconv.Itoa(runtimeSec))
			hf.logger.WithFields(logrus.Fields{
				"runtime_seconds": runtimeSec,
				"duration":        hf.config.Duration,
			}).Debug("Setting honggfuzz runtime duration")
		}
	}

	// Dictionary
	if hf.config.Dictionary != "" {
		args = append(args, "--dict", hf.config.Dictionary)
	}

	// Extract HongFuzzConfig from FuzzerOptions
	var hongFuzzConfig *HongFuzzConfig
	if hongFuzzOpts, ok := hf.config.FuzzerOptions["honggfuzz_config"].(HongFuzzConfig); ok {
		hongFuzzConfig = &hongFuzzOpts
	} else if hongFuzzOpts, ok := hf.config.FuzzerOptions["honggfuzz_config"].(*HongFuzzConfig); ok {
		hongFuzzConfig = hongFuzzOpts
	}

	// Apply HongFuzz-specific configuration if available
	if hongFuzzConfig != nil {
		// Persistent mode support
		if hongFuzzConfig.PersistentMode {
			args = append(args, "-P")
		}

		// Instrumentation support
		if hongFuzzConfig.UseInstrumentation {
			args = append(args, "-z")
		}

		// Crash verification
		if hongFuzzConfig.VerifyCrashes {
			args = append(args, "-V")
		}

		// Hardware feedback flags
		switch hongFuzzConfig.HardwareFeedback {
		case "instructions":
			args = append(args, "--linux_perf_instr")
		case "branches":
			args = append(args, "--linux_perf_branch")
		case "edges":
			args = append(args, "--linux_perf_bts_edge")
		}

		// Report file
		if hongFuzzConfig.ReportFile != "" {
			args = append(args, "-R", hongFuzzConfig.ReportFile)
		}

		// Mutations per run
		if hongFuzzConfig.MutationsPerRun > 0 {
			args = append(args, "--mutations_per_run", strconv.Itoa(hongFuzzConfig.MutationsPerRun))
		}

		// Max file size
		if hongFuzzConfig.MaxFileSize > 0 {
			args = append(args, "--max_file_size", strconv.Itoa(hongFuzzConfig.MaxFileSize))
		}
	}

	// Don't exit upon crash - we want to continue fuzzing to find more crashes
	// args = append(args, "--exit_upon_crash")

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

	// Add recover to handle pipe closure gracefully
	defer func() {
		if r := recover(); r != nil {
			hf.logger.WithField("panic", r).Warn("Recovered from panic in monitorOutput")
		}
	}()

	// Write that we started monitoring
	if hf.config.OutputWriter != nil {
		timestamp := time.Now().Format("15:04:05")
		fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] Started monitoring %s\n", timestamp, "info", "fuzzer", name)
	}

	scanner := bufio.NewScanner(pipe)
	// Increase buffer size for scanner to handle long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Write to output file if available
		if hf.config.OutputWriter != nil {
			timestamp := time.Now().Format("15:04:05")
			_, err := fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] [%s] %s\n", timestamp, "info", name, "honggfuzz", line)
			if err != nil {
				hf.logger.WithError(err).Error("Failed to write to OutputWriter")
			}
		}

		// Log output - use Info for both streams to ensure we see everything
		hf.logger.WithFields(logrus.Fields{
			"stream": name,
			"line":   line,
		}).Info("HongFuzz output")

		// Parse Honggfuzz output for important information
		// Look for specific crash indicators, not just any line containing "crash"
		// HongFuzz reports crashes in format: "Crashes: X (unique: Y, blacklist: Z, verified: W)"
		if strings.Contains(line, "Crashes:") && strings.Contains(line, "(unique:") {
			// Extract crash count from line
			if parts := strings.Split(line, "Crashes:"); len(parts) > 1 {
				if crashParts := strings.Fields(parts[1]); len(crashParts) > 0 {
					if crashCount, err := strconv.Atoi(crashParts[0]); err == nil && crashCount > 0 {
						hf.detectAndEmitCrash(line)
					}
				}
			}
		}
		// Also detect specific crash messages from HongFuzz
		if strings.Contains(line, "Crash found") || strings.Contains(line, "Found issue!") {
			hf.detectAndEmitCrash(line)
		}

		// Parse stats from output
		hf.parseOutputStats(line)
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		// Only log as error if not due to context cancellation or file closure
		if hf.ctx.Err() == nil && !strings.Contains(err.Error(), "file already closed") && !strings.Contains(err.Error(), "bad file descriptor") {
			hf.logger.WithError(err).WithField("stream", name).Error("Scanner error in monitorOutput")
			if hf.config.OutputWriter != nil {
				timestamp := time.Now().Format("15:04:05")
				fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] Scanner error on %s: %v\n", timestamp, "error", "fuzzer", name, err)
			}
		} else {
			hf.logger.WithField("stream", name).Debug("Scanner stopped due to pipe closure or context cancellation")
		}
	}

	// Log that monitoring ended
	if hf.config.OutputWriter != nil {
		timestamp := time.Now().Format("15:04:05")
		fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] Stopped monitoring %s\n", timestamp, "info", "fuzzer", name)
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

	// Get process PID for logging
	pid := -1
	if hf.cmd.Process != nil {
		pid = hf.cmd.Process.Pid
	}

	hf.logger.WithField("pid", pid).Info("Starting to monitor Honggfuzz process")

	// Log process start
	if hf.config.OutputWriter != nil {
		timestamp := time.Now().Format("15:04:05")
		fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] Monitoring HongFuzz process (PID: %d)\n", timestamp, "info", "fuzzer", pid)
	}

	// Create a channel to signal when process exits
	processDone := make(chan error, 1)
	go func() {
		// Wait for process to exit
		err := hf.cmd.Wait()
		processDone <- err
	}()

	// Monitor with timeout to prevent hanging on zombies
	var processErr error
	select {
	case err := <-processDone:
		processErr = err

		// Get exit code if available
		exitCode := -1
		if exitError, ok := err.(*exec.ExitError); ok {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			}
		}

		hf.logger.WithFields(logrus.Fields{
			"pid":       pid,
			"error":     err,
			"exit_code": exitCode,
		}).Error("Honggfuzz process exited")

		// Process exited
		if hf.config.OutputWriter != nil {
			timestamp := time.Now().Format("15:04:05")
			fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] HongFuzz process exited (PID: %d, exit_code: %d)\n", timestamp, "error", "fuzzer", pid, exitCode)
			if err != nil {
				fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] Exit error: %v\n", timestamp, "error", "fuzzer", err)
			}
		}
	case <-hf.ctx.Done():
		// Context cancelled, force cleanup
		if hf.cmd.Process != nil {
			// Try to kill the process group to ensure all children are cleaned up
			if err := syscall.Kill(-hf.cmd.Process.Pid, syscall.SIGKILL); err != nil {
				hf.logger.WithError(err).Debug("Failed to kill process group")
			}
			// Also kill the process directly
			hf.cmd.Process.Kill()
		}
		// Wait briefly for cleanup
		select {
		case err := <-processDone:
			processErr = err
			// Process cleaned up
		case <-time.After(2 * time.Second):
			// Force reap the zombie
			if hf.cmd.Process != nil {
				hf.cmd.Process.Release()
			}
		}
		if hf.config.OutputWriter != nil {
			timestamp := time.Now().Format("15:04:05")
			fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] HongFuzz process force cleaned\n", timestamp, "info", "fuzzer")
		}
	}

	hf.mu.Lock()
	defer hf.mu.Unlock()

	if processErr != nil {
		exitCode := -1
		if hf.cmd.ProcessState != nil {
			exitCode = hf.cmd.ProcessState.ExitCode()
		}
		hf.logger.WithError(processErr).WithFields(logrus.Fields{
			"exit_code": exitCode,
			"pid":       hf.cmd.Process.Pid,
		}).Warn("Honggfuzz process exited with error")

		// Write exit info to OutputWriter
		if hf.config.OutputWriter != nil {
			timestamp := time.Now().Format("15:04:05")
			fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] Process exit code: %d\n", timestamp, "error", "fuzzer", exitCode)
		}

		hf.status = StatusError
		if hf.eventHandler != nil {
			hf.eventHandler.OnError(hf, processErr)
		}
		// Emit error event through base fuzzer
		hf.EmitErrorEvent(hf.ctx, hf.config.Target, processErr)
	} else {
		exitCode := 0
		if hf.cmd.ProcessState != nil {
			exitCode = hf.cmd.ProcessState.ExitCode()
		}
		hf.logger.WithFields(logrus.Fields{
			"exit_code": exitCode,
			"pid":       hf.cmd.Process.Pid,
		}).Info("Honggfuzz process exited normally")

		// Write exit info to OutputWriter
		if hf.config.OutputWriter != nil {
			timestamp := time.Now().Format("15:04:05")
			fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] Process exited normally with code: %d\n", timestamp, "info", "fuzzer", exitCode)
		}

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
				// Also monitor the stats file and write to OutputWriter
				hf.monitorStatsFile()
			}
		}
	}()
}

// monitorStatsFile reads the HongFuzz stats file and writes it to the OutputWriter
func (hf *Honggfuzz) monitorStatsFile() {
	if hf.config.OutputWriter == nil {
		return
	}

	// Read the stats file
	data, err := os.ReadFile(hf.statsFile)
	if err != nil {
		// File might not exist yet early in fuzzing
		return
	}

	// Only write if the file has content
	if len(data) == 0 {
		return
	}

	// Write stats file content to OutputWriter
	timestamp := time.Now().Format("15:04:05")

	// Add a separator for clarity
	fmt.Fprintf(hf.config.OutputWriter, "\n[%s] [%s] [%s] === HongFuzz Stats Update ===\n", timestamp, "info", "stats")

	// Parse and format key stats from the file
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		// Format important lines
		if strings.Contains(line, "Summary iterations:") {
			// Parse summary line for key metrics
			fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] %s\n", timestamp, "info", "stats", line)
		} else if strings.Contains(line, "Crash:") || strings.Contains(line, "Entering phase") || strings.Contains(line, "Launched") {
			// Include important status updates
			fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] %s\n", timestamp, "info", "stats", line)
		} else if strings.Contains(line, "Start time:") {
			// Include start info
			fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] %s\n", timestamp, "info", "stats", line)
		}
	}

	// Also check for the latest line which often contains the current stats
	if len(lines) > 0 {
		lastLine := lines[len(lines)-1]
		if lastLine != "" && strings.Contains(lastLine, "speed:") {
			fmt.Fprintf(hf.config.OutputWriter, "[%s] [%s] [%s] Current: %s\n", timestamp, "info", "stats", lastLine)
		}
	}
}

func (hf *Honggfuzz) updateStats() {
	// Try to parse the HongFuzz report file for accurate statistics
	report, err := hf.parseReportFile()
	if err == nil && report != nil {
		hf.mu.Lock()
		// Update stats from report data
		hf.stats.Executions = int64(report.Iterations)
		hf.stats.UniqueCrashes = int(report.Crashes)
		hf.stats.TotalCrashes = int(report.Crashes) // HongFuzz typically reports unique crashes
		hf.stats.CoveragePercent = report.Coverage
		hf.stats.ExecPerSecond = report.Speed

		// Update corpus size based on coverage metrics
		if report.GuardCoverage > 0 {
			hf.stats.TotalEdges = int(report.GuardCoverage)
		}
		if report.BranchCoverage > 0 {
			hf.stats.PathsTotal = int(report.BranchCoverage)
		}

		// Track peak execution speed
		if report.Speed > hf.peakExecSpeed {
			hf.peakExecSpeed = report.Speed
		}

		// Add to execution history
		hf.execHistory = append(hf.execHistory, report.Speed)
		if len(hf.execHistory) > 100 {
			hf.execHistory = hf.execHistory[1:]
		}

		// Calculate coverage growth rate
		elapsed := time.Since(hf.stats.StartTime)
		if elapsed.Seconds() > 0 && hf.stats.NewPaths > 0 {
			coverageGrowthRate := float64(hf.stats.NewPaths) / elapsed.Seconds()
			hf.logger.WithFields(logrus.Fields{
				"coverage_growth_rate": coverageGrowthRate,
				"coverage_percent":     report.Coverage,
			}).Debug("Calculated coverage growth rate")
		}
		hf.mu.Unlock()
	} else {
		// Fall back to parsing from stats file or output
		if data, err := os.ReadFile(hf.statsFile); err == nil {
			// Parse stats from log file
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				hf.parseOutputStats(line)
			}
		}
	}

	hf.mu.Lock()
	defer hf.mu.Unlock()

	// Update derived stats
	hf.stats.ElapsedTime = time.Since(hf.stats.StartTime)
	if hf.stats.Executions > 0 && hf.stats.ElapsedTime.Seconds() > 0 {
		// Only update exec per second if not already set from report
		if report == nil || report.Speed == 0 {
			hf.stats.ExecPerSecond = float64(hf.stats.Executions) / hf.stats.ElapsedTime.Seconds()
		}
	}

	// Get system resource usage
	if hf.cmd != nil && hf.cmd.Process != nil {
		// This would require platform-specific code to get actual CPU/memory usage
		// For now, using placeholder values
		hf.stats.CPUUsage = 50.0
		hf.stats.MemoryUsage = hf.config.MemoryLimit * 1024 * 1024
	}

	// Collect enhanced metrics with accurate data
	hf.collectEnhancedMetricsFromReport(report)

	// Notify event handler
	if hf.eventHandler != nil {
		hf.eventHandler.OnStats(hf, hf.stats)
		hf.eventHandler.OnProgress(hf, hf.GetProgress())
	}

	// Emit stats event through base fuzzer with accurate data
	hf.parseAndEmitStats(map[string]string{
		"execs_done":     strconv.FormatInt(hf.stats.Executions, 10),
		"execs_per_sec":  strconv.FormatFloat(hf.stats.ExecPerSecond, 'f', 2, 64),
		"paths_total":    strconv.Itoa(hf.stats.PathsTotal),
		"unique_crashes": strconv.Itoa(hf.stats.UniqueCrashes),
		"total_crashes":  strconv.Itoa(hf.stats.TotalCrashes),
		"coverage":       strconv.FormatFloat(hf.stats.CoveragePercent, 'f', 2, 64),
	})

	// Check for new crashes
	hf.checkForNewCrashes()
}

func (hf *Honggfuzz) checkForNewCrashes() {
	crashes, err := hf.GetCrashes()
	if err != nil {
		hf.logger.WithError(err).Error("Failed to get crashes in checkForNewCrashes")
		return
	}

	hf.logger.WithFields(logrus.Fields{
		"crashes_found":  len(crashes),
		"previous_total": hf.stats.TotalCrashes,
	}).Debug("Checking for new crashes")

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
	// Return "hongfuzz" for all HongFuzz crashes instead of specific signal types
	return "hongfuzz"
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

// collectEnhancedMetricsFromReport collects enhanced metrics from HongFuzz report
func (hf *Honggfuzz) collectEnhancedMetricsFromReport(report *common.HongFuzzReport) {
	// Coverage metrics
	coverage := common.CoverageMetrics{
		NewEdgesFound:    int64(hf.stats.NewPaths),
		CoverageByModule: make(map[string]float64),
	}

	if report != nil {
		// Use accurate coverage data from report
		coverage.LineCoverage = report.Coverage
		coverage.BranchCoverage = report.Coverage

		// Calculate coverage growth rate
		if hf.stats.ElapsedTime.Seconds() > 0 {
			coverage.CoverageGrowth = float64(hf.stats.NewPaths) / hf.stats.ElapsedTime.Seconds()
		}

		// Use guard and branch coverage if available
		if report.GuardCoverage > 0 {
			coverage.TotalEdges = int64(report.GuardCoverage)
		}
		if report.BranchCoverage > 0 {
			coverage.BranchCoverage = float64(report.BranchCoverage) / float64(hf.stats.PathsTotal) * 100
		}
	} else {
		// Use estimated coverage from stats
		coverage.LineCoverage = hf.stats.CoveragePercent
		coverage.BranchCoverage = hf.stats.CoveragePercent

		// Calculate coverage growth rate
		if hf.stats.ElapsedTime.Seconds() > 0 {
			coverage.CoverageGrowth = float64(hf.stats.NewPaths) / hf.stats.ElapsedTime.Seconds()
		}
	}

	// Performance metrics
	performance := common.PerformanceMetrics{
		ExecutionsPerSecond: hf.stats.ExecPerSecond,
		ThroughputMBps:      hf.calculateThroughput(),
		InputGenerationRate: hf.stats.ExecPerSecond,
	}

	// Calculate efficiency metrics
	if report != nil && report.Speed > 0 {
		// Calculate fuzzing efficiency (coverage per execution)
		if report.Iterations > 0 {
			efficiency := report.Coverage / float64(report.Iterations) * 1000000 // Coverage per million execs
			hf.metricsCollector.UpdateFuzzerSpecificMetrics("efficiency", fmt.Sprintf("%.4f", efficiency))
		}

		// Track timeout rate
		if report.Iterations > 0 {
			timeoutRate := float64(report.Timeouts) / float64(report.Iterations) * 100
			hf.metricsCollector.UpdateFuzzerSpecificMetrics("timeout_rate", fmt.Sprintf("%.2f%%", timeoutRate))
		}
	}

	// Update metrics collector
	hf.metricsCollector.UpdateCoverageMetrics(coverage)
	hf.metricsCollector.UpdatePerformanceMetrics(performance)

	// HongFuzz specific metrics
	if report != nil {
		hf.metricsCollector.UpdateFuzzerSpecificMetrics("report_iterations", strconv.FormatUint(report.Iterations, 10))
		hf.metricsCollector.UpdateFuzzerSpecificMetrics("report_crashes", strconv.FormatUint(report.Crashes, 10))
		hf.metricsCollector.UpdateFuzzerSpecificMetrics("report_timeouts", strconv.FormatUint(report.Timeouts, 10))
		hf.metricsCollector.UpdateFuzzerSpecificMetrics("report_coverage", fmt.Sprintf("%.2f%%", report.Coverage))
		hf.metricsCollector.UpdateFuzzerSpecificMetrics("report_speed", fmt.Sprintf("%.0f", report.Speed))
	}
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

// parseReportFile parses the HongFuzz report file for accurate statistics
func (hf *Honggfuzz) parseReportFile() (*common.HongFuzzReport, error) {
	// Check if report file is configured
	reportFile, ok := hf.config.FuzzerOptions["report_file"].(string)
	if !ok || reportFile == "" {
		// Default report file name
		reportFile = "honggfuzz.report"
	}

	reportPath := filepath.Join(hf.outputDir, reportFile)
	data, err := os.ReadFile(reportPath)
	if err != nil {
		// Report file might not exist yet early in fuzzing
		return nil, err
	}

	report := &common.HongFuzzReport{}
	lines := strings.Split(string(data), "\n")

	// Parse key:value pairs from report file
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Parse known fields
		switch key {
		case "Iterations":
			report.Iterations, _ = strconv.ParseUint(value, 10, 64)
		case "Crashes":
			report.Crashes, _ = strconv.ParseUint(value, 10, 64)
		case "Timeouts":
			report.Timeouts, _ = strconv.ParseUint(value, 10, 64)
		case "Coverage":
			// Coverage might be a percentage like "85.3%"
			value = strings.TrimSuffix(value, "%")
			report.Coverage, _ = strconv.ParseFloat(value, 64)
		case "Speed":
			// Speed might include units like "1234 exec/s"
			fields := strings.Fields(value)
			if len(fields) > 0 {
				report.Speed, _ = strconv.ParseFloat(fields[0], 64)
			}
		case "GuardCoverage":
			report.GuardCoverage, _ = strconv.ParseUint(value, 10, 64)
		case "BranchCoverage":
			report.BranchCoverage, _ = strconv.ParseUint(value, 10, 64)
		}
	}

	return report, nil
}

// CreateHonggfuzz creates a new Honggfuzz instance with optional logger
func CreateHonggfuzz(logger *logrus.Logger) (Fuzzer, error) {
	return NewHonggfuzz(logger), nil
}
