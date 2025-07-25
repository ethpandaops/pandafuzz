package abstraction

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// DriverConfig provides a generic configuration interface for drivers
type DriverConfig interface{}

// FactoryConfig contains configuration for creating storage drivers
type FactoryConfig struct {
	// Type specifies which storage driver to create
	Type Type

	// Config contains the driver-specific configuration
	Config DriverConfig

	// Composite contains composite storage configuration
	Composite *CompositeConfig

	// Middleware contains middleware configuration
	Middleware MiddlewareConfig
}

// Factory creates storage drivers with middleware support
type Factory struct {
	logger logrus.FieldLogger
}

// NewFactory creates a new storage factory
func NewFactory(logger logrus.FieldLogger) *Factory {
	return &Factory{
		logger: logger,
	}
}

// NewDriver creates a new storage driver based on the provided configuration
func (f *Factory) NewDriver(config FactoryConfig) (Driver, error) {
	var driver Driver
	var err error

	// Create base driver
	switch config.Type {
	case TypeComposite:
		driver, err = f.createCompositeDriver(config)
	default:
		// Use the registered driver constructor
		driver, err = CreateDriver(config.Type, config.Config, f.logger)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create %s driver: %w", config.Type, err)
	}

	// Apply middleware layers
	driver = f.applyMiddleware(driver, config.Middleware)

	return driver, nil
}

func (f *Factory) createCompositeDriver(config FactoryConfig) (Driver, error) {
	if config.Composite == nil {
		return nil, fmt.Errorf("composite configuration is required for composite driver")
	}

	// Create primary driver
	primary, err := f.NewDriver(config.Composite.Primary)
	if err != nil {
		return nil, fmt.Errorf("failed to create primary driver: %w", err)
	}

	// Create secondary drivers
	var secondaries []Driver
	for i, secConfig := range config.Composite.Secondaries {
		secondary, err := f.NewDriver(secConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create secondary driver %d: %w", i, err)
		}
		secondaries = append(secondaries, secondary)
	}

	return NewCompositeDriver(CompositeDriverConfig{
		Primary:     primary,
		Secondaries: secondaries,
		WriteMode:   config.Composite.WriteMode,
		ReadMode:    config.Composite.ReadMode,
		Logger:      f.logger,
	})
}

func (f *Factory) applyMiddleware(driver Driver, config MiddlewareConfig) Driver {
	// Apply middleware in order: caching -> retry -> metrics -> logging
	// This ensures logging sees all operations, metrics track retries, etc.

	if config.EnableCaching {
		driver = NewCacheMiddleware(driver, config.CacheConfig, f.logger)
	}

	if config.EnableRetry {
		driver = NewRetryMiddleware(driver, config.RetryConfig, f.logger)
	}

	if config.EnableMetrics {
		driver = NewMetricsMiddleware(driver, f.logger)
	}

	if config.EnableLogging {
		driver = NewLoggingMiddleware(driver, f.logger)
	}

	return driver
}

// DefaultMiddlewareConfig returns default middleware configuration
func DefaultMiddlewareConfig() MiddlewareConfig {
	return MiddlewareConfig{
		EnableLogging: true,
		EnableMetrics: true,
		EnableRetry:   true,
		EnableCaching: false,
		RetryConfig: RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     5 * time.Second,
			Multiplier:   2.0,
		},
		CacheConfig: CacheConfig{
			MaxSize:    100 * 1024 * 1024, // 100MB
			TTL:        5 * time.Minute,
			MaxEntries: 1000,
		},
	}
}
