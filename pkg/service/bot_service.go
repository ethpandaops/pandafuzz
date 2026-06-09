package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// botService implements BotService interface
type botService struct {
	botRepo        BotRepository
	jobRepo        JobRepository // For GetCurrentJob
	timeoutManager TimeoutManager
	config         *common.MasterConfig
	logger         *logrus.Logger

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
}

// Compile-time interface compliance check
var _ BotService = (*botService)(nil)

// NewBotService creates a new bot service using repository interfaces.
func NewBotService(
	botRepo BotRepository,
	jobRepo JobRepository,
	timeoutManager TimeoutManager,
	config *common.MasterConfig,
	logger *logrus.Logger,
) BotService {
	return &botService{
		botRepo:        botRepo,
		jobRepo:        jobRepo,
		timeoutManager: timeoutManager,
		config:         config,
		logger:         logger,
	}
}

// RegisterBot registers a new bot
func (s *botService) RegisterBot(ctx context.Context, hostname string, name string, capabilities []string, apiEndpoint string) (*common.Bot, error) {
	// Validate input
	if hostname == "" {
		return nil, errors.NewValidationError("register_bot", "Hostname is required")
	}
	if len(capabilities) == 0 {
		return nil, errors.NewValidationError("register_bot", "Capabilities are required")
	}
	if apiEndpoint == "" {
		return nil, errors.NewValidationError("register_bot", "API endpoint is required")
	}

	// Create new bot
	botID := uuid.New().String()
	now := time.Now()
	timeout := now.Add(s.config.Timeouts.BotHeartbeat)

	// Use provided name or default to hostname
	botName := name
	if botName == "" {
		botName = hostname
	}

	bot := &common.Bot{
		ID:           botID,
		Name:         botName,
		Hostname:     hostname,
		Status:       common.BotStatusIdle,
		LastSeen:     now,
		RegisteredAt: now,
		Capabilities: capabilities,
		TimeoutAt:    timeout,
		IsOnline:     true,
		FailureCount: 0,
		APIEndpoint:  apiEndpoint,
	}

	// Save bot using repository
	if err := s.botRepo.Create(ctx, bot); err != nil {
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "register_bot", "Failed to save bot", err)
	}

	// Set timeout
	s.timeoutManager.SetBotTimeout(botID, s.config.Timeouts.BotHeartbeat)

	s.logger.WithFields(logrus.Fields{
		"bot_id":       botID,
		"name":         botName,
		"hostname":     hostname,
		"capabilities": capabilities,
	}).Info("Bot registered successfully")

	return bot, nil
}

// GetBot retrieves a bot by ID
func (s *botService) GetBot(ctx context.Context, botID string) (*common.Bot, error) {
	if botID == "" {
		return nil, errors.NewValidationError("get_bot", "Bot ID is required")
	}

	bot, err := s.botRepo.Get(ctx, botID)
	if err != nil {
		if common.IsNotFoundError(err) {
			return nil, errors.NewNotFoundError("get_bot", "bot")
		}
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "get_bot", "Failed to get bot", err)
	}

	return bot, nil
}

// DeleteBot removes a bot from the system
func (s *botService) DeleteBot(ctx context.Context, botID string) error {
	if botID == "" {
		return errors.NewValidationError("delete_bot", "Bot ID is required")
	}

	// Remove timeout
	s.timeoutManager.RemoveBotTimeout(botID)

	// Delete bot using repository
	if err := s.botRepo.Delete(ctx, botID); err != nil {
		return errors.Wrap(errors.ErrorTypeDatabase, "delete_bot", "Failed to delete bot", err)
	}

	s.logger.WithField("bot_id", botID).Info("Bot deregistered successfully")
	return nil
}

// UpdateHeartbeat updates bot heartbeat
func (s *botService) UpdateHeartbeat(ctx context.Context, botID string, status common.BotStatus, currentJob *string) error {
	if botID == "" {
		return errors.NewValidationError("update_heartbeat", "Bot ID is required")
	}

	// Use repository heartbeat update
	if err := s.botRepo.UpdateHeartbeat(ctx, botID, status, currentJob); err != nil {
		return errors.Wrap(errors.ErrorTypeDatabase, "update_heartbeat", "Failed to update bot heartbeat", err)
	}

	// Update timeout
	s.timeoutManager.UpdateBotHeartbeat(botID)

	s.logger.WithFields(logrus.Fields{
		"bot_id": botID,
		"status": status,
	}).Debug("Bot heartbeat received")

	return nil
}

// ListBots returns all bots, optionally filtered by status
func (s *botService) ListBots(ctx context.Context, statusFilter *common.BotStatus) ([]*common.Bot, error) {
	// Use status filter if provided
	if statusFilter != nil {
		return s.botRepo.ListByStatus(ctx, *statusFilter)
	}

	bots, err := s.botRepo.List(ctx)
	if err != nil {
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "list_bots", "Failed to list bots", err)
	}

	return bots, nil
}

// GetAvailableBot finds an available bot for job assignment
func (s *botService) GetAvailableBot(ctx context.Context, requiredCapabilities []string) (*common.Bot, error) {
	// Try to use optimized query
	bot, err := s.botRepo.FindAvailableWithCapabilities(ctx, requiredCapabilities)
	if err == nil {
		return bot, nil
	}

	// If not found error, return appropriate message
	if common.IsNotFoundError(err) {
		return nil, errors.New(errors.ErrorTypeCapability, "get_available_bot",
			fmt.Sprintf("No available bot with required capabilities: %v", requiredCapabilities))
	}

	// Fallback to listing all bots
	bots, listErr := s.botRepo.List(ctx)
	if listErr != nil {
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "get_available_bot", "Failed to list bots", listErr)
	}

	// Find available bot with required capabilities
	for _, b := range bots {
		// Check if bot is available
		if b.Status != common.BotStatusIdle || !b.IsOnline {
			continue
		}

		// Check capabilities
		if hasRequiredCapabilities(b.Capabilities, requiredCapabilities) {
			return b, nil
		}
	}

	return nil, errors.New(errors.ErrorTypeCapability, "get_available_bot",
		fmt.Sprintf("No available bot with required capabilities: %v", requiredCapabilities))
}

// hasRequiredCapabilities checks if bot has all required capabilities
func hasRequiredCapabilities(botCapabilities, requiredCapabilities []string) bool {
	capMap := make(map[string]bool)
	for _, cap := range botCapabilities {
		capMap[cap] = true
	}

	for _, required := range requiredCapabilities {
		if !capMap[required] {
			return false
		}
	}

	return true
}

// Start starts the bot service
func (s *botService) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Start heartbeat monitoring goroutine
	go s.monitorBotHeartbeats()

	s.logger.Info("Bot service started")
	return nil
}

// Stop stops the bot service
func (s *botService) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}

	// Clean up any resources
	// Currently bot service doesn't have resources to clean up

	s.logger.Info("Bot service stopped")
	return nil
}

// DeregisterBot deregisters a bot
func (s *botService) DeregisterBot(ctx context.Context, botID string) error {
	// Same as DeleteBot
	return s.DeleteBot(ctx, botID)
}

// Heartbeat updates bot heartbeat (simple version)
func (s *botService) Heartbeat(ctx context.Context, botID string) error {
	// Get current bot to maintain status
	bot, err := s.GetBot(ctx, botID)
	if err != nil {
		return err
	}
	return s.UpdateHeartbeat(ctx, botID, bot.Status, bot.CurrentJob)
}

// GetCurrentJob retrieves the current job for a bot
func (s *botService) GetCurrentJob(ctx context.Context, botID string) (*common.Job, error) {
	if botID == "" {
		return nil, errors.NewValidationError("get_current_job", "Bot ID is required")
	}

	bot, err := s.GetBot(ctx, botID)
	if err != nil {
		return nil, err
	}

	if bot.CurrentJob == nil || *bot.CurrentJob == "" {
		return nil, nil
	}

	// Get job details using job repository
	if s.jobRepo == nil {
		return nil, errors.New(errors.ErrorTypeMethodNotFound, "get_current_job", "Job repository not available")
	}

	return s.jobRepo.Get(ctx, *bot.CurrentJob)
}

// GetMetrics retrieves metrics for a bot
func (s *botService) GetMetrics(ctx context.Context, botID string) (*BotMetrics, error) {
	if botID == "" {
		return nil, errors.NewValidationError("get_metrics", "Bot ID is required")
	}

	// Create basic metrics from available data
	bot, err := s.GetBot(ctx, botID)
	if err != nil {
		return nil, err
	}

	metrics := &BotMetrics{
		BotID:      botID,
		LastActive: bot.LastSeen,
		// Other fields would need to be populated from job history
		// This is a simplified implementation
	}

	return metrics, nil
}

// monitorBotHeartbeats monitors bot heartbeats in the background
func (s *botService) monitorBotHeartbeats() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Check for offline bots
			bots, err := s.botRepo.List(s.ctx)
			if err != nil {
				s.logger.WithError(err).Error("Failed to list bots for heartbeat monitoring")
				continue
			}

			now := time.Now()
			// Batch process bot timeout checks
			var timedOutBots []string
			for _, bot := range bots {
				if bot.IsOnline && now.After(bot.TimeoutAt) {
					timedOutBots = append(timedOutBots, bot.ID)
					s.logger.WithFields(logrus.Fields{
						"bot_id":     bot.ID,
						"last_seen":  bot.LastSeen,
						"timeout_at": bot.TimeoutAt,
					}).Warn("Bot missed heartbeat deadline")
				}
			}

			// Batch update timed out bots
			if len(timedOutBots) > 0 {
				ctxWithTimeout, cancel := context.WithTimeout(s.ctx, 10*time.Second)
				if err := s.botRepo.BatchUpdateStatus(ctxWithTimeout, timedOutBots, common.BotStatusTimedOut); err != nil {
					s.logger.WithError(err).Error("Failed to batch update bot statuses")
				}
				cancel()
			}
		}
	}
}
