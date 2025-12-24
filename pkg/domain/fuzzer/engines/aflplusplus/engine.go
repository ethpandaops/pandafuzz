package aflplusplus

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
)

// Verify interface compliance
var _ types.Fuzzer = (*Engine)(nil)

// Engine implements the Fuzzer interface for AFL++
type Engine struct {
	// Configuration
	target    string
	args      []string
	config    *types.FuzzerConfig
	inputDir  string
	outputDir string

	// Runtime state
	isRunning atomic.Bool
	version   string

	// Process management
	cmd        *exec.Cmd
	ctx        context.Context
	cancelFunc context.CancelFunc
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderr     io.ReadCloser

	// Channels
	crashChan    chan *types.CrashInfo
	progressChan chan *types.ProgressUpdate

	// Statistics tracking
	stats      *types.FuzzerStats
	statsMutex sync.RWMutex

	// AFL++ specific stats
	lastPaths        uint64
	lastCrashes      uint64
	lastHangs        uint64
	lastExecs        uint64
	lastSpeed        uint64
	lastPendingPaths uint64
	lastPendingFavs  uint64

	// Crash tracking
	crashes          map[string]*types.CrashInfo
	crashesMutex     sync.RWMutex
	seenCrashes      map[string]bool
	seenCrashesMutex sync.Mutex

	// Coverage tracking
	coverageData  map[string]interface{}
	coverageMutex sync.RWMutex

	// Synchronization
	wg            sync.WaitGroup
	statsUpdateMu sync.Mutex
	lastStatsTime time.Time

	// Logging
	log logrus.FieldLogger
}

// NewEngine creates a new AFL++ engine instance
func NewEngine(target string, args []string, log logrus.FieldLogger) *Engine {
	if log == nil {
		log = logrus.New()
	}

	engine := &Engine{
		target:       target,
		args:         args,
		crashChan:    make(chan *types.CrashInfo, 100),
		progressChan: make(chan *types.ProgressUpdate, 100),
		crashes:      make(map[string]*types.CrashInfo),
		seenCrashes:  make(map[string]bool),
		stats: &types.FuzzerStats{
			StartTime: time.Now(),
		},
		coverageData: make(map[string]interface{}),
		log:          log.WithField("engine", "afl++"),
	}

	// Try to get version
	engine.version = engine.detectVersion()

	return engine
}

// Start begins the fuzzing process
func (e *Engine) Start(ctx context.Context) error {
	if e.isRunning.Load() {
		return errors.New("fuzzer is already running")
	}

	// Validate configuration
	if e.config != nil {
		if err := e.config.Validate(); err != nil {
			return fmt.Errorf("invalid configuration: %w", err)
		}
	}

	// Ensure we have AFL++ options
	if e.config == nil || e.config.AFLPlusPlusOptions == nil {
		return errors.New("AFL++ configuration is required")
	}

	// Set input directory
	e.inputDir = e.config.AFLPlusPlusOptions.InputDir
	if e.inputDir == "" {
		return errors.New("input directory is required for AFL++")
	}

	// Create directories
	if err := e.ensureDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Build command arguments
	cmdArgs := e.buildCommandArgs()

	// Find afl-fuzz binary
	aflBinary := "afl-fuzz"
	if path, err := exec.LookPath("afl-fuzz"); err == nil {
		aflBinary = path
	}

	// Create command
	e.ctx, e.cancelFunc = context.WithCancel(ctx)
	e.cmd = exec.CommandContext(e.ctx, aflBinary, cmdArgs...)

	// Log the command being executed
	e.log.WithFields(logrus.Fields{
		"binary": aflBinary,
		"args":   strings.Join(cmdArgs, " "),
	}).Info("Starting AFL++ with command")

	// Set process group for proper signal handling (Linux-specific)
	e.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pgid:      0,               // Create new process group
		Pdeathsig: syscall.SIGKILL, // Kill children if parent dies
	}

	// Set environment
	env := os.Environ()
	if e.config.Environment != nil {
		for k, v := range e.config.Environment {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Configure coverage environment if enabled
	if e.config != nil && e.config.EnableCoverage {
		if err := e.configureCoverageEnvironment(&env); err != nil {
			e.log.WithError(err).Warn("Failed to configure coverage environment")
		}
	}

	// AFL++ specific environment variables
	env = append(env, "AFL_SKIP_CPUFREQ=1")
	env = append(env, "AFL_I_DONT_CARE_ABOUT_MISSING_CRASHES=1")
	// Enable fork-server mode
	env = append(env, "AFL_NO_FORKSRV=0")
	env = append(env, "AFL_FORKSRV_INIT_TIMEOUT=30000") // 30s timeout
	env = append(env, fmt.Sprintf("__AFL_SHM_ID=afl_%d_%d", os.Getpid(), time.Now().UnixNano()))
	env = append(env, "AFL_MAP_SIZE=65536") // Standard AFL map size

	// Set AFL_PATH to standard location where AFL++ libraries are installed
	// This helps AFL++ find afl-compiler-rt.o and other runtime files
	aflPath := "/usr/local/lib/afl"
	if _, err := os.Stat(aflPath); err == nil {
		env = append(env, fmt.Sprintf("AFL_PATH=%s", aflPath))
		e.log.WithField("AFL_PATH", aflPath).Debug("Setting AFL_PATH environment variable")
	} else {
		// Try alternative common locations
		alternativePaths := []string{
			"/usr/lib/afl",
			"/usr/share/afl",
			"/opt/afl++",
		}
		pathFound := false
		for _, altPath := range alternativePaths {
			if _, err := os.Stat(altPath); err == nil {
				env = append(env, fmt.Sprintf("AFL_PATH=%s", altPath))
				e.log.WithField("AFL_PATH", altPath).Debug("Setting AFL_PATH to alternative location")
				pathFound = true
				break
			}
		}
		if !pathFound {
			// If AFL_PATH cannot be determined, skip binary checks to avoid failures
			e.log.Warn("AFL_PATH could not be determined, setting AFL_SKIP_BIN_CHECK=1")
			env = append(env, "AFL_SKIP_BIN_CHECK=1")
		}
	}

	// Also set AFL_SKIP_BIN_CHECK to allow fuzzing non-instrumented binaries
	// This is useful for testing and when binaries are not compiled with afl-cc
	env = append(env, "AFL_SKIP_BIN_CHECK=1")
	e.log.Debug("Setting AFL_SKIP_BIN_CHECK=1 to allow fuzzing non-instrumented binaries")

	e.cmd.Env = env

	// Setup pipes
	var err error
	e.stdin, err = e.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	e.stdout, err = e.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	e.stderr, err = e.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := e.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start fuzzer: %w", err)
	}

	e.isRunning.Store(true)
	e.stats.StartTime = time.Now()

	// Wait for AFL++ fork-server initialization
	e.log.Info("Waiting for AFL++ fork-server initialization...")
	if err := e.waitForInitialization(ctx, 5*time.Second); err != nil {
		e.log.WithError(err).Warn("AFL++ initialization check failed")
		// Don't fail, continue anyway as AFL++ might still work
	} else {
		e.log.Info("AFL++ fork-server initialized successfully")
	}

	// Start output processors
	e.wg.Add(2)
	go e.processOutput(e.stdout, "stdout")
	go e.processOutput(e.stderr, "stderr")

	// Start monitoring goroutines
	e.wg.Add(2)
	go e.monitorStats()
	go e.monitorCrashes()

	// Start process monitor
	e.wg.Add(1)
	go e.monitorProcess()

	// Start coverage collection if enabled
	if e.config != nil && e.config.EnableCoverage {
		e.wg.Add(1)
		go e.monitorCoverage()
	}

	e.log.Info("AFL++ started successfully")
	return nil
}

// Stop gracefully stops the fuzzing process
func (e *Engine) Stop() error {
	if !e.isRunning.Load() {
		return errors.New("fuzzer is not running")
	}

	e.log.Info("Stopping AFL++...")

	// Mark as not running FIRST to prevent race conditions
	// This ensures monitorCrashes will exit on next iteration
	e.isRunning.Store(false)

	// Cancel context to signal shutdown to all goroutines
	if e.cancelFunc != nil {
		e.cancelFunc()
	}

	// Try to gracefully terminate the process group
	if e.cmd != nil && e.cmd.Process != nil {
		pid := e.cmd.Process.Pid

		// Send SIGTERM to the process group
		if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
			e.log.WithError(err).Debug("Failed to send SIGTERM to process group")
		}
	}

	// Wait for ALL monitoring goroutines to fully exit
	// This ensures no concurrent access to seenCrashes map
	done := make(chan bool, 1)
	go func() {
		e.wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// All goroutines exited gracefully
		e.log.Info("AFL++ monitoring goroutines exited gracefully")
	case <-time.After(5 * time.Second):
		// Force kill if not exited
		if e.cmd != nil && e.cmd.Process != nil {
			pid := e.cmd.Process.Pid
			e.log.Warn("Force killing AFL++ process group")

			// Kill the entire process group
			if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
				e.log.WithError(err).Error("Failed to kill process group")
				// Try to kill just the process
				e.cmd.Process.Kill()
			}

			// Wait a bit more for cleanup
			select {
			case <-done:
				e.log.Info("AFL++ process killed successfully")
			case <-time.After(2 * time.Second):
				e.log.Warn("AFL++ process may not have fully terminated")
			}
		}
	}

	// NOW do final crash scan - all monitoring goroutines have exited
	// This is safe because no concurrent access to seenCrashes or crashChan
	e.log.Info("Performing final crash scan (all goroutines stopped)...")
	e.checkForCrashesWithTimeout(3 * time.Second)

	// Close channels AFTER final scan completes
	close(e.crashChan)
	close(e.progressChan)

	e.log.Info("AFL++ stopped")
	return nil
}

// checkForCrashesWithTimeout wraps checkForCrashes with a timeout
func (e *Engine) checkForCrashesWithTimeout(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		e.checkForCrashes()
		close(done)
	}()

	select {
	case <-done:
		e.log.Debug("Final crash scan completed")
	case <-time.After(timeout):
		e.log.Warn("Final crash scan timed out, some crashes may be missed")
	}
}

// GetStats returns current fuzzing statistics
func (e *Engine) GetStats() (*types.FuzzerStats, error) {
	if !e.isRunning.Load() {
		return nil, errors.New("fuzzer is not running")
	}

	e.statsMutex.RLock()
	defer e.statsMutex.RUnlock()

	// Create a copy of stats
	stats := *e.stats
	stats.RunTime = time.Since(e.stats.StartTime)

	return &stats, nil
}

// GetCrashes returns a channel that emits discovered crashes
func (e *Engine) GetCrashes() <-chan *types.CrashInfo {
	return e.crashChan
}

// GetProgress returns a channel that emits progress updates
func (e *Engine) GetProgress() <-chan *types.ProgressUpdate {
	return e.progressChan
}

// IsRunning checks if the fuzzer is currently running
func (e *Engine) IsRunning() bool {
	return e.isRunning.Load()
}

// GetType returns the fuzzer engine type
func (e *Engine) GetType() string {
	return types.FuzzerTypeAFLPlusPlus.String()
}

// GetVersion returns the fuzzer engine version
func (e *Engine) GetVersion() string {
	return e.version
}

// SetCorpus sets the input corpus directory
func (e *Engine) SetCorpus(path string) error {
	if e.isRunning.Load() {
		return errors.New("cannot set corpus while fuzzer is running")
	}
	if e.config != nil && e.config.AFLPlusPlusOptions != nil {
		e.config.AFLPlusPlusOptions.InputDir = path
	}
	return nil
}

// SetOutput sets the output directory for crashes and artifacts
func (e *Engine) SetOutput(path string) error {
	if e.isRunning.Load() {
		return errors.New("cannot set output directory while fuzzer is running")
	}
	e.outputDir = path
	return nil
}

