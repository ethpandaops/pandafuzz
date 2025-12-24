package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/errors"
	"github.com/sirupsen/logrus"
)

// ContextKey is a type for context keys to avoid collisions
type ContextKey string

const (
	// UserContextKey is the key for storing user information in context
	UserContextKey ContextKey = "user"
	// ClaimsContextKey is the key for storing JWT claims in context
	ClaimsContextKey ContextKey = "claims"
	// APIKeyContextKey is the key for storing API key information in context
	APIKeyContextKey ContextKey = "api_key"
)

// Claims represents JWT claims
type Claims struct {
	UserID      string   `json:"user_id"`
	Username    string   `json:"username"`
	Permissions []string `json:"permissions"`
	Role        string   `json:"role"`
	IssuedAt    int64    `json:"iat"`
	ExpiresAt   int64    `json:"exp"`
	Issuer      string   `json:"iss"`
	Subject     string   `json:"sub"`
}

// APIKeyInfo represents API key information
type APIKeyInfo struct {
	KeyID       string    `json:"key_id"`
	Name        string    `json:"name"`
	Permissions []string  `json:"permissions"`
	RateLimit   int       `json:"rate_limit"`
	CreatedAt   time.Time `json:"created_at"`
}

// APIKeyValidator is a function type for validating API keys
type APIKeyValidator func(apiKey string) (*APIKeyInfo, error)

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret       string
	APIKeyValidator APIKeyValidator
	Logger          logrus.FieldLogger
	SkipPaths       []string // Paths to skip authentication
}

// JWTAuth creates a JWT authentication middleware
func JWTAuth(secret string) func(http.Handler) http.Handler {
	return JWTAuthWithConfig(AuthConfig{
		JWTSecret: secret,
		Logger:    logrus.NewEntry(logrus.StandardLogger()),
	})
}

// JWTAuthWithConfig creates a JWT authentication middleware with configuration
func JWTAuthWithConfig(config AuthConfig) func(http.Handler) http.Handler {
	if config.Logger == nil {
		config.Logger = logrus.NewEntry(logrus.StandardLogger())
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip authentication for configured paths
			for _, path := range config.SkipPaths {
				if strings.HasPrefix(r.URL.Path, path) {
					next.ServeHTTP(w, r)
					return
				}
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				config.Logger.WithField("path", r.URL.Path).Warn("Missing Authorization header")
				errors.WriteErrorSimple(w, http.StatusUnauthorized, "Missing Authorization header")
				return
			}

			if !strings.HasPrefix(authHeader, "Bearer ") {
				config.Logger.WithField("path", r.URL.Path).Warn("Invalid Authorization header format")
				errors.WriteErrorSimple(w, http.StatusUnauthorized, "Invalid Authorization header format")
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := validateJWT(token, config.JWTSecret)
			if err != nil {
				config.Logger.WithFields(logrus.Fields{
					"path":  r.URL.Path,
					"error": err.Error(),
				}).Warn("Invalid JWT token")
				errors.WriteErrorSimple(w, http.StatusUnauthorized, "Invalid or expired token")
				return
			}

			// Check token expiration
			if claims.ExpiresAt > 0 && time.Now().Unix() > claims.ExpiresAt {
				config.Logger.WithFields(logrus.Fields{
					"path":       r.URL.Path,
					"expires_at": claims.ExpiresAt,
					"user_id":    claims.UserID,
				}).Warn("Expired JWT token")
				errors.WriteErrorSimple(w, http.StatusUnauthorized, "Token has expired")
				return
			}

			// Add claims to context
			ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)
			ctx = context.WithValue(ctx, UserContextKey, claims.UserID)

			config.Logger.WithFields(logrus.Fields{
				"path":    r.URL.Path,
				"user_id": claims.UserID,
				"role":    claims.Role,
			}).Debug("JWT authentication successful")

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// APIKeyAuth creates an API key authentication middleware
func APIKeyAuth(validator APIKeyValidator) func(http.Handler) http.Handler {
	return APIKeyAuthWithConfig(AuthConfig{
		APIKeyValidator: validator,
		Logger:          logrus.NewEntry(logrus.StandardLogger()),
	})
}

// APIKeyAuthWithConfig creates an API key authentication middleware with configuration
func APIKeyAuthWithConfig(config AuthConfig) func(http.Handler) http.Handler {
	if config.Logger == nil {
		config.Logger = logrus.NewEntry(logrus.StandardLogger())
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip authentication for configured paths
			for _, path := range config.SkipPaths {
				if strings.HasPrefix(r.URL.Path, path) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Check for API key in header
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				// Check for API key in Authorization header
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "ApiKey ") {
					apiKey = strings.TrimPrefix(authHeader, "ApiKey ")
				}
			}

			if apiKey == "" {
				config.Logger.WithField("path", r.URL.Path).Warn("Missing API key")
				errors.WriteErrorSimple(w, http.StatusUnauthorized, "Missing API key")
				return
			}

			if config.APIKeyValidator == nil {
				config.Logger.WithField("path", r.URL.Path).Error("API key validator not configured")
				errors.WriteErrorSimple(w, http.StatusInternalServerError, "Authentication not properly configured")
				return
			}

			keyInfo, err := config.APIKeyValidator(apiKey)
			if err != nil {
				config.Logger.WithFields(logrus.Fields{
					"path":  r.URL.Path,
					"error": err.Error(),
				}).Warn("Invalid API key")
				errors.WriteErrorSimple(w, http.StatusUnauthorized, "Invalid API key")
				return
			}

			// Add API key info to context
			ctx := context.WithValue(r.Context(), APIKeyContextKey, keyInfo)
			ctx = context.WithValue(ctx, UserContextKey, keyInfo.KeyID)

			config.Logger.WithFields(logrus.Fields{
				"path":   r.URL.Path,
				"key_id": keyInfo.KeyID,
				"name":   keyInfo.Name,
			}).Debug("API key authentication successful")

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermission creates a middleware that checks for specific permissions
func RequirePermission(permission string) func(http.Handler) http.Handler {
	return RequirePermissionWithConfig(permission, logrus.NewEntry(logrus.StandardLogger()))
}

// RequirePermissionWithConfig creates a permission checking middleware with configuration
func RequirePermissionWithConfig(permission string, logger logrus.FieldLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check JWT claims first
			if claims, ok := r.Context().Value(ClaimsContextKey).(*Claims); ok {
				if hasPermission(claims.Permissions, permission) {
					next.ServeHTTP(w, r)
					return
				}
				logger.WithFields(logrus.Fields{
					"path":       r.URL.Path,
					"user_id":    claims.UserID,
					"permission": permission,
					"user_perms": claims.Permissions,
				}).Warn("Insufficient permissions")
			} else if keyInfo, ok := r.Context().Value(APIKeyContextKey).(*APIKeyInfo); ok {
				// Check API key permissions
				if hasPermission(keyInfo.Permissions, permission) {
					next.ServeHTTP(w, r)
					return
				}
				logger.WithFields(logrus.Fields{
					"path":       r.URL.Path,
					"key_id":     keyInfo.KeyID,
					"permission": permission,
					"key_perms":  keyInfo.Permissions,
				}).Warn("Insufficient permissions")
			} else {
				logger.WithFields(logrus.Fields{
					"path":       r.URL.Path,
					"permission": permission,
				}).Warn("No authentication context found")
			}

			errors.WriteErrorWithDetails(w, http.StatusForbidden, "Insufficient permissions", map[string]interface{}{
				"required_permission": permission,
			})
		})
	}
}

// ExtractClaims extracts JWT claims from the request context
func ExtractClaims(r *http.Request) (*Claims, error) {
	claims, ok := r.Context().Value(ClaimsContextKey).(*Claims)
	if !ok {
		return nil, fmt.Errorf("no JWT claims found in context")
	}
	return claims, nil
}

// ExtractAPIKeyInfo extracts API key information from the request context
func ExtractAPIKeyInfo(r *http.Request) (*APIKeyInfo, error) {
	keyInfo, ok := r.Context().Value(APIKeyContextKey).(*APIKeyInfo)
	if !ok {
		return nil, fmt.Errorf("no API key info found in context")
	}
	return keyInfo, nil
}

// ExtractUserID extracts user ID from the request context (works for both JWT and API key auth)
func ExtractUserID(r *http.Request) (string, error) {
	userID, ok := r.Context().Value(UserContextKey).(string)
	if !ok {
		return "", fmt.Errorf("no user ID found in context")
	}
	return userID, nil
}

// validateJWT validates a JWT token and returns the claims
func validateJWT(token, secret string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Decode header
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid header encoding: %w", err)
	}

	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("invalid header format: %w", err)
	}

	if header.Algorithm != "HS256" {
		return nil, fmt.Errorf("unsupported algorithm: %s", header.Algorithm)
	}

	// Decode payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid payload encoding: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("invalid payload format: %w", err)
	}

	// Verify signature
	message := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	expectedSignature := mac.Sum(nil)

	if !hmac.Equal(signature, expectedSignature) {
		return nil, fmt.Errorf("invalid signature")
	}

	return &claims, nil
}

// hasPermission checks if a permission exists in the permissions slice
func hasPermission(permissions []string, required string) bool {
	for _, perm := range permissions {
		if perm == required || perm == "*" {
			return true
		}
	}
	return false
}

// GenerateJWT generates a JWT token with the given claims (utility function for testing)
func GenerateJWT(claims Claims, secret string) (string, error) {
	// Create header
	header := map[string]interface{}{
		"alg": "HS256",
		"typ": "JWT",
	}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("failed to marshal header: %w", err)
	}
	headerEncoded := base64.RawURLEncoding.EncodeToString(headerBytes)

	// Create payload
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal claims: %w", err)
	}
	payloadEncoded := base64.RawURLEncoding.EncodeToString(payloadBytes)

	// Create signature
	message := headerEncoded + "." + payloadEncoded
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	signature := mac.Sum(nil)
	signatureEncoded := base64.RawURLEncoding.EncodeToString(signature)

	return message + "." + signatureEncoded, nil
}
