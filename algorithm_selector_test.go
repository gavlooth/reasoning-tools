package main

import (
	"testing"
)

func TestQuickSelect(t *testing.T) {
	tests := []struct {
		name     string
		problem  string
		expected AlgorithmType
	}{
		{
			name:     "debate problem",
			problem:  "What are the pros and cons of remote work?",
			expected: AlgorithmDialectic,
		},
		{
			name:     "compare problem",
			problem:  "Compare and contrast Python versus Go for web development",
			expected: AlgorithmDialectic,
		},
		{
			name:     "puzzle problem",
			problem:  "Solve this puzzle: a farmer needs to cross a river...",
			expected: AlgorithmReflexion,
		},
		{
			name:     "debug problem",
			problem:  "Debug this code that has an error in the loop",
			expected: AlgorithmReflexion,
		},
		{
			name:     "brainstorm problem",
			problem:  "Brainstorm creative ideas for a new mobile app",
			expected: AlgorithmGoT,
		},
		{
			name:     "explore problem",
			problem:  "Explore all possible solutions to optimize this algorithm",
			expected: AlgorithmGoT,
		},
		{
			name:     "simple problem",
			problem:  "Calculate the sum of 15 and 27",
			expected: AlgorithmSequential,
		},
		{
			name:     "explain problem",
			problem:  "Explain how photosynthesis works",
			expected: AlgorithmSequential,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QuickSelect(tt.problem)
			if got != tt.expected {
				t.Errorf("QuickSelect(%q) = %v, want %v", tt.problem, got, tt.expected)
			}
		})
	}
}

func TestAlgorithmRecommendation(t *testing.T) {
	selector := &AlgorithmSelector{}

	tests := []struct {
		name     string
		chars    *ProblemCharacteristics
		expected AlgorithmType
	}{
		{
			name: "controversial topic",
			chars: &ProblemCharacteristics{
				IsControversial: true,
				Complexity:      0.7,
			},
			expected: AlgorithmDialectic,
		},
		{
			name: "multiple perspectives",
			chars: &ProblemCharacteristics{
				HasMultiplePerspectives: true,
				Complexity:              0.5,
			},
			expected: AlgorithmDialectic,
		},
		{
			name: "requires iteration",
			chars: &ProblemCharacteristics{
				RequiresIteration: true,
				Complexity:        0.7,
				IsFactual:         false,
			},
			expected: AlgorithmReflexion,
		},
		{
			name: "complex exploration",
			chars: &ProblemCharacteristics{
				RequiresExploration: true,
				Complexity:          0.8,
				IsFactual:           true, // Set to true to avoid Reflexion trigger from complexity
			},
			expected: AlgorithmGoT,
		},
		{
			name: "simple problem",
			chars: &ProblemCharacteristics{
				Complexity: 0.2,
			},
			expected: AlgorithmSequential,
		},
		{
			name: "verification with facts",
			chars: &ProblemCharacteristics{
				RequiresVerification: true,
				IsFactual:            true,
			},
			expected: AlgorithmDialectic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := selector.Recommend(tt.chars)
			if rec.Algorithm != tt.expected {
				t.Errorf("Recommend() = %v, want %v", rec.Algorithm, tt.expected)
			}
			if rec.Confidence <= 0 || rec.Confidence > 1 {
				t.Errorf("Confidence should be between 0 and 1, got %v", rec.Confidence)
			}
			if rec.Reasoning == "" {
				t.Error("Reasoning should not be empty")
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple json",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "json with text before",
			input:    `Here is the result: {"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "json with text after",
			input:    `{"key": "value"} That's the answer.`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "nested json",
			input:    `{"outer": {"inner": "value"}}`,
			expected: `{"outer": {"inner": "value"}}`,
		},
		{
			name:     "no json",
			input:    "just some text",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.expected {
				t.Errorf("extractJSON(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
