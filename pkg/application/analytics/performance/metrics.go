package performance

import (
	"time"
)

// Metric represents a single performance metric
type Metric struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Type      MetricType             `json:"type"`
	Value     float64                `json:"value"`
	Unit      string                 `json:"unit"`
	Timestamp time.Time              `json:"timestamp"`
	Tags      map[string]string      `json:"tags,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// MetricType represents the type of metric
type MetricType string

const (
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeCounter   MetricType = "counter"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeRate      MetricType = "rate"
)

// CampaignMetrics represents metrics for a campaign
type CampaignMetrics struct {
	CampaignID        string                 `json:"campaign_id"`
	CollectedAt       time.Time              `json:"collected_at"`
	Duration          time.Duration          `json:"duration"`
	ExecutionMetrics  *ExecutionMetrics      `json:"execution_metrics"`
	ResourceMetrics   *ResourceMetrics       `json:"resource_metrics"`
	QueueMetrics      *QueueMetrics          `json:"queue_metrics"`
	EfficiencyMetrics *EfficiencyMetrics     `json:"efficiency_metrics"`
	BotMetrics        map[string]*BotMetrics `json:"bot_metrics,omitempty"`
}

// ExecutionMetrics represents execution-related metrics
type ExecutionMetrics struct {
	TotalExecutions     int64   `json:"total_executions"`
	ExecutionsPerSecond float64 `json:"executions_per_second"`
	AverageExecTime     float64 `json:"average_exec_time_ms"`
	MedianExecTime      float64 `json:"median_exec_time_ms"`
	P95ExecTime         float64 `json:"p95_exec_time_ms"`
	P99ExecTime         float64 `json:"p99_exec_time_ms"`
	FailedExecutions    int64   `json:"failed_executions"`
	FailureRate         float64 `json:"failure_rate"`
	Throughput          float64 `json:"throughput_mb_per_sec"`
}

// ResourceMetrics represents resource utilization metrics
type ResourceMetrics struct {
	CPUUsage      *CPUMetrics     `json:"cpu_usage"`
	MemoryUsage   *MemoryMetrics  `json:"memory_usage"`
	DiskUsage     *DiskMetrics    `json:"disk_usage"`
	NetworkUsage  *NetworkMetrics `json:"network_usage"`
	ResourceScore float64         `json:"resource_efficiency_score"`
}

// CPUMetrics represents CPU usage metrics
type CPUMetrics struct {
	AverageUsage float64 `json:"average_usage_percent"`
	PeakUsage    float64 `json:"peak_usage_percent"`
	CoreCount    int     `json:"core_count"`
	Efficiency   float64 `json:"efficiency_score"`
}

// MemoryMetrics represents memory usage metrics
type MemoryMetrics struct {
	AverageUsage float64 `json:"average_usage_mb"`
	PeakUsage    float64 `json:"peak_usage_mb"`
	Available    float64 `json:"available_mb"`
	SwapUsage    float64 `json:"swap_usage_mb"`
	GCPressure   float64 `json:"gc_pressure"`
}

// DiskMetrics represents disk usage metrics
type DiskMetrics struct {
	ReadThroughput  float64 `json:"read_throughput_mb_per_sec"`
	WriteThroughput float64 `json:"write_throughput_mb_per_sec"`
	IOPS            float64 `json:"iops"`
	QueueDepth      float64 `json:"average_queue_depth"`
	Latency         float64 `json:"average_latency_ms"`
}

// NetworkMetrics represents network usage metrics
type NetworkMetrics struct {
	InboundBandwidth  float64 `json:"inbound_bandwidth_mbps"`
	OutboundBandwidth float64 `json:"outbound_bandwidth_mbps"`
	PacketLoss        float64 `json:"packet_loss_percent"`
	Latency           float64 `json:"average_latency_ms"`
}

// QueueMetrics represents queue processing metrics
type QueueMetrics struct {
	QueueDepth         int64   `json:"queue_depth"`
	AverageWaitTime    float64 `json:"average_wait_time_ms"`
	MaxWaitTime        float64 `json:"max_wait_time_ms"`
	ProcessingRate     float64 `json:"processing_rate_per_sec"`
	BackpressureEvents int64   `json:"backpressure_events"`
	DroppedItems       int64   `json:"dropped_items"`
	QueueUtilization   float64 `json:"queue_utilization_percent"`
}

// EfficiencyMetrics represents efficiency-related metrics
type EfficiencyMetrics struct {
	CoveragePerExecution float64        `json:"coverage_per_execution"`
	CrashesPerHour       float64        `json:"crashes_per_hour"`
	UniquePathsPerHour   float64        `json:"unique_paths_per_hour"`
	EfficiencyScore      float64        `json:"overall_efficiency_score"`
	ResourceEfficiency   float64        `json:"resource_efficiency"`
	TimeToFirstCrash     *time.Duration `json:"time_to_first_crash,omitempty"`
	TimeToTargetCoverage *time.Duration `json:"time_to_target_coverage,omitempty"`
}

// BotMetrics represents metrics for a specific bot
type BotMetrics struct {
	BotID            string            `json:"bot_id"`
	Status           string            `json:"status"`
	Uptime           time.Duration     `json:"uptime"`
	ExecutionMetrics *ExecutionMetrics `json:"execution_metrics"`
	ResourceMetrics  *ResourceMetrics  `json:"resource_metrics"`
	EfficiencyScore  float64           `json:"efficiency_score"`
	HealthScore      float64           `json:"health_score"`
	LastHealthCheck  time.Time         `json:"last_health_check"`
	Errors           []BotError        `json:"errors,omitempty"`
}

// BotError represents an error encountered by a bot
type BotError struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	Count     int       `json:"count"`
	LastSeen  time.Time `json:"last_seen"`
	Impact    string    `json:"impact"`
}

// AggregatedMetrics represents aggregated metrics
type AggregatedMetrics struct {
	Period          time.Duration          `json:"period"`
	StartTime       time.Time              `json:"start_time"`
	EndTime         time.Time              `json:"end_time"`
	AggregationType AggregationType        `json:"aggregation_type"`
	Metrics         map[string]float64     `json:"metrics"`
	Statistics      *MetricStatistics      `json:"statistics"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// MetricStatistics represents statistical analysis of metrics
