-- +goose Up

-- +goose StatementBegin


CREATE OR REPLACE FUNCTION aggregate_address_group_hosts(ag_namespace TEXT, ag_name TEXT)
RETURNS JSONB AS $$
DECLARE
    aggregated_hosts_json JSONB := '[]'::jsonb;
    host_ref JSONB;
    host_record RECORD;
    hosts_field JSONB;
BEGIN
    SELECT COALESCE(hosts, '[]'::jsonb) INTO hosts_field
    FROM address_groups
    WHERE namespace = ag_namespace AND name = ag_name;

    IF hosts_field IS NOT NULL AND hosts_field != 'null'::jsonb
       AND jsonb_typeof(hosts_field) = 'array' AND jsonb_array_length(hosts_field) > 0 THEN
        FOR host_ref IN
            SELECT jsonb_array_elements(hosts_field) as host_obj
        LOOP
            SELECT h.uuid INTO host_record
            FROM hosts h
            WHERE h.namespace = ag_namespace::namespace_name
            AND h.name = (host_ref->>'name')::resource_name;

            aggregated_hosts_json := aggregated_hosts_json || jsonb_build_array(
                jsonb_build_object(
                    'ref', host_ref,
                    'uuid', COALESCE(host_record.uuid, ''),
                    'source', 'spec'
                )
            );
        END LOOP;
    END IF;

    FOR host_record IN
        SELECT h.namespace, h.name, h.uuid
        FROM host_bindings hb
        JOIN hosts h ON h.namespace = hb.host_namespace AND h.name = hb.host_name
        JOIN k8s_metadata m ON m.resource_version = hb.resource_version
        WHERE hb.address_group_namespace = ag_namespace::namespace_name
        AND hb.address_group_name = ag_name::resource_name
        AND m.deletion_timestamp IS NULL
    LOOP
        aggregated_hosts_json := aggregated_hosts_json || jsonb_build_array(
            jsonb_build_object(
                'ref', jsonb_build_object(
                    'apiVersion', 'netguard.sgroups.io/v1beta1',
                    'kind', 'Host',
                    'name', host_record.name
                ),
                'uuid', host_record.uuid,
                'source', 'binding'
            )
        );
    END LOOP;

    RETURN aggregated_hosts_json;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION aggregate_address_group_hosts(TEXT, TEXT) IS
'Aggregates hosts from both AG.spec.hosts and HostBindings.
FILTERS OUT HostBindings with deletion_timestamp != NULL to support coordinated deletion.
See: docs/architecture/COORDINATED_BINDING_DELETION.md';


CREATE OR REPLACE FUNCTION trigger_host_binding_on_deletion_mark()
RETURNS TRIGGER AS $$
DECLARE
    v_host_namespace namespace_name;
    v_host_name resource_name;
    v_ag_namespace namespace_name;
    v_ag_name resource_name;
    v_host_rv BIGINT;
    v_ag_rv BIGINT;
    v_binding_record RECORD;
    v_uid UUID;
    v_binding_namespace namespace_name;
    v_binding_name resource_name;
    v_affected_resources JSONB;
    v_binding_found BOOLEAN := false;
BEGIN
    IF NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL THEN

        v_uid := NEW.uid;

        SELECT host_namespace, host_name, address_group_namespace, address_group_name,
               namespace, name
        INTO v_host_namespace, v_host_name, v_ag_namespace, v_ag_name,
             v_binding_namespace, v_binding_name
        FROM host_bindings
        WHERE resource_version = NEW.resource_version;

        v_binding_found := FOUND;


        IF v_binding_found THEN
            RAISE NOTICE 'HostBinding deletion marked: host=%.%, ag=%.%',
                v_host_namespace, v_host_name, v_ag_namespace, v_ag_name;

            SELECT resource_version INTO v_host_rv
            FROM hosts
            WHERE namespace = v_host_namespace AND name = v_host_name;

            SELECT resource_version INTO v_ag_rv
            FROM address_groups
            WHERE namespace = v_ag_namespace AND name = v_ag_name;

            v_affected_resources := jsonb_build_array(
                jsonb_build_object(
                    'resourceType', 'Host',
                    'namespace', v_host_namespace,
                    'name', v_host_name,
                    'resourceVersion', v_host_rv
                ),
                jsonb_build_object(
                    'resourceType', 'AddressGroup',
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
                attempts,
                max_retries,
                created_at,
                updated_at,
                next_retry_at
            )
            VALUES (
                'HostBinding',
                v_uid,
                v_binding_namespace,
                v_binding_name,
                'DELETE'::sync_operation,
                'INTERNAL'::target_system,
                jsonb_build_object(
                    'namespace', v_binding_namespace,
                    'name', v_binding_name,
                    'hostRef', jsonb_build_object(
                        'namespace', v_host_namespace,
                        'name', v_host_name
                    ),
                    'addressGroupRef', jsonb_build_object(
                        'namespace', v_ag_namespace,
                        'name', v_ag_name
                    ),
                    'affectedResources', v_affected_resources
                ),
                'PENDING'::outbox_status,
                0,
                5,
                NOW(),
                NOW(),
                NOW()
            )
            ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

            RAISE NOTICE 'HostBinding %.% DELETE entry created in sync_outbox (target_system=INTERNAL)',
                v_binding_namespace, v_binding_name;

            IF v_host_rv IS NOT NULL THEN
                UPDATE hosts
                SET is_bound = false,
                    address_group_name = NULL,
                    address_group_ref_namespace = NULL,
                    address_group_ref_name = NULL
                WHERE namespace = v_host_namespace AND name = v_host_name;

                UPDATE k8s_metadata
                SET conditions = jsonb_build_array(
                    jsonb_build_object(
                        'type', 'Ready',
                        'status', 'False',
                        'reason', 'BindingDeleting',
                        'message', 'HostBinding deleted, waiting for SGROUP sync',
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
                WHERE resource_version = v_host_rv;

                RAISE NOTICE 'Host %.% updated: isBound=false, Ready=False', v_host_namespace, v_host_name;
            ELSE
                RAISE WARNING 'Host %.% not found for binding deletion', v_host_namespace, v_host_name;
            END IF;

            IF v_ag_rv IS NOT NULL THEN
                UPDATE address_groups
                SET aggregated_hosts = aggregate_address_group_hosts(v_ag_namespace::text, v_ag_name::text)
                WHERE namespace = v_ag_namespace AND name = v_ag_name;

                UPDATE k8s_metadata
                SET conditions = jsonb_build_array(
                    jsonb_build_object(
                        'type', 'Ready',
                        'status', 'False',
                        'reason', 'BindingDeleting',
                        'message', 'Host binding removed, waiting for SGROUP sync',
                        'lastTransitionTime', NOW()
                    ),
                    jsonb_build_object(
                        'type', 'PendingSync',
                        'status', 'True',
                        'reason', 'AggregationUpdate',
                        'message', 'Syncing aggregation update to SGROUP',
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

                RAISE NOTICE 'AddressGroup %.% updated: aggregated_hosts recalculated, Ready=False',
                    v_ag_namespace, v_ag_name;
            ELSE
                RAISE WARNING 'AddressGroup %.% not found for binding deletion', v_ag_namespace, v_ag_name;
            END IF;

        ELSE
            RAISE WARNING 'HostBinding not found for resource_version %, creating minimal DELETE entry', NEW.resource_version;

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
                'HostBinding',
                v_uid,
                'unknown',
                'unknown',
                'DELETE'::sync_operation,
                'INTERNAL'::target_system,
                jsonb_build_object(
                    'resourceVersion', NEW.resource_version,
                    'note', 'Minimal DELETE entry - HostBinding already removed from table (race condition)'
                ),
                'PENDING'::outbox_status,
                0,
                5,
                NOW(),
                NOW(),
                NOW()
            )
            ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

            RAISE NOTICE 'Minimal HostBinding DELETE entry created for resource_version % (uid=%)', NEW.resource_version, v_uid;
        END IF;

    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_host_binding_on_deletion_mark() IS
'Handles HostBinding soft delete: updates Host and AddressGroup when deletion_timestamp is set.
Part of coordinated deletion architecture ensuring SGROUP sync before physical deletion.
See: docs/architecture/COORDINATED_BINDING_DELETION.md';

CREATE TRIGGER host_binding_on_deletion_mark
AFTER UPDATE OF deletion_timestamp ON k8s_metadata
FOR EACH ROW
WHEN (
    NEW.deletion_timestamp IS NOT NULL
    AND OLD.deletion_timestamp IS NULL
)
EXECUTE FUNCTION trigger_host_binding_on_deletion_mark();

COMMENT ON TRIGGER host_binding_on_deletion_mark ON k8s_metadata IS
'Fires when HostBinding is soft-deleted (deletion_timestamp set).
Updates Host and AddressGroup to reflect binding removal before SGROUP sync.';


CREATE OR REPLACE FUNCTION trigger_update_aggregated_hosts_on_spec_change()
RETURNS TRIGGER AS $$
DECLARE
    old_hosts_json JSONB;
    new_hosts_json JSONB;
    host_name_val TEXT;
    added_hosts JSONB;
    removed_hosts JSONB;
    v_host_rv BIGINT;
BEGIN
    NEW.aggregated_hosts := aggregate_address_group_hosts(NEW.namespace::text, NEW.name::text);

    old_hosts_json := COALESCE(OLD.hosts, '[]'::jsonb);
    new_hosts_json := COALESCE(NEW.hosts, '[]'::jsonb);

    IF old_hosts_json <> new_hosts_json THEN

        RAISE NOTICE 'AG %.% spec.hosts changed, updating affected Hosts', NEW.namespace, NEW.name;

        SELECT jsonb_agg(elem)
        INTO added_hosts
        FROM jsonb_array_elements(new_hosts_json) elem
        WHERE NOT EXISTS (
            SELECT 1 FROM jsonb_array_elements(old_hosts_json) old_elem
            WHERE old_elem->>'name' = elem->>'name'
        );

        IF added_hosts IS NOT NULL AND jsonb_array_length(added_hosts) > 0 THEN
            RAISE NOTICE 'Found % added hosts to AG.spec.hosts', jsonb_array_length(added_hosts);

            FOR host_name_val IN
                SELECT elem->>'name'
                FROM jsonb_array_elements(added_hosts) elem
            LOOP
                SELECT resource_version INTO v_host_rv
                FROM hosts
                WHERE namespace = NEW.namespace AND name = host_name_val::resource_name;

                IF v_host_rv IS NOT NULL THEN
                    UPDATE hosts
                    SET is_bound = true,
                        address_group_name = NEW.name,
                        address_group_ref_namespace = NEW.namespace,
                        address_group_ref_name = NEW.name
                    WHERE namespace = NEW.namespace AND name = host_name_val::resource_name;

                    UPDATE k8s_metadata
                    SET conditions = jsonb_build_array(
                        jsonb_build_object(
                            'type', 'Ready',
                            'status', 'False',
                            'reason', 'AddedToAddressGroup',
                            'message', 'Added to AddressGroup via spec.hosts, waiting for SGROUP sync',
                            'lastTransitionTime', NOW()
                        ),
                        jsonb_build_object(
                            'type', 'PendingSync',
                            'status', 'True',
                            'reason', 'SpecUpdate',
                            'message', 'Syncing to SGROUP',
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
                    WHERE resource_version = v_host_rv;

                    RAISE NOTICE 'Host %.% added to AG: isBound=true, Ready=False',
                        NEW.namespace, host_name_val;
                ELSE
                    RAISE WARNING 'Host %.% not found when adding to AG.spec.hosts',
                        NEW.namespace, host_name_val;
                END IF;
            END LOOP;
        END IF;

        SELECT jsonb_agg(elem)
        INTO removed_hosts
        FROM jsonb_array_elements(old_hosts_json) elem
        WHERE NOT EXISTS (
            SELECT 1 FROM jsonb_array_elements(new_hosts_json) new_elem
            WHERE new_elem->>'name' = elem->>'name'
        );

        IF removed_hosts IS NOT NULL AND jsonb_array_length(removed_hosts) > 0 THEN
            RAISE NOTICE 'Found % removed hosts from AG.spec.hosts', jsonb_array_length(removed_hosts);

            FOR host_name_val IN
                SELECT elem->>'name'
                FROM jsonb_array_elements(removed_hosts) elem
            LOOP
                SELECT resource_version INTO v_host_rv
                FROM hosts
                WHERE namespace = NEW.namespace AND name = host_name_val::resource_name;

                IF v_host_rv IS NOT NULL THEN
                    UPDATE hosts
                    SET is_bound = false,
                        address_group_name = NULL,
                        address_group_ref_namespace = NULL,
                        address_group_ref_name = NULL
                    WHERE namespace = NEW.namespace AND name = host_name_val::resource_name;

                    UPDATE k8s_metadata
                    SET conditions = jsonb_build_array(
                        jsonb_build_object(
                            'type', 'Ready',
                            'status', 'False',
                            'reason', 'RemovedFromAddressGroup',
                            'message', 'Removed from AddressGroup via spec.hosts, waiting for SGROUP sync',
                            'lastTransitionTime', NOW()
                        ),
                        jsonb_build_object(
                            'type', 'PendingSync',
                            'status', 'True',
                            'reason', 'SpecUpdate',
                            'message', 'Syncing to SGROUP',
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
                    WHERE resource_version = v_host_rv;

                    RAISE NOTICE 'Host %.% removed from AG: isBound=false, Ready=False',
                        NEW.namespace, host_name_val;
                ELSE
                    RAISE WARNING 'Host %.% not found when removing from AG.spec.hosts',
                        NEW.namespace, host_name_val;
                END IF;
            END LOOP;
        END IF;

    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_update_aggregated_hosts_on_spec_change() IS
'Updates AG.aggregated_hosts when spec.hosts changes.
ALSO updates affected Host.status (isBound, addressGroupRef, conditions) for Case 3.
See: docs/architecture/COORDINATED_BINDING_DELETION.md';

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

DROP TRIGGER IF EXISTS host_binding_on_deletion_mark ON k8s_metadata;
DROP FUNCTION IF EXISTS trigger_host_binding_on_deletion_mark();

CREATE OR REPLACE FUNCTION aggregate_address_group_hosts(ag_namespace TEXT, ag_name TEXT)
RETURNS JSONB AS $$
DECLARE
    aggregated_hosts_json JSONB := '[]'::jsonb;
    host_ref JSONB;
    host_record RECORD;
    hosts_field JSONB;
BEGIN
    SELECT COALESCE(hosts, '[]'::jsonb) INTO hosts_field
    FROM address_groups
    WHERE namespace = ag_namespace AND name = ag_name;

    IF hosts_field IS NOT NULL AND hosts_field != 'null'::jsonb
       AND jsonb_typeof(hosts_field) = 'array' AND jsonb_array_length(hosts_field) > 0 THEN
        FOR host_ref IN
            SELECT jsonb_array_elements(hosts_field) as host_obj
        LOOP
            SELECT h.uuid INTO host_record
            FROM hosts h
            WHERE h.namespace = ag_namespace::namespace_name
            AND h.name = (host_ref->>'name')::resource_name;

            aggregated_hosts_json := aggregated_hosts_json || jsonb_build_array(
                jsonb_build_object(
                    'ref', host_ref,
                    'uuid', COALESCE(host_record.uuid, ''),
                    'source', 'spec'
                )
            );
        END LOOP;
    END IF;

    FOR host_record IN
        SELECT h.namespace, h.name, h.uuid
        FROM host_bindings hb
        JOIN hosts h ON h.namespace = hb.host_namespace AND h.name = hb.host_name
        WHERE hb.address_group_namespace = ag_namespace::namespace_name
        AND hb.address_group_name = ag_name::resource_name
    LOOP
        aggregated_hosts_json := aggregated_hosts_json || jsonb_build_array(
            jsonb_build_object(
                'ref', jsonb_build_object(
                    'apiVersion', 'netguard.sgroups.io/v1beta1',
                    'kind', 'Host',
                    'name', host_record.name
                ),
                'uuid', host_record.uuid,
                'source', 'binding'
            )
        );
    END LOOP;

    RETURN aggregated_hosts_json;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION trigger_update_aggregated_hosts_on_spec_change()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE address_groups
    SET aggregated_hosts = aggregate_address_group_hosts(NEW.namespace, NEW.name)
    WHERE namespace = NEW.namespace AND name = NEW.name;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd
