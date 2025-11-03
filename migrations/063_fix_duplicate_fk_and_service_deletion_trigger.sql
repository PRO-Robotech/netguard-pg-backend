-- +goose Up
-- +goose StatementBegin

-- =====================================================
-- Migration 063: Fix Duplicate FK Constraints + Fix Service Deletion Trigger
-- =====================================================
-- Purpose: Fix two critical bugs preventing Service coordinated deletion
--
-- Problem 1: Duplicate FK constraints on address_group_bindings → address_groups
--   address_group_bindings has TWO FK constraints to address_groups:
--   - ..._address_gro_fkey (CASCADE) - from old schema
--   - ..._address_group_na (RESTRICT) - from Migration 055
--   This conflict may cause undefined behavior.
--
-- Problem 2: Service deletion trigger not working
--   Migration 057 trigger does NOT fire when Service is deleted.
--   Service stays with deletion_timestamp set, binding не удаляется, FK RESTRICT блокирует.
--
-- Solution:
--   Part 1: Remove CASCADE FK, keep only RESTRICT FK
--   Part 2: Debug and fix Service deletion trigger
--
-- Date: 2025-10-31
-- =====================================================

-- ==================== PART 1: Fix Duplicate FK ====================

-- Drop CASCADE FK constraint (if exists)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'address_group_bindings_address_group_namespace_address_gro_fkey'
    ) THEN
        ALTER TABLE address_group_bindings
        DROP CONSTRAINT address_group_bindings_address_group_namespace_address_gro_fkey;

        RAISE NOTICE '[Migration 063] Dropped CASCADE FK constraint';
    ELSE
        RAISE NOTICE '[Migration 063] CASCADE FK constraint already removed';
    END IF;
END $$;

-- Verify RESTRICT FK exists (it should from Migration 055)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'address_group_bindings_address_group_namespace_address_group_na'
    ) THEN
        -- If RESTRICT FK doesn't exist, create it
        ALTER TABLE address_group_bindings
        ADD CONSTRAINT address_group_bindings_address_group_namespace_address_group_na
            FOREIGN KEY (address_group_namespace, address_group_name)
            REFERENCES address_groups(namespace, name)
            ON DELETE RESTRICT;

        RAISE NOTICE '[Migration 063] Created RESTRICT FK constraint';
    ELSE
        RAISE NOTICE '[Migration 063] RESTRICT FK constraint exists';
    END IF;
END $$;

-- ==================== PART 2: Fix Service Deletion Trigger ====================

-- The trigger exists but doesn't fire. Debugging approach:
-- 1. Add extensive logging
-- 2. Verify trigger fires on ALL k8s_metadata updates
-- 3. Check Service lookup logic

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

-- Ensure trigger exists
DROP TRIGGER IF EXISTS service_on_deletion_mark_for_ag_bindings ON k8s_metadata;

CREATE TRIGGER service_on_deletion_mark_for_ag_bindings
AFTER UPDATE OF deletion_timestamp ON k8s_metadata
FOR EACH ROW
WHEN (NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL)
EXECUTE FUNCTION trigger_service_on_deletion_mark_for_ag_bindings();

-- Log migration completion (must be in DO block)
DO $$
BEGIN
    RAISE NOTICE '[Migration 063] Service deletion trigger recreated with debug logging';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to Migration 057 version (without debug logging)
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
    IF NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL THEN
        SELECT namespace, name
        INTO v_service_namespace, v_service_name
        FROM services
        WHERE resource_version = NEW.resource_version;

        IF FOUND THEN
            RAISE NOTICE '[Migration 057] Service %.% marked for deletion, processing AddressGroupBindings',
                v_service_namespace, v_service_name;

            FOR binding_rec IN
                SELECT agb.namespace, agb.name,
                       agb.address_group_namespace, agb.address_group_name,
                       agb.resource_version
                FROM address_group_bindings agb
                WHERE agb.service_namespace = v_service_namespace
                  AND agb.service_name = v_service_name
            LOOP
                RAISE NOTICE '[Migration 057] Processing AddressGroupBinding %.%',
                    binding_rec.namespace, binding_rec.name;

                SELECT m.uid INTO v_binding_uid
                FROM k8s_metadata m
                WHERE m.resource_version = binding_rec.resource_version;

                SELECT m.uid, ag.resource_version
                INTO v_ag_uid, v_ag_rv
                FROM address_groups ag
                JOIN k8s_metadata m ON m.resource_version = ag.resource_version
                WHERE ag.namespace = binding_rec.address_group_namespace
                  AND ag.name = binding_rec.address_group_name;

                INSERT INTO sync_outbox (
                    resource_type,
                    resource_id,
                    resource_namespace,
                    resource_name,
                    operation,
                    target_system,
                    status,
                    sync_subject_type,
                    sync_subject_id,
                    affects_resources
                ) VALUES (
                    'AddressGroupBinding',
                    v_binding_uid,
                    binding_rec.namespace,
                    binding_rec.name,
                    'DELETE',
                    'INTERNAL',
                    'PENDING',
                    'Service',
                    (SELECT m.uid FROM k8s_metadata m WHERE m.resource_version = NEW.resource_version),
                    jsonb_build_array(
                        jsonb_build_object(
                            'type', 'AddressGroup',
                            'id', v_ag_uid,
                            'resource_version', v_ag_rv
                        )
                    )
                );

                PERFORM aggregate_service_address_groups(v_service_namespace, v_service_name);
            END LOOP;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Restore CASCADE FK if needed for rollback compatibility
ALTER TABLE address_group_bindings
ADD CONSTRAINT address_group_bindings_address_group_namespace_address_gro_fkey
    FOREIGN KEY (address_group_namespace, address_group_name)
    REFERENCES address_groups(namespace, name)
    ON DELETE CASCADE;

-- +goose StatementEnd
