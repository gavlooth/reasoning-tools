package main

import (
	"context"
	"encoding/json"
	"fmt"

	"reasoning-tools/utils"
)

// SequentialClient performs simple linear sequential thinking
type SequentialClient struct {
	provider      Provider
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
		Provider: c.provider.Name(),
	}

	messages := []ChatMessage{
		{Role: "system", Content: sequentialSystemPrompt},
		{Role: "user", Content: fmt.Sprintf("Problem to solve:\n\n%s\n\nBegin your sequential thinking process.", problem)},
	}

	// Check if provider supports streaming
	streamingProvider, canStream := c.provider.(StreamingProvider)
	useStreaming := canStream && c.enableStreams && streamingProvider.SupportsStreaming()

	for i := 0; i < maxThoughts; i++ {
		// Emit progress: generating thought
		c.emitProgress(ProgressUpdate{
			Type:    EventTypeProgress,
			NodeID:  fmt.Sprintf("t%d", i+1),
			Message: fmt.Sprintf("Generating thought %d...", i+1),
			Depth:   i + 1,
		})

		var response string
		var err error

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
			response, err = c.provider.Chat(ctx, messages, ChatOptions{
				Temperature: 0.7,
				MaxTokens:   2048,
			})
		}

		if err != nil {
			return result, fmt.Errorf("LLM call failed at step %d: %w", i+1, err)
		}

		// Parse the response as JSON
		var thinkingResp LLMThinkingResponse
		if err := json.Unmarshal([]byte(response), &thinkingResp); err != nil {
			// Try to extract JSON from the response if it has extra text
			jsonStr := utils.ExtractJSON(response)
			if jsonStr == "" {
				return result, fmt.Errorf("failed to parse thinking response at step %d: %w\nResponse: %s", i+1, err, response)
			}
			if err := json.Unmarshal([]byte(jsonStr), &thinkingResp); err != nil {
				return result, fmt.Errorf("failed to parse extracted JSON at step %d: %w\nJSON: %s", i+1, err, jsonStr)
			}
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
