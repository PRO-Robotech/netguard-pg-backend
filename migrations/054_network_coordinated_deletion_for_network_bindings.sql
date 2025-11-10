-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_network_on_deletion_mark()
RETURNS TRIGGER AS $$
DECLARE
    v_network_namespace namespace_name;
    v_network_name resource_name;
    v_network_cidr CIDR;
    binding_rec RECORD;
    v_binding_uid UUID;
    v_ag_uid UUID;
    v_ag_rv BIGINT;
BEGIN
    IF NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL THEN

        SELECT namespace, name, cidr
        INTO v_network_namespace, v_network_name, v_network_cidr
        FROM networks
        WHERE resource_version = NEW.resource_version;

        IF FOUND THEN
            RAISE NOTICE 'Network %.% (CIDR: %) marked for deletion, processing NetworkBindings',
                v_network_namespace, v_network_name, v_network_cidr;

            FOR binding_rec IN
                SELECT nb.namespace, nb.name,
                       nb.address_group_namespace, nb.address_group_name,
                       nb.resource_version
                FROM network_bindings nb
                WHERE nb.network_namespace = v_network_namespace
                  AND nb.network_name = v_network_name
            LOOP
                SELECT m.uid INTO v_binding_uid
                FROM k8s_metadata m
                WHERE m.resource_version = binding_rec.resource_version;

                SELECT ag.resource_version, m.uid
                INTO v_ag_rv, v_ag_uid
                FROM address_groups ag
                JOIN k8s_metadata m ON m.resource_version = ag.resource_version
                WHERE ag.namespace = binding_rec.address_group_namespace
                  AND ag.name = binding_rec.address_group_name;

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
                                'namespace', v_network_namespace,
                                'name', v_network_name
                            ),
                            'addressGroupRef', jsonb_build_object(
                                'namespace', binding_rec.address_group_namespace,
                                'name', binding_rec.address_group_name
                            ),
                            'affectedResources', jsonb_build_array(
                                jsonb_build_object(
                                    'resourceType', 'AddressGroup',
                                    'namespace', binding_rec.address_group_namespace,
                                    'name', binding_rec.address_group_name,
                                    'resourceVersion', v_ag_rv,
                                    'uid', v_ag_uid
                                )
                            )
                        ),
                        'PENDING'::outbox_status,
                        CASE
                            WHEN v_ag_uid IS NOT NULL THEN
                                jsonb_build_array(
                                    jsonb_build_object(
                                        'type', 'AddressGroup',
                                        'namespace', binding_rec.address_group_namespace,
                                        'name', binding_rec.address_group_name,
                                        'uid', v_ag_uid
                                    )
                                )
                            ELSE '[]'::jsonb
                        END,
                        NOW(),
                        NOW(),
                        NOW()
                    )
                    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

                    RAISE NOTICE 'NetworkBinding %.% DELETE entry created (affects AddressGroup %.%)',
                        binding_rec.namespace, binding_rec.name,
                        binding_rec.address_group_namespace, binding_rec.address_group_name;
                END IF;

                IF v_ag_rv IS NOT NULL THEN
                    PERFORM rebuild_address_group_networks(
                        binding_rec.address_group_namespace,
                        binding_rec.address_group_name
                    );

                    DECLARE
                        v_networks_count INT;
                    BEGIN
                        SELECT jsonb_array_length(networks)
                        INTO v_networks_count
                        FROM address_groups
                        WHERE namespace = binding_rec.address_group_namespace
                          AND name = binding_rec.address_group_name;

                        IF v_networks_count = 0 THEN
                            UPDATE k8s_metadata
                            SET conditions = jsonb_build_array(
                                jsonb_build_object(
                                    'type', 'Ready',
                                    'status', 'False',
                                    'reason', 'NoNetworks',
                                    'message', 'Network deleted, AddressGroup has no networks',
                                    'lastTransitionTime', NOW()
                                ),
                                jsonb_build_object(
                                    'type', 'PendingSync',
                                    'status', 'True',
                                    'reason', 'NetworkCleanup',
                                    'message', 'Syncing network cleanup to SGROUP',
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
                            WHERE resource_version = v_ag_rv;

                            RAISE NOTICE 'AddressGroup %.% updated: networks=[], Ready=False',
                                binding_rec.address_group_namespace, binding_rec.address_group_name;
                        ELSE
                            RAISE NOTICE 'AddressGroup %.% updated: networks array rebuilt (count: %)',
                                binding_rec.address_group_namespace, binding_rec.address_group_name,
                                v_networks_count;
                        END IF;
                    END;
                ELSE
                    RAISE WARNING 'AddressGroup %.% not found for networks rebuild',
                        binding_rec.address_group_namespace, binding_rec.address_group_name;
                END IF;

            END LOOP;

        ELSE
            RAISE WARNING 'Network not found for resource_version %', NEW.resource_version;
        END IF;

    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

COMMENT ON FUNCTION trigger_network_on_deletion_mark() IS
'Handles Network soft delete: creates DELETE outbox entries for all NetworkBindings
and rebuilds AddressGroup.networks when deletion_timestamp is set. Part of coordinated deletion
architecture ensuring SGROUP sync before physical deletion.';

CREATE TRIGGER network_on_deletion_mark
AFTER UPDATE OF deletion_timestamp ON k8s_metadata
FOR EACH ROW
WHEN (
    NEW.deletion_timestamp IS NOT NULL
    AND OLD.deletion_timestamp IS NULL
)
EXECUTE FUNCTION trigger_network_on_deletion_mark();

COMMENT ON TRIGGER network_on_deletion_mark ON k8s_metadata IS
'Fires when Network is soft-deleted (deletion_timestamp set).
Creates DELETE outbox entries for NetworkBindings and rebuilds AddressGroup.networks before SGROUP sync.';

-- +goose Down

-- +goose StatementBegin
DROP TRIGGER IF EXISTS network_on_deletion_mark ON k8s_metadata;

DROP FUNCTION IF EXISTS trigger_network_on_deletion_mark();
-- +goose StatementEnd
