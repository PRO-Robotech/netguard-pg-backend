# Migration 029 - SvcSvcRule Tables - Acceptance Checklist

**Migration**: `029_create_svc_svc_rule_tables.sql`
**Story**: CLOUD-210-svcsvc-rule
**Date**: 2025-10-20
**Author**: migration-expert (Claude Code Agent)

---

## Overview

Migration 029 creates the complete database infrastructure for Service-to-Service firewall rules:
- **2 tables**: `svc_svc_rules`, `service_rule_refs`
- **12 indexes**: UNIQUE, JSONB (B-tree), junction, services (GIN)
- **4 triggers**: Junction maintenance, OUTBOX upsert, OUTBOX delete, xSvcSvcRules maintenance
- **2 services columns**: `xsvcsvc_rules_as_from`, `xsvcsvc_rules_as_to`

---

## Pre-Migration Checklist

### Environment Verification
- [ ] PostgreSQL version 12+ (required for JSONB features)
- [ ] Database connection established
- [ ] Current migration version checked: `SELECT version_id FROM netguard_db_ver ORDER BY id DESC LIMIT 1;`
- [ ] Expected: version 28 or higher
- [ ] Backup created (if production)

### Prerequisites
- [ ] `uuid-ossp` extension exists: `SELECT * FROM pg_extension WHERE extname = 'uuid-ossp';`
- [ ] `sync_outbox` table exists (migration 025)
- [ ] `services` table exists (base schema)
- [ ] `sync_operation`, `target_system`, `outbox_status` enums exist (migration 025)

---

## Migration Execution

### Step 1: Run Migration
```bash
# Using Goose
./bin/goose -dir migrations postgres "$PG_URI" up

# Or using kubectl (if in Kubernetes)
kubectl exec netguard-postgresql-0 -n incloud-sgroups -- \
  env PGPASSWORD=netguard psql -U netguard -d netguard \
  -f /migrations/029_create_svc_svc_rule_tables.sql
```

### Step 2: Verify Execution
- [ ] Migration completed without errors
- [ ] Check migration version: Should be 29
  ```sql
  SELECT version_id FROM netguard_db_ver ORDER BY id DESC LIMIT 1;
  ```

---

## Post-Migration Verification

### Part 1: Tables Created

#### Main Table: svc_svc_rules
```sql
\d svc_svc_rules
```

**Expected columns**:
- [ ] `id` (UUID, PRIMARY KEY)
- [ ] `namespace` (VARCHAR(253), NOT NULL)
- [ ] `name` (VARCHAR(253), NOT NULL)
- [ ] `service_from_ref` (JSONB, NOT NULL)
- [ ] `service_to_ref` (JSONB, NOT NULL)
- [ ] `action` (VARCHAR(50), NOT NULL, CHECK constraint)
- [ ] `priority` (INT, NOT NULL, DEFAULT 0, CHECK constraint)
- [ ] `logs` (BOOLEAN, NOT NULL, DEFAULT false)
- [ ] `trace` (BOOLEAN, NOT NULL, DEFAULT false)
- [ ] `created_at` (TIMESTAMPTZ, NOT NULL)
- [ ] `updated_at` (TIMESTAMPTZ, NOT NULL)
- [ ] `resource_version` (VARCHAR(50))

**Constraints**:
- [ ] CHECK: `action IN ('ACCEPT', 'DROP')`
- [ ] CHECK: `priority >= 0 AND priority <= 1000`

#### Junction Table: service_rule_refs
```sql
\d service_rule_refs
```

**Expected columns**:
- [ ] `id` (UUID, PRIMARY KEY)
- [ ] `service_ref` (VARCHAR(510), NOT NULL)
- [ ] `rule_id` (UUID, NOT NULL, FK to svc_svc_rules)
- [ ] `role` (VARCHAR(50), NOT NULL, CHECK constraint)
- [ ] `created_at` (TIMESTAMPTZ, NOT NULL)

**Constraints**:
- [ ] CHECK: `role IN ('SERVICE_FROM', 'SERVICE_TO')`
- [ ] FK: `rule_id REFERENCES svc_svc_rules(id) ON DELETE CASCADE`

### Part 2: Indexes Created (12 total)

```sql
-- List all indexes
\di *svc_svc_rules*
\di *service_rule_refs*
\di *xsvcsvc_rules*
```

**Expected indexes**:

**svc_svc_rules table (7 indexes)**:
- [ ] `idx_svc_svc_rules_unique_pair` - UNIQUE (namespace + JSONB fields)
- [ ] `idx_svc_svc_rules_service_from_name` - B-tree on (service_from_ref->>'name')
- [ ] `idx_svc_svc_rules_service_from_namespace` - B-tree on (service_from_ref->>'namespace')
- [ ] `idx_svc_svc_rules_service_to_name` - B-tree on (service_to_ref->>'name')
- [ ] `idx_svc_svc_rules_service_to_namespace` - B-tree on (service_to_ref->>'namespace')
- [ ] `idx_svc_svc_rules_namespace_name` - Composite (namespace, name)
- [ ] `idx_svc_svc_rules_action` - Single column (action)

**service_rule_refs table (2 indexes)**:
- [ ] `idx_service_rule_refs_service` - Single column (service_ref)
- [ ] `idx_service_rule_refs_rule_id` - Single column (rule_id)

**services table (2 GIN indexes)**:
- [ ] `idx_services_xsvcsvc_rules_as_from` - GIN (xsvcsvc_rules_as_from)
- [ ] `idx_services_xsvcsvc_rules_as_to` - GIN (xsvcsvc_rules_as_to)

**Plus 1 primary key index** = 12 total

### Part 3: Triggers Created (4 total)

```sql
-- List all triggers
SELECT tgname, tgrelid::regclass, tgtype, tgenabled
FROM pg_trigger
WHERE tgname LIKE '%svcsvc%' OR tgname LIKE '%service_rule%';
```

**Expected triggers**:
- [ ] `trg_sync_service_rule_refs` (on `svc_svc_rules`, AFTER INSERT/UPDATE/DELETE)
- [ ] `trg_svcsvc_rule_upsert_outbox` (on `svc_svc_rules`, AFTER INSERT/UPDATE)
- [ ] `trg_svcsvc_rule_delete_outbox` (on `svc_svc_rules`, AFTER DELETE)
- [ ] `trg_update_service_xsvcsvc_rules` (on `service_rule_refs`, AFTER INSERT/DELETE)

**Verify trigger functions**:
```sql
\df *svcsvc*
\df sync_service_rule_refs
\df update_service_xsvcsvc_rules
```

**Expected functions**:
- [ ] `sync_service_rule_refs()`
- [ ] `trigger_svcsvc_rule_upsert_outbox()`
- [ ] `trigger_svcsvc_rule_delete_outbox()`
- [ ] `update_service_xsvcsvc_rules()`

### Part 4: Services Table Updates

```sql
\d+ services
```

**Expected new columns**:
- [ ] `xsvcsvc_rules_as_from` (JSONB, NOT NULL, DEFAULT '[]'::jsonb)
- [ ] `xsvcsvc_rules_as_to` (JSONB, NOT NULL, DEFAULT '[]'::jsonb)

**Verify defaults**:
```sql
SELECT COUNT(*) as total_services,
       COUNT(*) FILTER (WHERE xsvcsvc_rules_as_from = '[]'::jsonb) as default_from,
       COUNT(*) FILTER (WHERE xsvcsvc_rules_as_to = '[]'::jsonb) as default_to
FROM services;
```
- [ ] All existing services have `[]` defaults

---

## Functional Testing

### Test 1: INSERT Rule (Triggers 1, 2, 4)

```sql
-- Create test rule
INSERT INTO svc_svc_rules (
    namespace,
    name,
    service_from_ref,
    service_to_ref,
    action
) VALUES (
    'test-ns',
    'test-rule-001',
    '{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "service-a", "namespace": "test-ns"}'::jsonb,
    '{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "service-b", "namespace": "test-ns"}'::jsonb,
    'ACCEPT'
);
```

**Verify**:

**1. Junction table populated (Trigger 1)**:
```sql
SELECT * FROM service_rule_refs
WHERE rule_id = (SELECT id FROM svc_svc_rules WHERE name = 'test-rule-001');
```
- [ ] 2 entries exist (SERVICE_FROM + SERVICE_TO)
- [ ] `service_ref` format: "test-ns/service-a" and "test-ns/service-b"

**2. OUTBOX entry created (Trigger 2)**:
```sql
SELECT * FROM sync_outbox
WHERE resource_type = 'SvcSvcRule'
  AND resource_name = 'test-rule-001'
ORDER BY created_at DESC LIMIT 1;
```
- [ ] Entry exists with operation = 'CREATE'
- [ ] status = 'PENDING'
- [ ] target_system = 'SGROUP'
- [ ] payload contains service_from, service_to, action, priority, logs, trace

