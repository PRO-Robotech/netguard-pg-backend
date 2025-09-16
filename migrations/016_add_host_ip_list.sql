-- +goose Up
-- Add ipList field to hosts table for Host IP synchronization from SGROUP

-- Add ip_list column as JSONB array of IP items
-- Structure: [{"ip": "192.168.1.1"}, {"ip": "10.0.0.1"}, ...]
ALTER TABLE hosts ADD COLUMN ip_list JSONB DEFAULT NULL;

-- Create index for efficient querying of IP lists
CREATE INDEX idx_hosts_ip_list ON hosts USING gin(ip_list);

-- Add constraint to ensure ip_list is either null or an array
ALTER TABLE hosts ADD CONSTRAINT check_ip_list_is_array
    CHECK (ip_list IS NULL OR jsonb_typeof(ip_list) = 'array');

-- +goose Down
DROP INDEX IF EXISTS idx_hosts_ip_list;
ALTER TABLE hosts DROP COLUMN IF EXISTS ip_list;