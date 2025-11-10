-- +goose Up
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION trigger_host_binding_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_affected_resources JSONB;
BEGIN
    -- Get metadata UID for this HostBinding
    SELECT m.uid INTO v_resource_id
    FROM k8s_metadata m
    WHERE m.resource_version = NEW.resource_version;

    IF v_resource_id IS NULL THEN
        RAISE EXCEPTION 'HostBinding metadata not found for resource_version=%', NEW.resource_version;
    END IF;

    -- Determine operation type
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
    END IF;

    -- Build affected_resources array: [Host, AddressGroup]
    -- IMPORTANT: HostBinding does NOT involve Network!
    v_affected_resources := jsonb_build_array(
        jsonb_build_object(
            'type', 'Host',
            'namespace', NEW.host_namespace,
            'name', NEW.host_name
        ),
        jsonb_build_object(
            'type', 'AddressGroup',
            'namespace', NEW.address_group_namespace,
            'name', NEW.address_group_name
        )
    );

    -- Insert or update Outbox entry
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
        v_resource_id,
        v_operation_type,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'host', jsonb_build_object(
                'namespace', NEW.host_namespace,
                'name', NEW.host_name
            ),
            'addressGroup', jsonb_build_object(
                'namespace', NEW.address_group_namespace,
                'name', NEW.address_group_name
            )
        ),
        NEW.namespace,
        NEW.name,
        v_affected_resources,
        'PENDING'::outbox_status,
        0,
        5,
        CURRENT_TIMESTAMP,
        CURRENT_TIMESTAMP,
        CURRENT_TIMESTAMP
    )
    -- FIX: Add target_system to match UNIQUE constraint
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO UPDATE SET
        payload = EXCLUDED.payload,
        resource_namespace = EXCLUDED.resource_namespace,
        resource_name = EXCLUDED.resource_name,
        affects_resources = EXCLUDED.affects_resources,
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = CURRENT_TIMESTAMP,
        updated_at = CURRENT_TIMESTAMP;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Restore version with 3 columns (broken)
CREATE OR REPLACE FUNCTION trigger_host_binding_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_affected_resources JSONB;
BEGIN
    SELECT m.uid INTO v_resource_id
    FROM k8s_metadata m
    WHERE m.resource_version = NEW.resource_version;

    IF v_resource_id IS NULL THEN
        RAISE EXCEPTION 'HostBinding metadata not found for resource_version=%', NEW.resource_version;
    END IF;

    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
    END IF;

    v_affected_resources := jsonb_build_array(
        jsonb_build_object(
            'type', 'Host',
            'namespace', NEW.host_namespace,
            'name', NEW.host_name
        ),
        jsonb_build_object(
            'type', 'AddressGroup',
            'namespace', NEW.address_group_namespace,
            'name', NEW.address_group_name
        )
    );

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
        v_resource_id,
        v_operation_type,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'host', jsonb_build_object(
                'namespace', NEW.host_namespace,
                'name', NEW.host_name
            ),
            'addressGroup', jsonb_build_object(
                'namespace', NEW.address_group_namespace,
                'name', NEW.address_group_name
            )
        ),
        NEW.namespace,
        NEW.name,
        v_affected_resources,
        'PENDING'::outbox_status,
        0,
        5,
        CURRENT_TIMESTAMP,
        CURRENT_TIMESTAMP,
        CURRENT_TIMESTAMP
    )
    ON CONFLICT (resource_type, resource_id, operation) DO UPDATE SET
        payload = EXCLUDED.payload,
        resource_namespace = EXCLUDED.resource_namespace,
        resource_name = EXCLUDED.resource_name,
        affects_resources = EXCLUDED.affects_resources,
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = CURRENT_TIMESTAMP,
        updated_at = CURRENT_TIMESTAMP;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd
