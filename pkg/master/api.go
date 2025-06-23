package master

import (
	"encoding/json"
	"net/http"
	"strconv"
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
	Name     string            `json:"name" validate:"required"`
	Target   string            `json:"target" validate:"required"`
	Fuzzer   string            `json:"fuzzer" validate:"required"`
	Duration time.Duration     `json:"duration"`
	Config   common.JobConfig  `json:"config"`
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", err)
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
	
	// Delete bot
	if err := s.state.DeleteBot(botID); err != nil {
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
	
	// Get existing bot
	bot, err := s.state.GetBot(botID)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "Bot not found", err)
		return
	}
	
	// Update bot status
	now := time.Now()
	bot.LastSeen = now
	bot.Status = req.Status
	bot.CurrentJob = req.CurrentJob
	bot.IsOnline = true
	bot.TimeoutAt = now.Add(s.config.Timeouts.BotHeartbeat)
	
	// Save bot
	if err := s.state.SaveBotWithRetry(bot); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to update bot", err)
		return
	}
	
	// Update timeout
	s.timeoutManager.UpdateBotHeartbeat(botID)
	
	s.logger.WithFields(logrus.Fields{
		"bot_id": botID,
		"status": req.Status,
	}).Debug("Bot heartbeat received")
	
	response := map[string]interface{}{
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
	
	// Atomic job assignment
	job, err := s.state.AtomicJobAssignmentWithRetry(botID)
	if err != nil {
		if err.Error() == "no jobs available" {
			s.writeJSONResponse(w, map[string]interface{}{
				"status": "no_jobs_available",
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
	
	// Get bot's current job
	bot, err := s.state.GetBot(botID)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "Bot not found", err)
		return
	}
	
	if bot.CurrentJob == nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Bot has no active job", nil)
		return
	}
	
	jobID := *bot.CurrentJob
	
	// Complete job
	if err := s.state.CompleteJobWithRetry(jobID, botID, req.Success); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to complete job", err)
		return
	}
	
	// Remove job timeout
	s.timeoutManager.RemoveJobTimeout(jobID)
	
	s.logger.WithFields(logrus.Fields{
		"bot_id":  botID,
		"job_id":  jobID,
		"success": req.Success,
		"message": req.Message,
	}).Info("Job completed")
	
	response := map[string]interface{}{
		"status":    "completed",
		"timestamp": time.Now(),
	}
	
	s.writeJSONResponse(w, response)
}

// handleBotList handles listing all bots
func (s *Server) handleBotList(w http.ResponseWriter, r *http.Request) {
	bots, err := s.state.ListBots()
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to list bots", err)
		return
	}
	
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
	
	response := map[string]interface{}{
		"bots":  bots,
		"count": len(bots),
	}
	
	s.writeJSONResponse(w, response)
}

// Result communication handlers

// handleResultCrash handles crash result submission
func (s *Server) handleResultCrash(w http.ResponseWriter, r *http.Request) {
	var crash common.CrashResult
	if err := json.NewDecoder(r.Body).Decode(&crash); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid crash result", err)
		return
	}
	
	// Validate crash result
	if crash.JobID == "" || crash.BotID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID and Bot ID are required", nil)
		return
	}
	
	// Set timestamp if not provided
	if crash.Timestamp.IsZero() {
		crash.Timestamp = time.Now()
	}
	
	// Process crash result
	if err := s.state.ProcessCrashResultWithRetry(&crash); err != nil {
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
	
	response := map[string]interface{}{
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
	
	// Process coverage result
	if err := s.state.ProcessCoverageResultWithRetry(&coverage); err != nil {
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
	
	response := map[string]interface{}{
		"status":     "processed",
		"coverage_id": coverage.ID,
		"timestamp":  time.Now(),
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
	
	// Process corpus update
	if err := s.state.ProcessCorpusUpdateWithRetry(&corpus); err != nil {
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
	
	response := map[string]interface{}{
		"status":    "processed",
		"corpus_id": corpus.ID,
		"timestamp": time.Now(),
	}
	
	s.writeJSONResponse(w, response)
}

// handleResultStatus handles general status updates
func (s *Server) handleResultStatus(w http.ResponseWriter, r *http.Request) {
	var status map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid status update", err)
		return
	}
	
	// Log status update
	s.logger.WithField("status", status).Debug("Status update received")
	
	response := map[string]interface{}{
		"status":    "received",
		"timestamp": time.Now(),
	}
	
	s.writeJSONResponse(w, response)
}

// Job management handlers

// handleJobCreate handles job creation
func (s *Server) handleJobCreate(w http.ResponseWriter, r *http.Request) {
	var req JobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid job request", err)
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
	jobs, err := s.state.ListJobs()
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to list jobs", err)
		return
	}
	
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
	
	response := map[string]interface{}{
		"jobs":       filtered,
		"count":      len(filtered),
		"page":       page,
		"limit":      limit,
		"total":      len(jobs),
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
	
	job, err := s.state.GetJob(jobID)
	if err != nil {
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
	
	job, err := s.state.GetJob(jobID)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "Job not found", err)
		return
	}
	
	// Update job status
	job.Status = common.JobStatusCancelled
	now := time.Now()
	job.CompletedAt = &now
	
	// If job is assigned, free up the bot
	if job.AssignedBot != nil {
		if err := s.state.CompleteJobWithRetry(jobID, *job.AssignedBot, false); err != nil {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to cancel job", err)
			return
		}
	} else {
		// Just update job status
		if err := s.state.SaveJobWithRetry(job); err != nil {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to cancel job", err)
			return
		}
	}
	
	// Remove job timeout
	s.timeoutManager.RemoveJobTimeout(jobID)
	
	s.logger.WithField("job_id", jobID).Info("Job cancelled")
	
	response := map[string]interface{}{
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
	logs := map[string]interface{}{
		"job_id":    jobID,
		"logs":      []string{"Log entry 1", "Log entry 2", "Log entry 3"},
		"timestamp": time.Now(),
	}
	
	s.writeJSONResponse(w, logs)
}

// System management handlers

// handleSystemStats handles system statistics retrieval
func (s *Server) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	stats := map[string]interface{}{
		"server":     s.GetStats(),
		"state":      s.state.GetStats(),
		"timeouts":   s.timeoutManager.GetStats(),
		"database":   s.state.GetDatabaseStats(),
		"timestamp":  time.Now(),
	}
	
	s.writeJSONResponse(w, stats)
}

// handleSystemRecovery handles manual system recovery
func (s *Server) handleSystemRecovery(w http.ResponseWriter, r *http.Request) {
	if s.recoveryManager == nil {
		s.writeErrorResponse(w, http.StatusServiceUnavailable, "Recovery manager not available", nil)
		return
	}
	
	// Trigger recovery
	if err := s.recoveryManager.RecoverOnStartup(); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Recovery failed", err)
		return
	}
	
	s.logger.Info("Manual system recovery triggered")
	
	response := map[string]interface{}{
		"status":    "recovery_completed",
		"timestamp": time.Now(),
	}
	
	s.writeJSONResponse(w, response)
}

// handleTimeoutsList handles listing active timeouts
func (s *Server) handleTimeoutsList(w http.ResponseWriter, r *http.Request) {
	botTimeouts := s.timeoutManager.ListBotTimeouts()
	jobTimeouts := s.timeoutManager.ListJobTimeouts()
	
	response := map[string]interface{}{
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
	
	response := map[string]interface{}{
		"status":    "timeout_forced",
		"type":      timeoutType,
		"entity_id": entityID,
		"timestamp": time.Now(),
	}
	
	s.writeJSONResponse(w, response)
}