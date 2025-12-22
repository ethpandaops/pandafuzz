// Package config provides configuration management for PandaFuzz components.
//
// The package defines configuration structures for all system components
// including database, storage, queuing, and runtime settings. Configuration
// supports loading from YAML files, environment variables, and runtime defaults.
//
// # Configuration Loading
//
// Configuration is loaded in the following order (later sources override earlier):
//
//  1. Default values from Default*() functions
//  2. YAML configuration file values
//  3. Environment variables with PANDAFUZZ_ prefix
//
// # Default Values
//
// All configuration types have corresponding Default*() functions that return
// sensible defaults:
//
//	serverCfg := config.DefaultServerConfig()     // Host, port, timeouts
//	dbCfg := config.DefaultDatabaseConfig()       // SQLite, connections
//	storageCfg := config.DefaultStorageConfig()   // Filesystem backend
//	timeoutCfg := config.DefaultTimeoutConfig()   // Operation timeouts
//
// # Validation
//
// Configuration structs provide Validate() methods for checking correctness:
//
//	cfg := &config.DatabaseConfig{...}
//	if err := cfg.Validate(); err != nil {
//	    return fmt.Errorf("invalid database config: %w", err)
//	}
//
// # Environment Variables
//
// Environment variables use the PANDAFUZZ_ prefix with underscore-separated
// sections:
//
//	PANDAFUZZ_SERVER_PORT=9090
//	PANDAFUZZ_DATABASE_PATH=/data/pandafuzz.db
//	PANDAFUZZ_STORAGE_TYPE=s3
//	PANDAFUZZ_TIMEOUTS_BOT_HEARTBEAT=120s
//
// # Storage Configuration
//
// The package supports multiple storage backends:
//
//	// Filesystem storage (default)
//	storage := config.StorageConfig{
//	    Type: config.StorageTypeFilesystem,
//	    Filesystem: config.FilesystemConfig{
//	        BasePath: "./storage/corpus",
//	    },
//	}
//
//	// S3-compatible storage (MinIO or AWS S3)
//	storage := config.StorageConfig{
//	    Type: config.StorageTypeS3,
//	    S3: config.S3Config{
//	        Endpoint:  "localhost:9000",
//	        AccessKey: os.Getenv("AWS_ACCESS_KEY_ID"),
//	        SecretKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
//	        Bucket:    "pandafuzz-corpus",
//	    },
//	}
//
// # Database Configuration
//
// Database settings control SQLite connection behavior:
//
//	db := config.DatabaseConfig{
//	    Type:            "sqlite",
//	    Path:            "./data/pandafuzz.db",
//	    MaxOpenConns:    1,           // SQLite works best with 1 writer
//	    MaxIdleConns:    1,
//	    ConnMaxLifetime: 30 * time.Second,
//	}
package config
