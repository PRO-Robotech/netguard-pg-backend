#!/bin/bash

# Direct E2E Test - bypasses kubectl api-resources cache issues
# Tests all NetGuard v1beta1 resources using direct kubectl calls

set -euo pipefail

# Configuration
NAMESPACE="netguard-direct-test"
API_VERSION="netguard.sgroups.io/v1beta1"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m' # No Color

echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}${BLUE} 🚀 NetGuard v1beta1 Direct E2E Test Suite${NC}"
echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Test results tracking
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

record_test() {
    local test_name="$1"
    local success="$2"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    if [[ "$success" == "true" ]]; then
        PASSED_TESTS=$((PASSED_TESTS + 1))
        log_success "✅ PASS: $test_name"
    else
        FAILED_TESTS=$((FAILED_TESTS + 1))
        log_error "❌ FAIL: $test_name"
    fi
}

# Setup test namespace
log_info "Setting up namespace: $NAMESPACE"
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
log_success "Namespace $NAMESPACE ready"

# Test 1: Direct resource availability tests
log_info "Test 1: Direct Resource Availability"

# All 10 NetGuard v1beta1 resources
resources=(
    "services"
    "addressgroups" 
    "networks"
    "networkbindings"
    "rules2s"
    "ieagagrules"
    "servicealiases"
    "addressgroupbindings"
    "addressgroupportmappings"
    "addressgroupbindingpolicies"
)

for resource in "${resources[@]}"; do
    if kubectl get "${resource}.v1beta1.netguard.sgroups.io" -n "$NAMESPACE" --no-headers >/dev/null 2>&1; then
        record_test "${resource}.v1beta1.netguard.sgroups.io availability" true
    else
        record_test "${resource}.v1beta1.netguard.sgroups.io availability" false
    fi
done

# Test 2: Basic CRUD Operations
log_info "Test 2: Basic CRUD Operations"

# Test Service CRUD
log_info "Testing Service CRUD operations"
cat <<EOF | kubectl apply -f - >/dev/null 2>&1
apiVersion: $API_VERSION
kind: Service
metadata:
  name: direct-test-service
  namespace: $NAMESPACE
spec:
  description: "Direct test service"
  ingressPorts:
    - protocol: "TCP"
      port: "80"
EOF

if kubectl get services.v1beta1.netguard.sgroups.io direct-test-service -n "$NAMESPACE" >/dev/null 2>&1; then
    record_test "Service CREATE" true
    
    # Test UPDATE
    kubectl patch services.v1beta1.netguard.sgroups.io direct-test-service -n "$NAMESPACE" --type=merge -p='{"spec":{"description":"Updated description"}}' >/dev/null 2>&1
    record_test "Service UPDATE" true
    
    # Test DELETE
    kubectl delete services.v1beta1.netguard.sgroups.io direct-test-service -n "$NAMESPACE" >/dev/null 2>&1
    record_test "Service DELETE" true
else
    record_test "Service CREATE" false
    record_test "Service UPDATE" false
    record_test "Service DELETE" false
fi

# Test AddressGroup CRUD
log_info "Testing AddressGroup CRUD operations"
cat <<EOF | kubectl apply -f - >/dev/null 2>&1
apiVersion: $API_VERSION
kind: AddressGroup
metadata:
  name: direct-test-ag
  namespace: $NAMESPACE
spec:
  defaultAction: ACCEPT
  logs: false
  trace: false
EOF

if kubectl get addressgroups.v1beta1.netguard.sgroups.io direct-test-ag -n "$NAMESPACE" >/dev/null 2>&1; then
    record_test "AddressGroup CREATE" true
    
    # Test UPDATE
    kubectl patch addressgroups.v1beta1.netguard.sgroups.io direct-test-ag -n "$NAMESPACE" --type=merge -p='{"spec":{"logs":true}}' >/dev/null 2>&1
    record_test "AddressGroup UPDATE" true
    
    # Test DELETE
    kubectl delete addressgroups.v1beta1.netguard.sgroups.io direct-test-ag -n "$NAMESPACE" >/dev/null 2>&1
    record_test "AddressGroup DELETE" true
else
    record_test "AddressGroup CREATE" false
    record_test "AddressGroup UPDATE" false
    record_test "AddressGroup DELETE" false
fi

# Test Network CRUD
log_info "Testing Network CRUD operations"
cat <<EOF | kubectl apply -f - >/dev/null 2>&1
apiVersion: $API_VERSION
kind: Network
metadata:
  name: direct-test-network
  namespace: $NAMESPACE
spec:
  cidr: "192.168.100.0/24"
EOF

if kubectl get networks.v1beta1.netguard.sgroups.io direct-test-network -n "$NAMESPACE" >/dev/null 2>&1; then
    record_test "Network CREATE" true
    
    # Networks have immutable CIDR, so we test metadata update
    kubectl patch networks.v1beta1.netguard.sgroups.io direct-test-network -n "$NAMESPACE" --type=merge -p='{"metadata":{"annotations":{"test":"updated"}}}' >/dev/null 2>&1
    record_test "Network UPDATE" true
    
    # Test DELETE
    kubectl delete networks.v1beta1.netguard.sgroups.io direct-test-network -n "$NAMESPACE" >/dev/null 2>&1
    record_test "Network DELETE" true
