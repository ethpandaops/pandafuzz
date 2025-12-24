package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// AnalyticsService provides analytics and metrics for fuzzing campaigns
type AnalyticsService interface {
	// Lifecycle methods
	Start(ctx context.Context) error
	Stop() error

	// Coverage analytics
	GetCoverageTrend(ctx context.Context, campaignID string, interval time.Duration) (*CoverageTrend, error)
	GetCoverageComparison(ctx context.Context, campaignIDs []string) (*CoverageComparison, error)

	// Crash analytics
	GetCrashRate(ctx context.Context, campaignID string, window time.Duration) (*CrashRateMetrics, error)
	GetCrashDistribution(ctx context.Context, campaignID string) (*CrashDistribution, error)
	GetTopCrashGroups(ctx context.Context, campaignID string, limit int) ([]*CrashGroupStats, error)

	// Performance analytics
	GetFuzzerPerformance(ctx context.Context, fuzzerType string, window time.Duration) (*FuzzerPerformance, error)
	GetBotUtilization(ctx context.Context, window time.Duration) (*BotUtilization, error)
	GetJobThroughput(ctx context.Context, window time.Duration) (*JobThroughput, error)

	// Campaign analytics
	GetCampaignSummary(ctx context.Context, campaignID string) (*CampaignSummary, error)
	GetCampaignProgress(ctx context.Context, campaignID string) (*CampaignProgress, error)
	CompareCampaigns(ctx context.Context, campaignIDs []string) (*CampaignComparison, error)

	// Real-time metrics
	GetRealtimeMetrics(ctx context.Context, campaignID string) (*RealtimeMetrics, error)
	SubscribeToMetrics(ctx context.Context, campaignID string) (<-chan *RealtimeMetrics, error)
	UnsubscribeFromMetrics(ctx context.Context, subscriptionID string) error
}

// Analytics response types

// CoverageTrend represents coverage growth over time
type CoverageTrend struct {
	CampaignID  string              `json:"campaign_id"`
	Interval    time.Duration       `json:"interval"`
	StartTime   time.Time           `json:"start_time"`
	EndTime     time.Time           `json:"end_time"`
	DataPoints  []CoveragePoint     `json:"data_points"`
	TotalGrowth int64               `json:"total_growth"`
	GrowthRate  float64             `json:"growth_rate"` // Edges per hour
	Projection  *CoverageProjection `json:"projection,omitempty"`
}

// CoveragePoint represents a single coverage measurement
type CoveragePoint struct {
	Timestamp   time.Time `json:"timestamp"`
	TotalEdges  int64     `json:"total_edges"`
	NewEdges    int64     `json:"new_edges"`
	ExecCount   int64     `json:"exec_count"`
	ExecPerSec  float64   `json:"exec_per_sec"`
	CorpusSize  int       `json:"corpus_size"`
	CorpusBytes int64     `json:"corpus_bytes"`
}

// CoverageProjection estimates future coverage
type CoverageProjection struct {
	EstimatedMaxCoverage int64         `json:"estimated_max_coverage"`
	TimeToReachMax       time.Duration `json:"time_to_reach_max"`
	Confidence           float64       `json:"confidence"` // 0-1
	ProjectedAt          time.Time     `json:"projected_at"`
}

// CoverageComparison compares coverage between campaigns
type CoverageComparison struct {
	Campaigns     []CampaignCoverage `json:"campaigns"`
	BestCoverage  string             `json:"best_coverage"`  // Campaign ID with highest coverage
	FastestGrowth string             `json:"fastest_growth"` // Campaign ID with fastest growth rate
	MostEfficient string             `json:"most_efficient"` // Campaign ID with best coverage/exec ratio
}

// CampaignCoverage represents coverage data for a single campaign
type CampaignCoverage struct {
	CampaignID      string    `json:"campaign_id"`
	CampaignName    string    `json:"campaign_name"`
	TotalCoverage   int64     `json:"total_coverage"`
	UniqueEdges     int64     `json:"unique_edges"`
	GrowthRate      float64   `json:"growth_rate"`
	EfficiencyRatio float64   `json:"efficiency_ratio"` // Coverage per execution
	LastUpdated     time.Time `json:"last_updated"`
}

// CrashRateMetrics represents crash rate over time
type CrashRateMetrics struct {
	CampaignID      string           `json:"campaign_id"`
	Window          time.Duration    `json:"window"`
	TotalCrashes    int              `json:"total_crashes"`
	UniqueCrashes   int              `json:"unique_crashes"`
	CrashRate       float64          `json:"crash_rate"`        // Crashes per hour
	UniqueCrashRate float64          `json:"unique_crash_rate"` // Unique crashes per hour
	Trend           string           `json:"trend"`             // "increasing", "decreasing", "stable"
	TrendConfidence float64          `json:"trend_confidence"`
	TimeSeriesData  []CrashRatePoint `json:"time_series_data"`
}

// CrashRatePoint represents crash rate at a point in time
type CrashRatePoint struct {
	Timestamp     time.Time `json:"timestamp"`
	CrashCount    int       `json:"crash_count"`
	UniqueCrashes int       `json:"unique_crashes"`
	Rate          float64   `json:"rate"`
}

// CrashDistribution shows crash types and their frequencies
type CrashDistribution struct {
	CampaignID       string               `json:"campaign_id"`
	TotalCrashes     int                  `json:"total_crashes"`
	ByType           map[string]int       `json:"by_type"`     // segfault, assertion, timeout, etc.
	BySignal         map[int]int          `json:"by_signal"`   // Signal number -> count
	BySeverity       map[string]int       `json:"by_severity"` // high, medium, low
	ByBot            map[string]int       `json:"by_bot"`      // Bot ID -> crash count
	TimeDistribution []HourlyDistribution `json:"time_distribution"`
}

// HourlyDistribution shows crashes per hour of day
type HourlyDistribution struct {
	Hour       int `json:"hour"` // 0-23
	CrashCount int `json:"crash_count"`
}

// CrashGroupStats represents statistics for a crash group
type CrashGroupStats struct {
	GroupID          string    `json:"group_id"`
	StackHash        string    `json:"stack_hash"`
	Count            int       `json:"count"`
	FirstSeen        time.Time `json:"first_seen"`
	LastSeen         time.Time `json:"last_seen"`
	Severity         string    `json:"severity"`
	TopFunction      string    `json:"top_function"`
	AffectedVersions []string  `json:"affected_versions"`
	Reproducibility  float64   `json:"reproducibility"` // 0-1
}

