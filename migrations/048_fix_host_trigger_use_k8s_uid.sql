-- +goose Up
-- +goose StatementBegin

-- КРИТИЧЕСКИЙ БАГ-ФИКС: Host UPSERT trigger использует hosts.uuid вместо k8s_metadata.uid
--
-- ПРОБЛЕМА:
-- - trigger_host_upsert_outbox() использует NEW.uuid (Host spec UUID)
-- - trigger_host_before_delete() использует k8s_metadata.uid (Kubernetes UID)
-- - Это РАЗНЫЕ UUID! → CREATE и DELETE записи используют разные resource_id
-- - ON CONFLICT не срабатывает, потому что resource_id разные
--
-- СЛЕДСТВИЕ:
-- - CREATE entry: resource_id = Host.uuid (38fcf701-...)
-- - DELETE entry: resource_id = k8s_metadata.uid (0a2806d7-...)
-- - Пользователь видит только DELETE entry с "неправильным" UUID
-- - CREATE entry был обработан успешно и удалён retention policy
--
-- ДОКАЗАТЕЛЬСТВО:
-- - Migration 026: Host UPSERT использует NEW.uuid (НЕПРАВИЛЬНО)
-- - Migration 026: Host DELETE использует k8s_metadata.uid (ПРАВИЛЬНО)
-- - Migration 041: AddressGroup ИСПРАВЛЕН на k8s_metadata.uid
-- - Migration 042: Network/Service ИСПРАВЛЕНЫ на k8s_metadata.uid
-- - Host trigger НЕ БЫЛ ИСПРАВЛЕН!
--
-- РЕШЕНИЕ:
-- Обновить trigger_host_upsert_outbox() чтобы использовать k8s_metadata.uid
-- как все остальные ресурсы (AddressGroup, Network, Service).

CREATE OR REPLACE FUNCTION trigger_host_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_has_spec_changes BOOLEAN := FALSE;
BEGIN
    -- Определение типа операции
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
        v_has_spec_changes := TRUE;
    ELSIF TG_OP = 'UPDATE' THEN
        -- Проверка изменения spec полей
        IF OLD.uuid IS DISTINCT FROM NEW.uuid OR
           OLD.is_bound IS DISTINCT FROM NEW.is_bound OR
           OLD.host_name_sync IS DISTINCT FROM NEW.host_name_sync OR
           OLD.address_group_name IS DISTINCT FROM NEW.address_group_name OR
           OLD.ip_list IS DISTINCT FROM NEW.ip_list THEN
            v_has_spec_changes := TRUE;
            v_operation_type := 'UPDATE'::sync_operation;
        ELSE
            -- Нет изменений spec, пропускаем sync
            RETURN NEW;
        END IF;
    ELSE
        RETURN NEW;
    END IF;

    -- ✅ КРИТИЧЕСКИЙ ФИКС: Используем k8s_metadata.uid вместо hosts.uuid
    -- Теперь CREATE и DELETE будут использовать ОДИН И ТОТ ЖЕ resource_id!
    SELECT km.uid INTO v_resource_id
    FROM k8s_metadata km
    WHERE km.resource_version = NEW.resource_version;

    IF v_resource_id IS NULL THEN
        -- Fallback (не должно происходить в нормальной работе)
        -- Генерируем UUID как uuid_generate_v5 на основе namespace/name
        v_resource_id := uuid_generate_v5(
            uuid_ns_dns(),
            'Host:' || NEW.namespace || '/' || NEW.name
        );
    END IF;

    -- Вставка или обновление outbox entry
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        resource_namespace,
        resource_name,
        operation,
        target_system,
        payload,
        status,
        attempts,
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'Host',
        v_resource_id,  -- Теперь k8s_metadata.uid!
        NEW.namespace,
        NEW.name,
        v_operation_type,
        'SGROUP',
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'uuid', NEW.uuid::text,
            'is_bound', NEW.is_bound,
            'host_name_sync', NEW.host_name_sync,
            'ip_list', NEW.ip_list
        ),
        'PENDING',
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING',
        attempts = 0,
        next_retry_at = NOW(),
        updated_at = NOW(),
        payload = EXCLUDED.payload;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Откатываем к старой (НЕПРАВИЛЬНОЙ) версии trigger
CREATE OR REPLACE FUNCTION trigger_host_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
    END IF;

    v_resource_id := NEW.uuid;  -- ❌ Старая (неправильная) логика

    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        operation,
        target_system,
        payload,
        resource_namespace,
        resource_name,
        status,
        attempts,
        max_retries,
        next_retry_at,
        created_at,
        updated_at
    )
    VALUES (
        'Host',
        v_resource_id,
        v_operation_type,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'uuid', NEW.uuid::text
        ),
        NEW.namespace,
        NEW.name,
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = NOW(),
        updated_at = NOW();

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd
