-- +goose Up

-- +goose StatementBegin

DO $$
BEGIN
    DROP TRIGGER IF EXISTS cascade_host_from_address_groups ON hosts;
    RAISE NOTICE 'Dropped existing cascade_host_from_address_groups trigger';
EXCEPTION
    WHEN OTHERS THEN
        RAISE NOTICE 'Trigger cascade_host_from_address_groups did not exist or could not be dropped: %', SQLERRM;
END $$;

DROP FUNCTION IF EXISTS cascade_host_deletion();

CREATE OR REPLACE FUNCTION cascade_host_deletion() RETURNS TRIGGER AS $$
DECLARE
    host_obj jsonb;
BEGIN
    host_obj := jsonb_build_object(
        'name', OLD.name,
        'apiVersion', 'netguard.sgroups.io/v1beta1',
        'kind', 'Host'
    );
    
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
    
    RAISE NOTICE 'Removed host %.% from all AddressGroups due to host deletion', OLD.namespace, OLD.name;
    
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'hosts') THEN
        DROP TRIGGER IF EXISTS cascade_host_from_address_groups ON hosts;
        
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
-- Remove cascade functionality completely for rollback

-- +goose StatementBegin

DROP TRIGGER IF EXISTS cascade_host_from_address_groups ON hosts;

DROP FUNCTION IF EXISTS cascade_host_deletion();

DO $$
BEGIN
    RAISE NOTICE 'Removed cascade_host_deletion function and trigger - no cascade functionality in pre-013 state';
END $$;

-- +goose StatementEnd