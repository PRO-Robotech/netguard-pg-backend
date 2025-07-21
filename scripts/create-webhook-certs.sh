#!/bin/bash

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

NAMESPACE="netguard-system"
SECRET_NAME="netguard-webhook-tls"
SERVICE_NAME="netguard-apiserver-webhook"

echo -e "${BLUE}🔐 Creating TLS certificates for Netguard webhook...${NC}"
echo "Namespace: $NAMESPACE"
echo "Secret: $SECRET_NAME"
echo "Service: $SERVICE_NAME"
echo ""

# Function to print status
print_status() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_info() {
    echo -e "${BLUE}📋 $1${NC}"
}

# Create temporary directory for certificates
TEMP_DIR=$(mktemp -d)
trap "rm -rf $TEMP_DIR" EXIT

cd "$TEMP_DIR"

# Create certificate configuration
cat > webhook.conf <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = ${SERVICE_NAME}.${NAMESPACE}.svc

[v3_req]
keyUsage = keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = ${SERVICE_NAME}
DNS.2 = ${SERVICE_NAME}.${NAMESPACE}
DNS.3 = ${SERVICE_NAME}.${NAMESPACE}.svc
DNS.4 = ${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local
EOF

print_info "Creating private key and certificate..."

# Generate private key
openssl genrsa -out tls.key 2048

# Generate certificate signing request
openssl req -new -key tls.key -out webhook.csr -config webhook.conf

# Generate self-signed certificate
openssl x509 -req -in webhook.csr -signkey tls.key -out tls.crt -days 365 -extensions v3_req -extfile webhook.conf

print_status "Certificates created successfully"

# Verify certificate
print_info "Certificate details:"
openssl x509 -in tls.crt -text -noout | grep -A 5 "Subject Alternative Name" || true

# Create or update Kubernetes secret
print_info "Creating Kubernetes secret..."

if kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" >/dev/null 2>&1; then
    print_info "Secret already exists, updating..."
    kubectl delete secret "$SECRET_NAME" -n "$NAMESPACE"
fi

kubectl create secret tls "$SECRET_NAME" \
    --cert=tls.crt \
    --key=tls.key \
    -n "$NAMESPACE"

print_status "Secret '$SECRET_NAME' created in namespace '$NAMESPACE'"

# Update webhook configurations with CA bundle
print_info "Updating webhook configurations with CA bundle..."

CA_BUNDLE=$(base64 < tls.crt | tr -d '\n')

# Update validating webhook
if kubectl get validatingwebhookconfiguration netguard-validator >/dev/null 2>&1; then
    webhooks_count=$(kubectl get validatingwebhookconfiguration netguard-validator -o jsonpath='{.webhooks[*].name}' | wc -w)
    print_info "Updating $webhooks_count validating webhooks..."
    
    for ((i=0; i<webhooks_count; i++)); do
        kubectl patch validatingwebhookconfiguration netguard-validator \
            --type='json' \
            -p="[{\"op\": \"replace\", \"path\": \"/webhooks/$i/clientConfig/caBundle\", \"value\": \"$CA_BUNDLE\"}]"
    done
    print_status "Updated validating webhook configuration"
else
    print_info "Validating webhook configuration not found, skipping..."
fi

# Update mutating webhook
if kubectl get mutatingwebhookconfiguration netguard-mutator >/dev/null 2>&1; then
    webhooks_count=$(kubectl get mutatingwebhookconfiguration netguard-mutator -o jsonpath='{.webhooks[*].name}' | wc -w)
    print_info "Updating $webhooks_count mutating webhooks..."
    
    for ((i=0; i<webhooks_count; i++)); do
        kubectl patch mutatingwebhookconfiguration netguard-mutator \
            --type='json' \
            -p="[{\"op\": \"replace\", \"path\": \"/webhooks/$i/clientConfig/caBundle\", \"value\": \"$CA_BUNDLE\"}]"
    done
    print_status "Updated mutating webhook configuration"
else
    print_info "Mutating webhook configuration not found, skipping..."
fi

print_status "TLS certificates setup completed!"
print_info ""
print_info "📋 Next steps:"
print_info "1. Restart API server deployment: kubectl rollout restart deployment/netguard-apiserver -n $NAMESPACE"
print_info "2. Check pod status: kubectl get pods -n $NAMESPACE"
print_info "3. Check logs: kubectl logs -n $NAMESPACE deployment/netguard-apiserver -c webhook"
print_info ""
print_info "🔍 Verification commands:"
print_info "kubectl get secret $SECRET_NAME -n $NAMESPACE"
print_info "kubectl describe secret $SECRET_NAME -n $NAMESPACE" 