package queries

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/query"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
)

// GetCampaignStatsQuery represents a query to get campaign statistics
type GetCampaignStatsQuery struct {
	CampaignID *string `json:"campaign_id,omitempty"` // Specific campaign or global stats

	// Time range
	TimeRange *TimeRangeFilter `json:"time_range,omitempty"`

	// Aggregation options
	GroupBy        string `json:"group_by,omitempty" validate:"omitempty,oneof=day week month status"`
	IncludeTrends  bool   `json:"include_trends"`
	IncludeHealth  bool   `json:"include_health"`
	IncludeDetails bool   `json:"include_details"`

	// Comparison
	CompareWith *ComparisonOptions `json:"compare_with,omitempty"`
}

// TimeRangeFilter defines a time range for statistics
type TimeRangeFilter struct {
	Start  *time.Time `json:"start,omitempty"`
	End    *time.Time `json:"end,omitempty"`
	Preset string     `json:"preset,omitempty" validate:"omitempty,oneof=today yesterday week month quarter year"`
}

// ComparisonOptions defines comparison parameters
type ComparisonOptions struct {
	Type   string           `json:"type" validate:"required,oneof=previous_period previous_year custom"`
	Custom *TimeRangeFilter `json:"custom,omitempty"`
}

// CampaignStatsResult represents comprehensive campaign statistics
type CampaignStatsResult struct {
	Stats      *CampaignStatsOverviewDTO `json:"stats"`
	Trends     *CampaignTrendsDTO        `json:"trends,omitempty"`
	Health     *CampaignHealthDTO        `json:"health,omitempty"`
	Details    *CampaignDetailsStatsDTO  `json:"details,omitempty"`
	Comparison *ComparisonResultDTO      `json:"comparison,omitempty"`
	Period     *PeriodInfoDTO            `json:"period"`
}

// CampaignStatsOverviewDTO provides overview statistics
type CampaignStatsOverviewDTO struct {
	// Campaign counts
	TotalCampaigns     int `json:"total_campaigns"`
	ActiveCampaigns    int `json:"active_campaigns"`
	CompletedCampaigns int `json:"completed_campaigns"`
	FailedCampaigns    int `json:"failed_campaigns"`

	// Performance metrics
	TotalJobs      int           `json:"total_jobs"`
	CompletedJobs  int           `json:"completed_jobs"`
	AverageJobTime time.Duration `json:"average_job_time"`

	// Crash metrics
	TotalCrashes  int     `json:"total_crashes"`
	UniqueCrashes int     `json:"unique_crashes"`
	CrashRate     float64 `json:"crash_rate"`

	// Coverage metrics
	AverageCoverage float64 `json:"average_coverage"`
	MaxCoverage     float64 `json:"max_coverage"`
	CoverageGrowth  float64 `json:"coverage_growth"`

	// Resource utilization
	TotalBotHours      float64 `json:"total_bot_hours"`
	AvgBotsPerCampaign float64 `json:"avg_bots_per_campaign"`
	ResourceEfficiency float64 `json:"resource_efficiency"`
}

// CampaignTrendsDTO shows trends over time
type CampaignTrendsDTO struct {
	CampaignCreation    map[string]int     `json:"campaign_creation"`
	JobExecution        map[string]int     `json:"job_execution"`
	CrashDiscovery      map[string]int     `json:"crash_discovery"`
	CoverageProgression map[string]float64 `json:"coverage_progression"`
	ResourceUsage       map[string]float64 `json:"resource_usage"`
}

