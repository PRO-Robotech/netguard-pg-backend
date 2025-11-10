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

        IF NOT v_binding_found THEN
            RETURN NEW;
        END IF;


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
                'reason', 'Coordinated deletion',
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

        RAISE NOTICE '[Migration 081] Created DELETE outbox entry for AddressGroupBinding %.%',
            v_binding_namespace, v_binding_name;

        UPDATE services
        SET aggregated_address_groups = aggregate_service_address_groups(
            v_service_namespace::text,
            v_service_name::text
        )
        WHERE namespace = v_service_namespace AND name = v_service_name;

        RAISE NOTICE '[Migration 081] Updated Service %.% aggregated_address_groups (binding deleted)',
            v_service_namespace, v_service_name;

    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_addressgroupbinding_on_deletion_mark() IS
'Creates DELETE outbox entry when AddressGroupBinding is marked for deletion (deletion_timestamp set).
Migration 081 fix: Added early return when binding not found to prevent cross-firing for other resource types.
This prevents "update ALL Services" fallback from executing for HostBinding/NetworkBinding deletions.';


DO $$
BEGIN
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '[Migration 081] Cross-firing trigger fix COMPLETE';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '';
    RAISE NOTICE 'FIXED: trigger_addressgroupbinding_on_deletion_mark() now returns early for non-AddressGroupBinding';
    RAISE NOTICE '';
    RAISE NOTICE 'Before Migration 081:';
    RAISE NOTICE '  - HostBinding deleted → AddressGroupBinding trigger fires ✗';
    RAISE NOTICE '  - Binding NOT found in table ✗';
    RAISE NOTICE '  - Fallback strategy: UPDATE ALL Services ✗✗✗ (UNACCEPTABLE!)';
    RAISE NOTICE '  - Creates UPDATE outbox entries for HUNDREDS of Services ✗';
    RAISE NOTICE '  - OutboxWorker overwhelmed with spurious sync operations ✗';
    RAISE NOTICE '';
    RAISE NOTICE 'After Migration 081:';
    RAISE NOTICE '  - HostBinding deleted → AddressGroupBinding trigger fires ✓';
    RAISE NOTICE '  - Binding NOT found in table → RETURN NEW (early exit) ✓';
    RAISE NOTICE '  - No fallback strategy executed ✓';
    RAISE NOTICE '  - No spurious Service updates ✓';
    RAISE NOTICE '  - HostBinding trigger handles deletion correctly ✓';
    RAISE NOTICE '';
    RAISE NOTICE 'Result: Trigger only processes actual AddressGroupBinding deletions!';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
END $$;

-- +goose StatementEnd

-- +goose Down
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
    v_service_found BOOLEAN := FALSE;
    v_outbox_payload JSONB;
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

        IF NOT v_binding_found THEN
            RAISE WARNING '[Migration 078] AddressGroupBinding not found in table for resource_version %, attempting outbox lookup', NEW.resource_version;

            SELECT payload INTO v_outbox_payload
            FROM sync_outbox
            WHERE resource_id = v_uid
              AND resource_type = 'AddressGroupBinding'
              AND operation IN ('CREATE', 'UPDATE')
              AND target_system = 'INTERNAL'
            ORDER BY created_at DESC
            LIMIT 1;

            IF FOUND AND v_outbox_payload IS NOT NULL THEN
                v_service_namespace := (v_outbox_payload->'serviceRef'->>'namespace')::namespace_name;
                v_service_name := (v_outbox_payload->'serviceRef'->>'name')::resource_name;
                v_ag_namespace := (v_outbox_payload->'addressGroupRef'->>'namespace')::namespace_name;
                v_ag_name := (v_outbox_payload->'addressGroupRef'->>'name')::resource_name;
                v_binding_namespace := (v_outbox_payload->>'namespace')::namespace_name;
                v_binding_name := (v_outbox_payload->>'name')::resource_name;

                v_service_found := TRUE;
                RAISE NOTICE '[Migration 078] Extracted Service info from outbox: service=%.%, ag=%.%',
                    v_service_namespace, v_service_name, v_ag_namespace, v_ag_name;
            ELSE
                RAISE WARNING '[Migration 078] No outbox entry found for AddressGroupBinding UID %, cannot determine affected Service', v_uid;
            END IF;
        ELSE
            v_service_found := TRUE;
        END IF;

        IF v_service_found THEN
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
                    'reason', 'Coordinated deletion',
                    'deletionTimestamp', NEW.deletion_timestamp::text,
                    'recoveredFromOutbox', NOT v_binding_found
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

            RAISE NOTICE '[Migration 078] Created DELETE outbox entry for AddressGroupBinding %.% (recovered=%)',
                v_binding_namespace, v_binding_name, NOT v_binding_found;

            UPDATE services
            SET aggregated_address_groups = aggregate_service_address_groups(
                v_service_namespace::text,
                v_service_name::text
            )
            WHERE namespace = v_service_namespace AND name = v_service_name;

            RAISE NOTICE '[Migration 078] Updated Service %.% aggregated_address_groups (binding may be deleted)',
                v_service_namespace, v_service_name;

        ELSE
            RAISE WARNING '[Migration 078] Cannot determine Service for AddressGroupBinding (rv=%, uid=%), creating minimal DELETE entry',
                NEW.resource_version, v_uid;

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
                'AddressGroupBinding',
                v_uid,
                'unknown',
                'unknown',
                'DELETE'::sync_operation,
                'INTERNAL'::target_system,
                jsonb_build_object(
                    'resourceVersion', NEW.resource_version,
                    'uid', v_uid,
                    'note', 'Minimal DELETE entry - binding and outbox data not found (race condition)'
                ),
                'PENDING'::outbox_status,
                0,
                20,
                NOW(),
                NOW(),
                NOW()
            )
            ON CONFLICT DO NOTHING;

            UPDATE services
            SET aggregated_address_groups = aggregate_service_address_groups(
                namespace::text,
                name::text
            );

            RAISE WARNING '[Migration 078] Updated ALL Services (fallback strategy for unknown binding)';
        END IF;

    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    RAISE WARNING '[Migration 081 Rollback] Reverted to Migration 078 - cross-firing bug will return!';
END $$;

-- +goose StatementEnd
