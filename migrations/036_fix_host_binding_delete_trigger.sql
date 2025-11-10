-- +goose Up
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION trigger_host_binding_before_delete()
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

    -- Build affected_resources array: [Host, AddressGroup]
    -- IMPORTANT: HostBinding does NOT involve Network!
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
        operation,
        target_system,
        payload,
        resource_namespace,
        resource_name,
        affects_resources,
        status,
        attempts,
        max_retries,
        next_retry_at,
        created_at,
        updated_at
    )
    VALUES (
        'HostBinding',
        v_uid,
        'DELETE'::sync_operation,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'host', jsonb_build_object(
                'namespace', OLD.host_namespace,
                'name', OLD.host_name
            ),
            'addressGroup', jsonb_build_object(
                'namespace', OLD.address_group_namespace,
                'name', OLD.address_group_name
            )
        ),
        OLD.namespace,
        OLD.name,
        v_affected_resources,
        'PENDING'::outbox_status,
        0,
        5,
        CURRENT_TIMESTAMP,
        CURRENT_TIMESTAMP,
        CURRENT_TIMESTAMP
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO UPDATE SET
        payload = EXCLUDED.payload,
        resource_namespace = EXCLUDED.resource_namespace,
        resource_name = EXCLUDED.resource_name,
        affects_resources = EXCLUDED.affects_resources,
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = CURRENT_TIMESTAMP,
        updated_at = CURRENT_TIMESTAMP;

    RAISE NOTICE 'Created DELETE outbox entry for HostBinding %/%', OLD.namespace, OLD.name;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Restore old (broken) version with network_namespace references
CREATE OR REPLACE FUNCTION trigger_host_binding_before_delete()
RETURNS TRIGGER AS $$
DECLARE
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
    v_network_namespace TEXT;
    v_network_name TEXT;
    v_affected_resources JSONB;
BEGIN
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    IF v_already_marked_for_deletion THEN
        RETURN OLD;
    END IF;

    -- BROKEN: Try to get network_namespace/network_name from hosts (columns don't exist)
    SELECT h.network_namespace, h.network_name
    INTO v_network_namespace, v_network_name
    FROM hosts h
    WHERE h.namespace = OLD.host_namespace
      AND h.name = OLD.host_name;

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

    IF v_network_namespace IS NOT NULL AND v_network_name IS NOT NULL THEN
        v_affected_resources := v_affected_resources || jsonb_build_array(
            jsonb_build_object(
                'type', 'Network',
                'namespace', v_network_namespace,
                'name', v_network_name
            )
        );
    END IF;

    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting internal processing before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        operation,
        target_system,
        payload,
        resource_namespace,
        resource_name,
        affects_resources,
        status,
        attempts,
        max_retries,
        next_retry_at,
        created_at,
        updated_at
    )
    VALUES (
        'HostBinding',
        v_uid,
        'DELETE'::sync_operation,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'host', jsonb_build_object(
                'namespace', OLD.host_namespace,
                'name', OLD.host_name
            ),
            'addressGroup', jsonb_build_object(
                'namespace', OLD.address_group_namespace,
                'name', OLD.address_group_name
            )
        ),
        OLD.namespace,
        OLD.name,
        v_affected_resources,
        'PENDING'::outbox_status,
        0,
        5,
        CURRENT_TIMESTAMP,
        CURRENT_TIMESTAMP,
        CURRENT_TIMESTAMP
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO UPDATE SET
        payload = EXCLUDED.payload,
        resource_namespace = EXCLUDED.resource_namespace,
        resource_name = EXCLUDED.resource_name,
        affects_resources = EXCLUDED.affects_resources,
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = CURRENT_TIMESTAMP,
        updated_at = CURRENT_TIMESTAMP;

    RAISE NOTICE 'Created DELETE outbox entry for HostBinding %/%', OLD.namespace, OLD.name;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd
