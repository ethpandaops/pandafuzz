package executor

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
	jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
)

// Executor defines the interface for executing work on bots
type Executor interface {
	// Execute runs a job on the specified bot
	Execute(ctx context.Context, bot *types.Agent, job *jobtypes.Job) error

	// Cancel cancels a running job
	Cancel(ctx context.Context, botID, jobID string) error

	// GetStatus returns the current execution status
	GetStatus(ctx context.Context, botID, jobID string) (*ExecutionStatus, error)

	// GetCapabilities returns the capabilities required by this executor
	GetCapabilities() []types.Capability

	// ValidateJob validates if a job can be executed by this executor
	ValidateJob(job *jobtypes.Job) error
}

// ExecutionStatus represents the status of a job execution
type ExecutionStatus struct {
	BotID      string                 `json:"bot_id"`
	JobID      string                 `json:"job_id"`
	Status     jobtypes.JobStatus     `json:"status"`
	StartTime  time.Time              `json:"start_time"`
	LastUpdate time.Time              `json:"last_update"`
	Progress   *jobtypes.JobProgress  `json:"progress,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// ExecutorManager manages multiple executors
type ExecutorManager interface {
	// RegisterExecutor registers an executor for a specific job type
	RegisterExecutor(jobType string, executor Executor) error

	// GetExecutor retrieves an executor for a job type
	GetExecutor(jobType string) (Executor, error)

	// ExecuteJob executes a job using the appropriate executor
	ExecuteJob(ctx context.Context, bot *types.Agent, job *jobtypes.Job) error

	// ListExecutors returns all registered executors
	ListExecutors() map[string]Executor
}

// ExecutorConfig provides configuration for executors
type ExecutorConfig struct {
	// MaxConcurrentJobs limits concurrent jobs per bot
	MaxConcurrentJobs int

	// JobTimeout is the default timeout for jobs
	JobTimeout time.Duration

	// HeartbeatInterval for job progress updates
	HeartbeatInterval time.Duration

	// RetryPolicy defines retry behavior
	RetryPolicy *RetryPolicy

	// ResourceLimits defines resource constraints
	ResourceLimits *ResourceLimits
}

// RetryPolicy defines retry behavior for failed executions
type RetryPolicy struct {
	MaxAttempts     int           `json:"max_attempts"`
	InitialDelay    time.Duration `json:"initial_delay"`
	MaxDelay        time.Duration `json:"max_delay"`
	BackoffFactor   float64       `json:"backoff_factor"`
	RetryableErrors []string      `json:"retryable_errors"`
}

// ResourceLimits defines resource constraints for execution
type ResourceLimits struct {
	MaxCPU       float64 `json:"max_cpu"`       // CPU cores
	MaxMemory    uint64  `json:"max_memory"`    // Bytes
	MaxDisk      uint64  `json:"max_disk"`      // Bytes
	MaxProcesses int     `json:"max_processes"` // Number of processes
}

// ExecutionContext provides context for job execution
type ExecutionContext struct {
	Bot       *types.Agent
	Job       *jobtypes.Job
	Config    *ExecutorConfig
	StartTime time.Time
	CancelFn  context.CancelFunc
}

// ExecutionResult represents the result of a job execution
type ExecutionResult struct {
	Success       bool                   `json:"success"`
	Error         error                  `json:"error,omitempty"`
	Duration      time.Duration          `json:"duration"`
	ExitCode      int                    `json:"exit_code"`
	Output        string                 `json:"output,omitempty"`
	Artifacts     []string               `json:"artifacts,omitempty"`
	Metrics       map[string]interface{} `json:"metrics,omitempty"`
	ResourceUsage *ResourceUsage         `json:"resource_usage,omitempty"`
}

// ResourceUsage tracks resource consumption during execution
type ResourceUsage struct {
	CPUTime     time.Duration `json:"cpu_time"`
	MaxMemory   uint64        `json:"max_memory"`
	DiskWritten uint64        `json:"disk_written"`
	NetworkSent uint64        `json:"network_sent"`
	NetworkRecv uint64        `json:"network_recv"`
}

// ExecutorHooks provides lifecycle hooks for execution
type ExecutorHooks interface {
	// OnExecutionStart is called before execution begins
	OnExecutionStart(ctx *ExecutionContext) error

	// OnExecutionComplete is called after execution completes
	OnExecutionComplete(ctx *ExecutionContext, result *ExecutionResult) error

	// OnExecutionError is called when execution fails
	OnExecutionError(ctx *ExecutionContext, err error) error

	// OnProgressUpdate is called on progress updates
	OnProgressUpdate(ctx *ExecutionContext, progress *jobtypes.JobProgress) error
}

// BaseExecutor provides common functionality for executors
type BaseExecutor struct {
	config     *ExecutorConfig
	eventPub   types.BotEventPublisher
	hooks      ExecutorHooks
	executions map[string]*ExecutionContext
}

// NewBaseExecutor creates a new base executor
func NewBaseExecutor(config *ExecutorConfig, eventPub types.BotEventPublisher, hooks ExecutorHooks) *BaseExecutor {
	if config == nil {
		config = &ExecutorConfig{
			MaxConcurrentJobs: 1,
			JobTimeout:        1 * time.Hour,
			HeartbeatInterval: 30 * time.Second,
		}
	}

	return &BaseExecutor{
		config:     config,
		eventPub:   eventPub,
		hooks:      hooks,
		executions: make(map[string]*ExecutionContext),
	}
}

// StoreExecution stores an execution context
func (be *BaseExecutor) StoreExecution(botID, jobID string, ctx *ExecutionContext) {
	key := botID + ":" + jobID
	be.executions[key] = ctx
}

// GetExecution retrieves an execution context
func (be *BaseExecutor) GetExecution(botID, jobID string) (*ExecutionContext, bool) {
	key := botID + ":" + jobID
	ctx, exists := be.executions[key]
	return ctx, exists
}

// RemoveExecution removes an execution context
func (be *BaseExecutor) RemoveExecution(botID, jobID string) {
	key := botID + ":" + jobID
	delete(be.executions, key)
}

// PublishEvent publishes a bot event
func (be *BaseExecutor) PublishEvent(event types.BotEvent) error {
	if be.eventPub != nil {
		return be.eventPub.PublishEvent(event)
	}
	return nil
}

// GetConfig returns the executor configuration
func (be *BaseExecutor) GetConfig() *ExecutorConfig {
	return be.config
}

// ExecutorError represents an executor-specific error
type ExecutorError struct {
	Type      string                 `json:"type"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Retryable bool                   `json:"retryable"`
}

// Error implements the error interface
func (e *ExecutorError) Error() string {
	return e.Message
}

// NewExecutorError creates a new executor error
func NewExecutorError(errType, message string, retryable bool) *ExecutorError {
	return &ExecutorError{
		Type:      errType,
		Message:   message,
		Retryable: retryable,
		Details:   make(map[string]interface{}),
	}
}

// WithDetail adds a detail to the error
func (e *ExecutorError) WithDetail(key string, value interface{}) *ExecutorError {
	e.Details[key] = value
	return e
}
