package query

import (
	"context"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
)

// MockCampaignRepository is a simple mock for testing
type MockCampaignRepository struct {
	campaigns []*types.Campaign
}

func (m *MockCampaignRepository) Create(ctx context.Context, campaign *types.Campaign) error {
	m.campaigns = append(m.campaigns, campaign)
	return nil
}

func (m *MockCampaignRepository) Update(ctx context.Context, campaign *types.Campaign) error {
	for i, c := range m.campaigns {
		if c.ID == campaign.ID {
			m.campaigns[i] = campaign
			return nil
		}
	}
	return nil
}

func (m *MockCampaignRepository) Delete(ctx context.Context, id string) error {
	for i, c := range m.campaigns {
		if c.ID == id {
			m.campaigns = append(m.campaigns[:i], m.campaigns[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *MockCampaignRepository) FindByID(ctx context.Context, id string) (*types.Campaign, error) {
	for _, c := range m.campaigns {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, nil
}

func (m *MockCampaignRepository) FindByName(ctx context.Context, name string) ([]*types.Campaign, error) {
	var result []*types.Campaign
	for _, c := range m.campaigns {
		if contains(c.Name, name) {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *MockCampaignRepository) FindByStatus(ctx context.Context, status types.State) ([]*types.Campaign, error) {
	var result []*types.Campaign
	for _, c := range m.campaigns {
		if c.Status == status {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *MockCampaignRepository) FindActive(ctx context.Context) ([]*types.Campaign, error) {
	var result []*types.Campaign
	for _, c := range m.campaigns {
		if c.IsActive() {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *MockCampaignRepository) List(ctx context.Context, offset, limit int) ([]*types.Campaign, int, error) {
	total := len(m.campaigns)
	end := offset + limit
	if end > total {
		end = total
	}
	if offset > total {
		return []*types.Campaign{}, total, nil
	}
	return m.campaigns[offset:end], total, nil
}

func (m *MockCampaignRepository) Exists(ctx context.Context, id string) (bool, error) {
	for _, c := range m.campaigns {
		if c.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func (m *MockCampaignRepository) CountByStatus(ctx context.Context, status types.State) (int, error) {
	count := 0
	for _, c := range m.campaigns {
		if c.Status == status {
			count++
		}
	}
	return count, nil
}

func TestCampaignFinder(t *testing.T) {
	// Create mock repository with test data
	repo := &MockCampaignRepository{
		campaigns: []*types.Campaign{
			{
				ID:          "1",
				Name:        "Test Campaign 1",
				Description: "First test campaign",
				Status:      types.StateActive,
				CreatedAt:   time.Now().Add(-24 * time.Hour),
				UpdatedAt:   time.Now().Add(-1 * time.Hour),
			},
			{
				ID:          "2",
				Name:        "Test Campaign 2",
				Description: "Second test campaign",
				Status:      types.StateDraft,
				CreatedAt:   time.Now().Add(-48 * time.Hour),
				UpdatedAt:   time.Now().Add(-2 * time.Hour),
			},
		},
	}

	finder := NewCampaignFinder(repo)
	ctx := context.Background()

	// Test FindByID
	dto, err := finder.FindByID(ctx, "1")
	if err != nil {
		t.Errorf("FindByID failed: %v", err)
	}
	if dto == nil || dto.ID != "1" {
		t.Error("FindByID returned incorrect result")
	}

	// Test FindActive
	active, err := finder.FindActive(ctx)
	if err != nil {
		t.Errorf("FindActive failed: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("Expected 1 active campaign, got %d", len(active))
	}

	// Test List with pagination
	list, err := finder.List(ctx, 1, 10)
	if err != nil {
		t.Errorf("List failed: %v", err)
	}
	if list.TotalCount != 2 {
		t.Errorf("Expected total count 2, got %d", list.TotalCount)
	}
}

