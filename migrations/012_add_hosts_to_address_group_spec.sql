-- +goose Up
-- Add hosts field to AddressGroup spec for direct host management without HostBinding

-- +goose StatementBegin

-- Add hosts JSONB field to address_groups table
ALTER TABLE address_groups ADD COLUMN hosts JSONB NOT NULL DEFAULT '[]';

-- Create index for efficient host searching in AddressGroups
CREATE INDEX idx_address_groups_hosts ON address_groups USING GIN(hosts);

-- Function to check host exclusivity across AddressGroups
-- Ensures that each host can belong to only one AddressGroup
CREATE OR REPLACE FUNCTION check_host_exclusivity() RETURNS TRIGGER AS $$
BEGIN
    -- Check if any host in NEW.hosts already belongs to another AddressGroup
    IF EXISTS (
        SELECT 1 FROM address_groups 
        WHERE (namespace, name) != (NEW.namespace, NEW.name)
        AND hosts ?| ARRAY(
            SELECT jsonb_array_elements_text(
                jsonb_path_query_array(NEW.hosts, '$[*].name')
            )
        )
        AND hosts ?| ARRAY(
            SELECT jsonb_array_elements_text(
                jsonb_path_query_array(NEW.hosts, '$[*].namespace')
            )
        )
    ) THEN
        RAISE EXCEPTION 'Host already belongs to another AddressGroup - each host can belong to only one AddressGroup';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to enforce host exclusivity on INSERT/UPDATE
CREATE TRIGGER enforce_host_exclusivity 
    BEFORE INSERT OR UPDATE ON address_groups
    FOR EACH ROW 
    WHEN (NEW.hosts != '[]'::jsonb)
    EXECUTE FUNCTION check_host_exclusivity();

-- Function for cascading host removal from AddressGroups when host is deleted
CREATE OR REPLACE FUNCTION cascade_host_deletion() RETURNS TRIGGER AS $$
DECLARE
    host_obj jsonb;
BEGIN
    -- Build ObjectReference for the deleted host
    host_obj := jsonb_build_object(
        'name', OLD.name,
        'namespace', OLD.namespace,
        'apiVersion', 'netguard.sgroups.io/v1beta1',
        'kind', 'Host'
    );
    
    -- Remove host from all AddressGroup.spec.hosts arrays
    UPDATE address_groups 
    SET hosts = hosts - host_obj
    WHERE hosts @> jsonb_build_array(host_obj);
    
    -- Log the cascade operation
    RAISE NOTICE 'Removed host %.% from all AddressGroups due to host deletion', OLD.namespace, OLD.name;
    
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

-- Trigger for cascading host deletion (only if hosts table exists)
DO $$
BEGIN
    IF EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'hosts') THEN
        CREATE TRIGGER cascade_host_from_address_groups
            BEFORE DELETE ON hosts
            FOR EACH ROW 
            EXECUTE FUNCTION cascade_host_deletion();
        RAISE NOTICE 'Created cascade_host_from_address_groups trigger on hosts table';
    ELSE
        RAISE NOTICE 'Hosts table does not exist, skipping cascade_host_from_address_groups trigger creation';
    END IF;
END $$;

-- Add comment explaining the new hosts field
COMMENT ON COLUMN address_groups.hosts IS 'ObjectReference[] - List of hosts that belong exclusively to this AddressGroup';

-- +goose StatementEnd

-- +goose Down
-- Remove hosts functionality from AddressGroups

-- +goose StatementBegin

-- Drop triggers (with table existence check)
DO $$
BEGIN
    IF EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'hosts') THEN
        DROP TRIGGER IF EXISTS cascade_host_from_address_groups ON hosts;
        RAISE NOTICE 'Dropped cascade_host_from_address_groups trigger from hosts table';
    ELSE
        RAISE NOTICE 'Hosts table does not exist, skipping cascade_host_from_address_groups trigger removal';
    END IF;
END $$;

DROP TRIGGER IF EXISTS enforce_host_exclusivity ON address_groups;

-- Drop functions
DROP FUNCTION IF EXISTS cascade_host_deletion();
DROP FUNCTION IF EXISTS check_host_exclusivity();

-- Drop index
DROP INDEX IF EXISTS idx_address_groups_hosts;

-- Remove hosts column
ALTER TABLE address_groups DROP COLUMN IF EXISTS hosts;

-- +goose StatementEnd