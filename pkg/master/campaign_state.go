package master

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// CampaignStateManager manages campaign states in memory
type CampaignStateManager struct {
	campaigns map[string]*CampaignState
	mu        sync.RWMutex
	storage   common.Storage
	jobSvc    common.JobService
	corpusSvc common.CorpusService
	dedupSvc  common.DeduplicationService
	wsHub     *WSHub
	logger    logrus.FieldLogger

	// Control channels
	stopCh chan struct{}
	doneCh chan struct{}
}

// CampaignState represents the runtime state of a campaign
type CampaignState struct {
	Campaign      *common.Campaign
	ActiveJobs    map[string]*common.Job
	CompletedJobs map[string]*common.Job
	LastUpdate    time.Time
	Metrics       *CampaignMetrics
	mu            sync.RWMutex
}

// CampaignMetrics represents real-time campaign metrics
type CampaignMetrics struct {
	TotalJobs      int        `json:"total_jobs"`
	ActiveJobs     int        `json:"active_jobs"`
	CompletedJobs  int        `json:"completed_jobs"`
	FailedJobs     int        `json:"failed_jobs"`
	TotalCrashes   int        `json:"total_crashes"`
	UniqueCrashes  int        `json:"unique_crashes"`
	CorpusSize     int64      `json:"corpus_size"`
	TotalCoverage  int64      `json:"total_coverage"`
	ExecPerSecond  float64    `json:"exec_per_second"`
	LastCrashTime  *time.Time `json:"last_crash_time,omitempty"`
	LastUpdateTime time.Time  `json:"last_update_time"`
}

// NewCampaignStateManager creates a new campaign state manager
func NewCampaignStateManager(
	storage common.Storage,
	jobSvc common.JobService,
	corpusSvc common.CorpusService,
	dedupSvc common.DeduplicationService,
	wsHub *WSHub,
	logger logrus.FieldLogger,
) *CampaignStateManager {
	return &CampaignStateManager{
		campaigns: make(map[string]*CampaignState),
		storage:   storage,
		jobSvc:    jobSvc,
		corpusSvc: corpusSvc,
		dedupSvc:  dedupSvc,
		wsHub:     wsHub,
		logger:    logger.WithField("component", "campaign_state_manager"),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
}

// Start starts the campaign state manager
func (csm *CampaignStateManager) Start(ctx context.Context) error {
	csm.logger.Info("Starting campaign state manager")

	// Load existing campaigns
	if err := csm.loadCampaigns(ctx); err != nil {
		return fmt.Errorf("failed to load campaigns: %w", err)
	}

	// Start monitoring goroutine
	go csm.monitorCampaigns(ctx)

	return nil
}

// Stop stops the campaign state manager
func (csm *CampaignStateManager) Stop() error {
	csm.logger.Info("Stopping campaign state manager")

	// Signal stop
	close(csm.stopCh)

	// Wait for monitoring to complete
	select {
	case <-csm.doneCh:
		csm.logger.Info("Campaign state manager stopped")
	case <-time.After(5 * time.Second):
		csm.logger.Warn("Campaign state manager stop timeout")
	}

	return nil
}

// loadCampaigns loads all campaigns from storage
func (csm *CampaignStateManager) loadCampaigns(ctx context.Context) error {
	campaigns, err := csm.storage.ListCampaigns(ctx, 1000, 0, "")
	if err != nil {
		return err
	}

	csm.mu.Lock()
	defer csm.mu.Unlock()

	for _, campaign := range campaigns {
		// Only track active campaigns
		if campaign.Status == common.CampaignStatusRunning ||
			campaign.Status == common.CampaignStatusPending {

			state := &CampaignState{
				Campaign:      campaign,
				ActiveJobs:    make(map[string]*common.Job),
				CompletedJobs: make(map[string]*common.Job),
				LastUpdate:    time.Now(),
				Metrics:       &CampaignMetrics{LastUpdateTime: time.Now()},
			}

			// Load jobs for campaign
			jobs, err := csm.storage.GetCampaignJobs(ctx, campaign.ID)
			if err != nil {
				csm.logger.WithError(err).WithField("campaign_id", campaign.ID).Error("Failed to load campaign jobs")
				continue
			}

			// Categorize jobs
			for _, job := range jobs {
				switch job.Status {
				case common.JobStatusRunning, common.JobStatusAssigned:
					state.ActiveJobs[job.ID] = job
				case common.JobStatusCompleted, common.JobStatusFailed:
					state.CompletedJobs[job.ID] = job
				}
			}

			csm.campaigns[campaign.ID] = state
			csm.logger.WithField("campaign_id", campaign.ID).Info("Loaded campaign state")
		}
	}

	csm.logger.WithField("campaign_count", len(csm.campaigns)).Info("Loaded campaign states")
	return nil
}

// monitorCampaigns monitors campaign states and updates metrics
func (csm *CampaignStateManager) monitorCampaigns(ctx context.Context) {
	defer close(csm.doneCh)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-csm.stopCh:
			return
		case <-ticker.C:
			csm.updateAllCampaigns(ctx)
		}
	}
}

