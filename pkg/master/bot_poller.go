package master

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/sirupsen/logrus"
)

// BotPoller periodically polls bots for their job status
type BotPoller struct {
	state          *PersistentState
	services       *service.Manager
	httpClient     *http.Client
	logger         *logrus.Logger
	interval       time.Duration
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	running        bool
	mu             sync.RWMutex
	
	// Backoff configuration
	backoffConfig  BackoffConfig
	botBackoffs    map[string]*ExponentialBackoff // Track backoff per bot
}

// BackoffConfig defines backoff parameters
type BackoffConfig struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	MaxRetries      int
}

// ExponentialBackoff tracks backoff state
type ExponentialBackoff struct {
	interval    time.Duration
	maxInterval time.Duration
	multiplier  float64
	retries     int
	maxRetries  int
	lastAttempt time.Time
}

// BotJobStatus represents job status from bot API
type BotJobStatus struct {
	JobID      string    `json:"job_id"`
	Status     string    `json:"status"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time,omitempty"`
	Success    bool      `json:"success,omitempty"`
	Message    string    `json:"message,omitempty"`
	Output     string    `json:"output,omitempty"`
	CrashCount int       `json:"crash_count,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// BotHealthStatus represents bot health from API
type BotHealthStatus struct {
	Status        string    `json:"status"`
	BotID         string    `json:"bot_id"`
	CurrentJob    string    `json:"current_job,omitempty"`
	JobStatus     string    `json:"job_status,omitempty"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	Uptime        string    `json:"uptime"`
	Version       string    `json:"version"`
}

// NewBotPoller creates a new bot poller
func NewBotPoller(
	state *PersistentState,
	services *service.Manager,
	logger *logrus.Logger,
	interval time.Duration,
) *BotPoller {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &BotPoller{
		state:      state,
		services:   services,
		logger:     logger,
		interval:   interval,
		ctx:        ctx,
		cancel:     cancel,
		botBackoffs: make(map[string]*ExponentialBackoff),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		backoffConfig: BackoffConfig{
			InitialInterval: 1 * time.Second,
			MaxInterval:     60 * time.Second,
			Multiplier:      2.0,
			MaxRetries:      5,
		},
	}
}

// Start begins polling bots
func (p *BotPoller) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.running {
		return fmt.Errorf("bot poller already running")
	}
	
	p.running = true
	p.wg.Add(1)
	go p.pollLoop()
	
	p.logger.WithField("interval", p.interval).Info("Bot poller started")
	return nil
}

// Stop stops the bot poller
func (p *BotPoller) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if !p.running {
		return nil
	}
	
	p.cancel()
	p.wg.Wait()
	p.running = false
	
	p.logger.Info("Bot poller stopped")
	return nil
}

// pollLoop is the main polling loop
func (p *BotPoller) pollLoop() {
	defer p.wg.Done()
	
	// Use 5 seconds as minimum interval for more responsive polling
	interval := p.interval
	if interval > 5*time.Second {
		interval = 5 * time.Second
	}
	
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	// Initial poll
	p.pollAllBots()
	
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.pollAllBots()
		}
	}
}

// pollAllBots polls all registered bots
func (p *BotPoller) pollAllBots() {
	bots, err := p.services.Bot.ListBots(p.ctx, nil)
	if err != nil {
		p.logger.WithError(err).Error("Failed to list bots for polling")
		return
	}
	
	p.logger.WithField("bot_count", len(bots)).Debug("Starting bot polling cycle")
	
	// Poll each bot concurrently
	var wg sync.WaitGroup
	for _, bot := range bots {
		if bot.APIEndpoint == "" {
			p.logger.WithField("bot_id", bot.ID).Debug("Skipping bot without API endpoint")
			continue // Skip bots without API endpoints (legacy bots)
		}
		
		p.logger.WithFields(logrus.Fields{
			"bot_id": bot.ID,
			"api_endpoint": bot.APIEndpoint,
			"current_job": bot.CurrentJob,
		}).Debug("Polling bot")
		
		wg.Add(1)
		go func(b *common.Bot) {
			defer wg.Done()
			p.pollBot(b)
		}(bot)
	}
	
	wg.Wait()
}

// pollBot polls a single bot
func (p *BotPoller) pollBot(bot *common.Bot) {
	p.mu.RLock()
	backoff, exists := p.botBackoffs[bot.ID]
	p.mu.RUnlock()
	
	if !exists {
		backoff = p.newBackoff()
		p.mu.Lock()
		p.botBackoffs[bot.ID] = backoff
		p.mu.Unlock()
	}
	
	// Check if we should retry based on backoff
	if !backoff.shouldRetry() {
		return
	}
	
	// Poll health status
	health, err := p.pollBotHealth(bot)
	if err != nil {
		p.handlePollError(bot, backoff, err)
		return
	}
	
	// Reset backoff on success
	backoff.reset()
	
	// Update bot status based on health
	p.updateBotStatus(bot, health)
	
	// Poll job status if bot has a job
	if health.CurrentJob != "" {
		jobStatus, err := p.pollJobStatus(bot, health.CurrentJob)
		if err != nil {
			p.logger.WithError(err).WithFields(logrus.Fields{
				"bot_id": bot.ID,
				"job_id": health.CurrentJob,
			}).Warn("Failed to poll job status")
		} else {
			p.updateJobStatus(bot, jobStatus)
		}
	} else if bot.CurrentJob != nil {
		// Bot reports no current job but master thinks it has one
		// First check if the job is already completed in master's view
		job, err := p.services.Job.GetJob(p.ctx, *bot.CurrentJob)
		if err == nil && (job.Status == common.JobStatusCompleted || job.Status == common.JobStatusFailed || job.Status == common.JobStatusCancelled) {
			// Job is already completed, just need to clear bot's reference
			p.logger.WithFields(logrus.Fields{
				"bot_id":        bot.ID,
				"job_id":        *bot.CurrentJob,
				"job_status":    job.Status,
			}).Info("Clearing stale job reference from bot - job already completed")
			
			bot.CurrentJob = nil
			bot.Status = common.BotStatusIdle
			if err := p.state.SaveBotWithRetry(p.ctx, bot); err != nil {
				p.logger.WithError(err).WithField("bot_id", bot.ID).Error("Failed to clear bot's job reference")
			}
			return
		}
		
		// Job not completed in master's view, try to poll for status
		p.logger.WithFields(logrus.Fields{
			"bot_id":        bot.ID,
			"master_job_id": *bot.CurrentJob,
		}).Info("Bot reports no job but master has one assigned, checking job status")
		
		jobStatus, err := p.pollJobStatus(bot, *bot.CurrentJob)
		if err != nil {
			p.logger.WithError(err).WithFields(logrus.Fields{
				"bot_id": bot.ID,
				"job_id": *bot.CurrentJob,
			}).Debug("Job not found on bot, likely completed")
		} else {
			p.updateJobStatus(bot, jobStatus)
		}
	}
}

// pollBotHealth polls bot's health endpoint
func (p *BotPoller) pollBotHealth(bot *common.Bot) (*BotHealthStatus, error) {
	url := fmt.Sprintf("%s/api/v1/health", bot.APIEndpoint)
	
	resp, err := p.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}
	
	var health BotHealthStatus
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, err
	}
	
	return &health, nil
}

// pollJobStatus polls bot's job status endpoint
func (p *BotPoller) pollJobStatus(bot *common.Bot, jobID string) (*BotJobStatus, error) {
	url := fmt.Sprintf("%s/api/v1/job/%s/status", bot.APIEndpoint, jobID)
	
	p.logger.WithFields(logrus.Fields{
		"bot_id": bot.ID,
		"job_id": jobID,
		"url":    url,
	}).Debug("Polling job status from bot")
	
	resp, err := p.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("job status check failed: status %d", resp.StatusCode)
	}
	
	var status BotJobStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	
	p.logger.WithFields(logrus.Fields{
		"bot_id":     bot.ID,
		"job_id":     jobID,
		"job_status": status.Status,
		"success":    status.Success,
	}).Debug("Received job status from bot")
	
	return &status, nil
}

