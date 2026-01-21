package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"reasoning-tools/utils"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// StreamMode controls the type of streaming output
type StreamMode string

const (
	StreamModeNone   StreamMode = "none"   // Default, backward compatible - no streaming
	StreamModeTokens StreamMode = "tokens" // LLM tokens only
	StreamModeEvents StreamMode = "events" // Progress events only (current behavior)
	StreamModeBoth   StreamMode = "both"   // Both tokens and events
)

// ParseStreamMode converts a string to StreamMode with fallback to default
func ParseStreamMode(s string) StreamMode {
	switch strings.ToLower(s) {
	case "tokens":
		return StreamModeTokens
	case "events":
		return StreamModeEvents
	case "both":
		return StreamModeBoth
	case "none", "":
		return StreamModeNone
	default:
		return StreamModeNone
	}
}

// ShouldStreamTokens returns true if token streaming is enabled
func (m StreamMode) ShouldStreamTokens() bool {
	return m == StreamModeTokens || m == StreamModeBoth
}

// ShouldStreamEvents returns true if event streaming is enabled
func (m StreamMode) ShouldStreamEvents() bool {
	return m == StreamModeEvents || m == StreamModeBoth
}

// EventType constants for streaming events
const (
	EventTypeThought    = "thought"
	EventTypeEvaluation = "evaluation"
	EventTypeMerge      = "merge"
	EventTypeSolution   = "solution"
	EventTypeError      = "error"
	EventTypeProgress   = "progress"
	EventTypeToken      = "token"
	EventTypeTool       = "tool"
)

// StreamingManager handles progress streaming for reasoning operations
type StreamingManager struct {
	buffer    []StreamEvent
	mu        sync.RWMutex
	startTime time.Time
	toolName  string
}

// StreamEvent represents a single streaming event
type StreamEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"` // thought, evaluation, merge, solution, error, progress
	NodeID      string    `json:"node_id,omitempty"`
	Content     string    `json:"content"`
	Score       float64   `json:"score,omitempty"`
	Depth       int       `json:"depth,omitempty"`
	TotalNodes  int       `json:"total_nodes,omitempty"`
	IsSolution  bool      `json:"is_solution,omitempty"`
	FinalAnswer string    `json:"final_answer,omitempty"`
	ElapsedMs   int64     `json:"elapsed_ms"`
}

// NewStreamingManager creates a new streaming manager
func NewStreamingManager(toolName string) *StreamingManager {
	return &StreamingManager{
		buffer:    []StreamEvent{},
		startTime: time.Now(),
		toolName:  toolName,
	}
}

// AddEvent adds a new event to the stream
func (sm *StreamingManager) AddEvent(eventType, content string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	event := StreamEvent{
		Timestamp: time.Now(),
		Type:      eventType,
		Content:   content,
		ElapsedMs: time.Since(sm.startTime).Milliseconds(),
	}
	sm.buffer = append(sm.buffer, event)
}

// AddProgressEvent adds a progress update event
func (sm *StreamingManager) AddProgressEvent(update ProgressUpdate) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	event := StreamEvent{
		Timestamp:   time.Now(),
		Type:        update.Type,
		NodeID:      update.NodeID,
		Content:     update.Message,
		Score:       update.Score,
		Depth:       update.Depth,
		TotalNodes:  update.TotalNodes,
		IsSolution:  update.IsSolution,
		FinalAnswer: update.FinalAnswer,
		ElapsedMs:   time.Since(sm.startTime).Milliseconds(),
	}

	if event.Content == "" && update.Thought != "" {
		event.Content = update.Thought
	}

	sm.buffer = append(sm.buffer, event)
}

// AddTokenEvent adds a token streaming event
func (sm *StreamingManager) AddTokenEvent(token, accumulated string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	event := StreamEvent{
		Timestamp: time.Now(),
		Type:      EventTypeToken,
		Content:   token,
		ElapsedMs: time.Since(sm.startTime).Milliseconds(),
	}
	sm.buffer = append(sm.buffer, event)
}

// GetEvents returns all events
func (sm *StreamingManager) GetEvents() []StreamEvent {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return append([]StreamEvent{}, sm.buffer...)
}

// GetLastEvents returns the last n events
func (sm *StreamingManager) GetLastEvents(n int) []StreamEvent {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if n >= len(sm.buffer) {
		return append([]StreamEvent{}, sm.buffer...)
	}
	return append([]StreamEvent{}, sm.buffer[len(sm.buffer)-n:]...)
}

// Clear removes all events from the buffer, preventing memory leaks
func (sm *StreamingManager) Clear() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.buffer = sm.buffer[:0]
}

// FormatAsText formats events as human-readable text
func (sm *StreamingManager) FormatAsText() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## %s Stream Log\n\n", sm.toolName))

	for _, event := range sm.buffer {
		icon := getEventIcon(event.Type)
		elapsed := formatDuration(event.ElapsedMs)

		if event.Type == "solution" {
			sb.WriteString(fmt.Sprintf("\n%s **[%s] SOLUTION FOUND**\n", icon, elapsed))
			if event.FinalAnswer != "" {
				sb.WriteString(fmt.Sprintf("Answer: %s\n", event.FinalAnswer))
			}
		} else if event.Type == "merge" {
			sb.WriteString(fmt.Sprintf("%s [%s] Merged: %s\n", icon, elapsed, event.Content))
		} else if event.Type == "evaluation" {
			sb.WriteString(fmt.Sprintf("%s [%s] Eval (%.2f): %s\n", icon, elapsed, event.Score, utils.TruncateStr(event.Content, 80)))
		} else {
			nodeInfo := ""
			if event.NodeID != "" {
				nodeInfo = fmt.Sprintf("[%s] ", event.NodeID)
			}
			depthInfo := ""
			if event.Depth > 0 {
				depthInfo = fmt.Sprintf("(d%d) ", event.Depth)
			}
			scoreInfo := ""
			if event.Score > 0 {
				scoreInfo = fmt.Sprintf("(%.2f) ", event.Score)
			}
			sb.WriteString(fmt.Sprintf("%s [%s] %s%s%s%s\n", icon, elapsed, nodeInfo, depthInfo, scoreInfo, utils.TruncateStr(event.Content, 80)))
		}
	}

	return sb.String()
}

// FormatAsJSON formats events as JSON
func (sm *StreamingManager) FormatAsJSON() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	data, _ := json.MarshalIndent(map[string]interface{}{
		"tool":         sm.toolName,
		"start_time":   sm.startTime.Format(time.RFC3339),
		"total_events": len(sm.buffer),
		"events":       sm.buffer,
	}, "", "  ")
	return string(data)
}

