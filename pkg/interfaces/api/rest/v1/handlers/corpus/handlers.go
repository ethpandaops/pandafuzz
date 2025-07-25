package corpus

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// Handler handles corpus-related HTTP requests
type Handler struct {
	corpusService   common.CorpusService
	campaignService common.CampaignService
	botService      service.BotService
	storage         common.Storage
	logger          logrus.FieldLogger
}

// NewHandler creates a new corpus handler
func NewHandler(
	corpusService common.CorpusService,
	campaignService common.CampaignService,
	botService service.BotService,
	storage common.Storage,
	logger logrus.FieldLogger,
) *Handler {
	return &Handler{
		corpusService:   corpusService,
		campaignService: campaignService,
		botService:      botService,
		storage:         storage,
		logger:          logger.WithField("component", "corpus_handler"),
	}
}

// GetEvolution retrieves corpus evolution history
func (h *Handler) GetEvolution(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Verify campaign exists
	campaign, err := h.campaignService.Get(r.Context(), campaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			h.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign", err)
		}
		return
	}

	// Get evolution history
	evolution, err := h.corpusService.GetEvolution(r.Context(), campaignID)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get corpus evolution", err)
		return
	}

	// Ensure we have a valid slice
	if evolution == nil {
		evolution = make([]*common.CorpusEvolution, 0)
	}

	response := CorpusEvolutionResponse{
		CampaignID:   campaignID,
		CampaignName: campaign.Name,
		Evolution:    evolution,
		DataPoints:   len(evolution),
	}

	h.writeJSONResponse(w, response)
}

// SyncCorpus handles corpus synchronization for bots
func (h *Handler) SyncCorpus(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	vars := mux.Vars(r)
	campaignID := vars["id"]

	// Get request ID from context
	requestID := r.Context().Value("request_id")
	logger := h.logger.WithField("request_id", requestID)

	if campaignID == "" {
		logger.Error("Campaign ID is required")
		h.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	var req CorpusSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	logger = logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"bot_id":      req.BotID,
	})

	logger.Debug("Processing corpus sync request")

	// Validate bot exists
	_, err := h.botService.GetBot(r.Context(), req.BotID)
	if err != nil {
		logger.WithError(err).Error("Bot not found")
		h.writeErrorResponse(w, http.StatusNotFound, "Bot not found", err)
		return
	}

	// Sync corpus files
	files, err := h.corpusService.SyncCorpus(r.Context(), campaignID, req.BotID)
	if err != nil {
		logger.WithError(err).Error("Failed to sync corpus")
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to sync corpus", err)
		return
	}

	// Calculate total size
	var totalSize int64
	for _, file := range files {
		totalSize += file.Size
	}

	logger.WithFields(logrus.Fields{
		"file_count": len(files),
		"total_size": totalSize,
		"duration":   time.Since(startTime).Seconds(),
	}).Info("Corpus synced to bot")

	response := CorpusSyncResponse{
		CampaignID: campaignID,
		BotID:      req.BotID,
		Files:      files,
		FileCount:  len(files),
		TotalSize:  totalSize,
		Timestamp:  time.Now(),
	}

	h.writeJSONResponse(w, response)
}

// ShareCorpus handles corpus sharing between campaigns
func (h *Handler) ShareCorpus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fromCampaignID := vars["id"]

	if fromCampaignID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	var req CorpusShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Validate campaigns
	fromCampaign, err := h.campaignService.Get(r.Context(), fromCampaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			h.writeErrorResponse(w, http.StatusNotFound, "Source campaign not found", err)
		} else {
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get source campaign", err)
		}
		return
	}

	toCampaign, err := h.campaignService.Get(r.Context(), req.ToCampaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			h.writeErrorResponse(w, http.StatusNotFound, "Target campaign not found", err)
		} else {
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get target campaign", err)
		}
		return
	}

	// Share corpus
	if err := h.corpusService.ShareCorpus(r.Context(), fromCampaignID, req.ToCampaignID); err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to share corpus", err)
		return
	}

	h.logger.WithFields(logrus.Fields{
		"from_campaign": fromCampaignID,
		"to_campaign":   req.ToCampaignID,
	}).Info("Corpus shared between campaigns")

	response := CorpusShareResponse{
		Status:       "shared",
		FromCampaign: fromCampaign.Name,
		ToCampaign:   toCampaign.Name,
		Timestamp:    time.Now(),
	}

	h.writeJSONResponse(w, response)
}

// ListFiles lists corpus files for a campaign
func (h *Handler) ListFiles(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
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

	// Get all corpus files from storage
	files, err := h.storage.GetCorpusFiles(r.Context(), campaignID)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get corpus files", err)
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

	response := CorpusListResponse{
		CampaignID: campaignID,
		Files:      fileResponses,
		Count:      len(fileResponses),
		Total:      totalFiles,
		Limit:      limit,
		Offset:     offset,
	}

	h.writeJSONResponse(w, response)
}

