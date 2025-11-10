-- +goose Up

-- +goose StatementBegin

ALTER TABLE address_group_bindings
DROP CONSTRAINT IF EXISTS address_group_bindings_resource_version_fkey;

ALTER TABLE address_group_bindings
ADD CONSTRAINT address_group_bindings_resource_version_fkey
FOREIGN KEY (resource_version) REFERENCES k8s_metadata(resource_version) ON DELETE RESTRICT;

COMMENT ON CONSTRAINT address_group_bindings_resource_version_fkey ON address_group_bindings IS
'Migration 077: Changed from CASCADE to RESTRICT to fix timing issue.
Physical deletion happens via OutboxWorker AFTER successful SGROUP synchronization.
This ensures AFTER UPDATE trigger on k8s_metadata can read binding details before deletion.';

DO $$
BEGIN
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '[Migration 077] AddressGroupBinding CASCADE timing fix COMPLETE';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
    RAISE NOTICE '';
    RAISE NOTICE 'FIXED: Foreign Key constraint changed from CASCADE to RESTRICT';
    RAISE NOTICE '';
    RAISE NOTICE 'New deletion flow:';
    RAISE NOTICE '  1. kubectl delete → deletion_timestamp set in k8s_metadata';
    RAISE NOTICE '  2. AFTER UPDATE trigger fires → binding STILL EXISTS';
    RAISE NOTICE '  3. Trigger reads binding, updates Service.aggregated_address_groups';
    RAISE NOTICE '  4. Service UPDATE trigger creates SGROUP outbox entry';
    RAISE NOTICE '  5. OutboxWorker syncs to SGROUP';
    RAISE NOTICE '  6. OutboxWorker deletes binding AFTER successful sync ✓';
    RAISE NOTICE '';
    RAISE NOTICE 'Result: SGROUP receives correct UPDATE with empty aggregated_address_groups!';
    RAISE NOTICE '════════════════════════════════════════════════════════════════════════════════════════';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE address_group_bindings
DROP CONSTRAINT IF EXISTS address_group_bindings_resource_version_fkey;

ALTER TABLE address_group_bindings
ADD CONSTRAINT address_group_bindings_resource_version_fkey
FOREIGN KEY (resource_version) REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE;

DO $$
BEGIN
    RAISE WARNING '[Migration 077 Rollback] Reverted to CASCADE - deletion timing bug will return!';
END $$;

-- +goose StatementEnd
