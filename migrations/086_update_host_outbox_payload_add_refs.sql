-- +goose Up
-- +goose StatementBegin

-- Migration 086: Enrich host outbox payload with binding information.
-- Purpose: ensure OutboxWorker reconstructs Host with AddressGroup/Binding refs
-- so Host sync can populate sgName when bound to an AddressGroup.

CREATE OR REPLACE FUNCTION trigger_host_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_has_spec_changes BOOLEAN := FALSE;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
        v_has_spec_changes := TRUE;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.uuid IS DISTINCT FROM NEW.uuid OR
           OLD.is_bound IS DISTINCT FROM NEW.is_bound OR
           OLD.host_name_sync IS DISTINCT FROM NEW.host_name_sync OR
           OLD.address_group_name IS DISTINCT FROM NEW.address_group_name OR
           OLD.binding_ref_namespace IS DISTINCT FROM NEW.binding_ref_namespace OR
           OLD.binding_ref_name IS DISTINCT FROM NEW.binding_ref_name OR
           OLD.address_group_ref_namespace IS DISTINCT FROM NEW.address_group_ref_namespace OR
           OLD.address_group_ref_name IS DISTINCT FROM NEW.address_group_ref_name OR
           OLD.ip_list IS DISTINCT FROM NEW.ip_list THEN
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

    IF v_resource_id IS NULL THEN
        v_resource_id := uuid_generate_v5(
            uuid_ns_dns(),
            'Host:' || NEW.namespace || '/' || NEW.name
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
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'Host',
        v_resource_id,
        NEW.namespace,
        NEW.name,
        v_operation_type,
        'SGROUP',
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'uuid', NEW.uuid::text,
            'is_bound', NEW.is_bound,
            'host_name_sync', NEW.host_name_sync,
            'ip_list', NEW.ip_list,
            'address_group_name', NEW.address_group_name,
            'address_group_ref_namespace', NEW.address_group_ref_namespace,
            'address_group_ref_name', NEW.address_group_ref_name,
            'binding_ref_namespace', NEW.binding_ref_namespace,
            'binding_ref_name', NEW.binding_ref_name
        ),
        'PENDING',
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING',
        attempts = 0,
        next_retry_at = NOW(),
        updated_at = NOW(),
        payload = EXCLUDED.payload;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_host_upsert_outbox() IS
'Creates sync_outbox entry when Host is created or updated. Migration 086 adds binding references to payload so Host sync can compute sgName.';

DO $$
BEGIN
    RAISE NOTICE '[Migration 086] ✅ trigger_host_upsert_outbox now includes binding refs in payload';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to Migration 048 version (without binding refs)
CREATE OR REPLACE FUNCTION trigger_host_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.uuid IS DISTINCT FROM NEW.uuid OR
           OLD.is_bound IS DISTINCT FROM NEW.is_bound OR
           OLD.host_name_sync IS DISTINCT FROM NEW.host_name_sync OR
           OLD.ip_list IS DISTINCT FROM NEW.ip_list THEN
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
            'Host:' || NEW.namespace || '/' || NEW.name
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
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'Host',
        v_resource_id,
        NEW.namespace,
        NEW.name,
        v_operation_type,
        'SGROUP',
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'uuid', NEW.uuid::text,
            'is_bound', NEW.is_bound,
            'host_name_sync', NEW.host_name_sync,
            'ip_list', NEW.ip_list
        ),
        'PENDING',
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING',
        attempts = 0,
        next_retry_at = NOW(),
        updated_at = NOW(),
        payload = EXCLUDED.payload;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    RAISE NOTICE '[Migration 086 Rollback] Restored trigger_host_upsert_outbox without binding refs.';
END $$;

-- +goose StatementEnd