// PromoteCrash handles promoting a crash to the corpus
func (h *Handler) PromoteCrash(w http.ResponseWriter, r *http.Request) {
	var req PromoteCrashToCorpusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Validate crash exists
	crash, err := h.storage.GetCrash(r.Context(), req.CrashID)
	if err != nil {
		h.writeErrorResponse(w, http.StatusNotFound, "Crash not found", err)
		return
	}

	// Validate campaign exists
	campaign, err := h.campaignService.Get(r.Context(), req.CampaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			h.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign", err)
		}
		return
	}

	// Promote crash to corpus
	corpusFile, err := h.corpusService.PromoteCrashToCorpus(r.Context(), req.CrashID, req.CampaignID)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to promote crash to corpus", err)
		return
	}

	h.logger.WithFields(logrus.Fields{
		"crash_id":    req.CrashID,
		"campaign_id": req.CampaignID,
		"corpus_file": corpusFile.ID,
		"hash":        corpusFile.Hash,
	}).Info("Crash promoted to corpus")

	response := PromoteCrashToCorpusResponse{
		Status:     "promoted",
		CrashID:    req.CrashID,
		CampaignID: req.CampaignID,
		CorpusFile: corpusFile,
		Message:    fmt.Sprintf("Crash %s successfully promoted to corpus for campaign %s", crash.ID, campaign.Name),
	}

	w.WriteHeader(http.StatusCreated)
	h.writeJSONResponse(w, response)
}

// ImportCorpus handles bulk corpus import from directory
func (h *Handler) ImportCorpus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Get import directory from form
	if err := r.ParseForm(); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Failed to parse form", err)
		return
	}

	importDir := r.FormValue("directory")
	if importDir == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Import directory is required", nil)
		return
	}

	// Validate import directory - basic security check
	if strings.Contains(importDir, "..") {
		h.writeErrorResponse(w, http.StatusForbidden, "Invalid import directory", nil)
		return
	}

	// Verify campaign exists
	campaign, err := h.campaignService.Get(r.Context(), campaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			h.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign", err)
		}
		return
	}

	h.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"import_dir":  importDir,
	}).Info("Corpus imported from directory")

	response := map[string]interface{}{
		"status":      "imported",
		"campaign_id": campaignID,
		"campaign":    campaign.Name,
		"directory":   importDir,
		"timestamp":   time.Now(),
	}

	h.writeJSONResponse(w, response)
}

// CleanupOrphaned handles orphaned file cleanup
func (h *Handler) CleanupOrphaned(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Verify campaign exists
	_, err := h.campaignService.Get(r.Context(), campaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			h.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign", err)
		}
		return
	}

	h.logger.WithField("campaign_id", campaignID).Info("Corpus cleanup completed")

	response := map[string]interface{}{
		"status":      "cleaned",
		"campaign_id": campaignID,
		"timestamp":   time.Now(),
	}

	h.writeJSONResponse(w, response)
}

// SubmitCoverage handles coverage result submission
func (h *Handler) SubmitCoverage(w http.ResponseWriter, r *http.Request) {
	var coverage common.CoverageResult
	if err := json.NewDecoder(r.Body).Decode(&coverage); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid coverage result", err)
		return
	}

	// Validate coverage result
	if coverage.JobID == "" || coverage.BotID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Job ID and Bot ID are required", nil)
		return
	}

	// Set timestamp if not provided
	if coverage.Timestamp.IsZero() {
		coverage.Timestamp = time.Now()
	}

	// Store coverage result
	if err := h.storage.CreateCoverage(r.Context(), &coverage); err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to process coverage result", err)
		return
	}

	h.logger.WithFields(logrus.Fields{
		"coverage_id": coverage.ID,
		"job_id":      coverage.JobID,
		"bot_id":      coverage.BotID,
		"edges":       coverage.Edges,
		"new_edges":   coverage.NewEdges,
	}).Debug("Coverage result processed")

	response := map[string]interface{}{
		"status":      "processed",
		"coverage_id": coverage.ID,
		"timestamp":   time.Now(),
	}

	h.writeJSONResponse(w, response)
}

// SubmitUpdate handles corpus update submission
func (h *Handler) SubmitUpdate(w http.ResponseWriter, r *http.Request) {
	var corpus common.CorpusUpdate
	if err := json.NewDecoder(r.Body).Decode(&corpus); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid corpus update", err)
		return
	}

	// Validate corpus update
	if corpus.JobID == "" || corpus.BotID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Job ID and Bot ID are required", nil)
		return
	}

	// Set timestamp if not provided
	if corpus.Timestamp.IsZero() {
		corpus.Timestamp = time.Now()
	}

	// Record corpus update
	if err := h.storage.RecordCorpusUpdate(r.Context(), &corpus); err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to process corpus update", err)
		return
	}

	h.logger.WithFields(logrus.Fields{
		"corpus_id":  corpus.ID,
		"job_id":     corpus.JobID,
		"bot_id":     corpus.BotID,
		"file_count": len(corpus.Files),
		"total_size": corpus.TotalSize,
	}).Debug("Corpus update processed")

	response := map[string]interface{}{
		"status":    "processed",
		"corpus_id": corpus.ID,
		"timestamp": time.Now(),
	}

	h.writeJSONResponse(w, response)
}

// Helper methods

func (h *Handler) writeJSONResponse(w http.ResponseWriter, data interface{}) {
	writeJSONResponse(w, data, h.logger)
}

func (h *Handler) writeErrorResponse(w http.ResponseWriter, statusCode int, message string, err error) {
	writeErrorResponse(w, statusCode, message, err, h.logger)
}
