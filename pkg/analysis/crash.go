package analysis

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// CrashAnalyzer performs crash analysis and deduplication
type CrashAnalyzer struct {
	logger         *logrus.Logger
	config         *AnalyzerConfig
	signatureCache map[string]*CrashSignature
	patterns       map[string]*regexp.Regexp
}

// AnalyzerConfig contains configuration for crash analysis
type AnalyzerConfig struct {
	EnableStackTrace     bool
	EnableSymbolization  bool
	EnableDeduplication  bool
	EnableTriage         bool
	MaxStackFrames       int
	MinStackFrames       int
	SymbolsPath          string
	OutputDirectory      string
	DeduplicationMethod  string // "stack", "hash", "fuzzy"
}

// CrashSignature represents a unique crash signature
type CrashSignature struct {
	ID               string
	Hash             string
	Type             CrashType
	Severity         SeverityLevel
	StackFingerprint string
	TopFrames        []string
	FaultAddress     string
	SignalNumber     int
	Exploitability   ExploitabilityLevel
}

// CrashType represents the type of crash
type CrashType string

const (
	CrashTypeSegFault        CrashType = "segmentation_fault"
	CrashTypeAbort           CrashType = "abort"
	CrashTypeStackOverflow   CrashType = "stack_overflow"
	CrashTypeHeapOverflow    CrashType = "heap_overflow"
	CrashTypeUseAfterFree    CrashType = "use_after_free"
	CrashTypeDoubleFree      CrashType = "double_free"
	CrashTypeNullDeref       CrashType = "null_dereference"
	CrashTypeAssert          CrashType = "assertion_failure"
	CrashTypeTimeout         CrashType = "timeout"
	CrashTypeMemoryLeak      CrashType = "memory_leak"
	CrashTypeIntegerOverflow CrashType = "integer_overflow"
	CrashTypeUnknown         CrashType = "unknown"
)

// SeverityLevel represents crash severity
type SeverityLevel string

const (
	SeverityCritical SeverityLevel = "critical"
	SeverityHigh     SeverityLevel = "high"
	SeverityMedium   SeverityLevel = "medium"
	SeverityLow      SeverityLevel = "low"
	SeverityInfo     SeverityLevel = "info"
)

// ExploitabilityLevel represents crash exploitability
type ExploitabilityLevel string

const (
	ExploitabilityHigh       ExploitabilityLevel = "high"
	ExploitabilityMedium     ExploitabilityLevel = "medium"
	ExploitabilityLow        ExploitabilityLevel = "low"
	ExploitabilityUnknown    ExploitabilityLevel = "unknown"
	ExploitabilityNotCrash   ExploitabilityLevel = "not_crash"
)

// AnalysisResult contains the complete crash analysis
type AnalysisResult struct {
	CrashResult      *common.CrashResult
	Signature        *CrashSignature
	StackTrace       *StackTrace
	Triage           *TriageInfo
	Similar          []*common.CrashResult
	IsUnique         bool
	IsSecurity       bool
	Recommendations  []string
	AnalysisTime     time.Duration
}

// StackTrace represents parsed stack trace information
type StackTrace struct {
	Frames       []StackFrame
	Signal       string
	SignalNumber int
	FaultAddress string
	Registers    map[string]string
	RawTrace     string
}

// StackFrame represents a single stack frame
type StackFrame struct {
	Number       int
	Address      string
	Function     string
	Source       string
	Line         int
	Module       string
	Offset       string
	IsSymbolized bool
}

// TriageInfo contains crash triage information
type TriageInfo struct {
	Priority         int
	AssignedTo       string
	Status           string
	Tags             []string
	Notes            string
	FirstSeen        time.Time
	LastSeen         time.Time
	OccurrenceCount  int
	AffectedVersions []string
}

// NewCrashAnalyzer creates a new crash analyzer
func NewCrashAnalyzer(config *AnalyzerConfig, logger *logrus.Logger) *CrashAnalyzer {
	
	// Set defaults
	if config.MaxStackFrames == 0 {
		config.MaxStackFrames = 50
	}
	if config.MinStackFrames == 0 {
		config.MinStackFrames = 3
	}
	if config.DeduplicationMethod == "" {
		config.DeduplicationMethod = "stack"
	}
	
	analyzer := &CrashAnalyzer{
		logger:         logger,
		config:         config,
		signatureCache: make(map[string]*CrashSignature),
		patterns:       compilePatterns(),
	}
	
	return analyzer
}

