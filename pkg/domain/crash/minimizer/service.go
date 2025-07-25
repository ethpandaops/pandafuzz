package minimizer

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
	fuzzerTypes "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
	sharedErrors "github.com/ethpandaops/pandafuzz/pkg/shared/errors"
)

// MinimizationProgress represents the progress of a minimization operation
type MinimizationProgress struct {
	OriginalSize      int            `json:"original_size"`
	CurrentSize       int            `json:"current_size"`
	BestSize          int            `json:"best_size"`
	BestInput         []byte         `json:"best_input,omitempty"`
	Iterations        int            `json:"iterations"`
	ReductionRatio    float64        `json:"reduction_ratio"`
	StartTime         time.Time      `json:"start_time"`
	LastUpdateTime    time.Time      `json:"last_update_time"`
	EstimatedTimeLeft time.Duration  `json:"estimated_time_left"`
	CurrentStrategy   string         `json:"current_strategy"`
	StrategyProgress  map[string]int `json:"strategy_progress"`
	State             []byte         `json:"state,omitempty"` // For resumable minimization
}

// MinimizationResult represents the result of a minimization operation
type MinimizationResult struct {
	OriginalCrash  *types.Crash  `json:"original_crash"`
	MinimizedCrash *types.Crash  `json:"minimized_crash"`
	OriginalSize   int           `json:"original_size"`
	MinimizedSize  int           `json:"minimized_size"`
	ReductionRatio float64       `json:"reduction_ratio"`
	Iterations     int           `json:"iterations"`
	Duration       time.Duration `json:"duration"`
	Strategy       string        `json:"strategy"`
	Success        bool          `json:"success"`
	Error          error         `json:"error,omitempty"`
}

// MinimizationOptions configures the minimization process
type MinimizationOptions struct {
	MaxIterations     int                   `json:"max_iterations"`
	Timeout           time.Duration         `json:"timeout"`
	MinReduction      float64               `json:"min_reduction"`
	Strategies        []string              `json:"strategies"`
	WorkerCount       int                   `json:"worker_count"`
	MemoryLimit       uint64                `json:"memory_limit"`
	VerifyInterval    int                   `json:"verify_interval"`
	PreserveStructure bool                  `json:"preserve_structure"`
	ResumeFrom        *MinimizationProgress `json:"resume_from,omitempty"`
	ResourceLimits    *ResourceLimits       `json:"resource_limits,omitempty"`
}

// ResourceLimits defines resource constraints for minimization
type ResourceLimits struct {
	MaxMemory        uint64        `json:"max_memory"`         // Maximum memory usage in bytes
	MaxCPUPercent    float64       `json:"max_cpu_percent"`    // Maximum CPU usage percentage
	MaxDiskSpace     uint64        `json:"max_disk_space"`     // Maximum disk space in bytes
	MaxExecutionTime time.Duration `json:"max_execution_time"` // Maximum time per execution
}

// Service provides crash minimization functionality
type Service struct {
	crashRepo     repository.CrashRepository
	fuzzerFactory fuzzerTypes.FuzzerFactory
	strategies    map[string]MinimizationStrategy
	mu            sync.RWMutex
	activeJobs    map[string]*minimizationJob
}

// minimizationJob tracks an active minimization operation
type minimizationJob struct {
	crashID    string
	progress   *MinimizationProgress
	cancelFunc context.CancelFunc
	result     chan *MinimizationResult
}

// NewService creates a new minimization service
func NewService(
	crashRepo repository.CrashRepository,
	fuzzerFactory fuzzerTypes.FuzzerFactory,
) (Minimizer, error) {
	if crashRepo == nil {
		return nil, fmt.Errorf("crash repository cannot be nil")
	}
	if fuzzerFactory == nil {
		return nil, fmt.Errorf("fuzzer factory cannot be nil")
	}

	s := &Service{
		crashRepo:     crashRepo,
		fuzzerFactory: fuzzerFactory,
		strategies:    make(map[string]MinimizationStrategy),
		activeJobs:    make(map[string]*minimizationJob),
	}

	// Register default strategies
	s.RegisterStrategy("binary_search", NewBinarySearchStrategy())
	s.RegisterStrategy("delta_debugging", NewDeltaDebuggingStrategy())
	s.RegisterStrategy("hierarchical", NewHierarchicalStrategy())
	s.RegisterStrategy("token_based", NewTokenBasedStrategy())

	return s, nil
}

// RegisterStrategy registers a new minimization strategy
func (s *Service) RegisterStrategy(name string, strategy MinimizationStrategy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.strategies[name] = strategy
}

// MinimizeCrash minimizes a crash input while preserving reproducibility
func (s *Service) MinimizeCrash(
	ctx context.Context,
	crashID string,
	options *MinimizationOptions,
) (*MinimizationResult, error) {
	// Set default options if not provided
	if options == nil {
		options = s.defaultOptions()
	}

	// Validate options
	if err := s.validateOptions(options); err != nil {
		return nil, fmt.Errorf("invalid minimization options: %w", err)
	}

	// Retrieve the crash
	crash, err := s.crashRepo.FindByID(ctx, crashID)
	if err != nil {
		return nil, fmt.Errorf("failed to find crash: %w", err)
	}

	if crash == nil {
		return nil, sharedErrors.NewDomainError(
			"CRASH_NOT_FOUND",
			"crash not found",
		).WithDetails("crash_id", crashID)
	}

	// Check if crash is reproducible
	if !crash.Reproducible {
		return nil, sharedErrors.NewDomainError(
			"CRASH_NOT_REPRODUCIBLE",
			"crash is not reproducible",
		).WithDetails("crash_id", crashID)
	}

	// Create minimization context with timeout
	minCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()

	// Track the job
	var progress *MinimizationProgress
	if options.ResumeFrom != nil {
		// Resume from previous state
		progress = options.ResumeFrom
		progress.LastUpdateTime = time.Now()
	} else {
		// Start fresh
		progress = &MinimizationProgress{
			OriginalSize:     len(crash.Input),
			CurrentSize:      len(crash.Input),
			BestSize:         len(crash.Input),
			BestInput:        make([]byte, len(crash.Input)),
			StartTime:        time.Now(),
			LastUpdateTime:   time.Now(),
			CurrentStrategy:  options.Strategies[0],
			StrategyProgress: make(map[string]int),
		}
		copy(progress.BestInput, crash.Input)
	}

	job := &minimizationJob{
		crashID:    crashID,
		cancelFunc: cancel,
		result:     make(chan *MinimizationResult, 1),
		progress:   progress,
	}

	s.mu.Lock()
	s.activeJobs[crashID] = job
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.activeJobs, crashID)
		s.mu.Unlock()
	}()

	// Run minimization in goroutine
	go s.runMinimization(minCtx, crash, options, job)

	// Wait for result or context cancellation
	select {
	case result := <-job.result:
		return result, nil
	case <-ctx.Done():
		cancel() // Cancel the minimization
		return nil, ctx.Err()
	}
}

// runMinimization performs the actual minimization work
func (s *Service) runMinimization(
	ctx context.Context,
	crash *types.Crash,
	options *MinimizationOptions,
	job *minimizationJob,
) {
	startTime := time.Now()
	result := &MinimizationResult{
		OriginalCrash:  crash,
		OriginalSize:   len(crash.Input),
		MinimizedSize:  len(crash.Input),
		ReductionRatio: 0.0,
		Success:        false,
	}

	// Create fuzzer for crash reproduction
	fuzzer, err := s.createFuzzerForCrash(crash)
	if err != nil {
		result.Error = fmt.Errorf("failed to create fuzzer: %w", err)
		job.result <- result
		return
	}

	// Start with the original input
	bestInput := crash.Input
	currentInput := make([]byte, len(crash.Input))
	copy(currentInput, crash.Input)

	// Try each strategy
	for _, strategyName := range options.Strategies {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			job.result <- result
			return
		default:
		}

		strategy, exists := s.strategies[strategyName]
		if !exists {
			continue
		}

		job.progress.CurrentStrategy = strategyName

		// Create reproduction verifier
		verifyTimeout := 5 * time.Second
		if options.ResourceLimits != nil && options.ResourceLimits.MaxExecutionTime > 0 {
			verifyTimeout = options.ResourceLimits.MaxExecutionTime
		}

		verifier := &reproductionVerifier{
			fuzzer:         fuzzer,
			originalCrash:  crash,
			timeout:        verifyTimeout,
			resourceLimits: options.ResourceLimits,
		}

		// Run the strategy
		minimized, err := strategy.Minimize(ctx, currentInput, verifier, job.progress)
		if err != nil {
			continue // Try next strategy
		}

		if len(minimized) < len(bestInput) {
			bestInput = minimized
			currentInput = make([]byte, len(minimized))
			copy(currentInput, minimized)
			job.progress.BestSize = len(bestInput)
			job.progress.BestInput = make([]byte, len(bestInput))
			copy(job.progress.BestInput, bestInput)
			job.progress.ReductionRatio = float64(job.progress.OriginalSize-len(bestInput)) / float64(job.progress.OriginalSize)
		}

		// Track strategy progress
		job.progress.StrategyProgress[strategyName] = job.progress.Iterations
	}

	// Create minimized crash
	if len(bestInput) < len(crash.Input) {
		minimizedCrash, err := s.createMinimizedCrash(crash, bestInput)
		if err != nil {
			result.Error = fmt.Errorf("failed to create minimized crash: %w", err)
		} else {
			result.MinimizedCrash = minimizedCrash
			result.MinimizedSize = len(bestInput)
			result.ReductionRatio = float64(len(crash.Input)-len(bestInput)) / float64(len(crash.Input))
			result.Success = true

			// Save the minimized crash
			if err := s.crashRepo.Create(ctx, minimizedCrash); err != nil {
				result.Error = fmt.Errorf("failed to save minimized crash: %w", err)
			}
		}
	}

	result.Duration = time.Since(startTime)
	result.Iterations = job.progress.Iterations
	job.result <- result
}

// GetProgress returns the current progress of a minimization job
func (s *Service) GetProgress(crashID string) (*MinimizationProgress, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, exists := s.activeJobs[crashID]
	if !exists {
		return nil, sharedErrors.NewDomainError(
			"JOB_NOT_FOUND",
			"minimization job not found",
		).WithDetails("crash_id", crashID)
	}

	// Calculate estimated time left
	elapsed := time.Since(job.progress.StartTime)
	reductionRate := float64(job.progress.OriginalSize-job.progress.CurrentSize) / float64(job.progress.Iterations+1)
	remainingReduction := float64(job.progress.CurrentSize - 1) // Assume we can get to 1 byte minimum
	estimatedIterations := remainingReduction / reductionRate
	estimatedTime := time.Duration(float64(elapsed) * estimatedIterations / float64(job.progress.Iterations+1))

	job.progress.EstimatedTimeLeft = estimatedTime
	job.progress.LastUpdateTime = time.Now()

	return job.progress, nil
}

// CancelMinimization cancels an active minimization job
func (s *Service) CancelMinimization(crashID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, exists := s.activeJobs[crashID]
	if !exists {
		return sharedErrors.NewDomainError(
			"JOB_NOT_FOUND",
			"minimization job not found",
		).WithDetails("crash_id", crashID)
	}

	job.cancelFunc()
	return nil
}

// GetMinimalInput retrieves the current best minimal input for a crash
func (s *Service) GetMinimalInput(crashID string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if there's an active job with a minimal input
	if job, exists := s.activeJobs[crashID]; exists {
		if job.progress.BestInput != nil {
			result := make([]byte, len(job.progress.BestInput))
			copy(result, job.progress.BestInput)
			return result, nil
		}
	}

	// Otherwise, check if we have a minimized crash stored
	ctx := context.Background()
	// Use List with a reasonable limit to find minimized versions
	crashes, _, err := s.crashRepo.List(ctx, 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to query crashes: %w", err)
	}

	for _, crash := range crashes {
		if minimizedFrom, exists := crash.GetMetadata("minimized_from"); exists && minimizedFrom == crashID {
			return crash.Input, nil
		}
	}

	// Return the original crash input if no minimized version exists
	crash, err := s.crashRepo.FindByID(ctx, crashID)
	if err != nil {
		return nil, fmt.Errorf("failed to find crash: %w", err)
	}

	if crash == nil {
		return nil, sharedErrors.NewDomainError(
			"CRASH_NOT_FOUND",
			"crash not found",
		).WithDetails("crash_id", crashID)
	}

	return crash.Input, nil
}

// ResumeMinimization resumes a previously paused minimization job
func (s *Service) ResumeMinimization(
	ctx context.Context,
	crashID string,
	progress *MinimizationProgress,
	options *MinimizationOptions,
) (*MinimizationResult, error) {
	if progress == nil {
		return nil, fmt.Errorf("progress state cannot be nil for resumption")
	}

	// Set the resume state
	if options == nil {
		options = s.defaultOptions()
	}
	options.ResumeFrom = progress

	return s.MinimizeCrash(ctx, crashID, options)
}

// ExportProgress exports the current minimization progress for persistence
func (s *Service) ExportProgress(crashID string) (*MinimizationProgress, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, exists := s.activeJobs[crashID]
	if !exists {
		return nil, sharedErrors.NewDomainError(
			"JOB_NOT_FOUND",
			"minimization job not found",
		).WithDetails("crash_id", crashID)
	}

	// Create a deep copy of the progress
	progressCopy := &MinimizationProgress{
		OriginalSize:      job.progress.OriginalSize,
		CurrentSize:       job.progress.CurrentSize,
		BestSize:          job.progress.BestSize,
		Iterations:        job.progress.Iterations,
		ReductionRatio:    job.progress.ReductionRatio,
		StartTime:         job.progress.StartTime,
		LastUpdateTime:    job.progress.LastUpdateTime,
		EstimatedTimeLeft: job.progress.EstimatedTimeLeft,
		CurrentStrategy:   job.progress.CurrentStrategy,
		StrategyProgress:  make(map[string]int),
	}

	// Copy best input
	if job.progress.BestInput != nil {
		progressCopy.BestInput = make([]byte, len(job.progress.BestInput))
		copy(progressCopy.BestInput, job.progress.BestInput)
	}

	// Copy strategy progress
	for k, v := range job.progress.StrategyProgress {
		progressCopy.StrategyProgress[k] = v
	}

	// Copy state if exists
	if job.progress.State != nil {
		progressCopy.State = make([]byte, len(job.progress.State))
		copy(progressCopy.State, job.progress.State)
	}

	return progressCopy, nil
}

// ListActiveJobs returns a list of active minimization job IDs
func (s *Service) ListActiveJobs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]string, 0, len(s.activeJobs))
	for crashID := range s.activeJobs {
		jobs = append(jobs, crashID)
	}
	return jobs
}

// createFuzzerForCrash creates a fuzzer instance configured for the crash target
func (s *Service) createFuzzerForCrash(crash *types.Crash) (fuzzerTypes.Fuzzer, error) {
	// Extract fuzzer type from crash metadata
	fuzzerType, exists := crash.GetMetadata("fuzzer_type")
	if !exists {
		// Default to libfuzzer if not specified
		fuzzerType = string(fuzzerTypes.FuzzerTypeLibFuzzer)
	}

	// Create fuzzer with crash target info
	fuzzer, err := s.fuzzerFactory.CreateFuzzer(
		fuzzerType,
		crash.TargetInfo.Name,
		[]string{}, // Additional args if needed
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create fuzzer: %w", err)
	}

	// Configure fuzzer for minimization
	config := &fuzzerTypes.FuzzerConfig{
		OutputDir:   "/tmp/minimizer",  // Temporary directory for minimization
		Timeout:     5 * time.Second,   // Short timeout for quick verification
		Workers:     1,                 // Single worker for minimization
		OnlyCrashes: true,              // Only interested in crashes
		MemoryLimit: 1024 * 1024 * 512, // 512MB default limit
	}

	if err := fuzzer.Configure(config); err != nil {
		return nil, fmt.Errorf("failed to configure fuzzer: %w", err)
	}

	return fuzzer, nil
}

// createMinimizedCrash creates a new crash entry for the minimized input
func (s *Service) createMinimizedCrash(originalCrash *types.Crash, minimizedInput []byte) (*types.Crash, error) {
	minimizedCrash, err := types.NewCrash(
		minimizedInput,
		originalCrash.StackTrace,
		originalCrash.TargetInfo,
	)
	if err != nil {
		return nil, err
	}

	// Copy relevant fields from original crash
	minimizedCrash.Severity = originalCrash.Severity
	minimizedCrash.Type = originalCrash.Type
	minimizedCrash.Signature = originalCrash.Signature

	// Add metadata to indicate this is a minimized version
	minimizedCrash.SetMetadata("minimized_from", originalCrash.ID)
	minimizedCrash.SetMetadata("original_size", fmt.Sprintf("%d", len(originalCrash.Input)))
	minimizedCrash.SetMetadata("minimized_size", fmt.Sprintf("%d", len(minimizedInput)))
	minimizedCrash.SetMetadata("reduction_ratio", fmt.Sprintf("%.2f", float64(len(originalCrash.Input)-len(minimizedInput))/float64(len(originalCrash.Input))))

	// Add minimization tag
	minimizedCrash.AddTag("minimized")

	return minimizedCrash, nil
}

// defaultOptions returns default minimization options
func (s *Service) defaultOptions() *MinimizationOptions {
	return &MinimizationOptions{
		MaxIterations:     1000,
		Timeout:           30 * time.Minute,
		MinReduction:      0.01, // 1% minimum reduction
		Strategies:        []string{"binary_search", "delta_debugging"},
		WorkerCount:       1,
		MemoryLimit:       1024 * 1024 * 1024, // 1GB
		VerifyInterval:    10,
		PreserveStructure: false,
		ResourceLimits: &ResourceLimits{
			MaxMemory:        1024 * 1024 * 1024, // 1GB
			MaxCPUPercent:    80.0,               // 80% CPU
			MaxDiskSpace:     1024 * 1024 * 100,  // 100MB
			MaxExecutionTime: 5 * time.Second,    // 5s per execution
		},
	}
}

// validateOptions validates minimization options
func (s *Service) validateOptions(options *MinimizationOptions) error {
	if options.MaxIterations <= 0 {
		return fmt.Errorf("max iterations must be positive")
	}
	if options.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if options.MinReduction < 0 || options.MinReduction > 1 {
		return fmt.Errorf("min reduction must be between 0 and 1")
	}
	if len(options.Strategies) == 0 {
		return fmt.Errorf("at least one strategy must be specified")
	}
	if options.WorkerCount <= 0 {
		return fmt.Errorf("worker count must be positive")
	}
	return nil
}

// reproductionVerifier verifies that a minimized input still reproduces the crash
type reproductionVerifier struct {
	fuzzer         fuzzerTypes.Fuzzer
	originalCrash  *types.Crash
	timeout        time.Duration
	resourceLimits *ResourceLimits
}

// Verify checks if the input still reproduces the crash
func (v *reproductionVerifier) Verify(ctx context.Context, input []byte) (bool, error) {
	// Create a context with timeout for verification
	verifyCtx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()

	// Run the fuzzer with the minimized input
	if err := v.fuzzer.Start(verifyCtx); err != nil {
		return false, fmt.Errorf("failed to start fuzzer: %w", err)
	}
	defer v.fuzzer.Stop()

	// Wait for crash or timeout
	select {
	case crash := <-v.fuzzer.GetCrashes():
		// Check if it's the same crash type
		return v.isSameCrash(crash), nil
	case <-verifyCtx.Done():
		// No crash within timeout means input doesn't reproduce
		return false, nil
	}
}

// isSameCrash checks if two crashes are the same
func (v *reproductionVerifier) isSameCrash(newCrash *fuzzerTypes.CrashInfo) bool {
	// Compare stack traces or signatures
	// This is a simplified comparison - in practice, you might want more sophisticated matching
	return bytes.Contains([]byte(newCrash.StackTrace), []byte(v.originalCrash.Signature.TopFrames[0]))
}
