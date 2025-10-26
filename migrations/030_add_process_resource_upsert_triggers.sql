-- +goose Up
-- +goose StatementBegin

-- HostBinding: INSERT/UPDATE trigger to create Outbox entries
CREATE OR REPLACE FUNCTION trigger_host_binding_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_network_namespace TEXT;
    v_network_name TEXT;
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

    -- Get Network reference from Host
    SELECT h.network_namespace, h.network_name
    INTO v_network_namespace, v_network_name
    FROM hosts h
    WHERE h.namespace = NEW.host_namespace
      AND h.name = NEW.host_name;

    -- Build affected_resources array: [Host, Network (if exists), AddressGroup]
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
            'namespace', NEW.namespace,
            'name', NEW.name,
            'host', jsonb_build_object(
                'namespace', NEW.host_namespace,
                'name', NEW.host_name
            ),
            'address_group', jsonb_build_object(
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
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        payload = EXCLUDED.payload,
        affects_resources = EXCLUDED.affects_resources,
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = NOW(),
        updated_at = NOW();

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_host_binding_upsert_outbox
    AFTER INSERT OR UPDATE ON host_bindings
    FOR EACH ROW
    EXECUTE FUNCTION trigger_host_binding_upsert_outbox();

-- NetworkBinding: INSERT/UPDATE trigger to create Outbox entries
CREATE OR REPLACE FUNCTION trigger_network_binding_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_affected_resources JSONB;
BEGIN
    -- Get metadata UID for this NetworkBinding
    SELECT m.uid INTO v_resource_id
    FROM k8s_metadata m
    WHERE m.resource_version = NEW.resource_version;

    IF v_resource_id IS NULL THEN
        RAISE EXCEPTION 'NetworkBinding metadata not found for resource_version=%', NEW.resource_version;
    END IF;

    -- Determine operation type
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
    END IF;

    -- Build affected_resources array: [Network, AddressGroup]
    v_affected_resources := jsonb_build_array(
        jsonb_build_object(
            'type', 'Network',
            'namespace', NEW.network_namespace,
            'name', NEW.network_name
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
        'NetworkBinding',
        v_resource_id,
        v_operation_type,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'network', jsonb_build_object(
                'namespace', NEW.network_namespace,
                'name', NEW.network_name
            ),
            'address_group', jsonb_build_object(
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
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        payload = EXCLUDED.payload,
        affects_resources = EXCLUDED.affects_resources,
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = NOW(),
        updated_at = NOW();

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_network_binding_upsert_outbox
    AFTER INSERT OR UPDATE ON network_bindings
    FOR EACH ROW
    EXECUTE FUNCTION trigger_network_binding_upsert_outbox();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS trg_host_binding_upsert_outbox ON host_bindings;
DROP FUNCTION IF EXISTS trigger_host_binding_upsert_outbox();

DROP TRIGGER IF EXISTS trg_network_binding_upsert_outbox ON network_bindings;
DROP FUNCTION IF EXISTS trigger_network_binding_upsert_outbox();

-- +goose StatementEnd
