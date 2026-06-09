package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/errors"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

// RateLimiter provides rate limiting functionality
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
	ttl      time.Duration
	logger   logrus.FieldLogger
}

// EndpointLimit defines rate limits for specific endpoints
type EndpointLimit struct {
	Rate   int           // Requests per window
	Window time.Duration // Time window for rate limiting
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Rate           int                        // Requests per second (global default)
	Burst          int                        // Burst capacity
	TTL            time.Duration              // Time to live for rate limiter entries
	KeyExtractor   func(*http.Request) string // Function to extract rate limit key
	SkipPaths      []string                   // Paths to skip rate limiting
	EndpointLimits map[string]EndpointLimit   // Endpoint-specific rate limits (key: "METHOD /path")
	RedisClient    *redis.Client              // Optional Redis client for distributed rate limiting
	Logger         logrus.FieldLogger
}

// RedisRateLimiter provides Redis-based distributed rate limiting
type RedisRateLimiter struct {
	client *redis.Client
	rate   int
	window time.Duration
	logger logrus.FieldLogger
}

// NewRateLimiter creates a new in-memory rate limiter
func NewRateLimiter(rateLimit int, burst int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(rateLimit),
		burst:    burst,
		ttl:      5 * time.Minute, // Default TTL for cleanup
		logger:   logrus.NewEntry(logrus.StandardLogger()),
	}
}

// NewRateLimiterWithConfig creates a rate limiter with configuration
func NewRateLimiterWithConfig(config RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(config.Rate),
		burst:    config.Burst,
		ttl:      config.TTL,
		logger:   config.Logger,
	}

	if rl.ttl == 0 {
		rl.ttl = 5 * time.Minute
	}

	if rl.logger == nil {
		rl.logger = logrus.NewEntry(logrus.StandardLogger())
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// NewRedisRateLimiter creates a Redis-based rate limiter
func NewRedisRateLimiter(client *redis.Client, rateLimit int, window time.Duration) *RedisRateLimiter {
	return &RedisRateLimiter{
		client: client,
		rate:   rateLimit,
		window: window,
		logger: logrus.NewEntry(logrus.StandardLogger()),
	}
}

// Allow checks if a request should be allowed for the given key
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.RLock()
	limiter, exists := rl.limiters[key]
	rl.mu.RUnlock()

	if !exists {
		rl.mu.Lock()
		// Double-check after acquiring write lock
		if limiter, exists = rl.limiters[key]; !exists {
			limiter = rate.NewLimiter(rl.rate, rl.burst)
			rl.limiters[key] = limiter
		}
		rl.mu.Unlock()
	}

	return limiter.Allow()
}

// AllowN checks if N requests should be allowed for the given key
func (rl *RateLimiter) AllowN(key string, n int) bool {
	rl.mu.RLock()
	limiter, exists := rl.limiters[key]
	rl.mu.RUnlock()

	if !exists {
		rl.mu.Lock()
		// Double-check after acquiring write lock
		if limiter, exists = rl.limiters[key]; !exists {
			limiter = rate.NewLimiter(rl.rate, rl.burst)
			rl.limiters[key] = limiter
		}
		rl.mu.Unlock()
	}

	return limiter.AllowN(time.Now(), n)
}

// Reservation reserves N requests for the given key
func (rl *RateLimiter) Reservation(key string, n int) *rate.Reservation {
	rl.mu.RLock()
	limiter, exists := rl.limiters[key]
	rl.mu.RUnlock()

	if !exists {
		rl.mu.Lock()
		// Double-check after acquiring write lock
		if limiter, exists = rl.limiters[key]; !exists {
			limiter = rate.NewLimiter(rl.rate, rl.burst)
			rl.limiters[key] = limiter
		}
		rl.mu.Unlock()
	}

	return limiter.ReserveN(time.Now(), n)
}

