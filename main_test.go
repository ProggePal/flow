package main

import (
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

	runFlow(conf)

	if results["step1"] != "Mocked response for: Hello" {
		t.Errorf("Expected step1 result, got %s", results["step1"])
	}

	expectedStep2 := "Mocked response for: Previous was Mocked response for: Hello"
	if results["step2"] != expectedStep2 {
		t.Errorf("Expected step2 result %s, got %s", expectedStep2, results["step2"])
	}
}
