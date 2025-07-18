package common

import (
	"context"
	"time"
)

// MetricsCollector handles collection of enhanced fuzzing metrics
type MetricsCollector interface {
	Start(ctx context.Context) error
	Stop() error
	Collect() (*EnhancedMetrics, error)
	GetMetrics() *EnhancedMetrics
	UpdateCoverageMetrics(coverage CoverageMetrics)
	UpdatePerformanceMetrics(perf PerformanceMetrics)
	UpdateFuzzerSpecificMetrics(key string, value interface{})
	RecordExecution(duration time.Duration)
}

// EnhancedMetrics contains comprehensive fuzzing metrics
type EnhancedMetrics struct {
	Timestamp      time.Time              `json:"timestamp"`
	Coverage       CoverageMetrics        `json:"coverage"`
	Performance    PerformanceMetrics     `json:"performance"`
	ResourceUsage  ResourceUsageMetrics   `json:"resource_usage"`
	FuzzerSpecific map[string]interface{} `json:"fuzzer_specific"`
}

// CoverageMetrics contains detailed coverage information
type CoverageMetrics struct {
	LineCoverage     float64            `json:"line_coverage"`
	FunctionCoverage float64            `json:"function_coverage"`
	BranchCoverage   float64            `json:"branch_coverage"`
	PathCoverage     float64            `json:"path_coverage"`
	CoverageGrowth   float64            `json:"coverage_growth_rate"`
	NewEdgesFound    int64              `json:"new_edges_found"`
	TotalEdges       int64              `json:"total_edges"`
	CoverageByModule map[string]float64 `json:"coverage_by_module"`
	HotSpots         []CodeHotSpot      `json:"hot_spots"`
}

// CodeHotSpot represents frequently executed code areas
type CodeHotSpot struct {
	Location string  `json:"location"`
	HitCount int64   `json:"hit_count"`
	Coverage float64 `json:"coverage"`
}

// PerformanceMetrics contains performance-related metrics
type PerformanceMetrics struct {
	ExecutionsPerSecond float64               `json:"executions_per_second"`
	AverageExecTime     time.Duration         `json:"average_exec_time"`
	MedianExecTime      time.Duration         `json:"median_exec_time"`
	P95ExecTime         time.Duration         `json:"p95_exec_time"`
	P99ExecTime         time.Duration         `json:"p99_exec_time"`
	ThroughputMBps      float64               `json:"throughput_mbps"`
	InputGenerationRate float64               `json:"input_generation_rate"`
	MutationEfficiency  float64               `json:"mutation_efficiency"`
	QueueUtilization    float64               `json:"queue_utilization"`
	PerformanceHistory  []PerformanceSnapshot `json:"performance_history"`
}

// PerformanceSnapshot represents a point-in-time performance measurement
type PerformanceSnapshot struct {
	Timestamp           time.Time `json:"timestamp"`
	ExecutionsPerSecond float64   `json:"executions_per_second"`
	CPUUsage            float64   `json:"cpu_usage"`
	MemoryUsage         float64   `json:"memory_usage"`
}

// ResourceUsageMetrics contains resource utilization metrics
type ResourceUsageMetrics struct {
	CPUUsagePercent    float64            `json:"cpu_usage_percent"`
	MemoryUsageMB      float64            `json:"memory_usage_mb"`
	DiskUsageGB        float64            `json:"disk_usage_gb"`
	NetworkBandwidthMB float64            `json:"network_bandwidth_mb"`
	FileDescriptors    int                `json:"file_descriptors"`
	Threads            int                `json:"threads"`
	ResourceEfficiency float64            `json:"resource_efficiency"`
	ResourceHistory    []ResourceSnapshot `json:"resource_history"`
}

// ResourceSnapshot represents a point-in-time resource measurement
type ResourceSnapshot struct {
	Timestamp   time.Time `json:"timestamp"`
	CPUUsage    float64   `json:"cpu_usage"`
	MemoryUsage float64   `json:"memory_usage"`
	DiskIO      float64   `json:"disk_io"`
}
