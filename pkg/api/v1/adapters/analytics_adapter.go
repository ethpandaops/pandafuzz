package adapters

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/sse"
	campaignRepo "github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	crashRepo "github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	"github.com/ethpandaops/pandafuzz/pkg/service"
)

// AnalyticsAdapter handles analytics-related API requests
type AnalyticsAdapter struct {
	jobRepo      repository.JobRepository
	crashRepo    crashRepo.CrashRepository
	campaignRepo campaignRepo.CampaignRepository
	analytics    service.AnalyticsService
	sse          *sse.Manager
	logger       logrus.FieldLogger
}

// NewAnalyticsAdapter creates a new analytics adapter
func NewAnalyticsAdapter(
	jobRepo repository.JobRepository,
	crashRepo crashRepo.CrashRepository,
	campaignRepo campaignRepo.CampaignRepository,
	analytics service.AnalyticsService,
	sse *sse.Manager,
	logger logrus.FieldLogger,
) *AnalyticsAdapter {
	return &AnalyticsAdapter{
		jobRepo:      jobRepo,
		crashRepo:    crashRepo,
		campaignRepo: campaignRepo,
		analytics:    analytics,
		sse:          sse,
		logger:       logger.WithField("adapter", "analytics"),
	}
}

// GetAnalytics returns analytics summary
func (a *AnalyticsAdapter) GetAnalytics(w http.ResponseWriter, r *http.Request, params generated.GetAnalyticsParams) {
	a.logger.Debug("getting analytics")

	now := time.Now()
	startTime := now.Add(-24 * time.Hour)
	endTime := now
	duration := "24h"

	// Mock implementation - replace with actual service calls
	response := generated.AnalyticsResponse{
		GeneratedAt: now,
		TimeRange: struct {
			Duration *string    `json:"duration,omitempty"`
			End      *time.Time `json:"end,omitempty"`
			Start    *time.Time `json:"start,omitempty"`
		}{
			Start:    &startTime,
			End:      &endTime,
			Duration: &duration,
		},
		SystemOverview: struct {
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
			TotalCampaigns:     &[]int{10}[0],
			ActiveCampaigns:    &[]int{3}[0],
			TotalJobs:          &[]int{100}[0],
			ActiveJobs:         &[]int{10}[0],
			TotalBots:          &[]int{20}[0],
			OnlineBots:         &[]int{18}[0],
			TotalCrashes:       &[]int{50}[0],
			UniqueCrashes:      &[]int{30}[0],
			TotalCorpusEntries: &[]int{5000}[0],
			TotalCoverageEdges: &[]int{50000}[0],
		},
		PerformanceMetrics: &struct {
			AvgCoveragePerHour          *float32 `json:"avg_coverage_per_hour,omitempty"`
			AvgExecutionsPerSecond      *float32 `json:"avg_executions_per_second,omitempty"`
			AvgJobCompletionTimeSeconds *float32 `json:"avg_job_completion_time_seconds,omitempty"`
			CrashDiscoveryRatePerHour   *float32 `json:"crash_discovery_rate_per_hour,omitempty"`
			SystemEfficiencyScore       *float32 `json:"system_efficiency_score,omitempty"`
		}{
			AvgExecutionsPerSecond:      &[]float32{10000}[0],
			AvgCoveragePerHour:          &[]float32{2.5}[0],
			AvgJobCompletionTimeSeconds: &[]float32{1800}[0],
			CrashDiscoveryRatePerHour:   &[]float32{0.5}[0],
			SystemEfficiencyScore:       &[]float32{85.5}[0],
		},
		ResourceUsage: &struct {
			CpuUtilizationPercent *float32 `json:"cpu_utilization_percent,omitempty"`
			MemoryUsageBytes      *int     `json:"memory_usage_bytes,omitempty"`
			NetworkThroughputBps  *int     `json:"network_throughput_bps,omitempty"`
			StorageUsageBytes     *int     `json:"storage_usage_bytes,omitempty"`
		}{
			CpuUtilizationPercent: &[]float32{75.5}[0],
			MemoryUsageBytes:      &[]int{2147483648}[0], // 2GB
			StorageUsageBytes:     &[]int{1879048192}[0], // ~1.75GB
			NetworkThroughputBps:  &[]int{104857600}[0],  // 100Mbps
		},
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// GetCoverageTrends returns coverage trends over time
func (a *AnalyticsAdapter) GetCoverageTrends(w http.ResponseWriter, r *http.Request, params generated.GetCoverageTrendsParams) {
	a.logger.Debug("getting coverage trends")

	now := time.Now()
	startTime := now.Add(-24 * time.Hour)
	endTime := now
	campaignID := openapi_types.UUID(uuid.New())

	// Mock implementation using CoverageTrendsResponse
	response := generated.CoverageTrendsResponse{
		CampaignId:  &campaignID,
		Granularity: generated.CoverageTrendsResponseGranularityHour,
		TimeRange: struct {
			End   *time.Time `json:"end,omitempty"`
			Start *time.Time `json:"start,omitempty"`
		}{
			Start: &startTime,
			End:   &endTime,
		},
		DataPoints: []struct {
			CoverageDensity *float32   `json:"coverage_density,omitempty"`
			CumulativeEdges *int       `json:"cumulative_edges,omitempty"`
			ExecutionCount  *int       `json:"execution_count,omitempty"`
			NewEdges        *int       `json:"new_edges,omitempty"`
			Timestamp       *time.Time `json:"timestamp,omitempty"`
			TotalEdges      *int       `json:"total_edges,omitempty"`
		}{
			{
				Timestamp:       &[]time.Time{now.Add(-24 * time.Hour)}[0],
				TotalEdges:      &[]int{45000}[0],
				NewEdges:        &[]int{1000}[0],
				CumulativeEdges: &[]int{45000}[0],
				ExecutionCount:  &[]int{1000000}[0],
				CoverageDensity: &[]float32{0.045}[0],
			},
			{
				Timestamp:       &[]time.Time{now.Add(-12 * time.Hour)}[0],
				TotalEdges:      &[]int{47000}[0],
				NewEdges:        &[]int{2000}[0],
				CumulativeEdges: &[]int{47000}[0],
				ExecutionCount:  &[]int{2000000}[0],
				CoverageDensity: &[]float32{0.0235}[0],
			},
			{
				Timestamp:       &[]time.Time{now}[0],
				TotalEdges:      &[]int{50000}[0],
				NewEdges:        &[]int{3000}[0],
				CumulativeEdges: &[]int{50000}[0],
				ExecutionCount:  &[]int{3000000}[0],
				CoverageDensity: &[]float32{0.0167}[0],
			},
		},
		Summary: &struct {
			EfficiencyScore   *float32   `json:"efficiency_score,omitempty"`
			GrowthRate        *float32   `json:"growth_rate,omitempty"`
			PeakDiscoveryTime *time.Time `json:"peak_discovery_time,omitempty"`
			TotalGrowth       *int       `json:"total_growth,omitempty"`
		}{
			TotalGrowth:       &[]int{5000}[0],
			GrowthRate:        &[]float32{208.33}[0], // edges per hour
			PeakDiscoveryTime: &[]time.Time{now.Add(-24 * time.Hour)}[0],
			EfficiencyScore:   &[]float32{0.75}[0],
		},
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// GetMetrics returns system metrics
func (a *AnalyticsAdapter) GetMetrics(w http.ResponseWriter, r *http.Request) {
	a.logger.Debug("getting metrics")

	// Mock implementation - in production, this would integrate with Prometheus or similar
	metrics := generated.MetricsResponse{
		Timestamp: time.Now(),
		Metrics: struct {
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
				Online: &[]int{18}[0],
				Total:  &[]int{20}[0],
				Idle:   &[]int{5}[0],
				Busy:   &[]int{13}[0],
				Error:  &[]int{0}[0],
			},
			Campaigns: &struct {
				Active    *int `json:"active,omitempty"`
				Completed *int `json:"completed,omitempty"`
				Paused    *int `json:"paused,omitempty"`
				Total     *int `json:"total,omitempty"`
			}{
				Total:     &[]int{10}[0],
				Active:    &[]int{3}[0],
				Completed: &[]int{7}[0],
				Paused:    &[]int{0}[0],
			},
			Coverage: &struct {
				EdgesPerSecond *float32 `json:"edges_per_second,omitempty"`
				GrowthRate     *float32 `json:"growth_rate,omitempty"`
				TotalEdges     *int     `json:"total_edges,omitempty"`
			}{
				TotalEdges:     &[]int{50000}[0],
				EdgesPerSecond: &[]float32{10.5}[0],
				GrowthRate:     &[]float32{2.5}[0],
			},
			Crashes: &struct {
				Critical *int `json:"critical,omitempty"`
				Today    *int `json:"today,omitempty"`
				Total    *int `json:"total,omitempty"`
				Unique   *int `json:"unique,omitempty"`
			}{
				Total:    &[]int{50}[0],
				Unique:   &[]int{30}[0],
				Critical: &[]int{5}[0],
				Today:    &[]int{2}[0],
			},
			Jobs: &struct {
				Completed *int `json:"completed,omitempty"`
				Failed    *int `json:"failed,omitempty"`
				Pending   *int `json:"pending,omitempty"`
				Running   *int `json:"running,omitempty"`
				Total     *int `json:"total,omitempty"`
			}{
				Total:     &[]int{100}[0],
				Completed: &[]int{85}[0],
				Running:   &[]int{10}[0],
				Pending:   &[]int{0}[0],
				Failed:    &[]int{5}[0],
			},
			System: &struct {
				CpuUsagePercent      *float32 `json:"cpu_usage_percent,omitempty"`
				DiskUsageBytes       *int     `json:"disk_usage_bytes,omitempty"`
				ErrorRatePerSecond   *float32 `json:"error_rate_per_second,omitempty"`
				MemoryUsageBytes     *int     `json:"memory_usage_bytes,omitempty"`
				RequestRatePerSecond *float32 `json:"request_rate_per_second,omitempty"`
				UptimeSeconds        *int     `json:"uptime_seconds,omitempty"`
			}{
				CpuUsagePercent:      &[]float32{75.5}[0],
				MemoryUsageBytes:     &[]int{2147483648}[0], // 2GB
				DiskUsageBytes:       &[]int{1879048192}[0], // ~1.75GB
				UptimeSeconds:        &[]int{86400}[0],      // 24 hours
				RequestRatePerSecond: &[]float32{100.5}[0],
				ErrorRatePerSecond:   &[]float32{0.1}[0],
			},
		},
	}

	a.writeJSONResponse(w, http.StatusOK, metrics)
}

// GetPerformanceStats returns performance statistics
func (a *AnalyticsAdapter) GetPerformanceStats(w http.ResponseWriter, r *http.Request, params generated.GetPerformanceStatsParams) {
	a.logger.Debug("getting performance stats")

	// Mock implementation
	stats := generated.PerformanceStatsResponse{
		TimeRange: struct {
			End   *time.Time `json:"end,omitempty"`
			Start *time.Time `json:"start,omitempty"`
		}{
			Start: &[]time.Time{time.Now().Add(-24 * time.Hour)}[0],
			End:   &[]time.Time{time.Now()}[0],
		},
		ComponentStats: struct {
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
				AvgJobCompletionTimeSeconds: &[]float32{1800}[0],
				AvgUtilizationPercent:       &[]float32{75.5}[0],
				FailureRatePercent:          &[]float32{5.0}[0],
				ThroughputJobsPerHour:       &[]float32{20.5}[0],
			},
			Database: &struct {
				AvgQueryTimeMs                   *float32 `json:"avg_query_time_ms,omitempty"`
				ConnectionPoolUtilizationPercent *float32 `json:"connection_pool_utilization_percent,omitempty"`
				DeadlockCount                    *int     `json:"deadlock_count,omitempty"`
				SlowQueryCount                   *int     `json:"slow_query_count,omitempty"`
			}{
				AvgQueryTimeMs:                   &[]float32{15.5}[0],
				ConnectionPoolUtilizationPercent: &[]float32{60.0}[0],
				DeadlockCount:                    &[]int{0}[0],
				SlowQueryCount:                   &[]int{5}[0],
			},
			Jobs: &struct {
				AvgExecutionTimeSeconds *float32 `json:"avg_execution_time_seconds,omitempty"`
				AvgQueueTimeSeconds     *float32 `json:"avg_queue_time_seconds,omitempty"`
				SuccessRatePercent      *float32 `json:"success_rate_percent,omitempty"`
				TimeoutRatePercent      *float32 `json:"timeout_rate_percent,omitempty"`
			}{
				AvgExecutionTimeSeconds: &[]float32{1800}[0],
				AvgQueueTimeSeconds:     &[]float32{30}[0],
				SuccessRatePercent:      &[]float32{95.0}[0],
				TimeoutRatePercent:      &[]float32{2.5}[0],
			},
			Storage: &struct {
				AvgReadLatencyMs  *float32 `json:"avg_read_latency_ms,omitempty"`
				AvgWriteLatencyMs *float32 `json:"avg_write_latency_ms,omitempty"`
				ErrorRatePercent  *float32 `json:"error_rate_percent,omitempty"`
				ThroughputMbps    *float32 `json:"throughput_mbps,omitempty"`
			}{
				AvgReadLatencyMs:  &[]float32{5.5}[0],
				AvgWriteLatencyMs: &[]float32{10.2}[0],
				ErrorRatePercent:  &[]float32{0.1}[0],
				ThroughputMbps:    &[]float32{150.7}[0],
			},
		},
		Bottlenecks: &[]struct {
			Component      *string                                                `json:"component,omitempty"`
			Impact         *string                                                `json:"impact,omitempty"`
			Issue          *string                                                `json:"issue,omitempty"`
			Recommendation *string                                                `json:"recommendation,omitempty"`
			Severity       *generated.PerformanceStatsResponseBottlenecksSeverity `json:"severity,omitempty"`
		}{
			{
				Component:      &[]string{"cpu"}[0],
				Issue:          &[]string{"CPU usage consistently above 70%"}[0],
				Impact:         &[]string{"Reduced fuzzing throughput"}[0],
				Recommendation: &[]string{"Consider scaling horizontally"}[0],
				Severity:       &[]generated.PerformanceStatsResponseBottlenecksSeverity{generated.PerformanceStatsResponseBottlenecksSeverityMedium}[0],
			},
		},
	}

	a.writeJSONResponse(w, http.StatusOK, stats)
}

// Helper methods

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
