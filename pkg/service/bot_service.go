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
	state          StateStore
	timeoutManager TimeoutManager
	config         *common.MasterConfig
	logger         *logrus.Logger
}

// NewBotService creates a new bot service
func NewBotService(
	state StateStore,
	timeoutManager TimeoutManager,
	config *common.MasterConfig,
	logger *logrus.Logger,
) BotService {
	return &botService{
		state:          state,
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

	// Save bot with retry
	if err := s.state.SaveBotWithRetry(bot); err != nil {
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

	bot, err := s.state.GetBot(botID)
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

	// Delete bot
	if err := s.state.DeleteBot(botID); err != nil {
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

	// Get existing bot
	bot, err := s.state.GetBot(botID)
	if err != nil {
		return errors.Wrap(errors.ErrorTypeDatabase, "update_heartbeat", "Failed to get bot", err)
	}

	// Update bot status
	now := time.Now()
	bot.LastSeen = now
	bot.Status = status
	bot.CurrentJob = currentJob
	bot.IsOnline = true
	bot.TimeoutAt = now.Add(s.config.Timeouts.BotHeartbeat)

	// Save bot
	if err := s.state.SaveBotWithRetry(bot); err != nil {
		return errors.Wrap(errors.ErrorTypeDatabase, "update_heartbeat", "Failed to update bot", err)
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
	bots, err := s.state.ListBots()
	if err != nil {
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "list_bots", "Failed to list bots", err)
	}

	// Apply filter if provided
	if statusFilter != nil {
		var filtered []*common.Bot
		for _, bot := range bots {
			if bot.Status == *statusFilter {
				filtered = append(filtered, bot)
			}
		}
		return filtered, nil
	}

	return bots, nil
}

// GetAvailableBot finds an available bot for job assignment
func (s *botService) GetAvailableBot(ctx context.Context, requiredCapabilities []string) (*common.Bot, error) {
	bots, err := s.state.ListBots()
	if err != nil {
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "get_available_bot", "Failed to list bots", err)
	}

	// Find available bot with required capabilities
	for _, bot := range bots {
		// Check if bot is available
		if bot.Status != common.BotStatusIdle || !bot.IsOnline {
			continue
		}

		// Check capabilities
		if hasRequiredCapabilities(bot.Capabilities, requiredCapabilities) {
			return bot, nil
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