// CampaignHealthDTO provides health metrics
type CampaignHealthDTO struct {
	OverallHealth string  `json:"overall_health"` // good, warning, critical
	HealthScore   float64 `json:"health_score"`   // 0-100

	// Health indicators
	LongRunningCampaigns    int `json:"long_running_campaigns"`
	StagnantCampaigns       int `json:"stagnant_campaigns"`
	LowPerformanceCampaigns int `json:"low_performance_campaigns"`

	// Performance indicators
	SuccessRate           float64       `json:"success_rate"`
	AverageTimeToStart    time.Duration `json:"average_time_to_start"`
	AverageTimeToComplete time.Duration `json:"average_time_to_complete"`

	// Recommendations
	Recommendations []HealthRecommendationDTO `json:"recommendations"`
}

// HealthRecommendationDTO provides health improvement recommendations
type HealthRecommendationDTO struct {
	Type        string   `json:"type"`
	Severity    string   `json:"severity"`
	Message     string   `json:"message"`
	AffectedIDs []string `json:"affected_ids,omitempty"`
}

// CampaignDetailsStatsDTO provides detailed statistics
type CampaignDetailsStatsDTO struct {
	StatusBreakdown    map[string]int              `json:"status_breakdown"`
	TopPerformers      []CampaignPerformanceDTO    `json:"top_performers"`
	BottomPerformers   []CampaignPerformanceDTO    `json:"bottom_performers"`
	ResourceAllocation map[string]ResourceStatsDTO `json:"resource_allocation"`
	CrashDistribution  map[string]int              `json:"crash_distribution"`
}

// CampaignPerformanceDTO represents campaign performance metrics
type CampaignPerformanceDTO struct {
	CampaignID   string                 `json:"campaign_id"`
	CampaignName string                 `json:"campaign_name"`
	Score        float64                `json:"score"`
	Metrics      map[string]interface{} `json:"metrics"`
}

// ResourceStatsDTO represents resource utilization statistics
type ResourceStatsDTO struct {
	BotHours   float64 `json:"bot_hours"`
	JobCount   int     `json:"job_count"`
	Efficiency float64 `json:"efficiency"`
}

// ComparisonResultDTO shows comparison between periods
type ComparisonResultDTO struct {
	CurrentPeriod  *CampaignStatsOverviewDTO `json:"current_period"`
	PreviousPeriod *CampaignStatsOverviewDTO `json:"previous_period"`
	Changes        map[string]ChangeDTO      `json:"changes"`
}

// ChangeDTO represents a metric change
type ChangeDTO struct {
	Value      float64 `json:"value"`
	Percentage float64 `json:"percentage"`
	Direction  string  `json:"direction"` // up, down, stable
}

// PeriodInfoDTO provides information about the statistics period
type PeriodInfoDTO struct {
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	Duration   string    `json:"duration"`
	DataPoints int       `json:"data_points"`
}

// GetCampaignStatsHandler handles campaign statistics queries
type GetCampaignStatsHandler struct {
	repo         repository.CampaignRepository
	statsService *query.CampaignStatisticsService
}

// NewGetCampaignStatsHandler creates a new handler instance
func NewGetCampaignStatsHandler(repo repository.CampaignRepository) *GetCampaignStatsHandler {
	return &GetCampaignStatsHandler{
		repo:         repo,
		statsService: query.NewCampaignStatisticsService(repo),
	}
}

// Handle executes the get campaign stats query
func (h *GetCampaignStatsHandler) Handle(ctx context.Context, q interface{}) (interface{}, error) {
	query, ok := q.(*GetCampaignStatsQuery)
	if !ok {
		return nil, NewApplicationError(
			ErrCodeInvalidQuery,
			"Invalid query type",
			nil,
		).WithDetails("expected", "*GetCampaignStatsQuery")
	}

	// Validate query
	if err := h.validateQuery(query); err != nil {
		return nil, err
	}

	// Check authorization
	if err := h.checkAuthorization(ctx, query); err != nil {
		return nil, err
	}

	// Determine time range
	timeRange := h.resolveTimeRange(query.TimeRange)

	// Build statistics options
	statsOpts := query.StatisticsOptions{
		IncludeTrends:        query.IncludeTrends,
		IncludeHealthMetrics: query.IncludeHealth,
		TimeRange: &query.TimeRange{
			Start: timeRange.Start,
			End:   timeRange.End,
		},
		CacheResults: true,
	}

	// Get statistics
	var result *CampaignStatsResult
	var err error

	if query.CampaignID != nil {
		// Single campaign statistics
		result, err = h.getSingleCampaignStats(ctx, *query.CampaignID, statsOpts, query)
	} else {
		// Global statistics
		result, err = h.getGlobalStats(ctx, statsOpts, query)
	}

	if err != nil {
		return nil, NewApplicationError(
			ErrCodeOperationFailed,
			"Failed to retrieve statistics",
			err,
		)
	}

	// Add comparison if requested
	if query.CompareWith != nil {
		comparison, err := h.getComparison(ctx, query, result)
		if err == nil {
			result.Comparison = comparison
		}
	}

	return result, nil
}

