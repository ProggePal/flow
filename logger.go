package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type SessionLog struct {
	Timestamp time.Time         `json:"timestamp"`
	FlowName  string            `json:"flow_name"`
	Input     string            `json:"input"`
	Clipboard string            `json:"clipboard"`
	Config    Config            `json:"config"`
	Results   map[string]string `json:"results"`
}

func saveSessionLog(flowName, input, clipboard string, conf Config, results map[string]string) {
	log := SessionLog{
		Timestamp: time.Now(),
		FlowName:  flowName,
		Input:     input,
		Clipboard: clipboard,
		Config:    conf,
		Results:   results,
	}

	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		// Silently fail or print to stderr? TUI might be running or just finished.
		// Since this runs in the background goroutine, printing might interfere if TUI is still active,
		// but we call this after the flow is done.
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	logDir := filepath.Join(home, "fast-flows", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return
	}

	filename := fmt.Sprintf("%s_%s.json", time.Now().Format("2006-01-02_15-04-05"), flowName)
	filePath := filepath.Join(logDir, filename)

	_ = os.WriteFile(filePath, data, 0644)
}
