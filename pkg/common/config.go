package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Config represents the complete PandaFuzz configuration
type Config struct {
	Master *MasterConfig `yaml:"master,omitempty" json:"master,omitempty"`
	Bot    *BotConfig    `yaml:"bot,omitempty" json:"bot,omitempty"`
}

// MasterConfig holds all master node configuration
type MasterConfig struct {
	Server     ServerConfig     `yaml:"server" json:"server" validate:"required"`
	Database   DatabaseConfig   `yaml:"database" json:"database" validate:"required"`
	Storage    StorageConfig    `yaml:"storage" json:"storage" validate:"required"`
	Timeouts   TimeoutConfig    `yaml:"timeouts" json:"timeouts" validate:"required"`
	Limits     ResourceLimits   `yaml:"limits" json:"limits" validate:"required"`
	Retry      RetryConfigs     `yaml:"retry" json:"retry"`
	Circuit    CircuitConfig    `yaml:"circuit" json:"circuit"`
	Monitoring MonitoringConfig `yaml:"monitoring" json:"monitoring"`
	Security   SecurityConfig   `yaml:"security" json:"security"`
	Logging    LoggingConfig    `yaml:"logging" json:"logging"`
}

// BotConfig holds all bot agent configuration  
type BotConfig struct {
	ID           string            `yaml:"id" json:"id" validate:"required"`
	Name         string            `yaml:"name" json:"name"`
	MasterURL    string            `yaml:"master_url" json:"master_url" validate:"required,url"`
	APIPort      int               `yaml:"api_port" json:"api_port"`
	Capabilities []string          `yaml:"capabilities" json:"capabilities" validate:"required"`
	Fuzzing      FuzzingConfig     `yaml:"fuzzing" json:"fuzzing" validate:"required"`
	Timeouts     BotTimeoutConfig  `yaml:"timeouts" json:"timeouts" validate:"required"`
	Retry        BotRetryConfig    `yaml:"retry" json:"retry"`
	Resources    BotResourceConfig `yaml:"resources" json:"resources"`
	Logging      LoggingConfig     `yaml:"logging" json:"logging"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host            string        `yaml:"host" json:"host"`
	Port            int           `yaml:"port" json:"port" validate:"required,min=1,max=65535"`
	ReadTimeout     time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout" json:"write_timeout"`
	IdleTimeout     time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
	MaxHeaderBytes  int           `yaml:"max_header_bytes" json:"max_header_bytes"`
	EnableTLS       bool          `yaml:"enable_tls" json:"enable_tls"`
	TLSCertFile     string        `yaml:"tls_cert_file" json:"tls_cert_file"`
	TLSKeyFile      string        `yaml:"tls_key_file" json:"tls_key_file"`
	EnableCORS      bool          `yaml:"enable_cors" json:"enable_cors"`
	CORSOrigins     []string      `yaml:"cors_origins" json:"cors_origins"`
	RateLimitRPS    int           `yaml:"rate_limit_rps" json:"rate_limit_rps"`
	RateLimitBurst  int           `yaml:"rate_limit_burst" json:"rate_limit_burst"`
}

// StorageConfig holds storage path configuration
type StorageConfig struct {
	BasePath    string `yaml:"base_path" json:"base_path" validate:"required"`
	CorpusPath  string `yaml:"corpus_path" json:"corpus_path"`
	CrashPath   string `yaml:"crash_path" json:"crash_path"`
	LogPath     string `yaml:"log_path" json:"log_path"`
	BackupPath  string `yaml:"backup_path" json:"backup_path"`
	TempPath    string `yaml:"temp_path" json:"temp_path"`
	Permissions int    `yaml:"permissions" json:"permissions"`
}

// TimeoutConfig holds all timeout configurations
type TimeoutConfig struct {
	BotHeartbeat    time.Duration `yaml:"bot_heartbeat" json:"bot_heartbeat" validate:"required"`
	JobExecution    time.Duration `yaml:"job_execution" json:"job_execution" validate:"required"`
	MasterRecovery  time.Duration `yaml:"master_recovery" json:"master_recovery" validate:"required"`
	DatabaseOp      time.Duration `yaml:"database_op" json:"database_op"`
	HTTPRequest     time.Duration `yaml:"http_request" json:"http_request"`
	BotRegistration time.Duration `yaml:"bot_registration" json:"bot_registration"`
	JobAssignment   time.Duration `yaml:"job_assignment" json:"job_assignment"`
}

// BotTimeoutConfig holds bot-specific timeout configurations
type BotTimeoutConfig struct {
	HeartbeatInterval     time.Duration `yaml:"heartbeat_interval" json:"heartbeat_interval" validate:"required"`
	JobExecution          time.Duration `yaml:"job_execution" json:"job_execution" validate:"required"`
	MasterCommunication   time.Duration `yaml:"master_communication" json:"master_communication" validate:"required"`
	FuzzerStartup         time.Duration `yaml:"fuzzer_startup" json:"fuzzer_startup"`
	ResultReporting       time.Duration `yaml:"result_reporting" json:"result_reporting"`
	FileUpload            time.Duration `yaml:"file_upload" json:"file_upload"`
}

// RetryConfigs holds all retry configurations for master
type RetryConfigs struct {
	Database     RetryPolicy `yaml:"database" json:"database"`
	BotOperation RetryPolicy `yaml:"bot_operation" json:"bot_operation"`
	FileSystem   RetryPolicy `yaml:"file_system" json:"file_system"`
	Network      RetryPolicy `yaml:"network" json:"network"`
}

// BotRetryConfig holds bot-specific retry configurations
type BotRetryConfig struct {
	Communication RetryPolicy `yaml:"communication" json:"communication"`
	UpdateRecovery RetryPolicy `yaml:"update_recovery" json:"update_recovery"`
	FileOperation RetryPolicy `yaml:"file_operation" json:"file_operation"`
}

// CircuitConfig holds circuit breaker configuration
type CircuitConfig struct {
	MaxFailures   int           `yaml:"max_failures" json:"max_failures"`
	ResetTimeout  time.Duration `yaml:"reset_timeout" json:"reset_timeout"`
	Enabled       bool          `yaml:"enabled" json:"enabled"`
}

// MonitoringConfig holds monitoring and observability configuration
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

// SecurityConfig holds security-related configuration
type SecurityConfig struct {
	EnableInputValidation  bool     `yaml:"enable_input_validation" json:"enable_input_validation"`
	MaxRequestSize         int64    `yaml:"max_request_size" json:"max_request_size"`
	AllowedFileExtensions  []string `yaml:"allowed_file_extensions" json:"allowed_file_extensions"`
	ForbiddenPaths         []string `yaml:"forbidden_paths" json:"forbidden_paths"`
	EnableSanitization     bool     `yaml:"enable_sanitization" json:"enable_sanitization"`
	MaxCrashFileSize       int64    `yaml:"max_crash_file_size" json:"max_crash_file_size"`
	MaxCorpusFileSize      int64    `yaml:"max_corpus_file_size" json:"max_corpus_file_size"`
	ProcessIsolationLevel  string   `yaml:"process_isolation_level" json:"process_isolation_level"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level       string `yaml:"level" json:"level" validate:"required"`
	Format      string `yaml:"format" json:"format"`
	Output      string `yaml:"output" json:"output"`
	FilePath    string `yaml:"file_path" json:"file_path"`
	MaxSize     int    `yaml:"max_size" json:"max_size"`
	MaxBackups  int    `yaml:"max_backups" json:"max_backups"`
	MaxAge      int    `yaml:"max_age" json:"max_age"`
	Compress    bool   `yaml:"compress" json:"compress"`
	EnableTrace bool   `yaml:"enable_trace" json:"enable_trace"`
}

// FuzzingConfig holds fuzzing-specific configuration
type FuzzingConfig struct {
	WorkDir           string            `yaml:"work_dir" json:"work_dir" validate:"required"`
	MaxJobs           int               `yaml:"max_jobs" json:"max_jobs"`
	JobCleanup        bool              `yaml:"job_cleanup" json:"job_cleanup"`
	CorpusSync        bool              `yaml:"corpus_sync" json:"corpus_sync"`
	CrashReporting    bool              `yaml:"crash_reporting" json:"crash_reporting"`
	CoverageReporting bool              `yaml:"coverage_reporting" json:"coverage_reporting"`
	FuzzerPaths       map[string]string `yaml:"fuzzer_paths" json:"fuzzer_paths"`
	DefaultDictionary string            `yaml:"default_dictionary" json:"default_dictionary"`
}

// BotResourceConfig holds bot resource limitations
type BotResourceConfig struct {
	MaxMemoryMB     int `yaml:"max_memory_mb" json:"max_memory_mb"`
	MaxCPUPercent   int `yaml:"max_cpu_percent" json:"max_cpu_percent"`
	MaxDiskSpaceMB  int `yaml:"max_disk_space_mb" json:"max_disk_space_mb"`
	MaxOpenFiles    int `yaml:"max_open_files" json:"max_open_files"`
	MaxProcesses    int `yaml:"max_processes" json:"max_processes"`
}

// ConfigManager handles configuration loading and validation
type ConfigManager struct {
	config     *Config
	configPath string
	viper      *viper.Viper
}

// NewConfigManager creates a new configuration manager
func NewConfigManager() *ConfigManager {
	v := viper.New()
	v.SetConfigType("yaml")
	v.AutomaticEnv()
	v.SetEnvPrefix("PANDAFUZZ")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	
	return &ConfigManager{
		viper: v,
	}
}

// LoadConfig loads configuration from file with environment variable substitution
func (cm *ConfigManager) LoadConfig(configPath string) (*Config, error) {
	cm.configPath = configPath
	
	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, NewValidationError("load_config", fmt.Errorf("config file not found: %s", configPath))
	}
	
	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, NewValidationError("read_config", err)
	}
	
	// Environment variable substitution
	expandedData := os.ExpandEnv(string(data))
	
	// Parse YAML
	var config Config
	if err := yaml.Unmarshal([]byte(expandedData), &config); err != nil {
		return nil, NewValidationError("parse_config", err)
	}
	
	// Set defaults
	cm.setDefaults(&config)
	
	// Validate configuration
	if err := cm.validateConfig(&config); err != nil {
		return nil, NewValidationError("validate_config", err)
	}
	
	cm.config = &config
	return &config, nil
}

// LoadMasterConfig loads only master configuration
func (cm *ConfigManager) LoadMasterConfig(configPath string) (*MasterConfig, error) {
	config, err := cm.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	
	if config.Master == nil {
		return nil, NewValidationError("load_master_config", fmt.Errorf("master configuration not found"))
	}
	
	return config.Master, nil
}

// LoadBotConfig loads only bot configuration
func (cm *ConfigManager) LoadBotConfig(configPath string) (*BotConfig, error) {
	config, err := cm.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	
	if config.Bot == nil {
		return nil, NewValidationError("load_bot_config", fmt.Errorf("bot configuration not found"))
	}
	
	return config.Bot, nil
}

// setDefaults sets default values for configuration
func (cm *ConfigManager) setDefaults(config *Config) {
	if config.Master != nil {
		cm.setMasterDefaults(config.Master)
	}
	
	if config.Bot != nil {
		cm.setBotDefaults(config.Bot)
	}
}

// setMasterDefaults sets default values for master configuration
func (cm *ConfigManager) setMasterDefaults(master *MasterConfig) {
	// Server defaults
	if master.Server.Host == "" {
		master.Server.Host = "0.0.0.0"
	}
	if master.Server.Port == 0 {
		master.Server.Port = 8080
	}
	if master.Server.ReadTimeout == 0 {
		master.Server.ReadTimeout = 30 * time.Second
	}
	if master.Server.WriteTimeout == 0 {
		master.Server.WriteTimeout = 30 * time.Second
	}
	if master.Server.IdleTimeout == 0 {
		master.Server.IdleTimeout = 60 * time.Second
	}
	if master.Server.MaxHeaderBytes == 0 {
		master.Server.MaxHeaderBytes = 1048576 // 1MB
	}
	
	// Database defaults
	if master.Database.Type == "" {
		master.Database.Type = "sqlite"
	}
	if master.Database.Path == "" {
		master.Database.Path = "/storage/data/pandafuzz.db"
	}
	if master.Database.MaxConns == 0 {
		master.Database.MaxConns = 1
	}
	if master.Database.IdleConns == 0 {
		master.Database.IdleConns = 1
	}
	if master.Database.Timeout == "" {
		master.Database.Timeout = "30s"
	}
	
	// Storage defaults
	if master.Storage.BasePath == "" {
		master.Storage.BasePath = "/storage"
	}
	if master.Storage.CorpusPath == "" {
		master.Storage.CorpusPath = filepath.Join(master.Storage.BasePath, "corpus")
	}
	if master.Storage.CrashPath == "" {
		master.Storage.CrashPath = filepath.Join(master.Storage.BasePath, "crashes")
	}
	if master.Storage.LogPath == "" {
		master.Storage.LogPath = filepath.Join(master.Storage.BasePath, "logs")
	}
	if master.Storage.BackupPath == "" {
		master.Storage.BackupPath = filepath.Join(master.Storage.BasePath, "backups")
	}
	if master.Storage.TempPath == "" {
		master.Storage.TempPath = filepath.Join(master.Storage.BasePath, "temp")
	}
	if master.Storage.Permissions == 0 {
		master.Storage.Permissions = 0755
	}
	
	// Timeout defaults
	if master.Timeouts.BotHeartbeat == 0 {
		master.Timeouts.BotHeartbeat = 60 * time.Second
	}
	if master.Timeouts.JobExecution == 0 {
		master.Timeouts.JobExecution = 3600 * time.Second
	}
	if master.Timeouts.MasterRecovery == 0 {
		master.Timeouts.MasterRecovery = 300 * time.Second
	}
	if master.Timeouts.DatabaseOp == 0 {
		master.Timeouts.DatabaseOp = 30 * time.Second
	}
	if master.Timeouts.HTTPRequest == 0 {
		master.Timeouts.HTTPRequest = 30 * time.Second
	}
	
	// Resource limits defaults
	if master.Limits.MaxConcurrentJobs == 0 {
		master.Limits.MaxConcurrentJobs = 10
	}
	if master.Limits.MaxCorpusSize == 0 {
		master.Limits.MaxCorpusSize = 1024 * 1024 * 1024 // 1GB
	}
	if master.Limits.MaxCrashSize == 0 {
		master.Limits.MaxCrashSize = 10 * 1024 * 1024 // 10MB
	}
	if master.Limits.MaxCrashCount == 0 {
		master.Limits.MaxCrashCount = 1000
	}
	if master.Limits.MaxJobDuration == 0 {
		master.Limits.MaxJobDuration = 24 * time.Hour
	}
	
	// Retry defaults
	if master.Retry.Database.MaxRetries == 0 {
		master.Retry.Database = DatabaseRetryPolicy
	}
	if master.Retry.BotOperation.MaxRetries == 0 {
		master.Retry.BotOperation = DefaultRetryPolicy
	}
	if master.Retry.Network.MaxRetries == 0 {
		master.Retry.Network = NetworkRetryPolicy
	}
	
	// Circuit breaker defaults
	if master.Circuit.MaxFailures == 0 {
		master.Circuit.MaxFailures = 5
	}
	if master.Circuit.ResetTimeout == 0 {
		master.Circuit.ResetTimeout = 60 * time.Second
	}
	
	// Monitoring defaults
	if master.Monitoring.MetricsPort == 0 {
		master.Monitoring.MetricsPort = 9090
	}
	if master.Monitoring.MetricsPath == "" {
		master.Monitoring.MetricsPath = "/metrics"
	}
	if master.Monitoring.HealthPath == "" {
		master.Monitoring.HealthPath = "/health"
	}
	if master.Monitoring.StatsInterval == 0 {
		master.Monitoring.StatsInterval = 30 * time.Second
	}
	
	// Security defaults
	if master.Security.MaxRequestSize == 0 {
		master.Security.MaxRequestSize = 10 * 1024 * 1024 // 10MB
	}
	if len(master.Security.AllowedFileExtensions) == 0 {
		master.Security.AllowedFileExtensions = []string{".txt", ".bin", ".data", ".input"}
	}
	if master.Security.MaxCrashFileSize == 0 {
		master.Security.MaxCrashFileSize = 10 * 1024 * 1024 // 10MB
	}
	if master.Security.MaxCorpusFileSize == 0 {
		master.Security.MaxCorpusFileSize = 1024 * 1024 // 1MB
	}
	if master.Security.ProcessIsolationLevel == "" {
		master.Security.ProcessIsolationLevel = "sandbox"
	}
	
	// Logging defaults
	if master.Logging.Level == "" {
		master.Logging.Level = "info"
	}
	if master.Logging.Format == "" {
		master.Logging.Format = "json"
	}
	if master.Logging.Output == "" {
		master.Logging.Output = "file"
	}
}

// setBotDefaults sets default values for bot configuration
func (cm *ConfigManager) setBotDefaults(bot *BotConfig) {
	// Generate bot ID if not provided
	if bot.ID == "" {
		hostname, _ := os.Hostname()
		bot.ID = fmt.Sprintf("bot-%s-%d", hostname, time.Now().Unix())
	}
	
	// API port default
	if bot.APIPort == 0 {
		bot.APIPort = 9049
	}
	
	// Fuzzing defaults
	if bot.Fuzzing.WorkDir == "" {
		bot.Fuzzing.WorkDir = "/tmp/pandafuzz"
	}
	if bot.Fuzzing.MaxJobs == 0 {
		bot.Fuzzing.MaxJobs = 1
	}
	bot.Fuzzing.JobCleanup = true
	bot.Fuzzing.CrashReporting = true
	bot.Fuzzing.CoverageReporting = true
	
	// Timeout defaults
	if bot.Timeouts.HeartbeatInterval == 0 {
		bot.Timeouts.HeartbeatInterval = 30 * time.Second
	}
	if bot.Timeouts.JobExecution == 0 {
		bot.Timeouts.JobExecution = 3600 * time.Second
	}
	if bot.Timeouts.MasterCommunication == 0 {
		bot.Timeouts.MasterCommunication = 30 * time.Second
	}
	if bot.Timeouts.FuzzerStartup == 0 {
		bot.Timeouts.FuzzerStartup = 60 * time.Second
	}
	if bot.Timeouts.ResultReporting == 0 {
		bot.Timeouts.ResultReporting = 30 * time.Second
	}
	
	// Retry defaults
	if bot.Retry.Communication.MaxRetries == 0 {
		bot.Retry.Communication = NetworkRetryPolicy
	}
	if bot.Retry.UpdateRecovery.MaxRetries == 0 {
		bot.Retry.UpdateRecovery = UpdateRetryPolicy
	}
	
	// Resource defaults
	if bot.Resources.MaxMemoryMB == 0 {
		bot.Resources.MaxMemoryMB = 2048 // 2GB
	}
	if bot.Resources.MaxCPUPercent == 0 {
		bot.Resources.MaxCPUPercent = 80
	}
	if bot.Resources.MaxDiskSpaceMB == 0 {
		bot.Resources.MaxDiskSpaceMB = 10240 // 10GB
	}
	if bot.Resources.MaxOpenFiles == 0 {
		bot.Resources.MaxOpenFiles = 1024
	}
	if bot.Resources.MaxProcesses == 0 {
		bot.Resources.MaxProcesses = 100
	}
	
	// Logging defaults
	if bot.Logging.Level == "" {
		bot.Logging.Level = "info"
	}
	if bot.Logging.Format == "" {
		bot.Logging.Format = "json"
	}
	if bot.Logging.Output == "" {
		bot.Logging.Output = "file"
	}
}

// validateConfig validates the configuration
func (cm *ConfigManager) validateConfig(config *Config) error {
	if config.Master != nil {
		if err := cm.validateMasterConfig(config.Master); err != nil {
			return err
		}
	}
	
	if config.Bot != nil {
		if err := cm.validateBotConfig(config.Bot); err != nil {
			return err
		}
	}
	
	return nil
}

// validateMasterConfig validates master configuration
func (cm *ConfigManager) validateMasterConfig(master *MasterConfig) error {
	// Validate server configuration
	if master.Server.Port < 1 || master.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", master.Server.Port)
	}
	
	// Validate database configuration
	if master.Database.Type != "sqlite" && master.Database.Type != "badger" && master.Database.Type != "memory" {
		return fmt.Errorf("unsupported database type: %s", master.Database.Type)
	}
	
	// Validate storage paths
	if master.Storage.BasePath == "" {
		return fmt.Errorf("storage base path is required")
	}
	
	// Validate timeouts
	if master.Timeouts.BotHeartbeat <= 0 {
		return fmt.Errorf("bot heartbeat timeout must be positive")
	}
	if master.Timeouts.JobExecution <= 0 {
		return fmt.Errorf("job execution timeout must be positive")
	}
	
	// Validate resource limits
	if master.Limits.MaxConcurrentJobs <= 0 {
		return fmt.Errorf("max concurrent jobs must be positive")
	}
	
	return nil
}

// validateBotConfig validates bot configuration
func (cm *ConfigManager) validateBotConfig(bot *BotConfig) error {
	// Validate bot ID
	if bot.ID == "" {
		return fmt.Errorf("bot ID is required")
	}
	
	// Validate master URL
	if bot.MasterURL == "" {
		return fmt.Errorf("master URL is required")
	}
	if !strings.HasPrefix(bot.MasterURL, "http://") && !strings.HasPrefix(bot.MasterURL, "https://") {
		return fmt.Errorf("master URL must be a valid HTTP/HTTPS URL")
	}
	
	// Validate capabilities
	if len(bot.Capabilities) == 0 {
		return fmt.Errorf("bot capabilities are required")
	}
	
	// Validate work directory
	if bot.Fuzzing.WorkDir == "" {
		return fmt.Errorf("fuzzing work directory is required")
	}
	
	return nil
}

// GetConfig returns the loaded configuration
func (cm *ConfigManager) GetConfig() *Config {
	return cm.config
}

// Reload reloads the configuration from file
func (cm *ConfigManager) Reload() error {
	if cm.configPath == "" {
		return fmt.Errorf("no config path set")
	}
	
	_, err := cm.LoadConfig(cm.configPath)
	return err
}

// SaveConfig saves configuration to file
func (cm *ConfigManager) SaveConfig(config *Config, path string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return NewValidationError("marshal_config", err)
	}
	
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return NewStorageError("create_config_dir", err)
	}
	
	// Write config file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return NewStorageError("write_config", err)
	}
	
	return nil
}