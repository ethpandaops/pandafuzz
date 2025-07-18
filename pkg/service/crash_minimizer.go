package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// MinimizationStrategy represents the strategy to use for test case minimization
type MinimizationStrategy string

const (
	MinimizationStrategyDeltaDebug     MinimizationStrategy = "delta_debug"
	MinimizationStrategyCoverageGuided MinimizationStrategy = "coverage_guided"
	MinimizationStrategyBinarySearch   MinimizationStrategy = "binary_search"
	MinimizationStrategyHierarchical   MinimizationStrategy = "hierarchical"
)

// MinimizationConfig contains configuration for minimization
type MinimizationConfig struct {
	MaxIterations    int           `json:"max_iterations"`
	MaxTime          time.Duration `json:"max_time"`
	MinChunkSize     int           `json:"min_chunk_size"`
	TargetBinary     string        `json:"target_binary"`
	BinaryArgs       []string      `json:"binary_args"`
	ExpectedSignal   int           `json:"expected_signal"`
	ExpectedExitCode int           `json:"expected_exit_code"`
	Timeout          time.Duration `json:"timeout"`
	WorkDir          string        `json:"work_dir"`
}

// MinimizationResultWithData wraps common.MinimizationResult with the actual minimized data
type MinimizationResultWithData struct {
	*common.MinimizationResult
	MinimizedInput []byte `json:"-"` // Not persisted directly
}

// Minimizer defines the interface for crash test case minimizers
type Minimizer interface {
	// Minimize attempts to reduce the size of a crash input while maintaining the crash
	Minimize(ctx context.Context, input []byte, config MinimizationConfig) (*MinimizationResultWithData, error)

	// GetStrategy returns the minimization strategy used
	GetStrategy() MinimizationStrategy
}

// CrashMinimizerService manages crash minimization across different strategies
type CrashMinimizerService interface {
	// Lifecycle methods
	Start(ctx context.Context) error
	Stop() error

	// MinimizeCrash minimizes a crash input using the specified strategy
	MinimizeCrash(ctx context.Context, crashID string, strategy string) (*common.MinimizationResult, error)

	// GetMinimizationResult retrieves a previous minimization result
	GetMinimizationResult(ctx context.Context, resultID string) (*common.MinimizationResult, error)

	// ListMinimizationResults lists minimization results for a crash
	ListMinimizationResults(ctx context.Context, crashID string) ([]*common.MinimizationResult, error)

	// GetBestMinimization returns the best (smallest) minimization for a crash
	GetBestMinimization(ctx context.Context, crashID string) (*common.MinimizationResult, error)
}

// crashMinimizerService implements CrashMinimizerService
type crashMinimizerService struct {
	log         logrus.FieldLogger
	storage     common.Storage
	fileStorage common.FileStorage
	minimizers  map[string]Minimizer
}

// NewCrashMinimizerService creates a new crash minimizer service
func NewCrashMinimizerService(log logrus.FieldLogger, storage common.Storage, fileStorage common.FileStorage) CrashMinimizerService {
	log = log.WithField("service", "crash_minimizer")

	service := &crashMinimizerService{
		log:         log,
		storage:     storage,
		fileStorage: fileStorage,
		minimizers:  make(map[string]Minimizer),
	}

	// Register minimizers
	service.minimizers[string(MinimizationStrategyDeltaDebug)] = NewDeltaDebugMinimizer(log)
	service.minimizers[string(MinimizationStrategyCoverageGuided)] = NewCoverageGuidedMinimizer(log)

	return service
}

func (s *crashMinimizerService) Start(ctx context.Context) error {
	s.log.Info("Starting crash minimizer service")
	return nil
}

func (s *crashMinimizerService) Stop() error {
	s.log.Info("Stopping crash minimizer service")
	return nil
}

