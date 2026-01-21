// Package utils provides shared utility functions for the reasoning-tools MCP server.
package utils

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// StripChainOfThought removes common chain-of-thought reasoning patterns from model output.
// This handles reasoning models (like z.ai glm-4.7) that output their thinking process.
func StripChainOfThought(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	// Patterns that indicate chain-of-thought reasoning to remove
	cotPatterns := []string{
		`(?m)^\s*\*?\*?\d+\.\s+\*?\*?(?:Analyze|Deconstruct|Brainstorm|Draft|Review|Refine|Select|Identify|Synthesize|Evaluate|Construct|Final)[^:]*:\*?\*?\s*`,
		`(?m)^\s*\*?\*?(?:Step|Phase|Stage)\s*\d+[:\-]\*?\*?\s*`,
		`(?m)^\s*\*?\*?(?:Let me|I need to|I will|I should|First,|Now,|Next,|Finally,)\s*`,
	}

	// Check if the response looks like chain-of-thought
	isCot := false
	for _, pattern := range cotPatterns {
		re := regexp.MustCompile(pattern)
		if re.MatchString(s) {
			isCot = true
			break
		}
	}

	if !isCot {
		return s
	}

	// Try to extract the final output from chain-of-thought
	// Look for patterns like "Final Answer:", "Draft X:", etc. and take the last one
	finalPatterns := []regexp.Regexp{
		*regexp.MustCompile(`(?is)(?:final\s+(?:answer|output|response|polish|version|draft)[:\-]?\s*)(.*?)(?:\n\n\d+\.|\z)`),
		*regexp.MustCompile(`(?is)(?:draft\s*\d+[:\-]?\s*\([^)]*\)[:\-]?\s*)(.*?)(?:\n\n\d+\.|\n\n\*|\z)`),
	}

	for _, re := range finalPatterns {
		matches := re.FindAllStringSubmatch(s, -1)
		if len(matches) > 0 {
			lastMatch := matches[len(matches)-1]
			if len(lastMatch) > 1 {
				extracted := strings.TrimSpace(lastMatch[1])
				if len(extracted) > 50 { // Only use if substantial
					return cleanOutput(extracted)
				}
			}
		}
	}

	// If no final section found, try to extract non-numbered content
	// Split by double newlines and take paragraphs that don't start with numbered steps
	paragraphs := strings.Split(s, "\n\n")
	var cleanParagraphs []string

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		// Skip paragraphs that look like numbered analysis steps
		if regexp.MustCompile(`^\s*\*?\*?\d+\.\s+\*?\*?(?:Analyze|Deconstruct|Brainstorm|Draft|Review)`).MatchString(p) {
			continue
		}
		// Skip paragraphs starting with bullet points that describe process
		if regexp.MustCompile(`^\s*[\*\-]\s*\*?(?:Draft|Attempt|Step|Goal|Task|Role|Input|Output)`).MatchString(p) {
			continue
		}
		if p != "" {
			cleanParagraphs = append(cleanParagraphs, p)
		}
	}

	if len(cleanParagraphs) > 0 {
		// Return the last substantial paragraph (likely the final output)
		for i := len(cleanParagraphs) - 1; i >= 0; i-- {
			if len(cleanParagraphs[i]) > 50 {
				return cleanOutput(cleanParagraphs[i])
			}
		}
		return cleanOutput(cleanParagraphs[len(cleanParagraphs)-1])
	}

	// Fallback: return original with basic cleanup
	return cleanOutput(s)
}

// cleanOutput performs basic cleanup on extracted output
func cleanOutput(s string) string {
	s = strings.TrimSpace(s)
	// Remove trailing incomplete sentences (ending with "...")
	if strings.HasSuffix(s, "...") && !strings.HasSuffix(s, "...\"") {
		// Find the last complete sentence
		lastPeriod := strings.LastIndex(s[:len(s)-3], ".")
		if lastPeriod > len(s)/2 { // Only trim if we keep most of the content
			s = s[:lastPeriod+1]
		}
	}
	return s
}

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
