package bot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestClassifyJobOutcome(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	classifier := NewJobStatusClassifier(logger)

	tests := []struct {
		name             string
		jobID            string
		err              error
		duration         time.Duration
		crashesFound     int
		expectedDuration time.Duration
		wantSuccess      bool
		wantReason       string
	}{
		{
			name:             "Clean exit with no crashes",
			jobID:            "job-1",
			err:              nil,
			duration:         10 * time.Minute,
			crashesFound:     0,
			expectedDuration: 30 * time.Minute,
			wantSuccess:      true,
			wantReason:       "clean_exit",
		},
		{
			name:             "Clean exit with crashes",
			jobID:            "job-2",
			err:              nil,
			duration:         10 * time.Minute,
			crashesFound:     5,
			expectedDuration: 30 * time.Minute,
			wantSuccess:      true,
			wantReason:       "crashes_found",
		},
		{
			name:             "Timeout with no crashes",
			jobID:            "job-3",
			err:              context.DeadlineExceeded,
			duration:         30 * time.Minute,
			crashesFound:     0,
			expectedDuration: 30 * time.Minute,
			wantSuccess:      true,
			wantReason:       "expected_termination",
		},
		{
			name:             "Timeout with crashes",
			jobID:            "job-4",
			err:              context.DeadlineExceeded,
			duration:         30 * time.Minute,
			crashesFound:     10,
			expectedDuration: 30 * time.Minute,
			wantSuccess:      true,
			wantReason:       "crashes_found",
		},
		{
			name:             "Signal killed is expected termination",
			jobID:            "job-5",
			err:              errors.New("signal: killed"),
			duration:         30 * time.Minute,
			crashesFound:     0,
			expectedDuration: 30 * time.Minute,
			wantSuccess:      true,
			wantReason:       "expected_termination",
		},
		{
			name:             "Signal terminated is expected termination",
			jobID:            "job-6",
			err:              errors.New("signal: terminated"),
			duration:         30 * time.Minute,
			crashesFound:     0,
			expectedDuration: 30 * time.Minute,
			wantSuccess:      true,
			wantReason:       "expected_termination",
		},
		{
			name:             "Unknown error without crashes",
			jobID:            "job-7",
			err:              errors.New("some unexpected error"),
			duration:         10 * time.Minute,
			crashesFound:     0,
			expectedDuration: 30 * time.Minute,
			wantSuccess:      false,
			wantReason:       "unknown_error",
		},
		{
			name:             "Unknown error with crashes is success",
			jobID:            "job-8",
			err:              errors.New("some unexpected error"),
			duration:         10 * time.Minute,
			crashesFound:     5,
			expectedDuration: 30 * time.Minute,
			wantSuccess:      true,
			wantReason:       "crashes_found",
		},
		{
			name:             "Segfault early is harness crash",
			jobID:            "job-9",
			err:              errors.New("signal: segmentation fault"),
			duration:         1 * time.Second,
			crashesFound:     0,
			expectedDuration: 30 * time.Minute,
			wantSuccess:      false,
			wantReason:       "harness_crash",
		},
		{
			name:             "Binary not found is harness crash",
			jobID:            "job-10",
			err:              errors.New("no such file or directory"),
			duration:         0,
			crashesFound:     0,
			expectedDuration: 30 * time.Minute,
			wantSuccess:      false,
			wantReason:       "harness_crash",
		},
		{
			name:             "Permission denied is harness crash",
			jobID:            "job-11",
			err:              errors.New("permission denied"),
			duration:         0,
			crashesFound:     0,
			expectedDuration: 30 * time.Minute,
			wantSuccess:      false,
			wantReason:       "harness_crash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.ClassifyJobOutcome(tt.jobID, tt.err, tt.duration, tt.crashesFound, tt.expectedDuration)

			if result.Success != tt.wantSuccess {
				t.Errorf("Success = %v, want %v", result.Success, tt.wantSuccess)
			}

			if result.Reason != tt.wantReason {
				t.Errorf("Reason = %v, want %v", result.Reason, tt.wantReason)
			}
		})
	}
}

func TestJobStatusClassifier_isExpectedTermination(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	classifier := NewJobStatusClassifier(logger)

	tests := []struct {
		name             string
		errLower         string
		duration         time.Duration
		expectedDuration time.Duration
		want             bool
	}{
		{
			name:             "signal killed",
			errLower:         "signal: killed",
			duration:         30 * time.Minute,
			expectedDuration: 30 * time.Minute,
			want:             true,
		},
		{
			name:             "signal terminated",
			errLower:         "signal: terminated",
			duration:         30 * time.Minute,
			expectedDuration: 30 * time.Minute,
			want:             true,
		},
		{
			name:             "sigterm",
			errLower:         "sigterm",
			duration:         30 * time.Minute,
			expectedDuration: 30 * time.Minute,
			want:             true,
		},
		{
			name:             "context deadline exceeded",
			errLower:         "context deadline exceeded",
			duration:         30 * time.Minute,
			expectedDuration: 30 * time.Minute,
			want:             true,
		},
		{
			name:             "random error",
			errLower:         "some random error",
			duration:         10 * time.Minute,
			expectedDuration: 30 * time.Minute,
			want:             false,
		},
		{
			name:             "afl++ completed",
			errLower:         "afl++ completed normally",
			duration:         30 * time.Minute,
			expectedDuration: 30 * time.Minute,
			want:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifier.isExpectedTermination(tt.errLower, tt.duration, tt.expectedDuration)
			if got != tt.want {
				t.Errorf("isExpectedTermination() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJobStatusClassifier_isHarnessCrash(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	classifier := NewJobStatusClassifier(logger)

	tests := []struct {
		name     string
		errLower string
		duration time.Duration
		want     bool
	}{
		{
			name:     "segfault early",
			errLower: "segmentation fault",
			duration: 1 * time.Second,
			want:     true,
		},
		{
			name:     "segfault late",
			errLower: "segmentation fault",
			duration: 10 * time.Minute,
			want:     false,
		},
		{
			name:     "sigsegv early",
			errLower: "sigsegv",
			duration: 2 * time.Second,
			want:     true,
		},
		{
			name:     "no such file",
			errLower: "no such file",
			duration: 0,
			want:     true,
		},
		{
			name:     "permission denied",
			errLower: "permission denied",
			duration: 0,
			want:     true,
		},
		{
			name:     "cannot execute",
			errLower: "cannot execute binary",
			duration: 0,
			want:     true,
		},
		{
			name:     "core dumped early",
			errLower: "core dumped",
			duration: 3 * time.Second,
			want:     true,
		},
		{
			name:     "random error",
			errLower: "random error",
			duration: 0,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifier.isHarnessCrash(tt.errLower, tt.duration)
			if got != tt.want {
				t.Errorf("isHarnessCrash() = %v, want %v", got, tt.want)
			}
		})
	}
}
