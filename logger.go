package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func saveSessionLog(flowName, input, clipboard string, conf Config, results map[string]string, histories map[string][]ChatMessage) {
	// Create a copy of config to modify
	logConf := conf
	logConf.Timestamp = time.Now()
	logConf.FlowName = flowName
	logConf.Input = input
	logConf.Clipboard = clipboard
	
	newSteps := make([]Step, len(conf.Steps))
	copy(newSteps, conf.Steps)
	logConf.Steps = newSteps

	for i := range logConf.Steps {
		stepID := logConf.Steps[i].ID
		if res, ok := results[stepID]; ok {
			logConf.Steps[i].Output = res
		}
		if hist, ok := histories[stepID]; ok {
			logConf.Steps[i].History = hist
		}
	}

	data, err := json.MarshalIndent(logConf, "", "  ")
	if err != nil {
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
