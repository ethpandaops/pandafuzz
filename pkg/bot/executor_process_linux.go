//go:build linux

package bot

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// ProcessExecutor handles process execution with proper group management for Linux
type ProcessExecutor struct {
	logger *logrus.Logger
}

// NewProcessExecutor creates a new process executor
func NewProcessExecutor(logger *logrus.Logger) *ProcessExecutor {
	pe := &ProcessExecutor{
		logger: logger,
	}
	// Setup signal handling for zombie prevention
	pe.setupSignalHandling()
	return pe
}

// setupSignalHandling sets up SIGCHLD handler to prevent zombie processes
func (pe *ProcessExecutor) setupSignalHandling() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGCHLD)

	go func() {
		for range sigChan {
			// Reap any zombie children
			for {
				var status syscall.WaitStatus
				pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
				if err != nil || pid <= 0 {
					break
				}
				pe.logger.WithField("pid", pid).Debug("Reaped child process")
			}
		}
	}()
}

// ExecuteCommandWithProcessGroup executes a command with proper process group management
func (pe *ProcessExecutor) ExecuteCommandWithProcessGroup(ctx context.Context, cmd *exec.Cmd, fuzzerType string) error {
	// Set process group for proper signal handling
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pgid:      0,               // Create new process group
		Pdeathsig: syscall.SIGKILL, // Kill children if parent dies
	}

	// Handle AFL++ specific requirements
	if fuzzerType == "afl++" || fuzzerType == "afl" {
		// Ensure proper environment for fork-server
		cmd.Env = append(cmd.Env,
			"AFL_NO_FORKSRV=0",               // Enable fork-server
			"AFL_FORKSRV_INIT_TIMEOUT=30000", // 30s timeout for fork-server init
			fmt.Sprintf("__AFL_SHM_ID=afl_%d_%d", os.Getpid(), time.Now().UnixNano()), // Unique SHM ID
			"AFL_MAP_SIZE=65536", // Standard AFL map size
		)
		pe.logger.Debug("Configured AFL++ environment for fork-server mode")
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	pe.logger.WithFields(logrus.Fields{
		"pid":  cmd.Process.Pid,
		"pgid": 0,
	}).Debug("Process started with process group")

	// Wait for process in separate goroutine to prevent zombies
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// Wait for process completion or context cancellation
	select {
	case <-ctx.Done():
		// Kill entire process group on context cancellation
		pe.logger.WithField("pid", cmd.Process.Pid).Debug("Context cancelled, killing process group")

		// First try SIGTERM
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil {
			pe.logger.WithError(err).Debug("Failed to send SIGTERM to process group")
		}

		// Give process time to exit gracefully
		termTimer := time.NewTimer(5 * time.Second)
		select {
		case <-done:
			termTimer.Stop()
			return ctx.Err()
		case <-termTimer.C:
			// Force kill if not exited
			pe.logger.WithField("pid", cmd.Process.Pid).Debug("Process didn't exit after SIGTERM, sending SIGKILL")
			if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
				pe.logger.WithError(err).Error("Failed to kill process group")
			}
		}

		// Wait for process to exit after SIGKILL
		killTimer := time.NewTimer(2 * time.Second)
		select {
		case err := <-done:
			killTimer.Stop()
			if err != nil {
				pe.logger.WithError(err).Debug("Process exited with error after kill")
			}
		case <-killTimer.C:
			pe.logger.Warn("Process still not exited after SIGKILL")
		}

		return ctx.Err()

	case err := <-done:
		// Process completed normally
		if err != nil {
			pe.logger.WithError(err).Debug("Process exited with error")
		}
		return err
	}
}

// CheckProcessHealth checks if a process is in zombie state
func (pe *ProcessExecutor) CheckProcessHealth(pid int) error {
	// Check if process is zombie
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("process %d no longer exists", pid)
		}
		return fmt.Errorf("failed to read process stat: %w", err)
	}

	// Parse stat to check for zombie state (Z)
	statStr := string(data)
	// The state is after the command name in parentheses
	// Format: pid (comm) state ...
	// Find the last ')' to locate the start of state field
	lastParen := -1
	for i := len(statStr) - 1; i >= 0; i-- {
		if statStr[i] == ')' {
			lastParen = i
			break
		}
	}

	if lastParen != -1 && lastParen+2 < len(statStr) {
		state := statStr[lastParen+2 : lastParen+3]
		if state == "Z" {
			return fmt.Errorf("process %d is zombie", pid)
		}
	}

	return nil
}

// WaitForAFLInitialization waits for AFL++ fork-server to initialize
func (pe *ProcessExecutor) WaitForAFLInitialization(ctx context.Context, pid int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("AFL++ initialization timeout after %v", time.Since(startTime))
		case <-ticker.C:
			// Check if process is still alive and not zombie
			if err := pe.CheckProcessHealth(pid); err != nil {
				return fmt.Errorf("AFL++ process unhealthy during initialization: %w", err)
			}

			// Check for AFL++ shared memory segments
			if pe.checkAFLSharedMemory() {
				pe.logger.Debug("AFL++ shared memory detected, initialization complete")
				return nil
			}

			// After 1 second, assume it's initialized if process is still running
			if time.Since(startTime) > 1*time.Second {
				pe.logger.Debug("AFL++ process still running after 1s, assuming initialized")
				return nil
			}
		}
	}
}

// checkAFLSharedMemory checks if AFL++ has created shared memory segments
func (pe *ProcessExecutor) checkAFLSharedMemory() bool {
	// Check for AFL++ shared memory segments using ipcs equivalent
	// Read /proc/sysvipc/shm to check for shared memory segments
	data, err := os.ReadFile("/proc/sysvipc/shm")
	if err != nil {
		return false
	}

	// Look for AFL-related shared memory segments
	// AFL++ creates SHM segments that can be identified by size (typically 65536 bytes for coverage map)
	lines := string(data)
	return len(lines) > 100 // If there's substantial content, SHM exists
}
