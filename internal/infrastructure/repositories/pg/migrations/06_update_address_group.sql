-- Migration 06: Update AddressGroup table to support Networks and AddressGroupName
-- Add Networks field as JSONB and AddressGroupName field

-- Add new columns to tbl_address_group
ALTER TABLE netguard.tbl_address_group 
ADD COLUMN networks JSONB DEFAULT '[]'::jsonb,
ADD COLUMN address_group_name TEXT;

-- Create index for Networks JSONB field for better query performance
CREATE INDEX IF NOT EXISTS idx_address_group_networks ON netguard.tbl_address_group USING GIN (networks);

-- Create index for AddressGroupName field
CREATE INDEX IF NOT EXISTS idx_address_group_name ON netguard.tbl_address_group (address_group_name);

-- Add comment to document the new fields
COMMENT ON COLUMN netguard.tbl_address_group.networks IS 'Networks associated with this address group (JSON array of NetworkItem objects)';
COMMENT ON COLUMN netguard.tbl_address_group.address_group_name IS 'Name used in sgroups synchronization';