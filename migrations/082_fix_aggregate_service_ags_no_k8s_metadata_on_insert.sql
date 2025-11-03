-- +goose Up
-- Migration 082: Fix aggregate_service_address_groups() for INSERT binding (k8s_metadata not yet created)
--
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
-- CRITICAL BUG FIX: Service UPDATE outbox entry NOT created when AddressGroupBinding created
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
--
-- Root Cause (discovered 2025-11-01):
--   Migration 078 added JOIN k8s_metadata to aggregate_service_address_groups() to filter soft-deleted bindings:
--
--   FROM address_group_bindings agb
--   JOIN k8s_metadata m ON m.resource_version = agb.resource_version  -- ← INNER JOIN!
--   WHERE ... AND m.deletion_timestamp IS NULL
--
--   PROBLEM: When INSERT AddressGroupBinding happens:
--     1. INSERT into address_group_bindings table
--     2. AFTER triggers fire (including trigger_update_aggregated_ags_on_binding_change)
--     3. trigger calls aggregate_service_address_groups()
--     4. Function does INNER JOIN with k8s_metadata
--     5. BUT k8s_metadata record does NOT exist yet! (created later by Go code)
--     6. JOIN fails → binding NOT included in result
--     7. Function returns EMPTY aggregated_address_groups []
--     8. Service UPDATE: [] → [] (no change!)
--     9. trigger_service_upsert_outbox sees no change → does NOT create outbox entry!
--     10. SGROUP never receives Service UPDATE → shows old/missing binding!
--
-- Evidence from test race-fix-1761999661-svc:
--   - Service CREATE at 12:21:01 → aggregated_address_groups = []
--   - Binding CREATE at 12:21:02
--   - Service UPDATE outbox at 12:21:02.707 → aggregated_address_groups = [] (WRONG! Should contain AG)
--   - Only 2 outbox entries instead of expected 3 (CREATE, UPDATE after binding CREATE, UPDATE after binding DELETE)
--
-- Impact:
--   - kubectl apply addressgroupbinding → binding created ✓
--   - trigger_update_aggregated_ags_on_binding_change fires ✓
--   - aggregate_service_address_groups() returns [] ❌ (binding not visible!)
--   - Service UPDATE: [] → [] (no change) ❌
--   - No Service UPDATE outbox entry created ❌
--   - SGROUP never receives binding → shows incorrect/stale data ❌
--
-- Solution (Migration 082):
--   Change INNER JOIN to LEFT JOIN in aggregate_service_address_groups():
--   - During INSERT: k8s_metadata doesn't exist yet → LEFT JOIN returns NULL → filter as NOT deleted
--   - During DELETE: k8s_metadata exists with deletion_timestamp → filter correctly
--   - During normal operation: k8s_metadata exists → filter soft-deleted bindings correctly
--
-- Expected Flow After Fix:
--   1. kubectl apply addressgroupbinding
--   2. INSERT into address_group_bindings
--   3. AFTER trigger fires → calls aggregate_service_address_groups()
--   4. Function uses LEFT JOIN → binding included even without k8s_metadata
--   5. WHERE clause: (m.deletion_timestamp IS NULL OR m.deletion_timestamp IS NULL) → TRUE
--   6. Function returns [AG] ✓
--   7. Service UPDATE: [] → [AG] (change detected!)
--   8. trigger_service_upsert_outbox creates Service UPDATE outbox entry ✓
--   9. OutboxWorker syncs to SGROUP with correct payload ✓
--   10. SGROUP shows binding correctly ✓
--
-- Date: 2025-11-01
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════

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
    -- Get address_groups field, handling null case
    SELECT COALESCE(address_groups, '[]'::jsonb) INTO address_groups_field
    FROM services
    WHERE namespace = svc_namespace AND name = svc_name;

    -- Source 1: Collect AddressGroups from spec.address_groups (source = 'spec')
    IF address_groups_field IS NOT NULL AND address_groups_field != 'null'::jsonb
       AND jsonb_typeof(address_groups_field) = 'array' AND jsonb_array_length(address_groups_field) > 0 THEN
        FOR ag_ref IN
            SELECT jsonb_array_elements(address_groups_field) as ag_obj
        LOOP
            -- Add AddressGroup reference with source information
            aggregated_ags_json := aggregated_ags_json || jsonb_build_array(
                jsonb_build_object(
                    'ref', ag_ref,
                    'source', 'spec'
                )
            );
        END LOOP;
    END IF;

    -- Source 2: Collect AddressGroups from AddressGroupBindings (source = 'binding')
    -- ✨ CRITICAL FIX (Migration 082): Changed INNER JOIN to LEFT JOIN
    -- This allows bindings without k8s_metadata (during INSERT) to be included
    FOR ag_record IN
        SELECT ag.namespace, ag.name
        FROM address_group_bindings agb
        JOIN address_groups ag ON ag.namespace = agb.address_group_namespace
                              AND ag.name = agb.address_group_name
        LEFT JOIN k8s_metadata m ON m.resource_version = agb.resource_version  -- ← Changed to LEFT JOIN
        WHERE agb.service_namespace = svc_namespace::namespace_name
        AND agb.service_name = svc_name::resource_name
        AND (m.deletion_timestamp IS NULL OR m.resource_version IS NULL)  -- ← Updated: include if no metadata OR not deleted
    LOOP
        -- Add AddressGroup reference with source information
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

-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
-- Verification and summary
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════

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

-- Revert to Migration 078 version (INNER JOIN - INSERT binding bug returns)
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
        JOIN k8s_metadata m ON m.resource_version = agb.resource_version  -- ← INNER JOIN (bug!)
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
