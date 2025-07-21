#!/bin/bash

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
OLD_NAMESPACE="netguard-test"
NEW_NAMESPACE="netguard-system"
MINIKUBE_PROFILE="incloud"

echo -e "${BLUE}🚀 Starting Netguard cleanup and migration process...${NC}"
echo -e "${BLUE}============================================${NC}"
echo "From: ${OLD_NAMESPACE}"
echo "To: ${NEW_NAMESPACE}"
echo "Minikube profile: ${MINIKUBE_PROFILE}"
echo ""

# Function to print status
print_status() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_info() {
    echo -e "${BLUE}📋 $1${NC}"
}

# Function to check if command exists
check_command() {
    if ! command -v "$1" >/dev/null 2>&1; then
        print_error "Command '$1' is required but not installed"
        exit 1
    fi
}

# Function to wait for user confirmation
confirm() {
    read -p "$(echo -e "${YELLOW}$1 [y/N]:${NC} ")" -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_info "Operation cancelled by user"
        exit 0
    fi
}

# Verify prerequisites
verify_prerequisites() {
    print_info "Verifying prerequisites..."
    
    check_command kubectl
    check_command minikube
    check_command docker
    
    # Check if we're connected to the right cluster
    current_context=$(kubectl config current-context)
    if [[ "$current_context" != "$MINIKUBE_PROFILE" ]]; then
        print_error "Current kubectl context is '$current_context', expected '$MINIKUBE_PROFILE'"
        print_info "Available contexts:"
        kubectl config get-contexts
        exit 1
    fi
    
    print_status "Prerequisites verified"
}

# Backup current state
backup_current_state() {
    print_info "Backing up current state..."
    
    local backup_dir="backup-$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$backup_dir"
    
    # Backup namespace resources
    if kubectl get namespace "$OLD_NAMESPACE" >/dev/null 2>&1; then
        kubectl get all,secrets,configmaps,pvc -n "$OLD_NAMESPACE" -o yaml > "$backup_dir/namespace-$OLD_NAMESPACE.yaml"
    fi
    
    # Backup webhook configurations
    kubectl get validatingwebhookconfiguration,mutatingwebhookconfiguration -o yaml > "$backup_dir/webhook-configurations.yaml" 2>/dev/null || true
    
    # Backup API service registrations
    kubectl get apiservices -o yaml > "$backup_dir/apiservices.yaml" 2>/dev/null || true
    
    print_status "Backup created in $backup_dir"
}

# Clean up old webhook configurations
cleanup_webhooks() {
    print_info "Cleaning up webhook configurations..."
    
    # List of webhook configurations to delete
    local webhooks=(
        "netguard-validator"
        "netguard-mutator"
        "netguard-netguard-validatingwebhookconfiguration"
    )
    
    for webhook in "${webhooks[@]}"; do
        if kubectl get validatingwebhookconfiguration "$webhook" >/dev/null 2>&1; then
            kubectl delete validatingwebhookconfiguration "$webhook" || true
            print_status "Deleted validatingwebhookconfiguration: $webhook"
        fi
        
        if kubectl get mutatingwebhookconfiguration "$webhook" >/dev/null 2>&1; then
            kubectl delete mutatingwebhookconfiguration "$webhook" || true
            print_status "Deleted mutatingwebhookconfiguration: $webhook"
        fi
    done
}

# Clean up API service registrations
cleanup_apiservices() {
    print_info "Cleaning up API service registrations..."
    
    # List of API services to delete
    local apiservices=(
        "v1beta1.netguard.sgroups.io"
        "v1alpha1.netguard.sgroups.io"
    )
    
    for apiservice in "${apiservices[@]}"; do
        if kubectl get apiservice "$apiservice" >/dev/null 2>&1; then
            kubectl delete apiservice "$apiservice" || true
            print_status "Deleted apiservice: $apiservice"
        fi
    done
}

# Clean up old namespace
cleanup_old_namespace() {
    print_info "Cleaning up old namespace: $OLD_NAMESPACE"
    
    if kubectl get namespace "$OLD_NAMESPACE" >/dev/null 2>&1; then
        # Delete all resources in the namespace
        kubectl delete all --all -n "$OLD_NAMESPACE" --timeout=60s || true
        
        # Delete secrets and configmaps
        kubectl delete secrets --all -n "$OLD_NAMESPACE" --timeout=60s || true
        kubectl delete configmaps --all -n "$OLD_NAMESPACE" --timeout=60s || true
        
        # Delete the namespace itself
        kubectl delete namespace "$OLD_NAMESPACE" --timeout=60s || true
        
        print_status "Deleted namespace: $OLD_NAMESPACE"
    else
        print_warning "Namespace $OLD_NAMESPACE not found"
    fi
}

# Create new namespace
create_new_namespace() {
    print_info "Creating new namespace: $NEW_NAMESPACE"
    
    if kubectl get namespace "$NEW_NAMESPACE" >/dev/null 2>&1; then
        print_warning "Namespace $NEW_NAMESPACE already exists"
        confirm "Do you want to delete and recreate it?"
        kubectl delete namespace "$NEW_NAMESPACE" --timeout=60s || true
    fi
    
    kubectl create namespace "$NEW_NAMESPACE"
    print_status "Created namespace: $NEW_NAMESPACE"
}

# Deploy to new namespace
deploy_to_new_namespace() {
    print_info "Deploying to new namespace: $NEW_NAMESPACE"
    
    # Apply all configurations
    kubectl apply -f config/k8s/namespace.yaml || true
    kubectl apply -f config/k8s/rbac.yaml
    kubectl apply -f config/k8s/configmap.yaml
    kubectl apply -f config/k8s/deployment.yaml
    kubectl apply -f config/k8s/backend-deployment.yaml
    
    print_status "Deployed base resources to $NEW_NAMESPACE"
}

# Deploy webhook configurations
deploy_webhooks() {
    print_info "Deploying webhook configurations..."
    
    # Apply webhook configurations
    kubectl apply -f config/k8s/validating-webhook.yaml
    kubectl apply -f config/k8s/mutating-webhook.yaml
    
    print_status "Deployed webhook configurations"
}

# Wait for deployments to be ready
wait_for_deployments() {
    print_info "Waiting for deployments to be ready..."
    
    local deployments=(
        "netguard-apiserver"
        "netguard-backend"
    )
    
    for deployment in "${deployments[@]}"; do
        print_info "Waiting for deployment/$deployment to be ready..."
        kubectl wait --for=condition=Available deployment/$deployment -n "$NEW_NAMESPACE" --timeout=300s || {
            print_error "Deployment $deployment failed to become ready"
            kubectl get pods -n "$NEW_NAMESPACE" -l app=$deployment
            kubectl logs -n "$NEW_NAMESPACE" deployment/$deployment --tail=50 || true
            return 1
        }
        print_status "Deployment $deployment is ready"
    done
}

# Verify deployment
verify_deployment() {
    print_info "Verifying deployment..."
    
    # Check pods
    print_info "Checking pods in $NEW_NAMESPACE:"
    kubectl get pods -n "$NEW_NAMESPACE"
    
    # Check services
    print_info "Checking services in $NEW_NAMESPACE:"
    kubectl get services -n "$NEW_NAMESPACE"
    
    # Check API service registration
    print_info "Checking API service registration:"
    kubectl get apiservices | grep netguard || true
    
    # Check webhook configurations
    print_info "Checking webhook configurations:"
    kubectl get validatingwebhookconfiguration,mutatingwebhookconfiguration | grep netguard || true
    
    print_status "Deployment verification completed"
}

# Update deployment scripts
update_deployment_scripts() {
    print_info "Updating deployment scripts..."
    
    # Update redeploy-apiserver.sh
    sed -i.bak "s/NAMESPACE=\"netguard-test\"/NAMESPACE=\"netguard-system\"/g" scripts/redeploy-apiserver.sh
    print_status "Updated scripts/redeploy-apiserver.sh"
    
    # Update redeploy-backend.sh
    sed -i.bak "s/NAMESPACE=\"netguard-test\"/NAMESPACE=\"netguard-system\"/g" scripts/redeploy-backend.sh
    print_status "Updated scripts/redeploy-backend.sh"
    
    # Make scripts executable
    chmod +x scripts/redeploy-apiserver.sh scripts/redeploy-backend.sh
    
    print_status "Updated deployment scripts"
}

# Test the deployment
test_deployment() {
    print_info "Testing deployment..."
    
    # Wait a bit for services to be fully ready
    sleep 10
    
    # Test API server health
    print_info "Testing API server health..."
    kubectl exec -n "$NEW_NAMESPACE" deployment/netguard-apiserver -c apiserver -- curl -k https://localhost:8443/healthz || {
        print_warning "API server health check failed (this might be normal during startup)"
    }
    
    # Test backend health
    print_info "Testing backend health..."
    kubectl exec -n "$NEW_NAMESPACE" deployment/netguard-backend -- nc -z localhost 9090 || {
        print_warning "Backend health check failed (this might be normal during startup)"
    }
    
    print_status "Deployment tests completed"
}

# Main execution
main() {
    print_info "Starting cleanup and migration process..."
    
    verify_prerequisites
    
    print_warning "This will:"
    print_warning "1. Delete ALL resources in namespace: $OLD_NAMESPACE"
    print_warning "2. Delete webhook configurations: netguard-validator, netguard-mutator"
    print_warning "3. Delete API service registrations"
    print_warning "4. Create new namespace: $NEW_NAMESPACE"
    print_warning "5. Deploy all components to: $NEW_NAMESPACE"
    print_warning "6. Update deployment scripts"
    print_warning ""
    confirm "Are you sure you want to proceed?"
    
    backup_current_state
    
    cleanup_webhooks
    cleanup_apiservices
    cleanup_old_namespace
    
    create_new_namespace
    deploy_to_new_namespace
    wait_for_deployments
    deploy_webhooks
    
    verify_deployment
    update_deployment_scripts
    test_deployment
    
    print_status "Migration completed successfully!"
    print_info ""
    print_info "📋 Next steps:"
    print_info "1. Use scripts/redeploy-apiserver.sh for API server redeployment"
    print_info "2. Use scripts/redeploy-backend.sh for backend redeployment"
    print_info "3. Test API functionality: kubectl api-resources --api-group=netguard.sgroups.io"
    print_info "4. Check logs: kubectl logs -n $NEW_NAMESPACE deployment/netguard-apiserver"
    print_info "5. Verify webhooks: kubectl get validatingwebhookconfiguration,mutatingwebhookconfiguration"
}

# Handle script arguments
case "${1:-main}" in
    "cleanup-only")
        verify_prerequisites
        confirm "Are you sure you want to clean up $OLD_NAMESPACE?"
        backup_current_state
        cleanup_webhooks
        cleanup_apiservices
        cleanup_old_namespace
        ;;
    "deploy-only")
        verify_prerequisites
        create_new_namespace
        deploy_to_new_namespace
        wait_for_deployments
        deploy_webhooks
        verify_deployment
        ;;
    "test-only")
        verify_prerequisites
        test_deployment
        ;;
    "main"|*)
        main
        ;;
esac 