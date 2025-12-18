# Fast Flow 🚀

### One-Time Setup
Paste this in your Terminal (replace `YOUR_KEY_HERE` with your actual Gemini API Key):
`GEMINI_API_KEY="YOUR_KEY_HERE" curl -sSL https://raw.githubusercontent.com/ProggePal/flow/main/install.sh | bash`

### How to run a workflow
1. Copy your source text (transcript, notes, etc).
2. Type `fast scoping` (where 'scoping' is the name of a file in the /flows folder).
3. The result is automatically copied to your clipboard.

### How it works
- It automatically grabs your **clipboard** text.
- It runs steps in **parallel** where possible.
- It copies the **final result** back to your clipboard automatically.
