#!/bin/bash
# PATCH Operations Validation Script for NetGuard v1beta1
# Tests JSON Patch, Merge Patch, and Strategic Merge Patch operations

set -e

# Configuration
NAMESPACE="netguard-patch-test"
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
    log_info "Setting up patch test namespace: $NAMESPACE"
    kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
    
    # Create test resources
    kubectl apply -f "$RESOURCES_DIR/01-basic/" -n "$NAMESPACE"
    kubectl apply -f "$RESOURCES_DIR/02-dependencies/" -n "$NAMESPACE"
    
    log_success "Test resources created"
}

cleanup_namespace() {
    log_info "Cleaning up patch test namespace: $NAMESPACE"
    kubectl delete namespace "$NAMESPACE" --ignore-not-found=true --wait=true --timeout=60s
}

# PATCH testing functions
test_json_patch() {
    local resource_type=$1
    local resource_name=$2
    
    log_info "🔧 Testing JSON Patch on $resource_type/$resource_name"
    
    # JSON Patch - Replace description
    local json_patch='[{"op": "replace", "path": "/spec/description", "value": "Updated via JSON Patch"}]'
    
    if kubectl patch "$resource_type.$API_VERSION/$resource_name" -n "$NAMESPACE" --type='json' -p="$json_patch" >/dev/null 2>&1; then
        log_success "✓ JSON Patch applied successfully"
        
        # Verify the change
        local description
        description=$(kubectl get "$resource_type.$API_VERSION/$resource_name" -n "$NAMESPACE" -o jsonpath='{.spec.description}' 2>/dev/null)
        if [[ "$description" == "Updated via JSON Patch" ]]; then
            log_success "✓ JSON Patch verification successful"
            return 0
        else
            log_error "✗ JSON Patch verification failed: expected 'Updated via JSON Patch', got '$description'"
            return 1
        fi
    else
        log_error "✗ JSON Patch failed"
        return 1
    fi
}

test_merge_patch() {
    local resource_type=$1
    local resource_name=$2
    
    log_info "🔧 Testing Merge Patch on $resource_type/$resource_name"
    
    # Merge Patch - Update description and add label
    local merge_patch='{"metadata":{"labels":{"patch-test":"merge"}},"spec":{"description":"Updated via Merge Patch"}}'
    
    if kubectl patch "$resource_type.$API_VERSION/$resource_name" -n "$NAMESPACE" --type='merge' -p="$merge_patch" >/dev/null 2>&1; then
        log_success "✓ Merge Patch applied successfully"
        
        # Verify the changes
        local description label
        description=$(kubectl get "$resource_type.$API_VERSION/$resource_name" -n "$NAMESPACE" -o jsonpath='{.spec.description}' 2>/dev/null)
        label=$(kubectl get "$resource_type.$API_VERSION/$resource_name" -n "$NAMESPACE" -o jsonpath='{.metadata.labels.patch-test}' 2>/dev/null)
        
        if [[ "$description" == "Updated via Merge Patch" ]] && [[ "$label" == "merge" ]]; then
            log_success "✓ Merge Patch verification successful"
            return 0
        else
            log_error "✗ Merge Patch verification failed"
            log_error "  Description: expected 'Updated via Merge Patch', got '$description'"
            log_error "  Label: expected 'merge', got '$label'"
            return 1
        fi
    else
        log_error "✗ Merge Patch failed"
        return 1
    fi
}

test_strategic_merge_patch() {
    local resource_type=$1
    local resource_name=$2
    
    log_info "🔧 Testing Strategic Merge Patch on $resource_type/$resource_name"
    
    # Strategic Merge Patch (default) - Update description and add annotation
    local strategic_patch='{"metadata":{"annotations":{"patch-test":"strategic"}},"spec":{"description":"Updated via Strategic Merge Patch"}}'
    
    if kubectl patch "$resource_type.$API_VERSION/$resource_name" -n "$NAMESPACE" -p="$strategic_patch" >/dev/null 2>&1; then
        log_success "✓ Strategic Merge Patch applied successfully"
        
        # Verify the changes
        local description annotation
        description=$(kubectl get "$resource_type.$API_VERSION/$resource_name" -n "$NAMESPACE" -o jsonpath='{.spec.description}' 2>/dev/null)
        annotation=$(kubectl get "$resource_type.$API_VERSION/$resource_name" -n "$NAMESPACE" -o jsonpath='{.metadata.annotations.patch-test}' 2>/dev/null)
        
        if [[ "$description" == "Updated via Strategic Merge Patch" ]] && [[ "$annotation" == "strategic" ]]; then
            log_success "✓ Strategic Merge Patch verification successful"
            return 0
        else
            log_error "✗ Strategic Merge Patch verification failed"
            log_error "  Description: expected 'Updated via Strategic Merge Patch', got '$description'"
            log_error "  Annotation: expected 'strategic', got '$annotation'"
            return 1
        fi
    else
        log_error "✗ Strategic Merge Patch failed"
        return 1
    fi
}

