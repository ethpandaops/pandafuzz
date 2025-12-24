package bot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/adapter"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// ReproducibilityExecutor handles crash reproduction tasks
type ReproducibilityExecutor struct {
	client       *RetryClient
	config       *common.BotConfig
	logger       *logrus.Logger
	activeRepros map[string]*ReproductionExecution
	mu           sync.RWMutex
	botID        string
}

// ReproductionExecution represents an active reproduction execution
type ReproductionExecution struct {
	Request    *common.ReproductionRequest
	Fuzzer     adapter.Fuzzer
	Context    context.Context
	Cancel     context.CancelFunc
	StartTime  time.Time
	Status     string
	LastUpdate time.Time
}

// NewReproducibilityExecutor creates a new reproducibility executor
func NewReproducibilityExecutor(
	client *RetryClient,
	config *common.BotConfig,
	logger *logrus.Logger,
) *ReproducibilityExecutor {
	return &ReproducibilityExecutor{
		client:       client,
		config:       config,
		logger:       logger,
		activeRepros: make(map[string]*ReproductionExecution),
		botID:        config.ID,
	}
}

// ExecuteReproduction executes a crash reproduction request
func (re *ReproducibilityExecutor) ExecuteReproduction(
	ctx context.Context,
	request *common.ReproductionRequest,
) (*common.ReproductionResult, error) {
	logger := re.logger.WithFields(logrus.Fields{
		"crash_id":   request.CrashID,
		"request_id": request.ID,
		"attempt":    request.AttemptCount,
		"priority":   request.Priority,
	})

	logger.Info("Starting crash reproduction execution")

	// Create execution context with timeout
	execCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	execution := &ReproductionExecution{
		Request:    request,
		Context:    execCtx,
		Cancel:     cancel,
		StartTime:  time.Now(),
		Status:     "starting",
		LastUpdate: time.Now(),
	}

	// Track active reproduction
	re.mu.Lock()
	re.activeRepros[request.ID] = execution
	re.mu.Unlock()

	defer func() {
		// Remove from active reproductions
		re.mu.Lock()
		delete(re.activeRepros, request.ID)
		re.mu.Unlock()
	}()

	// Step 1: Download crash input from master
	crashInput, err := re.downloadCrashInput(execCtx, request.CrashID)
	if err != nil {
		logger.WithError(err).Error("Failed to download crash input")
		return re.createFailedResult(request, "Failed to download crash input", err), nil
	}

	logger.WithField("input_size", len(crashInput)).Debug("Downloaded crash input")

	// Step 2: Prepare working directory
	workDir := filepath.Join(re.config.Fuzzing.WorkDir, "reproductions", request.ID)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		logger.WithError(err).Error("Failed to create work directory")
		return re.createFailedResult(request, "Failed to create work directory", err), nil
	}
	defer func() {
		// Cleanup work directory after reproduction
		if err := os.RemoveAll(workDir); err != nil {
			logger.WithError(err).Warn("Failed to cleanup work directory")
		}
	}()

	// Save crash input to file
	inputPath := filepath.Join(workDir, "crash_input.bin")
	if err := os.WriteFile(inputPath, crashInput, 0644); err != nil {
		logger.WithError(err).Error("Failed to save crash input")
		return re.createFailedResult(request, "Failed to save crash input", err), nil
	}

	// Step 3: Get or create fuzzer instance
	targetFuzzer, err := re.getFuzzerForReproduction(request)
	if err != nil {
		logger.WithError(err).Error("Failed to create fuzzer for reproduction")
		return re.createFailedResult(request, "Failed to create fuzzer", err), nil
	}

	execution.Fuzzer = targetFuzzer
	execution.Status = "reproducing"
	execution.LastUpdate = time.Now()

	// Step 4: Configure reproduction
	reproConfig := adapter.ReproductionConfig{
		Attempts:         1, // Single attempt per request
		Timeout:          5 * time.Minute,
		CollectDebugInfo: true,
		OriginalCrashID:  request.CrashID,
		Environment: map[string]string{
			"ASAN_OPTIONS":  "symbolize=1:print_stats=1:check_initialization_order=1",
			"UBSAN_OPTIONS": "print_stacktrace=1",
			"MSAN_OPTIONS":  "symbolize=1",
			"TSAN_OPTIONS":  "symbolize=1",
		},
		Options: map[string]any{
			"work_dir":       workDir,
			"save_artifacts": true,
		},
	}

	// Step 5: Execute reproduction
	logger.Info("Executing crash reproduction")
	reproResult, err := targetFuzzer.ReproduceCrash(execCtx, crashInput, reproConfig)
	if err != nil {
		logger.WithError(err).Error("Failed to reproduce crash")
		return re.createFailedResult(request, "Failed to reproduce crash", err), nil
	}

	// Step 6: Process and enhance result
	result := re.processReproductionResult(request, reproResult)

	// Step 7: Report result back to master
	if err := re.reportReproductionResult(execCtx, result); err != nil {
		logger.WithError(err).Error("Failed to report reproduction result")
		// Don't fail the result, just log the error
	}

	logger.WithFields(logrus.Fields{
		"reproduced":       result.Reproduced,
		"matches_original": result.MatchesOriginal,
		"signal":           result.Signal,
		"exit_code":        result.ExitCode,
	}).Info("Crash reproduction completed")

	return result, nil
}

