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
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	campaignService "github.com/ethpandaops/pandafuzz/pkg/domain/campaign/service"
	campaignTypes "github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
)

// CampaignAdapter implements the campaign-related endpoints of the generated ServerInterface
type CampaignAdapter struct {
	service    *campaignService.Service
	repository repository.CampaignRepository
	sse        *sse.Manager
	logger     logrus.FieldLogger
}

// NewCampaignAdapter creates a new campaign adapter
func NewCampaignAdapter(
	service *campaignService.Service,
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

	// Get campaigns from repository
	campaigns, total, err := a.repository.List(ctx, offset, limit)
	if err != nil {
		a.logger.WithError(err).Error("failed to list campaigns")
		a.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve campaigns", err)
		return
	}

	// Filter by status if specified
	if params.Status != nil {
		filtered := make([]*campaignTypes.Campaign, 0)
		for _, campaign := range campaigns {
			if campaignStatusToGenerated(campaign.Status) == *params.Status {
				filtered = append(filtered, campaign)
			}
		}
		campaigns = filtered
	}

	// Convert to API types
	apiCampaigns := make([]generated.Campaign, len(campaigns))
	for i, campaign := range campaigns {
		apiCampaigns[i] = a.convertCampaignToAPI(campaign)
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

	// Create new campaign
	campaign, err := campaignTypes.NewCampaign(req.Name, req.TargetBinary)
	if err != nil {
		a.logger.WithError(err).Error("failed to create campaign")
		a.writeError(w, http.StatusBadRequest, "CAMPAIGN_CREATION_FAILED", "Failed to create campaign", err)
		return
	}

	// Set optional fields
	if req.Description != nil {
		campaign.Description = *req.Description
	}

	if req.MaxJobs != nil {
		campaign.MaxJobs = *req.MaxJobs
	}

	if req.MaxDurationSeconds != nil {
		campaign.MaxDurationSeconds = *req.MaxDurationSeconds
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

	// Save campaign using service
	if err := a.service.CreateCampaign(ctx, campaign); err != nil {
		a.logger.WithError(err).Error("failed to save campaign")
		a.writeError(w, http.StatusInternalServerError, "SAVE_FAILED", "Failed to save campaign", err)
		return
	}

	apiCampaign := a.convertCampaignToAPI(campaign)

	// Publish SSE event
	campaignUUID := uuid.MustParse(campaign.ID)
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

	campaign, err := a.repository.FindByID(ctx, campaignId.String())
	if err != nil {
		a.logger.WithError(err).WithField("campaign_id", campaignId).Error("failed to get campaign")
		a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
		return
	}

	apiCampaign := a.convertCampaignToAPI(campaign)
	a.writeJSONResponse(w, http.StatusOK, apiCampaign)
}

// UpdateCampaign updates an existing campaign
func (a *CampaignAdapter) UpdateCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	ctx := r.Context()

	var req generated.CampaignUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Get existing campaign
	campaign, err := a.repository.FindByID(ctx, campaignId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
		return
	}

	// Check if campaign can be updated
	if campaign.Status == campaignTypes.StateRunning {
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
		campaign.MaxDurationSeconds = *req.MaxDurationSeconds
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

	// Save changes
	if err := a.repository.Update(ctx, campaign); err != nil {
		a.logger.WithError(err).Error("failed to update campaign")
		a.writeError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update campaign", err)
		return
	}

	apiCampaign := a.convertCampaignToAPI(campaign)

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

	// Get campaign to check its status
	campaign, err := a.repository.FindByID(ctx, campaignId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
		return
	}

	// Check if campaign can be deleted
	if campaign.Status == campaignTypes.StateRunning {
		a.writeError(w, http.StatusConflict, "INVALID_STATUS", "Cannot delete running campaign", nil)
		return
	}

	// Delete campaign using service
	if err := a.service.DeleteCampaign(ctx, campaign.ID); err != nil {
		a.logger.WithError(err).Error("failed to delete campaign")
		a.writeError(w, http.StatusInternalServerError, "DELETE_FAILED", "Failed to delete campaign", err)
		return
	}

	// Publish SSE event
	campaignUUID := uuid.MustParse(campaign.ID)
	event := sse.NewCampaignEvent("campaign.deleted", campaignUUID, map[string]any{
		"campaign_id": campaign.ID,
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

	// Get campaign
	campaign, err := a.repository.FindByID(ctx, campaignId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
		return
	}

	// Check if campaign can be started
	if campaign.Status == campaignTypes.StateRunning {
		a.writeError(w, http.StatusConflict, "ALREADY_RUNNING", "Campaign is already running", nil)
		return
	}

	// Start campaign using service
	if err := a.service.StartCampaign(ctx, campaign.ID); err != nil {
		a.logger.WithError(err).Error("failed to start campaign")
		a.writeError(w, http.StatusInternalServerError, "START_FAILED", "Failed to start campaign", err)
		return
	}

	// Get updated campaign
	updatedCampaign, err := a.repository.FindByID(ctx, campaignId.String())
	if err != nil {
		a.logger.WithError(err).Warn("failed to get updated campaign after start")
		updatedCampaign = campaign
	}

	apiCampaign := a.convertCampaignToAPI(updatedCampaign)

	// Publish SSE event
	campaignUUID := uuid.MustParse(campaign.ID)
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

	// Get campaign
	campaign, err := a.repository.FindByID(ctx, campaignId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
		return
	}

	// Check if campaign is running
	if campaign.Status != campaignTypes.StateRunning {
		a.writeError(w, http.StatusConflict, "NOT_RUNNING", "Campaign is not running", nil)
		return
	}

	reason := "Manual stop"
	if req.Reason != nil {
		reason = *req.Reason
	}

	// Stop campaign using service
	if err := a.service.StopCampaign(ctx, campaign.ID, reason); err != nil {
		a.logger.WithError(err).Error("failed to stop campaign")
		a.writeError(w, http.StatusInternalServerError, "STOP_FAILED", "Failed to stop campaign", err)
		return
	}

	// Get updated campaign
	updatedCampaign, err := a.repository.FindByID(ctx, campaignId.String())
	if err != nil {
		a.logger.WithError(err).Warn("failed to get updated campaign after stop")
		updatedCampaign = campaign
	}

	apiCampaign := a.convertCampaignToAPI(updatedCampaign)

	// Publish SSE event
	campaignUUID := uuid.MustParse(campaign.ID)
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

	// Verify campaign exists
	campaign, err := a.repository.FindByID(ctx, campaignId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Campaign not found", err)
		return
	}

	// Get campaign statistics
	stats := a.getCampaignStats(ctx, campaign)

	a.writeJSONResponse(w, http.StatusOK, stats)
}

// Helper methods

func (a *CampaignAdapter) convertCampaignToAPI(campaign *campaignTypes.Campaign) generated.Campaign {
	apiCampaign := generated.Campaign{
		Id:           uuid.MustParse(campaign.ID),
		Name:         campaign.Name,
		TargetBinary: campaign.TargetBinary,
		Status:       campaignStatusToGenerated(campaign.Status),
		CreatedAt:    campaign.CreatedAt,
	}

	if campaign.Description != "" {
		apiCampaign.Description = &campaign.Description
	}

	if campaign.StartedAt != nil {
		apiCampaign.StartedAt = campaign.StartedAt
	}

	if campaign.CompletedAt != nil {
		apiCampaign.CompletedAt = campaign.CompletedAt
	}

	if campaign.MaxJobs > 0 {
		apiCampaign.MaxJobs = &campaign.MaxJobs
	}

	if campaign.MaxDurationSeconds > 0 {
		apiCampaign.MaxDurationSeconds = &campaign.MaxDurationSeconds
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
		return generated.Draft
	case campaignTypes.StateRunning:
		return generated.Active
	case campaignTypes.StatePaused:
		return generated.Paused
	case campaignTypes.StateCompleted:
		return generated.Completed
	case campaignTypes.StateCanceled:
		return generated.Cancelled
	case campaignTypes.StateArchived:
		return generated.Archived
	default:
		return generated.Draft
	}
}

func generatedToCampaignStatus(status generated.CampaignStatus) campaignTypes.State {
	switch status {
	case generated.Draft:
		return campaignTypes.StateDraft
	case generated.Active:
		return campaignTypes.StateRunning
	case generated.Paused:
		return campaignTypes.StatePaused
	case generated.Completed:
		return campaignTypes.StateCompleted
	case generated.Cancelled:
		return campaignTypes.StateCanceled
	case generated.Archived:
		return campaignTypes.StateArchived
	default:
		return campaignTypes.StateDraft
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
