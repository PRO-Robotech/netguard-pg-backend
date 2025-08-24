-- Migration to add Networks field to AddressGroups table
-- This fixes the critical bug where AddressGroup.Networks field exists in domain model and K8s API
-- but is never persisted to the database, causing empty Networks in kubectl output and sgroups sync

-- +goose Up
-- Add networks JSONB column to address_groups table
ALTER TABLE address_groups ADD COLUMN networks JSONB NOT NULL DEFAULT '[]';

-- Create GIN index for networks field performance (since it's JSONB)
CREATE INDEX idx_address_groups_networks ON address_groups USING GIN(networks);

-- Add comment explaining the field
COMMENT ON COLUMN address_groups.networks IS 'NetworkItem[] - List of networks bound to this AddressGroup via NetworkBindings';

-- +goose Down
-- Remove networks column and index
DROP INDEX IF EXISTS idx_address_groups_networks;
ALTER TABLE address_groups DROP COLUMN IF EXISTS networks;