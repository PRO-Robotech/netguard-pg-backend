# SvcSvcRule kubectl Integration Tests

This directory contains test manifests and scripts for validating the SvcSvcRule feature implementation.

## Test Manifests

### Services (Prerequisites)

- **test-service-web.yaml** - Web service with HTTP/HTTPS ports
- **test-service-db.yaml** - Database service with PostgreSQL port

### SvcSvcRules

- **test-svcsvc-rule-001.yaml** - Valid SvcSvcRule (web → db, ACCEPT)
- **test-svcsvc-rule-invalid-action.yaml** - Invalid SvcSvcRule (bad action enum)

## Test Scripts

### 1. kubectl Integration Test

```bash
./test-kubectl.sh
```

**What it does:**
1. Creates prerequisite Services (web, db)
2. Verifies Services exist
3. Creates valid SvcSvcRule
4. Checks SvcSvcRule status and conditions
5. Tests invalid SvcSvcRule (validation)
6. Cleanup

**Expected Result:** All steps pass, invalid rule is rejected

### 2. Database Verification

```bash
./verify-database.sh
```

**What it does:**
1. Checks `svc_svc_rules` table
2. Checks `service_rule_refs` junction table
3. Checks `sync_outbox` for SvcSvcRule entries
4. Checks `xSvcSvcRules` fields in services table

**Expected Result:** Data is correctly stored in PostgreSQL tables

### 3. OutboxWorker Verification

```bash
./verify-outbox-worker.sh
```

**What it does:**
1. Creates Services and SvcSvcRule
2. Checks initial outbox entry (PENDING)
3. Waits for OutboxWorker (10s)
4. Checks final outbox entry (COMPLETED/FAILED)
5. Cleanup

**Expected Result:** OutboxWorker processes SvcSvcRule entry (status: COMPLETED)

## Prerequisites

- Kubernetes cluster with netguard-pg-backend deployed
- Namespace: `incloud-sgroups`
- PostgreSQL pod: `netguard-postgresql-0`
- kubectl configured with correct context
- jq installed (for JSON processing)

## Running All Tests

```bash
# Full test suite
./test-kubectl.sh
./verify-database.sh
./verify-outbox-worker.sh
```

## Troubleshooting

### SvcSvcRule not getting Ready condition

Check:
1. ProcessSvcSvcRuleConditions is called (check backend logs)
2. ServiceFrom and ServiceTo exist
3. No backend errors in logs

### OutboxWorker not processing entries

Check:
1. OutboxWorker is enabled (`OUTBOX_WORKER_ENABLED=true`)
2. OutboxWorker pod is running
3. Poll interval (`OUTBOX_WORKER_POLL_INTERVAL`)
4. Backend logs for errors

### Database triggers not firing

Check migration 029:
```sql
\d+ svc_svc_rules
\d+ service_rule_refs
SELECT * FROM pg_trigger WHERE tgrelid = 'svc_svc_rules'::regclass;
```

## Phase 4 DoD Checklist

- [x] Test manifests created (4 files)
- [x] kubectl test script created
- [x] Database verification script created
- [x] OutboxWorker verification script created
- [ ] Tests executed successfully (pending cluster availability)
- [ ] Documentation created

## Related Documentation

- **Feature**: `docs/features/svcsvc-rule/`
- **Migration**: `migrations/029_create_svc_svc_rule_tables.sql`
- **Domain Model**: `internal/domain/models/svcsvc-rule.go`
- **ConditionManager**: `internal/application/services/condition_manager.go`
