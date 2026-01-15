package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ChatMessage represents a message in the chat
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Provider interface for LLM backends
type Provider interface {
	Name() string
	Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (string, error)
}

// ChatOptions for provider calls
type ChatOptions struct {
	Temperature float64
	MaxTokens   int
	Model       string
}

// ProviderConfig holds provider configuration
type ProviderConfig struct {
	Type    string // openai, anthropic, groq, ollama, deepseek, openrouter, zai
	APIKey  string
	BaseURL string
	Model   string
}

// NewProvider creates a provider from config
func NewProvider(cfg ProviderConfig) (Provider, error) {
	switch strings.ToLower(cfg.Type) {
	case "openai":
		return &OpenAIProvider{
			apiKey:  cfg.APIKey,
			baseURL: withDefault(cfg.BaseURL, "https://api.openai.com/v1"),
			model:   withDefault(cfg.Model, "gpt-4o-mini"),
			client:  &http.Client{Timeout: 120 * time.Second},
		}, nil
	case "anthropic":
		return &AnthropicProvider{
			apiKey:  cfg.APIKey,
			baseURL: withDefault(cfg.BaseURL, "https://api.anthropic.com/v1"),
			model:   withDefault(cfg.Model, "claude-3-haiku-20240307"),
			client:  &http.Client{Timeout: 120 * time.Second},
		}, nil
	case "groq":
		return &OpenAIProvider{
			apiKey:  cfg.APIKey,
			baseURL: withDefault(cfg.BaseURL, "https://api.groq.com/openai/v1"),
			model:   withDefault(cfg.Model, "llama-3.1-70b-versatile"),
			client:  &http.Client{Timeout: 120 * time.Second},
			name:    "groq",
		}, nil
	case "ollama":
		return &OllamaProvider{
			baseURL: withDefault(cfg.BaseURL, "http://localhost:11434"),
			model:   withDefault(cfg.Model, "llama3.1"),
			client:  &http.Client{Timeout: 300 * time.Second},
		}, nil
	case "deepseek":
		return &OpenAIProvider{
			apiKey:  cfg.APIKey,
			baseURL: withDefault(cfg.BaseURL, "https://api.deepseek.com/v1"),
			model:   withDefault(cfg.Model, "deepseek-chat"),
			client:  &http.Client{Timeout: 120 * time.Second},
			name:    "deepseek",
		}, nil
	case "openrouter":
		return &OpenAIProvider{
			apiKey:  cfg.APIKey,
			baseURL: withDefault(cfg.BaseURL, "https://openrouter.ai/api/v1"),
			model:   withDefault(cfg.Model, "meta-llama/llama-3.1-70b-instruct"),
			client:  &http.Client{Timeout: 120 * time.Second},
			name:    "openrouter",
			headers: map[string]string{
				"HTTP-Referer": "https://github.com/glm-sequential-thinking",
				"X-Title":      "GLM Sequential Thinking",
			},
		}, nil
	case "zai", "glm", "zhipu":
		return &OpenAIProvider{
			apiKey:  cfg.APIKey,
			baseURL: withDefault(cfg.BaseURL, "https://open.bigmodel.cn/api/paas/v4"),
			model:   withDefault(cfg.Model, "glm-4"),
			client:  &http.Client{Timeout: 120 * time.Second},
			name:    "zai",
		}, nil
	case "together":
		return &OpenAIProvider{
			apiKey:  cfg.APIKey,
			baseURL: withDefault(cfg.BaseURL, "https://api.together.xyz/v1"),
			model:   withDefault(cfg.Model, "meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo"),
			client:  &http.Client{Timeout: 120 * time.Second},
			name:    "together",
		}, nil
	default:
		return nil, fmt.Errorf("unknown provider type: %s", cfg.Type)
	}
}

// NewProviderFromEnv creates provider from environment variables
func NewProviderFromEnv() (Provider, error) {
	providerType := os.Getenv("LLM_PROVIDER")
	if providerType == "" {
		// Auto-detect based on available API keys
		providerType = detectProviderFromEnv()
	}

	cfg := ProviderConfig{
		Type:    providerType,
		APIKey:  getAPIKeyForProvider(providerType),
		BaseURL: os.Getenv("LLM_BASE_URL"),
		Model:   os.Getenv("LLM_MODEL"),
	}

	return NewProvider(cfg)
}

func detectProviderFromEnv() string {
	checks := []struct {
		envKey   string
		provider string
	}{
		{"ZAI_API_KEY", "zai"},
		{"GLM_API_KEY", "zai"},
		{"GROQ_API_KEY", "groq"},
		{"DEEPSEEK_API_KEY", "deepseek"},
		{"OPENROUTER_API_KEY", "openrouter"},
		{"TOGETHER_API_KEY", "together"},
		{"ANTHROPIC_API_KEY", "anthropic"},
		{"OPENAI_API_KEY", "openai"},
	}

	for _, c := range checks {
		if os.Getenv(c.envKey) != "" {
			return c.provider
		}
	}

	// Default to ollama if no API keys found
	return "ollama"
}

func getAPIKeyForProvider(provider string) string {
	switch provider {
	case "zai", "glm", "zhipu":
		if key := os.Getenv("ZAI_API_KEY"); key != "" {
			return key
		}
		return os.Getenv("GLM_API_KEY")
	case "groq":
		return os.Getenv("GROQ_API_KEY")
	case "deepseek":
		return os.Getenv("DEEPSEEK_API_KEY")
	case "openrouter":
		return os.Getenv("OPENROUTER_API_KEY")
	case "together":
		return os.Getenv("TOGETHER_API_KEY")
	case "anthropic":
		return os.Getenv("ANTHROPIC_API_KEY")
	case "openai":
		return os.Getenv("OPENAI_API_KEY")
	default:
		return ""
	}
}

func withDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

// ============ OpenAI-Compatible Provider ============

type OpenAIProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
	name    string
	headers map[string]string
}

func (p *OpenAIProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return "openai"
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (string, error) {
	model := opts.Model
	if model == "" {
		model = p.model
	}

	reqBody := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}
	if opts.Temperature > 0 {
		reqBody["temperature"] = opts.Temperature
	}
	if opts.MaxTokens > 0 {
		reqBody["max_tokens"] = opts.MaxTokens
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	for k, v := range p.headers {
		req.Header.Set(k, v)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// ============ Anthropic Provider ============

type AnthropicProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (string, error) {
	model := opts.Model
	if model == "" {
		model = p.model
	}

	// Convert messages to Anthropic format
	var systemPrompt string
	var anthropicMessages []map[string]string

	for _, msg := range messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content
		} else {
			anthropicMessages = append(anthropicMessages, map[string]string{
				"role":    msg.Role,
				"content": msg.Content,
			})
		}
	}

	reqBody := map[string]interface{}{
		"model":      model,
		"messages":   anthropicMessages,
		"max_tokens": withDefaultInt(opts.MaxTokens, 2048),
	}
	if systemPrompt != "" {
		reqBody["system"] = systemPrompt
	}
	if opts.Temperature > 0 {
		reqBody["temperature"] = opts.Temperature
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var chatResp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Content) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return chatResp.Content[0].Text, nil
}

// ============ Ollama Provider ============

type OllamaProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

func (p *OllamaProvider) Name() string {
	return "ollama"
}

func (p *OllamaProvider) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (string, error) {
	model := opts.Model
	if model == "" {
		model = p.model
	}

	reqBody := map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   false,
	}
	if opts.Temperature > 0 {
		reqBody["options"] = map[string]interface{}{
			"temperature": opts.Temperature,
		}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var chatResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return chatResp.Message.Content, nil
}

func withDefaultInt(val, def int) int {
	if val == 0 {
		return def
	}
	return val
}
