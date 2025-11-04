#!/bin/bash
set -e

NAMESPACE="incloud-sgroups"

echo "======================================"
echo "  Restarting Port Forwards"
echo "======================================"
echo

# 1. Kill existing port-forwards
echo "1. Stopping current port-forwards..."
pkill -f "port-forward.*netguard.*debug" 2>/dev/null && echo "   ✅ Stopped" || echo "   ℹ️  No port-forwards running"
sleep 2
echo

# 2. Check if debug pods are running
echo "2. Checking debug pods..."
DEBUG_PODS=$(kubectl get pod -n $NAMESPACE -l app.kubernetes.io/component=debug --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l | tr -d ' ')
if [ "$DEBUG_PODS" -eq "0" ]; then
  echo "   ❌ No debug pods are running"
  echo
  echo "   Start debug session first: ./start-debug.sh"
  exit 1
fi
echo "   ✅ Found $DEBUG_PODS running debug pod(s)"
echo

# 3. Start port-forwards
echo "3. Starting new port-forwards..."

kubectl port-forward -n $NAMESPACE deployment/netguard-backend-debug 2345:2345 > /tmp/pf-backend.log 2>&1 &
BACKEND_PID=$!
echo "   Backend (Delve): localhost:2345 (PID: $BACKEND_PID)"

kubectl port-forward -n $NAMESPACE deployment/netguard-apiserver-debug 2346:2346 > /tmp/pf-apiserver.log 2>&1 &
APISERVER_PID=$!
echo "   APIServer (Delve): localhost:2346 (PID: $APISERVER_PID)"

kubectl port-forward -n $NAMESPACE deployment/netguard-webhook-debug 2347:2347 > /tmp/pf-webhook.log 2>&1 &
WEBHOOK_PID=$!
echo "   Webhook (Delve): localhost:2347 (PID: $WEBHOOK_PID)"

sleep 3
echo

# 4. Verify
echo "4. Verifying..."
FAILED=0

if ps -p $BACKEND_PID > /dev/null 2>&1; then
  echo "   ✅ Backend port-forward active"
else
  echo "   ❌ Backend port-forward failed"
  FAILED=1
fi

if ps -p $APISERVER_PID > /dev/null 2>&1; then
  echo "   ✅ APIServer port-forward active"
else
  echo "   ❌ APIServer port-forward failed"
  FAILED=1
fi

if ps -p $WEBHOOK_PID > /dev/null 2>&1; then
  echo "   ✅ Webhook port-forward active"
else
  echo "   ❌ Webhook port-forward failed"
  FAILED=1
fi
echo

if [ $FAILED -eq 1 ]; then
  echo "⚠️  Some port-forwards failed. Check logs:"
  echo "   tail -f /tmp/pf-*.log"
  exit 1
fi

echo "======================================"
echo "  Port forwards restarted!"
echo "======================================"
echo
echo "Debug ports available:"
echo "  - Backend:   localhost:2345"
echo "  - APIServer: localhost:2346"
echo "  - Webhook:   localhost:2347"
echo
