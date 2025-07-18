package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// DatabaseConfig holds database configuration with enhanced features
type DatabaseConfig struct {
	// Basic Configuration
	Type     string            `yaml:"type" json:"type" validate:"required,oneof=sqlite postgres mysql"`
	Path     string            `yaml:"path" json:"path"`         // For SQLite
	Host     string            `yaml:"host" json:"host"`         // For PostgreSQL/MySQL
	Port     int               `yaml:"port" json:"port"`         // For PostgreSQL/MySQL
	Username string            `yaml:"username" json:"username"` // For PostgreSQL/MySQL
	Password string            `yaml:"password" json:"password"` // For PostgreSQL/MySQL
	Database string            `yaml:"database" json:"database"` // Database name
	SSLMode  string            `yaml:"ssl_mode" json:"ssl_mode"` // SSL mode for PostgreSQL
	Options  map[string]string `yaml:"options" json:"options"`   // Additional connection options

	// Connection Pooling
	MaxOpenConns    int           `yaml:"max_open_conns" json:"max_open_conns" validate:"min=0,max=1000"`
	MaxIdleConns    int           `yaml:"max_idle_conns" json:"max_idle_conns" validate:"min=0,max=1000"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" json:"conn_max_idle_time"`

	// SQLite Specific Settings
	Pragmas []string `yaml:"pragmas" json:"pragmas"` // SQLite PRAGMA statements

	// Retry Configuration
	RetryAttempts   int           `yaml:"retry_attempts" json:"retry_attempts" validate:"min=0,max=10"`
	RetryDelay      time.Duration `yaml:"retry_delay" json:"retry_delay"`
	RetryMaxDelay   time.Duration `yaml:"retry_max_delay" json:"retry_max_delay"`
	RetryMultiplier float64       `yaml:"retry_multiplier" json:"retry_multiplier" validate:"min=1,max=5"`
	RetryJitter     bool          `yaml:"retry_jitter" json:"retry_jitter"`

	// Timeouts
	ConnectionTimeout  time.Duration `yaml:"connection_timeout" json:"connection_timeout"`
	QueryTimeout       time.Duration `yaml:"query_timeout" json:"query_timeout"`
	TransactionTimeout time.Duration `yaml:"transaction_timeout" json:"transaction_timeout"`

	// Backup Configuration
	BackupEnabled   bool          `yaml:"backup_enabled" json:"backup_enabled"`
	BackupPath      string        `yaml:"backup_path" json:"backup_path"`
	BackupInterval  time.Duration `yaml:"backup_interval" json:"backup_interval"`
	BackupRetention time.Duration `yaml:"backup_retention" json:"backup_retention"`
	BackupCompress  bool          `yaml:"backup_compress" json:"backup_compress"`

	// Performance Tuning
	EnableQueryCache  bool `yaml:"enable_query_cache" json:"enable_query_cache"`
	QueryCacheSize    int  `yaml:"query_cache_size" json:"query_cache_size"`
	PreparedStmtCache bool `yaml:"prepared_stmt_cache" json:"prepared_stmt_cache"`
	StmtCacheSize     int  `yaml:"stmt_cache_size" json:"stmt_cache_size"`

	// Monitoring
	EnableMetrics      bool          `yaml:"enable_metrics" json:"enable_metrics"`
	SlowQueryLog       bool          `yaml:"slow_query_log" json:"slow_query_log"`
	SlowQueryThreshold time.Duration `yaml:"slow_query_threshold" json:"slow_query_threshold"`

	// Migration Settings
	MigrationsPath   string        `yaml:"migrations_path" json:"migrations_path"`
	AutoMigrate      bool          `yaml:"auto_migrate" json:"auto_migrate"`
	MigrationTimeout time.Duration `yaml:"migration_timeout" json:"migration_timeout"`
}

// SetDefaults sets default values for database configuration
func (dc *DatabaseConfig) SetDefaults() {
	// Connection pooling defaults
	if dc.MaxOpenConns == 0 {
		switch dc.Type {
		case "sqlite":
			dc.MaxOpenConns = 1 // SQLite doesn't benefit from multiple connections
		case "postgres", "mysql":
			dc.MaxOpenConns = 25
		default:
			dc.MaxOpenConns = 10
		}
	}

	if dc.MaxIdleConns == 0 {
		dc.MaxIdleConns = dc.MaxOpenConns / 4
		if dc.MaxIdleConns < 1 {
			dc.MaxIdleConns = 1
		}
	}

	if dc.ConnMaxLifetime == 0 {
		dc.ConnMaxLifetime = 30 * time.Minute
	}

	if dc.ConnMaxIdleTime == 0 {
		dc.ConnMaxIdleTime = 10 * time.Minute
	}

	// SQLite specific defaults
	if dc.Type == "sqlite" && len(dc.Pragmas) == 0 {
		dc.Pragmas = []string{
			"PRAGMA journal_mode=WAL",
			"PRAGMA synchronous=NORMAL",
			"PRAGMA cache_size=10000",
			"PRAGMA temp_store=MEMORY",
			"PRAGMA mmap_size=268435456", // 256MB
			"PRAGMA busy_timeout=5000",
		}
	}

	// Retry configuration defaults
	if dc.RetryAttempts == 0 {
		dc.RetryAttempts = 3
	}

	if dc.RetryDelay == 0 {
		dc.RetryDelay = 1 * time.Second
	}

	if dc.RetryMaxDelay == 0 {
		dc.RetryMaxDelay = 30 * time.Second
	}

	if dc.RetryMultiplier == 0 {
		dc.RetryMultiplier = 2.0
	}

	// Timeout defaults
	if dc.ConnectionTimeout == 0 {
		dc.ConnectionTimeout = 10 * time.Second
	}

	if dc.QueryTimeout == 0 {
		dc.QueryTimeout = 30 * time.Second
	}

	if dc.TransactionTimeout == 0 {
		dc.TransactionTimeout = 5 * time.Minute
	}

	// Backup defaults
	if dc.BackupEnabled && dc.BackupInterval == 0 {
		dc.BackupInterval = 24 * time.Hour
	}

	if dc.BackupEnabled && dc.BackupRetention == 0 {
		dc.BackupRetention = 7 * 24 * time.Hour // 7 days
	}

	// Performance defaults
	if dc.QueryCacheSize == 0 && dc.EnableQueryCache {
		dc.QueryCacheSize = 1000
	}

	if dc.StmtCacheSize == 0 && dc.PreparedStmtCache {
		dc.StmtCacheSize = 100
	}

	// Monitoring defaults
	if dc.SlowQueryThreshold == 0 && dc.SlowQueryLog {
		dc.SlowQueryThreshold = 1 * time.Second
	}

	// Migration defaults
	if dc.MigrationsPath == "" {
		dc.MigrationsPath = "./migrations"
	}

	if dc.MigrationTimeout == 0 {
		dc.MigrationTimeout = 5 * time.Minute
	}
}

// Validate validates the database configuration
func (dc *DatabaseConfig) Validate() error {
	// Validate database type
	switch dc.Type {
	case "sqlite":
		if dc.Path == "" {
			return fmt.Errorf("database path is required for SQLite")
		}
	case "postgres", "mysql":
		if dc.Host == "" {
			return fmt.Errorf("database host is required for %s", dc.Type)
		}
		if dc.Port == 0 {
			return fmt.Errorf("database port is required for %s", dc.Type)
		}
		if dc.Database == "" {
			return fmt.Errorf("database name is required for %s", dc.Type)
		}
	default:
		return fmt.Errorf("unsupported database type: %s", dc.Type)
	}

	// Validate connection pool settings
	if dc.MaxOpenConns < 0 {
		return fmt.Errorf("max_open_conns must be non-negative")
	}

	if dc.MaxIdleConns < 0 {
		return fmt.Errorf("max_idle_conns must be non-negative")
	}

	if dc.MaxIdleConns > dc.MaxOpenConns {
		return fmt.Errorf("max_idle_conns cannot exceed max_open_conns")
	}

	// Validate retry settings
	if dc.RetryAttempts < 0 {
		return fmt.Errorf("retry_attempts must be non-negative")
	}

	if dc.RetryMultiplier < 1.0 {
		return fmt.Errorf("retry_multiplier must be at least 1.0")
	}

	// Validate backup settings
	if dc.BackupEnabled && dc.BackupPath == "" {
		return fmt.Errorf("backup_path is required when backup is enabled")
	}

	// Validate performance settings
	if dc.EnableQueryCache && dc.QueryCacheSize <= 0 {
		return fmt.Errorf("query_cache_size must be positive when query cache is enabled")
	}

	if dc.PreparedStmtCache && dc.StmtCacheSize <= 0 {
		return fmt.Errorf("stmt_cache_size must be positive when statement cache is enabled")
	}

	return nil
}

