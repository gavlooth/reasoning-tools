package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Tool represents an executable tool available during reasoning
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  string `json:"parameters"` // JSON schema or description
}

// ToolCall represents a request to use a tool
type ToolCall struct {
	Tool   string `json:"tool"`
	Input  string `json:"input"`
	Reason string `json:"reason,omitempty"` // Why the tool is being used
}

// ToolResult represents the result of a tool call
type ToolResult struct {
	Tool    string `json:"tool"`
	Input   string `json:"input"`
	Output  string `json:"output"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// ToolRegistry manages available tools
type ToolRegistry struct {
	tools   map[string]ToolExecutor
	enabled map[string]bool
}

// ToolExecutor is the interface for tool implementations
type ToolExecutor interface {
	Name() string
	Description() string
	Execute(ctx context.Context, input string) (string, error)
}

// NewToolRegistry creates a new tool registry with built-in tools
func NewToolRegistry() *ToolRegistry {
	registry := &ToolRegistry{
		tools:   make(map[string]ToolExecutor),
		enabled: make(map[string]bool),
	}

	// Register built-in tools
	registry.Register(&CalculatorTool{})
	registry.Register(&CodeExecutorTool{})
	registry.Register(&WebFetchTool{})
	registry.Register(&StringTool{})

	// Enable tools by default, EXCEPT code_exec which requires explicit opt-in
	// due to security implications
	for name := range registry.tools {
		if name == "code_exec" {
			// code_exec is disabled by default for security reasons
			// users must explicitly enable it
			registry.enabled[name] = false
		} else {
			registry.enabled[name] = true
		}
	}

	return registry
}

// Register adds a tool to the registry
func (r *ToolRegistry) Register(tool ToolExecutor) {
	r.tools[tool.Name()] = tool
}

// Enable enables a tool
func (r *ToolRegistry) Enable(name string) {
	r.enabled[name] = true
}

// Disable disables a tool
func (r *ToolRegistry) Disable(name string) {
	r.enabled[name] = false
}

// SetEnabled sets which tools are enabled
// Note: code_exec requires explicit opt-in via CODE_EXEC_ENABLED environment variable
// for security reasons and cannot be enabled through this method alone.
func (r *ToolRegistry) SetEnabled(names []string) {
	// Disable all first
	for name := range r.enabled {
		r.enabled[name] = false
	}
	// Enable specified tools (except code_exec which requires env var)
	for _, name := range names {
		if _, exists := r.tools[name]; exists {
			// code_exec requires explicit environment variable opt-in
			if name == "code_exec" {
				if os.Getenv("CODE_EXEC_ENABLED") != "true" && os.Getenv("CODE_EXEC_ENABLED") != "1" {
					continue // Skip enabling code_exec without explicit opt-in
				}
			}
			r.enabled[name] = true
		}
	}
}

// Execute runs a tool by name
func (r *ToolRegistry) Execute(ctx context.Context, name, input string) ToolResult {
	result := ToolResult{
		Tool:  name,
		Input: input,
	}

	tool, exists := r.tools[name]
	if !exists {
		result.Error = fmt.Sprintf("unknown tool: %s", name)
		return result
	}

	if !r.enabled[name] {
		result.Error = fmt.Sprintf("tool disabled: %s", name)
		return result
	}

	output, err := tool.Execute(ctx, input)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	result.Output = output
	result.Success = true
	return result
}

// GetAvailableTools returns descriptions of enabled tools
func (r *ToolRegistry) GetAvailableTools() []Tool {
	var tools []Tool
	for name, tool := range r.tools {
		if r.enabled[name] {
			tools = append(tools, Tool{
				Name:        tool.Name(),
				Description: tool.Description(),
			})
		}
	}
	return tools
}

// GetRegisteredToolNames returns all registered tool names (including disabled ones)
func (r *ToolRegistry) GetRegisteredToolNames() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// GetToolsPrompt generates a prompt describing available tools
func (r *ToolRegistry) GetToolsPrompt() string {
	tools := r.GetAvailableTools()
	if len(tools) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("You have access to the following tools. Use them when needed by outputting a tool call in your response:\n\n")

	for _, tool := range tools {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", tool.Name, tool.Description))
	}

	sb.WriteString("\nTo use a tool, include in your response:\n")
	sb.WriteString("```tool\n{\"tool\": \"tool_name\", \"input\": \"your input\", \"reason\": \"why you need this\"}\n```\n")
	sb.WriteString("\nWait for the tool result before continuing your reasoning.\n")

	return sb.String()
}

