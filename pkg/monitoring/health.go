package monitoring

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

// HealthChecker provides health checks for various components
type HealthChecker struct {
	redisClient *redis.Client
	logger      *logrus.Logger
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(redisAddr string, logger *logrus.Logger) *HealthChecker {
	client := redis.NewClient(&redis.Options{
		Addr:         redisAddr,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	return &HealthChecker{
		redisClient: client,
		logger:      logger,
	}
}

// CheckRedis checks Redis connectivity
func (h *HealthChecker) CheckRedis(ctx context.Context) error {
	return h.redisClient.Ping(ctx).Err()
}

// CheckAll performs all health checks
func (h *HealthChecker) CheckAll(ctx context.Context) map[string]string {
	results := make(map[string]string)

	// Check Redis
	if err := h.CheckRedis(ctx); err != nil {
		results["redis"] = "unhealthy: " + err.Error()
		h.logger.WithError(err).Error("Redis health check failed")
	} else {
		results["redis"] = "healthy"
	}

	return results
}

// Close closes the health checker
func (h *HealthChecker) Close() error {
	return h.redisClient.Close()
}

// RedisHealthCheck is a simple function for checking Redis health
func RedisHealthCheck(ctx context.Context, redisAddr string) error {
	client := redis.NewClient(&redis.Options{
		Addr:         redisAddr,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	defer client.Close()

	return client.Ping(ctx).Err()
}
