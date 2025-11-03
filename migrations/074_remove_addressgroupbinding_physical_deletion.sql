-- +goose Up
-- Migration 074: Remove Physical Deletion from AddressGroupBinding Coordinated Deletion Trigger
--
-- ROOT CAUSE IDENTIFIED:
-- Migration 073 trigger `trigger_address_group_binding_on_deletion_mark()` physically
-- deletes AddressGroupBinding from the table (line 147-149) WHILE `MarkForDeletionWithStatus()`
-- is still executing in the same transaction and expects the binding to exist.
--
-- THE FLOW (BROKEN):
-- 1. kubectl delete → apiserver → MarkForDeletionWithStatus()
-- 2. UPDATE k8s_metadata SET deletion_timestamp = NOW() ...
-- 3. ▶️ Migration 073 trigger fires (AFTER UPDATE)
-- 4.    Trigger reads binding → updates Service → creates outbox → ❌ DELETES BINDING
-- 5. ◀️ Control returns to MarkForDeletionWithStatus
-- 6. SELECT uid FROM address_group_bindings WHERE ... ← ❌ ERROR: no rows in result set!
--
-- SOLUTION:
-- Remove physical DELETE from trigger. Let binding remain with deletion_timestamp set.
-- Physical deletion happens automatically via CASCADE when k8s_metadata is removed
-- (after finalizers complete).
--
-- COMPARISON WITH HostBinding (Migration 044):
-- HostBinding trigger DOES NOT physically delete. Same pattern should apply here.

-- +goose StatementBegin

-- ============================================================================
-- PART 1: Recreate trigger function WITHOUT physical deletion
-- ============================================================================

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
            -- ✅ FIXED: DO NOT physically delete binding here!
            -- ═══════════════════════════════════════════════════════════════════
            -- Physical deletion will happen automatically via CASCADE DELETE when
            -- k8s_metadata row is removed (after finalizers complete).
            --
            -- This matches HostBinding pattern (Migration 044) which also does NOT
            -- physically delete in the trigger.
            --
            -- Binding now remains with deletion_timestamp set, allowing
            -- MarkForDeletionWithStatus() to complete successfully.

            RAISE NOTICE 'AddressGroupBinding %.% marked for deletion (physical deletion via CASCADE later)',
                v_binding_namespace, v_binding_name;

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
Binding remains with deletion_timestamp set. Physical deletion via CASCADE later.
Fixed Migration 073 bug where trigger deleted binding while MarkForDeletionWithStatus was executing.';

-- Trigger already exists from Migration 073, no need to recreate

-- +goose StatementEnd

-- +goose Down
-- Rollback to Migration 073 behavior (with physical deletion)

-- +goose StatementBegin

-- Restore Migration 073 version (with DELETE statement)
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
            UPDATE services
            SET aggregated_address_groups = aggregate_service_address_groups(
                v_service_namespace::text,
                v_service_name::text
            )
            WHERE namespace = v_service_namespace
              AND name = v_service_name;

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

            -- Restore physical deletion (Migration 073 behavior)
            DELETE FROM address_group_bindings
            WHERE resource_version = NEW.resource_version;

        ELSE
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
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd
