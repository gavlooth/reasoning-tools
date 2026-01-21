package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"reasoning-tools/utils"
)

// DialecticalReasoner implements Debate + Chain of Verification
type DialecticalReasoner struct {
	provider      Provider
	config        DialecticConfig
	tools         *ToolRegistry
	toolCalls     int
	toolCallsMu   sync.Mutex
	onProgress    func(ProgressUpdate)
	onToken       func(token string)
	enableStreams bool
}

// DialecticConfig configures the dialectical reasoning process
type DialecticConfig struct {
	MaxRounds        int      // Maximum debate rounds (default: 5)
	VerifyThreshold  float64  // Minimum verification score to accept (default: 0.7)
	ConfidenceTarget float64  // Stop when synthesis reaches this confidence (default: 0.85)
	Temperature      float64  // LLM temperature (default: 0.7)
	MaxTokens        int      // Maximum tokens per LLM call (default: 1024)
	FastMode         bool     // Run a single-pass dialectic (default: false)
	ThesisModel      string   // Override model for thesis generation (optional)
	AntithesisModel  string   // Override model for antithesis generation (optional)
	SynthesisModel   string   // Override model for synthesis generation (optional)
	EnableTools      bool     // Whether to use tools during verification (default: false)
	MaxToolCalls     int      // Maximum tool calls total (default: 10)
	EnabledTools     []string // Which tools to enable (empty = all)
}

// DefaultDialecticConfig returns sensible defaults
func DefaultDialecticConfig() DialecticConfig {
	return DialecticConfig{
		MaxRounds:        5,
		VerifyThreshold:  0.7,
		ConfidenceTarget: 0.85,
		Temperature:      0.7,
		MaxTokens:        1024,
		FastMode:         false,
		ThesisModel:      "",
		AntithesisModel:  "",
		SynthesisModel:   "",
		EnableTools:      false,
		MaxToolCalls:     10,
		EnabledTools:     []string{},
	}
}

// DialecticStep represents one round of thesis-antithesis-synthesis
type DialecticStep struct {
	Round      int   `json:"round"`
	Thesis     Claim `json:"thesis"`
	Antithesis Claim `json:"antithesis"`
	Synthesis  Claim `json:"synthesis"`
	Resolved   bool  `json:"resolved"`
}

// Claim represents a reasoned claim with verification
type Claim struct {
	Content      string       `json:"content"`
	Verification Verification `json:"verification"`
}

// VerificationStatus represents the explicit status of verification
type VerificationStatus string

const (
	StatusVerified   VerificationStatus = "verified"   // Successfully verified
	StatusUnverified VerificationStatus = "unverified" // Verification attempted but failed
	StatusSkipped    VerificationStatus = "skipped"    // Verification not attempted
)

// Verification represents the result of verifying a claim
type Verification struct {
	IsValid     bool               `json:"is_valid"`
	Score       float64            `json:"score"`                  // 0-1 confidence
	Status      VerificationStatus `json:"status"`                 // Explicit verification status
	Issues      []string           `json:"issues"`                 // Identified problems
	Strengths   []string           `json:"strengths"`              // What's good about it
	Suggestion  string             `json:"suggestion"`             // How to improve
	ToolResults []ToolResult       `json:"tool_results,omitempty"` // Results from tool-based verification
	ErrorReason string             `json:"error_reason,omitempty"` // Why verification failed (if applicable)
}

// DialecticResult represents the complete reasoning result
type DialecticResult struct {
	Problem        string          `json:"problem"`
	Steps          []DialecticStep `json:"steps"`
	FinalAnswer    string          `json:"final_answer"`
	Confidence     float64         `json:"confidence"`
	TotalRounds    int             `json:"total_rounds"`
	TotalToolCalls int             `json:"total_tool_calls,omitempty"`
	ToolsUsed      map[string]int  `json:"tools_used,omitempty"`
	Success        bool            `json:"success"`
	Provider       string          `json:"provider"`
}

type fastPayload struct {
	Thesis     string  `json:"thesis"`
	Antithesis string  `json:"antithesis"`
	Synthesis  string  `json:"synthesis"`
	Confidence float64 `json:"confidence"`
}

// NewDialecticalReasoner creates a new dialectical reasoner
func NewDialecticalReasoner(provider Provider, config DialecticConfig) *DialecticalReasoner {
	d := &DialecticalReasoner{
		provider: provider,
		config:   config,
	}

	// Initialize tools if enabled
	if config.EnableTools {
		d.tools = NewToolRegistry()
		if len(config.EnabledTools) > 0 {
			d.tools.SetEnabled(config.EnabledTools)
		}
	}

	return d
}

