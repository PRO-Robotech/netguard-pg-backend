#!/bin/bash
set -euo pipefail

# setup-webhook-ca.sh - Automatically setup CA bundle for webhook configurations
# This script can be used instead of cert-manager if needed

NAMESPACE=${WEBHOOK_NAMESPACE:-netguard-system}
SECRET_NAME=${WEBHOOK_SECRET:-netguard-webhook-tls}
SERVICE_NAME=${WEBHOOK_SERVICE:-netguard-apiserver-webhook}

echo "🔐 Setting up CA Bundle for Netguard Webhooks"
echo "Namespace: $NAMESPACE"
echo "Secret: $SECRET_NAME"
echo "Service: $SERVICE_NAME"

# Function to wait for secret
wait_for_secret() {
    echo "⏳ Waiting for TLS secret $SECRET_NAME in namespace $NAMESPACE..."
    local timeout=300
    local count=0
    
    while ! kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" >/dev/null 2>&1; do
        if [ $count -ge $timeout ]; then
            echo "❌ Timeout waiting for secret $SECRET_NAME"
            exit 1
        fi
        sleep 2
        count=$((count + 2))
    done
    echo "✅ Secret $SECRET_NAME found"
}

# Function to extract CA bundle
extract_ca_bundle() {
    echo "📜 Extracting CA bundle from secret..."
    local ca_bundle
    ca_bundle=$(kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" -o jsonpath='{.data.tls\.crt}')
    
    if [ -z "$ca_bundle" ]; then
        echo "❌ Failed to extract CA bundle from secret $SECRET_NAME"
        exit 1
    fi
    
    echo "✅ CA bundle extracted successfully"
    echo "$ca_bundle"
}

# Function to patch webhook configuration
patch_webhook_config() {
    local config_type="$1"
    local config_name="$2"
    local ca_bundle="$3"
    
    echo "🔧 Patching $config_type webhook configuration $config_name..."
    
    # Create JSON patch for all webhooks in the configuration
    local webhooks_count
    webhooks_count=$(kubectl get "$config_type" "$config_name" -o jsonpath='{.webhooks[*].name}' | wc -w)
    
    echo "   Found $webhooks_count webhooks to patch"
    
    # Build JSON patch array for all webhooks
    local patch_json="["
    for ((i=0; i<webhooks_count; i++)); do
        if [ $i -gt 0 ]; then
            patch_json="$patch_json,"
        fi
        patch_json="$patch_json{\"op\": \"replace\", \"path\": \"/webhooks/$i/clientConfig/caBundle\", \"value\": \"$ca_bundle\"}"
    done
    patch_json="$patch_json]"
    
    # Apply the patch
    if kubectl patch "$config_type" "$config_name" --type='json' -p="$patch_json"; then
        echo "✅ Successfully patched $config_type $config_name"
    else
        echo "❌ Failed to patch $config_type $config_name"
        return 1
    fi
}

# Main execution
main() {
    echo "🚀 Starting webhook CA setup process..."
    
    # Check if kubectl is available
    if ! command -v kubectl >/dev/null 2>&1; then
        echo "❌ kubectl is required but not installed"
        exit 1
    fi
    
    # Check if namespace exists
    if ! kubectl get namespace "$NAMESPACE" >/dev/null 2>&1; then
        echo "❌ Namespace $NAMESPACE does not exist"
        exit 1
    fi
    
    # Wait for the TLS secret to be created (by cert-manager or manual process)
    wait_for_secret
    
    # Extract CA bundle
    local ca_bundle
    ca_bundle=$(extract_ca_bundle)
    
    # Patch validating webhook configuration
    if kubectl get validatingwebhookconfiguration netguard-validator >/dev/null 2>&1; then
        patch_webhook_config "validatingwebhookconfiguration" "netguard-validator" "$ca_bundle"
    else
        echo "⚠️  ValidatingWebhookConfiguration netguard-validator not found"
    fi
    
    # Patch mutating webhook configuration
    if kubectl get mutatingwebhookconfiguration netguard-mutator >/dev/null 2>&1; then
        patch_webhook_config "mutatingwebhookconfiguration" "netguard-mutator" "$ca_bundle"
    else
        echo "⚠️  MutatingWebhookConfiguration netguard-mutator not found"
    fi
    
    echo "🎉 Webhook CA setup completed successfully!"
    echo ""
    echo "📋 Next steps:"
    echo "   1. Verify webhook configurations: kubectl get validatingwebhookconfiguration,mutatingwebhookconfiguration"
    echo "   2. Test webhook functionality by creating a sample resource"
    echo "   3. Check webhook server logs: kubectl logs -n $NAMESPACE deployment/netguard-apiserver -c webhook"
}

# Handle script arguments
case "${1:-main}" in
    "extract-ca")
        wait_for_secret
        extract_ca_bundle
        ;;
    "patch-validating")
        ca_bundle=$(extract_ca_bundle)
        patch_webhook_config "validatingwebhookconfiguration" "netguard-validator" "$ca_bundle"
        ;;
    "patch-mutating")
        ca_bundle=$(extract_ca_bundle)
        patch_webhook_config "mutatingwebhookconfiguration" "netguard-mutator" "$ca_bundle"
        ;;
    "main"|*)
        main
        ;;
esac 