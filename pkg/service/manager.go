package service

import (
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
	}
}