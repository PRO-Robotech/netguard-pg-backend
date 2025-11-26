-- +goose Up

-- Ensure all resource tables cascade updates to their k8s_metadata rows
ALTER TABLE services
    DROP CONSTRAINT IF EXISTS services_resource_version_fkey;
ALTER TABLE services
    ADD CONSTRAINT services_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE address_groups
    DROP CONSTRAINT IF EXISTS address_groups_resource_version_fkey;
ALTER TABLE address_groups
    ADD CONSTRAINT address_groups_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE address_group_bindings
    DROP CONSTRAINT IF EXISTS address_group_bindings_resource_version_fkey;
ALTER TABLE address_group_bindings
    ADD CONSTRAINT address_group_bindings_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON UPDATE CASCADE
    ON DELETE RESTRICT;

ALTER TABLE address_group_port_mappings
    DROP CONSTRAINT IF EXISTS address_group_port_mappings_resource_version_fkey;
ALTER TABLE address_group_port_mappings
    ADD CONSTRAINT address_group_port_mappings_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE rule_s2s
    DROP CONSTRAINT IF EXISTS rule_s2s_resource_version_fkey;
ALTER TABLE rule_s2s
    ADD CONSTRAINT rule_s2s_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE service_aliases
    DROP CONSTRAINT IF EXISTS service_aliases_resource_version_fkey;
ALTER TABLE service_aliases
    ADD CONSTRAINT service_aliases_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE address_group_binding_policies
    DROP CONSTRAINT IF EXISTS address_group_binding_policies_resource_version_fkey;
ALTER TABLE address_group_binding_policies
    ADD CONSTRAINT address_group_binding_policies_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE ie_ag_ag_rules
    DROP CONSTRAINT IF EXISTS ie_ag_ag_rules_resource_version_fkey;
ALTER TABLE ie_ag_ag_rules
    ADD CONSTRAINT ie_ag_ag_rules_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE networks
    DROP CONSTRAINT IF EXISTS networks_resource_version_fkey;
ALTER TABLE networks
    ADD CONSTRAINT networks_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE network_bindings
    DROP CONSTRAINT IF EXISTS network_bindings_resource_version_fkey;
ALTER TABLE network_bindings
    ADD CONSTRAINT network_bindings_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE hosts
    DROP CONSTRAINT IF EXISTS hosts_resource_version_fkey;
ALTER TABLE hosts
    ADD CONSTRAINT hosts_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE host_bindings
    DROP CONSTRAINT IF EXISTS host_bindings_resource_version_fkey;
ALTER TABLE host_bindings
    ADD CONSTRAINT host_bindings_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON UPDATE CASCADE
    ON DELETE CASCADE;

-- Automatically bump resource_version on every metadata UPDATE
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION bump_k8s_resource_version()
RETURNS TRIGGER AS $$
BEGIN
    NEW.resource_version := nextval('k8s_metadata_resource_version_seq');
    NEW.updated_at := NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_k8s_metadata_bump_rv ON k8s_metadata;
CREATE TRIGGER trg_k8s_metadata_bump_rv
BEFORE UPDATE ON k8s_metadata
FOR EACH ROW
EXECUTE FUNCTION bump_k8s_resource_version();
-- +goose StatementEnd

-- +goose Down

DROP TRIGGER IF EXISTS trg_k8s_metadata_bump_rv ON k8s_metadata;
DROP FUNCTION IF EXISTS bump_k8s_resource_version;

ALTER TABLE services
    DROP CONSTRAINT IF EXISTS services_resource_version_fkey;
ALTER TABLE services
    ADD CONSTRAINT services_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON DELETE CASCADE;

ALTER TABLE address_groups
    DROP CONSTRAINT IF EXISTS address_groups_resource_version_fkey;
ALTER TABLE address_groups
    ADD CONSTRAINT address_groups_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON DELETE CASCADE;

ALTER TABLE address_group_bindings
    DROP CONSTRAINT IF EXISTS address_group_bindings_resource_version_fkey;
ALTER TABLE address_group_bindings
    ADD CONSTRAINT address_group_bindings_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON DELETE RESTRICT;

ALTER TABLE address_group_port_mappings
    DROP CONSTRAINT IF EXISTS address_group_port_mappings_resource_version_fkey;
ALTER TABLE address_group_port_mappings
    ADD CONSTRAINT address_group_port_mappings_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON DELETE CASCADE;

ALTER TABLE rule_s2s
    DROP CONSTRAINT IF EXISTS rule_s2s_resource_version_fkey;
ALTER TABLE rule_s2s
    ADD CONSTRAINT rule_s2s_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON DELETE CASCADE;

ALTER TABLE service_aliases
    DROP CONSTRAINT IF EXISTS service_aliases_resource_version_fkey;
ALTER TABLE service_aliases
    ADD CONSTRAINT service_aliases_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON DELETE CASCADE;

ALTER TABLE address_group_binding_policies
    DROP CONSTRAINT IF EXISTS address_group_binding_policies_resource_version_fkey;
ALTER TABLE address_group_binding_policies
    ADD CONSTRAINT address_group_binding_policies_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON DELETE CASCADE;

ALTER TABLE ie_ag_ag_rules
    DROP CONSTRAINT IF EXISTS ie_ag_ag_rules_resource_version_fkey;
ALTER TABLE ie_ag_ag_rules
    ADD CONSTRAINT ie_ag_ag_rules_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON DELETE CASCADE;

ALTER TABLE networks
    DROP CONSTRAINT IF EXISTS networks_resource_version_fkey;
ALTER TABLE networks
    ADD CONSTRAINT networks_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON DELETE CASCADE;

ALTER TABLE network_bindings
    DROP CONSTRAINT IF EXISTS network_bindings_resource_version_fkey;
ALTER TABLE network_bindings
    ADD CONSTRAINT network_bindings_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON DELETE CASCADE;

ALTER TABLE hosts
    DROP CONSTRAINT IF EXISTS hosts_resource_version_fkey;
ALTER TABLE hosts
    ADD CONSTRAINT hosts_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON DELETE CASCADE;

ALTER TABLE host_bindings
    DROP CONSTRAINT IF EXISTS host_bindings_resource_version_fkey;
ALTER TABLE host_bindings
    ADD CONSTRAINT host_bindings_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON DELETE CASCADE;

