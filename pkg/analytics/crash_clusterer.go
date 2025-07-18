package analytics

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// CrashClusterer provides crash clustering and analysis capabilities
type CrashClusterer interface {
	// ClusterCrashes groups similar crashes together based on stack traces
	ClusterCrashes(ctx context.Context, campaignID string, options ClusterOptions) ([]*CrashCluster, error)

	// CalculateSimilarity computes similarity between two crash stack traces
	CalculateSimilarity(trace1, trace2 string) float64

	// NormalizeStackTrace normalizes a stack trace for comparison
	NormalizeStackTrace(trace string) string

	// Start starts the crash clusterer service
	Start(ctx context.Context) error

	// Stop stops the crash clusterer service
	Stop() error
}

// ClusterOptions configures crash clustering behavior
type ClusterOptions struct {
	SimilarityThreshold float64       // Minimum similarity to group crashes (0-1)
	TimeWindow          time.Duration // Time window for crash analysis
	MinClusterSize      int           // Minimum crashes to form a cluster
	MaxClusters         int           // Maximum number of clusters to return
	IncludeFlaky        bool          // Include flaky crashes in clustering
	GroupingStrategy    string        // "stack_based", "signal_based", "hybrid"
}

// CrashCluster represents a group of similar crashes
type CrashCluster struct {
	ID              string                       `json:"id"`
	CampaignID      string                       `json:"campaign_id"`
	SignatureHash   string                       `json:"signature_hash"`
	CrashType       string                       `json:"crash_type"`
	Severity        CrashSeverity                `json:"severity"`
	Count           int                          `json:"count"`
	FirstSeen       time.Time                    `json:"first_seen"`
	LastSeen        time.Time                    `json:"last_seen"`
	StackSignature  string                       `json:"stack_signature"`
	RootCause       *RootCauseAnalysis           `json:"root_cause"`
	Members         []string                     `json:"member_crash_ids"`
	Representative  *common.CrashResult          `json:"representative_crash"`
	CommonFrames    []common.StackFrame          `json:"common_frames"`
	Reproducibility common.ReproducibilityStatus `json:"reproducibility"`
	Impact          CrashImpact                  `json:"impact"`
}

// CrashSeverity represents the severity of a crash cluster
type CrashSeverity string

const (
	SeverityCritical CrashSeverity = "critical"
	SeverityHigh     CrashSeverity = "high"
	SeverityMedium   CrashSeverity = "medium"
	SeverityLow      CrashSeverity = "low"
)

// RootCauseAnalysis provides analysis of crash root cause
type RootCauseAnalysis struct {
	Function        string   `json:"function"`
	File            string   `json:"file"`
	Line            int      `json:"line"`
	CauseType       string   `json:"cause_type"` // "null_deref", "buffer_overflow", "use_after_free", etc.
	Confidence      float64  `json:"confidence"` // 0-1
	PossibleCauses  []string `json:"possible_causes"`
	SuggestedFixes  []string `json:"suggested_fixes"`
	RelatedClusters []string `json:"related_cluster_ids"`
}

// CrashImpact quantifies the impact of a crash cluster
type CrashImpact struct {
	FrequencyScore       float64 `json:"frequency_score"`       // How often it occurs
	RecencyScore         float64 `json:"recency_score"`         // How recently it occurred
	DistributionScore    float64 `json:"distribution_score"`    // How widely distributed across bots
	ReproducibilityScore float64 `json:"reproducibility_score"` // How consistently reproducible
	OverallScore         float64 `json:"overall_score"`         // Combined impact score
}

// crashClusterer implementation
type crashClusterer struct {
	storage       common.Storage
	logger        logrus.FieldLogger
	framePatterns map[string]*regexp.Regexp
}

