-- +goose Up
-- +goose StatementBegin

-- КРИТИЧЕСКИЙ БАГ-ФИКС: cascade_host_deletion() возвращает NULL вместо пустого массива
--
-- ПРОБЛЕМА:
-- - В миграции 039 функция cascade_host_deletion() была изменена
-- - Старая версия использовала JSONB оператор "-" который работал корректно
-- - Новая версия использует jsonb_agg() который возвращает NULL если нет элементов
-- - Это нарушает NOT NULL constraint на колонке hosts
--
-- СИМПТОМЫ:
-- - При удалении Host который является последним в AG.spec.hosts[], возникает ошибка:
--   "ERROR: null value in column "hosts" of relation "address_groups" violates not-null constraint"
-- - Host зависает с deletionTimestamp но никогда не удаляется
-- - OutboxWorker бесконечно повторяет операцию DELETE
--
-- РЕШЕНИЕ:
-- Обернуть jsonb_agg() в COALESCE чтобы вернуть '[]'::jsonb вместо NULL

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
        '[]'::jsonb  -- ✅ FIX: Вернуть пустой массив вместо NULL
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

-- Откат к версии из миграции 039 (с багом)
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
        SELECT jsonb_agg(host_item)  -- ❌ Баг: может вернуть NULL
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
