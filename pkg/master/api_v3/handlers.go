package api_v3

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/monitoring/health"
	"github.com/ethpandaops/pandafuzz/pkg/master/repository"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
	"github.com/ethpandaops/pandafuzz/pkg/storage/backend"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// HandlerV3 implements the v3 API handlers
type HandlerV3 struct {
	services       *service.Manager
	campaign       *CampaignServiceAdapter
	corpus         *CorpusServiceAdapter
	coverageRepo   repository.CoverageRepository
	storageBackend backend.StorageBackend
	validator      *Validator
	logger         logrus.FieldLogger
	config         *Config
	db             interface{}                    // Database connection (SQLite)
	version        *common.VersionInfo            // Build version information
	healthChecker  *health.DataConsistencyChecker // Health checker for detailed checks
}

// Config holds API v3 configuration
type Config struct {
	MaxRequestSize  int64
	RequestTimeout  time.Duration
	MaxBatchSize    int
	EnableSwaggerUI bool
}

// NewHandlerV3 creates a new v3 API handler
func NewHandlerV3(services *service.Manager, coverageRepo repository.CoverageRepository, storageBackend backend.StorageBackend, db interface{}, logger logrus.FieldLogger, config *Config, version *common.VersionInfo) *HandlerV3 {
	var campaign *CampaignServiceAdapter
	var corpus *CorpusServiceAdapter

	if services != nil {
		campaign = NewCampaignServiceAdapter(services.Campaign)
		corpus = NewCorpusServiceAdapter(services.Corpus)
	}

	// Create health checker for detailed checks
	// Note: This uses a placeholder interface since we need to define the actual service interface
	var healthChecker *health.DataConsistencyChecker
	if services != nil {
		// In a real implementation, we'd pass the actual service manager
		// For now, we'll create it without services to avoid compilation issues
		healthChecker = health.NewDataConsistencyChecker(nil, logger.WithField("component", "health_checker"))
	}

	return &HandlerV3{
		services:       services,
		campaign:       campaign,
		corpus:         corpus,
		coverageRepo:   coverageRepo,
		storageBackend: storageBackend,
		validator:      NewValidator(),
		logger:         logger.WithField("api_version", "v3"),
		config:         config,
		db:             db,
		version:        version,
		healthChecker:  healthChecker,
	}
}

// RegisterRoutes registers all v3 API routes
func (h *HandlerV3) RegisterRoutes(router *mux.Router) {
	// Apply versioning middleware
	// Note: Don't create /api/v3 prefix here as Integration already does it
	router.Use(h.versioningMiddleware)
	router.Use(h.requestValidationMiddleware)
	router.Use(h.loggingMiddleware)

	// Bot management
	router.HandleFunc("/bots", h.listBots).Methods("GET")
	router.HandleFunc("/bots", h.registerBot).Methods("POST")
	router.HandleFunc("/bots/{botId}", h.getBot).Methods("GET")
	router.HandleFunc("/bots/{botId}", h.deregisterBot).Methods("DELETE")
	router.HandleFunc("/bots/{botId}/heartbeat", h.botHeartbeat).Methods("POST")
	router.HandleFunc("/bots/{botId}/jobs/next", h.getNextJob).Methods("POST")
	router.HandleFunc("/bots/{botId}/jobs/complete", h.completeJob).Methods("POST")
	router.HandleFunc("/bots/{botId}/metrics", h.getBotMetrics).Methods("GET")

	// Job management
	router.HandleFunc("/jobs", h.listJobs).Methods("GET")
	router.HandleFunc("/jobs", h.createJob).Methods("POST")
	router.HandleFunc("/jobs/{jobId}", h.getJob).Methods("GET")
	router.HandleFunc("/jobs/{jobId}", h.cancelJob).Methods("DELETE")
	router.HandleFunc("/jobs/{jobId}/logs", h.getJobLogs).Methods("GET")
	router.HandleFunc("/jobs/{jobId}/progress", h.getJobProgress).Methods("GET")
	router.HandleFunc("/jobs/{jobId}/crashes", h.getJobCrashes).Methods("GET")

	// Coverage management
	router.HandleFunc("/jobs/{jobId}/coverage", h.listJobCoverage).Methods("GET")
	router.HandleFunc("/jobs/{jobId}/coverage/{reportId}", h.getJobCoverageReport).Methods("GET")
	router.HandleFunc("/jobs/{jobId}/coverage/{reportId}/metadata", h.getJobCoverageMetadata).Methods("GET")

	// Campaign management
	router.HandleFunc("/campaigns", h.listCampaigns).Methods("GET")
	router.HandleFunc("/campaigns", h.createCampaign).Methods("POST")
	router.HandleFunc("/campaigns/{campaignId}", h.getCampaign).Methods("GET")
	router.HandleFunc("/campaigns/{campaignId}", h.updateCampaign).Methods("PATCH")
	router.HandleFunc("/campaigns/{campaignId}", h.deleteCampaign).Methods("DELETE")
	router.HandleFunc("/campaigns/{campaignId}/stats", h.getCampaignStats).Methods("GET")

	// Corpus management
	router.HandleFunc("/corpus", h.listCorpus).Methods("GET")
	router.HandleFunc("/corpus", h.uploadCorpus).Methods("POST")
	router.HandleFunc("/corpus/{corpusId}", h.getCorpusFile).Methods("GET")
	router.HandleFunc("/corpus/{corpusId}", h.deleteCorpusFile).Methods("DELETE")
	router.HandleFunc("/corpus/{corpusId}/download", h.downloadCorpusFile).Methods("GET")
	router.HandleFunc("/corpus/sync", h.syncCorpus).Methods("POST")
	router.HandleFunc("/corpus/promote", h.promoteCrashToCorpus).Methods("POST")

	// Crash management
	router.HandleFunc("/crashes", h.listCrashes).Methods("GET")
	router.HandleFunc("/crashes/{crashId}", h.getCrash).Methods("GET")
	router.HandleFunc("/crashes/{crashId}/input", h.getCrashInput).Methods("GET")

	// Reproducibility
	router.HandleFunc("/reproducibility/requests", h.listReproductionRequests).Methods("GET")
	router.HandleFunc("/reproducibility/requests", h.createReproductionRequest).Methods("POST")
	router.HandleFunc("/reproducibility/requests/{requestId}", h.getReproductionRequest).Methods("GET")
	router.HandleFunc("/reproducibility/results", h.submitReproductionResult).Methods("POST")

	// Result submission
	router.HandleFunc("/results/batch", h.submitBatchResults).Methods("POST")
	router.HandleFunc("/results/crash", h.submitCrashResult).Methods("POST")
	router.HandleFunc("/results/coverage", h.submitCoverageResult).Methods("POST")
	router.HandleFunc("/results/corpus", h.submitCorpusUpdate).Methods("POST")

	// System management
	router.HandleFunc("/system/stats", h.getSystemStats).Methods("GET")
	router.HandleFunc("/system/health", h.healthCheck).Methods("GET")
	router.HandleFunc("/system/recovery", h.triggerRecovery).Methods("POST")
	router.HandleFunc("/system/maintenance", h.triggerMaintenance).Methods("POST")
	router.HandleFunc("/system/timeouts", h.listTimeouts).Methods("GET")
	router.HandleFunc("/system/timeouts/{type}/{id}", h.forceTimeout).Methods("POST")

	// Health endpoints
	router.HandleFunc("/health/detailed", h.detailedHealthCheck).Methods("GET")

	// Version information
	router.HandleFunc("/version", h.getVersion).Methods("GET")

	// Swagger UI (if enabled)
	if h.config.EnableSwaggerUI {
		router.HandleFunc("/docs", h.swaggerUI).Methods("GET")
		router.HandleFunc("/openapi.yaml", h.openAPISpec).Methods("GET")
	}
}

// Middleware functions

