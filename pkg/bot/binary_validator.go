package bot

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// BinaryValidator provides utilities to validate fuzzer binaries before execution
type BinaryValidator struct {
	logger *logrus.Logger
}

// NewBinaryValidator creates a new binary validator
func NewBinaryValidator(logger *logrus.Logger) *BinaryValidator {
	return &BinaryValidator{logger: logger}
}

// ValidateFuzzerBinary checks if a fuzzer binary is valid and properly instrumented
func (v *BinaryValidator) ValidateFuzzerBinary(binaryPath string, fuzzerType string) error {
	// Check if file exists
	info, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("binary not found: %w", err)
	}

	// Check if it's a regular file
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", binaryPath)
	}

	// Check if it's executable
	if info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("binary is not executable")
	}

	// Check binary size
	if info.Size() == 0 {
		return fmt.Errorf("binary is empty (0 bytes)")
	}

	// Run fuzzer-specific validation
	switch fuzzerType {
	case "libfuzzer":
		return v.validateLibFuzzerBinary(binaryPath)
	case "afl++", "aflplusplus", "afl":
		return v.validateAFLBinary(binaryPath)
	case "honggfuzz":
		return v.validateHonggfuzzBinary(binaryPath)
	default:
		// For unknown fuzzers, just do a basic execution test
		return v.basicExecutionTest(binaryPath)
	}
}

// validateLibFuzzerBinary validates a LibFuzzer binary
func (v *BinaryValidator) validateLibFuzzerBinary(binaryPath string) error {
	// Try to validate as a real LibFuzzer binary first
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "-help=1")
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Check for LibFuzzer-specific output FIRST, even if process crashed
	// LibFuzzer+ASAN binaries may crash when output is piped but still produce valid output
	isLibFuzzerOutput := strings.Contains(outputStr, "libFuzzer") ||
		strings.Contains(outputStr, "LLVMFuzzerTestOneInput") ||
		strings.Contains(outputStr, "-max_total_time") ||
		strings.Contains(outputStr, "Usage:") && strings.Contains(outputStr, "fuzzing")

	if isLibFuzzerOutput {
		v.logger.WithField("binary", binaryPath).Info("LibFuzzer binary validation passed (detected LibFuzzer output)")
		return nil
	}

	// Check if binary has LLVMFuzzerTestOneInput symbol (heuristic for valid LibFuzzer binary)
	fileContent, readErr := os.ReadFile(binaryPath)
	if readErr == nil {
		contentStr := string(fileContent)
		if strings.Contains(contentStr, "LLVMFuzzerTestOneInput") {
			v.logger.WithField("binary", binaryPath).Info("LibFuzzer binary validation passed (found LLVMFuzzerTestOneInput symbol)")
			return nil
		}
	}

	if err != nil {
		// Check if it crashed immediately without any LibFuzzer output
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == -1 {
				// Process was killed by signal - for LibFuzzer, don't try stdin test
				// Instead, check for the symbol directly
				v.logger.WithFields(logrus.Fields{
					"binary": binaryPath,
					"signal": exitErr.ExitCode(),
					"output": outputStr,
				}).Debug("Binary crashed with signal during -help=1")

				// If no LibFuzzer symbols found, it's not a valid LibFuzzer binary
				return fmt.Errorf("binary crashed with -help=1 and has no LibFuzzer symbols - not a valid LibFuzzer binary")
			}
		}
		v.logger.WithFields(logrus.Fields{
			"binary": binaryPath,
			"output": outputStr,
			"error":  err,
		}).Debug("Binary doesn't respond to -help=1 as LibFuzzer")
		return fmt.Errorf("not a valid LibFuzzer binary: %w", err)
	}

	v.logger.WithField("binary", binaryPath).Debug("No LibFuzzer signatures found")
	return fmt.Errorf("binary does not appear to be a LibFuzzer instrumented binary")
}

