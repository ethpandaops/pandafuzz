package bot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// MinimizerClient handles crash minimization tasks on the bot side
type MinimizerClient struct {
	config      *common.BotConfig
	httpClient  *http.Client
	masterURL   string
	logger      logrus.FieldLogger
	cacheMutex  sync.RWMutex
	resultCache map[string]*common.MinimizationResult
	cacheExpiry time.Duration
	workDir     string
	maxAttempts int
	timeout     time.Duration
}

// MinimizationStrategy represents the minimization strategy to use
type MinimizationStrategy string

const (
	// MinimizationStrategyDeltaDebug uses delta debugging approach
	MinimizationStrategyDeltaDebug MinimizationStrategy = "delta_debug"
	// MinimizationStrategyCoverageGuided uses coverage feedback
	MinimizationStrategyCoverageGuided MinimizationStrategy = "coverage_guided"
	// MinimizationStrategyBinary uses binary search
	MinimizationStrategyBinary MinimizationStrategy = "binary"
)

// MinimizationConfig holds configuration for crash minimization
type MinimizationConfig struct {
	Strategy          MinimizationStrategy `json:"strategy"`
	MaxIterations     int                  `json:"max_iterations"`
	Timeout           time.Duration        `json:"timeout"`
	PreserveSemantics bool                 `json:"preserve_semantics"`
	UseCoverage       bool                 `json:"use_coverage"`
}

// NewMinimizerClient creates a new minimizer client
func NewMinimizerClient(config *common.BotConfig, logger logrus.FieldLogger) (*MinimizerClient, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	// Set defaults
	cacheExpiry := 15 * time.Minute
	maxAttempts := 3
	timeout := 30 * time.Minute

	// Override from config if available
	if config.Timeouts.JobExecution > 0 {
		timeout = config.Timeouts.JobExecution
	}

	httpClient := &http.Client{
		Timeout: config.Timeouts.MasterCommunication,
		Transport: &http.Transport{
			MaxIdleConns:       10,
			IdleConnTimeout:    30 * time.Second,
			DisableCompression: false,
		},
	}

	return &MinimizerClient{
		config:      config,
		httpClient:  httpClient,
		masterURL:   config.MasterURL,
		logger:      logger.WithField("component", "minimizer_client"),
		resultCache: make(map[string]*common.MinimizationResult),
		cacheExpiry: cacheExpiry,
		workDir:     config.Fuzzing.WorkDir,
		maxAttempts: maxAttempts,
		timeout:     timeout,
	}, nil
}

// MinimizeCrash performs crash minimization locally
func (c *MinimizerClient) MinimizeCrash(ctx context.Context, crash *common.CrashResult, config MinimizationConfig) (*common.MinimizationResult, error) {
	c.logger.WithFields(logrus.Fields{
		"crash_id": crash.ID,
		"strategy": config.Strategy,
		"size":     crash.Size,
	}).Info("Starting crash minimization")

	// Check cache first
	if cached := c.getCachedResult(crash.ID); cached != nil {
		c.logger.Debug("Returning cached minimization result")
		return cached, nil
	}

	// Prepare work directory
	minDir := filepath.Join(c.workDir, "minimization", crash.ID)
	if err := os.MkdirAll(minDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create minimization directory: %w", err)
	}
	defer func() {
		// Clean up work directory after completion
		if err := os.RemoveAll(minDir); err != nil {
			c.logger.WithError(err).Warn("Failed to clean up minimization directory")
		}
	}()

	// Write original crash input
	originalPath := filepath.Join(minDir, "original")
	if err := os.WriteFile(originalPath, crash.Input, 0644); err != nil {
		return nil, fmt.Errorf("failed to write original crash input: %w", err)
	}

	// Start minimization
	startTime := time.Now()
	result := &common.MinimizationResult{
		ID:            fmt.Sprintf("min_%s_%d", crash.ID, time.Now().Unix()),
		CrashID:       crash.ID,
		JobID:         crash.JobID,
		BotID:         c.config.Name,
		Strategy:      string(config.Strategy),
		OriginalSize:  crash.Size,
		MinimizedSize: crash.Size, // Start with original size
		Timestamp:     time.Now(),
	}

	// Execute minimization based on strategy
	var minimizedInput []byte
	var err error

	switch config.Strategy {
	case MinimizationStrategyDeltaDebug:
		minimizedInput, err = c.deltaDebugMinimize(ctx, crash, originalPath, minDir, config)
	case MinimizationStrategyCoverageGuided:
		minimizedInput, err = c.coverageGuidedMinimize(ctx, crash, originalPath, minDir, config)
	case MinimizationStrategyBinary:
		minimizedInput, err = c.binarySearchMinimize(ctx, crash, originalPath, minDir, config)
	default:
		return nil, fmt.Errorf("unsupported minimization strategy: %s", config.Strategy)
	}

	// Record execution time
	result.ExecutionTime = time.Since(startTime)

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		c.logger.WithError(err).Error("Minimization failed")
	} else {
		// Save minimized input
		minimizedPath := filepath.Join(minDir, "minimized")
		if err := os.WriteFile(minimizedPath, minimizedInput, 0644); err != nil {
			return nil, fmt.Errorf("failed to write minimized input: %w", err)
		}

		// Calculate hash
		hash := sha256.Sum256(minimizedInput)
		result.MinimizedHash = hex.EncodeToString(hash[:])
		result.MinimizedSize = int64(len(minimizedInput))
		result.ReductionPercent = float64(result.OriginalSize-result.MinimizedSize) / float64(result.OriginalSize) * 100
		result.Success = true
		result.MinimizedPath = minimizedPath

		// Verify minimized input still triggers crash
		stillCrashes, err := c.VerifyCrash(ctx, crash, minimizedInput)
		if err != nil {
			c.logger.WithError(err).Warn("Failed to verify minimized crash")
		}
		result.StillReproduces = stillCrashes

		c.logger.WithFields(logrus.Fields{
			"original_size":  result.OriginalSize,
			"minimized_size": result.MinimizedSize,
			"reduction":      fmt.Sprintf("%.2f%%", result.ReductionPercent),
			"still_crashes":  result.StillReproduces,
		}).Info("Minimization completed successfully")
	}

	// Cache result
	c.cacheResult(crash.ID, result)

	// Report to master
	if err := c.reportResult(ctx, result); err != nil {
		c.logger.WithError(err).Error("Failed to report minimization result to master")
		// Don't fail the operation if reporting fails
	}

	return result, nil
}

