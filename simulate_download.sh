#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}🧪 Starting Flow Simulation...${NC}"

# 1. Setup isolated environment
SIM_DIR="tmp_simulation"
rm -rf "$SIM_DIR"
mkdir -p "$SIM_DIR/bin"
mkdir -p "$SIM_DIR/home"

# We need the absolute path for the build artifact
LOCAL_BINARY="$(pwd)/fast"

if [ ! -f "$LOCAL_BINARY" ]; then
    echo -e "${RED}❌ Binary 'fast' not found. Run 'make build' first.${NC}"
    exit 1
fi

# 2. Patch install.sh to run in simulation
# - Remove /dev/tty to allow piping input
# - Remove sudo
# - Change install path to our sim bin
# - Replace download curl with local copy
echo "🔧 Patching install.sh for simulation..."
cp install.sh "$SIM_DIR/install_sim.sh"

# Remove < /dev/tty
sed -i '' 's|< /dev/tty||g' "$SIM_DIR/install_sim.sh"

# Remove sudo
sed -i '' 's|sudo ||g' "$SIM_DIR/install_sim.sh"

# Change install path
# We use | as delimiter to avoid conflict with / in paths
sed -i '' "s|/usr/local/bin/fast|$SIM_DIR/bin/fast|g" "$SIM_DIR/install_sim.sh"

# Replace the download command with a copy command
# Matches: curl -L "$REPO_URL/releases/latest/download/fast" -o ...
sed -i '' "s|curl -L \"\$REPO_URL/releases/latest/download/fast\" -o .*|cp \"$LOCAL_BINARY\" \"$SIM_DIR/bin/fast\"|g" "$SIM_DIR/install_sim.sh"

# Patch flows download to use local file
# Matches: curl -sSL "https://raw.githubusercontent.com/ProggePal/flow/main/flows/scoping.json" -o ...
LOCAL_FLOWS_DIR="$(pwd)/flows"
sed -i '' "s|curl -sSL \".*scoping.json\" -o|cp \"$LOCAL_FLOWS_DIR/scoping.json\"|g" "$SIM_DIR/install_sim.sh"

# 3. Run the installer
echo "📦 Running simulated installation..."
export HOME="$(pwd)/$SIM_DIR/home"
export PATH="$(pwd)/$SIM_DIR/bin:$PATH"

# Simulate user inputting the key
FAKE_KEY="simulated-gemini-api-key-12345"
export GEMINI_API_KEY="$FAKE_KEY"
echo "Running install with GEMINI_API_KEY set..."
bash "$SIM_DIR/install_sim.sh"

# 4. Verify Installation
echo "🔍 Verifying installation..."

if [ ! -f "$SIM_DIR/bin/fast" ]; then
    echo -e "${RED}❌ Binary was not installed to $SIM_DIR/bin/fast${NC}"
    exit 1
fi

if [ ! -f "$HOME/.fast_key" ]; then
    echo -e "${RED}❌ Key file was not created at $HOME/.fast_key${NC}"
    exit 1
fi

INSTALLED_KEY=$(cat "$HOME/.fast_key")
if [ "$INSTALLED_KEY" != "$FAKE_KEY" ]; then
    echo -e "${RED}❌ Key mismatch! Expected '$FAKE_KEY', got '$INSTALLED_KEY'${NC}"
    exit 1
fi

# 5. Run the tool to check if it picks up the key
echo "🏃 Running fast to test key detection..."

# Create a dummy flow for testing
mkdir -p "$HOME/fast-flows/flows"
echo '{"Model": "gemini-pro", "Steps": [{"ID": "1", "Prompt": "Hello"}]}' > "$HOME/fast-flows/flows/test.json"

# Run flow. It will fail to connect to Gemini (fake key), but we check if it complains about MISSING key.
OUTPUT=$("$SIM_DIR/bin/fast" test 2>&1 || true)

if echo "$OUTPUT" | grep -q "No API Key found"; then
    echo -e "${RED}❌ FAIL: Fast Flow reported 'No API Key found!'${NC}"
    echo "Debug info:"
    ls -la "$HOME/.fast_key"
    echo "Content: $(cat "$HOME/.fast_key")"
    exit 1
else
    echo -e "${GREEN}✅ SUCCESS: Fast Flow detected the API key (it likely failed on the API call, which is expected).${NC}"
fi

echo -e "${GREEN}🎉 Simulation complete!${NC}"
