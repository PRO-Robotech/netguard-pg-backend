-- +goose Up
-- Fix RuleS2S schema to match current Go code expectations
-- Migrate from old flat column structure to JSONB format for ObjectReferences

-- Add new JSONB columns for ObjectReference fields and missing trace column
ALTER TABLE rule_s2s 
ADD COLUMN service_local_ref JSONB,
ADD COLUMN service_ref JSONB,
ADD COLUMN ieagag_rule_refs JSONB DEFAULT '[]'::JSONB,
ADD COLUMN trace BOOLEAN DEFAULT FALSE;

-- Migrate existing data from flat columns to JSONB format
-- service_local_ref: Create NamespacedObjectReference structure
UPDATE rule_s2s SET 
    service_local_ref = json_build_object(
        'apiVersion', 'netguard.sgroups.io/v1beta1',
        'kind', 'ServiceAlias',
        'name', service_local_name,
        'namespace', service_local_namespace
    )
WHERE service_local_name IS NOT NULL AND service_local_namespace IS NOT NULL;

-- service_ref: Create NamespacedObjectReference structure  
UPDATE rule_s2s SET
    service_ref = json_build_object(
        'apiVersion', 'netguard.sgroups.io/v1beta1',
        'kind', 'ServiceAlias', 
        'name', service_name,
        'namespace', service_namespace
    )
WHERE service_name IS NOT NULL AND service_namespace IS NOT NULL;

-- Set empty array for ieagag_rule_refs (populated by autogeneration logic)
UPDATE rule_s2s SET ieagag_rule_refs = '[]'::JSONB WHERE ieagag_rule_refs IS NULL;

-- Make new JSONB columns NOT NULL after data migration
ALTER TABLE rule_s2s 
ALTER COLUMN service_local_ref SET NOT NULL,
ALTER COLUMN service_ref SET NOT NULL,
ALTER COLUMN ieagag_rule_refs SET NOT NULL;

-- Create indexes on JSONB fields for better query performance (B-tree indexes for text extractions)
CREATE INDEX idx_rule_s2s_service_local_ref_name ON rule_s2s USING btree ((service_local_ref->>'name'));
CREATE INDEX idx_rule_s2s_service_local_ref_namespace ON rule_s2s USING btree ((service_local_ref->>'namespace'));
CREATE INDEX idx_rule_s2s_service_ref_name ON rule_s2s USING btree ((service_ref->>'name'));
CREATE INDEX idx_rule_s2s_service_ref_namespace ON rule_s2s USING btree ((service_ref->>'namespace'));

-- Drop old flat columns now that data is migrated to JSONB
ALTER TABLE rule_s2s 
DROP COLUMN service_local_namespace,
DROP COLUMN service_local_name,
DROP COLUMN service_namespace,
DROP COLUMN service_name;

-- +goose Down  
-- Revert RuleS2S schema back to flat column structure

-- Add back the old flat columns
ALTER TABLE rule_s2s
ADD COLUMN service_local_namespace namespace_name,
ADD COLUMN service_local_name resource_name,
ADD COLUMN service_namespace namespace_name,
ADD COLUMN service_name resource_name;

-- Migrate data back from JSONB to flat columns
UPDATE rule_s2s SET
    service_local_namespace = (service_local_ref->>'namespace')::namespace_name,
    service_local_name = (service_local_ref->>'name')::resource_name,
    service_namespace = (service_ref->>'namespace')::namespace_name,
    service_name = (service_ref->>'name')::resource_name;

-- Make old flat columns NOT NULL
ALTER TABLE rule_s2s
ALTER COLUMN service_local_namespace SET NOT NULL,
ALTER COLUMN service_local_name SET NOT NULL,
ALTER COLUMN service_namespace SET NOT NULL,
ALTER COLUMN service_name SET NOT NULL;

-- Drop indexes and JSONB columns
DROP INDEX IF EXISTS idx_rule_s2s_service_local_ref_name;
DROP INDEX IF EXISTS idx_rule_s2s_service_local_ref_namespace;
DROP INDEX IF EXISTS idx_rule_s2s_service_ref_name;
DROP INDEX IF EXISTS idx_rule_s2s_service_ref_namespace;

ALTER TABLE rule_s2s
DROP COLUMN service_local_ref,
DROP COLUMN service_ref,
DROP COLUMN ieagag_rule_refs,
DROP COLUMN trace;