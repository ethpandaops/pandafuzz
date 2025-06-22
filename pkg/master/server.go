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
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// Server represents the master HTTP server
type Server struct {
	config           *common.MasterConfig
	state            *PersistentState
	timeoutManager   *TimeoutManager
	recoveryManager  *RecoveryManager
	services         *service.Manager
	httpServer       *http.Server
	router           *mux.Router
	logger           *logrus.Logger
	retryManager     *common.RetryManager
	circuitBreaker   *common.CircuitBreaker
	responseWriter   *httputil.ResponseWriter
	middleware       []Middleware
	shutdownTimeout  time.Duration
	mu               sync.RWMutex
	running          bool
	stats            ServerStats
}

// ServerStats tracks server performance metrics
type ServerStats struct {
	StartTime       time.Time `json:"start_time"`
	RequestCount    int64     `json:"request_count"`
	ErrorCount      int64     `json:"error_count"`
	ActiveRequests  int64     `json:"active_requests"`
	AverageLatency  time.Duration `json:"average_latency"`
	LastRequest     time.Time `json:"last_request"`
	HealthyUptime   time.Duration `json:"healthy_uptime"`
	TotalConnections int64     `json:"total_connections"`
}

// Middleware represents HTTP middleware
type Middleware func(http.Handler) http.Handler

// NewServer creates a new master server instance
func NewServer(config *common.MasterConfig, state *PersistentState, timeoutManager *TimeoutManager) *Server {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	
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
	
	return &Server{
		config:          config,
		state:           state,
		timeoutManager:  timeoutManager,
		logger:          logger,
		retryManager:    common.NewRetryManager(retryPolicy),
		circuitBreaker:  circuitBreaker,
		responseWriter:  httputil.NewResponseWriter(logger),
		shutdownTimeout: 30 * time.Second,
		stats: ServerStats{
			StartTime: time.Now(),
		},
	}
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
	
	// Start monitoring service if enabled
	if s.services != nil && s.config.Monitoring.Enabled {
		ctx := context.Background()
		go func() {
			s.logger.Info("Starting monitoring service")
			if err := s.services.Monitoring.Start(ctx); err != nil {
				s.logger.WithError(err).Error("Monitoring service stopped")
			}
		}()
		
		// Start separate metrics server if configured
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
	// Initialize services after recovery manager is set
	s.services = service.NewManager(s.state, s.timeoutManager, s.recoveryManager, s.config, s.logger)
}