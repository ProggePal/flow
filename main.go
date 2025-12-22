package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/genai"
)

// --- Configuration & Types ---

type Step struct {
	ID, TabID, Model, Prompt string
	Type                     string        `json:"type"`               // "text", "interaction", "selector", "file_write"
	MaxTurns                 *int          `json:"max_turns"`          // for interaction
	Source                   string        `json:"source"`             // for selector
	Filename                 string        `json:"filename"`           // for file_write
	Content                  string        `json:"content"`            // for file_write
	If                       string        `json:"if"`                 // conditional execution
	Output                   string        `json:"output,omitempty"`   // Result of the step
	History                  []ChatMessage `json:"history,omitempty"`  // Chat history for interaction steps
}

type Config struct {
	Model, SystemPrompt string
	Steps               []Step
	Timestamp           time.Time       `json:"timestamp,omitempty"`
	FlowName            string          `json:"flow_name,omitempty"`
	Input               string          `json:"input,omitempty"`
	Clipboard           string          `json:"clipboard,omitempty"`
	MCP                 json.RawMessage `json:"mcp,omitempty"`
}

// --- Main Logic ---

var (
	results    = make(map[string]string)
	histories  = make(map[string][]ChatMessage)
	userInput  string
	mu         sync.Mutex
	inputChan  = make(chan string) // Channel for TUI to send user input
	mcpManager *MCPManager
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
	// Try to clean up markdown code blocks if present
	cleanData := []byte(cleanMarkdown(string(data)))

	if err := json.Unmarshal(cleanData, &conf); err != nil {
		fmt.Printf("❌ Failed to parse flow configuration: %v\n", err)
		return
	}

	if len(conf.Steps) == 0 {
		fmt.Println("❌ Flow configuration has no steps.")
		return
	}

	// Initialize MCP Manager
	mcpManager = NewMCPManager()
	defer mcpManager.Close()
	ctx := context.Background()

	// Parse MCP config
	var allowedServers map[string]bool
	if len(conf.MCP) > 0 {
		var mcpStr string
		if err := json.Unmarshal(conf.MCP, &mcpStr); err == nil {
			if mcpStr == "none" {
				allowedServers = make(map[string]bool) // Empty map = allow none
			} else if mcpStr == "all" {
				allowedServers = nil // Nil map = allow all
			}
		} else {
			var mcpList []string
			if err := json.Unmarshal(conf.MCP, &mcpList); err == nil {
				allowedServers = make(map[string]bool)
				for _, s := range mcpList {
					allowedServers[s] = true
				}
			}
		}
	}

	// Load MCPs from local ./mcp folder
	if err := mcpManager.LoadFromDirectory(ctx, "./mcp", allowedServers); err != nil {
		// fmt.Printf("⚠️ Failed to load local MCPs: %v\n", err)
	}

	// Load MCPs from global ~/fast-flows/mcp folder
	home, _ := os.UserHomeDir()
	globalMcpPath := filepath.Join(home, "fast-flows", "mcp")
	if err := mcpManager.LoadFromDirectory(ctx, globalMcpPath, allowedServers); err != nil {
		// fmt.Printf("⚠️ Failed to load global MCPs: %v\n", err)
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
		saveSessionLog(flowName, userInput, clipboardContent, conf, results, histories)
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
	// Build map of valid step IDs
	validIDs := make(map[string]bool)
	for _, s := range conf.Steps {
		validIDs[s.ID] = true
	}

	var wg sync.WaitGroup
	for _, step := range conf.Steps {
		wg.Add(1)
		go func(s Step) {
			defer wg.Done()
			for !depsReady(s, validIDs) {
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
				// Fill tags in the prompt so the user sees the actual content (e.g. the draft flow)
				filledPrompt := fillTags(s.Prompt)
				if filledPrompt != "" {
					p.Send(StepInteractionOutputMsg{ID: s.ID, Output: filledPrompt})
				}

				if s.MaxTurns != nil && *s.MaxTurns == 1 {
					p.Send(StepInteractionRequiredMsg{ID: s.ID})
					res = <-inputChan
				} else {
					history := []ChatMessage{}
					// Add the initial system prompt/context if provided
					if filledPrompt != "" {
						history = append(history, ChatMessage{Role: "model", Parts: []map[string]string{{"text": filledPrompt}}})
					}

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

					mu.Lock()
					histories[s.ID] = history
					mu.Unlock()
				}
			} else if s.Type == "selector" {
				// Resolve source path
				sourcePath := fillTags(s.Source)
				if strings.HasPrefix(sourcePath, "./") {
					// Relative to current dir
				} else if strings.HasPrefix(sourcePath, "~/") {
					home, _ := os.UserHomeDir()
					sourcePath = filepath.Join(home, sourcePath[2:])
				}

				files, err := os.ReadDir(sourcePath)
				if err != nil {
					res = "" // Fail
				} else {
					// Sort files by modification time (newest first)
					sort.Slice(files, func(i, j int) bool {
						infoI, _ := files[i].Info()
						infoJ, _ := files[j].Info()
						return infoI.ModTime().After(infoJ.ModTime())
					})

					var fileList []FileInfo
					for _, f := range files {
						if !f.IsDir() {
							info, _ := f.Info()
							size := info.Size()
							sizeStr := fmt.Sprintf("%d B", size)
							if size > 1024 {
								sizeStr = fmt.Sprintf("%.1f KB", float64(size)/1024)
							}
							desc := fmt.Sprintf("%s • %s", info.ModTime().Format("2006-01-02 15:04"), sizeStr)
							fileList = append(fileList, FileInfo{Name: f.Name(), Desc: desc})
						}
					}
					p.Send(StepSelectorRequiredMsg{ID: s.ID, Files: fileList, Prompt: s.Prompt})
					selectedFile := <-inputChan
					
					content, err := os.ReadFile(filepath.Join(sourcePath, selectedFile))
					if err != nil {
						res = ""
					} else {
						res = string(content)
					}
				}
			} else if s.Type == "file_write" {
				// Check condition if present
				shouldRun := true
				if s.If != "" {
					cond := fillTags(s.If)
					// Very basic check: if string contains "false" or "no" or "0", skip
					// Or better: evaluate simple equality like "no != no"
					// For now, let's just check if it evaluates to "false" string literal
					// The user example is: "{{user_review}} != 'no'"
					// This is hard to parse without an expression engine.
					// Let's simplify: The user prompt says "Type a filename... or type 'no'".
					// So if the filename is "no", we abort.
					// Let's assume the condition is evaluated by the user logic before calling this, 
					// OR we implement a very simple "value != 'no'" check.
					
					// Hacky eval:
					parts := strings.Split(cond, "!=")
					if len(parts) == 2 {
						left := strings.TrimSpace(parts[0])
						right := strings.Trim(strings.TrimSpace(parts[1]), "'\"")
						if left == right {
							shouldRun = false
						}
					}
				}

				if shouldRun {
					fname := fillTags(s.Filename)
					content := fillTags(s.Content)
					
					// Clean markdown if it's a JSON file we are writing
					if strings.HasSuffix(fname, ".json") {
						content = cleanMarkdown(content)
					}
					
					// Ensure directory exists
					dir := filepath.Dir(fname)
					if strings.HasPrefix(fname, "./") {
						// relative
					} else if strings.HasPrefix(fname, "~/") {
						home, _ := os.UserHomeDir()
						fname = filepath.Join(home, fname[2:])
						dir = filepath.Dir(fname)
					}
					
					os.MkdirAll(dir, 0755)
					err := os.WriteFile(fname, []byte(content), 0644)
					if err != nil {
						res = fmt.Sprintf("Error writing file: %v", err)
					} else {
						res = fmt.Sprintf("File saved to %s", fname)
					}
				} else {
					res = "Skipped (Condition met)"
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

func depsReady(s Step, validIDs map[string]bool) bool {
	// Combine all fields that might contain tags
	combined := s.Prompt + s.Filename + s.Content + s.If + s.Source
	
	tags := regexp.MustCompile(`{{(.*?)}}`).FindAllStringSubmatch(combined, -1)
	mu.Lock()
	defer mu.Unlock()
	for _, t := range tags {
		tag := t[1]
		// Only wait for tags that are actual step IDs
		if validIDs[tag] && results[tag] == "" {
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
	ctx := context.Background()
	
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
	})
	if err != nil {
		fmt.Printf("Error creating client: %v\n", err)
		return ""
	}

	// Convert history to GenAI format
	var contents []*genai.Content
	for _, msg := range history {
		role := "user"
		if msg.Role == "model" {
			role = "model"
		}
		parts := make([]*genai.Part, 0)
		for _, p := range msg.Parts {
			if text, ok := p["text"]; ok {
				parts = append(parts, &genai.Part{Text: text})
			}
		}
		contents = append(contents, &genai.Content{
			Role:  role,
			Parts: parts,
		})
	}

	// Configure tools
	var tools []*genai.Tool
	if len(mcpManager.Tools) > 0 {
		tools = convertTools(mcpManager.Tools)
	}

	conf := &genai.GenerateContentConfig{
		Tools: tools,
	}
	if sys != "" {
		conf.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: sys}},
		}
	}

	// Main interaction loop for function calling
	for {
		resp, err := client.Models.GenerateContent(ctx, model, contents, conf)
		if err != nil {
			fmt.Printf("Error generating content: %v\n", err)
			return ""
		}

		if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
			return ""
		}

		part := resp.Candidates[0].Content.Parts[0]

		// Check for function call
		if part.FunctionCall != nil {
			fc := part.FunctionCall
			fmt.Printf("🤖 Calling tool: %s\n", fc.Name)
			
			// Execute tool
			result, err := mcpManager.CallTool(ctx, fc.Name, fc.Args)
			var toolResult map[string]interface{}
			if err != nil {
				toolResult = map[string]interface{}{"error": err.Error()}
			} else {
				// Convert MCP result to map for GenAI
				// MCP result content is usually a list of content objects.
				// We'll just dump the whole thing or extract text.
				// For simplicity, let's just pass the content list.
				toolResult = map[string]interface{}{"content": result.Content}
			}

			// Append model's function call to history
			contents = append(contents, &genai.Content{
				Role: "model",
				Parts: []*genai.Part{{FunctionCall: fc}},
			})

			// Append our function response to history
			contents = append(contents, &genai.Content{
				Role: "user",
				Parts: []*genai.Part{{
					FunctionResponse: &genai.FunctionResponse{
						Name:     fc.Name,
						Response: toolResult,
					},
				}},
			})
			
			// Loop to get the next response from the model
			continue
		}

		// If it's text, return it
		if part.Text != "" {
			return part.Text
		}
		
		return ""
	}
}

func convertTools(mcpTools []mcp.Tool) []*genai.Tool {
	var genaiTools []*genai.Tool
	for _, t := range mcpTools {
		genaiTools = append(genaiTools, &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  mapSchema(t.InputSchema),
				},
			},
		})
	}
	return genaiTools
}

