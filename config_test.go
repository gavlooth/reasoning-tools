package main

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.OpenAITimeout != 120*time.Second {
		t.Errorf("Expected OpenAITimeout to be 120s, got %v", cfg.OpenAITimeout)
	}
	if cfg.AnthropicTimeout != 120*time.Second {
		t.Errorf("Expected AnthropicTimeout to be 120s, got %v", cfg.AnthropicTimeout)
	}
	if cfg.GroqTimeout != 120*time.Second {
		t.Errorf("Expected GroqTimeout to be 120s, got %v", cfg.GroqTimeout)
	}
	if cfg.OllamaTimeout != 300*time.Second {
		t.Errorf("Expected OllamaTimeout to be 300s, got %v", cfg.OllamaTimeout)
	}
	if cfg.DeepSeekTimeout != 120*time.Second {
		t.Errorf("Expected DeepSeekTimeout to be 120s, got %v", cfg.DeepSeekTimeout)
	}
	if cfg.OpenRouterTimeout != 120*time.Second {
		t.Errorf("Expected OpenRouterTimeout to be 120s, got %v", cfg.OpenRouterTimeout)
	}
	if cfg.ZaiTimeout != 120*time.Second {
		t.Errorf("Expected ZaiTimeout to be 120s, got %v", cfg.ZaiTimeout)
	}
	if cfg.TogetherTimeout != 120*time.Second {
		t.Errorf("Expected TogetherTimeout to be 120s, got %v", cfg.TogetherTimeout)
	}
	if cfg.CodeExecTimeout != 10*time.Second {
		t.Errorf("Expected CodeExecTimeout to be 10s, got %v", cfg.CodeExecTimeout)
	}
	if cfg.WebFetchTimeout != 15*time.Second {
		t.Errorf("Expected WebFetchTimeout to be 15s, got %v", cfg.WebFetchTimeout)
	}
}

func TestLoadConfigWithDefaults(t *testing.T) {
	// Clear all relevant env vars
	envVars := []string{
		"OPENAI_TIMEOUT", "ANTHROPIC_TIMEOUT", "GROQ_TIMEOUT", "OLLAMA_TIMEOUT",
		"DEEPSEEK_TIMEOUT", "OPENROUTER_TIMEOUT", "ZAI_TIMEOUT", "TOGETHER_TIMEOUT",
		"CODE_EXEC_TIMEOUT", "WEB_FETCH_TIMEOUT",
	}
	for _, v := range envVars {
		os.Unsetenv(v)
	}

	cfg := LoadConfig()

	// Should match default values
	if cfg.OpenAITimeout != 120*time.Second {
		t.Errorf("Expected default OpenAITimeout to be 120s, got %v", cfg.OpenAITimeout)
	}
}

func TestLoadConfigWithCustomValues(t *testing.T) {
	tests := []struct {
		name     string
		envVar   string
		value    string
		expected time.Duration
		field    string
	}{
		{"OpenAI custom timeout", "OPENAI_TIMEOUT", "60", 60 * time.Second, "OpenAITimeout"},
		{"Anthropic custom timeout", "ANTHROPIC_TIMEOUT", "90", 90 * time.Second, "AnthropicTimeout"},
		{"Groq custom timeout", "GROQ_TIMEOUT", "45", 45 * time.Second, "GroqTimeout"},
		{"Ollama custom timeout", "OLLAMA_TIMEOUT", "600", 600 * time.Second, "OllamaTimeout"},
		{"DeepSeek custom timeout", "DEEPSEEK_TIMEOUT", "30", 30 * time.Second, "DeepSeekTimeout"},
		{"OpenRouter custom timeout", "OPENROUTER_TIMEOUT", "60", 60 * time.Second, "OpenRouterTimeout"},
		{"Zai custom timeout", "ZAI_TIMEOUT", "180", 180 * time.Second, "ZaiTimeout"},
		{"Together custom timeout", "TOGETHER_TIMEOUT", "90", 90 * time.Second, "TogetherTimeout"},
		{"CodeExec custom timeout", "CODE_EXEC_TIMEOUT", "20", 20 * time.Second, "CodeExecTimeout"},
		{"WebFetch custom timeout", "WEB_FETCH_TIMEOUT", "30", 30 * time.Second, "WebFetchTimeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up env var after test
			originalValue := os.Getenv(tt.envVar)
			if originalValue != "" {
				defer os.Setenv(tt.envVar, originalValue)
			} else {
				defer os.Unsetenv(tt.envVar)
			}

			os.Setenv(tt.envVar, tt.value)
			cfg := LoadConfig()

			var got time.Duration
			switch tt.field {
			case "OpenAITimeout":
				got = cfg.OpenAITimeout
			case "AnthropicTimeout":
				got = cfg.AnthropicTimeout
			case "GroqTimeout":
				got = cfg.GroqTimeout
			case "OllamaTimeout":
				got = cfg.OllamaTimeout
			case "DeepSeekTimeout":
				got = cfg.DeepSeekTimeout
			case "OpenRouterTimeout":
				got = cfg.OpenRouterTimeout
			case "ZaiTimeout":
				got = cfg.ZaiTimeout
			case "TogetherTimeout":
				got = cfg.TogetherTimeout
			case "CodeExecTimeout":
				got = cfg.CodeExecTimeout
			case "WebFetchTimeout":
				got = cfg.WebFetchTimeout
			}

			if got != tt.expected {
				t.Errorf("Expected %s to be %v, got %v", tt.field, tt.expected, got)
			}
		})
	}
}

