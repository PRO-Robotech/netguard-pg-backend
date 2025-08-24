# K8s тесты

K8s тесты проверяют интеграцию с Kubernetes API Server, включая конвертеры, валидаторы и клиенты.

## Расположение тестов

- `internal/k8s/client/` - Клиент для взаимодействия с backend
- `internal/k8s/registry/base/` - Базовая функциональность хранилища
- `internal/k8s/registry/convert/` - Конвертеры между K8s и domain моделями
- `internal/k8s/registry/service/` - Сервисные операции
- `internal/k8s/registry/validation/` - Валидация K8s объектов

## Статус тестов

| Компонент | Статус | Проблемы |
|-----------|--------|----------|
| Client | ✅ Работает | Нет |
| Base Storage | ✅ Работает | Нет |
| Convert | ⚠️ Частично | nil vs empty slice |
| Service | ❌ Провал | Service not found |
| Validation | ✅ Работает | Нет |

## Успешные компоненты

### 1. Backend Client Extensions
**Файл**: `internal/k8s/client/backend_extensions_test.go`

#### Тестируемая функциональность:
- `TestPhase4_1_BackendClientExtensions` ✅
  - **Ping method**: Проверка соединения с backend
  - **UpdateMeta methods**: Обновление метаданных
  - **Helper methods for subresources**: Вспомогательные методы
  - **All BackendClient methods compile**: Компиляция всех методов интерфейса

- `TestPhase4_1_PerformanceComparison` ✅
  - Сравнение производительности Ping vs HealthCheck

### 2. Base Storage
**Файл**: `internal/k8s/registry/base/storage_test.go`

#### Все тесты проходят успешно:
- `TestBaseStorage_New` - Создание объектов
- `TestBaseStorage_NewList` - Создание списков
- `TestBaseStorage_ConvertToTable_SingleObject` - Конвертация в таблицы
- `TestBaseStorage_ConvertToTable_List` - Конвертация списков
- `TestBaseStorage_GetConverter` - Получение конвертеров
- `TestBaseStorage_GetBackendOps` - Backend операции
- `TestBaseStorage_BroadcastWatchEvent` - Watch события
- `TestBaseStorage_ApplyPatch_*` - Применение патчей
- `TestBaseStorage_GetObjectName` - Получение имен объектов
- `TestMockConverter_*` - Тесты мок конвертеров
- `TestMockValidator_*` - Тесты мок валидаторов

### 3. Validation
**Файл**: `internal/k8s/registry/validation/addressgroup_test.go`

#### AddressGroup валидация:
- `TestAddressGroupValidator_ValidateCreate` ✅
- `TestAddressGroupValidator_ValidateUpdate` ✅  
- `TestAddressGroupValidator_ValidateDelete` ✅
- `TestAddressGroupValidator_validateMetadata` ✅
- `TestAddressGroupValidator_validateSpec` ✅
- `TestAddressGroupValidator_validateDefaultAction` ✅

## Проблемные компоненты

### 1. Convert тесты
**Файл**: `internal/k8s/registry/convert/addressgroup_test.go`

#### Проблемы с AddressGroup конвертером:
- `TestAddressGroupConverter_ToDomain` ❌
- `TestAddressGroupConverter_FromDomain` ❌

**Проблема**: Несоответствие в инициализации Networks field
```diff
- Networks: ([]models.NetworkItem) <nil>
+ Networks: ([]models.NetworkItem) {}
```

**Причина**: Конвертеры создают пустой slice вместо nil
**Решение**: Привести к консистентности - везде использовать либо nil, либо empty slice

#### Успешные convert тесты:
- `TestAddressGroupConverter_RoundTrip` ✅
- `TestAddressGroupConverter_ToList` ✅
- `TestAddressGroupConverter_EnumConversion` ✅
- `TestIEAgAgRuleConverter_*` ✅ (все тесты)
- `TestServiceConverter_*` ✅ (все тесты)

### 2. Service Registry
**Файл**: `internal/k8s/registry/service/addressgroups_test.go`

#### Проблемный тест:
- `TestAddressGroupsREST_Get` ❌
  - **Ошибка**: "failed to get service: service not found: default/test-service"
  - **Причина**: Тест пытается получить несуществующий сервис
  - **Решение**: Создать сервис перед попыткой получения

#### Успешные тесты:
- `TestAddressGroupsREST_New` ✅
- `TestAddressGroupsREST_ConvertToTable` ✅

## Архитектурные особенности

### K8s API Server интеграция
Тесты проверяют корректность интеграции с Kubernetes:
- Конвертация между K8s CRD и domain моделями
- Валидация K8s объектов
- REST API операции
- Watch механизм

### Типы данных
K8s тесты работают с:
- `netguardv1beta1.AddressGroup`
- `netguardv1beta1.Service`  
- `netguardv1beta1.IEAgAgRule`
- `metav1.Condition`
- `metav1.ObjectMeta`

## Команды запуска

```bash
# Все K8s тесты
go test -v ./internal/k8s/...

# Конкретные компоненты
go test -v ./internal/k8s/client/...
go test -v ./internal/k8s/registry/base/...
go test -v ./internal/k8s/registry/convert/...
go test -v ./internal/k8s/registry/validation/...

# Проблемные тесты
go test -v ./internal/k8s/registry/convert/ -run TestAddressGroupConverter_ToDomain
go test -v ./internal/k8s/registry/service/ -run TestAddressGroupsREST_Get
```

## Исправления для проблемных тестов

### 1. Конвертеры (convert)
**Необходимо привести к консистентности**:
```go
// Вариант 1: Везде nil
if len(slice) == 0 {
    slice = nil
}

// Вариант 2: Везде empty slice
if slice == nil {
    slice = make([]Type, 0)
}
```

### 2. Service Registry
**Создать тестовые данные**:
```go
// Перед TestAddressGroupsREST_Get
func setupTestService(t *testing.T) {
    // Создать тестовый сервис default/test-service
}
```

## Влияние на систему

### Критичность проблем:
- **Convert тесты**: Низкая - не влияет на функциональность
- **Service Registry**: Средняя - может указывать на проблемы в REST API

### Рекомендации:
1. **Исправить nil vs empty slice** - для консистентности
2. **Добавить setup методы** в service тесты
3. **Расширить покрытие** convert тестов
4. **Добавить integration тесты** с реальным K8s API

## Связь с основной системой

K8s тесты важны для:
- Проверки совместимости с Kubernetes
- Валидации CRD объектов
- Корректности API Server интеграции
- Работы с etcd через K8s API

Эти тесты менее критичны для core функциональности, но важны для K8s deployment.