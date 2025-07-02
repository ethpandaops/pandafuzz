package master

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// API request/response structures

// BotRegisterRequest represents a bot registration request
type BotRegisterRequest struct {
	Hostname     string   `json:"hostname" validate:"required"`
	Name         string   `json:"name,omitempty"`
	Capabilities []string `json:"capabilities" validate:"required"`
	APIEndpoint  string   `json:"api_endpoint" validate:"required"` // Bot's API endpoint for polling
}

// BotRegisterResponse represents a bot registration response
type BotRegisterResponse struct {
	BotID     string    `json:"bot_id"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Timeout   time.Time `json:"timeout"`
}

// BotHeartbeatRequest represents a bot heartbeat request
type BotHeartbeatRequest struct {
	Status       common.BotStatus `json:"status"`
	CurrentJob   *string          `json:"current_job,omitempty"`
	LastActivity time.Time        `json:"last_activity"`
}

// JobRequest represents a job creation request
type JobRequest struct {
	Name     string           `json:"name" validate:"required"`
	Target   string           `json:"target" validate:"required"`
	Fuzzer   string           `json:"fuzzer" validate:"required"`
	Duration time.Duration    `json:"duration"`
	Config   common.JobConfig `json:"config"`
}

// JobCompleteRequest represents a job completion request
type JobCompleteRequest struct {
	Success   bool      `json:"success"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message,omitempty"`
}

// Bot lifecycle management handlers

// handleBotRegister handles bot registration
func (s *Server) handleBotRegister(w http.ResponseWriter, r *http.Request) {
	var req BotRegisterRequest
	if err := s.decodeJSONBody(w, r, &req); err != nil {
		// Error response already written by decodeJSONBody
		return
	}

	// Validate request
	if err := s.validateBotRegisterRequest(&req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid request", err)
		return
	}

	// Use service layer
	bot, err := s.services.Bot.RegisterBot(r.Context(), req.Hostname, req.Name, req.Capabilities, req.APIEndpoint)
	if err != nil {
		s.responseWriter.WriteError(w, err)
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
	s.writeJSONResponse(w, response)
}

// handleBotGet handles bot information retrieval
func (s *Server) handleBotGet(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	botID := vars["id"]

	bot, err := s.services.Bot.GetBot(r.Context(), botID)
	if err != nil {
		s.responseWriter.WriteError(w, err)
		return
	}

	s.writeJSONResponse(w, bot)
}

// handleBotDelete handles bot deregistration
func (s *Server) handleBotDelete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	botID := vars["id"]

	if botID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Bot ID is required", nil)
		return
	}

	// Remove timeout
	s.timeoutManager.RemoveBotTimeout(botID)

	// Add timeout for database operation
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Delete bot
	if err := s.state.DeleteBot(ctx, botID); err != nil {
		// Check for timeout error
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete bot", err)
		return
	}

	s.logger.WithField("bot_id", botID).Info("Bot deregistered successfully")

	w.WriteHeader(http.StatusNoContent)
}

// handleBotHeartbeat handles bot heartbeat
func (s *Server) handleBotHeartbeat(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	botID := vars["id"]

	if botID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Bot ID is required", nil)
		return
	}

	var req BotHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Add timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Get existing bot
	bot, err := s.state.GetBot(ctx, botID)
	if err != nil {
		// Check for timeout error
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusNotFound, "Bot not found", err)
		return
	}

	// Update bot status
	now := time.Now()
	bot.LastSeen = now
	bot.Status = req.Status
	bot.IsOnline = true
	bot.TimeoutAt = now.Add(s.config.Timeouts.BotHeartbeat)

	// Only update CurrentJob if there's a mismatch that needs to be resolved
	if bot.CurrentJob != req.CurrentJob {
		// If bot reports no job but master thinks it has one
		if req.CurrentJob == nil && bot.CurrentJob != nil {
			// Check if the job is already completed
			job, err := s.state.GetJob(ctx, *bot.CurrentJob)
			if err == nil && (job.Status == common.JobStatusCompleted || job.Status == common.JobStatusFailed || job.Status == common.JobStatusCancelled) {
				// Job is already completed, update bot's state
				s.logger.WithFields(logrus.Fields{
					"bot_id":     botID,
					"job_id":     *bot.CurrentJob,
					"job_status": job.Status,
				}).Info("Clearing bot's current job reference - job already completed")
				bot.CurrentJob = nil
			} else {
				// Job might be stuck, log warning but update state
				jobStatus := "unknown"
				if err == nil && job != nil {
					jobStatus = string(job.Status)
				}
				masterJobID := ""
				if bot.CurrentJob != nil {
					masterJobID = *bot.CurrentJob
				}
				s.logger.WithFields(logrus.Fields{
					"bot_id":        botID,
					"master_job_id": masterJobID,
					"bot_job":       req.CurrentJob,
					"job_status":    jobStatus,
				}).Warn("Bot-master job mismatch detected in heartbeat")
				bot.CurrentJob = req.CurrentJob
			}
		} else {
			// Normal update
			bot.CurrentJob = req.CurrentJob
		}
	}

	// Save bot
	if err := s.state.SaveBotWithRetry(ctx, bot); err != nil {
		// Check for timeout error
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to update bot", err)
		return
	}

	// Update timeout
	s.timeoutManager.UpdateBotHeartbeat(botID)

	s.logger.WithFields(logrus.Fields{
		"bot_id": botID,
		"status": req.Status,
	}).Debug("Bot heartbeat received")

	response := map[string]any{
		"status":    "ok",
		"timestamp": now,
		"timeout":   bot.TimeoutAt,
	}

	s.writeJSONResponse(w, response)
}

