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
            SELECT jsonb_array_elements(hosts_field) AS host_obj
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
                    'namespace', host_record.namespace,
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
'Aggregates hosts from AddressGroup.spec.hosts and HostBindings. Includes namespace for binding refs and filters soft-deleted bindings (deletion_timestamp IS NULL).';

UPDATE address_groups
SET aggregated_hosts = aggregate_address_group_hosts(namespace::text, name::text);

-- +goose StatementEnd

-- +goose Down
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
            SELECT jsonb_array_elements(hosts_field) AS host_obj
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
'Aggregates hosts from AddressGroup.spec.hosts and HostBindings, filtering soft-deleted bindings.';

UPDATE address_groups
SET aggregated_hosts = aggregate_address_group_hosts(namespace::text, name::text);

-- +goose StatementEnd

