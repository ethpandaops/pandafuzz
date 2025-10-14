package handlers

import (
	"net/http"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// HandleGetAnalytics handles GET /api/v1/analytics
func (h *Handlers) HandleGetAnalytics(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	params := generated.GetAnalyticsParams{}

	if timeRange := r.URL.Query().Get("time_range"); timeRange != "" {
		timeRangeVal := generated.GetAnalyticsParamsTimeRange(timeRange)
		params.TimeRange = &timeRangeVal
	}

	if campaignId := r.URL.Query().Get("campaign_id"); campaignId != "" {
		// Parse UUID
		if uuid, err := parseUUID(campaignId); err == nil {
			params.CampaignId = &uuid
		}
	}

	if metrics := r.URL.Query().Get("metrics"); metrics != "" {
		params.Metrics = &metrics
	}

	// Delegate to adapter
	h.adapter.GetAnalytics(w, r, params)
}

// HandleGetMetrics handles GET /api/v1/analytics/metrics
func (h *Handlers) HandleGetMetrics(w http.ResponseWriter, r *http.Request) {
	h.adapter.GetMetrics(w, r)
}

// HandleGetCoverageTrends handles GET /api/v1/analytics/coverage
func (h *Handlers) HandleGetCoverageTrends(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	params := generated.GetCoverageTrendsParams{}

	if timeRange := r.URL.Query().Get("time_range"); timeRange != "" {
		timeRangeVal := generated.GetCoverageTrendsParamsTimeRange(timeRange)
		params.TimeRange = &timeRangeVal
	}

	if granularity := r.URL.Query().Get("granularity"); granularity != "" {
		granularityVal := generated.GetCoverageTrendsParamsGranularity(granularity)
		params.Granularity = &granularityVal
	}

	if campaignId := r.URL.Query().Get("campaign_id"); campaignId != "" {
		// Parse UUID
		if uuid, err := parseUUID(campaignId); err == nil {
			params.CampaignId = &uuid
		}
	}

	// Delegate to adapter
	h.adapter.GetCoverageTrends(w, r, params)
}

// HandleGetPerformanceStats handles GET /api/v1/analytics/performance
func (h *Handlers) HandleGetPerformanceStats(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	params := generated.GetPerformanceStatsParams{}

	if timeRange := r.URL.Query().Get("time_range"); timeRange != "" {
		timeRangeVal := generated.GetPerformanceStatsParamsTimeRange(timeRange)
		params.TimeRange = &timeRangeVal
	}

	if component := r.URL.Query().Get("component"); component != "" {
		componentVal := generated.GetPerformanceStatsParamsComponent(component)
		params.Component = &componentVal
	}

	// Delegate to adapter
	h.adapter.GetPerformanceStats(w, r, params)
}

// parseUUID is a helper function to parse UUID strings
func parseUUID(s string) (openapi_types.UUID, error) {
	// Parse and validate UUID format
	parsedUUID, err := uuid.Parse(s)
	if err != nil {
		return openapi_types.UUID{}, err
	}
	return openapi_types.UUID(parsedUUID), nil
}
