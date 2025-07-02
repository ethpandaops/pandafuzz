package bot

import (
	"context"
	"fmt"
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
	}).Info("Starting fuzzer job execution")

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

	// Ensure cleanup on exit
	defer func() {
		fje.mu.Lock()
		delete(fje.activeFuzzers, job.ID)
		fje.mu.Unlock()

		// Clean up fuzzer
		if cleanupErr := fuzz.Cleanup(); cleanupErr != nil {
			fje.logger.WithError(cleanupErr).Warn("Failed to cleanup fuzzer")
		}
	}()

	// Configure fuzzer
	config := fuzzer.FuzzConfig{
		Target:          filepath.Join(job.WorkDir, "target_binary"),
		WorkDirectory:   job.WorkDir,
		Duration:        job.Config.Duration,
		Timeout:         job.Config.Timeout,
		MemoryLimit:     job.Config.MemoryLimit,
		SeedDirectory:   filepath.Join(job.WorkDir, "input"),
		OutputDirectory: filepath.Join(job.WorkDir, "output"),
		CrashDirectory:  filepath.Join(job.WorkDir, "crashes"),
		CorpusDirectory: filepath.Join(job.WorkDir, "corpus"),
		StatsInterval:   10 * time.Second,
		LogLevel:        fje.logger.Level.String(),
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

	// Create execution context with timeout
	var ctx context.Context
	var cancel context.CancelFunc

	if job.Config.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), job.Config.Timeout)
	} else {
		// Use duration if no timeout specified
		if job.Config.Duration > 0 {
			ctx, cancel = context.WithTimeout(context.Background(), job.Config.Duration)
		} else {
			// Default to 1 hour
			ctx, cancel = context.WithTimeout(context.Background(), time.Hour)
		}
	}
	defer cancel()

	// Start event handler goroutine
	go fje.handleFuzzerEvents(job.ID)

	// Start fuzzing
	fje.logger.WithField("job_id", job.ID).Info("Starting fuzzer")
	if err := fuzz.Start(ctx); err != nil {
		// Check if it was a context cancellation
		if ctx.Err() == context.DeadlineExceeded {
			msg := fmt.Sprintf("%s job completed (timeout/duration reached)", job.Fuzzer)
			fje.logger.WithField("job_id", job.ID).Info(msg)
			return true, msg, nil
		} else if ctx.Err() == context.Canceled {
			msg := fmt.Sprintf("%s job was cancelled", job.Fuzzer)
			fje.logger.WithField("job_id", job.ID).Info(msg)
			return false, msg, nil
		}

		msg := fmt.Sprintf("Fuzzer execution failed: %v", err)
		fje.logger.WithError(err).Error("Fuzzer execution failed")
		return false, msg, err
	}

	// Get final results
	results, err := fuzz.GetResults()
	if err != nil {
		fje.logger.WithError(err).Warn("Failed to get fuzzer results")
	} else {
		fje.logger.WithFields(logrus.Fields{
			"job_id":           job.ID,
			"total_executions": results.Summary.TotalExecutions,
			"unique_crashes":   results.Summary.UniqueCrashes,
			"coverage":         results.Summary.CoverageAchieved,
			"new_inputs":       results.Summary.NewInputsFound,
		}).Info("Fuzzing completed")
	}

	msg := fmt.Sprintf("%s execution completed successfully", job.Fuzzer)
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
