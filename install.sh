#!/bin/bash
REPO_URL="https://github.com/ProggePal/flow"

echo "🚀 Installing 'flow'..."
sudo curl -L "$REPO_URL/releases/latest/download/flow" -o /usr/local/bin/flow
sudo chmod +x /usr/local/bin/flow
sudo xattr -dr com.apple.quarantine /usr/local/bin/flow 2>/dev/null || true

# SETUP DEFAULT FLOWS
mkdir -p "$HOME/.flow/flows"
echo "📂 Setting up default flows in ~/.flow/flows..."
curl -sSL "https://raw.githubusercontent.com/ProggePal/flow/main/flows/scoping.json" -o "$HOME/.flow/flows/scoping.json"

# API KEY SETUP
if [ ! -f "$HOME/.flow_key" ]; then
    echo "🔑 Enter your Gemini API Key: "
    read -r api_key < /dev/tty
    echo "$api_key" > "$HOME/.flow_key"
    chmod 600 "$HOME/.flow_key"
    echo "✅ API Key saved safely to ~/.flow_key"
fi

echo "✅ Success! Just type 'flow <name>' to start."
