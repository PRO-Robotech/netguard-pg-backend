-- +goose Up
-- +goose StatementBegin

-- Migration 091: Restore AddressGroup networks tracking in outbox trigger
-- Background:
--   - Migration 087 отключил обработку поля networks, чтобы не реагировать на Host/Network bindings
--   - Для сетей требуется AddressGroup-centric синхронизация через массив networks (SGROUP не
--     поддерживает Network.sgName). Нужен полный список сетей в payload, включая очистку ([""]).

CREATE OR REPLACE FUNCTION trigger_address_group_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.default_action IS DISTINCT FROM NEW.default_action OR
           OLD.logs IS DISTINCT FROM NEW.logs OR
           OLD.trace IS DISTINCT FROM NEW.trace OR
           OLD.networks IS DISTINCT FROM NEW.networks THEN
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

    IF v_resource_id IS NULL THEN
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
        NEW.namespace,
        NEW.name,
        v_operation_type,
        'SGROUP',
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'defaultAction', NEW.default_action,
            'logs', NEW.logs,
            'trace', NEW.trace,
            'networks', NEW.networks
        ),
        'PENDING',
        0,
        5
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING',
        attempts = 0,
        updated_at = NOW(),
        payload = EXCLUDED.payload;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_address_group_upsert_outbox() IS
'Creates sync_outbox entry when AddressGroup intrinsic spec or networks change. Hosts remain ignored (host-centric sync).';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Roll back to Migration 090 behaviour (no networks tracking)
CREATE OR REPLACE FUNCTION trigger_address_group_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.default_action IS DISTINCT FROM NEW.default_action OR
           OLD.logs IS DISTINCT FROM NEW.logs OR
           OLD.trace IS DISTINCT FROM NEW.trace THEN
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

    IF v_resource_id IS NULL THEN
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
        NEW.namespace,
        NEW.name,
        v_operation_type,
        'SGROUP',
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'defaultAction', NEW.default_action,
            'logs', NEW.logs,
            'trace', NEW.trace
        ),
        'PENDING',
        0,
        5
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING',
        attempts = 0,
        updated_at = NOW(),
        payload = EXCLUDED.payload;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_address_group_upsert_outbox() IS
'Creates sync_outbox entry when AddressGroup intrinsic spec changes (default_action, logs, trace).';

-- +goose StatementEnd

