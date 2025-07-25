package coverage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	campaignTypes "github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
	crashRepo "github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	crashTypes "github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
	"github.com/sirupsen/logrus"
)

// Analyzer implements the CoverageAnalyzer interface
type Analyzer struct {
	campaignRepo repository.CampaignRepository
	crashRepo    crashRepo.CrashRepository
	reporter     *Reporter
	logger       logrus.FieldLogger
}

// NewAnalyzer creates a new coverage analyzer
func NewAnalyzer(
	campaignRepo repository.CampaignRepository,
	crashRepo crashRepo.CrashRepository,
	logger logrus.FieldLogger,
) *Analyzer {
	return &Analyzer{
		campaignRepo: campaignRepo,
		crashRepo:    crashRepo,
		reporter:     NewReporter(logger),
		logger:       logger.WithField("component", "coverage_analyzer"),
	}
}

// AnalyzeCampaign analyzes coverage data for a specific campaign
func (a *Analyzer) AnalyzeCampaign(ctx context.Context, campaignID string) (*CoverageReport, error) {
	a.logger.WithField("campaign_id", campaignID).Debug("Analyzing campaign coverage")

	// Get campaign data
	campaign, err := a.campaignRepo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("failed to get campaign: %w", err)
	}

	// Get crashes for the campaign
	crashes, err := a.getCampaignCrashes(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("failed to get crashes: %w", err)
	}

	// Calculate time range
	timeRange := TimeRange{
		Start: campaign.CreatedAt,
		End:   time.Now(),
	}

	// Generate coverage summary
	summary := a.generateCoverageSummary(campaign, crashes)

	// Generate coverage details
	details := a.generateCoverageDetails(crashes)

	// Generate coverage breakdown
	breakdown := a.generateCoverageBreakdown(crashes)

	// Generate insights
	insights := a.generateInsights(summary, details, breakdown)

	// Generate trends if enough data
	var trends *CoverageTrendData
	if time.Since(campaign.CreatedAt) > 24*time.Hour {
		trends = a.generateTrends(campaign, crashes, TrendPeriodHourly)
	}

	report := &CoverageReport{
		ID:          generateReportID(campaignID),
		CampaignID:  campaignID,
		GeneratedAt: time.Now(),
		TimeRange:   timeRange,
		Summary:     summary,
		Details:     details,
		Breakdown:   breakdown,
		Trends:      trends,
		Insights:    insights,
		Metadata: map[string]interface{}{
			"campaign_name":   campaign.Name,
			"campaign_status": string(campaign.Status),
			"total_crashes":   len(crashes),
		},
	}

	return report, nil
}

// AnalyzeTimeRange analyzes coverage data within a time range
func (a *Analyzer) AnalyzeTimeRange(ctx context.Context, start, end time.Time) (*CoverageReport, error) {
	a.logger.WithFields(logrus.Fields{
		"start": start,
		"end":   end,
	}).Debug("Analyzing coverage for time range")

	// Get all active campaigns in the time range
	campaigns, err := a.getActiveCampaignsInRange(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get campaigns: %w", err)
	}

	// Aggregate coverage data across campaigns
	var allCrashes []*crashTypes.Crash
	campaignNames := make([]string, 0)

	for _, campaign := range campaigns {
		crashes, err := a.getCampaignCrashesInRange(ctx, campaign.ID, start, end)
		if err != nil {
			a.logger.WithError(err).WithField("campaign_id", campaign.ID).Warn("Failed to get crashes")
			continue
		}
		allCrashes = append(allCrashes, crashes...)
		campaignNames = append(campaignNames, campaign.Name)
	}

	// Generate aggregated report
	summary := a.generateAggregatedSummary(allCrashes)
	details := a.generateCoverageDetails(allCrashes)
	breakdown := a.generateCoverageBreakdown(allCrashes)
	insights := a.generateInsights(summary, details, breakdown)

	report := &CoverageReport{
		ID:          generateTimeRangeReportID(start, end),
		GeneratedAt: time.Now(),
		TimeRange: TimeRange{
			Start: start,
			End:   end,
		},
		Summary:   summary,
		Details:   details,
		Breakdown: breakdown,
		Insights:  insights,
		Metadata: map[string]interface{}{
			"campaigns_analyzed": len(campaigns),
			"campaign_names":     campaignNames,
			"total_crashes":      len(allCrashes),
		},
	}

	return report, nil
}

// CompareCampaigns compares coverage between multiple campaigns
func (a *Analyzer) CompareCampaigns(ctx context.Context, campaignIDs []string) (*CoverageComparison, error) {
	a.logger.WithField("campaign_ids", campaignIDs).Debug("Comparing campaign coverage")

	if len(campaignIDs) < 2 {
		return nil, fmt.Errorf("at least 2 campaigns required for comparison")
	}

	// Get coverage reports for each campaign
	summaries := make(map[string]*CoverageSummary)
	var minStart, maxEnd time.Time

	for _, id := range campaignIDs {
		report, err := a.AnalyzeCampaign(ctx, id)
		if err != nil {
			a.logger.WithError(err).WithField("campaign_id", id).Warn("Failed to analyze campaign")
			continue
		}
		summaries[id] = report.Summary

		if minStart.IsZero() || report.TimeRange.Start.Before(minStart) {
			minStart = report.TimeRange.Start
		}
		if report.TimeRange.End.After(maxEnd) {
			maxEnd = report.TimeRange.End
		}
	}

	if len(summaries) < 2 {
		return nil, fmt.Errorf("insufficient valid campaigns for comparison")
	}

	// Calculate differences
	differences := a.calculateDifferences(summaries)

	// Generate rankings
	rankings := a.generateRankings(summaries)

	// Find common patterns
	patterns := a.findCommonPatterns(summaries)

	// Generate recommendations
	recommendations := a.generateComparisonRecommendations(summaries, differences)

	comparison := &CoverageComparison{
		CampaignIDs: campaignIDs,
		GeneratedAt: time.Now(),
		TimeRange: TimeRange{
			Start: minStart,
			End:   maxEnd,
		},
		Summaries:       summaries,
		Differences:     differences,
		Rankings:        rankings,
		CommonPatterns:  patterns,
		Recommendations: recommendations,
	}

	return comparison, nil
}

// GenerateReport generates a coverage report in the specified format
func (a *Analyzer) GenerateReport(ctx context.Context, report *CoverageReport, format ReportFormat) (io.Reader, error) {
	switch format {
	case ReportFormatJSON:
		return a.reporter.GenerateJSONReport(ctx, report)
	case ReportFormatHTML:
		return a.reporter.GenerateHTMLReport(ctx, report)
	case ReportFormatMarkdown:
		return a.reporter.GenerateMarkdownReport(ctx, report)
	default:
		return nil, fmt.Errorf("unsupported report format: %s", format)
	}
}

// Helper methods

func (a *Analyzer) getCampaignCrashes(ctx context.Context, campaignID string) ([]*crashTypes.Crash, error) {
	// In a real implementation, this would query crashes associated with the campaign
	// For now, return empty slice
	return []*crashTypes.Crash{}, nil
}

func (a *Analyzer) getCampaignCrashesInRange(ctx context.Context, campaignID string, start, end time.Time) ([]*crashTypes.Crash, error) {
	// In a real implementation, this would query crashes within time range
	return []*crashTypes.Crash{}, nil
}

func (a *Analyzer) getActiveCampaignsInRange(ctx context.Context, start, end time.Time) ([]*campaignTypes.Campaign, error) {
	// In a real implementation, this would query active campaigns in range
	return []*campaignTypes.Campaign{}, nil
}

func (a *Analyzer) generateCoverageSummary(campaign *campaignTypes.Campaign, crashes []*crashTypes.Crash) *CoverageSummary {
	// Calculate coverage metrics based on crash data
	// This is a simplified implementation
	totalEdges := int64(10000) // Would come from actual coverage data
	coveredEdges := int64(len(crashes) * 100)
	totalFunctions := int64(1000)
	coveredFunctions := int64(len(crashes) * 10)
	totalBranches := int64(5000)
	coveredBranches := int64(len(crashes) * 50)

	coverage := float64(coveredEdges) / float64(totalEdges) * 100
	lineCoverage := coverage * 0.95
	functionCoverage := float64(coveredFunctions) / float64(totalFunctions) * 100
	branchCoverage := float64(coveredBranches) / float64(totalBranches) * 100

	// Calculate growth rate
	campaignDuration := time.Since(campaign.CreatedAt).Hours()
	growthRate := coverage / campaignDuration

	// Calculate quality score
	qualityScore := a.calculateQualityScore(coverage, functionCoverage, branchCoverage)

	return &CoverageSummary{
		TotalCoverage:       coverage,
		LineCoverage:        lineCoverage,
		FunctionCoverage:    functionCoverage,
		BranchCoverage:      branchCoverage,
		TotalEdges:          totalEdges,
		CoveredEdges:        coveredEdges,
		TotalFunctions:      totalFunctions,
		CoveredFunctions:    coveredFunctions,
		TotalBranches:       totalBranches,
		CoveredBranches:     coveredBranches,
		NewCoverageFound:    int64(len(crashes)),
		CoverageGrowthRate:  growthRate,
		EstimatedCompletion: math.Min(coverage/80*100, 100), // Assume 80% is practical max
		QualityScore:        qualityScore,
	}
}

func (a *Analyzer) generateAggregatedSummary(crashes []*crashTypes.Crash) *CoverageSummary {
	// Similar to generateCoverageSummary but without campaign-specific data
	totalEdges := int64(15000)
	coveredEdges := int64(len(crashes) * 150)
	totalFunctions := int64(1500)
	coveredFunctions := int64(len(crashes) * 15)
	totalBranches := int64(7500)
	coveredBranches := int64(len(crashes) * 75)

	coverage := float64(coveredEdges) / float64(totalEdges) * 100
	lineCoverage := coverage * 0.95
	functionCoverage := float64(coveredFunctions) / float64(totalFunctions) * 100
	branchCoverage := float64(coveredBranches) / float64(totalBranches) * 100

	qualityScore := a.calculateQualityScore(coverage, functionCoverage, branchCoverage)

	return &CoverageSummary{
		TotalCoverage:       coverage,
		LineCoverage:        lineCoverage,
		FunctionCoverage:    functionCoverage,
		BranchCoverage:      branchCoverage,
		TotalEdges:          totalEdges,
		CoveredEdges:        coveredEdges,
		TotalFunctions:      totalFunctions,
		CoveredFunctions:    coveredFunctions,
		TotalBranches:       totalBranches,
		CoveredBranches:     coveredBranches,
		NewCoverageFound:    int64(len(crashes)),
		CoverageGrowthRate:  0, // Not applicable for aggregated
		EstimatedCompletion: math.Min(coverage/80*100, 100),
		QualityScore:        qualityScore,
	}
}

func (a *Analyzer) generateCoverageDetails(crashes []*crashTypes.Crash) *CoverageDetails {
	// Generate detailed coverage information
	// This is a simplified implementation
	byModule := make(map[string]*ModuleCoverage)
	byFunction := make(map[string]*FunctionCoverage)
	byFile := make(map[string]*FileCoverage)

	// Example module coverage
	byModule["core"] = &ModuleCoverage{
		Name:             "core",
		Path:             "/src/core",
		TotalCoverage:    75.5,
		LineCoverage:     72.3,
		FunctionCoverage: 80.2,
		BranchCoverage:   70.1,
		Complexity:       150,
		Risk:             "medium",
	}

	// Generate hotspots and coldspots
	hotSpots := a.identifyHotSpots(crashes)
	coldSpots := a.identifyColdSpots()
	recentChanges := a.getRecentCoverageChanges(crashes)

	return &CoverageDetails{
		ByModule:      byModule,
		ByFunction:    byFunction,
		ByFile:        byFile,
		HotSpots:      hotSpots,
		ColdSpots:     coldSpots,
		RecentChanges: recentChanges,
	}
}

func (a *Analyzer) generateCoverageBreakdown(crashes []*crashTypes.Crash) *CoverageBreakdown {
	// Generate coverage breakdown by various dimensions
	return &CoverageBreakdown{
		ByComplexity: map[string]float64{
			"low":    85.5,
			"medium": 72.3,
			"high":   45.6,
		},
		ByRisk: map[string]float64{
			"low":      82.1,
			"medium":   68.4,
			"high":     52.3,
			"critical": 38.9,
		},
		ByAge: map[string]float64{
			"new":    78.9,
			"recent": 74.5,
			"old":    65.2,
			"legacy": 45.8,
		},
		ByType: map[string]float64{
			"unit":        89.2,
			"integration": 76.5,
			"system":      62.3,
			"e2e":         54.1,
		},
	}
}

func (a *Analyzer) generateInsights(summary *CoverageSummary, details *CoverageDetails, breakdown *CoverageBreakdown) []CoverageInsight {
	insights := make([]CoverageInsight, 0)

	// Check for low coverage areas
	if summary.TotalCoverage < 50 {
		insights = append(insights, CoverageInsight{
			ID:          "low-coverage",
			Type:        InsightTypeRisk,
			Severity:    InsightSeverityHigh,
			Title:       "Low Overall Coverage",
			Description: fmt.Sprintf("Total coverage is %.1f%%, which is below the recommended threshold", summary.TotalCoverage),
			Impact:      "Increased risk of undetected bugs in production",
			Evidence: map[string]interface{}{
				"total_coverage": summary.TotalCoverage,
				"threshold":      50.0,
			},
			Actions: []string{
				"Increase fuzzing duration",
				"Add more diverse seed inputs",
				"Review and optimize fuzzing configuration",
			},
		})
	}

	// Check for high-risk uncovered areas
	if riskCoverage, ok := breakdown.ByRisk["critical"]; ok && riskCoverage < 50 {
		insights = append(insights, CoverageInsight{
			ID:          "critical-risk-low-coverage",
			Type:        InsightTypeRisk,
			Severity:    InsightSeverityCritical,
			Title:       "Critical Risk Areas Have Low Coverage",
			Description: fmt.Sprintf("Critical risk areas have only %.1f%% coverage", riskCoverage),
			Impact:      "High probability of critical bugs in sensitive code paths",
			Evidence: map[string]interface{}{
				"critical_coverage": riskCoverage,
				"threshold":         80.0,
			},
			Actions: []string{
				"Prioritize fuzzing of critical components",
				"Implement targeted fuzzing for high-risk areas",
				"Review security-sensitive code paths",
			},
		})
	}

	// Check for coverage plateau
	if summary.CoverageGrowthRate < 0.1 {
		insights = append(insights, CoverageInsight{
			ID:          "coverage-plateau",
			Type:        InsightTypeTrend,
			Severity:    InsightSeverityMedium,
			Title:       "Coverage Growth Has Plateaued",
			Description: "Coverage growth rate has slowed significantly",
			Impact:      "Diminishing returns from current fuzzing strategy",
			Evidence: map[string]interface{}{
				"growth_rate": summary.CoverageGrowthRate,
				"threshold":   0.1,
			},
			Actions: []string{
				"Try different mutation strategies",
				"Add grammar-based fuzzing",
				"Implement structure-aware mutations",
			},
		})
	}

	// Positive insights
	if summary.QualityScore > 80 {
		insights = append(insights, CoverageInsight{
			ID:          "high-quality-coverage",
			Type:        InsightTypeAchievement,
			Severity:    InsightSeverityInfo,
			Title:       "High Quality Coverage Achieved",
			Description: fmt.Sprintf("Coverage quality score is %.1f, indicating well-distributed coverage", summary.QualityScore),
			Impact:      "Good test effectiveness and bug detection capability",
			Evidence: map[string]interface{}{
				"quality_score": summary.QualityScore,
			},
		})
	}

	return insights
}

func (a *Analyzer) generateTrends(campaign *campaignTypes.Campaign, crashes []*crashTypes.Crash, period TrendPeriod) *CoverageTrendData {
	// Generate trend data points
	dataPoints := a.generateTrendDataPoints(campaign, crashes, period)

	// Analyze growth patterns
	growth := a.analyzeGrowthPatterns(dataPoints)

	// Generate projections
	projections := a.generateProjections(dataPoints, growth)

	return &CoverageTrendData{
		Period:      period,
		DataPoints:  dataPoints,
		Growth:      growth,
		Projections: projections,
	}
}

func (a *Analyzer) generateTrendDataPoints(campaign *campaignTypes.Campaign, crashes []*crashTypes.Crash, period TrendPeriod) []CoverageTrendPoint {
	// Generate sample trend points
	points := make([]CoverageTrendPoint, 0)

	start := campaign.CreatedAt
	end := time.Now()

	var interval time.Duration
	switch period {
	case TrendPeriodHourly:
		interval = time.Hour
	case TrendPeriodDaily:
		interval = 24 * time.Hour
	case TrendPeriodWeekly:
		interval = 7 * 24 * time.Hour
	default:
		interval = time.Hour
	}

	totalEdges := int64(0)
	for current := start; current.Before(end); current = current.Add(interval) {
		// Simulate coverage growth
		elapsed := current.Sub(start).Hours()
		coverage := math.Min(80, 10+elapsed*2-elapsed*elapsed/1000)
		newEdges := int64(math.Max(0, 100-elapsed/10))
		totalEdges += newEdges

		points = append(points, CoverageTrendPoint{
			Timestamp:        current,
			TotalCoverage:    coverage,
			LineCoverage:     coverage * 0.95,
			FunctionCoverage: coverage * 1.1,
			BranchCoverage:   coverage * 0.85,
			NewEdges:         newEdges,
			TotalEdges:       totalEdges,
		})
	}

	return points
}

func (a *Analyzer) analyzeGrowthPatterns(dataPoints []CoverageTrendPoint) *GrowthAnalysis {
	if len(dataPoints) < 2 {
		return nil
	}

	// Calculate growth rates
	totalGrowth := dataPoints[len(dataPoints)-1].TotalCoverage - dataPoints[0].TotalCoverage
	timeSpan := dataPoints[len(dataPoints)-1].Timestamp.Sub(dataPoints[0].Timestamp).Hours()
	avgGrowthRate := totalGrowth / timeSpan

	// Current growth rate (last few points)
	recentPoints := 5
	if len(dataPoints) < recentPoints {
		recentPoints = len(dataPoints)
	}
	recentGrowth := dataPoints[len(dataPoints)-1].TotalCoverage - dataPoints[len(dataPoints)-recentPoints].TotalCoverage
	recentTimeSpan := dataPoints[len(dataPoints)-1].Timestamp.Sub(dataPoints[len(dataPoints)-recentPoints].Timestamp).Hours()
	currentGrowthRate := recentGrowth / recentTimeSpan

	// Growth acceleration
	acceleration := currentGrowthRate - avgGrowthRate

	// Estimate saturation
	saturationPoint := 85.0 // Typical saturation point
	currentCoverage := dataPoints[len(dataPoints)-1].TotalCoverage

	var timeToSaturation *time.Duration
	if currentGrowthRate > 0 && currentCoverage < saturationPoint {
		hours := (saturationPoint - currentCoverage) / currentGrowthRate
		duration := time.Duration(hours) * time.Hour
		timeToSaturation = &duration
	}

	// Determine growth pattern
	pattern := "steady"
	if acceleration > 0.1 {
		pattern = "accelerating"
	} else if acceleration < -0.1 {
		pattern = "decelerating"
	}
	if currentGrowthRate < 0.01 {
		pattern = "plateaued"
	}

	return &GrowthAnalysis{
		AverageGrowthRate:  avgGrowthRate,
		CurrentGrowthRate:  currentGrowthRate,
		GrowthAcceleration: acceleration,
		TimeToSaturation:   timeToSaturation,
		SaturationPoint:    saturationPoint,
		GrowthPattern:      pattern,
	}
}

func (a *Analyzer) generateProjections(dataPoints []CoverageTrendPoint, growth *GrowthAnalysis) *CoverageProjections {
	if growth == nil || len(dataPoints) == 0 {
		return nil
	}

	currentCoverage := dataPoints[len(dataPoints)-1].TotalCoverage
	growthRate := growth.CurrentGrowthRate

	// Apply decay factor for more realistic projections
	decayFactor := 0.95

	// Calculate projections
	oneDayGrowth := growthRate * 24 * decayFactor
	oneWeekGrowth := growthRate * 24 * 7 * math.Pow(decayFactor, 7)
	oneMonthGrowth := growthRate * 24 * 30 * math.Pow(decayFactor, 30)

	// Cap at saturation point
	oneDay := math.Min(growth.SaturationPoint, currentCoverage+oneDayGrowth)
	oneWeek := math.Min(growth.SaturationPoint, currentCoverage+oneWeekGrowth)
	oneMonth := math.Min(growth.SaturationPoint, currentCoverage+oneMonthGrowth)

	// Calculate confidence based on growth pattern stability
	confidence := 0.8
	if growth.GrowthPattern == "plateaued" {
		confidence = 0.9
	} else if growth.GrowthPattern == "accelerating" || growth.GrowthPattern == "decelerating" {
		confidence = 0.6
	}

	return &CoverageProjections{
		OneDay:      oneDay,
		OneWeek:     oneWeek,
		OneMonth:    oneMonth,
		Confidence:  confidence,
		Methodology: "exponential decay model",
		Assumptions: []string{
			"Current fuzzing configuration remains unchanged",
			"No major code changes",
			fmt.Sprintf("Growth rate decay factor: %.2f", decayFactor),
		},
	}
}

func (a *Analyzer) identifyHotSpots(crashes []*crashTypes.Crash) []CoverageHotSpot {
	// Identify areas with high coverage activity
	hotSpots := make([]CoverageHotSpot, 0)

	// Group crashes by location (simplified)
	locationMap := make(map[string][]*crashTypes.Crash)
	for _, crash := range crashes {
		// Extract location from stack trace (simplified)
		location := extractLocationFromStackTrace(crash.StackTrace)
		locationMap[location] = append(locationMap[location], crash)
	}

	// Find hotspots
	for location, locationCrashes := range locationMap {
		if len(locationCrashes) >= 5 {
			var firstHit, lastHit time.Time
			for _, crash := range locationCrashes {
				if firstHit.IsZero() || crash.DiscoveredAt.Before(firstHit) {
					firstHit = crash.DiscoveredAt
				}
				if crash.LastSeenAt.After(lastHit) {
					lastHit = crash.LastSeenAt
				}
			}

			hotSpots = append(hotSpots, CoverageHotSpot{
				Location:    location,
				Type:        "function",
				HitCount:    int64(len(locationCrashes)),
				Coverage:    85.5, // Would be calculated from actual coverage data
				Description: fmt.Sprintf("High activity area with %d hits", len(locationCrashes)),
				FirstHit:    firstHit,
				LastHit:     lastHit,
			})
		}
	}

	// Sort by hit count
	sort.Slice(hotSpots, func(i, j int) bool {
		return hotSpots[i].HitCount > hotSpots[j].HitCount
	})

	// Return top 10
	if len(hotSpots) > 10 {
		hotSpots = hotSpots[:10]
	}

	return hotSpots
}

func (a *Analyzer) identifyColdSpots() []CoverageColdSpot {
	// Identify areas with low coverage
	// This would analyze actual coverage data
	return []CoverageColdSpot{
		{
			Location:    "auth/validation.go",
			Type:        "module",
			Coverage:    15.5,
			Complexity:  85,
			Risk:        "high",
			Description: "Authentication validation has very low coverage",
			Suggestions: []string{
				"Add targeted fuzzing for authentication flows",
				"Create auth-specific seed inputs",
				"Implement property-based testing",
			},
		},
		{
			Location:    "crypto/encryption.go",
			Type:        "module",
			Coverage:    22.3,
			Complexity:  92,
			Risk:        "critical",
			Description: "Cryptographic functions have insufficient coverage",
			Suggestions: []string{
				"Use crypto-specific fuzzing strategies",
				"Add edge case testing for crypto operations",
				"Implement differential fuzzing",
			},
		},
	}
}

func (a *Analyzer) getRecentCoverageChanges(crashes []*crashTypes.Crash) []CoverageChange {
	// Get recent significant coverage changes
	changes := make([]CoverageChange, 0)

	// Sort crashes by time
	sort.Slice(crashes, func(i, j int) bool {
		return crashes[i].DiscoveredAt.After(crashes[j].DiscoveredAt)
	})

	// Take recent crashes and simulate coverage changes
	for i, crash := range crashes {
		if i >= 5 {
			break
		}

		changes = append(changes, CoverageChange{
			Timestamp:   crash.DiscoveredAt,
			Type:        "improvement",
			Location:    extractLocationFromStackTrace(crash.StackTrace),
			OldCoverage: 65.5,
			NewCoverage: 68.2,
			Delta:       2.7,
			Reason:      "New code path discovered",
		})
	}

	return changes
}

func (a *Analyzer) calculateDifferences(summaries map[string]*CoverageSummary) *CoverageDifferences {
	if len(summaries) < 2 {
		return nil
	}

	var maxDiff, minDiff, totalDiff float64
	minDiff = 100.0
	significantDiffs := make([]SignificantDifference, 0)

	// Compare all pairs
	ids := make([]string, 0, len(summaries))
	for id := range summaries {
		ids = append(ids, id)
	}

	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			summaryA := summaries[ids[i]]
			summaryB := summaries[ids[j]]

			diff := math.Abs(summaryA.TotalCoverage - summaryB.TotalCoverage)
			percentage := diff / math.Max(summaryA.TotalCoverage, summaryB.TotalCoverage) * 100

			if diff > maxDiff {
				maxDiff = diff
			}
			if diff < minDiff {
				minDiff = diff
			}
			totalDiff += diff

			// Check if significant
			if percentage > 10 {
				significantDiffs = append(significantDiffs, SignificantDifference{
					Metric:       "total_coverage",
					CampaignA:    ids[i],
					CampaignB:    ids[j],
					ValueA:       summaryA.TotalCoverage,
					ValueB:       summaryB.TotalCoverage,
					Difference:   diff,
					Percentage:   percentage,
					Significance: determineSignificance(percentage),
				})
			}
		}
	}

	avgDiff := totalDiff / float64(len(ids)*(len(ids)-1)/2)

	// Analyze convergence
	convergence := a.analyzeConvergence(summaries)

	return &CoverageDifferences{
		MaxDifference:       maxDiff,
		MinDifference:       minDiff,
		AverageDifference:   avgDiff,
		SignificantDiffs:    significantDiffs,
		ConvergenceAnalysis: convergence,
	}
}

func (a *Analyzer) analyzeConvergence(summaries map[string]*CoverageSummary) *ConvergenceAnalysis {
	// Check if campaigns are converging towards similar coverage
	coverages := make([]float64, 0, len(summaries))
	for _, summary := range summaries {
		coverages = append(coverages, summary.TotalCoverage)
	}

	// Calculate variance
	var sum, mean float64
	for _, cov := range coverages {
		sum += cov
	}
	mean = sum / float64(len(coverages))

	var variance float64
	for _, cov := range coverages {
		variance += math.Pow(cov-mean, 2)
	}
	variance /= float64(len(coverages))
	stdDev := math.Sqrt(variance)

	// Low standard deviation indicates convergence
	isConverging := stdDev < 5.0
	convergenceRate := 1.0 / (1.0 + stdDev/10.0) // Higher rate means faster convergence

	var estimatedConvergence *time.Time
	if isConverging && convergenceRate > 0.5 {
		// Estimate when campaigns will converge
		days := int(stdDev * 10) // Simplified estimation
		convergenceTime := time.Now().Add(time.Duration(days) * 24 * time.Hour)
		estimatedConvergence = &convergenceTime
	}

	return &ConvergenceAnalysis{
		IsConverging:         isConverging,
		ConvergenceRate:      convergenceRate,
		EstimatedConvergence: estimatedConvergence,
		ConvergencePoint:     mean,
		Metadata: map[string]interface{}{
			"standard_deviation": stdDev,
			"variance":           variance,
		},
	}
}

func (a *Analyzer) generateRankings(summaries map[string]*CoverageSummary) *CoverageRankings {
	// Create ranking entries for each metric
	entries := make([]RankingEntry, 0, len(summaries))

	for id, summary := range summaries {
		entries = append(entries, RankingEntry{
			CampaignID: id,
			Score:      summary.TotalCoverage,
		})
	}

	// Sort by total coverage
	byTotalCoverage := make([]RankingEntry, len(entries))
	copy(byTotalCoverage, entries)
	sort.Slice(byTotalCoverage, func(i, j int) bool {
		return byTotalCoverage[i].Score > byTotalCoverage[j].Score
	})
	for i := range byTotalCoverage {
		byTotalCoverage[i].Rank = i + 1
		byTotalCoverage[i].Details = fmt.Sprintf("%.1f%% total coverage", byTotalCoverage[i].Score)
	}

	// Sort by growth rate
	byGrowthRate := make([]RankingEntry, 0)
	for id, summary := range summaries {
		byGrowthRate = append(byGrowthRate, RankingEntry{
			CampaignID: id,
			Score:      summary.CoverageGrowthRate,
		})
	}
	sort.Slice(byGrowthRate, func(i, j int) bool {
		return byGrowthRate[i].Score > byGrowthRate[j].Score
	})
	for i := range byGrowthRate {
		byGrowthRate[i].Rank = i + 1
		byGrowthRate[i].Details = fmt.Sprintf("%.2f%%/hour growth rate", byGrowthRate[i].Score)
	}

	// Sort by efficiency (coverage per edge)
	byEfficiency := make([]RankingEntry, 0)
	for id, summary := range summaries {
		efficiency := float64(summary.CoveredEdges) / float64(summary.TotalEdges) * 100
		byEfficiency = append(byEfficiency, RankingEntry{
			CampaignID: id,
			Score:      efficiency,
		})
	}
	sort.Slice(byEfficiency, func(i, j int) bool {
		return byEfficiency[i].Score > byEfficiency[j].Score
	})
	for i := range byEfficiency {
		byEfficiency[i].Rank = i + 1
		byEfficiency[i].Details = fmt.Sprintf("%.1f%% edge efficiency", byEfficiency[i].Score)
	}

	// Sort by quality score
	byQuality := make([]RankingEntry, 0)
	for id, summary := range summaries {
		byQuality = append(byQuality, RankingEntry{
			CampaignID: id,
			Score:      summary.QualityScore,
		})
	}
	sort.Slice(byQuality, func(i, j int) bool {
		return byQuality[i].Score > byQuality[j].Score
	})
	for i := range byQuality {
		byQuality[i].Rank = i + 1
		byQuality[i].Details = fmt.Sprintf("%.1f quality score", byQuality[i].Score)
	}

	// Calculate overall ranking (weighted average of ranks)
	overallScores := make(map[string]float64)
	for id := range summaries {
		var totalRank float64
		var count float64

		// Find ranks for this campaign
		for _, entry := range byTotalCoverage {
			if entry.CampaignID == id {
				totalRank += float64(entry.Rank) * 0.4 // 40% weight
				count += 0.4
				break
			}
		}
		for _, entry := range byGrowthRate {
			if entry.CampaignID == id {
				totalRank += float64(entry.Rank) * 0.2 // 20% weight
				count += 0.2
				break
			}
		}
		for _, entry := range byEfficiency {
			if entry.CampaignID == id {
				totalRank += float64(entry.Rank) * 0.2 // 20% weight
				count += 0.2
				break
			}
		}
		for _, entry := range byQuality {
			if entry.CampaignID == id {
				totalRank += float64(entry.Rank) * 0.2 // 20% weight
				count += 0.2
				break
			}
		}

		// Lower average rank is better
		overallScores[id] = totalRank / count
	}

	// Create overall ranking
	overallRanking := make([]RankingEntry, 0)
	for id, score := range overallScores {
		overallRanking = append(overallRanking, RankingEntry{
			CampaignID: id,
			Score:      score,
		})
	}
	sort.Slice(overallRanking, func(i, j int) bool {
		return overallRanking[i].Score < overallRanking[j].Score // Lower is better for rank average
	})
	for i := range overallRanking {
		overallRanking[i].Rank = i + 1
		overallRanking[i].Details = fmt.Sprintf("Weighted rank score: %.1f", overallRanking[i].Score)
	}

	return &CoverageRankings{
		ByTotalCoverage: byTotalCoverage,
		ByGrowthRate:    byGrowthRate,
		ByEfficiency:    byEfficiency,
		ByQuality:       byQuality,
		OverallRanking:  overallRanking,
	}
}

func (a *Analyzer) findCommonPatterns(summaries map[string]*CoverageSummary) []Pattern {
	patterns := make([]Pattern, 0)

	// Check for common coverage levels
	var lowCoverage, highCoverage, plateaued int
	for _, summary := range summaries {
		if summary.TotalCoverage < 40 {
			lowCoverage++
		} else if summary.TotalCoverage > 70 {
			highCoverage++
		}
		if summary.CoverageGrowthRate < 0.1 {
			plateaued++
		}
	}

	if lowCoverage > len(summaries)/2 {
		patterns = append(patterns, Pattern{
			ID:          "low-coverage-pattern",
			Type:        "coverage_level",
			Description: "Majority of campaigns have low coverage",
			Frequency:   lowCoverage,
			Impact:      "System-wide testing gaps",
			Examples:    []string{"Consider reviewing fuzzing strategies", "Check seed corpus quality"},
		})
	}

	if plateaued > len(summaries)/2 {
		patterns = append(patterns, Pattern{
			ID:          "plateau-pattern",
			Type:        "growth_rate",
			Description: "Most campaigns have plateaued growth",
			Frequency:   plateaued,
			Impact:      "Diminishing returns across campaigns",
			Examples:    []string{"Implement new mutation strategies", "Add grammar-based fuzzing"},
		})
	}

	return patterns
}

func (a *Analyzer) generateComparisonRecommendations(summaries map[string]*CoverageSummary, differences *CoverageDifferences) []ComparisonRecommendation {
	recommendations := make([]ComparisonRecommendation, 0)

	// Find best and worst performers
	var bestID, worstID string
	var bestCoverage, worstCoverage float64 = 0, 100
	for id, summary := range summaries {
		if summary.TotalCoverage > bestCoverage {
			bestCoverage = summary.TotalCoverage
			bestID = id
		}
		if summary.TotalCoverage < worstCoverage {
			worstCoverage = summary.TotalCoverage
			worstID = id
		}
	}

	if differences.AverageDifference > 20 {
		recommendations = append(recommendations, ComparisonRecommendation{
			ID:           "standardize-config",
			Type:         "configuration",
			Priority:     1,
			Title:        "Standardize Fuzzing Configuration",
			Description:  fmt.Sprintf("Large coverage differences (avg %.1f%%) suggest inconsistent configurations", differences.AverageDifference),
			ForCampaigns: getAllCampaignIDs(summaries),
			Actions: []string{
				"Review and align fuzzing parameters",
				"Ensure consistent seed corpus",
				"Standardize mutation strategies",
			},
			Expected: "More consistent coverage across campaigns",
		})
	}

	if worstCoverage < 40 && bestCoverage > 60 {
		recommendations = append(recommendations, ComparisonRecommendation{
			ID:           "learn-from-best",
			Type:         "best_practice",
			Priority:     2,
			Title:        "Apply Best Practices from High-Performing Campaign",
			Description:  fmt.Sprintf("Campaign %s achieved %.1f%% coverage", bestID, bestCoverage),
			ForCampaigns: []string{worstID},
			Actions: []string{
				fmt.Sprintf("Analyze configuration of campaign %s", bestID),
				"Replicate successful strategies",
				"Consider using same seed corpus",
			},
			Expected: "Improved coverage for underperforming campaigns",
		})
	}

	return recommendations
}

func (a *Analyzer) calculateQualityScore(totalCoverage, functionCoverage, branchCoverage float64) float64 {
	// Weighted quality score
	weights := map[string]float64{
		"total":    0.3,
		"function": 0.4,
		"branch":   0.3,
	}

	score := totalCoverage*weights["total"] +
		functionCoverage*weights["function"] +
		branchCoverage*weights["branch"]

	// Apply penalties for imbalanced coverage
	imbalance := math.Abs(functionCoverage-branchCoverage) + math.Abs(totalCoverage-functionCoverage)
	penalty := imbalance / 100 * 10 // Max 10 point penalty

	return math.Max(0, math.Min(100, score-penalty))
}

// Helper functions

func generateReportID(campaignID string) string {
	timestamp := time.Now().Unix()
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", campaignID, timestamp)))
	return hex.EncodeToString(hash[:])[:16]
}

func generateTimeRangeReportID(start, end time.Time) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d-%d", start.Unix(), end.Unix())))
	return hex.EncodeToString(hash[:])[:16]
}

func extractLocationFromStackTrace(stackTrace string) string {
	// Simplified extraction - in reality would parse stack trace
	lines := strings.Split(stackTrace, "\n")
	if len(lines) > 0 {
		parts := strings.Fields(lines[0])
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return "unknown"
}

func determineSignificance(percentage float64) string {
	if percentage > 50 {
		return "very_high"
	} else if percentage > 30 {
		return "high"
	} else if percentage > 15 {
		return "moderate"
	} else if percentage > 5 {
		return "low"
	}
	return "minimal"
}

func getAllCampaignIDs(summaries map[string]*CoverageSummary) []string {
	ids := make([]string, 0, len(summaries))
	for id := range summaries {
		ids = append(ids, id)
	}
	return ids
}
