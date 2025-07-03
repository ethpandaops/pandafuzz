package master

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// Corpus API request/response structures

// CorpusSyncRequest represents a corpus synchronization request
type CorpusSyncRequest struct {
	BotID string `json:"bot_id" validate:"required"`
}

// CorpusSyncResponse represents a corpus synchronization response
type CorpusSyncResponse struct {
	CampaignID string               `json:"campaign_id"`
	BotID      string               `json:"bot_id"`
	Files      []*common.CorpusFile `json:"files"`
	FileCount  int                  `json:"file_count"`
	TotalSize  int64                `json:"total_size"`
	Timestamp  time.Time            `json:"timestamp"`
}

// CorpusShareRequest represents a corpus sharing request
type CorpusShareRequest struct {
	ToCampaignID    string `json:"to_campaign_id" validate:"required"`
	OnlyNewCoverage bool   `json:"only_new_coverage,omitempty"`
}

// CorpusFileResponse represents a corpus file with download URL
type CorpusFileResponse struct {
	*common.CorpusFile
	DownloadURL string `json:"download_url"`
}

// handleGetCorpusEvolution retrieves corpus evolution history
func (s *Server) handleGetCorpusEvolution(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Verify campaign exists
	campaign, err := s.services.Campaign.Get(r.Context(), campaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			s.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign", err)
		}
		return
	}

	// Get evolution history
	evolution, err := s.services.Corpus.GetEvolution(r.Context(), campaignID)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get corpus evolution", err)
		return
	}

	// Ensure we have a valid slice
	if evolution == nil {
		evolution = make([]*common.CorpusEvolution, 0)
	}

	response := map[string]any{
		"campaign_id":   campaignID,
		"campaign_name": campaign.Name,
		"evolution":     evolution,
		"data_points":   len(evolution),
	}

	s.writeJSONResponse(w, response)
}

// handleSyncCorpus handles corpus synchronization for bots
func (s *Server) handleSyncCorpus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	var req CorpusSyncRequest
	if err := s.decodeJSONBody(w, r, &req); err != nil {
		// Error response already written by decodeJSONBody
		return
	}

	// Validate bot exists
	_, err := s.services.Bot.GetBot(r.Context(), req.BotID)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "Bot not found", err)
		return
	}

	// Sync corpus files
	files, err := s.services.Corpus.SyncCorpus(r.Context(), campaignID, req.BotID)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to sync corpus", err)
		return
	}

	// Calculate total size
	var totalSize int64
	for _, file := range files {
		totalSize += file.Size
	}

	s.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"bot_id":      req.BotID,
		"file_count":  len(files),
		"total_size":  totalSize,
	}).Info("Corpus synced to bot")

	response := CorpusSyncResponse{
		CampaignID: campaignID,
		BotID:      req.BotID,
		Files:      files,
		FileCount:  len(files),
		TotalSize:  totalSize,
		Timestamp:  time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// handleShareCorpus handles corpus sharing between campaigns
func (s *Server) handleShareCorpus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fromCampaignID := vars["id"]

	if fromCampaignID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	var req CorpusShareRequest
	if err := s.decodeJSONBody(w, r, &req); err != nil {
		// Error response already written by decodeJSONBody
		return
	}

	// Validate campaigns
	fromCampaign, err := s.services.Campaign.Get(r.Context(), fromCampaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			s.writeErrorResponse(w, http.StatusNotFound, "Source campaign not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get source campaign", err)
		}
		return
	}

	toCampaign, err := s.services.Campaign.Get(r.Context(), req.ToCampaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			s.writeErrorResponse(w, http.StatusNotFound, "Target campaign not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get target campaign", err)
		}
		return
	}

	// Share corpus
	if err := s.services.Corpus.ShareCorpus(r.Context(), fromCampaignID, req.ToCampaignID); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to share corpus", err)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"from_campaign": fromCampaignID,
		"to_campaign":   req.ToCampaignID,
	}).Info("Corpus shared between campaigns")

	response := map[string]any{
		"status":        "shared",
		"from_campaign": fromCampaign.Name,
		"to_campaign":   toCampaign.Name,
		"timestamp":     time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// handleListCorpusFiles lists corpus files for a campaign
func (s *Server) handleListCorpusFiles(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Get query parameters
	limit := 100
	offset := 0
	filterSeed := r.URL.Query().Get("is_seed")
	filterGeneration := r.URL.Query().Get("generation")

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Get all corpus files (in real implementation, would support pagination in storage layer)
	// TODO: Implement GetCorpusFiles in state
	// files, err := s.state.GetCorpusFiles(r.Context(), campaignID)
	var files []*common.CorpusFile
	var err error
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get corpus files", err)
		return
	}

	// Apply filters
	var filtered []*common.CorpusFile
	for _, file := range files {
		// Filter by seed status
		if filterSeed != "" {
			isSeed := filterSeed == "true"
			if file.IsSeed != isSeed {
				continue
			}
		}

		// Filter by generation
		if filterGeneration != "" {
			if gen, err := strconv.Atoi(filterGeneration); err == nil {
				if file.Generation != gen {
					continue
				}
			}
		}

		filtered = append(filtered, file)
	}

	// Apply pagination
	totalFiles := len(filtered)
	if offset >= len(filtered) {
		filtered = []*common.CorpusFile{}
	} else if offset+limit > len(filtered) {
		filtered = filtered[offset:]
	} else {
		filtered = filtered[offset : offset+limit]
	}

	// Add download URLs
	fileResponses := make([]*CorpusFileResponse, len(filtered))
	for i, file := range filtered {
		fileResponses[i] = &CorpusFileResponse{
			CorpusFile:  file,
			DownloadURL: fmt.Sprintf("/api/v1/campaigns/%s/corpus/files/%s", campaignID, file.Hash),
		}
	}

	response := map[string]any{
		"campaign_id": campaignID,
		"files":       fileResponses,
		"count":       len(fileResponses),
		"total":       totalFiles,
		"limit":       limit,
		"offset":      offset,
	}

	s.writeJSONResponse(w, response)
}

// handleGetCrashGroups retrieves deduplicated crash groups for a campaign
func (s *Server) handleGetCrashGroups(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Get query parameters for filtering
	severityFilter := r.URL.Query().Get("severity")
	minCount := 1
	if minCountStr := r.URL.Query().Get("min_count"); minCountStr != "" {
		if mc, err := strconv.Atoi(minCountStr); err == nil && mc > 0 {
			minCount = mc
		}
	}

	// Get crash groups
	// TODO: Implement deduplication service
	// groups, err := s.services.Deduplication.GetCrashGroups(r.Context(), campaignID)
	var groups []*common.CrashGroup
	// err := error(nil)
	// if err != nil {
	//	s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get crash groups", err)
	//	return
	// }

	// Apply filters
	var filtered []*common.CrashGroup
	for _, group := range groups {
		// Filter by severity
		if severityFilter != "" && group.Severity != severityFilter {
			continue
		}

		// Filter by minimum count
		if group.Count < minCount {
			continue
		}

		filtered = append(filtered, group)
	}

	// Calculate statistics
	uniqueCrashes := len(filtered)
	totalCrashes := 0
	severityCounts := make(map[string]int)

	for _, group := range filtered {
		totalCrashes += group.Count
		severityCounts[group.Severity]++
	}

	response := map[string]any{
		"campaign_id":    campaignID,
		"crash_groups":   filtered,
		"unique_crashes": uniqueCrashes,
		"total_crashes":  totalCrashes,
		"severities":     severityCounts,
		"filters": map[string]any{
			"severity":  severityFilter,
			"min_count": minCount,
		},
	}

	s.writeJSONResponse(w, response)
}

// handleGetStackTrace retrieves detailed stack trace for a crash
func (s *Server) handleGetStackTrace(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	crashID := vars["crash_id"]

	if crashID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Crash ID is required", nil)
		return
	}

	// Get stack trace
	// TODO: Implement deduplication service
	// stackTrace, err := s.services.Deduplication.GetStackTrace(r.Context(), crashID)
	var stackTrace *common.StackTrace
	// err := error(nil)
	// if err != nil {
	//	s.writeErrorResponse(w, http.StatusNotFound, "Stack trace not found", err)
	//	return
	// }

	// Get crash details for context
	crash, err := s.state.GetCrash(r.Context(), crashID)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to get crash details")
		// Continue without crash details
	}

	response := map[string]any{
		"crash_id":    crashID,
		"stack_trace": stackTrace,
	}

	if crash != nil {
		response["crash_details"] = map[string]any{
			"job_id":    crash.JobID,
			"bot_id":    crash.BotID,
			"timestamp": crash.Timestamp,
			"type":      crash.Type,
			"signal":    crash.Signal,
		}
	}

	s.writeJSONResponse(w, response)
}

// handleCorpusImport handles bulk corpus import from directory
func (s *Server) handleCorpusImport(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Get import directory from form
	if err := r.ParseForm(); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Failed to parse form", err)
		return
	}

	importDir := r.FormValue("directory")
	if importDir == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Import directory is required", nil)
		return
	}

	// Validate import directory
	if !strings.HasPrefix(importDir, s.config.Storage.BasePath) {
		s.writeErrorResponse(w, http.StatusForbidden, "Import directory must be within storage path", nil)
		return
	}

	// Verify campaign exists
	campaign, err := s.services.Campaign.Get(r.Context(), campaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			s.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign", err)
		}
		return
	}

	// Import seed corpus
	// TODO: Implement ImportSeedCorpus method
	// if err := s.services.Corpus.(*corpusService).ImportSeedCorpus(r.Context(), campaignID, importDir); err != nil {
	if false {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to import corpus", err)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"import_dir":  importDir,
	}).Info("Corpus imported from directory")

	response := map[string]any{
		"status":      "imported",
		"campaign_id": campaignID,
		"campaign":    campaign.Name,
		"directory":   importDir,
		"timestamp":   time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// handleCorpusCleanup handles orphaned file cleanup
func (s *Server) handleCorpusCleanup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Verify campaign exists
	_, err := s.services.Campaign.Get(r.Context(), campaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			s.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign", err)
		}
		return
	}

	// Cleanup orphaned files
	// TODO: Implement CleanupOrphanedFiles method
	// if err := s.services.Corpus.(*corpusService).CleanupOrphanedFiles(r.Context(), campaignID); err != nil {
	if false {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to cleanup corpus", err)
		return
	}

	s.logger.WithField("campaign_id", campaignID).Info("Corpus cleanup completed")

	response := map[string]any{
		"status":      "cleaned",
		"campaign_id": campaignID,
		"timestamp":   time.Now(),
	}

	s.writeJSONResponse(w, response)
}