// ParseToolCalls extracts tool calls from LLM response
func ParseToolCalls(response string) []ToolCall {
	var calls []ToolCall

	// Look for ```tool ... ``` blocks
	re := regexp.MustCompile("(?s)```tool\\s*\\n?(.*?)\\n?```")
	matches := re.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		if len(match) > 1 {
			var call ToolCall
			if err := json.Unmarshal([]byte(strings.TrimSpace(match[1])), &call); err == nil {
				calls = append(calls, call)
			}
		}
	}

	// Also look for inline JSON tool calls
	re2 := regexp.MustCompile(`\{"tool":\s*"([^"]+)"[^}]*\}`)
	matches2 := re2.FindAllString(response, -1)
	for _, match := range matches2 {
		var call ToolCall
		if err := json.Unmarshal([]byte(match), &call); err == nil {
			// Avoid duplicates
			found := false
			for _, existing := range calls {
				if existing.Tool == call.Tool && existing.Input == call.Input {
					found = true
					break
				}
			}
			if !found {
				calls = append(calls, call)
			}
		}
	}

	return calls
}

// ============ Calculator Tool ============

type CalculatorTool struct{}

func (t *CalculatorTool) Name() string {
	return "calculator"
}

func (t *CalculatorTool) Description() string {
	return "Evaluate mathematical expressions. Input: math expression (e.g., '2 + 3 * 4', 'sqrt(16)', 'sin(3.14159/2)'). Supports: +, -, *, /, ^, sqrt, sin, cos, tan, log, exp, abs, floor, ceil, round, pi, e"
}

func (t *CalculatorTool) Execute(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty expression")
	}

	result, err := evaluateMathExpr(input)
	if err != nil {
		return "", fmt.Errorf("calculation error for expression %q: %w", input, err)
	}

	// Format result nicely
	if result == float64(int64(result)) {
		return fmt.Sprintf("%.0f", result), nil
	}

	// Use threshold-based formatting for non-integer results
	// to avoid scientific notation for values like 1e-5 or 1e10
	absResult := math.Abs(result)
	if absResult < 0.00001 && absResult > 0 {
		// Very small numbers: use high precision fixed-point (e.g., 1e-5 -> 0.00001)
		return strconv.FormatFloat(result, 'f', 10, 64), nil
	}
	if absResult >= 10000 {
		// Large numbers: also use fixed-point (e.g., 1e10 -> 10000000000)
		return strconv.FormatFloat(result, 'f', 0, 64), nil
	}
	// For numbers in the "sweet spot" (0.00001 to 10000), use fixed-point with reasonable precision
	// Precision of 10 should cover most cases while removing trailing zeros
	formatted := strconv.FormatFloat(result, 'f', 10, 64)
	// Remove trailing zeros after decimal point
	formatted = strings.TrimRight(formatted, "0")
	formatted = strings.TrimRight(formatted, ".")
	return formatted, nil
}

// evaluateMathExpr evaluates a mathematical expression safely
func evaluateMathExpr(expr string) (float64, error) {
	// Normalize the expression
	expr = strings.ToLower(expr)
	expr = strings.ReplaceAll(expr, " ", "")

	// Replace constants
	// For "pi" we can use simple replacement since it's unlikely to conflict
	expr = strings.ReplaceAll(expr, "pi", fmt.Sprintf("%.15f", math.Pi))
	// For "e" we need to be careful NOT to replace it when part of scientific notation (e.g., 1e10)
	expr = replaceConstant(expr, "e", fmt.Sprintf("%.15f", math.E))

	// Replace ^ with ** for power (we'll handle it)
	// First handle functions
	expr = replaceFunctions(expr)

	// Parse and evaluate
	return parseExpr(expr)
}

func replaceFunctions(expr string) string {
	funcs := map[string]func(float64) float64{
		"sqrt":  math.Sqrt,
		"sin":   math.Sin,
		"cos":   math.Cos,
		"tan":   math.Tan,
		"log":   math.Log,
		"log10": math.Log10,
		"exp":   math.Exp,
		"abs":   math.Abs,
		"floor": math.Floor,
		"ceil":  math.Ceil,
		"round": math.Round,
	}

	for name, fn := range funcs {
		for {
			re := regexp.MustCompile(name + `\(([^()]+)\)`)
			match := re.FindStringSubmatchIndex(expr)
			if match == nil {
				break
			}
			inner := expr[match[2]:match[3]]
			innerVal, err := parseExpr(inner)
			if err != nil {
				break
			}
			result := fn(innerVal)
			expr = expr[:match[0]] + fmt.Sprintf("%.15f", result) + expr[match[1]:]
		}
	}

	return expr
}

