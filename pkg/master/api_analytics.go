package master

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/gorilla/mux"
)

// Analytics API request/response structures

// TimeRangeParams represents common time range parameters
type TimeRangeParams struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Interval  string    `json:"interval"` // "hour", "day", "week", "month"
}

// CoverageTrendResponse represents coverage trend data
type CoverageTrendResponse struct {
	TimeRange TimeRangeParams      `json:"time_range"`
	Data      []CoverageTrendPoint `json:"data"`
	Summary   CoverageSummary      `json:"summary"`
}

// CoverageTrendPoint represents a single data point in coverage trend
type CoverageTrendPoint struct {
	Timestamp      time.Time `json:"timestamp"`
	TotalEdges     int64     `json:"total_edges"`
	NewEdges       int64     `json:"new_edges"`
	EdgeGrowthRate float64   `json:"edge_growth_rate"`
	ExecutionCount int64     `json:"execution_count"`
	BotCount       int       `json:"bot_count"`
}

// CoverageSummary represents aggregated coverage statistics
type CoverageSummary struct {
	TotalEdgesCovered   int64     `json:"total_edges_covered"`
	AverageGrowthRate   float64   `json:"average_growth_rate"`
	PeakGrowthTimestamp time.Time `json:"peak_growth_timestamp"`
	TotalExecutions     int64     `json:"total_executions"`
}

// CrashTimelineResponse represents crash timeline data
type CrashTimelineResponse struct {
	TimeRange TimeRangeParams      `json:"time_range"`
	Data      []CrashTimelinePoint `json:"data"`
	Summary   CrashSummary         `json:"summary"`
	ByType    map[string]int       `json:"by_type"`
	BySignal  map[int]int          `json:"by_signal"`
}

// CrashTimelinePoint represents a single data point in crash timeline
type CrashTimelinePoint struct {
	Timestamp     time.Time `json:"timestamp"`
	CrashCount    int       `json:"crash_count"`
	UniqueCrashes int       `json:"unique_crashes"`
	NewCrashes    int       `json:"new_crashes"`
	CrashRate     float64   `json:"crash_rate"` // Crashes per execution
}

// CrashSummary represents aggregated crash statistics
type CrashSummary struct {
	TotalCrashes     int       `json:"total_crashes"`
	UniqueCrashes    int       `json:"unique_crashes"`
	AverageCrashRate float64   `json:"average_crash_rate"`
	PeakCrashTime    time.Time `json:"peak_crash_time"`
	MostCommonType   string    `json:"most_common_type"`
	MostCommonSignal int       `json:"most_common_signal"`
}

// FuzzerComparisonResponse represents fuzzer performance comparison
type FuzzerComparisonResponse struct {
	TimeRange TimeRangeParams     `json:"time_range"`
	Fuzzers   []FuzzerPerformance `json:"fuzzers"`
	Winner    string              `json:"winner"` // Best performing fuzzer
}

// FuzzerPerformance represents performance metrics for a single fuzzer
type FuzzerPerformance struct {
	Fuzzer           string  `json:"fuzzer"`
	TotalJobs        int     `json:"total_jobs"`
	CompletedJobs    int     `json:"completed_jobs"`
	TotalCrashes     int     `json:"total_crashes"`
	UniqueCrashes    int     `json:"unique_crashes"`
	CoverageEdges    int64   `json:"coverage_edges"`
	ExecutionsPerSec float64 `json:"executions_per_sec"`
	CrashesPerHour   float64 `json:"crashes_per_hour"`
	EdgeGrowthRate   float64 `json:"edge_growth_rate"`
	SuccessRate      float64 `json:"success_rate"`
	AverageJobTime   float64 `json:"average_job_time"` // In seconds
}

// CampaignInsightsResponse represents campaign-level insights
type CampaignInsightsResponse struct {
	CampaignID      string              `json:"campaign_id"`
	TimeRange       TimeRangeParams     `json:"time_range"`
	Overview        CampaignOverview    `json:"overview"`
	FuzzerBreakdown []FuzzerPerformance `json:"fuzzer_breakdown"`
	BotUtilization  BotUtilizationStats `json:"bot_utilization"`
	CorpusGrowth    CorpusGrowthStats   `json:"corpus_growth"`
	Recommendations []string            `json:"recommendations"`
}

// CampaignOverview represents high-level campaign statistics
type CampaignOverview struct {
	Status           string    `json:"status"`
	StartTime        time.Time `json:"start_time"`
	Duration         float64   `json:"duration_hours"`
	TotalJobs        int       `json:"total_jobs"`
	ActiveJobs       int       `json:"active_jobs"`
	TotalCrashes     int       `json:"total_crashes"`
	UniqueCrashes    int       `json:"unique_crashes"`
	TotalCoverage    int64     `json:"total_coverage"`
	CorpusSize       int64     `json:"corpus_size"`
	ExecutionsPerSec float64   `json:"executions_per_sec"`
	EfficiencyScore  float64   `json:"efficiency_score"` // 0-100
}

