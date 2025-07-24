# Исправление проблемы с отсутствующими conditions при изменении портов сервисов

## Проблема

При изменении портов в сервисах IEAGAG правила пересоздавались без conditions. Пользователь сообщил: "Сначала при создании правил кондишены были, как только поменял порты в сервисах, правила IEAGAG правила пересоздались без кондишенов".

### Анализ проблемы

1. **Функция SyncServices** обрабатывает обновления сервисов
2. При изменении сервиса система находит связанные RuleS2S через `findRuleS2SForServices`
3. Для найденных RuleS2S вызывается `updateIEAgAgRulesForRuleS2S` для пересоздания IEAGAG правил
4. **Проблема:** conditions для пересозданных IEAGAG правил обрабатывались внутри `updateIEAgAgRulesForRuleS2S` ДО финального commit транзакции
5. Это приводило к тому, что conditions могли не сохраняться корректно

## Решение

Модифицирована функция `SyncServices` для корректной обработки conditions после commit транзакции.

### Внесенные изменения

**Файл:** `/Users/zhd/Projects/newPro/netguard-pg-backend/internal/application/services/service.go`  
**Функция:** `SyncServices` (строки 1028-1076)

#### Было:
```go
// Update IEAgAgRules for affected RuleS2S
if len(affectedRules) > 0 {
    if err = s.updateIEAgAgRulesForRuleS2S(ctx, writer, affectedRules, models.SyncOpFullSync); err != nil {
        writer.Abort()
        return errors.Wrap(err, "failed to update IEAgAgRules for affected RuleS2S")
    }
}

if err = writer.Commit(); err != nil {
    return errors.Wrap(err, "failed to commit")
}

// Обработка conditions после успешного commit
for i := range services {
    s.conditionManager.ProcessServiceConditions(ctx, &services[i])
    if err := s.conditionManager.saveResourceConditions(ctx, &services[i]); err != nil {
        log.Printf("Failed to save service conditions for %s: %v", services[i].Key(), err)
    }
}
```

#### Стало:
```go
// Update IEAgAgRules for affected RuleS2S and collect created rules
var allNewIEAgAgRules []models.IEAgAgRule
if len(affectedRules) > 0 {
    // Get reader that can see changes in current transaction
    txReader, err := s.registry.ReaderFromWriter(ctx, writer)
    if err != nil {
        writer.Abort()
        return errors.Wrap(err, "failed to get transaction reader")
    }
    defer txReader.Close()

    // Generate IEAGAG rules for affected RuleS2S to collect them for conditions processing
    for _, rule := range affectedRules {
        ieAgAgRules, err := s.GenerateIEAgAgRulesFromRuleS2SWithReader(ctx, txReader, rule)
        if err != nil {
            writer.Abort()
            return errors.Wrapf(err, "failed to generate IEAgAgRules for RuleS2S %s", rule.Key())
        }
        allNewIEAgAgRules = append(allNewIEAgAgRules, ieAgAgRules...)
    }

    if err = s.updateIEAgAgRulesForRuleS2S(ctx, writer, affectedRules, models.SyncOpFullSync); err != nil {
        writer.Abort()
        return errors.Wrap(err, "failed to update IEAgAgRules for affected RuleS2S")
    }
}

if err = writer.Commit(); err != nil {
    return errors.Wrap(err, "failed to commit")
}

// Обработка conditions после успешного commit
for i := range services {
    s.conditionManager.ProcessServiceConditions(ctx, &services[i])
    if err := s.conditionManager.saveResourceConditions(ctx, &services[i]); err != nil {
        log.Printf("Failed to save service conditions for %s: %v", services[i].Key(), err)
    }
}

// Process conditions for IEAGAG rules created during service update
for i := range allNewIEAgAgRules {
    if err := s.conditionManager.ProcessIEAgAgRuleConditions(ctx, &allNewIEAgAgRules[i]); err != nil {
        log.Printf("Failed to process IEAgAgRule conditions for %s: %v", allNewIEAgAgRules[i].Key(), err)
    }
    if err := s.conditionManager.saveResourceConditions(ctx, &allNewIEAgAgRules[i]); err != nil {
        log.Printf("Failed to save IEAgAgRule conditions for %s: %v", allNewIEAgAgRules[i].Key(), err)
    }
}
```

### Ключевые изменения

1. **Сбор информации о создаваемых IEAGAG правилах** перед их обновлением
2. **Использование transaction reader** для получения актуальной информации в рамках транзакции
3. **Обработка conditions ПОСЛЕ commit** транзакции для всех пересозданных IEAGAG правил
4. **Добавление обработки ошибок** для ProcessIEAgAgRuleConditions и saveResourceConditions

## Ожидаемый результат

После исправления при изменении портов в сервисах:

1. **IEAGAG правила пересоздаются** с новыми портами
2. **Conditions обрабатываются корректно** после commit транзакции
3. **Каждое пересозданное IEAGAG правило получает conditions:**
   - `Synced = True` - после успешного commit
   - `Validated = True` - после прохождения валидации
   - `Ready = True` - после проверки всех зависимостей

## Тестирование

### Ручное тестирование

1. **Получить текущие IEAGAG правила:**
   ```bash
   kubectl get ieagagrules -A -o json | jq '.items[] | {name: .metadata.name, namespace: .metadata.namespace, conditions: .status.conditions}'
   ```

2. **Изменить порты в сервисе:**
   ```bash
   kubectl patch service backend -n application --type='merge' -p='{"spec":{"ingressPorts":[{"protocol":"TCP","port":"90","description":"Modified HTTP"},{"protocol":"TCP","port":"91","description":"Modified HTTPS"}]}}'
   ```

3. **Проверить пересозданные IEAGAG правила:**
   ```bash
   # Подождать несколько секунд для обработки
   sleep 5
   
   # Проверить правила с новыми портами
   kubectl get ieagagrules -A -o json | jq '.items[] | select(.spec.ports[]?.destination | contains("90") or contains("91")) | {name: .metadata.name, ports: .spec.ports, conditions: .status.conditions}'
   ```

4. **Проверить логи backend сервиса:**
   ```bash
   kubectl logs -f deployment/netguard-backend | grep -i "ieagag\|condition"
   ```

### Автоматическое тестирование

Создан тестовый скрипт `test_service_port_change_conditions.go`, который:
1. Получает существующие IEAGAG правила
2. Изменяет порты в сервисе backend
3. Проверяет что пересозданные IEAGAG правила имеют conditions
4. Выводит детальный отчет о результатах

## Связанные исправления

Это исправление дополняет предыдущие изменения:
- Добавление ProcessIEAgAgRuleConditions в `updateIEAgAgRulesForRuleS2S`
- Добавление ProcessIEAgAgRuleConditions в `SyncRuleS2SWithIEAgAgRules`  
- Добавление ProcessIEAgAgRuleConditions в `SyncRuleS2S`

Теперь все места автоматического создания/пересоздания IEAGAG правил корректно обрабатывают conditions.

## Заключение

Проблема с отсутствующими conditions при изменении портов сервисов решена. Теперь все IEAGAG правила, пересоздаваемые при обновлении сервисов, будут иметь корректные conditions, что обеспечивает полную информацию о состоянии правил в системе.