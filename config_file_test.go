package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultFileConfig(t *testing.T) {
	cfg := DefaultFileConfig()

	if cfg == nil {
		t.Fatal("DefaultFileConfig returned nil")
	}

	// Check algorithm defaults
	if cfg.Algorithms.Sequential.MaxThoughts != 10 {
		t.Errorf("Expected MaxThoughts=10, got %d", cfg.Algorithms.Sequential.MaxThoughts)
	}
	if cfg.Algorithms.GraphOfThoughts.BranchingFactor != 3 {
		t.Errorf("Expected BranchingFactor=3, got %d", cfg.Algorithms.GraphOfThoughts.BranchingFactor)
	}
	if cfg.Algorithms.Reflexion.MaxAttempts != 3 {
		t.Errorf("Expected MaxAttempts=3, got %d", cfg.Algorithms.Reflexion.MaxAttempts)
	}
	if cfg.Algorithms.Dialectic.MaxRounds != 5 {
		t.Errorf("Expected MaxRounds=5, got %d", cfg.Algorithms.Dialectic.MaxRounds)
	}

	// Check timeout defaults
	if cfg.Timeouts.OpenAI != 120 {
		t.Errorf("Expected OpenAI timeout=120, got %d", cfg.Timeouts.OpenAI)
	}
	if cfg.Timeouts.Ollama != 300 {
		t.Errorf("Expected Ollama timeout=300, got %d", cfg.Timeouts.Ollama)
	}

	// Check rate limiting defaults
	if cfg.RateLimiting.MaxConcurrent != 2 {
		t.Errorf("Expected MaxConcurrent=2, got %d", cfg.RateLimiting.MaxConcurrent)
	}
}

func TestConfigFilePaths(t *testing.T) {
	paths := ConfigFilePaths()

	if len(paths) == 0 {
		t.Fatal("ConfigFilePaths returned empty list")
	}

	// Should include local paths
	hasLocal := false
	for _, p := range paths {
		if p == "reasoning-tools.yaml" || p == "reasoning-tools.yml" {
			hasLocal = true
			break
		}
	}
	if !hasLocal {
		t.Error("ConfigFilePaths should include local directory paths")
	}
}

func TestLoadConfigFromYAML(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `
providers:
  default: "groq"
  fallbacks:
    - "deepseek"
    - "ollama"

algorithms:
  sequential:
    max_thoughts: 15
  graph_of_thoughts:
    branching_factor: 5
    max_nodes: 50
  reflexion:
    max_attempts: 5
  dialectic:
    max_rounds: 7
    confidence_target: 0.9

timeouts:
  openai: 180
  groq: 60

rate_limiting:
  max_concurrent: 4
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Set env var to point to test config
	os.Setenv("REASONING_TOOLS_CONFIG", configPath)
	defer os.Unsetenv("REASONING_TOOLS_CONFIG")

	// Reset global state
	fileConfigLoaded = false
	globalFileConfig = nil

	cfg, path, err := LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig failed: %v", err)
	}

	if path != configPath {
		t.Errorf("Expected path=%s, got %s", configPath, path)
	}

	// Check parsed values
	if cfg.Providers.Default != "groq" {
		t.Errorf("Expected default provider=groq, got %s", cfg.Providers.Default)
	}
	if len(cfg.Providers.Fallbacks) != 2 {
		t.Errorf("Expected 2 fallbacks, got %d", len(cfg.Providers.Fallbacks))
	}
	if cfg.Algorithms.Sequential.MaxThoughts != 15 {
		t.Errorf("Expected MaxThoughts=15, got %d", cfg.Algorithms.Sequential.MaxThoughts)
	}
	if cfg.Algorithms.GraphOfThoughts.BranchingFactor != 5 {
		t.Errorf("Expected BranchingFactor=5, got %d", cfg.Algorithms.GraphOfThoughts.BranchingFactor)
	}
	if cfg.Algorithms.Dialectic.ConfidenceTarget != 0.9 {
		t.Errorf("Expected ConfidenceTarget=0.9, got %f", cfg.Algorithms.Dialectic.ConfidenceTarget)
	}
	if cfg.Timeouts.OpenAI != 180 {
		t.Errorf("Expected OpenAI timeout=180, got %d", cfg.Timeouts.OpenAI)
	}
	if cfg.RateLimiting.MaxConcurrent != 4 {
		t.Errorf("Expected MaxConcurrent=4, got %d", cfg.RateLimiting.MaxConcurrent)
	}
}

func TestApplyToConfig(t *testing.T) {
	fc := DefaultFileConfig()
	fc.Timeouts.OpenAI = 200
	fc.Timeouts.Groq = 100
	fc.RateLimiting.MaxConcurrent = 5
	fc.RateLimiting.MaxTokensCap = 4096

	cfg := DefaultConfig()
	fc.ApplyToConfig(cfg)

	if cfg.OpenAITimeout != 200*time.Second {
		t.Errorf("Expected OpenAITimeout=200s, got %v", cfg.OpenAITimeout)
	}
	if cfg.GroqTimeout != 100*time.Second {
		t.Errorf("Expected GroqTimeout=100s, got %v", cfg.GroqTimeout)
	}
	if cfg.MaxConcurrentLLMRequests != 5 {
		t.Errorf("Expected MaxConcurrentLLMRequests=5, got %d", cfg.MaxConcurrentLLMRequests)
	}
	if cfg.MaxTokensCap != 4096 {
		t.Errorf("Expected MaxTokensCap=4096, got %d", cfg.MaxTokensCap)
	}
}

func TestGetDefaultGoTConfig(t *testing.T) {
	fc := DefaultFileConfig()
	fc.Algorithms.GraphOfThoughts.BranchingFactor = 4
	fc.Algorithms.GraphOfThoughts.MaxNodes = 40
	fc.Algorithms.GraphOfThoughts.MergeThreshold = 0.8

	got := fc.GetDefaultGoTConfig()

	if got.BranchingFactor != 4 {
		t.Errorf("Expected BranchingFactor=4, got %d", got.BranchingFactor)
	}
	if got.MaxNodes != 40 {
		t.Errorf("Expected MaxNodes=40, got %d", got.MaxNodes)
	}
	if got.MergeThreshold != 0.8 {
		t.Errorf("Expected MergeThreshold=0.8, got %f", got.MergeThreshold)
	}
}

func TestGetDefaultReflexionConfig(t *testing.T) {
	fc := DefaultFileConfig()
	fc.Algorithms.Reflexion.MaxAttempts = 5
	fc.Memory.Path = "/tmp/test-memory.json"
	fc.Memory.MaxEpisodes = 200
	fc.Memory.TTLDays = 30

	ref := fc.GetDefaultReflexionConfig()

	if ref.MaxAttempts != 5 {
		t.Errorf("Expected MaxAttempts=5, got %d", ref.MaxAttempts)
	}
	if ref.MemoryPath != "/tmp/test-memory.json" {
		t.Errorf("Expected MemoryPath=/tmp/test-memory.json, got %s", ref.MemoryPath)
	}
	if ref.MaxEpisodes != 200 {
		t.Errorf("Expected MaxEpisodes=200, got %d", ref.MaxEpisodes)
	}
	expectedTTL := 30 * 24 * time.Hour
	if ref.EpisodeTTL != expectedTTL {
		t.Errorf("Expected EpisodeTTL=%v, got %v", expectedTTL, ref.EpisodeTTL)
	}
}

func TestGetDefaultDialecticConfig(t *testing.T) {
	fc := DefaultFileConfig()
	fc.Algorithms.Dialectic.MaxRounds = 10
	fc.Algorithms.Dialectic.ConfidenceTarget = 0.95
	fc.Algorithms.Dialectic.FastMode = true

	dia := fc.GetDefaultDialecticConfig()

	if dia.MaxRounds != 10 {
		t.Errorf("Expected MaxRounds=10, got %d", dia.MaxRounds)
	}
	if dia.ConfidenceTarget != 0.95 {
		t.Errorf("Expected ConfidenceTarget=0.95, got %f", dia.ConfidenceTarget)
	}
	if !dia.FastMode {
		t.Error("Expected FastMode=true")
	}
}

func TestWriteExampleConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "example-config.yaml")

	if err := WriteExampleConfig(configPath); err != nil {
		t.Fatalf("WriteExampleConfig failed: %v", err)
	}

	// Verify file exists and is readable
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read example config: %v", err)
	}

	if len(data) == 0 {
		t.Error("Example config file is empty")
	}

	// Should contain expected sections
	content := string(data)
	if !containsAll(content, "providers:", "algorithms:", "timeouts:", "rate_limiting:") {
		t.Error("Example config missing expected sections")
	}
}

// Helper function for tests
func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		found := false
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
