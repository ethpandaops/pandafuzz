package algorithms

import (
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
)

// FuzzyMatching implements a fuzzy matching deduplication algorithm
// that uses various string similarity metrics
type FuzzyMatching struct {
	config FuzzyMatchingConfig
}

// FuzzyMatchingConfig contains configuration for the fuzzy matching algorithm
type FuzzyMatchingConfig struct {
	// MinSimilarity is the minimum similarity score to consider a match
	MinSimilarity float64

	// UseLevenshtein enables Levenshtein distance calculation
	UseLevenshtein bool

	// UseJaroWinkler enables Jaro-Winkler distance calculation
	UseJaroWinkler bool

	// UseLCS enables Longest Common Subsequence calculation
	UseLCS bool

	// WeightStackTrace weight for stack trace similarity
	WeightStackTrace float64

	// WeightFunctions weight for function names similarity
	WeightFunctions float64

	// WeightLibraries weight for library names similarity
	WeightLibraries float64

	// MaxEditDistance maximum edit distance for Levenshtein
	MaxEditDistance int
}

// DefaultFuzzyMatchingConfig returns default configuration
func DefaultFuzzyMatchingConfig() FuzzyMatchingConfig {
	return FuzzyMatchingConfig{
		MinSimilarity:    0.80,
		UseLevenshtein:   true,
		UseJaroWinkler:   true,
		UseLCS:           true,
		WeightStackTrace: 0.5,
		WeightFunctions:  0.3,
		WeightLibraries:  0.2,
		MaxEditDistance:  10,
	}
}

// NewFuzzyMatching creates a new fuzzy matching deduplication algorithm
func NewFuzzyMatching(config FuzzyMatchingConfig) *FuzzyMatching {
	return &FuzzyMatching{
		config: config,
	}
}

// Name returns the algorithm name
func (f *FuzzyMatching) Name() string {
	return "fuzzy_matching"
}

// IsDuplicate checks if the new crash is a duplicate of an existing crash
func (f *FuzzyMatching) IsDuplicate(existing, new *types.Crash) bool {
	similarity := f.CalculateSimilarity(existing, new)
	return similarity >= f.config.MinSimilarity
}

// FindDuplicates finds all duplicates of the given crash in a collection
func (f *FuzzyMatching) FindDuplicates(crash *types.Crash, candidates []*types.Crash) []*types.Crash {
	if crash == nil || len(candidates) == 0 {
		return nil
	}

	duplicates := make([]*types.Crash, 0)

	for _, candidate := range candidates {
		if candidate.ID == crash.ID {
			continue
		}

		if f.IsDuplicate(candidate, crash) {
			duplicates = append(duplicates, candidate)
		}
	}

	return duplicates
}

// CalculateSimilarity returns a similarity score between 0 and 1
func (f *FuzzyMatching) CalculateSimilarity(crash1, crash2 *types.Crash) float64 {
	if crash1 == nil || crash2 == nil {
		return 0.0
	}

	var totalScore float64
	var totalWeight float64

	// Calculate stack trace similarity
	if f.config.WeightStackTrace > 0 {
		stackScore := f.calculateTextSimilarity(crash1.StackTrace, crash2.StackTrace)
		totalScore += stackScore * f.config.WeightStackTrace
		totalWeight += f.config.WeightStackTrace
	}

	// Calculate function names similarity
	if f.config.WeightFunctions > 0 && crash1.Signature != nil && crash2.Signature != nil {
		funcScore := f.calculateSetSimilarity(crash1.Signature.FunctionNames, crash2.Signature.FunctionNames)
		totalScore += funcScore * f.config.WeightFunctions
		totalWeight += f.config.WeightFunctions
	}

	// Calculate library names similarity
	if f.config.WeightLibraries > 0 && crash1.Signature != nil && crash2.Signature != nil {
		libScore := f.calculateSetSimilarity(crash1.Signature.LibraryNames, crash2.Signature.LibraryNames)
		totalScore += libScore * f.config.WeightLibraries
		totalWeight += f.config.WeightLibraries
	}

	if totalWeight == 0 {
		return 0.0
	}

	return totalScore / totalWeight
}

// GroupCrashes groups crashes by similarity using fuzzy matching
func (f *FuzzyMatching) GroupCrashes(crashes []*types.Crash, threshold float64) [][]*types.Crash {
	if len(crashes) == 0 {
		return nil
	}

	// Use Union-Find data structure for efficient grouping
	uf := newUnionFind(len(crashes))
	crashIndex := make(map[string]int)

	// Create index mapping
	for i, crash := range crashes {
		crashIndex[crash.ID] = i
	}

	// Find all pairs with similarity above threshold
	for i := 0; i < len(crashes); i++ {
		for j := i + 1; j < len(crashes); j++ {
			similarity := f.CalculateSimilarity(crashes[i], crashes[j])
			if similarity >= threshold {
				uf.union(i, j)
			}
		}
	}

	// Extract groups
	groupMap := make(map[int][]*types.Crash)
	for i, crash := range crashes {
		root := uf.find(i)
		groupMap[root] = append(groupMap[root], crash)
	}

	// Convert to slice
	groups := make([][]*types.Crash, 0, len(groupMap))
	for _, group := range groupMap {
		groups = append(groups, group)
	}

	return groups
}

// Private helper methods

func (f *FuzzyMatching) calculateTextSimilarity(text1, text2 string) float64 {
	if text1 == text2 {
		return 1.0
	}
	if text1 == "" || text2 == "" {
		return 0.0
	}

	scores := make([]float64, 0, 3)

	// Levenshtein distance
	if f.config.UseLevenshtein {
		score := f.levenshteinSimilarity(text1, text2)
		scores = append(scores, score)
	}

	// Jaro-Winkler distance
	if f.config.UseJaroWinkler {
		score := f.jaroWinklerSimilarity(text1, text2)
		scores = append(scores, score)
	}

	// Longest Common Subsequence
	if f.config.UseLCS {
		score := f.lcsSimilarity(text1, text2)
		scores = append(scores, score)
	}

	if len(scores) == 0 {
		return 0.0
	}

	// Return average of all enabled metrics
	sum := 0.0
	for _, score := range scores {
		sum += score
	}
	return sum / float64(len(scores))
}

func (f *FuzzyMatching) calculateSetSimilarity(set1, set2 []string) float64 {
	if len(set1) == 0 && len(set2) == 0 {
		return 1.0
	}
	if len(set1) == 0 || len(set2) == 0 {
		return 0.0
	}

	// Use fuzzy matching for set elements
	matched := 0
	for _, s1 := range set1 {
		for _, s2 := range set2 {
			similarity := f.calculateTextSimilarity(s1, s2)
			if similarity >= 0.8 { // High threshold for set elements
				matched++
				break
			}
		}
	}

	// Calculate Jaccard-like coefficient
	union := len(set1) + len(set2) - matched
	if union == 0 {
		return 0.0
	}

	return float64(matched) / float64(union)
}

func (f *FuzzyMatching) levenshteinSimilarity(s1, s2 string) float64 {
	distance := levenshteinDistance(s1, s2)
	maxLen := len(s1)
	if len(s2) > maxLen {
		maxLen = len(s2)
	}

	if maxLen == 0 {
		return 1.0
	}

	// Normalize to [0, 1] range
	similarity := 1.0 - float64(distance)/float64(maxLen)
	if similarity < 0 {
		similarity = 0
	}

	return similarity
}

func (f *FuzzyMatching) jaroWinklerSimilarity(s1, s2 string) float64 {
	return jaroWinklerDistance(s1, s2)
}

func (f *FuzzyMatching) lcsSimilarity(s1, s2 string) float64 {
	lcs := longestCommonSubsequenceString(s1, s2)
	maxLen := len(s1)
	if len(s2) > maxLen {
		maxLen = len(s2)
	}

	if maxLen == 0 {
		return 1.0
	}

	return float64(lcs) / float64(maxLen)
}

// Utility functions

func levenshteinDistance(s1, s2 string) int {
	if s1 == s2 {
		return 0
	}

	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Create distance matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
	}

	// Initialize first column and row
	for i := 0; i <= len(s1); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}

			matrix[i][j] = min3(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}

func jaroWinklerDistance(s1, s2 string) float64 {
	jaro := jaroDistance(s1, s2)

	// Find common prefix length (up to 4 characters)
	prefixLen := 0
	for i := 0; i < min(len(s1), min(len(s2), 4)); i++ {
		if s1[i] == s2[i] {
			prefixLen++
		} else {
			break
		}
	}

	// Jaro-Winkler formula
	return jaro + float64(prefixLen)*0.1*(1.0-jaro)
}

func jaroDistance(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	len1, len2 := len(s1), len(s2)
	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	// Calculate the match window
	matchWindow := max(len1, len2)/2 - 1
	if matchWindow < 0 {
		matchWindow = 0
	}

	// Initialize the matched arrays
	s1Matches := make([]bool, len1)
	s2Matches := make([]bool, len2)

	matches := 0
	transpositions := 0

	// Identify matches
	for i := 0; i < len1; i++ {
		start := max(0, i-matchWindow)
		end := min(i+matchWindow+1, len2)

		for j := start; j < end; j++ {
			if s2Matches[j] || s1[i] != s2[j] {
				continue
			}
			s1Matches[i] = true
			s2Matches[j] = true
			matches++
			break
		}
	}

	if matches == 0 {
		return 0.0
	}

	// Count transpositions
	k := 0
	for i := 0; i < len1; i++ {
		if !s1Matches[i] {
			continue
		}
		for !s2Matches[k] {
			k++
		}
		if s1[i] != s2[k] {
			transpositions++
		}
		k++
	}

	// Calculate Jaro distance
	return (float64(matches)/float64(len1) +
		float64(matches)/float64(len2) +
		float64(matches-transpositions/2)/float64(matches)) / 3.0
}

func longestCommonSubsequenceString(s1, s2 string) int {
	if len(s1) == 0 || len(s2) == 0 {
		return 0
	}

	// Create DP table
	dp := make([][]int, len(s1)+1)
	for i := range dp {
		dp[i] = make([]int, len(s2)+1)
	}

	// Fill DP table
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			if s1[i-1] == s2[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				dp[i][j] = max(dp[i-1][j], dp[i][j-1])
			}
		}
	}

	return dp[len(s1)][len(s2)]
}

// Union-Find data structure for efficient grouping
type unionFind struct {
	parent []int
	rank   []int
}

func newUnionFind(n int) *unionFind {
	parent := make([]int, n)
	rank := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	return &unionFind{parent: parent, rank: rank}
}

func (uf *unionFind) find(x int) int {
	if uf.parent[x] != x {
		uf.parent[x] = uf.find(uf.parent[x]) // Path compression
	}
	return uf.parent[x]
}

func (uf *unionFind) union(x, y int) {
	rootX, rootY := uf.find(x), uf.find(y)
	if rootX == rootY {
		return
	}

	// Union by rank
	if uf.rank[rootX] < uf.rank[rootY] {
		uf.parent[rootX] = rootY
	} else if uf.rank[rootX] > uf.rank[rootY] {
		uf.parent[rootY] = rootX
	} else {
		uf.parent[rootY] = rootX
		uf.rank[rootX]++
	}
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min3(a, b, c int) int {
	return min(min(a, b), c)
}
