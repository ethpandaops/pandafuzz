package analytics

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/sirupsen/logrus"
)

// CoverageAnalyzer provides advanced coverage analysis capabilities
type CoverageAnalyzer interface {
	// AnalyzeCoverageTrend performs comprehensive trend analysis on coverage data
	AnalyzeCoverageTrend(ctx context.Context, campaignID string, window time.Duration) (*CoverageTrendAnalysis, error)

	// PredictCoveragePlateau predicts when coverage growth will plateau
	PredictCoveragePlateau(ctx context.Context, campaignID string) (*PlateauPrediction, error)

	// IdentifyCoverageAnomalies detects unusual patterns in coverage data
	IdentifyCoverageAnomalies(ctx context.Context, campaignID string, sensitivity float64) ([]*CoverageAnomaly, error)
}

// CoverageTrendAnalysis represents comprehensive coverage trend analysis
type CoverageTrendAnalysis struct {
	CampaignID         string               `json:"campaign_id"`
	Window             time.Duration        `json:"window"`
	StartTime          time.Time            `json:"start_time"`
	EndTime            time.Time            `json:"end_time"`
	DataPoints         []CoverageDataPoint  `json:"data_points"`
	TrendType          string               `json:"trend_type"` // "exponential", "linear", "logarithmic", "plateau"
	GrowthRate         float64              `json:"growth_rate"`
	GrowthAcceleration float64              `json:"growth_acceleration"`
	Seasonality        *SeasonalityAnalysis `json:"seasonality,omitempty"`
	Efficiency         *EfficiencyMetrics   `json:"efficiency"`
	Recommendations    []string             `json:"recommendations"`
}

// CoverageDataPoint represents a single coverage measurement with metadata
type CoverageDataPoint struct {
	Timestamp       time.Time `json:"timestamp"`
	TotalEdges      int64     `json:"total_edges"`
	NewEdges        int64     `json:"new_edges"`
	ExecCount       int64     `json:"exec_count"`
	ExecPerSec      float64   `json:"exec_per_sec"`
	CorpusSize      int       `json:"corpus_size"`
	CorpusSizeBytes int64     `json:"corpus_size_bytes"`
	ActiveBots      int       `json:"active_bots"`
	GrowthRate      float64   `json:"growth_rate"`      // Edges per hour at this point
	EfficiencyRatio float64   `json:"efficiency_ratio"` // New edges per execution
}

// SeasonalityAnalysis identifies periodic patterns in coverage growth
type SeasonalityAnalysis struct {
	HasSeasonality bool               `json:"has_seasonality"`
	Period         time.Duration      `json:"period"`
	Amplitude      float64            `json:"amplitude"`
	PeakTimes      []time.Time        `json:"peak_times"`
	TroughTimes    []time.Time        `json:"trough_times"`
	DailyPattern   map[int]float64    `json:"daily_pattern"`  // Hour -> average growth rate
	WeeklyPattern  map[string]float64 `json:"weekly_pattern"` // Day -> average growth rate
}

// EfficiencyMetrics tracks fuzzing efficiency over time
type EfficiencyMetrics struct {
	AverageEfficiency float64 `json:"average_efficiency"` // New edges per 1000 executions
	EfficiencyTrend   string  `json:"efficiency_trend"`   // "improving", "declining", "stable"
	OptimalCorpusSize int     `json:"optimal_corpus_size"`
	CurrentVsOptimal  float64 `json:"current_vs_optimal"` // Ratio
	CorpusDiversity   float64 `json:"corpus_diversity"`   // 0-1 score
	DuplicationRate   float64 `json:"duplication_rate"`   // Percentage of duplicate inputs
}

// PlateauPrediction predicts when coverage will plateau
type PlateauPrediction struct {
	CampaignID             string                 `json:"campaign_id"`
	CurrentCoverage        int64                  `json:"current_coverage"`
	PredictedPlateau       int64                  `json:"predicted_plateau"`
	TimeToPlateau          time.Duration          `json:"time_to_plateau"`
	PlateauConfidence      float64                `json:"plateau_confidence"` // 0-1
	PlateauDate            time.Time              `json:"plateau_date"`
	GrowthDecayRate        float64                `json:"growth_decay_rate"` // Rate at which growth is slowing
	ResourcesAtPlateau     ResourceEstimate       `json:"resources_at_plateau"`
	OptimizationStrategies []OptimizationStrategy `json:"optimization_strategies"`
}

// ResourceEstimate estimates resources needed to reach plateau
type ResourceEstimate struct {
	ExecutionsRequired int64         `json:"executions_required"`
	ComputeHours       float64       `json:"compute_hours"`
	EstimatedCost      float64       `json:"estimated_cost"`
	OptimalBotCount    int           `json:"optimal_bot_count"`
	TimeWithResources  time.Duration `json:"time_with_resources"`
}

// OptimizationStrategy suggests ways to improve coverage growth
type OptimizationStrategy struct {
	Strategy             string `json:"strategy"`
	Description          string `json:"description"`
	ExpectedImpact       string `json:"expected_impact"`
	ImplementationEffort string `json:"implementation_effort"` // "low", "medium", "high"
	Priority             int    `json:"priority"`              // 1-10
}

