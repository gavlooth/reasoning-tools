package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// AlgorithmType represents a reasoning algorithm
type AlgorithmType string

const (
	AlgorithmSequential AlgorithmType = "sequential"
	AlgorithmGoT        AlgorithmType = "graph_of_thoughts"
	AlgorithmReflexion  AlgorithmType = "reflexion"
	AlgorithmDialectic  AlgorithmType = "dialectic"
)

// AlgorithmRecommendation represents a recommendation with reasoning
type AlgorithmRecommendation struct {
	Algorithm   AlgorithmType `json:"algorithm"`
	Confidence  float64       `json:"confidence"`
	Reasoning   string        `json:"reasoning"`
	Alternative AlgorithmType `json:"alternative,omitempty"`
}

// ProblemCharacteristics describes the nature of a problem
type ProblemCharacteristics struct {
	RequiresExploration    bool    `json:"requires_exploration"`     // Multiple solution paths worth exploring
	RequiresVerification   bool    `json:"requires_verification"`    // Needs fact-checking or validation
	IsControversial        bool    `json:"is_controversial"`         // Has multiple valid perspectives
	RequiresIteration      bool    `json:"requires_iteration"`       // May need multiple attempts
	Complexity             float64 `json:"complexity"`               // 0-1 scale
	HasMultiplePerspectives bool   `json:"has_multiple_perspectives"` // Multiple valid viewpoints
	NeedsStructuredOutput  bool    `json:"needs_structured_output"`  // Requires specific format
	IsFactual              bool    `json:"is_factual"`               // Has objective answer
}

// AlgorithmSelector selects the best reasoning algorithm for a problem
type AlgorithmSelector struct {
	provider Provider
}

// NewAlgorithmSelector creates a new selector
func NewAlgorithmSelector(provider Provider) *AlgorithmSelector {
	return &AlgorithmSelector{provider: provider}
}

// Analyze analyzes a problem and returns its characteristics
func (s *AlgorithmSelector) Analyze(ctx context.Context, problem string) (*ProblemCharacteristics, error) {
	prompt := fmt.Sprintf(`Analyze this problem and determine its characteristics.

Problem: %s

Evaluate each characteristic on whether it applies to this problem:

1. requires_exploration: Does this problem have multiple possible solution paths that are worth exploring?
2. requires_verification: Does this problem require fact-checking, calculation verification, or validation?
3. is_controversial: Is this a topic with multiple valid perspectives or debate?
4. requires_iteration: Might the first solution attempt need refinement?
5. complexity: How complex is this problem? (0.0 = trivial, 0.5 = moderate, 1.0 = very complex)
6. has_multiple_perspectives: Are there genuinely different valid viewpoints on this?
7. needs_structured_output: Does the answer need a specific format or structure?
8. is_factual: Does this have an objective, verifiable answer?

Respond with ONLY a JSON object:
{
  "requires_exploration": <true/false>,
  "requires_verification": <true/false>,
  "is_controversial": <true/false>,
  "requires_iteration": <true/false>,
  "complexity": <0.0-1.0>,
  "has_multiple_perspectives": <true/false>,
  "needs_structured_output": <true/false>,
  "is_factual": <true/false>
}`, problem)

	messages := []ChatMessage{
		{Role: "system", Content: "You are an analytical assistant that classifies problems to help select reasoning strategies."},
		{Role: "user", Content: prompt},
	}

	response, err := s.provider.Chat(ctx, messages, ChatOptions{
		Temperature: 0.3,
		MaxTokens:   256,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to analyze problem: %w", err)
	}

	// Extract JSON from response
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		// Log and use defaults
		log.Printf("[SELECTOR] Failed to extract JSON from analysis, using defaults. Response: %s", response)
		return &ProblemCharacteristics{
			Complexity: 0.5,
		}, nil
	}

	var chars ProblemCharacteristics
	if err := json.Unmarshal([]byte(jsonStr), &chars); err != nil {
		log.Printf("[SELECTOR] Failed to parse analysis JSON: %v", err)
		return &ProblemCharacteristics{
			Complexity: 0.5,
		}, nil
	}

	return &chars, nil
}