func (h *HandlerV3) versioningMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("API-Version", "3.0.0")
		w.Header().Set("X-RateLimit-Limit", "1000")
		w.Header().Set("X-RateLimit-Remaining", "999")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
		next.ServeHTTP(w, r)
	})
}

func (h *HandlerV3) requestValidationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate request size
		r.Body = http.MaxBytesReader(w, r.Body, h.config.MaxRequestSize)

		// Add request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}
		ctx := context.WithValue(r.Context(), "request_id", requestID)
		r = r.WithContext(ctx)

		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r)
	})
}

func (h *HandlerV3) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		h.logger.WithFields(logrus.Fields{
			"method":     r.Method,
			"path":       r.URL.Path,
			"request_id": r.Context().Value("request_id"),
		}).Debug("API request started")

		next.ServeHTTP(wrapped, r)

		h.logger.WithFields(logrus.Fields{
			"method":     r.Method,
			"path":       r.URL.Path,
			"status":     wrapped.statusCode,
			"duration":   time.Since(start),
			"request_id": r.Context().Value("request_id"),
		}).Info("API request completed")
	})
}

// Bot management handlers

func (h *HandlerV3) listBots(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	params := parsePaginationParams(r)
	statusFilterStr := r.URL.Query().Get("status")
	var statusFilter *common.BotStatus
	if statusFilterStr != "" {
		Status := common.BotStatus(statusFilterStr)
		statusFilter = &Status
	}

	// Get bots from service
	bots, err := h.services.Bot.ListBots(r.Context(), statusFilter)
	if err != nil {
		h.writeError(w, err)
		return
	}

	// Apply filters
	statusStr := ""
	if statusFilter != nil {
		statusStr = string(*statusFilter)
	}
	filtered := filterBotsByStatus(bots, statusStr)

	// Apply pagination
	paginated := paginateBots(filtered, params.Page, params.Limit)

	// Write response
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"bots":  paginated,
		"count": len(paginated),
	})
}

func (h *HandlerV3) registerBot(w http.ResponseWriter, r *http.Request) {
	var req BotRegisterRequest
	if err := h.decodeAndValidate(w, r, &req); err != nil {
		return // Error already written
	}

	// Use service to register bot
	bot, err := h.services.Bot.RegisterBot(r.Context(), req.Hostname, req.Name, req.Capabilities, req.APIEndpoint)
	if err != nil {
		h.writeError(w, err)
		return
	}

	// Write response
	resp := BotRegisterResponse{
		BotID:     bot.ID,
		Status:    "registered",
		Timestamp: bot.RegisteredAt,
		Timeout:   bot.TimeoutAt,
	}

	h.writeJSON(w, http.StatusCreated, resp)
}

func (h *HandlerV3) getBot(w http.ResponseWriter, r *http.Request) {
	botID := mux.Vars(r)["botId"]

	bot, err := h.services.Bot.GetBot(r.Context(), botID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, bot)
}

func (h *HandlerV3) deregisterBot(w http.ResponseWriter, r *http.Request) {
	botID := mux.Vars(r)["botId"]

	if err := h.services.Bot.DeregisterBot(r.Context(), botID); err != nil {
		h.writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *HandlerV3) botHeartbeat(w http.ResponseWriter, r *http.Request) {
	botID := mux.Vars(r)["botId"]

	var req BotHeartbeatRequest
	if err := h.decodeAndValidate(w, r, &req); err != nil {
		return
	}

	// Update heartbeat with status and current job
	err := h.services.Bot.UpdateHeartbeat(r.Context(), botID, req.Status, req.CurrentJob)
	if err != nil {
		h.writeError(w, err)
		return
	}

	// Calculate timeout
	timeout := time.Now().Add(60 * time.Second) // Default 60s timeout

	resp := BotHeartbeatResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Timeout:   timeout,
	}

	h.writeJSON(w, http.StatusOK, resp)
}

func (h *HandlerV3) getNextJob(w http.ResponseWriter, r *http.Request) {
	botID := mux.Vars(r)["botId"]

	job, err := h.services.Job.AssignNextJob(r.Context(), botID)
	if err != nil {
		if err == service.ErrNoJobsAvailable {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, job)
}

func (h *HandlerV3) completeJob(w http.ResponseWriter, r *http.Request) {
	botID := mux.Vars(r)["botId"]

	var req JobCompleteRequest
	if err := h.decodeAndValidate(w, r, &req); err != nil {
		return
	}

	job, err := h.services.Bot.GetCurrentJob(r.Context(), botID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	if job == nil {
		h.writeError(w, &ValidationError{
			Field:   "bot",
			Message: "bot has no active job",
		})
		return
	}

	if err := h.services.Job.CompleteJob(r.Context(), job.ID, botID, req.Success); err != nil {
		h.writeError(w, err)
		return
	}

	resp := JobCompleteResponse{
		Acknowledged: true,
		JobID:        job.ID,
		Message:      "Job completion successfully recorded",
		Status:       "completed",
		Timestamp:    time.Now(),
	}

	h.writeJSON(w, http.StatusOK, resp)
}

func (h *HandlerV3) getBotMetrics(w http.ResponseWriter, r *http.Request) {
	botID := mux.Vars(r)["botId"]

	// Get bot details
	bot, err := h.services.Bot.GetBot(r.Context(), botID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	// Get metrics
	metrics, err := h.services.Bot.GetMetrics(r.Context(), botID)
	if err != nil {
		// Return placeholder metrics if not available
		metrics = &service.BotMetrics{
			BotID:            botID,
			TotalJobsRun:     0,
			SuccessfulJobs:   0,
			FailedJobs:       0,
			CrashesFound:     0,
			UniqueCrashes:    0,
			CorpusItemsAdded: 0,
			CPUTime:          0.0,
			LastActive:       bot.LastSeen,
		}
	}

	h.writeJSON(w, http.StatusOK, metrics)
}

// Job management handlers

func (h *HandlerV3) listJobs(w http.ResponseWriter, r *http.Request) {
	// Parse filters
	params := parsePaginationParams(r)
	filters := parseJobFilters(r)

	// Get jobs from service
	jobs, err := h.services.Job.ListJobs(r.Context(), *filters)
	if err != nil {
		h.writeError(w, err)
		return
	}

	// Apply sorting
	sortJobs(jobs, params.SortBy, params.SortOrder)

	// Apply pagination
	total := len(jobs)
	paginated := paginateJobs(jobs, params.Page, params.Limit)

	// Write response
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"jobs":       paginated,
		"count":      len(paginated),
		"page":       params.Page,
		"limit":      params.Limit,
		"total":      total,
		"sort_by":    params.SortBy,
		"sort_order": params.SortOrder,
	})
}

func (h *HandlerV3) createJob(w http.ResponseWriter, r *http.Request) {
	var req JobRequest
	if err := h.decodeAndValidate(w, r, &req); err != nil {
		return
	}

	// Handle timeout_sec field for backward compatibility
	duration := req.Duration
	if duration == 0 && req.TimeoutSec > 0 {
		duration = time.Duration(req.TimeoutSec) * time.Second
	}

	// Convert to service request
	createReq := service.CreateJobRequest{
		Name:              req.Name,
		Target:            req.Target,
		Fuzzer:            req.Fuzzer,
		Duration:          duration,
		Config:            req.Config,
		CampaignID:        req.CampaignID,
		CorpusID:          req.CorpusID,
		CollectionID:      req.CollectionID,
		UseCampaignCorpus: req.UseCampaignCorpus,
		EnableCoverage:    req.EnableCoverage,
		CoverageFormat:    req.CoverageFormat,
	}

	job, err := h.services.Job.CreateJob(r.Context(), createReq)
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, job)
}

func (h *HandlerV3) getJob(w http.ResponseWriter, r *http.Request) {
	jobID := mux.Vars(r)["jobId"]

	job, err := h.services.Job.GetJob(r.Context(), jobID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, job)
}