// replaceConstant replaces a mathematical constant (like "e" or "pi") with its value,
// but only when it's not part of a larger identifier or scientific notation.
// For example, in "1e10", the "e" should NOT be replaced.
func replaceConstant(expr, constant, value string) string {
	// Pattern matches the constant only when:
	// - It's preceded by start of string or a non-alphanumeric character
	// - It's followed by end of string or a non-alphanumeric, non-digit character
	// This prevents matching "e" inside "1e10" or "e" as part of a variable name
	pattern := `(^|[^a-zA-Z0-9])` + regexp.QuoteMeta(constant) + `($|[^a-zA-Z0-9])`
	re := regexp.MustCompile(pattern)

	// Replace while preserving the surrounding characters
	return re.ReplaceAllString(expr, `$1`+value+`$2`)
}

func parseExpr(expr string) (float64, error) {
	// Handle parentheses first
	for strings.Contains(expr, "(") {
		re := regexp.MustCompile(`\(([^()]+)\)`)
		match := re.FindStringSubmatchIndex(expr)
		if match == nil {
			return 0, fmt.Errorf("mismatched parentheses")
		}
		inner := expr[match[2]:match[3]]
		innerVal, err := parseExpr(inner)
		if err != nil {
			return 0, err
		}
		expr = expr[:match[0]] + fmt.Sprintf("%.15f", innerVal) + expr[match[1]:]
	}

	// Parse addition/subtraction (lowest precedence)
	// Split on + and - while respecting negative numbers
	return parseAddSub(expr)
}

func parseAddSub(expr string) (float64, error) {
	// Find last + or - that's not part of a number (e.g., scientific notation)
	for i := len(expr) - 1; i > 0; i-- {
		c := expr[i]
		if c == '+' || c == '-' {
			// Make sure it's not part of scientific notation
			if i > 0 && (expr[i-1] == 'e' || expr[i-1] == 'E') {
				continue
			}
			left, err := parseAddSub(expr[:i])
			if err != nil {
				continue // Try earlier position
			}
			right, err := parseMulDiv(expr[i+1:])
			if err != nil {
				return 0, err
			}
			if c == '+' {
				return left + right, nil
			}
			return left - right, nil
		}
	}
	return parseMulDiv(expr)
}

func parseMulDiv(expr string) (float64, error) {
	// Find last * or /
	for i := len(expr) - 1; i >= 0; i-- {
		c := expr[i]
		if c == '*' || c == '/' {
			left, err := parseMulDiv(expr[:i])
			if err != nil {
				return 0, err
			}
			right, err := parsePow(expr[i+1:])
			if err != nil {
				return 0, err
			}
			if c == '*' {
				return left * right, nil
			}
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			return left / right, nil
		}
	}
	return parsePow(expr)
}

func parsePow(expr string) (float64, error) {
	// Handle power operator ^
	if idx := strings.LastIndex(expr, "^"); idx > 0 {
		base, err := parsePow(expr[:idx])
		if err != nil {
			return 0, err
		}
		exp, err := parseNumber(expr[idx+1:])
		if err != nil {
			return 0, err
		}
		return math.Pow(base, exp), nil
	}
	return parseNumber(expr)
}

func parseNumber(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty number")
	}
	return strconv.ParseFloat(s, 64)
}

// ============ Code Executor Tool ============

type CodeExecutorTool struct{}

func (t *CodeExecutorTool) Name() string {
	return "code_exec"
}

func (t *CodeExecutorTool) Description() string {
	return "Execute Python code and return the output. Input: Python code snippet. WARNING: This tool executes code on the host system without full sandboxing. Only use in trusted environments. The code runs with a 10s timeout and basic restrictions. Print results to see them."
}

