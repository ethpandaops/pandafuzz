package campaign

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// Handler handles campaign-related HTTP requests
type Handler struct {
	campaignService common.CampaignService
	corpusService   common.CorpusService
	logger          logrus.FieldLogger
}

// NewHandler creates a new campaign handler
func NewHandler(
	campaignService common.CampaignService,
	corpusService common.CorpusService,
	logger logrus.FieldLogger,
) *Handler {
	return &Handler{
		campaignService: campaignService,
		corpusService:   corpusService,
		logger:          logger.WithField("component", "campaign_handler"),
	}
}

// Create handles campaign creation
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CampaignCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Validate request
	if err := h.validateCreateRequest(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid request", err)
		return
	}

	// Create campaign object
	campaign := &common.Campaign{
		Name:         req.Name,
		Description:  req.Description,
		TargetBinary: req.TargetBinary,
		BinaryHash:   req.BinaryHash,
		MaxJobs:      req.MaxJobs,
		MaxDuration:  req.MaxDuration,
		AutoRestart:  req.AutoRestart,
		SharedCorpus: req.SharedCorpus,
		JobTemplate:  req.JobTemplate,
		Tags:         req.Tags,
	}

	// Set default values
	if campaign.MaxJobs <= 0 {
		campaign.MaxJobs = 10
	}
	if req.StartAfterCreate {
		campaign.Status = common.CampaignStatusRunning
	} else {
		campaign.Status = common.CampaignStatusPending
	}

	// Create campaign using service
	if err := h.campaignService.Create(r.Context(), campaign); err != nil {
		h.logger.WithError(err).Error("Failed to create campaign")
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to create campaign", err)
		return
	}

	h.logger.WithFields(logrus.Fields{
		"campaign_id": campaign.ID,
		"name":        campaign.Name,
		"status":      campaign.Status,
	}).Info("Campaign created")

	w.WriteHeader(http.StatusCreated)
	h.writeJSONResponse(w, campaign)
}

// Get retrieves a specific campaign
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	campaign, err := h.campaignService.Get(r.Context(), campaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			h.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign", err)
		}
		return
	}

	h.writeJSONResponse(w, campaign)
}

// List lists campaigns with filters
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	filters := common.CampaignFilters{
		Limit:  50,
		Offset: 0,
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit <= 100 {
			filters.Limit = limit
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			filters.Offset = offset
		}
	}

	if status := r.URL.Query().Get("status"); status != "" {
		filters.Status = status
	}

	if binaryHash := r.URL.Query().Get("binary_hash"); binaryHash != "" {
		filters.BinaryHash = binaryHash
	}

	if tags := r.URL.Query().Get("tags"); tags != "" {
		filters.Tags = strings.Split(tags, ",")
	}

	// Get campaigns
	campaigns, err := h.campaignService.List(r.Context(), filters)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to list campaigns", err)
		return
	}

	// Ensure we have a valid slice
	if campaigns == nil {
		campaigns = make([]*common.Campaign, 0)
	}

	response := CampaignListResponse{
		Campaigns: campaigns,
		Count:     len(campaigns),
		Limit:     filters.Limit,
		Offset:    filters.Offset,
	}

	h.writeJSONResponse(w, response)
}

// Update updates a campaign
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	var req CampaignUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Convert to service updates structure
	updates := common.CampaignUpdates{
		Name:         req.Name,
		Description:  req.Description,
		MaxJobs:      req.MaxJobs,
		MaxDuration:  req.MaxDuration,
		AutoRestart:  req.AutoRestart,
		SharedCorpus: req.SharedCorpus,
		Tags:         req.Tags,
	}

	// Handle status update separately
	if req.Status != nil {
		status := common.CampaignStatus(*req.Status)
		updates.Status = &status
	}

	// Update campaign
	if err := h.campaignService.Update(r.Context(), campaignID, updates); err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to update campaign", err)
		return
	}

	// Get updated campaign
	campaign, err := h.campaignService.Get(r.Context(), campaignID)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get updated campaign", err)
		return
	}

	h.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"updates":     req,
	}).Info("Campaign updated")

	h.writeJSONResponse(w, campaign)
}

// Delete deletes a campaign
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	if err := h.campaignService.Delete(r.Context(), campaignID); err != nil {
		if err == common.ErrCampaignNotFound {
			h.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete campaign", err)
		}
		return
	}

	h.logger.WithField("campaign_id", campaignID).Info("Campaign deleted")

	w.WriteHeader(http.StatusNoContent)
}

// Restart restarts a completed campaign
func (h *Handler) Restart(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	if err := h.campaignService.RestartCampaign(r.Context(), campaignID); err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to restart campaign", err)
		return
	}

	// Get updated campaign
	campaign, err := h.campaignService.Get(r.Context(), campaignID)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get restarted campaign", err)
		return
	}

	h.logger.WithField("campaign_id", campaignID).Info("Campaign restarted")

	h.writeJSONResponse(w, campaign)
}

// GetStats retrieves comprehensive campaign statistics
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Get campaign
	campaign, err := h.campaignService.Get(r.Context(), campaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			h.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign", err)
		}
		return
	}

	// Get statistics
	stats, err := h.campaignService.GetStatistics(r.Context(), campaignID)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get campaign statistics")
		// Continue with nil stats
	}

	// Get crash groups - placeholder for when dedup service is implemented
	var crashGroups []*common.CrashGroup

	// Get corpus evolution
	evolution, err := h.corpusService.GetEvolution(r.Context(), campaignID)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get corpus evolution")
		evolution = []*common.CorpusEvolution{}
	}

	response := CampaignStatsResponse{
		CampaignID:      campaign.ID,
		Name:            campaign.Name,
		Status:          campaign.Status,
		CreatedAt:       campaign.CreatedAt,
		UpdatedAt:       campaign.UpdatedAt,
		CompletedAt:     campaign.CompletedAt,
		Statistics:      stats,
		CrashGroups:     crashGroups,
		CorpusEvolution: evolution,
	}

	h.writeJSONResponse(w, response)
}

// UploadBinary handles binary upload for a campaign
func (h *Handler) UploadBinary(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Parse multipart form (32MB max)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Failed to parse form", err)
		return
	}

	// Get the file
	file, header, err := r.FormFile("binary")
	if err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Missing binary file", err)
		return
	}
	defer file.Close()

	// Validate file
	if header.Size > 100<<20 { // 100MB max
		h.writeErrorResponse(w, http.StatusRequestEntityTooLarge, "Binary file too large (max 100MB)", nil)
		return
	}

	// Read file content
	content, err := io.ReadAll(file)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to read file", err)
		return
	}

	// Calculate hash
	hash := calculateSHA256(content)

	// Update campaign with binary hash
	updates := common.CampaignUpdates{}
	if err := h.campaignService.Update(r.Context(), campaignID, updates); err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to update campaign", err)
		return
	}

	h.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"filename":    header.Filename,
		"size":        header.Size,
		"hash":        hash,
	}).Info("Binary uploaded for campaign")

	response := map[string]interface{}{
		"campaign_id": campaignID,
		"filename":    header.Filename,
		"size":        header.Size,
		"hash":        hash,
		"timestamp":   time.Now(),
	}

	h.writeJSONResponse(w, response)
}

// UploadCorpus handles corpus upload for a campaign
func (h *Handler) UploadCorpus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Parse multipart form (100MB max for corpus)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Failed to parse form", err)
		return
	}

	// Get campaign to verify it exists
	campaign, err := h.campaignService.Get(r.Context(), campaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			h.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign", err)
		}
		return
	}

	// Process all uploaded files
	files := r.MultipartForm.File["corpus"]
	if len(files) == 0 {
		h.writeErrorResponse(w, http.StatusBadRequest, "No corpus files provided", nil)
		return
	}

	uploadedCount := 0
	duplicateCount := 0
	errors := []string{}

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: failed to open", fileHeader.Filename))
			continue
		}

		// Read file content
		content, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: failed to read", fileHeader.Filename))
			continue
		}

		// Calculate hash
		hash := calculateSHA256(content)

		// Create corpus file entry
		corpusFile := &common.CorpusFile{
			CampaignID: campaignID,
			Filename:   fileHeader.Filename,
			Hash:       hash,
			Size:       fileHeader.Size,
			IsSeed:     true,
		}

		// Add to campaign corpus
		if err := h.corpusService.AddFile(r.Context(), corpusFile); err != nil {
			if err == common.ErrDuplicateCorpusFile {
				duplicateCount++
			} else {
				errors = append(errors, fmt.Sprintf("%s: %v", fileHeader.Filename, err))
			}
			continue
		}

		uploadedCount++
	}

	h.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"uploaded":    uploadedCount,
		"duplicates":  duplicateCount,
		"errors":      len(errors),
	}).Info("Corpus files uploaded")

	response := map[string]interface{}{
		"campaign_id": campaignID,
		"campaign":    campaign.Name,
		"uploaded":    uploadedCount,
		"duplicates":  duplicateCount,
		"total_files": len(files),
		"timestamp":   time.Now(),
	}

	if len(errors) > 0 {
		response["errors"] = errors
	}

	h.writeJSONResponse(w, response)
}

// Helper methods

func (h *Handler) validateCreateRequest(req *CampaignCreateRequest) error {
	// Validate name
	if req.Name == "" {
		return fmt.Errorf("campaign name is required")
	}
	if len(req.Name) > 100 {
		return fmt.Errorf("campaign name too long (max 100 characters)")
	}

	// Validate target binary
	if req.TargetBinary == "" {
		return fmt.Errorf("target binary is required")
	}
	if len(req.TargetBinary) > 500 {
		return fmt.Errorf("target binary path too long (max 500 characters)")
	}

	// Validate description
	if len(req.Description) > 1000 {
		return fmt.Errorf("description too long (max 1000 characters)")
	}

	// Validate max jobs
	if req.MaxJobs < 0 {
		return fmt.Errorf("max jobs cannot be negative")
	}
	if req.MaxJobs > 100 {
		return fmt.Errorf("max jobs too high (max 100)")
	}

	// Validate max duration
	if req.MaxDuration < 0 {
		return fmt.Errorf("max duration cannot be negative")
	}
	if req.MaxDuration > 7*24*time.Hour {
		return fmt.Errorf("max duration too long (max 7 days)")
	}

	// Validate tags
	if len(req.Tags) > 10 {
		return fmt.Errorf("too many tags (max 10)")
	}
	for _, tag := range req.Tags {
		if len(tag) > 50 {
			return fmt.Errorf("tag too long (max 50 characters)")
		}
		if !isValidIdentifier(tag) {
			return fmt.Errorf("invalid tag format: %s", tag)
		}
	}

	// Validate job template
	if req.JobTemplate.Timeout < 0 {
		return fmt.Errorf("job timeout cannot be negative")
	}
	if req.JobTemplate.Timeout == 0 {
		req.JobTemplate.Timeout = 1 * time.Hour // Default 1 hour
	}
	if req.JobTemplate.Timeout > 24*time.Hour {
		return fmt.Errorf("job timeout too long (max 24 hours)")
	}
	if req.JobTemplate.MemoryLimit < 0 {
		return fmt.Errorf("memory limit cannot be negative")
	}
	if req.JobTemplate.MemoryLimit > 16*1024 { // 16GB max
		return fmt.Errorf("memory limit too high (max 16GB)")
	}

	return nil
}
