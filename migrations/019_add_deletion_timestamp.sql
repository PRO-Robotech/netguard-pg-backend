-- +goose Up
-- Add deletion_timestamp to k8s_metadata for tracking object deletion state

ALTER TABLE k8s_metadata
ADD COLUMN deletion_timestamp TIMESTAMP WITH TIME ZONE DEFAULT NULL;

CREATE INDEX idx_k8s_metadata_deletion_timestamp
ON k8s_metadata(deletion_timestamp)
WHERE deletion_timestamp IS NOT NULL;

COMMENT ON COLUMN k8s_metadata.deletion_timestamp IS 'Timestamp when object deletion was requested. NULL means object is not being deleted.';

-- +goose Down
DROP INDEX IF EXISTS idx_k8s_metadata_deletion_timestamp;
ALTER TABLE k8s_metadata DROP COLUMN IF EXISTS deletion_timestamp;
