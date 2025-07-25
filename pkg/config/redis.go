package config

import (
	"fmt"
	"time"
)

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	// Host is the Redis server hostname
	Host string `json:"host" yaml:"host" mapstructure:"host"`

	// Port is the Redis server port
	Port int `json:"port" yaml:"port" mapstructure:"port"`

	// Password is the Redis authentication password
	Password string `json:"password" yaml:"password" mapstructure:"password"`

	// DB is the Redis database number
	DB int `json:"db" yaml:"db" mapstructure:"db"`

	// PoolSize is the maximum number of connections
	PoolSize int `json:"pool_size" yaml:"pool_size" mapstructure:"pool_size"`

	// MinIdleConns is the minimum number of idle connections
	MinIdleConns int `json:"min_idle_conns" yaml:"min_idle_conns" mapstructure:"min_idle_conns"`

	// MaxRetries is the maximum number of retries for failed operations
	MaxRetries int `json:"max_retries" yaml:"max_retries" mapstructure:"max_retries"`

	// DialTimeout is the timeout for establishing connections
	DialTimeout time.Duration `json:"dial_timeout" yaml:"dial_timeout" mapstructure:"dial_timeout"`

	// ReadTimeout is the timeout for read operations
	ReadTimeout time.Duration `json:"read_timeout" yaml:"read_timeout" mapstructure:"read_timeout"`

	// WriteTimeout is the timeout for write operations
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout" mapstructure:"write_timeout"`

	// IdleTimeout is the timeout for idle connections
	IdleTimeout time.Duration `json:"idle_timeout" yaml:"idle_timeout" mapstructure:"idle_timeout"`

	// MaxConnAge is the maximum age of connections
	MaxConnAge time.Duration `json:"max_conn_age" yaml:"max_conn_age" mapstructure:"max_conn_age"`

	// TLS enables TLS/SSL connection
	TLS bool `json:"tls" yaml:"tls" mapstructure:"tls"`

	// TLSSkipVerify skips TLS certificate verification
	TLSSkipVerify bool `json:"tls_skip_verify" yaml:"tls_skip_verify" mapstructure:"tls_skip_verify"`
}

// DefaultRedisConfig returns default Redis configuration
func DefaultRedisConfig() *RedisConfig {
	return &RedisConfig{
		Host:          "localhost",
		Port:          6379,
		Password:      "",
		DB:            0,
		PoolSize:      10,
		MinIdleConns:  5,
		MaxRetries:    3,
		DialTimeout:   5 * time.Second,
		ReadTimeout:   3 * time.Second,
		WriteTimeout:  3 * time.Second,
		IdleTimeout:   5 * time.Minute,
		MaxConnAge:    30 * time.Minute,
		TLS:           false,
		TLSSkipVerify: false,
	}
}

// Validate checks if the Redis configuration is valid
func (c *RedisConfig) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("redis host cannot be empty")
	}

	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("redis port must be between 1 and 65535")
	}

	if c.DB < 0 {
		return fmt.Errorf("redis database number cannot be negative")
	}

	if c.PoolSize <= 0 {
		return fmt.Errorf("redis pool size must be positive")
	}

	if c.MinIdleConns < 0 {
		return fmt.Errorf("redis min idle connections cannot be negative")
	}

	if c.MinIdleConns > c.PoolSize {
		return fmt.Errorf("redis min idle connections cannot exceed pool size")
	}

	if c.MaxRetries < 0 {
		return fmt.Errorf("redis max retries cannot be negative")
	}

	return nil
}

// Addr returns the Redis server address in host:port format
func (c *RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// IsSecure returns true if TLS is enabled
func (c *RedisConfig) IsSecure() bool {
	return c.TLS
}

// Clone returns a deep copy of the configuration
func (c *RedisConfig) Clone() *RedisConfig {
	if c == nil {
		return nil
	}

	return &RedisConfig{
		Host:          c.Host,
		Port:          c.Port,
		Password:      c.Password,
		DB:            c.DB,
		PoolSize:      c.PoolSize,
		MinIdleConns:  c.MinIdleConns,
		MaxRetries:    c.MaxRetries,
		DialTimeout:   c.DialTimeout,
		ReadTimeout:   c.ReadTimeout,
		WriteTimeout:  c.WriteTimeout,
		IdleTimeout:   c.IdleTimeout,
		MaxConnAge:    c.MaxConnAge,
		TLS:           c.TLS,
		TLSSkipVerify: c.TLSSkipVerify,
	}
}
