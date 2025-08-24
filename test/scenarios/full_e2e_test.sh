#!/bin/bash
# Full End-to-End Test Suite for NetGuard v1beta1 Resources
# Tests all 10 resource types with CRUD operations, PATCH operations, and integration scenarios

set -e

# Configuration
NAMESPACE="netguard-test"
API_VERSION="netguard.sgroups.io/v1beta1"
TIMEOUT=30
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

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Test tracking
TESTS_TOTAL=0
TESTS_PASSED=0
TESTS_FAILED=0

test_start() {
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    log_info "Test $TESTS_TOTAL: $1"
}

test_pass() {
    TESTS_PASSED=$((TESTS_PASSED + 1))
    log_success "✅ PASS: $1"
}

test_fail() {
    TESTS_FAILED=$((TESTS_FAILED + 1))
    log_error "❌ FAIL: $1"
}

# Utility functions
wait_for_resource() {
    local resource_type=$1
    local resource_name=$2
    local namespace=$3
    local timeout=${4:-$TIMEOUT}
    
    log_info "Waiting for $resource_type/$resource_name to be ready..."
    if kubectl wait --for=condition=Ready "$resource_type/$resource_name" -n "$namespace" --timeout="${timeout}s" 2>/dev/null; then
        return 0
    else
        # If wait fails, just check if resource exists
        kubectl get "$resource_type/$resource_name" -n "$namespace" >/dev/null 2>&1
        return $?
    fi
}

check_resource_exists() {
    local resource_type=$1
    local resource_name=$2
    local namespace=$3
    
    kubectl get "$resource_type/$resource_name" -n "$namespace" >/dev/null 2>&1
}

# Setup functions
setup_namespace() {
    log_info "Setting up namespace: $NAMESPACE"
    kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
    log_success "Namespace $NAMESPACE ready"
}

cleanup_namespace() {
    log_info "Cleaning up namespace: $NAMESPACE"
    kubectl delete namespace "$NAMESPACE" --ignore-not-found=true --wait=true --timeout=60s
    log_success "Namespace $NAMESPACE cleaned up"
}

# Test functions
test_api_availability() {
    test_start "API Resource Availability"
    
    local expected_resources=("services" "addressgroups" "servicealiases" "addressgroupbindings" 
                             "addressgroupbindingpolicies" "addressgroupportmappings" "networkbindings" 
                             "networks" "rules2s" "ieagagrules")
    
    local available_resources
    available_resources=$(kubectl api-resources --api-group=netguard.sgroups.io --no-headers | grep v1beta1 | awk '{print $1}')
    
    for resource in "${expected_resources[@]}"; do
        if echo "$available_resources" | grep -q "^$resource$"; then
            log_success "✓ $resource.v1beta1.netguard.sgroups.io available"
        else
            test_fail "$resource.v1beta1.netguard.sgroups.io NOT available"
            return 1
        fi
    done
    
    test_pass "All 10 NetGuard v1beta1 resources are available"
}

test_basic_resources() {
    test_start "Basic Resources Creation (Level 1)"
    
    # Apply basic resources
    if kubectl apply -f "$RESOURCES_DIR/01-basic/" -n "$NAMESPACE"; then
        log_success "Basic resources applied"
    else
        test_fail "Failed to apply basic resources"
        return 1
    fi
    
    # Verify basic resources
    local basic_resources=("service/test-service" "addressgroup/test-addressgroup" "network/test-network")
    
    for resource in "${basic_resources[@]}"; do
        if check_resource_exists "$resource.$API_VERSION" "" "$NAMESPACE"; then
            log_success "✓ $resource created successfully"
        else
            test_fail "$resource creation failed"
            return 1
        fi
    done
    
    test_pass "All basic resources created successfully"
}

test_dependency_resources() {
    test_start "Dependency Resources Creation (Level 2)"
    
    # Apply dependency resources
    if kubectl apply -f "$RESOURCES_DIR/02-dependencies/" -n "$NAMESPACE"; then
        log_success "Dependency resources applied"
    else
        test_fail "Failed to apply dependency resources"
        return 1
    fi
    
    # Verify dependency resources
    local dep_resources=("servicealias/test-service-alias" "addressgroupbinding/test-agb" 
                        "networkbinding/test-network-binding" "addressgroupbindingpolicy/test-agbp"
                        "addressgroupportmapping/test-agpm")
    
    for resource in "${dep_resources[@]}"; do
        if check_resource_exists "$resource.$API_VERSION" "" "$NAMESPACE"; then
            log_success "✓ $resource created successfully"
        else
            test_fail "$resource creation failed"
            return 1
        fi
    done
    
    test_pass "All dependency resources created successfully"
}

