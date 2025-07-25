package executor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
	jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
)

// FuzzerExecutor implements the Executor interface for fuzzing jobs
type FuzzerExecutor struct {
	*BaseExecutor
	fuzzerFactory fuzzertypes.FuzzerFactory
	registry      BotRegistry
	mu            sync.RWMutex
	activeFuzzers map[string]fuzzertypes.Fuzzer
}

// BotRegistry interface for bot management operations
type BotRegistry interface {
	AssignWork(ctx context.Context, botID, jobID, jobType string, metadata map[string]interface{}) error
	CompleteWork(ctx context.Context, botID, jobID string, results map[string]interface{}) error
	FailWork(ctx context.Context, botID, jobID, errorMsg, reason string) error
}

// NewFuzzerExecutor creates a new fuzzer executor
func NewFuzzerExecutor(
	config *ExecutorConfig,
	eventPub types.BotEventPublisher,
	fuzzerFactory fuzzertypes.FuzzerFactory,
	registry BotRegistry,
	hooks ExecutorHooks,
) (*FuzzerExecutor, error) {
	if fuzzerFactory == nil {
		return nil, errors.New("fuzzer factory cannot be nil")
	}
	if registry == nil {
		return nil, errors.New("bot registry cannot be nil")
	}

	return &FuzzerExecutor{
		BaseExecutor:  NewBaseExecutor(config, eventPub, hooks),
		fuzzerFactory: fuzzerFactory,
		registry:      registry,
		activeFuzzers: make(map[string]fuzzertypes.Fuzzer),
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
		StartTime: execCtx.StartTime,
		Metrics:   make(map[string]interface{}),
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
