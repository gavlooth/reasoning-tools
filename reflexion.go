package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Reflexion implements episodic memory and learning from failures
type Reflexion struct {
	provider   Provider
	config     ReflexionConfig
	memory     *EpisodicMemory
	tools      *ToolRegistry
	toolCalls  int
	onProgress func(ProgressUpdate)
}

// ReflexionConfig configures the reflexion process
type ReflexionConfig struct {
	MaxAttempts           int      // Maximum reasoning attempts before giving up (default: 3)
	MaxThoughtsPerAttempt int      // Max thoughts per attempt (default: 10)
	MemoryPath            string   // Path to store episodic memory (default: ~/.local/share/reasoning-tools/memory.json)
	LearnFromPast         bool     // Whether to query past failures (default: true)
	Temperature           float64  // LLM temperature (default: 0.7)
	EnableTools           bool     // Enable tool usage during reasoning
	MaxToolCalls          int      // Maximum tool calls per attempt (default: 5)
	EnabledTools          []string // Which tools to enable (empty = all)
}

// DefaultReflexionConfig returns sensible defaults
func DefaultReflexionConfig() ReflexionConfig {
	homeDir, _ := os.UserHomeDir()
	return ReflexionConfig{
		MaxAttempts:          3,
		MaxThoughtsPerAttempt: 10,
		MemoryPath:           filepath.Join(homeDir, ".local", "share", "reasoning-tools", "memory.json"),
		LearnFromPast:        true,
		Temperature:          0.7,
	}
}

// EpisodicMemory stores past reasoning attempts and their outcomes
type EpisodicMemory struct {
	Episodes []Episode `json:"episodes"`
	mu       sync.RWMutex
	path     string
}

// Episode represents a single reasoning attempt
type Episode struct {
	ID           string    `json:"id"`
	Problem      string    `json:"problem"`
	ProblemHash  string    `json:"problem_hash"` // For similarity matching
	Attempt      int       `json:"attempt"`
	Thoughts     []string  `json:"thoughts"`
	FinalAnswer  string    `json:"final_answer"`
	WasSuccessful bool     `json:"was_successful"`
	FailureReason string   `json:"failure_reason,omitempty"`
	Reflection   string    `json:"reflection,omitempty"` // What went wrong / what to try differently
	Timestamp    time.Time `json:"timestamp"`
	Provider     string    `json:"provider"`
}

// ReflexionResult represents the complete result of reflexion reasoning
type ReflexionResult struct {
	Problem        string         `json:"problem"`
	Attempts       []Attempt      `json:"attempts"`
	FinalAnswer    string         `json:"final_answer"`
	TotalAttempts  int            `json:"total_attempts"`
	Success        bool           `json:"success"`
	Provider       string         `json:"provider"`
	LessonsLearned []string       `json:"lessons_learned,omitempty"`
	TotalToolCalls int            `json:"total_tool_calls,omitempty"`
	ToolsUsed      map[string]int `json:"tools_used,omitempty"`
}

// Attempt represents one reasoning attempt
type Attempt struct {
	Number        int          `json:"number"`
	Thoughts      []string     `json:"thoughts"`
	Answer        string       `json:"answer"`
	Evaluation    string       `json:"evaluation"`
	WasSuccessful bool         `json:"was_successful"`
	Reflection    string       `json:"reflection,omitempty"`
	ToolResults   []ToolResult `json:"tool_results,omitempty"`
}

// NewReflexion creates a new Reflexion instance
func NewReflexion(provider Provider, config ReflexionConfig) *Reflexion {
	memory := loadOrCreateMemory(config.MemoryPath)
	r := &Reflexion{
		provider: provider,
		config:   config,
		memory:   memory,
	}

	// Initialize tools if enabled
	if config.EnableTools {
		r.tools = NewToolRegistry()
		// Filter enabled tools if specified
		if len(config.EnabledTools) > 0 {
			r.tools.SetEnabled(config.EnabledTools)
		}
	}

	return r
}

// SetProgressCallback sets a callback for progress updates
func (r *Reflexion) SetProgressCallback(cb func(ProgressUpdate)) {
	r.onProgress = cb
}

func (r *Reflexion) emitProgress(update ProgressUpdate) {
	if r.onProgress != nil {
		r.onProgress(update)
	}
}

