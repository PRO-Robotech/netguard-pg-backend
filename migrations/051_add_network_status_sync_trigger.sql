-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION sync_network_status_on_binding_change()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE networks
        SET is_bound = true,
            binding_ref_namespace = NEW.namespace,
            binding_ref_name = NEW.name,
            address_group_ref_namespace = NEW.address_group_namespace,
            address_group_ref_name = NEW.address_group_name
        WHERE namespace = NEW.network_namespace
          AND name = NEW.network_name;

    ELSIF TG_OP = 'DELETE' THEN
        UPDATE networks
        SET is_bound = false,
            binding_ref_namespace = NULL,
            binding_ref_name = NULL,
            address_group_ref_namespace = NULL,
            address_group_ref_name = NULL
        WHERE namespace = OLD.network_namespace
          AND name = OLD.network_name;

    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.network_namespace != NEW.network_namespace
           OR OLD.network_name != NEW.network_name THEN
            UPDATE networks
            SET is_bound = false,
                binding_ref_namespace = NULL,
                binding_ref_name = NULL,
                address_group_ref_namespace = NULL,
                address_group_ref_name = NULL
            WHERE namespace = OLD.network_namespace
              AND name = OLD.network_name;
        END IF;

        UPDATE networks
        SET is_bound = true,
            binding_ref_namespace = NEW.namespace,
            binding_ref_name = NEW.name,
            address_group_ref_namespace = NEW.address_group_namespace,
            address_group_ref_name = NEW.address_group_name
        WHERE namespace = NEW.network_namespace
          AND name = NEW.network_name;
    END IF;

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_sync_network_status_on_binding
AFTER INSERT OR UPDATE OR DELETE ON network_bindings
FOR EACH ROW
EXECUTE FUNCTION sync_network_status_on_binding_change();

UPDATE networks n
SET is_bound = true,
    binding_ref_namespace = nb.namespace,
    binding_ref_name = nb.name,
    address_group_ref_namespace = nb.address_group_namespace,
    address_group_ref_name = nb.address_group_name
FROM network_bindings nb
WHERE n.namespace = nb.network_namespace
  AND n.name = nb.network_name;

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_sync_network_status_on_binding ON network_bindings;

DROP FUNCTION IF EXISTS sync_network_status_on_binding_change();

UPDATE networks
SET is_bound = false,
    binding_ref_namespace = NULL,
    binding_ref_name = NULL,
    address_group_ref_namespace = NULL,
    address_group_ref_name = NULL;
-- +goose StatementEnd