**3. Services.xSvcSvcRules updated (Trigger 4)**:
```sql
-- First, create test services if they don't exist
INSERT INTO services (namespace, name)
VALUES ('test-ns', 'service-a'), ('test-ns', 'service-b')
ON CONFLICT DO NOTHING;

-- Check xSvcSvcRules arrays
SELECT name, xsvcsvc_rules_as_from, xsvcsvc_rules_as_to
FROM services
WHERE namespace = 'test-ns' AND name IN ('service-a', 'service-b');
```
- [ ] service-a: `xsvcsvc_rules_as_from` contains rule ref (kind: "SvcSvcRule")
- [ ] service-b: `xsvcsvc_rules_as_to` contains rule ref

### Test 2: UPDATE Rule (Trigger 2 - payload refresh)

```sql
-- Update mutable field
UPDATE svc_svc_rules
SET priority = 100, logs = true
WHERE name = 'test-rule-001';
```

**Verify OUTBOX updated**:
```sql
SELECT payload FROM sync_outbox
WHERE resource_type = 'SvcSvcRule'
  AND resource_name = 'test-rule-001'
ORDER BY updated_at DESC LIMIT 1;
```
- [ ] payload contains updated priority (100)
- [ ] payload contains updated logs (true)
- [ ] operation = 'UPDATE'

### Test 3: UPDATE Rule - Immutability Check (Trigger 1)

```sql
-- Try to update service_from_ref (should FAIL)
UPDATE svc_svc_rules
SET service_from_ref = '{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "service-c", "namespace": "test-ns"}'::jsonb
WHERE name = 'test-rule-001';
```

**Expected result**:
- [ ] Error raised: "serviceFrom and serviceTo are immutable fields (validation #10, #11)"
- [ ] Rule not updated

### Test 4: DELETE Rule (Triggers 3, 4)

```sql
-- Delete test rule
DELETE FROM svc_svc_rules WHERE name = 'test-rule-001';
```

**Verify**:

**1. OUTBOX DELETE entry (Trigger 3)**:
```sql
SELECT * FROM sync_outbox
WHERE resource_type = 'SvcSvcRule'
  AND resource_name = 'test-rule-001'
  AND operation = 'DELETE'
ORDER BY created_at DESC LIMIT 1;
```
- [ ] Entry exists with operation = 'DELETE'
- [ ] status = 'PENDING'

**2. Junction table cleaned up**:
```sql
SELECT COUNT(*) FROM service_rule_refs
WHERE rule_id = (SELECT id FROM svc_svc_rules WHERE name = 'test-rule-001');
```
- [ ] 0 entries (CASCADE delete worked)

**3. Services.xSvcSvcRules cleaned up (Trigger 4)**:
```sql
SELECT name, xsvcsvc_rules_as_from, xsvcsvc_rules_as_to
FROM services
WHERE namespace = 'test-ns' AND name IN ('service-a', 'service-b');
```
- [ ] service-a: `xsvcsvc_rules_as_from` = `[]` (or empty array)
- [ ] service-b: `xsvcsvc_rules_as_to` = `[]`

### Test 5: UNIQUE Constraint

```sql
-- Create rule
INSERT INTO svc_svc_rules (
    namespace, name, service_from_ref, service_to_ref, action
) VALUES (
    'test-ns', 'test-rule-unique',
    '{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "svc-x", "namespace": "test-ns"}'::jsonb,
    '{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "svc-y", "namespace": "test-ns"}'::jsonb,
    'ACCEPT'
);

-- Try to create duplicate (should FAIL)
INSERT INTO svc_svc_rules (
    namespace, name, service_from_ref, service_to_ref, action
) VALUES (
    'test-ns', 'test-rule-unique-2',  -- Different name
    '{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "svc-x", "namespace": "test-ns"}'::jsonb,
    '{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "svc-y", "namespace": "test-ns"}'::jsonb,
    'ACCEPT'
);
```

**Expected result**:
- [ ] Second INSERT fails with UNIQUE constraint violation
- [ ] Error message mentions `idx_svc_svc_rules_unique_pair`

---

## Performance Verification

### Index Usage Analysis

```sql
-- Check if indexes are being used
EXPLAIN (ANALYZE, BUFFERS)
SELECT * FROM svc_svc_rules
WHERE service_from_ref->>'name' = 'service-a';

EXPLAIN (ANALYZE, BUFFERS)
SELECT * FROM service_rule_refs
WHERE service_ref = 'test-ns/service-a';
```

**Verify**:
- [ ] Query on `service_from_ref->>'name'` uses `idx_svc_svc_rules_service_from_name`
- [ ] Query on `service_ref` uses `idx_service_rule_refs_service`

### Trigger Performance

```sql
-- Measure trigger overhead (should be < 50ms for 100 inserts)
\timing on
BEGIN;
DO $$
BEGIN
    FOR i IN 1..100 LOOP
        INSERT INTO svc_svc_rules (
            namespace, name, service_from_ref, service_to_ref, action
        ) VALUES (
            'perf-test',
            'rule-' || i,
            jsonb_build_object('apiVersion', 'netguard.sgroups.io/v1beta1', 'kind', 'Service', 'name', 'svc-from-' || i, 'namespace', 'perf-test'),
            jsonb_build_object('apiVersion', 'netguard.sgroups.io/v1beta1', 'kind', 'Service', 'name', 'svc-to-' || i, 'namespace', 'perf-test'),
            'ACCEPT'
        );
    END LOOP;
END $$;
COMMIT;

-- Check results
SELECT COUNT(*) as rules_created FROM svc_svc_rules WHERE namespace = 'perf-test';
SELECT COUNT(*) as junction_entries FROM service_rule_refs WHERE service_ref LIKE 'perf-test/%';
SELECT COUNT(*) as outbox_entries FROM sync_outbox WHERE resource_namespace = 'perf-test';
```

**Expected**:
- [ ] 100 rules created
- [ ] 200 junction entries (2 per rule)
- [ ] 100 outbox entries (1 per rule)
- [ ] Total time < 5 seconds

**Cleanup**:
```sql
DELETE FROM svc_svc_rules WHERE namespace = 'perf-test';
```

---

## Rollback Testing

### Prepare Rollback Test

```sql
-- Create test data
INSERT INTO svc_svc_rules (namespace, name, service_from_ref, service_to_ref, action)
VALUES (
    'rollback-test', 'rollback-rule',
    '{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "svc-a", "namespace": "rollback-test"}'::jsonb,
    '{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "svc-b", "namespace": "rollback-test"}'::jsonb,
    'ACCEPT'
);
```

### Execute Rollback

```bash
./bin/goose -dir migrations postgres "$PG_URI" down
```

### Verify Rollback

**Tables removed**:
```sql
\dt svc_svc_rules
\dt service_rule_refs
```
- [ ] Tables do not exist

**Services columns removed**:
```sql
\d services
```
- [ ] `xsvcsvc_rules_as_from` column does not exist
- [ ] `xsvcsvc_rules_as_to` column does not exist

**Triggers removed**:
```sql
SELECT tgname FROM pg_trigger WHERE tgname LIKE '%svcsvc%';
```
- [ ] No triggers found

**Functions removed**:
```sql
\df *svcsvc*
```
- [ ] No functions found

**Migration version**:
```sql
SELECT version_id FROM netguard_db_ver ORDER BY id DESC LIMIT 1;
```
- [ ] Version is 28 (previous migration)

### Re-apply Migration

```bash
./bin/goose -dir migrations postgres "$PG_URI" up
```

**Verify**:
- [ ] Migration applies successfully
- [ ] All tables, indexes, triggers recreated
- [ ] Version is 29

---

## Cleanup

```sql
-- Remove all test data
DELETE FROM svc_svc_rules WHERE namespace IN ('test-ns', 'rollback-test', 'perf-test');
DELETE FROM services WHERE namespace IN ('test-ns', 'rollback-test', 'perf-test');
```

---

## Final Acceptance

### Summary Checklist

- [ ] All tables created (2)
- [ ] All indexes created (12)
- [ ] All triggers created (4)
- [ ] All functions created (4)
- [ ] Services columns added (2)
- [ ] INSERT creates junction entries + outbox entry + updates xSvcSvcRules
- [ ] UPDATE refreshes outbox payload
- [ ] UPDATE enforces immutability of service refs
- [ ] DELETE creates outbox entry + cleans up junction + updates xSvcSvcRules
- [ ] UNIQUE constraint prevents duplicate rules
- [ ] Indexes are being used by queries
- [ ] Rollback works correctly
- [ ] Performance is acceptable (< 50ms per rule with all triggers)

### Sign-off

**Tested by**: _________________
**Date**: _________________
**Status**: ⬜ PASS | ⬜ FAIL
**Notes**: _________________

---

## Troubleshooting

### Common Issues

**Issue**: Trigger function error "relation does not exist"
- **Cause**: Migration run out of order
- **Fix**: Verify migration 025 (sync_outbox) exists

**Issue**: UNIQUE constraint violation on re-run
- **Cause**: Test data not cleaned up
- **Fix**: Run cleanup queries above

**Issue**: Services columns already exist
- **Cause**: Migration already applied
- **Fix**: Check migration version, skip if 29+

**Issue**: Performance degradation
- **Cause**: Too many outbox entries
- **Fix**: Verify OutboxWorker is running and processing entries

---

**Document Version**: 1.0
**Last Updated**: 2025-10-20
