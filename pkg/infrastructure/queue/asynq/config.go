package asynq

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/config"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// NewRedisOpt creates asynq Redis client options from config
func NewRedisOpt(cfg *config.RedisConfig) (asynq.RedisConnOpt, error) {
	if cfg == nil {
		return nil, fmt.Errorf("redis config is nil")
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid redis config: %w", err)
	}

	opt := asynq.RedisClientOpt{
		Addr:         cfg.Addr(),
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		PoolSize:     cfg.PoolSize,
	}

	// Configure TLS if enabled
	if cfg.TLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: cfg.TLSSkipVerify,
		}
		// Convert to Redis universal options for TLS support
		redisOpt := &redis.UniversalOptions{
			Addrs:        []string{cfg.Addr()},
			Password:     cfg.Password,
			DB:           cfg.DB,
			DialTimeout:  cfg.DialTimeout,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			PoolSize:     cfg.PoolSize,
			MinIdleConns: cfg.MinIdleConns,
			MaxRetries:   cfg.MaxRetries,
			TLSConfig:    tlsConfig,
		}
		return asynq.RedisClusterClientOpt{
			Addrs:        redisOpt.Addrs,
			Password:     redisOpt.Password,
			DialTimeout:  redisOpt.DialTimeout,
			ReadTimeout:  redisOpt.ReadTimeout,
			WriteTimeout: redisOpt.WriteTimeout,
			TLSConfig:    tlsConfig,
		}, nil
	}

	return opt, nil
}

// NewServerConfig creates asynq server configuration from queue config
func NewServerConfig(queueCfg *config.QueueConfig, redisCfg *config.RedisConfig) (*asynq.Config, error) {
	if queueCfg == nil {
		return nil, fmt.Errorf("queue config is nil")
	}

	if err := queueCfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid queue config: %w", err)
	}

	// Build queue map for asynq
	queues := make(map[string]int)
	for name, priority := range queueCfg.Queues {
		queues[name] = priority
	}

	cfg := &asynq.Config{
		Concurrency: queueCfg.Concurrency,
		Queues:      queues,

		// Retry configuration
		RetryDelayFunc: func(n int, e error, t *asynq.Task) time.Duration {
			if !queueCfg.Retry.RetryOnFailure {
				return 0 // No retry
			}

			delay := queueCfg.Retry.RetryDelay
			if queueCfg.Retry.ExponentialBackoff {
				// Exponential backoff: delay * 2^(n-1)
				for i := 1; i < n; i++ {
					delay *= 2
					if delay > queueCfg.Retry.MaxRetryDelay {
						delay = queueCfg.Retry.MaxRetryDelay
						break
					}
				}
			}
			return delay
		},

		// Error handling
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			// This will be overridden by the actual error handler in the server
			// Just a placeholder for now
		}),

		// Health check interval
		HealthCheckInterval: queueCfg.HealthCheckInterval,

		// Graceful shutdown
		ShutdownTimeout: queueCfg.ShutdownTimeout,

		// Logging level
		LogLevel: parseLogLevel(queueCfg.LogLevel),

		// Strict priority mode
		StrictPriority: queueCfg.StrictPriority,

		// Group aggregation settings
		GroupGracePeriod: queueCfg.GroupGracePeriod,
		GroupMaxDelay:    queueCfg.GroupMaxDelay,
		GroupMaxSize:     queueCfg.GroupMaxSize,
	}

	return cfg, nil
}

// NewInspectorOpt creates inspector options
func NewInspectorOpt(cfg *config.RedisConfig) (asynq.RedisConnOpt, error) {
	// Use the same options as the client
	return NewRedisOpt(cfg)
}

// GetDefaultQueues returns default queue configuration
func GetDefaultQueues() map[string]int {
	return map[string]int{
		QueueCritical: 6,
		QueueDefault:  3,
		QueueLow:      1,
	}
}

// parseLogLevel converts string log level to asynq.LogLevel
func parseLogLevel(level string) asynq.LogLevel {
	switch level {
	case "debug":
		return asynq.DebugLevel
	case "info":
		return asynq.InfoLevel
	case "warn", "warning":
		return asynq.WarnLevel
	case "error":
		return asynq.ErrorLevel
	case "fatal":
		return asynq.FatalLevel
	default:
		return asynq.InfoLevel
	}
}

// ServerOption represents a server configuration option
type ServerOption func(*asynq.Config)

// WithConcurrency sets the number of concurrent workers
func WithConcurrency(n int) ServerOption {
	return func(cfg *asynq.Config) {
		cfg.Concurrency = n
	}
}

// WithQueues sets the queue priority map
func WithQueues(queues map[string]int) ServerOption {
	return func(cfg *asynq.Config) {
		cfg.Queues = queues
	}
}

// WithStrictPriority enables strict priority mode
func WithStrictPriority() ServerOption {
	return func(cfg *asynq.Config) {
		cfg.StrictPriority = true
	}
}

// WithHealthCheckInterval sets the health check interval
func WithHealthCheckInterval(d time.Duration) ServerOption {
	return func(cfg *asynq.Config) {
		cfg.HealthCheckInterval = d
	}
}

// WithShutdownTimeout sets the graceful shutdown timeout
func WithShutdownTimeout(d time.Duration) ServerOption {
	return func(cfg *asynq.Config) {
		cfg.ShutdownTimeout = d
	}
}

// WithLogLevel sets the logging level
func WithLogLevel(level string) ServerOption {
	return func(cfg *asynq.Config) {
		cfg.LogLevel = parseLogLevel(level)
	}
}

// ApplyServerOptions applies server options to config
func ApplyServerOptions(cfg *asynq.Config, opts ...ServerOption) {
	for _, opt := range opts {
		opt(cfg)
	}
}