// cleanup removes old rate limiters periodically
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.ttl)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		for key, limiter := range rl.limiters {
			// Remove limiters that haven't been used recently
			// This is a simple heuristic - in production you might want more sophisticated cleanup
			if limiter.TokensAt(time.Now()) == float64(rl.burst) {
				delete(rl.limiters, key)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow checks if a request should be allowed using Redis sliding window
func (rrl *RedisRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now()
	windowStart := now.Add(-rrl.window)

	pipe := rrl.client.Pipeline()

	// Remove expired entries
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart.UnixNano(), 10))

	// Count current requests in window
	pipe.ZCard(ctx, key)

	// Add current request
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})

	// Set expiration
	pipe.Expire(ctx, key, rrl.window+time.Minute)

	results, err := pipe.Exec(ctx)
	if err != nil {
		rrl.logger.WithFields(logrus.Fields{
			"key":   key,
			"error": err.Error(),
		}).Error("Redis pipeline failed")
		return false, fmt.Errorf("redis pipeline failed: %w", err)
	}

	// Get count from ZCard result
	countCmd := results[1].(*redis.IntCmd)
	count, err := countCmd.Result()
	if err != nil {
		return false, fmt.Errorf("failed to get count: %w", err)
	}

	allowed := count < int64(rrl.rate)
	if !allowed {
		// Remove the request we just added since it's not allowed
		rrl.client.ZRem(ctx, key, fmt.Sprintf("%d", now.UnixNano()))
	}

	return allowed, nil
}

// RateLimit creates a rate limiting middleware
func RateLimit() func(http.Handler) http.Handler {
	return RateLimitWithConfig(RateLimitConfig{
		Rate:  100, // Default: 100 requests per second
		Burst: 200, // Default: 200 burst capacity
	})
}

// EndpointRateLimiter provides rate limiting for specific endpoints with different limits
type EndpointRateLimiter struct {
	limiters map[string]*RateLimiter // Key: "METHOD /path"
	mu       sync.RWMutex
	logger   logrus.FieldLogger
}

// NewEndpointRateLimiter creates a new endpoint-specific rate limiter
func NewEndpointRateLimiter(limits map[string]EndpointLimit, logger logrus.FieldLogger) *EndpointRateLimiter {
	erl := &EndpointRateLimiter{
		limiters: make(map[string]*RateLimiter),
		logger:   logger,
	}

	// Create a rate limiter for each endpoint
	for endpoint, limit := range limits {
		// Convert per-window rate to per-second rate
		ratePerSecond := float64(limit.Rate) / limit.Window.Seconds()
		if ratePerSecond < 0.1 {
			ratePerSecond = 0.1 // Minimum rate to avoid issues
		}
		erl.limiters[endpoint] = NewRateLimiter(int(ratePerSecond)+1, limit.Rate)
	}

	return erl
}

// Allow checks if a request to a specific endpoint should be allowed
func (erl *EndpointRateLimiter) Allow(method, path, clientKey string) (bool, *EndpointLimit) {
	endpointKey := method + " " + path

	erl.mu.RLock()
	limiter, exists := erl.limiters[endpointKey]
	erl.mu.RUnlock()

	if !exists {
		return true, nil // No endpoint-specific limit, allow
	}

	fullKey := endpointKey + ":" + clientKey
	return limiter.Allow(fullKey), nil
}