// SetProgressCallback sets a callback for progress updates
func (d *DialecticalReasoner) SetProgressCallback(cb func(ProgressUpdate)) {
	d.onProgress = cb
}

// SetTokenCallback sets a callback for token streaming
func (d *DialecticalReasoner) SetTokenCallback(cb func(token string)) {
	d.onToken = cb
}

// SetEnableStreaming enables or disables LLM streaming
func (d *DialecticalReasoner) SetEnableStreaming(enable bool) {
	d.enableStreams = enable
}

func (d *DialecticalReasoner) emitProgress(update ProgressUpdate) {
	if d.onProgress != nil {
		d.onProgress(update)
	}
}

// Reason performs dialectical reasoning on a problem
func (d *DialecticalReasoner) Reason(ctx context.Context, problem string) (*DialecticResult, error) {
	if d.config.FastMode {
		return d.reasonFast(ctx, problem)
	}

	result := &DialecticResult{
		Problem:   problem,
		Steps:     []DialecticStep{},
		Provider:  d.provider.Name(),
		ToolsUsed: make(map[string]int),
	}
	d.toolCalls = 0

	var currentContext string
	var lastSynthesis string

	for round := 1; round <= d.config.MaxRounds; round++ {
		step := DialecticStep{Round: round}

		// === THESIS: Propose a solution/claim ===
		thesis, err := d.generateThesis(ctx, problem, currentContext, lastSynthesis)
		if err != nil {
			return result, fmt.Errorf("thesis generation failed at round %d: %w", round, err)
		}

		// Verify thesis
		thesisVerification, err := d.verify(ctx, problem, thesis, "thesis")
		if err != nil {
			// Verification failed - mark as invalid to prevent incorrect conclusions
			// Use Score: 0.5 as a neutral/unknown confidence indicator
			thesisVerification = Verification{
				IsValid:     false,
				Score:       0.5,
				Status:      StatusUnverified,
				ErrorReason: fmt.Sprintf("verification error: %v", err),
			}
		}
		step.Thesis = Claim{Content: thesis, Verification: thesisVerification}

		// === ANTITHESIS: Challenge the thesis ===
		antithesis, err := d.generateAntithesis(ctx, problem, thesis, thesisVerification)
		if err != nil {
			return result, fmt.Errorf("antithesis generation failed at round %d: %w", round, err)
		}

		// Verify antithesis
		antithesisVerification, err := d.verify(ctx, problem, antithesis, "antithesis")
		if err != nil {
			// Verification failed - mark as invalid to prevent incorrect conclusions
			antithesisVerification = Verification{
				IsValid:     false,
				Score:       0.5,
				Status:      StatusUnverified,
				ErrorReason: fmt.Sprintf("verification error: %v", err),
			}
		}
		step.Antithesis = Claim{Content: antithesis, Verification: antithesisVerification}

		// === SYNTHESIS: Resolve the debate ===
		synthesis, err := d.generateSynthesis(ctx, problem, step.Thesis, step.Antithesis)
		if err != nil {
			return result, fmt.Errorf("synthesis generation failed at round %d: %w", round, err)
		}

		// Verify synthesis
		synthesisVerification, err := d.verify(ctx, problem, synthesis, "synthesis")
		if err != nil {
			// Verification failed - mark as invalid to prevent incorrect conclusions
			synthesisVerification = Verification{
				IsValid:     false,
				Score:       0.5,
				Status:      StatusUnverified,
				ErrorReason: fmt.Sprintf("verification error: %v", err),
			}
		}
		step.Synthesis = Claim{Content: synthesis, Verification: synthesisVerification}

		// Check if we've reached resolution
		step.Resolved = synthesisVerification.Score >= d.config.ConfidenceTarget &&
			synthesisVerification.IsValid &&
			len(synthesisVerification.Issues) == 0

		result.Steps = append(result.Steps, step)

		// Update context for next round
		currentContext = d.buildContext(result.Steps)
		lastSynthesis = synthesis

		// Check termination conditions
		if step.Resolved {
			result.FinalAnswer = synthesis
			result.Confidence = synthesisVerification.Score
			result.Success = true
			result.TotalRounds = round
			d.toolCallsMu.Lock()
			result.TotalToolCalls = d.toolCalls
			d.toolCallsMu.Unlock()
			d.countToolsUsed(result)
			return result, nil
		}

		// If synthesis is good enough but not perfect, continue refining
		if synthesisVerification.Score >= d.config.VerifyThreshold {
			result.FinalAnswer = synthesis
			result.Confidence = synthesisVerification.Score
		}
	}

	// Max rounds reached
	result.TotalRounds = d.config.MaxRounds
	if result.FinalAnswer == "" && len(result.Steps) > 0 {
		lastStep := result.Steps[len(result.Steps)-1]
		result.FinalAnswer = lastStep.Synthesis.Content
		result.Confidence = lastStep.Synthesis.Verification.Score
	}
	result.Success = result.Confidence >= d.config.VerifyThreshold
	d.toolCallsMu.Lock()
	result.TotalToolCalls = d.toolCalls
	d.toolCallsMu.Unlock()
	d.countToolsUsed(result)

	return result, nil
}

func (d *DialecticalReasoner) reasonFast(ctx context.Context, problem string) (*DialecticResult, error) {
	result := &DialecticResult{
		Problem:   problem,
		Steps:     []DialecticStep{},
		Provider:  d.provider.Name(),
		ToolsUsed: make(map[string]int),
	}

	d.emitProgress(ProgressUpdate{
		Type:    "thought",
		Message: "Generating fast dialectic response...",
	})

	prompt := fmt.Sprintf(`You are a careful dialectical reasoner.

Problem: %s

Provide a concise thesis, antithesis, and synthesis. Respond with ONLY a JSON object:
{
  "thesis": "string",
  "antithesis": "string",
  "synthesis": "string",
  "confidence": 0.0
}

Confidence is your 0-1 confidence in the synthesis. Keep each field concise.

If you cannot produce JSON, respond in plain text using EXACT labels:
THESIS: ...
ANTITHESIS: ...
SYNTHESIS: ...
CONFIDENCE: 0.0`, problem)

	messages := []ChatMessage{
		{Role: "system", Content: "You produce clean JSON and concise dialectical analysis."},
		{Role: "user", Content: prompt},
	}

	var response string
	var err error
	if sp, ok := d.provider.(StreamingProvider); ok && d.enableStreams && sp.SupportsStreaming() {
		response, err = sp.ChatStream(ctx, messages, ChatOptions{
			Temperature: clampTemperature(d.config.Temperature),
			MaxTokens:   d.config.MaxTokens,
		}, func(token string) {
			if d.onToken != nil {
				d.onToken(token)
			}
		})
	} else {
		response, err = d.provider.Chat(ctx, messages, ChatOptions{
			Temperature: clampTemperature(d.config.Temperature),
			MaxTokens:   d.config.MaxTokens,
		})
	}
	if err != nil {
		return result, err
	}

	jsonStr := utils.ExtractJSON(response)
	var payload fastPayload
	if jsonStr != "" {
		if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
			return result, fmt.Errorf("failed to parse fast dialectic response: %w", err)
		}
	} else {
		var ok bool
		payload, ok = parseFastDialecticText(response)
		if !ok {
			return result, fmt.Errorf("failed to parse fast dialectic response")
		}
	}

	confidence := payload.Confidence
	if confidence < 0 {
		confidence = 0
	} else if confidence > 1 {
		confidence = 1
	}

	step := DialecticStep{
		Round: 1,
		Thesis: Claim{
			Content: payload.Thesis,
			Verification: Verification{
				IsValid: true,
				Score:   0.5,
				Status:  StatusSkipped,
			},
		},
		Antithesis: Claim{
			Content: payload.Antithesis,
			Verification: Verification{
				IsValid: true,
				Score:   0.5,
				Status:  StatusSkipped,
			},
		},
		Synthesis: Claim{
			Content: payload.Synthesis,
			Verification: Verification{
				IsValid: true,
				Score:   confidence,
				Status:  StatusSkipped,
			},
		},
		Resolved: confidence >= d.config.VerifyThreshold,
	}

	result.Steps = append(result.Steps, step)
	result.TotalRounds = 1
	result.FinalAnswer = payload.Synthesis
	result.Confidence = confidence
	result.Success = confidence >= d.config.VerifyThreshold

	d.emitProgress(ProgressUpdate{
		Type:        "solution",
		Message:     "Fast dialectic response complete",
		IsSolution:  true,
		FinalAnswer: result.FinalAnswer,
	})

	return result, nil
}

