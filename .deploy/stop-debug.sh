#!/bin/bash
set -e

NAMESPACE="incloud-sgroups"

echo "======================================"
echo "  Stopping NetGuard Debug Session"
echo "======================================"
echo

# 1. Kill port-forwards
echo "1. Stopping port-forwards..."
pkill -f "port-forward.*netguard.*debug" 2>/dev/null && echo "   ✅ Port-forwards stopped" || echo "   ℹ️  No port-forwards running"
echo

# 2. Restore production services
echo "2. Restoring production services..."
APISERVER_INSTANCE=$(kubectl get svc netguard-apiserver -n $NAMESPACE -o jsonpath='{.spec.selector.app\.kubernetes\.io/instance}')
if [ "$APISERVER_INSTANCE" == "netguard-debug" ]; then
  ./restore-production.sh
else
  echo "   ℹ️  Services already point to production pods"
fi
echo

echo "======================================"
echo "  Debug session stopped"
echo "======================================"
echo
echo "Production services restored."
echo "To start debug session again: ./start-debug.sh"
echo
