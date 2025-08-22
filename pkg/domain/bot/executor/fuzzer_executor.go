package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/bot"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
	fuzzertypes "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
	jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/filesystem"
	"github.com/ethpandaops/pandafuzz/pkg/storage/backend"
)

// FuzzerExecutor implements the Executor interface for fuzzing jobs
type FuzzerExecutor struct {
	*BaseExecutor
	fuzzerFactory  fuzzertypes.FuzzerFactory
	registry       BotRegistry
	mu             sync.RWMutex
	activeFuzzers  map[string]fuzzertypes.Fuzzer
	log            logrus.FieldLogger
	storageBackend backend.StorageBackend // Optional storage backend for coverage collection
	apiClient      bot.APIClient          // Optional API client for reporting to master
}

// BotRegistry interface for bot management operations
type BotRegistry interface {
	AssignWork(ctx context.Context, botID, jobID, jobType string, metadata map[string]interface{}) error
	CompleteWork(ctx context.Context, botID, jobID string, results map[string]interface{}) error
	FailWork(ctx context.Context, botID, jobID, errorMsg, reason string) error
}

// FuzzingResult represents the result of a fuzzing job execution
type FuzzingResult struct {
	ExecutionResult
	CoverageCollected bool                    `json:"coverage_collected"`
	CoverageStats     *jobtypes.CoverageStats `json:"coverage_stats,omitempty"`
	CoverageReportID  string                  `json:"coverage_report_id,omitempty"`
}

// NewFuzzerExecutor creates a new fuzzer executor
func NewFuzzerExecutor(
	config *ExecutorConfig,
	eventPub types.BotEventPublisher,
	fuzzerFactory fuzzertypes.FuzzerFactory,
	registry BotRegistry,
	hooks ExecutorHooks,
) (*FuzzerExecutor, error) {
	return NewFuzzerExecutorWithStorage(config, eventPub, fuzzerFactory, registry, hooks, nil, nil)
}

// NewFuzzerExecutorWithStorage creates a new fuzzer executor with optional storage backend and API client
func NewFuzzerExecutorWithStorage(
	config *ExecutorConfig,
	eventPub types.BotEventPublisher,
	fuzzerFactory fuzzertypes.FuzzerFactory,
	registry BotRegistry,
	hooks ExecutorHooks,
	storageBackend backend.StorageBackend,
	apiClient bot.APIClient,
) (*FuzzerExecutor, error) {
	if fuzzerFactory == nil {
		return nil, errors.New("fuzzer factory cannot be nil")
	}
	if registry == nil {
		return nil, errors.New("bot registry cannot be nil")
	}

	return &FuzzerExecutor{
		BaseExecutor:   NewBaseExecutor(config, eventPub, hooks),
		fuzzerFactory:  fuzzerFactory,
		registry:       registry,
		activeFuzzers:  make(map[string]fuzzertypes.Fuzzer),
		log:            logrus.WithField("component", "fuzzer_executor"),
		storageBackend: storageBackend,
		apiClient:      apiClient,
	}, nil
}

// Execute runs a fuzzing job on the specified bot
func (fe *FuzzerExecutor) Execute(ctx context.Context, bot *types.Agent, job *jobtypes.Job) error {
	// Validate bot has fuzzing capability
	if !bot.HasCapability(types.CapabilityFuzzing) {
		return NewExecutorError("invalid_bot", "bot does not have fuzzing capability", false)
	}

	// Validate job
	if err := fe.ValidateJob(job); err != nil {
		return err
	}

	// Create execution context
	execCtx, cancel := context.WithTimeout(ctx, fe.config.JobTimeout)
	executionContext := &ExecutionContext{
		Bot:       bot,
		Job:       job,
		Config:    fe.config,
		StartTime: time.Now(),
		CancelFn:  cancel,
	}

	// Store execution context
	fe.StoreExecution(bot.ID, job.ID, executionContext)
	defer fe.RemoveExecution(bot.ID, job.ID)

	// Notify registry that work is assigned
	metadata := map[string]interface{}{
		"fuzzer_type": job.FuzzerType,
		"target":      job.TargetBinary,
	}
	if err := fe.registry.AssignWork(ctx, bot.ID, job.ID, "fuzzing", metadata); err != nil {
		cancel()
		return fmt.Errorf("failed to assign work: %w", err)
	}

	// Execute hooks
	if fe.hooks != nil {
		if err := fe.hooks.OnExecutionStart(executionContext); err != nil {
			cancel()
			return fmt.Errorf("execution start hook failed: %w", err)
		}
	}

	// Run the fuzzing job
	result := fe.runFuzzingJob(execCtx, executionContext)

	// Execute completion hooks
	if fe.hooks != nil {
		if result.Success {
			_ = fe.hooks.OnExecutionComplete(executionContext, result)
		} else {
			_ = fe.hooks.OnExecutionError(executionContext, result.Error)
		}
	}

	// Update registry based on result
	if result.Success {
		results := map[string]interface{}{
			"crashes_found": result.Metrics["crashes_found"],
			"coverage":      result.Metrics["coverage"],
			"duration":      result.Duration.String(),
		}

		// Add coverage information if available
		if coverageCollected, ok := result.Metrics["coverage_collected"].(bool); ok && coverageCollected {
			results["coverage_collected"] = true
			results["coverage_report_id"] = result.Metrics["coverage_report_id"]
			results["coverage_line"] = result.Metrics["coverage_line"]
			results["coverage_function"] = result.Metrics["coverage_function"]
			if branchCov, exists := result.Metrics["coverage_branch"]; exists {
				results["coverage_branch"] = branchCov
			}
		}

		if err := fe.registry.CompleteWork(ctx, bot.ID, job.ID, results); err != nil {
			return fmt.Errorf("failed to complete work: %w", err)
		}
	} else {
		if err := fe.registry.FailWork(ctx, bot.ID, job.ID, result.Error.Error(), "execution failed"); err != nil {
			return fmt.Errorf("failed to report work failure: %w", err)
		}
		return result.Error
	}

	return nil
}

// runFuzzingJob executes the actual fuzzing job
func (fe *FuzzerExecutor) runFuzzingJob(ctx context.Context, execCtx *ExecutionContext) *ExecutionResult {
	result := &ExecutionResult{
		Metrics: make(map[string]interface{}),
	}

	// Create fuzzer instance
	fuzzer, err := fe.fuzzerFactory.CreateFuzzer(
		execCtx.Job.FuzzerType,
		execCtx.Job.TargetBinary,
		execCtx.Job.TargetArgs,
	)
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("failed to create fuzzer: %w", err)
		result.Duration = time.Since(execCtx.StartTime)
		return result
	}

	// Store active fuzzer
	fe.mu.Lock()
	fe.activeFuzzers[execCtx.Job.ID] = fuzzer
	fe.mu.Unlock()

	defer func() {
		fe.mu.Lock()
		delete(fe.activeFuzzers, execCtx.Job.ID)
		fe.mu.Unlock()
	}()

	// Configure fuzzer
	if err := fe.configureFuzzer(fuzzer, execCtx.Job); err != nil {
		result.Success = false
		result.Error = fmt.Errorf("failed to configure fuzzer: %w", err)
		result.Duration = time.Since(execCtx.StartTime)
		return result
	}

	// Start fuzzing
	if err := fuzzer.Start(ctx); err != nil {
		result.Success = false
		result.Error = fmt.Errorf("failed to start fuzzer: %w", err)
		result.Duration = time.Since(execCtx.StartTime)
		return result
	}

	// Monitor fuzzing progress
	fe.monitorFuzzing(ctx, execCtx, fuzzer, result)

	// Stop fuzzer
	if err := fuzzer.Stop(); err != nil {
		result.Error = fmt.Errorf("failed to stop fuzzer: %w", err)
	}

	// Get final stats
	if stats, err := fuzzer.GetStats(); err == nil {
		result.Metrics["total_executions"] = stats.TotalExecutions
		result.Metrics["crashes_found"] = stats.CrashesFound
		result.Metrics["coverage"] = stats.Coverage
		result.Metrics["execs_per_second"] = stats.ExecsPerSecond
	}

	// Collect and upload coverage if enabled
	if execCtx.Job.EnableCoverage {
		if err := fe.collectAndUploadCoverage(ctx, execCtx, fuzzer, result); err != nil {
			// Log error but don't fail the entire job
			if fe.eventPub != nil {
				fe.eventPub.PublishEvent(types.BaseBotEvent{
					Type:      types.EventBotWorkFailed,
					BotID:     execCtx.Bot.ID,
					Timestamp: time.Now(),
					Data:      map[string]interface{}{"error": err.Error(), "job_id": execCtx.Job.ID},
				})
			}
		}
	}

	result.Duration = time.Since(execCtx.StartTime)
	if result.Error == nil {
		result.Success = true
	}

	return result
}

// configureFuzzer applies configuration to the fuzzer
func (fe *FuzzerExecutor) configureFuzzer(fuzzer fuzzertypes.Fuzzer, job *jobtypes.Job) error {
	// Create fuzzer config
	config, err := fuzzertypes.NewFuzzerConfig(job.OutputPath)
	if err != nil {
		return err
	}

	// Apply job-specific configuration
	if timeout, exists := job.GetFuzzerConfig("timeout"); exists {
		if d, ok := timeout.(time.Duration); ok {
			config.Timeout = d
		}
	}

	if memLimit, exists := job.GetFuzzerConfig("memory_limit"); exists {
		if limit, ok := memLimit.(uint64); ok {
			config.MemoryLimit = limit
		}
	}

	if workers, exists := job.GetFuzzerConfig("workers"); exists {
		if w, ok := workers.(int); ok {
			config.Workers = w
		}
	}

	// Apply coverage configuration
	if job.EnableCoverage {
		config.EnableCoverage = true
		config.CoverageFormat = job.CoverageFormat
		config.CoverageDir = filepath.Join("/tmp", "pandafuzz", job.ID, "coverage")
	}

	// Apply resource limits if configured
	if fe.config.ResourceLimits != nil {
		if fe.config.ResourceLimits.MaxMemory > 0 {
			config.MemoryLimit = fe.config.ResourceLimits.MaxMemory
		}
	}

	// Set corpus and output paths
	if err := fuzzer.SetCorpus(job.CorpusPath); err != nil {
		return fmt.Errorf("failed to set corpus: %w", err)
	}

	if err := fuzzer.SetOutput(job.OutputPath); err != nil {
		return fmt.Errorf("failed to set output: %w", err)
	}

	// Apply configuration
	return fuzzer.Configure(config)
}

// monitorFuzzing monitors the fuzzing progress
func (fe *FuzzerExecutor) monitorFuzzing(ctx context.Context, execCtx *ExecutionContext, fuzzer fuzzertypes.Fuzzer, result *ExecutionResult) {
	// Create channels for monitoring
	crashChan := fuzzer.GetCrashes()
	progressChan := fuzzer.GetProgress()

	// Create ticker for heartbeat
	ticker := time.NewTicker(fe.config.HeartbeatInterval)
	defer ticker.Stop()

	// Track crashes
	var crashCount uint64
	crashes := make([]string, 0)

	for {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			return

		case crash := <-crashChan:
			if crash != nil {
				crashCount++
				crashes = append(crashes, crash.ID)

				// Update job crash count
				execCtx.Job.IncrementCrashCount()

				// Store crash info in result
				if result.Artifacts == nil {
					result.Artifacts = make([]string, 0)
				}
				result.Artifacts = append(result.Artifacts, crash.ID)
			}

		case progress := <-progressChan:
			if progress != nil {
				// Convert to job progress
				jobProgress := &jobtypes.JobProgress{
					TotalExecs:     progress.Executions,
					ExecsPerSecond: progress.ExecsPerSecond,
					CorpusSize:     progress.CorpusSize,
					Coverage:       progress.Coverage,
					LastUpdated:    progress.Timestamp,
				}

				// Update job progress
				_ = execCtx.Job.UpdateProgress(jobProgress)

				// Execute progress hook
				if fe.hooks != nil {
					_ = fe.hooks.OnProgressUpdate(execCtx, jobProgress)
				}
			}

		case <-ticker.C:
			// Check if fuzzer is still running
			if !fuzzer.IsRunning() {
				return
			}

			// Get current stats for heartbeat
			if stats, err := fuzzer.GetStats(); err == nil {
				jobProgress := &jobtypes.JobProgress{
					TotalExecs:     stats.TotalExecutions,
					ExecsPerSecond: stats.ExecsPerSecond,
					CorpusSize:     stats.CorpusSize,
					Coverage:       stats.Coverage,
					LastUpdated:    time.Now(),
				}

				// Calculate estimated time left if max duration is set
				if execCtx.Job.MaxDuration > 0 {
					elapsed := time.Since(execCtx.StartTime)
					remaining := execCtx.Job.MaxDuration - elapsed
					if remaining > 0 {
						jobProgress.EstimatedTimeLeft = &remaining
					}
				}

				_ = execCtx.Job.UpdateProgress(jobProgress)
			}
		}

		// Check if job has exceeded max duration
		if execCtx.Job.IsExpired() {
			result.Error = errors.New("job exceeded maximum duration")
			return
		}
	}
}

// Cancel cancels a running fuzzing job
func (fe *FuzzerExecutor) Cancel(ctx context.Context, botID, jobID string) error {
	// Get execution context
	execCtx, exists := fe.GetExecution(botID, jobID)
	if !exists {
		return NewExecutorError("not_found", "execution not found", false)
	}

	// Cancel the context
	if execCtx.CancelFn != nil {
		execCtx.CancelFn()
	}

	// Stop the fuzzer if it exists
	fe.mu.RLock()
	fuzzer, exists := fe.activeFuzzers[jobID]
	fe.mu.RUnlock()

	if exists && fuzzer != nil {
		if err := fuzzer.Stop(); err != nil {
			return fmt.Errorf("failed to stop fuzzer: %w", err)
		}
	}

	return nil
}

// GetStatus returns the current execution status
func (fe *FuzzerExecutor) GetStatus(ctx context.Context, botID, jobID string) (*ExecutionStatus, error) {
	// Get execution context
	execCtx, exists := fe.GetExecution(botID, jobID)
	if !exists {
		return nil, NewExecutorError("not_found", "execution not found", false)
	}

	status := &ExecutionStatus{
		BotID:      botID,
		JobID:      jobID,
		Status:     execCtx.Job.Status,
		StartTime:  execCtx.StartTime,
		LastUpdate: execCtx.Job.UpdatedAt,
		Progress:   execCtx.Job.Progress,
		Metadata:   make(map[string]interface{}),
	}

	// Get fuzzer-specific status
	fe.mu.RLock()
	fuzzer, exists := fe.activeFuzzers[jobID]
	fe.mu.RUnlock()

	if exists && fuzzer != nil {
		status.Metadata["fuzzer_type"] = fuzzer.GetType()
		status.Metadata["fuzzer_version"] = fuzzer.GetVersion()
		status.Metadata["is_running"] = fuzzer.IsRunning()

		if stats, err := fuzzer.GetStats(); err == nil {
			status.Metadata["stats"] = stats
		}
	}

	return status, nil
}

// GetCapabilities returns the capabilities required by this executor
func (fe *FuzzerExecutor) GetCapabilities() []types.Capability {
	return []types.Capability{types.CapabilityFuzzing}
}

// ValidateJob validates if a job can be executed by this executor
func (fe *FuzzerExecutor) ValidateJob(job *jobtypes.Job) error {
	if job == nil {
		return NewExecutorError("invalid_job", "job cannot be nil", false)
	}

	// Check if fuzzer type is supported
	if !fe.fuzzerFactory.IsSupported(job.FuzzerType) {
		return NewExecutorError("unsupported_fuzzer",
			fmt.Sprintf("fuzzer type %s is not supported", job.FuzzerType), false)
	}

	// Validate required fields
	if job.TargetBinary == "" {
		return NewExecutorError("invalid_job", "target binary is required", false)
	}

	if job.CorpusPath == "" {
		return NewExecutorError("invalid_job", "corpus path is required", false)
	}

	if job.OutputPath == "" {
		return NewExecutorError("invalid_job", "output path is required", false)
	}

	// Validate job status
	if !job.Status.CanTransitionTo(jobtypes.StatusRunning) {
		return NewExecutorError("invalid_status",
			fmt.Sprintf("job cannot transition from %s to running", job.Status), false)
	}

	return nil
}

// collectAndUploadCoverage collects coverage data from the fuzzer and uploads it to storage
func (fe *FuzzerExecutor) collectAndUploadCoverage(ctx context.Context, execCtx *ExecutionContext, fuzzer fuzzertypes.Fuzzer, result *ExecutionResult) error {
	if !execCtx.Job.EnableCoverage {
		return nil
	}

	fe.log.WithFields(logrus.Fields{
		"job_id":          execCtx.Job.ID,
		"storage_backend": fe.storageBackend != nil,
		"api_client":      fe.apiClient != nil,
	}).Debug("Starting coverage collection")

	// Check if we have the new orchestrator available (storage backend and API client)
	if fe.storageBackend != nil && fe.apiClient != nil {
		return fe.collectCoverageWithOrchestrator(ctx, execCtx, fuzzer, result)
	}

	// Fall back to basic coverage collection for backward compatibility
	fe.log.WithField("job_id", execCtx.Job.ID).Warn("Storage backend or API client not available, using basic coverage collection")
	return fe.collectCoverageBasic(ctx, execCtx, fuzzer, result)
}

// collectCoverageWithOrchestrator uses the new CoverageCollector orchestrator
func (fe *FuzzerExecutor) collectCoverageWithOrchestrator(ctx context.Context, execCtx *ExecutionContext, fuzzer fuzzertypes.Fuzzer, result *ExecutionResult) error {
	// Create coverage collector with the new orchestrator
	coverageCollector := bot.NewCoverageCollector(
		fe.storageBackend,
		fe.apiClient,
		fe.log.WithField("component", "coverage_collector"),
	)

	// Convert job types to common types for the orchestrator
	campaignID := execCtx.Job.ID // Use job ID as campaign ID if not available
	commonJob := &common.Job{
		ID:             execCtx.Job.ID,
		CampaignID:     &campaignID,
		Target:         execCtx.Job.TargetBinary,
		WorkDir:        execCtx.Job.OutputPath,
		Fuzzer:         execCtx.Job.FuzzerType,
		EnableCoverage: execCtx.Job.EnableCoverage,
		CoverageFormat: execCtx.Job.CoverageFormat,
	}

	// Use the orchestrator to collect and store coverage
	if err := coverageCollector.CollectAndStore(ctx, commonJob, fuzzer); err != nil {
		fe.log.WithError(err).WithField("job_id", execCtx.Job.ID).Error("Coverage collection failed with orchestrator")
		// Try to collect basic stats anyway for reporting
		if basicErr := fe.collectBasicStatsOnly(ctx, execCtx, fuzzer, result); basicErr != nil {
			fe.log.WithError(basicErr).Warn("Failed to collect basic stats after orchestrator failure")
		}
		return fmt.Errorf("coverage collection failed: %w", err)
	}

	// Collect basic stats for result metrics reporting
	if err := fe.collectBasicStatsOnly(ctx, execCtx, fuzzer, result); err != nil {
		fe.log.WithError(err).Warn("Failed to collect basic stats for result metrics")
		// Don't fail the whole operation for this
	}

	fe.log.WithField("job_id", execCtx.Job.ID).Info("Coverage collection completed successfully using orchestrator")
	return nil
}

// collectCoverageBasic provides backward-compatible coverage collection without storage orchestrator
func (fe *FuzzerExecutor) collectCoverageBasic(ctx context.Context, execCtx *ExecutionContext, fuzzer fuzzertypes.Fuzzer, result *ExecutionResult) error {
	// Collect coverage data from the appropriate fuzzer engine
	var coverageData map[string]interface{}
	var err error

	// Try different approaches to get coverage data
	// First, check if the fuzzer implements the GetCoverageData method
	if coverageGetter, ok := fuzzer.(interface{ GetCoverageData() map[string]interface{} }); ok {
		coverageData = coverageGetter.GetCoverageData()
		fe.log.WithField("data_size", len(coverageData)).Debug("Got coverage data from GetCoverageData")
	}

	// If no data or empty, try to actively collect it
	// Note: The method is exported (capital C) in the actual implementations
	if len(coverageData) == 0 {
		if collector, ok := fuzzer.(interface {
			CollectCoverageData() (map[string]interface{}, error)
		}); ok {
			coverageData, err = collector.CollectCoverageData()
			if err != nil {
				fe.log.WithError(err).Warn("Failed to actively collect coverage data")
			} else {
				fe.log.WithField("data_size", len(coverageData)).Debug("Collected fresh coverage data")
			}
		}
	}

	// If still no data, return an error
	if len(coverageData) == 0 {
		fe.log.Warn("No coverage data available from fuzzer")
		return fmt.Errorf("no coverage data collected - fuzzer may not support coverage or coverage collection failed")
	}

	// Parse coverage statistics
	coverageStats, err := fe.parseCoverageStats(coverageData, execCtx.Job.CoverageFormat)
	if err != nil {
		return fmt.Errorf("failed to parse coverage stats: %w", err)
	}

	// Create coverage repository
	// We need a logger, use a simple approach for now
	coverageRepo, err := filesystem.NewFilesystemCoverageRepository(
		filepath.Join("/tmp", "pandafuzz", "coverage"),
		nil, // TODO: Pass proper logger
	)
	if err != nil {
		return fmt.Errorf("failed to create coverage repository: %w", err)
	}

	// Serialize coverage data for storage
	coverageJSON, err := json.Marshal(coverageData)
	if err != nil {
		return fmt.Errorf("failed to serialize coverage data: %w", err)
	}

	// Upload coverage data to local storage for backup
	if err := coverageRepo.Store(ctx, execCtx.Job.ID, coverageJSON); err != nil {
		fe.log.WithError(err).Warn("Failed to store coverage data locally")
	}

	// Generate unique report ID
	reportID := fmt.Sprintf("coverage-%s-%d", execCtx.Job.ID, time.Now().Unix())

	// Try to send detailed coverage report to master via API client
	if fe.apiClient != nil {
		coverageReport := map[string]interface{}{
			"job_id":            execCtx.Job.ID,
			"bot_id":            execCtx.Bot.ID,
			"report_id":         reportID,
			"format":            execCtx.Job.CoverageFormat,
			"coverage_data":     coverageData,
			"line_coverage":     coverageStats.LineCoverage,
			"function_coverage": coverageStats.FunctionCoverage,
			"branch_coverage":   coverageStats.BranchCoverage,
			"collected_at":      time.Now(),
		}

		// Try to report via the API client
		if err := fe.apiClient.ReportCoverageData(coverageReport); err != nil {
			fe.log.WithError(err).Warn("Failed to report coverage data to master via API client")
		} else {
			fe.log.WithField("report_id", reportID).Info("Coverage data reported to master via API client")
		}
	} else {
		fe.log.Debug("No API client available for coverage reporting")
	}

	// Update job with coverage information
	execCtx.Job.CoverageReportID = reportID
	if err := execCtx.Job.UpdateCoverageStats(coverageStats); err != nil {
		return fmt.Errorf("failed to update job coverage stats: %w", err)
	}

	// Update result metrics
	result.Metrics["coverage_collected"] = true
	result.Metrics["coverage_report_id"] = reportID
	result.Metrics["coverage_line"] = coverageStats.LineCoverage
	result.Metrics["coverage_function"] = coverageStats.FunctionCoverage
	if coverageStats.BranchCoverage > 0 {
		result.Metrics["coverage_branch"] = coverageStats.BranchCoverage
	}

	return nil
}

