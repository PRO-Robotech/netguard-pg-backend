-- +goose Up
-- +goose StatementBegin
ALTER TABLE hosts ADD COLUMN meta_info JSONB DEFAULT NULL;
CREATE INDEX idx_hosts_meta_info ON hosts USING gin(meta_info);
ALTER TABLE hosts ADD CONSTRAINT check_meta_info_is_object
    CHECK (meta_info IS NULL OR jsonb_typeof(meta_info) = 'object');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_hosts_meta_info;
ALTER TABLE hosts DROP CONSTRAINT IF EXISTS check_meta_info_is_object;
ALTER TABLE hosts DROP COLUMN IF EXISTS meta_info;
-- +goose StatementEnd
