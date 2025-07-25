package service

import (
	"context"

	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
	"github.com/ethpandaops/pandafuzz/pkg/shared/errors"
)

// CampaignCreator handles campaign creation logic
type CampaignCreator struct {
	repository repository.CampaignRepository
	validator  *CampaignValidator
	eventBus   EventPublisher
}

// EventPublisher interface for publishing domain events
type EventPublisher interface {
	Publish(ctx context.Context, event types.Event) error
}

// NewCampaignCreator creates a new campaign creator service
func NewCampaignCreator(
	repo repository.CampaignRepository,
	validator *CampaignValidator,
	eventBus EventPublisher,
) *CampaignCreator {
	return &CampaignCreator{
		repository: repo,
		validator:  validator,
		eventBus:   eventBus,
	}
}

// CreateOptions contains options for campaign creation
type CreateOptions struct {
	ID          string
	Name        string
	Description string
	CreatedBy   string
	Metadata    map[string]interface{}
}

// Create creates a new campaign
func (c *CampaignCreator) Create(ctx context.Context, opts CreateOptions) (*types.Campaign, error) {
	// Create campaign instance
	campaign, err := types.NewCampaign(opts.ID, opts.Name, opts.Description)
	if err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeInvalidInput, "Failed to create campaign").
			WithDetails("error", err.Error())
	}

	// Validate the campaign
	if err := c.validator.ValidateCreate(campaign); err != nil {
		return nil, err
	}

	// Check if campaign with same ID already exists
	exists, err := c.repository.Exists(ctx, campaign.ID)
	if err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeOperationFailed, "Failed to check campaign existence").
			WithDetails("error", err.Error())
	}
	if exists {
		return nil, errors.NewDomainError(errors.ErrCodeAlreadyExists, "Campaign with this ID already exists").
			WithDetails("campaign_id", campaign.ID)
	}

	// Check if campaign with same name already exists
	existingCampaigns, err := c.repository.FindByName(ctx, campaign.Name)
	if err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeOperationFailed, "Failed to check campaign name").
			WithDetails("error", err.Error())
	}
	for _, existing := range existingCampaigns {
		if existing.Name == campaign.Name {
			return nil, errors.NewDomainError(errors.ErrCodeAlreadyExists, "Campaign with this name already exists").
				WithDetails("campaign_name", campaign.Name)
		}
	}

	// Save the campaign
	if err := c.repository.Create(ctx, campaign); err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeOperationFailed, "Failed to save campaign").
			WithDetails("error", err.Error())
	}

	// Publish campaign created event
	event := types.NewCampaignCreatedEvent(campaign)
	if err := c.eventBus.Publish(ctx, event); err != nil {
		// Log the error but don't fail the operation
		// In a production system, this might use a proper logger
		// For now, we'll just continue
	}

	return campaign, nil
}

// CreateBatch creates multiple campaigns in a single operation
func (c *CampaignCreator) CreateBatch(ctx context.Context, options []CreateOptions) ([]*types.Campaign, error) {
	if len(options) == 0 {
		return nil, errors.NewDomainError(errors.ErrCodeInvalidInput, "No campaigns to create")
	}

	campaigns := make([]*types.Campaign, 0, len(options))

	// First, validate all campaigns
	for _, opts := range options {
		campaign, err := types.NewCampaign(opts.ID, opts.Name, opts.Description)
		if err != nil {
			return nil, errors.NewDomainError(errors.ErrCodeInvalidInput, "Failed to create campaign").
				WithDetails("campaign_id", opts.ID).
				WithDetails("error", err.Error())
		}

		if err := c.validator.ValidateCreate(campaign); err != nil {
			return nil, err
		}

		campaigns = append(campaigns, campaign)
	}

	// Check for duplicates within the batch
	nameMap := make(map[string]bool)
	idMap := make(map[string]bool)
	for _, campaign := range campaigns {
		if nameMap[campaign.Name] {
			return nil, errors.NewDomainError(errors.ErrCodeInvalidInput, "Duplicate campaign name in batch").
				WithDetails("campaign_name", campaign.Name)
		}
		if idMap[campaign.ID] {
			return nil, errors.NewDomainError(errors.ErrCodeInvalidInput, "Duplicate campaign ID in batch").
				WithDetails("campaign_id", campaign.ID)
		}
		nameMap[campaign.Name] = true
		idMap[campaign.ID] = true
	}

	// Check existence in database
	for _, campaign := range campaigns {
		exists, err := c.repository.Exists(ctx, campaign.ID)
		if err != nil {
			return nil, errors.NewDomainError(errors.ErrCodeOperationFailed, "Failed to check campaign existence").
				WithDetails("campaign_id", campaign.ID).
				WithDetails("error", err.Error())
		}
		if exists {
			return nil, errors.NewDomainError(errors.ErrCodeAlreadyExists, "Campaign already exists").
				WithDetails("campaign_id", campaign.ID)
		}
	}

	// Create all campaigns
	createdCampaigns := make([]*types.Campaign, 0, len(campaigns))
	for _, campaign := range campaigns {
		if err := c.repository.Create(ctx, campaign); err != nil {
			// Rollback created campaigns
			for _, created := range createdCampaigns {
				_ = c.repository.Delete(ctx, created.ID)
			}
			return nil, errors.NewDomainError(errors.ErrCodeOperationFailed, "Failed to create campaign batch").
				WithDetails("failed_campaign_id", campaign.ID).
				WithDetails("error", err.Error())
		}
		createdCampaigns = append(createdCampaigns, campaign)

		// Publish event
		event := types.NewCampaignCreatedEvent(campaign)
		_ = c.eventBus.Publish(ctx, event)
	}

	return createdCampaigns, nil
}

// DuplicateCampaign creates a new campaign by copying an existing one
func (c *CampaignCreator) DuplicateCampaign(ctx context.Context, sourceID, newID, newName string) (*types.Campaign, error) {
	// Retrieve source campaign
	source, err := c.repository.FindByID(ctx, sourceID)
	if err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeNotFound, "Source campaign not found").
			WithDetails("source_id", sourceID)
	}

	// Create new campaign with copied data
	newCampaign, err := types.NewCampaign(newID, newName, source.Description)
	if err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeInvalidInput, "Failed to create duplicate campaign").
			WithDetails("error", err.Error())
	}

	// Validate the new campaign
	if err := c.validator.ValidateCreate(newCampaign); err != nil {
		return nil, err
	}

	// Check if new campaign ID already exists
	exists, err := c.repository.Exists(ctx, newCampaign.ID)
	if err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeOperationFailed, "Failed to check campaign existence").
			WithDetails("error", err.Error())
	}
	if exists {
		return nil, errors.NewDomainError(errors.ErrCodeAlreadyExists, "Campaign with new ID already exists").
			WithDetails("campaign_id", newCampaign.ID)
	}

	// Save the new campaign
	if err := c.repository.Create(ctx, newCampaign); err != nil {
		return nil, errors.NewDomainError(errors.ErrCodeOperationFailed, "Failed to save duplicated campaign").
			WithDetails("error", err.Error())
	}

	// Publish event
	event := types.NewCampaignCreatedEvent(newCampaign)
	_ = c.eventBus.Publish(ctx, event)

	return newCampaign, nil
}
