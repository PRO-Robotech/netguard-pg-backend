-- +goose Up
-- Add GIN indexes for label selectors on k8s_metadata.labels
-- GIN (Generalized Inverted Index) is optimized for JSONB operations

-- GIN index for JSONB existence checks (labels ? 'key')
-- Supports: LABEL_OPERATOR_EXISTS, LABEL_OPERATOR_NOT_EXISTS
CREATE INDEX IF NOT EXISTS idx_k8s_metadata_labels_gin
    ON k8s_metadata USING GIN (labels);

-- GIN index with jsonb_path_ops for faster key-value lookups
-- Supports: LABEL_OPERATOR_EQUALS, LABEL_OPERATOR_NOT_EQUALS, LABEL_OPERATOR_IN, LABEL_OPERATOR_NOT_IN
-- jsonb_path_ops is more compact and faster for containment queries
CREATE INDEX IF NOT EXISTS idx_k8s_metadata_labels_gin_path_ops
    ON k8s_metadata USING GIN (labels jsonb_path_ops);

-- +goose Down
-- Remove GIN indexes from k8s_metadata.labels

DROP INDEX IF EXISTS idx_k8s_metadata_labels_gin_path_ops;
DROP INDEX IF EXISTS idx_k8s_metadata_labels_gin;