func mapSchema(schema mcp.ToolInputSchema) *genai.Schema {
	s := &genai.Schema{
		Type:       genai.TypeObject,
		Properties: make(map[string]*genai.Schema),
		Required:   schema.Required,
	}

	for k, v := range schema.Properties {
		if subMap, ok := v.(map[string]interface{}); ok {
			s.Properties[k] = mapSubSchema(subMap)
		}
	}

	return s
}

func mapSubSchema(m map[string]interface{}) *genai.Schema {
	s := &genai.Schema{}
	if t, ok := m["type"].(string); ok {
		switch t {
		case "string":
			s.Type = genai.TypeString
		case "number":
			s.Type = genai.TypeNumber
		case "integer":
			s.Type = genai.TypeInteger
		case "boolean":
			s.Type = genai.TypeBoolean
		case "array":
			s.Type = genai.TypeArray
		case "object":
			s.Type = genai.TypeObject
		}
	}
	if d, ok := m["description"].(string); ok {
		s.Description = d
	}
	if props, ok := m["properties"].(map[string]interface{}); ok {
		s.Properties = make(map[string]*genai.Schema)
		for k, v := range props {
			if subMap, ok := v.(map[string]interface{}); ok {
				s.Properties[k] = mapSubSchema(subMap)
			}
		}
	}
	if items, ok := m["items"].(map[string]interface{}); ok {
		s.Items = mapSubSchema(items)
	}
	if req, ok := m["required"].([]interface{}); ok {
		for _, r := range req {
			if str, ok := r.(string); ok {
				s.Required = append(s.Required, str)
			}
		}
	}
	return s
}