// CoverageAnomaly represents an unusual pattern in coverage data
type CoverageAnomaly struct {
	Timestamp       time.Time     `json:"timestamp"`
	Type            string        `json:"type"`     // "spike", "drop", "stagnation", "regression"
	Severity        string        `json:"severity"` // "low", "medium", "high", "critical"
	Description     string        `json:"description"`
	AffectedMetrics []string      `json:"affected_metrics"`
	Duration        time.Duration `json:"duration"`
	Impact          AnomalyImpact `json:"impact"`
	PossibleCauses  []string      `json:"possible_causes"`
	Recommendations []string      `json:"recommendations"`
}

// AnomalyImpact quantifies the impact of an anomaly
type AnomalyImpact struct {
	CoverageLost     int64         `json:"coverage_lost"`
	EfficiencyImpact float64       `json:"efficiency_impact"` // Percentage change
	ResourcesWasted  float64       `json:"resources_wasted"`  // Compute hours
	RecoveryTime     time.Duration `json:"recovery_time"`
}

// coverageAnalyzer implementation
type coverageAnalyzer struct {
	storage   common.Storage
	analytics service.AnalyticsService
	logger    logrus.FieldLogger
}

// NewCoverageAnalyzer creates a new coverage analyzer
func NewCoverageAnalyzer(storage common.Storage, analytics service.AnalyticsService, logger *logrus.Logger) CoverageAnalyzer {
	fieldLogger := logger.WithField("component", "coverage_analyzer")
	return &coverageAnalyzer{
		storage:   storage,
		analytics: analytics,
		logger:    fieldLogger,
	}
}

// AnalyzeCoverageTrend performs comprehensive trend analysis
func (ca *coverageAnalyzer) AnalyzeCoverageTrend(ctx context.Context, campaignID string, window time.Duration) (*CoverageTrendAnalysis, error) {
	ca.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"window":      window,
	}).Debug("Analyzing coverage trend")

	// Get coverage data from analytics service
	trend, err := ca.analytics.GetCoverageTrend(ctx, campaignID, window)
	if err != nil {
		return nil, fmt.Errorf("failed to get coverage trend: %w", err)
	}

	// Convert to our data points format with additional analysis
	dataPoints := ca.enrichDataPoints(trend)

	// Determine trend type
	trendType := ca.determineTrendType(dataPoints)

	// Calculate growth metrics
	growthRate, growthAcceleration := ca.calculateGrowthMetrics(dataPoints)

	// Analyze seasonality
	seasonality := ca.analyzeSeasonality(dataPoints)

	// Calculate efficiency metrics
	efficiency := ca.calculateEfficiency(dataPoints)

	// Generate recommendations
	recommendations := ca.generateRecommendations(trendType, efficiency, seasonality)

	return &CoverageTrendAnalysis{
		CampaignID:         campaignID,
		Window:             window,
		StartTime:          trend.StartTime,
		EndTime:            trend.EndTime,
		DataPoints:         dataPoints,
		TrendType:          trendType,
		GrowthRate:         growthRate,
		GrowthAcceleration: growthAcceleration,
		Seasonality:        seasonality,
		Efficiency:         efficiency,
		Recommendations:    recommendations,
	}, nil
}

// PredictCoveragePlateau predicts when coverage will plateau
func (ca *coverageAnalyzer) PredictCoveragePlateau(ctx context.Context, campaignID string) (*PlateauPrediction, error) {
	ca.logger.WithField("campaign_id", campaignID).Debug("Predicting coverage plateau")

	// Get historical coverage data (last 7 days)
	trend, err := ca.analytics.GetCoverageTrend(ctx, campaignID, 7*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to get coverage trend: %w", err)
	}

	if len(trend.DataPoints) < 10 {
		return nil, fmt.Errorf("insufficient data points for plateau prediction")
	}

	dataPoints := ca.enrichDataPoints(trend)

	// Fit logarithmic growth model
	currentCoverage := dataPoints[len(dataPoints)-1].TotalEdges
	plateauCoverage, decayRate := ca.fitGrowthModel(dataPoints)

	// Calculate time to plateau
	timeToPlateau := ca.calculateTimeToPlateau(dataPoints, plateauCoverage, decayRate)

	// Calculate confidence based on model fit
	confidence := ca.calculatePlateauConfidence(dataPoints, plateauCoverage)

	// Estimate resources needed
	resources := ca.estimateResources(currentCoverage, plateauCoverage, dataPoints)

	// Generate optimization strategies
	strategies := ca.generateOptimizationStrategies(dataPoints, decayRate)

	return &PlateauPrediction{
		CampaignID:             campaignID,
		CurrentCoverage:        currentCoverage,
		PredictedPlateau:       plateauCoverage,
		TimeToPlateau:          timeToPlateau,
		PlateauConfidence:      confidence,
		PlateauDate:            time.Now().Add(timeToPlateau),
		GrowthDecayRate:        decayRate,
		ResourcesAtPlateau:     resources,
		OptimizationStrategies: strategies,
	}, nil
}

