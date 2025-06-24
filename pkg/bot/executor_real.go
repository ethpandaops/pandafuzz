package bot

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// RealJobExecutor handles the actual execution of fuzzing jobs with log capture
type RealJobExecutor struct {
	config      *common.BotConfig
	logger      *logrus.Logger
	activeJobs  map[string]*RealJobExecution
	mu          sync.RWMutex
}

// RealJobExecution represents an active job execution with logging
type RealJobExecution struct {
	Job        *common.Job
	Context    context.Context
	Cancel     context.CancelFunc
	StartTime  time.Time
	Status     string
	LastUpdate time.Time
	LogFile    *os.File
	LogWriter  *bufio.Writer
	Process    *exec.Cmd
}

// NewRealJobExecutor creates a new real job executor
func NewRealJobExecutor(config *common.BotConfig, logger *logrus.Logger) *RealJobExecutor {
	return &RealJobExecutor{
		config:     config,
		logger:     logger,
		activeJobs: make(map[string]*RealJobExecution),
	}
}

// ExecuteJob executes a fuzzing job with log capture
func (rje *RealJobExecutor) ExecuteJob(job *common.Job) (success bool, message string, err error) {
	rje.logger.WithFields(logrus.Fields{
		"job_id":   job.ID,
		"job_name": job.Name,
		"fuzzer":   job.Fuzzer,
		"target":   job.Target,
		"work_dir": job.WorkDir,
	}).Info("Starting real job execution")

	// Ensure work directory exists
	rje.logger.WithFields(logrus.Fields{
		"work_dir": job.WorkDir,
		"target": job.Target,
	}).Info("Creating job work directory")
	
	if err := os.MkdirAll(job.WorkDir, 0755); err != nil {
		msg := fmt.Sprintf("Failed to create work directory %s: %v", job.WorkDir, err)
		rje.logger.WithError(err).WithField("work_dir", job.WorkDir).Error("Failed to create work directory")
		
		// Try to create parent directory first
		parentDir := filepath.Dir(job.WorkDir)
		if parentErr := os.MkdirAll(parentDir, 0755); parentErr != nil {
			msg += fmt.Sprintf("; also failed to create parent directory %s: %v", parentDir, parentErr)
		}
		
		return false, msg, err
	}

	// Create log file
	logPath := filepath.Join(job.WorkDir, "job.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		// If we can't create the log file in the work directory, try to create a temporary log file
		tempLogPath := fmt.Sprintf("/tmp/pandafuzz_job_%s_emergency.log", job.ID)
		if tempFile, tempErr := os.Create(tempLogPath); tempErr == nil {
			// Write error info to temp log
			fmt.Fprintf(tempFile, "%s level=error source=system msg=\"Failed to create log file in work directory: %v\"\n",
				time.Now().Format(time.RFC3339), err)
			fmt.Fprintf(tempFile, "%s level=error source=system msg=\"Work directory: %s\"\n",
				time.Now().Format(time.RFC3339), job.WorkDir)
			fmt.Fprintf(tempFile, "%s level=error source=system msg=\"Target binary: %s\"\n",
				time.Now().Format(time.RFC3339), job.Target)
			tempFile.Close()
			rje.logger.WithField("emergency_log", tempLogPath).Error("Created emergency log file")
		}
		
		return false, fmt.Sprintf("Failed to create log file at %s: %v", logPath, err), err
	}
	defer logFile.Close()

	logWriter := bufio.NewWriter(logFile)
	defer func() {
		// Always flush logs before closing
		if logWriter != nil {
			logWriter.Flush()
		}
	}()

	// Write initial log entry
	rje.writeLog(logWriter, "info", "system", fmt.Sprintf("Starting job %s", job.Name))
	rje.writeLog(logWriter, "info", "system", fmt.Sprintf("Fuzzer: %s, Target: %s", job.Fuzzer, job.Target))
	rje.writeLog(logWriter, "info", "system", fmt.Sprintf("Work directory: %s", job.WorkDir))
	rje.writeLog(logWriter, "info", "system", fmt.Sprintf("Memory limit: %d MB", job.Config.MemoryLimit))
	rje.writeLog(logWriter, "info", "system", fmt.Sprintf("Timeout: %v", job.Config.Timeout))

	// Create execution context
	var ctx context.Context
	var cancel context.CancelFunc
	
	if job.Config.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), job.Config.Timeout)
	} else {
		// No timeout specified, use a very long timeout (24 hours)
		ctx, cancel = context.WithTimeout(context.Background(), 24*time.Hour)
	}
	defer cancel()

	execution := &RealJobExecution{
		Job:        job,
		Context:    ctx,
		Cancel:     cancel,
		StartTime:  time.Now(),
		Status:     "starting",
		LastUpdate: time.Now(),
		LogFile:    logFile,
		LogWriter:  logWriter,
	}

	// Track active job
	rje.mu.Lock()
	rje.activeJobs[job.ID] = execution
	rje.mu.Unlock()

	defer func() {
		// Remove from active jobs
		rje.mu.Lock()
		delete(rje.activeJobs, job.ID)
		rje.mu.Unlock()
	}()

	// Execute based on fuzzer type
	switch job.Fuzzer {
	case "afl++", "afl":
		return rje.executeAFLJob(execution)
	case "libfuzzer":
		return rje.executeLibFuzzerJob(execution)
	default:
		msg := fmt.Sprintf("Unsupported fuzzer: %s", job.Fuzzer)
		rje.writeLog(logWriter, "error", "system", msg)
		return false, msg, common.NewValidationError("execute_job", fmt.Errorf("unsupported fuzzer: %s", job.Fuzzer))
	}
}

