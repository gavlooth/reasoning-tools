package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// GraphOfThoughts implements reasoning as a graph where thoughts can merge
type GraphOfThoughts struct {
	provider    Provider
	config      GoTConfig
	tools       *ToolRegistry
	nodes       map[string]*GoTNode
	nodesMu     sync.RWMutex
	totalVisits int
	toolCalls   int
	onProgress  func(ProgressUpdate)
}

// GoTConfig configures the Graph of Thoughts algorithm
type GoTConfig struct {
	BranchingFactor int      // Number of candidate thoughts per expansion (default: 3)
	MaxNodes        int      // Maximum nodes to explore (default: 30)
	MaxDepth        int      // Maximum reasoning depth (default: 8)
	MergeThreshold  float64  // Similarity threshold for merging (default: 0.7)
	MinScore        float64  // Minimum score to continue a path (default: 0.3)
	Temperature     float64  // LLM temperature for diversity (default: 0.8)
	EnableMerging   bool     // Whether to allow merging paths (default: true)
	EnableTools     bool     // Whether to allow tool usage during reasoning (default: false)
	MaxToolCalls    int      // Maximum tool calls total (default: 10)
	EnabledTools    []string // Which tools to enable (empty = all)
}

// DefaultGoTConfig returns sensible defaults
func DefaultGoTConfig() GoTConfig {
	return GoTConfig{
		BranchingFactor: 3,
		MaxNodes:        30,
		MaxDepth:        8,
		MergeThreshold:  0.7,
		MinScore:        0.3,
		Temperature:     0.8,
		EnableMerging:   true,
		EnableTools:     false,
		MaxToolCalls:    10,
		EnabledTools:    []string{},
	}
}

// GoTNode represents a node in the thought graph (can have multiple parents)
type GoTNode struct {
	ID          string      `json:"id"`
	NodeType    string      `json:"node_type"`              // "thought" or "tool"
	Thought     string      `json:"thought"`
	Depth       int         `json:"depth"`
	Score       float64     `json:"score"`
	Visits      int         `json:"visits"`
	TotalReward float64     `json:"total_reward"`
	Parents     []string    `json:"parents,omitempty"`      // Multiple parents allowed
	Children    []string    `json:"children,omitempty"`     // IDs of children
	IsTerminal  bool        `json:"is_terminal"`
	IsSolution  bool        `json:"is_solution"`
	Answer      string      `json:"answer,omitempty"`
	MergedFrom  []string    `json:"merged_from,omitempty"`  // IDs of nodes merged into this
	ToolCall    *ToolCall   `json:"tool_call,omitempty"`    // If this is a tool node
	ToolResult  *ToolResult `json:"tool_result,omitempty"`  // Result of tool execution
}

// GoTResult represents the complete result
type GoTResult struct {
	Problem        string              `json:"problem"`
	BestPath       []*GoTNode          `json:"best_path"`
	Graph          map[string]*GoTNode `json:"graph,omitempty"`
	FinalAnswer    string              `json:"final_answer"`
	TotalNodes     int                 `json:"total_nodes"`
	MergeCount     int                 `json:"merge_count"`
	TotalToolCalls int                 `json:"total_tool_calls"`
	ToolsUsed      map[string]int      `json:"tools_used,omitempty"`
	MaxDepth       int                 `json:"max_depth_reached"`
	Success        bool                `json:"success"`
	Provider       string              `json:"provider"`
}

// ProgressUpdate for streaming progress
type ProgressUpdate struct {
	Type        string  `json:"type"` // "thought", "tool", "evaluation", "merge", "solution"
	NodeID      string  `json:"node_id,omitempty"`
	Thought     string  `json:"thought,omitempty"`
	Score       float64 `json:"score,omitempty"`
	Depth       int     `json:"depth,omitempty"`
	TotalNodes  int     `json:"total_nodes,omitempty"`
	Message     string  `json:"message,omitempty"`
	IsSolution  bool    `json:"is_solution,omitempty"`
	FinalAnswer string  `json:"final_answer,omitempty"`
	ToolName    string  `json:"tool_name,omitempty"`
	ToolInput   string  `json:"tool_input,omitempty"`
	ToolOutput  string  `json:"tool_output,omitempty"`
}

