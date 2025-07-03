package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// campaignService implements the CampaignService interface
type campaignService struct {
	storage    common.Storage
	jobService common.JobService
	logger     logrus.FieldLogger
}

// NewCampaignService creates a new campaign service instance
func NewCampaignService(storage common.Storage, jobService common.JobService, logger logrus.FieldLogger) common.CampaignService {
	return &campaignService{
		storage:    storage,
		jobService: jobService,
		logger:     logger.WithField("service", "campaign"),
	}
}

// Create creates a new campaign
func (cs *campaignService) Create(ctx context.Context, campaign *common.Campaign) error {
	// Validate campaign
	if campaign.Name == "" {
		return fmt.Errorf("campaign name is required")
	}
	if campaign.TargetBinary == "" {
		return fmt.Errorf("target binary is required")
	}
	if campaign.MaxJobs <= 0 {
		campaign.MaxJobs = 10 // Default
	}

	// Generate ID if not provided
	if campaign.ID == "" {
		campaign.ID = "campaign-" + uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	campaign.CreatedAt = now
	campaign.UpdatedAt = now

	// Set default status
	if campaign.Status == "" {
		campaign.Status = common.CampaignStatusPending
	}

	// Store in database
	if err := cs.storage.CreateCampaign(ctx, campaign); err != nil {
		return fmt.Errorf("failed to create campaign: %w", err)
	}

	cs.logger.WithFields(logrus.Fields{
		"campaign_id": campaign.ID,
		"name":        campaign.Name,
		"target":      campaign.TargetBinary,
	}).Info("Campaign created")

	// If auto-start is enabled, create initial jobs
	if campaign.Status == common.CampaignStatusRunning {
		if err := cs.createJobsForCampaign(ctx, campaign); err != nil {
			cs.logger.WithError(err).Error("Failed to create initial jobs")
			// Update campaign status to failed
			cs.storage.UpdateCampaign(ctx, campaign.ID, map[string]interface{}{
				"status": common.CampaignStatusFailed,
			})
			return err
		}
	}

	return nil
}

// Get retrieves a campaign by ID
func (cs *campaignService) Get(ctx context.Context, id string) (*common.Campaign, error) {
	campaign, err := cs.storage.GetCampaign(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get campaign: %w", err)
	}
	return campaign, nil
}

// List retrieves campaigns with filters
func (cs *campaignService) List(ctx context.Context, filters common.CampaignFilters) ([]*common.Campaign, error) {
	// Default pagination
	if filters.Limit <= 0 {
		filters.Limit = 50
	}
	if filters.Limit > 100 {
		filters.Limit = 100
	}

	campaigns, err := cs.storage.ListCampaigns(ctx, filters.Limit, filters.Offset, filters.Status)
	if err != nil {
		return nil, fmt.Errorf("failed to list campaigns: %w", err)
	}

	// Filter by tags if specified
	if len(filters.Tags) > 0 {
		filtered := make([]*common.Campaign, 0)
		for _, campaign := range campaigns {
			if hasAnyTag(campaign.Tags, filters.Tags) {
				filtered = append(filtered, campaign)
			}
		}
		campaigns = filtered
	}

	// Filter by binary hash if specified
	if filters.BinaryHash != "" {
		filtered := make([]*common.Campaign, 0)
		for _, campaign := range campaigns {
			if campaign.BinaryHash == filters.BinaryHash {
				filtered = append(filtered, campaign)
			}
		}
		campaigns = filtered
	}

	return campaigns, nil
}

// Update updates a campaign
func (cs *campaignService) Update(ctx context.Context, id string, updates common.CampaignUpdates) error {
	// Get existing campaign
	campaign, err := cs.storage.GetCampaign(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get campaign: %w", err)
	}

	// Build update map
	updateMap := make(map[string]interface{})

	if updates.Name != nil {
		updateMap["name"] = *updates.Name
	}
	if updates.Description != nil {
		updateMap["description"] = *updates.Description
	}
	if updates.Status != nil {
		// Validate status transition
		if err := cs.validateStatusTransition(campaign.Status, *updates.Status); err != nil {
			return err
		}
		updateMap["status"] = *updates.Status

		// Set completed_at if transitioning to completed
		if *updates.Status == common.CampaignStatusCompleted {
			now := time.Now()
			updateMap["completed_at"] = &now
		}
	}
	if updates.AutoRestart != nil {
		updateMap["auto_restart"] = *updates.AutoRestart
	}
	if updates.MaxDuration != nil {
		updateMap["max_duration"] = *updates.MaxDuration
	}
	if updates.MaxJobs != nil {
		updateMap["max_jobs"] = *updates.MaxJobs
	}
	if updates.SharedCorpus != nil {
		updateMap["shared_corpus"] = *updates.SharedCorpus
	}
	if len(updates.Tags) > 0 {
		updateMap["tags"] = updates.Tags
	}

	// Update in database
	if err := cs.storage.UpdateCampaign(ctx, id, updateMap); err != nil {
		return fmt.Errorf("failed to update campaign: %w", err)
	}

	cs.logger.WithFields(logrus.Fields{
		"campaign_id": id,
		"updates":     updateMap,
	}).Info("Campaign updated")

	// If status changed to running, create jobs
	if updates.Status != nil && *updates.Status == common.CampaignStatusRunning {
		campaign.Status = *updates.Status
		if err := cs.createJobsForCampaign(ctx, campaign); err != nil {
			cs.logger.WithError(err).Error("Failed to create jobs after status update")
		}
	}

	return nil
}

// Delete deletes a campaign
func (cs *campaignService) Delete(ctx context.Context, id string) error {
	// Get campaign to check if it's running
	campaign, err := cs.storage.GetCampaign(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get campaign: %w", err)
	}

	// Don't delete running campaigns
	if campaign.Status == common.CampaignStatusRunning {
		return fmt.Errorf("cannot delete running campaign")
	}

	// Cancel all associated jobs
	jobs, err := cs.storage.GetCampaignJobs(ctx, id)
	if err == nil {
		for _, job := range jobs {
			if job.Status == common.JobStatusRunning || job.Status == common.JobStatusPending {
				cs.jobService.UpdateJob(ctx, job.ID, map[string]interface{}{
					"status": common.JobStatusCancelled,
				})
			}
		}
	}

	// Delete campaign
	if err := cs.storage.DeleteCampaign(ctx, id); err != nil {
		return fmt.Errorf("failed to delete campaign: %w", err)
	}

	cs.logger.WithField("campaign_id", id).Info("Campaign deleted")
	return nil
}

// GetStatistics retrieves campaign statistics
func (cs *campaignService) GetStatistics(ctx context.Context, id string) (*common.CampaignStats, error) {
	stats, err := cs.storage.GetCampaignStatistics(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get campaign statistics: %w", err)
	}
	return stats, nil
}

// RestartCampaign restarts a completed campaign
func (cs *campaignService) RestartCampaign(ctx context.Context, id string) error {
	// Get campaign
	campaign, err := cs.storage.GetCampaign(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get campaign: %w", err)
	}

	// Check if campaign can be restarted
	if campaign.Status != common.CampaignStatusCompleted && campaign.Status != common.CampaignStatusFailed {
		return fmt.Errorf("can only restart completed or failed campaigns")
	}

	// Update campaign status to running
	if err := cs.storage.UpdateCampaign(ctx, id, map[string]interface{}{
		"status":       common.CampaignStatusRunning,
		"completed_at": nil,
	}); err != nil {
		return fmt.Errorf("failed to update campaign status: %w", err)
	}

	// Create new jobs
	campaign.Status = common.CampaignStatusRunning
	if err := cs.createJobsForCampaign(ctx, campaign); err != nil {
		// Revert status on error
		cs.storage.UpdateCampaign(ctx, id, map[string]interface{}{
			"status": common.CampaignStatusFailed,
		})
		return fmt.Errorf("failed to create jobs: %w", err)
	}

	cs.logger.WithField("campaign_id", id).Info("Campaign restarted")
	return nil
}

// createJobsForCampaign creates jobs for a campaign based on its configuration
func (cs *campaignService) createJobsForCampaign(ctx context.Context, campaign *common.Campaign) error {
	// Get current job count
	existingJobs, err := cs.storage.GetCampaignJobs(ctx, campaign.ID)
	if err != nil {
		return fmt.Errorf("failed to get existing jobs: %w", err)
	}

	// Calculate how many jobs to create
	activeJobs := 0
	for _, job := range existingJobs {
		if job.Status == common.JobStatusPending || job.Status == common.JobStatusRunning {
			activeJobs++
		}
	}

	jobsToCreate := campaign.MaxJobs - activeJobs
	if jobsToCreate <= 0 {
		return nil // Already at max jobs
	}

	// Create new jobs
	for i := 0; i < jobsToCreate; i++ {
		job := &common.Job{
			ID:        fmt.Sprintf("job-%s-%d-%s", campaign.ID, len(existingJobs)+i+1, uuid.New().String()),
			Name:      fmt.Sprintf("%s-job-%d", campaign.Name, len(existingJobs)+i+1),
			Target:    campaign.TargetBinary,
			Fuzzer:    "afl++", // Default fuzzer, could be in campaign config
			Status:    common.JobStatusPending,
			Config:    campaign.JobTemplate,
			WorkDir:   fmt.Sprintf("/tmp/pandafuzz/%s/%s", campaign.ID, uuid.New().String()),
			CreatedAt: time.Now(),
			TimeoutAt: time.Now().Add(campaign.JobTemplate.Duration),
		}

		// Create the job
		if err := cs.jobService.CreateJob(ctx, job); err != nil {
			cs.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to create job")
			continue
		}

		// Link job to campaign
		if err := cs.storage.LinkJobToCampaign(ctx, campaign.ID, job.ID); err != nil {
			cs.logger.WithError(err).WithFields(logrus.Fields{
				"campaign_id": campaign.ID,
				"job_id":      job.ID,
			}).Error("Failed to link job to campaign")
		}
	}

	return nil
}

// checkCampaignCompletion checks if all jobs in a campaign are completed
func (cs *campaignService) checkCampaignCompletion(ctx context.Context, campaignID string) error {
	// Get campaign
	campaign, err := cs.storage.GetCampaign(ctx, campaignID)
	if err != nil {
		return err
	}

	// Skip if not running
	if campaign.Status != common.CampaignStatusRunning {
		return nil
	}

	// Get all jobs
	jobs, err := cs.storage.GetCampaignJobs(ctx, campaignID)
	if err != nil {
		return err
	}

	// Check if all jobs are completed
	allCompleted := true
	hasFailures := false
	for _, job := range jobs {
		switch job.Status {
		case common.JobStatusPending, common.JobStatusRunning, common.JobStatusAssigned:
			allCompleted = false
		case common.JobStatusFailed, common.JobStatusTimedOut:
			hasFailures = true
		}
	}

	if !allCompleted {
		return nil
	}

	// Update campaign status
	newStatus := common.CampaignStatusCompleted
	if hasFailures {
		newStatus = common.CampaignStatusFailed
	}

	now := time.Now()
	if err := cs.storage.UpdateCampaign(ctx, campaignID, map[string]interface{}{
		"status":       newStatus,
		"completed_at": &now,
	}); err != nil {
		return err
	}

	cs.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"status":      newStatus,
	}).Info("Campaign completed")

	// Check if auto-restart is enabled
	if campaign.AutoRestart && newStatus == common.CampaignStatusCompleted {
		// Schedule restart
		go func() {
			time.Sleep(30 * time.Second) // Brief pause before restart
			if err := cs.RestartCampaign(context.Background(), campaignID); err != nil {
				cs.logger.WithError(err).Error("Failed to auto-restart campaign")
			}
		}()
	}

	return nil
}

