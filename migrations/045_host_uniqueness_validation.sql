-- +goose Up



-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_host_uniqueness(
    p_host_namespace TEXT,
    p_host_name TEXT,
    p_address_group_namespace TEXT,
    p_address_group_name TEXT,
    p_operation TEXT
)
RETURNS VOID AS $$
DECLARE
    v_existing_binding RECORD;
    v_spec_ag RECORD;
    v_host_full_name TEXT;
    v_ag_full_name TEXT;
BEGIN
    v_host_full_name := p_host_namespace || '/' || p_host_name;
    v_ag_full_name := p_address_group_namespace || '/' || p_address_group_name;

    SELECT
        hb.address_group_namespace,
        hb.address_group_name
    INTO v_existing_binding
    FROM host_bindings hb
    JOIN k8s_metadata m ON m.resource_version = hb.resource_version
    WHERE hb.host_namespace = p_host_namespace::namespace_name
    AND hb.host_name = p_host_name::resource_name
    AND m.deletion_timestamp IS NULL
    LIMIT 1;

    IF FOUND THEN
        RAISE EXCEPTION 'Host % already has HostBinding to AddressGroup %/%',
            v_host_full_name,
            v_existing_binding.address_group_namespace,
            v_existing_binding.address_group_name
        USING HINT = 'Each Host can only be bound to ONE AddressGroup',
              ERRCODE = '23505';
    END IF;

    SELECT
        ag.namespace,
        ag.name
    INTO v_spec_ag
    FROM address_groups ag,
    LATERAL jsonb_array_elements(COALESCE(ag.hosts, '[]'::jsonb)) as host_elem
    WHERE jsonb_typeof(ag.hosts) = 'array'
    AND host_elem->>'namespace' = p_host_namespace
    AND host_elem->>'name' = p_host_name
    LIMIT 1;

    IF FOUND THEN
        RAISE EXCEPTION 'Host % already exists in spec.hosts of AddressGroup %/%',
            v_host_full_name,
            v_spec_ag.namespace,
            v_spec_ag.name
        USING HINT = 'Remove Host from spec.hosts before creating HostBinding',
              ERRCODE = '23505';
    END IF;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION validate_host_uniqueness(TEXT, TEXT, TEXT, TEXT, TEXT) IS
'Validates that a Host can only be attached to ONE AddressGroup via ONE method.
Raises exception if Host already has HostBinding or exists in spec.hosts.';
-- +goose StatementEnd


-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_validate_host_binding_uniqueness()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM validate_host_uniqueness(
        NEW.host_namespace::text,
        NEW.host_name::text,
        NEW.address_group_namespace::text,
        NEW.address_group_name::text,
        'binding_insert'
    );

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_validate_host_binding_uniqueness ON host_bindings;

CREATE TRIGGER trigger_validate_host_binding_uniqueness
BEFORE INSERT ON host_bindings
FOR EACH ROW
EXECUTE FUNCTION trigger_validate_host_binding_uniqueness();

COMMENT ON TRIGGER trigger_validate_host_binding_uniqueness ON host_bindings IS
'Prevents creating HostBinding if Host already has binding or exists in spec.hosts';
-- +goose StatementEnd


-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_validate_address_group_hosts_uniqueness()
RETURNS TRIGGER AS $$
DECLARE
    old_hosts_json JSONB;
    new_hosts_json JSONB;
    host_elem JSONB;
    v_host_namespace TEXT;
    v_host_name TEXT;
BEGIN
    IF NEW.hosts IS DISTINCT FROM OLD.hosts THEN
        old_hosts_json := COALESCE(OLD.hosts, '[]'::jsonb);
        new_hosts_json := COALESCE(NEW.hosts, '[]'::jsonb);

        FOR host_elem IN
            SELECT * FROM jsonb_array_elements(new_hosts_json)
        LOOP
            IF NOT EXISTS (
                SELECT 1 FROM jsonb_array_elements(old_hosts_json) as old_elem
                WHERE old_elem->>'namespace' = host_elem->>'namespace'
                AND old_elem->>'name' = host_elem->>'name'
            ) THEN
                v_host_namespace := host_elem->>'namespace';
                v_host_name := host_elem->>'name';

                PERFORM validate_host_uniqueness(
                    v_host_namespace,
                    v_host_name,
                    NEW.namespace::text,
                    NEW.name::text,
                    'spec_update'
                );
            END IF;
        END LOOP;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_validate_address_group_hosts_uniqueness ON address_groups;

CREATE TRIGGER trigger_validate_address_group_hosts_uniqueness
BEFORE UPDATE ON address_groups
FOR EACH ROW
EXECUTE FUNCTION trigger_validate_address_group_hosts_uniqueness();

COMMENT ON TRIGGER trigger_validate_address_group_hosts_uniqueness ON address_groups IS
'Prevents adding Host to spec.hosts if Host already has HostBinding or exists in another AG spec.hosts';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS trigger_validate_address_group_hosts_uniqueness ON address_groups;
DROP TRIGGER IF EXISTS trigger_validate_host_binding_uniqueness ON host_bindings;
DROP FUNCTION IF EXISTS trigger_validate_address_group_hosts_uniqueness();
DROP FUNCTION IF EXISTS trigger_validate_host_binding_uniqueness();
DROP FUNCTION IF EXISTS validate_host_uniqueness(TEXT, TEXT, TEXT, TEXT, TEXT);
-- +goose StatementEnd
