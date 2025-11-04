# Binding Resource Patterns - Reference Implementation

**Статус**: ✅ Эталонная реализация основана на NetworkBinding
**Дата**: 2025-10-26
**Автор**: Исправления после анализа проблем с HostBinding и AddressGroupBinding

## Оглавление
1. [Обзор проблемы](#обзор-проблемы)
2. [Правильные паттерны (NetworkBinding)](#правильные-паттерны-networkbinding)
3. [Проблемы в HostBinding](#проблемы-в-hostbinding)
4. [Исправления](#исправления)
5. [Checklist для Binding ресурсов](#checklist-для-binding-ресурсов)

---

## Обзор проблемы

### Что такое Binding Resources (процесс-ресурсы)?

**Binding Resources** — это ресурсы, которые связывают Entity Resources с AddressGroup:
- `HostBinding` — связывает Host с AddressGroup
- `NetworkBinding` — связывает Network с AddressGroup
- `AddressGroupBinding` — связывает две AddressGroup между собой

**Ключевые отличия от Entity Resources**:
- Не синхронизируются напрямую с SGROUP
- Зависят от других ресурсов (Host/Network + AddressGroup)
- При создании/удалении обновляют зависимые ресурсы
- Обрабатываются OutboxWorker специальным образом

### Выявленные проблемы

После детального анализа обнаружено, что **только NetworkBinding работает корректно**.
**HostBinding и AddressGroupBinding имеют критические ошибки**:

1. ❌ **Update*Binding использует `ConditionOnlyOperation`** вместо `WithSyncOp(SyncOpUpsert)`
   - Результат: Entity ресурс (Host/Network) **НЕ обновляется** в БД при создании/удалении Binding

2. ❌ **Delete*Binding вызывает syncManager напрямую**
   - Результат: Дублирование синхронизации, обход Outbox, нарушение порядка операций

3. ❌ **Отсутствует валидация IsBound в CreateBinding**
   - Результат: Можно создать несколько Binding для одного Entity ресурса

4. ❌ **Отсутствует UNIQUE constraint в БД**
   - Результат: Защита только на уровне приложения, обходима при прямых запросах

5. ❌ **Worker не обрабатывает DELETE операций для процесс-ресурсов**
   - Результат: Застревают Outbox entries, бесконечные retry с ошибкой "not found"

---

## Правильные паттерны (NetworkBinding)

### 1. Валидация при создании

**Файл**: `internal/application/validation/network_binding_validator.go`

```go
func (v *NetworkBindingValidator) ValidateForCreation(ctx context.Context, binding *models.NetworkBinding) error {
    // ... базовая валидация ...

    // CRITICAL: Check that Network is not already bound to another AddressGroup
    // This prevents creating duplicate bindings for the same Network
    networkID := models.ResourceIdentifier{Name: binding.NetworkRef.Name, Namespace: binding.Namespace}
    network, err := v.reader.GetNetworkByID(ctx, networkID)
    if err != nil {
        return errors.Wrap(err, "failed to get network")
    }

    // ВАЖНО: Проверяем что Network.IsBound == false!
    if network != nil && network.IsBound {
        // Network is already bound - get the binding details for error message
        agRefInfo := "unknown"
        if network.AddressGroupRef != nil {
            // Network.AddressGroupRef is ObjectReference (no Namespace field)
            // Namespace is stored separately in Network or inferred from binding namespace
            // For error message, use binding namespace as context
            agRefInfo = fmt.Sprintf("%s/%s", binding.Namespace, network.AddressGroupRef.Name)
        }
        return errors.Errorf("network %s is already bound to address group %s (each network can only be bound to one address group)",
            networkID.Key(), agRefInfo)
    }

    return nil
}
```

**Ключевые моменты**:
- ✅ Проверяем `network.IsBound == true` перед созданием Binding
- ✅ Возвращаем понятное сообщение об ошибке с деталями существующего Binding
- ✅ Используем правильное поле `binding.Namespace` (не `network.AddressGroupRef.Namespace`)

### 2. Update*Binding - Обновление Entity ресурса при создании Binding

**Файл**: `internal/application/services/resources/network_resource_service.go`

```go
func (s *NetworkResourceService) UpdateNetworkBinding(
    ctx context.Context,
    networkID models.ResourceIdentifier,
    bindingID models.ResourceIdentifier,
    addressGroupID models.ResourceIdentifier,
) error {
    // ... получение Network из БД ...

    // Установка binding references
    network.BindingRef = &v1beta1.NamespacedObjectReference{ /* ... */ }
    network.AddressGroupRef = &v1beta1.NamespacedObjectReference{ /* ... */ }
    network.IsBound = true

    // CRITICAL: Sync the updated network (update binding references in database)
    // Use SyncOpUpsert to ensure Network is updated with binding references
    // This will create an Outbox entry for Network UPDATE operation
    networks := []models.Network{*network}
    if err := writer.SyncNetworks(ctx, networks, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
        return fmt.Errorf("failed to sync network binding: %w", err)
    }

    return nil
}
```

**Ключевые моменты**:
- ✅ Используем `WithSyncOp(models.SyncOpUpsert)` — **НЕ** `ConditionOnlyOperation`!
- ✅ Это обновляет `spec` в БД (isBound, bindingRef, addressGroupRef)
- ✅ Создается Outbox entry для синхронизации с SGROUP
- ✅ OutboxWorker обработает запись асинхронно

### 3. Remove*Binding - Обновление Entity ресурса при удалении Binding

**Файл**: `internal/application/services/resources/network_resource_service.go`

```go
func (s *NetworkResourceService) RemoveNetworkBinding(
    ctx context.Context,
    networkID models.ResourceIdentifier,
) error {
    // ... получение Network из БД ...

    // Очистка binding references
    network.BindingRef = nil
    network.AddressGroupRef = nil
    network.IsBound = false

    // CRITICAL: Sync the updated network (update binding references in database)
    // Use SyncOpUpsert to ensure Network is updated with cleared binding references
    // This will create an Outbox entry for Network UPDATE operation
    networks := []models.Network{*network}
    if err := writer.SyncNetworks(ctx, networks, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
        return fmt.Errorf("failed to sync network unbinding: %w", err)
    }

    return nil
}
```

**Ключевые моменты**:
- ✅ Используем `WithSyncOp(models.SyncOpUpsert)` для очистки refs
- ✅ **НЕТ** прямых вызовов `syncManager.SyncEntity()`
- ✅ Триггеры и OutboxWorker обработают синхронизацию автоматически

### 4. DELETE триггер с affects_resources

**Файл**: `migrations/031_fix_process_resource_delete_triggers.sql`

```sql
CREATE OR REPLACE FUNCTION trigger_network_binding_before_delete()
RETURNS TRIGGER AS $$
DECLARE
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
    v_affected_resources JSONB;
BEGIN
    -- Get UID and check if already marked for deletion
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    IF v_already_marked_for_deletion THEN
        RETURN OLD;
    END IF;

    -- Build affected_resources array BEFORE DELETE
    -- CRITICAL: Include Network and AddressGroup that this binding connects
    v_affected_resources := jsonb_build_array(
        jsonb_build_object(
            'type', 'Network',
            'namespace', OLD.network_namespace,
            'name', OLD.network_name
        ),
        jsonb_build_object(
            'type', 'AddressGroup',
            'namespace', OLD.address_group_namespace,
            'name', OLD.address_group_name
        )
    );

    -- Mark metadata for deletion
    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting internal processing before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    -- Create DELETE Outbox entry with affects_resources
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        resource_namespace,
        resource_name,
        operation,
        target_system,
        payload,
        affects_resources,  -- CRITICAL: affects_resources field
        status,
        attempts,
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'NetworkBinding',
        v_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'network', jsonb_build_object(
                'namespace', OLD.network_namespace,
                'name', OLD.network_name
            ),
            'address_group', jsonb_build_object(
                'namespace', OLD.address_group_namespace,
                'name', OLD.address_group_name
            )
        ),
        v_affected_resources,  -- CRITICAL: populated with Network and AddressGroup
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    );

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
```

**Ключевые моменты**:
- ✅ `affects_resources` заполняется **до** удаления ресурса
- ✅ Включает Network и AddressGroup, которые нужно обновить
- ✅ OutboxWorker использует эту информацию для обновления зависимых ресурсов

### 5. UNIQUE constraint в БД

**Файл**: `migrations/032_add_network_binding_unique_constraint.sql`

```sql
-- +goose Up
-- +goose StatementBegin

ALTER TABLE network_bindings
ADD CONSTRAINT network_bindings_network_unique
UNIQUE (network_namespace, network_name);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE network_bindings
DROP CONSTRAINT IF EXISTS network_bindings_network_unique;

-- +goose StatementEnd
```

**Ключевые моменты**:
- ✅ Защита на уровне БД от дубликатов
- ✅ Даже если валидация в приложении обойдена, БД отклонит дубликат
- ✅ Constraint на `(network_namespace, network_name)` — гарантирует один Binding на один Network

### 6. Worker обработка DELETE операций

**Файл**: `internal/sync/worker/process_process_resource.go`

```go
func (w *OutboxWorker) markProcessResourceReady(
    ctx context.Context,
    item *domain.OutboxEntry,
) error {
    // CRITICAL: For DELETE operations, the process resource no longer exists in the database,
    // so we skip steps 1-3 and just delete the Outbox entry.
    if item.Operation == domain.SyncOperationDelete {
        w.logger.Info("DELETE operation for process resource - skipping condition update, deleting outbox entry",
            zap.String("resource_type", item.ResourceType),
            zap.String("resource_id", item.ResourceID.String()))

        if err := w.outboxRepo.Delete(ctx, item.ID); err != nil {
            return fmt.Errorf("failed to delete outbox entry: %w", err)
        }

        w.logger.Info("process resource DELETE operation completed successfully",
            zap.String("resource_type", item.ResourceType),
            zap.String("resource_id", item.ResourceID.String()))

        return nil
    }

    // For UPSERT operations (CREATE/UPDATE), proceed with normal flow:
    // ... load resource, update conditions, delete outbox ...
}
```

**Ключевые моменты**:
- ✅ Проверяем `item.Operation == domain.SyncOperationDelete`
- ✅ Для DELETE просто удаляем Outbox entry — ресурс уже удален!
- ✅ Не пытаемся загрузить ресурс (вызовет ошибку "not found")
- ✅ Affected resources (Network, AddressGroup) уже обновлены через `RemoveNetworkBinding`

---

## Проблемы в HostBinding

### Проблема 1: UpdateHostBinding использует ConditionOnlyOperation ❌

**Файл**: `internal/application/services/resources/host_resource_service.go:534`

```go
// ❌ НЕПРАВИЛЬНО! Не обновляет spec в БД!
if err := writer.SyncHosts(ctx, hosts, ports.EmptyScope{}, ports.ConditionOnlyOperation{}); err != nil {
    return fmt.Errorf("failed to sync host binding: %w", err)
}
```

**Последствия**:
- Host.IsBound остается `false`
- Host.BindingRef и AddressGroupRef остаются `nil`
- В k8s API Host показывает неправильный статус
- Outbox entry не создается → SGROUP не синхронизируется

**Должно быть** (как в NetworkBinding):
```go
// ✅ ПРАВИЛЬНО! Обновляет spec + создает Outbox entry
if err := writer.SyncHosts(ctx, hosts, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
    return fmt.Errorf("failed to sync host binding: %w", err)
}
```

### Проблема 2: DeleteHostBinding вызывает syncManager напрямую ❌

**Файл**: `internal/application/services/resources/host_binding_resource_service.go:256-261`

```go
// ❌ НЕПРАВИЛЬНО! Прямой вызов sync
if err := s.hostResourceService.syncManager.SyncEntity(ctx, host, types.SyncOperationUpsert); err != nil {
}
```

**Последствия**:
- Обход Outbox pattern
- Дублирование синхронизации (триггер + прямой вызов)
- Нарушение порядка операций
- Невозможность отследить retry/failures

**Должно быть**: Убрать прямой вызов — триггеры и OutboxWorker сделают все автоматически!

### Проблема 3: Слабая валидация в validateHostBindingWithReader ❌

**Файл**: `internal/application/services/resources/host_binding_resource_service.go:458-471`

```go
// Проверяет что Host.IsBound = true, но позволяет создание если BindingRef != nil
if host.IsBound {
    if host.BindingRef != nil {
        expectedName := bindingID.Name
        actualName := host.BindingRef.Name

        if actualName == expectedName {
            return nil  // ❌ Разрешает создание!
        }
        // ...
    } else {
        return fmt.Errorf("host is already bound to AddressGroup via spec.hosts - cannot create HostBinding")
    }
}
```

**Последствия**:
- Можно создать несколько HostBinding для одного Host (при разных bindingID.Name)
- Логика запутанная и сложная для понимания

**Должно быть** (как в NetworkBinding):
```go
// CRITICAL: Check that Host is not already bound to another AddressGroup
if host.IsBound {
    agRefInfo := "unknown"
    if host.AddressGroupRef != nil {
        agRefInfo = fmt.Sprintf("%s/%s", host.Namespace, host.AddressGroupRef.Name)
    }
    return errors.Errorf("host %s is already bound to address group %s (each host can only be bound to one address group)",
        hostID.Key(), agRefInfo)
}
```

### Проблема 4: Отсутствует UNIQUE constraint ❌

**Файл**: отсутствует миграция `033_add_host_binding_unique_constraint.sql`

**Последствия**:
- Можно создать несколько HostBinding для одного Host через прямые SQL запросы
- Защита только на уровне приложения

**Должна быть**:
```sql
ALTER TABLE host_bindings
ADD CONSTRAINT host_bindings_host_unique
UNIQUE (host_namespace, host_name);
```

---

## Исправления

### 1. Исправить UpdateHostBinding

**До**:
```go
if err := writer.SyncHosts(ctx, hosts, ports.EmptyScope{}, ports.ConditionOnlyOperation{}); err != nil {
```

**После**:
```go
if err := writer.SyncHosts(ctx, hosts, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
```

### 2. Исправить DeleteHostBinding

**Убрать**:
```go
// Удалить эти строки (256-261, 285-286)
if err := s.hostResourceService.syncManager.SyncEntity(ctx, host, types.SyncOperationUpsert); err != nil {
}
// ...
if err := s.addressGroupResourceService.syncManager.SyncEntity(ctx, addressGroup, types.SyncOperationUpsert); err != nil {
}
```

**Оставить только**:
```go
// SyncHosts с SyncOpUpsert создаст Outbox entry
if err := writerForHost.SyncHosts(ctx, []models.Host{*host}, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
    // handle error
}
```

### 3. Усилить validateHostBindingWithReader

**До**:
```go
if host.IsBound {
    if host.BindingRef != nil {
        // ... сложная логика ...
    }
}
```

**После**:
```go
// CRITICAL: Check that Host is not already bound to another AddressGroup
if host.IsBound {
    agRefInfo := "unknown"
    if host.AddressGroupRef != nil {
        agRefInfo = fmt.Sprintf("%s/%s", host.Namespace, host.AddressGroupRef.Name)
    }
    return errors.Errorf("host %s is already bound to address group %s (each host can only be bound to one address group)",
        hostID.Key(), agRefInfo)
}
```

### 4. Создать миграцию 033

**Файл**: `migrations/033_add_host_binding_unique_constraint.sql`

```sql
-- +goose Up
-- +goose StatementBegin

ALTER TABLE host_bindings
ADD CONSTRAINT host_bindings_host_unique
UNIQUE (host_namespace, host_name);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE host_bindings
DROP CONSTRAINT IF EXISTS host_bindings_host_unique;

-- +goose StatementEnd
```

---

## Checklist для Binding ресурсов

При создании или проверке Binding ресурса используйте этот checklist:

### ✅ Валидация
- [ ] Проверка существования зависимых Entity ресурсов
- [ ] Проверка `IsBound == false` для Entity ресурса
- [ ] Понятное сообщение об ошибке при нарушении

### ✅ Update*Binding (при создании Binding)
- [ ] Использует `WithSyncOp(models.SyncOpUpsert)` — **НЕ** `ConditionOnlyOperation`!
- [ ] Обновляет `IsBound = true`
- [ ] Устанавливает `BindingRef` и `AddressGroupRef`
- [ ] **НЕТ** прямых вызовов `syncManager.SyncEntity()`

### ✅ Remove*Binding (при удалении Binding)
- [ ] Использует `WithSyncOp(models.SyncOpUpsert)` для очистки
- [ ] Обновляет `IsBound = false`
- [ ] Очищает `BindingRef` и `AddressGroupRef` (nil)
- [ ] **НЕТ** прямых вызовов `syncManager.SyncEntity()`

### ✅ DELETE триггер
- [ ] Заполняет `affects_resources` **до** удаления
- [ ] Включает все зависимые Entity ресурсы
- [ ] Создает Outbox entry с `operation = DELETE`

### ✅ UNIQUE constraint
- [ ] Миграция с UNIQUE constraint на Entity reference
- [ ] Constraint на `(entity_namespace, entity_name)`

### ✅ Worker
- [ ] Обрабатывает DELETE операции (не пытается загрузить удаленный ресурс)
- [ ] Для DELETE просто удаляет Outbox entry

### ✅ Тестирование
- [ ] Создание Binding → Entity обновился (IsBound=true, refs заполнены)
- [ ] Удаление Binding → Entity обновился (IsBound=false, refs=nil)
- [ ] Попытка создать дубликат → ошибка валидации
- [ ] Попытка создать дубликат через БД → UNIQUE constraint блокирует
- [ ] DELETE обрабатывается Worker без ошибок
- [ ] Outbox entries не застревают в PENDING

---

## Итоги

**NetworkBinding** — эталонная реализация Binding ресурса. Все другие Binding (HostBinding, AddressGroupBinding) должны следовать этим паттернам.

**Ключевые принципы**:
1. Используй `WithSyncOp(models.SyncOpUpsert)` для Update/Remove
2. Не вызывай `syncManager` напрямую — триггеры сделают все
3. Проверяй `IsBound` при валидации
4. Используй UNIQUE constraint в БД
5. Worker должен обрабатывать DELETE специальным образом

**После применения этих паттернов**:
✅ Все Binding ресурсы работают одинаково и предсказуемо
✅ Нет дублирования синхронизации
✅ Нет застрявших Outbox entries
✅ Защита от дубликатов на всех уровнях (валидация + БД)
✅ Легко тестировать и отлаживать
