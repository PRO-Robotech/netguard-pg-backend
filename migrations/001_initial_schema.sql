-- +goose Up
-- Initial schema for netguard PostgreSQL backend
-- Following sgroups patterns with K8s metadata support

-- Custom domains for type safety
CREATE DOMAIN resource_name AS TEXT CHECK (char_length(VALUE) > 0 AND char_length(VALUE) <= 253);
CREATE DOMAIN namespace_name AS TEXT CHECK (char_length(VALUE) > 0 AND char_length(VALUE) <= 253);
CREATE DOMAIN rule_action AS TEXT CHECK (VALUE IN ('ACCEPT', 'DROP'));
CREATE DOMAIN traffic_direction AS TEXT CHECK (VALUE IN ('INGRESS', 'EGRESS'));
CREATE DOMAIN transport_protocol AS TEXT CHECK (VALUE IN ('TCP', 'UDP', 'ICMP'));

-- K8s metadata table for shared fields
CREATE TABLE k8s_metadata (
    resource_version BIGSERIAL PRIMARY KEY,
    labels JSONB DEFAULT '{}',
    annotations JSONB DEFAULT '{}',
    finalizers TEXT[] DEFAULT '{}',
    conditions JSONB DEFAULT '[]',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Services table with K8s metadata
CREATE TABLE services (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    description TEXT DEFAULT '',
    ingress_ports JSONB NOT NULL DEFAULT '[]', -- IngressPort[]
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name)
);

-- AddressGroups table
CREATE TABLE address_groups (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    default_action rule_action NOT NULL DEFAULT 'DROP',
    logs BOOLEAN NOT NULL DEFAULT false,
    trace BOOLEAN NOT NULL DEFAULT false,
    description TEXT DEFAULT '',
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name)
);

-- AddressGroupBindings table for Service ↔ AddressGroup relationships
CREATE TABLE address_group_bindings (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    service_namespace namespace_name NOT NULL,
    service_name resource_name NOT NULL,
    address_group_namespace namespace_name NOT NULL,
    address_group_name resource_name NOT NULL,
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name),
    FOREIGN KEY (service_namespace, service_name) REFERENCES services(namespace, name) ON DELETE CASCADE,
    FOREIGN KEY (address_group_namespace, address_group_name) REFERENCES address_groups(namespace, name) ON DELETE CASCADE
);

-- AddressGroupPortMappings table
CREATE TABLE address_group_port_mappings (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    access_ports JSONB NOT NULL DEFAULT '{}', -- map[ServiceRef]ServicePorts
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name)
);

-- RuleS2S table for service-to-service rules
CREATE TABLE rule_s2s (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    traffic traffic_direction NOT NULL,
    service_local_namespace namespace_name NOT NULL,
    service_local_name resource_name NOT NULL,
    service_namespace namespace_name NOT NULL,
    service_name resource_name NOT NULL,
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name)
);

-- ServiceAliases table
CREATE TABLE service_aliases (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    service_namespace namespace_name NOT NULL,
    service_name resource_name NOT NULL,
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name),
    FOREIGN KEY (service_namespace, service_name) REFERENCES services(namespace, name) ON DELETE CASCADE
);

-- AddressGroupBindingPolicies table
CREATE TABLE address_group_binding_policies (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    policy_data JSONB NOT NULL DEFAULT '{}',
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name)
);

-- IEAgAgRules table for ingress/egress address group rules
CREATE TABLE ie_ag_ag_rules (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    transport transport_protocol NOT NULL,
    traffic traffic_direction NOT NULL,
    action rule_action NOT NULL,
    address_group_local_namespace namespace_name NOT NULL,
    address_group_local_name resource_name NOT NULL,
    address_group_namespace namespace_name NOT NULL,
    address_group_name resource_name NOT NULL,
    ports JSONB DEFAULT '[]', -- PortSpec[]
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name),
    FOREIGN KEY (address_group_local_namespace, address_group_local_name) REFERENCES address_groups(namespace, name) ON DELETE CASCADE,
    FOREIGN KEY (address_group_namespace, address_group_name) REFERENCES address_groups(namespace, name) ON DELETE CASCADE
);

