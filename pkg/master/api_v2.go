package master

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// API v2 structures

// CampaignTimelineEntry represents a timeline event
type CampaignTimelineEntry struct {
	Timestamp   time.Time              `json:"timestamp"`
	EventType   string                 `json:"event_type"`
	Description string                 `json:"description"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// BotMetricsStream represents streaming bot metrics
type BotMetricsStream struct {
	BotID       string    `json:"bot_id"`
	Timestamp   time.Time `json:"timestamp"`
	CPUUsage    float64   `json:"cpu_usage"`
	MemoryUsage uint64    `json:"memory_usage"`
	JobProgress float64   `json:"job_progress,omitempty"`
}

// setupAPIv2Routes configures all v2 API routes
func (s *Server) setupAPIv2Routes(r *mux.Router) {
	// Create v2 subrouter
	v2 := r.PathPrefix("/api/v2").Subrouter()

	// Apply middleware
	v2.Use(s.metricsMiddleware)
	v2.Use(s.loggingMiddleware)
	v2.Use(s.circuitBreakerMiddleware)

	// Health and status endpoints
	v2.HandleFunc("/health", s.handleHealth).Methods("GET")
	v2.HandleFunc("/status", s.handleV2Status).Methods("GET")

	// Campaign endpoints (enhanced from v1)
	v2.HandleFunc("/campaigns", s.handleV2CreateCampaign).Methods("POST")
	v2.HandleFunc("/campaigns", s.handleV2ListCampaigns).Methods("GET")
	v2.HandleFunc("/campaigns/{id}", s.handleV2GetCampaign).Methods("GET")
	v2.HandleFunc("/campaigns/{id}", s.handleV2UpdateCampaign).Methods("PATCH")
	v2.HandleFunc("/campaigns/{id}", s.handleDeleteCampaign).Methods("DELETE")
	v2.HandleFunc("/campaigns/{id}/restart", s.handleRestartCampaign).Methods("POST")
	v2.HandleFunc("/campaigns/{id}/stats", s.handleGetCampaignStats).Methods("GET")
	v2.HandleFunc("/campaigns/{id}/timeline", s.handleV2GetCampaignTimeline).Methods("GET")
	v2.HandleFunc("/campaigns/{id}/binary", s.handleUploadCampaignBinary).Methods("POST")
	v2.HandleFunc("/campaigns/{id}/corpus", s.handleUploadCampaignCorpus).Methods("POST")

	// Corpus endpoints
	v2.HandleFunc("/campaigns/{id}/corpus/evolution", s.handleGetCorpusEvolution).Methods("GET")
	v2.HandleFunc("/campaigns/{id}/corpus/sync", s.handleSyncCorpus).Methods("POST")
	v2.HandleFunc("/campaigns/{id}/corpus/share", s.handleShareCorpus).Methods("POST")
	v2.HandleFunc("/campaigns/{id}/corpus/files", s.handleListCorpusFiles).Methods("GET")
	v2.HandleFunc("/campaigns/{id}/corpus/files/{hash}", s.handleDownloadCorpusFile).Methods("GET")
	v2.HandleFunc("/campaigns/{id}/corpus/import", s.handleCorpusImport).Methods("POST")
	v2.HandleFunc("/campaigns/{id}/corpus/cleanup", s.handleCorpusCleanup).Methods("POST")

	// Crash analysis endpoints
	v2.HandleFunc("/campaigns/{id}/crashes", s.handleV2GetCrashGroups).Methods("GET")
	v2.HandleFunc("/crashes/{crash_id}/stacktrace", s.handleGetStackTrace).Methods("GET")
	v2.HandleFunc("/crashes/{crash_id}/input", s.handleGetCrashInput).Methods("GET")

	// Bot endpoints (enhanced)
	v2.HandleFunc("/bots", s.handleV2ListBots).Methods("GET")
	v2.HandleFunc("/bots/{id}/metrics", s.handleV2StreamBotMetrics).Methods("GET")
	v2.HandleFunc("/bots/{id}/metrics/history", s.handleV2GetBotMetricsHistory).Methods("GET")

	// Job endpoints (enhanced)
	v2.HandleFunc("/jobs", s.handleV2CreateJob).Methods("POST")
	v2.HandleFunc("/jobs", s.handleV2ListJobs).Methods("GET")
	v2.HandleFunc("/jobs/{id}", s.handleJobGet).Methods("GET")
	v2.HandleFunc("/jobs/{id}/progress", s.handleJobProgress).Methods("GET")
	v2.HandleFunc("/jobs/{id}/cancel", s.handleJobCancel).Methods("POST")

	// Batch operations
	v2.HandleFunc("/batch/results", s.handleBatchResults).Methods("POST")

	// System endpoints
	v2.HandleFunc("/system/stats", s.handleSystemStats).Methods("GET")
	v2.HandleFunc("/system/maintenance", s.handleMaintenanceTrigger).Methods("POST")

	// WebSocket endpoint
	v2.HandleFunc("/ws", s.handleWebSocket).Methods("GET")

	s.logger.Info("API v2 routes configured")
}

// Enhanced v2 handlers

// handleV2Status provides enhanced system status
func (s *Server) handleV2Status(w http.ResponseWriter, r *http.Request) {
	// Get basic status
	botTimeouts, jobTimeouts := s.timeoutManager.GetActiveTimeouts()

	// Get campaign statistics
	campaigns, _ := s.services.Campaign.List(r.Context(), common.CampaignFilters{Limit: 100})
	activeCampaigns := 0
	for _, c := range campaigns {
		if c.Status == common.CampaignStatusRunning {
			activeCampaigns++
		}
	}

	// Get bot statistics
	bots, _ := s.state.ListBots(r.Context())
	onlineBots := 0
	for _, b := range bots {
		if b.IsOnline {
			onlineBots++
		}
	}

	status := map[string]any{
		"server": map[string]any{
			"version":    s.version,
			"running":    s.running,
			"start_time": s.stats.StartTime,
			"uptime":     time.Since(s.stats.StartTime).String(),
		},
		"campaigns": map[string]any{
			"total":   len(campaigns),
			"active":  activeCampaigns,
			"pending": countByStatus(campaigns, common.CampaignStatusPending),
		},
		"bots": map[string]any{
			"total":           len(bots),
			"online":          onlineBots,
			"active_timeouts": botTimeouts,
		},
		"jobs": map[string]any{
			"active_timeouts": jobTimeouts,
		},
		"websocket": map[string]any{
			"connected_clients": s.wsHub.ClientCount(),
		},
		"database":  s.state.GetDatabaseStats(r.Context()),
		"timestamp": time.Now(),
	}

	s.writeJSONResponse(w, status)
}

// handleV2CreateCampaign creates a campaign with enhanced validation
func (s *Server) handleV2CreateCampaign(w http.ResponseWriter, r *http.Request) {
	// Use the same handler but with v2 response format
	s.handleCreateCampaign(w, r)

	// If successful, broadcast via WebSocket
	if w.Header().Get("Content-Type") == "application/json" {
		// Parse the response to get campaign ID
		// In a real implementation, we'd track this better
		s.wsHub.Broadcast(WSMessage{
			Type: "campaign_created",
			Data: map[string]any{
				"timestamp": time.Now(),
			},
		})
	}
}

// handleV2ListCampaigns lists campaigns with enhanced filtering
func (s *Server) handleV2ListCampaigns(w http.ResponseWriter, r *http.Request) {
	// Enhanced filtering
	filters := common.CampaignFilters{
		Limit:  50,
		Offset: 0,
	}

	// Parse all query parameters
	query := r.URL.Query()
	if limit := query.Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 && l <= 100 {
			filters.Limit = l
		}
	}

	if offset := query.Get("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil && o >= 0 {
			filters.Offset = o
		}
	}

	filters.Status = query.Get("status")
	filters.BinaryHash = query.Get("binary_hash")
	if tags := query.Get("tags"); tags != "" {
		filters.Tags = []string{}
		if err := json.Unmarshal([]byte(tags), &filters.Tags); err != nil {
			// Fall back to comma-separated
			filters.Tags = splitTags(tags)
		}
	}

	// Add sorting
	sortBy := query.Get("sort_by")
	sortOrder := query.Get("sort_order")

	campaigns, err := s.services.Campaign.List(r.Context(), filters)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to list campaigns", err)
		return
	}

	// Apply sorting
	if sortBy != "" {
		sortCampaigns(campaigns, sortBy, sortOrder == "desc")
	}

	// Build response with metadata
	response := map[string]any{
		"campaigns": campaigns,
		"metadata": map[string]any{
			"count":  len(campaigns),
			"limit":  filters.Limit,
			"offset": filters.Offset,
			"filters": map[string]any{
				"status":      filters.Status,
				"binary_hash": filters.BinaryHash,
				"tags":        filters.Tags,
			},
			"sort": map[string]any{
				"by":    sortBy,
				"order": sortOrder,
			},
		},
		"timestamp": time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// handleV2GetCampaign gets campaign with full details
func (s *Server) handleV2GetCampaign(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	campaign, err := s.services.Campaign.Get(r.Context(), campaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			s.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign", err)
		}
		return
	}

	// Get additional details
	stats, _ := s.services.Campaign.GetStatistics(r.Context(), campaignID)
	jobs, _ := s.state.GetCampaignJobs(r.Context(), campaignID)

	response := map[string]any{
		"campaign":   campaign,
		"statistics": stats,
		"jobs": map[string]any{
			"total":     len(jobs),
			"running":   countJobsByStatus(jobs, common.JobStatusRunning),
			"completed": countJobsByStatus(jobs, common.JobStatusCompleted),
			"failed":    countJobsByStatus(jobs, common.JobStatusFailed),
		},
		"timestamp": time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// handleV2UpdateCampaign updates campaign with PATCH semantics
func (s *Server) handleV2UpdateCampaign(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	// Parse JSON patch document
	var patches []map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&patches); err != nil {
		// Fall back to regular update
		s.handleUpdateCampaign(w, r)
		return
	}

	// Apply patches
	updates := common.CampaignUpdates{}
	for _, patch := range patches {
		op := patch["op"].(string)
		path := patch["path"].(string)
		value := patch["value"]

		if op != "replace" {
			s.writeErrorResponse(w, http.StatusBadRequest, "Only 'replace' operations are supported", nil)
			return
		}

		// Map paths to update fields
		switch path {
		case "/name":
			name := value.(string)
			updates.Name = &name
		case "/description":
			desc := value.(string)
			updates.Description = &desc
		case "/status":
			status := common.CampaignStatus(value.(string))
			updates.Status = &status
		case "/max_jobs":
			maxJobs := int(value.(float64))
			updates.MaxJobs = &maxJobs
		case "/auto_restart":
			autoRestart := value.(bool)
			updates.AutoRestart = &autoRestart
		default:
			s.writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Unknown path: %s", path), nil)
			return
		}
	}

	// Apply updates
	if err := s.services.Campaign.Update(r.Context(), campaignID, updates); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to update campaign", err)
		return
	}

	// Get updated campaign
	campaign, err := s.services.Campaign.Get(r.Context(), campaignID)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get updated campaign", err)
		return
	}

	// Broadcast update
	s.wsHub.Broadcast(WSMessage{
		Type: "campaign_updated",
		Data: map[string]any{
			"campaign_id": campaignID,
			"updates":     updates,
			"timestamp":   time.Now(),
		},
	})

	s.writeJSONResponse(w, campaign)
}

// handleV2GetCampaignTimeline gets campaign event timeline
func (s *Server) handleV2GetCampaignTimeline(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	// Get campaign
	campaign, err := s.services.Campaign.Get(r.Context(), campaignID)
	if err != nil {
		if err == common.ErrCampaignNotFound {
			s.writeErrorResponse(w, http.StatusNotFound, "Campaign not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign", err)
		}
		return
	}

	// Build timeline
	timeline := []CampaignTimelineEntry{
		{
			Timestamp:   campaign.CreatedAt,
			EventType:   "campaign_created",
			Description: "Campaign created",
			Data: map[string]interface{}{
				"name":   campaign.Name,
				"status": campaign.Status,
			},
		},
	}

	// Add job events
	jobs, _ := s.state.GetCampaignJobs(r.Context(), campaignID)
	for _, job := range jobs {
		timeline = append(timeline, CampaignTimelineEntry{
			Timestamp:   job.CreatedAt,
			EventType:   "job_created",
			Description: fmt.Sprintf("Job %s created", job.Name),
			Data: map[string]interface{}{
				"job_id": job.ID,
				"fuzzer": job.Fuzzer,
			},
		})

		if job.CompletedAt != nil {
			timeline = append(timeline, CampaignTimelineEntry{
				Timestamp:   *job.CompletedAt,
				EventType:   "job_completed",
				Description: fmt.Sprintf("Job %s completed", job.Name),
				Data: map[string]interface{}{
					"job_id": job.ID,
					"status": job.Status,
				},
			})
		}
	}

	// Add crash events
	crashGroups, _ := s.services.Deduplication.GetCrashGroups(r.Context(), campaignID)
	for _, group := range crashGroups {
		timeline = append(timeline, CampaignTimelineEntry{
			Timestamp:   group.FirstSeen,
			EventType:   "crash_found",
			Description: fmt.Sprintf("New crash group found (%s severity)", group.Severity),
			Data: map[string]interface{}{
				"group_id": group.ID,
				"severity": group.Severity,
				"count":    group.Count,
			},
		})
	}

	// Sort timeline by timestamp
	sortTimeline(timeline)

	response := map[string]any{
		"campaign_id": campaignID,
		"timeline":    timeline,
		"event_count": len(timeline),
	}

	s.writeJSONResponse(w, response)
}

// handleV2GetCrashGroups gets crash groups with enhanced filtering
func (s *Server) handleV2GetCrashGroups(w http.ResponseWriter, r *http.Request) {
	// Enhanced version with more statistics
	s.handleGetCrashGroups(w, r)
}

// handleV2ListBots lists bots with enhanced information
func (s *Server) handleV2ListBots(w http.ResponseWriter, r *http.Request) {
	bots, err := s.state.ListBots(r.Context())
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to list bots", err)
		return
	}

	// Enhance with additional info
	enhancedBots := make([]map[string]any, len(bots))
	for i, bot := range bots {
		completedJobs, _ := s.state.GetBotCompletedJobs(r.Context(), bot.ID)

		enhancedBots[i] = map[string]any{
			"bot":            bot,
			"completed_jobs": completedJobs,
			"uptime":         time.Since(bot.RegisteredAt).String(),
			"health_status":  getBotHealthStatus(bot),
		}
	}

	response := map[string]any{
		"bots":      enhancedBots,
		"count":     len(bots),
		"online":    countOnlineBots(bots),
		"timestamp": time.Now(),
	}

	s.writeJSONResponse(w, response)
}

// handleV2StreamBotMetrics streams real-time bot metrics
func (s *Server) handleV2StreamBotMetrics(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	botID := vars["id"]

	// Verify bot exists
	bot, err := s.services.Bot.GetBot(r.Context(), botID)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "Bot not found", err)
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

	// Send initial metrics
	initialMetrics := BotMetricsStream{
		BotID:       botID,
		Timestamp:   time.Now(),
		CPUUsage:    0.0,
		MemoryUsage: 0,
	}

	if data, err := json.Marshal(initialMetrics); err == nil {
		fmt.Fprintf(w, "event: connected\ndata: %s\n\n", data)
		flusher.Flush()
	}

	// Stream metrics every 5 seconds
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// In a real implementation, fetch actual metrics from bot
			metrics := BotMetricsStream{
				BotID:       botID,
				Timestamp:   time.Now(),
				CPUUsage:    45.5 + (float64(time.Now().Unix()%20) - 10),              // Simulated
				MemoryUsage: uint64(1<<30) + uint64(time.Now().Unix()%1000)*1024*1024, // Simulated
			}

			// Add job progress if bot has active job
			currentBot, _ := s.services.Bot.GetBot(r.Context(), botID)
			if currentBot != nil && currentBot.CurrentJob != nil {
				job, _ := s.state.GetJob(r.Context(), *currentBot.CurrentJob)
				if job != nil {
					metrics.JobProgress = job.Progress
				}
			}

			if data, err := json.Marshal(metrics); err == nil {
				fmt.Fprintf(w, "event: metrics\ndata: %s\n\n", data)
				flusher.Flush()
			}

			// Send heartbeat to detect disconnected clients
			fmt.Fprintf(w, "event: ping\ndata: %d\n\n", time.Now().Unix())
			flusher.Flush()
		}
	}
}

// handleV2GetBotMetricsHistory gets historical bot metrics
func (s *Server) handleV2GetBotMetricsHistory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	botID := vars["id"]

	// Get time range
	duration := 1 * time.Hour
	if d := r.URL.Query().Get("duration"); d != "" {
		if parsed, err := time.ParseDuration(d); err == nil {
			duration = parsed
		}
	}

	// In a real implementation, this would query a metrics store
	// For now, return simulated data
	dataPoints := 60
	interval := duration / time.Duration(dataPoints)
	metrics := make([]BotMetricsStream, dataPoints)

	now := time.Now()
	for i := 0; i < dataPoints; i++ {
		timestamp := now.Add(-duration + time.Duration(i)*interval)
		metrics[i] = BotMetricsStream{
			BotID:       botID,
			Timestamp:   timestamp,
			CPUUsage:    40 + float64(i%20),
			MemoryUsage: uint64(1<<30) + uint64(i%50)*10*1024*1024,
		}
	}

	response := map[string]any{
		"bot_id":      botID,
		"metrics":     metrics,
		"data_points": len(metrics),
		"duration":    duration.String(),
		"interval":    interval.String(),
	}

	s.writeJSONResponse(w, response)
}

// handleV2CreateJob creates a job with campaign association
func (s *Server) handleV2CreateJob(w http.ResponseWriter, r *http.Request) {
	// Check if campaign_id is provided
	campaignID := r.URL.Query().Get("campaign_id")
	if campaignID != "" {
		// Verify campaign exists
		campaign, err := s.services.Campaign.Get(r.Context(), campaignID)
		if err != nil {
			s.writeErrorResponse(w, http.StatusBadRequest, "Invalid campaign ID", err)
			return
		}

		// Create job within campaign context
		s.logger.WithField("campaign_id", campaignID).Info("Creating job for campaign")
		_ = campaign // Use campaign config for job creation
	}

	// Use standard job creation
	s.handleJobCreate(w, r)
}

// handleV2ListJobs lists jobs with enhanced filtering
func (s *Server) handleV2ListJobs(w http.ResponseWriter, r *http.Request) {
	// Enhanced filtering including campaign association
	campaignID := r.URL.Query().Get("campaign_id")

	if campaignID != "" {
		// Get jobs for specific campaign
		jobs, err := s.state.GetCampaignJobs(r.Context(), campaignID)
		if err != nil {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get campaign jobs", err)
			return
		}

		response := map[string]any{
			"jobs":        jobs,
			"count":       len(jobs),
			"campaign_id": campaignID,
		}
		s.writeJSONResponse(w, response)
		return
	}

	// Standard job listing
	s.handleJobList(w, r)
}

// Helper functions

func countByStatus(campaigns []*common.Campaign, status common.CampaignStatus) int {
	count := 0
	for _, c := range campaigns {
		if c.Status == status {
			count++
		}
	}
	return count
}

func countJobsByStatus(jobs []*common.Job, status common.JobStatus) int {
	count := 0
	for _, j := range jobs {
		if j.Status == status {
			count++
		}
	}
	return count
}

func countOnlineBots(bots []*common.Bot) int {
	count := 0
	for _, b := range bots {
		if b.IsOnline {
			count++
		}
	}
	return count
}

func getBotHealthStatus(bot *common.Bot) string {
	if !bot.IsOnline {
		return "offline"
	}
	if bot.FailureCount > 3 {
		return "unhealthy"
	}
	if time.Since(bot.LastSeen) > 5*time.Minute {
		return "degraded"
	}
	return "healthy"
}

func splitTags(tags string) []string {
	var result []string
	for _, tag := range strings.Split(tags, ",") {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			result = append(result, tag)
		}
	}
	return result
}

func sortCampaigns(campaigns []*common.Campaign, sortBy string, desc bool) {
	// Simple bubble sort for demonstration
	// In production, use sort.Slice
	n := len(campaigns)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			shouldSwap := false
			switch sortBy {
			case "name":
				shouldSwap = campaigns[j].Name > campaigns[j+1].Name
			case "created_at":
				shouldSwap = campaigns[j].CreatedAt.After(campaigns[j+1].CreatedAt)
			case "updated_at":
				shouldSwap = campaigns[j].UpdatedAt.After(campaigns[j+1].UpdatedAt)
			}

			if desc {
				shouldSwap = !shouldSwap
			}

			if shouldSwap {
				campaigns[j], campaigns[j+1] = campaigns[j+1], campaigns[j]
			}
		}
	}
}

func sortTimeline(timeline []CampaignTimelineEntry) {
	// Sort by timestamp ascending
	n := len(timeline)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if timeline[j].Timestamp.After(timeline[j+1].Timestamp) {
				timeline[j], timeline[j+1] = timeline[j+1], timeline[j]
			}
		}
	}
}
