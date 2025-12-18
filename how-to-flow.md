This is a great idea. Since your colleagues are essentially "Recipe Authors" now, they need a simple guide that explains how to use the "Tags," "Tabs," and "Parallelism" without getting bogged down in technical jargon.

Here is a clean, Markdown-formatted **`how-to-flow.md`** you can include in your repository.

---

# 🛠 How to Create a Flow

A "Flow" is a simple JSON file that tells the AI how to process your data. All flows live in the `/flows` folder. The name of the file becomes the command you type (e.g., `scope.json` → `fast scope`).

## 1. The Basic Template

Every flow starts with a global model and a list of steps.

```json
{
  "model": "gemini-1.5-pro",
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

* **`gemini-1.5-flash`**: Fast & Cheap (Best for summarizing, cleaning text, or extracting names).
* **`gemini-1.5-pro`**: Smart & Deep (Best for drafting complex documents or reasoning).

```json
{
  "id": "quick_cleanup",
  "model": "gemini-1.5-flash",
  "prompt": "Fix the typos in this: {{clipboard}}"
}

```

---

## 5. Getting the Result

The engine is smart: it automatically takes the **very last step** in your JSON file and copies it to your clipboard when the flow is done. You don't need to configure anything.

---

### 💡 Tips for Authors

1. **Be Specific:** Use the `system_prompt` to tell the AI to be "Concise," "Professional," or "Funny."
2. **Chain Logic:** Break big tasks into small steps. Instead of one giant prompt, do: `Clean` → `Extract` → `Draft`.
3. **Naming:** Give your steps clear IDs like `summary` or `tasks` so your tags are easy to read: `{{summary}}`.

---

Would you like me to add a **"Troubleshooting"** section to this guide for common errors like missing API keys or broken JSON?