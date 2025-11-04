#!/bin/bash
# Run Skaffold with hot reload enabled (Air + file sync)
# Perfect for rapid backend development iterations

set -e

cd "$(dirname "$0")"

echo "======================================"
echo "  NetGuard Hot Reload Mode"
echo "======================================"
echo ""
echo "🔥 Starting hot reload development mode..."
echo ""
echo "What this does:"
echo "  ✅ File sync (no image rebuild on *.go changes)"
echo "  ✅ Air watches for changes inside pod"
echo "  ✅ Auto-rebuild in ~2-5 seconds"
echo "  ✅ Pod stays alive, port-forwards preserved"
echo "  ✅ Delve debugger available on :2345"
echo ""
echo "Note: First build will take ~30-60 seconds"
echo "      Subsequent changes: only 2-5 seconds!"
echo ""

# Check if skaffold is installed
if ! command -v skaffold &> /dev/null; then
    echo "❌ skaffold not found. Please install it:"
    echo "   brew install skaffold"
    exit 1
fi

# Run Skaffold with hot-reload profile
echo "🚀 Starting Skaffold (profile: debug-hot-reload)..."
echo ""

skaffold dev -p debug-hot-reload --tail=false

echo ""
echo "Hot reload session ended."
