package main

import (
	"errors"
	"testing"
)

func TestCategorizeError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCategory
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "api key error",
			err:      errors.New("invalid api key provided"),
			expected: ErrorConfig,
		},
		{
			name:     "rate limit error",
			err:      errors.New("rate limit exceeded"),
			expected: ErrorTransient,
		},
		{
			name:     "429 error",
			err:      errors.New("status 429: too many requests"),
			expected: ErrorTransient,
		},
		{
			name:     "timeout error",
			err:      errors.New("request timed out"),
			expected: ErrorTransient,
		},
		{
			name:     "bad request error",
			err:      errors.New("bad request: invalid json"),
			expected: ErrorTerminal,
		},
		{
			name:     "401 unauthorized",
			err:      errors.New("401 unauthorized"),
			expected: ErrorTerminal,
		},
		{
			name:     "503 service unavailable",
			err:      errors.New("503 service unavailable"),
			expected: ErrorTransient,
		},
		{
			name:     "content policy error",
			err:      errors.New("content policy violation"),
			expected: ErrorTerminal,
		},
		{
			name:     "model not found",
			err:      errors.New("model not found"),
			expected: ErrorConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CategorizeError(tt.err)
			if got != tt.expected {
				t.Errorf("CategorizeError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "rate limit",
			err:      NewTransientError("rate limited", nil),
			expected: true,
		},
		{
			name:     "timeout",
			err:      errors.New("timeout occurred"),
			expected: true,
		},
		{
			name:     "config error",
			err:      NewConfigError("missing api key", nil),
			expected: false,
		},
		{
			name:     "terminal error",
			err:      NewTerminalError("invalid request", nil),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.err)
			if got != tt.expected {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestErrorSuggestion(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains string
	}{
		{
			name:     "api key error",
			err:      errors.New("invalid api key"),
			contains: "API key",
		},
		{
			name:     "rate limit",
			err:      errors.New("rate limit exceeded 429"),
			contains: "rate limited",
		},
		{
			name:     "timeout",
			err:      errors.New("request timed out"),
			contains: "timed out",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestion := ErrorSuggestion(tt.err)
			if suggestion == "" {
				t.Errorf("ErrorSuggestion(%v) returned empty string", tt.err)
			}
		})
	}
}
