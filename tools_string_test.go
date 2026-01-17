package main

import (
	"context"
	"testing"
)

func TestStringToolEdgeCases(t *testing.T) {
	tool := &StringTool{}

	testCases := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{"empty", "", "", true},
		{"invalid format", "no colon", "", true},
		{"length basic", "length:hello", "5", false},
		{"length empty", "length:", "0", false},
		{"length special", "length:hello world", "11", false},
		{"upper", "upper:hello", "HELLO", false},
		{"lower", "lower:HELLO", "hello", false},
		{"reverse", "reverse:abc", "cba", false},
		{"reverse empty", "reverse:", "", false},
		{"count", "count:l,hello", "2", false},
		{"count not found", "count:x,hello", "0", false},
		{"split empty delimiter error", "split:,,hello", "", true},
		{"split pipe", "split:|,a|b|c", "[a b c]", false},
		{"split space", "split: ,hello world test", "[hello world test]", false},
		{"split newline", "split:\n,line1\nline2\nline3", "[line1 line2 line3]", false},
		{"replace", "replace:o,e,hello", "helle", false},
		{"replace special", "replace: ,_,hello world", "hello_world", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), tc.input)
			if tc.hasError {
				if err == nil {
					t.Errorf("Input %q: expected error but got none", tc.input)
				}
			} else {
				if err != nil {
					t.Errorf("Input %q: unexpected error: %v", tc.input, err)
					return
				}
				if result != tc.expected {
					t.Errorf("Input %q: expected %q, got %q", tc.input, tc.expected, result)
				}
			}
		})
	}
}
