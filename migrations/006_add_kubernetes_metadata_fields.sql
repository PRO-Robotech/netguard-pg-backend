-- +goose Up
-- Add missing Kubernetes metadata fields for PATCH operation support
-- Without UID, objInfo.UpdatedObject() fails during PATCH operations

-- Add UUID extension if not exists
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Add UID field to k8s_metadata table
-- UID is critical for Kubernetes PATCH operations - objInfo.UpdatedObject() requires it
ALTER TABLE k8s_metadata 
ADD COLUMN uid UUID DEFAULT uuid_generate_v4() UNIQUE NOT NULL;

-- Add Generation field to k8s_metadata table  
-- Generation tracks resource specification changes
ALTER TABLE k8s_metadata 
ADD COLUMN generation BIGINT DEFAULT 1 NOT NULL;

-- Create index on UID for fast lookups during PATCH operations
CREATE INDEX idx_k8s_metadata_uid ON k8s_metadata(uid);

-- Update existing records to have proper UID and Generation values
-- This ensures existing resources work with PATCH operations
UPDATE k8s_metadata 
SET uid = uuid_generate_v4(), generation = 1 
WHERE uid IS NULL OR generation IS NULL;

-- +goose Down
-- Remove Kubernetes metadata fields

DROP INDEX IF EXISTS idx_k8s_metadata_uid;
ALTER TABLE k8s_metadata DROP COLUMN IF EXISTS generation;
ALTER TABLE k8s_metadata DROP COLUMN IF EXISTS uid;
DROP EXTENSION IF EXISTS "uuid-ossp";