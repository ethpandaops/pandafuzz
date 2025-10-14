package bot

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
	"testing"
)

func TestClassifyJobEnd(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		timedOut      bool
		stopInitiated bool
		crashesFound  int
		wantExpected  bool
		wantReason    JobEndReason
	}{
		{
			name:          "Clean exit with no crashes",
			err:           nil,
			timedOut:      false,
			stopInitiated: false,
			crashesFound:  0,
			wantExpected:  true,
			wantReason:    EndReasonCompleted,
		},
		{
			name:          "Clean exit with crashes",
			err:           nil,
			timedOut:      false,
			stopInitiated: false,
			crashesFound:  5,
			wantExpected:  true,
			wantReason:    EndReasonCrashFound,
		},
		{
			name:          "Timeout with no crashes",
			err:           context.DeadlineExceeded,
			timedOut:      true,
			stopInitiated: false,
			crashesFound:  0,
			wantExpected:  true,
			wantReason:    EndReasonTimeout,
		},
		{
			name:          "Timeout with crashes",
			err:           context.DeadlineExceeded,
			timedOut:      true,
			stopInitiated: false,
			crashesFound:  10,
			wantExpected:  true,
			wantReason:    EndReasonTimeout,
		},
		{
			name:          "Stopped by user",
			err:           errors.New("process killed"),
			timedOut:      false,
			stopInitiated: true,
			crashesFound:  0,
			wantExpected:  true,
			wantReason:    EndReasonStopped,
		},
		{
			name:          "Stopped by user with crashes",
			err:           errors.New("process killed"),
			timedOut:      false,
			stopInitiated: true,
			crashesFound:  3,
			wantExpected:  true,
			wantReason:    EndReasonStopped,
		},
		{
			name:          "Unknown error without crashes",
			err:           errors.New("some error"),
			timedOut:      false,
			stopInitiated: false,
			crashesFound:  0,
			wantExpected:  false,
			wantReason:    EndReasonError,
		},
		{
			name:          "Unknown error with crashes (still success)",
			err:           errors.New("some error"),
			timedOut:      false,
			stopInitiated: false,
			crashesFound:  5,
			wantExpected:  true,
			wantReason:    EndReasonCrashFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyJobEnd(tt.err, tt.timedOut, tt.stopInitiated, tt.crashesFound)

			if result.Expected != tt.wantExpected {
				t.Errorf("Expected = %v, want %v", result.Expected, tt.wantExpected)
			}

			if result.Reason != tt.wantReason {
				t.Errorf("Reason = %v, want %v", result.Reason, tt.wantReason)
			}

			if result.CrashesFound != tt.crashesFound {
				t.Errorf("CrashesFound = %v, want %v", result.CrashesFound, tt.crashesFound)
			}
		})
	}
}

func TestClassifyJobEnd_WithExitError(t *testing.T) {
	// Create a mock ExitError with SIGTERM signal
	// This simulates what happens when we stop AFL++ with a signal
	cmd := exec.Command("false")
	_ = cmd.Run() // This will create an ExitError

	// Get the error
	err := &exec.ExitError{}
	err.ProcessState = cmd.ProcessState

	// Test with signal termination but with crashes found
	result := ClassifyJobEnd(err, false, false, 100)

	// With crashes found, even an exit error should be treated as success
	if !result.Expected {
		t.Errorf("Expected job with crashes to be successful even with exit error")
	}

	if result.Reason != EndReasonCrashFound {
		t.Errorf("Expected reason to be EndReasonCrashFound, got %v", result.Reason)
	}
}

func TestJobEndState_GetMessage(t *testing.T) {
	tests := []struct {
		state   JobEndState
		wantMsg string
	}{
		{
			state: JobEndState{
				Expected:     true,
				Reason:       EndReasonCompleted,
				CrashesFound: 0,
			},
			wantMsg: "Job completed successfully",
		},
		{
			state: JobEndState{
				Expected:     true,
				Reason:       EndReasonTimeout,
				CrashesFound: 5,
			},
			wantMsg: "Job completed (duration reached, found 5 crashes)",
		},
		{
			state: JobEndState{
				Expected:     true,
				Reason:       EndReasonCrashFound,
				CrashesFound: 10,
			},
			wantMsg: "Job completed successfully (found 10 crashes)",
		},
		{
			state: JobEndState{
				Expected: false,
				Reason:   EndReasonHarnessCrash,
				Signal:   syscall.SIGSEGV,
			},
			wantMsg: "Fuzzer binary crashed (signal 11) - check binary compatibility",
		},
	}

	for _, tt := range tests {
		t.Run(tt.wantMsg, func(t *testing.T) {
			got := tt.state.GetMessage()
			if got != tt.wantMsg {
				t.Errorf("GetMessage() = %v, want %v", got, tt.wantMsg)
			}
		})
	}
}

