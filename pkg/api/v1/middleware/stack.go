package middleware

import (
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

// Stack provides a convenient way to manage and apply middleware
type Stack struct {
	logger           logrus.FieldLogger
	authConfig       *AuthConfig
	corsConfig       *CORSConfig
	metricsConfig    *MetricsConfig
	rateLimitConfig  *RateLimitConfig
	tracingConfig    *TracingConfig
	validationConfig *ValidationConfig
}

// NewStack creates a new middleware stack with default configurations
func NewStack(logger logrus.FieldLogger) *Stack {
	return &Stack{
		logger: logger.WithField("component", "middleware"),
		// Initialize with default configs
		authConfig: &AuthConfig{
			JWTSecret: "default-secret-key", // Should be overridden in production
			SkipPaths: []string{"/health", "/ready", "/metrics"},
			Logger:    logger.WithField("middleware", "auth"),
		},
		corsConfig: &CORSConfig{
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
			ExposedHeaders:   []string{"X-Request-ID", "X-Total-Count"},
			AllowCredentials: true,
			MaxAge:           300,
			Logger:           logger.WithField("middleware", "cors"),
		},
		metricsConfig: &MetricsConfig{
			Namespace: "pandafuzz",
			Subsystem: "api",
			SkipPaths: []string{"/health", "/ready", "/metrics"},
			Logger:    logger.WithField("middleware", "metrics"),
		},
		rateLimitConfig: &RateLimitConfig{
			Rate:      100, // 100 requests per second
			Burst:     10,
			TTL:       time.Minute,
			SkipPaths: []string{"/health", "/ready"},
			Logger:    logger.WithField("middleware", "ratelimit"),
		},
		tracingConfig: &TracingConfig{
			ServiceName:    "pandafuzz-api",
			ServiceVersion: "1.0.0",
			Environment:    "development",
			SkipPaths:      []string{"/health", "/ready", "/metrics"},
			Logger:         logger.WithField("middleware", "tracing"),
		},
		validationConfig: &ValidationConfig{
			MaxRequestSize:       10 * 1024 * 1024, // 10MB
			RequiredContentTypes: []string{"application/json"},
			SkipPaths:            []string{"/health", "/ready"},
			Logger:               logger.WithField("middleware", "validation"),
		},
	}
}

// WithAuthConfig sets the authentication configuration
func (s *Stack) WithAuthConfig(config *AuthConfig) *Stack {
	s.authConfig = config
	return s
}

// WithCORSConfig sets the CORS configuration
func (s *Stack) WithCORSConfig(config *CORSConfig) *Stack {
	s.corsConfig = config
	return s
}

// WithMetricsConfig sets the metrics configuration
func (s *Stack) WithMetricsConfig(config *MetricsConfig) *Stack {
	s.metricsConfig = config
	return s
}

// WithRateLimitConfig sets the rate limit configuration
func (s *Stack) WithRateLimitConfig(config *RateLimitConfig) *Stack {
	s.rateLimitConfig = config
	return s
}

// WithTracingConfig sets the tracing configuration
func (s *Stack) WithTracingConfig(config *TracingConfig) *Stack {
	s.tracingConfig = config
	return s
}

// WithValidationConfig sets the validation configuration
func (s *Stack) WithValidationConfig(config *ValidationConfig) *Stack {
	s.validationConfig = config
	return s
}

// Recovery returns the recovery middleware
func (s *Stack) Recovery() func(http.Handler) http.Handler {
	return Recovery()
}

// RequestLogger returns the request logging middleware
func (s *Stack) RequestLogger() func(http.Handler) http.Handler {
	return RequestLogger()
}

// CORS returns the CORS middleware with configuration
func (s *Stack) CORS() func(http.Handler) http.Handler {
	if s.corsConfig != nil {
		return CORSWithConfig(*s.corsConfig)
	}
	return CORS()
}

// Tracing returns the tracing middleware with configuration
func (s *Stack) Tracing() func(http.Handler) http.Handler {
	if s.tracingConfig != nil {
		return TracingWithConfig(*s.tracingConfig)
	}
	// Return a no-op middleware if tracing is disabled
	return func(next http.Handler) http.Handler {
		return next
	}
}

// RequestMetrics returns the metrics middleware with configuration
func (s *Stack) RequestMetrics() func(http.Handler) http.Handler {
	if s.metricsConfig != nil {
		return RequestMetricsWithConfig(*s.metricsConfig)
	}
	return RequestMetrics()
}

// RateLimit returns the rate limiting middleware with configuration
func (s *Stack) RateLimit() func(http.Handler) http.Handler {
	if s.rateLimitConfig != nil {
		return RateLimitWithConfig(*s.rateLimitConfig)
	}
	return RateLimit()
}

// ValidateRequest returns the validation middleware with configuration
func (s *Stack) ValidateRequest() func(http.Handler) http.Handler {
	if s.validationConfig != nil {
		return ValidateRequestWithConfig(*s.validationConfig)
	}
	return ValidateRequest()
}

// JWTAuth returns the JWT authentication middleware with configuration
func (s *Stack) JWTAuth() func(http.Handler) http.Handler {
	if s.authConfig != nil && s.authConfig.JWTSecret != "" {
		return JWTAuth(s.authConfig.JWTSecret)
	}
	return JWTAuth("default-secret-key")
}

// APIKeyAuth returns the API key authentication middleware
func (s *Stack) APIKeyAuth(validator APIKeyValidator) func(http.Handler) http.Handler {
	if validator != nil {
		return APIKeyAuth(validator)
	}
	// Return a no-op middleware if no validator is provided
	return func(next http.Handler) http.Handler {
		return next
	}
}

// RequirePermission returns a middleware that checks for specific permissions
func (s *Stack) RequirePermission(permission string) func(http.Handler) http.Handler {
	return RequirePermission(permission)
}
