-- +goose Up
-- Optimize k8s_metadata table for concurrent condition updates
-- This addresses the PostgreSQL timeout issues during condition processing

-- Add optimized index for condition updates with updated_at for efficient querying
CREATE INDEX IF NOT EXISTS idx_k8s_metadata_conditions_updated_at 
    ON k8s_metadata (updated_at DESC, resource_version) 
    WHERE conditions != '[]'::jsonb;

-- Add partial index for resources that frequently need condition updates (non-empty conditions)
CREATE INDEX IF NOT EXISTS idx_k8s_metadata_active_conditions 
    ON k8s_metadata (resource_version) 
    WHERE conditions != '[]'::jsonb AND conditions IS NOT NULL;

-- Add index for concurrent condition update pattern (resource_version lookup optimization)
CREATE INDEX IF NOT EXISTS idx_k8s_metadata_resource_version_conditions 
    ON k8s_metadata (resource_version, updated_at DESC);

-- Ensure all conditions are proper JSON arrays before creating expression index
-- This fixes the "cannot get array length of a scalar" error
UPDATE k8s_metadata 
SET conditions = '[]'::jsonb 
WHERE conditions IS NULL OR jsonb_typeof(conditions) != 'array';

-- Optimize condition update queries by creating expression index (with type safety)
CREATE INDEX IF NOT EXISTS idx_k8s_metadata_conditions_size 
    ON k8s_metadata (jsonb_array_length(conditions), resource_version)
    WHERE conditions IS NOT NULL AND jsonb_typeof(conditions) = 'array';

-- +goose Down
-- Remove the condition optimization indexes

DROP INDEX IF EXISTS idx_k8s_metadata_conditions_updated_at;
DROP INDEX IF EXISTS idx_k8s_metadata_active_conditions;
DROP INDEX IF EXISTS idx_k8s_metadata_resource_version_conditions;
DROP INDEX IF EXISTS idx_k8s_metadata_conditions_size;