// compilePatterns compiles regular expressions for crash analysis
func compilePatterns() map[string]*regexp.Regexp {
	patterns := map[string]string{
		"stack_frame":     `#(\d+)\s+0x([0-9a-fA-F]+)\s+in\s+(\S+)\s*\(([^)]*)\)(?:\s+at\s+([^:]+):(\d+))?`,
		"signal":          `Program received signal (\w+)`,
		"signal_num":      `Signal (\d+) \((\w+)\)`,
		"fault_address":   `(?:faulting address|address):\s*0x([0-9a-fA-F]+)`,
		"asan_error":      `==\d+==ERROR: AddressSanitizer: (\S+)`,
		"msan_error":      `==\d+==ERROR: MemorySanitizer: (\S+)`,
		"ubsan_error":     `runtime error: (.+)`,
		"assert_fail":     `Assertion '([^']+)' failed`,
		"segfault":        `Segmentation fault|SIGSEGV`,
		"abort":           `Aborted|SIGABRT`,
		"heap_overflow":   `heap-buffer-overflow`,
		"stack_overflow":  `stack-buffer-overflow|stack overflow`,
		"use_after_free":  `heap-use-after-free`,
		"double_free":     `double-free|attempting free on address which was not malloc`,
		"null_deref":      `null pointer dereference|SEGV on unknown address 0x000000000000`,
		"memory_leak":     `LeakSanitizer: detected memory leaks`,
		"integer_overflow": `integer overflow|signed integer overflow`,
	}
	
	compiled := make(map[string]*regexp.Regexp)
	for name, pattern := range patterns {
		compiled[name] = regexp.MustCompile(pattern)
	}
	
	return compiled
}

// AnalyzeCrash performs comprehensive crash analysis
func (ca *CrashAnalyzer) AnalyzeCrash(crash *common.CrashResult) (*AnalysisResult, error) {
	startTime := time.Now()
	
	ca.logger.WithFields(logrus.Fields{
		"crash_id": crash.ID,
		"job_id":   crash.JobID,
		"size":     crash.Size,
	}).Info("Starting crash analysis")
	
	result := &AnalysisResult{
		CrashResult: crash,
		IsUnique:    true,
	}
	
	// Parse stack trace
	if ca.config.EnableStackTrace {
		stackTrace, err := ca.parseStackTrace(crash)
		if err != nil {
			ca.logger.WithError(err).Warn("Failed to parse stack trace")
		} else {
			result.StackTrace = stackTrace
		}
	}
	
	// Generate crash signature
	signature := ca.generateSignature(crash, result.StackTrace)
	result.Signature = signature
	
	// Store in cache
	ca.signatureCache[signature.Hash] = signature
	
	// Deduplicate
	if ca.config.EnableDeduplication {
		similar := ca.findSimilarCrashes(signature)
		if len(similar) > 0 {
			result.IsUnique = false
			result.Similar = similar
		}
	}
	
	// Determine crash type and severity
	crashType := ca.determineCrashType(crash, result.StackTrace)
	signature.Type = crashType
	signature.Severity = ca.calculateSeverity(crashType, result.StackTrace)
	signature.Exploitability = ca.assessExploitability(crashType, result.StackTrace)
	
	// Security assessment
	result.IsSecurity = ca.isSecurityRelevant(crashType, signature.Exploitability)
	
	// Triage
	if ca.config.EnableTriage {
		result.Triage = ca.triageCrash(crash, signature)
	}
	
	// Generate recommendations
	result.Recommendations = ca.generateRecommendations(crashType, signature, result.StackTrace)
	
	result.AnalysisTime = time.Since(startTime)
	
	ca.logger.WithFields(logrus.Fields{
		"crash_id":       crash.ID,
		"type":           crashType,
		"severity":       signature.Severity,
		"exploitability": signature.Exploitability,
		"is_unique":      result.IsUnique,
		"is_security":    result.IsSecurity,
		"analysis_time":  result.AnalysisTime,
	}).Info("Crash analysis completed")
	
	return result, nil
}

