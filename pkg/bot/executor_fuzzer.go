package bot

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/fuzzer"
	"github.com/sirupsen/logrus"
)

// FuzzerJobExecutor handles job execution using the fuzzer interface
type FuzzerJobExecutor struct {
	config        *common.BotConfig
	logger        *logrus.Logger
	resultChan    chan common.FuzzerEvent
	activeFuzzers map[string]fuzzer.Fuzzer
	mu            sync.RWMutex
	botID         string
}

// NewFuzzerJobExecutor creates a new fuzzer-based job executor
func NewFuzzerJobExecutor(config *common.BotConfig, logger *logrus.Logger) *FuzzerJobExecutor {
	return &FuzzerJobExecutor{
		config:        config,
		logger:        logger,
		resultChan:    make(chan common.FuzzerEvent, 100),
		activeFuzzers: make(map[string]fuzzer.Fuzzer),
		botID:         config.ID,
	}
}

// ExecuteJob executes a fuzzing job using the appropriate fuzzer implementation
func (fje *FuzzerJobExecutor) ExecuteJob(job *common.Job) (success bool, message string, err error) {
	fje.logger.WithFields(logrus.Fields{
		"job_id":   job.ID,
		"job_name": job.Name,
		"fuzzer":   job.Fuzzer,
		"target":   job.Target,
		"work_dir": job.WorkDir,
		"config":   job.Config,
		"status":   job.Status,
	}).Info("FuzzerJobExecutor: Starting fuzzer job execution")

	// Create appropriate fuzzer
	fuzz, err := fje.createFuzzer(job)
	if err != nil {
		msg := fmt.Sprintf("Failed to create fuzzer: %v", err)
		fje.logger.WithError(err).Error("Failed to create fuzzer")
		return false, msg, err
	}

	// Store fuzzer reference
	fje.mu.Lock()
	fje.activeFuzzers[job.ID] = fuzz
	fje.mu.Unlock()

	// Note: Cleanup is now handled by CleanupJob method to allow crash detection
	// after job completion but before cleanup

	// Ensure we have absolute paths
	workDir := job.WorkDir
	if !filepath.IsAbs(workDir) {
		absWorkDir, err := filepath.Abs(workDir)
		if err != nil {
			fje.logger.WithError(err).Warn("Failed to get absolute work directory, using relative path")
		} else {
			workDir = absWorkDir
		}
	}

	// Create log file for job output
	logPath := filepath.Join(workDir, "job.log")
	logFile, err := os.Create(logPath)
	var logWriter io.Writer
	if err != nil {
		fje.logger.WithError(err).WithField("log_path", logPath).Error("Failed to create job log file")
		// Continue without log file - don't fail the job
		logWriter = nil
	} else {
		// Don't close the file yet - it needs to stay open for the fuzzer to write to it
		// Write initial log entry
		fmt.Fprintf(logFile, "%s Starting fuzzer job %s\n", time.Now().Format(time.RFC3339), job.ID)
		fmt.Fprintf(logFile, "Fuzzer: %s\n", job.Fuzzer)
		fmt.Fprintf(logFile, "Target: %s\n", job.Target)
		fmt.Fprintf(logFile, "Duration: %v\n", job.Config.Duration)
		fmt.Fprintf(logFile, "WorkDir: %s\n", workDir)
		fmt.Fprintf(logFile, "\n")
		logWriter = logFile
		fje.logger.WithField("log_path", logPath).Info("Successfully created log file for job output")
	}

	// Check if target binary exists and is executable
	targetPath := filepath.Join(workDir, "target_binary")
	if stat, err := os.Stat(targetPath); err != nil {
		fje.logger.WithError(err).WithField("target", targetPath).Error("Target binary not found")
	} else {
		fje.logger.WithFields(logrus.Fields{
			"target": targetPath,
			"size":   stat.Size(),
			"mode":   stat.Mode().String(),
		}).Debug("Target binary details")
	}

	// Configure fuzzer
	// job.Config.Timeout is already a time.Duration, no need to multiply by time.Second
	fuzzerTimeout := job.Config.Timeout
	if fuzzerTimeout <= 0 {
		fuzzerTimeout = 10 * time.Second // Default 10 second timeout for AFL++
	}

	config := fuzzer.FuzzConfig{
		JobID:           job.ID, // Set the actual job ID
		Target:          targetPath,
		WorkDirectory:   workDir,
		Duration:        job.Config.Duration,
		Timeout:         fuzzerTimeout,
		MemoryLimit:     job.Config.MemoryLimit,
		SeedDirectory:   filepath.Join(workDir, "input"),
		OutputDirectory: filepath.Join(workDir, "output"),
		CrashDirectory:  filepath.Join(workDir, "crashes"),
		CorpusDirectory: filepath.Join(workDir, "corpus"),
		StatsInterval:   10 * time.Second,
		LogLevel:        fje.logger.Level.String(),
		OutputWriter:    logWriter, // Set the log file writer
	}

	// Add dictionary if provided
	if job.Config.Dictionary != "" {
		config.Dictionary = job.Config.Dictionary
	}

	// Configure the fuzzer
	if err := fuzz.Configure(config); err != nil {
		msg := fmt.Sprintf("Failed to configure fuzzer: %v", err)
		fje.logger.WithError(err).Error("Failed to configure fuzzer")
		return false, msg, err
	}

	// Initialize fuzzer
	if err := fuzz.Initialize(); err != nil {
		msg := fmt.Sprintf("Failed to initialize fuzzer: %v", err)
		fje.logger.WithError(err).Error("Failed to initialize fuzzer")
		return false, msg, err
	}

	// Set event handler
	handler := &jobEventHandler{
		jobID:      job.ID,
		logger:     fje.logger,
		resultChan: fje.resultChan,
	}
	fuzz.SetEventHandler(handler)

	// Create execution context with timeout based on job duration
	var ctx context.Context
	var cancel context.CancelFunc

	// Use duration as the overall job timeout, not the per-test timeout
	jobTimeout := time.Hour // Default to 1 hour if not specified
	if job.Config.Duration > 0 {
		// Add some buffer time for fuzzer to start/stop gracefully
		jobTimeout = job.Config.Duration + 30*time.Second
	}

	ctx, cancel = context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()

	fje.logger.WithFields(logrus.Fields{
		"job_id":           job.ID,
		"job_timeout":      jobTimeout,
		"duration":         job.Config.Duration,
		"per_test_timeout": job.Config.Timeout,
	}).Info("Created execution context with job timeout")

	// Start event handler goroutine
	go fje.handleFuzzerEvents(job.ID)

	// Start fuzzing
	fje.logger.WithField("job_id", job.ID).Info("Starting fuzzer")
	if err := fuzz.Start(ctx); err != nil {
		msg := fmt.Sprintf("Fuzzer execution failed: %v", err)
		fje.logger.WithError(err).Error("Fuzzer execution failed")
		return false, msg, err
	}

	// Wait for fuzzer to complete or context to be done
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var timedOut bool

	for {
		select {
		case <-ctx.Done():
			// Context cancelled/timeout
			fje.logger.WithField("job_id", job.ID).Info("Context done, stopping fuzzer")
			if err := fuzz.Stop(); err != nil {
				fje.logger.WithError(err).Warn("Failed to stop fuzzer gracefully")
			}

			fje.logger.WithField("job_id", job.ID).Debug("Fuzzer stop called, waiting for it to stop")

			// Wait for fuzzer to actually stop
			stopTimeout := time.NewTimer(10 * time.Second)
			defer stopTimeout.Stop()

		stopWaitLoop:
			for fuzz.IsRunning() {
				fje.logger.WithField("job_id", job.ID).Debug("Waiting for fuzzer to stop, IsRunning=true")
				select {
				case <-stopTimeout.C:
					fje.logger.WithField("job_id", job.ID).Warn("Fuzzer did not stop within timeout")
					break stopWaitLoop
				case <-time.After(100 * time.Millisecond):
					// Check again
				}
				if !fuzz.IsRunning() {
					break
				}
			}

			fje.logger.WithField("job_id", job.ID).Debug("Exited fuzzer stop wait loop")

			if ctx.Err() == context.DeadlineExceeded {
				msg := fmt.Sprintf("%s job completed (timeout/duration reached)", job.Fuzzer)
				fje.logger.WithField("job_id", job.ID).Info(msg)
				timedOut = true
				// Exit the outer loop
				goto exitLoop
			} else {
				msg := fmt.Sprintf("%s job was cancelled", job.Fuzzer)
				fje.logger.WithField("job_id", job.ID).Info(msg)
				return false, msg, nil
			}

		case <-ticker.C:
			// Check if fuzzer is still running
			if !fuzz.IsRunning() {
				fje.logger.WithField("job_id", job.ID).Info("Fuzzer completed")
				break
			}
		}

		// Break the loop if fuzzer is not running
		if !fuzz.IsRunning() {
			break
		}
	}

exitLoop:
	fje.logger.WithField("job_id", job.ID).Debug("Reached exitLoop label")

	// Get final results only if we didn't timeout
	// Skip results collection on timeout as the fuzzer process may be in an inconsistent state
	if !timedOut {
		fje.logger.WithField("job_id", job.ID).Debug("Getting final results")

		// Add a timeout for GetResults to prevent blocking
		resultCtx, resultCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer resultCancel()

		resultChan := make(chan struct {
			results *fuzzer.FuzzerResults
			err     error
		}, 1)

		go func() {
			results, err := fuzz.GetResults()
			resultChan <- struct {
				results *fuzzer.FuzzerResults
				err     error
			}{results, err}
		}()

		select {
		case res := <-resultChan:
			if res.err != nil {
				fje.logger.WithError(res.err).Warn("Failed to get fuzzer results")
			} else if res.results != nil {
				fje.logger.WithFields(logrus.Fields{
					"job_id":           job.ID,
					"total_executions": res.results.Summary.TotalExecutions,
					"unique_crashes":   res.results.Summary.UniqueCrashes,
					"coverage":         res.results.Summary.CoverageAchieved,
					"new_inputs":       res.results.Summary.NewInputsFound,
				}).Info("Fuzzing completed")
			}
		case <-resultCtx.Done():
			fje.logger.WithField("job_id", job.ID).Warn("Timeout waiting for fuzzer results")
		}
	} else {
		fje.logger.WithField("job_id", job.ID).Info("Skipping results collection due to timeout")
	}

	fje.logger.WithField("job_id", job.ID).Debug("After getting results")

	// Close the log file if it was opened
	if logFile != nil {
		logFile.Close()
	}

	if timedOut {
		msg := fmt.Sprintf("%s execution completed (timeout reached after %v)", job.Fuzzer, job.Config.Duration)
		fje.logger.WithField("job_id", job.ID).Info("Returning from ExecuteJob with timeout")
		return true, msg, nil
	}

	msg := fmt.Sprintf("%s execution completed successfully", job.Fuzzer)
	fje.logger.WithField("job_id", job.ID).Info("Returning from ExecuteJob successfully")
	return true, msg, nil
}

