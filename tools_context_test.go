package main

import (
	"context"
	"testing"
	"time"
)

// TestValidatePythonCodeContextCancellation verifies that validatePythonCode respects
// context cancellation and returns quickly when context is cancelled.
func TestValidatePythonCodeContextCancellation(t *testing.T) {
	// Create a context that will be cancelled soon
	ctx, cancel := context.WithCancel(context.Background())

	// Start a validation call in a goroutine
	done := make(chan error, 1)
	go func() {
		err := validatePythonCode(ctx, "print('hello')")
		done <- err
	}()

	// Cancel the context immediately
	cancel()

	// Wait for the function to return
	select {
	case err := <-done:
		// We expect either no error (if validation completes before cancel)
		// or context.Canceled (if cancel happened first)
		if err == context.Canceled {
			t.Log("validatePythonCode correctly canceled")
		} else {
			t.Log("validatePythonCode completed before cancellation:", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("validatePythonCode did not return within 5 seconds after context cancellation")
	}
}

// TestValidatePythonCodeWithContext verifies that validatePythonCode works
// correctly with a non-canceled context.
func TestValidatePythonCodeWithContext(t *testing.T) {
	ctx := context.Background()

	// Simple valid code
	err := validatePythonCode(ctx, "x = 1 + 2\nprint(x)")
	if err != nil {
		t.Errorf("valid code should not error, got: %v", err)
	}

	// Invalid code (dangerous import)
	err = validatePythonCode(ctx, "import os\nprint(os.getcwd())")
	if err == nil {
		t.Error("dangerous code should be rejected")
	}
}