// IdentifyCoverageAnomalies detects unusual patterns
func (ca *coverageAnalyzer) IdentifyCoverageAnomalies(ctx context.Context, campaignID string, sensitivity float64) ([]*CoverageAnomaly, error) {
	ca.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"sensitivity": sensitivity,
	}).Debug("Identifying coverage anomalies")

	// Get coverage data for analysis (last 24 hours)
	trend, err := ca.analytics.GetCoverageTrend(ctx, campaignID, 24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to get coverage trend: %w", err)
	}

	dataPoints := ca.enrichDataPoints(trend)
	anomalies := make([]*CoverageAnomaly, 0)

	// Detect different types of anomalies
	anomalies = append(anomalies, ca.detectSpikes(dataPoints, sensitivity)...)
	anomalies = append(anomalies, ca.detectDrops(dataPoints, sensitivity)...)
	anomalies = append(anomalies, ca.detectStagnation(dataPoints, sensitivity)...)
	anomalies = append(anomalies, ca.detectRegression(dataPoints)...)

	// Sort by timestamp
	sort.Slice(anomalies, func(i, j int) bool {
		return anomalies[i].Timestamp.Before(anomalies[j].Timestamp)
	})

	return anomalies, nil
}

// Helper methods

func (ca *coverageAnalyzer) enrichDataPoints(trend *service.CoverageTrend) []CoverageDataPoint {
	points := make([]CoverageDataPoint, len(trend.DataPoints))

	for i, p := range trend.DataPoints {
		points[i] = CoverageDataPoint{
			Timestamp:       p.Timestamp,
			TotalEdges:      p.TotalEdges,
			NewEdges:        p.NewEdges,
			ExecCount:       p.ExecCount,
			ExecPerSec:      p.ExecPerSec,
			CorpusSize:      p.CorpusSize,
			CorpusSizeBytes: p.CorpusBytes,
		}

		// Calculate growth rate
		if i > 0 {
			timeDiff := p.Timestamp.Sub(trend.DataPoints[i-1].Timestamp).Hours()
			edgeDiff := float64(p.TotalEdges - trend.DataPoints[i-1].TotalEdges)
			points[i].GrowthRate = edgeDiff / timeDiff
		}

		// Calculate efficiency ratio
		if p.ExecCount > 0 {
			points[i].EfficiencyRatio = float64(p.NewEdges) / float64(p.ExecCount)
		}
	}

	return points
}

func (ca *coverageAnalyzer) determineTrendType(points []CoverageDataPoint) string {
	if len(points) < 3 {
		return "unknown"
	}

	// Calculate average growth rates for different periods
	earlyGrowth := ca.averageGrowthRate(points[:len(points)/3])
	midGrowth := ca.averageGrowthRate(points[len(points)/3 : 2*len(points)/3])
	lateGrowth := ca.averageGrowthRate(points[2*len(points)/3:])

	// Determine trend based on growth pattern
	if lateGrowth < earlyGrowth*0.1 {
		return "plateau"
	} else if lateGrowth > earlyGrowth*0.9 && midGrowth > earlyGrowth*0.9 {
		return "linear"
	} else if lateGrowth > earlyGrowth {
		return "exponential"
	} else {
		return "logarithmic"
	}
}

func (ca *coverageAnalyzer) calculateGrowthMetrics(points []CoverageDataPoint) (float64, float64) {
	if len(points) < 2 {
		return 0, 0
	}

	// Average growth rate
	totalGrowth := float64(0)
	for _, p := range points[1:] {
		totalGrowth += p.GrowthRate
	}
	avgGrowth := totalGrowth / float64(len(points)-1)

	// Growth acceleration (change in growth rate)
	if len(points) < 3 {
		return avgGrowth, 0
	}

	totalAcceleration := float64(0)
	for i := 2; i < len(points); i++ {
		acceleration := points[i].GrowthRate - points[i-1].GrowthRate
		totalAcceleration += acceleration
	}
	avgAcceleration := totalAcceleration / float64(len(points)-2)

	return avgGrowth, avgAcceleration
}

func (ca *coverageAnalyzer) analyzeSeasonality(points []CoverageDataPoint) *SeasonalityAnalysis {
	if len(points) < 24 { // Need at least 24 hours of data
		return nil
	}

	// Analyze hourly patterns
	hourlyGrowth := make(map[int][]float64)
	for _, p := range points {
		hour := p.Timestamp.Hour()
		hourlyGrowth[hour] = append(hourlyGrowth[hour], p.GrowthRate)
	}

	// Calculate average growth by hour
	dailyPattern := make(map[int]float64)
	var maxVariation float64
	for hour, rates := range hourlyGrowth {
		if len(rates) > 0 {
			avg := ca.average(rates)
			dailyPattern[hour] = avg
			maxVariation = math.Max(maxVariation, avg)
		}
	}

	// Check if there's significant seasonality
	hasSeasonality := maxVariation > ca.average(ca.getAllGrowthRates(points))*1.5

	if !hasSeasonality {
		return &SeasonalityAnalysis{
			HasSeasonality: false,
		}
	}

	// Find peak and trough times
	peakTimes := ca.findPeakTimes(points, dailyPattern)
	troughTimes := ca.findTroughTimes(points, dailyPattern)

	return &SeasonalityAnalysis{
		HasSeasonality: true,
		Period:         24 * time.Hour,
		Amplitude:      maxVariation,
		PeakTimes:      peakTimes,
		TroughTimes:    troughTimes,
		DailyPattern:   dailyPattern,
	}
}