func parseFastDialecticText(response string) (fastPayload, bool) {
	response = strings.ReplaceAll(response, "\r\n", "\n")
	response = strings.ReplaceAll(response, "\r", "\n")

	sections := map[string][]string{}
	current := ""

	normalizeLine := func(line string) string {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "-*• \t")
		return strings.TrimSpace(line)
	}

	detectLabel := func(line string) string {
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "thesis"):
			return "thesis"
		case strings.HasPrefix(lower, "antithesis"), strings.HasPrefix(lower, "anti-thesis"):
			return "antithesis"
		case strings.HasPrefix(lower, "synthesis"):
			return "synthesis"
		case strings.HasPrefix(lower, "confidence"):
			return "confidence"
		default:
			return ""
		}
	}

	lines := strings.Split(response, "\n")
	for _, raw := range lines {
		line := normalizeLine(raw)
		if line == "" {
			continue
		}

		if label := detectLabel(line); label != "" {
			remainder := strings.TrimSpace(line[len(label):])
			remainder = strings.TrimLeft(remainder, ":-–— \t")
			current = label
			if remainder != "" {
				sections[label] = append(sections[label], remainder)
			}
			continue
		}

		if current != "" {
			sections[current] = append(sections[current], line)
		}
	}

	clean := func(parts []string) string {
		if len(parts) == 0 {
			return ""
		}
		combined := strings.Join(parts, " ")
		combined = strings.TrimSpace(combined)
		if strings.HasPrefix(combined, "1.") || strings.HasPrefix(combined, "2.") || strings.HasPrefix(combined, "3.") {
			combined = strings.TrimSpace(strings.TrimLeft(combined[2:], " "))
		}
		return strings.TrimSpace(combined)
	}

	thesis := clean(sections["thesis"])
	antithesis := clean(sections["antithesis"])
	synthesis := clean(sections["synthesis"])

	confidence := 0.0
	confText := clean(sections["confidence"])
	if confText == "" {
		confText = response
	}
	confRe := regexp.MustCompile(`(?is)(?:^|\n)\s*confidence\s*[:\-]?\s*([0-9]*\.?[0-9]+)`)
	if match := confRe.FindStringSubmatch(confText); len(match) > 1 {
		if val, err := strconv.ParseFloat(match[1], 64); err == nil {
			confidence = val
		}
	}

	if thesis == "" || antithesis == "" || synthesis == "" {
		var fallback []string
		for _, raw := range lines {
			line := normalizeLine(raw)
			if line == "" {
				continue
			}
			fallback = append(fallback, line)
		}
		if len(fallback) >= 3 {
			if thesis == "" {
				thesis = fallback[0]
			}
			if antithesis == "" {
				antithesis = fallback[1]
			}
			if synthesis == "" {
				synthesis = fallback[2]
			}
		}
	}

	if thesis == "" || antithesis == "" || synthesis == "" {
		return fastPayload{}, false
	}

	return fastPayload{
		Thesis:     thesis,
		Antithesis: antithesis,
		Synthesis:  synthesis,
		Confidence: confidence,
	}, true
}

// countToolsUsed counts which tools were used
func (d *DialecticalReasoner) countToolsUsed(result *DialecticResult) {
	for _, step := range result.Steps {
		for _, tr := range step.Thesis.Verification.ToolResults {
			result.ToolsUsed[tr.Tool]++
		}
		for _, tr := range step.Antithesis.Verification.ToolResults {
			result.ToolsUsed[tr.Tool]++
		}
		for _, tr := range step.Synthesis.Verification.ToolResults {
			result.ToolsUsed[tr.Tool]++
		}
	}
}

// generateThesis proposes an initial solution or builds on previous synthesis
func (d *DialecticalReasoner) generateThesis(ctx context.Context, problem, context, lastSynthesis string) (string, error) {
	var prompt string
	if lastSynthesis == "" {
		prompt = fmt.Sprintf(`You are a thoughtful reasoner. Propose a clear, well-reasoned solution to this problem.

Problem: %s

Provide your thesis - a clear claim or solution approach. Be specific and justify your reasoning.
Respond with just your thesis, no preamble.`, problem)
	} else {
		prompt = fmt.Sprintf(`You are a thoughtful reasoner building on previous analysis.

Problem: %s

Previous context:
%s

Last synthesis to build upon:
%s

Propose a refined thesis that advances the reasoning. Address any remaining uncertainties.
Respond with just your thesis, no preamble.`, problem, context, lastSynthesis)
	}

	messages := []ChatMessage{
		{Role: "system", Content: "You are a precise, analytical thinker. Propose clear, defensible claims."},
		{Role: "user", Content: prompt},
	}

	// Check if provider supports streaming
	if sp, ok := d.provider.(StreamingProvider); ok && d.enableStreams && sp.SupportsStreaming() {
		return sp.ChatStream(ctx, messages, ChatOptions{
			Temperature: clampTemperature(d.config.Temperature),
			MaxTokens:   d.config.MaxTokens,
			Model:       d.config.ThesisModel,
		}, func(token string) {
			if d.onToken != nil {
				d.onToken(token)
			}
		})
	}

	return d.provider.Chat(ctx, messages, ChatOptions{
		Temperature: clampTemperature(d.config.Temperature),
		MaxTokens:   d.config.MaxTokens,
		Model:       d.config.ThesisModel,
	})
}

