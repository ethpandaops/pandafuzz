package trends

import (
	"time"
)

// CoverageTrends represents coverage trends analysis
type CoverageTrends struct {
	CampaignID string                 `json:"campaign_id"`
	Period     TrendPeriod            `json:"period"`
	TimeRange  TimeRange              `json:"time_range"`
	DataPoints []CoverageTrendPoint   `json:"data_points"`
	Analysis   *CoverageTrendAnalysis `json:"analysis"`
	Anomalies  []Anomaly              `json:"anomalies,omitempty"`
	Forecast   *CoverageForecast      `json:"forecast,omitempty"`
	Insights   []TrendInsight         `json:"insights"`
}

// CoverageTrendPoint represents a point in coverage trend
type CoverageTrendPoint struct {
	Timestamp          time.Time `json:"timestamp"`
	TotalCoverage      float64   `json:"total_coverage"`
	LineCoverage       float64   `json:"line_coverage"`
	FunctionCoverage   float64   `json:"function_coverage"`
	BranchCoverage     float64   `json:"branch_coverage"`
	NewEdgesDiscovered int64     `json:"new_edges_discovered"`
	CumulativeEdges    int64     `json:"cumulative_edges"`
	GrowthRate         float64   `json:"growth_rate"`
	DiscoveryVelocity  float64   `json:"discovery_velocity"`
}

// CoverageTrendAnalysis analyzes coverage trends
type CoverageTrendAnalysis struct {
	TrendType           TrendType         `json:"trend_type"`
	Direction           TrendDirection    `json:"direction"`
	Strength            float64           `json:"strength"`
	Consistency         float64           `json:"consistency"`
	Volatility          float64           `json:"volatility"`
	GrowthPattern       GrowthPattern     `json:"growth_pattern"`
	CurrentPhase        string            `json:"current_phase"`
	PhaseTransitions    []PhaseTransition `json:"phase_transitions,omitempty"`
	SeasonalityDetected bool              `json:"seasonality_detected"`
	CycleLength         *time.Duration    `json:"cycle_length,omitempty"`
}

// TrendType represents the type of trend
type TrendType string

const (
	TrendTypeLinear      TrendType = "linear"
	TrendTypeExponential TrendType = "exponential"
	TrendTypeLogarithmic TrendType = "logarithmic"
	TrendTypePlateau     TrendType = "plateau"
	TrendTypeCyclical    TrendType = "cyclical"
	TrendTypeIrregular   TrendType = "irregular"
)

// TrendDirection represents the direction of a trend
type TrendDirection string

const (
	TrendDirectionUp     TrendDirection = "up"
	TrendDirectionDown   TrendDirection = "down"
	TrendDirectionStable TrendDirection = "stable"
	TrendDirectionMixed  TrendDirection = "mixed"
)

// GrowthPattern represents the pattern of growth
type GrowthPattern string

const (
	GrowthPatternSteady       GrowthPattern = "steady"
	GrowthPatternAccelerating GrowthPattern = "accelerating"
	GrowthPatternDecelerating GrowthPattern = "decelerating"
	GrowthPatternVolatile     GrowthPattern = "volatile"
	GrowthPatternStagnant     GrowthPattern = "stagnant"
)

// PhaseTransition represents a transition between growth phases
type PhaseTransition struct {
	Timestamp  time.Time `json:"timestamp"`
	FromPhase  string    `json:"from_phase"`
	ToPhase    string    `json:"to_phase"`
	Trigger    string    `json:"trigger,omitempty"`
	Confidence float64   `json:"confidence"`
}

// CoverageForecast represents coverage trend forecast
type CoverageForecast struct {
	GeneratedAt     time.Time           `json:"generated_at"`
	ForecastPeriod  time.Duration       `json:"forecast_period"`
	PredictedPoints []ForecastPoint     `json:"predicted_points"`
	ConfidenceLevel float64             `json:"confidence_level"`
	Methodology     string              `json:"methodology"`
	Assumptions     []string            `json:"assumptions"`
	RiskFactors     []RiskFactor        `json:"risk_factors,omitempty"`
	SaturationPoint *SaturationAnalysis `json:"saturation_point,omitempty"`
}

// ForecastPoint represents a forecasted trend point
type ForecastPoint struct {
	Timestamp          time.Time `json:"timestamp"`
	PredictedValue     float64   `json:"predicted_value"`
	ConfidenceInterval Interval  `json:"confidence_interval"`
	Probability        float64   `json:"probability"`
}

// Interval represents a confidence interval
type Interval struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
}

