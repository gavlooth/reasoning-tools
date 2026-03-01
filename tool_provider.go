package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func getProviderFromArgsForTool(args map[string]interface{}, toolName string) (Provider, error) {
	providerType := ""
	if p, ok := args["provider"].(string); ok && p != "" {
		providerType = p
	}
	if providerType == "" {
		if env := os.Getenv(toolEnvKey(toolName, "PROVIDER")); env != "" {
			providerType = env
		}
	}

	model := ""
	if m, ok := args["model"].(string); ok && m != "" {
		model = m
	}
	if model == "" {
		if env := os.Getenv(toolEnvKey(toolName, "MODEL")); env != "" {
			model = env
		}
	}

	if providerType == "" {
		providerType = os.Getenv("LLM_PROVIDER")
		if providerType == "" {
			providerType = detectProviderFromEnv()
		}
	}

	primary, err := buildProvider(providerType, model)
	if err != nil {
		return nil, err
	}

	fallbackTypes := parseFallbackProviders(args, toolName)
	if len(fallbackTypes) == 0 {
		return primary, nil
	}

	var providers []Provider
	providers = append(providers, primary)
	for _, fallbackType := range fallbackTypes {
		if strings.EqualFold(fallbackType, providerType) {
			continue
		}
		fallbackProvider, err := buildProvider(fallbackType, "")
		if err != nil {
			return nil, err
		}
		providers = append(providers, fallbackProvider)
	}

	return NewFallbackProvider(providers), nil
}

// getProviderSelectorForTool builds a ProviderSelector for multi-provider reasoning
func getProviderSelectorForTool(args map[string]interface{}, toolName string) (*ProviderSelector, error) {
	// Get strategy (default: check config, then single)
	strategy := ""
	if s, ok := args["provider_strategy"].(string); ok && s != "" {
		strategy = strings.ToLower(s)
	}

	// Check config file for default strategy
	fileConfig, _ := GetFileConfig()
	if strategy == "" && fileConfig != nil {
		// Check if cascade is enabled in config
		if fileConfig.IsCascadeEnabled() {
			strategy = "cascade"
		} else if fileConfig.IsRoundRobinEnabled() {
			strategy = "round_robin"
		}
	}
	if strategy == "" {
		strategy = "single"
	}

	// For single strategy, use existing provider logic
	if strategy == "single" {
		primary, err := getProviderFromArgsForTool(args, toolName)
		if err != nil {
			return nil, err
		}
		return NewProviderSelector(primary, nil, nil, "single", 0), nil
	}

	// For round_robin strategy, parse providers list
	if strategy == "round_robin" || strategy == "custom" {
		providers, err := parseProvidersList(args, toolName)
		if err != nil {
			return nil, err
		}

		// If no providers in args, check config
		if len(providers) == 0 && fileConfig != nil && fileConfig.IsRoundRobinEnabled() {
			providers, err = buildProvidersFromList(fileConfig.GetRoundRobinProviders())
			if err != nil {
				return nil, err
			}
		}

		if len(providers) == 0 {
			// Fallback to single provider
			primary, err := getProviderFromArgsForTool(args, toolName)
			if err != nil {
				return nil, err
			}
			return NewProviderSelector(primary, nil, nil, "single", 0), nil
		}

		// Parse custom step mapping if provided
		var stepMapping map[int]int
		if strategy == "custom" {
			if mappingStr, ok := args["step_mapping"].(string); ok && mappingStr != "" {
				stepMapping = parseStepMappingToInt(mappingStr, len(providers))
			}
		}

		return NewProviderSelectorWithProviders(providers, strategy, stepMapping), nil
	}

	// For cascade/alternating strategies, use cheap/capable pattern
	// Get primary provider (fallback if cheap/capable not specified)
	primary, err := getProviderFromArgsForTool(args, toolName)
	if err != nil {
		return nil, err
	}

	// Get cheap provider
	var cheap Provider
	if cheapType, ok := args["cheap_provider"].(string); ok && cheapType != "" {
		cheapModel, _ := args["cheap_model"].(string)
		cheap, err = buildProvider(cheapType, cheapModel)
		if err != nil {
			return nil, fmt.Errorf("failed to build cheap provider: %w", err)
		}
	}

	// Get capable provider
	var capable Provider
	if capableType, ok := args["capable_provider"].(string); ok && capableType != "" {
		capableModel, _ := args["capable_model"].(string)
		capable, err = buildProvider(capableType, capableModel)
		if err != nil {
			return nil, fmt.Errorf("failed to build capable provider: %w", err)
		}
	}

	// If not specified in args, check config file
	if cheap == nil && capable == nil && fileConfig != nil && fileConfig.IsCascadeEnabled() {
		cheapType, cheapModel, capableType, capableModel, _ := fileConfig.GetCascadeConfig()
		if cheapType != "" {
			cheap, err = buildProvider(cheapType, cheapModel)
			if err != nil {
				return nil, fmt.Errorf("failed to build cheap provider from config: %w", err)
			}
		}
		if capableType != "" {
			capable, err = buildProvider(capableType, capableModel)
			if err != nil {
				return nil, fmt.Errorf("failed to build capable provider from config: %w", err)
			}
		}
	}

	// If still no cheap/capable, use smart defaults
	if cheap == nil && capable == nil {
		cheap, capable = getDefaultCheapAndCapable(primary)
	}

	// Get upgrade threshold for cascade strategy
	upgradeAfter := 2
	if u, ok := args["upgrade_after_step"].(float64); ok && u > 0 {
		upgradeAfter = int(u)
	} else if fileConfig != nil {
		_, _, _, _, cfgUpgradeAfter := fileConfig.GetCascadeConfig()
		if cfgUpgradeAfter > 0 {
			upgradeAfter = cfgUpgradeAfter
		}
	}

	return NewProviderSelector(primary, cheap, capable, strategy, upgradeAfter), nil
}

