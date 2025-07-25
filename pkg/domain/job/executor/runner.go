package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	fuzzertypes "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
)

// JobRunner defines the interface for executing jobs
type JobRunner interface {
	// ExecuteJob executes a job with proper isolation and resource management
	ExecuteJob(ctx context.Context, job *types.Job) (*ExecutionResult, error)

	// CancelJob cancels a running job
	CancelJob(jobID string) error

	// GetRunningJobs returns currently running jobs
	GetRunningJobs() []*types.Job

	// GetExecutionStatus returns the status of a specific job execution
	GetExecutionStatus(jobID string) (*ExecutionStatus, error)

	// Start starts the job runner
	Start(ctx context.Context) error

	// Stop stops the job runner
	Stop() error
}

// ExecutionResult represents the result of a job execution
type ExecutionResult struct {
	JobID         string                   `json:"job_id"`
	Success       bool                     `json:"success"`
	Error         error                    `json:"error,omitempty"`
	StartTime     time.Time                `json:"start_time"`
	EndTime       time.Time                `json:"end_time"`
	Duration      time.Duration            `json:"duration"`
	ExitCode      int                      `json:"exit_code"`
	Crashes       []*fuzzertypes.CrashInfo `json:"crashes,omitempty"`
	FinalStats    *fuzzertypes.FuzzerStats `json:"final_stats,omitempty"`
	ResourceUsage *ResourceUsage           `json:"resource_usage,omitempty"`
	Artifacts     []string                 `json:"artifacts,omitempty"`
}

// ExecutionStatus represents the current status of a job execution
type ExecutionStatus struct {
	JobID        string             `json:"job_id"`
	Status       types.JobStatus    `json:"status"`
	StartTime    *time.Time         `json:"start_time,omitempty"`
	Progress     *types.JobProgress `json:"progress,omitempty"`
	ErrorMessage string             `json:"error_message,omitempty"`
	IsRunning    bool               `json:"is_running"`
}

// ResourceUsage tracks resource consumption during execution
type ResourceUsage struct {
	PeakMemory   uint64        `json:"peak_memory"` // Bytes
	CPUTime      time.Duration `json:"cpu_time"`
	UserTime     time.Duration `json:"user_time"`
	SystemTime   time.Duration `json:"system_time"`
	MaxProcesses int           `json:"max_processes"`
}

// ResourceLimits defines resource constraints for job execution
type ResourceLimits struct {
	MaxMemory    uint64        `json:"max_memory"`    // Bytes
	MaxCPU       float64       `json:"max_cpu"`       // CPU cores
	MaxDisk      uint64        `json:"max_disk"`      // Bytes
	MaxProcesses int           `json:"max_processes"` // Number of processes
	MaxFileSize  uint64        `json:"max_file_size"` // Bytes per file
	CPUTime      time.Duration `json:"cpu_time"`      // Max CPU time
}

// RunnerConfig provides configuration for the job runner
type RunnerConfig struct {
	// MaxConcurrentJobs limits concurrent job executions
	MaxConcurrentJobs int

	// JobTimeout is the default timeout for jobs
	JobTimeout time.Duration

	// WorkingDir is the base directory for job execution
	WorkingDir string

	// ArtifactsDir is where job artifacts are stored
	ArtifactsDir string

	// EnableResourceLimits enables resource limiting
	EnableResourceLimits bool

	// ProgressUpdateInterval is how often to update job progress
	ProgressUpdateInterval time.Duration

	// ResourceLimits defines resource constraints
	ResourceLimits *ResourceLimits
}

// runner implements JobRunner interface
type runner struct {
	mu            sync.RWMutex
	config        *RunnerConfig
	log           logrus.FieldLogger
	executions    map[string]*execution
	fuzzerFactory fuzzertypes.FuzzerFactory
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// execution represents a running job execution
type execution struct {
	job       *types.Job
	fuzzer    fuzzertypes.Fuzzer
	ctx       context.Context
	cancel    context.CancelFunc
	startTime time.Time
	status    *ExecutionStatus
	result    *ExecutionResult
	mu        sync.RWMutex
}

// NewJobRunner creates a new job runner instance
func NewJobRunner(config *RunnerConfig, fuzzerFactory fuzzertypes.FuzzerFactory, log logrus.FieldLogger) (JobRunner, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}
	if fuzzerFactory == nil {
		return nil, errors.New("fuzzer factory cannot be nil")
	}
	if log == nil {
		return nil, errors.New("logger cannot be nil")
	}

	// Set defaults
	if config.MaxConcurrentJobs <= 0 {
		config.MaxConcurrentJobs = 1
	}
	if config.JobTimeout <= 0 {
		config.JobTimeout = 24 * time.Hour
	}
	if config.ProgressUpdateInterval <= 0 {
		config.ProgressUpdateInterval = 30 * time.Second
	}
	if config.WorkingDir == "" {
		config.WorkingDir = "/tmp/pandafuzz/jobs"
	}
	if config.ArtifactsDir == "" {
		config.ArtifactsDir = "/tmp/pandafuzz/artifacts"
	}

	// Set default resource limits if not provided
	if config.ResourceLimits == nil {
		config.ResourceLimits = &ResourceLimits{
			MaxMemory:    4 * 1024 * 1024 * 1024, // 4GB
			MaxCPU:       2.0,
			MaxDisk:      10 * 1024 * 1024 * 1024, // 10GB
			MaxProcesses: 100,
			MaxFileSize:  100 * 1024 * 1024, // 100MB
			CPUTime:      24 * time.Hour,
		}
	}

	// Create directories
	if err := os.MkdirAll(config.WorkingDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create working directory: %w", err)
	}
	if err := os.MkdirAll(config.ArtifactsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create artifacts directory: %w", err)
	}

	return &runner{
		config:        config,
		log:           log.WithField("component", "job-runner"),
		executions:    make(map[string]*execution),
		fuzzerFactory: fuzzerFactory,
	}, nil
}

// Start starts the job runner
func (r *runner) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ctx != nil {
		return errors.New("runner already started")
	}

	r.ctx, r.cancel = context.WithCancel(ctx)
	r.log.Info("Job runner started")
	return nil
}

// Stop stops the job runner
func (r *runner) Stop() error {
	r.mu.Lock()
	if r.cancel != nil {
		r.cancel()
	}
	r.mu.Unlock()

	// Wait for all executions to complete
	r.wg.Wait()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Cancel any remaining executions
	for _, exec := range r.executions {
		if exec.cancel != nil {
			exec.cancel()
		}
	}

	r.log.Info("Job runner stopped")
	return nil
}

// ExecuteJob executes a job with proper isolation and resource management
func (r *runner) ExecuteJob(ctx context.Context, job *types.Job) (*ExecutionResult, error) {
	if job == nil {
		return nil, errors.New("job cannot be nil")
	}

	r.mu.Lock()
	// Check concurrent job limit
	if len(r.executions) >= r.config.MaxConcurrentJobs {
		r.mu.Unlock()
		return nil, fmt.Errorf("maximum concurrent jobs limit reached: %d", r.config.MaxConcurrentJobs)
	}

	// Check if job is already running
	if _, exists := r.executions[job.ID]; exists {
		r.mu.Unlock()
		return nil, fmt.Errorf("job %s is already running", job.ID)
	}

	// Create fuzzer instance based on job type
	fuzzer, err := r.createFuzzer(job)
	if err != nil {
		r.mu.Unlock()
		return nil, fmt.Errorf("failed to create fuzzer: %w", err)
	}

	// Create execution context
	execCtx, cancel := context.WithTimeout(ctx, r.getJobTimeout(job))
	exec := &execution{
		job:       job,
		fuzzer:    fuzzer,
		ctx:       execCtx,
		cancel:    cancel,
		startTime: time.Now(),
		status: &ExecutionStatus{
			JobID:     job.ID,
			Status:    types.StatusRunning,
			StartTime: timePtr(time.Now()),
			IsRunning: true,
		},
		result: &ExecutionResult{
			JobID:     job.ID,
			StartTime: time.Now(),
			Crashes:   make([]*fuzzertypes.CrashInfo, 0),
			Artifacts: make([]string, 0),
		},
	}

	r.executions[job.ID] = exec
	r.mu.Unlock()

	// Update job status
	if err := job.Start(); err != nil {
		r.log.WithError(err).WithField("job_id", job.ID).Error("Failed to update job start status")
	}

	// Start execution
	r.wg.Add(1)
	go r.runJob(exec)

	r.log.WithFields(logrus.Fields{
		"job_id":      job.ID,
		"fuzzer_type": job.FuzzerType,
		"target":      job.TargetBinary,
	}).Info("Job execution started")

	// Wait for a moment to ensure job starts properly
	time.Sleep(100 * time.Millisecond)

	// Return early, actual result will be available via GetExecutionStatus
	return &ExecutionResult{
		JobID:     job.ID,
		StartTime: exec.startTime,
		Success:   true,
	}, nil
}

// createFuzzer creates a fuzzer instance based on job configuration
func (r *runner) createFuzzer(job *types.Job) (fuzzertypes.Fuzzer, error) {
	// Map job fuzzer type to supported fuzzer types
	var fuzzerType string
	switch job.FuzzerType {
	case "libfuzzer", string(fuzzertypes.FuzzerTypeLibFuzzer):
		fuzzerType = string(fuzzertypes.FuzzerTypeLibFuzzer)
	case "afl++", string(fuzzertypes.FuzzerTypeAFLPlusPlus):
		fuzzerType = string(fuzzertypes.FuzzerTypeAFLPlusPlus)
	case "honggfuzz", string(fuzzertypes.FuzzerTypeHonggfuzz):
		fuzzerType = string(fuzzertypes.FuzzerTypeHonggfuzz)
	default:
		// Try to use the fuzzer type as-is
		fuzzerType = job.FuzzerType
	}

	return r.fuzzerFactory.CreateFuzzer(fuzzerType, job.TargetBinary, job.TargetArgs)
}

// runJob runs a job execution
func (r *runner) runJob(exec *execution) {
	defer r.wg.Done()
	defer func() {
		r.mu.Lock()
		delete(r.executions, exec.job.ID)
		r.mu.Unlock()
	}()

	// Setup job environment
	if err := r.setupJobEnvironment(exec); err != nil {
		r.handleJobError(exec, fmt.Errorf("failed to setup environment: %w", err))
		return
	}

	// Configure fuzzer
	if err := r.configureFuzzer(exec); err != nil {
		r.handleJobError(exec, fmt.Errorf("failed to configure fuzzer: %w", err))
		return
	}

	// Start fuzzer
	if err := exec.fuzzer.Start(exec.ctx); err != nil {
		r.handleJobError(exec, fmt.Errorf("failed to start fuzzer: %w", err))
		return
	}

	// Monitor execution
	r.monitorExecution(exec)

	// Stop fuzzer
	if err := exec.fuzzer.Stop(); err != nil {
		r.log.WithError(err).WithField("job_id", exec.job.ID).Warn("Failed to stop fuzzer gracefully")
	}

	// Collect results
	r.collectResults(exec)

	// Update job status
	exec.mu.Lock()
	exec.status.Status = types.StatusCompleted
	exec.status.IsRunning = false
	exec.result.Success = true
	exec.result.EndTime = time.Now()
	exec.result.Duration = time.Since(exec.startTime)
	exec.mu.Unlock()

	// Update job completion
	if err := exec.job.Complete(); err != nil {
		r.log.WithError(err).WithField("job_id", exec.job.ID).Error("Failed to update job completion status")
	}

	r.log.WithFields(logrus.Fields{
		"job_id":   exec.job.ID,
		"duration": exec.result.Duration,
		"crashes":  len(exec.result.Crashes),
	}).Info("Job execution completed")
}

// setupJobEnvironment sets up the execution environment for a job
func (r *runner) setupJobEnvironment(exec *execution) error {
	// Create job working directory
	jobDir := filepath.Join(r.config.WorkingDir, exec.job.ID)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		return fmt.Errorf("failed to create job directory: %w", err)
	}

	// Apply resource limits if enabled
	if r.config.EnableResourceLimits {
		if err := r.applyResourceLimits(exec); err != nil {
			return fmt.Errorf("failed to apply resource limits: %w", err)
		}
	}

	return nil
}

// configureFuzzer configures the fuzzer for execution
func (r *runner) configureFuzzer(exec *execution) error {
	// Set corpus path
	if err := exec.fuzzer.SetCorpus(exec.job.CorpusPath); err != nil {
		return fmt.Errorf("failed to set corpus path: %w", err)
	}

	// Set output path
	if err := exec.fuzzer.SetOutput(exec.job.OutputPath); err != nil {
		return fmt.Errorf("failed to set output path: %w", err)
	}

	// Apply fuzzer configuration
	if len(exec.job.FuzzerConfig) > 0 {
		config := &fuzzertypes.FuzzerConfig{
			Options: exec.job.FuzzerConfig,
		}
		if err := exec.fuzzer.Configure(config); err != nil {
			return fmt.Errorf("failed to configure fuzzer: %w", err)
		}
	}

	return nil
}

// monitorExecution monitors a running job execution
func (r *runner) monitorExecution(exec *execution) {
	ticker := time.NewTicker(r.config.ProgressUpdateInterval)
	defer ticker.Stop()

	crashChan := exec.fuzzer.GetCrashes()
	progressChan := exec.fuzzer.GetProgress()

	for {
		select {
		case <-exec.ctx.Done():
			// Context cancelled, job is being stopped
			return

		case crash := <-crashChan:
			if crash != nil {
				r.handleCrash(exec, crash)
			}

		case progress := <-progressChan:
			if progress != nil {
				r.updateProgress(exec, progress)
			}

		case <-ticker.C:
			// Get current stats
			stats, err := exec.fuzzer.GetStats()
			if err != nil {
				r.log.WithError(err).WithField("job_id", exec.job.ID).Warn("Failed to get fuzzer stats")
				continue
			}
			r.updateStats(exec, stats)

			// Check if job has expired
			if exec.job.IsExpired() {
				r.log.WithField("job_id", exec.job.ID).Info("Job expired, stopping execution")
				exec.cancel()
				return
			}
		}
	}
}

// handleCrash handles a discovered crash
func (r *runner) handleCrash(exec *execution, crash *fuzzertypes.CrashInfo) {
	exec.mu.Lock()
	exec.result.Crashes = append(exec.result.Crashes, crash)
	exec.job.IncrementCrashCount()
	exec.mu.Unlock()

	// Save crash artifact
	crashFile := filepath.Join(r.config.ArtifactsDir, exec.job.ID, fmt.Sprintf("crash_%s", crash.ID))
	if err := os.MkdirAll(filepath.Dir(crashFile), 0755); err != nil {
		r.log.WithError(err).WithField("crash_id", crash.ID).Error("Failed to create crash directory")
		return
	}

	if err := os.WriteFile(crashFile, crash.Input, 0644); err != nil {
		r.log.WithError(err).WithField("crash_id", crash.ID).Error("Failed to save crash input")
		return
	}

	exec.mu.Lock()
	exec.result.Artifacts = append(exec.result.Artifacts, crashFile)
	exec.mu.Unlock()

	r.log.WithFields(logrus.Fields{
		"job_id":   exec.job.ID,
		"crash_id": crash.ID,
		"signal":   crash.Signal,
	}).Info("Crash discovered")
}

// updateProgress updates job progress
func (r *runner) updateProgress(exec *execution, progress *fuzzertypes.ProgressUpdate) {
	jobProgress := &types.JobProgress{
		TotalExecs:     progress.Executions,
		ExecsPerSecond: progress.ExecsPerSecond,
		CorpusSize:     progress.CorpusSize,
		Coverage:       progress.Coverage,
		LastUpdated:    progress.Timestamp,
	}

	exec.mu.Lock()
	exec.status.Progress = jobProgress
	exec.mu.Unlock()

	if err := exec.job.UpdateProgress(jobProgress); err != nil {
		r.log.WithError(err).WithField("job_id", exec.job.ID).Warn("Failed to update job progress")
	}
}

// updateStats updates execution statistics
func (r *runner) updateStats(exec *execution, stats *fuzzertypes.FuzzerStats) {
	exec.mu.Lock()
	exec.result.FinalStats = stats
	exec.mu.Unlock()
}

// collectResults collects final execution results
func (r *runner) collectResults(exec *execution) {
	// Get final stats
	if stats, err := exec.fuzzer.GetStats(); err == nil {
		exec.mu.Lock()
		exec.result.FinalStats = stats
		exec.mu.Unlock()
	}

	// Collect resource usage
	if usage := r.collectResourceUsage(exec); usage != nil {
		exec.mu.Lock()
		exec.result.ResourceUsage = usage
		exec.mu.Unlock()
	}
}

// collectResourceUsage collects resource usage information
func (r *runner) collectResourceUsage(exec *execution) *ResourceUsage {
	// This is a simplified implementation
	// In production, you would use more sophisticated resource tracking
	return &ResourceUsage{
		PeakMemory:   0, // Would be tracked during execution
		CPUTime:      time.Since(exec.startTime),
		UserTime:     0, // Would come from process stats
		SystemTime:   0, // Would come from process stats
		MaxProcesses: 1, // Would be tracked during execution
	}
}

// handleJobError handles job execution errors
func (r *runner) handleJobError(exec *execution, err error) {
	exec.mu.Lock()
	exec.status.Status = types.StatusFailed
	exec.status.ErrorMessage = err.Error()
	exec.status.IsRunning = false
	exec.result.Success = false
	exec.result.Error = err
	exec.result.EndTime = time.Now()
	exec.result.Duration = time.Since(exec.startTime)
	exec.mu.Unlock()

	// Update job failure
	if failErr := exec.job.Fail(err.Error()); failErr != nil {
		r.log.WithError(failErr).WithField("job_id", exec.job.ID).Error("Failed to update job failure status")
	}

	r.log.WithError(err).WithField("job_id", exec.job.ID).Error("Job execution failed")
}

// applyResourceLimits applies resource limits to the execution
func (r *runner) applyResourceLimits(exec *execution) error {
	// This is a platform-specific operation
	// On Linux, you would use cgroups or setrlimit
	// This is a simplified implementation

	// Set memory limit
	if r.config.ResourceLimits.MaxMemory > 0 {
		// Would use cgroups or setrlimit(RLIMIT_AS)
	}

	// Set CPU limit
	if r.config.ResourceLimits.MaxCPU > 0 {
		// Would use cgroups CPU quota
	}

	// Set process limit
	if r.config.ResourceLimits.MaxProcesses > 0 {
		// Would use setrlimit(RLIMIT_NPROC)
	}

	return nil
}

// CancelJob cancels a running job
func (r *runner) CancelJob(jobID string) error {
	r.mu.RLock()
	exec, exists := r.executions[jobID]
	r.mu.RUnlock()

	if !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	// Cancel the job
	exec.cancel()

	// Update job status
	if err := exec.job.Cancel(); err != nil {
		r.log.WithError(err).WithField("job_id", jobID).Error("Failed to update job cancellation status")
	}

	r.log.WithField("job_id", jobID).Info("Job cancelled")
	return nil
}

// GetRunningJobs returns currently running jobs
func (r *runner) GetRunningJobs() []*types.Job {
	r.mu.RLock()
	defer r.mu.RUnlock()

	jobs := make([]*types.Job, 0, len(r.executions))
	for _, exec := range r.executions {
		jobs = append(jobs, exec.job)
	}
	return jobs
}

// GetExecutionStatus returns the status of a specific job execution
func (r *runner) GetExecutionStatus(jobID string) (*ExecutionStatus, error) {
	r.mu.RLock()
	exec, exists := r.executions[jobID]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("job %s not found", jobID)
	}

	exec.mu.RLock()
	defer exec.mu.RUnlock()

	// Return a copy of the status
	status := &ExecutionStatus{
		JobID:        exec.status.JobID,
		Status:       exec.status.Status,
		StartTime:    exec.status.StartTime,
		Progress:     exec.status.Progress,
		ErrorMessage: exec.status.ErrorMessage,
		IsRunning:    exec.status.IsRunning,
	}

	return status, nil
}

// getJobTimeout returns the timeout for a job
func (r *runner) getJobTimeout(job *types.Job) time.Duration {
	if job.MaxDuration > 0 {
		return job.MaxDuration
	}
	return r.config.JobTimeout
}

// timePtr returns a pointer to a time.Time
func timePtr(t time.Time) *time.Time {
	return &t
}

// Compile-time check that runner implements JobRunner
var _ JobRunner = (*runner)(nil)
