#!/bin/bash

# 1. Build the binary
echo "📦 Building 'fast' binary..."
# Build for macOS (ARM64) - Change GOOS/GOARCH if you need to support other platforms
env GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w" -o fast .

if [ $? -ne 0 ]; then
    echo "❌ Build failed."
    exit 1
fi

echo "✅ Build successful: ./fast"

# 2. Instructions for GitHub Release
echo ""
echo "🚀 To deploy to GitHub:"
echo "1. Commit and push your changes:"
echo "   git add ."
echo "   git commit -m \"Prepare release\""
echo "   git push origin main"
echo "   git push origin main"
echo ""
echo "2. Create a new Release on GitHub:"
echo "   - Go to: https://github.com/ProggePal/flow/releases/new"
echo "   - Tag version: v1.0.0 (or similar)"
echo "   - Title: Release v1.0.0"
echo "   - Description: Initial release"
echo ""
echo "3. Upload the binary:"
echo "   - Drag and drop the 'fast' file generated in this folder to the release assets."
echo ""
echo "4. Publish Release!"