type MetricStatistics struct {
	Count    int     `json:"count"`
	Sum      float64 `json:"sum"`
	Average  float64 `json:"average"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	StdDev   float64 `json:"std_dev"`
	Variance float64 `json:"variance"`
	Median   float64 `json:"median"`
	P25      float64 `json:"p25"`
	P75      float64 `json:"p75"`
	P90      float64 `json:"p90"`
	P95      float64 `json:"p95"`
	P99      float64 `json:"p99"`
}

// PerformanceReport represents a comprehensive performance report
type PerformanceReport struct {
	ID              string                       `json:"id"`
	CampaignID      string                       `json:"campaign_id"`
	GeneratedAt     time.Time                    `json:"generated_at"`
	TimeRange       TimeRange                    `json:"time_range"`
	Summary         *PerformanceSummary          `json:"summary"`
	Metrics         *CampaignMetrics             `json:"metrics"`
	Analysis        *PerformanceAnalysis         `json:"analysis"`
	Bottlenecks     *BottleneckAnalysis          `json:"bottlenecks,omitempty"`
	Recommendations *OptimizationRecommendations `json:"recommendations,omitempty"`
	Trends          *PerformanceTrends           `json:"trends,omitempty"`
}

// PerformanceSummary provides high-level performance overview
type PerformanceSummary struct {
	OverallScore    float64  `json:"overall_score"`
	ExecutionScore  float64  `json:"execution_score"`
	ResourceScore   float64  `json:"resource_score"`
	EfficiencyScore float64  `json:"efficiency_score"`
	HealthStatus    string   `json:"health_status"`
	KeyHighlights   []string `json:"key_highlights"`
	CriticalIssues  []string `json:"critical_issues"`
}

// PerformanceAnalysis contains detailed performance analysis
type PerformanceAnalysis struct {
	ExecutionAnalysis  *ExecutionAnalysis   `json:"execution_analysis"`
	ResourceAnalysis   *ResourceAnalysis    `json:"resource_analysis"`
	EfficiencyAnalysis *EfficiencyAnalysis  `json:"efficiency_analysis"`
	Insights           []PerformanceInsight `json:"insights"`
}

// ExecutionAnalysis analyzes execution performance
type ExecutionAnalysis struct {
	ThroughputTrend   string  `json:"throughput_trend"`
	LatencyTrend      string  `json:"latency_trend"`
	StabilityScore    float64 `json:"stability_score"`
	PerformanceGrade  string  `json:"performance_grade"`
	AnomaliesDetected int     `json:"anomalies_detected"`
}

// ResourceAnalysis analyzes resource utilization
type ResourceAnalysis struct {
	UtilizationLevel string  `json:"utilization_level"`
	ResourceBalance  string  `json:"resource_balance"`
	ScalingPotential float64 `json:"scaling_potential"`
	WastedResources  float64 `json:"wasted_resources_percent"`
	OptimizationRoom float64 `json:"optimization_room_percent"`
}

// EfficiencyAnalysis analyzes overall efficiency
type EfficiencyAnalysis struct {
	ProductivityLevel string  `json:"productivity_level"`
	CostEfficiency    float64 `json:"cost_efficiency"`
	OutputQuality     float64 `json:"output_quality"`
	TimeEfficiency    float64 `json:"time_efficiency"`
}

// PerformanceInsight represents an insight from performance analysis
type PerformanceInsight struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Severity    string                 `json:"severity"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Impact      string                 `json:"impact"`
	Evidence    map[string]interface{} `json:"evidence"`
	Actions     []string               `json:"actions,omitempty"`
}

