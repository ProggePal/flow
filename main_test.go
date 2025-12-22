package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunFlow(t *testing.T) {
	// Mock callGemini
	originalCallGemini := callGemini
	defer func() { callGemini = originalCallGemini }()

	callGemini = func(model, sys, prompt string) string {
		return "Mocked response for: " + prompt
	}

	// Reset results
	results = make(map[string]string)

	conf := Config{
		Model: "test-model",
		Steps: []Step{
			{ID: "step1", Prompt: "Hello"},
			{ID: "step2", Prompt: "Previous was {{step1}}"},
		},
	}

	runFlow(conf, nil)

	if results["step1"] != "Mocked response for: Hello" {
		t.Errorf("Expected step1 result, got %s", results["step1"])
	}

	expectedStep2 := "Mocked response for: Previous was Mocked response for: Hello"
	if results["step2"] != expectedStep2 {
		t.Errorf("Expected step2 result %s, got %s", expectedStep2, results["step2"])
	}
}

func TestSaveSessionLog(t *testing.T) {
	// Create a temp directory to act as HOME
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Dummy data
	flowName := "test_flow"
	input := "test input"
	clipboard := "test clipboard"
	conf := Config{
		Model: "test-model",
		Steps: []Step{{ID: "step1", Prompt: "hi"}},
	}
	results := map[string]string{"step1": "result1"}

	// Run the function
	saveSessionLog(flowName, input, clipboard, conf, results)

	// Verify file creation
	logDir := filepath.Join(tempHome, "fast-flows", "logs")
	files, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("Failed to read log dir: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 log file, got %d", len(files))
	}

	// Verify content
	content, err := os.ReadFile(filepath.Join(logDir, files[0].Name()))
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	var log SessionLog
	if err := json.Unmarshal(content, &log); err != nil {
		t.Fatalf("Failed to unmarshal log: %v", err)
	}

	if log.FlowName != flowName {
		t.Errorf("Expected FlowName %s, got %s", flowName, log.FlowName)
	}
	if log.Results["step1"] != "result1" {
		t.Errorf("Expected result1, got %s", log.Results["step1"])
	}
}
