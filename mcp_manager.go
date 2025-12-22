package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPConfig represents the configuration for an MCP server
type MCPConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// MCPManager manages multiple MCP clients
type MCPManager struct {
	Clients    []*client.Client
	Tools      []mcp.Tool
	ToolClient map[string]*client.Client
}

// NewMCPManager creates a new MCP manager
func NewMCPManager() *MCPManager {
	return &MCPManager{
		Clients:    make([]*client.Client, 0),
		Tools:      make([]mcp.Tool, 0),
		ToolClient: make(map[string]*client.Client),
	}
}

// LoadFromDirectory scans a directory for JSON configs and starts MCP clients
func (m *MCPManager) LoadFromDirectory(ctx context.Context, dir string, allowedServers map[string]bool) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Directory doesn't exist, that's fine
		}
		return err
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) == ".json" {
			configPath := filepath.Join(dir, f.Name())
			
			// Read and parse config first to check name
			data, err := os.ReadFile(configPath)
			if err != nil {
				fmt.Printf("⚠️ Failed to read MCP config %s: %v\n", f.Name(), err)
				continue
			}

			var conf MCPConfig
			if err := json.Unmarshal(data, &conf); err != nil {
				fmt.Printf("⚠️ Failed to parse MCP config %s: %v\n", f.Name(), err)
				continue
			}

			// Check if allowed
			// If allowedServers is nil, allow all.
			// If allowedServers is not nil, check if name is in map.
			if allowedServers != nil {
				if !allowedServers[conf.Name] {
					continue
				}
			}

			if err := m.startClient(ctx, conf); err != nil {
				fmt.Printf("⚠️ Failed to start MCP %s: %v\n", conf.Name, err)
			}
		}
	}
	return nil
}

func (m *MCPManager) startClient(ctx context.Context, conf MCPConfig) error {
	if conf.Command == "" {
		return fmt.Errorf("command is required")
	}

	fmt.Printf("🔌 Starting MCP: %s (%s %v)...\n", conf.Name, conf.Command, conf.Args)

	// Convert env map to slice
	var env []string
	for k, v := range conf.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Create stdio transport
	tr := transport.NewStdio(conf.Command, env, conf.Args...)

	// Create client
	c := client.NewClient(tr)

	// Start client
	if err := c.Start(ctx); err != nil {
		return fmt.Errorf("failed to start client: %w", err)
	}

	// Initialize
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "flow-cli",
		Version: "1.0.0",
	}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}

	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := c.Initialize(initCtx, initReq)
	if err != nil {
		c.Close()
		return fmt.Errorf("failed to initialize: %w", err)
	}

	m.Clients = append(m.Clients, c)

	// Fetch tools
	listToolsReq := mcp.ListToolsRequest{}
	resp, err := c.ListTools(ctx, listToolsReq)
	if err != nil {
		fmt.Printf("⚠️ Failed to list tools for %s: %v\n", conf.Name, err)
	} else {
		m.Tools = append(m.Tools, resp.Tools...)
		for _, t := range resp.Tools {
			m.ToolClient[t.Name] = c
		}
		fmt.Printf("✅ Loaded MCP %s (%d tools)\n", conf.Name, len(resp.Tools))
	}

	return nil
}

// Close closes all clients
func (m *MCPManager) Close() {
	for _, c := range m.Clients {
		c.Close()
	}
}

// FindTool finds a tool by name
func (m *MCPManager) FindTool(name string) *mcp.Tool {
	for _, t := range m.Tools {
		if t.Name == name {
			return &t
		}
	}
	return nil
}

// CallTool calls a tool on the appropriate client
func (m *MCPManager) CallTool(ctx context.Context, name string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	client, ok := m.ToolClient[name]
	if !ok {
		return nil, fmt.Errorf("tool %s not found", name)
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	return client.CallTool(ctx, req)
}