func (h *HandlerV3) cancelJob(w http.ResponseWriter, r *http.Request) {
	jobID := mux.Vars(r)["jobId"]

	if err := h.services.Job.CancelJob(r.Context(), jobID); err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "cancelled",
		"timestamp": time.Now(),
	})
}

func (h *HandlerV3) getJobLogs(w http.ResponseWriter, r *http.Request) {
	jobID := mux.Vars(r)["jobId"]
	follow := r.URL.Query().Get("follow") == "true"
	lines := 1000
	if l := r.URL.Query().Get("lines"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 10000 {
			lines = parsed
		}
	}

	if follow {
		// Set up Server-Sent Events
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			h.writeError(w, &APIError{
				Code:    "streaming_not_supported",
				Message: "Streaming not supported",
			})
			return
		}

		// Stream logs
		logChan, err := h.services.Job.StreamLogs(r.Context(), jobID)
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
			flusher.Flush()
			return
		}
		for {
			select {
			case log, ok := <-logChan:
				if !ok {
					return
				}
				data, _ := json.Marshal(log)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	} else {
		// Return static logs
		logs, err := h.services.Job.GetLogs(r.Context(), jobID)
		// Apply lines limit if requested
		if lines > 0 && len(logs) > lines {
			logs = logs[len(logs)-lines:]
		}
		if err != nil {
			h.writeError(w, err)
			return
		}

		h.writeJSON(w, http.StatusOK, map[string]interface{}{
			"job_id":    jobID,
			"logs":      logs,
			"timestamp": time.Now(),
		})
	}
}

func (h *HandlerV3) getJobProgress(w http.ResponseWriter, r *http.Request) {
	jobID := mux.Vars(r)["jobId"]

	// Set up Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeError(w, &APIError{
			Code:    "streaming_not_supported",
			Message: "Streaming not supported",
		})
		return
	}

	// Send initial job details
	job, err := h.services.Job.GetJob(r.Context(), jobID)
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: {\"error\": \"%s\"}\n\n", err.Error())
		flusher.Flush()
		return
	}

	initialData := map[string]interface{}{
		"job_id":   job.ID,
		"name":     job.Name,
		"status":   job.Status,
		"fuzzer":   job.Fuzzer,
		"progress": job.Progress,
	}
	data, _ := json.Marshal(initialData)
	fmt.Fprintf(w, "event: connected\ndata: %s\n\n", data)
	flusher.Flush()

	// Stream progress updates
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Get updated job status
			job, err := h.services.Job.GetJob(r.Context(), jobID)
			if err != nil {
				fmt.Fprintf(w, "event: error\ndata: {\"error\": \"Failed to get job status\"}\n\n")
				flusher.Flush()
				return
			}

			// Get statistics
			stats, _ := h.services.Job.GetJobStats(r.Context(), jobID)

			progressData := map[string]interface{}{
				"job_id":         job.ID,
				"status":         job.Status,
				"progress":       job.Progress,
				"crash_count":    stats.CrashesFound,
				"coverage_edges": int64(stats.CoveragePercent * 1000), // Convert percentage to edges approximation
				"timestamp":      time.Now(),
			}

			if job.AssignedBot != nil {
				progressData["assigned_bot"] = *job.AssignedBot
			}

			data, _ := json.Marshal(progressData)
			fmt.Fprintf(w, "event: progress\ndata: %s\n\n", data)
			flusher.Flush()

			// Check if job completed
			if job.Status == common.JobStatusCompleted ||
				job.Status == common.JobStatusFailed ||
				job.Status == common.JobStatusCancelled ||
				job.Status == common.JobStatusTimedOut {

				finalData := map[string]interface{}{
					"job_id":    job.ID,
					"status":    job.Status,
					"completed": true,
					"timestamp": time.Now(),
				}

				data, _ := json.Marshal(finalData)
				fmt.Fprintf(w, "event: completed\ndata: %s\n\n", data)
				flusher.Flush()
				return
			}

		case <-r.Context().Done():
			return
		}
	}
}

func (h *HandlerV3) getJobCrashes(w http.ResponseWriter, r *http.Request) {
	jobID := mux.Vars(r)["jobId"]
	params := parsePaginationParams(r)

	crashes, err := h.services.Job.GetJobCrashes(r.Context(), jobID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	// Apply pagination
	paginated := paginateCrashes(crashes, params.Page, params.Limit)

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"crashes": paginated,
		"count":   len(paginated),
	})
}

// Campaign management handlers

func (h *HandlerV3) listCampaigns(w http.ResponseWriter, r *http.Request) {
	params := parsePaginationParams(r)
	filters := parseCampaignFilters(r)

	campaigns, err := h.campaign.ListCampaigns(r.Context(), *filters)
	if err != nil {
		h.writeError(w, err)
		return
	}

	// Apply pagination
	paginated := paginateCampaigns(campaigns, params.Page, params.Limit)

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"campaigns": paginated,
		"count":     len(paginated),
	})
}

func (h *HandlerV3) createCampaign(w http.ResponseWriter, r *http.Request) {
	var req CampaignRequest
	if err := h.decodeAndValidate(w, r, &req); err != nil {
		return
	}

	campaign, err := h.campaign.CreateCampaign(r.Context(), &common.Campaign{
		Name:         req.Name,
		Description:  req.Description,
		TargetBinary: req.TargetBinary,
		AutoRestart:  req.AutoRestart,
		MaxDuration:  req.MaxDuration,
		MaxJobs:      req.MaxJobs,
		JobTemplate:  req.JobTemplate,
		SharedCorpus: req.SharedCorpus,
		Tags:         req.Tags,
	})
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, campaign)
}

func (h *HandlerV3) getCampaign(w http.ResponseWriter, r *http.Request) {
	campaignID := mux.Vars(r)["campaignId"]

	campaign, err := h.campaign.GetCampaign(r.Context(), campaignID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, campaign)
}

func (h *HandlerV3) updateCampaign(w http.ResponseWriter, r *http.Request) {
	campaignID := mux.Vars(r)["campaignId"]

	var updates common.CampaignUpdates
	if err := h.decodeAndValidate(w, r, &updates); err != nil {
		return
	}

	campaign, err := h.campaign.UpdateCampaign(r.Context(), campaignID, &updates)
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, campaign)
}

func (h *HandlerV3) deleteCampaign(w http.ResponseWriter, r *http.Request) {
	campaignID := mux.Vars(r)["campaignId"]

	if err := h.campaign.DeleteCampaign(r.Context(), campaignID); err != nil {
		h.writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *HandlerV3) getCampaignStats(w http.ResponseWriter, r *http.Request) {
	campaignID := mux.Vars(r)["campaignId"]

	stats, err := h.campaign.GetCampaignStats(r.Context(), campaignID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, stats)
}

// Corpus management handlers

func (h *HandlerV3) listCorpus(w http.ResponseWriter, r *http.Request) {
	params := parsePaginationParams(r)
	campaignID := r.URL.Query().Get("campaignId")
	jobID := r.URL.Query().Get("jobId")

	// Get corpus files based on campaign or job ID
	var files []*common.CorpusFile
	var err error

	if jobID != "" {
		files, err = h.services.Corpus.GetCorpusForJob(r.Context(), jobID)
	} else if campaignID != "" {
		files, err = h.corpus.ListCorpusFiles(r.Context(), campaignID)
	} else {
		files = []*common.CorpusFile{}
	}
	if err != nil {
		h.writeError(w, err)
		return
	}

	// Apply pagination
	paginated := paginateCorpusFiles(files, params.Page, params.Limit)

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"files": paginated,
		"count": len(paginated),
	})
}

