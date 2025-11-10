-- +goose Up
-- +goose StatementBegin

ALTER TABLE network_bindings
ADD CONSTRAINT network_bindings_network_unique
UNIQUE (network_namespace, network_name);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE network_bindings
DROP CONSTRAINT IF EXISTS network_bindings_network_unique;

-- +goose StatementEnd
