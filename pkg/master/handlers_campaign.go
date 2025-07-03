package master

import (
	"crypto/sha256"
	"encoding/hex"
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

// Campaign API request/response structures

// CampaignCreateRequest represents a campaign creation request
type CampaignCreateRequest struct {
	Name             string           `json:"name" validate:"required"`
	Description      string           `json:"description,omitempty"`
	TargetBinary     string           `json:"target_binary" validate:"required"`
	BinaryHash       string           `json:"binary_hash,omitempty"`
	MaxJobs          int              `json:"max_jobs,omitempty"`
	MaxDuration      time.Duration    `json:"max_duration,omitempty"`
	AutoRestart      bool             `json:"auto_restart,omitempty"`
	SharedCorpus     bool             `json:"shared_corpus,omitempty"`
	JobTemplate      common.JobConfig `json:"job_template" validate:"required"`
	Tags             []string         `json:"tags,omitempty"`
	StartAfterCreate bool             `json:"start_after_create,omitempty"`
}

// CampaignUpdateRequest represents a campaign update request
type CampaignUpdateRequest struct {
	Name         *string        `json:"name,omitempty"`
	Description  *string        `json:"description,omitempty"`
	Status       *string        `json:"status,omitempty"`
	MaxJobs      *int           `json:"max_jobs,omitempty"`
	MaxDuration  *time.Duration `json:"max_duration,omitempty"`
	AutoRestart  *bool          `json:"auto_restart,omitempty"`
	SharedCorpus *bool          `json:"shared_corpus,omitempty"`
	Tags         []string       `json:"tags,omitempty"`
}

// CampaignStatsResponse represents campaign statistics
type CampaignStatsResponse struct {
	CampaignID      string                    `json:"campaign_id"`
	Name            string                    `json:"name"`
	Status          common.CampaignStatus     `json:"status"`
	CreatedAt       time.Time                 `json:"created_at"`
	UpdatedAt       time.Time                 `json:"updated_at"`
	CompletedAt     *time.Time                `json:"completed_at,omitempty"`
	Statistics      *common.CampaignStats     `json:"statistics"`
	CrashGroups     []*common.CrashGroup      `json:"crash_groups"`
	CorpusEvolution []*common.CorpusEvolution `json:"corpus_evolution"`
}

// handleCreateCampaign handles campaign creation
func (s *Server) handleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	var req CampaignCreateRequest
	if err := s.decodeJSONBody(w, r, &req); err != nil {
		// Error response already written by decodeJSONBody
		return
	}

	// Validate request
	if err := s.validateCampaignCreateRequest(&req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid request", err)
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
	if err := s.services.Campaign.Create(r.Context(), campaign); err != nil {
		s.logger.WithError(err).Error("Failed to create campaign")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to create campaign", err)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"campaign_id": campaign.ID,
		"name":        campaign.Name,
		"status":      campaign.Status,
	}).Info("Campaign created")

	w.WriteHeader(http.StatusCreated)
	s.writeJSONResponse(w, campaign)
}

// handleGetCampaign retrieves a specific campaign
func (s *Server) handleGetCampaign(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	campaign, err := s.services.Campaign.Get(r.Context(), campaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			s.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign", err)
		}
		return
	}

	s.writeJSONResponse(w, campaign)
}

// handleListCampaigns lists campaigns with filters
func (s *Server) handleListCampaigns(w http.ResponseWriter, r *http.Request) {
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
	campaigns, err := s.services.Campaign.List(r.Context(), filters)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to list campaigns", err)
		return
	}

	// Ensure we have a valid slice
	if campaigns == nil {
		campaigns = make([]*common.Campaign, 0)
	}

	response := map[string]any{
		"campaigns": campaigns,
		"count":     len(campaigns),
		"limit":     filters.Limit,
		"offset":    filters.Offset,
	}

	s.writeJSONResponse(w, response)
}

// handleUpdateCampaign updates a campaign
func (s *Server) handleUpdateCampaign(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	var req CampaignUpdateRequest
	if err := s.decodeJSONBody(w, r, &req); err != nil {
		// Error response already written by decodeJSONBody
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
	if err := s.services.Campaign.Update(r.Context(), campaignID, updates); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to update campaign", err)
		return
	}

	// Get updated campaign
	campaign, err := s.services.Campaign.Get(r.Context(), campaignID)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get updated campaign", err)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"updates":     req,
	}).Info("Campaign updated")

	s.writeJSONResponse(w, campaign)
}

// handleDeleteCampaign deletes a campaign
func (s *Server) handleDeleteCampaign(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	if err := s.services.Campaign.Delete(r.Context(), campaignID); err != nil {
		if err == common.ErrCampaignNotFound {
			s.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete campaign", err)
		}
		return
	}

	s.logger.WithField("campaign_id", campaignID).Info("Campaign deleted")

	w.WriteHeader(http.StatusNoContent)
}

// handleRestartCampaign restarts a completed campaign
func (s *Server) handleRestartCampaign(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	if err := s.services.Campaign.RestartCampaign(r.Context(), campaignID); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to restart campaign", err)
		return
	}

	// Get updated campaign
	campaign, err := s.services.Campaign.Get(r.Context(), campaignID)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get restarted campaign", err)
		return
	}

	s.logger.WithField("campaign_id", campaignID).Info("Campaign restarted")

	s.writeJSONResponse(w, campaign)
}

// handleGetCampaignStats retrieves comprehensive campaign statistics
func (s *Server) handleGetCampaignStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Get campaign
	campaign, err := s.services.Campaign.Get(r.Context(), campaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			s.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign", err)
		}
		return
	}

	// Get statistics
	stats, err := s.services.Campaign.GetStatistics(r.Context(), campaignID)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get campaign statistics")
		// Continue with nil stats
	}

	// Get crash groups
	// TODO: Implement deduplication service
	// crashGroups, err := s.services.Deduplication.GetCrashGroups(r.Context(), campaignID)
	var crashGroups []*common.CrashGroup
	if false { // Placeholder for when dedup service is implemented
		s.logger.WithError(err).Error("Failed to get crash groups")
		crashGroups = []*common.CrashGroup{}
	}

	// Get corpus evolution
	evolution, err := s.services.Corpus.GetEvolution(r.Context(), campaignID)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get corpus evolution")
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

	s.writeJSONResponse(w, response)
}

// handleUploadCampaignBinary handles binary upload for a campaign
func (s *Server) handleUploadCampaignBinary(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Parse multipart form (32MB max)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Failed to parse form", err)
		return
	}

	// Get the file
	file, header, err := r.FormFile("binary")
	if err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Missing binary file", err)
		return
	}
	defer file.Close()

	// Validate file
	if header.Size > 100<<20 { // 100MB max
		s.writeErrorResponse(w, http.StatusRequestEntityTooLarge, "Binary file too large (max 100MB)", nil)
		return
	}

	// Read file content
	content, err := io.ReadAll(file)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to read file", err)
		return
	}

	// Calculate hash
	// Calculate hash using SHA256
	hasher := sha256.New()
	hasher.Write(content)
	hash := hex.EncodeToString(hasher.Sum(nil))

	// Store binary file
	// binaryPath := filepath.Join(s.config.Storage.BasePath, "binaries", campaignID, hash)
	// TODO: Implement file storage
	// if err := s.fileStorage.Store(r.Context(), binaryPath, content); err != nil {
	if false {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to store binary", err)
		return
	}

	// Update campaign with binary hash
	// TODO: Add BinaryHash to CampaignUpdates type
	updates := common.CampaignUpdates{}
	if err := s.services.Campaign.Update(r.Context(), campaignID, updates); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to update campaign", err)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"filename":    header.Filename,
		"size":        header.Size,
		"hash":        hash,
	}).Info("Binary uploaded for campaign")

	response := map[string]any{
		"campaign_id": campaignID,
		"filename":    header.Filename,
		"size":        header.Size,
		"hash":        hash,
		"timestamp":   time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// handleUploadCampaignCorpus handles corpus upload for a campaign
func (s *Server) handleUploadCampaignCorpus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Parse multipart form (100MB max for corpus)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Failed to parse form", err)
		return
	}

	// Get campaign to verify it exists
	campaign, err := s.services.Campaign.Get(r.Context(), campaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			s.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign", err)
		}
		return
	}

	// Process all uploaded files
	files := r.MultipartForm.File["corpus"]
	if len(files) == 0 {
		s.writeErrorResponse(w, http.StatusBadRequest, "No corpus files provided", nil)
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
		// Calculate hash using SHA256
		hasher := sha256.New()
		hasher.Write(content)
		hash := hex.EncodeToString(hasher.Sum(nil))

		// Create corpus file entry
		corpusFile := &common.CorpusFile{
			CampaignID: campaignID,
			Filename:   fileHeader.Filename,
			Hash:       hash,
			Size:       fileHeader.Size,
			IsSeed:     true,
		}

		// Add to campaign corpus
		if err := s.services.Corpus.AddFile(r.Context(), corpusFile); err != nil {
			if err == common.ErrDuplicateCorpusFile {
				duplicateCount++
			} else {
				errors = append(errors, fmt.Sprintf("%s: %v", fileHeader.Filename, err))
			}
			continue
		}

		uploadedCount++
	}

	s.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"uploaded":    uploadedCount,
		"duplicates":  duplicateCount,
		"errors":      len(errors),
	}).Info("Corpus files uploaded")

	response := map[string]any{
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

	s.writeJSONResponse(w, response)
}

// validateCampaignCreateRequest validates campaign creation request
func (s *Server) validateCampaignCreateRequest(req *CampaignCreateRequest) error {
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