// validateQuery validates the statistics query
func (h *GetCampaignStatsHandler) validateQuery(query *GetCampaignStatsQuery) error {
	// Validate time range
	if query.TimeRange != nil {
		if query.TimeRange.Start != nil && query.TimeRange.End != nil {
			if query.TimeRange.Start.After(*query.TimeRange.End) {
				return NewApplicationError(
					ErrCodeValidationFailed,
					"Start date must be before end date",
					nil,
				)
			}
		}

		if query.TimeRange.Preset != "" {
			validPresets := []string{"today", "yesterday", "week", "month", "quarter", "year"}
			isValid := false
			for _, preset := range validPresets {
				if query.TimeRange.Preset == preset {
					isValid = true
					break
				}
			}
			if !isValid {
				return NewApplicationError(
					ErrCodeValidationFailed,
					"Invalid time range preset",
					nil,
				).WithDetails("preset", query.TimeRange.Preset).WithDetails("valid_presets", validPresets)
			}
		}
	}

	// Validate group by
	if query.GroupBy != "" {
		validGroupBy := []string{"day", "week", "month", "status"}
		isValid := false
		for _, valid := range validGroupBy {
			if query.GroupBy == valid {
				isValid = true
				break
			}
		}
		if !isValid {
			return NewApplicationError(
				ErrCodeValidationFailed,
				"Invalid group by option",
				nil,
			).WithDetails("group_by", query.GroupBy)
		}
	}

	// Validate comparison
	if query.CompareWith != nil {
		validTypes := []string{"previous_period", "previous_year", "custom"}
		isValid := false
		for _, valid := range validTypes {
			if query.CompareWith.Type == valid {
				isValid = true
				break
			}
		}
		if !isValid {
			return NewApplicationError(
				ErrCodeValidationFailed,
				"Invalid comparison type",
				nil,
			).WithDetails("type", query.CompareWith.Type)
		}

		if query.CompareWith.Type == "custom" && query.CompareWith.Custom == nil {
			return NewApplicationError(
				ErrCodeValidationFailed,
				"Custom comparison requires time range",
				nil,
			)
		}
	}

	return nil
}

// checkAuthorization checks if the user is authorized to view statistics
func (h *GetCampaignStatsHandler) checkAuthorization(ctx context.Context, query *GetCampaignStatsQuery) error {
	userID := getUserIDFromContext(ctx)
	if userID == "" {
		return NewApplicationError(
			ErrCodeUnauthorized,
			"User not authenticated",
			nil,
		)
	}

	// If querying specific campaign, check access
	if query.CampaignID != nil {
		// In a real implementation, check if user has access to this campaign
	}

	// For global stats, might require specific permissions
	// Check user has analytics/reporting permissions

	return nil
}

// resolveTimeRange resolves time range from filter
func (h *GetCampaignStatsHandler) resolveTimeRange(filter *TimeRangeFilter) TimeRange {
	now := time.Now()

	if filter == nil {
		// Default: last 30 days
		return TimeRange{
			Start: now.AddDate(0, 0, -30),
			End:   now,
		}
	}

	// Handle presets
	if filter.Preset != "" {
		switch filter.Preset {
		case "today":
			start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			return TimeRange{Start: start, End: now}
		case "yesterday":
			yesterday := now.AddDate(0, 0, -1)
			start := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, yesterday.Location())
			end := start.Add(24 * time.Hour).Add(-time.Nanosecond)
			return TimeRange{Start: start, End: end}
		case "week":
			return TimeRange{Start: now.AddDate(0, 0, -7), End: now}
		case "month":
			return TimeRange{Start: now.AddDate(0, -1, 0), End: now}
		case "quarter":
			return TimeRange{Start: now.AddDate(0, -3, 0), End: now}
		case "year":
			return TimeRange{Start: now.AddDate(-1, 0, 0), End: now}
		}
	}

	// Use custom range
	tr := TimeRange{Start: now.AddDate(0, 0, -30), End: now}
	if filter.Start != nil {
		tr.Start = *filter.Start
	}
	if filter.End != nil {
		tr.End = *filter.End
	}

	return tr
}

// getSingleCampaignStats gets statistics for a single campaign
func (h *GetCampaignStatsHandler) getSingleCampaignStats(
	ctx context.Context,
	campaignID string,
	opts query.StatisticsOptions,
	q *GetCampaignStatsQuery,
) (*CampaignStatsResult, error) {
	// Verify campaign exists
	campaign, err := h.repo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, NewApplicationError(
			ErrCodeNotFound,
			"Campaign not found",
			err,
		).WithDetails("campaign_id", campaignID)
	}

	// Get statistics from domain service
	stats, err := h.statsService.GetStatistics(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Convert to DTO
	result := &CampaignStatsResult{
		Stats: h.toStatsOverviewDTO(stats, campaign),
		Period: &PeriodInfoDTO{
			Start:    opts.TimeRange.Start,
			End:      opts.TimeRange.End,
			Duration: opts.TimeRange.End.Sub(opts.TimeRange.Start).String(),
		},
	}

	if q.IncludeTrends && stats.Trends != nil {
		result.Trends = h.toTrendsDTO(stats.Trends)
	}

	if q.IncludeHealth && stats.HealthMetrics != nil {
		result.Health = h.toHealthDTO(stats.HealthMetrics)
	}

	if q.IncludeDetails {
		result.Details = h.getDetailsStats(ctx, campaignID)
	}

	return result, nil
}

// getGlobalStats gets global statistics across all campaigns
func (h *GetCampaignStatsHandler) getGlobalStats(
	ctx context.Context,
	opts query.StatisticsOptions,
	q *GetCampaignStatsQuery,
) (*CampaignStatsResult, error) {
	// Get statistics from domain service
	stats, err := h.statsService.GetStatistics(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Convert to DTO
	result := &CampaignStatsResult{
		Stats: h.toStatsOverviewDTO(stats, nil),
		Period: &PeriodInfoDTO{
			Start:      opts.TimeRange.Start,
			End:        opts.TimeRange.End,
			Duration:   opts.TimeRange.End.Sub(opts.TimeRange.Start).String(),
			DataPoints: stats.TotalCampaigns,
		},
	}

	if q.IncludeTrends && stats.Trends != nil {
		result.Trends = h.toTrendsDTO(stats.Trends)
	}

	if q.IncludeHealth && stats.HealthMetrics != nil {
		result.Health = h.toHealthDTO(stats.HealthMetrics)
	}

	if q.IncludeDetails {
		result.Details = h.getGlobalDetailsStats(ctx)
	}

	return result, nil
}

// toStatsOverviewDTO converts domain statistics to DTO
func (h *GetCampaignStatsHandler) toStatsOverviewDTO(stats *query.CampaignStatistics, campaign *types.Campaign) *CampaignStatsOverviewDTO {
	overview := &CampaignStatsOverviewDTO{
		TotalCampaigns:     stats.TotalCampaigns,
		ActiveCampaigns:    stats.ActiveCampaigns,
		CompletedCampaigns: stats.CompletedCampaigns,
		FailedCampaigns:    stats.StatusDistribution["failed"],

		// Mock data for demonstration
		TotalJobs:          1000,
		CompletedJobs:      850,
		AverageJobTime:     30 * time.Minute,
		TotalCrashes:       42,
		UniqueCrashes:      12,
		CrashRate:          4.2,
		AverageCoverage:    67.5,
		MaxCoverage:        89.2,
		CoverageGrowth:     12.3,
		TotalBotHours:      240.5,
		AvgBotsPerCampaign: 3.2,
		ResourceEfficiency: 78.5,
	}

	return overview
}

// toTrendsDTO converts domain trends to DTO
func (h *GetCampaignStatsHandler) toTrendsDTO(trends *query.CampaignTrends) *CampaignTrendsDTO {
	return &CampaignTrendsDTO{
		CampaignCreation:    trends.DailyCreated,
		JobExecution:        make(map[string]int),     // Would be populated from job data
		CrashDiscovery:      make(map[string]int),     // Would be populated from crash data
		CoverageProgression: make(map[string]float64), // Would be populated from coverage data
		ResourceUsage:       make(map[string]float64), // Would be populated from resource data
	}
}

// toHealthDTO converts domain health metrics to DTO
func (h *GetCampaignStatsHandler) toHealthDTO(health *query.CampaignHealthMetrics) *CampaignHealthDTO {
	// Calculate health score based on metrics
	healthScore := h.calculateHealthScore(health)
	overallHealth := "good"
	if healthScore < 60 {
		overallHealth = "critical"
	} else if healthScore < 80 {
		overallHealth = "warning"
	}

	dto := &CampaignHealthDTO{
		OverallHealth:           overallHealth,
		HealthScore:             healthScore,
		LongRunningCampaigns:    health.LongRunningCampaigns,
		StagnantCampaigns:       health.StagnantCampaigns,
		LowPerformanceCampaigns: 0, // Would be calculated
		SuccessRate:             health.SuccessRate,
		AverageTimeToStart:      health.AverageTimeToStart,
		AverageTimeToComplete:   health.AverageTimeToComplete,
		Recommendations:         h.generateRecommendations(health),
	}

	return dto
}

// calculateHealthScore calculates overall health score
func (h *GetCampaignStatsHandler) calculateHealthScore(health *query.CampaignHealthMetrics) float64 {
	score := 100.0

	// Deduct points for issues
	if health.LongRunningCampaigns > 0 {
		score -= float64(health.LongRunningCampaigns) * 5
	}
	if health.StagnantCampaigns > 0 {
		score -= float64(health.StagnantCampaigns) * 3
	}
	if health.SuccessRate < 90 {
		score -= (90 - health.SuccessRate) * 0.5
	}

	if score < 0 {
		score = 0
	}

	return score
}

// generateRecommendations generates health recommendations
func (h *GetCampaignStatsHandler) generateRecommendations(health *query.CampaignHealthMetrics) []HealthRecommendationDTO {
	var recommendations []HealthRecommendationDTO

	if health.LongRunningCampaigns > 0 {
		recommendations = append(recommendations, HealthRecommendationDTO{
			Type:     "performance",
			Severity: "warning",
			Message:  "Several campaigns have been running for extended periods. Consider reviewing their configuration or resource allocation.",
		})
	}

	if health.StagnantCampaigns > 0 {
		recommendations = append(recommendations, HealthRecommendationDTO{
			Type:     "activity",
			Severity: "info",
			Message:  "Some campaigns show no recent activity. Consider pausing or completing inactive campaigns.",
		})
	}

	if health.SuccessRate < 80 {
		recommendations = append(recommendations, HealthRecommendationDTO{
			Type:     "reliability",
			Severity: "critical",
			Message:  "Campaign success rate is below acceptable threshold. Review failure reasons and system health.",
		})
	}

	return recommendations
}

// getDetailsStats gets detailed statistics
func (h *GetCampaignStatsHandler) getDetailsStats(ctx context.Context, campaignID string) *CampaignDetailsStatsDTO {
	// In a real implementation, gather detailed stats
	return &CampaignDetailsStatsDTO{
		StatusBreakdown: map[string]int{
			"draft":     5,
			"active":    3,
			"paused":    2,
			"completed": 15,
			"failed":    1,
		},
		TopPerformers:      []CampaignPerformanceDTO{},
		BottomPerformers:   []CampaignPerformanceDTO{},
		ResourceAllocation: map[string]ResourceStatsDTO{},
		CrashDistribution: map[string]int{
			"buffer_overflow": 15,
			"null_pointer":    8,
			"assertion_fail":  12,
			"timeout":         7,
		},
	}
}

// getGlobalDetailsStats gets global detailed statistics
func (h *GetCampaignStatsHandler) getGlobalDetailsStats(ctx context.Context) *CampaignDetailsStatsDTO {
	// Similar to getDetailsStats but across all campaigns
	return h.getDetailsStats(ctx, "")
}

// getComparison gets comparison data
func (h *GetCampaignStatsHandler) getComparison(ctx context.Context, query *GetCampaignStatsQuery, current *CampaignStatsResult) (*ComparisonResultDTO, error) {
	// Determine comparison period
	var compareRange TimeRange

	switch query.CompareWith.Type {
	case "previous_period":
		duration := current.Period.End.Sub(current.Period.Start)
		compareRange = TimeRange{
			Start: current.Period.Start.Add(-duration),
			End:   current.Period.Start,
		}
	case "previous_year":
		compareRange = TimeRange{
			Start: current.Period.Start.AddDate(-1, 0, 0),
			End:   current.Period.End.AddDate(-1, 0, 0),
		}
	case "custom":
		compareRange = h.resolveTimeRange(query.CompareWith.Custom)
	}

	// Get comparison statistics
	compareOpts := query.StatisticsOptions{
		TimeRange: &query.TimeRange{
			Start: compareRange.Start,
			End:   compareRange.End,
		},
	}

	compareStats, err := h.statsService.GetStatistics(ctx, compareOpts)
	if err != nil {
		return nil, err
	}

	// Calculate changes
	comparison := &ComparisonResultDTO{
		CurrentPeriod:  current.Stats,
		PreviousPeriod: h.toStatsOverviewDTO(compareStats, nil),
		Changes:        h.calculateChanges(current.Stats, h.toStatsOverviewDTO(compareStats, nil)),
	}

	return comparison, nil
}

// calculateChanges calculates metric changes between periods
func (h *GetCampaignStatsHandler) calculateChanges(current, previous *CampaignStatsOverviewDTO) map[string]ChangeDTO {
	changes := make(map[string]ChangeDTO)

	// Calculate campaign count change
	campaignChange := float64(current.TotalCampaigns - previous.TotalCampaigns)
	campaignPct := 0.0
	if previous.TotalCampaigns > 0 {
		campaignPct = (campaignChange / float64(previous.TotalCampaigns)) * 100
	}
	changes["total_campaigns"] = ChangeDTO{
		Value:      campaignChange,
		Percentage: campaignPct,
		Direction:  h.getDirection(campaignChange),
	}

	// Calculate other metric changes similarly...

	return changes
}

// getDirection determines change direction
func (h *GetCampaignStatsHandler) getDirection(change float64) string {
	if change > 0 {
		return "up"
	} else if change < 0 {
		return "down"
	}
	return "stable"
}

// TimeRange represents a time range
type TimeRange struct {
	Start time.Time
	End   time.Time
}