-- Networks table for CIDR definitions
CREATE TABLE networks (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    network_items JSONB NOT NULL DEFAULT '[]', -- NetworkItem[]
    is_bound BOOLEAN NOT NULL DEFAULT false,
    binding_ref_namespace namespace_name,
    binding_ref_name resource_name,
    address_group_ref_namespace namespace_name,
    address_group_ref_name resource_name,
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name),
    FOREIGN KEY (binding_ref_namespace, binding_ref_name) REFERENCES address_group_bindings(namespace, name) ON DELETE SET NULL,
    FOREIGN KEY (address_group_ref_namespace, address_group_ref_name) REFERENCES address_groups(namespace, name) ON DELETE SET NULL
);

-- NetworkBindings table
CREATE TABLE network_bindings (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    network_namespace namespace_name NOT NULL,
    network_name resource_name NOT NULL,
    address_group_namespace namespace_name NOT NULL,
    address_group_name resource_name NOT NULL,
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name),
    FOREIGN KEY (network_namespace, network_name) REFERENCES networks(namespace, name) ON DELETE CASCADE,
    FOREIGN KEY (address_group_namespace, address_group_name) REFERENCES address_groups(namespace, name) ON DELETE CASCADE
);

-- Sync status table
CREATE TABLE sync_status (
    id INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1), -- Singleton pattern
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Insert initial sync status
INSERT INTO sync_status (updated_at) VALUES (NOW());

-- Indexes for performance
CREATE INDEX idx_services_namespace ON services(namespace);
CREATE INDEX idx_address_groups_namespace ON address_groups(namespace);
CREATE INDEX idx_address_group_bindings_service ON address_group_bindings(service_namespace, service_name);
CREATE INDEX idx_address_group_bindings_ag ON address_group_bindings(address_group_namespace, address_group_name);
CREATE INDEX idx_ie_ag_ag_rules_local ON ie_ag_ag_rules(address_group_local_namespace, address_group_local_name);
CREATE INDEX idx_ie_ag_ag_rules_target ON ie_ag_ag_rules(address_group_namespace, address_group_name);
CREATE INDEX idx_networks_namespace ON networks(namespace);
CREATE INDEX idx_network_bindings_network ON network_bindings(network_namespace, network_name);
CREATE INDEX idx_network_bindings_ag ON network_bindings(address_group_namespace, address_group_name);

-- GIN indexes for JSONB performance
CREATE INDEX idx_k8s_metadata_labels ON k8s_metadata USING GIN(labels);
CREATE INDEX idx_k8s_metadata_annotations ON k8s_metadata USING GIN(annotations);
CREATE INDEX idx_k8s_metadata_conditions ON k8s_metadata USING GIN(conditions);
CREATE INDEX idx_services_ingress_ports ON services USING GIN(ingress_ports);
CREATE INDEX idx_address_group_port_mappings_access_ports ON address_group_port_mappings USING GIN(access_ports);
CREATE INDEX idx_ie_ag_ag_rules_ports ON ie_ag_ag_rules USING GIN(ports);
CREATE INDEX idx_networks_network_items ON networks USING GIN(network_items);

-- Note: Triggers and functions removed for Goose compatibility
-- The sync_status table will be updated manually by the application

-- +goose Down
-- Drop tables in reverse dependency order

DROP TABLE IF EXISTS network_bindings;
DROP TABLE IF EXISTS networks;
DROP TABLE IF EXISTS ie_ag_ag_rules;
DROP TABLE IF EXISTS address_group_binding_policies;
DROP TABLE IF EXISTS service_aliases;
DROP TABLE IF EXISTS rule_s2s;
DROP TABLE IF EXISTS address_group_port_mappings;
DROP TABLE IF EXISTS address_group_bindings;
DROP TABLE IF EXISTS address_groups;
DROP TABLE IF EXISTS services;
DROP TABLE IF EXISTS sync_status;
DROP TABLE IF EXISTS k8s_metadata;

-- Drop domains
DROP DOMAIN IF EXISTS transport_protocol;
DROP DOMAIN IF EXISTS traffic_direction;
DROP DOMAIN IF EXISTS rule_action;
DROP DOMAIN IF EXISTS namespace_name;
DROP DOMAIN IF EXISTS resource_name;