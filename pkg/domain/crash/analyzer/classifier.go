package analyzer

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
	"github.com/sirupsen/logrus"
)

// CrashClassifier provides advanced crash classification capabilities
type CrashClassifier interface {
	// ClassifyCrash analyzes a crash and determines its type and severity
	ClassifyCrash(ctx context.Context, crash *types.Crash) error

	// ClassifyByStackTrace analyzes just the stack trace for classification
	ClassifyByStackTrace(ctx context.Context, stackTrace string) (types.CrashType, types.Severity, error)

	// FindSimilarCrashes finds crashes with similar patterns
	FindSimilarCrashes(ctx context.Context, crash *types.Crash, threshold float64) ([]*types.Crash, error)

	// GetKnownPatterns returns all registered crash patterns
	GetKnownPatterns() []CrashPattern

	// RegisterPattern adds a new crash pattern for matching
	RegisterPattern(pattern CrashPattern) error

	// Start initializes the classifier
	Start(ctx context.Context) error

	// Stop cleanly shuts down the classifier
	Stop() error
}

// CrashPattern represents a known crash pattern for matching
type CrashPattern struct {
	ID          string
	Name        string
	Type        types.CrashType
	Severity    types.Severity
	Pattern     *regexp.Regexp
	Keywords    []string
	Description string
	Priority    int // Higher priority patterns are checked first
}

// Config holds configuration for the crash classifier
type Config struct {
	EnablePatternLearning bool
	SimilarityThreshold   float64
	MaxPatternCache       int
}

// classifier implements the CrashClassifier interface
type classifier struct {
	log        logrus.FieldLogger
	repo       repository.CrashRepository
	patterns   []CrashPattern
	config     Config
	patternMap map[string]*CrashPattern
}

// NewCrashClassifier creates a new crash classifier instance
func NewCrashClassifier(log logrus.FieldLogger, repo repository.CrashRepository, config Config) CrashClassifier {
	return &classifier{
		log:        log.WithField("component", "crash_classifier"),
		repo:       repo,
		config:     config,
		patterns:   make([]CrashPattern, 0),
		patternMap: make(map[string]*CrashPattern),
	}
}

// Start initializes the classifier with default patterns
func (c *classifier) Start(ctx context.Context) error {
	c.log.Info("Starting crash classifier")

	// Register default crash patterns
	if err := c.registerDefaultPatterns(); err != nil {
		return fmt.Errorf("failed to register default patterns: %w", err)
	}

	c.log.WithField("pattern_count", len(c.patterns)).Info("Crash classifier started")
	return nil
}

// Stop cleanly shuts down the classifier
func (c *classifier) Stop() error {
	c.log.Info("Stopping crash classifier")
	return nil
}

// ClassifyCrash analyzes a crash and determines its type and severity
func (c *classifier) ClassifyCrash(ctx context.Context, crash *types.Crash) error {
	if crash == nil {
		return errors.New("crash cannot be nil")
	}

	crashType, severity, err := c.ClassifyByStackTrace(ctx, crash.StackTrace)
	if err != nil {
		return fmt.Errorf("failed to classify crash: %w", err)
	}

	crash.Type = crashType
	if err := crash.UpdateSeverity(severity); err != nil {
		return fmt.Errorf("failed to update severity: %w", err)
	}

	// Add classification metadata
	crash.SetMetadata("classification_method", "pattern_matching")
	crash.SetMetadata("pattern_count", fmt.Sprintf("%d", len(c.patterns)))

	// Try to find matching known pattern
	if pattern := c.findMatchingPattern(crash.StackTrace); pattern != nil {
		crash.SetMetadata("matched_pattern", pattern.Name)
		crash.AddTag(fmt.Sprintf("pattern:%s", pattern.ID))
	}

	return nil
}

// ClassifyByStackTrace analyzes just the stack trace for classification
func (c *classifier) ClassifyByStackTrace(ctx context.Context, stackTrace string) (types.CrashType, types.Severity, error) {
	if stackTrace == "" {
		return types.CrashTypeOther, types.SeverityUnknown, errors.New("stack trace cannot be empty")
	}

	// Check against known patterns
	if pattern := c.findMatchingPattern(stackTrace); pattern != nil {
		c.log.WithFields(logrus.Fields{
			"pattern":  pattern.Name,
			"type":     pattern.Type,
			"severity": pattern.Severity,
		}).Debug("Matched known pattern")
		return pattern.Type, pattern.Severity, nil
	}

	// Fallback to enhanced heuristic classification
	crashType := c.classifyTypeEnhanced(stackTrace)
	severity := c.classifySeverityEnhanced(stackTrace, crashType)

	return crashType, severity, nil
}

// findMatchingPattern finds the first matching pattern for a stack trace
func (c *classifier) findMatchingPattern(stackTrace string) *CrashPattern {
	stackLower := strings.ToLower(stackTrace)

	// Check patterns in priority order
	for _, pattern := range c.patterns {
		// Check regex pattern
		if pattern.Pattern != nil && pattern.Pattern.MatchString(stackTrace) {
			return &pattern
		}

		// Check keywords
		allKeywordsMatch := true
		for _, keyword := range pattern.Keywords {
			if !strings.Contains(stackLower, strings.ToLower(keyword)) {
				allKeywordsMatch = false
				break
			}
		}
		if allKeywordsMatch && len(pattern.Keywords) > 0 {
			return &pattern
		}
	}

	return nil
}

// classifyTypeEnhanced performs enhanced crash type classification
func (c *classifier) classifyTypeEnhanced(stackTrace string) types.CrashType {
	stackLower := strings.ToLower(stackTrace)

	// Extended patterns for crash type detection
	typePatterns := map[types.CrashType][]string{
		types.CrashTypeSegmentationFault: {
			"segmentation fault", "sigsegv", "access violation",
			"null pointer dereference", "bad memory access",
			"signal 11", "segv", "core dumped",
		},
		types.CrashTypeHeapOverflow: {
			"heap overflow", "heap-buffer-overflow", "heap corruption",
			"malloc(): corrupted", "free(): invalid pointer",
			"heap-use-after-free", "double free", "corrupted double-linked list",
		},
		types.CrashTypeStackOverflow: {
			"stack overflow", "stack-buffer-overflow", "stack exhausted",
			"recursion limit", "call stack size exceeded",
			"stack smashing detected", "stack corruption",
		},
		types.CrashTypeAssertion: {
			"assertion failed", "assert(", "assertion `",
			"debug assertion", "runtime assertion", "invariant violation",
			"panic:", "fatal error:", "runtime error:",
		},
		types.CrashTypeTimeout: {
			"timeout", "timed out", "deadline exceeded",
			"operation cancelled", "context cancelled",
			"watchdog", "hang detected",
		},
		types.CrashTypeMemoryLeak: {
			"memory leak", "leak detected", "memory exhausted",
			"out of memory", "oom", "allocation failed",
			"cannot allocate memory",
		},
		types.CrashTypeUnhandledException: {
			"unhandled exception", "uncaught exception",
			"exception thrown", "std::exception",
			"runtime exception", "fatal exception",
		},
	}

	// Check each pattern set
	for crashType, patterns := range typePatterns {
		for _, pattern := range patterns {
			if strings.Contains(stackLower, pattern) {
				return crashType
			}
		}
	}

	// Check for specific error signatures
	if c.isUseAfterFree(stackTrace) {
		return types.CrashTypeHeapOverflow
	}

	if c.isIntegerOverflow(stackTrace) {
		return types.CrashTypeOther
	}

	return types.CrashTypeOther
}

// classifySeverityEnhanced performs enhanced severity classification
func (c *classifier) classifySeverityEnhanced(stackTrace string, crashType types.CrashType) types.Severity {
	stackLower := strings.ToLower(stackTrace)

	// Critical severity indicators
	criticalIndicators := []string{
		"arbitrary code execution", "remote code execution",
		"privilege escalation", "security violation",
		"authentication bypass", "buffer overflow in critical",
		"kernel panic", "system crash",
	}

	for _, indicator := range criticalIndicators {
		if strings.Contains(stackLower, indicator) {
			return types.SeverityCritical
		}
	}

	// High severity indicators
	highIndicators := []string{
		"data corruption", "memory corruption",
		"use after free", "double free",
		"write out of bounds", "format string vulnerability",
		"integer overflow leading to",
	}

	for _, indicator := range highIndicators {
		if strings.Contains(stackLower, indicator) {
			return types.SeverityHigh
		}
	}

	// Type-based severity assignment with context
	switch crashType {
	case types.CrashTypeHeapOverflow, types.CrashTypeStackOverflow:
		// Check if it's exploitable
		if c.isExploitable(stackTrace) {
			return types.SeverityCritical
		}
		return types.SeverityHigh

	case types.CrashTypeSegmentationFault:
		// Check for null pointer vs arbitrary address
		if c.isNullPointer(stackTrace) {
			return types.SeverityMedium
		}
		return types.SeverityHigh

	case types.CrashTypeAssertion, types.CrashTypeUnhandledException:
		// Check if in production code
		if c.isProductionCode(stackTrace) {
			return types.SeverityHigh
		}
		return types.SeverityMedium

	case types.CrashTypeTimeout:
		// Check if it affects availability
		if c.affectsAvailability(stackTrace) {
			return types.SeverityMedium
		}
		return types.SeverityLow

	case types.CrashTypeMemoryLeak:
		// Check leak severity
		if c.isSevereMemoryLeak(stackTrace) {
			return types.SeverityMedium
		}
		return types.SeverityLow

	default:
		return types.SeverityUnknown
	}
}

// Helper methods for classification

func (c *classifier) isUseAfterFree(stackTrace string) bool {
	patterns := []string{
		"use-after-free", "use after free",
		"freed memory", "dangling pointer",
		"accessing freed", "deallocated object",
	}
	stackLower := strings.ToLower(stackTrace)
	for _, pattern := range patterns {
		if strings.Contains(stackLower, pattern) {
			return true
		}
	}
	return false
}

func (c *classifier) isIntegerOverflow(stackTrace string) bool {
	patterns := []string{
		"integer overflow", "signed overflow",
		"unsigned overflow", "arithmetic overflow",
		"value too large", "numeric overflow",
	}
	stackLower := strings.ToLower(stackTrace)
	for _, pattern := range patterns {
		if strings.Contains(stackLower, pattern) {
			return true
		}
	}
	return false
}

func (c *classifier) isExploitable(stackTrace string) bool {
	patterns := []string{
		"exploitable", "arbitrary write",
		"controlled write", "attacker controlled",
		"user controlled", "tainted input",
	}
	stackLower := strings.ToLower(stackTrace)
	for _, pattern := range patterns {
		if strings.Contains(stackLower, pattern) {
			return true
		}
	}
	return false
}

func (c *classifier) isNullPointer(stackTrace string) bool {
	patterns := []string{
		"null pointer", "nullptr", "0x0",
		"address 0x0", "null dereference",
		"accessing null",
	}
	stackLower := strings.ToLower(stackTrace)
	for _, pattern := range patterns {
		if strings.Contains(stackLower, pattern) {
			return true
		}
	}
	return false
}

func (c *classifier) isProductionCode(stackTrace string) bool {
	// Debug/test indicators
	debugPatterns := []string{
		"_test.go", "test_", "_debug",
		"unittest", "gtest", "mock",
		"stub", "fake_", "example",
	}
	stackLower := strings.ToLower(stackTrace)
	for _, pattern := range debugPatterns {
		if strings.Contains(stackLower, pattern) {
			return false
		}
	}
	return true
}

func (c *classifier) affectsAvailability(stackTrace string) bool {
	patterns := []string{
		"service unavailable", "connection refused",
		"server down", "unresponsive",
		"deadlock", "infinite loop",
	}
	stackLower := strings.ToLower(stackTrace)
	for _, pattern := range patterns {
		if strings.Contains(stackLower, pattern) {
			return true
		}
	}
	return false
}

func (c *classifier) isSevereMemoryLeak(stackTrace string) bool {
	patterns := []string{
		"massive leak", "gb leaked",
		"rapid memory growth", "memory exhaustion",
		"oom killer", "system out of memory",
	}
	stackLower := strings.ToLower(stackTrace)
	for _, pattern := range patterns {
		if strings.Contains(stackLower, pattern) {
			return true
		}
	}
	return false
}

// FindSimilarCrashes finds crashes with similar patterns
func (c *classifier) FindSimilarCrashes(ctx context.Context, crash *types.Crash, threshold float64) ([]*types.Crash, error) {
	if crash == nil || crash.Signature == nil {
		return nil, errors.New("crash or signature cannot be nil")
	}

	return c.repo.FindSimilar(ctx, crash.Signature, threshold)
}

// GetKnownPatterns returns all registered crash patterns
func (c *classifier) GetKnownPatterns() []CrashPattern {
	patterns := make([]CrashPattern, len(c.patterns))
	copy(patterns, c.patterns)
	return patterns
}

// RegisterPattern adds a new crash pattern for matching
func (c *classifier) RegisterPattern(pattern CrashPattern) error {
	if pattern.ID == "" {
		return errors.New("pattern ID cannot be empty")
	}

	if _, exists := c.patternMap[pattern.ID]; exists {
		return fmt.Errorf("pattern with ID %s already exists", pattern.ID)
	}

	c.patterns = append(c.patterns, pattern)
	c.patternMap[pattern.ID] = &pattern

	// Sort patterns by priority (descending)
	c.sortPatternsByPriority()

	c.log.WithFields(logrus.Fields{
		"pattern_id":   pattern.ID,
		"pattern_name": pattern.Name,
		"priority":     pattern.Priority,
	}).Debug("Registered new crash pattern")

	return nil
}

func (c *classifier) sortPatternsByPriority() {
	// Simple bubble sort for small pattern sets
	n := len(c.patterns)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if c.patterns[j].Priority < c.patterns[j+1].Priority {
				c.patterns[j], c.patterns[j+1] = c.patterns[j+1], c.patterns[j]
			}
		}
	}
}

// registerDefaultPatterns registers the default set of crash patterns
func (c *classifier) registerDefaultPatterns() error {
	defaultPatterns := []CrashPattern{
		{
			ID:          "heap-uaf-01",
			Name:        "Heap Use After Free",
			Type:        types.CrashTypeHeapOverflow,
			Severity:    types.SeverityHigh,
			Pattern:     regexp.MustCompile(`(?i)heap-use-after-free|use[- ]after[- ]free|freed.*access`),
			Keywords:    []string{"use-after-free", "heap"},
			Description: "Memory accessed after being freed",
			Priority:    100,
		},
		{
			ID:          "stack-bof-01",
			Name:        "Stack Buffer Overflow",
			Type:        types.CrashTypeStackOverflow,
			Severity:    types.SeverityHigh,
			Pattern:     regexp.MustCompile(`(?i)stack[- ]buffer[- ]overflow|stack.*overflow.*detected`),
			Keywords:    []string{"stack", "buffer", "overflow"},
			Description: "Stack buffer overflow detected",
			Priority:    95,
		},
		{
			ID:          "null-deref-01",
			Name:        "Null Pointer Dereference",
			Type:        types.CrashTypeSegmentationFault,
			Severity:    types.SeverityMedium,
			Pattern:     regexp.MustCompile(`(?i)null[- ]pointer|nullptr|accessing.*0x0+[^1-9a-f]`),
			Keywords:    []string{"null", "pointer"},
			Description: "Attempted to dereference a null pointer",
			Priority:    90,
		},
		{
			ID:          "assert-fail-01",
			Name:        "Assertion Failure",
			Type:        types.CrashTypeAssertion,
			Severity:    types.SeverityMedium,
			Pattern:     regexp.MustCompile(`(?i)assert.*fail|assertion.*failed|panic:|fatal error:`),
			Keywords:    []string{"assert", "failed"},
			Description: "Assertion or invariant check failed",
			Priority:    85,
		},
		{
			ID:          "double-free-01",
			Name:        "Double Free",
			Type:        types.CrashTypeHeapOverflow,
			Severity:    types.SeverityHigh,
			Pattern:     regexp.MustCompile(`(?i)double[- ]free|free.*already.*freed|corrupted.*double.*linked`),
			Keywords:    []string{"double", "free"},
			Description: "Memory freed multiple times",
			Priority:    95,
		},
		{
			ID:          "oom-01",
			Name:        "Out of Memory",
			Type:        types.CrashTypeMemoryLeak,
			Severity:    types.SeverityMedium,
			Pattern:     regexp.MustCompile(`(?i)out[- ]of[- ]memory|cannot allocate|memory exhausted|oom`),
			Keywords:    []string{"memory", "exhausted"},
			Description: "System ran out of available memory",
			Priority:    80,
		},
		{
			ID:          "integer-overflow-01",
			Name:        "Integer Overflow",
			Type:        types.CrashTypeOther,
			Severity:    types.SeverityMedium,
			Pattern:     regexp.MustCompile(`(?i)integer[- ]overflow|arithmetic overflow|value too large`),
			Keywords:    []string{"integer", "overflow"},
			Description: "Integer overflow detected",
			Priority:    75,
		},
		{
			ID:          "race-condition-01",
			Name:        "Race Condition",
			Type:        types.CrashTypeOther,
			Severity:    types.SeverityHigh,
			Pattern:     regexp.MustCompile(`(?i)race[- ]condition|data[- ]race|concurrent.*access`),
			Keywords:    []string{"race", "concurrent"},
			Description: "Race condition or concurrent access violation",
			Priority:    85,
		},
		{
			ID:          "deadlock-01",
			Name:        "Deadlock",
			Type:        types.CrashTypeTimeout,
			Severity:    types.SeverityMedium,
			Pattern:     regexp.MustCompile(`(?i)deadlock|circular.*wait|mutex.*timeout`),
			Keywords:    []string{"deadlock"},
			Description: "Deadlock condition detected",
			Priority:    80,
		},
		{
			ID:          "format-string-01",
			Name:        "Format String Vulnerability",
			Type:        types.CrashTypeOther,
			Severity:    types.SeverityCritical,
			Pattern:     regexp.MustCompile(`(?i)format[- ]string|printf.*vulnerability|%[nspx].*user.*input`),
			Keywords:    []string{"format", "string", "vulnerability"},
			Description: "Format string vulnerability detected",
			Priority:    100,
		},
	}

	for _, pattern := range defaultPatterns {
		if err := c.RegisterPattern(pattern); err != nil {
			return fmt.Errorf("failed to register pattern %s: %w", pattern.ID, err)
		}
	}

	return nil
}
