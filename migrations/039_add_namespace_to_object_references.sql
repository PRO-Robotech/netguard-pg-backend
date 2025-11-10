-- +goose Up
-- +goose StatementBegin

UPDATE address_groups
SET hosts = (
    SELECT jsonb_agg(
        CASE
            WHEN jsonb_typeof(host_item) = 'object' THEN
                host_item || jsonb_build_object('namespace', namespace)
            ELSE
                host_item
        END
    )
    FROM jsonb_array_elements(hosts) AS host_item
)
WHERE hosts != '[]'::jsonb
  AND hosts IS NOT NULL
  AND jsonb_typeof(hosts) = 'array';

UPDATE address_groups
SET aggregated_hosts = (
    SELECT jsonb_agg(
        CASE
            WHEN jsonb_typeof(host_ref) = 'object' AND host_ref ? 'ref' THEN
                jsonb_set(
                    host_ref,
                    '{ref,namespace}',
                    to_jsonb(namespace::text)
                )
            ELSE
                host_ref
        END
    )
    FROM jsonb_array_elements(aggregated_hosts) AS host_ref
)
WHERE aggregated_hosts != '[]'::jsonb
  AND aggregated_hosts IS NOT NULL
  AND jsonb_typeof(aggregated_hosts) = 'array';

CREATE OR REPLACE FUNCTION aggregate_address_group_hosts(ag_namespace TEXT, ag_name TEXT) RETURNS JSONB AS $$
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
                    'name', host_record.name,
                    'namespace', host_record.namespace
                ),
                'uuid', host_record.uuid,
                'source', 'binding'
            )
        );
    END LOOP;

    RETURN aggregated_hosts_json;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION check_host_exclusivity() RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM address_groups
        WHERE (namespace, name) != (NEW.namespace, NEW.name)
        AND EXISTS (
            SELECT 1
            FROM jsonb_array_elements(NEW.hosts) AS new_host
            WHERE EXISTS (
                SELECT 1
                FROM jsonb_array_elements(hosts) AS existing_host
                WHERE existing_host->>'name' = new_host->>'name'
                AND existing_host->>'namespace' = new_host->>'namespace'
            )
        )
    ) THEN
        RAISE EXCEPTION 'Host already belongs to another AddressGroup';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION cascade_host_deletion() RETURNS TRIGGER AS $$
DECLARE
    host_obj jsonb;
BEGIN
    host_obj := jsonb_build_object(
        'name', OLD.name,
        'namespace', OLD.namespace,
        'apiVersion', 'netguard.sgroups.io/v1beta1',
        'kind', 'Host'
    );

    UPDATE address_groups
    SET hosts = (
        SELECT jsonb_agg(host_item)
        FROM jsonb_array_elements(hosts) AS host_item
        WHERE host_item->>'name' != OLD.name
           OR host_item->>'namespace' != OLD.namespace
    )
    WHERE hosts @> jsonb_build_array(host_obj)
       OR EXISTS (
           SELECT 1
           FROM jsonb_array_elements(hosts) AS h
           WHERE h->>'name' = OLD.name
             AND h->>'namespace' = OLD.namespace
       );

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION validate_host_binding_conflicts() RETURNS TRIGGER AS $$
DECLARE
    conflicting_ag RECORD;
    host_in_spec BOOLEAN := false;
BEGIN
    SELECT EXISTS(
        SELECT 1 FROM address_groups ag
        WHERE EXISTS (
            SELECT 1
            FROM jsonb_array_elements(ag.hosts) AS host_item
            WHERE host_item->>'name' = NEW.host_name
            AND host_item->>'namespace' = NEW.host_namespace
        )
    ) INTO host_in_spec;

    IF host_in_spec THEN
        SELECT ag.namespace, ag.name INTO conflicting_ag
        FROM address_groups ag
        WHERE EXISTS (
            SELECT 1
            FROM jsonb_array_elements(ag.hosts) AS host_item
            WHERE host_item->>'name' = NEW.host_name
            AND host_item->>'namespace' = NEW.host_namespace
        )
        LIMIT 1;

        RAISE EXCEPTION 'Host %.% already in AddressGroup %.% spec.hosts',
            NEW.host_namespace, NEW.host_name, conflicting_ag.namespace, conflicting_ag.name;
    END IF;

    SELECT ag.namespace, ag.name INTO conflicting_ag
    FROM host_bindings hb
    JOIN address_groups ag ON ag.namespace = hb.address_group_namespace AND ag.name = hb.address_group_name
    WHERE hb.host_namespace = NEW.host_namespace
    AND hb.host_name = NEW.host_name
    AND (hb.address_group_namespace != NEW.address_group_namespace OR hb.address_group_name != NEW.address_group_name)
    LIMIT 1;

    IF FOUND THEN
        RAISE EXCEPTION 'Host %.% already in AddressGroup %.% via HostBinding',
            NEW.host_namespace, NEW.host_name, conflicting_ag.namespace, conflicting_ag.name;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