// StopJob stops a running fuzzing job
func (fje *FuzzerJobExecutor) StopJob(jobID string) error {
	fje.mu.RLock()
	fuzz, exists := fje.activeFuzzers[jobID]
	fje.mu.RUnlock()

	if !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	fje.logger.WithField("job_id", jobID).Info("Stopping fuzzer job")

	// Stop the fuzzer
	if err := fuzz.Stop(); err != nil {
		fje.logger.WithError(err).Warn("Error stopping fuzzer")
		return err
	}

	return nil
}

// GetJobStatus returns the status of a running job
func (fje *FuzzerJobExecutor) GetJobStatus(jobID string) (string, bool) {
	fje.mu.RLock()
	fuzz, exists := fje.activeFuzzers[jobID]
	fje.mu.RUnlock()

	if !exists {
		return "", false
	}

	status := fuzz.GetStatus()
	return string(status), true
}

// GetEventChannel returns the event channel for receiving fuzzer events
func (fje *FuzzerJobExecutor) GetEventChannel() <-chan common.FuzzerEvent {
	return fje.resultChan
}

// GetFuzzer returns the fuzzer instance for a given job ID
func (fje *FuzzerJobExecutor) GetFuzzer(jobID string) (fuzzer.Fuzzer, bool) {
	fje.mu.RLock()
	defer fje.mu.RUnlock()

	fuzz, exists := fje.activeFuzzers[jobID]
	return fuzz, exists
}

