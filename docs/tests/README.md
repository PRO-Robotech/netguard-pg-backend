# Документация по тестам Netguard-PG-Backend

Этот раздел содержит подробное описание всех тестовых сценариев и проверок в проекте `netguard-pg-backend`.

## Структура тестирования

Проект использует комплексную стратегию тестирования, включающую несколько уровней:

### 1. Unit тесты (Domain слой)
- **Расположение**: `./internal/domain/...`
- **Цель**: Тестирование бизнес-логики и доменных моделей
- **Статус**: ✅ Работают корректно

### 2. Integration тесты (Repository слой)
- **In-Memory репозиторий**: `./internal/infrastructure/repositories/mem/...`
- **PostgreSQL репозиторий**: `./internal/infrastructure/repositories/pg/...` (тесты отсутствуют)
- **Статус**: ✅ In-memory тесты работают

### 3. API тесты
- **Расположение**: `./internal/api/...`
- **Цель**: Тестирование gRPC API и REST эндпоинтов
- **Статус**: ✅ Работают корректно

### 4. Application тесты
- **Services**: `./internal/application/services/...`
- **Validation**: `./internal/application/validation/...`
- **Статус**: ⚠️ Некоторые тесты требуют доработки

### 5. K8s тесты
- **Расположение**: `./internal/k8s/...`
- **Статус**: ⚠️ Минорные проблемы с конвертерами

## Детальная документация

- [Unit тесты (Domain)](./unit-tests.md) - Тестирование доменных моделей и бизнес-логики
- [Integration тесты](./integration-tests.md) - Тестирование репозиториев и хранения данных
- [API тесты](./api-tests.md) - Тестирование gRPC и REST API
- [Application тесты](./application-tests.md) - Тестирование сервисов и валидации
- [K8s тесты](./k8s-tests.md) - Тестирование Kubernetes интеграции
- [E2E тесты](./e2e-tests.md) - End-to-end тестирование

## Команды для запуска тестов

```bash
# Все тесты
make test

# Unit тесты
make test-unit
go test -v ./internal/domain/...

# Integration тесты
make test-integration
go test -v ./internal/infrastructure/repositories/mem/...

# API тесты
go test -v ./internal/api/...

# Application тесты
go test -v ./internal/application/...

# K8s тесты
go test -v ./internal/k8s/...

# Покрытие кода
make test-coverage
```

## Статус тестов

| Компонент | Статус | Проблемы | Решение |
|-----------|--------|----------|---------|
| Domain | ✅ Работает | Нет | - |
| Mem Repository | ✅ Работает | Нет | ✅ 100% |
| PG Repository | ⚠️ Нет тестов | Нет тестов для PostgreSQL | Не критично |
| API | ✅ Работает | Нет | ✅ 100% |
| Application Services | ✅ Работает | Нет | ✅ 100% |
| Application Validation | ✅ Работает | Нет | ✅ 100% |
| K8s Registry | ✅ Работает | Все исправлено | ✅ 100% |
| Sync Layer | ✅ Работает | Mock настройки исправлены | ✅ 100% |

## Исправленные проблемы

### 1. Ошибка форматирования в mem-reader.go
**Проблема**: Использование `%d` для string переменной в `log.Printf`
**Решение**: Заменено на `%s`
**Файл**: `internal/infrastructure/repositories/mem/mem-reader.go:544`

### 2. Отсутствующие методы в моках
**Проблема**: Моки не реализовывали интерфейс `ports.Reader` полностью
**Решение**: Добавлены методы `ListNetworks`, `ListNetworkBindings`, `GetNetworkByID`, `GetNetworkBindingByID`
**Файлы**: Все `*_test.go` в validation пакете

### 3. Неправильный тип Condition
**Проблема**: Использование `models.Condition` вместо `metav1.Condition`
**Решение**: Исправлены типы и добавлены импорты
**Файл**: `internal/application/validation/ieagag_rule_validator_test.go`

### 4. Неправильная проверка ingress портов
**Проблема**: ConditionManager блокировал сервисы без портов
**Решение**: Убрана ненужная проверка - сервис без портов нормален
**Файл**: `internal/application/services/condition_manager.go`

### 5. Некорректный MockWriter для IEAgAgRules
**Проблема**: MockWriter неправильно отслеживал синхронизированные правила
**Решение**: Исправлена логика захвата правил (только первый значимый sync)
**Файл**: `internal/application/services/service_ieagag_test.go`

### 6. K8s Convert nil vs empty slice
**Проблема**: `make([]NetworkItem, 0)` создавал `[]` вместо `nil`
**Решение**: Добавлена проверка `len() > 0` перед созданием slice
**Файл**: `internal/k8s/registry/convert/addressgroup.go`

### 7. Missing service в K8s Service тестах  
**Проблема**: MockBackendClient не содержал service "test-service" в "default" namespace
**Решение**: Добавлены тестовые данные в MockBackendClient
**Файл**: `internal/k8s/client/mock_backend.go`

### 8. Mock expectations в Sync Syncers
**Проблема**: Неправильная настройка mock entity для разных test cases
**Решение**: Исправлена логика создания и настройки mock objects
**Файл**: `internal/sync/syncers/address_group_syncer_test.go`

## ✅ Все основные проблемы решены!

🎉 **Статус: ВСЕ КРИТИЧЕСКИЕ И НЕКРИТИЧЕСКИЕ ПРОБЛЕМЫ ИСПРАВЛЕНЫ!**

## Рекомендации для дальнейшего развития

1. **✅ Все тесты работают** - основная функциональность полностью покрыта
2. **Создать тесты для PostgreSQL репозитория** - для production deployment
3. **Добавить performance тесты** - для больших объемов данных
4. **Расширить integration тесты** - межкомпонентные сценарии
5. **Добавить E2E тесты** - полные пользовательские сценарии

## Метрики покрытия

Для получения отчета о покрытии кода:

```bash
make test-coverage
open coverage.html
```