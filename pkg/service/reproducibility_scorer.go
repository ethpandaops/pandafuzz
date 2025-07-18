package service

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// ReproducibilityScore represents a detailed scoring of crash reproducibility
type ReproducibilityScore struct {
	CrashID        string                       `json:"crash_id"`
	BaseScore      float64                      `json:"base_score"`      // 0-100 main reproducibility score
	Confidence     float64                      `json:"confidence"`      // 0-100 confidence in the score
	Components     ScoreComponents              `json:"components"`      // Breakdown of scoring factors
	PlatformScores map[string]PlatformScore     `json:"platform_scores"` // Per-platform results
	Status         common.ReproducibilityStatus `json:"status"`          // Overall status
	LastUpdated    time.Time                    `json:"last_updated"`
	Metadata       map[string]interface{}       `json:"metadata"` // Additional scoring metadata
}

// ScoreComponents breaks down the factors contributing to the score
type ScoreComponents struct {
	ReproductionRate     float64 `json:"reproduction_rate"`     // % of successful reproductions
	ConsistencyScore     float64 `json:"consistency_score"`     // How consistent are the crashes
	TimeReliability      float64 `json:"time_reliability"`      // Reproduction timing consistency
	CrossPlatformScore   float64 `json:"cross_platform_score"`  // Works across platforms
	EnvironmentStability float64 `json:"environment_stability"` // Stable across environments
}

// PlatformScore tracks reproduction results per platform
type PlatformScore struct {
	Platform         string    `json:"platform"`     // e.g., "linux/amd64"
	Attempts         int       `json:"attempts"`     // Total attempts on this platform
	Successes        int       `json:"successes"`    // Successful reproductions
	AverageTime      float64   `json:"average_time"` // Avg reproduction time (seconds)
	SuccessRate      float64   `json:"success_rate"` // Platform-specific success rate
	LastAttempt      time.Time `json:"last_attempt"`
	ConsistentOutput bool      `json:"consistent_output"` // Output matches across runs
}

// FixVerification tracks automatic fix verification attempts
type FixVerification struct {
	ID              string    `json:"id"`
	CrashID         string    `json:"crash_id"`
	FixCommit       string    `json:"fix_commit"`       // Git commit with the fix
	VerificationJob string    `json:"verification_job"` // Job ID for verification
	Status          string    `json:"status"`           // "pending", "verified", "failed"
	Reproduced      bool      `json:"reproduced"`       // Whether crash still reproduces
	TestedAt        time.Time `json:"tested_at"`
	Result          string    `json:"result"` // Detailed result message
}

// reproducibilityScorer implements advanced scoring logic
type reproducibilityScorer struct {
	storage common.Storage
	logger  logrus.FieldLogger

	// Caching
	scoreCache   map[string]*ReproducibilityScore
	cacheMu      sync.RWMutex
	cacheTimeout time.Duration

	// Configuration
	minAttempts         int     // Minimum attempts before scoring
	confidenceThreshold float64 // Minimum confidence for status determination
	crossPlatformWeight float64 // Weight for cross-platform in scoring
	consistencyWeight   float64 // Weight for consistency in scoring
}

// NewReproducibilityScorer creates a new reproducibility scorer
func NewReproducibilityScorer(storage common.Storage, logger logrus.FieldLogger) *reproducibilityScorer {
	return &reproducibilityScorer{
		storage:             storage,
		logger:              logger.WithField("component", "reproducibility_scorer"),
		scoreCache:          make(map[string]*ReproducibilityScore),
		cacheTimeout:        5 * time.Minute,
		minAttempts:         3,
		confidenceThreshold: 70.0,
		crossPlatformWeight: 0.3,
		consistencyWeight:   0.2,
	}
}

// CalculateScore computes comprehensive reproducibility score for a crash
func (s *reproducibilityScorer) CalculateScore(ctx context.Context, crashID string) (*ReproducibilityScore, error) {
	// Check cache first
	s.cacheMu.RLock()
	if cached, ok := s.scoreCache[crashID]; ok && time.Since(cached.LastUpdated) < s.cacheTimeout {
		s.cacheMu.RUnlock()
		return cached, nil
	}
	s.cacheMu.RUnlock()

	// Get all reproduction results
	results, err := s.storage.GetReproductionResults(ctx, crashID)
	if err != nil {
		return nil, fmt.Errorf("failed to get reproduction results: %w", err)
	}

	if len(results) == 0 {
		return &ReproducibilityScore{
			CrashID:     crashID,
			BaseScore:   0,
			Confidence:  0,
			Status:      common.ReproducibilityStatusUnknown,
			LastUpdated: time.Now(),
		}, nil
	}

	// Calculate platform-specific scores
	platformScores := s.calculatePlatformScores(results)

	// Calculate component scores
	components := s.calculateComponents(results, platformScores)

	// Calculate base score
	baseScore := s.calculateBaseScore(components)

	// Calculate confidence
	confidence := s.calculateConfidence(results, platformScores)

	// Determine status
	status := s.determineStatus(baseScore, confidence, len(results))

	score := &ReproducibilityScore{
		CrashID:        crashID,
		BaseScore:      baseScore,
		Confidence:     confidence,
		Components:     components,
		PlatformScores: platformScores,
		Status:         status,
		LastUpdated:    time.Now(),
		Metadata: map[string]interface{}{
			"total_attempts":   len(results),
			"unique_platforms": len(platformScores),
			"first_attempt":    s.getFirstAttemptTime(results),
			"last_attempt":     s.getLastAttemptTime(results),
			"scoring_version":  "1.0",
		},
	}

	// Update cache
	s.cacheMu.Lock()
	s.scoreCache[crashID] = score
	s.cacheMu.Unlock()

	return score, nil
}

// calculatePlatformScores groups results by platform and calculates per-platform metrics
func (s *reproducibilityScorer) calculatePlatformScores(results []*common.ReproductionResult) map[string]PlatformScore {
	platformMap := make(map[string]*PlatformScore)

	for _, result := range results {
		platform := s.getPlatformKey(result.EnvironmentInfo)

		if _, exists := platformMap[platform]; !exists {
			platformMap[platform] = &PlatformScore{
				Platform: platform,
			}
		}

		ps := platformMap[platform]
		ps.Attempts++

		if result.Reproduced && result.MatchesOriginal {
			ps.Successes++
		}

		ps.AverageTime = (ps.AverageTime*float64(ps.Attempts-1) + result.ExecutionTime.Seconds()) / float64(ps.Attempts)
		ps.LastAttempt = result.Timestamp

		// Check output consistency
		if ps.Attempts > 1 && result.StackHash != "" {
			// This is simplified - in reality we'd track all stack hashes
			ps.ConsistentOutput = true
		}
	}

	// Calculate success rates
	scores := make(map[string]PlatformScore)
	for platform, ps := range platformMap {
		ps.SuccessRate = float64(ps.Successes) / float64(ps.Attempts) * 100
		scores[platform] = *ps
	}

	return scores
}

// calculateComponents calculates individual scoring components
func (s *reproducibilityScorer) calculateComponents(results []*common.ReproductionResult, platformScores map[string]PlatformScore) ScoreComponents {
	totalAttempts := len(results)
	successfulReproductions := 0
	var executionTimes []float64

	for _, result := range results {
		if result.Reproduced && result.MatchesOriginal {
			successfulReproductions++
		}
		executionTimes = append(executionTimes, result.ExecutionTime.Seconds())
	}

	components := ScoreComponents{
		ReproductionRate: float64(successfulReproductions) / float64(totalAttempts) * 100,
	}

	// Consistency score based on stack hash matching
	components.ConsistencyScore = s.calculateConsistencyScore(results)

	// Time reliability based on execution time variance
	components.TimeReliability = s.calculateTimeReliability(executionTimes)

	// Cross-platform score
	components.CrossPlatformScore = s.calculateCrossPlatformScore(platformScores)

	// Environment stability
	components.EnvironmentStability = s.calculateEnvironmentStability(results)

	return components
}

// calculateConsistencyScore measures how consistent crash outputs are
func (s *reproducibilityScorer) calculateConsistencyScore(results []*common.ReproductionResult) float64 {
	if len(results) < 2 {
		return 100.0 // Not enough data to measure inconsistency
	}

	stackHashes := make(map[string]int)
	reproducedCount := 0

	for _, result := range results {
		if result.Reproduced {
			reproducedCount++
			if result.StackHash != "" {
				stackHashes[result.StackHash]++
			}
		}
	}

	if reproducedCount == 0 {
		return 0.0
	}

	// Find the most common stack hash
	maxCount := 0
	for _, count := range stackHashes {
		if count > maxCount {
			maxCount = count
		}
	}

	// Consistency is the percentage of reproductions with the most common stack
	return float64(maxCount) / float64(reproducedCount) * 100
}

// calculateTimeReliability measures execution time consistency
func (s *reproducibilityScorer) calculateTimeReliability(times []float64) float64 {
	if len(times) < 2 {
		return 100.0
	}

	// Calculate mean and standard deviation
	mean := 0.0
	for _, t := range times {
		mean += t
	}
	mean /= float64(len(times))

	variance := 0.0
	for _, t := range times {
		variance += math.Pow(t-mean, 2)
	}
	variance /= float64(len(times))
	stdDev := math.Sqrt(variance)

	// Convert to reliability score (lower variance = higher reliability)
	// Using coefficient of variation
	if mean == 0 {
		return 100.0
	}

	cv := stdDev / mean
	reliability := math.Max(0, 100*(1-cv))

	return reliability
}

// calculateCrossPlatformScore measures reproduction across different platforms
func (s *reproducibilityScorer) calculateCrossPlatformScore(platformScores map[string]PlatformScore) float64 {
	if len(platformScores) == 0 {
		return 0.0
	}

	if len(platformScores) == 1 {
		// Only tested on one platform, use that platform's score but penalize
		for _, ps := range platformScores {
			return ps.SuccessRate * 0.7 // 30% penalty for single platform
		}
	}

	// Calculate weighted average across platforms
	totalScore := 0.0
	totalAttempts := 0
	successfulPlatforms := 0

	for _, ps := range platformScores {
		totalScore += ps.SuccessRate * float64(ps.Attempts)
		totalAttempts += ps.Attempts
		if ps.SuccessRate > 50 {
			successfulPlatforms++
		}
	}

	if totalAttempts == 0 {
		return 0.0
	}

	// Base score from weighted average
	baseScore := totalScore / float64(totalAttempts)

	// Bonus for working on multiple platforms
	platformBonus := float64(successfulPlatforms) / float64(len(platformScores)) * 20

	return math.Min(100, baseScore+platformBonus)
}

// calculateEnvironmentStability checks stability across different environments
func (s *reproducibilityScorer) calculateEnvironmentStability(results []*common.ReproductionResult) float64 {
	// Group by bot to check consistency per environment
	botResults := make(map[string][]bool)

	for _, result := range results {
		botResults[result.BotID] = append(botResults[result.BotID], result.Reproduced && result.MatchesOriginal)
	}

	// Calculate consistency per bot
	totalConsistency := 0.0
	botCount := 0

	for _, results := range botResults {
		if len(results) < 2 {
			continue
		}

		// Count transitions (changes in reproduction status)
		transitions := 0
		for i := 1; i < len(results); i++ {
			if results[i] != results[i-1] {
				transitions++
			}
		}

		// Fewer transitions = more stable
		consistency := 1.0 - float64(transitions)/float64(len(results)-1)
		totalConsistency += consistency
		botCount++
	}

	if botCount == 0 {
		return 100.0 // Not enough data
	}

	return (totalConsistency / float64(botCount)) * 100
}

// calculateBaseScore combines all components into final score
func (s *reproducibilityScorer) calculateBaseScore(components ScoreComponents) float64 {
	// Weighted combination of components
	score := components.ReproductionRate*0.4 +
		components.ConsistencyScore*s.consistencyWeight +
		components.TimeReliability*0.1 +
		components.CrossPlatformScore*s.crossPlatformWeight +
		components.EnvironmentStability*0.1

	return math.Min(100, math.Max(0, score))
}

// calculateConfidence determines confidence in the score
func (s *reproducibilityScorer) calculateConfidence(results []*common.ReproductionResult, platformScores map[string]PlatformScore) float64 {
	// Base confidence on number of attempts
	attemptConfidence := math.Min(100, float64(len(results))/float64(s.minAttempts*3)*100)

	// Platform diversity confidence
	platformConfidence := math.Min(100, float64(len(platformScores))*33.33)

	// Time span confidence (tests over longer period = higher confidence)
	timeSpan := s.getTimeSpan(results)
	timeConfidence := math.Min(100, timeSpan.Hours()/24*20) // 5 days = 100%

	// Recent test confidence (recent tests = higher confidence)
	recencyConfidence := 100.0
	if len(results) > 0 {
		lastAttempt := s.getLastAttemptTime(results)
		hoursSinceLastAttempt := time.Since(lastAttempt).Hours()
		recencyConfidence = math.Max(0, 100-hoursSinceLastAttempt*2) // -2% per hour
	}

	// Combine confidence factors
	confidence := (attemptConfidence*0.3 + platformConfidence*0.3 +
		timeConfidence*0.2 + recencyConfidence*0.2)

	return math.Min(100, math.Max(0, confidence))
}

