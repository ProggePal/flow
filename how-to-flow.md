# 🛠 How to Create a Flow

A "Flow" is a simple JSON file that tells the AI how to process your data. All flows live in the `/flows` folder. The name of the file becomes the command you type (e.g., `scope.json` → `fast scope`).

## 1. The Basic Template

Every flow starts with a global model and a list of steps.

```json
{
  "model": "gemini-2.5-flash",
  "system_prompt": "Optional: Describe the AI's personality here.",
  "steps": [
    {
      "id": "my_first_step",
      "prompt": "Do something with {{clipboard}}"
    }
  ]
}

```

---

## 2. Using Tags (The "Connectors")

Tags allow steps to talk to each other. You don't need to tell the engine what order to run things in; it figures it out by looking at your tags.

* **`{{clipboard}}`**: Injects whatever text you currently have copied.
* **`{{input}}`**: Injects text typed after the command (e.g., `fast reply "I am sick"`). If no input is provided, it becomes an empty string.
* **`{{id}}`**: Injects the result of a previous step (e.g., `{{analysis}}`).

### Automatic Parallelism

If Step B and Step C both use `{{step_A}}`, the engine will run B and C **at the same time** the moment A is finished.

---

## 3. Using Tabs (The "Memory")

By default, every step is a "Fresh Start" and the AI forgets what happened in the previous step. If you want the AI to remember the conversation, use a `tab_id`.

* **Same `tab_id**`: The AI remembers the context (like a chat thread).
* **Different/No `tab_id**`: The AI starts with a clean slate (faster and more focused).

```json
{
  "id": "refine_notes",
  "tab_id": "meeting_notes",
  "prompt": "Based on our previous discussion in this tab, summarize the next steps."
}

```

---

## 4. Overriding Models

You can use a cheaper, faster model for simple tasks and the big "Pro" model for the hard work.

* **`gemini-2.0-flash`**: Fast & Cheap (Best for summarizing, cleaning text, or extracting names).
* **`gemini-2.5-pro`**: Smart & Deep (Best for drafting complex documents or reasoning).

```json
{
  "id": "quick_cleanup",
  "model": "gemini-1.5-flash",
  "prompt": "Fix the typos in this: {{clipboard}}"
}

```

---

## 5. The Interaction Block (Human-in-the-Loop)

Sometimes you need to pause the flow and ask the user for input. You can do this with the `interaction` type.

### Single Input (Form Field)
Use `max_turns: 1` to ask a single question. The result is whatever the user types.

```json
{
  "id": "ask_topic",
  "type": "interaction",
  "max_turns": 1,
  "prompt": "What topic should I write about?"
}
```

### Chat Session (Conversation)
Use `max_turns: null` (or omit it) to start an open-ended chat. The flow pauses until the user presses **Esc**. The result is the full transcript of the conversation.

```json
{
  "id": "brainstorm",
  "type": "interaction",
  "max_turns": null,
  "prompt": "Let's brainstorm ideas. What's on your mind?"
}
```

---

## 6. Getting the Result

The engine is smart: it automatically takes the **very last step** in your JSON file and copies it to your clipboard when the flow is done. You don't need to configure anything.

---

### 💡 Tips for Authors

1. **Be Specific:** Use the `system_prompt` to tell the AI to be "Concise," "Professional," or "Funny."
2. **Chain Logic:** Break big tasks into small steps. Instead of one giant prompt, do: `Clean` → `Extract` → `Draft`.
3. **Naming:** Give your steps clear IDs like `summary` or `tasks` so your tags are easy to read: `{{summary}}`.

