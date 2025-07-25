package query

import (
	"context"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
)

// ExampleUsage demonstrates how to use the campaign query services
func ExampleUsage(repo repository.CampaignRepository) {
	ctx := context.Background()

	// Initialize query services
	finder := NewCampaignFinder(repo)
	stats := NewCampaignStatisticsService(repo)

	// Example 1: Find a specific campaign by ID
	campaign, err := finder.FindByID(ctx, "campaign-123")
	if err != nil {
		fmt.Printf("Error finding campaign: %v\n", err)
		return
	}
	if campaign != nil {
		fmt.Printf("Found campaign: %s (Status: %s)\n", campaign.Name, campaign.Status)
	}

	// Example 2: Find all active campaigns
	activeCampaigns, err := finder.FindActive(ctx)
	if err != nil {
		fmt.Printf("Error finding active campaigns: %v\n", err)
		return
	}
	fmt.Printf("Active campaigns: %d\n", len(activeCampaigns))

	// Example 3: Search campaigns by name
	searchResults, err := finder.FindByName(ctx, "test", 10)
	if err != nil {
		fmt.Printf("Error searching campaigns: %v\n", err)
		return
	}
	fmt.Printf("Found %d campaigns matching 'test'\n", len(searchResults))

	// Example 4: List campaigns with pagination
	page := 1
	pageSize := 20
	campaignList, err := finder.List(ctx, page, pageSize)
	if err != nil {
		fmt.Printf("Error listing campaigns: %v\n", err)
		return
	}
	fmt.Printf("Page %d of %d (Total campaigns: %d)\n",
		campaignList.CurrentPage, campaignList.TotalPages, campaignList.TotalCount)

	// Example 5: Complex filtering
	filter := FilterOptions{
		Status:       types.StateActive,
		CreatedAfter: time.Now().AddDate(0, -1, 0), // Last month
		OrderBy:      "created_at",
		Ascending:    false,
		Limit:        50,
	}
	filteredCampaigns, err := finder.FindWithFilter(ctx, filter)
	if err != nil {
		fmt.Printf("Error filtering campaigns: %v\n", err)
		return
	}
	fmt.Printf("Found %d campaigns matching filter criteria\n", len(filteredCampaigns))

	// Example 6: Get recently updated campaigns
	recentlyUpdated, err := finder.GetRecentlyUpdated(ctx, 24*time.Hour)
	if err != nil {
		fmt.Printf("Error finding recently updated campaigns: %v\n", err)
		return
	}
	fmt.Printf("Campaigns updated in last 24 hours: %d\n", len(recentlyUpdated))

	// Example 7: Get campaign statistics
	statsOpts := StatisticsOptions{
		IncludeTrends:        true,
		IncludeHealthMetrics: true,
		CacheResults:         true,
	}
	statistics, err := stats.GetStatistics(ctx, statsOpts)
	if err != nil {
		fmt.Printf("Error getting statistics: %v\n", err)
		return
	}
	fmt.Printf("Total campaigns: %d\n", statistics.TotalCampaigns)
	fmt.Printf("Active campaigns: %d\n", statistics.ActiveCampaigns)
	fmt.Printf("Completion rate: %.2f%%\n", statistics.CompletionRate)

	// Example 8: Get status distribution
	statusDist, err := stats.GetStatusDistribution(ctx)
	if err != nil {
		fmt.Printf("Error getting status distribution: %v\n", err)
		return
	}
	for status, count := range statusDist {
		fmt.Printf("  %s: %d\n", status, count)
	}

	// Example 9: Get activity metrics for the last week
	activityMetrics, err := stats.GetActivityMetrics(ctx, 7*24*time.Hour)
	if err != nil {
		fmt.Printf("Error getting activity metrics: %v\n", err)
		return
	}
	fmt.Printf("Campaigns created this week: %d\n", activityMetrics.CampaignsCreated)
	fmt.Printf("Campaigns completed this week: %d\n", activityMetrics.CampaignsCompleted)

	// Example 10: Get top campaigns
	topCampaigns, err := stats.GetTopCampaigns(ctx, "longest_running", 5)
	if err != nil {
		fmt.Printf("Error getting top campaigns: %v\n", err)
		return
	}
	fmt.Println("Top 5 longest running campaigns:")
	for i, c := range topCampaigns {
		fmt.Printf("  %d. %s\n", i+1, c.Name)
	}

	// Example 11: Clear cache when needed
	finder.ClearCache()
}

