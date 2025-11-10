-- +goose Up

-- +goose StatementBegin

DROP TRIGGER IF EXISTS address_group_binding_on_deletion_mark ON k8s_metadata;

DROP FUNCTION IF EXISTS trigger_address_group_binding_on_deletion_mark();

DO $$
DECLARE
    v_trigger_count INTEGER;
    v_remaining_trigger TEXT;
BEGIN
    SELECT COUNT(*) INTO v_trigger_count
    FROM pg_trigger t
    JOIN pg_proc p ON t.tgfoid = p.oid
    WHERE tgrelid = 'k8s_metadata'::regclass
      AND (p.proname LIKE '%addressgroupbinding%' OR p.proname LIKE '%address_group_binding%')
      AND t.tgname LIKE '%addressgroupbinding%';

    IF v_trigger_count = 0 THEN
        RAISE EXCEPTION '[Migration 080] FAILED: No AddressGroupBinding trigger found after dropping OLD trigger!';
    ELSIF v_trigger_count > 1 THEN
        RAISE EXCEPTION '[Migration 080] FAILED: Multiple AddressGroupBinding triggers still exist after dropping OLD trigger!';
    ELSE
        SELECT t.tgname INTO v_remaining_trigger
        FROM pg_trigger t
        JOIN pg_proc p ON t.tgfoid = p.oid
        WHERE tgrelid = 'k8s_metadata'::regclass
          AND p.proname LIKE '%addressgroupbinding%'
          AND t.tgname LIKE '%addressgroupbinding%';

        RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
        RAISE NOTICE '[Migration 080] Duplicate trigger cleanup COMPLETE';
        RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
        RAISE NOTICE '';
        RAISE NOTICE 'DROPPED:';
        RAISE NOTICE '  - Trigger: address_group_binding_on_deletion_mark (OLD, Migration 072/073)';
        RAISE NOTICE '  - Function: trigger_address_group_binding_on_deletion_mark() (OLD)';
        RAISE NOTICE '';
        RAISE NOTICE 'REMAINING:';
        RAISE NOTICE '  - Trigger: % (Migration 078)', v_remaining_trigger;
        RAISE NOTICE '  - Function: trigger_addressgroupbinding_on_deletion_mark() (Migration 078)';
        RAISE NOTICE '';
        RAISE NOTICE 'Result: Migration 078 multi-tier fallback strategy now active!';
        RAISE NOTICE '  ✓ Trigger will fire when binding deleted';
        RAISE NOTICE '  ✓ Service ALWAYS updated (table → outbox → fallback)';
        RAISE NOTICE '  ✓ Service UPDATE outbox entry ALWAYS created';
        RAISE NOTICE '  ✓ SGROUP receives correct empty aggregated_address_groups';
        RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    END IF;
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin


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
            RAISE NOTICE 'AddressGroupBinding deletion marked: service=%.%, ag=%/%',
                v_service_namespace, v_service_name, v_ag_namespace, v_ag_name;

            UPDATE services
            SET aggregated_address_groups = aggregate_service_address_groups(
                v_service_namespace::text,
                v_service_name::text
            )
            WHERE namespace = v_service_namespace
              AND name = v_service_name;

            RAISE NOTICE 'Service %.% updated: aggregated_address_groups recalculated (binding excluded)',
                v_service_namespace, v_service_name;

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
            ON CONFLICT DO NOTHING;

        ELSE
            RAISE WARNING 'AddressGroupBinding not found for resource_version %', NEW.resource_version;
        END IF;

    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER address_group_binding_on_deletion_mark
    AFTER UPDATE OF deletion_timestamp ON k8s_metadata
    FOR EACH ROW
    WHEN (NEW.deletion_timestamp IS NOT NULL AND OLD.deletion_timestamp IS NULL)
    EXECUTE FUNCTION trigger_address_group_binding_on_deletion_mark();

COMMENT ON FUNCTION trigger_address_group_binding_on_deletion_mark() IS
'(OLD - Migration 072/073) Handles AddressGroupBinding coordinated deletion when deletion_timestamp is set.
Updates Service.aggregated_address_groups which triggers Service UPDATE and SGROUP sync.
WARNING: This version only updates Service if binding found in table (race condition bug).
Use Migration 078 version instead (trigger_addressgroupbinding_on_deletion_mark).';

DO $$
BEGIN
    RAISE WARNING '[Migration 080 Rollback] Restored OLD duplicate trigger - Migration 078 fix will NOT work!';
    RAISE WARNING 'Recommended: Re-apply Migration 080 to use Migration 078 improved version.';
END $$;

-- +goose StatementEnd