// handleBotGetJob handles job assignment to bot
func (s *Server) handleBotGetJob(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	botID := vars["id"]

	if botID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Bot ID is required", nil)
		return
	}

	// Add timeout for database operation
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Atomic job assignment
	job, err := s.state.AtomicJobAssignmentWithRetry(ctx, botID)
	if err != nil {
		// Check for timeout error
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		if err.Error() == "no jobs available" {
			s.writeJSONResponse(w, map[string]any{
				"status":  "no_jobs_available",
				"message": "No jobs available for assignment",
			})
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to assign job", err)
		return
	}

	// Set job timeout
	s.timeoutManager.UpdateJobTimeout(job.ID)

	s.logger.WithFields(logrus.Fields{
		"bot_id":   botID,
		"job_id":   job.ID,
		"job_name": job.Name,
		"fuzzer":   job.Fuzzer,
	}).Info("Job assigned to bot")

	s.writeJSONResponse(w, job)
}

// handleBotCompleteJob handles job completion
func (s *Server) handleBotCompleteJob(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	botID := vars["id"]

	if botID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Bot ID is required", nil)
		return
	}

	var req JobCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Add timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Get bot's current job
	bot, err := s.state.GetBot(ctx, botID)
	if err != nil {
		// Check for timeout error
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusNotFound, "Bot not found", err)
		return
	}

	if bot.CurrentJob == nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Bot has no active job", nil)
		return
	}

	jobID := *bot.CurrentJob

	// Complete job
	if err := s.state.CompleteJobWithRetry(ctx, jobID, botID, req.Success); err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"bot_id": botID,
			"job_id": jobID,
		}).Error("Failed to complete job in state")

		// Send negative acknowledgment
		response := map[string]any{
			"acknowledged": false,
			"job_id":       jobID,
			"message":      fmt.Sprintf("Failed to update job state: %v", err),
			"timestamp":    time.Now(),
		}
		s.writeJSONResponse(w, response)
		return
	}

	// Remove job timeout
	s.timeoutManager.RemoveJobTimeout(jobID)

	s.logger.WithFields(logrus.Fields{
		"bot_id":  botID,
		"job_id":  jobID,
		"success": req.Success,
		"message": req.Message,
	}).Info("Job completed and acknowledged")

	// Send positive acknowledgment
	response := map[string]any{
		"acknowledged": true,
		"job_id":       jobID,
		"message":      "Job completion successfully recorded",
		"status":       "completed",
		"timestamp":    time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// handleBotList handles listing all bots
func (s *Server) handleBotList(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("handleBotList: Starting bot list request")

	bots, err := s.state.ListBots(r.Context())
	if err != nil {
		s.logger.WithError(err).Error("handleBotList: Failed to list bots")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to list bots", err)
		return
	}

	s.logger.WithField("bot_count", len(bots)).Debug("handleBotList: Retrieved bots from state")

	// Filter based on query parameters
	statusFilter := r.URL.Query().Get("status")
	if statusFilter != "" {
		var filtered []*common.Bot
		for _, bot := range bots {
			if string(bot.Status) == statusFilter {
				filtered = append(filtered, bot)
			}
		}
		bots = filtered
	}

	response := map[string]any{
		"bots":  bots,
		"count": len(bots),
	}

	s.logger.Debug("handleBotList: Writing response")
	s.writeJSONResponse(w, response)
	s.logger.Debug("handleBotList: Response sent")
}

// Result communication handlers

// handleResultCrash handles crash result submission
func (s *Server) handleResultCrash(w http.ResponseWriter, r *http.Request) {
	s.logger.Info("Received crash result submission request")

	var crash common.CrashResult
	if err := json.NewDecoder(r.Body).Decode(&crash); err != nil {
		s.logger.WithError(err).Error("Failed to decode crash result")
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid crash result", err)
		return
	}

	// Log crash details
	s.logger.WithFields(logrus.Fields{
		"crash_id": crash.ID,
		"job_id":   crash.JobID,
		"bot_id":   crash.BotID,
		"hash":     crash.Hash,
		"size":     crash.Size,
	}).Debug("Processing crash submission")

	// Validate crash result
	if crash.JobID == "" || crash.BotID == "" {
		s.logger.Error("Crash result missing required fields")
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID and Bot ID are required", nil)
		return
	}

	// Set timestamp if not provided
	if crash.Timestamp.IsZero() {
		crash.Timestamp = time.Now()
	}

	// Add timeout for database operation
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Process crash result
	if err := s.state.ProcessCrashResultWithRetry(ctx, &crash); err != nil {
		// Check for timeout error
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to process crash result", err)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"crash_id":  crash.ID,
		"job_id":    crash.JobID,
		"bot_id":    crash.BotID,
		"hash":      crash.Hash,
		"type":      crash.Type,
		"is_unique": crash.IsUnique,
	}).Info("Crash result processed")

	response := map[string]any{
		"status":    "processed",
		"crash_id":  crash.ID,
		"is_unique": crash.IsUnique,
		"timestamp": time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// handleResultCoverage handles coverage result submission
func (s *Server) handleResultCoverage(w http.ResponseWriter, r *http.Request) {
	var coverage common.CoverageResult
	if err := json.NewDecoder(r.Body).Decode(&coverage); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid coverage result", err)
		return
	}

	// Validate coverage result
	if coverage.JobID == "" || coverage.BotID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID and Bot ID are required", nil)
		return
	}

	// Set timestamp if not provided
	if coverage.Timestamp.IsZero() {
		coverage.Timestamp = time.Now()
	}

	// Add timeout for database operation
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Process coverage result
	if err := s.state.ProcessCoverageResultWithRetry(ctx, &coverage); err != nil {
		// Check for timeout error
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to process coverage result", err)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"coverage_id": coverage.ID,
		"job_id":      coverage.JobID,
		"bot_id":      coverage.BotID,
		"edges":       coverage.Edges,
		"new_edges":   coverage.NewEdges,
	}).Debug("Coverage result processed")

	response := map[string]any{
		"status":      "processed",
		"coverage_id": coverage.ID,
		"timestamp":   time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// handleResultCorpus handles corpus update submission
func (s *Server) handleResultCorpus(w http.ResponseWriter, r *http.Request) {
	var corpus common.CorpusUpdate
	if err := json.NewDecoder(r.Body).Decode(&corpus); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid corpus update", err)
		return
	}

	// Validate corpus update
	if corpus.JobID == "" || corpus.BotID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID and Bot ID are required", nil)
		return
	}

	// Set timestamp if not provided
	if corpus.Timestamp.IsZero() {
		corpus.Timestamp = time.Now()
	}

	// Add timeout for database operation
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Process corpus update
	if err := s.state.ProcessCorpusUpdateWithRetry(ctx, &corpus); err != nil {
		// Check for timeout error
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to process corpus update", err)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"corpus_id":  corpus.ID,
		"job_id":     corpus.JobID,
		"bot_id":     corpus.BotID,
		"file_count": len(corpus.Files),
		"total_size": corpus.TotalSize,
	}).Debug("Corpus update processed")

	response := map[string]any{
		"status":    "processed",
		"corpus_id": corpus.ID,
		"timestamp": time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// handleResultStatus handles general status updates
func (s *Server) handleResultStatus(w http.ResponseWriter, r *http.Request) {
	var status map[string]any
	if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid status update", err)
		return
	}

	// Log status update
	s.logger.WithField("status", status).Debug("Status update received")

	response := map[string]any{
		"status":    "received",
		"timestamp": time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// Job management handlers

// handleJobCreate handles job creation
func (s *Server) handleJobCreate(w http.ResponseWriter, r *http.Request) {
	var req JobRequest
	if err := s.decodeJSONBody(w, r, &req); err != nil {
		// Error response already written by decodeJSONBody
		return
	}

	// Validate request
	if err := s.validateJobRequest(&req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid request", err)
		return
	}

	// Use service layer
	jobReq := service.CreateJobRequest{
		Name:     req.Name,
		Target:   req.Target,
		Fuzzer:   req.Fuzzer,
		Duration: req.Duration,
		Config:   req.Config,
	}

	job, err := s.services.Job.CreateJob(r.Context(), jobReq)
	if err != nil {
		s.responseWriter.WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	s.writeJSONResponse(w, job)
}

// handleJobList handles listing jobs
func (s *Server) handleJobList(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("handleJobList: Starting job list request")

	jobs, err := s.state.ListJobs(r.Context())
	if err != nil {
		s.logger.WithError(err).Error("handleJobList: Failed to list jobs")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to list jobs", err)
		return
	}

	s.logger.WithField("job_count", len(jobs)).Debug("handleJobList: Retrieved jobs from state")

	// Filter based on query parameters
	statusFilter := r.URL.Query().Get("status")
	fuzzerFilter := r.URL.Query().Get("fuzzer")

	var filtered []*common.Job
	for _, job := range jobs {
		if statusFilter != "" && string(job.Status) != statusFilter {
			continue
		}
		if fuzzerFilter != "" && job.Fuzzer != fuzzerFilter {
			continue
		}
		filtered = append(filtered, job)
	}

	// Pagination
	page := 1
	limit := 50

	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	// Apply pagination
	start := (page - 1) * limit
	end := start + limit

	if start >= len(filtered) {
		filtered = []*common.Job{}
	} else if end > len(filtered) {
		filtered = filtered[start:]
	} else {
		filtered = filtered[start:end]
	}

	response := map[string]any{
		"jobs":  filtered,
		"count": len(filtered),
		"page":  page,
		"limit": limit,
		"total": len(jobs),
	}

	s.writeJSONResponse(w, response)
}

// handleJobGet handles getting a specific job
func (s *Server) handleJobGet(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	if jobID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID is required", nil)
		return
	}

	// Add timeout for database operation
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	job, err := s.state.GetJob(ctx, jobID)
	if err != nil {
		// Check for timeout error
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		if common.IsNotFoundError(err) {
			s.writeErrorResponse(w, http.StatusNotFound, "Job not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get job", err)
		}
		return
	}

	s.writeJSONResponse(w, job)
}

// handleJobCancel handles job cancellation
func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	if jobID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID is required", nil)
		return
	}

	// Add timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	job, err := s.state.GetJob(ctx, jobID)
	if err != nil {
		// Check for timeout error
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusNotFound, "Job not found", err)
		return
	}

	// Update job status
	job.Status = common.JobStatusCancelled
	now := time.Now()
	job.CompletedAt = &now

	// If job is assigned, free up the bot
	if job.AssignedBot != nil {
		if err := s.state.CompleteJobWithRetry(ctx, jobID, *job.AssignedBot, false); err != nil {
			// Check for timeout error
			if err == context.DeadlineExceeded {
				s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
				return
			}
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to cancel job", err)
			return
		}
	} else {
		// Just update job status
		if err := s.state.SaveJobWithRetry(ctx, job); err != nil {
			// Check for timeout error
			if err == context.DeadlineExceeded {
				s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
				return
			}
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to cancel job", err)
			return
		}
	}

	// Remove job timeout
	s.timeoutManager.RemoveJobTimeout(jobID)

	s.logger.WithField("job_id", jobID).Info("Job cancelled")

	response := map[string]any{
		"status":    "cancelled",
		"timestamp": now,
	}

	s.writeJSONResponse(w, response)
}

// handleJobLogs handles job log retrieval
func (s *Server) handleJobLogs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	if jobID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID is required", nil)
		return
	}

	// For now, return placeholder logs
	// In a real implementation, you would read from log files
	logs := map[string]any{
		"job_id":    jobID,
		"logs":      []string{"Log entry 1", "Log entry 2", "Log entry 3"},
		"timestamp": time.Now(),
	}

	s.writeJSONResponse(w, logs)
}

// System management handlers

// handleSystemStats handles system statistics retrieval
func (s *Server) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("handleSystemStats: Starting system stats request")
	start := time.Now()

	// Check if request context is already cancelled
	if err := r.Context().Err(); err != nil {
		s.logger.WithError(err).Error("handleSystemStats: Request context already cancelled on entry")
		s.writeErrorResponse(w, http.StatusGatewayTimeout, "Request context already cancelled", err)
		return
	}

	// Log the current timeout configuration
	timeoutValue := s.config.Timeouts.DatabaseOp
	if timeoutValue == 0 {
		s.logger.Error("handleSystemStats: DatabaseOp timeout is ZERO!")
		timeoutValue = 10 * time.Second // Default to 10 seconds
		s.logger.WithField("default_timeout", timeoutValue).Warn("handleSystemStats: Using default timeout")
	}

	s.logger.WithFields(logrus.Fields{
		"timeout_duration": timeoutValue,
		"timeout_seconds":  timeoutValue.Seconds(),
		"timeout_string":   timeoutValue.String(),
		"http_timeout":     s.config.Timeouts.HTTPRequest,
		"log_level":        s.logger.GetLevel().String(),
	}).Warn("handleSystemStats: Timeout configuration check")

	// Add timeout for database operation
	ctx, cancel := context.WithTimeout(r.Context(), timeoutValue)
	defer cancel()

	// Get server stats
	serverStart := time.Now()
	serverStats := s.GetStats()
	s.logger.WithField("duration", time.Since(serverStart)).Debug("handleSystemStats: Server stats retrieved")

	// Get state stats
	stateStart := time.Now()
	stateStats := s.state.GetStats(ctx)
	s.logger.WithField("duration", time.Since(stateStart)).Debug("handleSystemStats: State stats retrieved")

	// Get timeout stats
	timeoutStart := time.Now()
	timeoutStats := s.timeoutManager.GetStats()
	s.logger.WithField("duration", time.Since(timeoutStart)).Debug("handleSystemStats: Timeout stats retrieved")

	// Get database stats with timeout context
	dbStart := time.Now()
	dbStats := s.state.GetDatabaseStats(ctx)
	s.logger.WithField("duration", time.Since(dbStart)).Debug("handleSystemStats: Database stats retrieved")

	// Check if context was cancelled
	if ctx.Err() == context.DeadlineExceeded {
		s.logger.WithField("total_duration", time.Since(start)).Error("handleSystemStats: Database operation timed out")
		s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", ctx.Err())
		return
	}

	stats := map[string]any{
		"server":    serverStats,
		"state":     stateStats,
		"timeouts":  timeoutStats,
		"database":  dbStats,
		"timestamp": time.Now(),
	}

	s.logger.WithField("total_duration", time.Since(start)).Debug("handleSystemStats: Request completed")
	s.writeJSONResponse(w, stats)
}

// handleSystemRecovery handles manual system recovery
func (s *Server) handleSystemRecovery(w http.ResponseWriter, r *http.Request) {
	if s.recoveryManager == nil {
		s.writeErrorResponse(w, http.StatusServiceUnavailable, "Recovery manager not available", nil)
		return
	}

	// Add timeout for recovery operation (use longer timeout for recovery)
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.MasterRecovery)
	defer cancel()

	// Trigger recovery
	if err := s.recoveryManager.RecoverOnStartup(ctx); err != nil {
		// Check for timeout error
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Recovery operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Recovery failed", err)
		return
	}

	s.logger.Info("Manual system recovery triggered")

	response := map[string]any{
		"status":    "recovery_completed",
		"timestamp": time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// handleTimeoutsList handles listing active timeouts
func (s *Server) handleTimeoutsList(w http.ResponseWriter, r *http.Request) {
	botTimeouts := s.timeoutManager.ListBotTimeouts()
	jobTimeouts := s.timeoutManager.ListJobTimeouts()

	response := map[string]any{
		"bot_timeouts": botTimeouts,
		"job_timeouts": jobTimeouts,
		"timestamp":    time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// handleTimeoutForce handles forcing a timeout
func (s *Server) handleTimeoutForce(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	timeoutType := vars["type"]
	entityID := vars["id"]

	if timeoutType == "" || entityID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Timeout type and entity ID are required", nil)
		return
	}

	if err := s.timeoutManager.ForceTimeout(timeoutType, entityID); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to force timeout", err)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"type":      timeoutType,
		"entity_id": entityID,
	}).Info("Timeout forced manually")

	response := map[string]any{
		"status":    "timeout_forced",
		"type":      timeoutType,
		"entity_id": entityID,
		"timestamp": time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// handleGetCrashes retrieves all crashes
func (s *Server) handleGetCrashes(w http.ResponseWriter, r *http.Request) {
	// Get query parameters
	limit := 100
	offset := 0

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

	// Create a context with timeout from config to prevent long-running queries
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Get crashes from state with timeout context
	crashes, err := s.state.GetCrashes(ctx, limit, offset)
	if err != nil {
		// Check if it was a context cancellation or timeout
		if err == context.Canceled || err == context.DeadlineExceeded {
			s.logger.WithError(err).WithFields(logrus.Fields{
				"limit":  limit,
				"offset": offset,
			}).Warn("Crash retrieval cancelled or timed out")
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}

		s.logger.WithError(err).WithFields(logrus.Fields{
			"limit":  limit,
			"offset": offset,
		}).Error("Failed to retrieve crashes from database")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve crashes", err)
		return
	}

	// Ensure we have a valid slice even if no crashes found
	if crashes == nil {
		crashes = make([]*common.CrashResult, 0)
	}

	response := map[string]any{
		"crashes": crashes,
		"count":   len(crashes),
		"limit":   limit,
		"offset":  offset,
	}

	s.writeJSONResponse(w, response)
}

// handleGetCrash retrieves a specific crash by ID
func (s *Server) handleGetCrash(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	crashID := vars["id"]

	if crashID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Crash ID is required", nil)
		return
	}

	// Add timeout for database operation
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	crash, err := s.state.GetCrash(ctx, crashID)
	if err != nil {
		// Check for timeout error
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve crash", err)
		return
	}

	if crash == nil {
		s.writeErrorResponse(w, http.StatusNotFound, "Crash not found", nil)
		return
	}

	s.writeJSONResponse(w, crash)
}

// handleGetJobCrashes retrieves all crashes for a specific job
func (s *Server) handleGetJobCrashes(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	if jobID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID is required", nil)
		return
	}

	// Add timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Verify job exists
	job, err := s.state.GetJob(ctx, jobID)
	if err != nil {
		// Check for timeout error
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve job", err)
		return
	}

	if job == nil {
		s.writeErrorResponse(w, http.StatusNotFound, "Job not found", nil)
		return
	}

	// Get crashes for this job
	crashes, err := s.state.GetJobCrashes(ctx, jobID)
	if err != nil {
		// Check for timeout error
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve job crashes", err)
		return
	}

	response := map[string]any{
		"job_id":  jobID,
		"crashes": crashes,
		"count":   len(crashes),
	}

	s.writeJSONResponse(w, response)
}

// handleGetCrashInput retrieves the input file for a specific crash
func (s *Server) handleGetCrashInput(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	crashID := vars["id"]

	if crashID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Crash ID is required", nil)
		return
	}

	// Add timeout for database operation
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Get crash input data
	input, err := s.state.GetCrashInput(ctx, crashID)
	if err != nil {
		// Check for timeout error
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve crash input", err)
		return
	}

	if input == nil || len(input) == 0 {
		s.writeErrorResponse(w, http.StatusNotFound, "Crash input not found", nil)
		return
	}

	// Set appropriate headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"crash_%s.bin\"", crashID[:8]))
	w.Header().Set("Content-Length", strconv.Itoa(len(input)))

	// Write the binary data
	if _, err := w.Write(input); err != nil {
		s.logger.WithError(err).Error("Failed to write crash input response")
	}
}

// Helper methods for safe JSON decoding

// decodeJSONBody safely decodes JSON from request body with proper error handling
func (s *Server) decodeJSONBody(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	// Check content type
	contentType := r.Header.Get("Content-Type")
	if contentType != "" && contentType != "application/json" {
		msg := "Content-Type header is not application/json"
		s.writeErrorResponse(w, http.StatusUnsupportedMediaType, msg, nil)
		return fmt.Errorf(msg)
	}

	// Decode the body
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // Strict validation

	err := dec.Decode(dst)
	if err != nil {
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError

		switch {
		// Catch any syntax errors in the JSON and send a detailed error message
		case err.Error() == "http: request body too large":
			msg := "Request body must not be larger than the configured limit"
			s.writeErrorResponse(w, http.StatusRequestEntityTooLarge, msg, err)
			return err

		case syntaxError != nil && syntaxError.Error() == err.Error():
			msg := fmt.Sprintf("Request body contains badly-formed JSON (at position %d)", syntaxError.Offset)
			s.writeErrorResponse(w, http.StatusBadRequest, msg, err)
			return err

		// Catch JSON type errors
		case unmarshalTypeError != nil && unmarshalTypeError.Error() == err.Error():
			msg := fmt.Sprintf("Request body contains an invalid value for the %q field (at position %d)", unmarshalTypeError.Field, unmarshalTypeError.Offset)
			s.writeErrorResponse(w, http.StatusBadRequest, msg, err)
			return err

		// Catch the error caused by the request body being too large
		case err.Error() == "http: request body too large":
			msg := "Request body too large"
			s.writeErrorResponse(w, http.StatusRequestEntityTooLarge, msg, err)
			return err

		// Otherwise default to generic error
		default:
			msg := "Invalid request body"
			s.writeErrorResponse(w, http.StatusBadRequest, msg, err)
			return err
		}
	}

	// Check that the request body only contained a single JSON object
	if dec.More() {
		msg := "Request body must only contain a single JSON object"
		s.writeErrorResponse(w, http.StatusBadRequest, msg, nil)
		return fmt.Errorf(msg)
	}

	return nil
}

// Validation helper methods

// validateBotRegisterRequest validates bot registration request
func (s *Server) validateBotRegisterRequest(req *BotRegisterRequest) error {
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
	if !strings.HasPrefix(req.APIEndpoint, "http://") && !strings.HasPrefix(req.APIEndpoint, "https://") {
		return fmt.Errorf("API endpoint must start with http:// or https://")
	}

	return nil
}

// validateJobRequest validates job creation request
func (s *Server) validateJobRequest(req *JobRequest) error {
	// Validate name
	if req.Name == "" {
		return fmt.Errorf("job name is required")
	}
	if len(req.Name) > 100 {
		return fmt.Errorf("job name too long (max 100 characters)")
	}

	// Validate target
	if req.Target == "" {
		return fmt.Errorf("target is required")
	}
	if len(req.Target) > 500 {
		return fmt.Errorf("target path too long (max 500 characters)")
	}

	// Validate fuzzer
	if req.Fuzzer == "" {
		return fmt.Errorf("fuzzer is required")
	}
	if req.Fuzzer != "afl++" && req.Fuzzer != "libfuzzer" {
		return fmt.Errorf("unsupported fuzzer: %s (supported: afl++, libfuzzer)", req.Fuzzer)
	}

	// Validate duration
	if req.Duration < 0 {
		return fmt.Errorf("duration cannot be negative")
	}
	if req.Duration > 24*time.Hour {
		return fmt.Errorf("duration too long (max 24 hours)")
	}

	// Validate config
	if req.Config.MemoryLimit < 0 {
		return fmt.Errorf("memory limit cannot be negative")
	}
	if req.Config.MemoryLimit > 16*1024 { // 16GB max
		return fmt.Errorf("memory limit too high (max 16GB)")
	}
	if req.Config.Timeout < 0 {
		return fmt.Errorf("timeout cannot be negative")
	}
	if req.Config.Timeout > 24*time.Hour {
		return fmt.Errorf("timeout too long (max 24 hours)")
	}

	return nil
}

// isValidIdentifier checks if a string is a valid identifier (alphanumeric with underscores)
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

// Streaming and maintenance endpoints

// handleJobProgress handles real-time job progress updates via Server-Sent Events
func (s *Server) handleJobProgress(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	if jobID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID is required", nil)
		return
	}

	// Get job details
	job, err := s.services.Job.GetJob(r.Context(), jobID)
	if err != nil {
		s.responseWriter.WriteError(w, err)
		return
	}

	// Set up Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Streaming not supported", nil)
		return
	}

	// Send initial event with job details
	initialData := map[string]any{
		"job_id":   job.ID,
		"name":     job.Name,
		"status":   job.Status,
		"fuzzer":   job.Fuzzer,
		"progress": job.Progress,
	}
	if data, err := json.Marshal(initialData); err == nil {
		fmt.Fprintf(w, "event: connected\ndata: %s\n\n", data)
		flusher.Flush()
	}

	// Update interval
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Track last known values to only send updates on changes
	lastStatus := job.Status
	lastProgress := job.Progress
	lastCrashCount := 0
	lastCoverageEdges := 0

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// Get updated job details
			currentJob, err := s.services.Job.GetJob(r.Context(), jobID)
			if err != nil {
				fmt.Fprintf(w, "event: error\ndata: {\"error\": \"Failed to get job status\"}\n\n")
				flusher.Flush()
				return
			}

			// Get crash and coverage statistics
			ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
			crashes, _ := s.state.GetJobCrashes(ctx, jobID)
			crashCount := len(crashes)

			// Get latest coverage data
			coverageStats, _ := s.state.GetJobCoverageStats(ctx, jobID)
			coverageEdges := 0
			if coverageStats != nil {
				coverageEdges = coverageStats.TotalEdges
			}
			cancel()

			// Check if there are updates
			hasUpdate := currentJob.Status != lastStatus ||
				currentJob.Progress != lastProgress ||
				crashCount != lastCrashCount ||
				coverageEdges != lastCoverageEdges

			if hasUpdate {
				// Send progress update
				progressData := map[string]any{
					"job_id":         currentJob.ID,
					"status":         currentJob.Status,
					"progress":       currentJob.Progress,
					"crash_count":    crashCount,
					"coverage_edges": coverageEdges,
					"timestamp":      time.Now(),
				}

				if currentJob.AssignedBot != nil {
					progressData["assigned_bot"] = *currentJob.AssignedBot
				}

				if data, err := json.Marshal(progressData); err == nil {
					fmt.Fprintf(w, "event: progress\ndata: %s\n\n", data)
					flusher.Flush()
				}

				// Update last known values
				lastStatus = currentJob.Status
				lastProgress = currentJob.Progress
				lastCrashCount = crashCount
				lastCoverageEdges = coverageEdges
			}

			// If job is completed, send final event and close
			if currentJob.Status == common.JobStatusCompleted ||
				currentJob.Status == common.JobStatusFailed ||
				currentJob.Status == common.JobStatusCancelled ||
				currentJob.Status == common.JobStatusTimedOut {

				finalData := map[string]any{
					"job_id":    currentJob.ID,
					"status":    currentJob.Status,
					"completed": true,
					"timestamp": time.Now(),
				}

				if data, err := json.Marshal(finalData); err == nil {
					fmt.Fprintf(w, "event: completed\ndata: %s\n\n", data)
					flusher.Flush()
				}
				return
			}
		}
	}
}

