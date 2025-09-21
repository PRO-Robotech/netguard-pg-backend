-- +goose Up
-- Add triggers to automatically update host binding status when AddressGroup.spec.hosts changes

-- +goose StatementBegin

-- Function to update host binding status when AddressGroup.spec.hosts changes
CREATE OR REPLACE FUNCTION update_host_binding_status_on_spec_change() RETURNS TRIGGER AS $$
DECLARE
    old_hosts JSONB := COALESCE(OLD.hosts, '[]'::jsonb);
    new_hosts JSONB := COALESCE(NEW.hosts, '[]'::jsonb);
    host_ref JSONB;
    host_name TEXT;
BEGIN
    IF jsonb_typeof(old_hosts) != 'array' THEN
        old_hosts := '[]'::jsonb;
    END IF;
    IF jsonb_typeof(new_hosts) != 'array' THEN
        new_hosts := '[]'::jsonb;
    END IF;

    -- Only process if hosts field actually changed
    IF old_hosts = new_hosts THEN
        RETURN NEW;
    END IF;

    IF jsonb_typeof(old_hosts) = 'array' AND jsonb_array_length(old_hosts) > 0 THEN
        FOR host_ref IN SELECT jsonb_array_elements(old_hosts)
        LOOP
            host_name := host_ref->>'name';

            IF NOT (new_hosts @> jsonb_build_array(host_ref)) THEN
                UPDATE hosts
                SET
                    is_bound = false,
                    address_group_ref_namespace = NULL,
                    address_group_ref_name = NULL
                WHERE namespace = NEW.namespace::namespace_name
                AND name = host_name::resource_name
                AND is_bound = true
                AND address_group_ref_namespace = OLD.namespace
                AND address_group_ref_name = OLD.name;

                RAISE NOTICE 'Unbound host %.% from AddressGroup %.%', NEW.namespace, host_name, OLD.namespace, OLD.name;
            END IF;
        END LOOP;
    END IF;

    IF jsonb_typeof(new_hosts) = 'array' AND jsonb_array_length(new_hosts) > 0 THEN
        FOR host_ref IN SELECT jsonb_array_elements(new_hosts)
        LOOP
            host_name := host_ref->>'name';

            IF NOT (old_hosts @> jsonb_build_array(host_ref)) THEN
                UPDATE hosts
                SET
                    is_bound = true,
                    address_group_ref_namespace = NEW.namespace,
                    address_group_ref_name = NEW.name
                WHERE namespace = NEW.namespace::namespace_name
                AND name = host_name::resource_name;

                RAISE NOTICE 'Bound host %.% to AddressGroup %.%', NEW.namespace, host_name, NEW.namespace, NEW.name;
            END IF;
        END LOOP;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger on address_groups to update host binding status when spec.hosts changes
CREATE TRIGGER update_host_binding_status_on_spec_change_trigger
    AFTER UPDATE OF hosts ON address_groups
    FOR EACH ROW
    EXECUTE FUNCTION update_host_binding_status_on_spec_change();

-- Function to handle host binding status on AddressGroup deletion
CREATE OR REPLACE FUNCTION unbind_hosts_on_address_group_deletion() RETURNS TRIGGER AS $$
DECLARE
    host_ref JSONB;
    host_name TEXT;
    hosts_to_unbind JSONB := COALESCE(OLD.hosts, '[]'::jsonb);
BEGIN
    IF jsonb_typeof(hosts_to_unbind) != 'array' THEN
        hosts_to_unbind := '[]'::jsonb;
    END IF;

    IF hosts_to_unbind IS NOT NULL AND hosts_to_unbind != 'null'::jsonb
       AND jsonb_typeof(hosts_to_unbind) = 'array' AND jsonb_array_length(hosts_to_unbind) > 0 THEN
        FOR host_ref IN SELECT jsonb_array_elements(hosts_to_unbind)
        LOOP
            host_name := host_ref->>'name';

            -- Unbind the host: set is_bound = false and clear address_group_ref
            UPDATE hosts
            SET
                is_bound = false,
                address_group_ref_namespace = NULL,
                address_group_ref_name = NULL
            WHERE namespace = OLD.namespace::namespace_name
            AND name = host_name::resource_name
            AND is_bound = true
            AND address_group_ref_namespace = OLD.namespace
            AND address_group_ref_name = OLD.name;

            RAISE NOTICE 'Unbound host %.% from deleted AddressGroup %.%', OLD.namespace, host_name, OLD.namespace, OLD.name;
        END LOOP;
    END IF;

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

-- Trigger on address_groups to unbind hosts when AddressGroup is deleted
CREATE TRIGGER unbind_hosts_on_address_group_deletion_trigger
    BEFORE DELETE ON address_groups
    FOR EACH ROW
    EXECUTE FUNCTION unbind_hosts_on_address_group_deletion();

-- Add comment explaining the new functionality
COMMENT ON FUNCTION update_host_binding_status_on_spec_change() IS 'Automatically updates host binding status when AddressGroup.spec.hosts changes';
COMMENT ON FUNCTION unbind_hosts_on_address_group_deletion() IS 'Automatically unbinds hosts when AddressGroup is deleted';

-- +goose StatementEnd

-- +goose Down
-- Remove host binding status triggers

-- +goose StatementBegin

-- Drop triggers
DROP TRIGGER IF EXISTS update_host_binding_status_on_spec_change_trigger ON address_groups;
DROP TRIGGER IF EXISTS unbind_hosts_on_address_group_deletion_trigger ON address_groups;

-- Drop functions
DROP FUNCTION IF EXISTS update_host_binding_status_on_spec_change();
DROP FUNCTION IF EXISTS unbind_hosts_on_address_group_deletion();

-- +goose StatementEnd