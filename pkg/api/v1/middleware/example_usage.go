package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
)

// ExampleMiddlewareStack demonstrates how to use all middleware together
func ExampleMiddlewareStack() http.Handler {
	r := chi.NewRouter()

	// 1. Recovery middleware (should be first to catch panics in other middleware)
	r.Use(Recovery())

	// 2. Logging middleware (early to capture all requests)
	r.Use(RequestLogger())

	// 3. CORS middleware (handle preflight requests early)
	r.Use(CORS())

	// 4. Tracing middleware (for distributed tracing)
	r.Use(Tracing())

	// 5. Metrics middleware (measure everything)
	r.Use(RequestMetrics())

	// 6. Rate limiting middleware
	r.Use(RateLimit())

	// 7. Validation middleware (validate requests before processing)
	r.Use(ValidateRequest())

	// 8. Authentication middleware (protect endpoints)
	r.Use(JWTAuth("your-secret-key"))

	// Example routes
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	return r
}

// ProductionMiddlewareStack creates a production-ready middleware stack
func ProductionMiddlewareStack(jwtSecret string, logger logrus.FieldLogger) http.Handler {
	r := chi.NewRouter()

	// Recovery with production settings
	r.Use(RecoveryWithConfig(ProductionRecovery()))

	// Structured logging
	r.Use(RequestLoggerWithConfig(LoggingConfig{
		Logger:          logger,
		LogLevel:        logrus.InfoLevel,
		IncludeTimings:  true,
		MaxBodySize:     1024,
		LogRequestBody:  false,
		LogResponseBody: false,
		SkipPaths: []string{
			"/health",
			"/metrics",
		},
	}))

	// Secure CORS
	r.Use(CORSWithConfig(SecureCORS([]string{
		"https://app.pandafuzz.com",
		"https://dashboard.pandafuzz.com",
	})))

	// Distributed tracing
	r.Use(TracingWithConfig(TracingConfig{
		ServiceName:           "pandafuzz-api",
		ServiceVersion:        "v1.0.0",
		Environment:           "production",
		SamplingRate:          0.1, // Sample 10% of traces
		EnableW3CTraceContext: true,
		EnableB3Propagation:   false,
	}))

	// Comprehensive metrics
	r.Use(RequestMetricsWithConfig(MetricsConfig{
		Namespace: "pandafuzz",
		Subsystem: "api",
		SkipPaths: []string{"/health", "/metrics"},
	}))

	// Rate limiting with Redis backend
	r.Use(RateLimitWithConfig(RateLimitConfig{
		Rate:      1000, // 1000 requests per second
		Burst:     2000, // Allow bursts up to 2000
		SkipPaths: []string{"/health"},
	}))

	// Strict validation
	r.Use(ValidateRequestWithConfig(ValidationConfig{
		MaxRequestSize:     10 * 1024 * 1024, // 10MB
		StrictMode:         true,
		AllowUnknownFields: false,
		SkipPaths:          []string{"/health", "/metrics"},
	}))

	// JWT authentication
	r.Use(JWTAuthWithConfig(AuthConfig{
		JWTSecret: jwtSecret,
		Logger:    logger,
		SkipPaths: []string{
			"/health",
			"/metrics",
			"/api/v1/auth/login",
			"/api/v1/auth/register",
		},
	}))

	return r
}

// DevelopmentMiddlewareStack creates a development-friendly middleware stack
func DevelopmentMiddlewareStack(logger logrus.FieldLogger) http.Handler {
	r := chi.NewRouter()

	// Recovery with detailed error information
	r.Use(RecoveryWithConfig(DevelopmentRecovery()))

	// Verbose logging
	r.Use(RequestLoggerWithConfig(LoggingConfig{
		Logger:          logger,
		LogLevel:        logrus.DebugLevel,
		IncludeTimings:  true,
		LogRequestBody:  true,
		LogResponseBody: true,
		MaxBodySize:     8192,
	}))

	// Permissive CORS for development
	r.Use(CORSWithConfig(DevelopmentCORS()))

	// Full tracing
	r.Use(TracingWithConfig(TracingConfig{
		ServiceName:           "pandafuzz-api-dev",
		ServiceVersion:        "dev",
		Environment:           "development",
		SamplingRate:          1.0, // Sample all traces
		EnableW3CTraceContext: true,
		EnableB3Propagation:   true,
	}))

	// Detailed metrics
	r.Use(RequestMetrics())

	// Lenient rate limiting
	r.Use(RateLimitWithConfig(RateLimitConfig{
		Rate:  10000, // High rate limit for development
		Burst: 20000,
	}))

	// Lenient validation
	r.Use(ValidateRequestWithConfig(ValidationConfig{
		MaxRequestSize:     50 * 1024 * 1024, // 50MB for testing
		StrictMode:         false,
		AllowUnknownFields: true,
	}))

	return r
}

// APIKeyAuthExample demonstrates API key authentication
func APIKeyAuthExample() http.Handler {
	r := chi.NewRouter()

	// Basic middleware
	r.Use(Recovery())
	r.Use(RequestLogger())

	// API key validator function
	apiKeyValidator := func(apiKey string) (*APIKeyInfo, error) {
		// In production, this would validate against a database
		validKeys := map[string]*APIKeyInfo{
			"dev-key-123": {
				KeyID:       "dev-key-123",
				Name:        "Development Key",
				Permissions: []string{"read", "write"},
				RateLimit:   1000,
				CreatedAt:   time.Now(),
			},
			"admin-key-456": {
				KeyID:       "admin-key-456",
				Name:        "Admin Key",
				Permissions: []string{"*"},
				RateLimit:   10000,
				CreatedAt:   time.Now(),
			},
		}

		if keyInfo, exists := validKeys[apiKey]; exists {
			return keyInfo, nil
		}
		return nil, fmt.Errorf("invalid API key")
	}

	// API key authentication
	r.Use(APIKeyAuthWithConfig(AuthConfig{
		APIKeyValidator: apiKeyValidator,
		SkipPaths:       []string{"/health"},
	}))

	// Per-API-key rate limiting
	r.Use(PerAPIKeyRateLimit())

	// Permission-based route protection
	r.Route("/api/v1", func(r chi.Router) {
		// Read operations
		r.Group(func(r chi.Router) {
			r.Use(RequirePermission("read"))
			r.Get("/bots", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Bot list"))
			})
		})

		// Write operations
		r.Group(func(r chi.Router) {
			r.Use(RequirePermission("write"))
			r.Post("/bots", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte("Bot created"))
			})
		})

		// Admin operations
		r.Group(func(r chi.Router) {
			r.Use(RequirePermission("admin"))
			r.Delete("/bots/{id}", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
		})
	})

	return r
}

// MetricsExample demonstrates custom metrics usage
func MetricsExample() http.Handler {
	r := chi.NewRouter()

	// Basic middleware
	r.Use(Recovery())
	r.Use(RequestLogger())

	// Metrics with custom business logic
	metricsConfig := MetricsConfig{
		Namespace: "pandafuzz",
		Subsystem: "api",
	}
	r.Use(RequestMetricsWithConfig(metricsConfig))

	r.Post("/api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		// Record business metric
		if recorder := GetMetricsRecorder(r); recorder != nil {
			recorder.RecordJobCreated("libfuzzer", "campaign-123")
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("Job created"))
	})

	r.Post("/api/v1/crashes", func(w http.ResponseWriter, r *http.Request) {
		// Record crash found
		if recorder := GetMetricsRecorder(r); recorder != nil {
			recorder.RecordCrashFound("afl++", "high")
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("Crash recorded"))
	})

	return r
}
