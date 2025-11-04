#!/bin/bash
set -e

NAMESPACE="incloud-sgroups"
DEBUG_PORTS=(2345 2346 2347)

echo "======================================"
echo "  NetGuard Debug Session Startup"
echo "======================================"
echo

# 0. Check if debug pods exist
echo "0. Checking if debug pods exist..."
DEBUG_PODS=$(kubectl get pod -n $NAMESPACE -l app.kubernetes.io/component=debug --no-headers 2>/dev/null | wc -l | tr -d ' ')
if [ "$DEBUG_PODS" -eq "0" ]; then
  echo "   ❌ No debug pods found!"
  echo
  echo "   Please deploy debug pods first:"
  echo "   cd skaffold && skaffold dev -p debug-all"
  echo
  exit 1
fi
echo "   ✅ Found $DEBUG_PODS debug pod(s)"
echo

# 1. Check debug pods status
echo "1. Checking debug pods status..."
NOT_RUNNING=$(kubectl get pod -n $NAMESPACE -l app.kubernetes.io/component=debug --no-headers 2>/dev/null | grep -v "Running" | wc -l | tr -d ' ')
if [ "$NOT_RUNNING" -gt "0" ]; then
  echo "   ⚠️  Some debug pods are not running:"
  kubectl get pod -n $NAMESPACE -l app.kubernetes.io/component=debug
  echo
  echo "   Wait for pods to be ready or check logs: kubectl logs -n $NAMESPACE <pod-name>"
  read -p "   Continue anyway? (y/N): " -n 1 -r
  echo
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    exit 1
  fi
else
  echo "   ✅ All debug pods are running"
  kubectl get pod -n $NAMESPACE -l app.kubernetes.io/component=debug --no-headers
fi
echo

# 2. Check if debug ports are available
echo "2. Checking if debug ports are available..."
PORTS_IN_USE=()
for PORT in "${DEBUG_PORTS[@]}"; do
  if lsof -Pi :$PORT -sTCP:LISTEN -t >/dev/null 2>&1; then
    PID=$(lsof -Pi :$PORT -sTCP:LISTEN -t)
    PROCESS=$(ps -p $PID -o comm= 2>/dev/null || echo "unknown")
    PORTS_IN_USE+=("$PORT (PID: $PID, Process: $PROCESS)")
  fi
done

if [ ${#PORTS_IN_USE[@]} -gt 0 ]; then
  echo "   ⚠️  Following ports are already in use:"
  for PORT_INFO in "${PORTS_IN_USE[@]}"; do
    echo "      $PORT_INFO"
  done
  echo
  read -p "   Kill these processes and continue? (y/N): " -n 1 -r
  echo
  if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "   Killing processes..."
    for PORT in "${DEBUG_PORTS[@]}"; do
      lsof -ti :$PORT 2>/dev/null | xargs kill -9 2>/dev/null || true
    done
    sleep 2
  else
    echo "   Aborted. Please free up ports manually."
    exit 1
  fi
else
  echo "   ✅ All debug ports are available"
fi
echo

# 3. Switch production services to debug pods (if needed)
echo "3. Checking if services point to debug pods..."
APISERVER_INSTANCE=$(kubectl get svc netguard-apiserver -n $NAMESPACE -o jsonpath='{.spec.selector.app\.kubernetes\.io/instance}' 2>/dev/null || echo "")
if [ "$APISERVER_INSTANCE" != "netguard-debug" ]; then
  echo "   Switching services to debug pods..."
  ./switch-to-debug.sh
else
  echo "   ✅ Services already point to debug pods"
fi
echo

# 4. Kill any remaining netguard debug port-forwards
echo "4. Cleaning up old port-forwards..."
pkill -f "port-forward.*netguard.*debug" 2>/dev/null || true
sleep 2
echo "   ✅ Cleaned up"
echo

# 5. Start port-forwards in background
echo "5. Starting port-forwards..."
kubectl port-forward -n $NAMESPACE deployment/netguard-backend-debug 2345:2345 > /tmp/pf-backend.log 2>&1 &
BACKEND_PID=$!
echo "   Backend (Delve): localhost:2345 (PID: $BACKEND_PID)"

kubectl port-forward -n $NAMESPACE deployment/netguard-apiserver-debug 2346:2346 > /tmp/pf-apiserver.log 2>&1 &
APISERVER_PID=$!
echo "   APIServer (Delve): localhost:2346 (PID: $APISERVER_PID)"

kubectl port-forward -n $NAMESPACE deployment/netguard-webhook-debug 2347:2347 > /tmp/pf-webhook.log 2>&1 &
WEBHOOK_PID=$!
echo "   Webhook (Delve): localhost:2347 (PID: $WEBHOOK_PID)"

# Wait for port-forwards to establish
sleep 3
echo

# 6. Verify port-forwards
echo "6. Verifying port-forwards..."
FAILED=0

if ps -p $BACKEND_PID > /dev/null 2>&1; then
  echo "   ✅ Backend port-forward active"
else
  echo "   ❌ Backend port-forward failed (check /tmp/pf-backend.log)"
  FAILED=1
fi

if ps -p $APISERVER_PID > /dev/null 2>&1; then
  echo "   ✅ APIServer port-forward active"
else
  echo "   ❌ APIServer port-forward failed (check /tmp/pf-apiserver.log)"
  FAILED=1
fi

if ps -p $WEBHOOK_PID > /dev/null 2>&1; then
  echo "   ✅ Webhook port-forward active"
else
  echo "   ❌ Webhook port-forward failed (check /tmp/pf-webhook.log)"
  FAILED=1
fi
echo

if [ $FAILED -eq 1 ]; then
  echo "⚠️  Some port-forwards failed. Check logs in /tmp/pf-*.log"
  echo
fi

echo "======================================"
echo "  Debug session ready!"
echo "======================================"
echo
echo "Next steps:"
echo "  1. Connect your IDE debugger to:"
echo "     - Backend:   localhost:2345"
echo "     - APIServer: localhost:2346"
echo "     - Webhook:   localhost:2347"
echo
echo "  2. Set breakpoints in your code"
echo
echo "  3. Send requests from frontend (http://localhost:8081)"
echo "     Requests will hit the debug pods with breakpoints"
echo
echo "  4. Monitor logs:"
echo "     Backend:   tail -f /tmp/pf-backend.log"
echo "     APIServer: tail -f /tmp/pf-apiserver.log"
echo "     Webhook:   tail -f /tmp/pf-webhook.log"
echo
echo "  5. To stop debug session: ./stop-debug.sh"
echo
