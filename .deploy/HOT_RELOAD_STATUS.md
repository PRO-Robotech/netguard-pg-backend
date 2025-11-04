# Hot Reload Status

## ✅ What's Working

### 1. Air Hot Reload Inside Pod
- **Pod is running** with Air + Delve
- **Air watches for changes** inside the pod
- **Auto-rebuild works** when files change inside the pod (confirmed in logs at 17:52:58)
- **Delve debugger available** on port 2345
- **Pod stays alive** during rebuilds
- **Rebuild time**: ~2-5 seconds (vs 30-60 seconds for full Docker rebuild)

### 2. Infrastructure
- ✅ Skaffold profile `debug-hot-reload` created
- ✅ Dockerfile `.deploy/dockerfiles/Dockerfile.backend.debug-air` with Air + Delve
- ✅ Kubernetes manifest `.deploy/k8s/backend-hot-reload.yaml`
- ✅ Air configuration `.air.toml`
- ✅ Launch script `.deploy/skaffold/run-hot-reload.sh`
- ✅ Services switched to debug pods via `./switch-to-debug.sh`
- ✅ Port-forwarding active (2345, 9090, 8080, 5432)
- ✅ Namespace configured correctly (`incloud-sgroups`)

## ❌ What's NOT Working

### Skaffold File Sync
**Problem**: Skaffold is NOT syncing files from host to pod.

**Evidence**:
- No "file sync" or "syncing" messages in Skaffold logs (even with `-v debug`)
- File changes on host don't trigger Air rebuilds
- Skaffold detects changes (`Change detected<nil>`) but doesn't sync them

**Why This Matters**:
- You currently need to **manually** copy files into the pod OR restart Skaffold for changes to apply
- This defeats the purpose of hot reload

**Possible Causes**:
1. Skaffold file sync may not work with Minikube Docker driver
2. Configuration syntax issue in `skaffold.yaml`
3. Skaffold version incompatibility (v2.16.1 with apiVersion v2beta29)
4. Missing Skaffold flag or permission issue

## 🔧 Current Workaround

Since Skaffold file sync isn't working, you have two options:

### Option A: Manual File Sync (fastest for single file changes)
```bash
# After editing a file locally, copy it to the pod:
kubectl cp /path/to/file.go incloud-sgroups/pod-name:/src/path/to/file.go

# Example:
kubectl cp internal/api/service.go incloud-sgroups/netguard-backend-debug-xxx:/src/internal/api/service.go

# Air will detect the change and rebuild automatically
```

### Option B: Full Rebuild (for multiple file changes)
```bash
# Stop and restart Skaffold:
# Ctrl+C in the Skaffold terminal
cd .deploy/skaffold
./run-hot-reload.sh
```

## 🎯 Recommended Next Steps

### 1. Debug Skaffold File Sync
Try alternative sync configuration in `skaffold.yaml`:
```yaml
sync:
  auto: true  # Instead of manual rules
```

Or try `infer` mode:
```yaml
sync:
  infer:
    - "**/*.go"
```

### 2. Check Skaffold Documentation
- https://skaffold.dev/docs/filesync/
- Look for Minikube-specific requirements
- Check if Docker driver has limitations

### 3. Consider Alternative Approach
Use `kubectl port-forward` to expose a file sync service in the pod, or use `rsync` via a sidecar container.

### 4. Test with Minikube mount
```bash
minikube mount /Users/zhd/Projects/newPro/netguard-pg-backend:/host-code
```
Then modify Dockerfile to use mounted volume.

## 📊 Performance Comparison

| Method | Time | Pod Restart | Port-forwards |
|--------|------|-------------|---------------|
| **Old (no hot reload)** | 30-60s | Yes ❌ | Lost ❌ |
| **Current (Air only)** | 2-5s* | No ✅ | Kept ✅ |
| **Goal (Air + file sync)** | 2-5s | No ✅ | Kept ✅ |

\* Requires manual file copy currently

## 🚀 How to Use Current Setup

1. **Start hot reload**:
   ```bash
   cd .deploy/skaffold
   ./run-hot-reload.sh
   ```

2. **Edit code locally**

3. **Copy changed file to pod**:
   ```bash
   POD=$(kubectl get pod -n incloud-sgroups -l app.kubernetes.io/component=debug-hot-reload -o jsonpath='{.items[0].metadata.name}')
   kubectl cp internal/your/file.go incloud-sgroups/$POD:/src/internal/your/file.go
   ```

4. **Air rebuilds automatically** (~2-5 seconds)

5. **Connect debugger** to `localhost:2345`

## 🐛 Debugging

```bash
# Check pod logs (Air output):
kubectl logs -f deployment/netguard-backend-debug -n incloud-sgroups

# Check if Delve is running:
curl http://localhost:2345

# Verify port-forwards:
lsof -i:2345
lsof -i:9090
lsof -i:8080

# Check services route to debug pods:
kubectl get endpoints -n incloud-sgroups -l app.kubernetes.io/name=backend
```
