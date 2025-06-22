package master

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// TimeoutManager handles timeout tracking and enforcement
type TimeoutManager struct {
	state         *PersistentState
	botTimeouts   map[string]time.Time
	jobTimeouts   map[string]time.Time
	mu            sync.RWMutex
	logger        *logrus.Logger
	config        *common.MasterConfig
	ticker        *time.Ticker
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	retryManager  *common.RetryManager
	stats         TimeoutStats
}

// TimeoutStats tracks timeout-related statistics
type TimeoutStats struct {
	BotsTimedOut         int64     `json:"bots_timed_out"`
	JobsTimedOut         int64     `json:"jobs_timed_out"`
	TimeoutChecks        int64     `json:"timeout_checks"`
	LastCheck            time.Time `json:"last_check"`
	AverageCheckDuration time.Duration `json:"average_check_duration"`
	ActiveBotTimeouts    int       `json:"active_bot_timeouts"`
	ActiveJobTimeouts    int       `json:"active_job_timeouts"`
}

// TimeoutEvent represents a timeout event
type TimeoutEvent struct {
	Type      TimeoutType `json:"type"`
	EntityID  string      `json:"entity_id"`
	Timestamp time.Time   `json:"timestamp"`
	Duration  time.Duration `json:"duration"`
	Reason    string      `json:"reason"`
}

// TimeoutType represents the type of timeout
type TimeoutType string

const (
	TimeoutTypeBot TimeoutType = "bot"
	TimeoutTypeJob TimeoutType = "job"
)

// TimeoutCallback is called when a timeout occurs
type TimeoutCallback func(event TimeoutEvent) error

// NewTimeoutManager creates a new timeout manager
func NewTimeoutManager(state *PersistentState, config *common.MasterConfig) *TimeoutManager {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	
	ctx, cancel := context.WithCancel(context.Background())
	
	retryPolicy := config.Retry.BotOperation
	if retryPolicy.MaxRetries == 0 {
		retryPolicy = common.DefaultRetryPolicy
	}
	
	return &TimeoutManager{
		state:        state,
		botTimeouts:  make(map[string]time.Time),
		jobTimeouts:  make(map[string]time.Time),
		logger:       logger,
		config:       config,
		ctx:          ctx,
		cancel:       cancel,
		retryManager: common.NewRetryManager(retryPolicy),
	}
}

// Start begins the timeout monitoring process
func (tm *TimeoutManager) Start() error {
	tm.logger.Info("Starting timeout manager")
	
	// Start with an initial check
	if err := tm.initializeTimeouts(); err != nil {
		return common.NewSystemError("start_timeout_manager", err)
	}
	
	// Initialize LastCheck to current time to prevent health check failures on startup
	tm.mu.Lock()
	tm.stats.LastCheck = time.Now()
	tm.mu.Unlock()
	
	// Create ticker for periodic checks (every 30 seconds)
	tm.ticker = time.NewTicker(30 * time.Second)
	
	// Start background goroutine
	tm.wg.Add(1)
	go tm.run()
	
	tm.logger.Info("Timeout manager started successfully")
	return nil
}

// Stop gracefully shuts down the timeout manager
func (tm *TimeoutManager) Stop() error {
	tm.logger.Info("Stopping timeout manager")
	
	// Cancel context and stop ticker
	tm.cancel()
	if tm.ticker != nil {
		tm.ticker.Stop()
	}
	
	// Wait for background goroutine to finish
	tm.wg.Wait()
	
	tm.logger.Info("Timeout manager stopped")
	return nil
}

// run is the main loop for timeout checking
func (tm *TimeoutManager) run() {
	defer tm.wg.Done()
	
	for {
		select {
		case <-tm.ctx.Done():
			return
		case <-tm.ticker.C:
			tm.performTimeoutCheck()
		}
	}
}

// initializeTimeouts loads existing timeouts from persistent state
func (tm *TimeoutManager) initializeTimeouts() error {
	tm.logger.Debug("Initializing timeouts from persistent state")
	
	// Load bot timeouts
	bots, err := tm.state.ListBots()
	if err != nil {
		return common.NewSystemError("load_bot_timeouts", err)
	}
	
	tm.mu.Lock()
	for _, bot := range bots {
		tm.botTimeouts[bot.ID] = bot.TimeoutAt
	}
	tm.mu.Unlock()
	
	// Load job timeouts
	jobs, err := tm.state.ListJobs()
	if err != nil {
		return common.NewSystemError("load_job_timeouts", err)
	}
	
	tm.mu.Lock()
	for _, job := range jobs {
		tm.jobTimeouts[job.ID] = job.TimeoutAt
	}
	tm.mu.Unlock()
	
	tm.logger.WithFields(logrus.Fields{
		"bot_timeouts": len(tm.botTimeouts),
		"job_timeouts": len(tm.jobTimeouts),
	}).Debug("Timeouts initialized")
	
	return nil
}

