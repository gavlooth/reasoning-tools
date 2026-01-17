// Package utils provides shared utility functions for the reasoning-tools MCP server.
package utils

// extractJSONCore is a shared core parser for extracting JSON structures from strings.
// It handles both objects (delimited by {}) and arrays (delimited by []).
// The function correctly handles nested structures, escape sequences, and JSON string literals.
//
// Parameters:
//   - s: The input string that may contain JSON
//   - startChar: The opening character to search for ('{' for objects, '[' for arrays)
//   - endChar: The closing character to match ('}' for objects, ']' for arrays)
//
// Returns the extracted JSON string, or empty string if no valid JSON structure is found.
func extractJSONCore(s string, startChar, endChar byte) string {
	start := -1
	depth := 0
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if escaped {
			// After escape char, skip the next character
			escaped = false
			continue
		}

		switch c {
		case '\\':
			// Escape character - next char is literal
			escaped = true
		case '"':
			// Toggle string state
			inString = !inString
		case startChar, endChar:
			// Only count delimiters outside of strings
			if !inString {
				if c == startChar && start == -1 {
					start = i
					depth++
				} else if c == startChar {
					depth++
				} else if c == endChar && start != -1 {
					depth--
					if depth == 0 {
						return s[start : i+1]
					}
				}
			}
		}
	}
	return ""
}

// ExtractJSON extracts a JSON object from a string that might have extra text.
// It handles nested objects and JSON string literals correctly.
func ExtractJSON(s string) string {
	return extractJSONCore(s, '{', '}')
}

// ExtractJSONArray extracts a JSON array from a string that might have extra text.
// It handles nested arrays and JSON string literals correctly.
func ExtractJSONArray(s string) string {
	return extractJSONCore(s, '[', ']')
}
