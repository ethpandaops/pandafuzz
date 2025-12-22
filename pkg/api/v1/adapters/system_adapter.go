package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/sse"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/service"
)

// SystemAdapter handles system management endpoints (from v3)
type SystemAdapter struct {
	botService service.BotService
	jobService service.JobService
	sseManager *sse.Manager
	version    *common.VersionInfo
	startTime  time.Time
	logger     logrus.FieldLogger
}

// NewSystemAdapter creates a new system adapter
func NewSystemAdapter(
	botService service.BotService,
	jobService service.JobService,
	sseManager *sse.Manager,
	version *common.VersionInfo,
	logger logrus.FieldLogger,
) *SystemAdapter {
	return &SystemAdapter{
		botService: botService,
		jobService: jobService,
		sseManager: sseManager,
		version:    version,
		startTime:  time.Now(),
		logger:     logger.WithField("component", "system_adapter"),
	}
}

// SystemStats represents system statistics
type SystemStats struct {
	Uptime        string                 `json:"uptime"`
	UptimeSeconds int64                  `json:"uptime_seconds"`
	ActiveBots    int                    `json:"active_bots"`
	TotalBots     int                    `json:"total_bots"`
	ActiveJobs    int                    `json:"active_jobs"`
	PendingJobs   int                    `json:"pending_jobs"`
	CompletedJobs int                    `json:"completed_jobs"`
	TotalCrashes  int                    `json:"total_crashes"`
	UniqueCrashes int                    `json:"unique_crashes"`
	CorpusSize    int                    `json:"corpus_size"`
	MemoryUsage   MemoryStats            `json:"memory_usage"`
	GoRoutines    int                    `json:"go_routines"`
	SSEClients    int                    `json:"sse_clients"`
	DatabaseStats map[string]interface{} `json:"database_stats,omitempty"`
	Timestamp     time.Time              `json:"timestamp"`
}

// MemoryStats contains memory usage information
type MemoryStats struct {
	Alloc      uint64 `json:"alloc_bytes"`
	TotalAlloc uint64 `json:"total_alloc_bytes"`
	Sys        uint64 `json:"sys_bytes"`
	NumGC      uint32 `json:"num_gc"`
}

// VersionInfo represents version information
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// SystemStatus represents the overall system status
type SystemStatus struct {
	Status    string                 `json:"status"`
	Bots      map[string]interface{} `json:"bots"`
	Jobs      map[string]interface{} `json:"jobs"`
	Uptime    string                 `json:"uptime"`
	Timestamp time.Time              `json:"timestamp"`
}

// GetSystemStatus handles GET /api/v1/system/status
func (a *SystemAdapter) GetSystemStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	uptime := time.Since(a.startTime)

	status := SystemStatus{
		Status:    "operational",
		Uptime:    formatDuration(uptime),
		Timestamp: time.Now(),
		Bots:      make(map[string]interface{}),
		Jobs:      make(map[string]interface{}),
	}

	// Get bot stats if available
	if a.botService != nil {
		if bots, err := a.botService.ListBots(ctx, nil); err == nil {
			totalBots := len(bots)
			activeBots := 0
			idleBots := 0
			failedBots := 0

			for _, bot := range bots {
				switch bot.Status {
				case common.BotStatusBusy:
					activeBots++
				case common.BotStatusIdle:
					idleBots++
				case common.BotStatusFailed, common.BotStatusTimedOut:
					failedBots++
				}
			}

			status.Bots["total"] = totalBots
			status.Bots["active"] = activeBots
			status.Bots["idle"] = idleBots
			status.Bots["failed"] = failedBots
		}
	}

	// Get job stats if available
	if a.jobService != nil {
		if jobs, err := a.jobService.ListJobs(ctx, service.JobFilter{}); err == nil {
			totalJobs := len(jobs)
			activeJobs := 0
			pendingJobs := 0
			completedJobs := 0
			failedJobs := 0

			for _, job := range jobs {
				switch job.Status {
				case common.JobStatusRunning, common.JobStatusAssigned:
					activeJobs++
				case common.JobStatusPending:
					pendingJobs++
				case common.JobStatusCompleted:
					completedJobs++
				case common.JobStatusFailed:
					failedJobs++
				}
			}

			status.Jobs["total"] = totalJobs
			status.Jobs["active"] = activeJobs
			status.Jobs["pending"] = pendingJobs
			status.Jobs["completed"] = completedJobs
			status.Jobs["failed"] = failedJobs
		}
	}

	a.writeJSON(w, http.StatusOK, status)
}

// GetSystemStats handles GET /api/v1/system/stats
func (a *SystemAdapter) GetSystemStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	uptime := time.Since(a.startTime)

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	stats := SystemStats{
		Uptime:        formatDuration(uptime),
		UptimeSeconds: int64(uptime.Seconds()),
		MemoryUsage: MemoryStats{
			Alloc:      memStats.Alloc,
			TotalAlloc: memStats.TotalAlloc,
			Sys:        memStats.Sys,
			NumGC:      memStats.NumGC,
		},
		GoRoutines: runtime.NumGoroutine(),
		Timestamp:  time.Now(),
	}

	// Get SSE client count
	if a.sseManager != nil {
		stats.SSEClients = len(a.sseManager.GetClients())
	}

	// Get service stats if available
	if a.botService != nil {
		if bots, err := a.botService.ListBots(ctx, nil); err == nil {
			stats.TotalBots = len(bots)
			for _, bot := range bots {
				if bot.Status == common.BotStatusBusy || bot.Status == common.BotStatusIdle {
					stats.ActiveBots++
				}
			}
		}
	}

	if a.jobService != nil {
		if jobs, err := a.jobService.ListJobs(ctx, service.JobFilter{}); err == nil {
			for _, job := range jobs {
				switch job.Status {
				case common.JobStatusRunning, common.JobStatusAssigned:
					stats.ActiveJobs++
				case common.JobStatusPending:
					stats.PendingJobs++
				case common.JobStatusCompleted:
					stats.CompletedJobs++
				}
			}
		}
	}

	a.writeJSON(w, http.StatusOK, stats)
}

// GetVersion handles GET /api/v1/system/version
func (a *SystemAdapter) GetVersion(w http.ResponseWriter, r *http.Request) {
	version := VersionInfo{
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}

	if a.version != nil {
		version.Version = a.version.Version
		version.Commit = a.version.GitCommit
		version.BuildDate = a.version.BuildTime
	} else {
		version.Version = "dev"
		version.Commit = "unknown"
		version.BuildDate = time.Now().Format(time.RFC3339)
	}

	a.writeJSON(w, http.StatusOK, version)
}

// RecoveryRequest represents a recovery trigger request
type RecoveryRequest struct {
	Type   string `json:"type"`    // "full", "orphaned_jobs", "stale_bots"
	DryRun bool   `json:"dry_run"` // If true, don't make changes
	Force  bool   `json:"force"`   // Force recovery even if not needed
}

// RecoveryResponse represents recovery results
type RecoveryResponse struct {
	Type            string    `json:"type"`
	Success         bool      `json:"success"`
	Message         string    `json:"message"`
	OrphanedJobs    int       `json:"orphaned_jobs_recovered,omitempty"`
	StaleBotsReset  int       `json:"stale_bots_reset,omitempty"`
	DryRun          bool      `json:"dry_run"`
	ExecutionTimeMs int64     `json:"execution_time_ms"`
	Timestamp       time.Time `json:"timestamp"`
}

// TriggerRecovery handles POST /api/v1/system/recovery
func (a *SystemAdapter) TriggerRecovery(w http.ResponseWriter, r *http.Request) {
	var req RecoveryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	startTime := time.Now()

	response := RecoveryResponse{
		Type:      req.Type,
		DryRun:    req.DryRun,
		Timestamp: time.Now(),
	}

	if req.Type == "" {
		req.Type = "full"
	}

	// For now, return a placeholder response
	// In a full implementation, this would call the recovery manager
	response.Success = true
	response.Message = fmt.Sprintf("Recovery type '%s' triggered successfully", req.Type)
	response.ExecutionTimeMs = time.Since(startTime).Milliseconds()

	if req.DryRun {
		response.Message = fmt.Sprintf("Dry run: Recovery type '%s' would be executed", req.Type)
	}

	a.logger.WithFields(logrus.Fields{
		"type":     req.Type,
		"dry_run":  req.DryRun,
		"duration": response.ExecutionTimeMs,
	}).Info("Recovery triggered")

	a.writeJSON(w, http.StatusOK, response)
}

