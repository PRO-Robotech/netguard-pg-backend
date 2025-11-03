-- +goose Up
-- +goose StatementBegin

-- Migration 092: Filter soft-deleted NetworkBindings when rebuilding AddressGroup.networks
-- Problem: rebuild_address_group_networks() aggregated networks from bindings regardless of
--          deletion_timestamp, поэтому мягко удалённые NetworkBinding продолжали появляться
--          в AddressGroup.networks и, следовательно, просачивались в SGROUP payload.
-- Решение: добавляем JOIN на k8s_metadata для network_bindings и networks и фильтруем
--           записи с deletion_timestamp IS NULL, повторяя паттерн Migration 089 для hosts.

CREATE OR REPLACE FUNCTION rebuild_address_group_networks(ag_namespace TEXT, ag_name TEXT)
RETURNS JSONB AS $$
DECLARE
    networks_json JSONB;
BEGIN
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
        JOIN k8s_metadata nb_meta ON nb_meta.resource_version = nb.resource_version
        JOIN networks n ON n.namespace = nb.network_namespace AND n.name = nb.network_name
        JOIN k8s_metadata n_meta ON n_meta.resource_version = n.resource_version
        WHERE nb.address_group_namespace = ag_namespace::namespace_name
          AND nb.address_group_name = ag_name::resource_name
          AND nb_meta.deletion_timestamp IS NULL
          AND n_meta.deletion_timestamp IS NULL
          AND n.name IS NOT NULL
          AND n.namespace IS NOT NULL
          AND n.cidr IS NOT NULL
    ) subquery;

    RETURN networks_json;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION rebuild_address_group_networks(TEXT, TEXT) IS
'Aggregates networks from active NetworkBindings only (deletion_timestamp IS NULL for binding and network).';

-- Обновляем существующие записи, чтобы очистить массивы от мягко удалённых сетей
UPDATE address_groups ag
SET networks = rebuild_address_group_networks(ag.namespace, ag.name);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Восстанавливаем предыдущую версию функции (без проверки deletion_timestamp)
CREATE OR REPLACE FUNCTION rebuild_address_group_networks(ag_namespace TEXT, ag_name TEXT)
RETURNS JSONB AS $$
DECLARE
    networks_json JSONB;
BEGIN
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
          AND n.name IS NOT NULL
          AND n.namespace IS NOT NULL
          AND n.cidr IS NOT NULL
    ) subquery;

    RETURN networks_json;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION rebuild_address_group_networks(TEXT, TEXT) IS
'Aggregates networks from NetworkBindings without filtering soft-deleted bindings.';

-- Пересчитываем networks по старой логике
UPDATE address_groups ag
SET networks = rebuild_address_group_networks(ag.namespace, ag.name);

-- +goose StatementEnd