// Configure applies fuzzer-specific configuration
func (e *Engine) Configure(config *types.FuzzerConfig) error {
	if e.isRunning.Load() {
		return errors.New("cannot configure while fuzzer is running")
	}
	e.config = config

	// Set target binary from config if not already set
	if e.target == "" && config.Target != "" {
		e.target = config.Target
		e.log.WithField("target", e.target).Info("Set target binary from config")
	}

	// Set target args from config
	if len(e.args) == 0 && len(config.TargetArgs) > 0 {
		e.args = config.TargetArgs
	}

	// Set output directory from config if not already set
	if e.outputDir == "" && config.OutputDir != "" {
		e.outputDir = config.OutputDir
	}

	return nil
}

// isInstrumented checks if the binary is instrumented for AFL++
func (e *Engine) isInstrumented() bool {
	// Check for AFL++ instrumentation signatures in the binary
	// This is a basic check - looking for __afl_ symbols
	cmd := exec.Command("strings", e.target)
	output, err := cmd.Output()
	if err != nil {
		e.log.WithError(err).Warn("Failed to check binary instrumentation, assuming not instrumented")
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
			e.log.Debug("Found AFL++ instrumentation marker in binary")
			return true
		}
	}

	e.log.Warn("No AFL++ instrumentation detected in binary, will use dumb mode")
	return false
}

// buildCommandArgs builds the command line arguments for AFL++
func (e *Engine) buildCommandArgs() []string {
	args := []string{}
	opts := e.config.AFLPlusPlusOptions

	// Input directory (required)
	args = append(args, "-i", e.inputDir)

	// Output directory
	outputDir := e.outputDir
	if outputDir == "" && e.config != nil {
		outputDir = e.config.OutputDir
	}
	if outputDir == "" {
		outputDir = "/tmp/afl-output"
	}
	args = append(args, "-o", outputDir)

	// Memory limit
	if e.config.MemoryLimit > 0 {
		memMB := e.config.MemoryLimit / (1024 * 1024)
		e.log.WithFields(logrus.Fields{
			"memory_limit_bytes": e.config.MemoryLimit,
			"memory_limit_mb":    memMB,
		}).Debug("Converting memory limit from bytes to MB")
		if memMB == 0 {
			// If less than 1MB, use minimum of 1MB
			e.log.Debug("Memory limit less than 1MB, using minimum of 1MB")
			args = append(args, "-m", "1")
		} else {
			args = append(args, "-m", fmt.Sprintf("%d", memMB))
		}
	} else {
		e.log.Debug("No memory limit set, using 'none'")
		args = append(args, "-m", "none")
	}

	// Timeout (per test case execution timeout)
	if e.config.Timeout > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", int(e.config.Timeout.Milliseconds())))
	}

	// Time-limited fuzzing (graceful exit after specified seconds)
	// If MaxDuration is set, use AFL++'s -V flag to run for that duration
	if e.config.MaxDuration > 0 {
		// Convert duration to seconds
		seconds := int(e.config.MaxDuration.Seconds())
		if seconds > 0 {
			args = append(args, "-V", fmt.Sprintf("%d", seconds))
			e.log.WithField("duration_seconds", seconds).Info("AFL++ will run for limited time")
		}
	}

	// Dictionary
	if e.config.Dictionary != "" {
		args = append(args, "-x", e.config.Dictionary)
	}

	// AFL++ specific options
	if opts != nil {
		if opts.Mode != "" {
			args = append(args, "-p", opts.Mode)
		}

		if opts.PowerSchedule != "" {
			args = append(args, "-p", opts.PowerSchedule)
		}

		if opts.SkipCrashed {
			args = append(args, "-C")
		}

		if opts.NoUI {
			args = append(args, "-s")
		}

		if opts.Deterministic {
			args = append(args, "-D")
		}
	}

	// Check if we need dumb mode (either explicitly set or auto-detected)
	useDumbMode := false
	if opts != nil && opts.DumbMode {
		useDumbMode = true
		e.log.Info("Dumb mode explicitly enabled in configuration")
	} else if e.target != "" {
		// Auto-detect if binary is not instrumented
		if !e.isInstrumented() {
			e.log.Warn("Binary is not instrumented, automatically enabling dumb mode (-n)")
			useDumbMode = true
		}
	}

	if useDumbMode {
		args = append(args, "-n")
		e.log.Info("Running AFL++ in dumb mode (non-instrumented fuzzing)")
	}

	if opts != nil {
		if opts.MainNode {
			args = append(args, "-M", "main")
		} else if opts.SecondaryNode {
			args = append(args, "-S", "secondary")
		}

		if opts.FileExtension != "" {
			args = append(args, "-e", opts.FileExtension)
		}

		if opts.QemuMode {
			args = append(args, "-Q")
		}

		if opts.UniMode {
			args = append(args, "-U")
		}
	}

	// Add extra args
	if e.config.ExtraArgs != nil {
		args = append(args, e.config.ExtraArgs...)
	}

	// Add target and target args
	args = append(args, "--")
	args = append(args, e.target)
	args = append(args, e.args...)

	return args
}

// ensureDirectories creates necessary directories
func (e *Engine) ensureDirectories() error {
	dirs := []string{}

	if e.inputDir != "" {
		// Create input dir if it doesn't exist
		if _, err := os.Stat(e.inputDir); os.IsNotExist(err) {
			if err := os.MkdirAll(e.inputDir, 0755); err != nil {
				return fmt.Errorf("failed to create input directory %s: %w", e.inputDir, err)
			}
			e.log.WithField("input_dir", e.inputDir).Info("Created input directory")
		}

		// Check if input directory is empty and create a default seed
		entries, err := os.ReadDir(e.inputDir)
		if err != nil {
			return fmt.Errorf("failed to read input directory: %w", err)
		}
		if len(entries) == 0 {
			// Create a default seed file so AFL++ can start fuzzing
			seedPath := filepath.Join(e.inputDir, "seed_default")
			if err := os.WriteFile(seedPath, []byte("AAAA"), 0644); err != nil {
				return fmt.Errorf("failed to create default seed file: %w", err)
			}
			e.log.WithField("seed_file", seedPath).Info("Created default seed file")
		}
	}

	if e.outputDir != "" {
		dirs = append(dirs, e.outputDir)
	}
	if e.config != nil && e.config.OutputDir != "" {
		dirs = append(dirs, e.config.OutputDir)
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// processOutput processes stdout/stderr output from AFL++
func (e *Engine) processOutput(reader io.Reader, source string) {
	defer e.wg.Done()

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		e.log.WithField("source", source).Debug(line)

		// Write to output file if configured
		if e.config != nil && e.config.OutputWriter != nil {
			timestamp := time.Now().Format(time.RFC3339)
			fmt.Fprintf(e.config.OutputWriter, "%s [%s] %s\n", timestamp, source, line)
		}

		// AFL++ doesn't output stats to stdout/stderr in the same way as libfuzzer
		// Stats are read from files in the output directory
	}
}

// monitorStats monitors AFL++ statistics from the fuzzer_stats file
func (e *Engine) monitorStats() {
	defer e.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			if !e.isRunning.Load() {
				return
			}
			e.readStats()
		}
	}
}

