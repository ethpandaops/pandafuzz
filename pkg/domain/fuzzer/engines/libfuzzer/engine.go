package libfuzzer

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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
)

// Verify interface compliance
var _ types.Fuzzer = (*Engine)(nil)

// Engine implements the Fuzzer interface for LibFuzzer
type Engine struct {
	// Configuration
	target    string
	args      []string
	config    *types.FuzzerConfig
	corpusDir string
	outputDir string

	// Runtime state
	isRunning atomic.Bool
	version   string

	// Process management
	cmd           *exec.Cmd
	ctx           context.Context
	cancelFunc    context.CancelFunc
	stdin         io.WriteCloser
	stdout        io.ReadCloser
	stderr        io.ReadCloser
	stdoutScanner *bufio.Scanner
	stderrScanner *bufio.Scanner

	// Channels
	crashChan    chan *types.CrashInfo
	progressChan chan *types.ProgressUpdate

	// Statistics tracking
	stats      *types.FuzzerStats
	statsMutex sync.RWMutex

	// Crash tracking
	crashes      map[string]*types.CrashInfo
	crashesMutex sync.RWMutex

	// Parsing state
	lastExecCount  uint64
	lastCorpusSize uint64
	lastCoverage   float64
	lastExecPerSec uint64
	crashCount     uint64

	// Regular expressions for parsing output
	execRegex      *regexp.Regexp
	covRegex       *regexp.Regexp
	corpusRegex    *regexp.Regexp
	execSpeedRegex *regexp.Regexp
	rssRegex       *regexp.Regexp
	crashRegex     *regexp.Regexp

	// Coverage tracking
	coverageData  map[string]interface{}
	coverageMutex sync.RWMutex

	// Synchronization
	wg sync.WaitGroup

	// Logging
	log logrus.FieldLogger
}

// NewEngine creates a new LibFuzzer engine instance
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
		stats: &types.FuzzerStats{
			StartTime: time.Now(),
		},
		coverageData: make(map[string]interface{}),
		log:          log.WithField("engine", "libfuzzer"),
	}

	// Compile regex patterns
	engine.execRegex = regexp.MustCompile(`#(\d+)`)
	engine.covRegex = regexp.MustCompile(`cov: (\d+)`)
	engine.corpusRegex = regexp.MustCompile(`corp: (\d+)`)
	engine.execSpeedRegex = regexp.MustCompile(`exec/s: (\d+)`)
	engine.rssRegex = regexp.MustCompile(`rss: (\d+)Mb`)
	engine.crashRegex = regexp.MustCompile(`==\d+==ERROR: (.+)`)

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

	// Create directories
	if err := e.ensureDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Build command arguments
	cmdArgs := e.buildCommandArgs()

	// Create command
	e.ctx, e.cancelFunc = context.WithCancel(ctx)
	e.cmd = exec.CommandContext(e.ctx, e.target, cmdArgs...)

	// Set environment
	env := os.Environ()
	if e.config != nil && e.config.Environment != nil {
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
	go e.processStdout()
	go e.processStderr()

	// Start monitoring goroutine
	e.wg.Add(1)
	go e.monitorProcess()

	// Start coverage collection if enabled
	if e.config != nil && e.config.EnableCoverage {
		e.wg.Add(1)
		go e.monitorCoverage()
	}

	e.log.Info("LibFuzzer started successfully")
	return nil
}

// Stop gracefully stops the fuzzing process
func (e *Engine) Stop() error {
	if !e.isRunning.Load() {
		return errors.New("fuzzer is not running")
	}

	e.log.Info("Stopping LibFuzzer...")

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
			e.log.Warn("Force killing LibFuzzer process")
			e.cmd.Process.Kill()
		}
	}

	e.isRunning.Store(false)

	// Close channels
	close(e.crashChan)
	close(e.progressChan)

	e.log.Info("LibFuzzer stopped")
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
	return types.FuzzerTypeLibFuzzer.String()
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
	e.corpusDir = path
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

	// Set target from config if not already set
	if e.target == "" && config.Target != "" {
		e.target = config.Target
	}

	return nil
}

// buildCommandArgs builds the command line arguments for LibFuzzer
func (e *Engine) buildCommandArgs() []string {
	args := []string{}

	// Add corpus directory
	if e.corpusDir != "" {
		args = append(args, e.corpusDir)
	}

	// Add configuration options
	if e.config != nil {
		// Coverage options
		if e.config.EnableCoverage {
			args = append(args, "-use_counters=1")
			args = append(args, "-print_pcs=1")
			if e.config.LibFuzzerOptions != nil && e.config.LibFuzzerOptions.PrintCoveragePCs > 0 {
				args = append(args, "-print_coverage_pcs=1")
			}
		}

		// Common options
		if e.config.MaxLen > 0 {
			args = append(args, fmt.Sprintf("-max_len=%d", e.config.MaxLen))
		}
		if e.config.MemoryLimit > 0 {
			args = append(args, fmt.Sprintf("-rss_limit_mb=%d", e.config.MemoryLimit/(1024*1024)))
		}
		if e.config.Timeout > 0 {
			args = append(args, fmt.Sprintf("-timeout=%d", int(e.config.Timeout.Seconds())))
		}
		if e.config.Dictionary != "" {
			args = append(args, fmt.Sprintf("-dict=%s", e.config.Dictionary))
		}
		if e.config.Workers > 1 {
			args = append(args, fmt.Sprintf("-workers=%d", e.config.Workers))
		}

		// LibFuzzer-specific options
		if e.config.LibFuzzerOptions != nil {
			opts := e.config.LibFuzzerOptions
			if opts.Runs > 0 {
				args = append(args, fmt.Sprintf("-runs=%d", opts.Runs))
			}
			if opts.MaxTotalTime > 0 {
				args = append(args, fmt.Sprintf("-max_total_time=%d", opts.MaxTotalTime))
			}
			if opts.UseValueProfile > 0 {
				args = append(args, "-use_value_profile=1")
			}
			if opts.ShrinkCorpus > 0 {
				args = append(args, "-shrink=1")
			}
			if opts.PrintFinalStats > 0 {
				args = append(args, "-print_final_stats=1")
			}
		}

		// Add extra args
		args = append(args, e.config.ExtraArgs...)
	}

	// Add output directory
	if e.outputDir != "" {
		args = append(args, fmt.Sprintf("-artifact_prefix=%s/", e.outputDir))
	}

	// Add original args
	args = append(args, e.args...)

	return args
}

// ensureDirectories creates necessary directories
func (e *Engine) ensureDirectories() error {
	dirs := []string{}

	if e.corpusDir != "" {
		dirs = append(dirs, e.corpusDir)
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

// processStdout processes the stdout output from LibFuzzer
func (e *Engine) processStdout() {
	defer e.wg.Done()

	e.stdoutScanner = bufio.NewScanner(e.stdout)
	for e.stdoutScanner.Scan() {
		line := e.stdoutScanner.Text()
		e.parseLine(line)

		// Write to output file if configured
		if e.config != nil && e.config.OutputWriter != nil {
			timestamp := time.Now().Format(time.RFC3339)
			fmt.Fprintf(e.config.OutputWriter, "%s [stdout] %s\n", timestamp, line)
		}
	}
}

// processStderr processes the stderr output from LibFuzzer
func (e *Engine) processStderr() {
	defer e.wg.Done()

	e.stderrScanner = bufio.NewScanner(e.stderr)
	for e.stderrScanner.Scan() {
		line := e.stderrScanner.Text()
		e.parseLine(line)

		// Write to output file if configured
		if e.config != nil && e.config.OutputWriter != nil {
			timestamp := time.Now().Format(time.RFC3339)
			fmt.Fprintf(e.config.OutputWriter, "%s [stderr] %s\n", timestamp, line)
		}

		// Check for crashes in stderr
		if e.crashRegex.MatchString(line) {
			e.handleCrash(line)
		}
	}
}

// parseLine parses a line of LibFuzzer output
func (e *Engine) parseLine(line string) {
	// Parse execution count
	if matches := e.execRegex.FindStringSubmatch(line); len(matches) > 1 {
		if count, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
			e.lastExecCount = count
		}
	}

	// Parse coverage
	if matches := e.covRegex.FindStringSubmatch(line); len(matches) > 1 {
		if cov, err := strconv.ParseFloat(matches[1], 64); err == nil {
			e.lastCoverage = cov
		}
	}

	// Parse corpus size
	if matches := e.corpusRegex.FindStringSubmatch(line); len(matches) > 1 {
		if size, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
			e.lastCorpusSize = size
		}
	}

	// Parse execution speed
	if matches := e.execSpeedRegex.FindStringSubmatch(line); len(matches) > 1 {
		if speed, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
			e.lastExecPerSec = speed
		}
	}

	// Parse RSS memory
	if matches := e.rssRegex.FindStringSubmatch(line); len(matches) > 1 {
		if rss, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
			e.updateStats(func(stats *types.FuzzerStats) {
				stats.MemoryPeak = rss * 1024 * 1024 // Convert MB to bytes
			})
		}
	}

	// Update stats periodically
	e.updateStatsIfNeeded()
}

