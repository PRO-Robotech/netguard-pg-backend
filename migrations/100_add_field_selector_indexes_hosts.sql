-- +goose Up
-- Migration: Add indexes for field selector queries on hosts table
-- Created: 2025-10-29
-- Purpose: Optimize field-selector based List operations for Host resources

-- +goose StatementBegin

-- Index for metadata.name field selector
CREATE INDEX IF NOT EXISTS idx_hosts_name
    ON hosts(name);

-- Index for metadata.namespace field selector
CREATE INDEX IF NOT EXISTS idx_hosts_namespace
    ON hosts(namespace);

-- Composite index for namespace + name (most common combination)
CREATE INDEX IF NOT EXISTS idx_hosts_namespace_name
    ON hosts(namespace, name);

-- Index for status.addressGroupRef.name field selector
CREATE INDEX IF NOT EXISTS idx_hosts_address_group_ref_name
    ON hosts(address_group_ref_name)
    WHERE address_group_ref_name IS NOT NULL;

-- Index for status.addressGroupRef.namespace field selector
CREATE INDEX IF NOT EXISTS idx_hosts_address_group_ref_namespace
    ON hosts(address_group_ref_namespace)
    WHERE address_group_ref_namespace IS NOT NULL;

-- Composite index for addressGroupRef filtering
CREATE INDEX IF NOT EXISTS idx_hosts_ag_ref_ns_name
    ON hosts(address_group_ref_namespace, address_group_ref_name)
    WHERE address_group_ref_namespace IS NOT NULL AND address_group_ref_name IS NOT NULL;

-- Index for status.bindingRef.name field selector
CREATE INDEX IF NOT EXISTS idx_hosts_binding_ref_name
    ON hosts(binding_ref_name)
    WHERE binding_ref_name IS NOT NULL;

-- Index for status.bindingRef.namespace field selector
CREATE INDEX IF NOT EXISTS idx_hosts_binding_ref_namespace
    ON hosts(binding_ref_namespace)
    WHERE binding_ref_namespace IS NOT NULL;

-- Composite index for bindingRef filtering
CREATE INDEX IF NOT EXISTS idx_hosts_binding_ref_ns_name
    ON hosts(binding_ref_namespace, binding_ref_name)
    WHERE binding_ref_namespace IS NOT NULL AND binding_ref_name IS NOT NULL;

-- Index for status.isBound field selector (already exists from migration 010, but ensure it exists)
-- CREATE INDEX IF NOT EXISTS idx_hosts_is_bound ON hosts(is_bound);

-- Composite index for common query pattern: namespace + addressGroupRef.name
CREATE INDEX IF NOT EXISTS idx_hosts_ns_ag_name
    ON hosts(namespace, address_group_ref_name)
    WHERE address_group_ref_name IS NOT NULL;

-- Composite index for common query pattern: namespace + isBound
CREATE INDEX IF NOT EXISTS idx_hosts_ns_is_bound
    ON hosts(namespace, is_bound);

-- Add comments for documentation
COMMENT ON INDEX idx_hosts_name IS 'Supports field-selector: metadata.name';
COMMENT ON INDEX idx_hosts_namespace IS 'Supports field-selector: metadata.namespace';
COMMENT ON INDEX idx_hosts_namespace_name IS 'Composite index for namespace + name filtering';
COMMENT ON INDEX idx_hosts_address_group_ref_name IS 'Supports field-selector: status.addressGroupRef.name';
COMMENT ON INDEX idx_hosts_address_group_ref_namespace IS 'Supports field-selector: status.addressGroupRef.namespace';
COMMENT ON INDEX idx_hosts_ag_ref_ns_name IS 'Composite index for addressGroupRef filtering';
COMMENT ON INDEX idx_hosts_binding_ref_name IS 'Supports field-selector: status.bindingRef.name';
COMMENT ON INDEX idx_hosts_binding_ref_namespace IS 'Supports field-selector: status.bindingRef.namespace';
COMMENT ON INDEX idx_hosts_binding_ref_ns_name IS 'Composite index for bindingRef filtering';
COMMENT ON INDEX idx_hosts_ns_ag_name IS 'Composite index for common query pattern: namespace + addressGroupRef.name';
COMMENT ON INDEX idx_hosts_ns_is_bound IS 'Composite index for common query pattern: namespace + isBound';

-- +goose StatementEnd

-- +goose Down
-- Rollback: Drop field selector indexes for hosts table

-- +goose StatementBegin

DROP INDEX IF EXISTS idx_hosts_ns_is_bound;
DROP INDEX IF EXISTS idx_hosts_ns_ag_name;
DROP INDEX IF EXISTS idx_hosts_binding_ref_ns_name;
DROP INDEX IF EXISTS idx_hosts_binding_ref_namespace;
DROP INDEX IF EXISTS idx_hosts_binding_ref_name;
DROP INDEX IF EXISTS idx_hosts_ag_ref_ns_name;
DROP INDEX IF EXISTS idx_hosts_address_group_ref_namespace;
DROP INDEX IF EXISTS idx_hosts_address_group_ref_name;
DROP INDEX IF EXISTS idx_hosts_namespace_name;
DROP INDEX IF EXISTS idx_hosts_namespace;
DROP INDEX IF EXISTS idx_hosts_name;

-- +goose StatementEnd
