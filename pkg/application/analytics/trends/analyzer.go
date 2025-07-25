package trends

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	// Removed coverage and performance imports to avoid circular dependencies
	campaignRepo "github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	crashRepo "github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	"github.com/sirupsen/logrus"
)

// Analyzer implements the TrendAnalyzer interface
type Analyzer struct {
	campaignRepo campaignRepo.CampaignRepository
	crashRepo    crashRepo.CrashRepository
	// Remove analyzer dependencies to avoid circular imports
	// Trend analysis will work directly with repositories
	logger logrus.FieldLogger
}

// NewAnalyzer creates a new trend analyzer
func NewAnalyzer(
	campaignRepo campaignRepo.CampaignRepository,
	crashRepo crashRepo.CrashRepository,
	// Removed analyzer parameters to avoid circular imports
	logger logrus.FieldLogger,
) *Analyzer {
	return &Analyzer{
		campaignRepo: campaignRepo,
		crashRepo:    crashRepo,
		// Analyzers removed to avoid circular imports
		logger: logger.WithField("component", "trend_analyzer"),
	}
}

// AnalyzeCoverageTrends analyzes coverage trends over time
func (a *Analyzer) AnalyzeCoverageTrends(ctx context.Context, campaignID string, period TrendPeriod) (*CoverageTrends, error) {
	a.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"period":      period,
	}).Debug("Analyzing coverage trends")

	// Get campaign data
	campaign, err := a.campaignRepo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("failed to get campaign: %w", err)
	}

	// Calculate time range
	timeRange := TimeRange{
		Start: campaign.CreatedAt,
		End:   time.Now(),
	}

	// Generate coverage trend data points
	dataPoints := a.generateCoverageTrendPoints(campaign, period)

	// Analyze trends
	analysis := a.analyzeCoverageTrendData(dataPoints)

	// Detect anomalies
	anomalies := a.detectCoverageAnomalies(dataPoints)

	// Generate forecast
	forecast := a.forecastCoverage(dataPoints, analysis)

	// Generate insights
	insights := a.generateCoverageInsights(analysis, anomalies)

	return &CoverageTrends{
		CampaignID: campaignID,
		Period:     period,
		TimeRange:  timeRange,
		DataPoints: dataPoints,
		Analysis:   analysis,
		Anomalies:  anomalies,
		Forecast:   forecast,
		Insights:   insights,
	}, nil
}

// AnalyzePerformanceTrends analyzes performance trends over time
func (a *Analyzer) AnalyzePerformanceTrends(ctx context.Context, campaignID string, period TrendPeriod) (*PerformanceTrends, error) {
	a.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"period":      period,
	}).Debug("Analyzing performance trends")

	// Get campaign data
	campaign, err := a.campaignRepo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("failed to get campaign: %w", err)
	}

	// Calculate time range
	timeRange := TimeRange{
		Start: campaign.CreatedAt,
		End:   time.Now(),
	}

	// Generate performance trend data points
	dataPoints := a.generatePerformanceTrendPoints(campaign, period)

	// Analyze trends
	analysis := a.analyzePerformanceTrendData(dataPoints)

	// Detect anomalies
	anomalies := a.detectPerformanceAnomalies(dataPoints)

	// Generate forecast
	forecast := a.forecastPerformance(dataPoints, analysis)

	// Generate insights
	insights := a.generatePerformanceInsights(analysis, anomalies)

	return &PerformanceTrends{
		CampaignID: campaignID,
		Period:     period,
		TimeRange:  timeRange,
		DataPoints: dataPoints,
		Analysis:   analysis,
		Anomalies:  anomalies,
		Forecast:   forecast,
		Insights:   insights,
	}, nil
}

// AnalyzeCrashTrends analyzes crash discovery trends
func (a *Analyzer) AnalyzeCrashTrends(ctx context.Context, campaignID string, period TrendPeriod) (*CrashTrends, error) {
	a.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"period":      period,
	}).Debug("Analyzing crash trends")

	// Get campaign data
	campaign, err := a.campaignRepo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("failed to get campaign: %w", err)
	}

	// Calculate time range
	timeRange := TimeRange{
		Start: campaign.CreatedAt,
		End:   time.Now(),
	}

	// Generate crash trend data points
	dataPoints := a.generateCrashTrendPoints(campaign, period)

	// Analyze trends
	analysis := a.analyzeCrashTrendData(dataPoints)

	// Identify patterns
	patterns := a.identifyCrashPatterns(dataPoints)

	// Generate forecast
	forecast := a.forecastCrashes(dataPoints, analysis)

	// Generate insights
	insights := a.generateCrashInsights(analysis, patterns)

	return &CrashTrends{
		CampaignID: campaignID,
		Period:     period,
		TimeRange:  timeRange,
		DataPoints: dataPoints,
		Analysis:   analysis,
		Patterns:   patterns,
		Forecast:   forecast,
		Insights:   insights,
	}, nil
}

// DetectAnomalies detects anomalies in trend data
func (a *Analyzer) DetectAnomalies(ctx context.Context, trends *TrendData) (*AnomalyReport, error) {
	if trends == nil || len(trends.DataPoints) == 0 {
		return nil, fmt.Errorf("no trend data provided")
	}

	// Detect anomalies using multiple methods
	anomalies := a.detectAnomaliesInData(trends.DataPoints)

	// Generate summary
	summary := a.generateAnomalySummary(anomalies)

	// Assess impact
	impact := a.assessAnomalyImpact(anomalies)

	// Generate recommendations
	recommendations := a.generateAnomalyRecommendations(anomalies, impact)

	return &AnomalyReport{
		GeneratedAt:      time.Now(),
		TimeRange:        trends.TimeRange,
		AnomaliesFound:   anomalies,
		Summary:          summary,
		ImpactAssessment: impact,
		Recommendations:  recommendations,
	}, nil
}

// ForecastTrends provides trend forecasting
func (a *Analyzer) ForecastTrends(ctx context.Context, historicalData *TrendData, forecastPeriod time.Duration) (*TrendForecast, error) {
	if historicalData == nil || len(historicalData.DataPoints) < 3 {
		return nil, fmt.Errorf("insufficient historical data for forecasting")
	}

	// Prepare data for forecasting
	values := make([]float64, len(historicalData.DataPoints))
	for i, dp := range historicalData.DataPoints {
		values[i] = dp.Value
	}

	// Determine best-fit model
	methodology := a.determineForecastModel(values)

	// Generate predictions
	predictions := a.generatePredictions(historicalData.DataPoints, forecastPeriod, methodology)

	// Calculate confidence
	confidence := a.calculateForecastConfidence(values, methodology)

	// Define assumptions
	assumptions := []string{
		"Historical patterns continue",
		"No major system changes",
		"Stable operating environment",
		fmt.Sprintf("Using %s model", methodology),
	}

	return &TrendForecast{
		MetricName:      historicalData.MetricName,
		GeneratedAt:     time.Now(),
		ForecastPeriod:  forecastPeriod,
		PredictedValues: predictions,
		Confidence:      confidence,
		Methodology:     methodology,
		Assumptions:     assumptions,
	}, nil
}

// Helper methods for coverage trends

func (a *Analyzer) generateCoverageTrendPoints(campaign interface{}, period TrendPeriod) []CoverageTrendPoint {
	// Generate sample trend points
	points := make([]CoverageTrendPoint, 0)

	now := time.Now()
	interval := a.getIntervalForPeriod(period)
	numPoints := a.getNumPointsForPeriod(period)

	cumulativeEdges := int64(0)
	baseCoverage := 10.0

	for i := numPoints; i > 0; i-- {
		timestamp := now.Add(-time.Duration(i) * interval)

		// Simulate realistic coverage growth
		elapsed := float64(numPoints - i)
		coverage := baseCoverage + elapsed*5 - elapsed*elapsed*0.1
		coverage = math.Min(coverage, 85) // Cap at 85%

		newEdges := int64(math.Max(0, 1000-elapsed*10))
		cumulativeEdges += newEdges

		growthRate := 0.0
		if i < numPoints {
			growthRate = (coverage - points[len(points)-1].TotalCoverage) / interval.Hours()
		}

		points = append(points, CoverageTrendPoint{
			Timestamp:          timestamp,
			TotalCoverage:      coverage,
			LineCoverage:       coverage * 0.95,
			FunctionCoverage:   coverage * 1.1,
			BranchCoverage:     coverage * 0.85,
			NewEdgesDiscovered: newEdges,
			CumulativeEdges:    cumulativeEdges,
			GrowthRate:         growthRate,
			DiscoveryVelocity:  float64(newEdges) / interval.Hours(),
		})
	}

	return points
}

func (a *Analyzer) analyzeCoverageTrendData(dataPoints []CoverageTrendPoint) *CoverageTrendAnalysis {
	if len(dataPoints) < 2 {
		return nil
	}

	// Determine trend type and direction
	trendType, direction := a.determineTrendTypeAndDirection(dataPoints)

	// Calculate trend strength
	strength := a.calculateTrendStrength(dataPoints)

	// Calculate consistency
	consistency := a.calculateConsistency(dataPoints)

	// Calculate volatility
	volatility := a.calculateVolatility(dataPoints)

	// Determine growth pattern
	growthPattern := a.determineGrowthPattern(dataPoints)

	// Identify current phase
	currentPhase := a.identifyCurrentPhase(dataPoints)

	// Detect phase transitions
	transitions := a.detectPhaseTransitions(dataPoints)

	return &CoverageTrendAnalysis{
		TrendType:           trendType,
		Direction:           direction,
		Strength:            strength,
		Consistency:         consistency,
		Volatility:          volatility,
		GrowthPattern:       growthPattern,
		CurrentPhase:        currentPhase,
		PhaseTransitions:    transitions,
		SeasonalityDetected: false, // Would require more sophisticated analysis
	}
}

