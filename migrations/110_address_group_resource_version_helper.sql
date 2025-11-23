-- +goose Up
-- Introduce a helper that bumps AddressGroup resourceVersion in a controlled way,
-- and update the aggregation triggers to call it whenever they rewrite spec-derived fields.

-- +goose StatementBegin
-- Helper: bump metadata RV and return the new value
CREATE OR REPLACE FUNCTION bump_address_group_resource_version(ag_namespace TEXT, ag_name TEXT)
RETURNS BIGINT AS $$
DECLARE
    old_rv BIGINT;
    new_rv BIGINT;
BEGIN
    SELECT resource_version INTO old_rv
    FROM address_groups
    WHERE namespace = ag_namespace::namespace_name
      AND name = ag_name::resource_name
    FOR UPDATE;

    IF old_rv IS NULL THEN
        RAISE EXCEPTION 'AddressGroup %.% not found', ag_namespace, ag_name;
    END IF;

    UPDATE k8s_metadata
    SET updated_at = NOW()
    WHERE resource_version = old_rv
    RETURNING resource_version INTO new_rv;

    RETURN new_rv;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- Ensure aggregated_hosts updates always bump RV
CREATE OR REPLACE FUNCTION update_aggregated_hosts_for_address_group(ag_namespace TEXT, ag_name TEXT)
RETURNS VOID AS $$
DECLARE
    new_rv BIGINT;
BEGIN
    new_rv := bump_address_group_resource_version(ag_namespace, ag_name);

    UPDATE address_groups
    SET aggregated_hosts = aggregate_address_group_hosts(ag_namespace, ag_name),
        resource_version = new_rv
    WHERE namespace = ag_namespace::namespace_name
      AND name = ag_name::resource_name;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- Update binding-triggered network aggregation
CREATE OR REPLACE FUNCTION sync_address_group_networks_on_binding_change()
RETURNS TRIGGER AS $$
DECLARE
    ag_namespace TEXT;
    ag_name TEXT;
    new_networks JSONB;
    new_rv BIGINT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        ag_namespace := OLD.address_group_namespace;
        ag_name := OLD.address_group_name;
    ELSE
        ag_namespace := NEW.address_group_namespace;
        ag_name := NEW.address_group_name;
    END IF;

    new_networks := rebuild_address_group_networks(ag_namespace, ag_name);
    new_rv := bump_address_group_resource_version(ag_namespace, ag_name);

    UPDATE address_groups
    SET networks = new_networks,
        resource_version = new_rv
    WHERE namespace = ag_namespace
      AND name = ag_name;

    -- If the binding moved to another AddressGroup, update the old one too
    IF TG_OP = 'UPDATE' AND (OLD.address_group_namespace != NEW.address_group_namespace OR OLD.address_group_name != NEW.address_group_name) THEN
        new_networks := rebuild_address_group_networks(OLD.address_group_namespace, OLD.address_group_name);
        new_rv := bump_address_group_resource_version(OLD.address_group_namespace, OLD.address_group_name);

        UPDATE address_groups
        SET networks = new_networks,
            resource_version = new_rv
        WHERE namespace = OLD.address_group_namespace
          AND name = OLD.address_group_name;
    END IF;

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- Update network-triggered aggregation
CREATE OR REPLACE FUNCTION sync_address_group_networks_on_network_change()
RETURNS TRIGGER AS $$
DECLARE
    ag_record RECORD;
    new_networks JSONB;
    new_rv BIGINT;
BEGIN
    FOR ag_record IN
        SELECT DISTINCT nb.address_group_namespace, nb.address_group_name
        FROM network_bindings nb
        WHERE nb.network_namespace = COALESCE(NEW.namespace, OLD.namespace)
          AND nb.network_name = COALESCE(NEW.name, OLD.name)
    LOOP
        new_networks := rebuild_address_group_networks(ag_record.address_group_namespace, ag_record.address_group_name);
        new_rv := bump_address_group_resource_version(ag_record.address_group_namespace, ag_record.address_group_name);

        UPDATE address_groups
        SET networks = new_networks,
            resource_version = new_rv
        WHERE namespace = ag_record.address_group_namespace
          AND name = ag_record.address_group_name;
    END LOOP;

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
-- Restore previous definitions (without explicit RV bumps) and drop helper.

DROP FUNCTION IF EXISTS bump_address_group_resource_version(TEXT, TEXT);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_aggregated_hosts_for_address_group(ag_namespace TEXT, ag_name TEXT)
RETURNS VOID AS $$
BEGIN
    UPDATE address_groups
    SET aggregated_hosts = aggregate_address_group_hosts(ag_namespace, ag_name)
    WHERE namespace = ag_namespace::namespace_name
      AND name = ag_name::resource_name;
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

