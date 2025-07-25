package analyzer

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
)

// Service defines the interface for crash analysis operations
type Service interface {
	Start(ctx context.Context) error
	Stop() error
	AnalyzeCrash(ctx context.Context, rawOutput string, targetInfo types.TargetInfo) (*types.Crash, error)
	ClassifyCrash(ctx context.Context, crash *types.Crash) error
	ExtractSignature(ctx context.Context, stackTrace string) (*types.CrashSignature, error)
	ParseStackTrace(ctx context.Context, rawOutput string, fuzzerType FuzzerType) (string, error)
}

// FuzzerType represents different fuzzer engines
type FuzzerType string

const (
	FuzzerTypeLibFuzzer   FuzzerType = "libfuzzer"
	FuzzerTypeAFLPlusPlus FuzzerType = "afl++"
	FuzzerTypeHonggfuzz   FuzzerType = "honggfuzz"
	FuzzerTypeUnknown     FuzzerType = "unknown"
)

// ServiceConfig holds configuration for the analyzer service
type ServiceConfig struct {
	EnableDeepAnalysis bool
	StackTraceMaxDepth int
	SignatureMinFrames int
	ParallelWorkers    int
}

// DefaultServiceConfig returns a default configuration
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		EnableDeepAnalysis: true,
		StackTraceMaxDepth: 50,
		SignatureMinFrames: 3,
		ParallelWorkers:    4,
	}
}

// service implements the Service interface
type service struct {
	log        logrus.FieldLogger
	config     ServiceConfig
	classifier *Classifier
	parser     *SimpleStackParser
	started    bool
	stopCh     chan struct{}
}

// NewService creates a new crash analyzer service
func NewService(log logrus.FieldLogger, config ServiceConfig) Service {
	return &service{
		log:        log.WithField("component", "crash-analyzer"),
		config:     config,
		classifier: NewClassifier(log),
		parser:     NewSimpleStackParser(log, config.StackTraceMaxDepth),
		stopCh:     make(chan struct{}),
	}
}

// Start initializes the analyzer service
func (s *service) Start(ctx context.Context) error {
	if s.started {
		return errors.New("analyzer service already started")
	}

	s.log.Info("Starting crash analyzer service")
	s.started = true
	return nil
}

// Stop shuts down the analyzer service
func (s *service) Stop() error {
	if !s.started {
		return errors.New("analyzer service not started")
	}

	s.log.Info("Stopping crash analyzer service")
	close(s.stopCh)
	s.started = false
	return nil
}

// AnalyzeCrash performs comprehensive crash analysis
func (s *service) AnalyzeCrash(ctx context.Context, rawOutput string, targetInfo types.TargetInfo) (*types.Crash, error) {
	if !s.started {
		return nil, errors.New("analyzer service not started")
	}

	if rawOutput == "" {
		return nil, errors.New("raw output cannot be empty")
	}

	// Detect fuzzer type from output
	fuzzerType := s.detectFuzzerType(rawOutput)
	s.log.WithField("fuzzer_type", fuzzerType).Debug("Detected fuzzer type")

	// Extract crash input
	input, err := s.extractCrashInput(rawOutput, fuzzerType)
	if err != nil {
		return nil, fmt.Errorf("failed to extract crash input: %w", err)
	}

	// Parse stack trace
	stackTrace, err := s.ParseStackTrace(ctx, rawOutput, fuzzerType)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stack trace: %w", err)
	}

	// Create crash object
	crash, err := types.NewCrash(input, stackTrace, targetInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to create crash: %w", err)
	}

	// Perform classification
	if err := s.ClassifyCrash(ctx, crash); err != nil {
		s.log.WithError(err).Warn("Failed to classify crash")
		// Continue even if classification fails
	}

	// Add metadata
	crash.SetMetadata("fuzzer_type", string(fuzzerType))
	crash.SetMetadata("analysis_version", "1.0")
	crash.SetMetadata("analyzed_at", time.Now().UTC().Format(time.RFC3339))

	// Extract additional information if deep analysis is enabled
	if s.config.EnableDeepAnalysis {
		s.performDeepAnalysis(ctx, crash, rawOutput)
	}

	s.log.WithFields(logrus.Fields{
		"crash_id":  crash.ID,
		"type":      crash.Type,
		"severity":  crash.Severity,
		"signature": crash.Signature.Hash[:16],
	}).Info("Crash analysis completed")

	return crash, nil
}

// ClassifyCrash classifies a crash and updates its type and severity
func (s *service) ClassifyCrash(ctx context.Context, crash *types.Crash) error {
	if crash == nil {
		return errors.New("crash cannot be nil")
	}

	// Use classifier to determine crash type and severity
	crashType, severity := s.classifier.Classify(crash.StackTrace)

	crash.Type = crashType
	if err := crash.UpdateSeverity(severity); err != nil {
		return fmt.Errorf("failed to update severity: %w", err)
	}

	// Add classification tags
	s.addClassificationTags(crash)

	return nil
}

// ExtractSignature creates a crash signature from a stack trace
func (s *service) ExtractSignature(ctx context.Context, stackTrace string) (*types.CrashSignature, error) {
	if stackTrace == "" {
		return nil, errors.New("stack trace cannot be empty")
	}

	signature, err := types.NewCrashSignature(stackTrace)
	if err != nil {
		return nil, fmt.Errorf("failed to create signature: %w", err)
	}

	// Validate signature has minimum required frames
	if len(signature.TopFrames) < s.config.SignatureMinFrames {
		s.log.WithField("frame_count", len(signature.TopFrames)).
			Warn("Signature has fewer frames than minimum required")
	}

	return signature, nil
}

// ParseStackTrace extracts and formats a stack trace from raw fuzzer output
func (s *service) ParseStackTrace(ctx context.Context, rawOutput string, fuzzerType FuzzerType) (string, error) {
	return s.parser.Parse(rawOutput, fuzzerType)
}

// detectFuzzerType attempts to identify the fuzzer from output patterns
func (s *service) detectFuzzerType(output string) FuzzerType {
	lowerOutput := strings.ToLower(output)

	// LibFuzzer patterns
	if strings.Contains(lowerOutput, "libfuzzer") ||
		strings.Contains(lowerOutput, "==error") ||
		strings.Contains(lowerOutput, "artifact_prefix") {
		return FuzzerTypeLibFuzzer
	}

	// AFL++ patterns
	if strings.Contains(lowerOutput, "afl++") ||
		strings.Contains(lowerOutput, "american fuzzy lop") ||
		strings.Contains(lowerOutput, "crash file written to") {
		return FuzzerTypeAFLPlusPlus
	}

	// Honggfuzz patterns
	if strings.Contains(lowerOutput, "honggfuzz") ||
		strings.Contains(lowerOutput, "**crash**") ||
		strings.Contains(lowerOutput, "signal: ") {
		return FuzzerTypeHonggfuzz
	}

	return FuzzerTypeUnknown
}

// extractCrashInput attempts to extract the crash-causing input from fuzzer output
func (s *service) extractCrashInput(output string, fuzzerType FuzzerType) ([]byte, error) {
	var input []byte

	switch fuzzerType {
	case FuzzerTypeLibFuzzer:
		input = s.extractLibFuzzerInput(output)
	case FuzzerTypeAFLPlusPlus:
		input = s.extractAFLInput(output)
	case FuzzerTypeHonggfuzz:
		input = s.extractHonggfuzzInput(output)
	default:
		// Try generic extraction
		input = s.extractGenericInput(output)
	}

	if len(input) == 0 {
		// If we couldn't extract input, use a placeholder
		input = []byte("<input-extraction-failed>")
		s.log.Warn("Failed to extract crash input, using placeholder")
	}

	return input, nil
}

// extractLibFuzzerInput extracts input from LibFuzzer output
func (s *service) extractLibFuzzerInput(output string) []byte {
	// LibFuzzer typically shows: "artifact_prefix='./'; Test unit written to ./crash-<hash>"
	re := regexp.MustCompile(`Test unit written to ([^\s]+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) > 1 {
		return []byte(matches[1])
	}

	// Try to find hex dump
	re = regexp.MustCompile(`0x[0-9a-fA-F]{2}(?:,\s*0x[0-9a-fA-F]{2})*`)
	if hexMatch := re.FindString(output); hexMatch != "" {
		return s.parseHexDump(hexMatch)
	}

	return nil
}

// extractAFLInput extracts input from AFL++ output
func (s *service) extractAFLInput(output string) []byte {
	// AFL++ shows: "Crash file written to id:000000,sig:11,src:000000..."
	re := regexp.MustCompile(`Crash file written to ([^\s]+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) > 1 {
		return []byte(matches[1])
	}
	return nil
}

// extractHonggfuzzInput extracts input from Honggfuzz output
func (s *service) extractHonggfuzzInput(output string) []byte {
	// Honggfuzz shows crash files in various formats
	re := regexp.MustCompile(`Crash: ([^\s]+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) > 1 {
		return []byte(matches[1])
	}
	return nil
}

// extractGenericInput attempts generic input extraction
func (s *service) extractGenericInput(output string) []byte {
	// Look for file paths that might be crash inputs
	re := regexp.MustCompile(`(?:crash|input|testcase)[-_]?[0-9a-fA-F]+`)
	if match := re.FindString(output); match != "" {
		return []byte(match)
	}
	return nil
}

// parseHexDump converts hex dump to bytes
func (s *service) parseHexDump(hexDump string) []byte {
	// Remove 0x prefixes and spaces
	cleaned := strings.ReplaceAll(hexDump, "0x", "")
	cleaned = strings.ReplaceAll(cleaned, ",", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")

	// Convert to bytes (simplified - in production would use hex.DecodeString)
	return []byte(cleaned)
}

// performDeepAnalysis performs additional analysis when enabled
func (s *service) performDeepAnalysis(ctx context.Context, crash *types.Crash, rawOutput string) {
	// Extract additional patterns
	if addr := s.extractFaultAddress(rawOutput); addr != "" {
		crash.SetMetadata("fault_address", addr)
	}

	if signal := s.extractSignalInfo(rawOutput); signal != "" {
		crash.SetMetadata("signal", signal)
	}

	// Look for security-relevant patterns
	if s.containsSecurityIndicators(rawOutput) {
		crash.AddTag("security-relevant")
		if crash.Severity == types.SeverityMedium || crash.Severity == types.SeverityLow {
			_ = crash.UpdateSeverity(types.SeverityHigh)
		}
	}

	// Check for exploitability indicators
	if s.isLikelyExploitable(crash) {
		crash.AddTag("potentially-exploitable")
	}
}

// extractFaultAddress extracts the faulting address from crash output
func (s *service) extractFaultAddress(output string) string {
	re := regexp.MustCompile(`(?:fault|crash|segfault) (?:at|address:?) (0x[0-9a-fA-F]+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// extractSignalInfo extracts signal information
func (s *service) extractSignalInfo(output string) string {
	re := regexp.MustCompile(`(?:signal|sig)[:=]\s*(\d+|SIG\w+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// containsSecurityIndicators checks for security-relevant patterns
func (s *service) containsSecurityIndicators(output string) bool {
	securityPatterns := []string{
		"stack canary",
		"fortify",
		"buffer overflow",
		"use-after-free",
		"double-free",
		"heap corruption",
		"stack corruption",
		"format string",
		"integer overflow",
		"null pointer dereference",
		"out-of-bounds",
	}

	lowerOutput := strings.ToLower(output)
	for _, pattern := range securityPatterns {
		if strings.Contains(lowerOutput, pattern) {
			return true
		}
	}
	return false
}

// isLikelyExploitable checks if a crash might be exploitable
func (s *service) isLikelyExploitable(crash *types.Crash) bool {
	// Check crash type
	switch crash.Type {
	case types.CrashTypeHeapOverflow, types.CrashTypeStackOverflow:
		return true
	case types.CrashTypeSegmentationFault:
		// Check if it's a write operation
		if faultAddr, ok := crash.GetMetadata("fault_address"); ok {
			// Simple heuristic: low addresses are less likely exploitable
			if strings.HasPrefix(faultAddr, "0x0000") {
				return false
			}
			return true
		}
	}
	return false
}

// addClassificationTags adds relevant tags based on crash classification
func (s *service) addClassificationTags(crash *types.Crash) {
	// Add fuzzer tag
	if fuzzerType, ok := crash.GetMetadata("fuzzer_type"); ok {
		crash.AddTag(fmt.Sprintf("fuzzer:%s", fuzzerType))
	}

	// Add severity tag
	crash.AddTag(fmt.Sprintf("severity:%s", crash.Severity))

	// Add type tag
	crash.AddTag(fmt.Sprintf("type:%s", crash.Type))

	// Add confidence tag based on signature
	if crash.Signature != nil {
		if crash.Signature.Confidence > 0.8 {
			crash.AddTag("high-confidence")
		} else if crash.Signature.Confidence < 0.3 {
			crash.AddTag("low-confidence")
		}
	}
}

// Classifier provides crash classification functionality
type Classifier struct {
	log logrus.FieldLogger
}

// NewClassifier creates a new crash classifier
func NewClassifier(log logrus.FieldLogger) *Classifier {
	return &Classifier{
		log: log.WithField("component", "classifier"),
	}
}

// Classify determines the crash type and severity from a stack trace
func (c *Classifier) Classify(stackTrace string) (types.CrashType, types.Severity) {
	crashType := c.classifyType(stackTrace)
	severity := c.classifySeverity(stackTrace, crashType)
	return crashType, severity
}

// classifyType determines the crash type from patterns in the stack trace
func (c *Classifier) classifyType(stackTrace string) types.CrashType {
	stackLower := strings.ToLower(stackTrace)

	// Check for specific crash type patterns
	typePatterns := map[types.CrashType][]string{
		types.CrashTypeSegmentationFault: {
			"segmentation fault", "sigsegv", "access violation",
			"null pointer dereference", "bad memory access",
		},
		types.CrashTypeHeapOverflow: {
			"heap-buffer-overflow", "heap overflow", "heap corruption",
			"use-after-free", "double free", "corrupted double-linked list",
		},
		types.CrashTypeStackOverflow: {
			"stack-buffer-overflow", "stack overflow", "stack exhausted",
			"stack smashing detected", "stack corruption",
		},
		types.CrashTypeAssertion: {
			"assertion failed", "assert(", "assertion `",
			"panic:", "fatal error:",
		},
		types.CrashTypeTimeout: {
			"timeout", "timed out", "deadline exceeded",
			"watchdog", "hang detected",
		},
		types.CrashTypeMemoryLeak: {
			"memory leak", "leak detected", "out of memory",
			"oom", "allocation failed",
		},
		types.CrashTypeUnhandledException: {
			"unhandled exception", "uncaught exception",
			"exception thrown", "runtime exception",
		},
	}

	for crashType, patterns := range typePatterns {
		for _, pattern := range patterns {
			if strings.Contains(stackLower, pattern) {
				return crashType
			}
		}
	}

	return types.CrashTypeOther
}

// classifySeverity determines the severity based on crash type and patterns
func (c *Classifier) classifySeverity(stackTrace string, crashType types.CrashType) types.Severity {
	stackLower := strings.ToLower(stackTrace)

	// Check for critical severity indicators
	criticalPatterns := []string{
		"arbitrary code execution", "remote code execution",
		"privilege escalation", "format string vulnerability",
	}

	for _, pattern := range criticalPatterns {
		if strings.Contains(stackLower, pattern) {
			return types.SeverityCritical
		}
	}

	// Type-based severity
	switch crashType {
	case types.CrashTypeHeapOverflow, types.CrashTypeStackOverflow:
		return types.SeverityHigh
	case types.CrashTypeSegmentationFault:
		if strings.Contains(stackLower, "null pointer") || strings.Contains(stackLower, "0x0") {
			return types.SeverityMedium
		}
		return types.SeverityHigh
	case types.CrashTypeAssertion, types.CrashTypeUnhandledException:
		return types.SeverityMedium
	case types.CrashTypeTimeout, types.CrashTypeMemoryLeak:
		return types.SeverityLow
	default:
		return types.SeverityUnknown
	}
}

// SimpleStackParser provides stack trace parsing functionality
type SimpleStackParser struct {
	log      logrus.FieldLogger
	maxDepth int
}

// NewSimpleStackParser creates a new stack parser
func NewSimpleStackParser(log logrus.FieldLogger, maxDepth int) *SimpleStackParser {
	return &SimpleStackParser{
		log:      log.WithField("component", "stack-parser"),
		maxDepth: maxDepth,
	}
}

// Parse extracts a clean stack trace from raw fuzzer output
func (p *SimpleStackParser) Parse(rawOutput string, fuzzerType FuzzerType) (string, error) {
	switch fuzzerType {
	case FuzzerTypeLibFuzzer:
		return p.parseLibFuzzerStack(rawOutput)
	case FuzzerTypeAFLPlusPlus:
		return p.parseAFLStack(rawOutput)
	case FuzzerTypeHonggfuzz:
		return p.parseHonggfuzzStack(rawOutput)
	default:
		return p.parseGenericStack(rawOutput)
	}
}

// parseLibFuzzerStack extracts stack trace from LibFuzzer output
func (p *SimpleStackParser) parseLibFuzzerStack(output string) (string, error) {
	// LibFuzzer stack traces typically start with #0 and are preceded by ==ERROR==
	lines := strings.Split(output, "\n")
	var stackLines []string
	inStack := false

	for _, line := range lines {
		// Start collecting when we see the first frame
		if strings.TrimSpace(line) != "" && strings.HasPrefix(strings.TrimSpace(line), "#0") {
			inStack = true
		}

		if inStack {
			// Stop when we reach an empty line or non-frame line
			if strings.TrimSpace(line) == "" || (!strings.Contains(line, "#") && !strings.Contains(line, "at ")) {
				break
			}
			stackLines = append(stackLines, line)

			// Limit depth
			if len(stackLines) >= p.maxDepth {
				break
			}
		}
	}

	if len(stackLines) == 0 {
		return output, nil // Return original if no stack found
	}

	return strings.Join(stackLines, "\n"), nil
}

// parseAFLStack extracts stack trace from AFL++ output
func (p *SimpleStackParser) parseAFLStack(output string) (string, error) {
	// AFL++ typically includes gdb-style backtraces
	return p.parseGDBStyleStack(output)
}

// parseHonggfuzzStack extracts stack trace from Honggfuzz output
func (p *SimpleStackParser) parseHonggfuzzStack(output string) (string, error) {
	// Honggfuzz may include various stack trace formats
	// Try to find stack-like patterns
	lines := strings.Split(output, "\n")
	var stackLines []string

	for _, line := range lines {
		// Look for frame indicators
		if strings.Contains(line, " at ") ||
			strings.Contains(line, " in ") ||
			regexp.MustCompile(`#\d+\s+0x[0-9a-fA-F]+`).MatchString(line) {
			stackLines = append(stackLines, line)

			if len(stackLines) >= p.maxDepth {
				break
			}
		}
	}

	if len(stackLines) == 0 {
		return output, nil
	}

	return strings.Join(stackLines, "\n"), nil
}

// parseGenericStack attempts to extract stack trace from generic output
func (p *SimpleStackParser) parseGenericStack(output string) (string, error) {
	// Try multiple patterns to extract stack traces
	return p.parseGDBStyleStack(output)
}

// parseGDBStyleStack extracts GDB-style stack traces
func (p *SimpleStackParser) parseGDBStyleStack(output string) (string, error) {
	lines := strings.Split(output, "\n")
	var stackLines []string
	framePattern := regexp.MustCompile(`#\d+\s+0x[0-9a-fA-F]+`)

	for _, line := range lines {
		if framePattern.MatchString(line) {
			stackLines = append(stackLines, line)

			if len(stackLines) >= p.maxDepth {
				break
			}
		}
	}

	if len(stackLines) == 0 {
		// Fallback: return the original output
		return output, nil
	}

	return strings.Join(stackLines, "\n"), nil
}
