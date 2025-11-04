#!/bin/bash

set -e

NAMESPACE="incloud-sgroups"

echo "======================================"
echo "  Switch to Production Infrastructure"
echo "======================================"
echo ""
echo "This script will redirect Services to production deployments."
echo ""
echo "Services to modify:"
echo "  - netguard-backend → netguard (production)"
echo "  - netguard-apiserver → netguard (production)"
echo "  - netguard-webhook → netguard (production)"
echo ""

# Skip confirmation if AUTO_CONFIRM is set (for automation)
if [[ -z "${AUTO_CONFIRM}" ]]; then
    read -p "Continue? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Aborted."
        exit 1
    fi
    echo ""
fi

# Function to patch a service
patch_service() {
    local SERVICE_NAME=$1
    local PROD_INSTANCE=$2

    echo "Patching service: $SERVICE_NAME → $PROD_INSTANCE..."

    kubectl patch service "$SERVICE_NAME" -n "$NAMESPACE" --type=json -p='[
        {
            "op": "replace",
            "path": "/spec/selector/app.kubernetes.io~1instance",
            "value": "'"$PROD_INSTANCE"'"
        }
    ]' 2>&1 | grep -v "no change" || true

    echo "  ✅ $SERVICE_NAME now points to $PROD_INSTANCE"
}

# Patch Backend Service
patch_service "netguard-backend" "netguard"

# Patch APIServer Service (if exists)
if kubectl get service netguard-apiserver -n "$NAMESPACE" >/dev/null 2>&1; then
    patch_service "netguard-apiserver" "netguard"
else
    echo "⚠️  Service netguard-apiserver not found, skipping..."
fi

# Patch Webhook Service (if exists)
if kubectl get service netguard-webhook -n "$NAMESPACE" >/dev/null 2>&1; then
    patch_service "netguard-webhook" "netguard"
else
    echo "⚠️  Service netguard-webhook not found, skipping..."
fi

echo ""
echo "======================================"
echo "  Switch Complete!"
echo "======================================"
echo ""
echo "Current service endpoints:"
kubectl get endpoints -n "$NAMESPACE" -l app.kubernetes.io/name=backend -o wide 2>/dev/null || true
kubectl get endpoints -n "$NAMESPACE" -l app.kubernetes.io/name=apiserver -o wide 2>/dev/null || true
kubectl get endpoints -n "$NAMESPACE" -l app.kubernetes.io/name=webhook -o wide 2>/dev/null || true
echo ""
echo "Traffic is now routed to production pods."
echo ""
