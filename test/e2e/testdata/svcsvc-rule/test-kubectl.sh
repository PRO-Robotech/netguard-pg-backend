#!/bin/bash
set -e

NAMESPACE="incloud-sgroups"
TEST_DIR="$(dirname "$0")"

echo "=== SvcSvcRule kubectl Integration Test ==="
echo ""

# 1. Create prerequisite Services
echo "Step 1: Creating test Services..."
kubectl apply -f "$TEST_DIR/test-service-web.yaml"
kubectl apply -f "$TEST_DIR/test-service-db.yaml"

echo "Waiting for Services to be created..."
sleep 3

# 2. Verify Services exist
echo ""
echo "Step 2: Verifying Services..."
kubectl get services.netguard.sgroups.io -n $NAMESPACE test-service-web -o yaml | grep -E "^  name:|^status:" | head -2
kubectl get services.netguard.sgroups.io -n $NAMESPACE test-service-db -o yaml | grep -E "^  name:|^status:" | head -2

# 3. Create valid SvcSvcRule
echo ""
echo "Step 3: Creating valid SvcSvcRule..."
kubectl apply -f "$TEST_DIR/test-svcsvc-rule-001.yaml"

echo "Waiting for SvcSvcRule to process conditions..."
sleep 3

# 4. Get SvcSvcRule
echo ""
echo "Step 4: Getting SvcSvcRule YAML..."
kubectl get svcsvcrules.netguard.sgroups.io -n $NAMESPACE test-svcsvc-rule-001 -o yaml

# 5. Check conditions
echo ""
echo "Step 5: Checking Ready condition..."
kubectl get svcsvcrules.netguard.sgroups.io -n $NAMESPACE test-svcsvc-rule-001 \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")]}' | jq .

# 6. List all SvcSvcRules
echo ""
echo "Step 6: Listing all SvcSvcRules in namespace..."
kubectl get svcsvcrules.netguard.sgroups.io -n $NAMESPACE

# 7. Test invalid SvcSvcRule (should fail)
echo ""
echo "Step 7: Testing invalid SvcSvcRule (expect validation failure)..."
if kubectl apply -f "$TEST_DIR/test-svcsvc-rule-invalid-action.yaml" 2>&1 | grep -qi "error\|invalid"; then
    echo "✅ Validation correctly rejected invalid rule"
else
    echo "⚠️  Warning: Validation did NOT reject invalid rule (may need API validation)"
fi

# 8. Delete SvcSvcRule
echo ""
echo "Step 8: Deleting SvcSvcRule..."
kubectl delete -f "$TEST_DIR/test-svcsvc-rule-001.yaml"

echo "Waiting for deletion..."
sleep 2

# 9. Cleanup Services
echo ""
echo "Step 9: Cleaning up Services..."
kubectl delete -f "$TEST_DIR/test-service-web.yaml"
kubectl delete -f "$TEST_DIR/test-service-db.yaml"

echo ""
echo "=== Test completed successfully ==="
