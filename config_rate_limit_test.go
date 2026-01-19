package main

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

func TestLLMRateLimiting_DefaultConfig(t *testing.T) {
	ResetConfig()
	cfg := GetConfig()

	if cfg.MaxConcurrentLLMRequests != defaultMaxConcurrentLLMRequests {
		t.Errorf("Default MaxConcurrentLLMRequests should be %d, got %d",
			defaultMaxConcurrentLLMRequests, cfg.MaxConcurrentLLMRequests)
	}
}

func TestLLMRateLimiting_LoadFromEnv(t *testing.T) {
	ResetConfig()
	defer ResetConfig()

	tests := []struct {
		name     string
		envValue string
		expected int
	}{
		{
			name:     "valid value",
			envValue: "5",
			expected: 5,
		},
		{
			name:     "unlimited (zero)",
			envValue: "0",
			expected: 0,
		},
		{
			name:     "clamped above maximum",
			envValue: "100",
			expected: maxConcurrentLLMRequests,
		},
		{
			name:     "negative treated as unlimited",
			envValue: "-1",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("LLM_MAX_CONCURRENT", tt.envValue)
			defer os.Unsetenv("LLM_MAX_CONCURRENT")

			cfg := LoadConfig()
			if cfg.MaxConcurrentLLMRequests != tt.expected {
				t.Errorf("Expected MaxConcurrentLLMRequests=%d, got %d",
					tt.expected, cfg.MaxConcurrentLLMRequests)
			}
		})
	}
}

func TestAcquireLLMSlot_Unlimited(t *testing.T) {
	ResetConfig()

	// Set unlimited concurrent requests
	os.Setenv("LLM_MAX_CONCURRENT", "0")
	defer os.Unsetenv("LLM_MAX_CONCURRENT")
	LoadConfig()

	ctx := context.Background()
	release, err := AcquireLLMSlot(ctx)

	if err != nil {
		t.Errorf("AcquireLLMSlot should not error with unlimited requests: %v", err)
	}

	if release == nil {
		t.Error("AcquireLLMSlot should return a release function")
	}

	// Should be able to acquire multiple slots without blocking
	for i := 0; i < 10; i++ {
		r, err := AcquireLLMSlot(ctx)
		if err != nil {
			t.Errorf("Concurrent acquisition %d failed: %v", i, err)
		}
		if r != nil {
			r()
		}
	}

	// Clean up
	if release != nil {
		release()
	}
}

func TestAcquireLLMSlot_Limited(t *testing.T) {
	ResetConfig()

	// Set limit to 2 concurrent requests
	os.Setenv("LLM_MAX_CONCURRENT", "2")
	defer os.Unsetenv("LLM_MAX_CONCURRENT")
	LoadConfig()

	ctx := context.Background()

	// Acquire 2 slots successfully
	var releases []func()
	for i := 0; i < 2; i++ {
		release, err := AcquireLLMSlot(ctx)
		if err != nil {
			t.Errorf("Acquisition %d should succeed: %v", i, err)
		}
		if release == nil {
			t.Error("AcquireLLMSlot should return a release function")
		}
		releases = append(releases, release)
	}

	// Third acquisition should block
	blocking := make(chan struct{})
	go func() {
		release, err := AcquireLLMSlot(ctx)
		if err != nil {
			t.Errorf("Blocked acquisition should not error: %v", err)
		}
		if release == nil {
			t.Error("Blocked acquisition should return a release function")
		}
		close(blocking)
	}()

	// Verify it's blocked
	select {
	case <-blocking:
		t.Error("Third acquisition should block but didn't")
	case <-time.After(100 * time.Millisecond):
		// Expected - acquisition is blocked
	}

	// Release one slot
	if len(releases) > 0 {
		releases[0]()
		releases = releases[1:]
	}

	// Third acquisition should now succeed
	select {
	case <-blocking:
		// Success - acquisition completed
	case <-time.After(1 * time.Second):
		t.Error("Third acquisition should complete after release")
	}

	// Clean up
	for _, r := range releases {
		if r != nil {
			r()
		}
	}
}

func TestAcquireLLMSlot_ContextCancellation(t *testing.T) {
	ResetConfig()

	// Set limit to 1 concurrent request
	os.Setenv("LLM_MAX_CONCURRENT", "1")
	defer os.Unsetenv("LLM_MAX_CONCURRENT")
	LoadConfig()

	ctx, cancel := context.WithCancel(context.Background())

	// Acquire first slot
	release, err := AcquireLLMSlot(ctx)
	if err != nil {
		t.Fatalf("First acquisition should succeed: %v", err)
	}

	// Cancel context and try to acquire second slot
	cancel()
	_, err = AcquireLLMSlot(ctx)

	if err != context.Canceled {
		t.Errorf("AcquireLLMSlot should return context.Canceled, got: %v", err)
	}

	// Clean up
	if release != nil {
		release()
	}
}

func TestAcquireLLMSlot_ContextTimeout(t *testing.T) {
	ResetConfig()

	// Set limit to 1 concurrent request
	os.Setenv("LLM_MAX_CONCURRENT", "1")
	defer os.Unsetenv("LLM_MAX_CONCURRENT")
	LoadConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Acquire first slot
	release, err := AcquireLLMSlot(ctx)
	if err != nil {
		t.Fatalf("First acquisition should succeed: %v", err)
	}

	// Try to acquire second slot (should timeout)
	_, err = AcquireLLMSlot(ctx)

	if err != context.DeadlineExceeded {
		t.Errorf("AcquireLLMSlot should return context.DeadlineExceeded, got: %v", err)
	}

	// Clean up
	if release != nil {
		release()
	}
}

func TestAcquireLLMSlot_ReleaseFunctionality(t *testing.T) {
	ResetConfig()

	// Set limit to 1 concurrent request
	os.Setenv("LLM_MAX_CONCURRENT", "1")
	defer os.Unsetenv("LLM_MAX_CONCURRENT")
	LoadConfig()

	ctx := context.Background()

	// Acquire and immediately release
	release, err := AcquireLLMSlot(ctx)
	if err != nil {
		t.Fatalf("Acquisition should succeed: %v", err)
	}

	if release != nil {
		release()
	}

	// Should be able to acquire again
	release, err = AcquireLLMSlot(ctx)
	if err != nil {
		t.Errorf("Acquisition after release should succeed: %v", err)
	}

	if release != nil {
		release()
	}
}

func TestAcquireLLMSlot_ConcurrentStress(t *testing.T) {
	ResetConfig()

	// Set limit to 3 concurrent requests
	os.Setenv("LLM_MAX_CONCURRENT", "3")
	defer os.Unsetenv("LLM_MAX_CONCURRENT")
	LoadConfig()

	ctx := context.Background()

	// Try to acquire many slots concurrently
	const totalAttempts = 20

	var wg sync.WaitGroup

	for i := 0; i < totalAttempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			release, err := AcquireLLMSlot(ctx)
			if err != nil {
				return // Context cancelled or timed out
			}

			// Hold for a short time
			time.Sleep(10 * time.Millisecond)

			// Release
			if release != nil {
				release()
			}
		}()
	}

	wg.Wait()
	t.Log("Concurrent stress test completed successfully")
}

func TestResetConfig_ClearsSemaphore(t *testing.T) {
	ResetConfig()

	// Set limit to 1 concurrent request
	os.Setenv("LLM_MAX_CONCURRENT", "1")
	defer os.Unsetenv("LLM_MAX_CONCURRENT")
	LoadConfig()

	ctx := context.Background()

	// Acquire slot
	release, err := AcquireLLMSlot(ctx)
	if err != nil {
		t.Fatalf("Acquisition should succeed: %v", err)
	}

	// Reset config
	ResetConfig()

	// Clean up
	if release != nil {
		release()
	}

	// Should be able to acquire with new default settings
	_, err = AcquireLLMSlot(ctx)
	if err != nil {
		t.Errorf("Acquisition after reset should succeed: %v", err)
	}
}

func TestFIFOLimiter_Ordering(t *testing.T) {
	ResetConfig()

	// Set limit to 1 concurrent request to enforce strict ordering
	os.Setenv("LLM_MAX_CONCURRENT", "1")
	defer os.Unsetenv("LLM_MAX_CONCURRENT")
	LoadConfig()

	ctx := context.Background()
	const numRequests = 10

	// Track the order requests complete
	var (
		completionOrder []int
		orderMu         sync.Mutex
		wg              sync.WaitGroup
		startGate       = make(chan struct{}) // ensures all goroutines start together
	)

	// Launch requests in order 0, 1, 2, ...
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Wait for start signal
			<-startGate

			// Small stagger to ensure enqueue order matches id order
			time.Sleep(time.Duration(id) * time.Millisecond)

			release, err := AcquireLLMSlot(ctx)
			if err != nil {
				t.Errorf("Request %d failed to acquire: %v", id, err)
				return
			}

			// Record completion order
			orderMu.Lock()
			completionOrder = append(completionOrder, id)
			orderMu.Unlock()

			// Simulate work
			time.Sleep(5 * time.Millisecond)
			release()
		}(i)
	}

	// Start all goroutines
	close(startGate)
	wg.Wait()

	// Verify FIFO ordering
	orderMu.Lock()
	defer orderMu.Unlock()

	if len(completionOrder) != numRequests {
		t.Fatalf("Expected %d completions, got %d", numRequests, len(completionOrder))
	}

	for i := 0; i < numRequests; i++ {
		if completionOrder[i] != i {
			t.Errorf("FIFO violation: expected request %d at position %d, got %d", i, i, completionOrder[i])
			t.Logf("Full order: %v", completionOrder)
			break
		}
	}
	t.Logf("FIFO order verified: %v", completionOrder)
}
