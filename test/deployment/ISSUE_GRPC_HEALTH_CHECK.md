# Issue Report: Backend gRPC Health Check Missing

**Issue ID**: DEPLOY-001
**Priority**: P0 (CRITICAL - Blocks Deployment)
**Type**: Bug - Missing Implementation
**Status**: OPEN
**Created**: 2025-10-14
**Reporter**: QA Engineer
**Assignee**: backend-dev, tech-lead

---

## Summary

Backend pod fails Kubernetes liveness probe because application does not implement gRPC health check service, causing CrashLoopBackOff.

---

## Impact

**Severity**: HIGH
- Blocks all backend deployments
- Pod cannot reach Ready state
- Prevents end-to-end deployment testing
- Affects production reliability

**Affected Components**:
- Backend service (netguard-backend)
- All deployment workflows
- Health monitoring

---

## Symptoms

**Observed Behavior**:
```
Pod: netguard-backend-6c86c5b879-bkrzn
Status: CrashLoopBackOff
Restarts: 29 (over 113 minutes)
Ready: 0/1
```

**Error Message**:
```
Liveness probe failed: error: health rpc probe failed:
rpc error: code = NotFound desc = unknown service
```

**Pod Events**:
```
Warning  Unhealthy  12m (x84 over 112m)  kubelet  Liveness probe failed
Warning  BackOff    3m21s (x301)         kubelet  Back-off restarting failed container
```

---

## Root Cause

### Deployment Configuration

**File**: `k8s/base/deployment.yaml` (or similar)

The deployment manifest specifies gRPC health probe:
```yaml
livenessProbe:
  grpc:
    port: 9090
    service: liveness  # or empty
  initialDelaySeconds: 30
  timeoutSeconds: 5
  periodSeconds: 30
  failureThreshold: 3
```

### Application Implementation

**Problem**: Application does NOT implement `grpc.health.v1.Health` service.

**Evidence from logs**:
```
{"level":"info","ts":1760439103.0074422,"caller":"server/main.go:312","msg":"sync manager started successfully"}
{"level":"info","ts":1760439103.009895,"caller":"server/main.go:446","msg":"OutboxWorker initialized and started"}
{"level":"info","ts":1760439103.012344,"caller":"server/main.go:176","msg":"OutboxWorker health endpoint registered","path":"/healthz/worker"}
{"level":"info","ts":1760439103.0124154,"caller":"server/main.go:185","msg":"Prometheus metrics endpoint registered","path":"/metrics"}
```

**Analysis**:
1. Application starts successfully
2. Registers HTTP health endpoint (`/healthz/worker`)
3. Does NOT register gRPC health service
4. Kubernetes probes gRPC port (9090) for health service
5. gRPC server returns "NotFound" (service not registered)
6. Kubernetes marks probe failed
7. After 3 failures, pod restarted
8. Cycle repeats → CrashLoopBackOff

---

## Reproduction Steps

1. Deploy current backend code:
   ```bash
   ./scripts/deploy-local.sh backend
   ```

2. Watch pod status:
   ```bash
   kubectl get pods -n incloud-sgroups -w
   ```

3. Observe pod cycle:
   - Pod starts: Running
   - After 30s: Liveness probe starts
   - After ~1 minute: Probe fails 3 times
   - Pod restarts
   - Repeat

4. Check logs:
   ```bash
   kubectl logs -n incloud-sgroups <pod-name> -c backend
   # Shows clean startup and shutdown, no errors
   ```

5. Describe pod:
   ```bash
   kubectl describe pod -n incloud-sgroups <pod-name>
   # Shows "Liveness probe failed" events
   ```

---

## Expected Behavior

1. Application should implement gRPC health check service
2. Health service should respond to probe requests
3. Pod should remain Running and Ready
4. No restarts unless actual application failure

---

## Recommended Fix

### Option 1: Implement gRPC Health Service (RECOMMENDED)

**File**: `cmd/server/main.go`

**Add import**:
```go
import (
    "google.golang.org/grpc/health"
    "google.golang.org/grpc/health/grpc_health_v1"
)
```

**Register health service** (after creating gRPC server):
```go
// Create gRPC server
grpcServer := grpc.NewServer(
    grpc.UnaryInterceptor(...)
    // ... other options
)

// Register health service
healthServer := health.NewServer()
grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

// Set service as serving
healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

// Register other services...
// ...

// Before shutdown, mark as not serving
defer func() {
    healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
}()
```

**Update health status based on application state** (optional but recommended):
```go
// When components fail to initialize
healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

// When components recover
healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

// During graceful shutdown
healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
```

**Dependencies required**:
```bash
go get google.golang.org/grpc/health
go get google.golang.org/grpc/health/grpc_health_v1
```

**Estimated effort**: 30 minutes

---

### Option 2: Change to HTTP Health Probe

**Alternative if gRPC health service is not desired**.

**File**: `k8s/base/deployment.yaml`

Change from:
```yaml
livenessProbe:
  grpc:
    port: 9090
  initialDelaySeconds: 30
  timeoutSeconds: 5
```

To:
```yaml
livenessProbe:
  httpGet:
    path: /healthz/worker
    port: 8080  # or whatever port HTTP endpoint is on
  initialDelaySeconds: 30
  timeoutSeconds: 5
```

**Trade-offs**:
- Simpler (uses existing HTTP endpoint)
- Less standard for gRPC services
- May not reflect gRPC server health accurately

**Estimated effort**: 5 minutes

---

## Recommended Approach

**Use Option 1 (gRPC Health Service)** because:
1. Industry standard for gRPC services
2. More accurate health indication
3. Allows per-service health status
4. Better for future service mesh integration
5. Required by many gRPC tools/frameworks

---

## Testing Plan

After implementing fix:

1. **Unit Test**:
   ```bash
   # Test health service responds correctly
   grpcurl -plaintext localhost:9090 grpc.health.v1.Health/Check
   # Expected: {"status": "SERVING"}
   ```

2. **Integration Test**:
   ```bash
   # Deploy and watch pod
   ./scripts/deploy-local.sh backend
   kubectl get pods -n incloud-sgroups -w
   # Expected: Pod reaches Ready state and stays Running
   ```

3. **Stability Test**:
   ```bash
   # Watch for 10 minutes
   # Expected: No restarts, probe passes consistently
   ```

4. **Negative Test**:
   ```bash
   # Simulate health failure (if implemented)
   # Expected: Probe fails, pod restarts as expected
   ```

---

## References

### gRPC Health Checking Protocol
- Spec: https://github.com/grpc/grpc/blob/master/doc/health-checking.md
- Go implementation: https://pkg.go.dev/google.golang.org/grpc/health

### Kubernetes gRPC Probes
- Docs: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/#define-a-grpc-liveness-probe

### Example Implementations
```go
// Basic example
healthServer := health.NewServer()
grpc_health_v1.RegisterHealthServer(s, healthServer)
healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

// Advanced example with per-service health
healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
healthServer.SetServingStatus("myservice", grpc_health_v1.HealthCheckResponse_SERVING)
```

---

## Current Workaround

**None available**. Pod will continue to crash until fix is implemented.

**Temporary mitigation** (NOT recommended for production):
```yaml
# Remove liveness probe temporarily
# But this removes health checking entirely
```

---

## Related Issues

- Blocks: All deployment testing
- Blocks: Production rollout
- Related to: Health monitoring strategy

---

## Acceptance Criteria

Fix is complete when:
- [x] gRPC health service implemented in cmd/server/main.go
- [x] Health service registered with gRPC server
- [x] Health status set to SERVING on startup
- [x] Health status set to NOT_SERVING on shutdown
- [x] Unit test added for health service
- [x] Integration test passes (pod stays Running)
- [x] No CrashLoopBackOff observed for 10+ minutes
- [x] Liveness probe consistently passes

---

## Timeline

**Created**: 2025-10-14
**Priority**: P0 (Critical)
**Target Fix**: Within 24 hours
**Estimated Implementation**: 30 minutes
**Estimated Testing**: 30 minutes
**Total Estimated Time**: 1 hour

---

## Notes

### Why This Happened

Likely causes:
1. Deployment manifest updated to use gRPC probe
2. Application code not updated to implement health service
3. Mismatch between infrastructure and application

### Prevention

To prevent similar issues:
1. Always implement health endpoints before configuring probes
2. Test health probes in development before deployment
3. Add health service implementation to service template
4. Include in deployment checklist

---

## Attachments

**Pod Description**: See full output in test logs
**Application Logs**: See DEPLOYMENT_ENHANCEMENT_TEST_REPORT.md
**Probe Configuration**: Check k8s manifests

---

**Issue Status**: OPEN - Awaiting implementation
**Blocking**: Deployment testing and production rollout
**Next Action**: Backend team to implement gRPC health service

---

**Reported by**: QA Engineer (Claude Code Agent)
**Date**: 2025-10-14
**Report Version**: 1.0
