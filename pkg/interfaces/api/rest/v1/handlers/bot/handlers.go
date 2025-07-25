package bot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// Handler handles bot-related HTTP requests
type Handler struct {
	botService service.BotService
	jobService service.JobService
	logger     logrus.FieldLogger
}

// NewHandler creates a new bot handler
func NewHandler(
	botService service.BotService,
	jobService service.JobService,
	logger logrus.FieldLogger,
) *Handler {
	return &Handler{
		botService: botService,
		jobService: jobService,
		logger:     logger.WithField("component", "bot_handler"),
	}
}

// Register handles bot registration
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req BotRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Validate request
	if err := h.validateRegisterRequest(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid request", err)
		return
	}

	// Use service layer
	bot, err := h.botService.RegisterBot(r.Context(), req.Hostname, req.Name, req.Capabilities, req.APIEndpoint)
	if err != nil {
		h.logger.WithError(err).Error("Failed to register bot")
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to register bot", err)
		return
	}

	// Response
	response := BotRegisterResponse{
		BotID:     bot.ID,
		Status:    "registered",
		Timestamp: bot.RegisteredAt,
		Timeout:   bot.TimeoutAt,
	}

	w.WriteHeader(http.StatusCreated)
	h.writeJSONResponse(w, response)
}

// Get handles bot information retrieval
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	botID := vars["id"]

	bot, err := h.botService.GetBot(r.Context(), botID)
	if err != nil {
		if err == ErrBotNotFound {
			h.writeErrorResponse(w, http.StatusNotFound, "Bot not found", err)
		} else {
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get bot", err)
		}
		return
	}

	h.writeJSONResponse(w, bot)
}

// Delete handles bot deregistration
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	botID := vars["id"]

	if botID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Bot ID is required", nil)
		return
	}

	if err := h.botService.DeleteBot(r.Context(), botID); err != nil {
		if err == ErrBotNotFound {
			h.writeErrorResponse(w, http.StatusNotFound, "Bot not found", err)
		} else {
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete bot", err)
		}
		return
	}

	h.logger.WithField("bot_id", botID).Info("Bot deregistered successfully")

	w.WriteHeader(http.StatusNoContent)
}

// Heartbeat handles bot heartbeat
func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	botID := vars["id"]

	if botID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Bot ID is required", nil)
		return
	}

	var req BotHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Update heartbeat
	if err := h.botService.UpdateHeartbeat(r.Context(), botID, req.Status, req.CurrentJob); err != nil {
		if err == ErrBotNotFound {
			h.writeErrorResponse(w, http.StatusNotFound, "Bot not found", err)
		} else {
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to update heartbeat", err)
		}
		return
	}

	// Get updated bot info
	bot, err := h.botService.GetBot(r.Context(), botID)
	if err != nil {
		h.logger.WithError(err).Warn("Failed to get bot after heartbeat update")
		// Continue without bot details
	}

	h.logger.WithFields(logrus.Fields{
		"bot_id": botID,
		"status": req.Status,
	}).Debug("Bot heartbeat received")

	now := time.Now()
	response := BotHeartbeatResponse{
		Status:    "ok",
		Timestamp: now,
		Timeout:   now.Add(30 * time.Second), // Default timeout
	}

	if bot != nil {
		response.Timeout = bot.TimeoutAt
	}

	h.writeJSONResponse(w, response)
}

// GetJob handles job assignment to bot
func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	botID := vars["id"]

	if botID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Bot ID is required", nil)
		return
	}

	// Assign next available job
	job, err := h.jobService.AssignNextJob(r.Context(), botID)
	if err != nil {
		if err == service.ErrNoJobsAvailable {
			h.writeJSONResponse(w, map[string]interface{}{
				"status":  "no_jobs_available",
				"message": "No jobs available for assignment",
			})
			return
		}
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to assign job", err)
		return
	}

	h.logger.WithFields(logrus.Fields{
		"bot_id":   botID,
		"job_id":   job.ID,
		"job_name": job.Name,
		"fuzzer":   job.Fuzzer,
	}).Info("Job assigned to bot")

	h.writeJSONResponse(w, job)
}

// CompleteJob handles job completion
func (h *Handler) CompleteJob(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	botID := vars["id"]

	if botID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Bot ID is required", nil)
		return
	}

	var req JobCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Get bot's current job
	currentJob, err := h.botService.GetCurrentJob(r.Context(), botID)
	if err != nil {
		if err == ErrBotNotFound {
			h.writeErrorResponse(w, http.StatusNotFound, "Bot not found", err)
		} else {
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get bot's current job", err)
		}
		return
	}

	if currentJob == nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Bot has no active job", nil)
		return
	}

	// Complete job
	if err := h.jobService.CompleteJob(r.Context(), currentJob.ID, botID, req.Status == "completed"); err != nil {
		h.logger.WithError(err).WithFields(logrus.Fields{
			"bot_id": botID,
			"job_id": currentJob.ID,
		}).Error("Failed to complete job")

		// Send negative acknowledgment
		response := AcknowledgmentResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to update job state: %v", err),
		}
		h.writeJSONResponse(w, response)
		return
	}

	h.logger.WithFields(logrus.Fields{
		"bot_id": botID,
		"job_id": currentJob.ID,
		"status": req.Status,
		"error":  req.Error,
	}).Info("Job completed and acknowledged")

	// Send positive acknowledgment
	response := AcknowledgmentResponse{
		Success: true,
		Message: "Job completion successfully recorded",
	}

	h.writeJSONResponse(w, response)
}

// List handles listing all bots
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("Listing bots")

	// Parse status filter
	var statusFilter *common.BotStatus
	if statusParam := r.URL.Query().Get("status"); statusParam != "" {
		status := common.BotStatus(statusParam)
		statusFilter = &status
	}

	bots, err := h.botService.ListBots(r.Context(), statusFilter)
	if err != nil {
		h.logger.WithError(err).Error("Failed to list bots")
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to list bots", err)
		return
	}

	h.logger.WithField("bot_count", len(bots)).Debug("Retrieved bots")

	response := BotListResponse{
		Bots:  bots,
		Count: len(bots),
	}

	h.writeJSONResponse(w, response)
}

// GetResourceMetrics handles bot resource metrics retrieval
func (h *Handler) GetResourceMetrics(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	botID := vars["id"]

	if botID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Bot ID is required", nil)
		return
	}

	// Get bot details
	bot, err := h.botService.GetBot(r.Context(), botID)
	if err != nil {
		if err == ErrBotNotFound {
			h.writeErrorResponse(w, http.StatusNotFound, "Bot not found", err)
		} else {
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get bot", err)
		}
		return
	}

	// Get bot metrics
	_, err = h.botService.GetMetrics(r.Context(), botID)
	if err != nil {
		h.logger.WithError(err).Warn("Failed to get bot metrics")
		// Continue with placeholder metrics
	}

	// Calculate uptime
	// Not used currently, but could be added to metrics

	// Build resource metrics
	resourceMetrics := &BotResourceMetrics{
		Timestamp:   time.Now(),
		CPUPercent:  0.0, // Would be fetched from bot
		MemoryUsage: 0,   // Would be fetched from bot
		MemoryLimit: 0,   // Would be fetched from bot
		DiskUsage:   0,   // Would be fetched from bot
		NetworkSent: 0,   // Would be fetched from bot
		NetworkRecv: 0,   // Would be fetched from bot
	}

	// If bot has an API endpoint and is online, we could fetch real metrics
	// For now, use placeholder values
	if bot.APIEndpoint != "" && bot.IsOnline {
		resourceMetrics.CPUPercent = 45.5     // Placeholder
		resourceMetrics.MemoryUsage = 1 << 30 // 1GB placeholder
		resourceMetrics.DiskUsage = 10 << 30  // 10GB placeholder
	}

	response := ResourceMetricsResponse{
		BotID:   botID,
		Metrics: resourceMetrics,
	}

	h.writeJSONResponse(w, response)
}

// Helper methods

func (h *Handler) validateRegisterRequest(req *BotRegisterRequest) error {
	// Validate hostname
	if req.Hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	if len(req.Hostname) > 255 {
		return fmt.Errorf("hostname too long (max 255 characters)")
	}

	// Validate name if provided
	if req.Name != "" && len(req.Name) > 100 {
		return fmt.Errorf("name too long (max 100 characters)")
	}

	// Validate capabilities
	if len(req.Capabilities) == 0 {
		return fmt.Errorf("at least one capability is required")
	}
	if len(req.Capabilities) > 10 {
		return fmt.Errorf("too many capabilities (max 10)")
	}
	for _, cap := range req.Capabilities {
		if cap == "" {
			return fmt.Errorf("empty capability string not allowed")
		}
		if len(cap) > 50 {
			return fmt.Errorf("capability string too long (max 50 characters)")
		}
		// Validate capability format (alphanumeric with underscores)
		if !isValidIdentifier(cap) {
			return fmt.Errorf("invalid capability format: %s", cap)
		}
	}

	// Validate API endpoint
	if req.APIEndpoint == "" {
		return fmt.Errorf("API endpoint is required")
	}
	if len(req.APIEndpoint) > 500 {
		return fmt.Errorf("API endpoint too long (max 500 characters)")
	}
	// Basic URL validation
	if len(req.APIEndpoint) < 7 || (req.APIEndpoint[:7] != "http://" && (len(req.APIEndpoint) < 8 || req.APIEndpoint[:8] != "https://")) {
		return fmt.Errorf("API endpoint must start with http:// or https://")
	}

	return nil
}

func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && (r >= '0' && r <= '9') {
			return false // Can't start with a number
		}
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// Define service errors that aren't exposed by the service package
var (
	ErrBotNotFound = fmt.Errorf("bot not found")
)
