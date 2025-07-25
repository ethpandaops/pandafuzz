package honggfuzz

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
)

// Verify interface compliance
var _ types.Fuzzer = (*Engine)(nil)

// Engine implements the Fuzzer interface for Honggfuzz
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

	// Honggfuzz specific stats
	lastIterations uint64
	lastCrashes    uint64
	lastTimeouts   uint64
	lastSpeed      uint64
	lastCoverage   float64
	lastGuardNb    uint64
	lastCorpusSize uint64

	// Regular expressions for parsing output
	iterRegex   *regexp.Regexp
	speedRegex  *regexp.Regexp
	crashRegex  *regexp.Regexp
	covRegex    *regexp.Regexp
	corpusRegex *regexp.Regexp
	guardRegex  *regexp.Regexp

	// Crash tracking
	crashes      map[string]*types.CrashInfo
	crashesMutex sync.RWMutex
	seenCrashes  map[string]bool

	// Synchronization
	wg            sync.WaitGroup
	lastStatsTime time.Time

	// Logging
	log logrus.FieldLogger
}

// NewEngine creates a new Honggfuzz engine instance
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
		log: log.WithField("engine", "honggfuzz"),
	}

	// Compile regex patterns for parsing output
	engine.iterRegex = regexp.MustCompile(`Iterations: (\d+)`)
	engine.speedRegex = regexp.MustCompile(`Speed: (\d+)/sec`)
	engine.crashRegex = regexp.MustCompile(`Crashes: (\d+)`)
	engine.covRegex = regexp.MustCompile(`Coverage: ([\d.]+)%`)
	engine.corpusRegex = regexp.MustCompile(`Corpus: (\d+)`)
	engine.guardRegex = regexp.MustCompile(`Guard nb: (\d+)`)

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

	// Ensure we have Honggfuzz options
	if e.config == nil || e.config.HonggfuzzOptions == nil {
		return errors.New("Honggfuzz configuration is required")
	}

	// Set input directory
	e.inputDir = e.config.HonggfuzzOptions.InputDir
	if e.inputDir == "" {
		return errors.New("input directory is required for Honggfuzz")
	}

	// Create directories
	if err := e.ensureDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Build command arguments
	cmdArgs := e.buildCommandArgs()

	// Find honggfuzz binary
	hfBinary := "honggfuzz"
	if path, err := exec.LookPath("honggfuzz"); err == nil {
		hfBinary = path
	}

	// Create command
	e.ctx, e.cancelFunc = context.WithCancel(ctx)
	e.cmd = exec.CommandContext(e.ctx, hfBinary, cmdArgs...)

	// Set environment
	env := os.Environ()
	if e.config.Environment != nil {
		for k, v := range e.config.Environment {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Add Honggfuzz specific environment variables
	if e.config.HonggfuzzOptions.EnvVars != nil {
		env = append(env, e.config.HonggfuzzOptions.EnvVars...)
	}
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

	// Start output processors
	e.wg.Add(2)
	go e.processOutput(e.stdout, "stdout")
	go e.processOutput(e.stderr, "stderr")

	// Start crash monitor
	e.wg.Add(1)
	go e.monitorCrashes()

	// Start process monitor
	e.wg.Add(1)
	go e.monitorProcess()

	e.log.Info("Honggfuzz started successfully")
	return nil
}

// Stop gracefully stops the fuzzing process
func (e *Engine) Stop() error {
	if !e.isRunning.Load() {
		return errors.New("fuzzer is not running")
	}

	e.log.Info("Stopping Honggfuzz...")

	// Cancel context to signal shutdown
	if e.cancelFunc != nil {
		e.cancelFunc()
	}

	// Give process time to exit gracefully
	done := make(chan bool)
	go func() {
		e.wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// Process exited gracefully
	case <-time.After(10 * time.Second):
		// Force kill if not exited
		if e.cmd != nil && e.cmd.Process != nil {
			e.log.Warn("Force killing Honggfuzz process")
			e.cmd.Process.Kill()
		}
	}

	e.isRunning.Store(false)

	// Close channels
	close(e.crashChan)
	close(e.progressChan)

	e.log.Info("Honggfuzz stopped")
	return nil
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
	return types.FuzzerTypeHonggfuzz.String()
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
	if e.config != nil && e.config.HonggfuzzOptions != nil {
		e.config.HonggfuzzOptions.InputDir = path
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

	// Set output directory from config if not already set
	if e.outputDir == "" && config.OutputDir != "" {
		e.outputDir = config.OutputDir
	}

	return nil
}

// buildCommandArgs builds the command line arguments for Honggfuzz
func (e *Engine) buildCommandArgs() []string {
	args := []string{}
	opts := e.config.HonggfuzzOptions

	// Input directory (required)
	args = append(args, "-i", e.inputDir)

	// Output directory
	outputDir := e.outputDir
	if outputDir == "" && e.config != nil {
		outputDir = e.config.OutputDir
	}
	if outputDir == "" {
		outputDir = "/tmp/honggfuzz"
	}
	args = append(args, "-W", outputDir) // Workspace directory

	// Number of threads
	if opts.Threads > 0 {
		args = append(args, "-n", strconv.Itoa(opts.Threads))
	} else if e.config.Workers > 0 {
		args = append(args, "-n", strconv.Itoa(e.config.Workers))
	}

	// Iterations
	if opts.Iterations > 0 {
		args = append(args, "-N", strconv.FormatUint(opts.Iterations, 10))
	}

	// Run time
	if opts.RunTime > 0 {
		args = append(args, "-T", strconv.Itoa(opts.RunTime))
	}

	// Timeout
	if e.config.Timeout > 0 {
		args = append(args, "-t", strconv.Itoa(int(e.config.Timeout.Seconds())))
	}

	// Memory limit
	if e.config.MemoryLimit > 0 {
		args = append(args, "--rlimit_rss", strconv.FormatUint(e.config.MemoryLimit/(1024*1024), 10))
	}

	// Dictionary
	if e.config.Dictionary != "" {
		args = append(args, "-w", e.config.Dictionary)
	}

	// Max file size
	if opts.MaxFileSize > 0 {
		args = append(args, "-F", strconv.Itoa(opts.MaxFileSize))
	} else if e.config.MaxLen > 0 {
		args = append(args, "-F", strconv.Itoa(e.config.MaxLen))
	}

	// Extension filter
	if opts.ExtensionFilter != "" {
		args = append(args, "-e", opts.ExtensionFilter)
	}

	// Verbosity options
	if opts.Verbose {
		args = append(args, "-v")
	}
	if opts.Debug {
		args = append(args, "-d")
	}
	if opts.QuietMode {
		args = append(args, "-q")
	}

	// Saving options
	if opts.SaveAll {
		args = append(args, "-u")
	}
	if opts.SaveSmaller {
		args = append(args, "-s")
	}

	// Other options
	if opts.KeepOutput {
		args = append(args, "-Q")
	}
	if opts.NoFeedback {
		args = append(args, "--linux_perf_bts_edge")
	}
	if opts.ClearWorkspace {
		args = append(args, "--clear_env")
	}
	if opts.ExitUponCrash {
		args = append(args, "--exit_upon_crash")
	}
	if opts.NoMinify {
		args = append(args, "--no_minimize")
	}

	// Report file
	if opts.ReportFile != "" {
		args = append(args, "-r", opts.ReportFile)
	}

	// Socket fuzzing
	if opts.SocketFuzzing {
		args = append(args, "--socket_fuzzer")
	}

	// Network options
	if opts.NetDriver {
		args = append(args, "--netdriver")
	}
	if opts.NetBindTo != "" {
		args = append(args, "--bind_to", opts.NetBindTo)
	}
	if opts.NetConnectTo != "" {
		args = append(args, "--connect_to", opts.NetConnectTo)
	}

	// Resource limits
	if opts.AsLimit > 0 {
		args = append(args, "--rlimit_as", strconv.Itoa(opts.AsLimit))
	}
	if opts.RssLimit > 0 {
		args = append(args, "--rlimit_rss", strconv.Itoa(opts.RssLimit))
	}
	if opts.DataLimit > 0 {
		args = append(args, "--rlimit_data", strconv.Itoa(opts.DataLimit))
	}
	if opts.StackLimit > 0 {
		args = append(args, "--rlimit_stack", strconv.Itoa(opts.StackLimit))
	}

	// Add extra args
	if e.config.ExtraArgs != nil {
		args = append(args, e.config.ExtraArgs...)
	}

	// Add separator
	args = append(args, "--")

	// Add target and target args
	args = append(args, e.target)
	args = append(args, e.args...)

	return args
}

// ensureDirectories creates necessary directories
func (e *Engine) ensureDirectories() error {
	dirs := []string{}

	if e.inputDir != "" {
		// Check if input dir exists
		if _, err := os.Stat(e.inputDir); os.IsNotExist(err) {
			return fmt.Errorf("input directory does not exist: %s", e.inputDir)
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

// processOutput processes stdout/stderr output from Honggfuzz
func (e *Engine) processOutput(reader io.Reader, source string) {
	defer e.wg.Done()

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		e.log.WithField("source", source).Debug(line)

		// Parse statistics from output
		e.parseStats(line)

		// Check for crash notifications
		if strings.Contains(line, "Crash") || strings.Contains(line, "CRASH") {
			e.log.Warn("Crash detected in output")
		}
	}
}

// parseStats parses statistics from Honggfuzz output
func (e *Engine) parseStats(line string) {
	now := time.Now()
	updated := false

	// Parse iterations
	if matches := e.iterRegex.FindStringSubmatch(line); len(matches) > 1 {
		if iter, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
			e.lastIterations = iter
			updated = true
		}
	}

	// Parse speed
	if matches := e.speedRegex.FindStringSubmatch(line); len(matches) > 1 {
		if speed, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
			e.lastSpeed = speed
			updated = true
		}
	}

	// Parse crashes
	if matches := e.crashRegex.FindStringSubmatch(line); len(matches) > 1 {
		if crashes, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
			e.lastCrashes = crashes
			updated = true
		}
	}

	// Parse coverage
	if matches := e.covRegex.FindStringSubmatch(line); len(matches) > 1 {
		if cov, err := strconv.ParseFloat(matches[1], 64); err == nil {
			e.lastCoverage = cov
			updated = true
		}
	}

	// Parse corpus size
	if matches := e.corpusRegex.FindStringSubmatch(line); len(matches) > 1 {
		if corpus, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
			e.lastCorpusSize = corpus
			updated = true
		}
	}

	// Parse guard number
	if matches := e.guardRegex.FindStringSubmatch(line); len(matches) > 1 {
		if guard, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
			e.lastGuardNb = guard
			updated = true
		}
	}

	if updated {
		// Update internal stats
		e.statsMutex.Lock()
		e.stats.TotalExecutions = e.lastIterations
		e.stats.ExecsPerSecond = e.lastSpeed
		e.stats.CorpusSize = e.lastCorpusSize
		e.stats.Coverage = e.lastCoverage
		e.stats.CrashesFound = e.lastCrashes
		e.stats.RunTime = now.Sub(e.stats.StartTime)
		e.statsMutex.Unlock()

		// Send progress update
		if now.Sub(e.lastStatsTime) >= 100*time.Millisecond {
			e.lastStatsTime = now
			select {
			case e.progressChan <- &types.ProgressUpdate{
				Timestamp:      now,
				Executions:     e.lastIterations,
				ExecsPerSecond: e.lastSpeed,
				CorpusSize:     e.lastCorpusSize,
				Coverage:       e.lastCoverage,
				CrashCount:     e.lastCrashes,
			}:
			default:
				// Channel full, skip update
			}
		}
	}
}