func (t *CodeExecutorTool) Execute(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty code")
	}

	// Security: Validate input against dangerous patterns
	if err := validatePythonCode(ctx, input); err != nil {
		return "", fmt.Errorf("code validation failed: %w", err)
	}

	// Get configurable timeout from global config
	config := GetConfig()

	// Create a context with timeout
	execCtx, cancel := context.WithTimeout(ctx, config.CodeExecTimeout)
	defer cancel()

	// Try Python3 first, then Python (Python 3 is the modern standard)
	pythonCmd := "python3"
	if _, err := exec.LookPath("python3"); err != nil {
		pythonCmd = "python"
	}

	// Audit log: log code execution for security tracking
	// Log to stderr so it's visible but doesn't interfere with stdout capture
	fmt.Fprintf(os.Stderr, "[AUDIT] code_exec: executing Python code (%d chars)\n", len(input))

	cmd := exec.CommandContext(execCtx, pythonCmd, "-c", input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += "STDERR: " + stderr.String()
	}

	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("execution timed out (%v limit)", config.CodeExecTimeout)
		}
		if output == "" {
			return "", fmt.Errorf("execution failed: %w", err)
		}
		// Return output even if there was an error (might be useful)
		return output, nil
	}

	if output == "" {
		return "(no output)", nil
	}

	return strings.TrimSpace(output), nil
}

// ============ Web Fetch Tool ============

type WebFetchTool struct {
	client *http.Client
}

func (t *WebFetchTool) Name() string {
	return "web_fetch"
}

func (t *WebFetchTool) Description() string {
	return "Fetch content from a URL or search the web. Input: URL or search query. For search, prefix with 'search:'. Returns text content (HTML stripped)."
}

func (t *WebFetchTool) Execute(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty input")
	}

	if t.client == nil {
		config := GetConfig()
		t.client = &http.Client{Timeout: config.WebFetchTimeout}
	}

	// Check if it's a search query
	if strings.HasPrefix(strings.ToLower(input), "search:") {
		query := strings.TrimSpace(input[7:])
		return t.search(ctx, query)
	}

	// Otherwise treat as URL
	return t.fetch(ctx, input)
}

func (t *WebFetchTool) fetch(ctx context.Context, urlStr string) (string, error) {
	// Validate URL
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "https://" + urlStr
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", parsedURL.String(), nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ReasoningBot/1.0)")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Read limited content
	body, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024)) // 100KB limit
	if err != nil {
		return "", err
	}

	// Strip HTML tags for cleaner output
	content := stripHTML(string(body))

	// Truncate if too long
	if len(content) > 5000 {
		content = content[:5000] + "\n...(truncated)"
	}

	return content, nil
}

func (t *WebFetchTool) search(ctx context.Context, query string) (string, error) {
	// Use DuckDuckGo HTML search (no API key needed)
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ReasoningBot/1.0)")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
	if err != nil {
		return "", err
	}

	// Extract search results
	results := extractSearchResults(string(body))
	if results == "" {
		return "No results found", nil
	}

	return results, nil
}

func stripHTML(html string) string {
	// Remove script and style tags with content
	re := regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`)
	html = re.ReplaceAllString(html, "")
	re = regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`)
	html = re.ReplaceAllString(html, "")

	// Remove HTML tags
	re = regexp.MustCompile(`<[^>]+>`)
	html = re.ReplaceAllString(html, " ")

	// Decode common HTML entities
	html = strings.ReplaceAll(html, "&nbsp;", " ")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")

	// Normalize whitespace
	re = regexp.MustCompile(`\s+`)
	html = re.ReplaceAllString(html, " ")

	return strings.TrimSpace(html)
}

func extractSearchResults(html string) string {
	var results []string

	// Extract result links and snippets from DuckDuckGo HTML
	re := regexp.MustCompile(`<a[^>]+class="result__a"[^>]*>([^<]+)</a>`)
	titles := re.FindAllStringSubmatch(html, 10)

	re2 := regexp.MustCompile(`<a[^>]+class="result__snippet"[^>]*>([^<]+)</a>`)
	snippets := re2.FindAllStringSubmatch(html, 10)

	for i, title := range titles {
		if len(title) > 1 {
			result := fmt.Sprintf("%d. %s", i+1, strings.TrimSpace(title[1]))
			if i < len(snippets) && len(snippets[i]) > 1 {
				result += "\n   " + strings.TrimSpace(stripHTML(snippets[i][1]))
			}
			results = append(results, result)
		}
	}

	if len(results) == 0 {
		// Fallback: just strip HTML and return first chunk
		text := stripHTML(html)
		if len(text) > 2000 {
			text = text[:2000]
		}
		return text
	}

	return strings.Join(results, "\n\n")
}

// ============ String Tool ============

type StringTool struct{}

func (t *StringTool) Name() string {
	return "string_ops"
}

func (t *StringTool) Description() string {
	return "String operations. Input format: 'operation:argument'. Operations: length:text, upper:text, lower:text, reverse:text, count:char,text, split:delimiter,text, replace:old,new,text"
}

func (t *StringTool) Execute(ctx context.Context, input string) (string, error) {
	parts := strings.SplitN(input, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid format, use 'operation:argument'")
	}

	op := strings.ToLower(strings.TrimSpace(parts[0]))
	arg := parts[1]

	switch op {
	case "length", "len":
		return fmt.Sprintf("%d", len(arg)), nil

	case "upper":
		return strings.ToUpper(arg), nil

	case "lower":
		return strings.ToLower(arg), nil

	case "reverse":
		runes := []rune(arg)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return string(runes), nil

	case "count":
		subparts := strings.SplitN(arg, ",", 2)
		if len(subparts) != 2 {
			return "", fmt.Errorf("count requires: char,text")
		}
		return fmt.Sprintf("%d", strings.Count(subparts[1], subparts[0])), nil

	case "split":
		subparts := strings.SplitN(arg, ",", 2)
		if len(subparts) != 2 {
			return "", fmt.Errorf("split requires: delimiter,text")
		}
		if subparts[0] == "" {
			return "", fmt.Errorf("split delimiter cannot be empty (use 'chars' operation for character-by-character split)")
		}
		result := strings.Split(subparts[1], subparts[0])
		return fmt.Sprintf("%v", result), nil

	case "replace":
		subparts := strings.SplitN(arg, ",", 3)
		if len(subparts) != 3 {
			return "", fmt.Errorf("replace requires: old,new,text")
		}
		return strings.ReplaceAll(subparts[2], subparts[0], subparts[1]), nil

	default:
		return "", fmt.Errorf("unknown operation: %s", op)
	}
}

// ============ Go Expression Evaluator (safer alternative) ============

// SafeEval evaluates simple Go expressions for verification
func SafeEval(expr string) (string, error) {
	// This is a very restricted evaluator for simple expressions
	node, err := parser.ParseExpr(expr)
	if err != nil {
		return "", fmt.Errorf("parse error: %w", err)
	}

	result, err := evalNode(node)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%v", result), nil
}

func evalNode(node ast.Expr) (interface{}, error) {
	switch n := node.(type) {
	case *ast.BasicLit:
		switch n.Kind {
		case token.INT:
			return strconv.ParseInt(n.Value, 10, 64)
		case token.FLOAT:
			return strconv.ParseFloat(n.Value, 64)
		case token.STRING:
			return strconv.Unquote(n.Value)
		}
	case *ast.BinaryExpr:
		left, err := evalNode(n.X)
		if err != nil {
			return nil, err
		}
		right, err := evalNode(n.Y)
		if err != nil {
			return nil, err
		}
		return evalBinary(left, right, n.Op)
	case *ast.ParenExpr:
		return evalNode(n.X)
	case *ast.UnaryExpr:
		val, err := evalNode(n.X)
		if err != nil {
			return nil, err
		}
		if n.Op == token.SUB {
			switch v := val.(type) {
			case int64:
				return -v, nil
			case float64:
				return -v, nil
			}
		}
	}
	return nil, fmt.Errorf("unsupported expression type")
}

func evalBinary(left, right interface{}, op token.Token) (interface{}, error) {
	// Convert to float64 for consistency
	lf := toFloat64(left)
	rf := toFloat64(right)

	switch op {
	case token.ADD:
		return lf + rf, nil
	case token.SUB:
		return lf - rf, nil
	case token.MUL:
		return lf * rf, nil
	case token.QUO:
		if rf == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return lf / rf, nil
	case token.REM:
		if rf == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return math.Mod(lf, rf), nil
	}
	return nil, fmt.Errorf("unsupported operator: %s", op)
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case int64:
		return float64(val)
	case float64:
		return val
	case int:
		return float64(val)
	default:
		return 0
	}
}

// validatePythonCode checks for dangerous patterns in Python code before execution
// This function uses a multi-layered approach:
// 1. Fast string-based pattern matching for obvious threats
// 2. AST-based analysis for deeper structural validation
func validatePythonCode(ctx context.Context, code string) error {
	// Normalize code for checking
	lowerCode := strings.ToLower(code)

	// Blocklist of dangerous patterns
	dangerousPatterns := []struct {
		pattern string
		reason  string
	}{
		{"import os", "operating system access"},
		{"import subprocess", "process execution"},
		{"import sys", "system module access"},
		{"import shutil", "file operations"},
		{"import pathlib", "file system access"},
		{"import socket", "network access"},
		{"import urllib", "network requests"},
		{"import requests", "network requests"},
		{"import http", "network requests"},
		{"import ftplib", "network access"},
		{"import multiprocessing", "process execution"},
		{"import threading", "concurrency that may bypass timeouts"},
		{"import signal", "signal handling that may affect process control"},
		{"import fcntl", "file descriptor manipulation"},
		{"import resource", "system resource manipulation"},
		{"import pty", "pseudo-terminal operations"},
		{"import tty", "terminal control"},
		{"from os", "operating system access"},
		{"from subprocess", "process execution"},
		{"from sys", "system module access"},
		{"from shutil", "file operations"},
		{"from pathlib", "file system access"},
		{"from socket", "network access"},
		{"from urllib", "network requests"},
		{"from requests", "network requests"},
		{"from http", "network requests"},
		{"from multiprocessing", "process execution"},
		{"from threading", "concurrency"},
		{"from signal", "signal handling"},
		{"open(", "file operations"},
		{"__import__", "dynamic imports"},
		{"eval(", "code execution"},
		{"exec(", "code execution"},
		{"compile(", "code compilation"},
		{"globals()", "access to globals"},
		{"locals()", "access to locals"},
		{"vars(", "access to variables"},
		{"getattr(", "attribute access"},
		{"setattr(", "attribute modification"},
		{"delattr(", "attribute deletion"},
		{"__class__", "class manipulation"},
		{"__base__", "class manipulation"},
		{"__subclasses__", "class manipulation"},
		{"__mro__", "method resolution order"},
		{"__code__", "code object access"},
		{"__closure__", "closure access"},
		{"__globals__", "global access"},
		{"__dict__", "object attribute manipulation"},
		{"__getattribute__", "attribute access interception"},
		{"__setattribute__", "attribute modification interception"},
		{"__enter__", "context manager entry"},
		{"__exit__", "context manager exit"},
	}

	// Check each dangerous pattern
	for _, dp := range dangerousPatterns {
		if strings.Contains(lowerCode, dp.pattern) {
			return fmt.Errorf("code contains blocked pattern '%s': %s", dp.pattern, dp.reason)
		}
	}

	// Check for character escapes that might bypass filters
	if strings.Contains(code, "\\x") || strings.Contains(code, "\\u") || strings.Contains(code, "\\U") {
		return fmt.Errorf("code contains escape sequences that may bypass security filters")
	}

	// Check for base64 or encoded content that might be malicious
	if strings.Contains(lowerCode, "base64") || strings.Contains(lowerCode, "b64decode") {
		return fmt.Errorf("code contains base64 operations which may indicate malicious intent")
	}

	// Check for pickle or marshal (code serialization)
	if strings.Contains(lowerCode, "pickle") || strings.Contains(lowerCode, "marshal") || strings.Contains(lowerCode, "yaml") || strings.Contains(lowerCode, "shelve") {
		return fmt.Errorf("code contains serialization operations which can execute arbitrary code")
	}

	// Check for type and object manipulation
	if strings.Contains(lowerCode, "type(") || strings.Contains(lowerCode, "object(") {
		return fmt.Errorf("code contains type/object manipulation which may bypass security")
	}

	// Check for super() which can be used to access parent class methods
	if strings.Contains(lowerCode, "super(") {
		return fmt.Errorf("code contains super() which may be used for class manipulation")
	}

	// Additional bypass technique checks
	// Check for list comprehension or generator expressions with dangerous patterns
	if strings.Contains(lowerCode, "[__import__") || strings.Contains(lowerCode, "(__import__") {
		return fmt.Errorf("code contains obfuscated import patterns")
	}

	// Check for lambda with potentially dangerous operations
	if strings.Contains(lowerCode, "lambda __import__") || strings.Contains(lowerCode, "lambda exec") || strings.Contains(lowerCode, "lambda eval") {
		return fmt.Errorf("code contains lambda with dangerous functions")
	}

	// Check for string concatenation that might form dangerous function names
	// This is a basic heuristic; more sophisticated detection would be needed
	if strings.Contains(code, "+") && (strings.Contains(lowerCode, "exec") || strings.Contains(lowerCode, "eval")) {
		// Look for patterns like "ex" + "ec" that might bypass string matching
		if strings.Count(code, "\"+") > 2 || strings.Count(code, "'+") > 2 {
			return fmt.Errorf("code contains suspicious string concatenation patterns")
		}
	}

	// Check for bytearray or bytes operations that might encode malicious code
	if strings.Contains(lowerCode, "bytearray") || strings.Contains(lowerCode, "bytes.fromhex") || strings.Contains(lowerCode, "bytes.decode") {
		return fmt.Errorf("code contains byte operations that may encode malicious code")
	}

	// Check for format strings that might be used for obfuscation
	if strings.Contains(lowerCode, ".format(") && strings.Contains(code, "{") {
		// This is a heuristic - format strings can be legitimate
		// Check for suspicious format patterns
		if strings.Count(code, "{") > 10 {
			return fmt.Errorf("code contains excessive format string usage which may indicate obfuscation")
		}
	}

	// AST-based validation using Python's ast module
	// This provides deeper analysis of code structure
	config := GetConfig()

	// Try Python3 first, then Python (Python 3 is the modern standard)
	pythonCmd := "python3"
	if _, err := exec.LookPath("python3"); err != nil {
		pythonCmd = "python"
	}

	// Create a Python script to analyze the AST
	astAnalysisScript := `
import ast
import sys
import json

def analyze_code(code):
    try:
        tree = ast.parse(code)
        analyzer = ASTSecurityAnalyzer()
        analyzer.visit(tree)
        if analyzer.violations:
            return json.dumps({"status": "error", "violations": analyzer.violations})
        return json.dumps({"status": "ok"})
    except SyntaxError as e:
        return json.dumps({"status": "syntax_error", "message": str(e)})
    except Exception as e:
        return json.dumps({"status": "error", "message": str(e)})

class ASTSecurityAnalyzer(ast.NodeVisitor):
    def __init__(self):
        self.violations = []

    def visit_Import(self, node):
        for alias in node.names:
            module_name = alias.name.split('.')[0]
            if module_name in ['os', 'subprocess', 'sys', 'shutil', 'pathlib', 'socket',
                              'urllib', 'requests', 'http', 'ftplib', 'multiprocessing',
                              'threading', 'signal', 'fcntl', 'resource', 'pty', 'tty',
                              'ctypes', 'mmap', 'tempfile', 'io', 'importlib']:
                self.violations.append(f"import of blocked module: {module_name}")
        self.generic_visit(node)

    def visit_ImportFrom(self, node):
        if node.module:
            module_name = node.module.split('.')[0]
            if module_name in ['os', 'subprocess', 'sys', 'shutil', 'pathlib', 'socket',
                              'urllib', 'requests', 'http', 'ftplib', 'multiprocessing',
                              'threading', 'signal', 'fcntl', 'resource', 'pty', 'tty',
                              'ctypes', 'mmap', 'tempfile', 'io', 'importlib']:
                self.violations.append(f"from import of blocked module: {module_name}")
        self.generic_visit(node)

    def visit_Call(self, node):
        # Check for dangerous function calls
        if isinstance(node.func, ast.Name):
            func_name = node.func.id
            if func_name in ['eval', 'exec', 'compile', '__import__', 'open', 'globals',
                            'locals', 'vars', 'getattr', 'setattr', 'delattr', 'help',
                            'dir', 'type', 'super', 'hasattr', 'isinstance', 'issubclass']:
                self.violations.append(f"call to blocked function: {func_name}")
        elif isinstance(node.func, ast.Attribute):
            # Check for dangerous attribute access
            if node.func.attr in ['__class__', '__base__', '__bases__', '__subclasses__',
                                  '__mro__', '__code__', '__closure__', '__globals__', '__dict__',
                                  '__getattribute__', '__setattr__', '__delattr__']:
                self.violations.append(f"access to blocked attribute: {node.func.attr}")
        self.generic_visit(node)

    def visit_Attribute(self, node):
        # Check for access to dangerous dunder methods
        if node.attr in ['__class__', '__base__', '__bases__', '__subclasses__',
                        '__mro__', '__code__', '__closure__', '__globals__', '__dict__',
                        '__getattribute__', '__setattr__', '__delattr__', '__import__']:
            self.violations.append(f"access to blocked attribute: {node.attr}")
        self.generic_visit(node)

    def visit_FunctionDef(self, node):
        # Check for decorators that might bypass security
        for decorator in node.decorator_list:
            if isinstance(decorator, ast.Name) and decorator.id in ['staticmethod', 'classmethod', 'property']:
                # These are generally safe
                pass
        self.generic_visit(node)

    def visit_ClassDef(self, node):
        # Check for metaclass usage which can bypass restrictions
        for keyword in node.keywords:
            if keyword.arg == 'metaclass':
                self.violations.append("metaclass usage detected")
        self.generic_visit(node)

    def visit_Lambda(self, node):
        # Check for dangerous operations in lambda
        self.generic_visit(node)

    def visit_ListComp(self, node):
        # Check for dangerous operations in list comprehensions
        self.generic_visit(node)

    def visit_DictComp(self, node):
        # Check for dangerous operations in dict comprehensions
        self.generic_visit(node)

    def visit_SetComp(self, node):
        # Check for dangerous operations in set comprehensions
        self.generic_visit(node)

    def visit_GeneratorExp(self, node):
        # Check for dangerous operations in generator expressions
        self.generic_visit(node)

    def visit_Starred(self, node):
        # Check for starred expressions which might be used for obfuscation
        self.generic_visit(node)

    def visit_IfExp(self, node):
        # Check for conditional expressions
        self.generic_visit(node)

    def visit_JoinedStr(self, node):
        # Check for f-strings which might contain dangerous code
        self.generic_visit(node)

    def visit_FormattedValue(self, node):
        # Check for formatted values in f-strings
        self.generic_visit(node)

    def visit_Try(self, node):
        # Check for try-except blocks that might hide malicious behavior
        self.generic_visit(node)

    def visit_While(self, node):
        # Check for while loops which might be used for infinite loops
        self.generic_visit(node)

    def visit_For(self, node):
        # Check for for loops
        self.generic_visit(node)

    def visit_With(self, node):
        # Check for context managers
        for item in node.items:
            if isinstance(item.context_expr, ast.Call):
                if isinstance(item.context_expr.func, ast.Name):
                    if item.context_expr.func.id == 'open':
                        self.violations.append("file operation using 'with open()'")
        self.generic_visit(node)

if __name__ == "__main__":
    code = sys.stdin.read()
    print(analyze_code(code))
`

	// Create context with timeout for AST analysis
	astCtx, astCancel := context.WithTimeout(ctx, config.CodeExecTimeout)
	defer astCancel()

	// Run Python AST analysis
	cmd := exec.CommandContext(astCtx, pythonCmd, "-c", astAnalysisScript)

	// Setup stdin for writing code to the process
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil
	}

	// Write to stdin in a goroutine to avoid deadlock
	go func() {
		defer stdin.Close()
		stdin.Write([]byte(code))
	}()

	// Capture output to avoid deadlock with context cancellation
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Start command (CombinedOutput would block on stdin write)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] AST validation unavailable, using pattern matching only: %v\n", err)
		return nil
	}

	// Wait for command to complete (context already has timeout)
	err = cmd.Wait()

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	if err != nil {
		// If AST analysis fails (e.g., Python not available), log a warning but don't block
		// This maintains backward compatibility while adding security when possible
		fmt.Fprintf(os.Stderr, "[WARN] AST validation unavailable, using pattern matching only: %v\n", err)
		return nil
	}

	// Parse the JSON result from AST analysis
	var astResult struct {
		Status     string   `json:"status"`
		Violations []string `json:"violations"`
		Message    string   `json:"message"`
	}

	if err := json.Unmarshal([]byte(output), &astResult); err != nil {
		// If we can't parse the result, log but don't block
		fmt.Fprintf(os.Stderr, "[WARN] Could not parse AST validation result: %v\n", err)
		return nil
	}

	if astResult.Status == "error" && len(astResult.Violations) > 0 {
		return fmt.Errorf("AST validation failed: %s", strings.Join(astResult.Violations, "; "))
	}

	if astResult.Status == "syntax_error" {
		return fmt.Errorf("syntax error in code: %s", astResult.Message)
	}

	return nil
}
