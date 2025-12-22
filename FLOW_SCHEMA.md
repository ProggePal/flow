# Fast Flow JSON Schema Reference

This document describes the JSON structure used to define Flows in Fast Flow.

## Root Object

The root of the JSON file represents the Flow Configuration.

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `model` | `string` | **Yes** | The default Gemini model to use for all steps (e.g., `gemini-2.0-flash`, `gemini-1.5-pro`). |
| `system_prompt` | `string` | No | A global system instruction applied to all AI steps. Defines the persona or general rules. |
| `steps` | `Array<Step>` | **Yes** | An ordered list of steps to execute. |

---

## Step Object

Each object in the `steps` array represents a distinct action.

| Field | Type | Description |
| :--- | :--- | :--- |
| `id` | `string` | **Required.** A unique identifier for the step. Used for variable referencing (e.g., `{{step_id}}`). |
| `type` | `string` | The type of action. Defaults to `"text"` if omitted. See [Step Types](#step-types) below. |
| `prompt` | `string` | The instruction or text content for the step. Supports [Variables](#variables). |
| `model` | `string` | Override the global model for this specific step. |
| `tab_id` | `string` | Used to maintain conversation history across steps. Steps with the same `tab_id` share context. |
| `if` | `string` | A condition to evaluate before running the step. If it evaluates to "false" (string comparison), the step is skipped. |

### Step Types

#### 1. `text` (Default)
Standard AI generation. Sends the `prompt` to the model and stores the response.

```json
{
  "id": "summary",
  "type": "text",
  "prompt": "Summarize this: {{clipboard}}"
}
```

#### 2. `interaction`
Starts an interactive session in the terminal.

| Field | Type | Description |
| :--- | :--- | :--- |
| `max_turns` | `integer` | If set to `1`, it asks for a single user input and continues. If `null` or omitted, it enters an infinite chat loop until the user types `__END_INTERACTION__` (or Ctrl+C). |

```json
{
  "id": "ask_user",
  "type": "interaction",
  "prompt": "What is your name?",
  "max_turns": 1
}
```

#### 3. `selector`
Displays a file selection menu in the terminal.

| Field | Type | Description |
| :--- | :--- | :--- |
| `source` | `string` | **Required.** The directory path to list files from (e.g., `~/Documents`, `./src`). |

```json
{
  "id": "pick_file",
  "type": "selector",
  "source": "./logs",
  "prompt": "Select a log file to analyze:"
}
```

#### 4. `file_write`
Writes content to a file.

| Field | Type | Description |
| :--- | :--- | :--- |
| `filename` | `string` | **Required.** The destination path. Supports variables. |
| `content` | `string` | **Required.** The text to write to the file. Supports variables. |

```json
{
  "id": "save_report",
  "type": "file_write",
  "filename": "~/reports/{{input}}.md",
  "content": "{{summary}}"
}
```

---

## Variables

You can inject dynamic data into `prompt`, `filename`, `content`, `source`, and `if` fields using double curly braces `{{...}}`.

| Variable | Description |
| :--- | :--- |
| `{{clipboard}}` | The current content of the system clipboard. |
| `{{input}}` | The command-line argument provided after the flow name (e.g., `fast myflow "some input"`). |
| `{{step_id}}` | The output of a previous step with the matching `id`. |

---

## Example Flow

```json
{
  "model": "gemini-2.0-flash",
  "system_prompt": "You are a helpful coding assistant.",
  "steps": [
    {
      "id": "get_code",
      "type": "text",
      "prompt": "Fix the bugs in this code:\n\n{{clipboard}}"
    },
    {
      "id": "ask_filename",
      "type": "interaction",
      "prompt": "Where should I save the fixed code?",
      "max_turns": 1
    },
    {
      "id": "save_file",
      "type": "file_write",
      "filename": "{{ask_filename}}",
      "content": "{{get_code}}"
    }
  ]
}
```