func (s *crashMinimizerService) MinimizeCrash(ctx context.Context, crashID string, strategy string) (*common.MinimizationResult, error) {
	// Get the crash details
	crash, err := s.storage.GetCrash(ctx, crashID)
	if err != nil {
		return nil, fmt.Errorf("failed to get crash: %w", err)
	}

	// Get the job details for binary information
	job, err := s.storage.GetJob(ctx, crash.JobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	// Read the crash input
	crashPath := filepath.Join(job.WorkDir, crash.FilePath)
	input, err := s.fileStorage.ReadFile(ctx, crashPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read crash input: %w", err)
	}

	// Get the minimizer
	minimizer, exists := s.minimizers[strategy]
	if !exists {
		return nil, fmt.Errorf("unknown minimization strategy: %s", strategy)
	}

	// Create work directory for minimization
	workDir := filepath.Join(job.WorkDir, "minimization", crashID, strategy)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	// Create minimization config
	config := MinimizationConfig{
		MaxIterations:    1000,
		MaxTime:          30 * time.Minute,
		MinChunkSize:     1,
		TargetBinary:     job.Target,
		BinaryArgs:       []string{}, // TODO: Get from job config
		ExpectedSignal:   crash.Signal,
		ExpectedExitCode: crash.ExitCode,
		Timeout:          5 * time.Second,
		WorkDir:          workDir,
	}

	// Run minimization
	startTime := time.Now()
	resultWithData, err := minimizer.Minimize(ctx, input, config)
	if err != nil {
		return nil, fmt.Errorf("minimization failed: %w", err)
	}

	// Set additional fields
	resultWithData.CrashID = crashID
	resultWithData.JobID = crash.JobID
	resultWithData.BotID = crash.BotID
	resultWithData.ExecutionTime = time.Since(startTime)
	resultWithData.Timestamp = time.Now()

	// Save minimized input if successful
	if resultWithData.Success && len(resultWithData.MinimizedInput) > 0 {
		minPath := filepath.Join(workDir, fmt.Sprintf("minimized_%s", resultWithData.MinimizedHash[:8]))
		if err := s.fileStorage.SaveFile(ctx, minPath, resultWithData.MinimizedInput); err != nil {
			s.log.WithError(err).Error("Failed to save minimized input")
		} else {
			resultWithData.MinimizedPath = minPath
		}
	}

	// Store result in database
	// TODO: Add storage method for minimization results

	return resultWithData.MinimizationResult, nil
}

func (s *crashMinimizerService) GetMinimizationResult(ctx context.Context, resultID string) (*common.MinimizationResult, error) {
	// TODO: Implement storage retrieval
	return nil, fmt.Errorf("not implemented")
}

func (s *crashMinimizerService) ListMinimizationResults(ctx context.Context, crashID string) ([]*common.MinimizationResult, error) {
	// TODO: Implement storage retrieval
	return nil, fmt.Errorf("not implemented")
}

func (s *crashMinimizerService) GetBestMinimization(ctx context.Context, crashID string) (*common.MinimizationResult, error) {
	results, err := s.ListMinimizationResults(ctx, crashID)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no minimization results found for crash %s", crashID)
	}

	// Find the smallest successful minimization
	var best *common.MinimizationResult
	for _, result := range results {
		if !result.Success || !result.StillReproduces {
			continue
		}
		if best == nil || result.MinimizedSize < best.MinimizedSize {
			best = result
		}
	}

	if best == nil {
		return nil, fmt.Errorf("no successful minimization found for crash %s", crashID)
	}

	return best, nil
}

// DeltaDebugMinimizer implements delta debugging algorithm
type DeltaDebugMinimizer struct {
	log logrus.FieldLogger
}

// NewDeltaDebugMinimizer creates a new delta debug minimizer
func NewDeltaDebugMinimizer(log logrus.FieldLogger) Minimizer {
	return &DeltaDebugMinimizer{
		log: log.WithField("minimizer", "delta_debug"),
	}
}

func (m *DeltaDebugMinimizer) GetStrategy() MinimizationStrategy {
	return MinimizationStrategyDeltaDebug
}