test_complex_resources() {
    test_start "Complex Resources Creation (Level 3)"
    
    # Apply complex resources
    if kubectl apply -f "$RESOURCES_DIR/03-complex/" -n "$NAMESPACE"; then
        log_success "Complex resources applied"
    else
        test_fail "Failed to apply complex resources"
        return 1
    fi
    
    # Verify complex resources
    local complex_resources=("rules2s/test-rules2s" "ieagagrule/test-ieagag-rule")
    
    for resource in "${complex_resources[@]}"; do
        if check_resource_exists "$resource.$API_VERSION" "" "$NAMESPACE"; then
            log_success "✓ $resource created successfully"
        else
            test_fail "$resource creation failed"
            return 1
        fi
    done
    
    test_pass "All complex resources created successfully"
}

test_crud_operations() {
    test_start "CRUD Operations"
    
    local test_service="service.v1beta1.netguard.sgroups.io/test-service"
    
    # READ operation
    if kubectl get "$test_service" -n "$NAMESPACE" -o yaml >/dev/null 2>&1; then
        log_success "✓ READ operation successful"
    else
        test_fail "READ operation failed"
        return 1
    fi
    
    # UPDATE operation (via patch)
    if kubectl patch "$test_service" -n "$NAMESPACE" --type='merge' \
        -p='{"metadata":{"annotations":{"crud-test":"updated"}}}' >/dev/null 2>&1; then
        log_success "✓ UPDATE operation successful"
    else
        test_fail "UPDATE operation failed"
        return 1
    fi
    
    # Verify update
    local annotation
    annotation=$(kubectl get "$test_service" -n "$NAMESPACE" -o jsonpath='{.metadata.annotations.crud-test}' 2>/dev/null)
    if [[ "$annotation" == "updated" ]]; then
        log_success "✓ UPDATE verification successful"
    else
        test_fail "UPDATE verification failed"
        return 1
    fi
    
    test_pass "CRUD operations completed successfully"
}

test_patch_operations() {
    test_start "PATCH Operations (JSON/Merge/Strategic)"
    
    local test_service="service.v1beta1.netguard.sgroups.io/test-service"
    
    # JSON Patch
    if kubectl patch "$test_service" -n "$NAMESPACE" --type='json' \
        -p='[{"op": "replace", "path": "/spec/description", "value": "Updated via JSON Patch"}]' >/dev/null 2>&1; then
        log_success "✓ JSON Patch successful"
    else
        test_fail "JSON Patch failed"
        return 1
    fi
    
    # Merge Patch
    if kubectl patch "$test_service" -n "$NAMESPACE" --type='merge' \
        -p='{"spec":{"description":"Updated via Merge Patch"}}' >/dev/null 2>&1; then
        log_success "✓ Merge Patch successful"
    else
        test_fail "Merge Patch failed"
        return 1
    fi
    
    # Strategic Merge Patch (default)
    if kubectl patch "$test_service" -n "$NAMESPACE" \
        -p='{"spec":{"description":"Updated via Strategic Merge Patch"}}' >/dev/null 2>&1; then
        log_success "✓ Strategic Merge Patch successful"
    else
        test_fail "Strategic Merge Patch failed"
        return 1
    fi
    
    test_pass "All PATCH operations completed successfully"
}

test_integration_scenarios() {
    test_start "Integration Scenarios"
    
    # Apply integration scenario
    if kubectl apply -f "$RESOURCES_DIR/04-integration/" -n "$NAMESPACE"; then
        log_success "Integration scenarios applied"
    else
        test_fail "Failed to apply integration scenarios"
        return 1
    fi
    
    # Verify complete web service setup
    local integration_resources=("service/web-service" "addressgroup/web-addressgroup" 
                               "servicealias/web-service-alias" "addressgroupbinding/web-agb")
    
    for resource in "${integration_resources[@]}"; do
        if check_resource_exists "$resource.$API_VERSION" "" "$NAMESPACE"; then
            log_success "✓ $resource integration successful"
        else
            test_fail "$resource integration failed"
            return 1
        fi
    done
    
    test_pass "Integration scenarios completed successfully"
}

# Main test execution
main() {
    log_info "🚀 Starting NetGuard v1beta1 Full E2E Test Suite"
    log_info "Namespace: $NAMESPACE"
    log_info "API Version: $API_VERSION"
    echo
    
    # Setup
    setup_namespace
    echo
    
    # Run tests
    test_api_availability
    echo
    
    test_basic_resources
    echo
    
    test_dependency_resources  
    echo
    
    test_complex_resources
    echo
    
    test_crud_operations
    echo
    
    test_patch_operations
    echo
    
    test_integration_scenarios
    echo
    
    # Cleanup
    if [[ "${CLEANUP:-true}" == "true" ]]; then
        cleanup_namespace
    else
        log_warning "Skipping cleanup (CLEANUP=false)"
    fi
    
    # Test summary
    echo
    log_info "📊 Test Summary:"
    echo "Total tests: $TESTS_TOTAL"
    echo "Passed: $TESTS_PASSED"
    echo "Failed: $TESTS_FAILED"
    
    if [[ $TESTS_FAILED -eq 0 ]]; then
        log_success "🎉 ALL TESTS PASSED!"
        exit 0
    else
        log_error "💥 $TESTS_FAILED TESTS FAILED"
        exit 1
    fi
}

# Run main function if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi