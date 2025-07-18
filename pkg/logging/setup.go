package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// SetupLogger configures logrus based on the logging configuration
func SetupLogger(cfg common.LoggingConfig) (logrus.FieldLogger, error) {
	logger := logrus.New()

	// Set log level
	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level %s: %w", cfg.Level, err)
	}
	logger.SetLevel(level)

	// Set formatter
	switch strings.ToLower(cfg.Format) {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "@timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
			},
		})
	case "text", "":
		logger.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000",
			FullTimestamp:   true,
		})
	default:
		return nil, fmt.Errorf("unsupported log format: %s", cfg.Format)
	}

	// Set output
	output, err := setupOutput(cfg)
	if err != nil {
		return nil, err
	}
	logger.SetOutput(output)

	// Enable trace logging if configured
	if cfg.EnableTrace {
		logger.SetLevel(logrus.TraceLevel)
		logger.Trace("Trace logging enabled")
	}

	// Add common fields
	standardLogger := logger.WithFields(logrus.Fields{
		"service": "pandafuzz",
		"version": getVersion(),
		"pid":     os.Getpid(),
	})

	return standardLogger, nil
}

// setupOutput configures the log output based on configuration
func setupOutput(cfg common.LoggingConfig) (io.Writer, error) {
	switch strings.ToLower(cfg.Output) {
	case "stdout", "":
		return os.Stdout, nil
	case "stderr":
		return os.Stderr, nil
	case "file":
		if cfg.FilePath == "" {
			return nil, fmt.Errorf("file path required for file output")
		}
		return setupFileOutput(cfg)
	case "both":
		// Log to both stdout and file
		if cfg.FilePath == "" {
			return nil, fmt.Errorf("file path required for 'both' output")
		}
		fileWriter, err := setupFileOutput(cfg)
		if err != nil {
			return nil, err
		}
		return io.MultiWriter(os.Stdout, fileWriter), nil
	default:
		return nil, fmt.Errorf("unsupported output type: %s", cfg.Output)
	}
}

// setupFileOutput creates a file writer with rotation support
func setupFileOutput(cfg common.LoggingConfig) (io.Writer, error) {
	// Ensure directory exists
	dir := filepath.Dir(cfg.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Use lumberjack for log rotation
	return &lumberjack.Logger{
		Filename:   cfg.FilePath,
		MaxSize:    cfg.MaxSize,    // megabytes
		MaxBackups: cfg.MaxBackups, // number of backups
		MaxAge:     cfg.MaxAge,     // days
		Compress:   cfg.Compress,
		LocalTime:  true,
	}, nil
}

// SetupStructuredLogger creates a logger with structured logging fields
func SetupStructuredLogger(base logrus.FieldLogger, component string, fields map[string]interface{}) logrus.FieldLogger {
	logger := base.WithField("component", component)

	// Add additional fields if provided
	if fields != nil {
		logger = logger.WithFields(fields)
	}

	// Add hostname
	if hostname, err := os.Hostname(); err == nil {
		logger = logger.WithField("hostname", hostname)
	}

	return logger
}

// LoggerWithContext creates a logger with context fields
func LoggerWithContext(logger logrus.FieldLogger, ctx map[string]interface{}) logrus.FieldLogger {
	return logger.WithFields(ctx)
}

// getVersion returns the application version (placeholder)
func getVersion() string {
	// In production, this would read from a version file or build info
	return "dev"
}

// SetupComponentLogger creates a logger for a specific component with recommended fields
func SetupComponentLogger(base logrus.FieldLogger, component string) logrus.FieldLogger {
	switch component {
	case "storage":
		return base.WithFields(logrus.Fields{
			"component": "storage",
			"subsystem": "backend",
		})
	case "corpus":
		return base.WithFields(logrus.Fields{
			"component": "corpus",
			"subsystem": "service",
		})
	case "bot":
		return base.WithFields(logrus.Fields{
			"component": "bot",
			"subsystem": "agent",
		})
	case "master":
		return base.WithFields(logrus.Fields{
			"component": "master",
			"subsystem": "server",
		})
	default:
		return base.WithField("component", component)
	}
}

// Hook for adding custom log processing
type MetricsHook struct {
	counter map[logrus.Level]uint64
}

// NewMetricsHook creates a hook that counts log messages by level
func NewMetricsHook() *MetricsHook {
	return &MetricsHook{
		counter: make(map[logrus.Level]uint64),
	}
}

// Levels returns the levels this hook is interested in
func (h *MetricsHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire is called when a log entry is fired
func (h *MetricsHook) Fire(entry *logrus.Entry) error {
	h.counter[entry.Level]++
	return nil
}

// GetCounts returns the current log level counts
func (h *MetricsHook) GetCounts() map[logrus.Level]uint64 {
	counts := make(map[logrus.Level]uint64)
	for level, count := range h.counter {
		counts[level] = count
	}
	return counts
}
