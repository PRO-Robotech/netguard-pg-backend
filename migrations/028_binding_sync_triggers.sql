-- +goose Up
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION trigger_ag_update_on_binding_change()
RETURNS TRIGGER AS $$
DECLARE
    v_host_ready BOOLEAN;
    v_ag_ready BOOLEAN;
    v_ag_resource_version BIGINT;
    v_outbox_resource_id UUID;
    v_namespace TEXT;
    v_name TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        v_namespace := OLD.address_group_namespace;
        v_name := OLD.address_group_name;
    ELSE
        v_namespace := NEW.address_group_namespace;
        v_name := NEW.address_group_name;
    END IF;

    IF TG_OP = 'DELETE' THEN
        SELECT EXISTS(
            SELECT 1 FROM hosts h
            JOIN k8s_metadata km ON h.resource_version = km.resource_version
            WHERE h.namespace = OLD.host_namespace
              AND h.name = OLD.host_name
              AND km.conditions @> '[{"type":"Ready","status":"True"}]'::jsonb
        ) INTO v_host_ready;
    ELSE
        SELECT EXISTS(
            SELECT 1 FROM hosts h
            JOIN k8s_metadata km ON h.resource_version = km.resource_version
            WHERE h.namespace = NEW.host_namespace
              AND h.name = NEW.host_name
              AND km.conditions @> '[{"type":"Ready","status":"True"}]'::jsonb
        ) INTO v_host_ready;
    END IF;

    IF v_host_ready IS NULL OR NOT v_host_ready THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        ELSE
            RETURN NEW;
        END IF;
    END IF;

    SELECT ag.resource_version INTO v_ag_resource_version
    FROM address_groups ag
    WHERE ag.namespace = v_namespace
      AND ag.name = v_name;

    IF v_ag_resource_version IS NULL THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        ELSE
            RETURN NEW;
        END IF;
    END IF;

    SELECT EXISTS(
        SELECT 1 FROM k8s_metadata km
        WHERE km.resource_version = v_ag_resource_version
          AND km.conditions @> '[{"type":"Ready","status":"True"}]'::jsonb
    ) INTO v_ag_ready;

    IF NOT v_ag_ready THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        ELSE
            RETURN NEW;
        END IF;
    END IF;

    v_outbox_resource_id := gen_random_uuid();

    IF TG_OP = 'DELETE' THEN
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
            'AddressGroup',
            v_outbox_resource_id,
            OLD.address_group_namespace,
            OLD.address_group_name,
            'UPDATE'::sync_operation,
            'SGROUP'::target_system,
            jsonb_build_object(
                'namespace', OLD.address_group_namespace,
                'name', OLD.address_group_name,
                'binding', jsonb_build_object(
                    'namespace', OLD.namespace,
                    'name', OLD.name
                ),
                'host', jsonb_build_object(
                    'namespace', OLD.host_namespace,
                    'name', OLD.host_name
                )
            ),
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
    ELSE
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
            'AddressGroup',
            v_outbox_resource_id,
            NEW.address_group_namespace,
            NEW.address_group_name,
            'UPDATE'::sync_operation,
            'SGROUP'::target_system,
            jsonb_build_object(
                'namespace', NEW.address_group_namespace,
                'name', NEW.address_group_name,
                'binding', jsonb_build_object(
                    'namespace', NEW.namespace,
                    'name', NEW.name
                ),
                'host', jsonb_build_object(
                    'namespace', NEW.host_namespace,
                    'name', NEW.host_name
                )
            ),
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
    END IF;

    UPDATE address_groups
    SET aggregated_hosts = aggregate_address_group_hosts(v_namespace::TEXT, v_name::TEXT)
    WHERE namespace = v_namespace AND name = v_name;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    ELSE
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ag_update_on_binding_change
    AFTER INSERT OR UPDATE OR DELETE ON host_bindings
    FOR EACH ROW
    EXECUTE FUNCTION trigger_ag_update_on_binding_change();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS trg_ag_update_on_binding_change ON host_bindings;
DROP FUNCTION IF EXISTS trigger_ag_update_on_binding_change();

-- +goose StatementEnd
