# Netguard Webhook Testing Guide

## 🧪 Testing Webhook Functionality

This guide provides step-by-step instructions for testing the Netguard admission webhooks.

## Prerequisites

1. **Kubernetes cluster** with cert-manager installed
2. **kubectl** configured to access the cluster
3. **Netguard API Server** deployed and running

## 🚀 Quick Test Setup

### 1. Deploy Netguard with Webhooks

```bash
# Deploy everything including webhooks
cd netguard-pg-backend/config/k8s
kubectl apply -k .

# Wait for deployment to be ready
kubectl wait --for=condition=available --timeout=300s deployment/netguard-apiserver -n netguard-system

# Check webhook configurations
kubectl get validatingwebhookconfiguration netguard-validator
kubectl get mutatingwebhookconfiguration netguard-mutator
```

### 2. Verify CA Bundle Setup

```bash
# If using cert-manager (recommended):
kubectl get certificate -n netguard-system
kubectl get secret netguard-webhook-tls -n netguard-system

# If cert-manager auto-injection doesn't work, use helper script:
./scripts/setup-webhook-ca.sh
```

### 3. Check Webhook Server Health

```bash
# Check webhook server is running
kubectl get pods -n netguard-system -l app=netguard-apiserver

# Check webhook server logs
kubectl logs -n netguard-system deployment/netguard-apiserver -c webhook --tail=50

# Test webhook endpoints (if accessible)
kubectl port-forward -n netguard-system svc/netguard-apiserver-webhook 9443:443 &
curl -k https://localhost:9443/healthz
curl -k https://localhost:9443/readyz
```

## 🔍 Webhook Functionality Tests

### Test 1: Mutation Webhook (Adding Defaults)

```bash
# Apply valid resource without defaults
cat <<EOF | kubectl apply -f -
apiVersion: netguard.sgroups.io/v1beta1
kind: AddressGroup
metadata:
  name: test-mutation
  namespace: netguard-system
spec:
  logs: true
  # defaultAction missing - mutation webhook should add it
EOF

# Verify mutation webhook added defaults
kubectl get addressgroup test-mutation -n netguard-system -o yaml | grep -A 5 "spec:"
# Should show: defaultAction: ACCEPT

# Check for added labels and annotations
kubectl get addressgroup test-mutation -n netguard-system -o jsonpath='{.metadata.labels}'
# Should show: {"app.kubernetes.io/managed-by":"netguard-apiserver"}
```

### Test 2: Validation Webhook (Dependency Checks)

```bash
# This should FAIL because referenced service doesn't exist
cat <<EOF | kubectl apply -f -
apiVersion: netguard.sgroups.io/v1beta1
kind: AddressGroupBinding
metadata:
  name: test-validation-fail
  namespace: netguard-system
spec:
  addressGroupRef:
    name: nonexistent-addressgroup
    namespace: netguard-system
  serviceRef:
    name: nonexistent-service
    namespace: netguard-system
EOF
# Expected: Error from server (validation failed: dependency not found)
```

### Test 3: Storage Validation (Syntax Checks)

```bash
# This should FAIL at storage level due to invalid enum
cat <<EOF | kubectl apply -f -
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: test-storage-fail
  namespace: netguard-system
spec:
  description: "Invalid service"
  ingressPorts:
  - protocol: INVALID_PROTOCOL  # Invalid enum
    port: "99999"              # Invalid port range
EOF
# Expected: Error from server (validation failed: invalid enum value)
```

### Test 4: Complete Workflow (Valid Resources)

```bash
# Apply valid resources in order
kubectl apply -f ../samples/test-webhook-validation.yaml

# Verify all valid resources were created
kubectl get services,addressgroups,addressgroupbindings,rules2s -n netguard-system

# Check that invalid resources were rejected
kubectl get events -n netguard-system --field-selector reason=FailedCreate
```

## 🔧 Troubleshooting

### Webhook Not Called

```bash
# Check webhook configuration
kubectl get validatingwebhookconfiguration netguard-validator -o yaml

# Verify caBundle is populated
kubectl get validatingwebhookconfiguration netguard-validator -o jsonpath='{.webhooks[0].clientConfig.caBundle}'

# Check service endpoint
kubectl get endpoints netguard-apiserver-webhook -n netguard-system
```

### Certificate Issues

```bash
# Check certificate status
kubectl describe certificate netguard-apiserver-webhook-cert -n netguard-system

# Verify TLS secret
kubectl get secret netguard-webhook-tls -n netguard-system -o yaml

# Test TLS connection manually
openssl s_client -connect netguard-apiserver-webhook.netguard-system.svc.cluster.local:443 -servername netguard-apiserver-webhook.netguard-system.svc.cluster.local
```

### Webhook Server Errors

```bash
# Check webhook server logs
kubectl logs -n netguard-system deployment/netguard-apiserver -c webhook --tail=100

# Common errors and solutions:
# 1. "TLS certificate not found" → Check secret mounting
# 2. "Backend client connection failed" → Check backend service
# 3. "Validation timeout" → Check backend response time
```

### Backend Connectivity Issues

```bash
# Test backend connectivity from webhook
kubectl exec -n netguard-system deployment/netguard-apiserver -c webhook -- \
  curl http://netguard-backend:9090/health

# Check backend service
kubectl get service netguard-backend -n netguard-system
kubectl get endpoints netguard-backend -n netguard-system
```

## 📊 Expected Test Results

### ✅ Successful Cases

| Test Case | Expected Result |
|-----------|----------------|
| Valid Service | Created with managed-by label |
| Valid AddressGroup (no defaultAction) | Created with defaultAction: ACCEPT |
| Valid AddressGroupBinding | Created after dependency validation |
| Valid RuleS2S (missing namespace) | Created with normalized namespaces |

### ❌ Expected Failures

| Test Case | Expected Error |
|-----------|----------------|
| Invalid Protocol Enum | "validation failed: invalid enum value" |
| Nonexistent Service Reference | "validation failed: service not found" |
| Invalid Port Range | "validation failed: port must be 1-65535" |
| Missing Required Fields | "validation failed: field is required" |

## 🎯 Success Criteria

✅ **Mutation Webhook Working:**
- Default values added automatically
- Labels and annotations added
- Namespaces normalized

✅ **Validation Webhook Working:**  
- Dependency checks pass/fail correctly
- Backend connectivity working
- Error messages are descriptive

✅ **Storage Validation Working:**
- Syntax validation working
- Enum validation working
- Field validation working

✅ **Integration Working:**
- No duplicate validation errors
- Proper error priorities
- Webhook → Storage → Backend flow

## 🔄 Automated Testing

```bash
# Run complete webhook test suite
cd netguard-pg-backend/config/k8s
./scripts/run-webhook-tests.sh

# Expected output:
# ✅ Mutation webhook tests: PASSED
# ✅ Validation webhook tests: PASSED  
# ✅ Storage validation tests: PASSED
# ✅ Integration tests: PASSED
```

## 📝 Manual Verification

After running tests, manually verify:

1. **Webhook configurations** have correct caBundle
2. **Webhook server** responds to health checks
3. **Valid resources** are created with mutations applied
4. **Invalid resources** are rejected with clear error messages
5. **Backend connectivity** is working from webhook
6. **No duplicate validation** between webhook and storage

## 🎉 Success!

If all tests pass, your Netguard admission webhooks are working correctly and ready for production use! 