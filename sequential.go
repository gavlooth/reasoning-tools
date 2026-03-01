package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"reasoning-tools/utils"
)

// ProviderSelector chooses providers per step based on strategy
type ProviderSelector struct {
	primary      Provider
	cheap        Provider
	capable      Provider
	providers    []Provider    // For round_robin strategy - list of providers to cycle through
	strategy     string        // "single", "cascade", "alternating", "round_robin", "custom"
	upgradeAfter int           // Switch from cheap to capable after N steps
	stepMapping  map[int]int   // step -> provider index (for custom)
}

// SelectProvider returns the provider to use for a given step
func (ps *ProviderSelector) SelectProvider(stepNum int) Provider {
	switch ps.strategy {
	case "cascade":
		// Use cheap for first N steps, then upgrade to capable
		if stepNum <= ps.upgradeAfter && ps.cheap != nil {
			return ps.cheap
		}
		if ps.capable != nil {
			return ps.capable
		}
		return ps.primary

	case "alternating":
		// Alternate between cheap and capable
		if stepNum%2 == 1 && ps.cheap != nil {
			return ps.cheap
		}
		if ps.capable != nil {
			return ps.capable
		}
		return ps.primary

	case "round_robin":
		// Cycle through providers list
		if len(ps.providers) > 0 {
			idx := (stepNum - 1) % len(ps.providers) // step 1 -> idx 0, step 2 -> idx 1, etc.
			return ps.providers[idx]
		}
		return ps.primary

	case "custom":
		// Use step mapping (maps step number to provider index)
		if ps.stepMapping != nil {
			if idx, ok := ps.stepMapping[stepNum]; ok && idx < len(ps.providers) {
				return ps.providers[idx]
			}
		}
		// Fallback to round_robin
		if len(ps.providers) > 0 {
			idx := (stepNum - 1) % len(ps.providers)
			return ps.providers[idx]
		}
		return ps.primary

	default: // "single"
		return ps.primary
	}
}

// Name returns a description of the current provider selection
func (ps *ProviderSelector) Name() string {
	if ps.strategy == "single" {
		return ps.primary.Name()
	}
	if ps.strategy == "round_robin" && len(ps.providers) > 0 {
		names := make([]string, len(ps.providers))
		for i, p := range ps.providers {
			names[i] = p.Name()
		}
		return fmt.Sprintf("round_robin(%s)", strings.Join(names, "->"))
	}
	if ps.cheap != nil && ps.capable != nil {
		return fmt.Sprintf("%s(%s->%s)", ps.strategy, ps.cheap.Name(), ps.capable.Name())
	}
	return ps.primary.Name()
}

// Strategy returns the current strategy
func (ps *ProviderSelector) Strategy() string {
	return ps.strategy
}

// NewProviderSelector creates a provider selector from config
func NewProviderSelector(primary, cheap, capable Provider, strategy string, upgradeAfter int) *ProviderSelector {
	return &ProviderSelector{
		primary:      primary,
		cheap:        cheap,
		capable:      capable,
		strategy:     strategy,
		upgradeAfter: upgradeAfter,
	}
}

// NewProviderSelectorWithMapping creates a provider selector with custom step mapping
func NewProviderSelectorWithMapping(primary, cheap, capable Provider, strategy string, upgradeAfter int, stepMapping map[int]string) *ProviderSelector {
	return &ProviderSelector{
		primary:      primary,
		cheap:        cheap,
		capable:      capable,
		strategy:     strategy,
		upgradeAfter: upgradeAfter,
	}
}

// NewProviderSelectorRoundRobin creates a selector that cycles through providers
func NewProviderSelectorRoundRobin(providers []Provider) *ProviderSelector {
	return &ProviderSelector{
		primary:   providers[0], // Fallback
		providers: providers,
		strategy:  "round_robin",
	}
}

// NewProviderSelectorWithProviders creates a selector with a list of providers and optional custom mapping
func NewProviderSelectorWithProviders(providers []Provider, strategy string, stepMapping map[int]int) *ProviderSelector {
	primary := providers[0]
	if len(providers) > 0 {
		primary = providers[0]
	}
	return &ProviderSelector{
		primary:     primary,
		providers:   providers,
		strategy:    strategy,
		stepMapping: stepMapping,
	}
}

// SequentialClient performs simple linear sequential thinking
type SequentialClient struct {
	selector      *ProviderSelector
	onProgress    func(ProgressUpdate)
	onToken       func(token string)
	enableStreams bool
}

