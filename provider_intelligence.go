package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
	"time"
)

// ProviderMetrics tracks performance data for a single provider
type ProviderMetrics struct {
	Name string

	// Request counts
	TotalRequests   int64
	SuccessCount    int64
	FailureCount    int64
	TimeoutCount    int64
	RateLimitCount  int64

	// Latency tracking (milliseconds)
	TotalLatencyMs  int64
	MinLatencyMs    int64
	MaxLatencyMs    int64

	// Token tracking
	TotalTokensUsed int64

	// Error tracking
	LastError     error
	LastErrorTime time.Time

	// Availability
	ConsecutiveFailures int
	LastSuccessTime     time.Time
	IsAvailable         bool

	mu sync.RWMutex
}

// ProviderIntelligence manages provider selection and metrics
type ProviderIntelligence struct {
	metrics    map[string]*ProviderMetrics
	fallbacks  []string
	defaultPrv string
	mu         sync.RWMutex
}

// Global provider intelligence instance
var (
	globalProviderIntel     *ProviderIntelligence
	providerIntelLock       sync.Mutex
	providerIntelInitialized bool
)

// GetProviderIntelligence returns the global provider intelligence instance
func GetProviderIntelligence() *ProviderIntelligence {
	providerIntelLock.Lock()
	defer providerIntelLock.Unlock()

	if !providerIntelInitialized {
		globalProviderIntel = NewProviderIntelligence()
		providerIntelInitialized = true
	}
	return globalProviderIntel
}

// NewProviderIntelligence creates a new provider intelligence instance
func NewProviderIntelligence() *ProviderIntelligence {
	pi := &ProviderIntelligence{
		metrics: make(map[string]*ProviderMetrics),
	}

	// Initialize metrics for known providers
	providers := []string{"openai", "anthropic", "groq", "ollama", "deepseek", "openrouter", "zai", "together"}
	for _, p := range providers {
		pi.metrics[p] = &ProviderMetrics{
			Name:        p,
			IsAvailable: true,
			MinLatencyMs: math.MaxInt64,
		}
	}

	// Load fallback configuration from file config
	fileConfig, _ := GetFileConfig()
	if fileConfig != nil {
		pi.defaultPrv = fileConfig.Providers.Default
		pi.fallbacks = fileConfig.Providers.Fallbacks
	}

	return pi
}

// RecordRequest records the result of an LLM request
func (pi *ProviderIntelligence) RecordRequest(provider string, latencyMs int64, tokensUsed int, err error) {
	pi.mu.Lock()
	m, ok := pi.metrics[provider]
	if !ok {
		m = &ProviderMetrics{
			Name:        provider,
			IsAvailable: true,
			MinLatencyMs: math.MaxInt64,
		}
		pi.metrics[provider] = m
	}
	pi.mu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalRequests++
	m.TotalTokensUsed += int64(tokensUsed)

	if err == nil {
		// Success
		m.SuccessCount++
		m.TotalLatencyMs += latencyMs
		if latencyMs < m.MinLatencyMs {
			m.MinLatencyMs = latencyMs
		}
		if latencyMs > m.MaxLatencyMs {
			m.MaxLatencyMs = latencyMs
		}
		m.ConsecutiveFailures = 0
		m.LastSuccessTime = time.Now()
		m.IsAvailable = true
	} else {
		// Failure
		m.FailureCount++
		m.ConsecutiveFailures++
		m.LastError = err
		m.LastErrorTime = time.Now()

		// Categorize error
		category := CategorizeError(err)
		switch category {
		case ErrorTransient:
			errLower := err.Error()
			if contains(errLower, "timeout", "timed out") {
				m.TimeoutCount++
			}
			if contains(errLower, "rate limit", "429", "too many") {
				m.RateLimitCount++
			}
		}

		// Mark unavailable after too many consecutive failures
		if m.ConsecutiveFailures >= 3 {
			m.IsAvailable = false
			log.Printf("[PROVIDER-INTEL] Marking %s as unavailable after %d consecutive failures", provider, m.ConsecutiveFailures)
		}
	}
}

// contains checks if s contains any of the substrings
func contains(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(sub) > 0 && len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// GetMetrics returns metrics for a specific provider
func (pi *ProviderIntelligence) GetMetrics(provider string) *ProviderMetrics {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	return pi.metrics[provider]
}

// GetAllMetrics returns metrics for all providers
func (pi *ProviderIntelligence) GetAllMetrics() map[string]*ProviderMetrics {
	pi.mu.RLock()
	defer pi.mu.RUnlock()

	result := make(map[string]*ProviderMetrics)
	for k, v := range pi.metrics {
		result[k] = v
	}
	return result
}

// CalculateScore calculates a performance score for a provider (0-100)
// Higher is better
func (pi *ProviderIntelligence) CalculateScore(provider string) float64 {
	pi.mu.RLock()
	m, ok := pi.metrics[provider]
	pi.mu.RUnlock()

	if !ok {
		return 50.0 // Default score for unknown providers
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// If no requests yet, return neutral score
	if m.TotalRequests == 0 {
		return 50.0
	}

	// If unavailable, return 0
	if !m.IsAvailable {
		return 0.0
	}

	var score float64 = 100.0

	// Success rate component (0-40 points)
	successRate := float64(m.SuccessCount) / float64(m.TotalRequests)
	score -= (1.0 - successRate) * 40.0

	// Latency component (0-30 points)
	// Penalize slow providers
	if m.SuccessCount > 0 {
		avgLatency := float64(m.TotalLatencyMs) / float64(m.SuccessCount)
		// Target: 1000ms or less is ideal
		if avgLatency > 1000 {
			latencyPenalty := math.Min((avgLatency-1000)/5000*30, 30)
			score -= latencyPenalty
		}
	}

	// Rate limit penalty (0-15 points)
	rateLimitRate := float64(m.RateLimitCount) / float64(m.TotalRequests)
	score -= rateLimitRate * 15.0

	// Recency bonus (0-15 points)
	// Recently successful providers get a bonus
	if !m.LastSuccessTime.IsZero() {
		sinceSuccess := time.Since(m.LastSuccessTime)
		if sinceSuccess < 5*time.Minute {
			score += 15.0
		} else if sinceSuccess < 30*time.Minute {
			score += 10.0
		} else if sinceSuccess < 1*time.Hour {
			score += 5.0
		}
	}

	// Clamp to 0-100
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}

// ProviderScore represents a provider and its score
type ProviderScore struct {
	Provider string
	Score    float64
	Metrics  *ProviderMetrics
}

// GetRankedProviders returns providers sorted by score (highest first)
func (pi *ProviderIntelligence) GetRankedProviders() []ProviderScore {
	pi.mu.RLock()
	defer pi.mu.RUnlock()

	scores := make([]ProviderScore, 0, len(pi.metrics))
	for name, m := range pi.metrics {
		scores = append(scores, ProviderScore{
			Provider: name,
			Score:    pi.CalculateScore(name),
			Metrics:  m,
		})
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	return scores
}

// SelectBestProvider selects the best available provider
// It considers both configured fallbacks and performance scores
func (pi *ProviderIntelligence) SelectBestProvider(requiredProvider string) string {
	// If a specific provider is required and available, use it
	if requiredProvider != "" {
		m := pi.GetMetrics(requiredProvider)
		if m != nil && m.IsAvailable {
			return requiredProvider
		}
	}

	// If default provider is set and available, prefer it
	if pi.defaultPrv != "" {
		m := pi.GetMetrics(pi.defaultPrv)
		if m != nil && m.IsAvailable {
			return pi.defaultPrv
		}
	}

	// Try configured fallbacks in order
	for _, fb := range pi.fallbacks {
		m := pi.GetMetrics(fb)
		if m != nil && m.IsAvailable {
			return fb
		}
	}

	// Fall back to best scoring provider
	ranked := pi.GetRankedProviders()
	for _, ps := range ranked {
		if ps.Metrics.IsAvailable && ps.Score > 0 {
			return ps.Provider
		}
	}

	// No available providers - return the first with any possibility
	for _, ps := range ranked {
		if ps.Score > 0 {
			return ps.Provider
		}
	}

	return "" // No providers available
}

// ResetProvider resets a provider's availability (e.g., after manual intervention)
func (pi *ProviderIntelligence) ResetProvider(provider string) {
	pi.mu.RLock()
	m, ok := pi.metrics[provider]
	pi.mu.RUnlock()

	if ok {
		m.mu.Lock()
		m.IsAvailable = true
		m.ConsecutiveFailures = 0
		m.mu.Unlock()
		log.Printf("[PROVIDER-INTEL] Reset availability for %s", provider)
	}
}

// GetProviderSummary returns a human-readable summary of provider status
func (pi *ProviderIntelligence) GetProviderSummary() string {
	ranked := pi.GetRankedProviders()

	var result string
	result = "Provider Intelligence Summary:\n"
	result += "============================\n\n"

	for _, ps := range ranked {
		ps.Metrics.mu.RLock()
		status := "available"
		if !ps.Metrics.IsAvailable {
			status = "UNAVAILABLE"
		}

		avgLatency := int64(0)
		if ps.Metrics.SuccessCount > 0 {
			avgLatency = ps.Metrics.TotalLatencyMs / ps.Metrics.SuccessCount
		}

		successRate := float64(0)
		if ps.Metrics.TotalRequests > 0 {
			successRate = float64(ps.Metrics.SuccessCount) / float64(ps.Metrics.TotalRequests) * 100
		}

		result += fmt.Sprintf("%s (Score: %.1f, %s)\n", ps.Provider, ps.Score, status)
		result += fmt.Sprintf("  Requests: %d total, %d success (%.1f%%)\n",
			ps.Metrics.TotalRequests, ps.Metrics.SuccessCount, successRate)
		if avgLatency > 0 {
			result += fmt.Sprintf("  Latency: avg %dms, min %dms, max %dms\n",
				avgLatency, ps.Metrics.MinLatencyMs, ps.Metrics.MaxLatencyMs)
		}
		if ps.Metrics.RateLimitCount > 0 {
			result += fmt.Sprintf("  Rate limits: %d\n", ps.Metrics.RateLimitCount)
		}
		if ps.Metrics.LastError != nil {
			result += fmt.Sprintf("  Last error: %v (at %s)\n",
				ps.Metrics.LastError, ps.Metrics.LastErrorTime.Format(time.RFC3339))
		}
		ps.Metrics.mu.RUnlock()
		result += "\n"
	}

	return result
}

// SmartProviderWrapper wraps a provider with metrics tracking and fallback
type SmartProviderWrapper struct {
	primary      Provider
	primaryName  string
	fallbackCfgs []ProviderConfig
	intel        *ProviderIntelligence
}

// NewSmartProvider creates a provider with intelligent fallback
func NewSmartProvider(primaryType string, fallbackTypes []string) (Provider, error) {
	intel := GetProviderIntelligence()

	// Get or use best provider as primary
	if primaryType == "" {
		primaryType = intel.SelectBestProvider("")
		if primaryType == "" {
			primaryType = detectProviderFromEnv()
		}
	}

	// Create primary provider
	apiKey := getAPIKeyForProvider(primaryType)
	primary, err := NewProvider(ProviderConfig{
		Type:   primaryType,
		APIKey: apiKey,
	})
	if err != nil {
		return nil, NewConfigError(fmt.Sprintf("failed to create provider %s", primaryType), err)
	}

	// Store fallback configs (we create them lazily on failure)
	var fallbackCfgs []ProviderConfig
	for _, ft := range fallbackTypes {
		if ft != primaryType {
			fallbackCfgs = append(fallbackCfgs, ProviderConfig{
				Type:   ft,
				APIKey: getAPIKeyForProvider(ft),
			})
		}
	}

	return &SmartProviderWrapper{
		primary:      primary,
		primaryName:  primaryType,
		fallbackCfgs: fallbackCfgs,
		intel:        intel,
	}, nil
}

// Chat implements Provider interface with fallback support
func (s *SmartProviderWrapper) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (string, error) {
	start := time.Now()

	// Try primary provider
	response, err := s.primary.Chat(ctx, messages, opts)
	latency := time.Since(start).Milliseconds()

	if err == nil {
		s.intel.RecordRequest(s.primaryName, latency, 0, nil)
		return response, nil
	}

	// Record primary failure
	s.intel.RecordRequest(s.primaryName, latency, 0, err)

	// Check if error is retryable
	if !IsRetryable(err) {
		return "", err
	}

	// Try fallbacks
	for _, cfg := range s.fallbackCfgs {
		fb, createErr := NewProvider(cfg)
		if createErr != nil {
			continue // Skip providers that can't be created
		}

		start = time.Now()
		response, err = fb.Chat(ctx, messages, opts)
		latency = time.Since(start).Milliseconds()

		if err == nil {
			s.intel.RecordRequest(cfg.Type, latency, 0, nil)
			log.Printf("[PROVIDER-INTEL] Fallback to %s succeeded", cfg.Type)
			return response, nil
		}

		s.intel.RecordRequest(cfg.Type, latency, 0, err)
		if !IsRetryable(err) {
			return "", err
		}
	}

	return "", fmt.Errorf("all providers failed, last error: %w", err)
}

// Name returns the primary provider name
func (s *SmartProviderWrapper) Name() string {
	return s.primaryName + " (smart)"
}
