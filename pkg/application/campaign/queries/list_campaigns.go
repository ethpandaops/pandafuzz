package queries

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/query"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
)

// ListCampaignsQuery represents a query to list campaigns with filtering and pagination
type ListCampaignsQuery struct {
	// Pagination
	Page     int `json:"page" validate:"min=1"`
	PageSize int `json:"page_size" validate:"min=1,max=100"`

	// Filters
	Status    *string `json:"status,omitempty" validate:"omitempty,oneof=draft active paused completed failed"`
	Name      *string `json:"name,omitempty"`
	CreatedBy *string `json:"created_by,omitempty"`

	// Date filters
	CreatedAfter  *time.Time `json:"created_after,omitempty"`
	CreatedBefore *time.Time `json:"created_before,omitempty"`
	UpdatedAfter  *time.Time `json:"updated_after,omitempty"`
	UpdatedBefore *time.Time `json:"updated_before,omitempty"`

	// Sorting
	SortBy    string `json:"sort_by" validate:"oneof=created_at updated_at name status"`
	SortOrder string `json:"sort_order" validate:"oneof=asc desc"`

	// Include options
	IncludeStats bool `json:"include_stats"`
}

// CampaignListItemDTO represents a campaign in a list
type CampaignListItemDTO struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   string    `json:"created_by,omitempty"`

	// Optional stats
	Stats *CampaignQuickStatsDTO `json:"stats,omitempty"`
}

// CampaignQuickStatsDTO represents quick stats for list view
type CampaignQuickStatsDTO struct {
	TotalJobs     int     `json:"total_jobs"`
	CompletedJobs int     `json:"completed_jobs"`
	Coverage      float64 `json:"coverage"`
	TotalCrashes  int     `json:"total_crashes"`
}

// ListCampaignsResult represents the paginated result
type ListCampaignsResult struct {
	Campaigns  []CampaignListItemDTO `json:"campaigns"`
	Pagination PaginationDTO         `json:"pagination"`
	Filters    FiltersAppliedDTO     `json:"filters"`
}

// PaginationDTO contains pagination metadata
type PaginationDTO struct {
	Page       int  `json:"page"`
	PageSize   int  `json:"page_size"`
	TotalItems int  `json:"total_items"`
	TotalPages int  `json:"total_pages"`
	HasNext    bool `json:"has_next"`
	HasPrev    bool `json:"has_prev"`
}

// FiltersAppliedDTO shows which filters were applied
type FiltersAppliedDTO struct {
	Status        *string    `json:"status,omitempty"`
	Name          *string    `json:"name,omitempty"`
	CreatedBy     *string    `json:"created_by,omitempty"`
	CreatedAfter  *time.Time `json:"created_after,omitempty"`
	CreatedBefore *time.Time `json:"created_before,omitempty"`
	SortBy        string     `json:"sort_by"`
	SortOrder     string     `json:"sort_order"`
}

// ListCampaignsHandler handles list campaigns queries
type ListCampaignsHandler struct {
	repo   repository.CampaignRepository
	finder *query.CampaignFinder
}

// NewListCampaignsHandler creates a new handler instance
func NewListCampaignsHandler(repo repository.CampaignRepository) *ListCampaignsHandler {
	return &ListCampaignsHandler{
		repo:   repo,
		finder: query.NewCampaignFinder(repo),
	}
}

// Handle executes the list campaigns query
func (h *ListCampaignsHandler) Handle(ctx context.Context, q interface{}) (interface{}, error) {
	query, ok := q.(*ListCampaignsQuery)
	if !ok {
		return nil, NewApplicationError(
			ErrCodeInvalidQuery,
			"Invalid query type",
			nil,
		).WithDetails("expected", "*ListCampaignsQuery")
	}

	// Set defaults
	h.setDefaults(query)

	// Validate query
	if err := h.validateQuery(query); err != nil {
		return nil, err
	}

	// Check authorization
	if err := h.checkAuthorization(ctx, query); err != nil {
		return nil, err
	}

	// Build search options
	searchOpts := h.buildSearchOptions(query)

	// Execute search
	campaigns, total, err := h.executeCampaignSearch(ctx, searchOpts, query)
	if err != nil {
		return nil, NewApplicationError(
			ErrCodeOperationFailed,
			"Failed to list campaigns",
			err,
		)
	}

	// Convert to DTOs
	dtos := h.toDTOs(campaigns, query.IncludeStats)

	// Build result
	result := &ListCampaignsResult{
		Campaigns: dtos,
		Pagination: PaginationDTO{
			Page:       query.Page,
			PageSize:   query.PageSize,
			TotalItems: total,
			TotalPages: h.calculateTotalPages(total, query.PageSize),
			HasNext:    query.Page < h.calculateTotalPages(total, query.PageSize),
			HasPrev:    query.Page > 1,
		},
		Filters: FiltersAppliedDTO{
			Status:        query.Status,
			Name:          query.Name,
			CreatedBy:     query.CreatedBy,
			CreatedAfter:  query.CreatedAfter,
			CreatedBefore: query.CreatedBefore,
			SortBy:        query.SortBy,
			SortOrder:     query.SortOrder,
		},
	}

	return result, nil
}

// setDefaults sets default values for the query
func (h *ListCampaignsHandler) setDefaults(query *ListCampaignsQuery) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}
	if query.SortBy == "" {
		query.SortBy = "created_at"
	}
	if query.SortOrder == "" {
		query.SortOrder = "desc"
	}
}

// validateQuery validates the list campaigns query
func (h *ListCampaignsHandler) validateQuery(query *ListCampaignsQuery) error {
	// Validate pagination
	if query.Page < 1 {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"Page must be at least 1",
			nil,
		).WithDetails("field", "page").WithDetails("value", query.Page)
	}

	if query.PageSize < 1 || query.PageSize > 100 {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"Page size must be between 1 and 100",
			nil,
		).WithDetails("field", "page_size").WithDetails("value", query.PageSize)
	}

	// Validate status filter
	if query.Status != nil {
		validStatuses := []string{"draft", "active", "paused", "completed", "failed"}
		isValid := false
		for _, valid := range validStatuses {
			if *query.Status == valid {
				isValid = true
				break
			}
		}
		if !isValid {
			return NewApplicationError(
				ErrCodeValidationFailed,
				"Invalid status filter",
				nil,
			).WithDetails("field", "status").WithDetails("value", *query.Status)
		}
	}

	// Validate sort options
	validSortFields := []string{"created_at", "updated_at", "name", "status"}
	isValidSort := false
	for _, valid := range validSortFields {
		if query.SortBy == valid {
			isValidSort = true
			break
		}
	}
	if !isValidSort {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"Invalid sort field",
			nil,
		).WithDetails("field", "sort_by").WithDetails("value", query.SortBy)
	}

	if query.SortOrder != "asc" && query.SortOrder != "desc" {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"Sort order must be 'asc' or 'desc'",
			nil,
		).WithDetails("field", "sort_order").WithDetails("value", query.SortOrder)
	}

	// Validate date ranges
	if query.CreatedAfter != nil && query.CreatedBefore != nil {
		if query.CreatedAfter.After(*query.CreatedBefore) {
			return NewApplicationError(
				ErrCodeValidationFailed,
				"Created after date must be before created before date",
				nil,
			)
		}
	}

	if query.UpdatedAfter != nil && query.UpdatedBefore != nil {
		if query.UpdatedAfter.After(*query.UpdatedBefore) {
			return NewApplicationError(
				ErrCodeValidationFailed,
				"Updated after date must be before updated before date",
				nil,
			)
		}
	}

	return nil
}

// checkAuthorization checks if the user is authorized to list campaigns
func (h *ListCampaignsHandler) checkAuthorization(ctx context.Context, query *ListCampaignsQuery) error {
	userID := getUserIDFromContext(ctx)
	if userID == "" {
		return NewApplicationError(
			ErrCodeUnauthorized,
			"User not authenticated",
			nil,
		)
	}

	// In a real implementation:
	// - Check user permissions
	// - Apply user-specific filters (e.g., only show user's campaigns)
	// - Check organization/tenant boundaries

	return nil
}

// buildSearchOptions builds search options from query
func (h *ListCampaignsHandler) buildSearchOptions(q *ListCampaignsQuery) query.SearchOptions {
	opts := query.SearchOptions{
		Filters: query.FilterOptions{},
	}

	// Apply filters
	if q.Status != nil {
		status, _ := types.ParseState(*q.Status)
		opts.Filters.Status = &status
	}

	if q.Name != nil {
		opts.Filters.Name = q.Name
	}

	if q.CreatedAfter != nil || q.CreatedBefore != nil {
		opts.Filters.DateRange = &query.DateRange{
			Field: "created_at",
			From:  q.CreatedAfter,
			To:    q.CreatedBefore,
		}
	}

	return opts
}

// executeCampaignSearch executes the campaign search
func (h *ListCampaignsHandler) executeCampaignSearch(ctx context.Context, opts query.SearchOptions, q *ListCampaignsQuery) ([]*types.Campaign, int, error) {
	// Calculate offset
	offset := (q.Page - 1) * q.PageSize

	// Apply status filter
	if opts.Filters.Status != nil {
		return h.repo.FindByStatus(ctx, *opts.Filters.Status)
	}

	// Apply name filter
	if opts.Filters.Name != nil {
		campaigns, err := h.repo.FindByName(ctx, *opts.Filters.Name)
		return campaigns, len(campaigns), err
	}

	// Default: list all with pagination
	return h.repo.List(ctx, offset, q.PageSize)
}

// toDTOs converts domain models to DTOs
func (h *ListCampaignsHandler) toDTOs(campaigns []*types.Campaign, includeStats bool) []CampaignListItemDTO {
	dtos := make([]CampaignListItemDTO, len(campaigns))

	for i, campaign := range campaigns {
		dto := CampaignListItemDTO{
			ID:          campaign.ID,
			Name:        campaign.Name,
			Description: campaign.Description,
			Status:      campaign.Status.String(),
			CreatedAt:   campaign.CreatedAt,
			UpdatedAt:   campaign.UpdatedAt,
		}

		// Include stats if requested
		if includeStats {
			dto.Stats = h.getQuickStats(campaign.ID)
		}

		dtos[i] = dto
	}

	return dtos
}

// getQuickStats retrieves quick stats for a campaign
func (h *ListCampaignsHandler) getQuickStats(campaignID string) *CampaignQuickStatsDTO {
	// In a real implementation, this would query stats asynchronously
	// or from a cache/materialized view

	return &CampaignQuickStatsDTO{
		TotalJobs:     10,
		CompletedJobs: 8,
		Coverage:      75.5,
		TotalCrashes:  3,
	}
}

// calculateTotalPages calculates total number of pages
func (h *ListCampaignsHandler) calculateTotalPages(totalItems, pageSize int) int {
	if totalItems == 0 {
		return 0
	}
	pages := totalItems / pageSize
	if totalItems%pageSize > 0 {
		pages++
	}
	return pages
}

// Error codes
const (
	ErrCodeOperationFailed = "OPERATION_FAILED"
)