// SaturationAnalysis analyzes when growth will saturate
type SaturationAnalysis struct {
	EstimatedSaturation float64        `json:"estimated_saturation"`
	TimeToSaturation    *time.Duration `json:"time_to_saturation,omitempty"`
	CurrentUtilization  float64        `json:"current_utilization"`
	RemainingPotential  float64        `json:"remaining_potential"`
	ConfidenceLevel     float64        `json:"confidence_level"`
}

// RiskFactor represents a risk to the forecast
type RiskFactor struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Impact      string  `json:"impact"`
	Likelihood  float64 `json:"likelihood"`
	Mitigation  string  `json:"mitigation,omitempty"`
}

// PerformanceTrends represents performance trends analysis
type PerformanceTrends struct {
	CampaignID string                    `json:"campaign_id"`
	Period     TrendPeriod               `json:"period"`
	TimeRange  TimeRange                 `json:"time_range"`
	DataPoints []PerformanceTrendPoint   `json:"data_points"`
	Analysis   *PerformanceTrendAnalysis `json:"analysis"`
	Anomalies  []Anomaly                 `json:"anomalies,omitempty"`
	Forecast   *PerformanceForecast      `json:"forecast,omitempty"`
	Insights   []TrendInsight            `json:"insights"`
}

// PerformanceTrendPoint represents a point in performance trend
type PerformanceTrendPoint struct {
	Timestamp           time.Time `json:"timestamp"`
	ExecutionsPerSecond float64   `json:"executions_per_second"`
	AverageLatency      float64   `json:"average_latency_ms"`
	P95Latency          float64   `json:"p95_latency_ms"`
	P99Latency          float64   `json:"p99_latency_ms"`
	CPUUtilization      float64   `json:"cpu_utilization_percent"`
	MemoryUtilization   float64   `json:"memory_utilization_percent"`
	QueueDepth          int64     `json:"queue_depth"`
	ErrorRate           float64   `json:"error_rate"`
	EfficiencyScore     float64   `json:"efficiency_score"`
}

// PerformanceTrendAnalysis analyzes performance trends
type PerformanceTrendAnalysis struct {
	OverallTrend        TrendDirection    `json:"overall_trend"`
	StabilityScore      float64           `json:"stability_score"`
	PerformanceGrade    string            `json:"performance_grade"`
	DegradationDetected bool              `json:"degradation_detected"`
	BottleneckTrends    []BottleneckTrend `json:"bottleneck_trends,omitempty"`
	OptimizationWindows []TimeWindow      `json:"optimization_windows,omitempty"`
}

// BottleneckTrend represents a trend in bottlenecks
type BottleneckTrend struct {
	Type      string         `json:"type"`
	Severity  string         `json:"severity"`
	Trend     TrendDirection `json:"trend"`
	FirstSeen time.Time      `json:"first_seen"`
	Frequency float64        `json:"frequency"`
	Impact    string         `json:"impact"`
}

// TimeWindow represents a time window
type TimeWindow struct {
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	Description string    `json:"description"`
}

// PerformanceForecast represents performance trend forecast
type PerformanceForecast struct {
	GeneratedAt          time.Time                  `json:"generated_at"`
	ForecastPeriod       time.Duration              `json:"forecast_period"`
	PredictedPerformance []PerformanceForecastPoint `json:"predicted_performance"`
	ExpectedBottlenecks  []ExpectedBottleneck       `json:"expected_bottlenecks,omitempty"`
	RecommendedActions   []RecommendedAction        `json:"recommended_actions"`
	ConfidenceLevel      float64                    `json:"confidence_level"`
}

// PerformanceForecastPoint represents a forecasted performance point
type PerformanceForecastPoint struct {
	Timestamp           time.Time `json:"timestamp"`
	ExpectedThroughput  Interval  `json:"expected_throughput"`
	ExpectedLatency     Interval  `json:"expected_latency"`
	ExpectedUtilization Interval  `json:"expected_utilization"`
	RiskLevel           string    `json:"risk_level"`
}

// ExpectedBottleneck represents an expected future bottleneck
type ExpectedBottleneck struct {
	Type              string    `json:"type"`
	ExpectedTimestamp time.Time `json:"expected_timestamp"`
	Severity          string    `json:"severity"`
	Impact            string    `json:"impact"`
	PreventiveActions []string  `json:"preventive_actions"`
}

// RecommendedAction represents a recommended action based on trends
type RecommendedAction struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Priority    int     `json:"priority"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Timing      string  `json:"timing"`
	Impact      string  `json:"impact"`
	Trigger     Trigger `json:"trigger"`
}

// Trigger represents when an action should be triggered
type Trigger struct {
	Type      string                 `json:"type"`
	Condition string                 `json:"condition"`
	Threshold float64                `json:"threshold,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// CrashTrends represents crash discovery trends