// performTimeoutCheck checks for and handles timeouts
func (tm *TimeoutManager) performTimeoutCheck() {
	start := time.Now()
	
	tm.logger.Debug("Performing timeout check")
	
	// Check bot timeouts
	timedOutBots := tm.checkBotTimeouts()
	for _, botID := range timedOutBots {
		if err := tm.handleBotTimeout(botID); err != nil {
			tm.logger.WithError(err).WithField("bot_id", botID).Error("Failed to handle bot timeout")
		}
	}
	
	// Check job timeouts
	timedOutJobs := tm.checkJobTimeouts()
	for _, jobID := range timedOutJobs {
		if err := tm.handleJobTimeout(jobID); err != nil {
			tm.logger.WithError(err).WithField("job_id", jobID).Error("Failed to handle job timeout")
		}
	}
	
	// Update statistics
	duration := time.Since(start)
	tm.mu.Lock()
	tm.stats.TimeoutChecks++
	tm.stats.LastCheck = time.Now()
	tm.stats.AverageCheckDuration = (tm.stats.AverageCheckDuration + duration) / 2
	tm.stats.ActiveBotTimeouts = len(tm.botTimeouts)
	tm.stats.ActiveJobTimeouts = len(tm.jobTimeouts)
	tm.mu.Unlock()
	
	if len(timedOutBots) > 0 || len(timedOutJobs) > 0 {
		tm.logger.WithFields(logrus.Fields{
			"timed_out_bots": len(timedOutBots),
			"timed_out_jobs": len(timedOutJobs),
			"check_duration": duration,
		}).Info("Timeout check completed")
	}
}

// checkBotTimeouts returns bot IDs that have timed out
func (tm *TimeoutManager) checkBotTimeouts() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	now := time.Now()
	var timedOut []string
	
	for botID, timeout := range tm.botTimeouts {
		if now.After(timeout) {
			timedOut = append(timedOut, botID)
		}
	}
	
	return timedOut
}

// checkJobTimeouts returns job IDs that have timed out
func (tm *TimeoutManager) checkJobTimeouts() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	now := time.Now()
	var timedOut []string
	
	for jobID, timeout := range tm.jobTimeouts {
		if now.After(timeout) {
			timedOut = append(timedOut, jobID)
		}
	}
	
	return timedOut
}

// handleBotTimeout handles a bot timeout
func (tm *TimeoutManager) handleBotTimeout(botID string) error {
	tm.logger.WithField("bot_id", botID).Warn("Bot timeout detected")
	
	err := tm.retryManager.Execute(func() error {
		// Get bot information
		bot, err := tm.state.GetBot(botID)
		if err != nil {
			return err
		}
		
		// Create timeout event
		event := TimeoutEvent{
			Type:      TimeoutTypeBot,
			EntityID:  botID,
			Timestamp: time.Now(),
			Duration:  time.Since(bot.LastSeen),
			Reason:    "Heartbeat timeout",
		}
		
		// Reset the bot state
		if err := tm.state.ResetBot(botID); err != nil {
			return err
		}
		
		// Remove from timeout tracking
		tm.mu.Lock()
		delete(tm.botTimeouts, botID)
		tm.stats.BotsTimedOut++
		tm.mu.Unlock()
		
		tm.logger.WithFields(logrus.Fields{
			"bot_id":      botID,
			"hostname":    bot.Hostname,
			"last_seen":   bot.LastSeen,
			"timeout_at":  bot.TimeoutAt,
			"current_job": bot.CurrentJob,
		}).Info("Bot timeout handled")
		
		// Log the timeout event
		return tm.logTimeoutEvent(event)
	})
	
	if err != nil {
		return common.NewSystemError("handle_bot_timeout", err)
	}
	
	return nil
}

// handleJobTimeout handles a job timeout
func (tm *TimeoutManager) handleJobTimeout(jobID string) error {
	tm.logger.WithField("job_id", jobID).Warn("Job timeout detected")
	
	err := tm.retryManager.Execute(func() error {
		// Get job information
		job, err := tm.state.GetJob(jobID)
		if err != nil {
			return err
		}
		
		// Create timeout event
		event := TimeoutEvent{
			Type:      TimeoutTypeJob,
			EntityID:  jobID,
			Timestamp: time.Now(),
			Duration:  time.Since(job.CreatedAt),
			Reason:    "Execution timeout",
		}
		
		// Update job status to timed out
		if job.AssignedBot != nil {
			// Complete the job as failed to free up the bot
			if err := tm.state.CompleteJobWithRetry(jobID, *job.AssignedBot, false); err != nil {
				return err
			}
		}
		
		// Remove from timeout tracking
		tm.mu.Lock()
		delete(tm.jobTimeouts, jobID)
		tm.stats.JobsTimedOut++
		tm.mu.Unlock()
		
		tm.logger.WithFields(logrus.Fields{
			"job_id":       jobID,
			"job_name":     job.Name,
			"fuzzer":       job.Fuzzer,
			"created_at":   job.CreatedAt,
			"timeout_at":   job.TimeoutAt,
			"assigned_bot": job.AssignedBot,
		}).Info("Job timeout handled")
		
		// Log the timeout event
		return tm.logTimeoutEvent(event)
	})
	
	if err != nil {
		return common.NewSystemError("handle_job_timeout", err)
	}
	
	return nil
}

// SetBotTimeout sets a timeout for a bot
func (tm *TimeoutManager) SetBotTimeout(botID string, timeout time.Duration) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	timeoutAt := time.Now().Add(timeout)
	tm.botTimeouts[botID] = timeoutAt
	
	tm.logger.WithFields(logrus.Fields{
		"bot_id":     botID,
		"timeout":    timeout,
		"timeout_at": timeoutAt,
	}).Debug("Bot timeout set")
}

// SetJobTimeout sets a timeout for a job
func (tm *TimeoutManager) SetJobTimeout(jobID string, timeout time.Duration) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	timeoutAt := time.Now().Add(timeout)
	tm.jobTimeouts[jobID] = timeoutAt
	
	tm.logger.WithFields(logrus.Fields{
		"job_id":     jobID,
		"timeout":    timeout,
		"timeout_at": timeoutAt,
	}).Debug("Job timeout set")
}

// ExtendBotTimeout extends a bot's timeout
func (tm *TimeoutManager) ExtendBotTimeout(botID string, extension time.Duration) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	if existingTimeout, exists := tm.botTimeouts[botID]; exists {
		newTimeout := existingTimeout.Add(extension)
		tm.botTimeouts[botID] = newTimeout
		
		tm.logger.WithFields(logrus.Fields{
			"bot_id":      botID,
			"extension":   extension,
			"new_timeout": newTimeout,
		}).Debug("Bot timeout extended")
	}
}

// ExtendJobTimeout extends a job's timeout
func (tm *TimeoutManager) ExtendJobTimeout(jobID string, extension time.Duration) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	if existingTimeout, exists := tm.jobTimeouts[jobID]; exists {
		newTimeout := existingTimeout.Add(extension)
		tm.jobTimeouts[jobID] = newTimeout
		
		tm.logger.WithFields(logrus.Fields{
			"job_id":      jobID,
			"extension":   extension,
			"new_timeout": newTimeout,
		}).Debug("Job timeout extended")
	}
}

// RemoveBotTimeout removes a bot's timeout
func (tm *TimeoutManager) RemoveBotTimeout(botID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	delete(tm.botTimeouts, botID)
	
	tm.logger.WithField("bot_id", botID).Debug("Bot timeout removed")
}

// RemoveJobTimeout removes a job's timeout
func (tm *TimeoutManager) RemoveJobTimeout(jobID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	delete(tm.jobTimeouts, jobID)
	
	tm.logger.WithField("job_id", jobID).Debug("Job timeout removed")
}

// GetBotTimeout returns the timeout for a bot
func (tm *TimeoutManager) GetBotTimeout(botID string) (time.Time, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	timeout, exists := tm.botTimeouts[botID]
	return timeout, exists
}

// GetJobTimeout returns the timeout for a job
func (tm *TimeoutManager) GetJobTimeout(jobID string) (time.Time, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	timeout, exists := tm.jobTimeouts[jobID]
	return timeout, exists
}