func (m *DeltaDebugMinimizer) Minimize(ctx context.Context, input []byte, config MinimizationConfig) (*MinimizationResultWithData, error) {
	result := &MinimizationResultWithData{
		MinimizationResult: &common.MinimizationResult{
			Strategy:      string(MinimizationStrategyDeltaDebug),
			OriginalSize:  int64(len(input)),
			MinimizedSize: int64(len(input)),
			Success:       false,
		},
	}

	// Start with the full input
	current := make([]byte, len(input))
	copy(current, input)

	// Delta debugging algorithm
	n := 2
	iterations := 0

	for n <= len(current) && iterations < config.MaxIterations {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// Check time limit
		if time.Since(result.Timestamp) > config.MaxTime {
			break
		}

		chunkSize := len(current) / n
		if chunkSize < config.MinChunkSize {
			n = len(current) / config.MinChunkSize
			if n <= 1 {
				break
			}
		}

		foundReduction := false

		// Try removing each chunk
		for i := 0; i < n && !foundReduction; i++ {
			start := i * chunkSize
			end := start + chunkSize
			if i == n-1 {
				end = len(current)
			}

			// Create test input without this chunk
			test := make([]byte, 0, len(current)-chunkSize)
			test = append(test, current[:start]...)
			test = append(test, current[end:]...)

			// Test if it still crashes
			if m.testCrash(ctx, test, config) {
				current = test
				foundReduction = true
				n = 2 // Reset to try larger chunks again
				m.log.Debugf("Reduced input size from %d to %d bytes", len(current)+chunkSize, len(current))
			}
		}

		if !foundReduction {
			// Try complementary chunks
			for i := 0; i < n && !foundReduction; i++ {
				start := i * chunkSize
				end := start + chunkSize
				if i == n-1 {
					end = len(current)
				}

				// Create test input with only this chunk
				test := current[start:end]

				// Test if it still crashes
				if m.testCrash(ctx, test, config) {
					current = test
					foundReduction = true
					n = 2 // Reset
					m.log.Debugf("Reduced to single chunk: %d bytes", len(current))
				}
			}
		}

		if !foundReduction {
			n = n * 2
		}

		iterations++
		result.Iterations = iterations
	}

	// Calculate final statistics
	result.MinimizedInput = current
	result.MinimizedSize = int64(len(current))
	result.ReductionPercent = float64(result.OriginalSize-result.MinimizedSize) / float64(result.OriginalSize) * 100
	result.Success = result.MinimizedSize < result.OriginalSize
	result.StillReproduces = m.testCrash(ctx, current, config)

	// Calculate hash
	hash := sha256.Sum256(current)
	result.MinimizedHash = hex.EncodeToString(hash[:])

	return result, nil
}

func (m *DeltaDebugMinimizer) testCrash(ctx context.Context, input []byte, config MinimizationConfig) bool {
	// Write test input to temporary file
	testFile := filepath.Join(config.WorkDir, "test_input")
	if err := os.WriteFile(testFile, input, 0644); err != nil {
		m.log.WithError(err).Error("Failed to write test input")
		return false
	}
	defer os.Remove(testFile)

	// Create command with timeout
	ctx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	// Build command
	args := append(config.BinaryArgs, testFile)
	cmd := exec.CommandContext(ctx, config.TargetBinary, args...)

	// Set up stdin from test file
	stdin, err := os.Open(testFile)
	if err != nil {
		m.log.WithError(err).Error("Failed to open test file")
		return false
	}
	defer stdin.Close()
	cmd.Stdin = stdin

	// Run and check result
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err == nil {
		return false // No crash
	}

	// Check if it's the expected crash
	if exitErr, ok := err.(*exec.ExitError); ok {
		// Check signal
		if config.ExpectedSignal > 0 {
			// Platform-specific signal checking would go here
			// For now, just check that it crashed
			return exitErr.ExitCode() != 0
		}
		// Check exit code
		return exitErr.ExitCode() == config.ExpectedExitCode
	}

	return false
}

// CoverageGuidedMinimizer implements coverage-guided minimization
type CoverageGuidedMinimizer struct {
	log logrus.FieldLogger
}

// NewCoverageGuidedMinimizer creates a new coverage-guided minimizer
func NewCoverageGuidedMinimizer(log logrus.FieldLogger) Minimizer {
	return &CoverageGuidedMinimizer{
		log: log.WithField("minimizer", "coverage_guided"),
	}
}

