package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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

// TokenCallback is called for each token during streaming
type TokenCallback func(token string)

// StreamingProvider extends Provider with streaming capabilities
type StreamingProvider interface {
	Provider
	ChatStream(ctx context.Context, messages []ChatMessage, opts ChatOptions, onToken TokenCallback) (string, error)
	SupportsStreaming() bool
}

// ChatOptions for provider calls
type ChatOptions struct {
	Temperature float64
	MaxTokens   int
	Model       string
}

func normalizeChatOptions(opts ChatOptions) ChatOptions {
	if opts.Temperature != 0 {
		opts.Temperature = clampTemperature(opts.Temperature)
	}
	if opts.MaxTokens != 0 {
		opts.MaxTokens = clampMaxTokens(opts.MaxTokens)
	}
	return opts
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
	// Get global configuration for timeouts
	config := GetConfig()

	switch strings.ToLower(cfg.Type) {
	case "openai":
		return &OpenAIProvider{
			apiKey:  cfg.APIKey,
			baseURL: withDefault(cfg.BaseURL, "https://api.openai.com/v1"),
			model:   withDefault(cfg.Model, "gpt-4o-mini"),
			client:  &http.Client{Timeout: config.OpenAITimeout},
		}, nil
	case "anthropic":
		return &AnthropicProvider{
			apiKey:  cfg.APIKey,
			baseURL: withDefault(cfg.BaseURL, "https://api.anthropic.com/v1"),
			model:   withDefault(cfg.Model, "claude-3-haiku-20240307"),
			client:  &http.Client{Timeout: config.AnthropicTimeout},
		}, nil
	case "groq":
		return &OpenAIProvider{
			apiKey:  cfg.APIKey,
			baseURL: withDefault(cfg.BaseURL, "https://api.groq.com/openai/v1"),
			model:   withDefault(cfg.Model, "llama-3.1-70b-versatile"),
			client:  &http.Client{Timeout: config.GroqTimeout},
			name:    "groq",
		}, nil
	case "ollama":
		return &OllamaProvider{
			baseURL: withDefault(cfg.BaseURL, "http://localhost:11434"),
			model:   withDefault(cfg.Model, "llama3.1"),
			client:  &http.Client{Timeout: config.OllamaTimeout},
		}, nil
	case "deepseek":
		return &OpenAIProvider{
			apiKey:  cfg.APIKey,
			baseURL: withDefault(cfg.BaseURL, "https://api.deepseek.com/v1"),
			model:   withDefault(cfg.Model, "deepseek-chat"),
			client:  &http.Client{Timeout: config.DeepSeekTimeout},
			name:    "deepseek",
		}, nil
	case "openrouter":
		return &OpenAIProvider{
			apiKey:  cfg.APIKey,
			baseURL: withDefault(cfg.BaseURL, "https://openrouter.ai/api/v1"),
			model:   withDefault(cfg.Model, "meta-llama/llama-3.1-70b-instruct"),
			client:  &http.Client{Timeout: config.OpenRouterTimeout},
			name:    "openrouter",
			headers: map[string]string{
				"HTTP-Referer": "https://github.com/gavlooth/reasoning-tools",
				"X-Title":      "GLM Sequential Thinking",
			},
		}, nil
	case "zai", "glm", "zhipu":
		transport := &http.Transport{
			ForceAttemptHTTP2: true,
		}
		return &OpenAIProvider{
			apiKey:  cfg.APIKey,
			baseURL: withDefault(cfg.BaseURL, "https://api.z.ai/api/paas/v4"),
			model:   withDefault(cfg.Model, "glm-4.7"),
			client: &http.Client{
				Timeout:   config.ZaiTimeout,
				Transport: transport,
			},
			name: "zai",
		}, nil
	case "together":
		return &OpenAIProvider{
			apiKey:  cfg.APIKey,
			baseURL: withDefault(cfg.BaseURL, "https://api.together.xyz/v1"),
			model:   withDefault(cfg.Model, "meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo"),
			client:  &http.Client{Timeout: config.TogetherTimeout},
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
	release, err := AcquireLLMSlot(ctx)
	if err != nil {
		return "", err
	}
	if release != nil {
		defer release()
	}

	opts = normalizeChatOptions(opts)
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

	var lastErr error
	retryCfg := defaultRetryConfig()
	rateLimitHits := 0

	for attempt := 0; attempt < retryCfg.maxAttempts+retryCfg.rateLimitAttempts; attempt++ {
		if attempt > 0 {
			// Use exponential backoff
			delay := calculateBackoff(attempt-1, retryCfg)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}

		// Check if we've exceeded normal retries and only rate limit retries remain
		if attempt >= retryCfg.maxAttempts && rateLimitHits == 0 {
			break // No more retries unless we hit rate limits
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
			lastErr = err
			// Retry on connection reset or timeout
			if isTransientError(err) {
				continue
			}
			return "", fmt.Errorf("request failed: %w", err)
		}
		// Ensure response body is closed on all code paths (defer handles return statements,
		// but continue statements need explicit close to release resources before next iteration)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = err
			// Close response body before retrying
			resp.Body.Close()
			continue
		}

		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusTooManyRequests {
				rateLimitHits++
				lastErr = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
				// Close response body before retrying
				resp.Body.Close()
				continue
			}
			if resp.StatusCode >= 500 {
				lastErr = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
				// Close response body before retrying
				resp.Body.Close()
				continue
			}
			return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
		}

		var chatResp struct {
			Choices []struct {
				Message struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content,omitempty"` // Some models (e.g., z.ai) use this field
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
			// Include full response details in error for better debugging
			var responseSnippet string
			if len(body) > 500 {
				responseSnippet = string(body)[:500] + "..."
			} else {
				responseSnippet = string(body)
			}
			return "", fmt.Errorf("no choices in API response (provider: %s, model: %s, status: %d, body: %s)",
				p.Name(), model, resp.StatusCode, responseSnippet)
		}

		content := chatResp.Choices[0].Message.Content
		if content == "" {
			content = chatResp.Choices[0].Message.ReasoningContent
		}
		if content == "" {
			// Empty response - log full body for debugging
			var responseSnippet string
			if len(body) > 500 {
				responseSnippet = string(body)[:500] + "..."
			} else {
				responseSnippet = string(body)
			}
			return "", fmt.Errorf("empty content in API response (provider: %s, model: %s, status: %d, body: %s)",
				p.Name(), model, resp.StatusCode, responseSnippet)
		}

		return content, nil
	}

	// Note: response body is already closed above on all paths
	return "", fmt.Errorf("request failed after retries: %w", lastErr)
}

func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}
	if strings.Contains(err.Error(), "connection reset by peer") ||
		strings.Contains(err.Error(), "broken pipe") ||
		strings.Contains(err.Error(), "connection refused") {
		return true
	}
	return false
}

// retryConfig holds retry settings for API calls
type retryConfig struct {
	maxAttempts       int           // Maximum number of attempts
	baseDelay         time.Duration // Base delay for exponential backoff
	maxDelay          time.Duration // Maximum delay cap
	rateLimitAttempts int           // Extra attempts for rate limit errors
}

// defaultRetryConfig returns default retry settings
func defaultRetryConfig() retryConfig {
	return retryConfig{
		maxAttempts:       3,
		baseDelay:         2 * time.Second,
		maxDelay:          30 * time.Second,
		rateLimitAttempts: 5, // More attempts for rate limits
	}
}

// calculateBackoff returns the backoff duration for the given attempt.
// Uses exponential backoff: baseDelay * 2^attempt, capped at maxDelay.
func calculateBackoff(attempt int, cfg retryConfig) time.Duration {
	delay := cfg.baseDelay * time.Duration(1<<uint(attempt)) // 2^attempt
	if delay > cfg.maxDelay {
		delay = cfg.maxDelay
	}
	return delay
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
	release, err := AcquireLLMSlot(ctx)
	if err != nil {
		return "", err
	}
	if release != nil {
		defer release()
	}

	opts = normalizeChatOptions(opts)
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
	release, err := AcquireLLMSlot(ctx)
	if err != nil {
		return "", err
	}
	if release != nil {
		defer release()
	}

	opts = normalizeChatOptions(opts)
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
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content,omitempty"` // Some models (e.g., z.ai) use this field
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

// ============ Streaming Support ============

// SupportsStreaming returns true for OpenAI-compatible providers
func (p *OpenAIProvider) SupportsStreaming() bool {
	return true
}

// ChatStream streams tokens from OpenAI-compatible APIs
func (p *OpenAIProvider) ChatStream(ctx context.Context, messages []ChatMessage, opts ChatOptions, onToken TokenCallback) (string, error) {
	release, err := AcquireLLMSlot(ctx)
	if err != nil {
		return "", err
	}
	if release != nil {
		defer release()
	}

	opts = normalizeChatOptions(opts)
	model := opts.Model
	if model == "" {
		model = p.model
	}

	reqBody := map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   true,
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

	var lastErr error
	retryCfg := defaultRetryConfig()
	rateLimitHits := 0

	for attempt := 0; attempt < retryCfg.maxAttempts+retryCfg.rateLimitAttempts; attempt++ {
		if attempt > 0 {
			delay := calculateBackoff(attempt-1, retryCfg)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}

		if attempt >= retryCfg.maxAttempts && rateLimitHits == 0 {
			break
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
			lastErr = err
			if isTransientError(err) {
				continue
			}
			return "", fmt.Errorf("request failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				lastErr = fmt.Errorf("API error (status %d): failed to read response body: %w", resp.StatusCode, readErr)
				continue
			}
			if resp.StatusCode == http.StatusTooManyRequests {
				rateLimitHits++
				lastErr = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
				continue
			}
			if resp.StatusCode >= 500 {
				lastErr = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
				continue
			}
			return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
		}

		result, parseErr := parseOpenAISSE(resp.Body, onToken)
		resp.Body.Close()
		return result, parseErr
	}

	return "", fmt.Errorf("request failed after retries: %w", lastErr)
}

// parseOpenAISSE parses OpenAI-style SSE stream
func parseOpenAISSE(reader io.Reader, onToken TokenCallback) (string, error) {
	scanner := bufio.NewScanner(reader)
	var accumulated strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// Parse SSE data lines
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			// Check for stream end
			if data == "[DONE]" {
				break
			}

			// Parse JSON
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content          string `json:"content"`
						ReasoningContent string `json:"reasoning_content,omitempty"` // Some models (e.g., z.ai) use this field
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
			}

			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue // Skip malformed chunks
			}

			if len(chunk.Choices) > 0 {
				content := chunk.Choices[0].Delta.Content
				if content != "" {
					accumulated.WriteString(content)
					if onToken != nil {
						onToken(content)
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return accumulated.String(), fmt.Errorf("stream read error: %w", err)
	}

	return accumulated.String(), nil
}

// SupportsStreaming returns true for Anthropic
func (p *AnthropicProvider) SupportsStreaming() bool {
	return true
}

// ChatStream streams tokens from Anthropic API
func (p *AnthropicProvider) ChatStream(ctx context.Context, messages []ChatMessage, opts ChatOptions, onToken TokenCallback) (string, error) {
	release, err := AcquireLLMSlot(ctx)
	if err != nil {
		return "", err
	}
	if release != nil {
		defer release()
	}

	opts = normalizeChatOptions(opts)
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
		"stream":     true,
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

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("API error (status %d): failed to read response body: %w", resp.StatusCode, err)
		}
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	return parseAnthropicSSE(resp.Body, onToken)
}

