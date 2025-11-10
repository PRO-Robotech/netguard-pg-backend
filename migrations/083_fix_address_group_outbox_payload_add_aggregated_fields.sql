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
        NEW.namespace,
        NEW.name,
        v_operation_type,
        'SGROUP',
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'aggregated_hosts', NEW.aggregated_hosts,
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
'Creates sync_outbox entry when AddressGroup is created or updated.
Migration 041: Added aggregated_hosts/networks check.
Migration 083: Added aggregated_hosts and networks to PAYLOAD (critical fix!).
This ensures OutboxWorker sends COMPLETE data to SGROUP.';

DO $$
BEGIN
    RAISE NOTICE '[Migration 083] ✅ Fixed trigger_address_group_upsert_outbox() payload - now includes aggregated_hosts and networks';
END $$;

-- +goose StatementEnd

-- +goose Down
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
        NEW.namespace,
        NEW.name,
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

DO $$
BEGIN
    RAISE NOTICE '[Migration 083 Rollback] Reverted to Migration 041 version - payload WITHOUT aggregated_hosts and networks';
END $$;

-- +goose StatementEnd

