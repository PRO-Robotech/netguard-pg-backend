-- +goose Up
-- +goose StatementBegin

-- Remove hostname column from hosts table
ALTER TABLE hosts DROP COLUMN IF EXISTS hostname;

-- +goose StatementEnd

-- +goose Down 
-- +goose StatementBegin

-- Re-add hostname column in case of rollback
-- Note: This will be empty for all existing records
ALTER TABLE hosts ADD COLUMN hostname TEXT;

-- +goose StatementEnd