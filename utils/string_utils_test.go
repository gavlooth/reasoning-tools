package utils

import (
	"testing"
	"unicode/utf8"
)

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"Empty string", "", 10, ""},
		{"Short ASCII", "hello", 10, "hello"},
		{"Exact length ASCII", "hello", 5, "hello"},
		{"Truncate ASCII", "hello world", 5, "hello..."},
		{"Short Unicode - Chinese", "你好", 5, "你好"},
		{"Truncate Unicode - Chinese", "你好世界", 2, "你好..."},
		{"Truncate Unicode - Emoji", "Hi 👋 World", 4, "Hi 👋..."},
		{"Truncate Unicode - Mixed", "Hello 世界", 8, "Hello 世界"},
		{"Truncate Unicode - Mixed middle", "Hello 世界", 7, "Hello 世..."},
		{"Truncate Unicode - Mixed shorter", "Hello 世界", 5, "Hello..."},
		{"Truncate single char", "Hello", 1, "H..."},
		{"Zero maxLen", "Hello", 0, "..."},
		{"Korean text", "안녕하세요", 3, "안녕하..."},
		{"Emoji sequence", "😀😁😂🤣😃", 3, "😀😁😂..."},
		{"Combining diacritics", "café", 4, "café"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateStr(tt.input, tt.maxLen)
			if got != tt.expected {
				t.Errorf("TruncateStr(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
			}
			// Verify output is valid UTF-8
			if !utf8.ValidString(got) {
				t.Errorf("TruncateStr(%q, %d) returned invalid UTF-8: %q", tt.input, tt.maxLen, got)
			}
		})
	}
}

func TestTruncateStrBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"Empty string", "", 10, ""},
		{"Short ASCII", "hello", 10, "hello"},
		{"Exact length ASCII", "hello", 5, "hello"},
		{"Truncate ASCII", "hello world", 5, "hello..."},
		{"Truncate Unicode - byte based", "hello 世界", 10, "hello 世\xe7..."},
		{"Truncate emoji - byte based", "Hi 👋", 5, "Hi \xf0\x9f..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateStrBytes(tt.input, tt.maxLen)
			if got != tt.expected {
				t.Errorf("TruncateStrBytes(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
			}
		})
	}
}

func TestTruncateStrBytesSafe(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"Empty string", "", 10, ""},
		{"Short ASCII", "hello", 10, "hello"},
		{"Exact length ASCII", "hello", 5, "hello"},
		{"Truncate ASCII", "hello world", 5, "hello..."},
		{"Truncate Unicode - byte based", "hello 世界", 10, "hello 世..."},
		{"Truncate emoji - byte based", "Hi 👋", 5, "Hi ..."},
		{"Truncate mid-emoji", "😀😁😂", 5, "😀..."},
		{"Truncate to zero", "Hello", 0, "..."},
		{"Truncate Chinese", "你好世界", 7, "你好..."},
		{"Single emoji truncated", "👋", 2, "..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateStrBytesSafe(tt.input, tt.maxLen)
			if got != tt.expected {
				t.Errorf("TruncateStrBytesSafe(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
			}
			// Verify output is always valid UTF-8
			if !utf8.ValidString(got) {
				t.Errorf("TruncateStrBytesSafe(%q, %d) returned invalid UTF-8: %q", tt.input, tt.maxLen, got)
			}
		})
	}
}

func TestTruncateStrUTF8Safety(t *testing.T) {
	// Test that we never produce invalid UTF-8
	testCases := []string{
		"Hello 世界",       // Chinese characters (3 bytes each)
		"Привет мир",      // Cyrillic (2 bytes each)
		"مرحبا",           // Arabic
		"こんにちは",        // Japanese Hiragana (3 bytes each)
		"😀😁😂🤣😃😄😅😆",  // Emoji (4 bytes each)
		"école",           // Combining diacritics
		"â",               // Combining circumflex
	}

	for _, input := range testCases {
		for maxLen := 0; maxLen <= 20; maxLen++ {
			result := TruncateStr(input, maxLen)
			if !utf8.ValidString(result) {
				t.Errorf("TruncateStr(%q, %d) = %q is not valid UTF-8", input, maxLen, result)
			}
		}
	}
}

func TestTruncateStrCharacterCount(t *testing.T) {
	// Verify that we're actually counting characters (runes), not bytes
	tests := []struct {
		input       string
		maxLen      int
		wantRunes   int
		description string
	}{
		{"Hello 世界", 7, 10, "All characters (7 + 3 dots) should fit"},
		{"Hello 世界", 5, 8, "5 characters + 3 dots"},
		{"😀😁😂🤣", 3, 6, "3 emoji + 3 dots"},
		{"Привет", 3, 6, "3 Cyrillic chars + 3 dots"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := TruncateStr(tt.input, tt.maxLen)
			gotRunes := utf8.RuneCountInString(result)
			if gotRunes != tt.wantRunes {
				t.Errorf("TruncateStr(%q, %d) = %q has %d runes, want %d",
					tt.input, tt.maxLen, result, gotRunes, tt.wantRunes)
			}
		})
	}
}
