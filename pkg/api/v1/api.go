package apiv1

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/adapters"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/handlers"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/middleware"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/sse"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/executor"
	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/registry"
	botRepo "github.com/ethpandaops/pandafuzz/pkg/domain/bot/repository"
	campaignRepo "github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	crashRepo "github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	jobRepo "github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	"github.com/ethpandaops/pandafuzz/pkg/service"
)

// API represents the main API v1 instance
type API struct {
	router      chi.Router
	sse         *sse.Manager
	adapters    *adapters.CompositeAdapter
	handlers    *handlers.Handlers
	config      *Config
	logger      logrus.FieldLogger
	middlewares *middleware.Stack
	running     bool
	mu          sync.RWMutex
}

// Config holds API configuration options
type Config struct {
	// Authentication configuration
	EnableAuth bool              `yaml:"enable_auth" json:"enable_auth"`
	JWTSecret  string            `yaml:"jwt_secret" json:"jwt_secret"`
	APIKeys    map[string]string `yaml:"api_keys" json:"api_keys"`

	// CORS configuration
	CORSOrigins      []string `yaml:"cors_origins" json:"cors_origins"`
	EnableCORS       bool     `yaml:"enable_cors" json:"enable_cors"`
	CORSAllowHeaders []string `yaml:"cors_allow_headers" json:"cors_allow_headers"`
	CORSAllowMethods []string `yaml:"cors_allow_methods" json:"cors_allow_methods"`

	// Rate limiting configuration
	RateLimit       int           `yaml:"rate_limit" json:"rate_limit"`
	RateLimitBurst  int           `yaml:"rate_limit_burst" json:"rate_limit_burst"`
	RateLimitWindow time.Duration `yaml:"rate_limit_window" json:"rate_limit_window"`

	// Request size limits
	MaxRequestSize int64 `yaml:"max_request_size" json:"max_request_size"`

	// SSE configuration
	SSEEnabled bool       `yaml:"sse_enabled" json:"sse_enabled"`
	SSEConfig  sse.Config `yaml:"sse_config" json:"sse_config"`

	// Middleware configuration
	EnableTracing        bool          `yaml:"enable_tracing" json:"enable_tracing"`
	EnableMetrics        bool          `yaml:"enable_metrics" json:"enable_metrics"`
	EnableRequestLogging bool          `yaml:"enable_request_logging" json:"enable_request_logging"`
	RequestTimeout       time.Duration `yaml:"request_timeout" json:"request_timeout"`

	// Validation configuration
	StrictValidation       bool `yaml:"strict_validation" json:"strict_validation"`
	AllowUnknownFields     bool `yaml:"allow_unknown_fields" json:"allow_unknown_fields"`
	EnableSchemaValidation bool `yaml:"enable_schema_validation" json:"enable_schema_validation"`
}

// Services represents the domain services needed by the API
type Services struct {
	// Core services
	Bot             service.BotService
	Job             service.JobService
	Campaign        common.CampaignService
	Corpus          common.CorpusService
	Result          service.ResultService
	System          service.SystemService
	Monitoring      service.MonitoringService
	Reproducibility common.ReproducibilityService
	CrashMinimizer  common.CrashMinimizerService
	Deduplication   common.DeduplicationService
	Analytics       service.AnalyticsService

	// Storage
	Storage common.Storage

	// Repositories
	BotRepo      botRepo.AgentRepository
	JobRepo      jobRepo.JobRepository
	CampaignRepo campaignRepo.CampaignRepository
	CrashRepo    crashRepo.CrashRepository

	// Domain services
	BotRegistry *registry.Service
	Executor    executor.Executor
}

// DefaultConfig returns sensible defaults for API configuration
func DefaultConfig() *Config {
	return &Config{
		EnableAuth:             false,
		EnableCORS:             true,
		CORSOrigins:            []string{"*"},
		CORSAllowHeaders:       []string{"*"},
		CORSAllowMethods:       []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		RateLimit:              1000,
		RateLimitBurst:         2000,
		RateLimitWindow:        time.Minute,
		MaxRequestSize:         10 * 1024 * 1024, // 10MB
		SSEEnabled:             true,
		SSEConfig:              sse.DefaultConfig(),
		EnableTracing:          true,
		EnableMetrics:          true,
		EnableRequestLogging:   true,
		RequestTimeout:         30 * time.Second,
		StrictValidation:       false,
		AllowUnknownFields:     true,
		EnableSchemaValidation: true,
	}
}

// NewAPI creates a new API instance with all components initialized
func NewAPI(config *Config, services Services, logger logrus.FieldLogger) (*API, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	apiLogger := logger.WithField("component", "api_v1")

	// Initialize SSE manager if enabled
	var sseManager *sse.Manager
	if config.SSEEnabled {
		sseManager = sse.NewManager(config.SSEConfig, apiLogger)
	}

	// Create individual adapters using provided services and repositories
	botAdapter := adapters.NewBotAdapter(
		services.BotRegistry,
		services.BotRepo,
		services.JobRepo,
		services.Bot,
		services.Job,
		sseManager,
		apiLogger,
	)

	jobAdapter := adapters.NewJobAdapter(
		services.JobRepo,
		services.Executor,
		services.Job,
		sseManager,
		apiLogger,
	)

	campaignAdapter := adapters.NewCampaignAdapter(
		services.Campaign,
		services.CampaignRepo,
		sseManager,
		apiLogger,
	)

	// Create corpus adapter
	corpusAdapter := adapters.NewCorpusAdapter(
		services.Corpus,
		sseManager,
		apiLogger,
	)

	// Create crash adapter
	crashAdapter := adapters.NewCrashAdapter(
		services.CrashRepo,
		services.Storage,
		services.Deduplication,
		services.CrashMinimizer,
		services.Reproducibility,
		sseManager,
		apiLogger,
	)

	// Create analytics adapter
	analyticsAdapter := adapters.NewAnalyticsAdapter(
		services.JobRepo,
		services.CrashRepo,
		services.CampaignRepo,
		services.Analytics,
		sseManager,
		apiLogger,
	)

	// Create system adapter for system management endpoints
	systemAdapter := adapters.NewSystemAdapter(
		services.Bot,
		services.Job,
		sseManager,
		nil, // VersionInfo - can be passed if available
		apiLogger,
	)

	// Create composite adapter
	compositeAdapter := adapters.NewCompositeAdapter(
		botAdapter,
		jobAdapter,
		campaignAdapter,
		corpusAdapter,
		crashAdapter,
		analyticsAdapter,
		systemAdapter,
		sseManager,
		apiLogger,
	)

	// Create middleware stack
	middlewareStack := buildMiddlewareStack(config, apiLogger)

	// Create handlers
	handlersInstance := handlers.NewHandlers(
		compositeAdapter,
		middlewareStack,
		apiLogger,
	)

	api := &API{
		sse:         sseManager,
		adapters:    compositeAdapter,
		handlers:    handlersInstance,
		config:      config,
		logger:      apiLogger,
		middlewares: middlewareStack,
	}

	// Setup routes
	if err := api.setupRoutes(); err != nil {
		return nil, fmt.Errorf("failed to setup routes: %w", err)
	}

	return api, nil
}

