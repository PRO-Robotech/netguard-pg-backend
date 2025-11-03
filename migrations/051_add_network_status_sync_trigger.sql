-- +goose Up
-- Migration 051: Add Network.status synchronization with NetworkBinding
--
-- Problem: When NetworkBinding is created/deleted, Network.status fields
-- (is_bound, binding_ref_*, address_group_ref_*) are not updated automatically.
-- This causes Network.spec.isBound to remain false even when bound.
--
-- Solution: Add AFTER trigger on network_bindings to update Network.status
-- when binding is created, updated, or deleted.
--
-- Pattern: Similar to Migration 044 (Host binding status updates)

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION sync_network_status_on_binding_change()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        -- NetworkBinding created → Bind Network
        UPDATE networks
        SET is_bound = true,
            binding_ref_namespace = NEW.namespace,
            binding_ref_name = NEW.name,
            address_group_ref_namespace = NEW.address_group_namespace,
            address_group_ref_name = NEW.address_group_name
        WHERE namespace = NEW.network_namespace
          AND name = NEW.network_name;

    ELSIF TG_OP = 'DELETE' THEN
        -- NetworkBinding deleted → Unbind Network
        UPDATE networks
        SET is_bound = false,
            binding_ref_namespace = NULL,
            binding_ref_name = NULL,
            address_group_ref_namespace = NULL,
            address_group_ref_name = NULL
        WHERE namespace = OLD.network_namespace
          AND name = OLD.network_name;

    ELSIF TG_OP = 'UPDATE' THEN
        -- If Network reference changed → Unbind old Network, bind new Network
        IF OLD.network_namespace != NEW.network_namespace
           OR OLD.network_name != NEW.network_name THEN
            -- Unbind old Network
            UPDATE networks
            SET is_bound = false,
                binding_ref_namespace = NULL,
                binding_ref_name = NULL,
                address_group_ref_namespace = NULL,
                address_group_ref_name = NULL
            WHERE namespace = OLD.network_namespace
              AND name = OLD.network_name;
        END IF;

        -- Bind new/current Network
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

-- Create trigger on network_bindings
CREATE TRIGGER trg_sync_network_status_on_binding
AFTER INSERT OR UPDATE OR DELETE ON network_bindings
FOR EACH ROW
EXECUTE FUNCTION sync_network_status_on_binding_change();

-- Initialize Network.status for existing NetworkBindings
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
-- Drop trigger
DROP TRIGGER IF EXISTS trg_sync_network_status_on_binding ON network_bindings;

-- Drop function
DROP FUNCTION IF EXISTS sync_network_status_on_binding_change();

-- Reset Network.status to default (unbind all)
UPDATE networks
SET is_bound = false,
    binding_ref_namespace = NULL,
    binding_ref_name = NULL,
    address_group_ref_namespace = NULL,
    address_group_ref_name = NULL;
-- +goose StatementEnd
