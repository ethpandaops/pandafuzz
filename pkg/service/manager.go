package service

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// Manager holds all service instances
type Manager struct {
	Bot             BotService
	Job             JobService
	Result          ResultService
	System          SystemService
	Monitoring      MonitoringService
	Campaign        common.CampaignService
	Corpus          common.CorpusService
	Reproducibility common.ReproducibilityService
	CrashMinimizer  common.CrashMinimizerService

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
) (*Manager, error) {
	// Validate required dependencies
	if state == nil {
		return nil, fmt.Errorf("service manager: state store is required")
	}
	if timeoutManager == nil {
		return nil, fmt.Errorf("service manager: timeout manager is required")
	}
	if recoveryManager == nil {
		return nil, fmt.Errorf("service manager: recovery manager is required")
	}
	if config == nil {
		return nil, fmt.Errorf("service manager: configuration is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("service manager: logger is required")
	}

	// Create monitoring service first
	monitoringService := NewMonitoringService(state, logger)
	collector := monitoringService.GetCollector()

	// Create campaign-related services
	var campaignService common.CampaignService
	var corpusService common.CorpusService

	// Initialize campaign and corpus services if storage is available
	if storageProvider, ok := state.(interface{ GetStorage() common.Storage }); ok {
		if storage := storageProvider.GetStorage(); storage != nil {
			// Initialize campaign service (jobService can be nil for basic operations)
			campaignService = NewCampaignService(storage, nil, logger)
			// Create file storage based on configuration
			var fileStorage common.FileStorage
			var corpusDir string

			// Check if state provides file storage directly
			if fsProvider, ok := state.(interface{ GetFileStorage() common.FileStorage }); ok {
				fileStorage = fsProvider.GetFileStorage()
				corpusDir = "corpus" // Use as S3 key prefix
			} else {
				// Fall back to creating file storage based on config
				if config.Storage.Type == "filesystem" && config.Storage.Filesystem.BasePath != "" {
					fileStorage = NewLocalFileStorage(config.Storage.Filesystem.BasePath, logger)
					corpusDir = filepath.Join(config.Storage.Filesystem.BasePath, "corpus")
				} else {
					// For S3/MinIO backends, we'll use a default local path for now
					// The actual storage backend integration will handle S3 operations
					defaultPath := "./storage"
					fileStorage = NewLocalFileStorage(defaultPath, logger)
					corpusDir = "corpus" // This becomes the S3 key prefix
				}
			}

			var err error
			corpusService, err = NewCorpusService(storage, fileStorage, corpusDir, logger)
			if err != nil {
				logger.WithError(err).Error("Failed to create corpus service")
			}
		} else {
			logger.Warn("Storage not available in state provider, corpus service will be limited")
		}
	} else {
		logger.Warn("State does not implement storage provider interface")
	}

	// Initialize reproducibility service if storage is available
	var reproducibilityService common.ReproducibilityService
	if storageProvider, ok := state.(interface{ GetStorage() common.Storage }); ok {
		if storage := storageProvider.GetStorage(); storage != nil {
			reproducibilityService = NewReproducibilityService(storage, config, logger)
		}
	}

	// Initialize crash minimizer service if storage and file storage are available
	var crashMinimizerService common.CrashMinimizerService
	if storageProvider, ok := state.(interface{ GetStorage() common.Storage }); ok {
		if storage := storageProvider.GetStorage(); storage != nil {
			// Get file storage
			var fileStorage common.FileStorage
			if fsProvider, ok := state.(interface{ GetFileStorage() common.FileStorage }); ok {
				fileStorage = fsProvider.GetFileStorage()
			} else if config.Storage.Type == "filesystem" && config.Storage.Filesystem.BasePath != "" {
				fileStorage = NewLocalFileStorage(config.Storage.Filesystem.BasePath, logger)
			} else {
				fileStorage = NewLocalFileStorage("./storage", logger)
			}
			crashMinimizerService = NewCrashMinimizerService(logger, storage, fileStorage)
		}
	}

	// Create base services
	botService := NewBotService(state, timeoutManager, config, logger)
	jobService := NewJobService(state, timeoutManager, config, logger, corpusService)
	resultService := NewResultService(state, config, logger)
	systemService := NewSystemService(state, timeoutManager, recoveryManager, config, logger)

	// If using asynq queue backend, create and set the queue
	if config.Queue.Backend == "asynq" {
		// For now, skip asynq initialization in manager
		// The asynq queue requires a full JobRepository implementation
		// which the Storage interface doesn't provide
		logger.Info("Asynq queue backend configured, but initialization skipped in manager")
		logger.Info("Bot workers will connect directly to Redis for job processing")
	}

	// Wrap with monitoring if enabled
	if config.Monitoring.Enabled {
		botService = NewMonitoringAwareBotService(botService, collector)
		jobService = NewMonitoringAwareJobService(jobService, collector)
		resultService = NewMonitoringAwareResultService(resultService, collector)
	}

	return &Manager{
		Bot:             botService,
		Job:             jobService,
		Result:          resultService,
		System:          systemService,
		Monitoring:      monitoringService,
		Campaign:        campaignService,
		Corpus:          corpusService,
		Reproducibility: reproducibilityService,
		CrashMinimizer:  crashMinimizerService,
		logger:          logger,
	}, nil
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

	// Start reproducibility service if available
	if m.Reproducibility != nil {
		if err := m.Reproducibility.Start(serviceCtx); err != nil {
			return fmt.Errorf("failed to start reproducibility service: %w", err)
		}
	}

	// Start crash minimizer service if available
	if m.CrashMinimizer != nil {
		if err := m.CrashMinimizer.Start(serviceCtx); err != nil {
			return fmt.Errorf("failed to start crash minimizer service: %w", err)
		}
	}

	m.logger.Info("All services started successfully")
	return nil
}

// Stop stops all managed services
func (m *Manager) Stop() error {
	m.logger.Info("Stopping service manager")

	// Cancel context to signal all services to stop
	if m.cancelFunc != nil {
		m.cancelFunc()
	}

	var errs []error

	// Stop services in reverse order of dependency
	// Stop crash minimizer service first
	if m.CrashMinimizer != nil {
		m.logger.Debug("Stopping crash minimizer service")
		if err := m.CrashMinimizer.Stop(); err != nil && err != context.Canceled {
			errs = append(errs, fmt.Errorf("failed to stop crash minimizer service: %w", err))
		}
	}

	// Stop reproducibility service
	if m.Reproducibility != nil {
		m.logger.Debug("Stopping reproducibility service")
		if err := m.Reproducibility.Stop(); err != nil && err != context.Canceled {
			errs = append(errs, fmt.Errorf("failed to stop reproducibility service: %w", err))
		}
	}

	// Stop system service
	if stopper, ok := m.System.(interface{ Stop() error }); ok {
		m.logger.Debug("Stopping system service")
		if err := stopper.Stop(); err != nil && err != context.Canceled {
			errs = append(errs, fmt.Errorf("failed to stop system service: %w", err))
		}
	}

	// Stop result service
	if stopper, ok := m.Result.(interface{ Stop() error }); ok {
		m.logger.Debug("Stopping result service")
		if err := stopper.Stop(); err != nil && err != context.Canceled {
			errs = append(errs, fmt.Errorf("failed to stop result service: %w", err))
		}
	}

	// Stop job service
	if stopper, ok := m.Job.(interface{ Stop() error }); ok {
		m.logger.Debug("Stopping job service")
		if err := stopper.Stop(); err != nil && err != context.Canceled {
			errs = append(errs, fmt.Errorf("failed to stop job service: %w", err))
		}
	}

	// Stop bot service
	if stopper, ok := m.Bot.(interface{ Stop() error }); ok {
		m.logger.Debug("Stopping bot service")
		if err := stopper.Stop(); err != nil && err != context.Canceled {
			errs = append(errs, fmt.Errorf("failed to stop bot service: %w", err))
		}
	}

	// Stop monitoring service (handled by context cancellation)
	if m.Monitoring != nil {
		m.logger.Debug("Monitoring service stopping via context cancellation")
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors stopping services: %v", errs)
	}

	m.logger.Info("All services stopped successfully")
	return nil
}

// GetCollectionFilePath returns the storage path for a corpus collection file
func (m *Manager) GetCollectionFilePath(collectionID, fileHash string) string {
	// Collections are stored under: {basePath}/collections/{collectionID}/{hash}
	// This matches the path used in api_corpus_collections.go: "collections/{collectionID}/{hash}"
	basePath := "./storage" // Default path

	// Build the file path
	return filepath.Join(basePath, "collections", collectionID, fileHash)
}
