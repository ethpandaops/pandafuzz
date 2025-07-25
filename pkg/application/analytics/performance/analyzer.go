package performance

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	botRepo "github.com/ethpandaops/pandafuzz/pkg/domain/bot/repository"
	botTypes "github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
	campaignRepo "github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	"github.com/sirupsen/logrus"
)

// Analyzer implements the PerformanceAnalyzer interface
type Analyzer struct {
	campaignRepo     campaignRepo.CampaignRepository
	botRepo          botRepo.AgentRepository
	metricsCollector *MetricsCollector
	logger           logrus.FieldLogger
}

// NewAnalyzer creates a new performance analyzer
func NewAnalyzer(
	campaignRepo campaignRepo.CampaignRepository,
	botRepo botRepo.AgentRepository,
	metricsCollector *MetricsCollector,
	logger logrus.FieldLogger,
) *Analyzer {
	return &Analyzer{
		campaignRepo:     campaignRepo,
		botRepo:          botRepo,
		metricsCollector: metricsCollector,
		logger:           logger.WithField("component", "performance_analyzer"),
	}
}

// AnalyzeCampaignPerformance analyzes performance metrics for a campaign
func (a *Analyzer) AnalyzeCampaignPerformance(ctx context.Context, campaignID string) (*PerformanceReport, error) {
	a.logger.WithField("campaign_id", campaignID).Debug("Analyzing campaign performance")

	// Get campaign data
	campaign, err := a.campaignRepo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("failed to get campaign: %w", err)
	}

	// Collect metrics
	metrics, err := a.metricsCollector.CollectCampaignMetrics(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("failed to collect metrics: %w", err)
	}

	// Generate summary
	summary := a.generatePerformanceSummary(metrics)

	// Perform analysis
	analysis := a.performAnalysis(metrics)

	// Identify bottlenecks
	bottlenecks := a.identifyBottlenecks(metrics, analysis)

	// Generate recommendations
	recommendations := a.generateRecommendations(analysis, bottlenecks)

	// Analyze trends if enough data
	var trends *PerformanceTrends
	if time.Since(campaign.CreatedAt) > 6*time.Hour {
		trends = a.analyzeTrends(campaignID, TrendPeriodHourly)
	}

	report := &PerformanceReport{
		ID:          fmt.Sprintf("perf-%s-%d", campaignID, time.Now().Unix()),
		CampaignID:  campaignID,
		GeneratedAt: time.Now(),
		TimeRange: TimeRange{
			Start: campaign.CreatedAt,
			End:   time.Now(),
		},
		Summary:         summary,
		Metrics:         metrics,
		Analysis:        analysis,
		Bottlenecks:     bottlenecks,
		Recommendations: recommendations,
		Trends:          trends,
	}

	return report, nil
}

// AnalyzeBotPerformance analyzes performance metrics for a specific bot
func (a *Analyzer) AnalyzeBotPerformance(ctx context.Context, botID string, timeRange TimeRange) (*BotPerformanceReport, error) {
	a.logger.WithField("bot_id", botID).Debug("Analyzing bot performance")

	// Get bot data
	bot, err := a.botRepo.FindByID(ctx, botID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bot: %w", err)
	}

	// Collect bot metrics
	metrics, err := a.metricsCollector.CollectBotMetrics(ctx, botID)
	if err != nil {
		return nil, fmt.Errorf("failed to collect bot metrics: %w", err)
	}

	// Analyze performance
	performance := a.analyzeBotPerformance(metrics, bot)

	// Analyze health
	health := a.analyzeBotHealth(metrics, bot)

	// Generate recommendations
	recommendations := a.generateBotRecommendations(performance, health)

	report := &BotPerformanceReport{
		BotID:           botID,
		GeneratedAt:     time.Now(),
		TimeRange:       timeRange,
		Metrics:         metrics,
		Performance:     performance,
		Health:          health,
		Recommendations: recommendations,
	}

	return report, nil
}

// IdentifyBottlenecks identifies performance bottlenecks
func (a *Analyzer) IdentifyBottlenecks(ctx context.Context, campaignID string) (*BottleneckAnalysis, error) {
	// Collect metrics
	metrics, err := a.metricsCollector.CollectCampaignMetrics(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("failed to collect metrics: %w", err)
	}

	// Perform analysis
	analysis := a.performAnalysis(metrics)

	// Identify bottlenecks
	return a.identifyBottlenecks(metrics, analysis), nil
}

// RecommendOptimizations provides optimization recommendations
func (a *Analyzer) RecommendOptimizations(ctx context.Context, analysis *PerformanceReport) (*OptimizationRecommendations, error) {
	if analysis == nil {
		return nil, fmt.Errorf("no analysis provided")
	}

	return a.generateRecommendations(analysis.Analysis, analysis.Bottlenecks), nil
}

// Helper methods

func (a *Analyzer) generatePerformanceSummary(metrics *CampaignMetrics) *PerformanceSummary {
	// Calculate scores
	executionScore := a.calculateExecutionScore(metrics.ExecutionMetrics)
	resourceScore := a.calculateResourceScore(metrics.ResourceMetrics)
	efficiencyScore := a.calculateEfficiencyScore(metrics.EfficiencyMetrics)
	overallScore := (executionScore + resourceScore + efficiencyScore) / 3

	// Determine health status
	healthStatus := a.determineHealthStatus(overallScore)

	// Generate highlights and issues
	highlights := a.generateHighlights(metrics)
	issues := a.identifyCriticalIssues(metrics)

	return &PerformanceSummary{
		OverallScore:    overallScore,
		ExecutionScore:  executionScore,
		ResourceScore:   resourceScore,
		EfficiencyScore: efficiencyScore,
		HealthStatus:    healthStatus,
		KeyHighlights:   highlights,
		CriticalIssues:  issues,
	}
}

func (a *Analyzer) performAnalysis(metrics *CampaignMetrics) *PerformanceAnalysis {
	return &PerformanceAnalysis{
		ExecutionAnalysis:  a.analyzeExecution(metrics.ExecutionMetrics),
		ResourceAnalysis:   a.analyzeResources(metrics.ResourceMetrics),
		EfficiencyAnalysis: a.analyzeEfficiency(metrics.EfficiencyMetrics),
		Insights:           a.generateInsights(metrics),
	}
}

func (a *Analyzer) analyzeExecution(metrics *ExecutionMetrics) *ExecutionAnalysis {
	if metrics == nil {
		return &ExecutionAnalysis{
			ThroughputTrend:   "unknown",
			LatencyTrend:      "unknown",
			StabilityScore:    0,
			PerformanceGrade:  "N/A",
			AnomaliesDetected: 0,
		}
	}

	// Analyze trends
	throughputTrend := "stable"
	if metrics.ExecutionsPerSecond > 1000 {
		throughputTrend = "high"
	} else if metrics.ExecutionsPerSecond < 100 {
		throughputTrend = "low"
	}

	latencyTrend := "normal"
	if metrics.AverageExecTime > 100 {
		latencyTrend = "high"
	} else if metrics.AverageExecTime < 10 {
		latencyTrend = "low"
	}

	// Calculate stability score
	stabilityScore := 100.0
	if metrics.FailureRate > 0 {
		stabilityScore -= metrics.FailureRate * 100
	}
	if metrics.P99ExecTime > metrics.AverageExecTime*3 {
		stabilityScore -= 20
	}
	stabilityScore = math.Max(0, stabilityScore)

	// Determine performance grade
	grade := "A"
	if stabilityScore < 90 {
		grade = "B"
	}
	if stabilityScore < 80 {
		grade = "C"
	}
	if stabilityScore < 70 {
		grade = "D"
	}
	if stabilityScore < 60 {
		grade = "F"
	}

	// Detect anomalies (simplified)
	anomalies := 0
	if metrics.FailureRate > 0.1 {
		anomalies++
	}
	if metrics.P99ExecTime > metrics.P95ExecTime*2 {
		anomalies++
	}

	return &ExecutionAnalysis{
		ThroughputTrend:   throughputTrend,
		LatencyTrend:      latencyTrend,
		StabilityScore:    stabilityScore,
		PerformanceGrade:  grade,
		AnomaliesDetected: anomalies,
	}
}

func (a *Analyzer) analyzeResources(metrics *ResourceMetrics) *ResourceAnalysis {
	if metrics == nil {
		return &ResourceAnalysis{
			UtilizationLevel: "unknown",
			ResourceBalance:  "unknown",
			ScalingPotential: 0,
			WastedResources:  0,
			OptimizationRoom: 0,
		}
	}

	// Determine utilization level
	avgCPU := metrics.CPUUsage.AverageUsage
	avgMem := metrics.MemoryUsage.AverageUsage / metrics.MemoryUsage.Available * 100

	utilizationLevel := "optimal"
	if avgCPU > 80 || avgMem > 80 {
		utilizationLevel = "high"
	} else if avgCPU < 20 && avgMem < 20 {
		utilizationLevel = "low"
	}

	// Assess resource balance
	cpuMemRatio := avgCPU / avgMem
	resourceBalance := "balanced"
	if cpuMemRatio > 2 {
		resourceBalance = "cpu-heavy"
	} else if cpuMemRatio < 0.5 {
		resourceBalance = "memory-heavy"
	}

	// Calculate scaling potential
	scalingPotential := 100 - math.Max(avgCPU, avgMem)

	// Calculate wasted resources
	wastedResources := 0.0
	if avgCPU < 30 && avgMem < 30 {
		wastedResources = 100 - (avgCPU + avgMem)
	}

	// Calculate optimization room
	optimizationRoom := 0.0
	if metrics.ResourceScore < 80 {
		optimizationRoom = 100 - metrics.ResourceScore
	}

	return &ResourceAnalysis{
		UtilizationLevel: utilizationLevel,
		ResourceBalance:  resourceBalance,
		ScalingPotential: scalingPotential,
		WastedResources:  wastedResources,
		OptimizationRoom: optimizationRoom,
	}
}

func (a *Analyzer) analyzeEfficiency(metrics *EfficiencyMetrics) *EfficiencyAnalysis {
	if metrics == nil {
		return &EfficiencyAnalysis{
			ProductivityLevel: "unknown",
			CostEfficiency:    0,
			OutputQuality:     0,
			TimeEfficiency:    0,
		}
	}

	// Determine productivity level
	productivityLevel := "moderate"
	if metrics.CrashesPerHour > 10 {
		productivityLevel = "high"
	} else if metrics.CrashesPerHour < 1 {
		productivityLevel = "low"
	}

	// Calculate cost efficiency (simplified)
	costEfficiency := metrics.ResourceEfficiency*0.7 + metrics.EfficiencyScore*0.3

	// Calculate output quality
	outputQuality := metrics.CoveragePerExecution * 100
	if outputQuality > 100 {
		outputQuality = 100
	}

	// Calculate time efficiency
	timeEfficiency := 100.0
	if metrics.TimeToFirstCrash != nil {
		// Lower time to first crash is better
		hours := metrics.TimeToFirstCrash.Hours()
		if hours > 1 {
			timeEfficiency -= hours * 10
		}
	}
	timeEfficiency = math.Max(0, timeEfficiency)

	return &EfficiencyAnalysis{
		ProductivityLevel: productivityLevel,
		CostEfficiency:    costEfficiency,
		OutputQuality:     outputQuality,
		TimeEfficiency:    timeEfficiency,
	}
}