// Reason performs reflexion-style reasoning with learning from failures
func (r *Reflexion) Reason(ctx context.Context, problem string) (*ReflexionResult, error) {
	// Reset tool call counter for this reasoning session
	r.toolCalls = 0

	result := &ReflexionResult{
		Problem:   problem,
		Attempts:  []Attempt{},
		Provider:  r.provider.Name(),
		ToolsUsed: make(map[string]int),
	}

	// Get lessons from similar past problems
	var pastLessons []string
	if r.config.LearnFromPast {
		pastLessons = r.getPastLessons(problem)
		if len(pastLessons) > 0 {
			result.LessonsLearned = pastLessons
			r.emitProgress(ProgressUpdate{
				Type:    "thought",
				Message: fmt.Sprintf("Found %d relevant lessons from past attempts", len(pastLessons)),
			})
		}
	}

	var lastReflection string

	for attemptNum := 1; attemptNum <= r.config.MaxAttempts; attemptNum++ {
		r.emitProgress(ProgressUpdate{
			Type:    "thought",
			Message: fmt.Sprintf("Starting attempt %d/%d", attemptNum, r.config.MaxAttempts),
			Depth:   attemptNum,
		})

		attempt := Attempt{
			Number:   attemptNum,
			Thoughts: []string{},
		}

		// Generate reasoning with awareness of past failures
		thoughts, answer, attemptToolResults, err := r.generateReasoning(ctx, problem, pastLessons, lastReflection, attemptNum)
		if err != nil {
			attempt.Evaluation = fmt.Sprintf("Error: %v", err)
			result.Attempts = append(result.Attempts, attempt)
			continue
		}

		attempt.Thoughts = thoughts
		attempt.Answer = answer
		attempt.ToolResults = attemptToolResults

		// Track tool usage
		for _, tr := range attemptToolResults {
			result.ToolsUsed[tr.Tool]++
		}

		// Evaluate the answer
		evaluation, isCorrect, err := r.evaluateAnswer(ctx, problem, thoughts, answer)
		if err != nil {
			attempt.Evaluation = fmt.Sprintf("Evaluation error: %v", err)
			result.Attempts = append(result.Attempts, attempt)
			continue
		}

		attempt.Evaluation = evaluation
		attempt.WasSuccessful = isCorrect

		r.emitProgress(ProgressUpdate{
			Type:       "evaluation",
			Message:    fmt.Sprintf("Attempt %d evaluation: %s", attemptNum, evaluation),
			IsSolution: isCorrect,
			Score:      boolToFloat(isCorrect),
		})

		if isCorrect {
			attempt.WasSuccessful = true
			result.Attempts = append(result.Attempts, attempt)
			result.FinalAnswer = answer
			result.Success = true
			result.TotalAttempts = attemptNum
			result.TotalToolCalls = r.toolCalls

			// Store successful episode
			r.storeEpisode(problem, attemptNum, thoughts, answer, true, "", "")

			r.emitProgress(ProgressUpdate{
				Type:        "solution",
				FinalAnswer: answer,
				Message:     fmt.Sprintf("Success on attempt %d!", attemptNum),
			})

			return result, nil
		}

		// Generate reflection on what went wrong
		reflection, err := r.generateReflection(ctx, problem, thoughts, answer, evaluation)
		if err != nil {
			reflection = "Unable to generate reflection"
		}

		attempt.Reflection = reflection
		lastReflection = reflection
		result.Attempts = append(result.Attempts, attempt)

		r.emitProgress(ProgressUpdate{
			Type:    "thought",
			Message: fmt.Sprintf("Reflection: %s", truncateStr(reflection, 100)),
		})

		// Store failed episode
		r.storeEpisode(problem, attemptNum, thoughts, answer, false, evaluation, reflection)
	}

	// All attempts failed
	result.TotalAttempts = r.config.MaxAttempts
	result.TotalToolCalls = r.toolCalls
	if len(result.Attempts) > 0 {
		// Use the last attempt's answer
		result.FinalAnswer = result.Attempts[len(result.Attempts)-1].Answer
	}

	return result, nil
}

