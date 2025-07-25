package service

import (
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
	"github.com/ethpandaops/pandafuzz/pkg/shared/errors"
	"github.com/ethpandaops/pandafuzz/pkg/shared/validation"
)

// CampaignValidator provides campaign validation logic
type CampaignValidator struct {
	campaignNameMinLength int
	campaignNameMaxLength int
	descriptionMaxLength  int
}

// NewCampaignValidator creates a new campaign validator
func NewCampaignValidator() *CampaignValidator {
	return &CampaignValidator{
		campaignNameMinLength: 3,
		campaignNameMaxLength: 100,
		descriptionMaxLength:  500,
	}
}

// ValidateCreate validates a campaign for creation
func (v *CampaignValidator) ValidateCreate(campaign *types.Campaign) error {
	var validationErrors validation.ValidationErrors

	// Validate ID
	if campaign.ID == "" {
		validationErrors.Add("id", "Campaign ID is required")
	} else if !validation.IsValidUUID(campaign.ID) {
		validationErrors.Add("id", "Campaign ID must be a valid UUID")
	}

	// Validate name
	if err := v.validateName(campaign.Name); err != nil {
		validationErrors.AddError(err)
	}

	// Validate description
	if err := v.validateDescription(campaign.Description); err != nil {
		validationErrors.AddError(err)
	}

	// Validate initial state
	if campaign.Status != types.StateDraft {
		validationErrors.Add("status", "New campaigns must start in draft state")
	}

	if validationErrors.HasErrors() {
		return errors.NewDomainError(errors.ErrCodeInvalidInput, "Campaign validation failed").
			WithDetails("errors", validationErrors)
	}

	return nil
}

// ValidateUpdate validates a campaign for update
func (v *CampaignValidator) ValidateUpdate(current, updated *types.Campaign) error {
	var validationErrors validation.ValidationErrors

	// Ensure IDs match
	if current.ID != updated.ID {
		validationErrors.Add("id", "Campaign ID cannot be changed")
	}

	// Validate name
	if err := v.validateName(updated.Name); err != nil {
		validationErrors.AddError(err)
	}

	// Validate description
	if err := v.validateDescription(updated.Description); err != nil {
		validationErrors.AddError(err)
	}

	// Check if campaign can be modified
	if !current.CanBeModified() {
		validationErrors.Add("status", "Campaign cannot be modified in current state")
	}

	if validationErrors.HasErrors() {
		return errors.NewDomainError(errors.ErrCodeInvalidInput, "Campaign update validation failed").
			WithDetails("errors", validationErrors)
	}

	return nil
}

// ValidateStateTransition validates a state transition
func (v *CampaignValidator) ValidateStateTransition(campaign *types.Campaign, newState types.State) error {
	// Validate the new state itself
	if err := newState.Validate(); err != nil {
		return errors.NewDomainError(errors.ErrCodeInvalidInput, "Invalid target state").
			WithDetails("state", newState).
			WithDetails("error", err.Error())
	}

	// Check if transition is allowed
	if !campaign.Status.CanTransitionTo(newState) {
		return errors.NewDomainError(errors.ErrCodeInvalidState, "State transition not allowed").
			WithDetails("current_state", campaign.Status).
			WithDetails("target_state", newState)
	}

	// Additional business rules for specific transitions
	switch newState {
	case types.StateActive:
		if err := v.validateCanActivate(campaign); err != nil {
			return err
		}
	case types.StateCompleted:
		if err := v.validateCanComplete(campaign); err != nil {
			return err
		}
	}

	return nil
}

// validateName validates the campaign name
func (v *CampaignValidator) validateName(name string) error {
	if err := validation.ValidateRequired(name, "name"); err != nil {
		return err
	}

	if err := validation.ValidateLength(name, "name", v.campaignNameMinLength, v.campaignNameMaxLength); err != nil {
		return err
	}

	// Additional name validation rules
	if err := validation.ValidateIdentifier(name); err != nil {
		return validation.NewValidationError("name", "Campaign name must contain only letters, numbers, underscores, and hyphens")
	}

	return nil
}

// validateDescription validates the campaign description
func (v *CampaignValidator) validateDescription(description string) error {
	// Description is optional, but if provided, must be within limits
	if description != "" {
		if err := validation.ValidateLength(description, "description", 0, v.descriptionMaxLength); err != nil {
			return err
		}
	}
	return nil
}

// validateCanActivate checks if a campaign can be activated
func (v *CampaignValidator) validateCanActivate(campaign *types.Campaign) error {
	// Add business rules for activation
	// For example, check if campaign has required configuration
	if campaign.Status != types.StateDraft {
		return errors.NewDomainError(errors.ErrCodePreconditionFailed, "Only draft campaigns can be activated").
			WithDetails("current_state", campaign.Status)
	}

	// Additional validation could include:
	// - Check if campaign has targets configured
	// - Check if campaign has bots assigned
	// - Check if campaign has valid schedule
	// These would require additional fields/methods on Campaign type

	return nil
}

// validateCanComplete checks if a campaign can be completed
func (v *CampaignValidator) validateCanComplete(campaign *types.Campaign) error {
	if campaign.Status != types.StateActive && campaign.Status != types.StatePaused {
		return errors.NewDomainError(errors.ErrCodePreconditionFailed, "Only active or paused campaigns can be completed").
			WithDetails("current_state", campaign.Status)
	}

	return nil
}

// ValidateDeletion validates if a campaign can be deleted
func (v *CampaignValidator) ValidateDeletion(campaign *types.Campaign) error {
	// Only allow deletion of draft or completed campaigns
	if campaign.Status != types.StateDraft && campaign.Status != types.StateCompleted {
		return errors.NewDomainError(errors.ErrCodePreconditionFailed, "Campaign cannot be deleted in current state").
			WithDetails("current_state", campaign.Status)
	}

	return nil
}
