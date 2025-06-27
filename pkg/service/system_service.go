package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/sirupsen/logrus"
)

// systemService implements SystemService interface
type systemService struct {
	state           StateStore
	timeoutManager  TimeoutManager
	recoveryManager RecoveryManager
	config          *common.MasterConfig
	logger          *logrus.Logger
	
	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
}

// Compile-time interface compliance check
var _ SystemService = (*systemService)(nil)

// NewSystemService creates a new system service
func NewSystemService(
	state StateStore,
	timeoutManager TimeoutManager,
	recoveryManager RecoveryManager,
	config *common.MasterConfig,
	logger *logrus.Logger,
) SystemService {
	return &systemService{
		state:           state,
		timeoutManager:  timeoutManager,
		recoveryManager: recoveryManager,
		config:          config,
		logger:          logger,
	}
}

// GetSystemStats returns system statistics
func (s *systemService) GetSystemStats(ctx context.Context) (SystemStats, error) {
	stats := SystemStats{
		StateStats:    s.state.GetStats(),
		TimeoutStats:  s.timeoutManager.GetStats(),
		DatabaseStats: s.state.GetDatabaseStats(),
		Timestamp:     time.Now(),
	}

	return stats, nil
}

// TriggerRecovery triggers system recovery
func (s *systemService) TriggerRecovery(ctx context.Context) error {
	if s.recoveryManager == nil {
		return errors.New(errors.ErrorTypeSystem, "trigger_recovery", "Recovery manager not available")
	}

	// Trigger recovery
	if err := s.recoveryManager.RecoverOnStartup(ctx); err != nil {
		return errors.Wrap(errors.ErrorTypeSystem, "trigger_recovery", "Recovery failed", err)
	}

	s.logger.Info("Manual system recovery triggered")
	return nil
}

// GetActiveTimeouts returns active timeouts
func (s *systemService) GetActiveTimeouts(ctx context.Context) (TimeoutInfo, error) {
	botTimeouts := s.timeoutManager.ListBotTimeouts()
	jobTimeouts := s.timeoutManager.ListJobTimeouts()

	// Convert to TimeoutEntry format
	botEntries := make([]TimeoutEntry, 0, len(botTimeouts))
	for id, timeout := range botTimeouts {
		remaining := time.Until(timeout)
		botEntries = append(botEntries, TimeoutEntry{
			EntityID:  id,
			Timeout:   timeout,
			Remaining: fmt.Sprintf("%.0f seconds", remaining.Seconds()),
		})
	}

	jobEntries := make([]TimeoutEntry, 0, len(jobTimeouts))
	for id, timeout := range jobTimeouts {
		remaining := time.Until(timeout)
		jobEntries = append(jobEntries, TimeoutEntry{
			EntityID:  id,
			Timeout:   timeout,
			Remaining: fmt.Sprintf("%.0f seconds", remaining.Seconds()),
		})
	}

	return TimeoutInfo{
		BotTimeouts: botEntries,
		JobTimeouts: jobEntries,
		Timestamp:   time.Now(),
	}, nil
}

// ForceTimeout forces a timeout for an entity
func (s *systemService) ForceTimeout(ctx context.Context, entityType string, entityID string) error {
	if entityType == "" || entityID == "" {
		return errors.NewValidationError("force_timeout", "Entity type and ID are required")
	}

	// Validate timeout type
	if entityType != "bot" && entityType != "job" {
		return errors.NewValidationError("force_timeout", "Invalid timeout type. Must be 'bot' or 'job'")
	}

	if err := s.timeoutManager.ForceTimeout(entityType, entityID); err != nil {
		return errors.Wrap(errors.ErrorTypeSystem, "force_timeout", "Failed to force timeout", err)
	}

	s.logger.WithFields(logrus.Fields{
		"type":      entityType,
		"entity_id": entityID,
	}).Info("Timeout forced manually")

	return nil
}

// Start starts the system service
func (s *systemService) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	
	// Start system monitoring goroutine
	go s.monitorSystemHealth()
	
	s.logger.Info("System service started")
	return nil
}

// Stop stops the system service
func (s *systemService) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	
	// Clean up any resources
	// Currently system service doesn't have resources to clean up
	
	s.logger.Info("System service stopped")
	return nil
}

// monitorSystemHealth monitors system health metrics
func (s *systemService) monitorSystemHealth() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Log system stats periodically
			stats, err := s.GetSystemStats(s.ctx)
			if err != nil {
				s.logger.WithError(err).Error("Failed to get system stats")
				continue
			}
			
			s.logger.WithFields(logrus.Fields{
				"state_stats":    stats.StateStats,
				"timeout_stats":  stats.TimeoutStats,
				"database_stats": stats.DatabaseStats,
			}).Debug("System health check")
		}
	}
}