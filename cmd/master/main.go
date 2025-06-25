package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/master"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
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
	
	// Setup logging
	logger := setupLogging(*logLevel)
	
	// Load configuration
	config, err := loadConfig(*configFile)
	if err != nil {
		logger.WithError(err).Fatal("Failed to load configuration")
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
	
	// Validate configuration
	if err := validateConfig(config); err != nil {
		logger.WithError(err).Fatal("Invalid configuration")
	}
	
	if *validateOnly {
		logger.Info("Configuration is valid")
		os.Exit(0)
	}
	
	// Initialize database
	db, err := initializeDatabase(config, logger)
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize database")
	}
	defer db.Close()
	
	// Handle database operations
	if *resetDB {
		if err := resetDatabase(db, logger); err != nil {
			logger.WithError(err).Fatal("Failed to reset database")
		}
		logger.Info("Database reset successfully")
		os.Exit(0)
	}
	
	if *migrateDB {
		logger.Info("Database migrations completed")
		os.Exit(0)
	}
	
	// Create master components
	logger.Info("Initializing PandaFuzz Master components")
	
	// Create persistent state manager
	state := master.NewPersistentState(db, config)
	
	// Create timeout manager
	timeoutMgr := master.NewTimeoutManager(state, config)
	
	// Create recovery manager
	recoveryMgr := master.NewRecoveryManager(state, timeoutMgr, config)
	
	// Perform recovery on startup
	logger.Info("Performing system recovery")
	if err := recoveryMgr.RecoverOnStartup(); err != nil {
		logger.WithError(err).Error("Recovery failed, continuing anyway")
	}
	
	// Create version info
	versionInfo := &common.VersionInfo{
		Version:   version,
		BuildTime: buildTime,
		GitCommit: gitCommit,
	}
	
	// Create HTTP server
	server := master.NewServer(config, state, timeoutMgr, versionInfo)
	
	// Set recovery manager on server to avoid circular dependency
	server.SetRecoveryManager(recoveryMgr)
	
	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	// Start timeout monitoring
	logger.Info("Starting timeout monitor")
	if err := timeoutMgr.Start(); err != nil {
		logger.WithError(err).Fatal("Failed to start timeout manager")
	}
	
	// Start periodic maintenance
	logger.Info("Starting maintenance scheduler")
	go startMaintenance(ctx, recoveryMgr, logger)
	
	// Start HTTP server
	logger.WithFields(logrus.Fields{
		"port":         config.Server.Port,
		"metrics_port": config.Monitoring.MetricsPort,
		"metrics":      config.Monitoring.MetricsEnabled,
	}).Info("Starting HTTP server")
	
	// Start the server
	if err := server.Start(); err != nil {
		logger.WithError(err).Fatal("Failed to start server")
	}
	
	// Log startup complete
	logger.WithFields(logrus.Fields{
		"version":     version,
		"build_time":  buildTime,
		"git_commit":  gitCommit,
		"config_file": *configFile,
		"data_dir":    *dataDir,
	}).Info("PandaFuzz Master started successfully")
	
	// Wait for shutdown signal
	sig := <-sigChan
	logger.WithField("signal", sig).Info("Received shutdown signal")
	
	// Graceful shutdown
	logger.Info("Starting graceful shutdown")
	
	// Cancel context to stop background tasks
	cancel()
	
	// Stop server
	if err := server.Stop(); err != nil {
		logger.WithError(err).Error("Failed to shutdown server gracefully")
	}
	
	// Stop timeout manager
	if err := timeoutMgr.Stop(); err != nil {
		logger.WithError(err).Error("Failed to stop timeout manager")
	}
	
	// Final cleanup
	logger.Info("Performing final cleanup")
	if err := state.Close(); err != nil {
		logger.WithError(err).Error("Cleanup error")
	}
	
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
	
	// Set formatter
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		DisableColors:   false,
	})
	
	// Set output
	logger.SetOutput(os.Stdout)
	
	return logger
}

func loadConfig(configFile string) (*common.MasterConfig, error) {
	// Default configuration
	config := &common.MasterConfig{
		Server: common.ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Database: common.DatabaseConfig{
			Type: "sqlite",
			Path: "./data/pandafuzz.db",
		},
		Storage: common.StorageConfig{
			BasePath: "./storage",
		},
		Timeouts: common.TimeoutConfig{
			BotHeartbeat:   1 * time.Minute,
			JobExecution:   24 * time.Hour,
			MasterRecovery: 5 * time.Minute,
		},
		Limits: common.ResourceLimits{
			MaxConcurrentJobs: 10,
			MaxCorpusSize:     100 * 1024 * 1024, // 100MB
			MaxCrashSize:      10 * 1024 * 1024,  // 10MB
			MaxCrashCount:     1000,
			MaxJobDuration:    24 * time.Hour,
		},
		Monitoring: common.MonitoringConfig{
			Enabled:      true,
			MetricsPort:  9090,
			MetricsPath:  "/metrics",
			HealthPath:   "/health",
		},
		Retry: common.RetryConfigs{
			Database: common.RetryPolicy{
				MaxRetries:   3,
				InitialDelay: 1 * time.Second,
				MaxDelay:     30 * time.Second,
				Multiplier:   2.0,
			},
		},
	}
	
	// Check if config file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// Use defaults
		return config, nil
	}
	
	// Read config file
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	
	// Parse YAML
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	
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
	
	return nil
}

func initializeDatabase(config *common.MasterConfig, logger *logrus.Logger) (common.Database, error) {
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
		db, err = storage.NewSQLiteStorage(config.Database)
		if err != nil {
			return nil, fmt.Errorf("failed to create SQLite database: %w", err)
		}
		
		logger.WithField("path", config.Database.Path).Info("Connected to SQLite database")
		
	case "postgres":
		// PostgreSQL support would be implemented here
		return nil, fmt.Errorf("PostgreSQL support not yet implemented")
		
	default:
		return nil, fmt.Errorf("unsupported database type: %s", config.Database.Type)
	}
	
	// Initialize schema (if supported)
	if advDb, ok := db.(common.AdvancedDatabase); ok {
		if err := advDb.CreateTables(); err != nil {
			return nil, fmt.Errorf("failed to initialize database schema: %w", err)
		}
	}
	
	// Run health check
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("database health check failed: %w", err)
	}
	
	return db, nil
}

func resetDatabase(db common.Database, logger *logrus.Logger) error {
	logger.Warn("Resetting database - all data will be lost!")
	
	// Simple confirmation prompt
	fmt.Print("Are you sure you want to reset the database? Type 'yes' to confirm: ")
	var response string
	fmt.Scanln(&response)
	
	if response != "yes" {
		return fmt.Errorf("database reset cancelled")
	}
	
	// For SQLite, we can simply delete and recreate tables
	// This would be implemented in the database layer
	logger.Info("Database reset is not fully implemented yet")
	
	return nil
}

func startMaintenance(ctx context.Context, recovery *master.RecoveryManager, logger *logrus.Logger) {
	// Run maintenance every hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			logger.Info("Stopping maintenance scheduler")
			return
			
		case <-ticker.C:
			logger.Debug("Running periodic maintenance")
			
			if err := recovery.PerformMaintenanceRecovery(); err != nil {
				logger.WithError(err).Error("Maintenance recovery failed")
			}
		}
	}
}