// FuzzerPerformance tracks performance metrics for a fuzzer type
type FuzzerPerformance struct {
	FuzzerType       string             `json:"fuzzer_type"`
	Window           time.Duration      `json:"window"`
	TotalJobs        int                `json:"total_jobs"`
	SuccessfulJobs   int                `json:"successful_jobs"`
	FailedJobs       int                `json:"failed_jobs"`
	AverageRuntime   time.Duration      `json:"average_runtime"`
	TotalExecCount   int64              `json:"total_exec_count"`
	AverageExecSpeed float64            `json:"average_exec_speed"` // Execs per second
	CoverageGain     int64              `json:"coverage_gain"`
	CrashesFound     int                `json:"crashes_found"`
	EfficiencyScore  float64            `json:"efficiency_score"` // Composite metric
	ResourceUsage    ResourceUsageStats `json:"resource_usage"`
}

// ResourceUsageStats tracks resource consumption
type ResourceUsageStats struct {
	AverageCPU       float64 `json:"average_cpu"`       // Percentage
	AverageMemory    int64   `json:"average_memory"`    // Bytes
	PeakMemory       int64   `json:"peak_memory"`       // Bytes
	DiskUsage        int64   `json:"disk_usage"`        // Bytes
	NetworkBandwidth int64   `json:"network_bandwidth"` // Bytes per second
}

// BotUtilization tracks bot usage efficiency
type BotUtilization struct {
	Window          time.Duration       `json:"window"`
	TotalBots       int                 `json:"total_bots"`
	ActiveBots      int                 `json:"active_bots"`
	IdleBots        int                 `json:"idle_bots"`
	UtilizationRate float64             `json:"utilization_rate"` // 0-1
	AverageJobTime  time.Duration       `json:"average_job_time"`
	BotPerformance  map[string]BotStats `json:"bot_performance"`
	CapabilityUsage map[string]int      `json:"capability_usage"`
}

// BotStats represents individual bot statistics
type BotStats struct {
	BotID          string        `json:"bot_id"`
	JobsCompleted  int           `json:"jobs_completed"`
	SuccessRate    float64       `json:"success_rate"`
	AverageRuntime time.Duration `json:"average_runtime"`
	IdleTime       time.Duration `json:"idle_time"`
	CrashesFound   int           `json:"crashes_found"`
	CoverageGained int64         `json:"coverage_gained"`
}

// JobThroughput measures job processing rates
type JobThroughput struct {
	Window           time.Duration `json:"window"`
	TotalJobs        int           `json:"total_jobs"`
	CompletedJobs    int           `json:"completed_jobs"`
	FailedJobs       int           `json:"failed_jobs"`
	AverageQueueTime time.Duration `json:"average_queue_time"`
	AverageRunTime   time.Duration `json:"average_run_time"`
	ThroughputRate   float64       `json:"throughput_rate"` // Jobs per hour
	QueueLength      int           `json:"queue_length"`
	Backlog          int           `json:"backlog"`
}

// CampaignSummary provides high-level campaign overview
type CampaignSummary struct {
	CampaignID       string                `json:"campaign_id"`
	Name             string                `json:"name"`
	Status           common.CampaignStatus `json:"status"`
	StartTime        time.Time             `json:"start_time"`
	Runtime          time.Duration         `json:"runtime"`
	TotalJobs        int                   `json:"total_jobs"`
	CompletedJobs    int                   `json:"completed_jobs"`
	TotalCoverage    int64                 `json:"total_coverage"`
	UniqueCrashes    int                   `json:"unique_crashes"`
	CorpusSize       int                   `json:"corpus_size"`
	ExecutionCount   int64                 `json:"execution_count"`
	ExecPerSecond    float64               `json:"exec_per_second"`
	ResourceCost     float64               `json:"resource_cost"` // Estimated cost
	EfficiencyRating string                `json:"efficiency_rating"`
}

// CampaignProgress tracks campaign completion
type CampaignProgress struct {
	CampaignID          string        `json:"campaign_id"`
	TargetDuration      time.Duration `json:"target_duration"`
	ElapsedTime         time.Duration `json:"elapsed_time"`
	ProgressPercentage  float64       `json:"progress_percentage"`
	EstimatedCompletion time.Time     `json:"estimated_completion"`
	CoverageSaturation  float64       `json:"coverage_saturation"` // How close to plateau
	MilestonesReached   []Milestone   `json:"milestones_reached"`
	NextMilestone       *Milestone    `json:"next_milestone,omitempty"`
}

// Milestone represents a campaign achievement
type Milestone struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ReachedAt   time.Time `json:"reached_at"`
	Value       int64     `json:"value"`
}

// CampaignComparison compares multiple campaigns
type CampaignComparison struct {
	Campaigns       []CampaignComparisonData `json:"campaigns"`
	BestPerformer   string                   `json:"best_performer"`
	MostEfficient   string                   `json:"most_efficient"`
	HighestCoverage string                   `json:"highest_coverage"`
	MostCrashes     string                   `json:"most_crashes"`
}

// CampaignComparisonData holds comparison data for a campaign
type CampaignComparisonData struct {
	CampaignID    string  `json:"campaign_id"`
	Name          string  `json:"name"`
	Coverage      int64   `json:"coverage"`
	CrashCount    int     `json:"crash_count"`
	ExecSpeed     float64 `json:"exec_speed"`
	Efficiency    float64 `json:"efficiency"`
	ResourceUsage float64 `json:"resource_usage"`
	OverallScore  float64 `json:"overall_score"`
}

// RealtimeMetrics provides live campaign metrics
type RealtimeMetrics struct {
	CampaignID       string    `json:"campaign_id"`
	Timestamp        time.Time `json:"timestamp"`
	ExecPerSecond    float64   `json:"exec_per_second"`
	CurrentCoverage  int64     `json:"current_coverage"`
	RecentCrashes    int       `json:"recent_crashes"` // Last 5 minutes
	ActiveBots       int       `json:"active_bots"`
	QueueLength      int       `json:"queue_length"`
	MemoryUsage      int64     `json:"memory_usage"`
	CPUUsage         float64   `json:"cpu_usage"`
	NetworkBandwidth int64     `json:"network_bandwidth"`
	Alerts           []Alert   `json:"alerts,omitempty"`
}

