package registry

import (
	"context"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
)

// HealthChecker manages health checking for registered bots
type HealthChecker struct {
	repo         repository.AgentRepository
	eventPub     types.BotEventPublisher
	staleTimeout time.Duration

	// Control
	stopCh chan struct{}
	wg     sync.WaitGroup
	mu     sync.Mutex

	// Health check state
	failureCounts map[string]int
	lastCheckTime map[string]time.Time
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(repo repository.AgentRepository, eventPub types.BotEventPublisher, staleTimeout time.Duration) *HealthChecker {
	return &HealthChecker{
		repo:          repo,
		eventPub:      eventPub,
		staleTimeout:  staleTimeout,
		failureCounts: make(map[string]int),
		lastCheckTime: make(map[string]time.Time),
	}
}

// Start begins the health checking process
func (hc *HealthChecker) Start(ctx context.Context, interval time.Duration) {
	hc.mu.Lock()
	if hc.stopCh != nil {
		hc.mu.Unlock()
		return // Already running
	}
	hc.stopCh = make(chan struct{})
	hc.mu.Unlock()

	hc.wg.Add(1)
	go hc.healthCheckLoop(ctx, interval)
}

// Stop halts the health checking process
func (hc *HealthChecker) Stop() {
	hc.mu.Lock()
	if hc.stopCh == nil {
		hc.mu.Unlock()
		return // Not running
	}
	close(hc.stopCh)
	hc.stopCh = nil
	hc.mu.Unlock()

	hc.wg.Wait()
}

// healthCheckLoop runs the periodic health check
func (hc *HealthChecker) healthCheckLoop(ctx context.Context, interval time.Duration) {
	defer hc.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run initial check
	hc.performHealthCheck(ctx)

	for {
		select {
		case <-ticker.C:
			hc.performHealthCheck(ctx)
		case <-hc.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// performHealthCheck checks the health of all registered bots
func (hc *HealthChecker) performHealthCheck(ctx context.Context) {
	// Find stale bots
	staleBots, err := hc.repo.FindStale(ctx, hc.staleTimeout)
	if err != nil {
		// In production, this would be logged
		return
	}

	// Process each stale bot
	for _, bot := range staleBots {
		hc.handleStaleBot(ctx, bot)
	}

	// Clean up bots that have recovered
	hc.cleanupRecoveredBots(ctx)
}

// handleStaleBot processes a bot that hasn't sent heartbeat
func (hc *HealthChecker) handleStaleBot(ctx context.Context, bot *types.Agent) {
	hc.mu.Lock()
	failureCount := hc.failureCounts[bot.ID] + 1
	hc.failureCounts[bot.ID] = failureCount
	hc.lastCheckTime[bot.ID] = time.Now()
	hc.mu.Unlock()

	// Determine if we should retry
	willRetry := failureCount < 3
	var nextRetry *time.Time
	if willRetry {
		t := time.Now().Add(time.Duration(failureCount) * time.Minute)
		nextRetry = &t
	}

	// Emit health check failed event
	event := types.NewBotHealthCheckFailedEvent(
		bot.ID,
		"bot heartbeat timeout",
		bot.LastHeartbeat,
		failureCount,
		willRetry,
		nextRetry,
	)
	_ = hc.eventPub.PublishEvent(event)

	// Update bot status based on failure count
	if failureCount >= 3 {
		// Mark as offline after 3 failures
		_ = hc.repo.UpdateStatus(ctx, bot.ID, types.StatusOffline)

		// Clear work assignment if bot was working
		if bot.Status == types.StatusWorking {
			// In production, this would trigger work reassignment
			hc.handleWorkingBotOffline(ctx, bot)
		}
	} else if bot.Status != types.StatusMaintenance {
		// Mark as error state for transient failures
		_ = hc.repo.UpdateStatus(ctx, bot.ID, types.StatusError)
	}
}

// handleWorkingBotOffline handles the case when a working bot goes offline
func (hc *HealthChecker) handleWorkingBotOffline(ctx context.Context, bot *types.Agent) {
	// Extract job information from metadata
	jobID, _ := bot.GetMetadata("current_job_id")
	if jobID != nil {
		// Emit work failed event
		event := types.NewBotWorkFailedEvent(
			bot.ID,
			jobID.(string),
			"bot went offline during work execution",
			"health check failure",
		)
		_ = hc.eventPub.PublishEvent(event)
	}
}

// cleanupRecoveredBots removes failure counts for bots that have recovered
func (hc *HealthChecker) cleanupRecoveredBots(ctx context.Context) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	// Get online bots
	onlineBots, err := hc.repo.FindOnline(ctx)
	if err != nil {
		return
	}

	// Create a map for quick lookup
	onlineMap := make(map[string]bool)
	for _, bot := range onlineBots {
		onlineMap[bot.ID] = true
	}

	// Remove failure counts for online bots
	for botID := range hc.failureCounts {
		if onlineMap[botID] {
			delete(hc.failureCounts, botID)
			delete(hc.lastCheckTime, botID)
		}
	}

	// Also clean up old entries (bots that have been removed)
	cutoff := time.Now().Add(-24 * time.Hour)
	for botID, lastCheck := range hc.lastCheckTime {
		if lastCheck.Before(cutoff) {
			delete(hc.failureCounts, botID)
			delete(hc.lastCheckTime, botID)
		}
	}
}

// GetHealthStatus returns the current health status of a bot
func (hc *HealthChecker) GetHealthStatus(botID string) *HealthStatus {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	return &HealthStatus{
		BotID:         botID,
		FailureCount:  hc.failureCounts[botID],
		LastCheckTime: hc.lastCheckTime[botID],
		IsHealthy:     hc.failureCounts[botID] == 0,
	}
}

// GetAllHealthStatuses returns health status for all monitored bots
func (hc *HealthChecker) GetAllHealthStatuses() map[string]*HealthStatus {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	statuses := make(map[string]*HealthStatus)
	for botID, failureCount := range hc.failureCounts {
		statuses[botID] = &HealthStatus{
			BotID:         botID,
			FailureCount:  failureCount,
			LastCheckTime: hc.lastCheckTime[botID],
			IsHealthy:     failureCount == 0,
		}
	}

	return statuses
}

// ResetBotHealth resets the health status for a specific bot
func (hc *HealthChecker) ResetBotHealth(botID string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	delete(hc.failureCounts, botID)
	delete(hc.lastCheckTime, botID)
}

// HealthStatus represents the health status of a bot
type HealthStatus struct {
	BotID         string    `json:"bot_id"`
	FailureCount  int       `json:"failure_count"`
	LastCheckTime time.Time `json:"last_check_time"`
	IsHealthy     bool      `json:"is_healthy"`
}
