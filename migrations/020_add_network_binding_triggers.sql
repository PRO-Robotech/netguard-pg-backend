-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION rebuild_address_group_networks(ag_namespace TEXT, ag_name TEXT)
RETURNS JSONB AS $$
DECLARE
    networks_json JSONB;
BEGIN
    SELECT COALESCE(jsonb_agg(
        jsonb_build_object(
            'name', n.name,
            'namespace', n.namespace,
            'cidr', n.cidr
        )
    ), '[]'::jsonb)
    INTO networks_json
    FROM network_bindings nb
    INNER JOIN networks n ON nb.network_namespace = n.namespace AND nb.network_name = n.name
    WHERE nb.address_group_namespace = ag_namespace
      AND nb.address_group_name = ag_name;

    RETURN networks_json;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION sync_address_group_networks_on_binding_change()
RETURNS TRIGGER AS $$
DECLARE
    ag_namespace TEXT;
    ag_name TEXT;
    new_networks JSONB;
BEGIN
    IF TG_OP = 'DELETE' THEN
        ag_namespace := OLD.address_group_namespace;
        ag_name := OLD.address_group_name;
    ELSE
        ag_namespace := NEW.address_group_namespace;
        ag_name := NEW.address_group_name;
    END IF;

    new_networks := rebuild_address_group_networks(ag_namespace, ag_name);

    UPDATE address_groups
    SET networks = new_networks
    WHERE namespace = ag_namespace
      AND name = ag_name;

    -- If this was an UPDATE that changed the AddressGroup reference, also update the old AddressGroup
    IF TG_OP = 'UPDATE' AND (OLD.address_group_namespace != NEW.address_group_namespace OR OLD.address_group_name != NEW.address_group_name) THEN
        new_networks := rebuild_address_group_networks(OLD.address_group_namespace, OLD.address_group_name);
        UPDATE address_groups
        SET networks = new_networks
        WHERE namespace = OLD.address_group_namespace
          AND name = OLD.address_group_name;
    END IF;

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_sync_address_group_networks
    AFTER INSERT OR UPDATE OR DELETE ON network_bindings
    FOR EACH ROW
    EXECUTE FUNCTION sync_address_group_networks_on_binding_change();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION sync_address_group_networks_on_network_change()
RETURNS TRIGGER AS $$
DECLARE
    ag_record RECORD;
    new_networks JSONB;
BEGIN
    FOR ag_record IN
        SELECT DISTINCT nb.address_group_namespace, nb.address_group_name
        FROM network_bindings nb
        WHERE nb.network_namespace = COALESCE(NEW.namespace, OLD.namespace)
          AND nb.network_name = COALESCE(NEW.name, OLD.name)
    LOOP
        new_networks := rebuild_address_group_networks(ag_record.address_group_namespace, ag_record.address_group_name);

        UPDATE address_groups
        SET networks = new_networks
        WHERE namespace = ag_record.address_group_namespace
          AND name = ag_record.address_group_name;
    END LOOP;

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_sync_address_group_networks_on_network
    AFTER INSERT OR UPDATE OR DELETE ON networks
    FOR EACH ROW
    EXECUTE FUNCTION sync_address_group_networks_on_network_change();

UPDATE address_groups ag
SET networks = rebuild_address_group_networks(ag.namespace, ag.name);

-- +goose Down
-- +goose StatementBegin
-- Drop triggers
DROP TRIGGER IF EXISTS trg_sync_address_group_networks ON network_bindings;
DROP TRIGGER IF EXISTS trg_sync_address_group_networks_on_network ON networks;

-- Drop functions
DROP FUNCTION IF EXISTS sync_address_group_networks_on_binding_change();
DROP FUNCTION IF EXISTS sync_address_group_networks_on_network_change();
DROP FUNCTION IF EXISTS rebuild_address_group_networks(TEXT, TEXT);
-- +goose StatementEnd
