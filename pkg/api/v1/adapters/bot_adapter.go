package adapters

import (
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
	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/registry"
	botRepo "github.com/ethpandaops/pandafuzz/pkg/domain/bot/repository"
	botTypes "github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
	jobRepo "github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	jobTypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/ethpandaops/pandafuzz/pkg/service"
)

// BotAdapter implements the bot-related endpoints of the generated ServerInterface
type BotAdapter struct {
	registry   *registry.Service
	botRepo    botRepo.AgentRepository
	jobRepo    jobRepo.JobRepository
	botService service.BotService
	jobService service.JobService
	sse        *sse.Manager
	logger     logrus.FieldLogger
}

// Compile-time check to ensure BotAdapter implements part of ServerInterface
var _ generated.ServerInterface = (*BotAdapter)(nil)

// NewBotAdapter creates a new bot adapter
func NewBotAdapter(
	registry *registry.Service,
	botRepo botRepo.AgentRepository,
	jobRepo jobRepo.JobRepository,
	botService service.BotService,
	jobService service.JobService,
	sse *sse.Manager,
	logger logrus.FieldLogger,
) *BotAdapter {
	return &BotAdapter{
		registry:   registry,
		botRepo:    botRepo,
		jobRepo:    jobRepo,
		botService: botService,
		jobService: jobService,
		sse:        sse,
		logger:     logger.WithField("component", "bot_adapter"),
	}
}

// ListBots retrieves all registered bots with filtering and pagination
func (a *BotAdapter) ListBots(w http.ResponseWriter, r *http.Request, params generated.ListBotsParams) {
	ctx := r.Context()

	// Check if botService is available (preferred) or botRepo as fallback
	if a.botService == nil && a.botRepo == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Bot service not configured", nil)
		return
	}

	// Set defaults for pagination
	limit := 50
	offset := 0

	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
		if limit > 1000 {
			limit = 1000 // Cap at reasonable maximum
		}
	}

	if params.Offset != nil && *params.Offset >= 0 {
		offset = *params.Offset
	}

	// Use botService if available
	if a.botService != nil {
		// Convert status filter if provided
		var statusFilter *common.BotStatus
		if params.Status != nil {
			status := generatedToBotStatus(*params.Status)
			statusFilter = &status
		}

		// Get bots from service
		commonBots, err := a.botService.ListBots(ctx, statusFilter)
		if err != nil {
			a.logger.WithError(err).Error("failed to list bots")
			a.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve bots", err)
			return
		}

		// Filter by online status if specified
		if params.OnlineOnly != nil && *params.OnlineOnly {
			filtered := make([]*common.Bot, 0)
			for _, bot := range commonBots {
				if bot.IsOnline {
					filtered = append(filtered, bot)
				}
			}
			commonBots = filtered
		}

		// Apply pagination
		total := len(commonBots)
		if offset >= len(commonBots) {
			commonBots = []*common.Bot{}
		} else {
			end := offset + limit
			if end > len(commonBots) {
				end = len(commonBots)
			}
			commonBots = commonBots[offset:end]
		}

		// Convert to API types
		bots := make([]generated.Bot, len(commonBots))
		for i, bot := range commonBots {
			bots[i] = a.convertCommonBotToAPIBot(bot)
		}

		// Create pagination info
		hasMore := offset+len(bots) < total
		pagination := generated.Pagination{
			Limit:   limit,
			Offset:  offset,
			Total:   total,
			HasMore: hasMore,
		}

		response := generated.BotListResponse{
			Data:       bots,
			Pagination: pagination,
		}

		a.writeJSONResponse(w, http.StatusOK, response)
		return
	}

	// Fallback to repository if service is not available
	agents, total, err := a.botRepo.List(ctx, offset, limit)
	if err != nil {
		a.logger.WithError(err).Error("failed to list bots")
		a.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve bots", err)
		return
	}

	// Filter by status if specified
	if params.Status != nil {
		filtered := make([]*botTypes.Agent, 0)
		for _, agent := range agents {
			if botStatusToGenerated(agent.Status) == *params.Status {
				filtered = append(filtered, agent)
			}
		}
		agents = filtered
	}

	// Filter by online status if specified
	if params.OnlineOnly != nil && *params.OnlineOnly {
		filtered := make([]*botTypes.Agent, 0)
		for _, agent := range agents {
			if agent.IsOnline() {
				filtered = append(filtered, agent)
			}
		}
		agents = filtered
	}

	// Convert to API types
	bots := make([]generated.Bot, len(agents))
	for i, agent := range agents {
		bots[i] = a.convertAgentToBot(agent)
	}

	// Create pagination info
	hasMore := offset+len(bots) < total
	pagination := generated.Pagination{
		Limit:   limit,
		Offset:  offset,
		Total:   total,
		HasMore: hasMore,
	}

	response := generated.BotListResponse{
		Data:       bots,
		Pagination: pagination,
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// CreateBot registers a new bot
func (a *BotAdapter) CreateBot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check dependencies - prefer botService over registry
	if a.botService == nil && a.registry == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Bot service not configured", nil)
		return
	}

	var req generated.BotCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Validate required fields
	if req.Hostname == "" {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Hostname is required", nil)
		return
	}
	if len(req.Capabilities) == 0 {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "At least one capability is required", nil)
		return
	}

	// Convert capabilities to strings
	capabilities := make([]string, len(req.Capabilities))
	for i, cap := range req.Capabilities {
		capabilities[i] = capabilityToString(cap)
	}

	// Use botService if available
	if a.botService != nil {
		// Get API endpoint from request - use direct field first, then check metadata
		apiEndpoint := req.ApiEndpoint
		if apiEndpoint == "" && req.Metadata != nil {
			if ep, ok := (*req.Metadata)["api_endpoint"]; ok {
				if epStr, ok := ep.(string); ok {
					apiEndpoint = epStr
				}
			}
		}

		commonBot, err := a.botService.RegisterBot(ctx, req.Hostname, req.Name, capabilities, apiEndpoint)
		if err != nil {
			a.logger.WithError(err).Error("failed to register bot")
			a.writeError(w, http.StatusInternalServerError, "REGISTRATION_FAILED", "Failed to register bot", err)
			return
		}

		bot := a.convertCommonBotToAPIBot(commonBot)

		// Publish SSE event
		if a.sse != nil {
			event := sse.NewBotEvent("bot.created", bot.Id, map[string]any{
				"bot":       bot,
				"timestamp": time.Now(),
			})
			if err := a.sse.Broadcast(event); err != nil {
				a.logger.WithError(err).Warn("failed to broadcast bot created event")
			}
		}

		a.writeJSONResponse(w, http.StatusCreated, bot)
		return
	}

	// Fallback to registry
	botID := uuid.New().String()
	domainCapabilities := make([]botTypes.Capability, len(req.Capabilities))
	for i, cap := range req.Capabilities {
		domainCapabilities[i] = generatedToCapability(cap)
	}

	agent, err := a.registry.RegisterBot(ctx, botID, req.Name, domainCapabilities)
	if err != nil {
		a.logger.WithError(err).Error("failed to register bot")
		a.writeError(w, http.StatusInternalServerError, "REGISTRATION_FAILED", "Failed to register bot", err)
		return
	}

	// Set additional metadata
	if req.Metadata != nil {
		for key, value := range *req.Metadata {
			agent.SetMetadata(key, value)
		}
		if a.botRepo != nil {
			if err := a.botRepo.Update(ctx, agent); err != nil {
				a.logger.WithError(err).Warn("failed to update bot metadata")
			}
		}
	}

	bot := a.convertAgentToBot(agent)

	// Publish SSE event
	botUUID := uuid.MustParse(botID)
	if a.sse != nil {
		event := sse.NewBotEvent("bot.created", botUUID, map[string]any{
			"bot":       bot,
			"timestamp": time.Now(),
		})
		if err := a.sse.Broadcast(event); err != nil {
			a.logger.WithError(err).Warn("failed to broadcast bot created event")
		}
	}

	a.writeJSONResponse(w, http.StatusCreated, bot)
}

// GetBot retrieves a specific bot by ID
func (a *BotAdapter) GetBot(w http.ResponseWriter, r *http.Request, botId generated.BotIdParam, params generated.GetBotParams) {
	ctx := r.Context()

	// Check dependencies
	if a.botService == nil && a.botRepo == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Bot service not configured", nil)
		return
	}

	// Use botService if available
	if a.botService != nil {
		commonBot, err := a.botService.GetBot(ctx, botId.String())
		if err != nil {
			a.logger.WithError(err).WithField("bot_id", botId).Error("failed to get bot")
			a.writeError(w, http.StatusNotFound, "BOT_NOT_FOUND", "Bot not found", err)
			return
		}

		bot := a.convertCommonBotToAPIBot(commonBot)
		a.writeJSONResponse(w, http.StatusOK, bot)
		return
	}

	// Fallback to repository
	agent, err := a.botRepo.FindByID(ctx, botId.String())
	if err != nil {
		a.logger.WithError(err).WithField("bot_id", botId).Error("failed to get bot")
		a.writeError(w, http.StatusNotFound, "BOT_NOT_FOUND", "Bot not found", err)
		return
	}

	bot := a.convertAgentToBot(agent)
	a.writeJSONResponse(w, http.StatusOK, bot)
}

// UpdateBot updates an existing bot
func (a *BotAdapter) UpdateBot(w http.ResponseWriter, r *http.Request, botId generated.BotIdParam) {
	ctx := r.Context()

	if a.botService == nil && a.botRepo == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Bot service not configured", nil)
		return
	}

	var req generated.BotUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Get existing bot
	agent, err := a.botRepo.FindByID(ctx, botId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "BOT_NOT_FOUND", "Bot not found", err)
		return
	}

	// Update fields if provided
	if req.Name != nil {
		agent.Name = *req.Name
	}

	if req.ApiEndpoint != nil {
		agent.SetMetadata("api_endpoint", *req.ApiEndpoint)
	}

	if req.Capabilities != nil {
		capabilities := make([]botTypes.Capability, len(*req.Capabilities))
		for i, cap := range *req.Capabilities {
			capabilities[i] = generatedUpdateToCapability(cap)
		}
		agent.Capabilities = capabilities
	}

	if req.Metadata != nil {
		for key, value := range *req.Metadata {
			agent.SetMetadata(key, value)
		}
	}

	// Save changes
	if err := a.botRepo.Update(ctx, agent); err != nil {
		a.logger.WithError(err).Error("failed to update bot")
		a.writeError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update bot", err)
		return
	}

	bot := a.convertAgentToBot(agent)

	// Publish SSE event
	event := sse.NewBotEvent("bot.updated", botId, map[string]any{
		"bot":       bot,
		"timestamp": time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast bot updated event")
	}

	a.writeJSONResponse(w, http.StatusOK, bot)
}

