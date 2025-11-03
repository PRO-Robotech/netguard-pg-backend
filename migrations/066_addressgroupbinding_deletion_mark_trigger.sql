-- +goose Up
-- +goose StatementBegin

-- =====================================================
-- Migration 066: AddressGroupBinding Deletion Mark Trigger
-- =====================================================
-- Purpose: Create DELETE outbox entry when AddressGroupBinding.deletion_timestamp is set
-- Pattern: Identical to Migration 044 for HostBinding
--
-- Problem:
--   When coordinateServiceDelete() calls markBindingForDeletion(),
--   it sets deletion_timestamp on AddressGroupBinding, but NO TRIGGER creates DELETE outbox entry.
--   This causes binding to never be physically deleted, blocking Service deletion.
--
-- Solution:
--   Add AFTER UPDATE trigger on k8s_metadata.deletion_timestamp.
--   When deletion_timestamp is set (NULL → timestamp) for AddressGroupBinding:
--   1. Get binding details (service_namespace/name, address_group_namespace/name)
--   2. Create DELETE outbox entry (target=INTERNAL, affects_resources=[AddressGroup, Service])
--   3. OutboxWorker processes DELETE entry
--   4. Physical deletion happens after affected resources are synced
--
-- Flow:
--   coordinateServiceDelete() → markBindingForDeletion()
--     ↓
--   SET deletion_timestamp on AddressGroupBinding
--     ↓
--   THIS TRIGGER fires on deletion_timestamp change
--     ↓
--   CREATE DELETE outbox entry (target=INTERNAL)
--     ↓
--   OutboxWorker processes DELETE entry
--     ↓
--   Check affected resources (AddressGroup, Service) are synced
--     ↓
--   Physical DELETE AddressGroupBinding from database
--     ↓
--   Service deletion can proceed (FK no longer blocks)
--
-- Related Migrations:
--   - Migration 044: Same pattern for HostBinding
--   - Migration 056: AddressGroup → AddressGroupBinding CASCADE
--   - Migration 057: Service → AddressGroupBinding CASCADE
--
-- Date: 2025-10-31
-- =====================================================

-- Function: Create DELETE outbox entry when AddressGroupBinding is marked for deletion
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
    -- Only act when deletion_timestamp is being set (NULL → timestamp)
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

        -- ═══════════════════════════════════════════════════════════════════
        -- Create DELETE outbox entry
        -- ═══════════════════════════════════════════════════════════════════
        -- CRITICAL for coordinated deletion when Service or AG is deleted

        IF v_binding_found THEN
            -- AddressGroupBinding found - create DELETE entry with complete payload
            RAISE NOTICE '[Migration 066] AddressGroupBinding deletion marked: service=%.%, ag=%.%',
                v_service_namespace, v_service_name, v_ag_namespace, v_ag_name;

            -- Get resource_versions for Service and AddressGroup
            SELECT resource_version INTO v_service_rv
            FROM services
            WHERE namespace = v_service_namespace AND name = v_service_name;

            SELECT resource_version INTO v_ag_rv
            FROM address_groups
            WHERE namespace = v_ag_namespace AND name = v_ag_name;

            -- Build affected_resources array (Service and AddressGroup)
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

            -- Insert DELETE entry into sync_outbox with full payload
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
                0,  -- attempts
                20, -- max_retries
                NOW(),
                NOW(),
                NOW()
            )
            ON CONFLICT DO NOTHING;

            RAISE NOTICE '[Migration 066] Created DELETE outbox entry for AddressGroupBinding %.%',
                v_binding_namespace, v_binding_name;

        ELSE
            -- AddressGroupBinding not found - may have been deleted already
            RAISE WARNING '[Migration 066] AddressGroupBinding not found for resource_version %',
                NEW.resource_version;
        END IF;

    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger: Fire on deletion_timestamp UPDATE for AddressGroupBinding resources
-- Condition: deletion_timestamp changes from NULL to NOT NULL
CREATE TRIGGER addressgroupbinding_on_deletion_mark
AFTER UPDATE OF deletion_timestamp ON k8s_metadata
FOR EACH ROW
WHEN (NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL)
EXECUTE FUNCTION trigger_addressgroupbinding_on_deletion_mark();

COMMENT ON FUNCTION trigger_addressgroupbinding_on_deletion_mark() IS
'Creates DELETE outbox entry when AddressGroupBinding is marked for deletion (deletion_timestamp set).
Part of coordinated deletion pattern. See: docs/architecture/COORDINATED_BINDING_DELETION.md';

COMMENT ON TRIGGER addressgroupbinding_on_deletion_mark ON k8s_metadata IS
'Triggers DELETE outbox entry creation when AddressGroupBinding.deletion_timestamp is set.
Enables coordinated deletion for Service → AddressGroupBinding CASCADE.';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Drop trigger and function
DROP TRIGGER IF EXISTS addressgroupbinding_on_deletion_mark ON k8s_metadata;
DROP FUNCTION IF EXISTS trigger_addressgroupbinding_on_deletion_mark();

-- +goose StatementEnd
