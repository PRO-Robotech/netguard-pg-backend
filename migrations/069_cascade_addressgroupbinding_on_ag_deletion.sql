-- +goose Up
-- Migration 069: CASCADE Coordinated Deletion for AddressGroupBinding
--
-- This migration implements CASCADE coordinated deletion: when AddressGroup is marked for
-- deletion (deletion_timestamp set), all referencing AddressGroupBindings are automatically
-- marked for coordinated deletion as well.
--
-- Architecture pattern: Same as HostBinding in Migration 044
-- - AddressGroup gets deletion_timestamp (coordinated deletion initiated)
-- - CASCADE trigger sets deletion_timestamp on all referencing AddressGroupBindings
-- - aggregate_service_address_groups() filters deleted bindings
-- - OutboxWorker processes coordinated deletion for each binding
-- - After all bindings deleted → AG can be physically deleted
--
-- Related Migrations:
-- - Migration 044: Coordinated HostBinding deletion (pattern source)
-- - Migration 067: Service aggregation fix on AG deletion (incomplete - only recalculates)
-- - Migration 068: CASCADE AG deletion from Service.spec.addressGroups
--
-- Date: 2025-10-31

-- +goose StatementBegin

-- ============================================================================
-- PART 1: Modify aggregate_service_address_groups() to filter deleted bindings
-- ============================================================================
--
-- CRITICAL CHANGE: Add deletion_timestamp filter to exclude AddressGroupBindings
-- that are marked for deletion from aggregated_address_groups calculation.
--
-- This ensures that when an AddressGroupBinding is soft-deleted (deletion_timestamp set),
-- it immediately stops appearing in Service.aggregatedAddressGroups without
-- requiring physical deletion first.

CREATE OR REPLACE FUNCTION aggregate_service_address_groups(svc_namespace TEXT, svc_name TEXT)
RETURNS JSONB AS $$
DECLARE
    aggregated_ags_json JSONB := '[]'::jsonb;
    ag_ref JSONB;
    ag_record RECORD;
    ags_field JSONB;
BEGIN
    -- Part 1: Get AddressGroups from spec.addressGroups (unchanged logic)
    SELECT COALESCE(address_groups, '[]'::jsonb) INTO ags_field
    FROM services
    WHERE namespace = svc_namespace AND name = svc_name;

    IF ags_field IS NOT NULL AND ags_field != 'null'::jsonb
       AND jsonb_typeof(ags_field) = 'array' AND jsonb_array_length(ags_field) > 0 THEN
        FOR ag_ref IN
            SELECT jsonb_array_elements(ags_field) as ag_obj
        LOOP
            -- Validate that AddressGroup exists and is not deleted
            SELECT ag.namespace, ag.name INTO ag_record
            FROM address_groups ag
            JOIN k8s_metadata m ON m.resource_version = ag.resource_version
            WHERE ag.namespace = svc_namespace::namespace_name
            AND ag.name = (ag_ref->>'name')::resource_name
            AND m.deletion_timestamp IS NULL;  -- Exclude deleted AddressGroups

            IF FOUND THEN
                -- Add AG reference with source information
                aggregated_ags_json := aggregated_ags_json || jsonb_build_array(
                    jsonb_build_object(
                        'ref', ag_ref,
                        'source', 'spec'
                    )
                );
            END IF;
        END LOOP;
    END IF;

    -- Part 2: Add AddressGroups from AddressGroupBindings (WITH DELETION_TIMESTAMP FILTER)
    -- ✨ CRITICAL CHANGE: Added JOIN with k8s_metadata and deletion_timestamp filter
    FOR ag_record IN
        SELECT ag.namespace, ag.name
        FROM address_group_bindings agb
        JOIN address_groups ag ON ag.namespace = agb.address_group_namespace
                                AND ag.name = agb.address_group_name
        JOIN k8s_metadata m_agb ON m_agb.resource_version = agb.resource_version
        JOIN k8s_metadata m_ag ON m_ag.resource_version = ag.resource_version
        WHERE agb.service_namespace = svc_namespace::namespace_name
        AND agb.service_name = svc_name::resource_name
        AND m_agb.deletion_timestamp IS NULL  -- ← NEW: Exclude soft-deleted bindings
        AND m_ag.deletion_timestamp IS NULL   -- ← NEW: Exclude deleted AddressGroups
    LOOP
        -- Add AG reference with source information
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
'Aggregates AddressGroups from both Service.spec.addressGroups and AddressGroupBindings.
FILTERS OUT AddressGroupBindings with deletion_timestamp != NULL to support coordinated deletion.
Pattern: Same as aggregate_address_group_hosts() in Migration 044.';

-- ============================================================================
-- PART 2: CASCADE trigger - mark AddressGroupBindings for deletion when AG deleted
-- ============================================================================
--
-- This trigger fires when AddressGroup is marked for deletion (deletion_timestamp set).
-- It cascades the deletion to all AddressGroupBindings referencing this AddressGroup
-- by setting their deletion_timestamp, initiating coordinated deletion flow.
--
-- Flow:
-- 1. AddressGroup gets deletion_timestamp (user deletes AG)
-- 2. THIS TRIGGER fires on k8s_metadata UPDATE
-- 3. Sets deletion_timestamp on all AddressGroupBindings referencing this AG
-- 4. For each binding, trigger_addressgroupbinding_on_deletion_mark() fires (if exists)
-- 5. Service.aggregated_address_groups recalculated (binding excluded by filter)
-- 6. OutboxWorker processes DELETE for each binding
-- 7. After all bindings deleted → AG can be physically deleted (FK constraint released)

CREATE OR REPLACE FUNCTION cascade_addressgroupbinding_on_ag_deletion()
RETURNS TRIGGER AS $$
DECLARE
    ag_namespace namespace_name;
    ag_name resource_name;
    binding_count INT := 0;
BEGIN
    -- Only act when deletion_timestamp is being set (NULL → NOT NULL)
    IF NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL THEN

        -- Get AddressGroup details from address_groups table
        SELECT namespace, name INTO ag_namespace, ag_name
        FROM address_groups
        WHERE resource_version = NEW.resource_version;

        IF NOT FOUND THEN
            -- AddressGroup not found - might be already deleted or not an AG
            -- This is OK - trigger might fire for other resource types on k8s_metadata
            RETURN NEW;
        END IF;

        RAISE NOTICE '[Migration 069] AddressGroup %.% marked for deletion, cascading to AddressGroupBindings',
            ag_namespace, ag_name;

        -- CASCADE deletion_timestamp to all AddressGroupBindings referencing this AG
        UPDATE k8s_metadata
        SET deletion_timestamp = NOW()
        WHERE resource_version IN (
            SELECT resource_version
            FROM address_group_bindings
            WHERE address_group_namespace = ag_namespace
              AND address_group_name = ag_name
        );

        GET DIAGNOSTICS binding_count = ROW_COUNT;

        IF binding_count > 0 THEN
            RAISE NOTICE '[Migration 069] Marked % AddressGroupBinding(s) for deletion (AG: %.%)',
                binding_count, ag_namespace, ag_name;
        ELSE
            RAISE NOTICE '[Migration 069] No AddressGroupBindings found for AG %.%',
                ag_namespace, ag_name;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION cascade_addressgroupbinding_on_ag_deletion() IS
'Cascades coordinated deletion from AddressGroup to AddressGroupBindings.
Fires AFTER UPDATE of k8s_metadata.deletion_timestamp for address_groups.
Pattern: Same as Host → HostBinding cascade in Migration 044.';

-- Create trigger on k8s_metadata (fires for all resources, but filters for address_groups)
CREATE TRIGGER trigger_cascade_agb_on_ag_deletion
    AFTER UPDATE OF deletion_timestamp ON k8s_metadata
    FOR EACH ROW
    EXECUTE FUNCTION cascade_addressgroupbinding_on_ag_deletion();

COMMENT ON TRIGGER trigger_cascade_agb_on_ag_deletion ON k8s_metadata IS
'Cascades coordinated deletion from AddressGroup to AddressGroupBindings.
Fires when AddressGroup.deletion_timestamp is set.';

-- ============================================================================
-- PART 3: Data fix - recalculate aggregated_address_groups for all Services
-- ============================================================================
-- This ensures any existing deleted bindings are excluded from aggregated_address_groups

DO $$
BEGIN
    RAISE NOTICE '[Migration 069] Recalculating aggregated_address_groups for all Services...';

    UPDATE services
    SET aggregated_address_groups = aggregate_service_address_groups(namespace::text, name::text);

    RAISE NOTICE '[Migration 069] Migration complete. AddressGroupBinding CASCADE coordinated deletion enabled.';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert: Drop trigger and restore old function version
DROP TRIGGER IF EXISTS trigger_cascade_agb_on_ag_deletion ON k8s_metadata;
DROP FUNCTION IF EXISTS cascade_addressgroupbinding_on_ag_deletion();

-- Restore old aggregate_service_address_groups() without deletion_timestamp filter
-- (This reverts to the version before Migration 069)
CREATE OR REPLACE FUNCTION aggregate_service_address_groups(svc_namespace TEXT, svc_name TEXT)
RETURNS JSONB AS $$
DECLARE
    aggregated_ags_json JSONB := '[]'::jsonb;
    ag_ref JSONB;
    ag_record RECORD;
    ags_field JSONB;
BEGIN
    -- Part 1: Get AddressGroups from spec.addressGroups
    SELECT COALESCE(address_groups, '[]'::jsonb) INTO ags_field
    FROM services
    WHERE namespace = svc_namespace AND name = svc_name;

    IF ags_field IS NOT NULL AND ags_field != 'null'::jsonb
       AND jsonb_typeof(ags_field) = 'array' AND jsonb_array_length(ags_field) > 0 THEN
        FOR ag_ref IN
            SELECT jsonb_array_elements(ags_field) as ag_obj
        LOOP
            SELECT ag.namespace, ag.name INTO ag_record
            FROM address_groups ag
            WHERE ag.namespace = svc_namespace::namespace_name
            AND ag.name = (ag_ref->>'name')::resource_name;

            IF FOUND THEN
                aggregated_ags_json := aggregated_ags_json || jsonb_build_array(
                    jsonb_build_object(
                        'ref', ag_ref,
                        'source', 'spec'
                    )
                );
            END IF;
        END LOOP;
    END IF;

    -- Part 2: Add AddressGroups from AddressGroupBindings (NO FILTER - old version)
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

RAISE NOTICE '[Migration 069 Rollback] Cascade trigger removed. Manual cleanup may be required.';

-- +goose StatementEnd