// updateAllCampaigns updates all campaign states
func (csm *CampaignStateManager) updateAllCampaigns(ctx context.Context) {
	csm.mu.RLock()
	campaignIDs := make([]string, 0, len(csm.campaigns))
	for id := range csm.campaigns {
		campaignIDs = append(campaignIDs, id)
	}
	csm.mu.RUnlock()

	for _, campaignID := range campaignIDs {
		if err := csm.updateCampaignState(ctx, campaignID); err != nil {
			csm.logger.WithError(err).WithField("campaign_id", campaignID).Error("Failed to update campaign state")
		}
	}
}

// updateCampaignState updates a single campaign state
func (csm *CampaignStateManager) updateCampaignState(ctx context.Context, campaignID string) error {
	csm.mu.RLock()
	state, exists := csm.campaigns[campaignID]
	csm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("campaign state not found: %s", campaignID)
	}

	// Update campaign from storage
	campaign, err := csm.storage.GetCampaign(ctx, campaignID)
	if err != nil {
		return err
	}

	state.mu.Lock()
	state.Campaign = campaign
	state.mu.Unlock()

	// Update metrics
	if err := csm.updateMetrics(ctx, campaignID); err != nil {
		csm.logger.WithError(err).Error("Failed to update metrics")
	}

	// Check for completion
	if err := csm.checkCampaignCompletion(ctx, campaignID); err != nil {
		csm.logger.WithError(err).Error("Failed to check campaign completion")
	}

	// Check auto-restart
	if campaign.AutoRestart && campaign.Status == common.CampaignStatusCompleted {
		csm.checkAutoRestart(ctx, campaignID)
	}

	return nil
}

// updateMetrics updates campaign metrics
func (csm *CampaignStateManager) updateMetrics(ctx context.Context, campaignID string) error {
	csm.mu.RLock()
	state, exists := csm.campaigns[campaignID]
	csm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("campaign state not found")
	}

	// Get fresh job list
	jobs, err := csm.storage.GetCampaignJobs(ctx, campaignID)
	if err != nil {
		return err
	}

	// Update job states
	state.mu.Lock()
	state.ActiveJobs = make(map[string]*common.Job)
	state.CompletedJobs = make(map[string]*common.Job)

	metrics := &CampaignMetrics{
		LastUpdateTime: time.Now(),
	}

	for _, job := range jobs {
		metrics.TotalJobs++

		switch job.Status {
		case common.JobStatusRunning, common.JobStatusAssigned:
			state.ActiveJobs[job.ID] = job
			metrics.ActiveJobs++
		case common.JobStatusCompleted:
			state.CompletedJobs[job.ID] = job
			metrics.CompletedJobs++
		case common.JobStatusFailed, common.JobStatusTimedOut:
			state.CompletedJobs[job.ID] = job
			metrics.FailedJobs++
		}
	}

	// Get crash statistics
	crashGroups, err := csm.dedupSvc.GetCrashGroups(ctx, campaignID)
	if err == nil {
		metrics.UniqueCrashes = len(crashGroups)
		for _, group := range crashGroups {
			metrics.TotalCrashes += group.Count
			if metrics.LastCrashTime == nil || group.LastSeen.After(*metrics.LastCrashTime) {
				metrics.LastCrashTime = &group.LastSeen
			}
		}
	}

	// Get corpus statistics
	evolution, err := csm.corpusSvc.GetEvolution(ctx, campaignID)
	if err == nil && len(evolution) > 0 {
		latest := evolution[len(evolution)-1]
		metrics.CorpusSize = latest.TotalSize
		metrics.TotalCoverage = latest.TotalCoverage
	}

	// Calculate exec/s (aggregate from all active jobs)
	// This would require more detailed job metrics in a real implementation
	metrics.ExecPerSecond = float64(metrics.ActiveJobs) * 1000.0 // Placeholder

	state.Metrics = metrics
	state.LastUpdate = time.Now()
	state.mu.Unlock()

	// Broadcast metrics update
	csm.broadcastMetricsUpdate(campaignID, metrics)

	return nil
}

