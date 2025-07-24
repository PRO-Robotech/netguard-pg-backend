# Исправление проблемы с отсутствующими conditions в IEAGAG правилах

## Проблема

IEAGAG правила создавались автоматически из RuleS2S, но в них отсутствовали conditions (пустой массив `"conditions": []`). Это аналогично проблеме, которая была с PortMapping.

Пример проблемного JSON из issue:
```json
{
  "selfRef": {
    "name": "egr-438734a6-8907-cfb2-6dc9-1273a2d0f4c3",
    "namespace": "application"
  },
  "transport": "TCP",
  "traffic": "Egress",
  "addressGroupLocal": {
    "identifier": {
      "name": "egress-infra",
      "namespace": "infra"
    }
  },
  "addressGroup": {
    "identifier": {
      "name": "ingress-application",
      "namespace": "application"
    }
  },
  "ports": [
    {
      "source": "",
      "destination": "90"
    }
  ],
  "action": "ACCEPT",
  "logs": true,
  "priority": 100,
  "meta": {
    "uid": "0591c071-6954-4ed9-a50e-7e1a468d1148",
    "resourceVersion": "1752749568943987923",
    "generation": "1",
    "creationTs": "2025-07-17T10:52:48.943987340Z",
    "labels": {},
    "annotations": {},
    "conditions": [],  // ← ПРОБЛЕМА: пустой массив conditions
    "observedGeneration": "1"
  }
}
```

## Анализ

1. **Функция ProcessIEAgAgRuleConditions уже существует** в `condition_manager.go` и правильно устанавливает conditions:
   - Synced condition = true после успешного commit
   - Validated condition после валидации
   - Ready condition после проверки зависимостей

2. **Найдены 3 места где IEAGAG правила создаются автоматически**, но ProcessIEAgAgRuleConditions НЕ вызывается:
   - `updateIEAgAgRulesForRuleS2S` (строка ~935)
   - `SyncRuleS2SWithIEAgAgRules` (строка ~1840)
   - `SyncRuleS2S` (строка ~1950)

3. **Паттерн решения** найден в обработке PortMapping:
   - После sync/commit операции вызывается ProcessConditions
   - Затем вызывается saveResourceConditions для сохранения

## Решение

Добавлены вызовы `ProcessIEAgAgRuleConditions` во всех трех местах где создаются IEAGAG правила:

### 1. В функции updateIEAgAgRulesForRuleS2S

**Файл:** `/Users/zhd/Projects/newPro/netguard-pg-backend/internal/application/services/service.go`  
**Строки:** 938-944

```go
// Sync all new rules at once
if len(allNewRules) > 0 {
    if err = writer.SyncIEAgAgRules(ctx, allNewRules, nil, ports.WithSyncOp(syncOp)); err != nil {
        return errors.Wrap(err, "failed to sync new IEAgAgRules")
    }
    // Process conditions for newly created IEAGAG rules after sync
    for i := range allNewRules {
        s.conditionManager.ProcessIEAgAgRuleConditions(ctx, &allNewRules[i])
        if err := s.conditionManager.saveResourceConditions(ctx, &allNewRules[i]); err != nil {
            log.Printf("Failed to save IEAgAgRule conditions for %s: %v", allNewRules[i].Key(), err)
        }
    }
}
```

### 2. В функции SyncRuleS2SWithIEAgAgRules

**Файл:** `/Users/zhd/Projects/newPro/netguard-pg-backend/internal/application/services/service.go`  
**Строки:** 1844-1850

```go
if err := writer.Commit(); err != nil {
    return errors.Wrap(err, "failed to commit")
}

// Process conditions for newly created IEAGAG rules after successful commit
for i := range allNewRules {
    s.conditionManager.ProcessIEAgAgRuleConditions(ctx, &allNewRules[i])
    if err := s.conditionManager.saveResourceConditions(ctx, &allNewRules[i]); err != nil {
        log.Printf("Failed to save IEAgAgRule conditions for %s: %v", allNewRules[i].Key(), err)
    }
}
```

### 3. В функции SyncRuleS2S

**Файл:** `/Users/zhd/Projects/newPro/netguard-pg-backend/internal/application/services/service.go`  
**Строки:** 1954-1960

```go
if err = writer.Commit(); err != nil {
    return errors.Wrap(err, "failed to commit")
}

// Process conditions for newly created IEAGAG rules after successful commit
for i := range allNewRules {
    s.conditionManager.ProcessIEAgAgRuleConditions(ctx, &allNewRules[i])
    if err := s.conditionManager.saveResourceConditions(ctx, &allNewRules[i]); err != nil {
        log.Printf("Failed to save IEAgAgRule conditions for %s: %v", allNewRules[i].Key(), err)
    }
}
```

## Ожидаемый результат

После исправления IEAGAG правила будут создаваться с корректными conditions:

```json
{
  "meta": {
    "conditions": [
      {
        "type": "Synced",
        "status": "True",
        "reason": "Synced",
        "message": "IEAgAgRule committed to backend successfully"
      },
      {
        "type": "Validated", 
        "status": "True",
        "reason": "Validated",
        "message": "IEAgAgRule passed validation"
      },
      {
        "type": "Ready",
        "status": "True", 
        "reason": "Ready",
        "message": "IEAgAgRule is ready, 1 ports configured"
      }
    ]
  }
}
```

## Тестирование

Для тестирования исправления:

1. Создать RuleS2S правило
2. Дождаться автоматического создания IEAGAG правил
3. Проверить что в IEAGAG правилах присутствуют conditions:
   - Synced = True
   - Validated = True  
   - Ready = True (если все зависимости найдены)

## Заключение

Проблема решена добавлением вызовов `ProcessIEAgAgRuleConditions` во всех местах автоматического создания IEAGAG правил. Использован тот же паттерн, что применяется для других ресурсов (PortMapping, ServiceAlias и т.д.).