// BotPerformanceReport represents performance analysis for a specific bot
type BotPerformanceReport struct {
	BotID           string             `json:"bot_id"`
	GeneratedAt     time.Time          `json:"generated_at"`
	TimeRange       TimeRange          `json:"time_range"`
	Metrics         *BotMetrics        `json:"metrics"`
	Performance     *BotPerformance    `json:"performance"`
	Health          *BotHealthAnalysis `json:"health"`
	Recommendations []string           `json:"recommendations"`
}

// BotPerformance represents bot performance analysis
type BotPerformance struct {
	ProductivityScore   float64 `json:"productivity_score"`
	ReliabilityScore    float64 `json:"reliability_score"`
	EfficiencyScore     float64 `json:"efficiency_score"`
	ComparisonToAverage float64 `json:"comparison_to_average_percent"`
	Ranking             int     `json:"ranking"`
	PerformanceTrend    string  `json:"performance_trend"`
}

// BotHealthAnalysis represents bot health analysis
type BotHealthAnalysis struct {
	HealthScore  float64       `json:"health_score"`
	Status       string        `json:"status"`
	Issues       []HealthIssue `json:"issues,omitempty"`
	LastIncident *time.Time    `json:"last_incident,omitempty"`
	MTBF         time.Duration `json:"mtbf"` // Mean Time Between Failures
	MTTR         time.Duration `json:"mttr"` // Mean Time To Recovery
}

// HealthIssue represents a health issue
type HealthIssue struct {
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Description string    `json:"description"`
	FirstSeen   time.Time `json:"first_seen"`
	Frequency   int       `json:"frequency"`
	Impact      string    `json:"impact"`
}

// BottleneckAnalysis identifies performance bottlenecks
type BottleneckAnalysis struct {
	IdentifiedAt   time.Time       `json:"identified_at"`
	Bottlenecks    []Bottleneck    `json:"bottlenecks"`
	PrimaryLimiter string          `json:"primary_limiter"`
	ImpactAnalysis *ImpactAnalysis `json:"impact_analysis"`
}

// Bottleneck represents a performance bottleneck
type Bottleneck struct {
	ID          string                 `json:"id"`
	Type        BottleneckType         `json:"type"`
	Component   string                 `json:"component"`
	Severity    string                 `json:"severity"`
	Description string                 `json:"description"`
	Impact      float64                `json:"impact_percent"`
	Evidence    map[string]interface{} `json:"evidence"`
	Solutions   []Solution             `json:"solutions"`
}

// BottleneckType represents the type of bottleneck
type BottleneckType string

const (
	BottleneckTypeCPU       BottleneckType = "cpu"
	BottleneckTypeMemory    BottleneckType = "memory"
	BottleneckTypeDisk      BottleneckType = "disk"
	BottleneckTypeNetwork   BottleneckType = "network"
	BottleneckTypeQueue     BottleneckType = "queue"
	BottleneckTypeAlgorithm BottleneckType = "algorithm"
)

// Solution represents a solution to a bottleneck
type Solution struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Effort      string `json:"effort"`
	Impact      string `json:"impact"`
	Priority    int    `json:"priority"`
}

