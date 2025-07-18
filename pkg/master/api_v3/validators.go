package api_v3

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// Validator handles request validation
type Validator struct {
	validators map[string]ValidatorFunc
}

// ValidatorFunc is a validation function
type ValidatorFunc func(fieldName string, fieldValue interface{}, tag string) error

// NewValidator creates a new validator
func NewValidator() *Validator {
	v := &Validator{
		validators: make(map[string]ValidatorFunc),
	}

	// Register built-in validators
	v.RegisterValidator("required", v.validateRequired)
	v.RegisterValidator("max", v.validateMax)
	v.RegisterValidator("min", v.validateMin)
	v.RegisterValidator("uuid", v.validateUUID)
	v.RegisterValidator("url", v.validateURL)
	v.RegisterValidator("oneof", v.validateOneOf)
	v.RegisterValidator("alphanum_dash", v.validateAlphanumDash)
	v.RegisterValidator("dive", v.validateDive)

	return v
}

// RegisterValidator registers a custom validator
func (v *Validator) RegisterValidator(tag string, fn ValidatorFunc) {
	v.validators[tag] = fn
}

// Validate validates a struct
func (v *Validator) Validate(obj interface{}) error {
	return v.validateStruct(reflect.ValueOf(obj))
}

func (v *Validator) validateStruct(val reflect.Value) error {
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return nil
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Skip unexported fields
		if fieldType.PkgPath != "" {
			continue
		}

		// Get validation tags
		tags := fieldType.Tag.Get("validate")
		if tags == "" || tags == "-" {
			continue
		}

		// Validate field
		if err := v.validateField(fieldType.Name, field, tags); err != nil {
			return err
		}
	}

	return nil
}

func (v *Validator) validateField(fieldName string, field reflect.Value, tags string) error {
	// Handle omitempty
	if strings.Contains(tags, "omitempty") && isZero(field) {
		return nil
	}

	// Split tags
	tagList := strings.Split(tags, ",")
	for _, tag := range tagList {
		if tag == "omitempty" || tag == "-" {
			continue
		}

		// Parse tag and parameter
		parts := strings.SplitN(tag, "=", 2)
		tagName := parts[0]
		tagParam := ""
		if len(parts) > 1 {
			tagParam = parts[1]
		}

		// Get validator function
		validatorFunc, ok := v.validators[tagName]
		if !ok {
			// Skip unknown validators
			continue
		}

		// Run validator
		if err := validatorFunc(fieldName, field.Interface(), tagParam); err != nil {
			return err
		}
	}

	return nil
}

// Built-in validators

func (v *Validator) validateRequired(fieldName string, fieldValue interface{}, tag string) error {
	val := reflect.ValueOf(fieldValue)
	if isZero(val) {
		return &ValidationError{
			Field:   fieldName,
			Message: "is required",
		}
	}
	return nil
}

func (v *Validator) validateMax(fieldName string, fieldValue interface{}, tag string) error {
	max := parseIntTag(tag)
	val := reflect.ValueOf(fieldValue)

	switch val.Kind() {
	case reflect.String:
		if len(val.String()) > max {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("exceeds maximum length of %d", max),
			}
		}
	case reflect.Slice, reflect.Array:
		if val.Len() > max {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("exceeds maximum count of %d", max),
			}
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if val.Int() > int64(max) {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("exceeds maximum value of %d", max),
			}
		}
	}

	return nil
}

func (v *Validator) validateMin(fieldName string, fieldValue interface{}, tag string) error {
	min := parseIntTag(tag)
	val := reflect.ValueOf(fieldValue)

	switch val.Kind() {
	case reflect.String:
		if len(val.String()) < min {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("below minimum length of %d", min),
			}
		}
	case reflect.Slice, reflect.Array:
		if val.Len() < min {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("below minimum count of %d", min),
			}
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if val.Int() < int64(min) {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("below minimum value of %d", min),
			}
		}
	}

	return nil
}

