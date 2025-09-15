-- +goose Up
-- Add aggregated_hosts column to address_groups table with automatic aggregation via triggers

-- +goose StatementBegin

-- Add aggregated_hosts JSONB field to address_groups table
ALTER TABLE address_groups ADD COLUMN aggregated_hosts JSONB NOT NULL DEFAULT '[]';

-- Create index for efficient host searching in aggregated_hosts
CREATE INDEX idx_address_groups_aggregated_hosts ON address_groups USING GIN(aggregated_hosts);

-- Function to aggregate hosts from both spec.hosts and HostBindings
-- Returns JSONB array with HostReference objects
CREATE OR REPLACE FUNCTION aggregate_address_group_hosts(ag_namespace TEXT, ag_name TEXT) RETURNS JSONB AS $$
DECLARE
    aggregated_hosts_json JSONB := '[]'::jsonb;
    host_ref JSONB;
    host_record RECORD;
    hosts_field JSONB;
BEGIN
    -- Get hosts field, handling null case
    SELECT COALESCE(hosts, '[]'::jsonb) INTO hosts_field
    FROM address_groups 
    WHERE namespace = ag_namespace AND name = ag_name;
    
    -- Add hosts from spec.hosts with source = "spec" only if hosts is not null/empty
    IF hosts_field IS NOT NULL AND hosts_field != 'null'::jsonb AND jsonb_array_length(hosts_field) > 0 THEN
        FOR host_ref IN
            SELECT jsonb_array_elements(hosts_field) as host_obj
        LOOP
            -- Get host UUID from hosts table
            SELECT h.uuid INTO host_record
            FROM hosts h
            WHERE h.namespace = ag_namespace::namespace_name 
            AND h.name = (host_ref->>'name')::resource_name;
            
            -- Add host reference with source information
            aggregated_hosts_json := aggregated_hosts_json || jsonb_build_array(
                jsonb_build_object(
                    'ref', host_ref,
                    'uuid', COALESCE(host_record.uuid, ''),
                    'source', 'spec'
                )
            );
        END LOOP;
    END IF;
    
    -- Add hosts from HostBindings with source = "binding"
    FOR host_record IN
        SELECT h.namespace, h.name, h.uuid
        FROM host_bindings hb
        JOIN hosts h ON h.namespace = hb.host_namespace AND h.name = hb.host_name
        WHERE hb.address_group_namespace = ag_namespace::namespace_name 
        AND hb.address_group_name = ag_name::resource_name
    LOOP
        -- Add host reference with source information
        aggregated_hosts_json := aggregated_hosts_json || jsonb_build_array(
            jsonb_build_object(
                'ref', jsonb_build_object(
                    'apiVersion', 'netguard.sgroups.io/v1beta1',
                    'kind', 'Host',
                    'name', host_record.name
                ),
                'uuid', host_record.uuid,
                'source', 'binding'
            )
        );
    END LOOP;
    
    RETURN aggregated_hosts_json;
END;
$$ LANGUAGE plpgsql;

-- Function to update aggregated_hosts for specific AddressGroup
CREATE OR REPLACE FUNCTION update_aggregated_hosts_for_address_group(ag_namespace TEXT, ag_name TEXT) RETURNS VOID AS $$
BEGIN
    UPDATE address_groups 
    SET aggregated_hosts = aggregate_address_group_hosts(ag_namespace, ag_name)
    WHERE namespace = ag_namespace::namespace_name AND name = ag_name::resource_name;
    
    RAISE NOTICE 'Updated aggregated_hosts for AddressGroup %.%', ag_namespace, ag_name;
END;
$$ LANGUAGE plpgsql;

-- Function to validate HostBinding doesn't conflict with existing bindings or spec.hosts
CREATE OR REPLACE FUNCTION validate_host_binding_conflicts() RETURNS TRIGGER AS $$
DECLARE
    conflicting_ag RECORD;
    host_in_spec BOOLEAN := false;