// readStats reads statistics from AFL++ output files
func (e *Engine) readStats() {
	// AFL++ creates instance directories with fuzzer_stats
	var statsFile string
	if e.config.AFLPlusPlusOptions.MainNode {
		statsFile = filepath.Join(e.outputDir, "main", "fuzzer_stats")
	} else if e.config.AFLPlusPlusOptions.SecondaryNode {
		statsFile = filepath.Join(e.outputDir, "secondary", "fuzzer_stats")
	} else {
		// Default instance when no -M or -S is specified
		statsFile = filepath.Join(e.outputDir, "default", "fuzzer_stats")
	}

	data, err := os.ReadFile(statsFile)
	if err != nil {
		return // Stats file might not exist yet
	}

	stats := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			stats[key] = value
		}
	}

	// Parse statistics
	e.statsUpdateMu.Lock()
	defer e.statsUpdateMu.Unlock()

	now := time.Now()

	// Log detailed stats for debugging edge detection
	if edges, ok := stats["edges_found"]; ok {
		e.log.WithFields(logrus.Fields{
			"edges_found":    edges,
			"paths_total":    stats["paths_total"],
			"unique_crashes": stats["unique_crashes"],
			"execs_done":     stats["execs_done"],
		}).Debug("AFL++ edge coverage stats")
	}

	// Update stats
	if val, ok := stats["execs_done"]; ok {
		if execs, err := strconv.ParseUint(val, 10, 64); err == nil {
			e.lastExecs = execs
		}
	}

	if val, ok := stats["execs_per_sec"]; ok {
		if speed, err := strconv.ParseFloat(val, 64); err == nil {
			e.lastSpeed = uint64(speed)
		}
	}

	if val, ok := stats["paths_total"]; ok {
		if paths, err := strconv.ParseUint(val, 10, 64); err == nil {
			e.lastPaths = paths
		}
	}

	if val, ok := stats["unique_crashes"]; ok {
		if crashes, err := strconv.ParseUint(val, 10, 64); err == nil {
			e.lastCrashes = crashes
		}
	}

	if val, ok := stats["unique_hangs"]; ok {
		if hangs, err := strconv.ParseUint(val, 10, 64); err == nil {
			e.lastHangs = hangs
		}
	}

	if val, ok := stats["pending_paths"]; ok {
		if pending, err := strconv.ParseUint(val, 10, 64); err == nil {
			e.lastPendingPaths = pending
		}
	}

	if val, ok := stats["pending_favs"]; ok {
		if favs, err := strconv.ParseUint(val, 10, 64); err == nil {
			e.lastPendingFavs = favs
		}
	}

	// Calculate coverage percentage (approximate)
	coverage := float64(0)
	if e.lastPaths > 0 {
		coverage = float64(e.lastPaths-e.lastPendingPaths) / float64(e.lastPaths) * 100
	}

	// Update internal stats
	e.statsMutex.Lock()
	e.stats.TotalExecutions = e.lastExecs
	e.stats.ExecsPerSecond = e.lastSpeed
	e.stats.CorpusSize = e.lastPaths
	e.stats.Coverage = coverage
	e.stats.CrashesFound = e.lastCrashes
	e.stats.TimeoutsFound = e.lastHangs
	e.stats.RunTime = now.Sub(e.stats.StartTime)
	e.statsMutex.Unlock()

	// Send progress update
	if now.Sub(e.lastStatsTime) >= 100*time.Millisecond {
		e.lastStatsTime = now
		select {
		case e.progressChan <- &types.ProgressUpdate{
			Timestamp:      now,
			Executions:     e.lastExecs,
			ExecsPerSecond: e.lastSpeed,
			CorpusSize:     e.lastPaths,
			Coverage:       coverage,
			CrashCount:     e.lastCrashes,
		}:
		default:
			// Channel full, skip update
		}
	}
}

// monitorCrashes monitors for new crashes in the crashes directory
func (e *Engine) monitorCrashes() {
	defer e.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			if !e.isRunning.Load() {
				return
			}
			e.checkForCrashes()
		}
	}
}

