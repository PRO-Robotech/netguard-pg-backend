-- +goose Up
-- Migration: Add indexes for field selector queries on address_groups table
-- Created: 2025-10-29
-- Purpose: Optimize field-selector based List operations for AddressGroup resources

-- +goose StatementBegin

-- Index for metadata.name field selector
CREATE INDEX IF NOT EXISTS idx_address_groups_name
    ON address_groups(name);

-- Index for metadata.namespace field selector (already exists from migration 001, but ensure it exists)
-- CREATE INDEX IF NOT EXISTS idx_address_groups_namespace ON address_groups(namespace);

-- Composite index for namespace + name (most common combination)
CREATE INDEX IF NOT EXISTS idx_address_groups_namespace_name
    ON address_groups(namespace, name);

-- Index for spec.defaultAction field selector
CREATE INDEX IF NOT EXISTS idx_address_groups_default_action
    ON address_groups(default_action);

-- Index for spec.logs field selector
CREATE INDEX IF NOT EXISTS idx_address_groups_logs
    ON address_groups(logs);

-- Index for spec.trace field selector
CREATE INDEX IF NOT EXISTS idx_address_groups_trace
    ON address_groups(trace);

-- Composite index for common query pattern: namespace + defaultAction
CREATE INDEX IF NOT EXISTS idx_address_groups_ns_action
    ON address_groups(namespace, default_action);

-- Composite index for common query pattern: namespace + logs
CREATE INDEX IF NOT EXISTS idx_address_groups_ns_logs
    ON address_groups(namespace, logs);

-- Composite index for common query pattern: namespace + trace
CREATE INDEX IF NOT EXISTS idx_address_groups_ns_trace
    ON address_groups(namespace, trace);

-- Add comments for documentation
COMMENT ON INDEX idx_address_groups_name IS 'Supports field-selector: metadata.name';
COMMENT ON INDEX idx_address_groups_namespace_name IS 'Composite index for namespace + name filtering';
COMMENT ON INDEX idx_address_groups_default_action IS 'Supports field-selector: spec.defaultAction';
COMMENT ON INDEX idx_address_groups_logs IS 'Supports field-selector: spec.logs';
COMMENT ON INDEX idx_address_groups_trace IS 'Supports field-selector: spec.trace';
COMMENT ON INDEX idx_address_groups_ns_action IS 'Composite index for common query pattern: namespace + defaultAction';
COMMENT ON INDEX idx_address_groups_ns_logs IS 'Composite index for common query pattern: namespace + logs';
COMMENT ON INDEX idx_address_groups_ns_trace IS 'Composite index for common query pattern: namespace + trace';

-- +goose StatementEnd

-- +goose Down
-- Rollback: Drop field selector indexes for address_groups table

-- +goose StatementBegin

DROP INDEX IF EXISTS idx_address_groups_ns_trace;
DROP INDEX IF EXISTS idx_address_groups_ns_logs;
DROP INDEX IF EXISTS idx_address_groups_ns_action;
DROP INDEX IF EXISTS idx_address_groups_trace;
DROP INDEX IF EXISTS idx_address_groups_logs;
DROP INDEX IF EXISTS idx_address_groups_default_action;
DROP INDEX IF EXISTS idx_address_groups_namespace_name;
DROP INDEX IF EXISTS idx_address_groups_name;

-- +goose StatementEnd
