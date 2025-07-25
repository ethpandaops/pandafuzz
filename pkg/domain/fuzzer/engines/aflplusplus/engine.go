package aflplusplus

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
	crashes      map[string]*types.CrashInfo
	crashesMutex sync.RWMutex
	seenCrashes  map[string]bool

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
		log: log.WithField("engine", "afl++"),
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

	// Set environment
	env := os.Environ()
	if e.config.Environment != nil {
		for k, v := range e.config.Environment {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	// AFL++ specific environment variables
	env = append(env, "AFL_SKIP_CPUFREQ=1")
	env = append(env, "AFL_I_DONT_CARE_ABOUT_MISSING_CRASHES=1")
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

	// Start monitoring goroutines
	e.wg.Add(2)
	go e.monitorStats()
	go e.monitorCrashes()

	// Start process monitor
	e.wg.Add(1)
	go e.monitorProcess()

	e.log.Info("AFL++ started successfully")
	return nil
}

// Stop gracefully stops the fuzzing process
func (e *Engine) Stop() error {
	if !e.isRunning.Load() {
		return errors.New("fuzzer is not running")
	}

	e.log.Info("Stopping AFL++...")

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
			e.log.Warn("Force killing AFL++ process")
			e.cmd.Process.Kill()
		}
	}

	e.isRunning.Store(false)

	// Close channels
	close(e.crashChan)
	close(e.progressChan)

	e.log.Info("AFL++ stopped")
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

	// Set output directory from config if not already set
	if e.outputDir == "" && config.OutputDir != "" {
		e.outputDir = config.OutputDir
	}

	return nil
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
		args = append(args, "-m", fmt.Sprintf("%d", e.config.MemoryLimit/(1024*1024)))
	} else {
		args = append(args, "-m", "none")
	}

	// Timeout
	if e.config.Timeout > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", int(e.config.Timeout.Milliseconds())))
	}

	// Dictionary
	if e.config.Dictionary != "" {
		args = append(args, "-x", e.config.Dictionary)
	}

	// AFL++ specific options
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

	if opts.DumbMode {
		args = append(args, "-n")
	}

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

// processOutput processes stdout/stderr output from AFL++
func (e *Engine) processOutput(reader io.Reader, source string) {
	defer e.wg.Done()

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		e.log.WithField("source", source).Debug(line)

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
	statsFile := filepath.Join(e.outputDir, "fuzzer_stats")
	if e.config.AFLPlusPlusOptions.MainNode {
		statsFile = filepath.Join(e.outputDir, "main", "fuzzer_stats")
	} else if e.config.AFLPlusPlusOptions.SecondaryNode {
		statsFile = filepath.Join(e.outputDir, "secondary", "fuzzer_stats")
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
	crashDir := filepath.Join(e.outputDir, "crashes")
	if e.config.AFLPlusPlusOptions.MainNode {
		crashDir = filepath.Join(e.outputDir, "main", "crashes")
	} else if e.config.AFLPlusPlusOptions.SecondaryNode {
		crashDir = filepath.Join(e.outputDir, "secondary", "crashes")
	}

	files, err := os.ReadDir(crashDir)
	if err != nil {
		return // Crashes directory might not exist yet
	}

	for _, file := range files {
		if file.IsDir() || strings.HasSuffix(file.Name(), ".txt") {
			continue
		}

		// Check if we've seen this crash before
		if e.seenCrashes[file.Name()] {
			continue
		}
		e.seenCrashes[file.Name()] = true

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
