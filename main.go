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
		return
	}

	flowName := os.Args[1]
	data, err := os.ReadFile(fmt.Sprintf("./flows/%s.json", flowName))
	if err != nil {
		fmt.Printf("❌ Flow '%s' not found.\n", flowName)
		return
	}

	var conf Config
	json.Unmarshal(data, &conf)

	var wg sync.WaitGroup
	for _, step := range conf.Steps {
		wg.Add(1)
		go func(s Step) {
			defer wg.Done()
			for !depsReady(s.Prompt) {
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

	copyToClipboard(results[conf.Steps[len(conf.Steps)-1].ID])
	fmt.Printf("✅ %s finished. Result copied to clipboard.\n", flowName)
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

func callGemini(model, sys, prompt string) string {
	apiKey := getAPIKey()
	if apiKey == "" {
		fmt.Println("❌ No API Key! Run the installer or save to ~/.flow_key")
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
