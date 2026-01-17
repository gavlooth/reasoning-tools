// Package utils provides shared utility functions for the reasoning-tools MCP server.
package utils

import "unicode/utf8"

// TruncateStr truncates a string to the specified maximum number of UTF-8 characters.
// If the string has fewer than or equal to maxLen characters, returns the string as is.
// Otherwise, truncates the string to maxLen characters and appends "...".
// This function is UTF-8 safe and will not split multi-byte characters.
func TruncateStr(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	// Find the byte position of the maxLen-th rune
	byteCount := 0
	for i := 0; i < maxLen; i++ {
		_, size := utf8.DecodeRuneInString(s[byteCount:])
		if size == 0 {
			break
		}
		byteCount += size
	}
	return s[:byteCount] + "..."
}

// TruncateStrBytes truncates a string to the specified maximum byte length.
// If the string byte length is less than or equal to maxLen, returns the string as is.
// Otherwise, truncates the string to maxLen bytes and appends "...".
// WARNING: This function is NOT UTF-8 safe and may split multi-byte characters.
// Only use this when byte-level truncation is specifically required (e.g., for network protocols).
// For UTF-8 safe byte truncation, use TruncateStrBytesSafe instead.
func TruncateStrBytes(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// TruncateStrBytesSafe truncates a string to the specified maximum byte length
// while ensuring the result is always valid UTF-8. If the string byte length is
// less than or equal to maxLen, returns the string as is. Otherwise, finds the
// nearest valid UTF-8 boundary at or before maxLen bytes, truncates there, and appends "...".
func TruncateStrBytesSafe(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	// Find the last valid UTF-8 boundary at or before maxLen
	for i := maxLen; i > 0; i-- {
		if utf8.RuneStart(s[i]) {
			return s[:i] + "..."
		}
	}

	// If we can't find a valid boundary (very unlikely), truncate at 0
	return "..."
}
