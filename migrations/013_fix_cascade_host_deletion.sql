-- +goose Up
-- Fix cascade host deletion function - replace invalid jsonb - jsonb operation

-- +goose StatementBegin

-- Drop existing function
DROP FUNCTION IF EXISTS cascade_host_deletion();

-- Create corrected function for cascading host removal from AddressGroups
CREATE OR REPLACE FUNCTION cascade_host_deletion() RETURNS TRIGGER AS $$
DECLARE
    host_obj jsonb;
BEGIN
    -- Build ObjectReference for the deleted host (matching the format stored in database)
    host_obj := jsonb_build_object(
        'name', OLD.name,
        'apiVersion', 'netguard.sgroups.io/v1beta1',
        'kind', 'Host'
    );
    
    -- Remove host from all AddressGroup.spec.hosts arrays using proper JSONB array manipulation
    UPDATE address_groups 
    SET hosts = COALESCE(
        (
            SELECT jsonb_agg(host_element)
            FROM jsonb_array_elements(hosts) AS host_element
            WHERE host_element != host_obj
        ), 
        '[]'::jsonb  -- Return empty array if all elements removed
    )
    WHERE hosts @> jsonb_build_array(host_obj)
    AND hosts IS NOT NULL;
    
    -- Log the cascade operation
    RAISE NOTICE 'Removed host %.% from all AddressGroups due to host deletion', OLD.namespace, OLD.name;
    
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

-- Recreate trigger for cascading host deletion (only if hosts table exists)
DO $$
BEGIN
    IF EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'hosts') THEN
        -- Drop existing trigger first
        DROP TRIGGER IF EXISTS cascade_host_from_address_groups ON hosts;
        
        -- Create new trigger
        CREATE TRIGGER cascade_host_from_address_groups
            BEFORE DELETE ON hosts
            FOR EACH ROW 
            EXECUTE FUNCTION cascade_host_deletion();
        RAISE NOTICE 'Recreated cascade_host_from_address_groups trigger on hosts table with fixed function';
    ELSE
        RAISE NOTICE 'Hosts table does not exist, skipping cascade_host_from_address_groups trigger creation';
    END IF;
END $$;

-- +goose StatementEnd

-- +goose Down
-- Revert to original (broken) function for rollback

-- +goose StatementBegin

-- Drop corrected function
DROP FUNCTION IF EXISTS cascade_host_deletion();

-- Restore original function (with the jsonb - jsonb issue)
CREATE OR REPLACE FUNCTION cascade_host_deletion() RETURNS TRIGGER AS $$
DECLARE
    host_obj jsonb;
BEGIN
    -- Build ObjectReference for the deleted host (original broken version)
    host_obj := jsonb_build_object(
        'name', OLD.name,
        'namespace', OLD.namespace,
        'apiVersion', 'netguard.io/v1beta1',
        'kind', 'Host'
    );
    
    -- Remove host from all AddressGroup.spec.hosts arrays (original broken version)
    UPDATE address_groups 
    SET hosts = hosts - host_obj
    WHERE hosts @> jsonb_build_array(host_obj);
    
    -- Log the cascade operation
    RAISE NOTICE 'Removed host %.% from all AddressGroups due to host deletion', OLD.namespace, OLD.name;
    
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

-- Recreate trigger
DO $$
BEGIN
    IF EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'hosts') THEN
        DROP TRIGGER IF EXISTS cascade_host_from_address_groups ON hosts;
        CREATE TRIGGER cascade_host_from_address_groups
            BEFORE DELETE ON hosts
            FOR EACH ROW 
            EXECUTE FUNCTION cascade_host_deletion();
        RAISE NOTICE 'Restored original cascade_host_from_address_groups trigger on hosts table';
    ELSE
        RAISE NOTICE 'Hosts table does not exist, skipping cascade_host_from_address_groups trigger creation';
    END IF;
END $$;

-- +goose StatementEnd