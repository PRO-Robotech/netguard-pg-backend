-- +goose Up

-- Prevent condition-only metadata bumps from re-enqueuing SGROUP sync for svc-svc rules
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_svcsvc_rule_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.service_from_ref::text IS NOT DISTINCT FROM OLD.service_from_ref::text
           AND NEW.service_to_ref::text IS NOT DISTINCT FROM OLD.service_to_ref::text
           AND NEW.action IS NOT DISTINCT FROM OLD.action
           AND NEW.priority IS NOT DISTINCT FROM OLD.priority
           AND NEW.logs IS NOT DISTINCT FROM OLD.logs
           AND NEW.trace IS NOT DISTINCT FROM OLD.trace THEN
            -- Only resource_version/metadata changed → skip outbox update
            RETURN NEW;
        END IF;
        v_operation_type := 'UPDATE'::sync_operation;
    ELSE
        v_operation_type := 'CREATE'::sync_operation;
    END IF;

    v_resource_id := uuid_generate_v5(
        uuid_ns_dns(),
        'SvcSvcRule:' || NEW.namespace || '/' || NEW.name
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
        'SvcSvcRule',
        v_resource_id,
        v_operation_type,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'service_from', NEW.service_from_ref,
            'service_to', NEW.service_to_ref,
            'action', NEW.action,
            'priority', NEW.priority,
            'logs', NEW.logs,
            'trace', NEW.trace,
            'description', NEW.description
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
        updated_at = NOW(),
        payload = EXCLUDED.payload;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Prevent condition-only metadata bumps from re-enqueuing SGROUP sync for svc-fqdn rules
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_svcfqdn_rule_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.service_from_ref::text IS NOT DISTINCT FROM OLD.service_from_ref::text
           AND NEW.fqdn IS NOT DISTINCT FROM OLD.fqdn
           AND NEW.transport IS NOT DISTINCT FROM OLD.transport
           AND NEW.ports::text IS NOT DISTINCT FROM OLD.ports::text
           AND NEW.logs IS NOT DISTINCT FROM OLD.logs
           AND NEW.trace IS NOT DISTINCT FROM OLD.trace
           AND NEW.action IS NOT DISTINCT FROM OLD.action
           AND NEW.priority IS NOT DISTINCT FROM OLD.priority THEN
            RETURN NEW;
        END IF;
        v_operation_type := 'UPDATE'::sync_operation;
    ELSE
        v_operation_type := 'CREATE'::sync_operation;
    END IF;

    v_resource_id := uuid_generate_v5(
        uuid_ns_dns(),
        'SvcFqdnRule:' || NEW.namespace || '/' || NEW.name
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
        'SvcFqdnRule',
        v_resource_id,
        v_operation_type,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'service_from_ref', NEW.service_from_ref,
            'fqdn', NEW.fqdn,
            'transport', NEW.transport,
            'ports', NEW.ports,
            'logs', NEW.logs,
            'trace', NEW.trace,
            'action', NEW.action,
            'priority', NEW.priority,
            'description', NEW.description
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
        updated_at = NOW(),
        payload = EXCLUDED.payload;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down

-- Restore previous behavior for svc-svc rule trigger (always enqueue on UPDATE)
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_svcsvc_rule_upsert_outbox()
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
        'SvcSvcRule:' || NEW.namespace || '/' || NEW.name
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
        'SvcSvcRule',
        v_resource_id,
        v_operation_type,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'service_from', NEW.service_from_ref,
            'service_to', NEW.service_to_ref,
            'action', NEW.action,
            'priority', NEW.priority,
            'logs', NEW.logs,
            'trace', NEW.trace,
            'description', NEW.description
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
        updated_at = NOW(),
        payload = EXCLUDED.payload;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Restore previous behavior for svc-fqdn rule trigger
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_svcfqdn_rule_upsert_outbox()
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
        'SvcFqdnRule:' || NEW.namespace || '/' || NEW.name
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
        'SvcFqdnRule',
        v_resource_id,
        v_operation_type,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'service_from_ref', NEW.service_from_ref,
            'fqdn', NEW.fqdn,
            'transport', NEW.transport,
            'ports', NEW.ports,
            'logs', NEW.logs,
            'trace', NEW.trace,
            'action', NEW.action,
            'priority', NEW.priority,
            'description', NEW.description
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
        updated_at = NOW(),
        payload = EXCLUDED.payload;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