// Alert represents a real-time alert
type Alert struct {
	Level          string    `json:"level"` // info, warning, error
	Message        string    `json:"message"`
	Timestamp      time.Time `json:"timestamp"`
	Component      string    `json:"component"`
	ActionRequired bool      `json:"action_required"`
}

// analyticsService implementation
type analyticsService struct {
	store  StateStore
	logger *logrus.Logger
	config *AnalyticsConfig

	// Caching layer
	cache    *analyticsCache
	cacheTTL time.Duration

	// Real-time metrics
	metricsSubscribers map[string]chan *RealtimeMetrics
	subscribersMu      sync.RWMutex

	// Background workers
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// AnalyticsConfig holds configuration for analytics service
type AnalyticsConfig struct {
	CacheTTL          time.Duration `json:"cache_ttl"`
	MetricsInterval   time.Duration `json:"metrics_interval"`
	RetentionPeriod   time.Duration `json:"retention_period"`
	AggregationWindow time.Duration `json:"aggregation_window"`
	MaxSubscribers    int           `json:"max_subscribers"`
}

// analyticsCache provides caching for analytics queries
type analyticsCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	maxSize int
	ttl     time.Duration
}

type cacheEntry struct {
	data      interface{}
	timestamp time.Time
	hits      int
}

// NewAnalyticsService creates a new analytics service
func NewAnalyticsService(store StateStore, config *AnalyticsConfig, logger *logrus.Logger) AnalyticsService {
	if config == nil {
		config = &AnalyticsConfig{
			CacheTTL:          5 * time.Minute,
			MetricsInterval:   10 * time.Second,
			RetentionPeriod:   30 * 24 * time.Hour,
			AggregationWindow: 1 * time.Hour,
			MaxSubscribers:    100,
		}
	}

	if logger == nil {
		logger = logrus.New()
	}

	ctx, cancel := context.WithCancel(context.Background())

	svc := &analyticsService{
		store:              store,
		logger:             logger,
		config:             config,
		cache:              newAnalyticsCache(1000, config.CacheTTL),
		cacheTTL:           config.CacheTTL,
		metricsSubscribers: make(map[string]chan *RealtimeMetrics),
		ctx:                ctx,
		cancel:             cancel,
	}

	return svc
}

// Start initializes the analytics service
func (s *analyticsService) Start(ctx context.Context) error {
	s.logger.Info("Starting analytics service")

	// Start background workers
	s.wg.Add(2)
	go s.cacheCleanupWorker()
	go s.metricsAggregationWorker()

	s.logger.Info("Analytics service started successfully")
	return nil
}

// Stop gracefully shuts down the analytics service
func (s *analyticsService) Stop() error {
	s.logger.Info("Stopping analytics service")

	// Cancel context to stop workers
	s.cancel()

	// Close all metric subscriptions
	s.subscribersMu.Lock()
	for _, ch := range s.metricsSubscribers {
		close(ch)
	}
	s.metricsSubscribers = make(map[string]chan *RealtimeMetrics)
	s.subscribersMu.Unlock()

	// Wait for workers to finish
	s.wg.Wait()

	s.logger.Info("Analytics service stopped")
	return nil
}

// GetCoverageTrend analyzes coverage growth over time
func (s *analyticsService) GetCoverageTrend(ctx context.Context, campaignID string, interval time.Duration) (*CoverageTrend, error) {
	cacheKey := fmt.Sprintf("coverage_trend:%s:%v", campaignID, interval)

	// Check cache
	if cached := s.cache.get(cacheKey); cached != nil {
		if trend, ok := cached.(*CoverageTrend); ok {
			return trend, nil
		}
	}

	// Get coverage data from store
	endTime := time.Now()
	startTime := endTime.Add(-interval)

	// Aggregate coverage data
	trend := &CoverageTrend{
		CampaignID: campaignID,
		Interval:   interval,
		StartTime:  startTime,
		EndTime:    endTime,
		DataPoints: make([]CoveragePoint, 0),
	}

	// Get all jobs for this campaign
	jobs, err := s.store.GetCampaignJobs(ctx, campaignID)
	if err != nil {
		s.logger.WithError(err).WithField("campaign_id", campaignID).Warn("Failed to get campaign jobs")
	} else {
		// Aggregate coverage data points from all jobs
		coverageByTime := make(map[int64]*CoveragePoint)

		for _, job := range jobs {
			jobCoverage, err := s.store.GetJobCoverageHistory(ctx, job.ID, startTime, endTime)
			if err != nil {
				s.logger.WithError(err).WithField("job_id", job.ID).Warn("Failed to get job coverage history")
				continue
			}

			for _, cov := range jobCoverage {
				// Bucket by hour for trend analysis
				bucketTime := cov.Timestamp.Truncate(time.Hour).Unix()
				if point, exists := coverageByTime[bucketTime]; exists {
					point.TotalEdges += int64(cov.Edges)
					point.NewEdges += int64(cov.NewEdges)
					point.ExecCount += cov.ExecCount
				} else {
					coverageByTime[bucketTime] = &CoveragePoint{
						Timestamp:  time.Unix(bucketTime, 0),
						TotalEdges: int64(cov.Edges),
						NewEdges:   int64(cov.NewEdges),
						ExecCount:  cov.ExecCount,
					}
				}
			}
		}

		// Convert map to sorted slice
		for _, point := range coverageByTime {
			trend.DataPoints = append(trend.DataPoints, *point)
		}

		// Sort by timestamp
		for i := 0; i < len(trend.DataPoints)-1; i++ {
			for j := i + 1; j < len(trend.DataPoints); j++ {
				if trend.DataPoints[i].Timestamp.After(trend.DataPoints[j].Timestamp) {
					trend.DataPoints[i], trend.DataPoints[j] = trend.DataPoints[j], trend.DataPoints[i]
				}
			}
		}
	}

	// Calculate derived metrics
	if len(trend.DataPoints) > 0 {
		firstPoint := trend.DataPoints[0]
		lastPoint := trend.DataPoints[len(trend.DataPoints)-1]

		trend.TotalGrowth = lastPoint.TotalEdges - firstPoint.TotalEdges
		hours := interval.Hours()
		if hours > 0 {
			trend.GrowthRate = float64(trend.TotalGrowth) / hours
		}

		// Add projection if enough data
		if len(trend.DataPoints) >= 10 {
			trend.Projection = s.projectCoverage(trend.DataPoints)
		}
	}

	// Cache result
	s.cache.set(cacheKey, trend)

	return trend, nil
}