// NewGraphOfThoughts creates a new GoT instance
func NewGraphOfThoughts(provider Provider, config GoTConfig) *GraphOfThoughts {
	g := &GraphOfThoughts{
		provider: provider,
		config:   config,
		nodes:    make(map[string]*GoTNode),
	}

	// Initialize tools if enabled
	if config.EnableTools {
		g.tools = NewToolRegistry()
		if len(config.EnabledTools) > 0 {
			g.tools.SetEnabled(config.EnabledTools)
		}
	}

	return g
}

// SetProgressCallback sets a callback for progress updates
func (g *GraphOfThoughts) SetProgressCallback(cb func(ProgressUpdate)) {
	g.onProgress = cb
}

func (g *GraphOfThoughts) emitProgress(update ProgressUpdate) {
	if g.onProgress != nil {
		g.onProgress(update)
	}
}

// Solve runs the Graph of Thoughts algorithm on a problem
func (g *GraphOfThoughts) Solve(ctx context.Context, problem string) (*GoTResult, error) {
	// Initialize root
	root := &GoTNode{
		ID:       "root",
		NodeType: "thought",
		Thought:  problem,
		Depth:    0,
		Score:    1.0,
		Visits:   1,
	}
	g.nodes["root"] = root
	g.totalVisits = 1
	g.toolCalls = 0

	result := &GoTResult{
		Problem:   problem,
		Provider:  g.provider.Name(),
		ToolsUsed: make(map[string]int),
	}

	g.emitProgress(ProgressUpdate{
		Type:       "thought",
		NodeID:     "root",
		Thought:    "Starting reasoning...",
		TotalNodes: 1,
	})

	var bestPath []*GoTNode
	var bestScore float64 = -1
	mergeCount := 0

	// Main exploration loop
	for g.totalVisits < g.config.MaxNodes {
		// Get expandable nodes (non-terminal leaves or high-scoring nodes)
		candidates := g.getExpansionCandidates()
		if len(candidates) == 0 {
			break
		}

		// Select best candidate using UCB1-like scoring
		selected := g.selectBestCandidate(candidates)
		if selected == nil {
			break
		}

		// Generate actions (thoughts and/or tool calls) from selected node
		actions, err := g.generateActions(ctx, selected, problem)
		if err != nil {
			continue
		}

		for i, action := range actions {
			nodeID := fmt.Sprintf("n%d_%d", g.totalVisits, i)
			var newNode *GoTNode

			if action.Type == "tool" && g.config.EnableTools && g.tools != nil && g.toolCalls < g.config.MaxToolCalls {
				// Execute tool and create tool node
				toolResult := g.tools.Execute(ctx, action.Tool, action.Input)
				g.toolCalls++
				result.ToolsUsed[action.Tool]++

				// Determine score based on tool success
				score := 0.7
				if !toolResult.Success {
					score = 0.3
				}

				newNode = &GoTNode{
					ID:       nodeID,
					NodeType: "tool",
					Thought:  fmt.Sprintf("Tool %s: %s", action.Tool, action.Input),
					Depth:    selected.Depth + 1,
					Score:    score,
					Visits:   1,
					TotalReward: score,
					Parents:  []string{selected.ID},
					ToolCall: &ToolCall{
						Tool:   action.Tool,
						Input:  action.Input,
						Reason: action.Content,
					},
					ToolResult: &toolResult,
				}

				g.emitProgress(ProgressUpdate{
					Type:       "tool",
					NodeID:     nodeID,
					ToolName:   action.Tool,
					ToolInput:  action.Input,
					ToolOutput: truncateStr(toolResult.Output, 100),
					Score:      score,
					Depth:      newNode.Depth,
					TotalNodes: len(g.nodes) + 1,
				})
			} else {
				// Regular thought node
				thought := action.Content

				// Check if this thought can be merged with existing nodes
				if g.config.EnableMerging {
					if mergeTarget := g.findMergeCandidate(ctx, thought, selected.Depth+1); mergeTarget != nil {
						// Merge instead of creating new node
						g.mergeIntoNode(mergeTarget, thought, selected.ID)
						mergeCount++

						g.emitProgress(ProgressUpdate{
							Type:       "merge",
							NodeID:     mergeTarget.ID,
							Message:    fmt.Sprintf("Merged thought into existing node %s", mergeTarget.ID),
							TotalNodes: len(g.nodes),
						})
						continue
					}
				}

				// Evaluate the new thought
				score, isSolution, answer, err := g.evaluateThought(ctx, thought, problem, selected)
				if err != nil {
					score = 0.5
				}

				newNode = &GoTNode{
					ID:          nodeID,
					NodeType:    "thought",
					Thought:     thought,
					Depth:       selected.Depth + 1,
					Score:       score,
					Visits:      1,
					TotalReward: score,
					Parents:     []string{selected.ID},
					IsTerminal:  isSolution || score < g.config.MinScore || selected.Depth+1 >= g.config.MaxDepth,
					IsSolution:  isSolution,
					Answer:      answer,
				}

				g.emitProgress(ProgressUpdate{
					Type:       "thought",
					NodeID:     nodeID,
					Thought:    truncateStr(thought, 100),
					Score:      score,
					Depth:      newNode.Depth,
					TotalNodes: len(g.nodes) + 1,
					IsSolution: isSolution,
				})

				if isSolution {
					path := g.getPathToNode(newNode)
					pathScore := g.calculatePathScore(path)
					if pathScore > bestScore {
						bestScore = pathScore
						bestPath = path
						result.FinalAnswer = answer

						g.emitProgress(ProgressUpdate{
							Type:        "solution",
							NodeID:      nodeID,
							Score:       pathScore,
							FinalAnswer: answer,
							Message:     "Found solution!",
						})
					}
				}
			}

			// Add node to graph
			g.nodesMu.Lock()
			g.nodes[nodeID] = newNode
			selected.Children = append(selected.Children, nodeID)
			g.nodesMu.Unlock()

			g.totalVisits++

			// Backpropagate
			g.backpropagate(newNode, newNode.Score)
		}

		// Early termination if we have a high-confidence solution
		if bestScore > 0.85 {
			break
		}
	}

	// If no solution found, extract best path
	if bestPath == nil {
		bestPath = g.getBestPath()
		if len(bestPath) > 0 && result.FinalAnswer == "" {
			result.FinalAnswer = g.extractFinalAnswer(ctx, bestPath, problem)
		}
	}

	result.BestPath = bestPath
	result.Graph = g.nodes
	result.TotalNodes = len(g.nodes)
	result.MergeCount = mergeCount
	result.TotalToolCalls = g.toolCalls
	result.MaxDepth = g.getMaxDepth()
	result.Success = result.FinalAnswer != ""

	return result, nil
}

// getExpansionCandidates returns nodes that can be expanded
func (g *GraphOfThoughts) getExpansionCandidates() []*GoTNode {
	g.nodesMu.RLock()
	defer g.nodesMu.RUnlock()

	var candidates []*GoTNode
	for _, node := range g.nodes {
		if !node.IsTerminal && node.Depth < g.config.MaxDepth {
			candidates = append(candidates, node)
		}
	}
	return candidates
}

// selectBestCandidate selects the best node to expand using UCB1
func (g *GraphOfThoughts) selectBestCandidate(candidates []*GoTNode) *GoTNode {
	if len(candidates) == 0 {
		return nil
	}

	var best *GoTNode
	bestUCB := math.Inf(-1)

	for _, node := range candidates {
		ucb := g.ucb1(node)
		if ucb > bestUCB {
			bestUCB = ucb
			best = node
		}
	}

	return best
}

func (g *GraphOfThoughts) ucb1(node *GoTNode) float64 {
	if node.Visits == 0 {
		return math.Inf(1)
	}
	exploitation := node.TotalReward / float64(node.Visits)
	exploration := 1.414 * math.Sqrt(math.Log(float64(g.totalVisits))/float64(node.Visits))
	// Bonus for nodes with multiple parents (merged nodes are often valuable)
	mergeBonus := float64(len(node.MergedFrom)) * 0.1
	return exploitation + exploration + mergeBonus
}

