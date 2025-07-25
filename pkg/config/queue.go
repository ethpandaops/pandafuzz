package config

import (
	"fmt"
	"time"
)

// QueueConfig holds queue system configuration
type QueueConfig struct {
	// Backend specifies the queue backend (memory or asynq)
	Backend string `json:"backend" yaml:"backend" mapstructure:"backend"`

	// Concurrency is the number of concurrent workers
	Concurrency int `json:"concurrency" yaml:"concurrency" mapstructure:"concurrency"`

	// Queues maps queue names to their priority weights
	Queues map[string]int `json:"queues" yaml:"queues" mapstructure:"queues"`

	// StrictPriority enables strict priority queue processing
	StrictPriority bool `json:"strict_priority" yaml:"strict_priority" mapstructure:"strict_priority"`

	// RetryConfig holds retry configuration
	Retry RetryConfig `json:"retry" yaml:"retry" mapstructure:"retry"`

	// HealthCheckInterval is how often to check worker health
	HealthCheckInterval time.Duration `json:"health_check_interval" yaml:"health_check_interval" mapstructure:"health_check_interval"`

	// ShutdownTimeout is the graceful shutdown timeout
	ShutdownTimeout time.Duration `json:"shutdown_timeout" yaml:"shutdown_timeout" mapstructure:"shutdown_timeout"`

	// LogLevel controls queue system logging verbosity
	LogLevel string `json:"log_level" yaml:"log_level" mapstructure:"log_level"`

	// DisableRedisClusterMode disables Redis cluster mode
	DisableRedisClusterMode bool `json:"disable_redis_cluster_mode" yaml:"disable_redis_cluster_mode" mapstructure:"disable_redis_cluster_mode"`

	// GroupGracePeriod is the grace period for task groups
	GroupGracePeriod time.Duration `json:"group_grace_period" yaml:"group_grace_period" mapstructure:"group_grace_period"`

	// GroupMaxDelay is the maximum delay for task groups
	GroupMaxDelay time.Duration `json:"group_max_delay" yaml:"group_max_delay" mapstructure:"group_max_delay"`

	// GroupMaxSize is the maximum size for task groups
	GroupMaxSize int `json:"group_max_size" yaml:"group_max_size" mapstructure:"group_max_size"`
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int `json:"max_retries" yaml:"max_retries" mapstructure:"max_retries"`

	// RetryDelay is the base delay between retries
	RetryDelay time.Duration `json:"retry_delay" yaml:"retry_delay" mapstructure:"retry_delay"`

	// MaxRetryDelay is the maximum delay between retries
	MaxRetryDelay time.Duration `json:"max_retry_delay" yaml:"max_retry_delay" mapstructure:"max_retry_delay"`

	// ExponentialBackoff enables exponential backoff for retries
	ExponentialBackoff bool `json:"exponential_backoff" yaml:"exponential_backoff" mapstructure:"exponential_backoff"`

	// RetryOnFailure determines which errors trigger retries
	RetryOnFailure bool `json:"retry_on_failure" yaml:"retry_on_failure" mapstructure:"retry_on_failure"`
}

// QueuePriority defines standard queue priorities
const (
	QueuePriorityCritical = "critical"
	QueuePriorityDefault  = "default"
	QueuePriorityLow      = "low"
)

// DefaultQueueConfig returns default queue configuration
func DefaultQueueConfig() *QueueConfig {
	return &QueueConfig{
		Backend:     "memory",
		Concurrency: 10,
		Queues: map[string]int{
			QueuePriorityCritical: 6,
			QueuePriorityDefault:  3,
			QueuePriorityLow:      1,
		},
		StrictPriority: false,
		Retry: RetryConfig{
			MaxRetries:         5,
			RetryDelay:         30 * time.Second,
			MaxRetryDelay:      10 * time.Minute,
			ExponentialBackoff: true,
			RetryOnFailure:     true,
		},
		HealthCheckInterval:     30 * time.Second,
		ShutdownTimeout:         30 * time.Second,
		LogLevel:                "info",
		DisableRedisClusterMode: true,
		GroupGracePeriod:        10 * time.Second,
		GroupMaxDelay:           10 * time.Minute,
		GroupMaxSize:            100,
	}
}

// Validate checks if the queue configuration is valid
func (c *QueueConfig) Validate() error {
	if c.Backend != "memory" && c.Backend != "asynq" {
		return fmt.Errorf("invalid queue backend: %s (must be 'memory' or 'asynq')", c.Backend)
	}

	if c.Concurrency <= 0 {
		return fmt.Errorf("queue concurrency must be positive")
	}

	if len(c.Queues) == 0 {
		return fmt.Errorf("at least one queue must be configured")
	}

	totalWeight := 0
	for name, weight := range c.Queues {
		if weight <= 0 {
			return fmt.Errorf("queue %s weight must be positive", name)
		}
		totalWeight += weight
	}

	if err := c.Retry.Validate(); err != nil {
		return fmt.Errorf("invalid retry config: %w", err)
	}

	if c.HealthCheckInterval <= 0 {
		return fmt.Errorf("health check interval must be positive")
	}

	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("shutdown timeout must be positive")
	}

	if c.GroupMaxSize <= 0 {
		return fmt.Errorf("group max size must be positive")
	}

	return nil
}

// Validate checks if the retry configuration is valid
func (c *RetryConfig) Validate() error {
	if c.MaxRetries < 0 {
		return fmt.Errorf("max retries cannot be negative")
	}

	if c.RetryDelay <= 0 {
		return fmt.Errorf("retry delay must be positive")
	}

	if c.MaxRetryDelay <= 0 {
		return fmt.Errorf("max retry delay must be positive")
	}

	if c.MaxRetryDelay < c.RetryDelay {
		return fmt.Errorf("max retry delay must be greater than or equal to retry delay")
	}

	return nil
}

// IsAsynqBackend returns true if using asynq backend
func (c *QueueConfig) IsAsynqBackend() bool {
	return c.Backend == "asynq"
}

// GetQueueNames returns a slice of configured queue names
func (c *QueueConfig) GetQueueNames() []string {
	names := make([]string, 0, len(c.Queues))
	for name := range c.Queues {
		names = append(names, name)
	}
	return names
}

// GetQueuePriority returns the priority weight for a queue
func (c *QueueConfig) GetQueuePriority(queueName string) int {
	if weight, ok := c.Queues[queueName]; ok {
		return weight
	}
	return 1 // default weight
}

// Clone returns a deep copy of the configuration
func (c *QueueConfig) Clone() *QueueConfig {
	if c == nil {
		return nil
	}

	queues := make(map[string]int, len(c.Queues))
	for k, v := range c.Queues {
		queues[k] = v
	}

	return &QueueConfig{
		Backend:                 c.Backend,
		Concurrency:             c.Concurrency,
		Queues:                  queues,
		StrictPriority:          c.StrictPriority,
		Retry:                   c.Retry,
		HealthCheckInterval:     c.HealthCheckInterval,
		ShutdownTimeout:         c.ShutdownTimeout,
		LogLevel:                c.LogLevel,
		DisableRedisClusterMode: c.DisableRedisClusterMode,
		GroupGracePeriod:        c.GroupGracePeriod,
		GroupMaxDelay:           c.GroupMaxDelay,
		GroupMaxSize:            c.GroupMaxSize,
	}
}
