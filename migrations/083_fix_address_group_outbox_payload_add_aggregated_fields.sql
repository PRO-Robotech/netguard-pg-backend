-- +goose Up
-- Migration 083: Fix AddressGroup outbox payload - include aggregated_hosts and networks
--
-- ============================================================================
-- CRITICAL BUG FIX: AddressGroup UPDATE payload missing aggregated_hosts and networks!
-- ============================================================================
--
-- Problem:
--   trigger_address_group_upsert_outbox() creates outbox entries with payload:
--   {
--     "namespace": "incloud-sgroups",
--     "name": "my-ag"
--   }
--
--   BUT MISSING: aggregated_hosts and networks!
--
-- Impact:
--   When HostBinding/NetworkBinding/AddressGroupBinding is deleted:
--   1. ✅ Database correctly updates AddressGroup.aggregated_hosts/networks = []
--   2. ✅ Outbox entry created with operation = UPDATE
--   3. ✅ OutboxWorker processes entry successfully (status = SUCCESS)
--   4. ❌ BUT SGROUP receives INCOMPLETE payload!
--   5. ❌ SGROUP doesn't update aggregated_hosts/networks
--   6. ❌ SGROUP keeps OLD data: "hosts": [...], "networks": [...]
--
-- Root Cause:
--   Migration 041 added aggregated_hosts/networks CHECK to trigger condition
--   BUT DID NOT add aggregated_hosts/networks to PAYLOAD!
--
-- Solution:
--   Include aggregated_hosts and networks in payload:
--   jsonb_build_object(
--     'namespace', NEW.namespace,
--     'name', NEW.name,
--     'aggregated_hosts', NEW.aggregated_hosts,  -- ✅ ADD THIS!
--     'networks', NEW.networks                    -- ✅ ADD THIS!
--   )
--
-- Expected Flow (FIXED):
--   1. User deletes HostBinding/NetworkBinding
--   2. trigger_update_aggregated_hosts_on_binding_change() / sync_address_group_networks_on_binding_change() fires
--   3. Updates AddressGroup.aggregated_hosts/networks = []
--   4. trigger_address_group_upsert_outbox() fires
--   5. Creates outbox entry with COMPLETE payload including aggregated_hosts and networks
--   6. OutboxWorker sends COMPLETE data to SGROUP
--   7. SGROUP updates aggregated_hosts/networks correctly ✅
--
-- Related:
--   - Migration 014: aggregate_address_group_hosts() function
--   - Migration 020: sync_address_group_networks_on_binding_change() function
--   - Migration 041: Added aggregated_hosts/networks check to trigger condition
--   - Migration 076: Similar fix for Service.aggregated_address_groups
--   - Migration 079: Added payload update in ON CONFLICT for Service
--
-- Date: 2025-11-01
-- ============================================================================

-- +goose StatementBegin

CREATE OR REPLACE FUNCTION trigger_address_group_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_has_spec_changes BOOLEAN := FALSE;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSIF TG_OP = 'UPDATE' THEN
        -- Check for changes in spec fields including aggregated_hosts and networks (Migration 041)
        IF OLD.default_action IS DISTINCT FROM NEW.default_action OR
           OLD.logs IS DISTINCT FROM NEW.logs OR
           OLD.trace IS DISTINCT FROM NEW.trace OR
           OLD.hosts IS DISTINCT FROM NEW.hosts OR
           OLD.networks IS DISTINCT FROM NEW.networks OR
           OLD.aggregated_hosts IS DISTINCT FROM NEW.aggregated_hosts THEN
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
            'AddressGroup:' || NEW.namespace || '/' || NEW.name
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
        'AddressGroup',
        v_resource_id,
        NEW.namespace,
        NEW.name,
        v_operation_type,
        'SGROUP',
        -- ✅ CRITICAL FIX (Migration 083): Include aggregated_hosts and networks in payload!
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'aggregated_hosts', NEW.aggregated_hosts,
            'networks', NEW.networks
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
        -- ✅ CRITICAL FIX (Migration 083): Update payload to ensure latest data is synced!
        payload = EXCLUDED.payload;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_address_group_upsert_outbox() IS
'Creates sync_outbox entry when AddressGroup is created or updated.
Migration 041: Added aggregated_hosts/networks check.
Migration 083: Added aggregated_hosts and networks to PAYLOAD (critical fix!).
This ensures OutboxWorker sends COMPLETE data to SGROUP.';

DO $$
BEGIN
    RAISE NOTICE '[Migration 083] ✅ Fixed trigger_address_group_upsert_outbox() payload - now includes aggregated_hosts and networks';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to Migration 041 version (with aggregated_hosts/networks check but WITHOUT in payload)
CREATE OR REPLACE FUNCTION trigger_address_group_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_has_spec_changes BOOLEAN := FALSE;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.default_action IS DISTINCT FROM NEW.default_action OR
           OLD.logs IS DISTINCT FROM NEW.logs OR
           OLD.trace IS DISTINCT FROM NEW.trace OR
           OLD.hosts IS DISTINCT FROM NEW.hosts OR
           OLD.networks IS DISTINCT FROM NEW.networks OR
           OLD.aggregated_hosts IS DISTINCT FROM NEW.aggregated_hosts THEN
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
            'AddressGroup:' || NEW.namespace || '/' || NEW.name
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
        'AddressGroup',
        v_resource_id,
        NEW.namespace,
        NEW.name,
        v_operation_type,
        'SGROUP',
        -- INCOMPLETE payload (Migration 041 version)
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
    RAISE NOTICE '[Migration 083 Rollback] Reverted to Migration 041 version - payload WITHOUT aggregated_hosts and networks';
END $$;

-- +goose StatementEnd

