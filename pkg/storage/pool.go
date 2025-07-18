package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// PoolConfig holds configuration for database connection pooling
type PoolConfig struct {
	MaxOpenConns        int
	MaxIdleConns        int
	ConnMaxLifetime     time.Duration
	ConnMaxIdleTime     time.Duration
	HealthCheckInterval time.Duration
	RetryOptions        RetryOptions
}

// DefaultPoolConfig returns sensible defaults for SQLite connection pooling
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxOpenConns:        3, // SQLite with WAL mode can handle multiple readers + one writer
		MaxIdleConns:        2,
		ConnMaxLifetime:     0, // Don't expire connections for SQLite
		ConnMaxIdleTime:     10 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		RetryOptions:        DefaultRetryOptions(),
	}
}

// ConnectionPool manages database connections with health checking
type ConnectionPool struct {
	db                  *sql.DB
	config              PoolConfig
	logger              logrus.FieldLogger
	healthCheckStop     chan struct{}
	healthCheckDone     sync.WaitGroup
	metrics             *PoolMetrics
	mu                  sync.RWMutex
	lastHealthCheckTime time.Time
	isHealthy           bool
}

// PoolMetrics tracks connection pool statistics
type PoolMetrics struct {
	TotalConnections   int64
	ActiveConnections  int64
	IdleConnections    int64
	WaitCount          int64
	WaitDuration       time.Duration
	MaxIdleClosed      int64
	MaxLifetimeClosed  int64
	HealthCheckSuccess int64
	HealthCheckFailure int64
}

// NewConnectionPool creates a new connection pool with health checking
func NewConnectionPool(db *sql.DB, config PoolConfig, logger logrus.FieldLogger) *ConnectionPool {
	if logger == nil {
		logger = logrus.New()
	}

	pool := &ConnectionPool{
		db:              db,
		config:          config,
		logger:          logger.WithField("component", "connection_pool"),
		healthCheckStop: make(chan struct{}),
		metrics:         &PoolMetrics{},
		isHealthy:       true,
	}

	// Configure the underlying connection pool
	pool.configure()

	return pool
}

// Start begins health checking
func (p *ConnectionPool) Start(ctx context.Context) error {
	p.logger.Info("Starting connection pool health checks")

	// Perform initial health check
	if err := p.performHealthCheck(ctx); err != nil {
		return fmt.Errorf("initial health check failed: %w", err)
	}

	// Start periodic health checks
	p.healthCheckDone.Add(1)
	go p.healthCheckLoop()

	return nil
}

// Stop gracefully shuts down the connection pool
func (p *ConnectionPool) Stop() error {
	p.logger.Info("Stopping connection pool")

	// Stop health checks
	close(p.healthCheckStop)
	p.healthCheckDone.Wait()

	// Close the database connection
	if err := p.db.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}

	return nil
}

// DB returns the underlying database connection
func (p *ConnectionPool) DB() *sql.DB {
	return p.db
}

// IsHealthy returns the current health status
func (p *ConnectionPool) IsHealthy() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.isHealthy
}

// GetMetrics returns current pool metrics
func (p *ConnectionPool) GetMetrics() PoolMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Get real-time stats from the database
	stats := p.db.Stats()

	return PoolMetrics{
		TotalConnections:   int64(stats.OpenConnections),
		ActiveConnections:  int64(stats.InUse),
		IdleConnections:    int64(stats.Idle),
		WaitCount:          stats.WaitCount,
		WaitDuration:       stats.WaitDuration,
		MaxIdleClosed:      stats.MaxIdleClosed,
		MaxLifetimeClosed:  stats.MaxLifetimeClosed,
		HealthCheckSuccess: p.metrics.HealthCheckSuccess,
		HealthCheckFailure: p.metrics.HealthCheckFailure,
	}
}

// configure sets up the database connection pool parameters
func (p *ConnectionPool) configure() {
	p.db.SetMaxOpenConns(p.config.MaxOpenConns)
	p.db.SetMaxIdleConns(p.config.MaxIdleConns)
	p.db.SetConnMaxLifetime(p.config.ConnMaxLifetime)
	p.db.SetConnMaxIdleTime(p.config.ConnMaxIdleTime)

	p.logger.WithFields(logrus.Fields{
		"max_open_conns":    p.config.MaxOpenConns,
		"max_idle_conns":    p.config.MaxIdleConns,
		"conn_max_lifetime": p.config.ConnMaxLifetime,
		"conn_max_idle":     p.config.ConnMaxIdleTime,
	}).Info("Configured connection pool")
}

