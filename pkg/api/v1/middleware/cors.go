package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// CORSConfig holds configuration for CORS middleware
type CORSConfig struct {
	// AllowedOrigins is a list of allowed origins. Use ["*"] to allow all origins.
	AllowedOrigins []string

	// AllowedMethods is a list of allowed HTTP methods.
	// Default: ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"]
	AllowedMethods []string

	// AllowedHeaders is a list of allowed request headers.
	// Default: ["Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-API-Key"]
	AllowedHeaders []string

	// ExposedHeaders is a list of headers that are safe to expose to the API
	ExposedHeaders []string

	// AllowCredentials indicates whether credentials are allowed
	AllowCredentials bool

	// MaxAge indicates how long (in seconds) the results of a preflight request can be cached
	// Default: 86400 (24 hours)
	MaxAge int

	// OptionsPassthrough allows preflight requests to pass through to the handler
	// when true, preflight requests are handled by the next handler instead of being
	// responded to directly by the CORS middleware
	OptionsPassthrough bool

	// SkipPaths is a list of paths to skip CORS handling
	SkipPaths []string

	// Logger for debugging CORS requests
	Logger logrus.FieldLogger

	// Debug enables debug logging for CORS requests
	Debug bool
}

// DefaultCORSConfig returns a default CORS configuration
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodHead,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-CSRF-Token",
			"X-API-Key",
			"X-Requested-With",
			"X-Real-IP",
			"X-Forwarded-For",
			"X-Forwarded-Proto",
		},
		ExposedHeaders: []string{
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
			"X-RateLimit-Reset",
			"X-Request-ID",
		},
		AllowCredentials: false,
		MaxAge:           86400, // 24 hours
		Logger:           logrus.NewEntry(logrus.StandardLogger()),
	}
}

// CORS creates a CORS middleware with default configuration
func CORS() func(http.Handler) http.Handler {
	return CORSWithConfig(DefaultCORSConfig())
}

// CORSWithConfig creates a CORS middleware with custom configuration
func CORSWithConfig(config CORSConfig) func(http.Handler) http.Handler {
	// Apply defaults
	if len(config.AllowedOrigins) == 0 {
		config.AllowedOrigins = []string{"*"}
	}
	if len(config.AllowedMethods) == 0 {
		config.AllowedMethods = []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodHead,
			http.MethodOptions,
		}
	}
	if len(config.AllowedHeaders) == 0 {
		config.AllowedHeaders = []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-CSRF-Token",
			"X-API-Key",
		}
	}
	if config.MaxAge == 0 {
		config.MaxAge = 86400
	}
	if config.Logger == nil {
		config.Logger = logrus.NewEntry(logrus.StandardLogger())
	}

	// Pre-process configuration for performance
	allowAllOrigins := len(config.AllowedOrigins) == 1 && config.AllowedOrigins[0] == "*"
	allowedOriginsMap := make(map[string]bool)
	for _, origin := range config.AllowedOrigins {
		allowedOriginsMap[origin] = true
	}

	allowedMethodsStr := strings.Join(config.AllowedMethods, ", ")
	allowedHeadersStr := strings.Join(config.AllowedHeaders, ", ")
	exposedHeadersStr := strings.Join(config.ExposedHeaders, ", ")
	maxAgeStr := strconv.Itoa(config.MaxAge)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip CORS for configured paths
			for _, path := range config.SkipPaths {
				if strings.HasPrefix(r.URL.Path, path) {
					next.ServeHTTP(w, r)
					return
				}
			}

			origin := r.Header.Get("Origin")

			if config.Debug {
				config.Logger.WithFields(logrus.Fields{
					"method": r.Method,
					"path":   r.URL.Path,
					"origin": origin,
				}).Debug("Processing CORS request")
			}

			// Handle preflight requests
			if r.Method == http.MethodOptions {
				handlePreflightRequest(w, r, config, origin, allowAllOrigins, allowedOriginsMap,
					allowedMethodsStr, allowedHeadersStr, maxAgeStr)

				if !config.OptionsPassthrough {
					return // Stop processing for preflight unless passthrough is enabled
				}
			}

			// Handle actual request
			handleActualRequest(w, r, config, origin, allowAllOrigins, allowedOriginsMap, exposedHeadersStr)

			next.ServeHTTP(w, r)
		})
	}
}

// handlePreflightRequest handles CORS preflight (OPTIONS) requests
func handlePreflightRequest(w http.ResponseWriter, r *http.Request, config CORSConfig,
	origin string, allowAllOrigins bool, allowedOriginsMap map[string]bool,
	allowedMethodsStr, allowedHeadersStr, maxAgeStr string) {

	// Check if origin is allowed
	if origin != "" && (allowAllOrigins || allowedOriginsMap[origin]) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else if allowAllOrigins {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	// Set allowed methods
	if requestedMethod := r.Header.Get("Access-Control-Request-Method"); requestedMethod != "" {
		if isMethodAllowed(requestedMethod, config.AllowedMethods) {
			w.Header().Set("Access-Control-Allow-Methods", allowedMethodsStr)
		}
	} else {
		w.Header().Set("Access-Control-Allow-Methods", allowedMethodsStr)
	}

	// Set allowed headers
	if requestedHeaders := r.Header.Get("Access-Control-Request-Headers"); requestedHeaders != "" {
		if areHeadersAllowed(requestedHeaders, config.AllowedHeaders) {
			w.Header().Set("Access-Control-Allow-Headers", allowedHeadersStr)
		}
	} else {
		w.Header().Set("Access-Control-Allow-Headers", allowedHeadersStr)
	}

	// Set credentials
	if config.AllowCredentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	// Set max age
	w.Header().Set("Access-Control-Max-Age", maxAgeStr)

	// Set Vary header for proper caching
	w.Header().Add("Vary", "Origin")
	w.Header().Add("Vary", "Access-Control-Request-Method")
	w.Header().Add("Vary", "Access-Control-Request-Headers")

	if config.Debug {
		config.Logger.WithFields(logrus.Fields{
			"origin":          origin,
			"allowed_methods": allowedMethodsStr,
			"allowed_headers": allowedHeadersStr,
			"max_age":         maxAgeStr,
		}).Debug("Preflight request handled")
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleActualRequest handles actual CORS requests (non-preflight)
func handleActualRequest(w http.ResponseWriter, r *http.Request, config CORSConfig,
	origin string, allowAllOrigins bool, allowedOriginsMap map[string]bool, exposedHeadersStr string) {

	// Check if origin is allowed
	if origin != "" && (allowAllOrigins || allowedOriginsMap[origin]) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else if allowAllOrigins {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	// Set credentials
	if config.AllowCredentials && origin != "" && !allowAllOrigins {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	// Set exposed headers
	if exposedHeadersStr != "" {
		w.Header().Set("Access-Control-Expose-Headers", exposedHeadersStr)
	}

	// Set Vary header for proper caching
	w.Header().Add("Vary", "Origin")

	if config.Debug {
		config.Logger.WithFields(logrus.Fields{
			"method":          r.Method,
			"origin":          origin,
			"exposed_headers": exposedHeadersStr,
		}).Debug("Actual request handled")
	}
}

// isMethodAllowed checks if a method is in the allowed methods list
func isMethodAllowed(method string, allowedMethods []string) bool {
	for _, allowed := range allowedMethods {
		if strings.EqualFold(method, allowed) {
			return true
		}
	}
	return false
}

// areHeadersAllowed checks if all requested headers are in the allowed headers list
func areHeadersAllowed(requestedHeaders string, allowedHeaders []string) bool {
	// Parse requested headers
	headers := strings.Split(requestedHeaders, ",")

	// Create a map of allowed headers for faster lookup (case-insensitive)
	allowedMap := make(map[string]bool)
	for _, header := range allowedHeaders {
		allowedMap[strings.ToLower(strings.TrimSpace(header))] = true
	}

	// Check each requested header
	for _, header := range headers {
		normalized := strings.ToLower(strings.TrimSpace(header))
		if normalized != "" && !allowedMap[normalized] {
			return false
		}
	}

	return true
}

// SecureCORS returns a CORS configuration suitable for production environments
func SecureCORS(allowedOrigins []string) CORSConfig {
	return CORSConfig{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodHead,
		}, // Note: OPTIONS is handled automatically, no need to include it
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-API-Key",
			"X-Requested-With",
		},
		ExposedHeaders: []string{
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
			"X-RateLimit-Reset",
			"X-Request-ID",
		},
		AllowCredentials: true,
		MaxAge:           3600, // 1 hour
		Logger:           logrus.NewEntry(logrus.StandardLogger()),
	}
}

// DevelopmentCORS returns a permissive CORS configuration for development
func DevelopmentCORS() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodHead,
			http.MethodOptions,
		},
		AllowedHeaders: []string{"*"},
		ExposedHeaders: []string{
			"*",
		},
		AllowCredentials: false, // Cannot be true with wildcard origins
		MaxAge:           86400,
		Debug:            true,
		Logger:           logrus.NewEntry(logrus.StandardLogger()),
	}
}

// ValidateConfig validates the CORS configuration
func ValidateConfig(config CORSConfig) error {
	// Check for invalid combination: credentials + wildcard origins
	if config.AllowCredentials {
		for _, origin := range config.AllowedOrigins {
			if origin == "*" {
				return fmt.Errorf("cannot use wildcard origin with credentials")
			}
		}
	}

	return nil
}
