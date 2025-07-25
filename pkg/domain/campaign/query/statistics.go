package query

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
)

// CampaignStatistics represents aggregated campaign statistics
type CampaignStatistics struct {
	TotalCampaigns     int                    `json:"total_campaigns"`
	StatusDistribution map[string]int         `json:"status_distribution"`
	ActiveCampaigns    int                    `json:"active_campaigns"`
	CompletedCampaigns int                    `json:"completed_campaigns"`
	AverageDuration    time.Duration          `json:"average_duration"`
	CreatedToday       int                    `json:"created_today"`
	CreatedThisWeek    int                    `json:"created_this_week"`
	CreatedThisMonth   int                    `json:"created_this_month"`
	UpdatedRecently    int                    `json:"updated_recently"`
	CompletionRate     float64                `json:"completion_rate"`
	Trends             *CampaignTrends        `json:"trends,omitempty"`
	HealthMetrics      *CampaignHealthMetrics `json:"health_metrics,omitempty"`
}

// CampaignTrends represents campaign activity trends
type CampaignTrends struct {
	DailyCreated   map[string]int `json:"daily_created"`   // Last 30 days
	DailyCompleted map[string]int `json:"daily_completed"` // Last 30 days
	WeeklyActive   map[string]int `json:"weekly_active"`   // Last 12 weeks
	MonthlyGrowth  float64        `json:"monthly_growth"`  // Percentage
}

// CampaignHealthMetrics represents health and performance metrics
type CampaignHealthMetrics struct {
	LongRunningCampaigns  int           `json:"long_running_campaigns"` // Running > 7 days
	StagnantCampaigns     int           `json:"stagnant_campaigns"`     // Not updated > 3 days
	SuccessRate           float64       `json:"success_rate"`           // Completed without issues
	AverageTimeToStart    time.Duration `json:"average_time_to_start"`
	AverageTimeToComplete time.Duration `json:"average_time_to_complete"`
}

// TimeRange represents a time period for statistics
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// StatisticsOptions configures what statistics to calculate
type StatisticsOptions struct {
	IncludeTrends        bool
	IncludeHealthMetrics bool
	TimeRange            *TimeRange
	CacheResults         bool
}

// CampaignStatisticsService provides campaign statistics and analytics
type CampaignStatisticsService struct {
	repo       repository.CampaignRepository
	statsCache *statisticsCache
	mu         sync.RWMutex
}

// statisticsCache provides caching for expensive statistics calculations
type statisticsCache struct {
	stats     *CampaignStatistics
	expiresAt time.Time
	mu        sync.RWMutex
}

// NewCampaignStatisticsService creates a new statistics service
func NewCampaignStatisticsService(repo repository.CampaignRepository) *CampaignStatisticsService {
	return &CampaignStatisticsService{
		repo:       repo,
		statsCache: &statisticsCache{},
	}
}

// GetStatistics retrieves comprehensive campaign statistics
func (s *CampaignStatisticsService) GetStatistics(ctx context.Context, opts StatisticsOptions) (*CampaignStatistics, error) {
	// Check cache if enabled
	if opts.CacheResults {
		if cached := s.statsCache.get(); cached != nil {
			return cached, nil
		}
	}

	stats := &CampaignStatistics{
		StatusDistribution: make(map[string]int),
	}

	// Calculate basic statistics
	if err := s.calculateBasicStats(ctx, stats, opts.TimeRange); err != nil {
		return nil, fmt.Errorf("calculating basic stats: %w", err)
	}

	// Calculate trends if requested
	if opts.IncludeTrends {
		trends, err := s.calculateTrends(ctx, opts.TimeRange)
		if err != nil {
			return nil, fmt.Errorf("calculating trends: %w", err)
		}
		stats.Trends = trends
	}

	// Calculate health metrics if requested
	if opts.IncludeHealthMetrics {
		health, err := s.calculateHealthMetrics(ctx)
		if err != nil {
			return nil, fmt.Errorf("calculating health metrics: %w", err)
		}
		stats.HealthMetrics = health
	}

	// Cache results if enabled
	if opts.CacheResults {
		s.statsCache.set(stats, 10*time.Minute)
	}

	return stats, nil
}

// GetStatusDistribution returns campaign count by status
func (s *CampaignStatisticsService) GetStatusDistribution(ctx context.Context) (map[string]int, error) {
	distribution := make(map[string]int)

	for _, status := range types.AllStates() {
		count, err := s.repo.CountByStatus(ctx, status)
		if err != nil {
			return nil, fmt.Errorf("counting campaigns for status %s: %w", status, err)
		}
		distribution[status.String()] = count
	}

	return distribution, nil
}

// GetActivityMetrics returns campaign activity metrics for a time period
func (s *CampaignStatisticsService) GetActivityMetrics(ctx context.Context, period time.Duration) (*ActivityMetrics, error) {
	since := time.Now().Add(-period)

	// Get all campaigns to analyze activity
	campaigns, _, err := s.repo.List(ctx, 0, 10000) // Get all campaigns
	if err != nil {
		return nil, fmt.Errorf("listing campaigns: %w", err)
	}

	metrics := &ActivityMetrics{
		Period:             period,
		CampaignsCreated:   0,
		CampaignsStarted:   0,
		CampaignsCompleted: 0,
		AverageLifespan:    0,
	}

	var totalLifespan time.Duration
	completedCount := 0

	for _, campaign := range campaigns {
		// Count created in period
		if campaign.CreatedAt.After(since) {
			metrics.CampaignsCreated++
		}

		// Count started (transitioned to active) in period
		if campaign.Status == types.StateActive && campaign.UpdatedAt.After(since) {
			metrics.CampaignsStarted++
		}

		// Count completed in period
		if campaign.Status == types.StateCompleted && campaign.UpdatedAt.After(since) {
			metrics.CampaignsCompleted++
			lifespan := campaign.UpdatedAt.Sub(campaign.CreatedAt)
			totalLifespan += lifespan
			completedCount++
		}
	}

	if completedCount > 0 {
		metrics.AverageLifespan = totalLifespan / time.Duration(completedCount)
	}

	return metrics, nil
}

// GetTopCampaigns returns top campaigns by various criteria
func (s *CampaignStatisticsService) GetTopCampaigns(ctx context.Context, criteria string, limit int) ([]*CampaignDTO, error) {
	campaigns, _, err := s.repo.List(ctx, 0, 10000) // Get all campaigns
	if err != nil {
		return nil, fmt.Errorf("listing campaigns: %w", err)
	}

	// Sort by criteria
	switch criteria {
	case "longest_running":
		sortByRunningDuration(campaigns)
	case "recently_updated":
		sortByUpdatedAt(campaigns, false)
	case "recently_created":
		sortByCreatedAt(campaigns, false)
	default:
		return nil, fmt.Errorf("unknown criteria: %s", criteria)
	}

	// Apply limit
	if limit > 0 && len(campaigns) > limit {
		campaigns = campaigns[:limit]
	}

	return campaignsToDTOs(campaigns), nil
}

// calculateBasicStats calculates basic campaign statistics
func (s *CampaignStatisticsService) calculateBasicStats(ctx context.Context, stats *CampaignStatistics, timeRange *TimeRange) error {
	// Get all campaigns
	campaigns, total, err := s.repo.List(ctx, 0, 10000)
	if err != nil {
		return err
	}

	stats.TotalCampaigns = total

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	recentCutoff := now.Add(-24 * time.Hour)

	var totalDuration time.Duration
	completedCount := 0

	for _, campaign := range campaigns {
		// Apply time range filter if specified
		if timeRange != nil {
			if campaign.CreatedAt.Before(timeRange.Start) || campaign.CreatedAt.After(timeRange.End) {
				continue
			}
		}

		// Status distribution
		status := campaign.Status.String()
		stats.StatusDistribution[status]++

		// Active campaigns
		if campaign.IsActive() {
			stats.ActiveCampaigns++
		}

		// Completed campaigns
		if campaign.Status == types.StateCompleted {
			stats.CompletedCampaigns++
			duration := campaign.UpdatedAt.Sub(campaign.CreatedAt)
			totalDuration += duration
			completedCount++
		}

		// Created today/this week/this month
		if campaign.CreatedAt.After(todayStart) {
			stats.CreatedToday++
		}
		if campaign.CreatedAt.After(weekStart) {
			stats.CreatedThisWeek++
		}
		if campaign.CreatedAt.After(monthStart) {
			stats.CreatedThisMonth++
		}

		// Recently updated
		if campaign.UpdatedAt.After(recentCutoff) {
			stats.UpdatedRecently++
		}
	}

	// Calculate averages
	if completedCount > 0 {
		stats.AverageDuration = totalDuration / time.Duration(completedCount)
		stats.CompletionRate = float64(completedCount) / float64(len(campaigns)) * 100
	}

	return nil
}

// calculateTrends calculates campaign trends over time
func (s *CampaignStatisticsService) calculateTrends(ctx context.Context, timeRange *TimeRange) (*CampaignTrends, error) {
	trends := &CampaignTrends{
		DailyCreated:   make(map[string]int),
		DailyCompleted: make(map[string]int),
		WeeklyActive:   make(map[string]int),
	}

	// Get all campaigns
	campaigns, _, err := s.repo.List(ctx, 0, 10000)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	thirtyDaysAgo := now.AddDate(0, 0, -30)
	twelveWeeksAgo := now.AddDate(0, 0, -84) // 12 weeks

	// Initialize daily maps
	for i := 0; i < 30; i++ {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		trends.DailyCreated[date] = 0
		trends.DailyCompleted[date] = 0
	}

	// Initialize weekly maps
	for i := 0; i < 12; i++ {
		week := now.AddDate(0, 0, -i*7).Format("2006-W01")
		trends.WeeklyActive[week] = 0
	}

	// Count campaigns by date
	for _, campaign := range campaigns {
		// Daily created
		if campaign.CreatedAt.After(thirtyDaysAgo) {
			date := campaign.CreatedAt.Format("2006-01-02")
			trends.DailyCreated[date]++
		}

		// Daily completed
		if campaign.Status == types.StateCompleted && campaign.UpdatedAt.After(thirtyDaysAgo) {
			date := campaign.UpdatedAt.Format("2006-01-02")
			trends.DailyCompleted[date]++
		}

		// Weekly active
		if campaign.Status == types.StateActive && campaign.UpdatedAt.After(twelveWeeksAgo) {
			week := campaign.UpdatedAt.Format("2006-W01")
			trends.WeeklyActive[week]++
		}
	}

	// Calculate monthly growth
	trends.MonthlyGrowth = s.calculateMonthlyGrowth(campaigns)

	return trends, nil
}

// calculateHealthMetrics calculates campaign health and performance metrics
func (s *CampaignStatisticsService) calculateHealthMetrics(ctx context.Context) (*CampaignHealthMetrics, error) {
	health := &CampaignHealthMetrics{}

	// Get all campaigns
	campaigns, _, err := s.repo.List(ctx, 0, 10000)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	sevenDaysAgo := now.AddDate(0, 0, -7)
	threeDaysAgo := now.AddDate(0, 0, -3)

	var totalTimeToStart time.Duration
	var totalTimeToComplete time.Duration
	startCount := 0
	completeCount := 0

	for _, campaign := range campaigns {
		// Long running campaigns
		if campaign.Status == types.StateActive && campaign.UpdatedAt.Before(sevenDaysAgo) {
			health.LongRunningCampaigns++
		}

		// Stagnant campaigns (not completed and not updated recently)
		if campaign.Status != types.StateCompleted && campaign.UpdatedAt.Before(threeDaysAgo) {
			health.StagnantCampaigns++
		}

		// Calculate time metrics
		if campaign.Status != types.StateDraft {
			// Assume transition from draft to active means start
			timeToStart := campaign.UpdatedAt.Sub(campaign.CreatedAt)
			totalTimeToStart += timeToStart
			startCount++
		}

		if campaign.Status == types.StateCompleted {
			timeToComplete := campaign.UpdatedAt.Sub(campaign.CreatedAt)
			totalTimeToComplete += timeToComplete
			completeCount++
		}
	}

	// Calculate averages
	if startCount > 0 {
		health.AverageTimeToStart = totalTimeToStart / time.Duration(startCount)
	}
	if completeCount > 0 {
		health.AverageTimeToComplete = totalTimeToComplete / time.Duration(completeCount)
		health.SuccessRate = float64(completeCount) / float64(len(campaigns)) * 100
	}

	return health, nil
}

// calculateMonthlyGrowth calculates the month-over-month growth rate
func (s *CampaignStatisticsService) calculateMonthlyGrowth(campaigns []*types.Campaign) float64 {
	now := time.Now()
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	lastMonth := thisMonth.AddDate(0, -1, 0)

	var thisMonthCount, lastMonthCount int

	for _, campaign := range campaigns {
		if campaign.CreatedAt.After(thisMonth) {
			thisMonthCount++
		} else if campaign.CreatedAt.After(lastMonth) && campaign.CreatedAt.Before(thisMonth) {
			lastMonthCount++
		}
	}

	if lastMonthCount == 0 {
		return 0
	}

	return float64(thisMonthCount-lastMonthCount) / float64(lastMonthCount) * 100
}

// ActivityMetrics represents campaign activity over a time period
type ActivityMetrics struct {
	Period             time.Duration `json:"period"`
	CampaignsCreated   int           `json:"campaigns_created"`
	CampaignsStarted   int           `json:"campaigns_started"`
	CampaignsCompleted int           `json:"campaigns_completed"`
	AverageLifespan    time.Duration `json:"average_lifespan"`
}

// Helper function to sort campaigns by running duration
func sortByRunningDuration(campaigns []*types.Campaign) {
	now := time.Now()
	for i := 0; i < len(campaigns)-1; i++ {
		for j := i + 1; j < len(campaigns); j++ {
			duration1 := now.Sub(campaigns[i].CreatedAt)
			duration2 := now.Sub(campaigns[j].CreatedAt)
			if campaigns[i].Status != types.StateActive {
				duration1 = 0
			}
			if campaigns[j].Status != types.StateActive {
				duration2 = 0
			}
			if duration1 < duration2 {
				campaigns[i], campaigns[j] = campaigns[j], campaigns[i]
			}
		}
	}
}

// Cache implementation for statistics

func (c *statisticsCache) get() *CampaignStatistics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.stats == nil || time.Now().After(c.expiresAt) {
		return nil
	}
	return c.stats
}

func (c *statisticsCache) set(stats *CampaignStatistics, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stats = stats
	c.expiresAt = time.Now().Add(ttl)
}
