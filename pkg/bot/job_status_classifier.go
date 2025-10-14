package bot

import (
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// JobOutcome represents the classification of a job's completion
type JobOutcome struct {
	Success bool
	Message string
	Reason  string
}

// JobStatusClassifier determines the proper status for completed jobs
type JobStatusClassifier struct {
	logger logrus.FieldLogger
}

// NewJobStatusClassifier creates a new job status classifier
func NewJobStatusClassifier(logger logrus.FieldLogger) *JobStatusClassifier {
	return &JobStatusClassifier{
		logger: logger.WithField("component", "job_status_classifier"),
	}
}

// ClassifyJobOutcome determines if a job should be marked as completed or failed
func (c *JobStatusClassifier) ClassifyJobOutcome(
	jobID string,
	executionError error,
	duration time.Duration,
	crashesFound int,
	expectedDuration time.Duration,
) JobOutcome {
	// Rule 1: If crashes were found, the job is successful regardless of how it exited
	if crashesFound > 0 {
		c.logger.WithFields(logrus.Fields{
			"job_id":  jobID,
			"crashes": crashesFound,
		}).Info("Job found crashes - marking as completed")

		return JobOutcome{
			Success: true,
			Message: fmt.Sprintf("Fuzzing completed successfully - found %d crashes", crashesFound),
			Reason:  "crashes_found",
		}
	}

	// Rule 2: If no error, job completed successfully
	if executionError == nil {
		c.logger.WithField("job_id", jobID).Info("Job completed without errors")
		return JobOutcome{
			Success: true,
			Message: "Fuzzing completed successfully",
			Reason:  "clean_exit",
		}
	}

	errStr := executionError.Error()
	errLower := strings.ToLower(errStr)

	// Rule 3: Check if this was an expected termination (timeout, duration reached, orchestrated stop)
	if c.isExpectedTermination(errLower, duration, expectedDuration) {
		c.logger.WithFields(logrus.Fields{
			"job_id":   jobID,
			"duration": duration,
			"error":    errStr,
		}).Info("Job terminated as expected - marking as completed")

		return JobOutcome{
			Success: true,
			Message: "Fuzzing completed (duration reached)",
			Reason:  "expected_termination",
		}
	}

	// Rule 4: Check if the fuzzer harness itself crashed (immediate failure)
	if c.isHarnessCrash(errLower, duration) {
		c.logger.WithFields(logrus.Fields{
			"job_id":   jobID,
			"duration": duration,
			"error":    errStr,
		}).Error("Fuzzer harness crashed - marking as failed")

		return JobOutcome{
			Success: false,
			Message: fmt.Sprintf("Fuzzer harness failed: %v", executionError),
			Reason:  "harness_crash",
		}
	}

	// Rule 5: Default - if we can't classify it, mark as failed
	c.logger.WithFields(logrus.Fields{
		"job_id": jobID,
		"error":  errStr,
	}).Warn("Unable to classify job outcome - marking as failed")

	return JobOutcome{
		Success: false,
		Message: fmt.Sprintf("Job failed: %v", executionError),
		Reason:  "unknown_error",
	}
}

// isExpectedTermination checks if the error indicates an expected job termination
func (c *JobStatusClassifier) isExpectedTermination(errLower string, duration, expectedDuration time.Duration) bool {
	// Check for signals that indicate orchestrated termination
	expectedSignals := []string{
		"signal: killed",
		"signal: terminated",
		"signal: interrupt",
		"sigterm",
		"sigint",
		"cancelled",
		"context deadline exceeded",
		"timeout",
		"duration reached",
		"user requested",
	}

	for _, signal := range expectedSignals {
		if strings.Contains(errLower, signal) {
			// Additional check: if duration is close to expected, it's likely a normal timeout
			if expectedDuration > 0 {
				tolerance := 5 * time.Second
				if duration >= expectedDuration-tolerance {
					return true
				}
			}
			// Even without matching duration, these signals usually indicate orchestrated stops
			return true
		}
	}

	// AFL++ specific: check for normal completion indicators
	if strings.Contains(errLower, "afl++") && strings.Contains(errLower, "completed") {
		return true
	}

	return false
}

// isHarnessCrash checks if the error indicates the fuzzer harness itself crashed
func (c *JobStatusClassifier) isHarnessCrash(errLower string, duration time.Duration) bool {
	// If the fuzzer crashed within the first few seconds, it's likely a harness issue
	quickCrashThreshold := 5 * time.Second

	crashIndicators := []string{
		"segmentation fault",
		"sigsegv",
		"sigabrt",
		"sigbus",
		"sigfpe",
		"sigill",
		"core dumped",
		"assertion failed",
	}

	for _, indicator := range crashIndicators {
		if strings.Contains(errLower, indicator) {
			// If it crashed very quickly, it's likely the harness not the target
			if duration < quickCrashThreshold {
				return true
			}
			// For LibFuzzer specifically, immediate segfaults are harness crashes
			if strings.Contains(errLower, "libfuzzer") && duration < time.Second {
				return true
			}
		}
	}

	// Check for binary not found or permission errors
	if strings.Contains(errLower, "no such file") ||
		strings.Contains(errLower, "permission denied") ||
		strings.Contains(errLower, "cannot execute") {
		return true
	}

	return false
}
