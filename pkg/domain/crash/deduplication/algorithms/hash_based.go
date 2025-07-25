package algorithms

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
)

// HashBased implements a hash-based deduplication algorithm
type HashBased struct {
	// Config for the algorithm
	config HashBasedConfig
}

// HashBasedConfig contains configuration for the hash-based algorithm
type HashBasedConfig struct {
	// UseInputHash determines if input hash should be considered
	UseInputHash bool

	// UseSignatureHash determines if signature hash should be considered
	UseSignatureHash bool

	// UseStackTrace determines if raw stack trace should be considered
	UseStackTrace bool

	// NormalizeStackTrace determines if stack traces should be normalized
	NormalizeStackTrace bool

	// IgnoreAddresses removes memory addresses from comparison
	IgnoreAddresses bool

	// IgnoreLineNumbers removes line numbers from comparison
	IgnoreLineNumbers bool

	// TopFramesCount number of top frames to consider for hashing
	TopFramesCount int
}

// DefaultHashBasedConfig returns default configuration
func DefaultHashBasedConfig() HashBasedConfig {
	return HashBasedConfig{
		UseInputHash:        false,
		UseSignatureHash:    true,
		UseStackTrace:       true,
		NormalizeStackTrace: true,
		IgnoreAddresses:     true,
		IgnoreLineNumbers:   true,
		TopFramesCount:      5,
	}
}

// NewHashBased creates a new hash-based deduplication algorithm
func NewHashBased(config HashBasedConfig) *HashBased {
	return &HashBased{
		config: config,
	}
}

// Name returns the algorithm name
func (h *HashBased) Name() string {
	return "hash_based"
}

// IsDuplicate checks if the new crash is a duplicate of an existing crash
func (h *HashBased) IsDuplicate(existing, new *types.Crash) bool {
	if existing == nil || new == nil {
		return false
	}

	// Quick check: if both have signatures, compare them first
	if h.config.UseSignatureHash && existing.Signature != nil && new.Signature != nil {
		if existing.Signature.Hash == new.Signature.Hash {
			return true
		}
	}

	// Compare input hashes if enabled
	if h.config.UseInputHash && existing.InputHash == new.InputHash {
		return true
	}

	// Compare normalized stack traces
	if h.config.UseStackTrace {
		existingHash := h.computeStackTraceHash(existing)
		newHash := h.computeStackTraceHash(new)

		if existingHash == newHash {
			return true
		}
	}

	// Advanced: Compare top frames if signatures exist
	if existing.Signature != nil && new.Signature != nil {
		return h.compareTopFrames(existing.Signature, new.Signature)
	}

	return false
}

// FindDuplicates finds all duplicates of the given crash in a collection
func (h *HashBased) FindDuplicates(crash *types.Crash, candidates []*types.Crash) []*types.Crash {
	if crash == nil || len(candidates) == 0 {
		return nil
	}

	duplicates := make([]*types.Crash, 0)

	for _, candidate := range candidates {
		if candidate.ID == crash.ID {
			continue // Skip self
		}

		if h.IsDuplicate(candidate, crash) {
			duplicates = append(duplicates, candidate)
		}
	}

	return duplicates
}

// CalculateSimilarity returns a similarity score between 0 and 1
func (h *HashBased) CalculateSimilarity(crash1, crash2 *types.Crash) float64 {
	if crash1 == nil || crash2 == nil {
		return 0.0
	}

	scores := make([]float64, 0, 4)
	weights := make([]float64, 0, 4)

	// Compare crash types
	if crash1.Type == crash2.Type {
		scores = append(scores, 1.0)
	} else {
		scores = append(scores, 0.0)
	}
	weights = append(weights, 0.2)

	// Compare signatures
	if crash1.Signature != nil && crash2.Signature != nil {
		sigScore := h.calculateSignatureSimilarity(crash1.Signature, crash2.Signature)
		scores = append(scores, sigScore)
		weights = append(weights, 0.4)
	}

	// Compare stack traces
	if h.config.UseStackTrace {
		stackScore := h.calculateStackTraceSimilarity(crash1.StackTrace, crash2.StackTrace)
		scores = append(scores, stackScore)
		weights = append(weights, 0.3)
	}

	// Compare severity
	if crash1.Severity == crash2.Severity {
		scores = append(scores, 1.0)
	} else {
		scores = append(scores, 0.5) // Partial match for different severities
	}
	weights = append(weights, 0.1)

	// Calculate weighted average
	var totalScore, totalWeight float64
	for i, score := range scores {
		totalScore += score * weights[i]
		totalWeight += weights[i]
	}

	if totalWeight == 0 {
		return 0.0
	}

	return totalScore / totalWeight
}

