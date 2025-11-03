-- +goose Up
-- Migration 076: Fix Service outbox payload - include aggregated_address_groups
--
-- ============================================================================
-- CRITICAL BUG FIX: Service UPDATE payload missing aggregated_address_groups!
-- ============================================================================
--
-- Problem:
--   trigger_service_upsert_outbox() creates outbox entries with payload:
--   {
--     "namespace": "incloud-sgroups",
--     "name": "my-service"
--   }
--
--   BUT MISSING: aggregated_address_groups!
--
-- Impact:
--   When AddressGroupBinding is deleted:
--   1. ✅ Database correctly updates Service.aggregated_address_groups = []
--   2. ✅ Outbox entry created with operation = UPDATE
--   3. ✅ OutboxWorker processes entry successfully (status = SUCCESS)
--   4. ❌ BUT SGROUP receives INCOMPLETE payload!
--   5. ❌ SGROUP doesn't update aggregated_address_groups
--   6. ❌ SGROUP keeps OLD data: "sgNames": ["incloud-sgroups/old-ag"]
--
-- Root Cause:
--   Migration 070 added aggregated_address_groups CHECK to trigger condition
--   BUT DID NOT add aggregated_address_groups to PAYLOAD!
--
-- Solution:
--   Include aggregated_address_groups in payload:
--   jsonb_build_object(
--     'namespace', NEW.namespace,
--     'name', NEW.name,
--     'aggregated_address_groups', NEW.aggregated_address_groups  -- ✅ ADD THIS!
--   )
--
-- Expected Flow (FIXED):
--   1. User deletes AddressGroupBinding
--   2. trigger_update_aggregated_ags_on_binding_change() fires
--   3. Updates Service.aggregated_address_groups = []
--   4. trigger_service_upsert_outbox() fires
--   5. Creates outbox entry with COMPLETE payload including aggregated_address_groups
--   6. OutboxWorker sends COMPLETE data to SGROUP
--   7. SGROUP updates aggregated_address_groups correctly ✅
--
-- Related:
--   - Migration 018: aggregate_service_address_groups() function
--   - Migration 053: trigger_update_aggregated_ags_on_binding_change()
--   - Migration 070: Added aggregated_address_groups check to trigger condition
--   - Migration 075: Fixed AddressGroupBinding CASCADE DELETE race condition
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
        updated_at = NOW();

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_service_upsert_outbox() IS
'Creates sync_outbox entry when Service is created or updated.
Migration 070: Added aggregated_address_groups check.
Migration 076: Added aggregated_address_groups to PAYLOAD (critical fix!).
This ensures OutboxWorker sends COMPLETE data to SGROUP.';

DO $$
BEGIN
    RAISE NOTICE '[Migration 076] ✅ Fixed trigger_service_upsert_outbox() payload - now includes aggregated_address_groups';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to Migration 070 version (with aggregated_address_groups check but WITHOUT in payload)
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
        -- INCOMPLETE payload (Migration 070 version)
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
    RAISE NOTICE '[Migration 076 Rollback] Reverted to Migration 070 version - payload WITHOUT aggregated_address_groups';
END $$;

-- +goose StatementEnd