var callGemini = func(model, sys, prompt string) string {
	if os.Getenv("MOCK_FLOW") == "true" {
		return "Mocked response for: " + prompt
	}
	apiKey := getAPIKey()
	ctx := context.Background()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
	})
	if err != nil {
		fmt.Printf("Error creating client: %v\n", err)
		return ""
	}

	conf := &genai.GenerateContentConfig{}
	if sys != "" {
		conf.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: sys}},
		}
	}

	resp, err := client.Models.GenerateContent(ctx, model, []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{{Text: prompt}},
		},
	}, conf)

	if err != nil {
		fmt.Printf("Error generating content: %v\n", err)
		return ""
	}

	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		return resp.Candidates[0].Content.Parts[0].Text
	}
	return ""
}

func copyToClipboard(s string) {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(s)
	cmd.Run()
}

func cleanMarkdown(data string) string {
	strData := strings.TrimSpace(data)
	if strings.HasPrefix(strData, "```json") {
		strData = strings.TrimPrefix(strData, "```json")
		strData = strings.TrimSuffix(strData, "```")
	} else if strings.HasPrefix(strData, "```") {
		strData = strings.TrimPrefix(strData, "```")
		strData = strings.TrimSuffix(strData, "```")
	}
	return strings.TrimSpace(strData)
}
