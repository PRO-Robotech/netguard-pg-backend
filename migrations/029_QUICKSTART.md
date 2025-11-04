# Migration 029 - Quick Start Guide

**For developers who just want to apply and verify the migration.**

---

## 1. Apply Migration (30 seconds)

```bash
# Set your PostgreSQL connection string
export PG_URI="postgres://netguard:netguard@localhost:5432/netguard?sslmode=disable"

# Apply migration
./bin/goose -dir migrations postgres "$PG_URI" up

# Expected output:
# OK   029_create_svc_svc_rule_tables.sql
```

---

## 2. Verify Migration (30 seconds)

```bash
# Run automated verification
psql "$PG_URI" -f migrations/029_VERIFY.sql

# Expected summary at the end:
# Tables: 2
# Indexes: 12
# Triggers: 4
# Functions: 4
# Services Columns: 2
```

---

## 3. Quick Functional Test (1 minute)

```bash
psql "$PG_URI" << 'EOF'
-- Create test rule
INSERT INTO svc_svc_rules (namespace, name, service_from_ref, service_to_ref, action)
VALUES (
    'test', 'quick-test',
    '{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "svc-a", "namespace": "test"}'::jsonb,
    '{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "svc-b", "namespace": "test"}'::jsonb,
    'ACCEPT'
);

-- Check results
SELECT '✓ Rule created' as check;

SELECT
    CASE WHEN COUNT(*) = 2 THEN '✓ Junction entries (2)'
         ELSE '✗ Junction entries: ' || COUNT(*)
    END as check
FROM service_rule_refs
WHERE rule_id = (SELECT id FROM svc_svc_rules WHERE name = 'quick-test');

SELECT
    CASE WHEN COUNT(*) = 1 THEN '✓ OUTBOX entry (1)'
         ELSE '✗ OUTBOX entries: ' || COUNT(*)
    END as check
FROM sync_outbox
WHERE resource_type = 'SvcSvcRule' AND resource_name = 'quick-test';

-- Cleanup
DELETE FROM svc_svc_rules WHERE name = 'quick-test';

SELECT '✓ Cleanup complete' as check;
EOF
```

**Expected output:**
```
✓ Rule created
✓ Junction entries (2)
✓ OUTBOX entry (1)
✓ Cleanup complete
```

---

## 4. Test Immutability (30 seconds)

```bash
psql "$PG_URI" << 'EOF'
-- Create rule
INSERT INTO svc_svc_rules (namespace, name, service_from_ref, service_to_ref, action)
VALUES (
    'test', 'immutable-test',
    '{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "svc-a", "namespace": "test"}'::jsonb,
    '{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "svc-b", "namespace": "test"}'::jsonb,
    'ACCEPT'
);

-- Try to change service_from_ref (should fail!)
UPDATE svc_svc_rules
SET service_from_ref = '{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "svc-c", "namespace": "test"}'::jsonb
WHERE name = 'immutable-test';

-- This should NOT execute (error expected above)
EOF
```

**Expected output:**
```
ERROR:  serviceFrom and serviceTo are immutable fields (validation #10, #11)
```

**Cleanup after error:**
```bash
psql "$PG_URI" -c "DELETE FROM svc_svc_rules WHERE name = 'immutable-test';"
```

---

## 5. Rollback Test (Optional, 1 minute)

**WARNING**: This deletes all SvcSvcRule data!

```bash
# Rollback migration
./bin/goose -dir migrations postgres "$PG_URI" down

# Verify version is 28
psql "$PG_URI" -c "SELECT version_id FROM netguard_db_ver ORDER BY id DESC LIMIT 1;"

# Re-apply migration
./bin/goose -dir migrations postgres "$PG_URI" up

# Verify version is 29
psql "$PG_URI" -c "SELECT version_id FROM netguard_db_ver ORDER BY id DESC LIMIT 1;"
```

---

## Common Issues

### Issue: "uuid_generate_v5 does not exist"
**Fix:**
```bash
psql "$PG_URI" -c "CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\";"
```

### Issue: "table sync_outbox does not exist"
**Fix:** Run earlier migrations first
```bash
./bin/goose -dir migrations postgres "$PG_URI" up
```

### Issue: "migration already applied"
**Check version:**
```bash
psql "$PG_URI" -c "SELECT version_id FROM netguard_db_ver ORDER BY id DESC LIMIT 1;"
```
If version is 29+, migration already applied (skip).

---

## More Information

- **Full testing guide**: 029_ACCEPTANCE_CHECKLIST.md
- **Design decisions**: 029_NOTES.md
- **Quick reference**: 029_SUMMARY.md
- **Automated verification**: 029_VERIFY.sql

---

## Success Checklist

After running the above:

- [x] Migration applied (version 29)
- [x] Verification script passed (12 indexes, 4 triggers)
- [x] Functional test passed (rule created, junction populated, outbox entry exists)
- [x] Immutability test passed (error raised on service ref update)
- [x] (Optional) Rollback test passed

**Status**: ✅ Migration 029 is working correctly!

---

**Estimated total time**: 2-3 minutes
