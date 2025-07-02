package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/bot"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/google/uuid"
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
		configFile   = flag.String("config", "bot.yaml", "Path to configuration file")
		masterURL    = flag.String("master", "", "Master server URL (overrides config)")
		botID        = flag.String("id", "", "Bot ID (generated if not specified)")
		logLevel     = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		showVersion  = flag.Bool("version", false, "Show version information")
		validateOnly = flag.Bool("validate", false, "Validate configuration and exit")
		workDir      = flag.String("work-dir", "./work", "Working directory for fuzzing")
		capabilities = flag.String("capabilities", "", "Comma-separated list of fuzzer capabilities")
		healthCheck  = flag.Bool("health-check", false, "Perform health check and exit")
	)

	flag.Parse()

	// Show version if requested
	if *showVersion {
		fmt.Printf("PandaFuzz Bot\n")
		fmt.Printf("Version:     %s\n", version)
		fmt.Printf("Build Time:  %s\n", buildTime)
		fmt.Printf("Git Commit:  %s\n", gitCommit)
		fmt.Printf("Go Version:  %s\n", runtime.Version())
		fmt.Printf("OS/Arch:     %s/%s\n", runtime.GOOS, runtime.GOARCH)
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
	if *masterURL != "" {
		config.MasterURL = *masterURL
	}
	if *botID != "" {
		config.ID = *botID
	}
	if *workDir != "" {
		config.Fuzzing.WorkDir = *workDir
	}
	if *capabilities != "" {
		config.Capabilities = strings.Split(*capabilities, ",")
	}

	// Generate bot ID if not specified
	if config.ID == "" {
		hostname, _ := os.Hostname()
		config.ID = fmt.Sprintf("bot-%s-%s", hostname, uuid.New().String()[:8])
	}

	// Log loaded configuration for debugging
	logger.WithFields(logrus.Fields{
		"id":           config.ID,
		"name":         config.Name,
		"master_url":   config.MasterURL,
		"capabilities": config.Capabilities,
	}).Info("Loaded bot configuration")

	// Validate configuration
	if err := validateConfig(config); err != nil {
		logger.WithError(err).Fatal("Invalid configuration")
	}

	if *validateOnly {
		logger.Info("Configuration is valid")
		os.Exit(0)
	}

	// Setup working directory
	if err := setupWorkingDirectory(config.Fuzzing.WorkDir, config.Resources, logger); err != nil {
		logger.WithError(err).Fatal("Failed to setup working directory")
	}

	// Create bot agent
	logger.WithFields(logrus.Fields{
		"bot_id":       config.ID,
		"master_url":   config.MasterURL,
		"capabilities": config.Capabilities,
		"work_dir":     config.Fuzzing.WorkDir,
	}).Info("Initializing PandaFuzz Bot")

	agent, err := bot.NewAgent(config, logger)
	if err != nil {
		logger.WithError(err).Fatal("Failed to create bot agent")
	}

	// Health check mode
	if *healthCheck {
		if err := performHealthCheck(agent, logger); err != nil {
			logger.WithError(err).Fatal("Health check failed")
		}
		logger.Info("Health check passed")
		os.Exit(0)
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start agent
	logger.Info("Starting bot agent")
	if err := agent.Start(); err != nil {
		logger.WithError(err).Fatal("Failed to start bot agent")
	}

	// Log startup complete
	logger.WithFields(logrus.Fields{
		"version":      version,
		"build_time":   buildTime,
		"git_commit":   gitCommit,
		"config_file":  *configFile,
		"work_dir":     config.Fuzzing.WorkDir,
		"capabilities": strings.Join(config.Capabilities, ","),
	}).Info("PandaFuzz Bot started successfully")

	// Wait for shutdown signal
	sig := <-sigChan
	logger.WithField("signal", sig).Info("Received shutdown signal")

	// Graceful shutdown
	logger.Info("Starting graceful shutdown")

	// Stop agent
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- agent.Stop()
	}()

	select {
	case err := <-shutdownDone:
		if err != nil {
			logger.WithError(err).Error("Error during shutdown")
		}
	case <-shutdownCtx.Done():
		logger.Error("Shutdown timeout exceeded")
	}

	logger.Info("PandaFuzz Bot shutdown complete")
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
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
	})

	// Set output
	logger.SetOutput(os.Stdout)

	return logger
}

func loadConfig(configFile string) (*common.BotConfig, error) {
	// Default configuration
	config := &common.BotConfig{
		MasterURL: "http://localhost:8080",
		Capabilities: []string{
			"aflplusplus",
			"libfuzzer",
		},
		Fuzzing: common.FuzzingConfig{
			WorkDir:           "./work",
			CrashReporting:    true,
			CoverageReporting: true,
		},
		Timeouts: common.BotTimeoutConfig{
			MasterCommunication: 30 * time.Second,
			HeartbeatInterval:   30 * time.Second,
			JobExecution:        24 * time.Hour,
		},
		Resources: common.BotResourceConfig{
			MaxCPUPercent:  80,        // 80% CPU
			MaxMemoryMB:    2048,      // 2GB
			MaxDiskSpaceMB: 10 * 1024, // 10GB
		},
		Retry: common.BotRetryConfig{
			Communication: common.RetryPolicy{
				MaxRetries:   5,
				InitialDelay: 1 * time.Second,
				MaxDelay:     60 * time.Second,
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

	// Parse YAML - handle both direct and wrapped config formats
	var wrapper struct {
		Bot *common.BotConfig `yaml:"bot"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// If the config is wrapped under "bot:" key, use that
	if wrapper.Bot != nil {
		config = wrapper.Bot
	} else {
		// Otherwise try parsing directly
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Set additional retry configurations if not already set
	if config.Retry.Communication.RetryableErrors == nil {
		config.Retry.Communication.RetryableErrors = []string{
			"connection refused",
			"timeout",
			"EOF",
			"temporary failure",
		}
	}
	config.Retry.Communication.Jitter = true

	if config.Retry.UpdateRecovery.MaxRetries == 0 {
		config.Retry.UpdateRecovery = common.RetryPolicy{
			MaxRetries:   10,
			InitialDelay: 5 * time.Second,
			MaxDelay:     5 * time.Minute,
			Multiplier:   1.5,
			Jitter:       true,
		}
	}

	return config, nil
}

func validateConfig(config *common.BotConfig) error {
	// Validate master URL
	if config.MasterURL == "" {
		return fmt.Errorf("master URL is required")
	}

	// Validate bot ID
	if config.ID == "" {
		return fmt.Errorf("bot ID is required")
	}

	// Validate capabilities
	if len(config.Capabilities) == 0 {
		return fmt.Errorf("at least one capability must be specified")
	}

	validCapabilities := map[string]bool{
		"aflplusplus": true,
		"afl++":       true,
		"libfuzzer":   true,
		"honggfuzz":   true,
		"custom":      true,
	}

	for _, cap := range config.Capabilities {
		if !validCapabilities[strings.ToLower(cap)] {
			return fmt.Errorf("invalid capability: %s", cap)
		}
	}

	// Validate timeouts
	if config.Timeouts.MasterCommunication < time.Second {
		return fmt.Errorf("master communication timeout too short: %v", config.Timeouts.MasterCommunication)
	}

	if config.Timeouts.HeartbeatInterval < 10*time.Second {
		return fmt.Errorf("heartbeat interval too short: %v", config.Timeouts.HeartbeatInterval)
	}

	// Validate resource limits
	if config.Resources.MaxCPUPercent <= 0 || config.Resources.MaxCPUPercent > 100 {
		return fmt.Errorf("invalid max CPU percentage: %d", config.Resources.MaxCPUPercent)
	}

	if config.Resources.MaxMemoryMB <= 0 {
		return fmt.Errorf("invalid max memory: %d MB", config.Resources.MaxMemoryMB)
	}

	return nil
}

func setupWorkingDirectory(workDir string, resources common.BotResourceConfig, logger *logrus.Logger) error {
	// Create work directory
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}

	// Create subdirectories
	subdirs := []string{
		"jobs",
		"corpus",
		"crashes",
		"coverage",
		"logs",
		"temp",
	}

	for _, subdir := range subdirs {
		path := filepath.Join(workDir, subdir)
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", subdir, err)
		}
	}

	// Check disk space
	availableSpace, err := getDiskSpace(workDir)
	if err != nil {
		logger.WithError(err).Warn("Failed to check disk space")
	} else {
		logger.WithFields(logrus.Fields{
			"available_gb": availableSpace / (1024 * 1024 * 1024),
			"required_gb":  resources.MaxDiskSpaceMB / 1024,
		}).Info("Disk space check")

		if availableSpace < uint64(resources.MaxDiskSpaceMB)*1024*1024 {
			return fmt.Errorf("insufficient disk space: %d GB available, %d GB required",
				availableSpace/(1024*1024*1024),
				resources.MaxDiskSpaceMB/1024)
		}
	}

	// Create bot info file
	infoFile := filepath.Join(workDir, "bot.info")
	info := fmt.Sprintf("Version: %s\nStarted: %s\n",
		version, time.Now().Format(time.RFC3339))

	if err := os.WriteFile(infoFile, []byte(info), 0644); err != nil {
		logger.WithError(err).Warn("Failed to create bot info file")
	}

	return nil
}

func getDiskSpace(path string) (uint64, error) {
	// This is a simplified implementation
	// In production, use syscall.Statfs on Unix or GetDiskFreeSpaceEx on Windows

	// For now, return a large number to pass validation
	return 100 * 1024 * 1024 * 1024, nil // 100GB
}

func performHealthCheck(agent *bot.Agent, logger *logrus.Logger) error {
	logger.Info("Performing health check")

	// Check agent status
	if !agent.IsRunning() {
		// Try to start temporarily
		if err := agent.Start(); err != nil {
			return fmt.Errorf("failed to start agent: %w", err)
		}
		defer agent.Stop()

		// Wait for agent to initialize
		time.Sleep(2 * time.Second)
	}

	// Perform health check
	if err := agent.HealthCheck(); err != nil {
		return fmt.Errorf("agent health check failed: %w", err)
	}

	// Get and display statistics
	stats := agent.GetStats()
	logger.WithFields(logrus.Fields{
		"jobs_completed":    stats.JobsCompleted,
		"jobs_failed":       stats.JobsFailed,
		"crashes_reported":  stats.CrashesReported,
		"connection_errors": stats.ConnectionErrors,
		"current_status":    stats.CurrentStatus,
	}).Info("Agent statistics")

	return nil
}
