package fuzzer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// LibFuzzer implements the Fuzzer interface for LibFuzzer
type LibFuzzer struct {
	config        FuzzConfig
	status        FuzzerStatus
	logger        *logrus.Logger
	cmd           *exec.Cmd
	ctx           context.Context
	cancel        context.CancelFunc
	eventHandler  EventHandler
	stats         FuzzerStats
	outputDir     string
	crashDir      string
	corpusDir     string
	artifactDir   string
	mu            sync.RWMutex
	wg            sync.WaitGroup
	lastStats     time.Time
	statsRegex    map[string]*regexp.Regexp
	botID         string
}

// NewLibFuzzer creates a new LibFuzzer instance
func NewLibFuzzer() *LibFuzzer {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	
	return &LibFuzzer{
		status:       StatusUninitialized,
		logger:       logger,
		eventHandler: &DefaultEventHandler{},
		stats: FuzzerStats{
			StartTime: time.Now(),
		},
		statsRegex: compileStatsRegexes(),
	}
}

// compileStatsRegexes compiles regular expressions for parsing LibFuzzer output
func compileStatsRegexes() map[string]*regexp.Regexp {
	patterns := map[string]string{
		"executions": `#(\d+)`,
		"coverage":   `cov: (\d+)`,
		"features":   `ft: (\d+)`,
		"corpus":     `corp: (\d+)`,
		"exec_speed": `exec/s: (\d+)`,
		"rss_mb":     `rss: (\d+)Mb`,
	}
	
	regexes := make(map[string]*regexp.Regexp)
	for key, pattern := range patterns {
		regexes[key] = regexp.MustCompile(pattern)
	}
	
	return regexes
}

// Name returns the name of the fuzzer
func (lf *LibFuzzer) Name() string {
	return "LibFuzzer"
}

// Type returns the fuzzer type
func (lf *LibFuzzer) Type() FuzzerType {
	return FuzzerTypeLibFuzzer
}

// Version returns the LibFuzzer version
func (lf *LibFuzzer) Version() string {
	// LibFuzzer version is typically embedded in the target binary
	// Try to extract it by running with -help=1
	if lf.config.Target != "" {
		cmd := exec.Command(lf.config.Target, "-help=1")
		output, err := cmd.CombinedOutput()
		if err == nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.Contains(line, "libFuzzer") && strings.Contains(line, "version") {
					return strings.TrimSpace(line)
				}
			}
		}
	}
	
	return "unknown"
}

// GetCapabilities returns LibFuzzer capabilities
func (lf *LibFuzzer) GetCapabilities() []string {
	return []string{
		"coverage_guided",
		"sanitizers",
		"value_profile",
		"data_flow_trace",
		"fork_mode",
		"merge_mode",
		"minimize_crash",
		"minimize_corpus",
		"custom_mutators",
		"custom_crossover",
		"entropic",
		"len_control",
		"cmp_mutations",
		"dict_mutations",
		"focused_mutations",
	}
}

// Configure sets up the fuzzer configuration
func (lf *LibFuzzer) Configure(config FuzzConfig) error {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	
	if lf.status != StatusUninitialized && lf.status != StatusStopped {
		return &FuzzerError{
			Type:    ErrInvalidConfig,
			Message: "cannot configure fuzzer while running",
			Fuzzer:  lf.Name(),
			Code:    1,
		}
	}
	
	// Validate configuration
	if err := lf.validateConfig(config); err != nil {
		return err
	}
	
	lf.config = config
	
	// Set up directories
	lf.outputDir = filepath.Join(config.OutputDirectory, "libfuzzer_output")
	lf.corpusDir = filepath.Join(lf.outputDir, "corpus")
	lf.crashDir = filepath.Join(lf.outputDir, "crashes")
	lf.artifactDir = filepath.Join(lf.outputDir, "artifacts")
	
	lf.status = StatusInitialized
	
	lf.logger.WithFields(logrus.Fields{
		"target":     config.Target,
		"output_dir": lf.outputDir,
		"duration":   config.Duration,
	}).Info("LibFuzzer configured")
	
	return nil
}

// Initialize prepares LibFuzzer for execution
func (lf *LibFuzzer) Initialize() error {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	
	if lf.status != StatusInitialized {
		return &FuzzerError{
			Type:    ErrInternal,
			Message: "fuzzer must be configured before initialization",
			Fuzzer:  lf.Name(),
			Code:    2,
		}
	}
	
	// Create output directories
	dirs := []string{lf.outputDir, lf.corpusDir, lf.crashDir, lf.artifactDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return &FuzzerError{
				Type:    ErrPermissionDenied,
				Message: fmt.Sprintf("failed to create directory %s: %v", dir, err),
				Fuzzer:  lf.Name(),
				Code:    3,
			}
		}
	}
	
	// Check if target is LibFuzzer-enabled
	if err := lf.checkLibFuzzerBinary(); err != nil {
		return err
	}
	
	// Copy seed corpus if provided
	if lf.config.SeedDirectory != "" {
		if err := lf.copySeedCorpus(); err != nil {
			return err
		}
	}
	
	lf.logger.Info("LibFuzzer initialized")
	
	return nil
}

// Validate checks if the fuzzer is properly configured
func (lf *LibFuzzer) Validate() error {
	lf.mu.RLock()
	defer lf.mu.RUnlock()
	
	// Check target binary exists
	if _, err := os.Stat(lf.config.Target); err != nil {
		return &FuzzerError{
			Type:    ErrTargetNotFound,
			Message: fmt.Sprintf("target binary not found: %s", lf.config.Target),
			Fuzzer:  lf.Name(),
			Code:    4,
		}
	}
	
	// Check if binary is executable
	info, err := os.Stat(lf.config.Target)
	if err != nil {
		return err
	}
	
	if info.Mode()&0111 == 0 {
		return &FuzzerError{
			Type:    ErrPermissionDenied,
			Message: fmt.Sprintf("target binary is not executable: %s", lf.config.Target),
			Fuzzer:  lf.Name(),
			Code:    5,
		}
	}
	
	// Check dictionary if specified
	if lf.config.Dictionary != "" {
		if _, err := os.Stat(lf.config.Dictionary); err != nil {
			return &FuzzerError{
				Type:    ErrInvalidConfig,
				Message: fmt.Sprintf("dictionary file not found: %s", lf.config.Dictionary),
				Fuzzer:  lf.Name(),
				Code:    6,
			}
		}
	}
	
	return nil
}

// Start begins the fuzzing process
func (lf *LibFuzzer) Start(ctx context.Context) error {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	
	if lf.status == StatusRunning || lf.status == StatusStarting {
		return &FuzzerError{
			Type:    ErrInternal,
			Message: "fuzzer is already running",
			Fuzzer:  lf.Name(),
			Code:    7,
		}
	}
	
	if lf.status != StatusInitialized && lf.status != StatusPaused {
		return &FuzzerError{
			Type:    ErrInternal,
			Message: "fuzzer must be initialized before starting",
			Fuzzer:  lf.Name(),
			Code:    8,
		}
	}
	
	lf.status = StatusStarting
	lf.ctx, lf.cancel = context.WithCancel(ctx)
	
	// Build LibFuzzer command
	args := lf.buildLibFuzzerArgs()
	lf.cmd = exec.CommandContext(lf.ctx, lf.config.Target, args...)
	
	// Set up pipes for output
	stdout, err := lf.cmd.StdoutPipe()
	if err != nil {
		lf.status = StatusError
		return &FuzzerError{
			Type:    ErrInternal,
			Message: fmt.Sprintf("failed to create stdout pipe: %v", err),
			Fuzzer:  lf.Name(),
			Code:    9,
		}
	}
	
	stderr, err := lf.cmd.StderrPipe()
	if err != nil {
		lf.status = StatusError
		return &FuzzerError{
			Type:    ErrInternal,
			Message: fmt.Sprintf("failed to create stderr pipe: %v", err),
			Fuzzer:  lf.Name(),
			Code:    10,
		}
	}
	
	// Start LibFuzzer
	if err := lf.cmd.Start(); err != nil {
		lf.status = StatusError
		return &FuzzerError{
			Type:    ErrInternal,
			Message: fmt.Sprintf("failed to start LibFuzzer: %v", err),
			Fuzzer:  lf.Name(),
			Code:    11,
		}
	}
	
	lf.status = StatusRunning
	lf.stats.StartTime = time.Now()
	
	// Start output monitoring
	lf.wg.Add(2)
	go lf.monitorOutput(stdout, "stdout")
	go lf.monitorOutput(stderr, "stderr")
	
	// Notify event handler
	if lf.eventHandler != nil {
		lf.eventHandler.OnStart(lf)
	}
	
	// Monitor process completion
	lf.wg.Add(1)
	go lf.monitorProcess()
	
	lf.logger.WithField("pid", lf.cmd.Process.Pid).Info("LibFuzzer started")
	
	return nil
}

// Stop gracefully stops the fuzzing process
func (lf *LibFuzzer) Stop() error {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	
	if lf.status != StatusRunning && lf.status != StatusPaused {
		return nil
	}
	
	lf.status = StatusStopping
	
	// Cancel context to stop monitoring
	if lf.cancel != nil {
		lf.cancel()
	}
	
	// Send SIGINT to LibFuzzer (graceful shutdown)
	if lf.cmd != nil && lf.cmd.Process != nil {
		if err := lf.cmd.Process.Signal(os.Interrupt); err != nil {
			lf.logger.WithError(err).Warn("Failed to send interrupt signal")
			// Force kill if interrupt fails
			lf.cmd.Process.Kill()
		}
	}
	
	// Wait for goroutines to finish
	lf.wg.Wait()
	
	lf.status = StatusStopped
	
	// Notify event handler
	if lf.eventHandler != nil {
		lf.eventHandler.OnStop(lf, "user requested")
	}
	
	lf.logger.Info("LibFuzzer stopped")
	
	return nil
}

// Pause pauses the fuzzing process
func (lf *LibFuzzer) Pause() error {
	// LibFuzzer doesn't support pause/resume natively
	// We can only stop and restart
	return &FuzzerError{
		Type:    ErrInternal,
		Message: "LibFuzzer does not support pause operation",
		Fuzzer:  lf.Name(),
		Code:    12,
	}
}

// Resume resumes the fuzzing process
func (lf *LibFuzzer) Resume() error {
	// LibFuzzer doesn't support pause/resume natively
	return &FuzzerError{
		Type:    ErrInternal,
		Message: "LibFuzzer does not support resume operation",
		Fuzzer:  lf.Name(),
		Code:    13,
	}
}

// GetStatus returns the current fuzzer status
func (lf *LibFuzzer) GetStatus() FuzzerStatus {
	lf.mu.RLock()
	defer lf.mu.RUnlock()
	
	return lf.status
}

// GetStats returns current fuzzing statistics
func (lf *LibFuzzer) GetStats() FuzzerStats {
	lf.mu.RLock()
	defer lf.mu.RUnlock()
	
	stats := lf.stats
	stats.ElapsedTime = time.Since(stats.StartTime)
	
	return stats
}

// GetProgress returns fuzzing progress information
func (lf *LibFuzzer) GetProgress() FuzzerProgress {
	lf.mu.RLock()
	defer lf.mu.RUnlock()
	
	progress := FuzzerProgress{
		Phase:           lf.getPhase(),
		ProgressPercent: lf.calculateProgress(),
		QueuePosition:   0, // LibFuzzer doesn't expose queue position
		QueueSize:       lf.stats.CorpusSize,
		LastUpdate:      lf.lastStats,
	}
	
	if lf.config.Duration > 0 {
		elapsed := time.Since(lf.stats.StartTime)
		remaining := lf.config.Duration - elapsed
		if remaining > 0 {
			progress.ETA = remaining
		}
	}
	
	return progress
}

// IsRunning returns whether the fuzzer is currently running
func (lf *LibFuzzer) IsRunning() bool {
	lf.mu.RLock()
	defer lf.mu.RUnlock()
	
	return lf.status == StatusRunning
}

// GetResults retrieves all fuzzing results
func (lf *LibFuzzer) GetResults() (*FuzzerResults, error) {
	lf.mu.RLock()
	defer lf.mu.RUnlock()
	
	crashes, err := lf.GetCrashes()
	if err != nil {
		return nil, err
	}
	
	coverage, err := lf.GetCoverage()
	if err != nil {
		return nil, err
	}
	
	corpus, err := lf.GetCorpus()
	if err != nil {
		return nil, err
	}
	
	results := &FuzzerResults{
		Summary: ResultSummary{
			TotalExecutions:  lf.stats.Executions,
			ExecutionTime:    time.Since(lf.stats.StartTime),
			UniqueCrashes:    lf.stats.UniqueCrashes,
			CoverageAchieved: lf.calculateCoveragePercent(),
			NewInputsFound:   lf.stats.NewPaths,
			Success:          lf.stats.UniqueCrashes > 0 || lf.stats.NewPaths > 0,
			ExitReason:       lf.getExitReason(),
		},
		Crashes:  crashes,
		Coverage: coverage,
		Corpus:   corpus,
		Performance: PerformanceMetrics{
			AverageExecSpeed: lf.stats.ExecPerSecond,
			PeakExecSpeed:    lf.stats.ExecPerSecond, // LibFuzzer doesn't track peak separately
			AverageCPU:       lf.stats.CPUUsage,
			PeakMemory:       lf.stats.MemoryUsage * 1024 * 1024, // Convert MB to bytes
			StartupTime:      500 * time.Millisecond, // LibFuzzer starts quickly
		},
		Artifacts: lf.collectArtifacts(),
	}
	
	return results, nil
}

// GetCrashes retrieves crash information
func (lf *LibFuzzer) GetCrashes() ([]*common.CrashResult, error) {
	crashes := make([]*common.CrashResult, 0)
	
	// Read crashes from crash directory
	crashFiles, err := os.ReadDir(lf.crashDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	
	for _, file := range crashFiles {
		if file.IsDir() {
			continue
		}
		
		crashPath := filepath.Join(lf.crashDir, file.Name())
		crashData, err := os.ReadFile(crashPath)
		if err != nil {
			lf.logger.WithError(err).WithField("file", file.Name()).Warn("Failed to read crash file")
			continue
		}
		
		info, _ := file.Info()
		crash := &common.CrashResult{
			ID:        file.Name(),
			JobID:     lf.config.Target,
			BotID:     lf.botID,
			Timestamp: info.ModTime(),
			FilePath:  crashPath,
			Size:      int64(len(crashData)),
			Hash:      lf.hashInput(crashData),
			Type:      lf.detectCrashType(file.Name()),
		}
		
		crashes = append(crashes, crash)
	}
	
	// Also check artifact directory for crashes
	artifactFiles, err := os.ReadDir(lf.artifactDir)
	if err == nil {
		for _, file := range artifactFiles {
			if file.IsDir() || !strings.HasPrefix(file.Name(), "crash-") {
				continue
			}
			
			artifactPath := filepath.Join(lf.artifactDir, file.Name())
			crashData, err := os.ReadFile(artifactPath)
			if err != nil {
				continue
			}
			
			info, _ := file.Info()
			crash := &common.CrashResult{
				ID:        file.Name(),
				JobID:     lf.config.Target,
				BotID:     lf.botID,
				Timestamp: info.ModTime(),
				FilePath:  artifactPath,
				Size:      int64(len(crashData)),
				Hash:      lf.hashInput(crashData),
				Type:      "artifact_crash",
			}
			
			crashes = append(crashes, crash)
		}
	}
	
	return crashes, nil
}

// GetCoverage retrieves coverage information
func (lf *LibFuzzer) GetCoverage() (*common.CoverageResult, error) {
	lf.mu.RLock()
	defer lf.mu.RUnlock()
	
	coverage := &common.CoverageResult{
		ID:        fmt.Sprintf("libfuzzer_%d", time.Now().Unix()),
		JobID:     lf.config.Target,
		BotID:     lf.botID,
		Timestamp: time.Now(),
		Edges:     int(lf.stats.TotalEdges),
		NewEdges:  int(lf.stats.NewPaths),
		ExecCount: lf.stats.Executions,
	}
	
	return coverage, nil
}

// GetCorpus retrieves corpus entries
func (lf *LibFuzzer) GetCorpus() ([]*CorpusEntry, error) {
	corpus := make([]*CorpusEntry, 0)
	
	// Read corpus directory
	corpusFiles, err := os.ReadDir(lf.corpusDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	
	for _, file := range corpusFiles {
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
			Source:    "libfuzzer_corpus",
		}
		
		// Read file to calculate hash
		corpusPath := filepath.Join(lf.corpusDir, file.Name())
		if data, err := os.ReadFile(corpusPath); err == nil {
			entry.Hash = lf.hashInput(data)
		}
		
		corpus = append(corpus, entry)
	}
	
	return corpus, nil
}

// SetEventHandler sets the event handler for fuzzer events
func (lf *LibFuzzer) SetEventHandler(handler EventHandler) {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	
	lf.eventHandler = handler
}

// Cleanup cleans up fuzzer resources
func (lf *LibFuzzer) Cleanup() error {
	// Stop if running
	if lf.IsRunning() {
		if err := lf.Stop(); err != nil {
			return err
		}
	}
	
	// Remove temporary files if configured
	if cleanTemp, ok := lf.config.FuzzerOptions["clean_temp"].(bool); ok && cleanTemp {
		if err := os.RemoveAll(lf.outputDir); err != nil {
			lf.logger.WithError(err).Warn("Failed to clean temporary files")
		}
	}
	
	return nil
}

// Private helper methods

func (lf *LibFuzzer) validateConfig(config FuzzConfig) error {
	if config.Target == "" {
		return &FuzzerError{
			Type:    ErrInvalidConfig,
			Message: "target binary is required",
			Fuzzer:  lf.Name(),
			Code:    14,
		}
	}
	
	if config.OutputDirectory == "" {
		return &FuzzerError{
			Type:    ErrInvalidConfig,
			Message: "output directory is required",
			Fuzzer:  lf.Name(),
			Code:    15,
		}
	}
	
	return nil
}

func (lf *LibFuzzer) checkLibFuzzerBinary() error {
	// Run target with -help=1 to verify it's a LibFuzzer binary
	cmd := exec.Command(lf.config.Target, "-help=1")
	output, err := cmd.CombinedOutput()
	
	if err == nil && strings.Contains(string(output), "libFuzzer") {
		return nil
	}
	
	return &FuzzerError{
		Type:    ErrTargetNotFound,
		Message: "target does not appear to be a LibFuzzer-enabled binary",
		Fuzzer:  lf.Name(),
		Code:    16,
	}
}

func (lf *LibFuzzer) copySeedCorpus() error {
	seedFiles, err := os.ReadDir(lf.config.SeedDirectory)
	if err != nil {
		return err
	}
	
	for _, file := range seedFiles {
		if file.IsDir() {
			continue
		}
		
		src := filepath.Join(lf.config.SeedDirectory, file.Name())
		dst := filepath.Join(lf.corpusDir, file.Name())
		
		input, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		
		if err := os.WriteFile(dst, input, 0644); err != nil {
			return err
		}
	}
	
	return nil
}

func (lf *LibFuzzer) buildLibFuzzerArgs() []string {
	args := []string{}
	
	// Corpus directory
	args = append(args, lf.corpusDir)
	
	// Add seed directory if different from corpus
	if lf.config.SeedDirectory != "" && lf.config.SeedDirectory != lf.corpusDir {
		args = append(args, lf.config.SeedDirectory)
	}
	
	// Max total time
	if lf.config.Duration > 0 {
		seconds := int(lf.config.Duration.Seconds())
		args = append(args, fmt.Sprintf("-max_total_time=%d", seconds))
	}
	
	// Max runs
	if lf.config.MaxExecutions > 0 {
		args = append(args, fmt.Sprintf("-runs=%d", lf.config.MaxExecutions))
	}
	
	// Memory limit
	if lf.config.MemoryLimit > 0 {
		args = append(args, fmt.Sprintf("-rss_limit_mb=%d", lf.config.MemoryLimit))
	}
	
	// Timeout
	if lf.config.Timeout > 0 {
		seconds := int(lf.config.Timeout.Seconds())
		args = append(args, fmt.Sprintf("-timeout=%d", seconds))
	}
	
	// Dictionary
	if lf.config.Dictionary != "" {
		args = append(args, fmt.Sprintf("-dict=%s", lf.config.Dictionary))
	}
	
	// Artifact prefix
	args = append(args, fmt.Sprintf("-artifact_prefix=%s/", lf.artifactDir))
	
	// Print stats
	args = append(args, "-print_stats=1")
	
	// Jobs (workers)
	if workers, ok := lf.config.FuzzerOptions["workers"].(int); ok && workers > 1 {
		args = append(args, fmt.Sprintf("-jobs=%d", workers))
		args = append(args, fmt.Sprintf("-workers=%d", workers))
	}
	
	// Fork mode
	if fork, ok := lf.config.FuzzerOptions["fork"].(bool); ok && fork {
		args = append(args, "-fork=1")
	}
	
	// Value profile
	if valueProfile, ok := lf.config.FuzzerOptions["value_profile"].(bool); ok && valueProfile {
		args = append(args, "-use_value_profile=1")
	}
	
	// Entropic
	if entropic, ok := lf.config.FuzzerOptions["entropic"].(bool); ok && entropic {
		args = append(args, "-entropic=1")
	}
	
	// Additional LibFuzzer options
	if options, ok := lf.config.FuzzerOptions["libfuzzer_args"].([]string); ok {
		args = append(args, options...)
	}
	
	return args
}

func (lf *LibFuzzer) monitorOutput(pipe io.Reader, name string) {
	defer lf.wg.Done()
	
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		
		// Log output
		lf.logger.WithField("stream", name).Debug(line)
		
		// Parse statistics from output
		lf.parseStats(line)
		
		// Check for important events
		if strings.Contains(line, "NEW_PC") || strings.Contains(line, "NEW") {
			lf.handleNewCoverage(line)
		}
		
		if strings.Contains(line, "CRASH") || strings.Contains(line, "ERROR") {
			lf.handleCrash(line)
		}
		
		if strings.Contains(line, "DONE") || strings.Contains(line, "Done") {
			lf.logger.Info("LibFuzzer completed execution")
		}
	}
}

func (lf *LibFuzzer) parseStats(line string) {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	
	// Parse execution count
	if match := lf.statsRegex["executions"].FindStringSubmatch(line); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			lf.stats.Executions = val
		}
	}
	
	// Parse coverage
	if match := lf.statsRegex["coverage"].FindStringSubmatch(line); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			lf.stats.CoveredEdges = int(val)
		}
	}
	
	// Parse features
	if match := lf.statsRegex["features"].FindStringSubmatch(line); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			lf.stats.TotalEdges = int(val)
		}
	}
	
	// Parse corpus size
	if match := lf.statsRegex["corpus"].FindStringSubmatch(line); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			lf.stats.CorpusSize = int(val)
		}
	}
	
	// Parse execution speed
	if match := lf.statsRegex["exec_speed"].FindStringSubmatch(line); len(match) > 1 {
		if val, err := strconv.ParseFloat(match[1], 64); err == nil {
			lf.stats.ExecPerSecond = val
		}
	}
	
	// Parse memory usage
	if match := lf.statsRegex["rss_mb"].FindStringSubmatch(line); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			lf.stats.MemoryUsage = val
		}
	}
	
	lf.stats.ElapsedTime = time.Since(lf.stats.StartTime)
	lf.lastStats = time.Now()
	
	// Notify event handler periodically
	if time.Since(lf.lastStats) > 5*time.Second {
		if lf.eventHandler != nil {
			lf.eventHandler.OnStats(lf, lf.stats)
			lf.eventHandler.OnProgress(lf, lf.GetProgress())
		}
	}
}

func (lf *LibFuzzer) handleNewCoverage(line string) {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	
	lf.stats.NewPaths++
	lf.stats.LastNewPath = time.Now()
	
	if lf.eventHandler != nil {
		// Create a simple corpus entry for the new path
		entry := &CorpusEntry{
			ID:        fmt.Sprintf("new_path_%d", lf.stats.NewPaths),
			Timestamp: time.Now(),
			Source:    "libfuzzer",
		}
		lf.eventHandler.OnNewPath(lf, entry)
	}
}

func (lf *LibFuzzer) handleCrash(line string) {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	
	lf.stats.UniqueCrashes++
	lf.stats.TotalCrashes++
	lf.stats.LastCrash = time.Now()
	
	if lf.eventHandler != nil {
		// Create a simple crash result
		crash := &common.CrashResult{
			ID:        fmt.Sprintf("crash_%d", lf.stats.TotalCrashes),
			JobID:     lf.config.Target,
			Timestamp: time.Now(),
			Type:      "libfuzzer_crash",
		}
		lf.eventHandler.OnCrash(lf, crash)
	}
}

func (lf *LibFuzzer) monitorProcess() {
	defer lf.wg.Done()
	
	// Wait for process to exit
	err := lf.cmd.Wait()
	
	lf.mu.Lock()
	defer lf.mu.Unlock()
	
	if err != nil {
		lf.logger.WithError(err).Warn("LibFuzzer process exited with error")
		lf.status = StatusError
		if lf.eventHandler != nil {
			lf.eventHandler.OnError(lf, err)
		}
	} else {
		lf.status = StatusCompleted
	}
	
	// Notify completion
	if lf.eventHandler != nil {
		reason := "completed"
		if lf.ctx.Err() != nil {
			reason = "cancelled"
		}
		lf.eventHandler.OnStop(lf, reason)
	}
}

func (lf *LibFuzzer) getPhase() string {
	if lf.stats.Executions < 1000 {
		return "initialization"
	} else if lf.stats.Executions < 10000 {
		return "exploration"
	} else {
		return "fuzzing"
	}
}

func (lf *LibFuzzer) calculateProgress() float64 {
	if lf.config.Duration > 0 {
		elapsed := time.Since(lf.stats.StartTime)
		progress := elapsed.Seconds() / lf.config.Duration.Seconds() * 100
		if progress > 100 {
			progress = 100
		}
		return progress
	}
	
	if lf.config.MaxExecutions > 0 {
		progress := float64(lf.stats.Executions) / float64(lf.config.MaxExecutions) * 100
		if progress > 100 {
			progress = 100
		}
		return progress
	}
	
	return 0
}

func (lf *LibFuzzer) calculateCoveragePercent() float64 {
	if lf.stats.TotalEdges > 0 {
		return float64(lf.stats.CoveredEdges) / float64(lf.stats.TotalEdges) * 100
	}
	return 0
}

func (lf *LibFuzzer) getExitReason() string {
	switch lf.status {
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

func (lf *LibFuzzer) hashInput(data []byte) string {
	// Simple hash implementation
	hash := uint32(0)
	for _, b := range data {
		hash = hash*31 + uint32(b)
	}
	return fmt.Sprintf("%08x", hash)
}

func (lf *LibFuzzer) detectCrashType(filename string) string {
	// LibFuzzer typically includes crash type in filename
	if strings.Contains(filename, "leak") {
		return "memory_leak"
	} else if strings.Contains(filename, "oom") {
		return "out_of_memory"
	} else if strings.Contains(filename, "timeout") {
		return "timeout"
	} else if strings.Contains(filename, "crash") {
		return "crash"
	}
	
	return "unknown"
}

func (lf *LibFuzzer) collectArtifacts() []Artifact {
	artifacts := make([]Artifact, 0)
	
	// Collect artifacts from artifact directory
	artifactFiles, err := os.ReadDir(lf.artifactDir)
	if err != nil {
		return artifacts
	}
	
	for _, file := range artifactFiles {
		if file.IsDir() {
			continue
		}
		
		info, err := file.Info()
		if err != nil {
			continue
		}
		
		artifactType := ArtifactCorpus
		if strings.Contains(file.Name(), "crash") {
			artifactType = ArtifactCrash
		} else if strings.Contains(file.Name(), "leak") {
			artifactType = ArtifactCrash
		}
		
		artifact := Artifact{
			Type:        artifactType,
			Name:        file.Name(),
			Path:        filepath.Join(lf.artifactDir, file.Name()),
			Size:        info.Size(),
			Timestamp:   info.ModTime(),
			Description: fmt.Sprintf("LibFuzzer artifact: %s", file.Name()),
		}
		
		artifacts = append(artifacts, artifact)
	}
	
	return artifacts
}

// Factory function for creating LibFuzzer instances
func CreateLibFuzzer() (Fuzzer, error) {
	return NewLibFuzzer(), nil
}