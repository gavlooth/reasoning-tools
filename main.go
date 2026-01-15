package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Create MCP server
	s := server.NewMCPServer(
		"reasoning-tools",
		"3.2.0",
		server.WithToolCapabilities(true),
	)

	// Register simple sequential thinking tool
	simpleTool := mcp.NewTool("sequential_thinking",
		mcp.WithDescription("Simple sequential chain-of-thought reasoning. "+
			"Good for straightforward problems. Uses linear thinking without branching."),
		mcp.WithString("problem",
			mcp.Required(),
			mcp.Description("The problem or question to think through"),
		),
		mcp.WithNumber("max_thoughts",
			mcp.Description("Maximum number of thinking steps (default: 10)"),
		),
		mcp.WithString("provider",
			mcp.Description("LLM provider: openai, anthropic, groq, ollama, deepseek, openrouter, zai, together (auto-detected if not set)"),
		),
		mcp.WithString("model",
			mcp.Description("Model to use (provider-specific, uses default if not set)"),
		),
		mcp.WithBoolean("stream",
			mcp.Description("Include streaming event log in output (default: false)"),
		),
	)
	s.AddTool(simpleTool, handleSequentialThink)

	// Register Graph of Thoughts tool (replaces Tree of Thoughts)
	gotTool := mcp.NewTool("graph_of_thoughts",
		mcp.WithDescription("Graph of Thoughts reasoning with path merging and optional tool integration. "+
			"Unlike Tree of Thoughts, GoT can merge similar reasoning paths, combining insights. "+
			"Better for problems where multiple approaches might converge on the same insight. "+
			"When tools are enabled, can use calculator, code execution, and web fetch during reasoning."),
		mcp.WithString("problem",
			mcp.Required(),
			mcp.Description("The problem or question to solve"),
		),
		mcp.WithNumber("branching_factor",
			mcp.Description("Number of candidate thoughts per expansion (default: 3)"),
		),
		mcp.WithNumber("max_nodes",
			mcp.Description("Maximum nodes to explore (default: 30)"),
		),
		mcp.WithNumber("max_depth",
			mcp.Description("Maximum reasoning depth (default: 8)"),
		),
		mcp.WithBoolean("enable_merging",
			mcp.Description("Allow merging similar paths (default: true)"),
		),
		mcp.WithBoolean("enable_tools",
			mcp.Description("Enable tool usage during reasoning (default: false)"),
		),
		mcp.WithNumber("max_tool_calls",
			mcp.Description("Maximum tool calls during reasoning (default: 10)"),
		),
		mcp.WithString("enabled_tools",
			mcp.Description("Comma-separated list of tools: calculator,code_exec,web_fetch,string_ops (default: all)"),
		),
		mcp.WithString("provider",
			mcp.Description("LLM provider: openai, anthropic, groq, ollama, deepseek, openrouter, zai, together"),
		),
		mcp.WithString("model",
			mcp.Description("Model to use (provider-specific)"),
		),
		mcp.WithBoolean("stream",
			mcp.Description("Include streaming event log in output (default: false)"),
		),
	)
	s.AddTool(gotTool, handleGraphOfThoughts)

	// Register Reflexion tool (learning from failures)
	reflexionTool := mcp.NewTool("reflexion",
		mcp.WithDescription("Reflexion reasoning with episodic memory and optional tool integration. "+
			"Makes multiple attempts, learns from failures, and applies lessons from past similar problems. "+
			"Best for problems where you expect initial attempts might fail but want to learn and improve. "+
			"When tools are enabled, can use calculator, code execution, and web fetch during reasoning."),
		mcp.WithString("problem",
			mcp.Required(),
			mcp.Description("The problem or question to solve"),
		),
		mcp.WithNumber("max_attempts",
			mcp.Description("Maximum reasoning attempts (default: 3)"),
		),
		mcp.WithBoolean("learn_from_past",
			mcp.Description("Query lessons from similar past problems (default: true)"),
		),
		mcp.WithBoolean("enable_tools",
			mcp.Description("Enable tool usage during reasoning (default: false)"),
		),
		mcp.WithNumber("max_tool_calls",
			mcp.Description("Maximum tool calls per attempt (default: 5)"),
		),
		mcp.WithString("enabled_tools",
			mcp.Description("Comma-separated list of tools: calculator,code_exec,web_fetch,string_ops (default: all)"),
		),
		mcp.WithString("provider",
			mcp.Description("LLM provider: openai, anthropic, groq, ollama, deepseek, openrouter, zai, together"),
		),
		mcp.WithString("model",
			mcp.Description("Model to use (provider-specific)"),
		),
		mcp.WithBoolean("stream",
			mcp.Description("Include streaming event log in output (default: false)"),
		),
	)
	s.AddTool(reflexionTool, handleReflexion)

	// Register Dialectical Reasoning tool (Debate + Chain of Verification)
	dialecticTool := mcp.NewTool("dialectic_reason",
		mcp.WithDescription("Dialectical reasoning combining Debate and Chain of Verification with optional tool-backed fact-checking. "+
			"Uses thesis-antithesis-synthesis cycles where each claim is rigorously verified. "+
			"Best for controversial topics, complex decisions, or when you need high confidence. "+
			"When tools are enabled, uses calculator, web fetch etc. to fact-check claims."),
		mcp.WithString("problem",
			mcp.Required(),
			mcp.Description("The problem or question to reason about"),
		),
		mcp.WithNumber("max_rounds",
			mcp.Description("Maximum debate rounds (default: 5)"),
		),
		mcp.WithNumber("confidence_target",
			mcp.Description("Stop when synthesis reaches this confidence 0-1 (default: 0.85)"),
		),
		mcp.WithBoolean("enable_tools",
			mcp.Description("Enable tool-backed verification (default: false)"),
		),
		mcp.WithNumber("max_tool_calls",
			mcp.Description("Maximum tool calls for verification (default: 10)"),
		),
		mcp.WithString("enabled_tools",
			mcp.Description("Comma-separated list of tools: calculator,code_exec,web_fetch,string_ops (default: all)"),
		),
		mcp.WithString("provider",
			mcp.Description("LLM provider: openai, anthropic, groq, ollama, deepseek, openrouter, zai, together"),
		),
		mcp.WithString("model",
			mcp.Description("Model to use (provider-specific)"),
		),
		mcp.WithBoolean("stream",
			mcp.Description("Include streaming event log in output (default: false)"),
		),
	)
	s.AddTool(dialecticTool, handleDialecticReason)

	// Register provider list tool
	listTool := mcp.NewTool("list_providers",
		mcp.WithDescription("List available LLM providers and their configuration"),
	)
	s.AddTool(listTool, handleListProviders)

	// Register memory stats tool
	memoryTool := mcp.NewTool("memory_stats",
		mcp.WithDescription("Show reflexion episodic memory statistics"),
	)
	s.AddTool(memoryTool, handleMemoryStats)

	// Start stdio server
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func handleSequentialThink(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments format"), nil
	}

	problem, ok := args["problem"].(string)
	if !ok || problem == "" {
		return mcp.NewToolResultError("problem parameter is required"), nil
	}

	maxThoughts := 10
	if mt, ok := args["max_thoughts"].(float64); ok {
		maxThoughts = int(mt)
	}

	includeStream := false
	if s, ok := args["stream"].(bool); ok {
		includeStream = s
	}

	// Get provider
	provider, err := getProviderFromArgs(args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Provider error: %v", err)), nil
	}

	// Setup streaming
	sm := NewStreamingManager("sequential_thinking")

	// Create client and run sequential thinking
	client := &SequentialClient{provider: provider}
	result, err := client.Think(ctx, problem, maxThoughts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Thinking failed: %v", err)), nil
	}

	// Add steps to stream log
	for i, step := range result.Steps {
		sm.AddProgressEvent(ProgressUpdate{
			Type:    "thought",
			NodeID:  fmt.Sprintf("t%d", i+1),
			Thought: truncateStr(step.Thought, 100),
			Depth:   i + 1,
		})
	}
	if result.Success {
		sm.AddProgressEvent(ProgressUpdate{
			Type:        "solution",
			FinalAnswer: result.FinalAnswer,
			IsSolution:  true,
		})
	}

	// Format output
	var output string
	if includeStream {
		wrapped := WrapWithStreaming(result, sm, true)
		outputBytes, _ := json.MarshalIndent(wrapped, "", "  ")
		output = string(outputBytes)
	} else {
		outputBytes, _ := json.MarshalIndent(result, "", "  ")
		output = string(outputBytes)
	}

	return mcp.NewToolResultText(output), nil
}

