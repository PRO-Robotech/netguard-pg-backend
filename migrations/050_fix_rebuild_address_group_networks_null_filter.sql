-- +goose Up
-- Migration 050: Fix NULL values in AddressGroup.networks aggregation
--
-- Problem: The rebuild_address_group_networks() function from Migration 020
-- doesn't filter NULL values, which can cause NULL entries in the networks array
-- when networks are deleted or have NULL cidr values.
--
-- Solution: Add FILTER (WHERE network_obj IS NOT NULL) to jsonb_agg
-- and defensive NULL checks on all required fields.
--
-- Pattern: Similar to Migration 049 (Host COALESCE fix)

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION rebuild_address_group_networks(ag_namespace TEXT, ag_name TEXT)
RETURNS JSONB AS $$
DECLARE
    networks_json JSONB;
BEGIN
    -- Use subquery + FILTER for NULL-safe aggregation
    SELECT COALESCE(
        jsonb_agg(network_obj) FILTER (WHERE network_obj IS NOT NULL),
        '[]'::jsonb
    )
    INTO networks_json
    FROM (
        SELECT jsonb_build_object(
            'name', n.name,
            'namespace', n.namespace,
            'cidr', n.cidr
        ) AS network_obj
        FROM network_bindings nb
        INNER JOIN networks n
            ON nb.network_namespace = n.namespace
            AND nb.network_name = n.name
        WHERE nb.address_group_namespace = ag_namespace
          AND nb.address_group_name = ag_name
          -- Defensive NULL checks to prevent NULL in array
          AND n.name IS NOT NULL
          AND n.namespace IS NOT NULL
          AND n.cidr IS NOT NULL
    ) subquery;

    RETURN networks_json;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Rebuild all AddressGroup networks to clean up any existing NULL values
UPDATE address_groups ag
SET networks = rebuild_address_group_networks(ag.namespace, ag.name);

-- +goose Down
-- +goose StatementBegin
-- Restore original function from Migration 020 (without NULL filtering)
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
