-- +goose Up
-- Add CIDR column with uniqueness constraint for Network resources
-- This prevents duplicate CIDR definitions at the database level

-- Step 1: Add the cidr column (nullable initially for migration)
ALTER TABLE networks ADD COLUMN cidr CIDR;

-- Step 2: Populate the cidr column from existing network_items JSONB data
-- Extract CIDR from the first (and only) element in network_items array
UPDATE networks 
SET cidr = (network_items->0->>'cidr')::CIDR 
WHERE network_items IS NOT NULL 
  AND jsonb_array_length(network_items) > 0
  AND network_items->0->>'cidr' IS NOT NULL;

-- Step 3: Make the column NOT NULL after data migration
ALTER TABLE networks ALTER COLUMN cidr SET NOT NULL;

-- Step 4: Create unique index on cidr to enforce uniqueness
CREATE UNIQUE INDEX idx_networks_cidr_unique ON networks(cidr);

-- Step 5: Add comment to document the constraint purpose
COMMENT ON COLUMN networks.cidr IS 'Network CIDR block - must be unique across all networks';
COMMENT ON INDEX idx_networks_cidr_unique IS 'Ensures CIDR uniqueness across all networks to prevent conflicts';

-- +goose Down
-- Rollback: remove cidr column and related constraints

-- Drop the unique index
DROP INDEX IF EXISTS idx_networks_cidr_unique;

-- Drop the cidr column
ALTER TABLE networks DROP COLUMN IF EXISTS cidr;