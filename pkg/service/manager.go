package service

import (
	"context"
	"fmt"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// Manager holds all service instances
type Manager struct {
	Bot        BotService
	Job        JobService
	Result     ResultService
	System     SystemService
	Monitoring MonitoringService
	
	logger     *logrus.Logger
	cancelFunc context.CancelFunc
}

// NewManager creates a new service manager with all services
func NewManager(
	state StateStore,
	timeoutManager TimeoutManager,
	recoveryManager RecoveryManager,
	config *common.MasterConfig,
	logger *logrus.Logger,
) *Manager {
	// Create monitoring service first
	monitoring := NewMonitoringService(state, logger)
	collector := monitoring.GetCollector()
	
	// Create base services
	botService := NewBotService(state, timeoutManager, config, logger)
	jobService := NewJobService(state, timeoutManager, config, logger)
	resultService := NewResultService(state, config, logger)
	systemService := NewSystemService(state, timeoutManager, recoveryManager, config, logger)
	
	// Wrap with monitoring if enabled
	if config.Monitoring.Enabled {
		botService = NewMonitoringAwareBotService(botService, collector)
		jobService = NewMonitoringAwareJobService(jobService, collector)
		resultService = NewMonitoringAwareResultService(resultService, collector)
	}
	
	return &Manager{
		Bot:        botService,
		Job:        jobService,
		Result:     resultService,
		System:     systemService,
		Monitoring: monitoring,
		logger:     logger,
	}
}

// Start starts all managed services
func (m *Manager) Start(ctx context.Context) error {
	// Create context for service lifecycle
	serviceCtx, cancel := context.WithCancel(ctx)
	m.cancelFunc = cancel
	
	// Start monitoring service if it has a Start method
	if m.Monitoring != nil {
		go func() {
			if err := m.Monitoring.Start(serviceCtx); err != nil && err != context.Canceled {
				m.logger.WithError(err).Error("Monitoring service stopped with error")
			}
		}()
	}
	
	// Start other services if they implement lifecycle methods
	if starter, ok := m.Bot.(interface{ Start(context.Context) error }); ok {
		if err := starter.Start(serviceCtx); err != nil {
			return fmt.Errorf("failed to start bot service: %w", err)
		}
	}
	
	if starter, ok := m.Job.(interface{ Start(context.Context) error }); ok {
		if err := starter.Start(serviceCtx); err != nil {
			return fmt.Errorf("failed to start job service: %w", err)
		}
	}
	
	if starter, ok := m.Result.(interface{ Start(context.Context) error }); ok {
		if err := starter.Start(serviceCtx); err != nil {
			return fmt.Errorf("failed to start result service: %w", err)
		}
	}
	
	if starter, ok := m.System.(interface{ Start(context.Context) error }); ok {
		if err := starter.Start(serviceCtx); err != nil {
			return fmt.Errorf("failed to start system service: %w", err)
		}
	}
	
	m.logger.Info("All services started successfully")
	return nil
}

// Stop stops all managed services
func (m *Manager) Stop() error {
	if m.cancelFunc != nil {
		m.cancelFunc()
	}
	
	var errs []error
	
	// Stop services in reverse order
	if stopper, ok := m.System.(interface{ Stop() error }); ok {
		if err := stopper.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop system service: %w", err))
		}
	}
	
	if stopper, ok := m.Result.(interface{ Stop() error }); ok {
		if err := stopper.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop result service: %w", err))
		}
	}
	
	if stopper, ok := m.Job.(interface{ Stop() error }); ok {
		if err := stopper.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop job service: %w", err))
		}
	}
	
	if stopper, ok := m.Bot.(interface{ Stop() error }); ok {
		if err := stopper.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop bot service: %w", err))
		}
	}
	
	// Monitoring service stops when context is canceled
	
	if len(errs) > 0 {
		return fmt.Errorf("errors stopping services: %v", errs)
	}
	
	m.logger.Info("All services stopped successfully")
	return nil
}