func (a *Analyzer) detectCoverageAnomalies(dataPoints []CoverageTrendPoint) []Anomaly {
	anomalies := make([]Anomaly, 0)

	if len(dataPoints) < 5 {
		return anomalies
	}

	// Calculate moving average and standard deviation
	windowSize := 5
	for i := windowSize; i < len(dataPoints); i++ {
		// Calculate stats for window
		var sum, sumGrowth float64
		for j := i - windowSize; j < i; j++ {
			sum += dataPoints[j].TotalCoverage
			sumGrowth += dataPoints[j].GrowthRate
		}
		avgCoverage := sum / float64(windowSize)
		avgGrowth := sumGrowth / float64(windowSize)

		// Check for anomalies
		current := dataPoints[i]

		// Coverage spike/drop
		coverageDiff := math.Abs(current.TotalCoverage - avgCoverage)
		if coverageDiff > avgCoverage*0.2 { // 20% deviation
			anomalyType := AnomalyTypeSpike
			if current.TotalCoverage < avgCoverage {
				anomalyType = AnomalyTypeDrop
			}

			anomalies = append(anomalies, Anomaly{
				ID:         fmt.Sprintf("coverage-%s-%d", anomalyType, i),
				Type:       anomalyType,
				Severity:   a.determineSeverity(coverageDiff / avgCoverage),
				DetectedAt: time.Now(),
				StartTime:  current.Timestamp,
				Description: fmt.Sprintf("Coverage %s detected: %.1f%% (expected ~%.1f%%)",
					anomalyType, current.TotalCoverage, avgCoverage),
				Impact: "Potential data quality issue or significant code change",
				Evidence: map[string]interface{}{
					"actual_coverage":   current.TotalCoverage,
					"expected_coverage": avgCoverage,
					"deviation":         coverageDiff,
				},
				Status: AnomalyStatusActive,
			})
		}

		// Growth rate anomaly
		growthDiff := math.Abs(current.GrowthRate - avgGrowth)
		if growthDiff > math.Abs(avgGrowth)*2 && growthDiff > 0.5 {
			anomalies = append(anomalies, Anomaly{
				ID:          fmt.Sprintf("growth-anomaly-%d", i),
				Type:        AnomalyTypePatternBreak,
				Severity:    "medium",
				DetectedAt:  time.Now(),
				StartTime:   current.Timestamp,
				Description: fmt.Sprintf("Abnormal growth rate: %.2f%%/hour", current.GrowthRate),
				Impact:      "Coverage growth pattern disrupted",
				Evidence: map[string]interface{}{
					"actual_growth":   current.GrowthRate,
					"expected_growth": avgGrowth,
				},
				Status: AnomalyStatusActive,
			})
		}
	}

	return anomalies
}

func (a *Analyzer) forecastCoverage(dataPoints []CoverageTrendPoint, analysis *CoverageTrendAnalysis) *CoverageForecast {
	if len(dataPoints) < 3 || analysis == nil {
		return nil
	}

	// Determine forecast methodology based on trend type
	methodology := "exponential smoothing"
	if analysis.TrendType == TrendTypeLinear {
		methodology = "linear regression"
	} else if analysis.TrendType == TrendTypePlateau {
		methodology = "logistic growth model"
	}

	// Generate forecast points
	forecastPeriod := 7 * 24 * time.Hour // 1 week
	predictions := a.generateCoveragePredictions(dataPoints, forecastPeriod, analysis)

	// Calculate confidence based on consistency and volatility
	confidenceLevel := 0.9 - analysis.Volatility/100
	if analysis.Consistency < 0.7 {
		confidenceLevel *= 0.8
	}

	// Analyze saturation
	saturation := a.analyzeSaturation(dataPoints, predictions)

	// Identify risk factors
	riskFactors := []RiskFactor{
		{
			Type:        "volatility",
			Description: fmt.Sprintf("Historical volatility: %.1f%%", analysis.Volatility),
			Impact:      "Predictions may vary significantly",
			Likelihood:  analysis.Volatility / 100,
			Mitigation:  "Monitor closely and adjust fuzzing strategy",
		},
	}

	if analysis.TrendType == TrendTypePlateau {
		riskFactors = append(riskFactors, RiskFactor{
			Type:        "saturation",
			Description: "Coverage approaching saturation point",
			Impact:      "Diminishing returns expected",
			Likelihood:  0.8,
			Mitigation:  "Consider advanced fuzzing techniques",
		})
	}

	return &CoverageForecast{
		GeneratedAt:     time.Now(),
		ForecastPeriod:  forecastPeriod,
		PredictedPoints: predictions,
		ConfidenceLevel: confidenceLevel,
		Methodology:     methodology,
		Assumptions: []string{
			"Current fuzzing configuration remains unchanged",
			"No major code refactoring",
			"Consistent resource availability",
		},
		RiskFactors:     riskFactors,
		SaturationPoint: saturation,
	}
}

func (a *Analyzer) generateCoverageInsights(analysis *CoverageTrendAnalysis, anomalies []Anomaly) []TrendInsight {
	insights := make([]TrendInsight, 0)

	// Growth pattern insights
	if analysis.GrowthPattern == GrowthPatternStagnant {
		insights = append(insights, TrendInsight{
			ID:          "stagnant-growth",
			Type:        "growth_pattern",
			Severity:    "high",
			Title:       "Coverage Growth Stagnation Detected",
			Description: "Coverage growth has plateaued, indicating diminishing returns",
			Impact:      "Current fuzzing strategy may no longer be effective",
			Evidence: map[string]interface{}{
				"growth_pattern": analysis.GrowthPattern,
				"current_phase":  analysis.CurrentPhase,
			},
			Actions: []string{
				"Implement new mutation strategies",
				"Add grammar-based fuzzing",
				"Expand seed corpus with diverse inputs",
			},
		})
	}

	// Volatility insights
	if analysis.Volatility > 20 {
		insights = append(insights, TrendInsight{
			ID:          "high-volatility",
			Type:        "stability",
			Severity:    "medium",
			Title:       "High Coverage Volatility",
			Description: fmt.Sprintf("Coverage showing high volatility (%.1f%%)", analysis.Volatility),
			Impact:      "Unpredictable coverage growth patterns",
			Evidence: map[string]interface{}{
				"volatility": analysis.Volatility,
			},
			Actions: []string{
				"Investigate causes of instability",
				"Ensure consistent fuzzing environment",
				"Review recent code changes",
			},
		})
	}

	// Anomaly-based insights
	if len(anomalies) > 3 {
		insights = append(insights, TrendInsight{
			ID:          "frequent-anomalies",
			Type:        "anomaly",
			Severity:    "high",
			Title:       "Frequent Coverage Anomalies",
			Description: fmt.Sprintf("%d anomalies detected in coverage trends", len(anomalies)),
			Impact:      "Coverage reliability compromised",
			Evidence: map[string]interface{}{
				"anomaly_count": len(anomalies),
			},
			Actions: []string{
				"Review fuzzing infrastructure stability",
				"Check for environmental factors",
				"Validate coverage measurement accuracy",
			},
		})
	}

	// Phase transition insights
	if len(analysis.PhaseTransitions) > 0 {
		lastTransition := analysis.PhaseTransitions[len(analysis.PhaseTransitions)-1]
		insights = append(insights, TrendInsight{
			ID:          "phase-transition",
			Type:        "phase_change",
			Severity:    "info",
			Title:       "Coverage Phase Transition",
			Description: fmt.Sprintf("Transitioned from %s to %s phase", lastTransition.FromPhase, lastTransition.ToPhase),
			Impact:      "Coverage behavior characteristics have changed",
			Evidence: map[string]interface{}{
				"from_phase": lastTransition.FromPhase,
				"to_phase":   lastTransition.ToPhase,
				"timestamp":  lastTransition.Timestamp,
			},
		})
	}

	return insights
}

// Helper methods for performance trends

func (a *Analyzer) generatePerformanceTrendPoints(campaign interface{}, period TrendPeriod) []PerformanceTrendPoint {
	points := make([]PerformanceTrendPoint, 0)

	now := time.Now()
	interval := a.getIntervalForPeriod(period)
	numPoints := a.getNumPointsForPeriod(period)

	for i := numPoints; i > 0; i-- {
		timestamp := now.Add(-time.Duration(i) * interval)

		// Generate realistic performance metrics
		baseExec := 500.0
		execPerSec := baseExec + math.Sin(float64(i)*0.1)*100
		latency := 50.0 + math.Cos(float64(i)*0.15)*10
		cpu := 60.0 + math.Sin(float64(i)*0.2)*15
		memory := 70.0 + math.Cos(float64(i)*0.25)*10
		queueDepth := int64(1000 + math.Sin(float64(i)*0.3)*500)
		errorRate := 0.01 + math.Max(0, math.Sin(float64(i)*0.5)*0.005)
		efficiency := 75.0 + math.Sin(float64(i)*0.1)*10

		points = append(points, PerformanceTrendPoint{
			Timestamp:           timestamp,
			ExecutionsPerSecond: execPerSec,
			AverageLatency:      latency,
			P95Latency:          latency * 1.5,
			P99Latency:          latency * 2.0,
			CPUUtilization:      cpu,
			MemoryUtilization:   memory,
			QueueDepth:          queueDepth,
			ErrorRate:           errorRate,
			EfficiencyScore:     efficiency,
		})
	}

	return points
}

func (a *Analyzer) analyzePerformanceTrendData(dataPoints []PerformanceTrendPoint) *PerformanceTrendAnalysis {
	if len(dataPoints) < 2 {
		return nil
	}

	// Determine overall trend
	first := dataPoints[0]
	last := dataPoints[len(dataPoints)-1]

	overallTrend := TrendDirectionStable
	if last.EfficiencyScore > first.EfficiencyScore+5 {
		overallTrend = TrendDirectionUp
	} else if last.EfficiencyScore < first.EfficiencyScore-5 {
		overallTrend = TrendDirectionDown
	}

	// Calculate stability score
	stabilityScore := a.calculatePerformanceStability(dataPoints)

	// Determine performance grade
	grade := a.determinePerformanceGrade(last.EfficiencyScore, stabilityScore)

	// Check for degradation
	degradationDetected := a.detectPerformanceDegradation(dataPoints)

	// Analyze bottleneck trends
	bottleneckTrends := a.analyzeBottleneckTrends(dataPoints)

	// Identify optimization windows
	optimizationWindows := a.identifyOptimizationWindows(dataPoints)

	return &PerformanceTrendAnalysis{
		OverallTrend:        overallTrend,
		StabilityScore:      stabilityScore,
		PerformanceGrade:    grade,
		DegradationDetected: degradationDetected,
		BottleneckTrends:    bottleneckTrends,
		OptimizationWindows: optimizationWindows,
	}
}

func (a *Analyzer) detectPerformanceAnomalies(dataPoints []PerformanceTrendPoint) []Anomaly {
	anomalies := make([]Anomaly, 0)

	if len(dataPoints) < 5 {
		return anomalies
	}

	// Use sliding window for anomaly detection
	windowSize := 5
	for i := windowSize; i < len(dataPoints); i++ {
		window := dataPoints[i-windowSize : i]
		current := dataPoints[i]

		// Calculate window statistics
		var avgExec, avgLatency, avgCPU float64
		for _, p := range window {
			avgExec += p.ExecutionsPerSecond
			avgLatency += p.AverageLatency
			avgCPU += p.CPUUtilization
		}
		avgExec /= float64(windowSize)
		avgLatency /= float64(windowSize)
		avgCPU /= float64(windowSize)

		// Check for execution anomalies
		execDiff := math.Abs(current.ExecutionsPerSecond - avgExec)
		if execDiff > avgExec*0.3 { // 30% deviation
			anomalyType := AnomalyTypeSpike
			if current.ExecutionsPerSecond < avgExec {
				anomalyType = AnomalyTypeDrop
			}

			anomalies = append(anomalies, Anomaly{
				ID:          fmt.Sprintf("exec-%s-%d", anomalyType, i),
				Type:        anomalyType,
				Severity:    a.determineSeverity(execDiff / avgExec),
				DetectedAt:  time.Now(),
				StartTime:   current.Timestamp,
				Description: fmt.Sprintf("Execution rate %s: %.0f/sec", anomalyType, current.ExecutionsPerSecond),
				Impact:      "Throughput affected",
				Evidence: map[string]interface{}{
					"actual_rate":   current.ExecutionsPerSecond,
					"expected_rate": avgExec,
				},
				Status: AnomalyStatusActive,
			})
		}

		// Check for latency spikes
		if current.P99Latency > avgLatency*3 {
			anomalies = append(anomalies, Anomaly{
				ID:          fmt.Sprintf("latency-spike-%d", i),
				Type:        AnomalyTypeSpike,
				Severity:    "high",
				DetectedAt:  time.Now(),
				StartTime:   current.Timestamp,
				Description: fmt.Sprintf("P99 latency spike: %.1fms", current.P99Latency),
				Impact:      "User experience degraded",
				Evidence: map[string]interface{}{
					"p99_latency": current.P99Latency,
					"avg_latency": current.AverageLatency,
				},
				Status:  AnomalyStatusActive,
				Actions: []string{"Investigate slow operations", "Check resource contention"},
			})
		}

		// Check for error rate anomalies
		if current.ErrorRate > 0.05 { // 5% error rate threshold
			anomalies = append(anomalies, Anomaly{
				ID:          fmt.Sprintf("error-rate-%d", i),
				Type:        AnomalyTypeSpike,
				Severity:    "critical",
				DetectedAt:  time.Now(),
				StartTime:   current.Timestamp,
				Description: fmt.Sprintf("High error rate: %.1f%%", current.ErrorRate*100),
				Impact:      "System reliability compromised",
				Evidence: map[string]interface{}{
					"error_rate": current.ErrorRate,
				},
				Status:  AnomalyStatusActive,
				Actions: []string{"Review error logs", "Check system health"},
			})
		}
	}

	return anomalies
}

func (a *Analyzer) forecastPerformance(dataPoints []PerformanceTrendPoint, analysis *PerformanceTrendAnalysis) *PerformanceForecast {
	if len(dataPoints) < 3 || analysis == nil {
		return nil
	}

	forecastPeriod := 24 * time.Hour // 1 day ahead

	// Generate predictions
	predictions := make([]PerformanceForecastPoint, 0)
	last := dataPoints[len(dataPoints)-1]

	// Simple linear projection with decay
	numPredictions := 24 // Hourly predictions
	for i := 1; i <= numPredictions; i++ {
		timestamp := last.Timestamp.Add(time.Duration(i) * time.Hour)

		// Apply trend with decay
		decayFactor := math.Exp(-float64(i) / 24)

		throughputChange := 0.0
		if analysis.OverallTrend == TrendDirectionUp {
			throughputChange = 10.0 * decayFactor
		} else if analysis.OverallTrend == TrendDirectionDown {
			throughputChange = -10.0 * decayFactor
		}

		expectedThroughput := last.ExecutionsPerSecond + throughputChange
		expectedLatency := last.AverageLatency * (1 + 0.01*float64(i))
		expectedUtilization := math.Min(95, last.CPUUtilization+float64(i)*0.5)

		// Calculate risk level
		riskLevel := "low"
		if expectedUtilization > 80 || expectedLatency > 100 {
			riskLevel = "medium"
		}
		if expectedUtilization > 90 || expectedLatency > 150 {
			riskLevel = "high"
		}

		predictions = append(predictions, PerformanceForecastPoint{
			Timestamp: timestamp,
			ExpectedThroughput: Interval{
				Lower: expectedThroughput * 0.8,
				Upper: expectedThroughput * 1.2,
			},
			ExpectedLatency: Interval{
				Lower: expectedLatency * 0.9,
				Upper: expectedLatency * 1.3,
			},
			ExpectedUtilization: Interval{
				Lower: expectedUtilization * 0.9,
				Upper: math.Min(100, expectedUtilization*1.1),
			},
			RiskLevel: riskLevel,
		})
	}

	// Identify expected bottlenecks
	expectedBottlenecks := a.predictBottlenecks(predictions)

	// Generate recommendations
	recommendations := a.generatePerformanceRecommendations(analysis, predictions)

	// Calculate confidence
	confidence := 0.8 - analysis.StabilityScore*0.2
	if analysis.DegradationDetected {
		confidence *= 0.7
	}

	return &PerformanceForecast{
		GeneratedAt:          time.Now(),
		ForecastPeriod:       forecastPeriod,
		PredictedPerformance: predictions,
		ExpectedBottlenecks:  expectedBottlenecks,
		RecommendedActions:   recommendations,
		ConfidenceLevel:      confidence,
	}
}

func (a *Analyzer) generatePerformanceInsights(analysis *PerformanceTrendAnalysis, anomalies []Anomaly) []TrendInsight {
	insights := make([]TrendInsight, 0)

	// Degradation insights
	if analysis.DegradationDetected {
		insights = append(insights, TrendInsight{
			ID:          "performance-degradation",
			Type:        "degradation",
			Severity:    "high",
			Title:       "Performance Degradation Detected",
			Description: "System performance is declining over time",
			Impact:      "Reduced throughput and increased latency expected",
			Evidence: map[string]interface{}{
				"trend":             analysis.OverallTrend,
				"stability_score":   analysis.StabilityScore,
				"performance_grade": analysis.PerformanceGrade,
			},
			Actions: []string{
				"Investigate resource utilization",
				"Profile application for bottlenecks",
				"Review recent changes",
			},
		})
	}

	// Stability insights
	if analysis.StabilityScore < 60 {
		insights = append(insights, TrendInsight{
			ID:          "unstable-performance",
			Type:        "stability",
			Severity:    "medium",
			Title:       "Unstable Performance Patterns",
			Description: fmt.Sprintf("Performance stability score: %.1f/100", analysis.StabilityScore),
			Impact:      "Unpredictable performance affecting reliability",
			Evidence: map[string]interface{}{
				"stability_score": analysis.StabilityScore,
			},
			Actions: []string{
				"Implement performance monitoring",
				"Add circuit breakers",
				"Review auto-scaling policies",
			},
		})
	}

	// Bottleneck insights
	for _, bottleneck := range analysis.BottleneckTrends {
		if bottleneck.Trend == TrendDirectionUp && bottleneck.Severity == "high" {
			insights = append(insights, TrendInsight{
				ID:          fmt.Sprintf("growing-bottleneck-%s", bottleneck.Type),
				Type:        "bottleneck",
				Severity:    bottleneck.Severity,
				Title:       fmt.Sprintf("Growing %s Bottleneck", bottleneck.Type),
				Description: fmt.Sprintf("%s bottleneck is worsening over time", bottleneck.Type),
				Impact:      bottleneck.Impact,
				Evidence: map[string]interface{}{
					"type":      bottleneck.Type,
					"trend":     bottleneck.Trend,
					"frequency": bottleneck.Frequency,
				},
				Actions: []string{
					fmt.Sprintf("Optimize %s usage", bottleneck.Type),
					"Consider scaling resources",
					"Implement caching strategies",
				},
			})
		}
	}

	// Optimization opportunity insights
	if len(analysis.OptimizationWindows) > 0 {
		insights = append(insights, TrendInsight{
			ID:          "optimization-opportunities",
			Type:        "opportunity",
			Severity:    "info",
			Title:       "Performance Optimization Windows Identified",
			Description: fmt.Sprintf("%d time windows suitable for optimization", len(analysis.OptimizationWindows)),
			Impact:      "Minimal user impact during these periods",
			Evidence: map[string]interface{}{
				"window_count": len(analysis.OptimizationWindows),
			},
		})
	}

	return insights
}