// checkCampaignCompletion checks if all jobs in a campaign are completed
func (csm *CampaignStateManager) checkCampaignCompletion(ctx context.Context, campaignID string) error {
	csm.mu.RLock()
	state, exists := csm.campaigns[campaignID]
	csm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("campaign state not found")
	}

	state.mu.RLock()
	campaign := state.Campaign
	activeJobs := len(state.ActiveJobs)
	state.mu.RUnlock()

	// Skip if not running
	if campaign.Status != common.CampaignStatusRunning {
		return nil
	}

	// Check if all jobs are completed
	if activeJobs == 0 && state.Metrics.TotalJobs > 0 {
		// Update campaign status
		newStatus := common.CampaignStatusCompleted
		if state.Metrics.FailedJobs > state.Metrics.CompletedJobs {
			newStatus = common.CampaignStatusFailed
		}

		updates := map[string]interface{}{
			"status":       newStatus,
			"completed_at": time.Now(),
		}

		if err := csm.storage.UpdateCampaign(ctx, campaignID, updates); err != nil {
			return err
		}

		csm.logger.WithFields(logrus.Fields{
			"campaign_id": campaignID,
			"status":      newStatus,
		}).Info("Campaign completed")

		// Broadcast completion
		csm.wsHub.Broadcast(WSMessage{
			Type: WSTypeCampaignCompleted,
			Data: map[string]interface{}{
				"campaign_id": campaignID,
				"status":      newStatus,
				"metrics":     state.Metrics,
			},
			Timestamp: time.Now(),
		})
	}

	return nil
}

// checkAutoRestart checks if a campaign should be auto-restarted
func (csm *CampaignStateManager) checkAutoRestart(ctx context.Context, campaignID string) {
	csm.mu.RLock()
	state, exists := csm.campaigns[campaignID]
	csm.mu.RUnlock()

	if !exists {
		return
	}

	state.mu.RLock()
	campaign := state.Campaign
	state.mu.RUnlock()

	if !campaign.AutoRestart || campaign.Status != common.CampaignStatusCompleted {
		return
	}

	// Check if enough time has passed since completion
	if campaign.CompletedAt != nil && time.Since(*campaign.CompletedAt) < 30*time.Second {
		return
	}

	csm.logger.WithField("campaign_id", campaignID).Info("Auto-restarting campaign")

	// Schedule restart
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Update status to running
		if err := csm.storage.UpdateCampaign(ctx, campaignID, map[string]interface{}{
			"status":       common.CampaignStatusRunning,
			"completed_at": nil,
		}); err != nil {
			csm.logger.WithError(err).Error("Failed to restart campaign")
			return
		}

		// Refresh state
		csm.updateCampaignState(ctx, campaignID)
	}()
}

// handleJobCompletion handles job completion events
func (csm *CampaignStateManager) handleJobCompletion(jobID string) {
	// Find campaign for this job
	csm.mu.RLock()
	var campaignID string
	for cID, state := range csm.campaigns {
		state.mu.RLock()
		if _, exists := state.ActiveJobs[jobID]; exists {
			campaignID = cID
		}
		state.mu.RUnlock()
		if campaignID != "" {
			break
		}
	}
	csm.mu.RUnlock()

	if campaignID == "" {
		return
	}

	// Update campaign state
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := csm.updateCampaignState(ctx, campaignID); err != nil {
		csm.logger.WithError(err).Error("Failed to update campaign state after job completion")
	}
}

// broadcastMetricsUpdate broadcasts campaign metrics update
func (csm *CampaignStateManager) broadcastMetricsUpdate(campaignID string, metrics *CampaignMetrics) {
	msg := WSMessage{
		Type: "campaign_metrics_update",
		Data: map[string]interface{}{
			"campaign_id": campaignID,
			"metrics":     metrics,
		},
		Timestamp: time.Now(),
	}

	csm.wsHub.BroadcastToTopic("campaign:"+campaignID, msg)
}

// GetCampaignState retrieves the current state of a campaign
func (csm *CampaignStateManager) GetCampaignState(campaignID string) (*CampaignState, error) {
	csm.mu.RLock()
	defer csm.mu.RUnlock()

	state, exists := csm.campaigns[campaignID]
	if !exists {
		return nil, fmt.Errorf("campaign state not found: %s", campaignID)
	}

	return state, nil
}

// AddCampaign adds a new campaign to state management
func (csm *CampaignStateManager) AddCampaign(campaign *common.Campaign) {
	csm.mu.Lock()
	defer csm.mu.Unlock()

	state := &CampaignState{
		Campaign:      campaign,
		ActiveJobs:    make(map[string]*common.Job),
		CompletedJobs: make(map[string]*common.Job),
		LastUpdate:    time.Now(),
		Metrics:       &CampaignMetrics{LastUpdateTime: time.Now()},
	}

	csm.campaigns[campaign.ID] = state
	csm.logger.WithField("campaign_id", campaign.ID).Info("Added campaign to state management")
}

// RemoveCampaign removes a campaign from state management
func (csm *CampaignStateManager) RemoveCampaign(campaignID string) {
	csm.mu.Lock()
	defer csm.mu.Unlock()

	delete(csm.campaigns, campaignID)
	csm.logger.WithField("campaign_id", campaignID).Info("Removed campaign from state management")
}

// GetActiveCampaigns returns all active campaigns
func (csm *CampaignStateManager) GetActiveCampaigns() []*CampaignState {
	csm.mu.RLock()
	defer csm.mu.RUnlock()

	campaigns := make([]*CampaignState, 0, len(csm.campaigns))
	for _, state := range csm.campaigns {
		state.mu.RLock()
		if state.Campaign.Status == common.CampaignStatusRunning {
			campaigns = append(campaigns, state)
		}
		state.mu.RUnlock()
	}

	return campaigns
}

// GetMetrics returns aggregated metrics for all campaigns
func (csm *CampaignStateManager) GetMetrics() map[string]interface{} {
	csm.mu.RLock()
	defer csm.mu.RUnlock()

	totalActive := 0
	totalJobs := 0
	totalCrashes := 0
	totalCoverage := int64(0)

	for _, state := range csm.campaigns {
		state.mu.RLock()
		if state.Campaign.Status == common.CampaignStatusRunning {
			totalActive++
		}
		if state.Metrics != nil {
			totalJobs += state.Metrics.TotalJobs
			totalCrashes += state.Metrics.TotalCrashes
			totalCoverage += state.Metrics.TotalCoverage
		}
		state.mu.RUnlock()
	}

	return map[string]interface{}{
		"total_campaigns":  len(csm.campaigns),
		"active_campaigns": totalActive,
		"total_jobs":       totalJobs,
		"total_crashes":    totalCrashes,
		"total_coverage":   totalCoverage,
		"last_update":      time.Now(),
	}
}