// monitorCrashes monitors for new crashes
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
	crashDir := e.outputDir
	if crashDir == "" && e.config != nil {
		crashDir = e.config.OutputDir
	}
	if crashDir == "" {
		crashDir = "/tmp/honggfuzz"
	}

	// Honggfuzz saves crashes with specific naming pattern
	pattern := filepath.Join(crashDir, "*.fuzz")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return
	}

	for _, file := range files {
		// Check if we've seen this crash before
		if e.seenCrashes[filepath.Base(file)] {
			continue
		}
		e.seenCrashes[filepath.Base(file)] = true

		// Read crash file
		input, err := os.ReadFile(file)
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
				"file": filepath.Base(file),
				"path": file,
			},
		}

		// Try to get additional crash info from .txt file if exists
		txtFile := strings.TrimSuffix(file, ".fuzz") + ".txt"
		if info, err := os.ReadFile(txtFile); err == nil {
			crash.StackTrace = string(info)
		}

		// Store crash
		e.crashesMutex.Lock()
		e.crashes[crashID] = crash
		e.crashesMutex.Unlock()

		// Update stats
		e.statsMutex.Lock()
		e.stats.LastCrashTime = &now
		e.statsMutex.Unlock()

		// Send to channel
		select {
		case e.crashChan <- crash:
		default:
			e.log.Warn("Crash channel full, dropping crash notification")
		}
	}
}

// generateCrashID generates a unique crash ID
func (e *Engine) generateCrashID() string {
	hash := sha256.New()
	hash.Write([]byte(fmt.Sprintf("%s-%d-%d", time.Now().String(), e.lastCrashes, len(e.crashes))))
	return hex.EncodeToString(hash.Sum(nil))[:16]
}

// monitorProcess monitors the fuzzer process
func (e *Engine) monitorProcess() {
	defer e.wg.Done()

	if e.cmd != nil {
		err := e.cmd.Wait()
		if err != nil && !errors.Is(err, context.Canceled) {
			e.log.WithError(err).Error("Fuzzer process exited with error")
		}
	}

	e.isRunning.Store(false)
}

// detectVersion attempts to detect the Honggfuzz version
func (e *Engine) detectVersion() string {
	cmd := exec.Command("honggfuzz", "-h")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "unknown"
	}

	// Look for version string in output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "honggfuzz") && strings.Contains(line, "version") {
			// Extract version from line
			parts := strings.Fields(line)
			for i, part := range parts {
				if strings.Contains(strings.ToLower(part), "version") && i+1 < len(parts) {
					return parts[i+1]
				}
			}
		}
	}

	return "unknown"
}
