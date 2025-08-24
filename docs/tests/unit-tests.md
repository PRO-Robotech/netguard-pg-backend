# Unit тесты (Domain слой)

Unit тесты в проекте netguard-pg-backend покрывают доменные модели и бизнес-логику. Все тесты в domain слое проходят успешно.

## Расположение тестов

- `internal/domain/models/` - Тесты доменных моделей
- `internal/domain/ports/` - Тесты портов и интерфейсов

## Тестируемые компоненты

### 1. Модели Условий (Conditions)

**Файл**: `internal/domain/models/condition_test.go`

#### Тестируемая функциональность:
- Установка и получение условий (SetCondition, GetCondition)
- Проверка статуса условий (IsConditionTrue)
- Вспомогательные методы (Touch методы)
- Работа с ошибочными условиями

#### Тестовые сценарии:
- `TestMeta_SetCondition` - Установка условий в метаданные
- `TestMeta_GetCondition` - Получение условий по типу
- `TestMeta_IsConditionTrue` - Проверка статуса True/False
- `TestMeta_HelperMethods` - Тестирование вспомогательных методов
- `TestMeta_TouchMethods` - Методы обновления временных меток
- `TestConditionHelpers` - Работа с константами условий

#### Проверяемые условия:
- `ConditionReady` - Готовность ресурса
- `ConditionSynced` - Синхронизация с backend
- `ConditionValidated` - Валидация ресурса
- `ConditionError` - Ошибочное состояние

### 2. Network и NetworkBinding

**Файлы**: 
- Тесты моделей Network в `*_test.go` файлах

#### Тестируемая функциональность:
- Создание сетевых объектов
- Привязка сетей (NetworkBinding)
- Получение ключей и идентификаторов
- Управление поколениями (Generation)

#### Тестовые сценарии:
- `TestNetworkBinding_NewNetworkBinding` - Создание новых привязок
- `TestNetworkBinding_GetName` - Получение имени
- `TestNetworkBinding_GetNamespace` - Получение namespace
- `TestNetworkBinding_Key` - Формирование ключа ресурса
- `TestNetworkBinding_GetID` - Получение ID
- `TestNetworkBinding_GetGeneration` - Работа с generation
- `TestNetworkBinding_SetNetworkItem` - Установка сетевого элемента

- `TestNetwork_NewNetwork` - Создание сетей
- `TestNetwork_GetName` - Получение имени сети
- `TestNetwork_GetNamespace` - Получение namespace сети
- `TestNetwork_Key` - Ключи сетей
- `TestNetwork_GetID` - ID сетей
- `TestNetwork_GetGeneration` - Generation сетей
- `TestNetwork_SetIsBound` - Установка статуса привязки
- `TestNetwork_ClearBinding` - Очистка привязки

### 3. Протоколы и Порты

#### Тестируемая функциональность:
- Константы транспортных протоколов (TCP, UDP)
- Константы направления трафика (INGRESS, EGRESS)
- Валидация диапазонов портов
- Спецификации протоколов и портов

#### Тестовые сценарии:
- `TestTransportProtocolConstants` - Проверка TCP/UDP констант
- `TestTrafficConstants` - Проверка INGRESS/EGRESS
- `TestPortRange` - Валидация диапазонов портов:
  - Валидные диапазоны (80-8080)
  - Одиночные порты (80)
  - Невалидные диапазоны (8080-80)
  - Отрицательные порты
  - Порты вне диапазона (> 65535)
- `TestProtocolPorts` - Спецификации протоколов с портами

### 4. Бизнес-модели

#### Service (Сервисы)
- `TestService` - Создание и работа с сервисами
- Валидация портов ingress
- Управление метаданными

#### AddressGroup (Группы адресов)
- `TestAddressGroup` - Создание групп адресов
- Действия по умолчанию (ACCEPT/DROP)
- Настройки логирования и трассировки

#### AddressGroupBinding (Привязки групп адресов)
- `TestAddressGroupBinding` - Привязка групп к сервисам
- Создание портовых маппингов

#### ServiceAlias (Алиасы сервисов)
- `TestNewServiceAlias` - Создание алиасов
- Связывание с оригинальными сервисами

#### Rules (Правила)
- `TestRuleS2S` - Service-to-Service правила
- `TestAddressGroupPortMapping` - Маппинг портов групп адресов

### 5. Синхронизация

#### SyncStatus и SyncOp
- `TestSyncStatus` - Статус синхронизации
- `TestProtoToSyncOp` - Конвертация protobuf в SyncOp:
  - NoOp - нет операции
  - FullSync - полная синхронизация  
  - Upsert - создание/обновление
  - Delete - удаление
  - Обработка невалидных значений

- `TestSyncOpToProto` - Обратная конвертация
- `TestIsValidSyncOp` - Валидация операций синхронизации
- `TestDefaultSyncOp` - Операция по умолчанию
- `TestSyncOpString` - Строковое представление операций

### 6. Порты и опции

**Файл**: `internal/domain/ports/*_test.go`

#### Тестируемая функциональность:
- `TestWithSyncOp` - Опции синхронизации:
  - NoOp опции
  - FullSync опции  
  - Upsert опции
  - Delete опции

## Команды запуска

```bash
# Запуск всех unit тестов
go test -v ./internal/domain/...

# Запуск с покрытием
go test -cover ./internal/domain/...

# Детальное покрытие
go test -coverprofile=coverage.out ./internal/domain/...
go tool cover -html=coverage.out
```

## Статус и результаты

✅ **Все unit тесты проходят успешно**

Пример успешного прогона:
```
=== RUN   TestMeta_SetCondition
--- PASS: TestMeta_SetCondition (0.00s)
=== RUN   TestMeta_GetCondition  
--- PASS: TestMeta_GetCondition (0.00s)
=== RUN   TestPortRange
=== RUN   TestPortRange/Valid_range
=== RUN   TestPortRange/Single_port
=== RUN   TestPortRange/Invalid_range
--- PASS: TestPortRange (0.00s)
    --- PASS: TestPortRange/Valid_range (0.00s)
    --- PASS: TestPortRange/Single_port (0.00s)
    --- PASS: TestPortRange/Invalid_range (0.00s)
...
PASS
ok      netguard-pg-backend/internal/domain/models      1.761s
PASS  
ok      netguard-pg-backend/internal/domain/ports       1.553s
```

## Покрытие кода

Unit тесты обеспечивают высокое покрытие доменной логики:
- Все основные модели покрыты тестами
- Валидация входных данных
- Обработка граничных случаев
- Проверка бизнес-правил

## Рекомендации

1. **Добавить больше edge cases** для валидации портов
2. **Расширить тесты условий** для сложных сценариев
3. **Добавить benchmark тесты** для критичных операций
4. **Улучшить покрытие error cases** в конвертерах