// generateAntithesis challenges the thesis
func (d *DialecticalReasoner) generateAntithesis(ctx context.Context, problem, thesis string, thesisVerification Verification) (string, error) {
	issuesContext := ""
	if len(thesisVerification.Issues) > 0 {
		issuesContext = fmt.Sprintf("\n\nKnown issues with the thesis:\n- %s", strings.Join(thesisVerification.Issues, "\n- "))
	}

	prompt := fmt.Sprintf(`You are a critical challenger. Your job is to find flaws and argue against the thesis.

Problem: %s

Thesis to challenge:
%s
%s

Generate a strong antithesis that:
1. Identifies weaknesses, assumptions, or gaps in the thesis
2. Proposes alternative perspectives or counterarguments
3. Challenges any unjustified claims
4. Points out edge cases or failure modes

Be rigorous but fair. Don't strawman - engage with the strongest version of the thesis.
Respond with just your antithesis, no preamble.`, problem, thesis, issuesContext)

	messages := []ChatMessage{
		{Role: "system", Content: "You are a devil's advocate. Challenge ideas rigorously but fairly. Find real flaws, not nitpicks."},
		{Role: "user", Content: prompt},
	}

	// Check if provider supports streaming
	if sp, ok := d.provider.(StreamingProvider); ok && d.enableStreams && sp.SupportsStreaming() {
		return sp.ChatStream(ctx, messages, ChatOptions{
			Temperature: clampTemperature(d.config.Temperature + 0.1), // Slightly higher for creativity
			MaxTokens:   d.config.MaxTokens,
			Model:       d.config.AntithesisModel,
		}, func(token string) {
			if d.onToken != nil {
				d.onToken(token)
			}
		})
	}

	return d.provider.Chat(ctx, messages, ChatOptions{
		Temperature: clampTemperature(d.config.Temperature + 0.1), // Slightly higher for creativity
		MaxTokens:   d.config.MaxTokens,
		Model:       d.config.AntithesisModel,
	})
}

// generateSynthesis resolves thesis and antithesis
func (d *DialecticalReasoner) generateSynthesis(ctx context.Context, problem string, thesis, antithesis Claim) (string, error) {
	prompt := fmt.Sprintf(`You are a wise synthesizer. Resolve the debate between thesis and antithesis.

Problem: %s

THESIS (confidence: %.2f):
%s

Thesis strengths: %s
Thesis issues: %s

ANTITHESIS (confidence: %.2f):
%s

Antithesis strengths: %s
Antithesis issues: %s

Generate a synthesis that:
1. Acknowledges valid points from both sides
2. Resolves contradictions through deeper understanding
3. Integrates the strongest elements of each position
4. Addresses the issues raised by both verifications
5. Provides a more complete answer than either alone

If one side is clearly correct, explain why while acknowledging what the other side got right.
Respond with just your synthesis, no preamble.`,
		problem,
		thesis.Verification.Score, thesis.Content,
		strings.Join(thesis.Verification.Strengths, "; "),
		strings.Join(thesis.Verification.Issues, "; "),
		antithesis.Verification.Score, antithesis.Content,
		strings.Join(antithesis.Verification.Strengths, "; "),
		strings.Join(antithesis.Verification.Issues, "; "))

	messages := []ChatMessage{
		{Role: "system", Content: "You are a balanced synthesizer. Find truth by integrating opposing views. Produce clear, actionable conclusions."},
		{Role: "user", Content: prompt},
	}

	// Check if provider supports streaming
	if sp, ok := d.provider.(StreamingProvider); ok && d.enableStreams && sp.SupportsStreaming() {
		return sp.ChatStream(ctx, messages, ChatOptions{
			Temperature: clampTemperature(d.config.Temperature - 0.1), // Slightly lower for precision
			MaxTokens:   d.config.MaxTokens,
			Model:       d.config.SynthesisModel,
		}, func(token string) {
			if d.onToken != nil {
				d.onToken(token)
			}
		})
	}

	return d.provider.Chat(ctx, messages, ChatOptions{
		Temperature: clampTemperature(d.config.Temperature - 0.1), // Slightly lower for precision
		MaxTokens:   d.config.MaxTokens,
		Model:       d.config.SynthesisModel,
	})
}

