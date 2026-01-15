package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
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
			sb.WriteString(fmt.Sprintf("%s [%s] Eval (%.2f): %s\n", icon, elapsed, event.Score, truncateStr(event.Content, 80)))
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
			sb.WriteString(fmt.Sprintf("%s [%s] %s%s%s%s\n", icon, elapsed, nodeInfo, depthInfo, scoreInfo, truncateStr(event.Content, 80)))
		}
	}

	return sb.String()
}

// FormatAsJSON formats events as JSON
func (sm *StreamingManager) FormatAsJSON() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	data, _ := json.MarshalIndent(map[string]interface{}{
		"tool":        sm.toolName,
		"start_time":  sm.startTime.Format(time.RFC3339),
		"total_events": len(sm.buffer),
		"events":      sm.buffer,
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
		line := fmt.Sprintf("%s %s", icon, truncateStr(event.Content, 60))
		if event.Score > 0 {
			line = fmt.Sprintf("%s (%.2f)", line, event.Score)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// StreamingSummary provides a summary of the streaming session
type StreamingSummary struct {
	TotalEvents     int           `json:"total_events"`
	TotalDuration   time.Duration `json:"total_duration_ms"`
	EventsByType    map[string]int `json:"events_by_type"`
	SolutionFound   bool          `json:"solution_found"`
	FinalAnswer     string        `json:"final_answer,omitempty"`
	MaxDepthReached int           `json:"max_depth_reached"`
	MaxNodesExplored int          `json:"max_nodes_explored"`
}

// GetSummary returns a summary of the streaming session
func (sm *StreamingManager) GetSummary() StreamingSummary {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	summary := StreamingSummary{
		TotalEvents:  len(sm.buffer),
		TotalDuration: time.Since(sm.startTime),
		EventsByType: make(map[string]int),
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
		parts = append(parts, truncateStr(update.Thought, 80))
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