// CleanupJob removes the fuzzer from active jobs and performs cleanup
// This should be called AFTER crash detection is complete
func (fje *FuzzerJobExecutor) CleanupJob(jobID string) error {
	fje.mu.Lock()
	fuzz, exists := fje.activeFuzzers[jobID]
	if exists {
		delete(fje.activeFuzzers, jobID)
	}
	fje.mu.Unlock()

	if !exists {
		fje.logger.WithField("job_id", jobID).Debug("Job already cleaned up")
		return nil
	}

	fje.logger.WithField("job_id", jobID).Debug("Cleaning up fuzzer")

	// Clean up fuzzer
	if err := fuzz.Cleanup(); err != nil {
		fje.logger.WithError(err).WithField("job_id", jobID).Warn("Failed to cleanup fuzzer")
		return err
	}

	return nil
}

// createFuzzer creates the appropriate fuzzer instance based on job configuration
func (fje *FuzzerJobExecutor) createFuzzer(job *common.Job) (fuzzer.Fuzzer, error) {
	switch job.Fuzzer {
	case "aflplusplus", "afl++", "afl":
		aflFuzz := fuzzer.NewAFLPlusPlus(fje.logger)
		aflFuzz.SetBotID(fje.botID)
		return aflFuzz, nil

	case "libfuzzer":
		libFuzz := fuzzer.NewLibFuzzer(fje.logger)
		libFuzz.SetBotID(fje.botID)
		return libFuzz, nil

	case "honggfuzz":
		honggFuzz := fuzzer.NewHonggfuzz(fje.logger)
		honggFuzz.SetBotID(fje.botID)
		return honggFuzz, nil

	default:
		return nil, fmt.Errorf("unsupported fuzzer type: %s", job.Fuzzer)
	}
}

