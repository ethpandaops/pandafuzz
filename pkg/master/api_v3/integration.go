package api_v3

import (
	"net/http"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// Integration provides easy integration of API v3 into the master server
type Integration struct {
	handler    *HandlerV3
	middleware []func(http.Handler) http.Handler
}

// NewIntegration creates a new API v3 integration
func NewIntegration(services *service.Manager, logger logrus.FieldLogger, config *IntegrationConfig) *Integration {
	// Create v3 handler config
	handlerConfig := &Config{
		MaxRequestSize:  config.MaxRequestSize,
		RequestTimeout:  config.RequestTimeout,
		MaxBatchSize:    config.MaxBatchSize,
		EnableSwaggerUI: config.EnableSwaggerUI,
	}

	// Create handler
	handler := NewHandlerV3(services, logger, handlerConfig)

	// Setup middleware stack
	middleware := []func(http.Handler) http.Handler{}

	// Add CORS if enabled
	if config.EnableCORS {
		middleware = append(middleware, func(next http.Handler) http.Handler {
			return NewCORSMiddleware(next, config.AllowedOrigins)
		})
	}

	// Add backwards compatibility if enabled
	if config.EnableBackwardsCompatibility {
		middleware = append(middleware, func(next http.Handler) http.Handler {
			return NewBackwardsCompatibilityMiddleware(next)
		})
	}

	// Add deprecation warnings if enabled
	if config.EnableDeprecationWarnings {
		middleware = append(middleware, func(next http.Handler) http.Handler {
			return NewDeprecationMiddleware(next)
		})
	}

	return &Integration{
		handler:    handler,
		middleware: middleware,
	}
}

// IntegrationConfig holds configuration for API v3 integration
type IntegrationConfig struct {
	// Handler configuration
	MaxRequestSize  int64
	RequestTimeout  time.Duration
	MaxBatchSize    int
	EnableSwaggerUI bool

	// Middleware configuration
	EnableCORS                   bool
	AllowedOrigins               []string
	EnableBackwardsCompatibility bool
	EnableDeprecationWarnings    bool

	// Rate limiting configuration
	EnableRateLimiting bool
	RateLimit          int           // Requests per window
	RateLimitWindow    time.Duration // Time window

	// Authentication configuration
	EnableAuthentication bool
	AuthenticationFunc   func(r *http.Request) (bool, error)
}

// DefaultIntegrationConfig returns a default configuration
func DefaultIntegrationConfig() *IntegrationConfig {
	return &IntegrationConfig{
		MaxRequestSize:               10 * 1024 * 1024, // 10MB
		RequestTimeout:               30 * time.Second,
		MaxBatchSize:                 1000,
		EnableSwaggerUI:              true,
		EnableCORS:                   true,
		AllowedOrigins:               []string{"*"},
		EnableBackwardsCompatibility: true,
		EnableDeprecationWarnings:    true,
		EnableRateLimiting:           true,
		RateLimit:                    1000,
		RateLimitWindow:              time.Hour,
		EnableAuthentication:         false,
	}
}

// RegisterRoutes registers all API v3 routes on the given router
func (i *Integration) RegisterRoutes(router *mux.Router) {
	// Create subrouter for v3
	v3Router := router.PathPrefix("/api/v3").Subrouter()

	// Apply middleware in reverse order so they execute in correct order
	handler := http.Handler(v3Router)
	for idx := len(i.middleware) - 1; idx >= 0; idx-- {
		handler = i.middleware[idx](handler)
	}

	// Register v3 routes
	i.handler.RegisterRoutes(v3Router)

	// If backwards compatibility is enabled, also handle v2 routes
	if hasBackwardsCompatibility(i.middleware) {
		v2Router := router.PathPrefix("/api/v2").Subrouter()
		// v2 routes will be transformed by middleware
		i.handler.RegisterRoutes(v2Router)
	}
}

// GetHandler returns the HTTP handler with all middleware applied
func (i *Integration) GetHandler() http.Handler {
	router := mux.NewRouter()
	i.RegisterRoutes(router)

	// Apply middleware
	handler := http.Handler(router)
	for idx := len(i.middleware) - 1; idx >= 0; idx-- {
		handler = i.middleware[idx](handler)
	}

	return handler
}

// AddMiddleware adds additional middleware to the stack
func (i *Integration) AddMiddleware(middleware func(http.Handler) http.Handler) {
	i.middleware = append(i.middleware, middleware)
}

// Helper function to check if backwards compatibility middleware is enabled
func hasBackwardsCompatibility(middleware []func(http.Handler) http.Handler) bool {
	// This is a simplified check - in production you might want a more robust solution
	return len(middleware) > 0
}

// RateLimitMiddleware provides rate limiting
type RateLimitMiddleware struct {
	next      http.Handler
	rateLimit int
	window    time.Duration
	// In production, use a proper rate limiter like golang.org/x/time/rate
}

// NewRateLimitMiddleware creates a new rate limit middleware
func NewRateLimitMiddleware(next http.Handler, rateLimit int, window time.Duration) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		next:      next,
		rateLimit: rateLimit,
		window:    window,
	}
}

// ServeHTTP implements http.Handler
func (m *RateLimitMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Simplified rate limiting - in production use proper rate limiter
	// This is just for demonstration

	// Check rate limit
	// ... rate limiting logic ...

	// If within limits, proceed
	m.next.ServeHTTP(w, r)
}

// AuthenticationMiddleware provides authentication
type AuthenticationMiddleware struct {
	next http.Handler
	auth func(r *http.Request) (bool, error)
}

// NewAuthenticationMiddleware creates a new authentication middleware
func NewAuthenticationMiddleware(next http.Handler, auth func(r *http.Request) (bool, error)) *AuthenticationMiddleware {
	return &AuthenticationMiddleware{
		next: next,
		auth: auth,
	}
}

// ServeHTTP implements http.Handler
func (m *AuthenticationMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Skip auth for health check
	if r.URL.Path == "/api/v3/system/health" {
		m.next.ServeHTTP(w, r)
		return
	}

	// Check authentication
	authorized, err := m.auth(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"authentication_error","message":"Authentication failed"}`))
		return
	}

	if !authorized {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("WWW-Authenticate", `Bearer realm="PandaFuzz API"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized","message":"Invalid or missing authentication"}`))
		return
	}

	m.next.ServeHTTP(w, r)
}

// Example usage in master server:
/*
func (s *Server) setupAPIv3() {
	// Create integration config
	config := api_v3.DefaultIntegrationConfig()

	// Customize config
	config.EnableAuthentication = true
	config.AuthenticationFunc = s.authenticateRequest

	// Create integration
	integration := api_v3.NewIntegration(s.services, s.logger, config)

	// Add custom middleware if needed
	integration.AddMiddleware(s.customMiddleware)

	// Register routes
	integration.RegisterRoutes(s.router)
}
*/
