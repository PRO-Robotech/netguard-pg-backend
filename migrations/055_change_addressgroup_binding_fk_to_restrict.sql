-- +goose Up
-- +goose StatementBegin


ALTER TABLE address_group_bindings
DROP CONSTRAINT IF EXISTS address_group_bindings_address_group_namespace_address_group_name_fkey;

ALTER TABLE address_group_bindings
ADD CONSTRAINT address_group_bindings_address_group_namespace_address_group_name_fkey
    FOREIGN KEY (address_group_namespace, address_group_name)
    REFERENCES address_groups(namespace, name)
    ON DELETE RESTRICT;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE address_group_bindings
DROP CONSTRAINT IF EXISTS address_group_bindings_address_group_namespace_address_group_name_fkey;

ALTER TABLE address_group_bindings
ADD CONSTRAINT address_group_bindings_address_group_namespace_address_group_name_fkey
    FOREIGN KEY (address_group_namespace, address_group_name)
    REFERENCES address_groups(namespace, name)
    ON DELETE CASCADE;

-- +goose StatementEnd