// verify checks if a claim is valid and identifies issues
func (d *DialecticalReasoner) verify(ctx context.Context, problem, claim, claimType string) (Verification, error) {
	var toolResults []ToolResult

	// If tools enabled, first ask what to verify and use tools
	if d.config.EnableTools && d.tools != nil && d.toolCalls < d.config.MaxToolCalls {
		toolResults = d.gatherToolEvidence(ctx, problem, claim, claimType)
	}

	// Build verification prompt (with or without tool evidence)
	toolContext := ""
	if len(toolResults) > 0 {
		var evidence []string
		for _, tr := range toolResults {
			if tr.Success {
				evidence = append(evidence, fmt.Sprintf("- [%s] %s", tr.Tool, utils.TruncateStr(tr.Output, 200)))
			}
		}
		if len(evidence) > 0 {
			toolContext = fmt.Sprintf("\n\nTool-gathered evidence:\n%s\n", strings.Join(evidence, "\n"))
		}
	}

	prompt := fmt.Sprintf(`You are a rigorous verifier. Evaluate this %s for the given problem.

Problem: %s

%s to verify:
%s%s

Analyze this claim and respond with ONLY a JSON object:
{
  "is_valid": <true if the reasoning is sound, false if fundamentally flawed>,
  "score": <0.0 to 1.0 confidence score>,
  "issues": ["list of specific problems, gaps, or weaknesses"],
  "strengths": ["list of what's good about this claim"],
  "suggestion": "how to improve or address the issues"
}

Be thorough but fair. Look for:
- Logical fallacies or gaps
- Unsupported assumptions
- Missing considerations
- Factual errors
- Incomplete reasoning`, claimType, problem, cases.Title(language.English).String(claimType), claim, toolContext)

	messages := []ChatMessage{
		{Role: "system", Content: "You are a careful verifier. Identify both strengths and weaknesses objectively."},
		{Role: "user", Content: prompt},
	}

	var response string
	var err error

	// Check if provider supports streaming
	if sp, ok := d.provider.(StreamingProvider); ok && d.enableStreams && sp.SupportsStreaming() {
		response, err = sp.ChatStream(ctx, messages, ChatOptions{
			Temperature: clampTemperature(0.3), // Low temp for consistent verification
			MaxTokens:   d.config.MaxTokens,
		}, func(token string) {
			if d.onToken != nil {
				d.onToken(token)
			}
		})
	} else {
		response, err = d.provider.Chat(ctx, messages, ChatOptions{
			Temperature: clampTemperature(0.3), // Low temp for consistent verification
			MaxTokens:   d.config.MaxTokens,
		})
	}
	if err != nil {
		return Verification{}, err
	}

	v, err := parseVerification(response)
	v.ToolResults = toolResults
	return v, err
}

// gatherToolEvidence uses tools to gather evidence for verification
func (d *DialecticalReasoner) gatherToolEvidence(ctx context.Context, problem, claim, claimType string) []ToolResult {
	var results []ToolResult

	// Ask LLM what to verify with tools
	toolsPrompt := d.tools.GetToolsPrompt()
	prompt := fmt.Sprintf(`You need to verify this %s. What tool calls would help fact-check or verify it?

Problem: %s
Claim: %s

%s

Respond with a JSON array of tool calls to make (max 2):
[
  {"tool": "calculator", "input": "expression to verify"},
  {"tool": "web_fetch", "input": "search query for facts"}
]

Only suggest tools if they would genuinely help verify the claim. Respond with [] if no tools needed.`, claimType, problem, claim, toolsPrompt)

	messages := []ChatMessage{
		{Role: "user", Content: prompt},
	}

	var response string
	var err error

	// Check if provider supports streaming
	if sp, ok := d.provider.(StreamingProvider); ok && d.enableStreams && sp.SupportsStreaming() {
		response, err = sp.ChatStream(ctx, messages, ChatOptions{
			Temperature: clampTemperature(0.3),
			MaxTokens:   d.config.MaxTokens,
		}, func(token string) {
			if d.onToken != nil {
				d.onToken(token)
			}
		})
	} else {
		response, err = d.provider.Chat(ctx, messages, ChatOptions{
			Temperature: clampTemperature(0.3),
			MaxTokens:   d.config.MaxTokens,
		})
	}
	if err != nil {
		return results
	}

	// Parse tool calls
	var toolCalls []struct {
		Tool  string `json:"tool"`
		Input string `json:"input"`
	}

	jsonStr := utils.ExtractJSONArray(response)
	if jsonStr == "" {
		return results
	}

	if err := json.Unmarshal([]byte(jsonStr), &toolCalls); err != nil {
		return results
	}

	// Execute up to 2 tool calls
	for i, tc := range toolCalls {
		// Check tool call limit with mutex protection
		d.toolCallsMu.Lock()
		withinLimit := i < 2 && d.toolCalls < d.config.MaxToolCalls
		if withinLimit {
			d.toolCalls++
		}
		d.toolCallsMu.Unlock()

		if !withinLimit {
			break
		}

		result := d.tools.Execute(ctx, tc.Tool, tc.Input)
		results = append(results, result)

		d.emitProgress(ProgressUpdate{
			Type:       "tool",
			ToolName:   tc.Tool,
			ToolInput:  tc.Input,
			ToolOutput: utils.TruncateStr(result.Output, 100),
			Message:    fmt.Sprintf("Verification tool: %s", tc.Tool),
		})
	}

	return results
}

