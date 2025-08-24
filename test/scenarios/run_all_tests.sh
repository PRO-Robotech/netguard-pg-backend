#!/bin/bash
# Master test runner for NetGuard E2E Testing
# Runs all test scenarios in sequence with proper reporting

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
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

log_header() {
    echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BOLD}${BLUE} $1${NC}"
    echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

# Test tracking
SUITE_START_TIME=$(date +%s)
TESTS_PASSED=0
TESTS_FAILED=0
FAILED_TESTS=()

run_test_script() {
    local script_name=$1
    local script_path="$SCRIPT_DIR/$script_name"
    local description=$2
    
    log_header "$description"
    echo
    
    local start_time=$(date +%s)
    
    if [[ -f "$script_path" ]]; then
        if bash "$script_path"; then
            local end_time=$(date +%s)
            local duration=$((end_time - start_time))
            TESTS_PASSED=$((TESTS_PASSED + 1))
            log_success "✅ $description completed successfully (${duration}s)"
        else
            local end_time=$(date +%s)
            local duration=$((end_time - start_time))
            TESTS_FAILED=$((TESTS_FAILED + 1))
            FAILED_TESTS+=("$description")
            log_error "❌ $description failed (${duration}s)"
        fi
    else
        log_error "❌ Test script not found: $script_path"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        FAILED_TESTS+=("$description")
    fi
    
    echo
    echo
}

run_flow_test() {
    local script_path=$1
    local description=$2
    
    # Convert relative path to absolute
    local absolute_path="$SCRIPT_DIR/$script_path"
    
    log_header "$description"
    echo
    
    local start_time=$(date +%s)
    
    if [[ -f "$absolute_path" ]]; then
        # Pass cleanup flag if set
        local test_args=""
        if [[ "${CLEANUP:-true}" == "false" ]]; then
            test_args="--no-cleanup"
        fi
        
        if bash "$absolute_path" $test_args; then
            local end_time=$(date +%s)
            local duration=$((end_time - start_time))
            TESTS_PASSED=$((TESTS_PASSED + 1))
            log_success "✅ $description completed successfully (${duration}s)"
        else
            local end_time=$(date +%s)
            local duration=$((end_time - start_time))
            TESTS_FAILED=$((TESTS_FAILED + 1))
            FAILED_TESTS+=("$description")
            log_error "❌ $description failed (${duration}s)"
        fi
    else
        log_error "❌ Flow test script not found: $absolute_path"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        FAILED_TESTS+=("$description")
    fi
    
    echo
    echo
}

check_prerequisites() {
    log_info "🔍 Checking prerequisites..."
    
    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is required but not installed"
        exit 1
    fi
    
    # Check cluster connectivity
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    
    # Check NetGuard API server
    if ! kubectl api-resources --api-group=netguard.sgroups.io &> /dev/null; then
        log_error "NetGuard API server is not available"
        exit 1
    fi
    
    # Check jq (optional but recommended)
    if ! command -v jq &> /dev/null; then
        log_error "jq is recommended for advanced validations but not required"
    fi
    
    log_success "✅ All prerequisites met"
    echo
}

display_test_summary() {
    local suite_end_time=$(date +%s)
    local suite_duration=$((suite_end_time - SUITE_START_TIME))
    local total_tests=$((TESTS_PASSED + TESTS_FAILED))
    
    log_header "📊 TEST SUITE SUMMARY"
    echo
    echo "🕐 Total Duration: ${suite_duration}s"
    echo "📝 Total Test Suites: $total_tests"
    echo "✅ Passed: $TESTS_PASSED"
    echo "❌ Failed: $TESTS_FAILED"
    echo
    
    if [[ $TESTS_FAILED -gt 0 ]]; then
        log_error "Failed test suites:"
        for failed_test in "${FAILED_TESTS[@]}"; do
            echo "  • $failed_test"
        done
        echo
    fi
    
    local success_rate=0
    if [[ $total_tests -gt 0 ]]; then
        success_rate=$(( (TESTS_PASSED * 100) / total_tests ))
    fi
    
    echo "📈 Success Rate: ${success_rate}%"
    echo
    
    if [[ $TESTS_FAILED -eq 0 ]]; then
        log_success "🎉 ALL TEST SUITES PASSED!"
        log_success "🚀 NetGuard v1beta1 API is fully functional!"
    else
        log_error "💥 $TESTS_FAILED TEST SUITE(S) FAILED"
        log_error "🔧 Review the failures above and check your NetGuard deployment"
    fi
}

main() {
    log_header "🚀 NetGuard v1beta1 Complete Test Suite"
    echo
    log_info "Starting comprehensive validation of NetGuard functionality"
    echo
    
    # Prerequisites
    check_prerequisites
    
    # Run all test scenarios
    run_test_script "resource_validation.sh" "Resource CRUD Operations Validation"
    run_test_script "patch_validation.sh" "PATCH Operations Validation" 
    run_test_script "full_e2e_test.sh" "Full End-to-End Integration Test"
    
    # Run comprehensive flow test (Cross-RuleS2S aggregation validation)
    run_flow_test "../../config/samples/flow/run-complete-flow-test.sh" "Complete Flow Test (Cross-RuleS2S Aggregation)"
    
    # Display summary
    display_test_summary
    
    # Exit with appropriate code
    if [[ $TESTS_FAILED -eq 0 ]]; then
        exit 0
    else
        exit 1
    fi
}

# Help function
show_help() {
    echo "NetGuard Test Suite Runner"
    echo ""
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  -h, --help    Show this help message"
    echo "  --no-cleanup  Skip cleanup of test namespaces (for debugging)"
    echo ""
    echo "Test Scripts:"
    echo "  resource_validation.sh  - Validates CRUD operations on all 10 resources"
    echo "  patch_validation.sh     - Validates all PATCH operation types"
    echo "  full_e2e_test.sh       - Complete E2E scenario testing"
    echo "  complete-flow-test      - Multi-tier application with Cross-RuleS2S aggregation"
    echo ""
    echo "Prerequisites:"
    echo "  - kubectl installed and configured"
    echo "  - NetGuard API server running and accessible"
    echo "  - jq installed (recommended for advanced validations)"
    echo ""
    echo "Examples:"
    echo "  $0                      # Run all tests with cleanup"
    echo "  $0 --no-cleanup         # Run all tests without cleanup (for debugging)"
}

# Parse command line arguments
for arg in "$@"; do
    case $arg in
        -h|--help)
            show_help
            exit 0
            ;;
        --no-cleanup)
            export CLEANUP=false
            shift
            ;;
        *)
            log_error "Unknown option: $arg"
            show_help
            exit 1
            ;;
    esac
done

# Run main function if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi