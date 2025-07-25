package coverage

import (
	"time"
)

// CoverageReport represents a comprehensive coverage analysis report
type CoverageReport struct {
	ID          string                 `json:"id"`
	CampaignID  string                 `json:"campaign_id,omitempty"`
	GeneratedAt time.Time              `json:"generated_at"`
	TimeRange   TimeRange              `json:"time_range"`
	Summary     *CoverageSummary       `json:"summary"`
	Details     *CoverageDetails       `json:"details"`
	Breakdown   *CoverageBreakdown     `json:"breakdown"`
	Trends      *CoverageTrendData     `json:"trends,omitempty"`
	Insights    []CoverageInsight      `json:"insights"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CoverageSummary provides high-level coverage statistics
type CoverageSummary struct {
	TotalCoverage       float64 `json:"total_coverage"`
	LineCoverage        float64 `json:"line_coverage"`
	FunctionCoverage    float64 `json:"function_coverage"`
	BranchCoverage      float64 `json:"branch_coverage"`
	TotalEdges          int64   `json:"total_edges"`
	CoveredEdges        int64   `json:"covered_edges"`
	TotalFunctions      int64   `json:"total_functions"`
	CoveredFunctions    int64   `json:"covered_functions"`
	TotalBranches       int64   `json:"total_branches"`
	CoveredBranches     int64   `json:"covered_branches"`
	NewCoverageFound    int64   `json:"new_coverage_found"`
	CoverageGrowthRate  float64 `json:"coverage_growth_rate"`
	EstimatedCompletion float64 `json:"estimated_completion"`
	QualityScore        float64 `json:"quality_score"`
}

// CoverageDetails provides detailed coverage information
type CoverageDetails struct {
	ByModule      map[string]*ModuleCoverage   `json:"by_module"`
	ByFunction    map[string]*FunctionCoverage `json:"by_function"`
	ByFile        map[string]*FileCoverage     `json:"by_file"`
	HotSpots      []CoverageHotSpot            `json:"hot_spots"`
	ColdSpots     []CoverageColdSpot           `json:"cold_spots"`
	RecentChanges []CoverageChange             `json:"recent_changes"`
}

// ModuleCoverage represents coverage data for a module
type ModuleCoverage struct {
	Name             string                 `json:"name"`
	Path             string                 `json:"path"`
	TotalCoverage    float64                `json:"total_coverage"`
	LineCoverage     float64                `json:"line_coverage"`
	FunctionCoverage float64                `json:"function_coverage"`
	BranchCoverage   float64                `json:"branch_coverage"`
	Complexity       int                    `json:"complexity"`
	Risk             string                 `json:"risk"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// FunctionCoverage represents coverage data for a function
type FunctionCoverage struct {
	Name       string                 `json:"name"`
	Module     string                 `json:"module"`
	File       string                 `json:"file"`
	StartLine  int                    `json:"start_line"`
	EndLine    int                    `json:"end_line"`
	Coverage   float64                `json:"coverage"`
	Complexity int                    `json:"complexity"`
	HitCount   int64                  `json:"hit_count"`
	LastHit    *time.Time             `json:"last_hit,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// FileCoverage represents coverage data for a file
type FileCoverage struct {
	Path             string                 `json:"path"`
	TotalLines       int                    `json:"total_lines"`
	CoveredLines     int                    `json:"covered_lines"`
	TotalFunctions   int                    `json:"total_functions"`
	CoveredFunctions int                    `json:"covered_functions"`
	Coverage         float64                `json:"coverage"`
	LastModified     time.Time              `json:"last_modified"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// CoverageHotSpot represents an area with high coverage activity
type CoverageHotSpot struct {
	Location    string    `json:"location"`
	Type        string    `json:"type"`
	HitCount    int64     `json:"hit_count"`
	Coverage    float64   `json:"coverage"`
	Description string    `json:"description"`
	FirstHit    time.Time `json:"first_hit"`
	LastHit     time.Time `json:"last_hit"`
}

// CoverageColdSpot represents an area with low or no coverage
type CoverageColdSpot struct {
	Location    string   `json:"location"`
	Type        string   `json:"type"`
	Coverage    float64  `json:"coverage"`
	Complexity  int      `json:"complexity"`
	Risk        string   `json:"risk"`
	Description string   `json:"description"`
	Suggestions []string `json:"suggestions"`
}

// CoverageChange represents a change in coverage
type CoverageChange struct {
	Timestamp   time.Time              `json:"timestamp"`
	Type        string                 `json:"type"`
	Location    string                 `json:"location"`
	OldCoverage float64                `json:"old_coverage"`
	NewCoverage float64                `json:"new_coverage"`
	Delta       float64                `json:"delta"`
	Reason      string                 `json:"reason,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CoverageBreakdown provides coverage breakdown by various dimensions
type CoverageBreakdown struct {
	ByComplexity    map[string]float64     `json:"by_complexity"`
	ByRisk          map[string]float64     `json:"by_risk"`
	ByAge           map[string]float64     `json:"by_age"`
	ByType          map[string]float64     `json:"by_type"`
	ByTeam          map[string]float64     `json:"by_team,omitempty"`
	CustomBreakdown map[string]interface{} `json:"custom_breakdown,omitempty"`
}

// CoverageTrendData represents coverage trends over time
type CoverageTrendData struct {
	Period      TrendPeriod          `json:"period"`
	DataPoints  []CoverageTrendPoint `json:"data_points"`
	Growth      *GrowthAnalysis      `json:"growth"`
	Projections *CoverageProjections `json:"projections"`
}

// CoverageTrendPoint represents a single point in coverage trend
type CoverageTrendPoint struct {
	Timestamp        time.Time `json:"timestamp"`
	TotalCoverage    float64   `json:"total_coverage"`
	LineCoverage     float64   `json:"line_coverage"`
	FunctionCoverage float64   `json:"function_coverage"`
	BranchCoverage   float64   `json:"branch_coverage"`
	NewEdges         int64     `json:"new_edges"`
	TotalEdges       int64     `json:"total_edges"`
}

// GrowthAnalysis analyzes coverage growth patterns
type GrowthAnalysis struct {
	AverageGrowthRate  float64                `json:"average_growth_rate"`
	CurrentGrowthRate  float64                `json:"current_growth_rate"`
	GrowthAcceleration float64                `json:"growth_acceleration"`
	TimeToSaturation   *time.Duration         `json:"time_to_saturation,omitempty"`
	SaturationPoint    float64                `json:"saturation_point"`
	GrowthPattern      string                 `json:"growth_pattern"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
}

// CoverageProjections provides future coverage projections
type CoverageProjections struct {
	OneDay      float64                `json:"one_day"`
	OneWeek     float64                `json:"one_week"`
	OneMonth    float64                `json:"one_month"`
	Confidence  float64                `json:"confidence"`
	Methodology string                 `json:"methodology"`
	Assumptions []string               `json:"assumptions"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CoverageInsight represents an insight derived from coverage analysis
type CoverageInsight struct {
	ID          string                 `json:"id"`
	Type        InsightType            `json:"type"`
	Severity    InsightSeverity        `json:"severity"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Impact      string                 `json:"impact"`
	Evidence    map[string]interface{} `json:"evidence"`
	Actions     []string               `json:"actions,omitempty"`
}

// InsightType represents the type of coverage insight
type InsightType string

const (
	InsightTypeAnomaly     InsightType = "anomaly"
	InsightTypeTrend       InsightType = "trend"
	InsightTypeRisk        InsightType = "risk"
	InsightTypeOpportunity InsightType = "opportunity"
	InsightTypeAchievement InsightType = "achievement"
)

// InsightSeverity represents the severity of an insight
type InsightSeverity string

const (
	InsightSeverityCritical InsightSeverity = "critical"
	InsightSeverityHigh     InsightSeverity = "high"
	InsightSeverityMedium   InsightSeverity = "medium"
	InsightSeverityLow      InsightSeverity = "low"
	InsightSeverityInfo     InsightSeverity = "info"
)

// CoverageComparison compares coverage between multiple campaigns
type CoverageComparison struct {
	CampaignIDs     []string                    `json:"campaign_ids"`
	GeneratedAt     time.Time                   `json:"generated_at"`
	TimeRange       TimeRange                   `json:"time_range"`
	Summaries       map[string]*CoverageSummary `json:"summaries"`
	Differences     *CoverageDifferences        `json:"differences"`
	Rankings        *CoverageRankings           `json:"rankings"`
	CommonPatterns  []Pattern                   `json:"common_patterns"`
	Recommendations []ComparisonRecommendation  `json:"recommendations"`
}

// CoverageDifferences highlights differences between campaigns
type CoverageDifferences struct {
	MaxDifference       float64                 `json:"max_difference"`
	MinDifference       float64                 `json:"min_difference"`
	AverageDifference   float64                 `json:"average_difference"`
	SignificantDiffs    []SignificantDifference `json:"significant_differences"`
	ConvergenceAnalysis *ConvergenceAnalysis    `json:"convergence_analysis"`
}

// SignificantDifference represents a significant coverage difference
type SignificantDifference struct {
	Metric       string  `json:"metric"`
	CampaignA    string  `json:"campaign_a"`
	CampaignB    string  `json:"campaign_b"`
	ValueA       float64 `json:"value_a"`
	ValueB       float64 `json:"value_b"`
	Difference   float64 `json:"difference"`
	Percentage   float64 `json:"percentage"`
	Significance string  `json:"significance"`
}

// ConvergenceAnalysis analyzes if campaigns are converging in coverage
type ConvergenceAnalysis struct {
	IsConverging         bool                   `json:"is_converging"`
	ConvergenceRate      float64                `json:"convergence_rate"`
	EstimatedConvergence *time.Time             `json:"estimated_convergence,omitempty"`
	ConvergencePoint     float64                `json:"convergence_point"`
	Metadata             map[string]interface{} `json:"metadata,omitempty"`
}

// CoverageRankings ranks campaigns by various metrics
type CoverageRankings struct {
	ByTotalCoverage []RankingEntry `json:"by_total_coverage"`
	ByGrowthRate    []RankingEntry `json:"by_growth_rate"`
	ByEfficiency    []RankingEntry `json:"by_efficiency"`
	ByQuality       []RankingEntry `json:"by_quality"`
	OverallRanking  []RankingEntry `json:"overall_ranking"`
}

// RankingEntry represents an entry in a ranking
type RankingEntry struct {
	Rank       int     `json:"rank"`
	CampaignID string  `json:"campaign_id"`
	Score      float64 `json:"score"`
	Details    string  `json:"details,omitempty"`
}

// Pattern represents a common pattern found in coverage data
type Pattern struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Frequency   int                    `json:"frequency"`
	Impact      string                 `json:"impact"`
	Examples    []string               `json:"examples"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ComparisonRecommendation provides recommendations based on comparison
type ComparisonRecommendation struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	Priority     int      `json:"priority"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	ForCampaigns []string `json:"for_campaigns"`
	Actions      []string `json:"actions"`
	Expected     string   `json:"expected_outcome"`
}

// CoverageAnalysisConfig configures coverage analysis
type CoverageAnalysisConfig struct {
	IncludeTrends      bool                   `json:"include_trends"`
	IncludeProjections bool                   `json:"include_projections"`
	IncludeHotspots    bool                   `json:"include_hotspots"`
	DetailLevel        DetailLevel            `json:"detail_level"`
	CustomMetrics      []string               `json:"custom_metrics,omitempty"`
	Filters            map[string]interface{} `json:"filters,omitempty"`
}
