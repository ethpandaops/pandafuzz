package validation

import (
	"fmt"
	"strings"
)

// Validator defines the interface for validation
type Validator interface {
	Validate(interface{}) error
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface
func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error on field '%s': %s", e.Field, e.Message)
}

// ValidationErrors represents multiple validation errors
type ValidationErrors []ValidationError

// Error implements the error interface
func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}

	var messages []string
	for _, err := range e {
		messages = append(messages, err.Error())
	}

	return fmt.Sprintf("validation errors: %s", strings.Join(messages, "; "))
}

// Add adds a validation error
func (e *ValidationErrors) Add(field, message string) {
	*e = append(*e, ValidationError{
		Field:   field,
		Message: message,
	})
}

// AddError adds an error if it's a validation error
func (e *ValidationErrors) AddError(err error) {
	if err == nil {
		return
	}

	switch v := err.(type) {
	case ValidationError:
		*e = append(*e, v)
	case ValidationErrors:
		*e = append(*e, v...)
	default:
		// For non-validation errors, add as generic validation error
		*e = append(*e, ValidationError{
			Field:   "unknown",
			Message: err.Error(),
		})
	}
}

// HasErrors returns true if there are validation errors
func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

// ToError returns an error if there are validation errors, nil otherwise
func (e ValidationErrors) ToError() error {
	if e.HasErrors() {
		return e
	}
	return nil
}

// NewValidationError creates a new validation error
func NewValidationError(field, message string, args ...interface{}) ValidationError {
	if len(args) > 0 {
		message = fmt.Sprintf(message, args...)
	}
	return ValidationError{
		Field:   field,
		Message: message,
	}
}

// ValidatorFunc is a function that implements the Validator interface
type ValidatorFunc func(interface{}) error

// Validate implements the Validator interface
func (f ValidatorFunc) Validate(v interface{}) error {
	return f(v)
}

// CompositeValidator combines multiple validators
type CompositeValidator struct {
	validators []Validator
}

// NewCompositeValidator creates a new composite validator
func NewCompositeValidator(validators ...Validator) *CompositeValidator {
	return &CompositeValidator{
		validators: validators,
	}
}

// Add adds a validator to the composite
func (v *CompositeValidator) Add(validator Validator) {
	v.validators = append(v.validators, validator)
}

// Validate runs all validators and collects errors
func (v *CompositeValidator) Validate(value interface{}) error {
	var errors ValidationErrors

	for _, validator := range v.validators {
		if err := validator.Validate(value); err != nil {
			errors.AddError(err)
		}
	}

	return errors.ToError()
}

// FieldValidator validates a specific field
type FieldValidator struct {
	field      string
	validators []func(interface{}) error
}

// NewFieldValidator creates a new field validator
func NewFieldValidator(field string) *FieldValidator {
	return &FieldValidator{
		field:      field,
		validators: make([]func(interface{}) error, 0),
	}
}

// Required adds a required validation
func (v *FieldValidator) Required() *FieldValidator {
	v.validators = append(v.validators, func(value interface{}) error {
		if value == nil {
			return NewValidationError(v.field, "%s is required", v.field)
		}

		if str, ok := value.(string); ok && strings.TrimSpace(str) == "" {
			return NewValidationError(v.field, "%s is required", v.field)
		}

		return nil
	})
	return v
}

// MinLength adds a minimum length validation
func (v *FieldValidator) MinLength(min int) *FieldValidator {
	v.validators = append(v.validators, func(value interface{}) error {
		str, ok := value.(string)
		if !ok {
			return nil // Skip validation for non-strings
		}

		if len(str) < min {
			return NewValidationError(v.field, "%s must be at least %d characters long", v.field, min)
		}

		return nil
	})
	return v
}

// MaxLength adds a maximum length validation
func (v *FieldValidator) MaxLength(max int) *FieldValidator {
	v.validators = append(v.validators, func(value interface{}) error {
		str, ok := value.(string)
		if !ok {
			return nil // Skip validation for non-strings
		}

		if len(str) > max {
			return NewValidationError(v.field, "%s must be at most %d characters long", v.field, max)
		}

		return nil
	})
	return v
}

// Validate runs all field validators
func (v *FieldValidator) Validate(value interface{}) error {
	var errors ValidationErrors

	for _, validator := range v.validators {
		if err := validator(value); err != nil {
			errors.AddError(err)
		}
	}

	return errors.ToError()
}