func (h *HandlerV3) uploadCorpus(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32MB max
		h.writeError(w, &ValidationError{
			Field:   "form",
			Message: "failed to parse multipart form",
		})
		return
	}

	campaignID := r.FormValue("campaignId")
	if campaignID == "" {
		h.writeError(w, &ValidationError{
			Field:   "campaignId",
			Message: "campaign ID is required",
		})
		return
	}

	jobID := r.FormValue("jobId")

	// Process uploaded files
	uploaded := 0
	duplicates := 0
	errors := []string{}

	files := r.MultipartForm.File["files"]
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: failed to open", fileHeader.Filename))
			continue
		}
		defer file.Close()

		// Read file content
		content := make([]byte, fileHeader.Size)
		_, err = file.Read(content)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: failed to read", fileHeader.Filename))
			continue
		}

		// Create corpus file
		corpusFile := &common.CorpusFile{
			ID:         uuid.New().String(),
			CampaignID: campaignID,
			JobID:      jobID,
			Filename:   fileHeader.Filename,
			Size:       int64(len(content)),
			Hash:       fmt.Sprintf("%x", content), // Simple hash for now
			// Content is not a field - need to store separately
			CreatedAt: time.Now(),
		}

		// Upload file
		if _, err := h.corpus.UploadCorpusFile(r.Context(), corpusFile); err != nil {
			if strings.Contains(err.Error(), "duplicate") {
				duplicates++
			} else {
				errors = append(errors, fmt.Sprintf("%s: %v", fileHeader.Filename, err))
			}
		} else {
			uploaded++
		}
	}

	h.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"uploaded":   uploaded,
		"duplicates": duplicates,
		"errors":     errors,
	})
}

func (h *HandlerV3) getCorpusFile(w http.ResponseWriter, r *http.Request) {
	corpusID := mux.Vars(r)["corpusId"]

	file, err := h.corpus.GetCorpusFile(r.Context(), corpusID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, file)
}

func (h *HandlerV3) deleteCorpusFile(w http.ResponseWriter, r *http.Request) {
	corpusID := mux.Vars(r)["corpusId"]

	if err := h.corpus.DeleteCorpusFile(r.Context(), corpusID); err != nil {
		h.writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *HandlerV3) downloadCorpusFile(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement corpus file download
	h.writeError(w, common.ErrNotImplemented)
}

func (h *HandlerV3) syncCorpus(w http.ResponseWriter, r *http.Request) {
	var req CorpusSyncRequest
	if err := h.decodeAndValidate(w, r, &req); err != nil {
		return
	}

	// Call the actual SyncCorpus method with correct parameters
	files, err := h.services.Corpus.SyncCorpus(r.Context(), req.SourceCampaignID, req.TargetCampaignID)
	result := map[string]interface{}{
		"synced_files": files,
		"count":        len(files),
	}
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

func (h *HandlerV3) promoteCrashToCorpus(w http.ResponseWriter, r *http.Request) {
	var req CorpusPromotionRequest
	if err := h.decodeAndValidate(w, r, &req); err != nil {
		return
	}

	// Call the actual PromoteCrashToCorpus method with correct parameters
	file, err := h.services.Corpus.PromoteCrashToCorpus(r.Context(), req.CrashID, req.CampaignID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, file)
}

// Crash management handlers

func (h *HandlerV3) listCrashes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	params := parsePaginationParams(r)
	filters := parseCrashFilters(r)

	// Build query for crash listing
	query := `
		SELECT id, job_id, bot_id, campaign_id, hash, file_path, type, signal, exit_code, timestamp,
		       stack_trace, reproducible, minimized, metadata
		FROM crashes
		WHERE 1=1
	`
	args := []interface{}{}
	argIndex := 1

	// Apply filters
	if filters.CampaignID != "" {
		query += fmt.Sprintf(" AND campaign_id = $%d", argIndex)
		args = append(args, filters.CampaignID)
		argIndex++
	}

	if filters.JobID != "" {
		query += fmt.Sprintf(" AND job_id = $%d", argIndex)
		args = append(args, filters.JobID)
		argIndex++
	}

	if filters.Type != "" {
		query += fmt.Sprintf(" AND type = $%d", argIndex)
		args = append(args, filters.Type)
		argIndex++
	}

	if filters.Severity != "" {
		query += fmt.Sprintf(" AND severity = $%d", argIndex)
		args = append(args, filters.Severity)
		argIndex++
	}

	// Apply sorting
	if params.SortBy == "" {
		params.SortBy = "timestamp"
	}
	if params.SortOrder == "" {
		params.SortOrder = "desc"
	}

	validSortFields := map[string]bool{
		"timestamp": true,
		"type":      true,
		"severity":  true,
		"job_id":    true,
	}

	if validSortFields[params.SortBy] {
		query += fmt.Sprintf(" ORDER BY %s %s", params.SortBy, params.SortOrder)
	} else {
		query += " ORDER BY timestamp DESC"
	}

	// Apply pagination
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, params.Limit, params.Offset)

	// Execute query
	var crashes []*common.CrashResult
	if h.db != nil {
		db := h.db.(*sql.DB)
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			h.logger.WithError(err).Error("Failed to query crashes")
			h.writeError(w, errors.NewSystemError("list_crashes", err))
			return
		}
		defer rows.Close()

		crashes = make([]*common.CrashResult, 0)
		for rows.Next() {
			var crash common.CrashResult
			var stackTrace, metadata sql.NullString
			var reproducible, minimized sql.NullBool

			err := rows.Scan(
				&crash.ID, &crash.JobID, &crash.BotID, &crash.CampaignID,
				&crash.Hash, &crash.FilePath, &crash.Type, &crash.Signal,
				&crash.ExitCode, &crash.Timestamp, &stackTrace,
				&reproducible, &minimized, &metadata,
			)
			if err != nil {
				h.logger.WithError(err).Error("Failed to scan crash row")
				continue
			}

			if stackTrace.Valid {
				crash.StackTrace = stackTrace.String
			}
			if reproducible.Valid {
				crash.Reproducible = reproducible.Bool
			}
			if minimized.Valid {
				crash.Minimized = minimized.Bool
			}
			if metadata.Valid {
				if err := json.Unmarshal([]byte(metadata.String), &crash.Metadata); err != nil {
					crash.Metadata = make(map[string]interface{})
				}
			}

			crashes = append(crashes, &crash)
		}

		if err := rows.Err(); err != nil {
			h.logger.WithError(err).Error("Error iterating crash rows")
		}
	} else {
		// Fallback to storage backend if no database
		crashes = h.listCrashesFromStorage(ctx, filters, params)
	}

	// Get total count for pagination
	var totalCount int
	if h.db != nil {
		countQuery := "SELECT COUNT(*) FROM crashes WHERE 1=1"
		countArgs := []interface{}{}
		countArgIndex := 1

		if filters.CampaignID != "" {
			countQuery += fmt.Sprintf(" AND campaign_id = $%d", countArgIndex)
			countArgs = append(countArgs, filters.CampaignID)
			countArgIndex++
		}
		if filters.JobID != "" {
			countQuery += fmt.Sprintf(" AND job_id = $%d", countArgIndex)
			countArgs = append(countArgs, filters.JobID)
			countArgIndex++
		}
		if filters.Type != "" {
			countQuery += fmt.Sprintf(" AND type = $%d", countArgIndex)
			countArgs = append(countArgs, filters.Type)
			countArgIndex++
		}

		db := h.db.(*sql.DB)
		err := db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&totalCount)
		if err != nil {
			h.logger.WithError(err).Warn("Failed to get crash count")
			totalCount = len(crashes)
		}
	} else {
		totalCount = len(crashes)
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"crashes":     crashes,
		"total_count": totalCount,
		"count":       len(crashes),
		"limit":       params.Limit,
		"offset":      params.Offset,
		"sort_by":     params.SortBy,
		"sort_order":  params.SortOrder,
	})
}