// GroupCrashes groups crashes by similarity
func (h *HashBased) GroupCrashes(crashes []*types.Crash, threshold float64) [][]*types.Crash {
	if len(crashes) == 0 {
		return nil
	}

	// Create a map to track which crashes belong to which group
	crashToGroup := make(map[string]int)
	groups := make([][]*types.Crash, 0)

	for i, crash1 := range crashes {
		// Check if this crash is already in a group
		if _, exists := crashToGroup[crash1.ID]; exists {
			continue
		}

		// Create a new group with this crash
		group := []*types.Crash{crash1}
		groupIndex := len(groups)
		crashToGroup[crash1.ID] = groupIndex

		// Find all similar crashes
		for j := i + 1; j < len(crashes); j++ {
			crash2 := crashes[j]

			// Skip if already in a group
			if _, exists := crashToGroup[crash2.ID]; exists {
				continue
			}

			// Check similarity with all crashes in the current group
			isSimilar := false
			for _, groupCrash := range group {
				similarity := h.CalculateSimilarity(groupCrash, crash2)
				if similarity >= threshold {
					isSimilar = true
					break
				}
			}

			if isSimilar {
				group = append(group, crash2)
				crashToGroup[crash2.ID] = groupIndex
			}
		}

		groups = append(groups, group)
	}

	// Sort groups by size (largest first)
	sort.Slice(groups, func(i, j int) bool {
		return len(groups[i]) > len(groups[j])
	})

	return groups
}

// Private helper methods

func (h *HashBased) computeStackTraceHash(crash *types.Crash) string {
	stackTrace := crash.StackTrace

	if h.config.NormalizeStackTrace {
		stackTrace = h.normalizeStackTrace(stackTrace)
	}

	// If we have a signature with top frames, use those
	if crash.Signature != nil && len(crash.Signature.TopFrames) > 0 {
		frames := crash.Signature.TopFrames
		if h.config.TopFramesCount > 0 && len(frames) > h.config.TopFramesCount {
			frames = frames[:h.config.TopFramesCount]
		}
		stackTrace = strings.Join(frames, "\n")
	}

	hash := sha256.Sum256([]byte(stackTrace))
	return hex.EncodeToString(hash[:])
}

func (h *HashBased) normalizeStackTrace(stackTrace string) string {
	lines := strings.Split(stackTrace, "\n")
	normalized := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Remove memory addresses if configured
		if h.config.IgnoreAddresses {
			line = removeAddresses(line)
		}

		// Remove line numbers if configured
		if h.config.IgnoreLineNumbers {
			line = removeLineNumbers(line)
		}

		// Remove common variable parts
		line = removeVariableParts(line)

		normalized = append(normalized, line)
	}

	return strings.Join(normalized, "\n")
}

func (h *HashBased) compareTopFrames(sig1, sig2 *types.CrashSignature) bool {
	if len(sig1.TopFrames) == 0 || len(sig2.TopFrames) == 0 {
		return false
	}

	count := h.config.TopFramesCount
	if count <= 0 || count > len(sig1.TopFrames) {
		count = len(sig1.TopFrames)
	}
	if count > len(sig2.TopFrames) {
		count = len(sig2.TopFrames)
	}

	// Compare top N frames
	for i := 0; i < count; i++ {
		frame1 := sig1.TopFrames[i]
		frame2 := sig2.TopFrames[i]

		if h.config.NormalizeStackTrace {
			frame1 = normalizeFrame(frame1)
			frame2 = normalizeFrame(frame2)
		}

		if frame1 != frame2 {
			return false
		}
	}

	return true
}

func (h *HashBased) calculateSignatureSimilarity(sig1, sig2 *types.CrashSignature) float64 {
	// Quick exact match
	if sig1.Hash == sig2.Hash {
		return 1.0
	}

	// Use the built-in similarity method if available
	return calculateJaccardSimilarity(sig1.FunctionNames, sig2.FunctionNames)
}

func (h *HashBased) calculateStackTraceSimilarity(stack1, stack2 string) float64 {
	if stack1 == stack2 {
		return 1.0
	}

	// Normalize stack traces
	if h.config.NormalizeStackTrace {
		stack1 = h.normalizeStackTrace(stack1)
		stack2 = h.normalizeStackTrace(stack2)
	}

	// Split into lines
	lines1 := strings.Split(stack1, "\n")
	lines2 := strings.Split(stack2, "\n")

	// Calculate line-based similarity
	return calculateLinesSimilarity(lines1, lines2)
}

// Helper functions

func removeAddresses(line string) string {
	// Remove hex addresses (0x...)
	addressRegex := `0x[0-9a-fA-F]+`
	return strings.TrimSpace(replacePattern(line, addressRegex, ""))
}

func removeLineNumbers(line string) string {
	// Remove line numbers (e.g., :123)
	lineNumRegex := `:\d+`
	return strings.TrimSpace(replacePattern(line, lineNumRegex, ""))
}

func removeVariableParts(line string) string {
	// Remove thread IDs
	line = replacePattern(line, `Thread \d+`, "Thread")

	// Remove process IDs
	line = replacePattern(line, `\[pid \d+\]`, "[pid]")

	// Remove timestamps
	line = replacePattern(line, `\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}`, "")

	return strings.TrimSpace(line)
}

func normalizeFrame(frame string) string {
	frame = removeAddresses(frame)
	frame = removeLineNumbers(frame)
	frame = removeVariableParts(frame)
	return strings.TrimSpace(frame)
}

func replacePattern(text, pattern, replacement string) string {
	// In a real implementation, this would use regexp.ReplaceAllString
	// For simplicity, using strings.Replace for common patterns
	if pattern == `0x[0-9a-fA-F]+` {
		// Simple implementation: remove common address patterns
		for _, prefix := range []string{"0x", "0X"} {
			idx := strings.Index(text, prefix)
			if idx >= 0 {
				end := idx + 2
				for end < len(text) && isHexDigit(text[end]) {
					end++
				}
				text = text[:idx] + replacement + text[end:]
			}
		}
	}
	return text
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func calculateJaccardSimilarity(set1, set2 []string) float64 {
	if len(set1) == 0 && len(set2) == 0 {
		return 1.0
	}
	if len(set1) == 0 || len(set2) == 0 {
		return 0.0
	}

	// Create sets
	set1Map := make(map[string]bool)
	for _, s := range set1 {
		set1Map[s] = true
	}

	set2Map := make(map[string]bool)
	for _, s := range set2 {
		set2Map[s] = true
	}

	// Calculate intersection
	intersection := 0
	for s := range set1Map {
		if set2Map[s] {
			intersection++
		}
	}

	// Calculate union
	union := len(set1Map)
	for s := range set2Map {
		if !set1Map[s] {
			union++
		}
	}

	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

func calculateLinesSimilarity(lines1, lines2 []string) float64 {
	if len(lines1) == 0 && len(lines2) == 0 {
		return 1.0
	}
	if len(lines1) == 0 || len(lines2) == 0 {
		return 0.0
	}

	// Use longest common subsequence (LCS) for similarity
	lcs := longestCommonSubsequence(lines1, lines2)
	maxLen := len(lines1)
	if len(lines2) > maxLen {
		maxLen = len(lines2)
	}

	return float64(lcs) / float64(maxLen)
}

func longestCommonSubsequence(lines1, lines2 []string) int {
	m, n := len(lines1), len(lines2)
	if m == 0 || n == 0 {
		return 0
	}

	// Create DP table
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	// Fill DP table
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if lines1[i-1] == lines2[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				if dp[i-1][j] > dp[i][j-1] {
					dp[i][j] = dp[i-1][j]
				} else {
					dp[i][j] = dp[i][j-1]
				}
			}
		}
	}

	return dp[m][n]
}
