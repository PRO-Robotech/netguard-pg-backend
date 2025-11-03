-- +goose Up
-- Migration 053: Change NetworkBinding FK to RESTRICT for coordinated deletion
--
-- Problem: Current FK constraint uses ON DELETE CASCADE, which causes NetworkBinding
-- to be physically deleted BEFORE the coordinated deletion trigger can create outbox
-- entries. This breaks the coordinated deletion flow from Migration 052.
--
-- Solution: Change FK constraint from CASCADE to RESTRICT. This forces the coordinated
-- deletion trigger to handle NetworkBinding cleanup explicitly via outbox entries.
--
-- Pattern: Same as Host/HostBinding relationship (also uses RESTRICT)
--
-- Impact:
-- - AddressGroup deletion will now require coordinated deletion via trigger
-- - NetworkBindings will be deleted AFTER SGROUP sync (not immediately)
-- - Proper DELETE outbox entries will be created

-- +goose StatementBegin

-- Drop existing CASCADE constraint
ALTER TABLE network_bindings
DROP CONSTRAINT IF EXISTS network_bindings_address_group_namespace_address_group_name_fkey;

-- Add new RESTRICT constraint
ALTER TABLE network_bindings
ADD CONSTRAINT network_bindings_address_group_namespace_address_group_name_fkey
FOREIGN KEY (address_group_namespace, address_group_name)
REFERENCES address_groups(namespace, name)
ON DELETE RESTRICT;

COMMENT ON CONSTRAINT network_bindings_address_group_namespace_address_group_name_fkey
ON network_bindings IS
'RESTRICT ensures coordinated deletion: trigger must delete NetworkBindings via outbox before AG deletion.
See Migration 052 for coordinated deletion implementation.';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Restore CASCADE constraint
ALTER TABLE network_bindings
DROP CONSTRAINT IF EXISTS network_bindings_address_group_namespace_address_group_name_fkey;

ALTER TABLE network_bindings
ADD CONSTRAINT network_bindings_address_group_namespace_address_group_name_fkey
FOREIGN KEY (address_group_namespace, address_group_name)
REFERENCES address_groups(namespace, name)
ON DELETE CASCADE;

-- +goose StatementEnd