// ApplyEnvironmentOverrides applies environment variable overrides to the configuration
func (dc *DatabaseConfig) ApplyEnvironmentOverrides(prefix string) error {
	if prefix == "" {
		prefix = "PANDAFUZZ_DB"
	}

	// Helper function to get environment variable with prefix
	getEnv := func(key string) string {
		return os.Getenv(fmt.Sprintf("%s_%s", prefix, key))
	}

	// Basic configuration overrides
	if val := getEnv("TYPE"); val != "" {
		dc.Type = val
	}

	if val := getEnv("PATH"); val != "" {
		dc.Path = val
	}

	if val := getEnv("HOST"); val != "" {
		dc.Host = val
	}

	if val := getEnv("PORT"); val != "" {
		port, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("invalid database port from environment: %s", val)
		}
		dc.Port = port
	}

	if val := getEnv("USERNAME"); val != "" {
		dc.Username = val
	}

	if val := getEnv("PASSWORD"); val != "" {
		dc.Password = val
	}

	if val := getEnv("DATABASE"); val != "" {
		dc.Database = val
	}

	if val := getEnv("SSL_MODE"); val != "" {
		dc.SSLMode = val
	}

	// Connection pooling overrides
	if val := getEnv("MAX_OPEN_CONNS"); val != "" {
		maxConns, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("invalid max_open_conns from environment: %s", val)
		}
		dc.MaxOpenConns = maxConns
	}

	if val := getEnv("MAX_IDLE_CONNS"); val != "" {
		maxIdle, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("invalid max_idle_conns from environment: %s", val)
		}
		dc.MaxIdleConns = maxIdle
	}

	if val := getEnv("CONN_MAX_LIFETIME"); val != "" {
		duration, err := time.ParseDuration(val)
		if err != nil {
			return fmt.Errorf("invalid conn_max_lifetime from environment: %s", val)
		}
		dc.ConnMaxLifetime = duration
	}

	// SQLite pragmas override
	if val := getEnv("PRAGMAS"); val != "" {
		dc.Pragmas = strings.Split(val, ";")
	}

	// Retry configuration overrides
	if val := getEnv("RETRY_ATTEMPTS"); val != "" {
		attempts, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("invalid retry_attempts from environment: %s", val)
		}
		dc.RetryAttempts = attempts
	}

	if val := getEnv("RETRY_DELAY"); val != "" {
		duration, err := time.ParseDuration(val)
		if err != nil {
			return fmt.Errorf("invalid retry_delay from environment: %s", val)
		}
		dc.RetryDelay = duration
	}

	// Backup configuration overrides
	if val := getEnv("BACKUP_ENABLED"); val != "" {
		enabled, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("invalid backup_enabled from environment: %s", val)
		}
		dc.BackupEnabled = enabled
	}

	if val := getEnv("BACKUP_PATH"); val != "" {
		dc.BackupPath = val
	}

	// Performance tuning overrides
	if val := getEnv("ENABLE_QUERY_CACHE"); val != "" {
		enabled, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("invalid enable_query_cache from environment: %s", val)
		}
		dc.EnableQueryCache = enabled
	}

	if val := getEnv("SLOW_QUERY_LOG"); val != "" {
		enabled, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("invalid slow_query_log from environment: %s", val)
		}
		dc.SlowQueryLog = enabled
	}

	// Migration overrides
	if val := getEnv("AUTO_MIGRATE"); val != "" {
		enabled, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("invalid auto_migrate from environment: %s", val)
		}
		dc.AutoMigrate = enabled
	}

	if val := getEnv("MIGRATIONS_PATH"); val != "" {
		dc.MigrationsPath = val
	}

	return nil
}

// GetConnectionString returns the database connection string
func (dc *DatabaseConfig) GetConnectionString() (string, error) {
	switch dc.Type {
	case "sqlite":
		// Build SQLite connection string with options
		var options []string
		options = append(options, fmt.Sprintf("file:%s", dc.Path))

		if dc.QueryTimeout > 0 {
			options = append(options, fmt.Sprintf("_timeout=%d", dc.QueryTimeout.Milliseconds()))
		}

		// Add custom options
		for k, v := range dc.Options {
			options = append(options, fmt.Sprintf("%s=%s", k, v))
		}

		return strings.Join(options, "?"), nil

	case "postgres":
		// Build PostgreSQL connection string
		var parts []string

		if dc.Host != "" {
			parts = append(parts, fmt.Sprintf("host=%s", dc.Host))
		}
		if dc.Port > 0 {
			parts = append(parts, fmt.Sprintf("port=%d", dc.Port))
		}
		if dc.Username != "" {
			parts = append(parts, fmt.Sprintf("user=%s", dc.Username))
		}
		if dc.Password != "" {
			parts = append(parts, fmt.Sprintf("password=%s", dc.Password))
		}
		if dc.Database != "" {
			parts = append(parts, fmt.Sprintf("dbname=%s", dc.Database))
		}
		if dc.SSLMode != "" {
			parts = append(parts, fmt.Sprintf("sslmode=%s", dc.SSLMode))
		}
		if dc.ConnectionTimeout > 0 {
			parts = append(parts, fmt.Sprintf("connect_timeout=%d", int(dc.ConnectionTimeout.Seconds())))
		}

		// Add custom options
		for k, v := range dc.Options {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}

		return strings.Join(parts, " "), nil

	case "mysql":
		// Build MySQL connection string
		var auth string
		if dc.Username != "" {
			auth = dc.Username
			if dc.Password != "" {
				auth = fmt.Sprintf("%s:%s", dc.Username, dc.Password)
			}
			auth += "@"
		}

		// Build connection string
		dsn := fmt.Sprintf("%stcp(%s:%d)/%s", auth, dc.Host, dc.Port, dc.Database)

		// Add parameters
		var params []string
		if dc.ConnectionTimeout > 0 {
			params = append(params, fmt.Sprintf("timeout=%s", dc.ConnectionTimeout))
		}
		if dc.QueryTimeout > 0 {
			params = append(params, fmt.Sprintf("readTimeout=%s", dc.QueryTimeout))
			params = append(params, fmt.Sprintf("writeTimeout=%s", dc.QueryTimeout))
		}

		// Add custom options
		for k, v := range dc.Options {
			params = append(params, fmt.Sprintf("%s=%s", k, v))
		}

		if len(params) > 0 {
			dsn += "?" + strings.Join(params, "&")
		}

		return dsn, nil

	default:
		return "", fmt.Errorf("unsupported database type: %s", dc.Type)
	}
}

// Clone creates a deep copy of the database configuration
func (dc *DatabaseConfig) Clone() *DatabaseConfig {
	clone := *dc

	// Deep copy slices
	if dc.Pragmas != nil {
		clone.Pragmas = make([]string, len(dc.Pragmas))
		copy(clone.Pragmas, dc.Pragmas)
	}

	// Deep copy maps
	if dc.Options != nil {
		clone.Options = make(map[string]string)
		for k, v := range dc.Options {
			clone.Options[k] = v
		}
	}

	return &clone
}

// GetSQLitePragmas returns the SQLite pragma statements
func (dc *DatabaseConfig) GetSQLitePragmas() []string {
	if dc.Type != "sqlite" {
		return nil
	}
	return dc.Pragmas
}

// IsRetryableError determines if an error should trigger a retry
func (dc *DatabaseConfig) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Common retryable error patterns
	retryablePatterns := []string{
		"database is locked",
		"too many connections",
		"connection refused",
		"connection reset",
		"timeout",
		"deadlock",
		"temporary failure",
		"server has gone away",
		"broken pipe",
		"connection closed",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}