// BatchResultRequest represents a batch of results from collector
type BatchResultRequest struct {
	BotID    string                  `json:"bot_id" validate:"required"`
	JobID    string                  `json:"job_id" validate:"required"`
	Crashes  []common.CrashResult    `json:"crashes,omitempty"`
	Coverage []common.CoverageResult `json:"coverage,omitempty"`
	Corpus   []common.CorpusUpdate   `json:"corpus,omitempty"`
}

// handleBatchResults handles batch result submission from collector
func (s *Server) handleBatchResults(w http.ResponseWriter, r *http.Request) {
	var req BatchResultRequest
	if err := s.decodeJSONBody(w, r, &req); err != nil {
		// Error response already written by decodeJSONBody
		return
	}

	// Validate request
	if req.BotID == "" || req.JobID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Bot ID and Job ID are required", nil)
		return
	}

	// Add timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Track processing results
	processedCrashes := 0
	processedCoverage := 0
	processedCorpus := 0
	errors := []string{}

	// Process crashes
	for i := range req.Crashes {
		crash := &req.Crashes[i]
		crash.BotID = req.BotID
		crash.JobID = req.JobID
		if crash.Timestamp.IsZero() {
			crash.Timestamp = time.Now()
		}

		if err := s.state.ProcessCrashResultWithRetry(ctx, crash); err != nil {
			if err == context.DeadlineExceeded {
				s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
				return
			}
			errors = append(errors, fmt.Sprintf("crash %d: %v", i, err))
		} else {
			processedCrashes++
		}
	}

	// Process coverage
	for i := range req.Coverage {
		coverage := &req.Coverage[i]
		coverage.BotID = req.BotID
		coverage.JobID = req.JobID
		if coverage.Timestamp.IsZero() {
			coverage.Timestamp = time.Now()
		}

		if err := s.state.ProcessCoverageResultWithRetry(ctx, coverage); err != nil {
			if err == context.DeadlineExceeded {
				s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
				return
			}
			errors = append(errors, fmt.Sprintf("coverage %d: %v", i, err))
		} else {
			processedCoverage++
		}
	}

	// Process corpus updates
	for i := range req.Corpus {
		corpus := &req.Corpus[i]
		corpus.BotID = req.BotID
		corpus.JobID = req.JobID
		if corpus.Timestamp.IsZero() {
			corpus.Timestamp = time.Now()
		}

		if err := s.state.ProcessCorpusUpdateWithRetry(ctx, corpus); err != nil {
			if err == context.DeadlineExceeded {
				s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
				return
			}
			errors = append(errors, fmt.Sprintf("corpus %d: %v", i, err))
		} else {
			processedCorpus++
		}
	}

	s.logger.WithFields(logrus.Fields{
		"bot_id":             req.BotID,
		"job_id":             req.JobID,
		"crashes_processed":  processedCrashes,
		"coverage_processed": processedCoverage,
		"corpus_processed":   processedCorpus,
		"errors":             len(errors),
	}).Info("Batch results processed")

	response := map[string]any{
		"status": "processed",
		"bot_id": req.BotID,
		"job_id": req.JobID,
		"processed": map[string]int{
			"crashes":  processedCrashes,
			"coverage": processedCoverage,
			"corpus":   processedCorpus,
		},
		"timestamp": time.Now(),
	}

	if len(errors) > 0 {
		response["errors"] = errors
		response["partial_success"] = true
	}

	s.writeJSONResponse(w, response)
}

// MaintenanceRequest represents a maintenance trigger request
type MaintenanceRequest struct {
	Type   string            `json:"type" validate:"required"` // cleanup, optimize, backup
	Target string            `json:"target,omitempty"`         // specific target (e.g., "database", "logs", "storage")
	Force  bool              `json:"force,omitempty"`          // Force maintenance even if recently done
	Config map[string]string `json:"config,omitempty"`         // Additional configuration
}

// handleMaintenanceTrigger handles system maintenance requests
func (s *Server) handleMaintenanceTrigger(w http.ResponseWriter, r *http.Request) {
	var req MaintenanceRequest
	if err := s.decodeJSONBody(w, r, &req); err != nil {
		// Error response already written by decodeJSONBody
		return
	}

	// Validate maintenance type
	validTypes := []string{"cleanup", "optimize", "backup", "recovery", "vacuum"}
	isValid := false
	for _, t := range validTypes {
		if req.Type == t {
			isValid = true
			break
		}
	}
	if !isValid {
		s.writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid maintenance type. Valid types: %v", validTypes), nil)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"type":   req.Type,
		"target": req.Target,
		"force":  req.Force,
	}).Info("Maintenance triggered")

	// Add timeout for maintenance operations (longer timeout)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	// Execute maintenance based on type
	var maintenanceErr error
	startTime := time.Now()

	switch req.Type {
	case "cleanup":
		// Clean up old data
		maintenanceErr = s.performCleanup(ctx, req.Target, req.Force)
	case "optimize":
		// Optimize database
		maintenanceErr = s.state.OptimizeDatabase(ctx)
	case "backup":
		// Backup system state
		maintenanceErr = s.performBackup(ctx, req.Config)
	case "recovery":
		// Trigger recovery
		if s.recoveryManager != nil {
			maintenanceErr = s.recoveryManager.RecoverOnStartup(ctx)
		} else {
			maintenanceErr = fmt.Errorf("recovery manager not available")
		}
	case "vacuum":
		// Database vacuum
		maintenanceErr = s.state.VacuumDatabase(ctx)
	}

	duration := time.Since(startTime)

	if maintenanceErr != nil {
		if maintenanceErr == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Maintenance operation timed out", maintenanceErr)
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Maintenance failed", maintenanceErr)
		return
	}

	response := map[string]any{
		"status":    "completed",
		"type":      req.Type,
		"target":    req.Target,
		"duration":  duration.String(),
		"timestamp": time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// performCleanup performs cleanup maintenance
func (s *Server) performCleanup(ctx context.Context, target string, force bool) error {
	// Clean up based on target
	switch target {
	case "logs":
		// Clean old log files
		return s.cleanupOldLogs(ctx)
	case "database":
		// Clean old database records
		return s.state.CleanupOldRecords(ctx, 30*24*time.Hour) // 30 days
	case "storage":
		// Clean storage directory
		return s.cleanupStorage(ctx)
	default:
		// Clean everything
		if err := s.cleanupOldLogs(ctx); err != nil {
			return err
		}
		if err := s.state.CleanupOldRecords(ctx, 30*24*time.Hour); err != nil {
			return err
		}
		return s.cleanupStorage(ctx)
	}
}

// cleanupOldLogs removes old log files
func (s *Server) cleanupOldLogs(ctx context.Context) error {
	logDir := filepath.Join(s.config.Storage.BasePath, "logs")
	cutoffTime := time.Now().Add(-7 * 24 * time.Hour) // 7 days

	return filepath.Walk(logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() && info.ModTime().Before(cutoffTime) {
			os.Remove(path)
		}
		return nil
	})
}

