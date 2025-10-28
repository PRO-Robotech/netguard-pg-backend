-- +goose Up
-- +goose StatementBegin

-- =============================================================================
-- MIGRATION 040: Add maximum debug logging to AddressGroup outbox triggers
-- =============================================================================
-- Purpose: Investigate why 3 outbox entries are created (CREATE → UPDATE → CREATE)
--          and why conditions don't update without DEBUG logs in Go code
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 1. Replace trigger_address_group_upsert_outbox with DEBUG logging
-- -----------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION trigger_address_group_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_conflict_occurred INTEGER;
BEGIN
    -- ENTRY LOG
    RAISE WARNING '[TRIGGER-DEBUG] ==============================================';
    RAISE WARNING '[TRIGGER-DEBUG] trigger_address_group_upsert_outbox: ENTRY';
    RAISE WARNING '[TRIGGER-DEBUG] TG_OP: %', TG_OP;
    RAISE WARNING '[TRIGGER-DEBUG] TG_TABLE_NAME: %', TG_TABLE_NAME;
    RAISE WARNING '[TRIGGER-DEBUG] TG_WHEN: %', TG_WHEN;
    RAISE WARNING '[TRIGGER-DEBUG] NEW.namespace: %', NEW.namespace;
    RAISE WARNING '[TRIGGER-DEBUG] NEW.name: %', NEW.name;
    RAISE WARNING '[TRIGGER-DEBUG] Timestamp: %', clock_timestamp();

    -- Determine operation type
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
        RAISE WARNING '[TRIGGER-DEBUG] Operation determined: CREATE (from INSERT)';

        -- Log NEW values for INSERT
        RAISE WARNING '[TRIGGER-DEBUG] NEW.default_action: %, NEW.logs: %, NEW.trace: %',
            NEW.default_action, NEW.logs, NEW.trace;
        RAISE WARNING '[TRIGGER-DEBUG] NEW.hosts (first 200 chars): %',
            substring(NEW.hosts::text from 1 for 200);
        RAISE WARNING '[TRIGGER-DEBUG] NEW.networks (first 200 chars): %',
            substring(NEW.networks::text from 1 for 200);
        RAISE WARNING '[TRIGGER-DEBUG] NEW.aggregated_hosts (first 200 chars): %',
            substring(COALESCE(NEW.aggregated_hosts, '[]'::jsonb)::text from 1 for 200);
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
        RAISE WARNING '[TRIGGER-DEBUG] Operation determined: UPDATE (from UPDATE)';

        -- Log changes for UPDATE
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

        -- Log OLD vs NEW for changed fields
        IF OLD.aggregated_hosts IS DISTINCT FROM NEW.aggregated_hosts THEN
            RAISE WARNING '[TRIGGER-DEBUG]   OLD.aggregated_hosts (first 200): %',
                substring(COALESCE(OLD.aggregated_hosts, '[]'::jsonb)::text from 1 for 200);
            RAISE WARNING '[TRIGGER-DEBUG]   NEW.aggregated_hosts (first 200): %',
                substring(COALESCE(NEW.aggregated_hosts, '[]'::jsonb)::text from 1 for 200);
        END IF;
    END IF;

    -- Generate deterministic resource_id
    v_resource_id := uuid_generate_v5(
        uuid_ns_dns(),
        'AddressGroup:' || NEW.namespace || '/' || NEW.name
    );

    RAISE WARNING '[TRIGGER-DEBUG] Generated resource_id: %', v_resource_id;
    RAISE WARNING '[TRIGGER-DEBUG] Preparing INSERT INTO sync_outbox';
    RAISE WARNING '[TRIGGER-DEBUG]   resource_type: AddressGroup';
    RAISE WARNING '[TRIGGER-DEBUG]   resource_id: %', v_resource_id;
    RAISE WARNING '[TRIGGER-DEBUG]   operation: %', v_operation_type;
    RAISE WARNING '[TRIGGER-DEBUG]   target_system: SGROUP';
    RAISE WARNING '[TRIGGER-DEBUG]   ON CONFLICT key: (resource_type=%s, resource_id=%s, operation=%s, target_system=%s)',
        'AddressGroup', v_resource_id, v_operation_type, 'SGROUP';

    -- Insert into sync_outbox with conflict handling
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        operation,
        target_system,
        payload,
        resource_namespace,
        resource_name,
        status,
        attempts,
        max_retries,
        next_retry_at,
        created_at,
        updated_at
    )
    VALUES (
        'AddressGroup',
        v_resource_id,
        v_operation_type,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name
        ),
        NEW.namespace,
        NEW.name,
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = NOW(),
        updated_at = NOW();

    -- Check if it was INSERT or UPDATE (conflict)
    GET DIAGNOSTICS v_conflict_occurred = ROW_COUNT;

    IF v_conflict_occurred > 0 THEN
        RAISE WARNING '[TRIGGER-DEBUG] INSERT INTO sync_outbox completed';
        RAISE WARNING '[TRIGGER-DEBUG] Note: ROW_COUNT=%, cannot distinguish INSERT vs ON CONFLICT',
            v_conflict_occurred;
        RAISE WARNING '[TRIGGER-DEBUG] If entry already existed, it was UPDATED (status reset to PENDING)';
        RAISE WARNING '[TRIGGER-DEBUG] If entry was new, it was CREATED';
    ELSE
        RAISE WARNING '[TRIGGER-DEBUG] WARNING: INSERT returned ROW_COUNT=0 (unexpected!)';
    END IF;

    RAISE WARNING '[TRIGGER-DEBUG] trigger_address_group_upsert_outbox: EXIT';
    RAISE WARNING '[TRIGGER-DEBUG] ==============================================';

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- -----------------------------------------------------------------------------
-- 2. Replace trigger_update_aggregated_hosts_on_spec_change with DEBUG logging
-- -----------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION trigger_update_aggregated_hosts_on_spec_change()
RETURNS TRIGGER AS $$
DECLARE
    v_rows_affected INTEGER;
