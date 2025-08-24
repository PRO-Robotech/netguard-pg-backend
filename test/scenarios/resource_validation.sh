#!/bin/bash
# Resource Validation Script for NetGuard v1beta1
# Validates that all resources can be created, read, updated, and deleted correctly

set -e

# Configuration
NAMESPACE="netguard-validation-test"
API_VERSION="netguard.sgroups.io/v1beta1"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOURCES_DIR="$SCRIPT_DIR/../resources"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Setup
setup_namespace() {
    log_info "Setting up validation namespace: $NAMESPACE"
    kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
}

cleanup_namespace() {
    log_info "Cleaning up validation namespace: $NAMESPACE"
    kubectl delete namespace "$NAMESPACE" --ignore-not-found=true --wait=true --timeout=60s
}

# Resource validation functions
validate_resource_crud() {
    local resource_type=$1
    local resource_name=$2
    local yaml_file=$3
    
    log_info "🔍 Validating $resource_type/$resource_name CRUD operations"
    
    # CREATE (apply with namespace substitution)
    if sed "s/namespace: netguard-test/namespace: $NAMESPACE/g" "$yaml_file" | kubectl apply -f - >/dev/null 2>&1; then
        log_success "✓ CREATE: $resource_type/$resource_name"
    else
        log_error "✗ CREATE FAILED: $resource_type/$resource_name"
        return 1
    fi
    
    # READ
    if kubectl get "$resource_type.v1beta1.netguard.sgroups.io/$resource_name" -n "$NAMESPACE" >/dev/null 2>&1; then
        log_success "✓ READ: $resource_type/$resource_name"
    else
        log_error "✗ READ FAILED: $resource_type/$resource_name"
        return 1
    fi
    
    # UPDATE (via patch)
    if kubectl patch "$resource_type.v1beta1.netguard.sgroups.io/$resource_name" -n "$NAMESPACE" --type='merge' \
        -p='{"metadata":{"annotations":{"validation-test":"updated"}}}' >/dev/null 2>&1; then
        log_success "✓ UPDATE: $resource_type/$resource_name"
    else
        log_error "✗ UPDATE FAILED: $resource_type/$resource_name"
        return 1
    fi
    
    # Verify ObjectReference fields are preserved (for resources that have them)
    local obj_refs
    obj_refs=$(kubectl get "$resource_type.v1beta1.netguard.sgroups.io/$resource_name" -n "$NAMESPACE" -o json | jq -r '
        .. | objects | select(has("apiVersion") and has("kind") and has("name")) | 
        select(.apiVersion == "netguard.sgroups.io/v1beta1") | 
        "\(.kind)/\(.name)"
    ' 2>/dev/null | wc -l)
    
    if [[ $obj_refs -gt 0 ]]; then
        log_success "✓ ObjectReference fields preserved: $obj_refs references found"
    fi
    
    # DELETE
    if kubectl delete "$resource_type.v1beta1.netguard.sgroups.io/$resource_name" -n "$NAMESPACE" --wait=true >/dev/null 2>&1; then
        log_success "✓ DELETE: $resource_type/$resource_name"
    else
        log_error "✗ DELETE FAILED: $resource_type/$resource_name"
        return 1
    fi
    
    log_success "✅ $resource_type/$resource_name CRUD validation complete"
    echo
    return 0
}

# Main validation
main() {
    log_info "🔍 Starting NetGuard Resource Validation"
    echo
    
    setup_namespace
    echo
    
    local validation_failed=0
    
    # Validate basic resources first (no dependencies)
    log_info "📝 Validating Level 1: Basic Resources"
    
    validate_resource_crud "service" "test-service" "$RESOURCES_DIR/01-basic/service.yaml" || validation_failed=1
    validate_resource_crud "addressgroup" "test-addressgroup" "$RESOURCES_DIR/01-basic/addressgroup.yaml" || validation_failed=1  
    validate_resource_crud "network" "test-network" "$RESOURCES_DIR/01-basic/network.yaml" || validation_failed=1
    
    # For dependency resources, we need to create prerequisites first
    log_info "📝 Validating Level 2: Dependency Resources"
    
    # Create prerequisites
    kubectl apply -f "$RESOURCES_DIR/01-basic/" -n "$NAMESPACE" >/dev/null 2>&1
    
    validate_resource_crud "servicealias" "test-service-alias" "$RESOURCES_DIR/02-dependencies/servicealias.yaml" || validation_failed=1
    validate_resource_crud "addressgroupbinding" "test-agb" "$RESOURCES_DIR/02-dependencies/addressgroupbinding.yaml" || validation_failed=1
    validate_resource_crud "networkbinding" "test-network-binding" "$RESOURCES_DIR/02-dependencies/networkbinding.yaml" || validation_failed=1
    validate_resource_crud "addressgroupbindingpolicy" "test-agbp" "$RESOURCES_DIR/02-dependencies/addressgroupbindingpolicy.yaml" || validation_failed=1
    validate_resource_crud "addressgroupportmapping" "test-agpm" "$RESOURCES_DIR/02-dependencies/addressgroupportmapping.yaml" || validation_failed=1
    
    # For complex resources, recreate all prerequisites
    log_info "📝 Validating Level 3: Complex Resources"
    
    # Recreate prerequisites (some may have been deleted)
    kubectl apply -f "$RESOURCES_DIR/01-basic/" -n "$NAMESPACE" >/dev/null 2>&1
    kubectl apply -f "$RESOURCES_DIR/02-dependencies/servicealias.yaml" -n "$NAMESPACE" >/dev/null 2>&1
    
    validate_resource_crud "rules2s" "test-rules2s" "$RESOURCES_DIR/03-complex/rules2s.yaml" || validation_failed=1
    validate_resource_crud "ieagagrule" "test-ieagag-rule" "$RESOURCES_DIR/03-complex/ieagagrule.yaml" || validation_failed=1
    
    # Cleanup
    cleanup_namespace
    
    # Summary
    echo
    if [[ $validation_failed -eq 0 ]]; then
        log_success "🎉 ALL RESOURCE VALIDATIONS PASSED!"
        log_info "✅ All 10 NetGuard v1beta1 resources support full CRUD operations"
        exit 0
    else
        log_error "💥 SOME VALIDATIONS FAILED"
        log_error "❌ Check the errors above for details"
        exit 1
    fi
}

# Check dependencies
check_dependencies() {
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is required but not installed"
        exit 1
    fi
    
    if ! command -v jq &> /dev/null; then
        log_error "jq is required but not installed (for ObjectReference validation)"
        exit 1
    fi
}

# Run main function if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    check_dependencies
    main "$@"
fi