// healthCheckLoop runs periodic health checks
func (p *ConnectionPool) healthCheckLoop() {
	defer p.healthCheckDone.Done()

	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := p.performHealthCheck(ctx); err != nil {
				p.logger.WithError(err).Error("Health check failed")
			}
			cancel()

		case <-p.healthCheckStop:
			return
		}
	}
}

// performHealthCheck executes a health check on the database
func (p *ConnectionPool) performHealthCheck(ctx context.Context) error {
	p.mu.Lock()
	p.lastHealthCheckTime = time.Now()
	p.mu.Unlock()

	// Use retry logic for health checks
	err := WithRetryVoid(ctx, p.config.RetryOptions, func(ctx context.Context) error {
		return p.db.PingContext(ctx)
	})

	p.mu.Lock()
	defer p.mu.Unlock()

	if err != nil {
		p.isHealthy = false
		p.metrics.HealthCheckFailure++
		return err
	}

	p.isHealthy = true
	p.metrics.HealthCheckSuccess++

	// Log metrics periodically
	if p.metrics.HealthCheckSuccess%10 == 0 {
		stats := p.db.Stats()
		p.logger.WithFields(logrus.Fields{
			"open_connections": stats.OpenConnections,
			"in_use":           stats.InUse,
			"idle":             stats.Idle,
			"wait_count":       stats.WaitCount,
			"wait_duration":    stats.WaitDuration,
		}).Debug("Connection pool stats")
	}

	return nil
}

// ExecuteWithRetry executes a database operation with retry logic
func (p *ConnectionPool) ExecuteWithRetry(ctx context.Context, operation string, fn func(context.Context) error) error {
	// Check health before executing
	if !p.IsHealthy() {
		return common.NewDatabaseError(operation, fmt.Errorf("connection pool is unhealthy"))
	}

	retryOp := &RetryableOperation{
		Name:    operation,
		Logger:  p.logger,
		Options: p.config.RetryOptions,
	}

	return retryOp.Execute(ctx, fn)
}

// ExecuteWithRetryResult is now a standalone function that accepts generic types
// The pool instance is passed as a parameter to maintain the same functionality
func ExecutePoolWithRetryResult[T any](pool *ConnectionPool, ctx context.Context, operation string, fn func(context.Context) (T, error)) (T, error) {
	var zero T

	// Check health before executing
	if !pool.IsHealthy() {
		return zero, common.NewDatabaseError(operation, fmt.Errorf("connection pool is unhealthy"))
	}

	retryOp := &RetryableOperation{
		Name:    operation,
		Logger:  pool.logger,
		Options: pool.config.RetryOptions,
	}

	return ExecuteRetryableWithResult(retryOp, ctx, fn)
}

// ConfigurePool sets up a database connection with optimized pooling and retry logic
func ConfigurePool(db *sql.DB, config common.DatabaseConfig, logger logrus.FieldLogger) (*ConnectionPool, error) {
	poolConfig := DefaultPoolConfig()

	// Override with any custom configuration
	if config.MaxOpenConns > 0 {
		poolConfig.MaxOpenConns = config.MaxOpenConns
	}
	if config.MaxIdleConns > 0 {
		poolConfig.MaxIdleConns = config.MaxIdleConns
	}
	if config.ConnMaxLifetime > 0 {
		poolConfig.ConnMaxLifetime = config.ConnMaxLifetime
	}

	// Create the connection pool
	pool := NewConnectionPool(db, poolConfig, logger)

	// Start health checking
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := pool.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start connection pool: %w", err)
	}

	return pool, nil
}

// HealthCheck performs an immediate health check on the connection pool
func HealthCheck(pool *ConnectionPool) error {
	if pool == nil {
		return fmt.Errorf("connection pool is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return pool.performHealthCheck(ctx)
}