// parseAnthropicSSE parses Anthropic-style SSE stream
func parseAnthropicSSE(reader io.Reader, onToken TokenCallback) (string, error) {
	scanner := bufio.NewScanner(reader)
	var accumulated strings.Builder
	var eventType string

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			eventType = ""
			continue
		}

		// Parse event type
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		// Parse data
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			// Handle content_block_delta events
			if eventType == "content_block_delta" {
				var delta struct {
					Type  string `json:"type"`
					Delta struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"delta"`
				}

				if err := json.Unmarshal([]byte(data), &delta); err != nil {
					continue
				}

				if delta.Delta.Text != "" {
					accumulated.WriteString(delta.Delta.Text)
					if onToken != nil {
						onToken(delta.Delta.Text)
					}
				}
			}

			// Check for message_stop
			if eventType == "message_stop" {
				break
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return accumulated.String(), fmt.Errorf("stream read error: %w", err)
	}

	return accumulated.String(), nil
}

// SupportsStreaming returns true for Ollama
func (p *OllamaProvider) SupportsStreaming() bool {
	return true
}

// ChatStream streams tokens from Ollama API (NDJSON format)
func (p *OllamaProvider) ChatStream(ctx context.Context, messages []ChatMessage, opts ChatOptions, onToken TokenCallback) (string, error) {
	release, err := AcquireLLMSlot(ctx)
	if err != nil {
		return "", err
	}
	if release != nil {
		defer release()
	}

	opts = normalizeChatOptions(opts)
	model := opts.Model
	if model == "" {
		model = p.model
	}

	reqBody := map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   true,
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

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("API error (status %d): failed to read response body: %w", resp.StatusCode, err)
		}
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	return parseOllamaNDJSON(resp.Body, onToken)
}

// parseOllamaNDJSON parses Ollama NDJSON stream
func parseOllamaNDJSON(reader io.Reader, onToken TokenCallback) (string, error) {
	scanner := bufio.NewScanner(reader)
	var accumulated strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var chunk struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content,omitempty"` // Some models (e.g., z.ai) use this field
			} `json:"message"`
			Done bool `json:"done"`
		}

		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}

		if chunk.Message.Content != "" {
			accumulated.WriteString(chunk.Message.Content)
			if onToken != nil {
				onToken(chunk.Message.Content)
			}
		}

		if chunk.Done {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return accumulated.String(), fmt.Errorf("stream read error: %w", err)
	}

	return accumulated.String(), nil
}
