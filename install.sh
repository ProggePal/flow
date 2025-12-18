#!/bin/bash
REPO_URL="https://github.com/ProggePal/flow"
KEY_FILE="$HOME/.flow_key"

echo "👋 Let's get Flow set up on your Mac!"

# 1. Handle the API Key (The "Magic" behind the AI)
if [ -s "$KEY_FILE" ]; then
    echo "✅ Found your API Key."
else
    echo "🔑 To start, you'll need a Gemini API Key."
    echo "   (You can get one for free here: https://aistudio.google.com/app/apikey)"
    echo ""
    
    while [ -z "$api_key" ]; do
        printf "👉 Paste your API Key here: "
        read -r api_key < /dev/tty
        api_key=$(echo "$api_key" | xargs) # Clean up accidental spaces
        
        if [ -z "$api_key" ]; then
            echo "❌ That looked empty. Please try pasting the key again."
        fi
    done

    echo "$api_key" > "$KEY_FILE"
    chmod 600 "$KEY_FILE"
    echo "✅ Key saved safely."
fi

# 2. Add the Flow tool to your computer
echo "🚀 Installing the Flow command..."
sudo curl -L "$REPO_URL/releases/latest/download/flow" -o /usr/local/bin/flow
sudo chmod +x /usr/local/bin/flow

# This line ensures macOS doesn't show a "Damaged App" warning
sudo xattr -dr com.apple.quarantine /usr/local/bin/flow 2>/dev/null || true

# 3. Bring in your Workflows
echo "📂 Downloading your initial flows..."
mkdir -p "$HOME/.flow/flows"
curl -sSL "https://raw.githubusercontent.com/ProggePal/flow/main/flows/scoping.json" -o "$HOME/.flow/flows/scoping.json"

echo ""
echo "✨ All set! You can now use Flow anywhere."
echo "👉 Try it now: copy some text and type 'flow scoping' in your terminal."