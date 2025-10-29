-- +goose Up
-- +goose StatementBegin

-- Add comprehensive logging to trigger_host_before_delete
CREATE OR REPLACE FUNCTION trigger_host_before_delete()
RETURNS TRIGGER AS $$
DECLARE
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
BEGIN
    RAISE NOTICE '[TRIGGER_DELETE] ===== trigger_host_before_delete CALLED =====';
    RAISE NOTICE '[TRIGGER_DELETE] Host: %.% (resource_version=%)', OLD.namespace, OLD.name, OLD.resource_version;
    RAISE NOTICE '[TRIGGER_DELETE] Host UUID: %', OLD.uuid;

    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    RAISE NOTICE '[TRIGGER_DELETE] Metadata UID: %', v_uid;
    RAISE NOTICE '[TRIGGER_DELETE] deletion_timestamp already set: %', v_already_marked_for_deletion;

    IF v_already_marked_for_deletion THEN
        RAISE NOTICE '[TRIGGER_DELETE] *** ALLOWING PHYSICAL DELETION *** (deletion_timestamp already set)';
        RAISE NOTICE '[TRIGGER_DELETE] Host %.% will be PHYSICALLY DELETED from database', OLD.namespace, OLD.name;
        RETURN OLD;
    END IF;

    RAISE NOTICE '[TRIGGER_DELETE] *** PREVENTING PHYSICAL DELETION *** (first attempt)';
    RAISE NOTICE '[TRIGGER_DELETE] Setting deletion_timestamp and creating DELETE outbox entry for %.%', OLD.namespace, OLD.name;

    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting SGROUP sync before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    RAISE NOTICE '[TRIGGER_DELETE] deletion_timestamp set for %.%', OLD.namespace, OLD.name;

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
        'Host',
        v_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'uuid', OLD.uuid
        ),
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    RAISE NOTICE '[TRIGGER_DELETE] DELETE outbox entry created for %.%', OLD.namespace, OLD.name;
    RAISE NOTICE '[TRIGGER_DELETE] Returning NULL - physical deletion PREVENTED';

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Add comprehensive logging to trigger_network_before_delete
CREATE OR REPLACE FUNCTION trigger_network_before_delete()
RETURNS TRIGGER AS $$
DECLARE
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
BEGIN
    RAISE NOTICE '[TRIGGER_DELETE] ===== trigger_network_before_delete CALLED =====';
    RAISE NOTICE '[TRIGGER_DELETE] Network: %.% (resource_version=%)', OLD.namespace, OLD.name, OLD.resource_version;
    RAISE NOTICE '[TRIGGER_DELETE] Network CIDR: %', OLD.cidr;

    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    RAISE NOTICE '[TRIGGER_DELETE] Metadata UID: %', v_uid;
    RAISE NOTICE '[TRIGGER_DELETE] deletion_timestamp already set: %', v_already_marked_for_deletion;

    IF v_already_marked_for_deletion THEN
        RAISE NOTICE '[TRIGGER_DELETE] *** ALLOWING PHYSICAL DELETION *** (deletion_timestamp already set)';
        RAISE NOTICE '[TRIGGER_DELETE] Network %.% will be PHYSICALLY DELETED from database', OLD.namespace, OLD.name;
        RETURN OLD;
    END IF;

    RAISE NOTICE '[TRIGGER_DELETE] *** PREVENTING PHYSICAL DELETION *** (first attempt)';
    RAISE NOTICE '[TRIGGER_DELETE] Setting deletion_timestamp and creating DELETE outbox entry for %.%', OLD.namespace, OLD.name;

    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting SGROUP sync before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    RAISE NOTICE '[TRIGGER_DELETE] deletion_timestamp set for %.%', OLD.namespace, OLD.name;

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
        'Network',
        v_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'cidr', OLD.cidr::text
        ),
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    RAISE NOTICE '[TRIGGER_DELETE] DELETE outbox entry created for %.%', OLD.namespace, OLD.name;
    RAISE NOTICE '[TRIGGER_DELETE] Returning NULL - physical deletion PREVENTED';

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Restore original functions without logging (from migration 026)
CREATE OR REPLACE FUNCTION trigger_host_before_delete()
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
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting SGROUP sync before deletion"}]'::jsonb
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
        'Host',
        v_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'uuid', OLD.uuid
        ),
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION trigger_network_before_delete()
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
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting SGROUP sync before deletion"}]'::jsonb
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
        'Network',
        v_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'cidr', OLD.cidr::text
        ),
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd
