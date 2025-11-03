-- +goose Up
-- +goose StatementBegin

-- =====================================================
-- Migration 064: Fix Service Deletion Trigger SELECT Fields
-- =====================================================
-- Purpose: Add missing service_namespace and service_name to SELECT
-- Problem: Migration 063 trigger fails with "record has no field service_namespace"
-- Root Cause: FOR loop SELECT missing service_namespace and service_name fields
--
-- Error:
--   ERROR:  record "binding_rec" has no field "service_namespace"
--   CONTEXT:  PL/pgSQL function trigger_service_on_deletion_mark_for_ag_bindings() line 54
--
-- Solution:
--   Add service_namespace and service_name to binding_rec SELECT
--
-- Date: 2025-10-31
-- =====================================================

CREATE OR REPLACE FUNCTION trigger_service_on_deletion_mark_for_ag_bindings()
RETURNS TRIGGER AS $$
DECLARE
    v_service_namespace namespace_name;
    v_service_name resource_name;
    binding_rec RECORD;
    v_binding_uid UUID;
    v_ag_uid UUID;
    v_ag_rv BIGINT;
BEGIN
    -- Debug: Log every trigger invocation
    RAISE NOTICE '[Migration 064/DEBUG] Trigger fired: OLD.deletion_timestamp=%, NEW.deletion_timestamp=%',
        OLD.deletion_timestamp, NEW.deletion_timestamp;

    -- Only act when deletion_timestamp is being set (NULL → timestamp)
    IF NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL THEN
        RAISE NOTICE '[Migration 064/DEBUG] Deletion timestamp set, checking if this is a Service...';

        -- Find Service by resource_version
        SELECT namespace, name
        INTO v_service_namespace, v_service_name
        FROM services
        WHERE resource_version = NEW.resource_version;

        IF FOUND THEN
            RAISE NOTICE '[Migration 064] Service %.% marked for deletion, processing AddressGroupBindings',
                v_service_namespace, v_service_name;

            -- Process all AddressGroupBindings referencing this Service
            -- FIXED: Added service_namespace and service_name to SELECT
            FOR binding_rec IN
                SELECT agb.namespace, agb.name,
                       agb.service_namespace, agb.service_name,
                       agb.address_group_namespace, agb.address_group_name,
                       agb.resource_version
                FROM address_group_bindings agb
                WHERE agb.service_namespace = v_service_namespace
                  AND agb.service_name = v_service_name
            LOOP
                RAISE NOTICE '[Migration 064] Processing AddressGroupBinding %.%',
                    binding_rec.namespace, binding_rec.name;

                -- Get UID for AddressGroupBinding
                SELECT m.uid INTO v_binding_uid
                FROM k8s_metadata m
                WHERE m.resource_version = binding_rec.resource_version;

                -- Get AddressGroup UID and resource_version for affects_resources
                SELECT m.uid, ag.resource_version
                INTO v_ag_uid, v_ag_rv
                FROM address_groups ag
                JOIN k8s_metadata m ON m.resource_version = ag.resource_version
                WHERE ag.namespace = binding_rec.address_group_namespace
                  AND ag.name = binding_rec.address_group_name;

                -- Create DELETE outbox entry for AddressGroupBinding (target=INTERNAL)
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
                                'namespace', binding_rec.address_group_namespace,
                                'name', binding_rec.address_group_name
                            )
                        ),
                        'reason', 'Service deletion (coordinated)',
                        'deletionTimestamp', NEW.deletion_timestamp::text
                    ),
                    'PENDING'::outbox_status,
                    jsonb_build_array(
                        jsonb_build_object(
                            'type', 'AddressGroup',
                            'uid', v_ag_uid,
                            'namespace', binding_rec.address_group_namespace,
                            'name', binding_rec.address_group_name,
                            'resourceVersion', v_ag_rv
                        )
                    ),
                    NOW(),
                    NOW(),
                    NOW()
                ) ON CONFLICT DO NOTHING;

                RAISE NOTICE '[Migration 064] Created DELETE outbox entry for AddressGroupBinding %.%',
                    binding_rec.namespace, binding_rec.name;

                -- Update aggregated_address_groups in Service (same as Migration 057)
                PERFORM aggregate_service_address_groups(v_service_namespace, v_service_name);

                RAISE NOTICE '[Migration 064] Updated aggregated_address_groups for Service %.%',
                    v_service_namespace, v_service_name;
            END LOOP;

            IF NOT FOUND THEN
                RAISE NOTICE '[Migration 064] No AddressGroupBindings found for Service %.%',
                    v_service_namespace, v_service_name;
            END IF;
        ELSE
            RAISE NOTICE '[Migration 064/DEBUG] Not a Service (resource_version=%), skipping',
                NEW.resource_version;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger already exists from Migration 063, no need to recreate

