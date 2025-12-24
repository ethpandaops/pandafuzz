package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/sse"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	campaignTypes "github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
)

// CampaignAdapter implements the campaign-related endpoints of the generated ServerInterface
type CampaignAdapter struct {
	service    common.CampaignService
	repository repository.CampaignRepository
	sse        *sse.Manager
	logger     logrus.FieldLogger
}

// NewCampaignAdapter creates a new campaign adapter
func NewCampaignAdapter(
	service common.CampaignService,
	repository repository.CampaignRepository,
	sse *sse.Manager,
	logger logrus.FieldLogger,
) *CampaignAdapter {
	return &CampaignAdapter{
		service:    service,
		repository: repository,
		sse:        sse,
		logger:     logger.WithField("component", "campaign_adapter"),
	}
}

// ListCampaigns retrieves all campaigns with filtering and pagination
func (a *CampaignAdapter) ListCampaigns(w http.ResponseWriter, r *http.Request, params generated.ListCampaignsParams) {
	ctx := r.Context()

	// Set defaults for pagination
	limit := 50
	offset := 0

	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
		if limit > 1000 {
			limit = 1000
		}
	}

	if params.Offset != nil && *params.Offset >= 0 {
		offset = *params.Offset
	}

	var apiCampaigns []generated.Campaign
	var total int

	// Try repository first, fall back to service if repository is nil
	if a.repository != nil {
		// Get campaigns from repository
		campaigns, t, err := a.repository.List(ctx, offset, limit)
		if err != nil {
			a.logger.WithError(err).Error("failed to list campaigns from repository")
			a.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve campaigns", err)
			return
		}
		total = t

		// Filter by status if specified
		if params.Status != nil {
			filtered := make([]*campaignTypes.Campaign, 0, len(campaigns))
			for _, campaign := range campaigns {
				if campaignStatusToGenerated(campaign.Status) == *params.Status {
					filtered = append(filtered, campaign)
				}
			}
			campaigns = filtered
		}

		// Convert to API types
		apiCampaigns = make([]generated.Campaign, len(campaigns))
		for i, campaign := range campaigns {
			apiCampaigns[i] = a.convertCampaignToAPI(campaign)
		}
	} else if a.service != nil {
		// Use service if repository is not available
		filters := common.CampaignFilters{}
		if params.Status != nil {
			filters.Status = string(*params.Status)
		}

		campaigns, err := a.service.List(ctx, filters)
		if err != nil {
			a.logger.WithError(err).Error("failed to list campaigns from service")
			a.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve campaigns", err)
			return
		}
		total = len(campaigns)

		// Apply pagination manually
		start := offset
		if start > len(campaigns) {
			start = len(campaigns)
		}
		end := offset + limit
		if end > len(campaigns) {
			end = len(campaigns)
		}
		paginatedCampaigns := campaigns[start:end]

		// Convert to API types
		apiCampaigns = make([]generated.Campaign, len(paginatedCampaigns))
		for i, campaign := range paginatedCampaigns {
			apiCampaigns[i] = a.convertCommonCampaignToAPI(campaign)
		}
	} else {
		// Neither repository nor service available
		a.logger.Error("neither repository nor service is available for listing campaigns")
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Campaign service not available", nil)
		return
	}

	// Create pagination info
	hasMore := offset+len(apiCampaigns) < total
	pagination := generated.Pagination{
		Limit:   limit,
		Offset:  offset,
		Total:   total,
		HasMore: hasMore,
	}

	response := generated.CampaignListResponse{
		Data:       apiCampaigns,
		Pagination: pagination,
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// CreateCampaign creates a new fuzzing campaign
func (a *CampaignAdapter) CreateCampaign(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req generated.CampaignCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Create new campaign with domain types
	description := ""
	if req.Description != nil {
		description = *req.Description
	}

	campaignID := uuid.New().String()
	campaign, err := campaignTypes.NewCampaign(campaignID, req.Name, description)
	if err != nil {
		a.logger.WithError(err).Error("failed to create campaign")
		a.writeError(w, http.StatusBadRequest, "CAMPAIGN_CREATION_FAILED", "Failed to create campaign", err)
		return
	}

	// Create common.Campaign for service with all fields
	commonCampaign := &common.Campaign{
		ID:           campaign.ID,
		Name:         campaign.Name,
		Description:  campaign.Description,
		TargetBinary: req.TargetBinary,
		Status:       common.CampaignStatusPending,
		CreatedAt:    campaign.CreatedAt,
		UpdatedAt:    campaign.UpdatedAt,
	}

	// Set optional fields on common campaign
	if req.MaxJobs != nil {
		commonCampaign.MaxJobs = *req.MaxJobs
	}

	if req.MaxDurationSeconds != nil {
		commonCampaign.MaxDuration = time.Duration(*req.MaxDurationSeconds) * time.Second
	}

	if req.AutoRestart != nil {
		commonCampaign.AutoRestart = *req.AutoRestart
	}

	if req.SharedCorpus != nil {
		commonCampaign.SharedCorpus = *req.SharedCorpus
	}

	if req.Tags != nil {
		commonCampaign.Tags = *req.Tags
	}

	// Save campaign using service
	if err := a.service.Create(ctx, commonCampaign); err != nil {
		a.logger.WithError(err).Error("failed to save campaign")
		a.writeError(w, http.StatusInternalServerError, "SAVE_FAILED", "Failed to save campaign", err)
		return
	}

	apiCampaign := a.convertCommonCampaignToAPI(commonCampaign)

	// Publish SSE event
	campaignUUID := uuid.MustParse(commonCampaign.ID)
	event := sse.NewCampaignEvent("campaign.created", campaignUUID, map[string]any{
		"campaign":  apiCampaign,
		"timestamp": time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast campaign created event")
	}

	a.writeJSONResponse(w, http.StatusCreated, apiCampaign)
}

// GetCampaign retrieves a specific campaign by ID
func (a *CampaignAdapter) GetCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam, params generated.GetCampaignParams) {
	ctx := r.Context()

	// Try repository first, fall back to service
	if a.repository != nil {
		campaign, err := a.repository.FindByID(ctx, campaignId.String())
		if err != nil {
			a.logger.WithError(err).WithField("campaign_id", campaignId).Error("failed to get campaign")
			a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
			return
		}
		apiCampaign := a.convertCampaignToAPI(campaign)
		a.writeJSONResponse(w, http.StatusOK, apiCampaign)
		return
	}

	// Fall back to service
	if a.service != nil {
		campaign, err := a.service.Get(ctx, campaignId.String())
		if err != nil {
			a.logger.WithError(err).WithField("campaign_id", campaignId).Error("failed to get campaign")
			a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
			return
		}
		apiCampaign := a.convertCommonCampaignToAPI(campaign)
		a.writeJSONResponse(w, http.StatusOK, apiCampaign)
		return
	}

	a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Campaign service not available", nil)
}

// UpdateCampaign updates an existing campaign
func (a *CampaignAdapter) UpdateCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	ctx := r.Context()

	var req generated.CampaignUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Get existing campaign from service (returns common.Campaign)
	campaign, err := a.service.Get(ctx, campaignId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
		return
	}

	// Check if campaign can be updated
	if campaign.Status == common.CampaignStatusRunning {
		a.writeError(w, http.StatusConflict, "INVALID_STATUS", "Cannot update running campaign", nil)
		return
	}

	// Update fields if provided
	if req.Name != nil {
		campaign.Name = *req.Name
	}

	if req.Description != nil {
		campaign.Description = *req.Description
	}

	if req.MaxJobs != nil {
		campaign.MaxJobs = *req.MaxJobs
	}

	if req.MaxDurationSeconds != nil {
		campaign.MaxDuration = time.Duration(*req.MaxDurationSeconds) * time.Second
	}

	if req.AutoRestart != nil {
		campaign.AutoRestart = *req.AutoRestart
	}

	if req.SharedCorpus != nil {
		campaign.SharedCorpus = *req.SharedCorpus
	}

	if req.Tags != nil {
		campaign.Tags = *req.Tags
	}

	// Save changes using service Update method
	updates := common.CampaignUpdates{
		Name:         &campaign.Name,
		Description:  &campaign.Description,
		MaxJobs:      &campaign.MaxJobs,
		MaxDuration:  &campaign.MaxDuration,
		AutoRestart:  &campaign.AutoRestart,
		SharedCorpus: &campaign.SharedCorpus,
		Tags:         campaign.Tags,
	}
	if err := a.service.Update(ctx, campaign.ID, updates); err != nil {
		a.logger.WithError(err).Error("failed to update campaign")
		a.writeError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update campaign", err)
		return
	}

	apiCampaign := a.convertCommonCampaignToAPI(campaign)

	// Publish SSE event
	campaignUUID := uuid.MustParse(campaign.ID)
	event := sse.NewCampaignEvent("campaign.updated", campaignUUID, map[string]any{
		"campaign":  apiCampaign,
		"timestamp": time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast campaign updated event")
	}

	a.writeJSONResponse(w, http.StatusOK, apiCampaign)
}

// DeleteCampaign deletes a campaign
func (a *CampaignAdapter) DeleteCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	ctx := r.Context()

	var campaignID string
	var isRunning bool

	// Get campaign to check its status - try repository first, then service
	if a.repository != nil {
		campaign, err := a.repository.FindByID(ctx, campaignId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
			return
		}
		campaignID = campaign.ID
		isRunning = campaign.Status == campaignTypes.StateActive
	} else if a.service != nil {
		campaign, err := a.service.Get(ctx, campaignId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
			return
		}
		campaignID = campaign.ID
		isRunning = campaign.Status == common.CampaignStatusRunning
	} else {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Campaign service not available", nil)
		return
	}

	// Check if campaign can be deleted
	if isRunning {
		a.writeError(w, http.StatusConflict, "INVALID_STATUS", "Cannot delete running campaign", nil)
		return
	}

	// Delete campaign using service
	if a.service == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Campaign service not available", nil)
		return
	}

	if err := a.service.Delete(ctx, campaignID); err != nil {
		a.logger.WithError(err).Error("failed to delete campaign")
		a.writeError(w, http.StatusInternalServerError, "DELETE_FAILED", "Failed to delete campaign", err)
		return
	}

	// Publish SSE event
	campaignUUID := uuid.MustParse(campaignID)
	event := sse.NewCampaignEvent("campaign.deleted", campaignUUID, map[string]any{
		"campaign_id": campaignID,
		"timestamp":   time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast campaign deleted event")
	}

	w.WriteHeader(http.StatusNoContent)
}

// StartCampaign starts a campaign
func (a *CampaignAdapter) StartCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	ctx := r.Context()

	var campaignID string
	var isRunning bool
	var apiCampaign generated.Campaign

	// Get campaign - try repository first, then service
	if a.repository != nil {
		campaign, err := a.repository.FindByID(ctx, campaignId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
			return
		}
		campaignID = campaign.ID
		isRunning = campaign.Status == campaignTypes.StateActive
		apiCampaign = a.convertCampaignToAPI(campaign)
	} else if a.service != nil {
		campaign, err := a.service.Get(ctx, campaignId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
			return
		}
		campaignID = campaign.ID
		isRunning = campaign.Status == common.CampaignStatusRunning
		apiCampaign = a.convertCommonCampaignToAPI(campaign)
	} else {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Campaign service not available", nil)
		return
	}

	// Check if campaign can be started
	if isRunning {
		a.writeError(w, http.StatusConflict, "ALREADY_RUNNING", "Campaign is already running", nil)
		return
	}

	// Start campaign using service
	if a.service == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Campaign service not available", nil)
		return
	}

	if err := a.service.RestartCampaign(ctx, campaignID); err != nil {
		a.logger.WithError(err).Error("failed to start campaign")
		a.writeError(w, http.StatusInternalServerError, "START_FAILED", "Failed to start campaign", err)
		return
	}

	// Get updated campaign for response
	if a.repository != nil {
		updatedCampaign, err := a.repository.FindByID(ctx, campaignId.String())
		if err == nil {
			apiCampaign = a.convertCampaignToAPI(updatedCampaign)
		}
	} else if a.service != nil {
		updatedCampaign, err := a.service.Get(ctx, campaignId.String())
		if err == nil {
			apiCampaign = a.convertCommonCampaignToAPI(updatedCampaign)
		}
	}

	// Publish SSE event
	campaignUUID := uuid.MustParse(campaignID)
	event := sse.NewCampaignEvent("campaign.started", campaignUUID, map[string]any{
		"campaign":  apiCampaign,
		"timestamp": time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast campaign started event")
	}

	a.writeJSONResponse(w, http.StatusOK, apiCampaign)
}

// StopCampaign stops a running campaign
func (a *CampaignAdapter) StopCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	ctx := r.Context()

	var req generated.StopCampaignJSONBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Empty body is acceptable for stop
		req = generated.StopCampaignJSONBody{}
	}

	var campaignID string
	var isRunning bool
	var apiCampaign generated.Campaign

	// Get campaign - try repository first, then service
	if a.repository != nil {
		campaign, err := a.repository.FindByID(ctx, campaignId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
			return
		}
		campaignID = campaign.ID
		isRunning = campaign.Status == campaignTypes.StateActive
		apiCampaign = a.convertCampaignToAPI(campaign)
	} else if a.service != nil {
		campaign, err := a.service.Get(ctx, campaignId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
			return
		}
		campaignID = campaign.ID
		isRunning = campaign.Status == common.CampaignStatusRunning
		apiCampaign = a.convertCommonCampaignToAPI(campaign)
	} else {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Campaign service not available", nil)
		return
	}

	// Check if campaign is running
	if !isRunning {
		a.writeError(w, http.StatusConflict, "NOT_RUNNING", "Campaign is not running", nil)
		return
	}

	reason := "Manual stop"
	if req.Reason != nil {
		reason = *req.Reason
	}

	// Stop campaign using service
	if a.service == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Campaign service not available", nil)
		return
	}

	pausedStatus := common.CampaignStatusPaused
	updates := common.CampaignUpdates{Status: &pausedStatus}
	if err := a.service.Update(ctx, campaignID, updates); err != nil {
		a.logger.WithError(err).Error("failed to stop campaign")
		a.writeError(w, http.StatusInternalServerError, "STOP_FAILED", "Failed to stop campaign", err)
		return
	}

	// Get updated campaign for response
	if a.repository != nil {
		updatedCampaign, err := a.repository.FindByID(ctx, campaignId.String())
		if err == nil {
			apiCampaign = a.convertCampaignToAPI(updatedCampaign)
		}
	} else if a.service != nil {
		updatedCampaign, err := a.service.Get(ctx, campaignId.String())
		if err == nil {
			apiCampaign = a.convertCommonCampaignToAPI(updatedCampaign)
		}
	}

	// Publish SSE event
	campaignUUID := uuid.MustParse(campaignID)
	event := sse.NewCampaignEvent("campaign.stopped", campaignUUID, map[string]any{
		"campaign":  apiCampaign,
		"reason":    reason,
		"timestamp": time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast campaign stopped event")
	}

	a.writeJSONResponse(w, http.StatusOK, apiCampaign)
}

// GetCampaignStats retrieves campaign statistics
func (a *CampaignAdapter) GetCampaignStats(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	ctx := r.Context()

	// Verify campaign exists - try repository first, then service
	var campaign *campaignTypes.Campaign
	if a.repository != nil {
		c, err := a.repository.FindByID(ctx, campaignId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
			return
		}
		campaign = c
	} else if a.service != nil {
		c, err := a.service.Get(ctx, campaignId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
			return
		}
		// Convert common.Campaign to campaignTypes.Campaign for stats
		campaign = &campaignTypes.Campaign{
			ID:          c.ID,
			Name:        c.Name,
			Description: c.Description,
			CreatedAt:   c.CreatedAt,
			UpdatedAt:   c.UpdatedAt,
		}
	} else {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Campaign service not available", nil)
		return
	}

	// Get campaign statistics
	stats := a.getCampaignStats(ctx, campaign)

	a.writeJSONResponse(w, http.StatusOK, stats)
}

// Helper methods

func (a *CampaignAdapter) convertCampaignToAPI(campaign *campaignTypes.Campaign) generated.Campaign {
	// Basic conversion from domain Campaign type
	// Note: Domain Campaign is minimal, many fields need to come from service
	apiCampaign := generated.Campaign{
		Id:           uuid.MustParse(campaign.ID),
		Name:         campaign.Name,
		TargetBinary: "", // Domain type doesn't have this, set empty
		Status:       campaignStatusToGenerated(campaign.Status),
		CreatedAt:    campaign.CreatedAt,
	}

	if campaign.Description != "" {
		apiCampaign.Description = &campaign.Description
	}

	// Domain Campaign doesn't have these fields
	// They would need to come from common.Campaign via service

	return apiCampaign
}