// GetActiveTimeouts returns counts of active timeouts
func (tm *TimeoutManager) GetActiveTimeouts() (int, int) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	return len(tm.botTimeouts), len(tm.jobTimeouts)
}

// logTimeoutEvent logs a timeout event to persistent storage
func (tm *TimeoutManager) logTimeoutEvent(event TimeoutEvent) error {
	// Store timeout event in metadata for audit purposes
	key := fmt.Sprintf("timeout_event_%s_%d", event.EntityID, event.Timestamp.Unix())
	return tm.state.SetMetadata(key, event)
}

// GetStats returns timeout manager statistics
func (tm *TimeoutManager) GetStats() interface{} {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	stats := tm.stats
	stats.ActiveBotTimeouts = len(tm.botTimeouts)
	stats.ActiveJobTimeouts = len(tm.jobTimeouts)
	
	return stats
}

// GetStatsTyped returns typed timeout manager statistics
func (tm *TimeoutManager) GetStatsTyped() TimeoutStats {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	stats := tm.stats
	stats.ActiveBotTimeouts = len(tm.botTimeouts)
	stats.ActiveJobTimeouts = len(tm.jobTimeouts)
	
	return stats
}

// ListBotTimeouts returns all bot timeouts
func (tm *TimeoutManager) ListBotTimeouts() map[string]time.Time {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	result := make(map[string]time.Time)
	for botID, timeout := range tm.botTimeouts {
		result[botID] = timeout
	}
	
	return result
}

// ListJobTimeouts returns all job timeouts
func (tm *TimeoutManager) ListJobTimeouts() map[string]time.Time {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	result := make(map[string]time.Time)
	for jobID, timeout := range tm.jobTimeouts {
		result[jobID] = timeout
	}
	
	return result
}

// IsExpiredBot checks if a bot has timed out
func (tm *TimeoutManager) IsExpiredBot(botID string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	if timeout, exists := tm.botTimeouts[botID]; exists {
		return time.Now().After(timeout)
	}
	
	return false
}

// IsExpiredJob checks if a job has timed out
func (tm *TimeoutManager) IsExpiredJob(jobID string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	if timeout, exists := tm.jobTimeouts[jobID]; exists {
		return time.Now().After(timeout)
	}
	
	return false
}

// UpdateBotHeartbeat updates a bot's timeout based on heartbeat
func (tm *TimeoutManager) UpdateBotHeartbeat(botID string) {
	heartbeatTimeout := tm.config.Timeouts.BotHeartbeat
	tm.SetBotTimeout(botID, heartbeatTimeout)
}

// UpdateJobTimeout updates a job's timeout based on execution time
func (tm *TimeoutManager) UpdateJobTimeout(jobID string) {
	executionTimeout := tm.config.Timeouts.JobExecution
	tm.SetJobTimeout(jobID, executionTimeout)
}

// ForceTimeout immediately triggers a timeout for an entity (string version for interface)
func (tm *TimeoutManager) ForceTimeout(entityType string, entityID string) error {
	var tType TimeoutType
	switch entityType {
	case "bot":
		tType = TimeoutTypeBot
	case "job":
		tType = TimeoutTypeJob
	default:
		return common.NewValidationError("force_timeout", fmt.Errorf("invalid timeout type: %s", entityType))
	}
	return tm.ForceTimeoutTyped(tType, entityID)
}

// ForceTimeoutTyped immediately triggers a timeout for an entity with typed parameter
func (tm *TimeoutManager) ForceTimeoutTyped(entityType TimeoutType, entityID string) error {
	switch entityType {
	case TimeoutTypeBot:
		return tm.handleBotTimeout(entityID)
	case TimeoutTypeJob:
		return tm.handleJobTimeout(entityID)
	default:
		return common.NewValidationError("force_timeout", fmt.Errorf("invalid timeout type: %s", entityType))
	}
}

// HealthCheck verifies the timeout manager is functioning correctly
func (tm *TimeoutManager) HealthCheck() error {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	// If LastCheck is zero, the timeout manager hasn't started yet - this is OK during startup
	if tm.stats.LastCheck.IsZero() {
		return nil
	}
	
	// Check if we've performed a recent timeout check
	if time.Since(tm.stats.LastCheck) > 2*time.Minute {
		return common.NewSystemError("timeout_manager_health", fmt.Errorf("no recent timeout checks"))
	}
	
	return nil
}