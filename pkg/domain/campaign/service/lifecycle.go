package service

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
	"github.com/ethpandaops/pandafuzz/pkg/shared/errors"
)

// CampaignLifecycle manages campaign state transitions and lifecycle operations
type CampaignLifecycle struct {
	repository repository.CampaignRepository
	validator  *CampaignValidator
	eventBus   EventPublisher
}

// NewCampaignLifecycle creates a new campaign lifecycle manager
func NewCampaignLifecycle(
	repo repository.CampaignRepository,
	validator *CampaignValidator,
	eventBus EventPublisher,
) *CampaignLifecycle {
	return &CampaignLifecycle{
		repository: repo,
		validator:  validator,
		eventBus:   eventBus,
	}
}

// Start transitions a campaign from draft to active state
func (l *CampaignLifecycle) Start(ctx context.Context, campaignID string, startedBy string) (*types.Campaign, error) {
	// Retrieve campaign
	campaign, err := l.repository.FindByID(ctx, campaignID)
	if err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeNotFound, "Campaign not found").
			WithDetails("campaign_id", campaignID)
	}

	// Validate state transition
	if err := l.validator.ValidateStateTransition(campaign, types.StateActive); err != nil {
		return nil, err
	}

	// Update campaign state
	if err := campaign.UpdateStatus(types.StateActive); err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeInvalidState, "Failed to update campaign status").
			WithDetails("error", err.Error())
	}

	// Save updated campaign
	if err := l.repository.Update(ctx, campaign); err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeOperationFailed, "Failed to save campaign").
			WithDetails("error", err.Error())
	}

	// Publish event
	event := types.NewCampaignStartedEvent(campaign.ID, startedBy)
	_ = l.eventBus.Publish(ctx, event)

	return campaign, nil
}

// Pause transitions a campaign from active to paused state
func (l *CampaignLifecycle) Pause(ctx context.Context, campaignID string, pausedBy string) (*types.Campaign, error) {
	// Retrieve campaign
	campaign, err := l.repository.FindByID(ctx, campaignID)
	if err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeNotFound, "Campaign not found").
			WithDetails("campaign_id", campaignID)
	}

	// Validate state transition
	if err := l.validator.ValidateStateTransition(campaign, types.StatePaused); err != nil {
		return nil, err
	}

	// Update campaign state
	if err := campaign.UpdateStatus(types.StatePaused); err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeInvalidState, "Failed to update campaign status").
			WithDetails("error", err.Error())
	}

	// Save updated campaign
	if err := l.repository.Update(ctx, campaign); err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeOperationFailed, "Failed to save campaign").
			WithDetails("error", err.Error())
	}

	// Publish event
	event := &types.BaseEvent{
		Type:       types.EventCampaignPaused,
		CampaignID: campaign.ID,
		Timestamp:  time.Now(),
		Data: map[string]interface{}{
			"paused_by": pausedBy,
		},
	}
	_ = l.eventBus.Publish(ctx, event)

	return campaign, nil
}

// Resume transitions a campaign from paused to active state
func (l *CampaignLifecycle) Resume(ctx context.Context, campaignID string, resumedBy string) (*types.Campaign, error) {
	// Retrieve campaign
	campaign, err := l.repository.FindByID(ctx, campaignID)
	if err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeNotFound, "Campaign not found").
			WithDetails("campaign_id", campaignID)
	}

	// Validate current state is paused
	if campaign.Status != types.StatePaused {
		return nil, errors.NewDomainError(errors.ErrCodeInvalidState, "Only paused campaigns can be resumed").
			WithDetails("current_state", campaign.Status)
	}

	// Validate state transition
	if err := l.validator.ValidateStateTransition(campaign, types.StateActive); err != nil {
		return nil, err
	}

	// Update campaign state
	if err := campaign.UpdateStatus(types.StateActive); err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeInvalidState, "Failed to update campaign status").
			WithDetails("error", err.Error())
	}

	// Save updated campaign
	if err := l.repository.Update(ctx, campaign); err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeOperationFailed, "Failed to save campaign").
			WithDetails("error", err.Error())
	}

	// Publish event
	event := &types.BaseEvent{
		Type:       types.EventCampaignResumed,
		CampaignID: campaign.ID,
		Timestamp:  time.Now(),
		Data: map[string]interface{}{
			"resumed_by": resumedBy,
		},
	}
	_ = l.eventBus.Publish(ctx, event)

	return campaign, nil
}

// Complete transitions a campaign to completed state
func (l *CampaignLifecycle) Complete(ctx context.Context, campaignID string, results map[string]interface{}) (*types.Campaign, error) {
	// Retrieve campaign
	campaign, err := l.repository.FindByID(ctx, campaignID)
	if err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeNotFound, "Campaign not found").
			WithDetails("campaign_id", campaignID)
	}

	// Validate state transition
	if err := l.validator.ValidateStateTransition(campaign, types.StateCompleted); err != nil {
		return nil, err
	}

	// Calculate duration if campaign was active
	var duration time.Duration
	if campaign.Status == types.StateActive {
		duration = time.Since(campaign.UpdatedAt)
	}

	// Update campaign state
	if err := campaign.UpdateStatus(types.StateCompleted); err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeInvalidState, "Failed to update campaign status").
			WithDetails("error", err.Error())
	}

	// Save updated campaign
	if err := l.repository.Update(ctx, campaign); err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeOperationFailed, "Failed to save campaign").
			WithDetails("error", err.Error())
	}

	// Publish event
	event := types.NewCampaignCompletedEvent(campaign.ID, duration, results)
	_ = l.eventBus.Publish(ctx, event)

	return campaign, nil
}

// Fail marks a campaign as failed
func (l *CampaignLifecycle) Fail(ctx context.Context, campaignID string, errorMsg string, reason string) (*types.Campaign, error) {
	// Retrieve campaign
	campaign, err := l.repository.FindByID(ctx, campaignID)
	if err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeNotFound, "Campaign not found").
			WithDetails("campaign_id", campaignID)
	}

	// Only active or paused campaigns can fail
	if campaign.Status != types.StateActive && campaign.Status != types.StatePaused {
		return nil, errors.NewDomainError(errors.ErrCodeInvalidState, "Only active or paused campaigns can fail").
			WithDetails("current_state", campaign.Status)
	}

	// Update campaign state to completed (failed campaigns are a type of completion)
	if err := campaign.UpdateStatus(types.StateCompleted); err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeInvalidState, "Failed to update campaign status").
			WithDetails("error", err.Error())
	}

	// Save updated campaign
	if err := l.repository.Update(ctx, campaign); err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeOperationFailed, "Failed to save campaign").
			WithDetails("error", err.Error())
	}

	// Publish event
	event := types.NewCampaignFailedEvent(campaign.ID, errorMsg, reason)
	_ = l.eventBus.Publish(ctx, event)

	return campaign, nil
}

// Delete removes a campaign (only allowed for draft or completed campaigns)
func (l *CampaignLifecycle) Delete(ctx context.Context, campaignID string) error {
	// Retrieve campaign
	campaign, err := l.repository.FindByID(ctx, campaignID)
	if err != nil {
		return errors.NewDomainError(errors.ErrCodeNotFound, "Campaign not found").
			WithDetails("campaign_id", campaignID)
	}

	// Validate deletion
	if err := l.validator.ValidateDeletion(campaign); err != nil {
		return err
	}

	// Delete campaign
	if err := l.repository.Delete(ctx, campaignID); err != nil {
		return errors.NewDomainError(errors.ErrCodeOperationFailed, "Failed to delete campaign").
			WithDetails("error", err.Error())
	}

	// Note: We could publish a deletion event here if needed

	return nil
}

// BulkStateTransition performs state transitions on multiple campaigns
func (l *CampaignLifecycle) BulkStateTransition(ctx context.Context, campaignIDs []string, targetState types.State, operator string) (map[string]error, error) {
	if len(campaignIDs) == 0 {
		return nil, errors.NewDomainError(errors.ErrCodeInvalidInput, "No campaigns specified")
	}

	// Validate target state
	if err := targetState.Validate(); err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeInvalidInput, "Invalid target state").
			WithDetails("state", targetState).
			WithDetails("error", err.Error())
	}

	results := make(map[string]error)

	for _, campaignID := range campaignIDs {
		var err error

		switch targetState {
		case types.StateActive:
			// Check if we're resuming or starting
			campaign, findErr := l.repository.FindByID(ctx, campaignID)
			if findErr != nil {
				results[campaignID] = findErr
				continue
			}

			if campaign.Status == types.StatePaused {
				_, err = l.Resume(ctx, campaignID, operator)
			} else {
				_, err = l.Start(ctx, campaignID, operator)
			}

		case types.StatePaused:
			_, err = l.Pause(ctx, campaignID, operator)

		case types.StateCompleted:
			_, err = l.Complete(ctx, campaignID, nil)

		default:
			err = errors.NewDomainError(errors.ErrCodeInvalidInput, "Unsupported bulk state transition").
				WithDetails("state", targetState)
		}

		if err != nil {
			results[campaignID] = err
		}
	}

	return results, nil
}

// GetActiveCount returns the number of active campaigns
func (l *CampaignLifecycle) GetActiveCount(ctx context.Context) (int, error) {
	count, err := l.repository.CountByStatus(ctx, types.StateActive)
	if err != nil {
		return 0, errors.NewDomainError(errors.ErrCodeOperationFailed, "Failed to count active campaigns").
			WithDetails("error", err.Error())
	}
	return count, nil
}

// CanStartNewCampaign checks if a new campaign can be started based on resource limits
func (l *CampaignLifecycle) CanStartNewCampaign(ctx context.Context, maxActiveCampaigns int) (bool, error) {
	if maxActiveCampaigns <= 0 {
		return true, nil // No limit
	}

	activeCount, err := l.GetActiveCount(ctx)
	if err != nil {
		return false, err
	}

	return activeCount < maxActiveCampaigns, nil
}