// checkForCrashes checks for new crash files
func (e *Engine) checkForCrashes() {
	// AFL++ creates instance directories for crashes
	// Default is "default" when not using -M (main) or -S (secondary)
	var crashDir string
	if e.config != nil && e.config.AFLPlusPlusOptions.MainNode {
		crashDir = filepath.Join(e.outputDir, "main", "crashes")
	} else if e.config != nil && e.config.AFLPlusPlusOptions.SecondaryNode {
		crashDir = filepath.Join(e.outputDir, "secondary", "crashes")
	} else {
		// Default instance when no -M or -S is specified
		crashDir = filepath.Join(e.outputDir, "default", "crashes")
	}

	e.log.WithField("crash_dir", crashDir).Debug("Checking for crashes")

	files, err := os.ReadDir(crashDir)
	if err != nil {
		e.log.WithFields(logrus.Fields{
			"crash_dir": crashDir,
			"error":     err,
		}).Debug("Could not read crash directory")
		return // Crashes directory might not exist yet
	}

	e.log.WithFields(logrus.Fields{
		"crash_dir":  crashDir,
		"file_count": len(files),
	}).Debug("Found files in crash directory")

	for _, file := range files {
		if file.IsDir() || strings.HasSuffix(file.Name(), ".txt") {
			continue
		}

		// Check if we've seen this crash before (with mutex protection)
		e.seenCrashesMutex.Lock()
		if e.seenCrashes[file.Name()] {
			e.seenCrashesMutex.Unlock()
			continue
		}
		e.seenCrashes[file.Name()] = true
		e.seenCrashesMutex.Unlock()

		// Read crash file
		crashPath := filepath.Join(crashDir, file.Name())
		input, err := os.ReadFile(crashPath)
		if err != nil {
			e.log.WithError(err).Warn("Failed to read crash file")
			continue
		}

		// Create crash info
		crashID := e.generateCrashID()
		now := time.Now()

		crash := &types.CrashInfo{
			ID:           crashID,
			Input:        input,
			DiscoveredAt: now,
			FuzzerType:   e.GetType(),
			Metadata: map[string]string{
				"file": file.Name(),
				"path": crashPath,
			},
		}

		// Try to get stack trace from corresponding .txt file
		txtFile := crashPath + ".txt"
		if stackTrace, err := os.ReadFile(txtFile); err == nil {
			crash.StackTrace = string(stackTrace)
		}

		// Store crash
		e.crashesMutex.Lock()
		e.crashes[crashID] = crash
		e.crashesMutex.Unlock()

		// Update stats
		e.statsMutex.Lock()
		e.stats.LastCrashTime = &now
		e.statsMutex.Unlock()

		e.log.WithFields(logrus.Fields{
			"crash_id":   crashID,
			"crash_file": file.Name(),
			"crash_path": crashPath,
			"input_size": len(input),
		}).Info("New crash discovered")

		// Send to channel
		select {
		case e.crashChan <- crash:
			e.log.WithField("crash_id", crashID).Debug("Sent crash to channel")
		default:
			e.log.Warn("Crash channel full, dropping crash notification")
		}
	}
}

// generateCrashID generates a unique crash ID using UUID
func (e *Engine) generateCrashID() string {
	return uuid.New().String()
}

// monitorProcess monitors the fuzzer process
func (e *Engine) monitorProcess() {
	defer e.wg.Done()

	if e.cmd != nil && e.cmd.Process != nil {
		pid := e.cmd.Process.Pid
		e.log.WithField("pid", pid).Debug("Starting process monitor")

		// Periodically check process health
		healthTicker := time.NewTicker(5 * time.Second)
		defer healthTicker.Stop()

		done := make(chan error, 1)
		go func() {
			done <- e.cmd.Wait()
		}()

		for {
			select {
			case err := <-done:
				if err != nil && !errors.Is(err, context.Canceled) {
					e.log.WithFields(logrus.Fields{
						"pid":   pid,
						"error": err,
					}).Error("Fuzzer process exited with error")
				} else {
					e.log.WithField("pid", pid).Info("Fuzzer process exited normally")
				}
				e.isRunning.Store(false)
				return

			case <-healthTicker.C:
				if err := e.checkProcessHealth(pid); err != nil {
					e.log.WithFields(logrus.Fields{
						"pid":   pid,
						"error": err,
					}).Warn("Process health check failed")

					// If process is zombie, try to recover
					if strings.Contains(err.Error(), "zombie") {
						e.log.WithField("pid", pid).Error("AFL++ process became zombie, stopping fuzzer")
						e.isRunning.Store(false)
						return
					}
				} else {
					e.log.WithField("pid", pid).Debug("Process health check passed")
				}
			}
		}
	}

	e.isRunning.Store(false)
}

// waitForInitialization waits for AFL++ fork-server to initialize
func (e *Engine) waitForInitialization(ctx context.Context, timeout time.Duration) error {
	if e.cmd == nil || e.cmd.Process == nil {
		return fmt.Errorf("process not started")
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	startTime := time.Now()
	pid := e.cmd.Process.Pid

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("initialization timeout after %v", time.Since(startTime))
		case <-ticker.C:
			// Check if process is still alive
			if err := e.checkProcessHealth(pid); err != nil {
				return fmt.Errorf("process unhealthy during initialization: %w", err)
			}

			// Check for AFL++ shared memory or output directory creation
			if e.checkAFLInitialized() {
				e.log.Debug("AFL++ initialization detected")
				return nil
			}

			// After 1 second, assume initialized if process is still running
			if time.Since(startTime) > 1*time.Second {
				e.log.Debug("AFL++ process running after 1s, assuming initialized")
				return nil
			}
		}
	}
}

// checkProcessHealth checks if the process is healthy (not zombie)
func (e *Engine) checkProcessHealth(pid int) error {
	// Check if process exists
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process not found: %w", err)
	}

	// Send signal 0 to check if process is alive
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("process not responding: %w", err)
	}

	// Check for zombie state on Linux
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	if data, err := os.ReadFile(statPath); err == nil {
		statStr := string(data)
		// Find the last ')' to locate state field
		lastParen := -1
		for i := len(statStr) - 1; i >= 0; i-- {
			if statStr[i] == ')' {
				lastParen = i
				break
			}
		}
		if lastParen != -1 && lastParen+2 < len(statStr) {
			state := statStr[lastParen+2 : lastParen+3]
			if state == "Z" {
				return fmt.Errorf("process is zombie")
			}
		}
	}

	return nil
}

// checkAFLInitialized checks if AFL++ has initialized (created output dirs or SHM)
func (e *Engine) checkAFLInitialized() bool {
	// Check if fuzzer_stats file exists (AFL++ creates this early)
	// AFL++ creates instance directories with fuzzer_stats
	var statsFile string
	if e.config.AFLPlusPlusOptions.MainNode {
		statsFile = filepath.Join(e.outputDir, "main", "fuzzer_stats")
	} else if e.config.AFLPlusPlusOptions.SecondaryNode {
		statsFile = filepath.Join(e.outputDir, "secondary", "fuzzer_stats")
	} else {
		// Default instance when no -M or -S is specified
		statsFile = filepath.Join(e.outputDir, "default", "fuzzer_stats")
	}

	if _, err := os.Stat(statsFile); err == nil {
		return true
	}

	// Check if queue directory exists
	queueDir := filepath.Join(e.outputDir, "queue")
	if e.config.AFLPlusPlusOptions.MainNode {
		queueDir = filepath.Join(e.outputDir, "main", "queue")
	} else if e.config.AFLPlusPlusOptions.SecondaryNode {
		queueDir = filepath.Join(e.outputDir, "secondary", "queue")
	}

	if _, err := os.Stat(queueDir); err == nil {
		return true
	}

	// Check for shared memory segments (Linux-specific)
	if data, err := os.ReadFile("/proc/sysvipc/shm"); err == nil && len(data) > 100 {
		// If there's substantial SHM data, AFL++ likely created segments
		return true
	}

	return false
}

