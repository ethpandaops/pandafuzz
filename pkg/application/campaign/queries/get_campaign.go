package queries

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/query"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
)

// GetCampaignQuery represents a query to get a single campaign
type GetCampaignQuery struct {
	ID           string   `json:"id" validate:"required"`
	IncludeStats bool     `json:"include_stats"`
	IncludeBots  bool     `json:"include_bots"`
	IncludeJobs  bool     `json:"include_jobs"`
	Fields       []string `json:"fields,omitempty"` // Specific fields to include
}

// CampaignDetailDTO represents detailed campaign data
type CampaignDetailDTO struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Status      string                 `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CreatedBy   string                 `json:"created_by,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`

	// Optional includes
	Statistics *CampaignStatsDTO `json:"statistics,omitempty"`
	Bots       []BotSummaryDTO   `json:"bots,omitempty"`
	Jobs       []JobSummaryDTO   `json:"jobs,omitempty"`

	// Computed fields
	Duration      *time.Duration `json:"duration,omitempty"`
	IsActive      bool           `json:"is_active"`
	CanBeModified bool           `json:"can_be_modified"`
}

// CampaignStatsDTO represents campaign statistics
type CampaignStatsDTO struct {
	TotalJobs     int           `json:"total_jobs"`
	CompletedJobs int           `json:"completed_jobs"`
	FailedJobs    int           `json:"failed_jobs"`
	TotalCrashes  int           `json:"total_crashes"`
	UniqueCrashes int           `json:"unique_crashes"`
	Coverage      float64       `json:"coverage"`
	ExecutionTime time.Duration `json:"execution_time"`
	LastActivity  *time.Time    `json:"last_activity,omitempty"`
}

// BotSummaryDTO represents bot summary information
type BotSummaryDTO struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Status   string    `json:"status"`
	JoinedAt time.Time `json:"joined_at"`
}

// JobSummaryDTO represents job summary information
type JobSummaryDTO struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// GetCampaignResult wraps the query result
type GetCampaignResult struct {
	Campaign *CampaignDetailDTO `json:"campaign"`
}

// GetCampaignHandler handles get campaign queries
type GetCampaignHandler struct {
	repo       repository.CampaignRepository
	finder     *query.CampaignFinder
	statsQuery *query.CampaignStatisticsService
}

// NewGetCampaignHandler creates a new handler instance
func NewGetCampaignHandler(
	repo repository.CampaignRepository,
) *GetCampaignHandler {
	finder := query.NewCampaignFinder(repo)
	statsService := query.NewCampaignStatisticsService(repo)

	return &GetCampaignHandler{
		repo:       repo,
		finder:     finder,
		statsQuery: statsService,
	}
}

// Handle executes the get campaign query
func (h *GetCampaignHandler) Handle(ctx context.Context, q interface{}) (interface{}, error) {
	query, ok := q.(*GetCampaignQuery)
	if !ok {
		return nil, NewApplicationError(
			ErrCodeInvalidQuery,
			"Invalid query type",
			nil,
		).WithDetails("expected", "*GetCampaignQuery")
	}

	// Validate query
	if err := h.validateQuery(query); err != nil {
		return nil, err
	}

	// Check authorization
	if err := h.checkAuthorization(ctx, query); err != nil {
		return nil, err
	}

	// Get campaign
	campaign, err := h.repo.FindByID(ctx, query.ID)
	if err != nil {
		return nil, NewApplicationError(
			ErrCodeNotFound,
			"Campaign not found",
			err,
		).WithDetails("campaign_id", query.ID)
	}

	// Convert to DTO
	dto := h.toDetailDTO(campaign)

	// Include optional data
	if query.IncludeStats {
		stats, err := h.getStatistics(ctx, campaign.ID)
		if err == nil {
			dto.Statistics = stats
		}
	}

	if query.IncludeBots {
		bots, err := h.getBots(ctx, campaign.ID)
		if err == nil {
			dto.Bots = bots
		}
	}

	if query.IncludeJobs {
		jobs, err := h.getJobs(ctx, campaign.ID)
		if err == nil {
			dto.Jobs = jobs
		}
	}

	// Apply field filtering if specified
	if len(query.Fields) > 0 {
		dto = h.filterFields(dto, query.Fields)
	}

	return &GetCampaignResult{Campaign: dto}, nil
}

// validateQuery validates the get campaign query
func (h *GetCampaignHandler) validateQuery(query *GetCampaignQuery) error {
	if query.ID == "" {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"Campaign ID is required",
			nil,
		).WithDetails("field", "id")
	}

	// Validate fields if specified
	if len(query.Fields) > 0 {
		validFields := map[string]bool{
			"id": true, "name": true, "description": true, "status": true,
			"created_at": true, "updated_at": true, "created_by": true,
			"metadata": true, "statistics": true, "bots": true, "jobs": true,
			"duration": true, "is_active": true, "can_be_modified": true,
		}

		for _, field := range query.Fields {
			if !validFields[field] {
				return NewApplicationError(
					ErrCodeValidationFailed,
					"Invalid field specified",
					nil,
				).WithDetails("field", field).WithDetails("valid_fields", validFields)
			}
		}
	}

	return nil
}

// checkAuthorization checks if the user is authorized to view the campaign
func (h *GetCampaignHandler) checkAuthorization(ctx context.Context, query *GetCampaignQuery) error {
	userID := getUserIDFromContext(ctx)
	if userID == "" {
		return NewApplicationError(
			ErrCodeUnauthorized,
			"User not authenticated",
			nil,
		)
	}

	// In a real implementation, check:
	// - User has permission to view campaigns
	// - User owns the campaign or has appropriate role
	// - Campaign visibility settings

	return nil
}

// toDetailDTO converts domain model to detailed DTO
func (h *GetCampaignHandler) toDetailDTO(campaign *types.Campaign) *CampaignDetailDTO {
	dto := &CampaignDetailDTO{
		ID:            campaign.ID,
		Name:          campaign.Name,
		Description:   campaign.Description,
		Status:        campaign.Status.String(),
		CreatedAt:     campaign.CreatedAt,
		UpdatedAt:     campaign.UpdatedAt,
		IsActive:      campaign.IsActive(),
		CanBeModified: campaign.CanBeModified(),
	}

	// Calculate duration for completed campaigns
	if campaign.Status == types.StateCompleted {
		duration := campaign.UpdatedAt.Sub(campaign.CreatedAt)
		dto.Duration = &duration
	}

	// In a real implementation, fetch metadata from a separate store
	// dto.CreatedBy = getCreatedBy(campaign.ID)
	// dto.Metadata = getMetadata(campaign.ID)

	return dto
}

// getStatistics retrieves campaign statistics
func (h *GetCampaignHandler) getStatistics(ctx context.Context, campaignID string) (*CampaignStatsDTO, error) {
	// In a real implementation, this would query various repositories
	// to gather statistics about jobs, crashes, coverage, etc.

	// For now, return mock data
	stats := &CampaignStatsDTO{
		TotalJobs:     100,
		CompletedJobs: 85,
		FailedJobs:    5,
		TotalCrashes:  42,
		UniqueCrashes: 12,
		Coverage:      67.5,
		ExecutionTime: 2 * time.Hour,
	}

	lastActivity := time.Now().Add(-30 * time.Minute)
	stats.LastActivity = &lastActivity

	return stats, nil
}

// getBots retrieves bots associated with the campaign
func (h *GetCampaignHandler) getBots(ctx context.Context, campaignID string) ([]BotSummaryDTO, error) {
	// In a real implementation, this would query bot repository
	// to get bots assigned to this campaign

	// For now, return empty list
	return []BotSummaryDTO{}, nil
}

// getJobs retrieves recent jobs for the campaign
func (h *GetCampaignHandler) getJobs(ctx context.Context, campaignID string) ([]JobSummaryDTO, error) {
	// In a real implementation, this would query job repository
	// to get jobs for this campaign

	// For now, return empty list
	return []JobSummaryDTO{}, nil
}

// filterFields applies field filtering to the DTO
func (h *GetCampaignHandler) filterFields(dto *CampaignDetailDTO, fields []string) *CampaignDetailDTO {
	// In a real implementation, use reflection or a more sophisticated
	// approach to filter fields dynamically

	// For now, return the full DTO
	return dto
}

// Helper function to get user ID from context
func getUserIDFromContext(ctx context.Context) string {
	if userID, ok := ctx.Value("user_id").(string); ok {
		return userID
	}
	return ""
}

// Error codes
const (
	ErrCodeInvalidQuery     = "INVALID_QUERY"
	ErrCodeValidationFailed = "VALIDATION_FAILED"
	ErrCodeUnauthorized     = "UNAUTHORIZED"
	ErrCodeNotFound         = "NOT_FOUND"
)

// ApplicationError represents an application layer error
type ApplicationError struct {
	Code    string
	Message string
	Details map[string]interface{}
	Cause   error
}

// Error implements the error interface
func (e ApplicationError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

// WithDetails adds details to the error
func (e ApplicationError) WithDetails(key string, value interface{}) ApplicationError {
	if e.Details == nil {
		e.Details = make(map[string]interface{}())
	}
	e.Details[key] = value
	return e
}

// NewApplicationError creates a new application error
func NewApplicationError(code, message string, cause error) ApplicationError {
	return ApplicationError{
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
		Cause:   cause,
	}
}
