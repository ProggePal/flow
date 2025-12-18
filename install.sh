#!/bin/bash
REPO_URL="https://github.com/ProggePal/flow"
KEY_FILE="$HOME/.fast_key"

echo "👋 Let's get Fast Flow set up on your Mac!"

# 1. Handle the API Key (The "Magic" behind the AI)
if [ -s "$KEY_FILE" ]; then
    echo "✅ Found your API Key."
elif [ -n "$GEMINI_API_KEY" ]; then
    echo "✅ Found GEMINI_API_KEY in environment."
    echo "$GEMINI_API_KEY" > "$KEY_FILE"
    chmod 600 "$KEY_FILE"
    echo "✅ Key saved safely."
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
echo "🚀 Installing the Fast Flow command..."
sudo curl -L "$REPO_URL/releases/latest/download/fast" -o /usr/local/bin/fast
sudo chmod +x /usr/local/bin/fast

# This line ensures macOS doesn't show a "Damaged App" warning
sudo xattr -dr com.apple.quarantine /usr/local/bin/fast 2>/dev/null || true

# 3. Bring in your Workflows
echo "📂 Downloading your initial flows..."
mkdir -p "$HOME/fast-flows/flows"
curl -sSL "https://raw.githubusercontent.com/ProggePal/flow/main/flows/scope.json" -o "$HOME/fast-flows/flows/scope.json"
curl -sSL "https://raw.githubusercontent.com/ProggePal/flow/main/flows/sum.json" -o "$HOME/fast-flows/flows/sum.json"

echo ""
echo "✨ All set! You can now use Fast Flow anywhere."
echo "👉 Try it now: copy some text and type 'fast scope' in your terminal."