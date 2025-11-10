-- +goose Up
-- +goose StatementBegin


CREATE OR REPLACE FUNCTION cascade_host_deletion() RETURNS TRIGGER AS $$
DECLARE
    host_obj jsonb;
BEGIN
    host_obj := jsonb_build_object(
        'name', OLD.name,
        'namespace', OLD.namespace,
        'apiVersion', 'netguard.sgroups.io/v1beta1',
        'kind', 'Host'
    );

    UPDATE address_groups
    SET hosts = COALESCE(
        (
            SELECT jsonb_agg(host_item)
            FROM jsonb_array_elements(hosts) AS host_item
            WHERE host_item->>'name' != OLD.name
               OR host_item->>'namespace' != OLD.namespace
        ),
        '[]'::jsonb
    )
    WHERE hosts @> jsonb_build_array(host_obj)
       OR EXISTS (
           SELECT 1
           FROM jsonb_array_elements(hosts) AS h
           WHERE h->>'name' = OLD.name
             AND h->>'namespace' = OLD.namespace
       );

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION cascade_host_deletion() RETURNS TRIGGER AS $$
DECLARE
    host_obj jsonb;
BEGIN
    host_obj := jsonb_build_object(
        'name', OLD.name,
        'namespace', OLD.namespace,
        'apiVersion', 'netguard.sgroups.io/v1beta1',
        'kind', 'Host'
    );

    UPDATE address_groups
    SET hosts = (
        SELECT jsonb_agg(host_item)
        FROM jsonb_array_elements(hosts) AS host_item
        WHERE host_item->>'name' != OLD.name
           OR host_item->>'namespace' != OLD.namespace
    )
    WHERE hosts @> jsonb_build_array(host_obj)
       OR EXISTS (
           SELECT 1
           FROM jsonb_array_elements(hosts) AS h
           WHERE h->>'name' = OLD.name
             AND h->>'namespace' = OLD.namespace
       );

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd
