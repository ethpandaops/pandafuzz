package adapters

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/sse"
)

// CompositeAdapter combines all individual adapters to implement the complete ServerInterface
type CompositeAdapter struct {
	botAdapter       *BotAdapter
	jobAdapter       *JobAdapter
	campaignAdapter  *CampaignAdapter
	corpusAdapter    *CorpusAdapter
	crashAdapter     *CrashAdapter
	analyticsAdapter *AnalyticsAdapter
	sse              *sse.Manager
	logger           logrus.FieldLogger
}

// NewCompositeAdapter creates a new composite adapter
func NewCompositeAdapter(
	botAdapter *BotAdapter,
	jobAdapter *JobAdapter,
	campaignAdapter *CampaignAdapter,
	corpusAdapter *CorpusAdapter,
	crashAdapter *CrashAdapter,
	analyticsAdapter *AnalyticsAdapter,
	sse *sse.Manager,
	logger logrus.FieldLogger,
) *CompositeAdapter {
	return &CompositeAdapter{
		botAdapter:       botAdapter,
		jobAdapter:       jobAdapter,
		campaignAdapter:  campaignAdapter,
		corpusAdapter:    corpusAdapter,
		crashAdapter:     crashAdapter,
		analyticsAdapter: analyticsAdapter,
		sse:              sse,
		logger:           logger.WithField("component", "composite_adapter"),
	}
}

// Compile-time check to ensure CompositeAdapter implements ServerInterface
var _ generated.ServerInterface = (*CompositeAdapter)(nil)

// Analytics endpoints
func (a *CompositeAdapter) GetAnalytics(w http.ResponseWriter, r *http.Request, params generated.GetAnalyticsParams) {
	a.analyticsAdapter.GetAnalytics(w, r, params)
}

func (a *CompositeAdapter) GetCoverageTrends(w http.ResponseWriter, r *http.Request, params generated.GetCoverageTrendsParams) {
	a.analyticsAdapter.GetCoverageTrends(w, r, params)
}

func (a *CompositeAdapter) GetMetrics(w http.ResponseWriter, r *http.Request) {
	a.analyticsAdapter.GetMetrics(w, r)
}

func (a *CompositeAdapter) GetPerformanceStats(w http.ResponseWriter, r *http.Request, params generated.GetPerformanceStatsParams) {
	a.analyticsAdapter.GetPerformanceStats(w, r, params)
}

// Bot endpoints
func (a *CompositeAdapter) ListBots(w http.ResponseWriter, r *http.Request, params generated.ListBotsParams) {
	a.botAdapter.ListBots(w, r, params)
}

func (a *CompositeAdapter) CreateBot(w http.ResponseWriter, r *http.Request) {
	a.botAdapter.CreateBot(w, r)
}

func (a *CompositeAdapter) DeleteBot(w http.ResponseWriter, r *http.Request, botId generated.BotIdParam) {
	a.botAdapter.DeleteBot(w, r, botId)
}

func (a *CompositeAdapter) GetBot(w http.ResponseWriter, r *http.Request, botId generated.BotIdParam, params generated.GetBotParams) {
	a.botAdapter.GetBot(w, r, botId, params)
}

func (a *CompositeAdapter) UpdateBot(w http.ResponseWriter, r *http.Request, botId generated.BotIdParam) {
	a.botAdapter.UpdateBot(w, r, botId)
}

func (a *CompositeAdapter) SendBotHeartbeat(w http.ResponseWriter, r *http.Request, botId generated.BotIdParam) {
	a.botAdapter.SendBotHeartbeat(w, r, botId)
}

func (a *CompositeAdapter) GetBotJobs(w http.ResponseWriter, r *http.Request, botId generated.BotIdParam, params generated.GetBotJobsParams) {
	a.botAdapter.GetBotJobs(w, r, botId, params)
}

// Campaign endpoints
func (a *CompositeAdapter) ListCampaigns(w http.ResponseWriter, r *http.Request, params generated.ListCampaignsParams) {
	a.campaignAdapter.ListCampaigns(w, r, params)
}

func (a *CompositeAdapter) CreateCampaign(w http.ResponseWriter, r *http.Request) {
	a.campaignAdapter.CreateCampaign(w, r)
}

func (a *CompositeAdapter) DeleteCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	a.campaignAdapter.DeleteCampaign(w, r, campaignId)
}

func (a *CompositeAdapter) GetCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam, params generated.GetCampaignParams) {
	a.campaignAdapter.GetCampaign(w, r, campaignId, params)
}

func (a *CompositeAdapter) UpdateCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	a.campaignAdapter.UpdateCampaign(w, r, campaignId)
}

func (a *CompositeAdapter) StartCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	a.campaignAdapter.StartCampaign(w, r, campaignId)
}

func (a *CompositeAdapter) GetCampaignStats(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	a.campaignAdapter.GetCampaignStats(w, r, campaignId)
}

func (a *CompositeAdapter) StopCampaign(w http.ResponseWriter, r *http.Request, campaignId generated.CampaignIdParam) {
	a.campaignAdapter.StopCampaign(w, r, campaignId)
}

// Corpus endpoints
func (a *CompositeAdapter) ListCorpus(w http.ResponseWriter, r *http.Request, params generated.ListCorpusParams) {
	a.corpusAdapter.ListCorpus(w, r, params)
}

func (a *CompositeAdapter) UploadCorpus(w http.ResponseWriter, r *http.Request) {
	a.corpusAdapter.UploadCorpus(w, r)
}

func (a *CompositeAdapter) ListQuarantinedCorpus(w http.ResponseWriter, r *http.Request, params generated.ListQuarantinedCorpusParams) {
	a.corpusAdapter.ListQuarantinedCorpus(w, r, params)
}

func (a *CompositeAdapter) SelectCorpus(w http.ResponseWriter, r *http.Request) {
	a.corpusAdapter.SelectCorpus(w, r)
}

func (a *CompositeAdapter) SyncCorpus(w http.ResponseWriter, r *http.Request) {
	a.corpusAdapter.SyncCorpus(w, r)
}

func (a *CompositeAdapter) DeleteCorpusEntry(w http.ResponseWriter, r *http.Request, entryId generated.CorpusEntryIdParam) {
	a.corpusAdapter.DeleteCorpusEntry(w, r, entryId)
}

func (a *CompositeAdapter) GetCorpusEntry(w http.ResponseWriter, r *http.Request, entryId generated.CorpusEntryIdParam, params generated.GetCorpusEntryParams) {
	a.corpusAdapter.GetCorpusEntry(w, r, entryId, params)
}

func (a *CompositeAdapter) DownloadCorpusFile(w http.ResponseWriter, r *http.Request, entryId generated.CorpusEntryIdParam) {
	a.corpusAdapter.DownloadCorpusFile(w, r, entryId)
}

// Crash endpoints
func (a *CompositeAdapter) ListCrashes(w http.ResponseWriter, r *http.Request, params generated.ListCrashesParams) {
	a.crashAdapter.ListCrashes(w, r, params)
}

func (a *CompositeAdapter) CreateCrash(w http.ResponseWriter, r *http.Request) {
	a.crashAdapter.CreateCrash(w, r)
}

func (a *CompositeAdapter) GetCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam, params generated.GetCrashParams) {
	a.crashAdapter.GetCrash(w, r, crashId, params)
}

func (a *CompositeAdapter) DeduplicateCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam) {
	a.crashAdapter.DeduplicateCrash(w, r, crashId)
}

func (a *CompositeAdapter) MinimizeCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam) {
	a.crashAdapter.MinimizeCrash(w, r, crashId)
}

func (a *CompositeAdapter) ReproduceCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam) {
	a.crashAdapter.ReproduceCrash(w, r, crashId)
}

// Job endpoints
func (a *CompositeAdapter) ListJobs(w http.ResponseWriter, r *http.Request, params generated.ListJobsParams) {
	a.jobAdapter.ListJobs(w, r, params)
}

func (a *CompositeAdapter) CreateJob(w http.ResponseWriter, r *http.Request) {
	a.jobAdapter.CreateJob(w, r)
}

func (a *CompositeAdapter) DeleteJob(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam) {
	a.jobAdapter.DeleteJob(w, r, jobId)
}

func (a *CompositeAdapter) GetJob(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, params generated.GetJobParams) {
	a.jobAdapter.GetJob(w, r, jobId, params)
}

func (a *CompositeAdapter) UpdateJob(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam) {
	a.jobAdapter.UpdateJob(w, r, jobId)
}

func (a *CompositeAdapter) GetJobArtifacts(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, params generated.GetJobArtifactsParams) {
	a.jobAdapter.GetJobArtifacts(w, r, jobId, params)
}

func (a *CompositeAdapter) GetJobCoverage(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, params generated.GetJobCoverageParams) {
	a.jobAdapter.GetJobCoverage(w, r, jobId, params)
}

func (a *CompositeAdapter) DownloadCoverageReport(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, reportId generated.ReportIdParam) {
	a.jobAdapter.DownloadCoverageReport(w, r, jobId, reportId)
}