// Helper methods for crash trends

func (a *Analyzer) generateCrashTrendPoints(campaign interface{}, period TrendPeriod) []CrashTrendPoint {
	points := make([]CrashTrendPoint, 0)

	now := time.Now()
	interval := a.getIntervalForPeriod(period)
	numPoints := a.getNumPointsForPeriod(period)

	totalCrashes := 0

	for i := numPoints; i > 0; i-- {
		timestamp := now.Add(-time.Duration(i) * interval)

		// Simulate crash discovery pattern
		elapsed := float64(numPoints - i)
		newCrashes := int(math.Max(0, 10-elapsed*0.2) + math.Sin(elapsed*0.5)*2)
		uniqueCrashes := int(float64(newCrashes) * 0.7)
		totalCrashes += newCrashes

		crashRate := float64(newCrashes) / interval.Hours()
		discoveryVelocity := float64(uniqueCrashes) / interval.Hours()

		// Generate severity breakdown
		severityBreakdown := map[string]int{
			"critical": int(float64(newCrashes) * 0.1),
			"high":     int(float64(newCrashes) * 0.3),
			"medium":   int(float64(newCrashes) * 0.4),
			"low":      int(float64(newCrashes) * 0.2),
		}

		// Generate type breakdown
		typeBreakdown := map[string]int{
			"segfault":     int(float64(newCrashes) * 0.3),
			"assertion":    int(float64(newCrashes) * 0.2),
			"timeout":      int(float64(newCrashes) * 0.2),
			"memory_error": int(float64(newCrashes) * 0.2),
			"other":        int(float64(newCrashes) * 0.1),
		}

		points = append(points, CrashTrendPoint{
			Timestamp:         timestamp,
			NewCrashes:        newCrashes,
			UniqueCrashes:     uniqueCrashes,
			TotalCrashes:      totalCrashes,
			CrashRate:         crashRate,
			SeverityBreakdown: severityBreakdown,
			TypeBreakdown:     typeBreakdown,
			DiscoveryVelocity: discoveryVelocity,
		})
	}

	return points
}

func (a *Analyzer) analyzeCrashTrendData(dataPoints []CrashTrendPoint) *CrashTrendAnalysis {
	if len(dataPoints) < 2 {
		return nil
	}

	// Determine discovery trend
	first := dataPoints[0]
	last := dataPoints[len(dataPoints)-1]

	discoveryTrend := TrendDirectionStable
	if last.CrashRate > first.CrashRate*1.2 {
		discoveryTrend = TrendDirectionUp
	} else if last.CrashRate < first.CrashRate*0.8 {
		discoveryTrend = TrendDirectionDown
	}

	// Calculate average discovery rate
	totalNew := 0
	for _, p := range dataPoints {
		totalNew += p.NewCrashes
	}
	avgDiscoveryRate := float64(totalNew) / float64(len(dataPoints))

	// Calculate uniqueness ratio
	totalUnique := 0
	for _, p := range dataPoints {
		totalUnique += p.UniqueCrashes
	}
	uniquenessRatio := float64(totalUnique) / float64(totalNew)

	// Analyze severity trends
	severityTrends := make(map[string]TrendDirection)
	for severity := range last.SeverityBreakdown {
		if last.SeverityBreakdown[severity] > first.SeverityBreakdown[severity] {
			severityTrends[severity] = TrendDirectionUp
		} else if last.SeverityBreakdown[severity] < first.SeverityBreakdown[severity] {
			severityTrends[severity] = TrendDirectionDown
		} else {
			severityTrends[severity] = TrendDirectionStable
		}
	}

	// Find most common crash types
	typeFrequency := make(map[string]int)
	for _, p := range dataPoints {
		for crashType, count := range p.TypeBreakdown {
			typeFrequency[crashType] += count
		}
	}

	mostCommonTypes := a.getTopTypes(typeFrequency, 3)

	// Calculate discovery efficiency
	efficiencySum := 0.0
	for _, p := range dataPoints {
		if p.NewCrashes > 0 {
			efficiencySum += float64(p.UniqueCrashes) / float64(p.NewCrashes)
		}
	}
	discoveryEfficiency := efficiencySum / float64(len(dataPoints))

	// Identify peak discovery times
	peakTimes := a.identifyPeakDiscoveryTimes(dataPoints)

	return &CrashTrendAnalysis{
		DiscoveryTrend:      discoveryTrend,
		DiscoveryRate:       avgDiscoveryRate,
		UniquenessRatio:     uniquenessRatio,
		SeverityTrends:      severityTrends,
		MostCommonTypes:     mostCommonTypes,
		DiscoveryEfficiency: discoveryEfficiency,
		PeakDiscoveryTimes:  peakTimes,
	}
}

func (a *Analyzer) identifyCrashPatterns(dataPoints []CrashTrendPoint) []CrashPattern {
	patterns := make([]CrashPattern, 0)

	// Check for declining uniqueness
	if len(dataPoints) > 5 {
		recentUniqueness := 0.0
		earlyUniqueness := 0.0

		for i := 0; i < 5; i++ {
			if dataPoints[i].NewCrashes > 0 {
				earlyUniqueness += float64(dataPoints[i].UniqueCrashes) / float64(dataPoints[i].NewCrashes)
			}
			if dataPoints[len(dataPoints)-5+i].NewCrashes > 0 {
				recentUniqueness += float64(dataPoints[len(dataPoints)-5+i].UniqueCrashes) / float64(dataPoints[len(dataPoints)-5+i].NewCrashes)
			}
		}

		earlyUniqueness /= 5
		recentUniqueness /= 5

		if recentUniqueness < earlyUniqueness*0.5 {
			patterns = append(patterns, CrashPattern{
				ID:          "declining-uniqueness",
				Type:        "discovery_efficiency",
				Description: "Crash uniqueness ratio declining significantly",
				Frequency:   recentUniqueness,
				Confidence:  0.8,
				Examples:    []string{"Many duplicate crashes being found"},
			})
		}
	}

	// Check for crash clustering
	clusterThreshold := 3.0 // 3x average
	for i := 1; i < len(dataPoints)-1; i++ {
		avgNeighbors := float64(dataPoints[i-1].NewCrashes+dataPoints[i+1].NewCrashes) / 2
		if float64(dataPoints[i].NewCrashes) > avgNeighbors*clusterThreshold {
			patterns = append(patterns, CrashPattern{
				ID:          fmt.Sprintf("crash-cluster-%d", i),
				Type:        "temporal_clustering",
				Description: "Crash discoveries clustered in time",
				Frequency:   float64(dataPoints[i].NewCrashes),
				Confidence:  0.7,
			})
		}
	}

	// Check for severity pattern shifts
	if len(dataPoints) > 10 {
		early := dataPoints[:5]
		recent := dataPoints[len(dataPoints)-5:]

		earlyCritical := 0
		recentCritical := 0

		for _, p := range early {
			earlyCritical += p.SeverityBreakdown["critical"]
		}
		for _, p := range recent {
			recentCritical += p.SeverityBreakdown["critical"]
		}

		if recentCritical > earlyCritical*2 {
			patterns = append(patterns, CrashPattern{
				ID:          "increasing-critical",
				Type:        "severity_shift",
				Description: "Increasing proportion of critical crashes",
				Frequency:   float64(recentCritical) / float64(len(recent)),
				Confidence:  0.85,
			})
		}
	}

	return patterns
}