func handleGraphOfThoughts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments format"), nil
	}

	problem, ok := args["problem"].(string)
	if !ok || problem == "" {
		return mcp.NewToolResultError("problem parameter is required"), nil
	}

	// Get provider
	provider, err := getProviderFromArgs(args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Provider error: %v", err)), nil
	}

	includeStream := false
	if s, ok := args["stream"].(bool); ok {
		includeStream = s
	}

	// Build config
	config := DefaultGoTConfig()
	if bf, ok := args["branching_factor"].(float64); ok {
		config.BranchingFactor = int(bf)
	}
	if mn, ok := args["max_nodes"].(float64); ok {
		config.MaxNodes = int(mn)
	}
	if md, ok := args["max_depth"].(float64); ok {
		config.MaxDepth = int(md)
	}
	if em, ok := args["enable_merging"].(bool); ok {
		config.EnableMerging = em
	}
	if et, ok := args["enable_tools"].(bool); ok {
		config.EnableTools = et
	}
	if mtc, ok := args["max_tool_calls"].(float64); ok {
		config.MaxToolCalls = int(mtc)
	}
	if tools, ok := args["enabled_tools"].(string); ok && tools != "" {
		toolList := strings.Split(tools, ",")
		for i := range toolList {
			toolList[i] = strings.TrimSpace(toolList[i])
		}
		config.EnabledTools = toolList
	}

	// Setup streaming
	sm := NewStreamingManager("graph_of_thoughts")

	// Run Graph of Thoughts
	got := NewGraphOfThoughts(provider, config)
	got.SetProgressCallback(func(update ProgressUpdate) {
		sm.AddProgressEvent(update)
	})

	result, err := got.Solve(ctx, problem)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("GoT failed: %v", err)), nil
	}

	// Format output
	var output string
	if includeStream {
		output = FormatGoTResult(result)
		output += "\n\n## Stream Log\n\n"
		output += sm.FormatCompact()
	} else {
		output = FormatGoTResult(result)
	}

	return mcp.NewToolResultText(output), nil
}

func handleReflexion(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments format"), nil
	}

	problem, ok := args["problem"].(string)
	if !ok || problem == "" {
		return mcp.NewToolResultError("problem parameter is required"), nil
	}

	// Get provider
	provider, err := getProviderFromArgs(args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Provider error: %v", err)), nil
	}

	includeStream := false
	if s, ok := args["stream"].(bool); ok {
		includeStream = s
	}

	// Build config
	config := DefaultReflexionConfig()
	if ma, ok := args["max_attempts"].(float64); ok {
		config.MaxAttempts = int(ma)
	}
	if lp, ok := args["learn_from_past"].(bool); ok {
		config.LearnFromPast = lp
	}
	if et, ok := args["enable_tools"].(bool); ok {
		config.EnableTools = et
	}
	if mtc, ok := args["max_tool_calls"].(float64); ok {
		config.MaxToolCalls = int(mtc)
	}
	if tools, ok := args["enabled_tools"].(string); ok && tools != "" {
		toolList := strings.Split(tools, ",")
		for i := range toolList {
			toolList[i] = strings.TrimSpace(toolList[i])
		}
		config.EnabledTools = toolList
	}

	// Setup streaming
	sm := NewStreamingManager("reflexion")

	// Run Reflexion
	reflexion := NewReflexion(provider, config)
	reflexion.SetProgressCallback(func(update ProgressUpdate) {
		sm.AddProgressEvent(update)
	})

	result, err := reflexion.Reason(ctx, problem)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Reflexion failed: %v", err)), nil
	}

	// Format output
	var output string
	if includeStream {
		output = FormatReflexionResult(result)
		output += "\n\n## Stream Log\n\n"
		output += sm.FormatCompact()
	} else {
		output = FormatReflexionResult(result)
	}

	return mcp.NewToolResultText(output), nil
}

func handleDialecticReason(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments format"), nil
	}

	problem, ok := args["problem"].(string)
	if !ok || problem == "" {
		return mcp.NewToolResultError("problem parameter is required"), nil
	}

	// Get provider
	provider, err := getProviderFromArgs(args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Provider error: %v", err)), nil
	}

	includeStream := false
	if s, ok := args["stream"].(bool); ok {
		includeStream = s
	}

	// Build config
	config := DefaultDialecticConfig()
	if mr, ok := args["max_rounds"].(float64); ok {
		config.MaxRounds = int(mr)
	}
	if ct, ok := args["confidence_target"].(float64); ok {
		config.ConfidenceTarget = ct
	}
	if et, ok := args["enable_tools"].(bool); ok {
		config.EnableTools = et
	}
	if mtc, ok := args["max_tool_calls"].(float64); ok {
		config.MaxToolCalls = int(mtc)
	}
	if tools, ok := args["enabled_tools"].(string); ok && tools != "" {
		toolList := strings.Split(tools, ",")
		for i := range toolList {
			toolList[i] = strings.TrimSpace(toolList[i])
		}
		config.EnabledTools = toolList
	}

	// Setup streaming
	sm := NewStreamingManager("dialectic_reason")

	// Run dialectical reasoning
	reasoner := NewDialecticalReasoner(provider, config)
	reasoner.SetProgressCallback(func(update ProgressUpdate) {
		sm.AddProgressEvent(update)
	})
	result, err := reasoner.Reason(ctx, problem)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Dialectic reasoning failed: %v", err)), nil
	}

	// Add steps to stream log
	for _, step := range result.Steps {
		sm.AddProgressEvent(ProgressUpdate{
			Type:    "thought",
			Message: fmt.Sprintf("Round %d: Thesis", step.Round),
			Score:   step.Thesis.Verification.Score,
		})
		sm.AddProgressEvent(ProgressUpdate{
			Type:    "thought",
			Message: fmt.Sprintf("Round %d: Antithesis", step.Round),
			Score:   step.Antithesis.Verification.Score,
		})
		sm.AddProgressEvent(ProgressUpdate{
			Type:       "evaluation",
			Message:    fmt.Sprintf("Round %d: Synthesis", step.Round),
			Score:      step.Synthesis.Verification.Score,
			IsSolution: step.Resolved,
		})
	}
	if result.Success {
		sm.AddProgressEvent(ProgressUpdate{
			Type:        "solution",
			FinalAnswer: result.FinalAnswer,
			Score:       result.Confidence,
		})
	}

	// Format output
	var output string
	if includeStream {
		output = FormatDialecticResult(result)
		output += "\n\n## Stream Log\n\n"
		output += sm.FormatCompact()
	} else {
		output = FormatDialecticResult(result)
	}

	return mcp.NewToolResultText(output), nil
}

func handleListProviders(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	providers := []map[string]interface{}{
		{
			"name":          "zai",
			"aliases":       []string{"glm", "zhipu"},
			"env_key":       "ZAI_API_KEY or GLM_API_KEY",
			"default_model": "glm-4",
			"base_url":      "https://open.bigmodel.cn/api/paas/v4",
		},
		{
			"name":          "openai",
			"env_key":       "OPENAI_API_KEY",
			"default_model": "gpt-4o-mini",
			"base_url":      "https://api.openai.com/v1",
		},
		{
			"name":          "anthropic",
			"env_key":       "ANTHROPIC_API_KEY",
			"default_model": "claude-3-haiku-20240307",
			"base_url":      "https://api.anthropic.com/v1",
		},
		{
			"name":          "groq",
			"env_key":       "GROQ_API_KEY",
			"default_model": "llama-3.1-70b-versatile",
			"base_url":      "https://api.groq.com/openai/v1",
			"note":          "Very fast inference",
		},
		{
			"name":          "deepseek",
			"env_key":       "DEEPSEEK_API_KEY",
			"default_model": "deepseek-chat",
			"base_url":      "https://api.deepseek.com/v1",
			"note":          "Good for reasoning, cheap",
		},
		{
			"name":          "openrouter",
			"env_key":       "OPENROUTER_API_KEY",
			"default_model": "meta-llama/llama-3.1-70b-instruct",
			"base_url":      "https://openrouter.ai/api/v1",
			"note":          "Aggregator - access many models",
		},
		{
			"name":          "together",
			"env_key":       "TOGETHER_API_KEY",
			"default_model": "meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo",
			"base_url":      "https://api.together.xyz/v1",
		},
		{
			"name":          "ollama",
			"env_key":       "(none - local)",
			"default_model": "llama3.1",
			"base_url":      "http://localhost:11434",
			"note":          "Local inference",
		},
	}

	// Check which are configured
	for i := range providers {
		providers[i]["configured"] = isProviderConfigured(providers[i]["name"].(string))
	}

	output, _ := json.MarshalIndent(providers, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func handleMemoryStats(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Create a temporary reflexion instance to get memory stats
	provider, _ := NewProviderFromEnv()
	if provider == nil {
		// Use a dummy provider just for memory access
		provider = &OpenAIProvider{name: "dummy"}
	}

	config := DefaultReflexionConfig()
	reflexion := NewReflexion(provider, config)
	stats := reflexion.GetMemoryStats()

	output, _ := json.MarshalIndent(stats, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func getProviderFromArgs(args map[string]interface{}) (Provider, error) {
	providerType := ""
	if p, ok := args["provider"].(string); ok {
		providerType = p
	}

	model := ""
	if m, ok := args["model"].(string); ok {
		model = m
	}

	if providerType == "" {
		// Use environment-based detection
		return NewProviderFromEnv()
	}

	cfg := ProviderConfig{
		Type:    providerType,
		APIKey:  getAPIKeyForProvider(providerType),
		BaseURL: os.Getenv("LLM_BASE_URL"),
		Model:   model,
	}

	// Check for provider-specific env overrides
	if providerType == "zai" || providerType == "glm" {
		if url := os.Getenv("ZAI_BASE_URL"); url != "" {
			cfg.BaseURL = url
		}
		if m := os.Getenv("GLM_MODEL"); m != "" && model == "" {
			cfg.Model = m
		}
	}

	return NewProvider(cfg)
}

func isProviderConfigured(name string) bool {
	switch name {
	case "zai", "glm":
		return os.Getenv("ZAI_API_KEY") != "" || os.Getenv("GLM_API_KEY") != ""
	case "openai":
		return os.Getenv("OPENAI_API_KEY") != ""
	case "anthropic":
		return os.Getenv("ANTHROPIC_API_KEY") != ""
	case "groq":
		return os.Getenv("GROQ_API_KEY") != ""
	case "deepseek":
		return os.Getenv("DEEPSEEK_API_KEY") != ""
	case "openrouter":
		return os.Getenv("OPENROUTER_API_KEY") != ""
	case "together":
		return os.Getenv("TOGETHER_API_KEY") != ""
	case "ollama":
		return true // Always available locally
	}
	return false
}

