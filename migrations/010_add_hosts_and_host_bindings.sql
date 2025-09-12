-- +goose Up
-- Add hosts and host_bindings tables for Host and HostBinding K8s resources

-- Hosts table - represents Host K8s resources (formerly Agent)
CREATE TABLE hosts (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    uuid TEXT NOT NULL UNIQUE, -- Host UUID from spec
    hostname TEXT NOT NULL, -- DNS hostname from spec
    
    -- Status fields
    host_name_sync TEXT, -- Host name used for synchronization
    address_group_name TEXT, -- Name of bound AddressGroup
    is_bound BOOLEAN NOT NULL DEFAULT false,
    binding_ref_namespace namespace_name, -- Reference to HostBinding
    binding_ref_name resource_name,
    address_group_ref_namespace namespace_name, -- Reference to AddressGroup
    address_group_ref_name resource_name,
    
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name),
    -- Note: binding_ref foreign key constraint added after host_bindings table creation
    FOREIGN KEY (address_group_ref_namespace, address_group_ref_name) REFERENCES address_groups(namespace, name) ON DELETE SET NULL
);

-- HostBindings table - represents HostBinding K8s resources
CREATE TABLE host_bindings (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    host_namespace namespace_name NOT NULL,
    host_name resource_name NOT NULL,
    address_group_namespace namespace_name NOT NULL,
    address_group_name resource_name NOT NULL,
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name),
    FOREIGN KEY (host_namespace, host_name) REFERENCES hosts(namespace, name) ON DELETE CASCADE,
    FOREIGN KEY (address_group_namespace, address_group_name) REFERENCES address_groups(namespace, name) ON DELETE CASCADE,
    UNIQUE (host_namespace, host_name) -- One binding per host
);

-- Fix circular reference: Add foreign key constraint for hosts -> host_bindings after table creation
ALTER TABLE hosts ADD CONSTRAINT fk_hosts_binding_ref 
    FOREIGN KEY (binding_ref_namespace, binding_ref_name) 
    REFERENCES host_bindings(namespace, name) ON DELETE SET NULL;

-- Indexes for performance
CREATE INDEX idx_hosts_uuid ON hosts(uuid);
CREATE INDEX idx_hosts_hostname ON hosts(hostname);
CREATE INDEX idx_hosts_is_bound ON hosts(is_bound);
CREATE INDEX idx_hosts_address_group_name ON hosts(address_group_name) WHERE address_group_name IS NOT NULL;
CREATE INDEX idx_host_bindings_host ON host_bindings(host_namespace, host_name);
CREATE INDEX idx_host_bindings_address_group ON host_bindings(address_group_namespace, address_group_name);

-- +goose Down
DROP TABLE IF EXISTS hosts CASCADE;
DROP TABLE IF EXISTS host_bindings CASCADE;