// updateBotStatus updates bot status based on health check
func (p *BotPoller) updateBotStatus(bot *common.Bot, health *BotHealthStatus) {
	// Check if bot is reconnecting after being marked as timed out
	wasTimedOut := bot.Status == common.BotStatusTimedOut
	
	// Update bot heartbeat
	status := common.BotStatusIdle
	if health.CurrentJob != "" {
		status = common.BotStatusBusy
	}
	
	if health.Status == "unhealthy" {
		status = common.BotStatusFailed
	}
	
	err := p.services.Bot.UpdateHeartbeat(p.ctx, bot.ID, status, &health.CurrentJob)
	if err != nil {
		p.logger.WithError(err).WithField("bot_id", bot.ID).Error("Failed to update bot heartbeat")
		return
	}
	
	// Handle reconnection after timeout
	if wasTimedOut {
		p.handleBotReconnection(bot, health)
	}
}

// updateJobStatus updates job status based on bot's report
func (p *BotPoller) updateJobStatus(bot *common.Bot, jobStatus *BotJobStatus) {
	// Process completed, failed, or pending_ack jobs
	if jobStatus.Status != "completed" && jobStatus.Status != "failed" && jobStatus.Status != "pending_ack" {
		return
	}
	
	// Get the job from master's perspective first
	job, err := p.services.Job.GetJob(p.ctx, jobStatus.JobID)
	if err != nil {
		p.logger.WithError(err).WithFields(logrus.Fields{
			"bot_id": bot.ID,
			"job_id": jobStatus.JobID,
		}).Warn("Failed to get job for status update")
		return
	}
	
	// Check if job is already completed/failed in master's view
	if job.Status == common.JobStatusCompleted || job.Status == common.JobStatusCancelled {
		p.logger.WithFields(logrus.Fields{
			"bot_id":      bot.ID,
			"job_id":      jobStatus.JobID,
			"master_status": job.Status,
			"bot_status":   jobStatus.Status,
		}).Debug("Job already completed in master's view")
		return
	}
	
	// If job is unassigned or assigned to different bot, but this bot completed it,
	// we should update it if the bot actually did the work
	if job.AssignedBot == nil || (job.AssignedBot != nil && *job.AssignedBot != bot.ID) {
		p.logger.WithFields(logrus.Fields{
			"bot_id":       bot.ID,
			"job_id":       jobStatus.JobID,
			"assigned_bot": job.AssignedBot,
			"bot_status":   jobStatus.Status,
		}).Warn("Job assignment mismatch, attempting to reconcile")
		
		// If the bot has proof it completed the job (has end time), update the job
		if !jobStatus.EndTime.IsZero() && jobStatus.Status == "completed" {
			// Check if bot reported crashes but master doesn't have them
			if jobStatus.CrashCount > 0 {
				p.fetchMissingCrashes(bot, jobStatus)
			}
			
			// Directly update the job status
			job.Status = common.JobStatusCompleted
			job.AssignedBot = &bot.ID
			now := time.Now()
			job.CompletedAt = &now
			
			if err := p.state.SaveJobWithRetry(p.ctx, job); err != nil {
				p.logger.WithError(err).Error("Failed to save reconciled job status")
				return
			}
			
			// Clear bot's current job
			bot.CurrentJob = nil
			bot.Status = common.BotStatusIdle
			if err := p.state.SaveBotWithRetry(p.ctx, bot); err != nil {
				p.logger.WithError(err).Error("Failed to update bot after job reconciliation")
			}
			
			p.logger.WithFields(logrus.Fields{
				"bot_id":  bot.ID,
				"job_id":  jobStatus.JobID,
				"status":  jobStatus.Status,
				"crashes": jobStatus.CrashCount,
				"message": "Reconciled job completion from bot polling",
			}).Info("Job status reconciled via polling")
			return
		}
	}
	
	// Handle pending acknowledgment jobs
	if jobStatus.Status == "pending_ack" {
		p.logger.WithFields(logrus.Fields{
			"bot_id": bot.ID,
			"job_id": jobStatus.JobID,
		}).Info("Found job pending acknowledgment, completing it now")
		
		// Check if bot reported crashes but master doesn't have them
		if jobStatus.CrashCount > 0 {
			p.fetchMissingCrashes(bot, jobStatus)
		}
		
		// Complete the job
		err = p.services.Job.CompleteJob(p.ctx, jobStatus.JobID, bot.ID, jobStatus.Success)
		if err != nil {
			p.logger.WithError(err).WithFields(logrus.Fields{
				"bot_id": bot.ID,
				"job_id": jobStatus.JobID,
			}).Error("Failed to complete pending job from poll")
			return
		}
		
		p.logger.WithFields(logrus.Fields{
			"bot_id":  bot.ID,
			"job_id":  jobStatus.JobID,
			"success": jobStatus.Success,
			"crashes": jobStatus.CrashCount,
			"message": jobStatus.Message,
		}).Info("Pending job completed via polling")
		
		// TODO: Notify bot that the job is now acknowledged
		return
	}
	
	// Check if bot reported crashes but master doesn't have them
	if jobStatus.Status == "completed" && jobStatus.CrashCount > 0 {
		p.fetchMissingCrashes(bot, jobStatus)
	}
	
	// Determine success based on status
	success := jobStatus.Status == "completed" && jobStatus.Success
	
	// Try normal completion path
	err = p.services.Job.CompleteJob(p.ctx, jobStatus.JobID, bot.ID, success)
	if err != nil {
		p.logger.WithError(err).WithFields(logrus.Fields{
			"bot_id": bot.ID,
			"job_id": jobStatus.JobID,
		}).Error("Failed to complete job from poll")
		return
	}
	
	p.logger.WithFields(logrus.Fields{
		"bot_id":  bot.ID,
		"job_id":  jobStatus.JobID,
		"status":  jobStatus.Status,
		"success": success,
		"crashes": jobStatus.CrashCount,
		"message": jobStatus.Message,
	}).Info("Job status updated via polling")
}