// determineStatus determines the reproducibility status based on score and confidence
func (s *reproducibilityScorer) determineStatus(score, confidence float64, attempts int) common.ReproducibilityStatus {
	// Not enough attempts
	if attempts < s.minAttempts {
		return common.ReproducibilityStatusTesting
	}

	// Low confidence
	if confidence < s.confidenceThreshold {
		return common.ReproducibilityStatusTesting
	}

	// Determine based on score
	switch {
	case score >= 80:
		return common.ReproducibilityStatusConfirmed
	case score >= 50:
		return common.ReproducibilityStatusFlaky
	case score > 0:
		return common.ReproducibilityStatusFailed
	default:
		return common.ReproducibilityStatusFailed
	}
}

// Helper methods

func (s *reproducibilityScorer) getPlatformKey(envInfo map[string]string) string {
	os := envInfo["os"]
	arch := envInfo["arch"]
	if os == "" {
		os = "unknown"
	}
	if arch == "" {
		arch = "unknown"
	}
	return fmt.Sprintf("%s/%s", os, arch)
}

func (s *reproducibilityScorer) getFirstAttemptTime(results []*common.ReproductionResult) time.Time {
	if len(results) == 0 {
		return time.Time{}
	}

	earliest := results[0].Timestamp
	for _, r := range results[1:] {
		if r.Timestamp.Before(earliest) {
			earliest = r.Timestamp
		}
	}
	return earliest
}

func (s *reproducibilityScorer) getLastAttemptTime(results []*common.ReproductionResult) time.Time {
	if len(results) == 0 {
		return time.Time{}
	}

	latest := results[0].Timestamp
	for _, r := range results[1:] {
		if r.Timestamp.After(latest) {
			latest = r.Timestamp
		}
	}
	return latest
}

func (s *reproducibilityScorer) getTimeSpan(results []*common.ReproductionResult) time.Duration {
	if len(results) < 2 {
		return 0
	}

	first := s.getFirstAttemptTime(results)
	last := s.getLastAttemptTime(results)
	return last.Sub(first)
}

// VerifyFix checks if a crash has been fixed in a given commit
func (s *reproducibilityScorer) VerifyFix(ctx context.Context, crashID, fixCommit string) (*FixVerification, error) {
	s.logger.WithFields(logrus.Fields{
		"crash_id":   crashID,
		"fix_commit": fixCommit,
	}).Info("Verifying fix for crash")

	// Create verification job
	verification := &FixVerification{
		ID:        fmt.Sprintf("fix-%s-%d", crashID, time.Now().Unix()),
		CrashID:   crashID,
		FixCommit: fixCommit,
		Status:    "pending",
		TestedAt:  time.Now(),
	}

	// This would trigger a reproduction job with the fixed code
	// For now, we'll just return the verification structure
	// In a real implementation, this would:
	// 1. Create a special reproduction job with the fixed binary
	// 2. Queue it with high priority
	// 3. Track the result

	return verification, nil
}

// GetTrendAnalysis analyzes reproduction trends over time
func (s *reproducibilityScorer) GetTrendAnalysis(ctx context.Context, crashID string) (map[string]interface{}, error) {
	results, err := s.storage.GetReproductionResults(ctx, crashID)
	if err != nil {
		return nil, fmt.Errorf("failed to get reproduction results: %w", err)
	}

	if len(results) < 2 {
		return map[string]interface{}{
			"trend":       "insufficient_data",
			"data_points": len(results),
		}, nil
	}

	// Group results by day
	dailyStats := make(map[string]struct {
		attempts  int
		successes int
	})

	for _, result := range results {
		day := result.Timestamp.Format("2006-01-02")
		stats := dailyStats[day]
		stats.attempts++
		if result.Reproduced && result.MatchesOriginal {
			stats.successes++
		}
		dailyStats[day] = stats
	}

	// Calculate trend
	var rates []float64
	for _, stats := range dailyStats {
		rate := float64(stats.successes) / float64(stats.attempts)
		rates = append(rates, rate)
	}

	// Simple trend detection
	trend := "stable"
	if len(rates) >= 3 {
		// Check if rates are increasing or decreasing
		increasing := 0
		decreasing := 0
		for i := 1; i < len(rates); i++ {
			if rates[i] > rates[i-1] {
				increasing++
			} else if rates[i] < rates[i-1] {
				decreasing++
			}
		}

		if increasing > decreasing && increasing > len(rates)/2 {
			trend = "improving"
		} else if decreasing > increasing && decreasing > len(rates)/2 {
			trend = "degrading"
		}
	}

	return map[string]interface{}{
		"trend":         trend,
		"data_points":   len(results),
		"daily_stats":   dailyStats,
		"success_rates": rates,
	}, nil
}