// downloadCrashInput downloads the crash input from the master API
func (re *ReproducibilityExecutor) downloadCrashInput(ctx context.Context, crashID string) ([]byte, error) {
	return re.client.DownloadCrashInput(crashID, re.config.ID)
}

// getFuzzerForReproduction gets or creates a fuzzer instance for reproduction
func (re *ReproducibilityExecutor) getFuzzerForReproduction(request *common.ReproductionRequest) (adapter.Fuzzer, error) {
	// Determine fuzzer type from request or default to libfuzzer
	fuzzerType := "libfuzzer"
	if request.Config.Dictionary != "" {
		// If dictionary is specified, might be AFL++
		fuzzerType = "aflplusplus"
	}

	// Create appropriate fuzzer instance like FuzzerJobExecutor does
	var fuzz *adapter.FuzzerAdapter

	switch fuzzerType {
	case "aflplusplus", "afl++", "afl":
		fuzz = adapter.NewAFLPlusPlus(re.logger)
		fuzz.SetBotID(re.botID)

	case "libfuzzer":
		fuzz = adapter.NewLibFuzzer(re.logger)
		fuzz.SetBotID(re.botID)

	case "honggfuzz":
		fuzz = adapter.NewHonggfuzz(re.logger)
		fuzz.SetBotID(re.botID)

	default:
		return nil, fmt.Errorf("unsupported fuzzer type: %s", fuzzerType)
	}

	// Prepare corpus directory if seed corpus is specified
	workDir := filepath.Join(re.config.Fuzzing.WorkDir, "reproductions", request.ID)
	corpusDir := filepath.Join(workDir, "corpus")

	if len(request.Config.SeedCorpus) > 0 {
		// Create corpus directory
		if err := os.MkdirAll(corpusDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create corpus directory: %w", err)
		}

		// Download or copy corpus files
		re.logger.WithField("corpus_count", len(request.Config.SeedCorpus)).Debug("Initializing corpus for reproduction")

		for i, corpusPath := range request.Config.SeedCorpus {
			// If corpus path is a URL, download it
			// Otherwise, assume it's a local path relative to the job
			destPath := filepath.Join(corpusDir, fmt.Sprintf("seed_%d", i))

			// For now, we'll assume corpus files are available locally
			// In a full implementation, you might need to download from master
			re.logger.WithFields(logrus.Fields{
				"corpus_path": corpusPath,
				"dest_path":   destPath,
			}).Debug("Adding corpus file for reproduction")
		}
	}

	// Configure the fuzzer for reproduction
	fuzzConfig := adapter.FuzzConfig{
		JobID:         request.JobID,
		Target:        request.Config.OutputDir, // Use from job config
		WorkDirectory: workDir,
		Timeout:       request.Config.Timeout,
		MemoryLimit:   request.Config.MemoryLimit,
		Duration:      request.Config.Duration,
		Dictionary:    request.Config.Dictionary,
		SeedDirectory: corpusDir, // Add corpus directory
		FuzzerOptions: map[string]any{
			"reproduction_mode": true,
			"max_total_time":    300, // 5 minutes max
			"seed_corpus":       request.Config.SeedCorpus,
		},
	}

	// Configure the fuzzer
	if err := fuzz.Configure(fuzzConfig); err != nil {
		return nil, fmt.Errorf("failed to configure fuzzer: %w", err)
	}

	// Initialize fuzzer
	if err := fuzz.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize fuzzer: %w", err)
	}

	return fuzz, nil
}