// NewCrashClusterer creates a new crash clusterer
func NewCrashClusterer(storage common.Storage, logger *logrus.Logger) CrashClusterer {
	fieldLogger := logger.WithField("component", "crash_clusterer")

	// Pre-compile common frame patterns for normalization
	framePatterns := map[string]*regexp.Regexp{
		"address":    regexp.MustCompile(`0x[0-9a-fA-F]+`),
		"offset":     regexp.MustCompile(`\+0x[0-9a-fA-F]+`),
		"numbers":    regexp.MustCompile(`\b\d+\b`),
		"thread":     regexp.MustCompile(`Thread \d+`),
		"pid":        regexp.MustCompile(`\[pid \d+\]`),
		"whitespace": regexp.MustCompile(`\s+`),
	}

	return &crashClusterer{
		storage:       storage,
		logger:        fieldLogger,
		framePatterns: framePatterns,
	}
}

// Start starts the crash clusterer service
func (cc *crashClusterer) Start(ctx context.Context) error {
	cc.logger.Info("Starting crash clusterer")
	return nil
}

// Stop stops the crash clusterer service
func (cc *crashClusterer) Stop() error {
	cc.logger.Info("Stopping crash clusterer")
	return nil
}

// ClusterCrashes groups similar crashes together
func (cc *crashClusterer) ClusterCrashes(ctx context.Context, campaignID string, options ClusterOptions) ([]*CrashCluster, error) {
	cc.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"options":     options,
	}).Debug("Clustering crashes")

	// Set defaults
	if options.SimilarityThreshold == 0 {
		options.SimilarityThreshold = 0.8
	}
	if options.MinClusterSize == 0 {
		options.MinClusterSize = 2
	}
	if options.GroupingStrategy == "" {
		options.GroupingStrategy = "hybrid"
	}

	// Get crashes for the campaign
	crashes, err := cc.getCampaignCrashes(ctx, campaignID, options.TimeWindow)
	if err != nil {
		return nil, fmt.Errorf("failed to get campaign crashes: %w", err)
	}

	if len(crashes) == 0 {
		return []*CrashCluster{}, nil
	}

	// Normalize stack traces
	normalizedCrashes := cc.normalizeCrashes(crashes)

	// Perform clustering based on strategy
	var clusters []*CrashCluster
	switch options.GroupingStrategy {
	case "stack_based":
		clusters = cc.clusterByStackTrace(normalizedCrashes, options)
	case "signal_based":
		clusters = cc.clusterBySignal(normalizedCrashes, options)
	case "hybrid":
		clusters = cc.hybridClustering(normalizedCrashes, options)
	default:
		return nil, fmt.Errorf("unknown grouping strategy: %s", options.GroupingStrategy)
	}

	// Filter clusters by size
	filteredClusters := cc.filterClustersBySize(clusters, options.MinClusterSize)

	// Analyze root causes
	for _, cluster := range filteredClusters {
		cluster.RootCause = cc.analyzeRootCause(cluster)
		cluster.Impact = cc.calculateImpact(cluster)
		cluster.Severity = cc.calculateSeverity(cluster)
	}

	// Sort by impact score
	sort.Slice(filteredClusters, func(i, j int) bool {
		return filteredClusters[i].Impact.OverallScore > filteredClusters[j].Impact.OverallScore
	})

	// Limit to max clusters
	if options.MaxClusters > 0 && len(filteredClusters) > options.MaxClusters {
		filteredClusters = filteredClusters[:options.MaxClusters]
	}

	return filteredClusters, nil
}

// CalculateSimilarity computes similarity between two crash stack traces
func (cc *crashClusterer) CalculateSimilarity(trace1, trace2 string) float64 {
	// Normalize traces first
	norm1 := cc.NormalizeStackTrace(trace1)
	norm2 := cc.NormalizeStackTrace(trace2)

	// Extract frames
	frames1 := cc.extractFrames(norm1)
	frames2 := cc.extractFrames(norm2)

	if len(frames1) == 0 || len(frames2) == 0 {
		return 0.0
	}

	// Calculate frame-level similarity using weighted approach
	// Top frames are more important for crash similarity
	similarity := cc.calculateWeightedFrameSimilarity(frames1, frames2)

	// Apply signal-based adjustment if signals are present
	signal1 := cc.extractSignal(trace1)
	signal2 := cc.extractSignal(trace2)
	if signal1 != "" && signal2 != "" {
		if signal1 == signal2 {
			similarity = similarity*0.8 + 0.2 // Boost similarity for same signal
		} else {
			similarity = similarity * 0.8 // Reduce similarity for different signals
		}
	}

	return similarity
}

// NormalizeStackTrace normalizes a stack trace for comparison
func (cc *crashClusterer) NormalizeStackTrace(trace string) string {
	normalized := trace

	// Remove memory addresses
	normalized = cc.framePatterns["address"].ReplaceAllString(normalized, "ADDR")

	// Remove offsets
	normalized = cc.framePatterns["offset"].ReplaceAllString(normalized, "+OFFSET")

	// Remove line numbers but keep structure
	normalized = cc.framePatterns["numbers"].ReplaceAllString(normalized, "N")

	// Remove thread IDs
	normalized = cc.framePatterns["thread"].ReplaceAllString(normalized, "Thread N")

	// Remove process IDs
	normalized = cc.framePatterns["pid"].ReplaceAllString(normalized, "[pid N]")

	// Normalize whitespace
	normalized = cc.framePatterns["whitespace"].ReplaceAllString(normalized, " ")

	// Trim
	normalized = strings.TrimSpace(normalized)

	return normalized
}

// Helper methods

func (cc *crashClusterer) getCampaignCrashes(ctx context.Context, campaignID string, window time.Duration) ([]*common.CrashResult, error) {
	// Get all crashes for the campaign
	crashes, err := cc.storage.GetCrashesByCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	// Filter by time window if specified
	if window > 0 {
		cutoff := time.Now().Add(-window)
		filtered := make([]*common.CrashResult, 0)
		for _, crash := range crashes {
			if crash.Timestamp.After(cutoff) {
				filtered = append(filtered, crash)
			}
		}
		crashes = filtered
	}

	return crashes, nil
}

func (cc *crashClusterer) normalizeCrashes(crashes []*common.CrashResult) []*normalizedCrash {
	normalized := make([]*normalizedCrash, len(crashes))
	for i, crash := range crashes {
		normalized[i] = &normalizedCrash{
			crash:           crash,
			normalizedTrace: cc.NormalizeStackTrace(crash.StackTrace),
			frames:          cc.extractFrames(crash.StackTrace),
			signal:          cc.extractSignal(crash.StackTrace),
		}
	}
	return normalized
}

type normalizedCrash struct {
	crash           *common.CrashResult
	normalizedTrace string
	frames          []string
	signal          string
}

func (cc *crashClusterer) clusterByStackTrace(crashes []*normalizedCrash, options ClusterOptions) []*CrashCluster {
	clusters := make([]*CrashCluster, 0)
	clustered := make(map[int]bool)

	for i, crash1 := range crashes {
		if clustered[i] {
			continue
		}

		cluster := &CrashCluster{
			ID:             fmt.Sprintf("cluster_%s_%d", crash1.crash.CampaignID, i),
			CampaignID:     crash1.crash.CampaignID,
			CrashType:      crash1.crash.Type,
			Count:          1,
			FirstSeen:      crash1.crash.Timestamp,
			LastSeen:       crash1.crash.Timestamp,
			Members:        []string{crash1.crash.ID},
			Representative: crash1.crash,
			CommonFrames:   cc.parseStackFrames(crash1.frames),
		}

		clustered[i] = true

		// Find similar crashes
		for j := i + 1; j < len(crashes); j++ {
			if clustered[j] {
				continue
			}

			similarity := cc.CalculateSimilarity(crash1.crash.StackTrace, crashes[j].crash.StackTrace)
			if similarity >= options.SimilarityThreshold {
				cluster.Count++
				cluster.Members = append(cluster.Members, crashes[j].crash.ID)
				if crashes[j].crash.Timestamp.Before(cluster.FirstSeen) {
					cluster.FirstSeen = crashes[j].crash.Timestamp
				}
				if crashes[j].crash.Timestamp.After(cluster.LastSeen) {
					cluster.LastSeen = crashes[j].crash.Timestamp
				}
				clustered[j] = true
			}
		}

		// Generate signature for the cluster
		cluster.SignatureHash = cc.generateClusterSignature(cluster)
		cluster.StackSignature = cc.generateStackSignature(cluster.CommonFrames)

		clusters = append(clusters, cluster)
	}

	return clusters
}

func (cc *crashClusterer) clusterBySignal(crashes []*normalizedCrash, options ClusterOptions) []*CrashCluster {
	// Group by signal first, then apply stack-based clustering within each group
	signalGroups := make(map[string][]*normalizedCrash)
	for _, crash := range crashes {
		signal := crash.signal
		if signal == "" {
			signal = fmt.Sprintf("exit_%d", crash.crash.ExitCode)
		}
		signalGroups[signal] = append(signalGroups[signal], crash)
	}

	clusters := make([]*CrashCluster, 0)
	for signal, group := range signalGroups {
		// Apply stack-based clustering within signal group
		signalClusters := cc.clusterByStackTrace(group, options)
		for _, cluster := range signalClusters {
			cluster.CrashType = signal
			clusters = append(clusters, cluster)
		}
	}

	return clusters
}

func (cc *crashClusterer) hybridClustering(crashes []*normalizedCrash, options ClusterOptions) []*CrashCluster {
	// First cluster by signal to create initial groups
	signalClusters := cc.clusterBySignal(crashes, options)

	// Then merge clusters across signals if they have very high stack similarity
	mergedClusters := cc.mergeSimilarClusters(signalClusters, options.SimilarityThreshold*1.1)

	return mergedClusters
}

func (cc *crashClusterer) mergeSimilarClusters(clusters []*CrashCluster, threshold float64) []*CrashCluster {
	merged := make([]*CrashCluster, 0)
	used := make(map[int]bool)

	for i, cluster1 := range clusters {
		if used[i] {
			continue
		}

		mergedCluster := cluster1
		used[i] = true

		for j := i + 1; j < len(clusters); j++ {
			if used[j] {
				continue
			}

			// Compare representative crashes
			similarity := cc.CalculateSimilarity(
				cluster1.Representative.StackTrace,
				clusters[j].Representative.StackTrace,
			)

			if similarity >= threshold {
				// Merge clusters
				mergedCluster.Count += clusters[j].Count
				mergedCluster.Members = append(mergedCluster.Members, clusters[j].Members...)
				if clusters[j].FirstSeen.Before(mergedCluster.FirstSeen) {
					mergedCluster.FirstSeen = clusters[j].FirstSeen
				}
				if clusters[j].LastSeen.After(mergedCluster.LastSeen) {
					mergedCluster.LastSeen = clusters[j].LastSeen
				}
				used[j] = true
			}
		}

		merged = append(merged, mergedCluster)
	}

	return merged
}

func (cc *crashClusterer) filterClustersBySize(clusters []*CrashCluster, minSize int) []*CrashCluster {
	filtered := make([]*CrashCluster, 0)
	for _, cluster := range clusters {
		if cluster.Count >= minSize {
			filtered = append(filtered, cluster)
		}
	}
	return filtered
}

func (cc *crashClusterer) analyzeRootCause(cluster *CrashCluster) *RootCauseAnalysis {
	if len(cluster.CommonFrames) == 0 {
		return nil
	}

	// Get the topmost common frame as likely root cause
	topFrame := cluster.CommonFrames[0]

	analysis := &RootCauseAnalysis{
		Function:   topFrame.Function,
		File:       topFrame.File,
		Line:       topFrame.Line,
		Confidence: 0.8, // Base confidence
	}

	// Analyze crash type based on patterns
	stackSig := cluster.StackSignature
	if strings.Contains(stackSig, "null") || strings.Contains(stackSig, "0x0") {
		analysis.CauseType = "null_dereference"
		analysis.PossibleCauses = []string{
			"Null pointer dereference",
			"Uninitialized pointer access",
			"Use after free with cleared memory",
		}
		analysis.SuggestedFixes = []string{
			"Add null pointer checks",
			"Initialize pointers before use",
			"Validate input parameters",
		}
	} else if strings.Contains(stackSig, "overflow") || strings.Contains(stackSig, "stack smashing") {
		analysis.CauseType = "buffer_overflow"
		analysis.PossibleCauses = []string{
			"Buffer overflow",
			"Stack overflow",
			"Heap overflow",
		}
		analysis.SuggestedFixes = []string{
			"Use bounds checking",
			"Validate input sizes",
			"Use safe string functions",
		}
	} else if strings.Contains(stackSig, "free") || strings.Contains(stackSig, "malloc") {
		analysis.CauseType = "memory_corruption"
		analysis.PossibleCauses = []string{
			"Double free",
			"Use after free",
			"Heap corruption",
		}
		analysis.SuggestedFixes = []string{
			"Implement proper memory management",
			"Use smart pointers or RAII",
			"Add memory debugging tools",
		}
	} else if strings.Contains(stackSig, "assert") {
		analysis.CauseType = "assertion_failure"
		analysis.PossibleCauses = []string{
			"Failed assertion",
			"Invalid program state",
			"Violated invariant",
		}
		analysis.SuggestedFixes = []string{
			"Review assertion conditions",
			"Add proper error handling",
			"Validate inputs before assertions",
		}
	} else {
		analysis.CauseType = "unknown"
		analysis.Confidence = 0.5
	}

	return analysis
}

func (cc *crashClusterer) calculateImpact(cluster *CrashCluster) CrashImpact {
	now := time.Now()

	// Frequency score: crashes per hour
	duration := cluster.LastSeen.Sub(cluster.FirstSeen)
	if duration < time.Hour {
		duration = time.Hour
	}
	frequencyScore := float64(cluster.Count) / duration.Hours()
	frequencyScore = math.Min(frequencyScore/10.0, 1.0) // Normalize to 0-1

	// Recency score: exponential decay based on last seen
	hoursSinceLastSeen := now.Sub(cluster.LastSeen).Hours()
	recencyScore := math.Exp(-hoursSinceLastSeen / 24.0) // 24-hour half-life

	// Distribution score: how many unique bots saw this crash
	// For now, estimate based on crash count (would need bot info in real impl)
	distributionScore := math.Min(float64(cluster.Count)/10.0, 1.0)

	// Reproducibility score
	reproScore := 0.5 // Default
	switch cluster.Reproducibility {
	case common.ReproducibilityStatusConfirmed:
		reproScore = 1.0
	case common.ReproducibilityStatusFlaky:
		reproScore = 0.3
	case common.ReproducibilityStatusFailed:
		reproScore = 0.1
	}

	// Calculate overall score with weights
	overallScore := (frequencyScore * 0.3) +
		(recencyScore * 0.2) +
		(distributionScore * 0.2) +
		(reproScore * 0.3)

	return CrashImpact{
		FrequencyScore:       frequencyScore,
		RecencyScore:         recencyScore,
		DistributionScore:    distributionScore,
		ReproducibilityScore: reproScore,
		OverallScore:         overallScore,
	}
}

func (cc *crashClusterer) calculateSeverity(cluster *CrashCluster) CrashSeverity {
	impact := cluster.Impact.OverallScore

	// Consider crash type
	if cluster.RootCause != nil {
		switch cluster.RootCause.CauseType {
		case "buffer_overflow", "memory_corruption":
			impact += 0.2 // Boost for security-critical issues
		case "null_dereference":
			impact += 0.1
		}
	}

	// Map to severity levels
	if impact >= 0.8 {
		return SeverityCritical
	} else if impact >= 0.6 {
		return SeverityHigh
	} else if impact >= 0.4 {
		return SeverityMedium
	}
	return SeverityLow
}

func (cc *crashClusterer) extractFrames(trace string) []string {
	lines := strings.Split(trace, "\n")
	frames := make([]string, 0)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Common stack frame patterns
		if strings.Contains(line, " at ") ||
			strings.Contains(line, " in ") ||
			strings.HasPrefix(line, "#") ||
			strings.Contains(line, "()") ||
			strings.Contains(line, "0x") {
			frames = append(frames, line)
		}
	}

	return frames
}

func (cc *crashClusterer) extractSignal(trace string) string {
	// Look for signal information in the trace
	signalPatterns := []string{
		"SIGSEGV", "SIGABRT", "SIGFPE", "SIGILL", "SIGBUS", "SIGTRAP",
		"signal 11", "signal 6", "signal 8", "signal 4", "signal 7", "signal 5",
	}

	traceLower := strings.ToLower(trace)
	for _, pattern := range signalPatterns {
		if strings.Contains(traceLower, strings.ToLower(pattern)) {
			return pattern
		}
	}

	return ""
}

func (cc *crashClusterer) calculateWeightedFrameSimilarity(frames1, frames2 []string) float64 {
	if len(frames1) == 0 || len(frames2) == 0 {
		return 0.0
	}

	// Use dynamic programming for longest common subsequence
	maxLen := len(frames1)
	if len(frames2) > maxLen {
		maxLen = len(frames2)
	}

	// Calculate LCS length
	lcs := cc.longestCommonSubsequence(frames1, frames2)

	// Weight by position (top frames more important)
	weightedScore := 0.0
	totalWeight := 0.0

	for i := 0; i < maxLen; i++ {
		weight := 1.0 / float64(i+1) // Higher weight for top frames
		totalWeight += weight

		if i < len(frames1) && i < len(frames2) {
			if cc.framesMatch(frames1[i], frames2[i]) {
				weightedScore += weight
			}
		}
	}

	// Combine LCS and weighted position matching
	lcsScore := float64(lcs) / float64(maxLen)
	positionScore := weightedScore / totalWeight

	return (lcsScore * 0.6) + (positionScore * 0.4)
}

func (cc *crashClusterer) longestCommonSubsequence(frames1, frames2 []string) int {
	m, n := len(frames1), len(frames2)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if cc.framesMatch(frames1[i-1], frames2[j-1]) {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				dp[i][j] = max(dp[i-1][j], dp[i][j-1])
			}
		}
	}

	return dp[m][n]
}

func (cc *crashClusterer) framesMatch(frame1, frame2 string) bool {
	// Normalize and compare
	norm1 := cc.NormalizeStackTrace(frame1)
	norm2 := cc.NormalizeStackTrace(frame2)

	// Exact match after normalization
	if norm1 == norm2 {
		return true
	}

	// Extract function names and compare
	func1 := cc.extractFunctionName(norm1)
	func2 := cc.extractFunctionName(norm2)

	return func1 != "" && func1 == func2
}

func (cc *crashClusterer) extractFunctionName(frame string) string {
	// Common patterns for function names in stack traces
	patterns := []string{
		`(\w+)\s*\(`,    // function_name(
		`in\s+(\w+)`,    // in function_name
		`at\s+(\w+)`,    // at function_name
		`^(\w+)`,        // function_name at start
		`\s+(\w+)\s+at`, // function_name at
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(frame)
		if len(matches) > 1 {
			return matches[1]
		}
	}

	return ""
}

func (cc *crashClusterer) parseStackFrames(frameStrings []string) []common.StackFrame {
	frames := make([]common.StackFrame, 0, len(frameStrings))

	for _, frameStr := range frameStrings {
		frame := common.StackFrame{
			Function: cc.extractFunctionName(frameStr),
		}

		// Extract file and line if present
		filePattern := regexp.MustCompile(`(?:at|in)\s+([^:]+):(\d+)`)
		if matches := filePattern.FindStringSubmatch(frameStr); len(matches) > 2 {
			frame.File = matches[1]
			// Line number would be parsed from matches[2]
		}

		if frame.Function != "" {
			frames = append(frames, frame)
		}
	}

	return frames
}

func (cc *crashClusterer) generateClusterSignature(cluster *CrashCluster) string {
	// Create a unique signature for the cluster
	h := sha256.New()
	h.Write([]byte(cluster.CampaignID))
	h.Write([]byte(cluster.StackSignature))
	h.Write([]byte(cluster.CrashType))
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

func (cc *crashClusterer) generateStackSignature(frames []common.StackFrame) string {
	if len(frames) == 0 {
		return ""
	}

	// Use top 3 frames for signature
	sig := ""
	for i := 0; i < 3 && i < len(frames); i++ {
		if frames[i].Function != "" {
			sig += frames[i].Function + ":"
		}
	}

	return strings.TrimSuffix(sig, ":")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