// parseStackTrace extracts stack trace information
func (ca *CrashAnalyzer) parseStackTrace(crash *common.CrashResult) (*StackTrace, error) {
	if crash.StackTrace == "" && crash.Output == "" {
		return nil, fmt.Errorf("no stack trace or output available")
	}
	
	// Use stack trace if available, otherwise try to extract from output
	traceData := crash.StackTrace
	if traceData == "" {
		traceData = crash.Output
	}
	
	stackTrace := &StackTrace{
		Frames:    make([]StackFrame, 0),
		Registers: make(map[string]string),
		RawTrace:  traceData,
	}
	
	scanner := bufio.NewScanner(strings.NewReader(traceData))
	frameNum := 0
	
	for scanner.Scan() {
		line := scanner.Text()
		
		// Parse stack frames
		if matches := ca.patterns["stack_frame"].FindStringSubmatch(line); len(matches) > 0 {
			frame := StackFrame{
				Number:       frameNum,
				Address:      matches[2],
				Function:     matches[3],
				IsSymbolized: matches[3] != "??",
			}
			
			if len(matches) > 5 && matches[5] != "" {
				frame.Source = matches[5]
				if len(matches) > 6 {
					fmt.Sscanf(matches[6], "%d", &frame.Line)
				}
			}
			
			stackTrace.Frames = append(stackTrace.Frames, frame)
			frameNum++
			
			if frameNum >= ca.config.MaxStackFrames {
				break
			}
		}
		
		// Parse signal information
		if matches := ca.patterns["signal"].FindStringSubmatch(line); len(matches) > 1 {
			stackTrace.Signal = matches[1]
		}
		
		if matches := ca.patterns["signal_num"].FindStringSubmatch(line); len(matches) > 2 {
			fmt.Sscanf(matches[1], "%d", &stackTrace.SignalNumber)
			if stackTrace.Signal == "" {
				stackTrace.Signal = matches[2]
			}
		}
		
		// Parse fault address
		if matches := ca.patterns["fault_address"].FindStringSubmatch(line); len(matches) > 1 {
			stackTrace.FaultAddress = matches[1]
		}
		
		// Parse registers (simplified)
		if strings.Contains(line, "rax=") || strings.Contains(line, "eax=") {
			// Extract register values with a simple approach
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.Contains(part, "=") {
					kv := strings.SplitN(part, "=", 2)
					if len(kv) == 2 {
						stackTrace.Registers[kv[0]] = kv[1]
					}
				}
			}
		}
	}
	
	// Symbolize if enabled and needed
	if ca.config.EnableSymbolization {
		ca.symbolizeStackTrace(stackTrace)
	}
	
	return stackTrace, nil
}

// generateSignature creates a unique signature for the crash
func (ca *CrashAnalyzer) generateSignature(crash *common.CrashResult, stackTrace *StackTrace) *CrashSignature {
	signature := &CrashSignature{
		ID:           crash.ID,
		Hash:         crash.Hash,
		TopFrames:    make([]string, 0),
		FaultAddress: "",
	}
	
	// Extract top frames for signature
	if stackTrace != nil && len(stackTrace.Frames) > 0 {
		numFrames := ca.config.MinStackFrames
		if len(stackTrace.Frames) < numFrames {
			numFrames = len(stackTrace.Frames)
		}
		
		for i := 0; i < numFrames; i++ {
			frame := stackTrace.Frames[i]
			frameStr := frame.Function
			if frameStr == "" || frameStr == "??" {
				frameStr = frame.Address
			}
			signature.TopFrames = append(signature.TopFrames, frameStr)
		}
		
		signature.FaultAddress = stackTrace.FaultAddress
		signature.SignalNumber = stackTrace.SignalNumber
	}
	
	// Generate stack fingerprint based on deduplication method
	switch ca.config.DeduplicationMethod {
	case "stack":
		signature.StackFingerprint = ca.generateStackFingerprint(signature.TopFrames)
	case "fuzzy":
		signature.StackFingerprint = ca.generateFuzzyFingerprint(crash, stackTrace)
	default:
		signature.StackFingerprint = crash.Hash
	}
	
	// Generate unique hash for the signature
	h := sha256.New()
	h.Write([]byte(signature.StackFingerprint))
	h.Write([]byte(fmt.Sprintf("%d", signature.SignalNumber)))
	signature.Hash = hex.EncodeToString(h.Sum(nil))[:16]
	
	return signature
}

// generateStackFingerprint creates a fingerprint from stack frames
func (ca *CrashAnalyzer) generateStackFingerprint(frames []string) string {
	// Normalize and join frames
	normalized := make([]string, len(frames))
	for i, frame := range frames {
		// Remove addresses and offsets for better matching
		normalized[i] = normalizeFrameFunction(frame)
	}
	
	return strings.Join(normalized, "|")
}

// normalizeFrameFunction normalizes a function name for comparison
func normalizeFrameFunction(function string) string {
	// Remove common prefixes and suffixes
	function = strings.TrimPrefix(function, "__")
	function = strings.TrimSuffix(function, "_impl")
	
	// Remove template parameters
	if idx := strings.Index(function, "<"); idx > 0 {
		function = function[:idx]
	}
	
	// Remove parameters
	if idx := strings.Index(function, "("); idx > 0 {
		function = function[:idx]
	}
	
	return function
}

// generateFuzzyFingerprint creates a fuzzy fingerprint for similarity matching
func (ca *CrashAnalyzer) generateFuzzyFingerprint(crash *common.CrashResult, stackTrace *StackTrace) string {
	parts := []string{}
	
	// Include crash type
	crashType := ca.determineCrashType(crash, stackTrace)
	parts = append(parts, string(crashType))
	
	// Include top function names (normalized)
	if stackTrace != nil {
		for i, frame := range stackTrace.Frames {
			if i >= 3 {
				break
			}
			if frame.Function != "" && frame.Function != "??" {
				parts = append(parts, normalizeFrameFunction(frame.Function))
			}
		}
	}
	
	// Include signal if available
	if stackTrace != nil && stackTrace.Signal != "" {
		parts = append(parts, stackTrace.Signal)
	}
	
	return strings.Join(parts, ":")
}

