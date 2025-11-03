-- +goose Up
-- Migration 075: Fix AddressGroupBinding CASCADE Pattern (Apply Migration 031 Pattern)
--
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
-- ROOT CAUSE (from previous debugging session):
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
--
-- Migration 027 created AddressGroupBinding BEFORE DELETE trigger with `RETURN NULL` (obsolete pattern).
-- `RETURN NULL` ABORTS the DELETE operation, preventing CASCADE DELETE from proceeding.
--
-- Migration 031 updated NetworkBinding and HostBinding BEFORE DELETE triggers to `RETURN OLD`
-- (allows CASCADE DELETE to proceed) but **missed AddressGroupBinding**.
--
-- This inconsistency causes race conditions when:
-- 1. kubectl delete → apiserver calls MarkForDeletionWithStatus()
-- 2. MarkForDeletionWithStatus() sets deletion_timestamp on k8s_metadata
-- 3. AFTER UPDATE trigger (Migration 073/074) tries to read binding details
-- 4. But binding state is unpredictable due to RETURN NULL blocking CASCADE DELETE
--
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
-- SOLUTION:
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
--
-- Update `trigger_address_group_binding_before_delete()` to match Migration 031 pattern:
-- Change `RETURN NULL;` → `RETURN OLD;`
--
-- This allows CASCADE DELETE to proceed immediately after soft delete is marked,
-- providing consistent behavior with HostBinding and NetworkBinding.
--
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
-- EXPECTED FLOW AFTER FIX:
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
--
-- 1. kubectl delete AddressGroupBinding
-- 2. BEFORE DELETE trigger fires:
--    - Sets deletion_timestamp on k8s_metadata
--    - Creates DELETE outbox entry (target=INTERNAL)
--    - Returns OLD ← CASCADE DELETE PROCEEDS
-- 3. AFTER UPDATE (deletion_timestamp) trigger fires (Migration 073/074):
--    - Reads binding details (binding still exists at this point)
--    - Updates Service.aggregated_address_groups (excludes deleted binding)
--    - Service UPDATE trigger creates SGROUP outbox entry
-- 4. CASCADE DELETE completes (binding physically deleted)
-- 5. OutboxWorker syncs changes to SGROUP
--
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
-- RELATED MIGRATIONS:
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
--
-- - Migration 027: Created AddressGroupBinding BEFORE DELETE trigger (RETURN NULL - obsolete)
-- - Migration 031: Fixed NetworkBinding/HostBinding to RETURN OLD (missed AddressGroupBinding!)
-- - Migration 044: HostBinding coordinated deletion reference pattern
-- - Migration 072: Updated aggregate function with deletion_timestamp filter ✅
-- - Migration 073: Added Service UPDATE to deletion trigger ✅
-- - Migration 074: Removed manual DELETE from deletion trigger ✅
-- - Migration 075: THIS MIGRATION - Fix BEFORE DELETE to RETURN OLD ✅
--
-- Date: 2025-10-31
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════

-- +goose StatementBegin

-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
-- Update AddressGroupBinding BEFORE DELETE trigger (apply Migration 031 pattern)
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════

CREATE OR REPLACE FUNCTION trigger_address_group_binding_before_delete()
RETURNS TRIGGER AS $$
DECLARE
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
BEGIN
    -- Get UID and check if already marked for deletion
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    -- If already marked, allow CASCADE DELETE to proceed immediately
    IF v_already_marked_for_deletion THEN
        RETURN OLD;  -- ✅ Consistent with Migration 031 pattern
    END IF;

    -- Set deletion_timestamp (soft delete)
    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting internal processing before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    -- Create DELETE outbox entry
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        resource_namespace,
        resource_name,
        operation,
        target_system,
        payload,
        status,
        attempts,
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'AddressGroupBinding',
        v_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'service', jsonb_build_object(
                'namespace', OLD.service_namespace,
                'name', OLD.service_name
            ),
            'address_group', jsonb_build_object(
                'namespace', OLD.address_group_namespace,
                'name', OLD.address_group_name
            )
        ),
        'PENDING'::outbox_status,
        0,  -- attempts
        5,  -- max_retries
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    -- ✅ CRITICAL FIX (Migration 075): RETURN OLD instead of NULL
    -- This allows CASCADE DELETE to proceed immediately after soft delete is marked.
    -- Matches Migration 031 pattern for NetworkBinding and HostBinding.
    RETURN OLD;  -- ← Changed from NULL
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_address_group_binding_before_delete() IS
'AddressGroupBinding BEFORE DELETE trigger.
Sets deletion_timestamp (soft delete), creates DELETE outbox entry, then returns OLD.
Migration 075: Fixed to RETURN OLD (was NULL) - applies Migration 031 pattern consistently.
Allows CASCADE DELETE to proceed, preventing race conditions in coordinated deletion.';

-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
-- Verification and summary
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════

DO $$
BEGIN
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '[Migration 075] AddressGroupBinding CASCADE Pattern Fix COMPLETE';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '';
    RAISE NOTICE 'FIXED: trigger_address_group_binding_before_delete() now returns OLD (was NULL)';
    RAISE NOTICE '';
    RAISE NOTICE 'This matches Migration 031 pattern for NetworkBinding and HostBinding.';
    RAISE NOTICE 'BEFORE DELETE trigger now allows CASCADE DELETE to proceed immediately.';
    RAISE NOTICE '';
    RAISE NOTICE 'Expected behavior:';
    RAISE NOTICE '  1. kubectl delete → BEFORE DELETE sets deletion_timestamp';
    RAISE NOTICE '  2. BEFORE DELETE returns OLD → CASCADE DELETE proceeds';
    RAISE NOTICE '  3. AFTER UPDATE (deletion_timestamp) trigger reads binding (still exists)';
    RAISE NOTICE '  4. AFTER UPDATE updates Service.aggregated_address_groups';
    RAISE NOTICE '  5. Service UPDATE trigger creates SGROUP outbox entry';
    RAISE NOTICE '  6. CASCADE DELETE completes (binding physically deleted)';
    RAISE NOTICE '  7. OutboxWorker syncs changes to SGROUP ✓';
    RAISE NOTICE '';
    RAISE NOTICE 'Race conditions eliminated - binding deletion timing now deterministic.';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to Migration 027 version (RETURN NULL - breaks CASCADE)
CREATE OR REPLACE FUNCTION trigger_address_group_binding_before_delete()
RETURNS TRIGGER AS $$
DECLARE
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
BEGIN
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    IF v_already_marked_for_deletion THEN
        RETURN OLD;
    END IF;

    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting internal processing before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        resource_namespace,
        resource_name,
        operation,
        target_system,
        payload,
        status,
        attempts,
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'AddressGroupBinding',
        v_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'service', jsonb_build_object(
                'namespace', OLD.service_namespace,
                'name', OLD.service_name
            ),
            'address_group', jsonb_build_object(
                'namespace', OLD.address_group_namespace,
                'name', OLD.address_group_name
            )
        ),
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    -- ❌ Reverted: RETURN NULL (blocks CASCADE DELETE)
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    RAISE WARNING '[Migration 075 Rollback] Reverted to Migration 027 - RETURN NULL blocks CASCADE DELETE';
END $$;

-- +goose StatementEnd
