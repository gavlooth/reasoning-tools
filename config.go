package main

import (
	"context"
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

	// Concurrency control
	MaxConcurrentLLMRequests int // Maximum concurrent LLM API requests (0 = unlimited)

	// LLM request limits
	MaxTokensCap int // Max tokens allowed in a single LLM request (0 = default cap)
}

// Validation bounds for timeout values
const (
	// Minimum timeout: 1 second (no timeouts shorter than this make sense)
	minTimeout = 1 * time.Second
	// Maximum timeout: 1 hour (prevents resource exhaustion and hangs)
	maxTimeout = 1 * time.Hour
	// Maximum tool timeout: 5 minutes (tools should complete quickly)
	maxToolTimeout = 5 * time.Minute

	// Concurrency limits
	defaultMaxConcurrentLLMRequests = 2  // Default: allow 2 concurrent LLM requests
	maxConcurrentLLMRequests        = 20 // Hard cap to prevent abuse

	defaultMaxTokensCap = 8192
)

// DefaultConfig returns default configuration values
func DefaultConfig() *Config {
	return &Config{
		OpenAITimeout:            120 * time.Second,
		AnthropicTimeout:         120 * time.Second,
		GroqTimeout:              120 * time.Second,
		OllamaTimeout:            300 * time.Second,
		DeepSeekTimeout:          120 * time.Second,
		OpenRouterTimeout:        120 * time.Second,
		ZaiTimeout:               120 * time.Second,
		TogetherTimeout:          120 * time.Second,
		CodeExecTimeout:          10 * time.Second,
		WebFetchTimeout:          15 * time.Second,
		MaxConcurrentLLMRequests: defaultMaxConcurrentLLMRequests,
		MaxTokensCap:             defaultMaxTokensCap,
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

	// Concurrency control
	if v := os.Getenv("LLM_MAX_CONCURRENT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			if n <= 0 {
				cfg.MaxConcurrentLLMRequests = 0 // 0 means unlimited
				log.Printf("[CONFIG] LLM_MAX_CONCURRENT set to unlimited")
			} else if n > maxConcurrentLLMRequests {
				cfg.MaxConcurrentLLMRequests = maxConcurrentLLMRequests
				log.Printf("[CONFIG] LLM_MAX_CONCURRENT (%d) exceeds maximum (%d), clamping", n, maxConcurrentLLMRequests)
			} else {
				cfg.MaxConcurrentLLMRequests = n
			}
		}
	}

	// Max tokens cap (per request)
	if v := os.Getenv("LLM_MAX_TOKENS_CAP"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			switch {
			case n < minTokensCap:
				cfg.MaxTokensCap = minTokensCap
				log.Printf("[CONFIG] LLM_MAX_TOKENS_CAP (%d) below minimum (%d), clamping", n, minTokensCap)
			case n > maxTokensCap:
				cfg.MaxTokensCap = maxTokensCap
				log.Printf("[CONFIG] LLM_MAX_TOKENS_CAP (%d) exceeds maximum (%d), clamping", n, maxTokensCap)
			default:
				cfg.MaxTokensCap = n
			}
		}
	}

	return cfg
}

// Global config instance (initialized lazily)
var (
	globalConfig *Config
	configLock   sync.RWMutex
)

// GetConfig returns the global configuration, initializing it lazily on first access.
// This allows tests to set environment variables before the first call to GetConfig(),
// resolving the initialization order dependency issue.
func GetConfig() *Config {
	// Fast path: read lock for concurrent reads
	configLock.RLock()
	if globalConfig != nil {
		configLock.RUnlock()
		return globalConfig
	}
	configLock.RUnlock()

	// Slow path: write lock for initialization
	configLock.Lock()
	defer configLock.Unlock()

	// Double-check: another goroutine might have initialized while we were waiting
	if globalConfig != nil {
		return globalConfig
	}

	// Initialize config
	globalConfig = LoadConfig()
	return globalConfig
}

// ResetConfig resets global configuration for testing purposes.
// This allows tests to set different environment variables and reload configuration.
//
// NOTE: This function is designed for test scenarios only. It is not safe
// for concurrent use with GetConfig() outside of controlled test environments.
func ResetConfig() {
	// Acquire write lock to safely reset configuration
	// This blocks any concurrent reads and ensures we don't reset during initialization
	configLock.Lock()
	defer configLock.Unlock()

	// Reset global config pointer to nil
	globalConfig = nil

	// Reset FIFO limiter
	llmLimiterLock.Lock()
	if llmLimiter != nil {
		llmLimiter.Stop()
		llmLimiter = nil
	}
	llmLimiterLock.Unlock()
}

// ============ LLM Request Rate Limiting (FIFO Queue) ============

// FIFOLimiter implements a fair, first-in-first-out rate limiter using channels.
// Requests are processed in the order they arrive, with bounded concurrency.
type FIFOLimiter struct {
	queue chan chan func() // queue of response channels, preserves FIFO order
	done  chan struct{}    // signals shutdown
}

// NewFIFOLimiter creates a new FIFO rate limiter with the given concurrency limit.
func NewFIFOLimiter(maxConcurrent int) *FIFOLimiter {
	l := &FIFOLimiter{
		queue: make(chan chan func(), 10000), // large buffer to avoid blocking enqueuers
		done:  make(chan struct{}),
	}
	go l.dispatcher(maxConcurrent)
	return l
}

// dispatcher processes queued requests in FIFO order, respecting concurrency limits.
func (l *FIFOLimiter) dispatcher(maxConcurrent int) {
	sem := make(chan struct{}, maxConcurrent)

	for {
		select {
		case respChan := <-l.queue:
			// Acquire a slot (blocks if all slots are in use)
			sem <- struct{}{}
			// Send release function to the waiting goroutine
			select {
			case respChan <- func() { <-sem }:
				// Successfully sent
			default:
				// Waiter gave up (context cancelled), release slot immediately
				<-sem
			}
		case <-l.done:
			return
		}
	}
}

// Acquire waits for a slot in FIFO order.
// Returns a release function that MUST be called when done, or an error if context was cancelled.
func (l *FIFOLimiter) Acquire(ctx context.Context) (func(), error) {
	respChan := make(chan func(), 1)

	// Enqueue our response channel - FIFO order preserved by channel semantics
	select {
	case l.queue <- respChan:
		// Enqueued successfully
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Wait for our turn
	select {
	case release := <-respChan:
		return release, nil
	case <-ctx.Done():
		// Context cancelled while waiting - dispatcher will handle the orphaned entry
		return nil, ctx.Err()
	}
}

// Stop shuts down the dispatcher goroutine.
func (l *FIFOLimiter) Stop() {
	close(l.done)
}

// Global FIFO limiter instance
var (
	llmLimiter     *FIFOLimiter
	llmLimiterLock sync.Mutex
)

// getLLMLimiter returns the global LLM request limiter, initializing it if needed.
// Returns nil if rate limiting is disabled (MaxConcurrentLLMRequests == 0).
func getLLMLimiter() *FIFOLimiter {
	llmLimiterLock.Lock()
	defer llmLimiterLock.Unlock()

	if llmLimiter != nil {
		return llmLimiter
	}

	cfg := GetConfig()
	if cfg.MaxConcurrentLLMRequests <= 0 {
		return nil // Unlimited
	}

	llmLimiter = NewFIFOLimiter(cfg.MaxConcurrentLLMRequests)
	log.Printf("[RATE-LIMIT] FIFO limiter initialized with capacity %d", cfg.MaxConcurrentLLMRequests)
	return llmLimiter
}

// AcquireLLMSlot acquires a slot for making an LLM request in FIFO order.
// Blocks until a slot is available or context is cancelled.
// Returns a release function that MUST be called when done, or an error if context was cancelled.
func AcquireLLMSlot(ctx context.Context) (release func(), err error) {
	limiter := getLLMLimiter()
	if limiter == nil {
		// No rate limiting, return no-op release
		return func() {}, nil
	}

	return limiter.Acquire(ctx)
}
