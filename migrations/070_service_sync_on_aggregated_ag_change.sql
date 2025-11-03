-- +goose Up
-- Migration 070: Fix Service sync when aggregated_address_groups changes via AddressGroupBinding
--
-- ============================================================================
-- CRITICAL BUG FIX: Service не синхронизируется с SGROUP через AddressGroupBinding
-- ============================================================================
--
-- Problem:
--   trigger_service_upsert_outbox() проверяет изменения ТОЛЬКО в:
--   - description
--   - ingress_ports
--   - address_groups (spec.addressGroups)
--
--   НО НЕ проверяет aggregated_address_groups!
--
-- Flow (BROKEN):
--   1. User creates AddressGroupBinding
--   2. trigger_update_aggregated_ags_on_binding_change() fires
--   3. Updates Service.aggregated_address_groups in DB
--   4. trigger_service_upsert_outbox() fires on UPDATE
--   5. Checks spec fields (description, ingress_ports, address_groups)
--   6. Finds NO CHANGES in spec → returns NEW without creating outbox entry
--   7. aggregated_address_groups change NOT synced to SGROUP! ❌
--
-- Solution:
--   Add aggregated_address_groups check to trigger condition:
--     OLD.aggregated_address_groups IS DISTINCT FROM NEW.aggregated_address_groups
--
-- Expected Flow (FIXED):
--   1. User creates AddressGroupBinding
--   2. trigger_update_aggregated_ags_on_binding_change() fires
--   3. Updates Service.aggregated_address_groups in DB
--   4. trigger_service_upsert_outbox() fires on UPDATE
--   5. Checks aggregated_address_groups → FINDS CHANGE!
--   6. Creates UPDATE outbox entry
--   7. OutboxWorker syncs change to SGROUP ✅
--
-- Related:
--   - Migration 018: aggregate_service_address_groups() function
--   - Migration 053: trigger_update_aggregated_ags_on_binding_change()
--   - Migration 069: CASCADE deletion for AddressGroupBinding
--
-- Date: 2025-10-31
-- ============================================================================

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
        -- ✨ CRITICAL CHANGE: Added aggregated_address_groups check
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
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name
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

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_service_upsert_outbox() IS
'Creates sync_outbox entry when Service is created or updated.
NOW CHECKS aggregated_address_groups changes (Migration 070 fix).
This ensures Service syncs to SGROUP when AddressGroupBinding is created/deleted.';

DO $$
BEGIN
    RAISE NOTICE '[Migration 070] Fixed trigger_service_upsert_outbox() to check aggregated_address_groups';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to old version WITHOUT aggregated_address_groups check
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
           OLD.address_groups IS DISTINCT FROM NEW.address_groups THEN
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
        -- Use real K8s UID from metadata
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
            'name', NEW.name
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

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    RAISE NOTICE '[Migration 070 Rollback] Reverted trigger_service_upsert_outbox() - aggregated_address_groups NOT checked';
END $$;

-- +goose StatementEnd