BEGIN
    -- Check if host is already in spec.hosts of ANY AddressGroup (including the target one)
    SELECT EXISTS(
        SELECT 1 FROM address_groups ag
        WHERE ag.hosts @> jsonb_build_array(
            jsonb_build_object(
                'apiVersion', 'netguard.sgroups.io/v1beta1',
                'kind', 'Host',
                'name', NEW.host_name
            )
        )
    ) INTO host_in_spec;

    IF host_in_spec THEN
        -- Find which AddressGroup contains this host in spec.hosts
        SELECT ag.namespace, ag.name INTO conflicting_ag
        FROM address_groups ag
        WHERE ag.hosts @> jsonb_build_array(
            jsonb_build_object(
                'apiVersion', 'netguard.sgroups.io/v1beta1',
                'kind', 'Host',
                'name', NEW.host_name
            )
        )
        LIMIT 1;

        RAISE EXCEPTION 'Host %.% already belongs to AddressGroup %.% via spec.hosts - cannot create HostBinding',
            NEW.host_namespace, NEW.host_name, conflicting_ag.namespace, conflicting_ag.name;
    END IF;
    
    -- Check if host is already bound to a different AddressGroup via HostBinding
    SELECT ag.namespace, ag.name INTO conflicting_ag
    FROM host_bindings hb
    JOIN address_groups ag ON ag.namespace = hb.address_group_namespace AND ag.name = hb.address_group_name
    WHERE hb.host_namespace = NEW.host_namespace 
    AND hb.host_name = NEW.host_name
    AND (hb.address_group_namespace != NEW.address_group_namespace OR hb.address_group_name != NEW.address_group_name)
    LIMIT 1;
    
    IF FOUND THEN
        RAISE EXCEPTION 'Host %.% already belongs to AddressGroup %.% via HostBinding - each host can belong to only one AddressGroup', 
            NEW.host_namespace, NEW.host_name, conflicting_ag.namespace, conflicting_ag.name;
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Wrapper trigger function for address_groups
CREATE OR REPLACE FUNCTION trigger_update_aggregated_hosts_on_spec_change() RETURNS TRIGGER AS $$
BEGIN
    -- Update aggregated_hosts with separate UPDATE statement after the main operation
    UPDATE address_groups
    SET aggregated_hosts = aggregate_address_group_hosts(NEW.namespace, NEW.name)
    WHERE namespace = NEW.namespace AND name = NEW.name;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger on address_groups to update aggregated_hosts when spec.hosts changes
CREATE TRIGGER update_aggregated_hosts_on_spec_change
    AFTER INSERT OR UPDATE OF hosts ON address_groups
    FOR EACH ROW
    EXECUTE FUNCTION trigger_update_aggregated_hosts_on_spec_change();

-- Trigger on host_bindings to validate conflicts before INSERT/UPDATE
CREATE TRIGGER validate_host_binding_conflicts_trigger
    BEFORE INSERT OR UPDATE ON host_bindings
    FOR EACH ROW
    EXECUTE FUNCTION validate_host_binding_conflicts();

-- Trigger function for host_bindings to update aggregated_hosts after changes
CREATE OR REPLACE FUNCTION trigger_update_aggregated_hosts_on_binding_change() RETURNS TRIGGER AS $$
BEGIN
    -- Update aggregated_hosts for the affected AddressGroup(s)
    IF TG_OP = 'DELETE' THEN
        UPDATE address_groups 
        SET aggregated_hosts = aggregate_address_group_hosts(OLD.address_group_namespace, OLD.address_group_name)
        WHERE namespace = OLD.address_group_namespace AND name = OLD.address_group_name;
        RETURN OLD;
    ELSE
        UPDATE address_groups 
        SET aggregated_hosts = aggregate_address_group_hosts(NEW.address_group_namespace, NEW.address_group_name)
        WHERE namespace = NEW.address_group_namespace AND name = NEW.address_group_name;
        
        -- If UPDATE changed the AddressGroup reference, also update the old one
        IF TG_OP = 'UPDATE' AND (OLD.address_group_namespace != NEW.address_group_namespace OR OLD.address_group_name != NEW.address_group_name) THEN
            UPDATE address_groups 
            SET aggregated_hosts = aggregate_address_group_hosts(OLD.address_group_namespace, OLD.address_group_name)
            WHERE namespace = OLD.address_group_namespace AND name = OLD.address_group_name;
        END IF;
        
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_aggregated_hosts_on_binding_change_trigger
    AFTER INSERT OR UPDATE OR DELETE ON host_bindings
    FOR EACH ROW
    EXECUTE FUNCTION trigger_update_aggregated_hosts_on_binding_change();

-- Initialize aggregated_hosts for existing AddressGroups
UPDATE address_groups 
SET aggregated_hosts = aggregate_address_group_hosts(namespace, name);

-- Add comment explaining the new aggregated_hosts field
COMMENT ON COLUMN address_groups.aggregated_hosts IS 'HostReference[] - Aggregated list of all hosts from both spec.hosts and HostBindings with source tracking';

-- +goose StatementEnd

-- +goose Down
-- Remove aggregated_hosts functionality

-- +goose StatementBegin

-- Drop triggers
DROP TRIGGER IF EXISTS update_aggregated_hosts_on_spec_change ON address_groups;
DROP TRIGGER IF EXISTS validate_host_binding_conflicts_trigger ON host_bindings;
DROP TRIGGER IF EXISTS update_aggregated_hosts_on_binding_change_trigger ON host_bindings;

-- Drop functions
DROP FUNCTION IF EXISTS trigger_update_aggregated_hosts_on_binding_change();
DROP FUNCTION IF EXISTS trigger_update_aggregated_hosts_on_spec_change();
DROP FUNCTION IF EXISTS validate_host_binding_conflicts();
DROP FUNCTION IF EXISTS update_aggregated_hosts_for_address_group(TEXT, TEXT);
DROP FUNCTION IF EXISTS aggregate_address_group_hosts(TEXT, TEXT);

-- Drop index
DROP INDEX IF EXISTS idx_address_groups_aggregated_hosts;

-- Remove aggregated_hosts column
ALTER TABLE address_groups DROP COLUMN IF EXISTS aggregated_hosts;

-- +goose StatementEnd