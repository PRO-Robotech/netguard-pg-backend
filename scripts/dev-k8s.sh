#!/bin/bash

# Development script for k8s-apiserver
# Usage: ./scripts/dev-k8s.sh [command]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
NAMESPACE="netguard-system"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prereqs() {
    log_info "Checking prerequisites..."
    
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed"
        exit 1
    fi
    
    if ! command -v docker &> /dev/null; then
        log_error "docker is not installed"
        exit 1
    fi
    
    if ! command -v kustomize &> /dev/null; then
        log_warning "kustomize is not installed, using kubectl kustomize"
    fi
    
    log_success "Prerequisites check passed"
}

# Generate K8s code
generate() {
    log_info "Generating Kubernetes code..."
    cd "$PROJECT_ROOT"
    make generate-k8s
    log_success "Code generation completed"
}

# Build binary
build() {
    log_info "Building k8s-apiserver binary..."
    cd "$PROJECT_ROOT"
    make build-k8s-apiserver
    log_success "Binary built successfully"
}

# Build Docker image
build_image() {
    log_info "Building Docker image..."
    cd "$PROJECT_ROOT"
    make docker-build-k8s-apiserver
    log_success "Docker image built successfully"
}

# Deploy to Kubernetes
deploy() {
    log_info "Deploying to Kubernetes..."
    cd "$PROJECT_ROOT"
    
    # Create namespace if it doesn't exist
    kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
    
    # Apply manifests
    kubectl apply -k config/k8s/
    
    log_success "Deployment completed"
}

# Check deployment status
status() {
    log_info "Checking deployment status..."
    
    echo "Namespace:"
    kubectl get namespace "$NAMESPACE" 2>/dev/null || log_warning "Namespace not found"
    
    echo -e "\nAPIService:"
    kubectl get apiservice v1beta1.netguard.sgroups.io 2>/dev/null || log_warning "APIService not found"
    
    echo -e "\nPods:"
    kubectl get pods -n "$NAMESPACE" 2>/dev/null || log_warning "No pods found"
    
    echo -e "\nServices:"
    kubectl get services -n "$NAMESPACE" 2>/dev/null || log_warning "No services found"
    
    echo -e "\nWebhooks:"
    kubectl get validatingwebhookconfigurations netguard-validator 2>/dev/null || log_warning "Validating webhook not found"
    kubectl get mutatingwebhookconfigurations netguard-mutator 2>/dev/null || log_warning "Mutating webhook not found"
}

# Show logs
logs() {
    log_info "Showing logs..."
    kubectl logs -f deployment/netguard-apiserver -n "$NAMESPACE"
}

# Port forward for local access
port_forward() {
    log_info "Setting up port forwarding..."
    log_info "API Server will be available at https://localhost:8443"
    kubectl port-forward service/netguard-apiserver 8443:443 -n "$NAMESPACE"
}

# Test API
test_api() {
    log_info "Testing API..."
    
    # Check if API is available
    if kubectl api-resources --api-group=netguard.sgroups.io &> /dev/null; then
        log_success "API group is available"
        kubectl api-resources --api-group=netguard.sgroups.io
    else
        log_error "API group is not available"
        return 1
    fi
    
    # Try to create a test resource
    cat <<EOF | kubectl apply -f -
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: test-service
  namespace: default
spec:
  description: "Test service created by dev script"
  ingressPorts:
  - protocol: TCP
    port: "80"
    description: "HTTP port"
EOF
    
    if [ $? -eq 0 ]; then
        log_success "Test resource created successfully"
        kubectl get services.netguard.sgroups.io test-service -o yaml
        
        # Clean up
        kubectl delete services.netguard.sgroups.io test-service
        log_info "Test resource cleaned up"
    else
        log_error "Failed to create test resource"
    fi
}

# Clean up deployment
cleanup() {
    log_info "Cleaning up deployment..."
    cd "$PROJECT_ROOT"
    kubectl delete -k config/k8s/ --ignore-not-found=true
    log_success "Cleanup completed"
}

# Full development cycle
dev() {
    log_info "Running full development cycle..."
    check_prereqs
    generate
    build
    build_image
    deploy
    sleep 10
    status
    log_success "Development cycle completed"
}

# Show help
help() {
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  check      - Check prerequisites"
    echo "  generate   - Generate Kubernetes code"
    echo "  build      - Build binary"
    echo "  image      - Build Docker image"
    echo "  deploy     - Deploy to Kubernetes"
    echo "  status     - Check deployment status"
    echo "  logs       - Show logs"
    echo "  forward    - Port forward for local access"
    echo "  test       - Test API functionality"
    echo "  cleanup    - Clean up deployment"
    echo "  dev        - Run full development cycle"
    echo "  help       - Show this help"
    echo ""
    echo "Examples:"
    echo "  $0 dev           # Full development cycle"
    echo "  $0 deploy        # Just deploy"
    echo "  $0 logs          # Show logs"
    echo "  $0 test          # Test API"
}

# Main script logic
case "${1:-help}" in
    check)      check_prereqs ;;
    generate)   generate ;;
    build)      build ;;
    image)      build_image ;;
    deploy)     deploy ;;
    status)     status ;;
    logs)       logs ;;
    forward)    port_forward ;;
    test)       test_api ;;
    cleanup)    cleanup ;;
    dev)        dev ;;
    help|*)     help ;;
esac 