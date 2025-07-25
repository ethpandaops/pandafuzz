package master

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/httputil"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/ethpandaops/pandafuzz/pkg/storage/backend"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// Server represents the master HTTP server
type Server struct {
	config          *common.MasterConfig
	state           *PersistentState
	timeoutManager  *TimeoutManager
	recoveryManager *RecoveryManager
	botPoller       *BotPoller
	services        *service.Manager
	httpServer      *http.Server
	router          *mux.Router
	logger          *logrus.Logger
	retryManager    *common.RetryManager
	circuitBreaker  *common.CircuitBreaker
	responseWriter  *httputil.ResponseWriter
	wsHub           *WSHub
	middleware      []Middleware
	shutdownTimeout time.Duration
	mu              sync.RWMutex
	running         bool
	stats           ServerStats
	version         string
	buildTime       string
	gitCommit       string
	storageBackend  backend.StorageBackend
}

// ServerStats tracks server performance metrics
type ServerStats struct {
	StartTime        time.Time     `json:"start_time"`
	RequestCount     int64         `json:"request_count"`
	ErrorCount       int64         `json:"error_count"`
	ActiveRequests   int64         `json:"active_requests"`
	AverageLatency   time.Duration `json:"average_latency"`
	LastRequest      time.Time     `json:"last_request"`
	HealthyUptime    time.Duration `json:"healthy_uptime"`
	TotalConnections int64         `json:"total_connections"`
}

// Middleware represents HTTP middleware
type Middleware func(http.Handler) http.Handler

// NewServer creates a new master server instance
func NewServer(config *common.MasterConfig, state *PersistentState, timeoutManager *TimeoutManager, versionInfo *common.VersionInfo, logger *logrus.Logger) *Server {

	// Configure retry manager for server operations
	retryPolicy := config.Retry.Network
	if retryPolicy.MaxRetries == 0 {
		retryPolicy = common.NetworkRetryPolicy
	}

	// Configure circuit breaker
	circuitBreaker := common.NewCircuitBreaker(
		config.Circuit.MaxFailures,
		config.Circuit.ResetTimeout,
	)

	server := &Server{
		config:          config,
		state:           state,
		timeoutManager:  timeoutManager,
		logger:          logger,
		retryManager:    common.NewRetryManager(retryPolicy),
		circuitBreaker:  circuitBreaker,
		responseWriter:  httputil.NewResponseWriter(logger),
		wsHub:           NewWSHub(logger),
		shutdownTimeout: 30 * time.Second,
		stats: ServerStats{
			StartTime: time.Now(),
		},
		version:   "dev",
		buildTime: "unknown",
		gitCommit: "unknown",
	}

	// Set version info if provided
	if versionInfo != nil {
		server.version = versionInfo.Version
		server.buildTime = versionInfo.BuildTime
		server.gitCommit = versionInfo.GitCommit
	}

	return server
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return common.NewSystemError("start_server", fmt.Errorf("server already running"))
	}

	s.logger.Info("Starting master HTTP server")

	// Setup router and middleware
	if err := s.setupRouter(); err != nil {
		return common.NewSystemError("setup_router", err)
	}

	// Configure HTTP server
	s.httpServer = &http.Server{
		Addr:           fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port),
		Handler:        s.router,
		ReadTimeout:    s.config.Server.ReadTimeout,
		WriteTimeout:   s.config.Server.WriteTimeout,
		IdleTimeout:    s.config.Server.IdleTimeout,
		MaxHeaderBytes: s.config.Server.MaxHeaderBytes,
	}

	// Start services through manager
	if s.services != nil {
		ctx := context.Background()
		if err := s.services.Start(ctx); err != nil {
			return common.NewSystemError("start_services", err)
		}

		// Start separate metrics server if configured
		if s.config.Monitoring.Enabled {
			metricsAddr := s.config.Monitoring.GetMetricsAddr()
			if metricsAddr != "" {
				go func() {
					collector := s.services.Monitoring.GetCollector()
					if err := collector.StartMetricsServer(ctx, metricsAddr); err != nil {
						s.logger.WithError(err).Error("Metrics server stopped")
					}
				}()
			}
		}
	}

	// Start bot poller
	if s.botPoller != nil {
		s.logger.Info("Starting bot poller")
		if err := s.botPoller.Start(); err != nil {
			return common.NewSystemError("start_bot_poller", err)
		}
	}

	// Start server in background
	go func() {
		s.logger.WithFields(logrus.Fields{
			"host": s.config.Server.Host,
			"port": s.config.Server.Port,
		}).Info("HTTP server listening")

		var err error
		if s.config.Server.EnableTLS {
			err = s.httpServer.ListenAndServeTLS(s.config.Server.TLSCertFile, s.config.Server.TLSKeyFile)
		} else {
			err = s.httpServer.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			s.logger.WithError(err).Error("HTTP server error")
		}
	}()

	s.running = true
	s.logger.Info("Master HTTP server started successfully")

	return nil
}

// Stop gracefully shuts down the HTTP server
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.logger.Info("Stopping master HTTP server")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	// Stop services through manager
	if s.services != nil {
		s.logger.Info("Stopping services")
		if err := s.services.Stop(); err != nil {
			s.logger.WithError(err).Error("Error stopping services")
		}
	}

	// Stop bot poller
	if s.botPoller != nil {
		s.logger.Info("Stopping bot poller")
		if err := s.botPoller.Stop(); err != nil {
			s.logger.WithError(err).Error("Error stopping bot poller")
		}
	}

	// Graceful shutdown
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.WithError(err).Error("Error during server shutdown")
		return common.NewSystemError("stop_server", err)
	}

	s.running = false
	s.logger.Info("Master HTTP server stopped")

	return nil
}

// GetStats returns server statistics
func (s *Server) GetStats() ServerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.stats
	stats.HealthyUptime = time.Since(s.stats.StartTime)

	return stats
}

// AddMiddleware adds custom middleware to the server
func (s *Server) AddMiddleware(middleware Middleware) {
	s.middleware = append(s.middleware, middleware)
}

// IsRunning returns whether the server is running
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// SetRecoveryManager sets the recovery manager (to avoid circular dependencies)
func (s *Server) SetRecoveryManager(rm *RecoveryManager) {
	s.recoveryManager = rm
	// Don't initialize services here anymore - wait for storage to be initialized
}

// SetServiceManager sets the service manager (to avoid duplicate creation)
func (s *Server) SetServiceManager(sm *service.Manager) {
	s.services = sm
}

// GetRouter returns the configured router
func (s *Server) GetRouter() *mux.Router {
	return s.router
}

// InitializeRouter sets up the router without starting the server
func (s *Server) InitializeRouter() error {
	return s.setupRouter()
}

// InitializeStorage creates and initializes the storage backend based on configuration
func (s *Server) InitializeStorage() error {
	s.logger.Info("Initializing storage backend")

	// Create storage backend based on configuration
	storageBackend, err := backend.NewStorageBackend(s.config.Storage, s.logger)
	if err != nil {
		return fmt.Errorf("failed to initialize storage backend: %w", err)
	}

	// Perform health check
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := storageBackend.HealthCheck(ctx); err != nil {
		return fmt.Errorf("storage backend health check failed: %w", err)
	}

	s.storageBackend = storageBackend
	s.logger.WithField("type", s.config.Storage.Type).Info("Storage backend initialized successfully")

	// Now initialize services with the storage backend
	s.initializeServices()

	return nil
}

// initializeServices initializes the service manager with storage backend
func (s *Server) initializeServices() {
	// Skip if services are already set (from dependency injection)
	if s.services != nil {
		s.logger.Info("Service manager already initialized, skipping creation")
		// Just initialize bot poller
		s.botPoller = NewBotPoller(s.state, s.services, s.logger, 5*time.Second)
		return
	}

	// Initialize services after storage backend is ready
	stateAdapter := NewStateStoreAdapter(s.state)

	// Create custom service manager initialization to use storage backend
	s.services = s.createServiceManager(stateAdapter)

	// Initialize bot poller with 5 second interval for more responsive updates
	s.botPoller = NewBotPoller(s.state, s.services, s.logger, 5*time.Second)
}

// createServiceManager creates a service manager with the storage backend
func (s *Server) createServiceManager(stateAdapter service.StateStore) *service.Manager {
	// Create a custom state adapter that provides the storage backend
	customStateAdapter := &storageBackendAdapter{
		StateStore:     stateAdapter,
		storageBackend: s.storageBackend,
		config:         s.config,
		logger:         s.logger,
	}

	// Use the NewManager constructor with proper dependency injection
	return service.NewManager(
		customStateAdapter,
		s.timeoutManager,
		s.recoveryManager,
		s.config,
		s.logger,
	)
}

// GetStorageBackend returns the storage backend instance
func (s *Server) GetStorageBackend() backend.StorageBackend {
	return s.storageBackend
}

// getStorageBasePath returns the base path for storage operations
// For filesystem backend, it returns the configured base path
// For S3/MinIO backends, it returns a default local path for temporary files
func (s *Server) getStorageBasePath() string {
	if s.config.Storage.Type == "filesystem" {
		return s.config.Storage.Filesystem.BasePath
	}
	// For S3/MinIO backends, return a default local path for temporary files
	return "./storage"
}

// storageBackendAdapter wraps StateStore to provide storage backend access
type storageBackendAdapter struct {
	service.StateStore
	storageBackend backend.StorageBackend
	config         *common.MasterConfig
	logger         *logrus.Logger
}

// GetFileStorage returns appropriate file storage based on configuration
func (a *storageBackendAdapter) GetFileStorage() common.FileStorage {
	if a.storageBackend != nil {
		return service.NewBackendFileStorage(a.storageBackend, a.logger)
	}
	// Fallback to local file storage
	basePath := "./storage"
	if a.config.Storage.Type == "filesystem" {
		basePath = a.config.Storage.Filesystem.BasePath
	}
	return service.NewLocalFileStorage(basePath, a.logger)
}
