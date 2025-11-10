-- +goose Up

-- +goose StatementBegin

CREATE OR REPLACE FUNCTION aggregate_service_address_groups(svc_namespace text, svc_name text)
RETURNS jsonb
LANGUAGE plpgsql
AS $$
DECLARE
    aggregated_ags_json JSONB := '[]'::jsonb;
    ag_ref JSONB;
    ag_record RECORD;
    address_groups_field JSONB;
BEGIN
    SELECT COALESCE(address_groups, '[]'::jsonb) INTO address_groups_field
    FROM services
    WHERE namespace = svc_namespace AND name = svc_name;

    IF address_groups_field IS NOT NULL AND address_groups_field != 'null'::jsonb
       AND jsonb_typeof(address_groups_field) = 'array' AND jsonb_array_length(address_groups_field) > 0 THEN
        FOR ag_ref IN
            SELECT jsonb_array_elements(address_groups_field) as ag_obj
        LOOP
            aggregated_ags_json := aggregated_ags_json || jsonb_build_array(
                jsonb_build_object(
                    'ref', ag_ref,
                    'source', 'spec'
                )
            );
        END LOOP;
    END IF;

    FOR ag_record IN
        SELECT ag.namespace, ag.name
        FROM address_group_bindings agb
        JOIN address_groups ag ON ag.namespace = agb.address_group_namespace
                              AND ag.name = agb.address_group_name
        LEFT JOIN k8s_metadata m ON m.resource_version = agb.resource_version
        WHERE agb.service_namespace = svc_namespace::namespace_name
        AND agb.service_name = svc_name::resource_name
        AND (m.deletion_timestamp IS NULL OR m.resource_version IS NULL)
    LOOP
        aggregated_ags_json := aggregated_ags_json || jsonb_build_array(
            jsonb_build_object(
                'ref', jsonb_build_object(
                    'apiVersion', 'netguard.sgroups.io/v1beta1',
                    'kind', 'AddressGroup',
                    'name', ag_record.name,
                    'namespace', ag_record.namespace
                ),
                'source', 'binding'
            )
        );
    END LOOP;

    RETURN aggregated_ags_json;
END;
$$;

COMMENT ON FUNCTION aggregate_service_address_groups(text, text) IS
'Aggregates AddressGroups for a Service from spec.address_groups and AddressGroupBindings.
Migration 082 fix: Uses LEFT JOIN with k8s_metadata to include bindings during INSERT (before k8s_metadata created).
Filters soft-deleted bindings when k8s_metadata exists with deletion_timestamp.';


DO $$
BEGIN
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '[Migration 082] aggregate_service_address_groups() INSERT fix COMPLETE';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '';
    RAISE NOTICE 'FIXED: Changed INNER JOIN to LEFT JOIN with k8s_metadata';
    RAISE NOTICE '';
    RAISE NOTICE 'Behavior:';
    RAISE NOTICE '  - During INSERT: k8s_metadata not yet created → LEFT JOIN returns NULL → binding included ✓';
    RAISE NOTICE '  - During DELETE: k8s_metadata exists with deletion_timestamp → binding filtered out ✓';
    RAISE NOTICE '  - Normal operation: k8s_metadata exists → soft-deleted bindings filtered correctly ✓';
    RAISE NOTICE '';
    RAISE NOTICE 'Result: Service UPDATE outbox entry will be created when binding is created/deleted!';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION aggregate_service_address_groups(svc_namespace text, svc_name text)
RETURNS jsonb
LANGUAGE plpgsql
AS $$
DECLARE
    aggregated_ags_json JSONB := '[]'::jsonb;
    ag_ref JSONB;
    ag_record RECORD;
    address_groups_field JSONB;
BEGIN
    SELECT COALESCE(address_groups, '[]'::jsonb) INTO address_groups_field
    FROM services
    WHERE namespace = svc_namespace AND name = svc_name;

    IF address_groups_field IS NOT NULL AND address_groups_field != 'null'::jsonb
       AND jsonb_typeof(address_groups_field) = 'array' AND jsonb_array_length(address_groups_field) > 0 THEN
        FOR ag_ref IN
            SELECT jsonb_array_elements(address_groups_field) as ag_obj
        LOOP
            aggregated_ags_json := aggregated_ags_json || jsonb_build_array(
                jsonb_build_object(
                    'ref', ag_ref,
                    'source', 'spec'
                )
            );
        END LOOP;
    END IF;

    FOR ag_record IN
        SELECT ag.namespace, ag.name
        FROM address_group_bindings agb
        JOIN address_groups ag ON ag.namespace = agb.address_group_namespace
                              AND ag.name = agb.address_group_name
        JOIN k8s_metadata m ON m.resource_version = agb.resource_version
        WHERE agb.service_namespace = svc_namespace::namespace_name
        AND agb.service_name = svc_name::resource_name
        AND m.deletion_timestamp IS NULL
    LOOP
        aggregated_ags_json := aggregated_ags_json || jsonb_build_array(
            jsonb_build_object(
                'ref', jsonb_build_object(
                    'apiVersion', 'netguard.sgroups.io/v1beta1',
                    'kind', 'AddressGroup',
                    'name', ag_record.name,
                    'namespace', ag_record.namespace
                ),
                'source', 'binding'
            )
        );
    END LOOP;

    RETURN aggregated_ags_json;
END;
$$;

DO $$
BEGIN
    RAISE WARNING '[Migration 082 Rollback] Reverted - INSERT binding bug will return!';
END $$;

-- +goose StatementEnd
