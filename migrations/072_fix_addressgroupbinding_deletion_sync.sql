-- +goose Up

-- +goose StatementBegin


CREATE OR REPLACE FUNCTION aggregate_service_address_groups(
    svc_namespace TEXT,
    svc_name TEXT
) RETURNS JSONB AS $$
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
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION aggregate_service_address_groups(TEXT, TEXT) IS
'Aggregates AddressGroups for a Service from TWO sources:
1. spec.addressGroups (source=spec)
2. AddressGroupBindings (source=binding)
NOW FILTERS deleted bindings (Migration 072 fix) - only includes bindings with deletion_timestamp IS NULL.
Similar to aggregate_address_group_hosts() in Migration 044.';


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
            RAISE NOTICE '[Migration 066/072] AddressGroupBinding deletion marked: service=%.%, ag=%.%',
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

            RAISE NOTICE '[Migration 066/072] Created DELETE outbox entry for AddressGroupBinding %.%',
                v_binding_namespace, v_binding_name;

            UPDATE services
            SET aggregated_address_groups = aggregate_service_address_groups(
                v_service_namespace::text,
                v_service_name::text
            )
            WHERE namespace = v_service_namespace AND name = v_service_name;

            RAISE NOTICE '[Migration 072] Updated Service %.% aggregated_address_groups (excluded deleted binding)',
                v_service_namespace, v_service_name;

        ELSE
            RAISE WARNING '[Migration 066/072] AddressGroupBinding not found for resource_version %',
                NEW.resource_version;
        END IF;

    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_addressgroupbinding_on_deletion_mark() IS
'Creates DELETE outbox entry when AddressGroupBinding is marked for deletion (deletion_timestamp set).
NOW UPDATES Service.aggregated_address_groups (Migration 072 fix) which triggers Service UPDATE outbox entry for SGROUP sync.
Similar to trigger_host_binding_on_deletion_mark() in Migration 044.';


DO $$
BEGIN
    RAISE NOTICE '════════════════════════════════════════════════════════════════';
    RAISE NOTICE '[Migration 072] AddressGroupBinding deletion SGROUP sync fix COMPLETE';
    RAISE NOTICE '════════════════════════════════════════════════════════════════';
    RAISE NOTICE '';
    RAISE NOTICE 'Fix 1: aggregate_service_address_groups() NOW filters deleted bindings';
    RAISE NOTICE '       - Added: JOIN k8s_metadata m ON m.resource_version = agb.resource_version';
    RAISE NOTICE '       - Added: WHERE m.deletion_timestamp IS NULL';
    RAISE NOTICE '';
    RAISE NOTICE 'Fix 2: trigger_addressgroupbinding_on_deletion_mark() NOW updates Service';
    RAISE NOTICE '       - Added: UPDATE services SET aggregated_address_groups = aggregate_service_address_groups(...)';
    RAISE NOTICE '       - This triggers Service UPDATE trigger (Migration 070)';
    RAISE NOTICE '       - Which creates SGROUP outbox entry';
    RAISE NOTICE '';
    RAISE NOTICE 'Expected behavior after fix:';
    RAISE NOTICE '  1. DELETE AddressGroupBinding → deletion_timestamp set';
    RAISE NOTICE '  2. Trigger fires → CREATE DELETE outbox (INTERNAL) + UPDATE Service.aggregated_address_groups';
    RAISE NOTICE '  3. Service UPDATE trigger fires → CREATE UPDATE outbox (SGROUP)';
    RAISE NOTICE '  4. OutboxWorker → Sync empty array to SGROUP ✓';
    RAISE NOTICE '';
    RAISE NOTICE 'Pattern follows Migration 044 (HostBinding) - proven and tested.';
    RAISE NOTICE '════════════════════════════════════════════════════════════════';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin


CREATE OR REPLACE FUNCTION aggregate_service_address_groups(
    svc_namespace TEXT,
    svc_name TEXT
) RETURNS JSONB AS $$
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
        WHERE agb.service_namespace = svc_namespace::namespace_name
        AND agb.service_name = svc_name::resource_name
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
$$ LANGUAGE plpgsql;

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

        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    RAISE WARNING '[Migration 072 Rollback] Reverted to Migration 066/018 - AddressGroupBinding deletion will NOT sync to SGROUP';
END $$;

-- +goose StatementEnd