// GoTAction represents a candidate action (thought or tool call)
type GoTAction struct {
	Type    string `json:"type"`    // "thought" or "tool"
	Content string `json:"content"` // The thought content or tool reason
	Tool    string `json:"tool"`    // Tool name (if type == "tool")
	Input   string `json:"input"`   // Tool input (if type == "tool")
}

// generateActions generates candidate actions (thoughts and optionally tool calls)
func (g *GraphOfThoughts) generateActions(ctx context.Context, node *GoTNode, problem string) ([]GoTAction, error) {
	path := g.getPathToNode(node)
	pathStr := g.formatPathWithTools(path)

	var prompt string
	if g.config.EnableTools && g.tools != nil && g.toolCalls < g.config.MaxToolCalls {
		toolsPrompt := g.tools.GetToolsPrompt()
		prompt = fmt.Sprintf(`Problem: %s

Previous reasoning path:
%s

%s

Generate %d different next actions. Each can be either:
1. A reasoning thought that advances toward the solution
2. A tool call to compute, verify, or fetch information

Respond with ONLY a JSON array:
[
  {"type": "thought", "content": "reasoning step..."},
  {"type": "tool", "tool": "calculator", "input": "17 * 23", "content": "verify multiplication"},
  {"type": "thought", "content": "another approach..."}
]

Be strategic - use tools when computation or verification would help.`, problem, pathStr, toolsPrompt, g.config.BranchingFactor)
	} else {
		prompt = fmt.Sprintf(`Problem: %s

Previous reasoning path:
%s

Generate %d different next reasoning steps. Each should:
1. Explore a different angle or approach
2. Build meaningfully on the previous reasoning
3. Be specific and actionable

Respond with ONLY a JSON array of strings:
["thought 1", "thought 2", "thought 3"]`, problem, pathStr, g.config.BranchingFactor)
	}

	messages := []ChatMessage{
		{Role: "system", Content: "You are a thoughtful reasoning assistant. Generate diverse, creative reasoning steps."},
		{Role: "user", Content: prompt},
	}

	response, err := g.provider.Chat(ctx, messages, ChatOptions{
		Temperature: g.config.Temperature,
		MaxTokens:   2048,
	})
	if err != nil {
		return nil, err
	}

	return g.parseActions(response), nil
}

// parseActions parses the LLM response into actions
func (g *GraphOfThoughts) parseActions(response string) []GoTAction {
	var actions []GoTAction

	// Try to parse as JSON array of actions
	jsonStr := extractJSONArray(response)
	if jsonStr != "" {
		// Try parsing as array of GoTAction objects
		if err := json.Unmarshal([]byte(jsonStr), &actions); err == nil && len(actions) > 0 {
			return actions
		}

		// Try parsing as array of strings (old format)
		var strings []string
		if err := json.Unmarshal([]byte(jsonStr), &strings); err == nil {
			for _, s := range strings {
				actions = append(actions, GoTAction{Type: "thought", Content: s})
			}
			return actions
		}
	}

	// Fallback: parse as candidates (old behavior)
	candidates := g.parseCandidates(response)
	for _, c := range candidates {
		actions = append(actions, GoTAction{Type: "thought", Content: c})
	}

	return actions
}

// formatPathWithTools formats a path including tool results
func (g *GraphOfThoughts) formatPathWithTools(path []*GoTNode) string {
	if len(path) <= 1 {
		return "(starting point)"
	}

	var parts []string
	for i, node := range path[1:] {
		mergeInfo := ""
		if len(node.MergedFrom) > 0 {
			mergeInfo = fmt.Sprintf(" [merged %d paths]", len(node.MergedFrom)+1)
		}

		if node.NodeType == "tool" && node.ToolResult != nil {
			parts = append(parts, fmt.Sprintf("%d. (%.2f)%s [TOOL: %s] %s\n   → Result: %s",
				i+1, node.Score, mergeInfo, node.ToolCall.Tool, node.ToolCall.Input,
				truncateStr(node.ToolResult.Output, 100)))
		} else {
			parts = append(parts, fmt.Sprintf("%d. (%.2f)%s %s", i+1, node.Score, mergeInfo, node.Thought))
		}
	}
	return strings.Join(parts, "\n")
}

