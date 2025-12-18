#!/bin/bash
REPO_URL="https://github.com/YOUR_USER/flow"

echo "🚀 Installing 'flow'..."
sudo curl -L "$REPO_URL/releases/latest/download/flow" -o /usr/local/bin/flow
sudo chmod +x /usr/local/bin/flow
sudo xattr -dr com.apple.quarantine /usr/local/bin/flow 2>/dev/null || true

# API KEY SETUP
if [ ! -f "$HOME/.flow_key" ]; then
    read -p "🔑 Enter your Gemini API Key: " api_key
    echo "$api_key" > "$HOME/.flow_key"
    chmod 600 "$HOME/.flow_key"
    echo "✅ API Key saved safely to ~/.flow_key"
fi

echo "✅ Success! Just type 'flow <name>' to start."
