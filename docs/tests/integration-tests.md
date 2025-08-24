# Integration тесты

Integration тесты проверяют взаимодействие между компонентами системы, в основном тестируя репозитории и хранение данных.

## Расположение тестов

- `internal/infrastructure/repositories/mem/` - In-memory репозиторий
- `internal/infrastructure/repositories/pg/` - PostgreSQL репозиторий (тесты отсутствуют)

## In-Memory репозиторий

**Файл**: `internal/infrastructure/repositories/mem/*_test.go`

### Статус: ✅ Все тесты работают

После исправления ошибки форматирования в `mem-reader.go`, все тесты проходят успешно.

### Тестируемые компоненты:

#### 1. Базовая функциональность репозитория
- `TestMemRegistry` - Основные операции с реестром
- `TestMemRegistryAbort` - Откат транзакций

#### 2. Управление ресурсами
- `TestMemRegistryAddressGroups` - Работа с группами адресов
- `TestMemRegistryAddressGroupBindings` - Привязки групп адресов
- `TestMemRegistryAddressGroupPortMappings` - Портовые маппинги
- `TestMemRegistryRuleS2S` - Service-to-Service правила
- `TestMemRegistryServiceAliases` - Алиасы сервисов
- `TestMemRegistrySyncStatus` - Статус синхронизации

#### 3. Операции синхронизации
- `TestSyncServicesWithDifferentOperations` - Тестирование различных операций синхронизации:
  - **FullSync** - Полная синхронизация
  - **Upsert** - Создание/обновление
  - **Delete** - Удаление  
  - **FullSyncWithScope** - Синхронизация с областью видимости

#### 4. CRUD операции
- `TestListServices` - Получение списков сервисов
- `TestListAddressGroups` - Получение групп адресов
- `TestListAddressGroupBindings` - Получение привязок
- `TestListAddressGroupPortMappings` - Получение портовых маппингов
- `TestListRuleS2S` - Получение правил S2S

### Исправленная проблема

**Ошибка форматирования в `mem-reader.go`:**
```go
// Было (ошибка):
log.Printf("🔍 LISTING IEAgAgRule[%d] %s: ...", i, rule.Key(), ...)

// Стало (исправлено):
log.Printf("🔍 LISTING IEAgAgRule[%s] %s: ...", i, rule.Key(), ...)
```

**Причина**: В цикле `for i, rule := range rules` переменная `i` является ключом map (string), а не индексом (int).

### Пример успешного прогона:

```
=== RUN   TestMemRegistry
--- PASS: TestMemRegistry (0.00s)
=== RUN   TestSyncServicesWithDifferentOperations
=== RUN   TestSyncServicesWithDifferentOperations/FullSync
=== RUN   TestSyncServicesWithDifferentOperations/Upsert
=== RUN   TestSyncServicesWithDifferentOperations/Delete
--- PASS: TestSyncServicesWithDifferentOperations (0.00s)
PASS
ok      netguard-pg-backend/internal/infrastructure/repositories/mem    0.663s
```

## PostgreSQL репозиторий

**Расположение**: `internal/infrastructure/repositories/pg/`

### Статус: ❌ Тесты отсутствуют

При запуске `go test -v ./internal/infrastructure/repositories/pg/...` получаем:
```
testing: warning: no tests to run
PASS
ok      netguard-pg-backend/internal/infrastructure/repositories/pg     0.201s [no tests to run]
```

### Необходимые тесты для PostgreSQL:

#### 1. Базовые операции
- Подключение к базе данных
- Создание/удаление таблиц
- Транзакции

#### 2. CRUD операции
- Создание записей
- Чтение записей
- Обновление записей  
- Удаление записей

#### 3. Сложные сценарии
- Каскадные удаления
- Проверка ограничений (constraints)
- Индексы и производительность
- Concurrent access

#### 4. Миграции
- Применение миграций
- Откат миграций
- Версионирование схемы

## Команды запуска

```bash
# In-memory тесты
go test -v ./internal/infrastructure/repositories/mem/...

# PostgreSQL тесты (пока отсутствуют)
go test -v ./internal/infrastructure/repositories/pg/...

# Все integration тесты
make test-integration
```

## Конфигурация для PostgreSQL тестов

Для создания PostgreSQL тестов понадобится:

### Docker Compose для тестовой БД:
```yaml
services:
  test-postgres:
    image: postgres:14
    environment:
      POSTGRES_PASSWORD: postgres
      POSTGRES_USER: postgres
      POSTGRES_DB: netguard_test
    ports:
      - "5432:5432"
    command: >
      postgres
      -c log_statement=all
      -c log_duration=on
```

### Переменные окружения:
```bash
export PG_URI="postgres://postgres:postgres@localhost:5432/netguard_test?sslmode=disable"
```

## Рекомендации

### 1. Создать тесты для PostgreSQL репозитория
**Приоритет**: Высокий
**Причина**: Критично для production deployment

### 2. Добавить benchmark тесты
**Приоритет**: Средний  
**Цель**: Сравнение производительности in-memory vs PostgreSQL

### 3. Интеграция с CI/CD
**Приоритет**: Высокий
**Цель**: Автоматический запуск тестов с реальной БД

### 4. Тесты миграций
**Приоритет**: Высокий
**Цель**: Обеспечение безопасности обновлений схемы

## Заметки по архитектуре

In-memory репозиторий используется для:
- Быстрых unit тестов
- Локальной разработки
- CI/CD pipeline

PostgreSQL репозиторий используется для:
- Production окружения
- Staging тестирования
- Performance тестирования

Оба репозитория реализуют одни и те же интерфейсы из `internal/domain/ports/`, что обеспечивает взаимозаменяемость.