// validateStandaloneLibFuzzerBinary validates a standalone LibFuzzer-compatible binary
// that reads from stdin (used for testing without real LibFuzzer instrumentation)
func (v *BinaryValidator) validateStandaloneLibFuzzerBinary(binaryPath string) error {
	v.logger.WithField("binary", binaryPath).Debug("Validating as standalone LibFuzzer-compatible binary")

	// Test with minimal input to ensure it doesn't crash immediately
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	testInput := []byte("test")
	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Stdin = bytes.NewReader(testInput)

	_, err := cmd.CombinedOutput()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == -1 {
				// Crashed with signal
				return fmt.Errorf("binary crashed immediately with test input - not a valid test binary")
			}
			// Non-zero exit is okay for test binaries
			v.logger.WithFields(logrus.Fields{
				"binary":    binaryPath,
				"exit_code": exitErr.ExitCode(),
			}).Debug("Standalone binary exited with non-zero (acceptable for test)")
		} else if ctx.Err() == context.DeadlineExceeded {
			// Timeout is actually good - means it's waiting for input
			v.logger.WithField("binary", binaryPath).Debug("Binary is waiting for input (good)")
		}
	}

	// Check if binary has LLVMFuzzerTestOneInput symbol (heuristic)
	fileContent, err := os.ReadFile(binaryPath)
	if err == nil {
		contentStr := string(fileContent)
		if strings.Contains(contentStr, "LLVMFuzzerTestOneInput") {
			v.logger.WithField("binary", binaryPath).Info("Found LLVMFuzzerTestOneInput symbol - valid test binary")
		} else {
			v.logger.WithField("binary", binaryPath).Info("No LLVMFuzzerTestOneInput symbol found - assuming valid test binary")
		}
	}

	v.logger.WithField("binary", binaryPath).Info("Standalone LibFuzzer-compatible binary validation passed")
	return nil
}

// validateAFLBinary validates an AFL++ instrumented binary
func (v *BinaryValidator) validateAFLBinary(binaryPath string) error {
	// For AFL++, we can check if the binary has AFL++ instrumentation
	// by running it with a simple input and checking for AFL++ specific behavior

	// First, try to run with no input to see if it exits cleanly
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Create a minimal test input
	testInput := []byte("test")

	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Stdin = bytes.NewReader(testInput)

	_, err := cmd.CombinedOutput()

	// AFL++ instrumented binaries usually work with stdin input
	// They shouldn't segfault immediately
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == -1 {
				return fmt.Errorf("binary crashed immediately - may not be properly instrumented for AFL++")
			}
		}
		// Non-zero exit is okay for AFL++ binaries
		v.logger.WithFields(logrus.Fields{
			"binary": binaryPath,
			"error":  err,
		}).Debug("AFL++ binary exited with non-zero (expected)")
	}

	// Check if we can read the binary to look for AFL++ signatures
	// This is a simple heuristic check
	fileContent, err := os.ReadFile(binaryPath)
	if err == nil && len(fileContent) > 0 {
		// Look for common AFL++ instrumentation signatures
		contentStr := string(fileContent)
		if strings.Contains(contentStr, "__afl_") ||
			strings.Contains(contentStr, "AFL") ||
			strings.Contains(contentStr, "american fuzzy lop") {
			v.logger.WithField("binary", binaryPath).Info("Found AFL++ instrumentation signatures")
		}
	}

	v.logger.WithField("binary", binaryPath).Info("AFL++ binary validation passed")
	return nil
}

// validateHonggfuzzBinary validates a Honggfuzz instrumented binary
func (v *BinaryValidator) validateHonggfuzzBinary(binaryPath string) error {
	// Similar to AFL++, test with minimal input
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	testInput := []byte("test")

	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Stdin = bytes.NewReader(testInput)

	_, err := cmd.CombinedOutput()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == -1 {
				return fmt.Errorf("binary crashed immediately - may not be properly instrumented for Honggfuzz")
			}
		}
	}

	v.logger.WithField("binary", binaryPath).Info("Honggfuzz binary validation passed")
	return nil
}

// basicExecutionTest performs a basic execution test
func (v *BinaryValidator) basicExecutionTest(binaryPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Stdin = strings.NewReader("test")

	_, err := cmd.CombinedOutput()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == -1 {
				return fmt.Errorf("binary crashed immediately (signal)")
			}
		}
		// Non-zero exit is acceptable
		v.logger.WithField("binary", binaryPath).Debug("Binary executed with non-zero exit (may be normal)")
	}

	return nil
}
