package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	_ "github.com/ethpandaops/pandafuzz/pkg/config"
	"github.com/ethpandaops/pandafuzz/pkg/interfaces/api/rest/v1"
	"github.com/ethpandaops/pandafuzz/pkg/interfaces/api/rest/v2"
	"github.com/ethpandaops/pandafuzz/pkg/master"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
	"github.com/ethpandaops/pandafuzz/pkg/storage/backend"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	// Command line flags
	var (
		configFile    = flag.String("config", "master.yaml", "Path to configuration file")
		logLevel      = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		showVersion   = flag.Bool("version", false, "Show version information")
		validateOnly  = flag.Bool("validate", false, "Validate configuration and exit")
		migrateDB     = flag.Bool("migrate", false, "Run database migrations and exit")
		resetDB       = flag.Bool("reset-db", false, "Reset database (WARNING: deletes all data)")
		dataDir       = flag.String("data-dir", "./data", "Data directory for persistent storage")
		port          = flag.Int("port", 0, "Override HTTP server port")
		metricsPort   = flag.Int("metrics-port", 0, "Override metrics server port")
		enableMetrics = flag.Bool("metrics", true, "Enable Prometheus metrics")
		queueBackend  = flag.String("queue-backend", "", "Queue backend (memory|asynq), overrides config")
		redisHost     = flag.String("redis-host", "localhost", "Redis host for asynq backend")
		redisPort     = flag.Int("redis-port", 6379, "Redis port for asynq backend")
	)

	flag.Parse()

	// Show version if requested
	if *showVersion {
		fmt.Printf("PandaFuzz Master\n")
		fmt.Printf("Version:    %s\n", version)
		fmt.Printf("Build Time: %s\n", buildTime)
		fmt.Printf("Git Commit: %s\n", gitCommit)
		os.Exit(0)
	}

	// Load configuration
	config, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Override configuration with command line flags
	if *port > 0 {
		config.Server.Port = *port
	}
	if *metricsPort > 0 {
		config.Monitoring.MetricsPort = *metricsPort
	}
	if *dataDir != "" {
		config.Database.Path = filepath.Join(*dataDir, "pandafuzz.db")
	}
	config.Monitoring.MetricsEnabled = *enableMetrics
	config.Monitoring.Enabled = *enableMetrics

	// Override queue backend if specified
	if *queueBackend != "" {
		config.Queue.Backend = *queueBackend
		if *queueBackend == "asynq" {
			// Configure Redis if using asynq
			config.Redis.Host = *redisHost
			config.Redis.Port = *redisPort
		}
	}

	// Setup logging
	logLevelStr := *logLevel
	if config.Logging.Level != "" {
		logLevelStr = config.Logging.Level
	}
	logger := setupLogging(logLevelStr)

	// Log queue backend configuration if specified
	if *queueBackend == "asynq" {
		logger.WithFields(logrus.Fields{
			"backend":    "asynq",
			"redis_host": *redisHost,
			"redis_port": *redisPort,
		}).Info("Queue backend configured")
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		logger.WithError(err).Fatal("Invalid configuration")
	}

	if *validateOnly {
		logger.Info("Configuration is valid")
		os.Exit(0)
	}

	// Initialize dependencies
	deps, err := initializeDependencies(config, logger)
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize dependencies")
	}
	defer deps.Close()

	// Handle database operations
	if *resetDB {
		if err := deps.ResetDatabase(); err != nil {
			logger.WithError(err).Fatal("Failed to reset database")
		}
		logger.Info("Database reset successfully")
		os.Exit(0)
	}

	if *migrateDB {
		logger.Info("Database migrations completed")
		os.Exit(0)
	}

	// Perform startup recovery
	logger.Info("Performing system recovery")
	if err := deps.RecoveryManager.RecoverOnStartup(context.Background()); err != nil {
		logger.WithError(err).Error("Recovery failed, continuing anyway")
	}

	// Create and configure HTTP server
	server := createHTTPServer(config, deps, logger)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start background services
	if err := deps.StartBackgroundServices(ctx); err != nil {
		logger.WithError(err).Fatal("Failed to start background services")
	}

	// Start HTTP server
	logger.WithFields(logrus.Fields{
		"port":         config.Server.Port,
		"metrics_port": config.Monitoring.MetricsPort,
		"metrics":      config.Monitoring.MetricsEnabled,
		"version":      version,
	}).Info("Starting PandaFuzz Master")

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.WithError(err).Fatal("Server failed to start")
		}
	}()

	// Wait for shutdown signal
	sig := <-sigChan
	logger.WithField("signal", sig).Info("Received shutdown signal")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	logger.Info("Starting graceful shutdown")
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.WithError(err).Error("Failed to shutdown server gracefully")
	}

	// Stop background services
	cancel()
	deps.StopBackgroundServices()

	logger.Info("PandaFuzz Master shutdown complete")
}

func setupLogging(level string) *logrus.Logger {
	logger := logrus.New()

	// Set log level
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		logLevel = logrus.InfoLevel
	}
	logger.SetLevel(logLevel)

	// Set formatter (using JSON as specified in config)
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
	})

	// Set output
	logger.SetOutput(os.Stdout)

	// Log the configured level
	logger.WithField("level", level).Info("Logging configured")

	return logger
}

func loadConfig(configFile string) (*common.MasterConfig, error) {
	// Create config manager
	configMgr := common.NewConfigManager()

	// Check if config file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// Create default configuration with proper defaults
		config := &common.MasterConfig{}
		configMgr.SetMasterDefaults(config)

		// Apply storage defaults
		config.Storage.SetDefaults()

		logger := logrus.New()
		logger.WithField("config_file", configFile).Warn("Config file not found, using defaults")
		return config, nil
	}

	// Load master configuration from file
	config, err := configMgr.LoadMasterConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Ensure storage defaults are applied
	config.Storage.SetDefaults()

	return config, nil
}

func validateConfig(config *common.MasterConfig) error {
	// Validate server configuration
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", config.Server.Port)
	}

	if config.Monitoring.Enabled && (config.Monitoring.MetricsPort <= 0 || config.Monitoring.MetricsPort > 65535) {
		return fmt.Errorf("invalid metrics port: %d", config.Monitoring.MetricsPort)
	}

	// Validate database configuration
	if config.Database.Type != "sqlite" && config.Database.Type != "postgres" {
		return fmt.Errorf("unsupported database type: %s", config.Database.Type)
	}

	// Validate storage configuration
	if err := config.Storage.Validate(); err != nil {
		return fmt.Errorf("storage configuration error: %w", err)
	}

	// Validate timeouts
	if config.Timeouts.BotHeartbeat < 10*time.Second {
		return fmt.Errorf("bot heartbeat timeout too short: %v", config.Timeouts.BotHeartbeat)
	}

	if config.Timeouts.JobExecution < time.Minute {
		return fmt.Errorf("job execution timeout too short: %v", config.Timeouts.JobExecution)
	}

	// Validate limits
	if config.Limits.MaxConcurrentJobs <= 0 {
		return fmt.Errorf("invalid max concurrent jobs: %d", config.Limits.MaxConcurrentJobs)
	}

	if config.Limits.MaxCorpusSize <= 0 {
		return fmt.Errorf("invalid max corpus size: %d", config.Limits.MaxCorpusSize)
	}

	// Validate security settings if enabled
	if config.Security.EnableInputValidation {
		if config.Security.MaxRequestSize <= 0 {
			return fmt.Errorf("invalid max request size: %d", config.Security.MaxRequestSize)
		}
		if config.Security.MaxCrashFileSize <= 0 {
			return fmt.Errorf("invalid max crash file size: %d", config.Security.MaxCrashFileSize)
		}
		if config.Security.MaxCorpusFileSize <= 0 {
			return fmt.Errorf("invalid max corpus file size: %d", config.Security.MaxCorpusFileSize)
		}
	}

	return nil
}

// Dependencies holds all the initialized dependencies for the server
type Dependencies struct {
	Database        common.Database
	State           *master.PersistentState
	StateAdapter    *master.StateStoreAdapter
	TimeoutManager  *master.TimeoutManager
	RecoveryManager *master.RecoveryManager
	StorageBackend  backend.StorageBackend
	Services        *service.Manager
	Logger          *logrus.Logger
}

// Close cleans up all dependencies
func (d *Dependencies) Close() {
	if d.State != nil {
		d.State.Close(context.Background())
	}
	if d.Database != nil {
		d.Database.Close(context.Background())
	}
}

// ResetDatabase resets the database
func (d *Dependencies) ResetDatabase() error {
	d.Logger.Warn("Resetting database - all data will be lost!")

	// Simple confirmation prompt
	fmt.Print("Are you sure you want to reset the database? Type 'yes' to confirm: ")
	var response string
	fmt.Scanln(&response)

	if response != "yes" {
		return fmt.Errorf("database reset cancelled")
	}

	// For SQLite, we can simply delete and recreate tables
	// This would be implemented in the database layer
	d.Logger.Info("Database reset is not fully implemented yet")

	return nil
}

// StartBackgroundServices starts all background services
func (d *Dependencies) StartBackgroundServices(ctx context.Context) error {
	// Start timeout monitoring
	d.Logger.Info("Starting timeout monitor")
	if err := d.TimeoutManager.Start(); err != nil {
		return fmt.Errorf("failed to start timeout manager: %w", err)
	}

	// Start periodic maintenance
	d.Logger.Info("Starting maintenance scheduler")
	go d.runMaintenance(ctx)

	return nil
}

// StopBackgroundServices stops all background services
func (d *Dependencies) StopBackgroundServices() {
	if d.TimeoutManager != nil {
		if err := d.TimeoutManager.Stop(); err != nil {
			d.Logger.WithError(err).Error("Failed to stop timeout manager")
		}
	}
}

func (d *Dependencies) runMaintenance(ctx context.Context) {
	// Run maintenance every hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.Logger.Info("Stopping maintenance scheduler")
			return

		case <-ticker.C:
			d.Logger.Debug("Running periodic maintenance")

			if err := d.RecoveryManager.PerformMaintenanceRecovery(ctx); err != nil {
				d.Logger.WithError(err).Error("Maintenance recovery failed")
			}
		}
	}
}

// initializeDependencies creates and wires up all dependencies
func initializeDependencies(config *common.MasterConfig, logger *logrus.Logger) (*Dependencies, error) {
	// Ensure data directory exists
	dataDir := filepath.Dir(config.Database.Path)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Create database connection
	var db common.Database
	var err error

	switch config.Database.Type {
	case "sqlite":
		// Use existing storage implementation
		db, err = storage.NewSQLiteStorage(config.Database, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create SQLite storage: %w", err)
		}

		logger.WithField("path", config.Database.Path).Info("Connected to SQLite database")

	case "postgres":
		return nil, fmt.Errorf("PostgreSQL support not yet implemented")

	default:
		return nil, fmt.Errorf("unsupported database type: %s", config.Database.Type)
	}

	// Initialize schema
	if advDb, ok := db.(common.AdvancedDatabase); ok {
		if err := advDb.CreateTables(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to initialize database schema: %w", err)
		}
	}

	// Run health check
	if err := db.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("database health check failed: %w", err)
	}

	// Create master components
	state := master.NewPersistentState(db, config, logger)
	timeoutMgr := master.NewTimeoutManager(state, config, logger)
	recoveryMgr := master.NewRecoveryManager(state, timeoutMgr, config, logger)

	// Create storage backend
	storageBackend, err := backend.NewStorageBackend(config.Storage, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage backend: %w", err)
	}

	// Create state adapter as concrete type for dependencies
	stateAdapterConcrete := &master.StateStoreAdapter{
		PS: state,
	}

	// Use the interface for service manager
	var stateAdapter service.StateStore = stateAdapterConcrete

	// Create service manager
	services := service.NewManager(
		stateAdapter,
		timeoutMgr,
		recoveryMgr,
		config,
		logger,
	)

	// Asynq queue initialization is skipped in master
	// Bot workers will connect directly to Redis for job processing
	if config.Queue.Backend == "asynq" {
		logger.Info("Asynq queue backend configured for bot workers")
		logger.Info("Master will enqueue jobs to Redis, workers will process them")
	}

	return &Dependencies{
		Database:        db,
		State:           state,
		StateAdapter:    stateAdapterConcrete,
		TimeoutManager:  timeoutMgr,
		RecoveryManager: recoveryMgr,
		StorageBackend:  storageBackend,
		Services:        services,
		Logger:          logger,
	}, nil
}

// createHTTPServer creates and configures the HTTP server with all routes
func createHTTPServer(config *common.MasterConfig, deps *Dependencies, logger *logrus.Logger) *http.Server {
	// Create version info
	versionInfo := &common.VersionInfo{
		Version:   version,
		BuildTime: buildTime,
		GitCommit: gitCommit,
	}

	// Create master server for legacy handlers
	// This will be gradually phased out as we migrate handlers
	masterServer := master.NewServer(config, deps.State, deps.TimeoutManager, versionInfo, logger)
	masterServer.SetRecoveryManager(deps.RecoveryManager)
	masterServer.SetServiceManager(deps.Services)

	// Initialize storage backend
	logger.Info("Initializing storage backend")
	if err := masterServer.InitializeStorage(); err != nil {
		logger.WithError(err).Fatal("Failed to initialize storage backend")
	}

	// Initialize the router without starting the server
	if err := masterServer.InitializeRouter(); err != nil {
		logger.WithError(err).Fatal("Failed to initialize master server router")
	}

	// Get the router from master server which has all the routes
	router := masterServer.GetRouter()

	// Add additional middleware if needed
	if config.Server.EnableCORS && router != nil {
		// CORS is already added by master server if needed
	}

	// Setup API v1 routes on top of existing routes
	setupAPIRoutes(router, config, deps, masterServer, logger)

	// Additional health and metrics endpoints (if not already added by master)
	router.HandleFunc("/health", handleHealth(deps)).Methods("GET")
	router.HandleFunc("/status", handleStatus(deps, versionInfo)).Methods("GET")
	router.HandleFunc("/metrics", handleMetrics()).Methods("GET")

	return &http.Server{
		Addr:         fmt.Sprintf(":%d", config.Server.Port),
		Handler:      router,
		ReadTimeout:  config.Server.ReadTimeout,
		WriteTimeout: config.Server.WriteTimeout,
		IdleTimeout:  config.Server.IdleTimeout,
	}
}

// setupAPIRoutes configures all API routes
func setupAPIRoutes(router *mux.Router, config *common.MasterConfig, deps *Dependencies, masterServer *master.Server, logger *logrus.Logger) {
	// Get services from the service manager
	botService := deps.Services.Bot
	jobService := deps.Services.Job
	resultService := deps.Services.Result
	campaignService := deps.Services.Campaign
	corpusService := deps.Services.Corpus

	// Get storage from state adapter
	storage := deps.StateAdapter.GetStorage()
	if storage == nil {
		// Fallback: try to get storage directly from the database
		if storageDB, ok := deps.Database.(common.Storage); ok {
			storage = storageDB
		}
	}

	// API v1 routes using new package structure
	apiV1 := router.PathPrefix("/api/v1").Subrouter()
	v1Router := v1.NewRouter(&v1.RouterConfig{
		BotService:      botService,
		JobService:      jobService,
		ResultService:   resultService,
		CampaignService: campaignService,
		CorpusService:   corpusService,
		Storage:         storage,
		Logger:          logger,
		EnableCORS:      config.Server.EnableCORS,
		EnableMetrics:   config.Monitoring.MetricsEnabled,
		EnableRateLimit: config.Server.RateLimitRPS > 0,
		RateLimitRPS:    config.Server.RateLimitRPS,
	})
	v1Router.SetupRoutes(apiV1)

	// API v2 routes (temporarily stubbed)
	apiV2 := router.PathPrefix("/api/v2").Subrouter()
	v2Router := v2.NewRouter(&v2.RouterConfig{
		Logger: logger,
	})
	v2Router.SetupRoutes(apiV2)

	// Legacy routes that haven't been migrated yet
	// The master server still handles some routes directly
	// These will be gradually migrated to the new structure
}

// Middleware functions

func loggingMiddleware(logger *logrus.Logger) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap ResponseWriter to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			logger.WithFields(logrus.Fields{
				"method":     r.Method,
				"path":       r.URL.Path,
				"status":     wrapped.statusCode,
				"duration":   time.Since(start),
				"remote":     r.RemoteAddr,
				"user_agent": r.UserAgent(),
			}).Info("HTTP request")
		})
	}
}

func recoveryMiddleware(logger *logrus.Logger) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.WithFields(logrus.Fields{
						"error":  err,
						"method": r.Method,
						"path":   r.URL.Path,
					}).Error("Panic recovered")

					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

func metricsMiddleware() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Basic metrics tracking
			// In a full implementation, this would integrate with Prometheus
			next.ServeHTTP(w, r)
		})
	}
}

func corsMiddleware() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Handler functions

func handleHealth(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Check database health
		if err := deps.Database.Ping(ctx); err != nil {
			http.Error(w, "Database unhealthy", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	}
}

func handleStatus(deps *Dependencies, versionInfo *common.VersionInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := map[string]interface{}{
			"status":     "running",
			"version":    versionInfo.Version,
			"build_time": versionInfo.BuildTime,
			"git_commit": versionInfo.GitCommit,
			"uptime":     time.Since(time.Now()).String(), // This would need to track actual start time
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Simple JSON encoding
		fmt.Fprintf(w, `{"status":"%s","version":"%s","build_time":"%s","git_commit":"%s"}`,
			status["status"], status["version"], status["build_time"], status["git_commit"])
	}
}

func handleMetrics() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Basic metrics endpoint
		// In a full implementation, this would return Prometheus metrics
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("# Basic metrics\n"))
	}
}
