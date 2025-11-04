#!/bin/bash
# Run Skaffold debug without streaming pod logs to console
# Logs are still available via: kubectl logs -f <pod-name>

set -e

PROFILE="${1:-debug-all}"
NAMESPACE="incloud-sgroups"

echo "🚀 Starting Skaffold in quiet mode (profile: $PROFILE)"
echo "📝 Pod logs are NOT streamed to console"
echo "💡 To view logs: kubectl logs -f <pod-name> -n $NAMESPACE"
echo ""

# Function to wait for deployments and switch services
auto_switch_to_debug() {
    # Sleep to let Skaffold create deployments first
    sleep 10

    echo "⏳ Waiting for debug deployments to be created..."

    # Wait for deployments to exist first (max 2 minutes)
    for i in {1..24}; do
        if kubectl get deployment/netguard-backend-debug \
            deployment/netguard-apiserver-debug \
            deployment/netguard-webhook-debug \
            -n "$NAMESPACE" &>/dev/null; then
            break
        fi
        sleep 5
    done

    echo "⏳ Waiting for debug pods to become ready..."

    # Now wait for all deployments to be ready (max 5 minutes)
    if kubectl wait --for=condition=available --timeout=300s \
        deployment/netguard-backend-debug \
        deployment/netguard-apiserver-debug \
        deployment/netguard-webhook-debug \
        -n "$NAMESPACE" 2>/dev/null; then

        echo "✅ All debug pods are ready!"
        echo "🔀 Switching services to debug pods..."

        # Run switch-to-debug.sh with auto-confirm
        cd "$(dirname "$0")/../" && AUTO_CONFIRM=1 ./switch-to-debug.sh

        echo ""
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "✅ DEBUG ENVIRONMENT READY!"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        echo "🐛 Debuggers available at:"
        echo "   Backend:    localhost:2345"
        echo "   APIServer:  localhost:2346"
        echo "   Webhook:    localhost:2347"
        echo ""
        echo "🌐 Services available at:"
        echo "   Backend gRPC:  localhost:9090"
        echo "   Backend HTTP:  localhost:8080"
        echo "   PostgreSQL:    localhost:5432"
        echo ""
        echo "📝 View logs:"
        echo "   kubectl logs -f deployment/netguard-backend-debug -n $NAMESPACE"
        echo "   kubectl logs -f deployment/netguard-apiserver-debug -n $NAMESPACE"
        echo "   kubectl logs -f deployment/netguard-webhook-debug -n $NAMESPACE"
        echo ""
    else
        echo "❌ Timeout waiting for debug pods"
    fi
}

# Cleanup function to switch back to production on exit
cleanup() {
    echo ""
    echo "🔄 Switching services back to production..."
    cd "$(dirname "$0")/../" && AUTO_CONFIRM=1 ./switch-to-prod.sh 2>/dev/null || true
    echo "✅ Cleanup complete!"
}

# Only auto-switch for debug-all profile
if [ "$PROFILE" = "debug-all" ]; then
    trap cleanup EXIT
    auto_switch_to_debug &
fi

skaffold dev -p "$PROFILE" --tail=false
