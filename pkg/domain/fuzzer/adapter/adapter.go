// Package adapter provides compatibility adapters for the domain fuzzer interface.
// It bridges the channel-based domain fuzzer API with callback-based patterns
// used by the bot package.
package adapter

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
	"github.com/sirupsen/logrus"
)

// EventHandler handles fuzzer events (callback-based interface).
// This mirrors the legacy fuzzer.EventHandler interface for backward compatibility.
type EventHandler interface {
	OnStart(fuzzer Fuzzer)
	OnStop(fuzzer Fuzzer, reason string)
	OnCrash(fuzzer Fuzzer, crash *common.CrashResult)
	OnNewPath(fuzzer Fuzzer, path *CorpusEntry)
	OnStats(fuzzer Fuzzer, stats FuzzerStats)
	OnError(fuzzer Fuzzer, err error)
	OnProgress(fuzzer Fuzzer, progress FuzzerProgress)
}

// DefaultEventHandler provides a default no-op implementation of EventHandler.
type DefaultEventHandler struct{}

func (h *DefaultEventHandler) OnStart(fuzzer Fuzzer)                            {}
func (h *DefaultEventHandler) OnStop(fuzzer Fuzzer, reason string)              {}
func (h *DefaultEventHandler) OnCrash(fuzzer Fuzzer, crash *common.CrashResult) {}
func (h *DefaultEventHandler) OnNewPath(fuzzer Fuzzer, path *CorpusEntry)       {}
func (h *DefaultEventHandler) OnStats(fuzzer Fuzzer, stats FuzzerStats)         {}
func (h *DefaultEventHandler) OnError(fuzzer Fuzzer, err error)                 {}
func (h *DefaultEventHandler) OnProgress(fuzzer Fuzzer, progress FuzzerProgress) {
}

// Fuzzer provides a legacy-compatible interface wrapping the domain fuzzer.
type Fuzzer interface {
	// Identity
	Name() string
	Type() FuzzerType
	Version() string
	GetCapabilities() []string

	// Configuration and initialization
	Configure(config FuzzConfig) error
	Initialize() error

	// Execution
	Start(ctx context.Context) error
	Stop() error
	IsRunning() bool

	// Status
	GetStatus() FuzzerStatus
	GetStats() FuzzerStats

	// Results
	GetResults() (*FuzzerResults, error)
	GetCrashes() ([]*common.CrashResult, error)

	// Events
	SetEventHandler(handler EventHandler)

	// Cleanup
	Cleanup() error

	// Coverage
	CollectCoverageData() (map[string]interface{}, error)

	// Crash reproduction
	ReproduceCrash(ctx context.Context, input []byte, config ReproductionConfig) (*common.ReproductionResult, error)
}

// FuzzerAdapter wraps a domain.Fuzzer to provide legacy-compatible interface.
type FuzzerAdapter struct {
	engine       types.Fuzzer
	config       *FuzzConfig
	handler      EventHandler
	log          logrus.FieldLogger
	botID        string
	crashes      []*common.CrashResult
	crashesMu    sync.RWMutex
	stats        FuzzerStats
	statsMu      sync.RWMutex
	status       FuzzerStatus
	statusMu     sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	initialized  bool
	outputWriter io.Writer
}

// NewAdapter creates a new FuzzerAdapter wrapping a domain fuzzer engine.
func NewAdapter(engine types.Fuzzer, log logrus.FieldLogger) *FuzzerAdapter {
	if log == nil {
		log = logrus.New()
	}
	return &FuzzerAdapter{
		engine:  engine,
		log:     log.WithField("component", "fuzzer-adapter"),
		handler: &DefaultEventHandler{},
		crashes: make([]*common.CrashResult, 0),
		status:  StatusUninitialized,
	}
}

// SetBotID sets the bot ID for crash reporting.
func (a *FuzzerAdapter) SetBotID(botID string) {
	a.botID = botID
}

// Name returns the fuzzer name.
func (a *FuzzerAdapter) Name() string {
	return a.engine.GetType()
}

// Type returns the fuzzer type.
func (a *FuzzerAdapter) Type() FuzzerType {
	switch a.engine.GetType() {
	case "afl++":
		return FuzzerTypeAFL
	case "libfuzzer":
		return FuzzerTypeLibFuzzer
	case "honggfuzz":
		return FuzzerTypeHonggfuzz
	default:
		return FuzzerTypeCustom
	}
}

// Version returns the fuzzer version.
func (a *FuzzerAdapter) Version() string {
	return a.engine.GetVersion()
}

// GetCapabilities returns the fuzzer capabilities.
func (a *FuzzerAdapter) GetCapabilities() []string {
	return []string{"coverage", "crash_detection"}
}

// Configure configures the fuzzer with the given config.
func (a *FuzzerAdapter) Configure(config FuzzConfig) error {
	a.config = &config
	a.outputWriter = config.OutputWriter

	// Convert legacy config to domain config
	domainConfig := a.convertConfig(config)
	return a.engine.Configure(domainConfig)
}

// Initialize initializes the fuzzer (no-op for domain fuzzers).
func (a *FuzzerAdapter) Initialize() error {
	a.initialized = true
	a.setStatus(StatusInitialized)
	return nil
}

// Start starts the fuzzer.
func (a *FuzzerAdapter) Start(ctx context.Context) error {
	if !a.initialized {
		if err := a.Initialize(); err != nil {
			return err
		}
	}

	a.ctx, a.cancel = context.WithCancel(ctx)
	a.setStatus(StatusStarting)

	// Notify handler
	if a.handler != nil {
		a.handler.OnStart(a)
	}

	// Start monitoring goroutines for channels
	a.wg.Add(2)
	go a.monitorCrashes()
	go a.monitorProgress()

	// Start the engine
	if err := a.engine.Start(a.ctx); err != nil {
		a.setStatus(StatusError)
		if a.handler != nil {
			a.handler.OnError(a, err)
		}
		return err
	}

	a.setStatus(StatusRunning)
	return nil
}

// Stop stops the fuzzer.
func (a *FuzzerAdapter) Stop() error {
	a.setStatus(StatusStopping)

	// Cancel context to stop monitoring goroutines
	if a.cancel != nil {
		a.cancel()
	}

	// Stop the engine
	err := a.engine.Stop()

	// Wait for monitoring goroutines
	a.wg.Wait()

	// Notify handler
	if a.handler != nil {
		reason := "stopped"
		if err != nil {
			reason = err.Error()
		}
		a.handler.OnStop(a, reason)
	}

	a.setStatus(StatusStopped)
	return err
}

// IsRunning returns whether the fuzzer is running.
func (a *FuzzerAdapter) IsRunning() bool {
	return a.engine.IsRunning()
}

// GetStatus returns the current fuzzer status.
func (a *FuzzerAdapter) GetStatus() FuzzerStatus {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.status
}

// GetStats returns the current fuzzer statistics.
func (a *FuzzerAdapter) GetStats() FuzzerStats {
	a.statsMu.RLock()
	defer a.statsMu.RUnlock()
	return a.stats
}

// GetResults returns the fuzzer results.
func (a *FuzzerAdapter) GetResults() (*FuzzerResults, error) {
	stats := a.GetStats()
	crashes := a.getCrashes()

	return &FuzzerResults{
		Summary: ResultSummary{
			TotalExecutions:  int64(stats.Executions),
			ExecutionTime:    stats.ElapsedTime,
			UniqueCrashes:    len(crashes),
			CoverageAchieved: stats.CoveragePercent,
			NewInputsFound:   stats.CorpusSize,
			Success:          true,
		},
		Crashes: crashes,
	}, nil
}

// GetCrashes returns all discovered crashes.
func (a *FuzzerAdapter) GetCrashes() ([]*common.CrashResult, error) {
	return a.getCrashes(), nil
}

// SetEventHandler sets the event handler for callbacks.
func (a *FuzzerAdapter) SetEventHandler(handler EventHandler) {
	a.handler = handler
}

// Cleanup cleans up fuzzer resources.
func (a *FuzzerAdapter) Cleanup() error {
	// Domain fuzzers handle cleanup in Stop()
	return nil
}

// Validate validates the fuzzer configuration.
func (a *FuzzerAdapter) Validate() error {
	if a.config == nil {
		return fmt.Errorf("fuzzer not configured")
	}
	if a.config.Target == "" {
		return fmt.Errorf("target not specified")
	}
	// Check if target exists
	if _, err := os.Stat(a.config.Target); os.IsNotExist(err) {
		return fmt.Errorf("target not found: %s", a.config.Target)
	}
	return nil
}

// GetProgress returns the current fuzzer progress.
func (a *FuzzerAdapter) GetProgress() FuzzerProgress {
	stats := a.GetStats()
	progress := FuzzerProgress{
		Phase:           "calibration",
		ProgressPercent: stats.CoveragePercent,
		LastUpdate:      time.Now(),
	}
	if a.IsRunning() {
		progress.Phase = "running"
	}
	return progress
}

// GetCoverage returns coverage information.
func (a *FuzzerAdapter) GetCoverage() (*common.CoverageResult, error) {
	stats := a.GetStats()
	jobID := ""
	if a.config != nil {
		jobID = a.config.Target
	}
	return &common.CoverageResult{
		ID:        fmt.Sprintf("cov-%d", time.Now().UnixNano()),
		JobID:     jobID,
		Edges:     int(stats.CoveragePercent * 100),
		Timestamp: time.Now(),
	}, nil
}

// Pause pauses the fuzzer. Not all fuzzers support this.
func (a *FuzzerAdapter) Pause() error {
	return fmt.Errorf("%s does not support pause", a.Name())
}

// Resume resumes a paused fuzzer. Not all fuzzers support this.
func (a *FuzzerAdapter) Resume() error {
	return fmt.Errorf("%s does not support resume", a.Name())
}

// GetCorpus returns the current corpus entries.
func (a *FuzzerAdapter) GetCorpus() ([]*CorpusEntry, error) {
	entries := make([]*CorpusEntry, 0)
	if a.config == nil || a.config.OutputDirectory == "" {
		return entries, nil
	}

	// Look for corpus in output directory
	corpusDir := filepath.Join(a.config.OutputDirectory, "corpus")
	if _, err := os.Stat(corpusDir); os.IsNotExist(err) {
		// Try seed directory
		if a.config.SeedDirectory != "" {
			corpusDir = a.config.SeedDirectory
		} else {
			return entries, nil
		}
	}

	// Read corpus files
	files, err := os.ReadDir(corpusDir)
	if err != nil {
		return entries, nil
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		filePath := filepath.Join(corpusDir, file.Name())
		info, err := file.Info()
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		entries = append(entries, &CorpusEntry{
			ID:        hash,
			FileName:  file.Name(),
			Size:      info.Size(),
			Hash:      hash,
			Timestamp: info.ModTime(),
		})
	}
	return entries, nil
}

// CollectCoverageData collects coverage data from the fuzzer.
func (a *FuzzerAdapter) CollectCoverageData() (map[string]interface{}, error) {
	// Check if engine implements coverage collection
	if collector, ok := a.engine.(interface {
		CollectCoverageData() (map[string]interface{}, error)
	}); ok {
		return collector.CollectCoverageData()
	}
	return nil, nil
}

// ReproduceCrash attempts to reproduce a crash with the given input.
// This is a stub implementation - crash reproduction is not yet supported
// by the domain fuzzer engines.
func (a *FuzzerAdapter) ReproduceCrash(ctx context.Context, input []byte, config ReproductionConfig) (*common.ReproductionResult, error) {
	// For now, return a result indicating reproduction is not supported
	// The domain fuzzer engines don't yet implement crash reproduction
	a.log.Warn("Crash reproduction not yet implemented in domain fuzzer adapter")

	return &common.ReproductionResult{
		Reproduced:      false,
		MatchesOriginal: false,
		Status:          common.ReproducibilityStatusFailed,
		Output:          "Crash reproduction not yet implemented",
		EnvironmentInfo: map[string]string{
			"fuzzer_type": a.engine.GetType(),
			"bot_id":      a.botID,
		},
	}, nil
}

// Helper methods

func (a *FuzzerAdapter) setStatus(status FuzzerStatus) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.status = status
}

func (a *FuzzerAdapter) getCrashes() []*common.CrashResult {
	a.crashesMu.RLock()
	defer a.crashesMu.RUnlock()
	result := make([]*common.CrashResult, len(a.crashes))
	copy(result, a.crashes)
	return result
}

func (a *FuzzerAdapter) addCrash(crash *common.CrashResult) {
	a.crashesMu.Lock()
	defer a.crashesMu.Unlock()
	a.crashes = append(a.crashes, crash)
}

func (a *FuzzerAdapter) monitorCrashes() {
	defer a.wg.Done()

	crashChan := a.engine.GetCrashes()
	if crashChan == nil {
		return
	}

	for {
		select {
		case <-a.ctx.Done():
			return
		case crashInfo, ok := <-crashChan:
			if !ok {
				return
			}
			if crashInfo == nil {
				continue
			}

			// Convert domain crash to common crash
			crash := a.convertCrash(crashInfo)
			a.addCrash(crash)

			// Notify handler
			if a.handler != nil {
				a.handler.OnCrash(a, crash)
			}
		}
	}
}

func (a *FuzzerAdapter) monitorProgress() {
	defer a.wg.Done()

	progressChan := a.engine.GetProgress()
	if progressChan == nil {
		return
	}

	for {
		select {
		case <-a.ctx.Done():
			return
		case progress, ok := <-progressChan:
			if !ok {
				return
			}
			if progress == nil {
				continue
			}

			// Update stats
			a.statsMu.Lock()
			a.stats.Executions = int64(progress.Executions)
			a.stats.ExecPerSecond = float64(progress.ExecsPerSecond)
			a.stats.CorpusSize = int(progress.CorpusSize)
			a.stats.CoveragePercent = progress.Coverage
			a.stats.ElapsedTime = time.Since(a.stats.StartTime)
			stats := a.stats
			a.statsMu.Unlock()

			// Notify handler
			if a.handler != nil {
				a.handler.OnStats(a, stats)
				a.handler.OnProgress(a, FuzzerProgress{
					ProgressPercent: progress.Coverage,
					LastUpdate:      progress.Timestamp,
				})
			}
		}
	}
}

func (a *FuzzerAdapter) convertConfig(config FuzzConfig) *types.FuzzerConfig {
	domainConfig := &types.FuzzerConfig{
		Timeout:        config.Timeout,
		MaxDuration:    config.Duration,
		MemoryLimit:    uint64(config.MemoryLimit),
		Dictionary:     config.Dictionary,
		SeedCorpus:     config.SeedDirectory,
		OutputDir:      config.OutputDirectory,
		EnableCoverage: config.Coverage != "",
		CoverageFormat: string(config.Coverage),
		CoverageDir:    config.OutputDirectory,
	}

	// Set fuzzer-specific options based on fuzzer type
	switch a.engine.GetType() {
	case "afl++":
		domainConfig.AFLPlusPlusOptions = &types.AFLPlusPlusOptions{
			InputDir: config.SeedDirectory,
			NoUI:     true,
		}
		if opts, ok := config.FuzzerOptions["dumb_mode"].(bool); ok && opts {
			domainConfig.AFLPlusPlusOptions.DumbMode = true
		}
	case "libfuzzer":
		domainConfig.LibFuzzerOptions = &types.LibFuzzerOptions{
			MaxTotalTime: int(config.Duration.Seconds()),
		}
	case "honggfuzz":
		domainConfig.HonggfuzzOptions = &types.HonggfuzzOptions{
			InputDir: config.SeedDirectory,
			RunTime:  int(config.Duration.Seconds()),
		}
	}

	return domainConfig
}

func (a *FuzzerAdapter) convertCrash(info *types.CrashInfo) *common.CrashResult {
	crashType := "crash"
	if info.Metadata != nil {
		if t, ok := info.Metadata["type"]; ok {
			crashType = t
		}
	}

	return &common.CrashResult{
		ID:         info.ID,
		JobID:      a.config.JobID,
		BotID:      a.botID,
		Hash:       info.ID, // Use ID as hash if not available
		Type:       crashType,
		Signal:     info.Signal,
		StackTrace: info.StackTrace,
		Input:      info.Input,
		Size:       int64(len(info.Input)),
		Timestamp:  info.DiscoveredAt,
		IsUnique:   true,
	}
}
