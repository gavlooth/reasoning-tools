package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// FileConfig represents the structure of the configuration file
type FileConfig struct {
	// API keys (centralized, supports env vars and file references)
	APIKeys APIKeysSettings `yaml:"api_keys"`

	// Provider settings
	Providers ProviderSettings `yaml:"providers"`

	// Algorithm defaults
	Algorithms AlgorithmSettings `yaml:"algorithms"`

	// Timeouts (in seconds)
	Timeouts TimeoutSettings `yaml:"timeouts"`

	// Rate limiting
	RateLimiting RateLimitSettings `yaml:"rate_limiting"`

	// Memory settings for reflexion
	Memory MemorySettings `yaml:"memory"`

	// Tool settings
	Tools ToolSettings `yaml:"tools"`
}

// APIKeysSettings configures API keys for all providers
type APIKeysSettings struct {
	Zai        string `yaml:"zai"`
	Groq       string `yaml:"groq"`
	DeepSeek   string `yaml:"deepseek"`
	OpenRouter string `yaml:"openrouter"`
	Together   string `yaml:"together"`
	OpenAI     string `yaml:"openai"`
	Anthropic  string `yaml:"anthropic"`
}

// ProviderSettings configures LLM providers
type ProviderSettings struct {
	Default    string                        `yaml:"default"`    // Default provider to use
	Fallbacks  []string                      `yaml:"fallbacks"`  // Fallback provider order
	Configs    map[string]FileProviderConfig `yaml:"configs"`    // Per-provider configuration
	Strategies ProviderStrategies            `yaml:"strategies"` // Multi-provider strategies
}

// ProviderStrategies configures provider selection strategies
type ProviderStrategies struct {
	Cascade    CascadeStrategy    `yaml:"cascade"`
	RoundRobin RoundRobinStrategy `yaml:"round_robin"`
}

// CascadeStrategy configures cheap->capable escalation
type CascadeStrategy struct {
	Enabled          bool   `yaml:"enabled"`
	Cheap            string `yaml:"cheap"`             // Provider for cheap operations
	CheapModel       string `yaml:"cheap_model"`       // Model for cheap provider
	Capable          string `yaml:"capable"`           // Provider for capable operations
	CapableModel     string `yaml:"capable_model"`     // Model for capable provider
	UpgradeAfterStep int    `yaml:"upgrade_after_step"` // Step number to upgrade (default: 2)
}

// RoundRobinStrategy configures round-robin provider rotation
type RoundRobinStrategy struct {
	Enabled  bool     `yaml:"enabled"`
	Providers []string `yaml:"providers"` // List of providers to rotate through
}

// FileProviderConfig represents configuration for a specific provider (from config file)
type FileProviderConfig struct {
	APIKey  string `yaml:"api_key"`  // Can reference env var with ${VAR_NAME}
	BaseURL string `yaml:"base_url"` // Override base URL
	Model   string `yaml:"model"`    // Default model
	Timeout int    `yaml:"timeout"`  // Timeout in seconds
}

// AlgorithmSettings configures default parameters for reasoning algorithms
type AlgorithmSettings struct {
	Sequential     SequentialSettings  `yaml:"sequential"`
	GraphOfThoughts GoTSettings        `yaml:"graph_of_thoughts"`
	Reflexion      ReflexionSettings   `yaml:"reflexion"`
	Dialectic      DialecticSettings   `yaml:"dialectic"`
}

// SequentialSettings configures sequential thinking defaults
type SequentialSettings struct {
	MaxThoughts int     `yaml:"max_thoughts"`
	Temperature float64 `yaml:"temperature"`
	// Provider strategy
	Strategy         string `yaml:"strategy"`           // "single", "cascade", "alternating", "round_robin"
	CheapProvider    string `yaml:"cheap_provider"`
	CheapModel       string `yaml:"cheap_model"`
	CapableProvider  string `yaml:"capable_provider"`
	CapableModel     string `yaml:"capable_model"`
	UpgradeAfterStep int    `yaml:"upgrade_after_step"`
	Providers        string `yaml:"providers"`          // Comma-separated for round_robin
}

// GoTSettings configures Graph of Thoughts defaults
type GoTSettings struct {
	BranchingFactor int     `yaml:"branching_factor"`
	MaxNodes        int     `yaml:"max_nodes"`
	MaxDepth        int     `yaml:"max_depth"`
	MergeThreshold  float64 `yaml:"merge_threshold"`
	EnableMerging   bool    `yaml:"enable_merging"`
	// Per-operation providers
	ThoughtProvider    string `yaml:"thought_provider"`
	ThoughtModel       string `yaml:"thought_model"`
	EvaluationProvider string `yaml:"evaluation_provider"`
	EvaluationModel    string `yaml:"evaluation_model"`
	MergeProvider      string `yaml:"merge_provider"`
	MergeModel         string `yaml:"merge_model"`
}

// ReflexionSettings configures Reflexion defaults
type ReflexionSettings struct {
	MaxAttempts   int  `yaml:"max_attempts"`
	LearnFromPast bool `yaml:"learn_from_past"`
	// Per-phase providers
	ReasoningProvider   string `yaml:"reasoning_provider"`
	ReasoningModel      string `yaml:"reasoning_model"`
	EvaluationProvider  string `yaml:"evaluation_provider"`
	EvaluationModel     string `yaml:"evaluation_model"`
	ReflectionProvider  string `yaml:"reflection_provider"`
	ReflectionModel     string `yaml:"reflection_model"`
}

// DialecticSettings configures Dialectic reasoning defaults
type DialecticSettings struct {
	MaxRounds        int     `yaml:"max_rounds"`
	ConfidenceTarget float64 `yaml:"confidence_target"`
	FastMode         bool    `yaml:"fast_mode"`
	// Per-role providers
	ThesisProvider     string `yaml:"thesis_provider"`
	ThesisModel        string `yaml:"thesis_model"`
	AntithesisProvider string `yaml:"antithesis_provider"`
	AntithesisModel    string `yaml:"antithesis_model"`
	SynthesisProvider  string `yaml:"synthesis_provider"`
	SynthesisModel     string `yaml:"synthesis_model"`
}

// TimeoutSettings configures various timeouts
type TimeoutSettings struct {
	OpenAI     int `yaml:"openai"`
	Anthropic  int `yaml:"anthropic"`
	Groq       int `yaml:"groq"`
	Ollama     int `yaml:"ollama"`
	DeepSeek   int `yaml:"deepseek"`
	OpenRouter int `yaml:"openrouter"`
	Zai        int `yaml:"zai"`
	Together   int `yaml:"together"`
	CodeExec   int `yaml:"code_exec"`
	WebFetch   int `yaml:"web_fetch"`
}

// RateLimitSettings configures rate limiting
type RateLimitSettings struct {
	MaxConcurrent int `yaml:"max_concurrent"` // Max concurrent LLM requests
	MaxTokensCap  int `yaml:"max_tokens_cap"` // Max tokens per request
}

// MemorySettings configures reflexion episodic memory
type MemorySettings struct {
	Path        string `yaml:"path"`         // Path to memory file
	MaxEpisodes int    `yaml:"max_episodes"` // Maximum episodes to keep
	TTLDays     int    `yaml:"ttl_days"`     // Episode TTL in days (0 = no expiry)
}

// ToolSettings configures built-in tools
type ToolSettings struct {
	EnableCodeExec bool     `yaml:"enable_code_exec"` // Enable code execution
	EnabledTools   []string `yaml:"enabled_tools"`    // List of enabled tools
}

// DefaultFileConfig returns default configuration
func DefaultFileConfig() *FileConfig {
	return &FileConfig{
		APIKeys: APIKeysSettings{},
		Providers: ProviderSettings{
			Default:   "",
			Fallbacks: []string{},
			Configs:   make(map[string]FileProviderConfig),
			Strategies: ProviderStrategies{
				Cascade: CascadeStrategy{
					Enabled:          false,
					UpgradeAfterStep: 2,
				},
				RoundRobin: RoundRobinStrategy{
					Enabled: false,
				},
			},
		},
		Algorithms: AlgorithmSettings{
			Sequential: SequentialSettings{
				MaxThoughts: 10,
				Temperature: 0.7,
			},
			GraphOfThoughts: GoTSettings{
				BranchingFactor: 3,
				MaxNodes:        30,
				MaxDepth:        8,
				MergeThreshold:  0.7,
				EnableMerging:   true,
			},
			Reflexion: ReflexionSettings{
				MaxAttempts:   3,
				LearnFromPast: true,
			},
			Dialectic: DialecticSettings{
				MaxRounds:        5,
				ConfidenceTarget: 0.85,
				FastMode:         false,
			},
		},
		Timeouts: TimeoutSettings{
			OpenAI:     120,
			Anthropic:  120,
			Groq:       120,
			Ollama:     300,
			DeepSeek:   120,
			OpenRouter: 120,
			Zai:        120,
			Together:   120,
			CodeExec:   10,
			WebFetch:   15,
		},
		RateLimiting: RateLimitSettings{
			MaxConcurrent: 2,
			MaxTokensCap:  8192,
		},
		Memory: MemorySettings{
			Path:        "",
			MaxEpisodes: 100,
			TTLDays:     0,
		},
		Tools: ToolSettings{
			EnableCodeExec: false,
			EnabledTools:   []string{},
		},
	}
}

