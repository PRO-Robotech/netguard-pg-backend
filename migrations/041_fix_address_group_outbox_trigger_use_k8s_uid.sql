-- +goose Up
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
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_address_group_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_conflict_occurred INTEGER;
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
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
