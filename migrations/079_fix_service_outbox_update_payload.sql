-- +goose Up
-- Migration 079: Fix Service outbox trigger - UPDATE payload on conflict
--
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
-- CRITICAL BUG FIX: Service UPDATE outbox entry uses OLD payload after aggregated_address_groups change
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
--
-- Root Cause (discovered 2025-10-31):
--   trigger_service_upsert_outbox() has ON CONFLICT DO UPDATE but does NOT update payload:
--
--   ON CONFLICT (resource_type, resource_id, operation, target_system)
--   DO UPDATE SET
--       status = 'PENDING',
--       attempts = 0,
--       updated_at = NOW();
--       -- ❌ BUG: payload NOT updated!
--
--   When Service.aggregated_address_groups changes:
--   1. Trigger fires and detects change ✅
--   2. Tries to INSERT new outbox entry with correct payload ✅
--   3. ON CONFLICT updates status = 'PENDING' ✅
--   4. BUT payload remains OLD (with deleted binding!) ❌
--   5. OutboxWorker sends OLD payload to SGROUP ❌
--
-- Impact:
--   1. AddressGroupBinding deleted → Service.aggregated_address_groups updated to [] in DB ✅
--   2. Service UPDATE trigger fires ✅
--   3. ON CONFLICT updates existing entry but keeps OLD payload ❌
--   4. OutboxWorker syncs to SGROUP with OLD payload (shows deleted binding!) ❌
--   5. SGROUP shows incorrect data (binding still exists in SGROUP) ❌
--
-- Solution (Migration 079):
--   Add `payload = EXCLUDED.payload` to ON CONFLICT DO UPDATE clause.
--   This ensures payload is ALWAYS updated to reflect current Service state.
--
-- Expected Flow After Fix:
--   1. AddressGroupBinding deleted
--   2. Service.aggregated_address_groups updated to [] in DB
--   3. Service UPDATE trigger fires
--   4. ON CONFLICT updates: status='PENDING', attempts=0, payload=NEW payload ✅
--   5. OutboxWorker syncs NEW payload to SGROUP ✅
--   6. SGROUP shows correct data (aggregated_address_groups=[]) ✅
--
-- Date: 2025-10-31
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════

-- +goose StatementBegin

CREATE OR REPLACE FUNCTION trigger_service_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_has_spec_changes BOOLEAN := FALSE;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSIF TG_OP = 'UPDATE' THEN
        -- Check for changes in spec fields including aggregated_address_groups (Migration 070)
        IF OLD.description IS DISTINCT FROM NEW.description OR
           OLD.ingress_ports IS DISTINCT FROM NEW.ingress_ports OR
           OLD.address_groups IS DISTINCT FROM NEW.address_groups OR
           OLD.aggregated_address_groups IS DISTINCT FROM NEW.aggregated_address_groups THEN
            v_has_spec_changes := TRUE;
            v_operation_type := 'UPDATE'::sync_operation;
        ELSE
            RETURN NEW;
        END IF;
    ELSE
        RETURN NEW;
    END IF;

    -- Try to get real Kubernetes UID from k8s_metadata first
    SELECT km.uid INTO v_resource_id
    FROM k8s_metadata km
    WHERE km.resource_version = NEW.resource_version;

    IF v_resource_id IS NOT NULL THEN
        -- Use real K8s UID from metadata (preferred)
    ELSE
        -- Fallback to UUID v5 if k8s_metadata not found (shouldn't happen in normal operation)
        v_resource_id := uuid_generate_v5(
            uuid_ns_dns(),
            'Service:' || NEW.namespace || '/' || NEW.name
        );
    END IF;

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
        max_retries
    ) VALUES (
        'Service',
        v_resource_id,
        NEW.namespace,
        NEW.name,
        v_operation_type,
        'SGROUP',
        -- ✅ CRITICAL FIX (Migration 076): Include aggregated_address_groups in payload!
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'aggregated_address_groups', NEW.aggregated_address_groups
        ),
        'PENDING',
        0,
        5
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING',
        attempts = 0,
        updated_at = NOW(),
        payload = EXCLUDED.payload;  -- ✨ CRITICAL FIX (Migration 079): Update payload!

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_service_upsert_outbox() IS
'Service AFTER INSERT/UPDATE trigger.
Creates/updates sync_outbox entry when Service changes.
Migration 076: Added aggregated_address_groups to payload.
Migration 079: Fixed ON CONFLICT to update payload (was only updating status/attempts).';

-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
-- Verification and summary
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════

DO $$
BEGIN
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '[Migration 079] Service outbox payload update fix COMPLETE';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '';
    RAISE NOTICE 'FIXED: trigger_service_upsert_outbox() ON CONFLICT now updates payload';
    RAISE NOTICE '';
    RAISE NOTICE 'Before Migration 079:';
    RAISE NOTICE '  - Service.aggregated_address_groups updated ✓';
    RAISE NOTICE '  - Trigger fires and detects change ✓';
    RAISE NOTICE '  - ON CONFLICT updates status=PENDING ✓';
    RAISE NOTICE '  - BUT payload stays OLD ✗';
    RAISE NOTICE '  - OutboxWorker syncs OLD data to SGROUP ✗';
    RAISE NOTICE '';
    RAISE NOTICE 'After Migration 079:';
    RAISE NOTICE '  - Service.aggregated_address_groups updated ✓';
    RAISE NOTICE '  - Trigger fires and detects change ✓';
    RAISE NOTICE '  - ON CONFLICT updates status=PENDING AND payload=NEW ✓';
    RAISE NOTICE '  - OutboxWorker syncs NEW data to SGROUP ✓';
    RAISE NOTICE '';
    RAISE NOTICE 'Result: SGROUP will receive correct Service data with updated aggregated_address_groups!';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to Migration 076 version (does NOT update payload on conflict)
CREATE OR REPLACE FUNCTION trigger_service_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_has_spec_changes BOOLEAN := FALSE;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.description IS DISTINCT FROM NEW.description OR
           OLD.ingress_ports IS DISTINCT FROM NEW.ingress_ports OR
           OLD.address_groups IS DISTINCT FROM NEW.address_groups OR
           OLD.aggregated_address_groups IS DISTINCT FROM NEW.aggregated_address_groups THEN
            v_has_spec_changes := TRUE;
            v_operation_type := 'UPDATE'::sync_operation;
        ELSE
            RETURN NEW;
        END IF;
    ELSE
        RETURN NEW;
    END IF;

    SELECT km.uid INTO v_resource_id
    FROM k8s_metadata km
    WHERE km.resource_version = NEW.resource_version;

    IF v_resource_id IS NOT NULL THEN
    ELSE
        v_resource_id := uuid_generate_v5(
            uuid_ns_dns(),
            'Service:' || NEW.namespace || '/' || NEW.name
        );
    END IF;

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
        max_retries
    ) VALUES (
        'Service',
        v_resource_id,
        NEW.namespace,
        NEW.name,
        v_operation_type,
        'SGROUP',
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'aggregated_address_groups', NEW.aggregated_address_groups
        ),
        'PENDING',
        0,
        5
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING',
        attempts = 0,
        updated_at = NOW();
        -- ❌ Reverted: payload NOT updated (Migration 076 version)

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    RAISE WARNING '[Migration 079 Rollback] Reverted to Migration 076 - payload update bug will return!';
END $$;

-- +goose StatementEnd
