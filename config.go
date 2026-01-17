package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"
)

// Config holds application-wide configuration
type Config struct {
	// HTTP timeouts for various providers
	OpenAITimeout     time.Duration
	AnthropicTimeout  time.Duration
	GroqTimeout       time.Duration
	OllamaTimeout     time.Duration
	DeepSeekTimeout   time.Duration
	OpenRouterTimeout time.Duration
	ZaiTimeout        time.Duration
	TogetherTimeout   time.Duration

	// Tool timeouts
	CodeExecTimeout time.Duration
	WebFetchTimeout time.Duration
}

// Validation bounds for timeout values
const (
	// Minimum timeout: 1 second (no timeouts shorter than this make sense)
	minTimeout = 1 * time.Second
	// Maximum timeout: 1 hour (prevents resource exhaustion and hangs)
	maxTimeout = 1 * time.Hour
	// Maximum tool timeout: 5 minutes (tools should complete quickly)
	maxToolTimeout = 5 * time.Minute
)

// DefaultConfig returns default configuration values
func DefaultConfig() *Config {
	return &Config{
		OpenAITimeout:     120 * time.Second,
		AnthropicTimeout:  120 * time.Second,
		GroqTimeout:       120 * time.Second,
		OllamaTimeout:     300 * time.Second,
		DeepSeekTimeout:   120 * time.Second,
		OpenRouterTimeout: 120 * time.Second,
		ZaiTimeout:        120 * time.Second,
		TogetherTimeout:   120 * time.Second,
		CodeExecTimeout:   10 * time.Second,
		WebFetchTimeout:   15 * time.Second,
	}
}

// Validate checks that all timeout values are within reasonable bounds.
// Returns an error if any timeout is out of bounds, nil otherwise.
func (c *Config) Validate() error {
	// Validate provider timeouts (all use the same maxTimeout bound)
	providerTimeouts := []struct {
		name    string
		timeout time.Duration
	}{
		{"OpenAITimeout", c.OpenAITimeout},
		{"AnthropicTimeout", c.AnthropicTimeout},
		{"GroqTimeout", c.GroqTimeout},
		{"OllamaTimeout", c.OllamaTimeout},
		{"DeepSeekTimeout", c.DeepSeekTimeout},
		{"OpenRouterTimeout", c.OpenRouterTimeout},
		{"ZaiTimeout", c.ZaiTimeout},
		{"TogetherTimeout", c.TogetherTimeout},
	}

	for _, pt := range providerTimeouts {
		if pt.timeout < minTimeout {
			return fmt.Errorf("provider timeout %s (%v) is below minimum (%v)", pt.name, pt.timeout, minTimeout)
		}
		if pt.timeout > maxTimeout {
			return fmt.Errorf("provider timeout %s (%v) exceeds maximum (%v)", pt.name, pt.timeout, maxTimeout)
		}
	}

	// Validate tool timeouts (use stricter maxToolTimeout bound)
	toolTimeouts := []struct {
		name    string
		timeout time.Duration
	}{
		{"CodeExecTimeout", c.CodeExecTimeout},
		{"WebFetchTimeout", c.WebFetchTimeout},
	}

	for _, tt := range toolTimeouts {
		if tt.timeout < minTimeout {
			return fmt.Errorf("tool timeout %s (%v) is below minimum (%v)", tt.name, tt.timeout, minTimeout)
		}
		if tt.timeout > maxToolTimeout {
			return fmt.Errorf("tool timeout %s (%v) exceeds maximum (%v)", tt.name, tt.timeout, maxToolTimeout)
		}
	}

	return nil
}

// LoadConfig loads configuration from environment variables
// Falls back to defaults if not set. Invalid values are clamped to safe ranges.
func LoadConfig() *Config {
	cfg := DefaultConfig()

	// clampDuration ensures a duration is within [min, max] and logs if clamped
	clampDuration := func(name string, value, minVal, maxVal time.Duration) time.Duration {
		if value < minVal {
			log.Printf("[CONFIG] %s (%v) below minimum (%v), clamping to minimum", name, value, minVal)
			return minVal
		}
		if value > maxVal {
			log.Printf("[CONFIG] %s (%v) exceeds maximum (%v), clamping to maximum", name, value, maxVal)
			return maxVal
		}
		return value
	}

	// Provider timeouts (in seconds)
	if v := os.Getenv("OPENAI_TIMEOUT"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			cfg.OpenAITimeout = clampDuration("OPENAI_TIMEOUT", time.Duration(s)*time.Second, minTimeout, maxTimeout)
		}
	}
	if v := os.Getenv("ANTHROPIC_TIMEOUT"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			cfg.AnthropicTimeout = clampDuration("ANTHROPIC_TIMEOUT", time.Duration(s)*time.Second, minTimeout, maxTimeout)
		}
	}
	if v := os.Getenv("GROQ_TIMEOUT"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			cfg.GroqTimeout = clampDuration("GROQ_TIMEOUT", time.Duration(s)*time.Second, minTimeout, maxTimeout)
		}
	}
	if v := os.Getenv("OLLAMA_TIMEOUT"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			cfg.OllamaTimeout = clampDuration("OLLAMA_TIMEOUT", time.Duration(s)*time.Second, minTimeout, maxTimeout)
		}
	}
	if v := os.Getenv("DEEPSEEK_TIMEOUT"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			cfg.DeepSeekTimeout = clampDuration("DEEPSEEK_TIMEOUT", time.Duration(s)*time.Second, minTimeout, maxTimeout)
		}
	}
	if v := os.Getenv("OPENROUTER_TIMEOUT"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			cfg.OpenRouterTimeout = clampDuration("OPENROUTER_TIMEOUT", time.Duration(s)*time.Second, minTimeout, maxTimeout)
		}
	}
	if v := os.Getenv("ZAI_TIMEOUT"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			cfg.ZaiTimeout = clampDuration("ZAI_TIMEOUT", time.Duration(s)*time.Second, minTimeout, maxTimeout)
		}
	}
	if v := os.Getenv("TOGETHER_TIMEOUT"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			cfg.TogetherTimeout = clampDuration("TOGETHER_TIMEOUT", time.Duration(s)*time.Second, minTimeout, maxTimeout)
		}
	}

	// Tool timeouts (use stricter maxToolTimeout bound)
	if v := os.Getenv("CODE_EXEC_TIMEOUT"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			cfg.CodeExecTimeout = clampDuration("CODE_EXEC_TIMEOUT", time.Duration(s)*time.Second, minTimeout, maxToolTimeout)
		}
	}
	if v := os.Getenv("WEB_FETCH_TIMEOUT"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			cfg.WebFetchTimeout = clampDuration("WEB_FETCH_TIMEOUT", time.Duration(s)*time.Second, minTimeout, maxToolTimeout)
		}
	}

	return cfg
}

// Global config instance (initialized lazily)
var (
	globalConfig *Config
	configOnce   sync.Once
)

// GetConfig returns the global configuration, initializing it lazily on first access.
// This allows tests to set environment variables before the first call to GetConfig(),
// resolving the initialization order dependency issue.
func GetConfig() *Config {
	configOnce.Do(func() {
		globalConfig = LoadConfig()
	})
	return globalConfig
}

// ResetConfig resets the global configuration for testing purposes.
// This allows tests to set different environment variables and reload configuration.
// NOTE: This function is not thread-safe and should only be used in tests.
func ResetConfig() {
	globalConfig = nil
	configOnce = sync.Once{}
}