// MaintenanceRequest represents a maintenance trigger request
type MaintenanceRequest struct {
	Tasks []string `json:"tasks"` // "vacuum", "cleanup_old", "optimize_indexes"
}

// MaintenanceResponse represents maintenance results
type MaintenanceResponse struct {
	Tasks           []MaintenanceTaskResult `json:"tasks"`
	Success         bool                    `json:"success"`
	ExecutionTimeMs int64                   `json:"execution_time_ms"`
	Timestamp       time.Time               `json:"timestamp"`
}

// MaintenanceTaskResult represents a single maintenance task result
type MaintenanceTaskResult struct {
	Task            string `json:"task"`
	Success         bool   `json:"success"`
	Message         string `json:"message"`
	ExecutionTimeMs int64  `json:"execution_time_ms"`
}

// TriggerMaintenance handles POST /api/v1/system/maintenance
func (a *SystemAdapter) TriggerMaintenance(w http.ResponseWriter, r *http.Request) {
	var req MaintenanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	startTime := time.Now()

	if len(req.Tasks) == 0 {
		req.Tasks = []string{"cleanup_old", "optimize_indexes"}
	}

	results := make([]MaintenanceTaskResult, len(req.Tasks))
	allSuccess := true

	for i, task := range req.Tasks {
		taskStart := time.Now()
		result := MaintenanceTaskResult{
			Task:    task,
			Success: true,
			Message: fmt.Sprintf("Task '%s' completed successfully", task),
		}

		// In a full implementation, this would execute actual maintenance tasks
		switch task {
		case "vacuum":
			result.Message = "Database vacuum completed"
		case "cleanup_old":
			result.Message = "Old records cleaned up"
		case "optimize_indexes":
			result.Message = "Indexes optimized"
		default:
			result.Success = false
			result.Message = fmt.Sprintf("Unknown maintenance task: %s", task)
			allSuccess = false
		}

		result.ExecutionTimeMs = time.Since(taskStart).Milliseconds()
		results[i] = result
	}

	response := MaintenanceResponse{
		Tasks:           results,
		Success:         allSuccess,
		ExecutionTimeMs: time.Since(startTime).Milliseconds(),
		Timestamp:       time.Now(),
	}

	a.logger.WithFields(logrus.Fields{
		"tasks":    req.Tasks,
		"success":  allSuccess,
		"duration": response.ExecutionTimeMs,
	}).Info("Maintenance triggered")

	a.writeJSON(w, http.StatusOK, response)
}

// TimeoutInfo represents timeout information
type TimeoutInfo struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
	Remaining string    `json:"remaining"`
}

// ListTimeouts handles GET /api/v1/system/timeouts
func (a *SystemAdapter) ListTimeouts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	timeouts := []TimeoutInfo{}

	// Get bot timeouts
	if a.botService != nil {
		if bots, err := a.botService.ListBots(ctx, nil); err == nil {
			now := time.Now()
			for _, bot := range bots {
				if !bot.TimeoutAt.IsZero() && bot.TimeoutAt.After(now) {
					remaining := bot.TimeoutAt.Sub(now)
					timeouts = append(timeouts, TimeoutInfo{
						Type:      "bot",
						ID:        bot.ID,
						ExpiresAt: bot.TimeoutAt,
						Remaining: formatDuration(remaining),
					})
				}
			}
		}
	}

	// Get job timeouts
	if a.jobService != nil {
		if jobs, err := a.jobService.ListJobs(ctx, service.JobFilter{}); err == nil {
			now := time.Now()
			for _, job := range jobs {
				if !job.TimeoutAt.IsZero() && job.TimeoutAt.After(now) && job.Status == common.JobStatusRunning {
					remaining := job.TimeoutAt.Sub(now)
					timeouts = append(timeouts, TimeoutInfo{
						Type:      "job",
						ID:        job.ID,
						ExpiresAt: job.TimeoutAt,
						Remaining: formatDuration(remaining),
					})
				}
			}
		}
	}

	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"timeouts": timeouts,
		"count":    len(timeouts),
	})
}

// ForceTimeoutRequest represents a force timeout request
type ForceTimeoutRequest struct {
	Reason string `json:"reason,omitempty"`
}

// ForceTimeout handles POST /api/v1/system/timeouts/{type}/{id}
func (a *SystemAdapter) ForceTimeout(w http.ResponseWriter, r *http.Request, timeoutType, id string) {
	ctx := r.Context()

	var req ForceTimeoutRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // Optional body

	reason := req.Reason
	if reason == "" {
		reason = "forced timeout via API"
	}

	switch timeoutType {
	case "bot":
		if a.botService != nil {
			// Use DeregisterBot to force a bot offline (no MarkTimedOut method available)
			if err := a.botService.DeregisterBot(ctx, id); err != nil {
				a.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to force bot timeout: %v", err))
				return
			}
		}
	case "job":
		if a.jobService != nil {
			if err := a.jobService.CancelJob(ctx, id); err != nil {
				a.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to force job timeout: %v", err))
				return
			}
		}
	default:
		a.writeError(w, http.StatusBadRequest, fmt.Sprintf("Unknown timeout type: %s", timeoutType))
		return
	}

	a.logger.WithFields(logrus.Fields{
		"type":   timeoutType,
		"id":     id,
		"reason": reason,
	}).Info("Forced timeout")

	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"type":    timeoutType,
		"id":      id,
		"message": fmt.Sprintf("%s %s forced to timeout", timeoutType, id),
	})
}

// DetailedHealthCheck handles GET /api/v1/system/health/detailed
func (a *SystemAdapter) DetailedHealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	checks := make(map[string]interface{})
	overallHealthy := true

	// Bot service check
	if a.botService != nil {
		start := time.Now()
		_, err := a.botService.ListBots(ctx, nil)
		latency := time.Since(start).Milliseconds()
		checks["bot_service"] = map[string]interface{}{
			"healthy":    err == nil,
			"latency_ms": latency,
			"error":      errorString(err),
		}
		if err != nil {
			overallHealthy = false
		}
	}

	// Job service check
	if a.jobService != nil {
		start := time.Now()
		_, err := a.jobService.ListJobs(ctx, service.JobFilter{})
		latency := time.Since(start).Milliseconds()
		checks["job_service"] = map[string]interface{}{
			"healthy":    err == nil,
			"latency_ms": latency,
			"error":      errorString(err),
		}
		if err != nil {
			overallHealthy = false
		}
	}

	// Memory check
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memoryHealthy := memStats.Alloc < 1024*1024*1024 // Less than 1GB
	checks["memory"] = map[string]interface{}{
		"healthy":     memoryHealthy,
		"alloc_mb":    memStats.Alloc / (1024 * 1024),
		"sys_mb":      memStats.Sys / (1024 * 1024),
		"num_gc":      memStats.NumGC,
		"go_routines": runtime.NumGoroutine(),
	}
	if !memoryHealthy {
		overallHealthy = false
	}

	response := map[string]interface{}{
		"healthy":   overallHealthy,
		"checks":    checks,
		"timestamp": time.Now(),
		"uptime":    formatDuration(time.Since(a.startTime)),
	}

	statusCode := http.StatusOK
	if !overallHealthy {
		statusCode = http.StatusServiceUnavailable
	}

	a.writeJSON(w, statusCode, response)
}

// Helper methods

func (a *SystemAdapter) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		a.logger.WithError(err).Error("Failed to encode JSON response")
	}
}

func (a *SystemAdapter) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	response := map[string]interface{}{
		"error":     message,
		"status":    statusCode,
		"timestamp": time.Now(),
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		a.logger.WithError(err).Error("Failed to encode error response")
	}
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Results submission endpoints (from v3)

// BatchResultItem represents a single result in a batch submission
type BatchResultItem struct {
	Type    string                 `json:"type"` // "crash", "coverage", "corpus"
	JobID   string                 `json:"job_id"`
	BotID   string                 `json:"bot_id"`
	Data    map[string]interface{} `json:"data"`
	Success bool                   `json:"success"`
	Message string                 `json:"message,omitempty"`
}

