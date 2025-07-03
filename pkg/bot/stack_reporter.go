package bot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// StackReporter handles stack trace parsing and reporting
type StackReporter struct {
	logger logrus.FieldLogger
}

// NewStackReporter creates a new stack reporter
func NewStackReporter(logger logrus.FieldLogger) *StackReporter {
	return &StackReporter{
		logger: logger.WithField("component", "stack_reporter"),
	}
}

// ParseStackTrace parses a raw stack trace into structured format
func (sr *StackReporter) ParseStackTrace(output string) (*common.StackTrace, error) {
	if output == "" {
		return nil, fmt.Errorf("empty stack trace")
	}

	frames := sr.ExtractFrames(output)
	if len(frames) == 0 {
		// Try to extract from full output
		frames = sr.extractFramesFromOutput(output)
		if len(frames) == 0 {
			return nil, fmt.Errorf("no stack frames found")
		}
	}

	// Compute hashes
	topNHash := sr.computeStackHash(frames, 5) // Top 5 frames
	fullHash := sr.computeStackHash(frames, len(frames))

	return &common.StackTrace{
		Frames:   frames,
		TopNHash: topNHash,
		FullHash: fullHash,
		RawTrace: output,
	}, nil
}

// ExtractFrames extracts stack frames from various formats
func (sr *StackReporter) ExtractFrames(trace string) []common.StackFrame {
	var frames []common.StackFrame

	// Try different stack trace formats
	// Format 1: AddressSanitizer style
	if asanFrames := sr.extractASANFrames(trace); len(asanFrames) > 0 {
		return asanFrames
	}

	// Format 2: GDB/LLDB style
	if gdbFrames := sr.extractGDBFrames(trace); len(gdbFrames) > 0 {
		return gdbFrames
	}

	// Format 3: LibFuzzer style
	if libfuzzerFrames := sr.extractLibFuzzerFrames(trace); len(libfuzzerFrames) > 0 {
		return libfuzzerFrames
	}

	// Format 4: Generic format
	if genericFrames := sr.extractGenericFrames(trace); len(genericFrames) > 0 {
		return genericFrames
	}

	return frames
}

// extractASANFrames extracts frames from AddressSanitizer output
func (sr *StackReporter) extractASANFrames(trace string) []common.StackFrame {
	var frames []common.StackFrame

	// ASAN format: #0 0x7ffff... in function_name file.c:123:45
	asanPattern := regexp.MustCompile(`#(\d+)\s+0x[0-9a-fA-F]+\s+in\s+([^\s]+)\s+([^:]+):(\d+)(?::(\d+))?`)
	matches := asanPattern.FindAllStringSubmatch(trace, -1)

	for _, match := range matches {
		frameNum, _ := strconv.Atoi(match[1])
		line, _ := strconv.Atoi(match[4])

		frame := common.StackFrame{
			Function: match[2],
			File:     match[3],
			Line:     line,
		}

		// Try to parse offset if available
		if strings.Contains(match[0], "+0x") {
			offsetPattern := regexp.MustCompile(`\+0x([0-9a-fA-F]+)`)
			if offsetMatch := offsetPattern.FindStringSubmatch(match[0]); len(offsetMatch) > 1 {
				if offset, err := strconv.ParseUint(offsetMatch[1], 16, 64); err == nil {
					frame.Offset = offset
				}
			}
		}

		frames = append(frames, frame)
		sr.logger.WithFields(logrus.Fields{
			"frame":    frameNum,
			"function": frame.Function,
			"file":     frame.File,
			"line":     frame.Line,
		}).Debug("Extracted ASAN frame")
	}

	return frames
}

// extractGDBFrames extracts frames from GDB/LLDB output
func (sr *StackReporter) extractGDBFrames(trace string) []common.StackFrame {
	var frames []common.StackFrame

	// GDB format: #0  0x00000000 in function_name () at file.c:123
	gdbPattern := regexp.MustCompile(`#(\d+)\s+0x[0-9a-fA-F]+\s+in\s+([^\s(]+)[^a]+at\s+([^:]+):(\d+)`)
	matches := gdbPattern.FindAllStringSubmatch(trace, -1)

	for _, match := range matches {
		line, _ := strconv.Atoi(match[4])

		frame := common.StackFrame{
			Function: match[2],
			File:     match[3],
			Line:     line,
		}

		frames = append(frames, frame)
	}

	return frames
}

// extractLibFuzzerFrames extracts frames from LibFuzzer output
func (sr *StackReporter) extractLibFuzzerFrames(trace string) []common.StackFrame {
	var frames []common.StackFrame

	// LibFuzzer format can vary, but often includes:
	// ==12345==ERROR: AddressSanitizer: heap-buffer-overflow
	// #0 0x... in function_name
	// #1 0x... in another_function

	// First check if this is LibFuzzer output
	if !strings.Contains(trace, "ERROR:") && !strings.Contains(trace, "libFuzzer") {
		return frames
	}

	// Extract frames using simple pattern
	lines := strings.Split(trace, "\n")
	framePattern := regexp.MustCompile(`#(\d+)\s+0x[0-9a-fA-F]+\s+in\s+([^\s]+)`)

	for _, line := range lines {
		if match := framePattern.FindStringSubmatch(line); len(match) > 2 {
			frame := common.StackFrame{
				Function: match[2],
			}

			// Try to extract file and line from the same line or next line
			filePattern := regexp.MustCompile(`([^:]+):(\d+)`)
			if fileMatch := filePattern.FindStringSubmatch(line); len(fileMatch) > 2 {
				frame.File = fileMatch[1]
				frame.Line, _ = strconv.Atoi(fileMatch[2])
			}

			frames = append(frames, frame)
		}
	}

	return frames
}

// extractGenericFrames extracts frames using generic patterns
func (sr *StackReporter) extractGenericFrames(trace string) []common.StackFrame {
	var frames []common.StackFrame

	lines := strings.Split(trace, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip non-frame lines
		if strings.HasPrefix(line, "==") || strings.HasPrefix(line, "SUMMARY:") {
			continue
		}

		// Look for function names with various patterns
		var frame common.StackFrame

		// Pattern 1: function_name (file.c:123)
		pattern1 := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\s*\(([^:)]+):(\d+)\)`)
		if match := pattern1.FindStringSubmatch(line); len(match) > 3 {
			frame.Function = match[1]
			frame.File = match[2]
			frame.Line, _ = strconv.Atoi(match[3])
			frames = append(frames, frame)
			continue
		}

		// Pattern 2: at function_name (file.c:123)
		pattern2 := regexp.MustCompile(`at\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(([^:)]+):(\d+)\)`)
		if match := pattern2.FindStringSubmatch(line); len(match) > 3 {
			frame.Function = match[1]
			frame.File = match[2]
			frame.Line, _ = strconv.Atoi(match[3])
			frames = append(frames, frame)
			continue
		}

		// Pattern 3: Just function names after "in"
		if strings.Contains(line, " in ") {
			parts := strings.Split(line, " in ")
			if len(parts) > 1 {
				funcPart := strings.TrimSpace(parts[1])
				// Extract just the function name
				funcName := strings.Fields(funcPart)[0]
				if isValidFunctionName(funcName) {
					frame.Function = funcName
					frames = append(frames, frame)
				}
			}
		}
	}

	return frames
}

// extractFramesFromOutput extracts frames from full fuzzer output
func (sr *StackReporter) extractFramesFromOutput(output string) []common.StackFrame {
	var frames []common.StackFrame

	// Look for crash indicators
	crashIndicators := []string{
		"ERROR:",
		"SEGV",
		"heap-buffer-overflow",
		"stack-buffer-overflow",
		"use-after-free",
		"double-free",
		"==ABORTING",
	}

	hasCrash := false
	for _, indicator := range crashIndicators {
		if strings.Contains(output, indicator) {
			hasCrash = true
			break
		}
	}

	if !hasCrash {
		return frames
	}

	// Try to extract any function names mentioned after the crash
	lines := strings.Split(output, "\n")
	inStackTrace := false

	for _, line := range lines {
		// Start looking for stack trace after crash indicator
		if !inStackTrace {
			for _, indicator := range crashIndicators {
				if strings.Contains(line, indicator) {
					inStackTrace = true
					break
				}
			}
			continue
		}

		// Extract frames from subsequent lines
		if strings.Contains(line, "#") && strings.Contains(line, "0x") {
			// This looks like a stack frame
			frames = sr.ExtractFrames(line)
			if len(frames) > 0 {
				return frames
			}
		}
	}

	return frames
}

// EnhanceCrashResult enhances a crash result with parsed stack trace
func (sr *StackReporter) EnhanceCrashResult(crash *common.CrashResult, trace *common.StackTrace) {
	if trace == nil || len(trace.Frames) == 0 {
		return
	}

	// Set the structured stack trace
	crash.StackTrace = trace.RawTrace

	// Try to determine crash type from stack trace
	if crash.Type == "" || crash.Type == "generic" {
		crash.Type = sr.inferCrashType(trace)
	}

	// Extract the most relevant frame for display
	if len(trace.Frames) > 0 {
		topFrame := trace.Frames[0]
		// Add top frame info to crash metadata
		if crash.Output == "" {
			crash.Output = fmt.Sprintf("Crash in %s at %s:%d",
				topFrame.Function, topFrame.File, topFrame.Line)
		}
	}
}

// computeStackHash computes a hash of the top N stack frames
func (sr *StackReporter) computeStackHash(frames []common.StackFrame, n int) string {
	if n > len(frames) {
		n = len(frames)
	}

	// Create a normalized representation
	var parts []string
	for i := 0; i < n; i++ {
		frame := frames[i]
		// Include function name and file, but not line number
		// This allows grouping similar crashes even if line numbers change slightly
		part := fmt.Sprintf("%s@%s", frame.Function, frame.File)
		parts = append(parts, part)
	}

	// Compute hash
	data := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// inferCrashType tries to determine crash type from stack trace
func (sr *StackReporter) inferCrashType(trace *common.StackTrace) string {
	// Check function names for clues
	for _, frame := range trace.Frames {
		funcLower := strings.ToLower(frame.Function)

		if strings.Contains(funcLower, "malloc") || strings.Contains(funcLower, "free") {
			return "heap_corruption"
		}
		if strings.Contains(funcLower, "memcpy") || strings.Contains(funcLower, "strcpy") {
			return "buffer_overflow"
		}
		if strings.Contains(funcLower, "assert") {
			return "assertion"
		}
	}

	// Check raw trace for keywords
	traceLower := strings.ToLower(trace.RawTrace)
	if strings.Contains(traceLower, "heap-buffer-overflow") {
		return "heap_overflow"
	}
	if strings.Contains(traceLower, "stack-buffer-overflow") {
		return "stack_overflow"
	}
	if strings.Contains(traceLower, "use-after-free") {
		return "use_after_free"
	}
	if strings.Contains(traceLower, "double-free") {
		return "double_free"
	}
	if strings.Contains(traceLower, "null pointer") || strings.Contains(traceLower, "null deref") {
		return "null_deref"
	}
	if strings.Contains(traceLower, "segmentation fault") || strings.Contains(traceLower, "sigsegv") {
		return "segfault"
	}

	return "generic"
}

// isValidFunctionName checks if a string looks like a valid function name
func isValidFunctionName(name string) bool {
	if name == "" || len(name) > 256 {
		return false
	}

	// Should start with letter or underscore
	if !((name[0] >= 'a' && name[0] <= 'z') ||
		(name[0] >= 'A' && name[0] <= 'Z') ||
		name[0] == '_') {
		return false
	}

	// Check for common non-function patterns
	invalidPatterns := []string{
		"0x", "==", "ERROR", "SUMMARY", "ABORTING",
		"pc", "bp", "sp", "??", "<unknown>",
	}

	for _, pattern := range invalidPatterns {
		if strings.Contains(name, pattern) {
			return false
		}
	}

	return true
}
