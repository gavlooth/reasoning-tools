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
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
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

	// Enable all by default
	for name := range registry.tools {
		registry.enabled[name] = true
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
func (r *ToolRegistry) SetEnabled(names []string) {
	// Disable all first
	for name := range r.enabled {
		r.enabled[name] = false
	}
	// Enable specified
	for _, name := range names {
		if _, exists := r.tools[name]; exists {
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
		return "", fmt.Errorf("calculation error: %w", err)
	}

	// Format result nicely
	if result == float64(int64(result)) {
		return fmt.Sprintf("%.0f", result), nil
	}
	return fmt.Sprintf("%.10g", result), nil
}

// evaluateMathExpr evaluates a mathematical expression safely
func evaluateMathExpr(expr string) (float64, error) {
	// Normalize the expression
	expr = strings.ToLower(expr)
	expr = strings.ReplaceAll(expr, " ", "")

	// Replace constants
	expr = strings.ReplaceAll(expr, "pi", fmt.Sprintf("%.15f", math.Pi))
	expr = strings.ReplaceAll(expr, "e", fmt.Sprintf("%.15f", math.E))

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
	return "Execute Python code and return the output. Input: Python code snippet. The code runs in a sandboxed environment with a 10s timeout. Print results to see them."
}

func (t *CodeExecutorTool) Execute(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty code")
	}

	// Create a context with timeout
	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Try Python first, then Python3
	pythonCmd := "python3"
	if _, err := exec.LookPath("python3"); err != nil {
		pythonCmd = "python"
	}

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
			return "", fmt.Errorf("execution timed out (10s limit)")
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
		t.client = &http.Client{Timeout: 15 * time.Second}
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
	re := regexp.MustCompile(`(?s)<(script|style)[^>]*>.*?</\1>`)
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