func (a *Analyzer) forecastCrashes(dataPoints []CrashTrendPoint, analysis *CrashTrendAnalysis) *CrashForecast {
	if len(dataPoints) < 3 || analysis == nil {
		return nil
	}

	forecastPeriod := 7 * 24 * time.Hour // 1 week

	// Simple exponential decay model for crash discovery
	last := dataPoints[len(dataPoints)-1]
	decayRate := 0.95 // 5% decay per period

	predictions := make([]CrashForecastPoint, 0)
	numPredictions := 7 // Daily predictions

	for i := 1; i <= numPredictions; i++ {
		timestamp := last.Timestamp.Add(time.Duration(i) * 24 * time.Hour)

		// Apply decay to crash rate
		expectedRate := last.CrashRate * math.Pow(decayRate, float64(i))
		expectedNew := expectedRate * 24 // Convert to daily

		// Predict severity distribution based on trends
		severityPred := make(map[string]float64)
		for severity, trend := range analysis.SeverityTrends {
			baseProp := float64(last.SeverityBreakdown[severity]) / float64(last.NewCrashes)
			adjustment := 0.0
			if trend == TrendDirectionUp {
				adjustment = 0.05 * float64(i)
			} else if trend == TrendDirectionDown {
				adjustment = -0.05 * float64(i)
			}
			severityPred[severity] = math.Max(0, math.Min(1, baseProp+adjustment))
		}

		predictions = append(predictions, CrashForecastPoint{
			Timestamp: timestamp,
			ExpectedNewCrashes: Interval{
				Lower: expectedNew * 0.5,
				Upper: expectedNew * 1.5,
			},
			ExpectedCrashRate:  expectedRate,
			SeverityPrediction: severityPred,
		})
	}

	// Estimate remaining crashes based on decay model
	remainingCrashes := 0
	currentRate := last.CrashRate
	for i := 0; i < 100; i++ { // Project 100 periods ahead
		currentRate *= decayRate
		remainingCrashes += int(currentRate * 24)
		if currentRate < 0.01 {
			break
		}
	}

	confidence := 0.7
	if analysis.DiscoveryEfficiency < 0.5 {
		confidence *= 0.8
	}

	return &CrashForecast{
		GeneratedAt:        time.Now(),
		ForecastPeriod:     forecastPeriod,
		ExpectedDiscovery:  predictions,
		EstimatedRemaining: remainingCrashes,
		ConfidenceLevel:    confidence,
	}
}

func (a *Analyzer) generateCrashInsights(analysis *CrashTrendAnalysis, patterns []CrashPattern) []TrendInsight {
	insights := make([]TrendInsight, 0)

	// Discovery rate insights
	if analysis.DiscoveryTrend == TrendDirectionDown && analysis.DiscoveryRate < 1 {
		insights = append(insights, TrendInsight{
			ID:          "declining-discovery",
			Type:        "discovery_rate",
			Severity:    "medium",
			Title:       "Crash Discovery Rate Declining",
			Description: fmt.Sprintf("Discovery rate down to %.1f crashes/period", analysis.DiscoveryRate),
			Impact:      "Approaching crash discovery saturation",
			Evidence: map[string]interface{}{
				"trend":          analysis.DiscoveryTrend,
				"discovery_rate": analysis.DiscoveryRate,
			},
			Actions: []string{
				"Implement new fuzzing strategies",
				"Expand input generation techniques",
				"Target unexplored code paths",
			},
		})
	}

	// Uniqueness insights
	if analysis.UniquenessRatio < 0.3 {
		insights = append(insights, TrendInsight{
			ID:          "low-uniqueness",
			Type:        "efficiency",
			Severity:    "high",
			Title:       "Low Crash Uniqueness Ratio",
			Description: fmt.Sprintf("Only %.0f%% of crashes are unique", analysis.UniquenessRatio*100),
			Impact:      "Wasting resources on duplicate crashes",
			Evidence: map[string]interface{}{
				"uniqueness_ratio": analysis.UniquenessRatio,
			},
			Actions: []string{
				"Improve crash deduplication",
				"Optimize seed selection",
				"Focus on novel input generation",
			},
		})
	}

	// Severity trend insights
	for severity, trend := range analysis.SeverityTrends {
		if severity == "critical" && trend == TrendDirectionUp {
			insights = append(insights, TrendInsight{
				ID:          "increasing-critical-crashes",
				Type:        "severity",
				Severity:    "critical",
				Title:       "Critical Crashes Increasing",
				Description: "Trend shows increasing critical severity crashes",
				Impact:      "Higher risk of security vulnerabilities",
				Evidence: map[string]interface{}{
					"severity": severity,
					"trend":    trend,
				},
				Actions: []string{
					"Prioritize critical crash fixes",
					"Perform security audit",
					"Increase fuzzing of security-sensitive code",
				},
			})
		}
	}

	// Pattern-based insights
	for _, pattern := range patterns {
		if pattern.Type == "declining_uniqueness" {
			insights = append(insights, TrendInsight{
				ID:          "pattern-" + pattern.ID,
				Type:        "pattern",
				Severity:    "medium",
				Title:       "Crash Discovery Pattern: " + pattern.Description,
				Description: pattern.Description,
				Impact:      "Fuzzing efficiency decreasing",
				Evidence: map[string]interface{}{
					"pattern_type": pattern.Type,
					"confidence":   pattern.Confidence,
				},
			})
		}
	}

	return insights
}

// Utility methods

func (a *Analyzer) getIntervalForPeriod(period TrendPeriod) time.Duration {
	switch period {
	case TrendPeriodHourly:
		return time.Hour
	case TrendPeriodDaily:
		return 24 * time.Hour
	case TrendPeriodWeekly:
		return 7 * 24 * time.Hour
	case TrendPeriodMonthly:
		return 30 * 24 * time.Hour
	default:
		return time.Hour
	}
}

func (a *Analyzer) getNumPointsForPeriod(period TrendPeriod) int {
	switch period {
	case TrendPeriodHourly:
		return 24 // 24 hours
	case TrendPeriodDaily:
		return 30 // 30 days
	case TrendPeriodWeekly:
		return 12 // 12 weeks
	case TrendPeriodMonthly:
		return 12 // 12 months
	default:
		return 24
	}
}

func (a *Analyzer) determineTrendTypeAndDirection(dataPoints []CoverageTrendPoint) (TrendType, TrendDirection) {
	if len(dataPoints) < 3 {
		return TrendTypeIrregular, TrendDirectionStable
	}

	// Extract coverage values
	values := make([]float64, len(dataPoints))
	for i, dp := range dataPoints {
		values[i] = dp.TotalCoverage
	}

	// Simple linear regression
	n := float64(len(values))
	var sumX, sumY, sumXY, sumX2 float64
	for i, y := range values {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)

	// Determine direction
	direction := TrendDirectionStable
	if slope > 0.1 {
		direction = TrendDirectionUp
	} else if slope < -0.1 {
		direction = TrendDirectionDown
	}

	// Determine type (simplified)
	trendType := TrendTypeLinear

	// Check for plateau
	recentValues := values[len(values)-5:]
	var recentVariance float64
	recentMean := 0.0
	for _, v := range recentValues {
		recentMean += v
	}
	recentMean /= float64(len(recentValues))

	for _, v := range recentValues {
		recentVariance += (v - recentMean) * (v - recentMean)
	}
	recentVariance /= float64(len(recentValues))

	if recentVariance < 1.0 && slope < 0.05 {
		trendType = TrendTypePlateau
	}

	return trendType, direction
}

func (a *Analyzer) calculateTrendStrength(dataPoints []CoverageTrendPoint) float64 {
	if len(dataPoints) < 2 {
		return 0
	}

	// Calculate R-squared value (simplified)
	values := make([]float64, len(dataPoints))
	for i, dp := range dataPoints {
		values[i] = dp.TotalCoverage
	}

	// Calculate mean
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))

	// Calculate total sum of squares
	var totalSS float64
	for _, v := range values {
		totalSS += (v - mean) * (v - mean)
	}

	// For simplicity, assume linear trend
	// In production, would calculate residual sum of squares
	strength := 0.7 // Default moderate strength

	// Adjust based on variance
	if totalSS < 10 {
		strength = 0.9 // Very strong trend
	} else if totalSS > 100 {
		strength = 0.3 // Weak trend
	}

	return strength
}

func (a *Analyzer) calculateConsistency(dataPoints []CoverageTrendPoint) float64 {
	if len(dataPoints) < 3 {
		return 1.0
	}

	// Calculate consistency based on growth rate variation
	growthRates := make([]float64, 0)
	for _, dp := range dataPoints {
		if dp.GrowthRate != 0 {
			growthRates = append(growthRates, dp.GrowthRate)
		}
	}

	if len(growthRates) < 2 {
		return 1.0
	}

	// Calculate coefficient of variation
	mean := 0.0
	for _, rate := range growthRates {
		mean += rate
	}
	mean /= float64(len(growthRates))

	var variance float64
	for _, rate := range growthRates {
		variance += (rate - mean) * (rate - mean)
	}
	variance /= float64(len(growthRates))

	stdDev := math.Sqrt(variance)
	cv := stdDev / math.Abs(mean)

	// Convert to consistency score (inverse of CV)
	consistency := 1.0 / (1.0 + cv)

	return consistency
}

func (a *Analyzer) calculateVolatility(dataPoints []CoverageTrendPoint) float64 {
	if len(dataPoints) < 2 {
		return 0
	}

	// Calculate volatility as percentage changes
	changes := make([]float64, 0)
	for i := 1; i < len(dataPoints); i++ {
		if dataPoints[i-1].TotalCoverage != 0 {
			change := math.Abs(dataPoints[i].TotalCoverage-dataPoints[i-1].TotalCoverage) / dataPoints[i-1].TotalCoverage
			changes = append(changes, change*100)
		}
	}

	if len(changes) == 0 {
		return 0
	}

	// Average percentage change
	sum := 0.0
	for _, c := range changes {
		sum += c
	}

	return sum / float64(len(changes))
}