func (m *CoverageGuidedMinimizer) GetStrategy() MinimizationStrategy {
	return MinimizationStrategyCoverageGuided
}

func (m *CoverageGuidedMinimizer) Minimize(ctx context.Context, input []byte, config MinimizationConfig) (*MinimizationResultWithData, error) {
	result := &MinimizationResultWithData{
		MinimizationResult: &common.MinimizationResult{
			Strategy:      string(MinimizationStrategyCoverageGuided),
			OriginalSize:  int64(len(input)),
			MinimizedSize: int64(len(input)),
			Success:       false,
		},
	}

	// Get initial coverage
	baseCoverage, err := m.getCoverage(ctx, input, config)
	if err != nil {
		return result, fmt.Errorf("failed to get base coverage: %w", err)
	}

	// Start with the full input
	current := make([]byte, len(input))
	copy(current, input)

	iterations := 0
	improved := true

	// Iteratively remove bytes that don't affect coverage
	for improved && iterations < config.MaxIterations {
		improved = false

		// Check context and time limits
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		if time.Since(result.Timestamp) > config.MaxTime {
			break
		}

		// Try removing each byte
		for i := 0; i < len(current); i++ {
			// Create test input without this byte
			test := make([]byte, 0, len(current)-1)
			test = append(test, current[:i]...)
			test = append(test, current[i+1:]...)

			// Check if it still crashes with same coverage
			if m.testCrashWithCoverage(ctx, test, config, baseCoverage) {
				current = test
				improved = true
				i-- // Adjust index since we removed a byte
			}
		}

		iterations++
		result.Iterations = iterations

		// Log progress
		if improved {
			m.log.Debugf("Coverage-guided reduction: %d -> %d bytes", len(current)+1, len(current))
		}
	}

	// Try additional strategies for coverage-guided minimization
	current = m.tryBlockRemoval(ctx, current, config, baseCoverage)
	current = m.tryPatternSimplification(ctx, current, config, baseCoverage)

	// Calculate final statistics
	result.MinimizedInput = current
	result.MinimizedSize = int64(len(current))
	result.ReductionPercent = float64(result.OriginalSize-result.MinimizedSize) / float64(result.OriginalSize) * 100
	result.Success = result.MinimizedSize < result.OriginalSize
	result.StillReproduces = m.testCrashWithCoverage(ctx, current, config, baseCoverage)

	// Calculate hash
	hash := sha256.Sum256(current)
	result.MinimizedHash = hex.EncodeToString(hash[:])

	return result, nil
}

func (m *CoverageGuidedMinimizer) getCoverage(ctx context.Context, input []byte, config MinimizationConfig) ([]byte, error) {
	// This would integrate with coverage instrumentation
	// For now, return a placeholder
	// In practice, this would:
	// 1. Run the binary with coverage instrumentation
	// 2. Extract coverage bitmap/counters
	// 3. Return coverage data
	return []byte("placeholder_coverage"), nil
}

func (m *CoverageGuidedMinimizer) testCrashWithCoverage(ctx context.Context, input []byte, config MinimizationConfig, targetCoverage []byte) bool {
	// Test if input still crashes
	testFile := filepath.Join(config.WorkDir, "test_input_cov")
	if err := os.WriteFile(testFile, input, 0644); err != nil {
		return false
	}
	defer os.Remove(testFile)

	ctx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	args := append(config.BinaryArgs, testFile)
	cmd := exec.CommandContext(ctx, config.TargetBinary, args...)

	stdin, err := os.Open(testFile)
	if err != nil {
		return false
	}
	defer stdin.Close()
	cmd.Stdin = stdin

	err = cmd.Run()
	if err == nil {
		return false // No crash
	}

	// Check if it's the expected crash
	// In practice, would also verify coverage matches
	if exitErr, ok := err.(*exec.ExitError); ok {
		if config.ExpectedSignal > 0 {
			return exitErr.ExitCode() != 0
		}
		return exitErr.ExitCode() == config.ExpectedExitCode
	}

	return false
}

func (m *CoverageGuidedMinimizer) tryBlockRemoval(ctx context.Context, input []byte, config MinimizationConfig, targetCoverage []byte) []byte {
	// Try removing larger blocks that don't affect coverage
	blockSizes := []int{256, 128, 64, 32, 16, 8, 4, 2}
	current := make([]byte, len(input))
	copy(current, input)

	for _, blockSize := range blockSizes {
		if blockSize > len(current)/2 {
			continue
		}

		improved := true
		for improved {
			improved = false

			for i := 0; i <= len(current)-blockSize; i += blockSize {
				// Try removing this block
				test := make([]byte, 0, len(current)-blockSize)
				test = append(test, current[:i]...)
				test = append(test, current[i+blockSize:]...)

				if m.testCrashWithCoverage(ctx, test, config, targetCoverage) {
					current = test
					improved = true
					i -= blockSize // Adjust position
					if i < 0 {
						i = 0
					}
				}
			}
		}
	}

	return current
}

func (m *CoverageGuidedMinimizer) tryPatternSimplification(ctx context.Context, input []byte, config MinimizationConfig, targetCoverage []byte) []byte {
	// Try simplifying patterns (e.g., replace sequences with simpler ones)
	current := make([]byte, len(input))
	copy(current, input)

	// Try replacing non-zero bytes with zeros
	for i := 0; i < len(current); i++ {
		if current[i] != 0 {
			old := current[i]
			current[i] = 0
			if !m.testCrashWithCoverage(ctx, current, config, targetCoverage) {
				current[i] = old // Restore if it doesn't work
			}
		}
	}

	// Try replacing with 'A' (common simplification)
	for i := 0; i < len(current); i++ {
		if current[i] != 'A' && current[i] != 0 {
			old := current[i]
			current[i] = 'A'
			if !m.testCrashWithCoverage(ctx, current, config, targetCoverage) {
				current[i] = old
			}
		}
	}

	return current
}

// MinimizationJob represents a job specifically for crash minimization
type MinimizationJob struct {
	*common.Job
	CrashID  string               `json:"crash_id"`
	Strategy MinimizationStrategy `json:"strategy"`
}

// CreateMinimizationJob creates a new minimization job
func CreateMinimizationJob(crash *common.CrashResult, strategy MinimizationStrategy) *MinimizationJob {
	jobID := fmt.Sprintf("min_%s_%s_%d", crash.ID[:8], strategy, time.Now().Unix())

	return &MinimizationJob{
		Job: &common.Job{
			ID:        jobID,
			Name:      fmt.Sprintf("Minimize crash %s", crash.ID[:8]),
			Target:    crash.JobID, // Reference to original job
			Fuzzer:    "minimizer",
			Status:    common.JobStatusPending,
			CreatedAt: time.Now(),
			TimeoutAt: time.Now().Add(1 * time.Hour),
			Config: common.JobConfig{
				Duration:    1 * time.Hour,
				MemoryLimit: 1024 * 1024 * 1024, // 1GB
				Timeout:     5 * time.Second,
			},
		},
		CrashID:  crash.ID,
		Strategy: strategy,
	}
}

// MinimizationStats tracks minimization statistics
type MinimizationStats struct {
	TotalMinimizations      int64                     `json:"total_minimizations"`
	SuccessfulMinimizations int64                     `json:"successful_minimizations"`
	TotalReduction          int64                     `json:"total_reduction_bytes"`
	AverageReduction        float64                   `json:"average_reduction_percent"`
	BestReduction           float64                   `json:"best_reduction_percent"`
	TotalTime               time.Duration             `json:"total_time"`
	AverageTime             time.Duration             `json:"average_time"`
	ByStrategy              map[string]*StrategyStats `json:"by_strategy"`
}

// StrategyStats tracks statistics per minimization strategy
type StrategyStats struct {
	Count            int64         `json:"count"`
	Successes        int64         `json:"successes"`
	TotalReduction   int64         `json:"total_reduction"`
	AverageReduction float64       `json:"average_reduction"`
	AverageTime      time.Duration `json:"average_time"`
}
