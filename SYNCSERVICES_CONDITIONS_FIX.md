# Исправление проблемы с отсутствующими conditions в IEAGAG правилах при изменении портов сервисов

## Проблема

При изменении портов в сервисах IEAGAG правила пересоздавались без conditions. Пользователь сообщил в логах:

```
I0717 11:41:45.060353       1 port_utils.go:16] 🔧 ParsePortRanges: parsing port string '8082'
...
2025/07/17 11:41:45 ✅ COMMIT: Database commit operation completed successfully
```

Логи показывают, что сервисы обрабатываются корректно с conditions, но IEAGAG правила создаются без conditions.

## Анализ проблемы

### Корневая причина

1. **Приватная функция syncServices** (строка 621) вызывается при обновлении сервисов
2. В ней вызывается `updateIEAgAgRulesForRuleS2SWithReader` (строка 711) для пересоздания IEAGAG правил
3. **Проблема:** В оригинальной функции `updateIEAgAgRulesForRuleS2SWithReader` conditions обрабатывались ДО commit транзакции
4. После commit в syncServices (строка 741) НЕ БЫЛО обработки conditions для пересозданных IEAGAG правил
5. Функция завершалась без обработки conditions

### Отличие от публичной SyncServices

Публичная функция `SyncServices` (строка 972) уже была исправлена ранее и корректно обрабатывает conditions после commit. Но приватная функция `syncServices` использовалась в других местах и не имела такой обработки.

## Решение

### 1. Добавление сбора информации о IEAGAG правилах

**Файл:** `/Users/zhd/Projects/newPro/netguard-pg-backend/internal/application/services/service.go`  
**Строки:** 686, 710-717

```go
// Если это не удаление, обновляем связанные ресурсы
var allNewIEAgAgRules []models.IEAgAgRule
if syncOp != models.SyncOpDelete {
    // ...
    
    // Собираем информацию о IEAGAG правилах, которые будут созданы
    for _, rule := range affectedRules {
        ieAgAgRules, err := s.GenerateIEAgAgRulesFromRuleS2SWithReader(ctx, txReader, rule)
        if err != nil {
            return errors.Wrapf(err, "failed to generate IEAgAgRules for RuleS2S %s", rule.Key())
        }
        allNewIEAgAgRules = append(allNewIEAgAgRules, ieAgAgRules...)
    }
}
```

### 2. Создание функции без обработки conditions

**Файл:** `/Users/zhd/Projects/newPro/netguard-pg-backend/internal/application/services/service.go`  
**Строки:** 920-982

Создана новая функция `updateIEAgAgRulesForRuleS2SWithReaderNoConditions`, которая:
- Создает IEAGAG правила без обработки conditions
- Используется в syncServices для избежания дублирования обработки conditions
- Содержит комментарий: `// NOTE: Conditions are NOT processed here - they will be processed by the caller`

### 3. Использование новой функции в syncServices

**Файл:** `/Users/zhd/Projects/newPro/netguard-pg-backend/internal/application/services/service.go`  
**Строки:** 719-725

```go
// Обновляем IE AG AG правила для затронутых RuleS2S, используя reader из транзакции
// Используем версию без обработки conditions, так как conditions будут обработаны после commit
if len(affectedRules) > 0 {
    if err = s.updateIEAgAgRulesForRuleS2SWithReaderNoConditions(ctx, writer, txReader, affectedRules, models.SyncOpFullSync); err != nil {
        return errors.Wrap(err, "failed to update IEAgAgRules for affected RuleS2S")
    }
}
```

### 4. Обработка conditions после commit

**Файл:** `/Users/zhd/Projects/newPro/netguard-pg-backend/internal/application/services/service.go`  
**Строки:** 755-763

```go
// Process conditions for IEAGAG rules created during service sync
for i := range allNewIEAgAgRules {
    if err := s.conditionManager.ProcessIEAgAgRuleConditions(ctx, &allNewIEAgAgRules[i]); err != nil {
        log.Printf("Failed to process IEAgAgRule conditions for %s: %v", allNewIEAgAgRules[i].Key(), err)
    }
    if err := s.conditionManager.saveResourceConditions(ctx, &allNewIEAgAgRules[i]); err != nil {
        log.Printf("Failed to save IEAgAgRule conditions for %s: %v", allNewIEAgAgRules[i].Key(), err)
    }
}
```

## Ожидаемый результат

После исправления при изменении портов в сервисах:

1. **IEAGAG правила пересоздаются** с новыми портами
2. **Conditions обрабатываются корректно** после commit транзакции
3. **Каждое пересозданное IEAGAG правило получает conditions:**
   - `Synced = True` - после успешного commit
   - `Validated = True` - после прохождения валидации
   - `Ready = True` - после проверки всех зависимостей

## Тестирование

### Сценарий тестирования

1. **Изменить порты в сервисе:**
   ```bash
   kubectl patch service backend -n application --type='merge' -p='{"spec":{"ingressPorts":[{"protocol":"TCP","port":"8082"},{"protocol":"TCP","port":"8083"}]}}'
   ```

2. **Проверить пересозданные IEAGAG правила:**
   ```bash
   kubectl get ieagagrules -A -o json | jq '.items[] | select(.spec.ports[]?.destination | contains("8082") or contains("8083")) | {name: .metadata.name, conditions: .status.conditions}'
   ```

3. **Проверить логи backend сервиса:**
   ```bash
   kubectl logs -f deployment/netguard-backend | grep -i "ieagag\|condition"
   ```

### Ожидаемые логи

После исправления в логах должны появиться сообщения:
```
Process conditions for IEAGAG rules created during service sync
✅ ConditionManager: Setting Validated=true for application/egr-xxxxx
✅ ConditionManager: Setting Ready=true for application/egr-xxxxx
```

## Связанные исправления

Это исправление дополняет предыдущие изменения:
- ✅ Добавление ProcessIEAgAgRuleConditions в `updateIEAgAgRulesForRuleS2S`
- ✅ Добавление ProcessIEAgAgRuleConditions в `SyncRuleS2SWithIEAgAgRules`  
- ✅ Добавление ProcessIEAgAgRuleConditions в `SyncRuleS2S`
- ✅ Добавление ProcessIEAgAgRuleConditions в публичную `SyncServices`
- ✅ **Добавление ProcessIEAgAgRuleConditions в приватную `syncServices`** (новое исправление)

## Заключение

Проблема с отсутствующими conditions при изменении портов сервисов решена. Теперь все места автоматического создания/пересоздания IEAGAG правил корректно обрабатывают conditions, включая случай изменения портов сервисов через приватную функцию syncServices.

Ключевое отличие этого исправления - создание отдельной версии функции обновления IEAGAG правил без обработки conditions, что позволяет избежать дублирования и обеспечить правильный порядок операций: создание → commit → обработка conditions.