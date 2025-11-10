-- +goose Up
-- +goose StatementBegin

ALTER TABLE host_bindings
ADD CONSTRAINT host_bindings_host_unique
UNIQUE (host_namespace, host_name);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE host_bindings
DROP CONSTRAINT IF EXISTS host_bindings_host_unique;

-- +goose StatementEnd