UPDATE address_groups
SET aggregated_hosts = aggregate_address_group_hosts(namespace, name);

COMMENT ON COLUMN address_groups.hosts IS 'NamespacedObjectReference[] - hosts list (namespace must match AddressGroup)';
COMMENT ON COLUMN address_groups.aggregated_hosts IS 'HostReference[] - aggregated hosts from spec.hosts and HostBindings';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

UPDATE address_groups
SET hosts = (
    SELECT jsonb_agg(
        CASE
            WHEN jsonb_typeof(host_item) = 'object' THEN
                host_item - 'namespace'
            ELSE
                host_item
        END
    )
    FROM jsonb_array_elements(hosts) AS host_item
)
WHERE hosts != '[]'::jsonb
  AND hosts IS NOT NULL
  AND jsonb_typeof(hosts) = 'array';

UPDATE address_groups
SET aggregated_hosts = (
    SELECT jsonb_agg(
        CASE
            WHEN jsonb_typeof(host_ref) = 'object' AND host_ref ? 'ref' THEN
                jsonb_set(
                    host_ref,
                    '{ref}',
                    (host_ref->'ref') - 'namespace'
                )
            ELSE
                host_ref
        END
    )
    FROM jsonb_array_elements(aggregated_hosts) AS host_ref
)
WHERE aggregated_hosts != '[]'::jsonb
  AND aggregated_hosts IS NOT NULL
  AND jsonb_typeof(aggregated_hosts) = 'array';

CREATE OR REPLACE FUNCTION aggregate_address_group_hosts(ag_namespace TEXT, ag_name TEXT) RETURNS JSONB AS $$
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

CREATE OR REPLACE FUNCTION check_host_exclusivity() RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM address_groups
        WHERE (namespace, name) != (NEW.namespace, NEW.name)
        AND hosts ?| ARRAY(
            SELECT jsonb_array_elements_text(
                jsonb_path_query_array(NEW.hosts, '$[*].name')
            )
        )
        AND hosts ?| ARRAY(
            SELECT jsonb_array_elements_text(
                jsonb_path_query_array(NEW.hosts, '$[*].namespace')
            )
        )
    ) THEN
        RAISE EXCEPTION 'Host already belongs to another AddressGroup';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION cascade_host_deletion() RETURNS TRIGGER AS $$
DECLARE
    host_obj jsonb;
BEGIN
    host_obj := jsonb_build_object(
        'name', OLD.name,
        'namespace', OLD.namespace,
        'apiVersion', 'netguard.sgroups.io/v1beta1',
        'kind', 'Host'
    );

    UPDATE address_groups
    SET hosts = hosts - host_obj
    WHERE hosts @> jsonb_build_array(host_obj);

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION validate_host_binding_conflicts() RETURNS TRIGGER AS $$
DECLARE
    conflicting_ag RECORD;
    host_in_spec BOOLEAN := false;
BEGIN
    SELECT EXISTS(
        SELECT 1 FROM address_groups ag
        WHERE ag.hosts @> jsonb_build_array(
            jsonb_build_object(
                'apiVersion', 'netguard.sgroups.io/v1beta1',
                'kind', 'Host',
                'name', NEW.host_name
            )
        )
    ) INTO host_in_spec;

    IF host_in_spec THEN
        SELECT ag.namespace, ag.name INTO conflicting_ag
        FROM address_groups ag
        WHERE ag.hosts @> jsonb_build_array(
            jsonb_build_object(
                'apiVersion', 'netguard.sgroups.io/v1beta1',
                'kind', 'Host',
                'name', NEW.host_name
            )
        )
        LIMIT 1;

        RAISE EXCEPTION 'Host %.% already in AddressGroup %.% spec.hosts',
            NEW.host_namespace, NEW.host_name, conflicting_ag.namespace, conflicting_ag.name;
    END IF;

    SELECT ag.namespace, ag.name INTO conflicting_ag
    FROM host_bindings hb
    JOIN address_groups ag ON ag.namespace = hb.address_group_namespace AND ag.name = hb.address_group_name
    WHERE hb.host_namespace = NEW.host_namespace
    AND hb.host_name = NEW.host_name
    AND (hb.address_group_namespace != NEW.address_group_namespace OR hb.address_group_name != NEW.address_group_name)
    LIMIT 1;

    IF FOUND THEN
        RAISE EXCEPTION 'Host %.% already in AddressGroup %.% via HostBinding',
            NEW.host_namespace, NEW.host_name, conflicting_ag.namespace, conflicting_ag.name;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

UPDATE address_groups
SET aggregated_hosts = aggregate_address_group_hosts(namespace, name);

COMMENT ON COLUMN address_groups.hosts IS 'ObjectReference[] - hosts list';
COMMENT ON COLUMN address_groups.aggregated_hosts IS 'HostReference[] - aggregated hosts from spec.hosts and HostBindings';

-- +goose StatementEnd