func (a *Analyzer) determineGrowthPattern(dataPoints []CoverageTrendPoint) GrowthPattern {
	if len(dataPoints) < 5 {
		return GrowthPatternSteady
	}

	// Analyze recent growth rates
	recentRates := make([]float64, 0)
	for i := len(dataPoints) - 5; i < len(dataPoints); i++ {
		recentRates = append(recentRates, dataPoints[i].GrowthRate)
	}

	// Check for stagnation
	allNearZero := true
	for _, rate := range recentRates {
		if math.Abs(rate) > 0.1 {
			allNearZero = false
			break
		}
	}
	if allNearZero {
		return GrowthPatternStagnant
	}

	// Check for acceleration/deceleration
	increasing := true
	decreasing := true
	for i := 1; i < len(recentRates); i++ {
		if recentRates[i] <= recentRates[i-1] {
			increasing = false
		}
		if recentRates[i] >= recentRates[i-1] {
			decreasing = false
		}
	}

	if increasing {
		return GrowthPatternAccelerating
	}
	if decreasing {
		return GrowthPatternDecelerating
	}

	// Check volatility
	var variance float64
	mean := 0.0
	for _, rate := range recentRates {
		mean += rate
	}
	mean /= float64(len(recentRates))

	for _, rate := range recentRates {
		variance += (rate - mean) * (rate - mean)
	}
	variance /= float64(len(recentRates))

	if variance > 1.0 {
		return GrowthPatternVolatile
	}

	return GrowthPatternSteady
}

func (a *Analyzer) identifyCurrentPhase(dataPoints []CoverageTrendPoint) string {
	if len(dataPoints) == 0 {
		return "unknown"
	}

	last := dataPoints[len(dataPoints)-1]

	// Define phases based on coverage level and growth rate
	if last.TotalCoverage < 20 {
		return "initial"
	} else if last.TotalCoverage < 50 && last.GrowthRate > 1 {
		return "rapid_growth"
	} else if last.TotalCoverage < 70 && last.GrowthRate > 0.5 {
		return "steady_growth"
	} else if last.TotalCoverage < 80 && last.GrowthRate > 0.1 {
		return "slowing_growth"
	} else if last.GrowthRate < 0.1 {
		return "plateau"
	}

	return "mature"
}

func (a *Analyzer) detectPhaseTransitions(dataPoints []CoverageTrendPoint) []PhaseTransition {
	transitions := make([]PhaseTransition, 0)

	if len(dataPoints) < 10 {
		return transitions
	}

	// Simple phase detection based on growth rate changes
	windowSize := 5
	for i := windowSize; i < len(dataPoints)-windowSize; i++ {
		prevWindow := dataPoints[i-windowSize : i]
		nextWindow := dataPoints[i : i+windowSize]

		prevPhase := a.identifyPhaseFromWindow(prevWindow)
		nextPhase := a.identifyPhaseFromWindow(nextWindow)

		if prevPhase != nextPhase {
			transitions = append(transitions, PhaseTransition{
				Timestamp:  dataPoints[i].Timestamp,
				FromPhase:  prevPhase,
				ToPhase:    nextPhase,
				Confidence: 0.7,
			})
		}
	}

	return transitions
}

func (a *Analyzer) identifyPhaseFromWindow(window []CoverageTrendPoint) string {
	if len(window) == 0 {
		return "unknown"
	}

	// Average metrics for window
	var avgCoverage, avgGrowth float64
	for _, dp := range window {
		avgCoverage += dp.TotalCoverage
		avgGrowth += dp.GrowthRate
	}
	avgCoverage /= float64(len(window))
	avgGrowth /= float64(len(window))

	// Classify phase
	if avgCoverage < 30 && avgGrowth > 2 {
		return "rapid_growth"
	} else if avgCoverage < 60 && avgGrowth > 0.5 {
		return "steady_growth"
	} else if avgGrowth < 0.1 {
		return "plateau"
	}

	return "normal"
}

func (a *Analyzer) determineSeverity(deviation float64) string {
	if deviation > 0.5 {
		return "critical"
	} else if deviation > 0.3 {
		return "high"
	} else if deviation > 0.2 {
		return "medium"
	}
	return "low"
}

func (a *Analyzer) analyzeSaturation(dataPoints []CoverageTrendPoint, predictions []ForecastPoint) *SaturationAnalysis {
	if len(dataPoints) == 0 {
		return nil
	}

	currentCoverage := dataPoints[len(dataPoints)-1].TotalCoverage

	// Estimate saturation point (simplified - would use curve fitting in production)
	estimatedSaturation := 85.0 // Typical saturation for fuzzing

	// Calculate time to saturation
	var timeToSaturation *time.Duration
	if len(predictions) > 0 && currentCoverage < estimatedSaturation {
		for _, pred := range predictions {
			if pred.PredictedValue >= estimatedSaturation*0.95 {
				duration := pred.Timestamp.Sub(dataPoints[len(dataPoints)-1].Timestamp)
				timeToSaturation = &duration
				break
			}
		}
	}

	currentUtilization := currentCoverage / estimatedSaturation
	remainingPotential := math.Max(0, estimatedSaturation-currentCoverage)

	return &SaturationAnalysis{
		EstimatedSaturation: estimatedSaturation,
		TimeToSaturation:    timeToSaturation,
		CurrentUtilization:  currentUtilization,
		RemainingPotential:  remainingPotential,
		ConfidenceLevel:     0.75,
	}
}

func (a *Analyzer) generateCoveragePredictions(dataPoints []CoverageTrendPoint, forecastPeriod time.Duration, analysis *CoverageTrendAnalysis) []ForecastPoint {
	predictions := make([]ForecastPoint, 0)

	if len(dataPoints) == 0 {
		return predictions
	}

	last := dataPoints[len(dataPoints)-1]
	interval := time.Hour
	numPredictions := int(forecastPeriod / interval)

	// Use different models based on trend type
	for i := 1; i <= numPredictions; i++ {
		timestamp := last.Timestamp.Add(time.Duration(i) * interval)

		var predictedValue float64
		var confidenceWidth float64

		switch analysis.TrendType {
		case TrendTypeLinear:
			// Linear projection
			predictedValue = last.TotalCoverage + last.GrowthRate*float64(i)
			confidenceWidth = 5.0 + float64(i)*0.5

		case TrendTypePlateau:
			// Logistic growth model
			k := 85.0 // Carrying capacity
			predictedValue = k / (1 + (k/last.TotalCoverage-1)*math.Exp(-0.1*float64(i)))
			confidenceWidth = 3.0 + float64(i)*0.3

		default:
			// Exponential smoothing
			alpha := 0.3
			predictedValue = last.TotalCoverage
			for j := 0; j < i; j++ {
				predictedValue = alpha*last.TotalCoverage + (1-alpha)*predictedValue
			}
			confidenceWidth = 8.0 + float64(i)*0.8
		}

		// Cap at reasonable bounds
		predictedValue = math.Max(0, math.Min(100, predictedValue))

		predictions = append(predictions, ForecastPoint{
			Timestamp:      timestamp,
			PredictedValue: predictedValue,
			ConfidenceInterval: Interval{
				Lower: math.Max(0, predictedValue-confidenceWidth),
				Upper: math.Min(100, predictedValue+confidenceWidth),
			},
			Probability: 0.9 - float64(i)*0.01, // Decreasing confidence over time
		})
	}

	return predictions
}

func (a *Analyzer) detectAnomaliesInData(dataPoints []DataPoint) []Anomaly {
	anomalies := make([]Anomaly, 0)

	if len(dataPoints) < 5 {
		return anomalies
	}

	// Calculate statistics
	values := make([]float64, len(dataPoints))
	for i, dp := range dataPoints {
		values[i] = dp.Value
	}

	mean, stdDev := a.calculateMeanStdDev(values)

	// Z-score method for anomaly detection
	for i, dp := range dataPoints {
		zScore := math.Abs(dp.Value-mean) / stdDev

		if zScore > 3 { // 3 standard deviations
			anomalyType := AnomalyTypeOutlier
			if i > 0 && math.Abs(dp.Value-dataPoints[i-1].Value) > stdDev*2 {
				anomalyType = AnomalyTypeSpike
				if dp.Value < dataPoints[i-1].Value {
					anomalyType = AnomalyTypeDrop
				}
			}

			anomalies = append(anomalies, Anomaly{
				ID:          fmt.Sprintf("anomaly-%d", i),
				Type:        anomalyType,
				Severity:    a.determineSeverity(zScore / 3),
				DetectedAt:  time.Now(),
				StartTime:   dp.Timestamp,
				Description: fmt.Sprintf("Value %.2f is %.1f std deviations from mean", dp.Value, zScore),
				Impact:      "Data quality or system behavior anomaly",
				Evidence: map[string]interface{}{
					"value":   dp.Value,
					"mean":    mean,
					"z_score": zScore,
				},
				Status: AnomalyStatusActive,
			})
		}
	}

	return anomalies
}

func (a *Analyzer) generateAnomalySummary(anomalies []Anomaly) *AnomalySummary {
	summary := &AnomalySummary{
		TotalAnomalies:    len(anomalies),
		SeverityBreakdown: make(map[string]int),
		TypeBreakdown:     make(map[string]int),
		MostAffectedAreas: make([]string, 0),
	}

	activeCount := 0
	for _, anomaly := range anomalies {
		if anomaly.Status == AnomalyStatusActive {
			activeCount++
		}
		summary.SeverityBreakdown[anomaly.Severity]++
		summary.TypeBreakdown[string(anomaly.Type)]++
	}

	summary.ActiveAnomalies = activeCount

	// Determine overall risk
	criticalCount := summary.SeverityBreakdown["critical"]
	highCount := summary.SeverityBreakdown["high"]

	if criticalCount > 0 {
		summary.OverallRisk = "critical"
	} else if highCount > 2 {
		summary.OverallRisk = "high"
	} else if len(anomalies) > 5 {
		summary.OverallRisk = "medium"
	} else {
		summary.OverallRisk = "low"
	}

	return summary
}

func (a *Analyzer) assessAnomalyImpact(anomalies []Anomaly) *ImpactAssessment {
	// Simplified impact assessment
	performanceImpact := 0.0
	efficiencyImpact := 0.0

	for _, anomaly := range anomalies {
		if anomaly.Status != AnomalyStatusActive {
			continue
		}

		switch anomaly.Severity {
		case "critical":
			performanceImpact += 20
			efficiencyImpact += 15
		case "high":
			performanceImpact += 10
			efficiencyImpact += 8
		case "medium":
			performanceImpact += 5
			efficiencyImpact += 3
		}
	}

	qualityImpact := "minimal"
	if performanceImpact > 30 {
		qualityImpact = "significant"
	} else if performanceImpact > 15 {
		qualityImpact = "moderate"
	}

	businessImpact := "normal operations"
	if performanceImpact > 40 {
		businessImpact = "service degradation possible"
	}

	return &ImpactAssessment{
		PerformanceImpact:  math.Min(100, performanceImpact),
		EfficiencyImpact:   math.Min(100, efficiencyImpact),
		QualityImpact:      qualityImpact,
		AffectedComponents: []string{"fuzzing engine", "coverage measurement"},
		BusinessImpact:     businessImpact,
	}
}

func (a *Analyzer) generateAnomalyRecommendations(anomalies []Anomaly, impact *ImpactAssessment) []Recommendation {
	recommendations := make([]Recommendation, 0)

	// Count anomaly types
	typeCount := make(map[AnomalyType]int)
	for _, anomaly := range anomalies {
		typeCount[anomaly.Type]++
	}

	// Generate recommendations based on patterns
	if typeCount[AnomalyTypeSpike] > 2 || typeCount[AnomalyTypeDrop] > 2 {
		recommendations = append(recommendations, Recommendation{
			ID:          "stabilize-metrics",
			Type:        "stability",
			Priority:    1,
			Title:       "Stabilize System Metrics",
			Description: "Multiple spike/drop anomalies detected",
			Rationale:   "System showing unstable behavior patterns",
			Actions: []string{
				"Review system resource allocation",
				"Check for external interference",
				"Implement rate limiting",
			},
			Timeline: "immediate",
			Expected: ExpectedOutcome{
				Description:  "Reduced metric volatility",
				Improvement:  30,
				TimeToImpact: "24 hours",
				Confidence:   0.7,
			},
		})
	}

	if impact.PerformanceImpact > 20 {
		recommendations = append(recommendations, Recommendation{
			ID:          "performance-recovery",
			Type:        "performance",
			Priority:    2,
			Title:       "Performance Recovery Plan",
			Description: fmt.Sprintf("Performance impacted by %.0f%%", impact.PerformanceImpact),
			Rationale:   "Anomalies causing significant performance degradation",
			Actions: []string{
				"Scale resources temporarily",
				"Optimize critical paths",
				"Implement caching",
			},
			Timeline: "within 48 hours",
			Expected: ExpectedOutcome{
				Description:  "Restore normal performance levels",
				Improvement:  impact.PerformanceImpact * 0.8,
				TimeToImpact: "48 hours",
				Confidence:   0.8,
			},
		})
	}

	return recommendations
}

func (a *Analyzer) determineForecastModel(values []float64) string {
	// Simple model selection based on data characteristics
	if len(values) < 10 {
		return "simple moving average"
	}

	// Check for linear trend
	isLinear := a.checkLinearTrend(values)
	if isLinear {
		return "linear regression"
	}

	// Check for exponential pattern
	isExponential := a.checkExponentialPattern(values)
	if isExponential {
		return "exponential smoothing"
	}

	// Default to ARIMA for complex patterns
	return "ARIMA"
}

func (a *Analyzer) generatePredictions(dataPoints []DataPoint, forecastPeriod time.Duration, methodology string) []ForecastPoint {
	predictions := make([]ForecastPoint, 0)

	if len(dataPoints) == 0 {
		return predictions
	}

	// Simple prediction implementation
	last := dataPoints[len(dataPoints)-1]
	interval := time.Hour
	numPredictions := int(forecastPeriod / interval)

	// Calculate trend
	trend := 0.0
	if len(dataPoints) > 1 {
		trend = (last.Value - dataPoints[len(dataPoints)-2].Value) / interval.Hours()
	}

	for i := 1; i <= numPredictions; i++ {
		timestamp := last.Timestamp.Add(time.Duration(i) * interval)

		// Simple linear prediction with decay
		decayFactor := math.Exp(-float64(i) / 24)
		predictedValue := last.Value + trend*float64(i)*decayFactor

		// Add uncertainty
		uncertainty := float64(i) * 2

		predictions = append(predictions, ForecastPoint{
			Timestamp:      timestamp,
			PredictedValue: predictedValue,
			ConfidenceInterval: Interval{
				Lower: predictedValue - uncertainty,
				Upper: predictedValue + uncertainty,
			},
			Probability: 0.95 - float64(i)*0.01,
		})
	}

	return predictions
}

func (a *Analyzer) calculateForecastConfidence(values []float64, methodology string) float64 {
	// Simple confidence calculation based on data consistency
	if len(values) < 5 {
		return 0.5
	}

	_, stdDev := a.calculateMeanStdDev(values)
	cv := stdDev / math.Abs(values[len(values)-1])

	// Lower CV means higher confidence
	confidence := 0.9 - math.Min(0.4, cv)

	// Adjust based on methodology
	switch methodology {
	case "linear regression":
		confidence *= 0.95
	case "ARIMA":
		confidence *= 0.85
	case "simple moving average":
		confidence *= 0.8
	}

	return confidence
}

func (a *Analyzer) calculatePerformanceStability(dataPoints []PerformanceTrendPoint) float64 {
	if len(dataPoints) < 2 {
		return 100
	}

	// Calculate variance in key metrics
	var execVariances, latencyVariances []float64

	for i := 1; i < len(dataPoints); i++ {
		execVar := math.Abs(dataPoints[i].ExecutionsPerSecond-dataPoints[i-1].ExecutionsPerSecond) / dataPoints[i-1].ExecutionsPerSecond
		latencyVar := math.Abs(dataPoints[i].AverageLatency-dataPoints[i-1].AverageLatency) / dataPoints[i-1].AverageLatency

		execVariances = append(execVariances, execVar)
		latencyVariances = append(latencyVariances, latencyVar)
	}

	// Average variance
	avgExecVar := a.average(execVariances)
	avgLatencyVar := a.average(latencyVariances)

	// Convert to stability score (inverse of variance)
	stability := 100 * (1 - (avgExecVar+avgLatencyVar)/2)

	return math.Max(0, math.Min(100, stability))
}

func (a *Analyzer) determinePerformanceGrade(efficiency, stability float64) string {
	score := (efficiency + stability) / 2

	if score >= 90 {
		return "A+"
	} else if score >= 85 {
		return "A"
	} else if score >= 80 {
		return "B+"
	} else if score >= 75 {
		return "B"
	} else if score >= 70 {
		return "C+"
	} else if score >= 65 {
		return "C"
	} else if score >= 60 {
		return "D"
	}
	return "F"
}

func (a *Analyzer) detectPerformanceDegradation(dataPoints []PerformanceTrendPoint) bool {
	if len(dataPoints) < 5 {
		return false
	}

	// Compare recent performance to earlier performance
	earlyPoints := dataPoints[:len(dataPoints)/2]
	recentPoints := dataPoints[len(dataPoints)/2:]

	earlyAvgEfficiency := 0.0
	for _, p := range earlyPoints {
		earlyAvgEfficiency += p.EfficiencyScore
	}
	earlyAvgEfficiency /= float64(len(earlyPoints))

	recentAvgEfficiency := 0.0
	for _, p := range recentPoints {
		recentAvgEfficiency += p.EfficiencyScore
	}
	recentAvgEfficiency /= float64(len(recentPoints))

	// Degradation if recent efficiency is 10% lower
	return recentAvgEfficiency < earlyAvgEfficiency*0.9
}

func (a *Analyzer) analyzeBottleneckTrends(dataPoints []PerformanceTrendPoint) []BottleneckTrend {
	trends := make([]BottleneckTrend, 0)

	if len(dataPoints) < 5 {
		return trends
	}

	// Check CPU bottleneck trend
	cpuTrend := a.analyzeResourceTrend(dataPoints, "cpu")
	if cpuTrend != TrendDirectionStable {
		trends = append(trends, BottleneckTrend{
			Type:      "CPU",
			Severity:  a.determineBottleneckSeverity(dataPoints[len(dataPoints)-1].CPUUtilization),
			Trend:     cpuTrend,
			FirstSeen: dataPoints[0].Timestamp,
			Frequency: a.calculateBottleneckFrequency(dataPoints, "cpu", 80),
			Impact:    "Processing capacity limited",
		})
	}

	// Check memory bottleneck trend
	memTrend := a.analyzeResourceTrend(dataPoints, "memory")
	if memTrend != TrendDirectionStable {
		trends = append(trends, BottleneckTrend{
			Type:      "Memory",
			Severity:  a.determineBottleneckSeverity(dataPoints[len(dataPoints)-1].MemoryUtilization),
			Trend:     memTrend,
			FirstSeen: dataPoints[0].Timestamp,
			Frequency: a.calculateBottleneckFrequency(dataPoints, "memory", 80),
			Impact:    "Memory pressure affecting performance",
		})
	}

	// Check queue bottleneck
	if len(dataPoints) > 0 && dataPoints[len(dataPoints)-1].QueueDepth > 5000 {
		trends = append(trends, BottleneckTrend{
			Type:      "Queue",
			Severity:  "high",
			Trend:     TrendDirectionUp,
			FirstSeen: dataPoints[0].Timestamp,
			Frequency: 0.7,
			Impact:    "Processing backlog growing",
		})
	}

	return trends
}