// findSimilarCrashes finds crashes with similar signatures
func (ca *CrashAnalyzer) findSimilarCrashes(signature *CrashSignature) []*common.CrashResult {
	similar := make([]*common.CrashResult, 0)
	
	// This is a simplified implementation
	// In production, this would query a database or index
	for hash, cached := range ca.signatureCache {
		if hash == signature.Hash {
			continue
		}
		
		// Check similarity based on method
		if ca.areSimilar(signature, cached) {
			// Would retrieve actual crash result from storage
			similar = append(similar, &common.CrashResult{
				ID:   cached.ID,
				Hash: cached.Hash,
			})
		}
	}
	
	return similar
}

// areSimilar checks if two signatures are similar
func (ca *CrashAnalyzer) areSimilar(sig1, sig2 *CrashSignature) bool {
	switch ca.config.DeduplicationMethod {
	case "stack":
		return sig1.StackFingerprint == sig2.StackFingerprint
	case "fuzzy":
		return ca.fuzzyMatch(sig1.StackFingerprint, sig2.StackFingerprint)
	default:
		return sig1.Hash == sig2.Hash
	}
}

// fuzzyMatch performs fuzzy string matching
func (ca *CrashAnalyzer) fuzzyMatch(s1, s2 string) bool {
	// Simple implementation - check if strings share significant parts
	parts1 := strings.Split(s1, ":")
	parts2 := strings.Split(s2, ":")
	
	if len(parts1) == 0 || len(parts2) == 0 {
		return false
	}
	
	// Type must match
	if parts1[0] != parts2[0] {
		return false
	}
	
	// Check frame similarity
	matches := 0
	for i := 1; i < len(parts1) && i < len(parts2); i++ {
		if parts1[i] == parts2[i] {
			matches++
		}
	}
	
	// Consider similar if at least 50% of frames match
	minLen := len(parts1) - 1
	if len(parts2)-1 < minLen {
		minLen = len(parts2) - 1
	}
	
	return minLen > 0 && float64(matches)/float64(minLen) >= 0.5
}

// determineCrashType identifies the type of crash
func (ca *CrashAnalyzer) determineCrashType(crash *common.CrashResult, stackTrace *StackTrace) CrashType {
	// Check output and stack trace for patterns
	data := crash.Output + crash.StackTrace
	if stackTrace != nil {
		data += stackTrace.RawTrace
	}
	
	// Check sanitizer outputs first (most specific)
	if ca.patterns["asan_error"].MatchString(data) {
		matches := ca.patterns["asan_error"].FindStringSubmatch(data)
		if len(matches) > 1 {
			switch matches[1] {
			case "heap-buffer-overflow":
				return CrashTypeHeapOverflow
			case "stack-buffer-overflow":
				return CrashTypeStackOverflow
			case "heap-use-after-free":
				return CrashTypeUseAfterFree
			case "double-free":
				return CrashTypeDoubleFree
			}
		}
	}
	
	// Check other patterns
	if ca.patterns["null_deref"].MatchString(data) {
		return CrashTypeNullDeref
	}
	if ca.patterns["assert_fail"].MatchString(data) {
		return CrashTypeAssert
	}
	if ca.patterns["memory_leak"].MatchString(data) {
		return CrashTypeMemoryLeak
	}
	if ca.patterns["integer_overflow"].MatchString(data) {
		return CrashTypeIntegerOverflow
	}
	if ca.patterns["segfault"].MatchString(data) {
		return CrashTypeSegFault
	}
	if ca.patterns["abort"].MatchString(data) {
		return CrashTypeAbort
	}
	
	// Check by signal
	if stackTrace != nil {
		switch stackTrace.Signal {
		case "SIGSEGV":
			return CrashTypeSegFault
		case "SIGABRT":
			return CrashTypeAbort
		case "SIGFPE":
			return CrashTypeIntegerOverflow
		}
	}
	
	// Check crash type from metadata
	if crash.Type != "" {
		switch strings.ToLower(crash.Type) {
		case "timeout":
			return CrashTypeTimeout
		case "oom", "out_of_memory":
			return CrashTypeHeapOverflow
		}
	}
	
	return CrashTypeUnknown
}

// calculateSeverity determines crash severity
func (ca *CrashAnalyzer) calculateSeverity(crashType CrashType, stackTrace *StackTrace) SeverityLevel {
	// Base severity by crash type
	baseSeverity := map[CrashType]SeverityLevel{
		CrashTypeHeapOverflow:    SeverityCritical,
		CrashTypeStackOverflow:   SeverityCritical,
		CrashTypeUseAfterFree:    SeverityCritical,
		CrashTypeDoubleFree:      SeverityHigh,
		CrashTypeIntegerOverflow: SeverityHigh,
		CrashTypeNullDeref:       SeverityMedium,
		CrashTypeSegFault:        SeverityMedium,
		CrashTypeAbort:           SeverityLow,
		CrashTypeAssert:          SeverityLow,
		CrashTypeTimeout:         SeverityInfo,
		CrashTypeMemoryLeak:      SeverityLow,
		CrashTypeUnknown:         SeverityMedium,
	}
	
	severity := baseSeverity[crashType]
	
	// Adjust based on additional factors
	if stackTrace != nil && len(stackTrace.Frames) > 0 {
		// Check for sensitive functions
		for _, frame := range stackTrace.Frames {
			fn := strings.ToLower(frame.Function)
			if strings.Contains(fn, "malloc") || strings.Contains(fn, "free") {
				severity = upgradeSeverity(severity)
			}
			if strings.Contains(fn, "strcpy") || strings.Contains(fn, "sprintf") {
				severity = upgradeSeverity(severity)
			}
			if strings.Contains(fn, "system") || strings.Contains(fn, "exec") {
				severity = SeverityCritical
			}
		}
	}
	
	return severity
}

// upgradeSeverity increases severity level
func upgradeSeverity(current SeverityLevel) SeverityLevel {
	switch current {
	case SeverityInfo:
		return SeverityLow
	case SeverityLow:
		return SeverityMedium
	case SeverityMedium:
		return SeverityHigh
	case SeverityHigh:
		return SeverityCritical
	default:
		return current
	}
}

// assessExploitability evaluates crash exploitability
func (ca *CrashAnalyzer) assessExploitability(crashType CrashType, stackTrace *StackTrace) ExploitabilityLevel {
	// Base exploitability by crash type
	baseExploitability := map[CrashType]ExploitabilityLevel{
		CrashTypeHeapOverflow:    ExploitabilityHigh,
		CrashTypeStackOverflow:   ExploitabilityHigh,
		CrashTypeUseAfterFree:    ExploitabilityHigh,
		CrashTypeDoubleFree:      ExploitabilityMedium,
		CrashTypeIntegerOverflow: ExploitabilityMedium,
		CrashTypeNullDeref:       ExploitabilityLow,
		CrashTypeSegFault:        ExploitabilityUnknown,
		CrashTypeAbort:           ExploitabilityNotCrash,
		CrashTypeAssert:          ExploitabilityNotCrash,
		CrashTypeTimeout:         ExploitabilityNotCrash,
		CrashTypeMemoryLeak:      ExploitabilityNotCrash,
		CrashTypeUnknown:         ExploitabilityUnknown,
	}
	
	exploitability := baseExploitability[crashType]
	
	// Adjust based on crash characteristics
	if stackTrace != nil {
		// Check if crash is in user-controlled path
		for _, frame := range stackTrace.Frames {
			if strings.Contains(frame.Function, "parse") ||
				strings.Contains(frame.Function, "decode") ||
				strings.Contains(frame.Function, "input") {
				if exploitability == ExploitabilityLow {
					exploitability = ExploitabilityMedium
				}
			}
		}
		
		// Check fault address
		if stackTrace.FaultAddress != "" {
			addr := strings.ToLower(stackTrace.FaultAddress)
			// Near-null crashes are typically less exploitable
			if strings.HasPrefix(addr, "0x0000") {
				if exploitability == ExploitabilityHigh {
					exploitability = ExploitabilityMedium
				}
			}
		}
	}
	
	return exploitability
}

// isSecurityRelevant determines if crash is security-relevant
func (ca *CrashAnalyzer) isSecurityRelevant(crashType CrashType, exploitability ExploitabilityLevel) bool {
	// Security-relevant crash types
	securityTypes := map[CrashType]bool{
		CrashTypeHeapOverflow:    true,
		CrashTypeStackOverflow:   true,
		CrashTypeUseAfterFree:    true,
		CrashTypeDoubleFree:      true,
		CrashTypeIntegerOverflow: true,
	}
	
	if securityTypes[crashType] {
		return true
	}
	
	// Also consider exploitability
	return exploitability == ExploitabilityHigh || exploitability == ExploitabilityMedium
}

// triageCrash performs crash triage
func (ca *CrashAnalyzer) triageCrash(crash *common.CrashResult, signature *CrashSignature) *TriageInfo {
	triage := &TriageInfo{
		FirstSeen:       crash.Timestamp,
		LastSeen:        crash.Timestamp,
		OccurrenceCount: 1,
		Tags:            make([]string, 0),
		Status:          "new",
	}
	
	// Set priority based on severity and exploitability
	priorityMap := map[SeverityLevel]int{
		SeverityCritical: 1,
		SeverityHigh:     2,
		SeverityMedium:   3,
		SeverityLow:      4,
		SeverityInfo:     5,
	}
	
	triage.Priority = priorityMap[signature.Severity]
	
	// Adjust priority based on exploitability
	if signature.Exploitability == ExploitabilityHigh {
		triage.Priority = 1
	}
	
	// Add tags
	triage.Tags = append(triage.Tags, string(signature.Type))
	triage.Tags = append(triage.Tags, string(signature.Severity))
	
	if ca.isSecurityRelevant(signature.Type, signature.Exploitability) {
		triage.Tags = append(triage.Tags, "security")
		triage.Status = "needs_review"
	}
	
	if signature.Exploitability != ExploitabilityNotCrash {
		triage.Tags = append(triage.Tags, fmt.Sprintf("exploitability_%s", signature.Exploitability))
	}
	
	// Generate notes
	notes := []string{
		fmt.Sprintf("Crash Type: %s", signature.Type),
		fmt.Sprintf("Severity: %s", signature.Severity),
		fmt.Sprintf("Exploitability: %s", signature.Exploitability),
	}
	
	if len(signature.TopFrames) > 0 {
		notes = append(notes, fmt.Sprintf("Top Frame: %s", signature.TopFrames[0]))
	}
	
	triage.Notes = strings.Join(notes, "\n")
	
	return triage
}