// detectVersion attempts to detect the AFL++ version
func (e *Engine) detectVersion() string {
	cmd := exec.Command("afl-fuzz", "-h")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "unknown"
	}

	// Look for version string in output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "afl-fuzz++") || strings.Contains(line, "AFL++") {
			// Extract version from line like "afl-fuzz++ 4.05c"
			parts := strings.Fields(line)
			for i, part := range parts {
				if strings.Contains(strings.ToLower(part), "afl") && i+1 < len(parts) {
					return parts[i+1]
				}
			}
		}
	}

	return "unknown"
}

// configureCoverageEnvironment configures environment variables for coverage collection
func (e *Engine) configureCoverageEnvironment(env *[]string) error {
	if e.config == nil || !e.config.EnableCoverage || e.config.AFLPlusPlusOptions == nil {
		return nil
	}

	opts := e.config.AFLPlusPlusOptions

	// Configure LLVM coverage if in LLVM mode
	if e.isLLVMMode() {
		profileFile := filepath.Join(e.config.CoverageDir, "afl-llvm.profraw")
		*env = append(*env, fmt.Sprintf("LLVM_PROFILE_FILE=%s", profileFile))
		e.log.WithField("profile_file", profileFile).Debug("Configured LLVM coverage environment")
	}

	// Configure AFL-specific coverage environment
	if e.shouldUseAFLCov() {
		*env = append(*env, "AFL_ENABLE_COVERAGE=1")
		if opts.SourceDir != "" {
			*env = append(*env, fmt.Sprintf("AFL_SOURCE_DIR=%s", opts.SourceDir))
		}
		e.log.Debug("Configured AFL coverage environment")
	}

	return nil
}

// isLLVMMode checks if AFL++ is running in LLVM mode
func (e *Engine) isLLVMMode() bool {
	if e.config == nil || e.config.AFLPlusPlusOptions == nil {
		return false
	}
	return e.config.AFLPlusPlusOptions.LLVMMode
}

// shouldUseAFLCov checks if AFL coverage should be used
func (e *Engine) shouldUseAFLCov() bool {
	if e.config == nil || e.config.AFLPlusPlusOptions == nil {
		return false
	}
	return e.config.AFLPlusPlusOptions.UseAFLCov
}

// CollectCoverageData collects coverage data from AFL++
func (e *Engine) CollectCoverageData() (map[string]interface{}, error) {
	if e.config == nil || !e.config.EnableCoverage {
		return nil, errors.New("coverage collection not enabled")
	}

	coverageData := make(map[string]interface{})

	if e.isLLVMMode() {
		if data, err := e.collectLLVMCoverageData(); err == nil {
			for k, v := range data {
				coverageData[k] = v
			}
		} else {
			e.log.WithError(err).Warn("Failed to collect LLVM coverage data")
		}
	}

	if e.shouldUseAFLCov() {
		if data, err := e.collectAFLCovData(); err == nil {
			for k, v := range data {
				coverageData[k] = v
			}
		} else {
			e.log.WithError(err).Warn("Failed to collect AFL coverage data")
		}
	}

	// Always try to collect basic AFL++ coverage statistics
	if data, err := e.collectBasicCoverageStats(); err == nil {
		for k, v := range data {
			coverageData[k] = v
		}
	}

	coverageData["timestamp"] = time.Now().Unix()
	coverageData["collected_at"] = time.Now().Format(time.RFC3339)
	coverageData["mode"] = e.getCoverageMode()

	e.log.WithField("mode", e.getCoverageMode()).Debug("Coverage data collected successfully")
	return coverageData, nil
}

// getCoverageMode returns the current coverage mode
func (e *Engine) getCoverageMode() string {
	modes := []string{}
	if e.isLLVMMode() {
		modes = append(modes, "llvm")
	}
	if e.shouldUseAFLCov() {
		modes = append(modes, "afl-cov")
	}
	if len(modes) == 0 {
		return "basic"
	}
	return strings.Join(modes, "+")
}

// collectLLVMCoverageData collects LLVM-based coverage data
func (e *Engine) collectLLVMCoverageData() (map[string]interface{}, error) {
	coverageData := make(map[string]interface{})
	profileFile := filepath.Join(e.config.CoverageDir, "afl-llvm.profraw")

	// Check if profile file exists
	if _, err := os.Stat(profileFile); os.IsNotExist(err) {
		e.log.Debug("LLVM coverage profile file not found")
		return coverageData, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Merge profile data using llvm-profdata
	profdataFile := filepath.Join(e.config.CoverageDir, "afl-merged.profdata")
	profdataBinary := "llvm-profdata"
	if e.config.AFLPlusPlusOptions.LLVMProfData != "" {
		profdataBinary = e.config.AFLPlusPlusOptions.LLVMProfData
	}

	profdataCmd := exec.CommandContext(ctx, profdataBinary, "merge", "-sparse", profileFile, "-o", profdataFile)
	if output, err := profdataCmd.CombinedOutput(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("llvm-profdata merge timed out after 30 seconds")
		}
		return nil, fmt.Errorf("llvm-profdata merge failed: %w, output: %s", err, string(output))
	}

	coverageData["llvm_profdata_file"] = profdataFile
	coverageData["llvm_profile_file"] = profileFile

	// Generate coverage report based on format
	switch e.config.CoverageFormat {
	case "json":
		if err := e.generateLLVMJSONCoverage(ctx, profdataFile, coverageData); err != nil {
			return nil, fmt.Errorf("failed to generate LLVM JSON coverage: %w", err)
		}
	case "lcov":
		if err := e.generateLLVMLCOVCoverage(ctx, profdataFile, coverageData); err != nil {
			return nil, fmt.Errorf("failed to generate LLVM LCOV coverage: %w", err)
		}
	case "html":
		if err := e.generateLLVMHTMLCoverage(ctx, profdataFile, coverageData); err != nil {
			return nil, fmt.Errorf("failed to generate LLVM HTML coverage: %w", err)
		}
	case "profdata":
		// Already have profdata file
		coverageData["llvm_format"] = "profdata"
	default:
		coverageData["llvm_format"] = "profdata" // Fallback
	}

	return coverageData, nil
}

// collectAFLCovData collects AFL-specific coverage data using afl-cov
func (e *Engine) collectAFLCovData() (map[string]interface{}, error) {
	coverageData := make(map[string]interface{})

	if e.config.AFLPlusPlusOptions.SourceDir == "" {
		return nil, errors.New("source directory required for AFL coverage analysis")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Use afl-cov to generate coverage analysis
	covDir := filepath.Join(e.config.CoverageDir, "afl-cov")
	if err := os.MkdirAll(covDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create afl-cov directory: %w", err)
	}

	// Try afl-cov-fast first, fallback to afl-cov
	aflCovBinary := "afl-cov-fast"
	if _, err := exec.LookPath(aflCovBinary); err != nil {
		aflCovBinary = "afl-cov"
	}

	args := []string{
		"-d", e.outputDir,
		"-e", e.target,
		"-c", covDir,
		"--source-dir", e.config.AFLPlusPlusOptions.SourceDir,
		"--coverage-at-exit",
	}

	if e.config.CoverageFormat == "lcov" {
		args = append(args, "--lcov-web-all")
	} else if e.config.CoverageFormat == "html" {
		args = append(args, "--func-search", "--line-search")
	}

	cmd := exec.CommandContext(ctx, aflCovBinary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("afl-cov analysis timed out after 60 seconds")
		}
		return nil, fmt.Errorf("afl-cov analysis failed: %w, output: %s", err, string(output))
	}

	coverageData["afl_cov_dir"] = covDir
	coverageData["afl_cov_output"] = string(output)
	coverageData["afl_cov_binary"] = aflCovBinary

	// Check for generated files
	if e.config.CoverageFormat == "lcov" {
		lcovFile := filepath.Join(covDir, "coverage.info")
		if _, err := os.Stat(lcovFile); err == nil {
			coverageData["afl_lcov_file"] = lcovFile
		}
	}

	if e.config.CoverageFormat == "html" {
		htmlIndex := filepath.Join(covDir, "web", "index.html")
		if _, err := os.Stat(htmlIndex); err == nil {
			coverageData["afl_html_index"] = htmlIndex
			coverageData["afl_html_dir"] = filepath.Join(covDir, "web")
		}
	}

	return coverageData, nil
}

// extractAFLShowmapCoverage uses afl-showmap to extract coverage from queue files
func (e *Engine) extractAFLShowmapCoverage(coverageData map[string]interface{}) error {
	// Find queue directory
	queueDir := filepath.Join(e.outputDir, "queue")
	if e.config.AFLPlusPlusOptions.MainNode {
		queueDir = filepath.Join(e.outputDir, "main", "queue")
	} else if e.config.AFLPlusPlusOptions.SecondaryNode {
		queueDir = filepath.Join(e.outputDir, "secondary", "queue")
	}

	// Check if queue directory exists
	if _, err := os.Stat(queueDir); os.IsNotExist(err) {
		return fmt.Errorf("queue directory not found: %s", queueDir)
	}

	// Get all queue files
	queueFiles, err := filepath.Glob(filepath.Join(queueDir, "id:*"))
	if err != nil || len(queueFiles) == 0 {
		return fmt.Errorf("no queue files found in %s", queueDir)
	}

	// Use afl-showmap to extract coverage from the latest queue file
	latestQueue := queueFiles[len(queueFiles)-1]
	coverageMapFile := filepath.Join(e.config.CoverageDir, "afl_coverage_map.txt")

	// Create coverage directory if it doesn't exist
	if err := os.MkdirAll(e.config.CoverageDir, 0755); err != nil {
		return fmt.Errorf("failed to create coverage directory: %w", err)
	}

	// Run afl-showmap
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "afl-showmap", "-o", coverageMapFile, "-e", "--", e.target)

	// Provide input from queue file
	inputData, err := os.ReadFile(latestQueue)
	if err != nil {
		return fmt.Errorf("failed to read queue file: %w", err)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	go func() {
		defer stdin.Close()
		stdin.Write(inputData)
	}()

	output, err := cmd.CombinedOutput()
	if err != nil {
		e.log.WithError(err).WithField("output", string(output)).Debug("afl-showmap failed")
		// Don't return error, just log it
	}

	// Parse coverage map file
	if mapData, err := os.ReadFile(coverageMapFile); err == nil {
		lines := strings.Split(string(mapData), "\n")
		uniqueTuples := len(lines) - 1 // Subtract one for trailing newline
		if uniqueTuples > 0 {
			coverageData["afl_showmap_tuples"] = uniqueTuples
			coverageData["afl_showmap_file"] = coverageMapFile

			// Estimate coverage percentage based on typical map size
			// AFL++ typically uses a 64KB map
			estimatedCoverage := float64(uniqueTuples) / 65536.0 * 100.0
			if estimatedCoverage > 100 {
				estimatedCoverage = 100
			}
			coverageData["afl_estimated_coverage"] = estimatedCoverage
		}
	}

	return nil
}

