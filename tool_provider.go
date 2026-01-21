package main

import (
	"fmt"
	"os"
	"regexp"
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

func buildProvider(providerType, model string) (Provider, error) {
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
