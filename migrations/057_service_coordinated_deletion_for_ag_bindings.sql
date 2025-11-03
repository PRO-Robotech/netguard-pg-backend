-- +goose Up
-- +goose StatementBegin

-- =====================================================
-- Migration 057: Service Coordinated Deletion for AddressGroupBindings
-- =====================================================
-- Purpose: Create DELETE outbox entries when Service is marked for deletion
-- Pattern: Identical to Migration 054 (Network coordinated deletion)
--
-- Problem:
--   Service deletion is blocked by RESTRICT FK when AddressGroupBindings exist.
--   Current FK: FOREIGN KEY (service_namespace, service_name)
--               REFERENCES services(namespace, name) ON DELETE RESTRICT
--   No coordination trigger exists for Service deletion.
--
-- Solution:
--   Add AFTER UPDATE trigger on k8s_metadata.deletion_timestamp for Service resources.
--   When deletion_timestamp is set (NULL → timestamp):
--   1. Find all AddressGroupBindings referencing this Service
--   2. For each binding: create DELETE outbox entry (target=INTERNAL)
--   3. OutboxWorker processes DELETE entries
--   4. Physical deletion happens after SGROUP sync
--
-- Flow:
--   User → Delete Service
--     ↓
--   BEFORE DELETE trigger: service_before_delete
--     ↓
--   SET deletion_timestamp in k8s_metadata
--     ↓
--   AFTER UPDATE trigger: service_on_deletion_mark (THIS MIGRATION)
--     ↓
--   For each AddressGroupBinding:
--     - Create DELETE outbox entry (target=INTERNAL)
--     ↓
--   OutboxWorker processes AddressGroupBinding DELETE entries
--     ↓
--   Physical DELETE AddressGroupBinding from database (CASCADE)
--     ↓
--   Service physical deletion after SGROUP sync
--
-- Related Migrations:
--   - Migration 054: Same pattern for Network → NetworkBinding
--   - Migration 056: Same pattern for AG → AddressGroupBinding
--
-- Date: 2025-10-31
-- =====================================================

-- Function: Create DELETE outbox entries for AddressGroupBindings when Service is marked for deletion
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
    -- Only act when deletion_timestamp is being set (NULL → timestamp)
    IF NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL THEN
        -- Find Service by resource_version
        SELECT namespace, name
        INTO v_service_namespace, v_service_name
        FROM services
        WHERE resource_version = NEW.resource_version;

        IF FOUND THEN
            RAISE NOTICE '[Migration 057] Service %.% marked for deletion, processing AddressGroupBindings',
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
                RAISE NOTICE '[Migration 057] Processing AddressGroupBinding %.%',
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
                                'namespace', v_service_namespace,
                                'name', v_service_name
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

                RAISE NOTICE '[Migration 057] Created DELETE outbox entry for AddressGroupBinding %.%',
                    binding_rec.namespace, binding_rec.name;
            END LOOP;

            RAISE NOTICE '[Migration 057] Finished processing AddressGroupBindings for Service %.%',
                v_service_namespace, v_service_name;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger: Fire on deletion_timestamp UPDATE for Service resources
-- Note: WHEN clause cannot use subqueries (PostgreSQL limitation)
--       Condition check moved inside function
CREATE TRIGGER service_on_deletion_mark_for_ag_bindings
AFTER UPDATE OF deletion_timestamp ON k8s_metadata
FOR EACH ROW
WHEN (NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL)
EXECUTE FUNCTION trigger_service_on_deletion_mark_for_ag_bindings();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Drop trigger and function
DROP TRIGGER IF EXISTS service_on_deletion_mark_for_ag_bindings ON k8s_metadata;
DROP FUNCTION IF EXISTS trigger_service_on_deletion_mark_for_ag_bindings();

-- +goose StatementEnd