// generateRecommendations creates actionable recommendations
func (ca *CrashAnalyzer) generateRecommendations(crashType CrashType, signature *CrashSignature, stackTrace *StackTrace) []string {
	recommendations := make([]string, 0)
	
	// Type-specific recommendations
	switch crashType {
	case CrashTypeHeapOverflow:
		recommendations = append(recommendations, "Enable AddressSanitizer (ASAN) in development builds")
		recommendations = append(recommendations, "Review all heap allocations in the affected code path")
		recommendations = append(recommendations, "Consider using safe string functions")
		
	case CrashTypeStackOverflow:
		recommendations = append(recommendations, "Review recursive functions for proper termination")
		recommendations = append(recommendations, "Check for large stack allocations")
		recommendations = append(recommendations, "Consider increasing stack size or using heap allocation")
		
	case CrashTypeUseAfterFree:
		recommendations = append(recommendations, "Enable AddressSanitizer (ASAN) to catch all UAF bugs")
		recommendations = append(recommendations, "Review object lifetime management")
		recommendations = append(recommendations, "Consider using smart pointers or reference counting")
		
	case CrashTypeDoubleFree:
		recommendations = append(recommendations, "Review all free() calls in the code path")
		recommendations = append(recommendations, "Ensure pointers are nullified after freeing")
		recommendations = append(recommendations, "Consider using a memory pool or custom allocator")
		
	case CrashTypeNullDeref:
		recommendations = append(recommendations, "Add null pointer checks before dereferencing")
		recommendations = append(recommendations, "Review error handling in the affected code")
		recommendations = append(recommendations, "Consider using assertions in debug builds")
		
	case CrashTypeIntegerOverflow:
		recommendations = append(recommendations, "Enable UndefinedBehaviorSanitizer (UBSAN)")
		recommendations = append(recommendations, "Use safe integer arithmetic functions")
		recommendations = append(recommendations, "Add overflow checks for user-controlled values")
		
	case CrashTypeMemoryLeak:
		recommendations = append(recommendations, "Enable LeakSanitizer in testing")
		recommendations = append(recommendations, "Review all allocation/deallocation pairs")
		recommendations = append(recommendations, "Consider using RAII or garbage collection")
		
	case CrashTypeTimeout:
		recommendations = append(recommendations, "Profile the code to identify performance bottlenecks")
		recommendations = append(recommendations, "Add timeout handling for long operations")
		recommendations = append(recommendations, "Consider optimizing algorithms or data structures")
		
	case CrashTypeAssert:
		recommendations = append(recommendations, "Review the assertion condition")
		recommendations = append(recommendations, "Determine if this is a logic error or invalid input")
		recommendations = append(recommendations, "Add proper error handling instead of assertions in production")
	}
	
	// Severity-based recommendations
	if signature.Severity == SeverityCritical || signature.Severity == SeverityHigh {
		recommendations = append(recommendations, "Prioritize fixing this issue immediately")
		recommendations = append(recommendations, "Consider releasing a security patch")
	}
	
	// Exploitability-based recommendations
	if signature.Exploitability == ExploitabilityHigh {
		recommendations = append(recommendations, "This crash appears to be exploitable")
		recommendations = append(recommendations, "Conduct a thorough security review")
		recommendations = append(recommendations, "Consider implementing additional mitigations (ASLR, DEP, etc.)")
	}
	
	// General recommendations
	recommendations = append(recommendations, "Add a regression test for this crash")
	recommendations = append(recommendations, "Review similar code patterns for the same issue")
	
	return recommendations
}

// symbolizeStackTrace attempts to symbolize stack frames
func (ca *CrashAnalyzer) symbolizeStackTrace(stackTrace *StackTrace) {
	if ca.config.SymbolsPath == "" {
		return
	}
	
	// This is a placeholder for actual symbolization
	// In production, this would use addr2line, llvm-symbolizer, or similar tools
	for i := range stackTrace.Frames {
		frame := &stackTrace.Frames[i]
		if !frame.IsSymbolized && frame.Address != "" {
			// Attempt symbolization
			// This would involve calling external tools or using symbol files
			ca.logger.WithField("address", frame.Address).Debug("Would symbolize address")
		}
	}
}