// collectBasicCoverageStats collects basic AFL++ coverage statistics
func (e *Engine) collectBasicCoverageStats() (map[string]interface{}, error) {
	coverageData := make(map[string]interface{})

	// Read bitmap coverage from AFL++ if available
	bitmapFile := filepath.Join(e.outputDir, "fuzz_bitmap")
	if e.config.AFLPlusPlusOptions.MainNode {
		bitmapFile = filepath.Join(e.outputDir, "main", "fuzz_bitmap")
	} else if e.config.AFLPlusPlusOptions.SecondaryNode {
		bitmapFile = filepath.Join(e.outputDir, "secondary", "fuzz_bitmap")
	}

	if bitmapData, err := os.ReadFile(bitmapFile); err == nil {
		// Calculate bitmap coverage statistics
		totalBits := len(bitmapData) * 8
		setBits := 0
		for _, b := range bitmapData {
			for i := 0; i < 8; i++ {
				if (b>>i)&1 == 1 {
					setBits++
				}
			}
		}

		coverageData["bitmap_file"] = bitmapFile
		coverageData["bitmap_size"] = len(bitmapData)
		coverageData["total_bits"] = totalBits
		coverageData["set_bits"] = setBits
		if totalBits > 0 {
			coverageData["bitmap_density"] = float64(setBits) / float64(totalBits) * 100
		}
	}

	// Extract coverage using afl-showmap from queue files
	if err := e.extractAFLShowmapCoverage(coverageData); err != nil {
		e.log.WithError(err).Debug("Failed to extract afl-showmap coverage")
	}

	// Include current fuzzer statistics
	coverageData["paths_total"] = e.lastPaths
	coverageData["pending_paths"] = e.lastPendingPaths
	coverageData["pending_favs"] = e.lastPendingFavs

	// Calculate approximate line coverage based on AFL++ metrics
	if e.lastPaths > 0 {
		// Estimate line coverage from path coverage
		pathCoverage := float64(e.lastPaths-e.lastPendingPaths) / float64(e.lastPaths) * 100
		coverageData["line_coverage"] = pathCoverage
		coverageData["function_coverage"] = pathCoverage * 0.8 // Conservative estimate
		coverageData["coverage_percent"] = pathCoverage
	}

	return coverageData, nil
}

// Helper methods for LLVM coverage generation
func (e *Engine) generateLLVMJSONCoverage(ctx context.Context, profdataFile string, coverageData map[string]interface{}) error {
	jsonFile := filepath.Join(e.config.CoverageDir, "afl-llvm-coverage.json")
	llvmCovBinary := "llvm-cov"
	if e.config.AFLPlusPlusOptions.LLVMCovBinary != "" {
		llvmCovBinary = e.config.AFLPlusPlusOptions.LLVMCovBinary
	}

	cmd := exec.CommandContext(ctx, llvmCovBinary, "export", e.target, "-instr-profile", profdataFile, "-format=text")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("llvm-cov export timed out after 30 seconds")
		}
		return fmt.Errorf("llvm-cov export failed: %w, output: %s", err, string(output))
	}

	if err := os.WriteFile(jsonFile, output, 0644); err != nil {
		return fmt.Errorf("failed to write JSON coverage file: %w", err)
	}

	// Parse JSON for summary
	var coverageReport map[string]interface{}
	if err := json.Unmarshal(output, &coverageReport); err == nil {
		if data, ok := coverageReport["data"]; ok {
			coverageData["llvm_json_data"] = data
		}
	}

	coverageData["llvm_json_file"] = jsonFile
	coverageData["llvm_format"] = "json"
	return nil
}

func (e *Engine) generateLLVMLCOVCoverage(ctx context.Context, profdataFile string, coverageData map[string]interface{}) error {
	lcovFile := filepath.Join(e.config.CoverageDir, "afl-llvm-coverage.info")
	llvmCovBinary := "llvm-cov"
	if e.config.AFLPlusPlusOptions.LLVMCovBinary != "" {
		llvmCovBinary = e.config.AFLPlusPlusOptions.LLVMCovBinary
	}

	cmd := exec.CommandContext(ctx, llvmCovBinary, "export", e.target, "-instr-profile", profdataFile, "-format=lcov")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("llvm-cov export timed out after 30 seconds")
		}
		return fmt.Errorf("llvm-cov export failed: %w, output: %s", err, string(output))
	}

	if err := os.WriteFile(lcovFile, output, 0644); err != nil {
		return fmt.Errorf("failed to write LCOV coverage file: %w", err)
	}

	coverageData["llvm_lcov_file"] = lcovFile
	coverageData["llvm_format"] = "lcov"
	return nil
}

func (e *Engine) generateLLVMHTMLCoverage(ctx context.Context, profdataFile string, coverageData map[string]interface{}) error {
	htmlDir := filepath.Join(e.config.CoverageDir, "afl-llvm-html")
	if err := os.MkdirAll(htmlDir, 0755); err != nil {
		return fmt.Errorf("failed to create HTML directory: %w", err)
	}

	llvmCovBinary := "llvm-cov"
	if e.config.AFLPlusPlusOptions.LLVMCovBinary != "" {
		llvmCovBinary = e.config.AFLPlusPlusOptions.LLVMCovBinary
	}

	cmd := exec.CommandContext(ctx, llvmCovBinary, "show", e.target, "-instr-profile", profdataFile, "-format=html", "-output-dir", htmlDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("llvm-cov show timed out after 30 seconds")
		}
		return fmt.Errorf("llvm-cov show failed: %w, output: %s", err, string(output))
	}

	coverageData["llvm_html_dir"] = htmlDir
	coverageData["llvm_html_index"] = filepath.Join(htmlDir, "index.html")
	coverageData["llvm_format"] = "html"
	return nil
}

// monitorCoverage monitors and periodically collects coverage data
func (e *Engine) monitorCoverage() {
	defer e.wg.Done()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			// Final coverage collection before exit
			if data, err := e.CollectCoverageData(); err == nil {
				e.coverageMutex.Lock()
				e.coverageData = data
				e.coverageMutex.Unlock()
			}
			return
		case <-ticker.C:
			if !e.isRunning.Load() {
				return
			}
			if data, err := e.CollectCoverageData(); err == nil {
				e.coverageMutex.Lock()
				e.coverageData = data
				e.coverageMutex.Unlock()
			} else {
				e.log.WithError(err).Debug("Failed to collect coverage data")
			}
		}
	}
}

// GetCoverageData returns the current coverage data
func (e *Engine) GetCoverageData() map[string]interface{} {
	e.coverageMutex.RLock()
	defer e.coverageMutex.RUnlock()

	// Return a copy of the coverage data
	data := make(map[string]interface{})
	for k, v := range e.coverageData {
		data[k] = v
	}
	return data
}
