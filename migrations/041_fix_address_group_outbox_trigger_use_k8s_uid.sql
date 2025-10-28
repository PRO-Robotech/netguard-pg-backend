-- +goose Up
-- +goose StatementBegin

-- ============================================================================
-- Migration 041: Fix AddressGroup outbox trigger to use k8s_metadata UID
-- ============================================================================
--
-- Problem:
--   Triple outbox entries created for single AddressGroup:
--   1. Trigger creates CREATE (UUID v5)
--   2. Trigger creates UPDATE (UUID v5)
--   3. Go code creates CREATE (Kubernetes UID) ← DUPLICATE!
--
-- Root Cause:
--   - Trigger uses uuid_generate_v5() (deterministic)
--   - Go code uses ag.Meta.UID (Kubernetes generated)
--   - Different resource_id values → no ON CONFLICT → 2 CREATE entries!
--
-- Solution:
--   - Modify trigger to read UID from k8s_metadata (via resource_version)
--   - Use real Kubernetes UID (not UUID v5)
--   - Remove Go outbox creation (done in Go code separately)
--   - Fallback to UUID v5 if k8s_metadata not found (safety)
--
-- Result:
--   - Only 2 outbox entries: CREATE + UPDATE
--   - resource_id matches k8s_metadata.uid
--   - No duplicates
--
-- Date: 2025-10-28
-- Related: migrations/040 (debug logging)
-- ============================================================================

CREATE OR REPLACE FUNCTION trigger_address_group_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_has_spec_changes BOOLEAN := FALSE;
BEGIN
    -- Determine operation type
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;

        RAISE WARNING '[TRIGGER] trigger_address_group_upsert_outbox: INSERT detected';
        RAISE WARNING '[TRIGGER]   namespace: %, name: %', NEW.namespace, NEW.name;
        RAISE WARNING '[TRIGGER]   resource_version: %', NEW.resource_version;

    ELSIF TG_OP = 'UPDATE' THEN
        -- Check if spec fields changed (trigger sync only for spec changes)
        IF OLD.default_action IS DISTINCT FROM NEW.default_action OR
           OLD.logs IS DISTINCT FROM NEW.logs OR
           OLD.trace IS DISTINCT FROM NEW.trace OR
           OLD.hosts IS DISTINCT FROM NEW.hosts OR
           OLD.networks IS DISTINCT FROM NEW.networks OR
           OLD.aggregated_hosts IS DISTINCT FROM NEW.aggregated_hosts THEN
            v_has_spec_changes := TRUE;
            v_operation_type := 'UPDATE'::sync_operation;

            RAISE WARNING '[TRIGGER] trigger_address_group_upsert_outbox: UPDATE with spec changes';
            RAISE WARNING '[TRIGGER]   namespace: %, name: %', NEW.namespace, NEW.name;
        ELSE
            -- No spec changes, skip outbox creation
            RAISE WARNING '[TRIGGER] trigger_address_group_upsert_outbox: UPDATE without spec changes, SKIP';
            RETURN NEW;
        END IF;
    ELSE
        -- DELETE handled by separate trigger
        RETURN NEW;
    END IF;

    -- ========================================================================
    -- Get UID from k8s_metadata (using resource_version)
    -- ========================================================================
    -- This is SAFE because:
    -- 1. Go code INSERTs k8s_metadata BEFORE address_groups
    -- 2. Trigger fires AFTER address_groups INSERT
    -- 3. k8s_metadata is already present when trigger runs
    -- ========================================================================

    SELECT km.uid INTO v_resource_id
    FROM k8s_metadata km
    WHERE km.resource_version = NEW.resource_version;

    IF v_resource_id IS NOT NULL THEN
        RAISE WARNING '[TRIGGER]   UID from k8s_metadata: %', v_resource_id;
    ELSE
        -- Fallback to UUID v5 (deterministic) if k8s_metadata not found
        -- This should NEVER happen in normal operation
        RAISE WARNING '[TRIGGER]   k8s_metadata UID NOT FOUND! Using UUID v5 fallback';

        v_resource_id := uuid_generate_v5(
            uuid_ns_dns(),
            'AddressGroup:' || NEW.namespace || '/' || NEW.name
        );

        RAISE WARNING '[TRIGGER]   UUID v5 fallback: %', v_resource_id;
    END IF;

    -- ========================================================================
    -- Create/update outbox entry
    -- ========================================================================
    -- ON CONFLICT ensures idempotency:
    -- - If entry exists with same (resource_type, resource_id, operation, target_system)
    -- - Reset status to PENDING and attempts to 0
    -- ========================================================================

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

    RAISE WARNING '[TRIGGER] trigger_address_group_upsert_outbox: Outbox entry created/updated';
    RAISE WARNING '[TRIGGER]   resource_id: %, operation: %', v_resource_id, v_operation_type;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger is already created by previous migration, just replace function
-- Migration 041: AddressGroup outbox trigger now uses k8s_metadata UID

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Restore old version with UUID v5 (from migration 040)
CREATE OR REPLACE FUNCTION trigger_address_group_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_conflict_occurred INTEGER;
BEGIN
    RAISE WARNING '[TRIGGER-DEBUG] ==============================================';
    RAISE WARNING '[TRIGGER-DEBUG] trigger_address_group_upsert_outbox: ENTRY';
    RAISE WARNING '[TRIGGER-DEBUG] TG_OP: %', TG_OP;
    RAISE WARNING '[TRIGGER-DEBUG] TG_TABLE_NAME: %', TG_TABLE_NAME;
    RAISE WARNING '[TRIGGER-DEBUG] TG_WHEN: %', TG_WHEN;
    RAISE WARNING '[TRIGGER-DEBUG] NEW.namespace: %', NEW.namespace;
    RAISE WARNING '[TRIGGER-DEBUG] NEW.name: %', NEW.name;
    RAISE WARNING '[TRIGGER-DEBUG] Timestamp: %', clock_timestamp();

    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
        RAISE WARNING '[TRIGGER-DEBUG] Operation determined: CREATE (from INSERT)';

        RAISE WARNING '[TRIGGER-DEBUG] NEW.default_action: %, NEW.logs: %, NEW.trace: %',
            NEW.default_action, NEW.logs, NEW.trace;
        RAISE WARNING '[TRIGGER-DEBUG] NEW.hosts (first 200 chars): %',
            substring(NEW.hosts::text from 1 for 200);
        RAISE WARNING '[TRIGGER-DEBUG] NEW.networks (first 200 chars): %',
            substring(NEW.networks::text from 1 for 200);
        RAISE WARNING '[TRIGGER-DEBUG] NEW.aggregated_hosts (first 200 chars): %',
            substring(NEW.aggregated_hosts::text from 1 for 200);

    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
        RAISE WARNING '[TRIGGER-DEBUG] Operation determined: UPDATE (from UPDATE)';

        RAISE WARNING '[TRIGGER-DEBUG] Field changes:';
        RAISE WARNING '[TRIGGER-DEBUG]   default_action changed: %',
            (OLD.default_action IS DISTINCT FROM NEW.default_action);
        RAISE WARNING '[TRIGGER-DEBUG]   logs changed: %',
            (OLD.logs IS DISTINCT FROM NEW.logs);
        RAISE WARNING '[TRIGGER-DEBUG]   trace changed: %',
            (OLD.trace IS DISTINCT FROM NEW.trace);
        RAISE WARNING '[TRIGGER-DEBUG]   hosts changed: %',
            (OLD.hosts IS DISTINCT FROM NEW.hosts);
        RAISE WARNING '[TRIGGER-DEBUG]   networks changed: %',
            (OLD.networks IS DISTINCT FROM NEW.networks);
        RAISE WARNING '[TRIGGER-DEBUG]   aggregated_hosts changed: %',
            (OLD.aggregated_hosts IS DISTINCT FROM NEW.aggregated_hosts);
    END IF;

    -- Generate UUID v5 (deterministic)
    v_resource_id := uuid_generate_v5(
        uuid_ns_dns(),
        'AddressGroup:' || NEW.namespace || '/' || NEW.name
    );

    RAISE WARNING '[TRIGGER-DEBUG] Generated resource_id: %', v_resource_id;

    RAISE WARNING '[TRIGGER-DEBUG] Preparing INSERT INTO sync_outbox';
    RAISE WARNING '[TRIGGER-DEBUG]   resource_type: %', 'AddressGroup';
    RAISE WARNING '[TRIGGER-DEBUG]   resource_id: %', v_resource_id;
    RAISE WARNING '[TRIGGER-DEBUG]   operation: %', v_operation_type;
    RAISE WARNING '[TRIGGER-DEBUG]   target_system: %', 'SGROUP';
    RAISE WARNING '[TRIGGER-DEBUG]   ON CONFLICT key: (resource_type=%s, resource_id=%s, operation=%s, target_system=%s)',
        'AddressGroup', v_resource_id || 's', v_operation_type || 's', 'SGROUP' || 's';

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

    GET DIAGNOSTICS v_conflict_occurred = ROW_COUNT;

    RAISE WARNING '[TRIGGER-DEBUG] INSERT INTO sync_outbox completed';
    RAISE WARNING '[TRIGGER-DEBUG] Note: ROW_COUNT=%, cannot distinguish INSERT vs ON CONFLICT', v_conflict_occurred;
    RAISE WARNING '[TRIGGER-DEBUG] If entry already existed, it was UPDATED (status reset to PENDING)';
    RAISE WARNING '[TRIGGER-DEBUG] If entry was new, it was CREATED';
    RAISE WARNING '[TRIGGER-DEBUG] trigger_address_group_upsert_outbox: EXIT';
    RAISE WARNING '[TRIGGER-DEBUG] ==============================================';

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Migration 041 DOWN: Restored old AddressGroup outbox trigger with UUID v5

-- +goose StatementEnd
