-- +goose Up
-- +goose StatementBegin
ALTER TABLE address_groups
ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
COMMENT ON COLUMN address_groups.description IS
    'Optional human-readable description (Netguard-only, not synced to SGROUPS)';
-- +goose StatementEnd

-- +goose Down
ALTER TABLE address_groups
DROP COLUMN IF EXISTS description;