// handlePollError handles polling errors with backoff
func (p *BotPoller) handlePollError(bot *common.Bot, backoff *ExponentialBackoff, err error) {
	backoff.recordFailure()
	
	p.logger.WithError(err).WithFields(logrus.Fields{
		"bot_id":  bot.ID,
		"retries": backoff.retries,
		"backoff": backoff.interval,
	}).Warn("Failed to poll bot")
	
	// If max retries exceeded, mark bot as timed out
	if backoff.retries >= backoff.maxRetries {
		p.markBotUnreachable(bot)
	}
}

// markBotUnreachable marks a bot as unreachable and handles its jobs
func (p *BotPoller) markBotUnreachable(bot *common.Bot) {
	p.logger.WithField("bot_id", bot.ID).Error("Bot is unreachable, marking as timed out")
	
	// Update bot status to timed out
	bot.Status = common.BotStatusTimedOut
	bot.IsOnline = false
	
	if err := p.state.SaveBotWithRetry(p.ctx, bot); err != nil {
		p.logger.WithError(err).Error("Failed to save bot timeout status")
		return
	}
	
	// Handle current job if any
	if bot.CurrentJob != nil {
		job, err := p.services.Job.GetJob(p.ctx, *bot.CurrentJob)
		if err != nil {
			p.logger.WithError(err).Error("Failed to get job for timeout handling")
			return
		}
		
		// Mark job as errored
		job.Status = common.JobStatusFailed
		if err := p.state.SaveJobWithRetry(p.ctx, job); err != nil {
			p.logger.WithError(err).Error("Failed to save job error status")
		}
		
		// Free bot's job assignment
		bot.CurrentJob = nil
		if err := p.state.SaveBotWithRetry(p.ctx, bot); err != nil {
			p.logger.WithError(err).Error("Failed to clear bot job assignment")
		}
	}
}

