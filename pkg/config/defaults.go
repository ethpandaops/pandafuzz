package config

import "time"

// Default values for all configuration sections.
// These can be used as a reference for what values are expected.

// DefaultServerConfig returns default server configuration
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Host:           "0.0.0.0",
		Port:           8080,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
		EnableTLS:      false,
		EnableCORS:     false,
		RateLimitRPS:   100,
		RateLimitBurst: 200,
	}
}

// DefaultDatabaseConfig returns default database configuration
func DefaultDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		Type:            "sqlite",
		Path:            "./data/pandafuzz.db",
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 30 * time.Second,
	}
}

// DefaultTimeoutConfig returns default timeout configuration
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		BotHeartbeat:    60 * time.Second,
		JobExecution:    3600 * time.Second,
		MasterRecovery:  300 * time.Second,
		DatabaseOp:      10 * time.Second,
		DatabaseRetries: 5,
		HTTPRequest:     30 * time.Second,
		BotRegistration: 60 * time.Second,
		JobAssignment:   30 * time.Second,
		QueueBuffer:     300 * time.Second,
	}
}

// DefaultResourceLimits returns default resource limits
func DefaultResourceLimits() ResourceLimits {
	return ResourceLimits{
		MaxConcurrentJobs: 10,
		MaxCorpusSize:     1024 * 1024 * 1024, // 1GB
		MaxCrashSize:      10 * 1024 * 1024,   // 10MB
		MaxCrashCount:     1000,
		MaxJobDuration:    24 * time.Hour,
		MaxBotsPerCluster: 100,
		MaxPendingJobs:    1000,
	}
}

// DefaultCircuitConfig returns default circuit breaker configuration
func DefaultCircuitConfig() CircuitConfig {
	return CircuitConfig{
		MaxFailures:  5,
		ResetTimeout: 60 * time.Second,
		Enabled:      true,
	}
}

// DefaultMonitoringConfig returns default monitoring configuration
func DefaultMonitoringConfig() MonitoringConfig {
	return MonitoringConfig{
		Enabled:         true,
		MetricsEnabled:  true,
		MetricsPort:     9090,
		MetricsPath:     "/metrics",
		HealthEnabled:   true,
		HealthPath:      "/health",
		StatsInterval:   30 * time.Second,
		ProfilerEnabled: false,
		ProfilerPort:    6060,
	}
}

// DefaultSecurityConfig returns default security configuration
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		EnableInputValidation: true,
		EnableAuth:            true,
		AllowInsecure:         false,
		MaxRequestSize:        10 * 1024 * 1024, // 10MB
		AllowedFileExtensions: []string{".txt", ".bin", ".data", ".input"},
		ForbiddenPaths:        []string{"/etc", "/proc", "/sys"},
		EnableSanitization:    true,
		MaxCrashFileSize:      10 * 1024 * 1024, // 10MB
		MaxCorpusFileSize:     1024 * 1024,      // 1MB
		ProcessIsolationLevel: "sandbox",
	}
}

// DefaultLoggingConfig returns default logging configuration
func DefaultLoggingConfig() LoggingConfig {
	return LoggingConfig{
		Level:       "info",
		Format:      "json",
		Output:      "file",
		FilePath:    "./logs/pandafuzz.log",
		MaxSize:     100,
		MaxBackups:  10,
		MaxAge:      30,
		Compress:    true,
		EnableTrace: false,
	}
}

// DefaultRetryPolicy returns the default retry policy
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:   3,
		InitialDelay: time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
}

// DefaultStorageConfig returns default storage configuration (filesystem)
func DefaultStorageConfig() StorageConfig {
	return StorageConfig{
		Type: StorageTypeFilesystem,
		Filesystem: FilesystemConfig{
			BasePath: "./storage/corpus",
		},
		MaxFileSize:       100 * 1024 * 1024, // 100MB
		EnableDedup:       true,
		EnableCompression: false,
	}
}

// ServerConfig mirrors the common.ServerConfig for use in this package
type ServerConfig struct {
	Host           string        `yaml:"host" json:"host"`
	Port           int           `yaml:"port" json:"port"`
	ReadTimeout    time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout" json:"write_timeout"`
	IdleTimeout    time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
	MaxHeaderBytes int           `yaml:"max_header_bytes" json:"max_header_bytes"`
	EnableTLS      bool          `yaml:"enable_tls" json:"enable_tls"`
	EnableCORS     bool          `yaml:"enable_cors" json:"enable_cors"`
	RateLimitRPS   int           `yaml:"rate_limit_rps" json:"rate_limit_rps"`
	RateLimitBurst int           `yaml:"rate_limit_burst" json:"rate_limit_burst"`
}

// TimeoutConfig mirrors the common.TimeoutConfig for use in this package
type TimeoutConfig struct {
	BotHeartbeat    time.Duration `yaml:"bot_heartbeat" json:"bot_heartbeat"`
	JobExecution    time.Duration `yaml:"job_execution" json:"job_execution"`
	MasterRecovery  time.Duration `yaml:"master_recovery" json:"master_recovery"`
	DatabaseOp      time.Duration `yaml:"database_op" json:"database_op"`
	DatabaseRetries int           `yaml:"database_retries" json:"database_retries"`
	HTTPRequest     time.Duration `yaml:"http_request" json:"http_request"`
	BotRegistration time.Duration `yaml:"bot_registration" json:"bot_registration"`
	JobAssignment   time.Duration `yaml:"job_assignment" json:"job_assignment"`
	QueueBuffer     time.Duration `yaml:"queue_buffer" json:"queue_buffer"`
}

// ResourceLimits mirrors the common.ResourceLimits for use in this package
type ResourceLimits struct {
	MaxConcurrentJobs int           `yaml:"max_concurrent_jobs" json:"max_concurrent_jobs"`
	MaxCorpusSize     int64         `yaml:"max_corpus_size" json:"max_corpus_size"`
	MaxCrashSize      int64         `yaml:"max_crash_size" json:"max_crash_size"`
	MaxCrashCount     int           `yaml:"max_crash_count" json:"max_crash_count"`
	MaxJobDuration    time.Duration `yaml:"max_job_duration" json:"max_job_duration"`
	MaxBotsPerCluster int           `yaml:"max_bots_per_cluster" json:"max_bots_per_cluster"`
	MaxPendingJobs    int           `yaml:"max_pending_jobs" json:"max_pending_jobs"`
}

// CircuitConfig mirrors the common.CircuitConfig for use in this package
type CircuitConfig struct {
	MaxFailures  int           `yaml:"max_failures" json:"max_failures"`
	ResetTimeout time.Duration `yaml:"reset_timeout" json:"reset_timeout"`
	Enabled      bool          `yaml:"enabled" json:"enabled"`
}

// MonitoringConfig mirrors the common.MonitoringConfig for use in this package
type MonitoringConfig struct {
	Enabled         bool          `yaml:"enabled" json:"enabled"`
	MetricsEnabled  bool          `yaml:"metrics_enabled" json:"metrics_enabled"`
	MetricsPort     int           `yaml:"metrics_port" json:"metrics_port"`
	MetricsPath     string        `yaml:"metrics_path" json:"metrics_path"`
	HealthEnabled   bool          `yaml:"health_enabled" json:"health_enabled"`
	HealthPath      string        `yaml:"health_path" json:"health_path"`
	StatsInterval   time.Duration `yaml:"stats_interval" json:"stats_interval"`
	ProfilerEnabled bool          `yaml:"profiler_enabled" json:"profiler_enabled"`
	ProfilerPort    int           `yaml:"profiler_port" json:"profiler_port"`
}

// SecurityConfig mirrors the common.SecurityConfig for use in this package
type SecurityConfig struct {
	EnableInputValidation bool     `yaml:"enable_input_validation" json:"enable_input_validation"`
	EnableAuth            bool     `yaml:"enable_auth" json:"enable_auth"`
	AllowInsecure         bool     `yaml:"allow_insecure" json:"allow_insecure"`
	JWTSecret             string   `yaml:"jwt_secret" json:"jwt_secret"`
	APIKeys               map[string]string `yaml:"api_keys" json:"api_keys"`
	MaxRequestSize        int64    `yaml:"max_request_size" json:"max_request_size"`
	AllowedFileExtensions []string `yaml:"allowed_file_extensions" json:"allowed_file_extensions"`
	ForbiddenPaths        []string `yaml:"forbidden_paths" json:"forbidden_paths"`
	EnableSanitization    bool     `yaml:"enable_sanitization" json:"enable_sanitization"`
	MaxCrashFileSize      int64    `yaml:"max_crash_file_size" json:"max_crash_file_size"`
	MaxCorpusFileSize     int64    `yaml:"max_corpus_file_size" json:"max_corpus_file_size"`
	ProcessIsolationLevel string   `yaml:"process_isolation_level" json:"process_isolation_level"`
}

// LoggingConfig mirrors the common.LoggingConfig for use in this package
type LoggingConfig struct {
	Level       string `yaml:"level" json:"level"`
	Format      string `yaml:"format" json:"format"`
	Output      string `yaml:"output" json:"output"`
	FilePath    string `yaml:"file_path" json:"file_path"`
	MaxSize     int    `yaml:"max_size" json:"max_size"`
	MaxBackups  int    `yaml:"max_backups" json:"max_backups"`
	MaxAge      int    `yaml:"max_age" json:"max_age"`
	Compress    bool   `yaml:"compress" json:"compress"`
	EnableTrace bool   `yaml:"enable_trace" json:"enable_trace"`
}

// RetryPolicy defines retry behavior configuration
type RetryPolicy struct {
	MaxRetries   int           `yaml:"max_retries" json:"max_retries"`
	InitialDelay time.Duration `yaml:"initial_delay" json:"initial_delay"`
	MaxDelay     time.Duration `yaml:"max_delay" json:"max_delay"`
	Multiplier   float64       `yaml:"multiplier" json:"multiplier"`
}