// buildMiddlewareStack creates a configured middleware stack based on the provided config
func buildMiddlewareStack(config *Config, logger logrus.FieldLogger) *middleware.Stack {
	stack := middleware.NewStack(logger)

	// Configure CORS if enabled
	if config.EnableCORS {
		corsConfig := &middleware.CORSConfig{
			AllowedOrigins:   config.CORSOrigins,
			AllowedMethods:   config.CORSAllowMethods,
			AllowedHeaders:   config.CORSAllowHeaders,
			ExposedHeaders:   []string{"X-Request-ID", "X-Total-Count"},
			AllowCredentials: true,
			MaxAge:           300,
			Logger:           logger.WithField("middleware", "cors"),
		}
		stack = stack.WithCORSConfig(corsConfig)
	}

	// Configure rate limiting if enabled
	if config.RateLimit > 0 {
		rateLimitConfig := &middleware.RateLimitConfig{
			Rate:      config.RateLimit,
			Burst:     config.RateLimitBurst,
			TTL:       config.RateLimitWindow,
			SkipPaths: []string{"/health", "/ready"},
			Logger:    logger.WithField("middleware", "ratelimit"),
		}
		stack = stack.WithRateLimitConfig(rateLimitConfig)
	}

	// Configure validation if enabled
	if config.EnableSchemaValidation {
		validationConfig := &middleware.ValidationConfig{
			MaxRequestSize:       config.MaxRequestSize,
			RequiredContentTypes: []string{"application/json"},
			SkipPaths:            []string{"/health", "/ready"},
			Logger:               logger.WithField("middleware", "validation"),
		}
		stack = stack.WithValidationConfig(validationConfig)
	}

	// Configure authentication if enabled
	if config.EnableAuth && config.JWTSecret != "" {
		authConfig := &middleware.AuthConfig{
			JWTSecret: config.JWTSecret,
			SkipPaths: []string{"/health", "/ready", "/metrics"},
			Logger:    logger.WithField("middleware", "auth"),
		}
		stack = stack.WithAuthConfig(authConfig)
	}

	// Configure tracing if enabled
	if config.EnableTracing {
		tracingConfig := &middleware.TracingConfig{
			ServiceName:    "pandafuzz-api",
			ServiceVersion: "1.0.0",
			Environment:    "production",
			SkipPaths:      []string{"/health", "/ready", "/metrics"},
			Logger:         logger.WithField("middleware", "tracing"),
		}
		stack = stack.WithTracingConfig(tracingConfig)
	}

	// Configure metrics if enabled
	if config.EnableMetrics {
		metricsConfig := &middleware.MetricsConfig{
			Namespace: "pandafuzz",
			Subsystem: "api",
			SkipPaths: []string{"/health", "/ready", "/metrics"},
			Logger:    logger.WithField("middleware", "metrics"),
		}
		stack = stack.WithMetricsConfig(metricsConfig)
	}

	return stack
}

// setupRoutes configures all routes using the handlers
func (a *API) setupRoutes() error {
	a.router = chi.NewRouter()

	// Register routes through handlers
	a.handlers.RegisterRoutes(a.router)

	a.logger.Info("API v1 routes configured successfully")
	return nil
}

// GetRouter returns the configured Chi router
func (a *API) GetRouter() chi.Router {
	return a.router
}

// GetSSEManager returns the SSE manager instance
func (a *API) GetSSEManager() *sse.Manager {
	return a.sse
}

// Start initializes and starts the API services
func (a *API) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return fmt.Errorf("API v1 is already running")
	}

	a.logger.Info("Starting API v1 services")

	// Start SSE manager if enabled
	if a.sse != nil {
		if err := a.sse.Start(ctx); err != nil {
			return fmt.Errorf("failed to start SSE manager: %w", err)
		}
		a.logger.Info("SSE manager started successfully")
	}

	a.running = true
	a.logger.Info("API v1 started successfully")
	return nil
}

// Stop gracefully shuts down the API services
func (a *API) Stop(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.running {
		return nil
	}

	a.logger.Info("Stopping API v1 services")

	// Stop SSE manager if running
	if a.sse != nil {
		if err := a.sse.Stop(); err != nil {
			a.logger.WithError(err).Error("Error stopping SSE manager")
		} else {
			a.logger.Info("SSE manager stopped successfully")
		}
	}

	a.running = false
	a.logger.Info("API v1 stopped successfully")
	return nil
}

// IsRunning returns whether the API is currently running
func (a *API) IsRunning() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.running
}

// GetConfig returns the API configuration
func (a *API) GetConfig() *Config {
	return a.config
}

// GetAdapters returns the composite adapter
func (a *API) GetAdapters() *adapters.CompositeAdapter {
	return a.adapters
}

// GetHandlers returns the handlers instance
func (a *API) GetHandlers() *handlers.Handlers {
	return a.handlers
}