// SaveAnalysisResult saves the analysis result to disk
func (ca *CrashAnalyzer) SaveAnalysisResult(result *AnalysisResult) error {
	if ca.config.OutputDirectory == "" {
		return nil
	}
	
	// Create output directory
	outputDir := filepath.Join(ca.config.OutputDirectory, "analysis", result.CrashResult.ID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	
	// Save analysis report
	reportPath := filepath.Join(outputDir, "analysis.txt")
	report := ca.generateReport(result)
	
	if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
		return fmt.Errorf("failed to write analysis report: %w", err)
	}
	
	// Save minimized input if available
	if len(result.CrashResult.Input) > 0 {
		inputPath := filepath.Join(outputDir, "input.bin")
		if err := os.WriteFile(inputPath, result.CrashResult.Input, 0644); err != nil {
			ca.logger.WithError(err).Warn("Failed to save crash input")
		}
	}
	
	return nil
}

// generateReport creates a human-readable analysis report
func (ca *CrashAnalyzer) generateReport(result *AnalysisResult) string {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	
	fmt.Fprintf(w, "Crash Analysis Report\n")
	fmt.Fprintf(w, "====================\n\n")
	
	fmt.Fprintf(w, "Crash ID: %s\n", result.CrashResult.ID)
	fmt.Fprintf(w, "Job ID: %s\n", result.CrashResult.JobID)
	fmt.Fprintf(w, "Timestamp: %s\n", result.CrashResult.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(w, "Analysis Time: %v\n\n", result.AnalysisTime)
	
	if result.Signature != nil {
		fmt.Fprintf(w, "Signature\n")
		fmt.Fprintf(w, "---------\n")
		fmt.Fprintf(w, "Hash: %s\n", result.Signature.Hash)
		fmt.Fprintf(w, "Type: %s\n", result.Signature.Type)
		fmt.Fprintf(w, "Severity: %s\n", result.Signature.Severity)
		fmt.Fprintf(w, "Exploitability: %s\n", result.Signature.Exploitability)
		fmt.Fprintf(w, "Is Unique: %v\n", result.IsUnique)
		fmt.Fprintf(w, "Is Security: %v\n\n", result.IsSecurity)
		
		if len(result.Signature.TopFrames) > 0 {
			fmt.Fprintf(w, "Top Frames:\n")
			for i, frame := range result.Signature.TopFrames {
				fmt.Fprintf(w, "  %d: %s\n", i, frame)
			}
			fmt.Fprintf(w, "\n")
		}
	}
	
	if result.StackTrace != nil {
		fmt.Fprintf(w, "Stack Trace\n")
		fmt.Fprintf(w, "-----------\n")
		if result.StackTrace.Signal != "" {
			fmt.Fprintf(w, "Signal: %s (%d)\n", result.StackTrace.Signal, result.StackTrace.SignalNumber)
		}
		if result.StackTrace.FaultAddress != "" {
			fmt.Fprintf(w, "Fault Address: 0x%s\n", result.StackTrace.FaultAddress)
		}
		fmt.Fprintf(w, "\n")
		
		for _, frame := range result.StackTrace.Frames {
			fmt.Fprintf(w, "#%d  0x%s in %s", frame.Number, frame.Address, frame.Function)
			if frame.Source != "" {
				fmt.Fprintf(w, " at %s:%d", frame.Source, frame.Line)
			}
			fmt.Fprintf(w, "\n")
		}
		fmt.Fprintf(w, "\n")
	}
	
	if result.Triage != nil {
		fmt.Fprintf(w, "Triage Information\n")
		fmt.Fprintf(w, "-----------------\n")
		fmt.Fprintf(w, "Priority: %d\n", result.Triage.Priority)
		fmt.Fprintf(w, "Status: %s\n", result.Triage.Status)
		fmt.Fprintf(w, "Tags: %s\n", strings.Join(result.Triage.Tags, ", "))
		fmt.Fprintf(w, "Notes:\n%s\n\n", result.Triage.Notes)
	}
	
	if len(result.Recommendations) > 0 {
		fmt.Fprintf(w, "Recommendations\n")
		fmt.Fprintf(w, "--------------\n")
		for i, rec := range result.Recommendations {
			fmt.Fprintf(w, "%d. %s\n", i+1, rec)
		}
		fmt.Fprintf(w, "\n")
	}
	
	if !result.IsUnique && len(result.Similar) > 0 {
		fmt.Fprintf(w, "Similar Crashes\n")
		fmt.Fprintf(w, "--------------\n")
		for _, similar := range result.Similar {
			fmt.Fprintf(w, "- %s (hash: %s)\n", similar.ID, similar.Hash)
		}
	}
	
	w.Flush()
	return buf.String()
}

// BatchAnalyze analyzes multiple crashes
func (ca *CrashAnalyzer) BatchAnalyze(crashes []*common.CrashResult) ([]*AnalysisResult, error) {
	results := make([]*AnalysisResult, 0, len(crashes))
	
	// Sort by timestamp for better deduplication
	sort.Slice(crashes, func(i, j int) bool {
		return crashes[i].Timestamp.Before(crashes[j].Timestamp)
	})
	
	for _, crash := range crashes {
		result, err := ca.AnalyzeCrash(crash)
		if err != nil {
			ca.logger.WithError(err).WithField("crash_id", crash.ID).Error("Failed to analyze crash")
			continue
		}
		
		results = append(results, result)
		
		// Save result if configured
		if err := ca.SaveAnalysisResult(result); err != nil {
			ca.logger.WithError(err).WithField("crash_id", crash.ID).Warn("Failed to save analysis result")
		}
	}
	
	return results, nil
}

// GetStatistics returns analysis statistics
func (ca *CrashAnalyzer) GetStatistics() map[string]any {
	stats := make(map[string]any)
	
	// Count crash types
	typeCounts := make(map[CrashType]int)
	severityCounts := make(map[SeverityLevel]int)
	exploitabilityCounts := make(map[ExploitabilityLevel]int)
	
	for _, sig := range ca.signatureCache {
		typeCounts[sig.Type]++
		severityCounts[sig.Severity]++
		exploitabilityCounts[sig.Exploitability]++
	}
	
	stats["total_signatures"] = len(ca.signatureCache)
	stats["crash_types"] = typeCounts
	stats["severity_distribution"] = severityCounts
	stats["exploitability_distribution"] = exploitabilityCounts
	
	return stats
}

// MinimizeCrashInput attempts to minimize crash input
func (ca *CrashAnalyzer) MinimizeCrashInput(crash *common.CrashResult, target string) ([]byte, error) {
	// This is a placeholder for crash minimization
	// In production, this would use tools like afl-tmin or custom minimizers
	
	ca.logger.WithFields(logrus.Fields{
		"crash_id":    crash.ID,
		"input_size":  len(crash.Input),
		"target":      target,
	}).Info("Would minimize crash input")
	
	// For now, just return the original input
	return crash.Input, nil
}

// CompareCrashes compares two crashes for similarity
func (ca *CrashAnalyzer) CompareCrashes(crash1, crash2 *common.CrashResult) (float64, error) {
	// Analyze both crashes
	result1, err := ca.AnalyzeCrash(crash1)
	if err != nil {
		return 0, err
	}
	
	result2, err := ca.AnalyzeCrash(crash2)
	if err != nil {
		return 0, err
	}
	
	// Calculate similarity score
	score := 0.0
	weights := 0.0
	
	// Compare crash types (highest weight)
	if result1.Signature.Type == result2.Signature.Type {
		score += 0.4
	}
	weights += 0.4
	
	// Compare stack traces
	if result1.StackTrace != nil && result2.StackTrace != nil {
		stackSimilarity := ca.compareStackTraces(result1.StackTrace, result2.StackTrace)
		score += stackSimilarity * 0.3
		weights += 0.3
	}
	
	// Compare signatures
	if ca.areSimilar(result1.Signature, result2.Signature) {
		score += 0.2
	}
	weights += 0.2
	
	// Compare severity
	if result1.Signature.Severity == result2.Signature.Severity {
		score += 0.1
	}
	weights += 0.1
	
	return score / weights, nil
}

// compareStackTraces calculates similarity between stack traces
func (ca *CrashAnalyzer) compareStackTraces(st1, st2 *StackTrace) float64 {
	if len(st1.Frames) == 0 || len(st2.Frames) == 0 {
		return 0
	}
	
	// Compare top frames
	matches := 0
	maxFrames := ca.config.MinStackFrames
	if len(st1.Frames) < maxFrames {
		maxFrames = len(st1.Frames)
	}
	if len(st2.Frames) < maxFrames {
		maxFrames = len(st2.Frames)
	}
	
	for i := 0; i < maxFrames; i++ {
		f1 := normalizeFrameFunction(st1.Frames[i].Function)
		f2 := normalizeFrameFunction(st2.Frames[i].Function)
		
		if f1 == f2 && f1 != "" && f1 != "??" {
			matches++
		}
	}
	
	return float64(matches) / float64(maxFrames)
}

// ExportResults exports analysis results in various formats
func (ca *CrashAnalyzer) ExportResults(results []*AnalysisResult, format string, output io.Writer) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		return encoder.Encode(results)
		
	case "csv":
		// CSV export would be implemented here
		return fmt.Errorf("CSV export not yet implemented")
		
	case "html":
		// HTML report generation would be implemented here
		return fmt.Errorf("HTML export not yet implemented")
		
	default:
		return fmt.Errorf("unsupported export format: %s", format)
	}
}