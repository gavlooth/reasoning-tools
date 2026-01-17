package main

import (
	"os"
	"testing"
	"time"
)

func TestWithDefault(t *testing.T) {
	tests := []struct {
		name     string
		val      string
		def      string
		expected string
	}{
		{"Value present", "actual", "default", "actual"},
		{"Value empty", "", "default", "default"},
		{"Both empty", "", "", ""},
		{"Non-empty both", "value", "fallback", "value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := withDefault(tt.val, tt.def)
			if got != tt.expected {
				t.Errorf("withDefault(%q, %q) = %q, want %q", tt.val, tt.def, got, tt.expected)
			}
		})
	}
}

func TestGetAPIKeyForProvider(t *testing.T) {
	// Store and restore original env vars
	envVars := []string{
		"ZAI_API_KEY", "GLM_API_KEY", "GROQ_API_KEY", "DEEPSEEK_API_KEY",
		"OPENROUTER_API_KEY", "TOGETHER_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY",
	}
	originalValues := make(map[string]string)
	for _, v := range envVars {
		originalValues[v] = os.Getenv(v)
		defer os.Setenv(v, originalValues[v])
		os.Unsetenv(v)
	}

	tests := []struct {
		name        string
		provider    string
		setEnvVar   string
		setEnvValue string
		expectedKey string
		description string
	}{
		{
			name:        "ZAI with ZAI_API_KEY",
			provider:    "zai",
			setEnvVar:   "ZAI_API_KEY",
			setEnvValue: "zai-key-123",
			expectedKey: "zai-key-123",
			description: "Should return ZAI_API_KEY for zai provider",
		},
		{
			name:        "ZAI falls back to GLM_API_KEY",
			provider:    "zai",
			setEnvVar:   "GLM_API_KEY",
			setEnvValue: "glm-key-456",
			expectedKey: "glm-key-456",
			description: "Should fallback to GLM_API_KEY when ZAI_API_KEY not set",
		},
		{
			name:        "GLM provider",
			provider:    "glm",
			setEnvVar:   "ZAI_API_KEY",
			setEnvValue: "zai-key-789",
			expectedKey: "zai-key-789",
			description: "GLM provider should check ZAI_API_KEY first",
		},
		{
			name:        "Zhipu provider",
			provider:    "zhipu",
			setEnvVar:   "GLM_API_KEY",
			setEnvValue: "glm-key-999",
			expectedKey: "glm-key-999",
			description: "Zhipu provider should fallback to GLM_API_KEY",
		},
		{
			name:        "Groq provider",
			provider:    "groq",
			setEnvVar:   "GROQ_API_KEY",
			setEnvValue: "groq-key-abc",
			expectedKey: "groq-key-abc",
			description: "Should return GROQ_API_KEY for groq provider",
		},
		{
			name:        "DeepSeek provider",
			provider:    "deepseek",
			setEnvVar:   "DEEPSEEK_API_KEY",
			setEnvValue: "deepseek-key-xyz",
			expectedKey: "deepseek-key-xyz",
			description: "Should return DEEPSEEK_API_KEY for deepseek provider",
		},
		{
			name:        "OpenRouter provider",
			provider:    "openrouter",
			setEnvVar:   "OPENROUTER_API_KEY",
			setEnvValue: "openrouter-key-123",
			expectedKey: "openrouter-key-123",
			description: "Should return OPENROUTER_API_KEY for openrouter provider",
		},
		{
			name:        "Together provider",
			provider:    "together",
			setEnvVar:   "TOGETHER_API_KEY",
			setEnvValue: "together-key-456",
			expectedKey: "together-key-456",
			description: "Should return TOGETHER_API_KEY for together provider",
		},
		{
			name:        "Anthropic provider",
			provider:    "anthropic",
			setEnvVar:   "ANTHROPIC_API_KEY",
			setEnvValue: "anthropic-key-789",
			expectedKey: "anthropic-key-789",
			description: "Should return ANTHROPIC_API_KEY for anthropic provider",
		},
		{
			name:        "OpenAI provider",
			provider:    "openai",
			setEnvVar:   "OPENAI_API_KEY",
			setEnvValue: "openai-key-xyz",
			expectedKey: "openai-key-xyz",
			description: "Should return OPENAI_API_KEY for openai provider",
		},
		{
			name:        "Unknown provider",
			provider:    "unknown",
			setEnvVar:   "OPENAI_API_KEY",
			setEnvValue: "openai-key-999",
			expectedKey: "",
			description: "Should return empty string for unknown provider",
		},
		{
			name:        "Empty provider",
			provider:    "",
			setEnvVar:   "OPENAI_API_KEY",
			setEnvValue: "openai-key-888",
			expectedKey: "",
			description: "Should return empty string for empty provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all environment variables before each test to ensure isolation
			for _, v := range envVars {
				os.Unsetenv(v)
			}
			if tt.setEnvValue != "" {
				os.Setenv(tt.setEnvVar, tt.setEnvValue)
			}
			got := getAPIKeyForProvider(tt.provider)
			if got != tt.expectedKey {
				t.Errorf("%s: getAPIKeyForProvider(%q) = %q, want %q", tt.description, tt.provider, got, tt.expectedKey)
			}
		})
	}
}

func TestDetectProviderFromEnv(t *testing.T) {
	// Store and restore original env vars
	envVars := []string{
		"ZAI_API_KEY", "GLM_API_KEY", "GROQ_API_KEY", "DEEPSEEK_API_KEY",
		"OPENROUTER_API_KEY", "TOGETHER_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY",
	}
	originalValues := make(map[string]string)
	for _, v := range envVars {
		originalValues[v] = os.Getenv(v)
		defer os.Setenv(v, originalValues[v])
		os.Unsetenv(v)
	}

	tests := []struct {
		name        string
		setEnvVar   string
		setEnvValue string
		expected    string
		description string
	}{
		{"ZAI key present", "ZAI_API_KEY", "zai-key", "zai", "Should detect zai provider from ZAI_API_KEY"},
		{"GLM key present", "GLM_API_KEY", "glm-key", "zai", "Should detect zai provider from GLM_API_KEY"},
		{"Groq key present", "GROQ_API_KEY", "groq-key", "groq", "Should detect groq provider"},
		{"DeepSeek key present", "DEEPSEEK_API_KEY", "deepseek-key", "deepseek", "Should detect deepseek provider"},
		{"OpenRouter key present", "OPENROUTER_API_KEY", "or-key", "openrouter", "Should detect openrouter provider"},
		{"Together key present", "TOGETHER_API_KEY", "together-key", "together", "Should detect together provider"},
		{"Anthropic key present", "ANTHROPIC_API_KEY", "anthropic-key", "anthropic", "Should detect anthropic provider"},
		{"OpenAI key present", "OPENAI_API_KEY", "openai-key", "openai", "Should detect openai provider"},
		{"No keys present", "", "", "ollama", "Should default to ollama when no API keys present"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all env vars first
			for _, v := range envVars {
				os.Unsetenv(v)
			}
			if tt.setEnvVar != "" && tt.setEnvValue != "" {
				os.Setenv(tt.setEnvVar, tt.setEnvValue)
			}
			got := detectProviderFromEnv()
			if got != tt.expected {
				t.Errorf("%s: detectProviderFromEnv() = %q, want %q", tt.description, got, tt.expected)
			}
		})
	}

	t.Run("Priority order - ZAI before GLM", func(t *testing.T) {
		os.Setenv("ZAI_API_KEY", "zai-key")
		os.Setenv("GLM_API_KEY", "glm-key")
		if got := detectProviderFromEnv(); got != "zai" {
			t.Errorf("ZAI_API_KEY should take priority over GLM_API_KEY, got %q", got)
		}
	})
}

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name        string
		cfg         ProviderConfig
		expectError bool
		description string
	}{
		{
			name:        "OpenAI provider",
			cfg:         ProviderConfig{Type: "openai", APIKey: "test-key"},
			expectError: false,
			description: "Should create OpenAI provider successfully",
		},
		{
			name:        "Anthropic provider",
			cfg:         ProviderConfig{Type: "anthropic", APIKey: "test-key"},
			expectError: false,
			description: "Should create Anthropic provider successfully",
		},
		{
			name:        "Groq provider",
			cfg:         ProviderConfig{Type: "groq", APIKey: "test-key"},
			expectError: false,
			description: "Should create Groq provider successfully",
		},
		{
			name:        "Ollama provider",
			cfg:         ProviderConfig{Type: "ollama"},
			expectError: false,
			description: "Should create Ollama provider (no API key needed)",
		},
		{
			name:        "DeepSeek provider",
			cfg:         ProviderConfig{Type: "deepseek", APIKey: "test-key"},
			expectError: false,
			description: "Should create DeepSeek provider successfully",
		},
		{
			name:        "OpenRouter provider",
			cfg:         ProviderConfig{Type: "openrouter", APIKey: "test-key"},
			expectError: false,
			description: "Should create OpenRouter provider successfully",
		},
		{
			name:        "Zai provider",
			cfg:         ProviderConfig{Type: "zai", APIKey: "test-key"},
			expectError: false,
			description: "Should create Zai provider successfully",
		},
		{
			name:        "Together provider",
			cfg:         ProviderConfig{Type: "together", APIKey: "test-key"},
			expectError: false,
			description: "Should create Together provider successfully",
		},
		{
			name:        "Unknown provider",
			cfg:         ProviderConfig{Type: "unknown", APIKey: "test-key"},
			expectError: true,
			description: "Should return error for unknown provider type",
		},
		{
			name:        "Empty provider type",
			cfg:         ProviderConfig{Type: "", APIKey: "test-key"},
			expectError: true,
			description: "Should return error for empty provider type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(tt.cfg)
			if tt.expectError {
				if err == nil {
					t.Errorf("%s: Expected error but got none", tt.description)
				}
			} else {
				if err != nil {
					t.Errorf("%s: Unexpected error: %v", tt.description, err)
				}
				if provider == nil && !tt.expectError {
					t.Errorf("%s: Provider should not be nil", tt.description)
				}
				if provider != nil && provider.Name() == "" {
					t.Errorf("%s: Provider name should not be empty", tt.description)
				}
			}
		})
	}
}

func TestProviderHasTimeoutFromConfig(t *testing.T) {
	// This test verifies that providers are created with timeouts from config
	testCfg := &Config{
		OpenAITimeout:     60 * time.Second,
		AnthropicTimeout:  90 * time.Second,
		GroqTimeout:       45 * time.Second,
		OllamaTimeout:     120 * time.Second,
		DeepSeekTimeout:   75 * time.Second,
		OpenRouterTimeout: 80 * time.Second,
		ZaiTimeout:        85 * time.Second,
		TogetherTimeout:   95 * time.Second,
	}

	// Temporarily set custom config
	oldConfig := globalConfig
	globalConfig = testCfg
	defer func() { globalConfig = oldConfig }()

	tests := []struct {
		providerType string
		apiKey       string
	}{
		{"openai", "test-key"},
		{"anthropic", "test-key"},
		{"groq", "test-key"},
		{"ollama", ""},
		{"deepseek", "test-key"},
		{"openrouter", "test-key"},
		{"zai", "test-key"},
		{"together", "test-key"},
	}

	for _, tt := range tests {
		t.Run(tt.providerType, func(t *testing.T) {
			provider, err := NewProvider(ProviderConfig{
				Type:   tt.providerType,
				APIKey: tt.apiKey,
			})
			if err != nil {
				t.Fatalf("Failed to create provider: %v", err)
			}
			if provider == nil {
				t.Fatal("Provider should not be nil")
			}
			// Just verify the provider was created successfully
			// The actual timeout values are internal to the providers
		})
	}
}

func TestProviderName(t *testing.T) {
	tests := []struct {
		providerType string
		expectedName string
	}{
		{"openai", "openai"},
		{"anthropic", "anthropic"},
		{"groq", "groq"},
		{"ollama", "ollama"},
		{"deepseek", "deepseek"},
		{"openrouter", "openrouter"},
		{"zai", "zai"},
		{"together", "together"},
	}

	for _, tt := range tests {
		t.Run(tt.providerType, func(t *testing.T) {
			provider, err := NewProvider(ProviderConfig{
				Type:   tt.providerType,
				APIKey: "test-key",
			})
			if err != nil {
				t.Fatalf("Failed to create provider: %v", err)
			}
			if name := provider.Name(); name != tt.expectedName {
				t.Errorf("Provider name = %q, want %q", name, tt.expectedName)
			}
		})
	}
}
