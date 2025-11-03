-- +goose Up
-- Migration 078: Fix AddressGroupBinding deletion - ALWAYS update Service
--
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
-- CRITICAL BUG FIX: Service UPDATE outbox entry NOT created when AddressGroupBinding deleted
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
--
-- Root Cause (discovered 2025-10-31):
--   Migration 072 trigger_addressgroupbinding_on_deletion_mark() reads binding from table:
--
--   SELECT service_namespace, service_name, ...
--   FROM address_group_bindings
--   WHERE resource_version = NEW.resource_version;
--
--   v_binding_found := FOUND;
--
--   IF v_binding_found THEN
--       -- Update Service.aggregated_address_groups ✅
--   ELSE
--       -- Service NOT updated ❌ ← BUG!
--   END IF;
--
--   PROBLEM: By the time AFTER UPDATE trigger fires, binding is ALREADY physically deleted
--            from address_group_bindings table (asynchronously by OutboxWorker or other process).
--            PostgreSQL logs show: "WARNING: AddressGroupBinding not found for resource_version XXX"
--            Result: v_binding_found = FALSE → Service UPDATE does NOT happen!
--
-- Impact:
--   1. kubectl delete addressgroupbinding → deletion_timestamp set in k8s_metadata ✅
--   2. Binding physically deleted from address_group_bindings (race condition) ❌
--   3. AFTER UPDATE trigger fires → binding NOT found ❌
--   4. Service.aggregated_address_groups NOT updated ❌
--   5. Service UPDATE outbox entry NOT created ❌
--   6. SGROUP never receives UPDATE → shows old binding data ❌
--
-- Solution (Migration 078):
--   Trigger should ALWAYS update Service, even if binding not found in table!
--
--   Strategy:
--   1. Try to find binding in address_group_bindings (as before)
--   2. If NOT found → extract service info from sync_outbox (previous CREATE entry)
--   3. If still not found → UPDATE ALL Services (last resort, triggers aggregate recalculation)
--   4. aggregate_service_address_groups() automatically filters deleted bindings ✅
--
-- Expected Flow After Fix:
--   1. kubectl delete addressgroupbinding
--   2. deletion_timestamp set in k8s_metadata
--   3. AFTER UPDATE trigger fires (binding may or may not exist in table)
--   4. Trigger finds Service info (from table OR from outbox)
--   5. UPDATE services SET aggregated_address_groups = aggregate_service_address_groups(...) ✅
--   6. Service UPDATE trigger creates SGROUP outbox entry ✅
--   7. OutboxWorker syncs to SGROUP with correct payload (aggregated_address_groups=[]) ✅
--
-- Date: 2025-10-31
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════

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
    -- Only act when deletion_timestamp is being set (NULL → timestamp)
    IF NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL THEN

        -- Get UID from k8s_metadata (ALWAYS available)
        v_uid := NEW.uid;

        -- ═══════════════════════════════════════════════════════════════════
        -- Step 1: Try to get AddressGroupBinding details from table
        -- ═══════════════════════════════════════════════════════════════════
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
        -- Step 2: If binding NOT found in table, try to get Service info from outbox
        -- ═══════════════════════════════════════════════════════════════════
        IF NOT v_binding_found THEN
            RAISE WARNING '[Migration 078] AddressGroupBinding not found in table for resource_version %, attempting outbox lookup', NEW.resource_version;

            -- Try to find previous CREATE/UPDATE outbox entry for this binding
            SELECT payload INTO v_outbox_payload
            FROM sync_outbox
            WHERE resource_id = v_uid
              AND resource_type = 'AddressGroupBinding'
              AND operation IN ('CREATE', 'UPDATE')
              AND target_system = 'INTERNAL'
            ORDER BY created_at DESC
            LIMIT 1;

            IF FOUND AND v_outbox_payload IS NOT NULL THEN
                -- Extract service info from payload
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

        -- ═══════════════════════════════════════════════════════════════════
        -- Step 3: Create DELETE outbox entry (using available data)
        -- ═══════════════════════════════════════════════════════════════════
        IF v_service_found THEN
            -- Get resource_versions for Service and AddressGroup
            SELECT resource_version INTO v_service_rv
            FROM services
            WHERE namespace = v_service_namespace AND name = v_service_name;

            SELECT resource_version INTO v_ag_rv
            FROM address_groups
            WHERE namespace = v_ag_namespace AND name = v_ag_name;

            -- Build affected_resources array
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

            -- Insert DELETE entry into sync_outbox
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
                    'recoveredFromOutbox', NOT v_binding_found  -- Flag indicating data source
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

            -- ═══════════════════════════════════════════════════════════════════
            -- ✨ CRITICAL FIX (Migration 078): ALWAYS update Service!
            -- ═══════════════════════════════════════════════════════════════════
            -- This ensures Service UPDATE outbox entry is created even if binding
            -- was already physically deleted from table.
            UPDATE services
            SET aggregated_address_groups = aggregate_service_address_groups(
                v_service_namespace::text,
                v_service_name::text
            )
            WHERE namespace = v_service_namespace AND name = v_service_name;

            RAISE NOTICE '[Migration 078] Updated Service %.% aggregated_address_groups (binding may be deleted)',
                v_service_namespace, v_service_name;

        ELSE
            -- ═══════════════════════════════════════════════════════════════════
            -- Last resort: Create minimal DELETE entry and trigger full Services update
            -- ═══════════════════════════════════════════════════════════════════
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

            -- ⚠️ WARNING: This updates ALL Services - inefficient but ensures correctness
            -- aggregate_service_address_groups() will filter deleted bindings automatically
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

COMMENT ON FUNCTION trigger_addressgroupbinding_on_deletion_mark() IS
'Creates DELETE outbox entry when AddressGroupBinding is marked for deletion (deletion_timestamp set).
Migration 078 fix: ALWAYS updates Service.aggregated_address_groups even if binding not found in table.
Uses multi-tier strategy: table → outbox → fallback (update all Services).
This ensures Service UPDATE outbox entry is ALWAYS created for SGROUP synchronization.';

-- ═══════════════════════════════════════════════════════════════════════════════════════════════════
-- Verification and summary
-- ═══════════════════════════════════════════════════════════════════════════════════════════════════

DO $$
BEGIN
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '[Migration 078] AddressGroupBinding Service UPDATE fix COMPLETE';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '';
    RAISE NOTICE 'FIXED: trigger_addressgroupbinding_on_deletion_mark() now ALWAYS updates Service';
    RAISE NOTICE '';
    RAISE NOTICE 'Strategy:';
    RAISE NOTICE '  1. Try to read binding from address_group_bindings table';
    RAISE NOTICE '  2. If NOT found → extract Service info from sync_outbox (previous CREATE entry)';
    RAISE NOTICE '  3. If still NOT found → update ALL Services (fallback)';
    RAISE NOTICE '  4. UPDATE services SET aggregated_address_groups = aggregate_service_address_groups(...)';
    RAISE NOTICE '  5. Service UPDATE trigger creates SGROUP outbox entry ✓';
    RAISE NOTICE '';
    RAISE NOTICE 'Result: SGROUP will ALWAYS receive correct Service UPDATE with empty aggregated_address_groups!';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to Migration 072 version (does NOT update Service if binding not found)
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
            RAISE NOTICE '[Migration 066/072] AddressGroupBinding deletion marked: service=%.%, ag=%.%',
                v_service_namespace, v_service_name, v_ag_namespace, v_ag_name;

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

            RAISE NOTICE '[Migration 066/072] Created DELETE outbox entry for AddressGroupBinding %.%',
                v_binding_namespace, v_binding_name;

            -- ❌ Migration 072 version: Service UPDATE only if binding found
            UPDATE services
            SET aggregated_address_groups = aggregate_service_address_groups(
                v_service_namespace::text,
                v_service_name::text
            )
            WHERE namespace = v_service_namespace AND name = v_service_name;

            RAISE NOTICE '[Migration 072] Updated Service %.% aggregated_address_groups (excluded deleted binding)',
                v_service_namespace, v_service_name;

        ELSE
            RAISE WARNING '[Migration 066/072] AddressGroupBinding not found for resource_version %',
                NEW.resource_version;
        END IF;

    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    RAISE WARNING '[Migration 078 Rollback] Reverted to Migration 072 - Service UPDATE bug will return!';
END $$;

-- +goose StatementEnd