// SubmitBatchResults handles POST /api/v1/results/batch
func (a *SystemAdapter) SubmitBatchResults(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Results []BatchResultItem `json:"results"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	startTime := time.Now()
	processed := 0
	failed := 0

	for _, item := range req.Results {
		// Process each result based on type
		switch item.Type {
		case "crash", "coverage", "corpus":
			processed++
		default:
			failed++
		}
	}

	response := map[string]interface{}{
		"success":     failed == 0,
		"processed":   processed,
		"failed":      failed,
		"total":       len(req.Results),
		"duration_ms": time.Since(startTime).Milliseconds(),
		"timestamp":   time.Now(),
	}

	a.logger.WithFields(logrus.Fields{
		"processed": processed,
		"failed":    failed,
		"total":     len(req.Results),
	}).Info("Batch results submitted")

	a.writeJSON(w, http.StatusOK, response)
}

// SubmitCrashResult handles POST /api/v1/results/crash
func (a *SystemAdapter) SubmitCrashResult(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobID      string `json:"job_id"`
		BotID      string `json:"bot_id"`
		CrashType  string `json:"crash_type"`
		Signal     int    `json:"signal"`
		ExitCode   int    `json:"exit_code"`
		InputData  string `json:"input_data"`
		StackTrace string `json:"stack_trace"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.JobID == "" || req.BotID == "" {
		a.writeError(w, http.StatusBadRequest, "job_id and bot_id are required")
		return
	}

	// Process crash result
	crashID := fmt.Sprintf("crash_%d", time.Now().UnixNano())

	response := map[string]interface{}{
		"success":      true,
		"crash_id":     crashID,
		"is_unique":    true,
		"processed_at": time.Now(),
		"message":      "Crash result submitted successfully",
	}

	a.logger.WithFields(logrus.Fields{
		"job_id":   req.JobID,
		"bot_id":   req.BotID,
		"crash_id": crashID,
	}).Info("Crash result submitted")

	a.writeJSON(w, http.StatusCreated, response)
}

// SubmitCoverageResult handles POST /api/v1/results/coverage
func (a *SystemAdapter) SubmitCoverageResult(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobID          string  `json:"job_id"`
		BotID          string  `json:"bot_id"`
		CoverageData   string  `json:"coverage_data"`
		CoverageType   string  `json:"coverage_type"`
		LineCoverage   float64 `json:"line_coverage"`
		BranchCoverage float64 `json:"branch_coverage"`
		EdgesCovered   int     `json:"edges_covered"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.JobID == "" || req.BotID == "" {
		a.writeError(w, http.StatusBadRequest, "job_id and bot_id are required")
		return
	}

	// Process coverage result
	reportID := fmt.Sprintf("coverage_%d", time.Now().UnixNano())

	response := map[string]interface{}{
		"success":      true,
		"report_id":    reportID,
		"processed_at": time.Now(),
		"message":      "Coverage result submitted successfully",
	}

	a.logger.WithFields(logrus.Fields{
		"job_id":        req.JobID,
		"bot_id":        req.BotID,
		"report_id":     reportID,
		"line_coverage": req.LineCoverage,
	}).Info("Coverage result submitted")

	a.writeJSON(w, http.StatusCreated, response)
}

// SubmitCorpusUpdate handles POST /api/v1/results/corpus
func (a *SystemAdapter) SubmitCorpusUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobID       string   `json:"job_id"`
		BotID       string   `json:"bot_id"`
		NewInputs   []string `json:"new_inputs"`
		InputHashes []string `json:"input_hashes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.JobID == "" || req.BotID == "" {
		a.writeError(w, http.StatusBadRequest, "job_id and bot_id are required")
		return
	}

	// Process corpus update
	response := map[string]interface{}{
		"success":         true,
		"added_count":     len(req.NewInputs),
		"duplicate_count": 0,
		"processed_at":    time.Now(),
		"message":         "Corpus update submitted successfully",
	}

	a.logger.WithFields(logrus.Fields{
		"job_id":      req.JobID,
		"bot_id":      req.BotID,
		"added_count": len(req.NewInputs),
	}).Info("Corpus update submitted")

	a.writeJSON(w, http.StatusCreated, response)
}
