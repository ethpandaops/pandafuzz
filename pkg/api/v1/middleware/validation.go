package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/ethpandaops/pandafuzz/pkg/interfaces/api/rest/v1"
	"github.com/go-playground/validator/v10"
	"github.com/sirupsen/logrus"
)

// ValidationConfig holds configuration for validation middleware
type ValidationConfig struct {
	// MaxRequestSize is the maximum allowed request body size in bytes
	MaxRequestSize int64

	// RequiredContentTypes lists the allowed content types for requests with bodies
	RequiredContentTypes []string

	// SkipPaths is a list of paths to skip validation
	SkipPaths []string

	// SkipMethods is a list of HTTP methods to skip validation
	SkipMethods []string

	// CustomValidators is a map of custom validation functions
	CustomValidators map[string]validator.Func

	// Logger for validation messages
	Logger logrus.FieldLogger

	// StrictMode enables strict validation (additional checks)
	StrictMode bool

	// AllowUnknownFields allows JSON requests to contain unknown fields
	AllowUnknownFields bool
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string      `json:"field"`
	Tag     string      `json:"tag"`
	Value   interface{} `json:"value,omitempty"`
	Message string      `json:"message"`
	Param   string      `json:"param,omitempty"`
}

// RequestValidator provides request validation functionality
type RequestValidator struct {
	validator *validator.Validate
	config    ValidationConfig
}

// DefaultValidationConfig returns a default validation configuration
func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		MaxRequestSize: 10 * 1024 * 1024, // 10MB
		RequiredContentTypes: []string{
			"application/json",
			"application/x-www-form-urlencoded",
			"multipart/form-data",
		},
		SkipMethods: []string{
			http.MethodGet,
			http.MethodHead,
			http.MethodOptions,
		},
		Logger:             logrus.NewEntry(logrus.StandardLogger()),
		StrictMode:         false,
		AllowUnknownFields: true,
	}
}

// NewRequestValidator creates a new request validator
func NewRequestValidator(config ValidationConfig) *RequestValidator {
	if config.Logger == nil {
		config.Logger = logrus.NewEntry(logrus.StandardLogger())
	}

	v := validator.New()

	// Register custom validators
	for tag, fn := range config.CustomValidators {
		v.RegisterValidation(tag, fn)
	}

	// Register common custom validators for PandaFuzz
	registerPandaFuzzValidators(v)

	// Use JSON field names for validation
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	return &RequestValidator{
		validator: v,
		config:    config,
	}
}

// ValidateRequest creates a validation middleware
func ValidateRequest() func(http.Handler) http.Handler {
	return ValidateRequestWithConfig(DefaultValidationConfig())
}

// ValidateRequestWithConfig creates a validation middleware with configuration
func ValidateRequestWithConfig(config ValidationConfig) func(http.Handler) http.Handler {
	validator := NewRequestValidator(config)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip validation for configured paths
			for _, path := range config.SkipPaths {
				if strings.HasPrefix(r.URL.Path, path) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Skip validation for configured methods
			for _, method := range config.SkipMethods {
				if strings.EqualFold(r.Method, method) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Validate request size
			if r.ContentLength > config.MaxRequestSize {
				config.Logger.WithFields(logrus.Fields{
					"path":           r.URL.Path,
					"content_length": r.ContentLength,
					"max_size":       config.MaxRequestSize,
				}).Warn("Request body too large")

				v1.WriteErrorWithDetails(w, http.StatusRequestEntityTooLarge, "Request body too large", map[string]interface{}{
					"max_size_bytes": config.MaxRequestSize,
					"received_bytes": r.ContentLength,
				})
				return
			}

			// Validate content type for requests with bodies
			if r.ContentLength > 0 || r.Header.Get("Transfer-Encoding") == "chunked" {
				contentType := r.Header.Get("Content-Type")
				if contentType != "" && !validator.isAllowedContentType(contentType) {
					config.Logger.WithFields(logrus.Fields{
						"path":         r.URL.Path,
						"content_type": contentType,
						"allowed":      config.RequiredContentTypes,
					}).Warn("Invalid content type")

					v1.WriteErrorWithDetails(w, http.StatusUnsupportedMediaType, "Unsupported content type", map[string]interface{}{
						"received":      contentType,
						"allowed_types": config.RequiredContentTypes,
					})
					return
				}
			}

			// Validate query parameters
			if errors := validator.validateQueryParams(r); len(errors) > 0 {
				config.Logger.WithFields(logrus.Fields{
					"path":   r.URL.Path,
					"errors": errors,
				}).Warn("Query parameter validation failed")

				v1.WriteErrorWithDetails(w, http.StatusBadRequest, "Invalid query parameters", map[string]interface{}{
					"validation_errors": errors,
				})
				return
			}

			// Validate path parameters
			if errors := validator.validatePathParams(r); len(errors) > 0 {
				config.Logger.WithFields(logrus.Fields{
					"path":   r.URL.Path,
					"errors": errors,
				}).Warn("Path parameter validation failed")

				v1.WriteErrorWithDetails(w, http.StatusBadRequest, "Invalid path parameters", map[string]interface{}{
					"validation_errors": errors,
				})
				return
			}

			// For JSON requests, validate the request body
			if strings.Contains(r.Header.Get("Content-Type"), "application/json") && r.ContentLength > 0 {
				if err := validator.validateJSONBody(w, r); err != nil {
					return // Error already written by validateJSONBody
				}
			}

			config.Logger.WithField("path", r.URL.Path).Debug("Request validation passed")
			next.ServeHTTP(w, r)
		})
	}
}

// validateJSONBody validates JSON request bodies
func (rv *RequestValidator) validateJSONBody(w http.ResponseWriter, r *http.Request) error {
	// Read the body
	body, err := io.ReadAll(io.LimitReader(r.Body, rv.config.MaxRequestSize))
	if err != nil {
		rv.config.Logger.WithError(err).Error("Failed to read request body")
		v1.WriteError(w, http.StatusBadRequest, "Failed to read request body")
		return err
	}

	// Replace the body so it can be read again by handlers
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	// Skip validation for empty bodies
	if len(body) == 0 {
		return nil
	}

	// Validate JSON syntax
	var jsonData interface{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if !rv.config.AllowUnknownFields {
		decoder.DisallowUnknownFields()
	}

	if err := decoder.Decode(&jsonData); err != nil {
		rv.config.Logger.WithError(err).Warn("Invalid JSON in request body")

		var details map[string]interface{}
		if strings.Contains(err.Error(), "unknown field") {
			details = map[string]interface{}{
				"error": "Unknown fields are not allowed",
				"hint":  "Remove any extra fields from your JSON request",
			}
		} else {
			details = map[string]interface{}{
				"error": err.Error(),
			}
		}

		v1.WriteErrorWithDetails(w, http.StatusBadRequest, "Invalid JSON format", details)
		return err
	}

	// Additional strict mode validations
	if rv.config.StrictMode {
		if err := rv.validateJSONStructure(jsonData); err != nil {
			rv.config.Logger.WithError(err).Warn("JSON structure validation failed")
			v1.WriteErrorWithDetails(w, http.StatusBadRequest, "Invalid JSON structure", map[string]interface{}{
				"validation_errors": err.Error(),
			})
			return err
		}
	}

	return nil
}

// validateQueryParams validates query parameters
func (rv *RequestValidator) validateQueryParams(r *http.Request) []ValidationError {
	var errors []ValidationError

	// Common query parameter validations
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if val, err := strconv.Atoi(limit); err != nil || val < 1 || val > 1000 {
			errors = append(errors, ValidationError{
				Field:   "limit",
				Tag:     "range",
				Value:   limit,
				Message: "limit must be between 1 and 1000",
				Param:   "1-1000",
			})
		}
	}

	if offset := r.URL.Query().Get("offset"); offset != "" {
		if val, err := strconv.Atoi(offset); err != nil || val < 0 {
			errors = append(errors, ValidationError{
				Field:   "offset",
				Tag:     "min",
				Value:   offset,
				Message: "offset must be non-negative",
				Param:   "0",
			})
		}
	}

	// Validate sort parameter
	if sort := r.URL.Query().Get("sort"); sort != "" {
		validSortFields := []string{"id", "name", "created_at", "updated_at", "status"}
		field := strings.TrimPrefix(sort, "-") // Remove desc prefix
		if !contains(validSortFields, field) {
			errors = append(errors, ValidationError{
				Field:   "sort",
				Tag:     "oneof",
				Value:   sort,
				Message: fmt.Sprintf("sort field must be one of: %s", strings.Join(validSortFields, ", ")),
			})
		}
	}

	if len(errors) > 0 {
		return errors
	}
	return nil
}

// validatePathParams validates path parameters
func (rv *RequestValidator) validatePathParams(r *http.Request) []ValidationError {
	var errors []ValidationError

	// Extract path segments for validation
	pathSegments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	// Validate UUID parameters (common in REST APIs)
	for i, segment := range pathSegments {
		if isLikelyUUID(segment) {
			if !isValidUUID(segment) {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("path_param_%d", i),
					Tag:     "uuid",
					Value:   segment,
					Message: "invalid UUID format",
				})
			}
		}
	}

	if len(errors) > 0 {
		return errors
	}
	return nil
}

// validateJSONStructure performs additional JSON structure validations
func (rv *RequestValidator) validateJSONStructure(data interface{}) error {
	switch v := data.(type) {
	case map[string]interface{}:
		return rv.validateJSONObject(v)
	case []interface{}:
		return rv.validateJSONArray(v)
	default:
		return nil
	}
}

// validateJSONObject validates a JSON object structure
func (rv *RequestValidator) validateJSONObject(obj map[string]interface{}) error {
	// Check for excessively deep nesting
	if rv.getObjectDepth(obj) > 10 {
		return fmt.Errorf("JSON object nesting too deep (max 10 levels)")
	}

	// Check for excessively large objects
	if len(obj) > 100 {
		return fmt.Errorf("JSON object has too many fields (max 100)")
	}

	// Recursively validate nested objects
	for key, value := range obj {
		if len(key) > 100 {
			return fmt.Errorf("JSON field name too long: %s", key)
		}

		if err := rv.validateJSONStructure(value); err != nil {
			return err
		}
	}

	return nil
}

// validateJSONArray validates a JSON array structure
func (rv *RequestValidator) validateJSONArray(arr []interface{}) error {
	// Check array size
	if len(arr) > 1000 {
		return fmt.Errorf("JSON array too large (max 1000 elements)")
	}

	// Validate array elements
	for _, item := range arr {
		if err := rv.validateJSONStructure(item); err != nil {
			return err
		}
	}

	return nil
}

// isAllowedContentType checks if content type is allowed
func (rv *RequestValidator) isAllowedContentType(contentType string) bool {
	// Parse main content type (ignore charset and other parameters)
	mainType := strings.SplitN(contentType, ";", 2)[0]
	mainType = strings.TrimSpace(mainType)

	for _, allowed := range rv.config.RequiredContentTypes {
		if strings.EqualFold(mainType, allowed) {
			return true
		}
	}
	return false
}

// registerPandaFuzzValidators registers custom validators for PandaFuzz
func registerPandaFuzzValidators(v *validator.Validate) {
	// Fuzzer type validator
	v.RegisterValidation("fuzzer_type", func(fl validator.FieldLevel) bool {
		fuzzerType := fl.Field().String()
		validTypes := []string{"libfuzzer", "afl++", "honggfuzz"}
		return contains(validTypes, fuzzerType)
	})

	// Campaign status validator
	v.RegisterValidation("campaign_status", func(fl validator.FieldLevel) bool {
		status := fl.Field().String()
		validStatuses := []string{"active", "paused", "completed", "failed"}
		return contains(validStatuses, status)
	})

	// Job status validator
	v.RegisterValidation("job_status", func(fl validator.FieldLevel) bool {
		status := fl.Field().String()
		validStatuses := []string{"pending", "running", "completed", "failed", "cancelled"}
		return contains(validStatuses, status)
	})

	// Bot capability validator
	v.RegisterValidation("bot_capability", func(fl validator.FieldLevel) bool {
		capability := fl.Field().String()
		validCapabilities := []string{"fuzzing", "coverage", "minimization", "reproduction", "analysis"}
		return contains(validCapabilities, capability)
	})
}

// Helper functions

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}

// isLikelyUUID checks if a string looks like a UUID
func isLikelyUUID(s string) bool {
	return len(s) == 36 && strings.Count(s, "-") == 4
}

// isValidUUID validates UUID format
func isValidUUID(s string) bool {
	if len(s) != 36 {
		return false
	}

	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// getObjectDepth calculates the maximum depth of a JSON object
func (rv *RequestValidator) getObjectDepth(obj map[string]interface{}) int {
	maxDepth := 0
	for _, value := range obj {
		depth := 1
		if nestedObj, ok := value.(map[string]interface{}); ok {
			depth += rv.getObjectDepth(nestedObj)
		} else if arr, ok := value.([]interface{}); ok {
			for _, item := range arr {
				if nestedObj, ok := item.(map[string]interface{}); ok {
					itemDepth := 1 + rv.getObjectDepth(nestedObj)
					if itemDepth > depth {
						depth = itemDepth
					}
				}
			}
		}
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	return maxDepth
}
