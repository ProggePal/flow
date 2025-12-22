# MCP Extensions 🔌

This folder allows you to extend the capabilities of the Flow CLI by adding **Model Context Protocol (MCP)** servers.

Simply drop a `.json` configuration file here, and the CLI will automatically start the MCP server and make its tools available to the AI.

## How to add an MCP

1.  Create a `.json` file in this directory (e.g., `sqlite.json`, `git.json`).
2.  Define the command to run the MCP server in the JSON file.

## Configuration Format

```json
{
  "name": "unique-name",
  "command": "program-to-run",
  "args": ["arg1", "arg2"],
  "env": {
    "OPTIONAL_ENV_VAR": "value"
  }
}
```

## Examples

### 1. SQLite (Python/uv)
If you have `uv` installed, you can run the official SQLite MCP server:

**File:** `sqlite.json`
```json
{
  "name": "sqlite",
  "command": "uv",
  "args": ["x", "mcp-server-sqlite", "--db-path", "./my-data.db"]
}
```

### 2. GitHub (Node.js/npx)
If you have Node.js installed, you can run the GitHub MCP server:

**File:** `github.json`
```json
{
  "name": "github",
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-github"],
  "env": {
    "GITHUB_PERSONAL_ACCESS_TOKEN": "your-token-here"
  }
}
```

### 3. Filesystem (Node.js/npx)
Give the AI access to a specific folder on your computer:

**File:** `filesystem.json`
```json
{
  "name": "filesystem",
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-filesystem", "/Users/username/Documents/Project"]
}
```
