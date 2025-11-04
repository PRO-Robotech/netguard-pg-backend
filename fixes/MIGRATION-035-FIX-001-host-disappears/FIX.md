# [OBSOLETE] FIX-001: Host Resources Disappear After Creation

> ⚠️ **OBSOLETE**: This fix document references Migration 035 which does not exist in the current codebase.
> The migration system was refactored, and there are only 28 migrations (001-028).
> This issue may have been resolved in migrations 026-028 (entity/process resource triggers).

**Fix ID**: MIGRATION-035-FIX-001
**Parent**: Migration 035 (auto sync ready from conditions) ← **NON-EXISTENT**
**Type**: fix
**Status**: ~~todo~~ **OBSOLETE**
**Severity**: P0 CRITICAL BLOCKER
**Assignee**: backend-api-dev OR orm-expert
**Created**: 2025-10-15
**QA Engineer**: agent.qa-engineer

---

## Problem Description

**Host resources DISAPPEAR after successful creation**, causing:
- ❌ kubectl get shows "Not Found" after ~10 seconds
- ❌ Host DELETED from database (cascade delete)
- ❌ OutboxWorker stuck in retry loop ("resource not found")
- ❌ User cannot create stable Host resources

**Impact**: 🚨 **BLOCKS PRODUCTION DEPLOYMENT**

---

## Steps to Reproduce

1. Create Host via kubectl:
   ```bash
   cat <<EOF | kubectl apply -f -
   apiVersion: netguard.sgroups.io/v1beta1
   kind: Host
   metadata:
     name: test-host
     namespace: incloud-sgroups
   spec:
     uuid: "11111111-2222-3333-4444-555555555001"
   EOF
   ```

2. Verify created:
   ```bash
   kubectl get host test-host -n incloud-sgroups
   # Shows: test-host ... 5s
   ```

3. Wait 15 seconds

4. Check again:
   ```bash
   kubectl get host test-host -n incloud-sgroups
   # ERROR: Not Found ❌
   ```

5. Check database:
   ```sql
   SELECT * FROM hosts WHERE name='test-host';
   # (0 rows) - Host deleted! ❌
   ```

**Expected**: Host should stay alive indefinitely

**Actual**: Host disappears after ~10 seconds

---

## Root Cause Analysis

### Timeline of Events

```
T+0s:  User creates Host via kubectl apply
       → apiserver creates Host in DB
       → trigger creates CREATE outbox entry

T+10s: OutboxWorker processes CREATE entry
       → Syncs to SGROUP (SUCCESS)
       → Calls markEntityResourceReady() to update status
       ⚠️ THIS SOMEHOW DELETES k8s_metadata row

T+10s: CASCADE DELETE triggered
       → k8s_metadata deleted
       → hosts.resource_version FK CASCADE deletes Host
       → trigger_host_before_delete() fires
       → Creates DELETE outbox entry

T+20s: OutboxWorker processes DELETE entry
       → Syncs DELETE to SGROUP
       → Deletes Host from DB (cleanup)
       → DELETE entry removed

T+30s: OutboxWorker tries CREATE again
       → ERROR: "resource not found (deleted)"
       → Repeats forever...
```

### Suspected Code Issue

**File**: `internal/sync/worker/process_entity.go`
**Function**: `markEntityResourceReady()`
**Lines**: 234-319

```go
func (w *OutboxWorker) markEntityResourceReady(
	ctx context.Context,
	resourceType string,
	namespace string,
	name string,
	outboxID interface{},
) error {
	// Get writer from registry
	writer, err := w.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Load the resource again
	resource, err := w.loadEntityResource(ctx, resourceType, namespace, name)
	if err != nil {
		return fmt.Errorf("failed to load resource for update: %w", err)
	}

	// Update the resource's Meta to set Ready condition
	switch r := resource.(type) {
	case *models.Host:
		r.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Synced to SGROUP")
		r.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Successfully synced")
		resourceScope = ports.NewResourceIdentifierScope(models.ResourceIdentifier{
			Name:      r.Name,
			Namespace: r.Namespace,
		})
		// ⚠️ SUSPECTED ISSUE: Does this DELETE instead of UPDATE?
		if err := writer.SyncHosts(ctx, []models.Host{*r}, resourceScope, ports.ConditionOnlyOperation{}); err != nil {
			return fmt.Errorf("failed to update host: %w", err)
		}
	// ...
	}

	// Commit the changes
	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	// Delete outbox entry
	if err := w.outboxRepo.Delete(ctx, outboxID.(uuid.UUID)); err != nil {
		return fmt.Errorf("failed to delete outbox entry: %w", err)
	}

	return nil
}
```

**Questions**:
1. What does `ports.ConditionOnlyOperation{}` actually do?
2. Does `writer.SyncHosts()` have DELETE logic when it should only UPDATE?
3. Is the resourceScope filter causing incorrect behavior?
4. Could there be a transaction issue causing k8s_metadata deletion?

### Database Evidence

**Foreign Key Constraint**:
```sql
-- From: \d+ hosts
hosts.resource_version → k8s_metadata.resource_version ON DELETE CASCADE
```

This means: If k8s_metadata row is deleted → Host is CASCADE DELETED

**Backend logs show**:
```
"processing entity resource" operation="DELETE"
"SGROUP sync successful"
"resource and outbox entry deleted from DB"
"DELETE operation complete - resource deleted from DB"
```

DELETE entry WAS created (by trigger_host_before_delete when cascade fired),
processed by Worker, and then Host actually deleted.

---

## Files to Investigate

### Priority 1: Writer Implementation

**File**: `internal/infrastructure/repositories/pg/writers/host.go`
**Function**: `SyncHosts()`
**Investigation**:
- Does it DELETE k8s_metadata when using ConditionOnlyOperation?
- Is there a bug in UPDATE vs DELETE logic?
- Are transactions handled correctly?

### Priority 2: Operation Flags

**File**: `internal/domain/ports/writer.go`
**Type**: `ConditionOnlyOperation`
**Investigation**:
- What is this flag supposed to do?
- Is it implemented correctly in host writer?
- Does it prevent unintended deletes?

### Priority 3: Resource Scope

**File**: `internal/domain/ports/scope.go`
**Function**: `NewResourceIdentifierScope()`
**Investigation**:
- Is the scope filter correct?
- Could it be matching multiple rows?
- Could it be causing deletes instead of updates?

---

## Proposed Fix Strategy

### Option A: Fix writer.SyncHosts() (PREFERRED)

**Root cause fix** - Ensure SyncHosts with ConditionOnlyOperation:
- ✅ ONLY updates k8s_metadata.conditions
- ✅ NEVER deletes k8s_metadata
- ✅ Uses UPDATE, not DELETE+INSERT pattern

**Changes needed**:
1. Review `host.go` SyncHosts() implementation
2. Verify ConditionOnlyOperation is respected
3. Add safeguards against accidental deletion
4. Add logging for k8s_metadata operations

### Option B: Defensive trigger (BAND-AID)

**Make trigger_host_before_delete() smarter**:
```sql
-- Check if DELETE is from CASCADE (not user-initiated)
-- Only create DELETE entry if user explicitly requested delete
```

**NOT PREFERRED** - Doesn't fix root cause

### Option C: Change FK constraint (BREAKING)

```sql
ALTER TABLE hosts
DROP CONSTRAINT hosts_resource_version_fkey,
ADD CONSTRAINT hosts_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON DELETE RESTRICT;  -- Changed from CASCADE
```

**RISKY** - Would break intentional deletions

---

## Testing Requirements

Before declaring fix complete:

### Functional Tests

1. ✅ Create Host → Host stays alive for 5 minutes
   ```bash
   kubectl apply -f test-host.yaml
   sleep 300
   kubectl get host test-host  # Should exist
   ```

2. ✅ Host status updates correctly
   ```bash
   kubectl get host test-host -o jsonpath='{.status.conditions}'
   # Should show Ready=True, Synced=True
   ```

3. ✅ No spurious DELETE operations
   ```sql
   SELECT * FROM sync_outbox WHERE operation='DELETE'
   -- Should be empty (unless user explicitly deleted something)
   ```

4. ✅ k8s_metadata not deleted
   ```sql
   SELECT COUNT(*) FROM k8s_metadata m
   JOIN hosts h ON h.resource_version = m.resource_version
   WHERE h.name='test-host';
   -- Should return 1
   ```

### Regression Tests

5. ✅ Network resource also stays alive
6. ✅ AddressGroup resource stays alive
7. ✅ Intentional deletion still works
   ```bash
   kubectl delete host test-host
   # Should delete successfully
   ```

### Load Tests

8. ✅ Create 10 Hosts simultaneously → all stay alive
9. ✅ OutboxWorker processes entries without errors
10. ✅ No orphaned outbox entries

---

## Acceptance Criteria

- [ ] Root cause identified and documented
- [ ] Fix implemented in appropriate file(s)
- [ ] All functional tests pass (1-4)
- [ ] All regression tests pass (5-7)
- [ ] Load tests pass (8-10)
- [ ] Code reviewed by senior engineer
- [ ] Tested in staging environment
- [ ] No kubectl "Not Found" errors
- [ ] No "resource not found (deleted)" backend errors
- [ ] Documentation updated with findings

---

## Related Documents

- **Test Report**: `/Users/zhd/Projects/newPro/netguard-pg-backend/E2E_KUBECTL_TEST_REPORT_CRITICAL_FINDING.md`
- **Backend Logs**: `/tmp/e2e-host-deletion-backend-logs.txt`
- **Migration 035**: `migrations/029_auto_sync_ready_from_conditions.sql`

---

## Priority Justification

**Why P0**:
- Makes system completely unusable for Host creation
- Causes data loss
- Blocks production deployment
- Affects all entity resources (Host, Network, AddressGroup, Service)
- User-facing impact: cannot create any resources

**Business Impact**:
- Cannot deploy to production
- Cannot complete Migration 035 rollout
- Dev/staging environments unstable
- Customer demos impossible

---

## Next Steps

1. **Assign to backend-api-dev OR orm-expert** (whoever owns writer implementation)
2. **Priority investigation**: Why does markEntityResourceReady() delete k8s_metadata?
3. **Code review**: SyncHosts() implementation
4. **Fix and test**: Implement Option A (preferred)
5. **Validation**: Run full E2E test suite
6. **Deploy fix**: After QA approval

---

**Status**: 🚨 OPEN - AWAITING FIX
**Blocking**: Migration 035, Production Deployment
**Est. Fix Time**: 4-8 hours (depends on root cause complexity)
