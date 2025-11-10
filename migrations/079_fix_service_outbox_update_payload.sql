-- +goose Up

-- +goose StatementBegin

CREATE OR REPLACE FUNCTION trigger_service_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_has_spec_changes BOOLEAN := FALSE;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.description IS DISTINCT FROM NEW.description OR
           OLD.ingress_ports IS DISTINCT FROM NEW.ingress_ports OR
           OLD.address_groups IS DISTINCT FROM NEW.address_groups OR
           OLD.aggregated_address_groups IS DISTINCT FROM NEW.aggregated_address_groups THEN
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
            'Service:' || NEW.namespace || '/' || NEW.name
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
        'Service',
        v_resource_id,
        NEW.namespace,
        NEW.name,
        v_operation_type,
        'SGROUP',
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'aggregated_address_groups', NEW.aggregated_address_groups
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

COMMENT ON FUNCTION trigger_service_upsert_outbox() IS
'Service AFTER INSERT/UPDATE trigger.
Creates/updates sync_outbox entry when Service changes.
Migration 076: Added aggregated_address_groups to payload.
Migration 079: Fixed ON CONFLICT to update payload (was only updating status/attempts).';


DO $$
BEGIN
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '[Migration 079] Service outbox payload update fix COMPLETE';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '';
    RAISE NOTICE 'FIXED: trigger_service_upsert_outbox() ON CONFLICT now updates payload';
    RAISE NOTICE '';
    RAISE NOTICE 'Before Migration 079:';
    RAISE NOTICE '  - Service.aggregated_address_groups updated ✓';
    RAISE NOTICE '  - Trigger fires and detects change ✓';
    RAISE NOTICE '  - ON CONFLICT updates status=PENDING ✓';
    RAISE NOTICE '  - BUT payload stays OLD ✗';
    RAISE NOTICE '  - OutboxWorker syncs OLD data to SGROUP ✗';
    RAISE NOTICE '';
    RAISE NOTICE 'After Migration 079:';
    RAISE NOTICE '  - Service.aggregated_address_groups updated ✓';
    RAISE NOTICE '  - Trigger fires and detects change ✓';
    RAISE NOTICE '  - ON CONFLICT updates status=PENDING AND payload=NEW ✓';
    RAISE NOTICE '  - OutboxWorker syncs NEW data to SGROUP ✓';
    RAISE NOTICE '';
    RAISE NOTICE 'Result: SGROUP will receive correct Service data with updated aggregated_address_groups!';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION trigger_service_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_has_spec_changes BOOLEAN := FALSE;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.description IS DISTINCT FROM NEW.description OR
           OLD.ingress_ports IS DISTINCT FROM NEW.ingress_ports OR
           OLD.address_groups IS DISTINCT FROM NEW.address_groups OR
           OLD.aggregated_address_groups IS DISTINCT FROM NEW.aggregated_address_groups THEN
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
            'Service:' || NEW.namespace || '/' || NEW.name
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
        'Service',
        v_resource_id,
        NEW.namespace,
        NEW.name,
        v_operation_type,
        'SGROUP',
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'aggregated_address_groups', NEW.aggregated_address_groups
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
    RAISE WARNING '[Migration 079 Rollback] Reverted to Migration 076 - payload update bug will return!';
END $$;

-- +goose StatementEnd
