-- +goose Up
-- Description: Remove deprecated legacy entities (RuleS2S, ServiceAlias, IEAgAgRule)

-- Drop triggers first
DROP TRIGGER IF EXISTS trg_rule_s2s_rv_bump ON rule_s2s;
DROP TRIGGER IF EXISTS trg_service_aliases_rv_bump ON service_aliases;
DROP TRIGGER IF EXISTS trg_ie_ag_ag_rules_rv_bump ON ie_ag_ag_rules;

-- Drop indexes on ie_ag_ag_rules
DROP INDEX IF EXISTS idx_ie_ag_ag_rules_local;
DROP INDEX IF EXISTS idx_ie_ag_ag_rules_target;
DROP INDEX IF EXISTS idx_ie_ag_ag_rules_ports;

-- Drop indexes on rule_s2s
DROP INDEX IF EXISTS idx_rule_s2s_service_local_ref_name;
DROP INDEX IF EXISTS idx_rule_s2s_service_local_ref_namespace;
DROP INDEX IF EXISTS idx_rule_s2s_service_ref_name;
DROP INDEX IF EXISTS idx_rule_s2s_service_ref_namespace;

-- Drop tables (order matters for foreign key constraints)
DROP TABLE IF EXISTS ie_ag_ag_rules CASCADE;
DROP TABLE IF EXISTS service_aliases CASCADE;
DROP TABLE IF EXISTS rule_s2s CASCADE;

-- +goose Down
-- Recreate tables (reverse migration - minimal schema, data is lost)

CREATE TABLE rule_s2s (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    traffic traffic_direction NOT NULL,
    service_local_ref JSONB NOT NULL DEFAULT '{}'::JSONB,
    service_ref JSONB NOT NULL DEFAULT '{}'::JSONB,
    ieagag_rule_refs JSONB NOT NULL DEFAULT '[]'::JSONB,
    trace BOOLEAN NOT NULL DEFAULT false,
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name)
);

CREATE TABLE service_aliases (
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    service_namespace namespace_name NOT NULL,
    service_name resource_name NOT NULL,
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name)
);

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
    ports JSONB DEFAULT '[]',
    resource_version BIGINT NOT NULL REFERENCES k8s_metadata(resource_version) ON DELETE CASCADE,
    PRIMARY KEY (namespace, name),
    FOREIGN KEY (address_group_local_namespace, address_group_local_name) REFERENCES address_groups(namespace, name) ON DELETE CASCADE,
    FOREIGN KEY (address_group_namespace, address_group_name) REFERENCES address_groups(namespace, name) ON DELETE CASCADE
);

-- Recreate indexes
CREATE INDEX idx_ie_ag_ag_rules_local ON ie_ag_ag_rules(address_group_local_namespace, address_group_local_name);
CREATE INDEX idx_ie_ag_ag_rules_target ON ie_ag_ag_rules(address_group_namespace, address_group_name);
CREATE INDEX idx_ie_ag_ag_rules_ports ON ie_ag_ag_rules USING GIN(ports);
CREATE INDEX idx_rule_s2s_service_local_ref_name ON rule_s2s USING btree ((service_local_ref->>'name'));
CREATE INDEX idx_rule_s2s_service_local_ref_namespace ON rule_s2s USING btree ((service_local_ref->>'namespace'));
CREATE INDEX idx_rule_s2s_service_ref_name ON rule_s2s USING btree ((service_ref->>'name'));
CREATE INDEX idx_rule_s2s_service_ref_namespace ON rule_s2s USING btree ((service_ref->>'namespace'));
