package utils

import "testing"

// TestExtractJSON tests the ExtractJSON function with various edge cases
func TestExtractJSON(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple object",
			input:    `{"key":"value"}`,
			expected: `{"key":"value"}`,
		},
		{
			name:     "object with text before",
			input:    `Here is the JSON: {"key":"value"}`,
			expected: `{"key":"value"}`,
		},
		{
			name:     "object with text after",
			input:    `{"key":"value"} and more text`,
			expected: `{"key":"value"}`,
		},
		{
			name:     "nested objects",
			input:    `{"outer":{"inner":"value"}}`,
			expected: `{"outer":{"inner":"value"}}`,
		},
		{
			name:     "object with string containing braces",
			input:    `{"text":"{hello}"}`,
			expected: `{"text":"{hello}"}`,
		},
		{
			name:     "object with escaped quote",
			input:    `{"text":"hello \"world\""}`,
			expected: `{"text":"hello \"world\""}`,
		},
		{
			name:     "object with escaped backslash",
			input:    `{"text":"hello\\world"}`,
			expected: `{"text":"hello\\world"}`,
		},
		{
			name:     "object with string containing nested braces",
			input:    `{"text":"{a:{b:c}}"}`,
			expected: `{"text":"{a:{b:c}}"}`,
		},
		{
			name:     "empty object",
			input:    `{}`,
			expected: `{}`,
		},
		{
			name:     "no JSON",
			input:    `just text here`,
			expected: ``,
		},
		{
			name:     "incomplete JSON",
			input:    `{"key":"value"`,
			expected: ``,
		},
		{
			name:     "object with array containing strings with braces",
			input:    `{"arr":["{a}","{b}"]}`,
			expected: `{"arr":["{a}","{b}"]}`,
		},
		{
			name:     "complex nested with escaped quotes in strings",
			input:    `{"outer":"{\"inner\":\"value\"}"}`,
			expected: `{"outer":"{\"inner\":\"value\"}"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractJSON(tc.input)
			if result != tc.expected {
				t.Errorf("ExtractJSON(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// TestExtractJSONArray tests the ExtractJSONArray function with various edge cases
func TestExtractJSONArray(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple array",
			input:    `["a","b","c"]`,
			expected: `["a","b","c"]`,
		},
		{
			name:     "array with text before",
			input:    `Response: ["a","b","c"]`,
			expected: `["a","b","c"]`,
		},
		{
			name:     "array with text after",
			input:    `["a","b","c"] done`,
			expected: `["a","b","c"]`,
		},
		{
			name:     "nested arrays",
			input:    `[["a","b"],["c","d"]]`,
			expected: `[["a","b"],["c","d"]]`,
		},
		{
			name:     "array with strings containing brackets",
			input:    `["a[hello]","b[test]"]`,
			expected: `["a[hello]","b[test]"]`,
		},
		{
			name:     "array with escaped quote",
			input:    `["hello \"world\""]`,
			expected: `["hello \"world\""]`,
		},
		{
			name:     "array with escaped backslash",
			input:    `["hello\\world"]`,
			expected: `["hello\\world"]`,
		},
		{
			name:     "array with string containing nested brackets",
			input:    `["[a:[b:c]]","[x:[y:z]]"]`,
			expected: `["[a:[b:c]]","[x:[y:z]]"]`,
		},
		{
			name:     "empty array",
			input:    `[]`,
			expected: `[]`,
		},
		{
			name:     "no JSON",
			input:    `just text here`,
			expected: ``,
		},
		{
			name:     "incomplete JSON",
			input:    `["a","b"`,
			expected: ``,
		},
		{
			name:     "array with objects containing strings with brackets",
			input:    `[{"text":"{a}"},{"text":"{b}"}]`,
			expected: `[{"text":"{a}"},{"text":"{b}"}]`,
		},
		{
			name:     "array with escaped quotes in strings",
			input:    `["{\"inner\":\"value\"}"]`,
			expected: `["{\"inner\":\"value\"}"]`,
		},
		{
			name:     "array with mixed nested structures",
			input:    `[["a",["b","c"]],"d"]`,
			expected: `[["a",["b","c"]],"d"]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractJSONArray(tc.input)
			if result != tc.expected {
				t.Errorf("ExtractJSONArray(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}