func (ca *coverageAnalyzer) calculateEfficiency(points []CoverageDataPoint) *EfficiencyMetrics {
	if len(points) == 0 {
		return &EfficiencyMetrics{}
	}

	// Calculate average efficiency
	totalEfficiency := float64(0)
	validPoints := 0
	for _, p := range points {
		if p.ExecCount > 0 {
			efficiency := float64(p.NewEdges) * 1000 / float64(p.ExecCount)
			totalEfficiency += efficiency
			validPoints++
		}
	}

	avgEfficiency := float64(0)
	if validPoints > 0 {
		avgEfficiency = totalEfficiency / float64(validPoints)
	}

	// Determine efficiency trend
	efficiencyTrend := "stable"
	if len(points) > 10 {
		earlyEff := ca.averageEfficiency(points[:len(points)/2])
		lateEff := ca.averageEfficiency(points[len(points)/2:])

		if lateEff > earlyEff*1.1 {
			efficiencyTrend = "improving"
		} else if lateEff < earlyEff*0.9 {
			efficiencyTrend = "declining"
		}
	}

	// Estimate optimal corpus size (simplified heuristic)
	optimalSize := ca.estimateOptimalCorpusSize(points)
	currentSize := points[len(points)-1].CorpusSize
	currentVsOptimal := float64(currentSize) / float64(optimalSize)

	return &EfficiencyMetrics{
		AverageEfficiency: avgEfficiency,
		EfficiencyTrend:   efficiencyTrend,
		OptimalCorpusSize: optimalSize,
		CurrentVsOptimal:  currentVsOptimal,
		CorpusDiversity:   ca.estimateCorpusDiversity(points),
		DuplicationRate:   ca.estimateDuplicationRate(points),
	}
}

func (ca *coverageAnalyzer) generateRecommendations(trendType string, efficiency *EfficiencyMetrics, seasonality *SeasonalityAnalysis) []string {
	recommendations := make([]string, 0)

	// Trend-based recommendations
	switch trendType {
	case "plateau":
		recommendations = append(recommendations, "Coverage growth is plateauing. Consider introducing new seed inputs or mutation strategies.")
		recommendations = append(recommendations, "Analyze corpus for redundancy and remove duplicate or similar inputs.")
	case "logarithmic":
		recommendations = append(recommendations, "Coverage growth is slowing. This is normal but consider corpus minimization.")
	}

	// Efficiency-based recommendations
	if efficiency.EfficiencyTrend == "declining" {
		recommendations = append(recommendations, "Fuzzing efficiency is declining. Consider corpus pruning or adjusting fuzzer parameters.")
	}
	if efficiency.CurrentVsOptimal > 2 {
		recommendations = append(recommendations, "Corpus size exceeds optimal by 2x. Consider aggressive corpus minimization.")
	}
	if efficiency.DuplicationRate > 0.3 {
		recommendations = append(recommendations, "High duplication rate detected (>30%). Implement better deduplication strategies.")
	}

	// Seasonality-based recommendations
	if seasonality != nil && seasonality.HasSeasonality {
		recommendations = append(recommendations, "Coverage growth shows daily patterns. Schedule intensive fuzzing during peak hours.")
	}

	return recommendations
}

func (ca *coverageAnalyzer) fitGrowthModel(points []CoverageDataPoint) (int64, float64) {
	// Simplified logarithmic growth model: coverage = a * log(executions) + b
	// Returns predicted plateau coverage and decay rate

	if len(points) < 3 {
		return points[len(points)-1].TotalEdges, 0
	}

	// Calculate growth decay
	decayRates := make([]float64, 0)
	for i := 2; i < len(points); i++ {
		if points[i-1].GrowthRate > 0 {
			decay := (points[i-1].GrowthRate - points[i].GrowthRate) / points[i-1].GrowthRate
			if decay > 0 {
				decayRates = append(decayRates, decay)
			}
		}
	}

	avgDecay := ca.average(decayRates)

	// Estimate plateau based on current coverage and decay rate
	currentCoverage := points[len(points)-1].TotalEdges
	currentGrowth := points[len(points)-1].GrowthRate

	// Simplified estimation: plateau when growth rate approaches 0
	remainingGrowth := currentGrowth
	estimatedAdditional := int64(0)

	for remainingGrowth > 1 { // 1 edge per hour threshold
		estimatedAdditional += int64(remainingGrowth)
		remainingGrowth *= (1 - avgDecay)
	}

	plateau := currentCoverage + estimatedAdditional

	return plateau, avgDecay
}

