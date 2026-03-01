package main

import (
	"errors"
	"testing"
	"time"
)

func TestProviderIntelligenceRecordRequest(t *testing.T) {
	pi := NewProviderIntelligence()

	// Record successful request
	pi.RecordRequest("openai", 150, 100, nil)

	metrics := pi.GetMetrics("openai")
	if metrics == nil {
		t.Fatal("Expected metrics for openai")
	}

	if metrics.TotalRequests != 1 {
		t.Errorf("Expected TotalRequests=1, got %d", metrics.TotalRequests)
	}
	if metrics.SuccessCount != 1 {
		t.Errorf("Expected SuccessCount=1, got %d", metrics.SuccessCount)
	}
	if metrics.TotalLatencyMs != 150 {
		t.Errorf("Expected TotalLatencyMs=150, got %d", metrics.TotalLatencyMs)
	}

	// Record failed request
	pi.RecordRequest("openai", 100, 0, errors.New("rate limit exceeded"))

	if metrics.TotalRequests != 2 {
		t.Errorf("Expected TotalRequests=2, got %d", metrics.TotalRequests)
	}
	if metrics.FailureCount != 1 {
		t.Errorf("Expected FailureCount=1, got %d", metrics.FailureCount)
	}
	if metrics.RateLimitCount != 1 {
		t.Errorf("Expected RateLimitCount=1, got %d", metrics.RateLimitCount)
	}
}

func TestProviderIntelligenceCalculateScore(t *testing.T) {
	pi := NewProviderIntelligence()

	// Unknown provider should have neutral score
	score := pi.CalculateScore("unknown")
	if score != 50.0 {
		t.Errorf("Expected score=50.0 for unknown provider, got %f", score)
	}

	// Provider with no requests should have neutral score
	score = pi.CalculateScore("openai")
	if score != 50.0 {
		t.Errorf("Expected score=50.0 for provider with no requests, got %f", score)
	}

	// Provider with all successes should have high score
	pi.RecordRequest("groq", 100, 100, nil)
	pi.RecordRequest("groq", 150, 100, nil)
	pi.RecordRequest("groq", 120, 100, nil)

	score = pi.CalculateScore("groq")
	if score < 80.0 {
		t.Errorf("Expected high score for all successes, got %f", score)
	}

	// Provider with failures should have lower score than perfect provider
	pi.RecordRequest("deepseek", 100, 100, nil)
	pi.RecordRequest("deepseek", 0, 0, errors.New("failed"))
	pi.RecordRequest("deepseek", 0, 0, errors.New("failed"))

	deepseekScore := pi.CalculateScore("deepseek")
	groqScore := pi.CalculateScore("groq")

	// Deepseek has 33% success vs groq's 100%, so should score lower
	if deepseekScore >= groqScore {
		t.Errorf("Expected deepseek score (%f) to be lower than groq score (%f)", deepseekScore, groqScore)
	}
}

func TestProviderIntelligenceUnavailable(t *testing.T) {
	pi := NewProviderIntelligence()

	// Three consecutive failures should mark provider unavailable
	pi.RecordRequest("together", 0, 0, errors.New("error 1"))
	pi.RecordRequest("together", 0, 0, errors.New("error 2"))
	pi.RecordRequest("together", 0, 0, errors.New("error 3"))

	metrics := pi.GetMetrics("together")
	if metrics.IsAvailable {
		t.Error("Expected provider to be unavailable after 3 consecutive failures")
	}

	score := pi.CalculateScore("together")
	if score != 0.0 {
		t.Errorf("Expected score=0 for unavailable provider, got %f", score)
	}

	// Reset should make it available again
	pi.ResetProvider("together")
	if !metrics.IsAvailable {
		t.Error("Expected provider to be available after reset")
	}
}

func TestProviderIntelligenceSelectBestProvider(t *testing.T) {
	pi := NewProviderIntelligence()

	// Record some requests to establish scores
	pi.RecordRequest("openai", 200, 100, nil)
	pi.RecordRequest("groq", 100, 100, nil)
	pi.RecordRequest("groq", 100, 100, nil)
	pi.RecordRequest("anthropic", 0, 0, errors.New("failed"))
	pi.RecordRequest("anthropic", 0, 0, errors.New("failed"))
	pi.RecordRequest("anthropic", 0, 0, errors.New("failed"))

	// Specific provider if available
	result := pi.SelectBestProvider("openai")
	if result != "openai" {
		t.Errorf("Expected openai when requested, got %s", result)
	}

	// Unavailable specific provider should fallback
	result = pi.SelectBestProvider("anthropic")
	if result == "anthropic" {
		t.Error("Should not select unavailable provider")
	}
}

func TestProviderIntelligenceGetRankedProviders(t *testing.T) {
	pi := NewProviderIntelligence()

	// Record some requests
	pi.RecordRequest("openai", 200, 100, nil)
	pi.RecordRequest("groq", 50, 100, nil)  // Lower latency = better
	pi.RecordRequest("groq", 50, 100, nil)

	ranked := pi.GetRankedProviders()
	if len(ranked) == 0 {
		t.Fatal("Expected ranked providers list")
	}

	// Groq should rank higher due to lower latency and more successes
	// but this depends on scoring, so just check the list is sorted
	prevScore := ranked[0].Score
	for i := 1; i < len(ranked); i++ {
		if ranked[i].Score > prevScore {
			t.Errorf("Expected scores to be sorted descending, got %f after %f", ranked[i].Score, prevScore)
		}
		prevScore = ranked[i].Score
	}
}

func TestProviderIntelligenceRecencyBonus(t *testing.T) {
	pi := NewProviderIntelligence()

	// Record some mixed results so base score isn't 100
	pi.RecordRequest("openai", 100, 100, nil)
	pi.RecordRequest("openai", 0, 0, errors.New("error"))

	// Get score immediately (should have recency bonus since we had recent success)
	scoreWithBonus := pi.CalculateScore("openai")

	// Simulate waiting by manipulating the metrics
	metrics := pi.GetMetrics("openai")
	metrics.mu.Lock()
	metrics.LastSuccessTime = time.Now().Add(-2 * time.Hour)
	metrics.mu.Unlock()

	scoreWithoutBonus := pi.CalculateScore("openai")

	// With recent success: base + 15 bonus
	// Without recent success: base + 0 bonus
	if scoreWithBonus <= scoreWithoutBonus {
		t.Errorf("Expected recency bonus: immediate=%f, after 2h=%f", scoreWithBonus, scoreWithoutBonus)
	}
}
