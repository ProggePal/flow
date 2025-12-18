package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Step struct {
	ID, TabID, Model, Prompt string
}

type Config struct {
	Model, SystemPrompt string
	Steps               []Step
}

var (
	results   = make(map[string]string)
	userInput string
	mu        sync.Mutex
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: fast <name> [input]")
		listFlows()
		return
	}

	flowName := os.Args[1]
	if len(os.Args) > 2 {
		userInput = strings.Join(os.Args[2:], " ")
	}

	// 1. Try local ./flows folder
	path := fmt.Sprintf("./flows/%s.json", flowName)
	data, err := os.ReadFile(path)
	
	// 2. Try global ~/fast-flows/flows folder
	if err != nil {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, "fast-flows", "flows", flowName+".json")
		data, err = os.ReadFile(path)
	}

	if err != nil {
		fmt.Printf("❌ Flow '%s' not found.\n", flowName)
		listFlows()
		return
	}

	var conf Config
	if err := json.Unmarshal(data, &conf); err != nil {
		fmt.Printf("❌ Failed to parse flow configuration: %v\n", err)
		return
	}

	if len(conf.Steps) == 0 {
		fmt.Println("❌ Flow configuration has no steps.")
		return
	}

	runFlow(conf)

	copyToClipboard(results[conf.Steps[len(conf.Steps)-1].ID])
	fmt.Printf("✅ %s finished. Result copied to clipboard.\n", flowName)
}

func listFlows() {
	fmt.Println("\nAvailable flows:")
	
	// Check local
	files, _ := filepath.Glob("./flows/*.json")
	for _, f := range files {
		fmt.Printf("  - %s (local)\n", strings.TrimSuffix(filepath.Base(f), ".json"))
	}

	// Check global
	home, _ := os.UserHomeDir()
	globalFiles, _ := filepath.Glob(filepath.Join(home, "fast-flows", "flows", "*.json"))
	for _, f := range globalFiles {
		fmt.Printf("  - %s\n", strings.TrimSuffix(filepath.Base(f), ".json"))
	}
	fmt.Println()
}

func runFlow(conf Config) {
	var wg sync.WaitGroup
	for _, step := range conf.Steps {
		wg.Add(1)
		go func(s Step) {
			defer wg.Done()
			for !depsReady(s.Prompt) {
				time.Sleep(100 * time.Millisecond)
			} // Automatic Parallel Detection

			// Feedback
			tags := regexp.MustCompile(`{{(.*?)}}`).FindAllStringSubmatch(s.Prompt, -1)
			var sources []string
			for _, t := range tags {
				sources = append(sources, t[1])
			}
			sourceStr := strings.Join(sources, " + ")
			if len(sources) == 0 {
				sourceStr = "Start"
			}
			fmt.Printf("  %s -> %s ...\n", sourceStr, s.ID)

			model := conf.Model
			if s.Model != "" {
				model = s.Model
			}

			res := callGemini(model, conf.SystemPrompt, fillTags(s.Prompt))
			if res == "" {
				fmt.Printf("❌ Step '%s' failed. Stopping flow.\n", s.ID)
				os.Exit(1)
			}

			mu.Lock()
			results[s.ID] = res
			mu.Unlock()
		}(step)
	}
	wg.Wait()
}

func getAPIKey() string {
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		return key
	}
	home, _ := os.UserHomeDir()
	keyData, _ := os.ReadFile(filepath.Join(home, ".fast_key"))
	return strings.TrimSpace(string(keyData))
}

func depsReady(prompt string) bool {
	tags := regexp.MustCompile(`{{(.*?)}}`).FindAllStringSubmatch(prompt, -1)
	mu.Lock()
	defer mu.Unlock()
	for _, t := range tags {
		if t[1] != "clipboard" && t[1] != "input" && results[t[1]] == "" {
			return false
		}
	}
	return true
}

func fillTags(prompt string) string {
	mu.Lock()
	defer mu.Unlock()
	res := prompt
	if strings.Contains(res, "{{clipboard}}") {
		out, _ := exec.Command("pbpaste").Output()
		res = strings.ReplaceAll(res, "{{clipboard}}", string(out))
	}
	if strings.Contains(res, "{{input}}") {
		res = strings.ReplaceAll(res, "{{input}}", userInput)
	}
	for k, v := range results {
		res = strings.ReplaceAll(res, "{{"+k+"}}", v)
	}
	return res
}

var callGemini = func(model, sys, prompt string) string {
	if os.Getenv("MOCK_FLOW") == "true" {
		return "Mocked response for: " + prompt
	}
	apiKey := getAPIKey()
	if apiKey == "" {
		fmt.Println("❌ No API Key found!")
		home, _ := os.UserHomeDir()
		keyPath := filepath.Join(home, ".fast_key")
		fmt.Printf("   (Checked environment variable GEMINI_API_KEY and file: %s)\n", keyPath)
		fmt.Println("👉 Please run the installer again to set up your key.")
		os.Exit(1)
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{{"parts": []map[string]string{{"text": prompt}}}},
	}
	if sys != "" {
		payload["system_instruction"] = map[string]interface{}{"parts": []map[string]string{{"text": sys}}}
	}

	jsonData, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("❌ Network error: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var res map[string]interface{}
	if err := json.Unmarshal(body, &res); err != nil {
		fmt.Printf("❌ Failed to parse API response: %v\nBody: %s\n", err, string(body))
		return ""
	}

	if errVal, ok := res["error"]; ok {
		fmt.Printf("❌ API Error: %v\n", errVal)
		return ""
	}

	candidates, ok := res["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		// Check if it was blocked due to safety
		if promptFeedback, ok := res["promptFeedback"]; ok {
			fmt.Printf("❌ Prompt blocked. Feedback: %v\n", promptFeedback)
		} else {
			fmt.Printf("❌ No candidates returned. Response: %s\n", string(body))
		}
		return ""
	}

	candidate := candidates[0].(map[string]interface{})
	content, ok := candidate["content"].(map[string]interface{})
	if !ok {
		if finishReason, ok := candidate["finishReason"]; ok {
			fmt.Printf("❌ Generation stopped. Reason: %v\n", finishReason)
		} else {
			fmt.Printf("❌ Unexpected response structure: %s\n", string(body))
		}
		return ""
	}

	return content["parts"].([]interface{})[0].(map[string]interface{})["text"].(string)
}

func copyToClipboard(s string) {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(s)
	cmd.Run()
}
