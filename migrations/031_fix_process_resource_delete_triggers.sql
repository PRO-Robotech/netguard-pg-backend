-- +goose Up
-- +goose StatementBegin

DROP TRIGGER IF EXISTS network_binding_before_delete ON network_bindings;
DROP FUNCTION IF EXISTS trigger_network_binding_before_delete();

CREATE OR REPLACE FUNCTION trigger_network_binding_before_delete()
RETURNS TRIGGER AS $$
DECLARE
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
    v_affected_resources JSONB;
BEGIN
    -- Get UID and check if already marked for deletion
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    IF v_already_marked_for_deletion THEN
        RETURN OLD;
    END IF;

    -- Build affected_resources array BEFORE DELETE
    -- CRITICAL: Include Network and AddressGroup that this binding connects
    v_affected_resources := jsonb_build_array(
        jsonb_build_object(
            'type', 'Network',
            'namespace', OLD.network_namespace,
            'name', OLD.network_name
        ),
        jsonb_build_object(
            'type', 'AddressGroup',
            'namespace', OLD.address_group_namespace,
            'name', OLD.address_group_name
        )
    );

    -- Mark metadata for deletion
    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting internal processing before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    -- Create DELETE Outbox entry with affects_resources
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        resource_namespace,
        resource_name,
        operation,
        target_system,
        payload,
        affects_resources,  -- ADDED: affects_resources field
        status,
        attempts,
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'NetworkBinding',
        v_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'network', jsonb_build_object(
                'namespace', OLD.network_namespace,
                'name', OLD.network_name
            ),
            'address_group', jsonb_build_object(
                'namespace', OLD.address_group_namespace,
                'name', OLD.address_group_name
            )
        ),
        v_affected_resources,  -- ADDED: populated with Network and AddressGroup
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    );

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER network_binding_before_delete
    BEFORE DELETE ON network_bindings
    FOR EACH ROW
    EXECUTE FUNCTION trigger_network_binding_before_delete();

DROP TRIGGER IF EXISTS host_binding_before_delete ON host_bindings;
DROP FUNCTION IF EXISTS trigger_host_binding_before_delete();

CREATE OR REPLACE FUNCTION trigger_host_binding_before_delete()
RETURNS TRIGGER AS $$
DECLARE
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
    v_network_namespace TEXT;
    v_network_name TEXT;
    v_affected_resources JSONB;
BEGIN
    -- Get UID and check if already marked for deletion
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    IF v_already_marked_for_deletion THEN
        RETURN OLD;
    END IF;

    -- Get Network reference from Host (if exists)
    SELECT h.network_namespace, h.network_name
    INTO v_network_namespace, v_network_name
    FROM hosts h
    WHERE h.namespace = OLD.host_namespace
      AND h.name = OLD.host_name;

    -- Build affected_resources array: [Host, AddressGroup, Network (optional)]
    v_affected_resources := jsonb_build_array(
        jsonb_build_object(
            'type', 'Host',
            'namespace', OLD.host_namespace,
            'name', OLD.host_name
        ),
        jsonb_build_object(
            'type', 'AddressGroup',
            'namespace', OLD.address_group_namespace,
            'name', OLD.address_group_name
        )
    );

    -- Add Network if Host has one
    IF v_network_namespace IS NOT NULL AND v_network_name IS NOT NULL THEN
        v_affected_resources := v_affected_resources || jsonb_build_array(
            jsonb_build_object(
                'type', 'Network',
                'namespace', v_network_namespace,
                'name', v_network_name
            )
        );
    END IF;

    -- Mark metadata for deletion
    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting internal processing before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    -- Create DELETE Outbox entry with affects_resources
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        resource_namespace,
        resource_name,
        operation,
        target_system,
        payload,
        affects_resources,  -- ADDED: affects_resources field
        status,
        attempts,
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'HostBinding',
        v_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'host', jsonb_build_object(
                'namespace', OLD.host_namespace,
                'name', OLD.host_name
            ),
            'address_group', jsonb_build_object(
                'namespace', OLD.address_group_namespace,
                'name', OLD.address_group_name
            )
        ),
        v_affected_resources,  -- ADDED: populated with Host, AddressGroup, and optionally Network
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    );

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER host_binding_before_delete
    BEFORE DELETE ON host_bindings
    FOR EACH ROW
    EXECUTE FUNCTION trigger_host_binding_before_delete();

-- NOTE: UNIQUE constraint (network_namespace, network_name) NOT added in this migration
-- because existing duplicate data prevents constraint creation.
-- Application-level validation in NetworkBindingValidator will prevent new duplicates.
-- Database constraint can be added manually after cleaning up existing duplicates.

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to migration 027 triggers (without affects_resources)
DROP TRIGGER IF EXISTS network_binding_before_delete ON network_bindings;
DROP FUNCTION IF EXISTS trigger_network_binding_before_delete();

CREATE OR REPLACE FUNCTION trigger_network_binding_before_delete()
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
        'NetworkBinding',
        v_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'network', jsonb_build_object(
                'namespace', OLD.network_namespace,
                'name', OLD.network_name
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
    );

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER network_binding_before_delete
    BEFORE DELETE ON network_bindings
    FOR EACH ROW
    EXECUTE FUNCTION trigger_network_binding_before_delete();

DROP TRIGGER IF EXISTS host_binding_before_delete ON host_bindings;
DROP FUNCTION IF EXISTS trigger_host_binding_before_delete();

CREATE OR REPLACE FUNCTION trigger_host_binding_before_delete()
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
        'HostBinding',
        v_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'host', jsonb_build_object(
                'namespace', OLD.host_namespace,
                'name', OLD.host_name
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
    );

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER host_binding_before_delete
    BEFORE DELETE ON host_bindings
    FOR EACH ROW
    EXECUTE FUNCTION trigger_host_binding_before_delete();

-- +goose StatementEnd
