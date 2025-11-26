-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notify_k8s_resource_change()
RETURNS TRIGGER AS $$
DECLARE
    notification JSON;
    resource_type TEXT;
    operation TEXT;
    ns TEXT;
    n TEXT;
    rv BIGINT;
    data_payload JSON;
BEGIN
    -- Determine operation type
    IF (TG_OP = 'DELETE') THEN
        operation := 'DELETE';
        ns := OLD.namespace;
        n := OLD.name;
        rv := OLD.resource_version;
        resource_type := TG_TABLE_NAME;
        
        -- For DELETE, include OLD data in notification
        data_payload := row_to_json(OLD);
    ELSIF (TG_OP = 'INSERT') THEN
        operation := 'INSERT';
        ns := NEW.namespace;
        n := NEW.name;
        rv := NEW.resource_version;
        resource_type := TG_TABLE_NAME;
        data_payload := row_to_json(NEW);
    ELSIF (TG_OP = 'UPDATE') THEN
        operation := 'UPDATE';
        ns := NEW.namespace;
        n := NEW.name;
        rv := NEW.resource_version;
        resource_type := TG_TABLE_NAME;
        data_payload := row_to_json(NEW);
    END IF;

    -- Build notification JSON
    notification := json_build_object(
        'operation', operation,
        'resource_type', resource_type,
        'namespace', ns,
        'name', n,
        'resource_version', rv,
        'data', data_payload,
        'timestamp', NOW()
    );

    -- Send notification to k8s_resource_changes channel
    PERFORM pg_notify('k8s_resource_changes', notification::TEXT);

    -- Return appropriate value based on operation
    IF (TG_OP = 'DELETE') THEN
        RETURN OLD;
    ELSE
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Add triggers for all K8s resource tables
-- These will fire AFTER INSERT/UPDATE/DELETE and send NOTIFY

-- Hosts table
CREATE TRIGGER notify_hosts_change
    AFTER INSERT OR UPDATE OR DELETE ON hosts
    FOR EACH ROW
    EXECUTE FUNCTION notify_k8s_resource_change();

-- Services table
CREATE TRIGGER notify_services_change
    AFTER INSERT OR UPDATE OR DELETE ON services
    FOR EACH ROW
    EXECUTE FUNCTION notify_k8s_resource_change();

-- AddressGroups table
CREATE TRIGGER notify_address_groups_change
    AFTER INSERT OR UPDATE OR DELETE ON address_groups
    FOR EACH ROW
    EXECUTE FUNCTION notify_k8s_resource_change();

-- Networks table
CREATE TRIGGER notify_networks_change
    AFTER INSERT OR UPDATE OR DELETE ON networks
    FOR EACH ROW
    EXECUTE FUNCTION notify_k8s_resource_change();

-- AddressGroupBindings table
CREATE TRIGGER notify_address_group_bindings_change
    AFTER INSERT OR UPDATE OR DELETE ON address_group_bindings
    FOR EACH ROW
    EXECUTE FUNCTION notify_k8s_resource_change();

-- HostBindings table (if exists)
DROP TRIGGER IF EXISTS notify_host_bindings_change ON host_bindings;
CREATE TRIGGER notify_host_bindings_change
    AFTER INSERT OR UPDATE OR DELETE ON host_bindings
    FOR EACH ROW
    EXECUTE FUNCTION notify_k8s_resource_change();

-- NetworkBindings table
CREATE TRIGGER notify_network_bindings_change
    AFTER INSERT OR UPDATE OR DELETE ON network_bindings
    FOR EACH ROW
    EXECUTE FUNCTION notify_k8s_resource_change();

-- ServiceAliases table
CREATE TRIGGER notify_service_aliases_change
    AFTER INSERT OR UPDATE OR DELETE ON service_aliases
    FOR EACH ROW
    EXECUTE FUNCTION notify_k8s_resource_change();

-- RuleS2S table
CREATE TRIGGER notify_rule_s2s_change
    AFTER INSERT OR UPDATE OR DELETE ON rule_s2s
    FOR EACH ROW
    EXECUTE FUNCTION notify_k8s_resource_change();

-- IE_AG_AG_Rules table
CREATE TRIGGER notify_ie_ag_ag_rules_change
    AFTER INSERT OR UPDATE OR DELETE ON ie_ag_ag_rules
    FOR EACH ROW
    EXECUTE FUNCTION notify_k8s_resource_change();

-- AddressGroupPortMappings table
CREATE TRIGGER notify_address_group_port_mappings_change
    AFTER INSERT OR UPDATE OR DELETE ON address_group_port_mappings
    FOR EACH ROW
    EXECUTE FUNCTION notify_k8s_resource_change();

-- SvcSvc_Rules table (if exists)
DROP TRIGGER IF EXISTS notify_svc_svc_rules_change ON svc_svc_rules;
CREATE TRIGGER notify_svc_svc_rules_change
    AFTER INSERT OR UPDATE OR DELETE ON svc_svc_rules
    FOR EACH ROW
    EXECUTE FUNCTION notify_k8s_resource_change();

-- SvcFQDN_Rules table (if exists)
DROP TRIGGER IF EXISTS notify_svc_fqdn_rules_change ON svc_fqdn_rules;
CREATE TRIGGER notify_svc_fqdn_rules_change
    AFTER INSERT OR UPDATE OR DELETE ON svc_fqdn_rules
    FOR EACH ROW
    EXECUTE FUNCTION notify_k8s_resource_change();

-- Create index on k8s_metadata for watch queries
-- This helps with efficient resourceVersion-based queries
CREATE INDEX IF NOT EXISTS idx_k8s_metadata_rv_created 
    ON k8s_metadata(resource_version DESC, created_at DESC);

-- Comment explaining the purpose
COMMENT ON FUNCTION notify_k8s_resource_change() IS 
'Sends PostgreSQL NOTIFY event on any K8s resource change. Used by watch mechanism for real-time updates.';

-- +goose Down
-- Remove all watch-related triggers and functions

-- Drop triggers
DROP TRIGGER IF EXISTS notify_hosts_change ON hosts;
DROP TRIGGER IF EXISTS notify_services_change ON services;
DROP TRIGGER IF EXISTS notify_address_groups_change ON address_groups;
DROP TRIGGER IF EXISTS notify_networks_change ON networks;
DROP TRIGGER IF EXISTS notify_address_group_bindings_change ON address_group_bindings;
DROP TRIGGER IF EXISTS notify_host_bindings_change ON host_bindings;
DROP TRIGGER IF EXISTS notify_network_bindings_change ON network_bindings;
DROP TRIGGER IF EXISTS notify_service_aliases_change ON service_aliases;
DROP TRIGGER IF EXISTS notify_rule_s2s_change ON rule_s2s;
DROP TRIGGER IF EXISTS notify_ie_ag_ag_rules_change ON ie_ag_ag_rules;
DROP TRIGGER IF EXISTS notify_address_group_port_mappings_change ON address_group_port_mappings;
DROP TRIGGER IF EXISTS notify_svc_svc_rules_change ON svc_svc_rules;
DROP TRIGGER IF EXISTS notify_svc_fqdn_rules_change ON svc_fqdn_rules;
-- Drop function
DROP FUNCTION IF EXISTS notify_k8s_resource_change();

-- Drop index
DROP INDEX IF EXISTS idx_k8s_metadata_rv_created;

