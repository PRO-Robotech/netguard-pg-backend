-- +goose Up

-- +goose StatementBegin


CREATE OR REPLACE FUNCTION trigger_address_group_binding_on_deletion_mark()
RETURNS TRIGGER AS $$
DECLARE
    v_service_namespace namespace_name;
    v_service_name resource_name;
    v_ag_namespace namespace_name;
    v_ag_name resource_name;
    v_binding_namespace namespace_name;
    v_binding_name resource_name;
    v_uid UUID;
    v_binding_found BOOLEAN := false;
BEGIN
    IF NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL THEN

        v_uid := NEW.uid;

        SELECT service_namespace, service_name,
               address_group_namespace, address_group_name,
               namespace, name
        INTO v_service_namespace, v_service_name,
             v_ag_namespace, v_ag_name,
             v_binding_namespace, v_binding_name
        FROM address_group_bindings
        WHERE resource_version = NEW.resource_version;

        v_binding_found := FOUND;

        IF v_binding_found THEN
            RAISE NOTICE 'AddressGroupBinding deletion marked: service=%.%, ag=%/%',
                v_service_namespace, v_service_name, v_ag_namespace, v_ag_name;

            UPDATE services
            SET aggregated_address_groups = aggregate_service_address_groups(
                v_service_namespace::text,
                v_service_name::text
            )
            WHERE namespace = v_service_namespace
              AND name = v_service_name;

            RAISE NOTICE 'Service %.% updated: aggregated_address_groups recalculated (binding excluded)',
                v_service_namespace, v_service_name;

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
                'AddressGroupBinding',
                v_uid,
                v_binding_namespace,
                v_binding_name,
                'DELETE'::sync_operation,
                'INTERNAL'::target_system,
                jsonb_build_object(
                    'namespace', v_binding_namespace,
                    'name', v_binding_name,
                    'serviceRef', jsonb_build_object(
                        'namespace', v_service_namespace,
                        'name', v_service_name
                    ),
                    'addressGroupRef', jsonb_build_object(
                        'namespace', v_ag_namespace,
                        'name', v_ag_name
                    )
                ),
                'PENDING'::outbox_status,
                0,
                5,
                NOW(),
                NOW(),
                NOW()
            )
            ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

            RAISE NOTICE 'AddressGroupBinding %.% DELETE entry created in sync_outbox (target_system=INTERNAL)',
                v_binding_namespace, v_binding_name;

            DELETE FROM address_group_bindings
            WHERE resource_version = NEW.resource_version;

            RAISE NOTICE 'AddressGroupBinding %.% physically deleted', v_binding_namespace, v_binding_name;

        ELSE
            RAISE WARNING 'AddressGroupBinding not found for resource_version %, creating minimal DELETE entry', NEW.resource_version;

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
                'AddressGroupBinding',
                v_uid,
                'unknown',
                'unknown',
                'DELETE'::sync_operation,
                'INTERNAL'::target_system,
                jsonb_build_object(
                    'resourceVersion', NEW.resource_version,
                    'note', 'Minimal DELETE entry - binding already removed (race condition)'
                ),
                'PENDING'::outbox_status,
                0,
                5,
                NOW(),
                NOW(),
                NOW()
            )
            ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

            RAISE NOTICE 'Minimal AddressGroupBinding DELETE entry created for resource_version % (uid=%)',
                NEW.resource_version, v_uid;
        END IF;

    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_address_group_binding_on_deletion_mark() IS
'Handles AddressGroupBinding coordinated deletion when deletion_timestamp is set.
Updates Service.aggregated_address_groups which triggers Service UPDATE and SGROUP sync.
Physically deletes binding after Service update.
Fixes CASCADE DELETE issue where triggers were bypassed.';

CREATE TRIGGER address_group_binding_on_deletion_mark
AFTER UPDATE OF deletion_timestamp ON k8s_metadata
FOR EACH ROW
WHEN (
    NEW.deletion_timestamp IS NOT NULL
    AND OLD.deletion_timestamp IS NULL
)
EXECUTE FUNCTION trigger_address_group_binding_on_deletion_mark();

COMMENT ON TRIGGER address_group_binding_on_deletion_mark ON k8s_metadata IS
'Fires when AddressGroupBinding is soft-deleted (deletion_timestamp set).
Updates Service before physical deletion to ensure SGROUP sync.';

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

DROP TRIGGER IF EXISTS address_group_binding_on_deletion_mark ON k8s_metadata;
DROP FUNCTION IF EXISTS trigger_address_group_binding_on_deletion_mark();

-- +goose StatementEnd
