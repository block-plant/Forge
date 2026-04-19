package utils

import (
	"fmt"
	"strings"
	"unicode"
)

// ValidationError represents one or more validation failures.
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed: %s — %s", e.Field, e.Message)
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors []ValidationError

// Error implements the error interface for multiple validation failures.
func (ve ValidationErrors) Error() string {
	if len(ve) == 0 {
		return "no validation errors"
	}
	msgs := make([]string, len(ve))
	for i, e := range ve {
		msgs[i] = fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return "validation failed: " + strings.Join(msgs, "; ")
}

// HasErrors returns true if there are any validation errors.
func (ve ValidationErrors) HasErrors() bool {
	return len(ve) > 0
}

// ValidateRequired checks that a string field is non-empty.
func ValidateRequired(field, value string) *ValidationError {
	if strings.TrimSpace(value) == "" {
		return &ValidationError{Field: field, Message: "is required"}
	}
	return nil
}

// ValidateMinLength checks that a string meets a minimum length.
func ValidateMinLength(field, value string, min int) *ValidationError {
	if len(value) < min {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("must be at least %d characters", min),
		}
	}
	return nil
}

// ValidateMaxLength checks that a string does not exceed a maximum length.
func ValidateMaxLength(field, value string, max int) *ValidationError {
	if len(value) > max {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("must be at most %d characters", max),
		}
	}
	return nil
}

// ValidateEmail performs a basic email format validation.
// Checks for: non-empty, contains exactly one @, has a domain with a dot,
// local part and domain are non-empty.
func ValidateEmail(field, email string) *ValidationError {
	email = strings.TrimSpace(email)
	if email == "" {
		return &ValidationError{Field: field, Message: "email is required"}
	}

	atIndex := strings.LastIndex(email, "@")
	if atIndex < 1 {
		return &ValidationError{Field: field, Message: "invalid email format"}
	}

	local := email[:atIndex]
	domain := email[atIndex+1:]

	if local == "" || domain == "" {
		return &ValidationError{Field: field, Message: "invalid email format"}
	}

	if !strings.Contains(domain, ".") {
		return &ValidationError{Field: field, Message: "invalid email domain"}
	}

	// Check domain doesn't start or end with a dot or hyphen
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") ||
		strings.HasPrefix(domain, "-") || strings.HasSuffix(domain, "-") {
		return &ValidationError{Field: field, Message: "invalid email domain"}
	}

	// Check for consecutive dots in domain
	if strings.Contains(domain, "..") {
		return &ValidationError{Field: field, Message: "invalid email domain"}
	}

	return nil
}

// ValidateURL performs basic URL validation.
// Checks for http:// or https:// prefix and a non-empty host.
func ValidateURL(field, url string) *ValidationError {
	url = strings.TrimSpace(url)
	if url == "" {
		return &ValidationError{Field: field, Message: "URL is required"}
	}

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return &ValidationError{Field: field, Message: "URL must start with http:// or https://"}
	}

	// Extract host part
	withoutScheme := url
	if strings.HasPrefix(url, "https://") {
		withoutScheme = url[8:]
	} else {
		withoutScheme = url[7:]
	}

	if withoutScheme == "" || withoutScheme[0] == '/' {
		return &ValidationError{Field: field, Message: "URL must have a host"}
	}

	return nil
}

// ValidateAlphanumeric checks that a string contains only letters and numbers.
func ValidateAlphanumeric(field, value string) *ValidationError {
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return &ValidationError{
				Field:   field,
				Message: "must contain only letters and numbers",
			}
		}
	}
	return nil
}

// ValidateRange checks that an integer is within a range [min, max].
func ValidateRange(field string, value, min, max int) *ValidationError {
	if value < min || value > max {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("must be between %d and %d", min, max),
		}
	}
	return nil
}

// Sanitize removes leading/trailing whitespace and common injection characters.
// This does NOT replace proper escaping — it's a first-pass safety measure.
func Sanitize(s string) string {
	s = strings.TrimSpace(s)
	// Remove null bytes
	s = strings.ReplaceAll(s, "\x00", "")
	return s
}
