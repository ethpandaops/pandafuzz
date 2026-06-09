package mappers

import (
	campaigntypes "github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/models"
)

// DomainCampaignToRow converts a domain Campaign to a database row
func DomainCampaignToRow(campaign *campaigntypes.Campaign) *models.DomainCampaignRow {
	if campaign == nil {
		return nil
	}

	return &models.DomainCampaignRow{
		ID:          campaign.ID,
		Name:        campaign.Name,
		Description: campaign.Description,
		Status:      string(campaign.Status),
		CreatedAt:   campaign.CreatedAt,
		UpdatedAt:   campaign.UpdatedAt,
	}
}

// CampaignRowToDomain converts a database row to a domain Campaign
func CampaignRowToDomain(row *models.DomainCampaignRow) *campaigntypes.Campaign {
	if row == nil {
		return nil
	}

	return &campaigntypes.Campaign{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		Status:      campaigntypes.State(row.Status),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

// CampaignStateStringToDomain converts a string to domain State
func CampaignStateStringToDomain(s string) campaigntypes.State {
	state, err := campaigntypes.ParseState(s)
	if err != nil {
		return campaigntypes.StateDraft
	}
	return state
}