// extractJSONArray extracts a JSON array from a string
func extractJSONArray(s string) string {
	start := strings.Index(s, "[")
	if start == -1 {
		return ""
	}

	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// findMergeCandidate finds a node to merge with based on semantic similarity
func (g *GraphOfThoughts) findMergeCandidate(ctx context.Context, thought string, depth int) *GoTNode {
	g.nodesMu.RLock()
	defer g.nodesMu.RUnlock()

	// Find nodes at similar depth that might be similar
	var candidates []*GoTNode
	for _, node := range g.nodes {
		if node.Depth == depth && !node.IsTerminal && !node.IsSolution {
			candidates = append(candidates, node)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Use LLM to check similarity
	for _, candidate := range candidates {
		similar, err := g.checkSimilarity(ctx, thought, candidate.Thought)
		if err == nil && similar {
			return candidate
		}
	}

	return nil
}

// checkSimilarity asks LLM if two thoughts are semantically similar
func (g *GraphOfThoughts) checkSimilarity(ctx context.Context, thought1, thought2 string) (bool, error) {
	prompt := fmt.Sprintf(`Are these two reasoning steps essentially expressing the same idea or reaching the same conclusion?

Thought 1: %s

Thought 2: %s

Respond with ONLY "yes" or "no".`, thought1, thought2)

	messages := []ChatMessage{
		{Role: "user", Content: prompt},
	}

	response, err := g.provider.Chat(ctx, messages, ChatOptions{
		Temperature: 0.1,
		MaxTokens:   10,
	})
	if err != nil {
		return false, err
	}

	return strings.ToLower(strings.TrimSpace(response)) == "yes", nil
}

// mergeIntoNode merges a new thought into an existing node
func (g *GraphOfThoughts) mergeIntoNode(target *GoTNode, newThought, parentID string) {
	g.nodesMu.Lock()
	defer g.nodesMu.Unlock()

	// Add new parent
	hasParent := false
	for _, p := range target.Parents {
		if p == parentID {
			hasParent = true
			break
		}
	}
	if !hasParent {
		target.Parents = append(target.Parents, parentID)
	}

	// Track merged thoughts
	target.MergedFrom = append(target.MergedFrom, newThought)

	// Boost score slightly for merged nodes
	target.Score = math.Min(1.0, target.Score+0.05)
	target.Visits++
	target.TotalReward += target.Score

	// Update parent's children
	if parent, ok := g.nodes[parentID]; ok {
		hasChild := false
		for _, c := range parent.Children {
			if c == target.ID {
				hasChild = true
				break
			}
		}
		if !hasChild {
			parent.Children = append(parent.Children, target.ID)
		}
	}
}

// evaluateThought scores a thought and checks if it's a solution
func (g *GraphOfThoughts) evaluateThought(ctx context.Context, thought, problem string, parent *GoTNode) (float64, bool, string, error) {
	path := g.getPathToNode(parent)
	pathStr := g.formatPath(path)

	prompt := fmt.Sprintf(`Evaluate this reasoning step for the problem.

Problem: %s

Previous reasoning:
%s

New thought to evaluate:
%s

Respond with ONLY a JSON object:
{
  "score": <0.0 to 1.0, how promising is this thought>,
  "is_solution": <true if this completes the reasoning with a final answer>,
  "answer": "<final answer if is_solution is true, otherwise empty>",
  "reasoning": "<brief explanation>"
}`, problem, pathStr, thought)

	messages := []ChatMessage{
		{Role: "system", Content: "You are a critical evaluator of reasoning steps. Be strict but fair."},
		{Role: "user", Content: prompt},
	}

	response, err := g.provider.Chat(ctx, messages, ChatOptions{
		Temperature: 0.3,
		MaxTokens:   512,
	})
	if err != nil {
		return 0.5, false, "", err
	}

	return parseGoTEvaluation(response)
}

// backpropagate updates scores up the graph (handles multiple parents)
func (g *GraphOfThoughts) backpropagate(node *GoTNode, reward float64) {
	g.nodesMu.Lock()
	defer g.nodesMu.Unlock()

	visited := make(map[string]bool)
	g.backpropagateRecursive(node, reward, visited)
}

func (g *GraphOfThoughts) backpropagateRecursive(node *GoTNode, reward float64, visited map[string]bool) {
	for _, parentID := range node.Parents {
		if visited[parentID] {
			continue
		}
		visited[parentID] = true

		if parent, ok := g.nodes[parentID]; ok {
			parent.Visits++
			parent.TotalReward += reward
			g.backpropagateRecursive(parent, reward, visited)
		}
	}
}

// getPathToNode returns one path from root to node (picks first parent if multiple)
func (g *GraphOfThoughts) getPathToNode(node *GoTNode) []*GoTNode {
	g.nodesMu.RLock()
	defer g.nodesMu.RUnlock()

	var path []*GoTNode
	current := node
	visited := make(map[string]bool)

	for current != nil {
		if visited[current.ID] {
			break // Avoid cycles
		}
		visited[current.ID] = true
		path = append([]*GoTNode{current}, path...)

		if len(current.Parents) > 0 {
			current = g.nodes[current.Parents[0]]
		} else {
			break
		}
	}
	return path
}

// getBestPath returns the highest-scoring path to a terminal/solution node
func (g *GraphOfThoughts) getBestPath() []*GoTNode {
	g.nodesMu.RLock()
	defer g.nodesMu.RUnlock()

	var bestPath []*GoTNode
	var bestScore float64 = -1

	for _, node := range g.nodes {
		if node.IsSolution || (node.IsTerminal && len(node.Children) == 0) {
			path := g.getPathToNode(node)
			score := g.calculatePathScore(path)
			if score > bestScore {
				bestScore = score
				bestPath = path
			}
		}
	}

	return bestPath
}

// calculatePathScore calculates the average score of a path
func (g *GraphOfThoughts) calculatePathScore(path []*GoTNode) float64 {
	if len(path) == 0 {
		return 0
	}
	var total float64
	for _, node := range path {
		total += node.Score
	}
	return total / float64(len(path))
}

// getMaxDepth returns the maximum depth reached in the graph
func (g *GraphOfThoughts) getMaxDepth() int {
	g.nodesMu.RLock()
	defer g.nodesMu.RUnlock()

	maxDepth := 0
	for _, node := range g.nodes {
		if node.Depth > maxDepth {
			maxDepth = node.Depth
		}
	}
	return maxDepth
}

// extractFinalAnswer generates a final answer from the best path
func (g *GraphOfThoughts) extractFinalAnswer(ctx context.Context, path []*GoTNode, problem string) string {
	pathStr := g.formatPath(path)

	prompt := fmt.Sprintf(`Based on this reasoning chain, provide the final answer.

Problem: %s

Reasoning:
%s

Provide ONLY the final answer, nothing else.`, problem, pathStr)

	messages := []ChatMessage{
		{Role: "user", Content: prompt},
	}

	response, err := g.provider.Chat(ctx, messages, ChatOptions{
		Temperature: 0.3,
		MaxTokens:   512,
	})
	if err != nil {
		return ""
	}

	return strings.TrimSpace(response)
}

// formatPath formats a path for display
func (g *GraphOfThoughts) formatPath(path []*GoTNode) string {
	if len(path) <= 1 {
		return "(starting point)"
	}

	var parts []string
	for i, node := range path[1:] { // Skip root
		mergeInfo := ""
		if len(node.MergedFrom) > 0 {
			mergeInfo = fmt.Sprintf(" [merged %d paths]", len(node.MergedFrom)+1)
		}
		parts = append(parts, fmt.Sprintf("%d. (%.2f)%s %s", i+1, node.Score, mergeInfo, node.Thought))
	}
	return strings.Join(parts, "\n")
}

// parseCandidates parses LLM response into thought candidates
func (g *GraphOfThoughts) parseCandidates(response string) []string {
	var candidates []string

	jsonStr := extractJSON(response)
	if jsonStr != "" {
		if err := json.Unmarshal([]byte(jsonStr), &candidates); err == nil {
			return candidates
		}
	}

	// Fallback: parse numbered list
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "[" || line == "]" {
			continue
		}
		line = strings.TrimLeft(line, "0123456789.-) \"")
		line = strings.TrimRight(line, "\",")
		if len(line) > 10 {
			candidates = append(candidates, line)
		}
	}

	if len(candidates) > g.config.BranchingFactor {
		candidates = candidates[:g.config.BranchingFactor]
	}

	return candidates
}

func parseGoTEvaluation(response string) (float64, bool, string, error) {
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return 0.5, false, "", fmt.Errorf("no JSON in response")
	}

	var eval struct {
		Score      float64 `json:"score"`
		IsSolution bool    `json:"is_solution"`
		Answer     string  `json:"answer"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &eval); err != nil {
		return 0.5, false, "", err
	}

	// Clamp score
	if eval.Score < 0 {
		eval.Score = 0
	}
	if eval.Score > 1 {
		eval.Score = 1
	}

	return eval.Score, eval.IsSolution, eval.Answer, nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// FormatGoTResult formats the result for display
func FormatGoTResult(result *GoTResult) string {
	var sb strings.Builder

	sb.WriteString("## Graph of Thoughts Result\n\n")
	sb.WriteString(fmt.Sprintf("**Problem:** %s\n\n", result.Problem))
	sb.WriteString(fmt.Sprintf("**Provider:** %s\n", result.Provider))
	sb.WriteString(fmt.Sprintf("**Nodes explored:** %d\n", result.TotalNodes))
	sb.WriteString(fmt.Sprintf("**Path merges:** %d\n", result.MergeCount))
	if result.TotalToolCalls > 0 {
		sb.WriteString(fmt.Sprintf("**Tool calls:** %d\n", result.TotalToolCalls))
	}
	sb.WriteString(fmt.Sprintf("**Max depth:** %d\n\n", result.MaxDepth))

	if len(result.ToolsUsed) > 0 {
		sb.WriteString("### Tools Used\n\n")
		for tool, count := range result.ToolsUsed {
			sb.WriteString(fmt.Sprintf("- %s: %d calls\n", tool, count))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("### Best Reasoning Path\n\n")
	for i, node := range result.BestPath {
		if i == 0 {
			continue // Skip root
		}
		mergeInfo := ""
		if len(node.MergedFrom) > 0 {
			mergeInfo = fmt.Sprintf(" [merged %d paths]", len(node.MergedFrom)+1)
		}

		icon := "💭"
		if node.NodeType == "tool" {
			icon = "🔧"
		}

		if node.NodeType == "tool" && node.ToolResult != nil {
			sb.WriteString(fmt.Sprintf("%d. %s (score: %.2f)%s [%s] %s\n", i, icon, node.Score, mergeInfo, node.ToolCall.Tool, node.ToolCall.Input))
			sb.WriteString(fmt.Sprintf("   → %s\n\n", truncateStr(node.ToolResult.Output, 100)))
		} else {
			sb.WriteString(fmt.Sprintf("%d. %s (score: %.2f)%s %s\n\n", i, icon, node.Score, mergeInfo, node.Thought))
		}
	}

	sb.WriteString(fmt.Sprintf("### Final Answer\n\n%s\n", result.FinalAnswer))

	// JSON summary
	summary := map[string]interface{}{
		"success":      result.Success,
		"final_answer": result.FinalAnswer,
		"total_nodes":  result.TotalNodes,
		"merge_count":  result.MergeCount,
		"max_depth":    result.MaxDepth,
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

// SortNodesByScore returns nodes sorted by score
func SortNodesByScore(nodes map[string]*GoTNode) []*GoTNode {
	var sorted []*GoTNode
	for _, n := range nodes {
		sorted = append(sorted, n)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})
	return sorted
}
