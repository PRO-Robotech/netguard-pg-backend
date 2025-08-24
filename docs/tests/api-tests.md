# API тесты

API тесты проверяют работу gRPC API и REST эндпоинтов системы. Все API тесты работают корректно.

## Расположение тестов

- `internal/api/netguard/` - gRPC API тесты

## Статус: ✅ Все тесты работают

```
PASS
ok      netguard-pg-backend/internal/api/netguard       0.251s
```

## Тестируемые компоненты

### 1. Основная функциональность API

#### Базовые операции синхронизации
- `TestSync` - Основной тест синхронизации
  - **Что проверяет**: Базовая синхронизация сервисов
  - **Логика**: Создание сервиса, парсинг портов, установка условий
  - **Результат**: Сервис успешно синхронизирован с 3 условиями

#### Различные операции синхронизации  
- `TestSyncWithDifferentOperations` - Комплексный тест различных операций
  - **Services**: Тестирование FullSync, Upsert, Delete для сервисов
  - **AddressGroups**: Тестирование операций для групп адресов
  - **AddressGroupBindings**: Тестирование привязок групп адресов

### 2. Детальные тесты операций

#### Services (Сервисы)
- **FullSync**: Полная синхронизация нескольких сервисов
  - Создание web сервиса (порт 80)
  - Создание db сервиса (порт 5432)  
  - Добавление api сервиса (порт 8080)
  
- **Upsert**: Создание/обновление сервисов
  - Создание базовых сервисов
  - Обновление портов существующих сервисов
  - Добавление новых сервисов

- **Delete**: Удаление сервисов
  - Создание сервисов
  - Удаление одного сервиса
  - Проверка оставшихся сервисов

#### AddressGroups (Группы адресов)
- **FullSync**: Синхронизация групп адресов
  - internal группа
  - external группа
  - dmz группа

- **Upsert**: Обновление групп
- **Delete**: Удаление с проверкой зависимостей

#### AddressGroupBindings (Привязки)
- **FullSync**: Создание привязок между группами и сервисами
  - Проверка портовых маппингов
  - Валидация связей

### 3. Конвертация и утилиты

#### SyncOp конвертация
- `TestConvertSyncOp` - Конвертация операций синхронизации:
  - **NoOp**: Нет операции
  - **FullSync**: Полная синхронизация
  - **Upsert**: Создание/обновление
  - **Delete**: Удаление
  - **Invalid**: Обработка невалидных значений

#### Статус синхронизации
- `TestSyncStatus` - Проверка статуса синхронизации
  - Создание сервиса
  - Получение статуса
  - Проверка метаданных

### 4. Операции чтения (List)

#### Listing операции
- `TestListServices` - Получение списка сервисов
  - Создание сервисов в разных namespace
  - Фильтрация по namespace
  - Проверка результатов

- `TestListAddressGroups` - Список групп адресов
- `TestListAddressGroupBindings` - Список привязок
- `TestListAddressGroupPortMappings` - Список портовых маппингов  
- `TestListRuleS2S` - Список Service-to-Service правил

## Проверяемая функциональность

### 1. Парсинг портов
Во всех тестах проверяется правильность парсинга портов:
```
🔧 ParsePortRanges: parsing port string '80'
🔧 Split port string into 1 items
🔧 Processing item 0: '80'
🔧 Item 0 is a single port
🔧 Added single port 80
✅ ParsePortRanges: successfully parsed 1 port ranges from '80'
```

### 2. Управление условиями (Conditions)
Каждый ресурс получает соответствующие условия:
- **Synced=True**: Ресурс синхронизирован
- **Validated=True**: Ресурс прошел валидацию
- **Ready=True**: Ресурс готов к использованию

### 3. Портовые маппинги
Для AddressGroupBindings проверяется:
- Создание портовых маппингов
- Обновление существующих маппингов
- Валидация портов сервисов

### 4. Транзакционность
Все операции выполняются в рамках транзакций:
```
💾 COMMIT: Starting database commit operation
💾 COMMIT: Committing N resources to database
✅ COMMIT: Database commit operation completed successfully
```

## Команды запуска

```bash
# Все API тесты
go test -v ./internal/api/...

# С детальным выводом
go test -v ./internal/api/netguard/...

# Конкретный тест
go test -v ./internal/api/netguard/ -run TestSync

# С покрытием
go test -cover ./internal/api/...
```

## Логирование в тестах

API тесты демонстрируют детальное логирование:

### Парсинг портов:
```
🔧 ParsePortRanges: parsing port string '8080'
🔧 Split port string into 1 items  
🔧 Processing item 0: '8080'
🔧 Item 0 is a single port
🔧 Added single port 8080
✅ ParsePortRanges: successfully parsed 1 port ranges from '8080'
```

### Работа с условиями:
```
🔄 ConditionManager.ProcessServiceConditions: processing service default/web after commit
✅ ConditionManager: Setting Synced=true for default/web
🔄 ConditionManager: Validating committed service default/web
✅ ConditionManager: Setting Validated=true for default/web
✅ ConditionManager: Setting Ready=true for default/web
```

### Операции с базой данных:
```
💾 COMMIT: Starting database commit operation
💾 COMMIT: Committing 1 services to database
💾 COMMIT: Service[default/web] has 3 conditions
✅ COMMIT: Services committed to database
```

### Портовые маппинги:
```
🔧 UpdatePortMapping: updating port mapping for address group default/internal and service default/web
🔧 Service has 1 ingress ports
🔧 Processing port 0: Protocol=TCP, Port=80
🔧 Updated port mapping has 1 service entries
```

## Покрываемые сценарии

### 1. Базовые CRUD операции
- Создание ресурсов
- Чтение ресурсов
- Обновление ресурсов
- Удаление ресурсов

### 2. Сложные операции
- Каскадные обновления
- Проверка зависимостей
- Валидация ссылок

### 3. Граничные случаи
- Работа с пустыми данными
- Обработка ошибок
- Конвертация типов

## Архитектурные проверки

API тесты подтверждают правильность:
- Разделения на слои (API → Service → Repository)
- Обработки ошибок
- Транзакционности операций
- Логирования и трассировки

## Рекомендации

1. **Добавить тесты производительности** для больших объемов данных
2. **Расширить тесты ошибок** для различных error cases
3. **Добавить тесты concurrent access** для многопользовательских сценариев
4. **Создать integration тесты** с реальным gRPC клиентом