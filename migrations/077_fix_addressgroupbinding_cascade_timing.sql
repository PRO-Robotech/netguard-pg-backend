-- +goose Up
-- Migration 077: Fix AddressGroupBinding CASCADE DELETE timing issue
--
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
-- CRITICAL BUG FIX: AddressGroupBinding deleted BEFORE AFTER UPDATE trigger can process it
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
--
-- Root Cause:
--   Foreign Key `address_group_bindings_resource_version_fkey` has `ON DELETE CASCADE`.
--   When k8s_metadata record is deleted (or deletion_timestamp is set), CASCADE DELETE
--   removes the binding IMMEDIATELY, BEFORE the AFTER UPDATE trigger on k8s_metadata
--   can process the deletion_timestamp change.
--
-- Impact:
--   1. kubectl delete addressgroupbinding → deletion_timestamp set in k8s_metadata
--   2. CASCADE DELETE removes binding from address_group_bindings table
--   3. AFTER UPDATE trigger fires: trigger_addressgroupbinding_on_deletion_mark()
--   4. Trigger tries to SELECT binding → NOT FOUND (already deleted by CASCADE!)
--   5. PostgreSQL logs: "WARNING: AddressGroupBinding not found for resource_version XXX"
--   6. Service.aggregated_address_groups NOT updated (still contains deleted binding)
--   7. Service UPDATE outbox entry NOT created
--   8. SGROUP doesn't receive UPDATE → keeps old binding data ❌
--
-- Correct Flow (like HostBinding):
--   1. kubectl delete → BEFORE DELETE trigger sets deletion_timestamp
--   2. AFTER UPDATE trigger fires (binding STILL EXISTS at this point)
--   3. Trigger reads binding details, updates Service.aggregated_address_groups
--   4. Service UPDATE trigger creates SGROUP outbox entry
--   5. Physical deletion happens LATER via OutboxWorker after sync success
--
-- Solution:
--   Change FK constraint from `ON DELETE CASCADE` to `ON DELETE RESTRICT`.
--   Physical deletion will be handled by OutboxWorker AFTER successful synchronization.
--   This matches the pattern established in Migrations 055/056 for coordinated deletion.
--
-- Related Migrations:
--   - Migration 055: Changed AddressGroupBinding FK to RESTRICT (for other FKs)
--   - Migration 072: Added Service UPDATE to deletion trigger (but trigger doesn't fire due to this bug!)
--   - Migration 075: Fixed BEFORE DELETE trigger to RETURN OLD
--   - Migration 076: Added aggregated_address_groups to Service payload
--
-- Date: 2025-10-31
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════

-- +goose StatementBegin

-- Drop existing CASCADE constraint
ALTER TABLE address_group_bindings
DROP CONSTRAINT IF EXISTS address_group_bindings_resource_version_fkey;

-- Recreate with RESTRICT (no automatic cascade deletion)
ALTER TABLE address_group_bindings
ADD CONSTRAINT address_group_bindings_resource_version_fkey
FOREIGN KEY (resource_version) REFERENCES k8s_metadata(resource_version) ON DELETE RESTRICT;

COMMENT ON CONSTRAINT address_group_bindings_resource_version_fkey ON address_group_bindings IS
'Migration 077: Changed from CASCADE to RESTRICT to fix timing issue.
Physical deletion happens via OutboxWorker AFTER successful SGROUP synchronization.
This ensures AFTER UPDATE trigger on k8s_metadata can read binding details before deletion.';

DO $$
BEGIN
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '[Migration 077] AddressGroupBinding CASCADE timing fix COMPLETE';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '';
    RAISE NOTICE 'FIXED: Foreign Key constraint changed from CASCADE to RESTRICT';
    RAISE NOTICE '';
    RAISE NOTICE 'New deletion flow:';
    RAISE NOTICE '  1. kubectl delete → deletion_timestamp set in k8s_metadata';
    RAISE NOTICE '  2. AFTER UPDATE trigger fires → binding STILL EXISTS';
    RAISE NOTICE '  3. Trigger reads binding, updates Service.aggregated_address_groups';
    RAISE NOTICE '  4. Service UPDATE trigger creates SGROUP outbox entry';
    RAISE NOTICE '  5. OutboxWorker syncs to SGROUP';
    RAISE NOTICE '  6. OutboxWorker deletes binding AFTER successful sync ✓';
    RAISE NOTICE '';
    RAISE NOTICE 'Result: SGROUP receives correct UPDATE with empty aggregated_address_groups!';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to CASCADE (original problematic behavior)
ALTER TABLE address_group_bindings
DROP CONSTRAINT IF EXISTS address_group_bindings_resource_version_fkey;

ALTER TABLE address_group_bindings
ADD CONSTRAINT address_group_bindings_resource_version_fkey
FOREIGN KEY (resource_version) REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE;

DO $$
BEGIN
    RAISE WARNING '[Migration 077 Rollback] Reverted to CASCADE - deletion timing bug will return!';
END $$;

-- +goose StatementEnd