// ConfigFilePaths returns the list of paths to search for config files
func ConfigFilePaths() []string {
	paths := []string{}

	// Current directory
	paths = append(paths, "reasoning-tools.yaml", "reasoning-tools.yml")

	// Home directory
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, ".config", "reasoning-tools", "config.yaml"),
			filepath.Join(home, ".config", "reasoning-tools", "config.yml"),
			filepath.Join(home, ".reasoning-tools.yaml"),
			filepath.Join(home, ".reasoning-tools.yml"),
		)
	}

	// XDG config directory
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		paths = append(paths,
			filepath.Join(xdgConfig, "reasoning-tools", "config.yaml"),
			filepath.Join(xdgConfig, "reasoning-tools", "config.yml"),
		)
	}

	// System config
	paths = append(paths,
		"/etc/reasoning-tools/config.yaml",
		"/etc/reasoning-tools/config.yml",
	)

	return paths
}

// LoadFileConfig loads configuration from a YAML file
// It searches in standard locations and returns the first found config
func LoadFileConfig() (*FileConfig, string, error) {
	// Check for explicit config file path
	if configPath := os.Getenv("REASONING_TOOLS_CONFIG"); configPath != "" {
		cfg, err := loadConfigFromPath(configPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to load config from %s: %w", configPath, err)
		}
		return cfg, configPath, nil
	}

	// Search standard paths
	for _, path := range ConfigFilePaths() {
		if _, err := os.Stat(path); err == nil {
			cfg, err := loadConfigFromPath(path)
			if err != nil {
				log.Printf("[CONFIG] Warning: found config file at %s but failed to load: %v", path, err)
				continue
			}
			return cfg, path, nil
		}
	}

	// No config file found, return defaults
	return DefaultFileConfig(), "", nil
}

// loadConfigFromPath loads configuration from a specific file path
func loadConfigFromPath(path string) (*FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultFileConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Expand environment variables in sensitive fields
	cfg.expandEnvVars()

	return cfg, nil
}

// expandEnvVars expands ${VAR_NAME} references in config values
func (fc *FileConfig) expandEnvVars() {
	// Expand API keys
	fc.APIKeys.Zai = os.ExpandEnv(fc.APIKeys.Zai)
	fc.APIKeys.Groq = os.ExpandEnv(fc.APIKeys.Groq)
	fc.APIKeys.DeepSeek = os.ExpandEnv(fc.APIKeys.DeepSeek)
	fc.APIKeys.OpenRouter = os.ExpandEnv(fc.APIKeys.OpenRouter)
	fc.APIKeys.Together = os.ExpandEnv(fc.APIKeys.Together)
	fc.APIKeys.OpenAI = os.ExpandEnv(fc.APIKeys.OpenAI)
	fc.APIKeys.Anthropic = os.ExpandEnv(fc.APIKeys.Anthropic)

	// Expand provider configs
	for name, cfg := range fc.Providers.Configs {
		if cfg.APIKey != "" {
			cfg.APIKey = os.ExpandEnv(cfg.APIKey)
			fc.Providers.Configs[name] = cfg
		}
	}
	if fc.Memory.Path != "" {
		fc.Memory.Path = os.ExpandEnv(fc.Memory.Path)
	}
}

// GetAPIKey returns the API key for a provider from multiple sources
// Priority: api_keys section > provider configs > environment variables
func (fc *FileConfig) GetAPIKey(provider string) string {
	provider = strings.ToLower(provider)

	// 1. Check api_keys section
	switch provider {
	case "zai", "glm", "zhipu":
		if fc.APIKeys.Zai != "" {
			return fc.APIKeys.Zai
		}
	case "groq":
		if fc.APIKeys.Groq != "" {
			return fc.APIKeys.Groq
		}
	case "deepseek":
		if fc.APIKeys.DeepSeek != "" {
			return fc.APIKeys.DeepSeek
		}
	case "openrouter":
		if fc.APIKeys.OpenRouter != "" {
			return fc.APIKeys.OpenRouter
		}
	case "together":
		if fc.APIKeys.Together != "" {
			return fc.APIKeys.Together
		}
	case "openai":
		if fc.APIKeys.OpenAI != "" {
			return fc.APIKeys.OpenAI
		}
	case "anthropic":
		if fc.APIKeys.Anthropic != "" {
			return fc.APIKeys.Anthropic
		}
	}

	// 2. Check provider configs
	if cfg, ok := fc.Providers.Configs[provider]; ok && cfg.APIKey != "" {
		return cfg.APIKey
	}

	// 3. Fall back to environment variables
	return getAPIKeyForProvider(provider)
}