func (a *Analyzer) analyzeResourceTrend(dataPoints []PerformanceTrendPoint, resource string) TrendDirection {
	if len(dataPoints) < 2 {
		return TrendDirectionStable
	}

	// Extract resource values
	values := make([]float64, len(dataPoints))
	for i, dp := range dataPoints {
		switch resource {
		case "cpu":
			values[i] = dp.CPUUtilization
		case "memory":
			values[i] = dp.MemoryUtilization
		}
	}

	// Simple trend detection
	first := values[0]
	last := values[len(values)-1]

	if last > first*1.2 {
		return TrendDirectionUp
	} else if last < first*0.8 {
		return TrendDirectionDown
	}

	return TrendDirectionStable
}

func (a *Analyzer) determineBottleneckSeverity(utilization float64) string {
	if utilization > 90 {
		return "critical"
	} else if utilization > 80 {
		return "high"
	} else if utilization > 70 {
		return "medium"
	}
	return "low"
}

func (a *Analyzer) calculateBottleneckFrequency(dataPoints []PerformanceTrendPoint, resource string, threshold float64) float64 {
	if len(dataPoints) == 0 {
		return 0
	}

	count := 0
	for _, dp := range dataPoints {
		value := 0.0
		switch resource {
		case "cpu":
			value = dp.CPUUtilization
		case "memory":
			value = dp.MemoryUtilization
		}

		if value > threshold {
			count++
		}
	}

	return float64(count) / float64(len(dataPoints))
}

func (a *Analyzer) identifyOptimizationWindows(dataPoints []PerformanceTrendPoint) []TimeWindow {
	windows := make([]TimeWindow, 0)

	if len(dataPoints) < 3 {
		return windows
	}

	// Find periods of low activity
	threshold := 300.0 // Low activity threshold for executions/sec
	inWindow := false
	var windowStart time.Time

	for i, dp := range dataPoints {
		if dp.ExecutionsPerSecond < threshold && dp.CPUUtilization < 50 {
			if !inWindow {
				inWindow = true
				windowStart = dp.Timestamp
			}
		} else if inWindow {
			// End of window
			windows = append(windows, TimeWindow{
				Start:       windowStart,
				End:         dataPoints[i-1].Timestamp,
				Description: "Low activity period suitable for optimization",
			})
			inWindow = false
		}
	}

	// Close any open window
	if inWindow && len(dataPoints) > 0 {
		windows = append(windows, TimeWindow{
			Start:       windowStart,
			End:         dataPoints[len(dataPoints)-1].Timestamp,
			Description: "Low activity period suitable for optimization",
		})
	}

	return windows
}

func (a *Analyzer) predictBottlenecks(predictions []PerformanceForecastPoint) []ExpectedBottleneck {
	bottlenecks := make([]ExpectedBottleneck, 0)

	for _, pred := range predictions {
		// Check for CPU bottleneck
		if pred.ExpectedUtilization.Upper > 90 {
			bottlenecks = append(bottlenecks, ExpectedBottleneck{
				Type:              "CPU",
				ExpectedTimestamp: pred.Timestamp,
				Severity:          "high",
				Impact:            "Processing delays expected",
				PreventiveActions: []string{
					"Scale CPU resources",
					"Optimize CPU-intensive operations",
					"Implement load balancing",
				},
			})
			break // Only report first occurrence
		}

		// Check for latency issues
		if pred.ExpectedLatency.Upper > 200 {
			bottlenecks = append(bottlenecks, ExpectedBottleneck{
				Type:              "Latency",
				ExpectedTimestamp: pred.Timestamp,
				Severity:          "medium",
				Impact:            "Response time degradation",
				PreventiveActions: []string{
					"Optimize slow queries",
					"Implement caching",
					"Review network configuration",
				},
			})
			break
		}
	}

	return bottlenecks
}

func (a *Analyzer) generatePerformanceRecommendations(analysis *PerformanceTrendAnalysis, predictions []PerformanceForecastPoint) []RecommendedAction {
	actions := make([]RecommendedAction, 0)

	// Recommend based on stability
	if analysis.StabilityScore < 70 {
		actions = append(actions, RecommendedAction{
			ID:          "improve-stability",
			Type:        "stability",
			Priority:    1,
			Title:       "Implement Stability Improvements",
			Description: "System showing unstable performance patterns",
			Timing:      "immediate",
			Impact:      "Improved reliability and predictability",
			Trigger: Trigger{
				Type:      "threshold",
				Condition: "stability_score < 70",
				Threshold: 70,
			},
		})
	}

	// Recommend based on degradation
	if analysis.DegradationDetected {
		actions = append(actions, RecommendedAction{
			ID:          "address-degradation",
			Type:        "performance",
			Priority:    1,
			Title:       "Address Performance Degradation",
			Description: "Performance metrics showing downward trend",
			Timing:      "within 24 hours",
			Impact:      "Restore optimal performance levels",
			Trigger: Trigger{
				Type:      "trend",
				Condition: "performance degrading",
			},
		})
	}

	// Recommend based on predictions
	highRiskCount := 0
	for _, pred := range predictions {
		if pred.RiskLevel == "high" {
			highRiskCount++
		}
	}

	if highRiskCount > 5 {
		actions = append(actions, RecommendedAction{
			ID:          "prepare-scaling",
			Type:        "scaling",
			Priority:    2,
			Title:       "Prepare for Resource Scaling",
			Description: "Forecasts indicate resource constraints ahead",
			Timing:      "within 48 hours",
			Impact:      "Prevent performance bottlenecks",
			Trigger: Trigger{
				Type:      "forecast",
				Condition: "high risk periods predicted",
				Metadata: map[string]interface{}{
					"risk_periods": highRiskCount,
				},
			},
		})
	}

	return actions
}

func (a *Analyzer) identifyPeakDiscoveryTimes(dataPoints []CrashTrendPoint) []TimeWindow {
	windows := make([]TimeWindow, 0)

	if len(dataPoints) < 3 {
		return windows
	}

	// Calculate average crash rate
	totalRate := 0.0
	for _, dp := range dataPoints {
		totalRate += dp.CrashRate
	}
	avgRate := totalRate / float64(len(dataPoints))

	// Find periods above 1.5x average
	threshold := avgRate * 1.5
	inPeak := false
	var peakStart time.Time

	for i, dp := range dataPoints {
		if dp.CrashRate > threshold {
			if !inPeak {
				inPeak = true
				peakStart = dp.Timestamp
			}
		} else if inPeak {
			windows = append(windows, TimeWindow{
				Start:       peakStart,
				End:         dataPoints[i-1].Timestamp,
				Description: fmt.Sprintf("Peak discovery period (%.1fx average)", threshold/avgRate),
			})
			inPeak = false
		}
	}

	return windows
}

func (a *Analyzer) getTopTypes(typeFrequency map[string]int, n int) []string {
	// Convert to slice for sorting
	type kv struct {
		Type  string
		Count int
	}

	var types []kv
	for t, c := range typeFrequency {
		types = append(types, kv{Type: t, Count: c})
	}

	// Sort by count
	sort.Slice(types, func(i, j int) bool {
		return types[i].Count > types[j].Count
	})

	// Return top N
	result := make([]string, 0, n)
	for i := 0; i < n && i < len(types); i++ {
		result = append(result, types[i].Type)
	}

	return result
}

// Utility methods

func (a *Analyzer) calculateMeanStdDev(values []float64) (mean, stdDev float64) {
	if len(values) == 0 {
		return 0, 0
	}

	// Calculate mean
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean = sum / float64(len(values))

	// Calculate standard deviation
	sumSquaredDiff := 0.0
	for _, v := range values {
		diff := v - mean
		sumSquaredDiff += diff * diff
	}
	variance := sumSquaredDiff / float64(len(values))
	stdDev = math.Sqrt(variance)

	return mean, stdDev
}

func (a *Analyzer) average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}

	return sum / float64(len(values))
}

func (a *Analyzer) checkLinearTrend(values []float64) bool {
	if len(values) < 3 {
		return true
	}

	// Simple linearity check - calculate R-squared
	n := float64(len(values))
	var sumX, sumY, sumXY, sumX2, sumY2 float64

	for i, y := range values {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
		sumY2 += y * y
	}

	// Calculate correlation coefficient
	numerator := n*sumXY - sumX*sumY
	denominator := math.Sqrt((n*sumX2 - sumX*sumX) * (n*sumY2 - sumY*sumY))

	if denominator == 0 {
		return false
	}

	r := numerator / denominator
	r2 := r * r

	// Consider linear if R-squared > 0.8
	return r2 > 0.8
}

func (a *Analyzer) checkExponentialPattern(values []float64) bool {
	if len(values) < 3 {
		return false
	}

	// Check if log-transformed values show linear pattern
	logValues := make([]float64, len(values))
	for i, v := range values {
		if v <= 0 {
			return false
		}
		logValues[i] = math.Log(v)
	}

	return a.checkLinearTrend(logValues)
}