test_objectreference_preservation() {
    local resource_type=$1
    local resource_name=$2
    
    log_info "🔧 Testing ObjectReference preservation on $resource_type/$resource_name"
    
    # Get ObjectReferences before patch
    local refs_before
    refs_before=$(kubectl get "$resource_type.$API_VERSION/$resource_name" -n "$NAMESPACE" -o json | jq -r '
        .. | objects | select(has("apiVersion") and has("kind") and has("name")) | 
        select(.apiVersion == "netguard.sgroups.io/v1beta1") | 
        "\(.apiVersion):\(.kind):\(.name)"
    ' 2>/dev/null | sort)
    
    # Apply patch
    local patch='{"metadata":{"annotations":{"objectref-test":"true"}}}'
    kubectl patch "$resource_type.$API_VERSION/$resource_name" -n "$NAMESPACE" --type='merge' -p="$patch" >/dev/null 2>&1
    
    # Get ObjectReferences after patch
    local refs_after
    refs_after=$(kubectl get "$resource_type.$API_VERSION/$resource_name" -n "$NAMESPACE" -o json | jq -r '
        .. | objects | select(has("apiVersion") and has("kind") and has("name")) | 
        select(.apiVersion == "netguard.sgroups.io/v1beta1") | 
        "\(.apiVersion):\(.kind):\(.name)"
    ' 2>/dev/null | sort)
    
    if [[ "$refs_before" == "$refs_after" ]] && [[ -n "$refs_before" ]]; then
        local count
        count=$(echo "$refs_before" | wc -l)
        log_success "✓ ObjectReference fields preserved ($count references)"
        return 0
    elif [[ -z "$refs_before" ]]; then
        log_success "✓ No ObjectReference fields to preserve (expected for basic resources)"
        return 0
    else
        log_error "✗ ObjectReference preservation failed"
        log_error "  Before: $refs_before"
        log_error "  After:  $refs_after"
        return 1
    fi
}

# Test all patch types on a resource
test_all_patches() {
    local resource_type=$1
    local resource_name=$2
    
    log_info "🎯 Testing all PATCH operations on $resource_type/$resource_name"
    
    local patch_failed=0
    
    test_json_patch "$resource_type" "$resource_name" || patch_failed=1
    test_merge_patch "$resource_type" "$resource_name" || patch_failed=1  
    test_strategic_merge_patch "$resource_type" "$resource_name" || patch_failed=1
    test_objectreference_preservation "$resource_type" "$resource_name" || patch_failed=1
    
    if [[ $patch_failed -eq 0 ]]; then
        log_success "✅ All PATCH operations successful on $resource_type/$resource_name"
    else
        log_error "❌ Some PATCH operations failed on $resource_type/$resource_name"
    fi
    
    echo
    return $patch_failed
}

# Main test execution
main() {
    log_info "🔧 Starting NetGuard PATCH Operations Validation"
    echo
    
    setup_namespace
    echo
    
    local total_failed=0
    
    # Test PATCH operations on different resource types
    log_info "📝 Testing PATCH operations on basic resources"
    test_all_patches "service" "test-service" || total_failed=1
    test_all_patches "addressgroup" "test-addressgroup" || total_failed=1
    test_all_patches "network" "test-network" || total_failed=1
    
    log_info "📝 Testing PATCH operations on dependency resources"
    test_all_patches "servicealias" "test-service-alias" || total_failed=1
    test_all_patches "addressgroupbinding" "test-agb" || total_failed=1
    test_all_patches "networkbinding" "test-network-binding" || total_failed=1
    
    # Cleanup
    cleanup_namespace
    
    # Summary
    echo
    if [[ $total_failed -eq 0 ]]; then
        log_success "🎉 ALL PATCH OPERATIONS VALIDATED SUCCESSFULLY!"
        log_info "✅ JSON Patch, Merge Patch, and Strategic Merge Patch all work correctly"
        log_info "✅ ObjectReference fields are properly preserved during PATCH operations"
        exit 0
    else
        log_error "💥 SOME PATCH OPERATIONS FAILED"
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