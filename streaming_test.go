package main

import (
	"sync"
	"testing"
	"time"
)

func TestStreamingManager_Clear(t *testing.T) {
	sm := NewStreamingManager("test_tool")

	// Add some events
	sm.AddEvent("thought", "Test thought 1")
	sm.AddEvent("thought", "Test thought 2")
	sm.AddEvent("evaluation", "Test evaluation")

	// Verify events were added
	events := sm.GetEvents()
	if len(events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(events))
	}

	// Clear the buffer
	sm.Clear()

	// Verify buffer is empty
	events = sm.GetEvents()
	if len(events) != 0 {
		t.Errorf("Expected 0 events after Clear, got %d", len(events))
	}
}

func TestStreamingManager_ClearConcurrency(t *testing.T) {
	sm := NewStreamingManager("test_tool")

	// Add initial events
	for i := 0; i < 100; i++ {
		sm.AddEvent("thought", "Test thought")
	}

	// Verify events were added
	events := sm.GetEvents()
	if len(events) != 100 {
		t.Fatalf("Expected 100 events, got %d", len(events))
	}

	// Test concurrent clear and add
	var wg sync.WaitGroup
	done := make(chan struct{})

	// Goroutine 1: Keep adding events
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			select {
			case <-done:
				return
			default:
				sm.AddEvent("thought", "Concurrent thought")
				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	// Goroutine 2: Clear buffer
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		sm.Clear()
		close(done)
	}()

	wg.Wait()

	// Verify buffer was cleared (may have some events added after clear)
	eventsAfter := sm.GetEvents()
	if len(eventsAfter) > 50 {
		t.Errorf("Expected at most 50 events after concurrent clear, got %d", len(eventsAfter))
	}
}

func TestStreamingManager_ClearAndReuse(t *testing.T) {
	sm := NewStreamingManager("test_tool")

	// First session
	sm.AddEvent("thought", "Session 1 thought")
	sm.AddEvent("solution", "Session 1 solution")

	if len(sm.GetEvents()) != 2 {
		t.Fatalf("Expected 2 events in first session, got %d", len(sm.GetEvents()))
	}

	// Clear for reuse
	sm.Clear()

	// Second session
	sm.AddEvent("thought", "Session 2 thought")
	sm.AddEvent("evaluation", "Session 2 evaluation")
	sm.AddEvent("solution", "Session 2 solution")

	if len(sm.GetEvents()) != 3 {
		t.Fatalf("Expected 3 events in second session, got %d", len(sm.GetEvents()))
	}

	// Verify content is from second session only
	events := sm.GetEvents()
	if events[0].Content != "Session 2 thought" {
		t.Errorf("Expected 'Session 2 thought', got '%s'", events[0].Content)
	}
}

func TestStreamingManager_ClearEmpty(t *testing.T) {
	sm := NewStreamingManager("test_tool")

	// Clear an empty buffer (should not panic)
	sm.Clear()

	if len(sm.GetEvents()) != 0 {
		t.Errorf("Expected 0 events after clearing empty buffer, got %d", len(sm.GetEvents()))
	}
}

func TestStreamingManager_ClearMultipleTimes(t *testing.T) {
	sm := NewStreamingManager("test_tool")

	// Add events
	sm.AddEvent("thought", "Test thought")

	// Clear multiple times (should not panic)
	sm.Clear()
	sm.Clear()
	sm.Clear()

	if len(sm.GetEvents()) != 0 {
		t.Errorf("Expected 0 events after multiple clears, got %d", len(sm.GetEvents()))
	}
}

func TestEnvGetter_ResetFunctionality(t *testing.T) {
	// Save original function
	originalGetter := envGetter

	// Mock envGetter to return a fixed value
	envGetter = func(key string) string {
		return "mocked_value"
	}

	// Verify mock is working
	result := getEnv("ANY_KEY")
	if result != "mocked_value" {
		t.Errorf("Expected 'mocked_value', got '%s'", result)
	}

	// Reset using the new function
	resetEnvGetter()

	// Verify it's restored to os.Getenv behavior
	// (REAL_ENV_TEST_* keys should not exist, so should return empty string)
	result = getEnv("REAL_ENV_TEST_NONEXISTENT_KEY")
	if result != "" {
		t.Errorf("Expected empty string for non-existent env var after reset, got '%s'", result)
	}

	// Also verify by checking envGetter points to os.Getenv by setting a real env var
	t.Setenv("REAL_ENV_TEST_KEY", "test_value")
	result = getEnv("REAL_ENV_TEST_KEY")
	if result != "test_value" {
		t.Errorf("Expected 'test_value' from real env var after reset, got '%s'", result)
	}

	// Restore original getter to prevent affecting other tests
	envGetter = originalGetter
}

func TestEnvGetter_IsolationBetweenTests(t *testing.T) {
	// This test verifies that resetEnvGetter allows tests to be isolated
	// First, ensure we start with a clean state
	resetEnvGetter()

	// Simulate Test 1: Mock envGetter
	envGetter = func(key string) string {
		if key == "TEST1_VAR" {
			return "test1_value"
		}
		return ""
	}

	result1 := getEnv("TEST1_VAR")
	if result1 != "test1_value" {
		t.Errorf("Test 1 failed: expected 'test1_value', got '%s'", result1)
	}

	// Simulate test cleanup - reset for next test
	resetEnvGetter()

	// Simulate Test 2: Should NOT see Test 1's mock
	// Using t.Setenv to verify we're using real os.Getenv
	t.Setenv("TEST2_VAR", "test2_value")

	result2 := getEnv("TEST2_VAR")
	if result2 != "test2_value" {
		t.Errorf("Test 2 failed: expected 'test2_value' from real env, got '%s'", result2)
	}

	// Verify Test 1's mock is gone
	result1After := getEnv("TEST1_VAR")
	if result1After != "" {
		t.Errorf("Test 2 isolation failed: Test 1's mock still active, got '%s'", result1After)
	}

	resetEnvGetter()
}