// DeleteBot unregisters a bot
func (a *BotAdapter) DeleteBot(w http.ResponseWriter, r *http.Request, botId generated.BotIdParam) {
	ctx := r.Context()

	if a.botService == nil && (a.botRepo == nil || a.registry == nil) {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Bot services not configured", nil)
		return
	}

	// Use botService if available
	if a.botService != nil {
		if err := a.botService.DeregisterBot(ctx, botId.String()); err != nil {
			a.logger.WithError(err).Error("failed to deregister bot")
			a.writeError(w, http.StatusInternalServerError, "DEREGISTRATION_FAILED", "Failed to deregister bot", err)
			return
		}

		// Publish SSE event
		if a.sse != nil {
			event := sse.NewBotEvent("bot.deleted", botId, map[string]any{
				"bot_id":    botId.String(),
				"timestamp": time.Now(),
			})
			if err := a.sse.Broadcast(event); err != nil {
				a.logger.WithError(err).Warn("failed to broadcast bot deleted event")
			}
		}

		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Check if bot exists first
	_, err := a.botRepo.FindByID(ctx, botId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "BOT_NOT_FOUND", "Bot not found", err)
		return
	}

	// Use registry to properly deregister
	if err := a.registry.DeregisterBot(ctx, botId.String()); err != nil {
		a.logger.WithError(err).Error("failed to deregister bot")
		a.writeError(w, http.StatusInternalServerError, "DEREGISTRATION_FAILED", "Failed to deregister bot", err)
		return
	}

	// Publish SSE event
	event := sse.NewBotEvent("bot.deleted", botId, map[string]any{
		"bot_id":    botId.String(),
		"timestamp": time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast bot deleted event")
	}

	w.WriteHeader(http.StatusNoContent)
}

// SendBotHeartbeat handles bot heartbeat and returns commands
func (a *BotAdapter) SendBotHeartbeat(w http.ResponseWriter, r *http.Request, botId generated.BotIdParam) {
	ctx := r.Context()

	if a.botService == nil && (a.registry == nil || a.botRepo == nil) {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Bot services not configured", nil)
		return
	}

	var req generated.BotHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Use botService if available
	if a.botService != nil {
		// Update heartbeat through service
		status := generatedToBotStatus(req.Status)
		// Pass current job ID from request if provided
		var currentJobID *string
		if req.CurrentJobId != nil {
			jobIDStr := req.CurrentJobId.String()
			currentJobID = &jobIDStr
		}
		if err := a.botService.UpdateHeartbeat(ctx, botId.String(), status, currentJobID); err != nil {
			a.logger.WithError(err).Error("failed to record heartbeat")
			// Check if bot not found error
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not_found") {
				a.writeError(w, http.StatusNotFound, "BOT_NOT_FOUND", "Bot not found", err)
				return
			}
			a.writeError(w, http.StatusInternalServerError, "HEARTBEAT_FAILED", "Failed to record heartbeat", err)
			return
		}

		// Get current job if any
		var assignedJobId *uuid.UUID
		currentJob, err := a.botService.GetCurrentJob(ctx, botId.String())
		if err == nil && currentJob != nil {
			if id, err := uuid.Parse(currentJob.ID); err == nil {
				assignedJobId = &id
			}
		}

		response := generated.BotHeartbeatResponse{
			Acknowledged:                 true,
			AssignedJobId:                assignedJobId,
			NextHeartbeatIntervalSeconds: 30,
		}

		// Publish SSE heartbeat event
		if a.sse != nil {
			event := sse.NewBotEvent("bot.heartbeat", botId, map[string]any{
				"bot_id":    botId.String(),
				"status":    req.Status,
				"timestamp": time.Now(),
			})
			if err := a.sse.BroadcastToTopic("bot."+botId.String(), event); err != nil {
				a.logger.WithError(err).Warn("failed to broadcast heartbeat event")
			}
		}

		a.writeJSONResponse(w, http.StatusOK, response)
		return
	}

	// Fallback: Record heartbeat using registry
	if err := a.registry.RecordHeartbeat(ctx, botId.String()); err != nil {
		a.logger.WithError(err).Error("failed to record heartbeat")
		a.writeError(w, http.StatusInternalServerError, "HEARTBEAT_FAILED", "Failed to record heartbeat", err)
		return
	}

	// Update bot status if provided
	if req.Status != "" {
		domainStatus := generatedToStatus(req.Status)
		if err := a.registry.UpdateBotStatus(ctx, botId.String(), domainStatus, ""); err != nil {
			a.logger.WithError(err).Warn("failed to update bot status from heartbeat")
		}
	}

	// Get bot to check for assigned jobs
	agent, err := a.botRepo.FindByID(ctx, botId.String())
	if err != nil {
		a.logger.WithError(err).Error("failed to get bot after heartbeat")
		a.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to process heartbeat", err)
		return
	}

	// Check for newly assigned jobs
	var assignedJobId *uuid.UUID
	if currentJobID, exists := agent.GetMetadata("current_job_id"); exists && currentJobID != nil {
		if id, err := uuid.Parse(currentJobID.(string)); err == nil {
			assignedJobId = &id
		}
	}

	response := generated.BotHeartbeatResponse{
		Acknowledged:                 true,
		AssignedJobId:                assignedJobId,
		NextHeartbeatIntervalSeconds: 30, // Default 30 seconds
	}

	// Publish SSE heartbeat event
	event := sse.NewBotEvent("bot.heartbeat", botId, map[string]any{
		"bot_id":    botId.String(),
		"status":    req.Status,
		"timestamp": time.Now(),
	})
	if err := a.sse.BroadcastToTopic("bot."+botId.String(), event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast heartbeat event")
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// GetBotJobs retrieves jobs assigned to a specific bot
func (a *BotAdapter) GetBotJobs(w http.ResponseWriter, r *http.Request, botId generated.BotIdParam, params generated.GetBotJobsParams) {
	ctx := r.Context()

	if (a.botService == nil && a.jobService == nil) && (a.botRepo == nil || a.jobRepo == nil) {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Bot or job service not configured", nil)
		return
	}

	// Verify bot exists - use service if available
	if a.botService != nil {
		_, err := a.botService.GetBot(ctx, botId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "BOT_NOT_FOUND", "Bot not found", err)
			return
		}
	} else if a.botRepo != nil {
		_, err := a.botRepo.FindByID(ctx, botId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "BOT_NOT_FOUND", "Bot not found", err)
			return
		}
	}

	// Build filter for jobs assigned to this bot
	filter := jobRepo.JobFilter{
		Limit:  50,
		Offset: 0,
	}

	if params.Limit != nil && *params.Limit > 0 {
		filter.Limit = *params.Limit
		if filter.Limit > 1000 {
			filter.Limit = 1000
		}
	}

	if params.Offset != nil && *params.Offset >= 0 {
		filter.Offset = *params.Offset
	}

	if params.Status != nil {
		domainStatus := generatedJobStatusToDomain(*params.Status)
		filter.Status = &domainStatus
	}

	// Get jobs from repository
	jobs, err := a.jobRepo.List(ctx, filter)
	if err != nil {
		a.logger.WithError(err).Error("failed to get bot jobs")
		a.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve jobs", err)
		return
	}

	// Filter jobs assigned to this bot
	var botJobs []*jobTypes.Job
	for _, job := range jobs {
		if job.LockedBy == botId.String() {
			botJobs = append(botJobs, job)
		}
	}

	// Convert to API response
	apiJobs := make([]generated.Job, len(botJobs))
	for i, job := range botJobs {
		apiJobs[i] = a.convertJobToAPI(job)
	}

	// Create pagination info
	pagination := generated.Pagination{
		Limit:   filter.Limit,
		Offset:  filter.Offset,
		Total:   len(apiJobs),
		HasMore: false, // Since we're filtering after query, we can't determine this accurately
	}

	response := generated.JobListResponse{
		Data:       apiJobs,
		Pagination: pagination,
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// Helper methods

func (a *BotAdapter) convertAgentToBot(agent *botTypes.Agent) generated.Bot {
	bot := generated.Bot{
		Id:            uuid.MustParse(agent.ID),
		Name:          agent.Name,
		Hostname:      agent.Name, // Use name as hostname for now
		Status:        botStatusToGenerated(agent.Status),
		IsOnline:      agent.Status == botTypes.StatusIdle || agent.Status == botTypes.StatusWorking,
		RegisteredAt:  agent.CreatedAt,
		LastHeartbeat: agent.LastHeartbeat,
	}

	// Convert capabilities
	bot.Capabilities = make([]generated.BotCapabilities, len(agent.Capabilities))
	for i, cap := range agent.Capabilities {
		bot.Capabilities[i] = capabilityToGenerated(cap)
	}

	// Set current job ID if exists
	if currentJobID, exists := agent.GetMetadata("current_job_id"); exists && currentJobID != nil {
		if id, err := uuid.Parse(currentJobID.(string)); err == nil {
			bot.CurrentJobId = &id
		}
	}

	// Set API endpoint if exists
	if apiEndpoint, exists := agent.GetMetadata("api_endpoint"); exists && apiEndpoint != nil {
		endpoint := apiEndpoint.(string)
		bot.ApiEndpoint = &endpoint
	}

	// Set metadata
	if len(agent.Metadata) > 0 {
		metadata := make(generated.Metadata)
		for key, value := range agent.Metadata {
			metadata[key] = value
		}
		bot.Metadata = &metadata
	}

	return bot
}

func (a *BotAdapter) convertJobToAPI(job *jobTypes.Job) generated.Job {
	// Calculate timeout from scheduled time + max duration
	timeoutAt := time.Now().Add(24 * time.Hour) // Default timeout
	if job.ScheduledAt != nil && job.MaxDuration > 0 {
		timeoutAt = job.ScheduledAt.Add(job.MaxDuration)
	}

	apiJob := generated.Job{
		Id:           uuid.MustParse(job.ID),
		Name:         job.Name,
		Status:       domainJobStatusToGenerated(job.Status),
		CreatedAt:    job.CreatedAt,
		TargetBinary: job.TargetBinary,
		TimeoutAt:    timeoutAt,
		Fuzzer:       generated.FuzzerType(job.FuzzerType),
	}

	// Check if campaign ID is in metadata
	if job.Metadata != nil {
		if campaignID, ok := job.Metadata["campaign_id"]; ok {
			if id, err := uuid.Parse(campaignID); err == nil {
				apiJob.CampaignId = &id
			}
		}
	}

	// Use LockedBy as AssignedBotId
	if job.LockedBy != "" {
		if botID, err := uuid.Parse(job.LockedBy); err == nil {
			apiJob.AssignedBotId = &botID
		}
	}

	if job.StartedAt != nil {
		apiJob.StartedAt = job.StartedAt
	}

	if job.CompletedAt != nil {
		apiJob.CompletedAt = job.CompletedAt
	}

	return apiJob
}

// convertCommonJobToAPI converts a common.Job to a generated.Job
func (a *BotAdapter) convertCommonJobToAPI(job *common.Job) generated.Job {
	// Try to parse ID as UUID, if it fails generate a deterministic UUID from the ID
	jobID, err := uuid.Parse(job.ID)
	if err != nil {
		// Generate a deterministic UUID from the job ID using UUID v5 with DNS namespace
		jobID = uuid.NewSHA1(uuid.NameSpaceDNS, []byte(job.ID))
	}

	apiJob := generated.Job{
		Id:           jobID,
		Name:         job.Name,
		Status:       commonJobStatusToGenerated(job.Status),
		CreatedAt:    job.CreatedAt,
		TargetBinary: job.Target,
		TimeoutAt:    job.TimeoutAt,
		Fuzzer:       generated.FuzzerType(job.Fuzzer),
	}

	// Set campaign ID if exists
	if job.CampaignID != nil {
		if id, err := uuid.Parse(*job.CampaignID); err == nil {
			apiJob.CampaignId = &id
		}
	}

	// Set assigned bot ID
	if job.AssignedBot != nil {
		botID, err := uuid.Parse(*job.AssignedBot)
		if err != nil {
			botID = uuid.NewSHA1(uuid.NameSpaceDNS, []byte(*job.AssignedBot))
		}
		apiJob.AssignedBotId = &botID
	}

	if job.StartedAt != nil {
		apiJob.StartedAt = job.StartedAt
	}

	if job.CompletedAt != nil {
		apiJob.CompletedAt = job.CompletedAt
	}

	// Set priority
	priority := job.Priority
	apiJob.Priority = &priority

	// Set coverage
	enableCoverage := job.EnableCoverage
	apiJob.EnableCoverage = &enableCoverage

	return apiJob
}

// commonJobStatusToGenerated converts common.JobStatus to generated.JobStatus
func commonJobStatusToGenerated(status common.JobStatus) generated.JobStatus {
	switch status {
	case common.JobStatusPending:
		return generated.JobStatusPending
	case common.JobStatusAssigned:
		return generated.JobStatusAssigned
	case common.JobStatusStarting:
		return generated.JobStatusRunning
	case common.JobStatusRunning:
		return generated.JobStatusRunning
	case common.JobStatusCompleted:
		return generated.JobStatusCompleted
	case common.JobStatusFailed:
		return generated.JobStatusFailed
	case common.JobStatusTimedOut:
		return generated.JobStatusTimeout
	case common.JobStatusCancelled:
		return generated.JobStatusCancelled
	default:
		return generated.JobStatusPending
	}
}

// Conversion helpers
func botStatusToGenerated(status botTypes.Status) generated.BotStatus {
	switch status {
	case botTypes.StatusIdle:
		return generated.BotStatusIdle
	case botTypes.StatusWorking:
		return generated.BotStatusBusy
	case botTypes.StatusError:
		return generated.BotStatusError
	case botTypes.StatusMaintenance:
		return generated.BotStatusMaintenance
	case botTypes.StatusOffline:
		return generated.BotStatusOffline
	default:
		return generated.BotStatusOffline
	}
}

func generatedToStatus(status generated.BotStatus) botTypes.Status {
	switch status {
	case generated.BotStatusIdle:
		return botTypes.StatusIdle
	case generated.BotStatusBusy:
		return botTypes.StatusWorking
	case generated.BotStatusError:
		return botTypes.StatusError
	case generated.BotStatusMaintenance:
		return botTypes.StatusMaintenance
	case generated.BotStatusOffline:
		return botTypes.StatusOffline
	default:
		return botTypes.StatusOffline
	}
}

func capabilityToGenerated(cap botTypes.Capability) generated.BotCapabilities {
	switch cap {
	case botTypes.CapabilityFuzzing:
		return generated.BotCapabilitiesFuzzing
	case botTypes.CapabilityAnalysis:
		return generated.BotCapabilitiesAnalysis
	case botTypes.CapabilityReporting:
		return generated.BotCapabilitiesReproduction
	case botTypes.CapabilityCoordination:
		return generated.BotCapabilitiesCoverage
	default:
		return generated.BotCapabilitiesFuzzing
	}
}

func generatedToCapability(cap generated.BotCreateRequestCapabilities) botTypes.Capability {
	switch cap {
	case generated.BotCreateRequestCapabilitiesFuzzing:
		return botTypes.CapabilityFuzzing
	case generated.BotCreateRequestCapabilitiesAnalysis:
		return botTypes.CapabilityAnalysis
	case generated.BotCreateRequestCapabilitiesReproduction:
		return botTypes.CapabilityReporting
	case generated.BotCreateRequestCapabilitiesCoverage:
		return botTypes.CapabilityCoordination
	default:
		return botTypes.CapabilityFuzzing
	}
}

func generatedUpdateToCapability(cap generated.BotUpdateRequestCapabilities) botTypes.Capability {
	// Convert update capabilities to bot types capabilities
	switch cap {
	case generated.BotUpdateRequestCapabilitiesFuzzing:
		return botTypes.CapabilityFuzzing
	case generated.BotUpdateRequestCapabilitiesAnalysis:
		return botTypes.CapabilityAnalysis
	case generated.BotUpdateRequestCapabilitiesReproduction:
		return botTypes.CapabilityReporting
	case generated.BotUpdateRequestCapabilitiesCoverage:
		return botTypes.CapabilityCoordination
	default:
		return botTypes.CapabilityFuzzing
	}
}

// generatedToBotStatus converts generated.BotStatus to common.BotStatus
func generatedToBotStatus(status generated.BotStatus) common.BotStatus {
	switch status {
	case generated.BotStatusIdle:
		return common.BotStatusIdle
	case generated.BotStatusBusy:
		return common.BotStatusBusy
	case generated.BotStatusError:
		return common.BotStatusFailed
	case generated.BotStatusMaintenance:
		return common.BotStatusIdle // No direct mapping
	case generated.BotStatusOffline:
		return common.BotStatusTimedOut
	default:
		return common.BotStatusIdle
	}
}

// commonBotStatusToGenerated converts common.BotStatus to generated.BotStatus
func commonBotStatusToGenerated(status common.BotStatus) generated.BotStatus {
	switch status {
	case common.BotStatusIdle:
		return generated.BotStatusIdle
	case common.BotStatusBusy:
		return generated.BotStatusBusy
	case common.BotStatusRegistering:
		return generated.BotStatusIdle
	case common.BotStatusTimedOut:
		return generated.BotStatusOffline
	case common.BotStatusFailed:
		return generated.BotStatusError
	default:
		return generated.BotStatusOffline
	}
}

// convertCommonBotToAPIBot converts a common.Bot to a generated.Bot
func (a *BotAdapter) convertCommonBotToAPIBot(bot *common.Bot) generated.Bot {
	// Try to parse ID as UUID, if it fails generate a deterministic UUID from the ID
	botID, err := uuid.Parse(bot.ID)
	if err != nil {
		// Generate a deterministic UUID from the bot ID using UUID v5 with DNS namespace
		botID = uuid.NewSHA1(uuid.NameSpaceDNS, []byte(bot.ID))
	}

	apiBot := generated.Bot{
		Id:            botID,
		Name:          bot.Name,
		Hostname:      bot.Hostname,
		Status:        commonBotStatusToGenerated(bot.Status),
		IsOnline:      bot.IsOnline,
		RegisteredAt:  bot.RegisteredAt,
		LastHeartbeat: bot.LastSeen,
	}

	// Convert capabilities
	apiBot.Capabilities = make([]generated.BotCapabilities, len(bot.Capabilities))
	for i, cap := range bot.Capabilities {
		apiBot.Capabilities[i] = stringToCapability(cap)
	}

	// Set current job ID if exists
	if bot.CurrentJob != nil {
		if id, err := uuid.Parse(*bot.CurrentJob); err == nil {
			apiBot.CurrentJobId = &id
		}
	}

	// Set API endpoint if exists
	if bot.APIEndpoint != "" {
		apiBot.ApiEndpoint = &bot.APIEndpoint
	}

	return apiBot
}

// stringToCapability converts a string capability to generated.BotCapabilities
func stringToCapability(cap string) generated.BotCapabilities {
	switch cap {
	case "fuzzing":
		return generated.BotCapabilitiesFuzzing
	case "analysis":
		return generated.BotCapabilitiesAnalysis
	case "reproduction":
		return generated.BotCapabilitiesReproduction
	case "coverage":
		return generated.BotCapabilitiesCoverage
	default:
		return generated.BotCapabilitiesFuzzing
	}
}

// capabilityToString converts generated.BotCreateRequestCapabilities to string
func capabilityToString(cap generated.BotCreateRequestCapabilities) string {
	// Pass through the capability string as-is to support fuzzer-specific capabilities
	// like "afl++", "libfuzzer", "honggfuzz" etc.
	capStr := string(cap)
	if capStr != "" {
		return capStr
	}
	// Only map known capabilities if needed for fallback
	switch cap {
	case generated.BotCreateRequestCapabilitiesFuzzing:
		return "fuzzing"
	case generated.BotCreateRequestCapabilitiesAnalysis:
		return "analysis"
	case generated.BotCreateRequestCapabilitiesReproduction:
		return "reproduction"
	case generated.BotCreateRequestCapabilitiesCoverage:
		return "coverage"
	default:
		return "fuzzing"
	}
}

func generatedJobStatusToDomain(status generated.JobStatus) jobTypes.JobStatus {
	switch status {
	case generated.JobStatusPending:
		return jobTypes.StatusPending
	case generated.JobStatusAssigned:
		return jobTypes.StatusQueued // Use Queued for Assigned
	case generated.JobStatusRunning:
		return jobTypes.StatusRunning
	case generated.JobStatusCompleted:
		return jobTypes.StatusCompleted
	case generated.JobStatusFailed:
		return jobTypes.StatusFailed
	case generated.JobStatusCancelled:
		return jobTypes.StatusCancelled
	case generated.JobStatusTimeout:
		return jobTypes.StatusFailed // No timeout status in domain, map to failed
	default:
		return jobTypes.StatusPending
	}
}

func domainJobStatusToGenerated(status jobTypes.JobStatus) generated.JobStatus {
	switch status {
	case jobTypes.StatusPending:
		return generated.JobStatusPending
	case jobTypes.StatusQueued:
		return generated.JobStatusAssigned // Map queued to assigned
	case jobTypes.StatusRunning:
		return generated.JobStatusRunning
	case jobTypes.StatusCompleted:
		return generated.JobStatusCompleted
	case jobTypes.StatusFailed:
		return generated.JobStatusFailed
	case jobTypes.StatusCancelled:
		return generated.JobStatusCancelled
	case jobTypes.StatusPaused:
		return generated.JobStatusPending // Map paused to pending
	default:
		return generated.JobStatusPending
	}
}

func (a *BotAdapter) writeJSONResponse(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		a.logger.WithError(err).Error("failed to encode JSON response")
	}
}

func (a *BotAdapter) writeError(w http.ResponseWriter, statusCode int, errorType, title string, err error) {
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

// Placeholder implementations for unhandled endpoints
func (a *BotAdapter) GetAnalytics(w http.ResponseWriter, r *http.Request, params generated.GetAnalyticsParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) GetCoverageTrends(w http.ResponseWriter, r *http.Request, params generated.GetCoverageTrendsParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) GetMetrics(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) GetPerformanceStats(w http.ResponseWriter, r *http.Request, params generated.GetPerformanceStatsParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) ExecuteBatch(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) ListCampaigns(w http.ResponseWriter, r *http.Request, params generated.ListCampaignsParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) CreateCampaign(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) DeleteCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) GetCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam, params generated.GetCampaignParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) UpdateCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) StartCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) GetCampaignStats(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) StopCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) ListCorpus(w http.ResponseWriter, r *http.Request, params generated.ListCorpusParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) UploadCorpus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) ListQuarantinedCorpus(w http.ResponseWriter, r *http.Request, params generated.ListQuarantinedCorpusParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) SelectCorpus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) SyncCorpus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) DeleteCorpusEntry(w http.ResponseWriter, r *http.Request, entryId generated.CorpusEntryIdParam) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) GetCorpusEntry(w http.ResponseWriter, r *http.Request, entryId generated.CorpusEntryIdParam, params generated.GetCorpusEntryParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) DownloadCorpusFile(w http.ResponseWriter, r *http.Request, entryId generated.CorpusEntryIdParam) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) ListCrashes(w http.ResponseWriter, r *http.Request, params generated.ListCrashesParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) CreateCrash(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) GetCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam, params generated.GetCrashParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) DeduplicateCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) MinimizeCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) ReproduceCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) GetEventStream(w http.ResponseWriter, r *http.Request, params generated.GetEventStreamParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) GetHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) ListJobs(w http.ResponseWriter, r *http.Request, params generated.ListJobsParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) CreateJob(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) DeleteJob(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) GetJob(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, params generated.GetJobParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) UpdateJob(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) GetJobArtifacts(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, params generated.GetJobArtifactsParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) GetJobCoverage(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, params generated.GetJobCoverageParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) DownloadCoverageReport(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, reportId generated.ReportIdParam) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) GetJobLogs(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, params generated.GetJobLogsParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (a *BotAdapter) GetReadiness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

// GetNextJob assigns the next available job to a bot (from v3)
func (a *BotAdapter) GetNextJob(w http.ResponseWriter, r *http.Request, botId generated.BotIdParam) {
	ctx := r.Context()

	if (a.botService == nil || a.jobService == nil) && (a.botRepo == nil || a.jobRepo == nil) {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Bot or job service not configured", nil)
		return
	}

	// Use services if available
	if a.botService != nil && a.jobService != nil {
		// Verify bot exists and is available
		bot, err := a.botService.GetBot(ctx, botId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "BOT_NOT_FOUND", "Bot not found", err)
			return
		}

		if bot.Status != common.BotStatusIdle {
			a.writeError(w, http.StatusConflict, "BOT_NOT_AVAILABLE", "Bot is not available for new jobs", nil)
			return
		}

		// Assign next job through service
		commonJob, err := a.jobService.AssignNextJob(ctx, botId.String())
		if err != nil {
			// No jobs available
			a.writeJSONResponse(w, http.StatusOK, map[string]interface{}{
				"job":     nil,
				"message": "No pending jobs available",
			})
			return
		}

		apiJob := a.convertCommonJobToAPI(commonJob)
		response := map[string]interface{}{
			"job":              apiJob,
			"lease_token":      "", // Service handles lease internally
			"lease_expires_at": time.Now().Add(60 * time.Second),
		}

		a.writeJSONResponse(w, http.StatusOK, response)
		return
	}

	// Fallback to repository
	// Verify bot exists and is available
	agent, err := a.botRepo.FindByID(ctx, botId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "BOT_NOT_FOUND", "Bot not found", err)
		return
	}

	if agent.Status != botTypes.StatusIdle {
		a.writeError(w, http.StatusConflict, "BOT_NOT_AVAILABLE", "Bot is not available for new jobs", nil)
		return
	}

	// Find pending jobs
	pendingStatus := jobTypes.StatusPending
	filter := jobRepo.JobFilter{
		Status: &pendingStatus,
		Limit:  1,
	}

	jobs, err := a.jobRepo.List(ctx, filter)
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch pending jobs", err)
		return
	}

	if len(jobs) == 0 {
		// No jobs available
		a.writeJSONResponse(w, http.StatusOK, map[string]interface{}{
			"job":     nil,
			"message": "No pending jobs available",
		})
		return
	}

	job := jobs[0]

	// Generate lease token
	leaseToken := uuid.New().String()
	leaseExpiresAt := time.Now().Add(60 * time.Second)

	// Assign job to bot
	job.Status = jobTypes.StatusQueued
	job.LockedBy = botId.String()
	job.LeaseToken = &leaseToken
	job.LeaseExpiresAt = &leaseExpiresAt

	if err := a.jobRepo.Update(ctx, job); err != nil {
		a.writeError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to assign job", err)
		return
	}

	// Update bot status
	agent.Status = botTypes.StatusWorking
	agent.SetMetadata("current_job_id", job.ID)
	if err := a.botRepo.Update(ctx, agent); err != nil {
		a.logger.WithError(err).Warn("failed to update bot status")
	}

	apiJob := a.convertJobToAPI(job)
	response := map[string]interface{}{
		"job":              apiJob,
		"lease_token":      leaseToken,
		"lease_expires_at": leaseExpiresAt,
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// CompleteJob marks a job as completed by a bot (from v3)
func (a *BotAdapter) CompleteJob(w http.ResponseWriter, r *http.Request, botId generated.BotIdParam) {
	ctx := r.Context()

	if (a.botService == nil || a.jobService == nil) && (a.botRepo == nil || a.jobRepo == nil) {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Bot or job service not configured", nil)
		return
	}

	var req struct {
		JobID        string `json:"job_id"`
		LeaseToken   string `json:"lease_token"`
		Success      bool   `json:"success"`
		ErrorMessage string `json:"error_message,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Use services if available
	if a.jobService != nil {
		// Complete the job through the service
		if err := a.jobService.CompleteJob(ctx, req.JobID, botId.String(), req.Success); err != nil {
			a.logger.WithError(err).Error("failed to complete job")
			a.writeError(w, http.StatusInternalServerError, "COMPLETION_FAILED", "Failed to complete job", err)
			return
		}

		// Publish SSE event
		if a.sse != nil {
			jobUUID, _ := uuid.Parse(req.JobID)
			if jobUUID == uuid.Nil {
				jobUUID = uuid.NewSHA1(uuid.NameSpaceDNS, []byte(req.JobID))
			}
			campaignUUID := uuid.New()
			eventType := "job.completed"
			if !req.Success {
				eventType = "job.failed"
			}
			event := sse.NewJobEvent(eventType, jobUUID, campaignUUID, map[string]any{
				"job_id":    req.JobID,
				"bot_id":    botId.String(),
				"success":   req.Success,
				"timestamp": time.Now(),
			})
			if err := a.sse.Broadcast(event); err != nil {
				a.logger.WithError(err).Warn("failed to broadcast job completion event")
			}
		}

		a.writeJSONResponse(w, http.StatusOK, map[string]interface{}{
			"acknowledged": true,
			"message":      "Job completion recorded",
		})
		return
	}

	// Get job
	job, err := a.jobRepo.Get(ctx, req.JobID)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", err)
		return
	}

	// Verify lease token
	if job.LeaseToken != nil && *job.LeaseToken != req.LeaseToken {
		a.writeError(w, http.StatusUnauthorized, "INVALID_LEASE", "Invalid lease token", nil)
		return
	}

	// Verify bot owns the job
	if job.LockedBy != botId.String() {
		a.writeError(w, http.StatusForbidden, "NOT_OWNER", "Bot does not own this job", nil)
		return
	}

	// Update job status
	now := time.Now()
	job.CompletedAt = &now
	if req.Success {
		job.Status = jobTypes.StatusCompleted
	} else {
		job.Status = jobTypes.StatusFailed
		if job.Metadata == nil {
			job.Metadata = make(map[string]string)
		}
		job.Metadata["error_message"] = req.ErrorMessage
	}

	if err := a.jobRepo.Update(ctx, job); err != nil {
		a.writeError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update job", err)
		return
	}

	// Update bot status back to idle
	agent, err := a.botRepo.FindByID(ctx, botId.String())
	if err == nil {
		agent.Status = botTypes.StatusIdle
		if agent.Metadata != nil {
			delete(agent.Metadata, "current_job_id")
		}
		if err := a.botRepo.Update(ctx, agent); err != nil {
			a.logger.WithError(err).Warn("failed to update bot status")
		}
	}

	// Publish SSE event
	jobUUID := uuid.MustParse(job.ID)
	campaignUUID := uuid.New()
	eventType := "job.completed"
	if !req.Success {
		eventType = "job.failed"
	}
	event := sse.NewJobEvent(eventType, jobUUID, campaignUUID, map[string]any{
		"job_id":    job.ID,
		"bot_id":    botId.String(),
		"success":   req.Success,
		"timestamp": time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast job completion event")
	}

	a.writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"acknowledged": true,
		"message":      "Job completion recorded",
	})
}

// GetBotMetrics returns metrics for a specific bot (from v3)
func (a *BotAdapter) GetBotMetrics(w http.ResponseWriter, r *http.Request, botId generated.BotIdParam) {
	ctx := r.Context()

	if (a.botService == nil || a.jobService == nil) && (a.botRepo == nil || a.jobRepo == nil) {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Bot or job service not configured", nil)
		return
	}

	// Use services if available
	if a.botService != nil && a.jobService != nil {
		bot, err := a.botService.GetBot(ctx, botId.String())
		if err != nil {
			a.writeError(w, http.StatusNotFound, "BOT_NOT_FOUND", "Bot not found", err)
			return
		}

		// Basic metrics from bot info
		metrics := map[string]interface{}{
			"bot_id":         botId.String(),
			"name":           bot.Name,
			"status":         commonBotStatusToGenerated(bot.Status),
			"last_heartbeat": bot.LastSeen,
			"registered_at":  bot.RegisteredAt,
			"timestamp":      time.Now(),
		}

		a.writeJSONResponse(w, http.StatusOK, metrics)
		return
	}

	// Fallback: Verify bot exists
	agent, err := a.botRepo.FindByID(ctx, botId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "BOT_NOT_FOUND", "Bot not found", err)
		return
	}

	// Get job metrics for this bot
	filter := jobRepo.JobFilter{
		Limit: 1000, // Get all jobs to calculate metrics
	}

	jobs, err := a.jobRepo.List(ctx, filter)
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch job metrics", err)
		return
	}

	// Calculate metrics
	var totalJobs, completedJobs, failedJobs int
	var totalDuration time.Duration

	for _, job := range jobs {
		if job.LockedBy == botId.String() {
			totalJobs++
			switch job.Status {
			case jobTypes.StatusCompleted:
				completedJobs++
				if job.StartedAt != nil && job.CompletedAt != nil {
					totalDuration += job.CompletedAt.Sub(*job.StartedAt)
				}
			case jobTypes.StatusFailed:
				failedJobs++
			}
		}
	}

	avgDuration := float64(0)
	if completedJobs > 0 {
		avgDuration = totalDuration.Seconds() / float64(completedJobs)
	}

	successRate := float64(0)
	if totalJobs > 0 {
		successRate = float64(completedJobs) / float64(totalJobs) * 100
	}

	metrics := map[string]interface{}{
		"bot_id":                   botId.String(),
		"name":                     agent.Name,
		"status":                   botStatusToGenerated(agent.Status),
		"total_jobs":               totalJobs,
		"completed_jobs":           completedJobs,
		"failed_jobs":              failedJobs,
		"success_rate":             successRate,
		"average_job_duration_sec": avgDuration,
		"last_heartbeat":           agent.LastHeartbeat,
		"registered_at":            agent.CreatedAt,
		"timestamp":                time.Now(),
	}

	a.writeJSONResponse(w, http.StatusOK, metrics)
}