// buildContext creates a summary of previous rounds
func (d *DialecticalReasoner) buildContext(steps []DialecticStep) string {
	if len(steps) == 0 {
		return ""
	}

	var parts []string
	for _, step := range steps {
		parts = append(parts, fmt.Sprintf("Round %d:\n- Thesis: %s\n- Antithesis: %s\n- Synthesis: %s",
			step.Round,
			utils.TruncateStr(step.Thesis.Content, 200),
			utils.TruncateStr(step.Antithesis.Content, 200),
			utils.TruncateStr(step.Synthesis.Content, 200)))
	}
	return strings.Join(parts, "\n\n")
}

// parseVerification extracts verification from LLM response
func parseVerification(response string) (Verification, error) {
	jsonStr := utils.ExtractJSON(response)
	if jsonStr != "" {
		// Successfully extracted JSON
		var v Verification
		if err := json.Unmarshal([]byte(jsonStr), &v); err != nil {
			return Verification{IsValid: false, Score: 0, Status: StatusUnverified, ErrorReason: fmt.Sprintf("JSON parse error: %v", err)}, err
		}

		// Set status to verified for successful verifications
		if v.Status == "" {
			v.Status = StatusVerified
		}

		// Clamp score
		if v.Score < 0 {
			v.Score = 0
		}
		if v.Score > 1 {
			v.Score = 1
		}

		return v, nil
	}

	// No JSON found - try to parse text response and convert to Verification
	// This handles z.ai (glm-4.7) which often returns plain text instead of JSON
	v := parseTextToVerification(response)

	// Set status since text parser doesn't set it
	if v.Status == "" {
		v.Status = StatusVerified
	}

	return v, nil
}

// parseTextToVerification attempts to convert a plain text response into a Verification object
// This is a fallback for providers (like z.ai's glm-4.7) that don't always return JSON
func parseTextToVerification(text string) Verification {
	textLower := strings.ToLower(text)

	v := Verification{
		Issues:      []string{},
		Strengths:   []string{},
		Suggestion:  "",
		ErrorReason: "Parsed from text response (no JSON provided)",
	}

	// Look for explicit validity indicators
	if strings.Contains(textLower, "is_valid: true") || strings.Contains(textLower, "\"is_valid\":true") {
		v.IsValid = true
	} else if strings.Contains(textLower, "is_valid: false") || strings.Contains(textLower, "\"is_valid\":false") {
		v.IsValid = false
	} else {
		// Default: assume valid if no explicit issues found
		v.IsValid = true
	}

	// Extract score if present
	scorePattern := regexp.MustCompile(`score["\s:=]*["\s=:]*(\d+\.?\d*)`)
	if match := scorePattern.FindStringSubmatch(text); len(match) > 1 {
		if score, err := strconv.ParseFloat(match[1], 64); err == nil {
			v.Score = score
		}
	}
	if v.Score == 0 {
		// Default score based on validity
		if v.IsValid {
			v.Score = 0.8 // Default to 0.8 if valid
		} else {
			v.Score = 0.2 // Default to 0.2 if invalid
		}
	}

	// Look for issues in text
	issuePatterns := []string{
		"issue",
		"problem",
		"weakness",
		"gap",
		"flaw",
		"error",
		"incorrect",
		"missing",
		"concern",
	}
	for _, pattern := range issuePatterns {
		if strings.Contains(textLower, pattern) {
			// Try to extract specific issues mentioned
			// For now, just note that issues were found
			v.Issues = append(v.Issues, "Response mentioned issues: "+pattern)
			break
		}
	}

	// Look for strengths in text
	if !v.IsValid || len(v.Issues) == 0 {
		v.Strengths = append(v.Strengths, "Response provided analysis")
	} else {
		strengthPatterns := []string{"good", "strong", "valid", "correct", "solid"}
		for _, pattern := range strengthPatterns {
			if strings.Contains(textLower, pattern) {
				v.Strengths = append(v.Strengths, "Response noted positive aspects")
				break
			}
		}
	}
	return v

}

