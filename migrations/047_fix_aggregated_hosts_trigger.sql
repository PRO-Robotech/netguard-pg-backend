-- +goose Up

-- +goose StatementBegin
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
    UPDATE address_groups
    SET aggregated_hosts = aggregate_address_group_hosts(NEW.namespace::text, NEW.name::text)
    WHERE namespace = NEW.namespace AND name = NEW.name;

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
'Updates aggregated_hosts field after AddressGroup spec.hosts changes.
Also updates Host status (isBound, addressGroupRef, conditions) when hosts are added/removed.
FIXED in Migration 047: Now uses separate UPDATE statement instead of NEW assignment (works in AFTER triggers).';

UPDATE address_groups
SET aggregated_hosts = aggregate_address_group_hosts(namespace::text, name::text);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

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
'Updates aggregated_hosts field after AddressGroup spec.hosts changes (Migration 044 version with bug).';

-- +goose StatementEnd
