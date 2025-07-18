package backend

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/config"
	"github.com/ethpandaops/pandafuzz/pkg/monitoring"
)

// NewStorageBackend creates a storage backend based on configuration.
// It validates the configuration and returns an appropriate backend implementation.
func NewStorageBackend(cfg config.StorageConfig, logger logrus.FieldLogger) (StorageBackend, error) {
	// Validate configuration first
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid storage configuration: %w", err)
	}

	// Create nil metrics for migration tool (it doesn't need monitoring)
	var metrics *monitoring.StorageMetrics = nil

	switch cfg.Type {
	case config.StorageTypeFilesystem:
		return NewFilesystemBackend(cfg.Filesystem, logger, metrics)

	case config.StorageTypeS3:
		return NewS3Backend(cfg.S3, logger, metrics)

	case config.StorageTypeMinIO:
		// MinIO uses the same S3 backend
		return NewS3Backend(cfg.MinIO.S3Config, logger, metrics)

	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Type)
	}
}

// MigrateBackend creates a storage backend from raw configuration.
// This is used by the migration tool to create backends from different configuration types.
func MigrateBackend(cfg interface{}, logger logrus.FieldLogger) (StorageBackend, error) {
	// Create nil metrics for migration tool
	var metrics *monitoring.StorageMetrics = nil

	switch c := cfg.(type) {
	case config.FilesystemConfig:
		return NewFilesystemBackend(c, logger, metrics)
	case config.S3Config:
		return NewS3Backend(c, logger, metrics)
	case config.MinIOConfig:
		return NewS3Backend(c.S3Config, logger, metrics)
	default:
		return nil, fmt.Errorf("unsupported configuration type: %T", cfg)
	}
}

// NewStorageBackendWithMetrics creates a storage backend with monitoring capabilities.
// This should be used by production services that need monitoring.
func NewStorageBackendWithMetrics(cfg config.StorageConfig, logger logrus.FieldLogger, metrics *monitoring.StorageMetrics) (StorageBackend, error) {
	// Validate configuration first
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid storage configuration: %w", err)
	}

	// Log storage backend type
	logger.WithField("storage_type", cfg.Type).Info("Creating storage backend")

	switch cfg.Type {
	case config.StorageTypeFilesystem:
		return NewFilesystemBackend(cfg.Filesystem, logger, metrics)

	case config.StorageTypeS3:
		return NewS3Backend(cfg.S3, logger, metrics)

	case config.StorageTypeMinIO:
		// MinIO uses the same S3 backend
		return NewS3Backend(cfg.MinIO.S3Config, logger, metrics)

	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Type)
	}
}
