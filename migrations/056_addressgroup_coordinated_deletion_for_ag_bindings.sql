-- +goose Up
-- +goose StatementBegin


CREATE OR REPLACE FUNCTION trigger_addressgroup_on_deletion_mark_for_ag_bindings()
RETURNS TRIGGER AS $$
DECLARE
    v_ag_namespace namespace_name;
    v_ag_name resource_name;
    binding_rec RECORD;
    v_binding_uid UUID;
    v_service_uid UUID;
    v_service_rv BIGINT;
BEGIN
    IF NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL THEN
        SELECT namespace, name
        INTO v_ag_namespace, v_ag_name
        FROM address_groups
        WHERE resource_version = NEW.resource_version;

        IF FOUND THEN
            RAISE NOTICE '[Migration 056] AddressGroup %.% marked for deletion, processing AddressGroupBindings',
                v_ag_namespace, v_ag_name;

            FOR binding_rec IN
                SELECT agb.namespace, agb.name,
                       agb.service_namespace, agb.service_name,
                       agb.resource_version
                FROM address_group_bindings agb
                WHERE agb.address_group_namespace = v_ag_namespace
                  AND agb.address_group_name = v_ag_name
            LOOP
                RAISE NOTICE '[Migration 056] Processing AddressGroupBinding %.%',
                    binding_rec.namespace, binding_rec.name;

                SELECT m.uid INTO v_binding_uid
                FROM k8s_metadata m
                WHERE m.resource_version = binding_rec.resource_version;

                SELECT m.uid, s.resource_version
                INTO v_service_uid, v_service_rv
                FROM services s
                JOIN k8s_metadata m ON m.resource_version = s.resource_version
                WHERE s.namespace = binding_rec.service_namespace
                  AND s.name = binding_rec.service_name;

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
                    created_at,
                    updated_at,
                    next_retry_at
                ) VALUES (
                    'AddressGroupBinding',
                    v_binding_uid,
                    binding_rec.namespace,
                    binding_rec.name,
                    'DELETE'::sync_operation,
                    'INTERNAL'::target_system,
                    jsonb_build_object(
                        'addressGroupBinding', jsonb_build_object(
                            'namespace', binding_rec.namespace,
                            'name', binding_rec.name,
                            'serviceRef', jsonb_build_object(
                                'namespace', binding_rec.service_namespace,
                                'name', binding_rec.service_name
                            ),
                            'addressGroupRef', jsonb_build_object(
                                'namespace', v_ag_namespace,
                                'name', v_ag_name
                            )
                        ),
                        'reason', 'AddressGroup deletion (coordinated)',
                        'deletionTimestamp', NEW.deletion_timestamp::text
                    ),
                    'PENDING'::outbox_status,
                    jsonb_build_array(
                        jsonb_build_object(
                            'type', 'Service',
                            'uid', v_service_uid,
                            'namespace', binding_rec.service_namespace,
                            'name', binding_rec.service_name,
                            'resourceVersion', v_service_rv
                        )
                    ),
                    NOW(),
                    NOW(),
                    NOW()
                ) ON CONFLICT DO NOTHING;

                RAISE NOTICE '[Migration 056] Created DELETE outbox entry for AddressGroupBinding %.%',
                    binding_rec.namespace, binding_rec.name;

                PERFORM aggregate_service_address_groups(
                    binding_rec.service_namespace,
                    binding_rec.service_name
                );

                RAISE NOTICE '[Migration 056] Rebuilt Service %.% aggregated_address_groups',
                    binding_rec.service_namespace, binding_rec.service_name;
            END LOOP;

            RAISE NOTICE '[Migration 056] Finished processing AddressGroupBindings for AddressGroup %.%',
                v_ag_namespace, v_ag_name;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER addressgroup_on_deletion_mark_for_ag_bindings
AFTER UPDATE OF deletion_timestamp ON k8s_metadata
FOR EACH ROW
WHEN (NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL)
EXECUTE FUNCTION trigger_addressgroup_on_deletion_mark_for_ag_bindings();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS addressgroup_on_deletion_mark_for_ag_bindings ON k8s_metadata;
DROP FUNCTION IF EXISTS trigger_addressgroup_on_deletion_mark_for_ag_bindings();

-- +goose StatementEnd