func TestLoadConfigWithInvalidValues(t *testing.T) {
	tests := []struct {
		name      string
		envVar    string
		value     string
		field     string
		wantPanic bool
	}{
		{"Negative timeout", "OPENAI_TIMEOUT", "-10", "OpenAITimeout", false},
		{"Zero timeout", "GROQ_TIMEOUT", "0", "GroqTimeout", false},
		{"Non-numeric timeout", "ANTHROPIC_TIMEOUT", "invalid", "AnthropicTimeout", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up env var after test
			originalValue := os.Getenv(tt.envVar)
			if originalValue != "" {
				defer os.Setenv(tt.envVar, originalValue)
			} else {
				defer os.Unsetenv(tt.envVar)
			}

			// Should not panic
			defer func() {
				if r := recover(); r != nil {
					if !tt.wantPanic {
						t.Errorf("LoadConfig panicked unexpectedly: %v", r)
					}
				}
			}()

			os.Setenv(tt.envVar, tt.value)
			cfg := LoadConfig()

			// Invalid values (negative or non-numeric) should fall back to defaults
			// Zero values are ignored (s > 0 check in LoadConfig)
			var got time.Duration
			var wantDefault time.Duration

			switch tt.field {
			case "OpenAITimeout":
				got = cfg.OpenAITimeout
				wantDefault = 120 * time.Second
			case "GroqTimeout":
				got = cfg.GroqTimeout
				wantDefault = 120 * time.Second
			case "AnthropicTimeout":
				got = cfg.AnthropicTimeout
				wantDefault = 120 * time.Second
			}

			if got != wantDefault {
				t.Errorf("Expected %s to fall back to default %v, got %v", tt.field, wantDefault, got)
			}
		})
	}
}

func TestLoadConfigClampsOutOfRangeValues(t *testing.T) {
	tests := []struct {
		name              string
		envVar            string
		value             string
		field             string
		expectedClamped   time.Duration
	}{
		// Below minimum (1 second)
		{"Below minimum", "OPENAI_TIMEOUT", "0", "OpenAITimeout", 120 * time.Second}, // Falls back to default (s > 0 check)
		// Above maximum for provider timeouts (1 hour)
		{"Above maximum provider", "OPENAI_TIMEOUT", "7200", "OpenAITimeout", maxTimeout},
		// Above maximum for tool timeouts (5 minutes)
		{"Above maximum tool", "CODE_EXEC_TIMEOUT", "600", "CodeExecTimeout", maxToolTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up env var after test
			originalValue := os.Getenv(tt.envVar)
			if originalValue != "" {
				defer os.Setenv(tt.envVar, originalValue)
			} else {
				defer os.Unsetenv(tt.envVar)
			}

			os.Setenv(tt.envVar, tt.value)
			cfg := LoadConfig()

			var got time.Duration
			switch tt.field {
			case "OpenAITimeout":
				got = cfg.OpenAITimeout
			case "CodeExecTimeout":
				got = cfg.CodeExecTimeout
			}

			if got != tt.expectedClamped {
				t.Errorf("Expected %s to be clamped to %v, got %v", tt.field, tt.expectedClamped, got)
			}
		})
	}
}

func TestGetConfig(t *testing.T) {
	cfg := GetConfig()
	if cfg == nil {
		t.Error("GetConfig returned nil")
	}
	if cfg.OpenAITimeout == 0 {
		t.Error("GetConfig returned config with uninitialized values")
	}
}

// TestGetConfigLazyInitialization tests that GetConfig uses lazy initialization,
// allowing environment variables to be set before the first call
func TestGetConfigLazyInitialization(t *testing.T) {
	// Reset config to ensure clean state
	ResetConfig()

	// Set environment variable before first GetConfig call
	os.Setenv("OPENAI_TIMEOUT", "75")
	defer os.Unsetenv("OPENAI_TIMEOUT")

	cfg := GetConfig()

	// Should load the environment variable set before the call
	if cfg.OpenAITimeout != 75*time.Second {
		t.Errorf("Expected OpenAITimeout to be 75s (from env var set before GetConfig), got %v", cfg.OpenAITimeout)
	}
}

// TestGetConfigSingleton tests that GetConfig returns the same instance on multiple calls
func TestGetConfigSingleton(t *testing.T) {
	ResetConfig()

	cfg1 := GetConfig()
	cfg2 := GetConfig()

	if cfg1 != cfg2 {
		t.Error("GetConfig should return the same instance (singleton pattern)")
	}
}

func TestConfigValidate_ValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("DefaultConfig should be valid, got error: %v", err)
	}
}

func TestConfigValidate_ProviderTimeoutBelowMinimum(t *testing.T) {
	cfg := DefaultConfig()
	cfg.OpenAITimeout = 100 * time.Millisecond // Below minTimeout (1s)

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for timeout below minimum, got nil")
	}
}

func TestConfigValidate_ProviderTimeoutAboveMaximum(t *testing.T) {
	cfg := DefaultConfig()
	cfg.OpenAITimeout = 2 * time.Hour // Above maxTimeout (1h)

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for provider timeout above maximum, got nil")
	}
}

func TestConfigValidate_ToolTimeoutAboveMaximum(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CodeExecTimeout = 10 * time.Minute // Above maxToolTimeout (5m)

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for tool timeout above maximum, got nil")
	}
}

func TestConfigValidate_AllProviderTimeouts(t *testing.T) {
	cfg := DefaultConfig()

	// Test that each provider timeout is validated
	providerFields := []string{
		"OpenAITimeout", "AnthropicTimeout", "GroqTimeout", "OllamaTimeout",
		"DeepSeekTimeout", "OpenRouterTimeout", "ZaiTimeout", "TogetherTimeout",
	}

	for _, field := range providerFields {
		t.Run(field+" exceeds max", func(t *testing.T) {
			testCfg := *cfg // Copy default config
			switch field {
			case "OpenAITimeout":
				testCfg.OpenAITimeout = 2 * time.Hour
			case "AnthropicTimeout":
				testCfg.AnthropicTimeout = 2 * time.Hour
			case "GroqTimeout":
				testCfg.GroqTimeout = 2 * time.Hour
			case "OllamaTimeout":
				testCfg.OllamaTimeout = 2 * time.Hour
			case "DeepSeekTimeout":
				testCfg.DeepSeekTimeout = 2 * time.Hour
			case "OpenRouterTimeout":
				testCfg.OpenRouterTimeout = 2 * time.Hour
			case "ZaiTimeout":
				testCfg.ZaiTimeout = 2 * time.Hour
			case "TogetherTimeout":
				testCfg.TogetherTimeout = 2 * time.Hour
			}

			err := testCfg.Validate()
			if err == nil {
				t.Errorf("Expected validation error for %s exceeding maximum, got nil", field)
			}
		})
	}
}

func TestConfigValidate_AllToolTimeouts(t *testing.T) {
	cfg := DefaultConfig()

	toolFields := []string{
		"CodeExecTimeout", "WebFetchTimeout",
	}

	for _, field := range toolFields {
		t.Run(field+" exceeds max", func(t *testing.T) {
			testCfg := *cfg // Copy default config
			switch field {
			case "CodeExecTimeout":
				testCfg.CodeExecTimeout = 10 * time.Minute
			case "WebFetchTimeout":
				testCfg.WebFetchTimeout = 10 * time.Minute
			}

			err := testCfg.Validate()
			if err == nil {
				t.Errorf("Expected validation error for %s exceeding maximum, got nil", field)
			}
		})
	}
}

func TestConfigValidate_BoundaryValues(t *testing.T) {
	tests := []struct {
		name      string
		timeout   time.Duration
		wantError bool
		field     string
	}{
		{"Exactly minTimeout", minTimeout, false, "OpenAITimeout"},
		{"Exactly maxTimeout", maxTimeout, false, "OpenAITimeout"},
		{"Exactly maxToolTimeout", maxToolTimeout, false, "CodeExecTimeout"},
		{"Just below minTimeout", minTimeout - 1, true, "OpenAITimeout"},
		{"Just above maxTimeout", maxTimeout + 1, true, "OpenAITimeout"},
		{"Just above maxToolTimeout", maxToolTimeout + 1, true, "CodeExecTimeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			switch tt.field {
			case "OpenAITimeout":
				cfg.OpenAITimeout = tt.timeout
			case "CodeExecTimeout":
				cfg.CodeExecTimeout = tt.timeout
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}
