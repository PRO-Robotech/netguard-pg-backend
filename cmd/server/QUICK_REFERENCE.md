# Quick Reference - SGroupConnectionMonitor Integration

## Как это работает

### 1. Startup Flow

```
1. main() starts
2. Create sgroupsClient (if sync.enabled)
3. Create ConnectionMonitor
4. connectionMonitor.Start() → launches goroutine
5. Wait for connection (if sync.required)
6. Setup components (pass client + monitor)
7. Register /healthz/sync endpoint
8. Server running...
```

### 2. Connection States

**CONNECTED:**
- ConnectionMonitor continuously streams from SGROUP
- ReverseSyncSystem processes timestamps
- OutboxWorker processes entries
- `/healthz/sync` → 200 OK

**DISCONNECTED:**
- ConnectionMonitor retries with exponential backoff
- ReverseSyncSystem waits for reconnection
- OutboxWorker paused
- `/healthz/sync` → 503 Unavailable

### 3. Endpoints

**Health Check:**
```bash
curl http://localhost:8080/healthz/sync

# Connected:
{"status":"healthy","last_healthy":"2025-10-18T12:00:00Z","component":"sgroup_sync"}

# Disconnected:
{"status":"unhealthy","component":"sgroup_sync","last_unhealthy":"...","error":"..."}
```

**Worker Health:**
```bash
curl http://localhost:8080/healthz/worker
```

**Metrics (if enabled):**
```bash
curl http://localhost:8080/metrics | grep sgroup
```

### 4. Configuration

**config.yaml:**
```yaml
sync:
  enabled: true
  required: false  # If true - wait for connection on startup

  sgroups:
    grpc_address: "sgroups.example.com:9001"
    request_timeout: 30s
    tls:
      enabled: true
      verify: "verify"
```

**Environment Variables:**
```bash
# See .env.example
OUTBOX_WORKER_ENABLED=true
OUTBOX_WORKER_POLL_INTERVAL=5s
OUTBOX_WORKER_BATCH_SIZE=10
```

### 5. Graceful Shutdown

**Signal (CTRL+C):**
```
1. Health endpoints → NOT_SERVING
2. OutboxWorker.Stop() (30s timeout)
3. ReverseSyncSystem.Stop()
4. ConnectionMonitor.Stop() (via defer)
5. gRPC/HTTP servers shutdown
```

### 6. Logs to Watch

**Startup:**
```
[INFO] Waiting for SGROUP connection (required mode)...
[INFO] SGROUP connection established
```

**Connection Issues:**
```
[WARN] SGROUP not connected, will retry in background
[ERROR] Failed to establish stream: rpc error: ...
[INFO] Attempting reconnect in 5s...
```

**ReverseSyncSystem:**
```
[INFO] ReverseSyncSystem: SGROUP connected
[WARN] ReverseSyncSystem: SGROUP disconnected
[DEBUG] ReverseSyncSystem received timestamp: ...
```

**OutboxWorker:**
```
[INFO] OutboxWorker: Processing paused (SGROUP disconnected)
[INFO] OutboxWorker: Processing resumed (SGROUP connected)
```

### 7. Testing Scenarios

**Test 1: Normal Operation**
```bash
# Start server
go run cmd/server/main.go --pg-uri="..." --config=config/config.yaml

# Check health
curl http://localhost:8080/healthz/sync
# Expected: 200 OK

# Create resource
kubectl apply -f test/e2e/testdata/cloud233/hosts/test-host-001.yaml

# Check outbox processed
# Check logs for sync activity
```

**Test 2: SGROUP Unavailable (required=false)**
```bash
# Stop SGROUP (or use invalid address in config)
# Start server
go run cmd/server/main.go --pg-uri="..." --config=config/config.yaml

# Server starts successfully
# Expected log: "SGROUP not connected, will retry in background"

# Check health
curl http://localhost:8080/healthz/sync
# Expected: 503 Unavailable

# Reconnect attempts in logs every 5s, 10s, 20s, ...
```

**Test 3: SGROUP Unavailable (required=true)**
```bash
# Update config: sync.required=true
# Stop SGROUP
# Start server
go run cmd/server/main.go --pg-uri="..." --config=config/config.yaml

# Server waits 10s for connection
# If timeout → Fatal: "SGROUP connection required but not established"
# Server exits
```

**Test 4: Connection Loss During Operation**
```bash
# Start server (SGROUP available)
# Create resources → should sync

# Stop SGROUP
# Expected logs:
#   "ReverseSyncSystem: SGROUP disconnected"
#   "OutboxWorker: Processing paused"

# Check health → 503 Unavailable

# Restart SGROUP
# Expected logs:
#   "Reconnect successful"
#   "ReverseSyncSystem: SGROUP connected"
#   "OutboxWorker: Processing resumed"

# Check health → 200 OK
```

### 8. Troubleshooting

**Problem: Server exits immediately**
- Check: `sync.required=true` and SGROUP unavailable
- Solution: Set `required=false` or fix SGROUP

**Problem: /healthz/sync not found**
- Check: `cfg.Sync.Enabled` is true
- Check: connMonitor created successfully

**Problem: OutboxWorker not processing**
- Check: `/healthz/sync` status
- Check: SGROUP connection
- Check: `OUTBOX_WORKER_ENABLED=true`

**Problem: Continuous reconnect attempts**
- Check: SGROUP address correct
- Check: TLS configuration
- Check: Network connectivity

### 9. Architecture Diagram

```
┌─────────────────────────────────────────────────┐
│                   main.go                       │
├─────────────────────────────────────────────────┤
│                                                 │
│  sgroupsClient (SINGLE INSTANCE)                │
│       │                                         │
│       ├──> ConnectionMonitor                    │
│       │         │                               │
│       │         ├──> HealthEndpointListener     │
│       │         │         │                     │
│       │         │         └──> /healthz/sync    │
│       │         │                               │
│       │         ├──> ReverseSyncSystem          │
│       │         │                               │
│       │         └──> OutboxWorker               │
│       │                                         │
│       ├──> setupSyncManager()                   │
│       │         └──> SyncManager                │
│       │                                         │
│       └──> setupReverseSyncSystem()             │
│                 └──> ReverseSyncSystem          │
│                                                 │
└─────────────────────────────────────────────────┘
```

### 10. Files Changed

**Modified:**
- `cmd/server/main.go` (462 lines)
  - Added imports
  - Centralized client creation
  - Added ConnectionMonitor
  - Updated all setup functions
  - Added health endpoint
  - Updated graceful shutdown

**Created:**
- `cmd/server/INTEGRATION_SUMMARY.md` (full documentation)
- `cmd/server/QUICK_REFERENCE.md` (this file)

**Dependencies (already implemented):**
- `internal/sync/monitor/connection_monitor.go`
- `internal/app/server/health_listener.go`
- `internal/sync/example_integration.go` (ReverseSyncSystem)
- `internal/sync/worker/outbox_worker.go` (updated)

---

## Next Steps

1. **Test locally:**
   ```bash
   make run-pg
   ```

2. **Deploy to minikube:**
   ```bash
   ./scripts/deploy-local.sh backend
   ```

3. **Monitor health:**
   ```bash
   kubectl port-forward -n incloud-sgroups svc/netguard-backend 8080:8080
   curl http://localhost:8080/healthz/sync
   ```

4. **Check logs:**
   ```bash
   kubectl logs -f -n incloud-sgroups deployment/netguard-backend
   ```

---

**Version:** 1.0.0
**Date:** 2025-10-18
**Status:** ✅ READY FOR TESTING