// GetModel returns the default model for a provider
func (fc *FileConfig) GetModel(provider string) string {
	provider = strings.ToLower(provider)
	if cfg, ok := fc.Providers.Configs[provider]; ok && cfg.Model != "" {
		return cfg.Model
	}
	return ""
}

// BuildProvider creates a Provider from config settings
func (fc *FileConfig) BuildProvider(providerType, modelOverride string) (Provider, error) {
	apiKey := fc.GetAPIKey(providerType)
	model := modelOverride
	if model == "" {
		model = fc.GetModel(providerType)
	}

	var baseURL string
	if cfg, ok := fc.Providers.Configs[providerType]; ok {
		baseURL = cfg.BaseURL
	}

	cfg := ProviderConfig{
		Type:    providerType,
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	}

	return NewProvider(cfg)
}

// GetCascadeConfig returns cascade strategy configuration
func (fc *FileConfig) GetCascadeConfig() (cheapProvider, cheapModel, capableProvider, capableModel string, upgradeAfter int) {
	s := fc.Providers.Strategies.Cascade
	return s.Cheap, s.CheapModel, s.Capable, s.CapableModel, s.UpgradeAfterStep
}

// GetRoundRobinProviders returns round robin provider list
func (fc *FileConfig) GetRoundRobinProviders() []string {
	return fc.Providers.Strategies.RoundRobin.Providers
}

// IsCascadeEnabled returns true if cascade strategy is enabled
func (fc *FileConfig) IsCascadeEnabled() bool {
	s := fc.Providers.Strategies.Cascade
	return s.Enabled && s.Cheap != "" && s.Capable != ""
}

// IsRoundRobinEnabled returns true if round robin strategy is enabled
func (fc *FileConfig) IsRoundRobinEnabled() bool {
	s := fc.Providers.Strategies.RoundRobin
	return s.Enabled && len(s.Providers) > 0
}

// ApplyToConfig applies file configuration to the runtime Config
func (fc *FileConfig) ApplyToConfig(cfg *Config) {
	// Apply timeouts
	if fc.Timeouts.OpenAI > 0 {
		cfg.OpenAITimeout = time.Duration(fc.Timeouts.OpenAI) * time.Second
	}
	if fc.Timeouts.Anthropic > 0 {
		cfg.AnthropicTimeout = time.Duration(fc.Timeouts.Anthropic) * time.Second
	}
	if fc.Timeouts.Groq > 0 {
		cfg.GroqTimeout = time.Duration(fc.Timeouts.Groq) * time.Second
	}
	if fc.Timeouts.Ollama > 0 {
		cfg.OllamaTimeout = time.Duration(fc.Timeouts.Ollama) * time.Second
	}
	if fc.Timeouts.DeepSeek > 0 {
		cfg.DeepSeekTimeout = time.Duration(fc.Timeouts.DeepSeek) * time.Second
	}
	if fc.Timeouts.OpenRouter > 0 {
		cfg.OpenRouterTimeout = time.Duration(fc.Timeouts.OpenRouter) * time.Second
	}
	if fc.Timeouts.Zai > 0 {
		cfg.ZaiTimeout = time.Duration(fc.Timeouts.Zai) * time.Second
	}
	if fc.Timeouts.Together > 0 {
		cfg.TogetherTimeout = time.Duration(fc.Timeouts.Together) * time.Second
	}
	if fc.Timeouts.CodeExec > 0 {
		cfg.CodeExecTimeout = time.Duration(fc.Timeouts.CodeExec) * time.Second
	}
	if fc.Timeouts.WebFetch > 0 {
		cfg.WebFetchTimeout = time.Duration(fc.Timeouts.WebFetch) * time.Second
	}

	// Apply rate limiting
	if fc.RateLimiting.MaxConcurrent > 0 {
		cfg.MaxConcurrentLLMRequests = fc.RateLimiting.MaxConcurrent
	}
	if fc.RateLimiting.MaxTokensCap > 0 {
		cfg.MaxTokensCap = fc.RateLimiting.MaxTokensCap
	}
}

// GetDefaultGoTConfig returns GoT config with file settings applied
func (fc *FileConfig) GetDefaultGoTConfig() GoTConfig {
	cfg := DefaultGoTConfig()
	s := fc.Algorithms.GraphOfThoughts

	if s.BranchingFactor > 0 {
		cfg.BranchingFactor = s.BranchingFactor
	}
	if s.MaxNodes > 0 {
		cfg.MaxNodes = s.MaxNodes
	}
	if s.MaxDepth > 0 {
		cfg.MaxDepth = s.MaxDepth
	}
	if s.MergeThreshold > 0 {
		cfg.MergeThreshold = s.MergeThreshold
	}
	cfg.EnableMerging = s.EnableMerging

	return cfg
}

// GetDefaultReflexionConfig returns Reflexion config with file settings applied
func (fc *FileConfig) GetDefaultReflexionConfig() ReflexionConfig {
	cfg := DefaultReflexionConfig()
	s := fc.Algorithms.Reflexion

	if s.MaxAttempts > 0 {
		cfg.MaxAttempts = s.MaxAttempts
	}
	cfg.LearnFromPast = s.LearnFromPast

	// Apply memory settings
	if fc.Memory.Path != "" {
		cfg.MemoryPath = fc.Memory.Path
	}
	if fc.Memory.MaxEpisodes > 0 {
		cfg.MaxEpisodes = fc.Memory.MaxEpisodes
	}
	if fc.Memory.TTLDays > 0 {
		cfg.EpisodeTTL = time.Duration(fc.Memory.TTLDays) * 24 * time.Hour
	}

	return cfg
}

// GetDefaultDialecticConfig returns Dialectic config with file settings applied
func (fc *FileConfig) GetDefaultDialecticConfig() DialecticConfig {
	cfg := DefaultDialecticConfig()
	s := fc.Algorithms.Dialectic

	if s.MaxRounds > 0 {
		cfg.MaxRounds = s.MaxRounds
	}
	if s.ConfidenceTarget > 0 {
		cfg.ConfidenceTarget = s.ConfidenceTarget
	}
	cfg.FastMode = s.FastMode

	return cfg
}

// WriteExampleConfig writes an example configuration file
func WriteExampleConfig(path string) error {
	exampleConfig := `# Reasoning Tools Configuration
# This file configures the reasoning-tools MCP server

# Provider settings
providers:
  # Default provider (auto-detected if not set)
  default: ""

  # Fallback providers to try on failure (in order)
  fallbacks:
    - groq
    - deepseek
    - ollama

  # Per-provider configuration
  # configs:
  #   openai:
  #     api_key: ${OPENAI_API_KEY}  # Use env var
  #     model: gpt-4o-mini
  #   anthropic:
  #     api_key: ${ANTHROPIC_API_KEY}
  #     model: claude-sonnet-4-6

# Algorithm defaults
algorithms:
  sequential:
    max_thoughts: 10
    temperature: 0.7

  graph_of_thoughts:
    branching_factor: 3
    max_nodes: 30
    max_depth: 8
    merge_threshold: 0.7
    enable_merging: true

  reflexion:
    max_attempts: 3
    learn_from_past: true

  dialectic:
    max_rounds: 5
    confidence_target: 0.85
    fast_mode: false

# Timeouts (in seconds)
timeouts:
  openai: 120
  anthropic: 120
  groq: 120
  ollama: 300
  deepseek: 120
  openrouter: 120
  zai: 120
  together: 120
  code_exec: 10
  web_fetch: 15

# Rate limiting
rate_limiting:
  max_concurrent: 2
  max_tokens_cap: 8192

# Reflexion memory settings
memory:
  # path: ~/.local/share/reasoning-tools/memory.json
  max_episodes: 100
  ttl_days: 0  # 0 = no expiry

# Tool settings
tools:
  enable_code_exec: false
  # enabled_tools:
  #   - calculator
  #   - web_fetch
  #   - string_ops
`

	return os.WriteFile(path, []byte(exampleConfig), 0644)
}

// Global file config (loaded once)
var (
	globalFileConfig     *FileConfig
	globalFileConfigPath string
	fileConfigLoaded     bool
)

// GetFileConfig returns the loaded file configuration
func GetFileConfig() (*FileConfig, string) {
	if !fileConfigLoaded {
		var err error
		globalFileConfig, globalFileConfigPath, err = LoadFileConfig()
		if err != nil {
			log.Printf("[CONFIG] Error loading config file: %v", err)
			globalFileConfig = DefaultFileConfig()
		} else if globalFileConfigPath != "" {
			log.Printf("[CONFIG] Loaded configuration from %s", globalFileConfigPath)
		}
		fileConfigLoaded = true
	}
	return globalFileConfig, globalFileConfigPath
}
