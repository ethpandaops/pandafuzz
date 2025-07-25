package commands

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/service"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
)

// CreateCampaignCommand represents a command to create a new campaign
type CreateCampaignCommand struct {
	ID          string                 `json:"id" validate:"required"`
	Name        string                 `json:"name" validate:"required,min=3,max=100"`
	Description string                 `json:"description" validate:"max=500"`
	CreatedBy   string                 `json:"created_by" validate:"required"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CreateCampaignResult represents the result of campaign creation
type CreateCampaignResult struct {
	Campaign  *CampaignDTO `json:"campaign"`
	EventID   string       `json:"event_id"`
	CreatedAt time.Time    `json:"created_at"`
}

// CampaignDTO represents campaign data for API responses
type CampaignDTO struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Status      string                 `json:"status"`
	CreatedBy   string                 `json:"created_by"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CreateCampaignHandler handles campaign creation commands
type CreateCampaignHandler struct {
	creator  *service.CampaignCreator
	eventBus service.EventPublisher
}

// NewCreateCampaignHandler creates a new handler instance
func NewCreateCampaignHandler(
	repo repository.CampaignRepository,
	validator *service.CampaignValidator,
	eventBus service.EventPublisher,
) *CreateCampaignHandler {
	creator := service.NewCampaignCreator(repo, validator, eventBus)
	return &CreateCampaignHandler{
		creator:  creator,
		eventBus: eventBus,
	}
}

// Handle executes the create campaign command
func (h *CreateCampaignHandler) Handle(ctx context.Context, cmd interface{}) error {
	command, ok := cmd.(*CreateCampaignCommand)
	if !ok {
		return NewApplicationError(
			ErrCodeInvalidCommand,
			"Invalid command type",
			nil,
		).WithDetails("expected", "*CreateCampaignCommand")
	}

	// Validate command
	if err := h.validateCommand(command); err != nil {
		return err
	}

	// Check authorization
	if err := h.checkAuthorization(ctx, command); err != nil {
		return err
	}

	// Create campaign through domain service
	createOpts := service.CreateOptions{
		ID:          command.ID,
		Name:        command.Name,
		Description: command.Description,
		CreatedBy:   command.CreatedBy,
		Metadata:    command.Metadata,
	}

	campaign, err := h.creator.Create(ctx, createOpts)
	if err != nil {
		return NewApplicationError(
			ErrCodeOperationFailed,
			"Failed to create campaign",
			err,
		).WithDetails("campaign_id", command.ID)
	}

	// Additional application-level event publishing can be done here
	// The domain service already publishes domain events

	return nil
}

// HandleWithResult executes the command and returns the result
func (h *CreateCampaignHandler) HandleWithResult(ctx context.Context, cmd *CreateCampaignCommand) (*CreateCampaignResult, error) {
	// Validate command
	if err := h.validateCommand(cmd); err != nil {
		return nil, err
	}

	// Check authorization
	if err := h.checkAuthorization(ctx, cmd); err != nil {
		return nil, err
	}

	// Create campaign through domain service
	createOpts := service.CreateOptions{
		ID:          cmd.ID,
		Name:        cmd.Name,
		Description: cmd.Description,
		CreatedBy:   cmd.CreatedBy,
		Metadata:    cmd.Metadata,
	}

	campaign, err := h.creator.Create(ctx, createOpts)
	if err != nil {
		return nil, NewApplicationError(
			ErrCodeOperationFailed,
			"Failed to create campaign",
			err,
		).WithDetails("campaign_id", cmd.ID)
	}

	// Convert to DTO
	dto := h.toDTO(campaign, cmd.CreatedBy, cmd.Metadata)

	result := &CreateCampaignResult{
		Campaign:  dto,
		EventID:   generateEventID(),
		CreatedAt: campaign.CreatedAt,
	}

	return result, nil
}

// validateCommand validates the create campaign command
func (h *CreateCampaignHandler) validateCommand(cmd *CreateCampaignCommand) error {
	if cmd.ID == "" {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"Campaign ID is required",
			nil,
		).WithDetails("field", "id")
	}

	if cmd.Name == "" {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"Campaign name is required",
			nil,
		).WithDetails("field", "name")
	}

	if len(cmd.Name) < 3 || len(cmd.Name) > 100 {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"Campaign name must be between 3 and 100 characters",
			nil,
		).WithDetails("field", "name").WithDetails("length", len(cmd.Name))
	}

	if len(cmd.Description) > 500 {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"Campaign description must not exceed 500 characters",
			nil,
		).WithDetails("field", "description").WithDetails("length", len(cmd.Description))
	}

	if cmd.CreatedBy == "" {
		return NewApplicationError(
			ErrCodeValidationFailed,
			"CreatedBy is required",
			nil,
		).WithDetails("field", "created_by")
	}

	return nil
}

// checkAuthorization checks if the user is authorized to create campaigns
func (h *CreateCampaignHandler) checkAuthorization(ctx context.Context, cmd *CreateCampaignCommand) error {
	// In a real implementation, this would check:
	// - User permissions from context
	// - Rate limits
	// - Quota limits
	// - Organization policies

	// For now, we'll just ensure the context has a user
	userID := getUserIDFromContext(ctx)
	if userID == "" {
		return NewApplicationError(
			ErrCodeUnauthorized,
			"User not authenticated",
			nil,
		)
	}

	// Ensure the user is creating a campaign for themselves
	// In a multi-tenant system, this might check organization membership
	if userID != cmd.CreatedBy {
		return NewApplicationError(
			ErrCodeUnauthorized,
			"Cannot create campaign for another user",
			nil,
		).WithDetails("user_id", userID).WithDetails("created_by", cmd.CreatedBy)
	}

	return nil
}

// toDTO converts domain model to DTO
func (h *CreateCampaignHandler) toDTO(campaign *types.Campaign, createdBy string, metadata map[string]interface{}) *CampaignDTO {
	return &CampaignDTO{
		ID:          campaign.ID,
		Name:        campaign.Name,
		Description: campaign.Description,
		Status:      campaign.Status.String(),
		CreatedBy:   createdBy,
		CreatedAt:   campaign.CreatedAt,
		UpdatedAt:   campaign.UpdatedAt,
		Metadata:    metadata,
	}
}

// Helper functions

// getUserIDFromContext extracts user ID from context
func getUserIDFromContext(ctx context.Context) string {
	// In a real implementation, this would extract from:
	// - JWT claims
	// - Session data
	// - Request headers
	if userID, ok := ctx.Value("user_id").(string); ok {
		return userID
	}
	return ""
}

// generateEventID generates a unique event ID
func generateEventID() string {
	// In a real implementation, use a proper ID generator
	return "evt_" + time.Now().Format("20060102150405")
}

// Error codes specific to this command
const (
	ErrCodeInvalidCommand   = "INVALID_COMMAND"
	ErrCodeValidationFailed = "VALIDATION_FAILED"
	ErrCodeUnauthorized     = "UNAUTHORIZED"
	ErrCodeOperationFailed  = "OPERATION_FAILED"
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
		e.Details = make(map[string]interface{})
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