// handleFuzzerEvents processes events from the fuzzer
func (fje *FuzzerJobExecutor) handleFuzzerEvents(jobID string) {
	fje.logger.WithField("job_id", jobID).Debug("Started fuzzer event handler")

	// This method monitors the result channel for events from this job
	// In a real implementation, events would be sent to the master node
	// For now, we just log them

	timeout := time.NewTimer(24 * time.Hour) // Maximum event handler lifetime
	defer timeout.Stop()

	for {
		select {
		case event := <-fje.resultChan:
			if event.JobID == jobID {
				fje.logger.WithFields(logrus.Fields{
					"job_id": jobID,
					"type":   event.Type,
					"data":   event.Data,
				}).Debug("Received fuzzer event")

				// Handle specific event types
				switch event.Type {
				case common.FuzzerEventCrashFound:
					fje.logger.WithField("job_id", jobID).Info("Crash found by fuzzer")
				case common.FuzzerEventCoverage:
					fje.logger.WithField("job_id", jobID).Debug("Coverage update received")
				case common.FuzzerEventError:
					fje.logger.WithField("job_id", jobID).Error("Fuzzer reported error")
				}
			}

		case <-timeout.C:
			fje.logger.WithField("job_id", jobID).Warn("Event handler timeout reached")
			return
		}

		// Check if fuzzer is still active
		fje.mu.RLock()
		_, exists := fje.activeFuzzers[jobID]
		fje.mu.RUnlock()

		if !exists {
			fje.logger.WithField("job_id", jobID).Debug("Fuzzer no longer active, stopping event handler")
			return
		}
	}
}

// jobEventHandler implements the fuzzer.EventHandler interface
type jobEventHandler struct {
	jobID      string
	logger     *logrus.Logger
	resultChan chan common.FuzzerEvent
}

func (h *jobEventHandler) OnStart(fuzz fuzzer.Fuzzer) {
	h.logger.WithFields(logrus.Fields{
		"job_id": h.jobID,
		"fuzzer": fuzz.Name(),
	}).Info("Fuzzer started")

	h.resultChan <- common.FuzzerEvent{
		Type:      common.FuzzerEventStarted,
		Timestamp: time.Now(),
		JobID:     h.jobID,
		Data: map[string]interface{}{
			"fuzzer": fuzz.Name(),
			"type":   fuzz.Type(),
		},
	}
}

func (h *jobEventHandler) OnStop(fuzz fuzzer.Fuzzer, reason string) {
	h.logger.WithFields(logrus.Fields{
		"job_id": h.jobID,
		"fuzzer": fuzz.Name(),
		"reason": reason,
	}).Info("Fuzzer stopped")

	h.resultChan <- common.FuzzerEvent{
		Type:      common.FuzzerEventStopped,
		Timestamp: time.Now(),
		JobID:     h.jobID,
		Data: map[string]interface{}{
			"fuzzer": fuzz.Name(),
			"reason": reason,
		},
	}
}

