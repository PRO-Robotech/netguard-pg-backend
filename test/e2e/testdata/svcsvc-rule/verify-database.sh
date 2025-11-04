#!/bin/bash

NAMESPACE="incloud-sgroups"
POD="netguard-postgresql-0"

echo "=== Database Verification for SvcSvcRule ==="
echo ""

# Check if PostgreSQL pod exists
if ! kubectl get pod -n $NAMESPACE $POD > /dev/null 2>&1; then
    echo "ERROR: PostgreSQL pod $POD not found in namespace $NAMESPACE"
    exit 1
fi

# 1. Check svc_svc_rules table
echo "1. Checking svc_svc_rules table..."
echo "   SQL: SELECT namespace, name, action, priority, logs, trace FROM svc_svc_rules ORDER BY created_at DESC LIMIT 5;"
kubectl exec $POD -n $NAMESPACE -- \
  env PGPASSWORD=netguard psql -U netguard -d netguard -c \
  "SELECT namespace, name, action, priority, logs, trace FROM svc_svc_rules ORDER BY created_at DESC LIMIT 5;"

echo ""
echo "2. Checking service_rule_refs junction table..."
echo "   SQL: SELECT service_ref, role FROM service_rule_refs ORDER BY created_at DESC LIMIT 10;"
kubectl exec $POD -n $NAMESPACE -- \
  env PGPASSWORD=netguard psql -U netguard -d netguard -c \
  "SELECT service_ref, role FROM service_rule_refs ORDER BY created_at DESC LIMIT 10;"

echo ""
echo "3. Checking sync_outbox for SvcSvcRule entries..."
echo "   SQL: SELECT resource_type, resource_namespace, resource_name, operation, status FROM sync_outbox WHERE resource_type = 'SvcSvcRule' ORDER BY created_at DESC LIMIT 5;"
kubectl exec $POD -n $NAMESPACE -- \
  env PGPASSWORD=netguard psql -U netguard -d netguard -c \
  "SELECT resource_type, resource_namespace, resource_name, operation, status FROM sync_outbox WHERE resource_type = 'SvcSvcRule' ORDER BY created_at DESC LIMIT 5;"

echo ""
echo "4. Checking xSvcSvcRules fields in services table..."
echo "   SQL: SELECT namespace, name, xsvcsvc_rules_as_from, xsvcsvc_rules_as_to FROM services WHERE namespace = 'incloud-sgroups' AND (name = 'test-service-web' OR name = 'test-service-db');"
kubectl exec $POD -n $NAMESPACE -- \
  env PGPASSWORD=netguard psql -U netguard -d netguard -c \
  "SELECT namespace, name, xsvcsvc_rules_as_from, xsvcsvc_rules_as_to FROM services WHERE namespace = 'incloud-sgroups' AND (name = 'test-service-web' OR name = 'test-service-db');" || echo "Services not found (expected if cleaned up)"

echo ""
echo "=== Database verification completed ==="