// updateStatsIfNeeded updates statistics if enough time has passed
func (e *Engine) updateStatsIfNeeded() {
	e.statsMutex.Lock()
	defer e.statsMutex.Unlock()

	now := time.Now()
	if now.Sub(e.stats.StartTime) < 100*time.Millisecond {
		return
	}

	// Update stats
	e.stats.TotalExecutions = e.lastExecCount
	e.stats.ExecsPerSecond = e.lastExecPerSec
	e.stats.CorpusSize = e.lastCorpusSize
	e.stats.Coverage = e.lastCoverage
	e.stats.RunTime = now.Sub(e.stats.StartTime)
	e.stats.CrashesFound = e.crashCount

	// Send progress update
	select {
	case e.progressChan <- &types.ProgressUpdate{
		Timestamp:      now,
		Executions:     e.lastExecCount,
		ExecsPerSecond: e.lastExecPerSec,
		CorpusSize:     e.lastCorpusSize,
		Coverage:       e.lastCoverage,
		CrashCount:     e.crashCount,
	}:
	default:
		// Channel full, skip update
	}
}

// updateStats safely updates statistics
func (e *Engine) updateStats(fn func(*types.FuzzerStats)) {
	e.statsMutex.Lock()
	defer e.statsMutex.Unlock()
	fn(e.stats)
}

// handleCrash processes a crash detection
func (e *Engine) handleCrash(line string) {
	e.crashCount++

	crashID := e.generateCrashID()
	now := time.Now()

	crash := &types.CrashInfo{
		ID:           crashID,
		StackTrace:   line,
		DiscoveredAt: now,
		FuzzerType:   e.GetType(),
		Metadata: map[string]string{
			"line": line,
		},
	}

	// Try to find the crash file
	if e.outputDir != "" {
		pattern := filepath.Join(e.outputDir, "crash-*")
		matches, _ := filepath.Glob(pattern)
		if len(matches) > 0 {
			// Get the most recent crash file
			latestCrash := matches[len(matches)-1]
			if input, err := os.ReadFile(latestCrash); err == nil {
				crash.Input = input
				crash.Metadata["file"] = latestCrash
			}
		}
	}

	// Store crash
	e.crashesMutex.Lock()
	e.crashes[crashID] = crash
	e.crashesMutex.Unlock()

	// Update stats
	e.updateStats(func(stats *types.FuzzerStats) {
		stats.CrashesFound = e.crashCount
		stats.LastCrashTime = &now
	})

	// Send to channel
	select {
	case e.crashChan <- crash:
	default:
		e.log.Warn("Crash channel full, dropping crash notification")
	}
}

// generateCrashID generates a unique crash ID using UUID
func (e *Engine) generateCrashID() string {
	return uuid.New().String()
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

// detectVersion attempts to detect the LibFuzzer version
func (e *Engine) detectVersion() string {
	if e.target == "" {
		return "unknown"
	}

	// Try running with -help=1 to get version info
	cmd := exec.Command(e.target, "-help=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "unknown"
	}

	// Look for version string in output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "libFuzzer") || strings.Contains(line, "LibFuzzer") {
			// Extract version if present
			parts := strings.Fields(line)
			for i, part := range parts {
				if strings.Contains(strings.ToLower(part), "version") && i+1 < len(parts) {
					return parts[i+1]
				}
			}
			return line // Return the whole line if we can't extract version
		}
	}

	return "unknown"
}

// configureCoverageEnvironment configures environment variables for coverage collection
func (e *Engine) configureCoverageEnvironment(env *[]string) error {
	if e.config == nil || !e.config.EnableCoverage {
		return nil
	}

	// Set LLVM_PROFILE_FILE for LLVM coverage
	profileFile := filepath.Join(e.config.CoverageDir, "libfuzzer.profraw")
	*env = append(*env, fmt.Sprintf("LLVM_PROFILE_FILE=%s", profileFile))

	e.log.WithField("profile_file", profileFile).Debug("Configured LLVM coverage environment")
	return nil
}

// CollectCoverageData collects coverage data using llvm-profdata and llvm-cov
func (e *Engine) CollectCoverageData() (map[string]interface{}, error) {
	if e.config == nil || !e.config.EnableCoverage {
		return nil, errors.New("coverage collection not enabled")
	}

	coverageData := make(map[string]interface{})
	profileFile := filepath.Join(e.config.CoverageDir, "libfuzzer.profraw")

	// Check if profile file exists
	if _, err := os.Stat(profileFile); os.IsNotExist(err) {
		e.log.Warn("Coverage profile file not found")
		return coverageData, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First, merge profile data using llvm-profdata
	profdataFile := filepath.Join(e.config.CoverageDir, "merged.profdata")
	profdataCmd := exec.CommandContext(ctx, "llvm-profdata", "merge", "-sparse", profileFile, "-o", profdataFile)
	if output, err := profdataCmd.CombinedOutput(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("llvm-profdata merge timed out after 30 seconds")
		}
		return nil, fmt.Errorf("llvm-profdata merge failed: %w, output: %s", err, string(output))
	}

	coverageData["profdata_file"] = profdataFile
	coverageData["profile_file"] = profileFile

	// Generate coverage report based on format
	switch e.config.CoverageFormat {
	case "json":
		if err := e.generateJSONCoverage(ctx, profdataFile, coverageData); err != nil {
			return nil, fmt.Errorf("failed to generate JSON coverage: %w", err)
		}
	case "lcov":
		if err := e.generateLCOVCoverage(ctx, profdataFile, coverageData); err != nil {
			return nil, fmt.Errorf("failed to generate LCOV coverage: %w", err)
		}
	case "html":
		if err := e.generateHTMLCoverage(ctx, profdataFile, coverageData); err != nil {
			return nil, fmt.Errorf("failed to generate HTML coverage: %w", err)
		}
	case "profdata":
		// Already have profdata file
		coverageData["format"] = "profdata"
	default:
		e.log.WithField("format", e.config.CoverageFormat).Warn("Unsupported coverage format")
		coverageData["format"] = "profdata" // Fallback to profdata
	}

	coverageData["timestamp"] = time.Now().Unix()
	coverageData["collected_at"] = time.Now().Format(time.RFC3339)

	e.log.WithField("format", e.config.CoverageFormat).Debug("Coverage data collected successfully")
	return coverageData, nil
}

// generateJSONCoverage generates JSON coverage report
func (e *Engine) generateJSONCoverage(ctx context.Context, profdataFile string, coverageData map[string]interface{}) error {
	jsonFile := filepath.Join(e.config.CoverageDir, "coverage.json")
	cmd := exec.CommandContext(ctx, "llvm-cov", "export", e.target, "-instr-profile", profdataFile, "-format=text")
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

	// Parse the JSON to extract summary information
	var coverageReport map[string]interface{}
	if err := json.Unmarshal(output, &coverageReport); err == nil {
		if data, ok := coverageReport["data"]; ok {
			if dataArray, ok := data.([]interface{}); ok && len(dataArray) > 0 {
				if firstFile, ok := dataArray[0].(map[string]interface{}); ok {
					if totals, ok := firstFile["totals"]; ok {
						coverageData["totals"] = totals
					}
				}
			}
		}
	}

	coverageData["json_file"] = jsonFile
	coverageData["format"] = "json"
	return nil
}

// generateLCOVCoverage generates LCOV coverage report
func (e *Engine) generateLCOVCoverage(ctx context.Context, profdataFile string, coverageData map[string]interface{}) error {
	lcovFile := filepath.Join(e.config.CoverageDir, "coverage.info")
	cmd := exec.CommandContext(ctx, "llvm-cov", "export", e.target, "-instr-profile", profdataFile, "-format=lcov")
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

	coverageData["lcov_file"] = lcovFile
	coverageData["format"] = "lcov"
	return nil
}

// generateHTMLCoverage generates HTML coverage report
func (e *Engine) generateHTMLCoverage(ctx context.Context, profdataFile string, coverageData map[string]interface{}) error {
	htmlDir := filepath.Join(e.config.CoverageDir, "html")
	if err := os.MkdirAll(htmlDir, 0755); err != nil {
		return fmt.Errorf("failed to create HTML directory: %w", err)
	}

	cmd := exec.CommandContext(ctx, "llvm-cov", "show", e.target, "-instr-profile", profdataFile, "-format=html", "-output-dir", htmlDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("llvm-cov show timed out after 30 seconds")
		}
		return fmt.Errorf("llvm-cov show failed: %w, output: %s", err, string(output))
	}

	coverageData["html_dir"] = htmlDir
	coverageData["html_index"] = filepath.Join(htmlDir, "index.html")
	coverageData["format"] = "html"
	return nil
}

// monitorCoverage monitors and periodically collects coverage data
func (e *Engine) monitorCoverage() {
	defer e.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
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