// BotUtilizationStats represents bot resource utilization
type BotUtilizationStats struct {
	TotalBots          int                `json:"total_bots"`
	ActiveBots         int                `json:"active_bots"`
	AverageUtilization float64            `json:"average_utilization"` // Percentage
	PeakUtilization    float64            `json:"peak_utilization"`
	IdleTime           float64            `json:"idle_time_hours"`
	BotEfficiency      map[string]float64 `json:"bot_efficiency"` // Bot ID -> efficiency score
}

// CorpusGrowthStats represents corpus growth statistics
type CorpusGrowthStats struct {
	InitialSize      int64   `json:"initial_size"`
	CurrentSize      int64   `json:"current_size"`
	GrowthRate       float64 `json:"growth_rate"` // Percentage
	FilesAdded       int     `json:"files_added"`
	FilesRemoved     int     `json:"files_removed"`
	AverageFileSize  int64   `json:"average_file_size"`
	InterestingFiles int     `json:"interesting_files"` // Files that triggered new coverage
}

// Analytics handlers

// handleGetCoverageTrend handles coverage trend analytics
func (s *Server) handleGetCoverageTrend(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	params, err := s.parseTimeRangeParams(r)
	if err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid time range parameters", err)
		return
	}

	// Get campaign/job ID from query
	campaignID := r.URL.Query().Get("campaign_id")
	jobID := r.URL.Query().Get("job_id")

	// Validate that at least one filter is provided
	if campaignID == "" && jobID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Either campaign_id or job_id is required", nil)
		return
	}

	// Add timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Get coverage data from state
	var coverageData []*common.CoverageResult
	if jobID != "" {
		coverageData, err = s.state.GetJobCoverageHistory(ctx, jobID, params.StartTime, params.EndTime)
	} else {
		coverageData, err = s.state.GetCampaignCoverageHistory(ctx, campaignID, params.StartTime, params.EndTime)
	}

	if err != nil {
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve coverage data", err)
		return
	}

	// Aggregate data by interval
	trendData := s.aggregateCoverageTrend(coverageData, params.Interval)

	// Calculate summary statistics
	summary := s.calculateCoverageSummary(trendData)

	response := CoverageTrendResponse{
		TimeRange: *params,
		Data:      trendData,
		Summary:   summary,
	}

	// Handle export format
	format := r.URL.Query().Get("format")
	if format == "csv" {
		s.exportCoverageTrendCSV(w, response)
		return
	}

	s.writeJSONResponse(w, response)
}

// handleGetCrashTimeline handles crash timeline analytics
func (s *Server) handleGetCrashTimeline(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	params, err := s.parseTimeRangeParams(r)
	if err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid time range parameters", err)
		return
	}

	// Get campaign/job ID from query
	campaignID := r.URL.Query().Get("campaign_id")
	jobID := r.URL.Query().Get("job_id")

	// Add timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Get crash data from state
	var crashes []*common.CrashResult
	if jobID != "" {
		crashes, err = s.state.GetJobCrashesInTimeRange(ctx, jobID, params.StartTime, params.EndTime)
	} else if campaignID != "" {
		crashes, err = s.state.GetCampaignCrashesInTimeRange(ctx, campaignID, params.StartTime, params.EndTime)
	} else {
		// Get all crashes in time range
		crashes, err = s.state.GetCrashesInTimeRange(ctx, params.StartTime, params.EndTime)
	}

	if err != nil {
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve crash data", err)
		return
	}

	// Aggregate data by interval
	timelineData := s.aggregateCrashTimeline(crashes, params.Interval)

	// Calculate statistics by type and signal
	byType := make(map[string]int)
	bySignal := make(map[int]int)
	uniqueHashes := make(map[string]bool)

	for _, crash := range crashes {
		byType[crash.Type]++
		bySignal[crash.Signal]++
		uniqueHashes[crash.Hash] = true
	}

	// Calculate summary
	summary := s.calculateCrashSummary(crashes, timelineData)

	response := CrashTimelineResponse{
		TimeRange: *params,
		Data:      timelineData,
		Summary:   summary,
		ByType:    byType,
		BySignal:  bySignal,
	}

	// Handle export format
	format := r.URL.Query().Get("format")
	if format == "csv" {
		s.exportCrashTimelineCSV(w, response)
		return
	}

	s.writeJSONResponse(w, response)
}

// handleGetFuzzerComparison handles fuzzer performance comparison
func (s *Server) handleGetFuzzerComparison(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	params, err := s.parseTimeRangeParams(r)
	if err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid time range parameters", err)
		return
	}

	// Get campaign ID from query (optional)
	campaignID := r.URL.Query().Get("campaign_id")

	// Add timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Get jobs grouped by fuzzer
	var jobs []*common.Job
	if campaignID != "" {
		jobs, err = s.state.GetCampaignJobs(ctx, campaignID)
	} else {
		jobs, err = s.state.GetJobsInTimeRange(ctx, params.StartTime, params.EndTime)
	}

	if err != nil {
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve job data", err)
		return
	}

	// Group jobs by fuzzer and calculate performance metrics
	fuzzerMetrics := make(map[string]*FuzzerPerformance)

	for _, job := range jobs {
		if _, exists := fuzzerMetrics[job.Fuzzer]; !exists {
			fuzzerMetrics[job.Fuzzer] = &FuzzerPerformance{
				Fuzzer: job.Fuzzer,
			}
		}

		metrics := fuzzerMetrics[job.Fuzzer]
		metrics.TotalJobs++

		if job.Status == common.JobStatusCompleted {
			metrics.CompletedJobs++

			// Calculate job duration
			if job.StartedAt != nil && job.CompletedAt != nil {
				duration := job.CompletedAt.Sub(*job.StartedAt).Seconds()
				metrics.AverageJobTime = (metrics.AverageJobTime*float64(metrics.CompletedJobs-1) + duration) / float64(metrics.CompletedJobs)
			}
		}

		// Get crashes for this job
		crashes, _ := s.state.GetJobCrashes(ctx, job.ID)
		metrics.TotalCrashes += len(crashes)

		// Count unique crashes
		uniqueHashes := make(map[string]bool)
		for _, crash := range crashes {
			uniqueHashes[crash.Hash] = true
		}
		metrics.UniqueCrashes += len(uniqueHashes)

		// Get coverage stats
		coverageStats, _ := s.state.GetJobCoverageStats(ctx, job.ID)
		if coverageStats != nil {
			metrics.CoverageEdges += int64(coverageStats.TotalEdges)
			if coverageStats.ExecCount > 0 && job.StartedAt != nil && job.CompletedAt != nil {
				duration := job.CompletedAt.Sub(*job.StartedAt).Seconds()
				if duration > 0 {
					metrics.ExecutionsPerSec += float64(coverageStats.ExecCount) / duration
				}
			}
		}
	}

	// Calculate derived metrics and find winner
	var performances []FuzzerPerformance
	var winner string
	var bestScore float64

	for _, metrics := range fuzzerMetrics {
		// Calculate success rate
		if metrics.TotalJobs > 0 {
			metrics.SuccessRate = float64(metrics.CompletedJobs) / float64(metrics.TotalJobs) * 100
		}

		// Calculate crashes per hour
		if metrics.AverageJobTime > 0 && metrics.CompletedJobs > 0 {
			totalHours := metrics.AverageJobTime * float64(metrics.CompletedJobs) / 3600
			if totalHours > 0 {
				metrics.CrashesPerHour = float64(metrics.TotalCrashes) / totalHours
			}
		}

		// Calculate edge growth rate
		if metrics.CompletedJobs > 0 {
			metrics.EdgeGrowthRate = float64(metrics.CoverageEdges) / float64(metrics.CompletedJobs)
		}

		// Calculate overall score for determining winner
		score := metrics.SuccessRate*0.3 +
			metrics.CrashesPerHour*0.3 +
			metrics.EdgeGrowthRate*0.2 +
			float64(metrics.UniqueCrashes)*0.2

		if score > bestScore {
			bestScore = score
			winner = metrics.Fuzzer
		}

		performances = append(performances, *metrics)
	}

	response := FuzzerComparisonResponse{
		TimeRange: *params,
		Fuzzers:   performances,
		Winner:    winner,
	}

	// Handle export format
	format := r.URL.Query().Get("format")
	if format == "csv" {
		s.exportFuzzerComparisonCSV(w, response)
		return
	}

	s.writeJSONResponse(w, response)
}

// handleGetCampaignInsights handles campaign-level insights and recommendations
func (s *Server) handleGetCampaignInsights(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Parse query parameters
	params, err := s.parseTimeRangeParams(r)
	if err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid time range parameters", err)
		return
	}

	// Add timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Get campaign details
	campaignManager := s.state.GetCampaignManager()
	campaignState, err := campaignManager.GetCampaignState(campaignID)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		return
	}
	campaign := campaignState.Campaign

	// Get campaign statistics
	// TODO: Implement GetCampaignStats method or use an alternative approach
	stats := &common.CampaignStats{
		CampaignID: campaignID,
		TotalJobs:  len(campaignState.ActiveJobs) + len(campaignState.CompletedJobs),
	}

	// Calculate campaign overview
	overview := CampaignOverview{
		Status:        string(campaign.Status),
		StartTime:     campaign.CreatedAt,
		TotalJobs:     stats.TotalJobs,
		TotalCrashes:  stats.TotalCrashes,
		UniqueCrashes: stats.UniqueCrashes,
		TotalCoverage: stats.TotalCoverage,
		CorpusSize:    stats.CorpusSize,
	}

	// Calculate duration
	if campaign.CompletedAt != nil {
		overview.Duration = campaign.CompletedAt.Sub(campaign.CreatedAt).Hours()
	} else {
		overview.Duration = time.Since(campaign.CreatedAt).Hours()
	}

	// Get jobs for this campaign
	jobs, _ := s.state.GetCampaignJobs(ctx, campaignID)
	activeJobs := 0
	for _, job := range jobs {
		if job.Status == common.JobStatusRunning || job.Status == common.JobStatusAssigned {
			activeJobs++
		}
	}
	overview.ActiveJobs = activeJobs

	// Calculate executions per second
	totalExecs := int64(0)
	totalDuration := 0.0
	for _, job := range jobs {
		if job.Status == common.JobStatusCompleted && job.StartedAt != nil && job.CompletedAt != nil {
			coverageStats, _ := s.state.GetJobCoverageStats(ctx, job.ID)
			if coverageStats != nil {
				totalExecs += coverageStats.ExecCount
				totalDuration += job.CompletedAt.Sub(*job.StartedAt).Seconds()
			}
		}
	}
	if totalDuration > 0 {
		overview.ExecutionsPerSec = float64(totalExecs) / totalDuration
	}

	// Calculate efficiency score (0-100)
	overview.EfficiencyScore = s.calculateCampaignEfficiency(stats, overview, jobs)

	// Get fuzzer breakdown
	fuzzerBreakdown := s.getFuzzerBreakdownForCampaign(ctx, jobs)

	// Calculate bot utilization
	botUtilization := s.calculateBotUtilization(ctx, campaignID, jobs)

	// Calculate corpus growth
	corpusGrowth := s.calculateCorpusGrowth(ctx, campaignID)

	// Generate recommendations
	recommendations := s.generateCampaignRecommendations(overview, fuzzerBreakdown, botUtilization, corpusGrowth)

	response := CampaignInsightsResponse{
		CampaignID:      campaignID,
		TimeRange:       *params,
		Overview:        overview,
		FuzzerBreakdown: fuzzerBreakdown,
		BotUtilization:  botUtilization,
		CorpusGrowth:    corpusGrowth,
		Recommendations: recommendations,
	}

	// Handle export format
	format := r.URL.Query().Get("format")
	if format == "json" || format == "" {
		s.writeJSONResponse(w, response)
	} else if format == "html" {
		s.exportCampaignInsightsHTML(w, response)
	}
}

// Helper methods

// parseTimeRangeParams parses time range parameters from request
func (s *Server) parseTimeRangeParams(r *http.Request) (*TimeRangeParams, error) {
	params := &TimeRangeParams{
		Interval: "hour", // Default interval
	}

	// Parse start time
	startStr := r.URL.Query().Get("start_time")
	if startStr != "" {
		startTime, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			return nil, fmt.Errorf("invalid start_time format: %v", err)
		}
		params.StartTime = startTime
	} else {
		// Default to 24 hours ago
		params.StartTime = time.Now().Add(-24 * time.Hour)
	}

	// Parse end time
	endStr := r.URL.Query().Get("end_time")
	if endStr != "" {
		endTime, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			return nil, fmt.Errorf("invalid end_time format: %v", err)
		}
		params.EndTime = endTime
	} else {
		// Default to now
		params.EndTime = time.Now()
	}

	// Validate time range
	if params.EndTime.Before(params.StartTime) {
		return nil, fmt.Errorf("end_time must be after start_time")
	}

	// Parse interval
	interval := r.URL.Query().Get("interval")
	if interval != "" {
		validIntervals := []string{"minute", "hour", "day", "week", "month"}
		isValid := false
		for _, v := range validIntervals {
			if interval == v {
				isValid = true
				break
			}
		}
		if !isValid {
			return nil, fmt.Errorf("invalid interval: %s", interval)
		}
		params.Interval = interval
	}

	return params, nil
}

// aggregateCoverageTrend aggregates coverage data by time interval
func (s *Server) aggregateCoverageTrend(data []*common.CoverageResult, interval string) []CoverageTrendPoint {
	if len(data) == 0 {
		return []CoverageTrendPoint{}
	}

	// Group data by time interval
	buckets := make(map[time.Time]*CoverageTrendPoint)

	for _, coverage := range data {
		// Round timestamp to interval
		bucketTime := s.roundToInterval(coverage.Timestamp, interval)

		if _, exists := buckets[bucketTime]; !exists {
			buckets[bucketTime] = &CoverageTrendPoint{
				Timestamp: bucketTime,
			}
		}

		point := buckets[bucketTime]
		point.TotalEdges += int64(coverage.Edges)
		point.NewEdges += int64(coverage.NewEdges)
		point.ExecutionCount += coverage.ExecCount
		point.BotCount++ // This counts occurrences, not unique bots
	}

	// Convert map to sorted slice
	var result []CoverageTrendPoint
	for _, point := range buckets {
		result = append(result, *point)
	}

	// Sort by timestamp
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.Before(result[j].Timestamp)
	})

	// Calculate growth rates
	for i := 1; i < len(result); i++ {
		if result[i-1].TotalEdges > 0 {
			result[i].EdgeGrowthRate = float64(result[i].TotalEdges-result[i-1].TotalEdges) / float64(result[i-1].TotalEdges) * 100
		}
	}

	return result
}

// aggregateCrashTimeline aggregates crash data by time interval
func (s *Server) aggregateCrashTimeline(crashes []*common.CrashResult, interval string) []CrashTimelinePoint {
	if len(crashes) == 0 {
		return []CrashTimelinePoint{}
	}

	// Group crashes by time interval
	buckets := make(map[time.Time]*CrashTimelinePoint)
	uniqueHashes := make(map[time.Time]map[string]bool)

	for _, crash := range crashes {
		// Round timestamp to interval
		bucketTime := s.roundToInterval(crash.Timestamp, interval)

		if _, exists := buckets[bucketTime]; !exists {
			buckets[bucketTime] = &CrashTimelinePoint{
				Timestamp: bucketTime,
			}
			uniqueHashes[bucketTime] = make(map[string]bool)
		}

		point := buckets[bucketTime]
		point.CrashCount++

		// Track unique crashes
		if !uniqueHashes[bucketTime][crash.Hash] {
			uniqueHashes[bucketTime][crash.Hash] = true
			point.UniqueCrashes++
			if crash.IsUnique {
				point.NewCrashes++
			}
		}
	}

	// Convert map to sorted slice
	var result []CrashTimelinePoint
	for timestamp, point := range buckets {
		point.UniqueCrashes = len(uniqueHashes[timestamp])
		result = append(result, *point)
	}

	// Sort by timestamp
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.Before(result[j].Timestamp)
	})

	return result
}

// roundToInterval rounds a timestamp to the specified interval
func (s *Server) roundToInterval(t time.Time, interval string) time.Time {
	switch interval {
	case "minute":
		return t.Truncate(time.Minute)
	case "hour":
		return t.Truncate(time.Hour)
	case "day":
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	case "week":
		// Round to start of week (Monday)
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return time.Date(t.Year(), t.Month(), t.Day()-weekday+1, 0, 0, 0, 0, t.Location())
	case "month":
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	default:
		return t.Truncate(time.Hour)
	}
}

// calculateCoverageSummary calculates coverage summary statistics
func (s *Server) calculateCoverageSummary(data []CoverageTrendPoint) CoverageSummary {
	if len(data) == 0 {
		return CoverageSummary{}
	}

	summary := CoverageSummary{}
	totalGrowthRate := 0.0
	peakGrowth := 0.0

	for _, point := range data {
		if point.TotalEdges > summary.TotalEdgesCovered {
			summary.TotalEdgesCovered = point.TotalEdges
		}

		summary.TotalExecutions += point.ExecutionCount

		if point.EdgeGrowthRate > peakGrowth {
			peakGrowth = point.EdgeGrowthRate
			summary.PeakGrowthTimestamp = point.Timestamp
		}

		totalGrowthRate += point.EdgeGrowthRate
	}

	if len(data) > 0 {
		summary.AverageGrowthRate = totalGrowthRate / float64(len(data))
	}

	return summary
}

// calculateCrashSummary calculates crash summary statistics
func (s *Server) calculateCrashSummary(crashes []*common.CrashResult, timeline []CrashTimelinePoint) CrashSummary {
	summary := CrashSummary{
		TotalCrashes: len(crashes),
	}

	// Count unique crashes
	uniqueHashes := make(map[string]bool)
	typeCount := make(map[string]int)
	signalCount := make(map[int]int)

	for _, crash := range crashes {
		uniqueHashes[crash.Hash] = true
		typeCount[crash.Type]++
		signalCount[crash.Signal]++
	}

	summary.UniqueCrashes = len(uniqueHashes)

	// Find most common type and signal
	maxTypeCount := 0
	maxSignalCount := 0

	for crashType, count := range typeCount {
		if count > maxTypeCount {
			maxTypeCount = count
			summary.MostCommonType = crashType
		}
	}

	for signal, count := range signalCount {
		if count > maxSignalCount {
			maxSignalCount = count
			summary.MostCommonSignal = signal
		}
	}

	// Find peak crash time
	peakCrashes := 0
	for _, point := range timeline {
		if point.CrashCount > peakCrashes {
			peakCrashes = point.CrashCount
			summary.PeakCrashTime = point.Timestamp
		}
	}

	return summary
}

// Export helper methods

// exportCoverageTrendCSV exports coverage trend data as CSV
func (s *Server) exportCoverageTrendCSV(w http.ResponseWriter, data CoverageTrendResponse) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=coverage_trend.csv")

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header
	header := []string{"Timestamp", "Total Edges", "New Edges", "Edge Growth Rate %", "Execution Count", "Bot Count"}
	writer.Write(header)

	// Write data rows
	for _, point := range data.Data {
		row := []string{
			point.Timestamp.Format(time.RFC3339),
			strconv.FormatInt(point.TotalEdges, 10),
			strconv.FormatInt(point.NewEdges, 10),
			fmt.Sprintf("%.2f", point.EdgeGrowthRate),
			strconv.FormatInt(point.ExecutionCount, 10),
			strconv.Itoa(point.BotCount),
		}
		writer.Write(row)
	}
}

// exportCrashTimelineCSV exports crash timeline data as CSV
func (s *Server) exportCrashTimelineCSV(w http.ResponseWriter, data CrashTimelineResponse) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=crash_timeline.csv")

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header
	header := []string{"Timestamp", "Crash Count", "Unique Crashes", "New Crashes", "Crash Rate"}
	writer.Write(header)

	// Write data rows
	for _, point := range data.Data {
		row := []string{
			point.Timestamp.Format(time.RFC3339),
			strconv.Itoa(point.CrashCount),
			strconv.Itoa(point.UniqueCrashes),
			strconv.Itoa(point.NewCrashes),
			fmt.Sprintf("%.4f", point.CrashRate),
		}
		writer.Write(row)
	}
}

// exportFuzzerComparisonCSV exports fuzzer comparison data as CSV
func (s *Server) exportFuzzerComparisonCSV(w http.ResponseWriter, data FuzzerComparisonResponse) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=fuzzer_comparison.csv")

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header
	header := []string{
		"Fuzzer", "Total Jobs", "Completed Jobs", "Total Crashes", "Unique Crashes",
		"Coverage Edges", "Executions/Sec", "Crashes/Hour", "Edge Growth Rate",
		"Success Rate %", "Avg Job Time (sec)",
	}
	writer.Write(header)

	// Write data rows
	for _, fuzzer := range data.Fuzzers {
		row := []string{
			fuzzer.Fuzzer,
			strconv.Itoa(fuzzer.TotalJobs),
			strconv.Itoa(fuzzer.CompletedJobs),
			strconv.Itoa(fuzzer.TotalCrashes),
			strconv.Itoa(fuzzer.UniqueCrashes),
			strconv.FormatInt(fuzzer.CoverageEdges, 10),
			fmt.Sprintf("%.2f", fuzzer.ExecutionsPerSec),
			fmt.Sprintf("%.2f", fuzzer.CrashesPerHour),
			fmt.Sprintf("%.2f", fuzzer.EdgeGrowthRate),
			fmt.Sprintf("%.2f", fuzzer.SuccessRate),
			fmt.Sprintf("%.2f", fuzzer.AverageJobTime),
		}
		writer.Write(row)
	}
}

// exportCampaignInsightsHTML exports campaign insights as HTML report
func (s *Server) exportCampaignInsightsHTML(w http.ResponseWriter, data CampaignInsightsResponse) {
	w.Header().Set("Content-Type", "text/html")

	// Simple HTML template for insights report
	_ = `
<!DOCTYPE html>
<html>
<head>
    <title>Campaign Insights - {{.CampaignID}}</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        h1, h2 { color: #333; }
        table { border-collapse: collapse; width: 100%; margin: 20px 0; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #f2f2f2; }
        .metric { display: inline-block; margin: 10px 20px 10px 0; }
        .metric-value { font-size: 24px; font-weight: bold; color: #2196F3; }
        .recommendation { background-color: #fffacd; padding: 10px; margin: 5px 0; border-radius: 5px; }
    </style>
</head>
<body>
    <h1>Campaign Insights Report</h1>
    <p>Campaign ID: {{.CampaignID}}</p>
    <p>Generated: {{.Timestamp}}</p>

    <h2>Overview</h2>
    <div class="metrics">
        <div class="metric">
            <div>Status</div>
            <div class="metric-value">{{.Overview.Status}}</div>
        </div>
        <div class="metric">
            <div>Duration</div>
            <div class="metric-value">{{.Overview.Duration}} hours</div>
        </div>
        <div class="metric">
            <div>Total Jobs</div>
            <div class="metric-value">{{.Overview.TotalJobs}}</div>
        </div>
        <div class="metric">
            <div>Unique Crashes</div>
            <div class="metric-value">{{.Overview.UniqueCrashes}}</div>
        </div>
        <div class="metric">
            <div>Coverage</div>
            <div class="metric-value">{{.Overview.TotalCoverage}}</div>
        </div>
        <div class="metric">
            <div>Efficiency Score</div>
            <div class="metric-value">{{.Overview.EfficiencyScore}}%</div>
        </div>
    </div>

    <h2>Fuzzer Performance</h2>
    <table>
        <tr>
            <th>Fuzzer</th>
            <th>Jobs</th>
            <th>Crashes</th>
            <th>Coverage</th>
            <th>Success Rate</th>
        </tr>
        {{range .FuzzerBreakdown}}
        <tr>
            <td>{{.Fuzzer}}</td>
            <td>{{.TotalJobs}}</td>
            <td>{{.TotalCrashes}}</td>
            <td>{{.CoverageEdges}}</td>
            <td>{{.SuccessRate}}%</td>
        </tr>
        {{end}}
    </table>

    <h2>Recommendations</h2>
    {{range .Recommendations}}
    <div class="recommendation">{{.}}</div>
    {{end}}
</body>
</html>
`

	// For simplicity, just write a basic HTML response
	// In production, use html/template package
	fmt.Fprintf(w, "<html><body><h1>Campaign Insights for %s</h1><pre>%s</pre></body></html>",
		data.CampaignID, s.prettyPrintJSON(data))
}

// Helper methods for campaign insights

// calculateCampaignEfficiency calculates overall campaign efficiency score
func (s *Server) calculateCampaignEfficiency(stats *common.CampaignStats, overview CampaignOverview, jobs []*common.Job) float64 {
	score := 0.0
	factors := 0

	// Factor 1: Job completion rate (max 25 points)
	if overview.TotalJobs > 0 {
		completionRate := float64(stats.CompletedJobs) / float64(overview.TotalJobs)
		score += completionRate * 25
		factors++
	}

	// Factor 2: Crash discovery rate (max 25 points)
	if overview.Duration > 0 {
		crashRate := float64(overview.UniqueCrashes) / overview.Duration
		// Normalize: 1 unique crash per hour = 25 points
		score += math.Min(crashRate*25, 25)
		factors++
	}

	// Factor 3: Coverage growth (max 25 points)
	if stats.TotalCoverage > 0 {
		// Assume good coverage is 10000+ edges
		coverageScore := math.Min(float64(stats.TotalCoverage)/10000*25, 25)
		score += coverageScore
		factors++
	}

	// Factor 4: Resource utilization (max 25 points)
	activeTime := 0.0
	for _, job := range jobs {
		if job.StartedAt != nil && job.CompletedAt != nil {
			activeTime += job.CompletedAt.Sub(*job.StartedAt).Hours()
		}
	}
	if overview.Duration > 0 {
		utilization := activeTime / (overview.Duration * float64(len(jobs)))
		score += math.Min(utilization*25, 25)
		factors++
	}

	if factors > 0 {
		return score / float64(factors) * 4 // Scale to 0-100
	}
	return 0
}

// getFuzzerBreakdownForCampaign gets fuzzer performance for a campaign
func (s *Server) getFuzzerBreakdownForCampaign(ctx context.Context, jobs []*common.Job) []FuzzerPerformance {
	fuzzerMap := make(map[string]*FuzzerPerformance)

	for _, job := range jobs {
		if _, exists := fuzzerMap[job.Fuzzer]; !exists {
			fuzzerMap[job.Fuzzer] = &FuzzerPerformance{
				Fuzzer: job.Fuzzer,
			}
		}

		perf := fuzzerMap[job.Fuzzer]
		perf.TotalJobs++

		if job.Status == common.JobStatusCompleted {
			perf.CompletedJobs++
		}

		// Get job-specific metrics
		crashes, _ := s.state.GetJobCrashes(ctx, job.ID)
		perf.TotalCrashes += len(crashes)

		coverageStats, _ := s.state.GetJobCoverageStats(ctx, job.ID)
		if coverageStats != nil {
			perf.CoverageEdges += int64(coverageStats.TotalEdges)
		}
	}

	// Calculate derived metrics
	var result []FuzzerPerformance
	for _, perf := range fuzzerMap {
		if perf.TotalJobs > 0 {
			perf.SuccessRate = float64(perf.CompletedJobs) / float64(perf.TotalJobs) * 100
		}
		result = append(result, *perf)
	}

	return result
}

