package commands

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/service"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
)

// DeleteCampaignCommand represents a command to delete a campaign
type DeleteCampaignCommand struct {
	ID        string `json:"id" validate:"required"`
	DeletedBy string `json:"deleted_by" validate:"required"`
	Force     bool   `json:"force"` // Force delete even if campaign is active
	Reason    string `json:"reason,omitempty" validate:"max=200"`
}

// DeleteCampaignResult represents the result of campaign deletion
type DeleteCampaignResult struct {
	Success       bool      `json:"success"`
	EventID       string    `json:"event_id"`
	DeletedAt     time.Time `json:"deleted_at"`
	AffectedItems int       `json:"affected_items"` // Related items that were also deleted
}

// DeleteCampaignHandler handles campaign deletion commands
type DeleteCampaignHandler struct {
	repo      repository.CampaignRepository
	validator *service.CampaignValidator
	eventBus  service.EventPublisher
}

// NewDeleteCampaignHandler creates a new handler instance
func NewDeleteCampaignHandler(
	repo repository.CampaignRepository,
	validator *service.CampaignValidator,
	eventBus service.EventPublisher,
) *DeleteCampaignHandler {
	return &DeleteCampaignHandler{
		repo:      repo,
		validator: validator,
		eventBus:  eventBus,
	}
}

// Handle executes the delete campaign command
func (h *DeleteCampaignHandler) Handle(ctx context.Context, cmd interface{}) error {
	command, ok := cmd.(*DeleteCampaignCommand)
	if !ok {
		return NewApplicationError(
			ErrCodeInvalidCommand,
			"Invalid command type",
			nil,
		).WithDetails("expected", "*DeleteCampaignCommand")
	}

	// Validate command
	if err := h.validateCommand(command); err != nil {
		return err
	}

	// Check authorization
	if err := h.checkAuthorization(ctx, command); err != nil {
		return err
	}

	// Get existing campaign
	campaign, err := h.repo.FindByID(ctx, command.ID)
	if err != nil {
		return NewApplicationError(
			ErrCodeNotFound,
			"Campaign not found",
			err,
		).WithDetails("campaign_id", command.ID)
	}

	// Check if campaign can be deleted
	if err := h.checkDeletionRules(campaign, command); err != nil {
		return err
	}

	// Perform soft delete or hard delete based on business rules
	// In this implementation, we'll do hard delete
	if err := h.repo.Delete(ctx, command.ID); err != nil {
		return NewApplicationError(
			ErrCodeOperationFailed,
			"Failed to delete campaign",
			err,
		).WithDetails("campaign_id", command.ID)
	}

	// Publish deletion event
	event := NewCampaignDeletedEvent(campaign, command.DeletedBy, command.Reason)
	if err := h.eventBus.Publish(ctx, event); err != nil {
		// Log error but don't fail the operation
	}

	// Clean up related resources (in a real system, this might be async)
	// - Delete campaign jobs
	// - Archive campaign results
	// - Clean up temporary files
	// - Update statistics

	return nil
}

// HandleWithResult executes the command and returns the result
func (h *DeleteCampaignHandler) HandleWithResult(ctx context.Context, cmd *DeleteCampaignCommand) (*DeleteCampaignResult, error) {
	// Validate command
	if err := h.validateCommand(cmd); err != nil {
		return nil, err
	}

	// Check authorization
	if err := h.checkAuthorization(ctx, cmd); err != nil {
		return nil, err
	}

	// Get existing campaign
	campaign, err := h.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, NewApplicationError(
			ErrCodeNotFound,
			"Campaign not found",
			err,
		).WithDetails("campaign_id", cmd.ID)
	}

	// Check if campaign can be deleted
	if err := h.checkDeletionRules(campaign, cmd); err != nil {
		return nil, err
	}

	// Count related items before deletion
	affectedItems := h.countRelatedItems(ctx, cmd.ID)

	// Delete the campaign
	if err := h.repo.Delete(ctx, cmd.ID); err != nil {
		return nil, NewApplicationError(
			ErrCodeOperationFailed,
			"Failed to delete campaign",
			err,
		).WithDetails("campaign_id", cmd.ID)
	}

	// Publish deletion event
	event := NewCampaignDeletedEvent(campaign, cmd.DeletedBy, cmd.Reason)
	h.eventBus.Publish(ctx, event)

	result := &DeleteCampaignResult{
		Success:       true,
		EventID:       generateEventID(),
		DeletedAt:     time.Now(),
		AffectedItems: affectedItems,
	}

	return result, nil
}

// validateCommand validates the delete campaign command
func (h *DeleteCampaignHandler) validateCommand(cmd *DeleteCampaignCommand) error {
	if cmd.ID == "" {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"Campaign ID is required",
			nil,
		).WithDetails("field", "id")
	}

	if cmd.DeletedBy == "" {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"DeletedBy is required",
			nil,
		).WithDetails("field", "deleted_by")
	}

	if len(cmd.Reason) > 200 {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"Reason must not exceed 200 characters",
			nil,
		).WithDetails("field", "reason").WithDetails("length", len(cmd.Reason))
	}

	return nil
}

// checkAuthorization checks if the user is authorized to delete campaigns
func (h *DeleteCampaignHandler) checkAuthorization(ctx context.Context, cmd *DeleteCampaignCommand) error {
	userID := getUserIDFromContext(ctx)
	if userID == "" {
		return NewApplicationError(
			ErrCodeUnauthorized,
			"User not authenticated",
			nil,
		)
	}

	// In a real implementation, check:
	// - User has permission to delete campaigns
	// - User owns the campaign or has admin role
	// - Organization policies for deletion

	// For force delete, might require admin privileges
	if cmd.Force {
		// Check admin role
		if !isUserAdmin(ctx) {
			return NewApplicationError(
				ErrCodeUnauthorized,
				"Force delete requires admin privileges",
				nil,
			).WithDetails("user_id", userID)
		}
	}

	return nil
}

// checkDeletionRules checks if the campaign can be deleted
func (h *DeleteCampaignHandler) checkDeletionRules(campaign *types.Campaign, cmd *DeleteCampaignCommand) error {
	// Check if campaign is active
	if campaign.IsActive() && !cmd.Force {
		return NewApplicationError(
			ErrCodeConflict,
			"Cannot delete active campaign",
			nil,
		).WithDetails("campaign_id", campaign.ID).
			WithDetails("status", campaign.Status.String()).
			WithDetails("hint", "Use force=true to delete active campaigns")
	}

	// Check if campaign has running jobs
	// In a real implementation, this would query job repository
	// hasRunningJobs := h.checkRunningJobs(campaign.ID)
	// if hasRunningJobs && !cmd.Force {
	//     return NewApplicationError(
	//         ErrCodeConflict,
	//         "Cannot delete campaign with running jobs",
	//         nil,
	//     ).WithDetails("campaign_id", campaign.ID)
	// }

	// Check retention policy
	// Some campaigns might need to be retained for compliance
	if h.mustRetainCampaign(campaign) {
		return NewApplicationError(
			ErrCodeConflict,
			"Campaign must be retained for compliance",
			nil,
		).WithDetails("campaign_id", campaign.ID).
			WithDetails("retention_reason", "compliance_policy")
	}

	return nil
}

// mustRetainCampaign checks if campaign must be retained
func (h *DeleteCampaignHandler) mustRetainCampaign(campaign *types.Campaign) bool {
	// In a real implementation, check:
	// - Compliance requirements
	// - Legal holds
	// - Audit requirements
	// - Organization policies

	// For now, campaigns completed less than 30 days ago must be retained
	if campaign.Status == types.StateCompleted {
		retentionPeriod := 30 * 24 * time.Hour
		if time.Since(campaign.UpdatedAt) < retentionPeriod {
			return true
		}
	}

	return false
}

// countRelatedItems counts items that will be affected by deletion
func (h *DeleteCampaignHandler) countRelatedItems(ctx context.Context, campaignID string) int {
	// In a real implementation, count:
	// - Jobs
	// - Results
	// - Logs
	// - Metrics
	// - Artifacts

	// For now, return a placeholder
	return 0
}

// isUserAdmin checks if the user has admin role
func isUserAdmin(ctx context.Context) bool {
	// In a real implementation, check user roles from context
	if role, ok := ctx.Value("user_role").(string); ok {
		return role == "admin"
	}
	return false
}

// CampaignDeletedEvent represents a campaign deletion event
type CampaignDeletedEvent struct {
	types.BaseEvent
	DeletedBy    string      `json:"deleted_by"`
	Reason       string      `json:"reason,omitempty"`
	CampaignName string      `json:"campaign_name"`
	CampaignData interface{} `json:"campaign_data"` // Archived campaign data
}

// NewCampaignDeletedEvent creates a new campaign deleted event
func NewCampaignDeletedEvent(campaign *types.Campaign, deletedBy, reason string) *CampaignDeletedEvent {
	return &CampaignDeletedEvent{
		BaseEvent: types.BaseEvent{
			Type:       "campaign.deleted",
			CampaignID: campaign.ID,
			Timestamp:  time.Now(),
		},
		DeletedBy:    deletedBy,
		Reason:       reason,
		CampaignName: campaign.Name,
		CampaignData: campaign, // Archive the campaign data in the event
	}
}
