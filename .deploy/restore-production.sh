#!/bin/bash

set -e

NAMESPACE="incloud-sgroups"

echo "======================================"
echo "  Restore Production Infrastructure"
echo "======================================"
echo ""
echo "This script will restore production Services to their original state."
echo "All traffic will be routed back to production pods."
echo ""
echo "Services to restore:"
echo "  - netguard-backend → netguard"
echo "  - netguard-apiserver → netguard"
echo "  - netguard-webhook → netguard"
echo ""
read -p "Continue? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 1
fi
echo ""

# Function to patch a service back to production
patch_service() {
    local SERVICE_NAME=$1
    local PROD_INSTANCE=$2

    echo "Restoring service: $SERVICE_NAME → $PROD_INSTANCE..."

    kubectl patch service "$SERVICE_NAME" -n "$NAMESPACE" --type=json -p='[
        {
            "op": "replace",
            "path": "/spec/selector/app.kubernetes.io~1instance",
            "value": "'"$PROD_INSTANCE"'"
        }
    ]' 2>&1 | grep -v "no change" || true

    echo "  ✅ $SERVICE_NAME restored to $PROD_INSTANCE"
}

# Restore Backend Service
patch_service "netguard-backend" "netguard"

# Restore APIServer Service (if exists)
if kubectl get service netguard-apiserver -n "$NAMESPACE" >/dev/null 2>&1; then
    patch_service "netguard-apiserver" "netguard"
else
    echo "⚠️  Service netguard-apiserver not found, skipping..."
fi

# Restore Webhook Service (if exists)
if kubectl get service netguard-webhook -n "$NAMESPACE" >/dev/null 2>&1; then
    patch_service "netguard-webhook" "netguard"
else
    echo "⚠️  Service netguard-webhook not found, skipping..."
fi

echo ""
echo "======================================"
echo "  Restore Complete!"
echo "======================================"
echo ""
echo "Current service endpoints:"
kubectl get endpoints -n "$NAMESPACE" -l app.kubernetes.io/name=backend -o wide 2>/dev/null || true
kubectl get endpoints -n "$NAMESPACE" -l app.kubernetes.io/name=apiserver -o wide 2>/dev/null || true
kubectl get endpoints -n "$NAMESPACE" -l app.kubernetes.io/name=webhook -o wide 2>/dev/null || true
echo ""
echo "Production services restored."
echo "Debug deployments are still running but not receiving traffic."
echo ""
echo "To clean up debug deployments:"
echo "  kubectl delete deployment,service -l app.kubernetes.io/component=debug -n $NAMESPACE"
echo ""