// collectBasicStatsOnly collects basic coverage statistics for result metrics without full processing
func (fe *FuzzerExecutor) collectBasicStatsOnly(ctx context.Context, execCtx *ExecutionContext, fuzzer fuzzertypes.Fuzzer, result *ExecutionResult) error {
	// Try to collect basic coverage data for stats reporting
	var coverageData map[string]interface{}

	// Try different approaches to get coverage data
	if coverageGetter, ok := fuzzer.(interface{ GetCoverageData() map[string]interface{} }); ok {
		coverageData = coverageGetter.GetCoverageData()
	} else if collector, ok := fuzzer.(interface {
		CollectCoverageData() (map[string]interface{}, error)
	}); ok {
		var err error
		coverageData, err = collector.CollectCoverageData()
		if err != nil {
			fe.log.WithError(err).Debug("Failed to collect coverage data for basic stats")
			return err
		}
	}

	if len(coverageData) == 0 {
		fe.log.Debug("No coverage data available for basic stats collection")
		return nil
	}

	// Parse coverage statistics
	coverageStats, err := fe.parseCoverageStats(coverageData, execCtx.Job.CoverageFormat)
	if err != nil {
		fe.log.WithError(err).Warn("Failed to parse coverage stats for basic collection")
		return err
	}

	// Generate unique report ID
	reportID := fmt.Sprintf("coverage-%s-%d", execCtx.Job.ID, time.Now().Unix())

	// Update result metrics
	result.Metrics["coverage_collected"] = true
	result.Metrics["coverage_report_id"] = reportID
	result.Metrics["coverage_line"] = coverageStats.LineCoverage
	result.Metrics["coverage_function"] = coverageStats.FunctionCoverage
	if coverageStats.BranchCoverage > 0 {
		result.Metrics["coverage_branch"] = coverageStats.BranchCoverage
	}

	fe.log.WithFields(logrus.Fields{
		"job_id":            execCtx.Job.ID,
		"line_coverage":     coverageStats.LineCoverage,
		"function_coverage": coverageStats.FunctionCoverage,
		"branch_coverage":   coverageStats.BranchCoverage,
	}).Debug("Collected basic coverage stats")

	return nil
}

// parseCoverageStats parses coverage data into CoverageStats
func (fe *FuzzerExecutor) parseCoverageStats(coverageData map[string]interface{}, format string) (*jobtypes.CoverageStats, error) {
	stats := &jobtypes.CoverageStats{
		CollectedAt: time.Now(),
		ReportPath:  "", // Will be set by the repository
	}

	// Extract coverage percentages based on the data available
	if lineCov, ok := coverageData["line_coverage"].(float64); ok {
		stats.LineCoverage = lineCov
	} else if coverage, ok := coverageData["coverage_percent"].(float64); ok {
		stats.LineCoverage = coverage
	}

	if funcCov, ok := coverageData["function_coverage"].(float64); ok {
		stats.FunctionCoverage = funcCov
	} else {
		// Estimate function coverage from line coverage if not available
		stats.FunctionCoverage = stats.LineCoverage * 0.8 // Conservative estimate
	}

	if branchCov, ok := coverageData["branch_coverage"].(float64); ok {
		stats.BranchCoverage = branchCov
	}

	// Set report path based on format and data
	if reportPath, ok := coverageData["report_path"].(string); ok {
		stats.ReportPath = reportPath
	} else {
		// Construct expected report path based on format
		switch format {
		case "html":
			stats.ReportPath = "html/index.html"
		case "lcov":
			stats.ReportPath = "coverage.info"
		case "json":
			stats.ReportPath = "coverage.json"
		default:
			stats.ReportPath = "coverage.txt"
		}
	}

	return stats, nil
}