-- Log migration completion
DO $$
BEGIN
    RAISE NOTICE '[Migration 064] Service deletion trigger function updated with correct SELECT fields';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to Migration 063 version (with bug)
CREATE OR REPLACE FUNCTION trigger_service_on_deletion_mark_for_ag_bindings()
RETURNS TRIGGER AS $$
DECLARE
    v_service_namespace namespace_name;
    v_service_name resource_name;
    binding_rec RECORD;
    v_binding_uid UUID;
    v_ag_uid UUID;
    v_ag_rv BIGINT;
BEGIN
    -- Debug: Log every trigger invocation
    RAISE NOTICE '[Migration 063/DEBUG] Trigger fired: OLD.deletion_timestamp=%, NEW.deletion_timestamp=%',
        OLD.deletion_timestamp, NEW.deletion_timestamp;

    -- Only act when deletion_timestamp is being set (NULL → timestamp)
    IF NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL THEN
        RAISE NOTICE '[Migration 063/DEBUG] Deletion timestamp set, checking if this is a Service...';

        -- Find Service by resource_version
        SELECT namespace, name
        INTO v_service_namespace, v_service_name
        FROM services
        WHERE resource_version = NEW.resource_version;

        IF FOUND THEN
            RAISE NOTICE '[Migration 063] Service %.% marked for deletion, processing AddressGroupBindings',
                v_service_namespace, v_service_name;

            -- Process all AddressGroupBindings referencing this Service
            FOR binding_rec IN
                SELECT agb.namespace, agb.name,
                       agb.address_group_namespace, agb.address_group_name,
                       agb.resource_version
                FROM address_group_bindings agb
                WHERE agb.service_namespace = v_service_namespace
                  AND agb.service_name = v_service_name
            LOOP
                RAISE NOTICE '[Migration 063] Processing AddressGroupBinding %.%',
                    binding_rec.namespace, binding_rec.name;

                -- Get UID for AddressGroupBinding
                SELECT m.uid INTO v_binding_uid
                FROM k8s_metadata m
                WHERE m.resource_version = binding_rec.resource_version;

                -- Get AddressGroup UID and resource_version for affects_resources
                SELECT m.uid, ag.resource_version
                INTO v_ag_uid, v_ag_rv
                FROM address_groups ag
                JOIN k8s_metadata m ON m.resource_version = ag.resource_version
                WHERE ag.namespace = binding_rec.address_group_namespace
                  AND ag.name = binding_rec.address_group_name;

                -- Create DELETE outbox entry for AddressGroupBinding (target=INTERNAL)
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
                                'namespace', binding_rec.address_group_namespace,
                                'name', binding_rec.address_group_name
                            )
                        ),
                        'reason', 'Service deletion (coordinated)',
                        'deletionTimestamp', NEW.deletion_timestamp::text
                    ),
                    'PENDING'::outbox_status,
                    jsonb_build_array(
                        jsonb_build_object(
                            'type', 'AddressGroup',
                            'uid', v_ag_uid,
                            'namespace', binding_rec.address_group_namespace,
                            'name', binding_rec.address_group_name,
                            'resourceVersion', v_ag_rv
                        )
                    ),
                    NOW(),
                    NOW(),
                    NOW()
                ) ON CONFLICT DO NOTHING;

                RAISE NOTICE '[Migration 063] Created DELETE outbox entry for AddressGroupBinding %.%',
                    binding_rec.namespace, binding_rec.name;

                -- Update aggregated_address_groups in Service (same as Migration 057)
                PERFORM aggregate_service_address_groups(v_service_namespace, v_service_name);

                RAISE NOTICE '[Migration 063] Updated aggregated_address_groups for Service %.%',
                    v_service_namespace, v_service_name;
            END LOOP;

            IF NOT FOUND THEN
                RAISE NOTICE '[Migration 063] No AddressGroupBindings found for Service %.%',
                    v_service_namespace, v_service_name;
            END IF;
        ELSE
            RAISE NOTICE '[Migration 063/DEBUG] Not a Service (resource_version=%), skipping',
                NEW.resource_version;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd
