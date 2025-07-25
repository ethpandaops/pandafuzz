package executor

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/sirupsen/logrus"
)

// TimeoutManager manages job execution timeouts and cancellation
type TimeoutManager interface {
	// RegisterTimeout registers a job timeout
	RegisterTimeout(jobID string, timeout time.Duration, cancel context.CancelFunc) error

	// ExtendTimeout extends a job's timeout
	ExtendTimeout(jobID string, extension time.Duration) error

	// CancelTimeout cancels a job's timeout tracking
	CancelTimeout(jobID string)

	// CheckTimeout checks if a job has timed out
	CheckTimeout(jobID string) (timedOut bool, remaining time.Duration)

	// RegisterProcess associates a process with a job for termination
	RegisterProcess(jobID string, process *os.Process) error

	// Start starts the timeout manager
	Start(ctx context.Context) error

	// Stop stops the timeout manager
	Stop() error
}

// timeoutManager implements TimeoutManager interface
type timeoutManager struct {
	mu            sync.RWMutex
	log           logrus.FieldLogger
	entries       map[string]*timeoutEntry
	checkInterval time.Duration
	gracePeriod   time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// timeoutEntry tracks timeout information for a job
type timeoutEntry struct {
	jobID    string
	deadline time.Time
	cancel   context.CancelFunc
	process  *os.Process
	mu       sync.Mutex
}

// TimeoutConfig provides configuration for timeout management
type TimeoutConfig struct {
	// CheckInterval is how often to check for timeouts
	CheckInterval time.Duration

	// GracePeriod is how long to wait for graceful shutdown
	GracePeriod time.Duration
}

// NewTimeoutManager creates a new timeout manager
func NewTimeoutManager(config *TimeoutConfig, log logrus.FieldLogger) TimeoutManager {
	if config == nil {
		config = &TimeoutConfig{
			CheckInterval: 10 * time.Second,
			GracePeriod:   30 * time.Second,
		}
	}

	return &timeoutManager{
		log:           log.WithField("component", "timeout-manager"),
		entries:       make(map[string]*timeoutEntry),
		checkInterval: config.CheckInterval,
		gracePeriod:   config.GracePeriod,
	}
}

// Start starts the timeout manager
func (tm *timeoutManager) Start(ctx context.Context) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.ctx != nil {
		return fmt.Errorf("timeout manager already started")
	}

	tm.ctx, tm.cancel = context.WithCancel(ctx)

	// Start timeout checker
	tm.wg.Add(1)
	go tm.timeoutChecker()

	tm.log.Info("Timeout manager started")
	return nil
}

// Stop stops the timeout manager
func (tm *timeoutManager) Stop() error {
	tm.mu.Lock()
	if tm.cancel != nil {
		tm.cancel()
	}
	tm.mu.Unlock()

	// Wait for checker to stop
	tm.wg.Wait()

	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Cancel all remaining timeouts
	for _, entry := range tm.entries {
		if entry.cancel != nil {
			entry.cancel()
		}
	}

	tm.log.Info("Timeout manager stopped")
	return nil
}

// RegisterTimeout registers a job timeout
func (tm *timeoutManager) RegisterTimeout(jobID string, timeout time.Duration, cancel context.CancelFunc) error {
	if jobID == "" {
		return fmt.Errorf("job ID cannot be empty")
	}
	if timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if cancel == nil {
		return fmt.Errorf("cancel function cannot be nil")
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Check if already registered
	if _, exists := tm.entries[jobID]; exists {
		return fmt.Errorf("timeout already registered for job %s", jobID)
	}

	entry := &timeoutEntry{
		jobID:    jobID,
		deadline: time.Now().Add(timeout),
		cancel:   cancel,
	}

	tm.entries[jobID] = entry

	tm.log.WithFields(logrus.Fields{
		"job_id":   jobID,
		"timeout":  timeout,
		"deadline": entry.deadline,
	}).Debug("Registered job timeout")

	return nil
}

// ExtendTimeout extends a job's timeout
func (tm *timeoutManager) ExtendTimeout(jobID string, extension time.Duration) error {
	if extension <= 0 {
		return fmt.Errorf("extension must be positive")
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	entry, exists := tm.entries[jobID]
	if !exists {
		return fmt.Errorf("no timeout registered for job %s", jobID)
	}

	entry.deadline = entry.deadline.Add(extension)

	tm.log.WithFields(logrus.Fields{
		"job_id":       jobID,
		"extension":    extension,
		"new_deadline": entry.deadline,
	}).Info("Extended job timeout")

	return nil
}

// CancelTimeout cancels a job's timeout tracking
func (tm *timeoutManager) CancelTimeout(jobID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	delete(tm.entries, jobID)

	tm.log.WithField("job_id", jobID).Debug("Cancelled job timeout")
}

// CheckTimeout checks if a job has timed out
func (tm *timeoutManager) CheckTimeout(jobID string) (bool, time.Duration) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	entry, exists := tm.entries[jobID]
	if !exists {
		return false, 0
	}

	remaining := time.Until(entry.deadline)
	return remaining <= 0, remaining
}

// RegisterProcess associates a process with a job for termination
func (tm *timeoutManager) RegisterProcess(jobID string, process *os.Process) error {
	if process == nil {
		return fmt.Errorf("process cannot be nil")
	}

	tm.mu.RLock()
	entry, exists := tm.entries[jobID]
	tm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no timeout registered for job %s", jobID)
	}

	entry.mu.Lock()
	entry.process = process
	entry.mu.Unlock()

	tm.log.WithFields(logrus.Fields{
		"job_id": jobID,
		"pid":    process.Pid,
	}).Debug("Registered process for job")

	return nil
}

// timeoutChecker periodically checks for timed out jobs
func (tm *timeoutManager) timeoutChecker() {
	defer tm.wg.Done()

	ticker := time.NewTicker(tm.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tm.ctx.Done():
			return
		case <-ticker.C:
			tm.checkTimeouts()
		}
	}
}

// checkTimeouts checks all registered timeouts
func (tm *timeoutManager) checkTimeouts() {
	now := time.Now()

	tm.mu.Lock()
	// Collect timed out entries
	timedOut := make([]*timeoutEntry, 0)
	for jobID, entry := range tm.entries {
		if now.After(entry.deadline) {
			timedOut = append(timedOut, entry)
			delete(tm.entries, jobID)
		}
	}
	tm.mu.Unlock()

	// Handle timed out jobs
	for _, entry := range timedOut {
		tm.handleTimeout(entry)
	}
}

// handleTimeout handles a single timeout
func (tm *timeoutManager) handleTimeout(entry *timeoutEntry) {
	tm.log.WithField("job_id", entry.jobID).Warn("Job execution timed out")

	// Cancel the context
	if entry.cancel != nil {
		entry.cancel()
	}

	// Terminate process if registered
	entry.mu.Lock()
	process := entry.process
	entry.mu.Unlock()

	if process != nil {
		tm.terminateProcess(entry.jobID, process)
	}
}

// terminateProcess attempts to terminate a process gracefully, then forcefully
func (tm *timeoutManager) terminateProcess(jobID string, process *os.Process) {
	tm.log.WithFields(logrus.Fields{
		"job_id": jobID,
		"pid":    process.Pid,
	}).Info("Terminating process")

	// Try graceful termination first
	if err := process.Signal(syscall.SIGTERM); err != nil {
		tm.log.WithError(err).WithField("pid", process.Pid).Warn("Failed to send SIGTERM, killing process")
		tm.killProcess(process)
		return
	}

	// Wait for graceful shutdown
	done := make(chan struct{})
	go func() {
		_, _ = process.Wait()
		close(done)
	}()

	select {
	case <-done:
		tm.log.WithField("pid", process.Pid).Info("Process terminated gracefully")
	case <-time.After(tm.gracePeriod):
		tm.log.WithField("pid", process.Pid).Warn("Process did not terminate gracefully, forcing kill")
		tm.killProcess(process)
	}
}

// killProcess forcefully kills a process
func (tm *timeoutManager) killProcess(process *os.Process) {
	if err := process.Kill(); err != nil {
		tm.log.WithError(err).WithField("pid", process.Pid).Error("Failed to kill process")
	}
}

// CreateTimeoutContext creates a context with timeout management
func CreateTimeoutContext(ctx context.Context, job *types.Job, tm TimeoutManager) (context.Context, context.CancelFunc, error) {
	// Determine timeout
	timeout := job.MaxDuration
	if timeout <= 0 {
		timeout = 24 * time.Hour // Default timeout
	}

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, timeout)

	// Register with timeout manager
	if err := tm.RegisterTimeout(job.ID, timeout, cancel); err != nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to register timeout: %w", err)
	}

	// Wrap cancel function to also clean up timeout registration
	wrappedCancel := func() {
		cancel()
		tm.CancelTimeout(job.ID)
	}

	return ctx, wrappedCancel, nil
}

// TimeoutStats provides statistics about managed timeouts
type TimeoutStats struct {
	ActiveTimeouts int                    `json:"active_timeouts"`
	Timeouts       map[string]TimeoutInfo `json:"timeouts"`
}

// TimeoutInfo provides information about a specific timeout
type TimeoutInfo struct {
	JobID      string        `json:"job_id"`
	Deadline   time.Time     `json:"deadline"`
	Remaining  time.Duration `json:"remaining"`
	HasProcess bool          `json:"has_process"`
}

// GetStats returns timeout statistics
func (tm *timeoutManager) GetStats() TimeoutStats {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	stats := TimeoutStats{
		ActiveTimeouts: len(tm.entries),
		Timeouts:       make(map[string]TimeoutInfo),
	}

	now := time.Now()
	for jobID, entry := range tm.entries {
		entry.mu.Lock()
		hasProcess := entry.process != nil
		entry.mu.Unlock()

		stats.Timeouts[jobID] = TimeoutInfo{
			JobID:      jobID,
			Deadline:   entry.deadline,
			Remaining:  time.Until(entry.deadline),
			HasProcess: hasProcess,
		}
	}

	return stats
}

// TimeoutMiddleware provides timeout enforcement for job execution
type TimeoutMiddleware struct {
	runner         JobRunner
	timeoutManager TimeoutManager
	defaultTimeout time.Duration
}

// NewTimeoutMiddleware creates a new timeout middleware
func NewTimeoutMiddleware(runner JobRunner, tm TimeoutManager, defaultTimeout time.Duration) *TimeoutMiddleware {
	if defaultTimeout <= 0 {
		defaultTimeout = 24 * time.Hour
	}

	return &TimeoutMiddleware{
		runner:         runner,
		timeoutManager: tm,
		defaultTimeout: defaultTimeout,
	}
}

// ExecuteJob executes a job with timeout enforcement
func (m *TimeoutMiddleware) ExecuteJob(ctx context.Context, job *types.Job) (*ExecutionResult, error) {
	// Set default timeout if not specified
	if job.MaxDuration <= 0 {
		job.MaxDuration = m.defaultTimeout
	}

	// Create timeout context
	timeoutCtx, cancel, err := CreateTimeoutContext(ctx, job, m.timeoutManager)
	if err != nil {
		return nil, fmt.Errorf("failed to create timeout context: %w", err)
	}
	defer cancel()

	// Execute with timeout
	result, execErr := m.runner.ExecuteJob(timeoutCtx, job)

	// Check if timeout occurred
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return &ExecutionResult{
			JobID:     job.ID,
			Success:   false,
			Error:     fmt.Errorf("job execution timed out after %v", job.MaxDuration),
			StartTime: time.Now(),
			EndTime:   time.Now(),
			ExitCode:  -1,
		}, fmt.Errorf("job execution timed out after %v", job.MaxDuration)
	}

	return result, execErr
}

// CancelJob cancels a running job
func (m *TimeoutMiddleware) CancelJob(jobID string) error {
	// Cancel timeout tracking
	m.timeoutManager.CancelTimeout(jobID)

	// Cancel the actual job
	return m.runner.CancelJob(jobID)
}

// GetRunningJobs returns currently running jobs
func (m *TimeoutMiddleware) GetRunningJobs() []*types.Job {
	return m.runner.GetRunningJobs()
}

// GetExecutionStatus returns the status of a specific job execution
func (m *TimeoutMiddleware) GetExecutionStatus(jobID string) (*ExecutionStatus, error) {
	return m.runner.GetExecutionStatus(jobID)
}

// Start starts the timeout middleware
func (m *TimeoutMiddleware) Start(ctx context.Context) error {
	// Start timeout manager
	if err := m.timeoutManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start timeout manager: %w", err)
	}

	// Start underlying runner
	return m.runner.Start(ctx)
}

// Stop stops the timeout middleware
func (m *TimeoutMiddleware) Stop() error {
	// Stop timeout manager
	if err := m.timeoutManager.Stop(); err != nil {
		return fmt.Errorf("failed to stop timeout manager: %w", err)
	}

	// Stop underlying runner
	return m.runner.Stop()
}

// Compile-time check that TimeoutMiddleware implements JobRunner
var _ JobRunner = (*TimeoutMiddleware)(nil)
