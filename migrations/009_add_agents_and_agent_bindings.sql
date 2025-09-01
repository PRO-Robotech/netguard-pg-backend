-- +goose Up
-- Add agents and agent_bindings tables for Agent entity support

-- Agents table for Agent resources
CREATE TABLE agents (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    uuid TEXT NOT NULL UNIQUE, -- Agent UUID from registration
    hostname TEXT NOT NULL, -- DNS-style hostname
    is_bound BOOLEAN NOT NULL DEFAULT false,
    binding_ref_namespace namespace_name,
    binding_ref_name resource_name,
    address_group_ref_namespace namespace_name,
    address_group_ref_name resource_name,
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name),
    FOREIGN KEY (binding_ref_namespace, binding_ref_name) REFERENCES agent_bindings(namespace, name) ON DELETE SET NULL,
    FOREIGN KEY (address_group_ref_namespace, address_group_ref_name) REFERENCES address_groups(namespace, name) ON DELETE SET NULL,
    CHECK (char_length(uuid) > 0 AND char_length(uuid) <= 255),
    CHECK (hostname ~ '^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$')
);

-- AgentBindings table for Agent ↔ AddressGroup relationships
CREATE TABLE agent_bindings (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    agent_namespace namespace_name NOT NULL,
    agent_name resource_name NOT NULL,
    address_group_namespace namespace_name NOT NULL,
    address_group_name resource_name NOT NULL,
    agent_item JSONB NOT NULL DEFAULT '{}', -- AgentItem data
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name),
    FOREIGN KEY (agent_namespace, agent_name) REFERENCES agents(namespace, name) ON DELETE CASCADE,
    FOREIGN KEY (address_group_namespace, address_group_name) REFERENCES address_groups(namespace, name) ON DELETE CASCADE
);

-- Fix the circular reference for agents table by altering the foreign key constraint
ALTER TABLE agents
DROP CONSTRAINT IF EXISTS agents_binding_ref_namespace_fkey,
ADD CONSTRAINT agents_binding_ref_namespace_fkey 
    FOREIGN KEY (binding_ref_namespace, binding_ref_name) 
    REFERENCES agent_bindings(namespace, name) ON DELETE SET NULL;

-- Add agents JSONB field to address_groups table to store AgentItem array
ALTER TABLE address_groups 
ADD COLUMN agents JSONB NOT NULL DEFAULT '[]';

-- Indexes for performance
CREATE INDEX idx_agents_namespace ON agents(namespace);
CREATE INDEX idx_agents_uuid ON agents(uuid);
CREATE INDEX idx_agents_hostname ON agents(hostname);
CREATE INDEX idx_agent_bindings_agent ON agent_bindings(agent_namespace, agent_name);
CREATE INDEX idx_agent_bindings_ag ON agent_bindings(address_group_namespace, address_group_name);

-- GIN indexes for JSONB performance
CREATE INDEX idx_agent_bindings_agent_item ON agent_bindings USING GIN(agent_item);
CREATE INDEX idx_address_groups_agents ON address_groups USING GIN(agents);

-- +goose Down
-- Drop agents and agent_bindings tables and related changes

-- Remove agents field from address_groups
ALTER TABLE address_groups DROP COLUMN IF EXISTS agents;

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS agent_bindings;
DROP TABLE IF EXISTS agents;