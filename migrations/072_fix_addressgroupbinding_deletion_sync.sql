-- +goose Up
-- Migration 072: Fix AddressGroupBinding deletion SGROUP sync
--
-- =====================================================================================================================
-- CRITICAL BUG FIX: AddressGroupBinding deletion не синхронизируется с SGROUP
-- =====================================================================================================================
--
-- Problem:
--   При удалении AddressGroupBinding:
--   - База данных обновляется: Service.aggregated_address_groups = [] ✅
--   - НО SGROUP НЕ получает UPDATE: sgNames остается со старыми данными ❌
--
-- Root Cause:
--   AddressGroupBinding НЕ следует паттерну HostBinding (Migration 044) для обновления агрегированных полей.
--
--   Проблема №1 (Migration 018):
--     aggregate_service_address_groups() НЕ фильтрует удаленные bindings
--     - Отсутствует: JOIN k8s_metadata m
--     - Отсутствует: WHERE m.deletion_timestamp IS NULL
--     → Deleted binding все еще появляется в aggregated_address_groups
--
--   Проблема №2 (Migration 066):
--     trigger_addressgroupbinding_on_deletion_mark() НЕ обновляет Service.aggregated_address_groups
--     - Создает только DELETE outbox entry (target=INTERNAL) ✅
--     - НЕ вызывает пересчет aggregated_address_groups ❌
--     → Service UPDATE trigger не срабатывает (нет изменений)
--     → Outbox entry для SGROUP не создается
--
-- Comparison with HostBinding (Migration 044 - CORRECT implementation):
--   ✅ aggregate_address_group_hosts() ФИЛЬТРУЕТ deleted bindings (line 69-72)
--   ✅ trigger_host_binding_on_deletion_mark() ОБНОВЛЯЕТ aggregated_hosts (line 279-282)
--   ✅ AddressGroup UPDATE trigger СОЗДАЕТ SGROUP outbox entry
--   ✅ OutboxWorker синхронизирует изменения в SGROUP
--
-- Solution:
--   Fix 1: Обновить aggregate_service_address_groups() - добавить фильтр deletion_timestamp IS NULL
--   Fix 2: Обновить trigger_addressgroupbinding_on_deletion_mark() - добавить UPDATE services
--
-- Expected Flow After Fix:
--   1. User: DELETE AddressGroupBinding
--   2. BEFORE DELETE trigger: set deletion_timestamp
--   3. AFTER UPDATE (deletion_timestamp): trigger_addressgroupbinding_on_deletion_mark()
--      ├─ Create DELETE outbox (target=INTERNAL) ✅
--      └─ Update Service.aggregated_address_groups ← aggregate_service_address_groups() ФИЛЬТРУЕТ deleted binding ✅
--   4. AFTER UPDATE (aggregated_address_groups): trigger_service_upsert_outbox() (Migration 070)
--      └─ Create UPDATE outbox (target=SGROUP) ✅
--   5. OutboxWorker: Process SGROUP UPDATE with empty array ✅
--
-- Related:
--   - Migration 018: aggregate_service_address_groups() (ИСПРАВЛЯЕТСЯ)
--   - Migration 044: HostBinding deletion pattern (REFERENCE)
--   - Migration 066: AddressGroupBinding deletion trigger (ИСПРАВЛЯЕТСЯ)
--   - Migration 070: Service upsert trigger with aggregated_address_groups check
--   - Migration 071: processed_at backfill (separate bug)
--
-- Date: 2025-10-31
-- =====================================================================================================================

-- +goose StatementBegin

-- ═══════════════════════════════════════════════════════════════════════════════════════════════════════════════
-- FIX 1: Update aggregate_service_address_groups() to FILTER deleted AddressGroupBindings
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════════════════
-- Pattern: Similar to Migration 044 aggregate_address_group_hosts() lines 69-72

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
    -- ✨ CRITICAL CHANGE: Added k8s_metadata JOIN and deletion_timestamp filter
    FOR ag_record IN
        SELECT ag.namespace, ag.name
        FROM address_group_bindings agb
        JOIN address_groups ag ON ag.namespace = agb.address_group_namespace
                              AND ag.name = agb.address_group_name
        JOIN k8s_metadata m ON m.resource_version = agb.resource_version  -- ← ADDED
        WHERE agb.service_namespace = svc_namespace::namespace_name
        AND agb.service_name = svc_name::resource_name
        AND m.deletion_timestamp IS NULL  -- ← ADDED: Filter deleted bindings
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
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION aggregate_service_address_groups(TEXT, TEXT) IS
'Aggregates AddressGroups for a Service from TWO sources:
1. spec.addressGroups (source=spec)
2. AddressGroupBindings (source=binding)
NOW FILTERS deleted bindings (Migration 072 fix) - only includes bindings with deletion_timestamp IS NULL.
Similar to aggregate_address_group_hosts() in Migration 044.';

-- ═══════════════════════════════════════════════════════════════════════════════════════════════════════════════
-- FIX 2: Update trigger_addressgroupbinding_on_deletion_mark() to UPDATE Service.aggregated_address_groups
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════════════════
-- Pattern: Similar to Migration 044 trigger_host_binding_on_deletion_mark() lines 279-282

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
            RAISE NOTICE '[Migration 066/072] AddressGroupBinding deletion marked: service=%.%, ag=%.%',
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

            RAISE NOTICE '[Migration 066/072] Created DELETE outbox entry for AddressGroupBinding %.%',
                v_binding_namespace, v_binding_name;

            -- ✨ CRITICAL CHANGE (Migration 072): Update Service.aggregated_address_groups
            -- This triggers Service UPDATE trigger (Migration 070) which creates SGROUP outbox entry
            -- Pattern: Similar to Migration 044 lines 279-282 (trigger_host_binding_on_deletion_mark)
            UPDATE services
            SET aggregated_address_groups = aggregate_service_address_groups(
                v_service_namespace::text,
                v_service_name::text
            )
            WHERE namespace = v_service_namespace AND name = v_service_name;

            RAISE NOTICE '[Migration 072] Updated Service %.% aggregated_address_groups (excluded deleted binding)',
                v_service_namespace, v_service_name;

        ELSE
            -- AddressGroupBinding not found - may have been deleted already
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

-- ═══════════════════════════════════════════════════════════════════════════════════════════════════════════════
-- Verification and summary
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════════════════

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

-- Revert to Migration 066/018 versions WITHOUT fixes

-- Revert aggregate_service_address_groups() - remove deletion_timestamp filter
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

    -- ❌ Reverted: No deletion_timestamp filter
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

-- Revert trigger_addressgroupbinding_on_deletion_mark() - remove Service UPDATE
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

            -- ❌ Reverted: No Service UPDATE
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