// processReproductionResult processes and enhances the fuzzer reproduction result
func (re *ReproducibilityExecutor) processReproductionResult(
	request *common.ReproductionRequest,
	fuzzerResult *common.ReproductionResult,
) *common.ReproductionResult {
	// If fuzzer returned a result, enhance it
	if fuzzerResult != nil {
		fuzzerResult.ID = uuid.New().String()
		fuzzerResult.RequestID = request.ID
		fuzzerResult.CrashID = request.CrashID
		fuzzerResult.BotID = re.config.ID
		fuzzerResult.AttemptNumber = request.AttemptCount + 1
		fuzzerResult.Timestamp = time.Now()

		// Add environment info
		if fuzzerResult.EnvironmentInfo == nil {
			fuzzerResult.EnvironmentInfo = make(map[string]string)
		}
		fuzzerResult.EnvironmentInfo["bot_id"] = re.config.ID
		fuzzerResult.EnvironmentInfo["bot_name"] = re.config.Name
		fuzzerResult.EnvironmentInfo["job_id"] = request.JobID

		// Determine status based on reproduction result
		if fuzzerResult.Reproduced && fuzzerResult.MatchesOriginal {
			fuzzerResult.Status = common.ReproducibilityStatusConfirmed
		} else if fuzzerResult.Reproduced && !fuzzerResult.MatchesOriginal {
			fuzzerResult.Status = common.ReproducibilityStatusFlaky
		} else {
			fuzzerResult.Status = common.ReproducibilityStatusFailed
		}

		return fuzzerResult
	}

	// Create a failed result if fuzzer didn't return one
	return re.createFailedResult(request, "No result from fuzzer", nil)
}

// createFailedResult creates a failed reproduction result
func (re *ReproducibilityExecutor) createFailedResult(
	request *common.ReproductionRequest,
	message string,
	err error,
) *common.ReproductionResult {
	output := message
	if err != nil {
		output = fmt.Sprintf("%s: %v", message, err)
	}

	return &common.ReproductionResult{
		ID:              uuid.New().String(),
		RequestID:       request.ID,
		CrashID:         request.CrashID,
		BotID:           re.config.ID,
		AttemptNumber:   request.AttemptCount + 1,
		Status:          common.ReproducibilityStatusFailed,
		Reproduced:      false,
		ExecutionTime:   time.Since(time.Now()),
		Signal:          0,
		ExitCode:        -1,
		Output:          output,
		StackTrace:      "",
		StackHash:       "",
		MatchesOriginal: false,
		EnvironmentInfo: map[string]string{
			"bot_id":   re.config.ID,
			"bot_name": re.config.Name,
			"error":    message,
		},
		Timestamp: time.Now(),
	}
}

// reportReproductionResult reports the reproduction result back to master
func (re *ReproducibilityExecutor) reportReproductionResult(ctx context.Context, result *common.ReproductionResult) error {
	return re.client.ReportReproductionResult(result)
}

// GetActiveReproductions returns currently active reproductions
func (re *ReproducibilityExecutor) GetActiveReproductions() map[string]*ReproductionExecution {
	re.mu.RLock()
	defer re.mu.RUnlock()

	result := make(map[string]*ReproductionExecution)
	for k, v := range re.activeRepros {
		result[k] = v
	}

	return result
}

// StopReproduction stops an active reproduction
func (re *ReproducibilityExecutor) StopReproduction(requestID string) {
	re.mu.Lock()
	defer re.mu.Unlock()

	if execution, exists := re.activeRepros[requestID]; exists {
		re.logger.WithField("request_id", requestID).Info("Stopping reproduction execution")
		execution.Cancel()
		execution.Status = "stopped"
		execution.LastUpdate = time.Now()
	}
}

// IsReproductionRunning checks if a reproduction is currently running
func (re *ReproducibilityExecutor) IsReproductionRunning(requestID string) bool {
	re.mu.RLock()
	defer re.mu.RUnlock()

	_, exists := re.activeRepros[requestID]
	return exists
}

// HandleReproductionRequest handles a reproduction request from the master
func (re *ReproducibilityExecutor) HandleReproductionRequest(request *common.ReproductionRequest) error {
	if request == nil {
		return fmt.Errorf("reproduction request is nil")
	}

	re.logger.WithFields(logrus.Fields{
		"request_id": request.ID,
		"crash_id":   request.CrashID,
		"priority":   request.Priority,
	}).Info("Received reproduction request")

	// Execute reproduction in background
	go func() {
		ctx := context.Background()
		result, err := re.ExecuteReproduction(ctx, request)
		if err != nil {
			re.logger.WithError(err).Error("Reproduction execution failed")
			// Report failure result
			failedResult := re.createFailedResult(request, "Execution failed", err)
			if reportErr := re.reportReproductionResult(ctx, failedResult); reportErr != nil {
				re.logger.WithError(reportErr).Error("Failed to report failure result")
			}
		} else if result != nil {
			re.logger.WithFields(logrus.Fields{
				"result_id":  result.ID,
				"reproduced": result.Reproduced,
			}).Info("Reproduction execution completed")
		}
	}()

	return nil
}

// GetStats returns statistics about reproduction operations
func (re *ReproducibilityExecutor) GetStats() map[string]interface{} {
	re.mu.RLock()
	defer re.mu.RUnlock()

	return map[string]interface{}{
		"active_reproductions": len(re.activeRepros),
		"config": map[string]interface{}{
			"work_directory": re.config.Fuzzing.WorkDir,
			"bot_id":         re.config.ID,
		},
	}
}
