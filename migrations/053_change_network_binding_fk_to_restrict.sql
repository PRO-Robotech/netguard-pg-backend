-- +goose Up

-- +goose StatementBegin

ALTER TABLE network_bindings
DROP CONSTRAINT IF EXISTS network_bindings_address_group_namespace_address_group_name_fkey;

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

ALTER TABLE network_bindings
DROP CONSTRAINT IF EXISTS network_bindings_address_group_namespace_address_group_name_fkey;

ALTER TABLE network_bindings
ADD CONSTRAINT network_bindings_address_group_namespace_address_group_name_fkey
FOREIGN KEY (address_group_namespace, address_group_name)
REFERENCES address_groups(namespace, name)
ON DELETE CASCADE;

-- +goose StatementEnd
