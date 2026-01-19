package main

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestValidateToolNames(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)

	availableTools := []string{"calculator", "code_exec", "web_fetch", "string_ops"}

	tests := []struct {
		name           string
		toolList       []string
		expectedValid   []string
		expectedLogged  string
	}{
		{
			name:          "All valid tools",
			toolList:      []string{"calculator", "web_fetch"},
			expectedValid:  []string{"calculator", "web_fetch"},
			expectedLogged: "",
		},
		{
			name:          "All invalid tools",
			toolList:      []string{"invalid1", "invalid2"},
			expectedValid:  []string{},
			expectedLogged: "ignoring invalid tool name(s)",
		},
		{
			name:          "Mixed valid and invalid",
			toolList:      []string{"calculator", "invalid", "web_fetch", "unknown"},
			expectedValid:  []string{"calculator", "web_fetch"},
			expectedLogged: "ignoring invalid tool name(s)",
		},
		{
			name:          "Empty tool names",
			toolList:      []string{"", "calculator"},
			expectedValid:  []string{"calculator"},
			expectedLogged: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			result := validateToolNames(tt.toolList, availableTools)

			// Check valid tools
			if len(result) != len(tt.expectedValid) {
				t.Errorf("Expected %d valid tools, got %d", len(tt.expectedValid), len(result))
			}
			for i, tool := range tt.expectedValid {
				if i >= len(result) || result[i] != tool {
					t.Errorf("Expected tool %d to be %q, got %q", i, tool, result[i])
				}
			}

			// Check log output
			logOutput := buf.String()
			if tt.expectedLogged != "" && !strings.Contains(logOutput, tt.expectedLogged) {
				t.Errorf("Expected log to contain %q, got %q", tt.expectedLogged, logOutput)
			}
			if tt.expectedLogged == "" && strings.Contains(logOutput, "invalid tool name") {
				t.Errorf("Expected no warning log, got %q", logOutput)
			}
		})
	}
}

func TestGetAvailableToolNames(t *testing.T) {
	toolNames := getAvailableToolNames()

	// Check that we get the expected number of tools
	if len(toolNames) < 4 {
		t.Errorf("Expected at least 4 tools, got %d", len(toolNames))
	}

	// Check for expected tool names
	expectedTools := []string{"calculator", "code_exec", "web_fetch", "string_ops"}
	for _, expected := range expectedTools {
		found := false
		for _, name := range toolNames {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected tool %q to be available", expected)
		}
	}
}