// cleanupStorage cleans the storage directory
func (s *Server) cleanupStorage(ctx context.Context) error {
	// Clean crash inputs older than 30 days
	crashDir := filepath.Join(s.config.Storage.BasePath, "crashes")
	cutoffTime := time.Now().Add(-30 * 24 * time.Hour)

	return filepath.Walk(crashDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() && info.ModTime().Before(cutoffTime) {
			os.Remove(path)
		}
		return nil
	})
}

// performBackup performs system backup
func (s *Server) performBackup(ctx context.Context, config map[string]string) error {
	// This is a placeholder - implement actual backup logic
	backupPath := config["path"]
	if backupPath == "" {
		backupPath = filepath.Join(s.config.Storage.BasePath, "backups", fmt.Sprintf("backup_%s", time.Now().Format("20060102_150405")))
	}

	// Create backup directory
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %v", err)
	}

	// Backup database
	if err := s.state.BackupDatabase(ctx, filepath.Join(backupPath, "database.db")); err != nil {
		return fmt.Errorf("failed to backup database: %v", err)
	}

	return nil
}

// BotResourceMetrics represents resource usage metrics for a bot
type BotResourceMetrics struct {
	BotID         string    `json:"bot_id"`
	Timestamp     time.Time `json:"timestamp"`
	CPUUsage      float64   `json:"cpu_usage"`    // Percentage (0-100)
	MemoryUsage   uint64    `json:"memory_usage"` // Bytes
	DiskUsage     uint64    `json:"disk_usage"`   // Bytes
	NetworkTx     uint64    `json:"network_tx"`   // Bytes transmitted
	NetworkRx     uint64    `json:"network_rx"`   // Bytes received
	ActiveJobs    int       `json:"active_jobs"`
	JobsCompleted int       `json:"jobs_completed"`
	Uptime        string    `json:"uptime"`
}

// handleResourceMetrics handles bot resource metrics retrieval
func (s *Server) handleResourceMetrics(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	botID := vars["id"]

	if botID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Bot ID is required", nil)
		return
	}

	// Add timeout for database operation
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Get bot details
	bot, err := s.state.GetBot(ctx, botID)
	if err != nil {
		if err == context.DeadlineExceeded {
			s.writeErrorResponse(w, http.StatusGatewayTimeout, "Database operation timed out", err)
			return
		}
		s.writeErrorResponse(w, http.StatusNotFound, "Bot not found", err)
		return
	}

	// Get bot statistics
	completedJobs, err := s.state.GetBotCompletedJobs(ctx, botID)
	if err != nil {
		completedJobs = 0
	}

	// Calculate uptime
	uptime := "0s"
	if !bot.RegisteredAt.IsZero() {
		uptime = time.Since(bot.RegisteredAt).Round(time.Second).String()
	}

	// Get current resource metrics
	// In a real implementation, this would query actual metrics from the bot
	// For now, return placeholder data
	metrics := BotResourceMetrics{
		BotID:         botID,
		Timestamp:     time.Now(),
		CPUUsage:      0.0, // Would be fetched from bot
		MemoryUsage:   0,   // Would be fetched from bot
		DiskUsage:     0,   // Would be fetched from bot
		NetworkTx:     0,   // Would be fetched from bot
		NetworkRx:     0,   // Would be fetched from bot
		ActiveJobs:    0,
		JobsCompleted: completedJobs,
		Uptime:        uptime,
	}

	// If bot has a current job, count it as active
	if bot.CurrentJob != nil {
		metrics.ActiveJobs = 1
	}

	// If bot has an API endpoint, try to fetch real metrics
	if bot.APIEndpoint != "" && bot.IsOnline {
		// This would make an HTTP request to the bot's metrics endpoint
		// For now, just note it in the response
		metrics.CPUUsage = 45.5       // Placeholder
		metrics.MemoryUsage = 1 << 30 // 1GB placeholder
		metrics.DiskUsage = 10 << 30  // 10GB placeholder
	}

	response := map[string]any{
		"bot_id":   botID,
		"hostname": bot.Hostname,
		"status":   bot.Status,
		"online":   bot.IsOnline,
		"metrics":  metrics,
	}

	s.writeJSONResponse(w, response)
}