// validateStatusTransition validates if a status transition is allowed
func (cs *campaignService) validateStatusTransition(from, to common.CampaignStatus) error {
	// Define allowed transitions
	transitions := map[common.CampaignStatus][]common.CampaignStatus{
		common.CampaignStatusPending:   {common.CampaignStatusRunning, common.CampaignStatusCancelled},
		common.CampaignStatusRunning:   {common.CampaignStatusPaused, common.CampaignStatusCompleted, common.CampaignStatusFailed},
		common.CampaignStatusPaused:    {common.CampaignStatusRunning, common.CampaignStatusCancelled},
		common.CampaignStatusCompleted: {common.CampaignStatusRunning}, // For restart
		common.CampaignStatusFailed:    {common.CampaignStatusRunning}, // For restart
	}

	allowed, ok := transitions[from]
	if !ok {
		return fmt.Errorf("invalid current status: %s", from)
	}

	for _, status := range allowed {
		if status == to {
			return nil
		}
	}

	return fmt.Errorf("invalid status transition from %s to %s", from, to)
}

// hasAnyTag checks if campaign has any of the specified tags
func hasAnyTag(campaignTags, filterTags []string) bool {
	tagMap := make(map[string]bool)
	for _, tag := range campaignTags {
		tagMap[tag] = true
	}

	for _, tag := range filterTags {
		if tagMap[tag] {
			return true
		}
	}

	return false
}