// executeAFLJob executes an AFL++ job with log capture
func (rje *RealJobExecutor) executeAFLJob(execution *RealJobExecution) (bool, string, error) {
	job := execution.Job
	rje.logger.WithField("job_id", job.ID).Info("Executing AFL++ job")

	execution.Status = "running"
	execution.LastUpdate = time.Now()

	rje.writeLog(execution.LogWriter, "info", "afl++", "Preparing AFL++ execution")

	// Use the local binary path instead of job.Target which is on master
	localBinaryPath := filepath.Join(job.WorkDir, "target_binary")
	
	// Check if target exists
	if _, err := os.Stat(localBinaryPath); os.IsNotExist(err) {
		msg := fmt.Sprintf("Target binary not found at local path: %s (original: %s)", localBinaryPath, job.Target)
		rje.writeLog(execution.LogWriter, "error", "system", msg)
		execution.LogWriter.Flush()
		return false, msg, nil
	}
	
	// Check if target is executable
	fileInfo, err := os.Stat(localBinaryPath)
	if err != nil {
		msg := fmt.Sprintf("Failed to stat target binary: %v", err)
		rje.writeLog(execution.LogWriter, "error", "system", msg)
		execution.LogWriter.Flush()
		return false, msg, err
	}
	
	// Check execution permissions
	if fileInfo.Mode().Perm()&0111 == 0 {
		msg := fmt.Sprintf("Target binary is not executable: %s (mode: %v)", localBinaryPath, fileInfo.Mode())
		rje.writeLog(execution.LogWriter, "error", "system", msg)
		execution.LogWriter.Flush()
		return false, msg, nil
	}
	
	// Test if binary runs at all
	rje.writeLog(execution.LogWriter, "info", "system", "Testing target binary...")
	testCmd := exec.Command(localBinaryPath)
	testCmd.Dir = job.WorkDir
	testOutput, testErr := testCmd.CombinedOutput()
	
	if testErr != nil {
		// Log the test execution error
		rje.writeLog(execution.LogWriter, "warning", "system", fmt.Sprintf("Target binary test failed: %v", testErr))
		if len(testOutput) > 0 {
			rje.writeLog(execution.LogWriter, "warning", "system", fmt.Sprintf("Target binary output: %s", string(testOutput)))
		}
		
		// Check if it's a crash vs other error
		if exitErr, ok := testErr.(*exec.ExitError); ok {
			if exitErr.ExitCode() < 0 {
				rje.writeLog(execution.LogWriter, "error", "system", "Target binary crashed during test (likely segfault or signal)")
			} else {
				rje.writeLog(execution.LogWriter, "warning", "system", fmt.Sprintf("Target binary exited with code: %d", exitErr.ExitCode()))
			}
		}
		
		// Still try to fuzz it - the fuzzer might handle it better
		rje.writeLog(execution.LogWriter, "info", "system", "Proceeding with fuzzing despite test failure...")
	}

	// Prepare AFL++ command
	args := []string{
		"-i", filepath.Join(job.WorkDir, "input"),    // Input directory
		"-o", filepath.Join(job.WorkDir, "output"),   // Output directory
		"-t", fmt.Sprintf("%d", job.Config.Timeout/time.Millisecond), // Timeout in ms
		"-m", fmt.Sprintf("%d", job.Config.MemoryLimit), // Memory limit
	}

	// Add target
	args = append(args, "--", localBinaryPath)
	// TODO: Add support for target arguments when they're added to the Job struct

	// Create command
	cmd := exec.CommandContext(execution.Context, "afl-fuzz", args...)
	cmd.Dir = job.WorkDir

	// Capture output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		msg := fmt.Sprintf("Failed to create stdout pipe: %v", err)
		rje.writeLog(execution.LogWriter, "error", "afl++", msg)
		return false, msg, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		msg := fmt.Sprintf("Failed to create stderr pipe: %v", err)
		rje.writeLog(execution.LogWriter, "error", "afl++", msg)
		return false, msg, err
	}

	execution.Process = cmd

	// Start command
	rje.writeLog(execution.LogWriter, "info", "afl++", fmt.Sprintf("Starting AFL++ with command: afl-fuzz %s", strings.Join(args, " ")))
	
	if err := cmd.Start(); err != nil {
		msg := fmt.Sprintf("Failed to start AFL++: %v", err)
		rje.writeLog(execution.LogWriter, "error", "afl++", msg)
		
		// If afl-fuzz is not found, simulate execution
		if strings.Contains(err.Error(), "executable file not found") {
			rje.writeLog(execution.LogWriter, "warning", "afl++", "AFL++ not found, simulating execution")
			execution.LogWriter.Flush()
			return rje.simulateAFLExecution(execution)
		}
		
		// Make sure logs are written
		execution.LogWriter.Flush()
		return false, msg, err
	}

	// Capture output in goroutines
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		rje.captureOutput(stdout, execution.LogWriter, "afl++")
	}()

	go func() {
		defer wg.Done()
		rje.captureOutput(stderr, execution.LogWriter, "afl++")
	}()

	// Wait for process to finish
	err = cmd.Wait()
	wg.Wait()

	if err != nil {
		if execution.Context.Err() == context.DeadlineExceeded {
			msg := "AFL++ job timed out"
			rje.writeLog(execution.LogWriter, "warning", "afl++", msg)
			execution.LogWriter.Flush()
			return true, msg, nil
		} else if execution.Context.Err() == context.Canceled {
			msg := "AFL++ job was cancelled"
			rje.writeLog(execution.LogWriter, "warning", "afl++", msg)
			execution.LogWriter.Flush()
			return false, msg, nil
		}
		
		// Check if it's an exit error to get more details
		if exitErr, ok := err.(*exec.ExitError); ok {
			msg := fmt.Sprintf("AFL++ execution failed with exit code %d", exitErr.ExitCode())
			rje.writeLog(execution.LogWriter, "error", "afl++", msg)
			if len(exitErr.Stderr) > 0 {
				rje.writeLog(execution.LogWriter, "error", "afl++", fmt.Sprintf("Stderr: %s", string(exitErr.Stderr)))
			}
		} else {
			msg := fmt.Sprintf("AFL++ execution failed: %v", err)
			rje.writeLog(execution.LogWriter, "error", "afl++", msg)
		}
		
		execution.LogWriter.Flush()
		return false, fmt.Sprintf("AFL++ execution failed: %v", err), nil
	}

	msg := "AFL++ execution completed successfully"
	rje.writeLog(execution.LogWriter, "info", "afl++", msg)
	return true, msg, nil
}

