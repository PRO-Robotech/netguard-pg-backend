-- +goose Up
-- Add specific reference columns for AddressGroupBindingPolicy to support efficient querying
-- This replaces the generic policy_data JSONB with structured columns

-- Add new reference columns
ALTER TABLE address_group_binding_policies 
ADD COLUMN address_group_ref JSONB DEFAULT '{}',
ADD COLUMN service_ref JSONB DEFAULT '{}';

-- Create indexes for efficient querying
CREATE INDEX idx_address_group_binding_policies_ag_ref ON address_group_binding_policies USING GIN(address_group_ref);
CREATE INDEX idx_address_group_binding_policies_service_ref ON address_group_binding_policies USING GIN(service_ref);

-- Migrate existing data from policy_data to structured columns (if any exists)
-- Note: Since this is a new feature, there likely won't be any existing data
UPDATE address_group_binding_policies 
SET 
    address_group_ref = COALESCE((policy_data->>'addressGroupRef')::jsonb, '{}'),
    service_ref = COALESCE((policy_data->>'serviceRef')::jsonb, '{}')
WHERE policy_data IS NOT NULL AND policy_data != '{}';

-- Remove the old generic policy_data column
ALTER TABLE address_group_binding_policies DROP COLUMN policy_data;

-- +goose Down
-- Restore the original policy_data column and migrate data back

-- Add back the policy_data column
ALTER TABLE address_group_binding_policies 
ADD COLUMN policy_data JSONB NOT NULL DEFAULT '{}';

-- Migrate data back to policy_data
UPDATE address_group_binding_policies 
SET policy_data = jsonb_build_object(
    'addressGroupRef', address_group_ref,
    'serviceRef', service_ref
);

-- Drop the structured columns
ALTER TABLE address_group_binding_policies 
DROP COLUMN address_group_ref,
DROP COLUMN service_ref;

-- Drop indexes
DROP INDEX IF EXISTS idx_address_group_binding_policies_ag_ref;
DROP INDEX IF EXISTS idx_address_group_binding_policies_service_ref;