// SetProgressCallback sets a callback for progress updates
func (c *SequentialClient) SetProgressCallback(cb func(ProgressUpdate)) {
	c.onProgress = cb
}

// SetTokenCallback sets a callback for token streaming
func (c *SequentialClient) SetTokenCallback(cb func(token string)) {
	c.onToken = cb
}

// SetEnableStreaming enables or disables LLM streaming
func (c *SequentialClient) SetEnableStreaming(enable bool) {
	c.enableStreams = enable
}

func (c *SequentialClient) emitProgress(update ProgressUpdate) {
	if c.onProgress != nil {
		c.onProgress(update)
	}
}

// ThinkingStep represents a single step in the thinking process
type ThinkingStep struct {
	ThoughtNumber     int    `json:"thought_number"`
	TotalThoughts     int    `json:"total_thoughts"`
	Thought           string `json:"thought"`
	IsRevision        bool   `json:"is_revision,omitempty"`
	RevisesThought    int    `json:"revises_thought,omitempty"`
	BranchFromThought int    `json:"branch_from_thought,omitempty"`
	BranchID          string `json:"branch_id,omitempty"`
	NeedsMoreThoughts bool   `json:"needs_more_thoughts,omitempty"`
}

// ThinkingResult represents the complete result of sequential thinking
type ThinkingResult struct {
	Problem     string         `json:"problem"`
	Steps       []ThinkingStep `json:"steps"`
	FinalAnswer string         `json:"final_answer"`
	TotalSteps  int            `json:"total_steps"`
	Success     bool           `json:"success"`
	Provider    string         `json:"provider"`
}

// LLMThinkingResponse is what we expect from the LLM in JSON format
type LLMThinkingResponse struct {
	ThoughtNumber     int    `json:"thought_number"`
	TotalThoughts     int    `json:"total_thoughts"`
	Thought           string `json:"thought"`
	NextThoughtNeeded bool   `json:"next_thought_needed"`
	IsRevision        bool   `json:"is_revision,omitempty"`
	RevisesThought    int    `json:"revises_thought,omitempty"`
	BranchFromThought int    `json:"branch_from_thought,omitempty"`
	BranchID          string `json:"branch_id,omitempty"`
	NeedsMoreThoughts bool   `json:"needs_more_thoughts,omitempty"`
	FinalAnswer       string `json:"final_answer,omitempty"`
}

const sequentialSystemPrompt = `You are a sequential thinking assistant. Your task is to solve problems through careful, step-by-step reasoning.

For each thinking step, respond with ONLY a JSON object (no markdown, no extra text) in this format:
{
  "thought_number": <current step number>,
  "total_thoughts": <estimated total steps needed>,
  "thought": "<your current thinking step>",
  "next_thought_needed": <true if more thinking needed, false if done>,
  "is_revision": <true if revising a previous thought>,
  "revises_thought": <which thought number you're revising, if applicable>,
  "final_answer": "<your final answer, only when next_thought_needed is false>"
}

Guidelines:
1. Start with an initial estimate of needed thoughts, but adjust as you learn more
2. Feel free to question or revise previous thoughts
3. Express uncertainty when present
4. When you reach a satisfactory answer, set next_thought_needed to false and provide final_answer
5. Each thought should build meaningfully toward the solution
6. You can adjust total_thoughts up or down as needed`

// Think performs sequential thinking on a problem
func (c *SequentialClient) Think(ctx context.Context, problem string, maxThoughts int) (*ThinkingResult, error) {
	result := &ThinkingResult{
		Problem:  problem,
		Steps:    []ThinkingStep{},
		Success:  false,
		Provider: c.selector.Name(),
	}

	messages := []ChatMessage{
		{Role: "system", Content: sequentialSystemPrompt},
		{Role: "user", Content: fmt.Sprintf("Problem to solve:\n\n%s\n\nBegin your sequential thinking process.", problem)},
	}

	for i := 0; i < maxThoughts; i++ {
		// Select provider for this step
		provider := c.selector.SelectProvider(i + 1)

		// Emit progress: generating thought
		c.emitProgress(ProgressUpdate{
			Type:    EventTypeProgress,
			NodeID:  fmt.Sprintf("t%d", i+1),
			Message: fmt.Sprintf("Generating thought %d (via %s)...", i+1, provider.Name()),
			Depth:   i + 1,
		})

		var response string
		var err error

		// Check if provider supports streaming
		streamingProvider, canStream := provider.(StreamingProvider)
		useStreaming := canStream && c.enableStreams && streamingProvider.SupportsStreaming()

		// Call LLM with or without streaming
		if useStreaming {
			response, err = streamingProvider.ChatStream(ctx, messages, ChatOptions{
				Temperature: 0.7,
				MaxTokens:   2048,
			}, func(token string) {
				if c.onToken != nil {
					c.onToken(token)
				}
			})
		} else {
			response, err = provider.Chat(ctx, messages, ChatOptions{
				Temperature: 0.7,
				MaxTokens:   2048,
			})
		}

		if err != nil {
			return result, fmt.Errorf("LLM call failed at step %d: %w", i+1, err)
		}

		// Parse the response as JSON (with fallback to structured text)
		thinkingResp, usedFallback, err := parseThinkingResponse(response, i+1, maxThoughts)
		if err != nil {
			return result, fmt.Errorf("failed to parse thinking response at step %d: %w", i+1, err)
		}
		if usedFallback {
			fmt.Fprintf(os.Stderr, "[WARNING] sequential_thinking: falling back to text parsing at step %d. Response preview: %s\n",
				i+1, utils.TruncateStr(response, 120))
		}

		// Record the step
		step := ThinkingStep{
			ThoughtNumber:     thinkingResp.ThoughtNumber,
			TotalThoughts:     thinkingResp.TotalThoughts,
			Thought:           thinkingResp.Thought,
			IsRevision:        thinkingResp.IsRevision,
			RevisesThought:    thinkingResp.RevisesThought,
			BranchFromThought: thinkingResp.BranchFromThought,
			BranchID:          thinkingResp.BranchID,
			NeedsMoreThoughts: thinkingResp.NeedsMoreThoughts,
		}
		result.Steps = append(result.Steps, step)

		// Emit progress: thought generated
		c.emitProgress(ProgressUpdate{
			Type:    EventTypeThought,
			NodeID:  fmt.Sprintf("t%d", i+1),
			Thought: utils.TruncateStr(thinkingResp.Thought, 100),
			Depth:   i + 1,
		})

		// Add assistant response to conversation
		messages = append(messages, ChatMessage{Role: "assistant", Content: response})

		// Check if thinking is complete
		if !thinkingResp.NextThoughtNeeded {
			result.FinalAnswer = thinkingResp.FinalAnswer
			result.Success = true
			result.TotalSteps = len(result.Steps)

			// Emit solution progress
			c.emitProgress(ProgressUpdate{
				Type:        EventTypeSolution,
				FinalAnswer: thinkingResp.FinalAnswer,
				IsSolution:  true,
			})

			return result, nil
		}

		// Prompt for next thought
		messages = append(messages, ChatMessage{
			Role:    "user",
			Content: "Continue to the next thought.",
		})
	}

	// Reached max thoughts without completion
	result.TotalSteps = len(result.Steps)
	result.FinalAnswer = "Maximum thinking steps reached without a definitive answer. Review the steps above."
	return result, nil
}

func parseThinkingResponse(response string, stepNum, maxThoughts int) (LLMThinkingResponse, bool, error) {
	var thinkingResp LLMThinkingResponse
	if err := json.Unmarshal([]byte(response), &thinkingResp); err == nil {
		return thinkingResp, false, nil
	}

	jsonStr := utils.ExtractJSON(response)
	if jsonStr != "" {
		if err := json.Unmarshal([]byte(jsonStr), &thinkingResp); err == nil {
			return thinkingResp, false, nil
		}
	}

	thought := strings.TrimSpace(response)
	if thought == "" {
		return LLMThinkingResponse{}, true, fmt.Errorf("empty response")
	}

	answer := extractFinalAnswerFromText(response)
	nextNeeded := true
	if answer != "" {
		nextNeeded = false
	}
	if stepNum >= maxThoughts {
		nextNeeded = false
		if answer == "" {
			answer = thought
		}
	}
	if !nextNeeded && answer == "" {
		answer = thought
	}

	totalThoughts := maxThoughts
	if !nextNeeded {
		totalThoughts = stepNum
	}

	return LLMThinkingResponse{
		ThoughtNumber:     stepNum,
		TotalThoughts:     totalThoughts,
		Thought:           thought,
		NextThoughtNeeded: nextNeeded,
		FinalAnswer:       answer,
	}, true, nil
}

func extractFinalAnswerFromText(response string) string {
	re := regexp.MustCompile(`(?is)(?:final answer|answer)\s*[:\-]\s*(.+)$`)
	match := re.FindStringSubmatch(response)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}
