package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/sse"
	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/registry"
	botRepo "github.com/ethpandaops/pandafuzz/pkg/domain/bot/repository"
	botTypes "github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
	jobRepo "github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	jobTypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
)

// BotAdapter implements the bot-related endpoints of the generated ServerInterface
type BotAdapter struct {
	registry *registry.Service
	botRepo  botRepo.AgentRepository
	jobRepo  jobRepo.JobRepository
	sse      *sse.Manager
	logger   logrus.FieldLogger
}

// Compile-time check to ensure BotAdapter implements part of ServerInterface
var _ generated.ServerInterface = (*BotAdapter)(nil)

// NewBotAdapter creates a new bot adapter
func NewBotAdapter(
	registry *registry.Service,
	botRepo botRepo.AgentRepository,
	jobRepo jobRepo.JobRepository,
	sse *sse.Manager,
	logger logrus.FieldLogger,
) *BotAdapter {
	return &BotAdapter{
		registry: registry,
		botRepo:  botRepo,
		jobRepo:  jobRepo,
		sse:      sse,
		logger:   logger.WithField("component", "bot_adapter"),
	}
}

// ListBots retrieves all registered bots with filtering and pagination
func (a *BotAdapter) ListBots(w http.ResponseWriter, r *http.Request, params generated.ListBotsParams) {
	ctx := r.Context()

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

	// Get bots from repository with pagination
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

	var req generated.BotCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Generate a unique ID for the bot
	botID := uuid.New().String()

	// Convert capabilities
	capabilities := make([]botTypes.Capability, len(req.Capabilities))
	for i, cap := range req.Capabilities {
		capabilities[i] = generatedToCapability(cap)
	}

	// Register the bot
	agent, err := a.registry.RegisterBot(ctx, botID, req.Name, capabilities)
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
		// Save metadata updates
		if err := a.botRepo.Update(ctx, agent); err != nil {
			a.logger.WithError(err).Warn("failed to update bot metadata")
		}
	}

	// Convert to API response
	bot := a.convertAgentToBot(agent)

	// Publish SSE event
	botUUID := uuid.MustParse(botID)
	event := sse.NewBotEvent("bot.created", botUUID, map[string]any{
		"bot":       bot,
		"timestamp": time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast bot created event")
	}

	a.writeJSONResponse(w, http.StatusCreated, bot)
}

// GetBot retrieves a specific bot by ID
func (a *BotAdapter) GetBot(w http.ResponseWriter, r *http.Request, botId generated.BotIdParam, params generated.GetBotParams) {
	ctx := r.Context()

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
			capabilities[i] = generatedToCapability(cap)
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

	// Check if bot exists first
	agent, err := a.botRepo.FindByID(ctx, botId.String())
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

	var req generated.BotHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Record heartbeat
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

	// Verify bot exists
	_, err := a.botRepo.FindByID(ctx, botId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "BOT_NOT_FOUND", "Bot not found", err)
		return
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
		if job.AssignedBotID != nil && *job.AssignedBotID == botId.String() {
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
		Hostname:      agent.Hostname,
		Status:        botStatusToGenerated(agent.Status),
		IsOnline:      agent.IsOnline(),
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
	apiJob := generated.Job{
		Id:           uuid.MustParse(job.ID),
		Name:         job.Name,
		Status:       domainJobStatusToGenerated(job.Status),
		CreatedAt:    job.CreatedAt,
		TargetBinary: job.TargetBinary,
		TimeoutAt:    job.TimeoutAt,
		Fuzzer:       generated.FuzzerType(job.FuzzerType),
	}

	if job.CampaignID != nil {
		campaignID := uuid.MustParse(*job.CampaignID)
		apiJob.CampaignId = &campaignID
	}

	if job.AssignedBotID != nil {
		botID := uuid.MustParse(*job.AssignedBotID)
		apiJob.AssignedBotId = &botID
	}

	if job.StartedAt != nil {
		apiJob.StartedAt = job.StartedAt
	}

	if job.CompletedAt != nil {
		apiJob.CompletedAt = job.CompletedAt
	}

	return apiJob
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

func generatedJobStatusToDomain(status generated.JobStatus) jobTypes.JobStatus {
	switch status {
	case generated.JobStatusPending:
		return jobTypes.StatusPending
	case generated.JobStatusAssigned:
		return jobTypes.StatusAssigned
	case generated.JobStatusRunning:
		return jobTypes.StatusRunning
	case generated.JobStatusCompleted:
		return jobTypes.StatusCompleted
	case generated.JobStatusFailed:
		return jobTypes.StatusFailed
	case generated.JobStatusCancelled:
		return jobTypes.StatusCanceled
	case generated.JobStatusTimeout:
		return jobTypes.StatusTimeout
	default:
		return jobTypes.StatusPending
	}
}

func domainJobStatusToGenerated(status jobTypes.JobStatus) generated.JobStatus {
	switch status {
	case jobTypes.StatusPending:
		return generated.JobStatusPending
	case jobTypes.StatusAssigned:
		return generated.JobStatusAssigned
	case jobTypes.StatusRunning:
		return generated.JobStatusRunning
	case jobTypes.StatusCompleted:
		return generated.JobStatusCompleted
	case jobTypes.StatusFailed:
		return generated.JobStatusFailed
	case jobTypes.StatusCanceled:
		return generated.JobStatusCancelled
	case jobTypes.StatusTimeout:
		return generated.JobStatusTimeout
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