// GetCrashRate calculates crash rate metrics
func (s *analyticsService) GetCrashRate(ctx context.Context, campaignID string, window time.Duration) (*CrashRateMetrics, error) {
	cacheKey := fmt.Sprintf("crash_rate:%s:%v", campaignID, window)

	// Check cache
	if cached := s.cache.get(cacheKey); cached != nil {
		if metrics, ok := cached.(*CrashRateMetrics); ok {
			return metrics, nil
		}
	}

	endTime := time.Now()
	startTime := endTime.Add(-window)

	metrics := &CrashRateMetrics{
		CampaignID:     campaignID,
		Window:         window,
		TimeSeriesData: make([]CrashRatePoint, 0),
	}

	// Get all jobs for this campaign
	jobs, err := s.store.GetCampaignJobs(ctx, campaignID)
	if err != nil {
		s.logger.WithError(err).WithField("campaign_id", campaignID).Warn("Failed to get campaign jobs")
		return metrics, nil
	}

	// Collect crashes from all jobs
	crashesByHour := make(map[int64]*CrashRatePoint)
	uniqueHashes := make(map[string]bool)

	for _, job := range jobs {
		crashes, err := s.store.GetJobCrashes(ctx, job.ID)
		if err != nil {
			s.logger.WithError(err).WithField("job_id", job.ID).Warn("Failed to get job crashes")
			continue
		}

		for _, crash := range crashes {
			// Filter by time window
			if crash.Timestamp.Before(startTime) || crash.Timestamp.After(endTime) {
				continue
			}

			metrics.TotalCrashes++
			if !uniqueHashes[crash.Hash] {
				uniqueHashes[crash.Hash] = true
				metrics.UniqueCrashes++
			}

			// Bucket by hour for time series
			bucketTime := crash.Timestamp.Truncate(time.Hour).Unix()
			if point, exists := crashesByHour[bucketTime]; exists {
				point.CrashCount++
				if crash.IsUnique {
					point.UniqueCrashes++
				}
			} else {
				uniqueCount := 0
				if crash.IsUnique {
					uniqueCount = 1
				}
				crashesByHour[bucketTime] = &CrashRatePoint{
					Timestamp:     time.Unix(bucketTime, 0),
					CrashCount:    1,
					UniqueCrashes: uniqueCount,
				}
			}
		}
	}

	// Convert map to sorted slice and calculate rates
	for _, point := range crashesByHour {
		point.Rate = float64(point.CrashCount)
		metrics.TimeSeriesData = append(metrics.TimeSeriesData, *point)
	}

	// Sort by timestamp
	for i := 0; i < len(metrics.TimeSeriesData)-1; i++ {
		for j := i + 1; j < len(metrics.TimeSeriesData); j++ {
			if metrics.TimeSeriesData[i].Timestamp.After(metrics.TimeSeriesData[j].Timestamp) {
				metrics.TimeSeriesData[i], metrics.TimeSeriesData[j] = metrics.TimeSeriesData[j], metrics.TimeSeriesData[i]
			}
		}
	}

	// Calculate rates per hour
	hours := window.Hours()
	if hours > 0 {
		metrics.CrashRate = float64(metrics.TotalCrashes) / hours
		metrics.UniqueCrashRate = float64(metrics.UniqueCrashes) / hours
	}

	// Determine trend
	metrics.Trend = s.analyzeTrend(metrics.TimeSeriesData)
	metrics.TrendConfidence = s.calculateTrendConfidence(metrics.TimeSeriesData)

	// Cache result
	s.cache.set(cacheKey, metrics)

	return metrics, nil
}

// GetFuzzerPerformance analyzes performance by fuzzer type
func (s *analyticsService) GetFuzzerPerformance(ctx context.Context, fuzzerType string, window time.Duration) (*FuzzerPerformance, error) {
	cacheKey := fmt.Sprintf("fuzzer_perf:%s:%v", fuzzerType, window)

	// Check cache
	if cached := s.cache.get(cacheKey); cached != nil {
		if perf, ok := cached.(*FuzzerPerformance); ok {
			return perf, nil
		}
	}

	endTime := time.Now()
	startTime := endTime.Add(-window)

	perf := &FuzzerPerformance{
		FuzzerType: fuzzerType,
		Window:     window,
	}

	// Get all jobs and filter by fuzzer type
	allJobs, err := s.store.ListJobs()
	if err != nil {
		s.logger.WithError(err).Warn("Failed to list jobs for fuzzer performance")
		return perf, nil
	}

	var totalRuntime time.Duration
	var completedJobs int

	for _, job := range allJobs {
		// Filter by fuzzer type and time window
		if job.Fuzzer != fuzzerType {
			continue
		}
		if job.CreatedAt.Before(startTime) {
			continue
		}

		perf.TotalJobs++

		if job.Status == common.JobStatusCompleted {
			perf.SuccessfulJobs++
			completedJobs++
			if job.CompletedAt != nil && job.StartedAt != nil {
				totalRuntime += job.CompletedAt.Sub(*job.StartedAt)
			}
		} else if job.Status == common.JobStatusFailed || job.Status == common.JobStatusTimedOut {
			perf.FailedJobs++
		}

		// Get coverage and crashes for this job
		if job.Status == common.JobStatusCompleted || job.Status == common.JobStatusRunning {
			coverage, err := s.store.GetJobCoverageHistory(ctx, job.ID, startTime, endTime)
			if err == nil && len(coverage) > 0 {
				lastCov := coverage[len(coverage)-1]
				perf.CoverageGain += int64(lastCov.Edges)
				perf.TotalExecCount += lastCov.ExecCount
			}

			crashes, err := s.store.GetJobCrashes(ctx, job.ID)
			if err == nil {
				perf.CrashesFound += len(crashes)
			}
		}
	}

	// Calculate averages
	if completedJobs > 0 {
		perf.AverageRuntime = totalRuntime / time.Duration(completedJobs)
	}
	if perf.TotalExecCount > 0 && perf.AverageRuntime > 0 {
		perf.AverageExecSpeed = float64(perf.TotalExecCount) / perf.AverageRuntime.Seconds()
	}

	// Calculate efficiency score (composite metric)
	perf.EfficiencyScore = s.calculateEfficiencyScore(perf)

	// Cache result
	s.cache.set(cacheKey, perf)

	return perf, nil
}

// Helper methods