// extractJSON extracts JSON from a response that might contain other text
func extractJSON(response string) string {
	// Try to find JSON object
	start := strings.Index(response, "{")
	if start == -1 {
		return ""
	}

	// Find matching closing brace
	depth := 0
	for i := start; i < len(response); i++ {
		switch response[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return response[start : i+1]
			}
		}
	}

	return ""
}

// Recommend recommends the best algorithm based on problem characteristics
func (s *AlgorithmSelector) Recommend(chars *ProblemCharacteristics) *AlgorithmRecommendation {
	rec := &AlgorithmRecommendation{
		Confidence: 0.7,
	}

	// Decision tree for algorithm selection
	switch {
	case chars.IsControversial || chars.HasMultiplePerspectives:
		// Dialectic is best for controversial topics or multiple perspectives
		rec.Algorithm = AlgorithmDialectic
		rec.Reasoning = "Problem involves multiple perspectives or controversial aspects - dialectic reasoning will explore thesis/antithesis to find synthesis"
		rec.Alternative = AlgorithmGoT
		rec.Confidence = 0.85

	case chars.RequiresIteration || (chars.Complexity > 0.6 && !chars.IsFactual):
		// Reflexion is best when iteration might be needed
		rec.Algorithm = AlgorithmReflexion
		rec.Reasoning = "Problem may require multiple attempts and learning from failures"
		rec.Alternative = AlgorithmGoT
		rec.Confidence = 0.8

	case chars.RequiresExploration && chars.Complexity > 0.5:
		// Graph of Thoughts for complex exploration
		rec.Algorithm = AlgorithmGoT
		rec.Reasoning = "Problem has multiple solution paths worth exploring in parallel - graph-based reasoning can merge insights"
		rec.Alternative = AlgorithmSequential
		rec.Confidence = 0.85

	case chars.RequiresVerification && chars.IsFactual:
		// Dialectic with tools for verification needs
		rec.Algorithm = AlgorithmDialectic
		rec.Reasoning = "Problem requires verification - dialectic with tools can fact-check claims"
		rec.Alternative = AlgorithmReflexion
		rec.Confidence = 0.75

	case chars.Complexity < 0.4:
		// Simple problems -> sequential
		rec.Algorithm = AlgorithmSequential
		rec.Reasoning = "Straightforward problem best solved with linear chain-of-thought"
		rec.Confidence = 0.9

	default:
		// Default to sequential for moderate complexity
		rec.Algorithm = AlgorithmSequential
		rec.Reasoning = "Standard problem suitable for sequential reasoning"
		rec.Alternative = AlgorithmGoT
		rec.Confidence = 0.7
	}

	return rec
}

// Select analyzes a problem and returns the recommended algorithm
func (s *AlgorithmSelector) Select(ctx context.Context, problem string) (*AlgorithmRecommendation, error) {
	chars, err := s.Analyze(ctx, problem)
	if err != nil {
		// Fall back to default recommendation
		return &AlgorithmRecommendation{
			Algorithm:  AlgorithmSequential,
			Confidence: 0.5,
			Reasoning:  "Unable to analyze problem, defaulting to sequential reasoning",
		}, nil
	}

	return s.Recommend(chars), nil
}