// FormatDialecticResult formats the result for display
func FormatDialecticResult(result *DialecticResult) string {
	var sb strings.Builder

	sb.WriteString("## Dialectical Reasoning Result\n\n")
	sb.WriteString(fmt.Sprintf("**Problem:** %s\n\n", result.Problem))
	sb.WriteString(fmt.Sprintf("**Provider:** %s\n", result.Provider))
	sb.WriteString(fmt.Sprintf("**Rounds:** %d\n", result.TotalRounds))
	if result.TotalToolCalls > 0 {
		sb.WriteString(fmt.Sprintf("**Tool calls:** %d\n", result.TotalToolCalls))
	}
	sb.WriteString(fmt.Sprintf("**Confidence:** %.1f%%\n\n", result.Confidence*100))

	if len(result.ToolsUsed) > 0 {
		sb.WriteString("### Tools Used\n\n")
		for tool, count := range result.ToolsUsed {
			sb.WriteString(fmt.Sprintf("- %s: %d calls\n", tool, count))
		}
		sb.WriteString("\n")
	}

	for _, step := range result.Steps {
		sb.WriteString(fmt.Sprintf("### Round %d\n\n", step.Round))

		sb.WriteString(fmt.Sprintf("**Thesis** (%.0f%% confidence):\n%s\n\n",
			step.Thesis.Verification.Score*100, step.Thesis.Content))

		if len(step.Thesis.Verification.ToolResults) > 0 {
			sb.WriteString("*Tool evidence:*\n")
			for _, tr := range step.Thesis.Verification.ToolResults {
				sb.WriteString(fmt.Sprintf("  - 🔧 [%s] %s\n", tr.Tool, utils.TruncateStr(tr.Output, 80)))
			}
			sb.WriteString("\n")
		}

		if len(step.Thesis.Verification.Issues) > 0 {
			sb.WriteString(fmt.Sprintf("*Issues:* %s\n\n", strings.Join(step.Thesis.Verification.Issues, "; ")))
		}

		sb.WriteString(fmt.Sprintf("**Antithesis** (%.0f%% confidence):\n%s\n\n",
			step.Antithesis.Verification.Score*100, step.Antithesis.Content))

		if len(step.Antithesis.Verification.ToolResults) > 0 {
			sb.WriteString("*Tool evidence:*\n")
			for _, tr := range step.Antithesis.Verification.ToolResults {
				sb.WriteString(fmt.Sprintf("  - 🔧 [%s] %s\n", tr.Tool, utils.TruncateStr(tr.Output, 80)))
			}
			sb.WriteString("\n")
		}

		sb.WriteString(fmt.Sprintf("**Synthesis** (%.0f%% confidence):\n%s\n\n",
			step.Synthesis.Verification.Score*100, step.Synthesis.Content))

		if len(step.Synthesis.Verification.ToolResults) > 0 {
			sb.WriteString("*Tool evidence:*\n")
			for _, tr := range step.Synthesis.Verification.ToolResults {
				sb.WriteString(fmt.Sprintf("  - 🔧 [%s] %s\n", tr.Tool, utils.TruncateStr(tr.Output, 80)))
			}
			sb.WriteString("\n")
		}

		if step.Resolved {
			sb.WriteString("✓ *Resolved*\n\n")
		}
		sb.WriteString("---\n\n")
	}

	sb.WriteString(fmt.Sprintf("### Final Answer\n\n%s\n", result.FinalAnswer))

	// JSON summary
	summary := map[string]interface{}{
		"success":      result.Success,
		"confidence":   result.Confidence,
		"total_rounds": result.TotalRounds,
		"provider":     result.Provider,
	}
	if result.TotalToolCalls > 0 {
		summary["total_tool_calls"] = result.TotalToolCalls
		summary["tools_used"] = result.ToolsUsed
	}
	jsonResult, _ := json.MarshalIndent(summary, "", "  ")

	sb.WriteString("\n### JSON Summary\n```json\n")
	sb.WriteString(string(jsonResult))
	sb.WriteString("\n```\n")

	return sb.String()
}