// generateReasoning generates a chain of thoughts for the problem
func (r *Reflexion) generateReasoning(ctx context.Context, problem string, pastLessons []string, lastReflection string, attemptNum int) ([]string, string, []ToolResult, error) {
	var thoughts []string
	var toolResults []ToolResult

	// Build context from past lessons and reflections
	var contextParts []string
	if len(pastLessons) > 0 {
		contextParts = append(contextParts, "Lessons from similar past problems:")
		for _, lesson := range pastLessons {
			contextParts = append(contextParts, fmt.Sprintf("- %s", lesson))
		}
	}
	if lastReflection != "" {
		contextParts = append(contextParts, fmt.Sprintf("\nReflection from previous attempt:\n%s", lastReflection))
	}

	systemPrompt := `You are a thoughtful problem solver. You learn from past mistakes and adapt your approach.
Think step by step and show your reasoning clearly. Each thought should build toward a solution.`

	// Add tool instructions if enabled
	toolPrompt := ""
	if r.tools != nil {
		systemPrompt += "\n\nYou have access to tools that can help you compute, verify, or look up information."
		toolPrompt = r.tools.GetToolsPrompt()
	}

	var userPrompt string
	if r.tools != nil {
		userPrompt = fmt.Sprintf(`Problem: %s

%s

This is attempt %d. Think through the problem step by step, learning from any past mistakes.

%s

For each step, output a JSON object with ONE of these formats:

For a reasoning step:
{
  "type": "thought",
  "thought_number": <step number>,
  "thought": "<your reasoning for this step>",
  "is_final": <true if this is your final answer, false otherwise>,
  "answer": "<your answer, only if is_final is true>"
}

To use a tool:
{
  "type": "tool",
  "tool": "<tool name>",
  "input": "<tool input>"
}`, problem, strings.Join(contextParts, "\n"), attemptNum, toolPrompt)
	} else {
		userPrompt = fmt.Sprintf(`Problem: %s

%s

This is attempt %d. Think through the problem step by step, learning from any past mistakes.

For each step, output a JSON object:
{
  "thought_number": <step number>,
  "thought": "<your reasoning for this step>",
  "is_final": <true if this is your final answer, false otherwise>,
  "answer": "<your answer, only if is_final is true>"
}`, problem, strings.Join(contextParts, "\n"), attemptNum)
	}

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	maxToolCallsPerAttempt := r.config.MaxToolCalls
	if maxToolCallsPerAttempt == 0 {
		maxToolCallsPerAttempt = 5
	}
	attemptToolCalls := 0

	for i := 0; i < r.config.MaxThoughtsPerAttempt; i++ {
		response, err := r.provider.Chat(ctx, messages, ChatOptions{
			Temperature: r.config.Temperature,
			MaxTokens:   1024,
		})
		if err != nil {
			return thoughts, "", toolResults, fmt.Errorf("reasoning failed at step %d: %w", i+1, err)
		}

		// Parse the response
		jsonStr := extractJSON(response)
		if jsonStr == "" {
			// Try to extract thought anyway
			thoughts = append(thoughts, response)
			messages = append(messages, ChatMessage{Role: "assistant", Content: response})
			messages = append(messages, ChatMessage{Role: "user", Content: "Continue your reasoning."})
			continue
		}

		// First check if it's a tool call
		var toolStep struct {
			Type  string `json:"type"`
			Tool  string `json:"tool"`
			Input string `json:"input"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &toolStep); err == nil && toolStep.Type == "tool" && r.tools != nil {
			// Execute tool if within limits
			if attemptToolCalls < maxToolCallsPerAttempt {
				result := r.tools.Execute(ctx, toolStep.Tool, toolStep.Input)
				toolResults = append(toolResults, result)
				attemptToolCalls++
				r.toolCalls++

				r.emitProgress(ProgressUpdate{
					Type:       "tool",
					ToolName:   toolStep.Tool,
					ToolInput:  toolStep.Input,
					ToolOutput: result.Output,
				})

				messages = append(messages, ChatMessage{Role: "assistant", Content: response})
				messages = append(messages, ChatMessage{Role: "user", Content: fmt.Sprintf("Tool result: %s\n\nContinue your reasoning.", result.Output)})
			} else {
				messages = append(messages, ChatMessage{Role: "assistant", Content: response})
				messages = append(messages, ChatMessage{Role: "user", Content: "Tool limit reached. Continue reasoning without tools."})
			}
			continue
		}

		var step struct {
			ThoughtNumber int    `json:"thought_number"`
			Thought       string `json:"thought"`
			IsFinal       bool   `json:"is_final"`
			Answer        string `json:"answer"`
		}

		if err := json.Unmarshal([]byte(jsonStr), &step); err != nil {
			thoughts = append(thoughts, response)
		} else {
			thoughts = append(thoughts, step.Thought)

			r.emitProgress(ProgressUpdate{
				Type:    "thought",
				NodeID:  fmt.Sprintf("t%d", i+1),
				Thought: truncateStr(step.Thought, 100),
				Depth:   i + 1,
			})

			if step.IsFinal {
				return thoughts, step.Answer, toolResults, nil
			}
		}

		messages = append(messages, ChatMessage{Role: "assistant", Content: response})
		messages = append(messages, ChatMessage{Role: "user", Content: "Continue your reasoning. Output another JSON object for your next thought."})
	}

	// Reached max thoughts, ask for final answer
	finalPrompt := "You've reached the maximum number of reasoning steps. Please provide your final answer now as a JSON object with is_final: true."
	messages = append(messages, ChatMessage{Role: "user", Content: finalPrompt})

	response, err := r.provider.Chat(ctx, messages, ChatOptions{
		Temperature: r.config.Temperature,
		MaxTokens:   512,
	})
	if err != nil {
		return thoughts, "", toolResults, err
	}

	jsonStr := extractJSON(response)
	if jsonStr != "" {
		var step struct {
			Answer string `json:"answer"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &step); err == nil && step.Answer != "" {
			return thoughts, step.Answer, toolResults, nil
		}
	}

	// Fallback: use the response as the answer
	return thoughts, strings.TrimSpace(response), toolResults, nil
}

