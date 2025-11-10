-- +goose Up
-- +goose StatementBegin


CREATE OR REPLACE FUNCTION trigger_addressgroupbinding_on_deletion_mark()
RETURNS TRIGGER AS $$
DECLARE
    v_service_namespace namespace_name;
    v_service_name resource_name;
    v_ag_namespace namespace_name;
    v_ag_name resource_name;
    v_service_rv BIGINT;
    v_ag_rv BIGINT;
    v_binding_namespace namespace_name;
    v_binding_name resource_name;
    v_uid UUID;
    v_affected_resources JSONB;
    v_binding_found BOOLEAN := FALSE;
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
            RAISE NOTICE '[Migration 066] AddressGroupBinding deletion marked: service=%.%, ag=%.%',
                v_service_namespace, v_service_name, v_ag_namespace, v_ag_name;

            SELECT resource_version INTO v_service_rv
            FROM services
            WHERE namespace = v_service_namespace AND name = v_service_name;

            SELECT resource_version INTO v_ag_rv
            FROM address_groups
            WHERE namespace = v_ag_namespace AND name = v_ag_name;

            v_affected_resources := jsonb_build_array(
                jsonb_build_object(
                    'type', 'Service',
                    'namespace', v_service_namespace,
                    'name', v_service_name,
                    'resourceVersion', v_service_rv
                ),
                jsonb_build_object(
                    'type', 'AddressGroup',
                    'namespace', v_ag_namespace,
                    'name', v_ag_name,
                    'resourceVersion', v_ag_rv
                )
            );

            INSERT INTO sync_outbox (
                resource_type,
                resource_id,
                resource_namespace,
                resource_name,
                operation,
                target_system,
                payload,
                status,
                affects_resources,
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
                    ),
                    'affectedResources', v_affected_resources,
                    'reason', 'Coordinated deletion (Service or AddressGroup deletion)',
                    'deletionTimestamp', NEW.deletion_timestamp::text
                ),
                'PENDING'::outbox_status,
                v_affected_resources,
                0,
                20,
                NOW(),
                NOW(),
                NOW()
            )
            ON CONFLICT DO NOTHING;

            RAISE NOTICE '[Migration 066] Created DELETE outbox entry for AddressGroupBinding %.%',
                v_binding_namespace, v_binding_name;

        ELSE
            RAISE WARNING '[Migration 066] AddressGroupBinding not found for resource_version %',
                NEW.resource_version;
        END IF;

    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER addressgroupbinding_on_deletion_mark
AFTER UPDATE OF deletion_timestamp ON k8s_metadata
FOR EACH ROW
WHEN (NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL)
EXECUTE FUNCTION trigger_addressgroupbinding_on_deletion_mark();

COMMENT ON FUNCTION trigger_addressgroupbinding_on_deletion_mark() IS
'Creates DELETE outbox entry when AddressGroupBinding is marked for deletion (deletion_timestamp set).
Part of coordinated deletion pattern. See: docs/architecture/COORDINATED_BINDING_DELETION.md';

COMMENT ON TRIGGER addressgroupbinding_on_deletion_mark ON k8s_metadata IS
'Triggers DELETE outbox entry creation when AddressGroupBinding.deletion_timestamp is set.
Enables coordinated deletion for Service → AddressGroupBinding CASCADE.';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS addressgroupbinding_on_deletion_mark ON k8s_metadata;
DROP FUNCTION IF EXISTS trigger_addressgroupbinding_on_deletion_mark();

-- +goose StatementEnd