// ImpactAnalysis analyzes the impact of bottlenecks
type ImpactAnalysis struct {
	ThroughputLoss      float64       `json:"throughput_loss_percent"`
	EfficiencyLoss      float64       `json:"efficiency_loss_percent"`
	CostIncrease        float64       `json:"cost_increase_percent"`
	QualityImpact       string        `json:"quality_impact"`
	EstimatedResolution time.Duration `json:"estimated_resolution_time"`
}

// OptimizationRecommendations provides optimization recommendations
type OptimizationRecommendations struct {
	GeneratedAt     time.Time        `json:"generated_at"`
	Recommendations []Recommendation `json:"recommendations"`
	PotentialGains  *PotentialGains  `json:"potential_gains"`
	Priority        string           `json:"priority"`
}

// Recommendation represents an optimization recommendation
type Recommendation struct {
	ID          string                 `json:"id"`
	Category    string                 `json:"category"`
	Priority    int                    `json:"priority"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Rationale   string                 `json:"rationale"`
	Actions     []Action               `json:"actions"`
	Expected    ExpectedImprovement    `json:"expected_improvement"`
	Effort      string                 `json:"effort"`
	Risk        string                 `json:"risk"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Action represents a specific action to take
type Action struct {
	Step        int                    `json:"step"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"`
	Target      string                 `json:"target"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// ExpectedImprovement represents expected improvement from a recommendation
type ExpectedImprovement struct {
	ThroughputGain float64 `json:"throughput_gain_percent"`
	EfficiencyGain float64 `json:"efficiency_gain_percent"`
	ResourceSaving float64 `json:"resource_saving_percent"`
	Timeline       string  `json:"timeline"`
	Confidence     float64 `json:"confidence"`
}

// PotentialGains represents potential gains from all recommendations
type PotentialGains struct {
	TotalThroughputGain float64       `json:"total_throughput_gain_percent"`
	TotalEfficiencyGain float64       `json:"total_efficiency_gain_percent"`
	TotalResourceSaving float64       `json:"total_resource_saving_percent"`
	EstimatedTimeSaving time.Duration `json:"estimated_time_saving"`
	ROI                 float64       `json:"roi_percent"`
}

// PerformanceTrends represents performance trends over time
type PerformanceTrends struct {
	Period     TrendPeriod             `json:"period"`
	DataPoints []PerformanceTrendPoint `json:"data_points"`
	Analysis   *TrendAnalysis          `json:"analysis"`
	Forecast   *PerformanceForecast    `json:"forecast,omitempty"`
}

// PerformanceTrendPoint represents a point in performance trend
type PerformanceTrendPoint struct {
	Timestamp        time.Time `json:"timestamp"`
	ExecutionsPerSec float64   `json:"executions_per_sec"`
	AverageLatency   float64   `json:"average_latency_ms"`
	CPUUsage         float64   `json:"cpu_usage_percent"`
	MemoryUsage      float64   `json:"memory_usage_mb"`
	EfficiencyScore  float64   `json:"efficiency_score"`
}

// TrendAnalysis analyzes performance trends
type TrendAnalysis struct {
	TrendDirection    string      `json:"trend_direction"`
	TrendStrength     float64     `json:"trend_strength"`
	Volatility        float64     `json:"volatility"`
	SeasonalPattern   bool        `json:"seasonal_pattern"`
	AnomaliesDetected int         `json:"anomalies_detected"`
	ChangePoints      []time.Time `json:"change_points,omitempty"`
}

// PerformanceForecast forecasts future performance
type PerformanceForecast struct {
	OneHour     *ForecastPoint `json:"one_hour"`
	OneDay      *ForecastPoint `json:"one_day"`
	OneWeek     *ForecastPoint `json:"one_week"`
	Methodology string         `json:"methodology"`
	Confidence  float64        `json:"confidence"`
	Assumptions []string       `json:"assumptions"`
}

// ForecastPoint represents a forecasted performance point
type ForecastPoint struct {
	Timestamp        time.Time `json:"timestamp"`
	ExecutionsPerSec float64   `json:"executions_per_sec"`
	EfficiencyScore  float64   `json:"efficiency_score"`
	ResourceUsage    float64   `json:"resource_usage_percent"`
	ConfidenceLower  float64   `json:"confidence_lower"`
	ConfidenceUpper  float64   `json:"confidence_upper"`
}