func (h *HandlerV3) getCrash(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	crashID := mux.Vars(r)["crashId"]

	if crashID == "" {
		h.writeError(w, errors.NewValidationError("get_crash", "crash ID is required"))
		return
	}

	var crash *common.CrashResult

	if h.db != nil {
		// Query crash from database
		query := `
			SELECT id, job_id, bot_id, campaign_id, hash, file_path, type, signal, exit_code, timestamp,
			       stack_trace, reproducible, minimized, metadata
			FROM crashes
			WHERE id = $1
		`

		db := h.db.(*sql.DB)
		row := db.QueryRowContext(ctx, query, crashID)

		crash = &common.CrashResult{}
		var stackTrace, metadata sql.NullString
		var reproducible, minimized sql.NullBool

		err := row.Scan(
			&crash.ID, &crash.JobID, &crash.BotID, &crash.CampaignID,
			&crash.Hash, &crash.FilePath, &crash.Type, &crash.Signal,
			&crash.ExitCode, &crash.Timestamp, &stackTrace,
			&reproducible, &minimized, &metadata,
		)

		if err == sql.ErrNoRows {
			h.writeError(w, errors.NewNotFoundError("get_crash", "crash"))
			return
		}

		if err != nil {
			h.logger.WithError(err).Error("Failed to query crash")
			h.writeError(w, errors.NewSystemError("get_crash", err))
			return
		}

		if stackTrace.Valid {
			crash.StackTrace = stackTrace.String
		}
		if reproducible.Valid {
			crash.Reproducible = reproducible.Bool
		}
		if minimized.Valid {
			crash.Minimized = minimized.Bool
		}
		if metadata.Valid {
			if err := json.Unmarshal([]byte(metadata.String), &crash.Metadata); err != nil {
				crash.Metadata = make(map[string]interface{})
			}
		}
	} else {
		// Fallback to storage backend
		crash = h.getCrashFromStorage(ctx, crashID)
		if crash == nil {
			h.writeError(w, errors.NewNotFoundError("get_crash", "crash"))
			return
		}
	}

	h.writeJSON(w, http.StatusOK, crash)
}

func (h *HandlerV3) getCrashInput(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	crashID := mux.Vars(r)["crashId"]

	if crashID == "" {
		h.writeError(w, errors.NewValidationError("get_crash", "crash ID is required"))
		return
	}

	// First get crash details to find the file path
	var crashFilePath string
	var jobID string

	if h.db != nil {
		query := "SELECT file_path, job_id FROM crashes WHERE id = $1"
		db := h.db.(*sql.DB)
		err := db.QueryRowContext(ctx, query, crashID).Scan(&crashFilePath, &jobID)

		if err == sql.ErrNoRows {
			h.writeError(w, errors.NewNotFoundError("get_crash", "crash"))
			return
		}

		if err != nil {
			h.logger.WithError(err).Error("Failed to query crash details")
			h.writeError(w, errors.NewSystemError("get_crash_details", err))
			return
		}
	} else {
		// Fallback: try to get crash from storage
		crash := h.getCrashFromStorage(ctx, crashID)
		if crash == nil {
			h.writeError(w, errors.NewNotFoundError("get_crash", "crash"))
			return
		}
		crashFilePath = crash.FilePath
		jobID = crash.JobID
	}

	// Read crash input from storage
	var input []byte
	var err error

	if h.storageBackend != nil {
		// Construct the full path in storage
		// Typically crashes are stored as: crashes/{job_id}/{crash_hash}
		storagePath := filepath.Join("crashes", jobID, filepath.Base(crashFilePath))

		reader, err := h.storageBackend.Retrieve(ctx, storagePath)
		if err != nil {
			// Try alternative path structure
			storagePath = filepath.Join("jobs", jobID, "crashes", filepath.Base(crashFilePath))
			reader, err = h.storageBackend.Retrieve(ctx, storagePath)
			if err != nil {
				h.logger.WithError(err).WithField("path", storagePath).Error("Failed to retrieve crash input from storage")
				h.writeError(w, errors.NewNotFoundError("get_crash_input", "crash input"))
				return
			}
		}
		defer reader.Close()

		input, err = io.ReadAll(reader)
		if err != nil {
			h.logger.WithError(err).Error("Failed to read crash input")
			h.writeError(w, errors.NewSystemError("read_crash_input", err))
			return
		}
	} else {
		// Try to read from filesystem if no storage backend
		if crashFilePath != "" && filepath.IsAbs(crashFilePath) {
			input, err = os.ReadFile(crashFilePath)
			if err != nil {
				h.logger.WithError(err).WithField("path", crashFilePath).Error("Failed to read crash input from filesystem")
				h.writeError(w, errors.NewNotFoundError("get_crash_input", "crash input"))
				return
			}
		} else {
			h.writeError(w, errors.NewSystemError("storage_backend", fmt.Errorf("storage backend not configured")))
			return
		}
	}

	// Determine filename for download
	filename := fmt.Sprintf("crash_%s.bin", crashID)
	if len(crashID) > 8 {
		filename = fmt.Sprintf("crash_%s.bin", crashID[:8])
	}

	// Set headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(input)))
	w.Header().Set("X-Crash-ID", crashID)
	w.Header().Set("X-Job-ID", jobID)

	// Write content
	if _, err := w.Write(input); err != nil {
		h.logger.WithError(err).Error("Failed to write crash input response")
	}
}

// Reproducibility handlers

func (h *HandlerV3) listReproductionRequests(w http.ResponseWriter, r *http.Request) {
	_ = parsePaginationParams(r)    // TODO: Use params when implemented
	_ = r.URL.Query().Get("status") // TODO: Use status when implemented

	// TODO: Implement reproducibility request listing
	var requests []*common.ReproductionRequest
	err := common.ErrNotImplemented
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"requests": requests,
		"count":    len(requests),
	})
}

func (h *HandlerV3) createReproductionRequest(w http.ResponseWriter, r *http.Request) {
	var req ReproductionRequestCreate
	if err := h.decodeAndValidate(w, r, &req); err != nil {
		return
	}

	// TODO: Implement reproducibility request creation
	var request *common.ReproductionRequest
	err := common.ErrNotImplemented
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, request)
}

func (h *HandlerV3) getReproductionRequest(w http.ResponseWriter, r *http.Request) {
	_ = mux.Vars(r)["requestId"] // TODO: Use requestID when implemented

	// TODO: Implement GetReproductionStatus instead
	var request *common.ReproductionRequest
	err := common.ErrNotImplemented
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, request)
}

func (h *HandlerV3) submitReproductionResult(w http.ResponseWriter, r *http.Request) {
	var result common.ReproductionResult
	if err := h.decodeAndValidate(w, r, &result); err != nil {
		return
	}

	// TODO: Implement RecordReproductionResult instead
	err := common.ErrNotImplemented
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, result)
}

// Result submission handlers

func (h *HandlerV3) submitBatchResults(w http.ResponseWriter, r *http.Request) {
	var req BatchResultRequest
	if err := h.decodeAndValidate(w, r, &req); err != nil {
		return
	}

	// Validate batch size
	totalSize := len(req.Crashes) + len(req.Coverage) + len(req.Corpus)
	if totalSize > h.config.MaxBatchSize {
		h.writeError(w, &ValidationError{
			Field:   "batch",
			Message: fmt.Sprintf("batch size %d exceeds maximum %d", totalSize, h.config.MaxBatchSize),
		})
		return
	}

	// Process batch
	// TODO: Implement batch processing
	result := &BatchResultResponse{
		Status: "error",
		BotID:  req.BotID,
		JobID:  req.JobID,
		Processed: struct {
			Crashes  int `json:"crashes"`
			Coverage int `json:"coverage"`
			Corpus   int `json:"corpus"`
		}{
			Crashes:  0,
			Coverage: 0,
			Corpus:   0,
		},
		Timestamp:      time.Now(),
		Errors:         []string{"not implemented"},
		PartialSuccess: false,
	}

	// Determine status code
	status := http.StatusOK
	if len(result.Errors) > 0 && result.PartialSuccess {
		status = http.StatusMultiStatus // 207
	}

	h.writeJSON(w, status, result)
}

func (h *HandlerV3) submitCrashResult(w http.ResponseWriter, r *http.Request) {
	var crash common.CrashResult
	if err := h.decodeAndValidate(w, r, &crash); err != nil {
		return
	}

	err := h.services.Result.ProcessCrashResult(r.Context(), &crash)
	processedCrash := &crash // Return the same crash for now
	if err != nil {
		h.writeError(w, err)
		return
	}

	resp := map[string]interface{}{
		"status":    "processed",
		"crash_id":  processedCrash.ID,
		"is_unique": true, // TODO: Implement deduplication to determine uniqueness
		"timestamp": time.Now(),
	}

	h.writeJSON(w, http.StatusCreated, resp)
}

func (h *HandlerV3) submitCoverageResult(w http.ResponseWriter, r *http.Request) {
	var coverage common.CoverageResult
	if err := h.decodeAndValidate(w, r, &coverage); err != nil {
		return
	}

	if err := h.services.Result.ProcessCoverageResult(r.Context(), &coverage); err != nil {
		h.writeError(w, err)
		return
	}

	resp := map[string]interface{}{
		"status":      "processed",
		"coverage_id": coverage.ID,
		"timestamp":   time.Now(),
	}

	h.writeJSON(w, http.StatusCreated, resp)
}

func (h *HandlerV3) submitCorpusUpdate(w http.ResponseWriter, r *http.Request) {
	var corpus common.CorpusUpdate
	if err := h.decodeAndValidate(w, r, &corpus); err != nil {
		return
	}

	if err := h.services.Result.ProcessCorpusUpdate(r.Context(), &corpus); err != nil {
		h.writeError(w, err)
		return
	}

	resp := map[string]interface{}{
		"status":    "processed",
		"corpus_id": corpus.ID,
		"timestamp": time.Now(),
	}

	h.writeJSON(w, http.StatusCreated, resp)
}

// System management handlers

func (h *HandlerV3) getSystemStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.services.System.GetSystemStats(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, stats)
}

func (h *HandlerV3) healthCheck(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement health check properly
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
	}

	status := http.StatusOK
	if healthStatus, ok := health["status"].(string); ok && healthStatus == "unhealthy" {
		status = http.StatusServiceUnavailable
	}

	h.writeJSON(w, status, health)
}

func (h *HandlerV3) triggerRecovery(w http.ResponseWriter, r *http.Request) {
	if err := h.services.System.TriggerRecovery(r.Context()); err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "recovery_initiated",
		"timestamp": time.Now(),
	})
}

func (h *HandlerV3) triggerMaintenance(w http.ResponseWriter, r *http.Request) {
	var req MaintenanceRequest
	if err := h.decodeAndValidate(w, r, &req); err != nil {
		return
	}

	// TODO: Implement maintenance trigger
	var result interface{}
	err := common.ErrNotImplemented
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

func (h *HandlerV3) listTimeouts(w http.ResponseWriter, r *http.Request) {
	timeouts, err := h.services.System.GetActiveTimeouts(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, timeouts)
}

func (h *HandlerV3) forceTimeout(w http.ResponseWriter, r *http.Request) {
	timeoutType := mux.Vars(r)["type"]
	entityID := mux.Vars(r)["id"]

	if err := h.services.System.ForceTimeout(r.Context(), timeoutType, entityID); err != nil {
		h.writeError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "timeout_forced",
		"type":      timeoutType,
		"entity_id": entityID,
		"timestamp": time.Now(),
	})
}

// Documentation handlers

func (h *HandlerV3) swaggerUI(w http.ResponseWriter, r *http.Request) {
	// Serve Swagger UI
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(swaggerUIHTML))
}

func (h *HandlerV3) openAPISpec(w http.ResponseWriter, r *http.Request) {
	// Serve OpenAPI spec
	http.ServeFile(w, r, "api_v3/openapi.yaml")
}

// Coverage management handlers

// listJobCoverage lists all coverage reports for a specific job
func (h *HandlerV3) listJobCoverage(w http.ResponseWriter, r *http.Request) {
	jobID := mux.Vars(r)["jobId"]
	if jobID == "" {
		h.writeError(w, &ValidationError{
			Field:   "jobId",
			Message: "job ID is required",
		})
		return
	}

	// Parse query parameters
	params := parsePaginationParams(r)
	format := r.URL.Query().Get("format")
	fromTimeStr := r.URL.Query().Get("from")
	toTimeStr := r.URL.Query().Get("to")

	// Build filter
	filter := &repository.CoverageReportFilter{
		JobID:  jobID,
		Format: format,
	}

	if fromTimeStr != "" {
		fromTime, err := time.Parse(time.RFC3339, fromTimeStr)
		if err != nil {
			h.writeError(w, &ValidationError{
				Field:   "from",
				Message: "invalid date format, use RFC3339",
			})
			return
		}
		filter.FromTime = &fromTime
	}

	if toTimeStr != "" {
		toTime, err := time.Parse(time.RFC3339, toTimeStr)
		if err != nil {
			h.writeError(w, &ValidationError{
				Field:   "to",
				Message: "invalid date format, use RFC3339",
			})
			return
		}
		filter.ToTime = &toTime
	}

	// Get coverage reports from repository
	reports, total, err := h.coverageRepo.GetReportsByJobID(r.Context(), jobID, filter, params.Offset, params.Limit)
	if err != nil {
		h.writeError(w, err)
		return
	}

	// Convert to response format
	responseReports := make([]CoverageReportResponse, len(reports))
	for i, report := range reports {
		responseReports[i] = CoverageReportResponse{
			ID:        report.ID,
			JobID:     report.JobID,
			Format:    report.Format,
			Size:      report.Size,
			CreatedAt: report.CreatedAt,
			FilePath:  report.StoragePath,
		}
	}

	response := CoverageReportListResponse{
		Reports:   responseReports,
		Count:     len(responseReports),
		Page:      params.Page,
		Limit:     params.Limit,
		Total:     total,
		SortBy:    params.SortBy,
		SortOrder: params.SortOrder,
	}

	h.writeJSON(w, http.StatusOK, response)
}

