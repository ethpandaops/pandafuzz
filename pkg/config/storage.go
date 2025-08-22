package config

import (
	"fmt"
	"time"
)

// StorageType defines the type of storage backend
type StorageType string

const (
	StorageTypeFilesystem StorageType = "filesystem"
	StorageTypeS3         StorageType = "s3"
	StorageTypeMinIO      StorageType = "minio"
)

// StorageConfig defines storage backend configuration
type StorageConfig struct {
	Type       StorageType      `yaml:"type" json:"type" default:"filesystem" validate:"required,oneof=filesystem s3 minio"`
	Filesystem FilesystemConfig `yaml:"filesystem,omitempty" json:"filesystem,omitempty"`
	S3         S3Config         `yaml:"s3,omitempty" json:"s3,omitempty"`
	MinIO      MinIOConfig      `yaml:"minio,omitempty" json:"minio,omitempty"`

	// Common settings
	MaxFileSize       int64 `yaml:"max_file_size" json:"max_file_size" default:"104857600"` // 100MB default
	EnableDedup       bool  `yaml:"enable_dedup" json:"enable_dedup" default:"true"`
	EnableCompression bool  `yaml:"enable_compression" json:"enable_compression" default:"false"`
}

// FilesystemConfig for local filesystem storage
type FilesystemConfig struct {
	BasePath string `yaml:"base_path" json:"base_path" default:"./storage/corpus" validate:"required_if=Type filesystem"`
}

// S3Config for AWS S3 or compatible services
type S3Config struct {
	Endpoint        string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Region          string `yaml:"region" json:"region" default:"us-east-1" validate:"required_if=Type s3"`
	AccessKeyID     string `yaml:"access_key_id" json:"access_key_id" env:"AWS_ACCESS_KEY_ID" validate:"required_if=Type s3"`
	SecretAccessKey string `yaml:"secret_access_key" json:"secret_access_key" env:"AWS_SECRET_ACCESS_KEY" validate:"required_if=Type s3"`
	SessionToken    string `yaml:"session_token,omitempty" json:"session_token,omitempty" env:"AWS_SESSION_TOKEN"`

	// Bucket configuration
	CorpusBucket     string `yaml:"corpus_bucket" json:"corpus_bucket" default:"pandafuzz-corpus" validate:"required_if=Type s3"`
	QuarantineBucket string `yaml:"quarantine_bucket" json:"quarantine_bucket" default:"pandafuzz-quarantine"`
	BackupBucket     string `yaml:"backup_bucket" json:"backup_bucket" default:"pandafuzz-backup"`
	CoverageBucket   string `yaml:"coverage_bucket" json:"coverage_bucket" default:"pandafuzz-coverage"`

	// S3 specific options
	UseSSL       bool `yaml:"use_ssl" json:"use_ssl" default:"true"`
	UsePathStyle bool `yaml:"use_path_style" json:"use_path_style" default:"false"`

	// Performance tuning
	PartSize    int64 `yaml:"part_size" json:"part_size" default:"67108864"` // 64MB default
	Concurrency int   `yaml:"concurrency" json:"concurrency" default:"4"`    // Upload/download concurrency

	// Retry configuration
	MaxRetries int           `yaml:"max_retries" json:"max_retries" default:"3"`
	RetryDelay time.Duration `yaml:"retry_delay" json:"retry_delay" default:"1s"`
}

// MinIOConfig extends S3Config with MinIO-specific settings
type MinIOConfig struct {
	S3Config `yaml:",inline"`

	// MinIO specific
	ConsoleAddress string `yaml:"console_address,omitempty" json:"console_address,omitempty" default:"localhost:9001"`
	HealthCheck    bool   `yaml:"health_check" json:"health_check" default:"true"`
}

// Validate ensures storage configuration is valid
func (c *StorageConfig) Validate() error {
	switch c.Type {
	case StorageTypeFilesystem:
		if c.Filesystem.BasePath == "" {
			return fmt.Errorf("filesystem storage requires base_path")
		}
	case StorageTypeS3:
		if c.S3.CorpusBucket == "" {
			return fmt.Errorf("S3 storage requires corpus_bucket")
		}
		if c.S3.AccessKeyID == "" || c.S3.SecretAccessKey == "" {
			return fmt.Errorf("S3 storage requires access_key_id and secret_access_key")
		}
		if c.S3.Region == "" {
			return fmt.Errorf("S3 storage requires region")
		}
	case StorageTypeMinIO:
		if c.MinIO.Endpoint == "" {
			c.MinIO.Endpoint = "localhost:9000" // Default MinIO endpoint
		}
		if c.MinIO.CorpusBucket == "" {
			return fmt.Errorf("MinIO storage requires corpus_bucket")
		}
		if c.MinIO.AccessKeyID == "" || c.MinIO.SecretAccessKey == "" {
			return fmt.Errorf("MinIO storage requires access_key_id and secret_access_key")
		}
	default:
		return fmt.Errorf("unsupported storage type: %s", c.Type)
	}

	// Validate common settings
	if c.MaxFileSize <= 0 {
		return fmt.Errorf("max_file_size must be positive")
	}

	return nil
}

// GetActiveConfig returns the active storage configuration based on type
func (c *StorageConfig) GetActiveConfig() interface{} {
	switch c.Type {
	case StorageTypeFilesystem:
		return c.Filesystem
	case StorageTypeS3:
		return c.S3
	case StorageTypeMinIO:
		return c.MinIO
	default:
		return nil
	}
}

// SetDefaults sets default values for storage configuration
func (c *StorageConfig) SetDefaults() {
	if c.Type == "" {
		c.Type = StorageTypeFilesystem
	}

	if c.MaxFileSize == 0 {
		c.MaxFileSize = 100 * 1024 * 1024 // 100MB
	}

	switch c.Type {
	case StorageTypeFilesystem:
		if c.Filesystem.BasePath == "" {
			c.Filesystem.BasePath = "./storage/corpus"
		}
	case StorageTypeS3:
		c.S3.SetDefaults()
	case StorageTypeMinIO:
		c.MinIO.SetDefaults()
	}
}

// SetDefaults sets default values for S3 configuration
func (c *S3Config) SetDefaults() {
	if c.Region == "" {
		c.Region = "us-east-1"
	}
	if c.CorpusBucket == "" {
		c.CorpusBucket = "pandafuzz-corpus"
	}
	if c.QuarantineBucket == "" {
		c.QuarantineBucket = "pandafuzz-quarantine"
	}
	if c.BackupBucket == "" {
		c.BackupBucket = "pandafuzz-backup"
	}
	if c.CoverageBucket == "" {
		c.CoverageBucket = "pandafuzz-coverage"
	}
	if c.PartSize == 0 {
		c.PartSize = 64 * 1024 * 1024 // 64MB
	}
	if c.Concurrency == 0 {
		c.Concurrency = 4
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = 3
	}
	if c.RetryDelay == 0 {
		c.RetryDelay = 1 * time.Second
	}
	// S3 defaults to SSL enabled
	c.UseSSL = true
}

// SetDefaults sets default values for MinIO configuration
func (c *MinIOConfig) SetDefaults() {
	if c.Endpoint == "" {
		c.Endpoint = "localhost:9000"
	}
	if c.ConsoleAddress == "" {
		c.ConsoleAddress = "localhost:9001"
	}
	if c.CorpusBucket == "" {
		c.CorpusBucket = "corpus"
	}
	if c.QuarantineBucket == "" {
		c.QuarantineBucket = "quarantine"
	}
	if c.BackupBucket == "" {
		c.BackupBucket = "backup"
	}
	if c.CoverageBucket == "" {
		c.CoverageBucket = "coverage"
	}
	if c.PartSize == 0 {
		c.PartSize = 64 * 1024 * 1024 // 64MB
	}
	if c.Concurrency == 0 {
		c.Concurrency = 4
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = 3
	}
	if c.RetryDelay == 0 {
		c.RetryDelay = 1 * time.Second
	}
	// MinIO defaults
	c.UseSSL = false      // Local MinIO typically doesn't use SSL
	c.UsePathStyle = true // MinIO requires path-style access
	c.HealthCheck = true  // Enable health checks by default
}
