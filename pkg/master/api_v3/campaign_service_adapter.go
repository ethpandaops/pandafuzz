package api_v3

import (
	"context"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// CampaignServiceAdapter adapts the common.CampaignService interface to match API v3 expectations
type CampaignServiceAdapter struct {
	service common.CampaignService
}

// NewCampaignServiceAdapter creates a new adapter
func NewCampaignServiceAdapter(service common.CampaignService) *CampaignServiceAdapter {
	return &CampaignServiceAdapter{service: service}
}

// ListCampaigns lists campaigns with filters
func (a *CampaignServiceAdapter) ListCampaigns(ctx context.Context, filters common.CampaignFilters) ([]*common.Campaign, error) {
	return a.service.List(ctx, filters)
}

// CreateCampaign creates a new campaign
func (a *CampaignServiceAdapter) CreateCampaign(ctx context.Context, campaign *common.Campaign) (*common.Campaign, error) {
	err := a.service.Create(ctx, campaign)
	if err != nil {
		return nil, err
	}
	return campaign, nil
}

// GetCampaign retrieves a campaign by ID
func (a *CampaignServiceAdapter) GetCampaign(ctx context.Context, id string) (*common.Campaign, error) {
	return a.service.Get(ctx, id)
}

// UpdateCampaign updates a campaign
func (a *CampaignServiceAdapter) UpdateCampaign(ctx context.Context, id string, updates *common.CampaignUpdates) (*common.Campaign, error) {
	err := a.service.Update(ctx, id, *updates)
	if err != nil {
		return nil, err
	}
	return a.service.Get(ctx, id)
}

// DeleteCampaign deletes a campaign
func (a *CampaignServiceAdapter) DeleteCampaign(ctx context.Context, id string) error {
	return a.service.Delete(ctx, id)
}

// GetCampaignStats retrieves campaign statistics
func (a *CampaignServiceAdapter) GetCampaignStats(ctx context.Context, id string) (*common.CampaignStats, error) {
	return a.service.GetStatistics(ctx, id)
}
