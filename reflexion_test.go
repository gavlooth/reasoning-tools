package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadOrCreateMemory_InvalidJSON tests that invalid memory files
// are handled gracefully without data loss
func TestLoadOrCreateMemory_InvalidJSON(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()
	memoryPath := filepath.Join(tmpDir, "memory.json")

	// Write invalid JSON to simulate a corrupted file
	invalidData := []byte(`{"episodes": [{"id": "test", "problem": "test", "was_successful": true, INVALID_JSON_HERE}]`)
	if err := os.WriteFile(memoryPath, invalidData, 0644); err != nil {
		t.Fatalf("Failed to write invalid memory file: %v", err)
	}

	// Load the memory - should handle invalid JSON gracefully
	memory := loadOrCreateMemory(memoryPath)

	// Verify memory is initialized with empty episodes
	if len(memory.Episodes) != 0 {
		t.Errorf("Expected empty episodes for invalid JSON file, got %d episodes", len(memory.Episodes))
	}

	// Verify path is still set so we can save new data
	if memory.path != memoryPath {
		t.Errorf("Expected path to be %q, got %q", memoryPath, memory.path)
	}

	// Verify we can add episodes and save
	memory.Episodes = append(memory.Episodes, Episode{
		ID:            "test1",
		Problem:       "Test problem",
		ProblemHash:   "abc123",
		Attempt:       1,
		Thoughts:      []string{"thought1"},
		FinalAnswer:   "answer",
		WasSuccessful: true,
		Timestamp:     time.Now(),
		Provider:      "test",
	})

	memory.save()

	// Verify the file now contains valid JSON
	data, err := os.ReadFile(memoryPath)
	if err != nil {
		t.Fatalf("Failed to read memory file: %v", err)
	}

	// Should be parseable now
	var loaded EpisodicMemory
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Errorf("After saving, memory file should contain valid JSON, got error: %v", err)
	}

	// The original corrupted content should be replaced with valid data
	if len(loaded.Episodes) != 1 {
		t.Errorf("Expected 1 episode after save, got %d", len(loaded.Episodes))
	}
}