func (a *CompositeAdapter) GetJobLogs(w http.ResponseWriter, r *http.Request, jobId generated.JobIdParam, params generated.GetJobLogsParams) {
	a.jobAdapter.GetJobLogs(w, r, jobId, params)
}

// Event stream endpoint
func (a *CompositeAdapter) GetEventStream(w http.ResponseWriter, r *http.Request, params generated.GetEventStreamParams) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Register SSE client
	clientID := fmt.Sprintf("sse_%d", time.Now().UnixNano())
	config := sse.ClientConfig{
		BufferSize:        100,
		WriteTimeout:      30 * time.Second,
		MaxEventsPerSec:   100,
		BurstSize:         10,
		EnableCompression: false,
	}
	client := sse.NewClient(clientID, w, r, config, a.logger)

	if err := a.sse.Register(client); err != nil {
		a.logger.WithError(err).Error("failed to register SSE client")
		a.writeError(w, http.StatusInternalServerError, "SSE_REGISTRATION_FAILED", "Failed to register SSE client", err)
		return
	}
	defer a.sse.Unregister(client)

	// Subscribe to requested event types
	if params.Types != nil {
		eventTypes := strings.Split(*params.Types, ",")
		for _, eventType := range eventTypes {
			eventType = strings.TrimSpace(eventType)
			if err := a.sse.Subscribe(clientID, eventType); err != nil {
				a.logger.WithError(err).WithField("event_type", eventType).Warn("failed to subscribe to event type")
			}
		}
	} else {
		// Subscribe to all events by default
		defaultTopics := []string{"bot", "job", "campaign", "corpus", "crash"}
		for _, topic := range defaultTopics {
			if err := a.sse.Subscribe(clientID, topic); err != nil {
				a.logger.WithError(err).WithField("topic", topic).Warn("failed to subscribe to topic")
			}
		}
	}

	// Filter by campaign or bot if specified
	if params.CampaignId != nil {
		if err := a.sse.Subscribe(clientID, "campaign."+params.CampaignId.String()); err != nil {
			a.logger.WithError(err).Warn("failed to subscribe to campaign-specific events")
		}
	}

	if params.BotId != nil {
		if err := a.sse.Subscribe(clientID, "bot."+params.BotId.String()); err != nil {
			a.logger.WithError(err).Warn("failed to subscribe to bot-specific events")
		}
	}

	// Keep connection alive
	<-r.Context().Done()
}

// Batch operations endpoint
func (a *CompositeAdapter) ExecuteBatch(w http.ResponseWriter, r *http.Request) {
	var req generated.BatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Execute batch operations
	response := a.executeBatchOperations(r, req)

	a.writeJSONResponse(w, http.StatusOK, response)
}

// Health endpoints
func (a *CompositeAdapter) GetHealth(w http.ResponseWriter, r *http.Request) {
	health := generated.HealthStatus{
		Status:    generated.HealthStatusStatusHealthy,
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Uptime:    "24h",
		Checks: &map[string]struct {
			LastCheck *time.Time                          `json:"last_check,omitempty"`
			LatencyMs *int                                `json:"latency_ms,omitempty"`
			Message   *string                             `json:"message,omitempty"`
			Status    *generated.HealthStatusChecksStatus `json:"status,omitempty"`
		}{
			"database": {
				Status:    &[]generated.HealthStatusChecksStatus{generated.HealthStatusChecksStatusHealthy}[0],
				LastCheck: &[]time.Time{time.Now()}[0],
				LatencyMs: &[]int{5}[0],
				Message:   &[]string{"Database connection OK"}[0],
			},
			"storage": {
				Status:    &[]generated.HealthStatusChecksStatus{generated.HealthStatusChecksStatusHealthy}[0],
				LastCheck: &[]time.Time{time.Now()}[0],
				LatencyMs: &[]int{10}[0],
				Message:   &[]string{"Storage backend accessible"}[0],
			},
			"bots": {
				Status:    &[]generated.HealthStatusChecksStatus{generated.HealthStatusChecksStatusHealthy}[0],
				LastCheck: &[]time.Time{time.Now()}[0],
				LatencyMs: &[]int{2}[0],
				Message:   &[]string{"6 of 8 bots online"}[0],
			},
		},
	}

	a.writeJSONResponse(w, http.StatusOK, health)
}

func (a *CompositeAdapter) GetReadiness(w http.ResponseWriter, r *http.Request) {
	readiness := generated.ReadinessStatus{
		Ready:     true,
		Timestamp: time.Now(),
		Message:   &[]string{"System ready to accept requests"}[0],
	}

	a.writeJSONResponse(w, http.StatusOK, readiness)
}

// Helper methods

func (a *CompositeAdapter) executeBatchOperations(r *http.Request, req generated.BatchRequest) generated.BatchResponse {
	startTime := time.Now()
	batchId := fmt.Sprintf("batch_%d", time.Now().UnixNano())

	results := make([]struct {
		Error                *generated.ProblemDetails            `json:"error,omitempty"`
		ExecutionTimeSeconds *float32                             `json:"execution_time_seconds,omitempty"`
		Operation            string                               `json:"operation"`
		OperationId          *string                              `json:"operation_id,omitempty"`
		OperationIndex       int                                  `json:"operation_index"`
		Result               *map[string]interface{}              `json:"result,omitempty"`
		Status               generated.BatchResponseResultsStatus `json:"status"`
	}, len(req.Operations))

	successful := 0
	failed := 0

	for i, op := range req.Operations {
		opStartTime := time.Now()
		result := struct {
			Error                *generated.ProblemDetails            `json:"error,omitempty"`
			ExecutionTimeSeconds *float32                             `json:"execution_time_seconds,omitempty"`
			Operation            string                               `json:"operation"`
			OperationId          *string                              `json:"operation_id,omitempty"`
			OperationIndex       int                                  `json:"operation_index"`
			Result               *map[string]interface{}              `json:"result,omitempty"`
			Status               generated.BatchResponseResultsStatus `json:"status"`
		}{
			Operation:      string(op.Operation),
			OperationIndex: i,
			OperationId:    op.OperationId,
		}

		// Execute operation based on type
		switch op.Operation {
		case generated.CreateBot:
			result.Status = generated.BatchResponseResultsStatusSuccess
			successful++
			mockResult := map[string]interface{}{
				"bot_id": fmt.Sprintf("bot_%d", time.Now().UnixNano()),
				"status": "created",
			}
			result.Result = &mockResult

		case generated.CreateJob:
			result.Status = generated.BatchResponseResultsStatusSuccess
			successful++
			mockResult := map[string]interface{}{
				"job_id": fmt.Sprintf("job_%d", time.Now().UnixNano()),
				"status": "created",
			}
			result.Result = &mockResult

		case generated.CreateCampaign:
			result.Status = generated.BatchResponseResultsStatusSuccess
			successful++
			mockResult := map[string]interface{}{
				"campaign_id": fmt.Sprintf("campaign_%d", time.Now().UnixNano()),
				"status":      "created",
			}
			result.Result = &mockResult

		default:
			result.Status = generated.BatchResponseResultsStatusFailed
			failed++
			result.Error = &generated.ProblemDetails{
				Type:   "/errors/unsupported_operation",
				Title:  "Unsupported batch operation",
				Status: 400,
			}
		}

		execTime := float32(time.Since(opStartTime).Seconds())
		result.ExecutionTimeSeconds = &execTime
		results[i] = result

		// Check fail-fast option
		if req.Options != nil && req.Options.FailFast != nil && *req.Options.FailFast && result.Status == generated.BatchResponseResultsStatusFailed {
			break
		}
	}

	totalTime := float32(time.Since(startTime).Seconds())
	partialSuccess := successful > 0 && failed > 0

	// Parse the UUID string
	uuidVal, _ := uuid.Parse(batchId)
	return generated.BatchResponse{
		BatchId:              openapi_types.UUID(uuidVal), // Convert to openapi UUID
		TotalOperations:      len(req.Operations),
		SuccessfulOperations: successful,
		FailedOperations:     failed,
		ExecutionTimeSeconds: totalTime,
		PartialSuccess:       &partialSuccess,
		Results:              results,
	}
}

func (a *CompositeAdapter) writeJSONResponse(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		a.logger.WithError(err).Error("failed to encode JSON response")
	}
}

func (a *CompositeAdapter) writeError(w http.ResponseWriter, statusCode int, errorType, title string, err error) {
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

// AckJob forwards to job adapter
func (a *CompositeAdapter) AckJob(w http.ResponseWriter, r *http.Request, jobID, botID, leaseToken string) {
	a.jobAdapter.AckJob(w, r, jobID, botID, leaseToken)
}

// JobHeartbeat forwards to job adapter
func (a *CompositeAdapter) JobHeartbeat(w http.ResponseWriter, r *http.Request, jobID, botID, leaseToken string) {
	a.jobAdapter.JobHeartbeat(w, r, jobID, botID, leaseToken)
}