BEGIN
    RAISE WARNING '[TRIGGER-DEBUG] +++++++++++++++++++++++++++++++++++++++++++++';
    RAISE WARNING '[TRIGGER-DEBUG] trigger_update_aggregated_hosts_on_spec_change: ENTRY';
    RAISE WARNING '[TRIGGER-DEBUG] TG_OP: %', TG_OP;
    RAISE WARNING '[TRIGGER-DEBUG] NEW.namespace: %', NEW.namespace;
    RAISE WARNING '[TRIGGER-DEBUG] NEW.name: %', NEW.name;
    RAISE WARNING '[TRIGGER-DEBUG] Timestamp: %', clock_timestamp();
    RAISE WARNING '[TRIGGER-DEBUG] About to UPDATE address_groups SET aggregated_hosts = ...';
    RAISE WARNING '[TRIGGER-DEBUG] WHERE namespace = % AND name = %', NEW.namespace, NEW.name;

    -- Update aggregated_hosts with separate UPDATE statement after the main operation
    UPDATE address_groups
    SET aggregated_hosts = aggregate_address_group_hosts(NEW.namespace, NEW.name)
    WHERE namespace = NEW.namespace AND name = NEW.name;

    GET DIAGNOSTICS v_rows_affected = ROW_COUNT;

    RAISE WARNING '[TRIGGER-DEBUG] UPDATE address_groups completed';
    RAISE WARNING '[TRIGGER-DEBUG] Rows affected: %', v_rows_affected;

    IF v_rows_affected = 0 THEN
        RAISE WARNING '[TRIGGER-DEBUG] WARNING: No rows updated (unexpected!)';
    ELSIF v_rows_affected > 1 THEN
        RAISE WARNING '[TRIGGER-DEBUG] WARNING: Multiple rows updated (namespace+name should be unique!)';
    ELSE
        RAISE WARNING '[TRIGGER-DEBUG] Successfully updated 1 row';
    END IF;

    RAISE WARNING '[TRIGGER-DEBUG] This UPDATE will trigger trg_address_group_upsert_outbox (AFTER UPDATE)';
    RAISE WARNING '[TRIGGER-DEBUG] trigger_update_aggregated_hosts_on_spec_change: EXIT';
    RAISE WARNING '[TRIGGER-DEBUG] +++++++++++++++++++++++++++++++++++++++++++++';

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_address_group_upsert_outbox() IS
'DEBUG VERSION (Migration 040): Creates sync_outbox entries for AddressGroup changes with maximum logging';

COMMENT ON FUNCTION trigger_update_aggregated_hosts_on_spec_change() IS
'DEBUG VERSION (Migration 040): Updates aggregated_hosts after AddressGroup spec changes with maximum logging';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Restore original functions without debug logging (from migration 026 + 014)

CREATE OR REPLACE FUNCTION trigger_address_group_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
    END IF;

    v_resource_id := uuid_generate_v5(
        uuid_ns_dns(),
        'AddressGroup:' || NEW.namespace || '/' || NEW.name
    );

    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        operation,
        target_system,
        payload,
        resource_namespace,
        resource_name,
        status,
        attempts,
        max_retries,
        next_retry_at,
        created_at,
        updated_at
    )
    VALUES (
        'AddressGroup',
        v_resource_id,
        v_operation_type,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name
        ),
        NEW.namespace,
        NEW.name,
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = NOW(),
        updated_at = NOW();

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION trigger_update_aggregated_hosts_on_spec_change()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE address_groups
    SET aggregated_hosts = aggregate_address_group_hosts(NEW.namespace, NEW.name)
    WHERE namespace = NEW.namespace AND name = NEW.name;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_address_group_upsert_outbox() IS
'Creates sync_outbox entries for AddressGroup INSERT/UPDATE operations';

COMMENT ON FUNCTION trigger_update_aggregated_hosts_on_spec_change() IS
'Updates aggregated_hosts field after AddressGroup spec.hosts changes';

-- +goose StatementEnd
