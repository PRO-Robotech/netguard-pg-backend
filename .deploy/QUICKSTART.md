# NetGuard Debug - Quick Start Guide

Get debugging in under 2 minutes! ⚡

## Prerequisites

- Minikube running (`minikube status`)
- Kubectl configured
- Go IDE (GoLand or VSCode)

## Step 1: Deploy Debug Pods

```bash
cd .deploy/skaffold
skaffold dev -p debug-all
```

Wait for all 3 pods to be running. You'll see:
```
netguard-backend-debug    1/1   Running
netguard-apiserver-debug  1/1   Running
netguard-webhook-debug    1/1   Running
```

Leave this terminal open (Skaffold watches for code changes).

## Step 2: Start Debug Session

Open **new terminal**:

```bash
cd .deploy
./start-debug.sh
```

This script will:
- ✅ Check debug pods are running
- ✅ Check ports 2345/2346/2347 are available
- ✅ Switch production services to debug pods
- ✅ Start port-forwards in background

## Step 3: Connect Your IDE

### GoLand

1. **Run → Edit Configurations**
2. **Add New → Go Remote**
3. **Settings:**
   - Name: `Debug Backend`
   - Host: `localhost`
   - Port: `2345`
4. **Click Debug** (green bug icon)

Repeat for APIServer (port `2346`) and Webhook (port `2347`) if needed.

### VSCode

Add to `.vscode/launch.json`:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Attach to Backend",
      "type": "go",
      "request": "attach",
      "mode": "remote",
      "remotePath": "",
      "port": 2345,
      "host": "localhost"
    },
    {
      "name": "Attach to APIServer",
      "type": "go",
      "request": "attach",
      "mode": "remote",
      "remotePath": "",
      "port": 2346,
      "host": "localhost"
    }
  ]
}
```

Press **F5** and select configuration.

## Step 4: Set Breakpoints & Test

1. **Set breakpoints** in your code (e.g., `internal/k8s/client/grpc.go`)

2. **Send request** from frontend:
   ```bash
   # Open frontend
   open http://localhost:8081

   # Or use kubectl
   kubectl get services.netguard.sgroups.io -n incloud-sgroups
   ```

3. **Breakpoint hits!** 🎉
   - Step through code
   - Inspect variables
   - Debug request flow: Frontend → APIServer → Backend

## Common Tasks

### Check Status
```bash
./verify-debug-setup.sh
```

### Restart Port-Forwards
If connection drops:
```bash
./restart-port-forwards.sh
```

### Stop Debugging
```bash
./stop-debug.sh
```

This restores production services.

## Troubleshooting

### Ports Already in Use
```bash
# Kill processes on debug ports
lsof -ti :2345 | xargs kill -9
lsof -ti :2346 | xargs kill -9
lsof -ti :2347 | xargs kill -9

# Or use start-debug.sh (offers to kill automatically)
./start-debug.sh
```

### Debug Pods Not Running
```bash
# Check pod status
kubectl get pod -n incloud-sgroups -l app.kubernetes.io/component=debug

# Check logs
kubectl logs -f <pod-name> -n incloud-sgroups
```

### Debugger Won't Connect
```bash
# Check port-forward is active
ps aux | grep port-forward

# Check Delve is listening
kubectl logs <pod-name> -n incloud-sgroups | grep "API server listening"

# Should see: API server listening at: [::]:2345
```

### Breakpoints Not Hitting
1. ✅ Check services point to debug pods:
   ```bash
   kubectl get svc netguard-backend -n incloud-sgroups -o yaml | grep instance
   # Should show: instance: netguard-debug
   ```

2. ✅ Check APIServer connects to debug backend:
   ```bash
   kubectl exec <apiserver-pod> -n incloud-sgroups -- env | grep BACKEND
   # Should show: BACKEND_ENDPOINT=netguard-backend-debug:9090
   ```

3. ✅ Verify debugger is attached (check IDE status bar)

## Architecture

```
Frontend (localhost:8081)
    ↓
APIServer Service → APIServer Debug Pod (localhost:2346)
    ↓
Backend Service → Backend Debug Pod (localhost:2345)
    ↓
PostgreSQL, SGROUPS
```

All services automatically routed to debug pods with Delve attached.

## Next Steps

- **Full docs**: `.claude/knowledge/debug/`
- **Detailed guide**: `README.md`
- **Troubleshooting**: `.claude/knowledge/debug/07-troubleshooting.md`

---

**Happy Debugging!** 🐛🔍