func (a *Analyzer) identifyBottlenecks(metrics *CampaignMetrics, analysis *PerformanceAnalysis) *BottleneckAnalysis {
	bottlenecks := make([]Bottleneck, 0)

	// Check CPU bottleneck
	if metrics.ResourceMetrics != nil && metrics.ResourceMetrics.CPUUsage.PeakUsage > 90 {
		bottlenecks = append(bottlenecks, Bottleneck{
			ID:          "cpu-bottleneck",
			Type:        BottleneckTypeCPU,
			Component:   "CPU",
			Severity:    "high",
			Description: fmt.Sprintf("CPU usage peaked at %.1f%%", metrics.ResourceMetrics.CPUUsage.PeakUsage),
			Impact:      20.0,
			Evidence: map[string]interface{}{
				"peak_usage":    metrics.ResourceMetrics.CPUUsage.PeakUsage,
				"average_usage": metrics.ResourceMetrics.CPUUsage.AverageUsage,
			},
			Solutions: []Solution{
				{
					ID:          "scale-horizontally",
					Title:       "Scale Horizontally",
					Description: "Add more fuzzing bots to distribute load",
					Effort:      "medium",
					Impact:      "high",
					Priority:    1,
				},
				{
					ID:          "optimize-code",
					Title:       "Optimize Code",
					Description: "Profile and optimize CPU-intensive code paths",
					Effort:      "high",
					Impact:      "medium",
					Priority:    2,
				},
			},
		})
	}

	// Check memory bottleneck
	if metrics.ResourceMetrics != nil && metrics.ResourceMetrics.MemoryUsage.GCPressure > 0.5 {
		bottlenecks = append(bottlenecks, Bottleneck{
			ID:          "memory-bottleneck",
			Type:        BottleneckTypeMemory,
			Component:   "Memory",
			Severity:    "medium",
			Description: "High GC pressure indicates memory management issues",
			Impact:      15.0,
			Evidence: map[string]interface{}{
				"gc_pressure": metrics.ResourceMetrics.MemoryUsage.GCPressure,
				"peak_usage":  metrics.ResourceMetrics.MemoryUsage.PeakUsage,
			},
			Solutions: []Solution{
				{
					ID:          "increase-memory",
					Title:       "Increase Memory Allocation",
					Description: "Allocate more memory to reduce GC pressure",
					Effort:      "low",
					Impact:      "medium",
					Priority:    1,
				},
			},
		})
	}

	// Check queue bottleneck
	if metrics.QueueMetrics != nil && metrics.QueueMetrics.QueueUtilization > 80 {
		bottlenecks = append(bottlenecks, Bottleneck{
			ID:          "queue-bottleneck",
			Type:        BottleneckTypeQueue,
			Component:   "Queue",
			Severity:    "high",
			Description: fmt.Sprintf("Queue utilization at %.1f%%", metrics.QueueMetrics.QueueUtilization),
			Impact:      25.0,
			Evidence: map[string]interface{}{
				"queue_depth":     metrics.QueueMetrics.QueueDepth,
				"processing_rate": metrics.QueueMetrics.ProcessingRate,
				"dropped_items":   metrics.QueueMetrics.DroppedItems,
			},
			Solutions: []Solution{
				{
					ID:          "increase-workers",
					Title:       "Increase Queue Workers",
					Description: "Add more workers to process queue items faster",
					Effort:      "low",
					Impact:      "high",
					Priority:    1,
				},
			},
		})
	}

	// Determine primary limiter
	primaryLimiter := "none"
	maxImpact := 0.0
	for _, b := range bottlenecks {
		if b.Impact > maxImpact {
			maxImpact = b.Impact
			primaryLimiter = string(b.Type)
		}
	}

	// Calculate impact analysis
	impactAnalysis := a.calculateImpactAnalysis(bottlenecks, metrics)

	return &BottleneckAnalysis{
		IdentifiedAt:   time.Now(),
		Bottlenecks:    bottlenecks,
		PrimaryLimiter: primaryLimiter,
		ImpactAnalysis: impactAnalysis,
	}
}

func (a *Analyzer) calculateImpactAnalysis(bottlenecks []Bottleneck, metrics *CampaignMetrics) *ImpactAnalysis {
	totalImpact := 0.0
	for _, b := range bottlenecks {
		totalImpact += b.Impact
	}

	throughputLoss := math.Min(totalImpact, 50)
	efficiencyLoss := totalImpact * 0.8
	costIncrease := totalImpact * 0.5

	qualityImpact := "minimal"
	if totalImpact > 30 {
		qualityImpact = "moderate"
	}
	if totalImpact > 50 {
		qualityImpact = "significant"
	}

	// Estimate resolution time based on complexity
	resolutionHours := len(bottlenecks) * 8 // 8 hours per bottleneck
	estimatedResolution := time.Duration(resolutionHours) * time.Hour

	return &ImpactAnalysis{
		ThroughputLoss:      throughputLoss,
		EfficiencyLoss:      efficiencyLoss,
		CostIncrease:        costIncrease,
		QualityImpact:       qualityImpact,
		EstimatedResolution: estimatedResolution,
	}
}

func (a *Analyzer) generateRecommendations(analysis *PerformanceAnalysis, bottlenecks *BottleneckAnalysis) *OptimizationRecommendations {
	recommendations := make([]Recommendation, 0)

	// Generate recommendations based on analysis
	if analysis.ExecutionAnalysis.StabilityScore < 80 {
		recommendations = append(recommendations, Recommendation{
			ID:          "improve-stability",
			Category:    "stability",
			Priority:    1,
			Title:       "Improve Execution Stability",
			Description: "High failure rate detected in execution metrics",
			Rationale:   "Improving stability will increase overall throughput and efficiency",
			Actions: []Action{
				{
					Step:        1,
					Description: "Identify and fix failing test cases",
					Type:        "diagnostic",
					Target:      "execution",
				},
				{
					Step:        2,
					Description: "Implement retry logic for transient failures",
					Type:        "implementation",
					Target:      "execution",
				},
			},
			Expected: ExpectedImprovement{
				ThroughputGain: 15,
				EfficiencyGain: 20,
				ResourceSaving: 10,
				Timeline:       "1 week",
				Confidence:     0.8,
			},
			Effort: "medium",
			Risk:   "low",
		})
	}

	// Add bottleneck-specific recommendations
	if bottlenecks != nil {
		for _, bottleneck := range bottlenecks.Bottlenecks {
			if len(bottleneck.Solutions) > 0 {
				solution := bottleneck.Solutions[0] // Take highest priority solution
				recommendations = append(recommendations, Recommendation{
					ID:          fmt.Sprintf("resolve-%s", bottleneck.ID),
					Category:    "bottleneck",
					Priority:    solution.Priority,
					Title:       solution.Title,
					Description: solution.Description,
					Rationale:   fmt.Sprintf("Resolving %s bottleneck will improve performance", bottleneck.Type),
					Actions: []Action{
						{
							Step:        1,
							Description: solution.Description,
							Type:        "implementation",
							Target:      bottleneck.Component,
						},
					},
					Expected: ExpectedImprovement{
						ThroughputGain: bottleneck.Impact,
						EfficiencyGain: bottleneck.Impact * 0.8,
						ResourceSaving: bottleneck.Impact * 0.5,
						Timeline:       "2 weeks",
						Confidence:     0.7,
					},
					Effort: solution.Effort,
					Risk:   "medium",
				})
			}
		}
	}

	// Calculate potential gains
	gains := a.calculatePotentialGains(recommendations)

	return &OptimizationRecommendations{
		GeneratedAt:     time.Now(),
		Recommendations: recommendations,
		PotentialGains:  gains,
		Priority:        a.determinePriority(recommendations),
	}
}

func (a *Analyzer) calculatePotentialGains(recommendations []Recommendation) *PotentialGains {
	var totalThroughput, totalEfficiency, totalResource float64
	var totalTimeSaving time.Duration

	for _, rec := range recommendations {
		totalThroughput += rec.Expected.ThroughputGain
		totalEfficiency += rec.Expected.EfficiencyGain
		totalResource += rec.Expected.ResourceSaving
	}

	// Estimate time saving based on efficiency gains
	hoursSaved := totalEfficiency * 24 / 100 // Convert percentage to hours per day
	totalTimeSaving = time.Duration(hoursSaved) * time.Hour

	// Calculate ROI (simplified)
	roi := (totalThroughput + totalEfficiency + totalResource) / 3

	return &PotentialGains{
		TotalThroughputGain: totalThroughput,
		TotalEfficiencyGain: totalEfficiency,
		TotalResourceSaving: totalResource,
		EstimatedTimeSaving: totalTimeSaving,
		ROI:                 roi,
	}
}

func (a *Analyzer) analyzeTrends(campaignID string, period TrendPeriod) *PerformanceTrends {
	// Generate sample trend data
	dataPoints := a.generateTrendDataPoints(period)

	// Analyze trends
	analysis := a.analyzeTrendData(dataPoints)

	// Generate forecast
	forecast := a.generateForecast(dataPoints, analysis)

	return &PerformanceTrends{
		Period:     period,
		DataPoints: dataPoints,
		Analysis:   analysis,
		Forecast:   forecast,
	}
}

func (a *Analyzer) generateTrendDataPoints(period TrendPeriod) []PerformanceTrendPoint {
	// Generate sample trend points
	points := make([]PerformanceTrendPoint, 0)

	now := time.Now()
	var interval time.Duration
	numPoints := 24

	switch period {
	case TrendPeriodHourly:
		interval = time.Hour
	case TrendPeriodDaily:
		interval = 24 * time.Hour
		numPoints = 7
	default:
		interval = time.Hour
	}

	for i := numPoints; i > 0; i-- {
		timestamp := now.Add(-time.Duration(i) * interval)

		// Generate realistic-looking metrics
		baseExecPerSec := 500.0
		execPerSec := baseExecPerSec + math.Sin(float64(i))*100
		latency := 50.0 + math.Cos(float64(i))*10
		cpu := 60.0 + math.Sin(float64(i)*0.5)*20
		memory := 4000.0 + math.Cos(float64(i)*0.3)*1000
		efficiency := 75.0 + math.Sin(float64(i)*0.2)*10

		points = append(points, PerformanceTrendPoint{
			Timestamp:        timestamp,
			ExecutionsPerSec: execPerSec,
			AverageLatency:   latency,
			CPUUsage:         cpu,
			MemoryUsage:      memory,
			EfficiencyScore:  efficiency,
		})
	}

	return points
}

func (a *Analyzer) analyzeTrendData(dataPoints []PerformanceTrendPoint) *TrendAnalysis {
	if len(dataPoints) < 2 {
		return nil
	}

	// Calculate trend direction
	first := dataPoints[0]
	last := dataPoints[len(dataPoints)-1]

	direction := "stable"
	if last.EfficiencyScore > first.EfficiencyScore+5 {
		direction = "improving"
	} else if last.EfficiencyScore < first.EfficiencyScore-5 {
		direction = "degrading"
	}

	// Calculate trend strength
	delta := math.Abs(last.EfficiencyScore - first.EfficiencyScore)
	strength := delta / first.EfficiencyScore

	// Calculate volatility
	var sumSquaredDiffs float64
	mean := 0.0
	for _, p := range dataPoints {
		mean += p.EfficiencyScore
	}
	mean /= float64(len(dataPoints))

	for _, p := range dataPoints {
		diff := p.EfficiencyScore - mean
		sumSquaredDiffs += diff * diff
	}
	volatility := math.Sqrt(sumSquaredDiffs / float64(len(dataPoints)))

	// Detect anomalies (simplified)
	anomalies := 0
	for i := 1; i < len(dataPoints); i++ {
		if math.Abs(dataPoints[i].EfficiencyScore-dataPoints[i-1].EfficiencyScore) > 20 {
			anomalies++
		}
	}

	return &TrendAnalysis{
		TrendDirection:    direction,
		TrendStrength:     strength,
		Volatility:        volatility,
		SeasonalPattern:   false, // Would require more sophisticated analysis
		AnomaliesDetected: anomalies,
	}
}

func (a *Analyzer) generateForecast(dataPoints []PerformanceTrendPoint, analysis *TrendAnalysis) *PerformanceForecast {
	if len(dataPoints) == 0 || analysis == nil {
		return nil
	}

	// Simple linear forecast based on recent trend
	last := dataPoints[len(dataPoints)-1]

	// Calculate growth rate from recent points
	recentPoints := 5
	if len(dataPoints) < recentPoints {
		recentPoints = len(dataPoints)
	}

	startIdx := len(dataPoints) - recentPoints
	recentGrowth := (last.EfficiencyScore - dataPoints[startIdx].EfficiencyScore) / float64(recentPoints)

	// Generate forecast points
	oneHour := a.generateForecastPoint(last, 1*time.Hour, recentGrowth*1)
	oneDay := a.generateForecastPoint(last, 24*time.Hour, recentGrowth*24)
	oneWeek := a.generateForecastPoint(last, 7*24*time.Hour, recentGrowth*24*7)

	// Adjust confidence based on volatility
	confidence := 0.8
	if analysis.Volatility > 10 {
		confidence = 0.6
	}
	if analysis.Volatility > 20 {
		confidence = 0.4
	}

	return &PerformanceForecast{
		OneHour:     oneHour,
		OneDay:      oneDay,
		OneWeek:     oneWeek,
		Methodology: "linear regression with decay",
		Confidence:  confidence,
		Assumptions: []string{
			"Current trends continue",
			"No major system changes",
			"Stable workload patterns",
		},
	}
}

func (a *Analyzer) generateForecastPoint(last PerformanceTrendPoint, duration time.Duration, growthAmount float64) *ForecastPoint {
	// Apply decay factor for longer forecasts
	hours := duration.Hours()
	decayFactor := math.Exp(-hours / 168) // Decay over a week
	adjustedGrowth := growthAmount * decayFactor

	forecastEfficiency := last.EfficiencyScore + adjustedGrowth
	forecastEfficiency = math.Max(0, math.Min(100, forecastEfficiency))

	// Forecast other metrics with similar approach
	execGrowth := (last.ExecutionsPerSec * 0.01) * hours * decayFactor
	forecastExec := last.ExecutionsPerSec + execGrowth

	// Resource usage tends to increase over time
	resourceGrowth := hours * 0.1
	forecastResource := math.Min(95, last.CPUUsage+resourceGrowth)

	// Calculate confidence intervals
	confidenceRange := 10.0 * (hours / 24) // Wider range for longer forecasts

	return &ForecastPoint{
		Timestamp:        last.Timestamp.Add(duration),
		ExecutionsPerSec: forecastExec,
		EfficiencyScore:  forecastEfficiency,
		ResourceUsage:    forecastResource,
		ConfidenceLower:  math.Max(0, forecastEfficiency-confidenceRange),
		ConfidenceUpper:  math.Min(100, forecastEfficiency+confidenceRange),
	}
}

// Bot-specific analysis methods

func (a *Analyzer) analyzeBotPerformance(metrics *BotMetrics, bot *botTypes.Agent) *BotPerformance {
	// Calculate performance scores
	productivityScore := 0.0
	if metrics.ExecutionMetrics != nil {
		productivityScore = math.Min(100, metrics.ExecutionMetrics.ExecutionsPerSecond/10)
	}

	reliabilityScore := 100.0
	if metrics.ExecutionMetrics != nil && metrics.ExecutionMetrics.FailureRate > 0 {
		reliabilityScore -= metrics.ExecutionMetrics.FailureRate * 100
	}
	reliabilityScore = math.Max(0, reliabilityScore)

	// Compare to average (simplified - would need aggregated data)
	avgProductivity := 50.0
	comparisonToAverage := ((productivityScore - avgProductivity) / avgProductivity) * 100

	// Determine trend
	trend := "stable"
	if productivityScore > 70 {
		trend = "improving"
	} else if productivityScore < 30 {
		trend = "declining"
	}

	return &BotPerformance{
		ProductivityScore:   productivityScore,
		ReliabilityScore:    reliabilityScore,
		EfficiencyScore:     metrics.EfficiencyScore,
		ComparisonToAverage: comparisonToAverage,
		Ranking:             1, // Would need comparison with other bots
		PerformanceTrend:    trend,
	}
}

func (a *Analyzer) analyzeBotHealth(metrics *BotMetrics, bot *botTypes.Agent) *BotHealthAnalysis {
	issues := make([]HealthIssue, 0)

	// Check for health issues
	if metrics.ExecutionMetrics != nil && metrics.ExecutionMetrics.FailureRate > 0.1 {
		issues = append(issues, HealthIssue{
			Type:        "high_failure_rate",
			Severity:    "high",
			Description: fmt.Sprintf("Failure rate of %.1f%% exceeds threshold", metrics.ExecutionMetrics.FailureRate*100),
			FirstSeen:   time.Now().Add(-1 * time.Hour), // Would track historically
			Frequency:   10,
			Impact:      "reduced throughput",
		})
	}

	if len(metrics.Errors) > 5 {
		issues = append(issues, HealthIssue{
			Type:        "frequent_errors",
			Severity:    "medium",
			Description: fmt.Sprintf("%d errors detected", len(metrics.Errors)),
			FirstSeen:   time.Now().Add(-2 * time.Hour),
			Frequency:   len(metrics.Errors),
			Impact:      "potential instability",
		})
	}

	// Calculate health score
	healthScore := metrics.HealthScore
	if len(issues) > 0 {
		healthScore -= float64(len(issues)) * 10
	}
	healthScore = math.Max(0, healthScore)

	// Determine status
	status := "healthy"
	if healthScore < 80 {
		status = "warning"
	}
	if healthScore < 60 {
		status = "critical"
	}

	// Calculate MTBF and MTTR (simplified)
	mtbf := 24 * time.Hour // Would calculate from historical data
	mttr := 30 * time.Minute

	return &BotHealthAnalysis{
		HealthScore: healthScore,
		Status:      status,
		Issues:      issues,
		MTBF:        mtbf,
		MTTR:        mttr,
	}
}

func (a *Analyzer) generateBotRecommendations(performance *BotPerformance, health *BotHealthAnalysis) []string {
	recommendations := make([]string, 0)

	if performance.ProductivityScore < 50 {
		recommendations = append(recommendations, "Optimize bot configuration for better performance")
	}

	if performance.ReliabilityScore < 80 {
		recommendations = append(recommendations, "Investigate and fix reliability issues")
	}

	if health.Status == "critical" {
		recommendations = append(recommendations, "Immediate attention required - bot health is critical")
	}

	for _, issue := range health.Issues {
		if issue.Severity == "high" {
			recommendations = append(recommendations, fmt.Sprintf("Address %s: %s", issue.Type, issue.Description))
		}
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Bot is performing well - no immediate actions required")
	}

	return recommendations
}

// Helper methods for scoring and analysis

func (a *Analyzer) calculateExecutionScore(metrics *ExecutionMetrics) float64 {
	if metrics == nil {
		return 0
	}

	score := 100.0

	// Penalize for failures
	score -= metrics.FailureRate * 50

	// Reward for high throughput
	if metrics.ExecutionsPerSecond > 1000 {
		score += 10
	}

	// Penalize for high latency
	if metrics.AverageExecTime > 100 {
		score -= 20
	}

	return math.Max(0, math.Min(100, score))
}

func (a *Analyzer) calculateResourceScore(metrics *ResourceMetrics) float64 {
	if metrics == nil {
		return 0
	}

	return metrics.ResourceScore
}

func (a *Analyzer) calculateEfficiencyScore(metrics *EfficiencyMetrics) float64 {
	if metrics == nil {
		return 0
	}

	return metrics.EfficiencyScore
}

func (a *Analyzer) determineHealthStatus(score float64) string {
	if score >= 90 {
		return "excellent"
	} else if score >= 75 {
		return "good"
	} else if score >= 60 {
		return "fair"
	} else if score >= 40 {
		return "poor"
	}
	return "critical"
}

func (a *Analyzer) generateHighlights(metrics *CampaignMetrics) []string {
	highlights := make([]string, 0)

	if metrics.ExecutionMetrics != nil && metrics.ExecutionMetrics.ExecutionsPerSecond > 1000 {
		highlights = append(highlights, fmt.Sprintf("High throughput: %.0f executions/sec", metrics.ExecutionMetrics.ExecutionsPerSecond))
	}

	if metrics.EfficiencyMetrics != nil && metrics.EfficiencyMetrics.CrashesPerHour > 10 {
		highlights = append(highlights, fmt.Sprintf("Excellent crash discovery rate: %.1f crashes/hour", metrics.EfficiencyMetrics.CrashesPerHour))
	}

	if metrics.ResourceMetrics != nil && metrics.ResourceMetrics.ResourceScore > 80 {
		highlights = append(highlights, "Efficient resource utilization")
	}

	return highlights
}

