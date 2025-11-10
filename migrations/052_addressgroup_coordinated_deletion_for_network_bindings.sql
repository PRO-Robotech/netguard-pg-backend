-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_address_group_on_deletion_mark()
RETURNS TRIGGER AS $$
DECLARE
    v_ag_namespace namespace_name;
    v_ag_name resource_name;
    binding_rec RECORD;
    v_binding_uid UUID;
    v_network_uid UUID;
    v_network_rv BIGINT;
BEGIN
    IF NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL THEN

        SELECT namespace, name
        INTO v_ag_namespace, v_ag_name
        FROM address_groups
        WHERE resource_version = NEW.resource_version;

        IF FOUND THEN
            RAISE NOTICE 'AddressGroup %.% marked for deletion, processing NetworkBindings',
                v_ag_namespace, v_ag_name;

            FOR binding_rec IN
                SELECT nb.namespace, nb.name,
                       nb.network_namespace, nb.network_name,
                       nb.resource_version
                FROM network_bindings nb
                WHERE nb.address_group_namespace = v_ag_namespace
                  AND nb.address_group_name = v_ag_name
            LOOP
                SELECT m.uid INTO v_binding_uid
                FROM k8s_metadata m
                WHERE m.resource_version = binding_rec.resource_version;

                SELECT n.resource_version, m.uid
                INTO v_network_rv, v_network_uid
                FROM networks n
                JOIN k8s_metadata m ON m.resource_version = n.resource_version
                WHERE n.namespace = binding_rec.network_namespace
                  AND n.name = binding_rec.network_name;

                IF v_binding_uid IS NOT NULL THEN
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
                    )
                    VALUES (
                        'NetworkBinding',
                        v_binding_uid,
                        binding_rec.namespace,
                        binding_rec.name,
                        'DELETE'::sync_operation,
                        'INTERNAL'::target_system,
                        jsonb_build_object(
                            'namespace', binding_rec.namespace,
                            'name', binding_rec.name,
                            'networkRef', jsonb_build_object(
                                'namespace', binding_rec.network_namespace,
                                'name', binding_rec.network_name
                            ),
                            'addressGroupRef', jsonb_build_object(
                                'namespace', v_ag_namespace,
                                'name', v_ag_name
                            ),
                            'affectedResources', jsonb_build_array(
                                jsonb_build_object(
                                    'resourceType', 'Network',
                                    'namespace', binding_rec.network_namespace,
                                    'name', binding_rec.network_name,
                                    'resourceVersion', v_network_rv,
                                    'uid', v_network_uid
                                )
                            )
                        ),
                        'PENDING'::outbox_status,
                        CASE
                            WHEN v_network_uid IS NOT NULL THEN
                                jsonb_build_array(
                                    jsonb_build_object(
                                        'type', 'Network',
                                        'namespace', binding_rec.network_namespace,
                                        'name', binding_rec.network_name,
                                        'uid', v_network_uid
                                    )
                                )
                            ELSE '[]'::jsonb
                        END,
                        NOW(),
                        NOW(),
                        NOW()
                    )
                    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

                    RAISE NOTICE 'NetworkBinding %.% DELETE entry created (affects Network %.%)',
                        binding_rec.namespace, binding_rec.name,
                        binding_rec.network_namespace, binding_rec.network_name;
                END IF;

                IF v_network_rv IS NOT NULL THEN
                    UPDATE networks
                    SET is_bound = false,
                        binding_ref_namespace = NULL,
                        binding_ref_name = NULL,
                        address_group_ref_namespace = NULL,
                        address_group_ref_name = NULL
                    WHERE namespace = binding_rec.network_namespace
                      AND name = binding_rec.network_name;

                    UPDATE k8s_metadata
                    SET conditions = jsonb_build_array(
                        jsonb_build_object(
                            'type', 'Ready',
                            'status', 'False',
                            'reason', 'AddressGroupDeleting',
                            'message', 'AddressGroup deleted, NetworkBinding being removed',
                            'lastTransitionTime', NOW()
                        ),
                        jsonb_build_object(
                            'type', 'PendingSync',
                            'status', 'True',
                            'reason', 'BindingCleanup',
                            'message', 'Syncing binding cleanup to SGROUP',
                            'lastTransitionTime', NOW()
                        ),
                        jsonb_build_object(
                            'type', 'Synced',
                            'status', 'False',
                            'reason', 'PendingSync',
                            'message', 'Waiting for SGROUP sync',
                            'lastTransitionTime', NOW()
                        )
                    )
                    WHERE resource_version = v_network_rv;

                    RAISE NOTICE 'Network %.% unbound: is_bound=false, Ready=False',
                        binding_rec.network_namespace, binding_rec.network_name;
                ELSE
                    RAISE WARNING 'Network %.% not found for unbinding',
                        binding_rec.network_namespace, binding_rec.network_name;
                END IF;

            END LOOP;

        ELSE
            RAISE WARNING 'AddressGroup not found for resource_version %', NEW.resource_version;
        END IF;

    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

COMMENT ON FUNCTION trigger_address_group_on_deletion_mark() IS
'Handles AddressGroup soft delete: creates DELETE outbox entries for all NetworkBindings
and updates Network.status when deletion_timestamp is set. Part of coordinated deletion
architecture ensuring SGROUP sync before physical deletion.';

CREATE TRIGGER address_group_on_deletion_mark
AFTER UPDATE OF deletion_timestamp ON k8s_metadata
FOR EACH ROW
WHEN (
    NEW.deletion_timestamp IS NOT NULL
    AND OLD.deletion_timestamp IS NULL
)
EXECUTE FUNCTION trigger_address_group_on_deletion_mark();

COMMENT ON TRIGGER address_group_on_deletion_mark ON k8s_metadata IS
'Fires when AddressGroup is soft-deleted (deletion_timestamp set).
Creates DELETE outbox entries for NetworkBindings and updates Network.status before SGROUP sync.';

-- +goose Down

-- +goose StatementBegin
DROP TRIGGER IF EXISTS address_group_on_deletion_mark ON k8s_metadata;

DROP FUNCTION IF EXISTS trigger_address_group_on_deletion_mark();
-- +goose StatementEnd
