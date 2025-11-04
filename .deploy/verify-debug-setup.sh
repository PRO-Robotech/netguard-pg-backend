#!/bin/bash

set -e

NAMESPACE="incloud-sgroups"

echo "======================================"
echo "  NetGuard Debug Setup Verification"
echo "======================================"
echo ""

# Check if debug pods are running
echo "1. Checking debug pods..."
PODS=$(kubectl get pods -n $NAMESPACE -l app.kubernetes.io/component=debug --no-headers 2>/dev/null || true)
if [ -z "$PODS" ]; then
    echo "   ❌ No debug pods found"
    echo "   Run: cd .deploy/skaffold && skaffold dev -p debug-backend"
    exit 1
else
    echo "$PODS"
fi
echo ""

# Check pod status
echo "2. Checking pod readiness..."
NOT_READY=$(kubectl get pods -n $NAMESPACE -l app.kubernetes.io/component=debug --no-headers 2>/dev/null | grep -v "Running\|Completed" || true)
if [ -n "$NOT_READY" ]; then
    echo "   ⚠️  Some pods are not ready:"
    echo "$NOT_READY"
    echo ""
    echo "   Check logs with: kubectl logs -n $NAMESPACE <pod-name>"
else
    echo "   ✅ All debug pods are running"
fi
echo ""

# Check if migrations ran successfully
echo "3. Checking migrations (init container)..."
BACKEND_POD=$(kubectl get pods -n $NAMESPACE -l app.kubernetes.io/name=backend,app.kubernetes.io/instance=netguard-debug --no-headers -o custom-columns=":metadata.name" 2>/dev/null | head -1)
if [ -n "$BACKEND_POD" ]; then
    INIT_STATUS=$(kubectl get pod $BACKEND_POD -n $NAMESPACE -o jsonpath='{.status.initContainerStatuses[0].state}' 2>/dev/null || echo "")
    if echo "$INIT_STATUS" | grep -q "terminated"; then
        INIT_EXIT_CODE=$(kubectl get pod $BACKEND_POD -n $NAMESPACE -o jsonpath='{.status.initContainerStatuses[0].state.terminated.exitCode}' 2>/dev/null)
        if [ "$INIT_EXIT_CODE" = "0" ]; then
            echo "   ✅ Migration init container completed successfully"
        else
            echo "   ❌ Migration init container failed with exit code: $INIT_EXIT_CODE"
            echo "   View logs: kubectl logs -n $NAMESPACE $BACKEND_POD -c goose"
        fi
    else
        echo "   ⚠️  Migration init container: $INIT_STATUS"
    fi
else
    echo "   ⚠️  Backend pod not found"
fi
echo ""

# Check Delve debug ports
echo "4. Checking Delve debug ports..."
if [ -n "$BACKEND_POD" ]; then
    DELVE_CHECK=$(kubectl exec -n $NAMESPACE $BACKEND_POD -- sh -c "nc -zv localhost 2345 2>&1" || true)
    if echo "$DELVE_CHECK" | grep -q "succeeded\|open"; then
        echo "   ✅ Backend Delve listening on :2345"
    else
        echo "   ⚠️  Backend Delve not accessible on :2345"
        echo "   Check logs: kubectl logs -n $NAMESPACE $BACKEND_POD"
    fi
else
    echo "   ⚠️  Cannot check - backend pod not found"
fi
echo ""

# Check port forwards (if Skaffold is running)
echo "5. Checking port forwards..."
FORWARDS=$(ps aux | grep "port-forward" | grep -v grep || true)
if [ -n "$FORWARDS" ]; then
    echo "   ✅ Port forwards active:"
    echo "$FORWARDS" | awk '{print "      PID " $2 ": " substr($0, index($0,$11))}'
else
    echo "   ⚠️  No active port forwards detected"
    echo "   Skaffold should automatically set up port forwards"
fi
echo ""

# Check PostgreSQL connectivity
echo "6. Checking PostgreSQL connectivity..."
PSQL_SVC=$(kubectl get svc postgresql -n $NAMESPACE --no-headers 2>/dev/null || true)
if [ -n "$PSQL_SVC" ]; then
    echo "   ✅ PostgreSQL service found"
else
    echo "   ❌ PostgreSQL service not found"
fi
echo ""

# Check SGROUPS connectivity
echo "7. Checking SGROUPS connectivity..."
SGROUPS_SVC=$(kubectl get svc sgroups-server -n $NAMESPACE --no-headers 2>/dev/null || true)
if [ -n "$SGROUPS_SVC" ]; then
    echo "   ✅ SGROUPS service found"
else
    echo "   ❌ SGROUPS service not found"
fi
echo ""

# Check ConfigMaps
echo "8. Checking required ConfigMaps..."
CONFIGS=("netguard-config" "netguard-environment" "netguard-environment-debug")
for config in "${CONFIGS[@]}"; do
    if kubectl get configmap $config -n $NAMESPACE >/dev/null 2>&1; then
        echo "   ✅ ConfigMap: $config"
    else
        echo "   ❌ ConfigMap missing: $config"
    fi
done
echo

# Check APIServer backend endpoint configuration
echo "9. Checking APIServer backend endpoint..."
APISERVER_POD=$(kubectl get pods -n $NAMESPACE -l app.kubernetes.io/name=apiserver,app.kubernetes.io/instance=netguard-debug --no-headers -o custom-columns=":metadata.name" 2>/dev/null | head -1)
if [ -n "$APISERVER_POD" ]; then
    BACKEND_ENDPOINT=$(kubectl exec -n $NAMESPACE $APISERVER_POD -- env 2>/dev/null | grep BACKEND_ENDPOINT || echo "")
    if echo "$BACKEND_ENDPOINT" | grep -q "netguard-backend-debug:9090"; then
        echo "   ✅ APIServer configured to use debug backend"
    else
        echo "   ❌ APIServer not configured for debug backend"
        echo "   Current: $BACKEND_ENDPOINT"
    fi
else
    echo "   ⚠️  APIServer debug pod not found"
fi
echo ""

echo "======================================"
echo "  Verification Complete"
echo "======================================"
echo ""
echo "Next steps:"
echo "  - Connect IDE to localhost:2345 (Backend)"
echo "  - Set breakpoints in your code"
echo "  - Test via: grpcurl -d '{...}' localhost:9090 netguard.v1.ServiceAPI/..."
echo ""