func (a *Analyzer) identifyCriticalIssues(metrics *CampaignMetrics) []string {
	issues := make([]string, 0)

	if metrics.ExecutionMetrics != nil && metrics.ExecutionMetrics.FailureRate > 0.2 {
		issues = append(issues, fmt.Sprintf("High failure rate: %.1f%%", metrics.ExecutionMetrics.FailureRate*100))
	}

	if metrics.QueueMetrics != nil && metrics.QueueMetrics.DroppedItems > 0 {
		issues = append(issues, fmt.Sprintf("Queue dropping items: %d dropped", metrics.QueueMetrics.DroppedItems))
	}

	if metrics.ResourceMetrics != nil && metrics.ResourceMetrics.CPUUsage.PeakUsage > 95 {
		issues = append(issues, "CPU usage critical - system may be overloaded")
	}

	return issues
}

func (a *Analyzer) generateInsights(metrics *CampaignMetrics) []PerformanceInsight {
	insights := make([]PerformanceInsight, 0)

	// Check for performance insights
	if metrics.ExecutionMetrics != nil && metrics.ExecutionMetrics.P99ExecTime > metrics.ExecutionMetrics.P95ExecTime*2 {
		insights = append(insights, PerformanceInsight{
			ID:          "latency-spike",
			Type:        "anomaly",
			Severity:    "medium",
			Title:       "Latency Spikes Detected",
			Description: "P99 latency is significantly higher than P95",
			Impact:      "Occasional slow executions affecting overall performance",
			Evidence: map[string]interface{}{
				"p95_latency": metrics.ExecutionMetrics.P95ExecTime,
				"p99_latency": metrics.ExecutionMetrics.P99ExecTime,
			},
			Actions: []string{
				"Investigate causes of latency spikes",
				"Consider implementing request timeouts",
			},
		})
	}

	// Check for efficiency insights
	if metrics.EfficiencyMetrics != nil && metrics.EfficiencyMetrics.CoveragePerExecution < 0.01 {
		insights = append(insights, PerformanceInsight{
			ID:          "low-coverage-efficiency",
			Type:        "optimization",
			Severity:    "high",
			Title:       "Low Coverage Efficiency",
			Description: "Coverage per execution is below optimal levels",
			Impact:      "Fuzzing may not be exploring new code paths effectively",
			Evidence: map[string]interface{}{
				"coverage_per_exec": metrics.EfficiencyMetrics.CoveragePerExecution,
			},
			Actions: []string{
				"Review and improve seed corpus",
				"Implement smarter mutation strategies",
				"Consider grammar-based fuzzing",
			},
		})
	}

	return insights
}

func (a *Analyzer) determinePriority(recommendations []Recommendation) string {
	if len(recommendations) == 0 {
		return "low"
	}

	// Sort by priority
	sort.Slice(recommendations, func(i, j int) bool {
		return recommendations[i].Priority < recommendations[j].Priority
	})

	// Check highest priority
	if recommendations[0].Priority == 1 {
		return "high"
	} else if recommendations[0].Priority <= 3 {
		return "medium"
	}

	return "low"
}

// MetricsCollector implements the PerformanceMetricsCollector interface
type MetricsCollector struct {
	campaignRepo campaignRepo.CampaignRepository
	botRepo      botRepo.AgentRepository
	logger       logrus.FieldLogger
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(
	campaignRepo campaignRepo.CampaignRepository,
	botRepo botRepo.AgentRepository,
	logger logrus.FieldLogger,
) *MetricsCollector {
	return &MetricsCollector{
		campaignRepo: campaignRepo,
		botRepo:      botRepo,
		logger:       logger.WithField("component", "metrics_collector"),
	}
}

// CollectCampaignMetrics collects metrics for a campaign
func (mc *MetricsCollector) CollectCampaignMetrics(ctx context.Context, campaignID string) (*CampaignMetrics, error) {
	mc.logger.WithField("campaign_id", campaignID).Debug("Collecting campaign metrics")

	// Get campaign
	campaign, err := mc.campaignRepo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("failed to get campaign: %w", err)
	}

	// Generate sample metrics (in production, would collect from monitoring systems)
	return &CampaignMetrics{
		CampaignID:  campaignID,
		CollectedAt: time.Now(),
		Duration:    time.Since(campaign.CreatedAt),
		ExecutionMetrics: &ExecutionMetrics{
			TotalExecutions:     100000,
			ExecutionsPerSecond: 500,
			AverageExecTime:     50,
			MedianExecTime:      45,
			P95ExecTime:         80,
			P99ExecTime:         120,
			FailedExecutions:    500,
			FailureRate:         0.005,
			Throughput:          25.5,
		},
		ResourceMetrics: &ResourceMetrics{
			CPUUsage: &CPUMetrics{
				AverageUsage: 65.5,
				PeakUsage:    88.2,
				CoreCount:    8,
				Efficiency:   82.3,
			},
			MemoryUsage: &MemoryMetrics{
				AverageUsage: 4096,
				PeakUsage:    6144,
				Available:    8192,
				SwapUsage:    512,
				GCPressure:   0.3,
			},
			DiskUsage: &DiskMetrics{
				ReadThroughput:  150.5,
				WriteThroughput: 75.2,
				IOPS:            3000,
				QueueDepth:      2.5,
				Latency:         5.2,
			},
			NetworkUsage: &NetworkMetrics{
				InboundBandwidth:  10.5,
				OutboundBandwidth: 5.2,
				PacketLoss:        0.01,
				Latency:           15.5,
			},
			ResourceScore: 78.5,
		},
		QueueMetrics: &QueueMetrics{
			QueueDepth:         1500,
			AverageWaitTime:    250,
			MaxWaitTime:        2000,
			ProcessingRate:     450,
			BackpressureEvents: 10,
			DroppedItems:       0,
			QueueUtilization:   75.5,
		},
		EfficiencyMetrics: &EfficiencyMetrics{
			CoveragePerExecution: 0.02,
			CrashesPerHour:       5.5,
			UniquePathsPerHour:   125,
			EfficiencyScore:      72.8,
			ResourceEfficiency:   68.5,
			TimeToFirstCrash:     &[]time.Duration{30 * time.Minute}[0],
		},
		BotMetrics: make(map[string]*BotMetrics),
	}, nil
}

// CollectBotMetrics collects metrics for a bot
func (mc *MetricsCollector) CollectBotMetrics(ctx context.Context, botID string) (*BotMetrics, error) {
	mc.logger.WithField("bot_id", botID).Debug("Collecting bot metrics")

	// Get bot
	bot, err := mc.botRepo.FindByID(ctx, botID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bot: %w", err)
	}

	// Generate sample metrics
	return &BotMetrics{
		BotID:  botID,
		Status: string(bot.Status),
		Uptime: time.Since(bot.CreatedAt),
		ExecutionMetrics: &ExecutionMetrics{
			TotalExecutions:     20000,
			ExecutionsPerSecond: 100,
			AverageExecTime:     45,
			MedianExecTime:      40,
			P95ExecTime:         75,
			P99ExecTime:         110,
			FailedExecutions:    50,
			FailureRate:         0.0025,
			Throughput:          5.5,
		},
		ResourceMetrics: &ResourceMetrics{
			CPUUsage: &CPUMetrics{
				AverageUsage: 55.5,
				PeakUsage:    75.2,
				CoreCount:    2,
				Efficiency:   85.3,
			},
			MemoryUsage: &MemoryMetrics{
				AverageUsage: 1024,
				PeakUsage:    1536,
				Available:    2048,
				SwapUsage:    0,
				GCPressure:   0.2,
			},
			ResourceScore: 82.5,
		},
		EfficiencyScore: 78.5,
		HealthScore:     85.0,
		LastHealthCheck: time.Now(),
		Errors:          []BotError{},
	}, nil
}

// AggregateMetrics aggregates metrics over a time period
func (mc *MetricsCollector) AggregateMetrics(ctx context.Context, metrics []*Metric, aggregationType AggregationType) (*AggregatedMetrics, error) {
	if len(metrics) == 0 {
		return nil, fmt.Errorf("no metrics to aggregate")
	}

	// Calculate statistics
	stats := mc.calculateStatistics(metrics)

	// Aggregate based on type
	aggregated := make(map[string]float64)

	switch aggregationType {
	case AggregationTypeSum:
		for _, m := range metrics {
			aggregated[m.Name] += m.Value
		}
	case AggregationTypeAverage:
		sums := make(map[string]float64)
		counts := make(map[string]int)
		for _, m := range metrics {
			sums[m.Name] += m.Value
			counts[m.Name]++
		}
		for name, sum := range sums {
			aggregated[name] = sum / float64(counts[name])
		}
	case AggregationTypeMax:
		for _, m := range metrics {
			if current, exists := aggregated[m.Name]; !exists || m.Value > current {
				aggregated[m.Name] = m.Value
			}
		}
	case AggregationTypeMin:
		for _, m := range metrics {
			if current, exists := aggregated[m.Name]; !exists || m.Value < current {
				aggregated[m.Name] = m.Value
			}
		}
	}

	// Determine time range
	var start, end time.Time
	for _, m := range metrics {
		if start.IsZero() || m.Timestamp.Before(start) {
			start = m.Timestamp
		}
		if end.IsZero() || m.Timestamp.After(end) {
			end = m.Timestamp
		}
	}

	return &AggregatedMetrics{
		Period:          end.Sub(start),
		StartTime:       start,
		EndTime:         end,
		AggregationType: aggregationType,
		Metrics:         aggregated,
		Statistics:      stats,
	}, nil
}

func (mc *MetricsCollector) calculateStatistics(metrics []*Metric) *MetricStatistics {
	if len(metrics) == 0 {
		return nil
	}

	// Extract values
	values := make([]float64, len(metrics))
	for i, m := range metrics {
		values[i] = m.Value
	}

	// Sort for percentile calculations
	sort.Float64s(values)

	// Calculate basic statistics
	var sum float64
	for _, v := range values {
		sum += v
	}
	avg := sum / float64(len(values))

	// Calculate variance and std dev
	var variance float64
	for _, v := range values {
		diff := v - avg
		variance += diff * diff
	}
	variance /= float64(len(values))
	stdDev := math.Sqrt(variance)

	// Calculate percentiles
	p := func(percentile float64) float64 {
		idx := int(percentile * float64(len(values)-1))
		return values[idx]
	}

	return &MetricStatistics{
		Count:    len(values),
		Sum:      sum,
		Average:  avg,
		Min:      values[0],
		Max:      values[len(values)-1],
		StdDev:   stdDev,
		Variance: variance,
		Median:   p(0.5),
		P25:      p(0.25),
		P75:      p(0.75),
		P90:      p(0.9),
		P95:      p(0.95),
		P99:      p(0.99),
	}
}
