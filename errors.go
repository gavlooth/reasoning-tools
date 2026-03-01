package main

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// ErrorCategory represents the type of error for handling decisions
type ErrorCategory string

const (
	// ErrorConfig indicates a configuration error (e.g., missing API key, invalid settings)
	// These errors require user intervention and should not be retried
	ErrorConfig ErrorCategory = "config"

	// ErrorTransient indicates a temporary error that may succeed on retry
	// Examples: rate limits, network timeouts, temporary service unavailability
	ErrorTransient ErrorCategory = "transient"

	// ErrorTerminal indicates a permanent error that will not succeed on retry
	// Examples: invalid request, authentication failure, resource not found
	ErrorTerminal ErrorCategory = "terminal"
)

// CategorizedError wraps an error with its category for better handling
type CategorizedError struct {
	Category ErrorCategory
	Message  string
	Cause    error
}

func (e *CategorizedError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Category, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Category, e.Message)
}

func (e *CategorizedError) Unwrap() error {
	return e.Cause
}

// NewConfigError creates a configuration error
func NewConfigError(message string, cause error) *CategorizedError {
	return &CategorizedError{
		Category: ErrorConfig,
		Message:  message,
		Cause:    cause,
	}
}

// NewTransientError creates a transient/retryable error
func NewTransientError(message string, cause error) *CategorizedError {
	return &CategorizedError{
		Category: ErrorTransient,
		Message:  message,
		Cause:    cause,
	}
}

// NewTerminalError creates a terminal/non-retryable error
func NewTerminalError(message string, cause error) *CategorizedError {
	return &CategorizedError{
		Category: ErrorTerminal,
		Message:  message,
		Cause:    cause,
	}
}

// CategorizeError examines an error and returns its category
func CategorizeError(err error) ErrorCategory {
	if err == nil {
		return ""
	}

	// Check if it's already categorized
	var catErr *CategorizedError
	if errors.As(err, &catErr) {
		return catErr.Category
	}

	errStr := err.Error()
	errLower := strings.ToLower(errStr)

	// Configuration errors
	if isConfigError(errLower) {
		return ErrorConfig
	}

	// Transient errors (retryable)
	if isTransientError2(err, errLower) {
		return ErrorTransient
	}

	// Terminal errors (non-retryable)
	if isTerminalError(errLower) {
		return ErrorTerminal
	}

	// Default to terminal for unknown errors
	return ErrorTerminal
}

// isConfigError checks if the error is a configuration issue
func isConfigError(errLower string) bool {
	configPatterns := []string{
		"api key",
		"apikey",
		"api_key",
		"invalid key",
		"missing key",
		"authentication required",
		"not configured",
		"configuration error",
		"invalid configuration",
		"environment variable",
		"invalid model",
		"model not found",
		"invalid provider",
		"unknown provider",
	}

	for _, pattern := range configPatterns {
		if strings.Contains(errLower, pattern) {
			return true
		}
	}
	return false
}

// isTransientError2 checks if the error is temporary and retryable
func isTransientError2(err error, errLower string) bool {
	// Network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	transientPatterns := []string{
		"timeout",
		"timed out",
		"rate limit",
		"rate_limit",
		"ratelimit",
		"too many requests",
		"429",
		"503",
		"502",
		"504",
		"service unavailable",
		"temporarily unavailable",
		"connection reset",
		"connection refused",
		"broken pipe",
		"eof",
		"unexpected eof",
		"server busy",
		"overloaded",
		"try again",
		"retry",
		"temporary",
		"temporarily",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(errLower, pattern) {
			return true
		}
	}
	return false
}

// isTerminalError checks if the error is permanent
func isTerminalError(errLower string) bool {
	terminalPatterns := []string{
		"invalid request",
		"bad request",
		"400",
		"401",
		"403",
		"404",
		"not found",
		"unauthorized",
		"forbidden",
		"invalid json",
		"parse error",
		"validation error",
		"invalid parameter",
		"malformed",
		"content policy",
		"safety",
		"blocked",
	}

	for _, pattern := range terminalPatterns {
		if strings.Contains(errLower, pattern) {
			return true
		}
	}
	return false
}

// IsRetryable returns true if the error category allows retrying
func IsRetryable(err error) bool {
	category := CategorizeError(err)
	return category == ErrorTransient
}

// ShouldNotify returns true if the error should trigger user notification
func ShouldNotify(err error) bool {
	category := CategorizeError(err)
	return category == ErrorConfig || category == ErrorTerminal
}

// ErrorSuggestion provides a helpful suggestion for resolving an error
func ErrorSuggestion(err error) string {
	if err == nil {
		return ""
	}

	category := CategorizeError(err)
	errLower := strings.ToLower(err.Error())

	switch category {
	case ErrorConfig:
		if strings.Contains(errLower, "api key") || strings.Contains(errLower, "apikey") {
			return "Check that your API key is set correctly in the environment variables (e.g., OPENAI_API_KEY, ANTHROPIC_API_KEY)"
		}
		if strings.Contains(errLower, "model") {
			return "Verify the model name is correct and available for your provider"
		}
		if strings.Contains(errLower, "provider") {
			return "Use 'list_providers' tool to see available providers and their configuration"
		}
		return "Check your configuration settings and environment variables"

	case ErrorTransient:
		if strings.Contains(errLower, "rate limit") || strings.Contains(errLower, "429") {
			return "You're being rate limited. Wait a moment and try again, or consider using a different provider"
		}
		if strings.Contains(errLower, "timeout") {
			return "Request timed out. This may be due to high load. Try again or increase timeout settings"
		}
		return "This is a temporary error. Waiting and retrying should help"

	case ErrorTerminal:
		if strings.Contains(errLower, "content policy") || strings.Contains(errLower, "safety") {
			return "Your request was blocked by content safety filters. Rephrase your query"
		}
		if strings.Contains(errLower, "invalid") {
			return "Check that your input is valid and properly formatted"
		}
		return "This error cannot be resolved by retrying. Check your input and configuration"
	}

	return ""
}

// WrapWithCategory wraps an error with the appropriate category
func WrapWithCategory(err error, context string) error {
	if err == nil {
		return nil
	}

	category := CategorizeError(err)
	switch category {
	case ErrorConfig:
		return NewConfigError(context, err)
	case ErrorTransient:
		return NewTransientError(context, err)
	default:
		return NewTerminalError(context, err)
	}
}