func (s *analyticsService) projectCoverage(dataPoints []CoveragePoint) *CoverageProjection {
	// Simple projection based on growth rate decay
	// In real implementation, this would use more sophisticated modeling

	if len(dataPoints) < 2 {
		return nil
	}

	// Calculate average growth rate
	totalGrowth := float64(0)
	for i := 1; i < len(dataPoints); i++ {
		growth := float64(dataPoints[i].TotalEdges - dataPoints[i-1].TotalEdges)
		totalGrowth += growth
	}
	avgGrowth := totalGrowth / float64(len(dataPoints)-1)

	// Estimate maximum coverage (simplified)
	currentCoverage := dataPoints[len(dataPoints)-1].TotalEdges
	estimatedMax := int64(float64(currentCoverage) * 1.5) // Simplified estimate

	// Calculate time to reach max
	remainingCoverage := estimatedMax - currentCoverage
	hoursToMax := float64(remainingCoverage) / avgGrowth

	return &CoverageProjection{
		EstimatedMaxCoverage: estimatedMax,
		TimeToReachMax:       time.Duration(hoursToMax) * time.Hour,
		Confidence:           0.7, // Simplified confidence
		ProjectedAt:          time.Now(),
	}
}

func (s *analyticsService) analyzeTrend(data []CrashRatePoint) string {
	if len(data) < 3 {
		return "stable"
	}

	// Simple trend analysis - compare first and last thirds
	firstThird := len(data) / 3
	lastThird := len(data) - firstThird

	firstAvg := float64(0)
	for i := 0; i < firstThird; i++ {
		firstAvg += data[i].Rate
	}
	firstAvg /= float64(firstThird)

	lastAvg := float64(0)
	for i := lastThird; i < len(data); i++ {
		lastAvg += data[i].Rate
	}
	lastAvg /= float64(len(data) - lastThird)

	difference := lastAvg - firstAvg
	threshold := firstAvg * 0.1 // 10% change threshold

	if difference > threshold {
		return "increasing"
	} else if difference < -threshold {
		return "decreasing"
	}
	return "stable"
}

func (s *analyticsService) calculateTrendConfidence(data []CrashRatePoint) float64 {
	// Simplified confidence calculation based on data consistency
	if len(data) < 5 {
		return 0.3
	}
	if len(data) < 10 {
		return 0.6
	}
	return 0.8
}

func (s *analyticsService) calculateEfficiencyScore(perf *FuzzerPerformance) float64 {
	// Composite efficiency score based on multiple factors
	successRate := float64(perf.SuccessfulJobs) / float64(perf.TotalJobs)
	coverageEfficiency := float64(perf.CoverageGain) / float64(perf.TotalExecCount+1)
	crashEfficiency := float64(perf.CrashesFound) / float64(perf.TotalJobs+1)

	// Weighted average
	score := (successRate * 0.3) + (coverageEfficiency * 0.4) + (crashEfficiency * 0.3)

	// Normalize to 0-100
	return score * 100
}

// Background workers

func (s *analyticsService) cacheCleanupWorker() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cache.cleanup()
		}
	}
}

func (s *analyticsService) metricsAggregationWorker() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Aggregate and broadcast real-time metrics
			s.broadcastMetrics()
		}
	}
}

func (s *analyticsService) broadcastMetrics() {
	// TODO: Implement real-time metrics aggregation and broadcasting
	s.subscribersMu.RLock()
	defer s.subscribersMu.RUnlock()

	// For each active campaign with subscribers, calculate and send metrics
	for _, ch := range s.metricsSubscribers {
		select {
		case ch <- &RealtimeMetrics{
			Timestamp: time.Now(),
			// TODO: Fill with actual metrics
		}:
		default:
			// Channel full, skip
		}
	}
}

// Cache implementation

func newAnalyticsCache(maxSize int, ttl time.Duration) *analyticsCache {
	return &analyticsCache{
		entries: make(map[string]*cacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

func (c *analyticsCache) get(key string) interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil
	}

	// Check if expired
	if time.Since(entry.timestamp) > c.ttl {
		return nil
	}

	entry.hits++
	return entry.data
}

func (c *analyticsCache) set(key string, data interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict oldest entry if at capacity
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = &cacheEntry{
		data:      data,
		timestamp: time.Now(),
		hits:      0,
	}
}

func (c *analyticsCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.timestamp.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.timestamp
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

func (c *analyticsCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.Sub(entry.timestamp) > c.ttl {
			delete(c.entries, key)
		}
	}
}

// CompareCampaigns compares multiple campaigns
func (s *analyticsService) CompareCampaigns(ctx context.Context, campaignIDs []string) (*CampaignComparison, error) {
	comparison := &CampaignComparison{
		Campaigns: make([]CampaignComparisonData, 0, len(campaignIDs)),
	}

	var bestCoverage, mostCrashes, mostEfficient, bestPerformer string
	var maxCoverage int64
	var maxCrashes int
	var maxEfficiency, maxScore float64

	for _, campaignID := range campaignIDs {
		data := CampaignComparisonData{CampaignID: campaignID}

		// Get campaign jobs
		jobs, err := s.store.GetCampaignJobs(ctx, campaignID)
		if err != nil {
			s.logger.WithError(err).WithField("campaign_id", campaignID).Warn("Failed to get campaign jobs")
			comparison.Campaigns = append(comparison.Campaigns, data)
			continue
		}

		endTime := time.Now()
		startTime := endTime.Add(-24 * time.Hour)

		var totalExecs int64
		for _, job := range jobs {
			// Get coverage
			coverage, err := s.store.GetJobCoverageHistory(ctx, job.ID, startTime, endTime)
			if err == nil && len(coverage) > 0 {
				lastCov := coverage[len(coverage)-1]
				data.Coverage += int64(lastCov.Edges)
				totalExecs += lastCov.ExecCount
			}

			// Get crashes
			crashes, err := s.store.GetJobCrashes(ctx, job.ID)
			if err == nil {
				data.CrashCount += len(crashes)
			}
		}

		// Calculate efficiency
		if totalExecs > 0 {
			data.Efficiency = float64(data.Coverage) / float64(totalExecs) * 1000000
		}

		// Calculate overall score
		data.OverallScore = float64(data.Coverage)*0.4 + float64(data.CrashCount)*10*0.3 + data.Efficiency*0.3

		// Track best performers
		if data.Coverage > maxCoverage {
			maxCoverage = data.Coverage
			bestCoverage = campaignID
		}
		if data.CrashCount > maxCrashes {
			maxCrashes = data.CrashCount
			mostCrashes = campaignID
		}
		if data.Efficiency > maxEfficiency {
			maxEfficiency = data.Efficiency
			mostEfficient = campaignID
		}
		if data.OverallScore > maxScore {
			maxScore = data.OverallScore
			bestPerformer = campaignID
		}

		comparison.Campaigns = append(comparison.Campaigns, data)
	}

	comparison.HighestCoverage = bestCoverage
	comparison.MostCrashes = mostCrashes
	comparison.MostEfficient = mostEfficient
	comparison.BestPerformer = bestPerformer

	return comparison, nil
}

// GetBotUtilization gets bot utilization metrics
func (s *analyticsService) GetBotUtilization(ctx context.Context, window time.Duration) (*BotUtilization, error) {
	util := &BotUtilization{
		Window:          window,
		BotPerformance:  make(map[string]BotStats),
		CapabilityUsage: make(map[string]int),
	}

	// Get all bots
	bots, err := s.store.ListBots()
	if err != nil {
		s.logger.WithError(err).Warn("Failed to list bots for utilization")
		return util, nil
	}

	util.TotalBots = len(bots)
	endTime := time.Now()
	startTime := endTime.Add(-window)

	for _, bot := range bots {
		if bot.IsOnline {
			if bot.CurrentJob != nil {
				util.ActiveBots++
			} else {
				util.IdleBots++
			}
		}

		// Track capability usage
		for _, cap := range bot.Capabilities {
			util.CapabilityUsage[cap]++
		}

		// Calculate bot performance
		stats := BotStats{
			BotID: bot.ID,
		}

		// Get jobs completed by this bot in the time window
		allJobs, err := s.store.ListJobs()
		if err == nil {
			for _, job := range allJobs {
				if job.AssignedBot == nil || *job.AssignedBot != bot.ID {
					continue
				}
				if job.CompletedAt == nil || job.CompletedAt.Before(startTime) {
					continue
				}

				stats.JobsCompleted++
				if job.Status == common.JobStatusCompleted {
					if job.StartedAt != nil && job.CompletedAt != nil {
						stats.AverageRuntime += job.CompletedAt.Sub(*job.StartedAt)
					}
				}

				// Count crashes
				crashes, err := s.store.GetJobCrashes(ctx, job.ID)
				if err == nil {
					stats.CrashesFound += len(crashes)
				}
			}

			if stats.JobsCompleted > 0 {
				stats.AverageRuntime = stats.AverageRuntime / time.Duration(stats.JobsCompleted)
				stats.SuccessRate = 1.0 // Simplified - all completed jobs are successful
				util.AverageJobTime += stats.AverageRuntime
			}
		}

		util.BotPerformance[bot.ID] = stats
	}

	// Calculate utilization rate
	if util.TotalBots > 0 {
		util.UtilizationRate = float64(util.ActiveBots) / float64(util.TotalBots)
		if util.ActiveBots > 0 {
			util.AverageJobTime = util.AverageJobTime / time.Duration(util.ActiveBots)
		}
	}

	return util, nil
}

// GetCampaignProgress gets progress metrics for a campaign
func (s *analyticsService) GetCampaignProgress(ctx context.Context, campaignID string) (*CampaignProgress, error) {
	progress := &CampaignProgress{
		CampaignID:        campaignID,
		MilestonesReached: make([]Milestone, 0),
	}

	// Get campaign jobs
	jobs, err := s.store.GetCampaignJobs(ctx, campaignID)
	if err != nil {
		s.logger.WithError(err).WithField("campaign_id", campaignID).Warn("Failed to get campaign jobs")
		return progress, nil
	}

	if len(jobs) == 0 {
		return progress, nil
	}

	// Find start time from earliest job
	var startTime time.Time
	totalJobs := len(jobs)
	completedJobs := 0
	var totalCoverage int64

	for _, job := range jobs {
		if startTime.IsZero() || job.CreatedAt.Before(startTime) {
			startTime = job.CreatedAt
		}

		if job.Status == common.JobStatusCompleted {
			completedJobs++
		}

		// Get coverage for this job
		coverage, err := s.store.GetJobCoverageHistory(ctx, job.ID, startTime, time.Now())
		if err == nil && len(coverage) > 0 {
			totalCoverage += int64(coverage[len(coverage)-1].Edges)
		}
	}

	progress.ElapsedTime = time.Since(startTime)
	if totalJobs > 0 {
		progress.ProgressPercentage = float64(completedJobs) / float64(totalJobs) * 100
	}

	// Add milestones
	if completedJobs > 0 {
		progress.MilestonesReached = append(progress.MilestonesReached, Milestone{
			Name:        "First job completed",
			Description: "Completed first fuzzing job",
			ReachedAt:   time.Now(),
			Value:       1,
		})
	}
	if totalCoverage >= 1000 {
		progress.MilestonesReached = append(progress.MilestonesReached, Milestone{
			Name:        "1K coverage edges",
			Description: "Reached 1000 coverage edges",
			ReachedAt:   time.Now(),
			Value:       totalCoverage,
		})
	}

	return progress, nil
}

// GetCampaignSummary gets summary metrics for a campaign
func (s *analyticsService) GetCampaignSummary(ctx context.Context, campaignID string) (*CampaignSummary, error) {
	summary := &CampaignSummary{
		CampaignID: campaignID,
		StartTime:  time.Now(),
	}

	// Get campaign jobs
	jobs, err := s.store.GetCampaignJobs(ctx, campaignID)
	if err != nil {
		s.logger.WithError(err).WithField("campaign_id", campaignID).Warn("Failed to get campaign jobs")
		return summary, nil
	}

	summary.TotalJobs = len(jobs)

	for _, job := range jobs {
		if summary.StartTime.IsZero() || job.CreatedAt.Before(summary.StartTime) {
			summary.StartTime = job.CreatedAt
		}

		if job.Status == common.JobStatusCompleted {
			summary.CompletedJobs++
		}

		// Get coverage and crashes
		coverage, err := s.store.GetJobCoverageHistory(ctx, job.ID, summary.StartTime, time.Now())
		if err == nil && len(coverage) > 0 {
			lastCov := coverage[len(coverage)-1]
			summary.TotalCoverage += int64(lastCov.Edges)
			summary.ExecutionCount += lastCov.ExecCount
		}

		crashes, err := s.store.GetJobCrashes(ctx, job.ID)
		if err == nil {
			for _, c := range crashes {
				if c.IsUnique {
					summary.UniqueCrashes++
				}
			}
		}
	}

	summary.Runtime = time.Since(summary.StartTime)
	if summary.Runtime.Seconds() > 0 {
		summary.ExecPerSecond = float64(summary.ExecutionCount) / summary.Runtime.Seconds()
	}

	// Calculate efficiency rating
	if summary.TotalCoverage > 10000 && summary.UniqueCrashes > 5 {
		summary.EfficiencyRating = "high"
	} else if summary.TotalCoverage > 1000 || summary.UniqueCrashes > 0 {
		summary.EfficiencyRating = "medium"
	} else {
		summary.EfficiencyRating = "low"
	}

	return summary, nil
}

// GetCoverageComparison compares coverage across campaigns
func (s *analyticsService) GetCoverageComparison(ctx context.Context, campaignIDs []string) (*CoverageComparison, error) {
	comparison := &CoverageComparison{
		Campaigns: make([]CampaignCoverage, 0, len(campaignIDs)),
	}

	var bestCoverage, fastestGrowth, mostEfficient string
	var maxCoverage int64
	var maxGrowth, maxEfficiency float64

	for _, campaignID := range campaignIDs {
		campCov := CampaignCoverage{
			CampaignID:  campaignID,
			LastUpdated: time.Now(),
		}

		jobs, err := s.store.GetCampaignJobs(ctx, campaignID)
		if err != nil {
			comparison.Campaigns = append(comparison.Campaigns, campCov)
			continue
		}

		endTime := time.Now()
		startTime := endTime.Add(-24 * time.Hour)
		var totalExecs int64
		var firstEdges, lastEdges int64
		var firstTime, lastTime time.Time

		for _, job := range jobs {
			coverage, err := s.store.GetJobCoverageHistory(ctx, job.ID, startTime, endTime)
			if err == nil && len(coverage) > 0 {
				for _, cov := range coverage {
					campCov.TotalCoverage += int64(cov.Edges)
					totalExecs += cov.ExecCount

					if firstTime.IsZero() || cov.Timestamp.Before(firstTime) {
						firstTime = cov.Timestamp
						firstEdges = int64(cov.Edges)
					}
					if lastTime.IsZero() || cov.Timestamp.After(lastTime) {
						lastTime = cov.Timestamp
						lastEdges = int64(cov.Edges)
					}
				}
			}
		}

		// Calculate growth rate
		if !firstTime.IsZero() && !lastTime.IsZero() {
			hours := lastTime.Sub(firstTime).Hours()
			if hours > 0 {
				campCov.GrowthRate = float64(lastEdges-firstEdges) / hours
			}
		}

		// Calculate efficiency
		if totalExecs > 0 {
			campCov.EfficiencyRatio = float64(campCov.TotalCoverage) / float64(totalExecs)
		}

		// Track best performers
		if campCov.TotalCoverage > maxCoverage {
			maxCoverage = campCov.TotalCoverage
			bestCoverage = campaignID
		}
		if campCov.GrowthRate > maxGrowth {
			maxGrowth = campCov.GrowthRate
			fastestGrowth = campaignID
		}
		if campCov.EfficiencyRatio > maxEfficiency {
			maxEfficiency = campCov.EfficiencyRatio
			mostEfficient = campaignID
		}

		comparison.Campaigns = append(comparison.Campaigns, campCov)
	}

	comparison.BestCoverage = bestCoverage
	comparison.FastestGrowth = fastestGrowth
	comparison.MostEfficient = mostEfficient

	return comparison, nil
}

// GetCrashDistribution gets crash distribution metrics
func (s *analyticsService) GetCrashDistribution(ctx context.Context, campaignID string) (*CrashDistribution, error) {
	dist := &CrashDistribution{
		CampaignID:       campaignID,
		ByType:           make(map[string]int),
		BySignal:         make(map[int]int),
		BySeverity:       make(map[string]int),
		ByBot:            make(map[string]int),
		TimeDistribution: make([]HourlyDistribution, 24),
	}

	// Initialize hourly distribution
	for i := 0; i < 24; i++ {
		dist.TimeDistribution[i] = HourlyDistribution{Hour: i}
	}

	// Get campaign jobs
	jobs, err := s.store.GetCampaignJobs(ctx, campaignID)
	if err != nil {
		s.logger.WithError(err).WithField("campaign_id", campaignID).Warn("Failed to get campaign jobs")
		return dist, nil
	}

	for _, job := range jobs {
		crashes, err := s.store.GetJobCrashes(ctx, job.ID)
		if err != nil {
			continue
		}

		for _, crash := range crashes {
			dist.TotalCrashes++

			// By type
			dist.ByType[crash.Type]++

			// By signal
			if crash.Signal != 0 {
				dist.BySignal[crash.Signal]++
			}

			// By bot
			dist.ByBot[crash.BotID]++

			// By severity (based on signal)
			severity := "low"
			if crash.Signal == 11 || crash.Signal == 6 { // SIGSEGV or SIGABRT
				severity = "high"
			} else if crash.Signal != 0 {
				severity = "medium"
			}
			dist.BySeverity[severity]++

			// By hour
			hour := crash.Timestamp.Hour()
			dist.TimeDistribution[hour].CrashCount++
		}
	}

	return dist, nil
}

// GetTopCrashGroups gets top crash groups
func (s *analyticsService) GetTopCrashGroups(ctx context.Context, campaignID string, limit int) ([]*CrashGroupStats, error) {
	groups := make(map[string]*CrashGroupStats)

	// Get campaign jobs
	jobs, err := s.store.GetCampaignJobs(ctx, campaignID)
	if err != nil {
		s.logger.WithError(err).WithField("campaign_id", campaignID).Warn("Failed to get campaign jobs")
		return make([]*CrashGroupStats, 0), nil
	}

	for _, job := range jobs {
		crashes, err := s.store.GetJobCrashes(ctx, job.ID)
		if err != nil {
			continue
		}

		for _, crash := range crashes {
			if group, exists := groups[crash.Hash]; exists {
				group.Count++
				if crash.Timestamp.After(group.LastSeen) {
					group.LastSeen = crash.Timestamp
				}
			} else {
				groups[crash.Hash] = &CrashGroupStats{
					GroupID:   crash.Hash[:8],
					StackHash: crash.Hash,
					Count:     1,
					FirstSeen: crash.Timestamp,
					LastSeen:  crash.Timestamp,
					Severity:  determineSeverity(crash.Signal),
				}
			}
		}
	}

	// Convert to slice and sort by count
	result := make([]*CrashGroupStats, 0, len(groups))
	for _, group := range groups {
		result = append(result, group)
	}

	// Sort by count descending
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Count < result[j].Count {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	// Limit results
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// determineSeverity returns severity based on signal number
func determineSeverity(signal int) string {
	switch signal {
	case 11, 6: // SIGSEGV, SIGABRT
		return "high"
	case 4, 7, 8: // SIGILL, SIGBUS, SIGFPE
		return "medium"
	default:
		return "low"
	}
}

// GetJobThroughput gets job throughput metrics
func (s *analyticsService) GetJobThroughput(ctx context.Context, window time.Duration) (*JobThroughput, error) {
	throughput := &JobThroughput{
		Window: window,
	}

	endTime := time.Now()
	startTime := endTime.Add(-window)

	// Get all jobs
	allJobs, err := s.store.ListJobs()
	if err != nil {
		s.logger.WithError(err).Warn("Failed to list jobs for throughput")
		return throughput, nil
	}

	var totalQueueTime, totalRunTime time.Duration
	var queuedJobs, runningJobs int

	for _, job := range allJobs {
		if job.CreatedAt.Before(startTime) {
			continue
		}

		throughput.TotalJobs++

		switch job.Status {
		case common.JobStatusCompleted:
			throughput.CompletedJobs++
			if job.StartedAt != nil {
				totalQueueTime += job.StartedAt.Sub(job.CreatedAt)
			}
			if job.StartedAt != nil && job.CompletedAt != nil {
				totalRunTime += job.CompletedAt.Sub(*job.StartedAt)
			}
		case common.JobStatusFailed, common.JobStatusTimedOut:
			throughput.FailedJobs++
		case common.JobStatusPending:
			throughput.QueueLength++
		case common.JobStatusRunning, common.JobStatusAssigned:
			runningJobs++
			if job.StartedAt != nil {
				queuedJobs++
				totalQueueTime += job.StartedAt.Sub(job.CreatedAt)
			}
		}
	}

	// Calculate averages
	if throughput.CompletedJobs > 0 {
		throughput.AverageQueueTime = totalQueueTime / time.Duration(throughput.CompletedJobs+queuedJobs)
		throughput.AverageRunTime = totalRunTime / time.Duration(throughput.CompletedJobs)
	}

	// Calculate throughput rate (jobs per hour)
	hours := window.Hours()
	if hours > 0 {
		throughput.ThroughputRate = float64(throughput.CompletedJobs) / hours
	}

	throughput.Backlog = throughput.QueueLength

	return throughput, nil
}

// GetRealtimeMetrics gets real-time metrics for a campaign
func (s *analyticsService) GetRealtimeMetrics(ctx context.Context, campaignID string) (*RealtimeMetrics, error) {
	metrics := &RealtimeMetrics{
		CampaignID: campaignID,
		Timestamp:  time.Now(),
		Alerts:     make([]Alert, 0),
	}

	// Get campaign jobs
	jobs, err := s.store.GetCampaignJobs(ctx, campaignID)
	if err != nil {
		s.logger.WithError(err).WithField("campaign_id", campaignID).Warn("Failed to get campaign jobs")
		return metrics, nil
	}

	endTime := time.Now()
	startTime := endTime.Add(-5 * time.Minute)

	for _, job := range jobs {
		if job.Status == common.JobStatusRunning {
			metrics.ActiveBots++
		}
		if job.Status == common.JobStatusPending {
			metrics.QueueLength++
		}

		// Get recent coverage
		coverage, err := s.store.GetJobCoverageHistory(ctx, job.ID, startTime, endTime)
		if err == nil && len(coverage) > 0 {
			lastCov := coverage[len(coverage)-1]
			metrics.CurrentCoverage += int64(lastCov.Edges)
			if lastCov.ExecCount > 0 {
				// Approximate exec/sec based on recent data
				duration := endTime.Sub(startTime).Seconds()
				if duration > 0 {
					metrics.ExecPerSecond += float64(lastCov.ExecCount) / duration
				}
			}
		}

		// Get recent crashes
		crashes, err := s.store.GetJobCrashes(ctx, job.ID)
		if err == nil {
			for _, c := range crashes {
				if c.Timestamp.After(startTime) {
					metrics.RecentCrashes++
				}
			}
		}
	}

	// Add alerts for potential issues
	if metrics.QueueLength > 10 {
		metrics.Alerts = append(metrics.Alerts, Alert{
			Level:          "warning",
			Message:        fmt.Sprintf("Job queue is backing up (%d pending jobs)", metrics.QueueLength),
			Timestamp:      time.Now(),
			Component:      "scheduler",
			ActionRequired: false,
		})
	}

	return metrics, nil
}

// SubscribeToMetrics subscribes to real-time metrics updates
func (s *analyticsService) SubscribeToMetrics(ctx context.Context, campaignID string) (<-chan *RealtimeMetrics, error) {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()

	// Check max subscribers
	if len(s.metricsSubscribers) >= s.config.MaxSubscribers {
		return nil, fmt.Errorf("maximum number of subscribers reached")
	}

	// Create subscription channel
	ch := make(chan *RealtimeMetrics, 10)
	subscriptionID := fmt.Sprintf("%s_%d", campaignID, time.Now().UnixNano())
	s.metricsSubscribers[subscriptionID] = ch

	// Start a goroutine to push metrics for this subscription
	go func() {
		ticker := time.NewTicker(s.config.MetricsInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.subscribersMu.Lock()
				delete(s.metricsSubscribers, subscriptionID)
				close(ch)
				s.subscribersMu.Unlock()
				return
			case <-ticker.C:
				metrics, err := s.GetRealtimeMetrics(ctx, campaignID)
				if err != nil {
					continue
				}
				select {
				case ch <- metrics:
				default:
					// Channel full, skip this update
				}
			}
		}
	}()

	return ch, nil
}

// UnsubscribeFromMetrics unsubscribes from metrics updates
func (s *analyticsService) UnsubscribeFromMetrics(ctx context.Context, subscriptionID string) error {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()

	ch, exists := s.metricsSubscribers[subscriptionID]
	if !exists {
		return fmt.Errorf("subscription not found: %s", subscriptionID)
	}

	delete(s.metricsSubscribers, subscriptionID)
	close(ch)

	return nil
}