func (ca *coverageAnalyzer) calculateTimeToPlateau(points []CoverageDataPoint, plateau int64, decayRate float64) time.Duration {
	if len(points) == 0 || decayRate <= 0 {
		return 0
	}

	currentCoverage := points[len(points)-1].TotalEdges
	currentGrowth := points[len(points)-1].GrowthRate

	remaining := plateau - currentCoverage
	if remaining <= 0 || currentGrowth <= 0 {
		return 0
	}

	// Calculate hours to plateau
	hours := float64(0)
	growth := currentGrowth
	covered := int64(0)

	for covered < remaining && growth > 0.1 {
		covered += int64(growth)
		growth *= (1 - decayRate)
		hours++
	}

	return time.Duration(hours) * time.Hour
}

func (ca *coverageAnalyzer) calculatePlateauConfidence(points []CoverageDataPoint, plateau int64) float64 {
	// Confidence based on model fit and data consistency
	if len(points) < 10 {
		return 0.3
	}

	// Check consistency of decay pattern
	consistentDecay := ca.checkDecayConsistency(points)

	// Check if we're already near plateau
	currentCoverage := points[len(points)-1].TotalEdges
	nearPlateau := float64(currentCoverage) / float64(plateau)

	// Base confidence on multiple factors
	confidence := 0.0
	confidence += consistentDecay * 0.4
	confidence += math.Min(nearPlateau, 1.0) * 0.3
	confidence += math.Min(float64(len(points))/100, 1.0) * 0.3

	return math.Min(confidence, 0.95)
}

func (ca *coverageAnalyzer) estimateResources(current, plateau int64, points []CoverageDataPoint) ResourceEstimate {
	if len(points) == 0 {
		return ResourceEstimate{}
	}

	remaining := plateau - current

	// Estimate executions needed based on current efficiency
	avgEfficiency := ca.averageEfficiency(points)
	execsRequired := int64(0)
	if avgEfficiency > 0 {
		execsRequired = int64(float64(remaining) / avgEfficiency * 1000)
	}

	// Estimate compute hours
	avgExecSpeed := ca.averageExecSpeed(points)
	computeHours := float64(execsRequired) / avgExecSpeed / 3600

	// Estimate optimal bot count (simplified)
	optimalBots := int(math.Sqrt(float64(remaining)) / 100)
	if optimalBots < 1 {
		optimalBots = 1
	}
	if optimalBots > 100 {
		optimalBots = 100
	}

	// Time with optimal resources
	timeWithResources := time.Duration(computeHours/float64(optimalBots)) * time.Hour

	return ResourceEstimate{
		ExecutionsRequired: execsRequired,
		ComputeHours:       computeHours,
		EstimatedCost:      computeHours * 0.10, // $0.10 per compute hour estimate
		OptimalBotCount:    optimalBots,
		TimeWithResources:  timeWithResources,
	}
}

func (ca *coverageAnalyzer) generateOptimizationStrategies(points []CoverageDataPoint, decayRate float64) []OptimizationStrategy {
	strategies := make([]OptimizationStrategy, 0)

	// High decay rate suggests need for new approaches
	if decayRate > 0.1 {
		strategies = append(strategies, OptimizationStrategy{
			Strategy:             "Introduce Grammar-Based Fuzzing",
			Description:          "Use grammar-based generation to explore structured input spaces more effectively",
			ExpectedImpact:       "15-30% coverage increase",
			ImplementationEffort: "medium",
			Priority:             8,
		})
	}

	// Check corpus efficiency
	if len(points) > 0 && points[len(points)-1].CorpusSize > 10000 {
		strategies = append(strategies, OptimizationStrategy{
			Strategy:             "Corpus Minimization",
			Description:          "Reduce corpus to essential test cases that maintain coverage",
			ExpectedImpact:       "50% reduction in corpus size, 20% speed improvement",
			ImplementationEffort: "low",
			Priority:             9,
		})
	}

	// Low efficiency suggests parameter tuning
	avgEfficiency := ca.averageEfficiency(points)
	if avgEfficiency < 0.1 {
		strategies = append(strategies, OptimizationStrategy{
			Strategy:             "Fuzzer Parameter Tuning",
			Description:          "Optimize mutation rates, dictionary usage, and execution timeouts",
			ExpectedImpact:       "10-20% efficiency improvement",
			ImplementationEffort: "low",
			Priority:             7,
		})
	}

	// Always suggest coverage-guided strategies
	strategies = append(strategies, OptimizationStrategy{
		Strategy:             "Targeted Input Generation",
		Description:          "Focus on inputs that exercise rarely-hit code paths",
		ExpectedImpact:       "5-15% coverage increase in specific areas",
		ImplementationEffort: "high",
		Priority:             6,
	})

	return strategies
}