type CrashTrends struct {
	CampaignID string              `json:"campaign_id"`
	Period     TrendPeriod         `json:"period"`
	TimeRange  TimeRange           `json:"time_range"`
	DataPoints []CrashTrendPoint   `json:"data_points"`
	Analysis   *CrashTrendAnalysis `json:"analysis"`
	Patterns   []CrashPattern      `json:"patterns,omitempty"`
	Forecast   *CrashForecast      `json:"forecast,omitempty"`
	Insights   []TrendInsight      `json:"insights"`
}

// CrashTrendPoint represents a point in crash trend
type CrashTrendPoint struct {
	Timestamp         time.Time      `json:"timestamp"`
	NewCrashes        int            `json:"new_crashes"`
	UniqueCrashes     int            `json:"unique_crashes"`
	TotalCrashes      int            `json:"total_crashes"`
	CrashRate         float64        `json:"crash_rate"`
	SeverityBreakdown map[string]int `json:"severity_breakdown"`
	TypeBreakdown     map[string]int `json:"type_breakdown"`
	DiscoveryVelocity float64        `json:"discovery_velocity"`
}

// CrashTrendAnalysis analyzes crash discovery trends
type CrashTrendAnalysis struct {
	DiscoveryTrend      TrendDirection            `json:"discovery_trend"`
	DiscoveryRate       float64                   `json:"discovery_rate"`
	UniquenessRatio     float64                   `json:"uniqueness_ratio"`
	SeverityTrends      map[string]TrendDirection `json:"severity_trends"`
	MostCommonTypes     []string                  `json:"most_common_types"`
	DiscoveryEfficiency float64                   `json:"discovery_efficiency"`
	PeakDiscoveryTimes  []TimeWindow              `json:"peak_discovery_times,omitempty"`
}

// CrashPattern represents a pattern in crash discovery
type CrashPattern struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Frequency   float64                `json:"frequency"`
	Confidence  float64                `json:"confidence"`
	Examples    []string               `json:"examples,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CrashForecast represents crash discovery forecast
type CrashForecast struct {
	GeneratedAt        time.Time            `json:"generated_at"`
	ForecastPeriod     time.Duration        `json:"forecast_period"`
	ExpectedDiscovery  []CrashForecastPoint `json:"expected_discovery"`
	EstimatedRemaining int                  `json:"estimated_remaining_crashes"`
	ConfidenceLevel    float64              `json:"confidence_level"`
}

// CrashForecastPoint represents a forecasted crash discovery point
type CrashForecastPoint struct {
	Timestamp          time.Time          `json:"timestamp"`
	ExpectedNewCrashes Interval           `json:"expected_new_crashes"`
	ExpectedCrashRate  float64            `json:"expected_crash_rate"`
	SeverityPrediction map[string]float64 `json:"severity_prediction"`
}

// Anomaly represents an anomaly detected in trends
type Anomaly struct {
	ID          string                 `json:"id"`
	Type        AnomalyType            `json:"type"`
	Severity    string                 `json:"severity"`
	DetectedAt  time.Time              `json:"detected_at"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     *time.Time             `json:"end_time,omitempty"`
	Description string                 `json:"description"`
	Impact      string                 `json:"impact"`
	Evidence    map[string]interface{} `json:"evidence"`
	Status      AnomalyStatus          `json:"status"`
	Actions     []string               `json:"actions,omitempty"`
}

// AnomalyType represents the type of anomaly
type AnomalyType string

const (
	AnomalyTypeSpike        AnomalyType = "spike"
	AnomalyTypeDrop         AnomalyType = "drop"
	AnomalyTypePatternBreak AnomalyType = "pattern_break"
	AnomalyTypeOutlier      AnomalyType = "outlier"
	AnomalyTypeTrendShift   AnomalyType = "trend_shift"
	AnomalyTypeSeasonality  AnomalyType = "seasonality_break"
)

// AnomalyStatus represents the status of an anomaly
type AnomalyStatus string

const (
	AnomalyStatusActive    AnomalyStatus = "active"
	AnomalyStatusResolved  AnomalyStatus = "resolved"
	AnomalyStatusMonitored AnomalyStatus = "monitored"
	AnomalyStatusIgnored   AnomalyStatus = "ignored"
)

// TrendData represents generic trend data for analysis
type TrendData struct {
	MetricName string                 `json:"metric_name"`
	DataPoints []DataPoint            `json:"data_points"`
	TimeRange  TimeRange              `json:"time_range"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// DataPoint represents a generic data point
type DataPoint struct {
	Timestamp time.Time              `json:"timestamp"`
	Value     float64                `json:"value"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// AnomalyReport represents a comprehensive anomaly report
type AnomalyReport struct {
	GeneratedAt      time.Time         `json:"generated_at"`
	TimeRange        TimeRange         `json:"time_range"`
	AnomaliesFound   []Anomaly         `json:"anomalies_found"`
	Summary          *AnomalySummary   `json:"summary"`
	ImpactAssessment *ImpactAssessment `json:"impact_assessment"`
	Recommendations  []Recommendation  `json:"recommendations"`
}

// AnomalySummary summarizes anomalies
type AnomalySummary struct {
	TotalAnomalies    int            `json:"total_anomalies"`
	ActiveAnomalies   int            `json:"active_anomalies"`
	SeverityBreakdown map[string]int `json:"severity_breakdown"`
	TypeBreakdown     map[string]int `json:"type_breakdown"`
	MostAffectedAreas []string       `json:"most_affected_areas"`
	OverallRisk       string         `json:"overall_risk"`
}

// ImpactAssessment assesses the impact of anomalies
type ImpactAssessment struct {
	PerformanceImpact  float64       `json:"performance_impact_percent"`
	EfficiencyImpact   float64       `json:"efficiency_impact_percent"`
	QualityImpact      string        `json:"quality_impact"`
	EstimatedDowntime  time.Duration `json:"estimated_downtime,omitempty"`
	AffectedComponents []string      `json:"affected_components"`
	BusinessImpact     string        `json:"business_impact"`
}

// Recommendation represents a trend-based recommendation
type Recommendation struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Priority    int                    `json:"priority"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Rationale   string                 `json:"rationale"`
	Actions     []string               `json:"actions"`
	Timeline    string                 `json:"timeline"`
	Expected    ExpectedOutcome        `json:"expected_outcome"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ExpectedOutcome represents expected outcome from a recommendation
type ExpectedOutcome struct {
	Description  string  `json:"description"`
	Improvement  float64 `json:"improvement_percent"`
	TimeToImpact string  `json:"time_to_impact"`
	Confidence   float64 `json:"confidence"`
}

// TrendInsight represents an insight derived from trend analysis
type TrendInsight struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Severity    string                 `json:"severity"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Impact      string                 `json:"impact"`
	Evidence    map[string]interface{} `json:"evidence"`
	Actions     []string               `json:"actions,omitempty"`
	ValidUntil  *time.Time             `json:"valid_until,omitempty"`
}

// TrendForecast represents a generic trend forecast
type TrendForecast struct {
	MetricName      string            `json:"metric_name"`
	GeneratedAt     time.Time         `json:"generated_at"`
	ForecastPeriod  time.Duration     `json:"forecast_period"`
	PredictedValues []ForecastPoint   `json:"predicted_values"`
	Confidence      float64           `json:"confidence"`
	Methodology     string            `json:"methodology"`
	Assumptions     []string          `json:"assumptions"`
	Accuracy        *ForecastAccuracy `json:"accuracy,omitempty"`
}

// ForecastAccuracy represents forecast accuracy metrics
type ForecastAccuracy struct {
	MeanAbsoluteError      float64 `json:"mean_absolute_error"`
	MeanSquaredError       float64 `json:"mean_squared_error"`
	MeanAbsolutePercentage float64 `json:"mean_absolute_percentage"`
	R2Score                float64 `json:"r2_score"`
}

// TrendsReport represents a comprehensive trends report
type TrendsReport struct {
	GeneratedAt       time.Time             `json:"generated_at"`
	CampaignID        string                `json:"campaign_id"`
	TimeRange         TimeRange             `json:"time_range"`
	CoverageTrends    *CoverageTrends       `json:"coverage_trends,omitempty"`
	PerformanceTrends *PerformanceTrends    `json:"performance_trends,omitempty"`
	CrashTrends       *CrashTrends          `json:"crash_trends,omitempty"`
	OverallAnalysis   *OverallTrendAnalysis `json:"overall_analysis"`
	KeyFindings       []KeyFinding          `json:"key_findings"`
	Recommendations   []Recommendation      `json:"recommendations"`
}

// OverallTrendAnalysis provides overall trend analysis
type OverallTrendAnalysis struct {
	HealthTrend         TrendDirection `json:"health_trend"`
	MomentumScore       float64        `json:"momentum_score"`
	StabilityScore      float64        `json:"stability_score"`
	PredictabilityScore float64        `json:"predictability_score"`
	RiskLevel           string         `json:"risk_level"`
	Outlook             string         `json:"outlook"`
}

// KeyFinding represents a key finding from trend analysis
type KeyFinding struct {
	ID         string   `json:"id"`
	Category   string   `json:"category"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Importance string   `json:"importance"`
	Timeframe  string   `json:"timeframe"`
	Supporting []string `json:"supporting_data"`
}
