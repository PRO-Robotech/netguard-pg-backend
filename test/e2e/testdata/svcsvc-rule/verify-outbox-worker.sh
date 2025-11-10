#!/bin/bash
set -e

NAMESPACE="incloud-sgroups"
POD="netguard-postgresql-0"
TEST_DIR="$(dirname "$0")"

echo "=== OutboxWorker Verification for SvcSvcRule ==="
echo ""

# Check if PostgreSQL pod exists
if ! kubectl get pod -n $NAMESPACE $POD > /dev/null 2>&1; then
    echo "ERROR: PostgreSQL pod $POD not found in namespace $NAMESPACE"
    exit 1
fi

# 1. Create prerequisite Services
echo "1. Creating prerequisite Services..."
kubectl apply -f "$TEST_DIR/test-service-web.yaml"
kubectl apply -f "$TEST_DIR/test-service-db.yaml"
sleep 2

# 2. Create SvcSvcRule
echo ""
echo "2. Creating SvcSvcRule..."
kubectl apply -f "$TEST_DIR/test-svcsvc-rule-001.yaml"

sleep 2

# 3. Check initial outbox entry
echo ""
echo "3. Checking initial outbox entry (should be PENDING)..."
echo "   SQL: SELECT id, resource_name, operation, status, attempts FROM sync_outbox WHERE resource_type = 'SvcSvcRule' AND resource_name = 'test-svcsvc-rule-001' ORDER BY created_at DESC LIMIT 1;"
kubectl exec $POD -n $NAMESPACE -- \
  env PGPASSWORD=netguard psql -U netguard -d netguard -c \
  "SELECT id, resource_name, operation, status, attempts FROM sync_outbox WHERE resource_type = 'SvcSvcRule' AND resource_name = 'test-svcsvc-rule-001' ORDER BY created_at DESC LIMIT 1;"

# 4. Wait for OutboxWorker to process
echo ""
echo "4. Waiting for OutboxWorker to process (10 seconds)..."
sleep 10

# 5. Check final outbox entry
echo ""
echo "5. Checking final outbox entry (status should be COMPLETED or FAILED)..."
echo "   SQL: SELECT id, resource_name, operation, status, attempts, error_message FROM sync_outbox WHERE resource_type = 'SvcSvcRule' AND resource_name = 'test-svcsvc-rule-001' ORDER BY created_at DESC LIMIT 1;"
kubectl exec $POD -n $NAMESPACE -- \
  env PGPASSWORD=netguard psql -U netguard -d netguard -c \
  "SELECT id, resource_name, operation, status, attempts FROM sync_outbox WHERE resource_type = 'SvcSvcRule' AND resource_name = 'test-svcsvc-rule-001' ORDER BY created_at DESC LIMIT 1;"

# 6. Check if entry is completed or failed
echo ""
echo "6. Analyzing OutboxWorker result..."
STATUS=$(kubectl exec $POD -n $NAMESPACE -- \
  env PGPASSWORD=netguard psql -U netguard -d netguard -t -A -c \
  "SELECT status FROM sync_outbox WHERE resource_type = 'SvcSvcRule' AND resource_name = 'test-svcsvc-rule-001' ORDER BY created_at DESC LIMIT 1;" 2>/dev/null || echo "UNKNOWN")

case "$STATUS" in
  "COMPLETED")
    echo "✅ OutboxWorker successfully processed SvcSvcRule (status: COMPLETED)"
    ;;
  "FAILED")
    echo "⚠️  OutboxWorker failed to process SvcSvcRule (status: FAILED)"
    echo "   Checking error message..."
    kubectl exec $POD -n $NAMESPACE -- \
      env PGPASSWORD=netguard psql -U netguard -d netguard -c \
      "SELECT error_message FROM sync_outbox WHERE resource_type = 'SvcSvcRule' AND resource_name = 'test-svcsvc-rule-001' ORDER BY created_at DESC LIMIT 1;"
    ;;
  "PENDING")
    echo "⏳ OutboxWorker has NOT processed SvcSvcRule yet (status: PENDING)"
    echo "   This may indicate OutboxWorker is not running or poll interval is long"
    ;;
  *)
    echo "❓ Unknown status: $STATUS"
    ;;
esac

# 7. Cleanup
echo ""
echo "7. Cleaning up..."
kubectl delete -f "$TEST_DIR/test-svcsvc-rule-001.yaml"
kubectl delete -f "$TEST_DIR/test-service-web.yaml"
kubectl delete -f "$TEST_DIR/test-service-db.yaml"

echo ""
echo "=== OutboxWorker verification completed ==="
