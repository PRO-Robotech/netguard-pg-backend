# E2E (End-to-End) тесты

E2E тесты проверяют полный жизненный цикл системы от API до хранилища данных.

## Расположение

E2E тесты распределены по нескольким местам:
- `scripts/test-complete.sh` - Комплексные системные тесты
- `internal/application/validation/integration/` - Интеграционные тесты валидации
- Makefile targets для E2E сценариев

## Системные E2E тесты

### Script-based тесты
**Файл**: `scripts/test-complete.sh`

#### Инфраструктурные тесты:
- ✅ **Namespace существует** - Проверка K8s namespace
- 🚀 **Развертывания готовы** - Проверка deployments  
- 🟢 **Поды запущены** - Состояние pods
- 🔌 **Сервисы доступны** - Доступность K8s services

#### API тесты:
- 🔗 **APIService зарегистрирован** - Регистрация в K8s API
- 🎯 **API ресурсы обнаруживаются** - Discovery API
- 📝 **CRUD операции работают** - Полный цикл создания/чтения/обновления/удаления

#### Тесты подключения:
- 🔄 **Backend доступен** - Доступность gRPC backend
- ❤️ **Health endpoints работают** - Health check endpoints

#### Тесты качества:
- 📊 **Логи без критических ошибок** - Анализ логов

### Режимы запуска:
```bash
./test-complete.sh              # Все тесты
./test-complete.sh quick        # Быстрая проверка
./test-complete.sh performance  # Нагрузочное тестирование  
./test-complete.sh status       # Детальный статус
./test-complete.sh logs         # Показать логи
```

## Application E2E тесты

### Integration Validation тесты
**Файл**: `internal/application/validation/integration/*_test.go`

#### Статус: ⚠️ Один провалившийся тест

#### Успешные тесты (16/17):
- `TestIntegration_AddressGroupBindingValidation` ✅
- `TestIntegration_AddressGroupBindingReferences` ✅
- `TestIntegration_AddressGroupBindingValidateForCreation` ✅
- `TestIntegration_AddressGroupValidation` ✅
- `TestIntegration_AddressGroupBindingDependencies` ✅
- `TestIntegration_AddressGroupPortMappingValidation` ✅
- `TestIntegration_AddressGroupPortMappingReferences` ✅
- `TestIntegration_AddressGroupPortMappingValidateForCreation` ✅
- `TestIntegration_IEAgAgRuleValidation` ✅
- `TestIntegration_IEAgAgRuleReferences` ✅
- `TestIntegration_IEAgAgRuleValidateForCreation` ✅
- `TestIntegration_RuleS2SValidation` ✅
- `TestIntegration_RuleS2SReferences` ✅
- `TestIntegration_RuleS2SValidateForCreation` ✅
- `TestIntegration_ServiceAliasValidation` ✅
- `TestIntegration_ServiceValidation` ✅
- `TestIntegration_ServiceDependencies` ✅
- `TestIntegration_ServiceReferences` ✅
- `TestIntegration_ServiceAliasDependencies` ✅

#### Проблемный тест:
- `TestIntegration_AddressGroupDependencies` ❌
  - **Проблема**: Ожидается ошибка DependencyExistsError при удалении AddressGroup с зависимостями
  - **Результат**: Получается nil (ошибки нет)
  - **Нужно исправить**: Логика проверки зависимостей

## Полные E2E сценарии

### 1. Жизненный цикл Service
1. **Создание** сервиса через API
2. **Валидация** входных данных
3. **Сохранение** в репозиторий
4. **Обработка условий** (Synced, Validated, Ready)
5. **Синхронизация** с sgroups
6. **Чтение** через API
7. **Обновление** сервиса
8. **Удаление** с проверкой зависимостей

### 2. Жизненный цикл AddressGroup
1. **Создание** группы адресов
2. **Привязка** к сервисам (AddressGroupBinding)
3. **Создание портовых маппингов** (AddressGroupPortMapping)
4. **Генерация IEAgAg правил**
5. **Проверка зависимостей** при удалении

### 3. Service-to-Service правила
1. **Создание** исходного и целевого сервисов
2. **Создание алиасов** (ServiceAlias)
3. **Создание RuleS2S**
4. **Автогенерация IEAgAgRule**
5. **Синхронизация** с внешними системами

## Команды запуска

```bash
# Все E2E тесты
make test-e2e

# Integration тесты
go test -v ./internal/application/validation/integration/...

# Системные тесты
./scripts/test-complete.sh

# Конкретный integration тест
go test -v ./internal/application/validation/integration/ -run TestIntegration_AddressGroupDependencies
```

## Покрываемые сценарии

### Позитивные сценарии:
- ✅ Создание всех типов ресурсов
- ✅ Валидация корректных данных
- ✅ Обновление ресурсов
- ✅ Чтение ресурсов
- ✅ Каскадные операции

### Негативные сценарии:
- ✅ Валидация некорректных данных
- ✅ Попытки создания дублирующихся ресурсов
- ⚠️ Проверка зависимостей при удалении (1 проблема)
- ✅ Обработка отсутствующих ссылок

### Граничные случаи:
- ✅ Пустые списки
- ✅ Максимальные размеры данных
- ✅ Конкурентный доступ (частично)

## Метрики и результаты

### Время выполнения:
- Integration тесты: ~0.668s
- Unit тесты: ~1.5s
- API тесты: ~0.251s

### Покрытие:
- Бизнес-логика: Высокое
- API слой: Высокое  
- Репозиторий: Средний (только in-memory)
- K8s интеграция: Средний

## Нерешенные проблемы

### 1. Зависимости при удалении
**Тест**: `TestIntegration_AddressGroupDependencies`
**Проблема**: Не работает проверка зависимостей
**Влияние**: Можно удалить AddressGroup, на которую ссылается AddressGroupBinding

### 2. Отсутствие PostgreSQL E2E
**Проблема**: Все E2E тесты используют in-memory репозиторий
**Влияние**: Не проверяется production сценарий

### 3. Нет нагрузочных тестов
**Проблема**: Не проверяется поведение под нагрузкой
**Влияние**: Неизвестна производительность

## Рекомендации

### 1. Исправить проверку зависимостей
```go
// Добавить логику в DeleteAddressGroupsByIDs:
if hasBindings := checkAddressGroupBindings(agID); hasBindings {
    return DependencyExistsError{...}
}
```

### 2. Добавить PostgreSQL E2E тесты
- Настроить тестовую БД
- Создать E2E тесты с реальным PostgreSQL
- Проверить миграции и транзакции

### 3. Создать нагрузочные тесты
- Тестирование с большим количеством ресурсов
- Проверка производительности API
- Стресс-тестирование concurrent access

### 4. Улучшить системные тесты
- Автоматизация развертывания test environment
- Мониторинг метрик во время тестов
- Интеграция с CI/CD pipeline