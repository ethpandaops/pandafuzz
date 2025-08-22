package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/sse"
	campaignRepo "github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	crashRepo "github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	jobRepo "github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
)

// AnalyticsAdapter implements the analytics-related endpoints of the generated ServerInterface
type AnalyticsAdapter struct {
	jobRepo      jobRepo.JobRepository
	crashRepo    crashRepo.CrashRepository
	campaignRepo campaignRepo.CampaignRepository
	sse          *sse.Manager
	logger       logrus.FieldLogger
}

// NewAnalyticsAdapter creates a new analytics adapter
func NewAnalyticsAdapter(
	jobRepo jobRepo.JobRepository,
	crashRepo crashRepo.CrashRepository,
	campaignRepo campaignRepo.CampaignRepository,
	sse *sse.Manager,
	logger logrus.FieldLogger,
) *AnalyticsAdapter {
	return &AnalyticsAdapter{
		jobRepo:      jobRepo,
		crashRepo:    crashRepo,
		campaignRepo: campaignRepo,
		sse:          sse,
		logger:       logger.WithField("component", "analytics_adapter"),
	}
}

// GetAnalytics retrieves overall system analytics
func (a *AnalyticsAdapter) GetAnalytics(w http.ResponseWriter, r *http.Request, params generated.GetAnalyticsParams) {
	ctx := r.Context()

	// Determine time range
	timeRange := a.getTimeRange(params.TimeRange)

	// Get system overview
	systemOverview := a.getSystemOverview(ctx, params.CampaignId)

	// Get performance metrics
	performanceMetrics := a.getPerformanceMetrics(ctx, timeRange, params.CampaignId)

	// Get resource usage
	resourceUsage := a.getResourceUsage(ctx)

	// Get trends data
	trends := a.getTrends(ctx, timeRange, params.CampaignId)

	response := generated.AnalyticsResponse{
		GeneratedAt:        time.Now(),
		SystemOverview:     systemOverview,
		PerformanceMetrics: performanceMetrics,
		ResourceUsage:      resourceUsage,
		TimeRange: struct {
			Duration *string    `json:"duration,omitempty"`
			End      *time.Time `json:"end,omitempty"`
			Start    *time.Time `json:"start,omitempty"`
		}{
			Duration: &timeRange.Duration,
			Start:    &timeRange.Start,
			End:      &timeRange.End,
		},
		Trends: trends,
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// GetMetrics retrieves real-time system metrics
func (a *AnalyticsAdapter) GetMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	metrics := a.getRealTimeMetrics(ctx)

	response := generated.MetricsResponse{
		Timestamp: time.Now(),
		Metrics:   metrics,
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// GetCoverageTrends retrieves coverage trends over time
func (a *AnalyticsAdapter) GetCoverageTrends(w http.ResponseWriter, r *http.Request, params generated.GetCoverageTrendsParams) {
	ctx := r.Context()

	// Determine time range and granularity
	timeRange := a.getTimeRangeFromParams(params.TimeRange)
	granularity := generated.CoverageTrendsResponseGranularityHour
	if params.Granularity != nil {
		granularity = generated.CoverageTrendsResponseGranularity(*params.Granularity)
	}

	// Generate coverage trends data
	dataPoints := a.getCoverageTrendsData(ctx, timeRange, granularity, params.CampaignId)
	summary := a.getCoverageTrendsSummary(ctx, dataPoints)

	response := generated.CoverageTrendsResponse{
		CampaignId:  params.CampaignId,
		DataPoints:  dataPoints,
		Granularity: granularity,
		Summary:     summary,
		TimeRange: struct {
			End   *time.Time `json:"end,omitempty"`
			Start *time.Time `json:"start,omitempty"`
		}{
			Start: &timeRange.Start,
			End:   &timeRange.End,
		},
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// GetPerformanceStats retrieves performance statistics
func (a *AnalyticsAdapter) GetPerformanceStats(w http.ResponseWriter, r *http.Request, params generated.GetPerformanceStatsParams) {
	ctx := r.Context()

	timeRange := a.getTimeRangeFromPerformanceParams(params.TimeRange)

	// Get component statistics
	componentStats := a.getComponentStats(ctx, timeRange, params.Component)

	// Get bottlenecks
	bottlenecks := a.getBottlenecks(ctx, timeRange, params.Component)

	// Get optimization suggestions
	optimizationSuggestions := a.getOptimizationSuggestions(ctx, componentStats)

	response := generated.PerformanceStatsResponse{
		ComponentStats:          componentStats,
		Bottlenecks:             bottlenecks,
		OptimizationSuggestions: optimizationSuggestions,
		TimeRange: struct {
			End   *time.Time `json:"end,omitempty"`
			Start *time.Time `json:"start,omitempty"`
		}{
			Start: &timeRange.Start,
			End:   &timeRange.End,
		},
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// Helper methods

type TimeRange struct {
	Start    time.Time
	End      time.Time
	Duration string
}

func (a *AnalyticsAdapter) getTimeRange(timeRange *generated.GetAnalyticsParamsTimeRange) TimeRange {
	now := time.Now()

	if timeRange == nil {
		// Default to 24h
		return TimeRange{
			Start:    now.Add(-24 * time.Hour),
			End:      now,
			Duration: "24h",
		}
	}

	switch *timeRange {
	case generated.GetAnalyticsParamsTimeRangeN1h:
		return TimeRange{Start: now.Add(-time.Hour), End: now, Duration: "1h"}
	case generated.GetAnalyticsParamsTimeRangeN6h:
		return TimeRange{Start: now.Add(-6 * time.Hour), End: now, Duration: "6h"}
	case generated.GetAnalyticsParamsTimeRangeN24h:
		return TimeRange{Start: now.Add(-24 * time.Hour), End: now, Duration: "24h"}
	case generated.GetAnalyticsParamsTimeRangeN7d:
		return TimeRange{Start: now.Add(-7 * 24 * time.Hour), End: now, Duration: "7d"}
	case generated.GetAnalyticsParamsTimeRangeN30d:
		return TimeRange{Start: now.Add(-30 * 24 * time.Hour), End: now, Duration: "30d"}
	default:
		return TimeRange{Start: now.Add(-24 * time.Hour), End: now, Duration: "24h"}
	}
}

func (a *AnalyticsAdapter) getTimeRangeFromParams(timeRange *generated.GetCoverageTrendsParamsTimeRange) TimeRange {
	now := time.Now()

	if timeRange == nil {
		return TimeRange{Start: now.Add(-24 * time.Hour), End: now, Duration: "24h"}
	}

	switch *timeRange {
	case generated.GetCoverageTrendsParamsTimeRangeN1h:
		return TimeRange{Start: now.Add(-time.Hour), End: now, Duration: "1h"}
	case generated.GetCoverageTrendsParamsTimeRangeN6h:
		return TimeRange{Start: now.Add(-6 * time.Hour), End: now, Duration: "6h"}
	case generated.GetCoverageTrendsParamsTimeRangeN24h:
		return TimeRange{Start: now.Add(-24 * time.Hour), End: now, Duration: "24h"}
	case generated.GetCoverageTrendsParamsTimeRangeN7d:
		return TimeRange{Start: now.Add(-7 * 24 * time.Hour), End: now, Duration: "7d"}
	case generated.GetCoverageTrendsParamsTimeRangeN30d:
		return TimeRange{Start: now.Add(-30 * 24 * time.Hour), End: now, Duration: "30d"}
	default:
		return TimeRange{Start: now.Add(-24 * time.Hour), End: now, Duration: "24h"}
	}
}

func (a *AnalyticsAdapter) getTimeRangeFromPerformanceParams(timeRange *generated.GetPerformanceStatsParamsTimeRange) TimeRange {
	now := time.Now()

	if timeRange == nil {
		return TimeRange{Start: now.Add(-24 * time.Hour), End: now, Duration: "24h"}
	}

	switch *timeRange {
	case generated.N1h:
		return TimeRange{Start: now.Add(-time.Hour), End: now, Duration: "1h"}
	case generated.N6h:
		return TimeRange{Start: now.Add(-6 * time.Hour), End: now, Duration: "6h"}
	case generated.N24h:
		return TimeRange{Start: now.Add(-24 * time.Hour), End: now, Duration: "24h"}
	case generated.N7d:
		return TimeRange{Start: now.Add(-7 * 24 * time.Hour), End: now, Duration: "7d"}
	case generated.N30d:
		return TimeRange{Start: now.Add(-30 * 24 * time.Hour), End: now, Duration: "30d"}
	default:
		return TimeRange{Start: now.Add(-24 * time.Hour), End: now, Duration: "24h"}
	}
}

func (a *AnalyticsAdapter) getSystemOverview(ctx context.Context, campaignId *uuid.UUID) struct {
	ActiveCampaigns    *int `json:"active_campaigns,omitempty"`
	ActiveJobs         *int `json:"active_jobs,omitempty"`
	OnlineBots         *int `json:"online_bots,omitempty"`
	TotalBots          *int `json:"total_bots,omitempty"`
	TotalCampaigns     *int `json:"total_campaigns,omitempty"`
	TotalCorpusEntries *int `json:"total_corpus_entries,omitempty"`
	TotalCoverageEdges *int `json:"total_coverage_edges,omitempty"`
	TotalCrashes       *int `json:"total_crashes,omitempty"`
	TotalJobs          *int `json:"total_jobs,omitempty"`
	UniqueCrashes      *int `json:"unique_crashes,omitempty"`
} {
	// Mock implementation - in reality, this would query repositories
	return struct {
		ActiveCampaigns    *int `json:"active_campaigns,omitempty"`
		ActiveJobs         *int `json:"active_jobs,omitempty"`
		OnlineBots         *int `json:"online_bots,omitempty"`
		TotalBots          *int `json:"total_bots,omitempty"`
		TotalCampaigns     *int `json:"total_campaigns,omitempty"`
		TotalCorpusEntries *int `json:"total_corpus_entries,omitempty"`
		TotalCoverageEdges *int `json:"total_coverage_edges,omitempty"`
		TotalCrashes       *int `json:"total_crashes,omitempty"`
		TotalJobs          *int `json:"total_jobs,omitempty"`
		UniqueCrashes      *int `json:"unique_crashes,omitempty"`
	}{
		TotalCampaigns:     &[]int{5}[0],
		ActiveCampaigns:    &[]int{2}[0],
		TotalJobs:          &[]int{150}[0],
		ActiveJobs:         &[]int{12}[0],
		TotalBots:          &[]int{8}[0],
		OnlineBots:         &[]int{6}[0],
		TotalCorpusEntries: &[]int{2500}[0],
		TotalCoverageEdges: &[]int{15000}[0],
		TotalCrashes:       &[]int{45}[0],
		UniqueCrashes:      &[]int{32}[0],
	}
}

func (a *AnalyticsAdapter) getPerformanceMetrics(ctx context.Context, timeRange TimeRange, campaignId *uuid.UUID) *struct {
	AvgCoveragePerHour          *float32 `json:"avg_coverage_per_hour,omitempty"`
	AvgExecutionsPerSecond      *float32 `json:"avg_executions_per_second,omitempty"`
	AvgJobCompletionTimeSeconds *float32 `json:"avg_job_completion_time_seconds,omitempty"`
	CrashDiscoveryRatePerHour   *float32 `json:"crash_discovery_rate_per_hour,omitempty"`
	SystemEfficiencyScore       *float32 `json:"system_efficiency_score,omitempty"`
} {
	// Mock implementation
	return &struct {
		AvgCoveragePerHour          *float32 `json:"avg_coverage_per_hour,omitempty"`
		AvgExecutionsPerSecond      *float32 `json:"avg_executions_per_second,omitempty"`
		AvgJobCompletionTimeSeconds *float32 `json:"avg_job_completion_time_seconds,omitempty"`
		CrashDiscoveryRatePerHour   *float32 `json:"crash_discovery_rate_per_hour,omitempty"`
		SystemEfficiencyScore       *float32 `json:"system_efficiency_score,omitempty"`
	}{
		AvgCoveragePerHour:          &[]float32{125.5}[0],
		AvgExecutionsPerSecond:      &[]float32{850.2}[0],
		AvgJobCompletionTimeSeconds: &[]float32{3600.0}[0],
		CrashDiscoveryRatePerHour:   &[]float32{2.3}[0],
		SystemEfficiencyScore:       &[]float32{87.5}[0],
	}
}

func (a *AnalyticsAdapter) getResourceUsage(ctx context.Context) *struct {
	CpuUtilizationPercent *float32 `json:"cpu_utilization_percent,omitempty"`
	MemoryUsageBytes      *int     `json:"memory_usage_bytes,omitempty"`
	NetworkThroughputBps  *int     `json:"network_throughput_bps,omitempty"`
	StorageUsageBytes     *int     `json:"storage_usage_bytes,omitempty"`
} {
	// Mock implementation
	return &struct {
		CpuUtilizationPercent *float32 `json:"cpu_utilization_percent,omitempty"`
		MemoryUsageBytes      *int     `json:"memory_usage_bytes,omitempty"`
		NetworkThroughputBps  *int     `json:"network_throughput_bps,omitempty"`
		StorageUsageBytes     *int     `json:"storage_usage_bytes,omitempty"`
	}{
		CpuUtilizationPercent: &[]float32{65.2}[0],
		MemoryUsageBytes:      &[]int{1024 * 1024 * 1024 * 2}[0],  // 2GB
		NetworkThroughputBps:  &[]int{1024 * 1024 * 10}[0],        // 10MB/s
		StorageUsageBytes:     &[]int{1024 * 1024 * 1024 * 50}[0], // 50GB
	}
}

func (a *AnalyticsAdapter) getTrends(ctx context.Context, timeRange TimeRange, campaignId *uuid.UUID) *struct {
	CoverageGrowthTrend *[]struct {
		Timestamp     *time.Time `json:"timestamp,omitempty"`
		TotalCoverage *int       `json:"total_coverage,omitempty"`
	} `json:"coverage_growth_trend,omitempty"`
	CrashDiscoveryTrend *[]struct {
		NewCrashes *int       `json:"new_crashes,omitempty"`
		Timestamp  *time.Time `json:"timestamp,omitempty"`
	} `json:"crash_discovery_trend,omitempty"`
	JobCompletionTrend *[]struct {
		CompletedJobs *int       `json:"completed_jobs,omitempty"`
		Timestamp     *time.Time `json:"timestamp,omitempty"`
	} `json:"job_completion_trend,omitempty"`
} {
	// Mock implementation
	now := time.Now()
	coverageGrowth := []struct {
		Timestamp     *time.Time `json:"timestamp,omitempty"`
		TotalCoverage *int       `json:"total_coverage,omitempty"`
	}{
		{Timestamp: &[]time.Time{now.Add(-2 * time.Hour)}[0], TotalCoverage: &[]int{12000}[0]},
		{Timestamp: &[]time.Time{now.Add(-time.Hour)}[0], TotalCoverage: &[]int{13500}[0]},
		{Timestamp: &[]time.Time{now}[0], TotalCoverage: &[]int{15000}[0]},
	}

	crashDiscovery := []struct {
		NewCrashes *int       `json:"new_crashes,omitempty"`
		Timestamp  *time.Time `json:"timestamp,omitempty"`
	}{
		{Timestamp: &[]time.Time{now.Add(-2 * time.Hour)}[0], NewCrashes: &[]int{3}[0]},
		{Timestamp: &[]time.Time{now.Add(-time.Hour)}[0], NewCrashes: &[]int{2}[0]},
		{Timestamp: &[]time.Time{now}[0], NewCrashes: &[]int{4}[0]},
	}

	jobCompletion := []struct {
		CompletedJobs *int       `json:"completed_jobs,omitempty"`
		Timestamp     *time.Time `json:"timestamp,omitempty"`
	}{
		{Timestamp: &[]time.Time{now.Add(-2 * time.Hour)}[0], CompletedJobs: &[]int{15}[0]},
		{Timestamp: &[]time.Time{now.Add(-time.Hour)}[0], CompletedJobs: &[]int{18}[0]},
		{Timestamp: &[]time.Time{now}[0], CompletedJobs: &[]int{22}[0]},
	}

	return &struct {
		CoverageGrowthTrend *[]struct {
			Timestamp     *time.Time `json:"timestamp,omitempty"`
			TotalCoverage *int       `json:"total_coverage,omitempty"`
		} `json:"coverage_growth_trend,omitempty"`
		CrashDiscoveryTrend *[]struct {
			NewCrashes *int       `json:"new_crashes,omitempty"`
			Timestamp  *time.Time `json:"timestamp,omitempty"`
		} `json:"crash_discovery_trend,omitempty"`
		JobCompletionTrend *[]struct {
			CompletedJobs *int       `json:"completed_jobs,omitempty"`
			Timestamp     *time.Time `json:"timestamp,omitempty"`
		} `json:"job_completion_trend,omitempty"`
	}{
		CoverageGrowthTrend: &coverageGrowth,
		CrashDiscoveryTrend: &crashDiscovery,
		JobCompletionTrend:  &jobCompletion,
	}
}

func (a *AnalyticsAdapter) getRealTimeMetrics(ctx context.Context) struct {
	Bots *struct {
		Busy   *int `json:"busy,omitempty"`
		Error  *int `json:"error,omitempty"`
		Idle   *int `json:"idle,omitempty"`
		Online *int `json:"online,omitempty"`
		Total  *int `json:"total,omitempty"`
	} `json:"bots,omitempty"`
	Campaigns *struct {
		Active    *int `json:"active,omitempty"`
		Completed *int `json:"completed,omitempty"`
		Paused    *int `json:"paused,omitempty"`
		Total     *int `json:"total,omitempty"`
	} `json:"campaigns,omitempty"`
	Coverage *struct {
		EdgesPerSecond *float32 `json:"edges_per_second,omitempty"`
		GrowthRate     *float32 `json:"growth_rate,omitempty"`
		TotalEdges     *int     `json:"total_edges,omitempty"`
	} `json:"coverage,omitempty"`
	Crashes *struct {
		Critical *int `json:"critical,omitempty"`
		Today    *int `json:"today,omitempty"`
		Total    *int `json:"total,omitempty"`
		Unique   *int `json:"unique,omitempty"`
	} `json:"crashes,omitempty"`
	Jobs *struct {
		Completed *int `json:"completed,omitempty"`
		Failed    *int `json:"failed,omitempty"`
		Pending   *int `json:"pending,omitempty"`
		Running   *int `json:"running,omitempty"`
		Total     *int `json:"total,omitempty"`
	} `json:"jobs,omitempty"`
	System *struct {
		CpuUsagePercent      *float32 `json:"cpu_usage_percent,omitempty"`
		DiskUsageBytes       *int     `json:"disk_usage_bytes,omitempty"`
		ErrorRatePerSecond   *float32 `json:"error_rate_per_second,omitempty"`
		MemoryUsageBytes     *int     `json:"memory_usage_bytes,omitempty"`
		RequestRatePerSecond *float32 `json:"request_rate_per_second,omitempty"`
		UptimeSeconds        *int     `json:"uptime_seconds,omitempty"`
	} `json:"system,omitempty"`
} {
	// Mock implementation
	return struct {
		Bots *struct {
			Busy   *int `json:"busy,omitempty"`
			Error  *int `json:"error,omitempty"`
			Idle   *int `json:"idle,omitempty"`
			Online *int `json:"online,omitempty"`
			Total  *int `json:"total,omitempty"`
		} `json:"bots,omitempty"`
		Campaigns *struct {
			Active    *int `json:"active,omitempty"`
			Completed *int `json:"completed,omitempty"`
			Paused    *int `json:"paused,omitempty"`
			Total     *int `json:"total,omitempty"`
		} `json:"campaigns,omitempty"`
		Coverage *struct {
			EdgesPerSecond *float32 `json:"edges_per_second,omitempty"`
			GrowthRate     *float32 `json:"growth_rate,omitempty"`
			TotalEdges     *int     `json:"total_edges,omitempty"`
		} `json:"coverage,omitempty"`
		Crashes *struct {
			Critical *int `json:"critical,omitempty"`
			Today    *int `json:"today,omitempty"`
			Total    *int `json:"total,omitempty"`
			Unique   *int `json:"unique,omitempty"`
		} `json:"crashes,omitempty"`
		Jobs *struct {
			Completed *int `json:"completed,omitempty"`
			Failed    *int `json:"failed,omitempty"`
			Pending   *int `json:"pending,omitempty"`
			Running   *int `json:"running,omitempty"`
			Total     *int `json:"total,omitempty"`
		} `json:"jobs,omitempty"`
		System *struct {
			CpuUsagePercent      *float32 `json:"cpu_usage_percent,omitempty"`
			DiskUsageBytes       *int     `json:"disk_usage_bytes,omitempty"`
			ErrorRatePerSecond   *float32 `json:"error_rate_per_second,omitempty"`
			MemoryUsageBytes     *int     `json:"memory_usage_bytes,omitempty"`
			RequestRatePerSecond *float32 `json:"request_rate_per_second,omitempty"`
			UptimeSeconds        *int     `json:"uptime_seconds,omitempty"`
		} `json:"system,omitempty"`
	}{
		Bots: &struct {
			Busy   *int `json:"busy,omitempty"`
			Error  *int `json:"error,omitempty"`
			Idle   *int `json:"idle,omitempty"`
			Online *int `json:"online,omitempty"`
			Total  *int `json:"total,omitempty"`
		}{
			Total:  &[]int{8}[0],
			Online: &[]int{6}[0],
			Idle:   &[]int{3}[0],
			Busy:   &[]int{2}[0],
			Error:  &[]int{1}[0],
		},
		Campaigns: &struct {
			Active    *int `json:"active,omitempty"`
			Completed *int `json:"completed,omitempty"`
			Paused    *int `json:"paused,omitempty"`
			Total     *int `json:"total,omitempty"`
		}{
			Total:     &[]int{5}[0],
			Active:    &[]int{2}[0],
			Paused:    &[]int{1}[0],
			Completed: &[]int{2}[0],
		},
		Jobs: &struct {
			Completed *int `json:"completed,omitempty"`
			Failed    *int `json:"failed,omitempty"`
			Pending   *int `json:"pending,omitempty"`
			Running   *int `json:"running,omitempty"`
			Total     *int `json:"total,omitempty"`
		}{
			Total:     &[]int{150}[0],
			Running:   &[]int{12}[0],
			Pending:   &[]int{8}[0],
			Completed: &[]int{125}[0],
			Failed:    &[]int{5}[0],
		},
		Coverage: &struct {
			EdgesPerSecond *float32 `json:"edges_per_second,omitempty"`
			GrowthRate     *float32 `json:"growth_rate,omitempty"`
			TotalEdges     *int     `json:"total_edges,omitempty"`
		}{
			TotalEdges:     &[]int{15000}[0],
			EdgesPerSecond: &[]float32{12.5}[0],
			GrowthRate:     &[]float32{125.5}[0],
		},
		Crashes: &struct {
			Critical *int `json:"critical,omitempty"`
			Today    *int `json:"today,omitempty"`
			Total    *int `json:"total,omitempty"`
			Unique   *int `json:"unique,omitempty"`
		}{
			Total:    &[]int{45}[0],
			Unique:   &[]int{32}[0],
			Critical: &[]int{5}[0],
			Today:    &[]int{3}[0],
		},
		System: &struct {
			CpuUsagePercent      *float32 `json:"cpu_usage_percent,omitempty"`
			DiskUsageBytes       *int     `json:"disk_usage_bytes,omitempty"`
			ErrorRatePerSecond   *float32 `json:"error_rate_per_second,omitempty"`
			MemoryUsageBytes     *int     `json:"memory_usage_bytes,omitempty"`
			RequestRatePerSecond *float32 `json:"request_rate_per_second,omitempty"`
			UptimeSeconds        *int     `json:"uptime_seconds,omitempty"`
		}{
			CpuUsagePercent:      &[]float32{65.2}[0],
			MemoryUsageBytes:     &[]int{2 * 1024 * 1024 * 1024}[0],  // 2GB
			DiskUsageBytes:       &[]int{50 * 1024 * 1024 * 1024}[0], // 50GB
			RequestRatePerSecond: &[]float32{25.3}[0],
			ErrorRatePerSecond:   &[]float32{0.1}[0],
			UptimeSeconds:        &[]int{3600 * 24 * 7}[0], // 1 week
		},
	}
}

func (a *AnalyticsAdapter) getCoverageTrendsData(ctx context.Context, timeRange TimeRange, granularity generated.CoverageTrendsResponseGranularity, campaignId *uuid.UUID) []struct {
	CoverageDensity *float32   `json:"coverage_density,omitempty"`
	CumulativeEdges *int       `json:"cumulative_edges,omitempty"`
	ExecutionCount  *int       `json:"execution_count,omitempty"`
	NewEdges        *int       `json:"new_edges,omitempty"`
	Timestamp       *time.Time `json:"timestamp,omitempty"`
	TotalEdges      *int       `json:"total_edges,omitempty"`
} {
	// Mock implementation
	now := time.Now()
	return []struct {
		CoverageDensity *float32   `json:"coverage_density,omitempty"`
		CumulativeEdges *int       `json:"cumulative_edges,omitempty"`
		ExecutionCount  *int       `json:"execution_count,omitempty"`
		NewEdges        *int       `json:"new_edges,omitempty"`
		Timestamp       *time.Time `json:"timestamp,omitempty"`
		TotalEdges      *int       `json:"total_edges,omitempty"`
	}{
		{
			Timestamp:       &[]time.Time{now.Add(-2 * time.Hour)}[0],
			TotalEdges:      &[]int{12000}[0],
			NewEdges:        &[]int{150}[0],
			CumulativeEdges: &[]int{12000}[0],
			ExecutionCount:  &[]int{50000}[0],
			CoverageDensity: &[]float32{0.24}[0],
		},
		{
			Timestamp:       &[]time.Time{now.Add(-time.Hour)}[0],
			TotalEdges:      &[]int{13500}[0],
			NewEdges:        &[]int{1500}[0],
			CumulativeEdges: &[]int{13500}[0],
			ExecutionCount:  &[]int{75000}[0],
			CoverageDensity: &[]float32{0.18}[0],
		},
		{
			Timestamp:       &[]time.Time{now}[0],
			TotalEdges:      &[]int{15000}[0],
			NewEdges:        &[]int{1500}[0],
			CumulativeEdges: &[]int{15000}[0],
			ExecutionCount:  &[]int{100000}[0],
			CoverageDensity: &[]float32{0.15}[0],
		},
	}
}

func (a *AnalyticsAdapter) getCoverageTrendsSummary(ctx context.Context, dataPoints []struct {
	CoverageDensity *float32   `json:"coverage_density,omitempty"`
	CumulativeEdges *int       `json:"cumulative_edges,omitempty"`
	ExecutionCount  *int       `json:"execution_count,omitempty"`
	NewEdges        *int       `json:"new_edges,omitempty"`
	Timestamp       *time.Time `json:"timestamp,omitempty"`
	TotalEdges      *int       `json:"total_edges,omitempty"`
}) *struct {
	EfficiencyScore   *float32   `json:"efficiency_score,omitempty"`
	GrowthRate        *float32   `json:"growth_rate,omitempty"`
	PeakDiscoveryTime *time.Time `json:"peak_discovery_time,omitempty"`
	TotalGrowth       *int       `json:"total_growth,omitempty"`
} {
	// Mock implementation
	return &struct {
		EfficiencyScore   *float32   `json:"efficiency_score,omitempty"`
		GrowthRate        *float32   `json:"growth_rate,omitempty"`
		PeakDiscoveryTime *time.Time `json:"peak_discovery_time,omitempty"`
		TotalGrowth       *int       `json:"total_growth,omitempty"`
	}{
		TotalGrowth:       &[]int{3000}[0],
		GrowthRate:        &[]float32{1500.0}[0],
		EfficiencyScore:   &[]float32{82.5}[0],
		PeakDiscoveryTime: &[]time.Time{time.Now().Add(-time.Hour)}[0],
	}
}

func (a *AnalyticsAdapter) getComponentStats(ctx context.Context, timeRange TimeRange, component *generated.GetPerformanceStatsParamsComponent) struct {
	Bots *struct {
		AvgJobCompletionTimeSeconds *float32 `json:"avg_job_completion_time_seconds,omitempty"`
		AvgUtilizationPercent       *float32 `json:"avg_utilization_percent,omitempty"`
		FailureRatePercent          *float32 `json:"failure_rate_percent,omitempty"`
		ThroughputJobsPerHour       *float32 `json:"throughput_jobs_per_hour,omitempty"`
	} `json:"bots,omitempty"`
	Database *struct {
		AvgQueryTimeMs                   *float32 `json:"avg_query_time_ms,omitempty"`
		ConnectionPoolUtilizationPercent *float32 `json:"connection_pool_utilization_percent,omitempty"`
		DeadlockCount                    *int     `json:"deadlock_count,omitempty"`
		SlowQueryCount                   *int     `json:"slow_query_count,omitempty"`
	} `json:"database,omitempty"`
	Jobs *struct {
		AvgExecutionTimeSeconds *float32 `json:"avg_execution_time_seconds,omitempty"`
		AvgQueueTimeSeconds     *float32 `json:"avg_queue_time_seconds,omitempty"`
		SuccessRatePercent      *float32 `json:"success_rate_percent,omitempty"`
		TimeoutRatePercent      *float32 `json:"timeout_rate_percent,omitempty"`
	} `json:"jobs,omitempty"`
	Storage *struct {
		AvgReadLatencyMs  *float32 `json:"avg_read_latency_ms,omitempty"`
		AvgWriteLatencyMs *float32 `json:"avg_write_latency_ms,omitempty"`
		ErrorRatePercent  *float32 `json:"error_rate_percent,omitempty"`
		ThroughputMbps    *float32 `json:"throughput_mbps,omitempty"`
	} `json:"storage,omitempty"`
} {
	// Mock implementation
	return struct {
		Bots *struct {
			AvgJobCompletionTimeSeconds *float32 `json:"avg_job_completion_time_seconds,omitempty"`
			AvgUtilizationPercent       *float32 `json:"avg_utilization_percent,omitempty"`
			FailureRatePercent          *float32 `json:"failure_rate_percent,omitempty"`
			ThroughputJobsPerHour       *float32 `json:"throughput_jobs_per_hour,omitempty"`
		} `json:"bots,omitempty"`
		Database *struct {
			AvgQueryTimeMs                   *float32 `json:"avg_query_time_ms,omitempty"`
			ConnectionPoolUtilizationPercent *float32 `json:"connection_pool_utilization_percent,omitempty"`
			DeadlockCount                    *int     `json:"deadlock_count,omitempty"`
			SlowQueryCount                   *int     `json:"slow_query_count,omitempty"`
		} `json:"database,omitempty"`
		Jobs *struct {
			AvgExecutionTimeSeconds *float32 `json:"avg_execution_time_seconds,omitempty"`
			AvgQueueTimeSeconds     *float32 `json:"avg_queue_time_seconds,omitempty"`
			SuccessRatePercent      *float32 `json:"success_rate_percent,omitempty"`
			TimeoutRatePercent      *float32 `json:"timeout_rate_percent,omitempty"`
		} `json:"jobs,omitempty"`
		Storage *struct {
			AvgReadLatencyMs  *float32 `json:"avg_read_latency_ms,omitempty"`
			AvgWriteLatencyMs *float32 `json:"avg_write_latency_ms,omitempty"`
			ErrorRatePercent  *float32 `json:"error_rate_percent,omitempty"`
			ThroughputMbps    *float32 `json:"throughput_mbps,omitempty"`
		} `json:"storage,omitempty"`
	}{
		Bots: &struct {
			AvgJobCompletionTimeSeconds *float32 `json:"avg_job_completion_time_seconds,omitempty"`
			AvgUtilizationPercent       *float32 `json:"avg_utilization_percent,omitempty"`
			FailureRatePercent          *float32 `json:"failure_rate_percent,omitempty"`
			ThroughputJobsPerHour       *float32 `json:"throughput_jobs_per_hour,omitempty"`
		}{
			AvgUtilizationPercent:       &[]float32{75.5}[0],
			ThroughputJobsPerHour:       &[]float32{25.8}[0],
			AvgJobCompletionTimeSeconds: &[]float32{3600.0}[0],
			FailureRatePercent:          &[]float32{3.2}[0],
		},
		Jobs: &struct {
			AvgExecutionTimeSeconds *float32 `json:"avg_execution_time_seconds,omitempty"`
			AvgQueueTimeSeconds     *float32 `json:"avg_queue_time_seconds,omitempty"`
			SuccessRatePercent      *float32 `json:"success_rate_percent,omitempty"`
			TimeoutRatePercent      *float32 `json:"timeout_rate_percent,omitempty"`
		}{
			AvgExecutionTimeSeconds: &[]float32{3450.0}[0],
			AvgQueueTimeSeconds:     &[]float32{120.5}[0],
			SuccessRatePercent:      &[]float32{96.8}[0],
			TimeoutRatePercent:      &[]float32{1.5}[0],
		},
		Database: &struct {
			AvgQueryTimeMs                   *float32 `json:"avg_query_time_ms,omitempty"`
			ConnectionPoolUtilizationPercent *float32 `json:"connection_pool_utilization_percent,omitempty"`
			DeadlockCount                    *int     `json:"deadlock_count,omitempty"`
			SlowQueryCount                   *int     `json:"slow_query_count,omitempty"`
		}{
			AvgQueryTimeMs:                   &[]float32{15.2}[0],
			ConnectionPoolUtilizationPercent: &[]float32{45.0}[0],
			SlowQueryCount:                   &[]int{3}[0],
			DeadlockCount:                    &[]int{0}[0],
		},
		Storage: &struct {
			AvgReadLatencyMs  *float32 `json:"avg_read_latency_ms,omitempty"`
			AvgWriteLatencyMs *float32 `json:"avg_write_latency_ms,omitempty"`
			ErrorRatePercent  *float32 `json:"error_rate_percent,omitempty"`
			ThroughputMbps    *float32 `json:"throughput_mbps,omitempty"`
		}{
			AvgReadLatencyMs:  &[]float32{8.5}[0],
			AvgWriteLatencyMs: &[]float32{12.3}[0],
			ThroughputMbps:    &[]float32{125.7}[0],
			ErrorRatePercent:  &[]float32{0.1}[0],
		},
	}
}

func (a *AnalyticsAdapter) getBottlenecks(ctx context.Context, timeRange TimeRange, component *generated.GetPerformanceStatsParamsComponent) *[]struct {
	Component      *string                                                `json:"component,omitempty"`
	Impact         *string                                                `json:"impact,omitempty"`
	Issue          *string                                                `json:"issue,omitempty"`
	Recommendation *string                                                `json:"recommendation,omitempty"`
	Severity       *generated.PerformanceStatsResponseBottlenecksSeverity `json:"severity,omitempty"`
} {
	// Mock implementation
	bottlenecks := []struct {
		Component      *string                                                `json:"component,omitempty"`
		Impact         *string                                                `json:"impact,omitempty"`
		Issue          *string                                                `json:"issue,omitempty"`
		Recommendation *string                                                `json:"recommendation,omitempty"`
		Severity       *generated.PerformanceStatsResponseBottlenecksSeverity `json:"severity,omitempty"`
	}{
		{
			Component:      &[]string{"jobs"}[0],
			Issue:          &[]string{"High queue time"}[0],
			Impact:         &[]string{"Job processing delays"}[0],
			Recommendation: &[]string{"Increase bot capacity or optimize job scheduling"}[0],
			Severity:       &[]generated.PerformanceStatsResponseBottlenecksSeverity{generated.PerformanceStatsResponseBottlenecksSeverityMedium}[0],
		},
	}

	return &bottlenecks
}

func (a *AnalyticsAdapter) getOptimizationSuggestions(ctx context.Context, componentStats struct {
	Bots *struct {
		AvgJobCompletionTimeSeconds *float32 `json:"avg_job_completion_time_seconds,omitempty"`
		AvgUtilizationPercent       *float32 `json:"avg_utilization_percent,omitempty"`
		FailureRatePercent          *float32 `json:"failure_rate_percent,omitempty"`
		ThroughputJobsPerHour       *float32 `json:"throughput_jobs_per_hour,omitempty"`
	} `json:"bots,omitempty"`
	Database *struct {
		AvgQueryTimeMs                   *float32 `json:"avg_query_time_ms,omitempty"`
		ConnectionPoolUtilizationPercent *float32 `json:"connection_pool_utilization_percent,omitempty"`
		DeadlockCount                    *int     `json:"deadlock_count,omitempty"`
		SlowQueryCount                   *int     `json:"slow_query_count,omitempty"`
	} `json:"database,omitempty"`
	Jobs *struct {
		AvgExecutionTimeSeconds *float32 `json:"avg_execution_time_seconds,omitempty"`
		AvgQueueTimeSeconds     *float32 `json:"avg_queue_time_seconds,omitempty"`
		SuccessRatePercent      *float32 `json:"success_rate_percent,omitempty"`
		TimeoutRatePercent      *float32 `json:"timeout_rate_percent,omitempty"`
	} `json:"jobs,omitempty"`
	Storage *struct {
		AvgReadLatencyMs  *float32 `json:"avg_read_latency_ms,omitempty"`
		AvgWriteLatencyMs *float32 `json:"avg_write_latency_ms,omitempty"`
		ErrorRatePercent  *float32 `json:"error_rate_percent,omitempty"`
		ThroughputMbps    *float32 `json:"throughput_mbps,omitempty"`
	} `json:"storage,omitempty"`
}) *[]struct {
	Category                    *string                                                               `json:"category,omitempty"`
	EffortLevel                 *generated.PerformanceStatsResponseOptimizationSuggestionsEffortLevel `json:"effort_level,omitempty"`
	EstimatedImprovementPercent *float32                                                              `json:"estimated_improvement_percent,omitempty"`
	Suggestion                  *string                                                               `json:"suggestion,omitempty"`
} {
	// Mock implementation
	suggestions := []struct {
		Category                    *string                                                               `json:"category,omitempty"`
		EffortLevel                 *generated.PerformanceStatsResponseOptimizationSuggestionsEffortLevel `json:"effort_level,omitempty"`
		EstimatedImprovementPercent *float32                                                              `json:"estimated_improvement_percent,omitempty"`
		Suggestion                  *string                                                               `json:"suggestion,omitempty"`
	}{
		{
			Category:                    &[]string{"capacity"}[0],
			Suggestion:                  &[]string{"Add 2 more bot instances to reduce queue times"}[0],
			EffortLevel:                 &[]generated.PerformanceStatsResponseOptimizationSuggestionsEffortLevel{generated.Low}[0],
			EstimatedImprovementPercent: &[]float32{15.5}[0],
		},
		{
			Category:                    &[]string{"database"}[0],
			Suggestion:                  &[]string{"Optimize slow queries identified in recent analysis"}[0],
			EffortLevel:                 &[]generated.PerformanceStatsResponseOptimizationSuggestionsEffortLevel{generated.Medium}[0],
			EstimatedImprovementPercent: &[]float32{8.2}[0],
		},
	}

	return &suggestions
}

func (a *AnalyticsAdapter) writeJSONResponse(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		a.logger.WithError(err).Error("failed to encode JSON response")
	}
}

func (a *AnalyticsAdapter) writeError(w http.ResponseWriter, statusCode int, errorType, title string, err error) {
	problem := generated.ProblemDetails{
		Type:      fmt.Sprintf("/errors/%s", strings.ToLower(errorType)),
		Title:     title,
		Status:    statusCode,
		Timestamp: &[]time.Time{time.Now()}[0],
	}

	if err != nil {
		detail := err.Error()
		problem.Detail = &detail
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)
	if encodeErr := json.NewEncoder(w).Encode(problem); encodeErr != nil {
		a.logger.WithError(encodeErr).Error("failed to encode error response")
	}
}
