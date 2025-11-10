-- +goose Up
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
        IF OLD.aggregated_hosts IS DISTINCT FROM NEW.aggregated_hosts THEN
        END IF;
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
    GET DIAGNOSTICS v_conflict_occurred = ROW_COUNT;
    IF v_conflict_occurred > 0 THEN
    ELSE
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE OR REPLACE FUNCTION trigger_update_aggregated_hosts_on_spec_change()
RETURNS TRIGGER AS $$
DECLARE
    v_rows_affected INTEGER;
BEGIN
    UPDATE address_groups
    SET aggregated_hosts = aggregate_address_group_hosts(NEW.namespace, NEW.name)
    WHERE namespace = NEW.namespace AND name = NEW.name;
    GET DIAGNOSTICS v_rows_affected = ROW_COUNT;
    IF v_rows_affected = 0 THEN
    ELSIF v_rows_affected > 1 THEN
    ELSE
    END IF;
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