// VerifyCrash checks if the minimized input still triggers the crash
func (c *MinimizerClient) VerifyCrash(ctx context.Context, originalCrash *common.CrashResult, minimizedInput []byte) (bool, error) {
	c.logger.Debug("Verifying minimized crash input")

	// Create temporary file for input
	tmpFile, err := os.CreateTemp("", "verify_crash_*")
	if err != nil {
		return false, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.Write(minimizedInput); err != nil {
		return false, fmt.Errorf("failed to write input: %w", err)
	}
	tmpFile.Close()

	// Parse original command
	parts := strings.Fields(originalCrash.FilePath)
	if len(parts) == 0 {
		return false, fmt.Errorf("invalid command in crash result")
	}

	// Prepare command with minimized input
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Stdin, _ = os.Open(tmpFile.Name())

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd = exec.CommandContext(timeoutCtx, cmd.Path, cmd.Args...)
	err = cmd.Run()

	// Check if it crashed in the same way
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Check if exit code matches
			if exitErr.ExitCode() == originalCrash.ExitCode {
				c.logger.Debug("Minimized input still triggers the same crash")
				return true, nil
			}
		}
	}

	c.logger.Debug("Minimized input does not trigger the same crash")
	return false, nil
}

// deltaDebugMinimize implements delta debugging minimization
func (c *MinimizerClient) deltaDebugMinimize(ctx context.Context, crash *common.CrashResult, originalPath, workDir string, config MinimizationConfig) ([]byte, error) {
	c.logger.Debug("Using delta debugging minimization strategy")

	// Read original input
	input, err := os.ReadFile(originalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read original input: %w", err)
	}

	// Start with the full input
	currentInput := input
	iterations := 0
	maxIterations := config.MaxIterations
	if maxIterations == 0 {
		maxIterations = 100
	}

	for iterations < maxIterations {
		improved := false
		chunkSize := len(currentInput) / 2

		for chunkSize >= 1 {
			// Try removing chunks
			for i := 0; i < len(currentInput); i += chunkSize {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}

				end := i + chunkSize
				if end > len(currentInput) {
					end = len(currentInput)
				}

				// Create test input without this chunk
				testInput := append(currentInput[:i:i], currentInput[end:]...)

				// Test if it still crashes
				stillCrashes, err := c.VerifyCrash(ctx, crash, testInput)
				if err != nil {
					c.logger.WithError(err).Debug("Failed to verify test input")
					continue
				}

				if stillCrashes {
					currentInput = testInput
					improved = true
					c.logger.WithFields(logrus.Fields{
						"removed_bytes": chunkSize,
						"new_size":      len(currentInput),
					}).Debug("Successfully removed chunk")
					break
				}
			}

			if !improved {
				chunkSize /= 2
			} else {
				break // Start over with new input
			}
		}

		if !improved {
			break // No more improvements possible
		}
		iterations++
	}

	c.logger.WithFields(logrus.Fields{
		"iterations":    iterations,
		"original_size": len(input),
		"final_size":    len(currentInput),
	}).Debug("Delta debugging completed")

	return currentInput, nil
}

// coverageGuidedMinimize implements coverage-guided minimization
func (c *MinimizerClient) coverageGuidedMinimize(ctx context.Context, crash *common.CrashResult, originalPath, workDir string, config MinimizationConfig) ([]byte, error) {
	c.logger.Debug("Using coverage-guided minimization strategy")

	// For now, fall back to delta debugging
	// TODO: Implement proper coverage-guided minimization when coverage data is available
	return c.deltaDebugMinimize(ctx, crash, originalPath, workDir, config)
}

// binarySearchMinimize implements binary search minimization
func (c *MinimizerClient) binarySearchMinimize(ctx context.Context, crash *common.CrashResult, originalPath, workDir string, config MinimizationConfig) ([]byte, error) {
	c.logger.Debug("Using binary search minimization strategy")

	// Read original input
	input, err := os.ReadFile(originalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read original input: %w", err)
	}

	// Binary search for minimal crashing prefix
	left := 0
	right := len(input)
	minCrashingSize := right

	iterations := 0
	maxIterations := config.MaxIterations
	if maxIterations == 0 {
		maxIterations = 50
	}

	for left < right && iterations < maxIterations {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		mid := (left + right) / 2
		testInput := input[:mid]

		stillCrashes, err := c.VerifyCrash(ctx, crash, testInput)
		if err != nil {
			c.logger.WithError(err).Debug("Failed to verify test input")
			// Skip this iteration
			left = mid + 1
			continue
		}

		if stillCrashes {
			minCrashingSize = mid
			right = mid
		} else {
			left = mid + 1
		}
		iterations++
	}

	c.logger.WithFields(logrus.Fields{
		"iterations":    iterations,
		"original_size": len(input),
		"final_size":    minCrashingSize,
	}).Debug("Binary search completed")

	return input[:minCrashingSize], nil
}

// getCachedResult retrieves a cached minimization result
func (c *MinimizerClient) getCachedResult(crashID string) *common.MinimizationResult {
	c.cacheMutex.RLock()
	defer c.cacheMutex.RUnlock()

	if result, ok := c.resultCache[crashID]; ok {
		// Check if cache entry is still valid
		if time.Since(result.Timestamp) < c.cacheExpiry {
			return result
		}
	}
	return nil
}

// cacheResult stores a minimization result in cache
func (c *MinimizerClient) cacheResult(crashID string, result *common.MinimizationResult) {
	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()

	c.resultCache[crashID] = result

	// Clean up old entries
	for id, res := range c.resultCache {
		if time.Since(res.Timestamp) > c.cacheExpiry {
			delete(c.resultCache, id)
		}
	}
}

// reportResult reports minimization result to master
func (c *MinimizerClient) reportResult(ctx context.Context, result *common.MinimizationResult) error {
	endpoint := fmt.Sprintf("%s/api/v1/minimization/result", c.masterURL)

	jsonData, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bot-ID", c.config.Name)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned error: %d - %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetMinimizationJob retrieves a minimization job from the master
func (c *MinimizerClient) GetMinimizationJob(ctx context.Context, jobID string) (*common.Job, error) {
	endpoint := fmt.Sprintf("%s/api/v1/jobs/%s", c.masterURL, jobID)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Bot-ID", c.config.Name)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned error: %d - %s", resp.StatusCode, string(body))
	}

	var job common.Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &job, nil
}

// ProcessMinimizationJob processes a minimization job
func (c *MinimizerClient) ProcessMinimizationJob(ctx context.Context, job *common.Job) error {
	// Extract crash ID from job metadata
	crashID, ok := job.Metadata["crash_id"].(string)
	if !ok {
		return fmt.Errorf("crash_id not found in job metadata")
	}

	strategy, _ := job.Metadata["strategy"].(string)
	if strategy == "" {
		strategy = string(MinimizationStrategyDeltaDebug)
	}

	// Retrieve crash details from master
	crash, err := c.GetCrashDetails(ctx, crashID)
	if err != nil {
		return fmt.Errorf("failed to get crash details: %w", err)
	}

	// Prepare minimization config
	config := MinimizationConfig{
		Strategy:      MinimizationStrategy(strategy),
		MaxIterations: 100,
		Timeout:       job.Config.Timeout,
	}

	// Perform minimization
	result, err := c.MinimizeCrash(ctx, crash, config)
	if err != nil {
		return fmt.Errorf("minimization failed: %w", err)
	}

	// Update job with result
	result.JobID = job.ID

	return nil
}

// GetCrashDetails retrieves crash details from master
func (c *MinimizerClient) GetCrashDetails(ctx context.Context, crashID string) (*common.CrashResult, error) {
	endpoint := fmt.Sprintf("%s/api/v1/crashes/%s", c.masterURL, crashID)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Bot-ID", c.config.Name)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned error: %d - %s", resp.StatusCode, string(body))
	}

	var crash common.CrashResult
	if err := json.NewDecoder(resp.Body).Decode(&crash); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &crash, nil
}
