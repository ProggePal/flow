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
	results = make(map[string]string)
	mu      sync.Mutex
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: flow <name>")
		listFlows()
		return
	}

	flowName := os.Args[1]

	// 1. Try local ./flows folder
	path := fmt.Sprintf("./flows/%s.json", flowName)
	data, err := os.ReadFile(path)
	
	// 2. Try global ~/.flow/flows folder
	if err != nil {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".flow", "flows", flowName+".json")
		data, err = os.ReadFile(path)
	}

	if err != nil {
		fmt.Printf("❌ Flow '%s' not found.\n", flowName)
		listFlows()
		return
	}

	var conf Config
	json.Unmarshal(data, &conf)

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
	globalFiles, _ := filepath.Glob(filepath.Join(home, ".flow", "flows", "*.json"))
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

			model := conf.Model
			if s.Model != "" {
				model = s.Model
			}

			res := callGemini(model, conf.SystemPrompt, fillTags(s.Prompt))

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
	keyData, _ := os.ReadFile(filepath.Join(home, ".flow_key"))
	return strings.TrimSpace(string(keyData))
}

func depsReady(prompt string) bool {
	tags := regexp.MustCompile(`{{(.*?)}}`).FindAllStringSubmatch(prompt, -1)
	mu.Lock()
	defer mu.Unlock()
	for _, t := range tags {
		if t[1] != "clipboard" && results[t[1]] == "" {
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
	resp, _ := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var res map[string]interface{}
	json.Unmarshal(body, &res)

	return res["candidates"].([]interface{})[0].(map[string]interface{})["content"].(map[string]interface{})["parts"].([]interface{})[0].(map[string]interface{})["text"].(string)
}

func copyToClipboard(s string) {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(s)
	cmd.Run()
}
