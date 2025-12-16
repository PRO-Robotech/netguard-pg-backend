-- +goose Up
-- +goose StatementBegin
-- Add meta_info field to hosts table for Host metadata synchronization from SGROUP
-- Structure: {"hostName": "...", "os": "...", "platform": "...", "platformFamily": "...", "platformVersion": "...", "kernelVersion": "..."}

-- Add meta_info column as JSONB object
ALTER TABLE hosts ADD COLUMN meta_info JSONB DEFAULT NULL;

-- Create GIN index for efficient querying of meta_info fields
CREATE INDEX idx_hosts_meta_info ON hosts USING gin(meta_info);

-- Add constraint to ensure meta_info is either null or a JSON object
ALTER TABLE hosts ADD CONSTRAINT check_meta_info_is_object
    CHECK (meta_info IS NULL OR jsonb_typeof(meta_info) = 'object');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_hosts_meta_info;
ALTER TABLE hosts DROP CONSTRAINT IF EXISTS check_meta_info_is_object;
ALTER TABLE hosts DROP COLUMN IF EXISTS meta_info;
-- +goose StatementEnd
