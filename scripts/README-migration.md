# Netguard Cleanup and Migration Guide

## 🎯 Overview

This guide helps you migrate from the old `netguard-test` namespace to the new `netguard-system` namespace, ensuring all components are properly cleaned up and redeployed.

## 📋 Prerequisites

- kubectl configured for your minikube cluster
- Minikube profile `incloud` is active
- Docker is running
- You have backup access to your current deployment

## 🚀 Quick Migration

### Option 1: Full Automatic Migration (Recommended)

```bash
# Navigate to the project directory
cd netguard-pg-backend

# Run the migration script
./scripts/cleanup-and-migrate.sh
```

This will:
1. ✅ Create backup of current state
2. ✅ Clean up all resources in `netguard-test`
3. ✅ Clean up webhook configurations
4. ✅ Clean up API service registrations
5. ✅ Create `netguard-system` namespace
6. ✅ Deploy all components to `netguard-system`
7. ✅ Update deployment scripts
8. ✅ Verify the deployment

### Option 2: Step-by-Step Migration

```bash
# 1. Clean up only (if you want to do it manually)
./scripts/cleanup-and-migrate.sh cleanup-only

# 2. Deploy only (after cleanup)
./scripts/cleanup-and-migrate.sh deploy-only

# 3. Test deployment
./scripts/cleanup-and-migrate.sh test-only
```

## 🔧 Updated Scripts

After migration, use these updated scripts:

### API Server Redeploy
```bash
# Redeploy API server to netguard-system
./scripts/redeploy-apiserver.sh
```

### Backend Redeploy
```bash
# Redeploy backend to netguard-system
./scripts/redeploy-backend.sh
```

## 📊 Verification

After migration, verify everything is working:

```bash
# Check namespace
kubectl get namespace netguard-system

# Check deployments
kubectl get pods -n netguard-system

# Check services
kubectl get services -n netguard-system

# Check API resources
kubectl api-resources --api-group=netguard.sgroups.io

# Check webhook configurations
kubectl get validatingwebhookconfiguration,mutatingwebhookconfiguration | grep netguard

# Check API service registration
kubectl get apiservices | grep netguard
```

## 🐛 Troubleshooting

### If migration fails:

1. **Check logs**:
   ```bash
   kubectl logs -n netguard-system deployment/netguard-apiserver
   kubectl logs -n netguard-system deployment/netguard-backend
   ```

2. **Verify prerequisites**:
   ```bash
   kubectl config current-context  # Should be "incloud"
   minikube profile list           # Should show "incloud"
   ```

3. **Manual cleanup** (if needed):
   ```bash
   # Clean up webhooks
   kubectl delete validatingwebhookconfiguration netguard-validator
   kubectl delete mutatingwebhookconfiguration netguard-mutator
   
   # Clean up API services
   kubectl delete apiservice v1beta1.netguard.sgroups.io
   
   # Clean up namespace
   kubectl delete namespace netguard-test
   ```

### If you need to rollback:

1. **Restore from backup**:
   ```bash
   # Check backup directory (created during migration)
   ls -la backup-*
   
   # Restore namespace (example)
   kubectl apply -f backup-YYYYMMDD_HHMMSS/namespace-netguard-test.yaml
   ```

## 🎉 Success Indicators

After successful migration, you should see:

- ✅ `netguard-system` namespace exists
- ✅ `netguard-test` namespace is deleted
- ✅ All pods are running in `netguard-system`
- ✅ API resources are available: `kubectl api-resources --api-group=netguard.sgroups.io`
- ✅ Webhook configurations are active
- ✅ Updated deployment scripts work correctly

## 📝 Notes

- The migration creates a backup before starting
- All components are redeployed with the same configuration
- Docker images are reloaded to minikube
- Webhook configurations are properly registered
- API service registrations point to `netguard-system`

## 🔗 Next Steps

After successful migration, continue with Phase 8 tasks:
1. Update Makefile
2. Create CI/CD pipeline
3. Add comprehensive documentation
4. Performance testing 