func (h *jobEventHandler) OnCrash(fuzz fuzzer.Fuzzer, crash *common.CrashResult) {
	h.logger.WithFields(logrus.Fields{
		"job_id":   h.jobID,
		"fuzzer":   fuzz.Name(),
		"crash_id": crash.ID,
		"type":     crash.Type,
		"signal":   crash.Signal,
	}).Warn("Crash detected")

	// Send the full crash result including input data
	h.resultChan <- common.FuzzerEvent{
		Type:      common.FuzzerEventCrashFound,
		Timestamp: time.Now(),
		JobID:     h.jobID,
		Data: map[string]interface{}{
			"crash": crash, // Send the full crash result object
		},
	}
}

func (h *jobEventHandler) OnNewPath(fuzz fuzzer.Fuzzer, path *fuzzer.CorpusEntry) {
	h.logger.WithFields(logrus.Fields{
		"job_id":   h.jobID,
		"fuzzer":   fuzz.Name(),
		"path_id":  path.ID,
		"coverage": len(path.Coverage),
	}).Debug("New path discovered")

	h.resultChan <- common.FuzzerEvent{
		Type:      common.FuzzerEventCorpusUpdate,
		Timestamp: time.Now(),
		JobID:     h.jobID,
		Data: map[string]interface{}{
			"path_id":   path.ID,
			"file_name": path.FileName,
			"size":      path.Size,
			"coverage":  len(path.Coverage),
		},
	}
}

func (h *jobEventHandler) OnStats(fuzz fuzzer.Fuzzer, stats fuzzer.FuzzerStats) {
	h.logger.WithFields(logrus.Fields{
		"job_id":          h.jobID,
		"fuzzer":          fuzz.Name(),
		"executions":      stats.Executions,
		"exec_per_second": stats.ExecPerSecond,
		"coverage":        stats.CoveragePercent,
		"crashes":         stats.UniqueCrashes,
	}).Debug("Fuzzer stats update")

	h.resultChan <- common.FuzzerEvent{
		Type:      common.FuzzerEventStats,
		Timestamp: time.Now(),
		JobID:     h.jobID,
		Data: map[string]interface{}{
			"executions":       stats.Executions,
			"exec_per_second":  stats.ExecPerSecond,
			"coverage_percent": stats.CoveragePercent,
			"unique_crashes":   stats.UniqueCrashes,
			"corpus_size":      stats.CorpusSize,
			"cpu_usage":        stats.CPUUsage,
			"memory_usage":     stats.MemoryUsage,
		},
	}
}

func (h *jobEventHandler) OnError(fuzz fuzzer.Fuzzer, err error) {
	h.logger.WithFields(logrus.Fields{
		"job_id": h.jobID,
		"fuzzer": fuzz.Name(),
		"error":  err.Error(),
	}).Error("Fuzzer error")

	h.resultChan <- common.FuzzerEvent{
		Type:      common.FuzzerEventError,
		Timestamp: time.Now(),
		JobID:     h.jobID,
		Data: map[string]interface{}{
			"fuzzer": fuzz.Name(),
			"error":  err.Error(),
		},
	}
}

func (h *jobEventHandler) OnProgress(fuzz fuzzer.Fuzzer, progress fuzzer.FuzzerProgress) {
	h.logger.WithFields(logrus.Fields{
		"job_id":   h.jobID,
		"fuzzer":   fuzz.Name(),
		"phase":    progress.Phase,
		"progress": progress.ProgressPercent,
	}).Debug("Fuzzer progress update")

	// Progress updates can be very frequent, so we might not send all of them
	// Only send significant progress updates
	if int(progress.ProgressPercent)%10 == 0 {
		h.resultChan <- common.FuzzerEvent{
			Type:      common.FuzzerEventStats,
			Timestamp: time.Now(),
			JobID:     h.jobID,
			Data: map[string]interface{}{
				"phase":            progress.Phase,
				"progress_percent": progress.ProgressPercent,
				"queue_position":   progress.QueuePosition,
				"queue_size":       progress.QueueSize,
			},
		}
	}
}
