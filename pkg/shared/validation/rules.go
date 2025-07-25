package validation

import (
	"net/mail"
	"regexp"
	"strings"
)

// Common validation patterns
var (
	// UUID v4 pattern
	uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	// Phone number pattern (simple international format)
	phonePattern = regexp.MustCompile(`^\+?[1-9]\d{1,14}$`)

	// URL pattern
	urlPattern = regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)

	// Alphanumeric with underscores and hyphens
	identifierPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// ValidateEmail validates an email address
func ValidateEmail(email string) error {
	if email == "" {
		return NewValidationError("email", "Email is required")
	}

	_, err := mail.ParseAddress(email)
	if err != nil {
		return NewValidationError("email", "Invalid email format")
	}

	return nil
}

// ValidatePhone validates a phone number
func ValidatePhone(phone string) error {
	if phone == "" {
		return NewValidationError("phone", "Phone number is required")
	}

	// Remove spaces and dashes for validation
	cleaned := strings.ReplaceAll(phone, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")

	if !phonePattern.MatchString(cleaned) {
		return NewValidationError("phone", "Invalid phone number format")
	}

	return nil
}

// ValidatePassword validates a password meets security requirements
func ValidatePassword(password string) error {
	if password == "" {
		return NewValidationError("password", "Password is required")
	}

	if len(password) < 8 {
		return NewValidationError("password", "Password must be at least 8 characters long")
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, char := range password {
		switch {
		case 'A' <= char && char <= 'Z':
			hasUpper = true
		case 'a' <= char && char <= 'z':
			hasLower = true
		case '0' <= char && char <= '9':
			hasDigit = true
		case strings.ContainsRune("!@#$%^&*()_+-=[]{}|;:,.<>?", char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return NewValidationError("password", "Password must contain at least one uppercase letter")
	}
	if !hasLower {
		return NewValidationError("password", "Password must contain at least one lowercase letter")
	}
	if !hasDigit {
		return NewValidationError("password", "Password must contain at least one digit")
	}
	if !hasSpecial {
		return NewValidationError("password", "Password must contain at least one special character")
	}

	return nil
}

// IsValidUUID checks if a string is a valid UUID v4
func IsValidUUID(uuid string) bool {
	return uuidPattern.MatchString(strings.ToLower(uuid))
}

// ValidateUUID validates a UUID string
func ValidateUUID(uuid string) error {
	if uuid == "" {
		return NewValidationError("uuid", "UUID is required")
	}

	if !IsValidUUID(uuid) {
		return NewValidationError("uuid", "Invalid UUID format")
	}

	return nil
}

// ValidateURL validates a URL
func ValidateURL(url string) error {
	if url == "" {
		return NewValidationError("url", "URL is required")
	}

	if !urlPattern.MatchString(url) {
		return NewValidationError("url", "Invalid URL format")
	}

	return nil
}

// ValidateIdentifier validates an identifier (alphanumeric with underscores and hyphens)
func ValidateIdentifier(identifier string) error {
	if identifier == "" {
		return NewValidationError("identifier", "Identifier is required")
	}

	if !identifierPattern.MatchString(identifier) {
		return NewValidationError("identifier", "Identifier must contain only letters, numbers, underscores, and hyphens")
	}

	return nil
}

// ValidateLength validates string length
func ValidateLength(value, field string, min, max int) error {
	length := len(value)

	if min > 0 && length < min {
		return NewValidationError(field, "Must be at least %d characters long", min)
	}

	if max > 0 && length > max {
		return NewValidationError(field, "Must be at most %d characters long", max)
	}

	return nil
}

// ValidateRequired validates that a value is not empty
func ValidateRequired(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return NewValidationError(field, "%s is required", field)
	}
	return nil
}

// ValidateEnum validates that a value is one of the allowed values
func ValidateEnum(value string, allowed []string, field string) error {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return NewValidationError(field, "%s must be one of: %v", field, allowed)
}
