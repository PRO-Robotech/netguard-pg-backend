-- +goose Up
-- Any UPDATE that leaves resource_version untouched (e.g. Postgres triggers that rewrite JSON fields)
-- would previously reuse the old RV, causing watchers to drop the event.
-- The ensure_resource_version_bump() trigger detects those cases and bumps the referenced
-- k8s_metadata row so the table update emits a fresh resourceVersion.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION ensure_resource_version_bump()
RETURNS TRIGGER AS $$
DECLARE
    new_rv BIGINT;
BEGIN
    IF NEW.resource_version = OLD.resource_version THEN
        UPDATE k8s_metadata
        SET updated_at = NOW()
        WHERE resource_version = OLD.resource_version
        RETURNING resource_version INTO new_rv;

        NEW.resource_version := new_rv;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_services_rv_bump
BEFORE UPDATE ON services
FOR EACH ROW EXECUTE FUNCTION ensure_resource_version_bump();

CREATE TRIGGER trg_address_groups_rv_bump
BEFORE UPDATE ON address_groups
FOR EACH ROW EXECUTE FUNCTION ensure_resource_version_bump();

CREATE TRIGGER trg_address_group_bindings_rv_bump
BEFORE UPDATE ON address_group_bindings
FOR EACH ROW EXECUTE FUNCTION ensure_resource_version_bump();

CREATE TRIGGER trg_address_group_port_mappings_rv_bump
BEFORE UPDATE ON address_group_port_mappings
FOR EACH ROW EXECUTE FUNCTION ensure_resource_version_bump();

CREATE TRIGGER trg_address_group_binding_policies_rv_bump
BEFORE UPDATE ON address_group_binding_policies
FOR EACH ROW EXECUTE FUNCTION ensure_resource_version_bump();

CREATE TRIGGER trg_rule_s2s_rv_bump
BEFORE UPDATE ON rule_s2s
FOR EACH ROW EXECUTE FUNCTION ensure_resource_version_bump();

CREATE TRIGGER trg_service_aliases_rv_bump
BEFORE UPDATE ON service_aliases
FOR EACH ROW EXECUTE FUNCTION ensure_resource_version_bump();

CREATE TRIGGER trg_ie_ag_ag_rules_rv_bump
BEFORE UPDATE ON ie_ag_ag_rules
FOR EACH ROW EXECUTE FUNCTION ensure_resource_version_bump();

CREATE TRIGGER trg_networks_rv_bump
BEFORE UPDATE ON networks
FOR EACH ROW EXECUTE FUNCTION ensure_resource_version_bump();

CREATE TRIGGER trg_network_bindings_rv_bump
BEFORE UPDATE ON network_bindings
FOR EACH ROW EXECUTE FUNCTION ensure_resource_version_bump();

CREATE TRIGGER trg_hosts_rv_bump
BEFORE UPDATE ON hosts
FOR EACH ROW EXECUTE FUNCTION ensure_resource_version_bump();

CREATE TRIGGER trg_host_bindings_rv_bump
BEFORE UPDATE ON host_bindings
FOR EACH ROW EXECUTE FUNCTION ensure_resource_version_bump();

CREATE TRIGGER trg_svc_svc_rules_rv_bump
BEFORE UPDATE ON svc_svc_rules
FOR EACH ROW EXECUTE FUNCTION ensure_resource_version_bump();

CREATE TRIGGER trg_svc_fqdn_rules_rv_bump
BEFORE UPDATE ON svc_fqdn_rules
FOR EACH ROW EXECUTE FUNCTION ensure_resource_version_bump();

-- +goose Down
DROP TRIGGER IF EXISTS trg_services_rv_bump ON services;
DROP TRIGGER IF EXISTS trg_address_groups_rv_bump ON address_groups;
DROP TRIGGER IF EXISTS trg_address_group_bindings_rv_bump ON address_group_bindings;
DROP TRIGGER IF EXISTS trg_address_group_port_mappings_rv_bump ON address_group_port_mappings;
DROP TRIGGER IF EXISTS trg_address_group_binding_policies_rv_bump ON address_group_binding_policies;
DROP TRIGGER IF EXISTS trg_rule_s2s_rv_bump ON rule_s2s;
DROP TRIGGER IF EXISTS trg_service_aliases_rv_bump ON service_aliases;
DROP TRIGGER IF EXISTS trg_ie_ag_ag_rules_rv_bump ON ie_ag_ag_rules;
DROP TRIGGER IF EXISTS trg_networks_rv_bump ON networks;
DROP TRIGGER IF EXISTS trg_network_bindings_rv_bump ON network_bindings;
DROP TRIGGER IF EXISTS trg_hosts_rv_bump ON hosts;
DROP TRIGGER IF EXISTS trg_host_bindings_rv_bump ON host_bindings;
DROP TRIGGER IF EXISTS trg_svc_svc_rules_rv_bump ON svc_svc_rules;
DROP TRIGGER IF EXISTS trg_svc_fqdn_rules_rv_bump ON svc_fqdn_rules;

DROP FUNCTION IF EXISTS ensure_resource_version_bump();