else
    record_test "Network CREATE" false
    record_test "Network UPDATE" false
    record_test "Network DELETE" false
fi

# Test 3: Dependency Chain Creation
log_info "Test 3: Dependency Chain Creation"

# Create dependency chain: Service -> AddressGroup -> ServiceAlias -> RuleS2S
log_info "Creating dependency chain for complex resource test"

# Step 1: Create Service
cat <<EOF | kubectl apply -f - >/dev/null 2>&1
apiVersion: $API_VERSION
kind: Service
metadata:
  name: chain-service
  namespace: $NAMESPACE
spec:
  description: "Chain test service"
  ingressPorts:
    - protocol: "TCP"
      port: "80"
EOF

if kubectl get services.v1beta1.netguard.sgroups.io chain-service -n "$NAMESPACE" >/dev/null 2>&1; then
    record_test "Chain Step 1: Service creation" true
else
    record_test "Chain Step 1: Service creation" false
fi

# Step 2: Create AddressGroup
cat <<EOF | kubectl apply -f - >/dev/null 2>&1
apiVersion: $API_VERSION
kind: AddressGroup
metadata:
  name: chain-ag
  namespace: $NAMESPACE
spec:
  defaultAction: ACCEPT
  logs: false
  trace: false
EOF

if kubectl get addressgroups.v1beta1.netguard.sgroups.io chain-ag -n "$NAMESPACE" >/dev/null 2>&1; then
    record_test "Chain Step 2: AddressGroup creation" true
else
    record_test "Chain Step 2: AddressGroup creation" false
fi

# Step 3: Create ServiceAlias
cat <<EOF | kubectl apply -f - >/dev/null 2>&1
apiVersion: $API_VERSION
kind: ServiceAlias
metadata:
  name: chain-alias
  namespace: $NAMESPACE
spec:
  serviceRef:
    apiVersion: $API_VERSION
    kind: Service
    name: chain-service
EOF

if kubectl get servicealiases.v1beta1.netguard.sgroups.io chain-alias -n "$NAMESPACE" >/dev/null 2>&1; then
    record_test "Chain Step 3: ServiceAlias creation" true
else
    record_test "Chain Step 3: ServiceAlias creation" false
fi

# Step 4: Create RuleS2S
cat <<EOF | kubectl apply -f - >/dev/null 2>&1
apiVersion: $API_VERSION
kind: RuleS2S
metadata:
  name: chain-rule
  namespace: $NAMESPACE
spec:
  traffic: INGRESS
  serviceLocalRef:
    apiVersion: $API_VERSION
    kind: ServiceAlias
    name: chain-alias
  serviceRef:
    apiVersion: $API_VERSION
    kind: ServiceAlias  
    name: chain-alias
  trace: false
EOF

if kubectl get rules2s.v1beta1.netguard.sgroups.io chain-rule -n "$NAMESPACE" >/dev/null 2>&1; then
    record_test "Chain Step 4: RuleS2S creation" true
else
    record_test "Chain Step 4: RuleS2S creation" false
fi

# Test 4: Status Subresources
log_info "Test 4: Status Subresource Access"

# Test status endpoints for key resources
status_resources=("services" "addressgroups" "networks" "rules2s")

for resource in "${status_resources[@]}"; do
    # Get any resource of this type
    resource_name=$(kubectl get "${resource}.v1beta1.netguard.sgroups.io" -n "$NAMESPACE" --no-headers 2>/dev/null | head -1 | awk '{print $1}' || echo "")
    
    if [[ -n "$resource_name" ]]; then
        if kubectl get "${resource}.v1beta1.netguard.sgroups.io/${resource_name}" -n "$NAMESPACE" --subresource=status -o json >/dev/null 2>&1; then
            record_test "${resource}/status subresource access" true
        else
            record_test "${resource}/status subresource access" false
        fi
    else
        record_test "${resource}/status subresource access" false
    fi
done

# Cleanup
log_info "Cleaning up test namespace"
kubectl delete namespace "$NAMESPACE" --ignore-not-found=true >/dev/null 2>&1

# Test Results Summary
echo -e "\n${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}${BLUE} 📊 TEST RESULTS SUMMARY${NC}"
echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

echo -e "📝 Total Tests: $TOTAL_TESTS"
echo -e "${GREEN}✅ Passed: $PASSED_TESTS${NC}"
echo -e "${RED}❌ Failed: $FAILED_TESTS${NC}"

if [[ $TOTAL_TESTS -gt 0 ]]; then
    success_rate=$((PASSED_TESTS * 100 / TOTAL_TESTS))
    echo -e "📈 Success Rate: ${success_rate}%"
else
    success_rate=0
    echo -e "📈 Success Rate: 0%"
fi

if [[ $FAILED_TESTS -eq 0 ]]; then
    echo -e "\n${GREEN}🎉 ALL TESTS PASSED! NetGuard v1beta1 standardization is successful!${NC}"
    exit 0
else
    echo -e "\n${RED}💥 $FAILED_TESTS TEST(S) FAILED${NC}"
    echo -e "${RED}🔧 Review the failures above and investigate issues${NC}"
    exit 1
fi