// TestLoadOrCreateMemory_DirectoryExists tests loading when directory already exists
func TestLoadOrCreateMemory_DirectoryExists(t *testing.T) {
	tmpDir := t.TempDir()
	memoryPath := filepath.Join(tmpDir, "subdir", "memory.json")

	// Create the directory first
	if err := os.MkdirAll(filepath.Dir(memoryPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Create valid memory file
	episode := Episode{
		ID:            "existing",
		Problem:       "Existing problem",
		ProblemHash:   "hash123",
		Attempt:       1,
		Thoughts:      []string{"existing thought"},
		FinalAnswer:   "existing answer",
		WasSuccessful: true,
		Timestamp:     time.Now(),
		Provider:      "test",
	}
	existingMemory := &EpisodicMemory{
		Episodes: []Episode{episode},
		path:     memoryPath,
	}

	// Save the existing memory
	existingMemory.save()

	// Now load it
	loaded := loadOrCreateMemory(memoryPath)

	// Verify the existing episode was loaded
	if len(loaded.Episodes) != 1 {
		t.Errorf("Expected 1 existing episode, got %d", len(loaded.Episodes))
	}

	if len(loaded.Episodes) > 0 && loaded.Episodes[0].ID != "existing" {
		t.Errorf("Expected episode ID 'existing', got %q", loaded.Episodes[0].ID)
	}
}

// TestLoadOrCreateMemory_NewFile tests creating new memory when file doesn't exist
func TestLoadOrCreateMemory_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	memoryPath := filepath.Join(tmpDir, "memory.json")

	// Load memory that doesn't exist yet
	memory := loadOrCreateMemory(memoryPath)

	// Should have empty episodes
	if len(memory.Episodes) != 0 {
		t.Errorf("Expected empty episodes for new file, got %d", len(memory.Episodes))
	}

	// Path should be set correctly
	if memory.path != memoryPath {
		t.Errorf("Expected path %q, got %q", memoryPath, memory.path)
	}
}

// TestLoadOrCreateMemory_InvalidJSON_CorruptedFile tests data loss prevention
// when an existing valid memory file gets corrupted
func TestLoadOrCreateMemory_InvalidJSON_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	memoryPath := filepath.Join(tmpDir, "memory.json")

	// First, create a valid memory file with important data
	validMemory := &EpisodicMemory{
		Episodes: []Episode{
			{
				ID:            "important1",
				Problem:       "Important problem 1",
				ProblemHash:   "hash1",
				Attempt:       1,
				Thoughts:      []string{"thought1"},
				FinalAnswer:   "answer1",
				WasSuccessful: true,
				Reflection:    "Important reflection 1",
				Timestamp:     time.Now(),
				Provider:      "test",
			},
			{
				ID:            "important2",
				Problem:       "Important problem 2",
				ProblemHash:   "hash2",
				Attempt:       2,
				Thoughts:      []string{"thought2"},
				FinalAnswer:   "answer2",
				WasSuccessful: false,
				Reflection:    "Important reflection 2 - what went wrong",
				Timestamp:     time.Now(),
				Provider:      "test",
			},
		},
		path: memoryPath,
	}

	// Save the valid memory
	validMemory.save()

	// Verify file exists and has content
	data, err := os.ReadFile(memoryPath)
	if err != nil {
		t.Fatalf("Failed to read memory file: %v", err)
	}
	_ = len(data) // Original file size for reference

	// Now corrupt the file by writing invalid JSON over it
	corruptedData := []byte(`{"episodes": [{"id": "test", invalid json here}]`)
	if err := os.WriteFile(memoryPath, corruptedData, 0644); err != nil {
		t.Fatalf("Failed to corrupt memory file: %v", err)
	}

	// Load the corrupted memory
	loaded := loadOrCreateMemory(memoryPath)

	// FIX: The new implementation should create a backup of the corrupted file
	// Find the backup file
	backupFiles, err := filepath.Glob(memoryPath + ".corrupted.*")
	if err != nil {
		t.Fatalf("Failed to search for backup files: %v", err)
	}

	if len(backupFiles) == 0 {
		t.Error("FIX NOT WORKING: No backup file was created for the corrupted memory!")
	} else {
		t.Logf("FIX VERIFIED: Backup file created: %s", backupFiles[0])

		// Verify the backup contains the original corrupted data
		backupData, err := os.ReadFile(backupFiles[0])
		if err != nil {
			t.Fatalf("Failed to read backup file: %v", err)
		}

		// The backup should contain the corrupted data we wrote
		if string(backupData) != string(corruptedData) {
			t.Errorf("Backup file doesn't match the corrupted data")
		}
	}

	// The loaded memory should have empty episodes
	if len(loaded.Episodes) != 0 {
		t.Errorf("Expected empty episodes after loading corrupted file, got %d", len(loaded.Episodes))
	}

	// Path should still be set so we can save new data
	if loaded.path != memoryPath {
		t.Errorf("Expected path %q, got %q", memoryPath, loaded.path)
	}

	// Add a new episode and save
	loaded.Episodes = append(loaded.Episodes, Episode{
		ID:            "new1",
		Problem:       "New problem",
		ProblemHash:   "newhash",
		Attempt:       1,
		Thoughts:      []string{"new thought"},
		FinalAnswer:   "new answer",
		WasSuccessful: true,
		Timestamp:     time.Now(),
		Provider:      "test",
	})
	loaded.save()

	// Verify the main file was updated with new data
	newData, err := os.ReadFile(memoryPath)
	if err != nil {
		t.Fatalf("Failed to read memory file after save: %v", err)
	}

	// Should contain the new episode
	var loadedMemory EpisodicMemory
	if err := json.Unmarshal(newData, &loadedMemory); err != nil {
		t.Fatalf("Failed to parse new memory file: %v", err)
	}

	if len(loadedMemory.Episodes) != 1 {
		t.Errorf("Expected 1 episode in new file, got %d", len(loadedMemory.Episodes))
	}

	if loadedMemory.Episodes[0].ID != "new1" {
		t.Errorf("Expected episode ID 'new1', got %q", loadedMemory.Episodes[0].ID)
	}

	// Most importantly: verify the backup file still exists and wasn't deleted
	backupFiles, err = filepath.Glob(memoryPath + ".corrupted.*")
	if err != nil {
		t.Fatalf("Failed to search for backup files: %v", err)
	}

	if len(backupFiles) == 0 {
		t.Error("DATA LOSS: Backup file was deleted! The original corrupted data is lost forever.")
	} else {
		t.Logf("FIX VERIFIED: Backup file preserved after save: %s", backupFiles[0])
		t.Logf("FIX SUCCESS: Original corrupted data (corrupted file with %d bytes) is recoverable from backup", len(corruptedData))
	}
}