func (a *CampaignAdapter) convertCommonCampaignToAPI(campaign *common.Campaign) generated.Campaign {
	// Full conversion from common.Campaign type
	apiCampaign := generated.Campaign{
		Id:           uuid.MustParse(campaign.ID),
		Name:         campaign.Name,
		TargetBinary: campaign.TargetBinary,
		Status:       commonCampaignStatusToGenerated(campaign.Status),
		CreatedAt:    campaign.CreatedAt,
	}

	if campaign.Description != "" {
		apiCampaign.Description = &campaign.Description
	}

	if campaign.CompletedAt != nil {
		apiCampaign.CompletedAt = campaign.CompletedAt
	}

	if campaign.MaxJobs > 0 {
		apiCampaign.MaxJobs = &campaign.MaxJobs
	}

	if campaign.MaxDuration > 0 {
		seconds := int(campaign.MaxDuration.Seconds())
		apiCampaign.MaxDurationSeconds = &seconds
	}

	if campaign.AutoRestart {
		apiCampaign.AutoRestart = &campaign.AutoRestart
	}

	if campaign.SharedCorpus {
		apiCampaign.SharedCorpus = &campaign.SharedCorpus
	}

	if len(campaign.Tags) > 0 {
		apiCampaign.Tags = &campaign.Tags
	}

	return apiCampaign
}

func (a *CampaignAdapter) getCampaignStats(ctx context.Context, campaign *campaignTypes.Campaign) generated.CampaignStats {
	// Mock implementation - in reality, this would aggregate from various services
	stats := generated.CampaignStats{
		CampaignId:           uuid.MustParse(campaign.ID),
		TotalJobs:            10,
		ActiveJobs:           2,
		CompletedJobs:        7,
		FailedJobs:           1,
		CorpusSize:           150,
		CorpusSizeBytes:      &[]int{1024 * 1024}[0], // 1MB
		TotalCrashes:         5,
		UniqueCrashes:        3,
		TotalCoverageEdges:   1000,
		ExecutionTimeSeconds: 3600, // 1 hour
		LastUpdated:          time.Now(),
	}

	return stats
}

// Status conversion helpers
func campaignStatusToGenerated(status campaignTypes.State) generated.CampaignStatus {
	switch status {
	case campaignTypes.StateDraft:
		return "draft"
	case campaignTypes.StateActive:
		return "active"
	case campaignTypes.StatePaused:
		return "paused"
	case campaignTypes.StateCompleted:
		return "completed"
	default:
		return "draft"
	}
}

func commonCampaignStatusToGenerated(status common.CampaignStatus) generated.CampaignStatus {
	switch status {
	case common.CampaignStatusPending:
		return "draft"
	case common.CampaignStatusRunning:
		return "active"
	case common.CampaignStatusCompleted:
		return "completed"
	case common.CampaignStatusFailed:
		return "completed" // Map failed to completed
	case common.CampaignStatusPaused:
		return "paused"
	case common.CampaignStatusCancelled:
		return "cancelled"
	default:
		return "draft"
	}
}

func generatedToCampaignStatus(status generated.CampaignStatus) campaignTypes.State {
	switch status {
	case "draft":
		return campaignTypes.StateDraft
	case "active":
		return campaignTypes.StateActive
	case "paused":
		return campaignTypes.StatePaused
	case "completed":
		return campaignTypes.StateCompleted
	case "cancelled":
		return campaignTypes.StateCompleted // Map cancelled to completed since no cancelled state
	case "archived":
		return campaignTypes.StateCompleted // Map archived to completed since no archived state
	default:
		return campaignTypes.StateDraft
	}
}

func generatedToCommonCampaignStatus(status generated.CampaignStatus) common.CampaignStatus {
	switch status {
	case "draft":
		return common.CampaignStatusPending
	case "active":
		return common.CampaignStatusRunning
	case "paused":
		return common.CampaignStatusPaused
	case "completed":
		return common.CampaignStatusCompleted
	case "cancelled":
		return common.CampaignStatusCancelled
	case "archived":
		return common.CampaignStatusCompleted
	default:
		return common.CampaignStatusPending
	}
}

func (a *CampaignAdapter) writeJSONResponse(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		a.logger.WithError(err).Error("failed to encode JSON response")
	}
}

func (a *CampaignAdapter) writeError(w http.ResponseWriter, statusCode int, errorType, title string, err error) {
	problem := generated.ProblemDetails{
		Type:      fmt.Sprintf("/errors/%s", strings.ToLower(errorType)),
		Title:     title,
		Status:    statusCode,
		Timestamp: &[]time.Time{time.Now()}[0],
	}

	if err != nil {
		detail := err.Error()
		problem.Detail = &detail
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)
	if encodeErr := json.NewEncoder(w).Encode(problem); encodeErr != nil {
		a.logger.WithError(encodeErr).Error("failed to encode error response")
	}
}
