package types

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

// CrashSignature represents a unique identifier for crash deduplication
type CrashSignature struct {
	Hash          string   `json:"hash"`
	TopFrames     []string `json:"top_frames"`
	FunctionNames []string `json:"function_names"`
	LibraryNames  []string `json:"library_names"`
	SignatureType string   `json:"signature_type"`
	Confidence    float64  `json:"confidence"`
}

// Frame represents a single stack frame
type Frame struct {
	Address      string `json:"address"`
	Function     string `json:"function"`
	Library      string `json:"library"`
	SourceFile   string `json:"source_file,omitempty"`
	LineNumber   int    `json:"line_number,omitempty"`
	IsSymbolized bool   `json:"is_symbolized"`
}

// Regular expressions for parsing stack traces
var (
	// Generic frame pattern: #0 0x7fff12345678 in function_name at file.c:123
	genericFramePattern = regexp.MustCompile(`#\d+\s+(?:0x[0-9a-fA-F]+\s+)?(?:in\s+)?([^\s]+)(?:\s+at\s+([^:]+):(\d+))?`)

	// Address pattern: 0x7fff12345678
	addressPattern = regexp.MustCompile(`0x[0-9a-fA-F]+`)

	// Function name cleanup
	templatePattern = regexp.MustCompile(`<[^>]+>`)
	paramPattern    = regexp.MustCompile(`\([^)]*\)`)
)

// NewCrashSignature creates a new crash signature from a stack trace
func NewCrashSignature(stackTrace string) (*CrashSignature, error) {
	if stackTrace == "" {
		return nil, errors.New("stack trace cannot be empty")
	}

	frames := parseStackTrace(stackTrace)
	if len(frames) == 0 {
		return nil, errors.New("no valid frames found in stack trace")
	}

	sig := &CrashSignature{
		TopFrames:     extractTopFrames(frames, 5),
		FunctionNames: extractFunctionNames(frames),
		LibraryNames:  extractLibraryNames(frames),
		SignatureType: determineSignatureType(frames),
		Confidence:    calculateConfidence(frames),
	}

	// Generate hash from the signature components
	sig.Hash = generateSignatureHash(sig)

	return sig, nil
}

// parseStackTrace parses a stack trace into frames
func parseStackTrace(stackTrace string) []Frame {
	lines := strings.Split(stackTrace, "\n")
	frames := make([]Frame, 0)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		frame := parseFrame(line)
		if frame != nil {
			frames = append(frames, *frame)
		}
	}

	return frames
}

// parseFrame parses a single stack frame line
func parseFrame(line string) *Frame {
	// Try to match generic frame pattern
	matches := genericFramePattern.FindStringSubmatch(line)
	if len(matches) == 0 {
		return nil
	}

	frame := &Frame{
		IsSymbolized: true,
	}

	// Extract function name
	if len(matches) > 1 && matches[1] != "" {
		frame.Function = cleanFunctionName(matches[1])
		if strings.Contains(frame.Function, "0x") || frame.Function == "???" {
			frame.IsSymbolized = false
		}
	}

	// Extract source file
	if len(matches) > 2 && matches[2] != "" {
		frame.SourceFile = matches[2]
		// Extract library name from source file path
		parts := strings.Split(frame.SourceFile, "/")
		if len(parts) > 0 {
			for _, part := range parts {
				if strings.Contains(part, ".so") || strings.Contains(part, ".dll") || strings.Contains(part, ".dylib") {
					frame.Library = part
					break
				}
			}
		}
	}

	// Extract line number
	if len(matches) > 3 && matches[3] != "" {
		// Line number parsing handled as string to int conversion
		// In production, would use strconv.Atoi
		frame.LineNumber = 0 // Simplified for this example
	}

	// Extract address if present
	if addr := addressPattern.FindString(line); addr != "" {
		frame.Address = addr
	}

	return frame
}

// cleanFunctionName removes common decorations from function names
func cleanFunctionName(name string) string {
	// Remove template parameters
	name = templatePattern.ReplaceAllString(name, "")

	// Remove function parameters
	name = paramPattern.ReplaceAllString(name, "")

	// Remove common prefixes/suffixes
	name = strings.TrimPrefix(name, "_")
	name = strings.TrimSuffix(name, "_")

	return name
}

// extractTopFrames extracts the top N meaningful frames
func extractTopFrames(frames []Frame, n int) []string {
	topFrames := make([]string, 0, n)

	for _, frame := range frames {
		// Skip non-symbolized frames for signature
		if !frame.IsSymbolized || frame.Function == "" {
			continue
		}

		// Skip common runtime/system functions
		if isSystemFunction(frame.Function) {
			continue
		}

		topFrames = append(topFrames, frame.Function)

		if len(topFrames) >= n {
			break
		}
	}

	return topFrames
}

// extractFunctionNames extracts all unique function names
func extractFunctionNames(frames []Frame) []string {
	seen := make(map[string]bool)
	functions := make([]string, 0)

	for _, frame := range frames {
		if frame.Function != "" && !seen[frame.Function] && !isSystemFunction(frame.Function) {
			seen[frame.Function] = true
			functions = append(functions, frame.Function)
		}
	}

	return functions
}

// extractLibraryNames extracts all unique library names
func extractLibraryNames(frames []Frame) []string {
	seen := make(map[string]bool)
	libraries := make([]string, 0)

	for _, frame := range frames {
		if frame.Library != "" && !seen[frame.Library] {
			seen[frame.Library] = true
			libraries = append(libraries, frame.Library)
		}
	}

	return libraries
}

// isSystemFunction checks if a function is a common system/runtime function
func isSystemFunction(function string) bool {
	systemFunctions := []string{
		"malloc", "free", "calloc", "realloc",
		"memcpy", "memset", "memmove",
		"__libc_start_main", "_start",
		"raise", "abort", "__assert_fail",
		"pthread_", "dl_", "__cxa_",
	}

	functionLower := strings.ToLower(function)
	for _, sysFunc := range systemFunctions {
		if strings.HasPrefix(functionLower, sysFunc) {
			return true
		}
	}

	return false
}

// determineSignatureType determines the type of signature based on available information
func determineSignatureType(frames []Frame) string {
	symbolizedCount := 0
	for _, frame := range frames {
		if frame.IsSymbolized {
			symbolizedCount++
		}
	}

	symbolizationRatio := float64(symbolizedCount) / float64(len(frames))

	switch {
	case symbolizationRatio > 0.8:
		return "fully_symbolized"
	case symbolizationRatio > 0.3:
		return "partially_symbolized"
	default:
		return "address_based"
	}
}

// calculateConfidence calculates how confident we are in the signature uniqueness
func calculateConfidence(frames []Frame) float64 {
	if len(frames) == 0 {
		return 0.0
	}

	// Factors that increase confidence:
	// - More symbolized frames
	// - More unique functions
	// - Presence of source file information

	symbolizedCount := 0
	sourceInfoCount := 0
	uniqueFunctions := make(map[string]bool)

	for _, frame := range frames {
		if frame.IsSymbolized {
			symbolizedCount++
		}
		if frame.SourceFile != "" {
			sourceInfoCount++
		}
		if frame.Function != "" && !isSystemFunction(frame.Function) {
			uniqueFunctions[frame.Function] = true
		}
	}

	symbolizationScore := float64(symbolizedCount) / float64(len(frames))
	sourceInfoScore := float64(sourceInfoCount) / float64(len(frames))
	uniquenessScore := float64(len(uniqueFunctions)) / float64(len(frames))

	// Weighted average
	confidence := (symbolizationScore * 0.5) + (sourceInfoScore * 0.3) + (uniquenessScore * 0.2)

	// Ensure confidence is between 0 and 1
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// generateSignatureHash creates a hash from signature components
func generateSignatureHash(sig *CrashSignature) string {
	// Create a deterministic string from signature components
	components := make([]string, 0)

	// Add top frames
	components = append(components, sig.TopFrames...)

	// Add library names for additional context
	components = append(components, sig.LibraryNames...)

	// Join and hash
	combined := strings.Join(components, "|")
	hash := sha256.Sum256([]byte(combined))

	return hex.EncodeToString(hash[:])
}

// Equals checks if two signatures are equal
func (s *CrashSignature) Equals(other *CrashSignature) bool {
	if other == nil {
		return false
	}
	return s.Hash == other.Hash
}

// IsSimilar checks if two signatures are similar (fuzzy matching)
func (s *CrashSignature) IsSimilar(other *CrashSignature, threshold float64) bool {
	if other == nil {
		return false
	}

	// Quick exact match check
	if s.Equals(other) {
		return true
	}

	// Calculate similarity score based on common functions
	commonFunctions := 0
	for _, f1 := range s.FunctionNames {
		for _, f2 := range other.FunctionNames {
			if f1 == f2 {
				commonFunctions++
				break
			}
		}
	}

	// Calculate Jaccard similarity
	totalFunctions := len(s.FunctionNames) + len(other.FunctionNames) - commonFunctions
	if totalFunctions == 0 {
		return false
	}

	similarity := float64(commonFunctions) / float64(totalFunctions)
	return similarity >= threshold
}

// String returns a string representation of the signature
func (s *CrashSignature) String() string {
	if len(s.TopFrames) > 0 {
		return strings.Join(s.TopFrames[:min(3, len(s.TopFrames))], " -> ")
	}
	return s.Hash[:16] + "..."
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