// FormatCompact formats events in a compact format suitable for streaming
func (sm *StreamingManager) FormatCompact() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var lines []string
	for _, event := range sm.buffer {
		icon := getEventIcon(event.Type)
		line := fmt.Sprintf("%s %s", icon, utils.TruncateStr(event.Content, 60))
		if event.Score > 0 {
			line = fmt.Sprintf("%s (%.2f)", line, event.Score)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// Milliseconds represents a duration in milliseconds
type Milliseconds int64

// FromDuration creates Milliseconds from time.Duration
func FromDuration(d time.Duration) Milliseconds {
	return Milliseconds(d / time.Millisecond)
}

// ToDuration converts Milliseconds to time.Duration
func (m Milliseconds) ToDuration() time.Duration {
	return time.Duration(m) * time.Millisecond
}

// MarshalJSON implements json.Marshaler
func (m Milliseconds) MarshalJSON() ([]byte, error) {
	return json.Marshal(int64(m))
}

// UnmarshalJSON implements json.Unmarshaler
func (m *Milliseconds) UnmarshalJSON(data []byte) error {
	var raw int64
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = Milliseconds(raw)
	return nil
}

// StreamingSummary provides a summary of the streaming session
type StreamingSummary struct {
	TotalEvents      int            `json:"total_events"`
	TotalDuration    Milliseconds   `json:"total_duration_ms"`
	EventsByType     map[string]int `json:"events_by_type"`
	SolutionFound    bool           `json:"solution_found"`
	FinalAnswer      string         `json:"final_answer,omitempty"`
	MaxDepthReached  int            `json:"max_depth_reached"`
	MaxNodesExplored int            `json:"max_nodes_explored"`
}

// GetSummary returns a summary of the streaming session
func (sm *StreamingManager) GetSummary() StreamingSummary {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	summary := StreamingSummary{
		TotalEvents:   len(sm.buffer),
		TotalDuration: FromDuration(time.Since(sm.startTime)),
		EventsByType:  make(map[string]int),
	}

	for _, event := range sm.buffer {
		summary.EventsByType[event.Type]++

		if event.Type == "solution" {
			summary.SolutionFound = true
			summary.FinalAnswer = event.FinalAnswer
		}

		if event.Depth > summary.MaxDepthReached {
			summary.MaxDepthReached = event.Depth
		}
		if event.TotalNodes > summary.MaxNodesExplored {
			summary.MaxNodesExplored = event.TotalNodes
		}
	}

	return summary
}

// Helper functions

func getEventIcon(eventType string) string {
	switch eventType {
	case "thought":
		return "💭"
	case "evaluation":
		return "📊"
	case "merge":
		return "🔀"
	case "solution":
		return "✅"
	case "error":
		return "❌"
	case "progress":
		return "⏳"
	default:
		return "•"
	}
}

func formatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	if ms < 60000 {
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	minutes := ms / 60000
	seconds := (ms % 60000) / 1000
	return fmt.Sprintf("%dm%ds", minutes, seconds)
}

// StreamingResult wraps a result with its streaming data
type StreamingResult struct {
	Result  interface{}      `json:"result"`
	Stream  []StreamEvent    `json:"stream,omitempty"`
	Summary StreamingSummary `json:"summary"`
}

// WrapWithStreaming wraps any result with streaming data
func WrapWithStreaming(result interface{}, sm *StreamingManager, includeEvents bool) StreamingResult {
	wrapped := StreamingResult{
		Result:  result,
		Summary: sm.GetSummary(),
	}
	if includeEvents {
		wrapped.Stream = sm.GetEvents()
	}
	return wrapped
}

// StreamProgressToText converts ProgressUpdate to a text line for immediate output
func StreamProgressToText(update ProgressUpdate) string {
	icon := getEventIcon(update.Type)

	var parts []string
	parts = append(parts, icon)

	if update.NodeID != "" {
		parts = append(parts, fmt.Sprintf("[%s]", update.NodeID))
	}

	if update.Depth > 0 {
		parts = append(parts, fmt.Sprintf("(d%d)", update.Depth))
	}

	if update.Score > 0 {
		parts = append(parts, fmt.Sprintf("(%.2f)", update.Score))
	}

	if update.Thought != "" {
		parts = append(parts, utils.TruncateStr(update.Thought, 80))
	} else if update.Message != "" {
		parts = append(parts, update.Message)
	}

	if update.IsSolution {
		parts = append(parts, "✓")
	}

	return strings.Join(parts, " ")
}

// LiveStream provides a channel-based streaming interface
type LiveStream struct {
	events chan StreamEvent
	done   chan struct{}
	closed bool
	mu     sync.Mutex
}

// NewLiveStream creates a new live streaming channel
func NewLiveStream(bufferSize int) *LiveStream {
	return &LiveStream{
		events: make(chan StreamEvent, bufferSize),
		done:   make(chan struct{}),
	}
}

// Send sends an event to the stream
func (ls *LiveStream) Send(event StreamEvent) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if ls.closed {
		return
	}

	select {
	case ls.events <- event:
	default:
		// Buffer full, drop oldest
		select {
		case <-ls.events:
			ls.events <- event
		default:
		}
	}
}

// SendProgress sends a progress update as an event
func (ls *LiveStream) SendProgress(update ProgressUpdate, startTime time.Time) {
	event := StreamEvent{
		Timestamp:   time.Now(),
		Type:        update.Type,
		NodeID:      update.NodeID,
		Score:       update.Score,
		Depth:       update.Depth,
		TotalNodes:  update.TotalNodes,
		IsSolution:  update.IsSolution,
		FinalAnswer: update.FinalAnswer,
		ElapsedMs:   time.Since(startTime).Milliseconds(),
	}

	if update.Thought != "" {
		event.Content = update.Thought
	} else {
		event.Content = update.Message
	}

	ls.Send(event)
}

// Events returns the event channel for reading
func (ls *LiveStream) Events() <-chan StreamEvent {
	return ls.events
}

// Done returns a channel that's closed when the stream ends
func (ls *LiveStream) Done() <-chan struct{} {
	return ls.done
}

// Close closes the stream
func (ls *LiveStream) Close() {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if !ls.closed {
		ls.closed = true
		close(ls.events)
		close(ls.done)
	}
}

// ============ MCP Notifier ============

// MCPNotifier helps send structured notifications to MCP clients
type MCPNotifier struct {
	ctx          context.Context
	mcpServer    *server.MCPServer
	logger       string
	streamMode   StreamMode
	stderrStream bool // Enable real-time stderr output for tokens
	mcpLogging   bool // Enable MCP logging notifications
	mcpProgress  bool // Enable MCP progress notifications
}

// NotifierConfig holds configuration for the MCPNotifier
type NotifierConfig struct {
	StderrStream bool // Write tokens to stderr
	MCPLogging   bool // Send MCP logging notifications
	MCPProgress  bool // Send MCP progress notifications
}

// NewMCPNotifier creates a new MCP notifier from context
func NewMCPNotifier(ctx context.Context, logger string, mode StreamMode, config NotifierConfig) *MCPNotifier {
	mcpServer := server.ServerFromContext(ctx)
	return &MCPNotifier{
		ctx:          ctx,
		mcpServer:    mcpServer,
		logger:       logger,
		streamMode:   mode,
		stderrStream: config.StderrStream,
		mcpLogging:   config.MCPLogging,
		mcpProgress:  config.MCPProgress,
	}
}

// IsActive returns true if the notifier can send notifications
func (n *MCPNotifier) IsActive() bool {
	return n.mcpServer != nil && n.streamMode != StreamModeNone
}

// SendProgress sends a structured progress notification via MCP logging
func (n *MCPNotifier) SendProgress(update ProgressUpdate) {
	if n.mcpServer == nil || !n.mcpLogging || !n.streamMode.ShouldStreamEvents() {
		return
	}

	// Build structured data for the notification
	data := map[string]interface{}{
		"type": update.Type,
	}

	if update.NodeID != "" {
		data["node_id"] = update.NodeID
	}
	if update.Thought != "" {
		data["content"] = update.Thought
	} else if update.Message != "" {
		data["content"] = update.Message
	}
	if update.Score > 0 {
		data["score"] = update.Score
	}
	if update.Depth > 0 {
		data["depth"] = update.Depth
	}
	if update.TotalNodes > 0 {
		data["total_nodes"] = update.TotalNodes
	}
	if update.IsSolution {
		data["is_solution"] = true
	}
	if update.FinalAnswer != "" {
		data["final_answer"] = update.FinalAnswer
	}
	if update.ToolName != "" {
		data["tool_name"] = update.ToolName
		data["tool_input"] = update.ToolInput
		data["tool_output"] = update.ToolOutput
	}

	n.mcpServer.SendLogMessageToClient(n.ctx, mcp.LoggingMessageNotification{
		Params: mcp.LoggingMessageNotificationParams{
			Level:  mcp.LoggingLevelInfo,
			Logger: n.logger,
			Data:   data,
		},
	})
}

// SendToken sends a token notification
func (n *MCPNotifier) SendToken(token string) {
	// Write to stderr if enabled (real-time terminal output)
	if n.stderrStream {
		fmt.Fprint(os.Stderr, token)
	}

	// Send MCP logging notification if enabled
	if n.mcpServer != nil && n.mcpLogging && n.streamMode.ShouldStreamTokens() {
		data := map[string]interface{}{
			"type":  EventTypeToken,
			"token": token,
		}

		n.mcpServer.SendLogMessageToClient(n.ctx, mcp.LoggingMessageNotification{
			Params: mcp.LoggingMessageNotificationParams{
				Level:  mcp.LoggingLevelDebug,
				Logger: n.logger,
				Data:   data,
			},
		})
	}
}

// SendText sends a simple text notification (for backward compatibility)
func (n *MCPNotifier) SendText(message string) {
	if n.mcpServer == nil {
		return
	}

	n.mcpServer.SendLogMessageToClient(n.ctx, mcp.LoggingMessageNotification{
		Params: mcp.LoggingMessageNotificationParams{
			Level:  mcp.LoggingLevelInfo,
			Logger: n.logger,
			Data:   message,
		},
	})
}

// SendProgressNotification sends an MCP progress notification
// This is the standard MCP way to report progress to clients
func (n *MCPNotifier) SendProgressNotification(progressToken interface{}, progress, total int, message string) {
	if n.mcpServer == nil || !n.mcpProgress {
		return
	}

	params := map[string]interface{}{
		"progressToken": progressToken,
		"progress":      progress,
		"total":         total,
	}
	if message != "" {
		params["message"] = message
	}

	n.mcpServer.SendNotificationToClient(n.ctx, "notifications/progress", params)
}

// ============ Streaming Setup Helper ============

// StreamingContext holds all streaming components for a tool execution
type StreamingContext struct {
	Manager       *StreamingManager
	Notifier      *MCPNotifier
	Mode          StreamMode
	progressToken interface{} // Token for progress notifications
	currentStep   int         // Current progress step
	totalSteps    int         // Total expected steps
}

// SetProgressTotal sets the total number of steps for progress tracking
func (sc *StreamingContext) SetProgressTotal(total int) {
	sc.totalSteps = total
	sc.currentStep = 0
	sc.progressToken = fmt.Sprintf("%s-%d", sc.Manager.toolName, time.Now().UnixNano())
}

// SendProgressStep sends a progress notification for the current step
func (sc *StreamingContext) SendProgressStep(message string) {
	sc.currentStep++
	if sc.Notifier != nil && sc.totalSteps > 0 {
		sc.Notifier.SendProgressNotification(sc.progressToken, sc.currentStep, sc.totalSteps, message)
	}
}

// SetupStreaming creates streaming infrastructure for a tool execution
//
// Stream mode (what to include in result):
// 1. stream_mode parameter (values: "", "none", "events", "tokens", "both")
// 2. stream boolean parameter (true maps to "events" for backward compatibility)
// 3. REASONING_STREAM_MODE environment variable
// 4. REASONING_STREAM environment variable
//
// Output channels (all can be enabled simultaneously):
// - stderr_stream / MCP_STDERR_STREAM: Write tokens to stderr
// - mcp_logging / MCP_LOGGING_STREAM: Send MCP logging notifications
// - mcp_progress / MCP_PROGRESS_STREAM: Send MCP progress notifications
func SetupStreaming(ctx context.Context, args map[string]interface{}, toolName string) *StreamingContext {
	// Determine stream mode using clear precedence hierarchy
	mode := determineStreamMode(args)

	// Build notifier config - each channel is independent
	config := NotifierConfig{
		StderrStream: determineBoolFlag(args, "stderr_stream", "MCP_STDERR_STREAM"),
		MCPLogging:   determineBoolFlag(args, "mcp_logging", "MCP_LOGGING_STREAM"),
		MCPProgress:  determineBoolFlag(args, "mcp_progress", "MCP_PROGRESS_STREAM"),
	}

	return &StreamingContext{
		Manager:  NewStreamingManager(toolName),
		Notifier: NewMCPNotifier(ctx, toolName, mode, config),
		Mode:     mode,
	}
}

// determineBoolFlag checks a boolean flag from args then environment variable
func determineBoolFlag(args map[string]interface{}, argName, envName string) bool {
	// 1. Check parameter first
	if val, ok := args[argName].(bool); ok {
		return val
	}

	// 2. Check environment variable
	envVal := strings.ToLower(getEnvOrDefault(envName, ""))
	return envVal == "true" || envVal == "1"
}

// determineStreamMode implements the precedence hierarchy for streaming configuration
func determineStreamMode(args map[string]interface{}) StreamMode {
	// 1. Check stream_mode parameter first (new unified parameter)
	if modeStr, ok := args["stream_mode"].(string); ok && modeStr != "" {
		mode := ParseStreamMode(modeStr)
		if mode == StreamModeNone && modeStr != "" && modeStr != "none" {
			// Invalid stream_mode value was provided - log but still use default
			// This allows the function to continue gracefully while alerting to configuration issues
			fmt.Fprintf(os.Stderr, "Warning: Invalid stream_mode value '%s'. Expected 'none', 'events', 'tokens', or 'both'. Using default 'none'.\n", modeStr)
		}
		return mode
	}

	// 2. Check stream boolean parameter (backward compatibility)
	if stream, ok := args["stream"].(bool); ok && stream {
		// Emit deprecation warning via stderr (non-breaking)
		// In production, this can be controlled by REASONING_DEPRECATION_WARNINGS env var
		if shouldShowDeprecationWarnings() {
			fmt.Fprintf(os.Stderr, "DeprecationWarning: 'stream' parameter will be removed in version 2.0. Please use 'stream_mode' with values 'none', 'events', 'tokens', or 'both' instead.\n")
		}
		return StreamModeBoth
	}

	// 3. Check REASONING_STREAM_MODE environment variable (new granular control)
	if envMode := getEnvOrDefault("REASONING_STREAM_MODE", ""); envMode != "" {
		return ParseStreamMode(envMode)
	}

	// 4. Check REASONING_STREAM environment variable (backward compatibility)
	if envStream := strings.ToLower(getEnvOrDefault("REASONING_STREAM", "")); envStream == "true" {
		// Emit deprecation warning for environment variable
		if shouldShowDeprecationWarnings() {
			fmt.Fprintf(os.Stderr, "DeprecationWarning: 'REASONING_STREAM' environment variable will be removed in version 2.0. Please use 'REASONING_STREAM_MODE' with values 'none', 'events', 'tokens', or 'both' instead.\n")
		}
		return StreamModeBoth
	}

	return StreamModeNone
}

// shouldShowDeprecationWarnings checks if deprecation warnings should be shown
// Can be controlled by REASONING_DEPRECATION_WARNINGS environment variable (default: true)
func shouldShowDeprecationWarnings() bool {
	val := getEnvOrDefault("REASONING_DEPRECATION_WARNINGS", "true")
	return strings.ToLower(val) == "true" || val == "1"
}

// ShouldIncludeStream returns true if streaming output should be included in the result
func (sc *StreamingContext) ShouldIncludeStream() bool {
	return sc.Mode != StreamModeNone
}

// getEnvOrDefault gets an environment variable or returns a default
func getEnvOrDefault(key, defaultVal string) string {
	if val := strings.TrimSpace(getEnv(key)); val != "" {
		return val
	}
	return defaultVal
}

// getEnv is a helper to get environment variable (wrapper for testing)
func getEnv(key string) string {
	return strings.TrimSpace(envGetter(key))
}

// envGetter is the actual env getter function (can be replaced in tests)
var envGetter = os.Getenv

// resetEnvGetter restores the default environment getter function
// This should be called in test teardown to prevent test pollution
func resetEnvGetter() {
	envGetter = os.Getenv
}