// getJobCoverageReport downloads a specific coverage report
func (h *HandlerV3) getJobCoverageReport(w http.ResponseWriter, r *http.Request) {
	jobID := mux.Vars(r)["jobId"]
	reportID := mux.Vars(r)["reportId"]

	if jobID == "" {
		h.writeError(w, &ValidationError{
			Field:   "jobId",
			Message: "job ID is required",
		})
		return
	}

	if reportID == "" {
		h.writeError(w, &ValidationError{
			Field:   "reportId",
			Message: "report ID is required",
		})
		return
	}

	// Get the coverage report
	report, err := h.coverageRepo.GetReportByID(r.Context(), reportID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	// Verify the report belongs to the specified job
	if report.JobID != jobID {
		h.writeError(w, &NotFoundError{
			Resource: "coverage report",
			ID:       reportID,
		})
		return
	}

	var content []byte

	// Special handling for raw coverage files - return all files as a zip
	if report.Format == "raw" {
		// Query database for raw file paths
		sqliteDB, ok := h.db.(*storage.SQLiteStorage)
		if !ok {
			h.writeError(w, fmt.Errorf("database type not supported"))
			return
		}
		db := sqliteDB.GetDB()
		query := `
			SELECT fuzzer_stats_path, plot_data_path, fuzz_bitmap_path
			FROM coverage_reports 
			WHERE id = ? AND file_type = 'raw'
		`
		var fuzzerStatsPath, plotDataPath, fuzzBitmapPath sql.NullString
		err := db.QueryRowContext(r.Context(), query, reportID).Scan(&fuzzerStatsPath, &plotDataPath, &fuzzBitmapPath)
		if err != nil {
			h.logger.WithError(err).WithField("report_id", reportID).Error("Failed to find raw coverage file paths")
			h.writeError(w, &NotFoundError{
				Resource: "raw coverage files",
				ID:       reportID,
			})
			return
		}

		// Create a temporary directory for files
		tempDir := filepath.Join("/tmp", fmt.Sprintf("coverage_%s_%s", jobID[:8], reportID[:8]))
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			h.logger.WithError(err).Error("Failed to create temp directory")
			h.writeError(w, fmt.Errorf("failed to prepare files: %w", err))
			return
		}
		defer os.RemoveAll(tempDir)

		// Download each raw file if available
		files := map[string]sql.NullString{
			"fuzzer_stats": fuzzerStatsPath,
			"plot_data":    plotDataPath,
			"fuzz_bitmap":  fuzzBitmapPath,
		}

		hasFiles := false
		for fileType, pathValue := range files {
			if !pathValue.Valid || pathValue.String == "" {
				continue
			}

			if h.storageBackend != nil {
				reader, err := h.storageBackend.Retrieve(r.Context(), pathValue.String)
				if err != nil {
					h.logger.WithError(err).WithFields(logrus.Fields{
						"file_type": fileType,
						"path":      pathValue.String,
					}).Warn("Failed to retrieve raw file")
					continue
				}

				destPath := filepath.Join(tempDir, fileType)
				if fileType == "fuzz_bitmap" {
					destPath += ".bin"
				} else {
					destPath += ".txt"
				}

				file, err := os.Create(destPath)
				if err != nil {
					reader.Close()
					h.logger.WithError(err).WithField("file_type", fileType).Warn("Failed to create temp file")
					continue
				}

				_, err = io.Copy(file, reader)
				file.Close()
				reader.Close()

				if err != nil {
					h.logger.WithError(err).WithField("file_type", fileType).Warn("Failed to write temp file")
					continue
				}
				hasFiles = true
			}
		}

		if !hasFiles {
			h.writeError(w, &NotFoundError{
				Resource: "raw coverage files",
				ID:       reportID,
			})
			return
		}

		// Create zip file
		zipPath := filepath.Join("/tmp", fmt.Sprintf("raw_coverage_%s.zip", reportID[:8]))
		if err := h.createZipFile(tempDir, zipPath); err != nil {
			h.logger.WithError(err).Error("Failed to create zip file")
			h.writeError(w, fmt.Errorf("failed to create zip file: %w", err))
			return
		}
		defer os.Remove(zipPath)

		// Read zip file
		content, err = os.ReadFile(zipPath)
		if err != nil {
			h.logger.WithError(err).Error("Failed to read zip file")
			h.writeError(w, fmt.Errorf("failed to read zip file: %w", err))
			return
		}

		// Set zip-specific headers
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"raw_coverage_%s.zip\"", jobID[:8]))
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("ETag", fmt.Sprintf("\"%s\"", reportID))

		// Write the zip content
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(content); err != nil {
			h.logger.WithError(err).WithField("report_id", reportID).Error("Failed to write raw coverage zip response")
			return
		}

		h.logger.WithFields(logrus.Fields{
			"report_id": reportID,
			"job_id":    jobID,
			"format":    "raw",
			"size":      len(content),
		}).Info("Raw coverage files downloaded successfully as zip")
		return
	} else {
		// Regular coverage files (JSON, LCOV)
		// Try to read from storage backend first if available
		if h.storageBackend != nil {
			reader, err := h.storageBackend.Retrieve(r.Context(), report.StoragePath)
			if err == nil {
				defer reader.Close()
				content, err = io.ReadAll(reader)
				if err != nil {
					h.logger.WithError(err).WithFields(logrus.Fields{
						"report_id":    reportID,
						"job_id":       jobID,
						"storage_path": report.StoragePath,
					}).Error("Failed to read coverage report from storage backend")
				}
			}
		}

		// If storage backend failed or not available, try direct filesystem access
		if len(content) == 0 {
			// The storage path might be an absolute path like /app/data/coverage/...
			// or a relative path like coverage/...
			filePath := report.StoragePath
			if !strings.HasPrefix(filePath, "/") {
				// If it's a relative path, prepend the data directory
				filePath = filepath.Join("/app/data", filePath)
			}

			fileContent, err := os.ReadFile(filePath)
			if err != nil {
				h.logger.WithError(err).WithFields(logrus.Fields{
					"report_id":    reportID,
					"job_id":       jobID,
					"file_path":    filePath,
					"storage_path": report.StoragePath,
				}).Error("Failed to read coverage report file from filesystem")

				h.writeError(w, &NotFoundError{
					Resource: "coverage report file",
					ID:       reportID,
				})
				return
			}
			content = fileContent
		}
	}

	// Check if content was successfully read
	if len(content) == 0 {
		h.writeError(w, &NotFoundError{
			Resource: "coverage report file",
			ID:       reportID,
		})
		return
	}

	// Determine content type based on format
	contentType := h.getContentTypeForFormat(report.Format)

	// Set response headers for file download
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))

	// Set filename based on report format
	filename := fmt.Sprintf("coverage_report_%s_%s.%s", jobID[:8], reportID[:8], h.getFileExtensionForFormat(report.Format))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	// Set cache headers
	w.Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", reportID))

	// Write the file content
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(content); err != nil {
		h.logger.WithError(err).WithField("report_id", reportID).Error("Failed to write coverage report response")
		return
	}

	h.logger.WithFields(logrus.Fields{
		"report_id": reportID,
		"job_id":    jobID,
		"format":    report.Format,
		"size":      len(content),
	}).Debug("Coverage report downloaded successfully")
}

// getJobCoverageMetadata retrieves metadata for a specific coverage report
func (h *HandlerV3) getJobCoverageMetadata(w http.ResponseWriter, r *http.Request) {
	jobID := mux.Vars(r)["jobId"]
	reportID := mux.Vars(r)["reportId"]

	if jobID == "" {
		h.writeError(w, &ValidationError{
			Field:   "jobId",
			Message: "job ID is required",
		})
		return
	}

	if reportID == "" {
		h.writeError(w, &ValidationError{
			Field:   "reportId",
			Message: "report ID is required",
		})
		return
	}

	// Get the coverage report to verify it exists and belongs to the job
	report, err := h.coverageRepo.GetReportByID(r.Context(), reportID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	// Verify the report belongs to the specified job
	if report.JobID != jobID {
		h.writeError(w, &NotFoundError{
			Resource: "coverage report",
			ID:       reportID,
		})
		return
	}

	// Get the coverage metadata
	metadata, err := h.coverageRepo.GetMetadataByReportID(r.Context(), reportID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	// Convert to response format
	response := CoverageMetadataResponse{
		LineCoverage:     metadata.LineCoverage,
		FunctionCoverage: metadata.FunctionCoverage,
		BranchCoverage:   metadata.BranchCoverage,
		TotalLines:       metadata.TotalLines,
		CoveredLines:     metadata.CoveredLines,
		TotalFunctions:   metadata.TotalFunctions,
		CoveredFunctions: metadata.CoveredFunctions,
		CollectedAt:      metadata.CollectedAt,
		ReportID:         metadata.ReportID,
		JobID:            report.JobID,
	}

	h.writeJSON(w, http.StatusOK, response)
}

// Version information handler

// getVersion returns build version information
func (h *HandlerV3) getVersion(w http.ResponseWriter, r *http.Request) {
	versionInfo := &common.VersionInfo{
		Version:   "dev",
		BuildTime: "unknown",
		GitCommit: "unknown",
	}

	// Use provided version info if available
	if h.version != nil {
		versionInfo = h.version
	}

	h.writeJSON(w, http.StatusOK, versionInfo)
}

// detailedHealthCheck performs comprehensive health checks including data consistency
func (h *HandlerV3) detailedHealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Basic health status
	basicHealth := map[string]interface{}{
		"status":      "healthy",
		"timestamp":   time.Now(),
		"api_version": "3.0.0",
	}

	// Add version info if available
	if h.version != nil {
		basicHealth["version"] = h.version.Version
		basicHealth["build_time"] = h.version.BuildTime
		basicHealth["git_commit"] = h.version.GitCommit
	}

	// Perform data consistency checks if health checker is available
	var detailedChecks *health.SystemHealthSummary
	if h.healthChecker != nil {
		summary, err := h.healthChecker.GetSystemHealthSummary(ctx)
		if err != nil {
			h.logger.WithError(err).Warn("Failed to get detailed health summary")
			basicHealth["detailed_checks_error"] = err.Error()
		} else {
			detailedChecks = summary
			// Update overall status based on detailed checks
			if summary.OverallStatus != "healthy" {
				basicHealth["status"] = summary.OverallStatus
			}
		}
	}

	// Database health check
	if h.db != nil {
		if sqlDB, ok := h.db.(*sql.DB); ok {
			if err := sqlDB.PingContext(ctx); err != nil {
				basicHealth["status"] = "unhealthy"
				basicHealth["database_error"] = err.Error()
			} else {
				basicHealth["database_status"] = "healthy"
			}
		}
	}

	// Storage backend health check
	if h.storageBackend != nil {
		// Try a simple operation to check storage health
		// This is a basic check - in a real implementation you'd want more comprehensive checks
		basicHealth["storage_status"] = "healthy"
	}

	// Service manager health check
	if h.services != nil {
		basicHealth["services_status"] = "healthy"
		// In a real implementation, you'd check individual services
	}

	// Prepare response
	response := map[string]interface{}{
		"basic_health": basicHealth,
	}

	if detailedChecks != nil {
		response["detailed_checks"] = detailedChecks
	}

	// Set appropriate HTTP status
	status := http.StatusOK
	if basicHealth["status"] != "healthy" {
		status = http.StatusServiceUnavailable
	}

	h.writeJSON(w, status, response)
}

// Helper methods

// getContentTypeForFormat returns the appropriate content type for a coverage format
func (h *HandlerV3) getContentTypeForFormat(format string) string {
	switch strings.ToLower(format) {
	case "html":
		return "text/html"
	case "json":
		return "application/json"
	case "lcov":
		return "text/plain"
	case "cobertura":
		return "application/xml"
	default:
		return "application/octet-stream"
	}
}

// getFileExtensionForFormat returns the appropriate file extension for a coverage format
func (h *HandlerV3) getFileExtensionForFormat(format string) string {
	switch strings.ToLower(format) {
	case "html":
		return "html"
	case "json":
		return "json"
	case "lcov":
		return "lcov"
	case "cobertura":
		return "xml"
	default:
		return "bin"
	}
}

func (h *HandlerV3) decodeAndValidate(w http.ResponseWriter, r *http.Request, v interface{}) error {
	// Decode JSON
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(v); err != nil {
		h.writeError(w, &ValidationError{
			Field:   "body",
			Message: "invalid JSON: " + err.Error(),
		})
		return err
	}

	// Validate
	if err := h.validator.Validate(v); err != nil {
		h.writeError(w, err)
		return err
	}

	return nil
}

func (h *HandlerV3) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.WithError(err).Error("Failed to encode JSON response")
	}
}

// createZipFile creates a zip file from a directory
func (h *HandlerV3) createZipFile(sourceDir, destPath string) error {
	zipFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		zipEntry, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(zipEntry, file)
		return err
	})
}

func (h *HandlerV3) writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	resp := ErrorResponse{
		Error:     "internal_error",
		Message:   err.Error(),
		Timestamp: time.Now(),
	}

	// Add request ID if available
	if requestID, ok := w.Header()["X-Request-ID"]; ok && len(requestID) > 0 {
		resp.RequestID = requestID[0]
	}

	// Determine status code based on error type
	switch e := err.(type) {
	case *ValidationError:
		status = http.StatusBadRequest
		resp.Error = "validation_error"
		resp.Details = map[string]interface{}{
			"field": e.Field,
		}
	case *NotFoundError:
		status = http.StatusNotFound
		resp.Error = "not_found"
	case *ConflictError:
		status = http.StatusConflict
		resp.Error = "conflict"
	case *APIError:
		status = e.StatusCode
		resp.Error = e.Code
	case *errors.Error:
		switch e.Type {
		case errors.ErrorTypeNotFound:
			status = http.StatusNotFound
			resp.Error = "not_found"
		case errors.ErrorTypeValidation:
			status = http.StatusBadRequest
			resp.Error = "validation_error"
		case errors.ErrorTypeConflict:
			status = http.StatusConflict
			resp.Error = "conflict"
		case errors.ErrorTypeTimeout:
			status = http.StatusGatewayTimeout
			resp.Error = "timeout"
		}
	}

	h.writeJSON(w, status, resp)
}

// Response writer wrapper for logging
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// listCrashesFromStorage retrieves crashes from storage backend when database is not available
func (h *HandlerV3) listCrashesFromStorage(ctx context.Context, filters CrashFilters, params PaginationParams) []*common.CrashResult {
	crashes := make([]*common.CrashResult, 0)

	if h.storageBackend == nil {
		return crashes
	}

	// List crash directories from storage
	// This is a simplified implementation - in production you'd want to maintain an index
	crashPaths, err := h.storageBackend.List(ctx, "crashes/")
	if err != nil {
		h.logger.WithError(err).Error("Failed to list crashes from storage")
		return crashes
	}

	for _, objInfo := range crashPaths {
		// Parse crash metadata from path or read metadata file
		// This is a simplified stub - actual implementation would read crash metadata
		crash := &common.CrashResult{
			FilePath: objInfo.Key,
			// Populate other fields from metadata
		}
		crashes = append(crashes, crash)

		// Apply pagination limit
		if len(crashes) >= params.Limit {
			break
		}
	}

	return crashes
}

// getCrashFromStorage retrieves a single crash from storage backend
func (h *HandlerV3) getCrashFromStorage(ctx context.Context, crashID string) *common.CrashResult {
	if h.storageBackend == nil {
		return nil
	}

	// Try to find crash in storage
	// This is a simplified implementation - in production you'd want to maintain an index
	crashPaths, err := h.storageBackend.List(ctx, "crashes/")
	if err != nil {
		h.logger.WithError(err).Error("Failed to list crashes from storage")
		return nil
	}

	for _, objInfo := range crashPaths {
		if strings.Contains(objInfo.Key, crashID) {
			// Found potential match, read metadata
			metadataPath := strings.TrimSuffix(objInfo.Key, filepath.Ext(objInfo.Key)) + ".json"
			reader, err := h.storageBackend.Retrieve(ctx, metadataPath)
			if err == nil {
				defer reader.Close()
				var crash common.CrashResult
				if err := json.NewDecoder(reader).Decode(&crash); err == nil {
					if crash.ID == crashID {
						return &crash
					}
				}
			}
		}
	}

	return nil
}

// CrashFilters represents filters for crash queries
type CrashFilters struct {
	CampaignID string
	JobID      string
	Type       string
	Severity   string
}

// parseCrashFilters extracts crash filters from request
func parseCrashFilters(r *http.Request) CrashFilters {
	return CrashFilters{
		CampaignID: r.URL.Query().Get("campaign_id"),
		JobID:      r.URL.Query().Get("job_id"),
		Type:       r.URL.Query().Get("type"),
		Severity:   r.URL.Query().Get("severity"),
	}
}

// Swagger UI HTML template
const swaggerUIHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <title>PandaFuzz API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist/swagger-ui.css" />
    <style>
        html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin:0; background: #fafafa; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist/swagger-ui-standalone-preset.js"></script>
    <script>
    window.onload = function() {
        window.ui = SwaggerUIBundle({
            url: "/api/v3/openapi.yaml",
            dom_id: '#swagger-ui',
            deepLinking: true,
            presets: [
                SwaggerUIBundle.presets.apis,
                SwaggerUIStandalonePreset
            ],
            plugins: [
                SwaggerUIBundle.plugins.DownloadUrl
            ],
            layout: "StandaloneLayout"
        })
    }
    </script>
</body>
</html>
`
