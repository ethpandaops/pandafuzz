package commands

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/service"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
)

// UpdateCampaignCommand represents a command to update a campaign
type UpdateCampaignCommand struct {
	ID          string                 `json:"id" validate:"required"`
	Name        *string                `json:"name,omitempty" validate:"omitempty,min=3,max=100"`
	Description *string                `json:"description,omitempty" validate:"omitempty,max=500"`
	Status      *string                `json:"status,omitempty" validate:"omitempty,oneof=draft active paused completed failed"`
	UpdatedBy   string                 `json:"updated_by" validate:"required"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Version     int                    `json:"version"` // For optimistic locking
}

// UpdateCampaignResult represents the result of campaign update
type UpdateCampaignResult struct {
	Campaign      *CampaignDTO `json:"campaign"`
	EventID       string       `json:"event_id"`
	UpdatedAt     time.Time    `json:"updated_at"`
	ChangedFields []string     `json:"changed_fields"`
}

// UpdateCampaignHandler handles campaign update commands
type UpdateCampaignHandler struct {
	repo      repository.CampaignRepository
	lifecycle *service.CampaignLifecycle
	validator *service.CampaignValidator
	eventBus  service.EventPublisher
}

// NewUpdateCampaignHandler creates a new handler instance
func NewUpdateCampaignHandler(
	repo repository.CampaignRepository,
	validator *service.CampaignValidator,
	eventBus service.EventPublisher,
) *UpdateCampaignHandler {
	lifecycle := service.NewCampaignLifecycle(repo, eventBus)
	return &UpdateCampaignHandler{
		repo:      repo,
		lifecycle: lifecycle,
		validator: validator,
		eventBus:  eventBus,
	}
}

// Handle executes the update campaign command
func (h *UpdateCampaignHandler) Handle(ctx context.Context, cmd interface{}) error {
	command, ok := cmd.(*UpdateCampaignCommand)
	if !ok {
		return NewApplicationError(
			ErrCodeInvalidCommand,
			"Invalid command type",
			nil,
		).WithDetails("expected", "*UpdateCampaignCommand")
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

	// Check version for optimistic locking
	// In a real implementation, the campaign would have a version field
	// if campaign.Version != command.Version {
	//     return NewApplicationError(
	//         ErrCodeConflict,
	//         "Campaign has been modified by another user",
	//         nil,
	//     ).WithDetails("expected_version", command.Version).WithDetails("actual_version", campaign.Version)
	// }

	// Apply updates
	changedFields := h.applyUpdates(campaign, command)
	if len(changedFields) == 0 {
		return nil // No changes to apply
	}

	// Validate the updated campaign
	if err := h.validator.ValidateUpdate(campaign); err != nil {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"Updated campaign validation failed",
			err,
		)
	}

	// Handle status transitions through lifecycle service
	if command.Status != nil {
		newStatus, err := types.ParseState(*command.Status)
		if err != nil {
			return NewApplicationError(
				ErrCodeValidationFailed,
				"Invalid status value",
				err,
			).WithDetails("status", *command.Status)
		}

		switch newStatus {
		case types.StateActive:
			err = h.lifecycle.Start(ctx, command.ID, command.UpdatedBy)
		case types.StatePaused:
			err = h.lifecycle.Pause(ctx, command.ID, command.UpdatedBy)
		case types.StateCompleted:
			err = h.lifecycle.Complete(ctx, command.ID, command.UpdatedBy, nil)
		case types.StateFailed:
			err = h.lifecycle.Fail(ctx, command.ID, "Manual failure", command.UpdatedBy)
		default:
			// For draft status or other transitions, update directly
			err = campaign.UpdateStatus(newStatus)
			if err == nil {
				err = h.repo.Update(ctx, campaign)
			}
		}

		if err != nil {
			return NewApplicationError(
				ErrCodeOperationFailed,
				"Failed to update campaign status",
				err,
			).WithDetails("campaign_id", command.ID).WithDetails("new_status", newStatus.String())
		}
	} else {
		// Update non-status fields
		if err := h.repo.Update(ctx, campaign); err != nil {
			return NewApplicationError(
				ErrCodeOperationFailed,
				"Failed to update campaign",
				err,
			).WithDetails("campaign_id", command.ID)
		}

		// Publish update event
		event := NewCampaignUpdatedEvent(campaign, command.UpdatedBy, changedFields)
		if err := h.eventBus.Publish(ctx, event); err != nil {
			// Log error but don't fail the operation
		}
	}

	return nil
}

// HandleWithResult executes the command and returns the result
func (h *UpdateCampaignHandler) HandleWithResult(ctx context.Context, cmd *UpdateCampaignCommand) (*UpdateCampaignResult, error) {
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

	// Apply updates
	changedFields := h.applyUpdates(campaign, cmd)

	// Handle status transitions or regular updates
	if cmd.Status != nil {
		newStatus, err := types.ParseState(*cmd.Status)
		if err != nil {
			return nil, NewApplicationError(
				ErrCodeValidationFailed,
				"Invalid status value",
				err,
			).WithDetails("status", *cmd.Status)
		}

		switch newStatus {
		case types.StateActive:
			err = h.lifecycle.Start(ctx, cmd.ID, cmd.UpdatedBy)
		case types.StatePaused:
			err = h.lifecycle.Pause(ctx, cmd.ID, cmd.UpdatedBy)
		case types.StateCompleted:
			err = h.lifecycle.Complete(ctx, cmd.ID, cmd.UpdatedBy, nil)
		case types.StateFailed:
			err = h.lifecycle.Fail(ctx, cmd.ID, "Manual failure", cmd.UpdatedBy)
		default:
			err = campaign.UpdateStatus(newStatus)
			if err == nil {
				err = h.repo.Update(ctx, campaign)
			}
		}

		if err != nil {
			return nil, NewApplicationError(
				ErrCodeOperationFailed,
				"Failed to update campaign status",
				err,
			).WithDetails("campaign_id", cmd.ID)
		}

		// Refresh campaign after status update
		campaign, _ = h.repo.FindByID(ctx, cmd.ID)
	} else if len(changedFields) > 0 {
		// Update non-status fields
		if err := h.repo.Update(ctx, campaign); err != nil {
			return nil, NewApplicationError(
				ErrCodeOperationFailed,
				"Failed to update campaign",
				err,
			).WithDetails("campaign_id", cmd.ID)
		}

		// Publish update event
		event := NewCampaignUpdatedEvent(campaign, cmd.UpdatedBy, changedFields)
		h.eventBus.Publish(ctx, event)
	}

	// Convert to DTO
	dto := &CampaignDTO{
		ID:          campaign.ID,
		Name:        campaign.Name,
		Description: campaign.Description,
		Status:      campaign.Status.String(),
		CreatedBy:   cmd.UpdatedBy, // This would come from stored metadata
		CreatedAt:   campaign.CreatedAt,
		UpdatedAt:   campaign.UpdatedAt,
		Metadata:    cmd.Metadata,
	}

	result := &UpdateCampaignResult{
		Campaign:      dto,
		EventID:       generateEventID(),
		UpdatedAt:     campaign.UpdatedAt,
		ChangedFields: changedFields,
	}

	return result, nil
}

// validateCommand validates the update campaign command
func (h *UpdateCampaignHandler) validateCommand(cmd *UpdateCampaignCommand) error {
	if cmd.ID == "" {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"Campaign ID is required",
			nil,
		).WithDetails("field", "id")
	}

	if cmd.UpdatedBy == "" {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"UpdatedBy is required",
			nil,
		).WithDetails("field", "updated_by")
	}

	// Validate name if provided
	if cmd.Name != nil {
		if len(*cmd.Name) < 3 || len(*cmd.Name) > 100 {
			return NewApplicationError(
				ErrCodeValidationFailed,
				"Campaign name must be between 3 and 100 characters",
				nil,
			).WithDetails("field", "name").WithDetails("length", len(*cmd.Name))
		}
	}

	// Validate description if provided
	if cmd.Description != nil {
		if len(*cmd.Description) > 500 {
			return NewApplicationError(
				ErrCodeValidationFailed,
				"Campaign description must not exceed 500 characters",
				nil,
			).WithDetails("field", "description").WithDetails("length", len(*cmd.Description))
		}
	}

	// Validate status if provided
	if cmd.Status != nil {
		validStatuses := []string{"draft", "active", "paused", "completed", "failed"}
		isValid := false
		for _, valid := range validStatuses {
			if *cmd.Status == valid {
				isValid = true
				break
			}
		}
		if !isValid {
			return NewApplicationError(
				ErrCodeValidationFailed,
				"Invalid status value",
				nil,
			).WithDetails("field", "status").WithDetails("value", *cmd.Status).WithDetails("valid_values", validStatuses)
		}
	}

	return nil
}

// checkAuthorization checks if the user is authorized to update campaigns
func (h *UpdateCampaignHandler) checkAuthorization(ctx context.Context, cmd *UpdateCampaignCommand) error {
	userID := getUserIDFromContext(ctx)
	if userID == "" {
		return NewApplicationError(
			ErrCodeUnauthorized,
			"User not authenticated",
			nil,
		)
	}

	// In a real implementation, check:
	// - User has permission to update campaigns
	// - User owns the campaign or has admin role
	// - Campaign is in a state that allows updates

	return nil
}

// applyUpdates applies the updates to the campaign and returns changed fields
func (h *UpdateCampaignHandler) applyUpdates(campaign *types.Campaign, cmd *UpdateCampaignCommand) []string {
	var changedFields []string

	if cmd.Name != nil && *cmd.Name != campaign.Name {
		campaign.Name = *cmd.Name
		changedFields = append(changedFields, "name")
	}

	if cmd.Description != nil && *cmd.Description != campaign.Description {
		campaign.Description = *cmd.Description
		changedFields = append(changedFields, "description")
	}

	// Status is handled separately through lifecycle service
	if cmd.Status != nil {
		changedFields = append(changedFields, "status")
	}

	if len(changedFields) > 0 {
		campaign.UpdatedAt = time.Now()
	}

	return changedFields
}

// CampaignUpdatedEvent represents a campaign update event
type CampaignUpdatedEvent struct {
	types.BaseEvent
	UpdatedBy     string   `json:"updated_by"`
	ChangedFields []string `json:"changed_fields"`
}

// NewCampaignUpdatedEvent creates a new campaign updated event
func NewCampaignUpdatedEvent(campaign *types.Campaign, updatedBy string, changedFields []string) *CampaignUpdatedEvent {
	return &CampaignUpdatedEvent{
		BaseEvent: types.BaseEvent{
			Type:       "campaign.updated",
			CampaignID: campaign.ID,
			Timestamp:  time.Now(),
		},
		UpdatedBy:     updatedBy,
		ChangedFields: changedFields,
	}
}

// Error codes
const (
	ErrCodeNotFound = "NOT_FOUND"
	ErrCodeConflict = "CONFLICT"
)