// newBackoff creates a new backoff instance
func (p *BotPoller) newBackoff() *ExponentialBackoff {
	return &ExponentialBackoff{
		interval:    p.backoffConfig.InitialInterval,
		maxInterval: p.backoffConfig.MaxInterval,
		multiplier:  p.backoffConfig.Multiplier,
		maxRetries:  p.backoffConfig.MaxRetries,
		retries:     0,
	}
}

// shouldRetry checks if we should retry based on backoff
func (b *ExponentialBackoff) shouldRetry() bool {
	if b.retries >= b.maxRetries {
		return false
	}
	
	if time.Since(b.lastAttempt) < b.interval {
		return false
	}
	
	return true
}

// recordFailure records a failure and updates backoff
func (b *ExponentialBackoff) recordFailure() {
	b.retries++
	b.lastAttempt = time.Now()
	
	// Exponential backoff
	b.interval = time.Duration(float64(b.interval) * b.multiplier)
	if b.interval > b.maxInterval {
		b.interval = b.maxInterval
	}
}

// reset resets the backoff
func (b *ExponentialBackoff) reset() {
	b.retries = 0
	b.interval = time.Duration(0)
	b.lastAttempt = time.Time{}
}

// handleBotReconnection handles a bot reconnecting after timeout
func (p *BotPoller) handleBotReconnection(bot *common.Bot, health *BotHealthStatus) {
	p.logger.WithFields(logrus.Fields{
		"bot_id":      bot.ID,
		"current_job": health.CurrentJob,
		"job_status":  health.JobStatus,
	}).Info("Bot reconnected after timeout")
	
	// Reset backoff for this bot
	p.mu.Lock()
	if backoff, exists := p.botBackoffs[bot.ID]; exists {
		backoff.reset()
	}
	p.mu.Unlock()
	
	// If bot has a job, check if we need to reconcile
	if health.CurrentJob != "" {
		p.reconcileJobStatus(bot, health.CurrentJob)
	}
	
	// Check for any errored jobs that were assigned to this bot
	p.checkErroredJobs(bot)
}

// reconcileJobStatus reconciles job status after bot reconnection
func (p *BotPoller) reconcileJobStatus(bot *common.Bot, jobID string) {
	// Get job from master's perspective
	job, err := p.services.Job.GetJob(p.ctx, jobID)
	if err != nil {
		p.logger.WithError(err).WithFields(logrus.Fields{
			"bot_id": bot.ID,
			"job_id": jobID,
		}).Error("Failed to get job for reconciliation")
		return
	}
	
	// If job is marked as failed but bot still has it, poll for actual status
	if job.Status == common.JobStatusFailed || job.Status == common.JobStatusPending {
		jobStatus, err := p.pollJobStatus(bot, jobID)
		if err != nil {
			p.logger.WithError(err).Warn("Failed to poll job status during reconciliation")
			return
		}
		
		p.logger.WithFields(logrus.Fields{
			"bot_id":      bot.ID,
			"job_id":      jobID,
			"job_status":  jobStatus.Status,
			"master_view": job.Status,
		}).Info("Reconciling job status")
		
		// Update job status based on bot's report
		p.updateJobStatus(bot, jobStatus)
	}
}

// checkErroredJobs checks for errored jobs that were assigned to this bot
func (p *BotPoller) checkErroredJobs(bot *common.Bot) {
	// Query all jobs that might have been errored when bot timed out
	failedStatus := common.JobStatusFailed
	filter := service.JobFilter{
		Status: &failedStatus,
	}
	
	jobs, err := p.services.Job.ListJobs(p.ctx, filter)
	if err != nil {
		p.logger.WithError(err).Error("Failed to list jobs for error checking")
		return
	}
	
	// Check each failed job to see if it was assigned to this bot
	for _, job := range jobs {
		if job.AssignedBot != nil && *job.AssignedBot == bot.ID {
			// Poll bot for actual job status
			jobStatus, err := p.pollJobStatus(bot, job.ID)
			if err != nil {
				continue // Job might not exist on bot anymore
			}
			
			if jobStatus.Status == "completed" {
				p.logger.WithFields(logrus.Fields{
					"bot_id":  bot.ID,
					"job_id":  job.ID,
					"success": jobStatus.Success,
				}).Info("Found completed job that was marked as errored")
				
				// Update job status
				p.updateJobStatus(bot, jobStatus)
			}
		}
	}
}

// fetchMissingCrashes fetches crash data from bot when master doesn't have it
func (p *BotPoller) fetchMissingCrashes(bot *common.Bot, jobStatus *BotJobStatus) {
	// First check how many crashes we have for this job
	crashes, err := p.state.GetJobCrashes(p.ctx, jobStatus.JobID)
	if err != nil {
		p.logger.WithError(err).WithFields(logrus.Fields{
			"bot_id": bot.ID,
			"job_id": jobStatus.JobID,
		}).Error("Failed to get job crashes")
		return
	}
	
	// If we already have all crashes, nothing to do
	if len(crashes) >= jobStatus.CrashCount {
		p.logger.WithFields(logrus.Fields{
			"bot_id":        bot.ID,
			"job_id":        jobStatus.JobID,
			"have_crashes":  len(crashes),
			"bot_reported": jobStatus.CrashCount,
		}).Debug("Already have all crashes")
		return
	}
	
	p.logger.WithFields(logrus.Fields{
		"bot_id":        bot.ID,
		"job_id":        jobStatus.JobID,
		"have_crashes":  len(crashes),
		"bot_reported": jobStatus.CrashCount,
		"missing":      jobStatus.CrashCount - len(crashes),
	}).Info("Fetching missing crashes from bot")
	
	// Get job crashes from bot
	url := fmt.Sprintf("%s/api/v1/jobs/%s/crashes", bot.APIEndpoint, jobStatus.JobID)
	
	resp, err := p.httpClient.Get(url)
	if err != nil {
		p.logger.WithError(err).WithFields(logrus.Fields{
			"bot_id": bot.ID,
			"job_id": jobStatus.JobID,
			"url":    url,
		}).Error("Failed to fetch crashes from bot")
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		p.logger.WithFields(logrus.Fields{
			"bot_id": bot.ID,
			"job_id": jobStatus.JobID,
			"status": resp.StatusCode,
		}).Error("Bot returned error for crash fetch")
		return
	}
	
	var botCrashes []common.CrashResult
	if err := json.NewDecoder(resp.Body).Decode(&botCrashes); err != nil {
		p.logger.WithError(err).Error("Failed to decode crash data from bot")
		return
	}
	
	// Process each crash
	processed := 0
	for _, crash := range botCrashes {
		// Check if we already have this crash
		exists := false
		for _, existing := range crashes {
			if existing.Hash == crash.Hash {
				exists = true
				break
			}
		}
		
		if !exists {
			// Set timestamp if not provided
			if crash.Timestamp.IsZero() {
				crash.Timestamp = time.Now()
			}
			
			// Process the crash
			if err := p.state.ProcessCrashResultWithRetry(p.ctx, &crash); err != nil {
				p.logger.WithError(err).WithFields(logrus.Fields{
					"crash_id": crash.ID,
					"hash":     crash.Hash,
				}).Error("Failed to process fetched crash")
			} else {
				processed++
			}
		}
	}
	
	p.logger.WithFields(logrus.Fields{
		"bot_id":    bot.ID,
		"job_id":    jobStatus.JobID,
		"fetched":   len(botCrashes),
		"processed": processed,
	}).Info("Fetched and processed missing crashes")
}