// RateLimitWithConfig creates a rate limiting middleware with configuration
func RateLimitWithConfig(config RateLimitConfig) func(http.Handler) http.Handler {
	if config.Rate <= 0 {
		config.Rate = 100
	}
	if config.Burst <= 0 {
		config.Burst = config.Rate * 2
	}
	if config.KeyExtractor == nil {
		config.KeyExtractor = defaultKeyExtractor
	}
	if config.Logger == nil {
		config.Logger = logrus.NewEntry(logrus.StandardLogger())
	}

	var rateLimiter interface{}

	if config.RedisClient != nil {
		// Use Redis-based rate limiter for distributed environments
		window := time.Second // 1 second window for rate limiting
		rateLimiter = NewRedisRateLimiter(config.RedisClient, config.Rate, window)
	} else {
		// Use in-memory rate limiter
		rateLimiter = NewRateLimiterWithConfig(config)
	}

	// Create endpoint-specific rate limiters if configured
	var endpointLimiter *EndpointRateLimiter
	if len(config.EndpointLimits) > 0 {
		endpointLimiter = NewEndpointRateLimiter(config.EndpointLimits, config.Logger)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip rate limiting for configured paths
			for _, path := range config.SkipPaths {
				if r.URL.Path == path {
					next.ServeHTTP(w, r)
					return
				}
			}

			key := config.KeyExtractor(r)

			// Check endpoint-specific rate limit first
			if endpointLimiter != nil {
				if allowed, _ := endpointLimiter.Allow(r.Method, r.URL.Path, key); !allowed {
					config.Logger.WithFields(logrus.Fields{
						"key":      key,
						"path":     r.URL.Path,
						"method":   r.Method,
						"endpoint": r.Method + " " + r.URL.Path,
					}).Warn("Endpoint rate limit exceeded")

					// For endpoint limits, use a longer retry time
					retryAfter := 60 // Default 60 seconds for endpoint limits
					w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
					w.Header().Set("X-RateLimit-Remaining", "0")

					errors.WriteErrorWithDetails(w, http.StatusTooManyRequests, "Endpoint rate limit exceeded", map[string]interface{}{
						"retry_after_seconds": retryAfter,
						"endpoint":            r.Method + " " + r.URL.Path,
					})
					return
				}
			}

			// Then check global rate limit
			var allowed bool
			var err error

			switch rl := rateLimiter.(type) {
			case *RateLimiter:
				allowed = rl.Allow(key)
			case *RedisRateLimiter:
				allowed, err = rl.Allow(r.Context(), key)
				if err != nil {
					config.Logger.WithFields(logrus.Fields{
						"key":   key,
						"error": err.Error(),
					}).Error("Rate limiting failed")
					// Fail open - allow request if rate limiting fails
					allowed = true
				}
			}

			if !allowed {
				config.Logger.WithFields(logrus.Fields{
					"key":    key,
					"path":   r.URL.Path,
					"method": r.Method,
				}).Warn("Rate limit exceeded")

				// Calculate retry after based on rate
				retryAfter := time.Second / time.Duration(config.Rate)
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(config.Rate))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(retryAfter).Unix(), 10))

				errors.WriteErrorWithDetails(w, http.StatusTooManyRequests, "Rate limit exceeded", map[string]interface{}{
					"retry_after_seconds": int(retryAfter.Seconds()),
					"limit_per_second":    config.Rate,
				})
				return
			}

			// Add rate limit headers for successful requests
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(config.Rate))

			// Calculate remaining requests (approximate for in-memory limiter)
			if memLimiter, ok := rateLimiter.(*RateLimiter); ok {
				remaining := memLimiter.burst // Approximate remaining
				w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			}

			config.Logger.WithFields(logrus.Fields{
				"key":    key,
				"path":   r.URL.Path,
				"method": r.Method,
			}).Debug("Request allowed by rate limiter")

			next.ServeHTTP(w, r)
		})
	}
}

// PerAPIKeyRateLimit creates a rate limiting middleware that uses different limits per API key
func PerAPIKeyRateLimit() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get API key info from context (if available)
			keyInfo, err := ExtractAPIKeyInfo(r)
			if err != nil {
				// Fall back to IP-based rate limiting
				config := RateLimitConfig{
					Rate:         100,
					Burst:        200,
					KeyExtractor: defaultKeyExtractor,
				}
				RateLimitWithConfig(config)(next).ServeHTTP(w, r)
				return
			}

			// Use API key specific rate limit
			rateLimit := keyInfo.RateLimit
			if rateLimit <= 0 {
				rateLimit = 1000 // Default for API keys
			}

			config := RateLimitConfig{
				Rate:  rateLimit,
				Burst: rateLimit * 2,
				KeyExtractor: func(r *http.Request) string {
					return fmt.Sprintf("api_key:%s", keyInfo.KeyID)
				},
			}

			RateLimitWithConfig(config)(next).ServeHTTP(w, r)
		})
	}
}

// defaultKeyExtractor extracts the client IP address for rate limiting
func defaultKeyExtractor(r *http.Request) string {
	// Try to get real IP from headers (for proxy/load balancer setups)
	if xRealIP := r.Header.Get("X-Real-IP"); xRealIP != "" {
		return xRealIP
	}

	if xForwardedFor := r.Header.Get("X-Forwarded-For"); xForwardedFor != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		if ips := parseXForwardedFor(xForwardedFor); len(ips) > 0 {
			return ips[0]
		}
	}

	// Fall back to remote address
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return ip
}

// parseXForwardedFor parses the X-Forwarded-For header value
func parseXForwardedFor(header string) []string {
	var ips []string
	for _, ip := range splitHeader(header) {
		if parsedIP := net.ParseIP(ip); parsedIP != nil {
			ips = append(ips, ip)
		}
	}
	return ips
}

// splitHeader splits a comma-separated header value and trims spaces
func splitHeader(header string) []string {
	var parts []string
	for _, part := range strings.Split(header, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}