// QuickSelect performs fast heuristic-based selection without LLM
func QuickSelect(problem string) AlgorithmType {
	problemLower := strings.ToLower(problem)

	// Check for dialectic indicators
	dialecticPatterns := []string{
		"debate", "argue", "pros and cons", "versus", "vs",
		"compare", "contrast", "opinion", "perspective",
		"controversial", "disagree", "should we", "is it better",
	}
	for _, pattern := range dialecticPatterns {
		if strings.Contains(problemLower, pattern) {
			return AlgorithmDialectic
		}
	}

	// Check for reflexion indicators
	reflexionPatterns := []string{
		"solve", "puzzle", "riddle", "trick question",
		"what went wrong", "mistake", "error", "debug",
		"step by step", "work through",
	}
	for _, pattern := range reflexionPatterns {
		if strings.Contains(problemLower, pattern) {
			return AlgorithmReflexion
		}
	}

	// Check for GoT indicators
	gotPatterns := []string{
		"explore", "brainstorm", "creative", "generate ideas",
		"multiple ways", "different approaches", "alternatives",
		"all possible", "comprehensive",
	}
	for _, pattern := range gotPatterns {
		if strings.Contains(problemLower, pattern) {
			return AlgorithmGoT
		}
	}

	// Default to sequential
	return AlgorithmSequential
}

// AutoReason automatically selects and runs the best algorithm for a problem
func AutoReason(ctx context.Context, provider Provider, problem string, useQuickSelect bool) (interface{}, AlgorithmType, error) {
	return AutoReasonWithProviders(ctx, provider, provider, problem, useQuickSelect)
}

// AutoReasonWithProviders automatically selects and runs the best algorithm with separate analysis/execution providers
func AutoReasonWithProviders(ctx context.Context, analysisProvider Provider, executionProvider Provider, problem string, useQuickSelect bool) (interface{}, AlgorithmType, error) {
	var algorithm AlgorithmType

	if useQuickSelect {
		algorithm = QuickSelect(problem)
		log.Printf("[AUTO-REASON] Quick select chose: %s", algorithm)
	} else {
		// Use analysis provider (cheaper) for problem classification
		selector := NewAlgorithmSelector(analysisProvider)
		rec, err := selector.Select(ctx, problem)
		if err != nil {
			algorithm = AlgorithmSequential
		} else {
			algorithm = rec.Algorithm
			log.Printf("[AUTO-REASON] Selector chose %s (confidence: %.2f): %s", rec.Algorithm, rec.Confidence, rec.Reasoning)
		}
	}

	// Execute the selected algorithm with execution provider
	switch algorithm {
	case AlgorithmSequential:
		selector := NewProviderSelector(executionProvider, nil, nil, "single", 0)
		client := &SequentialClient{selector: selector}
		result, err := client.Think(ctx, problem, 10)
		return result, algorithm, err

	case AlgorithmGoT:
		fileConfig, _ := GetFileConfig()
		config := fileConfig.GetDefaultGoTConfig()
		got := NewGraphOfThoughts(executionProvider, config)
		result, err := got.Solve(ctx, problem)
		return result, algorithm, err

	case AlgorithmReflexion:
		fileConfig, _ := GetFileConfig()
		config := fileConfig.GetDefaultReflexionConfig()
		reflexion := NewReflexion(executionProvider, config)
		result, err := reflexion.Reason(ctx, problem)
		return result, algorithm, err

	case AlgorithmDialectic:
		fileConfig, _ := GetFileConfig()
		config := fileConfig.GetDefaultDialecticConfig()
		dialectic := NewDialecticalReasoner(executionProvider, config)
		result, err := dialectic.Reason(ctx, problem)
		return result, algorithm, err

	default:
		return nil, "", fmt.Errorf("unknown algorithm: %s", algorithm)
	}
}

// FormatRecommendation formats a recommendation for display
func FormatRecommendation(rec *AlgorithmRecommendation) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Recommended Algorithm: %s\n", rec.Algorithm))
	sb.WriteString(fmt.Sprintf("Confidence: %.0f%%\n", rec.Confidence*100))
	sb.WriteString(fmt.Sprintf("Reasoning: %s\n", rec.Reasoning))

	if rec.Alternative != "" {
		sb.WriteString(fmt.Sprintf("Alternative: %s\n", rec.Alternative))
	}

	return sb.String()
}
