-- +goose Up
-- Migration 073: Fix AddressGroupBinding Coordinated Deletion
--
-- ROOT CAUSE FOUND:
-- When kubectl delete removes AddressGroupBinding, it triggers CASCADE DELETE
-- via k8s_metadata.resource_version foreign key. CASCADE DELETE operations
-- **DO NOT ACTIVATE USER-DEFINED TRIGGERS** in PostgreSQL. This means:
-- 1. trigger_update_aggregated_ags_on_binding_change() NEVER FIRES
-- 2. Service.aggregated_address_groups NEVER UPDATES
-- 3. Service UPDATE trigger (Migration 070) NEVER FIRES
-- 4. NO Service UPDATE outbox entry created
-- 5. SGROUP NEVER RECEIVES UPDATE → OLD DATA IN SGROUP!
--
-- SOLUTION: Coordinated Deletion Pattern (like HostBinding in Migration 044)
-- 1. Soft delete: Set deletion_timestamp instead of physical DELETE
-- 2. AFTER UPDATE trigger on k8s_metadata handles deletion_timestamp change
-- 3. Update Service.aggregated_address_groups (activates Service UPDATE trigger)
-- 4. Service UPDATE trigger creates SGROUP outbox entry
-- 5. Physical deletion happens AFTER Service update
--
-- Migration 072 already prepared aggregate_service_address_groups() with
-- deletion_timestamp filter - this migration completes the coordinated deletion.

-- +goose StatementBegin

-- ============================================================================
-- PART 1: New trigger for AddressGroupBinding soft delete handling
-- ============================================================================
--
-- This trigger fires AFTER deletion_timestamp is set on AddressGroupBinding.
-- It updates Service to reflect binding removal BEFORE physical deletion.
--
-- Flow:
-- 1. kubectl delete → apiserver BEFORE DELETE trigger sets deletion_timestamp
-- 2. THIS TRIGGER fires on deletion_timestamp change
-- 3. Updates Service: recalculate aggregated_address_groups (binding excluded by filter)
-- 4. Service UPDATE trigger (Migration 070) fires → creates SGROUP outbox entry ✅
-- 5. Physical deletion happens after Service update

CREATE OR REPLACE FUNCTION trigger_address_group_binding_on_deletion_mark()
RETURNS TRIGGER AS $$
DECLARE
    v_service_namespace namespace_name;
    v_service_name resource_name;
    v_ag_namespace namespace_name;
    v_ag_name resource_name;
    v_binding_namespace namespace_name;
    v_binding_name resource_name;
    v_uid UUID;
    v_binding_found BOOLEAN := false;
BEGIN
    -- Only act when deletion_timestamp is being set (NULL → NOT NULL)
    IF NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL THEN

        -- Get UID from k8s_metadata (ALWAYS available)
        v_uid := NEW.uid;

        -- Try to get AddressGroupBinding details from address_group_bindings table
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
            RAISE NOTICE 'AddressGroupBinding deletion marked: service=%.%, ag=%/%',
                v_service_namespace, v_service_name, v_ag_namespace, v_ag_name;

            -- ═══════════════════════════════════════════════════════════════════
            -- CRITICAL: Update Service.aggregated_address_groups
            -- ═══════════════════════════════════════════════════════════════════
            -- This UPDATE triggers Service UPDATE trigger (Migration 070)
            -- which creates Service UPDATE outbox entry with target=SGROUP ✅
            --
            -- aggregate_service_address_groups() (from Migration 072) automatically
            -- EXCLUDES this binding because it has deletion_timestamp != NULL
            UPDATE services
            SET aggregated_address_groups = aggregate_service_address_groups(
                v_service_namespace::text,
                v_service_name::text
            )
            WHERE namespace = v_service_namespace
              AND name = v_service_name;

            RAISE NOTICE 'Service %.% updated: aggregated_address_groups recalculated (binding excluded)',
                v_service_namespace, v_service_name;

            -- ═══════════════════════════════════════════════════════════════════
            -- Create DELETE outbox entry for binding (target=INTERNAL)
            -- ═══════════════════════════════════════════════════════════════════
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
                    )
                ),
                'PENDING'::outbox_status,
                0,
                5,
                NOW(),
                NOW(),
                NOW()
            )
            ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

            RAISE NOTICE 'AddressGroupBinding %.% DELETE entry created in sync_outbox (target_system=INTERNAL)',
                v_binding_namespace, v_binding_name;

            -- ═══════════════════════════════════════════════════════════════════
            -- Physical deletion happens immediately after Service update
            -- ═══════════════════════════════════════════════════════════════════
            -- Unlike HostBinding which waits for OutboxWorker, we delete immediately
            -- because Service update already triggered SGROUP sync
            DELETE FROM address_group_bindings
            WHERE resource_version = NEW.resource_version;

            RAISE NOTICE 'AddressGroupBinding %.% physically deleted', v_binding_namespace, v_binding_name;

        ELSE
            -- AddressGroupBinding NOT found (race condition or already deleted)
            RAISE WARNING 'AddressGroupBinding not found for resource_version %, creating minimal DELETE entry', NEW.resource_version;

            -- Insert minimal DELETE entry
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
                    'note', 'Minimal DELETE entry - binding already removed (race condition)'
                ),
                'PENDING'::outbox_status,
                0,
                5,
                NOW(),
                NOW(),
                NOW()
            )
            ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

            RAISE NOTICE 'Minimal AddressGroupBinding DELETE entry created for resource_version % (uid=%)',
                NEW.resource_version, v_uid;
        END IF;

    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trigger_address_group_binding_on_deletion_mark() IS
'Handles AddressGroupBinding coordinated deletion when deletion_timestamp is set.
Updates Service.aggregated_address_groups which triggers Service UPDATE and SGROUP sync.
Physically deletes binding after Service update.
Fixes CASCADE DELETE issue where triggers were bypassed.';

-- Create trigger on k8s_metadata for AddressGroupBinding resources
-- Fires when deletion_timestamp is updated (NULL → timestamp)
CREATE TRIGGER address_group_binding_on_deletion_mark
AFTER UPDATE OF deletion_timestamp ON k8s_metadata
FOR EACH ROW
WHEN (
    NEW.deletion_timestamp IS NOT NULL
    AND OLD.deletion_timestamp IS NULL
)
EXECUTE FUNCTION trigger_address_group_binding_on_deletion_mark();

COMMENT ON TRIGGER address_group_binding_on_deletion_mark ON k8s_metadata IS
'Fires when AddressGroupBinding is soft-deleted (deletion_timestamp set).
Updates Service before physical deletion to ensure SGROUP sync.';

-- +goose StatementEnd

-- +goose Down
-- Rollback coordinated deletion changes

-- +goose StatementBegin

-- Drop new trigger
DROP TRIGGER IF EXISTS address_group_binding_on_deletion_mark ON k8s_metadata;
DROP FUNCTION IF EXISTS trigger_address_group_binding_on_deletion_mark();

-- +goose StatementEnd
