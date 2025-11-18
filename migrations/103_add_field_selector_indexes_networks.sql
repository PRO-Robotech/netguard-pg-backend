-- +goose Up
-- Add indexes for field selectors on networks table

-- Index for metadata.name field selector
CREATE INDEX IF NOT EXISTS idx_networks_name
    ON networks(name);

-- Index for metadata.namespace field selector
CREATE INDEX IF NOT EXISTS idx_networks_namespace
    ON networks(namespace);

-- Index for status.isBound field selector
CREATE INDEX IF NOT EXISTS idx_networks_is_bound
    ON networks(is_bound);

-- Index for status.bindingRef.name field selector
CREATE INDEX IF NOT EXISTS idx_networks_binding_ref_name
    ON networks(binding_ref_name);

-- Index for status.bindingRef.namespace field selector
CREATE INDEX IF NOT EXISTS idx_networks_binding_ref_namespace
    ON networks(binding_ref_namespace);

-- Index for status.addressGroupRef.name field selector
CREATE INDEX IF NOT EXISTS idx_networks_address_group_ref_name
    ON networks(address_group_ref_name);

-- Index for status.addressGroupRef.namespace field selector
CREATE INDEX IF NOT EXISTS idx_networks_address_group_ref_namespace
    ON networks(address_group_ref_namespace);

-- Composite index for binding references (useful for filtering by both namespace and name)
CREATE INDEX IF NOT EXISTS idx_networks_binding_ref
    ON networks(binding_ref_namespace, binding_ref_name);

-- Composite index for address group references
CREATE INDEX IF NOT EXISTS idx_networks_address_group_ref
    ON networks(address_group_ref_namespace, address_group_ref_name);

-- Index for bound networks filtering
CREATE INDEX IF NOT EXISTS idx_networks_is_bound_namespace
    ON networks(is_bound, namespace);

-- +goose Down
-- Remove field selector indexes from networks table

DROP INDEX IF EXISTS idx_networks_is_bound_namespace;
DROP INDEX IF EXISTS idx_networks_address_group_ref;
DROP INDEX IF EXISTS idx_networks_binding_ref;
DROP INDEX IF EXISTS idx_networks_address_group_ref_namespace;
DROP INDEX IF EXISTS idx_networks_address_group_ref_name;
DROP INDEX IF EXISTS idx_networks_binding_ref_namespace;
DROP INDEX IF EXISTS idx_networks_binding_ref_name;
DROP INDEX IF EXISTS idx_networks_is_bound;
DROP INDEX IF EXISTS idx_networks_namespace;
DROP INDEX IF EXISTS idx_networks_name;