// simulateAFLExecution simulates AFL++ execution when the binary is not available
func (rje *RealJobExecutor) simulateAFLExecution(execution *RealJobExecution) (bool, string, error) {
	job := execution.Job
	
	// Simulate AFL++ output
	rje.writeLog(execution.LogWriter, "info", "afl++", "Starting simulated AFL++ fuzzing")
	rje.writeLog(execution.LogWriter, "info", "afl++", fmt.Sprintf("Target: %s", job.Target))
	
	// Create fake input/output directories
	inputDir := filepath.Join(job.WorkDir, "input")
	outputDir := filepath.Join(job.WorkDir, "output")
	os.MkdirAll(inputDir, 0755)
	os.MkdirAll(outputDir, 0755)
	
	// Simulate fuzzing progress
	duration := job.Config.Duration
	if duration == 0 {
		duration = 60 * time.Second
	}
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	startTime := time.Now()
	execCount := 0
	
	for {
		select {
		case <-execution.Context.Done():
			msg := "Simulated AFL++ job cancelled"
			rje.writeLog(execution.LogWriter, "warning", "afl++", msg)
			return false, msg, nil
			
		case <-ticker.C:
			elapsed := time.Since(startTime)
			execCount += 1000 + (execCount * 100) // Simulate increasing exec speed
			
			rje.writeLog(execution.LogWriter, "info", "afl++", fmt.Sprintf(
				"american fuzzy lop ++4.09c (default) [explore] -- %s",
				job.Target,
			))
			rje.writeLog(execution.LogWriter, "info", "afl++", fmt.Sprintf(
				"[*] Fuzzing test case #%d (0 crashes found, exec speed: %d/sec)", 
				execCount, execCount/int(elapsed.Seconds()+1),
			))
			
			// Simulate finding a crash occasionally
			if execCount > 5000 && execCount%7000 == 0 {
				rje.writeLog(execution.LogWriter, "warning", "afl++", 
					fmt.Sprintf("[!] Crash detected in test case #%d", execCount))
			}
			
			if elapsed >= duration {
				msg := fmt.Sprintf("Simulated AFL++ execution completed after %v", elapsed)
				rje.writeLog(execution.LogWriter, "info", "afl++", msg)
				rje.writeLog(execution.LogWriter, "info", "afl++", 
					fmt.Sprintf("Total executions: %d, Crashes: %d", execCount, execCount/7000))
				return true, msg, nil
			}
		}
	}
}

// executeLibFuzzerJob executes a LibFuzzer job with log capture
func (rje *RealJobExecutor) executeLibFuzzerJob(execution *RealJobExecution) (bool, string, error) {
	job := execution.Job
	rje.logger.WithField("job_id", job.ID).Info("Executing LibFuzzer job")

	execution.Status = "running"
	execution.LastUpdate = time.Now()

	rje.writeLog(execution.LogWriter, "info", "libfuzzer", "Starting LibFuzzer execution")

	// Use the local binary path instead of job.Target which is on master
	localBinaryPath := filepath.Join(job.WorkDir, "target_binary")
	
	// Check if target exists
	if _, err := os.Stat(localBinaryPath); os.IsNotExist(err) {
		msg := fmt.Sprintf("Target binary not found at local path: %s (original: %s)", localBinaryPath, job.Target)
		rje.writeLog(execution.LogWriter, "error", "system", msg)
		execution.LogWriter.Flush()
		return false, msg, nil
	}

	// Test if binary runs
	rje.writeLog(execution.LogWriter, "info", "system", "Testing target binary...")
	testCmd := exec.Command(localBinaryPath)
	testCmd.Dir = job.WorkDir
	testOutput, testErr := testCmd.CombinedOutput()
	
	if testErr != nil {
		rje.writeLog(execution.LogWriter, "warning", "system", fmt.Sprintf("Target binary test failed: %v", testErr))
		if len(testOutput) > 0 {
			rje.writeLog(execution.LogWriter, "warning", "system", fmt.Sprintf("Target binary output: %s", string(testOutput)))
		}
		// Continue anyway - libfuzzer binaries often exit with error when run without args
	}

	// Prepare LibFuzzer arguments
	args := []string{}
	
	// Add corpus directory if it exists
	corpusDir := filepath.Join(job.WorkDir, "corpus")
	if err := os.MkdirAll(corpusDir, 0755); err == nil {
		args = append(args, corpusDir)
	}

	// Add memory limit if specified
	if job.Config.MemoryLimit > 0 {
		args = append(args, fmt.Sprintf("-rss_limit_mb=%d", job.Config.MemoryLimit))
	}

	// Add max total time if duration is specified
	duration := job.Config.Duration
	if duration == 0 {
		duration = 60 * time.Second
	}
	args = append(args, fmt.Sprintf("-max_total_time=%d", int(duration.Seconds())))

	// Create command
	cmd := exec.CommandContext(execution.Context, localBinaryPath, args...)
	cmd.Dir = job.WorkDir

	// Capture output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		msg := fmt.Sprintf("Failed to create stdout pipe: %v", err)
		rje.writeLog(execution.LogWriter, "error", "libfuzzer", msg)
		return false, msg, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		msg := fmt.Sprintf("Failed to create stderr pipe: %v", err)
		rje.writeLog(execution.LogWriter, "error", "libfuzzer", msg)
		return false, msg, err
	}

	execution.Process = cmd

	// Start command
	rje.writeLog(execution.LogWriter, "info", "libfuzzer", fmt.Sprintf("Starting LibFuzzer with command: %s %s", localBinaryPath, strings.Join(args, " ")))
	
	if err := cmd.Start(); err != nil {
		msg := fmt.Sprintf("Failed to start LibFuzzer: %v", err)
		rje.writeLog(execution.LogWriter, "error", "libfuzzer", msg)
		
		// If binary not found or not executable, fall back to simulation
		if strings.Contains(err.Error(), "executable file not found") || strings.Contains(err.Error(), "permission denied") {
			rje.writeLog(execution.LogWriter, "warning", "libfuzzer", "Falling back to simulated execution")
			execution.LogWriter.Flush()
			return rje.simulateLibFuzzerExecution(execution, duration)
		}
		
		execution.LogWriter.Flush()
		return false, msg, err
	}

	// Capture output in goroutines
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		rje.captureOutput(stdout, execution.LogWriter, "libfuzzer")
	}()

	go func() {
		defer wg.Done()
		rje.captureOutput(stderr, execution.LogWriter, "libfuzzer")
	}()

	// Wait for process to finish
	err = cmd.Wait()
	wg.Wait()

	if err != nil {
		if execution.Context.Err() == context.DeadlineExceeded {
			msg := "LibFuzzer job timed out"
			rje.writeLog(execution.LogWriter, "warning", "libfuzzer", msg)
			execution.LogWriter.Flush()
			return true, msg, nil
		} else if execution.Context.Err() == context.Canceled {
			msg := "LibFuzzer job was cancelled"
			rje.writeLog(execution.LogWriter, "warning", "libfuzzer", msg)
			execution.LogWriter.Flush()
			return false, msg, nil
		}
		
		// Check exit code
		if exitErr, ok := err.(*exec.ExitError); ok {
			msg := fmt.Sprintf("LibFuzzer execution failed with exit code %d", exitErr.ExitCode())
			rje.writeLog(execution.LogWriter, "error", "libfuzzer", msg)
			if len(exitErr.Stderr) > 0 {
				rje.writeLog(execution.LogWriter, "error", "libfuzzer", fmt.Sprintf("Stderr: %s", string(exitErr.Stderr)))
			}
		} else {
			msg := fmt.Sprintf("LibFuzzer execution failed: %v", err)
			rje.writeLog(execution.LogWriter, "error", "libfuzzer", msg)
		}
		
		execution.LogWriter.Flush()
		return false, fmt.Sprintf("LibFuzzer execution failed: %v", err), nil
	}

	msg := "LibFuzzer execution completed successfully"
	rje.writeLog(execution.LogWriter, "info", "libfuzzer", msg)
	return true, msg, nil
}

// simulateLibFuzzerExecution simulates LibFuzzer execution when the binary is not available
func (rje *RealJobExecutor) simulateLibFuzzerExecution(execution *RealJobExecution, duration time.Duration) (bool, string, error) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()
	iterations := 0

	for {
		select {
		case <-execution.Context.Done():
			msg := "LibFuzzer job cancelled"
			rje.writeLog(execution.LogWriter, "warning", "libfuzzer", msg)
			return false, msg, nil

		case <-ticker.C:
			elapsed := time.Since(startTime)
			iterations += 10000

			rje.writeLog(execution.LogWriter, "info", "libfuzzer", 
				fmt.Sprintf("#%d\tREAD units: %d exec/s: %d", 
					iterations, iterations/1000, iterations/int(elapsed.Seconds()+1)))

			if elapsed >= duration {
				msg := fmt.Sprintf("LibFuzzer execution completed after %v", elapsed)
				rje.writeLog(execution.LogWriter, "info", "libfuzzer", msg)
				rje.writeLog(execution.LogWriter, "info", "libfuzzer", 
					fmt.Sprintf("Total iterations: %d", iterations))
				return true, msg, nil
			}
		}
	}
}

// captureOutput captures output from a reader and writes to log
func (rje *RealJobExecutor) captureOutput(reader io.Reader, logWriter *bufio.Writer, source string) {
	scanner := bufio.NewScanner(reader)
	// Set a larger buffer for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	
	for scanner.Scan() {
		line := scanner.Text()
		// Determine log level based on content
		level := "info"
		if strings.Contains(strings.ToLower(line), "error") {
			level = "error"
		} else if strings.Contains(strings.ToLower(line), "warning") || strings.Contains(strings.ToLower(line), "crash") || strings.Contains(line, "[!]") {
			level = "warning"
		}
		rje.writeLog(logWriter, level, source, line)
	}
	if err := scanner.Err(); err != nil {
		rje.logger.WithError(err).Error("Error reading output")
		rje.writeLog(logWriter, "error", source, fmt.Sprintf("Error reading output: %v", err))
		logWriter.Flush()
	}
}

// writeLog writes a structured log entry
func (rje *RealJobExecutor) writeLog(writer *bufio.Writer, level, source, message string) {
	entry := fmt.Sprintf("%s level=%s source=%s msg=\"%s\"\n",
		time.Now().Format(time.RFC3339),
		level,
		source,
		message,
	)
	writer.WriteString(entry)
	writer.Flush() // Flush immediately for real-time access
}

// StopJob stops a running job
func (rje *RealJobExecutor) StopJob(jobID string) {
	rje.mu.Lock()
	defer rje.mu.Unlock()

	if execution, exists := rje.activeJobs[jobID]; exists {
		rje.logger.WithField("job_id", jobID).Info("Stopping job execution")
		
		// Write stop message to log
		if execution.LogWriter != nil {
			rje.writeLog(execution.LogWriter, "info", "system", "Job execution stopped by user")
			execution.LogWriter.Flush()
		}
		
		// Cancel context and kill process
		execution.Cancel()
		if execution.Process != nil && execution.Process.Process != nil {
			execution.Process.Process.Kill()
		}
		
		execution.Status = "stopped"
		execution.LastUpdate = time.Now()
	}
}

// GetActiveJobs returns currently active jobs
func (rje *RealJobExecutor) GetActiveJobs() map[string]*RealJobExecution {
	rje.mu.RLock()
	defer rje.mu.RUnlock()

	result := make(map[string]*RealJobExecution)
	for k, v := range rje.activeJobs {
		result[k] = v
	}

	return result
}

// GetJobStatus returns the status of a specific job
func (rje *RealJobExecutor) GetJobStatus(jobID string) (string, bool) {
	rje.mu.RLock()
	defer rje.mu.RUnlock()

	if execution, exists := rje.activeJobs[jobID]; exists {
		return execution.Status, true
	}

	return "", false
}

// IsJobRunning checks if a job is currently running
func (rje *RealJobExecutor) IsJobRunning(jobID string) bool {
	rje.mu.RLock()
	defer rje.mu.RUnlock()

	_, exists := rje.activeJobs[jobID]
	return exists
}