// evaluateAnswer evaluates if the answer is correct/satisfactory
func (r *Reflexion) evaluateAnswer(ctx context.Context, problem string, thoughts []string, answer string) (string, bool, error) {
	thoughtsStr := ""
	for i, t := range thoughts {
		thoughtsStr += fmt.Sprintf("%d. %s\n", i+1, t)
	}

	prompt := fmt.Sprintf(`Evaluate this reasoning and answer for the given problem.

Problem: %s

Reasoning:
%s

Final Answer: %s

Evaluate:
1. Is the reasoning logical and sound?
2. Does the answer correctly solve the problem?
3. Are there any errors or gaps?

Respond with ONLY a JSON object:
{
  "evaluation": "<brief evaluation of the reasoning and answer>",
  "is_correct": <true if the answer is correct and well-reasoned, false otherwise>,
  "issues": ["<list of any issues found>"]
}`, problem, thoughtsStr, answer)

	messages := []ChatMessage{
		{Role: "system", Content: "You are a strict evaluator. Check reasoning carefully for errors."},
		{Role: "user", Content: prompt},
	}

	response, err := r.provider.Chat(ctx, messages, ChatOptions{
		Temperature: 0.3,
		MaxTokens:   512,
	})
	if err != nil {
		return "", false, err
	}

	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return response, false, nil
	}

	var eval struct {
		Evaluation string   `json:"evaluation"`
		IsCorrect  bool     `json:"is_correct"`
		Issues     []string `json:"issues"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &eval); err != nil {
		return response, false, nil
	}

	evalStr := eval.Evaluation
	if len(eval.Issues) > 0 {
		evalStr += " Issues: " + strings.Join(eval.Issues, "; ")
	}

	return evalStr, eval.IsCorrect, nil
}

// generateReflection generates a reflection on what went wrong
func (r *Reflexion) generateReflection(ctx context.Context, problem string, thoughts []string, answer, evaluation string) (string, error) {
	thoughtsStr := ""
	for i, t := range thoughts {
		thoughtsStr += fmt.Sprintf("%d. %s\n", i+1, t)
	}

	prompt := fmt.Sprintf(`The following reasoning attempt was not successful. Generate a reflection to improve the next attempt.

Problem: %s

Reasoning:
%s

Answer Given: %s

Evaluation: %s

Generate a brief, actionable reflection:
1. What went wrong?
2. What should be done differently next time?
3. What key insight was missed?

Respond with just the reflection text, no JSON.`, problem, thoughtsStr, answer, evaluation)

	messages := []ChatMessage{
		{Role: "system", Content: "You are a thoughtful analyst. Generate concise, actionable reflections on reasoning failures."},
		{Role: "user", Content: prompt},
	}

	response, err := r.provider.Chat(ctx, messages, ChatOptions{
		Temperature: 0.5,
		MaxTokens:   512,
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response), nil
}

// getPastLessons retrieves relevant lessons from past similar problems
func (r *Reflexion) getPastLessons(problem string) []string {
	r.memory.mu.RLock()
	defer r.memory.mu.RUnlock()

	problemHash := hashProblem(problem)
	var relevantEpisodes []Episode

	// Find episodes with similar problems
	for _, ep := range r.memory.Episodes {
		// Check for exact match or similar hash
		if ep.ProblemHash == problemHash || stringSimilarity(ep.Problem, problem) > 0.5 {
			relevantEpisodes = append(relevantEpisodes, ep)
		}
	}

	if len(relevantEpisodes) == 0 {
		return nil
	}

	// Sort by timestamp (most recent first)
	sort.Slice(relevantEpisodes, func(i, j int) bool {
		return relevantEpisodes[i].Timestamp.After(relevantEpisodes[j].Timestamp)
	})

	// Extract lessons (reflections from failed attempts)
	var lessons []string
	seen := make(map[string]bool)

	for _, ep := range relevantEpisodes {
		if !ep.WasSuccessful && ep.Reflection != "" {
			if !seen[ep.Reflection] {
				lessons = append(lessons, ep.Reflection)
				seen[ep.Reflection] = true
			}
		}
		if len(lessons) >= 3 { // Limit to 3 lessons
			break
		}
	}

	return lessons
}

// storeEpisode stores a reasoning episode in memory
func (r *Reflexion) storeEpisode(problem string, attempt int, thoughts []string, answer string, successful bool, failureReason, reflection string) {
	r.memory.mu.Lock()
	defer r.memory.mu.Unlock()

	episode := Episode{
		ID:            fmt.Sprintf("%s_%d_%d", hashProblem(problem)[:8], attempt, time.Now().Unix()),
		Problem:       problem,
		ProblemHash:   hashProblem(problem),
		Attempt:       attempt,
		Thoughts:      thoughts,
		FinalAnswer:   answer,
		WasSuccessful: successful,
		FailureReason: failureReason,
		Reflection:    reflection,
		Timestamp:     time.Now(),
		Provider:      r.provider.Name(),
	}

	r.memory.Episodes = append(r.memory.Episodes, episode)

	// Keep only last 100 episodes
	if len(r.memory.Episodes) > 100 {
		r.memory.Episodes = r.memory.Episodes[len(r.memory.Episodes)-100:]
	}

	// Save to disk
	r.memory.save()
}

// loadOrCreateMemory loads existing memory or creates new
func loadOrCreateMemory(path string) *EpisodicMemory {
	memory := &EpisodicMemory{
		Episodes: []Episode{},
		path:     path,
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return memory
	}

	// Try to load existing memory
	data, err := os.ReadFile(path)
	if err != nil {
		return memory
	}

	if err := json.Unmarshal(data, memory); err != nil {
		return memory
	}

	memory.path = path
	return memory
}

// save persists the memory to disk
func (m *EpisodicMemory) save() {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}

	_ = os.WriteFile(m.path, data, 0644)
}

// hashProblem creates a hash for similarity matching
func hashProblem(problem string) string {
	// Normalize: lowercase, remove extra whitespace
	normalized := strings.ToLower(strings.TrimSpace(problem))
	normalized = strings.Join(strings.Fields(normalized), " ")

	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// stringSimilarity computes a simple similarity score between strings
func stringSimilarity(a, b string) float64 {
	a = strings.ToLower(a)
	b = strings.ToLower(b)

	wordsA := strings.Fields(a)
	wordsB := strings.Fields(b)

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}

	// Count common words
	wordSetB := make(map[string]bool)
	for _, w := range wordsB {
		wordSetB[w] = true
	}

	common := 0
	for _, w := range wordsA {
		if wordSetB[w] {
			common++
		}
	}

	// Jaccard-like similarity
	return float64(common) / float64(len(wordsA)+len(wordsB)-common)
}

func boolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

// FormatReflexionResult formats the result for display
func FormatReflexionResult(result *ReflexionResult) string {
	var sb strings.Builder

	sb.WriteString("## Reflexion Reasoning Result\n\n")
	sb.WriteString(fmt.Sprintf("**Problem:** %s\n\n", result.Problem))
	sb.WriteString(fmt.Sprintf("**Provider:** %s\n", result.Provider))
	sb.WriteString(fmt.Sprintf("**Total Attempts:** %d\n", result.TotalAttempts))
	sb.WriteString(fmt.Sprintf("**Success:** %v\n", result.Success))
	if result.TotalToolCalls > 0 {
		sb.WriteString(fmt.Sprintf("**Total Tool Calls:** %d\n", result.TotalToolCalls))
	}
	sb.WriteString("\n")

	if len(result.LessonsLearned) > 0 {
		sb.WriteString("### Lessons from Past (Applied)\n\n")
		for _, lesson := range result.LessonsLearned {
			sb.WriteString(fmt.Sprintf("- %s\n", truncateStr(lesson, 100)))
		}
		sb.WriteString("\n")
	}

	for _, attempt := range result.Attempts {
		sb.WriteString(fmt.Sprintf("### Attempt %d\n\n", attempt.Number))

		sb.WriteString("**Reasoning:**\n")
		for i, thought := range attempt.Thoughts {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, truncateStr(thought, 150)))
		}
		sb.WriteString("\n")

		// Display tool results for this attempt
		if len(attempt.ToolResults) > 0 {
			sb.WriteString("**Tools Used:**\n")
			for _, tr := range attempt.ToolResults {
				sb.WriteString(fmt.Sprintf("- `%s(%s)` → %s\n", tr.Tool, truncateStr(tr.Input, 30), truncateStr(tr.Output, 50)))
			}
			sb.WriteString("\n")
		}

		sb.WriteString(fmt.Sprintf("**Answer:** %s\n\n", attempt.Answer))
		sb.WriteString(fmt.Sprintf("**Evaluation:** %s\n\n", attempt.Evaluation))

		if attempt.WasSuccessful {
			sb.WriteString("**Result:** Success!\n\n")
		} else if attempt.Reflection != "" {
			sb.WriteString(fmt.Sprintf("**Reflection:** %s\n\n", attempt.Reflection))
		}

		sb.WriteString("---\n\n")
	}

	sb.WriteString(fmt.Sprintf("### Final Answer\n\n%s\n", result.FinalAnswer))

	// JSON summary
	summaryMap := map[string]interface{}{
		"success":        result.Success,
		"total_attempts": result.TotalAttempts,
		"final_answer":   result.FinalAnswer,
		"provider":       result.Provider,
	}
	if result.TotalToolCalls > 0 {
		summaryMap["total_tool_calls"] = result.TotalToolCalls
		summaryMap["tools_used"] = result.ToolsUsed
	}
	jsonResult, _ := json.MarshalIndent(summaryMap, "", "  ")

	sb.WriteString("\n### JSON Summary\n```json\n")
	sb.WriteString(string(jsonResult))
	sb.WriteString("\n```\n")

	return sb.String()
}

// ClearMemory clears the episodic memory (for testing)
func (r *Reflexion) ClearMemory() {
	r.memory.mu.Lock()
	defer r.memory.mu.Unlock()
	r.memory.Episodes = []Episode{}
	r.memory.save()
}

// GetMemoryStats returns statistics about the memory
func (r *Reflexion) GetMemoryStats() map[string]interface{} {
	r.memory.mu.RLock()
	defer r.memory.mu.RUnlock()

	successful := 0
	failed := 0
	for _, ep := range r.memory.Episodes {
		if ep.WasSuccessful {
			successful++
		} else {
			failed++
		}
	}

	return map[string]interface{}{
		"total_episodes":      len(r.memory.Episodes),
		"successful_episodes": successful,
		"failed_episodes":     failed,
		"memory_path":         r.memory.path,
	}
}
