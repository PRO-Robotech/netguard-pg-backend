#!/bin/bash
# Quick file sync helper for hot reload
# Usage: ./sync-file.sh path/to/file.go

set -e

if [ -z "$1" ]; then
    echo "Usage: $0 <file-path>"
    echo "Example: $0 internal/api/netguard/service.go"
    exit 1
fi

FILE_PATH="$1"
PROJECT_ROOT="/Users/zhd/Projects/newPro/netguard-pg-backend"

# Remove leading slash or ./ if present
FILE_PATH="${FILE_PATH#./}"
FILE_PATH="${FILE_PATH#/}"

# Get full path
FULL_PATH="$PROJECT_ROOT/$FILE_PATH"

if [ ! -f "$FULL_PATH" ]; then
    echo "❌ File not found: $FULL_PATH"
    exit 1
fi

# Get pod name
POD=$(kubectl get pod -n incloud-sgroups \
    -l app.kubernetes.io/name=backend,app.kubernetes.io/component=debug-hot-reload \
    -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

if [ -z "$POD" ]; then
    echo "❌ Debug pod not found. Is hot-reload running?"
    exit 1
fi

echo "📦 Syncing: $FILE_PATH"
echo "   Pod: $POD"

# Copy file to pod
kubectl cp "$FULL_PATH" "incloud-sgroups/$POD:/src/$FILE_PATH"

if [ $? -eq 0 ]; then
    echo "✅ Synced! Air will rebuild in ~2-5 seconds"
    echo ""
    echo "Watch logs:"
    echo "  kubectl logs -f deployment/netguard-backend-debug -n incloud-sgroups"
else
    echo "❌ Sync failed"
    exit 1
fi
