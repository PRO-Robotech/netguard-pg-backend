-- +goose Up
-- +goose StatementBegin

-- =====================================================
-- Migration 055: Change AddressGroup FK to RESTRICT
-- =====================================================
-- Purpose: Enable coordinated deletion for AddressGroup → AddressGroupBindings
-- Pattern: Identical to Migration 053 (NetworkBinding FK change)
--
-- Problem:
--   Current FK constraint uses CASCADE:
--   FOREIGN KEY (address_group_namespace, address_group_name)
--       REFERENCES address_groups(namespace, name) ON DELETE CASCADE
--
--   This causes physical deletion of bindings BEFORE coordinated deletion trigger can run.
--   Result: No DELETE outbox entries created for SGROUP sync.
--
-- Solution:
--   Change FK from CASCADE to RESTRICT.
--   This allows AFTER UPDATE trigger (Migration 056) to create outbox entries
--   BEFORE physical deletion.
--
-- Flow after this migration:
--   1. User deletes AddressGroup
--   2. BEFORE DELETE trigger sets deletion_timestamp
--   3. AFTER UPDATE trigger (Migration 056) creates DELETE outbox entries for bindings
--   4. OutboxWorker processes entries
--   5. Physical deletion after SGROUP sync
--
-- Related Migrations:
--   - Migration 053: NetworkBinding FK change (same pattern)
--   - Migration 056: AddressGroup coordinated deletion trigger (next)
--
-- Date: 2025-10-31
-- =====================================================

-- Drop existing CASCADE FK constraint
ALTER TABLE address_group_bindings
DROP CONSTRAINT IF EXISTS address_group_bindings_address_group_namespace_address_group_name_fkey;

-- Add new RESTRICT FK constraint
ALTER TABLE address_group_bindings
ADD CONSTRAINT address_group_bindings_address_group_namespace_address_group_name_fkey
    FOREIGN KEY (address_group_namespace, address_group_name)
    REFERENCES address_groups(namespace, name)
    ON DELETE RESTRICT;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to CASCADE FK constraint
ALTER TABLE address_group_bindings
DROP CONSTRAINT IF EXISTS address_group_bindings_address_group_namespace_address_group_name_fkey;

ALTER TABLE address_group_bindings
ADD CONSTRAINT address_group_bindings_address_group_namespace_address_group_name_fkey
    FOREIGN KEY (address_group_namespace, address_group_name)
    REFERENCES address_groups(namespace, name)
    ON DELETE CASCADE;

-- +goose StatementEnd