// calculateBotUtilization calculates bot utilization statistics
func (s *Server) calculateBotUtilization(ctx context.Context, campaignID string, jobs []*common.Job) BotUtilizationStats {
	stats := BotUtilizationStats{
		BotEfficiency: make(map[string]float64),
	}

	// Get all bots
	bots, _ := s.state.ListBots(ctx)
	stats.TotalBots = len(bots)

	// Track bot usage
	botJobTime := make(map[string]float64)
	botJobCount := make(map[string]int)

	for _, job := range jobs {
		if job.AssignedBot != nil && job.StartedAt != nil && job.CompletedAt != nil {
			botID := *job.AssignedBot
			duration := job.CompletedAt.Sub(*job.StartedAt).Hours()
			botJobTime[botID] += duration
			botJobCount[botID]++
		}
	}

	stats.ActiveBots = len(botJobTime)

	// Calculate utilization
	totalPossibleTime := 0.0
	totalActualTime := 0.0

	for botID, jobTime := range botJobTime {
		// Assume campaign duration as possible time
		campaignManager := s.state.GetCampaignManager()
		campaignState, _ := campaignManager.GetCampaignState(campaignID)
		if campaignState != nil && campaignState.Campaign != nil {
			campaign := campaignState.Campaign
			possibleTime := time.Since(campaign.CreatedAt).Hours()
			if campaign.CompletedAt != nil {
				possibleTime = campaign.CompletedAt.Sub(campaign.CreatedAt).Hours()
			}

			utilization := (jobTime / possibleTime) * 100
			stats.BotEfficiency[botID] = utilization

			totalPossibleTime += possibleTime
			totalActualTime += jobTime
		}
	}

	if totalPossibleTime > 0 {
		stats.AverageUtilization = (totalActualTime / totalPossibleTime) * 100
	}

	// Find peak utilization
	for _, efficiency := range stats.BotEfficiency {
		if efficiency > stats.PeakUtilization {
			stats.PeakUtilization = efficiency
		}
	}

	// Calculate idle time
	stats.IdleTime = totalPossibleTime - totalActualTime

	return stats
}

// calculateCorpusGrowth calculates corpus growth statistics
func (s *Server) calculateCorpusGrowth(ctx context.Context, campaignID string) CorpusGrowthStats {
	stats := CorpusGrowthStats{}

	// Get corpus updates for campaign
	corpusUpdates, err := s.state.GetCampaignCorpusUpdates(ctx, campaignID)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to get corpus updates")
		return stats
	}

	if len(corpusUpdates) == 0 {
		return stats
	}

	// Find initial and current sizes
	stats.InitialSize = corpusUpdates[0].TotalSize
	stats.CurrentSize = corpusUpdates[len(corpusUpdates)-1].TotalSize

	// Calculate growth rate
	if stats.InitialSize > 0 {
		stats.GrowthRate = float64(stats.CurrentSize-stats.InitialSize) / float64(stats.InitialSize) * 100
	}

	// Count files added
	for _, update := range corpusUpdates {
		stats.FilesAdded += len(update.Files)
	}

	// Calculate average file size
	if stats.FilesAdded > 0 {
		stats.AverageFileSize = stats.CurrentSize / int64(stats.FilesAdded)
	}

	// Count interesting files (those that triggered new coverage)
	// This would require correlating with coverage data
	// For now, estimate based on corpus growth pattern
	stats.InterestingFiles = stats.FilesAdded / 10 // Rough estimate

	return stats
}

// generateCampaignRecommendations generates actionable recommendations
func (s *Server) generateCampaignRecommendations(
	overview CampaignOverview,
	fuzzers []FuzzerPerformance,
	botUtil BotUtilizationStats,
	corpusGrowth CorpusGrowthStats,
) []string {
	recommendations := []string{}

	// Check efficiency score
	if overview.EfficiencyScore < 50 {
		recommendations = append(recommendations, "Campaign efficiency is below 50%. Consider reviewing fuzzer configurations and bot allocation.")
	}

	// Check bot utilization
	if botUtil.AverageUtilization < 60 {
		recommendations = append(recommendations, fmt.Sprintf("Bot utilization is only %.1f%%. Consider reducing bot count or increasing job parallelism.", botUtil.AverageUtilization))
	}

	// Check crash discovery rate
	if overview.Duration > 24 && overview.UniqueCrashes == 0 {
		recommendations = append(recommendations, "No crashes found after 24 hours. Consider updating the seed corpus or adjusting fuzzer parameters.")
	}

	// Check corpus growth
	if corpusGrowth.GrowthRate < 10 && overview.Duration > 12 {
		recommendations = append(recommendations, "Corpus growth is slow. Consider enabling corpus sharing between jobs or importing additional seed inputs.")
	}

	// Check fuzzer performance
	var bestFuzzer string
	var bestCrashRate float64
	for _, fuzzer := range fuzzers {
		if fuzzer.CrashesPerHour > bestCrashRate {
			bestCrashRate = fuzzer.CrashesPerHour
			bestFuzzer = fuzzer.Fuzzer
		}
	}
	if bestFuzzer != "" && len(fuzzers) > 1 {
		recommendations = append(recommendations, fmt.Sprintf("%s is performing best with %.2f crashes/hour. Consider allocating more resources to this fuzzer.", bestFuzzer, bestCrashRate))
	}

	// Check for stalled campaign
	if overview.ActiveJobs == 0 && overview.Status == "running" {
		recommendations = append(recommendations, "No active jobs in running campaign. Check for bot availability or job creation issues.")
	}

	// Check execution rate
	if overview.ExecutionsPerSec < 100 {
		recommendations = append(recommendations, "Low execution rate detected. Consider optimizing target binary or increasing CPU allocation.")
	}

	return recommendations
}

// prettyPrintJSON formats JSON for display
func (s *Server) prettyPrintJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return string(b)
}