func (ca *coverageAnalyzer) detectSpikes(points []CoverageDataPoint, sensitivity float64) []*CoverageAnomaly {
	anomalies := make([]*CoverageAnomaly, 0)

	if len(points) < 3 {
		return anomalies
	}

	// Calculate baseline growth rate
	baselineGrowth := ca.averageGrowthRate(points)
	threshold := baselineGrowth * (2.0 - sensitivity) // Lower sensitivity = higher threshold

	for i := 1; i < len(points); i++ {
		if points[i].GrowthRate > threshold && points[i].GrowthRate > baselineGrowth*2 {
			anomaly := &CoverageAnomaly{
				Timestamp:       points[i].Timestamp,
				Type:            "spike",
				Severity:        ca.calculateSeverity(points[i].GrowthRate, baselineGrowth),
				Description:     fmt.Sprintf("Unusual coverage spike detected: %.0f edges/hour (baseline: %.0f)", points[i].GrowthRate, baselineGrowth),
				AffectedMetrics: []string{"growth_rate", "new_edges"},
				Duration:        ca.calculateAnomalyDuration(points, i, "spike"),
				Impact: AnomalyImpact{
					CoverageLost:     0, // Spikes don't lose coverage
					EfficiencyImpact: (points[i].EfficiencyRatio - ca.averageEfficiency(points)) / ca.averageEfficiency(points) * 100,
				},
				PossibleCauses: []string{
					"New code paths discovered",
					"Effective mutation found",
					"Bot configuration change",
					"Target binary updated",
				},
				Recommendations: []string{
					"Investigate what triggered the spike",
					"Preserve inputs that caused the spike",
					"Consider increasing resources to maintain momentum",
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

func (ca *coverageAnalyzer) detectDrops(points []CoverageDataPoint, sensitivity float64) []*CoverageAnomaly {
	anomalies := make([]*CoverageAnomaly, 0)

	if len(points) < 3 {
		return anomalies
	}

	baselineGrowth := ca.averageGrowthRate(points)
	threshold := baselineGrowth * sensitivity

	for i := 1; i < len(points); i++ {
		if points[i].GrowthRate < threshold && points[i-1].GrowthRate > threshold {
			anomaly := &CoverageAnomaly{
				Timestamp:       points[i].Timestamp,
				Type:            "drop",
				Severity:        ca.calculateDropSeverity(points[i].GrowthRate, points[i-1].GrowthRate),
				Description:     fmt.Sprintf("Coverage growth dropped from %.0f to %.0f edges/hour", points[i-1].GrowthRate, points[i].GrowthRate),
				AffectedMetrics: []string{"growth_rate", "efficiency"},
				Duration:        ca.calculateAnomalyDuration(points, i, "drop"),
				Impact: AnomalyImpact{
					CoverageLost:     ca.estimateLostCoverage(points, i),
					EfficiencyImpact: -50.0, // Significant negative impact
					ResourcesWasted:  ca.calculateWastedResources(points, i),
				},
				PossibleCauses: []string{
					"Bot failures or timeouts",
					"Resource constraints",
					"Corpus corruption",
					"Network issues",
					"Fuzzer configuration problems",
				},
				Recommendations: []string{
					"Check bot health and logs",
					"Verify resource availability",
					"Review recent configuration changes",
					"Consider rolling back to previous corpus snapshot",
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

func (ca *coverageAnalyzer) detectStagnation(points []CoverageDataPoint, sensitivity float64) []*CoverageAnomaly {
	anomalies := make([]*CoverageAnomaly, 0)

	if len(points) < 5 {
		return anomalies
	}

	// Look for periods of near-zero growth
	stagnationThreshold := 1.0 * sensitivity // edges per hour
	stagnationStart := -1

	for i := 0; i < len(points); i++ {
		if points[i].GrowthRate < stagnationThreshold {
			if stagnationStart == -1 {
				stagnationStart = i
			}
		} else if stagnationStart != -1 {
			// End of stagnation period
			duration := points[i-1].Timestamp.Sub(points[stagnationStart].Timestamp)
			if duration > 2*time.Hour {
				anomaly := &CoverageAnomaly{
					Timestamp:       points[stagnationStart].Timestamp,
					Type:            "stagnation",
					Severity:        ca.calculateStagnationSeverity(duration),
					Description:     fmt.Sprintf("Coverage stagnated for %v", duration),
					AffectedMetrics: []string{"growth_rate", "new_edges", "efficiency"},
					Duration:        duration,
					Impact: AnomalyImpact{
						CoverageLost:    ca.estimateLostOpportunity(points, stagnationStart, i),
						ResourcesWasted: duration.Hours() * float64(ca.countActiveBots(points[stagnationStart:i])),
					},
					PossibleCauses: []string{
						"Corpus saturation",
						"Ineffective mutations",
						"Need for new seed inputs",
						"Fuzzer stuck in local optimum",
					},
					Recommendations: []string{
						"Add diverse seed inputs",
						"Try different fuzzing strategies",
						"Perform corpus minimization",
						"Consider manual analysis of uncovered code",
					},
				}
				anomalies = append(anomalies, anomaly)
			}
			stagnationStart = -1
		}
	}

	return anomalies
}

func (ca *coverageAnalyzer) detectRegression(points []CoverageDataPoint) []*CoverageAnomaly {
	anomalies := make([]*CoverageAnomaly, 0)

	if len(points) < 2 {
		return anomalies
	}

	for i := 1; i < len(points); i++ {
		if points[i].TotalEdges < points[i-1].TotalEdges {
			lostCoverage := points[i-1].TotalEdges - points[i].TotalEdges
			anomaly := &CoverageAnomaly{
				Timestamp:       points[i].Timestamp,
				Type:            "regression",
				Severity:        "critical",
				Description:     fmt.Sprintf("Coverage regressed by %d edges", lostCoverage),
				AffectedMetrics: []string{"total_edges", "corpus_integrity"},
				Duration:        time.Until(points[i].Timestamp),
				Impact: AnomalyImpact{
					CoverageLost:     lostCoverage,
					EfficiencyImpact: -100.0, // Complete failure
				},
				PossibleCauses: []string{
					"Corpus data loss",
					"Storage corruption",
					"Incorrect corpus synchronization",
					"Database inconsistency",
				},
				Recommendations: []string{
					"IMMEDIATE ACTION REQUIRED",
					"Check corpus integrity",
					"Restore from backup if available",
					"Investigate storage system health",
					"Halt fuzzing until resolved",
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

// Utility methods

func (ca *coverageAnalyzer) averageGrowthRate(points []CoverageDataPoint) float64 {
	if len(points) == 0 {
		return 0
	}

	total := float64(0)
	for _, p := range points {
		total += p.GrowthRate
	}
	return total / float64(len(points))
}

func (ca *coverageAnalyzer) average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	total := float64(0)
	for _, v := range values {
		total += v
	}
	return total / float64(len(values))
}

func (ca *coverageAnalyzer) averageEfficiency(points []CoverageDataPoint) float64 {
	if len(points) == 0 {
		return 0
	}

	total := float64(0)
	count := 0
	for _, p := range points {
		if p.EfficiencyRatio > 0 {
			total += p.EfficiencyRatio
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func (ca *coverageAnalyzer) averageExecSpeed(points []CoverageDataPoint) float64 {
	if len(points) == 0 {
		return 0
	}

	total := float64(0)
	count := 0
	for _, p := range points {
		if p.ExecPerSec > 0 {
			total += p.ExecPerSec
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func (ca *coverageAnalyzer) getAllGrowthRates(points []CoverageDataPoint) []float64 {
	rates := make([]float64, 0, len(points))
	for _, p := range points {
		rates = append(rates, p.GrowthRate)
	}
	return rates
}

func (ca *coverageAnalyzer) findPeakTimes(points []CoverageDataPoint, pattern map[int]float64) []time.Time {
	peaks := make([]time.Time, 0)

	// Find hours with above-average growth
	avg := ca.averageGrowthRate(points)
	for hour, rate := range pattern {
		if rate > avg*1.2 {
			// Find actual timestamps for this hour
			for _, p := range points {
				if p.Timestamp.Hour() == hour && p.GrowthRate > avg*1.2 {
					peaks = append(peaks, p.Timestamp)
					break
				}
			}
		}
	}

	return peaks
}

func (ca *coverageAnalyzer) findTroughTimes(points []CoverageDataPoint, pattern map[int]float64) []time.Time {
	troughs := make([]time.Time, 0)

	// Find hours with below-average growth
	avg := ca.averageGrowthRate(points)
	for hour, rate := range pattern {
		if rate < avg*0.8 {
			// Find actual timestamps for this hour
			for _, p := range points {
				if p.Timestamp.Hour() == hour && p.GrowthRate < avg*0.8 {
					troughs = append(troughs, p.Timestamp)
					break
				}
			}
		}
	}

	return troughs
}

func (ca *coverageAnalyzer) estimateOptimalCorpusSize(points []CoverageDataPoint) int {
	if len(points) == 0 {
		return 1000 // Default
	}

	// Find corpus size at peak efficiency
	maxEfficiency := float64(0)
	optimalSize := 1000

	for _, p := range points {
		if p.EfficiencyRatio > maxEfficiency && p.CorpusSize > 0 {
			maxEfficiency = p.EfficiencyRatio
			optimalSize = p.CorpusSize
		}
	}

	// Add 20% buffer
	return int(float64(optimalSize) * 1.2)
}

func (ca *coverageAnalyzer) estimateCorpusDiversity(points []CoverageDataPoint) float64 {
	// Simplified diversity estimate based on coverage vs corpus size
	if len(points) == 0 {
		return 0
	}

	lastPoint := points[len(points)-1]
	if lastPoint.CorpusSize == 0 {
		return 0
	}

	// Higher ratio = better diversity
	ratio := float64(lastPoint.TotalEdges) / float64(lastPoint.CorpusSize)

	// Normalize to 0-1
	normalized := math.Min(ratio/100, 1.0)
	return normalized
}

func (ca *coverageAnalyzer) estimateDuplicationRate(points []CoverageDataPoint) float64 {
	// Estimate based on efficiency decline
	if len(points) < 10 {
		return 0
	}

	earlyEff := ca.averageEfficiency(points[:5])
	lateEff := ca.averageEfficiency(points[len(points)-5:])

	if earlyEff == 0 {
		return 0
	}

	decline := (earlyEff - lateEff) / earlyEff
	return math.Max(0, math.Min(decline, 1.0))
}

func (ca *coverageAnalyzer) checkDecayConsistency(points []CoverageDataPoint) float64 {
	// Check how consistent the growth decay pattern is
	if len(points) < 5 {
		return 0.5
	}

	decayRates := make([]float64, 0)
	for i := 2; i < len(points); i++ {
		if points[i-1].GrowthRate > 0 {
			decay := (points[i-1].GrowthRate - points[i].GrowthRate) / points[i-1].GrowthRate
			if decay > 0 && decay < 1 {
				decayRates = append(decayRates, decay)
			}
		}
	}

	if len(decayRates) < 2 {
		return 0.5
	}

	// Calculate standard deviation
	avg := ca.average(decayRates)
	variance := float64(0)
	for _, d := range decayRates {
		variance += math.Pow(d-avg, 2)
	}
	stdDev := math.Sqrt(variance / float64(len(decayRates)))

	// Lower standard deviation = more consistent
	consistency := 1.0 - math.Min(stdDev/avg, 1.0)
	return consistency
}

func (ca *coverageAnalyzer) calculateSeverity(actual, baseline float64) string {
	ratio := actual / baseline
	if ratio > 5 {
		return "high"
	} else if ratio > 3 {
		return "medium"
	}
	return "low"
}

func (ca *coverageAnalyzer) calculateDropSeverity(current, previous float64) string {
	if previous == 0 {
		return "low"
	}

	dropPercent := (previous - current) / previous * 100
	if dropPercent > 80 {
		return "critical"
	} else if dropPercent > 50 {
		return "high"
	} else if dropPercent > 30 {
		return "medium"
	}
	return "low"
}

func (ca *coverageAnalyzer) calculateStagnationSeverity(duration time.Duration) string {
	hours := duration.Hours()
	if hours > 12 {
		return "critical"
	} else if hours > 6 {
		return "high"
	} else if hours > 3 {
		return "medium"
	}
	return "low"
}

func (ca *coverageAnalyzer) calculateAnomalyDuration(points []CoverageDataPoint, startIdx int, anomalyType string) time.Duration {
	if startIdx >= len(points)-1 {
		return time.Hour // Default
	}

	// Find when anomaly ends
	endIdx := startIdx + 1
	for endIdx < len(points) {
		if anomalyType == "spike" && points[endIdx].GrowthRate < points[startIdx].GrowthRate/2 {
			break
		} else if anomalyType == "drop" && points[endIdx].GrowthRate > points[startIdx].GrowthRate*2 {
			break
		}
		endIdx++
	}

	if endIdx >= len(points) {
		endIdx = len(points) - 1
	}

	return points[endIdx].Timestamp.Sub(points[startIdx].Timestamp)
}

func (ca *coverageAnalyzer) estimateLostCoverage(points []CoverageDataPoint, dropIdx int) int64 {
	if dropIdx == 0 || dropIdx >= len(points) {
		return 0
	}

	// Estimate based on previous growth rate
	expectedGrowth := points[dropIdx-1].GrowthRate
	actualGrowth := points[dropIdx].GrowthRate
	lostGrowth := expectedGrowth - actualGrowth

	if lostGrowth <= 0 {
		return 0
	}

	// Calculate for the duration of the drop
	duration := time.Hour // Assuming hourly data points
	if dropIdx < len(points)-1 {
		duration = points[dropIdx+1].Timestamp.Sub(points[dropIdx].Timestamp)
	}

	return int64(lostGrowth * duration.Hours())
}

func (ca *coverageAnalyzer) calculateWastedResources(points []CoverageDataPoint, anomalyIdx int) float64 {
	if anomalyIdx >= len(points) {
		return 0
	}

	// Estimate based on execution count and low efficiency
	point := points[anomalyIdx]
	normalEfficiency := ca.averageEfficiency(points)

	if normalEfficiency == 0 || point.EfficiencyRatio >= normalEfficiency {
		return 0
	}

	// Wasted executions
	wastedExecs := float64(point.ExecCount) * (1 - point.EfficiencyRatio/normalEfficiency)

	// Convert to compute hours (assuming 1000 exec/sec per compute unit)
	computeHours := wastedExecs / 1000 / 3600
	return computeHours
}

func (ca *coverageAnalyzer) estimateLostOpportunity(points []CoverageDataPoint, start, end int) int64 {
	if start >= end || end > len(points) {
		return 0
	}

	// Calculate expected coverage based on baseline growth
	baselineGrowth := ca.averageGrowthRate(points)
	duration := points[end-1].Timestamp.Sub(points[start].Timestamp)

	expectedCoverage := int64(baselineGrowth * duration.Hours())
	actualCoverage := points[end-1].TotalEdges - points[start].TotalEdges

	if expectedCoverage > actualCoverage {
		return expectedCoverage - actualCoverage
	}
	return 0
}

func (ca *coverageAnalyzer) countActiveBots(points []CoverageDataPoint) int {
	if len(points) == 0 {
		return 0
	}

	// Use average active bots if available
	totalBots := 0
	count := 0
	for _, p := range points {
		if p.ActiveBots > 0 {
			totalBots += p.ActiveBots
			count++
		}
	}

	if count == 0 {
		return 1 // Default assumption
	}
	return totalBots / count
}
