package query

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
)

// CampaignDTO represents a read-only view of a campaign
type CampaignDTO struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	IsActive    bool              `json:"is_active"`
	CanModify   bool              `json:"can_modify"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// CampaignListDTO represents a paginated list of campaigns
type CampaignListDTO struct {
	Campaigns   []*CampaignDTO `json:"campaigns"`
	TotalCount  int            `json:"total_count"`
	CurrentPage int            `json:"current_page"`
	TotalPages  int            `json:"total_pages"`
	PageSize    int            `json:"page_size"`
}

// FilterOptions represents campaign filter criteria
type FilterOptions struct {
	Name          string
	Status        types.State
	Active        *bool
	CreatedBefore time.Time
	CreatedAfter  time.Time
	UpdatedBefore time.Time
	UpdatedAfter  time.Time
	OrderBy       string // "created_at", "updated_at", "name"
	Ascending     bool
	Limit         int
	Offset        int
}

// CampaignFinder provides optimized read operations for campaigns
type CampaignFinder struct {
	repo  repository.CampaignRepository
	cache *campaignCache
	mu    sync.RWMutex
}

// campaignCache provides simple in-memory caching
type campaignCache struct {
	campaigns map[string]*cacheEntry
	ttl       time.Duration
	mu        sync.RWMutex
}

type cacheEntry struct {
	dto       *CampaignDTO
	expiresAt time.Time
}

// NewCampaignFinder creates a new campaign finder service
func NewCampaignFinder(repo repository.CampaignRepository) *CampaignFinder {
	return &CampaignFinder{
		repo: repo,
		cache: &campaignCache{
			campaigns: make(map[string]*cacheEntry),
			ttl:       5 * time.Minute, // Default cache TTL
		},
	}
}

// FindByID retrieves a campaign by ID with caching
func (f *CampaignFinder) FindByID(ctx context.Context, id string) (*CampaignDTO, error) {
	// Check cache first
	if dto := f.cache.get(id); dto != nil {
		return dto, nil
	}

	// Fetch from repository
	campaign, err := f.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding campaign by ID: %w", err)
	}

	dto := campaignToDTO(campaign)
	f.cache.set(id, dto)
	return dto, nil
}

// FindByName searches campaigns by name with partial matching
func (f *CampaignFinder) FindByName(ctx context.Context, name string, limit int) ([]*CampaignDTO, error) {
	campaigns, err := f.repo.FindByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("finding campaigns by name: %w", err)
	}

	// Apply limit if specified
	if limit > 0 && len(campaigns) > limit {
		campaigns = campaigns[:limit]
	}

	return campaignsToDTOs(campaigns), nil
}

// FindActive retrieves all active campaigns
func (f *CampaignFinder) FindActive(ctx context.Context) ([]*CampaignDTO, error) {
	campaigns, err := f.repo.FindActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("finding active campaigns: %w", err)
	}

	return campaignsToDTOs(campaigns), nil
}

// FindByStatus retrieves campaigns by status
func (f *CampaignFinder) FindByStatus(ctx context.Context, status types.State) ([]*CampaignDTO, error) {
	campaigns, err := f.repo.FindByStatus(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("finding campaigns by status: %w", err)
	}

	return campaignsToDTOs(campaigns), nil
}

// List retrieves campaigns with pagination
func (f *CampaignFinder) List(ctx context.Context, page, pageSize int) (*CampaignListDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize
	campaigns, total, err := f.repo.List(ctx, offset, pageSize)
	if err != nil {
		return nil, fmt.Errorf("listing campaigns: %w", err)
	}

	totalPages := (total + pageSize - 1) / pageSize

	return &CampaignListDTO{
		Campaigns:   campaignsToDTOs(campaigns),
		TotalCount:  total,
		CurrentPage: page,
		TotalPages:  totalPages,
		PageSize:    pageSize,
	}, nil
}

// FindWithFilter retrieves campaigns matching complex filter criteria
func (f *CampaignFinder) FindWithFilter(ctx context.Context, filter FilterOptions) ([]*CampaignDTO, error) {
	// In a real implementation, this would use a more sophisticated query builder
	// For now, we'll use the basic repository methods and filter in memory
	campaigns, _, err := f.repo.List(ctx, 0, 1000) // Get all campaigns
	if err != nil {
		return nil, fmt.Errorf("finding campaigns with filter: %w", err)
	}

	filtered := make([]*types.Campaign, 0)
	for _, campaign := range campaigns {
		if matchesFilter(campaign, filter) {
			filtered = append(filtered, campaign)
		}
	}

	// Apply ordering
	sortCampaigns(filtered, filter.OrderBy, filter.Ascending)

	// Apply pagination
	start := filter.Offset
	end := start + filter.Limit
	if start > len(filtered) {
		return []*CampaignDTO{}, nil
	}
	if end > len(filtered) {
		end = len(filtered)
	}

	if filter.Limit > 0 {
		filtered = filtered[start:end]
	}

	return campaignsToDTOs(filtered), nil
}

// Exists checks if a campaign exists by ID
func (f *CampaignFinder) Exists(ctx context.Context, id string) (bool, error) {
	return f.repo.Exists(ctx, id)
}

// GetRecentlyUpdated retrieves campaigns updated within the specified duration
func (f *CampaignFinder) GetRecentlyUpdated(ctx context.Context, since time.Duration) ([]*CampaignDTO, error) {
	filter := FilterOptions{
		UpdatedAfter: time.Now().Add(-since),
		OrderBy:      "updated_at",
		Ascending:    false,
	}
	return f.FindWithFilter(ctx, filter)
}

// ClearCache clears the campaign cache
func (f *CampaignFinder) ClearCache() {
	f.cache.clear()
}

// Helper functions

func campaignToDTO(campaign *types.Campaign) *CampaignDTO {
	return &CampaignDTO{
		ID:          campaign.ID,
		Name:        campaign.Name,
		Description: campaign.Description,
		Status:      campaign.Status.String(),
		CreatedAt:   campaign.CreatedAt,
		UpdatedAt:   campaign.UpdatedAt,
		IsActive:    campaign.IsActive(),
		CanModify:   campaign.CanBeModified(),
		Metadata:    make(map[string]string), // Placeholder for future metadata
	}
}

func campaignsToDTOs(campaigns []*types.Campaign) []*CampaignDTO {
	dtos := make([]*CampaignDTO, len(campaigns))
	for i, campaign := range campaigns {
		dtos[i] = campaignToDTO(campaign)
	}
	return dtos
}

func matchesFilter(campaign *types.Campaign, filter FilterOptions) bool {
	// Name filter (partial match)
	if filter.Name != "" && !contains(campaign.Name, filter.Name) {
		return false
	}

	// Status filter
	if filter.Status != "" && campaign.Status != filter.Status {
		return false
	}

	// Active filter
	if filter.Active != nil && campaign.IsActive() != *filter.Active {
		return false
	}

	// Date filters
	if !filter.CreatedAfter.IsZero() && campaign.CreatedAt.Before(filter.CreatedAfter) {
		return false
	}
	if !filter.CreatedBefore.IsZero() && campaign.CreatedAt.After(filter.CreatedBefore) {
		return false
	}
	if !filter.UpdatedAfter.IsZero() && campaign.UpdatedAt.Before(filter.UpdatedAfter) {
		return false
	}
	if !filter.UpdatedBefore.IsZero() && campaign.UpdatedAt.After(filter.UpdatedBefore) {
		return false
	}

	return true
}

func sortCampaigns(campaigns []*types.Campaign, orderBy string, ascending bool) {
	// Simple sorting implementation
	// In production, use a proper sorting library
	switch orderBy {
	case "name":
		sortByName(campaigns, ascending)
	case "created_at":
		sortByCreatedAt(campaigns, ascending)
	case "updated_at":
		sortByUpdatedAt(campaigns, ascending)
	}
}

func sortByName(campaigns []*types.Campaign, ascending bool) {
	for i := 0; i < len(campaigns)-1; i++ {
		for j := i + 1; j < len(campaigns); j++ {
			swap := false
			if ascending {
				swap = campaigns[i].Name > campaigns[j].Name
			} else {
				swap = campaigns[i].Name < campaigns[j].Name
			}
			if swap {
				campaigns[i], campaigns[j] = campaigns[j], campaigns[i]
			}
		}
	}
}

func sortByCreatedAt(campaigns []*types.Campaign, ascending bool) {
	for i := 0; i < len(campaigns)-1; i++ {
		for j := i + 1; j < len(campaigns); j++ {
			swap := false
			if ascending {
				swap = campaigns[i].CreatedAt.After(campaigns[j].CreatedAt)
			} else {
				swap = campaigns[i].CreatedAt.Before(campaigns[j].CreatedAt)
			}
			if swap {
				campaigns[i], campaigns[j] = campaigns[j], campaigns[i]
			}
		}
	}
}

func sortByUpdatedAt(campaigns []*types.Campaign, ascending bool) {
	for i := 0; i < len(campaigns)-1; i++ {
		for j := i + 1; j < len(campaigns); j++ {
			swap := false
			if ascending {
				swap = campaigns[i].UpdatedAt.After(campaigns[j].UpdatedAt)
			} else {
				swap = campaigns[i].UpdatedAt.Before(campaigns[j].UpdatedAt)
			}
			if swap {
				campaigns[i], campaigns[j] = campaigns[j], campaigns[i]
			}
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Cache implementation

func (c *campaignCache) get(id string) *CampaignDTO {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.campaigns[id]
	if !exists || time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.dto
}

func (c *campaignCache) set(id string, dto *CampaignDTO) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.campaigns[id] = &cacheEntry{
		dto:       dto,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *campaignCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.campaigns = make(map[string]*cacheEntry)
}