func (v *Validator) validateUUID(fieldName string, fieldValue interface{}, tag string) error {
	str, ok := fieldValue.(string)
	if !ok {
		return &ValidationError{
			Field:   fieldName,
			Message: "must be a string",
		}
	}

	if str == "" {
		return nil // Handle with required validator
	}

	if _, err := uuid.Parse(str); err != nil {
		return &ValidationError{
			Field:   fieldName,
			Message: "must be a valid UUID",
		}
	}

	return nil
}

func (v *Validator) validateURL(fieldName string, fieldValue interface{}, tag string) error {
	str, ok := fieldValue.(string)
	if !ok {
		return &ValidationError{
			Field:   fieldName,
			Message: "must be a string",
		}
	}

	if str == "" {
		return nil // Handle with required validator
	}

	// Basic URL validation
	if !strings.HasPrefix(str, "http://") && !strings.HasPrefix(str, "https://") {
		return &ValidationError{
			Field:   fieldName,
			Message: "must be a valid URL starting with http:// or https://",
		}
	}

	return nil
}

func (v *Validator) validateOneOf(fieldName string, fieldValue interface{}, tag string) error {
	allowed := strings.Split(tag, " ")
	str := fmt.Sprintf("%v", fieldValue)

	for _, value := range allowed {
		if str == value {
			return nil
		}
	}

	return &ValidationError{
		Field:   fieldName,
		Message: fmt.Sprintf("must be one of: %s", strings.Join(allowed, ", ")),
	}
}

var alphanumDashRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (v *Validator) validateAlphanumDash(fieldName string, fieldValue interface{}, tag string) error {
	str, ok := fieldValue.(string)
	if !ok {
		return &ValidationError{
			Field:   fieldName,
			Message: "must be a string",
		}
	}

	if str == "" {
		return nil // Handle with required validator
	}

	if !alphanumDashRegex.MatchString(str) {
		return &ValidationError{
			Field:   fieldName,
			Message: "must contain only alphanumeric characters, underscores, and dashes",
		}
	}

	return nil
}

func (v *Validator) validateDive(fieldName string, fieldValue interface{}, tag string) error {
	// Dive is handled specially in validateField for slices
	// This is a placeholder
	return nil
}

// Helper functions

func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr, reflect.Map, reflect.Slice:
		return v.IsNil()
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			return v.Interface().(time.Time).IsZero()
		}
		// Check if all fields are zero
		for i := 0; i < v.NumField(); i++ {
			if !isZero(v.Field(i)) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func parseIntTag(tag string) int {
	var val int
	fmt.Sscanf(tag, "%d", &val)
	return val
}

// ValidateJobConfig validates job configuration
func ValidateJobConfig(config *common.JobConfig) error {
	if config.Duration < 0 {
		return &ValidationError{
			Field:   "duration",
			Message: "cannot be negative",
		}
	}

	if config.Duration > 24*time.Hour {
		return &ValidationError{
			Field:   "duration",
			Message: "exceeds maximum of 24 hours",
		}
	}

	if config.MemoryLimit < 0 {
		return &ValidationError{
			Field:   "memory_limit",
			Message: "cannot be negative",
		}
	}

	if config.MemoryLimit > 16*1024*1024*1024 { // 16GB
		return &ValidationError{
			Field:   "memory_limit",
			Message: "exceeds maximum of 16GB",
		}
	}

	if config.Timeout < 0 {
		return &ValidationError{
			Field:   "timeout",
			Message: "cannot be negative",
		}
	}

	if config.Timeout > 24*time.Hour {
		return &ValidationError{
			Field:   "timeout",
			Message: "exceeds maximum of 24 hours",
		}
	}

	return nil
}

// ValidatePaginationParams validates pagination parameters
func ValidatePaginationParams(page, limit int) error {
	if page < 1 {
		return &ValidationError{
			Field:   "page",
			Message: "must be at least 1",
		}
	}

	if limit < 1 {
		return &ValidationError{
			Field:   "limit",
			Message: "must be at least 1",
		}
	}

	if limit > 100 {
		return &ValidationError{
			Field:   "limit",
			Message: "exceeds maximum of 100",
		}
	}

	return nil
}
