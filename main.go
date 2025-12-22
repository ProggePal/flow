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

	tea "github.com/charmbracelet/bubbletea"
)

// --- Configuration & Types ---

type Step struct {
	ID, TabID, Model, Prompt string
	Type                     string `json:"type"`      // "text" (default) or "interaction"
	MaxTurns                 *int   `json:"max_turns"` // null for infinite
}

type Config struct {
	Model, SystemPrompt string
	Steps               []Step
}

// --- Main Logic ---

var (
	results   = make(map[string]string)
	userInput string
	mu        sync.Mutex
	inputChan = make(chan string) // Channel for TUI to send user input
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

	// Get clipboard content for UI
	clipboardContent := ""
	out, _ := exec.Command("pbpaste").Output()
	clipboardContent = string(out)

	// Initialize TUI
	p := tea.NewProgram(InitialModel(conf, flowName, clipboardContent, userInput))

	// Run flow in background
	go func() {
		runFlow(conf, p)
		finalResult := results[conf.Steps[len(conf.Steps)-1].ID]
		copyToClipboard(finalResult)
		saveSessionLog(flowName, userInput, clipboardContent, conf, results)
		p.Send(FlowFinishedMsg{Result: finalResult})
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
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

func runFlow(conf Config, p *tea.Program) {
	var wg sync.WaitGroup
	for _, step := range conf.Steps {
		wg.Add(1)
		go func(s Step) {
			defer wg.Done()
			for !depsReady(s.Prompt) {
				time.Sleep(100 * time.Millisecond)
			} // Automatic Parallel Detection

			if p != nil {
				p.Send(StepStartedMsg{ID: s.ID})
			} else {
				fmt.Printf("Running %s...\n", s.ID)
			}

			model := conf.Model
			if s.Model != "" {
				model = s.Model
			}

			var res string
			if s.Type == "interaction" {
				if s.Prompt != "" {
					p.Send(StepInteractionOutputMsg{ID: s.ID, Output: s.Prompt})
				}

				if s.MaxTurns != nil && *s.MaxTurns == 1 {
					p.Send(StepInteractionRequiredMsg{ID: s.ID})
					res = <-inputChan
				} else {
					history := []ChatMessage{}
					for {
						p.Send(StepInteractionRequiredMsg{ID: s.ID})
						userIn := <-inputChan
						
						if userIn == "__END_INTERACTION__" {
							break
						}

						history = append(history, ChatMessage{Role: "user", Parts: []map[string]string{{"text": userIn}}})
						
						aiRes := callGeminiChat(model, conf.SystemPrompt, history)
						p.Send(StepInteractionOutputMsg{ID: s.ID, Output: aiRes})
						
						history = append(history, ChatMessage{Role: "model", Parts: []map[string]string{{"text": aiRes}}})
					}
					
					// Serialize history to text for result
					var sb strings.Builder
					for _, msg := range history {
						role := "User"
						if msg.Role == "model" {
							role = "AI"
						}
						sb.WriteString(fmt.Sprintf("%s: %s\n", role, msg.Parts[0]["text"]))
					}
					res = sb.String()
				}
			} else {
				res = callGemini(model, conf.SystemPrompt, fillTags(s.Prompt))
			}

			if res == "" {
				err := fmt.Errorf("step '%s' failed", s.ID)
				if p != nil {
					p.Send(StepFailedMsg{ID: s.ID, Err: err})
				} else {
					fmt.Printf("❌ %v\n", err)
				}
				os.Exit(1)
			}

			mu.Lock()
			results[s.ID] = res
			mu.Unlock()

			if p != nil {
				p.Send(StepDoneMsg{ID: s.ID})
			}
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

type ChatMessage struct {
	Role  string              `json:"role"`
	Parts []map[string]string `json:"parts"`
}

var callGeminiChat = func(model, sys string, history []ChatMessage) string {
	if os.Getenv("MOCK_FLOW") == "true" {
		return "Mocked response"
	}
	apiKey := getAPIKey()
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)

	payload := map[string]interface{}{
		"contents": history,
	}
	if sys != "" {
		payload["system_instruction"] = map[string]interface{}{"parts": []map[string]string{{"text": sys}}}
	}

	jsonData, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var res map[string]interface{}
	if err := json.Unmarshal(body, &res); err != nil {
		return ""
	}

	if _, ok := res["error"]; ok {
		return ""
	}

	candidates, ok := res["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return ""
	}

	candidate := candidates[0].(map[string]interface{})
	content, ok := candidate["content"].(map[string]interface{})
	if !ok {
		return ""
	}

	return content["parts"].([]interface{})[0].(map[string]interface{})["text"].(string)
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