// buildProvidersFromList creates providers from a list of provider names
func buildProvidersFromList(providerNames []string) ([]Provider, error) {
	var providers []Provider
	for _, name := range providerNames {
		p, err := buildProvider(name, "")
		if err != nil {
			return nil, fmt.Errorf("failed to build provider %s: %w", name, err)
		}
		providers = append(providers, p)
	}
	return providers, nil
}

// parseProvidersList parses a list of providers from args
// Supports formats:
//   - "providers": "deepseek,zai,groq"
//   - "providers": "deepseek:deepseek-chat,zai:glm-4.7,groq:llama-3.3-70b"
//   - "providers": ["deepseek", "zai", "groq"] (JSON array)
func parseProvidersList(args map[string]interface{}, toolName string) ([]Provider, error) {
	var providers []Provider

	// Check for "providers" parameter (comma-separated or JSON array)
	if providersStr, ok := args["providers"].(string); ok && providersStr != "" {
		return parseProviderSpecs(providersStr)
	}

	// Check for JSON array
	if providersRaw, ok := args["providers"].([]interface{}); ok {
		for _, p := range providersRaw {
			if spec, ok := p.(string); ok && spec != "" {
				provider, err := parseProviderSpec(spec)
				if err != nil {
					return nil, fmt.Errorf("failed to parse provider spec '%s': %w", spec, err)
				}
				providers = append(providers, provider)
			}
		}
		if len(providers) > 0 {
			return providers, nil
		}
	}

	return nil, nil
}

// parseProviderSpecs parses comma-separated provider specs like "deepseek,zai:glm-4.7,groq"
func parseProviderSpecs(s string) ([]Provider, error) {
	var providers []Provider
	parts := strings.Split(s, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		provider, err := parseProviderSpec(part)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	return providers, nil
}

// parseProviderSpec parses a single provider spec like "deepseek" or "deepseek:deepseek-reasoner"
func parseProviderSpec(spec string) (Provider, error) {
	parts := strings.SplitN(spec, ":", 2)
	providerType := strings.TrimSpace(parts[0])
	model := ""
	if len(parts) > 1 {
		model = strings.TrimSpace(parts[1])
	}
	return buildProvider(providerType, model)
}

// parseStepMappingToInt parses step mapping and converts provider names to indices
func parseStepMappingToInt(s string, numProviders int) map[int]int {
	result := make(map[int]int)

	// Try JSON first: {"1": "deepseek", "2": "zai"}
	if strings.HasPrefix(s, "{") {
		var jsonMap map[string]string
		if err := json.Unmarshal([]byte(s), &jsonMap); err == nil {
			for k, v := range jsonMap {
				if num, err := strconv.Atoi(k); err == nil {
					if idx := providerNameToIndex(v, numProviders); idx >= 0 {
						result[num] = idx
					}
				}
			}
			return result
		}
	}

	// Parse comma-separated format: "1:deepseek,2:zai,3:groq"
	parts := strings.Split(s, ",")
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])

		if num, err := strconv.Atoi(key); err == nil {
			if idx := providerNameToIndex(val, numProviders); idx >= 0 {
				result[num] = idx
			}
		}
	}

	return result
}

// providerNameToIndex converts a provider name to an index (0-based)
// For round_robin, we use the order in the providers list
func providerNameToIndex(name string, numProviders int) int {
	// For now, treat as index directly if it's a number
	if idx, err := strconv.Atoi(name); err == nil && idx >= 0 && idx < numProviders {
		return idx
	}
	return -1
}

// getDefaultCheapAndCapable returns smart defaults for cheap and capable providers
func getDefaultCheapAndCapable(primary Provider) (cheap, capable Provider) {
	// Default capable to primary
	capable = primary

	// Try to find a cheap provider (prefer ollama, then groq)
	cheapProviders := []string{"ollama", "groq"}
	for _, p := range cheapProviders {
		if provider, err := buildProvider(p, ""); err == nil {
			cheap = provider
			return
		}
	}

	// If no cheap provider available, use primary for both
	cheap = primary
	return
}

// parseStepMapping parses a step mapping string like "1:cheap,2:cheap,3+:capable"
func parseStepMapping(s string) map[int]string {
	result := make(map[int]string)

	// Try JSON first
	if strings.HasPrefix(s, "{") {
		var jsonMap map[string]string
		if err := json.Unmarshal([]byte(s), &jsonMap); err == nil {
			for k, v := range jsonMap {
				// Handle "3+" notation
				if strings.HasSuffix(k, "+") {
					numStr := strings.TrimSuffix(k, "+")
					if num, err := strconv.Atoi(numStr); err == nil {
						result[num] = v
					}
				} else if num, err := strconv.Atoi(k); err == nil {
					result[num] = v
				}
			}
			return result
		}
	}

	// Parse comma-separated format: "1:cheap,2:cheap,3+:capable"
	parts := strings.Split(s, ",")
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])

		// Handle "N+" notation
		if strings.HasSuffix(key, "+") {
			numStr := strings.TrimSuffix(key, "+")
			if num, err := strconv.Atoi(numStr); err == nil {
				result[num] = val
			}
		} else if num, err := strconv.Atoi(key); err == nil {
			result[num] = val
		}
	}

	return result
}

func buildProvider(providerType, model string) (Provider, error) {
	// Try config file first
	fileConfig, _ := GetFileConfig()
	if fileConfig != nil {
		return fileConfig.BuildProvider(providerType, model)
	}

	// Fall back to environment variables
	cfg := ProviderConfig{
		Type:    providerType,
		APIKey:  getAPIKeyForProvider(providerType),
		BaseURL: os.Getenv("LLM_BASE_URL"),
		Model:   model,
	}

	if providerType == "zai" || providerType == "glm" || providerType == "zhipu" {
		if url := os.Getenv("ZAI_BASE_URL"); url != "" {
			cfg.BaseURL = url
		}
		if m := os.Getenv("GLM_MODEL"); m != "" && cfg.Model == "" {
			cfg.Model = m
		}
	}

	return NewProvider(cfg)
}

func parseFallbackProviders(args map[string]interface{}, toolName string) []string {
	var raw string
	if val, ok := args["fallback_providers"].(string); ok && val != "" {
		raw = val
	} else if env := os.Getenv(toolEnvKey(toolName, "FALLBACKS")); env != "" {
		raw = env
	} else if env := os.Getenv("LLM_FALLBACKS"); env != "" {
		raw = env
	}
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var providers []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			providers = append(providers, p)
		}
	}
	return providers
}

func toolEnvKey(toolName, suffix string) string {
	key := strings.ToUpper(toolName)
	re := regexp.MustCompile(`[^A-Z0-9]+`)
	key = re.ReplaceAllString(key, "_")
	key = strings.Trim(key, "_")
	return fmt.Sprintf("%s_%s", key, suffix)
}
